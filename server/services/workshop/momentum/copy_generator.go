package momentum

import (
	"context"

	"go.ripls.org/ripls/server/logging"
)

// CopyGenerator finalizes the user-facing copy for a Detection —
// kicker, headline, atmosphere line, lever, and (optionally) evidence.
//
// The copy comes from the templates the detector already populated. An
// AI pass used to run first and overlay whatever it returned, with the
// templates as its fallback; #2936 removed it. The pass was already
// nearly vestigial — the scheduled pre-warm job
// (server/jobs/workshop_generation.go) has always persisted
// template-only copy, so the two paths could disagree about the same
// signal, and the AI prompt was fed the template copy as DETECTOR
// DEFAULTS and asked to "improve where you can".
//
// The lever word-count guard (ValidateLeverCopy) runs on the finalized
// copy. Detections whose lever exceeds 9 words are dropped so no
// non-conforming Hero card persists.
type CopyGenerator struct{}

// NewCopyGenerator returns a CopyGenerator.
func NewCopyGenerator() *CopyGenerator { return &CopyGenerator{} }

// Generate returns the copy-finalized Detection for persistence, or
// nil when the detection's lever fails the word-count guard.
func (g *CopyGenerator) Generate(ctx context.Context, det *Detection) *Detection {
	if det == nil {
		return nil
	}

	// Lever word-count guard. Drop the detection on failure — better to
	// render no Hero card than one with an overlong lever.
	if err := ValidateLeverCopy(det.CtaLabel); err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"copy generator: lever validation failed",
			"slot", det.Slot.String(),
			"context_id", det.ContextID,
			"cta_label", det.CtaLabel,
			"error", err,
		)
		return nil
	}

	return det
}
