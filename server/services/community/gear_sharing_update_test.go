package community

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestService_UpdateGearSharing(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	ownerID := setupTestUser(t, testStorage, "owner@example.com", "Owner")
	nonOwnerID := setupTestUser(t, testStorage, "nonowner@example.com", "Non-Owner")

	ctxOwner := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	ctxNonOwner := createAuthenticatedContext(nonOwnerID, "nonowner@example.com", models.Role_ROLE_USER)

	// Create community with owner
	createCommunityReq := connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Test Community",
	})
	createCommunityResp, err := service.CreateCommunity(ctxOwner, createCommunityReq)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}
	communityID := createCommunityResp.Msg.Id

	// Add non-owner to community
	addUserToCommunity(t, service, communityID, ownerID, nonOwnerID, "owner@example.com", "nonowner@example.com")

	// Create gear as owner
	gear := &models.Gear{
		Name:    "Test Gear",
		OwnerId: ownerID,
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	}
	gearID, err := testStorage.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("Failed to create gear: %v", err)
	}

	// Share gear with community (initially FOR_LOAN)
	shareGearForTestWithAvailability(
		t, service, ctxOwner, gearID, communityID, models.Availability_AVAILABILITY_FOR_LOAN,
	)

	t.Run("owner can update gear sharing availability", func(t *testing.T) {
		updateReq := connect.NewRequest(&api.UpdateGearSharingRequest{
			GearId:       gearID,
			CommunityId:  communityID,
			Availability: api.Availability_AVAILABILITY_FOR_GIVEAWAY,
		})

		_, err := service.UpdateGearSharing(ctxOwner, updateReq)
		if err != nil {
			t.Fatalf("UpdateGearSharing failed: %v", err)
		}

		// Verify the availability was updated
		communityGear, err := GetCommunityGear(context.Background(), testStorage, communityID, gearID)
		if err != nil {
			t.Fatalf("Failed to get community gear: %v", err)
		}
		if communityGear.Availability != models.Availability_AVAILABILITY_FOR_GIVEAWAY {
			t.Errorf("Expected availability FOR_GIVEAWAY, got %v", communityGear.Availability)
		}
	})

	t.Run("non-owner cannot update gear sharing", func(t *testing.T) {
		updateReq := connect.NewRequest(&api.UpdateGearSharingRequest{
			GearId:       gearID,
			CommunityId:  communityID,
			Availability: api.Availability_AVAILABILITY_FOR_LOAN,
		})

		_, err := service.UpdateGearSharing(ctxNonOwner, updateReq)
		if err == nil {
			t.Fatal("Expected error when non-owner tries to update gear sharing")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("Expected CodePermissionDenied, got %v", connect.CodeOf(err))
		}
	})

	t.Run("cannot update if gear not shared with community", func(t *testing.T) {
		// Create another gear not shared
		gear2 := &models.Gear{
			Name:    "Unshared Gear",
			OwnerId: ownerID,
			State:   models.GearState_GEAR_STATE_AVAILABLE,
		}
		gear2ID, err := testStorage.Insert(context.Background(), gear2)
		if err != nil {
			t.Fatalf("Failed to create gear2: %v", err)
		}

		updateReq := connect.NewRequest(&api.UpdateGearSharingRequest{
			GearId:       gear2ID,
			CommunityId:  communityID,
			Availability: api.Availability_AVAILABILITY_FOR_LOAN,
		})

		_, err = service.UpdateGearSharing(ctxOwner, updateReq)
		if err == nil {
			t.Fatal("Expected error when updating unshared gear")
		}
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("Expected CodeNotFound, got %v", connect.CodeOf(err))
		}
	})
}
