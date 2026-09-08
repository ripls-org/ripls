package middleware

import (
	"net/http"

	"go.ripls.org/ripls/server/l10n"
)

// AcceptLanguage returns middleware that captures the inbound
// request's Accept-Language header into the request context via
// l10n.WithAcceptLanguage. Downstream handlers (RPCs, fan-out
// subscribers) can resolve the recipient locale by calling
// l10n.LocaleFromContext without touching net/http.
//
// Place this middleware early in the chain (alongside RequestID and
// RemoteAddr) so every request — authenticated or not — carries the
// header forward. The l10n resolver applies the documented priority
// order (explicit > Accept-Language > stored preference > en) and
// no-ops cleanly when the header is absent.
func AcceptLanguage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Accept-Language")
		if header == "" {
			next.ServeHTTP(w, r)
			return
		}
		ctx := l10n.WithAcceptLanguage(r.Context(), header)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
