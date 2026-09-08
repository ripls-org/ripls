package momentum

import (
	"fmt"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// SurfaceFor returns the NudgeSurface a detection should be persisted on.
// Hero card is the catch-all for the five cascade-priority slots; future
// detectors that intentionally emit a Bring-Back or seed-catalyst card can
// extend this mapping.
func SurfaceFor(slot Slot) models.NudgeSurface {
	switch slot {
	case SlotActiveQuest, SlotESMRepeatSignal, SlotCalendarGap,
		SlotSeasonalTrigger, SlotIdleOffer:
		return models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_HERO_CARD
	}
	return models.NudgeSurface_NUDGE_SURFACE_FEED
}

// MaterializeDetection builds a non-persisted StoredNudge from a Detection
// + (user, community) target. Pure function: caller decides whether to
// insert. Sets Surface, headlines, and provenance fields; leaves MediaId
// nil so the workshop service's renderable check (which is relaxed for
// workshop surfaces) accepts it without a stock-imagery round-trip.
func MaterializeDetection(
	det *Detection,
	userID, communityID string,
	createdAtUnixSec int64,
) (*models.StoredNudge, error) {
	if det == nil {
		return nil, fmt.Errorf("momentum.MaterializeDetection: detection is nil")
	}
	if det.CtaLabel == "" {
		return nil, fmt.Errorf("momentum.MaterializeDetection: detection has empty CtaLabel")
	}
	if err := ValidateLeverCopy(det.CtaLabel); err != nil {
		return nil, fmt.Errorf("momentum.MaterializeDetection: lever-copy invalid: %w", err)
	}

	nudge := &models.StoredNudge{
		Id:               uuid.NewString(),
		UserId:           userID,
		CommunityId:      communityID,
		Surface:          SurfaceFor(det.Slot),
		NudgeVariant:     1,
		Headline:         det.Headline,
		Description:      det.Description,
		CtaLabel:         det.CtaLabel,
		CtaAction:        det.CtaAction,
		CreatedAtUnixSec: createdAtUnixSec,
		StockQuery:       "", // workshop surfaces skip stock imagery in v1
	}
	if det.LocationHint != "" {
		hint := det.LocationHint
		nudge.LocationHint = &hint
	}
	if det.ContextID != "" {
		ctxID := det.ContextID
		nudge.ContextId = &ctxID
	}
	if det.KickerLabel != "" {
		k := det.KickerLabel
		nudge.KickerLabel = &k
	}
	if det.AtmosphereLine != "" {
		a := det.AtmosphereLine
		nudge.AtmosphereLine = &a
	}
	if det.EvidenceQuote != "" {
		nudge.Evidence = &models.StoredNudgeEvidence{
			Quote:       det.EvidenceQuote,
			Attribution: det.EvidenceAttribution,
		}
	}
	return nudge, nil
}
