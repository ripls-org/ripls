package email

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/text/language"

	"go.ripls.org/ripls/server/l10n"
)

// testLegalEntity stands in for the deployment's operator name. It is a
// fixture, not our own: the templates take the name as data now (#2953), so a
// literal here would silently stop proving anything the day the plumbing broke.
const testLegalEntity = "Example Foundation"

func newTestService(t *testing.T) *MailgunService {
	t.Helper()
	svc, err := NewMailgunService("example.com", "test-api-key", "noreply@example.com")
	if err != nil {
		t.Fatalf("NewMailgunService: %v", err)
	}
	svc.SetLegalEntityName(testLegalEntity)
	return svc
}

// assertContainsAll fails the test if any wanted substring is missing
// from haystack. Reports each missing substring individually so a
// translator's typo is easy to localize.
func assertContainsAll(t *testing.T, label, haystack string, wantSubstrings ...string) {
	t.Helper()
	for _, want := range wantSubstrings {
		if !strings.Contains(haystack, want) {
			t.Errorf("%s missing %q", label, want)
		}
	}
}

func TestLocaleForRecipient(t *testing.T) {
	cases := []struct {
		name      string
		ctxAccept string // Accept-Language value to seed ctx with
		recipPref string // recipient User.preferred_language
		want      language.Tag
	}{
		{name: "recipient pref wins over Accept-Language", ctxAccept: "en", recipPref: "es", want: language.Spanish},
		{name: "recipient pref empty falls through to Accept-Language", ctxAccept: "es", recipPref: "", want: language.Spanish},
		{name: "no signal falls back to default", ctxAccept: "", recipPref: "", want: l10n.DefaultTag},
		{name: "unsupported recipient pref folds to default", ctxAccept: "es", recipPref: "fr", want: l10n.DefaultTag},
		{name: "region subtag stripped on recipient pref", ctxAccept: "", recipPref: "es-MX", want: language.Spanish},
		{name: "region subtag stripped on Accept-Language", ctxAccept: "en-US", recipPref: "", want: language.English},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.ctxAccept != "" {
				ctx = l10n.WithAcceptLanguage(ctx, tc.ctxAccept)
			}
			got := localeForRecipient(ctx, tc.recipPref)
			if got != tc.want {
				t.Errorf("localeForRecipient(_, %q) with Accept-Language %q = %s; want %s",
					tc.recipPref, tc.ctxAccept, got, tc.want)
			}
		})
	}
}

func TestRenderPasswordReset_EnglishDefault(t *testing.T) {
	svc := newTestService(t)
	html, text, subject, tag, err := svc.RenderPasswordReset(context.Background(), "https://example.com/reset?token=abc", "")
	if err != nil {
		t.Fatalf("RenderPasswordReset: %v", err)
	}
	if tag != l10n.DefaultTag {
		t.Errorf("tag = %s; want %s", tag, l10n.DefaultTag)
	}
	if subject != "Reset your Ripls password" {
		t.Errorf("subject = %q; want %q", subject, "Reset your Ripls password")
	}
	assertContainsAll(t, "password_reset HTML (en)", html,
		"Reset Your Password",
		"You requested to reset your password for your Ripls account.",
		"Reset Password",
		"This link will expire in 1 hour.",
		"https://example.com/reset?token=abc",
	)
	assertContainsAll(t, "password_reset text (en)", text,
		"Reset Your Password",
		"https://example.com/reset?token=abc",
		"This link will expire in 1 hour.",
	)
}

// TestRenderPasswordReset_RoutesThroughLocalizer exercises the locale
// pipeline for password-reset emails: when a non-default-locale
// recipient pref is set, the per-locale template + catalog actually
// get consulted (subject and body differ from the en render), the
// reset URL param round-trips in the new locale, and the English
// source text doesn't leak into the localized body. We deliberately
// don't assert specific Spanish copy — translator wording is human-
// review territory and catalog parity is CI-enforced.
func TestRenderPasswordReset_RoutesThroughLocalizer(t *testing.T) {
	svc := newTestService(t)
	resetURL := "https://example.com/reset?token=xyz"

	enHTML, enText, enSubject, enTag, err := svc.RenderPasswordReset(context.Background(), resetURL, "")
	if err != nil {
		t.Fatalf("RenderPasswordReset (en): %v", err)
	}
	otherHTML, otherText, otherSubject, otherTag, err := svc.RenderPasswordReset(context.Background(), resetURL, "es")
	if err != nil {
		t.Fatalf("RenderPasswordReset (other): %v", err)
	}

	if enTag == otherTag {
		t.Errorf("tag identical across locales (%s); recipient-pref routing broken", enTag)
	}
	if enSubject == otherSubject {
		t.Errorf("subject identical across locales (%q); subject catalog not consulted", enSubject)
	}
	if enHTML == otherHTML || enText == otherText {
		t.Errorf("body identical across locales; per-locale template not selected")
	}
	if !strings.Contains(otherHTML, resetURL) || !strings.Contains(otherText, resetURL) {
		t.Errorf("reset URL dropped from non-default-locale render: html=%q text=%q", otherHTML, otherText)
	}
	// English source-of-truth strings shouldn't leak into the
	// non-default-locale body. Catches the regression where a
	// non-default render accidentally falls back to the en template
	// — the en headline would appear instead of the localized one.
	for _, leak := range []string{"Reset Your Password", "You requested to reset"} {
		if strings.Contains(otherHTML, leak) {
			t.Errorf("English source string %q leaked into non-default-locale render", leak)
		}
	}
}

// TestRenderPasswordReset_RecipientPrefBeatsAcceptLanguage asserts
// the email locale-priority chain: when both signals are present, the
// recipient's stored preferred_language wins (the email is read by
// the recipient, who may not be the requester). Mechanism check —
// compares tags, doesn't assert any specific copy.
func TestRenderPasswordReset_RecipientPrefBeatsAcceptLanguage(t *testing.T) {
	svc := newTestService(t)
	// Two non-default locales would be ideal — for now we have only
	// es shipping, so we use Accept-Language=en (which would match
	// default) vs recipient-pref=es. If en accidentally wins, the
	// returned tag would be DefaultTag instead of Spanish.
	ctx := l10n.WithAcceptLanguage(context.Background(), "en")
	_, _, _, tag, err := svc.RenderPasswordReset(ctx, "https://example.com/reset?token=z", "es")
	if err != nil {
		t.Fatalf("RenderPasswordReset: %v", err)
	}
	if tag != language.Spanish {
		t.Errorf("recipient pref should beat Accept-Language; got tag %s, want %s", tag, language.Spanish)
	}
}

func TestRenderWaitlistWelcome_EnglishDefault(t *testing.T) {
	svc := newTestService(t)
	html, text, subject, tag, err := svc.RenderWaitlistWelcome(context.Background())
	if err != nil {
		t.Fatalf("RenderWaitlistWelcome: %v", err)
	}
	if tag != l10n.DefaultTag {
		t.Errorf("tag = %s; want %s", tag, l10n.DefaultTag)
	}
	if subject != "Welcome to the Ripls community" {
		t.Errorf("subject = %q; want %q", subject, "Welcome to the Ripls community")
	}
	assertContainsAll(t, "waitlist_welcome HTML (en)", html,
		"You're on the list",
		"Thanks for signing up.",
		testLegalEntity,
	)
	assertContainsAll(t, "waitlist_welcome text (en)", text,
		"You're on the list",
		testLegalEntity,
	)
}

// TestRenderWaitlistWelcome_RoutesThroughLocalizer exercises the
// waitlist-welcome localization pipeline via Accept-Language (the
// only signal available for waitlist signups since no User row
// exists yet). Mechanism check only — no specific non-default-locale
// copy asserted.
func TestRenderWaitlistWelcome_RoutesThroughLocalizer(t *testing.T) {
	svc := newTestService(t)

	enHTML, enText, enSubject, enTag, err := svc.RenderWaitlistWelcome(context.Background())
	if err != nil {
		t.Fatalf("RenderWaitlistWelcome (en): %v", err)
	}
	otherCtx := l10n.WithAcceptLanguage(context.Background(), "es")
	otherHTML, otherText, otherSubject, otherTag, err := svc.RenderWaitlistWelcome(otherCtx)
	if err != nil {
		t.Fatalf("RenderWaitlistWelcome (other): %v", err)
	}

	if enTag == otherTag {
		t.Errorf("tag identical across locales (%s); Accept-Language routing broken", enTag)
	}
	if enSubject == otherSubject {
		t.Errorf("subject identical across locales (%q); subject catalog not consulted", enSubject)
	}
	if enHTML == otherHTML || enText == otherText {
		t.Errorf("body identical across locales; per-locale template not selected")
	}
	// The operator name is not translated, so it round-trips in every locale —
	// both renders must include it.
	if !strings.Contains(enHTML, testLegalEntity) || !strings.Contains(otherHTML, testLegalEntity) {
		t.Errorf("operator name dropped: en=%q other=%q", enHTML, otherHTML)
	}
	// English source-of-truth strings shouldn't leak into the
	// non-default-locale body.
	for _, leak := range []string{"You're on the list", "Thanks for signing up"} {
		if strings.Contains(otherHTML, leak) {
			t.Errorf("English source string %q leaked into non-default-locale render", leak)
		}
	}
}

func TestRenderWaitlistWelcome_UnsupportedAcceptLanguageFallsBackToEnglish(t *testing.T) {
	svc := newTestService(t)
	ctx := l10n.WithAcceptLanguage(context.Background(), "fr-FR")
	_, _, subject, tag, err := svc.RenderWaitlistWelcome(ctx)
	if err != nil {
		t.Fatalf("RenderWaitlistWelcome: %v", err)
	}
	if tag != l10n.DefaultTag {
		t.Errorf("unsupported fr should fall back to %s, got %s", l10n.DefaultTag, tag)
	}
	if subject != "Welcome to the Ripls community" {
		t.Errorf("fallback should render en subject, got %q", subject)
	}
}

// TestRenderWaitlistWelcome_OmitsEntityWhenUnset proves an unconfigured
// operator name drops the sign-off line rather than rendering a blank one or
// the literal template (#2953). Signing a fork's mail with someone else's
// legal name would be worse than signing it with nothing, which is why there
// is no default to fall back to.
func TestRenderWaitlistWelcome_OmitsEntityWhenUnset(t *testing.T) {
	svc, err := NewMailgunService("example.com", "test-api-key", "noreply@example.com")
	if err != nil {
		t.Fatalf("NewMailgunService: %v", err)
	}
	// SetLegalEntityName deliberately not called.

	html, text, _, _, err := svc.RenderWaitlistWelcome(context.Background())
	if err != nil {
		t.Fatalf("RenderWaitlistWelcome: %v", err)
	}
	for label, body := range map[string]string{"HTML": html, "text": text} {
		if strings.Contains(body, "{{") {
			t.Errorf("%s body leaked an unrendered template action: %q", label, body)
		}
		// The welcome copy itself is not conditional — only the sign-off is.
		if !strings.Contains(body, "You're on the list") {
			t.Errorf("%s body lost its copy along with the sign-off: %q", label, body)
		}
	}
}
