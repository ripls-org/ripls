package l10n

import (
	"context"
	"testing"

	"golang.org/x/text/language"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want language.Tag
	}{
		{"empty falls back to default", "", DefaultTag},
		{"whitespace falls back to default", "   ", DefaultTag},
		{"english base", "en", language.English},
		{"spanish base", "es", language.Spanish},
		{"english region stripped", "en-US", language.English},
		{"spanish region stripped", "es-MX", language.Spanish},
		{"upper case folded", "EN", language.English},
		{"mixed case folded", "Es-mx", language.Spanish},
		{"unsupported tag falls back", "fr", DefaultTag},
		{"unsupported region tag falls back", "fr-CA", DefaultTag},
		{"garbage falls back", "not-a-tag-!!", DefaultTag},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Normalize(tt.in); got != tt.want {
				t.Errorf("Normalize(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestLocaleFromContext_Priority(t *testing.T) {
	// Explicit WithLocale beats every other signal.
	t.Run("explicit locale wins", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithPreferredLanguage(ctx, "en")
		ctx = WithAcceptLanguage(ctx, "en-US,en;q=0.9")
		ctx = WithLocale(ctx, "es")
		if got := LocaleFromContext(ctx); got != language.Spanish {
			t.Errorf("expected es from explicit override; got %v", got)
		}
	})

	t.Run("accept-language beats preferred", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithPreferredLanguage(ctx, "en")
		ctx = WithAcceptLanguage(ctx, "es-MX,en;q=0.9")
		if got := LocaleFromContext(ctx); got != language.Spanish {
			t.Errorf("expected es from Accept-Language; got %v", got)
		}
	})

	t.Run("preferred wins when no accept-language", func(t *testing.T) {
		ctx := context.Background()
		ctx = WithPreferredLanguage(ctx, "es")
		if got := LocaleFromContext(ctx); got != language.Spanish {
			t.Errorf("expected es from preferred_language; got %v", got)
		}
	})

	t.Run("default when nothing is set", func(t *testing.T) {
		if got := LocaleFromContext(context.Background()); got != DefaultTag {
			t.Errorf("expected DefaultTag; got %v", got)
		}
	})

	t.Run("nil context returns default", func(t *testing.T) {
		var nilCtx context.Context
		if got := LocaleFromContext(nilCtx); got != DefaultTag {
			t.Errorf("expected DefaultTag for nil ctx; got %v", got)
		}
	})
}

func TestLocaleFromContext_AcceptLanguageParsing(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   language.Tag
	}{
		{"single tag", "es", language.Spanish},
		{"first wins with q values", "es;q=1.0,en;q=0.8", language.Spanish},
		{"unsupported first falls back to default (no q-weighting in v1)", "fr,es;q=0.8", DefaultTag},
		{"whitespace around items", "  es-MX  , en;q=0.5", language.Spanish},
		{"empty header", "", DefaultTag},
		{"only separators", " , ", DefaultTag},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := WithAcceptLanguage(context.Background(), tt.header)
			if got := LocaleFromContext(ctx); got != tt.want {
				t.Errorf("Accept-Language=%q → %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func TestWithHelpers_IgnoreBlankValues(t *testing.T) {
	// Empty strings should be no-ops, not stored as a sentinel that
	// LocaleFromContext might then try to parse.
	ctx := context.Background()
	ctx = WithLocale(ctx, "")
	ctx = WithLocale(ctx, "   ")
	ctx = WithAcceptLanguage(ctx, "")
	ctx = WithPreferredLanguage(ctx, "")
	if got := LocaleFromContext(ctx); got != DefaultTag {
		t.Errorf("expected DefaultTag after only blank values; got %v", got)
	}
}

func TestSupported_IncludesEnAndEs(t *testing.T) {
	supported := Supported()
	wantInclude := map[language.Tag]bool{
		language.English: false,
		language.Spanish: false,
	}
	for _, tag := range supported {
		if _, ok := wantInclude[tag]; ok {
			wantInclude[tag] = true
		}
	}
	for tag, found := range wantInclude {
		if !found {
			t.Errorf("Supported() missing %v", tag)
		}
	}
}
