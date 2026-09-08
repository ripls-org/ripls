package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/text/language"

	"go.ripls.org/ripls/server/l10n"
)

func TestAcceptLanguage_CapturesHeaderIntoContext(t *testing.T) {
	var captured language.Tag
	h := AcceptLanguage(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = l10n.LocaleFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "es-MX,en;q=0.9")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if captured != language.Spanish {
		t.Errorf("captured locale = %v, want %v", captured, language.Spanish)
	}
}

func TestAcceptLanguage_AbsentHeaderFallsThrough(t *testing.T) {
	called := false
	h := AcceptLanguage(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if got := l10n.LocaleFromContext(r.Context()); got != l10n.DefaultTag {
			t.Errorf("locale without Accept-Language = %v, want default", got)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !called {
		t.Error("downstream handler was not invoked")
	}
}
