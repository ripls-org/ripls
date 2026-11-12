---
issue: 1717
title: Phase 3 of #1619: Settings → Communities deleted-list + Restore screen
---

# Issue #1717: Phase 3 of #1619: Settings → Communities deleted-list + Restore screen

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1717
**Filed:** 2026-05-04 | **Last activity:** 2026-05-04
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/community_delete_and_leave.md (§§ 2.5, 7.1, 7.4, 7.5)

## Audit findings (2026-05-05)

Reviewed against `docs/client/{architecture,caching,i18n,accessibility,testing,deep_linking}.md`
and `docs/server/{architecture,observability}.md`. Changes applied below:

1. **Navigation primitive corrected.** Original draft proposed
   `context.go('/settings/communities/<id>/restore')` for in-app taps.
   `docs/client/architecture.md` "Navigation & Gestures" forbids this:
   GoRouter is reserved for auth flows in `login_screen.dart`. In-app
   navigation must use `NavigationHelpers`. The Restore screen is a
   settings/utility screen → use `NavigationHelpers.pushWithSlide()`. The
   GoRoute is **deep-link-only** — it exists so the push notification's
   tap-action lands on the same screen, but the in-app tap from the
   deleted-list does not route through it. **Do not** add
   `SwipeToCloseMixin` (it's a utility screen, not a primary content
   screen — per the same doc).

2. **Error handling sharpened.** Original draft said "map
   `ConnectException` codes via `RpcUtils` or `ServiceException`."
   Replaced with the canonical `RpcErrorHandler.classify(e)` →
   `UserError` pattern documented in `docs/client/i18n.md` and
   `app/lib/core/errors/README.md`. But the issue's three outcome
   strings ("Already restored", "You can no longer restore this
   community", "Restored {name}") are not generic RPC errors —
   they're *outcome-specific* messaging on top of `FailedPrecondition`
   / `PermissionDenied`. So the ViewModel exposes a typed `RestoreOutcome`
   enum (sealed class with `success | alreadyRestored | noLongerEligible
   | error(UserError)`); the widget switches on the outcome to pick the
   right localised snackbar key. Generic RPC errors fall through to
   `error(UserError)` and get localised via `RpcErrorHandler.localize`.

3. **AsyncNotifier explicit.** `docs/client/architecture.md` says
   `AsyncNotifier` is the preferred pattern over manual `ref.mounted`
   checks. Both new ViewModels use `AsyncNotifierProvider.autoDispose`
   (precedent: `community_loans_view_model.dart`). Disposal tests are
   required per the same doc — added to test tasks.

4. **Accessibility primitives required.** Original draft said "tap
   navigates" without naming the widget. Per `docs/client/accessibility.md`:
   - Deleted-list rows use `Tappable` with a `semanticsLabel` from
     `context.l10n` (key: `a11yRestoreCommunityRow` → "Restore {name},
     {daysLeft, plural, ...}"). Lint `avoid_raw_gesture_detector` would
     reject `InkWell`/`GestureDetector`.
   - Restore screen CTA uses `Tappable` (or the codebase's standard
     primary-CTA wrapper, whichever is in use on adjacent screens —
     verify during Phase 3).
   - Countdown badge: per `avoid_color_only_status`, a class named
     `*Badge` must contain `Semantics(` or `Tooltip(`. Wrap the badge in
     `Semantics(label: ...)` matching the visible text — and verify
     urgency is conveyed by *text*, not color alone.
   - Success/error feedback uses `SemanticAnnouncer.announce(context, msg)`
     **alongside** the snackbar so screen reader users hear the outcome.

5. **i18n: both locales required up-front.** `docs/client/i18n.md` is
   explicit: "Each new string needs both locales. Add the key to
   `app_en.arb` (with `@key` metadata) and `app_es.arb` before running
   `npm run generate`." Removed the "punt Spanish to a follow-up" framing —
   plan now adds best-effort Spanish for every new key in the same PR,
   with a follow-up only if a translator review pass is desired
   afterwards.

6. **i18n key prefix consistency.** Standardised all new keys to the
   `community*` prefix per the convention table — `communityDeletedCountdownBadge`,
   `communityRestoreTitle`, `communityRestoreBody`, `communityRestoreCta`,
   `communityRestoreSuccess`, `communityRestoreAlreadyRestored`,
   `communityRestoreNoLongerEligible`, `communityRestoreDeletedByLine`.
   `settingsRecentlyDeletedHeader` keeps `settings*` because it's the
   header on the settings hub, not on a community surface.

7. **ARB metadata required.** Each new key must include the `@key`
   metadata block (description and placeholder declarations for
   ICU/named params). Added to ARB tasks.

8. **Section header nuance.** Verified against §7.1 of the spec: it says
   "Add a section (or filter toggle) **below the active-communities
   list**." Renders only when non-empty (Open Question 1 stays).

9. **Server-boundary reminder.** The two new RPCs, both server-paged
   (`limit = 100` server-side), already mask user IDs via
   `logging.MaskEmail` in `server/services/community/membership.go`. No
   client work needed for observability — the client just needs to
   pass the standard `X-Request-ID` header (which `_buildHeaders()` in
   `community_service.dart` already does).

## Problem Statement

Phase 2 of #1619 shipped the server-side soft-delete + 30-day-restore lifecycle
(`DeleteCommunity`, `RestoreCommunity`, snapshot-based eligibility, day-before-purge
push). #1722 — server-side `ListDeletedCommunitiesForRestore` RPC — is in PR
#1724 alongside #1723. The client side has none of it: there is no surface where
a user can see a community they (or another member) deleted, no way to discover
the 30-day window, and no Restore screen for the day-before-purge push to deep-link
to. Until this lands, `RestoreCommunity` is reachable only through that single
push deep-link (and only for the 24h before purge), which means a user who
notices the delete on day 2 has no in-app way to undo it.

This issue covers two surfaces:
1. **Settings → Communities deleted-list** (§7.1): below the active-communities
   list, an entry per restorable community with a "Deleted, X days left" badge
   and a tap → Restore screen.
2. **Restore screen** (§7.4): a confirmation screen at
   `/settings/communities/<id>/restore` with a single "Restore Community" CTA
   that calls `RestoreCommunity`, handles the documented error cases, invalidates
   the per-§7.5 cache fan-out, and navigates to the community on success.

## Context from Comments

One comment from briangbrown (2026-05-04, the day after filing):

> Confirmed via proto check: `ListCommunities` is membership-scoped and filters
> out soft-deleted communities — there is no flag to surface what this client
> surface needs. The required server-side RPC is filed as #1722
> (`ListDeletedCommunitiesForRestore`). #1717 now hard-depends on #1722; merge
> that first.

Translated: the body's "if a new RPC is needed, file that as a separate small
server-side issue first" branch is the live one. #1722 / #1723 ship in PR #1724;
this issue must wait for that PR to merge before it can pull the generated
`ListDeletedCommunitiesForRestore` symbols. (Branching strategy: cut this PR's
branch off `phase3-list-rpcs` so the new client wiring can use the new RPC types
during development, then rebase to `main` once #1724 lands.)

No other comments. The issue body is detailed enough that the Open Questions
section is small.

## Codebase reconnaissance

What already exists (relevant to this work):

- **Generated Dart** for `RestoreCommunity` and `ListDeletedCommunitiesForRestore`
  is on the `phase3-list-rpcs` branch (`app/lib/data/gen/ripls/api/community_service.{pb,connect.spec}.dart`).
- **`CommunityService`** (`app/lib/services/community_service.dart`, 680 lines)
  has `deleteCommunity`. It does **not** yet have `restoreCommunity` or
  `listDeletedCommunitiesForRestore`. Both will follow the existing
  `RpcUtils.executeRpc` + `_buildHeaders()` + `onUnauthenticated` pattern.
- **`CommunityRepository`** (`app/lib/data/repositories/community_repository.dart`,
  569 lines) has `deleteCommunity(id)` that invalidates the community cache and
  refreshes the user-communities list. It already composes `SearchRepository`,
  `MediaRepository`, `FeedRepository` and exposes `invalidateCommunity`,
  `refreshCommunityUsers`, `refreshCommunityGear`, etc. — the right hooks for the
  §7.5 fan-out are present.
- **`CommunityRepository`** does **not** yet depend on `GearRepository`,
  `RequestRepository`, `ExperienceRepository`, `TransferRepository`, `ChatRepository`,
  `StoryRepository`. The §7.5 list crosses repositories — the cleanest pattern
  is to invalidate via the existing repos at the call-site (the ViewModel that
  triggers Restore), not by giving `CommunityRepository` a fat new dependency
  graph.
- **`SettingsHubScreen`** (`app/lib/presentation/screens/settings/settings_hub_screen.dart`,
  184 lines) is a `ConsumerStatefulWidget` that watches `selectedCommunityProvider`
  and renders an active-communities section. Deleted-list belongs here as a new
  section below the active list (§7.1: "Hide deleted communities from every
  other list…only this surface should render them.").
- **Routing**: `app/lib/core/router/app_router.dart` uses go_router but
  `SettingsHubScreen` is reached **imperatively** (`Navigator.push` from the
  sidebar at `app/lib/presentation/screens/home/sidebar.dart:455`). Today there
  is no `/settings/...` GoRoute. The day-before-purge push deep-link target
  (`/settings/communities/<id>/restore`) needs a GoRoute. **Two options:** (a)
  add only the leaf restore route as a GoRoute; (b) move the whole settings
  subtree to GoRoute. (a) is sufficient for this issue.
- **Test patterns**: `app/test/data/repositories/community_repository_test.dart`
  + Mockito mocks; ViewModel tests under `app/test/presentation/viewmodels/`
  use `ProviderContainer`. Pattern is well-established.
- **ARB**: `app/lib/l10n/app_en.arb` and `app_es.arb`. New keys needed for
  badge, screen body, snackbars.
- **Proto** (`proto/ripls/api/community_service.proto`):
  - `ListDeletedCommunitiesForRestoreRequest` is empty (no params; server
    derives caller from auth).
  - `DeletedCommunityItem`: id, name, description, media_ids, deleted_at_unix_sec,
    deleted_by_user_id (optional, for "Deleted by …" byline). Server returns
    items ordered ASC by deleted_at — oldest deletion first (urgency-sorted).
  - `RestoreCommunityRequest`: community_id only.
  - `RestoreCommunityResponse`: empty.

## Potential Solutions

The shape of the work is largely fixed by the issue body and §7. The choices
worth calling out are about *where* the deleted-list belongs and *who* owns
cache invalidation.

### Option A: deleted-list as a new section in `SettingsHubScreen`, viewmodel-driven; cache invalidation orchestrated in the Restore ViewModel

- **Approach:** Add a `DeletedCommunitiesViewModel` (Riverpod
  `AsyncNotifierProvider`) that the existing `SettingsHubScreen` watches alongside
  `selectedCommunityProvider`. Render a new `_buildDeletedSection` below
  `_buildCommunitiesSection`. Tap navigates to a new `RestoreCommunityScreen`
  via go_router (so the deep link works). The Restore ViewModel owns the
  RPC call, success/error handling, and the per-§7.5 cache invalidation
  fan-out — by invoking each repository's existing `invalidate*` /
  `refresh*` methods. `CommunityRepository` gets a thin
  `listDeletedCommunitiesForRestore()` and `restoreCommunity(id)` pair; both
  invalidate the deleted-list cache and the community row.
- **Pros:** Mirrors existing patterns (one ViewModel per surface, repos own
  caches but ViewModels orchestrate cross-repo invalidation — same shape as
  `shareGear` callers today). Smallest change to `SettingsHubScreen`. Restore
  screen reachable both by tap and by deep link via the same route.
- **Cons:** Cache fan-out at the call-site means six or seven `invalidate*`
  calls in one ViewModel method — verbose. Works fine but reads like a list.
- **Effort:** Medium. ~3 new files (ViewModel, screen, deleted-section widget),
  ~2 edited (SettingsHubScreen, CommunityRepository), ~1 router edit, ARB
  additions, tests.

### Option B: same as A, but add a `restoreCommunity` method on `CommunityRepository` that internally calls every other repo's invalidate

- **Approach:** Wire `CommunityRepository` to depend on `GearRepository`,
  `RequestRepository`, `ExperienceRepository`, `TransferRepository`,
  `ChatRepository`, `StoryRepository`, `FeedRepository`. `restoreCommunity`
  fans out to each.
- **Pros:** Single call-site does everything. ViewModel stays thin.
- **Cons:** Violates the "shallow, one-way dependencies, max 1 level deep"
  rule from CLAUDE.md client-caching guidelines. `CommunityRepository` already
  composes `SearchRepository`, `MediaRepository`, `FeedRepository`; doubling
  that to seven dependencies starts to feel like a service layer pretending
  to be a repo. More importantly, this makes `CommunityRepository` impossible
  to instantiate in any test that doesn't want the whole tree.
- **Effort:** Medium-high (more provider wiring, more mocks).

### Option C: minimal — deleted-list and Restore as a separate `DeletedCommunitiesScreen`, not embedded in Settings hub

- **Approach:** Add a new top-level screen that contains only the deleted
  list, reached from a new "Recently deleted" entry inside Settings. Restore
  screen is unchanged from A.
- **Pros:** Fewer changes to the existing settings UI; isolated.
- **Cons:** Diverges from §7.1 ("Add a section (or filter toggle) **below
  the active-communities list**…"). Adds an extra hop. Bad for discovery
  (users would have to know to look for the entry).
- **Effort:** Small-medium. Skipped.

## Recommendation

**Option A.** It matches §7.1 verbatim, follows the existing
ViewModel-orchestrates-invalidation pattern (cf. `shareGear` invalidating
`searchRepository` and `feedRepository` from the same call), and keeps
`CommunityRepository`'s dependency graph at its current depth.

The only concession to verbosity: the cache fan-out in
`RestoreCommunityViewModel.restore()` will be ~7 lines of `await
ref.read(...).invalidate*(...)` calls. That's fine — it's explicit and matches
how the rest of the codebase handles cross-repo invalidation today.

For the deep-link target, add a single GoRoute for
`/settings/communities/:id/restore` (Option (a) above). The GoRoute is the
*push-deep-link entry only*: when the FCM tap-action fires, the router
opens the Restore screen. **In-app taps go through `NavigationHelpers.pushWithSlide()`**
to the same screen widget — `context.go(...)` is forbidden for in-app
navigation per `docs/client/architecture.md`. The settings hub itself
stays imperative.

ViewModels: both are `AsyncNotifierProvider.autoDispose`. Restore exposes
a `RestoreOutcome` sealed class (`success | alreadyRestored |
noLongerEligible | error(UserError)`) so widgets can pick the right
snackbar key without inspecting RPC codes themselves. Errors that fall
through to `error(UserError)` come from `RpcErrorHandler.classify(e)` and
render via `RpcErrorHandler.localize(error, context.l10n)`.

## Open Questions

- **Empty-state rendering — section header visible or fully hidden?**
  Recommendation: **fully hidden**. §7.1 says "no badge / no section if the
  user has no eligible deleted communities." Render the section only when
  the deleted-list ViewModel has a non-empty result. Saves vertical real
  estate for the 99% case where users have no deleted communities.

- **Loading state for deleted list — show spinner or render nothing while
  fetching?** Recommendation: **render nothing** during the first fetch.
  The list is a non-critical surface; flashing a spinner below the active
  list is jarring and the user wasn't looking for it. If the fetch errors,
  log via `_log.warning` and render nothing — do not surface the error in
  the hub. (A pull-to-refresh on the hub itself can re-trigger the fetch
  and surface real errors.) Open to "spinner if active list is also still
  loading" if the visual reads better in practice — decide during widget
  review.

- **Cache strategy for the deleted list.** Recommendation: cache it under
  `community:deleted:list` with a short TTL (e.g., 60s) to absorb hub
  re-mounts during a session, but invalidate it eagerly after Restore
  succeeds and after `DeleteCommunity` succeeds (so a user who deletes a
  community sees it appear in the deleted-list when they open Settings
  next). The cache key parallels `community:user:list`.

- **Snackbar copy and i18n keys.** Recommendation: three new ARB keys —
  `communityRestoreSuccess` ("Restored {name}"),
  `communityRestoreAlreadyRestored` ("Already restored"),
  `communityRestoreNoLongerEligible` ("You can no longer restore this
  community"). All English strings exactly as the issue body specifies.
  **Per `docs/client/i18n.md`, both locales are required in the same PR**
  — add best-effort Spanish for every new key alongside English, with
  `@key` metadata blocks declaring placeholders. A follow-up
  translator-review pass can refine the Spanish; the strings cannot
  ship English-only. Issue #1716's pending localisation work covers
  the five push strings only — this issue's keys are independent.

- **Countdown badge wording across boundaries.** Recommendation: render
  via a single ICU `plural` ARB key —
  `communityDeletedCountdownBadge: "{daysLeft, plural, =0{Deletes today}
  =1{Deletes tomorrow} other{Deleted, # days left}}"`. The 0-day edge is
  semantically a "deletes today" — the issue's literal "0 days left"
  reads wrong on the day of purge. Compute `daysLeft = max(0,
  ceil((deletedAt + 30d - now) / 1d))`. Use `ceil` so day 29.4 still
  reads "1 day left" (truthy, urgent). Confirm with widget golden tests
  at the boundaries (issue acceptance lists 1 day, 30 days, 0 days).
  The badge widget must wrap its visible label in `Semantics(label: ...)`
  matching the resolved ICU string — per the
  `avoid_color_only_status` lint rule, any `*Badge` class needs an
  explicit semantics or tooltip annotation.

- **`MediaRepository` integration for cover art.** Recommendation: reuse
  `CommunityAvatar` from `app/lib/presentation/widgets/community_avatar.dart`
  — the existing settings hub already uses it. It accepts a `CommunityItem`,
  not a `DeletedCommunityItem`, so either adapt the widget to accept either
  type (preferred — its ID + media-IDs surface is identical) or convert
  `DeletedCommunityItem` → an internal `CommunityItem` shape at the boundary.
  Adapt the widget — fewer types, same render path.

- **Should the Restore screen surface the "Deleted by …" byline?**
  Recommendation: **yes, when present**. The `deleted_by_user_id` field
  exists for exactly this. Render under the body text as
  `Deleted by <name> on <date>`. If `deleted_by_user_id` is empty, omit
  the line. Names come from `UserRepository.get(id)` — wrap in a
  best-effort fetch; if it fails, fall back to the date-only line.

- **Does cache invalidation include the leaderboard, impact, or portfolio
  repos?** Recommendation: **yes for leaderboard and impact** (community
  appears in those rollups), **no for portfolio** (portfolio is per-user, not
  per-community). Confirm during implementation by grepping the leaderboard
  and impact repos for `communityId` parameters; if they cache by community
  ID, invalidate. Cheap to do, cheap to skip when wrong.

## Implementation Plan

### Phase 1: service + repository wiring (no UI yet)

**Goal:** `CommunityService` and `CommunityRepository` expose the two new
operations, with caching for the deleted list and unit-test coverage.

**Tasks:**
- [ ] `community_service.dart`: add `restoreCommunity(String communityId)`
  and `listDeletedCommunitiesForRestore()`. Both follow the
  `RpcUtils.executeRpc` pattern with `_buildHeaders()` (which already
  emits `X-Request-ID` for server-log correlation); the latter returns
  `List<DeletedCommunityItem>`.
- [ ] `community_repository.dart`: add `restoreCommunity(String communityId)`,
  `listDeletedCommunitiesForRestore({bool refresh = false})`, and
  `invalidateDeletedList()` (called from `restoreCommunity` and
  `deleteCommunity` so the section updates after both lifecycle
  changes). Cache key `'deleted:list'` under the existing `'community'`
  namespace.
- [ ] Repository test: cache hit/miss on `listDeletedCommunitiesForRestore`,
  invalidation on `restoreCommunity` succeeds + on `deleteCommunity`
  succeeds, `restoreCommunity` failure does not invalidate (preserves
  snapshot — explicit assertion). Mockito mocks for `CommunityService`.
  Mirrors `community_repository_test.dart`.

**Validates:** the new RPCs round-trip end-to-end against the server (via
the next phase's run-against-real-server smoke), and the cache contract is
correct in isolation. No UI yet, so this phase can ship behind a hidden
provider if needed for incremental review.

### Phase 2: deleted-list section in Settings hub

**Goal:** users see deleted communities in the right place, with the
correct empty-state and countdown rendering. No Restore action yet — tap
is a no-op so the surface is reviewable in isolation.

**Tasks:**
- [ ] New ViewModel `deleted_communities_view_model.dart` —
  `AsyncNotifierProvider.autoDispose<DeletedCommunitiesNotifier,
  List<DeletedCommunityItem>>`. `build()` returns the future from
  `CommunityRepository.listDeletedCommunitiesForRestore()` so disposal
  is auto-handled. Refresh method uses `AsyncValue.guard` and
  `ref.mounted` checks per the AsyncNotifier pattern.
- [ ] New widget `deleted_communities_section.dart` under
  `presentation/screens/settings/widgets/`. Renders the section
  header + a list of `_buildDeletedItem` rows reusing
  `CommunityAvatar`. Each row uses **`Tappable`** with a localized
  `semanticsLabel` (key: `a11yRestoreCommunityRow`) — never raw
  `InkWell`/`GestureDetector`. Tap calls
  `NavigationHelpers.pushWithSlide(context, RestoreCommunityScreen(...))`.
  Returns `SizedBox.shrink()` when the list is empty.
- [ ] New widget `deleted_countdown_badge.dart`. Wraps its visible
  label in `Semantics(label: <resolved ICU string>)` to satisfy the
  `avoid_color_only_status` lint rule. Urgency conveyed primarily
  through text, with color as a secondary signal.
- [ ] ARB additions (English + Spanish, with `@key` metadata):
  `settingsRecentlyDeletedHeader`, `communityDeletedCountdownBadge`
  (ICU plural — see Open Questions), `a11yRestoreCommunityRow`
  (semantics label with `{name}` and `{daysLeft}` placeholders).
- [ ] `settings_hub_screen.dart` edit: watch the new ViewModel via
  `.when(...)` for `AsyncValue` rendering, place the section below
  `_buildCommunitiesSection`. Loading/error states render
  `SizedBox.shrink()` (Open Question 2 — render nothing while
  fetching, surface errors only via pull-to-refresh).
- [ ] ViewModel test (`ProviderContainer` + Mockito): state populates
  from the repo, sorts ascending by `deletedAtUnixSec` (contract
  test), empty-state. Disposal-safety test required per
  `docs/client/architecture.md` (start refresh → dispose →
  expect no crash).
- [ ] Widget golden test for `deleted_communities_section.dart` at
  `daysLeft` ∈ {0, 1, 15, 30}. Use a fake clock (or pass `now` in
  via a parameter) so the badge is deterministic. Test with both
  `MaterialApp` `localizationsDelegates` set per
  `docs/client/i18n.md` and `app/test/helpers/l10n_helpers.dart`.

**Validates:** the surface renders correctly across the
"Deleted, X days left" / "Deletes tomorrow" / "Deletes today" boundaries,
the section disappears cleanly when empty, and we have the i18n hooks in
place.

### Phase 3: Restore screen + RPC + deep link + cache fan-out

**Goal:** tap or push-deep-link reaches the Restore screen; CTA calls
`RestoreCommunity` and invalidates the §7.5 cache fan-out.

**Tasks:**
- [ ] New screen `restore_community_screen.dart` under
  `presentation/screens/settings/`. **No `SwipeToCloseMixin`** —
  utility/settings screen, reached via `pushWithSlide()` per
  `docs/client/architecture.md`. Body matches §7.4 ("You'll become the
  new owner of {name}. Members will be re-added.") rendered from
  `communityRestoreBody`. Single primary CTA wired through `Tappable`
  (or the codebase's standard primary-CTA wrapper — verify against
  adjacent settings screens for consistency). `AppBarBackButton` for
  cancel. Optional "Deleted by … on …" byline rendered when
  `deleted_by_user_id` is non-empty and `UserRepository.get(id)`
  succeeds.
- [ ] New ViewModel `restore_community_view_model.dart` —
  `AsyncNotifierProvider.autoDispose`. Exposes a sealed
  `RestoreOutcome` (`success | alreadyRestored | noLongerEligible |
  error(UserError)`). `build()` returns `RestoreOutcome` (or a Freezed
  state wrapping it). `restore()` calls
  `CommunityRepository.restoreCommunity(id)`; on success runs the §7.5
  cache fan-out by reading each repo provider and calling its
  existing `invalidate*` / `refresh*` method:
  - `communityRepositoryProvider` → `invalidate(id)` +
    `refreshUserCommunities()` + `invalidateDeletedList()` (Phase 1
    helper).
  - `gearRepositoryProvider` → `invalidateForCommunity(id)` (or
    equivalent existing method — verify during impl).
  - `requestRepositoryProvider` → community-scoped invalidation.
  - `experienceRepositoryProvider` → community-scoped invalidation.
  - `transferRepositoryProvider` → community-scoped invalidation.
  - `chatRepositoryProvider` → conversations for community.
  - `feedRepositoryProvider` → `invalidateFeed()`.
  - `storyRepositoryProvider` → community-scoped invalidation.
  - `leaderboardRepositoryProvider` /
    `impactRepositoryProvider` → community-scoped if the repos cache
    by community; skip otherwise (decide during impl by grepping for
    `communityId` keys). Catch with `RpcErrorHandler.classify(e,
    fallback: ...)`; map `Code.failedPrecondition` →
    `RestoreOutcome.alreadyRestored`,
    `Code.permissionDenied` → `RestoreOutcome.noLongerEligible`, all
    other codes → `RestoreOutcome.error(...)`.
- [ ] Wire `_buildDeletedItem` tap →
  `NavigationHelpers.pushWithSlide(context,
  RestoreCommunityScreen(communityId: ..., name: ...))`. Pass enough
  state on the route arguments to render the screen offline (so the
  push deep-link can pre-render before the screen fetches the
  community details).
- [ ] `app_router.dart` edit: add `GoRoute(path:
  '/settings/communities/:id/restore', builder: ...)` **for the
  push-deep-link entry only**. The route builder reads the community
  ID from path params and constructs the screen; in-app taps do not
  go through this route.
- [ ] ARB additions (English + Spanish, with `@key` metadata):
  `communityRestoreTitle`, `communityRestoreBody` (with `{name}`),
  `communityRestoreCta`, `communityRestoreSuccess` (with `{name}`),
  `communityRestoreAlreadyRestored`, `communityRestoreNoLongerEligible`,
  `communityRestoreDeletedByLine` (with `{name}` and `{date}`).
- [ ] ViewModel test (`ProviderContainer` + Mockito): success path
  dispatches RPC, sets state to `RestoreOutcome.success`, calls each
  expected `invalidate*` once (verify via `verify(...)`).
  `FailedPrecondition` → `alreadyRestored`. `PermissionDenied` →
  `noLongerEligible`. Disposal test: start `restore()`, dispose
  container before completion, expect no crash.
- [ ] Widget test: cancel via `AppBarBackButton` returns to settings
  without firing RPC. CTA tap dispatches `restore()`. After success,
  `SemanticAnnouncer.announce` is called (asserted via spy or
  test-mode plumbing).
- [ ] Manual run: re-build dev server with `phase3-list-rpcs` + the
  client branch, soft-delete a community via the existing
  `deleteCommunity`, open Settings, tap Restore, confirm community
  reappears + cache fan-out clears stale gear/feed state. Confirm
  push deep-link by triggering the day-before-purge job locally and
  tapping the resulting notification.

**Validates:** the entire delete → discover → restore loop in-app, the
deep link (one of the two acceptance entry points), and the §7.5 cache
fan-out (the other thing this issue is uniquely positioned to verify).

## Related Issues

- #1619 — parent (community soft-delete with 30-day restore window).
- #1722 / PR #1724 — server-side `ListDeletedCommunitiesForRestore` RPC.
  Hard dependency for this issue's Phase 1.
- #1655 / PR #1706 — `RestoreCommunity` server side, merged.
- #1716 — day-before-purge push notification; deep-link target needs
  this issue's `/settings/communities/<id>/restore` route.
- #1718 — Danger Zone choice modal; orthogonal but adjacent (same
  feature, different surface).
- #1721 — Rejoin entry point; will piggyback on the repository/cache
  patterns established here.
- #1620 — daily hard-delete cron job (Phase 4); independent.
