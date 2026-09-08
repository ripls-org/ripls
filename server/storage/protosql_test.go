package storage

import (
	"context"
	"errors"
	"os"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestMain sets up and tears down the shared PostgreSQL instance for all tests.
func TestMain(m *testing.M) {
	// Run all tests
	code := m.Run()

	// Clean up the shared PostgreSQL instance
	CleanupSharedPostgreSQL()

	os.Exit(code)
}

// TestProtoSQLStorage executes all storage tests against PostgreSQL.
func TestProtoSQLStorage(t *testing.T) {
	testFunctions := []struct {
		fn   func(t *testing.T, storage *ProtoSQLStorage)
		name string
	}{
		{testInsertAndGetByID, "InsertAndGetByID"},
		{testQueryByField, "QueryByField"},
		{testQueryByFields, "QueryByFields"},
		{testGetByIDNotFound, "GetByIDNotFound"},
		{testUpdateNotFound, "UpdateNotFound"},
		{testDeleteNotFound, "DeleteNotFound"},
		{testProtoReflection, "ProtoReflection"},
		{testReservedKeywordTable, "ReservedKeywordTable"},
		{testListAll, "ListAll"},
		{testHasAnyUsers, "HasAnyUsers"},
		{testRepeatedFields, "RepeatedFields"},
		{testFlattenedNestedFields, "FlattenedNestedFields"},
		{testSpatialIndexCreation, "SpatialIndexCreation"},
		{testQueryByProximity, "QueryByProximity"},
		{testQueryGearByCommunitySearch, "QueryGearByCommunitySearch"},
		{testDeleteByField, "DeleteByField"},
		{testDeleteByFieldIn, "DeleteByFieldIn"},
		{testQueryDistinctField, "QueryDistinctField"},
		{testCountByField, "CountByField"},
		{testIdentifierInjectionRejected, "IdentifierInjectionRejected"},
	}

	for _, testFunc := range testFunctions {
		t.Run(testFunc.name, func(t *testing.T) {
			storage, cleanup := SetupTestStorage(t)
			defer cleanup()
			testFunc.fn(t, storage)
		})
	}
}

// Individual test functions that take database name and storage as parameters.

func testInsertAndGetByID(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Create test gear (using stored type)
	testGear := &models.Gear{
		Name:        "Test Drill",
		Description: "A high-quality power drill",
	}

	// Insert gear
	id, err := storage.Insert(ctx, testGear)
	if err != nil {
		t.Fatalf("Failed to insert gear: %v", err)
	}

	if id == "" {
		t.Fatalf("Expected non-empty ID from insert")
	}

	// Verify the ID was set on the original message
	if testGear.Id != id {
		t.Errorf("Expected gear ID to be set to %s, got %s", id, testGear.Id)
	}

	// Retrieve gear
	retrievedGear := &models.Gear{}
	err = storage.GetByID(ctx, id, retrievedGear)
	if err != nil {
		t.Fatalf("Failed to get gear by ID: %v", err)
	}

	// Verify all fields
	if retrievedGear.Id != testGear.Id {
		t.Errorf("Expected ID %s, got %s", testGear.Id, retrievedGear.Id)
	}
	if retrievedGear.Name != testGear.Name {
		t.Errorf("Expected name %s, got %s", testGear.Name, retrievedGear.Name)
	}
	if retrievedGear.Description != testGear.Description {
		t.Errorf("Expected description %s, got %s", testGear.Description, retrievedGear.Description)
	}
}

func testQueryByField(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Insert multiple gear items (using stored type)
	gear1 := &models.Gear{
		Name:        "Drill Model A",
		Description: "First drill",
	}
	gear2 := &models.Gear{
		Name:        "Drill Model B",
		Description: "Second drill",
	}
	gear3 := &models.Gear{
		Name:        "Saw Model A",
		Description: "A saw tool",
	}

	_, err := storage.Insert(ctx, gear1)
	if err != nil {
		t.Fatalf("Failed to insert gear1: %v", err)
	}
	_, err = storage.Insert(ctx, gear2)
	if err != nil {
		t.Fatalf("Failed to insert gear2: %v", err)
	}
	_, err = storage.Insert(ctx, gear3)
	if err != nil {
		t.Fatalf("Failed to insert gear3: %v", err)
	}

	// Query by name
	template := &models.Gear{}
	results, err := storage.QueryByField(ctx, "name", "Drill Model A", template)
	if err != nil {
		t.Fatalf("Failed to query by name: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	resultGear := results[0].(*models.Gear)
	if resultGear.Name != "Drill Model A" {
		t.Errorf("Expected name 'Drill Model A', got %s", resultGear.Name)
	}
}

func testQueryByFields(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Create test users
	user1 := &models.User{
		Email: "user1@example.com",
		Name:  "User One",
		Role:  models.Role_ROLE_USER,
	}
	user2 := &models.User{
		Email: "user2@example.com",
		Name:  "User Two",
		Role:  models.Role_ROLE_USER,
	}

	user1ID, err := storage.Insert(ctx, user1)
	if err != nil {
		t.Fatalf("Failed to insert user1: %v", err)
	}
	user2ID, err := storage.Insert(ctx, user2)
	if err != nil {
		t.Fatalf("Failed to insert user2: %v", err)
	}

	// Create test communities
	community1 := &models.Community{
		Name:        "Community A",
		CreatorId:   user1ID,
		OwnerUserId: user1ID,
	}
	community2 := &models.Community{
		Name:        "Community B",
		CreatorId:   user1ID,
		OwnerUserId: user1ID,
	}
	community3 := &models.Community{
		Name:        "Community C",
		CreatorId:   user2ID,
		OwnerUserId: user2ID,
	}

	community1ID, err := storage.Insert(ctx, community1)
	if err != nil {
		t.Fatalf("Failed to insert community1: %v", err)
	}
	community2ID, err := storage.Insert(ctx, community2)
	if err != nil {
		t.Fatalf("Failed to insert community2: %v", err)
	}
	community3ID, err := storage.Insert(ctx, community3)
	if err != nil {
		t.Fatalf("Failed to insert community3: %v", err)
	}

	// Create memberships with different combinations
	memberships := []*models.CommunityUser{
		{CommunityId: community1ID, UserId: user1ID, InviterId: user1ID},
		{CommunityId: community1ID, UserId: user2ID, InviterId: user1ID},
		{CommunityId: community2ID, UserId: user1ID, InviterId: user1ID},
		{CommunityId: community3ID, UserId: user2ID, InviterId: user2ID},
	}

	for _, membership := range memberships {
		_, err := storage.Insert(ctx, membership)
		if err != nil {
			t.Fatalf("Failed to insert membership: %v", err)
		}
	}

	// Test 1: Query by two fields (community_id and user_id)
	template := &models.CommunityUser{}
	results, err := storage.QueryByFields(ctx, map[string]any{
		"community_id": community1ID,
		"user_id":      user1ID,
	}, template)
	if err != nil {
		t.Fatalf("Failed to query by fields: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result for community1+user1, got %d", len(results))
	}

	membership := results[0].(*models.CommunityUser)
	if membership.CommunityId != community1ID {
		t.Errorf("Expected community_id %s, got %s", community1ID, membership.CommunityId)
	}
	if membership.UserId != user1ID {
		t.Errorf("Expected user_id %s, got %s", user1ID, membership.UserId)
	}

	// Test 2: Query that returns multiple results
	results, err = storage.QueryByFields(ctx, map[string]any{
		"community_id": community1ID,
	}, template)
	if err != nil {
		t.Fatalf("Failed to query by single field: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results for community1, got %d", len(results))
	}

	// Test 3: Query with no results
	results, err = storage.QueryByFields(ctx, map[string]any{
		"community_id": community2ID,
		"user_id":      user2ID, // user2 is not a member of community2
	}, template)
	if err != nil {
		t.Fatalf("Failed to query with no matches: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results for non-existent combination, got %d", len(results))
	}

	// Test 4: Query with three fields
	results, err = storage.QueryByFields(ctx, map[string]any{
		"community_id": community1ID,
		"user_id":      user2ID,
		"inviter_id":   user1ID,
	}, template)
	if err != nil {
		t.Fatalf("Failed to query by three fields: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result for three-field query, got %d", len(results))
	}

	// Test 5: Error case - empty field map
	_, err = storage.QueryByFields(ctx, map[string]any{}, template)
	if err == nil {
		t.Errorf("Expected error for empty field map")
	}
	if err != nil && err.Error() != "at least one field must be specified" {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

func testGetByIDNotFound(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Try to get non-existent gear
	gear := &models.Gear{}
	err := storage.GetByID(ctx, "non-existent-id", gear)
	if err == nil {
		t.Fatalf("Expected error when getting non-existent gear")
	}

	if !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("Expected errors.Is(err, ErrRecordNotFound) = true, got error: %v", err)
	}
}

func testUpdateNotFound(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Try to update a gear with an ID that doesn't exist.
	gear := &models.Gear{
		Id:   "non-existent-id",
		Name: "Phantom",
	}
	err := storage.Update(ctx, gear)
	if err == nil {
		t.Fatalf("Expected error when updating non-existent gear")
	}

	if !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("Expected errors.Is(err, ErrRecordNotFound) = true, got error: %v", err)
	}
}

func testDeleteNotFound(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Try to delete a gear with an ID that doesn't exist.
	gear := &models.Gear{
		Id: "non-existent-id",
	}
	err := storage.Delete(ctx, gear)
	if err == nil {
		t.Fatalf("Expected error when deleting non-existent gear")
	}

	if !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("Expected errors.Is(err, ErrRecordNotFound) = true, got error: %v", err)
	}
}

func testProtoReflection(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Test that the reflection-based storage works with different field types (using stored type)
	testGear := &models.Gear{
		Name:        "Reflection Test Gear",
		Description: "Testing protobuf reflection",
	}

	// Insert and verify reflection works
	id, err := storage.Insert(ctx, testGear)
	if err != nil {
		t.Fatalf("Failed to insert gear for reflection test: %v", err)
	}

	// Verify that protobuf message descriptor was used correctly
	descriptor := testGear.ProtoReflect().Descriptor()
	fields := descriptor.Fields()

	// Check that we have the expected number of fields
	expectedFieldCount := 2 // id, name, description (3 total, but id is auto-generated)
	if fields.Len() < expectedFieldCount {
		t.Errorf("Expected at least %d fields in GearStored message, got %d", expectedFieldCount, fields.Len())
	}

	// Verify retrieval works with reflection
	retrievedGear := &models.Gear{}
	err = storage.GetByID(ctx, id, retrievedGear)
	if err != nil {
		t.Fatalf("Failed to retrieve gear for reflection test: %v", err)
	}

	if retrievedGear.Name != testGear.Name {
		t.Errorf("Reflection test failed: expected name %s, got %s", testGear.Name, retrievedGear.Name)
	}
}

func testReservedKeywordTable(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Test with User proto which creates "user" table - a reserved keyword in PostgreSQL
	testUser := &models.User{
		Email: "test@example.com",
		Name:  "Test User",
		Role:  models.Role_ROLE_USER,
	}

	// Insert user (this would fail without proper table name quoting in PostgreSQL)
	id, err := storage.Insert(ctx, testUser)
	if err != nil {
		t.Fatalf("Failed to insert user with reserved keyword table: %v", err)
	}

	if id == "" {
		t.Fatalf("Expected non-empty ID from user insert")
	}

	// Verify the ID was set on the original message
	if testUser.Id != id {
		t.Errorf("Expected user ID to be set to %s, got %s", id, testUser.Id)
	}

	// Retrieve user (this would also fail without proper table name quoting)
	retrievedUser := &models.User{}
	err = storage.GetByID(ctx, id, retrievedUser)
	if err != nil {
		t.Fatalf("Failed to get user by ID from reserved keyword table: %v", err)
	}

	// Verify all fields
	if retrievedUser.Id != testUser.Id {
		t.Errorf("Expected ID %s, got %s", testUser.Id, retrievedUser.Id)
	}
	if retrievedUser.Email != testUser.Email {
		t.Errorf("Expected email %s, got %s", testUser.Email, retrievedUser.Email)
	}
	if retrievedUser.Name != testUser.Name {
		t.Errorf("Expected name %s, got %s", testUser.Name, retrievedUser.Name)
	}
	if retrievedUser.Role != testUser.Role {
		t.Errorf("Expected role %v, got %v", testUser.Role, retrievedUser.Role)
	}

	// Test QueryByField on reserved keyword table
	template := &models.User{}
	results, err := storage.QueryByField(ctx, "email", "test@example.com", template)
	if err != nil {
		t.Fatalf("Failed to query user by email from reserved keyword table: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 user result, got %d", len(results))
	}

	resultUser := results[0].(*models.User)
	if resultUser.Email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got %s", resultUser.Email)
	}
}

func testListAll(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Test empty list
	template := &models.Gear{}
	results, err := storage.ListAll(ctx, template)
	if err != nil {
		t.Fatalf("Failed to list all from empty table: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results from empty table, got %d", len(results))
	}

	// Insert multiple gear items
	testGearItems := []*models.Gear{
		{
			Name:        "Drill A",
			Description: "Power drill A",
			OwnerId:     "user123",
		},
		{
			Name:        "Drill B",
			Description: "Power drill B",
			OwnerId:     "user123",
		},
		{
			Name:        "Saw",
			Description: "Circular saw",
			OwnerId:     "user456",
		},
	}

	insertedIDs := make(map[string]bool)
	for _, gear := range testGearItems {
		id, err := storage.Insert(ctx, gear)
		if err != nil {
			t.Fatalf("Failed to insert gear: %v", err)
		}
		insertedIDs[id] = true
	}

	// List all gear
	results, err = storage.ListAll(ctx, template)
	if err != nil {
		t.Fatalf("Failed to list all gear: %v", err)
	}

	if len(results) != len(testGearItems) {
		t.Fatalf("Expected %d results, got %d", len(testGearItems), len(results))
	}

	// Verify all inserted gear items are in the results
	foundIDs := make(map[string]bool)
	for _, result := range results {
		gear := result.(*models.Gear)
		foundIDs[gear.Id] = true

		// Verify all fields are populated
		if gear.Id == "" {
			t.Errorf("Expected non-empty ID")
		}
		if gear.Name == "" {
			t.Errorf("Expected non-empty Name")
		}
		if gear.Description == "" {
			t.Errorf("Expected non-empty Description")
		}
		if gear.OwnerId == "" {
			t.Errorf("Expected non-empty OwnerId")
		}
	}

	// Verify all inserted IDs are found
	for id := range insertedIDs {
		if !foundIDs[id] {
			t.Errorf("Inserted gear ID %s not found in ListAll results", id)
		}
	}

	// Test ListAll with different message type (User)
	userTemplate := &models.User{}
	userResults, err := storage.ListAll(ctx, userTemplate)
	if err != nil {
		t.Fatalf("Failed to list all users: %v", err)
	}

	// Should be empty since we haven't inserted any users in this test
	if len(userResults) != 0 {
		t.Errorf("Expected 0 user results, got %d", len(userResults))
	}

	// Insert a user and verify ListAll returns it
	testUser := &models.User{
		Email: "listall@example.com",
		Name:  "List All Test User",
		Role:  models.Role_ROLE_USER,
	}
	userID, err := storage.Insert(ctx, testUser)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	userResults, err = storage.ListAll(ctx, userTemplate)
	if err != nil {
		t.Fatalf("Failed to list all users after insert: %v", err)
	}

	if len(userResults) != 1 {
		t.Fatalf("Expected 1 user result, got %d", len(userResults))
	}

	retrievedUser := userResults[0].(*models.User)
	if retrievedUser.Id != userID {
		t.Errorf("Expected user ID %s, got %s", userID, retrievedUser.Id)
	}
	if retrievedUser.Email != testUser.Email {
		t.Errorf("Expected email %s, got %s", testUser.Email, retrievedUser.Email)
	}
}

func testHasAnyUsers(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Test 1: No users exist initially
	hasUsers, err := storage.HasAnyUsers(ctx)
	if err != nil {
		t.Fatalf("Failed to check for users: %v", err)
	}

	if hasUsers {
		t.Errorf("Expected no users initially, but HasAnyUsers returned true")
	}

	// Test 2: Insert a user
	testUser := &models.User{
		Email: "test@example.com",
		Name:  "Test User",
		Role:  models.Role_ROLE_USER,
	}

	_, err = storage.Insert(ctx, testUser)
	if err != nil {
		t.Fatalf("Failed to insert user: %v", err)
	}

	// Test 3: Check that users exist now
	hasUsers, err = storage.HasAnyUsers(ctx)
	if err != nil {
		t.Fatalf("Failed to check for users after insert: %v", err)
	}

	if !hasUsers {
		t.Errorf("Expected users to exist after insert, but HasAnyUsers returned false")
	}

	// Test 4: Insert another user and verify still returns true
	anotherUser := &models.User{
		Email: "another@example.com",
		Name:  "Another User",
		Role:  models.Role_ROLE_USER,
	}

	_, err = storage.Insert(ctx, anotherUser)
	if err != nil {
		t.Fatalf("Failed to insert second user: %v", err)
	}

	hasUsers, err = storage.HasAnyUsers(ctx)
	if err != nil {
		t.Fatalf("Failed to check for users after second insert: %v", err)
	}

	if !hasUsers {
		t.Errorf("Expected users to exist after second insert, but HasAnyUsers returned false")
	}
}
