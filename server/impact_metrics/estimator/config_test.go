package estimator

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestLoadConfigFromEmbed(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}
	if cfg.GetVersion() == "" {
		t.Error("expected non-empty version")
	}
}

func TestLoadConfig(t *testing.T) {
	_, err := LoadConfig("config.textproto")
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := LoadConfig("nonexistent.textproto")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadConfigInvalidTextproto(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.textproto")
	if err := os.WriteFile(path, []byte(":::invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Error("expected error for invalid textproto")
	}
}

func TestAllMaterialFactorsPopulated(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	// Every non-UNSPECIFIED MaterialCategory should have a factor.
	for val, name := range models.MaterialCategory_name {
		cat := models.MaterialCategory(val)
		if cat == models.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED {
			continue
		}
		factor := cfg.MaterialFactor(cat)
		if factor == nil {
			t.Errorf("missing material factor for %s", name)
			continue
		}
		if factor.GetKgCo2EPerKg() <= 0 {
			t.Errorf("material %s has invalid factor: %f", name, factor.GetKgCo2EPerKg())
		}
	}
}

func TestAllCarbonMethodStddevsPopulated(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	// Every configured method version should have a valid stddev.
	for _, entry := range cfg.CarbonMethodStddevs {
		stddev := cfg.MethodStddev(entry.Method)
		if stddev <= 0 || stddev > 1 {
			t.Errorf("MethodStddev(%s) = %f, want (0, 1]", entry.Method, stddev)
		}
	}
}

func TestMaterialFactorUnspecifiedReturnsNil(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}
	if cfg.MaterialFactor(models.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED) != nil {
		t.Error("expected nil for unspecified material category")
	}
}

func TestMethodStddevUnknownReturnsDefault(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}
	got := cfg.MethodStddev("nonexistent_method")
	if got != 0.5 {
		t.Errorf("MethodStddev(nonexistent) = %f, want 0.5 (default)", got)
	}
}

func TestValidationRejectsEmptyVersion(t *testing.T) {
	cfg := validTestConfig()
	cfg.Version = ""
	if err := cfg.validate(); err == nil {
		t.Error("expected validation error for empty version")
	}
}

func TestValidationRejectsInvalidPreventedPurchaseRate(t *testing.T) {
	for _, rate := range []float32{0, -0.1, 1.1} {
		cfg := validTestConfig()
		cfg.Global.LoanPreventedPurchaseRate = rate
		if err := cfg.validate(); err == nil {
			t.Errorf("expected validation error for loan_prevented_purchase_rate=%f", rate)
		}
	}
	for _, rate := range []float32{0, -0.1, 1.1} {
		cfg := validTestConfig()
		cfg.Global.GiveawayPreventedPurchaseRate = rate
		if err := cfg.validate(); err == nil {
			t.Errorf("expected validation error for giveaway_prevented_purchase_rate=%f", rate)
		}
	}
}

func TestValidationRejectsInvalidMethodStddev(t *testing.T) {
	cfg := validTestConfig()
	cfg.EstimatorConfig.CarbonMethodStddevs[0].RelativeStddev = 0
	if err := cfg.validate(); err == nil {
		t.Error("expected validation error for zero method stddev")
	}

	cfg = validTestConfig()
	cfg.EstimatorConfig.CarbonMethodStddevs[0].RelativeStddev = 1.5
	if err := cfg.validate(); err == nil {
		t.Error("expected validation error for method stddev > 1")
	}
}

func TestValidationRejectsZeroMaterialFactor(t *testing.T) {
	cfg := validTestConfig()
	cfg.EstimatorConfig.MaterialEmissionFactors[0].KgCo2EPerKg = 0
	if err := cfg.validate(); err == nil {
		t.Error("expected validation error for zero material factor")
	}
}

func TestValidationRejectsEmptyMaterialFactors(t *testing.T) {
	cfg := validTestConfig()
	cfg.MaterialEmissionFactors = nil
	if err := cfg.validate(); err == nil {
		t.Error("expected validation error for empty material factors")
	}
}

func TestValidationRejectsZeroSpendFactor(t *testing.T) {
	cfg := validTestConfig()
	cfg.SpendBasedEmissionFactors["test"] = 0
	if err := cfg.validate(); err == nil {
		t.Error("expected validation error for zero spend factor")
	}
}

func TestValidationRejectsInvalidTimeDefaults(t *testing.T) {
	cfg := validTestConfig()
	cfg.TimeDefaults.GearShoppingTimeMinutes = 0
	if err := cfg.validate(); err == nil {
		t.Error("expected validation error for zero gear_shopping_time_minutes")
	}
}

func TestValidationRejectsInvalidEquivalency(t *testing.T) {
	cfg := validTestConfig()
	cfg.Equivalencies.MilesDrivenPerKgCo2E = 0
	if err := cfg.validate(); err == nil {
		t.Error("expected validation error for zero miles_driven_per_kg_co2e")
	}
}

func TestValidationRejectsInvalidLLMConfidence(t *testing.T) {
	cfg := validTestConfig()
	cfg.LlmConfidenceBreakpoints = []*models.LlmConfidenceBreakpoint{
		{Confidence: 1.5, RelativeStddev: 0.3},
	}
	if err := cfg.validate(); err == nil {
		t.Error("expected validation error for confidence > 1")
	}
}

func TestValidationRejectsZeroLLMStddevValue(t *testing.T) {
	cfg := validTestConfig()
	cfg.LlmConfidenceBreakpoints = []*models.LlmConfidenceBreakpoint{
		{Confidence: 0.5, RelativeStddev: 0},
	}
	if err := cfg.validate(); err == nil {
		t.Error("expected validation error for zero LLM stddev value")
	}
}

func TestLLMConfidenceInterpolation(t *testing.T) {
	cfg := validTestConfig()
	// Breakpoints: 0.0 -> 1.0, 0.5 -> 0.5, 1.0 -> 0.2
	cfg.LlmConfidenceBreakpoints = []*models.LlmConfidenceBreakpoint{
		{Confidence: 0.0, RelativeStddev: 1.0},
		{Confidence: 0.5, RelativeStddev: 0.5},
		{Confidence: 1.0, RelativeStddev: 0.2},
	}
	cfg.buildIndexes()

	tests := []struct {
		confidence float32
		want       float32
	}{
		{0.0, 1.0},
		{0.5, 0.5},
		{1.0, 0.2},
		{0.25, 0.75}, // Midpoint of [0.0->1.0, 0.5->0.5].
		{0.75, 0.35}, // Midpoint of [0.5->0.5, 1.0->0.2].
		{-0.5, 1.0},  // Clamped to 0.0.
		{1.5, 0.2},   // Clamped to 1.0.
	}
	for _, tt := range tests {
		got := cfg.LLMConfidenceToRelativeStddev(tt.confidence)
		if math.Abs(float64(got-tt.want)) > 0.01 {
			t.Errorf("LLMConfidenceToRelativeStddev(%f) = %f, want %f", tt.confidence, got, tt.want)
		}
	}
}

func TestProvenanceVersionKnownName(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	names := []string{
		"prevented_purchase",
		"service_value",
		"commercial_value",
		"weight_material_carbon",
		"spend_based_carbon",
		"gear_shopping_time",
		"request_labor_time",
		"experience_duration",
		"genai_time_estimate",
		"genai_from_text",
		"genai_from_image",
		"genai_from_web",
		"genai_value_estimate",
	}
	for _, name := range names {
		v := cfg.ProvenanceVersion(name)
		if v <= 0 {
			t.Errorf("ProvenanceVersion(%q) = %d, want > 0", name, v)
		}
	}
}

func TestProvenanceVersionUnknownReturnsZero(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}
	if v := cfg.ProvenanceVersion("nonexistent"); v != 0 {
		t.Errorf("ProvenanceVersion(nonexistent) = %d, want 0", v)
	}
}

// validTestConfig returns a minimal valid Config for testing validation logic.
func validTestConfig() *Config {
	pb := &models.EstimatorConfig{
		Version: "1.0.0",
		Global: &models.EstimatorGlobalConfig{
			LoanPreventedPurchaseRate:     0.50,
			GiveawayPreventedPurchaseRate: 0.50,
			RepairCarbonFraction:          0.35,
			TransportationCo2EPerMileKg:   0.000393,
			MoneySavedRelativeStddev:      0.30,
			WasteCo2EPerKg:                1.0,
			RequestDefaultCarbonGrams:     5000,
		},
		CarbonMethodStddevs: []*models.CarbonMethodStddev{
			{Method: "product_lca_v1", RelativeStddev: 0.15},
		},
		LlmConfidenceBreakpoints: []*models.LlmConfidenceBreakpoint{
			{Confidence: 1.0, RelativeStddev: 0.20},
			{Confidence: 0.0, RelativeStddev: 1.00},
		},
		MaterialEmissionFactors: []*models.MaterialEmissionFactorEntry{
			{Category: models.MaterialCategory_MATERIAL_CATEGORY_SOLID_METAL, KgCo2EPerKg: 3.25},
		},
		SpendBasedEmissionFactors: map[string]float32{
			"general_consumer_goods": 0.10,
		},
		TimeDefaults: &models.EstimatorTimeDefaults{
			GearShoppingTimeMinutes:   120,
			RequestLaborTimeMinutes:   120,
			ExperienceDurationMinutes: 120,
			DefaultRelativeStddev:     0.50,
			HintRelativeStddev:        0.30,
		},
		Equivalencies: &models.EstimatorEquivalencies{
			MilesDrivenPerKgCo2E:           2.544,
			SmartphoneChargesPerKgCo2E:     80.645,
			TreeSeedlings_10YrPerTonneCo2E: 16.5,
		},
	}

	cfg := &Config{EstimatorConfig: pb}
	cfg.buildIndexes()
	return cfg
}
