package gear

import (
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/services"
)

// TestService_SaveGear_MetadataAndCarbon tests that SaveGear persists metadata fields
// and computes embodied carbon when the estimator config is set.
func TestService_SaveGear_MetadataAndCarbon(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	// Load real estimator config for carbon computation
	cfg, err := estimator.LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("Failed to load estimator config: %v", err)
	}
	service.SetEstimatorConfig(cfg)

	ctx := createAuthenticatedContext("user-meta-1", "meta@example.com", models.Role_ROLE_USER)

	t.Run("saves metadata fields on insert", func(t *testing.T) {
		req := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("DeWalt Cordless Drill"),
			Description: proto.String("20V MAX cordless drill"),
			Metadata: &api.GearMetadata{
				Category:         &api.TrackedString{Value: "Power Tools"},
				Brand:            &api.TrackedString{Value: "DeWalt"},
				Model:            &api.TrackedString{Value: "DCD771C2"},
				MaterialCategory: &api.TrackedMaterialCategory{Value: api.MaterialCategory_MATERIAL_CATEGORY_CORDLESS_POWER_TOOL},
				WeightGrams:      &api.TrackedEstimate{Value: &api.Estimate{Mean: 1800, Stddev: 540}},
				ValueEstimate: &api.ValueEstimate{
					EstimatedValueUsd: 99.0,
					Provenance: &api.Provenance{
						Source:    api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
						Name:      "genai_value_estimate",
						Reasoning: proto.String("Market price"),
					},
				},
			},
		})

		resp, err := service.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		// Verify all metadata was persisted
		stored := &models.Gear{}
		err = testStorage.GetByID(ctx, resp.Msg.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve stored gear: %v", err)
		}

		if stored.Category == nil || stored.Category.Value != "Power Tools" {
			t.Errorf("Expected category 'Power Tools', got %v", stored.Category)
		}
		if stored.Brand == nil || stored.Brand.Value != "DeWalt" {
			t.Errorf("Expected brand 'DeWalt', got %v", stored.Brand)
		}
		if stored.Model == nil || stored.Model.Value != "DCD771C2" {
			t.Errorf("Expected model 'DCD771C2', got %v", stored.Model)
		}
		if stored.MaterialCategory == nil || stored.MaterialCategory.Value != models.MaterialCategory_MATERIAL_CATEGORY_CORDLESS_POWER_TOOL {
			t.Errorf("Expected CORDLESS_POWER_TOOL, got %v", stored.MaterialCategory)
		}
		if stored.WeightGrams == nil || stored.WeightGrams.Value == nil {
			t.Fatal("Expected WeightGrams to be stored")
		}
		if stored.WeightGrams.Value.Mean != 1800 {
			t.Errorf("Expected weight mean 1800, got %f", stored.WeightGrams.Value.Mean)
		}
		if stored.WeightGrams.Value.Stddev != 540 {
			t.Errorf("Expected weight stddev 540, got %f", stored.WeightGrams.Value.Stddev)
		}

		t.Logf("Saved metadata: category=%q, brand=%q, model=%q, material=%v, weight=%.0f±%.0f g",
			stored.Category.GetValue(), stored.Brand.GetValue(), stored.Model.GetValue(),
			stored.MaterialCategory.GetValue(), stored.WeightGrams.Value.Mean, stored.WeightGrams.Value.Stddev)
	})

	t.Run("computes embodied carbon from material and weight", func(t *testing.T) {
		req := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Steel Hammer"),
			Description: proto.String("Heavy steel hammer"),
			Metadata: &api.GearMetadata{
				MaterialCategory: &api.TrackedMaterialCategory{Value: api.MaterialCategory_MATERIAL_CATEGORY_SOLID_METAL},
				WeightGrams:      &api.TrackedEstimate{Value: &api.Estimate{Mean: 1000, Stddev: 300}},
			},
		})

		resp, err := service.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		stored := &models.Gear{}
		err = testStorage.GetByID(ctx, resp.Msg.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve stored gear: %v", err)
		}

		// Embodied carbon should be computed from WEIGHT_MATERIAL path
		if stored.EmbodiedCarbon == nil {
			t.Fatal("Expected EmbodiedCarbon to be computed")
		}
		if stored.EmbodiedCarbon.Co2EGrams == nil {
			t.Fatal("Expected CO2e grams Estimate to be set")
		}
		if stored.EmbodiedCarbon.Co2EGrams.Mean <= 0 {
			t.Errorf("Expected positive CO2e grams, got %f", stored.EmbodiedCarbon.Co2EGrams.Mean)
		}
		if stored.EmbodiedCarbon.Co2EGrams.Stddev <= 0 {
			t.Errorf("Expected positive CO2e stddev, got %f", stored.EmbodiedCarbon.Co2EGrams.Stddev)
		}
		if stored.EmbodiedCarbon.Provenance == nil || stored.EmbodiedCarbon.Provenance.Name != "weight_material_carbon" {
			t.Errorf("Expected weight_material_carbon provenance, got %v", stored.EmbodiedCarbon.GetProvenance().GetName())
		}

		t.Logf("Embodied carbon: %.0f ± %.0f g CO2e (provenance: %v)",
			stored.EmbodiedCarbon.Co2EGrams.Mean, stored.EmbodiedCarbon.Co2EGrams.Stddev,
			stored.EmbodiedCarbon.GetProvenance().GetName())
	})

	t.Run("falls back to spend-based carbon when no material or weight", func(t *testing.T) {
		req := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Mystery Item"),
			Description: proto.String("Unknown item with only value"),
			Metadata: &api.GearMetadata{
				Category: &api.TrackedString{Value: "general_consumer_goods"},
				ValueEstimate: &api.ValueEstimate{
					EstimatedValueUsd: 100.0,
					Provenance: &api.Provenance{
						Source:    api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
						Name:      "genai_value_estimate",
						Reasoning: proto.String("Rough estimate"),
					},
				},
			},
		})

		resp, err := service.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		stored := &models.Gear{}
		err = testStorage.GetByID(ctx, resp.Msg.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve stored gear: %v", err)
		}

		// Should use spend-based method as fallback
		if stored.EmbodiedCarbon == nil {
			t.Fatal("Expected EmbodiedCarbon to be computed via spend-based fallback")
		}
		if stored.EmbodiedCarbon.Provenance == nil || stored.EmbodiedCarbon.Provenance.Name != "spend_based_carbon" {
			t.Errorf("Expected spend_based_carbon provenance, got %v", stored.EmbodiedCarbon.GetProvenance().GetName())
		}

		t.Logf("Spend-based carbon: %.0f ± %.0f g CO2e",
			stored.EmbodiedCarbon.Co2EGrams.Mean, stored.EmbodiedCarbon.Co2EGrams.Stddev)
	})

	t.Run("no carbon computed without estimator config", func(t *testing.T) {
		serviceNoConfig := New(testStorage, mockBucket)
		// Don't set estimator config

		req := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Drill Without Config"),
			Description: proto.String("Should have no carbon"),
			Metadata: &api.GearMetadata{
				MaterialCategory: &api.TrackedMaterialCategory{Value: api.MaterialCategory_MATERIAL_CATEGORY_SOLID_METAL},
				WeightGrams:      &api.TrackedEstimate{Value: &api.Estimate{Mean: 2000, Stddev: 600}},
			},
		})

		resp, err := serviceNoConfig.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		stored := &models.Gear{}
		err = testStorage.GetByID(ctx, resp.Msg.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve stored gear: %v", err)
		}

		if stored.EmbodiedCarbon != nil {
			t.Errorf("Expected nil EmbodiedCarbon without estimator config, got %+v", stored.EmbodiedCarbon)
		}
	})

	t.Run("no carbon computed with no material and no value", func(t *testing.T) {
		req := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Empty Item"),
			Description: proto.String("No metadata at all"),
		})

		resp, err := service.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		stored := &models.Gear{}
		err = testStorage.GetByID(ctx, resp.Msg.Id, stored)
		if err != nil {
			t.Fatalf("Failed to retrieve stored gear: %v", err)
		}

		// No material, no weight, no value - no carbon estimation possible
		if stored.EmbodiedCarbon != nil {
			t.Errorf("Expected nil EmbodiedCarbon with no metadata, got %+v", stored.EmbodiedCarbon)
		}
	})
}

func TestService_SaveGear_InsertProvenance(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	cfg, err := estimator.LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("Failed to load estimator config: %v", err)
	}
	service.SetEstimatorConfig(cfg)

	ctx := createAuthenticatedContext("user-prov-1", "prov@example.com", models.Role_ROLE_USER)

	t.Run("sets LLM provenance on metadata fields for text mode", func(t *testing.T) {
		req := connect.NewRequest(&api.SaveGearRequest{
			Name:           proto.String("AI-Generated Drill"),
			Description:    proto.String("From text generation"),
			GenerationMode: proto.String("text"),
			Metadata: &api.GearMetadata{
				Category:         &api.TrackedString{Value: "Power Tools"},
				Brand:            &api.TrackedString{Value: "DeWalt"},
				Model:            &api.TrackedString{Value: "DCD771C2"},
				MaterialCategory: &api.TrackedMaterialCategory{Value: api.MaterialCategory_MATERIAL_CATEGORY_CORDLESS_POWER_TOOL},
				WeightGrams:      &api.TrackedEstimate{Value: &api.Estimate{Mean: 1800, Stddev: 540}},
			},
		})

		resp, err := service.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		stored := &models.Gear{}
		if err := testStorage.GetByID(ctx, resp.Msg.Id, stored); err != nil {
			t.Fatalf("Failed to retrieve stored gear: %v", err)
		}

		// All metadata fields should have LLM provenance with genai_from_text name
		for _, tc := range []struct {
			name string
			prov *models.Provenance
		}{
			{"category", stored.Category.GetProvenance()},
			{"brand", stored.Brand.GetProvenance()},
			{"model", stored.Model.GetProvenance()},
			{"material_category", stored.MaterialCategory.GetProvenance()},
			{"weight_grams", stored.WeightGrams.GetProvenance()},
		} {
			if tc.prov == nil {
				t.Errorf("%s: expected provenance, got nil", tc.name)
				continue
			}
			if tc.prov.Source != models.ProvenanceSource_PROVENANCE_SOURCE_LLM {
				t.Errorf("%s: expected LLM source, got %v", tc.name, tc.prov.Source)
			}
			if tc.prov.Name != "genai_from_text" {
				t.Errorf("%s: expected genai_from_text, got %q", tc.name, tc.prov.Name)
			}
			if tc.prov.Version <= 0 {
				t.Errorf("%s: expected version > 0, got %d", tc.name, tc.prov.Version)
			}
		}
	})

	t.Run("sets LLM provenance for image mode", func(t *testing.T) {
		req := connect.NewRequest(&api.SaveGearRequest{
			Name:           proto.String("Image-Detected Tent"),
			Description:    proto.String("From image detection"),
			GenerationMode: proto.String("image"),
			Metadata: &api.GearMetadata{
				Category: &api.TrackedString{Value: "Camping"},
				Brand:    &api.TrackedString{Value: "REI"},
			},
		})

		resp, err := service.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		stored := &models.Gear{}
		if err := testStorage.GetByID(ctx, resp.Msg.Id, stored); err != nil {
			t.Fatalf("Failed to retrieve stored gear: %v", err)
		}

		if stored.Category.GetProvenance().GetName() != "genai_from_image" {
			t.Errorf("Expected genai_from_image, got %q", stored.Category.GetProvenance().GetName())
		}
	})

	t.Run("sets LLM provenance for web mode", func(t *testing.T) {
		req := connect.NewRequest(&api.SaveGearRequest{
			Name:           proto.String("Web-Extracted Table Saw"),
			Description:    proto.String("From product page"),
			GenerationMode: proto.String("web"),
			Metadata: &api.GearMetadata{
				Category: &api.TrackedString{Value: "Power Tools"},
			},
		})

		resp, err := service.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		stored := &models.Gear{}
		if err := testStorage.GetByID(ctx, resp.Msg.Id, stored); err != nil {
			t.Fatalf("Failed to retrieve stored gear: %v", err)
		}

		if stored.Category.GetProvenance().GetName() != "genai_from_web" {
			t.Errorf("Expected genai_from_web, got %q", stored.Category.GetProvenance().GetName())
		}
	})

	t.Run("no provenance set for manual creation (empty mode)", func(t *testing.T) {
		req := connect.NewRequest(&api.SaveGearRequest{
			Name:        proto.String("Manual Hammer"),
			Description: proto.String("User-created gear"),
			Metadata: &api.GearMetadata{
				Category: &api.TrackedString{Value: "Hand Tools"},
			},
		})

		resp, err := service.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		stored := &models.Gear{}
		if err := testStorage.GetByID(ctx, resp.Msg.Id, stored); err != nil {
			t.Fatalf("Failed to retrieve stored gear: %v", err)
		}

		if stored.Category.GetProvenance() != nil {
			t.Errorf("Expected nil provenance for manual creation, got %v", stored.Category.GetProvenance())
		}
	})

	t.Run("preserves existing provenance from client", func(t *testing.T) {
		req := connect.NewRequest(&api.SaveGearRequest{
			Name:           proto.String("Pre-Provenanced Item"),
			Description:    proto.String("Client sent explicit provenance"),
			GenerationMode: proto.String("text"),
			Metadata: &api.GearMetadata{
				Category: &api.TrackedString{
					Value: "Gardening",
					Provenance: &api.Provenance{
						Source: api.ProvenanceSource_PROVENANCE_SOURCE_USER,
						Name:   "user_override",
					},
				},
			},
		})

		resp, err := service.SaveGear(ctx, req)
		if err != nil {
			t.Fatalf("SaveGear failed: %v", err)
		}

		stored := &models.Gear{}
		if err := testStorage.GetByID(ctx, resp.Msg.Id, stored); err != nil {
			t.Fatalf("Failed to retrieve stored gear: %v", err)
		}

		// Client-provided provenance should be preserved, not overwritten
		if stored.Category.GetProvenance().GetName() != "user_override" {
			t.Errorf("Expected user_override provenance preserved, got %q", stored.Category.GetProvenance().GetName())
		}
		if stored.Category.GetProvenance().GetSource() != models.ProvenanceSource_PROVENANCE_SOURCE_USER {
			t.Errorf("Expected USER source preserved, got %v", stored.Category.GetProvenance().GetSource())
		}
	})
}

func TestService_SaveGear_UpdateUserProvenance(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	service := New(testStorage, mockBucket)

	cfg, err := estimator.LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("Failed to load estimator config: %v", err)
	}
	service.SetEstimatorConfig(cfg)

	ctx := createAuthenticatedContext("user-upd-1", "upd@example.com", models.Role_ROLE_USER)

	// Helper: insert gear with LLM provenance
	insertReq := connect.NewRequest(&api.SaveGearRequest{
		Name:           proto.String("Original Drill"),
		Description:    proto.String("Initial description"),
		GenerationMode: proto.String("text"),
		Metadata: &api.GearMetadata{
			Category:         &api.TrackedString{Value: "Power Tools"},
			Brand:            &api.TrackedString{Value: "DeWalt"},
			MaterialCategory: &api.TrackedMaterialCategory{Value: api.MaterialCategory_MATERIAL_CATEGORY_CORDLESS_POWER_TOOL},
			WeightGrams:      &api.TrackedEstimate{Value: &api.Estimate{Mean: 1800, Stddev: 540}},
		},
	})
	resp, err := service.SaveGear(ctx, insertReq)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	gearID := resp.Msg.Id

	t.Run("user edit sets USER provenance on changed field", func(t *testing.T) {
		updateReq := connect.NewRequest(&api.SaveGearRequest{
			Id: gearID,
			Metadata: &api.GearMetadata{
				Category: &api.TrackedString{Value: "Hand Tools"}, // changed
				Brand:    &api.TrackedString{Value: "DeWalt"},     // same
			},
		})
		_, err := service.SaveGear(ctx, updateReq)
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		stored := &models.Gear{}
		if err := testStorage.GetByID(ctx, gearID, stored); err != nil {
			t.Fatalf("GetByID failed: %v", err)
		}

		// Category changed → USER provenance
		if stored.Category.GetProvenance().GetSource() != models.ProvenanceSource_PROVENANCE_SOURCE_USER {
			t.Errorf("category: expected USER source, got %v", stored.Category.GetProvenance().GetSource())
		}

		// Brand unchanged → keeps original LLM provenance
		if stored.Brand.GetProvenance().GetSource() != models.ProvenanceSource_PROVENANCE_SOURCE_LLM {
			t.Errorf("brand: expected LLM source preserved, got %v", stored.Brand.GetProvenance().GetSource())
		}
		if stored.Brand.GetProvenance().GetName() != "genai_from_text" {
			t.Errorf("brand: expected genai_from_text, got %q", stored.Brand.GetProvenance().GetName())
		}
	})

	t.Run("material/weight edit recomputes embodied carbon", func(t *testing.T) {
		// Record original carbon
		stored := &models.Gear{}
		if err := testStorage.GetByID(ctx, gearID, stored); err != nil {
			t.Fatalf("GetByID failed: %v", err)
		}
		originalCarbon := float32(0)
		if stored.EmbodiedCarbon != nil && stored.EmbodiedCarbon.Co2EGrams != nil {
			originalCarbon = stored.EmbodiedCarbon.Co2EGrams.Mean
		}

		updateReq := connect.NewRequest(&api.SaveGearRequest{
			Id: gearID,
			Metadata: &api.GearMetadata{
				MaterialCategory: &api.TrackedMaterialCategory{Value: api.MaterialCategory_MATERIAL_CATEGORY_SOLID_METAL},
				WeightGrams:      &api.TrackedEstimate{Value: &api.Estimate{Mean: 5000, Stddev: 1500}},
			},
		})
		_, err := service.SaveGear(ctx, updateReq)
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		if err := testStorage.GetByID(ctx, gearID, stored); err != nil {
			t.Fatalf("GetByID failed: %v", err)
		}

		// Material category and weight should have USER provenance
		if stored.MaterialCategory.GetProvenance().GetSource() != models.ProvenanceSource_PROVENANCE_SOURCE_USER {
			t.Errorf("material: expected USER source, got %v", stored.MaterialCategory.GetProvenance().GetSource())
		}
		if stored.WeightGrams.GetProvenance().GetSource() != models.ProvenanceSource_PROVENANCE_SOURCE_USER {
			t.Errorf("weight: expected USER source, got %v", stored.WeightGrams.GetProvenance().GetSource())
		}

		// Carbon should be recomputed (different from original)
		if stored.EmbodiedCarbon == nil {
			t.Fatal("Expected embodied carbon to be recomputed")
		}
		if stored.EmbodiedCarbon.Co2EGrams.Mean == originalCarbon {
			t.Error("Expected carbon to change after material/weight edit")
		}
	})

	t.Run("unchanged metadata preserves existing provenance", func(t *testing.T) {
		// Re-read current stored values
		stored := &models.Gear{}
		if err := testStorage.GetByID(ctx, gearID, stored); err != nil {
			t.Fatalf("GetByID failed: %v", err)
		}
		currentCategory := stored.Category.GetValue()
		currentCategorySource := stored.Category.GetProvenance().GetSource()

		// Send same value back
		updateReq := connect.NewRequest(&api.SaveGearRequest{
			Id: gearID,
			Metadata: &api.GearMetadata{
				Category: &api.TrackedString{Value: currentCategory},
			},
		})
		_, err := service.SaveGear(ctx, updateReq)
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		if err := testStorage.GetByID(ctx, gearID, stored); err != nil {
			t.Fatalf("GetByID failed: %v", err)
		}

		// Provenance source should be preserved
		if stored.Category.GetProvenance().GetSource() != currentCategorySource {
			t.Errorf("Expected source %v preserved, got %v", currentCategorySource, stored.Category.GetProvenance().GetSource())
		}
	})
}
