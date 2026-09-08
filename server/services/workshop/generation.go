package workshop

import (
	"context"
	"fmt"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services/workshop/momentum"
	"go.ripls.org/ripls/server/storage"
)

// detectorChain returns the detector chain to run for Workshop generation.
// Exposed as a package-level variable so tests can stub it out.
var detectorChain = momentum.DefaultDetectors

// copyGenerator returns the per-Service copy generator used to run
// detections through the validation guards. Centralized here so the
// per-event and drift paths share a single instance.
func (s *Service) copyGenerator() *momentum.CopyGenerator {
	return momentum.NewCopyGenerator()
}

// BringBackMaxRendered caps how many Bring-Something-Back items the
// Workshop persists per ensure pass. Mirrors momentum.BringBackMaxItems
// to keep the surfacing-discipline budget honest.
const BringBackMaxRendered = momentum.BringBackMaxItems

// EnsureHeroCard runs the per-event Hero card detection for
// (user, community) and persists one `StoredNudge` per recently
// completed event that doesn't already have an active Hero card,
// plus any drift Hero cards for non-event-anchored signals
// (calendar gap, seasonal trigger, idle offer).
//
// Returns the list of newly-created nudges (empty when every recent
// event already has an active card and no drift signal fires).
//
// Per docs/ai/workshop.md §3.1: every recently-completed event
// always surfaces *something* — the per-event detector falls back
// to a generic "Schedule {event} again?" card when no slot fires.
// Drift detections are deduped against the per-event set by
// `context_id` so the same experience never renders twice.
//
// This is the on-demand entry point: the WorkshopService calls it
// before reading workshop nudges. The scheduled
// `WorkshopGenerationJob` calls the same shared libraries
// independently to pre-warm the tab.
func (s *Service) EnsureHeroCard(
	ctx context.Context,
	userID, communityID string,
	now time.Time,
) ([]*models.StoredNudge, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "EnsureHeroCard",
		"user_id", userID,
		"community_id", communityID,
	)

	existing, err := s.findActiveHeroCards(ctx, userID, communityID)
	if err != nil {
		return nil, err
	}
	coveredBy := make(map[string]*models.StoredNudge, len(existing))
	for _, n := range existing {
		if n.ContextId != nil && *n.ContextId != "" {
			coveredBy[*n.ContextId] = n
		}
	}

	copyGen := s.copyGenerator()
	created := []*models.StoredNudge{}

	// 1. Per-event Hero cards: one for every recently completed event.
	//    Events whose existing nudge is a recap-fallback (no specific
	//    signal at the time of materialization) are re-detected so a
	//    later signal — e.g., ESM yes-votes that arrive after the event
	//    completed — can upgrade the card. Events already covered by a
	//    specific-signal nudge stay as-is.
	events, err := momentum.RecentlyCompletedEvents(
		ctx, s.storage, userID, communityID, now, momentum.RecentEventLookbackDays,
	)
	if err != nil {
		return nil, fmt.Errorf("EnsureHeroCard: recent events: %w", err)
	}
	for _, exp := range events {
		prior := coveredBy[exp.Id]
		if prior != nil && !isRecapFallbackNudge(prior) {
			continue
		}
		det, err := momentum.DetectForEvent(ctx, s.storage, userID, communityID, exp, now)
		if err != nil {
			logger.WarnContext(ctx, "per-event detect failed",
				"experience_id", exp.Id,
				"error", err,
			)
			continue
		}
		if det == nil {
			continue
		}
		// If we already have a recap-fallback for this experience and
		// the new detection is also a fallback, nothing to do — the
		// stored copy is still current.
		if prior != nil && det.KickerLabel == recapFallbackKicker {
			continue
		}
		// Upgrading: soft-delete the prior recap-fallback so the new
		// specific-signal nudge replaces it.
		if prior != nil {
			consumedAt := now.Unix()
			prior.ConsumedAtUnixSec = &consumedAt
			if err := s.storage.Update(ctx, prior); err != nil {
				logger.WarnContext(ctx, "consume prior recap-fallback failed",
					"prior_nudge_id", prior.Id,
					"experience_id", exp.Id,
					"error", err,
				)
				continue
			}
		}
		nudge, err := s.persistHeroCard(ctx, det, userID, communityID, now, copyGen, logger)
		if err != nil || nudge == nil {
			continue
		}
		created = append(created, nudge)
		if det.ContextID != "" {
			coveredBy[det.ContextID] = nudge
		}
	}

	// 2. Drift Hero cards: signals that aren't anchored to a recent
	//    event (calendar gap, seasonal anniversary). Run the cascade
	//    chain and dedup against per-event coverage by context_id.
	driftSlot := ""
	drift, err := momentum.DetectHighestPriority(
		ctx, s.storage, userID, communityID, detectorChain(),
	)
	if err != nil {
		logger.WarnContext(ctx, "drift detection failed", "error", err)
	} else if drift != nil && coveredBy[drift.ContextID] == nil {
		nudge, err := s.persistHeroCard(ctx, drift, userID, communityID, now, copyGen, logger)
		if err == nil && nudge != nil {
			created = append(created, nudge)
			driftSlot = drift.Slot.String()
		}
	}

	if len(created) > 0 {
		// `drift_slot` names which cascade slot fired, so "what are we
		// actually putting on people's Home screens" is answerable from
		// the logs. Before #2892 the only way to find out was a screenshot
		// in a feedback ticket. Add fields here, never reword the message:
		// log-based metrics match on message text (#1613).
		logger.InfoContext(ctx, "workshop hero cards generated",
			"count", len(created),
			"drift_slot", driftSlot,
		)
	}
	return created, nil
}

// recapFallbackKicker is the kicker label that
// `momentum.recapFallbackDetection` writes onto the recap-style
// fallback Hero card — the one materialized for a recently-completed
// experience when no specific signal (active-quest, ESM-repeat) has
// fired yet. EnsureHeroCard re-detects per-event slots whose stored
// nudge carries this kicker so a later signal (e.g., ESM yes-votes
// arriving after the experience completed) can upgrade the card.
const recapFallbackKicker = "Just wrapped"

// isRecapFallbackNudge reports whether `n` was materialized from
// `recapFallbackDetection`. The recap fallback fires for any
// recently-completed experience that has no more-specific signal at
// the moment EnsureHeroCard ran; later passes upgrade these to a
// specific-signal card when a signal becomes available.
func isRecapFallbackNudge(n *models.StoredNudge) bool {
	if n == nil || n.KickerLabel == nil {
		return false
	}
	return *n.KickerLabel == recapFallbackKicker
}

// persistHeroCard runs the detection through the copy generator
// (validation guards + optional AI tuning), materializes a
// StoredNudge, and inserts it. Returns the inserted nudge or nil
// when the guards reject the copy. Errors are returned to the
// caller which logs and continues with sibling events.
// contextMediaID returns the first media id of the experience a detection
// points at, for the nudge card to wear as its background.
//
// These prompts skip stock imagery — asking a stock provider for a picture of
// "schedule Wednesday morning run again" would be both wasteful and worse than
// what is already to hand: the run's own photo. A card that shows the thing it
// is asking about is recognisable at a glance, where a bare headline on black
// is not.
//
// Every surviving lever references an experience; the one that pointed at
// gear was deleted with its detector (#2892). A missing entity is not an
// error — the read is swallowed on purpose, and returning "" leaves the card
// on a solid background rather than failing materialization.
func (s *Service) contextMediaID(ctx context.Context, contextID string) string {
	if contextID == "" {
		return ""
	}
	exp := &models.Experience{}
	if err := s.storage.GetByID(ctx, contextID, exp); err != nil {
		return ""
	}
	if len(exp.MediaIds) > 0 {
		return exp.MediaIds[0]
	}
	return ""
}

func (s *Service) persistHeroCard(
	ctx context.Context,
	det *momentum.Detection,
	userID, communityID string,
	now time.Time,
	copyGen *momentum.CopyGenerator,
	logger interface {
		WarnContext(ctx context.Context, msg string, args ...any)
	},
) (*models.StoredNudge, error) {
	det = copyGen.Generate(ctx, det)
	if det == nil {
		return nil, nil
	}
	nudge, err := momentum.MaterializeDetection(det, userID, communityID, now.Unix())
	if err != nil {
		logger.WarnContext(ctx, "materialize failed",
			"context_id", det.ContextID, "error", err)
		return nil, err
	}
	if mediaID := s.contextMediaID(ctx, det.ContextID); mediaID != "" {
		nudge.MediaId = &mediaID
	}
	if _, err := s.storage.Insert(ctx, nudge); err != nil {
		logger.WarnContext(ctx, "insert failed",
			"context_id", det.ContextID, "error", err)
		return nil, err
	}
	return nudge, nil
}

// EnsureBringBackItems runs the Bring-Back orchestrator and persists any
// detected love-revival items as `WORKSHOP_BRING_BACK` nudges, capped at
// BringBackMaxRendered. Skips items whose contextID is already covered
// by an active workshop nudge (so the Quest hero and the Bring-Back row
// never duplicate the same experience).
//
// Returns the count of newly-persisted nudges. Like EnsureQuestHero, this
// Returns the count of newly-persisted nudges.
func (s *Service) EnsureBringBackItems(
	ctx context.Context,
	userID, communityID string,
	excludeContextID string,
	now time.Time,
) (int, error) {
	existing, err := s.activeBringBackContextIDs(ctx, userID, communityID)
	if err != nil {
		return 0, err
	}

	dets, err := momentum.DetectBringBackItems(
		ctx, s.storage, userID, communityID, excludeContextID, now,
	)
	if err != nil {
		return 0, fmt.Errorf("EnsureBringBackItems: detect: %w", err)
	}
	if len(dets) == 0 {
		return 0, nil
	}

	persisted := 0
	for _, det := range dets {
		if existing[det.CtaLabel] {
			continue
		}
		nudge, err := momentum.MaterializeDetection(&det, userID, communityID, now.Unix())
		if err != nil {
			logging.LoggerWithContext(ctx).WarnContext(ctx,
				"materialize bring-back failed",
				"context_id", det.ContextID, "error", err)
			continue
		}
		// Override the surface — MaterializeDetection routes everything to
		// HERO_CARD since detector slots map there by default. Bring-Back
		// items get their own surface so the curation step groups them.
		nudge.Surface = models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_BRING_BACK
		if _, err := s.storage.Insert(ctx, nudge); err != nil {
			return persisted, fmt.Errorf("EnsureBringBackItems: insert: %w", err)
		}
		persisted++
	}
	if persisted > 0 {
		logging.LoggerWithContext(ctx).InfoContext(ctx,
			"workshop bring-back items generated",
			"user_id", userID,
			"community_id", communityID,
			"count", persisted,
		)
	}
	return persisted, nil
}

// activeBringBackContextIDs returns the set of context IDs already
// referenced by active workshop nudges (Quest hero + Bring-Back) for the
// (user, community) pair. Used to dedupe across surfaces — the same
// experience never appears in both Quest hero and Bring-Back rows.
//
// Note: ContextID is stored implicitly via the stable ID format the
// nudge generation pipeline writes; for v1 we approximate by checking
// existing Bring-Back rows by the ID portion of the cta_label or by a
// future ContextID column. Today we just dedupe by `cta_label` exact
// match since each context produces a deterministic lever string.
func (s *Service) activeBringBackContextIDs(
	ctx context.Context,
	userID, communityID string,
) (map[string]bool, error) {
	out := map[string]bool{}
	nudges, err := storage.QueryByFields[*models.StoredNudge](s.storage, ctx,
		map[string]any{
			"user_id":      userID,
			"community_id": communityID,
		})
	if err != nil {
		return nil, fmt.Errorf("activeBringBackContextIDs: %w", err)
	}
	for _, n := range nudges {
		if n.Surface != models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_BRING_BACK {
			continue
		}
		if n.ConsumedAtUnixSec != nil || n.Deleted != nil {
			continue
		}
		out[n.CtaLabel] = true
	}
	return out, nil
}

// findActiveHeroCards returns the active (non-consumed, non-deleted) Hero
// card nudges for (user, community). When the per-event iteration lands,
// callers will dedup against the returned set by `context_id`; today the
// list is at most one element by construction.
func (s *Service) findActiveHeroCards(
	ctx context.Context,
	userID, communityID string,
) ([]*models.StoredNudge, error) {
	nudges, err := storage.QueryByFields[*models.StoredNudge](
		s.storage, ctx,
		map[string]any{
			"user_id":      userID,
			"community_id": communityID,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("findActiveHeroCards: %w", err)
	}
	var out []*models.StoredNudge
	for _, n := range nudges {
		if n.Surface != models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_HERO_CARD {
			continue
		}
		if n.ConsumedAtUnixSec != nil {
			continue
		}
		if n.Deleted != nil {
			continue
		}
		out = append(out, n)
	}
	return out, nil
}
