package impact_metrics

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
)

// TestImpactEstimate_ProvenanceRoundTrip verifies that per-attribute provenance
// fields survive a proto marshal/unmarshal cycle with no loss of presence or value.
func TestImpactEstimate_ProvenanceRoundTrip(t *testing.T) {
	original := &api.ImpactEstimate{
		QualityTime: &api.QualityTimeEstimate{
			QualityTimeMinutes: &api.Estimate{Mean: 45, Stddev: 5},
			Attributes: &api.QualityTimeAttributes{
				EstimatedDurationMinutes: 60,
				EstimatedDurationMinutesProvenance: &api.Provenance{
					Source: api.ProvenanceSource_PROVENANCE_SOURCE_USER,
					Name:   "user_override",
				},
				GroupSize: 4,
				GroupSizeProvenance: &api.Provenance{
					Source: api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
					Name:   "llm_inference",
				},
				TieStrength: api.SocialTieStrength_SOCIAL_TIE_STRENGTH_ACTIVE,
				TieStrengthProvenance: &api.Provenance{
					Source: api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
					Name:   "prior_interactions",
				},
				Reciprocity: api.SocialReciprocity_SOCIAL_RECIPROCITY_MUTUAL,
				ReciprocityProvenance: &api.Provenance{
					Source: api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT,
					Name:   "default",
				},
				Novelty:       api.SocialNovelty_SOCIAL_NOVELTY_INFREQUENT,
				Vulnerability: api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_MEDIUM,
				Modality:      api.SocialModality_SOCIAL_MODALITY_IN_PERSON_SHARED,
				ModalityProvenance: &api.Provenance{
					Source: api.ProvenanceSource_PROVENANCE_SOURCE_USER,
					Name:   "user_selected",
				},
			},
		},
		MoneySaved: &api.MoneySavings{
			ValueUsd: &api.Estimate{Mean: 42.5},
			Provenance: &api.Provenance{
				Source: api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:   "hire_equivalent",
			},
		},
		EmissionsPrevented: &api.PreventedEmissions{
			ManufactureAvoidedCarbon: &api.CarbonEstimate{
				Co2EGrams: &api.Estimate{Mean: 3200},
				Provenance: &api.Provenance{
					Source: api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
					Name:   "embodied_carbon_rate",
				},
			},
			Provenance: &api.Provenance{
				Source: api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
				Name:   "emissions_composite",
			},
		},
	}

	// Marshal to bytes.
	b, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}

	// Unmarshal back.
	got := &api.ImpactEstimate{}
	if err := proto.Unmarshal(b, got); err != nil {
		t.Fatalf("proto.Unmarshal: %v", err)
	}

	// Verify QualityTime attributes round-trip.
	attrs := got.GetQualityTime().GetAttributes()
	if attrs == nil {
		t.Fatal("QualityTime.Attributes is nil after round-trip")
	}

	if attrs.GetEstimatedDurationMinutes() != 60 {
		t.Errorf("EstimatedDurationMinutes = %v, want 60", attrs.GetEstimatedDurationMinutes())
	}
	if attrs.EstimatedDurationMinutesProvenance == nil {
		t.Error("EstimatedDurationMinutesProvenance presence lost after round-trip")
	} else if attrs.EstimatedDurationMinutesProvenance.GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_USER {
		t.Errorf("EstimatedDurationMinutesProvenance.Source = %v, want USER",
			attrs.EstimatedDurationMinutesProvenance.GetSource())
	}

	if attrs.GroupSizeProvenance == nil {
		t.Error("GroupSizeProvenance presence lost after round-trip")
	} else if attrs.GroupSizeProvenance.GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_LLM {
		t.Errorf("GroupSizeProvenance.Source = %v, want LLM", attrs.GroupSizeProvenance.GetSource())
	}

	if attrs.TieStrengthProvenance == nil {
		t.Error("TieStrengthProvenance presence lost after round-trip")
	}
	if attrs.GetTieStrength() != api.SocialTieStrength_SOCIAL_TIE_STRENGTH_ACTIVE {
		t.Errorf("TieStrength = %v, want ACTIVE", attrs.GetTieStrength())
	}

	if attrs.ReciprocityProvenance == nil {
		t.Error("ReciprocityProvenance presence lost after round-trip")
	}
	if attrs.ModalityProvenance == nil {
		t.Error("ModalityProvenance presence lost after round-trip")
	} else if attrs.ModalityProvenance.GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_USER {
		t.Errorf("ModalityProvenance.Source = %v, want USER", attrs.ModalityProvenance.GetSource())
	}

	// Verify MoneySaved provenance round-trip.
	if got.GetMoneySaved() == nil {
		t.Fatal("MoneySaved is nil after round-trip")
	}
	if got.GetMoneySaved().GetProvenance().GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA {
		t.Errorf("MoneySaved.Provenance.Source = %v, want FORMULA",
			got.GetMoneySaved().GetProvenance().GetSource())
	}

	// Verify EmissionsPrevented provenance round-trip.
	if got.GetEmissionsPrevented() == nil {
		t.Fatal("EmissionsPrevented is nil after round-trip")
	}
	ep := got.GetEmissionsPrevented()
	if ep.GetManufactureAvoidedCarbon().GetCo2EGrams().GetMean() != 3200 {
		t.Errorf("ManufactureAvoidedCarbon.Co2eGrams.Mean = %v, want 3200",
			ep.GetManufactureAvoidedCarbon().GetCo2EGrams().GetMean())
	}
}

// TestImpactEstimate_OptionalProvenanceAbsence verifies that unset optional
// provenance fields correctly report as absent (not as zero-value present).
func TestImpactEstimate_OptionalProvenanceAbsence(t *testing.T) {
	// Build an estimate with only some provenance fields set.
	est := &api.ImpactEstimate{
		QualityTime: &api.QualityTimeEstimate{
			Attributes: &api.QualityTimeAttributes{
				EstimatedDurationMinutes: 30,
				// Deliberately omit EstimatedDurationMinutesProvenance.
				TieStrength: api.SocialTieStrength_SOCIAL_TIE_STRENGTH_NEW,
				TieStrengthProvenance: &api.Provenance{
					Source: api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT,
				},
			},
		},
	}

	b, err := proto.Marshal(est)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}

	got := &api.ImpactEstimate{}
	if err := proto.Unmarshal(b, got); err != nil {
		t.Fatalf("proto.Unmarshal: %v", err)
	}

	attrs := got.GetQualityTime().GetAttributes()

	// Duration has no provenance set — pointer should be nil.
	if attrs.EstimatedDurationMinutesProvenance != nil {
		t.Error("EstimatedDurationMinutesProvenance should be nil but is non-nil")
	}

	// TieStrength provenance is set — pointer should be non-nil.
	if attrs.TieStrengthProvenance == nil {
		t.Error("TieStrengthProvenance should be non-nil but is nil")
	} else if attrs.TieStrengthProvenance.GetSource() != api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT {
		t.Errorf("TieStrengthProvenance.Source = %v, want CONFIG_DEFAULT",
			attrs.TieStrengthProvenance.GetSource())
	}

	// GroupSize provenance was not set — pointer should be nil.
	if attrs.GroupSizeProvenance != nil {
		t.Error("GroupSizeProvenance should be nil but is non-nil")
	}
}
