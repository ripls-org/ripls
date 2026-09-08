package chat

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// StreamMessages streams messages for a conversation in real-time.
func (s *Service) StreamMessages(
	ctx context.Context,
	req *connect.Request[api.StreamMessagesRequest],
	stream *connect.ServerStream[api.StreamMessagesResponse],
) error {
	streamStartTime := time.Now()

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"conversation_id", req.Msg.ConversationId,
		"operation", "StreamMessages",
	)

	logger.InfoContext(ctx, "stream started")

	// Verify user has access to this conversation. Community-wide conversations
	// have no stored ParticipantIds (membership is dynamic), so fall back to
	// community membership check when the participant list is empty.
	if _, err := s.requireConversationAccess(ctx, req.Msg.ConversationId, authInfo.UserID); err != nil {
		return err
	}

	// Register for new messages BEFORE querying existing ones. This closes the
	// race window where a broadcast could fire between the query and registration,
	// causing the message to be lost (not in query results AND no listener).
	// Messages that land on the channel during the query are deduplicated below.
	messageChan := make(chan *api.StreamMessagesResponse, 10)
	s.registerStream(ctx, req.Msg.ConversationId, authInfo.UserID, messageChan)
	defer s.unregisterStream(ctx, req.Msg.ConversationId, messageChan)

	// Send existing messages immediately, sorted oldest-first so the client
	// receives backlog in chronological order rather than PostgreSQL heap order.
	messages, err := storage.ListByConversation(ctx, s.storage, req.Msg.ConversationId, true /* asc */, 0 /* default limit */)
	if err != nil {
		return connecterr.Internal(ctx, "StreamMessages.ListByConversation", err,
			"conversation_id", req.Msg.ConversationId,
		)
	}

	sentIDs := make(map[string]bool, len(messages))
	for _, chatMsg := range messages {
		// Convert to API format
		event, err := s.chatMessageToStreamResponse(ctx, chatMsg)
		if err != nil {
			return connecterr.Internal(ctx, "StreamMessages.chatMessageToStreamResponse", err,
				"conversation_id", req.Msg.ConversationId,
				"message_id", chatMsg.Id,
			)
		}

		if err := stream.Send(event); err != nil {
			duration := time.Since(streamStartTime)
			logger.WarnContext(ctx, "stream ended during initial message send",
				"error", err,
				"message_id", chatMsg.Id,
				"duration_ms", duration.Milliseconds(),
			)
			return err
		}
		sentIDs[chatMsg.Id] = true
		logger.DebugContext(ctx, "delivered backlog message", "message_id", chatMsg.Id)
	}

	// Stream new messages, skipping any already sent as existing
	for {
		select {
		case <-ctx.Done():
			duration := time.Since(streamStartTime)
			logger.InfoContext(ctx, "stream ended normally",
				"reason", "context_done",
				"context_error", ctx.Err(),
				"duration_ms", duration.Milliseconds(),
			)
			return nil
		case event := <-messageChan:
			if sentIDs[event.MessageId] {
				continue
			}
			if err := stream.Send(event); err != nil {
				duration := time.Since(streamStartTime)
				logger.WarnContext(ctx, "stream ended with send error",
					"error", err,
					"message_id", event.MessageId,
					"duration_ms", duration.Milliseconds(),
				)
				return err
			}
			logger.DebugContext(ctx, "delivered live message", "message_id", event.MessageId)
		}
	}
}

// registerStream adds a message channel to the stream registry.
// ctx is the StreamMessages handler's request context — its request_id
// scopes the entire stream so register/unregister log lines match every
// other line emitted during the stream's lifetime.
func (s *Service) registerStream(ctx context.Context, conversationID, userID string, ch chan *api.StreamMessagesResponse) {
	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()

	s.streams[conversationID] = append(s.streams[conversationID], streamInfo{
		userID: userID,
		ch:     ch,
	})

	logger := logging.LoggerWithContext(ctx).With(
		"conversation_id", conversationID,
		"user_id", userID,
		"total_streams", len(s.streams[conversationID]),
	)
	logger.InfoContext(ctx, "registered stream")

	// Set user as foreground when stream is registered (default assumption)
	s.presenceMu.Lock()
	s.userInForeground[userID] = true
	s.presenceMu.Unlock()
}

// unregisterStream removes a message channel from the stream registry.
func (s *Service) unregisterStream(ctx context.Context, conversationID string, ch chan *api.StreamMessagesResponse) {
	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()

	streams := s.streams[conversationID]
	var unregisteredUserID string
	for i, stream := range streams {
		if stream.ch == ch {
			unregisteredUserID = stream.userID
			// Remove this stream
			s.streams[conversationID] = append(streams[:i], streams[i+1:]...)
			close(ch)

			logger := logging.LoggerWithContext(ctx).With(
				"conversation_id", conversationID,
				"user_id", unregisteredUserID,
				"remaining_streams", len(s.streams[conversationID]),
			)
			logger.InfoContext(ctx, "unregistered stream")
			break
		}
	}

	// Clean up empty entries
	if len(s.streams[conversationID]) == 0 {
		delete(s.streams, conversationID)
	}

	// Set user as background when stream closes (fallback for app crashes)
	// Only if this user has no other active streams
	if unregisteredUserID != "" {
		hasOtherStreams := false
		for _, streamList := range s.streams {
			for _, stream := range streamList {
				if stream.userID == unregisteredUserID {
					hasOtherStreams = true
					break
				}
			}
			if hasOtherStreams {
				break
			}
		}

		if !hasOtherStreams {
			s.presenceMu.Lock()
			s.userInForeground[unregisteredUserID] = false
			s.presenceMu.Unlock()

			logger := logging.LoggerWithContext(ctx).With("user_id", unregisteredUserID)
			logger.InfoContext(ctx, "set user to background - no active streams remaining")
		}
	}
}

// broadcastMessage sends a message event to all registered streams for a conversation.
// ctx carries the originating request's correlation fields (request_id,
// user_id, etc.) so the fan-out lines can be tied back to the action that
// triggered the broadcast — e.g. a SendMessage RPC's request_id flows
// through to logs about delivering that message to subscribers.
func (s *Service) broadcastMessage(ctx context.Context, conversationID string, event *api.StreamMessagesResponse) {
	s.streamsMu.RLock()
	streams := s.streams[conversationID]
	// Make a copy to avoid holding the lock while sending
	streamsCopy := make([]streamInfo, len(streams))
	copy(streamsCopy, streams)
	s.streamsMu.RUnlock()

	if len(streamsCopy) == 0 {
		return
	}

	logger := logging.LoggerWithContext(ctx).With(
		"conversation_id", conversationID,
		"message_id", event.MessageId,
		"stream_count", len(streamsCopy),
	)
	logger.DebugContext(ctx, "broadcasting message")

	// Send to all streams with timeout to prevent blocking
	for _, stream := range streamsCopy {
		select {
		case stream.ch <- proto.Clone(event).(*api.StreamMessagesResponse):
			// Sent successfully
		case <-time.After(1 * time.Second):
			logger.ErrorContext(ctx, "timeout sending to stream",
				"recipient_user_id", stream.userID,
			)
		}
	}
}

// hasActiveStream checks if a user has an active stream for a conversation.
func (s *Service) hasActiveStream(conversationID, userID string) bool {
	s.streamsMu.RLock()
	defer s.streamsMu.RUnlock()

	streams, exists := s.streams[conversationID]
	if !exists {
		return false
	}

	// Check if any stream belongs to this user
	for _, stream := range streams {
		if stream.userID == userID {
			return true
		}
	}

	return false
}
