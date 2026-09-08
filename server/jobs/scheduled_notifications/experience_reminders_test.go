package scheduled_notifications

import (
	"context"
	"sort"
	"testing"
	"time"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

type fakeExperienceReader struct {
	experiences []*models.Experience
	rsvps       map[string][]*models.ExperienceRSVP                 // by experience_id
	byID        map[string]*models.Experience                       // for dispatcher GetExperience
	prefs       map[string]*models.CommunityNotificationPreferences // by "userID|communityID"
	prefsErr    error
	listErr     error
	rsvpErrByID map[string]error
	getErrByID  map[string]error
}

func (f *fakeExperienceReader) ListExperiences(_ context.Context) ([]*models.Experience, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.experiences, nil
}

func (f *fakeExperienceReader) ListRSVPsForExperience(_ context.Context, experienceID string) ([]*models.ExperienceRSVP, error) {
	if err := f.rsvpErrByID[experienceID]; err != nil {
		return nil, err
	}
	return f.rsvps[experienceID], nil
}

func (f *fakeExperienceReader) GetExperience(_ context.Context, experienceID string) (*models.Experience, error) {
	if err := f.getErrByID[experienceID]; err != nil {
		return nil, err
	}
	if exp, ok := f.byID[experienceID]; ok {
		return exp, nil
	}
	// Fall back to a linear scan of the list so tests that only set
	// `experiences` don't need to also set `byID`.
	for _, exp := range f.experiences {
		if exp.Id == experienceID {
			return exp, nil
		}
	}
	return nil, errExperienceNotFound
}

func (f *fakeExperienceReader) GetCommunityNotificationPreferences(_ context.Context, userID, communityID string) (*models.CommunityNotificationPreferences, error) {
	if f.prefsErr != nil {
		return nil, f.prefsErr
	}
	return f.prefs[userID+"|"+communityID], nil
}

var errExperienceNotFound = expNotFoundError("experience not found")

type expNotFoundError string

func (e expNotFoundError) Error() string { return string(e) }

func specificTimeExperience(id string, startUnix int64, tz string) *models.Experience {
	return &models.Experience{
		Id: id,
		Time: &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Specific{
				Specific: &models.SpecificTime{
					UnixTimestampSec: startUnix,
					Timezone:         tz,
				},
			},
		},
	}
}

func rangeTimeExperience(id string, startUnix int64) *models.Experience {
	return &models.Experience{
		Id: id,
		Time: &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Range{
				Range: &models.TimeRange{
					StartUnixSec: startUnix,
				},
			},
		},
	}
}

func tbdTimeExperience(id string) *models.Experience {
	return &models.Experience{
		Id: id,
		Time: &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Tbd{Tbd: &models.TimeTBD{}},
		},
	}
}

func rsvp(experienceID, userID, communityID string, intention models.RSVPIntention) *models.ExperienceRSVP {
	return &models.ExperienceRSVP{
		ExperienceId: experienceID,
		UserId:       userID,
		CommunityId:  communityID,
		Intention:    intention,
	}
}

func newReconciler(reader ExperienceReader) *ExperienceReminderReconciler {
	return &ExperienceReminderReconciler{
		world: reader,
		existing: func(_ context.Context) ([]*models.ScheduledNotification, error) {
			return nil, nil
		},
	}
}

// pinClock fixes clock.UnixSec(ctx) so tests can reason about
// "in the future" / "in the past" relative to a known moment.
func pinClock(now time.Time) context.Context {
	return clock.WithSimulationTime(context.Background(), now)
}

func TestExperienceReminders_EmitsThreeOffsetsPerYesAttendee(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	startUnix := now.Add(48 * time.Hour).Unix()

	reader := &fakeExperienceReader{
		experiences: []*models.Experience{
			specificTimeExperience("e-1", startUnix, "America/Los_Angeles"),
		},
		rsvps: map[string][]*models.ExperienceRSVP{
			"e-1": {rsvp("e-1", "u-1", "c-1", models.RSVPIntention_RSVP_INTENTION_YES)},
		},
	}

	got, err := newReconciler(reader).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3 (one per offset)", len(got))
	}

	offsets := []int64{}
	for _, row := range got {
		exp := row.GetExperience()
		if exp == nil {
			t.Fatalf("row %+v: missing experience variant", row)
		}
		if exp.ExperienceId != "e-1" {
			t.Errorf("ExperienceId = %q, want e-1", exp.ExperienceId)
		}
		if exp.Purpose != models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START {
			t.Errorf("Purpose = %v, want REMINDER_BEFORE_START", exp.Purpose)
		}
		if row.RecipientUserId != "u-1" {
			t.Errorf("RecipientUserId = %q, want u-1", row.RecipientUserId)
		}
		if row.GetCommunityId() != "c-1" {
			t.Errorf("CommunityId = %q, want c-1", row.GetCommunityId())
		}
		if row.GetQuietHoursTimezone() != "America/Los_Angeles" {
			t.Errorf("QuietHoursTimezone = %q, want America/Los_Angeles", row.GetQuietHoursTimezone())
		}
		offsets = append(offsets, exp.OffsetSecondsFromAnchor)
	}
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
	want := []int64{OffsetReminderDayBefore, OffsetReminderTwoHour, OffsetReminderStarting}
	for i, w := range want {
		if offsets[i] != w {
			t.Errorf("offset[%d] = %d, want %d", i, offsets[i], w)
		}
	}
}

func TestExperienceReminders_FireAtComputedFromAnchorPlusOffset(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	startUnix := now.Add(48 * time.Hour).Unix()

	reader := &fakeExperienceReader{
		experiences: []*models.Experience{
			specificTimeExperience("e-1", startUnix, ""),
		},
		rsvps: map[string][]*models.ExperienceRSVP{
			"e-1": {rsvp("e-1", "u-1", "c-1", models.RSVPIntention_RSVP_INTENTION_YES)},
		},
	}

	got, err := newReconciler(reader).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	wantByOffset := map[int64]int64{
		OffsetReminderDayBefore: startUnix + OffsetReminderDayBefore,
		OffsetReminderTwoHour:   startUnix + OffsetReminderTwoHour,
		OffsetReminderStarting:  startUnix + OffsetReminderStarting,
	}
	for _, row := range got {
		off := row.GetExperience().OffsetSecondsFromAnchor
		want, ok := wantByOffset[off]
		if !ok {
			t.Errorf("unexpected offset %d", off)
			continue
		}
		if row.FireAtUnixSec != want {
			t.Errorf("offset %d: FireAtUnixSec = %d, want %d", off, row.FireAtUnixSec, want)
		}
	}
}

func TestExperienceReminders_IncludesMaybeAttendees(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	startUnix := now.Add(48 * time.Hour).Unix()

	reader := &fakeExperienceReader{
		experiences: []*models.Experience{specificTimeExperience("e-1", startUnix, "")},
		rsvps: map[string][]*models.ExperienceRSVP{
			"e-1": {
				rsvp("e-1", "u-yes", "c-1", models.RSVPIntention_RSVP_INTENTION_YES),
				rsvp("e-1", "u-maybe", "c-1", models.RSVPIntention_RSVP_INTENTION_MAYBE),
			},
		},
	}

	got, err := newReconciler(reader).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	// Two attendees × 3 offsets each.
	if len(got) != 6 {
		t.Errorf("got %d rows, want 6 (2 attendees × 3 offsets)", len(got))
	}
}

func TestExperienceReminders_ExcludesNoAndProvisionalRSVPs(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	startUnix := now.Add(48 * time.Hour).Unix()

	provID := "prov-1"
	reader := &fakeExperienceReader{
		experiences: []*models.Experience{specificTimeExperience("e-1", startUnix, "")},
		rsvps: map[string][]*models.ExperienceRSVP{
			"e-1": {
				rsvp("e-1", "", "c-1", models.RSVPIntention_RSVP_INTENTION_YES), // missing user_id → skip
				{
					ExperienceId:      "e-1",
					ProvisionalUserId: &provID,
					CommunityId:       "c-1",
					Intention:         models.RSVPIntention_RSVP_INTENTION_YES,
				},
				rsvp("e-1", "u-no", "c-1", models.RSVPIntention_RSVP_INTENTION_NO), // declined
			},
		},
	}

	got, err := newReconciler(reader).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d rows, want 0 (no qualifying RSVPs)", len(got))
	}
}

func TestExperienceReminders_SkipsDeletedExperiencesAndRSVPs(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	startUnix := now.Add(48 * time.Hour).Unix()

	deletedExperience := specificTimeExperience("e-deleted", startUnix, "")
	deletedExperience.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: now.Unix()}

	rsvpDeleted := rsvp("e-1", "u-rsvp-deleted", "c-1", models.RSVPIntention_RSVP_INTENTION_YES)
	rsvpDeleted.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: now.Unix()}

	reader := &fakeExperienceReader{
		experiences: []*models.Experience{
			deletedExperience,
			specificTimeExperience("e-1", startUnix, ""),
		},
		rsvps: map[string][]*models.ExperienceRSVP{
			"e-1": {rsvpDeleted, rsvp("e-1", "u-active", "c-1", models.RSVPIntention_RSVP_INTENTION_YES)},
		},
	}

	got, err := newReconciler(reader).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	// Only u-active for e-1 (deleted experience contributes nothing).
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3 (one attendee × 3 offsets)", len(got))
	}
	for _, row := range got {
		if row.RecipientUserId != "u-active" {
			t.Errorf("unexpected recipient %s", row.RecipientUserId)
		}
	}
}

func TestExperienceReminders_SkipsTBDAndUnsetTimes(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)

	reader := &fakeExperienceReader{
		experiences: []*models.Experience{
			tbdTimeExperience("e-tbd"),
			{Id: "e-no-time"}, // no Time field set
		},
		rsvps: map[string][]*models.ExperienceRSVP{
			"e-tbd":     {rsvp("e-tbd", "u-1", "c-1", models.RSVPIntention_RSVP_INTENTION_YES)},
			"e-no-time": {rsvp("e-no-time", "u-2", "c-1", models.RSVPIntention_RSVP_INTENTION_YES)},
		},
	}

	got, err := newReconciler(reader).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d rows, want 0 (TBD/unset times produce no reminders)", len(got))
	}
}

func TestExperienceReminders_AcceptsRangeTimes(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	startUnix := now.Add(48 * time.Hour).Unix()

	reader := &fakeExperienceReader{
		experiences: []*models.Experience{rangeTimeExperience("e-1", startUnix)},
		rsvps: map[string][]*models.ExperienceRSVP{
			"e-1": {rsvp("e-1", "u-1", "c-1", models.RSVPIntention_RSVP_INTENTION_YES)},
		},
	}

	got, err := newReconciler(reader).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d rows, want 3 (range time should produce reminders)", len(got))
	}
}

func TestExperienceReminders_SkipsExperiencesPastBacklogGrace(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	// Started 25 hours ago — past the 24h backlog grace.
	startUnix := now.Add(-25 * time.Hour).Unix()

	reader := &fakeExperienceReader{
		experiences: []*models.Experience{specificTimeExperience("e-old", startUnix, "")},
		rsvps: map[string][]*models.ExperienceRSVP{
			"e-old": {rsvp("e-old", "u-1", "c-1", models.RSVPIntention_RSVP_INTENTION_YES)},
		},
	}

	got, err := newReconciler(reader).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d rows, want 0 (experience past backlog grace)", len(got))
	}
}

func TestExperienceReminders_OmitsPastFireAtSlotsForRecentlyStartedExperience(t *testing.T) {
	// An experience that started 1 hour ago is still within the 24h
	// backlog grace at the experience level, but all three reminder
	// slots have fire_at in the past (start-24h, start-2h, start).
	// Past-fire-at slots are not emitted — the dispatcher owns any
	// PENDING rows for them, and the reconciler must not re-emit a
	// just-fired tuple.
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	startUnix := now.Add(-1 * time.Hour).Unix()

	reader := &fakeExperienceReader{
		experiences: []*models.Experience{specificTimeExperience("e-recent", startUnix, "")},
		rsvps: map[string][]*models.ExperienceRSVP{
			"e-recent": {rsvp("e-recent", "u-1", "c-1", models.RSVPIntention_RSVP_INTENTION_YES)},
		},
	}

	got, err := newReconciler(reader).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d rows, want 0 (all slots are past fire_at)", len(got))
	}
}

func TestExperienceReminders_OnlyEmitsFutureSlots(t *testing.T) {
	// An experience starting in 3 hours: the 24h-before slot is
	// already past (fire_at = now-21h), but the 2h-before slot
	// (fire_at = now+1h) and the starting slot (fire_at = now+3h)
	// are still future. Only those two should be emitted.
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	startUnix := now.Add(3 * time.Hour).Unix()

	reader := &fakeExperienceReader{
		experiences: []*models.Experience{specificTimeExperience("e-soon", startUnix, "")},
		rsvps: map[string][]*models.ExperienceRSVP{
			"e-soon": {rsvp("e-soon", "u-1", "c-1", models.RSVPIntention_RSVP_INTENTION_YES)},
		},
	}
	got, err := newReconciler(reader).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2 (only future slots)", len(got))
	}
	offsets := map[int64]bool{}
	for _, row := range got {
		offsets[row.GetExperience().OffsetSecondsFromAnchor] = true
	}
	if offsets[OffsetReminderDayBefore] {
		t.Error("day-before slot should NOT be emitted (fire_at is in the past)")
	}
	if !offsets[OffsetReminderTwoHour] {
		t.Error("two-hour slot should be emitted (fire_at is in the future)")
	}
	if !offsets[OffsetReminderStarting] {
		t.Error("starting slot should be emitted (fire_at is in the future)")
	}
}

func TestExperienceReminders_EmitsClosePromptForActiveExperience(t *testing.T) {
	// Start time was 2 hours ago — the close prompt fires at
	// start_time + 24h, which is ~22 hours from now (future).
	// Pre-event slots are all past (filtered out by grace), so the
	// only emitted row should be the close-prompt to the host.
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	startUnix := now.Add(-2 * time.Hour).Unix()

	exp := specificTimeExperience("e-1", startUnix, "")
	exp.OwnerId = "u-host"
	exp.State = models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS

	reader := &fakeExperienceReader{
		experiences: []*models.Experience{exp},
		rsvps:       map[string][]*models.ExperienceRSVP{},
	}

	got, err := newReconciler(reader).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1 (close-prompt only)", len(got))
	}
	row := got[0]
	if row.RecipientUserId != "u-host" {
		t.Errorf("RecipientUserId = %q, want u-host", row.RecipientUserId)
	}
	expN := row.GetExperience()
	if expN == nil {
		t.Fatal("missing experience variant")
	}
	if expN.Purpose != models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_CLOSE_PROMPT {
		t.Errorf("Purpose = %v, want CLOSE_PROMPT", expN.Purpose)
	}
	if expN.OffsetSecondsFromAnchor != OffsetClosePromptAfterStart {
		t.Errorf("Offset = %d, want %d", expN.OffsetSecondsFromAnchor, OffsetClosePromptAfterStart)
	}
	if row.FireAtUnixSec != startUnix+OffsetClosePromptAfterStart {
		t.Errorf("FireAtUnixSec = %d, want %d", row.FireAtUnixSec, startUnix+OffsetClosePromptAfterStart)
	}
	// Community is unset by design — close-prompt is user-scoped.
	if row.CommunityId != nil {
		t.Errorf("CommunityId should be unset for close-prompt; got %q", *row.CommunityId)
	}
}

func TestExperienceReminders_OmitsClosePromptForCompletedExperience(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	startUnix := now.Add(-2 * time.Hour).Unix()

	exp := specificTimeExperience("e-1", startUnix, "")
	exp.OwnerId = "u-host"
	exp.State = models.ExperienceState_EXPERIENCE_STATE_COMPLETED

	reader := &fakeExperienceReader{
		experiences: []*models.Experience{exp},
	}

	got, err := newReconciler(reader).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d rows, want 0 (host already closed the event)", len(got))
	}
}

func TestExperienceReminders_OmitsClosePromptForCancelledExperience(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	startUnix := now.Add(-2 * time.Hour).Unix()

	exp := specificTimeExperience("e-1", startUnix, "")
	exp.OwnerId = "u-host"
	exp.State = models.ExperienceState_EXPERIENCE_STATE_CANCELLED

	reader := &fakeExperienceReader{
		experiences: []*models.Experience{exp},
	}

	got, err := newReconciler(reader).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d rows, want 0 (event was cancelled)", len(got))
	}
}

func TestExperienceReminders_EmitsBothReminderAndClosePromptForFutureExperience(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	startUnix := now.Add(36 * time.Hour).Unix() // 36h from now

	exp := specificTimeExperience("e-1", startUnix, "America/Los_Angeles")
	exp.OwnerId = "u-host"
	exp.State = models.ExperienceState_EXPERIENCE_STATE_ACTIVE

	reader := &fakeExperienceReader{
		experiences: []*models.Experience{exp},
		rsvps: map[string][]*models.ExperienceRSVP{
			"e-1": {rsvp("e-1", "u-host", "c-1", models.RSVPIntention_RSVP_INTENTION_YES), rsvp("e-1", "u-attendee", "c-1", models.RSVPIntention_RSVP_INTENTION_YES)},
		},
	}

	got, err := newReconciler(reader).Desired(pinClock(now))
	if err != nil {
		t.Fatalf("Desired: %v", err)
	}
	// 2 attendees × 3 pre-event slots + 1 close-prompt = 7
	if len(got) != 7 {
		t.Errorf("got %d rows, want 7 (2 attendees × 3 slots + 1 close-prompt)", len(got))
	}
	closePromptCount := 0
	for _, row := range got {
		if row.GetExperience().Purpose == models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_CLOSE_PROMPT {
			closePromptCount++
			if row.RecipientUserId != "u-host" {
				t.Errorf("close-prompt recipient = %q, want u-host", row.RecipientUserId)
			}
		}
	}
	if closePromptCount != 1 {
		t.Errorf("close-prompt count = %d, want 1", closePromptCount)
	}
}
