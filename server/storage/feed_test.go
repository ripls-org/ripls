package storage

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestNewFeedStorage(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	feedStorage := NewFeedStorage(storage)

	if feedStorage == nil {
		t.Fatal("Expected feedStorage to be created")
	}

	if feedStorage.storage != storage {
		t.Error("Expected storage to be set correctly")
	}
}

func TestIncrementViewCount_CreatesNewRecord(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	feedStorage := NewFeedStorage(storage)
	ctx := context.Background()

	userID := "user1"
	communityID := "community1"
	itemID := "item1"
	itemType := "event"

	// Increment view count for first time
	err := feedStorage.IncrementViewCount(ctx, userID, communityID, itemID, itemType)
	if err != nil {
		t.Fatalf("IncrementViewCount failed: %v", err)
	}

	// Verify record was created
	views, err := feedStorage.GetFeedItemViews(ctx, userID, communityID, []string{itemID})
	if err != nil {
		t.Fatalf("GetFeedItemViews failed: %v", err)
	}

	if len(views) != 1 {
		t.Fatalf("Expected 1 view record, got %d", len(views))
	}

	view := views[itemID]
	if view == nil {
		t.Fatal("Expected view record for item1")
	}

	if view.UserId != userID {
		t.Errorf("Expected user ID %s, got %s", userID, view.UserId)
	}

	if view.CommunityId != communityID {
		t.Errorf("Expected community ID %s, got %s", communityID, view.CommunityId)
	}

	if view.FeedItemId != itemID {
		t.Errorf("Expected feed item ID %s, got %s", itemID, view.FeedItemId)
	}

	if view.ItemType != itemType {
		t.Errorf("Expected item type %s, got %s", itemType, view.ItemType)
	}

	if view.ViewCount != 1 {
		t.Errorf("Expected view count 1, got %d", view.ViewCount)
	}

	if view.FirstViewedAtUnixSec == 0 {
		t.Error("Expected first_viewed_at to be set")
	}

	if view.LastViewedAtUnixSec == 0 {
		t.Error("Expected last_viewed_at to be set")
	}
}

func TestIncrementViewCount_UpdatesExistingRecord(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	feedStorage := NewFeedStorage(storage)
	ctx := context.Background()

	userID := "user1"
	communityID := "community1"
	itemID := "item1"
	itemType := "event"

	// First increment
	err := feedStorage.IncrementViewCount(ctx, userID, communityID, itemID, itemType)
	if err != nil {
		t.Fatalf("First IncrementViewCount failed: %v", err)
	}

	// Get first_viewed_at timestamp
	views, err := feedStorage.GetFeedItemViews(ctx, userID, communityID, []string{itemID})
	if err != nil {
		t.Fatalf("GetFeedItemViews failed: %v", err)
	}
	firstViewedAt := views[itemID].FirstViewedAtUnixSec

	// Use a simulated future time for the second call so LastViewedAtUnixSec
	// is guaranteed to be strictly greater than FirstViewedAtUnixSec.
	laterCtx := clock.WithSimulationTime(ctx, time.Unix(firstViewedAt, 0).Add(2*time.Second))

	// Second increment
	err = feedStorage.IncrementViewCount(laterCtx, userID, communityID, itemID, itemType)
	if err != nil {
		t.Fatalf("Second IncrementViewCount failed: %v", err)
	}

	// Verify view count was incremented
	views, err = feedStorage.GetFeedItemViews(ctx, userID, communityID, []string{itemID})
	if err != nil {
		t.Fatalf("GetFeedItemViews failed: %v", err)
	}

	view := views[itemID]
	if view.ViewCount != 2 {
		t.Errorf("Expected view count 2, got %d", view.ViewCount)
	}

	// Verify first_viewed_at didn't change
	if view.FirstViewedAtUnixSec != firstViewedAt {
		t.Error("Expected first_viewed_at to remain unchanged")
	}

	// Verify last_viewed_at was updated
	if view.LastViewedAtUnixSec <= firstViewedAt {
		t.Error("Expected last_viewed_at to be updated")
	}
}

func TestGetFeedItemViews_EmptyItemIDs(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	feedStorage := NewFeedStorage(storage)
	ctx := context.Background()

	views, err := feedStorage.GetFeedItemViews(ctx, "user1", "community1", []string{})
	if err != nil {
		t.Fatalf("GetFeedItemViews failed: %v", err)
	}

	if len(views) != 0 {
		t.Errorf("Expected empty map, got %d views", len(views))
	}
}

func TestGetFeedItemViews_MultipleItems(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	feedStorage := NewFeedStorage(storage)
	ctx := context.Background()

	userID := "user1"
	communityID := "community1"

	// Create view records for multiple items
	items := []string{"item1", "item2", "item3"}
	for _, itemID := range items {
		err := feedStorage.IncrementViewCount(ctx, userID, communityID, itemID, "event")
		if err != nil {
			t.Fatalf("IncrementViewCount failed for %s: %v", itemID, err)
		}
	}

	// Get all views
	views, err := feedStorage.GetFeedItemViews(ctx, userID, communityID, items)
	if err != nil {
		t.Fatalf("GetFeedItemViews failed: %v", err)
	}

	if len(views) != 3 {
		t.Fatalf("Expected 3 views, got %d", len(views))
	}

	for _, itemID := range items {
		if views[itemID] == nil {
			t.Errorf("Expected view record for %s", itemID)
		}
	}
}

func TestGetViewCountsForItems(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	feedStorage := NewFeedStorage(storage)
	ctx := context.Background()

	userID := "user1"
	communityID := "community1"

	// Create items with different view counts
	err := feedStorage.IncrementViewCount(ctx, userID, communityID, "item1", "event")
	if err != nil {
		t.Fatalf("IncrementViewCount failed: %v", err)
	}

	err = feedStorage.IncrementViewCount(ctx, userID, communityID, "item2", "event")
	if err != nil {
		t.Fatalf("IncrementViewCount failed: %v", err)
	}
	err = feedStorage.IncrementViewCount(ctx, userID, communityID, "item2", "event")
	if err != nil {
		t.Fatalf("IncrementViewCount failed: %v", err)
	}

	// Get view counts
	viewCounts, err := feedStorage.GetViewCountsForItems(ctx, userID, communityID, []string{"item1", "item2", "item3"})
	if err != nil {
		t.Fatalf("GetViewCountsForItems failed: %v", err)
	}

	if viewCounts["item1"] != 1 {
		t.Errorf("Expected view count 1 for item1, got %d", viewCounts["item1"])
	}

	if viewCounts["item2"] != 2 {
		t.Errorf("Expected view count 2 for item2, got %d", viewCounts["item2"])
	}

	if viewCounts["item3"] != 0 {
		t.Errorf("Expected view count 0 for item3 (not viewed), got %d", viewCounts["item3"])
	}
}

func TestGetFeedItemViews_UserIsolation(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	feedStorage := NewFeedStorage(storage)
	ctx := context.Background()

	communityID := "community1"
	itemID := "item1"

	// User 1 views item
	err := feedStorage.IncrementViewCount(ctx, "user1", communityID, itemID, "event")
	if err != nil {
		t.Fatalf("IncrementViewCount failed: %v", err)
	}

	// User 2 views item
	err = feedStorage.IncrementViewCount(ctx, "user2", communityID, itemID, "event")
	if err != nil {
		t.Fatalf("IncrementViewCount failed: %v", err)
	}

	// Get views for user 1
	views1, err := feedStorage.GetFeedItemViews(ctx, "user1", communityID, []string{itemID})
	if err != nil {
		t.Fatalf("GetFeedItemViews failed: %v", err)
	}

	if views1[itemID].ViewCount != 1 {
		t.Errorf("Expected user1 view count 1, got %d", views1[itemID].ViewCount)
	}

	// Get views for user 2
	views2, err := feedStorage.GetFeedItemViews(ctx, "user2", communityID, []string{itemID})
	if err != nil {
		t.Fatalf("GetFeedItemViews failed: %v", err)
	}

	if views2[itemID].ViewCount != 1 {
		t.Errorf("Expected user2 view count 1, got %d", views2[itemID].ViewCount)
	}

	// Verify they are separate records
	if views1[itemID].Id == views2[itemID].Id {
		t.Error("Expected separate view records for different users")
	}
}

func TestGetFeedItemViews_CommunityIsolation(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	feedStorage := NewFeedStorage(storage)
	ctx := context.Background()

	userID := "user1"
	itemID := "item1"

	// View item in community 1
	err := feedStorage.IncrementViewCount(ctx, userID, "community1", itemID, "event")
	if err != nil {
		t.Fatalf("IncrementViewCount failed: %v", err)
	}

	// View item in community 2
	err = feedStorage.IncrementViewCount(ctx, userID, "community2", itemID, "event")
	if err != nil {
		t.Fatalf("IncrementViewCount failed: %v", err)
	}

	// Get views for community 1
	views1, err := feedStorage.GetFeedItemViews(ctx, userID, "community1", []string{itemID})
	if err != nil {
		t.Fatalf("GetFeedItemViews failed: %v", err)
	}

	if len(views1) != 1 {
		t.Fatalf("Expected 1 view for community1, got %d", len(views1))
	}

	// Get views for community 2
	views2, err := feedStorage.GetFeedItemViews(ctx, userID, "community2", []string{itemID})
	if err != nil {
		t.Fatalf("GetFeedItemViews failed: %v", err)
	}

	if len(views2) != 1 {
		t.Fatalf("Expected 1 view for community2, got %d", len(views2))
	}

	// Verify they are separate records
	if views1[itemID].Id == views2[itemID].Id {
		t.Error("Expected separate view records for different communities")
	}
}

func TestFeedItemView_Persistence(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	feedStorage := NewFeedStorage(storage)
	ctx := context.Background()

	// Create a view record
	userID := "user1"
	communityID := "community1"
	itemID := "item1"

	err := feedStorage.IncrementViewCount(ctx, userID, communityID, itemID, "event")
	if err != nil {
		t.Fatalf("IncrementViewCount failed: %v", err)
	}

	// Verify we can retrieve it using storage.QueryByField
	views, err := storage.QueryByField(ctx, "feed_item_id", itemID, &models.FeedItemView{})
	if err != nil {
		t.Fatalf("QueryByField failed: %v", err)
	}

	if len(views) != 1 {
		t.Fatalf("Expected 1 view record, got %d", len(views))
	}

	view := views[0].(*models.FeedItemView)
	if view.FeedItemId != itemID {
		t.Errorf("Expected feed item ID %s, got %s", itemID, view.FeedItemId)
	}
}
