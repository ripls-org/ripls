package search

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func createAuthenticatedContext(userID, email string) context.Context {
	authInfo := &auth.Info{
		UserID: userID,
		Email:  email,
		Role:   models.Role_ROLE_USER,
	}
	return authn.SetInfo(context.Background(), authInfo)
}

func setupTestStorage(t *testing.T) *storage.ProtoSQLStorage {
	t.Helper()
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return sqlStorage
}

func setupTestUser(t *testing.T, s *storage.ProtoSQLStorage, email, name string) string {
	t.Helper()
	user := &models.User{
		Email: email,
		Name:  name,
		Role:  models.Role_ROLE_USER,
	}
	id, err := s.Insert(context.Background(), user)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	return id
}

func setupTestCommunity(t *testing.T, s *storage.ProtoSQLStorage, creatorID, name string) string {
	t.Helper()
	now := time.Now().Unix()
	community := &models.Community{
		Name:             name,
		Description:      "Test community",
		CreatorId:        creatorID,
		OwnerUserId:      creatorID,
		CreatedAtUnixSec: now,
		UpdatedAtUnixSec: now,
	}
	id, err := s.Insert(context.Background(), community)
	if err != nil {
		t.Fatalf("Failed to create test community: %v", err)
	}
	membership := &models.CommunityUser{
		CommunityId:      id,
		UserId:           creatorID,
		InviterId:        creatorID,
		CreatedAtUnixSec: now,
	}
	if _, err := s.Insert(context.Background(), membership); err != nil {
		t.Fatalf("Failed to add creator to community: %v", err)
	}
	return id
}

func setupTestGear(t *testing.T, s *storage.ProtoSQLStorage, ownerID, name string) string {
	t.Helper()
	gear := &models.Gear{
		Name:        name,
		Description: "Test gear",
		OwnerId:     ownerID,
		State:       models.GearState_GEAR_STATE_AVAILABLE,
	}
	id, err := s.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("Failed to create test gear: %v", err)
	}
	return id
}

func setupTestGearWithCategory(t *testing.T, s *storage.ProtoSQLStorage, ownerID, name, category string) string {
	t.Helper()
	gear := &models.Gear{
		Name:        name,
		Description: "Test gear",
		OwnerId:     ownerID,
		State:       models.GearState_GEAR_STATE_AVAILABLE,
		Category:    &models.TrackedString{Value: category},
	}
	id, err := s.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("Failed to create test gear: %v", err)
	}
	return id
}

func setupCommunityGear(t *testing.T, s *storage.ProtoSQLStorage, communityID, gearID string) {
	t.Helper()
	cg := &models.CommunityGear{
		CommunityId:      communityID,
		GearId:           gearID,
		Availability:     models.Availability_AVAILABILITY_FOR_LOAN,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	if _, err := s.Insert(context.Background(), cg); err != nil {
		t.Fatalf("Failed to create community gear: %v", err)
	}
}

func TestSearch_EmptyCommunityID(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := New(sqlStorage)

	ctx := createAuthenticatedContext("user-1", "user@test.com")
	req := connect.NewRequest(&api.SearchRequest{
		Query: "test",
	})

	_, err := service.Search(ctx, req)

	if err == nil {
		t.Fatal("Expected error for empty community_id")
	}

	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("Expected InvalidArgument error, got %v", connect.CodeOf(err))
	}
}

// TestSearch_CrossCommunityDeduplication verifies that an item shared with
// multiple communities appears exactly once in a multi-community search.
func TestSearch_CrossCommunityDeduplication(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := New(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "owner@test.com", "Owner")
	communityAID := setupTestCommunity(t, sqlStorage, userID, "Community A")
	communityBID := setupTestCommunity(t, sqlStorage, userID, "Community B")
	gearID := setupTestGear(t, sqlStorage, userID, "Shared Hammer")

	// Share the same gear with both communities.
	setupCommunityGear(t, sqlStorage, communityAID, gearID)
	setupCommunityGear(t, sqlStorage, communityBID, gearID)

	ctx := createAuthenticatedContext(userID, "owner@test.com")
	req := connect.NewRequest(&api.SearchRequest{
		CommunityIds: []string{communityAID, communityBID},
		Query:        "Hammer",
		Strategy:     api.SearchStrategy_SEARCH_STRATEGY_EXACT,
		ItemTypes:    []api.SearchItemType{api.SearchItemType_SEARCH_ITEM_TYPE_GEAR},
	})

	resp, err := service.Search(ctx, req)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	gearCount := 0
	for _, item := range resp.Msg.Results {
		if item.GetGear() != nil && item.GetGear().Id == gearID {
			gearCount++
		}
	}
	if gearCount != 1 {
		t.Errorf("expected gear to appear exactly once across both communities, got %d", gearCount)
	}
}

// TestSearch_GearCategory verifies that the stored gear category round-trips
// onto search results, and that gear without a category leaves the optional
// field absent.
func TestSearch_GearCategory(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := New(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "owner@test.com", "Owner")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Community")
	categorizedID := setupTestGearWithCategory(t, sqlStorage, userID, "Hammer Drill", "Power Tools")
	uncategorizedID := setupTestGear(t, sqlStorage, userID, "Hammer")
	setupCommunityGear(t, sqlStorage, communityID, categorizedID)
	setupCommunityGear(t, sqlStorage, communityID, uncategorizedID)

	ctx := createAuthenticatedContext(userID, "owner@test.com")
	req := connect.NewRequest(&api.SearchRequest{
		CommunityIds: []string{communityID},
		Query:        "Hammer",
		Strategy:     api.SearchStrategy_SEARCH_STRATEGY_EXACT,
		ItemTypes:    []api.SearchItemType{api.SearchItemType_SEARCH_ITEM_TYPE_GEAR},
	})

	resp, err := service.Search(ctx, req)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	found := map[string]*api.Gear{}
	for _, item := range resp.Msg.Results {
		if gear := item.GetGear(); gear != nil {
			found[gear.Id] = gear
		}
	}

	categorized, ok := found[categorizedID]
	if !ok {
		t.Fatal("categorized gear missing from search results")
	}
	if categorized.Category == nil || *categorized.Category != "Power Tools" {
		t.Errorf("expected category %q, got %v", "Power Tools", categorized.Category)
	}

	uncategorized, ok := found[uncategorizedID]
	if !ok {
		t.Fatal("uncategorized gear missing from search results")
	}
	if uncategorized.Category != nil {
		t.Errorf("expected absent category, got %q", *uncategorized.Category)
	}
}

// TestConvertToSearchResultItemBatch_GearCategory covers the semantic-search
// conversion path, which is not exercised by the exact-strategy tests above.
func TestConvertToSearchResultItemBatch_GearCategory(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := New(sqlStorage)

	owner := &api.User{Id: "owner-1", Name: "Owner"}
	userMap := map[string]*api.User{"owner-1": owner}

	withCategory := storage.UnifiedSearchResult{
		ItemType: "gear",
		Gear: &models.Gear{
			Id:       "gear-1",
			Name:     "Hammer Drill",
			OwnerId:  "owner-1",
			Category: &models.TrackedString{Value: "Power Tools"},
		},
	}
	item, err := service.convertToSearchResultItemBatch(withCategory, userMap)
	if err != nil {
		t.Fatalf("convertToSearchResultItemBatch failed: %v", err)
	}
	gear := item.GetGear()
	if gear.Category == nil || *gear.Category != "Power Tools" {
		t.Errorf("expected category %q, got %v", "Power Tools", gear.Category)
	}

	withoutCategory := storage.UnifiedSearchResult{
		ItemType: "gear",
		Gear: &models.Gear{
			Id:      "gear-2",
			Name:    "Hammer",
			OwnerId: "owner-1",
		},
	}
	item, err = service.convertToSearchResultItemBatch(withoutCategory, userMap)
	if err != nil {
		t.Fatalf("convertToSearchResultItemBatch failed: %v", err)
	}
	if gear := item.GetGear(); gear.Category != nil {
		t.Errorf("expected absent category, got %q", *gear.Category)
	}
}

// TestSearch_QueryCount locks the per-call query count for Search across two
// communities. The active-community gate is a single batched JOIN
// (FilterActiveMemberCommunities) regardless of N — that's the property
// this lock protects against future regressions to a per-id loop.
func TestSearch_QueryCount(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := New(sqlStorage)

	userID := setupTestUser(t, sqlStorage, "owner@test.com", "Owner")
	communityAID := setupTestCommunity(t, sqlStorage, userID, "Community A")
	communityBID := setupTestCommunity(t, sqlStorage, userID, "Community B")
	gearID := setupTestGear(t, sqlStorage, userID, "Hammer")
	setupCommunityGear(t, sqlStorage, communityAID, gearID)
	setupCommunityGear(t, sqlStorage, communityBID, gearID)

	ctx := storage.WithQueryStats(createAuthenticatedContext(userID, "owner@test.com"))

	// Seven queries measured post-Phase 3 with two communities and a single
	// gear shared with both:
	//   1. FilterActiveMemberCommunities — single JOIN for both communities
	//      (was 1+N pre-Phase-3, when each community was checked in a loop)
	//   2-7. Per-community search fan-out (gear ILIKE, location lookups,
	//        owner batch fetch) inside searchGearExact.
	// Bumping this upward without justification means an N+1 has crept back
	// in. Most likely regression signature: a per-community auth loop
	// replaces the single batched JOIN.
	const maxQueries = 7
	storage.AssertMaxQueries(t, ctx, maxQueries, func() {
		req := connect.NewRequest(&api.SearchRequest{
			CommunityIds: []string{communityAID, communityBID},
			Query:        "Hammer",
			Strategy:     api.SearchStrategy_SEARCH_STRATEGY_EXACT,
			ItemTypes:    []api.SearchItemType{api.SearchItemType_SEARCH_ITEM_TYPE_GEAR},
		})
		if _, err := service.Search(ctx, req); err != nil {
			t.Fatalf("Search: %v", err)
		}
	})
}
