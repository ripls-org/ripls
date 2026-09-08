package storage

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// ============================================================================
// Semantic Search Tests
// ============================================================================.

func TestQueryGearByCommunitySearch_UsesSemanticSearchWhenConfigured(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	embedder := SetupTestEmbedder(t)

	ctx := context.Background()

	// Configure embeddings
	if err := storage.SetEmbedder(ctx, embedder); err != nil {
		t.Fatalf("SetEmbedder() error = %v", err)
	}

	// Create test data: user, community, gear
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

	// Insert gear items (embeddings generated async)
	gear1 := &models.Gear{Name: "Power Drill", Description: "Cordless drill for home projects", OwnerId: userID}
	gear1ID, err := storage.Insert(ctx, gear1)
	if err != nil {
		t.Fatalf("Insert gear1 error = %v", err)
	}

	gear2 := &models.Gear{Name: "Hammer", Description: "Standard claw hammer", OwnerId: userID}
	gear2ID, err := storage.Insert(ctx, gear2)
	if err != nil {
		t.Fatalf("Insert gear2 error = %v", err)
	}

	// Share gear with community
	cg1 := &models.CommunityGear{CommunityId: communityID, GearId: gear1ID, CreatedAtUnixSec: time.Now().Unix()}
	if _, err := storage.Insert(ctx, cg1); err != nil {
		t.Fatalf("Insert community_gear error = %v", err)
	}
	cg2 := &models.CommunityGear{CommunityId: communityID, GearId: gear2ID, CreatedAtUnixSec: time.Now().Unix()}
	if _, err := storage.Insert(ctx, cg2); err != nil {
		t.Fatalf("Insert community_gear error = %v", err)
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
	results, err := storage.QueryGearByCommunitySearch(ctx, communityID, "drill tools", 0, 0)
	if err != nil {
		t.Fatalf("QueryGearByCommunitySearch() error = %v", err)
	}

	// Verify results returned
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestQueryGearByCommunitySearch_FallsBackToTextSearchWhenNotConfigured(t *testing.T) {
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

	// Insert gear
	gear := &models.Gear{Name: "Drill Press", Description: "Heavy duty drill press", OwnerId: userID}
	gearID, err := storage.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("Insert gear error = %v", err)
	}

	// Share with community
	cg := &models.CommunityGear{CommunityId: communityID, GearId: gearID, CreatedAtUnixSec: time.Now().Unix()}
	if _, err := storage.Insert(ctx, cg); err != nil {
		t.Fatalf("Insert community_gear error = %v", err)
	}

	// Search should use text search (no embedder)
	results, err := storage.QueryGearByCommunitySearch(ctx, communityID, "drill", 0, 0)
	if err != nil {
		t.Fatalf("QueryGearByCommunitySearch() error = %v", err)
	}

	// Should find the gear via text matching
	if len(results) != 1 {
		t.Errorf("Expected 1 result from text search, got %d", len(results))
	}
	if len(results) > 0 && results[0].Gear.Name != "Drill Press" {
		t.Errorf("Expected 'Drill Press', got '%s'", results[0].Gear.Name)
	}
}

func TestQueryGearByCommunitySearch_SemanticSearchFiltersByCommunity(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	embedder := SetupTestEmbedder(t)

	ctx := context.Background()

	if err := storage.SetEmbedder(ctx, embedder); err != nil {
		t.Fatalf("SetEmbedder() error = %v", err)
	}

	// Create user and two communities
	user := &models.User{Name: "Test User", Email: "test@example.com"}
	userID, err := storage.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Insert user error = %v", err)
	}

	community1 := &models.Community{Name: "Community 1", CreatorId: userID, OwnerUserId: userID}
	community1ID, err := storage.Insert(ctx, community1)
	if err != nil {
		t.Fatalf("Insert community1 error = %v", err)
	}

	community2 := &models.Community{Name: "Community 2", CreatorId: userID, OwnerUserId: userID}
	community2ID, err := storage.Insert(ctx, community2)
	if err != nil {
		t.Fatalf("Insert community2 error = %v", err)
	}

	// Set up embedding done channel BEFORE inserts
	embeddingDone := make(chan error, 10)
	storage.SetEmbeddingDoneChannel(embeddingDone)

	// Create gear shared with different communities
	gear1 := &models.Gear{Name: "Drill One", Description: "Drill for community 1", OwnerId: userID}
	gear1ID, err := storage.Insert(ctx, gear1)
	if err != nil {
		t.Fatalf("Insert gear1 error = %v", err)
	}

	gear2 := &models.Gear{Name: "Drill Two", Description: "Drill for community 2", OwnerId: userID}
	gear2ID, err := storage.Insert(ctx, gear2)
	if err != nil {
		t.Fatalf("Insert gear2 error = %v", err)
	}

	// Share gear1 with community1
	cg1 := &models.CommunityGear{CommunityId: community1ID, GearId: gear1ID, CreatedAtUnixSec: time.Now().Unix()}
	if _, err := storage.Insert(ctx, cg1); err != nil {
		t.Fatalf("Insert community_gear error = %v", err)
	}

	// Share gear2 with community2
	cg2 := &models.CommunityGear{CommunityId: community2ID, GearId: gear2ID, CreatedAtUnixSec: time.Now().Unix()}
	if _, err := storage.Insert(ctx, cg2); err != nil {
		t.Fatalf("Insert community_gear error = %v", err)
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

	// Search in community1 should only return gear1
	results1, err := storage.QueryGearByCommunitySearch(ctx, community1ID, "drill", 0, 0)
	if err != nil {
		t.Fatalf("QueryGearByCommunitySearch(community1) error = %v", err)
	}

	if len(results1) != 1 {
		t.Errorf("Expected 1 result for community1, got %d", len(results1))
	}
	if len(results1) > 0 && results1[0].Gear.Name != "Drill One" {
		t.Errorf("Expected 'Drill One', got '%s'", results1[0].Gear.Name)
	}

	// Search in community2 should only return gear2
	results2, err := storage.QueryGearByCommunitySearch(ctx, community2ID, "drill", 0, 0)
	if err != nil {
		t.Fatalf("QueryGearByCommunitySearch(community2) error = %v", err)
	}

	if len(results2) != 1 {
		t.Errorf("Expected 1 result for community2, got %d", len(results2))
	}
	if len(results2) > 0 && results2[0].Gear.Name != "Drill Two" {
		t.Errorf("Expected 'Drill Two', got '%s'", results2[0].Gear.Name)
	}
}

func TestQueryGearByCommunitySearch_SemanticSearchOrdersBySimilarity(t *testing.T) {
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

	// Set up embedding done channel BEFORE inserts to capture all signals
	embeddingDone := make(chan error, 10)
	storage.SetEmbeddingDoneChannel(embeddingDone)

	// Insert gear with intentionally different semantic distances from "power drill"
	// All items are power tools so they pass the similarity threshold, but drill ranks highest
	hammer := &models.Gear{Name: "Hammer", Description: "Heavy duty claw hammer for construction and woodworking", OwnerId: userID}
	hammerID, _ := storage.Insert(ctx, hammer)
	saw := &models.Gear{Name: "Circular Saw", Description: "Electric circular saw for cutting wood and lumber", OwnerId: userID}
	sawID, _ := storage.Insert(ctx, saw)
	drill := &models.Gear{Name: "Electric Drill", Description: "Professional cordless power drill for woodworking", OwnerId: userID}
	drillID, _ := storage.Insert(ctx, drill)

	// Share all with community
	for _, gearID := range []string{hammerID, sawID, drillID} {
		cg := &models.CommunityGear{CommunityId: communityID, GearId: gearID, CreatedAtUnixSec: time.Now().Unix()}
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

	// Search for "power drill" - the Electric Drill should be most similar
	results, err := storage.QueryGearByCommunitySearch(ctx, communityID, "power drill", 0, 0)
	if err != nil {
		t.Fatalf("QueryGearByCommunitySearch() error = %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	// Electric Drill should be first (most similar to "power drill")
	if results[0].Gear.Name != "Electric Drill" {
		t.Errorf("Position 0: expected 'Electric Drill' (most similar), got '%s'", results[0].Gear.Name)
	}
}

func TestQueryGearByCommunitySearch_EmptyQueryReturnsAllGear(t *testing.T) {
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

	// Set up embedding done channel BEFORE inserts to capture all signals
	embeddingDone := make(chan error, 10)
	storage.SetEmbeddingDoneChannel(embeddingDone)

	// Insert multiple gear items
	for _, name := range []string{"Drill", "Hammer", "Saw"} {
		gear := &models.Gear{Name: name, Description: "A " + name, OwnerId: userID}
		gearID, _ := storage.Insert(ctx, gear)
		cg := &models.CommunityGear{CommunityId: communityID, GearId: gearID, CreatedAtUnixSec: time.Now().Unix()}
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

	// Empty query should not use semantic search and should return all gear
	results, err := storage.QueryGearByCommunitySearch(ctx, communityID, "", 0, 0)
	if err != nil {
		t.Fatalf("QueryGearByCommunitySearch() error = %v", err)
	}

	// Should return all 3 gear items
	if len(results) != 3 {
		t.Errorf("Expected 3 results for empty query, got %d", len(results))
	}
}

func TestGetEmbedder_ReturnsNilWhenNotConfigured(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	// Don't configure any embedder
	embedder := storage.GetEmbedder()
	if embedder != nil {
		t.Error("GetEmbedder() should return nil when no embedder configured")
	}
}

func TestQueryGearByCommunitySearch_OnlyItemsWithEmbeddingsReturnedInSemanticSearch(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	embedder := SetupTestEmbedder(t)

	ctx := context.Background()

	// Create test data BEFORE configuring embedder
	// so these items won't have embeddings
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

	// Insert gear WITHOUT embeddings (embedder not configured yet)
	gearWithoutEmbedding := &models.Gear{Name: "Old Drill", Description: "Drill without embedding", OwnerId: userID}
	gearWithoutID, _ := storage.Insert(ctx, gearWithoutEmbedding)
	cg1 := &models.CommunityGear{CommunityId: communityID, GearId: gearWithoutID, CreatedAtUnixSec: time.Now().Unix()}
	_, _ = storage.Insert(ctx, cg1)

	// NOW configure embedder
	if err := storage.SetEmbedder(ctx, embedder); err != nil {
		t.Fatalf("SetEmbedder() error = %v", err)
	}

	// Set up embedding done channel BEFORE insert to capture signal
	embeddingDone := make(chan error, 10)
	storage.SetEmbeddingDoneChannel(embeddingDone)

	// Insert gear WITH embedding (after embedder configured)
	gearWithEmbedding := &models.Gear{Name: "New Drill", Description: "Drill with embedding", OwnerId: userID}
	gearWithID, _ := storage.Insert(ctx, gearWithEmbedding)
	cg2 := &models.CommunityGear{CommunityId: communityID, GearId: gearWithID, CreatedAtUnixSec: time.Now().Unix()}
	_, _ = storage.Insert(ctx, cg2)

	// Wait for embedding
	select {
	case <-embeddingDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for embedding")
	}

	// Semantic search should only return items WITH embeddings
	results, err := storage.QueryGearByCommunitySearch(ctx, communityID, "drill", 0, 0)
	if err != nil {
		t.Fatalf("QueryGearByCommunitySearch() error = %v", err)
	}

	// Only the gear with embedding should be returned
	if len(results) != 1 {
		t.Errorf("Expected 1 result (only item with embedding), got %d", len(results))
	}
	if len(results) > 0 && results[0].Gear.Name != "New Drill" {
		t.Errorf("Expected 'New Drill' (with embedding), got '%s'", results[0].Gear.Name)
	}
}
