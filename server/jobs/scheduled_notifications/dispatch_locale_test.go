package scheduled_notifications

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/text/language"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestDispatch_SpanishRecipientGetsSpanishEventReminder is #3276's done-when:
// a Spanish-language user gets a Spanish event-reminder push, through the real
// dispatch driver and the real reminder dispatcher.
func TestDispatch_SpanishRecipientGetsSpanishEventReminder(t *testing.T) {
	exp := specificTimeExperience("e-1", 1234567890, "")
	exp.Name = "Pasta Night"

	store := newFakeDispatchStorage()
	store.users = map[string]*models.User{
		"u-es": {Id: "u-es", PreferredLanguage: proto.String("es")},
		"u-en": {Id: "u-en", PreferredLanguage: proto.String("en")},
	}
	esRow := experienceRow("u-es", "e-1",
		models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
		OffsetReminderTwoHour, 100)
	esRow.Id = "id-es"
	enRow := experienceRow("u-en", "e-1",
		models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START,
		OffsetReminderTwoHour, 100)
	enRow.Id = "id-en"
	store.due = []*models.ScheduledNotification{esRow, enRow}

	notif := newFakeNotifService()
	disp := &ExperienceReminderDispatcher{world: &fakeExperienceReader{experiences: []*models.Experience{exp}}}

	stats, err := Dispatch(context.Background(), store, notif, []Dispatcher{disp}, 10)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if stats.Sent != 2 {
		t.Fatalf("Sent = %d, want 2", stats.Sent)
	}

	es := notif.sentByRecipient["u-es"]
	if len(es) != 1 {
		t.Fatalf("u-es pushes = %d, want 1", len(es))
	}
	if es[0].Title != "El evento empieza pronto" || es[0].Body != "Pasta Night empieza en 2 horas" {
		t.Errorf("Spanish reminder = (%q, %q), want Spanish copy", es[0].Title, es[0].Body)
	}

	en := notif.sentByRecipient["u-en"]
	if len(en) != 1 {
		t.Fatalf("u-en pushes = %d, want 1", len(en))
	}
	if en[0].Title != "Event starting soon" || en[0].Body != "Pasta Night starts in 2 hours" {
		t.Errorf("English reminder = (%q, %q), want English copy", en[0].Title, en[0].Body)
	}
}

// TestDispatch_LoadsLocalesOncePerTick asserts the tick costs one batched
// user read across all due rows — not one per row — and that the batch holds
// each recipient once.
func TestDispatch_LoadsLocalesOncePerTick(t *testing.T) {
	store := newFakeDispatchStorage()
	store.users = map[string]*models.User{
		"u-es":          {Id: "u-es", PreferredLanguage: proto.String("es-MX")},
		"u-empty":       {Id: "u-empty"},
		"u-unsupported": {Id: "u-unsupported", PreferredLanguage: proto.String("fr")},
	}
	var rows []*models.ScheduledNotification
	for _, r := range []struct{ id, user string }{
		{"r1", "u-es"}, {"r2", "u-es"}, {"r3", "u-empty"}, {"r4", "u-unsupported"}, {"r5", "u-missing"},
	} {
		row := experienceRow(r.user, "e-1",
			models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100)
		row.Id = r.id
		rows = append(rows, row)
	}
	store.due = rows

	disp := &fixedDispatcher{name: "d", canHandleAll: true, renderResult: newSimpleNotification("x")}
	if _, err := Dispatch(context.Background(), store, newFakeNotifService(), []Dispatcher{disp}, 10); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if store.getByIDsCalls != 1 {
		t.Fatalf("GetByIDs calls = %d, want 1 per tick", store.getByIDsCalls)
	}
	if got := len(store.getByIDsArgs[0]); got != 4 {
		t.Errorf("batched ids = %v, want the 4 distinct recipients", store.getByIDsArgs[0])
	}
	want := map[string]language.Tag{
		"r1": language.Spanish, "r2": language.Spanish,
		"r3": language.English, "r4": language.English, "r5": language.English,
	}
	for id, tag := range want {
		if got := disp.renderedLocales[id]; got != tag {
			t.Errorf("row %s rendered with %v, want %v", id, got, tag)
		}
	}
}

// TestDispatch_LocaleLoadFailureFallsBackToEnglish asserts a failed locale read
// is non-fatal: every row still renders (in English) and sends.
func TestDispatch_LocaleLoadFailureFallsBackToEnglish(t *testing.T) {
	store := newFakeDispatchStorage()
	store.usersErr = errors.New("db gone")
	row := experienceRow("u-1", "e-1",
		models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_REMINDER_BEFORE_START, -86400, 100)
	row.Id = "id-1"
	store.due = []*models.ScheduledNotification{row}

	notif := newFakeNotifService()
	disp := &fixedDispatcher{name: "d", canHandleAll: true, renderResult: newSimpleNotification("x")}
	stats, err := Dispatch(context.Background(), store, notif, []Dispatcher{disp}, 10)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if stats.Sent != 1 {
		t.Errorf("Sent = %d, want 1 despite the failed locale read", stats.Sent)
	}
	if got := disp.renderedLocales["id-1"]; got != language.English {
		t.Errorf("rendered with %v, want English fallback", got)
	}
}

// TestExperienceClosePromptDispatcher_RendersSpanish covers a second builder's
// Spanish push copy directly, so a dispatcher that dropped its locale argument
// would fail.
func TestExperienceClosePromptDispatcher_RendersSpanish(t *testing.T) {
	exp := specificTimeExperience("e-1", 1234567890, "")
	exp.Name = "Pasta Night"
	exp.State = models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS
	d := &ExperienceClosePromptDispatcher{world: &fakeExperienceReader{experiences: []*models.Experience{exp}}}
	row := experienceRow("u-host", "e-1",
		models.ExperienceNotificationPurpose_EXPERIENCE_NOTIFICATION_PURPOSE_CLOSE_PROMPT,
		OffsetClosePromptAfterStart, 100)

	n, err := d.Render(context.Background(), row, language.Spanish)
	if err != nil || n == nil {
		t.Fatalf("Render = (%v, %v)", n, err)
	}
	if n.Title != "Cierra tu evento" || n.Body != "Marca Pasta Night como completado cuando termines" {
		t.Errorf("close prompt = (%q, %q), want Spanish", n.Title, n.Body)
	}
}
