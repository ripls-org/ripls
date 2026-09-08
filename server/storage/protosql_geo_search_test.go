package storage

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func testSpatialIndexCreation(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Insert a location to ensure table and indices exist
	testLocation := &models.Location{
		Geolocation: &models.Geolocation{
			LatitudeDeg:  37.7749, // San Francisco
			LongitudeDeg: -122.4194,
		},
		Address: &models.Address{
			Locality:   "San Francisco",
			RegionCode: "US",
		},
	}

	_, err := storage.Insert(ctx, testLocation)
	if err != nil {
		t.Fatalf("Failed to insert location: %v", err)
	}

	// Query the database to verify indices exist
	query := "SELECT indexname FROM pg_indexes WHERE tablename='location' AND indexname LIKE 'idx_location_%'"

	rows, err := storage.db.QueryContext(ctx, query)
	if err != nil {
		t.Fatalf("Failed to query for indices: %v", err)
	}
	defer rows.Close()

	// Collect index names
	var indexNames []string
	for rows.Next() {
		var indexName string
		if err := rows.Scan(&indexName); err != nil {
			t.Fatalf("Failed to scan index name: %v", err)
		}
		indexNames = append(indexNames, indexName)
	}
	// Without this, an iteration error would present as "no indices found" and
	// the assertions below would fail with a misleading message.
	if err := rows.Err(); err != nil {
		t.Fatalf("Failed to iterate indices: %v", err)
	}

	// Verify at least the lat/lon indices exist
	// We expect idx_location_geolocation_latitude_deg and idx_location_geolocation_longitude_deg
	foundLatIndex := false
	foundLonIndex := false
	for _, name := range indexNames {
		if name == "idx_location_geolocation_latitude_deg" {
			foundLatIndex = true
		}
		if name == "idx_location_geolocation_longitude_deg" {
			foundLonIndex = true
		}
	}

	if !foundLatIndex {
		t.Errorf("Expected to find latitude index, indices found: %v", indexNames)
	}
	if !foundLonIndex {
		t.Errorf("Expected to find longitude index, indices found: %v", indexNames)
	}
}

func testQueryByProximity(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Create test locations at various distances from Austin, TX (30.2672, -97.7431)
	testLocations := []*models.Location{
		{
			// Austin city center - 0km
			Geolocation: &models.Geolocation{
				LatitudeDeg:  30.2672,
				LongitudeDeg: -97.7431,
			},
			Address: &models.Address{
				Locality:   "Austin",
				RegionCode: "US",
			},
		},
		{
			// ~5km north of Austin
			Geolocation: &models.Geolocation{
				LatitudeDeg:  30.3072,
				LongitudeDeg: -97.7431,
			},
			Address: &models.Address{
				Locality:   "North Austin",
				RegionCode: "US",
			},
		},
		{
			// ~10km east of Austin
			Geolocation: &models.Geolocation{
				LatitudeDeg:  30.2672,
				LongitudeDeg: -97.6431,
			},
			Address: &models.Address{
				Locality:   "East Austin",
				RegionCode: "US",
			},
		},
		{
			// San Antonio - ~130km away
			Geolocation: &models.Geolocation{
				LatitudeDeg:  29.4241,
				LongitudeDeg: -98.4936,
			},
			Address: &models.Address{
				Locality:   "San Antonio",
				RegionCode: "US",
			},
		},
	}

	// Insert all locations
	insertedIDs := make([]string, len(testLocations))
	for i, loc := range testLocations {
		id, err := storage.Insert(ctx, loc)
		if err != nil {
			t.Fatalf("Failed to insert location %d: %v", i, err)
		}
		insertedIDs[i] = id
	}

	// Test 1: Query within 6km radius - should find Austin center and North Austin
	centerLat := 30.2672
	centerLon := -97.7431
	radiusMeters := 6000.0

	results, err := storage.QueryByProximity(ctx, centerLat, centerLon, radiusMeters, &models.Location{})
	if err != nil {
		t.Fatalf("Failed to query by proximity: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results within 6km, got %d", len(results))
	}

	// Verify results are sorted by distance
	if len(results) > 1 {
		for i := 0; i < len(results)-1; i++ {
			if results[i].DistanceMeters > results[i+1].DistanceMeters {
				t.Errorf("Results not sorted by distance: %f > %f",
					results[i].DistanceMeters, results[i+1].DistanceMeters)
			}
		}
	}

	// Test 2: Query within 15km radius - should find 3 locations
	results, err = storage.QueryByProximity(ctx, centerLat, centerLon, 15000.0, &models.Location{})
	if err != nil {
		t.Fatalf("Failed to query by proximity (15km): %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results within 15km, got %d", len(results))
	}

	// Test 3: Query within 200km radius - should find all 4 locations
	results, err = storage.QueryByProximity(ctx, centerLat, centerLon, 200000.0, &models.Location{})
	if err != nil {
		t.Fatalf("Failed to query by proximity (200km): %v", err)
	}

	if len(results) != 4 {
		t.Errorf("Expected 4 results within 200km, got %d", len(results))
	}

	// Test 4: Verify distance calculation is reasonable
	// First result should be very close to center (Austin city center)
	if len(results) > 0 {
		firstResult := results[0]
		if firstResult.DistanceMeters > 100 { // Should be < 100m from center
			t.Errorf("First result should be close to center, got distance: %f meters",
				firstResult.DistanceMeters)
		}

		// Verify lat/lon are populated
		if firstResult.LatitudeDeg == 0 || firstResult.LongitudeDeg == 0 {
			t.Errorf("Expected lat/lon to be populated in result")
		}
	}

	// Test 5: Small radius finds nothing far away
	results, err = storage.QueryByProximity(ctx, centerLat, centerLon, 100.0, &models.Location{})
	if err != nil {
		t.Fatalf("Failed to query by proximity (100m): %v", err)
	}

	if len(results) > 1 {
		t.Errorf("Expected at most 1 result within 100m, got %d", len(results))
	}
}

func testQueryGearByCommunitySearch(t *testing.T, storage *ProtoSQLStorage) {
	ctx := context.Background()

	// Create test locations
	austinLocation := &models.Location{
		Geolocation: &models.Geolocation{
			LatitudeDeg:  30.2672,
			LongitudeDeg: -97.7431,
		},
		Address: &models.Address{
			Locality:   "Austin",
			RegionCode: "US",
		},
	}

	dallasLocation := &models.Location{
		Geolocation: &models.Geolocation{
			LatitudeDeg:  32.7767,
			LongitudeDeg: -96.7970,
		},
		Address: &models.Address{
			Locality:   "Dallas",
			RegionCode: "US",
		},
	}

	austinID, err := storage.Insert(ctx, austinLocation)
	if err != nil {
		t.Fatalf("Failed to insert Austin location: %v", err)
	}

	dallasID, err := storage.Insert(ctx, dallasLocation)
	if err != nil {
		t.Fatalf("Failed to insert Dallas location: %v", err)
	}

	// Create user
	user1 := &models.User{
		Email: "user1@example.com",
		Name:  "User 1",
		Role:  models.Role_ROLE_USER,
	}

	user1ID, err := storage.Insert(ctx, user1)
	if err != nil {
		t.Fatalf("Failed to insert user1: %v", err)
	}

	// Create gear with different names and locations
	drillGear := &models.Gear{
		Name:        "Power Drill",
		Description: "Professional grade power tool",
		OwnerId:     user1ID,
		LocationId:  austinID,
	}

	sawGear := &models.Gear{
		Name:        "Circular Saw",
		Description: "Professional woodworking saw",
		OwnerId:     user1ID,
		LocationId:  dallasID,
	}

	hammerGear := &models.Gear{
		Name:        "Hammer",
		Description: "Simple claw hammer",
		OwnerId:     user1ID,
		LocationId:  austinID,
	}

	screwdriverGear := &models.Gear{
		Name:        "Screwdriver Set",
		Description: "Complete set for home repairs",
		OwnerId:     user1ID,
		LocationId:  dallasID,
	}

	drillID, err := storage.Insert(ctx, drillGear)
	if err != nil {
		t.Fatalf("Failed to insert drill: %v", err)
	}

	sawID, err := storage.Insert(ctx, sawGear)
	if err != nil {
		t.Fatalf("Failed to insert saw: %v", err)
	}

	hammerID, err := storage.Insert(ctx, hammerGear)
	if err != nil {
		t.Fatalf("Failed to insert hammer: %v", err)
	}

	screwdriverID, err := storage.Insert(ctx, screwdriverGear)
	if err != nil {
		t.Fatalf("Failed to insert screwdriver: %v", err)
	}

	// Create community
	community1 := &models.Community{
		Name:        "Tool Sharing",
		Description: "Community tool library",
		CreatorId:   user1ID,
		OwnerUserId: user1ID,
	}

	comm1ID, err := storage.Insert(ctx, community1)
	if err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}

	// Create second community
	community2 := &models.Community{
		Name:        "Other Community",
		Description: "Different tool community",
		CreatorId:   user1ID,
		OwnerUserId: user1ID,
	}

	comm2ID, err := storage.Insert(ctx, community2)
	if err != nil {
		t.Fatalf("Failed to insert community2: %v", err)
	}

	// Share gear with community 1: drill, saw, hammer, screwdriver
	_, err = storage.Insert(ctx, &models.CommunityGear{CommunityId: comm1ID, GearId: drillID})
	if err != nil {
		t.Fatalf("Failed to share drill with comm1: %v", err)
	}
	_, err = storage.Insert(ctx, &models.CommunityGear{CommunityId: comm1ID, GearId: sawID})
	if err != nil {
		t.Fatalf("Failed to share saw with comm1: %v", err)
	}
	_, err = storage.Insert(ctx, &models.CommunityGear{CommunityId: comm1ID, GearId: hammerID})
	if err != nil {
		t.Fatalf("Failed to share hammer with comm1: %v", err)
	}
	_, err = storage.Insert(ctx, &models.CommunityGear{CommunityId: comm1ID, GearId: screwdriverID})
	if err != nil {
		t.Fatalf("Failed to share screwdriver with comm1: %v", err)
	}

	// Share only drill and hammer with community 2
	_, err = storage.Insert(ctx, &models.CommunityGear{CommunityId: comm2ID, GearId: drillID})
	if err != nil {
		t.Fatalf("Failed to share drill with comm2: %v", err)
	}
	_, err = storage.Insert(ctx, &models.CommunityGear{CommunityId: comm2ID, GearId: hammerID})
	if err != nil {
		t.Fatalf("Failed to share hammer with comm2: %v", err)
	}

	centerLat := 30.2672
	centerLon := -97.7431

	// Test 1: Search for "professional" - should match TWO items (drill and saw)
	results, err := storage.QueryGearByCommunitySearch(ctx, comm1ID, "professional", centerLat, centerLon)
	if err != nil {
		t.Fatalf("Failed to search for 'professional': %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results for 'professional', got %d", len(results))
	}

	// Verify results are sorted by distance (Austin drill should be first, Dallas saw second)
	if len(results) == 2 {
		if results[0].Gear.Id != drillID {
			t.Errorf("Expected drill to be first (closest), got %s", results[0].Gear.Id)
		}
		if results[1].Gear.Id != sawID {
			t.Errorf("Expected saw to be second, got %s", results[1].Gear.Id)
		}
		// Verify Austin is closer than Dallas
		if results[0].DistanceMeters >= results[1].DistanceMeters {
			t.Errorf("Expected Austin item closer than Dallas item")
		}

		// Verify location data is populated for drill (first result)
		if results[0].Location == nil {
			t.Errorf("Expected drill to have location data")
		} else {
			if results[0].Location.Geolocation == nil {
				t.Errorf("Expected drill location to have geolocation")
			} else {
				if results[0].Location.Geolocation.LatitudeDeg < 30.0 || results[0].Location.Geolocation.LatitudeDeg > 31.0 {
					t.Errorf("Expected Austin latitude ~30.27, got %f", results[0].Location.Geolocation.LatitudeDeg)
				}
				if results[0].Location.Geolocation.LongitudeDeg > -97.0 || results[0].Location.Geolocation.LongitudeDeg < -98.0 {
					t.Errorf("Expected Austin longitude ~-97.74, got %f", results[0].Location.Geolocation.LongitudeDeg)
				}
			}
		}
	}

	// Test 2: Search for "drill" - should find only 1 item
	results, err = storage.QueryGearByCommunitySearch(ctx, comm1ID, "drill", centerLat, centerLon)
	if err != nil {
		t.Fatalf("Failed to search for 'drill': %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result for 'drill', got %d", len(results))
	}

	if len(results) > 0 && results[0].Gear.Id != drillID {
		t.Errorf("Expected to find drill, got %s", results[0].Gear.Id)
	}

	// Test 3: Empty search returns all 4 items sorted by distance
	results, err = storage.QueryGearByCommunitySearch(ctx, comm1ID, "", centerLat, centerLon)
	if err != nil {
		t.Fatalf("Failed empty search: %v", err)
	}

	if len(results) != 4 {
		t.Errorf("Expected 4 results for empty query, got %d", len(results))
	}

	// Verify sorted by distance - Austin items should be first
	if len(results) == 4 {
		austinCount := 0
		for i := 0; i < 2; i++ {
			if results[i].Gear.LocationId == austinID {
				austinCount++
			}
		}
		if austinCount != 2 {
			t.Errorf("Expected first 2 results from Austin, got %d", austinCount)
		}
	}

	// Test 4: Case-insensitive search
	results, err = storage.QueryGearByCommunitySearch(ctx, comm1ID, "HAMMER", centerLat, centerLon)
	if err != nil {
		t.Fatalf("Failed case-insensitive search: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result for 'HAMMER', got %d", len(results))
	}

	// Test 5: Search for "set" - matches description and name
	results, err = storage.QueryGearByCommunitySearch(ctx, comm1ID, "set", centerLat, centerLon)
	if err != nil {
		t.Fatalf("Failed to search for 'set': %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result for 'set', got %d", len(results))
	}

	if len(results) > 0 && results[0].Gear.Id != screwdriverID {
		t.Errorf("Expected screwdriver for 'set', got %s", results[0].Gear.Id)
	}

	// Test 6: Search with no matches
	results, err = storage.QueryGearByCommunitySearch(ctx, comm1ID, "nonexistent", centerLat, centerLon)
	if err != nil {
		t.Fatalf("Failed search with no matches: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results for 'nonexistent', got %d", len(results))
	}

	// Test 7: Community filtering - search in community 2 should only return drill (not saw)
	results, err = storage.QueryGearByCommunitySearch(ctx, comm2ID, "professional", centerLat, centerLon)
	if err != nil {
		t.Fatalf("Failed to search comm2 for 'professional': %v", err)
	}

	// Community 2 only has drill with "professional" (saw not shared with comm2)
	if len(results) != 1 {
		t.Errorf("Expected 1 result in comm2 for 'professional', got %d", len(results))
	}

	if len(results) > 0 && results[0].Gear.Id != drillID {
		t.Errorf("Expected drill in comm2, got %s", results[0].Gear.Id)
	}

	// Test 8: Empty search in community 2 should only return 2 items (drill and hammer)
	results, err = storage.QueryGearByCommunitySearch(ctx, comm2ID, "", centerLat, centerLon)
	if err != nil {
		t.Fatalf("Failed empty search in comm2: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results in comm2 (drill and hammer), got %d", len(results))
	}

	// Verify both are from Austin (both have same location)
	if len(results) == 2 {
		foundDrill := false
		foundHammer := false
		for _, result := range results {
			if result.Gear.Id == drillID {
				foundDrill = true
			}
			if result.Gear.Id == hammerID {
				foundHammer = true
			}
		}
		if !foundDrill || !foundHammer {
			t.Errorf("Expected to find drill and hammer in comm2, found drill=%v hammer=%v", foundDrill, foundHammer)
		}
	}

	// Test 9: Verify saw and screwdriver are NOT in community 2
	results, err = storage.QueryGearByCommunitySearch(ctx, comm2ID, "saw", centerLat, centerLon)
	if err != nil {
		t.Fatalf("Failed to search comm2 for 'saw': %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results for 'saw' in comm2 (not shared), got %d", len(results))
	}

	results, err = storage.QueryGearByCommunitySearch(ctx, comm2ID, "screwdriver", centerLat, centerLon)
	if err != nil {
		t.Fatalf("Failed to search comm2 for 'screwdriver': %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results for 'screwdriver' in comm2 (not shared), got %d", len(results))
	}
}
