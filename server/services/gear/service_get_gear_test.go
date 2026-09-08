package gear

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/services"
)

// TestGetGear_InvitedIndividuals verifies that people individually invited to a
// gear's own ad-hoc origin community surface in InvitedIndividuals for the
// "Shared with" roster, excluding the owner. Gear has no RSVP, so every invited
// member other than the owner is listed (#2492).
func TestGetGear_InvitedIndividuals(t *testing.T) {
	testStorage := setupTestStorage(t)
	service := New(testStorage, &services.MockBucketStorage{})
	ctx := context.Background()

	ownerID, err := testStorage.Insert(ctx, &models.User{Name: "Owner", Email: "owner@example.com"})
	if err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	aliceID, err := testStorage.Insert(ctx, &models.User{Name: "Alice", Email: "alice@example.com"})
	if err != nil {
		t.Fatalf("insert alice: %v", err)
	}
	bobID, err := testStorage.Insert(ctx, &models.User{Name: "Bob", Email: "bob@example.com"})
	if err != nil {
		t.Fatalf("insert bob: %v", err)
	}

	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)
	gearResp, err := service.SaveGear(ownerCtx, connect.NewRequest(&api.SaveGearRequest{
		Name: proto.String("Cordless Drill"),
	}))
	if err != nil {
		t.Fatalf("SaveGear failed: %v", err)
	}
	gearID := gearResp.Msg.Id

	// Create the gear's ad-hoc origin community, share the gear into it, and add
	// the owner + two directly-invited members.
	originID, err := testStorage.Insert(ctx, &models.Community{
		CreatorId:   ownerID,
		OwnerUserId: ownerID,
		OriginItem:  &models.Community_OriginGearId{OriginGearId: gearID},
	})
	if err != nil {
		t.Fatalf("insert origin community: %v", err)
	}
	if _, err := testStorage.Insert(ctx, &models.CommunityGear{GearId: gearID, CommunityId: originID}); err != nil {
		t.Fatalf("insert community gear: %v", err)
	}
	for _, uid := range []string{ownerID, aliceID, bobID} {
		if _, err := testStorage.Insert(ctx, &models.CommunityUser{
			CommunityId: originID, UserId: uid, InviterId: ownerID,
		}); err != nil {
			t.Fatalf("add membership for %s: %v", uid, err)
		}
	}

	getResp, err := service.GetGear(ownerCtx, connect.NewRequest(&api.GetGearRequest{Id: gearID}))
	if err != nil {
		t.Fatalf("GetGear failed: %v", err)
	}
	got := make(map[string]bool, len(getResp.Msg.InvitedIndividuals))
	for _, u := range getResp.Msg.InvitedIndividuals {
		got[u.Id] = true
	}
	if got[ownerID] {
		t.Errorf("owner must not appear in InvitedIndividuals")
	}
	if !got[aliceID] || !got[bobID] || len(got) != 2 {
		t.Errorf("InvitedIndividuals = %v, want exactly alice (%s) + bob (%s)", got, aliceID, bobID)
	}
}

func TestService_GetGear(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	// Pre-populate storage with test data
	ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	// Create test user
	testUser := &models.User{
		Id:    "user123",
		Email: "test@example.com",
		Name:  "Test User",
	}
	_, err := testStorage.Insert(ctx, testUser)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	testGear := &models.Gear{
		Name:        "Test Drill",
		Description: "A test drill",
		OwnerId:     "user123",
		LocationId:  "location-workshop",
	}

	gearID, err := testStorage.Insert(ctx, testGear)
	if err != nil {
		t.Fatalf("Failed to insert test gear: %v", err)
	}

	t.Run("successful retrieval with authentication", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		})

		resp, err := service.GetGear(ctx, req)
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}

		if resp.Msg.Id != gearID {
			t.Errorf("Expected ID %s, got %s", gearID, resp.Msg.Id)
		}

		if resp.Msg.Name != testGear.Name {
			t.Errorf("Expected name %s, got %s", testGear.Name, resp.Msg.Name)
		}

		if resp.Msg.Description != testGear.Description {
			t.Errorf("Expected description %s, got %s", testGear.Description, resp.Msg.Description)
		}

		if resp.Msg.Owner == nil {
			t.Fatal("Expected Owner to be populated")
		}

		if resp.Msg.Owner.Id != testGear.OwnerId {
			t.Errorf("Expected Owner.Id %s, got %s", testGear.OwnerId, resp.Msg.Owner.Id)
		}

		if resp.Msg.LocationId != testGear.LocationId {
			t.Errorf("Expected LocationId %s, got %s", testGear.LocationId, resp.Msg.LocationId)
		}
	})

	t.Run("no authentication", func(t *testing.T) {
		ctx := context.Background()

		req := connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		})

		_, err := service.GetGear(ctx, req)
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

	t.Run("gear not found", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.GetGearRequest{
			Id: "nonexistent-id",
		})

		_, err := service.GetGear(ctx, req)
		if err == nil {
			t.Fatal("Expected error for non-existent gear")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodeNotFound {
			t.Errorf("Expected NotFound error, got %v", connectErr.Code())
		}
	})

	t.Run("non-member cannot read gear", func(t *testing.T) {
		ctx := createAuthenticatedContext("user456", "other@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		})

		_, err := service.GetGear(ctx, req)
		if err == nil {
			t.Fatal("expected PermissionDenied for non-member, got nil")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied, got %v", connectErr.Code())
		}
	})

	t.Run("admin without membership cannot read gear", func(t *testing.T) {
		ctx := createAuthenticatedContext("admin123", "admin@example.com", models.Role_ROLE_ADMIN)

		req := connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		})

		_, err := service.GetGear(ctx, req)
		if err == nil {
			t.Fatal("expected PermissionDenied for admin with no shared community, got nil")
		}
		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}
		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied, got %v", connectErr.Code())
		}
	})

	t.Run("source_url round-trips through storage", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Insert gear with source_url directly into storage (simulating URL-based creation)
		gearWithURL := &models.Gear{
			Name:        "REI Tent",
			Description: "4-person camping tent",
			OwnerId:     "user123",
			LocationId:  "location-workshop",
			SourceUrl:   "https://www.rei.com/product/tent-4p",
		}

		urlGearID, err := testStorage.Insert(ctx, gearWithURL)
		if err != nil {
			t.Fatalf("Failed to insert gear with source URL: %v", err)
		}

		req := connect.NewRequest(&api.GetGearRequest{
			Id: urlGearID,
		})

		resp, err := service.GetGear(ctx, req)
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}

		if resp.Msg.SourceUrl != "https://www.rei.com/product/tent-4p" {
			t.Errorf("Expected source_url 'https://www.rei.com/product/tent-4p', got %q", resp.Msg.SourceUrl)
		}
	})

	t.Run("source_url empty for gear without source", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		})

		resp, err := service.GetGear(ctx, req)
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}

		if resp.Msg.SourceUrl != "" {
			t.Errorf("Expected empty source_url for gear without source, got %q", resp.Msg.SourceUrl)
		}
	})

	t.Run("created_at_unix_sec falls back to Gear.created_at_unix_sec when not shared", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Gear with no CommunityGear rows — never shared.
		const expectedCreatedAt int64 = 1700000000
		orphanGear := &models.Gear{
			Name:             "Solo Tent",
			OwnerId:          "user123",
			LocationId:       "location-workshop",
			CreatedAtUnixSec: expectedCreatedAt,
		}
		orphanGearID, err := testStorage.Insert(ctx, orphanGear)
		if err != nil {
			t.Fatalf("Failed to insert gear: %v", err)
		}

		req := connect.NewRequest(&api.GetGearRequest{Id: orphanGearID})
		resp, err := service.GetGear(ctx, req)
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}

		if resp.Msg.CreatedAtUnixSec != expectedCreatedAt {
			t.Errorf("Expected CreatedAtUnixSec %d (gear-level fallback), got %d", expectedCreatedAt, resp.Msg.CreatedAtUnixSec)
		}
	})

	t.Run("created_at_unix_sec uses earliest CommunityGear when shared", func(t *testing.T) {
		ctx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

		// Legacy gear with no CreatedAtUnixSec on the gear record itself —
		// simulates a gear created before the field was added on 2026-03-29.
		// This is the issue #1669 scenario: without the CommunityGear-min
		// fix, the response would carry CreatedAtUnixSec = 0 and the client
		// would render "20575d ago".
		legacyGear := &models.Gear{
			Name:       "Legacy Bike",
			OwnerId:    "user123",
			LocationId: "location-workshop",
			// CreatedAtUnixSec intentionally unset (zero).
		}
		legacyGearID, err := testStorage.Insert(ctx, legacyGear)
		if err != nil {
			t.Fatalf("Failed to insert gear: %v", err)
		}

		// Insert a community first, then two CommunityGear rows with
		// distinct created_at timestamps. We expect the earliest to win.
		const earlierShare int64 = 1700000000
		const laterShare int64 = 1700100000

		comm1 := &models.Community{Name: "First Community", CreatorId: "user123", OwnerUserId: "user123"}
		comm1ID, err := testStorage.Insert(ctx, comm1)
		if err != nil {
			t.Fatalf("Failed to insert community: %v", err)
		}
		comm2 := &models.Community{Name: "Second Community", CreatorId: "user123", OwnerUserId: "user123"}
		comm2ID, err := testStorage.Insert(ctx, comm2)
		if err != nil {
			t.Fatalf("Failed to insert community: %v", err)
		}

		_, err = testStorage.Insert(ctx, &models.CommunityGear{
			GearId:           legacyGearID,
			CommunityId:      comm2ID,
			CreatedAtUnixSec: laterShare,
		})
		if err != nil {
			t.Fatalf("Failed to insert community_gear: %v", err)
		}
		_, err = testStorage.Insert(ctx, &models.CommunityGear{
			GearId:           legacyGearID,
			CommunityId:      comm1ID,
			CreatedAtUnixSec: earlierShare,
		})
		if err != nil {
			t.Fatalf("Failed to insert community_gear: %v", err)
		}

		req := connect.NewRequest(&api.GetGearRequest{Id: legacyGearID})
		resp, err := service.GetGear(ctx, req)
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}

		if resp.Msg.CreatedAtUnixSec != earlierShare {
			t.Errorf("Expected CreatedAtUnixSec %d (earliest CommunityGear), got %d", earlierShare, resp.Msg.CreatedAtUnixSec)
		}
	})
}

func TestService_GetGear_ValueEstimate(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	ctx := createAuthenticatedContext("user-get-value", "getvalue@example.com", models.Role_ROLE_USER)

	// Create test user
	testUser := &models.User{
		Id:    "user-get-value",
		Email: "getvalue@example.com",
		Name:  "Get Value User",
	}
	_, err := testStorage.Insert(ctx, testUser)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	t.Run("returns value_estimate when present", func(t *testing.T) {
		// Create gear with value estimate directly in storage
		testGear := &models.Gear{
			Name:        "Test Drill with Value",
			Description: "A drill with value estimate",
			OwnerId:     "user-get-value",
			ValueEstimate: &models.ValueEstimate{
				EstimatedValueUsd: 200.0,
				Provenance: &models.Provenance{
					Source:    models.ProvenanceSource_PROVENANCE_SOURCE_LLM,
					Name:      "genai_value_estimate",
					Reasoning: proto.String("AI-generated estimate from product page"),
					Sources:   []string{"manufacturer.com"},
				},
			},
		}

		gearID, err := testStorage.Insert(ctx, testGear)
		if err != nil {
			t.Fatalf("Failed to insert test gear: %v", err)
		}

		// Get the gear via API
		req := connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		})

		resp, err := service.GetGear(ctx, req)
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}

		if resp.Msg.ValueEstimate == nil {
			t.Fatal("Expected ValueEstimate in response")
		}

		if resp.Msg.ValueEstimate.EstimatedValueUsd != 200.0 {
			t.Errorf("Expected EstimatedValueUsd 200.0, got %f", resp.Msg.ValueEstimate.EstimatedValueUsd)
		}

		if resp.Msg.ValueEstimate.Provenance == nil {
			t.Fatal("Expected Provenance in ValueEstimate response")
		}

		if resp.Msg.ValueEstimate.Provenance.GetReasoning() != "AI-generated estimate from product page" {
			t.Errorf("Expected Reasoning 'AI-generated estimate from product page', got %s", resp.Msg.ValueEstimate.Provenance.GetReasoning())
		}

		if len(resp.Msg.ValueEstimate.Provenance.Sources) != 1 || resp.Msg.ValueEstimate.Provenance.Sources[0] != "manufacturer.com" {
			t.Errorf("Expected sources [manufacturer.com], got %v", resp.Msg.ValueEstimate.Provenance.Sources)
		}
	})

	t.Run("returns nil value_estimate when not present", func(t *testing.T) {
		// Create gear without value estimate
		testGear := &models.Gear{
			Name:        "Test Drill without Value",
			Description: "A drill without value estimate",
			OwnerId:     "user-get-value",
		}

		gearID, err := testStorage.Insert(ctx, testGear)
		if err != nil {
			t.Fatalf("Failed to insert test gear: %v", err)
		}

		// Get the gear via API
		req := connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		})

		resp, err := service.GetGear(ctx, req)
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}

		if resp.Msg.ValueEstimate != nil {
			t.Errorf("Expected nil ValueEstimate, got %+v", resp.Msg.ValueEstimate)
		}
	})
}

func TestService_GetGear_Metadata(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	ctx := createAuthenticatedContext("user-metadata", "metadata@example.com", models.Role_ROLE_USER)

	// Create test user
	testUser := &models.User{
		Id:    "user-metadata",
		Email: "metadata@example.com",
		Name:  "Metadata User",
	}
	_, err := testStorage.Insert(ctx, testUser)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	t.Run("returns metadata fields when present", func(t *testing.T) {
		// Create gear with all metadata fields
		testGear := &models.Gear{
			Name:             "DeWalt Cordless Drill",
			Description:      "A power drill with metadata",
			OwnerId:          "user-metadata",
			Category:         &models.TrackedString{Value: "Power Tools"},
			Brand:            &models.TrackedString{Value: "DeWalt"},
			Model:            &models.TrackedString{Value: "DCD771C2"},
			MaterialCategory: &models.TrackedMaterialCategory{Value: models.MaterialCategory_MATERIAL_CATEGORY_CORDLESS_POWER_TOOL},
			WeightGrams: &models.TrackedEstimate{
				Value: &models.Estimate{
					Mean:   2300.0,
					Stddev: 200.0,
				},
			},
			EmbodiedCarbon: &models.CarbonEstimate{
				Co2EGrams: &models.Estimate{
					Mean:   18400.0,
					Stddev: 2000.0,
				},
				Provenance: &models.Provenance{
					Source: models.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
					Name:   "weight_material_carbon",
				},
			},
		}

		gearID, err := testStorage.Insert(ctx, testGear)
		if err != nil {
			t.Fatalf("Failed to insert test gear: %v", err)
		}

		// Get the gear via API
		req := connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		})

		resp, err := service.GetGear(ctx, req)
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}

		// Verify metadata is present
		if resp.Msg.Metadata == nil {
			t.Fatal("Expected Metadata in response")
		}

		// Verify category
		if resp.Msg.Metadata.Category == nil || resp.Msg.Metadata.Category.Value != "Power Tools" {
			t.Errorf("Expected Category 'Power Tools', got %v", resp.Msg.Metadata.Category)
		}

		// Verify brand
		if resp.Msg.Metadata.Brand == nil || resp.Msg.Metadata.Brand.Value != "DeWalt" {
			t.Errorf("Expected Brand 'DeWalt', got %v", resp.Msg.Metadata.Brand)
		}

		// Verify model
		if resp.Msg.Metadata.Model == nil || resp.Msg.Metadata.Model.Value != "DCD771C2" {
			t.Errorf("Expected Model 'DCD771C2', got %v", resp.Msg.Metadata.Model)
		}

		// Verify material category
		if resp.Msg.Metadata.MaterialCategory == nil || resp.Msg.Metadata.MaterialCategory.Value != api.MaterialCategory_MATERIAL_CATEGORY_CORDLESS_POWER_TOOL {
			t.Errorf("Expected MaterialCategory CORDLESS_POWER_TOOL, got %v", resp.Msg.Metadata.MaterialCategory)
		}

		// Verify weight
		if resp.Msg.Metadata.WeightGrams == nil || resp.Msg.Metadata.WeightGrams.Value == nil {
			t.Fatal("Expected WeightGrams in response")
		}
		if resp.Msg.Metadata.WeightGrams.Value.Mean != 2300.0 {
			t.Errorf("Expected WeightGrams.Mean 2300.0, got %f", resp.Msg.Metadata.WeightGrams.Value.Mean)
		}
		if resp.Msg.Metadata.WeightGrams.Value.Stddev != 200.0 {
			t.Errorf("Expected WeightGrams.Stddev 200.0, got %f", resp.Msg.Metadata.WeightGrams.Value.Stddev)
		}

		// Verify embodied carbon
		if resp.Msg.Metadata.EmbodiedCarbon == nil {
			t.Fatal("Expected EmbodiedCarbon in response")
		}
		if resp.Msg.Metadata.EmbodiedCarbon.Co2EGrams == nil {
			t.Fatal("Expected EmbodiedCarbon.Co2EGrams in response")
		}
		if resp.Msg.Metadata.EmbodiedCarbon.Co2EGrams.Mean != 18400.0 {
			t.Errorf("Expected EmbodiedCarbon.Co2EGrams.Mean 18400.0, got %f", resp.Msg.Metadata.EmbodiedCarbon.Co2EGrams.Mean)
		}
		if resp.Msg.Metadata.EmbodiedCarbon.Co2EGrams.Stddev != 2000.0 {
			t.Errorf("Expected EmbodiedCarbon.Co2EGrams.Stddev 2000.0, got %f", resp.Msg.Metadata.EmbodiedCarbon.Co2EGrams.Stddev)
		}
		if resp.Msg.Metadata.EmbodiedCarbon.Provenance == nil || resp.Msg.Metadata.EmbodiedCarbon.Provenance.Name != "weight_material_carbon" {
			t.Errorf("Expected EmbodiedCarbon.Provenance.Name weight_material_carbon, got %s", resp.Msg.Metadata.EmbodiedCarbon.GetProvenance().GetName())
		}
	})

	t.Run("returns empty strings and nil for missing metadata", func(t *testing.T) {
		// Create gear without metadata
		testGear := &models.Gear{
			Name:        "Basic Item",
			Description: "An item without metadata",
			OwnerId:     "user-metadata",
		}

		gearID, err := testStorage.Insert(ctx, testGear)
		if err != nil {
			t.Fatalf("Failed to insert test gear: %v", err)
		}

		// Get the gear via API
		req := connect.NewRequest(&api.GetGearRequest{
			Id: gearID,
		})

		resp, err := service.GetGear(ctx, req)
		if err != nil {
			t.Fatalf("GetGear failed: %v", err)
		}

		// Verify metadata is nil when all fields are empty
		if resp.Msg.Metadata != nil {
			t.Errorf("Expected nil Metadata, got %+v", resp.Msg.Metadata)
		}
	})
}

// TestGetGear_AccessControl tests the community-scoped access gate on GetGear.
func TestGetGear_AccessControl(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)
	ctx := context.Background()

	ownerID := "acl-owner"
	memberID := "acl-member"
	strangerID := "acl-stranger"

	// Insert users
	for _, u := range []struct{ id, name string }{
		{ownerID, "ACL Owner"}, {memberID, "ACL Member"}, {strangerID, "ACL Stranger"},
	} {
		if _, err := testStorage.Insert(ctx, &models.User{Id: u.id, Name: u.name, Email: u.id + "@example.com"}); err != nil {
			t.Fatalf("insert user %s: %v", u.id, err)
		}
	}

	// Insert community and add owner + member
	comm := &models.Community{Id: "acl-community", Name: "ACL Community", CreatorId: ownerID, OwnerUserId: ownerID}
	if _, err := testStorage.Insert(ctx, comm); err != nil {
		t.Fatalf("insert community: %v", err)
	}
	for _, uid := range []string{ownerID, memberID} {
		if _, err := testStorage.Insert(ctx, &models.CommunityUser{CommunityId: "acl-community", UserId: uid, InviterId: ownerID}); err != nil {
			t.Fatalf("add member %s: %v", uid, err)
		}
	}

	// Insert gear owned by ownerID and shared with acl-community
	gearID := "acl-gear"
	if _, err := testStorage.Insert(ctx, &models.Gear{Id: gearID, Name: "ACL Gear", OwnerId: ownerID}); err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	if _, err := testStorage.Insert(ctx, &models.CommunityGear{CommunityId: "acl-community", GearId: gearID}); err != nil {
		t.Fatalf("insert community gear: %v", err)
	}

	// Insert unshared gear (not in any CommunityGear row)
	unsharedGearID := "acl-unshared-gear"
	if _, err := testStorage.Insert(ctx, &models.Gear{Id: unsharedGearID, Name: "Unshared ACL Gear", OwnerId: ownerID}); err != nil {
		t.Fatalf("insert unshared gear: %v", err)
	}

	t.Run("owner can read shared gear", func(t *testing.T) {
		ctx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)
		resp, err := service.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{Id: gearID}))
		if err != nil {
			t.Fatalf("GetGear failed for owner: %v", err)
		}
		if resp.Msg.Id != gearID {
			t.Errorf("expected gearID %s, got %s", gearID, resp.Msg.Id)
		}
	})

	t.Run("owner can read unshared gear (owner bypass)", func(t *testing.T) {
		ctx := createAuthenticatedContext(ownerID, ownerID+"@example.com", models.Role_ROLE_USER)
		resp, err := service.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{Id: unsharedGearID}))
		if err != nil {
			t.Fatalf("GetGear failed for owner on unshared gear: %v", err)
		}
		if resp.Msg.Id != unsharedGearID {
			t.Errorf("expected gearID %s, got %s", unsharedGearID, resp.Msg.Id)
		}
	})

	t.Run("community member can read shared gear", func(t *testing.T) {
		ctx := createAuthenticatedContext(memberID, memberID+"@example.com", models.Role_ROLE_USER)
		resp, err := service.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{Id: gearID}))
		if err != nil {
			t.Fatalf("GetGear failed for member: %v", err)
		}
		if resp.Msg.Id != gearID {
			t.Errorf("expected gearID %s, got %s", gearID, resp.Msg.Id)
		}
	})

	t.Run("stranger is denied for shared gear", func(t *testing.T) {
		ctx := createAuthenticatedContext(strangerID, strangerID+"@example.com", models.Role_ROLE_USER)
		_, err := service.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{Id: gearID}))
		if err == nil {
			t.Fatal("expected PermissionDenied for stranger, got nil")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("expected PermissionDenied, got %v", connect.CodeOf(err))
		}
	})

	t.Run("stranger is denied for unshared gear", func(t *testing.T) {
		ctx := createAuthenticatedContext(strangerID, strangerID+"@example.com", models.Role_ROLE_USER)
		_, err := service.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{Id: unsharedGearID}))
		if err == nil {
			t.Fatal("expected PermissionDenied for stranger on unshared gear, got nil")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("expected PermissionDenied, got %v", connect.CodeOf(err))
		}
	})

	t.Run("ex-member with soft-deleted membership is denied", func(t *testing.T) {
		exMemberID := "acl-ex-member"
		if _, err := testStorage.Insert(ctx, &models.User{Id: exMemberID, Name: "Ex Member", Email: exMemberID + "@example.com"}); err != nil {
			t.Fatalf("insert ex-member user: %v", err)
		}
		now := int64(1_700_000_000)
		membership := &models.CommunityUser{
			CommunityId: "acl-community",
			UserId:      exMemberID,
			InviterId:   ownerID,
			Deleted: &models.DeletedMetadata{
				DeletedAtUnixSec: now,
				DeletedByUserId:  exMemberID,
			},
		}
		if _, err := testStorage.Insert(ctx, membership); err != nil {
			t.Fatalf("insert ex-member membership: %v", err)
		}
		exCtx := createAuthenticatedContext(exMemberID, exMemberID+"@example.com", models.Role_ROLE_USER)
		_, err := service.GetGear(exCtx, connect.NewRequest(&api.GetGearRequest{Id: gearID}))
		if err == nil {
			t.Fatal("expected PermissionDenied for ex-member, got nil")
		}
		if connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("expected PermissionDenied, got %v", connect.CodeOf(err))
		}
	})
}
