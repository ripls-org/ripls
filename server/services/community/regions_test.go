package community

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestService_SetCommunityRegionOverride(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := setupTestService(t, testStorage)

	// Create a region to use in the tests.
	now := time.Now().Unix()
	region := &models.Region{
		RegionType:       "state",
		RegionName:       "Colorado",
		CreatedAtUnixSec: now,
		UpdatedAtUnixSec: now,
	}
	regionID, err := testStorage.Insert(context.Background(), region)
	if err != nil {
		t.Fatalf("failed to create test region: %v", err)
	}

	setOverride := func(ctx context.Context, communityID string) error {
		req := connect.NewRequest(&api.SetCommunityRegionOverrideRequest{
			CommunityId: communityID,
			RegionId:    regionID,
		})
		_, err := service.SetCommunityRegionOverride(ctx, req)
		return err
	}

	t.Run("owner_can_set", func(t *testing.T) {
		ownerID := setupTestUser(t, testStorage, "owner@example.com", "Owner")
		ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

		createResp, err := service.CreateCommunity(ownerCtx, connect.NewRequest(&api.CreateCommunityRequest{
			Name: "Owner Test Community",
		}))
		if err != nil {
			t.Fatalf("CreateCommunity: %v", err)
		}
		communityID := createResp.Msg.Id

		if err := setOverride(ownerCtx, communityID); err != nil {
			t.Fatalf("owner should be able to set region override, got: %v", err)
		}

		// Verify a CommunityRegion row was written with IsOverride=true.
		rows, err := testStorage.QueryByField(context.Background(), "community_id", communityID, &models.CommunityRegion{})
		if err != nil {
			t.Fatalf("QueryByField: %v", err)
		}
		if len(rows) == 0 {
			t.Fatal("expected at least one CommunityRegion row")
		}
		found := false
		for _, msg := range rows {
			cr := msg.(*models.CommunityRegion)
			if cr.RegionId == regionID && cr.IsOverride {
				found = true
			}
		}
		if !found {
			t.Error("expected an override CommunityRegion row for the specified region")
		}
	})

	t.Run("non_owner_member_denied", func(t *testing.T) {
		ownerID := setupTestUser(t, testStorage, "owner2@example.com", "Owner Two")
		memberID := setupTestUser(t, testStorage, "member2@example.com", "Member Two")
		ownerCtx := createAuthenticatedContext(ownerID, "owner2@example.com", models.Role_ROLE_USER)
		memberCtx := createAuthenticatedContext(memberID, "member2@example.com", models.Role_ROLE_USER)

		createResp, err := service.CreateCommunity(ownerCtx, connect.NewRequest(&api.CreateCommunityRequest{
			Name: "Non-owner Member Test Community",
		}))
		if err != nil {
			t.Fatalf("CreateCommunity: %v", err)
		}
		communityID := createResp.Msg.Id

		addUserToCommunity(t, service, communityID, ownerID, memberID, "owner2@example.com", "member2@example.com")

		err = setOverride(memberCtx, communityID)
		if err == nil {
			t.Fatal("expected PermissionDenied for non-owner member, got nil")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected *connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("expected PermissionDenied, got %v", connectErr.Code())
		}
	})

	t.Run("non_member_denied", func(t *testing.T) {
		ownerID := setupTestUser(t, testStorage, "owner3@example.com", "Owner Three")
		nonMemberID := setupTestUser(t, testStorage, "nonmember3@example.com", "Non Member")
		ownerCtx := createAuthenticatedContext(ownerID, "owner3@example.com", models.Role_ROLE_USER)
		nonMemberCtx := createAuthenticatedContext(nonMemberID, "nonmember3@example.com", models.Role_ROLE_USER)

		createResp, err := service.CreateCommunity(ownerCtx, connect.NewRequest(&api.CreateCommunityRequest{
			Name: "Non-member Test Community",
		}))
		if err != nil {
			t.Fatalf("CreateCommunity: %v", err)
		}
		communityID := createResp.Msg.Id

		err = setOverride(nonMemberCtx, communityID)
		if err == nil {
			t.Fatal("expected PermissionDenied for non-member, got nil")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected *connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("expected PermissionDenied, got %v", connectErr.Code())
		}
	})

	// Regression test for the exploit described in #2134: the original creator
	// who transferred ownership via LeaveCommunity and then left must be denied.
	t.Run("original_creator_after_handoff_denied", func(t *testing.T) {
		creatorID := setupTestUser(t, testStorage, "creator4@example.com", "Creator Four")
		newOwnerID := setupTestUser(t, testStorage, "newowner4@example.com", "New Owner Four")
		creatorCtx := createAuthenticatedContext(creatorID, "creator4@example.com", models.Role_ROLE_USER)
		newOwnerCtx := createAuthenticatedContext(newOwnerID, "newowner4@example.com", models.Role_ROLE_USER)

		// Creator founds the community.
		createResp, err := service.CreateCommunity(creatorCtx, connect.NewRequest(&api.CreateCommunityRequest{
			Name: "Handoff Exploit Test Community",
		}))
		if err != nil {
			t.Fatalf("CreateCommunity: %v", err)
		}
		communityID := createResp.Msg.Id

		// Invite new owner and accept.
		addUserToCommunity(t, service, communityID, creatorID, newOwnerID, "creator4@example.com", "newowner4@example.com")

		// Creator hands off ownership and leaves the community.
		_, err = service.LeaveCommunity(creatorCtx, connect.NewRequest(&api.LeaveCommunityRequest{
			CommunityId:    communityID,
			NewOwnerUserId: proto.String(newOwnerID),
		}))
		if err != nil {
			t.Fatalf("LeaveCommunity with handoff: %v", err)
		}

		// The ex-creator (now not a member) must be denied.
		err = setOverride(creatorCtx, communityID)
		if err == nil {
			t.Fatal("expected PermissionDenied for ex-creator after handoff, got nil")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("expected *connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("expected PermissionDenied, got %v", connectErr.Code())
		}

		// Sanity check: the new owner can still set the override.
		if err := setOverride(newOwnerCtx, communityID); err != nil {
			t.Fatalf("new owner should be able to set region override after handoff, got: %v", err)
		}
	})
}
