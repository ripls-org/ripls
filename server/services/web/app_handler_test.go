package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeApp_PlaceholderAtRoot(t *testing.T) {
	if bundleAvailable() {
		t.Skip("real Flutter Web bundle is built and embedded; the placeholder " +
			"is only the shell when app_assets/ contains nothing but placeholder.html. " +
			"Run `find server/services/web/app_assets -mindepth 1 ! -name placeholder.html -delete` " +
			"before this test if you want to exercise the placeholder fallback.")
	}

	svc, _, _ := setupTestService(t)
	handler := svc.ServeApp()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("/ returned %d, want 200", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html...", ct)
	}
	body := w.Body.String()
	// Before a Flutter Web build has been copied into app_assets/,
	// the placeholder is the only file present, so it serves as the
	// shell.
	if !strings.Contains(body, "Ripls Web is not built yet") {
		t.Errorf("expected placeholder body when no Flutter Web bundle is built, got: %s", body)
	}
}

func TestServeApp_SpaFallbackForClientRoutes(t *testing.T) {
	svc, _, _ := setupTestService(t)
	handler := svc.ServeApp()

	// Deep URLs the Flutter Web client owns (e.g. /event/{id},
	// /feed, /community/{id}) should fall through to the app shell
	// rather than 404 — that's how client-side routing claims those
	// routes.
	for _, target := range []string{
		"/event/some-experience-id",
		"/feed",
		"/community/abc-123",
		"/gear/def-456",
		"/profile",
	} {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("deep URL %s returned %d, want 200 (SPA fallback)", target, w.Code)
			continue
		}
		if got := w.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s Cache-Control = %q, want no-store on shell fallback", target, got)
		}
	}
}

// TestServeApp_MissingAssetReturns404 guards against the noise the
// PR-review observed: source-map fetches (and any other asset-shaped
// URL the bundle doesn't actually emit) were falling through to the
// SPA index.html response. The browser then tried to parse the HTML
// as a JSON source map and logged a confusing "JSON.parse: unexpected
// character" error in the dev console. The right answer is 404 for
// asset-shaped paths that aren't in the embed.
func TestServeApp_MissingAssetReturns404(t *testing.T) {
	svc, _, _ := setupTestService(t)
	handler := svc.ServeApp()

	for _, target := range []string{
		"/flutter.js.map",
		"/main.dart.js.map",
		"/missing-image.png",
		"/some-unknown.css",
	} {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusNotFound {
			t.Errorf("%s returned %d, want 404 (asset-shaped path that doesn't exist must not SPA-fallback)", target, w.Code)
		}
	}
}

// bundleAvailable returns true when a real Flutter Web bundle has
// been built and copied into `app_assets/` via `npm run build:web`
// (vs the placeholder-only state on a fresh checkout). Tests that
// assert on the bundle's shape skip cleanly when the bundle is
// absent so `go test ./...` works in both states.
func bundleAvailable() bool {
	subFS, err := fs.Sub(AppAssets, "app_assets")
	if err != nil {
		return false
	}
	_, err = fs.Stat(subFS, "index.html")
	return err == nil
}

// TestServeApp_BundleHasBaseHref guards the bug found in PR #2122
// review: if the bundle is built without the correct `--base-href`,
// the emitted index.html carries the wrong base and the browser
// resolves all script tags to a path that returns `text/plain` 404s,
// which `X-Content-Type-Options: nosniff` refuses to execute as JS.
// The user sees a blank page. With the catch-all `/` mount the
// bundle is built with `--base-href=/`. This test catches that
// regression at the HTTP layer before any browser ever loads the
// bundle.
func TestServeApp_BundleHasBaseHref(t *testing.T) {
	if !bundleAvailable() {
		t.Skip("no Flutter Web bundle present in app_assets/; run `npm run build:web` to enable this test")
	}

	svc, _, _ := setupTestService(t)
	handler := svc.ServeApp()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("/ returned %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `<base href="/">`) {
		excerpt := body
		if len(excerpt) > 500 {
			excerpt = excerpt[:500]
		}
		t.Errorf(
			"expected `<base href=\"/\">` in served index.html; "+
				"without it the browser resolves Flutter asset URLs to the "+
				"wrong path and gets text/plain 404s that nosniff refuses "+
				"to execute. build:web must include `--base-href=/`. "+
				"body excerpt: %s",
			excerpt,
		)
	}
}

// TestServeApp_BundleAssetsServeWithJSMime verifies the asset paths
// the bundle's index.html resolves to (under base href /) all
// return 200 + a JavaScript Content-Type — not text/plain, which
// the browser would reject as nosniff blocked.
func TestServeApp_BundleAssetsServeWithJSMime(t *testing.T) {
	if !bundleAvailable() {
		t.Skip("no Flutter Web bundle present in app_assets/; run `npm run build:web` to enable this test")
	}

	svc, _, _ := setupTestService(t)
	handler := svc.ServeApp()

	// flutter_bootstrap.js and main.dart.js are emitted in both the JS
	// and WASM builds (main.dart.js is the WASM build's fallback for
	// older browsers). main.dart.mjs is the WASM loader — present only in
	// a --wasm build, so it's asserted conditionally. All must serve a
	// JavaScript Content-Type, not text/plain, which the browser's
	// X-Content-Type-Options: nosniff guard would refuse to execute.
	jsTargets := []string{"/flutter_bootstrap.js", "/main.dart.js"}
	if hasEmbeddedFile("main.dart.mjs") {
		jsTargets = append(jsTargets, "/main.dart.mjs")
	}
	for _, target := range jsTargets {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("%s returned %d, want 200", target, w.Code)
			continue
		}
		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "javascript") {
			t.Errorf(
				"%s Content-Type = %q; expected text/javascript or "+
					"application/javascript. text/plain would be blocked by the "+
					"browser's X-Content-Type-Options: nosniff guard, leaving the "+
					"page blank.",
				target, ct,
			)
		}
	}

	// main.dart.wasm (WASM build only) must serve application/wasm —
	// streaming WASM compilation (WebAssembly.instantiateStreaming)
	// refuses any other MIME type, so a regression here silently breaks
	// the WASM renderer. Asserted only when the WASM bundle is present.
	if hasEmbeddedFile("main.dart.wasm") {
		r := httptest.NewRequest(http.MethodGet, "/main.dart.wasm", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("/main.dart.wasm returned %d, want 200", w.Code)
		} else if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/wasm") {
			t.Errorf("/main.dart.wasm Content-Type = %q; want application/wasm "+
				"(WebAssembly.instantiateStreaming rejects other types)", ct)
		}
	}
}

// hasEmbeddedFile reports whether the embedded bundle contains `name`.
// Used to skip precompression assertions when a bundle was built
// without running scripts/precompress_web_bundle.js (no .br/.gz
// siblings) so `go test` stays green in that state.
func hasEmbeddedFile(name string) bool {
	subFS, err := fs.Sub(AppAssets, "app_assets")
	if err != nil {
		return false
	}
	_, err = fs.Stat(subFS, name)
	return err == nil
}

// TestServeApp_PrecompressedNegotiation exercises the Accept-Encoding
// content negotiation added in #2119: with brotli/gzip siblings present,
// the handler serves the smallest variant the client accepts, tags it
// with the right Content-Encoding, keeps the CANONICAL Content-Type, and
// sets Vary — degrading br -> gzip -> identity. This is what stops the
// ~9.6 MB main.dart.wasm from shipping raw.
func TestServeApp_PrecompressedNegotiation(t *testing.T) {
	if !bundleAvailable() {
		t.Skip("no Flutter Web bundle present in app_assets/; run `npm run build:web` to enable this test")
	}
	if !hasEmbeddedFile("main.dart.wasm.br") || !hasEmbeddedFile("main.dart.wasm.gz") {
		t.Skip("bundle built without precompressed siblings; run `npm run build:web` " +
			"(which runs scripts/precompress_web_bundle.js) to enable this test")
	}

	svc, _, _ := setupTestService(t)
	handler := svc.ServeApp()

	const target = "/main.dart.wasm"
	const wantType = "application/wasm"

	// Capture the identity cache header so we can assert negotiation
	// doesn't disturb it.
	identityCache := func() string {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Header().Get("Cache-Control")
	}()

	cases := []struct {
		name           string
		acceptEncoding string
		wantEncoding   string // "" means identity (no Content-Encoding)
	}{
		{"brotli preferred", "gzip, deflate, br", "br"},
		{"gzip when br absent", "gzip, deflate", "gzip"},
		{"identity when none accepted", "", ""},
		{"identity when br refused via q=0", "br;q=0, gzip;q=0", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, target, nil)
			if tc.acceptEncoding != "" {
				r.Header.Set("Accept-Encoding", tc.acceptEncoding)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)

			if w.Code != http.StatusOK {
				t.Fatalf("%s returned %d, want 200", target, w.Code)
			}
			if got := w.Header().Get("Content-Encoding"); got != tc.wantEncoding {
				t.Errorf("Content-Encoding = %q, want %q", got, tc.wantEncoding)
			}
			// Content-Type must be the canonical wasm type regardless of
			// whether a compressed variant was served.
			if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, wantType) {
				t.Errorf("Content-Type = %q, want %s (canonical, not the .br/.gz type)", ct, wantType)
			}
			// Vary must advertise Accept-Encoding so shared caches don't
			// hand a brotli body to a gzip-only client.
			if v := w.Header().Get("Vary"); !strings.Contains(v, "Accept-Encoding") {
				t.Errorf("Vary = %q, want it to contain Accept-Encoding", v)
			}
			// Caching is keyed on the canonical name and must be identical
			// across encodings.
			if got := w.Header().Get("Cache-Control"); got != identityCache {
				t.Errorf("Cache-Control = %q, want %q (unchanged by negotiation)", got, identityCache)
			}
			// A compressed response must actually carry compressed (smaller)
			// bytes, not the raw file.
			if tc.wantEncoding != "" && w.Body.Len() == 0 {
				t.Errorf("expected a non-empty compressed body for %s", tc.wantEncoding)
			}
		})
	}
}

// TestServeApp_PrecompressedSiblingsNotServedDirectly guards that a
// direct request for a .br/.gz URL 404s — the siblings are reachable
// only through negotiation on the canonical URL, so serving one directly
// (with a wasm/js Content-Type but compressed bytes and no
// Content-Encoding) would corrupt the response. Runs without a bundle:
// the .br/.gz suffix is rejected before the embed is consulted.
func TestServeApp_PrecompressedSiblingsNotServedDirectly(t *testing.T) {
	svc, _, _ := setupTestService(t)
	handler := svc.ServeApp()

	for _, target := range []string{
		"/main.dart.wasm.br",
		"/main.dart.wasm.gz",
		"/main.dart.js.br",
		"/flutter_bootstrap.js.gz",
	} {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s returned %d, want 404 (precompressed siblings are not served directly)", target, w.Code)
		}
	}
}

func TestServeApp_HashedAssetGetsImmutableCache(t *testing.T) {
	// We don't have a real hashed asset in app_assets/ yet (those
	// land via build:web), but we can exercise the cache-header
	// helper directly.
	if got := func() string {
		w := httptest.NewRecorder()
		setAppCacheHeaders(w, "canvaskit/canvaskit.wasm")
		return w.Header().Get("Cache-Control")
	}(); got != "public, max-age=31536000, immutable" {
		t.Errorf("canvaskit asset Cache-Control = %q, want immutable", got)
	}

	if got := func() string {
		w := httptest.NewRecorder()
		setAppCacheHeaders(w, "main.dart.js_1.part.js")
		return w.Header().Get("Cache-Control")
	}(); got != "public, max-age=31536000, immutable" {
		t.Errorf("deferred-chunk Cache-Control = %q, want immutable", got)
	}

	if got := func() string {
		w := httptest.NewRecorder()
		setAppCacheHeaders(w, "index.html")
		return w.Header().Get("Cache-Control")
	}(); got != "no-store" {
		t.Errorf("index.html Cache-Control = %q, want no-store", got)
	}
}
