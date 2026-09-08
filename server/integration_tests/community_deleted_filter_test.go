package integration_tests

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
	feed_svc "go.ripls.org/ripls/server/services/feed"
	search_svc "go.ripls.org/ripls/server/services/search"
	"go.ripls.org/ripls/server/storage"
)

// TestCommunityDeletedReadFilter is the end-to-end proof for issue #1621:
// once a community is soft-deleted, every community-scoped read surface
// either rejects the request (single-community handlers) or silently drops
// the community from results (multi-community handlers). Members of a
// deleted community get FailedPrecondition so the client can route them to
// the restore flow; non-members get NotFound (no information leak).
//
// This test force-flips Community.Deleted via direct storage write — it
// does NOT exercise the soft-delete RPC (which lands in #1619 Phase 2).
// The point is to prove that the read-path filter is in place before any
// write path can produce deleted state.
func TestCommunityDeletedReadFilter(t *testing.T) {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	ctx := context.Background()

	// Seed: a community, a member, and a gear shared with the community.
	memberID := uuid.New().String()
	communityID := uuid.New().String()

	insertUser(t, db, memberID, "alice@example.com", "Alice")

	community := &models.Community{
		Id:          communityID,
		Name:        "Test Community",
		CreatorId:   memberID,
		OwnerUserId: memberID,
	}
	if _, err := db.Insert(ctx, community); err != nil {
		t.Fatalf("seed community: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: memberID,
	}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	gearID, err := db.Insert(ctx, &models.Gear{
		Id: uuid.New().String(), Name: "Tent", OwnerId: memberID,
		State: models.GearState_GEAR_STATE_AVAILABLE,
	})
	if err != nil {
		t.Fatalf("seed gear: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityGear{
		Id: uuid.New().String(), CommunityId: communityID, GearId: gearID,
		Availability: models.Availability_AVAILABILITY_FOR_LOAN,
	}); err != nil {
		t.Fatalf("seed community_gear: %v", err)
	}

	// Build the three services we exercise. Community service needs a
	// bucket and a (nil) story creator; feed and search need only storage.
	tempDir := t.TempDir()
	bucket, err := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	if err != nil {
		t.Fatalf("bucket: %v", err)
	}
	communitySvc, _ := newTestCommunityService(t, db, bucket, notifications.NewMockService())
	feedSvc := feed_svc.New(db)
	searchSvc := search_svc.New(db)

	memberCtx := makeAuthContext(memberID, "alice@example.com", models.Role_ROLE_USER)

	// === Pre-delete: every surface returns the community as expected. ===
	t.Run("baseline before delete", func(t *testing.T) {
		if _, err := communitySvc.GetCommunity(memberCtx, connect.NewRequest(&api.GetCommunityRequest{Id: communityID})); err != nil {
			t.Fatalf("GetCommunity should succeed: %v", err)
		}
		listResp, err := communitySvc.ListCommunityGear(memberCtx, connect.NewRequest(&api.ListCommunityGearRequest{CommunityId: communityID}))
		if err != nil {
			t.Fatalf("ListCommunityGear should succeed: %v", err)
		}
		if len(listResp.Msg.GearItems) != 1 {
			t.Errorf("expected 1 gear in pre-delete listing, got %d", len(listResp.Msg.GearItems))
		}
	})

	// === Force-flip the community to soft-deleted. ===
	community.Deleted = &models.DeletedMetadata{
		DeletedByUserId:  memberID,
		DeletedAtUnixSec: time.Now().Unix(),
	}
	if err := db.Update(ctx, community); err != nil {
		t.Fatalf("force-flip community to deleted: %v", err)
	}

	// === Post-delete: every surface respects the deleted state. ===

	t.Run("GetCommunity returns NotFound", func(t *testing.T) {
		_, err := communitySvc.GetCommunity(memberCtx, connect.NewRequest(&api.GetCommunityRequest{Id: communityID}))
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if got := connect.CodeOf(err); got != connect.CodeNotFound {
			t.Fatalf("expected NotFound, got %s", got)
		}
	})

	t.Run("ListCommunityGear returns FailedPrecondition for member", func(t *testing.T) {
		_, err := communitySvc.ListCommunityGear(memberCtx, connect.NewRequest(&api.ListCommunityGearRequest{CommunityId: communityID}))
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		// Caller is a member of the deleted community → distinguishable code so
		// the client can offer the restore flow.
		if got := connect.CodeOf(err); got != connect.CodeFailedPrecondition {
			t.Fatalf("expected FailedPrecondition, got %s", got)
		}
	})

	t.Run("ListCommunityGear returns NotFound for non-member", func(t *testing.T) {
		nonMemberID := uuid.New().String()
		insertUser(t, db, nonMemberID, "bob@example.com", "Bob")
		nonMemberCtx := makeAuthContext(nonMemberID, "bob@example.com", models.Role_ROLE_USER)

		_, err := communitySvc.ListCommunityGear(nonMemberCtx, connect.NewRequest(&api.ListCommunityGearRequest{CommunityId: communityID}))
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		// Non-members must not be able to distinguish "deleted" from
		// "doesn't exist" — both surface as NotFound.
		if got := connect.CodeOf(err); got != connect.CodeNotFound {
			t.Fatalf("expected NotFound, got %s", got)
		}
	})

	t.Run("GetFeed silently drops the deleted community", func(t *testing.T) {
		// Multi-community surfaces drop deleted communities from the result
		// set rather than failing the entire request — a stale community ID
		// in the caller's input must not break the feed.
		resp, err := feedSvc.GetFeed(memberCtx, connect.NewRequest(&api.GetFeedRequest{
			CommunityIds: []string{communityID},
			PageSize:     10,
		}))
		if err != nil {
			t.Fatalf("GetFeed should succeed with empty results, got err: %v", err)
		}
		if len(resp.Msg.Items) != 0 {
			t.Fatalf("expected 0 feed items for deleted community, got %d", len(resp.Msg.Items))
		}
	})

	t.Run("Search silently drops the deleted community", func(t *testing.T) {
		resp, err := searchSvc.Search(memberCtx, connect.NewRequest(&api.SearchRequest{
			Query:        "tent",
			CommunityIds: []string{communityID},
			ItemTypes:    []api.SearchItemType{api.SearchItemType_SEARCH_ITEM_TYPE_GEAR},
			Strategy:     api.SearchStrategy_SEARCH_STRATEGY_EXACT,
		}))
		if err != nil {
			t.Fatalf("Search should succeed with empty results, got err: %v", err)
		}
		if len(resp.Msg.Results) != 0 {
			t.Fatalf("expected 0 search results for deleted community, got %d", len(resp.Msg.Results))
		}
	})

	// Restore semantics (clearing Deleted) are out of scope for this issue —
	// they ship with #1619 Phase 2's RestoreCommunity RPC, which clears the
	// flat soft-delete columns explicitly.
}
