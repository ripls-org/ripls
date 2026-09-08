package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// newDeclineRequest builds a POST request to /go/{code}/decline with
// the path parameter wired through http.ServeMux's pattern routing so
// r.PathValue("code") resolves correctly inside the handler.
func newDeclineRequest(t *testing.T, shortCode string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/go/"+shortCode+"/decline", nil)
	r.SetPathValue("code", shortCode)
	return r
}

func TestHandleRecordWebDecline_IncrementsCounter(t *testing.T) {
	s := setupEventPageTest(t)
	ctx := context.Background()

	// First decline.
	w := httptest.NewRecorder()
	s.svc.HandleRecordWebDecline(w, newDeclineRequest(t, s.shareLink.ShortCode))
	if w.Code != http.StatusNoContent {
		t.Fatalf("first decline: expected 204, got %d (body=%q)", w.Code, w.Body.String())
	}

	got := &models.Experience{}
	if err := s.sqlStorage.GetByID(ctx, s.experience.Id, got); err != nil {
		t.Fatalf("GetByID experience: %v", err)
	}
	if c := got.GetWebDeclineCount(); c != 1 {
		t.Errorf("after first decline, web_decline_count = %d, want 1", c)
	}

	// Second decline accumulates (no rate limit at this layer — sub-issue 5 caps).
	w = httptest.NewRecorder()
	s.svc.HandleRecordWebDecline(w, newDeclineRequest(t, s.shareLink.ShortCode))
	if w.Code != http.StatusNoContent {
		t.Fatalf("second decline: expected 204, got %d", w.Code)
	}

	got2 := &models.Experience{}
	if err := s.sqlStorage.GetByID(ctx, s.experience.Id, got2); err != nil {
		t.Fatalf("GetByID experience: %v", err)
	}
	if c := got2.GetWebDeclineCount(); c != 2 {
		t.Errorf("after second decline, web_decline_count = %d, want 2", c)
	}
}

func TestHandleRecordWebDecline_MissingShortCode(t *testing.T) {
	s := setupEventPageTest(t)

	r := httptest.NewRequest(http.MethodPost, "/go//decline", nil)
	r.SetPathValue("code", "")
	w := httptest.NewRecorder()
	s.svc.HandleRecordWebDecline(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty short code, got %d", w.Code)
	}
}

func TestHandleRecordWebDecline_NotFound(t *testing.T) {
	s := setupEventPageTest(t)

	w := httptest.NewRecorder()
	s.svc.HandleRecordWebDecline(w, newDeclineRequest(t, "NOTREAL1"))

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown short code, got %d", w.Code)
	}
}

func TestHandleRecordWebDecline_Revoked(t *testing.T) {
	s := setupEventPageTest(t)
	ctx := context.Background()

	s.shareLink.IsRevoked = true
	if err := s.sqlStorage.Update(ctx, s.shareLink); err != nil {
		t.Fatalf("Update share link IsRevoked: %v", err)
	}

	w := httptest.NewRecorder()
	s.svc.HandleRecordWebDecline(w, newDeclineRequest(t, s.shareLink.ShortCode))

	if w.Code != http.StatusGone {
		t.Errorf("expected 410 for revoked link, got %d", w.Code)
	}
}

func TestHandleRecordWebDecline_NonEventTarget(t *testing.T) {
	s := setupEventPageTest(t)
	ctx := context.Background()

	// Mutate the existing share link into a community-invite target —
	// declines on non-event links are explicitly out of scope.
	s.shareLink.Target = &models.ShareLink_CommunityInviteId{
		CommunityInviteId: s.community.Id,
	}
	if err := s.sqlStorage.Update(ctx, s.shareLink); err != nil {
		t.Fatalf("Update share link target: %v", err)
	}

	w := httptest.NewRecorder()
	s.svc.HandleRecordWebDecline(w, newDeclineRequest(t, s.shareLink.ShortCode))

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-event share link, got %d", w.Code)
	}
}
