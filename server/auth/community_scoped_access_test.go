package auth

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// seedCommunity creates a community and returns its ID.
func seedCommunity(t *testing.T, ctx context.Context, s *storage.ProtoSQLStorage, ownerID string) string {
	t.Helper()
	c := &models.Community{
		Id:          uuid.New().String(),
		Name:        "Test Community",
		CreatorId:   ownerID,
		OwnerUserId: ownerID,
	}
	if _, err := s.Insert(ctx, c); err != nil {
		t.Fatalf("insert community: %v", err)
	}
	return c.Id
}

// seedMembership creates an active CommunityUser row.
func seedMembership(t *testing.T, ctx context.Context, s *storage.ProtoSQLStorage, communityID, userID string) {
	t.Helper()
	m := &models.CommunityUser{
		Id:          uuid.New().String(),
		CommunityId: communityID,
		UserId:      userID,
	}
	if _, err := s.Insert(ctx, m); err != nil {
		t.Fatalf("insert membership: %v", err)
	}
}

// seedGearShared creates a Gear and a CommunityGear row, returning the gear ID.
func seedGearShared(t *testing.T, ctx context.Context, s *storage.ProtoSQLStorage, ownerID, communityID string) string {
	t.Helper()
	g := &models.Gear{
		Id:      uuid.New().String(),
		OwnerId: ownerID,
		Name:    "Test Gear",
	}
	if _, err := s.Insert(ctx, g); err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	cg := &models.CommunityGear{
		Id:          uuid.New().String(),
		GearId:      g.Id,
		CommunityId: communityID,
	}
	if _, err := s.Insert(ctx, cg); err != nil {
		t.Fatalf("insert community gear: %v", err)
	}
	return g.Id
}

// seedExperienceShared creates an Experience and a CommunityExperience row, returning the experience ID.
func seedExperienceShared(t *testing.T, ctx context.Context, s *storage.ProtoSQLStorage, ownerID, communityID string) string {
	t.Helper()
	e := &models.Experience{
		Id:      uuid.New().String(),
		OwnerId: ownerID,
		Name:    "Test Experience",
	}
	if _, err := s.Insert(ctx, e); err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	ce := &models.CommunityExperience{
		Id:           uuid.New().String(),
		ExperienceId: e.Id,
		CommunityId:  communityID,
	}
	if _, err := s.Insert(ctx, ce); err != nil {
		t.Fatalf("insert community experience: %v", err)
	}
	return e.Id
}

func TestRequireAccessToCommunityScopedEntity(t *testing.T) {
	ctx := context.Background()
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	ownerID := uuid.New().String()
	memberID := uuid.New().String()
	nonMemberID := uuid.New().String()
	communityID := seedCommunity(t, ctx, s, ownerID)

	seedMembership(t, ctx, s, communityID, ownerID)
	seedMembership(t, ctx, s, communityID, memberID)

	gearID := seedGearShared(t, ctx, s, ownerID, communityID)
	expID := seedExperienceShared(t, ctx, s, ownerID, communityID)

	t.Run("member can read shared experience", func(t *testing.T) {
		shared, caller, err := RequireAccessToCommunityScopedEntity(ctx, s, memberID, EntityExperience, expID, ownerID)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(shared) != 1 || shared[0] != communityID {
			t.Fatalf("unexpected sharedCommunityIDs: %v", shared)
		}
		if len(caller) != 1 || caller[0] != communityID {
			t.Fatalf("unexpected callerCommunityIDs: %v", caller)
		}
	})

	t.Run("non-member cannot read shared experience", func(t *testing.T) {
		_, _, err := RequireAccessToCommunityScopedEntity(ctx, s, nonMemberID, EntityExperience, expID, ownerID)
		if err == nil {
			t.Fatal("expected PermissionDenied, got nil")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %s", connect.CodeOf(err))
		}
	})

	t.Run("owner can read own experience shared nowhere", func(t *testing.T) {
		// Create an unshared experience.
		unshared := &models.Experience{
			Id:      uuid.New().String(),
			OwnerId: ownerID,
			Name:    "Unshared Experience",
		}
		if _, err := s.Insert(ctx, unshared); err != nil {
			t.Fatalf("insert unshared experience: %v", err)
		}
		shared, caller, err := RequireAccessToCommunityScopedEntity(ctx, s, ownerID, EntityExperience, unshared.Id, ownerID)
		if err != nil {
			t.Fatalf("owner should be able to read own unshared experience, got %v", err)
		}
		if len(shared) != 0 {
			t.Fatalf("expected no shared communities, got %v", shared)
		}
		if len(caller) != 0 {
			t.Fatalf("expected no caller communities, got %v", caller)
		}
	})

	t.Run("ex-member (soft-deleted membership) cannot read", func(t *testing.T) {
		exMemberID := uuid.New().String()
		// Insert membership then soft-delete it.
		m := &models.CommunityUser{
			Id:          uuid.New().String(),
			CommunityId: communityID,
			UserId:      exMemberID,
		}
		if _, err := s.Insert(ctx, m); err != nil {
			t.Fatalf("insert ex-member membership: %v", err)
		}
		m.Deleted = &models.DeletedMetadata{
			DeletedByUserId:  exMemberID,
			DeletedAtUnixSec: 1700000000,
		}
		if err := s.Update(ctx, m); err != nil {
			t.Fatalf("soft-delete ex-member membership: %v", err)
		}

		_, _, err := RequireAccessToCommunityScopedEntity(ctx, s, exMemberID, EntityExperience, expID, ownerID)
		if err == nil {
			t.Fatal("expected PermissionDenied for ex-member, got nil")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %s", connect.CodeOf(err))
		}
	})

	t.Run("entity shared with soft-deleted community denies access", func(t *testing.T) {
		// Create a separate community and delete it.
		deletedOwnerID := uuid.New().String()
		deletedCommunityID := seedCommunity(t, ctx, s, deletedOwnerID)
		deletedMemberID := uuid.New().String()
		seedMembership(t, ctx, s, deletedCommunityID, deletedMemberID)

		// Share an experience with the deleted community.
		deletedExpID := seedExperienceShared(t, ctx, s, deletedOwnerID, deletedCommunityID)

		// Soft-delete the community.
		c := &models.Community{
			Id:          deletedCommunityID,
			Name:        "Deleted Community",
			CreatorId:   deletedOwnerID,
			OwnerUserId: deletedOwnerID,
		}
		if err := s.GetByID(ctx, deletedCommunityID, c); err != nil {
			t.Fatalf("fetch community: %v", err)
		}
		c.Deleted = &models.DeletedMetadata{
			DeletedByUserId:  deletedOwnerID,
			DeletedAtUnixSec: 1700000000,
		}
		if err := s.Update(ctx, c); err != nil {
			t.Fatalf("soft-delete community: %v", err)
		}

		_, _, err := RequireAccessToCommunityScopedEntity(ctx, s, deletedMemberID, EntityExperience, deletedExpID, deletedOwnerID)
		if err == nil {
			t.Fatal("expected PermissionDenied for member of soft-deleted community, got nil")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %s", connect.CodeOf(err))
		}
	})

	t.Run("caller in community A but not B sees only A in callerCommunityIDs", func(t *testing.T) {
		partialMemberID := uuid.New().String()
		communityBID := seedCommunity(t, ctx, s, ownerID)
		seedMembership(t, ctx, s, communityID, partialMemberID)
		// partialMember is NOT in communityB.

		// Share an experience with both communities.
		multiExp := &models.Experience{
			Id:      uuid.New().String(),
			OwnerId: ownerID,
			Name:    "Multi-community Experience",
		}
		if _, err := s.Insert(ctx, multiExp); err != nil {
			t.Fatalf("insert experience: %v", err)
		}
		ceA := &models.CommunityExperience{
			Id:           uuid.New().String(),
			ExperienceId: multiExp.Id,
			CommunityId:  communityID,
		}
		ceB := &models.CommunityExperience{
			Id:           uuid.New().String(),
			ExperienceId: multiExp.Id,
			CommunityId:  communityBID,
		}
		if _, err := s.Insert(ctx, ceA); err != nil {
			t.Fatalf("insert ceA: %v", err)
		}
		if _, err := s.Insert(ctx, ceB); err != nil {
			t.Fatalf("insert ceB: %v", err)
		}

		shared, caller, err := RequireAccessToCommunityScopedEntity(ctx, s, partialMemberID, EntityExperience, multiExp.Id, ownerID)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(shared) != 2 {
			t.Fatalf("expected 2 shared communities, got %v", shared)
		}
		if len(caller) != 1 || caller[0] != communityID {
			t.Fatalf("expected only communityA in callerCommunityIDs, got %v", caller)
		}
	})

	t.Run("gear kind works", func(t *testing.T) {
		_, _, err := RequireAccessToCommunityScopedEntity(ctx, s, memberID, EntityGear, gearID, ownerID)
		if err != nil {
			t.Fatalf("member should be able to read shared gear, got %v", err)
		}
		_, _, err = RequireAccessToCommunityScopedEntity(ctx, s, nonMemberID, EntityGear, gearID, ownerID)
		if err == nil {
			t.Fatal("non-member should not be able to read gear")
		}
	})

	t.Run("archived gear share: strict gate denies, read gate allows (#2695)", func(t *testing.T) {
		// A completed giveaway archives every CommunityGear row. The item
		// must stay READABLE to members of those communities, while the
		// strict gate (mutations) treats it as no longer shared.
		archivedGear := &models.Gear{
			Id:      uuid.New().String(),
			OwnerId: ownerID,
			Name:    "Given-Away Gear",
		}
		if _, err := s.Insert(ctx, archivedGear); err != nil {
			t.Fatalf("insert gear: %v", err)
		}
		cg := &models.CommunityGear{
			Id:          uuid.New().String(),
			GearId:      archivedGear.Id,
			CommunityId: communityID,
			Archived:    true,
		}
		if _, err := s.Insert(ctx, cg); err != nil {
			t.Fatalf("insert archived community gear: %v", err)
		}

		_, _, err := RequireAccessToCommunityScopedEntity(ctx, s, memberID, EntityGear, archivedGear.Id, ownerID)
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("strict gate: expected PermissionDenied for archived-only share, got %v", err)
		}

		shared, caller, err := RequireReadAccessToCommunityScopedEntity(ctx, s, memberID, EntityGear, archivedGear.Id, ownerID)
		if err != nil {
			t.Fatalf("read gate: member should read gear with archived share, got %v", err)
		}
		if len(shared) != 1 || shared[0] != communityID {
			t.Fatalf("read gate sharedCommunityIDs = %v, want [%s]", shared, communityID)
		}
		if len(caller) != 1 || caller[0] != communityID {
			t.Fatalf("read gate callerCommunityIDs = %v, want [%s]", caller, communityID)
		}

		// Non-members stay denied on both gates.
		_, _, err = RequireReadAccessToCommunityScopedEntity(ctx, s, nonMemberID, EntityGear, archivedGear.Id, ownerID)
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("read gate: non-member should stay denied, got %v", err)
		}

		// The strict and read gates must not share a cache entry.
		cachedCtx := WithCommunityCache(ctx)
		if _, _, err := RequireReadAccessToCommunityScopedEntity(cachedCtx, s, memberID, EntityGear, archivedGear.Id, ownerID); err != nil {
			t.Fatalf("read gate (cached ctx): %v", err)
		}
		if _, _, err := RequireAccessToCommunityScopedEntity(cachedCtx, s, memberID, EntityGear, archivedGear.Id, ownerID); connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("strict gate must not reuse the read gate's cached community set, got %v", err)
		}
	})

	t.Run("composes with WithCommunityCache: second call costs one fewer query", func(t *testing.T) {
		cachedCtx := WithCommunityCache(storage.WithQueryStats(ctx))

		// First call: QueryByField for entity community IDs + GetCommunitiesWithMembership = 2 queries.
		storage.AssertMaxQueries(t, cachedCtx, 2, func() {
			_, _, err := RequireAccessToCommunityScopedEntity(cachedCtx, s, memberID, EntityExperience, expID, ownerID)
			if err != nil {
				t.Fatalf("first call: %v", err)
			}
		})

		// Second call with same entityID/kind reuses the cached entity-community mapping,
		// so only GetCommunitiesWithMembership runs = 1 query.
		storage.AssertMaxQueries(t, cachedCtx, 1, func() {
			_, _, err := RequireAccessToCommunityScopedEntity(cachedCtx, s, memberID, EntityExperience, expID, ownerID)
			if err != nil {
				t.Fatalf("second call: %v", err)
			}
		})
	})
}

func TestRequireSharedCommunityWithUser(t *testing.T) {
	ctx := context.Background()
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	ownerID := uuid.New().String()
	communityID := seedCommunity(t, ctx, s, ownerID)

	userA := uuid.New().String()
	userB := uuid.New().String()
	userC := uuid.New().String() // shares no community with A or B

	seedMembership(t, ctx, s, communityID, userA)
	seedMembership(t, ctx, s, communityID, userB)

	t.Run("self-bypass: callerID == targetID returns nil", func(t *testing.T) {
		if err := RequireSharedCommunityWithUser(ctx, s, userA, userA); err != nil {
			t.Fatalf("expected nil for self-bypass, got %v", err)
		}
	})

	t.Run("shared community allows access", func(t *testing.T) {
		if err := RequireSharedCommunityWithUser(ctx, s, userA, userB); err != nil {
			t.Fatalf("expected nil for shared community, got %v", err)
		}
	})

	t.Run("disjoint membership returns PermissionDenied", func(t *testing.T) {
		err := RequireSharedCommunityWithUser(ctx, s, userA, userC)
		if err == nil {
			t.Fatal("expected PermissionDenied for disjoint membership")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %s", connect.CodeOf(err))
		}
	})

	t.Run("target with no memberships returns PermissionDenied", func(t *testing.T) {
		newUser := uuid.New().String()
		err := RequireSharedCommunityWithUser(ctx, s, userA, newUser)
		if err == nil {
			t.Fatal("expected PermissionDenied for target with no memberships")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %s", connect.CodeOf(err))
		}
	})

	t.Run("ex-member (soft-deleted) treated as non-member", func(t *testing.T) {
		exMemberID := uuid.New().String()
		m := &models.CommunityUser{
			Id:          uuid.New().String(),
			CommunityId: communityID,
			UserId:      exMemberID,
		}
		if _, err := s.Insert(ctx, m); err != nil {
			t.Fatalf("insert ex-member: %v", err)
		}
		m.Deleted = &models.DeletedMetadata{
			DeletedByUserId:  exMemberID,
			DeletedAtUnixSec: 1700000000,
		}
		if err := s.Update(ctx, m); err != nil {
			t.Fatalf("soft-delete ex-member: %v", err)
		}

		err := RequireSharedCommunityWithUser(ctx, s, userA, exMemberID)
		if err == nil {
			t.Fatal("expected PermissionDenied for ex-member")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %s", connect.CodeOf(err))
		}
	})
}
