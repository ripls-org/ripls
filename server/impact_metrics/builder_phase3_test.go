package impact_metrics

import (
	"testing"
)

// TestPhase3_EffortAsymmetry verifies that when contributors are a minority of
// attendees, the quality time minutes are scaled up by the asymmetry factor.
func TestPhase3_EffortAsymmetry(t *testing.T) {
	cfg := loadTestConfig(t)

	// Baseline: no contributors.
	baseHint := &QualityTimeHint{DurationMinutes: 60}
	base := BuildExperienceImpactMetrics(0, 10, cfg, nil, baseHint)

	// With 2 contributors out of 10 attendees — high asymmetry.
	asymHint := &QualityTimeHint{DurationMinutes: 60, EffortContributorCount: 2}
	asym := BuildExperienceImpactMetrics(0, 10, cfg, nil, asymHint)

	baseQT := base.GetQualityTime().GetQualityTimeMinutes().GetMean()
	asymQT := asym.GetQualityTime().GetQualityTimeMinutes().GetMean()

	if asymQT <= baseQT {
		t.Errorf("effort-asymmetry bonus not applied: asymQT %.2f <= baseQT %.2f", asymQT, baseQT)
	}

	maxFactor := cfg.QualityTime.GetQualityTimeMaxEffortAsymmetryFactor()
	if asymQT > baseQT*maxFactor+0.01 {
		t.Errorf("effort-asymmetry exceeds max factor: asymQT %.2f > baseQT %.2f × %.2f", asymQT, baseQT, maxFactor)
	}
}

// TestPhase3_EffortAsymmetry_NoScalingWhenAllContribute verifies that when
// contributors == attendees (everyone contributed), no bonus is applied.
func TestPhase3_EffortAsymmetry_NoScalingWhenAllContribute(t *testing.T) {
	cfg := loadTestConfig(t)

	hint := &QualityTimeHint{DurationMinutes: 60, EffortContributorCount: 5}
	base := BuildExperienceImpactMetrics(0, 5, cfg, nil, &QualityTimeHint{DurationMinutes: 60})
	withContribs := BuildExperienceImpactMetrics(0, 5, cfg, nil, hint)

	baseQT := base.GetQualityTime().GetQualityTimeMinutes().GetMean()
	withQT := withContribs.GetQualityTime().GetQualityTimeMinutes().GetMean()

	const tol = 0.001
	if withQT < baseQT-tol || withQT > baseQT+tol {
		t.Errorf("expected no asymmetry bonus when all attend: got %.4f, want ≈ %.4f", withQT, baseQT)
	}
}

// TestPhase3_CrossCommunityBonus verifies that the cross-community tie-strength
// multiplier increases the quality time minutes.
func TestPhase3_CrossCommunityBonus(t *testing.T) {
	cfg := loadTestConfig(t)

	base := BuildExperienceImpactMetrics(0, 4, cfg, nil, &QualityTimeHint{DurationMinutes: 60})
	withBonus := BuildExperienceImpactMetrics(0, 4, cfg, nil, &QualityTimeHint{
		DurationMinutes:            60,
		HasCrossCommunityAttendees: true,
	})

	baseQT := base.GetQualityTime().GetQualityTimeMinutes().GetMean()
	bonusQT := withBonus.GetQualityTime().GetQualityTimeMinutes().GetMean()

	if bonusQT <= baseQT {
		t.Errorf("cross-community bonus not applied: bonusQT %.2f <= baseQT %.2f", bonusQT, baseQT)
	}
}

// TestPhase3_MultiHostVulnerabilityBlend verifies that multi-host blending
// shifts the quality time compared to a single-host event.
func TestPhase3_MultiHostVulnerabilityBlend(t *testing.T) {
	cfg := loadTestConfig(t)

	singleHost := BuildExperienceImpactMetrics(0, 4, cfg, nil, &QualityTimeHint{
		DurationMinutes:    60,
		VulnerabilityLevel: "high",
	})
	multiHost := BuildExperienceImpactMetrics(0, 4, cfg, nil, &QualityTimeHint{
		DurationMinutes:        60,
		VulnerabilityLevel:     "high",
		IsMultiHost:            true,
		HostVulnerabilityLevel: 0.3, // lower secondary host vulnerability
	})

	// Multi-host blending toward a lower secondary host reduces the overall
	// vulnerability weight, which should reduce quality time minutes.
	singleQT := singleHost.GetQualityTime().GetQualityTimeMinutes().GetMean()
	multiQT := multiHost.GetQualityTime().GetQualityTimeMinutes().GetMean()

	blend := cfg.QualityTime.GetQualityTimeMultiHostVulnerabilityBlend()
	if blend > 0 && multiQT >= singleQT {
		t.Errorf("multi-host blend did not reduce QT: multi %.2f >= single %.2f", multiQT, singleQT)
	}
}
