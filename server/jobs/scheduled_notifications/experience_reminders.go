package scheduled_notifications

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/clock"
	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications/notification_content"
	"go.ripls.org/ripls/server/rsvpstate"
	"go.ripls.org/ripls/server/storage"
)

// Pre-event reminder offsets, relative to the Experience's start
// time (negative = before). Changing one of these is a policy edit:
// the next reconciler tick will UPDATE existing rows' fire_at in
// place so the new policy takes effect for every in-flight
// Experience without a migration.
const (
	// OffsetReminderDayBefore fires 24h before start.
	OffsetReminderDayBefore int64 = -24 * 3600
	// OffsetReminderTwoHour fires 2h before start.
	OffsetReminderTwoHour int64 = -2 * 3600
	// OffsetReminderStarting fires at start time.
	OffsetReminderStarting int64 = 0
	// OffsetClosePromptAfterStart fires 24h after start time, to the
	// host only, prompting them to mark the Experience completed.
	OffsetClosePromptAfterStart int64 = 24 * 3600
)

// experienceReminderOffsets is the canonical list of pre-event slots
// the reconciler emits per attendee. Order does not matter — the
// reconciler keys uniqueness by purpose+offset.
var experienceReminderOffsets = []int64{
	OffsetReminderDayBefore,
	OffsetReminderTwoHour,
	OffsetReminderStarting,
}

// ExperienceReader is the subset of storage the experience-reminder
// reconciler and dispatcher need. Defined as an interface so tests
// can pass a lightweight fake without spinning up Postgres.
type ExperienceReader interface {
	ListExperiences(ctx context.Context) ([]*models.Experience, error)
	ListRSVPsForExperience(ctx context.Context, experienceID string) ([]*models.ExperienceRSVP, error)
	GetExperience(ctx context.Context, experienceID string) (*models.Experience, error)
	// GetCommunityNotificationPreferences returns the recipient's
	// preference row for the given community, or nil if no row exists
	// (treated as "all categories on" per CommunityNotificationPreferences
	// semantics). Returns an error only on storage failure.
	GetCommunityNotificationPreferences(ctx context.Context, userID, communityID string) (*models.CommunityNotificationPreferences, error)
}

// ExperienceReminderReconciler emits desired-state tuples for the
// REMINDER_BEFORE_START purpose at the three policy-defined offsets,
// one per RSVP=YES/MAYBE attendee per in-flight Experience.
type ExperienceReminderReconciler struct {
	world    ExperienceReader
	existing func(ctx context.Context) ([]*models.ScheduledNotification, error)
}

// NewExperienceReminderReconciler wires the reconciler against a
// real storage layer.
func NewExperienceReminderReconciler(s *storage.ProtoSQLStorage) *ExperienceReminderReconciler {
	return &ExperienceReminderReconciler{
		world:    &storageExperienceReader{storage: s},
		existing: s.FindExperienceScheduledNotifications,
	}
}

// Name returns the stable identifier used in structured logs.
func (r *ExperienceReminderReconciler) Name() string {
	return "experience_reminders"
}

// Existing returns the rows currently in storage whose item variant
// is experience.
func (r *ExperienceReminderReconciler) Existing(ctx context.Context) ([]*models.ScheduledNotification, error) {
	return r.existing(ctx)
}

// Desired walks active Experiences and produces one desired tuple per
// (attendee, offset) pair.
func (r *ExperienceReminderReconciler) Desired(ctx context.Context) ([]*models.ScheduledNotification, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ScheduledNotificationReconcile",
		"reconciler", r.Name(),
	)
	experiences, err := r.world.ListExperiences(ctx)
	if err != nil {
		return nil, fmt.Errorf("list experiences: %w", err)
	}

	now := clock.UnixSec(ctx)
	var out []*models.ScheduledNotification
	for _, exp := range experiences {
		if exp.GetDeleted() != nil {
			continue
		}
		if !isExperienceInScope(exp) {
			continue
		}
		startUnix, tz, ok := experienceStartTime(exp)
		if !ok {
			continue
		}
		// Drop experiences whose latest reminder slot is far enough
		// in the past that the reconciler shouldn't emit any tuple
		// for them. Past-due rows still in the desired set get
		// repaired/fired by the dispatcher; this just prevents
		// continual re-emission for ancient events.
		latestSlot := startUnix + OffsetClosePromptAfterStart
		if latestSlot < now-reconcileBacklogGrace {
			continue
		}

		// Pre-event reminders for RSVP=YES/MAYBE attendees.
		rsvps, err := r.world.ListRSVPsForExperience(ctx, exp.Id)
		if err != nil {
			logger.WarnContext(ctx, "list RSVPs failed; skipping experience",
				"experience_id", exp.Id,
				"error", err,
			)
			continue
		}

		for _, rsvp := range rsvps {
			if !isReminderRecipient(rsvp) {
				continue
			}
			for _, offset := range experienceReminderOffsets {
				fireAt := startUnix + offset
				// Drop slots whose fire_at is more than
				// DesiredFireAtGrace seconds in the past — they have
				// either already fired and been deleted, or are
				// owned by the dispatcher (an in-flight catch-up
				// after a brief outage). Re-emitting would produce
				// a duplicate push.
				if fireAt < now-DesiredFireAtGrace {
					continue
				}
				out = append(out, buildExperienceReminder(rsvp, exp.Id, startUnix, tz, offset))
			}
		}

		// Close-prompt to the host, 24h after start. The dispatcher
		// re-checks state at render time so a host who marks the
		// Experience completed inside the dispatch window doesn't
		// get a stale prompt.
		if exp.OwnerId != "" && exp.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED && exp.State != models.ExperienceState_EXPERIENCE_STATE_CANCELLED {
			fireAt := startUnix + OffsetClosePromptAfterStart
			if fireAt >= now-DesiredFireAtGrace {
				out = append(out, buildExperienceClosePrompt(exp, startUnix, tz, OffsetClosePromptAfterStart))
			}
		}
	}
	return out, nil
}

// isExperienceInScope reports whether the Experience is in a state
// where reminders should be emitted. COMPLETED and CANCELLED
// Experiences produce no new rows (and existing future rows get
// deleted by the reconciler's diff branch).
func isExperienceInScope(exp *models.Experience) bool {
	switch exp.State {
	case models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
		models.ExperienceState_EXPERIENCE_STATE_CANCELLED:
		return false
	}
	return true
}

// reconcileBacklogGrace is how far past an Experience's start time
// the reconciler still considers it "in scope" for reminder
// emission. Past this point, the reminders are stale and the
// reconciler stops emitting them (the dispatcher will hard-delete
// any existing rows on its next pass).
const reconcileBacklogGrace = 24 * 3600

// experienceStartTime extracts a fixed start unix seconds and an
// optional IANA timezone from an Experience's ExperienceTime. Returns
// (0, "", false) for TBD / unset times.
func experienceStartTime(exp *models.Experience) (int64, string, bool) {
	t := exp.GetTime()
	if t == nil {
		return 0, "", false
	}
	switch variant := t.TimeType.(type) {
	case *models.ExperienceTime_Specific:
		s := variant.Specific
		if s == nil || s.UnixTimestampSec == 0 {
			return 0, "", false
		}
		return s.UnixTimestampSec, s.Timezone, true
	case *models.ExperienceTime_Range:
		rng := variant.Range
		if rng == nil || rng.StartUnixSec == 0 {
			return 0, "", false
		}
		return rng.StartUnixSec, "", true
	}
	return 0, "", false
}

// isReminderRecipient reports whether an RSVP entitles its user to
// pre-event reminders. RSVP=YES and RSVP=MAYBE qualify; NO does not.
// The intention is stored as a string; checked case-insensitively
// against the canonical values from experience/service.go.
func isReminderRecipient(rsvp *models.ExperienceRSVP) bool {
	if rsvp.GetDeleted() != nil {
		return false
	}
	if rsvp.UserId == "" {
		// Provisional RSVPs (no user_id) cannot receive a push.
		return false
	}
	return rsvpstate.IsGoing(rsvp.GetIntention())
}

func buildExperienceReminder(rsvp *models.ExperienceRSVP, experienceID string, startUnix int64, tz string, offset int64) *models.ScheduledNotification {
	row := &models.ScheduledNotification{
		RecipientUserId: rsvp.UserId,
		FireAtUnixSec:   startUnix + offset,
		Item: &models.ScheduledNotification_Experience{
			Experience: &models.ExperienceNotification{
				ExperienceId:            experienceID,
				Purpose:                 models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
				OffsetSecondsFromAnchor: offset,
			},
		},
	}
	// Community context comes from the RSVP (experiences can appear
	// in multiple communities; the RSVP records which one this user
	// engaged in).
	if rsvp.CommunityId != "" {
		cid := rsvp.CommunityId
		row.CommunityId = &cid
	}
	if tz != "" {
		tzCopy := tz
		row.QuietHoursTimezone = &tzCopy
	}
	return row
}

// buildExperienceClosePrompt assembles a close-prompt row to the
// Experience's host. CommunityId is intentionally unset: close
// prompts are user-scoped (gated by UserNotificationPreferences),
// and the host doesn't necessarily have an RSVP to read a community
// context from.
func buildExperienceClosePrompt(exp *models.Experience, startUnix int64, tz string, offset int64) *models.ScheduledNotification {
	row := &models.ScheduledNotification{
		RecipientUserId: exp.OwnerId,
		FireAtUnixSec:   startUnix + offset,
		Item: &models.ScheduledNotification_Experience{
			Experience: &models.ExperienceNotification{
				ExperienceId:            exp.Id,
				Purpose:                 models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_CLOSE_PROMPT,
				OffsetSecondsFromAnchor: offset,
			},
		},
	}
	if tz != "" {
		tzCopy := tz
		row.QuietHoursTimezone = &tzCopy
	}
	return row
}

// storageExperienceReader is the production ExperienceReader that
// wraps ProtoSQLStorage.
type storageExperienceReader struct {
	storage *storage.ProtoSQLStorage
}

func (r *storageExperienceReader) ListExperiences(ctx context.Context) ([]*models.Experience, error) {
	msgs, err := r.storage.ListAll(ctx, &models.Experience{})
	if err != nil {
		return nil, err
	}
	out := make([]*models.Experience, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.(*models.Experience))
	}
	return out, nil
}

func (r *storageExperienceReader) ListRSVPsForExperience(ctx context.Context, experienceID string) ([]*models.ExperienceRSVP, error) {
	msgs, err := r.storage.QueryByField(ctx, "experience_id", experienceID, &models.ExperienceRSVP{})
	if err != nil {
		return nil, err
	}
	out := make([]*models.ExperienceRSVP, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.(*models.ExperienceRSVP))
	}
	return out, nil
}

func (r *storageExperienceReader) GetExperience(ctx context.Context, experienceID string) (*models.Experience, error) {
	exp := &models.Experience{}
	if err := r.storage.GetByID(ctx, experienceID, exp); err != nil {
		return nil, err
	}
	return exp, nil
}

func (r *storageExperienceReader) GetCommunityNotificationPreferences(ctx context.Context, userID, communityID string) (*models.CommunityNotificationPreferences, error) {
	if userID == "" || communityID == "" {
		return nil, nil
	}
	prefsByUser, err := communitylib.FetchPreferencesForUsers(ctx, r.storage, communityID, []string{userID})
	if err != nil {
		return nil, err
	}
	return prefsByUser[userID], nil
}

// ExperienceReminderDispatcher renders the push payload for an
// experience-anchored row at dispatch time. The matching reconciler
// emits the rows; this type only handles delivery.
type ExperienceReminderDispatcher struct {
	world ExperienceReader
}

// NewExperienceReminderDispatcher wires the dispatcher against a real
// storage layer.
func NewExperienceReminderDispatcher(s *storage.ProtoSQLStorage) *ExperienceReminderDispatcher {
	return &ExperienceReminderDispatcher{world: &storageExperienceReader{storage: s}}
}

// Name returns the stable identifier used in structured logs.
func (d *ExperienceReminderDispatcher) Name() string {
	return "experience_reminders"
}

// CanHandle reports whether the row's item variant is an Experience
// reminder this dispatcher owns. Today every experience-anchored row
// has REMINDER_BEFORE_START as its purpose; future close-prompt and
// recurrence-reminder dispatchers will register separately and
// branch on Purpose.
func (d *ExperienceReminderDispatcher) CanHandle(row *models.ScheduledNotification) bool {
	exp := row.GetExperience()
	if exp == nil {
		return false
	}
	return exp.Purpose == models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START
}

// QuietHoursPolicy branches on the row's offset: the 24h-before slot
// can defer to 07:00 local without misleading the recipient (event is
// still ~21h away when delivered the morning after), but the 2h-before
// and starting-now slots become wrong copy if delayed by multiple
// hours — better to drop those than fire them late.
func (d *ExperienceReminderDispatcher) QuietHoursPolicy(row *models.ScheduledNotification) QuietHoursPolicy {
	exp := row.GetExperience()
	if exp == nil {
		return QuietHoursPolicyDefer
	}
	// Offsets are seconds-before-start (negative). A more-negative
	// offset means a longer lead time and tolerates more delay.
	if exp.OffsetSecondsFromAnchor <= OffsetReminderDayBefore {
		return QuietHoursPolicyDefer
	}
	return QuietHoursPolicySkip
}

// Render reads the current Experience and builds the Notification.
// Returns (nil, nil) when the anchor is gone so the dispatcher
// driver logs a skip and moves on — the row is already deleted at
// this point.
func (d *ExperienceReminderDispatcher) Render(ctx context.Context, row *models.ScheduledNotification) (*models.Notification, error) {
	exp := row.GetExperience()
	if exp == nil {
		return nil, fmt.Errorf("row missing experience variant")
	}
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ScheduledNotificationDispatch",
		"dispatcher", d.Name(),
		"experience_id", exp.ExperienceId,
		"offset_seconds_from_anchor", exp.OffsetSecondsFromAnchor,
	)

	experience, err := d.world.GetExperience(ctx, exp.ExperienceId)
	if err != nil {
		// Treat as "anchor gone" — common race when an Experience
		// is deleted between reconcile and dispatch.
		logger.InfoContext(ctx, "anchor Experience not loadable; skipping send", "error", err)
		return nil, nil
	}
	if experience.GetDeleted() != nil {
		logger.InfoContext(ctx, "anchor Experience soft-deleted; skipping send")
		return nil, nil
	}

	// Per-community preference gate. Storage errors on the prefs read
	// fail open (proceed to send) rather than swallow a reminder
	// because of a transient DB hiccup — same posture as
	// community.IsActive.
	communityID := row.GetCommunityId()
	if communityID != "" {
		prefs, err := d.world.GetCommunityNotificationPreferences(ctx, row.RecipientUserId, communityID)
		if err != nil {
			logger.WarnContext(ctx, "preference lookup failed; failing open and delivering",
				"community_id", communityID,
				"error", err,
			)
		} else if !communitylib.CategoryEnabled(prefs, models.NotificationCategory_NOTIFICATION_CATEGORY_EVENT_REMINDERS) {
			logger.InfoContext(ctx, "recipient has disabled event reminders for this community; skipping",
				"community_id", communityID,
			)
			return nil, nil
		}
	}

	return notification_content.SystemNotification(ctx, &models.CommunityEventPayload{
		CommunityId: row.GetCommunityId(),
		EventType:   reminderEventType(exp.OffsetSecondsFromAnchor),
		// The name is what every surface's copy interpolates, and the
		// Experience is already loaded above — no extra read.
		ExperienceName: experience.Name,
		ExperienceId:   &exp.ExperienceId,
	}), nil
}

// reminderEventType maps a pre-event offset to the event type whose copy says
// the right thing about it. The offset is not carried on the payload, so the
// slot has to be encoded in the type itself — which is free, since event_type
// is an ordinary string and these values are not CommunityEventType enum
// members. Splitting here keeps one catalog entry per wording instead of one
// entry with a branch the renderers would have to be taught.
func reminderEventType(offsetSeconds int64) string {
	switch {
	case offsetSeconds <= OffsetReminderDayBefore:
		return "EXPERIENCE_REMINDER_DAY_BEFORE"
	case offsetSeconds <= OffsetReminderTwoHour:
		return "EXPERIENCE_REMINDER_TWO_HOUR"
	default:
		return "EXPERIENCE_REMINDER_STARTING"
	}
}

// ExperienceClosePromptDispatcher renders the close-prompt push that
// reminds a host to mark a finished Experience completed. Same
// item-type as the pre-event dispatcher, distinguished by the
// CLOSE_PROMPT purpose. Gated by
// UserNotificationPreferences.NotifyEventClosePrompts, not by
// per-community prefs — the prompt is about "your event you're
// hosting", not about a community feed.
type ExperienceClosePromptDispatcher struct {
	world         ExperienceReader
	userPrefsRead userPrefsLoaderRecipient
}

// userPrefsLoaderRecipient is a closure that reads
// UserNotificationPreferences for a recipient. Injected from the
// caller (typically main.go) to avoid importing
// server/services/user from this package.
type userPrefsLoaderRecipient func(ctx context.Context, userID string) (*models.UserNotificationPreferences, error)

// NewExperienceClosePromptDispatcher wires the dispatcher.
func NewExperienceClosePromptDispatcher(s *storage.ProtoSQLStorage, userPrefsRead userPrefsLoaderRecipient) *ExperienceClosePromptDispatcher {
	return &ExperienceClosePromptDispatcher{
		world:         &storageExperienceReader{storage: s},
		userPrefsRead: userPrefsRead,
	}
}

// Name returns the stable identifier used in structured logs.
func (d *ExperienceClosePromptDispatcher) Name() string {
	return "experience_close_prompts"
}

// CanHandle reports whether the row is an Experience-anchored
// CLOSE_PROMPT.
func (d *ExperienceClosePromptDispatcher) CanHandle(row *models.ScheduledNotification) bool {
	exp := row.GetExperience()
	if exp == nil {
		return false
	}
	return exp.Purpose == models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_CLOSE_PROMPT
}

// QuietHoursPolicy returns Defer. The close prompt asks the host to
// take an action ("mark completed") and gives them as much time as
// they need; delivering the morning after a quiet-hours fire_at is
// harmless.
func (d *ExperienceClosePromptDispatcher) QuietHoursPolicy(_ *models.ScheduledNotification) QuietHoursPolicy {
	return QuietHoursPolicyDefer
}

// Render reads the current Experience and builds the close-prompt
// Notification. Returns (nil, nil) when the Experience is already
// completed/cancelled, the anchor is gone, or the host has disabled
// close-prompt notifications.
func (d *ExperienceClosePromptDispatcher) Render(ctx context.Context, row *models.ScheduledNotification) (*models.Notification, error) {
	exp := row.GetExperience()
	if exp == nil {
		return nil, fmt.Errorf("row missing experience variant")
	}
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ScheduledNotificationDispatch",
		"dispatcher", d.Name(),
		"experience_id", exp.ExperienceId,
		"recipient_user_id", row.RecipientUserId,
	)

	experience, err := d.world.GetExperience(ctx, exp.ExperienceId)
	if err != nil {
		logger.InfoContext(ctx, "anchor Experience not loadable; skipping send", "error", err)
		return nil, nil
	}
	if experience.GetDeleted() != nil {
		logger.InfoContext(ctx, "anchor Experience soft-deleted; skipping send")
		return nil, nil
	}
	if !isExperienceInScope(experience) {
		// Host already closed (or the event was cancelled) — no
		// prompt needed.
		logger.InfoContext(ctx, "Experience already terminal; skipping close-prompt",
			"experience_state", experience.State.String(),
		)
		return nil, nil
	}

	if d.userPrefsRead != nil {
		prefs, err := d.userPrefsRead(ctx, row.RecipientUserId)
		if err != nil {
			logger.WarnContext(ctx, "preference lookup failed; failing open and delivering", "error", err)
		} else if prefs != nil && prefs.NotifyEventClosePrompts != nil && !*prefs.NotifyEventClosePrompts {
			logger.InfoContext(ctx, "host has disabled close-prompt notifications; skipping")
			return nil, nil
		}
	}

	return notification_content.SystemNotification(ctx, &models.CommunityEventPayload{
		CommunityId:    row.GetCommunityId(),
		EventType:      "EXPERIENCE_CLOSE_PROMPT",
		ExperienceName: experience.Name,
		ExperienceId:   &exp.ExperienceId,
	}), nil
}
