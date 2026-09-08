package storage

import (
	"context"
	"sort"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestCascadeDeleteCommunityGearByOwnerInCommunity(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Unix()
	deleted := &models.DeletedMetadata{DeletedByUserId: "leaver", DeletedAtUnixSec: now}

	t.Run("empty community is a noop", func(t *testing.T) {
		if err := CascadeDeleteCommunityGearByOwnerInCommunity(ctx, store, "comm-empty", "user-empty", deleted); err != nil {
			t.Fatalf("expected noop, got: %v", err)
		}
	})

	t.Run("soft-deletes only leaver-owned join rows in target community", func(t *testing.T) {
		const communityID = "comm-leaver-1"
		const otherCommunityID = "comm-other"
		const leaverID = "user-leaver"
		const otherUserID = "user-other"

		leaverGear, err := store.Insert(ctx, &models.Gear{Name: "leaver gear", OwnerId: leaverID})
		if err != nil {
			t.Fatalf("insert leaver gear: %v", err)
		}
		otherGear, err := store.Insert(ctx, &models.Gear{Name: "other gear", OwnerId: otherUserID})
		if err != nil {
			t.Fatalf("insert other gear: %v", err)
		}

		targetCG, err := store.Insert(ctx, &models.CommunityGear{CommunityId: communityID, GearId: leaverGear})
		if err != nil {
			t.Fatalf("insert target cg: %v", err)
		}
		// Same gear in a different community: must NOT be touched.
		offScopeCG, err := store.Insert(ctx, &models.CommunityGear{CommunityId: otherCommunityID, GearId: leaverGear})
		if err != nil {
			t.Fatalf("insert off-scope cg: %v", err)
		}
		// Other user's gear in the target community: must NOT be touched.
		otherUserCG, err := store.Insert(ctx, &models.CommunityGear{CommunityId: communityID, GearId: otherGear})
		if err != nil {
			t.Fatalf("insert other-user cg: %v", err)
		}

		if err := CascadeDeleteCommunityGearByOwnerInCommunity(ctx, store, communityID, leaverID, deleted); err != nil {
			t.Fatalf("cascade: %v", err)
		}

		assertCGDeleted(t, store, targetCG, true, "leaver's row in target community")
		assertCGDeleted(t, store, offScopeCG, false, "leaver's row in other community")
		assertCGDeleted(t, store, otherUserCG, false, "other user's row in target community")
	})

	t.Run("idempotent: skips already-deleted rows preserving prior metadata", func(t *testing.T) {
		const communityID = "comm-leaver-2"
		const leaverID = "user-leaver-2"
		earlier := &models.DeletedMetadata{DeletedByUserId: "earlier-actor", DeletedAtUnixSec: now - 3600}

		leaverGear, err := store.Insert(ctx, &models.Gear{Name: "g", OwnerId: leaverID})
		if err != nil {
			t.Fatalf("insert gear: %v", err)
		}
		cg, err := store.Insert(ctx, &models.CommunityGear{CommunityId: communityID, GearId: leaverGear, Deleted: earlier})
		if err != nil {
			t.Fatalf("insert cg: %v", err)
		}

		if err := CascadeDeleteCommunityGearByOwnerInCommunity(ctx, store, communityID, leaverID, deleted); err != nil {
			t.Fatalf("cascade: %v", err)
		}
		got := &models.CommunityGear{}
		if err := store.GetByID(ctx, cg, got, QueryOptions{IncludeDeleted: true}); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Deleted == nil || got.Deleted.DeletedByUserId != "earlier-actor" {
			t.Errorf("expected prior metadata preserved, got %+v", got.Deleted)
		}
	})

	t.Run("leaves Gear row itself untouched", func(t *testing.T) {
		const communityID = "comm-leaver-3"
		const leaverID = "user-leaver-3"

		leaverGear, err := store.Insert(ctx, &models.Gear{Name: "g", OwnerId: leaverID})
		if err != nil {
			t.Fatalf("insert gear: %v", err)
		}
		if _, err := store.Insert(ctx, &models.CommunityGear{CommunityId: communityID, GearId: leaverGear}); err != nil {
			t.Fatalf("insert cg: %v", err)
		}
		if err := CascadeDeleteCommunityGearByOwnerInCommunity(ctx, store, communityID, leaverID, deleted); err != nil {
			t.Fatalf("cascade: %v", err)
		}
		got := &models.Gear{}
		if err := store.GetByID(ctx, leaverGear, got); err != nil {
			t.Fatalf("get gear: %v", err)
		}
		if got.Deleted != nil {
			t.Error("gear itself should not be soft-deleted by leaver cascade")
		}
	})
}

func TestCancelLeaverCommunityRequestsInCommunity(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Unix()
	deleted := &models.DeletedMetadata{DeletedByUserId: "leaver", DeletedAtUnixSec: now}

	t.Run("soft-deletes only leaver-authored join rows in target community", func(t *testing.T) {
		const communityID = "comm-req-1"
		const otherCommunityID = "comm-req-other"
		const leaverID = "user-req-leaver"
		const otherUserID = "user-req-other"

		leaverReq, err := store.Insert(ctx, &models.Request{Title: "leaver req", RequesterId: leaverID})
		if err != nil {
			t.Fatalf("insert leaver req: %v", err)
		}
		otherReq, err := store.Insert(ctx, &models.Request{Title: "other req", RequesterId: otherUserID})
		if err != nil {
			t.Fatalf("insert other req: %v", err)
		}

		targetCR, err := store.Insert(ctx, &models.CommunityRequest{CommunityId: communityID, RequestId: leaverReq})
		if err != nil {
			t.Fatalf("insert target cr: %v", err)
		}
		offScopeCR, err := store.Insert(ctx, &models.CommunityRequest{CommunityId: otherCommunityID, RequestId: leaverReq})
		if err != nil {
			t.Fatalf("insert off-scope cr: %v", err)
		}
		otherUserCR, err := store.Insert(ctx, &models.CommunityRequest{CommunityId: communityID, RequestId: otherReq})
		if err != nil {
			t.Fatalf("insert other-user cr: %v", err)
		}

		if err := CancelLeaverCommunityRequestsInCommunity(ctx, store, communityID, leaverID, deleted); err != nil {
			t.Fatalf("cascade: %v", err)
		}

		assertCRDeleted(t, store, targetCR, true, "leaver's CR in target community")
		assertCRDeleted(t, store, offScopeCR, false, "leaver's CR in other community")
		assertCRDeleted(t, store, otherUserCR, false, "other user's CR in target community")

		// Request itself stays active.
		got := &models.Request{}
		if err := store.GetByID(ctx, leaverReq, got); err != nil {
			t.Fatalf("get request: %v", err)
		}
		if got.Deleted != nil {
			t.Error("request itself should not be soft-deleted")
		}
	})

	t.Run("noop when leaver has no requests", func(t *testing.T) {
		if err := CancelLeaverCommunityRequestsInCommunity(ctx, store, "comm-nope", "user-nope", deleted); err != nil {
			t.Fatalf("expected noop, got: %v", err)
		}
	})
}

func TestCancelLeaverActiveTransfersInCommunity(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("cancels active transfers where leaver is owner or recipient", func(t *testing.T) {
		const communityID = "comm-tx-1"
		const leaverID = "user-tx-leaver"
		const otherID = "user-tx-other"
		const thirdID = "user-tx-third"

		asOwner, err := store.Insert(ctx, &models.Transfer{
			CommunityId: communityID, OwnerId: leaverID, RecipientId: otherID,
			State: models.TransferState_TRANSFER_STATE_INTEREST_EXPRESSED,
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		asRecipient, err := store.Insert(ctx, &models.Transfer{
			CommunityId: communityID, OwnerId: otherID, RecipientId: leaverID,
			State: models.TransferState_TRANSFER_STATE_ACTIVE,
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		uninvolved, err := store.Insert(ctx, &models.Transfer{
			CommunityId: communityID, OwnerId: otherID, RecipientId: thirdID,
			State: models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED,
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		alreadyCompleted, err := store.Insert(ctx, &models.Transfer{
			CommunityId: communityID, OwnerId: leaverID, RecipientId: otherID,
			State: models.TransferState_TRANSFER_STATE_COMPLETED,
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		offScope, err := store.Insert(ctx, &models.Transfer{
			CommunityId: "other-comm", OwnerId: leaverID, RecipientId: otherID,
			State: models.TransferState_TRANSFER_STATE_ACTIVE,
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}

		if err := CancelLeaverActiveTransfersInCommunity(ctx, store, communityID, leaverID); err != nil {
			t.Fatalf("cascade: %v", err)
		}

		assertTransferState(t, store, asOwner, models.TransferState_TRANSFER_STATE_CANCELLED, "leaver as owner")
		assertTransferState(t, store, asRecipient, models.TransferState_TRANSFER_STATE_CANCELLED, "leaver as recipient")
		assertTransferState(t, store, uninvolved, models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED, "uninvolved")
		assertTransferState(t, store, alreadyCompleted, models.TransferState_TRANSFER_STATE_COMPLETED, "already completed")
		assertTransferState(t, store, offScope, models.TransferState_TRANSFER_STATE_ACTIVE, "off-scope community")
	})

	t.Run("idempotent: already-cancelled stays cancelled", func(t *testing.T) {
		const communityID = "comm-tx-2"
		const leaverID = "user-tx-leaver-2"
		preCancelled, err := store.Insert(ctx, &models.Transfer{
			CommunityId: communityID, OwnerId: leaverID, RecipientId: "x",
			State: models.TransferState_TRANSFER_STATE_CANCELLED,
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		if err := CancelLeaverActiveTransfersInCommunity(ctx, store, communityID, leaverID); err != nil {
			t.Fatalf("cascade: %v", err)
		}
		assertTransferState(t, store, preCancelled, models.TransferState_TRANSFER_STATE_CANCELLED, "pre-cancelled")
	})
}

func TestDropLeaverExperienceRSVPsInCommunity(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Unix()
	deleted := &models.DeletedMetadata{DeletedByUserId: "leaver", DeletedAtUnixSec: now}

	t.Run("soft-deletes leaver's RSVPs in target community", func(t *testing.T) {
		const communityID = "comm-rsvp-1"
		const otherCommunityID = "comm-rsvp-other"
		const leaverID = "user-rsvp-leaver"
		const otherID = "user-rsvp-other"

		target, err := store.Insert(ctx, &models.ExperienceRSVP{
			ExperienceId: "exp-1", UserId: leaverID, CommunityId: communityID,
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		offScopeCommunity, err := store.Insert(ctx, &models.ExperienceRSVP{
			ExperienceId: "exp-2", UserId: leaverID, CommunityId: otherCommunityID,
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		offScopeUser, err := store.Insert(ctx, &models.ExperienceRSVP{
			ExperienceId: "exp-1", UserId: otherID, CommunityId: communityID,
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}

		if err := DropLeaverExperienceRSVPsInCommunity(ctx, store, communityID, leaverID, deleted); err != nil {
			t.Fatalf("cascade: %v", err)
		}

		assertRSVPDeleted(t, store, target, true, "leaver's RSVP in target community")
		assertRSVPDeleted(t, store, offScopeCommunity, false, "leaver's RSVP in other community")
		assertRSVPDeleted(t, store, offScopeUser, false, "other user's RSVP in target community")
	})

	t.Run("noop on empty set", func(t *testing.T) {
		if err := DropLeaverExperienceRSVPsInCommunity(ctx, store, "comm-empty", "user-empty", deleted); err != nil {
			t.Fatalf("expected noop, got: %v", err)
		}
	})
}

func TestDeleteLeaverCommunityNotificationPreferences(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Unix()
	deleted := &models.DeletedMetadata{DeletedByUserId: "leaver", DeletedAtUnixSec: now}

	t.Run("soft-deletes leaver's prefs in target community", func(t *testing.T) {
		const communityID = "comm-prefs-1"
		const leaverID = "user-prefs-leaver"

		target, err := store.Insert(ctx, &models.CommunityNotificationPreferences{
			CommunityId: communityID, UserId: leaverID,
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		offScopeCommunity, err := store.Insert(ctx, &models.CommunityNotificationPreferences{
			CommunityId: "comm-other", UserId: leaverID,
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		offScopeUser, err := store.Insert(ctx, &models.CommunityNotificationPreferences{
			CommunityId: communityID, UserId: "other-user",
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}

		if err := DeleteLeaverCommunityNotificationPreferences(ctx, store, communityID, leaverID, deleted); err != nil {
			t.Fatalf("cascade: %v", err)
		}

		assertPrefsDeleted(t, store, target, true, "leaver's prefs in target community")
		assertPrefsDeleted(t, store, offScopeCommunity, false, "leaver's prefs in other community")
		assertPrefsDeleted(t, store, offScopeUser, false, "other user's prefs in target community")
	})
}

func TestRevokeLeaverCommunityInvitationLinks(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().Unix()
	deleted := &models.DeletedMetadata{DeletedByUserId: "leaver", DeletedAtUnixSec: now}

	t.Run("soft-deletes leaver-authored links in target community", func(t *testing.T) {
		const communityID = "comm-inv-1"
		const leaverID = "user-inv-leaver"

		target, err := store.Insert(ctx, &models.CommunityInvitationLink{
			CommunityId: communityID, InviterId: leaverID, ShortCode: "AAA1",
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		offScopeCommunity, err := store.Insert(ctx, &models.CommunityInvitationLink{
			CommunityId: "comm-other", InviterId: leaverID, ShortCode: "BBB2",
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		offScopeUser, err := store.Insert(ctx, &models.CommunityInvitationLink{
			CommunityId: communityID, InviterId: "other-user", ShortCode: "CCC3",
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}

		if err := RevokeLeaverCommunityInvitationLinks(ctx, store, communityID, leaverID, deleted); err != nil {
			t.Fatalf("cascade: %v", err)
		}

		assertLinkDeleted(t, store, target, true, "leaver's link in target community")
		assertLinkDeleted(t, store, offScopeCommunity, false, "leaver's link in other community")
		assertLinkDeleted(t, store, offScopeUser, false, "other user's link in target community")
	})

	t.Run("preserves is_revoked flag independently", func(t *testing.T) {
		const communityID = "comm-inv-2"
		const leaverID = "user-inv-leaver-2"

		id, err := store.Insert(ctx, &models.CommunityInvitationLink{
			CommunityId: communityID, InviterId: leaverID, ShortCode: "REVKD", IsRevoked: true,
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}

		if err := RevokeLeaverCommunityInvitationLinks(ctx, store, communityID, leaverID, deleted); err != nil {
			t.Fatalf("cascade: %v", err)
		}

		got := &models.CommunityInvitationLink{}
		if err := store.GetByID(ctx, id, got, QueryOptions{IncludeDeleted: true}); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Deleted == nil {
			t.Error("expected soft-delete")
		}
		if !got.IsRevoked {
			t.Error("expected is_revoked to be preserved")
		}
	})
}

func TestClaimCommunityOwnerHandoff(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("succeeds when current owner matches and candidate is active member", func(t *testing.T) {
		const fromUser = "user-from-1"
		const toUser = "user-to-1"
		commID, err := store.Insert(ctx, &models.Community{Name: "C1", CreatorId: fromUser, OwnerUserId: fromUser})
		if err != nil {
			t.Fatalf("insert community: %v", err)
		}
		if _, err := store.Insert(ctx, &models.CommunityUser{CommunityId: commID, UserId: fromUser}); err != nil {
			t.Fatalf("insert from membership: %v", err)
		}
		if _, err := store.Insert(ctx, &models.CommunityUser{CommunityId: commID, UserId: toUser}); err != nil {
			t.Fatalf("insert to membership: %v", err)
		}

		claimed, err := store.ClaimCommunityOwnerHandoff(ctx, commID, fromUser, toUser)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if !claimed {
			t.Fatal("expected claim to succeed")
		}

		got := &models.Community{}
		if err := store.GetByID(ctx, commID, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		// Note: only the flat column updates; binary_proto resync is the caller's responsibility.
		// We verify the swap landed by re-running the conditional UPDATE — second call should fail
		// because owner_user_id no longer matches fromUser.
		secondClaim, err := store.ClaimCommunityOwnerHandoff(ctx, commID, fromUser, toUser)
		if err != nil {
			t.Fatalf("second claim: %v", err)
		}
		if secondClaim {
			t.Error("expected second claim to fail (idempotent owner-handoff)")
		}
	})

	t.Run("fails when current owner has changed", func(t *testing.T) {
		const fromUser = "user-from-2"
		const toUser = "user-to-2"
		const actualOwner = "user-actual-2"
		commID, err := store.Insert(ctx, &models.Community{Name: "C2", CreatorId: actualOwner, OwnerUserId: actualOwner})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		if _, err := store.Insert(ctx, &models.CommunityUser{CommunityId: commID, UserId: actualOwner}); err != nil {
			t.Fatalf("insert membership: %v", err)
		}
		if _, err := store.Insert(ctx, &models.CommunityUser{CommunityId: commID, UserId: toUser}); err != nil {
			t.Fatalf("insert membership: %v", err)
		}

		claimed, err := store.ClaimCommunityOwnerHandoff(ctx, commID, fromUser, toUser)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if claimed {
			t.Error("expected claim to fail; current owner is not fromUser")
		}
	})

	t.Run("fails when candidate is not an active member", func(t *testing.T) {
		const fromUser = "user-from-3"
		const toUser = "user-to-3"
		commID, err := store.Insert(ctx, &models.Community{Name: "C3", CreatorId: fromUser, OwnerUserId: fromUser})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		if _, err := store.Insert(ctx, &models.CommunityUser{CommunityId: commID, UserId: fromUser}); err != nil {
			t.Fatalf("insert membership: %v", err)
		}
		// toUser has no CommunityUser row.

		claimed, err := store.ClaimCommunityOwnerHandoff(ctx, commID, fromUser, toUser)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if claimed {
			t.Error("expected claim to fail; candidate is not a member")
		}
	})

	t.Run("fails when candidate's membership is soft-deleted", func(t *testing.T) {
		const fromUser = "user-from-4"
		const toUser = "user-to-4"
		commID, err := store.Insert(ctx, &models.Community{Name: "C4", CreatorId: fromUser, OwnerUserId: fromUser})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		if _, err := store.Insert(ctx, &models.CommunityUser{CommunityId: commID, UserId: fromUser}); err != nil {
			t.Fatalf("insert membership: %v", err)
		}
		if _, err := store.Insert(ctx, &models.CommunityUser{
			CommunityId: commID, UserId: toUser,
			Deleted: &models.DeletedMetadata{DeletedByUserId: toUser, DeletedAtUnixSec: time.Now().Unix()},
		}); err != nil {
			t.Fatalf("insert soft-deleted membership: %v", err)
		}

		claimed, err := store.ClaimCommunityOwnerHandoff(ctx, commID, fromUser, toUser)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if claimed {
			t.Error("expected claim to fail; candidate's membership is soft-deleted")
		}
	})

	t.Run("fails when community is soft-deleted", func(t *testing.T) {
		const fromUser = "user-from-5"
		const toUser = "user-to-5"
		commID, err := store.Insert(ctx, &models.Community{
			Name: "C5", CreatorId: fromUser, OwnerUserId: fromUser,
			Deleted: &models.DeletedMetadata{DeletedByUserId: fromUser, DeletedAtUnixSec: time.Now().Unix()},
		})
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		if _, err := store.Insert(ctx, &models.CommunityUser{CommunityId: commID, UserId: fromUser}); err != nil {
			t.Fatalf("insert membership: %v", err)
		}
		if _, err := store.Insert(ctx, &models.CommunityUser{CommunityId: commID, UserId: toUser}); err != nil {
			t.Fatalf("insert membership: %v", err)
		}

		claimed, err := store.ClaimCommunityOwnerHandoff(ctx, commID, fromUser, toUser)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if claimed {
			t.Error("expected claim to fail; community is soft-deleted")
		}
	})
}

// --- helpers ---.

func assertCGDeleted(t *testing.T, store *ProtoSQLStorage, id string, want bool, label string) {
	t.Helper()
	got := &models.CommunityGear{}
	if err := store.GetByID(context.Background(), id, got, QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("%s get: %v", label, err)
	}
	deleted := got.Deleted != nil && got.Deleted.DeletedAtUnixSec > 0
	if deleted != want {
		t.Errorf("%s: deleted=%v, want %v", label, deleted, want)
	}
}

func assertCRDeleted(t *testing.T, store *ProtoSQLStorage, id string, want bool, label string) {
	t.Helper()
	got := &models.CommunityRequest{}
	if err := store.GetByID(context.Background(), id, got, QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("%s get: %v", label, err)
	}
	deleted := got.Deleted != nil && got.Deleted.DeletedAtUnixSec > 0
	if deleted != want {
		t.Errorf("%s: deleted=%v, want %v", label, deleted, want)
	}
}

func assertTransferState(t *testing.T, store *ProtoSQLStorage, id string, want models.TransferState, label string) {
	t.Helper()
	got := &models.Transfer{}
	if err := store.GetByID(context.Background(), id, got, QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("%s get: %v", label, err)
	}
	if got.State != want {
		t.Errorf("%s: state=%v, want %v", label, got.State, want)
	}
}

func assertRSVPDeleted(t *testing.T, store *ProtoSQLStorage, id string, want bool, label string) {
	t.Helper()
	got := &models.ExperienceRSVP{}
	if err := store.GetByID(context.Background(), id, got, QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("%s get: %v", label, err)
	}
	deleted := got.Deleted != nil && got.Deleted.DeletedAtUnixSec > 0
	if deleted != want {
		t.Errorf("%s: deleted=%v, want %v", label, deleted, want)
	}
}

func assertPrefsDeleted(t *testing.T, store *ProtoSQLStorage, id string, want bool, label string) {
	t.Helper()
	got := &models.CommunityNotificationPreferences{}
	if err := store.GetByID(context.Background(), id, got, QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("%s get: %v", label, err)
	}
	deleted := got.Deleted != nil && got.Deleted.DeletedAtUnixSec > 0
	if deleted != want {
		t.Errorf("%s: deleted=%v, want %v", label, deleted, want)
	}
}

func assertLinkDeleted(t *testing.T, store *ProtoSQLStorage, id string, want bool, label string) {
	t.Helper()
	got := &models.CommunityInvitationLink{}
	if err := store.GetByID(context.Background(), id, got, QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("%s get: %v", label, err)
	}
	deleted := got.Deleted != nil && got.Deleted.DeletedAtUnixSec > 0
	if deleted != want {
		t.Errorf("%s: deleted=%v, want %v", label, deleted, want)
	}
}

func TestCascadeRemoveLeaverFromChatConversationsInCommunity(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	t.Run("empty community is a noop", func(t *testing.T) {
		if err := CascadeRemoveLeaverFromChatConversationsInCommunity(ctx, store, "comm-chat-noop", "user-chat-noop"); err != nil {
			t.Fatalf("expected noop, got: %v", err)
		}
	})

	t.Run("drops leaver from gear conversation", func(t *testing.T) {
		const communityID = "comm-chat-gear-1"
		const leaverID = "user-chat-leaver-g1"
		const otherID = "user-chat-other-g1"

		gear, err := store.Insert(ctx, &models.Gear{Name: "g"})
		if err != nil {
			t.Fatalf("insert gear: %v", err)
		}
		if _, err := store.Insert(ctx, &models.CommunityGear{CommunityId: communityID, GearId: gear}); err != nil {
			t.Fatalf("insert community_gear: %v", err)
		}
		convID, err := store.Insert(ctx, &models.ChatConversation{
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gear}},
			ParticipantIds: []string{leaverID, otherID},
		})
		if err != nil {
			t.Fatalf("insert conversation: %v", err)
		}

		if err := CascadeRemoveLeaverFromChatConversationsInCommunity(ctx, store, communityID, leaverID); err != nil {
			t.Fatalf("cascade: %v", err)
		}
		assertConvParticipants(t, store, convID, []string{otherID}, "gear conversation")
	})

	t.Run("retains leaver in gear conversation when gear is also in another community leaver still belongs to", func(t *testing.T) {
		const leavingCommunityID = "comm-chat-gear-2a"
		const otherCommunityID = "comm-chat-gear-2b"
		const leaverID = "user-chat-leaver-g2"
		const otherID = "user-chat-other-g2"

		// Community B must exist in the community table for GetCommunitiesWithMembership.
		if _, err := store.Insert(ctx, &models.Community{Id: otherCommunityID, OwnerUserId: "owner-g2"}); err != nil {
			t.Fatalf("insert community B: %v", err)
		}
		// Leaver is an active member of community B.
		if _, err := store.Insert(ctx, &models.CommunityUser{CommunityId: otherCommunityID, UserId: leaverID}); err != nil {
			t.Fatalf("insert community_user: %v", err)
		}

		gear, err := store.Insert(ctx, &models.Gear{Name: "g"})
		if err != nil {
			t.Fatalf("insert gear: %v", err)
		}
		if _, err := store.Insert(ctx, &models.CommunityGear{CommunityId: leavingCommunityID, GearId: gear}); err != nil {
			t.Fatalf("insert community_gear A: %v", err)
		}
		if _, err := store.Insert(ctx, &models.CommunityGear{CommunityId: otherCommunityID, GearId: gear}); err != nil {
			t.Fatalf("insert community_gear B: %v", err)
		}
		convID, err := store.Insert(ctx, &models.ChatConversation{
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gear}},
			ParticipantIds: []string{leaverID, otherID},
		})
		if err != nil {
			t.Fatalf("insert conversation: %v", err)
		}

		if err := CascadeRemoveLeaverFromChatConversationsInCommunity(ctx, store, leavingCommunityID, leaverID); err != nil {
			t.Fatalf("cascade: %v", err)
		}
		// Leaver still has active membership in community B: must remain.
		assertConvParticipants(t, store, convID, []string{leaverID, otherID}, "gear conversation cross-community")
	})

	t.Run("drops leaver from experience conversation", func(t *testing.T) {
		const communityID = "comm-chat-exp-1"
		const leaverID = "user-chat-leaver-e1"
		const otherID = "user-chat-other-e1"

		exp, err := store.Insert(ctx, &models.Experience{Name: "hike"})
		if err != nil {
			t.Fatalf("insert experience: %v", err)
		}
		if _, err := store.Insert(ctx, &models.CommunityExperience{CommunityId: communityID, ExperienceId: exp}); err != nil {
			t.Fatalf("insert community_experience: %v", err)
		}
		convID, err := store.Insert(ctx, &models.ChatConversation{
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_ExperienceId{ExperienceId: exp}},
			ParticipantIds: []string{leaverID, otherID},
		})
		if err != nil {
			t.Fatalf("insert conversation: %v", err)
		}

		if err := CascadeRemoveLeaverFromChatConversationsInCommunity(ctx, store, communityID, leaverID); err != nil {
			t.Fatalf("cascade: %v", err)
		}
		assertConvParticipants(t, store, convID, []string{otherID}, "experience conversation")
	})

	t.Run("drops leaver from request conversation", func(t *testing.T) {
		const communityID = "comm-chat-req-1"
		const leaverID = "user-chat-leaver-r1"
		const otherID = "user-chat-other-r1"

		req, err := store.Insert(ctx, &models.Request{Title: "need thing", RequesterId: leaverID})
		if err != nil {
			t.Fatalf("insert request: %v", err)
		}
		if _, err := store.Insert(ctx, &models.CommunityRequest{CommunityId: communityID, RequestId: req}); err != nil {
			t.Fatalf("insert community_request: %v", err)
		}
		convID, err := store.Insert(ctx, &models.ChatConversation{
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_RequestId{RequestId: req}},
			ParticipantIds: []string{leaverID, otherID},
		})
		if err != nil {
			t.Fatalf("insert conversation: %v", err)
		}

		if err := CascadeRemoveLeaverFromChatConversationsInCommunity(ctx, store, communityID, leaverID); err != nil {
			t.Fatalf("cascade: %v", err)
		}
		assertConvParticipants(t, store, convID, []string{otherID}, "request conversation")
	})

	t.Run("drops leaver from community-wide conversation", func(t *testing.T) {
		const communityID = "comm-chat-wide-1"
		const leaverID = "user-chat-leaver-w1"
		const otherID = "user-chat-other-w1"

		convID, err := store.Insert(ctx, &models.ChatConversation{
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_CommunityId{CommunityId: communityID}},
			ParticipantIds: []string{leaverID, otherID},
		})
		if err != nil {
			t.Fatalf("insert conversation: %v", err)
		}

		if err := CascadeRemoveLeaverFromChatConversationsInCommunity(ctx, store, communityID, leaverID); err != nil {
			t.Fatalf("cascade: %v", err)
		}
		assertConvParticipants(t, store, convID, []string{otherID}, "community-wide conversation")
	})

	t.Run("leaves transfer-topic conversation untouched", func(t *testing.T) {
		const communityID = "comm-chat-tx-1"
		const leaverID = "user-chat-leaver-tx1"
		const otherID = "user-chat-other-tx1"

		gear, err := store.Insert(ctx, &models.Gear{Name: "g"})
		if err != nil {
			t.Fatalf("insert gear: %v", err)
		}
		if _, err := store.Insert(ctx, &models.CommunityGear{CommunityId: communityID, GearId: gear}); err != nil {
			t.Fatalf("insert community_gear: %v", err)
		}
		// Transfer conversation: topic_transfer_id is set, not topic_gear_id.
		convID, err := store.Insert(ctx, &models.ChatConversation{
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_TransferId{TransferId: "transfer-tx1"}},
			ParticipantIds: []string{leaverID, otherID},
		})
		if err != nil {
			t.Fatalf("insert transfer conversation: %v", err)
		}

		if err := CascadeRemoveLeaverFromChatConversationsInCommunity(ctx, store, communityID, leaverID); err != nil {
			t.Fatalf("cascade: %v", err)
		}
		// Transfer conversations are never added to the candidate set.
		assertConvParticipants(t, store, convID, []string{leaverID, otherID}, "transfer conversation untouched")
	})

	t.Run("idempotent: re-running after leaver already stripped is a noop", func(t *testing.T) {
		const communityID = "comm-chat-idem-1"
		const otherID = "user-chat-other-idem1"
		const leaverID = "user-chat-leaver-idem1"

		convID, err := store.Insert(ctx, &models.ChatConversation{
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_CommunityId{CommunityId: communityID}},
			ParticipantIds: []string{otherID},
		})
		if err != nil {
			t.Fatalf("insert conversation: %v", err)
		}

		if err := CascadeRemoveLeaverFromChatConversationsInCommunity(ctx, store, communityID, leaverID); err != nil {
			t.Fatalf("first run: %v", err)
		}
		if err := CascadeRemoveLeaverFromChatConversationsInCommunity(ctx, store, communityID, leaverID); err != nil {
			t.Fatalf("second run: %v", err)
		}
		assertConvParticipants(t, store, convID, []string{otherID}, "idempotent")
	})

	t.Run("leaves non-leaver participants untouched", func(t *testing.T) {
		const communityID = "comm-chat-other-1"
		const leaverID = "user-chat-leaver-o1"
		const user2ID = "user-chat-u2-o1"
		const user3ID = "user-chat-u3-o1"

		convID, err := store.Insert(ctx, &models.ChatConversation{
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_CommunityId{CommunityId: communityID}},
			ParticipantIds: []string{leaverID, user2ID, user3ID},
		})
		if err != nil {
			t.Fatalf("insert conversation: %v", err)
		}

		if err := CascadeRemoveLeaverFromChatConversationsInCommunity(ctx, store, communityID, leaverID); err != nil {
			t.Fatalf("cascade: %v", err)
		}
		assertConvParticipants(t, store, convID, []string{user2ID, user3ID}, "non-leaver participants")
	})
}

// TestCascadeRemoveLeaverFromChatConversationsInCommunity_QueryBudget verifies
// that the helper issues O(1) SQL queries regardless of fixture size.
func TestCascadeRemoveLeaverFromChatConversationsInCommunity_QueryBudget(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	const communityID = "comm-chat-budget-1"
	const leaverID = "user-chat-budget-leaver"
	const otherID = "user-chat-budget-other"

	// One gear, experience, request — each with a conversation.
	gear, err := store.Insert(ctx, &models.Gear{Name: "g"})
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityGear{CommunityId: communityID, GearId: gear}); err != nil {
		t.Fatalf("insert community_gear: %v", err)
	}
	exp, err := store.Insert(ctx, &models.Experience{Name: "event"})
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityExperience{CommunityId: communityID, ExperienceId: exp}); err != nil {
		t.Fatalf("insert community_experience: %v", err)
	}
	req, err := store.Insert(ctx, &models.Request{Title: "need", RequesterId: leaverID})
	if err != nil {
		t.Fatalf("insert request: %v", err)
	}
	if _, err := store.Insert(ctx, &models.CommunityRequest{CommunityId: communityID, RequestId: req}); err != nil {
		t.Fatalf("insert community_request: %v", err)
	}
	for _, conv := range []*models.ChatConversation{
		{
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gear}},
			ParticipantIds: []string{leaverID, otherID},
		},
		{
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_ExperienceId{ExperienceId: exp}},
			ParticipantIds: []string{leaverID, otherID},
		},
		{
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_RequestId{RequestId: req}},
			ParticipantIds: []string{leaverID, otherID},
		},
		{
			Topic:          &models.ConversationTopic{TopicId: &models.ConversationTopic_CommunityId{CommunityId: communityID}},
			ParticipantIds: []string{leaverID, otherID},
		},
	} {
		if _, err := store.Insert(ctx, conv); err != nil {
			t.Fatalf("insert conversation: %v", err)
		}
	}

	statsCtx := WithQueryStats(ctx)
	// Budget: 3 (step-1 community join-table scans) + 4 (step-2 conversation
	// fetches) + 3 (step-3 fan-out join-table scans) + 0 (no other communities,
	// GetCommunitiesWithMembership skips query on empty input) + 8 (step-5
	// updates: each Update wraps two SQL statements — the main UPDATE and the
	// participant_ids array-column sync — so 4 conversations × 2 = 8) = 18 total.
	AssertMaxQueries(t, statsCtx, 18, func() {
		if err := CascadeRemoveLeaverFromChatConversationsInCommunity(statsCtx, store, communityID, leaverID); err != nil {
			t.Fatalf("cascade: %v", err)
		}
	})
}

func assertConvParticipants(t *testing.T, store *ProtoSQLStorage, id string, want []string, label string) {
	t.Helper()
	got := &models.ChatConversation{}
	if err := store.GetByID(context.Background(), id, got); err != nil {
		t.Fatalf("%s get: %v", label, err)
	}
	gotSorted := append([]string(nil), got.ParticipantIds...)
	wantSorted := append([]string(nil), want...)
	sort.Strings(gotSorted)
	sort.Strings(wantSorted)
	if len(gotSorted) != len(wantSorted) {
		t.Errorf("%s: participants=%v, want %v", label, got.ParticipantIds, want)
		return
	}
	for i := range gotSorted {
		if gotSorted[i] != wantSorted[i] {
			t.Errorf("%s: participants=%v, want %v", label, got.ParticipantIds, want)
			return
		}
	}
}
