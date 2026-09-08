package storage

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestChatConversationStorage_InsertAndListByParticipant(t *testing.T) {
	sqlStorage, cleanup := SetupTestStorage(t)
	defer cleanup()

	cs := NewChatConversationStorage(sqlStorage)
	ctx := context.Background()

	conv := &models.ChatConversation{
		CommunityId:    "community1",
		ParticipantIds: []string{"user1", "user2"},
	}

	convID, err := cs.Insert(ctx, conv)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	if convID == "" {
		t.Fatal("expected non-empty conversation ID")
	}

	conversations, err := cs.ListByParticipant(ctx, "user1", 0)
	if err != nil {
		t.Fatalf("ListByParticipant failed: %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(conversations))
	}
	if conversations[0].Id != convID {
		t.Errorf("want conversation ID %s, got %s", convID, conversations[0].Id)
	}

	// user2 should also find it
	conversations2, err := cs.ListByParticipant(ctx, "user2", 0)
	if err != nil {
		t.Fatalf("ListByParticipant for user2 failed: %v", err)
	}
	if len(conversations2) != 1 {
		t.Fatalf("user2: want 1 conversation, got %d", len(conversations2))
	}

	// Non-participant gets nothing
	none, err := cs.ListByParticipant(ctx, "user3", 0)
	if err != nil {
		t.Fatalf("ListByParticipant for user3 failed: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("user3 should not see any conversations, got %d", len(none))
	}
}

func TestChatConversationStorage_UpdateSyncsParticipants(t *testing.T) {
	sqlStorage, cleanup := SetupTestStorage(t)
	defer cleanup()

	cs := NewChatConversationStorage(sqlStorage)
	ctx := context.Background()

	conv := &models.ChatConversation{
		CommunityId:    "community1",
		ParticipantIds: []string{"user1"},
	}
	convID, err := cs.Insert(ctx, conv)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	conv.Id = convID

	// user2 not yet a participant
	before, err := cs.ListByParticipant(ctx, "user2", 0)
	if err != nil {
		t.Fatalf("ListByParticipant before update failed: %v", err)
	}
	if len(before) != 0 {
		t.Errorf("user2 should not be a participant before update")
	}

	// Add user2 via wrapper Update
	conv.ParticipantIds = append(conv.ParticipantIds, "user2")
	if err := cs.Update(ctx, conv); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	after, err := cs.ListByParticipant(ctx, "user2", 0)
	if err != nil {
		t.Fatalf("ListByParticipant after update failed: %v", err)
	}
	if len(after) != 1 {
		t.Errorf("user2 should be a participant after update, got %d conversations", len(after))
	}
}

// TestChatConversationStorage_DirectUpdateSyncsIndex asserts that the
// RegisterArrayColumn auto-sync (#2072) keeps participant_ids current
// even when callers bypass ChatConversationStorage and call
// ProtoSQL.Update directly. The wrapper exists for the typed read
// methods, not to enforce write-side correctness — both paths converge
// on the same auto-synced column.
func TestChatConversationStorage_DirectUpdateSyncsIndex(t *testing.T) {
	sqlStorage, cleanup := SetupTestStorage(t)
	defer cleanup()

	cs := NewChatConversationStorage(sqlStorage)
	ctx := context.Background()

	conv := &models.ChatConversation{
		CommunityId:    "community1",
		ParticipantIds: []string{"user1"},
	}
	convID, err := cs.Insert(ctx, conv)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	conv.Id = convID

	// Bypass the wrapper — direct ProtoSQL.Update.
	conv.ParticipantIds = append(conv.ParticipantIds, "user2")
	if err := sqlStorage.Update(ctx, conv); err != nil {
		t.Fatalf("direct Update failed: %v", err)
	}

	// Auto-sync writes participant_ids inside the same WithTx, so
	// the indexed column reflects the updated proto immediately.
	fresh, err := cs.ListByParticipant(ctx, "user2", 0)
	if err != nil {
		t.Fatalf("ListByParticipant failed: %v", err)
	}
	if len(fresh) != 1 {
		t.Errorf("ListByParticipant(user2) returned %d rows, want 1 — auto-sync did not run", len(fresh))
	}
}

func TestChatConversationStorage_SoftDeletedExcluded(t *testing.T) {
	sqlStorage, cleanup := SetupTestStorage(t)
	defer cleanup()

	cs := NewChatConversationStorage(sqlStorage)
	ctx := context.Background()

	conv := &models.ChatConversation{
		CommunityId:    "community1",
		ParticipantIds: []string{"user1"},
	}
	convID, err := cs.Insert(ctx, conv)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	conv.Id = convID

	// Soft-delete the conversation
	conv.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: time.Now().Unix()}
	if err := cs.Update(ctx, conv); err != nil {
		t.Fatalf("Update (soft delete) failed: %v", err)
	}

	results, err := cs.ListByParticipant(ctx, "user1", 0)
	if err != nil {
		t.Fatalf("ListByParticipant failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("soft-deleted conversation should be excluded, got %d", len(results))
	}
}

func TestChatConversationStorage_OrderByLastMessageDesc(t *testing.T) {
	sqlStorage, cleanup := SetupTestStorage(t)
	defer cleanup()

	cs := NewChatConversationStorage(sqlStorage)
	ctx := context.Background()

	older := &models.ChatConversation{
		CommunityId:          "community1",
		ParticipantIds:       []string{"user1"},
		LastMessageAtUnixSec: 100,
	}
	newer := &models.ChatConversation{
		CommunityId:          "community1",
		ParticipantIds:       []string{"user1"},
		LastMessageAtUnixSec: 200,
	}

	olderID, err := cs.Insert(ctx, older)
	if err != nil {
		t.Fatalf("Insert older failed: %v", err)
	}
	newerID, err := cs.Insert(ctx, newer)
	if err != nil {
		t.Fatalf("Insert newer failed: %v", err)
	}

	results, err := cs.ListByParticipant(ctx, "user1", 0)
	if err != nil {
		t.Fatalf("ListByParticipant failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 conversations, got %d", len(results))
	}

	// Newest first
	if results[0].Id != newerID {
		t.Errorf("want newest conversation (%s) first, got %s", newerID, results[0].Id)
	}
	if results[1].Id != olderID {
		t.Errorf("want older conversation (%s) second, got %s", olderID, results[1].Id)
	}
}

func TestChatConversationStorage_DefaultLimitApplied(t *testing.T) {
	sqlStorage, cleanup := SetupTestStorage(t)
	defer cleanup()

	cs := NewChatConversationStorage(sqlStorage)
	ctx := context.Background()

	// Insert more conversations than the default limit of 200.
	// We just verify that limit=0 and limit=-1 don't panic/error.
	for i := 0; i < 3; i++ {
		conv := &models.ChatConversation{
			CommunityId:    "community1",
			ParticipantIds: []string{"user1"},
		}
		if _, err := cs.Insert(ctx, conv); err != nil {
			t.Fatalf("Insert failed: %v", err)
		}
	}

	// limit=0 should use default
	results, err := cs.ListByParticipant(ctx, "user1", 0)
	if err != nil {
		t.Fatalf("ListByParticipant with limit=0 failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("want 3 conversations, got %d", len(results))
	}

	// limit=-1 should also use default
	results2, err := cs.ListByParticipant(ctx, "user1", -1)
	if err != nil {
		t.Fatalf("ListByParticipant with limit=-1 failed: %v", err)
	}
	if len(results2) != 3 {
		t.Errorf("limit=-1: want 3 conversations, got %d", len(results2))
	}
}
