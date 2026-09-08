package chat

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	chatlib "go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/chat_event_bus"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// SendMessage sends a message in a conversation.
func (s *Service) SendMessage(
	ctx context.Context,
	req *connect.Request[api.SendMessageRequest],
) (*connect.Response[api.SendMessageResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"conversation_id", req.Msg.ConversationId,
		"media_count", len(req.Msg.MediaIds),
	)

	// Validate message has content
	if req.Msg.Text == "" && len(req.Msg.MediaIds) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("message must have text or media"))
	}

	conversation, err := s.requireConversationAccess(ctx, req.Msg.ConversationId, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	// Get sender info for mention processing
	sender := &models.User{}
	if err := s.storage.GetByID(ctx, authInfo.UserID, sender); err != nil {
		return nil, connecterr.Internal(ctx, "SendMessage", err, "detail",

			// Process @-mentions: add mentioned users to conversation and create system messages
			"failed to fetch sender")
	}

	mentionedUsers, err := s.processMentions(ctx, conversation, req.Msg.Text, authInfo.UserID, sender.Name)
	if err != nil {
		return nil, connecterr.Internal(ctx, "SendMessage", err, "detail",

			// Extract mentioned user IDs for notification customization
			"failed to process mentions")
	}

	var mentionedUserIDs []string
	for _, mu := range mentionedUsers {
		mentionedUserIDs = append(mentionedUserIDs, mu.UserID)
	}

	// If message has media, append to parent entity
	if len(req.Msg.MediaIds) > 0 {
		if err := s.appendMediaToParentEntity(ctx, conversation, req.Msg.MediaIds); err != nil {
			// Log error but don't fail the message send
			// Media is still in the chat, just not in parent carousel
			logger.ErrorContext(
				ctx, "failed to append media to parent entity",
				"error", err,
				"conversation_id", req.Msg.ConversationId,
			)
		} else {
			logger.InfoContext(
				ctx, "media appended to parent entity",
				"media_count", len(req.Msg.MediaIds),
			)
		}
	}

	// Create message with per-user read status
	// Initialize map with sender marked as read, others as unread
	now := clock.UnixSec(ctx)
	participantReadStatus := make(map[string]bool)
	for _, participantID := range conversation.ParticipantIds {
		participantReadStatus[participantID] = (participantID == authInfo.UserID)
	}

	userMessage := &models.UserChatMessage{
		SenderId: authInfo.UserID,
		Text:     req.Msg.Text,
		MediaIds: req.Msg.MediaIds,
	}

	// When this is a reply, denormalize a quoted snippet of the parent message.
	if req.Msg.ReplyToMessageId != nil && *req.Msg.ReplyToMessageId != "" {
		userMessage.ReplyTo = s.buildReplyContext(ctx, req.Msg.ConversationId, *req.Msg.ReplyToMessageId)
	}

	message := &models.ChatMessage{
		ConversationId:        req.Msg.ConversationId,
		SentAtUnixSec:         now,
		ParticipantIdToIsRead: participantReadStatus,
		Message: &models.ChatMessage_UserMessage{
			UserMessage: userMessage,
		},
	}

	messageID, err := s.storage.Insert(ctx, message)
	if err != nil {
		logger.Error("failed to insert message", "error", err)
		return nil, connecterr.Internal(ctx, "SendMessage", err)
	}

	logger = logger.With("message_id", messageID)

	// Update conversation's last message timestamp
	conversation.LastMessageAtUnixSec = now
	if err := s.storage.Update(ctx, conversation); err != nil {
		logger.Error("failed to update conversation timestamp", "error", err)
		// Non-fatal
	}

	// Check if this conversation is for a request and transition state if needed
	if err := s.maybeTransitionRequestState(ctx, conversation, authInfo.UserID); err != nil {
		logger.Warn("failed to transition request state", "error", err)
		// Non-fatal - don't fail message send
	}

	message.Id = messageID

	// Publish to the chat bus. The stream subscriber fans out to live streams;
	// the notification subscriber dispatches push to inactive participants.
	if s.bus != nil {
		if err := s.bus.Publish(ctx, chat_event_bus.KindUserMessage, message, conversation,
			chat_event_bus.WithMentionedUserIDs(mentionedUserIDs)); err != nil {
			logger.WarnContext(ctx, "chat bus publish failed", "error", err)
		}
	}

	// Mark all watchers of the associated item as unread (except the sender).
	if watchType, watchItemID := watchKeyFromTopic(conversation.Topic); watchType != models.WatchedItemType_WATCHED_ITEM_TYPE_UNSPECIFIED {
		ws := storage.NewWatchStorage(s.storage)
		if err := ws.MarkUnreadForWatchers(ctx, watchType, watchItemID, authInfo.UserID); err != nil {
			logger.Warn("failed to mark watchers unread on message send", "error", err)
		}
	}

	logger.Info("sent message")

	return connect.NewResponse(&api.SendMessageResponse{
		MessageId:     messageID,
		SentAtUnixSec: now,
	}), nil
}

// watchKeyFromTopic derives the watch key (type, item ID) from a conversation topic.
func watchKeyFromTopic(topic *models.ConversationTopic) (models.WatchedItemType, string) {
	if topic == nil {
		return models.WatchedItemType_WATCHED_ITEM_TYPE_UNSPECIFIED, ""
	}
	switch t := topic.TopicId.(type) {
	case *models.ConversationTopic_GearId:
		return models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, t.GearId
	case *models.ConversationTopic_TransferId:
		// Transfer conversations map to the gear — but we only have the transfer ID here.
		// The lazy backfill and explicit watch creation on ExpressInterest handle this;
		// skip watch update for transfer-topic conversations to avoid an extra DB lookup.
		return models.WatchedItemType_WATCHED_ITEM_TYPE_UNSPECIFIED, ""
	case *models.ConversationTopic_ExperienceId:
		return models.WatchedItemType_WATCHED_ITEM_TYPE_EXPERIENCE, t.ExperienceId
	case *models.ConversationTopic_RequestId:
		return models.WatchedItemType_WATCHED_ITEM_TYPE_REQUEST, t.RequestId
	default:
		return models.WatchedItemType_WATCHED_ITEM_TYPE_UNSPECIFIED, ""
	}
}

// GetConversationHistory retrieves message history for a conversation.
func (s *Service) GetConversationHistory(
	ctx context.Context,
	req *connect.Request[api.GetConversationHistoryRequest],
) (*connect.Response[api.GetConversationHistoryResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"conversation_id", req.Msg.ConversationId,
	)

	// Verify user has access to this conversation and get conversation details
	conversation, err := s.requireConversationAccess(ctx, req.Msg.ConversationId, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	// Query messages for this conversation
	messages, err := s.storage.QueryByField(ctx, "conversation_id", req.Msg.ConversationId, &models.ChatMessage{})
	if err != nil {
		logger.Error("failed to query messages", "error", err)
		return nil, connecterr.Internal(ctx, "GetConversationHistory", err)
	}

	// Convert to MessageHistoryItem and apply filters/pagination
	maxMessages := req.Msg.MaxMessages
	if maxMessages == 0 {
		maxMessages = 50
	}

	var historyItems []*api.MessageHistoryItem
	for _, msg := range messages {
		chatMsg := msg.(*models.ChatMessage)

		// Apply before_unix_sec filter if specified
		if req.Msg.BeforeUnixSec > 0 && chatMsg.SentAtUnixSec >= req.Msg.BeforeUnixSec {
			continue
		}

		// Convert to API format
		historyItem, err := s.chatMessageToHistoryItem(ctx, chatMsg, authInfo.UserID)
		if err != nil {
			return nil, connecterr.Internal(ctx, "GetConversationHistory", err, "detail", "failed to convert message")
		}

		historyItems = append(historyItems, historyItem)
	}

	// Sort by timestamp descending (newest first)
	// Use message ID as secondary sort key for messages with same timestamp
	for i := 0; i < len(historyItems); i++ {
		for j := i + 1; j < len(historyItems); j++ {
			shouldSwap := historyItems[i].SentAtUnixSec < historyItems[j].SentAtUnixSec
			// If timestamps are equal, sort by ID descending to maintain chronological order
			if historyItems[i].SentAtUnixSec == historyItems[j].SentAtUnixSec {
				shouldSwap = historyItems[i].MessageId < historyItems[j].MessageId
			}
			if shouldSwap {
				historyItems[i], historyItems[j] = historyItems[j], historyItems[i]
			}
		}
	}

	// Apply limit and check if there are more
	hasMore := false
	if int32(len(historyItems)) > maxMessages {
		hasMore = true
		historyItems = historyItems[:maxMessages]
	}

	// Build response with topic
	response := &api.GetConversationHistoryResponse{
		Messages: historyItems,
		HasMore:  hasMore,
	}

	response.Topic = chatlib.ModelsTopicToAPI(conversation.GetTopic())

	return connect.NewResponse(response), nil
}

// MarkMessagesRead marks messages as read in a conversation.
func (s *Service) MarkMessagesRead(
	ctx context.Context,
	req *connect.Request[api.MarkMessagesReadRequest],
) (*connect.Response[api.MarkMessagesReadResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"conversation_id", req.Msg.ConversationId,
	)

	// Verify user has access to this conversation (participant or community member).
	conversation, err := s.requireConversationAccess(ctx, req.Msg.ConversationId, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	// Query messages for this conversation that are not from the current user
	messages, err := s.storage.QueryByField(ctx, "conversation_id", req.Msg.ConversationId, &models.ChatMessage{})
	if err != nil {
		logger.Error("failed to query messages", "error", err)
		return nil, connecterr.Internal(ctx, "MarkMessagesRead", err)
	}

	markedCount := int32(0)
	for _, msg := range messages {
		chatMsg := msg.(*models.ChatMessage)

		// Skip messages from current user
		if getSenderID(chatMsg) == authInfo.UserID {
			continue
		}

		// Skip already read messages by current user
		if chatMsg.ParticipantIdToIsRead[authInfo.UserID] {
			continue
		}

		// Apply timestamp filter if specified
		if req.Msg.UpToUnixSec > 0 && chatMsg.SentAtUnixSec > req.Msg.UpToUnixSec {
			continue
		}

		// Mark as read for current user
		if chatMsg.ParticipantIdToIsRead == nil {
			chatMsg.ParticipantIdToIsRead = make(map[string]bool)
		}
		chatMsg.ParticipantIdToIsRead[authInfo.UserID] = true
		if err := s.storage.Update(ctx, chatMsg); err != nil {
			logger.Error("failed to mark message as read", "message_id", chatMsg.Id, "error", err)
			continue
		}

		markedCount++
	}

	// After marking per-message read flags, check whether the user is now fully
	// caught up (no unread messages from other participants remain). If so, also
	// mark the associated watch entry read so the inbox row clears regardless of
	// how the user reached this conversation (e.g. via a deep-linked notification
	// that bypassed the inbox tap path).
	remainingUnread := 0
	for _, msg := range messages {
		chatMsg := msg.(*models.ChatMessage)
		if getSenderID(chatMsg) == authInfo.UserID {
			continue
		}
		if !chatMsg.ParticipantIdToIsRead[authInfo.UserID] {
			remainingUnread++
		}
	}

	if remainingUnread == 0 {
		watchType, watchItemID, watchErr := s.resolveWatchKey(ctx, conversation.Topic)
		if watchErr != nil {
			logger.WarnContext(ctx, "failed to resolve watch key for conversation", "error", watchErr)
		} else if watchType != models.WatchedItemType_WATCHED_ITEM_TYPE_UNSPECIFIED {
			logger = logger.With("watch_type", watchType.String(), "watch_item_id", watchItemID)
			ws := storage.NewWatchStorage(s.storage)
			if err := ws.MarkRead(ctx, authInfo.UserID, watchType, watchItemID); err != nil {
				logger.WarnContext(ctx, "failed to mark watcher read", "error", err)
			}
		}
	}

	return connect.NewResponse(&api.MarkMessagesReadResponse{
		MessagesMarked: markedCount,
	}), nil
}

// resolveWatchKey derives the (watch type, item ID) for a conversation topic,
// including transfer topics which require a DB lookup to resolve the gear ID.
// Returns UNSPECIFIED for community-wide topics and nil topics.
func (s *Service) resolveWatchKey(ctx context.Context, topic *models.ConversationTopic) (models.WatchedItemType, string, error) {
	if topic == nil {
		return models.WatchedItemType_WATCHED_ITEM_TYPE_UNSPECIFIED, "", nil
	}
	switch t := topic.TopicId.(type) {
	case *models.ConversationTopic_GearId:
		return models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, t.GearId, nil
	case *models.ConversationTopic_TransferId:
		gearID, err := s.resolveTransferGearID(ctx, t.TransferId)
		if err != nil {
			return models.WatchedItemType_WATCHED_ITEM_TYPE_UNSPECIFIED, "", err
		}
		return models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, gearID, nil
	case *models.ConversationTopic_ExperienceId:
		return models.WatchedItemType_WATCHED_ITEM_TYPE_EXPERIENCE, t.ExperienceId, nil
	case *models.ConversationTopic_RequestId:
		return models.WatchedItemType_WATCHED_ITEM_TYPE_REQUEST, t.RequestId, nil
	default:
		return models.WatchedItemType_WATCHED_ITEM_TYPE_UNSPECIFIED, "", nil
	}
}

// resolveTransferGearID loads the transfer and returns its gear ID, used to map
// transfer-topic conversations to their associated gear watch entry.
func (s *Service) resolveTransferGearID(ctx context.Context, transferID string) (string, error) {
	transfer := &models.Transfer{}
	if err := s.storage.GetByID(ctx, transferID, transfer); err != nil {
		return "", fmt.Errorf("failed to load transfer %s: %w", transferID, err)
	}
	return transfer.GearId, nil
}

// maybeTransitionRequestState checks if a conversation is for a request and transitions
// the request state to OFFERS_RECEIVED if this is the first message from a non-requester.
func (s *Service) maybeTransitionRequestState(ctx context.Context, conversation *models.ChatConversation, senderID string) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "maybeTransitionRequestState",
		"conversation_id", conversation.Id,
		"sender_id", senderID,
	)

	// Check if this conversation is for a request
	requestID := conversation.GetTopic().GetRequestId()
	if requestID == "" {
		// Not a request conversation, nothing to do
		return nil
	}

	logger = logger.With("target_request_id", requestID)

	// Get the request
	request := &models.Request{}
	if err := s.storage.GetByID(ctx, requestID, request); err != nil {
		logger.Error("failed to get request", "error", err)
		return err
	}

	// Only transition if currently ACTIVE
	if request.State != models.RequestState_REQUEST_STATE_ACTIVE {
		return nil
	}

	// Only transition if sender is not the requester
	if senderID == request.RequesterId {
		return nil
	}

	// Transition to OFFERS_RECEIVED
	request.State = models.RequestState_REQUEST_STATE_OFFERS_RECEIVED
	if err := s.storage.Update(ctx, request); err != nil {
		logger.Error("failed to update request state", "error", err)
		return err
	}

	logger.InfoContext(ctx, "transitioned request state to OFFERS_RECEIVED")
	return nil
}

// checkArchivedConversationAccess checks if a user can post in an archived gear conversation.
// Returns an error if the conversation is archived and the user was not a participant before archiving.
