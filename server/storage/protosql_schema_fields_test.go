package storage

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestAlternateStorageTypes verifies that non-default storage type configurations are respected.
func TestAlternateStorageTypes(t *testing.T) {
	t.Run("only allows registered alternate types", func(t *testing.T) {
		ctx := context.Background()

		// Use shared container with a unique database
		connStr, dbCleanup := SetupTestDatabase(t)
		defer dbCleanup()

		// Create storage with only User, not Gear
		alternateTypes := []TypeConfig{
			{MessageType: &models.User{}, TableName: "user"},
		}

		storage, err := InitializePostgreSQLDatabase(t.Context(), connStr, alternateTypes)
		if err != nil {
			t.Fatalf("Failed to initialize with alternate types: %v", err)
		}
		defer storage.Close()

		// User should work
		testUser := &models.User{
			Email: "alternate@example.com",
			Name:  "Alternate User",
			Role:  models.Role_ROLE_USER,
		}

		userID, err := storage.Insert(ctx, testUser)
		if err != nil {
			t.Fatalf("Expected User to be allowed: %v", err)
		}

		// Verify retrieval works
		retrievedUser := &models.User{}
		err = storage.GetByID(ctx, userID, retrievedUser)
		if err != nil {
			t.Fatalf("Failed to retrieve user: %v", err)
		}

		if retrievedUser.Email != testUser.Email {
			t.Errorf("Expected email %s, got %s", testUser.Email, retrievedUser.Email)
		}

		// Gear should NOT work (not registered)
		testGear := &models.Gear{
			Name: "Should Fail",
		}

		_, err = storage.Insert(ctx, testGear)
		if err == nil {
			t.Fatal("Expected error when inserting non-registered Gear type")
		}

		expectedMsg := "message type ripls.models.Gear is not registered for storage"
		if err.Error() != expectedMsg {
			t.Errorf("Expected error '%s', got '%s'", expectedMsg, err.Error())
		}
	})

	t.Run("custom table names are respected", func(t *testing.T) {
		ctx := context.Background()

		// Use shared container with a unique database
		connStr, dbCleanup := SetupTestDatabase(t)
		defer dbCleanup()

		// Create storage with custom table name
		customTypes := []TypeConfig{
			{MessageType: &models.Gear{}, TableName: "custom_gear_table"},
		}

		storage, err := InitializePostgreSQLDatabase(t.Context(), connStr, customTypes)
		if err != nil {
			t.Fatalf("Failed to initialize with custom table names: %v", err)
		}
		defer storage.Close()

		// Insert gear
		testGear := &models.Gear{
			Name:        "Custom Table Gear",
			Description: "Stored in custom table",
		}

		gearID, err := storage.Insert(ctx, testGear)
		if err != nil {
			t.Fatalf("Failed to insert into custom table: %v", err)
		}

		// Verify we can retrieve it (proves the custom table name worked)
		retrievedGear := &models.Gear{}
		err = storage.GetByID(ctx, gearID, retrievedGear)
		if err != nil {
			t.Fatalf("Failed to retrieve from custom table: %v", err)
		}

		if retrievedGear.Name != testGear.Name {
			t.Errorf("Expected name %s, got %s", testGear.Name, retrievedGear.Name)
		}
	})
}

func testRepeatedFields(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Test insert with repeated field populated
	testGear := &models.Gear{
		Name:        "Camera with Media",
		Description: "Camera with multiple photos",
		MediaIds:    []string{"media-001", "media-002", "media-003"},
		OwnerId:     "owner789",
		State:       models.GearState_GEAR_STATE_AVAILABLE,
	}

	// Insert gear with repeated field
	id, err := storage.Insert(ctx, testGear)
	if err != nil {
		t.Fatalf("Failed to insert gear with repeated field: %v", err)
	}

	if id == "" {
		t.Fatalf("Expected non-empty ID from insert")
	}

	// Retrieve and verify repeated field is preserved
	retrievedGear := &models.Gear{}
	err = storage.GetByID(ctx, id, retrievedGear)
	if err != nil {
		t.Fatalf("Failed to get gear by ID: %v", err)
	}

	// Verify basic fields
	if retrievedGear.Name != testGear.Name {
		t.Errorf("Expected name %s, got %s", testGear.Name, retrievedGear.Name)
	}

	// Verify repeated field is correctly populated
	if len(retrievedGear.MediaIds) != len(testGear.MediaIds) {
		t.Fatalf("Expected %d media IDs, got %d", len(testGear.MediaIds), len(retrievedGear.MediaIds))
	}

	for i, mediaID := range testGear.MediaIds {
		if retrievedGear.MediaIds[i] != mediaID {
			t.Errorf("Expected media ID[%d] = %s, got %s", i, mediaID, retrievedGear.MediaIds[i])
		}
	}

	// Test update with modified repeated field
	retrievedGear.MediaIds = []string{"media-004", "media-005"}
	retrievedGear.Description = "Updated description"

	err = storage.Update(ctx, retrievedGear)
	if err != nil {
		t.Fatalf("Failed to update gear with repeated field: %v", err)
	}

	// Retrieve again and verify updated repeated field
	updatedGear := &models.Gear{}
	err = storage.GetByID(ctx, id, updatedGear)
	if err != nil {
		t.Fatalf("Failed to get updated gear: %v", err)
	}

	if len(updatedGear.MediaIds) != 2 {
		t.Fatalf("Expected 2 media IDs after update, got %d", len(updatedGear.MediaIds))
	}

	if updatedGear.MediaIds[0] != "media-004" || updatedGear.MediaIds[1] != "media-005" {
		t.Errorf("Media IDs not updated correctly: got %v", updatedGear.MediaIds)
	}

	// Test with empty repeated field
	emptyGear := &models.Gear{
		Name:     "Gear with no media",
		MediaIds: []string{},
	}

	emptyID, err := storage.Insert(ctx, emptyGear)
	if err != nil {
		t.Fatalf("Failed to insert gear with empty repeated field: %v", err)
	}

	retrievedEmptyGear := &models.Gear{}
	err = storage.GetByID(ctx, emptyID, retrievedEmptyGear)
	if err != nil {
		t.Fatalf("Failed to get gear with empty repeated field: %v", err)
	}

	if len(retrievedEmptyGear.MediaIds) != 0 {
		t.Errorf("Expected empty MediaIds, got %v", retrievedEmptyGear.MediaIds)
	}

	// Test ListAll with repeated fields
	template := &models.Gear{}
	allGear, err := storage.ListAll(ctx, template)
	if err != nil {
		t.Fatalf("Failed to list all gear: %v", err)
	}

	// Find our gear with media in the list
	var foundGearWithMedia *models.Gear
	for _, gear := range allGear {
		g := gear.(*models.Gear)
		if g.Id == id {
			foundGearWithMedia = g
			break
		}
	}

	if foundGearWithMedia == nil {
		t.Fatalf("Could not find gear with media in ListAll results")
	}

	if len(foundGearWithMedia.MediaIds) != 2 {
		t.Errorf("Expected 2 media IDs in ListAll result, got %d", len(foundGearWithMedia.MediaIds))
	}
}

// TestSchemaEvolution tests that missing columns are automatically added to existing tables.
func TestSchemaEvolution(t *testing.T) {
	t.Run("PostgreSQL", func(t *testing.T) {
		ctx := context.Background()

		// Use shared container with a unique database
		connectionString, dbCleanup := SetupTestDatabase(t)
		defer dbCleanup()

		// Manually create an "old" schema with missing columns
		db, err := sql.Open("postgres", connectionString)
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}

		// Create a gear table with only id, name, and binary_proto (missing description, owner_id, etc.)
		oldSchemaSQL := `CREATE TABLE "gear" (
			id TEXT PRIMARY KEY,
			name TEXT,
			binary_proto BYTEA NOT NULL
		);`
		_, err = db.Exec(oldSchemaSQL)
		if err != nil {
			t.Fatalf("Failed to create old schema table: %v", err)
		}

		// Insert a gear using the old schema
		testGear := &models.Gear{
			Id:          "old-gear-id",
			Name:        "Old Gear",
			Description: "This was created with old schema",
			OwnerId:     "owner123",
		}

		protoData, err := proto.Marshal(testGear)
		if err != nil {
			t.Fatalf("Failed to marshal proto: %v", err)
		}

		_, err = db.Exec("INSERT INTO gear (id, name, binary_proto) VALUES ($1, $2, $3)",
			testGear.Id, testGear.Name, protoData)
		if err != nil {
			t.Fatalf("Failed to insert old gear: %v", err)
		}
		db.Close()

		// Now initialize with the full schema - this should add missing columns
		storage, err := InitializePostgreSQLDatabase(t.Context(), connectionString, DefaultStorageTypes())
		if err != nil {
			t.Fatalf("Failed to initialize with full schema: %v", err)
		}
		defer storage.Close()

		// Verify the old gear can still be retrieved
		retrievedGear := &models.Gear{}
		err = storage.GetByID(ctx, "old-gear-id", retrievedGear)
		if err != nil {
			t.Fatalf("Failed to retrieve old gear after schema evolution: %v", err)
		}

		if retrievedGear.Name != "Old Gear" {
			t.Errorf("Expected name 'Old Gear', got '%s'", retrievedGear.Name)
		}

		if retrievedGear.Description != "This was created with old schema" {
			t.Errorf("Expected description from binary_proto, got '%s'", retrievedGear.Description)
		}

		// Insert a new gear using the full schema
		newGear := &models.Gear{
			Name:        "New Gear",
			Description: "Created after schema evolution",
			OwnerId:     "owner456",
		}

		newID, err := storage.Insert(ctx, newGear)
		if err != nil {
			t.Fatalf("Failed to insert new gear after schema evolution: %v", err)
		}

		// Retrieve and verify the new gear
		retrievedNewGear := &models.Gear{}
		err = storage.GetByID(ctx, newID, retrievedNewGear)
		if err != nil {
			t.Fatalf("Failed to retrieve new gear: %v", err)
		}

		if retrievedNewGear.Name != "New Gear" {
			t.Errorf("Expected name 'New Gear', got '%s'", retrievedNewGear.Name)
		}

		if retrievedNewGear.Description != "Created after schema evolution" {
			t.Errorf("Expected description 'Created after schema evolution', got '%s'", retrievedNewGear.Description)
		}

		if retrievedNewGear.OwnerId != "owner456" {
			t.Errorf("Expected owner_id 'owner456', got '%s'", retrievedNewGear.OwnerId)
		}
	})
}

func testFlattenedNestedFields(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Create location with nested Geolocation and Address messages
	testLocation := &models.Location{
		Geolocation: &models.Geolocation{
			LatitudeDeg:  30.2672, // Austin, TX
			LongitudeDeg: -97.7431,
		},
		Address: &models.Address{
			RegionCode:   "US",
			PostalCode:   "78701",
			Locality:     "Austin",
			AddressLines: []string{"100 Congress Ave"},
		},
		CreatedAtUnixSec: 1234567890,
		UpdatedAtUnixSec: 1234567890,
	}

	// Insert location
	id, err := storage.Insert(ctx, testLocation)
	if err != nil {
		t.Fatalf("Failed to insert location with nested fields: %v", err)
	}

	if id == "" {
		t.Fatalf("Expected non-empty ID from insert")
	}

	// Test 1: Query the SQL table directly to verify flattened columns exist and have values
	query := fmt.Sprintf("SELECT geolocation_latitude_deg, geolocation_longitude_deg, address_region_code, address_postal_code, address_locality FROM %s WHERE id = %s",
		"\"location\"", storage.dbSpec.Placeholder(1))

	var lat, lon float64
	var regionCode, postalCode, locality string
	err = storage.db.QueryRowContext(ctx, query, id).Scan(&lat, &lon, &regionCode, &postalCode, &locality)
	if err != nil {
		t.Fatalf("Failed to query flattened fields directly from SQL: %v", err)
	}

	if lat != testLocation.Geolocation.LatitudeDeg {
		t.Errorf("Expected flattened latitude %f, got %f", testLocation.Geolocation.LatitudeDeg, lat)
	}
	if lon != testLocation.Geolocation.LongitudeDeg {
		t.Errorf("Expected flattened longitude %f, got %f", testLocation.Geolocation.LongitudeDeg, lon)
	}
	if regionCode != testLocation.Address.RegionCode {
		t.Errorf("Expected flattened region code %s, got %s", testLocation.Address.RegionCode, regionCode)
	}
	if postalCode != testLocation.Address.PostalCode {
		t.Errorf("Expected flattened postal code %s, got %s", testLocation.Address.PostalCode, postalCode)
	}
	if locality != testLocation.Address.Locality {
		t.Errorf("Expected flattened locality %s, got %s", testLocation.Address.Locality, locality)
	}

	// Test 2: Query by a flattened field using QueryByField
	template := &models.Location{}
	results, err := storage.QueryByField(ctx, "address_locality", "Austin", template)
	if err != nil {
		t.Fatalf("Failed to query by flattened field address_locality: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result when querying by flattened locality, got %d", len(results))
	}

	if len(results) > 0 {
		retrievedLocation := results[0].(*models.Location)
		if retrievedLocation.Id != id {
			t.Errorf("Expected to find location %s, got %s", id, retrievedLocation.Id)
		}
		// Verify nested structures are properly reconstructed from binary_proto
		if retrievedLocation.Address == nil || retrievedLocation.Address.Locality != "Austin" {
			t.Errorf("Expected retrieved location to have locality Austin")
		}
	}

	// Test 3: Query by multiple flattened fields using QueryByFields
	results, err = storage.QueryByFields(ctx, map[string]any{
		"address_region_code": "US",
		"address_postal_code": "78701",
	}, template)
	if err != nil {
		t.Fatalf("Failed to query by multiple flattened fields: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result when querying by flattened address fields, got %d", len(results))
	}

	// Test 4: Verify scalar fields still work
	if retrievedLocation := results[0].(*models.Location); retrievedLocation.CreatedAtUnixSec != testLocation.CreatedAtUnixSec {
		t.Errorf("Expected created_at %d, got %d",
			testLocation.CreatedAtUnixSec, retrievedLocation.CreatedAtUnixSec)
	}
}
