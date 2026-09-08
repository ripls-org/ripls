package gear

import (
	"context"
	"fmt"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

func TestService_ListUserGear(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	t.Run("empty list with authentication", func(t *testing.T) {
		ctx := createAuthenticatedContext("user-empty", "empty@example.com", models.Role_ROLE_USER)

		// Create test user
		testUser := &models.User{
			Id:    "user-empty",
			Email: "empty@example.com",
			Name:  "Empty User",
		}
		_, err := testStorage.Insert(ctx, testUser)
		if err != nil {
			t.Fatalf("Failed to insert test user: %v", err)
		}

		req := connect.NewRequest(&api.ListUserGearRequest{})

		resp, err := service.ListUserGear(ctx, req)
		if err != nil {
			t.Fatalf("ListUserGear failed: %v", err)
		}

		if resp.Msg.Items == nil {
			t.Error("Expected non-nil Items")
		}

		if len(resp.Msg.Items) != 0 {
			t.Errorf("Expected 0 items, got %d", len(resp.Msg.Items))
		}
	})

	t.Run("no authentication", func(t *testing.T) {
		ctx := context.Background()

		req := connect.NewRequest(&api.ListUserGearRequest{})

		_, err := service.ListUserGear(ctx, req)
		if err == nil {
			t.Fatal("Expected error for unauthenticated request")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeUnauthenticated {
			t.Errorf("Expected Unauthenticated error, got %v", connectErr.Code())
		}
	})

	t.Run("list multiple gear items", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Create test user
		testUser := &models.User{
			Id:    "user123",
			Email: "test@example.com",
			Name:  "Test User 123",
		}
		_, err := testStorage.Insert(ctx, testUser)
		if err != nil {
			t.Fatalf("Failed to insert test user: %v", err)
		}

		// Add multiple gear items
		testGearItems := []struct {
			Name        string
			Description string
			LocationID  string
		}{
			{Name: "Drill", Description: "Power drill", LocationID: "location-garage"},
			{Name: "Saw", Description: "Circular saw", LocationID: "location-shed"},
			{Name: "Hammer", Description: "Claw hammer", LocationID: "location-workshop"},
		}

		var addedIDs []string
		gearLocationMap := make(map[string]string)
		for _, gear := range testGearItems {
			addReq := connect.NewRequest(&api.SaveGearRequest{
				Name:        proto.String(gear.Name),
				Description: proto.String(gear.Description),
				LocationId:  proto.String(gear.LocationID),
			})
			addResp, err := service.SaveGear(ctx, addReq)
			if err != nil {
				t.Fatalf("Failed to add gear: %v", err)
			}
			addedIDs = append(addedIDs, addResp.Msg.Id)
			gearLocationMap[addResp.Msg.Id] = gear.LocationID
		}

		// List user's gear
		req := connect.NewRequest(&api.ListUserGearRequest{})
		resp, err := service.ListUserGear(ctx, req)
		if err != nil {
			t.Fatalf("ListUserGear failed: %v", err)
		}

		if len(resp.Msg.Items) != len(testGearItems) {
			t.Fatalf("Expected %d items, got %d", len(testGearItems), len(resp.Msg.Items))
		}

		// Verify all added gear items are in the response
		foundIDs := make(map[string]bool)
		for _, item := range resp.Msg.Items {
			foundIDs[item.Id] = true

			// Verify all fields are populated
			if item.Id == "" {
				t.Error("Expected non-empty ID")
			}
			if item.Name == "" {
				t.Error("Expected non-empty Name")
			}
			if item.Description == "" {
				t.Error("Expected non-empty Description")
			}
			if item.Owner == nil {
				t.Fatal("Expected Owner to be populated")
			}
			if item.Owner.Id != "user123" {
				t.Errorf("Expected Owner.Id user123, got %s", item.Owner.Id)
			}

			// Verify location_id matches what we set
			if expectedLocation, ok := gearLocationMap[item.Id]; ok {
				if item.LocationId != expectedLocation {
					t.Errorf("Expected LocationId %s for gear %s, got %s", expectedLocation, item.Id, item.LocationId)
				}
			}
		}

		for _, expectedID := range addedIDs {
			if !foundIDs[expectedID] {
				t.Errorf("Expected gear ID %s not found in list", expectedID)
			}
		}
	})

	t.Run("list gear items includes media_ids", func(t *testing.T) {
		ctx := createAuthenticatedContext("user-media", "media@example.com", models.Role_ROLE_USER)

		// Create test user
		testUser := &models.User{
			Id:    "user-media",
			Email: "media@example.com",
			Name:  "Media User",
		}
		_, err := testStorage.Insert(ctx, testUser)
		if err != nil {
			t.Fatalf("Failed to insert test user: %v", err)
		}

		// Create gear items with media IDs
		testGearItems := []struct {
			Name     string
			MediaIDs []string
		}{
			{Name: "Camera", MediaIDs: []string{"media-1", "media-2"}},
			{Name: "Tripod", MediaIDs: []string{"media-3"}},
			{Name: "Lens", MediaIDs: nil}, // No media
		}

		gearMediaMap := make(map[string][]string)
		for _, gear := range testGearItems {
			addReq := connect.NewRequest(&api.SaveGearRequest{
				Name:     proto.String(gear.Name),
				MediaIds: gear.MediaIDs,
			})
			addResp, err := service.SaveGear(ctx, addReq)
			if err != nil {
				t.Fatalf("Failed to add gear: %v", err)
			}
			gearMediaMap[addResp.Msg.Id] = gear.MediaIDs
		}

		// List user's gear
		req := connect.NewRequest(&api.ListUserGearRequest{})
		resp, err := service.ListUserGear(ctx, req)
		if err != nil {
			t.Fatalf("ListUserGear failed: %v", err)
		}

		// Verify media_ids are populated correctly for the gear we created
		foundCount := 0
		for _, item := range resp.Msg.Items {
			expectedMediaIDs, ok := gearMediaMap[item.Id]
			if !ok {
				// This is gear from another test, skip it
				continue
			}
			foundCount++

			// Check that media_ids match what we set
			if len(item.MediaIds) != len(expectedMediaIDs) {
				t.Errorf("Expected %d media IDs for gear %s, got %d", len(expectedMediaIDs), item.Id, len(item.MediaIds))
				continue
			}

			// Verify each media ID
			for i, mediaID := range item.MediaIds {
				if mediaID != expectedMediaIDs[i] {
					t.Errorf("Expected media ID %s at position %d for gear %s, got %s", expectedMediaIDs[i], i, item.Id, mediaID)
				}
			}
		}

		// Verify we found all the gear we created
		if foundCount != len(testGearItems) {
			t.Errorf("Expected to find %d gear items we created, but found %d", len(testGearItems), foundCount)
		}
	})

	t.Run("list gear includes shared_community_ids", func(t *testing.T) {
		userID := "user-shared"
		ctx := createAuthenticatedContext(userID, "shared@example.com", models.Role_ROLE_USER)

		// Create test user
		testUser := &models.User{
			Id:    userID,
			Email: "shared@example.com",
			Name:  "Shared User",
		}
		_, err := testStorage.Insert(ctx, testUser)
		if err != nil {
			t.Fatalf("Failed to insert test user: %v", err)
		}

		// Create two communities
		community1 := &models.Community{
			Name:        "Test Community 1",
			CreatorId:   userID,
			OwnerUserId: userID,
		}
		community1ID, err := testStorage.Insert(ctx, community1)
		if err != nil {
			t.Fatalf("Failed to create community 1: %v", err)
		}

		community2 := &models.Community{
			Name:        "Test Community 2",
			CreatorId:   userID,
			OwnerUserId: userID,
		}
		community2ID, err := testStorage.Insert(ctx, community2)
		if err != nil {
			t.Fatalf("Failed to create community 2: %v", err)
		}

		// Create gear item
		gearReq := connect.NewRequest(&api.SaveGearRequest{
			Name: proto.String("Shared Gear"),
		})
		gearResp, err := service.SaveGear(ctx, gearReq)
		if err != nil {
			t.Fatalf("Failed to create gear: %v", err)
		}
		sharedGearID := gearResp.Msg.Id

		// Create another gear item (not shared)
		unsharedReq := connect.NewRequest(&api.SaveGearRequest{
			Name: proto.String("Unshared Gear"),
		})
		unsharedResp, err := service.SaveGear(ctx, unsharedReq)
		if err != nil {
			t.Fatalf("Failed to create unshared gear: %v", err)
		}
		unsharedGearID := unsharedResp.Msg.Id

		// Share the first gear with both communities
		communityGear1 := &models.CommunityGear{
			CommunityId:  community1ID,
			GearId:       sharedGearID,
			Availability: models.Availability_AVAILABILITY_FOR_LOAN,
		}
		_, err = testStorage.Insert(ctx, communityGear1)
		if err != nil {
			t.Fatalf("Failed to share gear with community 1: %v", err)
		}

		communityGear2 := &models.CommunityGear{
			CommunityId:  community2ID,
			GearId:       sharedGearID,
			Availability: models.Availability_AVAILABILITY_FOR_GIVEAWAY,
		}
		_, err = testStorage.Insert(ctx, communityGear2)
		if err != nil {
			t.Fatalf("Failed to share gear with community 2: %v", err)
		}

		// List user's gear
		req := connect.NewRequest(&api.ListUserGearRequest{})
		resp, err := service.ListUserGear(ctx, req)
		if err != nil {
			t.Fatalf("ListUserGear failed: %v", err)
		}

		// Find the shared and unshared gear items
		var sharedGearItem, unsharedGearItem *api.GearItem
		for _, item := range resp.Msg.Items {
			if item.Id == sharedGearID {
				sharedGearItem = item
			}
			if item.Id == unsharedGearID {
				unsharedGearItem = item
			}
		}

		if sharedGearItem == nil {
			t.Fatal("Expected to find shared gear item")
		}
		if unsharedGearItem == nil {
			t.Fatal("Expected to find unshared gear item")
		}

		// Verify shared gear has both community IDs
		if len(sharedGearItem.SharedCommunityIds) != 2 {
			t.Fatalf("Expected 2 shared community IDs, got %d", len(sharedGearItem.SharedCommunityIds))
		}

		// Verify both community IDs are present (order doesn't matter)
		foundCommunity1 := false
		foundCommunity2 := false
		for _, id := range sharedGearItem.SharedCommunityIds {
			if id == community1ID {
				foundCommunity1 = true
			}
			if id == community2ID {
				foundCommunity2 = true
			}
		}
		if !foundCommunity1 {
			t.Error("Expected shared gear to include community 1 ID")
		}
		if !foundCommunity2 {
			t.Error("Expected shared gear to include community 2 ID")
		}

		// Verify unshared gear has empty shared_community_ids
		if len(unsharedGearItem.SharedCommunityIds) != 0 {
			t.Errorf("Expected 0 shared community IDs for unshared gear, got %d", len(unsharedGearItem.SharedCommunityIds))
		}
	})

	t.Run("shared_community_ids uses one query for all gear", func(t *testing.T) {
		userID := "user-n1-regression"
		ctx := createAuthenticatedContext(userID, "n1regression@example.com", models.Role_ROLE_USER)

		_, err := testStorage.Insert(ctx, &models.User{
			Id:    userID,
			Email: "n1regression@example.com",
			Name:  "N+1 Regression User",
		})
		if err != nil {
			t.Fatalf("Failed to insert test user: %v", err)
		}

		community := &models.Community{Name: "N+1 Regression Community", CreatorId: userID, OwnerUserId: userID}
		communityID, err := testStorage.Insert(ctx, community)
		if err != nil {
			t.Fatalf("Failed to create community: %v", err)
		}

		// Create 5 gear items and share each with the community.
		for i := range 5 {
			resp, err := service.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
				Name: proto.String(fmt.Sprintf("N+1 Gear %d", i)),
			}))
			if err != nil {
				t.Fatalf("Failed to create gear %d: %v", i, err)
			}
			if _, err := testStorage.Insert(ctx, &models.CommunityGear{
				CommunityId: communityID,
				GearId:      resp.Msg.Id,
			}); err != nil {
				t.Fatalf("Failed to share gear %d: %v", i, err)
			}
		}

		// ListUserGear must issue a fixed number of queries regardless of gear count.
		// Expected: 1 (list gear) + 1 (fetch user) + 1 (community gear batch) = 3.
		statsCtx := storage.WithQueryStats(ctx)
		var listErr error
		storage.AssertMaxQueries(t, statsCtx, 3, func() {
			_, listErr = service.ListUserGear(statsCtx, connect.NewRequest(&api.ListUserGearRequest{}))
		})
		if listErr != nil {
			t.Fatalf("ListUserGear failed: %v", listErr)
		}
	})

	t.Run("list gear filters by owner", func(t *testing.T) {
		// Create user-filter-1
		ctx123 := createAuthenticatedContext("user-filter-1", "filter1@example.com", models.Role_ROLE_USER)
		testUser123 := &models.User{
			Id:    "user-filter-1",
			Email: "filter1@example.com",
			Name:  "Filter User 1",
		}
		_, err := testStorage.Insert(ctx123, testUser123)
		if err != nil {
			t.Fatalf("Failed to insert user-filter-1: %v", err)
		}

		// Add gear from user-filter-1
		user123Req := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("User Filter 1's Drill"),
			Description: proto.String("Power drill"),
		})
		user123Resp, err := service.SaveGear(ctx123, user123Req)
		if err != nil {
			t.Fatalf("Failed to add gear for user-filter-1: %v", err)
		}
		user123GearID := user123Resp.Msg.Id

		// Create user-filter-2
		ctx456 := createAuthenticatedContext("user-filter-2", "filter2@example.com", models.Role_ROLE_USER)
		testUser456 := &models.User{
			Id:    "user-filter-2",
			Email: "filter2@example.com",
			Name:  "Filter User 2",
		}
		_, err = testStorage.Insert(ctx456, testUser456)
		if err != nil {
			t.Fatalf("Failed to insert user-filter-2: %v", err)
		}

		// Add gear from user-filter-2
		user456Req := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("User Filter 2's Wrench"),
			Description: proto.String("Adjustable wrench"),
		})
		user456Resp, err := service.SaveGear(ctx456, user456Req)
		if err != nil {
			t.Fatalf("Failed to add gear for user-filter-2: %v", err)
		}
		user456GearID := user456Resp.Msg.Id

		// List gear as user-filter-1 (should only see user-filter-1's gear)
		req123 := connect.NewRequest(&api.ListUserGearRequest{})
		resp123, err := service.ListUserGear(ctx123, req123)
		if err != nil {
			t.Fatalf("ListUserGear failed for user-filter-1: %v", err)
		}

		// Should only see user-filter-1's gear
		foundUser123Gear := false
		foundUser456Gear := false
		for _, item := range resp123.Msg.Items {
			if item.Id == user123GearID {
				foundUser123Gear = true
			}
			if item.Id == user456GearID {
				foundUser456Gear = true
			}
			// All gear should belong to user-filter-1
			if item.Owner == nil {
				t.Fatal("Expected Owner to be populated")
			}
			if item.Owner.Id != "user-filter-1" {
				t.Errorf("User-filter-1 should only see their own gear, but found gear with Owner.Id %s", item.Owner.Id)
			}
		}

		if !foundUser123Gear {
			t.Error("Expected to find gear owned by user-filter-1")
		}
		if foundUser456Gear {
			t.Error("User-filter-1 should not see user-filter-2's gear")
		}

		// List gear as user-filter-2 (should only see user-filter-2's gear)
		req456 := connect.NewRequest(&api.ListUserGearRequest{})
		resp456, err := service.ListUserGear(ctx456, req456)
		if err != nil {
			t.Fatalf("ListUserGear failed for user-filter-2: %v", err)
		}

		// Should only see user-filter-2's gear
		foundUser123GearIn456 := false
		foundUser456GearIn456 := false
		for _, item := range resp456.Msg.Items {
			if item.Id == user123GearID {
				foundUser123GearIn456 = true
			}
			if item.Id == user456GearID {
				foundUser456GearIn456 = true
			}
			// All gear should belong to user-filter-2
			if item.Owner == nil {
				t.Fatal("Expected Owner to be populated")
			}
			if item.Owner.Id != "user-filter-2" {
				t.Errorf("User-filter-2 should only see their own gear, but found gear with Owner.Id %s", item.Owner.Id)
			}
		}

		if foundUser123GearIn456 {
			t.Error("User-filter-2 should not see user-filter-1's gear")
		}
		if !foundUser456GearIn456 {
			t.Error("Expected to find gear owned by user-filter-2")
		}
	})

	t.Run("given-away gear is excluded from list", func(t *testing.T) {
		userID := "user-given-away"
		ctx := createAuthenticatedContext(userID, "givenaway@example.com", models.Role_ROLE_USER)

		_, err := testStorage.Insert(ctx, &models.User{
			Id:    userID,
			Email: "givenaway@example.com",
			Name:  "Given Away User",
		})
		if err != nil {
			t.Fatalf("Failed to insert test user: %v", err)
		}

		// Create an available gear item via service.
		availableResp, err := service.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
			Name: proto.String("Available Tent"),
		}))
		if err != nil {
			t.Fatalf("Failed to create available gear: %v", err)
		}
		availableID := availableResp.Msg.Id

		// Insert a given-away gear item directly with terminal state.
		givenAwayGear := &models.Gear{
			Name:    "Given Away Puzzle",
			OwnerId: userID,
			State:   models.GearState_GEAR_STATE_GIVEN_AWAY,
		}
		givenAwayID, err := testStorage.Insert(ctx, givenAwayGear)
		if err != nil {
			t.Fatalf("Failed to insert given-away gear: %v", err)
		}

		req := connect.NewRequest(&api.ListUserGearRequest{})
		resp, err := service.ListUserGear(ctx, req)
		if err != nil {
			t.Fatalf("ListUserGear failed: %v", err)
		}

		// The given-away gear must not appear in the list.
		for _, item := range resp.Msg.Items {
			if item.Id == givenAwayID {
				t.Errorf("given-away gear %s must not appear in ListUserGear response", givenAwayID)
			}
		}

		// The available gear must appear with the correct State field.
		var foundAvailable *api.GearItem
		for _, item := range resp.Msg.Items {
			if item.Id == availableID {
				foundAvailable = item
				break
			}
		}
		if foundAvailable == nil {
			t.Fatal("Expected available gear to appear in ListUserGear response")
		}
		if foundAvailable.State == nil {
			t.Error("Expected State field to be set on returned GearItem")
		}
		if foundAvailable.GetState() != api.GearItemState_GEAR_ITEM_STATE_AVAILABLE {
			t.Errorf("Expected State GEAR_ITEM_STATE_AVAILABLE, got %v", foundAvailable.GetState())
		}
	})
}

func TestService_ListUserGear_ValueEstimate(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	ctx := createAuthenticatedContext("user-list-value", "listvalue@example.com", models.Role_ROLE_USER)

	// Create test user
	testUser := &models.User{
		Id:    "user-list-value",
		Email: "listvalue@example.com",
		Name:  "List Value User",
	}
	_, err := testStorage.Insert(ctx, testUser)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	t.Run("returns value_estimate in gear items", func(t *testing.T) {
		// Create gear with value estimate via SaveGear
		req1 := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Valuable Item"),
			Description: proto.String("An item with value estimate"),
			Metadata: &api.GearMetadata{
				ValueEstimate: &api.ValueEstimate{
					EstimatedValueUsd: 500.0,
					Provenance: &api.Provenance{
						Source:    api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
						Name:      "genai_value_estimate",
						Reasoning: proto.String("High-end tool"),
						Sources:   []string{"toolstore.com"},
					},
				},
			},
		})

		_, err := service.SaveGear(ctx, req1)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		// Create gear without value estimate
		req2 := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Simple Item"),
			Description: proto.String("An item without value estimate"),
		})

		_, err = service.SaveGear(ctx, req2)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		// List user gear
		listReq := connect.NewRequest(&api.ListUserGearRequest{})
		listResp, err := service.ListUserGear(ctx, listReq)
		if err != nil {
			t.Fatalf("ListUserGear failed: %v", err)
		}

		if len(listResp.Msg.Items) != 2 {
			t.Fatalf("Expected 2 items, got %d", len(listResp.Msg.Items))
		}

		// Find the item with value estimate
		var valuableItem, simpleItem *api.GearItem
		for _, item := range listResp.Msg.Items {
			if item.Name == "Valuable Item" {
				valuableItem = item
			} else if item.Name == "Simple Item" {
				simpleItem = item
			}
		}

		if valuableItem == nil {
			t.Fatal("Expected to find Valuable Item")
		}
		if simpleItem == nil {
			t.Fatal("Expected to find Simple Item")
		}

		// Check valuable item has value estimate
		if valuableItem.ValueEstimate == nil {
			t.Fatal("Expected ValueEstimate for Valuable Item")
		}

		if valuableItem.ValueEstimate.EstimatedValueUsd != 500.0 {
			t.Errorf("Expected EstimatedValueUsd 500.0, got %f", valuableItem.ValueEstimate.EstimatedValueUsd)
		}

		if valuableItem.ValueEstimate.Provenance == nil {
			t.Fatal("Expected Provenance on ValueEstimate")
		}

		// Check simple item has no value estimate
		if simpleItem.ValueEstimate != nil {
			t.Errorf("Expected nil ValueEstimate for Simple Item, got %+v", simpleItem.ValueEstimate)
		}
	})
}
