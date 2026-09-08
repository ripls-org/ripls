package search

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// setupTestCommunityJoinedAt mirrors setupTestCommunity but lets the
// caller pin the membership's CreatedAtUnixSec. Required for ordering
// assertions because Time.Now().Unix() resolution can collapse
// multiple inserts inside the same second.
func setupTestCommunityJoinedAt(
	t *testing.T,
	s *storage.ProtoSQLStorage,
	creatorID, name string,
	joinedAt int64,
) string {
	t.Helper()
	community := &models.Community{
		Name:             name,
		Description:      "Test community",
		CreatorId:        creatorID,
		OwnerUserId:      creatorID,
		CreatedAtUnixSec: joinedAt,
		UpdatedAtUnixSec: joinedAt,
	}
	id, err := s.Insert(context.Background(), community)
	if err != nil {
		t.Fatalf("Failed to create test community: %v", err)
	}
	membership := &models.CommunityUser{
		CommunityId:      id,
		UserId:           creatorID,
		InviterId:        creatorID,
		CreatedAtUnixSec: joinedAt,
	}
	if _, err := s.Insert(context.Background(), membership); err != nil {
		t.Fatalf("Failed to add creator to community: %v", err)
	}
	return id
}

// TestGetSearchSuggestions_NoPortfolio covers a brand-new user whose
// only signal is the bare membership row created by setupTestCommunity.
// We expect no categories, exactly one community.
func TestGetSearchSuggestions_NoPortfolio(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := New(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "owner@test.com", "Owner")
	setupTestCommunity(t, sqlStorage, userID, "Solo Community")

	ctx := createAuthenticatedContext(userID, "owner@test.com")
	resp, err := service.GetSearchSuggestions(
		ctx,
		connect.NewRequest(&api.GetSearchSuggestionsRequest{}),
	)
	if err != nil {
		t.Fatalf("GetSearchSuggestions failed: %v", err)
	}

	if len(resp.Msg.TopKnownForCategories) != 0 {
		t.Errorf("expected no known-for categories, got %v", resp.Msg.TopKnownForCategories)
	}
	if len(resp.Msg.TopCommunities) != 1 {
		t.Fatalf("expected exactly one community, got %d", len(resp.Msg.TopCommunities))
	}
	if resp.Msg.TopCommunities[0].Name != "Solo Community" {
		t.Errorf("expected community 'Solo Community', got %q", resp.Msg.TopCommunities[0].Name)
	}
}

// TestGetSearchSuggestions_AllCommunitiesReturned verifies that every
// active community membership the caller has surfaces in the response,
// most-recently-joined first. The community list is uncapped — the
// carousel UI lets users scope a search to any of their communities.
func TestGetSearchSuggestions_AllCommunitiesReturned(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := New(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "owner@test.com", "Owner")
	base := time.Now().Unix() - 1000

	// Seven communities, each joined at base+i so order is fully
	// deterministic regardless of insert timing.
	for i := 0; i < 7; i++ {
		setupTestCommunityJoinedAt(
			t, sqlStorage, userID, fmt.Sprintf("Community %d", i), base+int64(i),
		)
	}

	ctx := createAuthenticatedContext(userID, "owner@test.com")
	resp, err := service.GetSearchSuggestions(
		ctx,
		connect.NewRequest(&api.GetSearchSuggestionsRequest{}),
	)
	if err != nil {
		t.Fatalf("GetSearchSuggestions failed: %v", err)
	}

	if got := len(resp.Msg.TopCommunities); got != 7 {
		t.Fatalf("expected all 7 communities, got %d", got)
	}
	wantNames := []string{
		"Community 6", "Community 5", "Community 4", "Community 3",
		"Community 2", "Community 1", "Community 0",
	}
	for i, want := range wantNames {
		if got := resp.Msg.TopCommunities[i].Name; got != want {
			t.Errorf("position %d: want %q, got %q", i, want, got)
		}
	}
}

// TestGetSearchSuggestions_IgnoresOtherUsersCommunities verifies the
// server scopes suggestions to the *authenticated* user's memberships
// — communities the caller does not belong to never appear, regardless
// of who else is a member.
func TestGetSearchSuggestions_IgnoresOtherUsersCommunities(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := New(sqlStorage)

	userA := setupTestUser(t, sqlStorage, "a@test.com", "Alice")
	userB := setupTestUser(t, sqlStorage, "b@test.com", "Bob")

	setupTestCommunity(t, sqlStorage, userA, "Alice's Community")
	setupTestCommunity(t, sqlStorage, userB, "Bob's Community")

	ctx := createAuthenticatedContext(userA, "a@test.com")
	resp, err := service.GetSearchSuggestions(
		ctx,
		connect.NewRequest(&api.GetSearchSuggestionsRequest{}),
	)
	if err != nil {
		t.Fatalf("GetSearchSuggestions failed: %v", err)
	}

	if len(resp.Msg.TopCommunities) != 1 {
		t.Fatalf("expected only Alice's community, got %d entries",
			len(resp.Msg.TopCommunities))
	}
	if resp.Msg.TopCommunities[0].Name != "Alice's Community" {
		t.Errorf("expected Alice's Community, got %q",
			resp.Msg.TopCommunities[0].Name)
	}
}

// TestGetSearchSuggestions_NoMembershipsReturnsEmpty verifies the
// degenerate case where the caller belongs to no communities. The
// server returns an empty response, not an error.
func TestGetSearchSuggestions_NoMembershipsReturnsEmpty(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := New(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "owner@test.com", "Owner")

	ctx := createAuthenticatedContext(userID, "owner@test.com")
	resp, err := service.GetSearchSuggestions(
		ctx,
		connect.NewRequest(&api.GetSearchSuggestionsRequest{}),
	)
	if err != nil {
		t.Fatalf("expected empty response, got error: %v", err)
	}

	if len(resp.Msg.TopKnownForCategories) != 0 || len(resp.Msg.TopCommunities) != 0 {
		t.Errorf("expected fully empty response, got categories=%v communities=%v",
			resp.Msg.TopKnownForCategories, resp.Msg.TopCommunities)
	}
}

// TestGetSearchSuggestions_QueryCount locks the per-call query budget.
// Beyond the documented budget likely indicates an N+1 regression on
// either the membership scan or the community-name lookup.
func TestGetSearchSuggestions_QueryCount(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := New(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "owner@test.com", "Owner")
	for i := 0; i < 3; i++ {
		setupTestCommunity(t, sqlStorage, userID, fmt.Sprintf("C%d", i))
	}

	ctx := storage.WithQueryStats(createAuthenticatedContext(userID, "owner@test.com"))
	storage.AssertMaxQueries(t, ctx, 12, func() {
		_, err := service.GetSearchSuggestions(
			ctx,
			connect.NewRequest(&api.GetSearchSuggestionsRequest{}),
		)
		if err != nil {
			t.Fatalf("GetSearchSuggestions failed: %v", err)
		}
	})
}
