# Observability Review - 2026-04-28

## Executive Summary

Significant observability progress has landed since the last review on 2026-04-24. Four of the five "immediate action" items from the previous report are complete: the Dockerfile HEALTHCHECK now probes `/health`, all five `debugPrint` calls in `main.dart` have been replaced with structured `_log` calls, a new `RemoteAddr` middleware is wired in, and comprehensive `authentication_failed` audit logging now covers email, OIDC, phone, and refresh-token paths with masked email, reason, provider, and remote address fields — backed by a dedicated `audit_logging_test.go` test file. Six additional client services (`community_service`, `chat_service`, `transfer_service`, `media_service`, `fcm_service`, `community_event_poller`) have migrated from raw `Logger` to `ObservableLogger`, joining `auth_service` for a total of seven adopters.

The remaining gaps are concentrated on the **monitoring layer** (Prometheus/OpenTelemetry metrics still absent), **the readiness signal** (Docker still uses `/health` which always returns 200 regardless of dependency state — there's no `/readyz`), the **hardcoded JWT signing key** at `server/main.go:524`, and the **pprof DefaultServeMux footgun**. Nine smaller services in `app/lib/services/` still use raw `Logger`. The trend is clearly **improving** — this is the most observability work that has landed in any single review cycle since the project's first review.

**Current Risk Assessment:** Medium-low. Debug-time observability (logs + crash reports + audit logs + health checks) is now strong. Proactive monitoring (metrics, alerting, tracing) remains the dominant gap.

## Changes Since Last Review

**Previous Review Date**: 2026-04-24

**Remediated Issues:**

- Dockerfile HEALTHCHECK now probes `/health`
  - Location: `Dockerfile:89` — changed from `--spider http://.../` to `wget --quiet --tries=1 http://localhost:${PORT:-8080}/health -O /dev/null`
- `debugPrint` calls in `main.dart` removed
  - All five calls converted to `_log.info()` / `_log.warning()` / `_log.severe()` (verified at `main.dart:327, 378, 384, 387, 435` — only `_log.*` calls remain).
- Failed-authentication audit logging added across all auth paths
  - `server/services/login/authentication.go:32-48` (email login: unknown email, wrong password)
  - `server/services/login/authentication.go:108-117, 132-141` (OIDC: token validation, unknown email)
  - `server/services/login/refresh.go:37, 60` (refresh token failures)
  - `server/services/login/phone_auth.go:169, 185` (phone auth failures)
  - All log lines include masked email, reason, remote_addr, and provider where relevant.
- `RemoteAddr` middleware introduced
  - `server/middleware/remote_addr.go` extracts `X-Forwarded-For` first hop, calls `logging.WithRemoteAddr(ctx, addr)` so all subsequent handlers see the client IP.
- Comprehensive audit-log test coverage
  - New file: `server/services/login/audit_logging_test.go` exercises email-unknown, email-wrong-password, OIDC-token-validation, OIDC-unknown-email failure paths.
- ObservableLogger adoption expanded from 1 → 7 services
  - New adopters: `community_service.dart`, `chat_service.dart`, `transfer_service.dart`, `media_service.dart`, `fcm_service.dart`, `community_event_poller.dart`.

**Unaddressed Issues:**

- No Prometheus/OpenTelemetry metrics collection
  - Original Priority: Critical
  - Days Open: ~511 days
  - No `server/metrics/` package; `grep -r "prometheus\|otel.Tracer\|otel.Meter"` in `server/` returns zero hits.
- Hardcoded JWT secret (`dev-secret-key`)
  - Original Priority: Medium
  - Days Open: ~49 days
  - `server/main.go:524` still has `Secret: []byte("dev-secret-key") // TODO: Use environment variable in production`. The line has shifted from 415 → 524 but the issue is unchanged.
- pprof handler attached to `http.DefaultServeMux` unconditionally
  - Original Priority: Medium
  - Days Open: ~4 days
  - `server/main.go:12` still has `_ "net/http/pprof"` as a top-level blank import.
- No `/readyz` endpoint separate from `/health`
  - Original Priority: Medium
  - Days Open: ~49 days
  - `server/services/health/service.go:147-168` still returns `http.StatusOK` regardless of dependency health.
- No distributed tracing
  - Original Priority: Medium
  - Days Open: ~511 days
  - OTel SDK still has no call sites.
- No error rate tracking by RPC × Connect-error-code (server-side)
  - Original Priority: Medium
  - Days Open: ~511 days
- 9 client services still use raw `Logger` instead of `ObservableLogger`
  - Original Priority: High → Medium (re-rated; the highest-traffic services have all migrated)
  - Days Open: ~49 days
  - Remaining: `app_badge_service`, `auth_state`, `community_event_stream`, `deferred_deep_link_service`, `device_location_service`, `event_router`, `mapbox_location_service`, `request_service`, `user_position_resolver`.
- Raw `print()` as client log sink
  - Original Priority: Low
  - Days Open: ~49 days
  - `app/lib/core/config/environment.dart:103` still terminates the entire pipeline in `print(...)`.
- Client cache stats tracked but never logged
  - Original Priority: Low
  - Days Open: ~49 days
  - `getStats()` defined in `cache_manager.dart:87` and `stash_cache_manager.dart:137` but no callers.
- No deployment event correlation
  - Original Priority: Low
  - Days Open: ~49 days
  - Server emits `"server started"` (`server/main.go:1009`) without `version` or `commit` fields.

**New Issues:**

_None this run._ All remaining items were carried forward from the previous review.

**Improvements:**

- `RemoteAddr` middleware (new infrastructure) enables structured client-IP logging anywhere via `logging.RemoteAddrFromContext(ctx)`. This is the foundation for future rate-limit / abuse-detection alerting.
- `audit_logging_test.go` makes the audit log a tested invariant — future regressions will fail CI.
- Server middleware request logger already includes `rpc_method`, `request_id`, `user_id`, `duration_ms`, `response_bytes`, `remote_addr`, and uses status-aware levels (`server/middleware/logging.go:42-70`).

**Trend**: **Improving**. Five distinct findings closed in a single cycle, including the highest-priority Dockerfile fix that had been open for ~145 days. The remaining gaps are primarily greenfield infrastructure (metrics, tracing) rather than fixable bugs.

---

## Observability Maturity

| Pillar | Status | Coverage | Notes |
|--------|--------|----------|-------|
| Logging (server) | Green | 95% | Structured JSON via `log/slog`, Cloud Logging field names, masked PII, request/user/remote-addr context. Failed-auth audit logging now comprehensive. |
| Logging (client) | Green | 75% | `ObservableLogger` adopted in 7 high-traffic services. 9 lower-traffic services still on raw `Logger`. Sink still terminates in `print()`. |
| Metrics | Red | 5% | Only per-request DB query-count logging. No time-series metrics, no `/metrics` endpoint. |
| Tracing | Red | 5% | Request ID + user ID propagated end-to-end via context; no spans. |
| Health Checks | Yellow | 75% | Deep `/health` RPC; Dockerfile now correctly probes it. Still no `/readyz` so HTTP 200 always returns regardless of dep state. |
| Client Crash Reporting | Green | 90% | Crashlytics with breadcrumbs, PII redaction, consent gating. |
| Client Analytics | Green | 85% | 30+ typed events, route observer. |
| Client Performance | Green | 80% | App-startup + per-screen traces via Firebase Performance. |
| Audit Logging | Green | 85% | Email/OIDC/phone/refresh failure paths logged with masked email, reason, remote_addr, provider. Tested. |
| Alerting | Red | 0% | No alerting wired up — gated on metrics. |

## Critical Issues

### 1. No Server Metrics Collection

- **Location**: Entire `server/` codebase (no `server/metrics/` package; OTel/Prometheus not in use).
- **Description**: Zero Prometheus, OpenTelemetry, or OTLP metrics instrumentation. The only quantitative signal exported is per-request DB query counts in `server/middleware/query_stats.go`. Status codes, latency, and bytes are logged via `server/middleware/logging.go:38-50` but not aggregated as time-series.
- **Impact**: No ability to detect gradual performance degradation, no alertable error rate, no SLI/SLO measurement, no capacity-planning data. Time-series questions ("P95 latency of `CompleteTransfer` over the last week") require log aggregation rather than metric queries.
- **Recommendation**: Add `github.com/prometheus/client_golang`, create `server/metrics/` with a shared registry, instrument `server/middleware/logging.go` with `http_request_duration_seconds{rpc_method,code}` + `http_requests_total{rpc_method,code}`, and expose `/metrics`. Gate `/metrics` behind dev-mode or a shared secret in production. Reuse existing `responseWriter.status` and `extractRPCMethod` to populate labels.

## High Priority

### 2. Health Endpoint Always Returns 200 — No Readiness Signal

- **Location**: `server/services/health/service.go:147-168`
- **Description**: The `/health` GET handler intentionally returns HTTP 200 regardless of dependency state ("Always returns HTTP 200 to indicate the server is reachable"). Container orchestrators (Cloud Run, k8s readiness probes) rely on HTTP status codes to route traffic away from instances with degraded dependencies; with always-200, a pod with a dead database will keep receiving requests. The Dockerfile HEALTHCHECK was upgraded to probe `/health` (good), but the response codes don't actually distinguish "alive but broken" from "alive and ready".
- **Impact**: Load balancers cannot steer traffic away from instances with broken dependencies. Users may receive `CodeInternal` / 500s during a partial outage until the next deploy. The Dockerfile fix is partial: liveness now works, but readiness routing still doesn't.
- **Recommendation**: Add a separate `/readyz` handler that returns 503 when `database` or `storage` dependencies fail their check while keeping `/health` as-is for uptime dashboards. Update the Dockerfile HEALTHCHECK to probe `/readyz` instead. Reuses the existing `Service.dependencies` map.

## Medium Priority

### 3. Hardcoded JWT Secret in Production Path

- **Location**: `server/main.go:524`
- **Description**: `Secret: []byte("dev-secret-key") // TODO: Use environment variable in production` — signs every user's access token. Tokens are valid for 1 hour but in principle any audit logging that uses `user_id` from a token can be forged with this known constant.
- **Impact**: Observability data integrity — `user_id` fields in logs cannot be trusted for forensic purposes. Security: tokens are forgeable in any environment that ships with this default.
- **Recommendation**: Read from `JWT_SIGNING_KEY` env var, fail fast at startup when unset in non-dev mode. ~30-minute change.

### 4. `pprof` Registered on DefaultServeMux Unconditionally at Init

- **Location**: `server/main.go:12` (`_ "net/http/pprof"` blank import); mount gated at `cfg.devMode`.
- **Description**: The blank import registers pprof handlers on `http.DefaultServeMux` at init time. The current code only mounts `DefaultServeMux` in dev mode, so pprof is correctly gated today — but the coupling is fragile: any future handler or server that touches `http.DefaultServeMux` (a metrics port, a debug port) re-exposes pprof without explicit choice.
- **Impact**: Latent footgun. Defense-in-depth gap: production images should not have pprof registered at all.
- **Recommendation**: Move the blank import behind a `//go:build dev_profiling` tag, or register specific pprof handlers (`pprof.Index`, `pprof.Profile`, etc.) explicitly on the app's own mux only when `cfg.devMode` is true. Never touch `DefaultServeMux`.

### 5. No Error Rate Tracking by RPC × Connect Code

- **Location**: Throughout `server/services/`; would be added in `server/middleware/logging.go` plus a new `server/metrics/` package.
- **Description**: Every service uses `connect.NewError(code, err)` with specific error codes. The HTTP middleware logs overall status but doesn't break down by RPC method × Connect error code. There's no counter aggregation for error rates.
- **Impact**: Cannot alert on "error rate of `SaveGear` jumped 5× in last 10 minutes" without log-based metrics. Cannot distinguish "users sending malformed requests" (InvalidArgument) from "server meltdown" (Internal) at dashboard level.
- **Recommendation**: When adding Prometheus (finding #1), add `rpc_errors_total{rpc_method, code}` as a counter. Interim mitigation: include `error_code` as a structured field in error-handling paths so Cloud Logging log-based metrics can aggregate it.

### 6. No Distributed Tracing

- **Location**: Entire server codebase
- **Description**: OTel SDK is installed (as indirect dep) but has no call sites. No span creation around AI calls, database queries, geocoding calls, or email calls. Request correlation works via request_id in logs, but there's no span-graph view.
- **Impact**: Time-in-dependency attribution requires manual log correlation. For slow endpoints (e.g., streaming gen), can't easily attribute slowness to a specific downstream service.
- **Recommendation**: Start with OTel spans around the four external dependency categories (AI, DB via `InstrumentedDB`, geocoding, email). Ship to Google Cloud Trace via the existing Google Cloud exporter.

## Low Priority

### 7. Nine Lower-Traffic Client Services Still Use Raw `Logger`

- **Location**: `app/lib/services/{app_badge_service,auth_state,community_event_stream,deferred_deep_link_service,device_location_service,event_router,mapbox_location_service,request_service,user_position_resolver}.dart`
- **Description**: These services use `Logger('Name')` instead of `ObservableLogger.named('Name')`, so their log lines lack request_id/user_id auto-injection, PII redaction, and breadcrumb recording. The high-traffic services migrated this cycle, leaving the long tail.
- **Impact**: Logs from these services don't correlate with server-side request IDs and don't appear in Crashlytics breadcrumbs.
- **Recommendation**: Mechanical find-and-replace migration; can be a single PR. Re-rated from High to Low because the highest-error-surface services are now done.

### 8. Raw `print()` as Client Log Sink

- **Location**: `app/lib/core/config/environment.dart:101-106`
- **Description**: The client's logging pipeline terminates in a single `print(...)` call: `Logger.root.onRecord.listen((record) { print('${record.level.name}: ...'); });`. Single-line unstructured string; no severity attached to OS log levels; warnings/errors don't auto-route into Crashlytics non-fatal reporting.
- **Impact**: Release-build log stream is low-signal. Searching adb/Console for WARNING vs INFO requires textual parsing.
- **Recommendation**: Route WARNING+ into `Crashlytics.recordError(..., fatal: false)` and INFO/DEBUG into `developer.log` (gives correct severity tags on iOS and Android).

### 9. Client Cache Stats Not Logged

- **Location**: `app/lib/data/cache/cache_manager.dart:87`, `app/lib/data/cache/stash_cache_manager.dart:137`
- **Description**: `getStats()` is defined but never called. Cache hit/miss rates are invisible.
- **Impact**: Cannot tune TTLs or detect cache thrashing.
- **Recommendation**: Periodically log stats via `ObservableLogger` and emit a `cache_hit_rate` Firebase Performance custom metric on foreground transitions.

### 10. No Deployment Event Correlation

- **Location**: `server/main.go:1009` and CI/CD layer
- **Description**: `logger.Info("server started", "address", addr)` lacks `version`, `commit`, `deploy_time` fields. No release annotation in Cloud Logging.
- **Impact**: Regression attribution to a specific deploy is manual.
- **Recommendation**: Inject build version + git SHA via `-ldflags "-X main.version=... -X main.commit=..."` in the Docker build, log them on startup, and set Firebase user property `app_version` on the client.

## Positive Findings

### 1. Comprehensive Audit Logging for Authentication Failures

**Location**: `server/services/login/authentication.go`, `refresh.go`, `phone_auth.go`; tested in `audit_logging_test.go`.

Every auth-failure branch now emits a structured `authentication_failed` log line with `user_email` (masked), `reason`, `remote_addr`, and (for OIDC) `provider`. Coverage spans email login, OIDC, phone auth, and refresh token validation. Test file enforces the invariant. This is one of the cleanest audit-log implementations in the codebase.

### 2. RemoteAddr Middleware Decouples IP Extraction From Handlers

**Location**: `server/middleware/remote_addr.go`, `server/logging/logger.go:236-250`

`X-Forwarded-For` first-hop extraction lives in middleware; handlers retrieve via `logging.RemoteAddrFromContext(ctx)`. Clean separation, easy to unit test, easy to switch IP-resolution policy in one place.

### 3. ObservableLogger Adoption Expanded to 7 Services

**Location**: `app/lib/services/` (auth, community, chat, transfer, media, fcm, community_event_poller)

The migration prioritized highest-error-surface services first. Crash reports and logs from these flows now get request-ID correlation, PII redaction, and breadcrumb support automatically.

### 4. AI Provider Calls Have Consistent Latency Logging

**Location**: `server/ai/provider_anthropic.go` (9+ sites)

Every AI provider call records `duration_ms` in structured form at both success and error paths.

### 5. Per-Request DB Query Stats Middleware

**Location**: `server/middleware/query_stats.go`, `server/storage/instrumented_db.go:53-77`

DB query counts + total DB duration logged at request-end; slow-query detection in instrumented DB; `AssertMaxQueries` test helper prevents N+1 regressions.

### 6. HTTP Middleware Logs With Status-Aware Levels and Full Context

**Location**: `server/middleware/logging.go:25-71`

Logs at ERROR for 5xx, WARN for 4xx, INFO for 2xx/3xx. Includes method, path, status, duration, bytes, remote addr, RPC method, request ID, and user ID. Textbook structured request logging.

### 7. Deep Health Dependency Tracking

**Location**: `server/services/health/service.go`, wired from `server/main.go`

8+ dependencies (database, storage, AI, geocoding, email, stock imagery, stock video, GitHub) each register a `CheckHealth` function with per-probe latency and status logging. Only the HTTP status code signal is missing — see finding #2.

### 8. Cloud Logging Field Name Remapping

**Location**: `server/logging/logger.go:112-134`

`cloudLoggingReplaceAttr` correctly renames `level` → `severity` and `WARN` → `WARNING` so Cloud Logging recognizes severity levels.

### 9. Request ID Propagation End-to-End

**Location**: `server/middleware/request_id.go`, `server/logging/logger.go:192-204`, `app/lib/core/observability/logging/logger.dart`

Request ID generated server-side, propagated to every log line, mirrored on the client via `LogContextHolder`. Minimum viable distributed tracing implemented correctly.

### 10. Client Observability Infrastructure

**Location**: `app/lib/core/observability/`

`ObservabilityService`, `ObservabilityManager`, `ObservabilitySettings`, `BreadcrumbBuffer`, `BreadcrumbNavigatorObserver`, `AnalyticsRouteObserver`, `PerformanceRouteObserver` — well-designed with clean provider interfaces and no-op implementations.

## Recommendations

### Immediate Actions (This Week)

1. **Fix hardcoded JWT secret** — 30 minutes. `server/main.go:524` → env var with fail-fast in non-dev mode.
2. **Harden pprof registration** — 30 minutes. Move blank import behind `//go:build dev_profiling` tag.
3. **Add `/readyz` endpoint** — 1 day. Fails when `database` dependency fails. Update Dockerfile.

### High Priority (Next 2 Weeks)

4. **Implement Prometheus metrics** — 2–3 days. New `server/metrics/` package, HTTP histogram + counter in middleware, `rpc_errors_total{rpc_method,code}`, `/metrics` endpoint (dev-mode only initially).
5. **Migrate remaining 9 client services to `ObservableLogger`** — ~0.5 day. Mechanical find-and-replace.

### Medium Priority (Month 1–2)

6. **Add OTel tracing for external calls** — 2–3 days. AI, DB, geocoding, email. Ship to Google Cloud Trace.
7. **Route client WARN/ERROR logs into Crashlytics non-fatal reports** — 0.5 day. Bridges `environment.dart` sink into `ObservabilityService`.
8. **Add deployment correlation** — 0.5 day. Inject version + commit at build time, log on startup.

### Long Term

9. Add Cloud Logging alert policies on error rate spikes and health check failures (unblocked by finding #5).
10. Build SLI/SLO dashboards on top of Prometheus + Cloud Monitoring (unblocked by finding #1).
11. Emit business metrics (gear_created_total, transfer_completed_total, etc.) once Prometheus is in place.

---

## Issues to File

### [Critical] No server metrics collection blocks alerting and SLI/SLO

- **Fingerprint**: `review:observability:no-server-metrics`
- **Type**: Task
- **Why it matters**: Without Prometheus/OTel metrics we cannot detect gradual performance degradation, set up error rate alerts, measure SLIs/SLOs, or plan capacity. Time-series questions like "P95 latency of `CompleteTransfer` this week" require log aggregation instead of direct metric queries. OTel packages are already present as indirect dependencies; this is greenfield instrumentation, not a refactor. Open ~511 days.
- **Locations**: Entire `server/` codebase; no `server/metrics/` package exists. Natural integration points: `server/middleware/logging.go` (request histogram + counter — `responseWriter.status` and `extractRPCMethod` already give the labels), `server/storage/instrumented_db.go` (DB latency histogram).
- **Recommendation**: Add `github.com/prometheus/client_golang`, create `server/metrics/` with a shared registry, instrument HTTP middleware with `http_request_duration_seconds{rpc_method,code}` + `http_requests_total{rpc_method,code}` + `rpc_errors_total{rpc_method,code}`, and expose `/metrics` (dev-mode or behind a shared secret in prod).

### [High] Health endpoint always returns 200 — no readiness signal for orchestrator routing

- **Fingerprint**: `review:observability:no-readyz-endpoint`
- **Type**: Task
- **Why it matters**: `/health` returns 200 regardless of dependency state, so Cloud Run / k8s cannot steer traffic away from a pod with a dead database or broken AI provider. The Dockerfile fix to probe `/health` (landed this cycle) gave us liveness but not readiness — partial outages still serve traffic until manual rollback. The `Service.dependencies` map already has all the data we need.
- **Locations**: `server/services/health/service.go:147-168`, `server/main.go:959-960`, `Dockerfile:89`
- **Recommendation**: Add `/readyz` handler that returns 503 when `database` or `storage` dependency checks fail. Keep `/health` as-is for uptime dashboards. Update Dockerfile HEALTHCHECK to probe `/readyz`.

### [Medium] Hardcoded `dev-secret-key` JWT signing key undermines audit-log integrity

- **Fingerprint**: `review:observability:hardcoded-jwt-secret`
- **Type**: Bug
- **Why it matters**: Tokens signed with a known constant can be forged with any user_id — every `user_id` field in the server log stream is therefore untrustworthy for forensic/audit purposes. With audit logging now comprehensive (this cycle), this is the gap that prevents trusting that audit log. Also a security finding; the TODO at the call site has been open ~49 days.
- **Locations**: `server/main.go:524`
- **Recommendation**: Read from `JWT_SIGNING_KEY` env var, fail fast at startup when unset in non-dev mode. Coordinate with security review which has likely flagged this independently.

### [Medium] pprof registers on DefaultServeMux unconditionally at init

- **Fingerprint**: `review:observability:pprof-default-mux-footgun`
- **Type**: Task
- **Why it matters**: `_ "net/http/pprof"` (`server/main.go:12`) registers pprof handlers on `http.DefaultServeMux` at package init regardless of dev-mode. Today only dev mode mounts `DefaultServeMux`, so pprof is correctly gated — but the coupling is fragile: any future handler that touches `http.DefaultServeMux` (e.g., a separate metrics port for finding #1 above) re-exposes pprof. Production images should not have pprof registered at all.
- **Locations**: `server/main.go:12`, `server/main.go` (dev-mode mount around line 828)
- **Recommendation**: Move the blank import into a `//go:build dev_profiling` file, or register specific pprof handlers (`pprof.Index`, `pprof.Profile`, etc.) explicitly on the app's own mux only when `cfg.devMode` is true. Never touch `http.DefaultServeMux`.
