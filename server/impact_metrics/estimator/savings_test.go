package estimator

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
)

func TestEstimateMoneySavedLoan(t *testing.T) {
	cfg := loadTestConfig(t)

	got := EstimateMoneySaved(100, TransactionLoan, cfg)
	if got == nil {
		t.Fatal("expected non-nil MoneySavings for loan")
	}
	if got.ValueUsd == nil || got.ValueUsd.Mean <= 0 {
		t.Error("expected positive mean for loan")
	}
	if got.ValueUsd.Mean >= 100 {
		t.Error("loan savings should be less than full value (purchase rate < 1)")
	}
	if got.ValueUsd.Stddev <= 0 {
		t.Error("expected positive stddev")
	}
	if got.Provenance == nil {
		t.Fatal("expected non-nil provenance")
	}
	if got.Provenance.GetReasoning() == "" {
		t.Error("expected non-empty reasoning")
	}
	if got.Provenance.Name != "prevented_purchase" {
		t.Errorf("provenance name = %q, want %q", got.Provenance.Name, "prevented_purchase")
	}
	if got.Provenance.Version != cfg.ProvenanceVersion("prevented_purchase") {
		t.Errorf("provenance version = %d, want %d", got.Provenance.Version, cfg.ProvenanceVersion("prevented_purchase"))
	}
	if len(got.Provenance.Sources) == 0 {
		t.Error("expected non-empty sources for loan")
	}
}

func TestEstimateMoneySavedGiveaway(t *testing.T) {
	cfg := loadTestConfig(t)

	got := EstimateMoneySaved(100, TransactionGiveaway, cfg)
	if got == nil {
		t.Fatal("expected non-nil MoneySavings for giveaway")
	}
	if got.ValueUsd == nil || got.ValueUsd.Mean <= 0 {
		t.Error("expected positive mean for giveaway")
	}
	if got.Provenance == nil {
		t.Fatal("expected non-nil provenance")
	}
	if got.Provenance.GetReasoning() == "" {
		t.Error("expected non-empty reasoning")
	}
	if got.Provenance.Name != "prevented_purchase" {
		t.Errorf("provenance name = %q, want %q", got.Provenance.Name, "prevented_purchase")
	}
	if got.Provenance.Version != cfg.ProvenanceVersion("prevented_purchase") {
		t.Errorf("provenance version = %d, want %d", got.Provenance.Version, cfg.ProvenanceVersion("prevented_purchase"))
	}
	if len(got.Provenance.Sources) == 0 {
		t.Error("expected non-empty sources for giveaway")
	}
}

func TestEstimateMoneySavedRequest(t *testing.T) {
	cfg := loadTestConfig(t)

	got := EstimateMoneySaved(75, TransactionRequestFulfilled, cfg)
	if got == nil {
		t.Fatal("expected non-nil MoneySavings for request")
	}
	// Requests pass through at 100%.
	if got.ValueUsd.Mean != 75 {
		t.Errorf("request mean = %f, want 75 (full value)", got.ValueUsd.Mean)
	}
	if got.Provenance == nil || got.Provenance.Name != "service_value" {
		t.Errorf("provenance name = %q, want %q", got.GetProvenance().GetName(), "service_value")
	}
	if got.Provenance.Version != cfg.ProvenanceVersion("service_value") {
		t.Errorf("provenance version = %d, want %d", got.Provenance.Version, cfg.ProvenanceVersion("service_value"))
	}
	if len(got.Provenance.Sources) == 0 {
		t.Error("expected non-empty sources for request")
	}
}

func TestEstimateMoneySavedExperience(t *testing.T) {
	cfg := loadTestConfig(t)

	got := EstimateMoneySaved(50, TransactionExperienceConcluded, cfg)
	if got == nil {
		t.Fatal("expected non-nil MoneySavings for experience")
	}
	if got.ValueUsd.Mean != 50 {
		t.Errorf("experience mean = %f, want 50 (full value)", got.ValueUsd.Mean)
	}
	if got.Provenance == nil || got.Provenance.Name != "commercial_value" {
		t.Errorf("provenance name = %q, want %q", got.GetProvenance().GetName(), "commercial_value")
	}
	if got.Provenance.Version != cfg.ProvenanceVersion("commercial_value") {
		t.Errorf("provenance version = %d, want %d", got.Provenance.Version, cfg.ProvenanceVersion("commercial_value"))
	}
	if len(got.GetProvenance().GetSources()) == 0 {
		t.Error("expected non-empty sources for experience")
	}
}

func TestEstimateMoneySavedZeroValue(t *testing.T) {
	cfg := loadTestConfig(t)

	if got := EstimateMoneySaved(0, TransactionLoan, cfg); got != nil {
		t.Error("expected nil for zero value")
	}
}

func TestEstimateEmissionsPreventedLoan(t *testing.T) {
	cfg := loadTestConfig(t)

	input := &EmissionsInput{
		EmbodiedCarbon: &api.CarbonEstimate{
			Co2EGrams: &api.Estimate{Mean: 12000, Stddev: 3600},
			Provenance: &api.Provenance{
				Source:  api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:    "weight_material_carbon",
				Version: cfg.ProvenanceVersion("weight_material_carbon"),
			},
		},
		WeightGrams: 2100,
	}

	got := EstimateEmissionsPrevented(input, TransactionLoan, cfg)
	if got == nil {
		t.Fatal("expected non-nil PreventedEmissions")
	}
	if got.ManufactureAvoidedCarbon == nil {
		t.Fatal("expected non-nil ManufactureAvoidedCarbon")
	}
	if got.ManufactureAvoidedCarbon.Co2EGrams.Mean <= 0 {
		t.Error("expected positive manufacture avoided carbon")
	}
	if got.ManufactureAvoidedCarbon.Co2EGrams.Mean >= 12000 {
		t.Error("manufacture avoided should be less than embodied (purchase rate < 1)")
	}
	if got.WasteReducedCarbon == nil {
		t.Fatal("expected non-nil WasteReducedCarbon")
	}
	if got.WasteReducedCarbon.Co2EGrams.Mean <= 0 {
		t.Error("expected positive waste reduced carbon")
	}
	if got.Provenance == nil {
		t.Fatal("expected non-nil provenance")
	}
	if got.Provenance.GetReasoning() == "" {
		t.Error("expected non-empty reasoning")
	}
	if got.Provenance.Name != "weight_material_carbon" {
		t.Errorf("provenance name = %q, want %q (derived from embodied carbon)", got.Provenance.Name, "weight_material_carbon")
	}
	if got.Provenance.Version != cfg.ProvenanceVersion("weight_material_carbon") {
		t.Errorf("provenance version = %d, want %d", got.Provenance.Version, cfg.ProvenanceVersion("weight_material_carbon"))
	}
	if len(got.Provenance.Sources) == 0 {
		t.Error("expected non-empty sources for gear emissions")
	}
}

func TestEstimateEmissionsPreventedGiveaway(t *testing.T) {
	cfg := loadTestConfig(t)

	input := &EmissionsInput{
		EmbodiedCarbon: &api.CarbonEstimate{
			Co2EGrams: &api.Estimate{Mean: 10000, Stddev: 3000},
			Provenance: &api.Provenance{
				Source:  api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:    "weight_material_carbon",
				Version: cfg.ProvenanceVersion("weight_material_carbon"),
			},
		},
		WeightGrams: 5000,
	}

	got := EstimateEmissionsPrevented(input, TransactionGiveaway, cfg)
	if got == nil {
		t.Fatal("expected non-nil PreventedEmissions for giveaway")
	}
	if got.ManufactureAvoidedCarbon == nil {
		t.Fatal("expected non-nil ManufactureAvoidedCarbon")
	}
	if got.WasteReducedCarbon == nil {
		t.Fatal("expected non-nil WasteReducedCarbon")
	}
}

func TestEstimateEmissionsPreventedLoanNoWeight(t *testing.T) {
	cfg := loadTestConfig(t)

	// Only embodied carbon, no weight → manufacture but no waste.
	input := &EmissionsInput{
		EmbodiedCarbon: &api.CarbonEstimate{
			Co2EGrams: &api.Estimate{Mean: 8000, Stddev: 2400},
			Provenance: &api.Provenance{
				Source:  api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:    "spend_based_carbon",
				Version: cfg.ProvenanceVersion("spend_based_carbon"),
			},
		},
		WeightGrams: 0,
	}

	got := EstimateEmissionsPrevented(input, TransactionLoan, cfg)
	if got == nil {
		t.Fatal("expected non-nil PreventedEmissions (manufacture only)")
	}
	if got.ManufactureAvoidedCarbon == nil {
		t.Fatal("expected non-nil ManufactureAvoidedCarbon")
	}
	if got.WasteReducedCarbon != nil {
		t.Error("expected nil WasteReducedCarbon when no weight")
	}
}

func TestEstimateEmissionsPreventedNilInput(t *testing.T) {
	cfg := loadTestConfig(t)

	if got := EstimateEmissionsPrevented(nil, TransactionLoan, cfg); got != nil {
		t.Error("expected nil for nil input on loan")
	}
}

func TestEstimateEmissionsPreventedRequest(t *testing.T) {
	cfg := loadTestConfig(t)

	// Requests use flat default; input is ignored.
	got := EstimateEmissionsPrevented(nil, TransactionRequestFulfilled, cfg)
	if got == nil {
		t.Fatal("expected non-nil PreventedEmissions from request flat default")
	}
	if got.ManufactureAvoidedCarbon == nil {
		t.Fatal("expected non-nil ManufactureAvoidedCarbon from flat default")
	}
	if got.ManufactureAvoidedCarbon.Co2EGrams.Mean <= 0 {
		t.Error("expected positive flat default carbon for request")
	}
	if got.Provenance == nil {
		t.Fatal("expected non-nil provenance")
	}
	if got.Provenance.GetReasoning() == "" {
		t.Error("expected non-empty reasoning")
	}
	if got.Provenance.Name != "spend_based_carbon" {
		t.Errorf("provenance name = %q, want %q", got.Provenance.Name, "spend_based_carbon")
	}
	if got.Provenance.Version != cfg.ProvenanceVersion("spend_based_carbon") {
		t.Errorf("provenance version = %d, want %d", got.Provenance.Version, cfg.ProvenanceVersion("spend_based_carbon"))
	}
	if len(got.Provenance.Sources) == 0 {
		t.Error("expected non-empty sources for request emissions")
	}
}

func TestEstimateEmissionsPreventedExperience(t *testing.T) {
	cfg := loadTestConfig(t)

	// experience_default_carbon_grams is 0, so should return nil.
	got := EstimateEmissionsPrevented(nil, TransactionExperienceConcluded, cfg)
	if cfg.Global.GetExperienceDefaultCarbonGrams() == 0 && got != nil {
		t.Error("expected nil when experience default carbon is 0")
	}
}

func TestScaleTimeSaved(t *testing.T) {
	ts := &api.TimeSavings{
		Minutes: &api.Estimate{Mean: 120, Stddev: 60},
		Provenance: &api.Provenance{
			Source:    api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT,
			Name:      "gear_shopping_time",
			Reasoning: proto.String("test reasoning"),
			Sources:   []string{"test source"},
		},
	}

	got := ScaleTimeSaved(ts, 1)
	if got == nil {
		t.Fatal("expected non-nil TimeSavings")
	}
	if got.Minutes.Mean != 120 {
		t.Errorf("mean = %f, want 120", got.Minutes.Mean)
	}
	if got.Provenance == nil {
		t.Fatal("expected non-nil provenance")
	}
	if got.Provenance.GetReasoning() != "test reasoning" {
		t.Errorf("reasoning = %q, want %q", got.Provenance.GetReasoning(), "test reasoning")
	}
	if got.Provenance.Source != api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT {
		t.Errorf("source = %v, want CONFIG_DEFAULT (should propagate)", got.Provenance.Source)
	}
	if len(got.Provenance.Sources) != 1 || got.Provenance.Sources[0] != "test source" {
		t.Errorf("sources = %v, want [test source] (should propagate)", got.Provenance.Sources)
	}
}

func TestScaleTimeSavedWithMultiplier(t *testing.T) {
	ts := &api.TimeSavings{
		Minutes: &api.Estimate{Mean: 60, Stddev: 30},
	}

	got := ScaleTimeSaved(ts, 3)
	if got == nil {
		t.Fatal("expected non-nil TimeSavings")
	}
	if got.Minutes.Mean != 180 {
		t.Errorf("mean = %f, want 180 (60 × 3)", got.Minutes.Mean)
	}
	if got.Minutes.Stddev != 90 {
		t.Errorf("stddev = %f, want 90 (30 × 3)", got.Minutes.Stddev)
	}
}

func TestScaleTimeSavedNil(t *testing.T) {
	if got := ScaleTimeSaved(nil, 1); got != nil {
		t.Error("expected nil for nil input")
	}
}

func TestScaleTimeSavedZeroMultiplier(t *testing.T) {
	ts := &api.TimeSavings{
		Minutes: &api.Estimate{Mean: 60, Stddev: 30},
	}

	// Zero multiplier should default to 1.
	got := ScaleTimeSaved(ts, 0)
	if got == nil {
		t.Fatal("expected non-nil TimeSavings")
	}
	if got.Minutes.Mean != 60 {
		t.Errorf("mean = %f, want 60 (zero multiplier defaults to 1)", got.Minutes.Mean)
	}
}

// loadTestConfig loads the embedded config for testing.
func loadTestConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}
	return cfg
}
