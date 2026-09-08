package chat

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// TopicQueryField returns the protosql column name prefix used to query conversations
// by topic type. The column names are prefixed with "topic_" because protosql flattens
// nested messages with the field name as prefix.
func TopicQueryField(topic *models.ConversationTopic) (queryField, topicID string, ok bool) {
	switch t := topic.GetTopicId().(type) {
	case *models.ConversationTopic_TransferId:
		return "topic_transfer_id", t.TransferId, true
	case *models.ConversationTopic_RequestId:
		return "topic_request_id", t.RequestId, true
	case *models.ConversationTopic_ExperienceId:
		return "topic_experience_id", t.ExperienceId, true
	case *models.ConversationTopic_GearId:
		return "topic_gear_id", t.GearId, true
	case *models.ConversationTopic_CommunityId:
		return "topic_community_id", t.CommunityId, true
	default:
		return "", "", false
	}
}

// CreateCommunityConversation creates (or retrieves an existing) community-wide conversation
// for the given community. Safe to call from any service; idempotent.
//
// Callers should treat a non-nil error as an internal failure — the community itself is
// created first, and a backfill job recovers any communities that miss this call.
func CreateCommunityConversation(
	ctx context.Context,
	protoStorage *storage.ProtoSQLStorage,
	chatConvStorage *storage.ChatConversationStorage,
	communityID string,
) (conversationID string, err error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CreateCommunityConversation",
		"community_id", communityID,
	)

	topic := &models.ConversationTopic{
		TopicId: &models.ConversationTopic_CommunityId{CommunityId: communityID},
	}

	// Check for an existing conversation to make this call idempotent.
	existing, err := protoStorage.QueryByFields(ctx, map[string]any{
		"topic_community_id": communityID,
		"community_id":       communityID,
	}, &models.ChatConversation{})
	if err != nil {
		logger.Error("failed to query existing community conversation", "error", err)
		return "", connecterr.Internal(ctx, "CreateCommunityConversation", err)
	}
	if len(existing) > 0 {
		conv := existing[0].(*models.ChatConversation)
		logger.Debug("returning existing community conversation", "conversation_id", conv.Id)
		return conv.Id, nil
	}

	now := time.Now().Unix()
	conversation := &models.ChatConversation{
		CommunityId:          communityID,
		Topic:                topic,
		ParticipantIds:       []string{},
		CreatedAtUnixSec:     now,
		LastMessageAtUnixSec: 0,
	}

	conversationID, err = chatConvStorage.Insert(ctx, conversation)
	if err != nil {
		logger.Error("failed to create community conversation", "error", err)
		return "", connecterr.Internal(ctx, "CreateCommunityConversation", err)
	}

	logger.Info("created community conversation", "conversation_id", conversationID)
	return conversationID, nil
}

// CreateOrGetConversation creates a new conversation or returns an existing one for the given topic.
// This is a library function that can be used by any service to create conversations.
//
// For transfer topics, it automatically adds the owner and recipient as participants.
// For request topics, participants must be added separately by the caller.
//
// Returns the conversation ID and participant IDs.
func CreateOrGetConversation(
	ctx context.Context,
	storage *storage.ProtoSQLStorage,
	chatConvStorage *storage.ChatConversationStorage,
	communityID string,
	topic *models.ConversationTopic,
) (conversationID string, participantIDs []string, err error) {
	queryField, topicID, ok := TopicQueryField(topic)
	if !ok {
		return "", nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("one of transfer_id, request_id, experience_id, or gear_id must be provided"))
	}

	logger := logging.LoggerWithContext(ctx)

	var participantIDsForNewConv []string

	switch topic.GetTopicId().(type) {
	case *models.ConversationTopic_TransferId:
		// Fetch the transfer to get participants.
		transfer := &models.Transfer{}
		if err := storage.GetByID(ctx, topicID, transfer); err != nil {
			logger.Error("failed to get transfer", "transfer_id", topicID, "error", err)
			return "", nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("transfer not found"))
		}
		participantIDsForNewConv = []string{transfer.OwnerId, transfer.RecipientId}

	case *models.ConversationTopic_RequestId:
		request := &models.Request{}
		if err := storage.GetByID(ctx, topicID, request); err != nil {
			logger.Error("failed to get request", "request_id", topicID, "error", err)
			return "", nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found"))
		}
		participantIDsForNewConv = []string{}

	case *models.ConversationTopic_ExperienceId:
		experience := &models.Experience{}
		if err := storage.GetByID(ctx, topicID, experience); err != nil {
			logger.Error("failed to get experience", "experience_id", topicID, "error", err)
			return "", nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("experience not found"))
		}
		participantIDsForNewConv = []string{}

	case *models.ConversationTopic_GearId:
		gear := &models.Gear{}
		if err := storage.GetByID(ctx, topicID, gear); err != nil {
			logger.Error("failed to get gear", "gear_id", topicID, "error", err)
			return "", nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
		}
		participantIDsForNewConv = []string{}

	case *models.ConversationTopic_CommunityId:
		// Community conversations are created via CreateCommunityConversation.
		// Participants are resolved dynamically from community membership.
		participantIDsForNewConv = []string{}
	}

	// Check if a conversation already exists for this topic in this community.
	queryFields := map[string]any{
		queryField:     topicID,
		"community_id": communityID,
	}
	existing, err := storage.QueryByFields(ctx, queryFields, &models.ChatConversation{})
	if err != nil {
		logger.Error("failed to query existing conversations", "topic_type", queryField, "topic_id", topicID, "community_id", communityID, "error", err)
		return "", nil, connecterr.Internal(ctx, "CreateOrGetConversation", err)
	}

	if len(existing) > 0 {
		conversation := existing[0].(*models.ChatConversation)
		logger.Info("returning existing conversation", "conversation_id", conversation.Id, "topic_type", queryField, "topic_id", topicID, "community_id", communityID)
		return conversation.Id, conversation.ParticipantIds, nil
	}

	// Create new conversation with the topic.
	now := time.Now().Unix()
	conversation := &models.ChatConversation{
		CommunityId:          communityID,
		Topic:                topic,
		ParticipantIds:       participantIDsForNewConv,
		CreatedAtUnixSec:     now,
		LastMessageAtUnixSec: 0,
	}

	conversationID, err = chatConvStorage.Insert(ctx, conversation)
	if err != nil {
		logger.Error("failed to create conversation", "topic_type", queryField, "topic_id", topicID, "error", err)
		return "", nil, connecterr.Internal(ctx, "CreateOrGetConversation", err)
	}

	logger.Info("created conversation",
		"conversation_id", conversationID,
		"topic_type", queryField,
		"topic_id", topicID,
		"participant_count", len(participantIDsForNewConv))

	return conversationID, participantIDsForNewConv, nil
}

// AddParticipantToConversation adds a participant to an existing conversation if not already present.
// Returns nil on success.
func AddParticipantToConversation(
	ctx context.Context,
	chatConvStorage *storage.ChatConversationStorage,
	conversationID string,
	participantID string,
) error {
	logger := logging.LoggerWithContext(ctx).With(
		"conversation_id", conversationID,
		"participant_id", participantID,
	)

	conversation, err := chatConvStorage.GetByID(ctx, conversationID)
	if err != nil {
		logger.Error("failed to get conversation", "error", err)
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("conversation not found"))
	}

	// Check if already a participant
	for _, pid := range conversation.ParticipantIds {
		if pid == participantID {
			return nil // Already in conversation
		}
	}

	// Add participant to conversation
	conversation.ParticipantIds = append(conversation.ParticipantIds, participantID)
	if err = chatConvStorage.Update(ctx, conversation); err != nil {
		logger.Error("failed to add participant to conversation", "error", err)
		return connecterr.Internal(ctx, "AddParticipantToConversation", err)
	}

	logger.Info("added participant to conversation")
	return nil
}

// RemoveParticipantFromConversation removes a participant from an existing conversation.
// Returns nil on success. Returns an error if participant is not in the conversation.
func RemoveParticipantFromConversation(
	ctx context.Context,
	chatConvStorage *storage.ChatConversationStorage,
	conversationID string,
	participantID string,
) error {
	logger := logging.LoggerWithContext(ctx).With(
		"conversation_id", conversationID,
		"participant_id", participantID,
	)

	conversation, err := chatConvStorage.GetByID(ctx, conversationID)
	if err != nil {
		logger.Error("failed to get conversation", "error", err)
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("conversation not found"))
	}

	// Find and remove participant
	found := false
	newParticipants := make([]string, 0, len(conversation.ParticipantIds))
	for _, pid := range conversation.ParticipantIds {
		if pid == participantID {
			found = true
			continue
		}
		newParticipants = append(newParticipants, pid)
	}

	if !found {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("participant not in conversation"))
	}

	conversation.ParticipantIds = newParticipants
	if err = chatConvStorage.Update(ctx, conversation); err != nil {
		logger.Error("failed to remove participant from conversation", "error", err)
		return connecterr.Internal(ctx, "RemoveParticipantFromConversation", err)
	}

	logger.Info("removed participant from conversation")
	return nil
}

// GetConversationParticipantsWhoPosted returns a list of user IDs who have posted messages in the conversation.
// If postedOnly is true, only users who have sent messages are returned.
// If postedOnly is false, all conversation participants are returned.
// Returns user IDs in chronological order (first to post appears first).
func GetConversationParticipantsWhoPosted(
	ctx context.Context,
	storage *storage.ProtoSQLStorage,
	conversationID string,
	postedOnly bool,
) ([]string, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetConversationParticipantsWhoPosted",
		"conversation_id", conversationID,
		"posted_only", postedOnly,
	)

	// Get conversation to verify it exists
	conversation := &models.ChatConversation{}
	err := storage.GetByID(ctx, conversationID, conversation)
	if err != nil {
		logger.Error("failed to get conversation", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("conversation not found"))
	}

	// If not filtering by posted, just return all participants
	if !postedOnly {
		return conversation.ParticipantIds, nil
	}

	// Query messages to find unique senders
	queryFields := map[string]any{
		"conversation_id": conversationID,
	}
	messages, err := storage.QueryByFields(ctx, queryFields, &models.ChatMessage{})
	if err != nil {
		logger.Error("failed to query messages", "error", err)
		return nil, connecterr.Internal(ctx, "GetConversationParticipantsWhoPosted", err)
	}

	// Build ordered list of users who posted (deduplicated, chronological)
	seen := make(map[string]bool)
	var postedUserIDs []string

	for _, msg := range messages {
		chatMsg := msg.(*models.ChatMessage)

		// Skip system messages (only process user messages)
		userMsg := chatMsg.GetUserMessage()
		if userMsg == nil {
			continue
		}

		// Add sender if not already seen
		senderID := userMsg.SenderId
		if !seen[senderID] {
			seen[senderID] = true
			postedUserIDs = append(postedUserIDs, senderID)
		}
	}

	logger.DebugContext(ctx, "found conversation participants who posted",
		"total_participants", len(conversation.ParticipantIds),
		"posted_count", len(postedUserIDs))

	return postedUserIDs, nil
}

// GetConversationMessageCount returns the number of messages in the conversation.
func GetConversationMessageCount(
	ctx context.Context,
	storage *storage.ProtoSQLStorage,
	conversationID string,
) (int32, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "GetConversationMessageCount",
		"conversation_id", conversationID,
	)

	// Query all messages for this conversation
	queryFields := map[string]any{
		"conversation_id": conversationID,
	}
	messages, err := storage.QueryByFields(ctx, queryFields, &models.ChatMessage{})
	if err != nil {
		logger.Error("failed to query messages", "error", err)
		return 0, connecterr.Internal(ctx, "GetConversationMessageCount", err)
	}

	var count int32
	for _, m := range messages {
		if !IsSystemMessage(m.(*models.ChatMessage)) {
			count++
		}
	}
	logger.DebugContext(ctx, "counted conversation messages", "count", count)

	return count, nil
}
