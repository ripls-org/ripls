package chat

import (
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// CountUnreadUserMessages counts messages from other users that are unread by
// the current user, excluding all system messages. System messages represent
// automated state transitions (joins, approvals, completions, etc.) and should
// not inflate unread badges.
func CountUnreadUserMessages(messages []proto.Message, currentUserID string) int32 {
	var count int32
	for _, m := range messages {
		msg := m.(*models.ChatMessage)
		if msg.GetSystemMessage() != nil {
			continue
		}
		senderID := userMessageSenderID(msg)
		if senderID != currentUserID && !msg.ParticipantIdToIsRead[currentUserID] {
			count++
		}
	}
	return count
}

// IsSystemMessage reports whether a message is a system message (any action).
func IsSystemMessage(msg *models.ChatMessage) bool {
	return msg.GetSystemMessage() != nil
}

// userMessageSenderID extracts the sender ID from a user message.
// Returns empty string for system messages or unknown types.
func userMessageSenderID(msg *models.ChatMessage) string {
	if um := msg.GetUserMessage(); um != nil {
		return um.SenderId
	}
	return ""
}
