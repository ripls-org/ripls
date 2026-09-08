// Daily activity digest job (#1924). Once per local day, in the
// configured timezone and send hour, the job:
//
//   1. Computes yesterday's [start, end) window in the configured TZ.
//   2. Tries to claim today's send via storage.ClaimActivityDigestSend.
//      A losing claim is a no-op (another instance owns the send).
//   3. Builds the digest via activity_digest.Builder.
//   4. Hands the result to email.Service.SendActivityDigest.
//
// On a send failure the claim is NOT reset: a transient Mailgun outage
// across two instances would otherwise produce a retry storm. The
// missed digest is logged; tomorrow's digest still ships.
//
// Schedule is hourly + same-day-claim: the job fires every hour, and
// only the run whose local-hour matches sendHour AND wins the claim
// actually sends. That keeps "send once per local day" robust to
// deploys that cross an hour boundary.

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

// ActivityDigestJob sends the daily activity digest email.
type ActivityDigestJob struct {
	storage      *storage.ProtoSQLStorage
	emailService email.Service
	builder      *activity_digest.Builder
	recipient    string
	timezone     *time.Location
	sendHour     int
	now          func() time.Time
}

// ActivityDigestConfig captures the user-facing knobs of the job.
type ActivityDigestConfig struct {
	Recipient    string
	TimezoneName string
	SendHour     int
}

// NewActivityDigestJob constructs a job. Returns an error when the
// configured timezone is invalid or the send hour is out of range.
func NewActivityDigestJob(
	s *storage.ProtoSQLStorage,
	emailService email.Service,
	cfg ActivityDigestConfig,
) (*ActivityDigestJob, error) {
	if s == nil {
		return nil, errors.New("ActivityDigestJob: storage is required")
	}
	if emailService == nil {
		return nil, errors.New("ActivityDigestJob: email service is required")
	}
	if cfg.Recipient == "" {
		return nil, errors.New("ActivityDigestJob: recipient is required")
	}
	if cfg.SendHour < 0 || cfg.SendHour > 23 {
		return nil, fmt.Errorf("ActivityDigestJob: send hour must be 0..23, got %d", cfg.SendHour)
	}
	loc, err := time.LoadLocation(cfg.TimezoneName)
	if err != nil {
		return nil, fmt.Errorf("ActivityDigestJob: invalid timezone %q: %w", cfg.TimezoneName, err)
	}
	return &ActivityDigestJob{
		storage:      s,
		emailService: emailService,
		builder:      activity_digest.New(s, nil),
		recipient:    cfg.Recipient,
		timezone:     loc,
		sendHour:     cfg.SendHour,
		now:          time.Now,
	}, nil
}

// Run performs one digest-tick. It is safe to call hourly: the local-
// hour gate and the claim primitive together guarantee at-most-one
// send per local day. Returns nil when nothing was sent (gate skipped
// or claim lost) — the caller cannot distinguish "didn't send because
// it wasn't time" from "didn't send because someone else got it,"
// which is fine because both are normal.
func (j *ActivityDigestJob) Run(ctx context.Context) error {
	nowLocal := j.now().In(j.timezone)
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ActivityDigest",
		"timezone", j.timezone.String(),
		"send_hour", j.sendHour,
		"local_hour", nowLocal.Hour(),
		"recipient_email", logging.MaskEmail(j.recipient),
	)

	if nowLocal.Hour() != j.sendHour {
		logger.DebugContext(ctx, "activity digest tick: outside send hour")
		return nil
	}

	// "Yesterday" in the configured TZ.
	yesterday := nowLocal.AddDate(0, 0, -1)
	// Today's date in TZ as YYYY-MM-DD — the claim moves forward
	// exactly once per local day.
	runDate := nowLocal.Format("2006-01-02")

	claimed, err := j.storage.ClaimActivityDigestSend(ctx, storage.CadenceDaily, runDate)
	if err != nil {
		logger.ErrorContext(ctx, "failed to claim activity digest send", "error", err, "run_date", runDate)
		return fmt.Errorf("ActivityDigestJob: claim: %w", err)
	}
	if !claimed {
		logger.DebugContext(ctx, "activity digest tick: claim lost; another instance owns today's send",
			"run_date", runDate,
		)
		return nil
	}

	startTime := time.Now()
	logger = logger.With("run_date", runDate, "window_local_date", yesterday.Format("2006-01-02"))
	logger.InfoContext(ctx, "activity digest tick: claim won; building digest")

	digest, err := j.builder.Build(ctx, yesterday)
	if err != nil {
		logger.ErrorContext(ctx, "failed to build activity digest",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return fmt.Errorf("ActivityDigestJob: build: %w", err)
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
		// Do NOT reset the claim — see file-level comment. Missing one
		// day's digest is far better than a retry storm.
		logger.ErrorContext(ctx, "failed to send activity digest",
			"error", err,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
		return fmt.Errorf("ActivityDigestJob: send: %w", err)
	}

	logger.InfoContext(ctx, "activity digest sent",
		"duration_ms", time.Since(startTime).Milliseconds(),
	)
	return nil
}
