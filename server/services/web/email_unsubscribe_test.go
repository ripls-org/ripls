package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/emailsuppression"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// testUnsubSecret must match the secret setupTestService builds the service with.
const testUnsubSecret = "test-unsubscribe-secret"

func TestHandleEmailUnsubscribe_ValidTokenOptsOut(t *testing.T) {
	svc, store, _ := setupTestService(t)
	ctx := context.Background()

	user := &models.User{Id: uuid.New().String(), Email: "u@example.com", Name: "U", Role: models.Role_ROLE_USER}
	if _, err := store.Insert(ctx, user); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	token := auth.SignUnsubscribeToken([]byte(testUnsubSecret), user.Id)
	req := httptest.NewRequest(http.MethodGet, "/email/unsubscribe?u="+user.Id+"&token="+token, nil)
	rec := httptest.NewRecorder()

	svc.HandleEmailUnsubscribe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	got := &models.User{}
	if err := store.GetByID(ctx, user.Id, got); err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if !got.OffAppEmailOptedOut {
		t.Error("user should be opted out after valid unsubscribe")
	}

	// Idempotent: a second click still returns 200.
	rec2 := httptest.NewRecorder()
	svc.HandleEmailUnsubscribe(rec2, httptest.NewRequest(http.MethodGet, "/email/unsubscribe?u="+user.Id+"&token="+token, nil))
	if rec2.Code != http.StatusOK {
		t.Errorf("repeat unsubscribe status = %d, want 200", rec2.Code)
	}
}

func TestHandleEmailUnsubscribe_InvalidToken(t *testing.T) {
	svc, store, _ := setupTestService(t)
	ctx := context.Background()

	user := &models.User{Id: uuid.New().String(), Email: "u@example.com", Name: "U", Role: models.Role_ROLE_USER}
	if _, err := store.Insert(ctx, user); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/email/unsubscribe?u="+user.Id+"&token=deadbeef", nil)
	rec := httptest.NewRecorder()
	svc.HandleEmailUnsubscribe(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for invalid token", rec.Code)
	}
	got := &models.User{}
	if err := store.GetByID(ctx, user.Id, got); err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if got.OffAppEmailOptedOut {
		t.Error("user must NOT be opted out by an invalid token")
	}
}

func TestHandleEmailUnsubscribe_MissingParams(t *testing.T) {
	svc, _, _ := setupTestService(t)
	rec := httptest.NewRecorder()
	svc.HandleEmailUnsubscribe(rec, httptest.NewRequest(http.MethodGet, "/email/unsubscribe", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for missing params", rec.Code)
	}
}

func TestHandleEmailUnsubscribe_EmailSubjectSuppresses(t *testing.T) {
	svc, store, _ := setupTestService(t)
	ctx := context.Background()

	addr := "invitee@example.com"
	token := auth.SignUnsubscribeToken([]byte(testUnsubSecret), addr)
	target := "/email/unsubscribe?u=" + url.QueryEscape(addr) + "&token=" + token

	rec := httptest.NewRecorder()
	svc.HandleEmailUnsubscribe(rec, httptest.NewRequest(http.MethodGet, target, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	supp, err := emailsuppression.IsSuppressed(ctx, store, addr)
	if err != nil {
		t.Fatalf("IsSuppressed: %v", err)
	}
	if !supp {
		t.Error("email invitee should be suppressed after unsubscribe")
	}

	// Idempotent repeat still confirms.
	rec2 := httptest.NewRecorder()
	svc.HandleEmailUnsubscribe(rec2, httptest.NewRequest(http.MethodGet, target, nil))
	if rec2.Code != http.StatusOK {
		t.Errorf("repeat unsubscribe status = %d, want 200", rec2.Code)
	}
}
