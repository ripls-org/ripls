# Issue #2157: Serve the whole Ripls app on web at ripls.app / dev.ripls.app

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2157
**Filed:** 2026-05-25 | **Last activity:** 2026-05-25
**Labels:** none | **Assignees:** unassigned
**Context docs:** none provided; this plan reads from `app/web/README.md`, `docs/issues/2050-web-rsvp-actions.md`, the live `app/lib/core/router/app_router.dart`, and `terraform/environments/{dev,prod}/main.tf`.

## Audit Notes (2026-05-25)

This plan was audited against `docs/client/architecture.md`,
`docs/client/caching.md`, `docs/client/i18n.md`,
`docs/client/accessibility.md`, `docs/client/testing.md`,
`docs/server/architecture.md`, and `docs/server/observability.md`.
Adjustments made during audit:

- **Phase 1 `WebMapPlaceholder`**: expanded with explicit l10n
  (parity across `app_en.arb` + `app_es.arb`), theme-aware
  `AppColors.*` usage, and an `a11y*`-prefixed `Semantics(label:)`
  pulled from ARB — the `check_a11y.js` allowlist is empty and
  rejects raw strings on `semanticLabel:`.
- **Phase 1 catch-all server handler**: added an explicit middleware
  preservation note (request_id + structured logging via
  `logging.LoggerWithContext`) — without it the new `/` route bypasses
  the standard logging conventions in `docs/server/observability.md`.
- **Phase 1 splash widget**: corrected the "two-line fix" framing —
  the redirect function returns a navigation path, not a widget, so
  the splash render belongs at the `MaterialApp.router` boundary.
  Two concrete options spelled out.
- **Phase 2 `WebUnsupported`**: same l10n/colors/a11y conventions
  added. CI test note distinguishes web-smoke (headless Chrome) from
  the `ProviderContainer` + Mockito ViewModel tests in
  `docs/client/testing.md`. Login flow noted as still using the
  standard MVVM + Stash repository / cache layers — no web-specific
  ViewModels.
- **Phase 3 observability**: rewrote the vague "structured-log
  scrape" task. Server side is already covered by the existing
  logging middleware + alert policies; the actual gap is *client-side*
  web errors with no Crashlytics sink. Decision deferred (accept gap
  vs. internal RPC).
- **Phase 5 SSR shells**: added explicit observability + error
  handling conventions for new server handlers
  (`logging.LoggerWithContext`, `http.Error` since these are HTTP not
  Connect, file-length targets, service independence).

No changes needed to the caching architecture — the web bundle uses
the same `StashCacheManager` + repository pattern as mobile, and the
platform-conditional video cache already has the `kIsWeb` split.

## Problem Statement

#2050 shipped a guest-only Flutter Web surface scoped to a single
route, `/app/event/{id}`. The bundle pipeline, embedded handler,
Connect API surface, and `<base href="/app/">` routing are all in
place. What's not yet in place is the operational lift: a returning
user can't open `https://ripls.app` (or `dev.ripls.app`), log in, and
do everything they can do on mobile.

Two things need clarifying from the issue body vs. the maintainer
comment:

1. **The "open the router" task is mostly conceptual.** The maintainer
   confirmed that "the whole app already is accessible, by following an
   RSVP link, then using the web app UI to navigate around." The router
   does not enforce a per-route allowlist — `app_router.dart` registers
   every route unconditionally; the only web-specific gate is the
   `/event/:experienceId` web entry point. The real blockers are the
   bare-domain landing, broken plugins, and missing observability —
   *not* a routing restriction.

2. **The root-domain decision is already made.** The issue's Phase 3
   ("decide on root-domain strategy") asked whether the app lives at
   `ripls.app` or stays at `ripls.app/app`. The maintainer comment
   resolves this: `ripls.app` and `dev.ripls.app` both serve 404 today,
   the marketing site is at `ripls.org`, and so the `.app` apex URLs
   are free to host the app. No marketing-site relocation needed.

The remaining contradictions / scope adjustments from the maintainer
comment:

- **Maps:** defer #2156 by rendering a "Maps not yet available on Ripls
  web" placeholder rather than blocking on the full plugin migration.
- **Videos:** #2155 is already closed (the maintainer confirms videos
  play on web); no work needed in this issue.
- **CDN:** preference for a Google Cloud–native solution that
  Terraform can configure and deploy; serving from the Go binary at
  the start is acceptable.
- **Analytics:** maintainer wants a walkthrough before wiring; treat
  this as a discussion artifact, not an immediate implementation task.

## Context from Comments

The issue has one comment, from the maintainer (maintainer), that
shapes the scope above. Key directives:

- **Universal-link safety is non-negotiable.** `/go/*` must keep
  routing iOS / Android visitors to the native app via
  `apple-app-site-association` (`server/services/web/app_assets/...`)
  and `assetlinks.json`. The current `applinks.details.paths` list is
  `["/go/*", "/invite", "/invite/*", "/reset-password",
  "/reset-password/*"]` — adding `/*` would steal every link from the
  installed app. Don't.
- **The marketing-site question is resolved.** Marketing stays on
  `ripls.org`; the `.app` apex serves the app.
- **Defer maps; don't gate.** A `kIsWeb` placeholder is the right
  shape, not a real migration.
- **Open question on CDN.** Maintainer wants options for Google
  Cloud–native automation, with serving-from-binary acceptable as a
  starting point.
- **Open question on analytics.** Maintainer wants a walkthrough
  before any wiring lands.

## Potential Solutions

### Option A: Single bundle served from the Go binary at the bare domain

- **Approach:** Drop the `/app/` prefix. Build the bundle with
  `--base-href=/`, mount the `ServeApp` handler at `/` as a fallback
  for any path the Go server doesn't otherwise own (`/go/`, `/api/`,
  `/.well-known/`, `/css/`, `/assets/`, the Connect RPC paths, etc.).
  Keep the Cloud Run domain mapping (`ripls.app`, `dev.ripls.app`) as
  is. Add a small Mapbox-web stub. Wire the plugin audit conclusions
  into `kIsWeb` guards. Land the analytics + CDN cutover as later
  phases.
- **Pros:** Minimal infra change. One handler, one origin, one
  Terraform diff. Existing `apple-app-site-association` continues to
  serve from `/.well-known/` so universal links keep working. CDN can
  be slotted in later without changing the routing.
- **Cons:** Every static asset request hits Cloud Run cold-start
  latency until CDN lands. The base-href flip cascades through the
  Flutter router (paths drop their `/app/` prefix) — needs a sweep of
  `context.go` callers (already correct: they pass post-strip paths
  today) plus an update to the SSR template's `<a href="/app/event/...">`.
- **Effort:** Medium. Most of the work is the plugin audit and the
  base-href flip; the bare-domain routing itself is two lines in
  `server/main.go`.

### Option B: Keep the `/app/` prefix; redirect `/` → `/app/`

- **Approach:** Mount a `/` redirect that 302s to `/app/`. Bundle and
  base-href stay as they are. Everything else (plugin audit,
  observability, CDN) proceeds identically to Option A.
- **Pros:** Smallest possible change. The bundle, the routing, the
  base-href, and the SSR templates all keep their existing shape.
- **Cons:** Two-hop landing for every cold visit; analytics double-
  counts the redirect; the user-visible URL is uglier
  (`ripls.app/app/feed` vs `ripls.app/feed`); deep links shared from a
  desktop browser carry the `/app/` segment. If we ever do want to
  drop the prefix, we'll have to do this work eventually.
- **Effort:** Small. But it's a stall, not a solution.

### Option C: Split bundle behind Cloud CDN + GCS up front

- **Approach:** Build the bundle, push hashed assets to a `ripls-web-{env}`
  GCS bucket fronted by Cloud CDN, leave only `index.html` + the
  Connect RPCs on Cloud Run. Land this in Phase 1 alongside the
  bare-domain routing.
- **Pros:** Right end state. Pays for itself the moment the bundle
  is large enough to be CDN-worthy. Decouples web bundle deploys from
  server deploys (push to GCS without a Cloud Run revision).
- **Cons:** Big Terraform diff (bucket, CDN, load balancer, IAM,
  build-script push step) bundled into the first ship. The maintainer
  comment explicitly said "may not be too bad to serve from go binary
  at the beginning?" — pushing CDN into Phase 1 defies that signal.
- **Effort:** Large. CDN is properly a separate phase.

## Recommendation

**Option A, phased.** Drop the `/app/` prefix in Phase 1 so the
bare-domain URL is the public surface from the start, serve from the
Go binary, and slot the CDN into Phase 4 once we know the asset
footprint and have a baseline to measure latency against. Option B's
two-hop URL is technical debt the moment it lands; Option C front-loads
infra work that the maintainer specifically asked us not to do first.

Tradeoffs being made:

- **Cloud Run cold-start latency on first asset fetch.** Acceptable
  for an internal-dogfood Phase 1; the bundle gate (Phase 3) will
  surface if it becomes a problem before CDN cutover.
- **Base-href flip is irreversible in spirit.** Once we ship URLs
  like `ripls.app/feed`, going back to `ripls.app/app/feed` would
  invalidate shared links. The decision should be made deliberately
  in Phase 1 and not reopened.
- **Map placeholder is a UX regression vs. mobile.** The maintainer
  explicitly accepted this; the placeholder copy goes into ARB strings
  so it's localized from day one.

## Open Questions

- **Should the base href flip happen in Phase 1, or should we keep
  `/app/` and revisit in a follow-up?** — Recommendation: flip in
  Phase 1. The maintainer's "ripls.app already 404s" signal means the
  `.app` apex is free to host the app at `/` directly. Living with
  `/app/` for any window means cementing it in shared links and SEO,
  which we'd have to migrate away from later. Flip once, flip now.

- **Mobile-app interception for `ripls.app/*` paths.** Today
  `apple-app-site-association` claims only `/go/*`, `/invite*`, and
  `/reset-password*` for the native app. If a user with the iOS app
  installed taps `https://ripls.app/feed` in Messages, Universal Links
  should *not* intercept (so the browser opens the web app). Verify
  this is the default behavior with the current AASA. — Recommendation:
  keep AASA paths as they are. The current allowlist is correct: only
  invitation / password-reset paths should leave the browser. Add a
  test or doc note to `app/web/README.md` so a future expansion of
  AASA doesn't accidentally steal `/feed`.

- **Auth redirect from a web cold-start.** The current `redirect`
  function in `app_router.dart` (lines 138-144) bounces unauthenticated
  users to `/login` while `authState.isLoading` is true, which on web
  produces a brief login-screen flash before the stored Firebase token
  resolves. — Recommendation: render a `SplashScreen` (logo + spinner)
  while `authState.isLoading` is true. The redirect function returns a
  navigation path, not a widget, so the splash render belongs at the
  `MaterialApp.router` boundary (a Consumer-wrapped `builder:` returning
  splash vs. router subtree). Phase 1 task spells out two implementation
  options; the first is preferred. Today's behavior works on mobile
  because the loading window is sub-frame; on web it's user-visible.

- **CDN topology.** Cloud CDN + GCS backend bucket is the Google-
  Cloud-native answer that Terraform can drive. The alternative is
  Cloudflare in front of Cloud Run, which is simpler but adds a non-GCP
  dependency. — Recommendation: Cloud CDN + GCS, in Phase 4. Match the
  pattern already in `terraform/modules/storage/` for media assets so
  the diff is small and reviewable.

- **Crashlytics web is in beta.** Wiring it in Phase 5 vs. waiting
  for GA? — Recommendation: wire Firebase Analytics in Phase 5; keep
  Crashlytics gated on `!kIsWeb` until the SDK leaves beta. Web errors
  flow through `_log.severe` already; analytics is the higher-value
  add.

- **Bundle-size budget shape.** The issue suggests "per-route bundle
  limits, not a single initial-chunk number." But Flutter's deferred-
  route compiled chunks are content-hashed and their names change
  across builds; tying CI to a specific chunk name is brittle. —
  Recommendation: keep the initial-chunk gate as the primary budget,
  but add a *total bundle-size* gate as a secondary check so deferred
  routes can't grow without limit. Per-route budgets stay informational
  until the deferred-route config stabilizes (#2050 Phase 2 followup).

- **Analytics walkthrough format.** The maintainer asked to be walked
  through analytics setup. — Recommendation: produce a short
  `docs/web/analytics.md` in Phase 5 (one screen of prose covering: the
  Firebase Analytics web SDK, what events we emit, consent flow on
  web, dashboards) and review it interactively with the maintainer
  before any wiring lands.

## Implementation Plan

### Phase 1: Bare-domain landing + maps stub
**Goal:** A logged-in user can open `https://dev.ripls.app`,
authenticate, and reach every primary screen without a crash. Maps
render a placeholder.

**Tasks:**
- [ ] Drop the `/app/` prefix: rebuild with `--base-href=/`, update
  `app/web/README.md`, update the `build:web*` npm scripts.
- [ ] In `server/main.go`, replace `mux.Handle("/app/", ...)` with a
  catch-all `mux.Handle("/", webService.ServeApp())` registered last
  so the existing prefix routes (`/.well-known/`, `/go/`, `/css/`,
  `/assets/`, `/media/`, the Connect API paths) take precedence. Verify
  `ServeApp`'s path-trimming logic still works with `/` instead of
  `/app/`. **Middleware chain must be preserved**: wrap the catch-all
  with the same `hostMiddleware` + request-ID + logging middleware that
  every other handler gets, so every web bundle / SPA-fallback request
  shows up in Cloud Logging with a `request_id` and standard fields per
  `docs/server/observability.md`. Any new error paths in `app_handler.go`
  use `logging.LoggerWithContext(ctx)`, not `log.Printf` or `fmt.Printf`.
- [ ] Update `WebExperienceScreen` callers and the SSR template's
  `<a href="/app/event/...">` to drop the `/app/` segment.
- [ ] Add a `WebMapPlaceholder` widget at
  `app/lib/presentation/widgets/map/web_map_placeholder.dart`.
  Stateless `ConsumerWidget` — no ViewModel needed (no state).
  Conventions to follow:
  - **Colors:** `AppColors.surface(context)` background,
    `AppColors.textSecondary(context)` for the icon + caption.
    No raw `Colors.grey[N]` / `Color(0xFF...)`.
  - **Strings:** add `webMapsPlaceholderTitle` and
    `webMapsPlaceholderBody` to **both** `app_en.arb` and `app_es.arb`
    (Spanish can be the English string for now — the parity gate
    enforces presence, not translation quality). Read via
    `context.l10n.webMapsPlaceholderTitle`. Run `npm run generate`
    to refresh `AppLocalizations`.
  - **Accessibility:** wrap the placeholder icon in `Semantics(label:
    context.l10n.a11yMapsNotAvailable)` and add the `a11yMapsNotAvailable`
    key (also to both ARB files). Do not use raw `GestureDetector` /
    `IconButton` — the placeholder is non-interactive, so no
    `Tappable` / `IconAction` is needed.
  Use under `kIsWeb` guards in every `MapWidget` call site listed in
  #2156: `discover_screen.dart`, `leaderboard_screen.dart`,
  `location_modal_widgets.dart`, `ripple_map_marker.dart`, and the two
  helpers in `core/utils/`.
- [ ] Replace the loading-state bounce-to-`/login` in
  `app_router.dart` redirect (lines 138-144) with a splash render
  while `authState.isLoading` is true. The redirect function is for
  *navigation*, not rendering — it can't return a widget. Two options
  (pick the smaller in implementation):
  - **Option A (preferred):** wrap `MaterialApp.router`'s `builder:`
    in a Consumer that returns a `SplashScreen` while
    `authState.isLoading`, and the router subtree otherwise. The
    redirect then short-circuits to `null` while loading instead of
    bouncing to `/login`.
  - **Option B:** add a `/splash` GoRoute and redirect there during
    `isLoading`; `SplashScreen.initState` polls `authStateProvider`
    until `!isLoading`, then `context.go(state.uri.toString())` to
    resume. More routing churn; only consider if Option A clashes
    with an existing top-level consumer.
  Widget conventions: `AppColors.background(context)` background,
  `CircularProgressIndicator` themed via existing app theme, no
  raw strings (or no strings at all — a logo + spinner is acceptable
  for a sub-second splash).
- [ ] Strengthen the AASA test in `server/services/web/service_test.go`
  (currently asserts only "contains `applinks`"): parse the served
  `/.well-known/apple-app-site-association` JSON and assert
  `applinks.details[0].paths` equals the exact list
  `["/go/*", "/invite", "/invite/*", "/reset-password", "/reset-password/*"]`.
  Same shape for `assetlinks.json` (`package_name` + paths). This is
  the gate that keeps installed-app users out of the web bundle when
  they tap a non-`/go/` URL; widening the path list silently is the
  failure mode worth pinning down with a test.
- [ ] Verify CORS allowed origins (`terraform/environments/{dev,prod}/main.tf`):
  `https://dev.ripls.app` and `https://ripls.app` are already in the
  list — sanity check post-deploy.
- [ ] Manual smoke: log in via web at `dev.ripls.app`, walk through
  feed → community → gear detail → profile → chat. File issues for
  any screen that crashes.

**Validates:** The bare-domain landing works end-to-end. Maps no
longer block dogfood. Universal Link interception is unchanged.

### Phase 2: Plugin audit and web-unsupported guards
**Goal:** No `MissingPluginException` or `Platform.is*` runtime crash
on any reachable web route.

**Tasks:**
- [ ] Audit each plugin called out in the issue body (`image_picker`,
  `camera`, `geolocator`, `flutter_local_notifications`,
  `firebase_messaging`, `flutter_secure_storage`, `gal`, `share_plus`,
  `add_2_calendar`, `audio_session`, `app_badge_plus`). For each: web
  support? `kIsWeb` stub? user-facing "unsupported on web" message?
- [ ] Sweep the 39 existing `Platform.is*` / `dart:io` call sites for
  reachability from web routes. The `inline_conversation_attachments.dart`
  `dart:io` import is already documented as unreachable
  (`app/web/README.md`); confirm the same for the rest.
- [ ] Add a localized `WebUnsupported` widget at
  `app/lib/presentation/widgets/web/web_unsupported.dart` for features
  (camera, calendar export, push) that render an in-app notice
  instead of crashing. Same conventions as `WebMapPlaceholder`:
  - **ARB:** one localized message per feature
    (`webUnsupportedCamera`, `webUnsupportedCalendar`,
    `webUnsupportedPush`) added to *both* `app_en.arb` and
    `app_es.arb` — the `lint:dart:l10n-parity` gate fails the build
    if either side is missing.
  - **Colors:** `AppColors.surface(context)` / `AppColors.textSecondary(context)`.
  - **A11y:** if the widget contains an action (e.g., a "Download
    the app" link out), use `Tappable` with a `semanticsLabel`
    sourced from a new `a11y*` ARB key, never a raw string literal —
    `scripts/check_a11y.js` rejects raw strings on `semanticLabel:` /
    `tooltip:` / `Semantics(label:)` arguments. If the widget is
    pure-static text, no `Tappable` / `IconAction` is needed.
- [ ] Web Smoke CI: add an authenticated-route test that hits at
  least feed, community detail, and gear detail — not just the
  unauthenticated event landing. **Note on test convention**: this is
  a headless-Chrome end-to-end test under
  `.github/workflows/test_flutter_web.yaml`, not the
  `ProviderContainer` + Mockito ViewModel test pattern from
  `docs/client/testing.md`. ViewModel-level coverage for the screens
  exercised here already exists under `app/test/presentation/viewmodels/`;
  this CI test verifies they boot end-to-end on the web target.
- [ ] Verify the auth cookie path: `EstablishWebSession` is already
  in place (#2050 Compliance L). Confirm a successful web login lands
  a session cookie that survives a page reload. Login flows on web
  continue to use the standard MVVM path — `LoginNotifier` /
  `RegisterNotifier` → `AuthService` (cookie set server-side) →
  `AuthStateNotifier` — with no web-specific ViewModels. Repository /
  cache layer (Stash) is unchanged.

**Validates:** A returning user can navigate the full primary nav
without runtime errors. CI catches regressions on the authenticated
paths.

### Phase 3: Bundle budget + observability foundations
**Goal:** Bundle growth is enforced in CI. Web errors and page views
are visible to operators (analytics wiring lands in Phase 5; this
phase is the foundation for it).

**Tasks:**
- [ ] Set a real baseline in `app/web/bundle_baseline.txt` (replace
  `0`). Take the measurement from a clean `npm run build:web:prod`.
- [ ] Add a total-bundle-size secondary check to
  `scripts/check_web_bundle_size.sh` so deferred routes can't grow
  unbounded. Per-route budgets stay informational.
- [ ] Audit current web-error sinks and decide what (if anything) to
  add. **Server side is already covered**: the standard logging
  middleware (`server/middleware/request_id.go` +
  `server/middleware/logging.go`) tags every HTTP request and Connect
  RPC with a `request_id`, masks sensitive fields per
  `docs/server/observability.md`, and feeds the existing
  `rpc_errors`, `all_errors`, and `Server Error Logged` alert
  policies. The catch-all `/` handler added in Phase 1 inherits this
  middleware (see Phase 1 task on preserving the chain). **Client
  side is the gap**: on web, `FlutterError.onError` and
  `PlatformDispatcher.instance.onError` (`app/lib/main.dart` lines
  166–193) route to `_log.severe`, which only writes to
  `console.log` in the browser — Crashlytics is gated on `!kIsWeb`.
  Until Crashlytics-web ships GA (Phase 5 keeps it gated), there is
  no server-side sink for client-side web errors. Decide in Phase 3:
  (a) accept the gap until Crashlytics-web GA's, or (b) add a tiny
  `/internal/client-error` Connect RPC that the web zone error
  handler POSTs to, which the server logs at `ERROR` so the existing
  alert policy catches a spike. Recommendation: (a) — the volume
  doesn't justify the RPC plumbing yet.
- [ ] Surface the new catch-all `/` request volume in the existing
  Cloud Run / Cloud Logging dashboards. The standard HTTP metrics
  (`http_requests_total`, etc.) already partition by path; this is a
  dashboard query change, not a code change.

**Validates:** Bundle regressions fail CI. Server-side observability
covers every web request; the client-side gap is explicit and
tracked.

### Phase 4: CDN cutover
**Goal:** Hashed static assets served from Cloud CDN, not Cloud Run.

**Tasks:**
- [ ] Add `terraform/modules/web_cdn/` mirroring the existing
  `terraform/modules/storage/` shape: GCS bucket
  (`ripls-web-{env}`), Cloud CDN backend bucket, HTTPS load balancer,
  TLS cert.
- [ ] Add a `build:web:push` step to the npm scripts: `gsutil rsync`
  the `app/build/web/` output into the bucket with the right
  Cache-Control headers (immutable for hashed assets, no-store for
  `index.html`).
- [ ] Cut over the Cloud Run domain mapping so the LB sits in front:
  `ripls.app` → LB → (CDN for hashed assets, Cloud Run for `index.html`
  + Connect API).
- [ ] Verify universal-link `.well-known/` paths still serve from
  Cloud Run (the LB must route `/.well-known/*` to the backend
  service, not the CDN bucket).
- [ ] Drop the `ServeApp` Go embed (`server/services/web/app_embed.go`
  + `app_handler.go`) once the LB is the source of truth. Keep the
  `placeholder.html` path for local dev.

**Validates:** Static-asset latency drops; Cloud Run cost per
deploy decouples from bundle size. Universal Links unchanged.

### Phase 5: Analytics + Crashlytics + SEO shells
**Goal:** Web sessions show up in dashboards; deep-app routes have
shareable OG previews.

**Tasks:**
- [ ] Write `docs/web/analytics.md` covering the Firebase Analytics
  web SDK, the events we emit, consent UX on web, and dashboards.
  Review with the maintainer before any wiring lands.
- [ ] Wire Firebase Analytics web. Verify the existing
  `AnalyticsRouteObserver` works under `kIsWeb`; add a `kIsWeb`-only
  init path in `observability_service.dart`.
- [ ] Wait on Crashlytics-web until GA. Document the gap.
- [ ] Add SSR OG shells for `/community/{id}` and `/gear/{id}`,
  mirroring `server/services/web/event_page.go`. Each renders a tiny
  HTML document with OG / Twitter meta tags, then bootstraps the
  Flutter Web bundle so the rendered page picks up. Conventions to
  follow (from `docs/server/architecture.md` +
  `docs/server/observability.md`):
  - **Service independence:** the handler lives in
    `server/services/web/community_page.go` /
    `gear_page.go`, depends on `storage.ProtoSQLStorage` (and the
    bucket for OG image presigning), and does **not** import other
    services.
  - **Logging:** use `logging.LoggerWithContext(ctx)` for every log
    line; include `operation` plus the entity ID
    (`community_id` / `gear_id`).
  - **Errors:** these are HTTP handlers (not Connect RPCs), so they
    `http.Error(w, ...)` directly rather than using `connecterr.Internal`.
    Log the underlying error at `ERROR` level with
    `logging.LoggerWithContext` before writing the response, so the
    existing `Server Error Logged` alert policy sees it. Don't leak
    raw storage error strings into the response body.
  - **Caching headers:** match what `event_page.go` does (currently
    no-store on the SSR shell so previews don't get cached across
    edits).
  - **File length:** keep each new `*_page.go` under 500 lines
    (target) / 1000 (max). The `event_page.go` shape will scale to
    two more handlers without splitting.
- [ ] Add a Web Smoke test that asserts the OG tags are present for
  a sample community and gear ID. Mirror the pattern in
  `server/services/web/event_page_test.go` — table-driven, table
  entries cover happy path + missing entity + soft-deleted entity.

**Validates:** Acceptance criterion ("Web Smoke CI workflow covers at
least one authenticated route end-to-end") plus link previews work
when web URLs are shared in Slack / Messages / etc.

## Related Issues

- #2050 — shipped the web bundle pipeline this builds on; closed
  scope plus the deferred work that this issue addresses.
- #2155 — web video playback; **closed**, no work needed here.
- #2156 — web maps; this plan ships a placeholder to defer rather
  than block. #2156 stays open for the eventual real fix.
- #1195 — original web-guest-RSVP epic; this issue extends the same
  surface to authenticated users.
- #2058 — typed-shortcode URL-query-param cleanup; touches the same
  redirect logic in `app_router.dart` that Phase 1 modifies. Coordinate
  the splash-widget change so neither PR steps on the other.
- #2119 — `--wasm` build mode; orthogonal but lives in the same
  bundle-config space as Phase 3's budget work.
