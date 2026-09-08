package chat

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestGetConversationHistory_ThreadsPollIdToAPI verifies that a
// SystemChatMessage stored with a poll_id surfaces that poll_id on the API
// response. Regression test for #1255 — previously the converter dropped
// poll_id, leaving the client unable to distinguish historical polls.
func TestGetConversationHistory_ThreadsPollIdToAPI(t *testing.T) {
	svc, sqlStorage := setupTestChatService(t)

	communityID := createTestCommunity(t, sqlStorage, "owner1")
	createTestCommunityMembership(t, sqlStorage, communityID, "owner1")
	loanID := createTestLoan(t, sqlStorage, "owner1", "owner1")
	createTestUsers(t, sqlStorage, "owner1", "Owner")

	ctx := contextWithAuth("owner1", "owner@example.com")
	startResp, err := svc.StartConversation(ctx, connect.NewRequest(&api.StartConversationRequest{
		CommunityId: communityID,
		Topic:       &api.ConversationTopic{TopicId: &api.ConversationTopic_TransferId{TransferId: loanID}},
	}))
	if err != nil {
		t.Fatalf("StartConversation failed: %v", err)
	}
	convID := startResp.Msg.ConversationId

	// Insert a system message directly with a poll_id set on it.
	actorID := "owner1"
	pollID := "poll-xyz-123"
	sysMsg := &models.ChatMessage{
		ConversationId: convID,
		SentAtUnixSec:  1000,
		Message: &models.ChatMessage_SystemMessage{
			SystemMessage: &models.SystemChatMessage{
				ActorId:     &actorID,
				Action:      models.ChatSystemAction_CHAT_SYSTEM_ACTION_TIME_PROPOSED,
				Description: "Owner is asking the group when to meet.",
				PollId:      proto.String(pollID),
			},
		},
	}
	if _, err := sqlStorage.Insert(context.Background(), sysMsg); err != nil {
		t.Fatalf("Failed to insert system message: %v", err)
	}

	historyResp, err := svc.GetConversationHistory(ctx, connect.NewRequest(&api.GetConversationHistoryRequest{
		ConversationId: convID,
	}))
	if err != nil {
		t.Fatalf("GetConversationHistory failed: %v", err)
	}

	var sysItem *api.SystemMessage
	for _, m := range historyResp.Msg.Messages {
		if s := m.GetSystemMessage(); s != nil && s.Action == api.ChatSystemAction_CHAT_SYSTEM_ACTION_TIME_PROPOSED {
			sysItem = s
			break
		}
	}
	if sysItem == nil {
		t.Fatalf("TIME_PROPOSED system message missing from history")
	}
	if sysItem.PollId == nil {
		t.Fatalf("expected PollId to be set on API SystemMessage, got nil")
	}
	if *sysItem.PollId != pollID {
		t.Errorf("expected PollId %q, got %q", pollID, *sysItem.PollId)
	}
}
