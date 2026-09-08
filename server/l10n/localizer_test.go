package l10n

import (
	"context"
	"testing"

	"golang.org/x/text/language"
)

func TestNewLocalizer_FallsThroughForUnsupportedTag(t *testing.T) {
	loc, err := NewLocalizer(language.French)
	if err != nil {
		t.Fatalf("NewLocalizer: %v", err)
	}
	if loc.Tag() != DefaultTag {
		t.Errorf("expected unsupported tag to normalize to DefaultTag; got %v", loc.Tag())
	}
}

func TestNewLocalizerForContext_UsesResolvedTag(t *testing.T) {
	ctx := WithLocale(context.Background(), "es")
	loc, err := NewLocalizerForContext(ctx)
	if err != nil {
		t.Fatalf("NewLocalizerForContext: %v", err)
	}
	if loc.Tag() != language.Spanish {
		t.Errorf("expected es; got %v", loc.Tag())
	}
}

func TestLocalizer_MissingKeyFallsBackToID(t *testing.T) {
	ResetMissLogForTest()
	loc, err := NewLocalizer(language.English)
	if err != nil {
		t.Fatalf("NewLocalizer: %v", err)
	}
	// No keys are loaded in the Phase 0 empty catalog, so every key
	// is "missing" — the call must not panic and must return a
	// stable visible fallback (the key itself).
	got := loc.T(context.Background(), "notif.does_not_exist.title", nil)
	if got != "notif.does_not_exist.title" {
		t.Errorf("missing-key fallback = %q, want the key string", got)
	}
}

func TestLocalizer_NilSafeT(t *testing.T) {
	// A nil Localizer should not panic when called; render the key.
	var loc *Localizer
	if got := loc.T(context.Background(), "any.key", nil); got != "any.key" {
		t.Errorf("nil Localizer.T = %q, want key string", got)
	}
}

func TestLocalizer_WarnOnceForSameKey(t *testing.T) {
	// Calling T twice for the same key should only log one warning.
	// The actual log assertion is indirect: we rely on the sync.Map
	// state in missLog. Reset, call once → first miss stored;
	// call again → already stored.
	ResetMissLogForTest()
	loc, err := NewLocalizer(language.English)
	if err != nil {
		t.Fatalf("NewLocalizer: %v", err)
	}
	_ = loc.T(context.Background(), "warn.once.key", nil)

	k := missKey{locale: language.English.String(), id: "warn.once.key"}
	if _, ok := missLog.Load(k); !ok {
		t.Fatalf("first miss should be recorded in missLog")
	}

	// Second call should be a no-op for the gate.
	_ = loc.T(context.Background(), "warn.once.key", nil)

	count := 0
	missLog.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count != 1 {
		t.Errorf("expected exactly 1 entry in missLog after duplicate call; got %d", count)
	}
}

func TestLocalizer_TCountSelectsPluralForm(t *testing.T) {
	// web.request.needs.items is a plural-bearing key (one/other) in both
	// en and es. TCount must pick the right CLDR form per locale and expose
	// Count to the template implicitly.
	cases := []struct {
		tag   language.Tag
		count int
		want  string
	}{
		{language.English, 1, "1 item"},
		{language.English, 5, "5 items"},
		{language.Spanish, 1, "1 artículo"},
		{language.Spanish, 5, "5 artículos"},
	}
	for _, tc := range cases {
		loc, err := NewLocalizer(tc.tag)
		if err != nil {
			t.Fatalf("NewLocalizer(%v): %v", tc.tag, err)
		}
		if got := loc.TCount(context.Background(), "web.request.needs.items", tc.count, nil); got != tc.want {
			t.Errorf("TCount(%v, %d) = %q, want %q", tc.tag, tc.count, got, tc.want)
		}
	}
}

func TestLocalizer_TCountNilSafe(t *testing.T) {
	var loc *Localizer
	if got := loc.TCount(context.Background(), "any.key", 3, nil); got != "any.key" {
		t.Errorf("nil Localizer.TCount = %q, want key string", got)
	}
}

func TestBundle_LoadsEmbeddedSourcesWithoutError(t *testing.T) {
	// Phase 0 ships empty TOML files; loading them must succeed.
	if _, err := Bundle(); err != nil {
		t.Fatalf("Bundle: %v", err)
	}
}
