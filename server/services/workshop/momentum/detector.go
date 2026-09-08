package momentum

import (
	"context"

	"go.ripls.org/ripls/server/storage"
)

// Slot identifies which Workshop cascade priority slot a detection fills.
// Values map directly to the priority order in the brief (1 highest, 5 lowest).
type Slot int

const (
	SlotUnspecified     Slot = iota
	SlotActiveQuest          // 1: an upcoming instance of a recurring activity
	SlotESMRepeatSignal      // 2: a recently-completed event with ≥60% repeat signal
	SlotCalendarGap          // 3: a recurring rhythm with detectable gaps
	SlotSeasonalTrigger      // 4: same date as last year's tradition

	// SlotIdleOffer (5) has no detector. The one that filled it claimed
	// "people keep asking about your {gear}" and "create a sharing event so
	// everyone can borrow it at once" without measuring either — it fired on
	// the mere existence of a listing, and its CTA passed a gear id to an
	// experience lookup, so every tap 404'd into a blank create modal
	// (#2892). The constant stays: Slot values are written to logs, removing
	// an iota member renumbers its neighbours, and a grounded offer signal
	// (Transfer state + latest_request_unix_sec) would land back here.
	SlotIdleOffer
)

// String returns a stable short name for the slot, suitable for logs.
func (s Slot) String() string {
	switch s {
	case SlotActiveQuest:
		return "active_quest"
	case SlotESMRepeatSignal:
		return "esm_repeat_signal"
	case SlotCalendarGap:
		return "calendar_gap"
	case SlotSeasonalTrigger:
		return "seasonal_trigger"
	case SlotIdleOffer:
		return "idle_offer"
	}
	return "unspecified"
}

// Detection is a single cascade slot match for a (user, community) pair.
//
// v1 detectors emit static copy alongside the structural signal so the
// pipeline can persist a complete StoredNudge without an AI round-trip.
// v2 will move copy generation to the AI provider; the Detection shape
// stays stable across both versions.
type Detection struct {
	// Which cascade slot this detection fills.
	Slot Slot

	// Pre-localized lever copy. Subject to ValidateLeverCopy on the
	// CtaLabel.
	Headline    string
	Description string
	CtaLabel    string

	// Action identifier the client uses to dispatch the lever tap. One of
	// "schedule_repeat", "revive_experience", "propose_share",
	// "seed_subhost" (#1579).
	CtaAction string

	// Stable identifier of the entity this detection refers to (an
	// experience id, a gear id, etc.). Stored on the resulting nudge for
	// the client to dispatch into the correct creation modal.
	ContextID string

	// Optional location hint surfaced on the Hero card.
	LocationHint string

	// Optional kicker label rendered above the headline (e.g.,
	// "On the way", "Worth doing again"). Empty when the detector
	// doesn't supply one.
	KickerLabel string

	// Optional atmosphere line rendered between headline and lever
	// (e.g., "Round 5 hosted by Mike · last Saturday at 9").
	AtmosphereLine string

	// Optional supporting quote + attribution rendered beneath the
	// narrative on calendar-gap / seasonal / idle-offer slots.
	EvidenceQuote       string
	EvidenceAttribution string
}

// Detector inspects a (user, community) pair and returns a Detection if
// the slot it owns matches the current state, or nil if not.
//
// Detectors must be pure functions over storage: no side effects, no
// hidden state. Errors are returned to the caller; the orchestrator logs
// and continues with the next detector so one failing detector does not
// block higher-priority slots.
type Detector interface {
	Slot() Slot
	Detect(ctx context.Context, store *storage.ProtoSQLStorage, userID, communityID string) (*Detection, error)
}

// DetectHighestPriority runs detectors in the order they appear and
// returns the first non-nil Detection. Pass detectors in cascade priority
// order; the orchestrator does not re-sort them.
//
// A nil return with nil error means no slot matched — the Workshop's
// empty-state behavior applies.
func DetectHighestPriority(
	ctx context.Context,
	store *storage.ProtoSQLStorage,
	userID, communityID string,
	detectors []Detector,
) (*Detection, error) {
	for _, d := range detectors {
		det, err := d.Detect(ctx, store, userID, communityID)
		if err != nil {
			// Per-detector failures don't block others — keep going.
			continue
		}
		if det != nil {
			return det, nil
		}
	}
	return nil, nil
}

// DefaultDetectors returns the standard cascade chain in priority order.
// Slots 1-4 have working implementations; SlotIdleOffer is deliberately
// unfilled (see its declaration above).
func DefaultDetectors() []Detector {
	return []Detector{
		ActiveQuestDetector{},
		ESMRepeatSignalDetector{},
		CalendarGapDetector{},
		SeasonalTriggerDetector{},
	}
}

// stubDetector always returns nil, nil. Used as a placeholder in
// DefaultDetectors() until the corresponding real detector lands.
type stubDetector struct {
	slot Slot
}

func (s stubDetector) Slot() Slot { return s.slot }
func (stubDetector) Detect(_ context.Context, _ *storage.ProtoSQLStorage, _, _ string) (*Detection, error) {
	return nil, nil
}
