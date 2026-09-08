package storage

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestRegisterArrayColumn_Panics covers the two cases where the registration
// helpers reject bad input at construction time (faster signal than waiting
// for a write to fail).
func TestRegisterArrayColumn_Panics(t *testing.T) {
	t.Run("TopLevelRepeatedString panics on missing field", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic for missing field, got none")
			}
		}()
		TopLevelRepeatedString(&models.Gear{}, "no_such_field")
	})

	t.Run("TopLevelRepeatedString panics on non-repeated field", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic for non-repeated field, got none")
			}
		}()
		TopLevelRepeatedString(&models.Gear{}, "name")
	})

	t.Run("RegisterArrayColumn panics on nil extractor", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic for nil extract, got none")
			}
		}()
		s := &ProtoSQLStorage{}
		s.RegisterArrayColumn(&models.Gear{}, "media_ids", nil)
	})
}

// TestMediaIdsAutoSync_InsertAndUpdate exercises the core invariant of the
// array-column denormalization: after Insert and Update, the indexed
// media_ids column reflects what's in the proto, and QueryByArrayContains
// returns the row by any of its ids.
func TestMediaIdsAutoSync_InsertAndUpdate(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	gear := &models.Gear{
		Name:     "drill",
		MediaIds: []string{"media-1", "media-2"},
	}
	id, err := storage.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	hits, err := storage.QueryByArrayContains(ctx, "media_ids", "media-1", &models.Gear{})
	if err != nil {
		t.Fatalf("QueryByArrayContains media-1: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("media-1 hits = %d, want 1", len(hits))
	}
	if gotID := hits[0].(*models.Gear).Id; gotID != id {
		t.Errorf("media-1 hit id = %q, want %q", gotID, id)
	}

	hits, err = storage.QueryByArrayContains(ctx, "media_ids", "media-2", &models.Gear{})
	if err != nil {
		t.Fatalf("QueryByArrayContains media-2: %v", err)
	}
	if len(hits) != 1 {
		t.Errorf("media-2 hits = %d, want 1", len(hits))
	}

	// Update replaces the media_ids slice — the index must reflect the
	// new state, including the removal of media-1 and media-2.
	gear.Id = id
	gear.MediaIds = []string{"media-3"}
	if err := storage.Update(ctx, gear); err != nil {
		t.Fatalf("Update: %v", err)
	}

	hits, err = storage.QueryByArrayContains(ctx, "media_ids", "media-1", &models.Gear{})
	if err != nil {
		t.Fatalf("QueryByArrayContains after update: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("after update, media-1 hits = %d, want 0 (removed from proto)", len(hits))
	}
	hits, err = storage.QueryByArrayContains(ctx, "media_ids", "media-3", &models.Gear{})
	if err != nil {
		t.Fatalf("QueryByArrayContains media-3: %v", err)
	}
	if len(hits) != 1 {
		t.Errorf("after update, media-3 hits = %d, want 1", len(hits))
	}
}

// TestMediaIdsAutoSync_ChatMessageNested covers the nested-field path:
// ChatMessage.media_ids lives inside the user_message oneof variant, so the
// auto-sync must walk into the variant rather than read a top-level field.
func TestMediaIdsAutoSync_ChatMessageNested(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	msg := &models.ChatMessage{
		ConversationId: "conv-1",
		Message: &models.ChatMessage_UserMessage{
			UserMessage: &models.UserChatMessage{
				SenderId: "user-1",
				Text:     "look",
				MediaIds: []string{"media-chat-1"},
			},
		},
	}
	if _, err := storage.Insert(ctx, msg); err != nil {
		t.Fatalf("Insert ChatMessage: %v", err)
	}

	hits, err := storage.QueryByArrayContains(ctx, "media_ids", "media-chat-1", &models.ChatMessage{})
	if err != nil {
		t.Fatalf("QueryByArrayContains: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("ChatMessage hits = %d, want 1", len(hits))
	}

	// System messages have no media; the extractor must return nil and
	// the row must not match a query for an arbitrary id.
	sysMsg := &models.ChatMessage{
		ConversationId: "conv-1",
		Message: &models.ChatMessage_SystemMessage{
			SystemMessage: &models.SystemChatMessage{
				Description: "joined",
			},
		},
	}
	if _, err := storage.Insert(ctx, sysMsg); err != nil {
		t.Fatalf("Insert system ChatMessage: %v", err)
	}
	hits, err = storage.QueryByArrayContains(ctx, "media_ids", "media-chat-1", &models.ChatMessage{})
	if err != nil {
		t.Fatalf("QueryByArrayContains after system insert: %v", err)
	}
	if len(hits) != 1 {
		t.Errorf("system-message insert leaked into media-chat-1 hits = %d, want 1", len(hits))
	}
}

// TestQueryByArrayContains_NoMatch is the negative case: an id that no row
// carries returns an empty slice (not an error).
func TestQueryByArrayContains_NoMatch(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	hits, err := storage.QueryByArrayContains(ctx, "media_ids", "no-such-media", &models.Gear{})
	if err != nil {
		t.Fatalf("QueryByArrayContains: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("hits = %d, want 0", len(hits))
	}
}

// TestQueryByArrayContains_SoftDeleted confirms the default WHERE filter
// excludes soft-deleted rows but [QueryOptions]{IncludeDeleted: true}
// retrieves them.
func TestQueryByArrayContains_SoftDeleted(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	gear := &models.Gear{
		Name:     "drill",
		MediaIds: []string{"media-sd-1"},
	}
	id, err := storage.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	gear.Id = id
	gear.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: 1}
	if err := storage.Update(ctx, gear); err != nil {
		t.Fatalf("Update with soft delete: %v", err)
	}

	hits, err := storage.QueryByArrayContains(ctx, "media_ids", "media-sd-1", &models.Gear{})
	if err != nil {
		t.Fatalf("QueryByArrayContains default: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("soft-deleted row returned by default query, hits=%d", len(hits))
	}

	hits, err = storage.QueryByArrayContains(ctx, "media_ids", "media-sd-1", &models.Gear{}, QueryOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("QueryByArrayContains include deleted: %v", err)
	}
	if len(hits) != 1 {
		t.Errorf("IncludeDeleted=true hits=%d, want 1", len(hits))
	}
}
