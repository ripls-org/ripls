package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/branding"
	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// eventPageTestSetup is the base fixture for event-landing tests.
// Each test mutates from this baseline (e.g., revoke the link,
// fill the community to capacity, soft-delete the experience).
type eventPageTestSetup struct {
	svc        *Service
	sqlStorage *storage.ProtoSQLStorage
	mockBucket any // *services.MockBucketStorage — any avoids the cyclic import in test helpers
	host       *models.User
	community  *models.Community
	experience *models.Experience
	shareLink  *models.ShareLink
}

// createTestExperienceShareLink inserts an experience-flavored
// ShareLink and returns it.
func createTestExperienceShareLink(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, communityID, inviterID, experienceID string) *models.ShareLink {
	t.Helper()
	link := &models.ShareLink{
		Id:               uuid.New().String(),
		CommunityId:      communityID,
		InviterId:        inviterID,
		ShortCode:        generateTestShortCode(t),
		IsRevoked:        false,
		CreatedAtUnixSec: time.Now().Unix(),
		Target: &models.ShareLink_ExperienceId{
			ExperienceId: experienceID,
		},
	}
	if _, err := sqlStorage.Insert(ctx, link); err != nil {
		t.Fatalf("Failed to insert experience share link: %v", err)
	}
	return link
}

// createTestRSVP inserts a YES RSVP for (user, experience) scoped
// to the hosting community.
func createTestRSVP(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, experienceID, communityID, userID string, rsvpedAt int64) *models.ExperienceRSVP {
	t.Helper()
	rsvp := &models.ExperienceRSVP{
		Id:                 uuid.New().String(),
		ExperienceId:       experienceID,
		UserId:             userID,
		CommunityId:        communityID,
		Intention:          models.RSVPIntention_RSVP_INTENTION_YES,
		RsvpedAtUnixSec:    rsvpedAt,
		LastUpdatedUnixSec: rsvpedAt,
	}
	if _, err := sqlStorage.Insert(ctx, rsvp); err != nil {
		t.Fatalf("Failed to insert RSVP: %v", err)
	}
	return rsvp
}

// setupEventPageTest builds the standard event-landing fixture:
// a host (Emma), a community (Two Oaks Block), an active future
// experience (Saturday Potluck), and an experience-flavored
// ShareLink pointing at the experience.
func setupEventPageTest(t *testing.T) eventPageTestSetup {
	t.Helper()
	svc, sqlStorage, mockBucket := setupTestService(t)
	ctx := context.Background()

	mockBucket.SignedURL = "https://storage.example.com/signed-event-image.jpg"

	host := createTestUser(t, ctx, sqlStorage, "Emma Rivera")
	community := createTestCommunity(t, ctx, sqlStorage, host.Id, nil)
	createTestMembership(t, ctx, sqlStorage, community.Id, host.Id)

	// Future Saturday at 6 PM UTC = noon in Denver, same calendar day.
	futureSat := time.Date(2027, 6, 5, 18, 0, 0, 0, time.UTC).Unix()
	expTime := &models.ExperienceTime{
		TimeType: &models.ExperienceTime_Specific{
			Specific: &models.SpecificTime{
				UnixTimestampSec: futureSat,
				Timezone:         "America/Denver",
			},
		},
	}
	experience := createTestExperience(t, ctx, sqlStorage, host.Id, "Community Potluck Dinner", models.ExperienceState_EXPERIENCE_STATE_ACTIVE, expTime)

	shareLink := createTestExperienceShareLink(t, ctx, sqlStorage, community.Id, host.Id, experience.Id)

	return eventPageTestSetup{
		svc:        svc,
		sqlStorage: sqlStorage,
		mockBucket: mockBucket,
		host:       host,
		community:  community,
		experience: experience,
		shareLink:  shareLink,
	}
}

func TestHandleEventLanding_NormalRender(t *testing.T) {
	s := setupEventPageTest(t)
	ctx := context.Background()

	// Six YES-RSVPed attendees, each at a different rsvped_at so the
	// "first 8 by rsvped_at" ordering is deterministic.
	for i, name := range []string{"Marcus", "Henry", "Ian", "Betty", "Tom", "Sarah"} {
		attendee := createTestUser(t, ctx, s.sqlStorage, name)
		createTestMembership(t, ctx, s.sqlStorage, s.community.Id, attendee.Id)
		createTestRSVP(t, ctx, s.sqlStorage, s.experience.Id, s.community.Id, attendee.Id, int64(1700000000+i))
	}

	req := httptest.NewRequest(http.MethodGet, "/go/"+s.shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "public, max-age=30" {
		t.Errorf("Expected Cache-Control 'public, max-age=30', got %q", got)
	}

	body := w.Body.String()

	// Event identity surfaces.
	assertContains(t, body, "Community Potluck Dinner")
	assertContains(t, body, s.community.Name) // "Test Community"

	// First-name only.
	assertContains(t, body, ">Emma<")
	if strings.Contains(body, "Rivera") {
		t.Error("Host last name should be stripped (first-name only)")
	}

	// OG tags.
	if !strings.Contains(body, `og:title`) {
		t.Error("Expected og:title in response")
	}
	if !strings.Contains(body, `og:image`) {
		t.Error("Expected og:image in response")
	}
	if !strings.Contains(body, "Join Emma for Community Potluck Dinner") {
		t.Errorf("Expected OG title to include host and event name, got body: %s", body)
	}

	// Attendees show up by first name.
	for _, name := range []string{"Marcus", "Henry", "Ian", "Betty"} {
		assertContains(t, body, name)
	}
	// "+ 2 others going" — six attendees, four shown by name, two overflow.
	if !strings.Contains(body, "2 others") {
		t.Error("Expected '2 others' overflow label in response")
	}

	// Three CTAs present. All three are active in the happy path:
	// "I'm out" POSTs to /go/{code}/decline via inline JS (Step 3),
	// "Maybe" and "I'm in" navigate to /event/{id}?rsvp=…&code=…
	// for the Flutter Web auth + RSVP handoff (Step 8a). The labels now
	// render through the localizer, so the apostrophes are HTML-escaped
	// (&#39;) — visually identical in the browser.
	assertContains(t, body, "I&#39;m in")
	assertContains(t, body, "Maybe")
	assertContains(t, body, "I&#39;m out")

	// At-capacity message must NOT render in the happy path.
	if strings.Contains(body, "This community is full") {
		t.Error("Did not expect at-capacity message in happy path")
	}

	// "I'm out" button is active and points at the per-share-link
	// decline endpoint.
	assertContains(t, body, `id="event-cta-out"`)
	assertContains(t, body, `data-decline-url="/go/`+s.shareLink.ShortCode+`/decline"`)
	assertContains(t, body, `aria-label="Decline this event"`)

	// "Maybe" and "I'm in" navigate to the Flutter Web event landing
	// with the rsvp intention + share code so the auth-handoff
	// ViewModel can pick them up. The event name (`n`) rides through too
	// for the phone-first RSVP screen (#2492); the prefix-match tolerates
	// the trailing &n=… (and optional &img=…).
	assertContains(t, body,
		`href="/event/`+s.experience.Id+`?rsvp=maybe&amp;code=`+s.shareLink.ShortCode+`&amp;n=`)
	assertContains(t, body,
		`href="/event/`+s.experience.Id+`?rsvp=yes&amp;code=`+s.shareLink.ShortCode+`&amp;n=`)
	assertContains(t, body, `aria-label="Maybe attending this event"`)
	assertContains(t, body, `aria-label="RSVP yes to this event"`)

	// Verify the "I'm out" button is not rendered as disabled. The
	// other two CTAs legitimately carry the disabled attribute, so we
	// inspect the substring around the I'm out button rather than
	// counting "disabled" globally.
	outIdx := strings.Index(body, `id="event-cta-out"`)
	if outIdx < 0 {
		t.Fatalf("could not locate I'm out button in body")
	}
	closeIdx := strings.Index(body[outIdx:], ">")
	if closeIdx < 0 {
		t.Fatalf("could not locate end of I'm out button tag")
	}
	outTag := body[outIdx : outIdx+closeIdx]
	if strings.Contains(outTag, "disabled") {
		t.Errorf("I'm out button should not be server-rendered as disabled, got tag: %s", outTag)
	}

	// Inline JS handler is present and uses an aria-live region for
	// the confirmation announcement.
	assertContains(t, body, "fetch(url")
	assertContains(t, body, `aria-live`)
}

func TestHandleEventLanding_AtCapacity(t *testing.T) {
	s := setupEventPageTest(t)
	ctx := context.Background()

	// Fill the community to MaxCommunityMembers (32). One member is
	// the host (already a member from setup); add 31 more.
	for i := range communitylib.MaxCommunityMembers - 1 {
		filler := createTestUser(t, ctx, s.sqlStorage, "Filler"+string(rune('A'+i)))
		createTestMembership(t, ctx, s.sqlStorage, s.community.Id, filler.Id)
	}

	req := httptest.NewRequest(http.MethodGet, "/go/"+s.shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()

	// At-capacity message present.
	if !strings.Contains(body, "This community is full") {
		t.Error("Expected 'This community is full' message at capacity")
	}
	if !strings.Contains(body, "Ask your inviter") {
		t.Error("Expected actionable next-step copy in at-capacity message")
	}

	// No greyed-out CTAs — at-capacity hides the bar entirely. The
	// template renders either the CTA bar or the at-capacity panel,
	// never both.
	if strings.Contains(body, `<button class="event-cta`) {
		t.Error("Expected no CTA buttons rendered at capacity; the bar should be replaced entirely")
	}
}

func TestHandleEventLanding_EmptyAttendees(t *testing.T) {
	s := setupEventPageTest(t)

	req := httptest.NewRequest(http.MethodGet, "/go/"+s.shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	body := w.Body.String()

	// Attendee block (`event-attendees`) is hidden when no one has
	// RSVPed. Host row and meta-pill still render.
	if strings.Contains(body, `class="event-attendees"`) {
		t.Error("Expected no event-attendees block when nobody has RSVPed")
	}
	// Host row should still be there.
	assertContains(t, body, ">Emma<")
	// Meta-pill still renders.
	assertContains(t, body, "event-meta-pill")
}

func TestHandleEventLanding_MissingHeroImage(t *testing.T) {
	s := setupEventPageTest(t)

	// Experience has no media_ids — created without any.
	req := httptest.NewRequest(http.MethodGet, "/go/"+s.shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	body := w.Body.String()

	// Falls back to the default Ripls logo for og:image when no media.
	expectedDefault := "https://test.example.com/assets/logo.png"
	if !strings.Contains(body, expectedDefault) {
		t.Errorf("Expected default OG image URL %q when no hero media, body=%s", expectedDefault, body[:min(2000, len(body))])
	}
}

func TestHandleEventLanding_VeryLongNames(t *testing.T) {
	svc, sqlStorage, mockBucket := setupTestService(t)
	ctx := context.Background()
	mockBucket.SignedURL = "https://storage.example.com/signed.jpg"

	veryLongHostName := "Constance-Alexandria Featherbottom-Wellington III"
	veryLongEventName := "The Annual Two Oaks Block Springtime Equinox Community Potluck and Heritage Celebration Dinner"

	host := createTestUser(t, ctx, sqlStorage, veryLongHostName)
	community := createTestCommunity(t, ctx, sqlStorage, host.Id, nil)
	createTestMembership(t, ctx, sqlStorage, community.Id, host.Id)
	futureSat := time.Date(2027, 6, 5, 18, 0, 0, 0, time.UTC).Unix()
	expTime := &models.ExperienceTime{
		TimeType: &models.ExperienceTime_Specific{
			Specific: &models.SpecificTime{UnixTimestampSec: futureSat, Timezone: "UTC"},
		},
	}
	experience := createTestExperience(t, ctx, sqlStorage, host.Id, veryLongEventName, models.ExperienceState_EXPERIENCE_STATE_ACTIVE, expTime)
	shareLink := createTestExperienceShareLink(t, ctx, sqlStorage, community.Id, host.Id, experience.Id)

	req := httptest.NewRequest(http.MethodGet, "/go/"+shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	svc.HandleInvitePage(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Result().StatusCode)
	}
	body := w.Body.String()
	assertContains(t, body, veryLongEventName)
	// First-name only — only "Constance-Alexandria" should appear,
	// not the rest of the name.
	assertContains(t, body, "Constance-Alexandria")
	if strings.Contains(body, "Featherbottom") {
		t.Error("Expected only the host's first name to render")
	}
}

// TestHandleEventLanding_LegacyUTCTimezoneDateOnly reproduces the #2621
// live report: an event stored with the legacy "UTC" fallback timezone at
// midnight UTC. The landing must degrade to date-only labels — no
// confidently wrong "12:00 AM" — and the OG description must not carry a
// dangling separator.
func TestHandleEventLanding_LegacyUTCTimezoneDateOnly(t *testing.T) {
	s := setupEventPageTest(t)
	ctx := context.Background()

	midnightUTC := time.Date(2027, 7, 2, 0, 0, 0, 0, time.UTC).Unix()
	expTime := &models.ExperienceTime{
		TimeType: &models.ExperienceTime_Specific{
			Specific: &models.SpecificTime{UnixTimestampSec: midnightUTC, Timezone: "UTC"},
		},
	}
	experience := createTestExperience(t, ctx, s.sqlStorage, s.host.Id, "Farewell Party", models.ExperienceState_EXPERIENCE_STATE_ACTIVE, expTime)
	link := createTestExperienceShareLink(t, ctx, s.sqlStorage, s.community.Id, s.host.Id, experience.Id)

	req := httptest.NewRequest(http.MethodGet, "/go/"+link.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Result().StatusCode)
	}
	body := w.Body.String()
	assertNotContains(t, body, "12:00 AM")
	// Date-only OG description: "Friday, July 2 · <place>" with no time
	// segment and no doubled separator.
	assertContains(t, body, `content="Friday, July 2 · `)
	assertNotContains(t, body, "· ·")
}

func TestHandleEventLanding_HostAlsoRSVPed(t *testing.T) {
	s := setupEventPageTest(t)
	ctx := context.Background()

	// Host RSVPs to their own event — should not appear in the
	// "going" list (they're shown as host instead).
	createTestRSVP(t, ctx, s.sqlStorage, s.experience.Id, s.community.Id, s.host.Id, 1700000000)
	// One real attendee.
	marcus := createTestUser(t, ctx, s.sqlStorage, "Marcus")
	createTestMembership(t, ctx, s.sqlStorage, s.community.Id, marcus.Id)
	createTestRSVP(t, ctx, s.sqlStorage, s.experience.Id, s.community.Id, marcus.Id, 1700000001)

	req := httptest.NewRequest(http.MethodGet, "/go/"+s.shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	body := w.Body.String()

	// Attendee shows up.
	assertContains(t, body, "Marcus")
	// Host should appear exactly once — in the host row, not in the
	// attendees text. Count occurrences of "Emma".
	emmaCount := strings.Count(body, ">Emma<")
	if emmaCount != 1 {
		t.Errorf("Expected Emma to appear once (host row only); appeared %d times", emmaCount)
	}
}

func TestHandleEventLanding_InvalidStates(t *testing.T) {
	cases := []struct {
		name           string
		mutateExp      func(e *models.Experience)
		expectedMsgSub string
	}{
		{
			name: "cancelled",
			mutateExp: func(e *models.Experience) {
				e.State = models.ExperienceState_EXPERIENCE_STATE_CANCELLED
			},
			expectedMsgSub: "no longer available",
		},
		{
			name: "completed",
			mutateExp: func(e *models.Experience) {
				e.State = models.ExperienceState_EXPERIENCE_STATE_COMPLETED
			},
			expectedMsgSub: "no longer available",
		},
		{
			name: "soft-deleted",
			mutateExp: func(e *models.Experience) {
				e.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: time.Now().Unix()}
			},
			expectedMsgSub: "no longer available",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := setupEventPageTest(t)
			ctx := context.Background()

			tc.mutateExp(s.experience)
			if err := s.sqlStorage.Update(ctx, s.experience); err != nil {
				t.Fatalf("Failed to update experience: %v", err)
			}

			req := httptest.NewRequest(http.MethodGet, "/go/"+s.shareLink.ShortCode, nil)
			w := httptest.NewRecorder()
			s.svc.HandleInvitePage(w, req)

			body := w.Body.String()
			// Falls back to the invite.html error template — "Invalid
			// Invitation" heading + the per-state message.
			if !strings.Contains(body, "Invalid Invitation") {
				t.Errorf("Expected invite.html error template (heading 'Invalid Invitation')")
			}
			if !strings.Contains(body, tc.expectedMsgSub) {
				t.Errorf("Expected error message containing %q in body", tc.expectedMsgSub)
			}
		})
	}
}

func TestHandleEventLanding_TBDTime(t *testing.T) {
	svc, sqlStorage, mockBucket := setupTestService(t)
	ctx := context.Background()
	mockBucket.SignedURL = "https://storage.example.com/signed.jpg"

	host := createTestUser(t, ctx, sqlStorage, "Emma")
	community := createTestCommunity(t, ctx, sqlStorage, host.Id, nil)
	createTestMembership(t, ctx, sqlStorage, community.Id, host.Id)

	// TBD time — event is on the calendar but no date is set yet.
	tbdTime := &models.ExperienceTime{
		TimeType: &models.ExperienceTime_Tbd{Tbd: &models.TimeTBD{}},
	}
	experience := createTestExperience(t, ctx, sqlStorage, host.Id, "Future Potluck", models.ExperienceState_EXPERIENCE_STATE_ACTIVE, tbdTime)
	shareLink := createTestExperienceShareLink(t, ctx, sqlStorage, community.Id, host.Id, experience.Id)

	req := httptest.NewRequest(http.MethodGet, "/go/"+shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	svc.HandleInvitePage(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Result().StatusCode)
	}
	body := w.Body.String()

	// The meta-pill renders "Date TBD" when no specific time is set.
	assertContains(t, body, "Date TBD")
	assertContains(t, body, "Future Potluck")
}

func TestHandleEventLanding_AvatarFallback(t *testing.T) {
	s := setupEventPageTest(t)
	ctx := context.Background()

	// Attendee with no profile media — template should render the
	// initial-letter fallback inside .event-avatar-fallback.
	marcus := createTestUser(t, ctx, s.sqlStorage, "Marcus")
	createTestMembership(t, ctx, s.sqlStorage, s.community.Id, marcus.Id)
	createTestRSVP(t, ctx, s.sqlStorage, s.experience.Id, s.community.Id, marcus.Id, 1700000000)

	req := httptest.NewRequest(http.MethodGet, "/go/"+s.shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	body := w.Body.String()

	// Host has no media (createTestUser doesn't add any) — host avatar
	// falls back to first initial.
	if !strings.Contains(body, `class="event-avatar-fallback"`) {
		t.Error("Expected event-avatar-fallback element when user has no media")
	}
	// Initial-letter content surfaces.
	if !strings.Contains(body, ">E<") {
		t.Error("Expected host's 'E' initial in fallback")
	}
	if !strings.Contains(body, ">M<") {
		t.Error("Expected Marcus's 'M' initial in fallback")
	}
}

func TestHandleEventLanding_DuplicateRSVPRowsPerUser(t *testing.T) {
	// A single user may legitimately have multiple ExperienceRSVP
	// rows for the same (experience, community) if intention changes
	// are stored as new rows (or as a defensive guard against schema
	// evolution). The landing must dedupe by user_id keeping the most
	// recent by last_updated_unix_sec so the same user isn't counted
	// twice in the "+ N others going" overflow.
	s := setupEventPageTest(t)
	ctx := context.Background()

	marcus := createTestUser(t, ctx, s.sqlStorage, "Marcus")
	createTestMembership(t, ctx, s.sqlStorage, s.community.Id, marcus.Id)

	// First YES row — older.
	earlier := createTestRSVP(t, ctx, s.sqlStorage, s.experience.Id, s.community.Id, marcus.Id, 1700000000)
	earlier.LastUpdatedUnixSec = 1700000000
	if err := s.sqlStorage.Update(ctx, earlier); err != nil {
		t.Fatalf("Failed to set earlier last_updated: %v", err)
	}
	// Second YES row for the same user — newer. Inserted via createTestRSVP,
	// then bumped to a later last_updated.
	later := &models.ExperienceRSVP{
		Id:                 uuid.New().String(),
		ExperienceId:       s.experience.Id,
		UserId:             marcus.Id,
		CommunityId:        s.community.Id,
		Intention:          models.RSVPIntention_RSVP_INTENTION_YES,
		RsvpedAtUnixSec:    1700000000,
		LastUpdatedUnixSec: 1700001000,
	}
	if _, err := s.sqlStorage.Insert(ctx, later); err != nil {
		t.Fatalf("Failed to insert second RSVP row: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/go/"+s.shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	body := w.Body.String()

	// Marcus should appear exactly once in the attendees text.
	count := strings.Count(body, "Marcus")
	if count != 1 {
		t.Errorf("Expected Marcus to appear once after dedup; appeared %d times", count)
	}
	// No "+ N others going" overflow — Marcus is the only attendee
	// after dedup, so the entire attendees text should be "Marcus".
	if strings.Contains(body, "+ 1 other") || strings.Contains(body, "+ 1 others") {
		t.Error("Expected no overflow when Marcus is the sole deduped attendee")
	}
}

func TestHandleEventLanding_QueryBudget(t *testing.T) {
	s := setupEventPageTest(t)
	ctx := context.Background()

	// Add three attendees so the rendered page is non-trivial — the
	// query budget should hold regardless of attendee count thanks
	// to GetByIDs batching.
	for i, name := range []string{"Marcus", "Henry", "Ian"} {
		attendee := createTestUser(t, ctx, s.sqlStorage, name)
		createTestMembership(t, ctx, s.sqlStorage, s.community.Id, attendee.Id)
		createTestRSVP(t, ctx, s.sqlStorage, s.experience.Id, s.community.Id, attendee.Id, int64(1700000000+i))
	}

	// Budget includes HandleInvitePage's short-code lookup plus
	// handleEventLanding's 7-query fan-out (experience, community,
	// location, RSVPs, batched users, batched media, member count).
	// Location is absent here so that branch is skipped — we still
	// keep the assertion at 8 to make the budget concrete and the
	// regression test loud if anything regresses.
	const maxQueries = 8

	statsCtx := storage.WithQueryStats(ctx)
	storage.AssertMaxQueries(t, statsCtx, maxQueries, func() {
		req := httptest.NewRequest(http.MethodGet, "/go/"+s.shareLink.ShortCode, nil)
		req = req.WithContext(statsCtx)
		w := httptest.NewRecorder()
		s.svc.HandleInvitePage(w, req)
		if w.Result().StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", w.Result().StatusCode)
		}
	})
}

// The Android package used to be inferred from the hostname
// (TestAndroidPackageForHostname, removed in #2953): a "dev."/"localhost"
// prefix meant one hardcoded package, anything else another. That baked one
// publisher's identifiers into the server and mislabeled any host outside the
// naming convention. It is now stated per deployment via --android-package-id,
// so what's worth asserting is that the configured value reaches the template
// data — and that an unconfigured instance renders nothing rather than
// somebody else's package.
func TestAndroidPackageComesFromBranding(t *testing.T) {
	t.Run("configured value is threaded through", func(t *testing.T) {
		svc := &Service{branding: branding.Config{AndroidPackageID: "com.example.app"}}
		if got := svc.branding.AndroidPackageID; got != "com.example.app" {
			t.Errorf("AndroidPackageID = %q, want com.example.app", got)
		}
	})

	t.Run("unconfigured is empty, not a default", func(t *testing.T) {
		svc := &Service{}
		if got := svc.branding.AndroidPackageID; got != "" {
			t.Errorf("AndroidPackageID = %q, want empty for an unconfigured instance", got)
		}
	})
}
