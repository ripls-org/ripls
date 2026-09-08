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
	"go.ripls.org/ripls/server/storage"
)

// createTestGear inserts a test gear item into the database.
func createTestGear(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, ownerID, name string, mediaIDs []string, state models.GearState) *models.Gear {
	t.Helper()
	gear := &models.Gear{
		Id:               uuid.New().String(),
		Name:             name,
		Description:      "Test gear description",
		OwnerId:          ownerID,
		MediaIds:         mediaIDs,
		State:            state,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	_, err := sqlStorage.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("Failed to create test gear: %v", err)
	}
	return gear
}

// createTestCommunityGear inserts a community-gear link with availability.
func createTestCommunityGear(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, communityID, gearID string, availability models.Availability) {
	t.Helper()
	cg := &models.CommunityGear{
		Id:               uuid.New().String(),
		CommunityId:      communityID,
		GearId:           gearID,
		CreatedAtUnixSec: time.Now().Unix(),
		Availability:     availability,
	}
	_, err := sqlStorage.Insert(ctx, cg)
	if err != nil {
		t.Fatalf("Failed to create test community gear: %v", err)
	}
}

// createTestRequest inserts a test request into the database.
func createTestRequest(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, requesterID, title string, state models.RequestState) *models.Request {
	t.Helper()
	req := &models.Request{
		Id:          uuid.New().String(),
		RequesterId: requesterID,
		Title:       title,
		Description: "Test request description",
		State:       state,
	}
	_, err := sqlStorage.Insert(ctx, req)
	if err != nil {
		t.Fatalf("Failed to create test request: %v", err)
	}
	return req
}

// createTestExperience inserts a test experience into the database.
func createTestExperience(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, ownerID, name string, state models.ExperienceState, experienceTime *models.ExperienceTime) *models.Experience {
	t.Helper()
	exp := &models.Experience{
		Id:      uuid.New().String(),
		OwnerId: ownerID,
		Name:    name,
		State:   state,
		Time:    experienceTime,
	}
	_, err := sqlStorage.Insert(ctx, exp)
	if err != nil {
		t.Fatalf("Failed to create test experience: %v", err)
	}
	return exp
}

// invitePageTestSetup creates the base entities for an invite page test:
// inviter, community, membership, and a non-revoked invitation link.
type invitePageTestSetup struct {
	svc        *Service
	sqlStorage *storage.ProtoSQLStorage
	inviter    *models.User
	community  *models.Community
	invitation *models.ShareLink
}

func setupInvitePageTest(t *testing.T) invitePageTestSetup {
	t.Helper()
	svc, sqlStorage, mockBucket := setupTestService(t)
	ctx := context.Background()

	mockBucket.SignedURL = "https://storage.example.com/signed-image.jpg"

	inviter := createTestUser(t, ctx, sqlStorage, "TestInviter")
	community := createTestCommunity(t, ctx, sqlStorage, inviter.Id, nil)
	createTestMembership(t, ctx, sqlStorage, community.Id, inviter.Id)
	invitation := createTestInvitation(t, ctx, sqlStorage, community.Id, inviter.Id, false)

	return invitePageTestSetup{
		svc:        svc,
		sqlStorage: sqlStorage,
		inviter:    inviter,
		community:  community,
		invitation: invitation,
	}
}

// TestHandleInvitePage_TypedGearShareLink covers the post-#2048 path
// where the share_link row's oneof target carries the gear_id directly
// — the recipient's URL has no query params. A typed gear target now
// routes to the phone-first gear landing (gear.html, #2492 WEB-4)
// rather than the generic invite.html preview; the data still comes
// from the row. Comprehensive gear-landing coverage is in
// gear_page_test.go.
func TestHandleInvitePage_TypedGearShareLink(t *testing.T) {
	s := setupInvitePageTest(t)
	ctx := context.Background()

	owner := createTestUser(t, ctx, s.sqlStorage, "TypedGearOwner")
	media := createTestMedia(t, ctx, s.sqlStorage, owner.Id)
	gear := createTestGear(t, ctx, s.sqlStorage, owner.Id, "Typed Drill", []string{media.Id}, models.GearState_GEAR_STATE_AVAILABLE)
	createTestCommunityGear(t, ctx, s.sqlStorage, s.community.Id, gear.Id, models.Availability_AVAILABILITY_FOR_LOAN)

	// Insert a typed gear share_link.
	typedLink := &models.ShareLink{
		Id:               uuid.New().String(),
		CommunityId:      s.community.Id,
		InviterId:        s.inviter.Id,
		ShortCode:        generateTestShortCode(t),
		CreatedAtUnixSec: time.Now().Unix(),
		Target: &models.ShareLink_GearId{
			GearId: gear.Id,
		},
	}
	if _, err := s.sqlStorage.Insert(ctx, typedLink); err != nil {
		t.Fatalf("Failed to insert typed gear share link: %v", err)
	}

	// URL is bare — no query params. The page must come from the row.
	req := httptest.NewRequest(http.MethodGet, "/go/"+typedLink.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Result().StatusCode)
	}

	body := w.Body.String()
	assertContains(t, body, `TypedGearOwner is lending Typed Drill`)
	assertContains(t, body, `Ask to borrow`)
	assertContains(t, body, `/item/`+gear.Id+`?intent=interest`)
}

// TestHandleInvitePage_NoItemParams covers a pure community invite
// (community_invite_id only, no typed item target): the generic
// invite.html "You're Invited" page. Item-flavored links route to their
// own landing pages (see TestHandleInvitePage_TypedGearShareLink and
// gear_page_test.go / request_page_test.go / event_page_test.go).
func TestHandleInvitePage_NoItemParams(t *testing.T) {
	s := setupInvitePageTest(t)

	req := httptest.NewRequest(http.MethodGet, "/go/"+s.invitation.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	body := w.Body.String()

	// Standard community-only preview.
	assertContains(t, body, `Join `+s.community.Name+``)
	assertContains(t, body, s.inviter.Name+` invited you to join`)
}

func TestFirstName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Robert Johnson", "Robert"},
		{"Alice", "Alice"},
		{"Mary Jane Watson", "Mary"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := firstName(tt.input)
			if got != tt.want {
				t.Errorf("firstName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// assertContains fails the test if body does not contain substr.
func assertContains(t *testing.T, body, substr string) {
	t.Helper()
	if !strings.Contains(body, substr) {
		t.Errorf("Expected body to contain %q, but it did not.\nBody (first 500 chars): %s", substr, truncate(body, 500))
	}
}

// assertNotContains fails the test if body contains substr.
func assertNotContains(t *testing.T, body, substr string) {
	t.Helper()
	if strings.Contains(body, substr) {
		t.Errorf("Expected body NOT to contain %q, but it did", substr)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
