package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	communitylib "go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// createTestRequestShareLink inserts a request-flavored ShareLink (typed
// target = request_id) and returns it.
func createTestRequestShareLink(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, communityID, inviterID, requestID string) *models.ShareLink {
	t.Helper()
	link := &models.ShareLink{
		Id:               uuid.New().String(),
		CommunityId:      communityID,
		InviterId:        inviterID,
		ShortCode:        generateTestShortCode(t),
		CreatedAtUnixSec: time.Now().Unix(),
		Target:           &models.ShareLink_RequestId{RequestId: requestID},
	}
	if _, err := sqlStorage.Insert(ctx, link); err != nil {
		t.Fatalf("Failed to insert request share link: %v", err)
	}
	return link
}

// createTestRequestNeed inserts a request-scoped PlanningNeed with
// `slots` total and `slotsRemaining` unclaimed.
func createTestRequestNeed(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage,
	requestID, proposerID, name string, slots, slotsRemaining int32,
) *models.PlanningNeed {
	t.Helper()
	need := &models.PlanningNeed{
		Id:               uuid.New().String(),
		ProposerId:       proposerID,
		Name:             name,
		Slots:            slots,
		SlotsRemaining:   slotsRemaining,
		CreatedAtUnixSec: time.Now().Unix(),
		Scope:            &models.PlanningNeed_RequestId{RequestId: requestID},
	}
	if _, err := sqlStorage.Insert(ctx, need); err != nil {
		t.Fatalf("Failed to insert planning need: %v", err)
	}
	return need
}

type requestPageTestSetup struct {
	svc        *Service
	sqlStorage *storage.ProtoSQLStorage
	mockBucket *services.MockBucketStorage
	requester  *models.User
	community  *models.Community
	request    *models.Request
	shareLink  *models.ShareLink
}

// render serves the fixture's share link and returns the response body.
func (s requestPageTestSetup) render(t *testing.T) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/go/"+s.shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)
	return w.Body.String()
}

// setupRequestPageTest builds the request fixture: a requester (Priya),
// a community, an active request, and a request-flavored ShareLink.
func setupRequestPageTest(t *testing.T) requestPageTestSetup {
	t.Helper()
	svc, sqlStorage, mockBucket := setupTestService(t)
	ctx := context.Background()
	mockBucket.SignedURL = "https://storage.example.com/signed-request-image.jpg"

	requester := createTestUser(t, ctx, sqlStorage, "Priya Nair")
	community := createTestCommunity(t, ctx, sqlStorage, requester.Id, nil)
	createTestMembership(t, ctx, sqlStorage, community.Id, requester.Id)
	request := createTestRequest(t, ctx, sqlStorage, requester.Id, "An extension ladder", models.RequestState_REQUEST_STATE_ACTIVE)
	shareLink := createTestRequestShareLink(t, ctx, sqlStorage, community.Id, requester.Id, request.Id)

	return requestPageTestSetup{svc, sqlStorage, mockBucket, requester, community, request, shareLink}
}

func TestHandleRequestLanding_Active(t *testing.T) {
	s := setupRequestPageTest(t)

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
	assertContains(t, body, "An extension ladder")
	assertContains(t, body, "Priya is looking for") // first name only
	assertNotContains(t, body, "Nair")
	assertContains(t, body, "Offer to help")
	assertContains(t, body, "/need/"+s.request.Id+"?intent=offer")
	assertContains(t, body, "code="+s.shareLink.ShortCode)
}

// The pitch is the whole reason a guest taps "Offer to help", so the
// landing must render the request's description, not just its title.
func TestHandleRequestLanding_RendersDescription(t *testing.T) {
	s := setupRequestPageTest(t)
	ctx := context.Background()

	s.request.Description = "Ours broke and the gutters are overflowing. Weekend works best."
	if err := s.sqlStorage.Update(ctx, s.request); err != nil {
		t.Fatalf("Failed to update request description: %v", err)
	}

	assertContains(t, s.render(t), "Ours broke and the gutters are overflowing. Weekend works best.")
}

// A requester with a profile photo gets a real avatar (mirrors the event
// landing's host avatar), not a gray initial.
func TestHandleRequestLanding_RequesterAvatarRendered(t *testing.T) {
	s := setupRequestPageTest(t)
	ctx := context.Background()

	// Empty SignedURL makes the mock bucket key its URLs, so the assertion
	// can pin the avatar to the requester's own media row.
	s.mockBucket.SignedURL = ""
	avatar := createTestMedia(t, ctx, s.sqlStorage, s.requester.Id)
	s.requester.MediaIds = []string{avatar.Id}
	if err := s.sqlStorage.Update(ctx, s.requester); err != nil {
		t.Fatalf("Failed to attach avatar media to requester: %v", err)
	}

	body := s.render(t)
	assertContains(t, body, `<img src="http://mock-url/`+storage.MediaBucketKey(s.requester.Id, avatar.Id)+`"`)
	assertNotContains(t, body, "event-avatar-fallback")
}

// No photo — the landing keeps the first-initial fallback.
func TestHandleRequestLanding_RequesterInitialFallback(t *testing.T) {
	s := setupRequestPageTest(t)

	body := s.render(t)
	assertContains(t, body, "event-avatar-fallback")
	assertContains(t, body, ">P<") // Priya
}

func TestHandleRequestLanding_NeedsSummary(t *testing.T) {
	s := setupRequestPageTest(t)
	ctx := context.Background()

	createTestRequestNeed(t, ctx, s.sqlStorage, s.request.Id, s.requester.Id, "Ladder", 1, 1)
	createTestRequestNeed(t, ctx, s.sqlStorage, s.request.Id, s.requester.Id, "Gutter scoop", 1, 0)
	createTestRequestNeed(t, ctx, s.sqlStorage, s.request.Id, s.requester.Id, "Tarp", 2, 0)

	assertContains(t, s.render(t), "3 items · 1 still open")
}

// A request with no needs drops the summary block rather than rendering
// a "0 items" line.
func TestHandleRequestLanding_NoNeedsNoSummary(t *testing.T) {
	s := setupRequestPageTest(t)

	assertNotContains(t, s.render(t), "still open")
}

func TestFormatNeedsSummary(t *testing.T) {
	ctx := context.Background()
	loc, err := l10n.NewLocalizerForContext(ctx)
	if err != nil {
		t.Fatalf("build localizer: %v", err)
	}
	need := func(slotsRemaining int32) *models.PlanningNeed {
		return &models.PlanningNeed{Slots: 1, SlotsRemaining: slotsRemaining}
	}
	tests := []struct {
		name  string
		needs []*models.PlanningNeed
		want  string
	}{
		{"no needs", nil, ""},
		{"single open need", []*models.PlanningNeed{need(1)}, "1 item · 1 still open"},
		{"single claimed need", []*models.PlanningNeed{need(0)}, "1 item · all covered"},
		{
			"mixed",
			[]*models.PlanningNeed{need(1), need(0), need(0), need(2), need(0)},
			"5 items · 2 still open",
		},
		{
			"all claimed",
			[]*models.PlanningNeed{need(0), need(0)},
			"2 items · all covered",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatNeedsSummary(ctx, loc, tt.needs); got != tt.want {
				t.Errorf("formatNeedsSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// OFFERS_RECEIVED is still open for new offers, so it must render.
func TestHandleRequestLanding_OffersReceivedStillOpen(t *testing.T) {
	s := setupRequestPageTest(t)
	ctx := context.Background()

	s.request.State = models.RequestState_REQUEST_STATE_OFFERS_RECEIVED
	if err := s.sqlStorage.Update(ctx, s.request); err != nil {
		t.Fatalf("Failed to update request state: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/go/"+s.shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Result().StatusCode)
	}
	assertContains(t, w.Body.String(), "Offer to help")
}

func TestHandleRequestLanding_CancelledUnavailable(t *testing.T) {
	s := setupRequestPageTest(t)
	ctx := context.Background()

	s.request.State = models.RequestState_REQUEST_STATE_CANCELLED
	if err := s.sqlStorage.Update(ctx, s.request); err != nil {
		t.Fatalf("Failed to update request state: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/go/"+s.shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	body := w.Body.String()
	assertContains(t, body, "no longer available")
	assertNotContains(t, body, "An extension ladder")
	assertNotContains(t, body, "Offer to help")
}

func TestHandleRequestLanding_FulfilledUnavailable(t *testing.T) {
	s := setupRequestPageTest(t)
	ctx := context.Background()

	s.request.State = models.RequestState_REQUEST_STATE_FULFILLED
	if err := s.sqlStorage.Update(ctx, s.request); err != nil {
		t.Fatalf("Failed to update request state: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/go/"+s.shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	assertContains(t, w.Body.String(), "no longer available")
}

func TestHandleRequestLanding_AtCapacity(t *testing.T) {
	s := setupRequestPageTest(t)
	ctx := context.Background()

	for i := range communitylib.MaxCommunityMembers - 1 {
		filler := createTestUser(t, ctx, s.sqlStorage, "Filler"+string(rune('A'+i)))
		createTestMembership(t, ctx, s.sqlStorage, s.community.Id, filler.Id)
	}

	req := httptest.NewRequest(http.MethodGet, "/go/"+s.shareLink.ShortCode, nil)
	w := httptest.NewRecorder()
	s.svc.HandleInvitePage(w, req)

	body := w.Body.String()
	assertContains(t, body, "This community is full")
	assertNotContains(t, body, "Offer to help")
}
