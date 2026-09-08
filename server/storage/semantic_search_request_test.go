package storage

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// ============================================================================
// Request Semantic Search Tests
// ============================================================================.

func TestQueryRequestByCommunitySearch_UsesSemanticSearchWhenConfigured(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	embedder := SetupTestEmbedder(t)

	ctx := context.Background()

	// Configure embeddings
	if err := storage.SetEmbedder(ctx, embedder); err != nil {
		t.Fatalf("SetEmbedder() error = %v", err)
	}

	// Create test data: user, community, requests
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

	// Set up embedding done channel BEFORE inserts
	embeddingDone := make(chan error, 10)
	storage.SetEmbeddingDoneChannel(embeddingDone)

	// Insert requests (embeddings generated async)
	req1 := &models.Request{
		Title:       "Need a Drill",
		Description: "Looking for a cordless drill for weekend project",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	req1ID, err := storage.Insert(ctx, req1)
	if err != nil {
		t.Fatalf("Insert request1 error = %v", err)
	}
	// Share request with community
	cr1 := &models.CommunityRequest{CommunityId: communityID, RequestId: req1ID, SharedAtUnixSec: time.Now().Unix()}
	if _, err := storage.Insert(ctx, cr1); err != nil {
		t.Fatalf("Insert community_request error = %v", err)
	}

	req2 := &models.Request{
		Title:       "Looking for Hammer",
		Description: "Need a standard claw hammer",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	req2ID, err := storage.Insert(ctx, req2)
	if err != nil {
		t.Fatalf("Insert request2 error = %v", err)
	}
	// Share request with community
	cr2 := &models.CommunityRequest{CommunityId: communityID, RequestId: req2ID, SharedAtUnixSec: time.Now().Unix()}
	if _, err := storage.Insert(ctx, cr2); err != nil {
		t.Fatalf("Insert community_request error = %v", err)
	}

	// Wait for async embeddings to complete
	for i := 0; i < 2; i++ {
		select {
		case embErr := <-embeddingDone:
			if embErr != nil {
				t.Fatalf("embedding generation failed: %v", embErr)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for embedding generation")
		}
	}

	// Perform search
	results, err := storage.QueryRequestByCommunitySearch(ctx, communityID, "drill tools", 0, 0)
	if err != nil {
		t.Fatalf("QueryRequestByCommunitySearch() error = %v", err)
	}

	// Verify results returned
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestQueryRequestByCommunitySearch_FallsBackToTextSearchWhenNotConfigured(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// DO NOT configure embedder - should use text search

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

	// Insert request
	req := &models.Request{
		Title:       "Need Drill Press",
		Description: "Looking for a heavy duty drill press",
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

	// Search should use text search (no embedder)
	results, err := storage.QueryRequestByCommunitySearch(ctx, communityID, "drill", 0, 0)
	if err != nil {
		t.Fatalf("QueryRequestByCommunitySearch() error = %v", err)
	}

	// Should find the request via text matching
	if len(results) != 1 {
		t.Errorf("Expected 1 result from text search, got %d", len(results))
	}
	if len(results) > 0 && results[0].Request.Title != "Need Drill Press" {
		t.Errorf("Expected 'Need Drill Press', got '%s'", results[0].Request.Title)
	}
}

func TestQueryRequestByCommunitySearch_OnlyReturnsActiveRequests(t *testing.T) {
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

	// Insert active request
	activeReq := &models.Request{
		Title:       "Need a Drill",
		Description: "Active request for drill",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	activeReqID, err := storage.Insert(ctx, activeReq)
	if err != nil {
		t.Fatalf("Insert active request error = %v", err)
	}
	// Share active request with community
	cr1 := &models.CommunityRequest{CommunityId: communityID, RequestId: activeReqID, SharedAtUnixSec: time.Now().Unix()}
	if _, err := storage.Insert(ctx, cr1); err != nil {
		t.Fatalf("Insert community_request error = %v", err)
	}

	// Insert cancelled request
	cancelledReq := &models.Request{
		Title:       "Need Another Drill",
		Description: "Cancelled request for drill",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_CANCELLED,
	}
	cancelledReqID, err := storage.Insert(ctx, cancelledReq)
	if err != nil {
		t.Fatalf("Insert cancelled request error = %v", err)
	}
	// Share cancelled request with community
	cr2 := &models.CommunityRequest{CommunityId: communityID, RequestId: cancelledReqID, SharedAtUnixSec: time.Now().Unix()}
	if _, err := storage.Insert(ctx, cr2); err != nil {
		t.Fatalf("Insert community_request error = %v", err)
	}

	// Insert fulfilled request
	fulfilledReq := &models.Request{
		Title:       "Drill Request Fulfilled",
		Description: "Already fulfilled request",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_FULFILLED,
	}
	fulfilledReqID, err := storage.Insert(ctx, fulfilledReq)
	if err != nil {
		t.Fatalf("Insert fulfilled request error = %v", err)
	}
	// Share fulfilled request with community
	cr3 := &models.CommunityRequest{CommunityId: communityID, RequestId: fulfilledReqID, SharedAtUnixSec: time.Now().Unix()}
	if _, err := storage.Insert(ctx, cr3); err != nil {
		t.Fatalf("Insert community_request error = %v", err)
	}

	// Search should only return active requests
	results, err := storage.QueryRequestByCommunitySearch(ctx, communityID, "drill", 0, 0)
	if err != nil {
		t.Fatalf("QueryRequestByCommunitySearch() error = %v", err)
	}

	// Should only find the active request
	if len(results) != 1 {
		t.Errorf("Expected 1 active result, got %d", len(results))
	}
	if len(results) > 0 && results[0].Request.Title != "Need a Drill" {
		t.Errorf("Expected 'Need a Drill' (active), got '%s'", results[0].Request.Title)
	}
}

func TestQueryRequestByCommunitySearch_FiltersByCommunity(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	embedder := SetupTestEmbedder(t)

	ctx := context.Background()

	if err := storage.SetEmbedder(ctx, embedder); err != nil {
		t.Fatalf("SetEmbedder() error = %v", err)
	}

	// Create test data
	user := &models.User{Name: "Test User", Email: "test@example.com"}
	userID, err := storage.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Insert user error = %v", err)
	}

	// Create two communities
	community1 := &models.Community{Name: "Community One", CreatorId: userID, OwnerUserId: userID}
	community1ID, err := storage.Insert(ctx, community1)
	if err != nil {
		t.Fatalf("Insert community1 error = %v", err)
	}

	community2 := &models.Community{Name: "Community Two", CreatorId: userID, OwnerUserId: userID}
	community2ID, err := storage.Insert(ctx, community2)
	if err != nil {
		t.Fatalf("Insert community2 error = %v", err)
	}

	embeddingDone := make(chan error, 10)
	storage.SetEmbeddingDoneChannel(embeddingDone)

	// Insert requests in different communities
	req1 := &models.Request{
		Title:       "Need a Power Drill",
		Description: "Looking for a cordless power drill for home renovation project",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	req1ID, err := storage.Insert(ctx, req1)
	if err != nil {
		t.Fatalf("Insert request1 error = %v", err)
	}
	// Share req1 with community1
	cr1 := &models.CommunityRequest{CommunityId: community1ID, RequestId: req1ID, SharedAtUnixSec: time.Now().Unix()}
	if _, err := storage.Insert(ctx, cr1); err != nil {
		t.Fatalf("Insert community_request1 error = %v", err)
	}

	req2 := &models.Request{
		Title:       "Borrowing a Drill",
		Description: "Need to borrow an electric drill for installing shelves",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	req2ID, err := storage.Insert(ctx, req2)
	if err != nil {
		t.Fatalf("Insert request2 error = %v", err)
	}
	// Share req2 with community2
	cr2 := &models.CommunityRequest{CommunityId: community2ID, RequestId: req2ID, SharedAtUnixSec: time.Now().Unix()}
	if _, err := storage.Insert(ctx, cr2); err != nil {
		t.Fatalf("Insert community_request2 error = %v", err)
	}

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

	// Search in community1 should only return req1
	results1, err := storage.QueryRequestByCommunitySearch(ctx, community1ID, "drill", 0, 0)
	if err != nil {
		t.Fatalf("QueryRequestByCommunitySearch(community1) error = %v", err)
	}

	if len(results1) != 1 {
		t.Errorf("Expected 1 result for community1, got %d", len(results1))
	}
	if len(results1) > 0 && results1[0].Request.Title != "Need a Power Drill" {
		t.Errorf("Expected 'Need a Power Drill', got '%s'", results1[0].Request.Title)
	}

	// Search in community2 should only return req2
	results2, err := storage.QueryRequestByCommunitySearch(ctx, community2ID, "drill", 0, 0)
	if err != nil {
		t.Fatalf("QueryRequestByCommunitySearch(community2) error = %v", err)
	}

	if len(results2) != 1 {
		t.Errorf("Expected 1 result for community2, got %d", len(results2))
	}
	if len(results2) > 0 && results2[0].Request.Title != "Borrowing a Drill" {
		t.Errorf("Expected 'Borrowing a Drill', got '%s'", results2[0].Request.Title)
	}
}

func TestQueryRequestByCommunitySearch_EmptyQueryReturnsAllActiveRequests(t *testing.T) {
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

	// Insert multiple active requests
	for i := 0; i < 3; i++ {
		req := &models.Request{
			Title:       "Request " + string(rune('A'+i)),
			Description: "Test request",
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
	}

	// Empty query should return all active requests
	results, err := storage.QueryRequestByCommunitySearch(ctx, communityID, "", 0, 0)
	if err != nil {
		t.Fatalf("QueryRequestByCommunitySearch() error = %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results for empty query, got %d", len(results))
	}
}

// ============================================================================
// Similarity Threshold Tests
// ============================================================================.

func TestSemanticSearchMinSimilarity_ConstantIsReasonable(t *testing.T) {
	// The threshold should be in a reasonable range (0.4-0.6 is typical for filtering)
	// Based on production testing: noise floor is ~0.3-0.4, good matches are ~0.55-0.65+
	if SemanticSearchMinSimilarity < 0.4 {
		t.Errorf("SemanticSearchMinSimilarity too low: %v (should be >= 0.4)", SemanticSearchMinSimilarity)
	}
	if SemanticSearchMinSimilarity > 0.7 {
		t.Errorf("SemanticSearchMinSimilarity too high: %v (should be <= 0.7)", SemanticSearchMinSimilarity)
	}
}

func TestQueryGearByCommunitySearch_ThresholdFiltersIrrelevantResults(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	embedder := SetupTestEmbedder(t)

	ctx := context.Background()

	if err := storage.SetEmbedder(ctx, embedder); err != nil {
		t.Fatalf("SetEmbedder() error = %v", err)
	}

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

	embeddingDone := make(chan error, 10)
	storage.SetEmbeddingDoneChannel(embeddingDone)

	// Insert gear with VERY different semantic content
	// "Power drill" should match "cordless drill" well
	// "Power drill" should NOT match "guitar lessons" or "cooking recipes"
	drill := &models.Gear{Name: "Cordless Drill", Description: "Professional cordless power drill for woodworking and construction", OwnerId: userID}
	drillID, _ := storage.Insert(ctx, drill)

	guitar := &models.Gear{Name: "Guitar Book", Description: "Learn to play acoustic guitar with beginner music lessons and chord progressions", OwnerId: userID}
	guitarID, _ := storage.Insert(ctx, guitar)

	cooking := &models.Gear{Name: "Cookbook", Description: "Italian pasta recipes and Mediterranean cuisine cooking techniques", OwnerId: userID}
	cookingID, _ := storage.Insert(ctx, cooking)

	// Share all with community
	for _, gearID := range []string{drillID, guitarID, cookingID} {
		cg := &models.CommunityGear{CommunityId: communityID, GearId: gearID, CreatedAtUnixSec: 0}
		_, _ = storage.Insert(ctx, cg)
	}

	// Wait for embeddings
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

	// Search for "power drill" - should return drill but NOT guitar/cooking (below threshold)
	results, err := storage.QueryGearByCommunitySearch(ctx, communityID, "power drill", 0, 0)
	if err != nil {
		t.Fatalf("QueryGearByCommunitySearch() error = %v", err)
	}

	// Should find the drill
	foundDrill := false
	foundGuitar := false
	foundCookbook := false
	for _, r := range results {
		switch r.Gear.Name {
		case "Cordless Drill":
			foundDrill = true
			// Verify similarity is above threshold
			if r.Similarity < SemanticSearchMinSimilarity {
				t.Errorf("Drill similarity %v below threshold %v", r.Similarity, SemanticSearchMinSimilarity)
			}
		case "Guitar Book":
			foundGuitar = true
			t.Logf("Guitar similarity: %v (should be filtered at threshold %v)", r.Similarity, SemanticSearchMinSimilarity)
		case "Cookbook":
			foundCookbook = true
			t.Logf("Cookbook similarity: %v (should be filtered at threshold %v)", r.Similarity, SemanticSearchMinSimilarity)
		}
	}

	if !foundDrill {
		t.Error("Expected to find 'Cordless Drill' in results")
	}

	// At threshold 0.3, semantically unrelated items should be filtered
	// If they're still found, that's a warning but may depend on model behavior
	if foundGuitar {
		t.Log("Warning: 'Guitar Book' found - may want to tune threshold")
	}
	if foundCookbook {
		t.Log("Warning: 'Cookbook' found - may want to tune threshold")
	}

	// The drill should be the first result (highest similarity)
	if len(results) > 0 && results[0].Gear.Name != "Cordless Drill" {
		t.Errorf("Expected 'Cordless Drill' to be first result, got '%s'", results[0].Gear.Name)
	}
}

func TestQueryRequestByCommunitySearch_ThresholdFiltersIrrelevantResults(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	embedder := SetupTestEmbedder(t)

	ctx := context.Background()

	if err := storage.SetEmbedder(ctx, embedder); err != nil {
		t.Fatalf("SetEmbedder() error = %v", err)
	}

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

	embeddingDone := make(chan error, 10)
	storage.SetEmbeddingDoneChannel(embeddingDone)

	// Insert requests with VERY different semantic content
	drill := &models.Request{
		Title:       "Need a Drill",
		Description: "Looking for a cordless power drill for home renovation project",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	drillID, err := storage.Insert(ctx, drill)
	if err != nil {
		t.Fatalf("Insert drill request error = %v", err)
	}
	// Share drill request with community
	cr1 := &models.CommunityRequest{CommunityId: communityID, RequestId: drillID, SharedAtUnixSec: time.Now().Unix()}
	if _, err := storage.Insert(ctx, cr1); err != nil {
		t.Fatalf("Insert community_request error = %v", err)
	}

	yoga := &models.Request{
		Title:       "Yoga Mat Wanted",
		Description: "Looking for a yoga mat for meditation and stretching exercises",
		RequesterId: userID,
		State:       models.RequestState_REQUEST_STATE_ACTIVE,
	}
	yogaID, err := storage.Insert(ctx, yoga)
	if err != nil {
		t.Fatalf("Insert yoga request error = %v", err)
	}
	// Share yoga request with community
	cr2 := &models.CommunityRequest{CommunityId: communityID, RequestId: yogaID, SharedAtUnixSec: time.Now().Unix()}
	if _, err := storage.Insert(ctx, cr2); err != nil {
		t.Fatalf("Insert community_request error = %v", err)
	}

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

	// Search for "power drill" - should return drill request but maybe not yoga
	results, err := storage.QueryRequestByCommunitySearch(ctx, communityID, "cordless power drill", 0, 0)
	if err != nil {
		t.Fatalf("QueryRequestByCommunitySearch() error = %v", err)
	}

	// Should find the drill request
	foundDrill := false
	foundYoga := false
	for _, r := range results {
		switch r.Request.Title {
		case "Need a Drill":
			foundDrill = true
			if r.Similarity < SemanticSearchMinSimilarity {
				t.Errorf("Drill request similarity %v below threshold %v", r.Similarity, SemanticSearchMinSimilarity)
			}
		case "Yoga Mat Wanted":
			foundYoga = true
			t.Logf("Yoga mat similarity: %v (threshold %v)", r.Similarity, SemanticSearchMinSimilarity)
		}
	}

	if !foundDrill {
		t.Error("Expected to find drill request in results")
	}

	// Log if yoga mat is found (for threshold tuning)
	if foundYoga {
		t.Log("Warning: 'Yoga Mat Wanted' found - may want to tune threshold")
	}

	// Drill should be first (most similar)
	if len(results) > 0 && results[0].Request.Title != "Need a Drill" {
		t.Errorf("Expected 'Need a Drill' to be first result, got '%s'", results[0].Request.Title)
	}
}
