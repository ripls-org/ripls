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
)

// TestService_SaveGear_ProvisionsItemCommunity verifies that a newly created
// gear is born with its per-item community (#2492): SaveGear calls the injected
// provisioner with the host + gear id + the creation-time availability (#2687)
// and returns the resulting item_community_id, while an update does not
// provision.
func TestService_SaveGear_ProvisionsItemCommunity(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := New(testStorage, &services.MockBucketStorage{})

	var gotHost, gotGear string
	var gotAvailability models.Availability
	service.SetItemCommunityProvisioner(func(_ context.Context, hostUserID, gearID string, availability models.Availability) (string, error) {
		gotHost, gotGear, gotAvailability = hostUserID, gearID, availability
		return "community-from-provisioner", nil
	})

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	resp, err := service.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
		Name: proto.String("Cordless Drill"),
	}))
	if err != nil {
		t.Fatalf("SaveGear failed: %v", err)
	}
	if resp.Msg.GetItemCommunityId() != "community-from-provisioner" {
		t.Errorf("expected ItemCommunityId from provisioner, got %q", resp.Msg.GetItemCommunityId())
	}
	if gotHost != "user123" {
		t.Errorf("provisioner got host %q, want user123", gotHost)
	}
	if gotGear != resp.Msg.Id {
		t.Errorf("provisioner got gear %q, want the new gear id %q", gotGear, resp.Msg.Id)
	}
	if gotAvailability != models.Availability_AVAILABILITY_FOR_LOAN {
		t.Errorf("provisioner got availability %v, want FOR_LOAN default", gotAvailability)
	}

	// The create flow's Give choice reaches the provisioner (#2687).
	giveaway := api.Availability_AVAILABILITY_FOR_GIVEAWAY
	_, err = service.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
		Name:         proto.String("Pantry Spaghetti"),
		Availability: &giveaway,
	}))
	if err != nil {
		t.Fatalf("SaveGear (giveaway) failed: %v", err)
	}
	if gotAvailability != models.Availability_AVAILABILITY_FOR_GIVEAWAY {
		t.Errorf("provisioner got availability %v, want FOR_GIVEAWAY", gotAvailability)
	}

	// Update must NOT provision — item_community_id is insert-only.
	updateResp, err := service.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
		Id:   resp.Msg.Id,
		Name: proto.String("Renamed"),
	}))
	if err != nil {
		t.Fatalf("update SaveGear failed: %v", err)
	}
	if updateResp.Msg.GetItemCommunityId() != "" {
		t.Errorf("expected no ItemCommunityId on update, got %q", updateResp.Msg.GetItemCommunityId())
	}
}

// TestService_SaveGear_ProvisionerErrorFailsCreate verifies the create fails
// loudly if per-item community provisioning errors (a gear without its audience
// community is a broken state, #2492).
func TestService_SaveGear_ProvisionerErrorFailsCreate(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := New(testStorage, &services.MockBucketStorage{})
	service.SetItemCommunityProvisioner(func(_ context.Context, _, _ string, _ models.Availability) (string, error) {
		return "", fmt.Errorf("provisioning boom")
	})

	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)
	_, err := service.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
		Name: proto.String("Doomed Gear"),
	}))
	if err == nil {
		t.Fatal("expected SaveGear to fail when the item-community provisioner errors")
	}
}

func TestService_SaveGear(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	t.Run("insert gear with authentication", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Test Drill"),
			Description: proto.String("A test drill"),
			MediaIds:    []string{"media-001", "media-002"},
			LocationId:  proto.String("location-garage"),
		})

		resp, err := service.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		if resp.Msg.Id == "" {
			t.Error("Expected non-empty ID")
		}

		// Verify gear was stored by retrieving it
		stored := &models.Gear{}
		err = testStorage.GetByID(ctx, resp.Msg.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve stored gear: %v", err)
		}

		if stored.OwnerId != "user123" {
			t.Errorf("Expected OwnerId to be user123, got %s", stored.OwnerId)
		}

		if stored.Name != "Test Drill" {
			t.Errorf("Expected name Test Drill, got %s", stored.Name)
		}

		if stored.Description != "A test drill" {
			t.Errorf("Expected description 'A test drill', got %s", stored.Description)
		}

		if len(stored.MediaIds) != 2 {
			t.Fatalf("Expected 2 media IDs, got %d", len(stored.MediaIds))
		}

		if stored.MediaIds[0] != "media-001" || stored.MediaIds[1] != "media-002" {
			t.Errorf("Expected media IDs [media-001, media-002], got %v", stored.MediaIds)
		}

		if stored.LocationId != "location-garage" {
			t.Errorf("Expected LocationId to be location-garage, got %s", stored.LocationId)
		}

		// Verify response header
		if resp.Header().Get("Gearserver-Version") != "v1" {
			t.Error("Expected GearServer-Version header to be v1")
		}
	})

	t.Run("update gear with authentication", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// First create gear
		createReq := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Original Name"),
			Description: proto.String("Original Description"),
			MediaIds:    []string{"media-001"},
			LocationId:  proto.String("location-original"),
		})

		createResp, err := service.SaveGear(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create gear: %v", err)
		}

		gearID := createResp.Msg.Id

		// Now update it
		updateReq := connect.NewRequest(&api.SaveGearRequest{
			Id:          gearID,
			Name:        proto.String("Updated Name"),
			Description: proto.String("Updated Description"),
			MediaIds:    []string{"media-002", "media-003"},
			LocationId:  proto.String("location-updated"),
		})

		updateResp, err := service.SaveGear(ctx, updateReq)
		if err != nil {
			t.Fatalf("Failed to update gear: %v", err)
		}

		if updateResp.Msg.Id != gearID {
			t.Errorf("Expected ID %s, got %s", gearID, updateResp.Msg.Id)
		}

		// Verify updates were applied
		stored := &models.Gear{}
		err = testStorage.GetByID(ctx, gearID, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve updated gear: %v", err)
		}

		if stored.Name != "Updated Name" {
			t.Errorf("Expected name 'Updated Name', got %s", stored.Name)
		}

		if stored.Description != "Updated Description" {
			t.Errorf("Expected description 'Updated Description', got %s", stored.Description)
		}

		// MediaIds should be replaced
		if len(stored.MediaIds) != 2 {
			t.Fatalf("Expected 2 media IDs, got %d", len(stored.MediaIds))
		}

		if stored.MediaIds[0] != "media-002" || stored.MediaIds[1] != "media-003" {
			t.Errorf("Expected media IDs [media-002, media-003], got %v", stored.MediaIds)
		}

		// LocationId should be updated
		if stored.LocationId != "location-updated" {
			t.Errorf("Expected LocationId to be location-updated, got %s", stored.LocationId)
		}
	})

	t.Run("update non-existent gear fails", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.SaveGearRequest{
			Id:   "nonexistent-id",
			Name: proto.String("Should Fail"),
		})

		_, err := service.SaveGear(ctx, req)
		if err == nil {
			t.Fatal("Expected error when updating non-existent gear")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("Expected NotFound error, got %v", connectErr.Code())
		}
	})

	t.Run("update gear by non-owner fails", func(t *testing.T) {
		ctx123 := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Create gear as user123
		createReq := connect.NewRequest(&api.SaveGearRequest{
			Name: proto.String("User123's Gear"),
		})

		createResp, err := service.SaveGear(ctx123, createReq)
		if err != nil {
			t.Fatalf("Failed to create gear: %v", err)
		}

		// Try to update as user456
		ctx456 := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)
		updateReq := connect.NewRequest(&api.SaveGearRequest{
			Id:   createResp.Msg.Id,
			Name: proto.String("Should Fail"),
		})

		_, err = service.SaveGear(ctx456, updateReq)
		if err == nil {
			t.Fatal("Expected error when non-owner tries to update gear")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", connectErr.Code())
		}
	})

	t.Run("no authentication", func(t *testing.T) {
		ctx := context.Background()

		req := connect.NewRequest(&api.SaveGearRequest{
			Name: proto.String("Test Drill"),
		})

		_, err := service.SaveGear(ctx, req)
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

	t.Run("owner ID is set correctly for different users", func(t *testing.T) {
		// Test with user456
		ctx := createAuthenticatedContext("user456", "user456@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.SaveGearRequest{
			Name: proto.String("User456's Hammer"),
		})

		resp, err := service.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		// Verify gear was stored with correct owner
		stored := &models.Gear{}
		err = testStorage.GetByID(ctx, resp.Msg.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve stored gear: %v", err)
		}

		if stored.OwnerId != "user456" {
			t.Errorf("Expected OwnerId to be user456, got %s", stored.OwnerId)
		}
	})

	t.Run("repeated field replacement", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Create gear with media IDs
		createReq := connect.NewRequest(&api.SaveGearRequest{
			Name:     proto.String("Camera"),
			MediaIds: []string{"media-001", "media-002", "media-003"},
		})

		createResp, err := service.SaveGear(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create gear: %v", err)
		}

		// Update with fewer media IDs (replacement, not append)
		updateReq := connect.NewRequest(&api.SaveGearRequest{
			Id:       createResp.Msg.Id,
			MediaIds: []string{"media-004"},
		})

		_, err = service.SaveGear(ctx, updateReq)
		if err != nil {
			t.Fatalf("Failed to update gear: %v", err)
		}

		// Verify media IDs were replaced
		stored := &models.Gear{}
		err = testStorage.GetByID(ctx, createResp.Msg.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve gear: %v", err)
		}

		if len(stored.MediaIds) != 1 {
			t.Fatalf("Expected 1 media ID, got %d", len(stored.MediaIds))
		}

		if stored.MediaIds[0] != "media-004" {
			t.Errorf("Expected media ID 'media-004', got %s", stored.MediaIds[0])
		}
	})

	t.Run("no auto-share with user communities", func(t *testing.T) {
		userID := "user789"
		ctx := createAuthenticatedContext(userID, "user789@example.com", models.Role_ROLE_USER)

		// Create two communities
		community1 := &models.Community{
			Name:        "Community 1",
			CreatorId:   userID,
			OwnerUserId: userID,
		}
		community1ID, err := testStorage.Insert(ctx, community1)
		if err != nil {
			t.Fatalf("Failed to create community 1: %v", err)
		}

		community2 := &models.Community{
			Name:        "Community 2",
			CreatorId:   "other-user",
			OwnerUserId: "other-user",
		}
		community2ID, err := testStorage.Insert(ctx, community2)
		if err != nil {
			t.Fatalf("Failed to create community 2: %v", err)
		}

		// Add user as member to both communities
		membership1 := &models.CommunityUser{
			CommunityId: community1ID,
			UserId:      userID,
			InviterId:   userID,
		}
		_, err = testStorage.Insert(ctx, membership1)
		if err != nil {
			t.Fatalf("Failed to create membership 1: %v", err)
		}

		membership2 := &models.CommunityUser{
			CommunityId: community2ID,
			UserId:      userID,
			InviterId:   "other-user",
		}
		_, err = testStorage.Insert(ctx, membership2)
		if err != nil {
			t.Fatalf("Failed to create membership 2: %v", err)
		}

		// Create gear - should NOT auto-share with any communities
		req := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Auto-Share Drill"),
			Description: proto.String("Should NOT be automatically shared"),
		})

		resp, err := service.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		gearID := resp.Msg.Id

		// Verify gear is NOT shared with any communities automatically
		shares, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.CommunityGear{})
		if err != nil {
			t.Fatalf("Failed to query gear shares: %v", err)
		}

		if len(shares) != 0 {
			t.Errorf("Expected gear to NOT be auto-shared with any communities, but got %d shares", len(shares))
		}
	})

	t.Run("gear creation succeeds for user with no communities", func(t *testing.T) {
		userID := "user-no-communities"
		ctx := createAuthenticatedContext(userID, "nocomm@example.com", models.Role_ROLE_USER)

		// Create gear - should succeed even with no communities
		req := connect.NewRequest(&api.SaveGearRequest{
			Name: proto.String("Lonely Drill"),
		})

		resp, err := service.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("SaveGear should succeed even with no communities: %v", err)
		}

		gearID := resp.Msg.Id

		// Verify no shares were created (as expected with no auto-share)
		shares, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.CommunityGear{})
		if err != nil {
			t.Fatalf("Failed to query gear shares: %v", err)
		}

		if len(shares) != 0 {
			t.Errorf("Expected 0 shares for user with no communities, got %d", len(shares))
		}
	})

	t.Run("update does not trigger auto-share", func(t *testing.T) {
		userID := "user-update-test"
		ctx := createAuthenticatedContext(userID, "update@example.com", models.Role_ROLE_USER)

		// Create gear (before joining any community)
		createReq := connect.NewRequest(&api.SaveGearRequest{
			Name: proto.String("Pre-Community Drill"),
		})

		createResp, err := service.SaveGear(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create gear: %v", err)
		}

		gearID := createResp.Msg.Id

		// Verify no shares
		shares, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.CommunityGear{})
		if err != nil {
			t.Fatalf("Failed to query shares: %v", err)
		}
		if len(shares) != 0 {
			t.Fatalf("Expected 0 initial shares, got %d", len(shares))
		}

		// Now join a community
		community := &models.Community{
			Name:        "New Community",
			CreatorId:   userID,
			OwnerUserId: userID,
		}
		communityID, err := testStorage.Insert(ctx, community)
		if err != nil {
			t.Fatalf("Failed to create community: %v", err)
		}

		membership := &models.CommunityUser{
			CommunityId: communityID,
			UserId:      userID,
			InviterId:   userID,
		}
		_, err = testStorage.Insert(ctx, membership)
		if err != nil {
			t.Fatalf("Failed to create membership: %v", err)
		}

		// Update the gear - should NOT auto-share with new community
		updateReq := connect.NewRequest(&api.SaveGearRequest{
			Id:   gearID,
			Name: proto.String("Updated Name"),
		})

		_, err = service.SaveGear(ctx, updateReq)
		if err != nil {
			t.Fatalf("Failed to update gear: %v", err)
		}

		// Verify still no shares (update doesn't trigger auto-share)
		shares, err = testStorage.QueryByField(ctx, "gear_id", gearID, &models.CommunityGear{})
		if err != nil {
			t.Fatalf("Failed to query shares after update: %v", err)
		}

		if len(shares) != 0 {
			t.Errorf("Expected 0 shares after update, got %d (update should not trigger auto-share)", len(shares))
		}
	})

	t.Run("insert gear with source_url", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		sourceURL := "https://www.rei.com/product/tent-4p"
		req := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("REI Tent"),
			Description: proto.String("4-person camping tent"),
			SourceUrl:   &sourceURL,
		})

		resp, err := service.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		// Verify source_url was stored
		stored := &models.Gear{}
		err = testStorage.GetByID(ctx, resp.Msg.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve stored gear: %v", err)
		}

		if stored.SourceUrl != "https://www.rei.com/product/tent-4p" {
			t.Errorf("Expected source_url 'https://www.rei.com/product/tent-4p', got %q", stored.SourceUrl)
		}

		// Verify round-trip through GetGear
		// Create user so GetGear can populate owner
		testUser := &models.User{
			Id:    "user123",
			Email: "test@example.com",
			Name:  "Test User",
		}
		_, _ = testStorage.Insert(ctx, testUser) // may already exist from other subtests

		getReq := connect.NewRequest(&api.GetGearRequest{
			Id: resp.Msg.Id,
		})

		getResp, err := service.GetGear(ctx, getReq)
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}

		if getResp.Msg.SourceUrl != "https://www.rei.com/product/tent-4p" {
			t.Errorf("Expected source_url round-trip, got %q", getResp.Msg.SourceUrl)
		}
	})
}

func TestService_SaveGear_ValueEstimate(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	t.Run("insert gear with value_estimate", func(t *testing.T) {
		ctx := createAuthenticatedContext("user-value-1", "value1@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Valuable Drill"),
			Description: proto.String("A valuable drill with estimate"),
			Metadata: &api.GearMetadata{
				ValueEstimate: &api.ValueEstimate{
					EstimatedValueUsd: 150.0, // $150.00
					Provenance: &api.Provenance{
						Source:    api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
						Name:      "genai_value_estimate",
						Reasoning: proto.String("Based on similar items in the market"),
						Sources:   []string{"amazon.com", "homedepot.com"},
					},
				},
			},
		})

		resp, err := service.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		if resp.Msg.Id == "" {
			t.Error("Expected non-empty ID")
		}

		// Verify value_estimate was stored
		stored := &models.Gear{}
		err = testStorage.GetByID(ctx, resp.Msg.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve stored gear: %v", err)
		}

		if stored.ValueEstimate == nil {
			t.Fatal("Expected ValueEstimate to be stored")
		}

		if stored.ValueEstimate.EstimatedValueUsd != 150.0 {
			t.Errorf("Expected EstimatedValueUsd 150.0, got %f", stored.ValueEstimate.EstimatedValueUsd)
		}

		if stored.ValueEstimate.Provenance == nil {
			t.Fatal("Expected Provenance to be stored on ValueEstimate")
		}

		if stored.ValueEstimate.Provenance.GetReasoning() != "Based on similar items in the market" {
			t.Errorf("Expected Reasoning 'Based on similar items in the market', got %s", stored.ValueEstimate.Provenance.GetReasoning())
		}

		if len(stored.ValueEstimate.Provenance.Sources) != 2 {
			t.Fatalf("Expected 2 sources, got %d", len(stored.ValueEstimate.Provenance.Sources))
		}

		if stored.ValueEstimate.Provenance.Sources[0] != "amazon.com" || stored.ValueEstimate.Provenance.Sources[1] != "homedepot.com" {
			t.Errorf("Expected sources [amazon.com, homedepot.com], got %v", stored.ValueEstimate.Provenance.Sources)
		}
	})

	t.Run("insert gear without value_estimate", func(t *testing.T) {
		ctx := createAuthenticatedContext("user-value-2", "value2@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Simple Hammer"),
			Description: proto.String("A hammer without value estimate"),
		})

		resp, err := service.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		// Verify value_estimate is nil
		stored := &models.Gear{}
		err = testStorage.GetByID(ctx, resp.Msg.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve stored gear: %v", err)
		}

		if stored.ValueEstimate != nil {
			t.Errorf("Expected ValueEstimate to be nil, got %+v", stored.ValueEstimate)
		}
	})

	t.Run("update gear with value_estimate", func(t *testing.T) {
		ctx := createAuthenticatedContext("user-value-3", "value3@example.com", models.Role_ROLE_USER)

		// First create gear without value estimate
		createReq := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Saw"),
			Description: proto.String("A saw"),
		})

		createResp, err := service.SaveGear(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create gear: %v", err)
		}

		gearID := createResp.Msg.Id

		// Now update with value estimate
		updateReq := connect.NewRequest(&api.SaveGearRequest{
			Id:          gearID,
			Name:        proto.String("Saw"),
			Description: proto.String("A saw with updated value"),
			Metadata: &api.GearMetadata{
				ValueEstimate: &api.ValueEstimate{
					EstimatedValueUsd: 75.0, // $75.00
					Provenance: &api.Provenance{
						Source:    api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
						Name:      "genai_value_estimate",
						Reasoning: proto.String("Updated market valuation"),
						Sources:   []string{"lowes.com"},
					},
				},
			},
		})

		_, err = service.SaveGear(ctx, updateReq)
		if err != nil {
			t.Fatalf("Failed to update gear: %v", err)
		}

		// Verify value_estimate was updated
		stored := &models.Gear{}
		err = testStorage.GetByID(ctx, gearID, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve updated gear: %v", err)
		}

		if stored.ValueEstimate == nil {
			t.Fatal("Expected ValueEstimate to be stored after update")
		}

		if stored.ValueEstimate.EstimatedValueUsd != 75.0 {
			t.Errorf("Expected EstimatedValueUsd 75.0, got %f", stored.ValueEstimate.EstimatedValueUsd)
		}

		if stored.ValueEstimate.Provenance == nil {
			t.Fatal("Expected Provenance to be stored on ValueEstimate after update")
		}
	})
}
