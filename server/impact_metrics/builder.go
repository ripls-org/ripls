package impact_metrics

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
)

// ImpactOverrides holds optional user-supplied values for individual impact inputs.
// Fields present here replace LLM-drafted values and stamp source=USER provenance
// on the affected input only; all other inputs retain source=LLM.
// Composite fields (quality_time_minutes, value_usd, manufacture_avoided_carbon)
// are always recomputed from inputs — setting them directly via overrides is rejected.
type ImpactOverrides struct {
	QualityTime  *api.QualityTimeAttributes
	MoneySavings *api.MoneySavings
	Emissions    *api.PreventedEmissions
}

// QualityTimeHint carries social attributes for a transaction from various sources.
// When non-nil, its values override config defaults in buildQualityTime.
type QualityTimeHint struct {
	// DurationMinutes is the LLM-estimated face-to-face interaction duration.
	// Zero means no LLM estimate; the config default for the transaction type is used.
	DurationMinutes float32

	// VulnerabilityLevel is the LLM-classified vulnerability level: "high", "medium", or "low".
	// Empty means no LLM classification; the config default for the transaction type is used.
	VulnerabilityLevel string

	// UserDurationMinutes is the host-set duration from the experience record.
	// When > 0, this takes priority over DurationMinutes and the config default.
	// The quality time provenance source will be set to PROVENANCE_SOURCE_USER.
	UserDurationMinutes float32

	// SocialContext carries owner-overridden social attribute tiers. When non-nil,
	// TieStrength, Reciprocity, Novelty, Vulnerability, and Modality override their
	// respective config defaults. Zero enum values mean "not overridden".
	SocialContext *api.SocialContext

	// EffortContributorCount is the number of distinct people who made non-trivial
	// PlanningContributions to this event (not counting passive attendees).
	// When > 1 and a significant minority of attendees are contributors, an
	// asymmetry bonus is applied up to QualityTimeMaxEffortAsymmetryFactor.
	EffortContributorCount int

	// HasCrossCommunityAttendees indicates that attendees came from more than one
	// community. When true, a tie-strength multiplier is applied.
	HasCrossCommunityAttendees bool

	// IsMultiHost is true when multiple PlanningContribution records indicate
	// distributed hosting (venue/space contributions from more than one user).
	// When true, the vulnerability score is blended toward the group average.
	IsMultiHost bool

	// HostVulnerabilityLevel is the LLM-classified vulnerability of the primary host.
	// Used alongside IsMultiHost to blend toward a group average.
	HostVulnerabilityLevel float32
}

// BuildTransferImpactMetrics builds a full ImpactEstimate for a single transfer (loan or giveaway).
// Works for any transfer state (active, completed, etc.) — the estimate represents projected
// impact of the transaction. Uses the gear's stored metadata (value, embodied carbon, weight)
// to compute all three savings dimensions via the estimator library.
// connCtx and hint are optional (may be nil); when nil, defaults are used.
func BuildTransferImpactMetrics(gear *models.Gear, transferType models.TransferType, cfg *estimator.Config, connCtx *api.ConnectionContext, hint *QualityTimeHint) *api.ImpactEstimate {
	txType := transferTypeToEstimator(transferType)

	var valueUSD float32
	if gear.ValueEstimate != nil {
		valueUSD = gear.ValueEstimate.EstimatedValueUsd
	}

	var emissionsInput *estimator.EmissionsInput
	if gear.EmbodiedCarbon != nil || gearWeightGrams(gear) > 0 {
		emissionsInput = &estimator.EmissionsInput{
			EmbodiedCarbon: gearCarbonToAPI(gear.EmbodiedCarbon),
			WeightGrams:    gearWeightGrams(gear),
		}
	}

	moneySaved := estimator.EstimateMoneySaved(valueUSD, txType, cfg)
	// Merge AI sources and reasoning from gear's ValueEstimate into the impact estimate
	moneySaved = mergeValueSources(moneySaved, gear.ValueEstimate)

	ie := &api.ImpactEstimate{
		MoneySaved:         moneySaved,
		EmissionsPrevented: estimator.EstimateEmissionsPrevented(emissionsInput, txType, cfg),
		TimeSaved:          estimator.ScaleTimeSaved(estimator.EstimateGearTime(cfg), 1),
	}

	ie.QualityTime = buildQualityTime(txType, 2, estimator.RoleGiving, connCtx, hint, cfg)

	return ie
}

// BuildGearCumulativeImpactMetrics builds cumulative ImpactEstimate across multiple loans for gear stats.
// Includes both active and completed loans. Computes single-loan impact then scales all
// dimensions by timesLoaned.
func BuildGearCumulativeImpactMetrics(gear *models.Gear, timesLoaned int32, cfg *estimator.Config) *api.ImpactEstimate {
	if timesLoaned <= 0 {
		return &api.ImpactEstimate{}
	}

	txType := estimator.TransactionLoan

	var valueUSD float32
	if gear.ValueEstimate != nil {
		valueUSD = gear.ValueEstimate.EstimatedValueUsd
	}

	var emissionsInput *estimator.EmissionsInput
	if gear.EmbodiedCarbon != nil || gearWeightGrams(gear) > 0 {
		emissionsInput = &estimator.EmissionsInput{
			EmbodiedCarbon: gearCarbonToAPI(gear.EmbodiedCarbon),
			WeightGrams:    gearWeightGrams(gear),
		}
	}

	// Compute per-loan money and emissions, then scale by timesLoaned.
	perLoanMoney := estimator.EstimateMoneySaved(valueUSD, txType, cfg)
	// Merge AI sources and reasoning from gear's ValueEstimate
	perLoanMoney = mergeValueSources(perLoanMoney, gear.ValueEstimate)
	perLoanEmissions := estimator.EstimateEmissionsPrevented(emissionsInput, txType, cfg)

	// Compute single-loan QT (no individual connection context for cumulative).
	perLoanQT := buildQualityTime(txType, 2, estimator.RoleGiving, nil, nil, cfg)

	return &api.ImpactEstimate{
		MoneySaved:         scaleMoneySaved(perLoanMoney, timesLoaned),
		EmissionsPrevented: scaleEmissionsPrevented(perLoanEmissions, timesLoaned),
		TimeSaved:          estimator.ScaleTimeSaved(estimator.EstimateGearTime(cfg), timesLoaned),
		QualityTime:        scaleQualityTime(perLoanQT, timesLoaned),
	}
}

// BuildRequestImpactMetrics builds ImpactEstimate for a request in any state.
// valueUSD comes from the stored ValueEstimate on the request.
// The estimate represents projected impact — valid for both active and fulfilled requests.
// connCtx and hint are optional (may be nil); when nil, defaults are used.
func BuildRequestImpactMetrics(valueUSD float32, cfg *estimator.Config, connCtx *api.ConnectionContext, hint *QualityTimeHint) *api.ImpactEstimate {
	txType := estimator.TransactionRequestFulfilled
	ie := &api.ImpactEstimate{
		MoneySaved:         estimator.EstimateMoneySaved(valueUSD, txType, cfg),
		EmissionsPrevented: estimator.EstimateEmissionsPrevented(nil, txType, cfg),
		TimeSaved:          estimator.ScaleTimeSaved(estimator.EstimateRequestTime(0, cfg), 1),
	}
	ie.QualityTime = buildQualityTime(txType, 2, estimator.RoleMutual, connCtx, hint, cfg)
	return ie
}

// BuildExperienceImpactMetrics builds ImpactEstimate for an experience in any state.
// valueUSD comes from the stored ValueEstimate on the experience.
// Time is scaled by attendeeCount. Works for both in-progress and completed experiences.
// connCtx and hint are optional (may be nil); when nil, defaults are used.
func BuildExperienceImpactMetrics(valueUSD float32, attendeeCount int32, cfg *estimator.Config, connCtx *api.ConnectionContext, hint *QualityTimeHint) *api.ImpactEstimate {
	txType := estimator.TransactionExperienceConcluded
	if attendeeCount < 1 {
		attendeeCount = 1
	}
	ie := &api.ImpactEstimate{
		MoneySaved:         estimator.EstimateMoneySaved(valueUSD, txType, cfg),
		EmissionsPrevented: estimator.EstimateEmissionsPrevented(nil, txType, cfg),
		TimeSaved:          estimator.ScaleTimeSaved(estimator.EstimateExperienceTime(0, cfg), attendeeCount),
	}
	ie.QualityTime = buildQualityTime(txType, attendeeCount, estimator.RoleMutual, connCtx, hint, cfg)
	return ie
}

// buildQualityTime computes the Quality Time estimate for a transaction.
// connCtx and hint are optional; when nil, config defaults are used for all attributes.
func buildQualityTime(txType estimator.TransactionType, attendeeCount int32, role estimator.TransactionRole, connCtx *api.ConnectionContext, hint *QualityTimeHint, cfg *estimator.Config) *api.QualityTimeEstimate {
	if cfg == nil {
		return nil
	}

	input := estimator.QualityTimeInput{
		TxType:        txType,
		Modality:      estimator.ClassifyModality(txType),
		AttendeeCount: attendeeCount,
		Role:          role,
		Novelty:       estimator.NoveltyNovel, // Default to novel for new interactions
	}

	if connCtx != nil {
		input.PriorInteractions = connCtx.PriorInteractionCount
		// Classify novelty: novel if first interaction, otherwise infrequent
		if connCtx.PriorInteractionCount > 0 {
			input.Novelty = estimator.NoveltyInfrequent
		}
	}

	// Apply user-set or LLM-inferred attributes when available.
	// Priority: user-set duration > LLM inference > config default.
	userSetDuration := false
	if hint != nil {
		if hint.UserDurationMinutes > 0 {
			// User-provided duration (e.g. host sets "2-hour potluck") takes highest priority.
			input.Duration = estimator.EstimateSocialDuration(hint.UserDurationMinutes, txType, cfg)
			userSetDuration = true
		} else {
			input.Duration = estimator.EstimateSocialDuration(hint.DurationMinutes, txType, cfg)
		}
		v := estimator.VulnerabilityFromLevel(hint.VulnerabilityLevel)
		if v > 0 {
			input.Vulnerability = v
			input.VulnerabilityRelStddev = cfg.QualityTime.GetVulnerabilityLlmRelativeStddev()
		}
		// Owner-overridden social context takes priority over all other sources.
		if sc := hint.SocialContext; sc != nil {
			if sc.TieStrength != api.SocialTieStrength_SOCIAL_TIE_STRENGTH_UNSPECIFIED {
				input.PriorInteractions = socialTieStrengthToPriorInteractions(sc.TieStrength)
			}
			if sc.Reciprocity != api.SocialReciprocity_SOCIAL_RECIPROCITY_UNSPECIFIED {
				input.Role = socialReciprocityToRole(sc.Reciprocity)
			}
			if sc.Novelty != api.SocialNovelty_SOCIAL_NOVELTY_UNSPECIFIED {
				input.Novelty = socialNoveltyToEstimator(sc.Novelty)
			}
			if sc.Vulnerability != api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_UNSPECIFIED {
				input.Vulnerability = socialVulnerabilityToWeight(sc.Vulnerability, cfg)
				input.VulnerabilityRelStddev = 0 // No uncertainty for user-set values
			}
			if sc.Modality != api.SocialModality_SOCIAL_MODALITY_UNSPECIFIED {
				input.Modality = socialModalityFromAPI(sc.Modality)
			}
		}
	}

	// Apply secondary signals before computing composites.
	if hint != nil {
		// Cross-community bridging bonus: when attendees come from multiple communities,
		// boost tie-strength by treating them as if they have more prior interactions.
		if hint.HasCrossCommunityAttendees {
			factor := cfg.QualityTime.GetQualityTimeCrossCommunityTieStrengthFactor()
			if factor > 1.0 && input.PriorInteractions < 10 {
				// Scale prior interactions up proportionally, capped to avoid over-inflation.
				scaled := float32(input.PriorInteractions)*factor + 1
				if scaled > 10 {
					scaled = 10
				}
				input.PriorInteractions = int32(scaled)
			}
		}

		// Multi-host vulnerability averaging: blend per-host vulnerability toward
		// the group average when the event was co-hosted.
		if hint.IsMultiHost && input.Vulnerability > 0 && hint.HostVulnerabilityLevel > 0 {
			blend := cfg.QualityTime.GetQualityTimeMultiHostVulnerabilityBlend()
			if blend > 0 && blend <= 1 {
				groupAvg := (input.Vulnerability + hint.HostVulnerabilityLevel) / 2
				input.Vulnerability = input.Vulnerability*(1-blend) + groupAvg*blend
			}
		}
	}

	result := estimator.EstimateQualityTime(input, cfg)
	if result != nil {
		result.Attributes = buildQualityTimeAttributes(input, hint, cfg)

		// Effort asymmetry: scale quality time minutes up when contributors
		// are a significant minority of the attendee set.
		if hint != nil && hint.EffortContributorCount > 0 && result.QualityTimeMinutes != nil {
			asymmetryFactor := computeEffortAsymmetryFactor(hint.EffortContributorCount, int(attendeeCount), cfg)
			if asymmetryFactor > 1.0 {
				result.QualityTimeMinutes = &api.Estimate{
					Mean:   result.QualityTimeMinutes.Mean * asymmetryFactor,
					Stddev: result.QualityTimeMinutes.Stddev * asymmetryFactor,
				}
			}
		}

		// When the host explicitly set the duration, record USER provenance.
		if userSetDuration && result.Provenance != nil {
			result.Provenance.Source = api.ProvenanceSource_PROVENANCE_SOURCE_USER
			result.Provenance.Reasoning = proto.String(fmt.Sprintf(
				"Duration set by host (%.0f min). %s",
				hint.UserDurationMinutes, result.Provenance.GetReasoning(),
			))
		}
	}
	return result
}

// computeEffortAsymmetryFactor returns a multiplier > 1.0 when contributors are
// a significant minority of the attendee group, reflecting the additional meaning
// of shared contribution to the event. Returns 1.0 when contributors == attendees
// (everyone contributed equally) or when the ratio is too high to signal asymmetry.
func computeEffortAsymmetryFactor(contributorCount, attendeeCount int, cfg *estimator.Config) float32 {
	if attendeeCount <= 0 || contributorCount >= attendeeCount {
		return 1.0
	}
	maxFactor := cfg.QualityTime.GetQualityTimeMaxEffortAsymmetryFactor()
	if maxFactor <= 1.0 {
		return 1.0
	}

	// Ratio of contributors to total attendees.
	ratio := float32(contributorCount) / float32(attendeeCount)

	// Apply factor proportionally: fewer contributors → larger asymmetry bonus.
	// At ratio=0 we hit max factor; at ratio=1 factor is 1.0.
	return 1.0 + (maxFactor-1.0)*(1.0-ratio)
}

// qtAttributeProvenance returns the ProvenanceSource for a single QT attribute given the hint.
// The hint nil-checks are safe; all attribute classification follows the priority order:
// user-set SocialContext > LLM inference > formula/config default.
func qtAttributeProvenance(hint *QualityTimeHint, attr string) api.ProvenanceSource {
	if hint == nil {
		if attr == "duration" || attr == "vulnerability" {
			return api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT
		}
		return api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA
	}
	sc := hint.SocialContext
	switch attr {
	case "duration":
		if hint.UserDurationMinutes > 0 {
			return api.ProvenanceSource_PROVENANCE_SOURCE_USER
		}
		if hint.DurationMinutes > 0 {
			return api.ProvenanceSource_PROVENANCE_SOURCE_LLM
		}
		return api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT
	case "vulnerability":
		if sc != nil && sc.Vulnerability != api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_UNSPECIFIED {
			return api.ProvenanceSource_PROVENANCE_SOURCE_USER
		}
		if hint.VulnerabilityLevel != "" {
			return api.ProvenanceSource_PROVENANCE_SOURCE_LLM
		}
		return api.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT
	case "modality":
		if sc != nil && sc.Modality != api.SocialModality_SOCIAL_MODALITY_UNSPECIFIED {
			return api.ProvenanceSource_PROVENANCE_SOURCE_USER
		}
		return api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA
	case "tie_strength":
		if sc != nil && sc.TieStrength != api.SocialTieStrength_SOCIAL_TIE_STRENGTH_UNSPECIFIED {
			return api.ProvenanceSource_PROVENANCE_SOURCE_USER
		}
		return api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA
	case "reciprocity":
		if sc != nil && sc.Reciprocity != api.SocialReciprocity_SOCIAL_RECIPROCITY_UNSPECIFIED {
			return api.ProvenanceSource_PROVENANCE_SOURCE_USER
		}
		return api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA
	case "novelty":
		if sc != nil && sc.Novelty != api.SocialNovelty_SOCIAL_NOVELTY_UNSPECIFIED {
			return api.ProvenanceSource_PROVENANCE_SOURCE_USER
		}
		return api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA
	case "group_size":
		return api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA
	}
	return api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA
}

// simpleProvenance builds a minimal Provenance with the given source.
func simpleProvenance(source api.ProvenanceSource) *api.Provenance {
	return &api.Provenance{Source: source}
}

// buildQualityTimeAttributes constructs a QualityTimeAttributes proto from a resolved QualityTimeInput.
// hint is used to stamp per-attribute provenance (USER / LLM / FORMULA / CONFIG_DEFAULT).
func buildQualityTimeAttributes(input estimator.QualityTimeInput, hint *QualityTimeHint, cfg *estimator.Config) *api.QualityTimeAttributes {
	// Resolve duration mean — use the LLM-inferred hint when present, else the config default.
	duration := input.Duration
	if duration == nil || duration.Mean <= 0 {
		duration = estimator.EstimateSocialDuration(0, input.TxType, cfg)
	}
	durationMins := float32(0)
	if duration != nil {
		durationMins = duration.Mean
	}

	// Resolve vulnerability weight — use explicit value or fall back to config default.
	vulnWeight := input.Vulnerability
	if vulnWeight <= 0 {
		vulnWeight = estimator.DefaultVulnerability(input.TxType, cfg)
	}

	attendees := input.AttendeeCount
	if attendees < 2 {
		attendees = 2
	}

	return &api.QualityTimeAttributes{
		EstimatedDurationMinutes:           durationMins,
		EstimatedDurationMinutesProvenance: simpleProvenance(qtAttributeProvenance(hint, "duration")),
		Modality:                           socialModalityToAPI(input.Modality),
		ModalityProvenance:                 simpleProvenance(qtAttributeProvenance(hint, "modality")),
		GroupSize:                          attendees,
		GroupSizeProvenance:                simpleProvenance(qtAttributeProvenance(hint, "group_size")),
		TieStrength:                        socialTieStrengthToAPI(input.PriorInteractions),
		TieStrengthProvenance:              simpleProvenance(qtAttributeProvenance(hint, "tie_strength")),
		Reciprocity:                        socialReciprocityToAPI(input.Role),
		ReciprocityProvenance:              simpleProvenance(qtAttributeProvenance(hint, "reciprocity")),
		Novelty:                            socialNoveltyToAPI(input.Novelty),
		NoveltyProvenance:                  simpleProvenance(qtAttributeProvenance(hint, "novelty")),
		Vulnerability:                      socialVulnerabilityToAPI(vulnWeight),
		VulnerabilityProvenance:            simpleProvenance(qtAttributeProvenance(hint, "vulnerability")),
	}
}

// socialModalityToAPI converts an estimator ModalityType to an api.SocialModality enum.
func socialModalityToAPI(m estimator.ModalityType) api.SocialModality {
	switch m {
	case estimator.ModalityInPersonShared:
		return api.SocialModality_SOCIAL_MODALITY_IN_PERSON_SHARED
	case estimator.ModalityInPersonTransactional:
		return api.SocialModality_SOCIAL_MODALITY_IN_PERSON_BRIEF
	case estimator.ModalityVideo:
		return api.SocialModality_SOCIAL_MODALITY_VIDEO
	case estimator.ModalityPhone:
		return api.SocialModality_SOCIAL_MODALITY_PHONE
	case estimator.ModalityText:
		return api.SocialModality_SOCIAL_MODALITY_TEXT
	default:
		return api.SocialModality_SOCIAL_MODALITY_UNSPECIFIED
	}
}

// socialTieStrengthToAPI converts a prior interaction count to an api.SocialTieStrength enum.
// Tiers mirror ClassifyTieStrength: new (0), acquaintance (1–3), active (4–10), close (11+).
func socialTieStrengthToAPI(priorInteractions int32) api.SocialTieStrength {
	switch {
	case priorInteractions == 0:
		return api.SocialTieStrength_SOCIAL_TIE_STRENGTH_NEW
	case priorInteractions <= 3:
		return api.SocialTieStrength_SOCIAL_TIE_STRENGTH_ACQUAINTANCE
	case priorInteractions <= 10:
		return api.SocialTieStrength_SOCIAL_TIE_STRENGTH_ACTIVE
	default:
		return api.SocialTieStrength_SOCIAL_TIE_STRENGTH_CLOSE
	}
}

// socialReciprocityToAPI converts a TransactionRole to an api.SocialReciprocity enum.
func socialReciprocityToAPI(role estimator.TransactionRole) api.SocialReciprocity {
	switch role {
	case estimator.RoleGiving:
		return api.SocialReciprocity_SOCIAL_RECIPROCITY_GIVING
	case estimator.RoleReceiving:
		return api.SocialReciprocity_SOCIAL_RECIPROCITY_RECEIVING
	case estimator.RoleMutual:
		return api.SocialReciprocity_SOCIAL_RECIPROCITY_MUTUAL
	default:
		return api.SocialReciprocity_SOCIAL_RECIPROCITY_UNSPECIFIED
	}
}

// socialNoveltyToAPI converts a NoveltyLevel to an api.SocialNovelty enum.
func socialNoveltyToAPI(n estimator.NoveltyLevel) api.SocialNovelty {
	switch n {
	case estimator.NoveltyNovel:
		return api.SocialNovelty_SOCIAL_NOVELTY_NOVEL
	case estimator.NoveltyInfrequent:
		return api.SocialNovelty_SOCIAL_NOVELTY_INFREQUENT
	case estimator.NoveltyRoutine:
		return api.SocialNovelty_SOCIAL_NOVELTY_ROUTINE
	default:
		return api.SocialNovelty_SOCIAL_NOVELTY_UNSPECIFIED
	}
}

// socialVulnerabilityToAPI maps a vulnerability weight to api.SocialVulnerabilityLevel.
// Thresholds: high ≥ 1.15 (1.3×), medium ≥ 0.9 (1.0×), low < 0.9 (0.8×).
func socialVulnerabilityToAPI(weight float32) api.SocialVulnerabilityLevel {
	switch {
	case weight >= 1.15:
		return api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_HIGH
	case weight >= 0.9:
		return api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_MEDIUM
	default:
		return api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_LOW
	}
}

// socialModalityFromAPI converts an api.SocialModality enum to an estimator.ModalityType.
func socialModalityFromAPI(m api.SocialModality) estimator.ModalityType {
	switch m {
	case api.SocialModality_SOCIAL_MODALITY_IN_PERSON_SHARED:
		return estimator.ModalityInPersonShared
	case api.SocialModality_SOCIAL_MODALITY_IN_PERSON_BRIEF:
		return estimator.ModalityInPersonTransactional
	case api.SocialModality_SOCIAL_MODALITY_VIDEO:
		return estimator.ModalityVideo
	case api.SocialModality_SOCIAL_MODALITY_PHONE:
		return estimator.ModalityPhone
	case api.SocialModality_SOCIAL_MODALITY_TEXT:
		return estimator.ModalityText
	default:
		return estimator.ModalityInPersonShared
	}
}

// socialTieStrengthToPriorInteractions converts a SocialTieStrength enum to a
// representative prior interaction count. Uses the lower bound of each tier.
func socialTieStrengthToPriorInteractions(t api.SocialTieStrength) int32 {
	switch t {
	case api.SocialTieStrength_SOCIAL_TIE_STRENGTH_NEW:
		return 0
	case api.SocialTieStrength_SOCIAL_TIE_STRENGTH_ACQUAINTANCE:
		return 1
	case api.SocialTieStrength_SOCIAL_TIE_STRENGTH_ACTIVE:
		return 4
	case api.SocialTieStrength_SOCIAL_TIE_STRENGTH_CLOSE:
		return 11
	default:
		return 0
	}
}

// socialReciprocityToRole converts a SocialReciprocity enum to an estimator.TransactionRole.
func socialReciprocityToRole(r api.SocialReciprocity) estimator.TransactionRole {
	switch r {
	case api.SocialReciprocity_SOCIAL_RECIPROCITY_GIVING:
		return estimator.RoleGiving
	case api.SocialReciprocity_SOCIAL_RECIPROCITY_RECEIVING:
		return estimator.RoleReceiving
	default:
		return estimator.RoleMutual
	}
}

// socialNoveltyToEstimator converts a SocialNovelty enum to an estimator.NoveltyLevel.
func socialNoveltyToEstimator(n api.SocialNovelty) estimator.NoveltyLevel {
	switch n {
	case api.SocialNovelty_SOCIAL_NOVELTY_NOVEL:
		return estimator.NoveltyNovel
	case api.SocialNovelty_SOCIAL_NOVELTY_INFREQUENT:
		return estimator.NoveltyInfrequent
	case api.SocialNovelty_SOCIAL_NOVELTY_ROUTINE:
		return estimator.NoveltyRoutine
	default:
		return estimator.NoveltyNovel
	}
}

// socialVulnerabilityToWeight converts a SocialVulnerabilityLevel enum to its
// multiplier weight. Mirrors VulnerabilityFromLevel thresholds: high=1.3, medium=1.0, low=0.8.
func socialVulnerabilityToWeight(v api.SocialVulnerabilityLevel, _ *estimator.Config) float32 {
	switch v {
	case api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_HIGH:
		return 1.3
	case api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_MEDIUM:
		return 1.0
	case api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_LOW:
		return 0.8
	default:
		return 0
	}
}

// ApplyImpactOverrides returns a new ImpactEstimate derived from base with the
// caller-supplied overrides applied. Each overridden input field is re-stamped
// with source=USER provenance; non-overridden inputs retain their original
// provenance. Composite output fields (quality_time_minutes, value_usd,
// manufacture_avoided_carbon) are always recomputed from the merged inputs.
// Returns an error if overrides attempt to set a composite field directly.
func ApplyImpactOverrides(base *api.ImpactEstimate, overrides *ImpactOverrides, cfg *estimator.Config) (*api.ImpactEstimate, error) {
	if overrides == nil || base == nil {
		return base, nil
	}

	result := proto.Clone(base).(*api.ImpactEstimate)

	if overrides.QualityTime != nil {
		hint := qualityTimeHintFromOverrides(overrides.QualityTime)
		// Determine attendee count: prefer override group_size, else base.
		attendeeCount := int32(2)
		if overrides.QualityTime.GroupSize > 0 {
			attendeeCount = overrides.QualityTime.GroupSize
		} else if base.QualityTime != nil && base.QualityTime.Attributes != nil {
			attendeeCount = base.QualityTime.Attributes.GroupSize
		}
		qt := buildQualityTime(estimator.TransactionExperienceConcluded, attendeeCount, estimator.RoleMutual, nil, hint, cfg)
		if qt != nil {
			if qt.Provenance != nil {
				qt.Provenance.Source = api.ProvenanceSource_PROVENANCE_SOURCE_USER
			}
			// Per-attribute provenance: USER on each overridden field; preserve base for the rest.
			if qt.Attributes != nil {
				stampQTAttributeProvenances(qt.Attributes, overrides.QualityTime, base.GetQualityTime().GetAttributes())
			}
		}
		result.QualityTime = qt
	}

	if overrides.MoneySavings != nil {
		result.MoneySaved = applyMoneySavingsOverride(base.MoneySaved, overrides.MoneySavings)
	}

	if overrides.Emissions != nil {
		result.EmissionsPrevented = applyEmissionsOverride(base.EmissionsPrevented, overrides.Emissions)
	}

	return result, nil
}

// stampQTAttributeProvenances updates attrs in-place: each attribute present in override gets
// USER provenance; each attribute absent from override inherits provenance from base (if any).
func stampQTAttributeProvenances(attrs, override, base *api.QualityTimeAttributes) {
	userProv := simpleProvenance(api.ProvenanceSource_PROVENANCE_SOURCE_USER)

	if override.EstimatedDurationMinutes > 0 {
		attrs.EstimatedDurationMinutesProvenance = userProv
	} else if base != nil {
		attrs.EstimatedDurationMinutesProvenance = base.EstimatedDurationMinutesProvenance
	}
	if override.Modality != api.SocialModality_SOCIAL_MODALITY_UNSPECIFIED {
		attrs.ModalityProvenance = userProv
	} else if base != nil {
		attrs.ModalityProvenance = base.ModalityProvenance
	}
	if override.GroupSize > 0 {
		attrs.GroupSizeProvenance = userProv
	} else if base != nil {
		attrs.GroupSizeProvenance = base.GroupSizeProvenance
	}
	if override.TieStrength != api.SocialTieStrength_SOCIAL_TIE_STRENGTH_UNSPECIFIED {
		attrs.TieStrengthProvenance = userProv
	} else if base != nil {
		attrs.TieStrengthProvenance = base.TieStrengthProvenance
	}
	if override.Reciprocity != api.SocialReciprocity_SOCIAL_RECIPROCITY_UNSPECIFIED {
		attrs.ReciprocityProvenance = userProv
	} else if base != nil {
		attrs.ReciprocityProvenance = base.ReciprocityProvenance
	}
	if override.Novelty != api.SocialNovelty_SOCIAL_NOVELTY_UNSPECIFIED {
		attrs.NoveltyProvenance = userProv
	} else if base != nil {
		attrs.NoveltyProvenance = base.NoveltyProvenance
	}
	if override.Vulnerability != api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_UNSPECIFIED {
		attrs.VulnerabilityProvenance = userProv
	} else if base != nil {
		attrs.VulnerabilityProvenance = base.VulnerabilityProvenance
	}
}

// applyMoneySavingsOverride merges caller-supplied money savings inputs into the base estimate.
// The composite value_usd is recomputed from the override's inputs: hire_equivalent_value when
// present, otherwise a sum of contribution value_usd fields. Composites from the override are
// never accepted directly. The top-level provenance is stamped USER.
func applyMoneySavingsOverride(base, override *api.MoneySavings) *api.MoneySavings {
	inputs := override.GetInputs()

	// Recompute composite value_usd from inputs.
	var valueUSD *api.Estimate
	if inputs != nil {
		if inputs.HireEquivalentValue != nil {
			valueUSD = inputs.HireEquivalentValue
		} else if len(inputs.ContributionInputs) > 0 {
			var parts []*api.Estimate
			for _, c := range inputs.ContributionInputs {
				if c.GetValueUsd() != nil {
					parts = append(parts, c.GetValueUsd())
				}
			}
			if len(parts) > 0 {
				valueUSD = estimator.SumEstimates(parts)
			}
		}
	}

	// Fall back to the base composite when no inputs provide a value.
	if valueUSD == nil && base != nil {
		valueUSD = base.GetValueUsd()
	}

	userProv := &api.Provenance{
		Source: api.ProvenanceSource_PROVENANCE_SOURCE_USER,
	}
	if base != nil && base.Provenance != nil {
		userProv.Name = base.Provenance.Name
		userProv.Version = base.Provenance.Version
	}

	return &api.MoneySavings{
		ValueUsd:   valueUSD,
		Inputs:     inputs,
		Provenance: userProv,
	}
}

// applyEmissionsOverride merges caller-supplied emissions inputs into the base estimate.
// The carbon composite fields are preserved from the base (they depend on gear data not
// present in the override). The inputs field is replaced with the override's inputs and
// the top-level provenance is stamped USER.
func applyEmissionsOverride(base, override *api.PreventedEmissions) *api.PreventedEmissions {
	inputs := override.GetInputs()

	// Preserve base composites — they cannot be recomputed without original gear data.
	var manufactureCarbon *api.CarbonEstimate
	var wasteCarbon *api.CarbonEstimate
	if base != nil {
		manufactureCarbon = base.ManufactureAvoidedCarbon
		wasteCarbon = base.WasteReducedCarbon
	}

	userProv := &api.Provenance{
		Source: api.ProvenanceSource_PROVENANCE_SOURCE_USER,
	}
	if base != nil && base.Provenance != nil {
		userProv.Name = base.Provenance.Name
		userProv.Version = base.Provenance.Version
	}

	return &api.PreventedEmissions{
		ManufactureAvoidedCarbon: manufactureCarbon,
		WasteReducedCarbon:       wasteCarbon,
		Inputs:                   inputs,
		Provenance:               userProv,
	}
}

// qualityTimeHintFromOverrides converts a QualityTimeAttributes override into a QualityTimeHint.
// All non-zero attribute values are treated as user-supplied.
func qualityTimeHintFromOverrides(attrs *api.QualityTimeAttributes) *QualityTimeHint {
	if attrs == nil {
		return nil
	}
	hint := &QualityTimeHint{}
	if attrs.EstimatedDurationMinutes > 0 {
		hint.UserDurationMinutes = attrs.EstimatedDurationMinutes
	}
	if attrs.Vulnerability != api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_UNSPECIFIED {
		switch attrs.Vulnerability {
		case api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_HIGH:
			hint.VulnerabilityLevel = "high"
		case api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_MEDIUM:
			hint.VulnerabilityLevel = "medium"
		case api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_LOW:
			hint.VulnerabilityLevel = "low"
		}
	}
	sc := &api.SocialContext{}
	if attrs.Modality != api.SocialModality_SOCIAL_MODALITY_UNSPECIFIED {
		sc.Modality = attrs.Modality
	}
	if attrs.TieStrength != api.SocialTieStrength_SOCIAL_TIE_STRENGTH_UNSPECIFIED {
		sc.TieStrength = attrs.TieStrength
	}
	if attrs.Reciprocity != api.SocialReciprocity_SOCIAL_RECIPROCITY_UNSPECIFIED {
		sc.Reciprocity = attrs.Reciprocity
	}
	if attrs.Novelty != api.SocialNovelty_SOCIAL_NOVELTY_UNSPECIFIED {
		sc.Novelty = attrs.Novelty
	}
	if attrs.Vulnerability != api.SocialVulnerabilityLevel_SOCIAL_VULNERABILITY_LEVEL_UNSPECIFIED {
		sc.Vulnerability = attrs.Vulnerability
	}
	hint.SocialContext = sc
	return hint
}

// scaleQualityTime scales a QualityTimeEstimate by a multiplier.
// Returns sf unchanged if multiplier <= 1 or sf is nil.
func scaleQualityTime(sf *api.QualityTimeEstimate, multiplier int32) *api.QualityTimeEstimate {
	if sf == nil || multiplier <= 1 {
		return sf
	}
	result := &api.QualityTimeEstimate{
		Provenance: sf.Provenance,
	}
	if sf.QualityTimeMinutes != nil {
		result.QualityTimeMinutes = estimator.ScaleEstimate(sf.QualityTimeMinutes, float32(multiplier))
	}
	if sf.BelongingMinutes != nil {
		result.BelongingMinutes = estimator.ScaleEstimate(sf.BelongingMinutes, float32(multiplier))
	}
	if sf.TrustCredits != nil {
		result.TrustCredits = estimator.ScaleEstimate(sf.TrustCredits, float32(multiplier))
	}
	return result
}

// RebuildQualityTimeForGroupSize rebuilds the QualityTimeEstimate for a transaction
// when only the group size has changed (e.g., RSVP added/removed, offer submitted).
// All other attributes (duration, vulnerability, tie strength, novelty) use config defaults.
// Returns nil if cfg is nil. The provenance source is PROVENANCE_SOURCE_FORMULA with
// reasoningMsg describing the data source (e.g., "Group size from 6 confirmed RSVPs").
func RebuildQualityTimeForGroupSize(
	txType estimator.TransactionType,
	groupSize int32,
	role estimator.TransactionRole,
	reasoningMsg string,
	cfg *estimator.Config,
) *api.QualityTimeEstimate {
	return RebuildQualityTimeForGroupSizeWithContext(txType, groupSize, role, nil, nil, reasoningMsg, cfg)
}

// RebuildQualityTimeForGroupSizeWithContext rebuilds the QualityTimeEstimate
// for a transaction from the live group size plus the caller's connection
// context and social hint. The preview and commit paths of a completion flow
// must call this with identical inputs so the number a user previews is the
// number that persists (#2724). Returns nil if cfg is nil; provenance is
// PROVENANCE_SOURCE_FORMULA with reasoningMsg describing the data source.
func RebuildQualityTimeForGroupSizeWithContext(
	txType estimator.TransactionType,
	groupSize int32,
	role estimator.TransactionRole,
	connCtx *api.ConnectionContext,
	hint *QualityTimeHint,
	reasoningMsg string,
	cfg *estimator.Config,
) *api.QualityTimeEstimate {
	if cfg == nil {
		return nil
	}
	sf := buildQualityTime(txType, groupSize, role, connCtx, hint, cfg)
	if sf != nil && sf.Provenance != nil {
		sf.Provenance.Source = api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA
		sf.Provenance.Reasoning = proto.String(reasoningMsg)
	}
	return sf
}

// UpdateQualityTime returns a copy of ie with the quality_time field replaced by qt.
// All other fields (money_saved, emissions_prevented, time_saved) are preserved unchanged.
// Returns nil if ie is nil.
func UpdateQualityTime(ie *models.ImpactEstimate, qt *api.QualityTimeEstimate) *models.ImpactEstimate {
	if ie == nil {
		return nil
	}
	apiIE := ModelsImpactToAPI(ie)
	apiIE.QualityTime = qt
	return APIImpactToModels(apiIE)
}

// mergeValueSources combines estimator-generated money savings with AI-provided value sources.
// Prepends AI reasoning to estimator reasoning and appends AI sources to estimator sources.
// This creates a complete provenance chain showing both the price source (AI) and the
// prevented purchase methodology (estimator).
func mergeValueSources(ms *api.MoneySavings, ve *models.ValueEstimate) *api.MoneySavings {
	if ms == nil || ve == nil {
		return ms
	}

	// Create a copy to avoid mutating the original
	result := &api.MoneySavings{
		ValueUsd: ms.ValueUsd,
	}

	// Build merged provenance from estimator provenance + AI value estimate fields.
	p := &api.Provenance{}
	if ms.Provenance != nil {
		p.Source = ms.Provenance.Source
		p.Name = ms.Provenance.Name
		p.Version = ms.Provenance.Version
	}

	// Extract AI provenance fields from ValueEstimate.
	var aiReasoning string
	var aiSources []string
	if ve.Provenance != nil {
		aiReasoning = ve.Provenance.GetReasoning()
		aiSources = ve.Provenance.Sources
	}

	// Merge reasoning: prepend AI reasoning to estimator reasoning
	estimatorReasoning := ""
	if ms.Provenance != nil {
		estimatorReasoning = ms.Provenance.GetReasoning()
	}
	if aiReasoning != "" && estimatorReasoning != "" {
		p.Reasoning = proto.String(aiReasoning + ". " + estimatorReasoning)
	} else if aiReasoning != "" {
		p.Reasoning = proto.String(aiReasoning)
	} else {
		p.Reasoning = proto.String(estimatorReasoning)
	}

	// Merge sources: append AI sources to estimator sources
	p.Sources = append([]string{}, aiSources...) // Copy AI sources first
	if ms.Provenance != nil {
		p.Sources = append(p.Sources, ms.Provenance.Sources...) // Append estimator sources
	}

	result.Provenance = p
	return result
}

// transferTypeToEstimator converts a proto TransferType to an estimator TransactionType.
func transferTypeToEstimator(tt models.TransferType) estimator.TransactionType {
	if tt == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
		return estimator.TransactionGiveaway
	}
	return estimator.TransactionLoan
}

// gearWeightGrams extracts the mean weight from a gear record, returning 0 if not set.
func gearWeightGrams(gear *models.Gear) float32 {
	if gear.WeightGrams != nil && gear.WeightGrams.Value != nil {
		return gear.WeightGrams.Value.Mean
	}
	return 0
}

// gearCarbonToAPI converts a models.CarbonEstimate to an api.CarbonEstimate.
func gearCarbonToAPI(mc *models.CarbonEstimate) *api.CarbonEstimate {
	if mc == nil {
		return nil
	}
	result := &api.CarbonEstimate{}
	if mc.Co2EGrams != nil {
		result.Co2EGrams = &api.Estimate{
			Mean:   mc.Co2EGrams.Mean,
			Stddev: mc.Co2EGrams.Stddev,
		}
	}
	if mc.Provenance != nil {
		result.Provenance = &api.Provenance{
			Source:    api.ProvenanceSource(mc.Provenance.Source),
			Name:      mc.Provenance.Name,
			Version:   mc.Provenance.Version,
			Reasoning: mc.Provenance.Reasoning,
			Sources:   mc.Provenance.Sources,
		}
	}
	return result
}

// scaleMoneySaved scales a MoneySavings by a multiplier.
func scaleMoneySaved(ms *api.MoneySavings, multiplier int32) *api.MoneySavings {
	if ms == nil || ms.ValueUsd == nil || multiplier <= 1 {
		return ms
	}
	return &api.MoneySavings{
		ValueUsd:   estimator.ScaleEstimate(ms.ValueUsd, float32(multiplier)),
		Provenance: ms.Provenance,
	}
}

// scaleEmissionsPrevented scales a PreventedEmissions by a multiplier.
func scaleEmissionsPrevented(pe *api.PreventedEmissions, multiplier int32) *api.PreventedEmissions {
	if pe == nil || multiplier <= 1 {
		return pe
	}
	result := &api.PreventedEmissions{
		Provenance: pe.Provenance,
	}
	if pe.ManufactureAvoidedCarbon != nil && pe.ManufactureAvoidedCarbon.Co2EGrams != nil {
		result.ManufactureAvoidedCarbon = &api.CarbonEstimate{
			Co2EGrams:  estimator.ScaleEstimate(pe.ManufactureAvoidedCarbon.Co2EGrams, float32(multiplier)),
			Provenance: pe.ManufactureAvoidedCarbon.Provenance,
		}
	}
	if pe.WasteReducedCarbon != nil && pe.WasteReducedCarbon.Co2EGrams != nil {
		result.WasteReducedCarbon = &api.CarbonEstimate{
			Co2EGrams:  estimator.ScaleEstimate(pe.WasteReducedCarbon.Co2EGrams, float32(multiplier)),
			Provenance: pe.WasteReducedCarbon.Provenance,
		}
	}
	return result
}
