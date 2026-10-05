package main

// HTTP surface for the server binary: public web routes (SSR landing pages,
// SMS webhooks, static assets, health probes), the table-driven registration
// of every authenticated Connect service, dev-only admin/pprof endpoints, and
// the per-stream middleware chain wrapped around the mux.

import (
	"context"
	"net/http"
	"net/http/pprof"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"github.com/gorilla/handlers"
	"golang.org/x/time/rate"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/config"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/middleware"
	"go.ripls.org/ripls/server/middleware/activitystamp"
	"go.ripls.org/ripls/server/middleware/ratelimit"
	"go.ripls.org/ripls/server/storage"
)

// connectRoute pairs a Connect handler with its mount path so registrations
// can be listed in a table.
type connectRoute struct {
	path    string
	handler http.Handler
}

// route adapts apiconnect.NewXServiceHandler's (path, handler) return into a
// connectRoute table entry.
func route(path string, handler http.Handler) connectRoute {
	return connectRoute{path: path, handler: handler}
}

// buildHandler assembles the full HTTP handler: mux with every public and
// authenticated route mounted, wrapped in the per-stream middleware chain.
// shutdown is cancelled when the server begins shutting down; streaming RPCs
// end then, so the drain is not held open by them.
func buildHandler(shutdown context.Context, cfg *config.Config, logger *logging.Logger, sqlStorage *storage.ProtoSQLStorage, bucketStorage storage.BucketStorage, authMiddleware *authn.Middleware, s *serverServices) http.Handler {
	mux := http.NewServeMux()

	// Serve media files from local bucket storage if using local mode
	if cfg.LocalMediaStorage != "" {
		localBucket, ok := bucketStorage.(*storage.LocalBucketStorage)
		if ok {
			logger.Warn("serving local media files", "path", "/media/")
			mediaFileServer := http.StripPrefix("/media/", http.FileServer(http.Dir(localBucket.GetBasePath())))
			mux.Handle("/media/", mediaFileServer)
		}
	}

	// hostMiddleware injects the client's request host into the context so
	// LocalBucketStorage can generate presigned URLs that work from any client
	// (e.g. Android emulator at 10.0.2.2, localhost, etc.). No-op with GCS,
	// whose presigned URLs carry their own host.
	hostMiddleware := func(next http.Handler) http.Handler { return next }
	if cfg.LocalMediaStorage != "" {
		hostMiddleware = middleware.RequestHost(cfg.Port)
	}

	// Register web pages (public, no auth required).
	// The POST /go/{code}/decline pattern is registered before the
	// /go/ prefix so http.ServeMux's more-specific-pattern-wins
	// routing dispatches decline POSTs to the dedicated handler
	// rather than the SSR landing.
	//
	// /go/{code}/decline is anonymous and unauthenticated; the only
	// abuse dimension is the client IP. Per-IP rate limit: 60 events
	// per hour (rate 1/60s) with a burst of 5 — a moderate visitor
	// can decline a few links rapidly without being capped, but
	// scripts that try to inflate the counter hit the bucket fast.
	// Sub-issue 5 (#2052) tightens further once production data
	// shows the abuse shape.
	declineRateLimit := ratelimit.PerIPHTTP(
		ratelimit.HTTPBudget{
			Rate:  rate.Limit(60.0 / 3600.0),
			Burst: 5,
		},
		"RecordWebDecline",
	)
	mux.Handle("POST /go/{code}/decline", hostMiddleware(declineRateLimit(http.HandlerFunc(s.web.HandleRecordWebDecline))))
	// AcceptLanguage captures the visitor's Accept-Language so the SSR landing
	// localizes server-rendered strings (e.g. the nameless-community label) for
	// this pre-auth surface — there's no logged-in user to read a preference from.
	mux.Handle("/go/", hostMiddleware(middleware.AcceptLanguage(http.HandlerFunc(s.web.HandleInvitePage))))
	// One-click unsubscribe for off-app notification emails (EMAIL-1). Public
	// and unauthenticated; authorization is the signed token in the link.
	mux.Handle("/email/unsubscribe", hostMiddleware(http.HandlerFunc(s.web.HandleEmailUnsubscribe)))
	mux.Handle("POST /sms/webhook", hostMiddleware(http.HandlerFunc(s.web.HandleSMSWebhook)))
	// Outbound SMS delivery status callbacks (#2569) — observability-only; the
	// Messaging Service posts delivery outcomes here. Distinct payload from the
	// inbound webhook (MessageStatus/ErrorCode, no From/Body); signature-verified
	// the same way.
	mux.Handle("POST /sms/status", hostMiddleware(http.HandlerFunc(s.web.HandleSMSStatusCallback)))
	mux.Handle("POST /email/status", hostMiddleware(http.HandlerFunc(s.web.HandleEmailStatusWebhook)))
	mux.Handle("/css/", s.web.ServeStaticAssets())
	mux.Handle("/assets/", s.web.ServeStaticAssets())
	// Self-hosted @font-face files for the SSR landing pages (fonts.css
	// references /fonts/*.woff2). Served from the same embedded website
	// content tree; unmatched paths fall through to the Flutter bundle.
	mux.Handle("/fonts/", s.web.ServeStaticAssets())
	mux.Handle("/.well-known/", s.web.ServeWellKnown())
	// Catch-all: any path not claimed by a more-specific handler falls
	// through to the Flutter Web bundle. Go's http.ServeMux uses
	// longest-prefix-wins, so /go/, /css/, /.well-known/, /health,
	// /ripls.api.*, etc. continue to dispatch to their dedicated
	// handlers; only paths like /, /feed, /community/{id}, /gear/{id}
	// land here. The bundle itself is built with `--base-href=/`
	// (npm run build:web*), so client-side routing maps the browser
	// URL directly to GoRouter paths with no base-href stripping.
	// Mounted last so it's clearly the fallback; mux dispatch order
	// is independent of registration order, but keeping it visually
	// last reflects the intent.
	mux.Handle("/", hostMiddleware(s.web.ServeApp()))

	// commonOpts apply to every Connect handler.
	//
	// Interceptor chain (outermost → innermost):
	//   1. rate-limit — prod gets every rule via ratelimit.AllRules() with
	//      each budget's MetricsOnly flag honored as declared. Dev gets
	//      ratelimit.AllRulesAsMetricsOnly(): the same rule set with
	//      every budget forced to MetricsOnly so dev never sees a 429
	//      (avoids throttling simulation runs and integration tests)
	//      while every rule still produces "rate_limited" warn logs. The
	//      mode field distinguishes "would-fire-in-prod" (mode="dev_shadow")
	//      from "already-soaking-in-prod" (mode="metrics_only") so local
	//      observation matches prod log queries.
	//   2. preferred-language — populates tier-3 of the LocaleFromContext
	//      priority chain (User.preferred_language) for authenticated callers
	//      that didn't send Accept-Language. No-op for the Flutter client,
	//      which always sends the header. See preferred_language.go.
	rateLimitRules := ratelimit.AllRules()
	if cfg.DevMode {
		rateLimitRules = ratelimit.AllRulesAsMetricsOnly()
	}
	commonOpts := []connect.HandlerOption{
		connect.WithInterceptors(ratelimit.Interceptor(rateLimitRules...)),
		connect.WithInterceptors(middleware.PreferredLanguageInterceptor(sqlStorage)),
		connect.WithInterceptors(middleware.StreamShutdownInterceptor(shutdown)),
	}

	// Per-user daily activity stamps for the ops digest (#2665). Sits
	// inside the authn wrap so it sees the authenticated user; async and
	// best-effort, so it adds no latency to the request path.
	stamper := activitystamp.New(sqlStorage, nil)

	// Authenticated Connect services. Every handler gets commonOpts — the
	// rate-limit interceptor is pass-through for procedures with no matching
	// rule, so listing a service here does not throttle it until a rule is
	// added in server/middleware/ratelimit.
	for _, r := range []connectRoute{
		route(apiconnect.NewChatServiceHandler(s.chat, commonOpts...)),
		route(apiconnect.NewCommunityServiceHandler(s.community, commonOpts...)),
		route(apiconnect.NewGearServiceHandler(s.gear, commonOpts...)),
		route(apiconnect.NewTransferServiceHandler(s.transfer, commonOpts...)),
		route(apiconnect.NewRequestServiceHandler(s.request, commonOpts...)),
		route(apiconnect.NewExperienceServiceHandler(s.experience, commonOpts...)),
		route(apiconnect.NewUnifiedCreateServiceHandler(s.unifiedCreate, commonOpts...)),
		route(apiconnect.NewLocationServiceHandler(s.location, commonOpts...)),
		route(apiconnect.NewUserServiceHandler(s.user, commonOpts...)),
		route(apiconnect.NewDeviceServiceHandler(s.device, commonOpts...)),
		route(apiconnect.NewPortfolioServiceHandler(s.portfolio, commonOpts...)),
		route(apiconnect.NewFeedServiceHandler(s.feed, commonOpts...)),
		route(apiconnect.NewESMServiceHandler(s.esm, commonOpts...)),
		route(apiconnect.NewWorkshopServiceHandler(s.workshop, commonOpts...)),
		route(apiconnect.NewProfileServiceHandler(s.profile, commonOpts...)),
		route(apiconnect.NewSearchServiceHandler(s.search, commonOpts...)),
		route(apiconnect.NewFeedbackServiceHandler(s.feedback, commonOpts...)),
		route(apiconnect.NewImpactServiceHandler(s.impact, commonOpts...)),
	} {
		mux.Handle(r.path, authMiddleware.Wrap(stamper.Wrap(r.handler)))
	}

	// MediaService is authenticated like the table above but carries two
	// extras: a transport-level body cap (#1953) sized to the largest allowed
	// upload (video) plus headroom for proto/base64 framing — a backstop ahead
	// of the per-kind checks in storage.StoreMedia — and the request-host
	// middleware for local presigned URLs. commonOpts is copied so the extra
	// option stays scoped to this handler.
	mediaOpts := append(append([]connect.HandlerOption{}, commonOpts...),
		connect.WithReadMaxBytes(int(storage.MaxVideoUploadBytes)+16*1024*1024))
	mediaPath, mediaHandler := apiconnect.NewMediaServiceHandler(s.media, mediaOpts...)
	mux.Handle(mediaPath, hostMiddleware(authMiddleware.Wrap(stamper.Wrap(mediaHandler))))

	// Register LoginService without authentication (required for registration/login).
	// Rate-limit rules for the login surface live in ratelimit.DefaultLoginRules
	// and are applied via the shared interceptor in commonOpts (prod only).
	loginPath, loginHandler := apiconnect.NewLoginServiceHandler(s.login, commonOpts...)
	mux.Handle(loginPath, hostMiddleware(loginHandler))
	if cfg.DevMode {
		logger.Warn("DEV MODE ENABLED — do not use in production")

		// Register AdminService only in dev mode. These endpoints expose
		// destructive operations (simulation cleanup) and must not exist in production.
		adminPath, adminHandler := apiconnect.NewAdminServiceHandler(s.admin, commonOpts...)
		mux.Handle(adminPath, adminHandler)

		// Expose pprof profiling endpoints (CPU, heap, goroutine, mutex, block).
		// These are only available in dev mode and should never be exposed in
		// production. Register specific handlers on the app's own mux rather
		// than mounting http.DefaultServeMux so future code that exposes
		// DefaultServeMux (e.g. a separate metrics port) cannot re-expose
		// pprof by accident. Index also serves the named profiles
		// (/debug/pprof/heap, /goroutine, /allocs, /block, /mutex,
		// /threadcreate) via its filename suffix routing.
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		logger.Info("pprof endpoints enabled at /debug/pprof/")
	} else {
		logger.Info("production mode: invitation tokens required for registration (except first user)")
	}

	// Register WaitlistService without authentication (public signup)
	waitlistPath, waitlistHandler := apiconnect.NewWaitlistServiceHandler(s.waitlist, commonOpts...)
	mux.Handle(waitlistPath, waitlistHandler)

	// Register HealthService without authentication (for Cloud Monitoring uptime checks)
	healthPath, healthHandler := apiconnect.NewHealthServiceHandler(s.health, commonOpts...)
	mux.Handle(healthPath, healthHandler)
	// Also register /health GET handler for simpler HTTP health checks
	mux.Handle("/health", s.health.HealthHandler())
	// Register /readyz GET handler for orchestrator readiness probes. Returns
	// 503 when critical dependencies (database, storage) are unhealthy so that
	// Cloud Run / k8s can route traffic away from this pod.
	mux.Handle("/readyz", s.health.ReadyHandler())

	// Configure CORS middleware. Origins were validated and resolved by
	// config.Parse before any deferred cleanup was registered; cfg.CORSOrigins
	// is safe to use directly.
	corsMiddleware := handlers.CORS(
		handlers.AllowedOriginValidator(middleware.CORSOriginValidator(cfg.CORSOrigins)),
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedHeaders([]string{"Content-Type", "Authorization", "Accept-Language", "Connect-Protocol-Version", "Connect-Timeout-Ms", middleware.RequestIDHeader}),
		handlers.ExposedHeaders([]string{"Connect-Protocol-Version", middleware.RequestIDHeader}),
	)

	// Build the per-request middleware chain around mux. The entire chain
	// is the server's Handler, so unencrypted HTTP/2 (h2c) — enabled via
	// server.Protocols in main — runs every stream through the full chain.
	// This ordering matters: if any middleware were placed outside the
	// handler the server dispatches per stream, it would run once on the
	// connection and be bypassed for every HTTP/2 stream that follows —
	// which is how X-Request-ID stopped reaching the server (#1799).
	//
	// Per-stream chain: [simulation clock (dev)] → request ID → remote
	// addr → accept-language → logging → query stats → CORS →
	// mux. Panic recovery is the outermost wrapper since it must catch
	// panics from anything downstream.
	muxStack := corsMiddleware(mux)
	muxStack = middleware.QueryStats(logger)(muxStack)
	muxStack = middleware.Logging(logger)(muxStack)
	muxStack = middleware.AcceptLanguage(muxStack)
	muxStack = middleware.RemoteAddr(muxStack)
	muxStack = middleware.RequestID(muxStack)
	if cfg.DevMode {
		muxStack = clock.SimulationTimestamp(muxStack)
	}
	return middleware.PanicRecovery(logger)(muxStack)
}
