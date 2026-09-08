package feed

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestNew(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	if service == nil {
		t.Fatal("Expected service to be created")
	}

	if service.sqlStorage != sqlStorage {
		t.Error("Expected sqlStorage to be set correctly")
	}

	if service.feedStorage == nil {
		t.Error("Expected feedStorage to be initialized")
	}
}

func TestGetFeed_Success(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Setup test data
	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")
	gearID := setupTestGear(t, sqlStorage, userID, "Test Gear")
	setupCommunityGear(t, sqlStorage, communityID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)
	eventID := createGearSharedEvent(t, sqlStorage, communityID, userID, gearID)

	// Create authenticated context
	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	// Call GetFeed
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})
	resp, err := service.GetFeed(ctx, req)
	// Assertions
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 feed item, got %d", len(resp.Msg.Items))
	}

	item := resp.Msg.Items[0]
	if item.Id != eventID {
		t.Errorf("Expected item ID %s, got %s", eventID, item.Id)
	}

	if item.ItemType != api.FeedItemType_FEED_ITEM_TYPE_GEAR_SHARED {
		t.Errorf("Expected GEAR_SHARED type, got %v", item.ItemType)
	}

	if !item.IsUnread {
		t.Error("Expected item to be unread (never seen)")
	}

	if item.GetGearShared() == nil {
		t.Fatal("Expected gear shared payload")
	}

	if item.GetGearShared().GearId != gearID {
		t.Errorf("Expected gear ID %s, got %s", gearID, item.GetGearShared().GearId)
	}
}

// TestGetFeed_OnceViewedStillFresh checks an item viewed once (below threshold) still appears.
func TestGetFeed_OnceViewedStillFresh(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")
	gearID := setupTestGear(t, sqlStorage, userID, "Test Gear")
	setupCommunityGear(t, sqlStorage, communityID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)
	eventID := createGearSharedEvent(t, sqlStorage, communityID, userID, gearID)

	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	// View the item once (default expiry is 2 views, so still fresh)
	feedStorage := storage.NewFeedStorage(sqlStorage)
	if err := feedStorage.IncrementViewCount(context.Background(), userID, communityID, eventID, "event"); err != nil {
		t.Fatalf("Failed to increment view count: %v", err)
	}

	req := connect.NewRequest(&api.GetFeedRequest{CommunityIds: []string{communityID}, PageSize: 20})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	if len(resp.Msg.Items) != 1 {
		t.Errorf("Expected 1 item (once-viewed is still within threshold), got %d", len(resp.Msg.Items))
	}
	if len(resp.Msg.Items) > 0 && resp.Msg.Items[0].IsUnread {
		t.Error("Expected item to be read after viewing (no new activity)")
	}
}

func TestGetFeed_SortsByTimestamp(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")

	// Create events with different timestamps
	gear1ID := setupTestGear(t, sqlStorage, userID, "Gear 1")
	gear2ID := setupTestGear(t, sqlStorage, userID, "Gear 2")

	// Share both gear items with the community
	setupCommunityGear(t, sqlStorage, communityID, gear1ID, models.Availability_AVAILABILITY_FOR_LOAN)
	setupCommunityGear(t, sqlStorage, communityID, gear2ID, models.Availability_AVAILABILITY_FOR_LOAN)

	// Create older event first
	event1 := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            gear1ID,
		OccurredAtUnixSec: time.Now().Unix() - 100,
	}
	_, err := sqlStorage.Insert(context.Background(), event1)
	if err != nil {
		t.Fatalf("Failed to create event 1: %v", err)
	}

	// Create newer event
	event2 := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           userID,
		GearId:            gear2ID,
		OccurredAtUnixSec: time.Now().Unix(),
	}
	_, err = sqlStorage.Insert(context.Background(), event2)
	if err != nil {
		t.Fatalf("Failed to create event 2: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	if len(resp.Msg.Items) != 2 {
		t.Fatalf("Expected 2 feed items, got %d", len(resp.Msg.Items))
	}

	// Verify newest item is first
	if resp.Msg.Items[0].GetGearShared().GearId != gear2ID {
		t.Error("Expected newest event first (descending order)")
	}

	if resp.Msg.Items[1].GetGearShared().GearId != gear1ID {
		t.Error("Expected older event second")
	}
}

func TestGetFeed_RequiresAuthentication(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Call without authentication
	ctx := context.Background()
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{"test-community"},
	})

	_, err := service.GetFeed(ctx, req)

	if err == nil {
		t.Fatal("Expected authentication error")
	}

	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("Expected Unauthenticated error, got %v", connect.CodeOf(err))
	}
}

func TestGetFeed_EmptyCommunityID(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GetFeedRequest{})

	_, err := service.GetFeed(ctx, req)

	if err == nil {
		t.Fatal("Expected error for empty community_id")
	}

	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("Expected InvalidArgument error, got %v", connect.CodeOf(err))
	}
}

func TestGetFeed_RequiresMembership(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Create two users
	user1ID := setupTestUser(t, sqlStorage, "user1@test.com", "User 1")
	user2ID := setupTestUser(t, sqlStorage, "user2@test.com", "User 2")

	// User 1 creates a community
	communityID := setupTestCommunity(t, sqlStorage, user1ID, "Test Community")

	// User 2 (not a member) tries to access the feed
	ctx := createAuthenticatedContext(user2ID, "user2@test.com", models.Role_ROLE_USER)
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
	})

	_, err := service.GetFeed(ctx, req)

	if err == nil {
		t.Fatal("Expected permission denied error")
	}

	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("Expected PermissionDenied error, got %v", connect.CodeOf(err))
	}
}

func TestMarkFeedItemsViewed_Success(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")
	gearID := setupTestGear(t, sqlStorage, userID, "Test Gear")
	eventID := createGearSharedEvent(t, sqlStorage, communityID, userID, gearID)

	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	// Mark item as viewed
	req := connect.NewRequest(&api.MarkFeedItemsViewedRequest{
		CommunityId: communityID,
		ItemIds:     []string{eventID},
	})

	_, err := service.MarkFeedItemsViewed(ctx, req)
	if err != nil {
		t.Fatalf("MarkFeedItemsViewed failed: %v", err)
	}

	// Verify view count was incremented
	feedStorage := storage.NewFeedStorage(sqlStorage)
	viewCounts, err := feedStorage.GetViewCountsForItems(context.Background(), userID, communityID, []string{eventID})
	if err != nil {
		t.Fatalf("Failed to get view counts: %v", err)
	}

	if viewCounts[eventID] != 1 {
		t.Errorf("Expected view count 1, got %d", viewCounts[eventID])
	}

	// Mark again
	_, err = service.MarkFeedItemsViewed(ctx, req)
	if err != nil {
		t.Fatalf("MarkFeedItemsViewed failed on second call: %v", err)
	}

	// Verify view count is now 2
	viewCounts, err = feedStorage.GetViewCountsForItems(context.Background(), userID, communityID, []string{eventID})
	if err != nil {
		t.Fatalf("Failed to get view counts: %v", err)
	}

	if viewCounts[eventID] != 2 {
		t.Errorf("Expected view count 2, got %d", viewCounts[eventID])
	}
}

func TestMarkFeedItemsViewed_RequiresAuthentication(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	ctx := context.Background()
	req := connect.NewRequest(&api.MarkFeedItemsViewedRequest{
		CommunityId: "test-community",
		ItemIds:     []string{"item1"},
	})

	_, err := service.MarkFeedItemsViewed(ctx, req)

	if err == nil {
		t.Fatal("Expected authentication error")
	}

	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("Expected Unauthenticated error, got %v", connect.CodeOf(err))
	}
}

func TestGetFeed_PageSize(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")

	// Create 5 events with shared gear
	for i := 0; i < 5; i++ {
		gearID := setupTestGear(t, sqlStorage, userID, "Gear")
		setupCommunityGear(t, sqlStorage, communityID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)
		createGearSharedEvent(t, sqlStorage, communityID, userID, gearID)
	}

	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	// Test default page size
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
	})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	if len(resp.Msg.Items) != 5 {
		t.Errorf("Expected 5 items with default page size, got %d", len(resp.Msg.Items))
	}

	// Test explicit page size
	req = connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     3,
	})
	resp, err = service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	if len(resp.Msg.Items) > 3 {
		t.Errorf("Expected at most 3 items, got %d", len(resp.Msg.Items))
	}
}
