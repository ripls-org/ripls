# Issue #2000: Manage members invite someone not working

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2000
**Filed:** 2026-05-17 | **Last activity:** 2026-05-18
**Labels:** user-feedback | **Assignees:** briangbrown
**Context docs:** none

## Problem Statement

On the **Manage Members** screen, the floating-action button labeled "Invite
Someone" is disabled (greyed out) and tapping it does nothing. This was
reported in app version 0.21.1 on Android (production).

Root cause: `ManageMembersScreen` derives the `CommunityItem` it passes
into the `InviteSheet` from the global
`selectedCommunityProvider.selectedCommunity` getter. Since PR #1895
("nav restructure"), per-community selection was removed and
`SelectedCommunityNotifier` was neutered — `selectCommunity` /
`setCommunities` / `switchTo` all force `selectedCommunityId = null`,
which means `selectedCommunity` is **always** `null` everywhere. See
`app/lib/services/providers/community_providers.dart:148-188`. The
governance screen and the settings hub were migrated to take an
explicit `CommunityItem` parameter at that time, but
`ManageMembersScreen` was missed.

Concretely, `app/lib/presentation/screens/communities/manage_members_screen.dart`:

- Line 62: `final community = communityState.selectedCommunity;` → always `null`.
- Line 237: FAB `onPressed: community != null ? ... : null` → button is permanently disabled.
- Line 261: the shadow-user "send invite" flow also depends on
  `community`; when null it falls back to a plain
  `ToastHelper.showInfo(context, inviteUrl)` instead of opening the
  invite sheet. So the secondary per-shadow-user invite button is
  also degraded, even though the issue only mentions the FAB.

The screen already receives `communityId` as a required parameter, so the
data needed to identify the community is on the widget — it just isn't
being used to resolve the `CommunityItem`.

## Context from Comments

One comment from the issue author (`briangbrown`): `@claude write plan`.
No other discussion, no workarounds proposed. The reporter is the
project owner so this is treated as a confirmed bug, not a request for
clarification.

The screenshot referenced in the body shows the Manage Members screen
with the disabled FAB — consistent with the `onPressed: null` analysis.

## Potential Solutions

### Option A: Look up the community by ID from `state.communities`

- **Approach:** Inside `manage_members_screen.dart`, replace
  `communityState.selectedCommunity` with
  `communityState.communities.where((c) => c.id == widget.communityId).firstOrNull`.
  The `communities` list on `SelectedCommunityState` still holds every
  community the signed-in user belongs to (populated by
  `setCommunities` via `listUserCommunities`), and only members of a
  community can land on its Manage Members screen, so the lookup is
  guaranteed to resolve.
- **Pros:** Smallest diff (one line + a small refactor in
  `_showShadowInviteSheet`). No call-site changes. No new
  dependencies. Same data source the rest of the codebase already
  trusts.
- **Cons:** Couples the screen to the assumption that
  `selectedCommunityProvider.communities` is populated before the user
  reaches Manage Members. In practice it always is — communities are
  loaded at app start and the only entry point (Governance) already
  required `selectedCommunity` to be present in the same list.
- **Effort:** Small (~10 lines + one regression test).

### Option B: Pass `CommunityItem` from the caller (mirror `GovernanceScreen`)

- **Approach:** Change `ManageMembersScreen` to accept `CommunityItem
  community` instead of (or in addition to) `String communityId`, and
  update `ManageMembersScreen.show(...)` and the single call site in
  `governance_screen.dart:175` to pass `selectedCommunity` directly.
- **Pros:** Matches the explicit-parameter pattern adopted by
  `GovernanceScreen` and `settings_hub_screen.dart` after #1895.
  Eliminates the dependency on `selectedCommunityProvider` for this
  screen entirely.
- **Cons:** Wider surface area than the bug requires — touches the
  screen's public constructor, the static `show` helper, and the call
  site. Risk of breaking other (future) call sites that only have an
  ID at hand. The `manageMembersProvider` still needs a `String
  communityId` for its `load()` call, so the screen ends up
  storing/threading both.
- **Effort:** Small-medium (~30 lines across two files plus call-site
  audit).

### Option C: Fetch via `communityProvider(communityId)` `FutureProvider`

- **Approach:** Use `ref.watch(communityProvider(widget.communityId))`
  to fetch the community details, gate the FAB on the future's data
  state, and pass the resulting community name into `InviteSheet`.
- **Pros:** Works even when the community isn't in the local
  `selectedCommunityProvider.communities` snapshot (e.g. a deep
  link).
- **Cons:** Adds an async dependency to a button that already has
  enough data to enable itself. Introduces a loading-state UI question
  for the FAB. Heavier than the bug warrants.
- **Effort:** Medium.

## Recommendation

**Option A.** It is the minimal root-cause fix: the screen already knows
which community it represents (`widget.communityId`), and the
`communities` list on `SelectedCommunityState` is exactly the lookup
table the screen needs. The call-site contract doesn't change, so
there's no risk of touching unrelated code. Option B is a defensible
refactor but is more scope than this bug deserves and would benefit
from being done as part of a broader sweep that also revisits the
`manageMembersProvider.load(communityId)` parameter and the shadow
invite sheet's invite-URL handling (see Open Questions below).

While here, also fix the parallel degradation in `_showShadowInviteSheet`
so the shadow-user invite button opens the sheet instead of dropping the
URL into a toast. Same root cause, same one-line resolution.

## Open Questions

- **Should the shadow-user invite path actually use the shadow-specific
  `inviteUrl` (not the generic community invite URL the sheet
  generates internally)?** The current code passes `inviteUrl` through
  to `_showShadowInviteSheet` but `InviteSheet` ignores it and
  regenerates a community invite link via
  `getOrCreateInviteLink(communityId, gearId)`. This looks like a
  pre-existing bug that #1895 didn't introduce — the shadow URL is
  fetched but never used. **Recommendation:** Out of scope for #2000;
  file a follow-up issue noting that `pendingInviteLink.inviteUrl`
  isn't reaching the share sheet. The current fix should restore
  pre-#1895 behavior (open the sheet at all) without trying to also
  fix the URL-routing question.
- **ViewModel-first vs widget-level regression test?**
  `docs/client/testing.md` is emphatic that the primary level is
  ViewModel tests, not widget tests. The bug here lives entirely in
  the screen's binding to `selectedCommunityProvider`, not in
  `ManageMembersNotifier`'s business logic — so the most-correct test
  level is the screen. **Recommendation:** Add a minimal widget test
  (one assertion: FAB `onPressed != null`) and accept the small
  testing-doctrine asymmetry; pulling the community lookup into the
  ViewModel just to enable a unit test would expand the change beyond
  what the bug warrants. If the audit insists on ViewModel-level
  coverage, move the lookup into `ManageMembersNotifier.load()` and
  expose `community` on `ManageMembersState`.
- **Should the hardcoded FAB label `'Invite Someone'` be localized as
  part of this fix?** The string at
  `manage_members_screen.dart:241` is a raw `const Text('Invite
  Someone')`, which violates the `docs/client/i18n.md` "everything user-
  visible goes through `context.l10n.*`" rule. A key already exists
  (`plusButtonInviteSomeone`). The accessibility lint
  (`require_l10n_for_semantic_labels`) targets `tooltip:` /
  `semanticLabel:` arguments, not FAB child `Text` widgets, so this
  isn't a lint blocker. **Recommendation:** Localize it as part of
  Phase 1 — it's a one-character-of-code change
  (`Text(context.l10n.plusButtonInviteSomeone)`), satisfies the i18n
  doctrine, and avoids leaving the file in a half-corrected state.
- **Is there a path to Manage Members that bypasses the community
  loading?** Audit shows the only caller is
  `governance_screen.dart:175`, which already requires
  `selectedCommunity` to be non-null (so the user must have communities
  loaded). No deep-link entry exists today. **Recommendation:** Treat
  "community must be in the local list" as an invariant; if the lookup
  ever fails, keep the FAB disabled (graceful degrade) rather than
  crashing.

## Implementation Plan

### Phase 1: Restore the FAB and the shadow-invite sheet

**Goal:** "Invite Someone" enables and opens the invite sheet on every
Manage Members entry. The shadow-user send-invite button opens the same
sheet instead of dropping a toast.

**Tasks:**

- [ ] In `app/lib/presentation/screens/communities/manage_members_screen.dart`,
  replace
  ```dart
  final community = communityState.selectedCommunity;
  ```
  (line 62) with
  ```dart
  final community = communityState.communities
      .where((c) => c.id == widget.communityId)
      .firstOrNull;
  ```
  `Iterable.firstOrNull` is part of the Dart 3 SDK
  (`dart:collection` extensions) and is already used widely in this
  file via `communities.where(...).firstOrNull` patterns elsewhere in
  the codebase — confirm no extra import is needed by running
  `flutter analyze` after the edit.
- [ ] At `manage_members_screen.dart:241`, replace
  `label: const Text('Invite Someone')` with
  `label: Text(context.l10n.plusButtonInviteSomeone)` to satisfy the
  i18n convention. The l10n extension is already imported on line 6.
- [ ] Confirm the FAB at lines 235-243 now resolves a non-null
  `community` and is therefore enabled.
- [ ] Confirm `_showShadowInviteSheet` (lines 259-275) now reaches the
  `InviteSheet` branch instead of the toast fallback.
- [ ] No changes to `ManageMembersScreen.show`, no changes to the
  governance call site, no changes to `manageMembersProvider` /
  `ManageMembersState` / `ManageMembersNotifier`.

**Validates:** The reported bug is fixed without expanding the public
surface of the screen, and the file's i18n compliance moves one notch
forward instead of staying half-corrected.

### Phase 2: Regression test

**Goal:** A widget test that fails if anyone re-introduces the
dependency on `selectedCommunity`. (Widget rather than ViewModel
level because the bug isn't in `ManageMembersNotifier` — it's in how
the screen binds `selectedCommunityProvider` to the FAB's enabled
state. See the "ViewModel-first vs widget-level" Open Question for
why this is an intentional deviation from
`docs/client/testing.md`'s ViewModel-first norm.)

**Tasks:**

- [ ] Add `app/test/presentation/screens/communities/manage_members_screen_test.dart`
  that:
  - Overrides `selectedCommunityProvider` with a state containing a
    `CommunityItem` matching the test `communityId` and
    `selectedCommunityId: null` (mirroring post-#1895 reality).
  - Pumps `ManageMembersScreen` inside a `ProviderScope` wrapped in
    `MaterialApp` with `AppLocalizations.delegate` configured so
    `context.l10n.plusButtonInviteSomeone` resolves.
  - Finds the `FloatingActionButton.extended` and asserts its
    `onPressed != null`.
  - Optionally taps it and asserts that `InviteSheet` is shown
    (find by widget type).
- [ ] Override `manageMembersProvider` so the test doesn't make
  network calls. Two options:
  - Override the autoDispose `NotifierProvider` with a fake notifier
    that returns an empty `ManageMembersState`, OR
  - Override `communityRepositoryProvider` /
    `shadowUserRepositoryProvider` with Mockito mocks (matching the
    pattern in `app/test/presentation/viewmodels/manage_members_view_model_test.dart`)
    so the real notifier's `load()` resolves immediately.
  Prefer the second so the test exercises the real screen→VM wiring.
- [ ] Generate any new Mockito mocks needed:
  `cd app && dart run build_runner build --delete-conflicting-outputs`.
- [ ] Run `cd app && flutter test --reporter expanded
  test/presentation/screens/communities/manage_members_screen_test.dart`.
- [ ] Add `tearDown` cleanup (`reset(mock...)`, container disposal)
  per the conventions in `docs/client/testing.md` §"Common Testing
  Pitfalls" — non-negotiable to keep the test isolated.

**Validates:** The fix sticks. CI catches a regression if someone
reintroduces a `selectedCommunity` dependency on this screen.

### Phase 3: Verify and ship

**Goal:** Build is green, no other call sites of `selectedCommunity`
fall into the same trap on screens that already received a
community-specific ID.

**Tasks:**

- [ ] `cd app && flutter test --reporter expanded` — full Flutter test suite.
- [ ] `cd app && flutter analyze` — static analyzer clean.
- [ ] `npm run lint:dart:a11y` — accessibility lint clean (the FAB
  change touches presentation code).
- [ ] `npm run lint:dart:size` — file-size lint clean.
- [ ] Grep for `selectedCommunityProvider).selectedCommunity` and
  `selectedCommunityProvider.notifier).state.selectedCommunity` across
  `app/lib/` and skim the matches for any other screen that already
  has a `communityId` in scope and could be silently broken the same
  way. **No fix in this PR** — file follow-ups if any are found.
- [ ] Manual test on a debug build: open Governance → Manage Members,
  confirm FAB is enabled and opens the invite sheet; for a community
  with shadow members, confirm the per-shadow send icon also opens
  the sheet.

**Validates:** No regressions, no scope creep, ready to merge.

## Related Issues

- #1895 — Nav restructure that removed per-community selection and
  neutered `SelectedCommunityNotifier`. Introduced the latent bug that
  this issue surfaces; the fix here is the same shape as the
  `GovernanceScreen` / `settings_hub_screen.dart` migrations that
  shipped with #1895.
- #1968, #1971 — Prior reports of users stuck on a single selected
  community after the migration; the same `selectedCommunity is now
  always null` invariant is what fixed them and what now breaks the
  invite FAB.
