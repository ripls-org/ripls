package notification_content

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/text/language"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// fakeUserReader serves User rows from memory and counts batch reads, so tests
// can assert the fan-out costs exactly one.
type fakeUserReader struct {
	users map[string]*models.User
	err   error
	calls int
}

func (f *fakeUserReader) GetByIDs(_ context.Context, ids []string, _ proto.Message, _ ...storage.QueryOptions) (map[string]proto.Message, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]proto.Message, len(ids))
	for _, id := range ids {
		if u, ok := f.users[id]; ok {
			out[id] = u
		}
	}
	return out, nil
}

// userWithLanguage returns a User whose preferred_language is lang, or unset
// when lang is "".
func userWithLanguage(id, lang string) *models.User {
	u := &models.User{Id: id}
	if lang != "" {
		u.PreferredLanguage = proto.String(lang)
	}
	return u
}

func TestLoadRecipientLocalesResolvesEachRecipient(t *testing.T) {
	reader := &fakeUserReader{users: map[string]*models.User{
		"es":          userWithLanguage("es", "es"),
		"es-mx":       userWithLanguage("es-mx", "es-MX"),
		"en":          userWithLanguage("en", "en"),
		"empty":       userWithLanguage("empty", ""),
		"unsupported": userWithLanguage("unsupported", "xx-YY"),
		"other":       userWithLanguage("other", "es"),
	}}

	got := LoadRecipientLocales(context.Background(), reader,
		[]string{"es", "es-mx", "en", "empty", "unsupported", "missing"})

	want := map[string]string{
		"es": "es", "es-mx": "es", "en": "en",
		"empty": "en", "unsupported": "en", "missing": "en",
	}
	for id, tag := range want {
		if got[id].String() != tag {
			t.Errorf("locale[%q] = %v, want %s", id, got[id], tag)
		}
	}
	if _, leaked := got["other"]; leaked {
		t.Error("result must hold only the requested recipients")
	}
	if reader.calls != 1 {
		t.Errorf("GetByIDs calls = %d, want 1 batched read", reader.calls)
	}
}

func TestLoadRecipientLocalesFailedReadDefaultsToEnglish(t *testing.T) {
	reader := &fakeUserReader{err: errors.New("db down")}

	got := LoadRecipientLocales(context.Background(), reader, []string{"a", "b"})

	for _, id := range []string{"a", "b"} {
		if got[id] != language.English {
			t.Errorf("locale[%q] = %v, want en after a failed read", id, got[id])
		}
	}
}

func TestLoadRecipientLocalesEmptyRecipientsSkipsRead(t *testing.T) {
	reader := &fakeUserReader{}
	if got := LoadRecipientLocales(context.Background(), reader, nil); len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
	if reader.calls != 0 {
		t.Errorf("GetByIDs calls = %d, want 0 for no recipients", reader.calls)
	}
}

// TestSystemNotificationsRendersPerRecipientLocale is the nudge fan-out case:
// one batched read, each recipient's push in their own language, and recipients
// sharing a locale sharing one render.
func TestSystemNotificationsRendersPerRecipientLocale(t *testing.T) {
	reader := &fakeUserReader{users: map[string]*models.User{
		"es-1": userWithLanguage("es-1", "es"),
		"es-2": userWithLanguage("es-2", "es"),
		"en-1": userWithLanguage("en-1", "en"),
		"none": userWithLanguage("none", ""),
	}}
	payload := &models.CommunityEventPayload{
		EventType:      "EXPERIENCE_NEEDS_NUDGE",
		ExperienceName: "Backyard BBQ",
	}

	got := SystemNotifications(context.Background(), reader,
		[]string{"es-1", "es-2", "en-1", "none"}, payload)

	if len(got) != 4 {
		t.Fatalf("got %d notifications, want one per recipient", len(got))
	}
	for _, id := range []string{"es-1", "es-2"} {
		if got[id].Title != "¿Quién lleva qué?" || got[id].Body != "Backyard BBQ todavía necesita algunas cosas" {
			t.Errorf("%s push = (%q, %q), want Spanish", id, got[id].Title, got[id].Body)
		}
	}
	for _, id := range []string{"en-1", "none"} {
		if got[id].Title != "Who's bringing what?" || got[id].Body != "Backyard BBQ still needs a few things" {
			t.Errorf("%s push = (%q, %q), want English", id, got[id].Title, got[id].Body)
		}
	}
	if got["es-1"] != got["es-2"] {
		t.Error("recipients sharing a locale should share one rendered notification")
	}
	if got["es-1"].GetCommunityEvent() != payload {
		t.Error("payload must ride along verbatim")
	}
	if reader.calls != 1 {
		t.Errorf("GetByIDs calls = %d, want 1 batched read", reader.calls)
	}
}

func TestSystemNotificationsFailedReadRendersEnglish(t *testing.T) {
	reader := &fakeUserReader{err: errors.New("db down")}
	payload := &models.CommunityEventPayload{
		EventType:    "REQUEST_NEEDS_NUDGE",
		RequestTitle: "Folding Table",
	}

	got := SystemNotifications(context.Background(), reader, []string{"a"}, payload)

	if got["a"] == nil || got["a"].Body != "Folding Table still needs a hand" {
		t.Errorf("push after failed read = %+v, want English copy", got["a"])
	}
}
