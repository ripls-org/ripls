// Serves the Flutter Web bundle (embedded via app_embed.go) as the
// catch-all root handler. Hashed assets (files with a content hash
// in the filename like main.dart.js, *.png with a content hash) get
// an immutable long-cache header; index.html and the Flutter
// manifest get no-store so the latest deploy lands immediately.
// Unknown paths fall through to index.html so client-side routing
// handles deep links such as /event/{experience_id}, /feed,
// /community/{id}, etc.

package web

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"go.ripls.org/ripls/server/logging"
)

// ServeApp returns an http.Handler that serves the embedded Flutter
// Web bundle. Mounted at "/" as a catch-all; more-specific mux
// patterns (/go/, /css/, /.well-known/, /health, /ripls.api.*, etc.)
// claim their paths first via Go's longest-prefix-wins dispatch.
func (s *Service) ServeApp() http.Handler {
	subFS, err := fs.Sub(AppAssets, "app_assets")
	if err != nil {
		// Compile-time invariant: app_assets is the embedded
		// directory's name. A failure here means the embed itself
		// is broken — surface it loudly at startup rather than at
		// request time.
		panic("web: app_assets sub-FS missing: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(subFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlPath := r.URL.Path
		if urlPath == "" || urlPath == "/" {
			s.serveIndex(w, r, subFS)
			return
		}

		cleaned := path.Clean(strings.TrimPrefix(urlPath, "/"))

		// Precompressed siblings (main.dart.wasm.br / .gz, produced by
		// scripts/precompress_web_bundle.js) are reachable ONLY through
		// content negotiation on the canonical URL below — never served
		// directly, which would leak the wrong Content-Type. A direct
		// request for one is an asset-shaped path that doesn't exist as
		// a canonical asset → 404.
		if strings.HasSuffix(cleaned, ".br") || strings.HasSuffix(cleaned, ".gz") {
			http.NotFound(w, r)
			return
		}

		if _, err := fs.Stat(subFS, cleaned); err != nil {
			// Two kinds of unknown paths land here. Disambiguate by
			// the presence of a file extension on the last segment:
			//
			//   * Asset-shaped requests (e.g. `/flutter.js.map`,
			//     `/missing-image.png`) → 404. Returning
			//     index.html (HTML) here would confuse browsers /
			//     CSS link tags / source-map loaders that expect
			//     a specific Content-Type.
			//   * Client-route-shaped requests (e.g. `/event/
			//     {experience_id}`, `/feed`, `/community/{id}`) →
			//     SPA fallback to index.html so the Flutter
			//     client-side router can claim them.
			if looksLikeAssetPath(cleaned) {
				http.NotFound(w, r)
				return
			}
			s.serveIndex(w, r, subFS)
			return
		}

		setAppCacheHeaders(w, cleaned)

		// The body varies by Accept-Encoding (we may serve a
		// precompressed variant), so any shared cache must key on it.
		// Set this on the identity path too, not just the compressed one.
		w.Header().Set("Vary", "Accept-Encoding")

		// Serve a precompressed sibling when the client accepts it and
		// one exists — brotli preferred, then gzip. Falls back to the
		// raw file otherwise. This is what stops the ~9.6 MB
		// main.dart.wasm from shipping raw on every cold web visit.
		if enc, ext := negotiateEncoding(r.Header.Get("Accept-Encoding"), subFS, cleaned); enc != "" {
			if s.servePrecompressed(w, r, subFS, cleaned, cleaned+ext, enc) {
				return
			}
			// servePrecompressed only returns false on an unexpected read
			// error after Stat succeeded; fall through to identity.
		}

		fileServer.ServeHTTP(w, r)
	})
}

// negotiateEncoding picks the best precompressed variant to serve for
// `cleaned` given the request's Accept-Encoding: brotli if accepted and
// a `.br` sibling exists, else gzip if accepted and a `.gz` sibling
// exists, else identity (empty encoding). Returns the Content-Encoding
// token and the sibling extension (".br"/".gz"), or ("", "") for
// identity.
func negotiateEncoding(acceptEncoding string, subFS fs.FS, cleaned string) (encoding, ext string) {
	if acceptsEncoding(acceptEncoding, "br") {
		if _, err := fs.Stat(subFS, cleaned+".br"); err == nil {
			return "br", ".br"
		}
	}
	if acceptsEncoding(acceptEncoding, "gzip") {
		if _, err := fs.Stat(subFS, cleaned+".gz"); err == nil {
			return "gzip", ".gz"
		}
	}
	return "", ""
}

// acceptsEncoding reports whether an Accept-Encoding header offers
// `coding` as acceptable. It tokenizes on commas, ignores parameters,
// and honors an explicit `q=0` ("not acceptable") for the coding.
func acceptsEncoding(header, coding string) bool {
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(part, ";")
		if !strings.EqualFold(strings.TrimSpace(fields[0]), coding) {
			continue
		}
		// Present. Honor an explicit q=0 as a refusal.
		for _, param := range fields[1:] {
			param = strings.TrimSpace(param)
			if strings.HasPrefix(param, "q=") && strings.TrimSpace(param[2:]) == "0" {
				return false
			}
		}
		return true
	}
	return false
}

// servePrecompressed writes the precompressed sibling `variant` for the
// canonical asset `cleaned` with the negotiated `encoding`. It sets
// Content-Type from the CANONICAL extension (so main.dart.wasm.br still
// serves application/wasm, not the meaningless .br type) and prevents
// content sniffing on the compressed bytes. Cache headers and Vary are
// already set by the caller. Returns false only if the sibling can't be
// read after its Stat succeeded, so the caller can fall back to identity.
func (s *Service) servePrecompressed(
	w http.ResponseWriter, r *http.Request, subFS fs.FS, cleaned, variant, encoding string,
) bool {
	body, err := fs.ReadFile(subFS, variant)
	if err != nil {
		logging.LoggerWithContext(r.Context()).WarnContext(r.Context(),
			"precompressed asset stat/read mismatch; serving identity",
			"operation", "ServeApp.servePrecompressed",
			"variant", variant,
			"error", err,
		)
		return false
	}

	// Content-Type from the canonical extension. Always set it explicitly:
	// with Content-Encoding present, letting net/http sniff would inspect
	// the compressed bytes and mislabel them. octet-stream is the safe
	// default for extensions Go doesn't know (.map/.symbols).
	contentType := mime.TypeByExtension(path.Ext(cleaned))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Encoding", encoding)
	// G705 sees request-derived data reaching the response body. `body` is a
	// build artifact read from the embedded app_assets FS, reachable only via
	// the cleaned path above, and Content-Type is set explicitly from the
	// canonical extension rather than sniffed — so the response cannot be
	// coerced into executing attacker markup.
	_, _ = w.Write(body) //nolint:gosec // G705: body is a static build asset from the embedded FS, served with an explicit Content-Type.
	return true
}

// serveIndex writes index.html (or the placeholder when no Flutter
// Web build has been copied into app_assets/ yet) with no-store
// caching so the latest deploy is always reflected.
func (s *Service) serveIndex(w http.ResponseWriter, r *http.Request, subFS fs.FS) {
	// Prefer the real index.html (post-build); fall back to the
	// placeholder so a fresh checkout still serves something
	// recognizable before `npm run build:web` has been run.
	target := "index.html"
	if _, err := fs.Stat(subFS, target); err != nil {
		target = "placeholder.html"
	}
	body, err := fs.ReadFile(subFS, target)
	if err != nil {
		logging.LoggerWithContext(r.Context()).ErrorContext(r.Context(),
			"failed to read app shell",
			"operation", "ServeApp.serveIndex",
			"target", target,
			"error", err,
		)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

// setAppCacheHeaders applies the long-immutable-cache header to
// hashed asset filenames; otherwise no-store so manifests and the
// shell stay fresh across deploys.
func setAppCacheHeaders(w http.ResponseWriter, cleanedPath string) {
	if isHashedAsset(cleanedPath) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
}

// isHashedAsset returns true when the filename carries a Flutter-
// generated content hash that makes its URL immutable. Flutter Web
// release builds emit files like main.dart.js, main.dart.js_<chunk>.part.js,
// assets/AssetManifest.bin.json (NOT hashed), and CanvasKit pieces in
// canvaskit/* (versioned). For now, treat the canvaskit/ subtree and
// any *.part.js as immutable; everything else is no-store. This is
// deliberately conservative — Step 8 can tighten when the deferred-
// route chunk naming stabilizes.
func isHashedAsset(cleanedPath string) bool {
	if strings.HasPrefix(cleanedPath, "canvaskit/") {
		return true
	}
	if strings.HasSuffix(cleanedPath, ".part.js") {
		return true
	}
	return false
}

// looksLikeAssetPath returns true when the request path looks like
// an asset URL rather than a client-side route. A file extension on
// the last segment is the heuristic — `flutter.js.map`, `logo.png`,
// `style.css` are assets and should 404 when missing; `event/abc-123`
// is a client route and should SPA-fallback to index.html.
//
// The heuristic isn't perfect (a client route happens to contain a
// `.` in a path parameter would be treated as an asset), but client
// routes in this codebase don't carry dots in their path segments,
// so it's a clean cut for our usage.
func looksLikeAssetPath(cleanedPath string) bool {
	// Inspect the last segment for an extension. `path.Ext` does
	// exactly this — returns "" when the last segment has no dot.
	return path.Ext(cleanedPath) != ""
}
