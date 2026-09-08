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
)

// quotedTextMaxLen bounds the denormalized reply snippet so a quoted block
// never carries an entire long message.
const quotedTextMaxLen = 150

// EditMessage updates the text of a message the caller authored. Only the
// author may edit, only user (not system) messages are editable, and a deleted
// message cannot be edited. The new text is broadcast so live streams replace
// it in place.
func (s *Service) EditMessage(
	ctx context.Context,
	req *connect.Request[api.EditMessageRequest],
) (*connect.Response[api.EditMessageResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "EditMessage",
		"user_id", authInfo.UserID,
		"conversation_id", req.Msg.ConversationId,
		"message_id", req.Msg.MessageId,
	)

	if req.Msg.ConversationId == "" || req.Msg.MessageId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("conversation_id and message_id are required"))
	}
	if req.Msg.Text == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("text is required"))
	}

	conversation, err := s.requireConversationAccess(ctx, req.Msg.ConversationId, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	chatMsg, err := s.loadAuthoredMessage(ctx, req.Msg.ConversationId, req.Msg.MessageId, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	now := clock.UnixSec(ctx)
	userMsg := chatMsg.GetUserMessage()
	userMsg.Text = req.Msg.Text
	userMsg.EditedAtUnixSec = &now

	if err := s.storage.Update(ctx, chatMsg); err != nil {
		return nil, connecterr.Internal(ctx, "EditMessage", err, "detail", "failed to update message")
	}

	// Broadcast the edit so live streams replace the text in place. Thread the
	// originating ctx so fan-out log lines carry this RPC's request_id.
	if s.bus != nil {
		if err := s.bus.Publish(ctx, chat_event_bus.KindUserMessageUpdate, chatMsg, conversation); err != nil {
			logger.WarnContext(ctx, "chat bus publish failed for edit", "error", err)
		}
	}

	logger.InfoContext(ctx, "message edited")

	return connect.NewResponse(&api.EditMessageResponse{
		EditedAtUnixSec: now,
	}), nil
}

// DeleteMessage soft-deletes a message the caller authored. Storage read paths
// filter soft-deleted messages out of all responses; the deletion is broadcast
// so live streams remove it in place.
func (s *Service) DeleteMessage(
	ctx context.Context,
	req *connect.Request[api.DeleteMessageRequest],
) (*connect.Response[api.DeleteMessageResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "DeleteMessage",
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

	chatMsg, err := s.loadAuthoredMessage(ctx, req.Msg.ConversationId, req.Msg.MessageId, authInfo.UserID)
	if err != nil {
		return nil, err
	}

	chatMsg.Deleted = &models.DeletedMetadata{
		DeletedByUserId:  authInfo.UserID,
		DeletedAtUnixSec: clock.UnixSec(ctx),
	}

	if err := s.storage.Update(ctx, chatMsg); err != nil {
		return nil, connecterr.Internal(ctx, "DeleteMessage", err, "detail", "failed to delete message")
	}

	// Broadcast the deletion so live streams remove it in place. Thread the
	// originating ctx so fan-out log lines carry this RPC's request_id.
	if s.bus != nil {
		if err := s.bus.Publish(ctx, chat_event_bus.KindMessageDelete, chatMsg, conversation); err != nil {
			logger.WarnContext(ctx, "chat bus publish failed for delete", "error", err)
		}
	}

	logger.InfoContext(ctx, "message deleted")

	return connect.NewResponse(&api.DeleteMessageResponse{}), nil
}

// loadAuthoredMessage fetches a message and verifies it belongs to the
// conversation, is a user (not system) message, and was authored by the caller.
// Returns CodeNotFound when missing or cross-conversation, CodePermissionDenied
// when the caller is not the author.
func (s *Service) loadAuthoredMessage(
	ctx context.Context,
	conversationID, messageID, userID string,
) (*models.ChatMessage, error) {
	chatMsg := &models.ChatMessage{}
	if err := s.storage.GetByID(ctx, messageID, chatMsg); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("message not found"))
	}

	if chatMsg.ConversationId != conversationID {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("message not found in conversation"))
	}

	userMsg := chatMsg.GetUserMessage()
	if userMsg == nil {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only user messages can be modified"))
	}

	if userMsg.SenderId != userID {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("only the author can modify this message"))
	}

	return chatMsg, nil
}

// buildReplyContext denormalizes the quoted message into a ReplyContext for a
// reply. Returns nil (with no error) when the parent is missing or not in the
// same conversation — a stale reply target should not block sending the message.
func (s *Service) buildReplyContext(
	ctx context.Context,
	conversationID, replyToMessageID string,
) *models.ReplyContext {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "buildReplyContext",
		"conversation_id", conversationID,
		"reply_to_message_id", replyToMessageID,
	)

	parent := &models.ChatMessage{}
	if err := s.storage.GetByID(ctx, replyToMessageID, parent); err != nil {
		logger.WarnContext(ctx, "reply target not found; sending without quote", "error", err)
		return nil
	}
	if parent.ConversationId != conversationID {
		logger.WarnContext(ctx, "reply target in different conversation; sending without quote")
		return nil
	}

	rc := &models.ReplyContext{
		ReplyToMessageId: replyToMessageID,
		QuotedSenderId:   getSenderID(parent),
		QuotedText:       truncateRunes(getMessageText(parent), quotedTextMaxLen),
	}

	// When the quoted message has no text, carry a representative media id so
	// the client can render a thumbnail instead of an empty snippet.
	if rc.QuotedText == "" {
		if userMsg := parent.GetUserMessage(); userMsg != nil && len(userMsg.MediaIds) > 0 {
			mediaID := userMsg.MediaIds[0]
			rc.QuotedMediaId = &mediaID
		}
	}

	return rc
}

// truncateRunes shortens s to at most maxLen runes, appending an ellipsis when
// truncated. Operates on runes so it never splits a multi-byte character.
func truncateRunes(s string, maxLen int) string {
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen]) + "…"
}
