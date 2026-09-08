package community

import (
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestService_UnshareGearFromCommunity covers the gear removal core that
// CommunityService.UnshareItem drives: the hard delete of the CommunityGear
// junction, the GEAR_UNSHARED event, the owner-only gate, and the idempotent
// no-op when the gear isn't shared with the community.
func TestService_UnshareGearFromCommunity(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	ownerID := setupTestUser(t, testStorage, "owner@example.com", "Owner")
	memberID := setupTestUser(t, testStorage, "member@example.com", "Member")

	ctxOwner := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	ctxMember := createAuthenticatedContext(memberID, "member@example.com", models.Role_ROLE_USER)

	// Create community
	createCommunityReq := connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Test Community",
	})
	createCommunityResp, err := service.CreateCommunity(ctxOwner, createCommunityReq)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}
	communityID := createCommunityResp.Msg.Id

	// Create and share gear
	gear := &models.Gear{
		Name:    "Test Gear",
		OwnerId: ownerID,
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	}
	gearID, err := testStorage.Insert(ctxOwner, gear)
	if err != nil {
		t.Fatalf("Failed to create gear: %v", err)
	}

	shareGearForTestWithAvailability(
		t, service, ctxOwner, gearID, communityID, models.Availability_AVAILABILITY_FOR_LOAN,
	)

	t.Run("owner can unshare their gear", func(t *testing.T) {
		if err := unshareGearFrom(t, service, ctxOwner, gearID, communityID); err != nil {
			t.Fatalf("unshare failed: %v", err)
		}

		// Verify gear is no longer shared
		shares, err := testStorage.QueryByField(ctxOwner, "gear_id", gearID, &models.CommunityGear{})
		if err != nil {
			t.Fatalf("Failed to query shares: %v", err)
		}

		if len(shares) != 0 {
			t.Errorf("Expected 0 shares after unshare, got %d", len(shares))
		}

		// Verify event was logged
		events, err := testStorage.QueryByField(ctxOwner, "community_id", communityID, &models.CommunityEvent{})
		if err != nil {
			t.Fatalf("Failed to query events: %v", err)
		}

		var foundEvent *models.CommunityEvent
		for _, msg := range events {
			event := msg.(*models.CommunityEvent)
			if event.EventType == models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_UNSHARED &&
				event.ActorId == ownerID &&
				event.GearId == gearID {
				foundEvent = event
				break
			}
		}

		if foundEvent == nil {
			t.Error("Expected GEAR_UNSHARED event to be logged")
		}
	})

	t.Run("non-owner cannot unshare gear", func(t *testing.T) {
		// Share gear again
		shareGearForTestWithAvailability(
			t, service, ctxOwner, gearID, communityID, models.Availability_AVAILABILITY_FOR_LOAN,
		)

		err := unshareGearFrom(t, service, ctxMember, gearID, communityID)
		if err == nil {
			t.Fatal("Expected error when non-owner tries to unshare gear")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok || connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", err)
		}
	})

	t.Run("unsharing is idempotent", func(t *testing.T) {
		// The gear is still shared here — the non-owner subtest re-shared it and
		// its unshare was rejected. Remove it, then remove it again: the second
		// call must be a silent no-op, which is what UnshareItem relies on.
		if err := unshareGearFrom(t, service, ctxOwner, gearID, communityID); err != nil {
			t.Fatalf("first unshare failed: %v", err)
		}
		if err := unshareGearFrom(t, service, ctxOwner, gearID, communityID); err != nil {
			t.Fatalf("second unshare should be idempotent: %v", err)
		}
	})
}
