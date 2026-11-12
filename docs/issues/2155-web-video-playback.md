# Issue #2155: Web app: videos don't play

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2155
**Filed:** 2026-05-24 | **Last activity:** 2026-05-24
**Labels:** none | **Assignees:** unassigned
**Context docs:** none
**Audited:** 2026-05-25 against `docs/client/{architecture,caching,i18n,
accessibility,testing}.md` and `docs/server/{architecture,observability}.md`.
Audit changes are summarized at the end of the document.

**Status (2026-05-25):** Phase 1 shipped in commit `c27b09adf` and
manually verified in a real browser. Phase 2 (GCS CORS hardening) was
deferred because playback works without it — no symptom to chase.
Phase 3 (automated CI regression coverage) is **out of scope for this
issue** and moved to **#2162** ("Flutter Web bundle as an E2E UI test
harness"). The auth-injection plumbing Phase 3 requires is bigger
than this bug and belongs in a reusable harness, not a one-off in
`server/cmd/web-smoke`. Phase 4 remains deferred until the full app
reaches web.

## Problem Statement

The Flutter Web bundle shipped by #2050 (guest event RSVP at
`/app/event/{id}`) reuses the mobile playback widgets, but those widgets
cannot run on web. Today, on any web surface that contains a video — the
event hero, the Plan, the chat preview — the user sees a blank box,
a frozen poster, or a snackbar that "the video failed to load."

The root cause is structural, not configuration. The shared helper that
every playback widget calls — `createCachedVideoController` in
`app/lib/core/utils/video_cache_helper.dart` — does two web-hostile
things:

1. It imports `dart:io` unconditionally. That import is illegal on web
   and means the file does not even compile for the web target. The
   Flutter Web build currently works only because tree-shaking happens
   to elide the body when no widget reaches it at runtime; the moment a
   widget actually tries to load a video controller, the build fails or
   the throw is swallowed by `VideoBackgroundHost`'s catch block (which
   was downgraded to `severe` + snackbar in #2073).
2. It runs every remote URL through `flutter_cache_manager` →
   `HttpFileService` → `VideoPlayerController.file(File(...))`. Both
   sides of that chain are mobile-only — `flutter_cache_manager`'s
   `HttpFileService` uses `dart:io`, and `VideoPlayerController.file`
   requires a filesystem. On web the correct constructor is
   `VideoPlayerController.networkUrl(...)`, which renders into an
   HTML5 `<video>` element via `video_player_web`.

The CORS, autoplay-policy, and codec questions raised in the issue body
are downstream of (1)+(2): until the controller is actually created
through the web-compatible code path, none of those concerns get
exercised. (CORS is the second-most-likely failure mode once the first
is fixed — see "Open Questions" for why.)

## Context from Comments

No comments on the issue — the description is recent and clear. The
"#XXXX" placeholder in the description ("the moment #XXXX 'serve the
whole Ripls app on web at ripls.app' lands") is unfilled; there is no
parent issue to cross-link yet, but the broader web-rollout effort is
tracked in #1195.

## Root Cause

`app/lib/core/utils/video_cache_helper.dart:1` imports `dart:io` at
top level. Lines 29–34 branch into two paths:

```dart
if (isLocalPath(url)) {
  controller = VideoPlayerController.file(File(url));
} else {
  final file = await repo.getVideoFile(mediaId, url);
  controller = VideoPlayerController.file(file);
}
```

Both branches end at `VideoPlayerController.file(...)`. Both require
`dart:io`. Neither has a web implementation.

The "remote" branch additionally fans out through
`MediaRepository.getVideoFile` (`app/lib/data/repositories/media_repository.dart:171`)
→ `VideoCacheManager.getFile` (`app/lib/data/cache/video_cache_manager.dart:48`)
→ `flutter_cache_manager.downloadFile`, which itself uses `dart:io`
via `HttpFileService`. So even a hypothetical web-aware
`VideoPlayerController.file` would have nothing valid to receive.

Direct callers of `createCachedVideoController` / `VideoPlayerController`
(11 files under `app/lib/presentation/`) all share this single helper,
so a fix in `video_cache_helper.dart` propagates to every playback
surface (event hero, carousel, conversation preview, story view, etc.)
without touching the widgets themselves.

The GCS bucket CORS policy
(`terraform/modules/storage/main.tf:170-176`) is permissive enough for
the URLs the bundle is loaded under:

```
cors {
  origin          = var.media_cors_origins   # includes
                                             # https://ripls.app,
                                             # https://dev.ripls.app
  method          = ["GET", "HEAD"]
  response_header = ["Content-Type"]
  max_age_seconds = 3600
}
```

`origin` and `method` are correct for `<video>` element fetches. The
`response_header` list is the weak point: it doesn't expose
`Content-Range`, `Accept-Ranges`, or `Content-Length`, which Chrome
needs in order to honor byte-range requests for seeking and progressive
playback. That's a likely secondary bug once the code path is fixed.

## Potential Solutions

### Option A: `kIsWeb` branch in `video_cache_helper.dart`, conditional `dart:io` import

- **Approach:** Move `dart:io` and the `File`-based code path behind a
  conditional import. Add a `kIsWeb` branch in
  `createCachedVideoController` that constructs
  `VideoPlayerController.networkUrl(Uri.parse(url))` directly, skipping
  the disk cache entirely. Mobile behavior is unchanged — same
  `flutter_cache_manager` → `VideoPlayerController.file` path. Web
  gets a simple direct-to-`<video>` path with no caching layer
  (the browser's HTTP cache handles repeats).
- **Pros:** One file changes. Mobile playback and the on-disk cache are
  untouched. Pattern is consistent with the existing `kIsWeb` branch in
  `app/lib/main.dart:112` (Firebase init) and
  `app/lib/core/utils/post_registration_destination.dart:38`. Test
  surface is small — one new branch, one stub `dart:io` shim for web
  builds.
- **Cons:** Web loses persistent video caching across page reloads.
  Acceptable for guest RSVP (single hero video per session); revisit
  if web ever hosts a video-heavy feed.
- **Effort:** Small (~half day, including the conditional-import
  scaffolding and a test).

### Option B: Replace `flutter_cache_manager` with a web-compatible wrapper

- **Approach:** Swap `flutter_cache_manager` for a package that ships
  IndexedDB-backed web support (e.g. building our own around
  `package:web` IndexedDB, since there is no first-party Flutter
  package that does this for arbitrary binary blobs today). Keep the
  unified `VideoPlayerController.file` (mobile) /
  `VideoPlayerController.networkUrl` (web) split.
- **Pros:** Web gets on-disk caching parity with mobile.
- **Cons:** Significant scope creep. No proven dependency exists; we'd
  build the IndexedDB layer ourselves. Adds a new failure mode (quota
  exhaustion in the browser) that we don't need yet. Doesn't change the
  fact that on web we still need `VideoPlayerController.networkUrl` —
  the cache only saves a network round trip.
- **Effort:** Large (2–3 days).

### Option C: Drop the file cache entirely and always use `networkUrl`

- **Approach:** Replace `createCachedVideoController`'s body with a
  single `VideoPlayerController.networkUrl` call on every platform.
  Delete `VideoCacheManager`, `getVideoFile`, and `evictVideo`.
- **Pros:** Smallest possible code surface. Removes two abstractions.
- **Cons:** Regresses mobile playback. The cache is there for a
  reason — without it, every scroll past a feed video re-downloads
  hundreds of MB on cellular. iOS/Android also fetch range requests
  less efficiently than a local file. The mobile experience would
  degrade noticeably.
- **Effort:** Small, but the mobile regression rules it out.

## Recommendation

**Option A.** It fixes the bug surgically and matches how every other
platform-specific branch in the codebase is structured. The mobile
caching layer stays exactly as it is; web gets a direct `<video>`
element with browser-managed HTTP caching, which is the standard pattern
for `video_player_web`.

The fix is intentionally narrow — it only addresses code-path

compatibility, not the secondary CORS-header gap. That gap is
addressed in Phase 2 of the implementation plan below, because the
fix is mechanically separate (Terraform change vs. Dart change) and
needs to be verified independently with real browser behavior.

## Open Questions

- **Should the web branch use `VideoPlayerOptions(mixWithOthers: true)`
  or similar web-specific options?** Recommendation: no, omit
  options entirely on web. The existing mobile path doesn't pass any
  `VideoPlayerOptions` either, and the browser's autoplay-with-mute
  policy already matches our `setVolume(0.0)` → `play()` startup. If a
  future audio-on-tap interaction breaks, we'll add the option then.

- **Does `MediaRepository.getVideoFile` need a web guard?** The web
  branch in Option A won't call it, but a future widget could. The
  cleanest defense is an `assert(!kIsWeb, ...)` at the top of
  `getVideoFile` so misuse fails loudly in debug rather than blowing up
  inside `flutter_cache_manager`. Recommendation: yes, add the assert
  in Phase 1. Cheap insurance.

- **Should we expose more headers in the GCS bucket CORS policy?**
  HTML5 `<video>` issues `Range` requests; without `Accept-Ranges` and
  `Content-Range` in `response_header`, Chrome can still play short
  clips but seeking and progressive download misbehave for larger
  files. Recommendation: extend `response_header` to
  `["Content-Type", "Content-Length", "Content-Range",
"Accept-Ranges"]` and add `"OPTIONS"` to `method`. Do this in
  Phase 2 once Phase 1 is on the smoke runner and we can measure the
  actual symptoms.

- **Do we need a `cached_video_player_plus`-style wrapper at all on
  mobile?** The issue body raises this; the answer is no — we already
  rolled our own (`VideoCacheManager` + `getVideoFile`) and it works
  on mobile. The issue's hypothesis that we use
  `cached_video_player_plus` is incorrect (see `pubspec.yaml`
  dependencies — only `video_player: ^2.9.2` and bare
  `flutter_cache_manager: ^3.4.1`).

- **Should video playback be part of the bundle-size CI gate?** Phase 1
  doesn't add code to the initial route bundle (event hero isn't on
  the first paint of `/app/event/`), but it could pull `<video>`
  initialization into the critical path. Recommendation: re-run
  `scripts/check_web_bundle_size.sh` before/after Phase 1 and update
  the baseline if it moves.

## Implementation Plan

### Phase 1: Make playback compile and play on web

**Goal:** A user opening `/app/event/{id}` with a video hero sees the
video play, muted, autoplaying, in Chrome and Safari.

**Tasks:**

- [ ] Split `app/lib/core/utils/video_cache_helper.dart` into a
      platform-shared entry and two implementation files using
      conditional imports:
      `video_cache_helper_io.dart` (mobile, current behavior) and
      `video_cache_helper_web.dart` (uses
      `VideoPlayerController.networkUrl`). Pattern: `export ... if
  (dart.library.io) ... if (dart.library.js_interop) ...`.
- [ ] In the web implementation, do not call
      `repo.getVideoFile`. Construct
      `VideoPlayerController.networkUrl(Uri.parse(url))` directly,
      then call `initialize() → setLooping(true) → setVolume(0.0) →
  play()` in the same order as today.
- [ ] Add `assert(!kIsWeb, 'getVideoFile is mobile-only')` to
      `MediaRepository.getVideoFile` so a stray web caller fails loudly
      in debug.
- [ ] Dart unit tests in
      `app/test/core/utils/video_cache_helper_test.dart` (path
      mirrors `app/lib/core/utils/`, which is the de-facto
      convention for non-ViewModel/non-repository helpers even
      though `docs/client/testing.md` only documents the
      `data/` and `presentation/viewmodels/` trees explicitly).
      Use `ProviderContainer` only if a provider override is
      needed; otherwise plain `test(...)` is fine since the
      helper is a free function. Always pair
      `final container = ProviderContainer(...)` with
      `addTearDown(() => container.dispose())`
      (per `docs/client/testing.md` Pitfall 1).
  - [ ] `isLocalPath` regression cases: `blob:abc-123` (web picker
        scheme — currently classified as "local" because host is
        empty, which on mobile would wrongly route to the file
        branch; on web we want it to route through `networkUrl`),
        plain `file:///tmp/foo.mp4`, empty string, malformed URI.
  - [ ] Web-target test (run with `flutter test -p chrome`) that
        constructs the helper against a fake `MediaRepository`
        (hand-rolled fake — `VideoPlayerController` is a plugin
        type and mocking it via Mockito is messy; the helper's
        public contract is the return type, not internal field
        access) and asserts: returns a `VideoPlayerController`,
        never calls `getVideoFile`, and the resulting controller's
        `dataSource` equals the input URL.
  - [ ] Mobile-target test that exercises the existing
        `flutter_cache_manager` path against a fake
        `VideoFileCache` (the abstraction already in
        `video_cache_manager.dart` for exactly this purpose) —
        preserves the current contract and prevents the web fix
        from accidentally degrading mobile.
- [ ] Widget test in
      `app/test/presentation/widgets/media/video_background_host_web_test.dart`
      (run with `flutter test -p chrome`).
      **Documented exception to the ViewModel-first testing
      preference in `docs/client/testing.md`:** the bug lives in a
      `ConsumerStatefulWidget`'s `State._maybeLoadController`
      method, not in a `Notifier`. The widget calls the helper
      directly from widget state rather than through a ViewModel,
      so there is no Notifier surface to drive. The test pumps a
      `VideoBackgroundHost` with `isVideo: true` and a fake remote
      URL, awaits the load, and asserts the snackbar fallback is
      _not_ shown (i.e. the swallowed-warning path from #2073
      stays cold on web). When the larger refactor that moves
      video-controller ownership into a Notifier lands, retire
      this widget test in favor of a ViewModel test.
- [ ] Wire `flutter test -p chrome` for the new web-target tests
      into `.github/workflows/test_flutter_web.yaml`. The workflow
      already builds for web; add a separate step that runs the
      narrow `app/test/**/*_web_test.dart` glob in Chrome. Keep it
      cheap — full `flutter test -p chrome` is slow and we don't
      need it.
- [ ] If `npm run build:web` after Phase 1 shows the bundle has
      grown (likely, because `video_player_web` may now be
      reached at runtime), update `app/web/bundle_baseline.txt`
      in the same PR with a note in the commit message
      explaining why. The CI gate
      (`scripts/check_web_bundle_size.sh`) lives in
      `test_flutter_web.yaml` and will otherwise fail.
- [ ] Update the **Video Cache** section of
      `docs/client/caching.md` (and the table at lines 308–313
      that compares Data / Image / Video caches) to document the
      mobile-vs-web split: mobile keeps the
      `flutter_cache_manager` disk cache; web uses the browser's
      HTTP cache via `VideoPlayerController.networkUrl`.
- [ ] Manual repro against the local server using a video already
      in `media_storage/` (the same surface where the bug was
      first noticed): `npm run start:server` with
      `--local-media-storage`, `npm run build:web`, navigate
      Chrome to an event with a video hero, confirm playback.

**Validates:** The compile-time `dart:io` block is removed, the
event-hero playback works on the surface that ships today, and the
helper has unit + widget coverage on both mobile and web targets.

### Phase 2: GCS CORS hardening for `<video>` range requests

**Goal:** Seeking and progressive download work in all target browsers
for videos longer than a few seconds.

**Tasks:**

- [ ] Confirm the symptom against a multi-MB video on `dev.ripls.app`:
      record whether Chrome devtools shows `(canceled)` requests, 206
      vs. 200 status codes, or CORS errors on the `<video>` fetch.
- [ ] If confirmed, extend
      `terraform/modules/storage/main.tf` `cors` block:
      `response_header = ["Content-Type", "Content-Length",
  "Content-Range", "Accept-Ranges"]` and
      `method = ["GET", "HEAD", "OPTIONS"]`.
- [ ] `terraform plan` against dev, then apply. Re-run the repro.
      Promote to prod once dev is verified.
- [ ] Cross-browser smoke: Chrome, Safari, Firefox, mobile Safari.

**Validates:** Real-world video sizes play and seek without 206/CORS
problems.

### Phase 3 — moved to #2162

**Status:** Out of scope for this issue. Tracked in
**#2162 "Flutter Web bundle as an E2E UI test harness"**.

While investigating Phase 3, we found that driving the production
video code path through a real browser requires authenticating the
session — `WebExperienceScreen` renders only an icon-and-text
placeholder when unauthenticated, so the unauth route never reaches
`createCachedVideoController`. Authenticating from a smoke driver
means either token injection (Flutter Web uses
`flutter_secure_storage` v10 with crypto.subtle-encrypted storage,
which we can't trivially poke from outside the app) or driving the
login UI from CDP. Both options are larger than the bug being
protected against and apply equally well to every future
"web-only-regression" test we'll want to write.

The right home for that investment is a general-purpose E2E harness,
not a one-off in `server/cmd/web-smoke`. #2162 carries the design
discussion (token injection vs. UI drive, seed-endpoint shape,
fixture management) and the first scenario it will validate is the
video-on-event-hero case originally drafted here. The acceptance
criterion there is "fails when `createCachedVideoController`
regresses" — the exact regression coverage Phase 3 promised.

The original detailed task list below is preserved as a reference
for the work that will eventually land in #2162.

#### Original Phase 3 scope (reference — implement in #2162)

**Goal:** A future change that re-breaks web video playback fails CI
instead of silently shipping.

**Test fixture:** A small, committed mp4 used by both the seed helper
and the smoke assertion. Options, easiest first:

- **Reuse an existing repo asset.** `website/content/homescreen-images/streaming-create.mp4`
  and a few siblings under `website/content/` are short, known-good
  mp4s already tracked in Git LFS (the workflow already does
  `git lfs pull`). Pick the smallest one and symlink/copy it into a
  new `server/cmd/web-smoke/testdata/video.mp4` so the fixture path
  is stable even if the website tree gets rearranged.
- **Use a video the user provides.** If we need something the player
  exercises differently (longer for range requests, h.264 instead of
  h.265, etc.), Bryan offered to provide one. The seed helper takes
  a `--video=PATH` flag so swapping fixtures is a one-line change in
  the workflow.

**Seeding strategy:** Add `server/cmd/web-smoke-seed/main.go` that:

1. Connects to the running smoke server via its public Connect
   client (the smoke runs the real server binary against the smoke
   Postgres — there is no need for the seed to touch the DB
   directly, and doing so would re-implement schema migrations).
2. Uploads the fixture mp4 via the public `AddMedia` RPC. The seed
   creates its own throwaway user via `RegisterUser` rather than
   importing helpers from `server/integration_tests/test_helpers.go`
   — that package is `package integration_tests` and pulling it in
   from a non-test binary drags in testcontainers wiring we don't
   need. Keep the seed self-contained and under ~300 lines (well
   inside the server architecture's <1000-line file guidance).
3. Creates a community + experience whose hero references the
   uploaded `media_id`, via the public `CreateCommunity` and
   `SaveExperience` RPCs.
4. Writes the public URL of the experience (e.g.
   `http://localhost:8080/app/event/{id}`) to stdout for the smoke
   driver to pick up.

Logging conventions (per `docs/server/observability.md`): mirror
`web-smoke-db`'s style — `log.Fatalf` on unrecoverable errors and
`fmt.Fprintf(os.Stderr, ...)` for diagnostics. The full
`slog`-based `logging.LoggerWithContext` machinery is overkill for a
one-shot CLI helper of this size; `web-smoke-db` sets the precedent.
Only stdout carries the seeded URL so callers can do
`URL=$(go run ./server/cmd/web-smoke-seed)`.

**Smoke assertions:** Add a `--check-video` mode to
`server/cmd/web-smoke/main.go` (or a new subcommand) that, after the
existing `wait` window expires, runs a CDP `Runtime.evaluate` to
assert:

- `document.querySelectorAll('video').length >= 1`
- For the first `<video>`: `readyState >= 2` (HAVE_CURRENT_DATA),
  `error === null`, `paused === false || currentTime > 0` (proves the
  decoder produced at least one frame).
- Pull a single-frame screenshot via CDP `Page.captureScreenshot`
  scoped to the `<video>` element's bounding box, save to
  `--screenshot-video=PATH`. A black/transparent frame is the
  signature CORS-or-codec failure mode and is easy to eyeball in CI
  artifacts.
- No console errors matching `/video_player|CORS|MissingPlugin/`.

Exit non-zero if any assertion fails — Phase 1 already passes a
manual repro, so the smoke's job is to prevent regression on every
PR.

**Tasks:**

- [ ] Pick + commit fixture mp4 to `server/cmd/web-smoke/testdata/`.
      Confirm size stays under the LFS budget the smoke workflow
      already pays for.
- [ ] Write `server/cmd/web-smoke-seed/main.go`. Reuse seed-user
      identities from `server/integration_tests/test_helpers.go`
      where possible.
- [ ] Extend `server/cmd/web-smoke/main.go` with `--check-video`
      assertions + per-element screenshot.
- [ ] Update `scripts/web_smoke.sh` to run the seed step before
      navigation and pass the resulting URL into the driver.
- [ ] Wire the new step into `.github/workflows/test_web_smoke.yaml`,
      after the existing console-error smoke. Upload the per-video
      screenshot as a workflow artifact so failures are diagnosable
      from the GitHub Actions UI. Add the new directories to the
      workflow's `paths:` filter so the smoke runs on any change to
      `server/cmd/web-smoke-seed/**` or
      `server/cmd/web-smoke/testdata/**`.
- [ ] Verify locally: `npm run build:web && scripts/web_smoke.sh`
      passes against a clean Postgres + fresh seed.

**Validates:** Regression protection. Catches both Phase 1 (controller
construction) and Phase 2 (CORS) breaks. The screenshot artifact also
catches subtler regressions like "video element initializes but
renders a black frame" that a `readyState` check alone would miss.

### Phase 4 (deferred — not required for #2155 closure): Web upload preview

**Goal:** Once the full app reaches web and users can upload videos
from a browser, the in-flight preview from a `blob:` URL works.

**Tasks:**

- [ ] Update `isLocalPath` to recognize `blob:` URLs as remote-ish
      (they're playable via `networkUrl` even though they parse with no
      host).
- [ ] Decide on a strategy for the post-upload swap from `blob:` to
      presigned GCS URL without flickering. This is the same problem
      #2073 solves on mobile; reuse the same state-shape.
- [ ] If any new user-visible copy is added (e.g. a web-specific
      upload error string), add the ARB key to both
      `app/lib/l10n/app_en.arb` and `app/lib/l10n/app_es.arb` —
      `npm run lint:dart:l10n-parity` and
      `npm run lint:dart:i18n` will block the PR otherwise.
      The existing `mediaVideoLoadFailed` key already covers the
      load-failure snackbar; no new strings are needed for Phase 1.

**Validates:** Upload-then-play round trip on web. Not blocking this
issue's closure — current web surfaces are read-only.

## Related Issues

- **#2162** — Follow-up. Carries the original Phase 3 scope
  (automated browser-driven regression coverage for video playback)
  as the first scenario of a general-purpose Flutter Web E2E
  harness. Filed because the auth-injection plumbing is bigger than
  this bug.
- #2050 — Sibling. Shipped the Web Smoke pipeline and the
  `/app/event/*` route that surfaces this bug. The harness #2162
  builds extends the infrastructure that issue introduced.
- #2073 — Mobile video upload preview bug. Cross-references the same
  `createCachedVideoController` helper but addresses the
  picker-path-as-URL failure mode; the audit trail there explains why
  the swallowed-warning was upgraded to a severe log + snackbar.
- #1195 — Parent program for the broader web rollout. Once the
  "serve the whole Ripls app on web" sub-issue is filed, link it here
  and replace the "#XXXX" placeholder in the issue body.

## Audit notes (2026-05-25)

Reviewed against `docs/client/{architecture,caching,i18n,
accessibility,testing}.md` and `docs/server/{architecture,
observability}.md`. Changes made to the plan:

- **Phase 1 testing:** Called out the `app/test/core/utils/` path
  as the de-facto convention (testing.md only documents the
  `data/` and `presentation/viewmodels/` test trees explicitly);
  added the `addTearDown(() => container.dispose())` requirement
  from testing.md Pitfall 1; switched from Mockito to hand-rolled
  fakes for `VideoPlayerController` since it's a plugin type;
  reused the existing `VideoFileCache` abstraction in
  `video_cache_manager.dart` for the mobile-path fake.
- **Phase 1 widget test:** Explicitly flagged the widget test as
  a documented exception to testing.md's "Test business logic,
  not Flutter widgets" guidance, with the rationale (the bug is
  in widget state, not in a Notifier) and a sunset condition
  (retire when video-controller ownership moves into a Notifier).
- **Phase 1 docs:** Added a task to update `docs/client/caching.md`'s
  Video Cache section and the cache-comparison table — the doc
  currently only describes the mobile flow, and the web flow is a
  documented architectural change.
- **Phase 1 bundle gate:** Added a task to refresh
  `app/web/bundle_baseline.txt` if the bundle grows, since the CI
  gate from #2050 will otherwise fail.
- **Phase 3 seed helper:** Removed the suggestion to reuse
  `server/integration_tests/test_helpers.go` — that's a test
  package and pulling it into a non-test binary drags in
  testcontainers wiring that doesn't belong. Replaced with
  "use the public Connect RPC surface directly" and bounded the
  file size to ~300 lines (well inside the server architecture's
  <1000-line guidance).
- **Phase 3 observability:** Explicitly stated that the seed
  helper uses `log.Fatalf` / `os.Stderr` (matching `web-smoke-db`)
  rather than the full `slog`-based `logging.LoggerWithContext`
  machinery — observability.md's structured-logging requirement
  targets the production server, and `web-smoke-db` is the
  established precedent for one-shot CLI helpers.
- **Phase 3 CI:** Added the new directories to the workflow
  `paths:` filter so the smoke runs on relevant changes.
- **Phase 4 i18n:** Added a reminder that any new user-visible
  copy needs ARB keys in both `app_en.arb` and `app_es.arb` per
  i18n.md (CI lint check
  `npm run lint:dart:l10n-parity` enforces this). Noted that
  Phase 1 reuses the existing `mediaVideoLoadFailed` key, so no
  new strings are required for the primary fix.

**No issues found** in the plan against:

- `docs/client/architecture.md` — the existing
  `VideoBackgroundHost`-calls-`MediaRepository` shape is a
  pre-existing violation (widget reaching past ViewModel into a
  repository), not introduced by this plan. Out of scope to
  refactor here.
- `docs/client/accessibility.md` — no new interactive widgets; the
  existing `CarouselVideoPlayer` already wraps the play/pause
  action in `Tappable` with an l10n-driven `semanticsLabel`.
- `docs/server/architecture.md` — the seed binary fits the
  established `server/cmd/<short-name>/main.go` shape (`web-smoke`,
  `web-smoke-db`, `check-goroutines`).
