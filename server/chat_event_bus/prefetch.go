package chat_event_bus

import (
	"context"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// prefetchEntities hydrates the Sender and Transfer fields of pe. The
// conversation is already provided by the caller, so it is not re-fetched.
// Each fetch failure logs at WARN and leaves the corresponding field nil —
// subscribers must tolerate nil. Returning an error here is wrong: the audit
// row was already inserted, so a transient storage hiccup on a denorm read
// must not abort dispatch.
func prefetchEntities(ctx context.Context, s *storage.ProtoSQLStorage, pe *PublishedEvent) {
	if pe == nil || s == nil {
		return
	}

	msgID := ""
	if pe.Message != nil {
		msgID = pe.Message.Id
	}
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "chat_event_bus.prefetch",
		"message_id", msgID,
		"conversation_id", pe.Conversation.Id,
		"chat_event_kind", pe.Kind,
	)

	// Prefetch sender User from the message. For UserMessage and on-behalf-of
	// messages the sender is the UserChatMessage.SenderId; for SystemMessage
	// the actor is SystemChatMessage.ActorId. Reaction updates carry no sender.
	senderID := senderIDFromMessage(pe.Message)
	if senderID != "" {
		user, err := services.FetchAPIUser(ctx, s, senderID)
		if err != nil {
			logger.WarnContext(ctx, "prefetch sender failed", "sender_user_id", senderID, "error", err)
		} else {
			pe.Sender = user
		}
	}

	// Prefetch Transfer when the conversation topic is a transfer. The push
	// subscriber needs the gear_id for the deep-link payload.
	if pe.Conversation != nil {
		if tid := pe.Conversation.GetTopic().GetTransferId(); tid != "" {
			transfer := &models.Transfer{}
			if err := s.GetByID(ctx, tid, transfer); err != nil {
				logger.WarnContext(ctx, "prefetch transfer failed", "transfer_id", tid, "error", err)
			} else {
				pe.Transfer = transfer
			}
		}
	}
}

// senderIDFromMessage extracts the sender/actor user ID from a ChatMessage.
// Returns empty string when the message is nil or carries no sender.
func senderIDFromMessage(msg *models.ChatMessage) string {
	if msg == nil {
		return ""
	}
	switch m := msg.Message.(type) {
	case *models.ChatMessage_UserMessage:
		return m.UserMessage.GetSenderId()
	case *models.ChatMessage_SystemMessage:
		return m.SystemMessage.GetActorId()
	}
	return ""
}
