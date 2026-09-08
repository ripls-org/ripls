// CommunityPurgeJob is the daily hard-delete job for #1620
// (follow-up to #1619). It runs in two stages:
//
//  1. Purge expired communities: any Community row whose
//     deleted_deleted_at_unix_sec is older than
//     communitylib.PurgeWindowSeconds is hard-deleted along with
//     its full cascade (chat messages and conversations,
//     stories, nudges, feed-item views, community_user,
//     community_gear, community_request, community_experience,
//     community_notification_preferences,
//     community_invitation_link, community_event,
//     community_region). Per #1620's safeguards, each
//     community's cascade runs inside a single transaction —
//     either every join row goes or none of them do.
//
//  2. Purge expired memberships: any CommunityUser row whose
//     deleted_deleted_at_unix_sec is older than
//     communitylib.RejoinWindowSeconds is hard-deleted (rejoin
//     window expired). The parent community may still be
//     active.
//
// Five safeguards (see docs/issues/1620-community-purge.md):
//
//  1. Threshold gate. Aborts the run if the candidate count
//     exceeds min(100, max(5, 5% of total communities)). One bug
//     that mass-flips communities to soft-deleted state cannot
//     cascade into mass hard-delete.
//  2. Dry-run mode (--community-purge-dry-run flag). Runs the
//     cascade inside a transaction that is always rolled back.
//     Logs include `dry_run=true`. Default false.
//  3. Kill switch (--community-purge-disabled flag). Returns
//     immediately at the top of every Run.
//  4. Audit row. Inserted before the cascade inside the
//     same transaction; survives the hard delete and carries
//     per-table row counts for forensic reconstruction.
//  5. Per-row re-check. Immediately before the cascade, the
//     orchestrator re-fetches the community FOR UPDATE and
//     verifies it is still soft-deleted past the purge window.
//     Catches the race where a RestoreCommunity slipped in
//     between candidate selection and cascade.

package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// thresholdAbsoluteFloor and thresholdPercent define the
// abort-if-too-many threshold (Safeguard #1).
//
//	threshold = min(CommunityPurgeBatchSize,
//	                max(thresholdAbsoluteFloor,
//	                    thresholdPercent * total))
//
// The absolute floor protects tiny databases where 5% rounds to
// zero. The percent ceiling caps explosive growth at large scale.
const (
	thresholdAbsoluteFloor = 5
	thresholdPercent       = 0.05
)

// CommunityPurgeJob orchestrates the daily community purge.
//
// Disabled and DryRun are wired from server flags
// (--community-purge-disabled and --community-purge-dry-run)
// rather than environment variables — see Safeguards #2 and #3
// in docs/issues/1620-community-purge.md. Tests construct the
// job directly with the desired booleans.
type CommunityPurgeJob struct {
	storage  *storage.ProtoSQLStorage
	disabled bool
	dryRun   bool
}

// NewCommunityPurgeJob constructs a CommunityPurgeJob.
//
// disabled is the kill switch (Safeguard #3): when true, every
// Run returns immediately without touching the database.
//
// dryRun is the cascade dry-run (Safeguard #2): when true, the
// cascade runs inside a transaction that is always rolled back,
// so per-table counts can be observed in logs without actually
// deleting anything.
func NewCommunityPurgeJob(s *storage.ProtoSQLStorage, disabled, dryRun bool) *CommunityPurgeJob {
	return &CommunityPurgeJob{
		storage:  s,
		disabled: disabled,
		dryRun:   dryRun,
	}
}

// Run is the daily entry point. Idempotent across runs and
// across server restarts. Returns the first error encountered;
// per-community failures are logged and the run continues so a
// single bad row cannot block the rest of the batch.
func (j *CommunityPurgeJob) Run(ctx context.Context) error {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "PurgeCommunityRun",
	)

	if j.disabled {
		logger.InfoContext(ctx, "community purge disabled via flag; skipping",
			"disabled", true,
		)
		return nil
	}
	dryRun := j.dryRun
	startTime := time.Now()

	communitiesPurged, errCount, err := j.purgeExpiredCommunities(ctx, dryRun)
	if err != nil {
		// purgeExpiredCommunities only returns a top-level error
		// for the threshold-gate abort or a candidate-fetch
		// failure. Both are surfaced as the run's error, but the
		// membership purge below still runs — they're
		// independent.
		logger.ErrorContext(ctx, "community purge step failed",
			"error", err,
			"dry_run", dryRun,
		)
	}

	membershipsPurged, mErr := j.purgeExpiredMemberships(ctx, dryRun)
	if mErr != nil {
		logger.ErrorContext(ctx, "membership purge step failed",
			"error", mErr,
			"dry_run", dryRun,
		)
		if err == nil {
			err = mErr
		}
	}

	logger.InfoContext(ctx, "community purge run completed",
		"communities_purged", communitiesPurged,
		"memberships_purged", membershipsPurged,
		"error_count", errCount,
		"duration_ms", time.Since(startTime).Milliseconds(),
		"dry_run", dryRun,
	)
	return err
}

// purgeExpiredCommunities runs Stage 1. Returns
// (communitiesPurged, errorCount, fatalError). fatalError is
// non-nil only when the threshold gate aborts or the candidate
// fetch itself fails — per-community errors are counted but do
// not abort the batch.
func (j *CommunityPurgeJob) purgeExpiredCommunities(
	ctx context.Context, dryRun bool,
) (int, int, error) {
	now := time.Now().Unix()

	candidates, err := j.storage.FindExpiredCommunities(
		ctx, now, communitylib.PurgeWindowSeconds, communitylib.CommunityPurgeBatchSize,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("FindExpiredCommunities: %w", err)
	}
	if len(candidates) == 0 {
		return 0, 0, nil
	}

	// Safeguard #1: threshold gate.
	total, err := j.storage.CountCommunities(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("CountCommunities: %w", err)
	}
	threshold := computeThreshold(total)
	if len(candidates) > threshold {
		logging.LoggerWithContext(ctx).ErrorContext(ctx,
			"community purge aborted: candidate count exceeds threshold",
			"operation", "PurgeCommunityRun",
			"candidate_count", len(candidates),
			"total_communities", total,
			"threshold", threshold,
			"aborted", true,
			"reason", "threshold_exceeded",
			"dry_run", dryRun,
		)
		return 0, 0, fmt.Errorf("threshold exceeded: %d candidates > threshold %d (total=%d)", len(candidates), threshold, total)
	}

	purged := 0
	errCount := 0
	for _, c := range candidates {
		if err := ctx.Err(); err != nil {
			logging.LoggerWithContext(ctx).InfoContext(ctx,
				"community purge cancelled mid-run",
				"operation", "PurgeCommunityRun",
				"error", err,
			)
			return purged, errCount, err
		}
		err := j.purgeCommunity(ctx, c, dryRun, now)
		switch {
		case err == nil:
			// Includes both "actually purged" and "dry-run
			// rolled back". Both ran the full cascade, so the
			// counter reflects work attempted regardless of
			// commit outcome.
			purged++
		case errors.Is(err, errSkipRestoreRace):
			// Safeguard #5 caught a restore race; not
			// counted as purged or errored. Already logged
			// inside purgeCommunity.
		default:
			errCount++
		}
	}
	return purged, errCount, nil
}

// purgeCommunity runs the cascade for a single community inside
// a transaction. All five safeguards apply per-community: the
// re-check (5), the audit insert (4), the cascade in
// child-before-parent order, and the dry-run rollback (2).
func (j *CommunityPurgeJob) purgeCommunity(
	ctx context.Context, candidate *models.Community, dryRun bool, nowUnixSec int64,
) error {
	startTime := time.Now()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "PurgeCommunity",
		"community_id", candidate.Id,
		"deleted_at_unix_sec", candidate.GetDeleted().GetDeletedAtUnixSec(),
		"dry_run", dryRun,
	)

	counts := make(map[string]int64)

	txErr := j.storage.WithTx(ctx, nil, func(tx *storage.ProtoSQLStorage) error {
		// Safeguard #5: per-row re-check. SELECT FOR UPDATE the
		// community row inside the tx and verify it is still
		// soft-deleted past the purge window. Catches a
		// concurrent RestoreCommunity that landed between
		// candidate selection and now.
		fresh := &models.Community{}
		if err := tx.GetByID(ctx, candidate.Id, fresh, storage.QueryOptions{IncludeDeleted: true}); err != nil {
			return fmt.Errorf("re-check GetByID: %w", err)
		}
		if fresh.GetDeleted() == nil || fresh.GetDeleted().GetDeletedAtUnixSec() <= 0 {
			logger.WarnContext(ctx, "community purge skipped: restore race",
				"restore_race", true,
			)
			return errSkipRestoreRace
		}
		if fresh.GetDeleted().GetDeletedAtUnixSec() >= nowUnixSec-communitylib.PurgeWindowSeconds {
			// Edge case: the soft-delete was bumped forward
			// since candidate selection. The base logger
			// already carries deleted_at_unix_sec from the
			// candidate snapshot; report the fresh value
			// under a distinct key for the diff.
			logger.WarnContext(ctx, "community purge skipped: deleted_at moved forward",
				"restore_race", true,
				"fresh_deleted_at_unix_sec", fresh.GetDeleted().GetDeletedAtUnixSec(),
			)
			return errSkipRestoreRace
		}

		// Run cascade helpers in child-before-parent order.
		// Each helper is idempotent (DeleteByField with no
		// matches returns 0). Counts feed the audit row +
		// per-community log line.
		var rows int64
		var err error

		if rows, err = storage.HardDeleteChatMessagesByCommunityID(ctx, tx, candidate.Id); err != nil {
			return fmt.Errorf("chat_message: %w", err)
		}
		counts["chat_message"] = rows

		if rows, err = storage.HardDeleteChatConversationsByCommunityID(ctx, tx, candidate.Id); err != nil {
			return fmt.Errorf("chat_conversation: %w", err)
		}
		counts["chat_conversation"] = rows

		if rows, err = storage.HardDeleteStoriesByCommunityID(ctx, tx, candidate.Id); err != nil {
			return fmt.Errorf("story: %w", err)
		}
		counts["Story"] = rows

		if rows, err = storage.HardDeleteStoredNudgesByCommunityID(ctx, tx, candidate.Id); err != nil {
			return fmt.Errorf("stored_nudge: %w", err)
		}
		counts["stored_nudge"] = rows

		if rows, err = storage.HardDeleteFeedItemViewsByCommunityID(ctx, tx, candidate.Id); err != nil {
			return fmt.Errorf("FeedItemView: %w", err)
		}
		counts["FeedItemView"] = rows

		if rows, err = storage.HardDeleteCommunityUserByCommunityID(ctx, tx, candidate.Id); err != nil {
			return fmt.Errorf("community_user: %w", err)
		}
		counts["community_user"] = rows

		if rows, err = storage.HardDeleteCommunityGearByCommunityID(ctx, tx, candidate.Id); err != nil {
			return fmt.Errorf("community_gear: %w", err)
		}
		counts["community_gear"] = rows

		if rows, err = storage.HardDeleteCommunityRequestByCommunityID(ctx, tx, candidate.Id); err != nil {
			return fmt.Errorf("community_request: %w", err)
		}
		counts["community_request"] = rows

		if rows, err = storage.HardDeleteCommunityExperienceByCommunityID(ctx, tx, candidate.Id); err != nil {
			return fmt.Errorf("community_experience: %w", err)
		}
		counts["community_experience"] = rows

		if rows, err = storage.HardDeleteCommunityNotificationPreferencesByCommunityID(ctx, tx, candidate.Id); err != nil {
			return fmt.Errorf("community_notification_preferences: %w", err)
		}
		counts["community_notification_preferences"] = rows

		if rows, err = storage.HardDeleteCommunityInvitationLinkByCommunityID(ctx, tx, candidate.Id); err != nil {
			return fmt.Errorf("community_invitation_link: %w", err)
		}
		counts["community_invitation_link"] = rows

		if rows, err = storage.HardDeleteCommunityEventByCommunityID(ctx, tx, candidate.Id); err != nil {
			return fmt.Errorf("community_event: %w", err)
		}
		counts["community_event"] = rows

		if rows, err = storage.HardDeleteCommunityRegionByCommunityID(ctx, tx, candidate.Id); err != nil {
			return fmt.Errorf("community_region: %w", err)
		}
		counts["community_region"] = rows

		if rows, err = storage.HardDeleteCommunityByID(ctx, tx, candidate.Id); err != nil {
			return fmt.Errorf("community: %w", err)
		}
		counts["community"] = rows

		// Safeguard #4: audit row. Insert AFTER the cascade so
		// per_table_row_counts is populated; this still happens
		// inside the same tx so commit-or-rollback is atomic
		// with the cascade.
		audit := &models.CommunityPurgeAudit{
			CommunityId:           candidate.Id,
			Name:                  candidate.Name,
			Description:           candidate.Description,
			DeletedAtUnixSec:      candidate.GetDeleted().GetDeletedAtUnixSec(),
			PurgedAtUnixSec:       nowUnixSec,
			SnapshotMemberUserIds: candidate.GetDeletedSnapshot().GetMemberUserIds(),
			PerTableRowCounts:     counts,
		}
		if _, err := tx.Insert(ctx, audit); err != nil {
			return fmt.Errorf("audit insert: %w", err)
		}

		// Safeguard #2: dry-run rollback. Returning a sentinel
		// here triggers the deferred Rollback in WithTx;
		// nothing is actually committed.
		if dryRun {
			return errDryRunRollback
		}
		return nil
	})

	if errors.Is(txErr, errSkipRestoreRace) {
		// Logged inside the tx. Return the sentinel so the
		// caller can distinguish skipped from purged for the
		// summary counters; treat-as-error tests use
		// errors.Is.
		return errSkipRestoreRace
	}
	if errors.Is(txErr, errDryRunRollback) {
		// Dry-run path. Log the per-table counts the cascade
		// *would* have produced; the rollback already fired.
		args := append([]any{"duration_ms", time.Since(startTime).Milliseconds()}, countsAsLogArgs(counts)...)
		logger.InfoContext(ctx, "community purge dry-run completed", args...)
		return nil
	}
	if txErr != nil {
		logger.ErrorContext(ctx, "community purge failed",
			"error", txErr,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return txErr
	}

	args := append([]any{"duration_ms", time.Since(startTime).Milliseconds()}, countsAsLogArgs(counts)...)
	logger.InfoContext(ctx, "community purge completed", args...)
	return nil
}

// purgeExpiredMemberships runs Stage 2: hard-delete soft-deleted
// CommunityUser rows whose 30-day rejoin window has expired.
// Bounded by CommunityPurgeBatchSize. Single bulk DELETE.
func (j *CommunityPurgeJob) purgeExpiredMemberships(
	ctx context.Context, dryRun bool,
) (int, error) {
	startTime := time.Now()
	now := time.Now().Unix()

	ids, err := j.storage.FindExpiredCommunityUserIDs(
		ctx, now, communitylib.RejoinWindowSeconds, communitylib.CommunityPurgeBatchSize,
	)
	if err != nil {
		return 0, fmt.Errorf("FindExpiredCommunityUserIDs: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "PurgeMemberships",
		"candidate_count", len(ids),
		"dry_run", dryRun,
	)

	if dryRun {
		logger.InfoContext(ctx, "membership purge dry-run completed",
			"would_delete", len(ids),
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return 0, nil
	}

	// Use WithTx so the bulk delete plays nicely with future
	// concurrent runners and matches the audit story shape — a
	// future enhancement could add per-membership audit rows
	// here too.
	var rowsDeleted int64
	if err := j.storage.WithTx(ctx, nil, func(tx *storage.ProtoSQLStorage) error {
		var err error
		// Bulk delete. CommunityUser is keyed by id, not
		// community_id, so we use DeleteByFieldIn(id IN ...).
		// Actually use the storage primitive directly: we need
		// the rows-affected count.
		rowsDeleted, err = tx.DeleteByFieldIn(ctx, "community_user", "id", ids)
		return err
	}); err != nil {
		logger.ErrorContext(ctx, "membership purge failed",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return 0, err
	}

	logger.InfoContext(ctx, "membership purge completed",
		"rows_deleted", rowsDeleted,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)
	return int(rowsDeleted), nil
}

// computeThreshold returns the abort threshold for the
// community-purge candidate count: min(CommunityPurgeBatchSize,
// max(thresholdAbsoluteFloor, thresholdPercent * total)).
func computeThreshold(total int) int {
	pct := int(float64(total) * thresholdPercent)
	floor := thresholdAbsoluteFloor
	if pct > floor {
		floor = pct
	}
	if floor > communitylib.CommunityPurgeBatchSize {
		return communitylib.CommunityPurgeBatchSize
	}
	return floor
}

// countsAsLogArgs flattens the per-table counts map into the
// alternating key/value pairs slog wants. Suffixes every key
// with "_rows_deleted" so log queries like
// `jsonPayload.gear_rows_deleted > 0` work uniformly.
func countsAsLogArgs(counts map[string]int64) []any {
	args := make([]any, 0, len(counts)*2)
	for table, rows := range counts {
		args = append(args, table+"_rows_deleted", rows)
	}
	return args
}

// errSkipRestoreRace and errDryRunRollback are sentinel errors
// used only inside WithTx callbacks to roll back the
// transaction and signal the caller to log a specific outcome
// rather than treating the rollback as a failure.
var (
	errSkipRestoreRace = errors.New("purge skipped: restore race")
	errDryRunRollback  = errors.New("dry-run rollback")
)

// RunCommunityPurge is a package-level convenience that
// constructs a job and runs it once. Mirrors
// RunCommunityPurgeReminder. Defaults to disabled=false and
// dryRun=false; production callers in main.go construct the job
// with explicit flag values.
func RunCommunityPurge(ctx context.Context, s *storage.ProtoSQLStorage) error {
	if err := NewCommunityPurgeJob(s, false, false).Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
