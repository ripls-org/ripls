package community

import (
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestService_ListCommunityGear(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	ownerID := setupTestUser(t, testStorage, "owner@example.com", "Owner")
	memberID := setupTestUser(t, testStorage, "member@example.com", "Member")
	nonMemberID := setupTestUser(t, testStorage, "nonmember@example.com", "Non-Member")

	ctxOwner := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	ctxMember := createAuthenticatedContext(memberID, "member@example.com", models.Role_ROLE_USER)
	ctxNonMember := createAuthenticatedContext(nonMemberID, "nonmember@example.com", models.Role_ROLE_USER)

	// Create community
	createCommunityReq := connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Test Community",
	})
	createCommunityResp, err := service.CreateCommunity(ctxOwner, createCommunityReq)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}
	communityID := createCommunityResp.Msg.Id

	// Add member
	addUserToCommunity(t, service, communityID, ownerID, memberID, "owner@example.com", "member@example.com")

	// Create location for gear1
	location1 := &models.Location{
		Geolocation: &models.Geolocation{
			LatitudeDeg:  37.7749,
			LongitudeDeg: -122.4194,
		},
		Address: &models.Address{
			RegionCode:   "US",
			PostalCode:   "94102",
			Locality:     "San Francisco",
			AddressLines: []string{"123 Main St"},
		},
		CreatedAtUnixSec: time.Now().Unix(),
		UpdatedAtUnixSec: time.Now().Unix(),
	}
	location1ID, err := testStorage.Insert(ctxOwner, location1)
	if err != nil {
		t.Fatalf("Failed to create location1: %v", err)
	}

	// Create and share multiple gear items
	gear1 := &models.Gear{
		Name:        "Tent",
		Description: "4-person tent",
		OwnerId:     ownerID,
		State:       models.GearState_GEAR_STATE_AVAILABLE,
		MediaIds:    []string{"media1", "media2"},
		LocationId:  location1ID,
	}
	gear1ID, err := testStorage.Insert(ctxOwner, gear1)
	if err != nil {
		t.Fatalf("Failed to create gear1: %v", err)
	}

	gear2 := &models.Gear{
		Name:    "Sleeping Bag",
		OwnerId: ownerID,
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	}
	gear2ID, err := testStorage.Insert(ctxOwner, gear2)
	if err != nil {
		t.Fatalf("Failed to create gear2: %v", err)
	}

	// Share both gear items
	shareGearForTestWithAvailability(
		t, service, ctxOwner, gear1ID, communityID, models.Availability_AVAILABILITY_FOR_LOAN,
	)
	shareGearForTestWithAvailability(
		t, service, ctxOwner, gear2ID, communityID, models.Availability_AVAILABILITY_FOR_LOAN,
	)

	t.Run("member can list community gear", func(t *testing.T) {
		listReq := connect.NewRequest(&api.ListCommunityGearRequest{
			CommunityId: communityID,
		})

		listResp, err := service.ListCommunityGear(ctxMember, listReq)
		if err != nil {
			t.Fatalf("ListCommunityGear failed: %v", err)
		}

		if len(listResp.Msg.GearItems) != 2 {
			t.Fatalf("Expected 2 gear items, got %d", len(listResp.Msg.GearItems))
		}

		// Check first gear item details
		var tent *api.CommunityGearItem
		for _, item := range listResp.Msg.GearItems {
			if item.Name == "Tent" {
				tent = item
				break
			}
		}

		if tent == nil {
			t.Fatal("Expected to find Tent in gear items")
		}

		if tent.Description != "4-person tent" {
			t.Errorf("Expected description '4-person tent', got %s", tent.Description)
		}

		if tent.Owner == nil {
			t.Fatal("Expected owner to be set")
		}
		if tent.Owner.Id != ownerID {
			t.Errorf("Expected owner ID %s, got %s", ownerID, tent.Owner.Id)
		}

		if len(tent.MediaIds) != 2 {
			t.Errorf("Expected 2 media IDs, got %d", len(tent.MediaIds))
		}

		// Verify location fields
		if tent.LocationId != location1ID {
			t.Errorf("Expected location ID %s, got %s", location1ID, tent.LocationId)
		}

		if tent.LatitudeDeg != 37.7749 {
			t.Errorf("Expected latitude 37.7749, got %f", tent.LatitudeDeg)
		}

		if tent.LongitudeDeg != -122.4194 {
			t.Errorf("Expected longitude -122.4194, got %f", tent.LongitudeDeg)
		}

		// Verify gear without location has empty location fields
		var sleepingBag *api.CommunityGearItem
		for _, item := range listResp.Msg.GearItems {
			if item.Name == "Sleeping Bag" {
				sleepingBag = item
				break
			}
		}

		if sleepingBag == nil {
			t.Fatal("Expected to find Sleeping Bag in gear items")
		}

		if sleepingBag.LocationId != "" {
			t.Errorf("Expected empty location ID for gear without location, got %s", sleepingBag.LocationId)
		}

		if sleepingBag.LatitudeDeg != 0 {
			t.Errorf("Expected latitude 0 for gear without location, got %f", sleepingBag.LatitudeDeg)
		}

		if sleepingBag.LongitudeDeg != 0 {
			t.Errorf("Expected longitude 0 for gear without location, got %f", sleepingBag.LongitudeDeg)
		}

		// Verify availability is included in the response (both were shared as FOR_LOAN)
		if tent.Availability != api.Availability_AVAILABILITY_FOR_LOAN {
			t.Errorf("Expected tent availability FOR_LOAN, got %v", tent.Availability)
		}

		if sleepingBag.Availability != api.Availability_AVAILABILITY_FOR_LOAN {
			t.Errorf("Expected sleeping bag availability FOR_LOAN, got %v", sleepingBag.Availability)
		}
	})

	t.Run("list returns different availability types", func(t *testing.T) {
		// Update gear2 to be FOR_GIVEAWAY
		shareGearForTestWithAvailability(
			t, service, ctxOwner, gear2ID, communityID, models.Availability_AVAILABILITY_FOR_GIVEAWAY,
		)

		// List gear again
		listReq := connect.NewRequest(&api.ListCommunityGearRequest{
			CommunityId: communityID,
		})
		listResp, err := service.ListCommunityGear(ctxMember, listReq)
		if err != nil {
			t.Fatalf("ListCommunityGear failed: %v", err)
		}

		// Find both items
		var tent, sleepingBag *api.CommunityGearItem
		for _, item := range listResp.Msg.GearItems {
			if item.Name == "Tent" {
				tent = item
			} else if item.Name == "Sleeping Bag" {
				sleepingBag = item
			}
		}

		if tent == nil || sleepingBag == nil {
			t.Fatal("Expected to find both gear items")
		}

		// Verify tent is still FOR_LOAN
		if tent.Availability != api.Availability_AVAILABILITY_FOR_LOAN {
			t.Errorf("Expected tent availability FOR_LOAN, got %v", tent.Availability)
		}

		// Verify sleeping bag is now FOR_GIVEAWAY
		if sleepingBag.Availability != api.Availability_AVAILABILITY_FOR_GIVEAWAY {
			t.Errorf("Expected sleeping bag availability FOR_GIVEAWAY, got %v", sleepingBag.Availability)
		}
	})

	t.Run("non-member cannot list community gear", func(t *testing.T) {
		listReq := connect.NewRequest(&api.ListCommunityGearRequest{
			CommunityId: communityID,
		})

		_, err := service.ListCommunityGear(ctxNonMember, listReq)
		if err == nil {
			t.Fatal("Expected error when non-member tries to list gear")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok || connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", err)
		}
	})

	t.Run("empty list when no gear shared", func(t *testing.T) {
		// Create new community with no gear
		createReq := connect.NewRequest(&api.CreateCommunityRequest{
			Name: "Empty Community",
		})
		createResp, err := service.CreateCommunity(ctxOwner, createReq)
		if err != nil {
			t.Fatalf("Failed to create empty community: %v", err)
		}

		listReq := connect.NewRequest(&api.ListCommunityGearRequest{
			CommunityId: createResp.Msg.Id,
		})

		listResp, err := service.ListCommunityGear(ctxOwner, listReq)
		if err != nil {
			t.Fatalf("ListCommunityGear failed: %v", err)
		}

		if len(listResp.Msg.GearItems) != 0 {
			t.Errorf("Expected 0 gear items, got %d", len(listResp.Msg.GearItems))
		}
	})

	t.Run("non-existent gear is skipped", func(t *testing.T) {
		// Create a new community for this test
		createReq := connect.NewRequest(&api.CreateCommunityRequest{
			Name: "Non-Existent Gear Test Community",
		})
		createResp, err := service.CreateCommunity(ctxOwner, createReq)
		if err != nil {
			t.Fatalf("Failed to create community: %v", err)
		}
		testCommunityID := createResp.Msg.Id

		// Manually insert a CommunityGear record pointing to a non-existent gear ID
		fakeGearID := "00000000-0000-0000-0000-000000000000"
		communityGear := &models.CommunityGear{
			CommunityId:      testCommunityID,
			GearId:           fakeGearID,
			Availability:     models.Availability_AVAILABILITY_FOR_LOAN,
			CreatedAtUnixSec: time.Now().Unix(),
		}
		_, err = testStorage.Insert(ctxOwner, communityGear)
		if err != nil {
			t.Fatalf("Failed to insert community gear: %v", err)
		}

		// List community gear - orphaned CommunityGear records are silently
		// skipped (batch fetch returns empty for missing IDs).
		listReq := connect.NewRequest(&api.ListCommunityGearRequest{
			CommunityId: testCommunityID,
		})
		listResp, err := service.ListCommunityGear(ctxOwner, listReq)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(listResp.Msg.GearItems) != 0 {
			t.Errorf("Expected 0 gear items (orphaned record skipped), got %d", len(listResp.Msg.GearItems))
		}
	})

	t.Run("soft-deleted gear is skipped without error", func(t *testing.T) {
		// Create a new community for this test
		createReq := connect.NewRequest(&api.CreateCommunityRequest{
			Name: "Soft Delete Test Community",
		})
		createResp, err := service.CreateCommunity(ctxOwner, createReq)
		if err != nil {
			t.Fatalf("Failed to create community: %v", err)
		}
		testCommunityID := createResp.Msg.Id

		// Create two gear items
		gear3 := &models.Gear{
			Name:    "Active Gear",
			OwnerId: ownerID,
			State:   models.GearState_GEAR_STATE_AVAILABLE,
		}
		gear3ID, err := testStorage.Insert(ctxOwner, gear3)
		if err != nil {
			t.Fatalf("Failed to create gear3: %v", err)
		}

		gear4 := &models.Gear{
			Name:    "To Be Deleted Gear",
			OwnerId: ownerID,
			State:   models.GearState_GEAR_STATE_AVAILABLE,
		}
		gear4ID, err := testStorage.Insert(ctxOwner, gear4)
		if err != nil {
			t.Fatalf("Failed to create gear4: %v", err)
		}

		// Share both gear items
		shareGearForTestWithAvailability(
			t, service, ctxOwner, gear3ID, testCommunityID, models.Availability_AVAILABILITY_FOR_LOAN,
		)
		shareGearForTestWithAvailability(
			t, service, ctxOwner, gear4ID, testCommunityID, models.Availability_AVAILABILITY_FOR_LOAN,
		)

		// Verify both are listed initially
		listReq := connect.NewRequest(&api.ListCommunityGearRequest{
			CommunityId: testCommunityID,
		})
		listResp, err := service.ListCommunityGear(ctxOwner, listReq)
		if err != nil {
			t.Fatalf("ListCommunityGear failed before deletion: %v", err)
		}
		if len(listResp.Msg.GearItems) != 2 {
			t.Fatalf("Expected 2 gear items before deletion, got %d", len(listResp.Msg.GearItems))
		}

		// Soft-delete gear4 by setting the Deleted metadata
		gear4.Id = gear4ID
		gear4.Deleted = &models.DeletedMetadata{
			DeletedByUserId:  ownerID,
			DeletedAtUnixSec: time.Now().Unix(),
		}
		err = testStorage.Update(ctxOwner, gear4)
		if err != nil {
			t.Fatalf("Failed to soft-delete gear4: %v", err)
		}

		// List community gear again - should NOT return an error and should skip deleted gear
		listResp, err = service.ListCommunityGear(ctxOwner, listReq)
		if err != nil {
			t.Fatalf("ListCommunityGear should not return error for soft-deleted gear: %v", err)
		}

		// Should only return the active gear, not the deleted one
		if len(listResp.Msg.GearItems) != 1 {
			t.Errorf("Expected 1 gear item after soft-deletion, got %d", len(listResp.Msg.GearItems))
		}

		// Verify the remaining item is the active one
		if len(listResp.Msg.GearItems) > 0 && listResp.Msg.GearItems[0].Name != "Active Gear" {
			t.Errorf("Expected remaining gear to be 'Active Gear', got %s", listResp.Msg.GearItems[0].Name)
		}
	})
}

// TestService_ListCommunityGear_QueryCount locks the per-call query count for
// ListCommunityGear. The endpoint is on the hot read path (community feed,
// gear browser); an N+1 regression here is one of the more expensive failure
// modes. The current count was measured after #1621 Phase 1+2 collapsed the
// pre-existing GetByID(community) + RequireUserIsCommunityMember pair into
// the single-JOIN RequireMemberOfActiveCommunity helper.
func TestService_ListCommunityGear_QueryCount(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	ownerID := setupTestUser(t, testStorage, "owner@example.com", "Owner")
	memberID := setupTestUser(t, testStorage, "member@example.com", "Member")
	ctxOwner := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	createResp, err := service.CreateCommunity(ctxOwner, connect.NewRequest(&api.CreateCommunityRequest{Name: "Test"}))
	if err != nil {
		t.Fatalf("CreateCommunity: %v", err)
	}
	communityID := createResp.Msg.Id
	addUserToCommunity(t, service, communityID, ownerID, memberID, "owner@example.com", "member@example.com")

	// Share two gear items so the response actually exercises the fanout
	// queries (gear, locations, owners, active loans). Single-row responses
	// hide N+1 bugs that only emerge with multiple rows.
	for i := range 2 {
		gearID, err := testStorage.Insert(ctxOwner, &models.Gear{
			Name: "Gear", OwnerId: ownerID, State: models.GearState_GEAR_STATE_AVAILABLE,
		})
		if err != nil {
			t.Fatalf("seed gear %d: %v", i, err)
		}
		shareGearForTestWithAvailability(
			t, service, ctxOwner, gearID, communityID, models.Availability_AVAILABILITY_FOR_LOAN,
		)
	}

	ctxMember := storage.WithQueryStats(createAuthenticatedContext(memberID, "member@example.com", models.Role_ROLE_USER))

	// Five queries measured post-Phase 2 with two gear items shared:
	//   1. RequireMemberOfActiveCommunity JOIN (auth + community-active check)
	//   2. QueryByFields on community_gear
	//   3. GetByIDs gear (batch)
	//   4. GetByIDs locations (batch — empty here, still a query)
	//   5. FetchAPIUsers for owners (batch)
	// Bumping this upward without justification means an N+1 has slipped in.
	// Any per-row query that scales with len(GearItems) is the regression
	// signature.
	const maxQueries = 5
	storage.AssertMaxQueries(t, ctxMember, maxQueries, func() {
		if _, err := service.ListCommunityGear(ctxMember, connect.NewRequest(&api.ListCommunityGearRequest{
			CommunityId: communityID,
		})); err != nil {
			t.Fatalf("ListCommunityGear: %v", err)
		}
	})
}
