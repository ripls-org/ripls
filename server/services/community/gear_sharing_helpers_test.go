package community

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// =============================================================================
// GEAR SHARING HELPER FUNCTION TESTS
// =============================================================================.

// TestVerifyGearViewer covers the view-access check gating link-only ShareItem
// calls (#2630): the owner (even for gear shared nowhere yet — the lazy
// per-item-community path) and any member of a community the gear is shared
// with pass (and get the owner ID back); everyone else is denied.
func TestVerifyGearViewer(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)
	ctx := context.Background()

	ownerID := setupTestUser(t, testStorage, "owner@example.com", "Owner")
	memberID := setupTestUser(t, testStorage, "member@example.com", "Member")
	strangerID := setupTestUser(t, testStorage, "stranger@example.com", "Stranger")

	gearID, err := testStorage.Insert(ctx, &models.Gear{Name: "Drill", OwnerId: ownerID})
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}

	t.Run("owner passes even when gear is shared nowhere", func(t *testing.T) {
		gotOwner, err := service.VerifyGearViewer(ctx, gearID, ownerID)
		if err != nil || gotOwner != ownerID {
			t.Errorf("got (%q, %v), want (%q, nil)", gotOwner, err, ownerID)
		}
	})

	t.Run("non-owner is denied when gear is shared nowhere", func(t *testing.T) {
		_, err := service.VerifyGearViewer(ctx, gearID, memberID)
		if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("expected PermissionDenied, got %v", err)
		}
	})

	// Share the gear into a community with owner + member.
	communityID, err := testStorage.Insert(ctx, &models.Community{Name: "Block", CreatorId: ownerID, OwnerUserId: ownerID})
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}
	for _, uid := range []string{ownerID, memberID} {
		if _, err := testStorage.Insert(ctx, &models.CommunityUser{CommunityId: communityID, UserId: uid}); err != nil {
			t.Fatalf("insert membership for %s: %v", uid, err)
		}
	}
	if _, err := testStorage.Insert(ctx, &models.CommunityGear{CommunityId: communityID, GearId: gearID}); err != nil {
		t.Fatalf("insert community gear: %v", err)
	}

	t.Run("shared-community member passes and gets owner id", func(t *testing.T) {
		gotOwner, err := service.VerifyGearViewer(ctx, gearID, memberID)
		if err != nil || gotOwner != ownerID {
			t.Errorf("got (%q, %v), want (%q, nil)", gotOwner, err, ownerID)
		}
	})

	t.Run("non-member is denied", func(t *testing.T) {
		_, err := service.VerifyGearViewer(ctx, gearID, strangerID)
		if err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("expected PermissionDenied, got %v", err)
		}
	})

	t.Run("missing gear is NotFound", func(t *testing.T) {
		_, err := service.VerifyGearViewer(ctx, "does-not-exist", ownerID)
		if err == nil || connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("expected NotFound, got %v", err)
		}
	})
}

func TestIsGearSharedWithCommunity(t *testing.T) {
	storage := setupTestStorage(t)
	ctx := context.Background()

	ownerID := setupTestUser(t, storage, "owner@example.com", "Owner")

	// Create a community
	community := &models.Community{
		Name:        "Test Community",
		CreatorId:   ownerID,
		OwnerUserId: ownerID,
	}
	communityID, err := storage.Insert(ctx, community)
	if err != nil {
		t.Fatalf("Failed to create test community: %v", err)
	}

	// Create gear
	gear := &models.Gear{
		Name:    "Test Gear",
		OwnerId: ownerID,
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	}
	gearID, err := storage.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("Failed to create gear: %v", err)
	}

	// Share gear with community
	communityGear := &models.CommunityGear{
		CommunityId: communityID,
		GearId:      gearID,
	}
	_, err = storage.Insert(ctx, communityGear)
	if err != nil {
		t.Fatalf("Failed to share gear: %v", err)
	}

	t.Run("returns true when gear is shared with community", func(t *testing.T) {
		isShared, err := IsGearSharedWithCommunity(ctx, storage, communityID, gearID)
		if err != nil {
			t.Fatalf("IsGearSharedWithCommunity failed: %v", err)
		}
		if !isShared {
			t.Error("Expected gear to be shared with community")
		}
	})

	t.Run("returns false when gear is not shared with community", func(t *testing.T) {
		// Create another gear that's not shared
		gear2 := &models.Gear{
			Name:    "Unshared Gear",
			OwnerId: ownerID,
			State:   models.GearState_GEAR_STATE_AVAILABLE,
		}
		gear2ID, err := storage.Insert(ctx, gear2)
		if err != nil {
			t.Fatalf("Failed to create gear2: %v", err)
		}

		isShared, err := IsGearSharedWithCommunity(ctx, storage, communityID, gear2ID)
		if err != nil {
			t.Fatalf("IsGearSharedWithCommunity failed: %v", err)
		}
		if isShared {
			t.Error("Expected gear to not be shared with community")
		}
	})
}

func TestGetCommunityGear(t *testing.T) {
	storage := setupTestStorage(t)
	ctx := context.Background()

	ownerID := setupTestUser(t, storage, "owner@example.com", "Owner")

	// Create a community
	community := &models.Community{
		Name:        "Test Community",
		CreatorId:   ownerID,
		OwnerUserId: ownerID,
	}
	communityID, err := storage.Insert(ctx, community)
	if err != nil {
		t.Fatalf("Failed to create test community: %v", err)
	}

	// Create gear
	gear := &models.Gear{
		Name:    "Test Gear",
		OwnerId: ownerID,
		State:   models.GearState_GEAR_STATE_AVAILABLE,
	}
	gearID, err := storage.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("Failed to create gear: %v", err)
	}

	// Share gear with community
	communityGear := &models.CommunityGear{
		CommunityId: communityID,
		GearId:      gearID,
	}
	communityGearID, err := storage.Insert(ctx, communityGear)
	if err != nil {
		t.Fatalf("Failed to share gear: %v", err)
	}

	t.Run("returns community gear when it exists", func(t *testing.T) {
		result, err := GetCommunityGear(ctx, storage, communityID, gearID)
		if err != nil {
			t.Fatalf("GetCommunityGear failed: %v", err)
		}
		if result == nil {
			t.Fatal("Expected community gear to be found")
		}
		if result.Id != communityGearID {
			t.Errorf("Expected community gear ID %s, got %s", communityGearID, result.Id)
		}
		if result.CommunityId != communityID {
			t.Errorf("Expected community ID %s, got %s", communityID, result.CommunityId)
		}
		if result.GearId != gearID {
			t.Errorf("Expected gear ID %s, got %s", gearID, result.GearId)
		}
	})

	t.Run("returns nil when community gear does not exist", func(t *testing.T) {
		// Create another gear that's not shared
		gear2 := &models.Gear{
			Name:    "Unshared Gear",
			OwnerId: ownerID,
			State:   models.GearState_GEAR_STATE_AVAILABLE,
		}
		gear2ID, err := storage.Insert(ctx, gear2)
		if err != nil {
			t.Fatalf("Failed to create gear2: %v", err)
		}

		result, err := GetCommunityGear(ctx, storage, communityID, gear2ID)
		if err != nil {
			t.Fatalf("GetCommunityGear failed: %v", err)
		}
		if result != nil {
			t.Error("Expected community gear to be nil for unshared gear")
		}
	})
}
