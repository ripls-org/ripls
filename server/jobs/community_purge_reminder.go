// CommunityPurgeReminderJob dispatches the day-before-purge push
// reminder to snapshot members of communities approaching the
// 30-day soft-delete boundary. See
// docs/community_delete_and_leave.md §6.6 step 3 and #1659.
//
// The reminder is dispatched directly via notifications.Service.NotifyUser,
// bypassing both the notification dispatcher (which suppresses
// pushes for soft-deleted communities per #1622) and the per-user
// CommunityNotificationPreferences category gate (no §8 category
// for this push). Both bypasses are intentional: the reminder
// MUST fire for soft-deleted communities, and §8 lists no
// user-toggleable category.
//
// Idempotency is enforced by the storage layer's
// ClaimCommunityPurgeReminder conditional UPDATE — a community
// can only be reminded once per delete cycle. The flat
// purge_reminder_sent_at_unix_sec column is the audit record;
// no CommunityEvent row is written.

package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

// CommunityPurgeReminderJob fires the day-before-purge reminder
// push to snapshot members of soft-deleted communities crossing
// the 29-day mark.
type CommunityPurgeReminderJob struct {
	storage             *storage.ProtoSQLStorage
	notificationService notifications.Service
}

// NewCommunityPurgeReminderJob constructs a CommunityPurgeReminderJob.
func NewCommunityPurgeReminderJob(
	s *storage.ProtoSQLStorage, notif notifications.Service,
) *CommunityPurgeReminderJob {
	return &CommunityPurgeReminderJob{
		storage:             s,
		notificationService: notif,
	}
}

// Run finds soft-deleted communities crossing the 29-day mark and
// dispatches the day-before-purge reminder push to their snapshot
// members. Idempotent across runs and across server restarts.
func (j *CommunityPurgeReminderJob) Run(ctx context.Context) error {
	now := time.Now().Unix()
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CommunityPurgeReminderJob",
		"start_unix_sec", now,
		"lead_seconds", communitylib.PurgeReminderLeadSeconds,
		"batch_limit", communitylib.PurgeReminderBatchSize,
	)
	logger.InfoContext(ctx, "starting community purge reminder job")
	startTime := time.Now()

	candidates, err := j.storage.FindCommunitiesNeedingPurgeReminder(
		ctx, now,
		communitylib.PurgeReminderLeadSeconds,
		communitylib.PurgeReminderBatchSize,
	)
	if err != nil {
		return fmt.Errorf("find communities needing purge reminder: %w", err)
	}

	processed := 0
	pushesAttempted := 0
	pushesFailed := 0

	for _, community := range candidates {
		if err := ctx.Err(); err != nil {
			logger.InfoContext(ctx, "context cancelled mid-run", "error", err)
			return err
		}
		perCommunity := logger.With(
			"community_id", community.Id,
			"deleted_at_unix_sec", community.GetDeleted().GetDeletedAtUnixSec(),
		)

		claimed, err := j.storage.ClaimCommunityPurgeReminder(ctx, community.Id, now)
		if err != nil {
			perCommunity.WarnContext(ctx, "failed to claim community purge reminder; skipping", "error", err)
			continue
		}
		if !claimed {
			// Lost the race to another job runner — fine, that
			// runner is dispatching the pushes.
			perCommunity.DebugContext(ctx, "purge reminder claim lost; skipping")
			continue
		}
		processed++

		recipients := community.GetDeletedSnapshot().GetMemberUserIds()
		if len(recipients) == 0 {
			perCommunity.InfoContext(ctx, "purge reminder claim won; no snapshot members to notify",
				"recipient_count", 0,
			)
			continue
		}

		notification := j.buildReminderNotification(community)
		perCommunity.InfoContext(ctx, "dispatching purge reminder push",
			"recipient_count", len(recipients),
		)
		for _, userID := range recipients {
			pushesAttempted++
			if err := j.notificationService.NotifyUser(ctx, userID, notification); err != nil {
				pushesFailed++
				perCommunity.WarnContext(ctx, "failed to send purge reminder push to user",
					"user_id", userID,
					"error", err,
				)
			}
		}
	}

	logger.InfoContext(ctx, "community purge reminder job completed",
		"communities_processed", processed,
		"pushes_attempted", pushesAttempted,
		"pushes_failed", pushesFailed,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)
	return nil
}

// buildReminderNotification renders the §8 day-before-purge copy.
// Server-rendered English; localisation is a follow-up that should
// cover all five community-lifecycle pushes together (the existing
// four in notifications.go:buildNotification plus this one).
func (j *CommunityPurgeReminderJob) buildReminderNotification(community *models.Community) *models.Notification {
	return &models.Notification{
		Title: fmt.Sprintf("%s will be deleted tomorrow", community.Name),
		Body:  fmt.Sprintf("%s will be permanently deleted tomorrow. Tap to restore.", community.Name),
	}
}

// Run is a package-level convenience that constructs a job and
// runs it once. Mirrors the pattern in main.go for the other
// jobs that don't reuse their struct between invocations.
func RunCommunityPurgeReminder(
	ctx context.Context, s *storage.ProtoSQLStorage, notif notifications.Service,
) error {
	if err := NewCommunityPurgeReminderJob(s, notif).Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
