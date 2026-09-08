package email

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/text/language"

	"go.ripls.org/ripls/server/l10n"
)

func TestRenderNotification_English(t *testing.T) {
	svc := newTestService(t)
	htmlBody, textBody, subject, tag, err := svc.RenderNotification(context.Background(), NotificationEmailInput{
		ToEmail:           "deviceless@example.com",
		PreferredLanguage: "en",
		Title:             "Alex said yes",
		Body:              "See who is coming.",
		ActionURL:         "https://example.com",
		UnsubscribeURL:    "https://example.com/email/unsubscribe?u=u1&token=abc",
	})
	if err != nil {
		t.Fatalf("RenderNotification: %v", err)
	}
	if tag != language.English {
		t.Errorf("tag = %v, want English", tag)
	}
	// Subject is the already-localized notification title.
	if subject != "Alex said yes" {
		t.Errorf("subject = %q, want the title", subject)
	}
	// Title/body, the CTA, and the unsubscribe link all appear in both bodies.
	// (Both templates render via html/template, so the URL's "&" is escaped;
	// assert the un-ampersanded prefix to stay agnostic to that.)
	assertContainsAll(t, "html", htmlBody,
		"Alex said yes", "See who is coming.",
		"/email/unsubscribe?u=u1", "Open Ripls", "Unsubscribe")
	assertContainsAll(t, "text", textBody,
		"Alex said yes", "See who is coming.", "/email/unsubscribe?u=u1")
}

func TestRenderNotification_SpanishFooter(t *testing.T) {
	svc := newTestService(t)
	htmlBody, _, _, tag, err := svc.RenderNotification(context.Background(), NotificationEmailInput{
		ToEmail:           "x@example.com",
		PreferredLanguage: "es",
		Title:             "Título",
		Body:              "Cuerpo",
		UnsubscribeURL:    "https://example.com/email/unsubscribe?u=u1&token=abc",
	})
	if err != nil {
		t.Fatalf("RenderNotification: %v", err)
	}
	if tag != language.Spanish {
		t.Errorf("tag = %v, want Spanish", tag)
	}
	assertContainsAll(t, "es html", htmlBody, "Cancela la suscripción", "Título", "Cuerpo")
}

// TestRenderNotification_PlainTextNotHTMLEscaped proves the text/plain part
// renders literal apostrophes and ampersands, not HTML entities — the text
// templates are parsed with text/template, not html/template. (The html part is
// correctly escaped; only the plain-text alternative is asserted here.)
func TestRenderNotification_PlainTextNotHTMLEscaped(t *testing.T) {
	svc := newTestService(t)
	_, textBody, _, _, err := svc.RenderNotification(context.Background(), NotificationEmailInput{
		ToEmail:        "x@example.com",
		Title:          "New Event",
		Body:           "Alex created 'Backyard BBQ' & invited you",
		UnsubscribeURL: "https://example.com/email/unsubscribe?u=u1&token=abc",
	})
	if err != nil {
		t.Fatalf("RenderNotification: %v", err)
	}
	for _, entity := range []string{"&#39;", "&amp;", "&#34;"} {
		if strings.Contains(textBody, entity) {
			t.Errorf("text/plain part contains HTML entity %q (should be literal):\n%s", entity, textBody)
		}
	}
	assertContainsAll(t, "text", textBody, "'Backyard BBQ'", "& invited you")
}

// TestRenderNotification_Summary proves the rich-card path: when a Summary is
// present the entity name becomes the heading (distinct from the action-sentence
// Title/subject), the hero image and per-field facts render, and the plain-text
// part carries the same detail.
func TestRenderNotification_Summary(t *testing.T) {
	svc := newTestService(t)
	htmlBody, textBody, subject, _, err := svc.RenderNotification(context.Background(), NotificationEmailInput{
		PreferredLanguage: "en",
		Title:             "Sam is attending Backyard BBQ", // action sentence == subject
		ActionURL:         "https://example.com/go/abc",
		CTALabel:          "Manage RSVPs",
		HeroImageURL:      "cid:hero.jpg",
		Summary: &ItemSummary{
			Eyebrow:     "Event",
			Name:        "Backyard BBQ",
			OwnerLine:   "Hosted by Alex",
			Date:        "June 27",
			Going:       "12 going",
			Description: "Burgers and lawn games.",
		},
	})
	if err != nil {
		t.Fatalf("RenderNotification: %v", err)
	}
	// Subject is the action sentence; heading is the entity name — they differ.
	if subject != "Sam is attending Backyard BBQ" {
		t.Errorf("subject = %q", subject)
	}
	assertContainsAll(t, "html", htmlBody,
		`src="cid:hero.jpg"`, "Event", "Backyard BBQ", "Sam is attending Backyard BBQ",
		"Hosted by Alex", "June 27", "12 going", "Burgers and lawn games.", "Manage RSVPs")
	assertContainsAll(t, "text", textBody,
		"Backyard BBQ", "Hosted by Alex", "12 going", "Burgers and lawn games.")
}

// TestRenderNotification_PostalAddress proves the CAN-SPAM footer renders the
// configured postal address in both bodies and both locales (it is the single
// source of truth, set via SetPostalAddress, and is not translated).
func TestRenderNotification_PostalAddress(t *testing.T) {
	const addr = "Example Foundation, PBC · 100 Main Street, Ste 1, Denver, CO 80203"
	svc := newTestService(t)
	svc.SetPostalAddress(addr)

	for _, lang := range []string{"en", "es"} {
		htmlBody, textBody, _, _, err := svc.RenderNotification(context.Background(), NotificationEmailInput{
			ToEmail:           "x@example.com",
			PreferredLanguage: lang,
			Title:             "T",
			Body:              "B",
		})
		if err != nil {
			t.Fatalf("RenderNotification(%s): %v", lang, err)
		}
		assertContainsAll(t, lang+" html", htmlBody, addr)
		assertContainsAll(t, lang+" text", textBody, addr)
	}
}

// TestRenderNotification_PostalAddressOmittedWhenEmpty proves an unset address
// renders no dangling footer line (the channel ships flag-off until it's set).
func TestRenderNotification_PostalAddressOmittedWhenEmpty(t *testing.T) {
	svc := newTestService(t) // SetPostalAddress not called → empty
	htmlBody, _, _, _, err := svc.RenderNotification(context.Background(), NotificationEmailInput{
		ToEmail: "x@example.com",
		Title:   "T",
		Body:    "B",
	})
	if err != nil {
		t.Fatalf("RenderNotification: %v", err)
	}
	if strings.Contains(htmlBody, "Example Foundation") {
		t.Error("footer rendered an entity/address line despite empty PostalAddress")
	}
}

func TestRenderNotification_OptionalFieldsOmitted(t *testing.T) {
	svc := newTestService(t)
	htmlBody, _, _, _, err := svc.RenderNotification(context.Background(), NotificationEmailInput{
		ToEmail: "x@example.com",
		Title:   "Heads up",
		Body:    "Something happened.",
		// No ActionURL, no UnsubscribeURL.
	})
	if err != nil {
		t.Fatalf("RenderNotification: %v", err)
	}
	if strings.Contains(htmlBody, "Open Ripls") {
		t.Error("CTA button rendered despite empty ActionURL")
	}
	if strings.Contains(htmlBody, "/email/unsubscribe") {
		t.Error("unsubscribe link rendered despite empty UnsubscribeURL")
	}
}

func TestRenderNotification_UnsupportedLocaleFallsBackToEnglish(t *testing.T) {
	svc := newTestService(t)
	_, _, _, tag, err := svc.RenderNotification(context.Background(), NotificationEmailInput{
		ToEmail:           "x@example.com",
		PreferredLanguage: "fr", // unsupported → folds to default
		Title:             "T",
		Body:              "B",
	})
	if err != nil {
		t.Fatalf("RenderNotification: %v", err)
	}
	if tag != l10n.DefaultTag {
		t.Errorf("tag = %v, want default (%v)", tag, l10n.DefaultTag)
	}
}
