package chat

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestChatMessageSoftDeleteFilterInvariant asserts the load-bearing
// invariant documented in docs/ai/undo_plan.md §7.1: once a
// ChatMessage has its Deleted field populated, it must not appear in
// any standard storage query result.
//
// The filter is applied by the generic storage layer (see
// server/storage/protosql_schema.go#buildDeletedFilter), not by chat
// code — so this test is really a regression guard: if someone removes
// the DeletedMetadata field from models.ChatMessage, or the storage
// layer stops honoring the field on ChatMessage, this test breaks
// before undo retraction silently leaks soft-deleted messages to
// clients.
func TestChatMessageSoftDeleteFilterInvariant(t *testing.T) {
	store, _ := setupTestStorage(t)
	ctx := context.Background()

	conversation := &models.ChatConversation{
		CommunityId:    "community123",
		ParticipantIds: []string{"user1", "user2"},
	}
	convID, err := store.Insert(ctx, conversation)
	if err != nil {
		t.Fatalf("failed to insert conversation: %v", err)
	}

	// Insert two messages — one that will stay, one that will be soft-deleted.
	keptID, err := store.Insert(ctx, &models.ChatMessage{
		ConversationId: convID,
		SentAtUnixSec:  clock.UnixSec(ctx),
		Message: &models.ChatMessage_SystemMessage{
			SystemMessage: &models.SystemChatMessage{
				Action:      models.ChatSystemAction_CHAT_SYSTEM_ACTION_JOINED,
				Description: "kept message",
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to insert kept message: %v", err)
	}

	deletedID, err := store.Insert(ctx, &models.ChatMessage{
		ConversationId: convID,
		SentAtUnixSec:  clock.UnixSec(ctx),
		Message: &models.ChatMessage_SystemMessage{
			SystemMessage: &models.SystemChatMessage{
				Action:      models.ChatSystemAction_CHAT_SYSTEM_ACTION_COMPLETED,
				Description: "will be soft-deleted",
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to insert deleted message: %v", err)
	}

	// Soft-delete the second message — same pattern an Undo* RPC would
	// use when retracting a system message.
	toDelete := &models.ChatMessage{}
	if err := store.GetByID(ctx, deletedID, toDelete); err != nil {
		t.Fatalf("failed to get message to delete: %v", err)
	}
	toDelete.Deleted = &models.DeletedMetadata{
		DeletedByUserId:  "user1",
		DeletedAtUnixSec: clock.UnixSec(ctx),
	}
	if err := store.Update(ctx, toDelete); err != nil {
		t.Fatalf("failed to soft-delete message: %v", err)
	}

	// QueryByField — the most common chat read pattern — must exclude the
	// deleted row.
	messages, err := store.QueryByField(ctx, "conversation_id", convID, &models.ChatMessage{})
	if err != nil {
		t.Fatalf("QueryByField failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message after soft-delete, got %d", len(messages))
	}
	if got := messages[0].(*models.ChatMessage).Id; got != keptID {
		t.Fatalf("QueryByField returned wrong message: got %q, want %q (deleted was %q)", got, keptID, deletedID)
	}

	// GetByID must also refuse to return the soft-deleted row.
	fetched := &models.ChatMessage{}
	if err := store.GetByID(ctx, deletedID, fetched); err == nil {
		t.Fatalf("GetByID returned a soft-deleted message; filter invariant violated")
	}
}
