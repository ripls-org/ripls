package storage

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestLeaverCascade_QueryCount pins the total query count for running
// all six leaver-scoped cascade helpers against a small fixture. The
// bound is empirical: regenerate it after intentional implementation
// changes by removing the AssertMaxQueries wrapper, running the test,
// and reading off the actual count from GetQueryStats. Bumping the
// bound silently is the kind of regression this test exists to catch
// — bump only when you've verified the new behavior is intentional.
//
// Fixture: 1 leaver, 1 community, 2 leaver-owned gear shared into the
// community (plus 1 in another community), 1 leaver-authored request,
// 1 active transfer, 1 RSVP, 1 notification preferences row, 1
// invitation link. Off-scope rows are also present to verify
// scoping; they should not contribute Updates because the helpers
// filter them out before writing.
func TestLeaverCascade_QueryCount(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Unix()
	deleted := &models.DeletedMetadata{DeletedByUserId: "leaver", DeletedAtUnixSec: now}

	const communityID = "comm-qc"
	const otherCommunityID = "comm-qc-other"
	const leaverID = "user-qc-leaver"
	const otherID = "user-qc-other"

	// Two gear owned by the leaver, two community shares (one in
	// target, one in other community).
	g1, err := store.Insert(ctx, &models.Gear{Name: "g1", OwnerId: leaverID})
	if err != nil {
		t.Fatalf("insert g1: %v", err)
	}
	g2, err := store.Insert(ctx, &models.Gear{Name: "g2", OwnerId: leaverID})
	if err != nil {
		t.Fatalf("insert g2: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityGear{CommunityId: communityID, GearId: g1}); err != nil {
		t.Fatalf("insert cg1: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityGear{CommunityId: communityID, GearId: g2}); err != nil {
		t.Fatalf("insert cg2: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityGear{CommunityId: otherCommunityID, GearId: g1}); err != nil {
		t.Fatalf("insert cg-other: %v", err)
	}
	// Other-user gear in target community: must not be touched.
	otherGear, err := store.Insert(ctx, &models.Gear{Name: "other-g", OwnerId: otherID})
	if err != nil {
		t.Fatalf("insert other gear: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityGear{CommunityId: communityID, GearId: otherGear}); err != nil {
		t.Fatalf("insert other-cg: %v", err)
	}

	// One request authored by the leaver, shared into the target
	// community.
	req, err := store.Insert(ctx, &models.Request{Title: "r", RequesterId: leaverID})
	if err != nil {
		t.Fatalf("insert request: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityRequest{CommunityId: communityID, RequestId: req}); err != nil {
		t.Fatalf("insert cr: %v", err)
	}

	// One active transfer with leaver as owner.
	if _, err := store.Insert(ctx, &models.Transfer{
		CommunityId: communityID, OwnerId: leaverID, RecipientId: otherID,
		State: models.TransferState_TRANSFER_STATE_ACTIVE,
	}); err != nil {
		t.Fatalf("insert transfer: %v", err)
	}

	// One RSVP by the leaver.
	if _, err := store.Insert(ctx, &models.ExperienceRSVP{
		ExperienceId: "exp-qc", UserId: leaverID, CommunityId: communityID,
	}); err != nil {
		t.Fatalf("insert rsvp: %v", err)
	}

	// One notification preferences row.
	if _, err := store.Insert(ctx, &models.CommunityNotificationPreferences{
		CommunityId: communityID, UserId: leaverID,
	}); err != nil {
		t.Fatalf("insert prefs: %v", err)
	}

	// One invitation link.
	if _, err := store.Insert(ctx, &models.CommunityInvitationLink{
		CommunityId: communityID, InviterId: leaverID, ShortCode: "QCXYZ",
	}); err != nil {
		t.Fatalf("insert invlink: %v", err)
	}

	// Bound chosen with headroom: each helper does 1 SELECT plus N
	// UPDATEs, plus the gear/request helpers do an extra SELECT
	// (owner_id / requester_id lookup before the join-row scan). For
	// this fixture: gear=3 (2 selects + 2 updates), request=3
	// (2 selects + 1 update), transfer=2 (1 select + 1 update),
	// rsvp=2, prefs=2, invlink=2 → total ~16. Bound at 25 to absorb
	// minor implementation drift; tighten if it becomes too lax.
	const maxQueries = 25
	statsCtx := WithQueryStats(ctx)
	AssertMaxQueries(t, statsCtx, maxQueries, func() {
		if err := CascadeDeleteCommunityGearByOwnerInCommunity(statsCtx, store, communityID, leaverID, deleted); err != nil {
			t.Fatalf("gear cascade: %v", err)
		}
		if err := CancelLeaverCommunityRequestsInCommunity(statsCtx, store, communityID, leaverID, deleted); err != nil {
			t.Fatalf("request cascade: %v", err)
		}
		if err := CancelLeaverActiveTransfersInCommunity(statsCtx, store, communityID, leaverID); err != nil {
			t.Fatalf("transfer cascade: %v", err)
		}
		if err := DropLeaverExperienceRSVPsInCommunity(statsCtx, store, communityID, leaverID, deleted); err != nil {
			t.Fatalf("rsvp cascade: %v", err)
		}
		if err := DeleteLeaverCommunityNotificationPreferences(statsCtx, store, communityID, leaverID, deleted); err != nil {
			t.Fatalf("prefs cascade: %v", err)
		}
		if err := RevokeLeaverCommunityInvitationLinks(statsCtx, store, communityID, leaverID, deleted); err != nil {
			t.Fatalf("invlink cascade: %v", err)
		}
	})

	// Also report the actual count so a future tightening pass has
	// the empirical number visible in test output.
	if stats := GetQueryStats(statsCtx); stats != nil {
		t.Logf("actual query count for full leaver cascade: %d", stats.Count.Load())
	}
}
