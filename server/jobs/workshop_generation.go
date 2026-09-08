package jobs

import (
	"context"
	"fmt"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services/workshop/momentum"
	"go.ripls.org/ripls/server/storage"
)

// WorkshopGenerationJob pre-warms Workshop Hero card and Bring-Back
// nudges for every (host, community) pair so a host opening the tab
// sees content immediately rather than paying generation latency.
//
// Per docs/ai/workshop.md Decision 8, the job **does not enqueue push
// notifications** — pre-warming the database is the whole point. The
// job consumes the workshop momentum subpackage directly rather than the
// Workshop service to avoid pulling in the Connect handlers it doesn't
// need.
type WorkshopGenerationJob struct {
	storage *storage.ProtoSQLStorage
}

// NewWorkshopGenerationJob constructs the job.
func NewWorkshopGenerationJob(store *storage.ProtoSQLStorage) *WorkshopGenerationJob {
	return &WorkshopGenerationJob{storage: store}
}

// Run walks the active (host, community) pairs and ensures each host
// has at least one Hero card materialized for each community they own
// experiences in. Per-host failures log and continue — one bad
// community doesn't block the rest.
//
// Returns the number of nudges newly persisted plus any unrecoverable
// error. Idempotency: the underlying detection check skips when an
// active Hero card already exists, so repeat runs are cheap.
func (j *WorkshopGenerationJob) Run(ctx context.Context) (int, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"job", "workshop_generation",
	)
	startTime := time.Now()
	logger.InfoContext(ctx, "starting workshop generation job")

	// Find every (owner, community) pair that has at least one
	// CommunityExperience or CommunityGear edge — those are the hosts
	// the Workshop tab can light up for.
	pairs, err := j.collectHostCommunityPairs(ctx)
	if err != nil {
		return 0, fmt.Errorf("collect host pairs: %w", err)
	}
	if len(pairs) == 0 {
		logger.InfoContext(ctx, "no host/community pairs to process")
		return 0, nil
	}

	persisted := 0
	failed := 0
	now := time.Now()
	for pair := range pairs {
		// Stop cleanly on shutdown rather than logging one warning per
		// remaining pair; main.go swallows the returned context.Canceled.
		if ctx.Err() != nil {
			return persisted, logBackfillCancelled(ctx, logger, "workshop_generation")
		}
		count, err := j.ensureForPair(ctx, pair.userID, pair.communityID, now)
		if err != nil {
			if isShutdownError(ctx, err) {
				return persisted, logBackfillCancelled(ctx, logger, "workshop_generation")
			}
			logger.WarnContext(ctx, "ensure for pair failed",
				"user_id", pair.userID,
				"community_id", pair.communityID,
				"error", err,
			)
			failed++
			continue
		}
		persisted += count
	}

	logger.InfoContext(ctx, "workshop generation job complete",
		"pairs_processed", len(pairs),
		"nudges_persisted", persisted,
		"failed_pairs", failed,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)
	return persisted, nil
}

type hostCommunityPair struct {
	userID, communityID string
}

// collectHostCommunityPairs returns the (owner, community) set
// derived from completed Experience rows joined to CommunityExperience.
// Drives generation off the host's actual hosting history; future
// iterations can union gear-owners + community organizers.
func (j *WorkshopGenerationJob) collectHostCommunityPairs(
	ctx context.Context,
) (map[hostCommunityPair]bool, error) {
	// Walk completed experiences; the (owner_id, community) pairs they
	// belong to are exactly the candidates the Workshop should
	// pre-warm. Bounded by `state == COMPLETED` so the query stays
	// indexed.
	experiencesRaw, err := j.storage.QueryByField(
		ctx,
		"state",
		int32(models.ExperienceState_EXPERIENCE_STATE_COMPLETED),
		&models.Experience{},
	)
	if err != nil {
		return nil, fmt.Errorf("query completed experiences: %w", err)
	}

	out := make(map[hostCommunityPair]bool)
	for _, raw := range experiencesRaw {
		// Bail on shutdown instead of running a failing join query per
		// remaining experience; the wrapped context.Canceled propagates up
		// through Run, which main.go swallows.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		exp := raw.(*models.Experience)
		if exp.Deleted != nil || exp.OwnerId == "" {
			continue
		}
		joinsRaw, err := j.storage.QueryByField(
			ctx,
			"experience_id",
			exp.Id,
			&models.CommunityExperience{},
		)
		if err != nil {
			if isShutdownError(ctx, err) {
				return nil, err
			}
			continue
		}
		for _, joinRaw := range joinsRaw {
			join := joinRaw.(*models.CommunityExperience)
			if join.CommunityId == "" {
				continue
			}
			out[hostCommunityPair{userID: exp.OwnerId, communityID: join.CommunityId}] = true
		}
	}
	return out, nil
}

// ensureForPair runs the cascade detector for one (user, community)
// pair and persists at most one Hero card if a slot fires. Returns
// the number of nudges newly persisted (0 or 1 in v1).
//
// Mirrors the on-demand path in `server/services/workshop` but lives
// here as part of the shared-library / job consumption pattern — the
// job does NOT inject the Workshop service.
func (j *WorkshopGenerationJob) ensureForPair(
	ctx context.Context,
	userID, communityID string,
	now time.Time,
) (int, error) {
	// Skip when an active Hero card already exists.
	existing, err := j.findActiveHeroCard(ctx, userID, communityID)
	if err != nil {
		return 0, fmt.Errorf("look up active hero card: %w", err)
	}
	if existing != nil {
		return 0, nil
	}

	det, err := momentum.DetectHighestPriority(
		ctx, j.storage, userID, communityID, momentum.DefaultDetectors(),
	)
	if err != nil {
		return 0, fmt.Errorf("detection: %w", err)
	}
	if det == nil {
		return 0, nil
	}

	det = momentum.NewCopyGenerator().Generate(ctx, det)
	if det == nil {
		return 0, nil
	}

	nudge, err := momentum.MaterializeDetection(det, userID, communityID, now.Unix())
	if err != nil {
		return 0, fmt.Errorf("materialize: %w", err)
	}
	if _, err := j.storage.Insert(ctx, nudge); err != nil {
		return 0, fmt.Errorf("insert: %w", err)
	}
	// This job runs at every server start with the template-only copy
	// generator — no AI pass ever rewrites what it persists — so whatever a
	// detector hardcodes is exactly what a host reads. Naming the slot makes
	// that answerable from the logs (#2892).
	logging.LoggerWithContext(ctx).InfoContext(ctx, "workshop hero card pre-warmed",
		"user_id", userID,
		"community_id", communityID,
		"slot", det.Slot.String(),
	)
	return 1, nil
}

// findActiveHeroCard returns the first non-consumed, non-deleted
// Hero card nudge for (user, community), or nil if none.
func (j *WorkshopGenerationJob) findActiveHeroCard(
	ctx context.Context,
	userID, communityID string,
) (*models.StoredNudge, error) {
	nudges, err := storage.QueryByFields[*models.StoredNudge](
		j.storage, ctx,
		map[string]any{
			"user_id":      userID,
			"community_id": communityID,
		},
	)
	if err != nil {
		return nil, err
	}
	for _, n := range nudges {
		if n.Surface != models.NudgeSurface_NUDGE_SURFACE_WORKSHOP_HERO_CARD {
			continue
		}
		if n.ConsumedAtUnixSec != nil || n.Deleted != nil {
			continue
		}
		return n, nil
	}
	return nil, nil
}
