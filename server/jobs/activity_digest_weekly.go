// Weekly activity digest job (#1924). On the configured weekday and
// local hour, the job:
//
//   1. Computes the prior ISO-week window in the configured timezone
//      (Monday 00:00 of the previous week through Monday 00:00 of the
//      current week).
//   2. Tries to claim "this week's send" via
//      storage.ClaimActivityDigestSend(CadenceWeekly, "YYYY-Www").
//      A losing claim is a no-op (another instance owns the send).
//   3. Builds the digest via activity_digest.Builder.BuildRange.
//   4. Hands the result to email.Service.SendActivityDigest, which
//      uses DigestPeriodWeekly to set the subject and banner.
//
// On a send failure the claim is NOT reset — same rationale as the
// daily job. The schedule is hourly + weekday-and-hour gate + claim,
// so a deploy that crosses Monday morning still produces exactly one
// send.

package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.ripls.org/ripls/server/activity_digest"
	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// ActivityWeeklyDigestJob sends the weekly activity digest email.
type ActivityWeeklyDigestJob struct {
	storage      *storage.ProtoSQLStorage
	emailService email.Service
	builder      *activity_digest.Builder
	recipient    string
	timezone     *time.Location
	sendDay      time.Weekday
	sendHour     int
	now          func() time.Time
}

// ActivityWeeklyDigestConfig captures the user-facing knobs of the
// weekly job.
type ActivityWeeklyDigestConfig struct {
	Recipient    string
	TimezoneName string
	SendWeekday  time.Weekday
	SendHour     int
}

// NewActivityWeeklyDigestJob constructs a weekly job.
func NewActivityWeeklyDigestJob(
	s *storage.ProtoSQLStorage,
	emailService email.Service,
	cfg ActivityWeeklyDigestConfig,
) (*ActivityWeeklyDigestJob, error) {
	if s == nil {
		return nil, errors.New("ActivityWeeklyDigestJob: storage is required")
	}
	if emailService == nil {
		return nil, errors.New("ActivityWeeklyDigestJob: email service is required")
	}
	if cfg.Recipient == "" {
		return nil, errors.New("ActivityWeeklyDigestJob: recipient is required")
	}
	if cfg.SendHour < 0 || cfg.SendHour > 23 {
		return nil, fmt.Errorf("ActivityWeeklyDigestJob: send hour must be 0..23, got %d", cfg.SendHour)
	}
	if cfg.SendWeekday < time.Sunday || cfg.SendWeekday > time.Saturday {
		return nil, fmt.Errorf("ActivityWeeklyDigestJob: send weekday out of range: %d", cfg.SendWeekday)
	}
	loc, err := time.LoadLocation(cfg.TimezoneName)
	if err != nil {
		return nil, fmt.Errorf("ActivityWeeklyDigestJob: invalid timezone %q: %w", cfg.TimezoneName, err)
	}
	return &ActivityWeeklyDigestJob{
		storage:      s,
		emailService: emailService,
		builder:      activity_digest.New(s, nil),
		recipient:    cfg.Recipient,
		timezone:     loc,
		sendDay:      cfg.SendWeekday,
		sendHour:     cfg.SendHour,
		now:          time.Now,
	}, nil
}

// Run performs one weekly-digest tick. Safe to call hourly: the
// weekday + hour gate plus the claim primitive together guarantee at
// most one send per local week.
func (j *ActivityWeeklyDigestJob) Run(ctx context.Context) error {
	nowLocal := j.now().In(j.timezone)
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ActivityWeeklyDigest",
		"timezone", j.timezone.String(),
		"send_weekday", j.sendDay.String(),
		"send_hour", j.sendHour,
		"local_weekday", nowLocal.Weekday().String(),
		"local_hour", nowLocal.Hour(),
		"recipient_email", logging.MaskEmail(j.recipient),
	)

	if nowLocal.Weekday() != j.sendDay || nowLocal.Hour() != j.sendHour {
		logger.DebugContext(ctx, "weekly digest tick: outside send window")
		return nil
	}

	// Window: the seven local days ending at midnight today.
	thisWeekStart := startOfDay(nowLocal)
	lastWeekStart := thisWeekStart.AddDate(0, 0, -7)
	year, week := lastWeekStart.ISOWeek()
	runKey := fmt.Sprintf("%04d-W%02d", year, week)
	logger = logger.With(
		"run_key", runKey,
		"window_local_start", lastWeekStart.Format("2006-01-02"),
		"window_local_end", thisWeekStart.Format("2006-01-02"),
	)

	claimed, err := j.storage.ClaimActivityDigestSend(ctx, storage.CadenceWeekly, runKey)
	if err != nil {
		logger.ErrorContext(ctx, "failed to claim weekly digest send", "error", err)
		return fmt.Errorf("ActivityWeeklyDigestJob: claim: %w", err)
	}
	if !claimed {
		logger.DebugContext(ctx, "weekly digest tick: claim lost; another instance owns this week's send")
		return nil
	}

	startTime := time.Now()
	logger.InfoContext(ctx, "weekly digest tick: claim won; building digest")
	digest, err := j.builder.BuildRange(ctx, lastWeekStart, thisWeekStart, email.DigestPeriodWeekly)
	if err != nil {
		logger.ErrorContext(ctx, "failed to build weekly digest",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return fmt.Errorf("ActivityWeeklyDigestJob: build: %w", err)
	}

	logger = logger.With(
		"window_start_unix_sec", digest.WindowStartUnixSec,
		"window_end_unix_sec", digest.WindowEndUnixSec,
		"new_user_count", len(digest.NewUsers),
		"interactive_signin_count", digest.InteractiveSignInCount,
		"active_user_count", digest.ActiveUserCount,
		"active_contributor_count", digest.ActiveContributorCount,
		"waitlist_signup_count", len(digest.WaitlistSignups),
		"password_reset_count", digest.PasswordResetCount,
		"community_count", len(digest.Communities),
		"total_community_event_count", digest.TotalMemberActionCount,
		"total_user_message_count", digest.TotalUserMessageCount,
	)

	if err := j.emailService.SendActivityDigest(ctx, j.recipient, digest); err != nil {
		// Same policy as the daily job: do not reset the claim on
		// send failure.
		logger.ErrorContext(ctx, "failed to send weekly digest",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return fmt.Errorf("ActivityWeeklyDigestJob: send: %w", err)
	}

	logger.InfoContext(ctx, "weekly digest sent",
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	// Retention for the user_active_day stamp table (#2665): keep
	// roughly a year of history. Runs after a successful send so a
	// prune failure never blocks the digest; the returned error is
	// logged by the scheduler and next week's tick retries.
	pruneBefore := j.now().AddDate(0, 0, -userActivityRetentionDays).Unix()
	pruned, err := j.storage.PruneUserActivityBefore(ctx, pruneBefore)
	if err != nil {
		logger.ErrorContext(ctx, "failed to prune user_active_day", "error", err)
		return fmt.Errorf("ActivityWeeklyDigestJob: prune user activity: %w", err)
	}
	logger.InfoContext(ctx, "pruned user activity stamps",
		"pruned_rows", pruned,
		"retention_days", userActivityRetentionDays,
	)
	return nil
}

// userActivityRetentionDays is how much user_active_day history the
// weekly prune keeps — a year plus headroom for year-over-year context.
const userActivityRetentionDays = 400

// startOfDay returns the local midnight that begins the day of t.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
