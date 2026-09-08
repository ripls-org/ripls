package impact_metrics

import (
	"math"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
)

func loadTestConfig(t *testing.T) *estimator.Config {
	t.Helper()
	cfg, err := estimator.LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("failed to load estimator config: %v", err)
	}
	return cfg
}

func TestBuildTransferImpactMetrics_Loan(t *testing.T) {
	cfg := loadTestConfig(t)

	gear := &models.Gear{
		ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 200},
		EmbodiedCarbon: &models.CarbonEstimate{
			Co2EGrams: &models.Estimate{Mean: 10000, Stddev: 3000},
			Provenance: &models.Provenance{
				Source: models.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:   "weight_material_carbon",
			},
		},
		WeightGrams: &models.TrackedEstimate{Value: &models.Estimate{Mean: 2000, Stddev: 500}},
	}

	ie := BuildTransferImpactMetrics(gear, models.TransferType_TRANSFER_TYPE_LOAN, cfg, nil, nil)

	if ie == nil {
		t.Fatal("BuildTransferImpactMetrics returned nil")
	}

	// Money saved: 200 × loan_prevented_purchase_rate (0.50) = 100
	if ie.MoneySaved == nil || ie.MoneySaved.ValueUsd == nil {
		t.Fatal("MoneySaved is nil")
	}
	wantMoney := float32(200) * cfg.Global.GetLoanPreventedPurchaseRate()
	if ie.MoneySaved.ValueUsd.Mean != wantMoney {
		t.Errorf("MoneySaved.Mean = %v, want %v", ie.MoneySaved.ValueUsd.Mean, wantMoney)
	}

	// Emissions prevented should be non-nil (has embodied carbon + weight).
	if ie.EmissionsPrevented == nil {
		t.Fatal("EmissionsPrevented is nil")
	}
	if ie.EmissionsPrevented.ManufactureAvoidedCarbon == nil {
		t.Error("ManufactureAvoidedCarbon is nil")
	}
	if ie.EmissionsPrevented.WasteReducedCarbon == nil {
		t.Error("WasteReducedCarbon is nil")
	}

	// Time saved should use config default.
	if ie.TimeSaved == nil || ie.TimeSaved.Minutes == nil {
		t.Fatal("TimeSaved is nil")
	}
	if ie.TimeSaved.Minutes.Mean <= 0 {
		t.Errorf("TimeSaved.Minutes.Mean = %v, want > 0", ie.TimeSaved.Minutes.Mean)
	}
}

func TestBuildTransferImpactMetrics_Giveaway(t *testing.T) {
	cfg := loadTestConfig(t)

	gear := &models.Gear{
		ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 100},
	}

	ie := BuildTransferImpactMetrics(gear, models.TransferType_TRANSFER_TYPE_GIVEAWAY, cfg, nil, nil)

	if ie == nil {
		t.Fatal("BuildTransferImpactMetrics returned nil")
	}

	// Money saved: 100 × giveaway_prevented_purchase_rate (0.50) = 50
	wantMoney := float32(100) * cfg.Global.GetGiveawayPreventedPurchaseRate()
	if ie.MoneySaved == nil || ie.MoneySaved.ValueUsd == nil || ie.MoneySaved.ValueUsd.Mean != wantMoney {
		t.Errorf("MoneySaved.Mean = %v, want %v", CostUSD(ie), wantMoney)
	}
}

func TestBuildTransferImpactMetrics_EmptyGear(t *testing.T) {
	cfg := loadTestConfig(t)

	gear := &models.Gear{} // No value, no carbon, no weight

	ie := BuildTransferImpactMetrics(gear, models.TransferType_TRANSFER_TYPE_LOAN, cfg, nil, nil)

	if ie == nil {
		t.Fatal("BuildTransferImpactMetrics returned nil")
	}

	// Money saved should be nil (no value).
	if ie.MoneySaved != nil {
		t.Errorf("MoneySaved should be nil for empty gear, got %v", ie.MoneySaved)
	}

	// Emissions prevented should be nil (no carbon data).
	if ie.EmissionsPrevented != nil {
		t.Errorf("EmissionsPrevented should be nil for empty gear, got %v", ie.EmissionsPrevented)
	}

	// Time saved should still have config default.
	if ie.TimeSaved == nil || ie.TimeSaved.Minutes == nil || ie.TimeSaved.Minutes.Mean <= 0 {
		t.Error("TimeSaved should have config default even with empty gear")
	}
}

func TestBuildGearCumulativeImpactMetrics(t *testing.T) {
	cfg := loadTestConfig(t)

	gear := &models.Gear{
		ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 200},
		EmbodiedCarbon: &models.CarbonEstimate{
			Co2EGrams: &models.Estimate{Mean: 10000, Stddev: 3000},
			Provenance: &models.Provenance{
				Source: models.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:   "weight_material_carbon",
			},
		},
		WeightGrams: &models.TrackedEstimate{Value: &models.Estimate{Mean: 2000, Stddev: 500}},
	}

	ie := BuildGearCumulativeImpactMetrics(gear, 3, cfg)

	if ie == nil {
		t.Fatal("BuildGearCumulativeImpactMetrics returned nil")
	}

	// Single loan money: 200 × 0.50 = 100. Cumulative: 100 × 3 = 300.
	singleMoney := float32(200) * cfg.Global.GetLoanPreventedPurchaseRate()
	wantMoney := singleMoney * 3
	gotMoney := CostUSD(ie)
	if math.Abs(float64(gotMoney-wantMoney)) > 0.01 {
		t.Errorf("CostUSD = %v, want %v", gotMoney, wantMoney)
	}

	// Time should be scaled by 3.
	singleTime := float32(cfg.TimeDefaults.GetGearShoppingTimeMinutes())
	wantTime := singleTime * 3
	gotTime := TimeMinutes(ie)
	if math.Abs(float64(gotTime-wantTime)) > 0.01 {
		t.Errorf("TimeMinutes = %v, want %v", gotTime, wantTime)
	}

	// Emissions should be scaled by 3.
	if ie.EmissionsPrevented == nil {
		t.Fatal("EmissionsPrevented is nil")
	}
}

func TestBuildGearCumulativeImpactMetrics_ZeroLoans(t *testing.T) {
	cfg := loadTestConfig(t)
	gear := &models.Gear{ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 200}}

	ie := BuildGearCumulativeImpactMetrics(gear, 0, cfg)

	if ie == nil {
		t.Fatal("BuildGearCumulativeImpactMetrics returned nil")
	}
	if CostUSD(ie) != 0 {
		t.Errorf("CostUSD should be 0 for zero loans, got %v", CostUSD(ie))
	}
}

func TestBuildRequestImpactMetrics(t *testing.T) {
	cfg := loadTestConfig(t)

	ie := BuildRequestImpactMetrics(0, cfg, nil, nil)

	if ie == nil {
		t.Fatal("BuildRequestImpactMetrics returned nil")
	}

	// Money should be nil when valueUSD is 0.
	if ie.MoneySaved != nil {
		t.Errorf("MoneySaved should be nil for zero valueUSD, got %v", ie.MoneySaved)
	}

	// Emissions should use flat default from config.
	if cfg.Global.GetRequestDefaultCarbonGrams() > 0 {
		if ie.EmissionsPrevented == nil {
			t.Error("EmissionsPrevented should be non-nil when config has request default carbon")
		}
	}

	// Time should use config default.
	if ie.TimeSaved == nil || ie.TimeSaved.Minutes == nil {
		t.Fatal("TimeSaved is nil")
	}
	if ie.TimeSaved.Minutes.Mean <= 0 {
		t.Errorf("TimeSaved.Minutes.Mean = %v, want > 0", ie.TimeSaved.Minutes.Mean)
	}
}

func TestBuildExperienceImpactMetrics(t *testing.T) {
	cfg := loadTestConfig(t)

	ie := BuildExperienceImpactMetrics(0, 5, cfg, nil, nil)

	if ie == nil {
		t.Fatal("BuildExperienceImpactMetrics returned nil")
	}

	// Money should be nil when valueUSD is 0.
	if ie.MoneySaved != nil {
		t.Errorf("MoneySaved should be nil for zero valueUSD, got %v", ie.MoneySaved)
	}

	// Time should be scaled by attendeeCount (5).
	if ie.TimeSaved == nil || ie.TimeSaved.Minutes == nil {
		t.Fatal("TimeSaved is nil")
	}
	baseTime := float32(cfg.TimeDefaults.GetExperienceDurationMinutes())
	wantTime := baseTime * 5
	if math.Abs(float64(ie.TimeSaved.Minutes.Mean-wantTime)) > 0.01 {
		t.Errorf("TimeSaved.Minutes.Mean = %v, want %v", ie.TimeSaved.Minutes.Mean, wantTime)
	}
}

func TestBuildExperienceImpactMetrics_ZeroAttendees(t *testing.T) {
	cfg := loadTestConfig(t)

	ie := BuildExperienceImpactMetrics(0, 0, cfg, nil, nil)

	if ie == nil {
		t.Fatal("BuildExperienceImpactMetrics returned nil")
	}

	// Time should still have config default (multiplier clamped to 1).
	if ie.TimeSaved == nil || ie.TimeSaved.Minutes == nil {
		t.Fatal("TimeSaved is nil")
	}
	baseTime := float32(cfg.TimeDefaults.GetExperienceDurationMinutes())
	if ie.TimeSaved.Minutes.Mean != baseTime {
		t.Errorf("TimeSaved.Minutes.Mean = %v, want %v (base time, multiplier clamped to 1)", ie.TimeSaved.Minutes.Mean, baseTime)
	}
}

func TestBuildRequestImpactMetrics_WithValue(t *testing.T) {
	cfg := loadTestConfig(t)

	tests := []struct {
		name               string
		valueUSD           float32
		wantMoneySaved     bool
		wantProvenanceName string
	}{
		{"with value", 75.0, true, "service_value"},
		{"zero value (old record)", 0, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ie := BuildRequestImpactMetrics(tt.valueUSD, cfg, nil, nil)

			if ie == nil {
				t.Fatal("BuildRequestImpactMetrics returned nil")
			}

			if tt.wantMoneySaved {
				if ie.MoneySaved == nil {
					t.Error("MoneySaved should be non-nil when valueUSD > 0")
				} else {
					if ie.MoneySaved.GetProvenance().GetName() != tt.wantProvenanceName {
						t.Errorf("MoneySaved.Provenance.Name = %q, want %q", ie.MoneySaved.GetProvenance().GetName(), tt.wantProvenanceName)
					}
					if ie.MoneySaved.ValueUsd == nil {
						t.Error("MoneySaved.ValueUsd should be non-nil")
					}
				}
			} else {
				if ie.MoneySaved != nil {
					t.Errorf("MoneySaved should be nil for zero valueUSD, got %v", ie.MoneySaved)
				}
			}

			// Time and emissions should always be populated
			if ie.TimeSaved == nil {
				t.Error("TimeSaved should be non-nil")
			}
		})
	}
}

func TestBuildExperienceImpactMetrics_WithValue(t *testing.T) {
	cfg := loadTestConfig(t)

	tests := []struct {
		name               string
		valueUSD           float32
		attendeeCount      int32
		wantMoneySaved     bool
		wantProvenanceName string
	}{
		{"with value", 50.0, 5, true, "commercial_value"},
		{"zero value (old record)", 0, 5, false, ""},
		{"with value, zero attendees", 50.0, 0, true, "commercial_value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ie := BuildExperienceImpactMetrics(tt.valueUSD, tt.attendeeCount, cfg, nil, nil)

			if ie == nil {
				t.Fatal("BuildExperienceImpactMetrics returned nil")
			}

			if tt.wantMoneySaved {
				if ie.MoneySaved == nil {
					t.Error("MoneySaved should be non-nil when valueUSD > 0")
				} else {
					if ie.MoneySaved.GetProvenance().GetName() != tt.wantProvenanceName {
						t.Errorf("MoneySaved.Provenance.Name = %q, want %q", ie.MoneySaved.GetProvenance().GetName(), tt.wantProvenanceName)
					}
					if ie.MoneySaved.ValueUsd == nil {
						t.Error("MoneySaved.ValueUsd should be non-nil")
					}
				}
			} else {
				if ie.MoneySaved != nil {
					t.Errorf("MoneySaved should be nil for zero valueUSD, got %v", ie.MoneySaved)
				}
			}

			// Time should always be populated
			if ie.TimeSaved == nil {
				t.Error("TimeSaved should be non-nil")
			}
		})
	}
}

func TestGearCarbonToAPI(t *testing.T) {
	// nil input
	if result := gearCarbonToAPI(nil); result != nil {
		t.Errorf("gearCarbonToAPI(nil) = %v, want nil", result)
	}

	// Valid input
	mc := &models.CarbonEstimate{
		Co2EGrams: &models.Estimate{Mean: 5000, Stddev: 1500},
		Provenance: &models.Provenance{
			Source: models.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
			Name:   "weight_material_carbon",
		},
	}
	result := gearCarbonToAPI(mc)
	if result == nil {
		t.Fatal("gearCarbonToAPI returned nil for valid input")
	}
	if result.Co2EGrams.Mean != 5000 || result.Co2EGrams.Stddev != 1500 {
		t.Errorf("Co2EGrams = (%v, %v), want (5000, 1500)", result.Co2EGrams.Mean, result.Co2EGrams.Stddev)
	}
	if result.Provenance == nil || result.Provenance.Name != "weight_material_carbon" {
		t.Errorf("Provenance.Name = %v, want weight_material_carbon", result.Provenance.GetName())
	}
}

func TestScaleMoneySaved(t *testing.T) {
	// nil input
	if result := scaleMoneySaved(nil, 3); result != nil {
		t.Error("scaleMoneySaved(nil, 3) should return nil")
	}

	// multiplier <= 1 returns original
	ms := &api.MoneySavings{ValueUsd: &api.Estimate{Mean: 100, Stddev: 30}}
	result := scaleMoneySaved(ms, 1)
	if result != ms {
		t.Error("scaleMoneySaved with multiplier 1 should return original")
	}

	// multiplier > 1 scales
	result = scaleMoneySaved(ms, 3)
	if result.ValueUsd.Mean != 300 {
		t.Errorf("scaleMoneySaved Mean = %v, want 300", result.ValueUsd.Mean)
	}
}

func TestScaleEmissionsPrevented(t *testing.T) {
	// nil input
	if result := scaleEmissionsPrevented(nil, 3); result != nil {
		t.Error("scaleEmissionsPrevented(nil, 3) should return nil")
	}

	// multiplier <= 1 returns original
	pe := &api.PreventedEmissions{
		ManufactureAvoidedCarbon: &api.CarbonEstimate{
			Co2EGrams: &api.Estimate{Mean: 5000, Stddev: 1500},
		},
		WasteReducedCarbon: &api.CarbonEstimate{
			Co2EGrams: &api.Estimate{Mean: 1000, Stddev: 300},
		},
	}
	result := scaleEmissionsPrevented(pe, 1)
	if result != pe {
		t.Error("scaleEmissionsPrevented with multiplier 1 should return original")
	}

	// multiplier > 1 scales both components
	result = scaleEmissionsPrevented(pe, 3)
	if result.ManufactureAvoidedCarbon.Co2EGrams.Mean != 15000 {
		t.Errorf("ManufactureAvoidedCarbon Mean = %v, want 15000", result.ManufactureAvoidedCarbon.Co2EGrams.Mean)
	}
	if result.WasteReducedCarbon.Co2EGrams.Mean != 3000 {
		t.Errorf("WasteReducedCarbon Mean = %v, want 3000", result.WasteReducedCarbon.Co2EGrams.Mean)
	}
}

func TestScaleMoneySaved_PreservesProvenance(t *testing.T) {
	ms := &api.MoneySavings{
		ValueUsd: &api.Estimate{Mean: 100, Stddev: 30},
		Provenance: &api.Provenance{
			Source:  api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
			Name:    "prevented_purchase",
			Version: 1,
		},
	}
	result := scaleMoneySaved(ms, 3)
	if result.Provenance == nil {
		t.Fatal("scaleMoneySaved should preserve provenance")
	}
	if result.Provenance.Name != "prevented_purchase" {
		t.Errorf("Provenance.Name = %q, want %q", result.Provenance.Name, "prevented_purchase")
	}
	if result.Provenance.Source != api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA {
		t.Errorf("Provenance.Source = %v, want FORMULA", result.Provenance.Source)
	}
	if result.Provenance.Version != 1 {
		t.Errorf("Provenance.Version = %d, want 1", result.Provenance.Version)
	}
}

func TestScaleEmissionsPrevented_PreservesProvenance(t *testing.T) {
	pe := &api.PreventedEmissions{
		ManufactureAvoidedCarbon: &api.CarbonEstimate{
			Co2EGrams: &api.Estimate{Mean: 5000, Stddev: 1500},
			Provenance: &api.Provenance{
				Source:  api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:    "weight_material_carbon",
				Version: 1,
			},
		},
		WasteReducedCarbon: &api.CarbonEstimate{
			Co2EGrams: &api.Estimate{Mean: 1000, Stddev: 300},
			Provenance: &api.Provenance{
				Source: api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:   "waste_carbon",
			},
		},
		Provenance: &api.Provenance{
			Source:  api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
			Name:    "weight_material_carbon",
			Version: 1,
		},
	}
	result := scaleEmissionsPrevented(pe, 3)

	// Top-level provenance.
	if result.Provenance == nil || result.Provenance.Name != "weight_material_carbon" {
		t.Errorf("top-level Provenance.Name = %v, want weight_material_carbon", result.GetProvenance().GetName())
	}
	if result.Provenance.Version != 1 {
		t.Errorf("top-level Provenance.Version = %d, want 1", result.Provenance.Version)
	}

	// Sub-component provenance.
	if result.ManufactureAvoidedCarbon.Provenance == nil || result.ManufactureAvoidedCarbon.Provenance.Name != "weight_material_carbon" {
		t.Errorf("ManufactureAvoidedCarbon.Provenance.Name = %v, want weight_material_carbon",
			result.ManufactureAvoidedCarbon.GetProvenance().GetName())
	}
	if result.WasteReducedCarbon.Provenance == nil || result.WasteReducedCarbon.Provenance.Name != "waste_carbon" {
		t.Errorf("WasteReducedCarbon.Provenance.Name = %v, want waste_carbon",
			result.WasteReducedCarbon.GetProvenance().GetName())
	}
}

func TestGearCarbonToAPI_ProvenanceFields(t *testing.T) {
	mc := &models.CarbonEstimate{
		Co2EGrams: &models.Estimate{Mean: 5000, Stddev: 1500},
		Provenance: &models.Provenance{
			Source:    models.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
			Name:      "weight_material_carbon",
			Version:   1,
			Reasoning: proto.String("calculated from weight and material"),
			Sources:   []string{"EPA data"},
		},
	}
	result := gearCarbonToAPI(mc)
	if result.Provenance == nil {
		t.Fatal("gearCarbonToAPI should preserve provenance")
	}
	if result.Provenance.Source != api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA {
		t.Errorf("Provenance.Source = %v, want FORMULA", result.Provenance.Source)
	}
	if result.Provenance.Name != "weight_material_carbon" {
		t.Errorf("Provenance.Name = %q, want %q", result.Provenance.Name, "weight_material_carbon")
	}
	if result.Provenance.Version != 1 {
		t.Errorf("Provenance.Version = %d, want 1", result.Provenance.Version)
	}
	if result.Provenance.GetReasoning() != "calculated from weight and material" {
		t.Errorf("Provenance.Reasoning = %q, want %q", result.Provenance.GetReasoning(), "calculated from weight and material")
	}
	if len(result.Provenance.Sources) != 1 || result.Provenance.Sources[0] != "EPA data" {
		t.Errorf("Provenance.Sources = %v, want [EPA data]", result.Provenance.Sources)
	}
}

func TestMergeValueSources_FullMerge(t *testing.T) {
	cfg := loadTestConfig(t)

	// Estimator output: MoneySavings with formula provenance.
	ms := &api.MoneySavings{
		ValueUsd: &api.Estimate{Mean: 100, Stddev: 30},
		Provenance: &api.Provenance{
			Source:    api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
			Name:      "prevented_purchase",
			Version:   cfg.ProvenanceVersion("prevented_purchase"),
			Reasoning: proto.String("50% prevented purchase rate"),
			Sources:   []string{"estimator config"},
		},
	}

	// AI-generated ValueEstimate with its own provenance.
	ve := &models.ValueEstimate{
		EstimatedValueUsd: 200,
		Provenance: &models.Provenance{
			Source:    models.ProvenanceSource_PROVENANCE_SOURCE_LLM,
			Reasoning: proto.String("based on Amazon listing"),
			Sources:   []string{"amazon.com/dp/B123"},
		},
	}

	result := mergeValueSources(ms, ve)

	// Source, name, version should come from the estimator (not the AI).
	if result.Provenance.Source != api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA {
		t.Errorf("Provenance.Source = %v, want FORMULA", result.Provenance.Source)
	}
	if result.Provenance.Name != "prevented_purchase" {
		t.Errorf("Provenance.Name = %q, want %q", result.Provenance.Name, "prevented_purchase")
	}
	if result.Provenance.Version != cfg.ProvenanceVersion("prevented_purchase") {
		t.Errorf("Provenance.Version = %d, want %d", result.Provenance.Version, cfg.ProvenanceVersion("prevented_purchase"))
	}

	// Reasoning should be merged: AI reasoning prepended.
	wantReasoning := "based on Amazon listing. 50% prevented purchase rate"
	if result.Provenance.GetReasoning() != wantReasoning {
		t.Errorf("Provenance.Reasoning = %q, want %q", result.Provenance.GetReasoning(), wantReasoning)
	}

	// Sources should be merged: AI sources first, then estimator sources.
	if len(result.Provenance.Sources) != 2 {
		t.Fatalf("Provenance.Sources len = %d, want 2", len(result.Provenance.Sources))
	}
	if result.Provenance.Sources[0] != "amazon.com/dp/B123" {
		t.Errorf("Provenance.Sources[0] = %q, want %q", result.Provenance.Sources[0], "amazon.com/dp/B123")
	}
	if result.Provenance.Sources[1] != "estimator config" {
		t.Errorf("Provenance.Sources[1] = %q, want %q", result.Provenance.Sources[1], "estimator config")
	}
}

func TestMergeValueSources_NilValueEstimate(t *testing.T) {
	ms := &api.MoneySavings{
		ValueUsd: &api.Estimate{Mean: 100, Stddev: 30},
		Provenance: &api.Provenance{
			Source: api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
			Name:   "prevented_purchase",
		},
	}
	result := mergeValueSources(ms, nil)
	if result != ms {
		t.Error("mergeValueSources with nil VE should return original")
	}
}

func TestMergeValueSources_NilMoneySavings(t *testing.T) {
	ve := &models.ValueEstimate{EstimatedValueUsd: 200}
	result := mergeValueSources(nil, ve)
	if result != nil {
		t.Error("mergeValueSources with nil MS should return nil")
	}
}

func TestBuildTransferImpactMetrics_Loan_AllProvenance(t *testing.T) {
	cfg := loadTestConfig(t)

	gear := &models.Gear{
		ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 200},
		EmbodiedCarbon: &models.CarbonEstimate{
			Co2EGrams: &models.Estimate{Mean: 10000, Stddev: 3000},
			Provenance: &models.Provenance{
				Source:  models.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:    "weight_material_carbon",
				Version: cfg.ProvenanceVersion("weight_material_carbon"),
			},
		},
		WeightGrams: &models.TrackedEstimate{Value: &models.Estimate{Mean: 2000, Stddev: 500}},
	}

	ie := BuildTransferImpactMetrics(gear, models.TransferType_TRANSFER_TYPE_LOAN, cfg, nil, nil)

	// All three dimensions should have provenance with version > 0.
	assertProvenance(t, "MoneySaved", ie.MoneySaved.GetProvenance(),
		api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA, "prevented_purchase", cfg)
	assertProvenance(t, "EmissionsPrevented", ie.EmissionsPrevented.GetProvenance(),
		api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA, "weight_material_carbon", cfg)
	assertProvenance(t, "TimeSaved", ie.TimeSaved.GetProvenance(),
		api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT, "gear_shopping_time", cfg)

	// EmissionsPrevented sub-components should also carry provenance.
	if ie.EmissionsPrevented.ManufactureAvoidedCarbon != nil {
		mac := ie.EmissionsPrevented.ManufactureAvoidedCarbon
		if mac.Provenance == nil || mac.Provenance.Name == "" {
			t.Error("ManufactureAvoidedCarbon should have provenance")
		}
	}
}

func TestBuildRequestImpactMetrics_Provenance(t *testing.T) {
	cfg := loadTestConfig(t)

	ie := BuildRequestImpactMetrics(75.0, cfg, nil, nil)

	// MoneySaved: service_value provenance.
	assertProvenance(t, "MoneySaved", ie.MoneySaved.GetProvenance(),
		api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA, "service_value", cfg)

	// EmissionsPrevented: request emissions use config default carbon.
	if ie.EmissionsPrevented != nil && ie.EmissionsPrevented.Provenance != nil {
		if ie.EmissionsPrevented.Provenance.Version <= 0 {
			t.Errorf("EmissionsPrevented.Provenance.Version = %d, want > 0", ie.EmissionsPrevented.Provenance.Version)
		}
	}

	// TimeSaved: request_labor_time provenance.
	assertProvenance(t, "TimeSaved", ie.TimeSaved.GetProvenance(),
		api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT, "request_labor_time", cfg)
}

func TestBuildExperienceImpactMetrics_Provenance(t *testing.T) {
	cfg := loadTestConfig(t)

	ie := BuildExperienceImpactMetrics(50.0, 5, cfg, nil, nil)

	// MoneySaved: commercial_value provenance.
	assertProvenance(t, "MoneySaved", ie.MoneySaved.GetProvenance(),
		api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA, "commercial_value", cfg)

	// TimeSaved: experience_duration provenance.
	assertProvenance(t, "TimeSaved", ie.TimeSaved.GetProvenance(),
		api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT, "experience_duration", cfg)
}

func TestBuildGearCumulativeImpactMetrics_Provenance(t *testing.T) {
	cfg := loadTestConfig(t)

	gear := &models.Gear{
		ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 200},
		EmbodiedCarbon: &models.CarbonEstimate{
			Co2EGrams: &models.Estimate{Mean: 10000, Stddev: 3000},
			Provenance: &models.Provenance{
				Source:  models.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:    "weight_material_carbon",
				Version: cfg.ProvenanceVersion("weight_material_carbon"),
			},
		},
		WeightGrams: &models.TrackedEstimate{Value: &models.Estimate{Mean: 2000, Stddev: 500}},
	}

	ie := BuildGearCumulativeImpactMetrics(gear, 3, cfg)

	// Provenance should survive scaling.
	assertProvenance(t, "MoneySaved", ie.MoneySaved.GetProvenance(),
		api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA, "prevented_purchase", cfg)
	assertProvenance(t, "EmissionsPrevented", ie.EmissionsPrevented.GetProvenance(),
		api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA, "weight_material_carbon", cfg)
	assertProvenance(t, "TimeSaved", ie.TimeSaved.GetProvenance(),
		api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT, "gear_shopping_time", cfg)
}

// assertProvenance verifies that a Provenance has the expected source, name, and a version > 0
// matching the config value.
func assertProvenance(t *testing.T, label string, p *api.Provenance,
	wantSource api.ProvenanceSource, wantName string, cfg *estimator.Config,
) {
	t.Helper()
	if p == nil {
		t.Fatalf("%s: Provenance is nil", label)
	}
	if p.Source != wantSource {
		t.Errorf("%s: Provenance.Source = %v, want %v", label, p.Source, wantSource)
	}
	if p.Name != wantName {
		t.Errorf("%s: Provenance.Name = %q, want %q", label, p.Name, wantName)
	}
	wantVersion := cfg.ProvenanceVersion(wantName)
	if wantVersion <= 0 {
		t.Errorf("%s: config has no version for %q", label, wantName)
	}
	if p.Version != wantVersion {
		t.Errorf("%s: Provenance.Version = %d, want %d", label, p.Version, wantVersion)
	}
}
