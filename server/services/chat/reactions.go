package chat

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/chat_event_bus"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
)

// AddReaction adds an emoji reaction to a message.
//
// One reaction per user per message: if the user already reacted, the previous
// reaction is replaced. After updating the stored message, the change is
// broadcast to all active streams for the conversation.
func (s *Service) AddReaction(
	ctx context.Context,
	req *connect.Request[api.AddReactionRequest],
) (*connect.Response[api.AddReactionResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "AddReaction",
		"user_id", authInfo.UserID,
		"conversation_id", req.Msg.ConversationId,
		"message_id", req.Msg.MessageId,
	)

	if req.Msg.ConversationId == "" || req.Msg.MessageId == "" || req.Msg.Emoji == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("conversation_id, message_id, and emoji are required"))
	}

	conversation, err := s.requireConversationAccess(ctx, req.Msg.ConversationId, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	chatMsg := &models.ChatMessage{}
	if err := s.storage.GetByID(ctx, req.Msg.MessageId, chatMsg); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("message not found"))
	}

	if chatMsg.ConversationId != req.Msg.ConversationId {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("message not found in conversation"))
	}

	// Replace existing reaction from this user, or append new one.
	replaced := false
	for i, r := range chatMsg.Reactions {
		if r.UserId == authInfo.UserID {
			chatMsg.Reactions[i] = &models.MessageReaction{
				UserId:           authInfo.UserID,
				Emoji:            req.Msg.Emoji,
				CreatedAtUnixSec: clock.Now(ctx).Unix(),
			}
			replaced = true
			break
		}
	}
	if !replaced {
		chatMsg.Reactions = append(chatMsg.Reactions, &models.MessageReaction{
			UserId:           authInfo.UserID,
			Emoji:            req.Msg.Emoji,
			CreatedAtUnixSec: clock.Now(ctx).Unix(),
		})
	}

	if err := s.storage.Update(ctx, chatMsg); err != nil {
		return nil, connecterr.Internal(ctx, "AddReaction", err, "detail", "failed to update message")
	}

	apiReactions := convertReactionsToAPI(ctx, s, chatMsg.Reactions)

	// Publish reaction update to the chat bus. The stream subscriber fans out to live streams.
	if s.bus != nil {
		if err := s.bus.Publish(ctx, chat_event_bus.KindReactionUpdate, nil, conversation,
			chat_event_bus.WithReactionUpdate(&api.ReactionUpdate{
				MessageId: req.Msg.MessageId,
				Reactions: apiReactions,
			})); err != nil {
			logger.WarnContext(ctx, "chat bus publish failed for reaction", "error", err)
		}
	}

	logger.InfoContext(ctx, "reaction added", "emoji", req.Msg.Emoji)

	return connect.NewResponse(&api.AddReactionResponse{
		Reactions: apiReactions,
	}), nil
}

// RemoveReaction removes the current user's reaction from a message.
func (s *Service) RemoveReaction(
	ctx context.Context,
	req *connect.Request[api.RemoveReactionRequest],
) (*connect.Response[api.RemoveReactionResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RemoveReaction",
		"user_id", authInfo.UserID,
		"conversation_id", req.Msg.ConversationId,
		"message_id", req.Msg.MessageId,
	)

	if req.Msg.ConversationId == "" || req.Msg.MessageId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("conversation_id and message_id are required"))
	}

	conversation, err := s.requireConversationAccess(ctx, req.Msg.ConversationId, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	chatMsg := &models.ChatMessage{}
	if err := s.storage.GetByID(ctx, req.Msg.MessageId, chatMsg); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("message not found"))
	}

	if chatMsg.ConversationId != req.Msg.ConversationId {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("message not found in conversation"))
	}

	// Remove the user's reaction.
	filtered := chatMsg.Reactions[:0]
	for _, r := range chatMsg.Reactions {
		if r.UserId != authInfo.UserID {
			filtered = append(filtered, r)
		}
	}
	chatMsg.Reactions = filtered

	if err := s.storage.Update(ctx, chatMsg); err != nil {
		return nil, connecterr.Internal(ctx, "RemoveReaction", err, "detail", "failed to update message")
	}

	apiReactions := convertReactionsToAPI(ctx, s, chatMsg.Reactions)

	// Publish reaction update to the chat bus. The stream subscriber fans out to live streams.
	if s.bus != nil {
		if err := s.bus.Publish(ctx, chat_event_bus.KindReactionUpdate, nil, conversation,
			chat_event_bus.WithReactionUpdate(&api.ReactionUpdate{
				MessageId: req.Msg.MessageId,
				Reactions: apiReactions,
			})); err != nil {
			logger.WarnContext(ctx, "chat bus publish failed for reaction", "error", err)
		}
	}

	logger.InfoContext(ctx, "reaction removed")

	return connect.NewResponse(&api.RemoveReactionResponse{
		Reactions: apiReactions,
	}), nil
}

// convertReactionsToAPI converts model reactions to API reactions, resolving user IDs to User protos.
func convertReactionsToAPI(ctx context.Context, s *Service, reactions []*models.MessageReaction) []*api.Reaction {
	if len(reactions) == 0 {
		return nil
	}

	// Collect unique user IDs for batch fetch.
	userIDs := make([]string, 0, len(reactions))
	seen := make(map[string]bool, len(reactions))
	for _, r := range reactions {
		if !seen[r.UserId] {
			userIDs = append(userIDs, r.UserId)
			seen[r.UserId] = true
		}
	}

	userMap, err := services.FetchAPIUsersBatch(ctx, s.storage, userIDs)
	if err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx, "failed to fetch reaction users", "error", err)
		userMap = map[string]*api.User{}
	}

	apiReactions := make([]*api.Reaction, 0, len(reactions))
	for _, r := range reactions {
		apiReactions = append(apiReactions, &api.Reaction{
			Sender: services.ResolveUserOrFormer(userMap, r.UserId),
			Emoji:  r.Emoji,
		})
	}

	return apiReactions
}
