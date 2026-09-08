package chat

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/connecterr"
	convstate "go.ripls.org/ripls/server/conversation"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

const maxLastMessageTextLength = 150

// validateAndCleanParticipants performs defensive validation of conversation participants.
// Removes any participants who are no longer members of the conversation's community.
// This handles edge cases where a user was removed from a community after joining a conversation.
//
// Note: Experience conversations are skipped — participants are added explicitly via RSVP
// and may come from multiple communities, so single-community membership validation would
// incorrectly evict valid participants.
func validateAndCleanParticipants(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	chatConvStore *storage.ChatConversationStorage,
	conversation *models.ChatConversation,
) error {
	logger := logging.LoggerWithContext(ctx).With(
		"conversation_id", conversation.Id,
		"community_id", conversation.CommunityId,
	)

	if len(conversation.ParticipantIds) == 0 {
		return nil // Nothing to validate
	}

	// Gear, experience, and request conversations can span multiple communities —
	// participants are added from any community the item is shared with. Community
	// conversations resolve participants dynamically from community membership on
	// every access. Skip membership validation for all four; otherwise we'd evict
	// legitimate participants who joined via a secondary sharing community.
	if topic := conversation.GetTopic(); topic != nil {
		if topic.GetGearId() != "" || topic.GetExperienceId() != "" || topic.GetRequestId() != "" || topic.GetCommunityId() != "" {
			return nil
		}
	}

	// Query all current community members
	communityUsers, err := store.QueryByField(ctx, "community_id", conversation.CommunityId, &models.CommunityUser{})
	if err != nil {
		logger.Warn("failed to query community members for participant validation", "error", err)
		return nil // Don't fail - this is defensive only
	}

	// Build set of valid community members
	validMembers := make(map[string]bool)
	for _, msg := range communityUsers {
		cu := msg.(*models.CommunityUser)
		validMembers[cu.UserId] = true
	}

	// Filter participants to only include current community members
	validParticipants := make([]string, 0, len(conversation.ParticipantIds))
	removedCount := 0
	for _, participantID := range conversation.ParticipantIds {
		if validMembers[participantID] {
			validParticipants = append(validParticipants, participantID)
		} else {
			removedCount++
			logger.Warn("removing participant who is no longer a community member",
				"participant_id", participantID)
		}
	}

	// Update conversation if any participants were removed
	if removedCount > 0 {
		conversation.ParticipantIds = validParticipants
		if err := chatConvStore.Update(ctx, conversation); err != nil {
			logger.Warn("failed to update conversation after participant cleanup", "error", err)
			return nil // Don't fail - this is defensive only
		}
		logger.Info("cleaned up conversation participants",
			"removed_count", removedCount,
			"remaining_count", len(validParticipants))
	}

	return nil
}

// addUserAsParticipant adds a user to a conversation's participant list if they are a member
// of the conversation's community (or any community the experience is shared with, for
// experience conversations that span multiple communities).
// Returns error if the user is not a member of any relevant community.
func addUserAsParticipant(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	chatConvStore *storage.ChatConversationStorage,
	conversation *models.ChatConversation,
	userID string,
) error {
	logger := logging.LoggerWithContext(ctx).With(
		"conversation_id", conversation.Id,
		"user_id", userID,
		"community_id", conversation.CommunityId,
	)

	// Gear, experience, and request conversations can each span multiple
	// communities (via CommunityGear / CommunityExperience / CommunityRequest).
	// Grant access if the caller is a member of ANY active community the topic
	// is shared with — not just conversation.CommunityId, which is only the
	// first community of the share.
	sharedCommunityIDs, err := collectSharedCommunityIDs(ctx, store, conversation)
	if err != nil {
		logger.Error("failed to collect shared community ids", "error", err)
		return err
	}
	if len(sharedCommunityIDs) == 0 {
		return connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("conversation has no associated community"))
	}

	// Single batched JOIN: fetch all candidate communities and the caller's
	// CommunityUser row for each. Grant access when the caller is a member
	// of ANY active (non-deleted) shared community.
	pairs, err := store.GetCommunitiesWithMembership(ctx, sharedCommunityIDs, userID)
	if err != nil {
		logger.Error("failed to batch-fetch shared communities", "error", err)
		return connecterr.Internal(ctx, "addUserAsParticipant", err, "shared_community_count", len(sharedCommunityIDs))
	}
	for _, communityID := range sharedCommunityIDs {
		pair, ok := pairs[communityID]
		if !ok {
			continue // community hard-deleted or never existed
		}
		if pair.Community.Deleted != nil && pair.Community.Deleted.DeletedAtUnixSec > 0 {
			continue // community soft-deleted
		}
		if pair.Membership == nil {
			continue // user never joined this community
		}
		if pair.Membership.Deleted != nil && pair.Membership.Deleted.DeletedAtUnixSec > 0 {
			continue // user left this community
		}
		// User is an active member of a shared community. Add them as a
		// participant only when not already present (idempotent on re-access
		// after a leave/rejoin cycle strips and re-adds them).
		for _, p := range conversation.ParticipantIds {
			if p == userID {
				return nil // already a participant; membership re-verified above
			}
		}
		conversation.ParticipantIds = append(conversation.ParticipantIds, userID)
		if err := chatConvStore.Update(ctx, conversation); err != nil {
			logger.Error("failed to add participant to conversation", "error", err)
			return fmt.Errorf("failed to add participant: %w", err)
		}
		logger.Info("added community member as conversation participant")
		return nil
	}
	return connect.NewError(connect.CodePermissionDenied,
		fmt.Errorf("user is not a member of any community this conversation is shared with"))
}

// StartConversation creates or retrieves a conversation about a topic.
// Verifies the user has permission to create a conversation about the topic.
func (s *Service) StartConversation(
	ctx context.Context,
	req *connect.Request[api.StartConversationRequest],
) (*connect.Response[api.StartConversationResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"community_id", req.Msg.CommunityId,
	)

	// Validate community_id is provided
	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("community_id is required"))
	}

	// Verify community is active and caller is a member (single round-trip).
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Extract topic and validate authorization
	apiTopic := req.Msg.GetTopic()
	if apiTopic == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("topic is required"))
	}

	switch topic := apiTopic.GetTopicId().(type) {
	case *api.ConversationTopic_TransferId:
		logger = logger.With("transfer_id", topic.TransferId)

		// Fetch the transfer to verify user has access
		transfer := &models.Transfer{}
		if err := s.storage.GetByID(ctx, topic.TransferId, transfer); err != nil {
			logger.Error("failed to get transfer", "error", err)
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("transfer not found"))
		}

		// Verify user is either owner or recipient
		if transfer.OwnerId != authInfo.UserID && transfer.RecipientId != authInfo.UserID {
			return nil, connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("user is not a participant in this transfer"))
		}

	case *api.ConversationTopic_RequestId:
		logger = logger.With("target_request_id", topic.RequestId)

		// Fetch the request to verify it exists
		request := &models.Request{}
		if err := s.storage.GetByID(ctx, topic.RequestId, request); err != nil {
			logger.Error("failed to get request", "error", err)
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("request not found"))
		}

		// No additional authorization needed for requests - any community member can view

	case *api.ConversationTopic_GearId:
		logger = logger.With("gear_id", topic.GearId, "topic_type", "gear")

		if err := s.authorizeAndPrepareGearConversation(ctx, logger, authInfo.UserID, topic.GearId, req.Msg.CommunityId); err != nil {
			return nil, err
		}

	default:
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("topic is required"))
	}

	modelsTopic := chat.APITopicToModels(apiTopic)
	conversationID, participantIDs, err := chat.CreateOrGetConversation(ctx, s.storage, s.chatConvStorage, req.Msg.CommunityId, modelsTopic)
	if err != nil {
		return nil, err
	}

	// For gear topics: persist conversation_id back onto the Gear row if it was
	// missing (recovery path for gear that missed the ShareGear-time creation),
	// and add the owner as a participant.
	if gearTopic, ok := apiTopic.GetTopicId().(*api.ConversationTopic_GearId); ok {
		if err := s.finalizeGearConversation(ctx, logger, req.Msg.CommunityId, gearTopic.GearId, conversationID); err != nil {
			return nil, err
		}
	}

	// Convert participant IDs to User objects
	participants, err := services.FetchAPIUsers(ctx, s.storage, participantIDs)
	if err != nil {
		return nil, connecterr.Internal(ctx, "StartConversation", err, "detail", "failed to fetch participants")
	}

	return connect.NewResponse(&api.StartConversationResponse{
		ConversationId: conversationID,
		Participants:   participants,
	}), nil
}

// GetConversation retrieves a specific conversation by ID.
func (s *Service) GetConversation(
	ctx context.Context,
	req *connect.Request[api.GetConversationRequest],
) (*connect.Response[api.GetConversationResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"conversation_id", req.Msg.ConversationId,
	)

	// Fetch the conversation
	conversation := &models.ChatConversation{}
	if err := s.storage.GetByID(ctx, req.Msg.ConversationId, conversation); err != nil {
		logger.Error("failed to get conversation", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("conversation not found"))
	}

	// Defensive validation: clean up participants who are no longer community members
	// This handles edge cases where users were removed from communities
	if err := validateAndCleanParticipants(ctx, s.storage, s.chatConvStorage, conversation); err != nil {
		// Log but don't fail - this is defensive only
		logger.Warn("participant validation encountered error", "error", err)
	}

	// For experience conversations, automatically add community members as participants when they access
	isParticipant := false
	for _, participantID := range conversation.ParticipantIds {
		if participantID == authInfo.UserID {
			isParticipant = true
			break
		}
	}

	if !isParticipant {
		// Check if this is a community-accessible conversation (experience, request, gear, or community-wide)
		topic := conversation.GetTopic()
		if topic.GetCommunityId() != "" {
			// Community conversations are open to all community members; membership check
			// replaces the participant list (participants are dynamic).
			isMember, err := auth.IsMemberOfCommunity(ctx, s.storage, conversation.CommunityId, authInfo.UserID)
			if err != nil {
				logger.Error("failed to check community membership for community conversation", "error", err)
				return nil, connecterr.Internal(ctx, "GetConversation", err)
			}
			if !isMember {
				return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "community_not_member", "you're not a member of this community", nil)
			}
			isParticipant = true
		} else if topic.GetExperienceId() != "" || topic.GetRequestId() != "" || topic.GetGearId() != "" {
			// Try to add user as participant if they're a community member
			if err := addUserAsParticipant(ctx, s.storage, s.chatConvStorage, conversation, authInfo.UserID); err != nil {
				// If not a community member, return the permission denied error
				return nil, err
			}
			isParticipant = true
		}

		// If still not a participant, return permission denied
		if !isParticipant {
			return nil, connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("user is not a participant in this conversation"))
		}
	}

	// Count unread messages
	messages, err := s.storage.QueryByField(ctx, "conversation_id", conversation.Id, &models.ChatMessage{})
	if err != nil {
		logger.Error("failed to query messages", "error", err)
		return nil, connecterr.Internal(ctx, "GetConversation", err)
	}

	unreadCount := chat.CountUnreadUserMessages(messages, authInfo.UserID)

	// Build ConversationItem
	item := &api.ConversationItem{
		ConversationId:       conversation.Id,
		CommunityId:          conversation.CommunityId,
		LastMessageAtUnixSec: conversation.LastMessageAtUnixSec,
		UnreadCount:          unreadCount,
	}

	// Convert participant IDs to User objects
	participants, err := services.FetchAPIUsers(ctx, s.storage, conversation.ParticipantIds)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetConversation", err, "detail", "failed to fetch participants")
	}
	item.Participants = participants

	// Set topic
	item.Topic = chat.ModelsTopicToAPI(conversation.GetTopic())

	// Populate last message preview
	if err := s.populateLastMessage(ctx, item); err != nil {
		logger.Error("failed to populate last message", "error", err)
		// Continue anyway - not critical to fail the whole request
	}

	return connect.NewResponse(&api.GetConversationResponse{
		Conversation: item,
	}), nil
}

// ListConversations lists all conversations for the current user.
func (s *Service) ListConversations(
	ctx context.Context,
	req *connect.Request[api.ListConversationsRequest],
) (*connect.Response[api.ListConversationsResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
	)

	// Fetch only the conversations where the authenticated user is a participant,
	// using the indexed participant_ids column for an O(log N) lookup.
	rawConversations, err := s.chatConvStorage.ListByParticipant(ctx, authInfo.UserID, 0)
	if err != nil {
		logger.Error("failed to list conversations", "error", err)
		return nil, connecterr.Internal(ctx, "ListConversations", err)
	}

	// Collect all participant IDs from the user's conversations.
	type conversationEntry struct {
		conversation *models.ChatConversation
	}
	var userConversations []conversationEntry
	allParticipantIDs := make([]string, 0)

	for _, conversation := range rawConversations {
		userConversations = append(userConversations, conversationEntry{conversation: conversation})
		allParticipantIDs = append(allParticipantIDs, conversation.ParticipantIds...)
	}

	// Batch fetch all participant users upfront
	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, allParticipantIDs)
	if err != nil {
		return nil, connecterr.Internal(ctx, "ListConversations", err, "detail", "failed to batch fetch participants")
	}

	// Collect all conversation IDs for batch queries.

	convIDs := make([]string, 0, len(userConversations))
	for _, entry := range userConversations {
		convIDs = append(convIDs, entry.conversation.Id)
	}

	// Batch fetch all messages across all user conversations in one query.
	allMessages, err := s.storage.QueryByFieldIn(ctx, "conversation_id", convIDs, &models.ChatMessage{})
	if err != nil {
		return nil, connecterr.Internal(ctx, "ListConversations", err, "detail", "failed to batch fetch messages")
	}
	messagesByConvID := make(map[string][]proto.Message, len(convIDs))
	for _, msg := range allMessages {
		chatMsg := msg.(*models.ChatMessage)
		messagesByConvID[chatMsg.ConversationId] = append(messagesByConvID[chatMsg.ConversationId], msg)
	}

	// Collect transfer and request IDs so we can batch-fetch topic items.
	var transferIDs, requestIDs []string
	for _, entry := range userConversations {
		switch topic := entry.conversation.GetTopic().GetTopicId().(type) {
		case *models.ConversationTopic_TransferId:
			transferIDs = append(transferIDs, topic.TransferId)
		case *models.ConversationTopic_RequestId:
			requestIDs = append(requestIDs, topic.RequestId)
		}
	}

	// Batch fetch transfers and requests for terminal-state checks.
	transferMap, err := s.storage.GetByIDs(ctx, transferIDs, &models.Transfer{})
	if err != nil {
		return nil, connecterr.Internal(ctx, "ListConversations", err, "detail", "failed to batch fetch transfers")
	}
	requestMap, err := s.storage.GetByIDs(ctx, requestIDs, &models.Request{})
	if err != nil {
		return nil, connecterr.Internal(ctx, "ListConversations", err, "detail", "failed to batch fetch requests")
	}

	// Second pass: build conversation items using pre-fetched data.

	var conversationItems []*api.ConversationItem
	for _, entry := range userConversations {
		conversation := entry.conversation
		messages := messagesByConvID[conversation.Id]

		// Skip conversations with no messages - they shouldn't appear in the inbox
		// until someone has sent at least one message
		if len(messages) == 0 {
			continue
		}

		unreadCount := chat.CountUnreadUserMessages(messages, authInfo.UserID)

		// Check if the associated item is in a terminal state using pre-fetched maps.
		itemDone := convstate.IsItemDoneFromMaps(conversation, transferMap, requestMap)

		// Determine if this conversation is archived for the user:
		// A conversation is archived when the item is done AND there are no unread messages
		isArchived := itemDone && unreadCount == 0

		// Filter based on the archived parameter
		if req.Msg.Archived != isArchived {
			continue
		}

		// Build conversation item
		item := &api.ConversationItem{
			ConversationId:       conversation.Id,
			CommunityId:          conversation.CommunityId,
			LastMessageAtUnixSec: conversation.LastMessageAtUnixSec,
			UnreadCount:          unreadCount,
			IsItemDone:           itemDone,
		}

		// Convert participant IDs to User objects from pre-fetched map
		participants := make([]*api.User, 0, len(conversation.ParticipantIds))
		for _, pid := range conversation.ParticipantIds {
			if u := userMap[pid]; u != nil {
				participants = append(participants, u)
			}
		}
		item.Participants = participants

		// Set topic
		item.Topic = chat.ModelsTopicToAPI(conversation.GetTopic())

		// Populate last message preview using pre-fetched messages and users.
		if err := populateLastMessageFromMessages(item, messages, userMap); err != nil {
			logger.Error("failed to populate last message", "conversation_id", conversation.Id, "error", err)
			// Continue anyway - not critical to fail the whole request
		}

		conversationItems = append(conversationItems, item)
	}

	// Sort by last message timestamp (descending)
	for i := 0; i < len(conversationItems); i++ {
		for j := i + 1; j < len(conversationItems); j++ {
			if conversationItems[i].LastMessageAtUnixSec < conversationItems[j].LastMessageAtUnixSec {
				conversationItems[i], conversationItems[j] = conversationItems[j], conversationItems[i]
			}
		}
	}

	return connect.NewResponse(&api.ListConversationsResponse{
		Conversations: conversationItems,
	}), nil
}

// GetConversationForTransfer retrieves the conversation associated with a transfer.
func (s *Service) GetConversationForTransfer(
	ctx context.Context,
	req *connect.Request[api.GetConversationForTransferRequest],
) (*connect.Response[api.GetConversationForTransferResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"transfer_id", req.Msg.TransferId,
	)

	// Both loans and giveaways use gear conversations (not transfer-specific conversations)
	// Look up the transfer to get its gear_id and community_id
	transfer := &models.Transfer{}
	if err := s.storage.GetByID(ctx, req.Msg.TransferId, transfer); err != nil {
		logger.Error("failed to get transfer", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("transfer not found"))
	}

	// Verify user is either owner or recipient
	if transfer.OwnerId != authInfo.UserID && transfer.RecipientId != authInfo.UserID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("user is not a participant in this transfer"))
	}

	// Get the gear to find its conversation_id (canonical location).
	gear := &models.Gear{}
	if err := s.storage.GetByID(ctx, transfer.GearId, gear); err != nil {
		logger.Error("failed to get gear", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("gear not found"))
	}

	conversationID := gear.ConversationId

	if conversationID == "" {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no conversation found for gear"))
	}

	// Get the conversation
	conversation := &models.ChatConversation{}
	if err := s.storage.GetByID(ctx, conversationID, conversation); err != nil {
		logger.Error("failed to get conversation", "error", err)
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("conversation not found"))
	}

	// Verify user is a participant
	if err := s.requireParticipant(ctx, conversation.Id, authInfo.UserID); err != nil {
		return nil, err
	}

	// Count unread messages
	messages, err := s.storage.QueryByField(ctx, "conversation_id", conversation.Id, &models.ChatMessage{})
	if err != nil {
		logger.Error("failed to query messages", "conversation_id", conversation.Id, "error", err)
		return nil, connecterr.Internal(ctx, "GetConversationForTransfer", err)
	}

	unreadCount := chat.CountUnreadUserMessages(messages, authInfo.UserID)

	// Build ConversationItem
	item := &api.ConversationItem{
		ConversationId:       conversation.Id,
		CommunityId:          conversation.CommunityId,
		LastMessageAtUnixSec: conversation.LastMessageAtUnixSec,
		UnreadCount:          unreadCount,
	}

	// Convert participant IDs to User objects
	participants, err := services.FetchAPIUsers(ctx, s.storage, conversation.ParticipantIds)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetConversationForTransfer", err, "detail", "failed to fetch participants")
	}
	item.Participants = participants

	// Set topic
	item.Topic = chat.ModelsTopicToAPI(conversation.GetTopic())

	// Populate last message preview
	if err := s.populateLastMessage(ctx, item); err != nil {
		logger.Error("failed to populate last message", "conversation_id", conversation.Id, "error", err)
		// Continue anyway - not critical to fail the whole request
	}

	return connect.NewResponse(&api.GetConversationForTransferResponse{
		Conversation: item,
	}), nil
}

// GetConversationForRequest retrieves the conversation associated with a request.
func (s *Service) GetConversationForRequest(
	ctx context.Context,
	req *connect.Request[api.GetConversationForRequestRequest],
) (*connect.Response[api.GetConversationForRequestResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"target_request_id", req.Msg.RequestId,
	)

	// Query for conversation by request_id
	conversations, err := s.storage.QueryByField(ctx, "topic_request_id", req.Msg.RequestId, &models.ChatConversation{})
	if err != nil {
		logger.Error("failed to query conversations by request_id", "error", err)
		return nil, connecterr.Internal(ctx, "GetConversationForRequest", err)
	}

	if len(conversations) == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no conversation found for request"))
	}

	conversation := conversations[0].(*models.ChatConversation)

	// Verify community is active and caller is a member (not just a participant).
	// Request conversations are open to all community members; rejects access
	// when the parent community is soft-deleted.
	if _, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, conversation.CommunityId, authInfo.UserID); err != nil {
		return nil, err
	}

	// Count unread messages
	messages, err := s.storage.QueryByField(ctx, "conversation_id", conversation.Id, &models.ChatMessage{})
	if err != nil {
		logger.Error("failed to query messages", "conversation_id", conversation.Id, "error", err)
		return nil, connecterr.Internal(ctx, "GetConversationForRequest", err)
	}

	unreadCount := chat.CountUnreadUserMessages(messages, authInfo.UserID)

	// Build ConversationItem
	item := &api.ConversationItem{
		ConversationId:       conversation.Id,
		CommunityId:          conversation.CommunityId,
		LastMessageAtUnixSec: conversation.LastMessageAtUnixSec,
		UnreadCount:          unreadCount,
	}

	// Convert participant IDs to User objects
	participants, err := services.FetchAPIUsers(ctx, s.storage, conversation.ParticipantIds)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetConversationForRequest", err, "detail", "failed to fetch participants")
	}
	item.Participants = participants

	// Set topic
	item.Topic = chat.ModelsTopicToAPI(conversation.GetTopic())

	// Populate last message preview
	if err := s.populateLastMessage(ctx, item); err != nil {
		logger.Error("failed to populate last message", "conversation_id", conversation.Id, "error", err)
		// Continue anyway - not critical to fail the whole request
	}

	return connect.NewResponse(&api.GetConversationForRequestResponse{
		Conversation: item,
	}), nil
}

// GetConversationForExperience retrieves the conversation associated with an experience.
func (s *Service) GetConversationForExperience(
	ctx context.Context,
	req *connect.Request[api.GetConversationForExperienceRequest],
) (*connect.Response[api.GetConversationForExperienceResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"experience_id", req.Msg.ExperienceId,
	)

	// Query for conversation by experience_id
	conversations, err := s.storage.QueryByField(ctx, "topic_experience_id", req.Msg.ExperienceId, &models.ChatConversation{})
	if err != nil {
		logger.Error("failed to query conversations by experience_id", "error", err)
		return nil, connecterr.Internal(ctx, "GetConversationForExperience", err)
	}

	if len(conversations) == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no conversation found for experience"))
	}

	conversation := conversations[0].(*models.ChatConversation)

	// Always re-verify active community membership — stale ParticipantIds entries
	// from prior memberships must not grant access. addUserAsParticipant checks
	// membership and is idempotent: it skips the update when the caller is already
	// a participant but membership was confirmed above.
	if err := addUserAsParticipant(ctx, s.storage, s.chatConvStorage, conversation, authInfo.UserID); err != nil {
		return nil, err
	}

	// Count unread messages
	messages, err := s.storage.QueryByField(ctx, "conversation_id", conversation.Id, &models.ChatMessage{})
	if err != nil {
		logger.Error("failed to query messages", "conversation_id", conversation.Id, "error", err)
		return nil, connecterr.Internal(ctx, "GetConversationForExperience", err)
	}

	unreadCount := chat.CountUnreadUserMessages(messages, authInfo.UserID)

	// Build ConversationItem
	item := &api.ConversationItem{
		ConversationId:       conversation.Id,
		CommunityId:          conversation.CommunityId,
		LastMessageAtUnixSec: conversation.LastMessageAtUnixSec,
		UnreadCount:          unreadCount,
	}

	// Convert participant IDs to User objects
	participants, err := services.FetchAPIUsers(ctx, s.storage, conversation.ParticipantIds)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetConversationForExperience", err, "detail", "failed to fetch participants")
	}
	item.Participants = participants

	// Set topic
	item.Topic = chat.ModelsTopicToAPI(conversation.GetTopic())

	// Populate last message preview
	if err := s.populateLastMessage(ctx, item); err != nil {
		logger.Error("failed to populate last message", "conversation_id", conversation.Id, "error", err)
		// Continue anyway - not critical to fail the whole request
	}

	return connect.NewResponse(&api.GetConversationForExperienceResponse{
		Conversation: item,
	}), nil
}

// GetConversationForCommunity retrieves (or lazily creates) the community-wide conversation.
func (s *Service) GetConversationForCommunity(
	ctx context.Context,
	req *connect.Request[api.GetConversationForCommunityRequest],
) (*connect.Response[api.GetConversationForCommunityResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"community_id", req.Msg.CommunityId,
	)

	if req.Msg.CommunityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("community_id is required"))
	}

	// Verify the caller is a member of the community.
	isMember, err := auth.IsMemberOfCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID)
	if err != nil {
		logger.Error("failed to check community membership", "error", err)
		return nil, connecterr.Internal(ctx, "GetConversationForCommunity", err)
	}
	if !isMember {
		return nil, connecterr.UserVisible(ctx, connect.CodePermissionDenied, "community_not_member", "you're not a member of this community", nil)
	}

	// Lazily create the conversation if it does not yet exist (backfill path).
	conversationID, err := chat.CreateCommunityConversation(ctx, s.storage, s.chatConvStorage, req.Msg.CommunityId)
	if err != nil {
		logger.Error("failed to get or create community conversation", "error", err)
		return nil, connecterr.Internal(ctx, "GetConversationForCommunity", err)
	}

	conversation := &models.ChatConversation{}
	if err := s.storage.GetByID(ctx, conversationID, conversation); err != nil {
		logger.Error("failed to fetch community conversation", "conversation_id", conversationID, "error", err)
		return nil, connecterr.Internal(ctx, "GetConversationForCommunity", err)
	}

	messages, err := s.storage.QueryByField(ctx, "conversation_id", conversationID, &models.ChatMessage{})
	if err != nil {
		logger.Error("failed to query messages", "conversation_id", conversationID, "error", err)
		return nil, connecterr.Internal(ctx, "GetConversationForCommunity", err)
	}

	unreadCount := chat.CountUnreadUserMessages(messages, authInfo.UserID)

	item := &api.ConversationItem{
		ConversationId:       conversation.Id,
		CommunityId:          conversation.CommunityId,
		LastMessageAtUnixSec: conversation.LastMessageAtUnixSec,
		UnreadCount:          unreadCount,
		IsItemDone:           false, // Community conversations are perpetual.
		Topic: &api.ConversationTopic{
			TopicId: &api.ConversationTopic_CommunityId{CommunityId: req.Msg.CommunityId},
		},
	}

	// Participants are computed dynamically from community membership.
	communityMembers, err := s.storage.QueryByField(ctx, "community_id", req.Msg.CommunityId, &models.CommunityUser{})
	if err != nil {
		logger.Error("failed to query community members", "error", err)
		return nil, connecterr.Internal(ctx, "GetConversationForCommunity", err)
	}
	memberIDs := make([]string, 0, len(communityMembers))
	for _, m := range communityMembers {
		memberIDs = append(memberIDs, m.(*models.CommunityUser).UserId)
	}
	participants, err := services.FetchAPIUsers(ctx, s.storage, memberIDs)
	if err != nil {
		return nil, connecterr.Internal(ctx, "GetConversationForCommunity", err, "detail", "failed to fetch participants")
	}
	item.Participants = participants

	if err := s.populateLastMessage(ctx, item); err != nil {
		logger.Warn("failed to populate last message", "conversation_id", conversationID, "error", err)
	}

	return connect.NewResponse(&api.GetConversationForCommunityResponse{
		Conversation: item,
	}), nil
}

// populateLastMessage populates the last_message_text and last_message_sender fields
// for a ConversationItem by fetching messages and the last-message sender.
// For batch callers that already hold pre-fetched messages and users,
// call populateLastMessageFromMessages directly.
func (s *Service) populateLastMessage(ctx context.Context, item *api.ConversationItem) error {
	logger := logging.LoggerWithContext(ctx).With("conversation_id", item.ConversationId)

	messages, err := s.storage.QueryByField(ctx, "conversation_id", item.ConversationId, &models.ChatMessage{})
	if err != nil {
		logger.Error("failed to query messages", "error", err)
		return err
	}

	// Find the most recent non-system message to identify the sender to fetch.
	var lastNonSystem *models.ChatMessage
	for _, m := range messages {
		cm := m.(*models.ChatMessage)
		if !chat.IsSystemMessage(cm) {
			if lastNonSystem == nil || cm.SentAtUnixSec > lastNonSystem.SentAtUnixSec {
				lastNonSystem = cm
			}
		}
	}

	var senderID string
	if lastNonSystem != nil {
		senderID = getSenderID(lastNonSystem)
	}

	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, []string{senderID})
	if err != nil {
		logger.Error("failed to fetch sender user", "sender_id", senderID, "error", err)
		return err
	}

	return populateLastMessageFromMessages(item, messages, userMap)
}

// populateLastMessageFromMessages populates last_message_text and last_message_sender
// using pre-fetched messages and a pre-fetched user map, avoiding additional DB queries.
func populateLastMessageFromMessages(item *api.ConversationItem, messages []proto.Message, userMap map[string]*api.User) error {
	if len(messages) == 0 {
		return nil
	}

	// Find the most recent user message (skip system messages for preview).
	var lastMessage *models.ChatMessage
	for _, msgProto := range messages {
		chatMsg := msgProto.(*models.ChatMessage)
		if chat.IsSystemMessage(chatMsg) {
			continue
		}
		if lastMessage == nil || chatMsg.SentAtUnixSec > lastMessage.SentAtUnixSec {
			lastMessage = chatMsg
		}
	}

	if lastMessage == nil {
		return nil
	}

	// Extract message text from user message. Decode any encoded @-mentions
	// (e.g. @[user:abc:Jane Doe] -> @Jane Doe) so previews are human-readable
	// rather than showing the raw on-wire format.
	var messageText string
	if um := lastMessage.GetUserMessage(); um != nil {
		messageText = chat.DecodeMentions(um.Text)
	}

	// Truncate message text if needed
	if len(messageText) > maxLastMessageTextLength {
		runes := []rune(messageText)
		if len(runes) > maxLastMessageTextLength {
			messageText = string(runes[:maxLastMessageTextLength])
		}
	}
	item.LastMessageText = messageText

	// Resolve the sender from the pre-fetched user map. Deleted senders
	// surface as a former-member placeholder so the inbox preview keeps
	// rendering.
	if senderID := getSenderID(lastMessage); senderID != "" {
		item.LastMessageSender = services.ResolveUserOrFormer(userMap, senderID)
	}

	return nil
}
