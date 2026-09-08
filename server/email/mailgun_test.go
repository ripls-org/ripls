package email

import (
	"bytes"
	"context"
	"image"
	_ "image/png"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestNewMailgunService(t *testing.T) {
	svc, err := NewMailgunService("example.com", "test-api-key", "noreply@example.com")
	if err != nil {
		t.Fatalf("NewMailgunService returned unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("NewMailgunService returned nil service")
	}
}

func TestMockEmailService(t *testing.T) {
	mock := &MockEmailService{}

	err := mock.SendPasswordReset(
		context.Background(),
		"test@example.com",
		"https://app.ripls.com/reset-password?token=abc123",
		"",
	)
	if err != nil {
		t.Fatalf("Mock service should not error: %v", err)
	}

	if len(mock.ResetEmails) != 1 {
		t.Fatalf("Expected 1 sent email, got %d", len(mock.ResetEmails))
	}

	sent := mock.ResetEmails[0]
	if sent.ToEmail != "test@example.com" {
		t.Errorf("Expected ToEmail=test@example.com, got %s", sent.ToEmail)
	}

	if sent.ResetURL != "https://app.ripls.com/reset-password?token=abc123" {
		t.Errorf("Expected specific URL, got %s", sent.ResetURL)
	}
}

// allHTMLTemplates is every embedded HTML email template, keyed by a name the
// failure output can point at. The template-wide guards below all range over
// this one map: when they each carried their own copy, a newly added template
// could be listed in one and forgotten in the other, which is precisely the
// copy-paste failure mode those guards exist to catch.
var allHTMLTemplates = map[string]string{
	"email_code_en":       emailCodeTemplateENHTML,
	"email_code_es":       emailCodeTemplateESHTML,
	"password_reset_en":   passwordResetTemplateENHTML,
	"password_reset_es":   passwordResetTemplateESHTML,
	"waitlist_welcome_en": waitlistWelcomeTemplateENHTML,
	"waitlist_welcome_es": waitlistWelcomeTemplateESHTML,
	"notification_en":     notificationTemplateENHTML,
	"notification_es":     notificationTemplateESHTML,
}

// imgTagRE matches a single <img> element so a style declaration can be
// attributed to the image it is on, rather than to anything else in the
// template that happens to sit nearby.
var imgTagRE = regexp.MustCompile(`(?s)<img\b[^>]*>`)

// autoMarginRE matches any margin property — shorthand or longhand — resolving
// to `auto`, e.g. `margin: 0 auto 20px` or `margin-left: auto`.
var autoMarginRE = regexp.MustCompile(`margin[a-z-]*\s*:[^;"]*\bauto\b`)

// The logo is embedded, not hotlinked. A remote <img> is a read receipt — the
// recipient's client fetches it on open, telling us who read what and when —
// and it renders as a broken box in the many clients that block remote content
// by default. Neither is acceptable on the emails people must act on.
//
// This guards both halves: no template may reintroduce a remote logo, and the
// embedded asset must actually be a usable image.
func TestTemplates_EmbedLogoRatherThanHotlinking(t *testing.T) {
	for name, tmpl := range allHTMLTemplates {
		if strings.Contains(tmpl, "example.com/assets/logo") {
			t.Errorf("%s hotlinks the logo; use the embedded cid:logo.png so opening the mail does not call home", name)
		}
	}

	if len(logoPNG) == 0 {
		t.Fatal("the embedded logo is empty; every email would render a broken image")
	}
	// Decode rather than trust the bytes: a truncated or mis-embedded asset
	// would still be non-empty and would still render broken.
	cfg, format, err := image.DecodeConfig(bytes.NewReader(logoPNG))
	if err != nil {
		t.Fatalf("embedded logo is not a decodable image: %v", err)
	}
	if format != "png" {
		t.Errorf("embedded logo format = %q, want png", format)
	}
	// It renders at 40–80px and rides on every message; an oversized source is
	// bytes spent on every recipient.
	if cfg.Width > 256 || cfg.Height > 256 {
		t.Errorf("embedded logo is %dx%d — larger than needed for a 40-80px render", cfg.Width, cfg.Height)
	}
	if len(logoPNG) > 20*1024 {
		t.Errorf("embedded logo is %d bytes; it is attached to every email sent", len(logoPNG))
	}
}

// Email clients disagree about how a block-level image may be centered. Apple
// Mail honors width, height and display on a block <img> but drops its `auto`
// margins, so `display: block; margin: 0 auto` — which centers correctly in
// Gmail and in every browser — laid the logo flush against the left content
// edge of six of these templates at once (#2927).
//
// Two idioms survive everywhere: an `align="center"` presentation table around
// the image, or an inline-block image inheriting `text-align: center` from its
// parent. This guards in both directions — no template may pair a block image
// with auto margins again, and every template that shows the logo must use one
// of the two safe idioms, so the defect cannot return by dropping the centering
// either. It reached six templates the first time because the header is
// copy-pasted rather than shared; until that is factored, this is what catches
// the seventh copy.
func TestTemplates_CenterTheLogoWithoutAutoMargins(t *testing.T) {
	sources := map[string]string{"partials": sharedPartialsHTML}
	for name, tmpl := range allHTMLTemplates {
		sources[name] = tmpl
	}
	for name, src := range sources {
		for _, tag := range imgTagRE.FindAllString(src, -1) {
			if strings.Contains(tag, "display: block") && autoMarginRE.MatchString(tag) {
				t.Errorf("%s centers a block <img> with an auto margin, which Apple Mail ignores (#2927); "+
					"wrap it in an align=\"center\" table or make it display: inline-block under a centered parent:\n%s", name, tag)
			}
		}
	}

	// The logo lives in the shared partials. A template that inlines its own
	// copy has stepped outside the one place this rule is enforced, which is
	// how the defect reached six templates in the first place.
	for name, tmpl := range allHTMLTemplates {
		if strings.Contains(tmpl, "cid:logo.png") {
			t.Errorf("%s inlines its own logo <img>; call {{template \"logo\" .}} or "+
				"{{template \"logo_brandbar\" .}} so the centering fix stays in one place", name)
		}
	}

	// Assert on rendered output too: the partial indirection means the raw
	// template text no longer shows what the recipient's client receives.
	for name, body := range renderHTMLBodies(t) {
		if !strings.Contains(body, `align="center"`) {
			t.Errorf("rendered %s has no align=\"center\" logo table", name)
		}
		for _, tag := range imgTagRE.FindAllString(body, -1) {
			if strings.Contains(tag, "display: block") && autoMarginRE.MatchString(tag) {
				t.Errorf("rendered %s centers a block <img> with an auto margin (#2927):\n%s", name, tag)
			}
		}
	}
}

// Apple Mail invents a dark rendering for any message that does not declare
// one, by inverting the light styles — which is how #2927's screenshot came
// back dark. Declaring color-scheme switches that off, so the declaration and
// the replacement rules have to travel together: shipping the first without
// the second would hand dark-mode readers a full-brightness email.
//
// Because the light palette is inline and the dark rules arrive in a
// stylesheet, an element is only themed if it carries the class the rules key
// on. This asserts exactly that — every rendered element painted with a token
// that changes between themes names an r- class — so a new colored element
// cannot quietly stay light while everything around it goes dark.
func TestTemplates_DeclareDarkModeForEveryColoredElement(t *testing.T) {
	// Tokens whose value differs between the palettes. A token that is the
	// same in both needs no override and no class.
	themed := map[string]string{
		"Background":    LightColors.Background,
		"Surface":       LightColors.Surface,
		"Border":        LightColors.Border,
		"TextPrimary":   LightColors.TextPrimary,
		"TextSecondary": LightColors.TextSecondary,
		"TextFaint":     LightColors.TextFaint,
		"Primary":       LightColors.Primary,
		"PrimaryHover":  LightColors.PrimaryHover,
		"OnPrimary":     LightColors.OnPrimary,
		"Info":          LightColors.Info,
	}

	for name, body := range renderHTMLBodies(t) {
		if !strings.Contains(body, `name="color-scheme"`) {
			t.Errorf("rendered %s does not declare color-scheme; Apple Mail will invert it instead", name)
		}
		if !strings.Contains(body, "prefers-color-scheme: dark") {
			t.Errorf("rendered %s declares color-scheme but ships no dark rules; "+
				"dark-mode readers would get the light email at full brightness", name)
		}
		if !strings.Contains(body, DarkColors.Background) {
			t.Errorf("rendered %s carries no dark palette values", name)
		}

		for _, tag := range styledTagRE.FindAllString(body, -1) {
			if strings.Contains(tag, `class="r-`) {
				continue
			}
			for token, hex := range themed {
				if strings.Contains(tag, hex) {
					t.Errorf("rendered %s paints an element with %s (%s) but names no r- class, "+
						"so it stays light while the rest of the email goes dark:\n%s", name, token, hex, tag)
					break
				}
			}
		}
	}
}

// styledTagRE matches an opening tag carrying a style attribute. Styles are
// pretty-printed across several lines in the templates, so the dot has to span
// newlines.
var styledTagRE = regexp.MustCompile(`(?s)<[a-zA-Z][a-zA-Z0-9]*\s[^<>]*style="[^"]*"[^<>]*>`)

// renderHTMLBodies renders the HTML part of every email whose inputs are cheap
// to fabricate, keyed by flow and locale. Rendering needs no network: the
// Mailgun client is only consulted on send.
func renderHTMLBodies(t *testing.T) map[string]string {
	t.Helper()

	svc, err := NewMailgunService("example.com", "test-api-key", "noreply@example.com")
	if err != nil {
		t.Fatalf("NewMailgunService: %v", err)
	}
	ctx := context.Background()
	out := make(map[string]string)

	for _, lang := range []string{"en", "es"} {
		html, _, _, _, err := svc.RenderEmailCode(ctx, "123456", 10*time.Minute, lang)
		if err != nil {
			t.Fatalf("RenderEmailCode(%s): %v", lang, err)
		}
		out["email_code_"+lang] = html

		html, _, _, _, err = svc.RenderPasswordReset(ctx, "https://example.com/reset?token=t", lang)
		if err != nil {
			t.Fatalf("RenderPasswordReset(%s): %v", lang, err)
		}
		out["password_reset_"+lang] = html
	}

	html, _, _, _, err := svc.RenderWaitlistWelcome(ctx)
	if err != nil {
		t.Fatalf("RenderWaitlistWelcome: %v", err)
	}
	out["waitlist_welcome"] = html

	return out
}
