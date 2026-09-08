package gear

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

// TestService_SaveGear_NonOwnerAppendsMedia covers the carve-out added for
// the media-carousel "Add media" flow: a community member who isn't the
// gear owner can append media IDs, but cannot change any other field or
// remove/reorder existing media.
func TestService_SaveGear_NonOwnerAppendsMedia(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := New(testStorage, &services.MockBucketStorage{})

	const ownerID = "owner-gear-1"
	const memberID = "member-gear-1"
	const strangerID = "stranger-gear-1"
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	memberCtx := createAuthenticatedContext(memberID, "member@example.com", models.Role_ROLE_USER)
	strangerCtx := createAuthenticatedContext(strangerID, "stranger@example.com", models.Role_ROLE_USER)

	createResp, err := service.SaveGear(ownerCtx, connect.NewRequest(&api.SaveGearRequest{
		Name:        proto.String("Cordless Drill"),
		Description: proto.String("Owner-authored description"),
		MediaIds:    []string{"existing-media-1"},
	}))
	if err != nil {
		t.Fatalf("seed: create gear: %v", err)
	}
	gearID := createResp.Msg.Id

	// Share the gear with a community and add memberID as a member. The
	// community row has a CHECK constraint requiring owner_user_id, so we
	// have to populate it on insert.
	communityID, err := testStorage.Insert(context.Background(), &models.Community{
		Name:        "Tool Library",
		OwnerUserId: ownerID,
	})
	if err != nil {
		t.Fatalf("seed: create community: %v", err)
	}
	if _, err := testStorage.Insert(context.Background(), &models.CommunityGear{
		CommunityId: communityID,
		GearId:      gearID,
	}); err != nil {
		t.Fatalf("seed: link community to gear: %v", err)
	}
	if _, err := testStorage.Insert(context.Background(), &models.CommunityUser{
		CommunityId: communityID,
		UserId:      memberID,
	}); err != nil {
		t.Fatalf("seed: add membership: %v", err)
	}

	t.Run("community member can append media", func(t *testing.T) {
		stored := &models.Gear{}
		if err := testStorage.GetByID(context.Background(), gearID, stored); err != nil {
			t.Fatalf("load gear: %v", err)
		}
		_, err := service.SaveGear(memberCtx, connect.NewRequest(&api.SaveGearRequest{
			Id:          gearID,
			Name:        proto.String(stored.Name),
			Description: proto.String(stored.Description),
			MediaIds:    append(append([]string{}, stored.MediaIds...), "member-uploaded-media"),
		}))
		if err != nil {
			t.Fatalf("expected non-owner append to succeed, got: %v", err)
		}

		after := &models.Gear{}
		if err := testStorage.GetByID(context.Background(), gearID, after); err != nil {
			t.Fatalf("load gear after append: %v", err)
		}
		if last := after.MediaIds[len(after.MediaIds)-1]; last != "member-uploaded-media" {
			t.Fatalf("expected last media id to be member's upload, got %v", after.MediaIds)
		}
	})

	t.Run("community member cannot change description", func(t *testing.T) {
		stored := &models.Gear{}
		if err := testStorage.GetByID(context.Background(), gearID, stored); err != nil {
			t.Fatalf("load gear: %v", err)
		}
		_, err := service.SaveGear(memberCtx, connect.NewRequest(&api.SaveGearRequest{
			Id:          gearID,
			Description: proto.String("Member-altered description"),
			MediaIds:    append(append([]string{}, stored.MediaIds...), "another-add"),
		}))
		if !gearIsPermissionDenied(err) {
			t.Fatalf("expected permission_denied for description change, got: %v", err)
		}
	})

	t.Run("community member cannot remove existing media", func(t *testing.T) {
		_, err := service.SaveGear(memberCtx, connect.NewRequest(&api.SaveGearRequest{
			Id:       gearID,
			MediaIds: []string{"only-new"}, // drops existing
		}))
		if !gearIsPermissionDenied(err) {
			t.Fatalf("expected permission_denied for removal, got: %v", err)
		}
	})

	t.Run("community member cannot reorder existing media", func(t *testing.T) {
		stored := &models.Gear{}
		if err := testStorage.GetByID(context.Background(), gearID, stored); err != nil {
			t.Fatalf("load gear: %v", err)
		}
		if len(stored.MediaIds) < 2 {
			t.Skipf("need at least 2 media items to test reorder; have %d", len(stored.MediaIds))
		}
		swapped := append([]string{}, stored.MediaIds...)
		swapped[0], swapped[1] = swapped[1], swapped[0]
		_, err := service.SaveGear(memberCtx, connect.NewRequest(&api.SaveGearRequest{
			Id:       gearID,
			MediaIds: swapped,
		}))
		if !gearIsPermissionDenied(err) {
			t.Fatalf("expected permission_denied for reorder, got: %v", err)
		}
	})

	t.Run("non-member cannot append media", func(t *testing.T) {
		stored := &models.Gear{}
		if err := testStorage.GetByID(context.Background(), gearID, stored); err != nil {
			t.Fatalf("load gear: %v", err)
		}
		_, err := service.SaveGear(strangerCtx, connect.NewRequest(&api.SaveGearRequest{
			Id:       gearID,
			MediaIds: append(append([]string{}, stored.MediaIds...), "stranger-add"),
		}))
		if !gearIsPermissionDenied(err) {
			t.Fatalf("expected permission_denied for non-member, got: %v", err)
		}
	})
}

func gearIsPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	var ce *connect.Error
	if !errors.As(err, &ce) {
		return false
	}
	return ce.Code() == connect.CodePermissionDenied
}
