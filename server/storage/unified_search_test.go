package storage

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// ============================================================================
// Composite Score Computation Tests
// ============================================================================.

func TestComputeCompositeScore_CalculatesCorrectly(t *testing.T) {
	tests := []struct {
		name           string
		semantic       float64
		distanceMeters float64
		wantMin        float64
		wantMax        float64
	}{
		{
			name:           "perfect semantic, zero distance",
			semantic:       1.0,
			distanceMeters: 0,
			wantMin:        0.99,
			wantMax:        1.0,
		},
		{
			name:           "perfect semantic, far distance (50km)",
			semantic:       1.0,
			distanceMeters: 50000,
			wantMin:        0.69,
			wantMax:        0.71,
		},
		{
			name:           "zero semantic, zero distance",
			semantic:       0.0,
			distanceMeters: 0,
			wantMin:        0.29,
			wantMax:        0.31,
		},
		{
			name:           "mid semantic, mid distance (25km)",
			semantic:       0.5,
			distanceMeters: 25000,
			wantMin:        0.49,
			wantMax:        0.51,
		},
		{
			name:           "distance beyond max (100km) caps at zero contribution",
			semantic:       1.0,
			distanceMeters: 100000,
			wantMin:        0.69,
			wantMax:        0.71,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeCompositeScore(tt.semantic, tt.distanceMeters)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("computeCompositeScore(%v, %v) = %v, want between %v and %v",
					tt.semantic, tt.distanceMeters, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestComputeCompositeScore_OrdersCorrectly(t *testing.T) {
	// Items closer with same semantic score should rank higher
	closeScore := computeCompositeScore(0.8, 1000) // 1km away
	farScore := computeCompositeScore(0.8, 40000)  // 40km away

	if closeScore <= farScore {
		t.Errorf("closer item should have higher score: close=%v, far=%v", closeScore, farScore)
	}

	// Items with higher semantic score but farther should still potentially rank higher
	highSemanticFar := computeCompositeScore(1.0, 30000) // perfect match, 30km
	lowSemanticClose := computeCompositeScore(0.3, 1000) // poor match, 1km

	if highSemanticFar <= lowSemanticClose {
		t.Errorf("high semantic relevance should outweigh distance: highFar=%v, lowClose=%v",
			highSemanticFar, lowSemanticClose)
	}
}

// ============================================================================
// Unified Search Tests
// ============================================================================.

func TestQueryCommunitySearch_ReturnsAllItemTypes(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create test data
	user := &models.User{Name: "Test User", Email: "test@example.com"}
	userID, err := storage.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Insert user error = %v", err)
	}

	community := &models.Community{Name: "Test Community", CreatorId: userID, OwnerUserId: userID}
	communityID, err := storage.Insert(ctx, community)
	if err != nil {
		t.Fatalf("Insert community error = %v", err)
	}

	// Insert gear
	gear := &models.Gear{Name: "Power Drill", Description: "Cordless drill", OwnerId: userID}
	gearID, err := storage.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("Insert gear error = %v", err)
	}
	cg := &models.CommunityGear{CommunityId: communityID, GearId: gearID, CreatedAtUnixSec: time.Now().Unix()}
	if _, err := storage.Insert(ctx, cg); err != nil {
		t.Fatalf("Insert community_gear error = %v", err)
	}

	// Insert request
	req := &models.Request{
		Title:       "Need a Drill",
		Description: "Looking for a drill",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	reqID, err := storage.Insert(ctx, req)
	if err != nil {
		t.Fatalf("Insert request error = %v", err)
	}
	// Share request with community
	cr := &models.CommunityRequest{CommunityId: communityID, RequestId: reqID, SharedAtUnixSec: time.Now().Unix()}
	if _, err := storage.Insert(ctx, cr); err != nil {
		t.Fatalf("Insert community_request error = %v", err)
	}

	// Insert experience
	exp := &models.Experience{
		Name:        "Drill Workshop",
		Description: "Learn to use a drill",
		OwnerId:     userID,
		State:       models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
	}
	expID, err := storage.Insert(ctx, exp)
	if err != nil {
		t.Fatalf("Insert experience error = %v", err)
	}
	ce := &models.CommunityExperience{CommunityId: communityID, ExperienceId: expID, SharedAtUnixSec: time.Now().Unix()}
	if _, err := storage.Insert(ctx, ce); err != nil {
		t.Fatalf("Insert community_experience error = %v", err)
	}

	// Search
	results, err := storage.QueryCommunitySearch(ctx, communityID, "drill", 0, 0, false)
	if err != nil {
		t.Fatalf("QueryCommunitySearch() error = %v", err)
	}

	// Verify we got all 3 types
	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	foundGear, foundRequest, foundExperience := false, false, false
	for _, r := range results {
		switch r.ItemType {
		case "gear":
			foundGear = true
			if r.Gear == nil {
				t.Error("Gear result has nil Gear")
			}
		case "request":
			foundRequest = true
			if r.Request == nil {
				t.Error("Request result has nil Request")
			}
		case "experience":
			foundExperience = true
			if r.Experience == nil {
				t.Error("Experience result has nil Experience")
			}
		}
	}

	if !foundGear {
		t.Error("No gear result found")
	}
	if !foundRequest {
		t.Error("No request result found")
	}
	if !foundExperience {
		t.Error("No experience result found")
	}
}

func TestQueryCommunitySearch_SortsByCompositeScoreDescending(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create test data
	user := &models.User{Name: "Test User", Email: "test@example.com"}
	userID, err := storage.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Insert user error = %v", err)
	}

	community := &models.Community{Name: "Test Community", CreatorId: userID, OwnerUserId: userID}
	communityID, err := storage.Insert(ctx, community)
	if err != nil {
		t.Fatalf("Insert community error = %v", err)
	}

	// Insert 5 gear items with different locations
	// Create location with coordinates to generate different distances
	loc1 := &models.Location{
		Geolocation: &models.Geolocation{LatitudeDeg: 40.0, LongitudeDeg: -105.0},
	}
	loc1ID, _ := storage.Insert(ctx, loc1)

	loc2 := &models.Location{
		Geolocation: &models.Geolocation{LatitudeDeg: 41.0, LongitudeDeg: -105.0}, // ~111km away
	}
	loc2ID, _ := storage.Insert(ctx, loc2)

	// Insert gear at different locations
	gearNear := &models.Gear{Name: "Drill Near", Description: "drill", OwnerId: userID, LocationId: loc1ID}
	gearNearID, _ := storage.Insert(ctx, gearNear)
	_, _ = storage.Insert(ctx, &models.CommunityGear{CommunityId: communityID, GearId: gearNearID, CreatedAtUnixSec: time.Now().Unix()})

	gearFar := &models.Gear{Name: "Drill Far", Description: "drill", OwnerId: userID, LocationId: loc2ID}
	gearFarID, _ := storage.Insert(ctx, gearFar)
	_, _ = storage.Insert(ctx, &models.CommunityGear{CommunityId: communityID, GearId: gearFarID, CreatedAtUnixSec: time.Now().Unix()})

	// Search from location close to loc1
	results, err := storage.QueryCommunitySearch(ctx, communityID, "drill", 40.0, -105.0, false)
	if err != nil {
		t.Fatalf("QueryCommunitySearch() error = %v", err)
	}

	if len(results) < 2 {
		t.Fatalf("Expected at least 2 results, got %d", len(results))
	}

	// Verify results are sorted by composite score descending
	for i := 1; i < len(results); i++ {
		if results[i].CompositeScore > results[i-1].CompositeScore {
			t.Errorf("Results not sorted by composite score descending: position %d (%v) > position %d (%v)",
				i, results[i].CompositeScore, i-1, results[i-1].CompositeScore)
		}
	}
}

func TestQueryCommunitySearch_FiltersByCommunity(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	user := &models.User{Name: "Test User", Email: "test@example.com"}
	userID, _ := storage.Insert(ctx, user)

	// Create two communities
	comm1 := &models.Community{Name: "Community 1", CreatorId: userID, OwnerUserId: userID}
	comm1ID, _ := storage.Insert(ctx, comm1)

	comm2 := &models.Community{Name: "Community 2", CreatorId: userID, OwnerUserId: userID}
	comm2ID, _ := storage.Insert(ctx, comm2)

	// Add gear to community 1 only
	gear1 := &models.Gear{Name: "Drill One", Description: "drill", OwnerId: userID}
	gear1ID, _ := storage.Insert(ctx, gear1)
	_, _ = storage.Insert(ctx, &models.CommunityGear{CommunityId: comm1ID, GearId: gear1ID, CreatedAtUnixSec: time.Now().Unix()})

	// Add request to community 2 only
	req := &models.Request{
		Title:       "Need Drill",
		Description: "drill request",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	reqID, _ := storage.Insert(ctx, req)
	// Share request with community 2
	cr := &models.CommunityRequest{CommunityId: comm2ID, RequestId: reqID, SharedAtUnixSec: time.Now().Unix()}
	_, _ = storage.Insert(ctx, cr)

	// Search community 1 should only return gear
	results1, _ := storage.QueryCommunitySearch(ctx, comm1ID, "drill", 0, 0, false)
	if len(results1) != 1 {
		t.Errorf("Community 1 search: expected 1 result, got %d", len(results1))
	}
	if len(results1) > 0 && results1[0].ItemType != "gear" {
		t.Errorf("Community 1 search: expected gear, got %s", results1[0].ItemType)
	}

	// Search community 2 should only return request
	results2, _ := storage.QueryCommunitySearch(ctx, comm2ID, "drill", 0, 0, false)
	if len(results2) != 1 {
		t.Errorf("Community 2 search: expected 1 result, got %d", len(results2))
	}
	if len(results2) > 0 && results2[0].ItemType != "request" {
		t.Errorf("Community 2 search: expected request, got %s", results2[0].ItemType)
	}
}

func TestQueryCommunitySearch_OnlyReturnsActiveRequestsAndExperiences(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	user := &models.User{Name: "Test User", Email: "test@example.com"}
	userID, _ := storage.Insert(ctx, user)

	comm := &models.Community{Name: "Test Community", CreatorId: userID, OwnerUserId: userID}
	commID, _ := storage.Insert(ctx, comm)

	// Insert active and inactive requests
	activeReq := &models.Request{
		Title:       "Active Drill Request",
		Description: "drill",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	activeReqID, _ := storage.Insert(ctx, activeReq)
	// Share active request with community
	cr1 := &models.CommunityRequest{CommunityId: commID, RequestId: activeReqID, SharedAtUnixSec: time.Now().Unix()}
	_, _ = storage.Insert(ctx, cr1)

	cancelledReq := &models.Request{
		Title:       "Cancelled Drill Request",
		Description: "drill",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_CANCELLED,
	}
	cancelledReqID, _ := storage.Insert(ctx, cancelledReq)
	// Share cancelled request with community
	cr2 := &models.CommunityRequest{CommunityId: commID, RequestId: cancelledReqID, SharedAtUnixSec: time.Now().Unix()}
	_, _ = storage.Insert(ctx, cr2)

	// Insert active and inactive experiences
	activeExp := &models.Experience{
		Name:        "Active Drill Workshop",
		Description: "drill",
		OwnerId:     userID,
		State:       models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
	}
	activeExpID, _ := storage.Insert(ctx, activeExp)
	_, _ = storage.Insert(ctx, &models.CommunityExperience{CommunityId: commID, ExperienceId: activeExpID, SharedAtUnixSec: time.Now().Unix()})

	cancelledExp := &models.Experience{
		Name:        "Cancelled Drill Workshop",
		Description: "drill",
		OwnerId:     userID,
		State:       models.ExperienceState_EXPERIENCE_STATE_CANCELLED,
	}
	cancelledExpID, _ := storage.Insert(ctx, cancelledExp)
	_, _ = storage.Insert(ctx, &models.CommunityExperience{CommunityId: commID, ExperienceId: cancelledExpID, SharedAtUnixSec: time.Now().Unix()})

	results, err := storage.QueryCommunitySearch(ctx, commID, "drill", 0, 0, false)
	if err != nil {
		t.Fatalf("QueryCommunitySearch() error = %v", err)
	}

	// Should have 2 results: 1 active request + 1 active experience (cancelled items excluded).
	if len(results) != 2 {
		t.Errorf("Expected 2 results (1 active request + 1 active experience), got %d", len(results))
	}

	// Verify we only get active requests.
	for _, r := range results {
		if r.ItemType == "request" && r.Request.Title != "Active Drill Request" {
			t.Errorf("Expected only active request, got %s", r.Request.Title)
		}
	}

	// Verify we only get the active experience.
	experienceCount := 0
	for _, r := range results {
		if r.ItemType == "experience" {
			experienceCount++
			if r.Experience.Name != "Active Drill Workshop" {
				t.Errorf("Expected only active experience, got %s", r.Experience.Name)
			}
		}
	}
	if experienceCount != 1 {
		t.Errorf("Expected 1 active experience, got %d", experienceCount)
	}
}

func TestQueryCommunitySearch_ExcludesDeletedRequests(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	user := &models.User{Name: "Test User", Email: "test@example.com"}
	userID, _ := storage.Insert(ctx, user)

	comm := &models.Community{Name: "Test Community", CreatorId: userID, OwnerUserId: userID}
	commID, _ := storage.Insert(ctx, comm)

	// Insert active request
	activeReq := &models.Request{
		Title:       "Active Drill Request",
		Description: "drill",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	activeReqID, _ := storage.Insert(ctx, activeReq)
	// Share active request with community
	cr1 := &models.CommunityRequest{CommunityId: commID, RequestId: activeReqID, SharedAtUnixSec: time.Now().Unix()}
	_, _ = storage.Insert(ctx, cr1)

	// Insert deleted request (same search terms but soft-deleted)
	deletedReq := &models.Request{
		Title:       "Deleted Drill Request",
		Description: "drill",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
		Deleted: &models.DeletedMetadata{
			DeletedByUserId:  userID,
			DeletedAtUnixSec: time.Now().Unix(),
		},
	}
	deletedReqID, _ := storage.Insert(ctx, deletedReq)
	// Share deleted request with community (so it can be filtered by deleted status)
	cr2 := &models.CommunityRequest{CommunityId: commID, RequestId: deletedReqID, SharedAtUnixSec: time.Now().Unix()}
	_, _ = storage.Insert(ctx, cr2)

	results, err := storage.QueryCommunitySearch(ctx, commID, "drill", 0, 0, false)
	if err != nil {
		t.Fatalf("QueryCommunitySearch() error = %v", err)
	}

	// Should only have the active (non-deleted) request
	requestCount := 0
	for _, r := range results {
		if r.ItemType == "request" {
			requestCount++
			if r.Request.Title == "Deleted Drill Request" {
				t.Error("Expected deleted request to be excluded from search results")
			}
		}
	}

	if requestCount != 1 {
		t.Errorf("Expected 1 non-deleted request, got %d", requestCount)
	}
}

// TestQueryCommunitySearch_ExcludesDeletedCommunityGear verifies that a
// soft-deleted community_gear row is not surfaced in search, even when the
// underlying gear is still live. Exercises the notDeletedFilter fragment on
// the join-table side for unifiedGearConfig.
func TestQueryCommunitySearch_ExcludesDeletedCommunityGear(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	user := &models.User{Name: "Test User", Email: "excl-cg@example.com"}
	userID, _ := store.Insert(ctx, user)

	comm := &models.Community{Name: "Test Community", CreatorId: userID, OwnerUserId: userID}
	commID, _ := store.Insert(ctx, comm)

	liveGear := &models.Gear{Name: "Live Drill", Description: "drill", OwnerId: userID}
	liveGearID, _ := store.Insert(ctx, liveGear)
	liveCG := &models.CommunityGear{CommunityId: commID, GearId: liveGearID, CreatedAtUnixSec: time.Now().Unix()}
	_, _ = store.Insert(ctx, liveCG)

	deletedCG := &models.CommunityGear{CommunityId: commID, GearId: liveGearID, CreatedAtUnixSec: time.Now().Unix()}
	deletedCGID, _ := store.Insert(ctx, deletedCG)

	// Soft-delete the second community_gear row directly.
	storedDeletedCG := &models.CommunityGear{}
	if err := store.GetByID(ctx, deletedCGID, storedDeletedCG); err != nil {
		t.Fatalf("get deletedCG: %v", err)
	}
	storedDeletedCG.Deleted = &models.DeletedMetadata{DeletedByUserId: userID, DeletedAtUnixSec: time.Now().Unix()}
	if err := store.Update(ctx, storedDeletedCG); err != nil {
		t.Fatalf("soft-delete deletedCG: %v", err)
	}

	results, err := store.QueryCommunitySearch(ctx, commID, "drill", 0, 0, false)
	if err != nil {
		t.Fatalf("QueryCommunitySearch: %v", err)
	}

	gearHits := 0
	for _, r := range results {
		if r.ItemType == "gear" {
			gearHits++
		}
	}
	// We expect exactly one hit: only the live community_gear row, even though
	// both rows point at the same live Gear.
	if gearHits != 1 {
		t.Errorf("expected 1 gear hit (live community_gear only), got %d", gearHits)
	}
}

// TestIsGearInCommunity_ExcludesSoftDeleted verifies the guard we added to the
// raw existence check. Soft-deleted join rows must not grant access.
func TestIsGearInCommunity_ExcludesSoftDeleted(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	user := &models.User{Email: "is-gear@example.com"}
	userID, _ := store.Insert(ctx, user)
	gear := &models.Gear{Name: "Some Gear", OwnerId: userID}
	gearID, _ := store.Insert(ctx, gear)
	comm := &models.Community{Name: "C", CreatorId: userID, OwnerUserId: userID}
	commID, _ := store.Insert(ctx, comm)

	cg := &models.CommunityGear{CommunityId: commID, GearId: gearID}
	cgID, _ := store.Insert(ctx, cg)

	got, err := store.IsGearInCommunity(ctx, gearID, commID)
	if err != nil {
		t.Fatalf("IsGearInCommunity (live): %v", err)
	}
	if !got {
		t.Error("expected true for live community_gear")
	}

	// Soft-delete the row.
	stored := &models.CommunityGear{}
	if err := store.GetByID(ctx, cgID, stored); err != nil {
		t.Fatalf("get cg: %v", err)
	}
	stored.Deleted = &models.DeletedMetadata{DeletedByUserId: userID, DeletedAtUnixSec: time.Now().Unix()}
	if err := store.Update(ctx, stored); err != nil {
		t.Fatalf("soft-delete cg: %v", err)
	}

	got, err = store.IsGearInCommunity(ctx, gearID, commID)
	if err != nil {
		t.Fatalf("IsGearInCommunity (deleted): %v", err)
	}
	if got {
		t.Error("expected false for soft-deleted community_gear")
	}
}

// ============================================================================
// Fallback to Text Search Tests
// ============================================================================.

func TestQueryCommunitySearch_FallsBackToTextSearchWhenNotConfigured(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// DO NOT configure embedder - should use text search

	user := &models.User{Name: "Test User", Email: "test@example.com"}
	userID, _ := storage.Insert(ctx, user)

	comm := &models.Community{Name: "Test Community", CreatorId: userID, OwnerUserId: userID}
	commID, _ := storage.Insert(ctx, comm)

	// Insert items with searchable text
	gear := &models.Gear{Name: "Drill Press", Description: "Heavy duty", OwnerId: userID}
	gearID, _ := storage.Insert(ctx, gear)
	_, _ = storage.Insert(ctx, &models.CommunityGear{CommunityId: commID, GearId: gearID, CreatedAtUnixSec: time.Now().Unix()})

	// Search should use text search and find by name
	results, err := storage.QueryCommunitySearch(ctx, commID, "drill", 0, 0, false)
	if err != nil {
		t.Fatalf("QueryCommunitySearch() error = %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result from text search, got %d", len(results))
	}
}

func TestQueryCommunitySearch_TextSearchSetsSemanticSimilarityToOne(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Don't configure embedder - force text search

	user := &models.User{Name: "Test User", Email: "test@example.com"}
	userID, _ := storage.Insert(ctx, user)

	comm := &models.Community{Name: "Test Community", CreatorId: userID, OwnerUserId: userID}
	commID, _ := storage.Insert(ctx, comm)

	gear := &models.Gear{Name: "Hammer", Description: "Claw hammer", OwnerId: userID}
	gearID, _ := storage.Insert(ctx, gear)
	_, _ = storage.Insert(ctx, &models.CommunityGear{CommunityId: commID, GearId: gearID, CreatedAtUnixSec: time.Now().Unix()})

	results, _ := storage.QueryCommunitySearch(ctx, commID, "hammer", 0, 0, false)

	if len(results) == 0 {
		t.Fatal("Expected at least 1 result")
	}

	// Text search should set semantic similarity to 1.0
	if results[0].SemanticSimilarity != 1.0 {
		t.Errorf("Expected SemanticSimilarity = 1.0 for text search, got %v", results[0].SemanticSimilarity)
	}
}

// ============================================================================
// Semantic Search Tests
// ============================================================================.

func TestQueryCommunitySearch_UsesSemanticSearchWhenConfigured(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	embedder := SetupTestEmbedder(t)

	ctx := context.Background()

	if err := storage.SetEmbedder(ctx, embedder); err != nil {
		t.Fatalf("SetEmbedder() error = %v", err)
	}

	user := &models.User{Name: "Test User", Email: "test@example.com"}
	userID, _ := storage.Insert(ctx, user)

	comm := &models.Community{Name: "Test Community", CreatorId: userID, OwnerUserId: userID}
	commID, _ := storage.Insert(ctx, comm)

	embeddingDone := make(chan error, 10)
	storage.SetEmbeddingDoneChannel(embeddingDone)

	// Insert gear
	gear := &models.Gear{Name: "Power Drill", Description: "Cordless drill", OwnerId: userID}
	gearID, _ := storage.Insert(ctx, gear)
	_, _ = storage.Insert(ctx, &models.CommunityGear{CommunityId: commID, GearId: gearID, CreatedAtUnixSec: time.Now().Unix()})

	// Insert request
	req := &models.Request{
		Title:       "Need Drill",
		Description: "Looking for drill",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	reqID, _ := storage.Insert(ctx, req)
	// Share request with community
	cr := &models.CommunityRequest{CommunityId: commID, RequestId: reqID, SharedAtUnixSec: time.Now().Unix()}
	_, _ = storage.Insert(ctx, cr)

	// Insert experience
	exp := &models.Experience{
		Name:        "Drill Class",
		Description: "Learn drilling",
		OwnerId:     userID,
		State:       models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
	}
	expID, _ := storage.Insert(ctx, exp)
	_, _ = storage.Insert(ctx, &models.CommunityExperience{CommunityId: commID, ExperienceId: expID, SharedAtUnixSec: time.Now().Unix()})

	// Wait for embeddings (3 items)
	for i := 0; i < 3; i++ {
		select {
		case embErr := <-embeddingDone:
			if embErr != nil {
				t.Fatalf("embedding generation failed: %v", embErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for embedding")
		}
	}

	results, err := storage.QueryCommunitySearch(ctx, commID, "drill tools", 0, 0, false)
	if err != nil {
		t.Fatalf("QueryCommunitySearch() error = %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}
}

func TestQueryCommunitySearch_SemanticSearchReturnsRelevanceScores(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	embedder := SetupTestEmbedder(t)

	ctx := context.Background()

	if err := storage.SetEmbedder(ctx, embedder); err != nil {
		t.Fatalf("SetEmbedder() error = %v", err)
	}

	user := &models.User{Name: "Test User", Email: "test@example.com"}
	userID, _ := storage.Insert(ctx, user)

	comm := &models.Community{Name: "Test Community", CreatorId: userID, OwnerUserId: userID}
	commID, _ := storage.Insert(ctx, comm)

	embeddingDone := make(chan error, 10)
	storage.SetEmbeddingDoneChannel(embeddingDone)

	// Insert items with different semantic distances from "power drill"
	// Both should be above the similarity threshold (0.5) but drill should rank higher
	drill := &models.Gear{Name: "Electric Drill", Description: "Professional cordless power drill for woodworking", OwnerId: userID}
	drillID, _ := storage.Insert(ctx, drill)
	_, _ = storage.Insert(ctx, &models.CommunityGear{CommunityId: commID, GearId: drillID, CreatedAtUnixSec: time.Now().Unix()})

	hammer := &models.Gear{Name: "Hammer", Description: "Heavy duty claw hammer for construction and home improvement projects", OwnerId: userID}
	hammerID, _ := storage.Insert(ctx, hammer)
	_, _ = storage.Insert(ctx, &models.CommunityGear{CommunityId: commID, GearId: hammerID, CreatedAtUnixSec: time.Now().Unix()})

	// Wait for embeddings
	for i := 0; i < 2; i++ {
		select {
		case embErr := <-embeddingDone:
			if embErr != nil {
				t.Fatalf("embedding generation failed: %v", embErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for embedding")
		}
	}

	results, _ := storage.QueryCommunitySearch(ctx, commID, "power drill", 0, 0, false)

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	// Verify semantic similarity scores are set and reasonable
	for _, r := range results {
		if r.SemanticSimilarity < 0 || r.SemanticSimilarity > 1.0 {
			t.Errorf("SemanticSimilarity out of range: %v", r.SemanticSimilarity)
		}
	}

	// First result (drill) should have higher similarity than second (hammer)
	if results[0].SemanticSimilarity <= results[1].SemanticSimilarity {
		t.Errorf("Expected first result to have higher similarity: %v <= %v",
			results[0].SemanticSimilarity, results[1].SemanticSimilarity)
	}
}

// ============================================================================
// Empty Query Tests
// ============================================================================.

func TestQueryCommunitySearch_EmptyQueryReturnsAllItems(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	user := &models.User{Name: "Test User", Email: "test@example.com"}
	userID, _ := storage.Insert(ctx, user)

	comm := &models.Community{Name: "Test Community", CreatorId: userID, OwnerUserId: userID}
	commID, _ := storage.Insert(ctx, comm)

	// Insert various items
	gear := &models.Gear{Name: "Drill", Description: "drill", OwnerId: userID}
	gearID, _ := storage.Insert(ctx, gear)
	_, _ = storage.Insert(ctx, &models.CommunityGear{CommunityId: commID, GearId: gearID, CreatedAtUnixSec: time.Now().Unix()})

	req := &models.Request{
		Title:       "Request",
		Description: "need something",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	reqID, _ := storage.Insert(ctx, req)
	// Share request with community
	cr := &models.CommunityRequest{CommunityId: commID, RequestId: reqID, SharedAtUnixSec: time.Now().Unix()}
	_, _ = storage.Insert(ctx, cr)

	exp := &models.Experience{
		Name:        "Workshop",
		Description: "learn stuff",
		OwnerId:     userID,
		State:       models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
	}
	expID, _ := storage.Insert(ctx, exp)
	_, _ = storage.Insert(ctx, &models.CommunityExperience{CommunityId: commID, ExperienceId: expID, SharedAtUnixSec: time.Now().Unix()})

	// Empty query should return all
	results, err := storage.QueryCommunitySearch(ctx, commID, "", 0, 0, false)
	if err != nil {
		t.Fatalf("QueryCommunitySearch() error = %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results for empty query, got %d", len(results))
	}
}

// ============================================================================
// Availability Tests
// ============================================================================.

func TestQueryCommunitySearch_ReturnsGearAvailability(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	user := &models.User{Name: "Test User", Email: "test@example.com"}
	userID, _ := storage.Insert(ctx, user)

	comm := &models.Community{Name: "Test Community", CreatorId: userID, OwnerUserId: userID}
	commID, _ := storage.Insert(ctx, comm)

	gear := &models.Gear{Name: "Drill", Description: "drill", OwnerId: userID}
	gearID, _ := storage.Insert(ctx, gear)
	cg := &models.CommunityGear{
		CommunityId:      commID,
		GearId:           gearID,
		CreatedAtUnixSec: time.Now().Unix(),
		Availability:     models.Availability_AVAILABILITY_FOR_GIVEAWAY,
	}
	_, _ = storage.Insert(ctx, cg)

	results, _ := storage.QueryCommunitySearch(ctx, commID, "drill", 0, 0, false)

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	expectedAvail := int32(models.Availability_AVAILABILITY_FOR_GIVEAWAY)
	if results[0].Availability != expectedAvail {
		t.Errorf("Expected giveaway availability (%v), got %v", expectedAvail, results[0].Availability)
	}
}
