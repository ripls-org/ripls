package estimator

import (
	"fmt"
	"math"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/api"
)

// TransactionRole describes the user's role in a transaction for reciprocity scoring.
type TransactionRole int

const (
	// RoleGiving is the lender, giver, or helper role.
	RoleGiving TransactionRole = iota
	// RoleReceiving is the borrower or recipient role.
	RoleReceiving
	// RoleMutual is the role for shared experiences and collaborative requests
	// where both parties participate equally.
	RoleMutual
)

// ModalityType describes the communication channel for an interaction.
type ModalityType int

const (
	// ModalityInPersonShared is an in-person shared activity (e.g., potluck, workshop).
	ModalityInPersonShared ModalityType = iota
	// ModalityInPersonTransactional is a brief in-person exchange (e.g., gear handoff).
	ModalityInPersonTransactional
	// ModalityVideo is a remote interaction via video call.
	ModalityVideo
	// ModalityPhone is a remote interaction via phone or voice.
	ModalityPhone
	// ModalityText is a remote interaction via text or async messaging.
	ModalityText
)

// NoveltyLevel describes whether an interaction type is novel for a pair of users.
type NoveltyLevel int

const (
	// NoveltyNovel is the first interaction of this type between these users.
	NoveltyNovel NoveltyLevel = iota
	// NoveltyInfrequent is the same interaction type but last occurred >30 days ago.
	NoveltyInfrequent
	// NoveltyRoutine is the same interaction type that occurred within the last 30 days.
	NoveltyRoutine
)

// ClassifyModality returns the modality type for a transaction type.
// Experiences default to in-person shared; gear transactions to in-person transactional.
// Requests default to in-person transactional; future enrichment may infer
// modality from request description.
func ClassifyModality(txType TransactionType) ModalityType {
	switch txType {
	case TransactionExperienceConcluded:
		return ModalityInPersonShared
	default:
		return ModalityInPersonTransactional
	}
}

// ModalityWeight returns the weight for a modality type from config.
// The weight represents the fraction of full in-person shared activity value.
func ModalityWeight(modality ModalityType, cfg *Config) float32 {
	sf := cfg.QualityTime
	switch modality {
	case ModalityInPersonShared:
		return sf.GetModalityInPersonShared()
	case ModalityInPersonTransactional:
		return sf.GetModalityInPersonTransactional()
	case ModalityVideo:
		return sf.GetModalityVideo()
	case ModalityPhone:
		return sf.GetModalityPhone()
	case ModalityText:
		return sf.GetModalityText()
	default:
		return sf.GetModalityInPersonTransactional()
	}
}

// ClassifyGroupSize returns the group size factor for an attendee count.
// The factor represents the per-person wellbeing multiplier for the group size.
// Tiers: dyadic (2), small (3–5), medium (6–15), large (16+).
func ClassifyGroupSize(attendeeCount int32, cfg *Config) float32 {
	sf := cfg.QualityTime
	switch {
	case attendeeCount <= 2:
		return sf.GetGroupSizeDyadic()
	case attendeeCount <= 5:
		return sf.GetGroupSizeSmall()
	case attendeeCount <= 15:
		return sf.GetGroupSizeMedium()
	default:
		return sf.GetGroupSizeLarge()
	}
}

// ClassifyTieStrength returns the tie strength weight for a prior interaction count.
// Tiers: new (0), acquaintance (1–3), active (4–10), close (11+).
// Source: social_connection_metrics.md Attribute 4; Granovetter (1973); Dunbar (1992).
func ClassifyTieStrength(priorInteractions int32, cfg *Config) float32 {
	sf := cfg.QualityTime
	switch {
	case priorInteractions == 0:
		return sf.GetTieStrengthNew()
	case priorInteractions <= 3:
		return sf.GetTieStrengthAcquaintance()
	case priorInteractions <= 10:
		return sf.GetTieStrengthActive()
	default:
		return sf.GetTieStrengthClose()
	}
}

// ClassifyReciprocity returns the reciprocity multiplier for a transaction role.
// Giving > receiving (Dunn, Aknin & Norton, 2008); mutual exchange equals giving.
func ClassifyReciprocity(role TransactionRole, cfg *Config) float32 {
	sf := cfg.QualityTime
	switch role {
	case RoleGiving:
		return sf.GetReciprocityGiving()
	case RoleReceiving:
		return sf.GetReciprocityReceiving()
	case RoleMutual:
		return sf.GetReciprocityMutual()
	default:
		return sf.GetReciprocityGiving()
	}
}

// ClassifyNovelty returns the novelty multiplier based on interaction history.
// Novel (first of type), infrequent (last > 30 days ago), or routine (≤ 30 days).
// Source: Aron et al. (2000) — shared novel experiences produce stronger bonding.
func ClassifyNovelty(level NoveltyLevel, cfg *Config) float32 {
	sf := cfg.QualityTime
	switch level {
	case NoveltyNovel:
		return sf.GetNoveltyNovel()
	case NoveltyInfrequent:
		return sf.GetNoveltyInfrequent()
	case NoveltyRoutine:
		return sf.GetNoveltyRoutine()
	default:
		return sf.GetNoveltyInfrequent()
	}
}

// NoveltyFromHistory returns the NoveltyLevel for a pair of users based on their
// interaction history. isFirstOfType indicates no prior interaction of this type;
// daysSinceLast is only used when isFirstOfType is false.
func NoveltyFromHistory(isFirstOfType bool, daysSinceLast int32) NoveltyLevel {
	if isFirstOfType {
		return NoveltyNovel
	}
	if daysSinceLast > 30 {
		return NoveltyInfrequent
	}
	return NoveltyRoutine
}

// EstimateSocialDuration returns a duration estimate for a social interaction.
// Uses the LLM-inferred hint (durationMinutes > 0) with lower relative stddev, or falls
// back to the config default for the transaction type with higher relative stddev.
func EstimateSocialDuration(durationMinutes float32, txType TransactionType, cfg *Config) *api.Estimate {
	if durationMinutes > 0 {
		return RelativeUncertainty(durationMinutes, cfg.QualityTime.GetDurationLlmRelativeStddev())
	}
	return defaultDurationForType(txType, cfg)
}

// VulnerabilityFromLevel maps an LLM-classified vulnerability level string to its
// multiplier value. Recognised values: "high" (1.3×), "medium" (1.0×), "low" (0.8×).
// These are canonical research-based multipliers (trust literature).
// Returns 0 for unrecognised strings so callers can fall back to DefaultVulnerability.
func VulnerabilityFromLevel(level string) float32 {
	switch level {
	case "high":
		return 1.3
	case "medium":
		return 1.0
	case "low":
		return 0.8
	default:
		return 0 // caller should fall back to DefaultVulnerability
	}
}

// DefaultVulnerability returns the config default vulnerability multiplier for a
// transaction type. Used when no LLM inference is available. All defaults are 1.0
// (medium) except experiences, which default to 1.3 (high) because hosting at home
// carries higher personal vulnerability.
func DefaultVulnerability(txType TransactionType, cfg *Config) float32 {
	sf := cfg.QualityTime
	switch txType {
	case TransactionLoan:
		return sf.GetVulnerabilityGearLoan()
	case TransactionGiveaway:
		return sf.GetVulnerabilityGiveaway()
	case TransactionRequestFulfilled:
		return sf.GetVulnerabilityRequest()
	case TransactionExperienceConcluded:
		return sf.GetVulnerabilityExperience()
	default:
		return sf.GetVulnerabilityGearLoan()
	}
}

// EstimateGearHandoffDuration returns the duration estimate for a gear handoff.
// Uses the config default for the given transaction type; gearCategory is reserved for
// future per-item LLM inference but currently ignored.
func EstimateGearHandoffDuration(gearCategory string, txType TransactionType, cfg *Config) *api.Estimate {
	sf := cfg.QualityTime
	var minutes int32
	switch txType {
	case TransactionGiveaway:
		minutes = sf.GetGiveawayHandoffDurationMinutes()
	default:
		minutes = sf.GetGearHandoffDurationMinutes()
	}
	relStddev := sf.GetDurationRelativeStddev()
	_ = gearCategory // reserved for future LLM inference
	return RelativeUncertainty(float32(minutes), relStddev)
}

// defaultDurationForType returns the default duration estimate for a transaction type.
func defaultDurationForType(txType TransactionType, cfg *Config) *api.Estimate {
	sf := cfg.QualityTime
	var minutes int32
	switch txType {
	case TransactionLoan:
		minutes = sf.GetGearHandoffDurationMinutes()
	case TransactionGiveaway:
		minutes = sf.GetGiveawayHandoffDurationMinutes()
	case TransactionRequestFulfilled:
		minutes = sf.GetRequestDurationMinutes()
	case TransactionExperienceConcluded:
		minutes = sf.GetExperienceDurationMinutes()
	default:
		minutes = sf.GetGearHandoffDurationMinutes()
	}
	return RelativeUncertainty(float32(minutes), sf.GetDurationRelativeStddev())
}

// groupSizeRelativeStddev returns the appropriate group size relative stddev
// based on whether the transaction is an experience (higher attendance variance).
func groupSizeRelativeStddev(txType TransactionType, cfg *Config) float32 {
	sf := cfg.QualityTime
	if txType == TransactionExperienceConcluded {
		return sf.GetGroupSizeExperienceRelativeStddev()
	}
	return sf.GetGroupSizeOtherRelativeStddev()
}

// QualityTimeInput holds all per-transaction attributes for Quality Time estimation.
// Callers populate this struct and pass it to EstimateQualityTime.
type QualityTimeInput struct {
	// Duration is the estimated face-to-face co-presence in minutes.
	// If nil, the config default for TxType is used.
	Duration *api.Estimate

	// TxType is the transaction type, used to look up config defaults.
	TxType TransactionType

	// Modality is the communication channel for the interaction.
	Modality ModalityType

	// AttendeeCount is the number of participants (≥2; defaults to 2 for gear).
	AttendeeCount int32

	// PriorInteractions is the prior completed transaction count between these users.
	PriorInteractions int32

	// Role is the initiating user's transaction role.
	Role TransactionRole

	// Novelty is the novelty level for this interaction type between these users.
	Novelty NoveltyLevel

	// Vulnerability is the vulnerability multiplier.
	// If zero, DefaultVulnerability(TxType) is used.
	Vulnerability float32

	// VulnerabilityRelStddev overrides the config vulnerability relative stddev.
	// Set to cfg.QualityTime.GetVulnerabilityLlmRelativeStddev() when vulnerability
	// is LLM-inferred; leave zero to use the config default.
	VulnerabilityRelStddev float32
}

// EstimateQualityTime computes the Quality Time estimate from per-transaction
// attributes using the formula:
//
//	QT = duration × modality × group_size × tie_strength × reciprocity × novelty × vulnerability
//
// Belonging Minutes = duration × modality (co-presence weighted by channel quality).
// Trust Credits = vulnerability × reciprocity (depth of trust exercised).
//
// Uncertainty is propagated via quadrature across all stochastic attribute estimates.
// All deterministic attributes (tie strength, reciprocity, novelty) contribute only
// their configured relative stddev to the composite uncertainty.
func EstimateQualityTime(input QualityTimeInput, cfg *Config) *api.QualityTimeEstimate {
	sf := cfg.QualityTime
	provName := "quality_time_v1"

	// Resolve duration.
	duration := input.Duration
	if duration == nil || duration.Mean <= 0 {
		duration = defaultDurationForType(input.TxType, cfg)
	}
	if duration == nil || duration.Mean <= 0 {
		return nil
	}

	// Resolve modality weight.
	modalityW := ModalityWeight(input.Modality, cfg)
	if modalityW <= 0 {
		return nil
	}

	// Resolve group size factor (default to dyadic if not set).
	attendees := input.AttendeeCount
	if attendees < 2 {
		attendees = 2
	}
	groupSizeW := ClassifyGroupSize(attendees, cfg)

	// Resolve tie strength.
	tieStrengthW := ClassifyTieStrength(input.PriorInteractions, cfg)

	// Resolve reciprocity.
	reciprocityW := ClassifyReciprocity(input.Role, cfg)

	// Resolve novelty.
	noveltyW := ClassifyNovelty(input.Novelty, cfg)

	// Resolve vulnerability.
	vulnerabilityW := input.Vulnerability
	if vulnerabilityW <= 0 {
		vulnerabilityW = DefaultVulnerability(input.TxType, cfg)
	}

	// Compute QT mean: duration × all multipliers.
	qtMean := duration.Mean * modalityW * groupSizeW * tieStrengthW * reciprocityW * noveltyW * vulnerabilityW

	// Compute composite relative stddev via quadrature (each factor's variance contribution).
	// stddev_rel² = sum of (σ_factor / factor)² for each stochastic factor.
	durationRel := relStddev(duration)
	modalityRel := sf.GetModalityRelativeStddev()
	groupSizeRel := groupSizeRelativeStddev(input.TxType, cfg)
	tieStrengthRel := sf.GetTieStrengthRelativeStddev()
	noveltyRel := sf.GetNoveltyRelativeStddev()
	vulnerabilityRel := input.VulnerabilityRelStddev
	if vulnerabilityRel <= 0 {
		vulnerabilityRel = sf.GetVulnerabilityRelativeStddev()
	}
	// Reciprocity is deterministic (0 stddev).

	compositeRelStddev := float32(math.Sqrt(float64(
		sq(durationRel) +
			sq(modalityRel) +
			sq(groupSizeRel) +
			sq(tieStrengthRel) +
			sq(noveltyRel) +
			sq(vulnerabilityRel),
	)))

	qtEstimate := &api.Estimate{
		Mean:   qtMean,
		Stddev: qtMean * compositeRelStddev,
	}

	// Belonging Minutes = duration × modality (co-presence quality).
	belongingMean := duration.Mean * modalityW
	belongingRelStddev := float32(math.Sqrt(float64(sq(durationRel) + sq(modalityRel))))
	belongingEstimate := &api.Estimate{
		Mean:   belongingMean,
		Stddev: belongingMean * belongingRelStddev,
	}

	// Trust Credits = vulnerability × reciprocity.
	trustMean := vulnerabilityW * reciprocityW
	trustEstimate := &api.Estimate{
		Mean:   trustMean,
		Stddev: trustMean * vulnerabilityRel, // reciprocity is deterministic
	}

	reasoning := fmt.Sprintf(
		"QT=%.1f: %.0fmin × modality %.1f × group %.1f × tie %.1f × reciprocity %.1f × novelty %.1f × vuln %.1f",
		qtMean, duration.Mean, modalityW, groupSizeW, tieStrengthW, reciprocityW, noveltyW, vulnerabilityW,
	)

	return &api.QualityTimeEstimate{
		QualityTimeMinutes: qtEstimate,
		BelongingMinutes:   belongingEstimate,
		TrustCredits:       trustEstimate,
		Provenance: &api.Provenance{
			Source:    api.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
			Name:      provName,
			Version:   cfg.ProvenanceVersion(provName),
			Reasoning: proto.String(reasoning),
			Sources:   []string{"social_connection_metrics.md — attribute weights and research references"},
		},
	}
}

// relStddev returns the relative stddev of an Estimate (stddev/mean), or 0 if mean is 0.
func relStddev(e *api.Estimate) float32 {
	if e == nil || e.Mean == 0 {
		return 0
	}
	return e.Stddev / e.Mean
}

// sq returns the square of a float32.
func sq(x float32) float32 {
	return x * x
}
