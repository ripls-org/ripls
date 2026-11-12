# Issue #2073: Video upload appears to succeed but playback fails silently (local picker path used as URL)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2073
**Filed:** 2026-05-20 | **Last activity:** 2026-05-20
**Labels:** bug | **Assignees:** unassigned
**Context docs:** none (referenced during research: [docs/client/media.md](../client/media.md))

## Audit notes (`/ripls-audit`, 2026-05-20)

Reviewed against [docs/client/architecture.md](../client/architecture.md),
[docs/client/caching.md](../client/caching.md),
[docs/client/i18n.md](../client/i18n.md),
[docs/client/accessibility.md](../client/accessibility.md),
[docs/client/testing.md](../client/testing.md), and
[docs/client/media.md](../client/media.md). Server docs
(`docs/server/architecture.md`, `docs/server/observability.md`) are not
applicable — this is a pure client change with no proto, RPC, or
storage impact. The existing `AddMedia` / `GetMedia` / `SaveExperience`
contracts already work; the server logs in the issue body confirm
upload succeeds.

Changes applied:

- **Pure client, no server work.** Made explicit at the top of the
  implementation plan so reviewers don't expect a Phase 0 server
  change.
- **Existing test files reused.** The original plan said "create if it
  doesn't exist" for `experience_media_actions_test.dart` and
  `gear_view_model_test.dart`.
  [app/test/presentation/viewmodels/experience_view_model_test.dart](../../app/test/presentation/viewmodels/experience_view_model_test.dart)
  and
  [gear_view_model_test.dart](../../app/test/presentation/viewmodels/gear_view_model_test.dart)
  both already exist with `MockMediaRepository` set up (lines 25, 31,
  43, 52 in the experience test). The new cases append to those files
  and reuse the existing generated `*.mocks.dart` rather than running
  `flutter pub run build_runner build` for a fresh mock set. No new
  `@GenerateMocks` entries are needed for Phase 1.
- **Error state pattern reconciled with `UserError` / `RpcErrorHandler`.**
  [app/lib/core/errors/README.md](../../app/lib/core/errors/README.md)
  is the canonical error-state pattern for *RPC* errors —
  `UserError? error` on ViewModel state, `classify()` in catch,
  `localize()` in widget. `VideoBackgroundHost`'s catch is NOT an RPC
  error (it's an `ArgumentError` from `flutter_cache_manager`), and
  the failure scope is "this widget's video can't load" rather than
  "the ViewModel's data fetch failed", so the snackbar-from-widget
  approach in Phase 3 is the correct fit. Audit confirms this is
  consistent with
  [docs/client/architecture.md:35-37](../client/architecture.md#L35-L37)
  ("Widgets resolve to localized text in `build()` via `context.l10n`")
  and does not violate the no-strings-in-ViewModel rule.
- **l10n key prefix verified.** `media*` is an established prefix in
  [app_en.arb](../../app/lib/l10n/app_en.arb) — see
  `mediaBatchUploadAllFailed`, `mediaDownloadFailed`,
  `mediaDeleteTitle`. `mediaVideoLoadFailed` slots in cleanly. The
  parallel Spanish key must land in
  [app_es.arb](../../app/lib/l10n/app_es.arb) in the same PR or
  `npm run lint:dart:l10n-parity` fails (i18n.md, "Every key must
  exist in every locale").
- **Phase 2 testing strategy revised.** Mocking
  `VideoPlayerPlatform.instance` to test
  `createCachedVideoController` end-to-end is high-effort and
  fragile (the `video_player` package's testing surface is
  documentation-light). Audit recommends extracting a pure
  `_isLocalPath(String)` helper inside `video_cache_helper.dart`
  and unit-testing *that* directly — every string-classification
  case (empty scheme, `file://`, `http://`, `https://`, malformed
  URI). The integration that calls
  `repo.getVideoFile()` vs `VideoPlayerController.file()` based on
  the helper's result is exercised indirectly via the Phase 1
  ViewModel tests (which mock `MediaRepository` and verify
  `addMedia → getMedia → state.mediaPath` flow). This trades one
  hard-to-write test for one trivial test plus existing coverage.
- **Snackbar de-dup uses a `Set<String>`, not a single field.** The
  draft's `_didShowErrorForMediaId` (singular) only blocks repeated
  snackbars for the same mediaId immediately after the first failure
  — but a user can switch backgrounds (`mediaId` changes), come back
  to a broken one, and re-trigger the snackbar. Audit updates Phase
  3 to use `final Set<String> _shownErrorMediaIds = {}`, gated on
  contains-then-add. Resets when the widget is disposed (natural
  lifecycle).
- **CLAUDE.md "no phase markers in comments" applied.** The plan
  itself uses "Phase 1/2/3" as section headers (fine — that's
  planning shorthand, not source). The implementation tasks must
  not write `// Phase 1` / `// pre-refactor` / `// new in this PR`
  comments in `*.dart` files. `scripts/check_ephemeral_comments.js`
  fails the build otherwise.
- **TODO discipline called out.** Phase 3 makes tap-to-retry
  optional. If it's deferred, no inline `TODO:` survives the PR —
  either implement, drop the language entirely, or file a follow-up
  issue and reference it as `TODO(#NNNN):` per CLAUDE.md "TODO
  Comments".
- **`lib/core/audio/` README precedent reused.** Not applicable — no
  new multi-file directory is created by this work. `core/audio/`
  was created by #1250 and already has a README.
- **Build / lint verification.** Each phase still ends with the
  explicit `flutter analyze`, `flutter test --reporter expanded`,
  and the relevant lint commands. Added
  `npm run lint:ephemeral-comments` to Phase 1 (commit-time
  safety) and a sanity `npm run lint:dart:size` run to confirm
  the changed files (none of which approach 1000 lines today)
  remain within the cap.

## Problem Statement

Uploading a video — either as the background of an Experience or as an
addition to the media carousel — completes on the server (`AddMedia` →
200, then `SaveExperience` → 200, then `GetMedia` returns the new
`media_id` with `content_type=video/mp4`) but never plays on the client.
The user sees no error, no toast, no fallback image — the background
just stays in its previous state. There are two distinct bugs stacked
on top of each other:

1. **Local picker path is used as a URL after upload.**
   [experience_media_actions.dart:121](../../app/lib/presentation/viewmodels/experience_media_actions.dart#L121)
   writes
   `mediaPath: isVideoFromServer ? filePath : mediaResponse.url`
   into state on the successful-upload path. The same shape lives in
   [gear_view_model.dart:695](../../app/lib/presentation/viewmodels/gear_view_model.dart#L695).
   The intent (visible in the surrounding comment, "VideoBackgroundHost
   creates the controller on its own when it sees mediaPath + isVideo")
   is to use the local file directly so playback starts without waiting
   for a presigned URL round-trip. But
   [VideoBackgroundHost._maybeLoadController](../../app/lib/presentation/widgets/media/video_background_host.dart#L59-L92)
   funnels every value through
   [createCachedVideoController](../../app/lib/core/utils/video_cache_helper.dart#L10-L22)
   →
   [MediaRepository.getVideoFile](../../app/lib/data/repositories/media_repository.dart#L114-L116)
   → `flutter_cache_manager`'s `HttpFileService.get`, which interprets
   the value as an HTTP URL. A local simulator path
   (`/Users/.../tmp/image_picker_…mp4`) has no scheme or host, so
   `_HttpClient._openUrl` throws
   `ArgumentError: No host specified in URI …`. The carousel path
   (`media_carousel.dart:_initializeCurrentVideo`) re-resolves via
   `getFullMediaUrl(mediaId)` so it usually escapes this trap once the
   server has persisted; the bug surfaces strongest for the *background*
   replacement flow on the just-uploaded item.

2. **The exception is swallowed.**
   [video_background_host.dart:87-89](../../app/lib/presentation/widgets/media/video_background_host.dart#L87-L89)
   catches every load failure with `_log.warning(...)` and silently
   leaves `_controller` null. The previous background stays painted (or
   a placeholder shows), no snackbar fires, no metric is bumped, and the
   user has no signal that something failed. The same swallow-and-log
   pattern lives in *every* call to `createCachedVideoController`
   (carousel, conversation, gen-experience, unified-create, community
   edit), so it's a systemic gap rather than one-widget-specific.

[docs/client/media.md:129](../client/media.md#L129) explicitly states
"All content ViewModels use this helper for server-loaded video. Local
uploads bypass it since the file is already on-device" — so the
documented design says local uploads should *not* go through
`createCachedVideoController`. The code never grew that branch (or it
regressed); either way, doc and behaviour disagree and behaviour is
wrong.

## Context from Comments

The issue was filed this session with no comments yet — there is no
external clarification or maintainer direction to factor in. The body
captured the full log trace, the relevant file/line pointers, and a
suggested fix direction. That last point doubles as the de-facto
maintainer recommendation (I wrote it before opening the issue) but it
is *not* an external review of the approach.

The absence of debate means three things are still my call rather than
group-decided:

- Whether the fix should land just for the experience background (the
  shortest user-visible bug repro) or extend to the gear-background and
  carousel paths in the same PR (they share the helper).
- Whether to switch to `VideoPlayerController.file()` for local-staged
  videos *and* keep `createCachedVideoController` for server videos, or
  unify by always uploading first and then loading via the server URL
  (slower perceived UX).
- Whether to surface load failures via a snackbar, an inline error
  pill on the background, or just a non-noisy fallback image — and
  whether to flag swallowed exceptions in every other
  `createCachedVideoController` caller now or in a follow-up.

See Open Questions.

## Potential Solutions

### Option A: Branch on scheme inside `VideoBackgroundHost`

- **Approach:** Inside `_maybeLoadController`, inspect
  `widget.mediaPath`: if it parses as a local file path (no scheme, or
  `file://`), construct `VideoPlayerController.file(File(mediaPath))`
  directly instead of going through `createCachedVideoController`. For
  http/https URLs, keep the existing path. Optionally pull this branch
  into `createCachedVideoController` itself so every other caller
  (`media_carousel`, `conversation_view_model`, `gen_experience_view_model`,
  `unified_create_view_model`, `community_edit_view_model`) benefits
  with no change at the call sites.
- **Pros:** Smallest diff. Preserves the "instant playback from local
  file" UX that the original code intended. Matches the design rule in
  [docs/client/media.md:129](../client/media.md#L129). Centralising
  the branch in `createCachedVideoController` fixes every caller at
  once.
- **Cons:** Keeps two playback paths (`File` vs cache-managed) that
  need to be reasoned about. Slightly leaky abstraction — the helper
  becomes aware of locality. After the experience save round-trips and
  the ViewModel rehydrates from the server, the local path is
  shadowed by the presigned URL; this transition needs to dispose the
  file-backed controller and create a network-backed one (handled by
  the existing `_loadedMediaId` guard, but worth a test).
- **Effort:** Small.

### Option B: After upload, swap `mediaPath` to the server URL immediately

- **Approach:** In
  [experience_media_actions.dart:119-124](../../app/lib/presentation/viewmodels/experience_media_actions.dart#L119-L124)
  and
  [gear_view_model.dart:694-700](../../app/lib/presentation/viewmodels/gear_view_model.dart#L694-L700),
  always write `mediaPath: mediaResponse.url` regardless of content
  type. Remove the `isVideoFromServer ? filePath : mediaResponse.url`
  ternary. `VideoBackgroundHost`'s existing cache path then works
  unchanged because the presigned URL has a scheme/host.
- **Pros:** Single playback code path. Eliminates the file-vs-URL
  dual mode. Matches how the carousel already behaves (it always
  re-resolves the presigned URL before constructing the controller).
- **Cons:** Loses the "instant local-file playback" optimisation —
  there's a perceptible pause between picker dismiss and playback
  start because the upload + presigned-URL fetch + first-segment
  download all gate the first frame. The placeholder logic at
  [experience_media_actions.dart:84-88](../../app/lib/presentation/viewmodels/experience_media_actions.dart#L84-L88)
  already shows an optimistic thumbnail in the carousel via
  `MediaUploadHelper.createPlaceholder`, but the **background**
  position has no equivalent placeholder, so the user would stare at
  the prior background for ~2-5s during a 10 MB video upload on
  iOS Simulator timings (the AddMedia call alone was 241ms in the
  repro log; first-byte from GCS adds more).
- **Effort:** Small.

### Option C: Both — branch in the helper AND surface failures

- **Approach:** Option A's helper-level scheme branch (so all
  `createCachedVideoController` callers benefit), plus an explicit
  error-surfacing pass in `VideoBackgroundHost` (and ideally every
  other caller) so future regressions don't silently rot. Surface
  errors as a one-time `ScaffoldMessenger` snackbar with a localised
  string, gated to fire at most once per `mediaId` to avoid spam from
  a retry loop. Add a route-level `ErrorWidget` or fallback image so
  the background isn't visually stuck on the prior content.
- **Pros:** Fixes the bug *and* removes the silent-failure class.
  The next time someone introduces a similar wiring error,
  observability surfaces it within the first repro instead of after
  user reports. Centralising the file/URL branch in the shared helper
  means the carousel + create flows + conversation media all benefit
  from a single fix.
- **Cons:** Larger diff. Surface-area decisions (which screens get
  snackbars, what the fallback image looks like, l10n keys) need
  product input. Risks scope creep into general media-error UX which
  the issue doesn't ask for.
- **Effort:** Medium.

## Recommendation

**Option C, scoped tightly:** push the scheme-aware branch into
`createCachedVideoController` so every existing caller benefits, fix
the two ViewModel sites that compute `mediaPath` for videos so they
behave correctly even before the helper change merges, and add a
single user-visible error surface in `VideoBackgroundHost` (the screen
the issue reports). Defer "surface failures in every other
`createCachedVideoController` caller" to a follow-up issue — it's the
right cleanup but it's larger than this bug and the call sites differ
enough that they want individual product decisions.

Reasoning: the bug spans two layers and Option A or B alone leaves one
risk in place. Option A alone keeps the silent-failure class for any
future wiring mistake; Option B alone loses the instant-playback UX
that the original code aimed for; Option C with the helper-level
branch is the smallest change that fixes both the data-shape bug and
the observability gap, without committing to a broader media-error UX
project.

The minimal viable PR could be Option A only (≤30 lines, no l10n
work, no design input) if we triage this as "stop bleeding now and
clean up later." That's worth proposing if the audit flags the
snackbar work as too large for one PR.

## Open Questions

- **Should the snackbar/error surface land in this PR or a follow-up?**
  — Recommendation: **land in this PR but minimally.** A single
  one-shot snackbar tied to the experience/gear background load failure
  is small (one new l10n key, one `ScaffoldMessenger` call inside
  `VideoBackgroundHost`'s catch block, one mutex on `_loadedMediaId`).
  Carousel and create-flow error surfaces are bigger and should be
  their own issue.
- **Local-file branch — inside `createCachedVideoController` or inside
  `VideoBackgroundHost`?** — Recommendation: **inside
  `createCachedVideoController`.** All five callers (`media_carousel`,
  `community_edit_view_model`, `conversation_view_model`,
  `gen_experience_view_model`, `unified_create_view_model`,
  `video_background_host`) have the same exposure when a local path
  sneaks in. Centralising in the helper is one branch versus six.
- **How do we detect "local file" vs "URL"?** — Recommendation: parse
  with `Uri.tryParse(path)` and treat anything with an empty scheme or
  scheme `file` as local; everything else routes through the cache
  manager. Falling back to `File(path).existsSync()` would force an
  unnecessary disk stat on every call and would still need to handle
  the scheme case for `file://` URIs.
- **Should the gear-background path
  ([gear_view_model.dart:695](../../app/lib/presentation/viewmodels/gear_view_model.dart#L695))
  get the same fix?** — Recommendation: **yes, in the same PR.** The
  ViewModel-level change is one identical line, the helper change
  fixes the actual playback for free, and the issue explicitly
  mentions "in replacing the background image" without specifying
  *which* content type. Excluding gear would leave the same crash
  half-fixed.
- **Story background
  ([story_content_view.dart:81](../../app/lib/presentation/screens/story/story_content_view.dart#L81))?**
  — Recommendation: **out of scope, no change needed.** Story doesn't
  upload media — it always reads from the server. The
  `mediaPath: state.mediaPath ?? mediaUrl?.url` shape already prefers
  a presigned URL via `mediaUrl`. Worth a one-line confirmation in the
  PR description but no code change.
- **Should the error log level stay `warning` or become `severe`?** —
  Recommendation: **`severe`** (matches the existing `_log.severe`
  used for upload failures in `experience_media_actions.dart:141`).
  Background playback failure is user-facing and we want it visible
  in client log exports — `warning` gets filtered out of the default
  console view.

## Implementation Plan

**Scope:** pure client. No proto, RPC, or storage changes. Reviewers
should not expect a Phase 0 server task.

### Phase 1: Fix the data shape — stop writing local paths as URLs

**Goal:** Eliminate the immediate cause. After a video upload, the
state's `mediaPath` always carries something that `VideoBackgroundHost`
can play, even before the helper change in Phase 2 lands.

**Tasks:**
- [ ] In
  [experience_media_actions.dart:119-124](../../app/lib/presentation/viewmodels/experience_media_actions.dart#L119-L124),
  replace
  `mediaPath: isVideoFromServer ? filePath : mediaResponse.url`
  with `mediaPath: mediaResponse.url`. Remove the now-unused
  `isVideoFromServer` ternary branch and update the surrounding
  comment to remove the "VideoBackgroundHost creates the controller
  when it sees mediaPath + isVideo" wording (it stays correct, but
  the file-path detail no longer applies).
- [ ] In
  [gear_view_model.dart:691-700](../../app/lib/presentation/viewmodels/gear_view_model.dart#L691-L700),
  apply the same change: `mediaPath: mediaResponse.url` instead of
  `mediaPath: filePath`. `mediaResponse` is the existing
  `getMedia(mediaId)` result a few lines up.
- [ ] Confirm the analogous spots in
  [experience_media_actions.dart:230](../../app/lib/presentation/viewmodels/experience_media_actions.dart#L230)
  and
  [gear_view_model.dart:805](../../app/lib/presentation/viewmodels/gear_view_model.dart#L805)
  (delete-media re-display) already use `firstMedia.url` — they do,
  so no change needed; spot-check is enough.
- [ ] Add a ViewModel-layer test to the existing
  [app/test/presentation/viewmodels/experience_view_model_test.dart](../../app/test/presentation/viewmodels/experience_view_model_test.dart)
  under the existing `ExperienceViewModel - Media Upload` group
  covering: upload a video → state `mediaPath` equals the mocked
  `mediaResponse.url` and never starts with `/Users/` or `/tmp/`.
  Reuse the existing `MockMediaRepository` and
  `mockMediaRepository.getMedia(mediaId)` mock pattern (the file
  already has `MediaRepository` in its `@GenerateMocks` list and
  `mediaRepositoryProvider.overrideWithValue(mockMediaRepository)`
  in `setUp`). Stub `getMedia` to return a `GetMediaResponse` with
  `contentType: 'video/mp4'` and `url: 'https://example.com/v.mp4'`.
  No `build_runner` rerun needed — no new mocks.
- [ ] Add the equivalent test to the existing
  [app/test/presentation/viewmodels/gear_view_model_test.dart](../../app/test/presentation/viewmodels/gear_view_model_test.dart),
  using the same `MockMediaRepository` already mocked in that file.
- [ ] Run `flutter analyze`, `flutter test --reporter expanded`,
  `npm run lint:dart:size`, and `npm run lint:ephemeral-comments`.
  Verify the test fails on the unfixed code and passes after the
  one-line change.

**Validates:** The reported "No host specified in URI" stack trace no
longer reproduces — verified both by the new unit tests and by manual
repro on iOS simulator. The instant-playback optimisation is gone but
the optimistic carousel placeholder (`MediaUploadHelper.createPlaceholder`)
covers the perceived-latency gap.

### Phase 2: Make `createCachedVideoController` scheme-aware

**Goal:** Future-proof every other caller of the helper so a local
path sneaking in doesn't crash. Brings the code into alignment with
[docs/client/media.md:129](../client/media.md#L129) ("Local uploads
bypass it since the file is already on-device").

**Tasks:**
- [ ] Extract a pure top-level helper
  `bool _isLocalPath(String value)` in
  [video_cache_helper.dart](../../app/lib/core/utils/video_cache_helper.dart):
  `Uri.tryParse(value)` → return true if scheme is empty, `file`, or
  the URI has no host for a non-file scheme. This is the
  unit-testable surface (no `video_player` platform dependency).
- [ ] In `createCachedVideoController`, branch on `_isLocalPath(url)`:
  the local branch constructs `VideoPlayerController.file(File(url))`
  directly and skips the cache lookup. The network branch runs the
  existing `repo.getVideoFile(mediaId, url)` path. Keep the
  `initialize()` / `setLooping(true)` / `setVolume(0.0)` / `play()`
  sequence common to both branches.
- [ ] Update the helper's doc comment to describe both branches and
  point at
  [MediaUploadHelper.createPlaceholder](../../app/lib/core/utils/media_upload_helper.dart#L31)
  as the documented source of local-path values that legitimately
  reach the helper (carousel optimistic preview before upload
  fires).
- [ ] Add a unit test
  `app/test/core/utils/video_cache_helper_test.dart` that
  exercises only the `_isLocalPath` classifier (export it as
  `@visibleForTesting` if necessary). Cases: empty string,
  `/Users/x.mp4`, `file:///Users/x.mp4`,
  `https://signed-bucket.googleapis.com/x.mp4?Signature=...`,
  `http://localhost:8080/x.mp4`, malformed `not a uri`. Do NOT
  attempt to drive `VideoPlayerController.initialize()` from this
  test — the platform binding makes that fragile; the actual
  branching is exercised end-to-end by the Phase 1 ViewModel tests
  (which mock `MediaRepository` and verify the
  `addMedia → getMedia → state.mediaPath` flow).
- [ ] Confirm no other call site changes are needed: grep all six
  `createCachedVideoController(` invocations and verify each
  continues to behave correctly (the network branch is unchanged;
  the local branch is reachable only via paths that previously
  crashed).
- [ ] Re-run `flutter analyze`, `flutter test --reporter expanded`,
  `npm run lint:ephemeral-comments`, and the existing Phase 1
  ViewModel tests to confirm no regression.

**Validates:** Any future code path that lands a local file in
`mediaPath` plays it as a file instead of crashing through the
`flutter_cache_manager` HTTP path. The behaviour matches the design
doc claim and removes the entire class of bug.

### Phase 3: Surface the failure if it happens

**Goal:** If `createCachedVideoController` *does* throw — for any
reason, including network failure or a corrupted file — the user
hears about it instead of seeing a stuck background.

**Tasks:**
- [ ] In
  [video_background_host.dart:87-89](../../app/lib/presentation/widgets/media/video_background_host.dart#L87-L89),
  change the log level from `warning` to `severe`. Add a
  `final Set<String> _shownErrorMediaIds = {}` field on
  `_VideoBackgroundHostState`. In the catch block, gated on
  `mounted` and `!_shownErrorMediaIds.contains(widget.mediaId!)`,
  add the mediaId to the set and call
  `ScaffoldMessenger.of(context).showSnackBar(...)` with a localised
  message. The set resets naturally when the widget is disposed,
  and using a set (not a single field) means a user who switches
  away from a broken background and comes back still only sees one
  snackbar per mediaId for the widget's lifetime.
- [ ] Add the new l10n key `mediaVideoLoadFailed` with value
  `"Couldn't load video"` to **both**
  [app/lib/l10n/app_en.arb](../../app/lib/l10n/app_en.arb) and
  [app/lib/l10n/app_es.arb](../../app/lib/l10n/app_es.arb) per the
  l10n parity rule. Spanish placeholder OK if real translation is
  unavailable (issue #1972). The `media*` prefix is established in
  the same file (`mediaBatchUploadAllFailed`,
  `mediaDownloadFailed`). Include the `@mediaVideoLoadFailed`
  metadata block with a `description`.
  `npm run lint:dart:l10n-parity` enforces parity — run locally;
  CI re-runs.
- [ ] **Tap-to-retry: drop from this PR.** Removing "Tap to retry"
  from the string above is the recommended path; if a follow-up
  ever wants retry, file a separate issue first and update the
  l10n key in that PR. Do NOT leave a plain `TODO:` (CLAUDE.md
  "TODO Comments" — every TODO must reference a tracked issue).
- [ ] Accessibility: `SnackBar` and `SnackBarAction` are built-in
  Material widgets and are not on the
  [a11y allowlist target set](../client/accessibility.md) (no
  `Tappable` wrapping needed). Do not add raw
  `GestureDetector` / `InkWell` inside this change —
  `scripts/check_a11y.js`'s `avoid_raw_gesture_detector` rule
  is enforced with an empty allowlist.
- [ ] Add a widget test
  `app/test/presentation/widgets/media/video_background_host_test.dart`
  covering: when `createCachedVideoController` throws, the
  `ScaffoldMessenger` receives one `showSnackBar` call with the
  l10n-resolved string, and a second throw on the same mediaId does
  not produce a second snackbar. Wrap the test widget in a
  `Scaffold` ancestor and the standard `localizationsDelegates` /
  `supportedLocales` boilerplate from
  [app/test/helpers/l10n_helpers.dart](../../app/test/helpers/l10n_helpers.dart).
  Inject a `MockMediaRepository` whose `getVideoFile` throws to
  drive the failure path without needing a real platform binding.
- [ ] Run `flutter analyze`, `flutter test --reporter expanded`,
  `npm run lint:dart:a11y`, `npm run lint:dart:i18n`,
  `npm run lint:dart:l10n-parity`, and
  `npm run lint:ephemeral-comments`.
- [ ] Manual QA: temporarily force the throw (e.g. pass a bogus URL
  to the helper from a debug-only branch) and confirm the snackbar
  appears. Revert the test perturbation before commit — do not
  leave debug toggles in the diff.

**Validates:** The next time a video load fails for any reason, the
user gets a visible signal, the failure logs at `severe`, and one-time
gating prevents snackbar spam.

### Phase 4 (optional follow-up, separate issue): error-surface parity across other callers

**Goal:** Apply the same one-shot-snackbar pattern to the five other
`createCachedVideoController` callers so the silent-failure class is
gone from the codebase entirely. Should be filed as a follow-up issue
linking back to this one.

**Tasks (sketch — to be detailed in the follow-up issue, not this PR):**
- [ ] Audit each caller's catch block, decide whether the snackbar
  belongs there or at the next layer up (e.g. the carousel's parent
  screen), and pick an appropriate l10n string per surface.
- [ ] Add ViewModel/widget tests at each site.

**Validates:** A regression in any video-controller call site is
visible the first time it happens, not after a user files a bug.

## Related Issues
- #1250 — Audio is taking over but not playing. Sibling work on the
  same `audio` branch; both touch `VideoBackgroundHost`. Phase 2 / 3
  of this plan should not collide with the `audio_session_manager`
  work but should be reviewed together if both are landing in the
  same PR queue.
- #1833 — Presigned URL expiry across closures. Provides the design
  for `getFullMediaUrl(mediaId)` re-resolution that the carousel
  already uses and that this fix relies on implicitly (Phase 1's
  switch to `mediaResponse.url` is fresh-at-upload-time, but
  long-lived background views still need #1833's pattern for the
  hours-later refresh).
- #929 — Dev server not generating videos (Pexels rate limit). Not a
  cause of this bug but the only other "videos don't play" issue
  open; cross-link in the PR description so triage doesn't conflate
  them.
- #847, #811, #1449 — Video thumbnails, video caching, and thumbnail
  streaming. Adjacent infrastructure; no overlap with this fix.
