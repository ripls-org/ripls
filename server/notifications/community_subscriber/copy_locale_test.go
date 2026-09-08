package community_subscriber

import (
	"context"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// The tests in this file exercise the localization PIPELINE, not the
// correctness of any specific translation. We assert that a non-
// default-locale Localizer is consulted (output differs from the
// English path), that ICU params (gear/event names) round-trip in any
// locale, and that names that should never be translated (entity
// names, actor names) flow through verbatim. We pick English vs
// Spanish because they're the two locales we ship today; the contract
// is identical for any future language.
//
// Catalog completeness (every key present in every locale) is
// enforced by `scripts/check_l10n_toml_parity.js`. Full per-event-
// type coverage of the rendered text lives in copy_test.go against
// the English catalog. Translator wording is a human-review concern,
// not a code-test concern — we deliberately avoid asserting specific
// non-default-locale strings here so a future translation refinement
// doesn't break a green test.

// TestBuildNotification_RoutesThroughLocalizer is the load-bearing
// regression guard for the locale-routing path: a small representative
// set of event types is rendered through both the default and a
// non-default Localizer, asserting the localizer is consulted
// (outputs differ), params round-trip, and entity names never
// translate. Adding another locale doesn't need another test file —
// catalog parity handles completeness; this test handles plumbing.
func TestBuildNotification_RoutesThroughLocalizer(t *testing.T) {
	s, f := seedBuildNotification(t)
	ctx := context.Background()

	tests := []struct {
		name              string
		event             *models.CommunityEvent
		wantTitleContains []string // substrings that must appear in BOTH locales (never-translated names)
		wantBodyContains  []string // substrings that must appear in BOTH locales
	}{
		{
			name: "TRANSFER_INTEREST_EXPRESSED (params: gear name + actor name)",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
				GearId:      f.gearID,
			},
			wantTitleContains: []string{"Cordless Drill"},
			wantBodyContains:  []string{"Alice Actor", "Cordless Drill"},
		},
		{
			name: "EXPERIENCE_CREATED (params: actor name + event name)",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
				Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
			},
			wantBodyContains: []string{"Alice Actor", "Trail Day"},
		},
		{
			name: "EXPERIENCE_STARTED (entity name round-trips inside the body)",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED,
				Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
			},
			// The body is the self-contained sentence and names the event;
			// the name itself is not translated, so it must appear in both
			// locales. The title is now a localized category label.
			wantBodyContains: []string{"Trail Day"},
		},
		{
			name: "Unrecognized event type uses the default-fallback catalog key",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_UNSPECIFIED,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			en := buildNotification(ctx, s, tt.event, enLoc(t))
			other := buildNotification(ctx, s, tt.event, esLoc(t))
			if en == nil || other == nil {
				t.Fatalf("buildNotification returned nil; en=%v other=%v", en, other)
			}

			// Title or body MUST differ between locales for an event
			// the catalog actually localizes. If both are identical
			// the localizer isn't routing. Entity names round-trip
			// verbatim inside the body, so we assert "differs
			// somewhere" rather than "both differ".
			if en.Title == other.Title && en.Body == other.Body {
				t.Errorf("title AND body identical across locales — localizer not consulted; title=%q body=%q",
					en.Title, en.Body)
			}

			// Substrings that should appear in BOTH locales (param
			// names that round-trip verbatim regardless of locale).
			for _, sub := range tt.wantTitleContains {
				if !strings.Contains(en.Title, sub) {
					t.Errorf("en title missing %q: got %q", sub, en.Title)
				}
				if !strings.Contains(other.Title, sub) {
					t.Errorf("other-locale title missing %q: got %q", sub, other.Title)
				}
			}
			for _, sub := range tt.wantBodyContains {
				if !strings.Contains(en.Body, sub) {
					t.Errorf("en body missing %q: got %q", sub, en.Body)
				}
				if !strings.Contains(other.Body, sub) {
					t.Errorf("other-locale body missing %q: got %q", sub, other.Body)
				}
			}
		})
	}
}

// TestLoadRecipientLocales verifies the per-recipient locale lookup
// helper returns the stored preferred_language for each user and
// falls through to DefaultTag for users with no recorded preference
// or a missing record. Locale-mechanism test (not copy content), so
// it lives alongside the other locale-routing tests.
func TestLoadRecipientLocales(t *testing.T) {
	s := setupTestStorage(t)
	ctx := context.Background()

	enUser := insertTestUserWithLanguage(t, s, "en@example.com", "EN", "en")
	esUser := insertTestUserWithLanguage(t, s, "es@example.com", "ES", "es")
	noLangUser := insertTestUser(t, s, "none@example.com", "None")

	got := loadRecipientLocales(ctx, s, []string{enUser, esUser, noLangUser, "missing-id"})

	if got[enUser].String() != "en" {
		t.Errorf("en user → %v, want en", got[enUser])
	}
	if got[esUser].String() != "es" {
		t.Errorf("es user → %v, want es", got[esUser])
	}
	if got[noLangUser].String() != "en" {
		t.Errorf("no-pref user → %v, want en (default)", got[noLangUser])
	}
	if got["missing-id"].String() != "en" {
		t.Errorf("missing user → %v, want en (default)", got["missing-id"])
	}
}

func insertTestUserWithLanguage(t *testing.T, s *storage.ProtoSQLStorage, email, name, lang string) string {
	t.Helper()
	id, err := s.Insert(context.Background(), &models.User{
		Email:             email,
		Name:              name,
		Role:              models.Role_ROLE_USER,
		PreferredLanguage: &lang,
	})
	if err != nil {
		t.Fatalf("insertTestUserWithLanguage: %v", err)
	}
	return id
}
