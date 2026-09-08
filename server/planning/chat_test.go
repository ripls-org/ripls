package planning

import (
	"context"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// createConversation inserts a minimal ChatConversation and returns its ID.
func createConversation(t *testing.T, ctx context.Context, st *storage.ProtoSQLStorage) string {
	t.Helper()
	conv := &models.ChatConversation{
		ParticipantIds: []string{"user-1"},
	}
	id, err := st.Insert(ctx, conv)
	if err != nil {
		t.Fatalf("insert ChatConversation: %v", err)
	}
	return id
}

// getSystemMsgs returns all system chat messages for a conversation.
func getSystemMsgs(t *testing.T, ctx context.Context, st *storage.ProtoSQLStorage, convID string) []*models.SystemChatMessage {
	t.Helper()
	raw, err := st.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("QueryByField chat_message: %v", err)
	}
	var out []*models.SystemChatMessage
	for _, m := range raw {
		chatMsg := m.(*models.ChatMessage)
		if sm, ok := chatMsg.Message.(*models.ChatMessage_SystemMessage); ok {
			out = append(out, sm.SystemMessage)
		}
	}
	return out
}

// getUserMsgs returns all user chat messages for a conversation.
func getUserMsgs(t *testing.T, ctx context.Context, st *storage.ProtoSQLStorage, convID string) []*models.UserChatMessage {
	t.Helper()
	raw, err := st.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("QueryByField chat_message: %v", err)
	}
	var out []*models.UserChatMessage
	for _, m := range raw {
		chatMsg := m.(*models.ChatMessage)
		if um, ok := chatMsg.Message.(*models.ChatMessage_UserMessage); ok {
			out = append(out, um.UserMessage)
		}
	}
	return out
}

// TestPostNeedAdded_WithNote_EmitsUserMessage verifies that when a note is provided,
// InsertUserMessageOnBehalfOf is called with the correct NeedID and NeedName.
func TestPostNeedAdded_WithNote_EmitsUserMessage(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()
	convID := createConversation(t, ctx, st)
	writer := chat.NewSystemMessageWriter(st, nil)

	note := "A 2019 Tempranillo"
	need := &models.PlanningNeed{Id: "need-wn-1", Name: "Red wine", Note: &note}
	if err := PostNeedAdded(ctx, writer, convID, "user-1", "Alice", need, nil); err != nil {
		t.Fatalf("PostNeedAdded: %v", err)
	}

	userMsgs := getUserMsgs(t, ctx, st, convID)
	var found bool
	for _, um := range userMsgs {
		if um.GetNeedId() == "need-wn-1" {
			found = true
			if !strings.Contains(um.Text, "Tempranillo") {
				t.Errorf("user message text %q missing note content", um.Text)
			}
		}
	}
	if !found {
		t.Error("no user message with NeedID='need-wn-1' found")
	}
}

// TestPostNeedAdded_NoNote_EmitsSystemMessage verifies that when no note is provided,
// a PLANNING_NEED_ADDED system message is inserted with the correct coalesce key.
func TestPostNeedAdded_NoNote_EmitsSystemMessage(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()
	convID := createConversation(t, ctx, st)
	writer := chat.NewSystemMessageWriter(st, nil)

	need := &models.PlanningNeed{Id: "need-nn-1", Name: "Chips"}
	if err := PostNeedAdded(ctx, writer, convID, "user-1", "Alice", need, nil); err != nil {
		t.Fatalf("PostNeedAdded: %v", err)
	}

	sysMsgs := getSystemMsgs(t, ctx, st, convID)
	var found bool
	for _, sm := range sysMsgs {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_ADDED {
			found = true
			if sm.GetCoalesceKey() != "need-nn-1" {
				t.Errorf("coalesce key = %q, want %q", sm.GetCoalesceKey(), "need-nn-1")
			}
		}
	}
	if !found {
		t.Error("no PLANNING_NEED_ADDED system message found")
	}
}

// TestPostNeedAdded_NilWriter_NoOp verifies that a nil writer returns nil error.
func TestPostNeedAdded_NilWriter_NoOp(t *testing.T) {
	ctx := context.Background()
	need := &models.PlanningNeed{Id: "need-nw", Name: "Test"}
	if err := PostNeedAdded(ctx, nil, "conv-nw", "user-1", "Alice", need, nil); err != nil {
		t.Errorf("PostNeedAdded with nil writer returned error: %v", err)
	}
}

// TestPostNeedAdded_EmptyConversationID_NoOp verifies that an empty conversationID is a no-op.
func TestPostNeedAdded_EmptyConversationID_NoOp(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()
	writer := chat.NewSystemMessageWriter(st, nil)

	need := &models.PlanningNeed{Id: "need-ec", Name: "Test"}
	if err := PostNeedAdded(ctx, writer, "", "user-1", "Alice", need, nil); err != nil {
		t.Errorf("PostNeedAdded with empty conversationID returned error: %v", err)
	}
}

// TestPostNeedRemoved_EmitsSystemMessage verifies PLANNING_NEED_REMOVED is emitted.
func TestPostNeedRemoved_EmitsSystemMessage(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()
	convID := createConversation(t, ctx, st)
	writer := chat.NewSystemMessageWriter(st, nil)

	PostNeedRemoved(ctx, writer, convID, "user-1", "Alice", "need-nr-1", "Chips")

	sysMsgs := getSystemMsgs(t, ctx, st, convID)
	var found bool
	for _, sm := range sysMsgs {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_REMOVED {
			found = true
		}
	}
	if !found {
		t.Error("no PLANNING_NEED_REMOVED system message found")
	}
}

// TestPostNeedRemoved_NilWriter_NoOp verifies nil writer and empty conversationID are no-ops.
func TestPostNeedRemoved_NilWriter_NoOp(t *testing.T) {
	ctx := context.Background()
	PostNeedRemoved(ctx, nil, "conv-nrnw", "user-1", "Alice", "need-1", "Test")
	PostNeedRemoved(ctx, nil, "", "user-1", "Alice", "need-1", "Test")
}

// TestPostNeedClaimed_WithNote_EmitsUserMessage verifies that when a contribution has
// a description, InsertUserMessageOnBehalfOf is called.
func TestPostNeedClaimed_WithNote_EmitsUserMessage(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()
	convID := createConversation(t, ctx, st)
	writer := chat.NewSystemMessageWriter(st, nil)

	desc := "Tiramisu"
	contrib := &models.PlanningContribution{Id: "contrib-nc-1", Title: "Dessert", Description: &desc}
	need := &models.PlanningNeed{Id: "need-nc-1", Name: "Bring dessert"}

	if err := PostNeedClaimed(ctx, writer, convID, "user-1", "Alice", need, contrib); err != nil {
		t.Fatalf("PostNeedClaimed: %v", err)
	}

	userMsgs := getUserMsgs(t, ctx, st, convID)
	var found bool
	for _, um := range userMsgs {
		if um.GetContributionId() == "contrib-nc-1" {
			found = true
		}
	}
	if !found {
		t.Error("no user message with ContributionID='contrib-nc-1' found")
	}
}

// TestPostNeedClaimed_NoNote_EmitsSystemMessage verifies that without a description
// a PLANNING_NEED_CLAIMED system message is emitted.
func TestPostNeedClaimed_NoNote_EmitsSystemMessage(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()
	convID := createConversation(t, ctx, st)
	writer := chat.NewSystemMessageWriter(st, nil)

	contrib := &models.PlanningContribution{Id: "contrib-nc-2", Title: "Drinks"}
	need := &models.PlanningNeed{Id: "need-nc-2", Name: "Bring drinks"}

	if err := PostNeedClaimed(ctx, writer, convID, "user-1", "Alice", need, contrib); err != nil {
		t.Fatalf("PostNeedClaimed: %v", err)
	}

	sysMsgs := getSystemMsgs(t, ctx, st, convID)
	var found bool
	for _, sm := range sysMsgs {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_NEED_CLAIMED {
			found = true
		}
	}
	if !found {
		t.Error("no PLANNING_NEED_CLAIMED system message found")
	}
}

// TestPostContributionAdded_WithDescription_EmitsUserMessage verifies that when a
// description is present, InsertUserMessageOnBehalfOf is called.
func TestPostContributionAdded_WithDescription_EmitsUserMessage(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()
	convID := createConversation(t, ctx, st)
	writer := chat.NewSystemMessageWriter(st, nil)

	desc := "A lovely salad"
	contrib := &models.PlanningContribution{Id: "contrib-ca-1", Title: "Salad", Description: &desc}

	if err := PostContributionAdded(ctx, writer, convID, "user-1", "Alice", contrib); err != nil {
		t.Fatalf("PostContributionAdded: %v", err)
	}

	userMsgs := getUserMsgs(t, ctx, st, convID)
	var found bool
	for _, um := range userMsgs {
		if um.GetContributionId() == "contrib-ca-1" {
			found = true
		}
	}
	if !found {
		t.Error("no user message with ContributionID='contrib-ca-1' found")
	}
}

// TestPostContributionAdded_NoDescription_EmitsSystemMessage verifies that without a
// description a PLANNING_CONTRIBUTION_ADDED system message is emitted.
func TestPostContributionAdded_NoDescription_EmitsSystemMessage(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()
	convID := createConversation(t, ctx, st)
	writer := chat.NewSystemMessageWriter(st, nil)

	contrib := &models.PlanningContribution{Id: "contrib-cano-1", Title: "Flowers"}
	if err := PostContributionAdded(ctx, writer, convID, "user-1", "Alice", contrib); err != nil {
		t.Fatalf("PostContributionAdded: %v", err)
	}

	sysMsgs := getSystemMsgs(t, ctx, st, convID)
	var found bool
	for _, sm := range sysMsgs {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_CONTRIBUTION_ADDED {
			found = true
		}
	}
	if !found {
		t.Error("no PLANNING_CONTRIBUTION_ADDED system message found")
	}
}

// TestPostContributionRemoved_EmitsSystemMessage verifies PLANNING_CONTRIBUTION_REMOVED is emitted.
func TestPostContributionRemoved_EmitsSystemMessage(t *testing.T) {
	st := setupTestStorage(t)
	ctx := context.Background()
	convID := createConversation(t, ctx, st)
	writer := chat.NewSystemMessageWriter(st, nil)

	PostContributionRemoved(ctx, writer, convID, "user-1", "Alice", "contrib-cr-1", "Flowers")

	sysMsgs := getSystemMsgs(t, ctx, st, convID)
	var found bool
	for _, sm := range sysMsgs {
		if sm.Action == models.ChatSystemAction_CHAT_SYSTEM_ACTION_PLANNING_CONTRIBUTION_REMOVED {
			found = true
		}
	}
	if !found {
		t.Error("no PLANNING_CONTRIBUTION_REMOVED system message found")
	}
}

// TestPostContributionRemoved_NilWriter_NoOp verifies nil writer and empty conversationID are no-ops.
func TestPostContributionRemoved_NilWriter_NoOp(t *testing.T) {
	ctx := context.Background()
	PostContributionRemoved(ctx, nil, "conv-crnw", "user-1", "Alice", "contrib-1", "Test")
	PostContributionRemoved(ctx, nil, "", "user-1", "Alice", "contrib-1", "Test")
}
