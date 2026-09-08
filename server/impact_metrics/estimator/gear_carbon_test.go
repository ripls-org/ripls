package estimator

import (
	"context"
	"math"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestEstimateGearCarbonMaterialWeight(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	got := EstimateGearCarbon(
		context.Background(),
		models.MaterialCategory_MATERIAL_CATEGORY_CORDLESS_POWER_TOOL,
		2100, // 2.1 kg
		150,  // $150 (should be ignored when material path succeeds)
		"tools_hardware",
		cfg,
		nil,
	)
	if got == nil {
		t.Fatal("expected non-nil CarbonEstimate for material+weight path")
	}
	if got.Provenance == nil || got.Provenance.Name != "weight_material_carbon" {
		t.Errorf("provenance.name = %v, want weight_material_carbon", got.Provenance.GetName())
	}

	// 2100g * 6.17 kg_co2e_per_kg = 12957 grams CO2e.
	wantMean := float32(2100 * 6.17)
	if math.Abs(float64(got.Co2EGrams.Mean-wantMean)) > 1.0 {
		t.Errorf("co2e_grams.mean = %f, want ~%f", got.Co2EGrams.Mean, wantMean)
	}
	if got.Co2EGrams.Stddev <= 0 {
		t.Error("expected positive stddev")
	}
}

func TestEstimateGearCarbonSpendFallback(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	// No material/weight → falls back to spend-based.
	got := EstimateGearCarbon(
		context.Background(),
		models.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED,
		0,
		150,
		"tools_hardware",
		cfg,
		nil,
	)
	if got == nil {
		t.Fatal("expected non-nil CarbonEstimate for spend-based fallback")
	}
	if got.Provenance == nil || got.Provenance.Name != "spend_based_carbon" {
		t.Errorf("provenance.name = %v, want spend_based_carbon", got.Provenance.GetName())
	}

	// $150 * 0.12 kg_co2e_per_usd * 1000 = 18000 grams CO2e.
	wantMean := float32(150 * 0.12 * 1000)
	if math.Abs(float64(got.Co2EGrams.Mean-wantMean)) > 1.0 {
		t.Errorf("co2e_grams.mean = %f, want ~%f", got.Co2EGrams.Mean, wantMean)
	}
}

func TestEstimateGearCarbonNeitherPath(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	// No material, no weight, no value → nil.
	got := EstimateGearCarbon(
		context.Background(),
		models.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED,
		0,
		0,
		"",
		cfg,
		nil,
	)
	if got != nil {
		t.Errorf("expected nil when neither path has data, got %v", got)
	}
}

func TestEstimateGearCarbonMaterialPreferredOverSpend(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	// Both paths available → material should win.
	got := EstimateGearCarbon(
		context.Background(),
		models.MaterialCategory_MATERIAL_CATEGORY_WOOD,
		5000, // 5 kg
		200,
		"tools_hardware",
		cfg,
		nil,
	)
	if got == nil {
		t.Fatal("expected non-nil CarbonEstimate")
	}
	if got.Provenance == nil || got.Provenance.Name != "weight_material_carbon" {
		t.Errorf("provenance.name = %v, want weight_material_carbon (should prefer material over spend)", got.Provenance.GetName())
	}
}
