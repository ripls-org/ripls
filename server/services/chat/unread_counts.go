package chat

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/connecterr"
	convstate "go.ripls.org/ripls/server/conversation"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// GetUnreadCounts returns unread message counts aggregated by community and conversation.
// Excludes archived conversations (where the associated item is in a terminal state).
func (s *Service) GetUnreadCounts(
	ctx context.Context,
	req *connect.Request[api.GetUnreadCountsRequest],
) (*connect.Response[api.GetUnreadCountsResponse], error) {
	logger := logging.LoggerWithContext(ctx).With("operation", "GetUnreadCounts")

	// Get authenticated user
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger = logger.With("user_id", authInfo.UserID)

	// Fetch only the conversations where the authenticated user is a participant,
	// using the indexed participant_ids column for an O(log N) lookup.
	// Soft-deleted conversations are excluded by ListByParticipant.
	userConversations, err := s.chatConvStorage.ListByParticipant(ctx, authInfo.UserID, 0)
	if err != nil {
		logger.ErrorContext(ctx, "failed to list conversations", "error", err)
		return nil, connecterr.Internal(ctx, "GetUnreadCounts", err, "detail", "failed to list conversations")
	}

	// Collect conversation IDs and topic item IDs for batch queries.
	convIDs := make([]string, 0, len(userConversations))
	var transferIDs, requestIDs []string
	for _, conv := range userConversations {
		convIDs = append(convIDs, conv.Id)
		switch topic := conv.GetTopic().GetTopicId().(type) {
		case *models.ConversationTopic_TransferId:
			transferIDs = append(transferIDs, topic.TransferId)
		case *models.ConversationTopic_RequestId:
			requestIDs = append(requestIDs, topic.RequestId)
		}
	}

	// Batch fetch all messages and topic items before iterating.
	allMessages, err := s.storage.QueryByFieldIn(ctx, "conversation_id", convIDs, &models.ChatMessage{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to batch fetch messages", "error", err)
		return nil, connecterr.Internal(ctx, "GetUnreadCounts", err, "detail", "failed to batch fetch messages")
	}
	messagesByConvID := make(map[string][]proto.Message, len(convIDs))
	for _, msg := range allMessages {
		chatMsg := msg.(*models.ChatMessage)
		messagesByConvID[chatMsg.ConversationId] = append(messagesByConvID[chatMsg.ConversationId], msg)
	}

	transferMap, err := s.storage.GetByIDs(ctx, transferIDs, &models.Transfer{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to batch fetch transfers", "error", err)
		return nil, connecterr.Internal(ctx, "GetUnreadCounts", err, "detail", "failed to batch fetch transfers")
	}
	requestMap, err := s.storage.GetByIDs(ctx, requestIDs, &models.Request{})
	if err != nil {
		logger.ErrorContext(ctx, "failed to batch fetch requests", "error", err)
		return nil, connecterr.Internal(ctx, "GetUnreadCounts", err, "detail", "failed to batch fetch requests")
	}

	totalCount := int32(0)
	communityIDToUnreadCount := make(map[string]int32)
	conversationIDToUnreadCount := make(map[string]int32)

	for _, conversation := range userConversations {
		// Skip archived conversations (item in terminal state).
		if convstate.IsItemDoneFromMaps(conversation, transferMap, requestMap) {
			continue
		}

		messages := messagesByConvID[conversation.Id]

		// Skip conversations with no messages
		if len(messages) == 0 {
			continue
		}

		// Count unread user messages (excludes system messages).
		unreadCount := chat.CountUnreadUserMessages(messages, authInfo.UserID)

		// Only include conversations with unread messages
		if unreadCount > 0 {
			conversationIDToUnreadCount[conversation.Id] = unreadCount
			communityIDToUnreadCount[conversation.CommunityId] += unreadCount
			totalCount += unreadCount
		}
	}

	logger.InfoContext(ctx, "unread counts retrieved",
		"total_count", totalCount,
		"community_count", len(communityIDToUnreadCount),
		"conversation_count", len(conversationIDToUnreadCount),
	)

	return connect.NewResponse(&api.GetUnreadCountsResponse{
		TotalUnreadCount:            totalCount,
		CommunityIdToUnreadCount:    communityIDToUnreadCount,
		ConversationIdToUnreadCount: conversationIDToUnreadCount,
	}), nil
}
