package scheduled_notifications

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestExperienceClosePromptDispatcher_CanHandle(t *testing.T) {
	d := &ExperienceClosePromptDispatcher{}

	t.Run("matches CLOSE_PROMPT rows", func(t *testing.T) {
		row := experienceRow("u-host", "e-1",
			models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_CLOSE_PROMPT,
			OffsetClosePromptAfterStart, 100)
		if !d.CanHandle(row) {
			t.Error("expected CanHandle=true for CLOSE_PROMPT")
		}
	})

	t.Run("does not match pre-event reminder rows", func(t *testing.T) {
		row := experienceRow("u-1", "e-1",
			models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
			-86400, 100)
		if d.CanHandle(row) {
			t.Error("expected CanHandle=false for REMINDER_BEFORE_START")
		}
	})
}

func TestExperienceClosePromptDispatcher_Render(t *testing.T) {
	exp := specificTimeExperience("e-1", 1234567890, "")
	exp.Name = "Pasta Night"
	exp.OwnerId = "u-host"
	exp.State = models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS

	reader := &fakeExperienceReader{experiences: []*models.Experience{exp}}
	d := &ExperienceClosePromptDispatcher{world: reader}

	row := experienceRow("u-host", "e-1",
		models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_CLOSE_PROMPT,
		OffsetClosePromptAfterStart, 100)
	notif, err := d.Render(context.Background(), row)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if notif == nil {
		t.Fatal("Render returned nil notification")
	}
	if notif.Title != "Wrap up your event" {
		t.Errorf("Title = %q, want \"Wrap up your event\"", notif.Title)
	}
	if notif.Body != "Mark Pasta Night as completed when you're done" {
		t.Errorf("Body = %q, want mention of Pasta Night", notif.Body)
	}
	payload := notif.GetCommunityEvent()
	if payload == nil {
		t.Fatal("missing CommunityEventPayload")
	}
	if payload.EventType != "EXPERIENCE_CLOSE_PROMPT" {
		t.Errorf("EventType = %q, want EXPERIENCE_CLOSE_PROMPT", payload.EventType)
	}
	if payload.GetExperienceId() != "e-1" {
		t.Errorf("ExperienceId = %q, want e-1", payload.GetExperienceId())
	}
}

func TestExperienceClosePromptDispatcher_SkipsCompletedExperience(t *testing.T) {
	// Race scenario: host marked the Experience completed between
	// reconcile and dispatch. The dispatcher's state re-check
	// catches it and skips.
	exp := specificTimeExperience("e-1", 1234567890, "")
	exp.Name = "Pasta Night"
	exp.OwnerId = "u-host"
	exp.State = models.ExperienceState_EXPERIENCE_STATE_COMPLETED

	reader := &fakeExperienceReader{experiences: []*models.Experience{exp}}
	d := &ExperienceClosePromptDispatcher{world: reader}

	row := experienceRow("u-host", "e-1",
		models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_CLOSE_PROMPT,
		OffsetClosePromptAfterStart, 100)
	notif, err := d.Render(context.Background(), row)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if notif != nil {
		t.Error("expected nil notification when Experience is already COMPLETED")
	}
}

func TestExperienceClosePromptDispatcher_SkipsWhenPreferenceDisabled(t *testing.T) {
	exp := specificTimeExperience("e-1", 1234567890, "")
	exp.Name = "Pasta Night"
	exp.OwnerId = "u-host"
	exp.State = models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS

	reader := &fakeExperienceReader{experiences: []*models.Experience{exp}}
	disabled := false
	d := &ExperienceClosePromptDispatcher{
		world: reader,
		userPrefsRead: func(_ context.Context, userID string) (*models.UserNotificationPreferences, error) {
			if userID == "u-host" {
				return &models.UserNotificationPreferences{NotifyEventClosePrompts: &disabled}, nil
			}
			return nil, nil
		},
	}

	row := experienceRow("u-host", "e-1",
		models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_CLOSE_PROMPT,
		OffsetClosePromptAfterStart, 100)
	notif, err := d.Render(context.Background(), row)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if notif != nil {
		t.Error("expected nil notification when host disabled close prompts")
	}
}

func TestExperienceClosePromptDispatcher_PreferenceFetchErrorsFailOpen(t *testing.T) {
	exp := specificTimeExperience("e-1", 1234567890, "")
	exp.Name = "Pasta Night"
	exp.OwnerId = "u-host"
	exp.State = models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS

	reader := &fakeExperienceReader{experiences: []*models.Experience{exp}}
	d := &ExperienceClosePromptDispatcher{
		world: reader,
		userPrefsRead: func(_ context.Context, _ string) (*models.UserNotificationPreferences, error) {
			return nil, errExperienceNotFound
		},
	}

	row := experienceRow("u-host", "e-1",
		models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_CLOSE_PROMPT,
		OffsetClosePromptAfterStart, 100)
	notif, err := d.Render(context.Background(), row)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if notif == nil {
		t.Error("expected fail-open delivery on pref lookup error")
	}
}

func TestExperienceReminderDispatcher_CanHandle(t *testing.T) {
	d := &ExperienceReminderDispatcher{}

	t.Run("matches REMINDER_BEFORE_START rows", func(t *testing.T) {
		row := experienceRow("u-1", "e-1",
			models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
			-86400, 100)
		if !d.CanHandle(row) {
			t.Error("expected CanHandle=true for REMINDER_BEFORE_START")
		}
	})

	t.Run("does not match close-prompt rows", func(t *testing.T) {
		row := experienceRow("u-1", "e-1",
			models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_CLOSE_PROMPT,
			86400, 100)
		if d.CanHandle(row) {
			t.Error("expected CanHandle=false for CLOSE_PROMPT")
		}
	})

	t.Run("does not match loan rows", func(t *testing.T) {
		row := &models.ScheduledNotification{
			Item: &models.ScheduledNotification_Loan{
				Loan: &models.LoanNotification{
					Purpose: models.LoanNotificationPurpose_LOAN_NOTIFICATION_PURPOSE_RETURN_REMINDER,
				},
			},
		}
		if d.CanHandle(row) {
			t.Error("expected CanHandle=false for loan rows")
		}
	})
}

func TestExperienceReminderDispatcher_Render(t *testing.T) {
	exp := specificTimeExperience("e-1", 1234567890, "America/Los_Angeles")
	exp.Name = "Pasta Night"
	reader := &fakeExperienceReader{
		experiences: []*models.Experience{exp},
	}
	d := &ExperienceReminderDispatcher{world: reader}
	ctx := context.Background()
	cid := "c-1"

	cases := []struct {
		name          string
		offset        int64
		wantTitle     string
		wantBody      string
		wantEventType string
	}{
		{"24h before", OffsetReminderDayBefore, "Event tomorrow", "Pasta Night is tomorrow", "EXPERIENCE_REMINDER_DAY_BEFORE"},
		{"2h before", OffsetReminderTwoHour, "Event starting soon", "Pasta Night starts in 2 hours", "EXPERIENCE_REMINDER_TWO_HOUR"},
		{"starting", OffsetReminderStarting, "Event starting now", "Pasta Night is starting", "EXPERIENCE_REMINDER_STARTING"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := &models.ScheduledNotification{
				Id:              "id-1",
				RecipientUserId: "u-1",
				CommunityId:     &cid,
				FireAtUnixSec:   exp.GetTime().GetSpecific().UnixTimestampSec + c.offset,
				Item: &models.ScheduledNotification_Experience{
					Experience: &models.ExperienceNotification{
						ExperienceId:            "e-1",
						Purpose:                 models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
						OffsetSecondsFromAnchor: c.offset,
					},
				},
			}
			notif, err := d.Render(ctx, row)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if notif == nil {
				t.Fatal("Render returned nil notification")
			}
			if notif.Title != c.wantTitle {
				t.Errorf("Title = %q, want %q", notif.Title, c.wantTitle)
			}
			if notif.Body != c.wantBody {
				t.Errorf("Body = %q, want %q", notif.Body, c.wantBody)
			}
			payload := notif.GetCommunityEvent()
			if payload == nil {
				t.Fatal("notification missing CommunityEventPayload")
			}
			// One event type per reminder slot: the offset is not carried on
			// the payload, so it is the event type that tells the off-app
			// renderers which wording to use.
			if payload.EventType != c.wantEventType {
				t.Errorf("EventType = %q, want %q", payload.EventType, c.wantEventType)
			}
			// The name has to be on the payload, not just interpolated into
			// Body: the off-app senders ignore Title/Body and re-render the
			// sentence from these fields (#2896).
			if payload.ExperienceName != "Pasta Night" {
				t.Errorf("payload ExperienceName = %q, want Pasta Night", payload.ExperienceName)
			}
			if payload.GetExperienceId() != "e-1" {
				t.Errorf("payload ExperienceId = %q, want e-1", payload.GetExperienceId())
			}
			if payload.CommunityId != "c-1" {
				t.Errorf("payload CommunityId = %q, want c-1", payload.CommunityId)
			}
		})
	}
}

func TestExperienceReminderDispatcher_RenderSkipsWhenAnchorMissing(t *testing.T) {
	reader := &fakeExperienceReader{} // no experiences
	d := &ExperienceReminderDispatcher{world: reader}

	row := experienceRow("u-1", "e-missing",
		models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
		OffsetReminderDayBefore, 100)
	notif, err := d.Render(context.Background(), row)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if notif != nil {
		t.Errorf("Render should return nil notification when anchor is gone; got %+v", notif)
	}
}

func TestExperienceReminderDispatcher_RenderSkipsWhenPreferenceDisabled(t *testing.T) {
	exp := specificTimeExperience("e-1", 1234567890, "")
	exp.Name = "Pasta Night"
	disabled := false
	reader := &fakeExperienceReader{
		experiences: []*models.Experience{exp},
		prefs: map[string]*models.CommunityNotificationPreferences{
			"u-1|c-1": {NotifyEventReminders: &disabled},
		},
	}
	d := &ExperienceReminderDispatcher{world: reader}

	cid := "c-1"
	row := &models.ScheduledNotification{
		Id:              "id-1",
		RecipientUserId: "u-1",
		CommunityId:     &cid,
		Item: &models.ScheduledNotification_Experience{
			Experience: &models.ExperienceNotification{
				ExperienceId: "e-1",
				Purpose:      models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
			},
		},
	}
	notif, err := d.Render(context.Background(), row)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if notif != nil {
		t.Errorf("Render should return nil when prefs disable event reminders; got %+v", notif)
	}
}

func TestExperienceReminderDispatcher_RenderProceedsWhenPreferenceUnsetOrEnabled(t *testing.T) {
	exp := specificTimeExperience("e-1", 1234567890, "")
	exp.Name = "Pasta Night"

	t.Run("no prefs row → unset = on", func(t *testing.T) {
		reader := &fakeExperienceReader{experiences: []*models.Experience{exp}}
		d := &ExperienceReminderDispatcher{world: reader}
		cid := "c-1"
		row := &models.ScheduledNotification{
			Id:              "id-1",
			RecipientUserId: "u-1",
			CommunityId:     &cid,
			Item: &models.ScheduledNotification_Experience{
				Experience: &models.ExperienceNotification{
					ExperienceId: "e-1",
					Purpose:      models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
				},
			},
		}
		notif, err := d.Render(context.Background(), row)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if notif == nil {
			t.Error("expected Render to proceed when no prefs row exists")
		}
	})

	t.Run("prefs row with notify_event_reminders=true", func(t *testing.T) {
		enabled := true
		reader := &fakeExperienceReader{
			experiences: []*models.Experience{exp},
			prefs: map[string]*models.CommunityNotificationPreferences{
				"u-1|c-1": {NotifyEventReminders: &enabled},
			},
		}
		d := &ExperienceReminderDispatcher{world: reader}
		cid := "c-1"
		row := &models.ScheduledNotification{
			Id:              "id-1",
			RecipientUserId: "u-1",
			CommunityId:     &cid,
			Item: &models.ScheduledNotification_Experience{
				Experience: &models.ExperienceNotification{
					ExperienceId: "e-1",
					Purpose:      models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
				},
			},
		}
		notif, err := d.Render(context.Background(), row)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if notif == nil {
			t.Error("expected Render to proceed when prefs explicitly enable event reminders")
		}
	})
}

func TestExperienceReminderDispatcher_PreferenceFetchErrorsFailOpen(t *testing.T) {
	exp := specificTimeExperience("e-1", 1234567890, "")
	exp.Name = "Pasta Night"

	reader := &fakeExperienceReader{
		experiences: []*models.Experience{exp},
		prefsErr:    errExperienceNotFound, // any non-nil error
	}
	d := &ExperienceReminderDispatcher{world: reader}
	cid := "c-1"
	row := &models.ScheduledNotification{
		Id:              "id-1",
		RecipientUserId: "u-1",
		CommunityId:     &cid,
		Item: &models.ScheduledNotification_Experience{
			Experience: &models.ExperienceNotification{
				ExperienceId: "e-1",
				Purpose:      models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
			},
		},
	}
	notif, err := d.Render(context.Background(), row)
	if err != nil {
		t.Fatalf("Render returned error instead of failing open: %v", err)
	}
	if notif == nil {
		t.Error("expected Render to proceed (fail open) on a transient prefs lookup error")
	}
}

func TestExperienceReminderDispatcher_QuietHoursPolicy(t *testing.T) {
	d := &ExperienceReminderDispatcher{}

	cases := []struct {
		name   string
		offset int64
		want   QuietHoursPolicy
	}{
		// 24h-before defers: the reminder fires at 07:00 local the
		// morning after the original fire_at, with ~21h until the
		// event — copy "your event tomorrow" still accurate.
		{"24h before defers", OffsetReminderDayBefore, QuietHoursPolicyDefer},
		// 2h-before skips: delaying to 07:00 local would deliver
		// "starts in 2 hours" hours after the event already started.
		{"2h before skips", OffsetReminderTwoHour, QuietHoursPolicySkip},
		// Starting skips for the same reason — "starting now" is
		// misleadingly late.
		{"starting skips", OffsetReminderStarting, QuietHoursPolicySkip},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := experienceRow("u-1", "e-1",
				models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
				c.offset, 100)
			got := d.QuietHoursPolicy(row)
			if got != c.want {
				t.Errorf("offset %d: policy = %v, want %v", c.offset, got, c.want)
			}
		})
	}

	t.Run("missing experience variant defaults to Defer", func(t *testing.T) {
		got := d.QuietHoursPolicy(&models.ScheduledNotification{})
		if got != QuietHoursPolicyDefer {
			t.Errorf("policy on empty row = %v, want Defer", got)
		}
	})
}

func TestExperienceClosePromptDispatcher_QuietHoursPolicy(t *testing.T) {
	// Close prompts are day-grained ("wrap up your event") and the
	// host has flexibility — Defer is always fine.
	d := &ExperienceClosePromptDispatcher{}
	row := experienceRow("u-host", "e-1",
		models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_CLOSE_PROMPT,
		OffsetClosePromptAfterStart, 100)
	if got := d.QuietHoursPolicy(row); got != QuietHoursPolicyDefer {
		t.Errorf("policy = %v, want Defer", got)
	}
}

func TestExperienceReminderDispatcher_RenderSkipsWhenAnchorSoftDeleted(t *testing.T) {
	exp := specificTimeExperience("e-1", 1234567890, "")
	exp.Name = "Pasta Night"
	exp.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: 1234567000}
	reader := &fakeExperienceReader{experiences: []*models.Experience{exp}}
	d := &ExperienceReminderDispatcher{world: reader}

	row := experienceRow("u-1", "e-1",
		models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
		OffsetReminderDayBefore, 100)
	notif, err := d.Render(context.Background(), row)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if notif != nil {
		t.Errorf("Render should return nil when anchor is soft-deleted; got %+v", notif)
	}
}

func TestExperienceReminders_IntegratesWithReconcileDriver(t *testing.T) {
	// End-to-end smoke through the Reconcile driver, using the
	// in-memory ReconcileStorage fake from reconcile_test.go. After
	// a reconcile pass the fake storage should hold exactly the
	// three desired rows for one attendee × three offsets.
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	startUnix := now.Add(48 * time.Hour).Unix()

	reader := &fakeExperienceReader{
		experiences: []*models.Experience{specificTimeExperience("e-1", startUnix, "")},
		rsvps: map[string][]*models.ExperienceRSVP{
			"e-1": {rsvp("e-1", "u-1", "c-1", models.RSVPIntention_RSVP_INTENTION_YES)},
		},
	}
	r := newReconciler(reader)
	store := newFakeReconcileStorage()

	stats, err := Reconcile(pinClock(now), store, r)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if stats.Inserted != 3 {
		t.Errorf("Inserted = %d, want 3", stats.Inserted)
	}
	if got := len(store.rowsByID); got != 3 {
		t.Errorf("stored rows = %d, want 3", got)
	}

	// Second pass with no policy or world change: all unchanged.
	r.existing = func(_ context.Context) ([]*models.ScheduledNotification, error) {
		rows := make([]*models.ScheduledNotification, 0, len(store.rowsByID))
		for _, v := range store.rowsByID {
			rows = append(rows, v)
		}
		return rows, nil
	}
	stats, err = Reconcile(pinClock(now), store, r)
	if err != nil {
		t.Fatalf("Reconcile second pass: %v", err)
	}
	if stats.Inserted != 0 {
		t.Errorf("second pass Inserted = %d, want 0", stats.Inserted)
	}
	if stats.Unchanged != 3 {
		t.Errorf("second pass Unchanged = %d, want 3", stats.Unchanged)
	}
}
