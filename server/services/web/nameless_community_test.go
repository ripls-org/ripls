package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/storage"
)

// wantAdHocLabel is the English (default-locale) rendering of the
// common.adhoc_community_name catalog key. The test requests carry no
// Accept-Language, so the SSR falls back to this.
const wantAdHocLabel = "a Ripls group"

func TestCommunityDisplayName(t *testing.T) {
	ctx := context.Background()
	loc, err := l10n.NewLocalizerForContext(ctx)
	if err != nil {
		t.Fatalf("build localizer: %v", err)
	}
	if got := communityDisplayName(ctx, loc, &models.Community{Name: "Two Oaks"}); got != "Two Oaks" {
		t.Errorf("named community = %q, want %q", got, "Two Oaks")
	}
	if got := communityDisplayName(ctx, loc, &models.Community{Name: ""}); got != wantAdHocLabel {
		t.Errorf("nameless community = %q, want fallback %q", got, wantAdHocLabel)
	}
	// With an es Accept-Language the label is localized — proving the SSR taps
	// the server l10n catalog, not a hardcoded English string.
	esCtx := l10n.WithAcceptLanguage(ctx, "es")
	esLoc, err := l10n.NewLocalizerForContext(esCtx)
	if err != nil {
		t.Fatalf("build es localizer: %v", err)
	}
	if got := communityDisplayName(esCtx, esLoc, &models.Community{Name: ""}); got != "un grupo de Ripls" {
		t.Errorf("nameless community (es) = %q, want %q", got, "un grupo de Ripls")
	}
}

// insertNamelessCommunity inserts an ad-hoc (nameless) community and returns it.
func insertNamelessCommunity(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, creatorID string) *models.Community {
	t.Helper()
	now := time.Now().Unix()
	c := &models.Community{
		Id:               uuid.New().String(),
		Name:             "", // ad-hoc: nameless
		CreatorId:        creatorID,
		OwnerUserId:      creatorID,
		CreatedAtUnixSec: now,
		UpdatedAtUnixSec: now,
	}
	if _, err := sqlStorage.Insert(ctx, c); err != nil {
		t.Fatalf("insert nameless community: %v", err)
	}
	return c
}

// TestHandleInvitePage_NamelessCommunity verifies the invite landing renders a
// graceful fallback label instead of an empty community name (no broken OG card
// like "...invited you to join  on Ripls").
func TestHandleInvitePage_NamelessCommunity(t *testing.T) {
	svc, sqlStorage, _ := setupTestService(t)
	ctx := context.Background()

	inviter := createTestUser(t, ctx, sqlStorage, "Alice")
	community := insertNamelessCommunity(t, ctx, sqlStorage, inviter.Id)
	createTestMembership(t, ctx, sqlStorage, community.Id, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id, false)

	req := httptest.NewRequest(http.MethodGet, "/go/"+invitation.ShortCode, nil)
	w := httptest.NewRecorder()
	svc.HandleInvitePage(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Result().StatusCode)
	}
	body := w.Body.String()

	if !strings.Contains(body, wantAdHocLabel) {
		t.Errorf("expected fallback label %q in nameless-community landing", wantAdHocLabel)
	}
	// Regression guard: the empty-name artifacts must not appear.
	if strings.Contains(body, "join  on Ripls") || strings.Contains(body, "Join  on Ripls") {
		t.Error("nameless community leaked an empty name into the landing copy")
	}
}

// TestHandleEventLanding_NamelessCommunity verifies the event SSR landing
// omits the community phrase entirely for a nameless ad-hoc community — the
// title/OG copy reads "Join Bob for Block Party", not "... in a Ripls group".
func TestHandleEventLanding_NamelessCommunity(t *testing.T) {
	svc, sqlStorage, mockBucket := setupTestService(t)
	ctx := context.Background()
	mockBucket.SignedURL = "https://storage.example.com/signed.jpg"

	host := createTestUser(t, ctx, sqlStorage, "Bob")
	community := insertNamelessCommunity(t, ctx, sqlStorage, host.Id)
	createTestMembership(t, ctx, sqlStorage, community.Id, host.Id)
	expTime := &models.ExperienceTime{
		TimeType: &models.ExperienceTime_Specific{
			Specific: &models.SpecificTime{UnixTimestampSec: time.Date(2027, 6, 5, 18, 0, 0, 0, time.UTC).Unix(), Timezone: "UTC"},
		},
	}
	experience := createTestExperience(t, ctx, sqlStorage, host.Id, "Block Party", models.ExperienceState_EXPERIENCE_STATE_ACTIVE, expTime)
	shareLink := createTestExperienceShareLink(t, ctx, sqlStorage, community.Id, host.Id, experience.Id)

	req := httptest.NewRequest(http.MethodGet, "/go/"+shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	svc.HandleInvitePage(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Result().StatusCode)
	}
	body := w.Body.String()
	assertContains(t, body, "Block Party")
	// The nameless community contributes no copy at all: no "in a Ripls
	// group" suffix on the title/OG tags and no community segment in the
	// kicker.
	assertNotContains(t, body, wantAdHocLabel)
	assertContains(t, body, `content="Join Bob for Block Party"`)
}

// TestHandleGearLanding_NamelessCommunity verifies the gear SSR landing omits
// the community phrase for a nameless ad-hoc community — the OG description
// reads "Dana is lending this.", not "... in a Ripls group.".
func TestHandleGearLanding_NamelessCommunity(t *testing.T) {
	svc, sqlStorage, mockBucket := setupTestService(t)
	ctx := context.Background()
	mockBucket.SignedURL = "https://storage.example.com/signed.jpg"

	owner := createTestUser(t, ctx, sqlStorage, "Dana")
	community := insertNamelessCommunity(t, ctx, sqlStorage, owner.Id)
	createTestMembership(t, ctx, sqlStorage, community.Id, owner.Id)
	gear := createTestGear(t, ctx, sqlStorage, owner.Id, "Cordless Drill", nil, models.GearState_GEAR_STATE_AVAILABLE)
	createTestCommunityGear(t, ctx, sqlStorage, community.Id, gear.Id, models.Availability_AVAILABILITY_FOR_LOAN)
	shareLink := createTestGearShareLink(t, ctx, sqlStorage, community.Id, owner.Id, gear.Id)

	req := httptest.NewRequest(http.MethodGet, "/go/"+shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	svc.HandleInvitePage(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Result().StatusCode)
	}
	body := w.Body.String()
	assertContains(t, body, "Cordless Drill")
	assertNotContains(t, body, wantAdHocLabel)
	assertContains(t, body, `content="Dana is lending this."`)
}
