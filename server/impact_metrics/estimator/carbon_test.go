package estimator

import (
	"context"
	"math"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestLookupMaterialCarbon(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	// Cordless power tool, 2.1 kg: co2e_grams = 2100 * 6.17 = 12957
	got := LookupMaterialCarbon(
		models.MaterialCategory_MATERIAL_CATEGORY_CORDLESS_POWER_TOOL,
		2100,
		cfg,
	)
	if got == nil {
		t.Fatal("expected non-nil estimate for cordless power tool")
	}
	wantMean := float32(2100 * 6.17)
	if math.Abs(float64(got.Mean-wantMean)) > 1.0 {
		t.Errorf("mean = %f, want ~%f", got.Mean, wantMean)
	}
	// Stddev should be mean * weight_material relative stddev.
	wantStddev := wantMean * cfg.MethodStddev("weight_material_v1")
	if math.Abs(float64(got.Stddev-wantStddev)) > 1.0 {
		t.Errorf("stddev = %f, want ~%f", got.Stddev, wantStddev)
	}
}

func TestLookupMaterialCarbonZeroWeight(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}
	got := LookupMaterialCarbon(models.MaterialCategory_MATERIAL_CATEGORY_WOOD, 0, cfg)
	if got != nil {
		t.Error("expected nil for zero weight")
	}
}

func TestLookupMaterialCarbonUnspecified(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}
	got := LookupMaterialCarbon(models.MaterialCategory_MATERIAL_CATEGORY_UNSPECIFIED, 1000, cfg)
	if got != nil {
		t.Error("expected nil for unspecified material")
	}
}

func TestLookupSpendCarbon(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	// $150 tool: co2e_grams = 150 * 0.12 * 1000 = 18000
	got := LookupSpendCarbon(context.Background(), 150, "tools_hardware", cfg, nil)
	if got == nil {
		t.Fatal("expected non-nil estimate for tools_hardware")
	}
	wantMean := float32(150 * 0.12 * 1000)
	if math.Abs(float64(got.Mean-wantMean)) > 1.0 {
		t.Errorf("mean = %f, want ~%f", got.Mean, wantMean)
	}
}

func TestLookupSpendCarbonZeroValue(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}
	got := LookupSpendCarbon(context.Background(), 0, "tools_hardware", cfg, nil)
	if got != nil {
		t.Error("expected nil for zero value")
	}
}

func TestLookupSpendCarbonFallsBackToGeneral(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	// Unknown category should fall back to general_consumer_goods.
	got := LookupSpendCarbon(context.Background(), 100, "unknown_category", cfg, nil)
	if got == nil {
		t.Fatal("expected non-nil estimate with fallback")
	}
	wantMean := float32(100 * 0.10 * 1000) // general_consumer_goods = 0.10
	if math.Abs(float64(got.Mean-wantMean)) > 1.0 {
		t.Errorf("mean = %f, want ~%f (general_consumer_goods fallback)", got.Mean, wantMean)
	}
}

func TestLookupSpendCarbonEmptyCategory(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	// Empty category should use general_consumer_goods.
	got := LookupSpendCarbon(context.Background(), 100, "", cfg, nil)
	if got == nil {
		t.Fatal("expected non-nil estimate for empty category")
	}
}

func TestLookupSpendCarbonExactMatchWithSpaces(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	// "Tools Hardware" -> "tools_hardware" after normalization.
	got := LookupSpendCarbon(context.Background(), 100, "Tools Hardware", cfg, nil)
	if got == nil {
		t.Fatal("expected non-nil estimate for 'Tools Hardware'")
	}
	wantMean := float32(100 * 0.12 * 1000)
	if math.Abs(float64(got.Mean-wantMean)) > 1.0 {
		t.Errorf("mean = %f, want ~%f (tools_hardware)", got.Mean, wantMean)
	}
}

// mockMatcher returns the first candidate with a fixed score.
type mockMatcher struct {
	result string
	score  float32
}

func (m *mockMatcher) Match(_ context.Context, _ string, _ []string) (string, float32, error) {
	return m.result, m.score, nil
}

func TestLookupSpendCarbonUsesMatcher(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	matcher := &mockMatcher{result: "books_media", score: 0.9}
	got := LookupSpendCarbon(context.Background(), 100, "reading material", cfg, matcher)
	if got == nil {
		t.Fatal("expected non-nil estimate when matcher returns a match")
	}
	wantMean := float32(100 * 0.05 * 1000) // books_media = 0.05
	if math.Abs(float64(got.Mean-wantMean)) > 1.0 {
		t.Errorf("mean = %f, want ~%f (books_media via matcher)", got.Mean, wantMean)
	}
}
