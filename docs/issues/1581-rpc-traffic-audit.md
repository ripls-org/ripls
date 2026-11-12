---
issue: 1581
title: Audit routine app usage for RPC traffic
status: planning
---

# Issue #1581: Audit routine app usage for RPC traffic

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1581
**Filed:** 2026-04-29 | **Last activity:** 2026-04-29
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Anecdotally, routine app usage appears to generate a lot more RPC traffic than
expected — repeated media fetches, repeated enumeration calls, possibly
unnecessary AI calls. We do not yet have a captured session that proves where
the chattiness comes from. The goal is to instrument, capture, and analyze a
real session of app + server traffic, identify the worst offenders, and then
file follow-up issues (or fix in this branch) for the highest-impact reductions.

The issue is exploratory: the deliverable is a diagnosis with prioritized
findings, not a predetermined fix. We do not yet know whether the chattiness
is caused by missing/short-lived caches, viewmodel rebuild loops, screen-level
re-fetches, lack of batching, or AI calls firing in unexpected places.

## Context from Comments

The issue has no comments. The author and the requester are the same person
(Bryan), and the agreed approach (clarified in chat) is:

1. Spin up a fresh local server.
2. Drive a sim load to seed realistic content (gear, communities, users,
   experiences, transfers, requests, media).
3. Run the Flutter app on a local emulator/simulator while watching the
   server logs and the Flutter app logs in parallel.
4. Click around the app doing routine things (open feed, browse a community,
   tap an item, RSVP, etc.) and look for suspect patterns: same RPC fired
   multiple times in quick succession, RPCs we expected to be cached but
   weren't, AI RPCs firing on screens they shouldn't.

This is a hands-on audit, not a benchmark. Profiling/load-test infrastructure
already exists (see Phase 6 below) and we will reuse it for content seeding,
but the diagnosis itself comes from human-in-the-loop observation.

## What we already have

The codebase has a meaningful amount of relevant infrastructure already:

- **Server middleware logs every RPC** with `rpc_method`, `duration_ms`,
  `response_bytes`, `request_id`, `user_id`, `db_queries`, `db_duration_ms`
  (`server/middleware/logging.go`, `server/middleware/query_stats.go`,
  `server/middleware/metrics.go`). Default level is `debug` in dev and `info`
  in prod, so server-side debug fields are already on locally.
- **pprof endpoints** are mounted in dev mode (`server/main.go`).
- **Simulation framework** (`scripts/profile.sh`, sim binary under
  `server/cmd/sim/`) generates realistic read/write traffic with personas,
  contextual reads, and concurrency. Scenarios: `load-test-small/medium/large`
  and `read-heavy`.
- **Past profiling rounds** documented in `docs/ai/profile_baseline.md` and
  `docs/ai/profile_optimized.md`; Phase 5/6 of the SQL refactor already
  removed several N+1s. That work was server-internal, not RPC-shape, so it
  doesn't directly answer this issue.
- **Client repositories** (`MediaRepository`, `UserRepository`,
  `GearRepository`, `LoanRepository`, etc.) cache reads via `CacheService` /
  `CacheManager` with a single global `CACHE_TTL_MINUTES`. Cache stats are
  exposed: `totalEntries`, `hits`, `misses`, `hitRate`, `evictions`.
- **Media fetching** uses `CachedMediaImage` with a stable mediaId-derived
  cache key, so the on-disk image cache survives presigned-URL refresh.
- **Client RPC layer** has no logging interceptor today. Calls dispatch from
  service classes wrapping generated Connect clients constructed in
  `app/lib/services/providers/auth_providers.dart`. Errors flow through
  `RpcErrorHandler`.

The single most important gap for this audit is **client-side RPC logging**.
Without it we can correlate server-side logs to the screen the user was on
only by guessing — but the gap is smaller than it looks because the client
already sends an `X-Request-ID` on every call and the server already logs
it as `request_id` (per `docs/server/observability.md`).

## Audit Notes

This plan was reviewed against the project's architecture docs
(`docs/client/architecture.md`, `docs/client/observability.md`,
`docs/server/observability.md`). Findings folded back in:

- **Hook into `executeRpc()`, not a new Connect interceptor.** The codebase
  already routes every RPC through `RpcUtils.executeRpc()` and that wrapper
  already accepts `operationName` and tracks duration. Adding a parallel
  Connect-transport interceptor would split the instrumentation surface in
  two for no benefit.
- **Use `ObservableLogger`, not a fresh log path.** It already does PII
  redaction via `LogRedactor` and propagates `LogContext` (request_id +
  user_id). Per `docs/client/observability.md` §"PII Protection" and
  §"Structured Logging", anything that bypasses it is a regression.
- **Use the `BreadcrumbBuffer` `network` category for RPC events.** The
  observability doc explicitly lists "Network — RPC calls (optional, via
  manual instrumentation)" as a breadcrumb category. Recording RPCs there
  serves the audit and makes the same data available to Crashlytics for
  free.
- **Correlate client and server logs by `request_id`.** The Flutter client
  already generates and sends an `X-Request-ID` header on every call;
  `server/middleware/request_id.go` extracts it and `LoggerWithContext`
  threads it through every server log line. Phase 4 analysis should join
  by this key, not by wall-clock timestamp.
- **No new viewmodel/repository layers required.** Per
  `docs/client/architecture.md` rule 2, repositories already mediate every
  read; instrumenting `CacheService.get()` once captures every cache
  decision the app makes without touching any individual repository. This
  preserves the layer boundaries and keeps the audit out of the
  presentation layer.
- **Server side already meets the audit's needs.** The server's logging
  middleware already records `rpc_method`, `duration_ms`, `response_bytes`,
  `db_queries`, `db_duration_ms`, `request_id`, and `user_id`; running
  with `--log-level=debug --log-format=text` is sufficient for local
  capture. No new server instrumentation is required for this issue.

## Potential Solutions

### Option A: Server-only audit (correlate by timeline)

- **Approach:** Use the existing server middleware logs as the source of
  truth. Tail the server log while clicking through the app, group RPCs
  by `request_id` / `user_id` / time bucket, look for repeated calls.
- **Pros:** Zero new code. Works today.
- **Cons:** Hard to attribute an RPC burst to a specific screen action.
  Won't catch client-side cache thrash directly (e.g., we see 10
  `GetMedia` calls but can't tell if it's one screen rebuilding 10× or
  ten distinct screens).
- **Effort:** Small.

### Option B: Add client-side RPC interceptor + structured log capture (recommended)

- **Approach:** Add a minimal Connect transport interceptor on the Dart
  client that logs `rpc_method`, `duration_ms`, response size (or item
  count), cache-bypass reason, and the originating widget/screen tag
  when available. Server stays as-is. Capture both streams during a
  scripted exploration of the app, then correlate.
- **Pros:** Direct attribution from RPC → screen. Catches "fired N times
  from one rebuild" cleanly. The interceptor is independently useful
  beyond this audit (debug builds, future regressions).
- **Cons:** New code. Need to be careful not to log PII or auth tokens.
- **Effort:** Small–Medium.

### Option C: Full distributed tracing (OpenTelemetry)

- **Approach:** Wire OTel from Flutter through the server, render traces
  in Jaeger or similar.
- **Pros:** Best-in-class long-term observability.
- **Cons:** Massive scope creep relative to what the issue asks for, and
  we'd likely never finish before the audit insights go stale.
- **Effort:** Large.

## Recommendation

**Option B**, scoped tight, hooked into the **existing** observability layer
rather than a new Connect interceptor. The codebase already runs every RPC
through `RpcUtils.executeRpc()`
(`app/lib/core/utils/rpc_utils.dart`), already has an `ObservableLogger` with
PII redaction, already maintains a `BreadcrumbBuffer` with a `network`
category for RPC events (per `docs/client/observability.md`), and already
propagates a client-generated `X-Request-ID` that the server logs use as
`request_id`. The audit instrumentation is therefore a few extra log fields
in `executeRpc()` plus a hit/miss tap inside `CacheService.get()`, all gated
on a debug flag. Decide post-audit whether the instrumentation stays
permanently for debug builds (probably yes).

If even this turns out to take more than half a day, fall back to Option A
(server-only audit, correlated by `request_id`) for this round and file the
client instrumentation as its own follow-up issue.

## Open Questions

- **Should the client interceptor log payload sizes?** — Recommendation: log
  response-byte length (already serialized) but not request bodies. Request
  bodies risk PII; response sizes are the metric we actually care about for
  this audit and are cheap.
- **Where do we tag "originating screen" for an RPC?** — Recommendation:
  reuse the existing `AnalyticsRouteObserver`
  (`app/lib/core/observability/analytics_route_observer.dart`) which
  already fires on every route push/pop/replace and calls `logScreenView`.
  Extend it (or the observability `Service`) to also keep the most recent
  screen name in a single field the RPC interceptor reads at log time.
  Avoids threading a context through every repository call. Falls back to
  `unknown` for RPCs that fire before the first route or from background
  jobs.
- **Do we run the seeded sim before each audit pass, or once per branch?** —
  Recommendation: once per branch; persist with a snapshot/restore on the
  test DB so reruns are cheap and start from identical content. The sim is
  for *content shape*, not for traffic measurement here.
- **Do we audit Android, iOS, or both?** — Recommendation: iOS Simulator
  first (faster log capture via `idevicesyslog`/`flutter logs`), then spot
  check Android for divergence. Most RPC chattiness will be platform-
  independent.
- **Where do findings land?** — Recommendation: a single
  `docs/ai/rpc_audit_findings.md` document with a ranked table
  (RPC, count, duplication factor, suspected cause, proposed fix), plus
  one GitHub issue per finding above a chosen impact threshold.

## Implementation Plan

### Phase 1: Seed a local environment

**Goal:** A local server pointed at a fresh DB, populated with realistic
content the way the sim would produce it, ready for a human to drive the
Flutter app against.

**Tasks:**

- [ ] Stand up a clean local Postgres + run server with `--dev-mode
      --log-level=debug --log-format=text` (the existing flags from
      `docs/server/observability.md`).
- [ ] Run `scripts/profile.sh --scenario load-test-small` (or sim binary
      directly) to seed users, communities, gear, requests, transfers,
      experiences, and media. Cap to a size the human reviewer can navigate
      meaningfully — small is fine.
- [ ] Snapshot the seeded DB so we can restore between audit passes
      (`pg_dump` to a fixture file checked in under `server/testdata/` or
      kept locally).
- [ ] Note the credentials for one or two seeded users we'll log in as.

**Validates:** Local server has enough content that real screens have real
data, and we can rerun the audit deterministically.

### Phase 2: Client RPC logging via `executeRpc` and breadcrumbs

**Goal:** Every RPC the Flutter client makes shows up in the local log
stream with method name, duration, response bytes (or item count for
streaming RPCs), screen tag, `request_id` for server correlation, and
cache-hit/miss attribution if it came through a repository.

**Hook points (use existing infrastructure, do not introduce a new layer):**

- `app/lib/core/utils/rpc_utils.dart` — already wraps every RPC, already
  records `Stopwatch.elapsedMilliseconds`, already accepts an
  `operationName` and `PerformanceMonitor`. This is the right place to
  emit the audit log line.
- `app/lib/core/observability/logging/logger.dart` (`ObservableLogger`) —
  already wired with `LogContext` (request_id, user_id) and
  `LogRedactor`. Use this rather than a new logger.
- `app/lib/core/observability/breadcrumbs.dart` (`BreadcrumbBuffer`) —
  already supports a `network` category for RPC events per the
  observability doc; recording network breadcrumbs benefits Crashlytics
  context as a side effect.
- `app/lib/core/observability/analytics_route_observer.dart` — already
  fires on every route push/pop/replace.

**Tasks:**

- [ ] Extend `executeRpc()` to log a single structured line per call when
      a `kDebugMode`-or-env-flag-gated audit mode is on:
      `rpc_method`, `duration_ms`, `response_bytes` (use the serialized
      response size; for streaming RPCs use `response_items`), `error`
      if any, and the propagated `request_id` (so server logs can be
      joined on the same key).
- [ ] Surface a "current screen" value from `AnalyticsRouteObserver` —
      either a getter on the observer or a tiny `currentScreenProvider`
      that the observer updates on each `_logScreenView` call — and
      include it as `screen` on every RPC log line. Default `unknown`
      when no route has yet been pushed.
- [ ] Record a `network`-category breadcrumb for each RPC via
      `ObservabilityService.addBreadcrumb` so the same data shows up in
      crash reports when an RPC pattern coincides with an error.
- [ ] Instrument `CacheService.get()` (one place,
      `app/lib/data/cache/cache_service.dart`) to log a `cache_hit` or
      `cache_miss` event with the namespaced key (already namespaced
      per repository) — also as a breadcrumb. This avoids touching
      every repository.
- [ ] Gate all of the above behind a single debug flag so production
      builds and the analytics provider see no change. PII redaction
      already happens inside `ObservableLogger` via `LogRedactor`; rely
      on it rather than reimplementing.
- [ ] Verify the logs surface in `flutter run` console output and in any
      attached IDE log view.

**Validates:** Tailing the Flutter logs during a single screen tap shows
exactly which RPCs that tap caused, attributed to the screen the tap
happened on, joinable to server logs via `request_id`.

### Phase 3: Joint capture session (the audit itself)

**Goal:** A reproducible recording of "user does routine thing X" → "RPCs
fired" for a short, scripted exploration of the app.

**Tasks:**

- [ ] Define a script of "routine things" to exercise (open feed, scroll,
      open community, tap gear, open chat, etc.). One paragraph in the
      findings doc; not exhaustive.
- [ ] Run the local server with `--log-level=debug --log-format=text`
      streaming to a file (`server.log`). The text format makes
      eyeballing easier; if a script needs JSON, capture both.
- [ ] Run the app on iOS Simulator with logs streaming to a file
      (`flutter.log`).
- [ ] Walk the script. Mark timestamps in a notes file when starting each
      step ("T+12s: tapped first feed item").
- [ ] Stop capture, save both log files into
      `docs/ai/rpc_audit/session-<date>.zip` (or just keep them locally if
      the size is too large to commit).

**Validates:** We have a single captured session both sides of which we can
analyze offline.

### Phase 4: Analysis & findings

**Goal:** A ranked list of suspect patterns with enough detail that each
becomes a tractable follow-up.

**Tasks:**

- [ ] Parse server `rpc_method` log into per-method counts and total bytes
      across the session (small awk/jq script; don't over-engineer).
- [ ] Join client and server log lines on `request_id` so each server-side
      RPC is attributed to the screen the client was on when it fired.
      This is the primary correlation key — wall-clock timestamps are a
      fallback only.
- [ ] Cross-reference Flutter cache_hit/cache_miss logs to spot RPCs that
      fired despite a fresh repository cache.
- [ ] Identify the top N (probably 5–10) suspect patterns. Examples to
      look for: same `GetMedia` ID multiple times, `GetUser` for the same
      user from multiple screens within seconds, AI/`StreamGen*` firing on
      a screen we didn't expect, list endpoints called with overlapping
      pagination, viewmodel rebuilds re-issuing RPCs.
- [ ] Write `docs/ai/rpc_audit_findings.md` — ranked table + paragraphs.
- [ ] For each high-impact finding, file a follow-up GitHub issue with
      a link back to this plan and to the captured session.

**Validates:** We've answered the issue's question — "where is the
chattiness?" — with concrete, prioritized findings.

### Phase 5 (optional): Quick wins in this branch

**Goal:** If a finding is small enough to fix in <1 hour and clearly safe,
land it on `rpc_audit` so the audit produces immediate value, not just
follow-up issues.

**Tasks:**

- [ ] Scope: only changes that don't require schema or proto edits.
- [ ] Each fix gets its own commit with the finding name in the message.
- [ ] Anything bigger stays as a filed issue.

**Validates:** The audit ships value, not just paperwork.

## Related Issues

- #1064 — Request cache stale (related cache-correctness flavor of this
  problem).
- #1280 — ListConversations N+1 (server-side; same family of fix on the
  server, but this issue is about client behavior).
- #1281 — Leaderboard N+1.
- #1283 — ListEvents N+1.
- #1520 — Server metrics (longer-term observability home for RPC counts).
- Phase 6 of `docs/ai/sql_improvement_plan.md` — most recent server-side
  N+1 work; sets the baseline this audit should not duplicate.
