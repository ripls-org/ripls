package impact_metrics

import (
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
)

// baseGearWithCarbon returns a Gear with carbon data for use in override tests.
func baseGearWithCarbon() *models.Gear {
	return &models.Gear{
		ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 200},
		EmbodiedCarbon: &models.CarbonEstimate{
			Co2EGrams: &models.Estimate{Mean: 8000, Stddev: 2000},
			Provenance: &models.Provenance{
				Source: models.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:   "weight_material_carbon",
			},
		},
		WeightGrams: &models.TrackedEstimate{Value: &models.Estimate{Mean: 1500}},
	}
}

// baseExperienceEstimate builds a realistic draft estimate for use in override tests.
func baseExperienceEstimate(t *testing.T, cfg *estimator.Config) *api.ImpactEstimate {
	t.Helper()
	hint := &QualityTimeHint{
		DurationMinutes:    90,
		VulnerabilityLevel: "high",
	}
	return BuildExperienceImpactMetrics(100, 8, cfg, nil, hint)
}

// TestApplyImpactOverrides_NilOverrides returns base unchanged.
func TestApplyImpactOverrides_NilOverrides(t *testing.T) {
	cfg := loadTestConfig(t)
	base := baseExperienceEstimate(t, cfg)

	result, err := ApplyImpactOverrides(base, nil, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != base {
		t.Error("expected same pointer when overrides is nil")
	}
}

// TestApplyImpactOverrides_QT_OverrideDuration_StampsUserOnDurationOnly verifies that
// when only EstimatedDurationMinutes is supplied, that attribute gets USER provenance
// while all other attribute provenances are preserved from the base estimate.
func TestApplyImpactOverrides_QT_OverrideDuration_StampsUserOnDurationOnly(t *testing.T) {
	cfg := loadTestConfig(t)
	base := baseExperienceEstimate(t, cfg)

	// Sanity-check: base should have LLM provenance on duration (from DurationMinutes hint).
	if base.QualityTime == nil || base.QualityTime.Attributes == nil {
		t.Fatal("base QualityTime or Attributes is nil")
	}
	baseDurProv := base.QualityTime.Attributes.EstimatedDurationMinutesProvenance
	if baseDurProv == nil || baseDurProv.Source != api.ProvenanceSource_PROVENANCE_SOURCE_LLM {
		t.Fatalf("expected base duration provenance = LLM, got %v", baseDurProv.GetSource())
	}

	overrides := &ImpactOverrides{
		QualityTime: &api.QualityTimeAttributes{
			EstimatedDurationMinutes: 45,
		},
	}

	result, err := ApplyImpactOverrides(base, overrides, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	attrs := result.QualityTime.GetAttributes()
	if attrs == nil {
		t.Fatal("result QualityTimeAttributes is nil")
	}

	// Duration was overridden → USER.
	if attrs.EstimatedDurationMinutesProvenance.GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_USER {
		t.Errorf("duration provenance = %v, want USER", attrs.EstimatedDurationMinutesProvenance.GetSource())
	}
	// Vulnerability was NOT overridden → should preserve base provenance (LLM, from the hint).
	if attrs.VulnerabilityProvenance.GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_LLM {
		t.Errorf("vulnerability provenance = %v, want LLM (preserved from base)", attrs.VulnerabilityProvenance.GetSource())
	}
	// Modality not overridden → FORMULA.
	if attrs.ModalityProvenance.GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA {
		t.Errorf("modality provenance = %v, want FORMULA (preserved from base)", attrs.ModalityProvenance.GetSource())
	}
}

// TestApplyImpactOverrides_QT_OverrideVulnerability_StampsUserOnVulnerabilityOnly mirrors
// the duration test for the vulnerability attribute.
func TestApplyImpactOverrides_QT_OverrideVulnerability_StampsUserOnVulnerabilityOnly(t *testing.T) {
	cfg := loadTestConfig(t)
	base := baseExperienceEstimate(t, cfg)

	overrides := &ImpactOverrides{
		QualityTime: &api.QualityTimeAttributes{
			Vulnerability: api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_LOW,
		},
	}

	result, err := ApplyImpactOverrides(base, overrides, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	attrs := result.QualityTime.GetAttributes()
	if attrs == nil {
		t.Fatal("result QualityTimeAttributes is nil")
	}

	if attrs.VulnerabilityProvenance.GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_USER {
		t.Errorf("vulnerability provenance = %v, want USER", attrs.VulnerabilityProvenance.GetSource())
	}
	// Duration not overridden → should carry LLM provenance from base.
	if attrs.EstimatedDurationMinutesProvenance.GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_LLM {
		t.Errorf("duration provenance = %v, want LLM (preserved)", attrs.EstimatedDurationMinutesProvenance.GetSource())
	}
}

// TestApplyImpactOverrides_MoneySavings_StampsUserProvenance verifies that a money
// savings override stamps USER provenance on the top-level provenance field.
func TestApplyImpactOverrides_MoneySavings_StampsUserProvenance(t *testing.T) {
	cfg := loadTestConfig(t)
	base := baseExperienceEstimate(t, cfg)

	overrides := &ImpactOverrides{
		MoneySavings: &api.MoneySavings{
			Inputs: &api.MoneySavingsInput{
				HireEquivalentValue: &api.Estimate{Mean: 75, Stddev: 10},
			},
		},
	}

	result, err := ApplyImpactOverrides(base, overrides, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.MoneySaved == nil {
		t.Fatal("MoneySaved is nil after override")
	}
	if result.MoneySaved.Provenance.GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_USER {
		t.Errorf("money provenance = %v, want USER", result.MoneySaved.Provenance.GetSource())
	}
	// Composite value_usd should be taken from hire_equivalent_value.
	if result.MoneySaved.ValueUsd == nil || result.MoneySaved.ValueUsd.Mean != 75 {
		t.Errorf("value_usd.mean = %v, want 75", result.MoneySaved.GetValueUsd().GetMean())
	}
}

// TestApplyImpactOverrides_Emissions_StampsUserProvenance verifies that an emissions
// override stamps USER provenance while preserving base composite fields.
func TestApplyImpactOverrides_Emissions_StampsUserProvenance(t *testing.T) {
	cfg := loadTestConfig(t)

	// Build a base with non-zero emissions (gear transfer provides embodied carbon).
	gear := baseGearWithCarbon()
	base := BuildTransferImpactMetrics(gear, 0 /* loan */, cfg, nil, nil)

	overrides := &ImpactOverrides{
		Emissions: &api.PreventedEmissions{
			Inputs: &api.PreventedEmissionsInput{
				TravelAvoidedCarbon: &api.Estimate{Mean: 500},
			},
		},
	}

	result, err := ApplyImpactOverrides(base, overrides, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.EmissionsPrevented == nil {
		t.Fatal("EmissionsPrevented is nil after override")
	}
	if result.EmissionsPrevented.Provenance.GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_USER {
		t.Errorf("emissions provenance = %v, want USER", result.EmissionsPrevented.Provenance.GetSource())
	}
}

// TestApplyImpactOverrides_QT_CompositeUnchangedByOverride confirms that the composite
// quality_time_minutes is recomputed by the server, not copied from any override.
func TestApplyImpactOverrides_QT_CompositeRecomputedNotCopied(t *testing.T) {
	cfg := loadTestConfig(t)
	base := baseExperienceEstimate(t, cfg)

	baseQTMins := base.QualityTime.GetQualityTimeMinutes().GetMean()

	// Override duration to double the base.
	baseDur := base.QualityTime.GetAttributes().GetEstimatedDurationMinutes()
	overrides := &ImpactOverrides{
		QualityTime: &api.QualityTimeAttributes{
			EstimatedDurationMinutes: baseDur * 2,
		},
	}

	result, err := ApplyImpactOverrides(base, overrides, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultQTMins := result.QualityTime.GetQualityTimeMinutes().GetMean()
	// With double the duration, QT minutes should be larger.
	if resultQTMins <= baseQTMins {
		t.Errorf("QT minutes after duration override = %.1f, want > %.1f", resultQTMins, baseQTMins)
	}
}

// TestBuildExperienceImpactMetrics_PerAttributeProvenance_ConfigDefault verifies that
// with no hint, all 7 QT attributes have CONFIG_DEFAULT or FORMULA provenance.
func TestBuildExperienceImpactMetrics_PerAttributeProvenance_ConfigDefault(t *testing.T) {
	cfg := loadTestConfig(t)
	ie := BuildExperienceImpactMetrics(50, 4, cfg, nil, nil)

	attrs := ie.GetQualityTime().GetAttributes()
	if attrs == nil {
		t.Fatal("QualityTimeAttributes is nil")
	}

	checks := []struct {
		name string
		prov *api.Provenance
		want api.ProvenanceSource
	}{
		{"duration", attrs.EstimatedDurationMinutesProvenance, api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT},
		{"modality", attrs.ModalityProvenance, api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA},
		{"group_size", attrs.GroupSizeProvenance, api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA},
		{"tie_strength", attrs.TieStrengthProvenance, api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA},
		{"reciprocity", attrs.ReciprocityProvenance, api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA},
		{"novelty", attrs.NoveltyProvenance, api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA},
		{"vulnerability", attrs.VulnerabilityProvenance, api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT},
	}
	for _, c := range checks {
		if c.prov == nil {
			t.Errorf("%s: provenance is nil", c.name)
			continue
		}
		if c.prov.Source != c.want {
			t.Errorf("%s: source = %v, want %v", c.name, c.prov.Source, c.want)
		}
	}
}

// TestBuildExperienceImpactMetrics_PerAttributeProvenance_LLMHint verifies that LLM-inferred
// duration and vulnerability carry LLM provenance.
func TestBuildExperienceImpactMetrics_PerAttributeProvenance_LLMHint(t *testing.T) {
	cfg := loadTestConfig(t)
	hint := &QualityTimeHint{
		DurationMinutes:    75,
		VulnerabilityLevel: "medium",
	}
	ie := BuildExperienceImpactMetrics(50, 4, cfg, nil, hint)
	attrs := ie.GetQualityTime().GetAttributes()
	if attrs == nil {
		t.Fatal("QualityTimeAttributes is nil")
	}
	if attrs.EstimatedDurationMinutesProvenance.GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_LLM {
		t.Errorf("duration provenance = %v, want LLM", attrs.EstimatedDurationMinutesProvenance.GetSource())
	}
	if attrs.VulnerabilityProvenance.GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_LLM {
		t.Errorf("vulnerability provenance = %v, want LLM", attrs.VulnerabilityProvenance.GetSource())
	}
	// Modality should still be FORMULA (derived from tx type, not LLM).
	if attrs.ModalityProvenance.GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA {
		t.Errorf("modality provenance = %v, want FORMULA", attrs.ModalityProvenance.GetSource())
	}
}

// TestBuildExperienceImpactMetrics_PerAttributeProvenance_UserDuration verifies that a
// host-set duration carries USER provenance.
func TestBuildExperienceImpactMetrics_PerAttributeProvenance_UserDuration(t *testing.T) {
	cfg := loadTestConfig(t)
	hint := &QualityTimeHint{UserDurationMinutes: 120}
	ie := BuildExperienceImpactMetrics(50, 4, cfg, nil, hint)
	attrs := ie.GetQualityTime().GetAttributes()
	if attrs == nil {
		t.Fatal("QualityTimeAttributes is nil")
	}
	if attrs.EstimatedDurationMinutesProvenance.GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_USER {
		t.Errorf("duration provenance = %v, want USER", attrs.EstimatedDurationMinutesProvenance.GetSource())
	}
}

// TestBuildExperienceImpactMetrics_PerAttributeProvenance_SocialContextOverride verifies
// that a user-set SocialContext field carries USER provenance on the overridden attribute.
func TestBuildExperienceImpactMetrics_PerAttributeProvenance_SocialContextOverride(t *testing.T) {
	cfg := loadTestConfig(t)
	hint := &QualityTimeHint{
		SocialContext: &api.SocialContext{
			TieStrength: api.SocialTieStrength_SOCIAL_TIE_STRENGTH_CLOSE,
		},
	}
	ie := BuildExperienceImpactMetrics(50, 4, cfg, nil, hint)
	attrs := ie.GetQualityTime().GetAttributes()
	if attrs == nil {
		t.Fatal("QualityTimeAttributes is nil")
	}
	if attrs.TieStrengthProvenance.GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_USER {
		t.Errorf("tie_strength provenance = %v, want USER", attrs.TieStrengthProvenance.GetSource())
	}
	// Unset attributes should still be FORMULA.
	if attrs.ModalityProvenance.GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA {
		t.Errorf("modality provenance = %v, want FORMULA", attrs.ModalityProvenance.GetSource())
	}
}
