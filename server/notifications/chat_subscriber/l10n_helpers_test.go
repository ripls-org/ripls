package chat_subscriber

import (
	"testing"

	"golang.org/x/text/language"

	"go.ripls.org/ripls/server/l10n"
)

// enLoc returns a Localizer bound to English for chat-notification
// tests that assert on English copy.
func enLoc(t *testing.T) *l10n.Localizer {
	t.Helper()
	loc, err := l10n.NewLocalizer(language.English)
	if err != nil {
		t.Fatalf("l10n.NewLocalizer(en): %v", err)
	}
	return loc
}

// esLoc returns a Localizer bound to Spanish for chat-notification
// tests that assert on Spanish copy.
func esLoc(t *testing.T) *l10n.Localizer {
	t.Helper()
	loc, err := l10n.NewLocalizer(language.Spanish)
	if err != nil {
		t.Fatalf("l10n.NewLocalizer(es): %v", err)
	}
	return loc
}
