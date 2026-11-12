---
issue: 1721
title: 'Phase 3 of #1619: Rejoin entry point + ''recently left'' surface'
---

# Issue #1721: Phase 3 of #1619: Rejoin entry point + 'recently left' surface

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1721
**Filed:** 2026-05-04 | **Last activity:** 2026-05-04
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/community_delete_and_leave.md (§§ 2.6, 2.7, 7.5)

## Audit findings (2026-05-05)

Reviewed against `docs/client/{architecture,caching,i18n,accessibility,testing}.md`
and `docs/server/{architecture,observability}.md`. Most of the plan already
mirrors the conventions established in #1717 / #1718 and survives review
intact. Changes applied below:

1. **Phase 1 test gap.** The original task list verified cache hit/miss and
   invalidation but didn't verify the new
   `rejoinableCommunitiesCacheInvalidationProvider` actually fires when
   `invalidateRejoinableList()` runs. Bug we caught testing #1717 (silent
   stale section). Added an explicit test that wires a counter into the
   provider and asserts it bumps on `rejoinCommunity` success and on
   `invalidateRejoinableList` directly.

2. **Phase 3 logging plumbing.** Original plan implied "mirror Restore VM"
   without spelling it out. Per `docs/server/observability.md` the request
   side is auto-correlated via `X-Request-ID` (already emitted by
   `RpcUtils.buildHeaders`). On the client, add explicit
   `Logger('RejoinCommunityViewModel')` plumbing — `_log.info` on success,
   `_log.info` on each typed `FailedPrecondition` outcome, `_log.warning`
   on the unrecognised-suffix branch, `_log.severe` on unexpected
   exceptions. Same shape as `RestoreCommunityViewModel`.

3. **Order of operations on success.** Original Phase 3 said
   "select the community in `selectedCommunityProvider`, navigate into the
   community" without specifying the order vs. the cache fan-out. The
   `selectCommunity(item)` call requires the rejoined community to be in
   `state.communities` — which only happens *after*
   `reloadCommunities()` runs in the fan-out. Made the order explicit:
   fan-out (including `reloadCommunities`) **first**, *then*
   `selectCommunity`, *then* navigate.

4. **`rejoinWindowExpired` "navigate to home".** Original plan said "pop to
   home." Per `docs/client/architecture.md`, `context.go(...)` is forbidden
   for in-app navigation (auth-flow-only). Replaced with
   `Navigator.of(context).popUntil((route) => route.isFirst)` — pops back
   through the stack until the first route, which is the home screen
   when Settings was opened from there. Belt-and-suspenders: also check
   `mounted` before each pop.

5. **`Code.unauthenticated` handling.** Not part of the four typed outcomes
   per the spec, and shouldn't be — `RpcUtils.executeRpc` already routes
   `Code.unauthenticated` to the `_onUnauthenticated` callback registered
   on `CommunityService` (logout + redirect to login). The VM's
   outcome-mapping never sees this code; documented as a comment in the
   VM so reviewers know it's handled at a different layer.

6. **ARB apostrophes.** The strings `"You're already a member"` and
   `"you'll start fresh"` contain literal apostrophes that ARB JSON
   requires to be unescaped *inside* simple strings, but for ICU plural
   strings the apostrophe needs to be doubled (`''`) to avoid being
   interpreted as a quoting delimiter. The Rejoin strings in this issue
   are simple (no ICU plural) except `communityRejoinCountdownBadge` —
   confirmed no apostrophes inside the plural body. Flagged here so
   the implementer doesn't trip over it.

7. **Section header naming consistency.** Original plan used
   `settingsRecentlyLeftHeader`. Standardised the "recently *X*" pattern
   alongside the existing `settingsRecentlyDeletedHeader` (#1717's key) —
   both keep the `settings*` prefix because they're hub-section headers,
   not community-domain strings.

8. **Cache key namespace.** Original `'rejoinable:list'` is correct —
   `CommunityRepository`'s namespace is `'community'` so the full key
   is `community:rejoinable:list`, parallel to
   `community:deleted:list`. Verified against `cache_service.dart`.

## Problem Statement

After #1656 (LeaveCommunity rewrite) and #1657 (RejoinCommunity) shipped server-side,
a leaver's `CommunityUser` row stays soft-deleted in the DB for 30 days, and the
server's `RejoinCommunity` RPC un-soft-deletes it on demand. There is no client
surface that lets the user **discover** they can rejoin, or **trigger** a rejoin —
the RPC is unreachable from the app.

This issue ships the missing surfaces: a "Recently Left" section in
**Settings → Communities** that lists communities the caller can rejoin, plus a
single-screen **Rejoin** confirmation that calls the RPC and handles the four
documented `FailedPrecondition` reasons. The pattern mirrors #1717's deleted-list
+ Restore screen exactly — same hub placement, same `AsyncNotifierProvider`,
same cache-invalidation signal.

## Context from Comments

One comment from briangbrown (2026-05-04):

> Confirmed via proto check: same gap as for #1717 — `ListCommunities` does not
> surface soft-deleted memberships in active communities. The required server-side
> RPC is filed as #1723 (`ListRejoinableCommunities`). **#1721 now hard-depends
> on #1723; merge that first.**

Status: **#1723 already merged via #1724.** Both `ListRejoinableCommunities` and
`RejoinCommunity` are on `main` and reachable through the generated Dart client.
No server prerequisite remains for this issue.

## Codebase reconnaissance

What already exists (relevant to this work):

- **API proto** (`proto/ripls/api/community_service.proto`):
  - `ListRejoinableCommunitiesRequest` empty (caller derived from auth) — line 151.
  - `ListRejoinableCommunitiesResponse` returns `repeated RejoinableCommunityItem
    communities` ordered descending by `left_at_unix_sec` (most-recently-left
    first) — line 153.
  - `RejoinableCommunityItem` has `id`, `name`, `description`, `media_ids`,
    `left_at_unix_sec` (the timestamp when the caller's CommunityUser was
    soft-deleted) — line 163.
  - `RejoinCommunityRequest` carries only `community_id` — already in proto.

- **Generated Dart client** (`app/lib/data/gen/ripls/api/community_service.connect.client.dart`):
  - `listRejoinableCommunities` — line 130.
  - `rejoinCommunity` — line 277.

- **Server implementation** (`server/services/community/lifecycle.go:817-928`):
  Returns `connect.CodeFailedPrecondition` for every negative path with a
  **discriminator suffix** in the error message: `"…: no_membership_to_rejoin"`,
  `"…: already_member"`, `"…: rejoin_window_expired"`, `"…: community_deleted"`.
  The client must parse the `e.message` suffix to disambiguate.

- **`CommunityService.dart`** does **not** yet have `rejoinCommunity` or
  `listRejoinableCommunities`. Both will follow the existing `RpcUtils.executeRpc`
  pattern — exactly the shape we used in #1717's Phase 1 for the
  `restoreCommunity` / `listDeletedCommunitiesForRestore` pair.

- **`CommunityRepository.dart`** does **not** yet have wrappers. Cache key
  `community:rejoinable:list` parallels `community:deleted:list`.
  `rejoinCommunity` invalidates that key, the community row, and refreshes
  user-communities (matching `restoreCommunity`'s shape).

- **`SettingsHubScreen`** already renders the deleted-list section we shipped in
  #1717 below the active-communities list. The new "Recently Left" section sits
  alongside it on the same screen — same layout primitives.

- **Patterns to mirror from #1717** (now living on `main`):
  - `DeletedCommunitiesNotifier` — `AsyncNotifierProvider.autoDispose` watching
    a counter-notifier signal.
  - `deletedCommunitiesCacheInvalidationProvider` — incremented on
    `invalidateDeletedList()`.
  - `DeletedCommunitiesSection` widget — `SizedBox.shrink()` when empty.
  - `DeletedCountdownBadge` + `daysLeftUntilPurge` helper — same shape works
    for the rejoin badge (different math: `RejoinWindowSeconds` minus
    elapsed since `left_at_unix_sec`).
  - `RestoreCommunityNotifier` — sealed `RestoreOutcome` (`idle | inProgress |
    success | alreadyRestored | noLongerEligible | error(UserError)`); RPC
    success path is best-effort (post-success cache fan-out cannot flip the
    outcome to error).
  - `RestoreCommunityScreen` — settings/utility screen (no `SwipeToCloseMixin`),
    listens to the notifier, reacts via snackbar + `SemanticAnnouncer`.
  - `selectedCommunityProvider.notifier.reloadCommunities()` in the cache
    fan-out so the side pane and "who can see" pickers pick up the rejoined
    community immediately.

- **`RejoinWindowSeconds`** — server constant (`server/community/constants.go`)
  is 30 days. Client computes badge math from `left_at_unix_sec` + the same
  window. Mirror the `daysLeftUntilPurge` helper as `daysLeftUntilRejoinExpiry`
  with the same ceiling-divided shape so the boundary cases (1 day / 29 days /
  30 days) read consistently with the deleted-list badge.

## Potential Solutions

The shape is largely fixed by the spec and #1717's precedent. The decisions worth
calling out are how to disambiguate the four `FailedPrecondition` reasons and
how the section relates visually to the existing deleted-list.

### Option A: clone #1717's pattern verbatim, parse error suffixes for outcomes

- **Approach:**
  1. **Service + Repository:** add `rejoinCommunity(id)` and
     `listRejoinableCommunities()` to `CommunityService`; cache wrappers under
     `community:rejoinable:list` in `CommunityRepository`.
  2. **`RejoinableCommunitiesNotifier`** (`AsyncNotifierProvider.autoDispose`)
     watches a new `rejoinableCommunitiesCacheInvalidationProvider`. The
     repository fires the signal from `invalidateRejoinableList()` (which
     `rejoinCommunity` calls on success).
  3. **`RecentlyLeftSection`** widget — placed in `SettingsHubScreen` after
     `DeletedCommunitiesSection`. Same layout primitives, same empty-state
     behavior. Each row uses `Tappable` with localised semantics labels and
     a `RejoinCountdownBadge` reading "Left X days ago, Y days to rejoin".
  4. **`RejoinCommunityScreen`** + **`RejoinCommunityNotifier`**. Sealed
     `RejoinOutcome`: `idle | inProgress | success | rejoinWindowExpired |
     communityDeleted | alreadyMember | noMembership | error(UserError)`.
     The notifier parses `e.message` to disambiguate the four
     `FailedPrecondition` reasons (server uses suffix discriminators —
     `…: no_membership_to_rejoin`, etc.).
  5. **Cache fan-out** matches §7.5 — narrower than #1717's restore because
     the leaver's prior gear/request/experience/transfer rows were
     soft-deleted by the leave cascade and **are not restored** on rejoin.
     Per the issue body: invalidate community row, recently-left list,
     active-communities list (`reloadCommunities`), conversation/chat,
     feed, stories. Skip gear/request/experience/transfer
     `invalidateForCommunity` calls.
- **Pros:** consistent with #1717 to the byte; reviewers who looked at #1717
  recognize every shape; tests are nearly copy-paste from the existing VM
  tests.
- **Cons:** parsing error suffixes from `e.message` is brittle (whitespace,
  punctuation, future server message-text changes). Mitigated by (a)
  centralising the parser in the notifier and (b) treating an unrecognised
  `FailedPrecondition` as a generic error rather than crashing.
- **Effort:** medium — same shape as #1717, so the second-time-through is
  faster.

### Option B: server-side typed reasons via Connect error details

- **Approach:** introduce a typed `RejoinFailureReason` enum on the server
  and attach it as a Connect error detail rather than relying on a string
  suffix. Client reads the typed detail.
- **Pros:** robust; future-proof against message-text changes.
- **Cons:** server changes; coordinated with another release; not strictly
  required because the parser in Option A is small and low-risk; we'd want
  to do this for the whole codebase if at all, not just this one RPC. The
  server already exists and its message format is in production via
  #1715 — changing it now is a server prereq for limited gain. Punts the
  client work.
- **Effort:** large (server + client + coordination).

### Option C: shrink scope — list only, no per-reason snackbars

- **Approach:** ship the recently-left list and the rejoin screen. On
  failure, show a generic "couldn't rejoin" snackbar regardless of reason.
- **Pros:** smallest PR.
- **Cons:** spec calls out four distinct user-facing snackbars for good
  reasons (the user needs to know whether to ask for a fresh invite vs.
  whether the community is gone vs. whether they're already a member). A
  generic message strands the user.
- **Effort:** small.

## Recommendation

**Option A.** Mirrors #1717 directly — patterns proven, audit findings
already applied, infrastructure (cache-invalidation signal, fan-out shape,
sealed outcome) lifts cleanly. Parsing error suffixes is contained to one
helper in the notifier and the unit tests pin every reason to its expected
outcome, so a server-message drift would surface as a failing test before
shipping.

Server-side typed reasons (Option B) is the right architectural change for
the whole codebase but doesn't belong in a single feature PR. File as a
separate "convert FailedPrecondition string discriminators to
google.rpc.ErrorInfo details" issue if we want to push that broader
refactor.

## Open Questions

- **Should the recently-left section appear above or below RECENTLY DELETED?**
  Recommendation: **below**. The deleted-list is more urgent — those communities
  hard-purge in 30 days and need the eyeball-attention. Recently-left is
  reversible without permanent data loss. Order: active list → RECENTLY DELETED →
  RECENTLY LEFT.

- **Empty-state rendering — section header visible or fully hidden?**
  Recommendation: **fully hidden** (matches #1717). Render only when the
  ViewModel returns a non-empty list. Same `SizedBox.shrink()` pattern.

- **Badge wording at the boundaries.** Recommendation: ICU plural mirroring the
  deleted countdown badge —
  `communityRejoinCountdownBadge: "{daysLeft, plural, =0{Rejoin window expires
  today} =1{Last day to rejoin} other{Left {daysAgo} days ago, # days to rejoin}}"`.
  But that has two variables (`daysAgo` and `daysLeft`) which complicates the
  ICU string. Simpler split: one string for the "left X days ago" part and one
  ICU plural for the "Y days to rejoin" tail, joined by ", " — easier to
  translate and less brittle. Confirm with widget golden tests at the
  boundaries (1, 15, 29, 30).

- **Snackbar copy — exact strings.** Recommendation: lift verbatim from the
  issue body. Specifically:
  - `communityRejoinSuccess` → `'Welcome back to {name}'`
  - `communityRejoinWindowExpired` → `'The 30-day rejoin window has expired.
    Ask a member to invite you back.'`
  - `communityRejoinCommunityDeleted` → `'This community is no longer available'`
  - `communityRejoinAlreadyMember` → `"You're already a member"`
  - `communityRejoinNoMembership` → `'No membership to rejoin'`
  - Plus a11y row label `a11yRejoinCommunityRow` and screen title /
    body / CTA keys mirroring the Restore screen's set.

- **Cache fan-out: which repos exactly?** Recommendation: match §7.5 narrowly
  per the issue body. The leaver's prior gear shares / requests / experiences
  were soft-deleted by the leave cascade (not by community delete) and
  remain soft-deleted after rejoin — the user "starts fresh." So:
  - `communityRepository.invalidate(id)` — community row.
  - `communityRepository.invalidateRejoinableList()` — the recently-left
    section.
  - `communityRepository.refreshUserCommunities()` — active-communities list.
  - `selectedCommunityProvider.notifier.reloadCommunities()` — push into the
    selection notifier so the side pane and pickers update immediately.
  - `chatRepository.invalidateAll()` *or* a community-scoped invalidator if
    one exists — the rejoiner is now a participant in community-scoped
    conversations again.
  - `feedRepository.invalidateFeed()` — feed re-includes content from the
    rejoined community.
  - `storyRepository.invalidateCommunity(id)` — stories.
  - **Skip** `gear/request/experience/transfer.invalidateForCommunity(id)`
    — those are not restored. (If a future change makes leave preserve
    leaver content for re-attach on rejoin, expand this list then.)

- **Navigation on success.** Recommendation: spec says "navigate to the
  community". After the rejoin success, push the user *into* the rejoined
  community's primary surface (the community page). That's a richer
  affirmation than dropping them on Settings. Implementation: after
  `_completeWith(success)`, call `selectedCommunityProvider.notifier
  .selectCommunity(...)` then navigate to the home screen / community page
  — mirror `_onAddCommunityTap` in `sidebar.dart` which selects the new
  community after creation.

- **`already_member` outcome destination.** Recommendation: spec says
  "navigate to the community". Same as success path — pop the rejoin
  screen and route into the community. The user already had access; we're
  just honouring the request.

- **`community_deleted` outcome destination.** Recommendation: spec says
  "dismiss". Pop back to Settings. Also invalidate the recently-left list
  so the now-stale entry disappears (the server will probably also have
  removed it from `ListRejoinableCommunities` per §2.7's active-community
  filter, but a local invalidation closes the race).

- **`no_membership_to_rejoin` outcome.** Recommendation: spec says "dismiss
  + remove from list". Invalidate the recently-left list (server will
  filter it out on next read). Pop the screen.

## Implementation Plan

### Phase 1: service + repository wiring (no UI yet)

**Goal:** `CommunityService` and `CommunityRepository` expose the two new
operations with caching, plus the cache-invalidation signal that the next
phase's ViewModel watches.

**Tasks:**
- [ ] `community_service.dart`: add `rejoinCommunity(String communityId)` and
  `listRejoinableCommunities()`. Both follow `RpcUtils.executeRpc` +
  `_buildHeaders()`; the latter returns `List<RejoinableCommunityItem>`. Add
  `RejoinableCommunityItem` to the `export` list at the top of the file
  (mirrors the Phase-1 export of `DeletedCommunityItem` for #1717).
- [ ] `community_repository.dart`: add `rejoinCommunity(String communityId)`,
  `listRejoinableCommunities({bool refresh = false})`, and
  `invalidateRejoinableList()`. Cache key `'rejoinable:list'` under the
  existing `'community'` namespace. `rejoinCommunity` invalidates the row,
  the rejoinable-list, and refreshes user-communities — mirrors
  `restoreCommunity`.
- [ ] `cache_providers.dart`: add `rejoinableCommunitiesCacheInvalidationProvider`
  + `RejoinableCommunitiesCacheInvalidationNotifier` (counter-notifier
  pattern, same shape as `deletedCommunitiesCacheInvalidationProvider`).
  Re-export from `services/providers.dart`.
- [ ] `community_providers.dart`: thread an `onRejoinableListInvalidated`
  callback into `CommunityRepository`'s constructor; the provider fires
  the signal.
- [ ] Repository test: cache hit/miss on `listRejoinableCommunities`,
  `rejoinCommunity` succeeds invalidate path, `rejoinCommunity` failure
  preserves the cache snapshot. Mockito mocks for `CommunityService`.
  Mirrors the corresponding tests for `restoreCommunity` /
  `listDeletedCommunitiesForRestore` in
  `community_repository_test.dart`.
- [ ] Provider test: wire a counter override on
  `rejoinableCommunitiesCacheInvalidationProvider`, call
  `repo.invalidateRejoinableList()` directly, assert the counter
  bumped. Repeat with `repo.rejoinCommunity(id)` succeeding to assert
  the same on the rejoin path. Catches the silent-stale-section bug
  we hit during #1717 testing before it ships.

**Validates:** the new RPCs round-trip end-to-end against the server
(via the next phase's manual smoke), the cache contract is correct,
and the invalidation signal is wired without any UI yet.

### Phase 2: Settings hub recently-left section

**Goal:** users see rejoinable communities in the right place, with the
correct empty state and countdown rendering. Tap is a no-op until Phase 3.

**Tasks:**
- [ ] New ViewModel `rejoinable_communities_view_model.dart` —
  `AsyncNotifierProvider.autoDispose<RejoinableCommunitiesNotifier,
  List<RejoinableCommunityItem>>`. `build()` watches
  `rejoinableCommunitiesCacheInvalidationProvider` (so refetches fire
  the moment delete/restore-elsewhere changes the eligibility set) and
  returns `repo.listRejoinableCommunities()`.
- [ ] New widget `rejoin_countdown_badge.dart`. Renders "Left X days ago,
  Y days to rejoin" via an ICU plural ARB key. Wraps in `Semantics(label:)`
  matching the visible text — `avoid_color_only_status` lint requires
  it. Helper `daysLeftUntilRejoinExpiry({leftAtUnixSec, nowUnixSec,
  windowDays = 30})` mirrors `daysLeftUntilPurge`.
- [ ] New widget `recently_left_section.dart` under
  `presentation/widgets/settings/`. Same layout primitives as
  `DeletedCommunitiesSection`. Each row: `CommunityAvatar` (cover via
  `mediaIds`), name, badge, chevron. `Tappable` with localised
  `a11yRejoinCommunityRow` semantics label. Returns `SizedBox.shrink()`
  when empty.
- [ ] ARB additions (English + Spanish, with `@key` metadata):
  `settingsRecentlyLeftHeader`, `communityRejoinCountdownBadge`
  (ICU plural), `a11yRejoinCommunityRow` (with `{name}` and `{daysLeft}`
  placeholders).
- [ ] `settings_hub_screen.dart` edit: append `const RecentlyLeftSection()`
  *after* `DeletedCommunitiesSection`. Section ordering is active list →
  RECENTLY DELETED → RECENTLY LEFT.
- [ ] ViewModel test (`ProviderContainer` + Mockito): state populates
  from the repo, server-side ordering preserved (descending by
  `leftAtUnixSec`), empty-state, refresh, cache-invalidation rebuild,
  disposal-during-async.
- [ ] Widget test for `rejoin_countdown_badge.dart` at `daysLeft` ∈
  {0, 1, 15, 29, 30}. Use a fake clock so the badge is deterministic.
  Verify Semantics annotation matches visible text.

**Validates:** the surface renders correctly across day boundaries, the
section disappears cleanly when empty, and the cache-invalidation signal
re-fires on changes.

### Phase 3: Rejoin confirm screen + cache fan-out

**Goal:** tap a recently-left entry → reach the Rejoin screen → confirm →
land in the rejoined community with all relevant caches refreshed.

**Tasks:**
- [ ] New screen `rejoin_community_screen.dart` under
  `presentation/screens/settings/`. Settings/utility screen, no
  `SwipeToCloseMixin`, reached via `NavigationHelpers.pushWithSlide`.
  Body matches the spec: "Rejoin {name}? Your previous gear shares,
  requests, and notification preferences are not restored — you'll
  start fresh." Single primary CTA *Rejoin*, Cancel via
  `AppBarBackButton`.
- [ ] New ViewModel `rejoin_community_view_model.dart` — `Notifier
  <RejoinOutcome>` with `SafeNotifierMixin`. Sealed `RejoinOutcome`:
  `idle | inProgress | success | rejoinWindowExpired |
  communityDeleted | alreadyMember | noMembership | error(UserError)`.
  Add `Logger('RejoinCommunityViewModel')` plumbing (mirrors
  `RestoreCommunityViewModel`) — info on success, info per typed
  outcome, warning on unrecognised suffix, severe on unexpected
  exception. Server-side correlation rides via `X-Request-ID` already
  emitted by `RpcUtils.buildHeaders` — no client change required.
  `rejoin(String communityId)` calls
  `CommunityRepository.rejoinCommunity(id)`. On
  `Code.failedPrecondition`, parse the suffix in `e.message` to
  disambiguate (`': no_membership_to_rejoin'`, `': already_member'`,
  `': rejoin_window_expired'`, `': community_deleted'`); unrecognised
  → `error(UserError)` to be safe. Anything else → `error(UserError)`
  via `RpcErrorHandler.classify`. **`Code.unauthenticated` is handled
  upstream** by `RpcUtils.executeRpc` via the `_onUnauthenticated`
  callback wired in `CommunityService`'s constructor; the VM's
  outcome-mapping never sees it. Document as a comment in the VM.
- [ ] On success: best-effort cache fan-out (community row, rejoinable
  list, user-communities, `selectedCommunityProvider.reloadCommunities`,
  feed, stories, chat — see Open Question 4 for the exact set, narrower
  than #1717's restore fan-out). Wrap fan-out in a try/catch — a stale
  repo must not flip the outcome to `error`. Same refactor we landed
  for the Restore VM.
- [ ] Widget reaction in the screen, matching the spec's snackbars.
  **Order of operations on success matters**: the cache fan-out
  (including `reloadCommunities()`) must run *before*
  `selectCommunity(item)` so the rejoined community is present in
  `state.communities` when the selection notifier looks for it.
  - `success` → `communityRejoinSuccess(name)` snackbar +
    `SemanticAnnouncer.announce` → pop screen → call
    `selectedCommunityProvider.notifier.selectCommunity(item)` (the
    fan-out's `reloadCommunities` already populated the state) →
    navigate into the community.
  - `alreadyMember` → same as success — community already accessible,
    drop user into it via the same select-then-navigate path. Skip
    the cache fan-out (nothing changed server-side) but do call
    `reloadCommunities()` defensively in case local state is out of
    sync.
  - `rejoinWindowExpired` → `communityRejoinWindowExpired`. Pop back
    to home via `Navigator.of(context).popUntil((route) =>
    route.isFirst)` (do *not* use `context.go` — forbidden for in-app
    navigation per `docs/client/architecture.md`). Add `mounted`
    check before each pop.
  - `communityDeleted` → `communityRejoinCommunityDeleted`, pop back
    to Settings, call `repo.invalidateRejoinableList()` locally to
    remove the stale entry. Server's `ListRejoinableCommunities`
    will already filter it out per §2.7's active-community guard;
    this just closes the race.
  - `noMembership` → `communityRejoinNoMembership`, pop, call
    `repo.invalidateRejoinableList()`.
  - `error` → `RpcErrorHandler.localize(error, l10n)` snackbar, stay
    on screen so the user can retry.
- [ ] Wire `_buildRow` tap in `RecentlyLeftSection` →
  `NavigationHelpers.pushWithSlide(context, RejoinCommunityScreen(
  community: item))`.
- [ ] ARB additions (English + Spanish, with `@key` metadata):
  `communityRejoinTitle`, `communityRejoinBody` (with `{name}`),
  `communityRejoinCta`, `communityRejoinSuccess` (with `{name}`),
  `communityRejoinWindowExpired`, `communityRejoinCommunityDeleted`,
  `communityRejoinAlreadyMember`, `communityRejoinNoMembership`.
- [ ] ViewModel test (`ProviderContainer` + Mockito,
  `TestWidgetsFlutterBinding.ensureInitialized()` +
  `SharedPreferences.setMockInitialValues({})` so the
  `reloadCommunities` call doesn't blow up):
  - success path verifies the fan-out fired in full;
  - each `FailedPrecondition` suffix maps to its outcome (no fan-out
    on failure);
  - generic error → `error(UserError)`;
  - disposal during async.
- [ ] Widget test: cancel via `AppBarBackButton` does not call rejoin.
- [ ] Manual run against local server: leave a community via the
  existing Leave stub (or via the dev-only hack until #1719 lands the
  proper picker), reopen Settings, see RECENTLY LEFT entry, tap →
  Rejoin screen → confirm → community returns to the active list and
  the user lands in it.

**Validates:** the entire leave → wait → rejoin loop works end-to-end,
the four documented `FailedPrecondition` reasons each surface the
correct localised snackbar, and the §7.5 cache fan-out keeps the side
pane / pickers honest.

## Related Issues

- #1619 — parent (community soft-delete with 30-day restore window).
- #1717 / merged — sibling deleted-list + Restore screen. **Mirror
  this PR's shape exactly** — same hub placement, same VM/section/badge
  trio, same cache-invalidation signal, same screen+notifier shape for
  the confirm.
- #1718 / merged — Danger Zone choice modal + real Delete flow. Side
  pane / "who can see" cache invalidation refactor is reused.
- #1657 / PR #1715 — server-side `RejoinCommunity` (merged).
- #1656 / PR #1709 — server-side `LeaveCommunity` rewrite that creates
  the soft-deleted membership rows the recently-left list reads from.
- #1719 — sibling: owner-leave member picker. Until #1719 ships, the
  in-app Leave button is still a stub, so users won't have a path to
  *create* the soft-deleted membership rows that this PR's surface
  reads. Manually creating them via direct SQL or a dev-only Leave
  trigger is the testing workaround.
- #1723 / PR #1724 — server `ListRejoinableCommunities` RPC (merged) —
  the data source for this PR's recently-left list.
