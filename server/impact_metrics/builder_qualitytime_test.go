package impact_metrics

import (
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
)

func TestBuildTransferImpactMetrics_WithConnCtx_QTPresent(t *testing.T) {
	cfg := loadTestConfig(t)

	gear := &models.Gear{
		ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 100},
	}
	connCtx := &api.ConnectionContext{
		PriorInteractionCount:    2,
		MutualConnectionCount:    3,
		DistinctContactsThisWeek: 5,
	}

	ie := BuildTransferImpactMetrics(gear, models.TransferType_TRANSFER_TYPE_LOAN, cfg, connCtx, nil)

	if ie == nil {
		t.Fatal("BuildTransferImpactMetrics returned nil")
	}
	if ie.QualityTime == nil {
		t.Fatal("QualityTime is nil with connCtx set")
	}
	if ie.QualityTime.QualityTimeMinutes == nil {
		t.Fatal("QualityTime.QualityTimeMinutes is nil")
	}
	if ie.QualityTime.QualityTimeMinutes.Mean <= 0 {
		t.Errorf("QualityTimeMinutes.Mean = %v, want > 0", ie.QualityTime.QualityTimeMinutes.Mean)
	}
}

func TestBuildTransferImpactMetrics_NilConnCtx_QTPresent(t *testing.T) {
	cfg := loadTestConfig(t)

	gear := &models.Gear{
		ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 100},
	}

	ie := BuildTransferImpactMetrics(gear, models.TransferType_TRANSFER_TYPE_LOAN, cfg, nil, nil)

	if ie == nil {
		t.Fatal("BuildTransferImpactMetrics returned nil")
	}
	// QT should still be computed with defaults when connCtx is nil.
	if ie.QualityTime == nil {
		t.Fatal("QualityTime is nil even with nil connCtx")
	}
	if ie.QualityTime.QualityTimeMinutes == nil {
		t.Fatal("QualityTime.QualityTimeMinutes is nil")
	}
	if ie.QualityTime.QualityTimeMinutes.Mean <= 0 {
		t.Errorf("QualityTimeMinutes.Mean = %v, want > 0", ie.QualityTime.QualityTimeMinutes.Mean)
	}
}

func TestBuildTransferImpactMetrics_ConnCtx_IncreasesRepeatTieStrength(t *testing.T) {
	cfg := loadTestConfig(t)

	gear := &models.Gear{
		ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 100},
	}

	// New connection (no prior interactions).
	ieNew := BuildTransferImpactMetrics(gear, models.TransferType_TRANSFER_TYPE_LOAN, cfg, nil, nil)
	// Established connection (2 prior interactions).
	connCtx := &api.ConnectionContext{PriorInteractionCount: 2}
	ieEstablished := BuildTransferImpactMetrics(gear, models.TransferType_TRANSFER_TYPE_LOAN, cfg, connCtx, nil)

	if ieNew.QualityTime == nil || ieEstablished.QualityTime == nil {
		t.Fatal("QualityTime is nil")
	}
	// Prior interactions reduce novelty, which affects QT. The established connection
	// has lower novelty so QT may be lower; but the test verifies the values differ.
	sfNew := ieNew.QualityTime.QualityTimeMinutes.Mean
	sfEstab := ieEstablished.QualityTime.QualityTimeMinutes.Mean
	if sfNew == sfEstab {
		t.Errorf("QT should differ between new (%v) and established (%v) connections", sfNew, sfEstab)
	}
}

func TestBuildRequestImpactMetrics_WithConnCtx_QTPresent(t *testing.T) {
	cfg := loadTestConfig(t)
	connCtx := &api.ConnectionContext{
		PriorInteractionCount: 1,
		MutualConnectionCount: 2,
	}

	ie := BuildRequestImpactMetrics(50.0, cfg, connCtx, nil)

	if ie == nil {
		t.Fatal("BuildRequestImpactMetrics returned nil")
	}
	if ie.QualityTime == nil || ie.QualityTime.QualityTimeMinutes == nil {
		t.Fatal("QualityTime or QualityTimeMinutes is nil")
	}
	if ie.QualityTime.QualityTimeMinutes.Mean <= 0 {
		t.Errorf("QualityTimeMinutes.Mean = %v, want > 0", ie.QualityTime.QualityTimeMinutes.Mean)
	}
}

func TestBuildExperienceImpactMetrics_WithConnCtx_QTScalesWithAttendees(t *testing.T) {
	cfg := loadTestConfig(t)
	connCtx := &api.ConnectionContext{
		PriorInteractionCount: 0,
		MutualConnectionCount: 1,
	}

	ie1 := BuildExperienceImpactMetrics(50.0, 2, cfg, connCtx, nil)
	ie5 := BuildExperienceImpactMetrics(50.0, 5, cfg, connCtx, nil)

	if ie1 == nil || ie5 == nil {
		t.Fatal("BuildExperienceImpactMetrics returned nil")
	}
	if ie1.QualityTime == nil || ie5.QualityTime == nil {
		t.Fatal("QualityTime is nil")
	}
	sf1 := ie1.QualityTime.QualityTimeMinutes.Mean
	sf5 := ie5.QualityTime.QualityTimeMinutes.Mean
	if sf5 <= sf1 {
		t.Errorf("QT with 5 attendees (%v) should exceed QT with 2 attendees (%v)", sf5, sf1)
	}
}

func TestBuildQualityTimeAttributes_Loan_DefaultsPopulated(t *testing.T) {
	cfg := loadTestConfig(t)

	gear := &models.Gear{
		ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 100},
	}

	ie := BuildTransferImpactMetrics(gear, models.TransferType_TRANSFER_TYPE_LOAN, cfg, nil, nil)

	if ie == nil || ie.QualityTime == nil {
		t.Fatal("QualityTime is nil")
	}
	attrs := ie.QualityTime.Attributes
	if attrs == nil {
		t.Fatal("QualityTimeAttributes is nil")
	}

	// Duration should be the config default for gear loans (> 0).
	if attrs.EstimatedDurationMinutes <= 0 {
		t.Errorf("EstimatedDurationMinutes = %v, want > 0", attrs.EstimatedDurationMinutes)
	}

	// Loan default: in-person brief handoff.
	if attrs.Modality != api.SocialModality_SOCIAL_MODALITY_IN_PERSON_BRIEF {
		t.Errorf("Modality = %v, want IN_PERSON_BRIEF", attrs.Modality)
	}

	// Default group size: 2 (dyadic).
	if attrs.GroupSize != 2 {
		t.Errorf("GroupSize = %v, want 2", attrs.GroupSize)
	}

	// No prior interactions → new contact.
	if attrs.TieStrength != api.SocialTieStrength_SOCIAL_TIE_STRENGTH_NEW {
		t.Errorf("TieStrength = %v, want NEW", attrs.TieStrength)
	}

	// Transfer builder uses RoleGiving for loans.
	if attrs.Reciprocity != api.SocialReciprocity_SOCIAL_RECIPROCITY_GIVING {
		t.Errorf("Reciprocity = %v, want GIVING", attrs.Reciprocity)
	}

	// Default novelty: novel (no prior interactions).
	if attrs.Novelty != api.SocialNovelty_SOCIAL_NOVELTY_NOVEL {
		t.Errorf("Novelty = %v, want NOVEL", attrs.Novelty)
	}

	// Loan vulnerability default is 1.0 (medium).
	if attrs.Vulnerability != api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_MEDIUM {
		t.Errorf("Vulnerability = %v, want MEDIUM", attrs.Vulnerability)
	}
}

func TestBuildQualityTimeAttributes_Experience_HighVulnerability(t *testing.T) {
	cfg := loadTestConfig(t)

	ie := BuildExperienceImpactMetrics(0, 5, cfg, nil, nil)

	if ie == nil || ie.QualityTime == nil {
		t.Fatal("QualityTime is nil")
	}
	attrs := ie.QualityTime.Attributes
	if attrs == nil {
		t.Fatal("QualityTimeAttributes is nil")
	}

	// Experience defaults: in-person shared.
	if attrs.Modality != api.SocialModality_SOCIAL_MODALITY_IN_PERSON_SHARED {
		t.Errorf("Modality = %v, want IN_PERSON_SHARED", attrs.Modality)
	}

	// Experience has 5 attendees.
	if attrs.GroupSize != 5 {
		t.Errorf("GroupSize = %v, want 5", attrs.GroupSize)
	}

	// Experience uses RoleMutual.
	if attrs.Reciprocity != api.SocialReciprocity_SOCIAL_RECIPROCITY_MUTUAL {
		t.Errorf("Reciprocity = %v, want MUTUAL", attrs.Reciprocity)
	}

	// Experience vulnerability default is 1.3 (high).
	if attrs.Vulnerability != api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_HIGH {
		t.Errorf("Vulnerability = %v, want HIGH", attrs.Vulnerability)
	}
}

func TestBuildQualityTimeAttributes_Request_MutualReciprocity(t *testing.T) {
	cfg := loadTestConfig(t)

	ie := BuildRequestImpactMetrics(50.0, cfg, nil, nil)

	if ie == nil || ie.QualityTime == nil {
		t.Fatal("QualityTime is nil")
	}
	attrs := ie.QualityTime.Attributes
	if attrs == nil {
		t.Fatal("QualityTimeAttributes is nil")
	}

	// Request uses RoleMutual.
	if attrs.Reciprocity != api.SocialReciprocity_SOCIAL_RECIPROCITY_MUTUAL {
		t.Errorf("Reciprocity = %v, want MUTUAL", attrs.Reciprocity)
	}
}

func TestBuildQualityTimeAttributes_ConnCtx_TieStrengthTiers(t *testing.T) {
	cfg := loadTestConfig(t)
	gear := &models.Gear{ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 100}}

	tests := []struct {
		name         string
		priorCount   int32
		wantStrength api.SocialTieStrength
		wantNovelty  api.SocialNovelty
	}{
		{"new contact (0)", 0, api.SocialTieStrength_SOCIAL_TIE_STRENGTH_NEW, api.SocialNovelty_SOCIAL_NOVELTY_NOVEL},
		{"acquaintance (2)", 2, api.SocialTieStrength_SOCIAL_TIE_STRENGTH_ACQUAINTANCE, api.SocialNovelty_SOCIAL_NOVELTY_INFREQUENT},
		{"active (7)", 7, api.SocialTieStrength_SOCIAL_TIE_STRENGTH_ACTIVE, api.SocialNovelty_SOCIAL_NOVELTY_INFREQUENT},
		{"close (15)", 15, api.SocialTieStrength_SOCIAL_TIE_STRENGTH_CLOSE, api.SocialNovelty_SOCIAL_NOVELTY_INFREQUENT},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connCtx := &api.ConnectionContext{PriorInteractionCount: tt.priorCount}
			ie := BuildTransferImpactMetrics(gear, models.TransferType_TRANSFER_TYPE_LOAN, cfg, connCtx, nil)

			if ie == nil || ie.QualityTime == nil {
				t.Fatal("QualityTime is nil")
			}
			attrs := ie.QualityTime.Attributes
			if attrs == nil {
				t.Fatal("QualityTimeAttributes is nil")
			}
			if attrs.TieStrength != tt.wantStrength {
				t.Errorf("TieStrength = %v, want %v", attrs.TieStrength, tt.wantStrength)
			}
			if attrs.Novelty != tt.wantNovelty {
				t.Errorf("Novelty = %v, want %v", attrs.Novelty, tt.wantNovelty)
			}
		})
	}
}

// --- Phase 12: Live Duration & Group Size ---.

func TestBuildExperienceImpactMetrics_UserSetDuration(t *testing.T) {
	cfg := loadTestConfig(t)

	hint := &QualityTimeHint{
		UserDurationMinutes: 240, // 4-hour potluck set by host
	}
	ie := BuildExperienceImpactMetrics(0, 8, cfg, nil, hint)

	if ie == nil || ie.QualityTime == nil {
		t.Fatal("QualityTime is nil")
	}

	// Provenance source must be USER when duration was host-provided.
	p := ie.QualityTime.Provenance
	if p == nil {
		t.Fatal("QualityTime.Provenance is nil")
	}
	if p.Source != api.ProvenanceSource_PROVENANCE_SOURCE_USER {
		t.Errorf("Provenance.Source = %v, want USER", p.Source)
	}
	if p.GetReasoning() == "" {
		t.Error("Provenance.Reasoning should be non-empty")
	}

	// Attributes must record the user-set duration.
	attrs := ie.QualityTime.Attributes
	if attrs == nil {
		t.Fatal("QualityTimeAttributes is nil")
	}
	if attrs.EstimatedDurationMinutes != 240 {
		t.Errorf("EstimatedDurationMinutes = %v, want 240", attrs.EstimatedDurationMinutes)
	}
}

func TestBuildExperienceImpactMetrics_UserDurationFallsBackToConfig(t *testing.T) {
	cfg := loadTestConfig(t)

	// No hint → config default.
	ieDefault := BuildExperienceImpactMetrics(0, 5, cfg, nil, nil)
	// Zero user duration → same as no hint.
	ieZero := BuildExperienceImpactMetrics(0, 5, cfg, nil, &QualityTimeHint{UserDurationMinutes: 0})

	if ieDefault == nil || ieZero == nil {
		t.Fatal("BuildExperienceImpactMetrics returned nil")
	}
	if ieDefault.QualityTime == nil || ieZero.QualityTime == nil {
		t.Fatal("QualityTime is nil")
	}

	// Both should have FORMULA provenance (config default path).
	if p := ieDefault.QualityTime.Provenance; p.Source != api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA {
		t.Errorf("default: Provenance.Source = %v, want FORMULA", p.Source)
	}
	if p := ieZero.QualityTime.Provenance; p.Source != api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA {
		t.Errorf("zero hint: Provenance.Source = %v, want FORMULA", p.Source)
	}

	// QT scores should be identical.
	sfDefault := ieDefault.QualityTime.QualityTimeMinutes.Mean
	sfZero := ieZero.QualityTime.QualityTimeMinutes.Mean
	if sfDefault != sfZero {
		t.Errorf("QT should be equal for nil hint vs zero user duration: %v vs %v", sfDefault, sfZero)
	}
}

func TestBuildExperienceImpactMetrics_UserDurationTakesPriorityOverLLM(t *testing.T) {
	cfg := loadTestConfig(t)

	// LLM says 30 min, user says 240 min — user wins.
	hintLLMOnly := &QualityTimeHint{DurationMinutes: 30}
	hintUserSet := &QualityTimeHint{DurationMinutes: 30, UserDurationMinutes: 240}

	ieLLM := BuildExperienceImpactMetrics(0, 5, cfg, nil, hintLLMOnly)
	ieUser := BuildExperienceImpactMetrics(0, 5, cfg, nil, hintUserSet)

	if ieLLM == nil || ieUser == nil {
		t.Fatal("BuildExperienceImpactMetrics returned nil")
	}
	if ieLLM.QualityTime == nil || ieUser.QualityTime == nil {
		t.Fatal("QualityTime is nil")
	}

	// User-set QT must be higher (longer duration).
	sfLLM := ieLLM.QualityTime.QualityTimeMinutes.Mean
	sfUser := ieUser.QualityTime.QualityTimeMinutes.Mean
	if sfUser <= sfLLM {
		t.Errorf("user-set QT (%v) should exceed LLM QT (%v) for longer duration", sfUser, sfLLM)
	}

	// User-set must have USER provenance.
	if p := ieUser.QualityTime.Provenance; p.Source != api.ProvenanceSource_PROVENANCE_SOURCE_USER {
		t.Errorf("user-set: Provenance.Source = %v, want USER", p.Source)
	}
}

func TestRebuildQualityTimeForGroupSize(t *testing.T) {
	cfg := loadTestConfig(t)

	tests := []struct {
		name      string
		groupSize int32
		reasoning string
	}{
		{"dyadic (default)", 2, "Default dyadic group size"},
		{"small group (5 RSVPs)", 5, "Group size from 5 confirmed RSVPs"},
		{"medium group (8 RSVPs)", 8, "Group size from 8 confirmed RSVPs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sf := RebuildQualityTimeForGroupSize(
				estimator.TransactionExperienceConcluded,
				tt.groupSize,
				estimator.RoleMutual,
				tt.reasoning,
				cfg,
			)

			if sf == nil {
				t.Fatal("RebuildQualityTimeForGroupSize returned nil")
			}
			if sf.QualityTimeMinutes == nil || sf.QualityTimeMinutes.Mean <= 0 {
				t.Errorf("QualityTimeMinutes.Mean = %v, want > 0", sf.GetQualityTimeMinutes().GetMean())
			}
			if sf.Provenance == nil {
				t.Fatal("Provenance is nil")
			}
			if sf.Provenance.Source != api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA {
				t.Errorf("Provenance.Source = %v, want FORMULA", sf.Provenance.Source)
			}
			if sf.Provenance.GetReasoning() != tt.reasoning {
				t.Errorf("Provenance.Reasoning = %q, want %q", sf.Provenance.GetReasoning(), tt.reasoning)
			}
			if sf.Attributes == nil {
				t.Fatal("Attributes is nil")
			}
			if sf.Attributes.GroupSize != tt.groupSize {
				t.Errorf("Attributes.GroupSize = %d, want %d", sf.Attributes.GroupSize, tt.groupSize)
			}
		})
	}
}

func TestRebuildQualityTimeForGroupSize_LargerGroupScoresHigher(t *testing.T) {
	cfg := loadTestConfig(t)

	sf2 := RebuildQualityTimeForGroupSize(estimator.TransactionExperienceConcluded, 2, estimator.RoleMutual, "", cfg)
	sf8 := RebuildQualityTimeForGroupSize(estimator.TransactionExperienceConcluded, 8, estimator.RoleMutual, "", cfg)

	if sf2 == nil || sf8 == nil {
		t.Fatal("RebuildQualityTimeForGroupSize returned nil")
	}
	if sf8.QualityTimeMinutes.Mean <= sf2.QualityTimeMinutes.Mean {
		t.Errorf("group of 8 (%v) should score higher than group of 2 (%v)", sf8.QualityTimeMinutes.Mean, sf2.QualityTimeMinutes.Mean)
	}
}

func TestRebuildQualityTimeForGroupSize_IdempotentSameGroupSize(t *testing.T) {
	cfg := loadTestConfig(t)

	sf1 := RebuildQualityTimeForGroupSize(estimator.TransactionExperienceConcluded, 5, estimator.RoleMutual, "same", cfg)
	sf2 := RebuildQualityTimeForGroupSize(estimator.TransactionExperienceConcluded, 5, estimator.RoleMutual, "same", cfg)

	if sf1 == nil || sf2 == nil {
		t.Fatal("RebuildQualityTimeForGroupSize returned nil")
	}
	if sf1.QualityTimeMinutes.Mean != sf2.QualityTimeMinutes.Mean {
		t.Errorf("same group size should produce same QT score: %v vs %v", sf1.QualityTimeMinutes.Mean, sf2.QualityTimeMinutes.Mean)
	}
}

func TestUpdateQualityTime_PreservesOtherDimensions(t *testing.T) {
	cfg := loadTestConfig(t)

	gear := &models.Gear{
		ValueEstimate: &models.ValueEstimate{EstimatedValueUsd: 200},
		EmbodiedCarbon: &models.CarbonEstimate{
			Co2EGrams: &models.Estimate{Mean: 10000, Stddev: 3000},
		},
		WeightGrams: &models.TrackedEstimate{Value: &models.Estimate{Mean: 2000, Stddev: 500}},
	}
	// Build a full IE and convert to models.
	ie := BuildTransferImpactMetrics(gear, models.TransferType_TRANSFER_TYPE_LOAN, cfg, nil, nil)
	modelsIE := APIImpactToModels(ie)

	// Build a new QT with a different group size.
	newSF := RebuildQualityTimeForGroupSize(
		estimator.TransactionExperienceConcluded, 8, estimator.RoleMutual,
		"Group size from 8 RSVPs", cfg,
	)

	updated := UpdateQualityTime(modelsIE, newSF)

	if updated == nil {
		t.Fatal("UpdateQualityTime returned nil")
	}

	// Money, carbon, and time must be preserved.
	if updated.MoneySaved == nil {
		t.Error("MoneySaved should be preserved after UpdateQualityTime")
	}
	if updated.EmissionsPrevented == nil {
		t.Error("EmissionsPrevented should be preserved after UpdateQualityTime")
	}
	if updated.TimeSaved == nil {
		t.Error("TimeSaved should be preserved after UpdateQualityTime")
	}

	// Quality time must reflect the new QT score (group of 8 > group of 2).
	if updated.QualityTime == nil {
		t.Fatal("QualityTime is nil after UpdateQualityTime")
	}
	if updated.QualityTime.QualityTimeMinutes == nil {
		t.Fatal("QualityTime.QualityTimeMinutes is nil after UpdateQualityTime")
	}
	// The updated QT (group of 8) should exceed the original (group of 2).
	if updated.QualityTime.QualityTimeMinutes.Mean <= ie.QualityTime.QualityTimeMinutes.Mean {
		t.Errorf("updated QT (%v) should exceed original QT (%v) for larger group",
			updated.QualityTime.QualityTimeMinutes.Mean, ie.QualityTime.QualityTimeMinutes.Mean)
	}
}

func TestUpdateQualityTime_NilInput(t *testing.T) {
	cfg := loadTestConfig(t)
	newSF := RebuildQualityTimeForGroupSize(estimator.TransactionExperienceConcluded, 4, estimator.RoleMutual, "", cfg)

	result := UpdateQualityTime(nil, newSF)
	if result != nil {
		t.Error("UpdateQualityTime(nil, ...) should return nil")
	}
}
