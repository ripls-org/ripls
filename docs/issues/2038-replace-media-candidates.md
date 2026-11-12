# Issue #2038: Offer 3-4 candidate images in 'Replace background' modal for URL and Pexels-mode unified-create

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2038
**Filed:** 2026-05-19 | **Last activity:** 2026-05-19
**Labels:** (none) | **Assignees:** unassigned
**Context docs:** none (per user, the specific clarification rides inline)

## Problem Statement

The unified-create flow lands a single preview image. When the user
disagrees with the pick, the only escape is **Replace Media** — which
opens an empty `MediaPickerDialog` with three buttons (Videos / Photos /
Camera) and starts a fresh search-or-upload. Yet at gen time the server
*already enumerated* candidates and just threw the alternates away:

- **URL mode:** og:image, twitter:image, JSON-LD `Product.image[]`,
  Amazon `data-a-dynamic-image` map, srcset ladders, Substack-CDN
  variants — `server/webfetch/fetcher.go:325` (`extractImageURL`)
  returns the *single best* match after walking these.
- **Pexels mode:** the provider returns dozens of matches —
  `server/media/pexels_provider.go:118` (`SearchPhotos`) / `:305`
  (`SearchVideos`) — and we keep one.

The original issue scoped this as a server change + a thumbnail row in
the existing modal. The clarification from the requester (2026-05-19)
pins the UX shape: **a row of 3-4 squircle thumbnails inside the
existing "Replace Media" modal, positioned below the title and above
the Videos button**. Image and video candidates both qualify; video
gets a play-icon overlay.

The win is biggest on Pexels-mode where the relevant alternates are
already in memory, but URL-mode also benefits — a single page often
yields 6-8 candidate images (Amazon `data-a-dynamic-image` ladder,
JSON-LD `Product.image` array, srcset entries).

## Context from Comments

No GitHub comments yet — but the issue body itself does the
recommendation work (Option A: URLs only, lazy fetch on selection) and
the requester's chat clarification narrows the UX (4-up squircle row in
the existing modal, between title and Videos).

## Codebase Findings

### Where the modal lives

- **Widget:** `app/lib/presentation/widgets/media/media_picker_dialog.dart`.
- Uses `showAccessibleModal` + `GlassSheet`; body is a `Column` with a
  title (`mediaPickerDialogTitleReplace` / `mediaPickerDialogTitleAdd`)
  followed by three `GlassInlineAction` rows.
- **Used from:** `unified_create_modal.dart:194-201`,
  `experience_preview_modal.dart:133`,
  `community_preview_modal.dart:66`, and via
  `ContentViewHelpers.showMediaPickerDialog` from
  `gear_content_view.dart`, `request_content_view.dart`,
  `experience_content_view.dart`, `community_content_view.dart`.
- **Only unified-create has candidates** at the time of the user's
  Replace-Media tap. The other surfaces (per-type preview modals,
  content views) open the picker without any gen context, so the
  thumbnail row should render conditionally.

### Where the candidates would come from

- **Server URL extraction:** `server/webfetch/fetcher.go:318-413`.
  `extractImageURL` is a "return the first hit and stop" function. To
  produce a list, refactor it (or sit a sibling
  `extractImageURLCandidates` next to it) so it walks JSON-LD →
  Amazon → og → twitter → main `<img>` srcset and returns the
  collected URLs in priority order with duplicates removed. Aspect-ratio
  prefilter (per issue's open-question recommendation): the
  `maxMobileFriendlyAspect` constant from #1917's phase 7 lives in
  `server/webfetch/srcset.go` / `image_url_upgrade.go` — reuse it.
- **Server Pexels:** `server/media/pexels_provider.go:60-66` already
  returns `PexelsSearchResponse.Photos[]`. The per-result
  `Src.Medium` / `Src.Small` URLs are public CDN URLs suitable for
  hotlinking from the client's thumbnail row. Pexels Videos
  (`SearchVideos`, `:305`) returns `Image` posters and `VideoFiles[]`
  with quality-tagged URLs.
- **Server-side gen call sites** that pick a single result and would
  need to retain the rest:
  - `server/services/gear/gen_ai_streaming.go:296, 475, 581`
  - `server/services/experience/gen_ai_streaming.go:250, 484, 722, 762, 970`
  - `server/services/request/gen_streaming.go:175, 356, 481`
- **MediaReady event:** `proto/ripls/api/gen_stream.proto:27` —
  currently only `repeated string media_ids = 1`. To carry alternates,
  add either (a) a parallel `repeated MediaCandidate candidates` field
  to `MediaReady`, or (b) a new event type `MediaCandidates` in
  `StreamGen*Response` siblings. See "Recommendation" for choice.

### Where the alternate URL becomes a real media

- **Client:** `app/lib/services/media_service.dart:42`
  (`addMedia(filePath: ..., bytes: ...)`) and `media_repository.dart:83`
  wrap `MediaService.AddMedia`. The current API takes
  `bytes + content_type + filename + description`.
- **Server `AddMedia`:** `proto/ripls/api/media_service.proto:12` —
  byte-payload only today. There is no `AddMediaFromURL` and no
  attribution-preserving "import this Pexels photo" flow exposed to
  the client.
- The internal Pexels copy path
  (`server/services/media/*` — `CopyStockImageForUser` / fanout
  pickers) already exists but is server-internal, used by `gen_*`
  picks. That logic is the right thing to reuse server-side when the
  user accepts a Pexels candidate.

### Existing UI patterns to mirror

- **Glass modal child layout:** see `MediaPickerDialog._MediaPickerContent`
  (header → 16px gap → action rows). Insert the thumbnail row
  immediately after the 16px gap and before the first action.
- **Rounded thumbnails:** carousel patterns use
  `BorderRadius.circular(8-12)`
  (`app/lib/presentation/widgets/media/carousel/carousel_thumbnail_strip.dart:161-163`).
  Flutter has no first-class squircle primitive; the codebase
  approximates with `circular(16)` for the squircle-feeling shape.
- **Image rendering:** every image-from-URL surface uses
  `CachedMediaImage` for cache + a11y. For hotlinked external URLs,
  the same widget works (it wraps `CachedNetworkImage`).
- **Play-icon overlay for video:** the codebase already uses
  `Icons.play_circle_fill` over media thumbnails in
  `app/lib/presentation/widgets/portfolio/portfolio_item_card.dart`
  family — reuse the same Stack pattern.

## Potential Solutions

### Option A: URLs only, lazy fetch on selection (recommended by the issue)

- **Approach:** Server enumerates candidates per mode and rides them on
  the `MediaReady` event as a `repeated MediaCandidate` (URL +
  content_type + optional attribution hints). Client renders a 4-up
  squircle row in `MediaPickerDialog` for the unified-create surface.
  On tap, client calls a new `AddMediaFromURL` RPC (or a server-side
  helper exposed via a thin wrapper) that fetches the URL, stores it,
  and returns a `media_id`. Client then swaps that id into the
  unified-create draft via the existing path.
- **Pros:** Smallest server change. Cheap — no extra storage until the
  user actually picks. Pexels-mode wins big since the alternates are
  in-memory at gen time. URL-mode falls naturally into the existing
  `webfetch` extraction.
- **Cons:** New RPC surface. Client hotlinks to amazon.com,
  pexels.com, substackcdn.com for thumbnails (already happens via the
  picked image's persisted form, but now also at preview time). Per
  the issue's recommendation, no privacy regression — just confirm
  with #1953 (image-upload sanitization) before merge.
- **Effort:** Medium. Proto change, two server points
  (`webfetch` + `pexels_provider` consumers in gen_*_streaming.go),
  one client surface (`media_picker_dialog.dart`), one new RPC, one
  new viewmodel method.

### Option B: Eager download all 4 server-side

- **Approach:** Server downloads all candidates during the stream and
  emits all four `media_id`s on `MediaReady`. Client renders the
  thumbnails using the existing `mediaUrlProvider`, swaps by id on
  tap.
- **Pros:** Selection is instant. No new RPC. Everything lives under
  presigned URLs. Attribution is set once at download time.
- **Cons:** 4× storage and 4× the bandwidth cost per unified-create
  draft, the vast majority of which the user never sees. The issue
  body specifically rejects this on cost grounds.
- **Effort:** Small server / small client — but bad economics.

### Option C: Hybrid — URL-mode lazy, Pexels-mode eager

- **Approach:** For Pexels, the alternates are already in our system
  (provider CDN); store them as media ids during gen. For URL-mode,
  send URLs as candidates and fetch on tap.
- **Pros:** Pexels selection is instant; URL-mode keeps the lazy
  economics.
- **Cons:** Two code paths, two client behaviors, two failure modes.
  Adds complexity without solving anything Option A doesn't already
  solve.
- **Effort:** Medium-large.

## Recommendation

**Option A.** Matches the issue's own recommendation, the requester's
UX clarification, and the cost profile. The new
`AddMediaFromURL` RPC is small surface but earns its keep — it lets
attribution flow correctly (Pexels candidates carry photographer
fields), and it keeps the client from having to download-then-upload.

If telemetry later shows per-selection latency is a problem, a small
server-side warm-up (eagerly fetch only the top alternate on stream
completion) is a cheap follow-up.

## Open Questions

- **3 vs 4 thumbnails?** Recommendation: 4. The glass sheet body width
  is `MediaQuery.size.width - 32` after the GlassSheet padding, ~344px
  on a Pixel 7 — four 72px squircles with 8px gaps fit cleanly. The
  issue's own recommendation aligns. Drop to 3 only if a Phase 1
  visual review shows wrap on small screens.
- **Where do the thumbnails appear when the modal is opened from
  non-unified-create surfaces (gear/request/experience preview,
  content views)?** Recommendation: hide the row entirely. The
  candidates only exist in the unified-create stream's response; on
  other surfaces, the modal stays exactly as it is today. Drive this
  with a nullable `candidates` parameter that defaults to `null`.
- **Should we also surface alternates for the *user-uploaded image*
  (image-mode unified-create)?** Recommendation: out of scope — there
  are no alternates in that mode, by definition. The issue's table
  reflects this.
- **Should the candidate proto carry image dimensions for layout
  decisions?** Recommendation: yes, optional `width_px` / `height_px`
  fields. Lets the client choose a better source URL (e.g., Pexels
  `medium` vs `small` based on density) and pre-allocate aspect ratio
  to avoid layout shift while the thumbnail loads.
- **Attribution: must the candidate carry creator name / source URL
  on the wire so the modal can render the credit?** Recommendation:
  not in Phase 1. The credit lives on the *final* media row after
  selection (the existing `Attribution` proto is populated by the
  server-side import). For the thumbnail row, a small Pexels/Unsplash
  source pill in a future polish phase is enough.
- **Should taps in the thumbnail row dismiss the modal?**
  Recommendation: yes, after the swap is enqueued. Matches the
  existing Videos/Photos/Camera rows which `Navigator.pop` then run
  the action. Add a brief inline progress spinner on the tapped
  thumbnail (Option A has a network round-trip) so the user knows
  something is happening.

## Implementation Plan

### Phase 1: Proto + server URL-mode candidates (no client UI yet)

**Goal:** The server emits up to 4 alternate URLs on `MediaReady` for
URL-mode unified-create. No client behavior change yet.

**Tasks:**
- [ ] Extend `proto/ripls/api/gen_stream.proto`:
  ```proto
  message MediaCandidate {
    string url = 1;
    string content_type = 2;            // "image/jpeg", "video/mp4", etc.
    optional int32 width_px = 3;
    optional int32 height_px = 4;
    optional StockImageProvider provider = 5;  // for attribution at import time
    optional string provider_photo_id = 6;     // Pexels photo id, etc.
  }

  message MediaReady {
    repeated string media_ids = 1;
    repeated MediaCandidate candidates = 2;    // additive, wire-compatible
  }
  ```
  Comments stay vendor-agnostic at the proto level ("stock image
  provider") per `CLAUDE.md` proto conventions, even though the
  server-internal types reference Pexels by name.
- [ ] Refactor `server/webfetch/fetcher.go:325` so the priority walk
  (JSON-LD → Amazon → og → twitter → main img srcset) returns the
  collected URLs in priority order with duplicates removed, capped at
  N=4 after the chosen one. Keep the existing
  `extractImageURL(...) string` as a thin wrapper around the new
  `extractImageURLCandidates(...) []string` so all current call sites
  continue working.
- [ ] Aspect-ratio prefilter: skip candidates whose advertised
  dimensions exceed `maxMobileFriendlyAspect` (already declared in
  `server/webfetch/image_url_upgrade.go` /
  `srcset.go`) before they enter the candidate list.
- [ ] At the URL-mode call site (the gear/experience/request streaming
  paths that own webpage-image fetching — `gear/webpage_image.go`,
  `experience/webpage_image.go`), pass the alternate URLs through to
  `MediaReady`.
- [ ] Tests:
  - `server/webfetch/fetcher_test.go` — extend
    `TestHTTPFetcher_ImageURLExtraction` with assertions on the
    candidates slice.
  - `server/services/gear/gen_ai_streaming.go` and
    `experience/gen_ai_streaming.go` — gen-stream tests assert
    `MediaReady.Candidates` populated on URL-mode paths.
- [ ] `npm run build` (lint + `go build ./...`) and
  `go build -tags=integration ./...` since `MediaReady` is touched
  by integration tests.
- [ ] **Proto comment hygiene (audit):** keep the new
  `MediaCandidate` comments vendor-agnostic per `CLAUDE.md` § Proto
  Comments — `provider` / `provider_photo_id` describe what the field
  *is*, not which vendor is current. Enum values can reference
  vendors (the existing `STOCK_IMAGE_PROVIDER_PEXELS` is the
  established exception because user-visible attribution depends on
  the actual provider).

**Validates:** The wire format is right and the server can produce
alternates without breaking existing clients.

### Phase 2: Server Pexels-mode candidates

**Goal:** Same `MediaReady.Candidates` populated when Pexels is the
image source.

**Tasks:**
- [ ] At the Pexels call sites in `gen_ai_streaming.go` (gear,
  experience, request), retain up to 4 alternate `PexelsPhoto` results
  in addition to the picked one.
- [ ] Map each retained alternate to a `MediaCandidate` with:
  - `url` = `Src.Medium` (the medium-sized CDN URL — fast, good for
    72-100px thumbnails).
  - `content_type` = `"image/jpeg"` (Pexels canonical).
  - `width_px` / `height_px` = `Width / Height`.
  - `provider` = `STOCK_IMAGE_PROVIDER_PEXELS`.
  - `provider_photo_id` = `strconv.Itoa(photo.ID)`.
- [ ] For Pexels videos: `url` = `Image` (poster),
  `content_type` = `"video/mp4"`, similar mapping. The play-icon
  overlay on the client is driven by `content_type` starting with
  `video/`.
- [ ] Tests: gen-stream tests that exercise Pexels paths assert
  candidate count + provider field.
- [ ] `npm run build` and integration build per Phase 1.

**Validates:** Pexels alternates ride the wire correctly for both
photos and videos.

### Phase 3: New RPC `AddMediaFromURL`

**Note (audit):** The handler must follow
`docs/server/observability.md` conventions — use
`logging.LoggerWithContext(ctx).With("operation", "AddMediaFromURL",
"user_id", authInfo.UserID, "user_email", logging.MaskEmail(authInfo.Email),
"provider", req.Msg.GetProvider().String(), "external_service",
"pexels"|"http")`. Use `DebugContext` for the fetch start, `InfoContext`
for success with `duration_ms`, `ErrorContext` only for true system
failures. Return errors via `connecterr.Internal(ctx, "AddMediaFromURL",
err, ...)` — bare `connect.NewError(connect.CodeInternal, err)` is
gated by `scripts/check_codeinternal.js` per `CLAUDE.md`.

**Security (audit):** `provider_photo_id` arrives from a previous
streaming response but the server should treat it as untrusted (a
malicious client could call `AddMediaFromURL` with arbitrary ids).
Apply a per-user rate limit on this RPC (same family as the existing
`AddMedia` limits if one exists; otherwise document a TODO with a
tracking issue per `CLAUDE.md` TODO conventions). For Pexels imports
that go through `CopyStockImageForUser`, the provider's API will 404
on bad ids — that's the practical defense. For generic-URL imports,
validate the URL scheme is `https`, content type starts with
`image/` or `video/`, and apply `webfetch`'s standard body-size cap.

**Goal:** Single-call import path the client uses on candidate
selection. Attribution is preserved server-side.

**Tasks:**
- [ ] Add to `proto/ripls/api/media_service.proto`:
  ```proto
  message AddMediaFromURLRequest {
    string url = 1;
    optional string description = 2;
    optional StockImageProvider provider = 3;
    optional string provider_photo_id = 4;
  }
  message AddMediaFromURLResponse {
    string id = 1;
  }
  rpc AddMediaFromURL(AddMediaFromURLRequest) returns (AddMediaFromURLResponse);
  ```
- [ ] Server handler in `server/services/media/`:
  - When `provider == STOCK_IMAGE_PROVIDER_PEXELS` and
    `provider_photo_id` is set, route through the existing
    Pexels-import helper (`CopyStockImageForUser` family) so the
    `Attribution` is populated correctly.
  - Otherwise fetch the URL via `webfetch.HTTPFetcher` with the same
    body-size caps used elsewhere, validate content type starts with
    `image/` or `video/`, store via the regular media writer.
  - Apply the same EXIF / sanitization rules referenced in #1953 once
    that lands; until then, document the gap.
- [ ] Generate Dart + Go code via `npm run generate`.
- [ ] Client `MediaRepository.addMediaFromUrl(...)` wrapper in
  `app/lib/data/repositories/media_repository.dart`.
- [ ] Service-layer wrapper in
  `app/lib/services/media_service.dart`.
- [ ] Unit + ViewModel tests for the new wrappers (mocks per
  `docs/client/testing.md` §1 ViewModel Testing).
- [ ] Server unit + integration tests:
  - Pexels path: feed a stub Pexels client, assert media row written
    with `Attribution{provider: PEXELS, creator_name, ...}`.
  - Generic URL path: stub fetcher, assert content-type validation
    and stored bytes.

**Validates:** End-to-end import works with attribution intact.

### Phase 4: Client UI — thumbnail row in `MediaPickerDialog`

**Architecture fit (audit):** preserves the MVVM ladder per
`docs/client/architecture.md` — widget watches `UnifiedCreateState`,
ViewModel calls `MediaRepository.addMediaFromUrl`, repository calls
service. No service-from-widget shortcut.

**Goal:** Render 4 squircle thumbnails between the title and the
Videos action; tap → import → swap into unified-create.

**Tasks:**
- [ ] In
  `app/lib/presentation/widgets/media/media_picker_dialog.dart`,
  add an optional parameter to `MediaPickerDialog.show` and
  `_MediaPickerContent`:
  ```dart
  final List<MediaCandidate>? candidates;
  final void Function(MediaCandidate)? onCandidateTap;
  ```
  When `candidates == null || candidates.isEmpty`, render nothing in
  the row (preserves the current modal exactly).
- [ ] Build a `_CandidateThumbnailRow` widget: horizontal `Row` of
  72×72 thumbnails, `BorderRadius.circular(16)` (squircle-feeling
  shape), `CachedMediaImage` for the URL, video items get an
  `Icons.play_circle_fill` overlay via `Stack`.
- [ ] **Image cache key (audit):** these are external URLs without a
  ripls `media_id`, so `ImageCacheKeys.thumbnail(mediaId)` does not
  apply (per `docs/client/caching.md` § Image Cache Keys). Pass an
  explicit cache key derived from the candidate identity — e.g.
  `'candidate:${provider}:${provider_photo_id ?? url-hash}'` — so the
  same Pexels photo across two unified-create runs hits the disk cache
  instead of re-downloading.
- [ ] A11y per `docs/client/accessibility.md`:
  - Use `Tappable` (not raw `GestureDetector`) for each thumbnail —
    the lint rule `avoid_raw_gesture_detector` will reject otherwise.
  - `semanticsLabel` from `context.l10n.a11y...` (new keys with `a11y`
    prefix) — values like
    `a11yReplaceMediaCandidate(int index)` /
    `a11yReplaceMediaCandidateVideo(int index)`.
  - `CachedMediaImage(semanticsLabel: ...)` is required by lint rule
    `require_semantic_label_on_cached_media_image`.
- [ ] i18n per `docs/client/i18n.md`:
  - Add new `replaceMedia*` keys to `app_en.arb` and `app_es.arb`
    with `@key` descriptions.
  - Run `npm run lint:dart:l10n-parity` and `npm run lint:dart:i18n`.
- [ ] Wire `unified_create_modal.dart:194-201` to pass the latest
  candidates from `UnifiedCreateState` and a `onCandidateTap` that
  calls a new viewmodel method.
- [ ] Add `UnifiedCreateState.candidates` (Freezed copyWith — new
  field, no migration needed). Populate it from the `MediaReady`
  event in the streaming reducer
  (`unified_create_streaming_actions.dart`).
- [ ] New ViewModel method
  `UnifiedCreateViewModel.useCandidate(MediaCandidate)`:
  - Show inline spinner state (`UnifiedCreateState.candidateImportingIndex`).
  - Call `MediaRepository.addMediaFromUrl(...)`.
  - On success, replace the current media in state (single-image
    case) and emit the success animation already used by photo
    replacement.
  - On failure, surface as `UserError` per
    `app/lib/core/errors/README.md` (`RpcErrorHandler.classify`).
  - **Async safety (audit):** if this ViewModel is autoDispose
    (verify in `unified_create_view_model.dart`), guard every
    post-await `state = ...` with `if (!ref.mounted) return;` per
    `docs/client/architecture.md` § "Async Safety in Notifiers".
    Wrap the RPC call in `AsyncValue.guard` if the surrounding state
    is `AsyncValue`-shaped; otherwise hand-roll the try/catch and
    classify into `UserError`. Always clear
    `candidateImportingIndex` in `finally` so a disposal mid-flight
    doesn't leave the spinner stuck for the next mount.
- [ ] Tests (per `docs/client/testing.md` ViewModel-first):
  - **Primary:** ViewModel test
    (`app/test/presentation/viewmodels/unified_create_view_model_test.dart`)
    with `ProviderContainer` + a mocked `MediaRepository`:
    `useCandidate` happy path, network-error path
    (`RpcErrorHandler.classify`-ed `UserError`), and a disposal-during-
    async test that calls `useCandidate` and disposes the container
    before completion (asserts no crash, matches
    `docs/client/architecture.md` § "Disposal Testing").
  - **Widget-layer:** `_CandidateThumbnailRow` test covering: no
    candidates → no row; 4 candidates → 4 thumbnails; video candidate
    → play overlay rendered. Use `localizationsDelegates` +
    `supportedLocales` per
    `app/test/helpers/l10n_helpers.dart`.

**Validates:** End-to-end UX — open modal, see thumbnails, tap one,
preview swaps.

### Phase 5: Telemetry + polish

**Goal:** Measure adoption so we can prove value and plan refinements.

**Tasks:**
- [ ] Emit an analytics event on candidate tap with `provider`
  (`pexels` / `url`), `index` (0..3), and `content_type`
  (`image/video`). Use the existing `observabilityServiceProvider`
  pattern.
- [ ] Emit a server-side log when `AddMediaFromURL` is called —
  structured `logging.LoggerWithContext(ctx)` with
  `operation`, `provider`, `user_id` (auto) per
  `docs/server/observability.md`.
- [ ] Decide whether to add a small "source: Pexels" pill on the
  modal once Phase 1-4 are in users' hands (deferred from open
  questions).
- [ ] Update `docs/design/unified-create.md` with a "Replace media
  candidates" section linking back to this issue.

**Validates:** We know whether anyone actually uses this and can plan
the next pass.

## Related Issues

- **#1917** — image-resolution work that solves the *first* pick.
  This issue solves the *second* pick.
- **#2035** — the PR whose manual testing surfaced this idea.
- **#2037** — bot-mitigated sites; orthogonal but cleanly chains
  (URL-fetch failure could fall back to Pexels-mode alternates here).
- **#1953** — image-upload content-type / size / EXIF sanitization.
  Phase 3's `AddMediaFromURL` should adopt the same sanitization once
  #1953 lands.
- **#2039** — type-toggle in unified-create discards the
  URL-extracted image; if both this and #2039 land, the candidate
  list survives across type toggles for free.

### Follow-ups closed out

- **#2063** — Unsplash candidate imports now use the trusted-provider
  path (parity with Pexels). Landed in PR #2085: adds
  `UnsplashClient.GetPhoto` + a real `UnsplashProvider.GetStockImageByID`
  so Unsplash candidates preserve photographer attribution instead of
  falling through to the generic-URL path.

---

## Audit notes (added by `/ripls-audit` on 2026-05-19)

Reviewed against `docs/client/architecture.md`,
`docs/client/caching.md`, `docs/client/i18n.md`,
`docs/client/accessibility.md`, `docs/client/testing.md`,
`docs/server/architecture.md`, and `docs/server/observability.md`.

Changes applied to the plan:
- **Phase 3 server logging** — added explicit
  `logging.LoggerWithContext(ctx).With(...)` shape with standard field
  names (`operation`, `user_id`, masked `user_email`, `provider`,
  `external_service`, `duration_ms`). Levels mapped to docs guidance
  (Debug for read steps, Info for success, Error only for system
  failures). `connecterr.Internal` use spelled out (CI gate).
- **Phase 3 security** — added a section flagging `provider_photo_id`
  as untrusted input, with rate-limit + content-type + body-size
  validation requirements. Pexels-path defense leans on the
  provider's 404 behavior; generic-URL path needs explicit checks.
- **Phase 4 architecture fit** — MVVM ladder
  (widget → ViewModel → Repository → Service) called out
  explicitly so reviewers can verify no shortcut.
- **Phase 4 async safety** — added `ref.mounted` guard +
  `AsyncValue.guard` guidance per `docs/client/architecture.md` §
  "Async Safety in Notifiers", plus a `finally` clear on
  `candidateImportingIndex` to prevent stuck-spinner-after-dispose.
- **Phase 4 image cache key** — Pointed out that external URLs lack
  a ripls `media_id`, so `ImageCacheKeys.thumbnail(mediaId)` doesn't
  apply. Suggested
  `'candidate:${provider}:${provider_photo_id ?? url-hash}'` for
  cross-session reuse via `flutter_cache_manager`.
- **Phase 4 tests** — re-anchored on ViewModel-first per
  `docs/client/testing.md`: ViewModel test is primary, widget test is
  for the rebuild behavior. Added disposal-during-async test per
  the architecture doc's "Disposal Testing" section.
- **Phase 1 proto comment hygiene** — added a reminder that
  `MediaCandidate` field comments stay vendor-agnostic per
  `CLAUDE.md` § Proto Comments, with the established
  `STOCK_IMAGE_PROVIDER_PEXELS` enum-value exception noted.
- **No changes needed in other areas** — Phase 4 a11y already
  matched `docs/client/accessibility.md` (Tappable, `a11y` l10n keys,
  `CachedMediaImage(semanticsLabel:)`). Phase 4 i18n already cited
  `app_en.arb` + `app_es.arb` + parity check. Phase 1/2 proto
  changes are additive and wire-compatible. Server architecture is
  sound — new RPC reuses `CopyStockImageForUser` family, no
  service-to-service calls per `docs/server/architecture.md` §
  "Service Independence".
