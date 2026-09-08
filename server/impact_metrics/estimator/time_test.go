package estimator

import (
	"math"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/api"
)

func TestEstimateGearTime(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	got := EstimateGearTime(cfg)
	if got == nil {
		t.Fatal("expected non-nil TimeSavings")
	}
	if got.Minutes == nil {
		t.Fatal("expected non-nil Minutes estimate")
	}

	wantMean := float32(120)
	if got.Minutes.Mean != wantMean {
		t.Errorf("mean = %f, want %f", got.Minutes.Mean, wantMean)
	}

	wantStddev := wantMean * cfg.TimeDefaults.GetDefaultRelativeStddev()
	if math.Abs(float64(got.Minutes.Stddev-wantStddev)) > 0.01 {
		t.Errorf("stddev = %f, want %f", got.Minutes.Stddev, wantStddev)
	}

	if got.Provenance == nil {
		t.Fatal("expected non-nil provenance")
	}
	if got.Provenance.Source != api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT {
		t.Errorf("source = %v, want CONFIG_DEFAULT", got.Provenance.Source)
	}
	if got.Provenance.Name != "gear_shopping_time" {
		t.Errorf("name = %q, want %q", got.Provenance.Name, "gear_shopping_time")
	}
	if got.Provenance.Version != cfg.ProvenanceVersion("gear_shopping_time") {
		t.Errorf("version = %d, want %d", got.Provenance.Version, cfg.ProvenanceVersion("gear_shopping_time"))
	}
	if len(got.Provenance.Sources) == 0 {
		t.Error("expected non-empty sources")
	}
}

func TestEstimateRequestTimeWithHint(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	got := EstimateRequestTime(90, cfg)
	if got == nil {
		t.Fatal("expected non-nil TimeSavings")
	}

	if got.Minutes.Mean != 90 {
		t.Errorf("mean = %f, want 90 (from hint)", got.Minutes.Mean)
	}

	wantStddev := float32(90) * cfg.TimeDefaults.GetHintRelativeStddev()
	if math.Abs(float64(got.Minutes.Stddev-wantStddev)) > 0.01 {
		t.Errorf("stddev = %f, want %f (hint uncertainty)", got.Minutes.Stddev, wantStddev)
	}

	if got.Provenance == nil {
		t.Fatal("expected non-nil provenance")
	}
	if got.Provenance.Source != api.ProvenanceSource_PROVENANCE_SOURCE_LLM {
		t.Errorf("source = %v, want LLM (hint path)", got.Provenance.Source)
	}
	if got.Provenance.Name != "genai_time_estimate" {
		t.Errorf("name = %q, want %q", got.Provenance.Name, "genai_time_estimate")
	}
	if got.Provenance.Version != cfg.ProvenanceVersion("genai_time_estimate") {
		t.Errorf("version = %d, want %d", got.Provenance.Version, cfg.ProvenanceVersion("genai_time_estimate"))
	}
	if len(got.Provenance.Sources) == 0 {
		t.Error("expected non-empty sources")
	}
}

func TestEstimateRequestTimeWithoutHint(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	got := EstimateRequestTime(0, cfg)
	if got == nil {
		t.Fatal("expected non-nil TimeSavings from config default")
	}

	wantMean := float32(120)
	if got.Minutes.Mean != wantMean {
		t.Errorf("mean = %f, want %f (config default)", got.Minutes.Mean, wantMean)
	}

	wantStddev := wantMean * cfg.TimeDefaults.GetDefaultRelativeStddev()
	if math.Abs(float64(got.Minutes.Stddev-wantStddev)) > 0.01 {
		t.Errorf("stddev = %f, want %f (default uncertainty)", got.Minutes.Stddev, wantStddev)
	}

	if got.Provenance == nil {
		t.Fatal("expected non-nil provenance")
	}
	if got.Provenance.Source != api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT {
		t.Errorf("source = %v, want CONFIG_DEFAULT (default path)", got.Provenance.Source)
	}
	if got.Provenance.Name != "request_labor_time" {
		t.Errorf("name = %q, want %q", got.Provenance.Name, "request_labor_time")
	}
	if got.Provenance.Version != cfg.ProvenanceVersion("request_labor_time") {
		t.Errorf("version = %d, want %d", got.Provenance.Version, cfg.ProvenanceVersion("request_labor_time"))
	}
}

func TestEstimateExperienceTimeWithHint(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	got := EstimateExperienceTime(60, cfg)
	if got == nil {
		t.Fatal("expected non-nil TimeSavings")
	}

	if got.Minutes.Mean != 60 {
		t.Errorf("mean = %f, want 60 (from hint)", got.Minutes.Mean)
	}

	if got.Provenance == nil {
		t.Fatal("expected non-nil provenance")
	}
	if got.Provenance.Source != api.ProvenanceSource_PROVENANCE_SOURCE_LLM {
		t.Errorf("source = %v, want LLM (hint path)", got.Provenance.Source)
	}
	if got.Provenance.Name != "genai_time_estimate" {
		t.Errorf("name = %q, want %q", got.Provenance.Name, "genai_time_estimate")
	}
	if got.Provenance.Version != cfg.ProvenanceVersion("genai_time_estimate") {
		t.Errorf("version = %d, want %d", got.Provenance.Version, cfg.ProvenanceVersion("genai_time_estimate"))
	}
}

func TestEstimateExperienceTimeWithoutHint(t *testing.T) {
	cfg, err := LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}

	got := EstimateExperienceTime(0, cfg)
	if got == nil {
		t.Fatal("expected non-nil TimeSavings from config default")
	}

	if got.Minutes.Mean != 120 {
		t.Errorf("mean = %f, want 120 (config default)", got.Minutes.Mean)
	}

	if got.Provenance == nil {
		t.Fatal("expected non-nil provenance")
	}
	if got.Provenance.Source != api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT {
		t.Errorf("source = %v, want CONFIG_DEFAULT (default path)", got.Provenance.Source)
	}
	if got.Provenance.Name != "experience_duration" {
		t.Errorf("name = %q, want %q", got.Provenance.Name, "experience_duration")
	}
	if got.Provenance.Version != cfg.ProvenanceVersion("experience_duration") {
		t.Errorf("version = %d, want %d", got.Provenance.Version, cfg.ProvenanceVersion("experience_duration"))
	}
}
