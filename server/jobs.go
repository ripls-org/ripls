package main

// Background-job startup for the server binary: the workshop pre-warm, the
// periodic stats log lines, the community purge pair, the scheduled-
// notification reconciler/dispatcher, and the opt-in activity digests. Every
// loop runs through jobs.RunPeriodic (or logging.GoSafe for one-shots) and
// stops on root-context cancellation at shutdown.

import (
	"context"
	"errors"
	"time"

	"go.ripls.org/ripls/server/config"
	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/errs"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/jobs"
	schedulednotif "go.ripls.org/ripls/server/jobs/scheduled_notifications"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/services/user"
	"go.ripls.org/ripls/server/statslog"
	"go.ripls.org/ripls/server/storage"
)

// startBackgroundJobs launches every background goroutine the server runs:
// stats loggers, purge jobs, scheduled notifications, and digests. All stop
// when ctx is cancelled.
func startBackgroundJobs(ctx context.Context, cfg *config.Config, logger *logging.Logger, sqlStorage *storage.ProtoSQLStorage, notificationService notifications.Service, emailService email.Service) {
	// Pre-warm Workshop Hero card / Bring-Back nudges for every (host,
	// community) pair so a host opening the Workshop tab sees content
	// immediately. Per docs/ai/workshop.md Decision 8, the job does NOT
	// enqueue push notifications — pre-warming the database is the
	// whole point.
	logging.GoSafe(ctx, "workshop-generation-startup", func() {
		count, err := jobs.NewWorkshopGenerationJob(sqlStorage).Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("workshop generation job failed", "error", err)
			return
		}
		logger.Info(
			"workshop generation job complete",
			"nudges_persisted", count,
		)
	})

	// Periodic DB-pool and Go-runtime stats lines. Cloud Monitoring log-based
	// metrics extract their fields for the pool-saturation alert and the
	// runtime dashboard (#1613); messages, fields, and intervals are a
	// contract owned by server/statslog.
	statslog.StartPoolStatsLogger(ctx, logger, sqlStorage.PoolStats)
	statslog.StartRuntimeStatsLogger(ctx, logger)

	// Day-before-purge reminder for soft-deleted communities (#1659).
	// Runs once at startup (catch-up if the previous day's run was
	// missed by a deploy) and then every 24h. Idempotent across runs
	// via storage.ClaimCommunityPurgeReminder. The job is bounded by
	// PurgeReminderBatchSize per invocation so a long outage doesn't
	// produce runaway batches.
	purgeReminderJob := jobs.NewCommunityPurgeReminderJob(sqlStorage, notificationService)
	jobs.RunPeriodic(ctx, logger, jobs.Periodic{
		Name:       "community-purge-reminder",
		Interval:   24 * time.Hour,
		StartupMsg: "community purge reminder job failed at startup",
		TickMsg:    "community purge reminder job failed",
		Run:        purgeReminderJob.Run,
	})

	// Daily community purge job (#1620). Hard-deletes
	// soft-deleted communities (and their cascade) and
	// soft-deleted CommunityUser rows whose 30-day rejoin window
	// has expired. Runs once at startup as a catch-up for any
	// deploy that crossed a day boundary, then every 24h.
	// Idempotent because hard-deleted rows stay deleted —
	// repeated runs find zero candidates.
	purgeJob := jobs.NewCommunityPurgeJob(sqlStorage, cfg.CommunityPurgeDisabled, cfg.CommunityPurgeDryRun)
	jobs.RunPeriodic(ctx, logger, jobs.Periodic{
		Name:       "community-purge",
		Interval:   24 * time.Hour,
		StartupMsg: "community purge job failed at startup",
		TickMsg:    "community purge job failed",
		Run:        purgeJob.Run,
	})

	// Scheduled-notification reconciler and dispatcher (#625). The
	// reconciler computes desired-state rows from in-flight entities
	// and upserts/deletes; the dispatcher pulls due rows, applies
	// quiet hours, claims atomically, renders, and sends. Two
	// independent tickers so each cadence can be tuned separately.
	loanWorld := schedulednotif.NewStorageLoanWorld(sqlStorage, user.LoadUserNotificationPreferences)
	userPrefsForRecipient := func(ctx context.Context, userID string) (*models.UserNotificationPreferences, error) {
		return user.LoadUserNotificationPreferences(ctx, sqlStorage, userID)
	}
	scheduledReconcilers := []schedulednotif.Reconciler{
		schedulednotif.NewExperienceReminderReconciler(sqlStorage),
		schedulednotif.NewLoanReturnReminderReconciler(sqlStorage, loanWorld),
		schedulednotif.NewRequestFollowupReconciler(sqlStorage),
	}
	scheduledDispatchers := []schedulednotif.Dispatcher{
		schedulednotif.NewExperienceReminderDispatcher(sqlStorage),
		schedulednotif.NewExperienceClosePromptDispatcher(sqlStorage, userPrefsForRecipient),
		schedulednotif.NewLoanReturnReminderDispatcher(loanWorld),
		schedulednotif.NewRequestFollowupDispatcher(sqlStorage),
	}
	const scheduledReconcileInterval = 10 * time.Minute
	const scheduledDispatchInterval = 1 * time.Minute
	const scheduledDispatchBatchLimit = 200

	// The reconciler sweeps all registered reconcilers per tick with one
	// shared error streak and per-reconciler log fields, so it logs inside
	// Run and always returns nil to the runner.
	reconcileStreak := errs.NewTransientStreak()
	jobs.RunPeriodic(ctx, logger, jobs.Periodic{
		Name:     "scheduled-notifications-reconciler",
		Interval: scheduledReconcileInterval,
		Run: func(ctx context.Context) error {
			for _, r := range scheduledReconcilers {
				_, err := schedulednotif.Reconcile(ctx, sqlStorage, r)
				if errors.Is(err, context.Canceled) {
					continue
				}
				errs.LogJobError(ctx, logger, reconcileStreak, "scheduled-notification reconcile failed", err,
					"reconciler", r.Name())
			}
			return nil
		},
	})

	jobs.RunPeriodic(ctx, logger, jobs.Periodic{
		Name:       "scheduled-notifications-dispatcher",
		Interval:   scheduledDispatchInterval,
		StartupMsg: "scheduled-notification dispatch failed",
		TickMsg:    "scheduled-notification dispatch failed",
		Run: func(ctx context.Context) error {
			_, err := schedulednotif.Dispatch(ctx, sqlStorage, notificationService, scheduledDispatchers, scheduledDispatchBatchLimit)
			return err
		},
	})

	// Daily activity digest job (#1924). Off by default; opt in via
	// --activity-digest-enabled once the dev output looks right. Hourly
	// tick + same-day claim = at most one send per local day even across
	// instance restarts.
	if cfg.ActivityDigestEnabled {
		digestJob, err := jobs.NewActivityDigestJob(sqlStorage, emailService, jobs.ActivityDigestConfig{
			Recipient:    cfg.ActivityDigestNotifyEmail,
			TimezoneName: cfg.ActivityDigestTimezone,
			SendHour:     cfg.ActivityDigestSendHour,
		})
		if err != nil {
			logger.Error("failed to construct activity digest job; digest disabled", "error", err)
		} else {
			jobs.RunPeriodic(ctx, logger, jobs.Periodic{
				Name:       "activity-digest",
				Interval:   time.Hour,
				StartupMsg: "activity digest job failed at startup",
				TickMsg:    "activity digest job failed",
				Run:        digestJob.Run,
			})
		}
	}

	// Weekly activity digest job (#1924). Independent of the daily
	// switch — toggle separately. Hourly tick + weekday/hour gate +
	// per-ISO-week claim = at most one send per local week.
	if cfg.ActivityWeeklyDigestEnabled {
		weekday, err := config.ParseWeekday(cfg.ActivityWeeklyDigestWeekday)
		if err != nil {
			logger.Error(
				"failed to parse weekly digest weekday; weekly digest disabled",
				"error", err, "weekday_input", cfg.ActivityWeeklyDigestWeekday,
			)
		} else {
			weeklyJob, jerr := jobs.NewActivityWeeklyDigestJob(sqlStorage, emailService, jobs.ActivityWeeklyDigestConfig{
				Recipient:    cfg.ActivityDigestNotifyEmail,
				TimezoneName: cfg.ActivityDigestTimezone,
				SendWeekday:  weekday,
				SendHour:     cfg.ActivityWeeklyDigestSendHour,
			})
			if jerr != nil {
				logger.Error("failed to construct weekly activity digest job; weekly digest disabled", "error", jerr)
			} else {
				jobs.RunPeriodic(ctx, logger, jobs.Periodic{
					Name:       "activity-digest-weekly",
					Interval:   time.Hour,
					StartupMsg: "weekly activity digest job failed at startup",
					TickMsg:    "weekly activity digest job failed",
					Run:        weeklyJob.Run,
				})
			}
		}
	}
}
