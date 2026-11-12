# Issue #2044: Three pre-existing community-display bugs surfaced during #2041 manual testing

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2044
**Filed:** 2026-05-19 | **Last activity:** 2026-05-19
**Labels:** (none) | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Three independent community-display bugs found while manually testing PR #2041.
None are regressions from that PR. All three live in adjacent community-display
code (`community_avatar.dart`, `workshop_community_carousel.dart`,
`community_public_screen.dart`) and share the same data flow
(`mediaIds` → `MediaRepository` → `cached_network_image`).

- **Bug A:** Tapping a community link in another user's profile subtitle opens
  `CommunityPublicScreen` but the page spins forever — never resolves to data
  or error.
  **Update 2026-05-19: verified not present on current `main`, dropped from
  this PR.** Could not reproduce against a local profile-mode APK + local
  debug server. Reviewing the proposed hypothesis ("subtitle lists all of
  target's communities, not just mutual — tapping a non-mutual one fails
  auth"), the design already enforces mutual-only:
  `server/services/profile/profile.go:199-287` intersects viewer + target
  memberships and returns only mutual ones in `SharedCommunities` with a
  count of the rest in `OtherCommunityCount`. The leak invariant is
  pinned by `profile_test.go:411-448` (`TestGetUserProfileForViewer_LeakageInvariant`).
  Client at `user_profile_identity.dart:144-175` renders only those
  mutual entries and appends `userProfileOtherCommunities` ("and N other
  communities") for the rest. If anyone reproduces the spinner on a
  later build, file fresh with logcat covering
  `CommunityImpactViewModel` and `getCommunityProvider`.
- **Bug B:** Tapping a community pill in the Workshop carousel causes every
  avatar in the carousel to briefly flash to its initials placeholder before
  re-rendering the image. Classic `FutureBuilder` antipattern at
  `app/lib/presentation/widgets/community_avatar.dart:56-65` — the future is
  re-created on every parent rebuild, cycling through `waiting` → `done`.
- **Bug C:** A newly-created community shows the **previous** community's
  thumbnail in the Workshop carousel, while the community detail screen shows
  the **correct** AI-generated thumbnail for the same community. Persists
  across logout and app restart, which rules out a client-side memory cache
  and points at either a persistent client cache layer or a server-side
  response inconsistency.

The umbrella framing is appropriate: all three are bounded, share a data path,
and a single contributor can repro them side-by-side. Bug C is the one most
likely to outgrow the umbrella if the server side is implicated.

## Context from Comments

No comments. The issue body is unusually detailed and self-contained — file
paths, line numbers, repro, hypothesis, and acceptance for each bug. Filed by
the maintainer who surfaced the bugs (briangbrown). No external clarification
is needed before starting.

## Codebase Findings

- **Bug A — provider chain.**
  `CommunityPublicScreen` (`app/lib/presentation/screens/communities/community_public_screen.dart:48-119`)
  watches `getCommunityProvider(communityId)`, which wraps
  `CommunityRepository.get(id)` and is cache-backed. When `community` resolves
  it renders `CommunityMetricsScreen`, which watches
  `communityImpactProvider(communityId)` — an `AsyncNotifierProvider.autoDispose.family`
  at `app/lib/presentation/viewmodels/community_impact_view_model.dart:401`. The
  user-visible "spinning forever" can come from either future. Repro is on a
  non-member community, so the impact RPC is the prime suspect (an unguarded
  authorization branch on the server, or a notifier that never transitions to
  `data`/`error` for non-members).
- **Bug B — antipattern.** Confirmed at
  `app/lib/presentation/widgets/community_avatar.dart:53-70`. The
  `FutureBuilder<String>` is fed `ref.read(mediaRepositoryProvider).getMediaUrl(mediaId).then(...)`
  — a brand-new `Future` on every build. The codebase already has the right
  abstraction: `mediaUrlProvider` at
  `app/lib/services/providers/media_providers.dart:30`
  (`FutureProvider.family<String, String>`) and `mediaObjectProvider` at line
  42. Switching to `ref.watch(mediaUrlProvider(mediaId))` deduplicates and
  fixes the flash with no new state machinery.
- **Bug C — server response shape.**
  `Service.ListCommunities`
  (`server/services/community/membership.go:120-202`) populates each
  `api.CommunityItem.MediaIds` directly from
  `community.MediaIds` after a `storage.GetByIDs[*models.Community]` batch
  fetch. `GetCommunity` reads from the same `models.Community` row, so a
  legitimate divergence between the two responses requires either (i)
  stale data being read by one path but not the other, or (ii) the client's
  list cache holding a snapshot from before media generation completed.
  `CommunityRepository.listUserCommunities`
  (`app/lib/data/repositories/community_repository.dart:83-103`) caches the
  list under `user:list[:region]`.
  **Important — per `docs/client/caching.md` the current cache is
  StashCacheManager Phase 1: memory-only, does NOT survive app restarts.**
  The reported symptom ("logout + restart does not fix it") therefore rules
  out the client cache and strongly points at a server-side cause. This
  sharpens diagnosis: focus Phase 3 logging on the server first.

## Potential Solutions

Each bug has a clean solution; I'm presenting them per-bug rather than as a
single grand option because they're independent.

### Bug A — make the impact path resolve or fail explicitly

**Option A1 (recommended): Fix the impact notifier / RPC so non-members get
either an `AsyncData` (with public-only metrics) or an `AsyncError`.**
- **Approach:** Reproduce with logcat to isolate which future hangs. If the
  server's impact RPC is silently dropping non-member requests, fix the
  handler to return a concrete error or a public-safe subset. If the
  `AsyncNotifier.build` method has a path that never completes (e.g.,
  awaits a stream that closes without yielding), add a timeout and a
  defined error state.
- **Pros:** Restores the documented behavior of `CommunityPublicScreen`.
  Public communities should be inspectable by non-members — that's the point
  of having a "public" screen.
- **Cons:** Diagnosis requires a device repro; the fix surface depends on
  what the logs show.
- **Effort:** Small if the bug is purely client-side; small-to-medium if it
  requires a server authorization change.

**Option A2: Guard the screen — if the user isn't a member, skip
`CommunityMetricsScreen` and show a name/description-only view.**
- **Approach:** Branch in `CommunityPublicScreen` on the viewer's membership.
- **Pros:** Sidesteps the hang.
- **Cons:** Hides the feature non-members are supposed to see (impact
  metrics). Treats a bug as a product decision.
- **Effort:** Small.

**Option A3: Add a top-level timeout/error surface.**
- **Approach:** Wrap the futures in a `Future.timeout` that surfaces an
  error UI after, say, 10s.
- **Pros:** Defensive against future hangs.
- **Cons:** Hides the root cause; the metric still won't load.
- **Effort:** Small.

### Bug B — eliminate the FutureBuilder antipattern

**Option B1 (recommended): Replace the inline `FutureBuilder` with
`ref.watch(mediaUrlProvider(mediaId))`.**
- **Approach:** In `community_avatar.dart`, drop the `FutureBuilder` branch
  and use the existing `mediaUrlProvider` family. Riverpod caches by
  `(provider, mediaId)`, so successive rebuilds reuse the same `AsyncValue`
  instead of creating a fresh future.
- **Pros:** Minimal diff. Reuses existing infrastructure
  (`mediaUrlProvider` is the established cross-codebase pattern — see
  `app/lib/services/providers/media_providers.dart:30`). Removes a class of
  bug from the codebase rather than fixing one widget.
- **Cons:** None significant. `mediaUrlProvider` is not `autoDispose`, so the
  URL stays cached for the app lifetime (which is fine — `MediaRepository`'s
  own cache governs freshness, and `CachedNetworkImage` keys image bytes by
  `ImageCacheKeys.thumbnail(mediaId)`).
- **Architecture note:** `docs/client/architecture.md` states "Screens/Widgets
  never access repositories directly" — they should go through a ViewModel.
  A `FutureProvider.family` that wraps a repository is the codebase's
  established escape hatch for this kind of leaf data load (cf. `mediaUrlProvider`,
  `mediaObjectProvider`, `getCommunityProvider`). This is consistent with the
  surrounding code; introducing a dedicated viewmodel for an avatar's image URL
  would be over-engineered.
- **Effort:** Small.

**Option B2: Convert `CommunityAvatar` to `ConsumerStatefulWidget` and hold
the future in `initState`.**
- **Approach:** Capture the future once and rebuild it only when
  `community.mediaIds.first` changes (`didUpdateWidget`).
- **Pros:** Self-contained.
- **Cons:** More state machinery than needed. Doesn't dedupe across many
  `CommunityAvatar` instances rendered on the same screen (the carousel
  renders several). Each instance still does its own fetch.
- **Effort:** Small.

### Bug C — diagnose the response divergence first

**Option C1 (recommended): Add server- and client-side logging of
`community.MediaIds` for the same community ID across `ListCommunities`
and `GetCommunity`, then fix wherever the divergence lives.**
- **Approach:** (1) Add a structured-log line in
  `server/services/community/membership.go:184-191` that logs `community.Id`
  and `community.MediaIds` for the newly-created community. (2) Add the same
  in `Service.GetCommunity`. (3) Add a client log in
  `CommunityRepository.listUserCommunities` / `get` showing the parsed
  `mediaIds[0]`. (4) Repro per issue and diff. (5) Land the fix on whichever
  side actually carries the stale data.
- **Pros:** Repro is deterministic per the issue. The fix is cheap once we
  know which side is wrong. Logging is reusable for future debugging.
- **Cons:** Two trips: diagnose, then fix. Worth it — guessing the fix is
  worse than landing the wrong one.
- **Effort:** Small (diagnostic) + small-to-medium (fix).

**Option C2: Add an unconditional `refreshUserCommunities` after community
creation success.**
- **Approach:** Wire the create-community success path to invalidate the
  `user:list` cache and force a fresh fetch.
- **Pros:** If the bug is client-cache, this fixes it.
- **Cons:** If the bug is server-side, this papers over it (the next user
  who hits the bug from a different path is still broken). The issue
  explicitly notes logout + restart does not fix the symptom, which weakens
  the "client cache" theory — though it's still possible if the persistent
  cache survives logout.
- **Effort:** Small.

**Option C3: Re-read media for each community in the list response
server-side, defeating any stale projection.**
- **Approach:** In `ListCommunities`, after `GetByIDs`, additionally fetch
  the media row for `community.MediaIds[0]` and verify it exists / is owned
  by that community.
- **Pros:** Defensive.
- **Cons:** Doubles the query count for a hot path. Doesn't actually fix the
  root cause.
- **Effort:** Small.

## Recommendation

Take all three bugs together but ship them as **independent commits / PRs**
under this umbrella:

1. **Bug B first.** It's the smallest diff, the fix is mechanical, and it
   touches `community_avatar.dart` only. Validates the umbrella approach
   without requiring diagnosis.
2. **Bug A second.** Repro is reliable per the issue. The fix surface depends
   on what logcat shows but is bounded by the two providers feeding
   `CommunityPublicScreen`.
3. **Bug C last.** Requires diagnostic logging, a repro pass, then a fix on
   whichever side is wrong. Highest risk of expanding scope, so isolating it
   to its own PR keeps Bugs A/B unblocked.

If Bug C's server side turns out to be the culprit and the fix is non-trivial
(e.g., a missing event hook from media-generation back to the list cache),
split it into its own sub-issue at that point. Don't pre-split.

## Open Questions

- **Bug A — which future hangs, the community fetch or the impact fetch?**
  Recommendation: Repro on a device, capture logcat, and inspect both
  providers' transitions. Don't guess. If we have to guess: bet on
  `communityImpactProvider` (the issue hypothesis matches a common
  server-side authorization-and-silently-drop pattern).
- **Bug A — should non-members see impact metrics for any community?**
  Recommendation: Yes for the metrics that already power the home-screen
  pitch (cost saved, problems solved, time saved). These are aggregate
  numbers, not PII. If product wants to gate any specific metric, gate at
  the field level on the server, not by hanging the response.
- **Bug B — should we keep the placeholder-during-load behavior at all?**
  Recommendation: Yes, but only show it on the **first** load for a
  `mediaId`, not on every rebuild. Switching to `mediaUrlProvider` gives us
  this for free — Riverpod's cached `AsyncValue` returns `data` immediately
  on subsequent watches.
- **Bug C — is `CommunityRepository`'s cache persistent across logout?**
  Recommendation: **No verification needed — already answered.** Per
  `docs/client/caching.md` the StashCacheManager Phase 1 implementation is
  memory-only and does not persist across restarts. Logout-and-relaunch
  guarantees a cold cache. So the bug is server-side (or in the
  create-community client flow that never re-fetches the list at all,
  which is also a server-shape symptom).
- **Bug C — does the community-create flow publish a "media ready" event
  that the client uses to invalidate the list?** Recommendation: Audit
  `StreamGenCommunityResponse_MediaReady` handling on the client. If the
  client doesn't react to it by invalidating the user-list cache, that's
  the root cause regardless of server behavior.

## Implementation Plan

### Phase 1: Bug B fix (FutureBuilder antipattern)
**Goal:** Stop the carousel avatar flash on every selection by removing the
re-created `Future` from `CommunityAvatar`.
**Tasks:**
- [ ] In `app/lib/presentation/widgets/community_avatar.dart:53-70`, replace
      the inline `FutureBuilder<String>` with
      `ref.watch(mediaUrlProvider(mediaId))` and an `AsyncValue.when` that
      maps `loading`/`error` to `_buildPlaceholder` and `data` to
      `_buildImageAvatar`. Keep the `cacheKey: ImageCacheKeys.thumbnail(mediaId)`
      argument on `CachedNetworkImageProvider` — image-byte caching is
      separate from `mediaUrlProvider`'s response cache
      (see `docs/client/caching.md` "Image Cache Keys").
- [ ] Confirm no other call sites depend on the `FutureBuilder` semantics.
- [ ] Manual verify on Workshop carousel selection (no flash) and on
      manage-members member list (same widget; same fix should apply).
- [ ] **Test layer note (per `docs/client/testing.md`):** preferred layer is
      ViewModel tests, but this bug is specifically widget rebuild behavior,
      so a `flutter_test` widget-test is the right layer. Use a
      `ProviderContainer` with `mediaUrlProvider(mediaId)` overridden to a
      controllable future and assert: after the first frame settles, a
      second rebuild does **not** transition the visible subtree back to the
      placeholder. Run with `flutter test --reporter expanded` per
      `CLAUDE.md`.
- [ ] `npm run test:app` and `flutter analyze`. Also run
      `npm run lint:dart:a11y` and `npm run lint:dart:i18n` — no new
      user-visible strings or interactive primitives are introduced, so
      these should pass without changes.

**Validates:** That the broader Riverpod provider pattern is the right
replacement for FutureBuilder in this codebase. Sets the precedent for other
similar widgets if any exist.

### Phase 2: Bug A fix (spinning community public screen)
**Goal:** Make `CommunityPublicScreen` resolve to data or error for any
viewer who can navigate to it.
**Tasks:**
- [ ] Repro Bug A on a device. Capture logcat with the
      `CommunityImpactMetricsNotifier` and `getCommunityProvider` paths
      logged.
- [ ] Identify which future never completes. Read the corresponding
      notifier / RPC handler.
- [ ] **If server-side** (per `docs/server/observability.md`): the handler
      must return either concrete data or a typed Connect error — never
      hang. For unexpected internal failures use
      `connecterr.Internal(ctx, "Operation", err)` (see `CLAUDE.md` — bare
      `connect.NewError(connect.CodeInternal, err)` is gated by
      `scripts/check_codeinternal.js`). Authorization failures use
      `connect.CodeUnauthenticated` / `CodePermissionDenied` as
      appropriate. Add an integration-style test exercising the
      non-member path (no need for the `integration` build tag unless
      live AI calls are involved).
- [ ] **If client-side:** fix the notifier to transition to `AsyncError`
      on the faulty path. Per `docs/client/architecture.md` "Async Safety
      in Notifiers", use `AsyncValue.guard()` and gate `state = ...`
      behind `if (!ref.mounted) return;` after each async gap. Add a
      ViewModel-layer unit test using `ProviderContainer` with mocked
      repositories (per `docs/client/testing.md` §1 "ViewModel Testing").
- [ ] **Error surface text must come from `context.l10n`** per
      `docs/client/i18n.md`. The current
      `community_public_screen.dart:99-115` already shows a hard-coded
      `'Failed to load community'`; if Bug A surfaces a new error path,
      either reuse `context.l10n.commonRetry` (already in use at line
      113) plus an existing error key or add a new `community*` key in
      `app_en.arb` + `app_es.arb` and run `npm run lint:dart:l10n-parity`.
- [ ] Manual verify: tap a community link from a non-member's profile;
      screen renders within a few seconds.

**Validates:** That the public community screen actually works for the
audience it exists for (non-members).

### Phase 3: Bug C diagnosis
**Goal:** Establish, with logs in hand, which side returns the stale
`mediaIds[0]`. (Given the client cache is memory-only and logout+restart
doesn't fix the bug, the server is the prime suspect — but log both sides
so the asymmetry is provable.)
**Tasks:**
- [ ] **Server logging (per `docs/server/observability.md`):**
      add structured logs in `server/services/community/membership.go`
      (`ListCommunities`, around lines 184-191) using
      `logger := logging.LoggerWithContext(ctx).With("operation", "ListCommunities", ...)`
      and emit one `DebugContext` entry per item with the standard field
      `community_id` and a `media_ids` slice. `Debug` is the right level
      per the observability doc ("verbose operational details… read
      operations"). Do **not** introduce a new request-correlation field;
      `request_id` is already populated by middleware and `user_id` /
      `user_email` (masked via `logging.MaskEmail`) come from the existing
      `LoggerWithContext` derivation.
- [ ] Add a matching `DebugContext` log in `GetCommunity` in
      `server/services/community/lifecycle.go` with `"operation", "GetCommunity"`
      and the same `community_id` / `media_ids` fields.
- [ ] Add a client log in
      `app/lib/data/repositories/community_repository.dart` `get()` and
      `listUserCommunities()` at `Logger.info` printing
      `community.mediaIds` for the newly-created community id (reuse the
      existing `_log = Logger('CommunityRepository')` at line 9).
- [ ] Run the repro from the issue. Capture both responses for the same
      community id. Correlate by `request_id` from `X-Request-ID` headers
      so the two server-side log lines tie back to the exact client calls.
- [ ] Document the divergence in this plan (update under "Bug C diagnosis
      result"). Suspected root causes to weigh:
      - **Storage read-after-write skew** in `GetByIDs` vs. single-row
        `Get` (e.g., different read paths bypassing a cache or hitting a
        replica). Inspect `server/storage/protosql_batch.go:25` vs. the
        single-row reader used by `GetCommunity`.
      - **Media generation completes but does not update the row used by
        `ListCommunities`** (e.g., it writes to a denormalized projection
        that `GetCommunity` reads but `ListCommunities` does not, or
        vice-versa). The streaming-gen flow in
        `server/services/community/gen.go` and the `MediaReady` event are
        the prime suspects.
      - **A stale projection or materialized view fed by `ListCommunities`'s
        sort path** (`sortCommunitiesByActivity` at
        `membership.go:204+`).

**Validates:** Which side actually returns the wrong `mediaIds`. Without
this step we are guessing.

### Phase 4: Bug C fix
**Goal:** Newly-created communities show their correct AI-generated
thumbnail in the carousel within one refresh cycle of image generation
completing.
**Tasks:**
- [ ] **If client-cache** (least likely given memory-only cache + restart
      repro, but include for completeness): hook the
      `StreamGenCommunityResponse_MediaReady` / `_Final` event in the
      create-community flow to `CommunityRepository.refreshUserCommunities()`
      (already exists at `community_repository.dart:110-121`). Per
      `docs/client/caching.md` Pattern 6/7, prefer event-driven
      invalidation over manual cache clears.
- [ ] **If server-side stale projection / duplicated `mediaIds`:** locate
      the writer (likely a path that creates the `Community` row at
      `StreamGenCommunity` start time and a separate path that completes
      media generation later). Patch the completion writer so the row
      that `ListCommunities` reads reflects the post-generation
      `mediaIds`. Per `docs/server/architecture.md` §"Service
      Independence", factor any shared logic into a library rather than
      service-to-service calls.
- [ ] **If server-side ordering / join bug** in `ListCommunities`: fix
      the loop/join so each `api.CommunityItem` is paired with its own
      row. Per SQL Efficiency rules in `CLAUDE.md`, prefer JOIN over
      multiple queries and avoid N+1 patterns.
- [ ] Add a regression test pinned to whichever side carried the bug. For
      server-side, use `storage.SetupTestStorage(t)` with the
      `pgvector/pgvector:pg16` testcontainer pattern. For client-side,
      a ViewModel test using `ProviderContainer` with a mocked
      `CommunityRepository` and `CommunityService`.
- [ ] Manual verify the original repro on a clean device.
- [ ] Remove the diagnostic `Debug`-level logs from Phase 3 unless they
      are broadly useful (low log-volume `Debug` calls are cheap to leave
      in place per `docs/server/observability.md` level guidance, but
      churn-only log lines should be pulled).

**Validates:** End-to-end correctness of the new-community thumbnail flow
in the carousel.

## Related Issues
- #2041 — the PR whose manual test pass surfaced these bugs; explicitly not a
  regression cause for any of them.
- #2040 — orthogonal follow-up for `TODO(#2040)` markers from #2041; mentioned
  for context only.
- **#1976 — Bug C is deferred to ride along with this issue.** The
  community-create flow is currently broken (Generate button stays
  disabled after entering text), so the Bug C repro cannot be exercised
  against the new diagnostic logs yet. Once #1976 lands a working create
  flow, the diagnostic logs already merged in #2046 will surface the
  `ListCommunities` vs `GetCommunity` `media_ids` divergence in a single
  repro pass.

---

## Audit notes (added by `/ripls-audit` on 2026-05-19)

Reviewed against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/client/accessibility.md`,
`docs/client/testing.md`, `docs/server/architecture.md`, and
`docs/server/observability.md`.

Changes applied to the plan:
- **Cache persistence claim corrected.** The original Bug C analysis
  hedged on whether `CommunityRepository`'s cache survives logout.
  `docs/client/caching.md` is explicit: StashCacheManager Phase 1 is
  memory-only and does not survive app restarts. The bug therefore must
  be server-side (or a missing server-shape refresh after media gen
  completes). The Open Question was updated to reflect this.
- **Bug B test layer clarified.** `docs/client/testing.md` says
  ViewModel-first, but this specific bug is widget rebuild behavior — so
  a `flutter_test` widget test with a `ProviderContainer` override of
  `mediaUrlProvider` is the right layer. Added `--reporter expanded`
  reminder per `CLAUDE.md`.
- **Bug B architecture note added.** The architecture doc says widgets
  shouldn't access repositories directly, but `FutureProvider.family`
  wrappers (`mediaUrlProvider`, `mediaObjectProvider`, `getCommunityProvider`)
  are the established codebase pattern for leaf data loads. The plan now
  documents this choice rather than leaving it implicit.
- **Bug B image cache key preserved.** Per `docs/client/caching.md`
  "Image Cache Keys", the `CachedNetworkImageProvider`'s `cacheKey:
  ImageCacheKeys.thumbnail(mediaId)` is a *separate* cache (disk-backed,
  managed by `flutter_cache_manager`) from `mediaUrlProvider`'s response
  cache. Explicitly called out so the refactor doesn't accidentally drop
  the cacheKey.
- **Bug A error handling tightened.** Added: client-side fixes must use
  `AsyncValue.guard()` + `ref.mounted` guards per the async-safety
  section of `docs/client/architecture.md`; server-side fixes must use
  `connecterr.Internal` (bare `connect.NewError(connect.CodeInternal,
  err)` is gated by CI per `CLAUDE.md`); error text must come from
  `context.l10n` per `docs/client/i18n.md`.
- **Bug C server logging conventions added.** Plan now specifies
  `logging.LoggerWithContext(ctx).With("operation", ...)`, the
  standard field names `community_id` / `media_ids`, and `Debug` level
  per `docs/server/observability.md`. Explicitly noted that
  `request_id`, `user_id`, and masked `user_email` come for free from
  `LoggerWithContext` derivation — don't re-add them.
- **Bug C server-fix scope expanded.** Added three concrete suspect
  paths (`GetByIDs` vs single-row read, `MediaReady` not updating the
  list-side row, stale projection in `sortCommunitiesByActivity`)
  drawn from the actual server file layout.
- **Accessibility not impacted.** No new interactive widgets or
  user-visible images are added; existing `CachedMediaImage` semantics
  labels in `user_profile_identity.dart:109-110` are preserved. The a11y
  primitive guide in `docs/client/accessibility.md` does not require
  changes for these fixes.
- **i18n CI gates referenced.** Bug A's error-text change must run
  `npm run lint:dart:l10n-parity` and `npm run lint:dart:i18n` per
  `docs/client/i18n.md`.
- **No proto changes implied.** Per `CLAUDE.md` proto conventions —
  none of the three fixes change the API surface, so no
  request/response message split is needed.
