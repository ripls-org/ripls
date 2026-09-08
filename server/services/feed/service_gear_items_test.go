package feed

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestGetFeed_GearAvailability(t *testing.T) {
	tests := []struct {
		name         string
		availability models.Availability
		wantType     api.Availability
	}{
		{
			name:         "gear available for loan",
			availability: models.Availability_AVAILABILITY_FOR_LOAN,
			wantType:     api.Availability_AVAILABILITY_FOR_LOAN,
		},
		{
			name:         "gear available for giveaway",
			availability: models.Availability_AVAILABILITY_FOR_GIVEAWAY,
			wantType:     api.Availability_AVAILABILITY_FOR_GIVEAWAY,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlStorage := setupTestStorage(t)
			service := setupTestService(sqlStorage)

			// Setup test data
			userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
			communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")
			gearID := setupTestGear(t, sqlStorage, userID, "Test Gear")

			// Create CommunityGear with specific availability
			communityGear := &models.CommunityGear{
				CommunityId:  communityID,
				GearId:       gearID,
				Availability: tt.availability,
			}
			_, err := sqlStorage.Insert(context.Background(), communityGear)
			if err != nil {
				t.Fatalf("Failed to create community gear: %v", err)
			}

			// Create gear shared event
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

			gearShared := item.GetGearShared()
			if gearShared == nil {
				t.Fatal("Expected gear shared payload")
			}

			if gearShared.Availability != tt.wantType {
				t.Errorf("Expected availability %v, got %v", tt.wantType, gearShared.Availability)
			}
		})
	}
}

func TestGetFeed_UnsharedGearExcluded(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Setup test data
	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")
	gearID := setupTestGear(t, sqlStorage, userID, "Test Gear")

	// Share gear with community
	communityGear := &models.CommunityGear{
		CommunityId:      communityID,
		GearId:           gearID,
		Availability:     models.Availability_AVAILABILITY_FOR_LOAN,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	communityGearID, err := sqlStorage.Insert(context.Background(), communityGear)
	if err != nil {
		t.Fatalf("Failed to create community gear: %v", err)
	}

	// Create gear shared event
	createGearSharedEvent(t, sqlStorage, communityID, userID, gearID)

	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	// Verify gear appears in feed when shared
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 feed item when gear is shared, got %d", len(resp.Msg.Items))
	}

	// Now unshare the gear by deleting the CommunityGear record
	communityGear.Id = communityGearID
	err = sqlStorage.Delete(context.Background(), communityGear)
	if err != nil {
		t.Fatalf("Failed to delete community gear: %v", err)
	}

	// Verify gear no longer appears in feed after unsharing
	resp, err = service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	if len(resp.Msg.Items) != 0 {
		t.Errorf("Expected 0 feed items after gear is unshared, got %d", len(resp.Msg.Items))
	}
}

func TestGetFeed_UnsharedGearExcluded_NeverShared(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Setup test data without creating CommunityGear
	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")
	gearID := setupTestGear(t, sqlStorage, userID, "Test Gear")

	// Create gear shared event without CommunityGear entry (orphaned event)
	createGearSharedEvent(t, sqlStorage, communityID, userID, gearID)

	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	// Gear without CommunityGear record should not appear in feed
	if len(resp.Msg.Items) != 0 {
		t.Errorf("Expected 0 feed items for gear without CommunityGear record, got %d", len(resp.Msg.Items))
	}
}

// TestGetFeed_NonExistentGearSkipped verifies that events referencing non-existent gear
// are gracefully skipped (returns nil feed item, not an error) since the feed should
// degrade gracefully for orphaned events.
func TestGetFeed_NonExistentGearSkipped(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Setup test data
	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")

	// Create a gear shared event referencing a non-existent gear ID
	fakeGearID := "00000000-0000-0000-0000-000000000000"
	createGearSharedEvent(t, sqlStorage, communityID, userID, fakeGearID)

	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	// Get feed - should NOT return an error (graceful degradation)
	// The feed service logs an error but returns nil for the feed item
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed should not return error for non-existent gear (graceful degradation): %v", err)
	}

	// Should not contain any gear shared items (the orphaned event is skipped)
	for _, item := range resp.Msg.Items {
		if item.ItemType == api.FeedItemType_FEED_ITEM_TYPE_GEAR_SHARED {
			t.Error("Non-existent gear should not appear in feed")
		}
	}
}

// TestGetFeed_ArchivedGearExcluded verifies that archived gear (completed/cancelled giveaways)
// is excluded from the feed.
func TestGetFeed_ArchivedGearExcluded(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Setup test data
	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")
	gearID := setupTestGear(t, sqlStorage, userID, "Test Gear")

	// Share gear with community
	communityGear := &models.CommunityGear{
		CommunityId:      communityID,
		GearId:           gearID,
		Availability:     models.Availability_AVAILABILITY_FOR_GIVEAWAY,
		Archived:         true, // Mark as archived (giveaway completed/cancelled)
		CreatedAtUnixSec: time.Now().Unix(),
	}
	_, err := sqlStorage.Insert(context.Background(), communityGear)
	if err != nil {
		t.Fatalf("Failed to create community gear: %v", err)
	}

	// Create gear shared event
	createGearSharedEvent(t, sqlStorage, communityID, userID, gearID)

	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	// Get feed - archived gear should not appear
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	// Archived gear should not appear in feed
	if len(resp.Msg.Items) != 0 {
		t.Errorf("Expected 0 feed items for archived gear, got %d", len(resp.Msg.Items))
	}
}

// TestGetFeed_NonArchivedGearIncluded verifies that non-archived gear appears in the feed.
func TestGetFeed_NonArchivedGearIncluded(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Setup test data
	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")
	gearID := setupTestGear(t, sqlStorage, userID, "Test Gear")

	// Share gear with community (not archived)
	communityGear := &models.CommunityGear{
		CommunityId:      communityID,
		GearId:           gearID,
		Availability:     models.Availability_AVAILABILITY_FOR_GIVEAWAY,
		Archived:         false, // Not archived - should appear in feed
		CreatedAtUnixSec: time.Now().Unix(),
	}
	_, err := sqlStorage.Insert(context.Background(), communityGear)
	if err != nil {
		t.Fatalf("Failed to create community gear: %v", err)
	}

	// Create gear shared event
	eventID := createGearSharedEvent(t, sqlStorage, communityID, userID, gearID)

	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	// Get feed - non-archived gear should appear
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed failed: %v", err)
	}

	// Non-archived gear should appear in feed
	if len(resp.Msg.Items) != 1 {
		t.Fatalf("Expected 1 feed item for non-archived gear, got %d", len(resp.Msg.Items))
	}

	item := resp.Msg.Items[0]
	if item.Id != eventID {
		t.Errorf("Expected item ID %s, got %s", eventID, item.Id)
	}

	if item.ItemType != api.FeedItemType_FEED_ITEM_TYPE_GEAR_SHARED {
		t.Errorf("Expected GEAR_SHARED type, got %v", item.ItemType)
	}

	gearShared := item.GetGearShared()
	if gearShared == nil {
		t.Fatal("Expected gear shared payload")
	}

	if gearShared.GearId != gearID {
		t.Errorf("Expected gear ID %s, got %s", gearID, gearShared.GearId)
	}
}

// TestGetFeed_SoftDeletedGearExcluded verifies that soft-deleted gear is gracefully
// skipped in the feed without returning an error.
func TestGetFeed_SoftDeletedGearExcluded(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Setup test data
	userID := setupTestUser(t, sqlStorage, "user@test.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")

	// Create two gear items
	activeGearID := setupTestGear(t, sqlStorage, userID, "Active Gear")
	deletedGearID := setupTestGear(t, sqlStorage, userID, "Deleted Gear")

	// Create CommunityGear records for both
	communityGear1 := &models.CommunityGear{
		CommunityId:  communityID,
		GearId:       activeGearID,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	}
	_, err := sqlStorage.Insert(context.Background(), communityGear1)
	if err != nil {
		t.Fatalf("Failed to create community gear 1: %v", err)
	}

	communityGear2 := &models.CommunityGear{
		CommunityId:  communityID,
		GearId:       deletedGearID,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	}
	_, err = sqlStorage.Insert(context.Background(), communityGear2)
	if err != nil {
		t.Fatalf("Failed to create community gear 2: %v", err)
	}

	// Create gear shared events for both
	createGearSharedEvent(t, sqlStorage, communityID, userID, activeGearID)
	createGearSharedEvent(t, sqlStorage, communityID, userID, deletedGearID)

	// Soft-delete the second gear
	deletedGear := &models.Gear{}
	err = sqlStorage.GetByID(context.Background(), deletedGearID, deletedGear)
	if err != nil {
		t.Fatalf("Failed to get gear: %v", err)
	}
	deletedGear.Deleted = &models.DeletedMetadata{
		DeletedByUserId:  userID,
		DeletedAtUnixSec: time.Now().Unix(),
	}
	err = sqlStorage.Update(context.Background(), deletedGear)
	if err != nil {
		t.Fatalf("Failed to soft-delete gear: %v", err)
	}

	ctx := createAuthenticatedContext(userID, "user@test.com", models.Role_ROLE_USER)

	// Get feed - should NOT return an error
	req := connect.NewRequest(&api.GetFeedRequest{
		CommunityIds: []string{communityID},
		PageSize:     20,
	})
	resp, err := service.GetFeed(ctx, req)
	if err != nil {
		t.Fatalf("GetFeed should not return error for soft-deleted gear: %v", err)
	}

	// Should only contain the active gear, not the deleted one
	gearSharedCount := 0
	for _, item := range resp.Msg.Items {
		if item.ItemType == api.FeedItemType_FEED_ITEM_TYPE_GEAR_SHARED {
			gearSharedCount++
			gearShared := item.GetGearShared()
			if gearShared.GearName == "Deleted Gear" {
				t.Error("Soft-deleted gear should not appear in feed")
			}
		}
	}

	if gearSharedCount != 1 {
		t.Errorf("Expected 1 gear shared item in feed, got %d", gearSharedCount)
	}
}
