package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/middleware"
)

// wantAll fails if any of subs is absent from body.
func wantAll(t *testing.T, body string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(body, s) {
			t.Errorf("missing expected substring %q", s)
		}
	}
}

// wantNone fails if any of subs is present in body (used to prove the other
// locale's copy didn't leak).
func wantNone(t *testing.T, body string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if strings.Contains(body, s) {
			t.Errorf("unexpected substring %q present", s)
		}
	}
}

// serveGoPage drives HandleInvitePage through the AcceptLanguage middleware,
// mirroring the production /go/ route (routes.go): the middleware captures the
// request's Accept-Language into context, where the handler's localizer reads
// it. Calling the handler directly would bypass locale resolution.
func serveGoPage(svc *Service, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	middleware.AcceptLanguage(http.HandlerFunc(svc.HandleInvitePage)).ServeHTTP(w, req)
	return w
}

// TestSSRErrorPage_LocalizedByAcceptLanguage verifies the shared invite.html
// error surface renders its message in the visitor's Accept-Language locale
// (via the server l10n catalog), and stamps the matching <html lang>. The
// /go/ route is wrapped in middleware.AcceptLanguage in production; these
// tests set the header directly on the request the handler reads.
func TestSSRErrorPage_LocalizedByAcceptLanguage(t *testing.T) {
	svc, _, _ := setupTestService(t)

	cases := []struct {
		name       string
		acceptLang string
		wantLang   string
		wantCopy   string // locale-specific substring that must appear
		notWant    string // the other locale's copy must NOT appear
	}{
		{
			name:       "default english",
			acceptLang: "",
			wantLang:   `lang="en"`,
			wantCopy:   "This invitation link is not valid.",
			notWant:    "no es válido",
		},
		{
			name:       "spanish",
			acceptLang: "es",
			wantLang:   `lang="es"`,
			wantCopy:   "Este enlace de invitación no es válido.",
			notWant:    "This invitation link is not valid.",
		},
		{
			name:       "spanish with region + q-value",
			acceptLang: "es-MX,es;q=0.9,en;q=0.8",
			wantLang:   `lang="es"`,
			wantCopy:   "Este enlace de invitación no es válido.",
			notWant:    "This invitation link is not valid.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/go/NONEXIST", nil)
			if tc.acceptLang != "" {
				req.Header.Set("Accept-Language", tc.acceptLang)
			}
			w := serveGoPage(svc, req)

			resp := w.Result()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200 (error page)", resp.StatusCode)
			}
			body := w.Body.String()
			if !strings.Contains(body, tc.wantCopy) {
				t.Errorf("missing localized error copy %q", tc.wantCopy)
			}
			if tc.notWant != "" && strings.Contains(body, tc.notWant) {
				t.Errorf("leaked other-locale copy %q", tc.notWant)
			}
			if !strings.Contains(body, tc.wantLang) {
				t.Errorf("missing %s on <html>", tc.wantLang)
			}
		})
	}
}

// TestSSREventUnavailable_LocalizedSpanish verifies the event-flavored error
// path (a share link pointing at a non-RSVPable event) renders the localized
// "event no longer available" copy rather than the English literal.
func TestSSREventUnavailable_LocalizedSpanish(t *testing.T) {
	svc, sqlStorage, _ := setupTestService(t)
	ctx := context.Background()

	host := createTestUser(t, ctx, sqlStorage, "Bob")
	community := createTestCommunity(t, ctx, sqlStorage, host.Id, nil)
	createTestMembership(t, ctx, sqlStorage, community.Id, host.Id)
	expTime := &models.ExperienceTime{
		TimeType: &models.ExperienceTime_Specific{
			Specific: &models.SpecificTime{
				UnixTimestampSec: time.Date(2027, 6, 5, 18, 0, 0, 0, time.UTC).Unix(),
				Timezone:         "UTC",
			},
		},
	}
	// A cancelled event is not RSVPable → renders the shared error surface.
	experience := createTestExperience(t, ctx, sqlStorage, host.Id, "Block Party",
		models.ExperienceState_EXPERIENCE_STATE_CANCELLED, expTime)
	shareLink := createTestExperienceShareLink(t, ctx, sqlStorage, community.Id, host.Id, experience.Id)

	req := httptest.NewRequest(http.MethodGet, "/go/"+shareLink.ShortCode, nil)
	req.Header.Set("Accept-Language", "es")
	w := serveGoPage(svc, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Result().StatusCode)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Este evento ya no está disponible.") {
		t.Error("missing localized event-unavailable copy")
	}
	if strings.Contains(body, "This event is no longer available.") {
		t.Error("leaked English event-unavailable copy on an es request")
	}
	if !strings.Contains(body, `lang="es"`) {
		t.Error(`missing lang="es" on <html>`)
	}
}

// TestSSRGearLanding_LocalizedSpanish verifies the gear landing's composed
// copy (headline, kicker, role, CTA) renders in Spanish, and — critically —
// that the loan/giveaway role line is chosen by item flavor, not by a
// string-compare against the (now localized) kicker. A giveaway must show the
// giveaway role, never the loan role: the regression the old
// {{if eq .Kicker "Free"}} sentinel would cause once "Free" became "Gratis".
func TestSSRGearLanding_LocalizedSpanish(t *testing.T) {
	serveEsGear := func(t *testing.T, availability models.Availability) string {
		t.Helper()
		svc, sqlStorage, mockBucket := setupTestService(t)
		ctx := context.Background()
		mockBucket.SignedURL = "https://storage.example.com/signed.jpg"
		owner := createTestUser(t, ctx, sqlStorage, "Dana")
		community := createTestCommunity(t, ctx, sqlStorage, owner.Id, nil)
		createTestMembership(t, ctx, sqlStorage, community.Id, owner.Id)
		gear := createTestGear(t, ctx, sqlStorage, owner.Id, "Taladro", nil, models.GearState_GEAR_STATE_AVAILABLE)
		createTestCommunityGear(t, ctx, sqlStorage, community.Id, gear.Id, availability)
		shareLink := createTestGearShareLink(t, ctx, sqlStorage, community.Id, owner.Id, gear.Id)

		req := httptest.NewRequest(http.MethodGet, "/go/"+shareLink.ShortCode, nil)
		req.Header.Set("Accept-Language", "es")
		return serveGoPage(svc, req).Body.String()
	}

	t.Run("loan", func(t *testing.T) {
		body := serveEsGear(t, models.Availability_AVAILABILITY_FOR_LOAN)
		wantAll(t, body,
			"presta esto",         // visible role phrase
			"Pedir prestado",      // kicker + CTA label
			"Dana presta Taladro", // OG/title headline
			`lang="es"`,
		)
		wantNone(t, body, "is lending", "Ask to borrow", "regala esto")
	})

	t.Run("giveaway", func(t *testing.T) {
		body := serveEsGear(t, models.Availability_AVAILABILITY_FOR_GIVEAWAY)
		wantAll(t, body,
			"regala esto", // giveaway role — proves flavor-based selection
			"Gratis",      // kicker
			"Lo quiero",   // CTA label
		)
		// The loan role must never leak onto a giveaway.
		wantNone(t, body, "presta esto", "is giving this away", "Pedir prestado")
	})
}

// TestSSRRequestLanding_LocalizedSpanish verifies the request landing's kicker,
// role, CTA, headline, and the pluralized needs-summary line all render in
// Spanish.
func TestSSRRequestLanding_LocalizedSpanish(t *testing.T) {
	svc, sqlStorage, mockBucket := setupTestService(t)
	ctx := context.Background()
	mockBucket.SignedURL = "https://storage.example.com/signed.jpg"
	requester := createTestUser(t, ctx, sqlStorage, "Dana")
	community := createTestCommunity(t, ctx, sqlStorage, requester.Id, nil)
	createTestMembership(t, ctx, sqlStorage, community.Id, requester.Id)
	request := createTestRequest(t, ctx, sqlStorage, requester.Id, "Escalera",
		models.RequestState_REQUEST_STATE_ACTIVE)
	// Two needs, one still open → "2 artículos · 1 pendiente".
	createTestRequestNeed(t, ctx, sqlStorage, request.Id, requester.Id, "n1", 1, 1)
	createTestRequestNeed(t, ctx, sqlStorage, request.Id, requester.Id, "n2", 1, 0)
	shareLink := createTestRequestShareLink(t, ctx, sqlStorage, community.Id, requester.Id, request.Id)

	req := httptest.NewRequest(http.MethodGet, "/go/"+shareLink.ShortCode, nil)
	req.Header.Set("Accept-Language", "es")
	body := serveGoPage(svc, req).Body.String()

	wantAll(t, body,
		"Se busca",                  // kicker
		"busca esto",                // role
		"Ofrecer ayuda",             // CTA
		"Dana busca Escalera",       // headline
		"2 artículos · 1 pendiente", // pluralized needs summary
		`lang="es"`,
	)
	wantNone(t, body, "is looking for this", "Offer to help", "still open")
}

// TestSSREventLanding_AttendeesLocalizedSpanish verifies the attendee overflow
// line ("+ N others going") renders as its Spanish plural ("y N más van").
func TestSSREventLanding_AttendeesLocalizedSpanish(t *testing.T) {
	svc, sqlStorage, mockBucket := setupTestService(t)
	ctx := context.Background()
	mockBucket.SignedURL = "https://storage.example.com/signed.jpg"
	host := createTestUser(t, ctx, sqlStorage, "Bob")
	community := createTestCommunity(t, ctx, sqlStorage, host.Id, nil)
	createTestMembership(t, ctx, sqlStorage, community.Id, host.Id)
	expTime := &models.ExperienceTime{
		TimeType: &models.ExperienceTime_Specific{
			Specific: &models.SpecificTime{
				UnixTimestampSec: time.Date(2027, 6, 5, 18, 0, 0, 0, time.UTC).Unix(),
				Timezone:         "UTC",
			},
		},
	}
	experience := createTestExperience(t, ctx, sqlStorage, host.Id, "Fiesta",
		models.ExperienceState_EXPERIENCE_STATE_ACTIVE, expTime)
	// 6 attendees RSVP yes → 4 shown, overflow 2.
	for i := 0; i < 6; i++ {
		u := createTestUser(t, ctx, sqlStorage, fmt.Sprintf("Guest%d", i))
		createTestMembership(t, ctx, sqlStorage, community.Id, u.Id)
		createTestRSVP(t, ctx, sqlStorage, experience.Id, community.Id, u.Id, int64(1000+i))
	}
	shareLink := createTestExperienceShareLink(t, ctx, sqlStorage, community.Id, host.Id, experience.Id)

	req := httptest.NewRequest(http.MethodGet, "/go/"+shareLink.ShortCode, nil)
	req.Header.Set("Accept-Language", "es")
	body := serveGoPage(svc, req).Body.String()

	wantAll(t, body, "y 2 más van")
	wantNone(t, body, "others going", "other going")
}

// TestSSRInviteLanding_LocalizedSpanish verifies the community landing
// (valid) and the shared error surface (invalid) render their static labels
// + title/OG copy in Spanish.
func TestSSRInviteLanding_LocalizedSpanish(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		svc, sqlStorage, _ := setupTestService(t)
		ctx := context.Background()
		inviter := createTestUser(t, ctx, sqlStorage, "Alice")
		community := createTestCommunity(t, ctx, sqlStorage, inviter.Id, nil)
		createTestMembership(t, ctx, sqlStorage, community.Id, inviter.Id)
		invite := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id, false)

		req := httptest.NewRequest(http.MethodGet, "/go/"+invite.ShortCode, nil)
		req.Header.Set("Accept-Language", "es")
		body := serveGoPage(svc, req).Body.String()

		wantAll(t, body,
			"Invitación",               // hero kicker
			"te invitó",                // inviter role line
			"Unirme al grupo",          // CTA label
			"1 miembro",                // member count
			"Abrir en la app de Ripls", // open-in-app hint
			"en Ripls",                 // title/OG "… en Ripls"
			`lang="es"`,
		)
		wantNone(t, body, "invited you", "Join the group", "Open in the Ripls app")
	})

	t.Run("invalid", func(t *testing.T) {
		svc, _, _ := setupTestService(t)
		req := httptest.NewRequest(http.MethodGet, "/go/NONEXIST", nil)
		req.Header.Set("Accept-Language", "es")
		body := serveGoPage(svc, req).Body.String()

		wantAll(t, body,
			"Invitación no válida",                    // heading
			"Pídele a quien compartió",                // error-help paragraph
			"Este enlace de invitación no es válido.", // error message
		)
		wantNone(t, body, "Invalid Invitation", "Ask the friend who shared")
	})
}

// TestSSREventLanding_StaticLabelsSpanish verifies the event landing's static
// chrome (host role, RSVP CTAs, open-in-app hint, meta aria) renders in
// Spanish.
func TestSSREventLanding_StaticLabelsSpanish(t *testing.T) {
	svc, sqlStorage, mockBucket := setupTestService(t)
	ctx := context.Background()
	mockBucket.SignedURL = "https://storage.example.com/signed.jpg"
	host := createTestUser(t, ctx, sqlStorage, "Bob")
	community := createTestCommunity(t, ctx, sqlStorage, host.Id, nil)
	createTestMembership(t, ctx, sqlStorage, community.Id, host.Id)
	expTime := &models.ExperienceTime{
		TimeType: &models.ExperienceTime_Specific{
			Specific: &models.SpecificTime{
				UnixTimestampSec: time.Date(2027, 6, 5, 18, 0, 0, 0, time.UTC).Unix(),
				Timezone:         "UTC",
			},
		},
	}
	experience := createTestExperience(t, ctx, sqlStorage, host.Id, "Fiesta",
		models.ExperienceState_EXPERIENCE_STATE_ACTIVE, expTime)
	shareLink := createTestExperienceShareLink(t, ctx, sqlStorage, community.Id, host.Id, experience.Id)

	req := httptest.NewRequest(http.MethodGet, "/go/"+shareLink.ShortCode, nil)
	req.Header.Set("Accept-Language", "es")
	body := serveGoPage(svc, req).Body.String()

	wantAll(t, body,
		"es anfitrión",             // host role
		"Voy",                      // RSVP-yes CTA
		"Quizás",                   // maybe CTA
		"No voy",                   // decline CTA
		"Abrir en la app de Ripls", // open-in-app hint
		"Detalles del evento",      // meta-pill aria
	)
	wantNone(t, body, "is hosting", "Open in the Ripls app", "I&#39;m in")
}

// TestSSRRenders_VaryAcceptLanguageHeader verifies every SSR render path sets
// Vary: Accept-Language so a shared cache keys the localized HTML on the
// visitor's locale rather than serving one language to everyone.
func TestSSRRenders_VaryAcceptLanguageHeader(t *testing.T) {
	svc, sqlStorage, _ := setupTestService(t)
	ctx := context.Background()

	inviter := createTestUser(t, ctx, sqlStorage, "Alice")
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id, nil)
	createTestMembership(t, ctx, sqlStorage, community.Id, inviter.Id)
	validInvite := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id, false)

	cases := []struct {
		name string
		path string
	}{
		{"valid invite render", "/go/" + validInvite.ShortCode},
		{"error render", "/go/NONEXIST"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := serveGoPage(svc, req)
			if got := w.Result().Header.Get("Vary"); got != "Accept-Language" {
				t.Errorf("Vary header = %q, want %q", got, "Accept-Language")
			}
		})
	}
}
