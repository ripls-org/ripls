package chat

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestCountUnreadUserMessages(t *testing.T) {
	tests := []struct {
		name          string
		messages      []proto.Message
		currentUserID string
		want          int32
	}{
		{
			name:          "empty slice",
			messages:      nil,
			currentUserID: "user1",
			want:          0,
		},
		{
			name: "all user messages unread from others",
			messages: []proto.Message{
				userMsg("user2", map[string]bool{"user1": false, "user2": true}),
				userMsg("user2", map[string]bool{"user1": false, "user2": true}),
			},
			currentUserID: "user1",
			want:          2,
		},
		{
			name: "own messages not counted",
			messages: []proto.Message{
				userMsg("user1", map[string]bool{"user1": true, "user2": false}),
				userMsg("user2", map[string]bool{"user1": false, "user2": true}),
			},
			currentUserID: "user1",
			want:          1,
		},
		{
			name: "read messages not counted",
			messages: []proto.Message{
				userMsg("user2", map[string]bool{"user1": true, "user2": true}),
			},
			currentUserID: "user1",
			want:          0,
		},
		{
			name: "system messages excluded",
			messages: []proto.Message{
				userMsg("user2", map[string]bool{"user1": false, "user2": true}),
				sysMsg("user2", models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED, map[string]bool{"user1": false, "user2": true}),
				sysMsg("user2", models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_CREATED, map[string]bool{"user1": false, "user2": true}),
				sysMsg("user2", models.ChatSystemAction_CHAT_SYSTEM_ACTION_COMPLETED, map[string]bool{"user1": false, "user2": true}),
			},
			currentUserID: "user1",
			want:          1, // Only the user message counts.
		},
		{
			name: "all system messages",
			messages: []proto.Message{
				sysMsg("user2", models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED, map[string]bool{"user1": false}),
				sysMsg("user2", models.ChatSystemAction_CHAT_SYSTEM_ACTION_APPROVED, map[string]bool{"user1": false}),
				sysMsg("user2", models.ChatSystemAction_CHAT_SYSTEM_ACTION_CANCELLED, map[string]bool{"user1": false}),
			},
			currentUserID: "user1",
			want:          0,
		},
		{
			name: "mix of read unread and system",
			messages: []proto.Message{
				userMsg("user2", map[string]bool{"user1": false, "user2": true}), // unread from other
				userMsg("user2", map[string]bool{"user1": true, "user2": true}),  // read from other
				userMsg("user1", map[string]bool{"user1": true, "user2": false}), // own message
				sysMsg("user2", models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED, map[string]bool{"user1": false, "user2": true}),
				userMsg("user3", map[string]bool{"user1": false, "user3": true}), // unread from third user
			},
			currentUserID: "user1",
			want:          2, // Two unread user messages from others.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CountUnreadUserMessages(tt.messages, tt.currentUserID)
			if got != tt.want {
				t.Errorf("CountUnreadUserMessages() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIsSystemMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  *models.ChatMessage
		want bool
	}{
		{
			name: "user message",
			msg: &models.ChatMessage{
				Message: &models.ChatMessage_UserMessage{
					UserMessage: &models.UserChatMessage{SenderId: "user1", Text: "hello"},
				},
			},
			want: false,
		},
		{
			name: "system message - joined",
			msg: &models.ChatMessage{
				Message: &models.ChatMessage_SystemMessage{
					SystemMessage: &models.SystemChatMessage{
						Action: models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED,
					},
				},
			},
			want: true,
		},
		{
			name: "system message - creation anchor",
			msg: &models.ChatMessage{
				Message: &models.ChatMessage_SystemMessage{
					SystemMessage: &models.SystemChatMessage{
						Action: models.ChatSystemAction_CHAT_SYSTEM_ACTION_EXPERIENCE_CREATED,
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSystemMessage(tt.msg)
			if got != tt.want {
				t.Errorf("IsSystemMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

// userMsg creates a user chat message for testing.
func userMsg(senderID string, readStatus map[string]bool) *models.ChatMessage {
	return &models.ChatMessage{
		Message: &models.ChatMessage_UserMessage{
			UserMessage: &models.UserChatMessage{
				SenderId: senderID,
				Text:     "test message",
			},
		},
		ParticipantIdToIsRead: readStatus,
	}
}

// sysMsg creates a system chat message for testing.
func sysMsg(actorID string, action models.ChatSystemAction, readStatus map[string]bool) *models.ChatMessage {
	return &models.ChatMessage{
		Message: &models.ChatMessage_SystemMessage{
			SystemMessage: &models.SystemChatMessage{
				ActorId:     &actorID,
				Action:      action,
				Description: "system event",
			},
		},
		ParticipantIdToIsRead: readStatus,
	}
}
