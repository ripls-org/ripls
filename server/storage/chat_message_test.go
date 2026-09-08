package storage

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func insertTestChatMessage(t *testing.T, s *ProtoSQLStorage, conversationID string, sentAtUnixSec int64) string {
	t.Helper()
	msg := &models.ChatMessage{
		ConversationId: conversationID,
		SentAtUnixSec:  sentAtUnixSec,
		Message: &models.ChatMessage_UserMessage{
			UserMessage: &models.UserChatMessage{
				SenderId: "user1",
			},
		},
	}
	id, err := s.Insert(context.Background(), msg)
	if err != nil {
		t.Fatalf("failed to insert test chat message: %v", err)
	}
	return id
}

func TestChatMessageStorage_ListByConversation_OrdersBySentAt(t *testing.T) {
	s, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	convID := "conv-order-test"

	// Insert in non-chronological order: t+30, t, t+15
	idT30 := insertTestChatMessage(t, s, convID, 1030)
	idT0 := insertTestChatMessage(t, s, convID, 1000)
	idT15 := insertTestChatMessage(t, s, convID, 1015)

	t.Run("ASC returns oldest first", func(t *testing.T) {
		msgs, err := ListByConversation(ctx, s, convID, true, 0)
		if err != nil {
			t.Fatalf("ListByConversation ASC failed: %v", err)
		}
		if len(msgs) != 3 {
			t.Fatalf("want 3 messages, got %d", len(msgs))
		}
		if msgs[0].Id != idT0 {
			t.Errorf("first (oldest): want %s, got %s", idT0, msgs[0].Id)
		}
		if msgs[1].Id != idT15 {
			t.Errorf("second: want %s, got %s", idT15, msgs[1].Id)
		}
		if msgs[2].Id != idT30 {
			t.Errorf("third (newest): want %s, got %s", idT30, msgs[2].Id)
		}
	})

	t.Run("DESC returns newest first", func(t *testing.T) {
		msgs, err := ListByConversation(ctx, s, convID, false, 0)
		if err != nil {
			t.Fatalf("ListByConversation DESC failed: %v", err)
		}
		if len(msgs) != 3 {
			t.Fatalf("want 3 messages, got %d", len(msgs))
		}
		if msgs[0].Id != idT30 {
			t.Errorf("first (newest): want %s, got %s", idT30, msgs[0].Id)
		}
		if msgs[2].Id != idT0 {
			t.Errorf("last (oldest): want %s, got %s", idT0, msgs[2].Id)
		}
	})
}

func TestChatMessageStorage_ListByConversation_RespectsLimit(t *testing.T) {
	s, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	convID := "conv-limit-test"

	for i := int64(1000); i <= 1005; i++ {
		insertTestChatMessage(t, s, convID, i)
	}

	msgs, err := ListByConversation(ctx, s, convID, true, 3)
	if err != nil {
		t.Fatalf("ListByConversation with limit=3 failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("want 3 messages (limit=3), got %d", len(msgs))
	}
}

func TestChatMessageStorage_ListByConversation_DefaultLimitApplied(t *testing.T) {
	s, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	convID := "conv-default-limit-test"

	// Insert a few messages; verify limit=0 and limit=-1 don't panic/error
	for i := int64(1); i <= 3; i++ {
		insertTestChatMessage(t, s, convID, i)
	}

	msgs0, err := ListByConversation(ctx, s, convID, true, 0)
	if err != nil {
		t.Fatalf("ListByConversation with limit=0 failed: %v", err)
	}
	if len(msgs0) != 3 {
		t.Errorf("limit=0: want 3 messages, got %d", len(msgs0))
	}

	msgsNeg, err := ListByConversation(ctx, s, convID, true, -1)
	if err != nil {
		t.Fatalf("ListByConversation with limit=-1 failed: %v", err)
	}
	if len(msgsNeg) != 3 {
		t.Errorf("limit=-1: want 3 messages, got %d", len(msgsNeg))
	}
}

func TestChatMessageStorage_ListByConversation_ExcludesSoftDeleted(t *testing.T) {
	s, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	convID := "conv-softdelete-test"

	// Insert two messages; soft-delete one
	idLive := insertTestChatMessage(t, s, convID, 1000)
	idDeleted := insertTestChatMessage(t, s, convID, 2000)

	// Soft-delete the second message via ProtoSQL Update
	deleted := &models.ChatMessage{
		Id:             idDeleted,
		ConversationId: convID,
		SentAtUnixSec:  2000,
		Deleted:        &models.DeletedMetadata{DeletedAtUnixSec: time.Now().Unix()},
		Message: &models.ChatMessage_UserMessage{
			UserMessage: &models.UserChatMessage{SenderId: "user1"},
		},
	}
	if err := s.Update(ctx, deleted); err != nil {
		t.Fatalf("soft-delete update failed: %v", err)
	}

	msgs, err := ListByConversation(ctx, s, convID, true, 0)
	if err != nil {
		t.Fatalf("ListByConversation failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 live message, got %d", len(msgs))
	}
	if msgs[0].Id != idLive {
		t.Errorf("want live message %s, got %s", idLive, msgs[0].Id)
	}
}

func TestChatMessageStorage_ListByConversation_IsolatesConversations(t *testing.T) {
	s, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	insertTestChatMessage(t, s, "conv-A", 1000)
	insertTestChatMessage(t, s, "conv-A", 2000)
	insertTestChatMessage(t, s, "conv-B", 3000)

	msgsA, err := ListByConversation(ctx, s, "conv-A", true, 0)
	if err != nil {
		t.Fatalf("ListByConversation conv-A failed: %v", err)
	}
	if len(msgsA) != 2 {
		t.Errorf("conv-A: want 2 messages, got %d", len(msgsA))
	}

	msgsB, err := ListByConversation(ctx, s, "conv-B", true, 0)
	if err != nil {
		t.Fatalf("ListByConversation conv-B failed: %v", err)
	}
	if len(msgsB) != 1 {
		t.Errorf("conv-B: want 1 message, got %d", len(msgsB))
	}
}
