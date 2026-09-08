package impact_metrics

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// TestAPIImpactToModelsRoundtrip verifies that api→models→api conversion preserves all fields.
func TestAPIImpactToModelsRoundtrip(t *testing.T) {
	original := &api.ImpactEstimate{
		MoneySaved: &api.MoneySavings{
			ValueUsd: &api.Estimate{Mean: 42.5, Stddev: 12.7},
			Provenance: &api.Provenance{
				Source:    api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:      "prevented_purchase",
				Version:   1,
				Reasoning: proto.String("test reasoning"),
				Sources:   []string{"source1", "source2"},
			},
		},
		EmissionsPrevented: &api.PreventedEmissions{
			ManufactureAvoidedCarbon: &api.CarbonEstimate{
				Co2EGrams: &api.Estimate{Mean: 3000, Stddev: 300},
				Provenance: &api.Provenance{
					Source:  api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
					Name:    "category_average_carbon",
					Version: 1,
				},
			},
			WasteReducedCarbon: &api.CarbonEstimate{
				Co2EGrams: &api.Estimate{Mean: 1000, Stddev: 100},
				Provenance: &api.Provenance{
					Source:  api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
					Name:    "weight_material_carbon",
					Version: 1,
				},
			},
			Provenance: &api.Provenance{
				Source:    api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:      "weight_material_carbon",
				Version:   1,
				Reasoning: proto.String("carbon reasoning"),
				Sources:   []string{"carbon source"},
			},
		},
		TimeSaved: &api.TimeSavings{
			Minutes: &api.Estimate{Mean: 60, Stddev: 18},
			Provenance: &api.Provenance{
				Source:    api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT,
				Name:      "gear_shopping_time",
				Version:   1,
				Reasoning: proto.String("time reasoning"),
				Sources:   []string{"time source"},
			},
		},
	}

	// Convert api → models → api.
	modelsIE := APIImpactToModels(original)
	roundtripped := ModelsImpactToAPI(modelsIE)

	// Verify MoneySaved.
	if roundtripped.MoneySaved == nil {
		t.Fatal("MoneySaved is nil after roundtrip")
	}
	if roundtripped.MoneySaved.ValueUsd.Mean != 42.5 || roundtripped.MoneySaved.ValueUsd.Stddev != 12.7 {
		t.Errorf("MoneySaved.ValueUsd = {%v, %v}, want {42.5, 12.7}",
			roundtripped.MoneySaved.ValueUsd.Mean, roundtripped.MoneySaved.ValueUsd.Stddev)
	}
	if roundtripped.MoneySaved.GetProvenance().GetReasoning() != "test reasoning" {
		t.Errorf("MoneySaved.Provenance.Reasoning = %q, want %q", roundtripped.MoneySaved.GetProvenance().GetReasoning(), "test reasoning")
	}
	if len(roundtripped.MoneySaved.GetProvenance().GetSources()) != 2 {
		t.Errorf("MoneySaved.Provenance.Sources len = %d, want 2", len(roundtripped.MoneySaved.GetProvenance().GetSources()))
	}
	if roundtripped.MoneySaved.GetProvenance().GetName() != "prevented_purchase" {
		t.Errorf("MoneySaved.Provenance.Name = %q, want %q", roundtripped.MoneySaved.GetProvenance().GetName(), "prevented_purchase")
	}
	if roundtripped.MoneySaved.GetProvenance().GetVersion() != 1 {
		t.Errorf("MoneySaved.Provenance.Version = %d, want 1", roundtripped.MoneySaved.GetProvenance().GetVersion())
	}

	// Verify EmissionsPrevented.
	if roundtripped.EmissionsPrevented == nil {
		t.Fatal("EmissionsPrevented is nil after roundtrip")
	}
	mac := roundtripped.EmissionsPrevented.ManufactureAvoidedCarbon
	if mac == nil || mac.Co2EGrams.Mean != 3000 || mac.Co2EGrams.Stddev != 300 {
		t.Errorf("ManufactureAvoidedCarbon = %v, want mean=3000 stddev=300", mac)
	}
	if mac.Provenance == nil || mac.Provenance.Name != "category_average_carbon" {
		t.Errorf("ManufactureAvoidedCarbon.Provenance.Name = %v, want category_average_carbon", mac.Provenance.GetName())
	}
	wrc := roundtripped.EmissionsPrevented.WasteReducedCarbon
	if wrc == nil || wrc.Co2EGrams.Mean != 1000 || wrc.Co2EGrams.Stddev != 100 {
		t.Errorf("WasteReducedCarbon = %v, want mean=1000 stddev=100", wrc)
	}
	if wrc.Provenance == nil || wrc.Provenance.Name != "weight_material_carbon" {
		t.Errorf("WasteReducedCarbon.Provenance.Name = %v, want weight_material_carbon", wrc.Provenance.GetName())
	}
	if roundtripped.EmissionsPrevented.GetProvenance().GetReasoning() != "carbon reasoning" {
		t.Errorf("EmissionsPrevented.Provenance.Reasoning = %q, want %q", roundtripped.EmissionsPrevented.GetProvenance().GetReasoning(), "carbon reasoning")
	}
	if roundtripped.EmissionsPrevented.GetProvenance().GetName() != "weight_material_carbon" {
		t.Errorf("EmissionsPrevented.Provenance.Name = %q, want %q", roundtripped.EmissionsPrevented.GetProvenance().GetName(), "weight_material_carbon")
	}
	if roundtripped.EmissionsPrevented.GetProvenance().GetVersion() != 1 {
		t.Errorf("EmissionsPrevented.Provenance.Version = %d, want 1", roundtripped.EmissionsPrevented.GetProvenance().GetVersion())
	}
	if mac.GetProvenance().GetVersion() != 1 {
		t.Errorf("ManufactureAvoidedCarbon.Provenance.Version = %d, want 1", mac.GetProvenance().GetVersion())
	}
	if wrc.GetProvenance().GetVersion() != 1 {
		t.Errorf("WasteReducedCarbon.Provenance.Version = %d, want 1", wrc.GetProvenance().GetVersion())
	}

	// Verify TimeSaved.
	if roundtripped.TimeSaved == nil {
		t.Fatal("TimeSaved is nil after roundtrip")
	}
	if roundtripped.TimeSaved.Minutes.Mean != 60 || roundtripped.TimeSaved.Minutes.Stddev != 18 {
		t.Errorf("TimeSaved.Minutes = {%v, %v}, want {60, 18}",
			roundtripped.TimeSaved.Minutes.Mean, roundtripped.TimeSaved.Minutes.Stddev)
	}
	if roundtripped.TimeSaved.GetProvenance().GetReasoning() != "time reasoning" {
		t.Errorf("TimeSaved.Provenance.Reasoning = %q, want %q", roundtripped.TimeSaved.GetProvenance().GetReasoning(), "time reasoning")
	}
	if roundtripped.TimeSaved.GetProvenance().GetName() != "gear_shopping_time" {
		t.Errorf("TimeSaved.Provenance.Name = %q, want %q", roundtripped.TimeSaved.GetProvenance().GetName(), "gear_shopping_time")
	}
	if roundtripped.TimeSaved.GetProvenance().GetVersion() != 1 {
		t.Errorf("TimeSaved.Provenance.Version = %d, want 1", roundtripped.TimeSaved.GetProvenance().GetVersion())
	}
}

// TestAPIImpactToModelsNil verifies nil input returns nil output.
func TestAPIImpactToModelsNil(t *testing.T) {
	if APIImpactToModels(nil) != nil {
		t.Error("APIImpactToModels(nil) should be nil")
	}
}

// TestCalculateTotalValue tests the total value calculation for community gear.
func TestCalculateTotalValue(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, db *storage.ProtoSQLStorage, communityID string)
		wantTotalUsd  float32
		wantGearCount int32
	}{
		{
			name:          "empty community",
			setup:         func(t *testing.T, db *storage.ProtoSQLStorage, communityID string) {},
			wantTotalUsd:  0,
			wantGearCount: 0,
		},
		{
			name: "single gear with value",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, communityID string) {
				ctx := context.Background()
				gear := &models.Gear{
					Id:   uuid.New().String(),
					Name: "Test Gear",
					ValueEstimate: &models.ValueEstimate{
						EstimatedValueUsd: 100.0,
					},
				}
				if _, err := db.Insert(ctx, gear); err != nil {
					t.Fatalf("Failed to insert gear: %v", err)
				}

				communityGear := &models.CommunityGear{
					Id:          uuid.New().String(),
					CommunityId: communityID,
					GearId:      gear.Id,
				}
				if _, err := db.Insert(ctx, communityGear); err != nil {
					t.Fatalf("Failed to insert community gear: %v", err)
				}
			},
			wantTotalUsd:  100.0,
			wantGearCount: 1,
		},
		{
			name: "multiple gear with values",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, communityID string) {
				ctx := context.Background()

				for _, g := range []struct {
					name  string
					value float32
				}{
					{"Gear 1", 100.0},
					{"Gear 2", 250.0},
					{"Gear 3", 50.0},
				} {
					gear := &models.Gear{
						Id:   uuid.New().String(),
						Name: g.name,
						ValueEstimate: &models.ValueEstimate{
							EstimatedValueUsd: g.value,
						},
					}
					if _, err := db.Insert(ctx, gear); err != nil {
						t.Fatalf("Failed to insert %s: %v", g.name, err)
					}
					cg := &models.CommunityGear{
						Id:          uuid.New().String(),
						CommunityId: communityID,
						GearId:      gear.Id,
					}
					if _, err := db.Insert(ctx, cg); err != nil {
						t.Fatalf("Failed to insert community gear for %s: %v", g.name, err)
					}
				}
			},
			wantTotalUsd:  400.0,
			wantGearCount: 3,
		},
		{
			name: "gear without value estimate",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, communityID string) {
				ctx := context.Background()

				// Gear with value.
				gear1 := &models.Gear{
					Id:            uuid.New().String(),
					Name:          "Valued Gear",
					ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 100.0},
				}
				if _, err := db.Insert(ctx, gear1); err != nil {
					t.Fatalf("Failed to insert gear1: %v", err)
				}
				cg1 := &models.CommunityGear{Id: uuid.New().String(), CommunityId: communityID, GearId: gear1.Id}
				if _, err := db.Insert(ctx, cg1); err != nil {
					t.Fatalf("Failed to insert community gear1: %v", err)
				}

				// Gear without value estimate.
				gear2 := &models.Gear{Id: uuid.New().String(), Name: "Unvalued Gear", ValueEstimate: nil}
				if _, err := db.Insert(ctx, gear2); err != nil {
					t.Fatalf("Failed to insert gear2: %v", err)
				}
				cg2 := &models.CommunityGear{Id: uuid.New().String(), CommunityId: communityID, GearId: gear2.Id}
				if _, err := db.Insert(ctx, cg2); err != nil {
					t.Fatalf("Failed to insert community gear2: %v", err)
				}

				// Gear with zero value.
				gear3 := &models.Gear{
					Id:            uuid.New().String(),
					Name:          "Zero Value Gear",
					ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 0},
				}
				if _, err := db.Insert(ctx, gear3); err != nil {
					t.Fatalf("Failed to insert gear3: %v", err)
				}
				cg3 := &models.CommunityGear{Id: uuid.New().String(), CommunityId: communityID, GearId: gear3.Id}
				if _, err := db.Insert(ctx, cg3); err != nil {
					t.Fatalf("Failed to insert community gear3: %v", err)
				}
			},
			wantTotalUsd:  100.0,
			wantGearCount: 3,
		},
		{
			name: "deleted gear excluded",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, communityID string) {
				ctx := context.Background()
				now := time.Now().Unix()

				gear1 := &models.Gear{
					Id:            uuid.New().String(),
					Name:          "Active Gear",
					ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 100.0},
				}
				if _, err := db.Insert(ctx, gear1); err != nil {
					t.Fatalf("Failed to insert gear1: %v", err)
				}
				cg1 := &models.CommunityGear{Id: uuid.New().String(), CommunityId: communityID, GearId: gear1.Id}
				if _, err := db.Insert(ctx, cg1); err != nil {
					t.Fatalf("Failed to insert community gear1: %v", err)
				}

				gear2 := &models.Gear{
					Id:            uuid.New().String(),
					Name:          "Deleted Gear",
					ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 500.0},
					Deleted:       &models.DeletedMetadata{DeletedAtUnixSec: now, DeletedByUserId: "user123"},
				}
				if _, err := db.Insert(ctx, gear2); err != nil {
					t.Fatalf("Failed to insert gear2: %v", err)
				}
				cg2 := &models.CommunityGear{Id: uuid.New().String(), CommunityId: communityID, GearId: gear2.Id}
				if _, err := db.Insert(ctx, cg2); err != nil {
					t.Fatalf("Failed to insert community gear2: %v", err)
				}
			},
			wantTotalUsd:  100.0,
			wantGearCount: 1,
		},
		{
			name: "mixed scenario",
			setup: func(t *testing.T, db *storage.ProtoSQLStorage, communityID string) {
				ctx := context.Background()
				now := time.Now().Unix()

				// Two active gear with values.
				for _, g := range []struct {
					name  string
					value float32
				}{
					{"Active Valued Gear 1", 150.0},
					{"Active Valued Gear 2", 350.0},
				} {
					gear := &models.Gear{
						Id:            uuid.New().String(),
						Name:          g.name,
						ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: g.value},
					}
					if _, err := db.Insert(ctx, gear); err != nil {
						t.Fatalf("Failed to insert %s: %v", g.name, err)
					}
					cg := &models.CommunityGear{Id: uuid.New().String(), CommunityId: communityID, GearId: gear.Id}
					if _, err := db.Insert(ctx, cg); err != nil {
						t.Fatalf("Failed to insert community gear for %s: %v", g.name, err)
					}
				}

				// Gear without value.
				gear3 := &models.Gear{Id: uuid.New().String(), Name: "Unvalued Gear", ValueEstimate: nil}
				if _, err := db.Insert(ctx, gear3); err != nil {
					t.Fatalf("Failed to insert gear3: %v", err)
				}
				cg3 := &models.CommunityGear{Id: uuid.New().String(), CommunityId: communityID, GearId: gear3.Id}
				if _, err := db.Insert(ctx, cg3); err != nil {
					t.Fatalf("Failed to insert community gear3: %v", err)
				}

				// Deleted gear with value (should not count).
				gear4 := &models.Gear{
					Id:            uuid.New().String(),
					Name:          "Deleted Gear",
					ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 1000.0},
					Deleted:       &models.DeletedMetadata{DeletedAtUnixSec: now, DeletedByUserId: "user123"},
				}
				if _, err := db.Insert(ctx, gear4); err != nil {
					t.Fatalf("Failed to insert gear4: %v", err)
				}
				cg4 := &models.CommunityGear{Id: uuid.New().String(), CommunityId: communityID, GearId: gear4.Id}
				if _, err := db.Insert(ctx, cg4); err != nil {
					t.Fatalf("Failed to insert community gear4: %v", err)
				}
			},
			wantTotalUsd:  500.0, // $150 + $350 (only active valued gear)
			wantGearCount: 3,     // Active gear only
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDatabase(t)
			ctx := context.Background()
			communityID := uuid.New().String()

			tt.setup(t, db, communityID)

			calc := NewCalculator(db, loadTestEstimatorConfig(t))

			result, err := calc.CalculateTotalValue(ctx, communityID)
			if err != nil {
				t.Fatalf("CalculateTotalValue() error = %v", err)
			}

			if math.Abs(float64(result.TotalValueUsd-tt.wantTotalUsd)) > 0.01 {
				t.Errorf("TotalValueUsd = %v, want %v", result.TotalValueUsd, tt.wantTotalUsd)
			}
			if result.GearCount != tt.wantGearCount {
				t.Errorf("GearCount = %v, want %v", result.GearCount, tt.wantGearCount)
			}
		})
	}
}

// TestExtractDimensionValue tests dimension value extraction from ImpactEstimate.
func TestExtractDimensionValue(t *testing.T) {
	ie := testImpactEstimate(100, 5000, 30)
	// Add quality time.
	ie.QualityTime = &models.QualityTimeEstimate{
		QualityTimeMinutes: &models.Estimate{Mean: 45, Stddev: 10},
	}

	money := ExtractDimensionValue(ie, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY)
	if math.Abs(money-100) > 0.01 {
		t.Errorf("money = %v, want 100", money)
	}

	co2 := ExtractDimensionValue(ie, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_EMISSIONS)
	if math.Abs(co2-5000) > 0.01 {
		t.Errorf("co2 = %v, want 5000", co2)
	}

	qt := ExtractDimensionValue(ie, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_QUALITY_TIME)
	if math.Abs(qt-45) > 0.01 {
		t.Errorf("quality_time = %v, want 45", qt)
	}

	// Nil estimate returns 0.
	if v := ExtractDimensionValue(nil, api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY); v != 0 {
		t.Errorf("nil estimate: got %v, want 0", v)
	}
}
