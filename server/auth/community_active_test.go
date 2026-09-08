package auth

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestRequireMemberOfActiveCommunity(t *testing.T) {
	ctx := context.Background()
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := uuid.New().String()
	memberID := uuid.New().String()
	nonMemberID := uuid.New().String()

	community := &models.Community{
		Id:          communityID,
		Name:        "Test Community",
		CreatorId:   memberID,
		OwnerUserId: memberID,
	}
	if _, err := s.Insert(ctx, community); err != nil {
		t.Fatalf("insert community: %v", err)
	}
	membership := &models.CommunityUser{
		Id:          uuid.New().String(),
		CommunityId: communityID,
		UserId:      memberID,
	}
	if _, err := s.Insert(ctx, membership); err != nil {
		t.Fatalf("insert membership: %v", err)
	}

	t.Run("active community + member returns nil error", func(t *testing.T) {
		c, m, err := RequireMemberOfActiveCommunity(ctx, s, communityID, memberID)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if c == nil || c.Id != communityID {
			t.Fatalf("expected community, got %+v", c)
		}
		if m == nil || m.UserId != memberID {
			t.Fatalf("expected membership, got %+v", m)
		}
	})

	t.Run("active community + non-member returns PermissionDenied", func(t *testing.T) {
		_, _, err := RequireMemberOfActiveCommunity(ctx, s, communityID, nonMemberID)
		if err == nil {
			t.Fatalf("expected error")
		}
		if got := connect.CodeOf(err); got != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %s", got)
		}
	})

	t.Run("missing community returns NotFound", func(t *testing.T) {
		_, _, err := RequireMemberOfActiveCommunity(ctx, s, uuid.New().String(), memberID)
		if err == nil {
			t.Fatalf("expected error")
		}
		if got := connect.CodeOf(err); got != connect.CodeNotFound {
			t.Fatalf("expected NotFound, got %s", got)
		}
	})

	t.Run("soft-deleted community + member returns FailedPrecondition", func(t *testing.T) {
		// Force-flip to soft-deleted.
		community.Deleted = &models.DeletedMetadata{
			DeletedByUserId:  memberID,
			DeletedAtUnixSec: 1700000000,
		}
		if err := s.Update(ctx, community); err != nil {
			t.Fatalf("update: %v", err)
		}
		t.Cleanup(func() {
			community.Deleted = nil
			_ = s.Update(ctx, community)
		})

		_, _, err := RequireMemberOfActiveCommunity(ctx, s, communityID, memberID)
		if err == nil {
			t.Fatalf("expected error")
		}
		if got := connect.CodeOf(err); got != connect.CodeFailedPrecondition {
			t.Fatalf("expected FailedPrecondition, got %s", got)
		}
	})

	t.Run("soft-deleted community + non-member returns NotFound (no info leak)", func(t *testing.T) {
		community.Deleted = &models.DeletedMetadata{
			DeletedByUserId:  memberID,
			DeletedAtUnixSec: 1700000000,
		}
		if err := s.Update(ctx, community); err != nil {
			t.Fatalf("update: %v", err)
		}
		t.Cleanup(func() {
			community.Deleted = nil
			_ = s.Update(ctx, community)
		})

		_, _, err := RequireMemberOfActiveCommunity(ctx, s, communityID, nonMemberID)
		if err == nil {
			t.Fatalf("expected error")
		}
		if got := connect.CodeOf(err); got != connect.CodeNotFound {
			t.Fatalf("expected NotFound, got %s", got)
		}
	})
}

func TestRequireMemberOfActiveCommunity_SingleRoundTrip(t *testing.T) {
	ctx := storage.WithQueryStats(context.Background())
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := uuid.New().String()
	userID := uuid.New().String()

	if _, err := s.Insert(ctx, &models.Community{Id: communityID, Name: "x", CreatorId: userID, OwnerUserId: userID}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := s.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: userID,
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	storage.AssertMaxQueries(t, ctx, 1, func() {
		if _, _, err := RequireMemberOfActiveCommunity(ctx, s, communityID, userID); err != nil {
			t.Fatalf("authorize: %v", err)
		}
	})
}

func TestRequireMemberOfActiveCommunity_Memoization(t *testing.T) {
	rawCtx := storage.WithQueryStats(context.Background())
	ctx := WithCommunityCache(rawCtx)
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := uuid.New().String()
	userID := uuid.New().String()

	if _, err := s.Insert(ctx, &models.Community{Id: communityID, Name: "x", CreatorId: userID, OwnerUserId: userID}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := s.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: userID,
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// First call costs one query.
	if _, _, err := RequireMemberOfActiveCommunity(ctx, s, communityID, userID); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Second call must hit the cache and issue zero queries.
	storage.AssertMaxQueries(t, ctx, 0, func() {
		if _, _, err := RequireMemberOfActiveCommunity(ctx, s, communityID, userID); err != nil {
			t.Fatalf("second call: %v", err)
		}
	})
}

func TestWithCommunityCache_Idempotent(t *testing.T) {
	ctx1 := WithCommunityCache(context.Background())
	ctx2 := WithCommunityCache(ctx1)
	if ctx1.Value(communityCacheCtxKey{}) != ctx2.Value(communityCacheCtxKey{}) {
		t.Fatalf("WithCommunityCache should be idempotent — wrapping twice must reuse the same cache")
	}
}

func TestFilterActiveMemberCommunities(t *testing.T) {
	ctx := context.Background()
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	memberID := uuid.New().String()
	otherID := uuid.New().String()

	// Three communities: c1 active+member, c2 active+non-member, c3 deleted+member.
	c1 := &models.Community{Id: uuid.New().String(), Name: "C1", CreatorId: memberID, OwnerUserId: memberID}
	c2 := &models.Community{Id: uuid.New().String(), Name: "C2", CreatorId: otherID, OwnerUserId: otherID}
	c3 := &models.Community{
		Id: uuid.New().String(), Name: "C3", CreatorId: memberID,
		Deleted: &models.DeletedMetadata{DeletedByUserId: memberID, DeletedAtUnixSec: 1700000000},
	}
	for _, c := range []*models.Community{c1, c2, c3} {
		if _, err := s.Insert(ctx, c); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	if _, err := s.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: c1.Id, UserId: memberID,
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := s.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: c3.Id, UserId: memberID,
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	t.Run("active member-only list passes through", func(t *testing.T) {
		active, resolved, err := FilterActiveMemberCommunities(ctx, s, []string{c1.Id}, memberID)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(active) != 1 || active[0] != c1.Id {
			t.Fatalf("expected [c1], got %v", active)
		}
		if resolved[c1.Id] == nil {
			t.Fatalf("expected resolved community for c1")
		}
	})

	t.Run("missing community silently dropped", func(t *testing.T) {
		missingID := uuid.New().String()
		active, _, err := FilterActiveMemberCommunities(ctx, s, []string{c1.Id, missingID}, memberID)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(active) != 1 {
			t.Fatalf("expected [c1], got %v", active)
		}
	})

	t.Run("soft-deleted community silently dropped, even when caller is a member", func(t *testing.T) {
		active, _, err := FilterActiveMemberCommunities(ctx, s, []string{c1.Id, c3.Id}, memberID)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(active) != 1 || active[0] != c1.Id {
			t.Fatalf("expected [c1], got %v", active)
		}
	})

	t.Run("non-member of an active community is a hard error", func(t *testing.T) {
		_, _, err := FilterActiveMemberCommunities(ctx, s, []string{c2.Id}, memberID)
		if err == nil {
			t.Fatalf("expected PermissionDenied, got nil")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Fatalf("expected PermissionDenied, got %s", connect.CodeOf(err))
		}
	})

	t.Run("single round-trip for N communities", func(t *testing.T) {
		statsCtx := storage.WithQueryStats(ctx)
		storage.AssertMaxQueries(t, statsCtx, 1, func() {
			if _, _, err := FilterActiveMemberCommunities(statsCtx, s, []string{c1.Id, c3.Id}, memberID); err != nil {
				t.Fatalf("err: %v", err)
			}
		})
	})
}

func TestRequireActiveCommunity(t *testing.T) {
	ctx := context.Background()
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := uuid.New().String()
	creatorID := uuid.New().String()
	community := &models.Community{
		Id:          communityID,
		Name:        "Test Community",
		CreatorId:   creatorID,
		OwnerUserId: creatorID,
	}
	if _, err := s.Insert(ctx, community); err != nil {
		t.Fatalf("insert: %v", err)
	}

	t.Run("active community returns ok", func(t *testing.T) {
		c, err := RequireActiveCommunity(ctx, s, communityID)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if c == nil || c.Id != communityID {
			t.Fatalf("expected community, got %+v", c)
		}
	})

	t.Run("missing community returns NotFound", func(t *testing.T) {
		_, err := RequireActiveCommunity(ctx, s, uuid.New().String())
		if err == nil {
			t.Fatalf("expected error")
		}
		if got := connect.CodeOf(err); got != connect.CodeNotFound {
			t.Fatalf("expected NotFound, got %s", got)
		}
	})

	t.Run("soft-deleted community returns NotFound", func(t *testing.T) {
		community.Deleted = &models.DeletedMetadata{
			DeletedByUserId:  uuid.New().String(),
			DeletedAtUnixSec: 1700000000,
		}
		if err := s.Update(ctx, community); err != nil {
			t.Fatalf("update: %v", err)
		}
		t.Cleanup(func() {
			community.Deleted = nil
			_ = s.Update(ctx, community)
		})

		_, err := RequireActiveCommunity(ctx, s, communityID)
		if err == nil {
			t.Fatalf("expected error")
		}
		if got := connect.CodeOf(err); got != connect.CodeNotFound {
			t.Fatalf("expected NotFound, got %s", got)
		}
	})
}
