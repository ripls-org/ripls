package portfolio

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func convWithTransfer(convID, transferID string) *models.ChatConversation {
	return &models.ChatConversation{
		Id: convID,
		Topic: &models.ConversationTopic{
			TopicId: &models.ConversationTopic_TransferId{TransferId: transferID},
		},
	}
}

func convWithRequest(convID, requestID string) *models.ChatConversation {
	return &models.ChatConversation{
		Id: convID,
		Topic: &models.ConversationTopic{
			TopicId: &models.ConversationTopic_RequestId{RequestId: requestID},
		},
	}
}

func makeUnreadMsg(convID, senderID string) *models.ChatMessage {
	return &models.ChatMessage{
		Id:             "msg-" + convID,
		ConversationId: convID,
		Message: &models.ChatMessage_UserMessage{
			UserMessage: &models.UserChatMessage{
				SenderId: senderID,
				Text:     "hello",
			},
		},
		ParticipantIdToIsRead: map[string]bool{senderID: true},
	}
}

// TestBuildConvUnreadCounts_SkipsTerminalItems verifies that conversations linked
// to completed/cancelled transfers or fulfilled/cancelled requests are excluded
// from unread counts, while active-item conversations are included.
func TestBuildConvUnreadCounts_SkipsTerminalItems(t *testing.T) {
	const viewer = "user-viewer"
	const sender = "user-sender"

	tests := []struct {
		name        string
		convByID    map[string]*models.ChatConversation
		transferMap map[string]proto.Message
		reqMap      map[string]proto.Message
		activeConv  string
		doneConv    string
	}{
		{
			name: "completed transfer excluded",
			convByID: map[string]*models.ChatConversation{
				"conv-active": convWithTransfer("conv-active", "t-active"),
				"conv-done":   convWithTransfer("conv-done", "t-done"),
			},
			transferMap: map[string]proto.Message{
				"t-active": &models.Transfer{Id: "t-active", State: models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED},
				"t-done":   &models.Transfer{Id: "t-done", State: models.TransferState_TRANSFER_STATE_COMPLETED},
			},
			activeConv: "conv-active",
			doneConv:   "conv-done",
		},
		{
			name: "cancelled transfer excluded",
			convByID: map[string]*models.ChatConversation{
				"conv-active": convWithTransfer("conv-active", "t-active"),
				"conv-done":   convWithTransfer("conv-done", "t-cancelled"),
			},
			transferMap: map[string]proto.Message{
				"t-active":    &models.Transfer{Id: "t-active", State: models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED},
				"t-cancelled": &models.Transfer{Id: "t-cancelled", State: models.TransferState_TRANSFER_STATE_CANCELLED},
			},
			activeConv: "conv-active",
			doneConv:   "conv-done",
		},
		{
			name: "fulfilled request excluded",
			convByID: map[string]*models.ChatConversation{
				"conv-active": convWithRequest("conv-active", "r-active"),
				"conv-done":   convWithRequest("conv-done", "r-fulfilled"),
			},
			reqMap: map[string]proto.Message{
				"r-active":    &models.Request{Id: "r-active", State: models.RequestState_REQUEST_STATE_ACTIVE},
				"r-fulfilled": &models.Request{Id: "r-fulfilled", State: models.RequestState_REQUEST_STATE_FULFILLED},
			},
			activeConv: "conv-active",
			doneConv:   "conv-done",
		},
		{
			name: "cancelled request excluded",
			convByID: map[string]*models.ChatConversation{
				"conv-active": convWithRequest("conv-active", "r-active"),
				"conv-done":   convWithRequest("conv-done", "r-cancelled"),
			},
			reqMap: map[string]proto.Message{
				"r-active":    &models.Request{Id: "r-active", State: models.RequestState_REQUEST_STATE_ACTIVE},
				"r-cancelled": &models.Request{Id: "r-cancelled", State: models.RequestState_REQUEST_STATE_CANCELLED},
			},
			activeConv: "conv-active",
			doneConv:   "conv-done",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgsByConv := map[string][]proto.Message{
				tt.activeConv: {makeUnreadMsg(tt.activeConv, sender)},
				tt.doneConv:   {makeUnreadMsg(tt.doneConv, sender)},
			}

			counts := buildConvUnreadCounts(
				tt.convByID, msgsByConv,
				tt.transferMap, tt.reqMap,
				nil,
				nil, nil,
				viewer,
			)

			if counts[tt.doneConv] != 0 {
				t.Errorf("expected zero unread count for terminal-item conv, got %d", counts[tt.doneConv])
			}
			if counts[tt.activeConv] == 0 {
				t.Errorf("expected non-zero unread count for active-item conv")
			}
		})
	}
}

// TestBuildConvUnreadCounts_NoDBCallRequired verifies that buildConvUnreadCounts
// operates entirely from pre-fetched maps without any storage dependency.
func TestBuildConvUnreadCounts_NoDBCallRequired(t *testing.T) {
	const viewer = "user-viewer"
	const sender = "user-sender"

	convID := "conv-1"
	convByID := map[string]*models.ChatConversation{
		convID: convWithTransfer(convID, "t1"),
	}
	msgsByConv := map[string][]proto.Message{
		convID: {makeUnreadMsg(convID, sender)},
	}
	transferMap := map[string]proto.Message{
		"t1": &models.Transfer{Id: "t1", State: models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED},
	}

	counts := buildConvUnreadCounts(
		convByID, msgsByConv,
		transferMap, nil,
		nil,
		nil, nil,
		viewer,
	)

	if _, ok := counts[convID]; !ok {
		t.Error("expected unread entry for active conversation")
	}
}
