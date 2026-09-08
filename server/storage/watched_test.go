package storage

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestWatchStorage_UpsertAndGetActive(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ws := NewWatchStorage(storage)
	ctx := context.Background()

	// Upsert a watch.
	err := ws.UpsertWatch(ctx, "user1", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear1")
	if err != nil {
		t.Fatalf("UpsertWatch failed: %v", err)
	}

	// Get active watches — should contain 1.
	watches, err := ws.GetActiveWatches(ctx, "user1")
	if err != nil {
		t.Fatalf("GetActiveWatches failed: %v", err)
	}
	if len(watches) != 1 {
		t.Fatalf("expected 1 watch, got %d", len(watches))
	}
	if watches[0].ItemId != "gear1" {
		t.Errorf("expected item_id gear1, got %s", watches[0].ItemId)
	}
	if watches[0].IsRead {
		t.Error("expected new watch to be unread")
	}

	// Upsert same watch again — should be idempotent.
	err = ws.UpsertWatch(ctx, "user1", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear1")
	if err != nil {
		t.Fatalf("second UpsertWatch failed: %v", err)
	}
	watches, err = ws.GetActiveWatches(ctx, "user1")
	if err != nil {
		t.Fatalf("GetActiveWatches failed: %v", err)
	}
	if len(watches) != 1 {
		t.Fatalf("expected 1 watch after idempotent upsert, got %d", len(watches))
	}
}

func TestWatchStorage_MarkRead(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ws := NewWatchStorage(storage)
	ctx := context.Background()

	err := ws.UpsertWatch(ctx, "user1", models.WatchedItemType_WATCHED_ITEM_TYPE_EXPERIENCE, "exp1")
	if err != nil {
		t.Fatalf("UpsertWatch failed: %v", err)
	}

	// Mark read.
	err = ws.MarkRead(ctx, "user1", models.WatchedItemType_WATCHED_ITEM_TYPE_EXPERIENCE, "exp1")
	if err != nil {
		t.Fatalf("MarkRead failed: %v", err)
	}

	// Verify it is read.
	isUnread, err := ws.IsWatchedUnread(ctx, "user1", models.WatchedItemType_WATCHED_ITEM_TYPE_EXPERIENCE, "exp1")
	if err != nil {
		t.Fatalf("IsWatchedUnread failed: %v", err)
	}
	if isUnread {
		t.Error("expected watch to be read after MarkRead")
	}

	// Unread count should be 0.
	count, err := ws.GetUnreadCount(ctx, "user1")
	if err != nil {
		t.Fatalf("GetUnreadCount failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected unread count 0, got %d", count)
	}
}

func TestWatchStorage_MarkUnreadForWatchers(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ws := NewWatchStorage(storage)
	ctx := context.Background()

	// Two users watching same gear.
	err := ws.UpsertWatch(ctx, "owner", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear1")
	if err != nil {
		t.Fatalf("UpsertWatch owner failed: %v", err)
	}
	err = ws.UpsertWatch(ctx, "borrower", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear1")
	if err != nil {
		t.Fatalf("UpsertWatch borrower failed: %v", err)
	}

	// Mark both as read.
	_ = ws.MarkRead(ctx, "owner", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear1")
	_ = ws.MarkRead(ctx, "borrower", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear1")

	// Borrower sends message → mark all watchers unread except borrower.
	err = ws.MarkUnreadForWatchers(ctx, models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear1", "borrower")
	if err != nil {
		t.Fatalf("MarkUnreadForWatchers failed: %v", err)
	}

	// Owner should be unread.
	ownerUnread, err := ws.IsWatchedUnread(ctx, "owner", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear1")
	if err != nil {
		t.Fatalf("IsWatchedUnread owner failed: %v", err)
	}
	if !ownerUnread {
		t.Error("expected owner to be unread after MarkUnreadForWatchers")
	}

	// Borrower should still be read.
	borrowerUnread, err := ws.IsWatchedUnread(ctx, "borrower", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear1")
	if err != nil {
		t.Fatalf("IsWatchedUnread borrower failed: %v", err)
	}
	if borrowerUnread {
		t.Error("expected borrower to remain read (they were the except user)")
	}
}

func TestWatchStorage_DismissAndReactivate(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ws := NewWatchStorage(storage)
	ctx := context.Background()

	err := ws.UpsertWatch(ctx, "user1", models.WatchedItemType_WATCHED_ITEM_TYPE_REQUEST, "req1")
	if err != nil {
		t.Fatalf("UpsertWatch failed: %v", err)
	}

	// Dismiss the watch.
	err = ws.DismissWatch(ctx, "user1", models.WatchedItemType_WATCHED_ITEM_TYPE_REQUEST, "req1")
	if err != nil {
		t.Fatalf("DismissWatch failed: %v", err)
	}

	// Active watches should be empty.
	watches, err := ws.GetActiveWatches(ctx, "user1")
	if err != nil {
		t.Fatalf("GetActiveWatches failed: %v", err)
	}
	if len(watches) != 0 {
		t.Fatalf("expected 0 active watches after dismiss, got %d", len(watches))
	}

	// Upsert again should reactivate.
	err = ws.UpsertWatch(ctx, "user1", models.WatchedItemType_WATCHED_ITEM_TYPE_REQUEST, "req1")
	if err != nil {
		t.Fatalf("UpsertWatch reactivate failed: %v", err)
	}

	watches, err = ws.GetActiveWatches(ctx, "user1")
	if err != nil {
		t.Fatalf("GetActiveWatches after reactivate failed: %v", err)
	}
	if len(watches) != 1 {
		t.Fatalf("expected 1 active watch after reactivate, got %d", len(watches))
	}
	if watches[0].IsRead {
		t.Error("expected reactivated watch to be unread")
	}
}

func TestWatchStorage_DismissAllForItem(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ws := NewWatchStorage(storage)
	ctx := context.Background()

	// Two users watching same item.
	_ = ws.UpsertWatch(ctx, "user1", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear1")
	_ = ws.UpsertWatch(ctx, "user2", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear1")

	// Dismiss all.
	err := ws.DismissAllForItem(ctx, models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear1")
	if err != nil {
		t.Fatalf("DismissAllForItem failed: %v", err)
	}

	// Both users should have 0 active watches.
	w1, _ := ws.GetActiveWatches(ctx, "user1")
	w2, _ := ws.GetActiveWatches(ctx, "user2")
	if len(w1) != 0 {
		t.Errorf("expected 0 active watches for user1, got %d", len(w1))
	}
	if len(w2) != 0 {
		t.Errorf("expected 0 active watches for user2, got %d", len(w2))
	}
}

func TestWatchStorage_GetUnreadCount(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ws := NewWatchStorage(storage)
	ctx := context.Background()

	// Create 3 watches, mark 1 as read.
	_ = ws.UpsertWatch(ctx, "user1", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear1")
	_ = ws.UpsertWatch(ctx, "user1", models.WatchedItemType_WATCHED_ITEM_TYPE_EXPERIENCE, "exp1")
	_ = ws.UpsertWatch(ctx, "user1", models.WatchedItemType_WATCHED_ITEM_TYPE_REQUEST, "req1")
	_ = ws.MarkRead(ctx, "user1", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear1")

	count, err := ws.GetUnreadCount(ctx, "user1")
	if err != nil {
		t.Fatalf("GetUnreadCount failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected unread count 2, got %d", count)
	}
}

func TestWatchStorage_GetAllWatches(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ws := NewWatchStorage(storage)
	ctx := context.Background()

	// Create two watches, dismiss one.
	_ = ws.UpsertWatch(ctx, "user1", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear1")
	_ = ws.UpsertWatch(ctx, "user1", models.WatchedItemType_WATCHED_ITEM_TYPE_EXPERIENCE, "exp1")
	_ = ws.DismissWatch(ctx, "user1", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "gear1")

	// GetActiveWatches should return only the non-dismissed one.
	active, err := ws.GetActiveWatches(ctx, "user1")
	if err != nil {
		t.Fatalf("GetActiveWatches failed: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active watch, got %d", len(active))
	}

	// GetAllWatches should return both (active + dismissed).
	all, err := ws.GetAllWatches(ctx, "user1")
	if err != nil {
		t.Fatalf("GetAllWatches failed: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 total watches, got %d", len(all))
	}

	// Verify the dismissed one is present with dismissed_at set.
	var dismissedFound bool
	for _, w := range all {
		if w.ItemId == "gear1" {
			if w.DismissedAtUnixSec == nil {
				t.Error("expected gear1 watch to have dismissed_at set")
			}
			dismissedFound = true
		}
	}
	if !dismissedFound {
		t.Error("dismissed watch for gear1 not found in GetAllWatches result")
	}
}

func TestWatchStorage_MarkReadNonexistent(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ws := NewWatchStorage(storage)
	ctx := context.Background()

	// MarkRead on nonexistent watch should be a no-op.
	err := ws.MarkRead(ctx, "user1", models.WatchedItemType_WATCHED_ITEM_TYPE_GEAR, "nonexistent")
	if err != nil {
		t.Fatalf("MarkRead on nonexistent should not error, got: %v", err)
	}
}
