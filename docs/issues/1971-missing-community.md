# Issue #1971: Missing a community

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1971
**Filed:** 2026-05-16 | **Last activity:** 2026-05-16
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none (related issue #1968 read inline)

## Audit Notes (2026-05-16)

Reviewed against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/testing.md`, `docs/client/i18n.md`,
`docs/client/accessibility.md`, and `docs/server/observability.md`.
Findings folded into Phase 1:

- **Test file path corrected.** Initial plan referenced
  `app/test/services/providers/community_providers_test.dart` (does
  not exist). The existing test for this notifier lives at
  `app/test/services/selected_community_test.dart`. Update that file
  rather than creating a new one — see Phase 1 tasks.
- **Existing tests encode the buggy behavior as truth.** Five tests
  in `selected_community_test.dart` will fail after Phase 1 because
  they assert exactly the behavior we're removing (auto-pick, restore
  persisted, migrate legacy). They must be rewritten, not just
  supplemented. The `selectCommunity (neutered)` group stays
  unchanged.
- **Architectural compliance:** the change stays inside
  `SelectedCommunityNotifier` (a `Notifier<SelectedCommunityState>`
  per docs/client/architecture.md). No layering violation: no
  ViewModel-to-service jump, no widget-to-repository jump, no manual
  cache map. Single emission via `copyWith` preserved.
- **No caching, i18n, or accessibility impact.** No
  repository / cache invalidation paths touched. No user-visible
  strings added or removed. No new interactive widget. Workshop
  title text change is post-state-update only (existing l10n keys).
- **Server observability (Phase 4 only).** The existing
  `ListCommunities` handler already uses `LoggerWithContext` with
  `operation`, `user_id`, `user_email` (masked). Filtering rows
  needs no new log lines; if we want forensics on soft-deleted skips,
  add a single `Debug`-level line — not required, not added by
  default. Out of scope for Phase 1.
- **Removed the "(Optional, defensive) make `isAllCircles` always
  return `true`" task.** Once `setCommunities` never emits a non-null
  `selectedCommunityId`, `isAllCircles` already returns `true`
  naturally. A defensive override would mask future regressions in
  the notifier and is the wrong instinct.

## Status

**Root cause confirmed live (2026-05-16).** Reproduced on local server
with simulated user Mike Patel (`00000000-0000-4000-8000-000000000008`),
member of two communities: `db98be32…` Oakwood Heights Neighbors and
`de7b7205…` Code Debug Squad.

- **Postgres is clean:** both `community_user` rows active (no
  `deleted_*` columns set), both `community` rows active, both
  visible to `ListCommunities`.
- **Server returned both:** Android log shows
  `CommunityRepository: Community fetch completed: 2 communities returned`.
- **Client immediately collapsed to one:** next log lines —
  ```
  SelectedCommunityNotifier: setCommunities: 2 communities received
  SelectedCommunityNotifier: setCommunities: restored single selection
    00000000-0000-4000-8000-000000000009
  SelectedCommunityNotifier: setCommunities: emitted 1 state change,
    selectedCommunityId=00000000-0000-4000-8000-000000000009
  ```
  Hits the "restored single selection" branch at
  `community_providers.dart:228-229` — the SharedPreferences key
  `selected_community_id` still holds Oakwood's ID from when Oakwood
  was Mike's sole community.
- **Downstream consumers split into two camps:**
  - `effectiveCommunityIds` / `enabledCommunities` readers see Oakwood
    only — `UnreadCountViewModel: communities=1`, `SearchViewModel:
    communities=1`, `cacheKey=search:unified:db98be32-…`. Code Debug
    Squad's gear, requests, experiences, chat, and unread are all
    filtered out at the source.
  - `.communities` readers (top switcher, workshop) see both —
    `GetWorkshopSynthesis` fires for both `db98be32-…` and
    `de7b7205-…` cache keys.

This matches the reporter's symptoms exactly. **Phase 1 below is the
fix.** No server change required to recover stuck users; the persisted
key clears on next launch once the patched client ships.

## Problem Statement

A prod user (user7@example.org, app 0.21.0 build 6957, Pixel 9 Pro)
reports that one of her communities, "Travis Heights Early Adopters,"
has functionally disappeared:

- It shows up by name in the community switcher at the **top** of the
  ripls tab.
- Tapping it in that switcher **does nothing** — the view does not
  filter to this community.
- The items and events associated with it are not visible anywhere.
- (Follow-up clarification from the reporter) she cannot see chat for
  this community either.
- (Reporter is "definitely still a member" — not a soft-deleted
  membership scenario.)

She filed a related earlier ticket, #1968: "When I create a new item
for giveaway, only one of my communities appears as an option to share
to." Same user, same community, same prod environment, ~12 minutes
apart. Treat #1971 and #1968 as one bug.

The user is the source of truth that she is still a member. Both
symptoms — the community appearing at the top picker but not in the
share picker, and tapping it not filtering — point to a **client-side
state divergence**, not a server-side membership problem.

## Context from Comments

No comments on either issue. The reporter has interacted with the app
enough to file two coherent bug reports back-to-back, including a
screenshot for #1968, so we can trust her observations.

The user-supplied clarification in this session ("she is still a
member; the community shows up in the picker UI at the top of the
ripls tab") rules out the most natural server-side theory (soft-deleted
membership) and points hard at the client.

## Investigation — the actual divergence

Three client-side surfaces consume the user's community list, and they
read **three different derived views** of the same provider:

1. **Top community switcher on the ripls tab** — reads `.communities`
   (full list returned by `ListCommunities`). Shows all memberships.
2. **Gear-creation share-target picker** — `CommunityPickerSheet` at
   `app/lib/presentation/widgets/sharing/community_picker_sheet.dart:57`
   reads `.enabledCommunities`. So does the experience share picker
   and several other call sites (`home_screen.dart:140`,
   `portfolio_inbox_screen.dart:405`, `community_helper.dart:22`).
3. **Per-community filter applied to feed/items/events on the ripls
   tab** — driven by `selectedCommunityId` in
   `SelectedCommunityState` (`app/lib/services/providers/community_providers.dart`).

The getter at `community_providers.dart:155-157` defines:

```dart
List<CommunityItem> get enabledCommunities => isAllCircles
    ? communities
    : communities.where((c) => c.id == selectedCommunityId).toList();
```

When `selectedCommunityId == null` (All Circles), `enabledCommunities`
equals `communities` and all three surfaces agree. When
`selectedCommunityId` points to a single community,
`enabledCommunities` collapses to just that one — and the share
picker, the invite sheet, the title bar, the inbox filter, etc. all
narrow to that single community while the top switcher still shows
everything.

### Why the user is stuck in single-select

PR **#1895** (commit `b9fcbc2b7`, "phase 4: delete sidebar + neuter
selectedCommunityProvider setter") removed the per-community sidebar
filter UI. As part of that, `SelectedCommunityNotifier.selectCommunity()`
and `switchTo()` were neutered to no-ops with a debug `assert(false)`
— they ignore the argument and force `selectedCommunityId = null`
(`community_providers.dart:181-188, 190-192`).

But `setCommunities()` — called once on every login/relaunch from
`main.dart:341-350` — **still restores the persisted selection from
SharedPreferences** (`community_providers.dart:204-264`):

- Line 224: reads `selected_community_id` key.
- Line 241-244: if no new-key value and exactly **one** valid
  community, **auto-picks it** and writes it to SharedPreferences
  ("auto-picked sole community").
- Lines 228-229: if the persisted ID is still a member, restores it.

So a user who:
1. Was on a pre-#1895 build with a single-community filter set, **or**
2. Joined her first community on a post-#1895 build (auto-pick at
   line 241-244 wrote her sole community's ID to prefs at that
   moment),

...will be permanently stuck with `selectedCommunityId` pointing at
that original community. Joining a second community ("Travis Heights")
adds it to `communities` but does **not** change the persisted
`selectedCommunityId`. The user cannot fix it from the UI because the
setter is neutered (tapping the new community in the top switcher
asserts/no-ops, matching the reporter's "selecting the community
doesn't change anything").

### Symptom-by-symptom match

| Symptom | Root cause |
|---|---|
| Community appears in top picker | Top picker reads `.communities`; full list is correct |
| Tapping the community does nothing | `selectCommunity()` is a no-op after #1895 |
| Items and events nowhere | Feed/items/events queries are scoped by `selectedCommunityId` (the *other* community), so Travis Heights content is filtered out |
| Chat nowhere | Conversation list also scoped by current selection |
| Only one community in share picker (#1968) | `CommunityPickerSheet` reads `.enabledCommunities`, which post-#1895 collapses to a single sticky community |

This is one bug, not two.

### Server-side ListCommunities is also leaky (separate, secondary)

While confirming the above I noticed
`server/services/community/membership.go:120-202` (`ListCommunities`)
filters neither `community.Deleted` nor `membership.Deleted`. That is
a real latent bug (soft-deleted communities or soft-deleted memberships
would appear in the user's `communities` list), but it does **not**
explain the reported symptoms — the user is an active member and the
community is active. Worth fixing in a follow-up; out of scope here.

## Potential Solutions

### Option A: Always collapse to All Circles client-side

- **Approach:** In `SelectedCommunityState`, hardwire
  `selectedCommunityId` to `null` whenever the field is read by any
  consumer. Concretely: change the `isAllCircles` getter to always
  return `true`, or have `enabledCommunities` simply return
  `communities`. Stop reading `selected_community_id` and the legacy
  `enabled_community_ids` keys from SharedPreferences in
  `setCommunities()`. Clear both keys on next launch as a one-shot
  cleanup.
- **Pros:** Smallest diff. Fixes the reported symptom directly. No
  server changes. Ships in the next app release; existing prod users
  recover on first relaunch.
- **Cons:** Leaves dead code: the `selectedCommunityId` field, the
  derived getters, the persistence key, the rejoin screen's
  `selectCommunity(match)` caller (`rejoin_community_screen.dart:156`).
  Tech-debt-shaped but harmless.
- **Effort:** Small.

### Option B: Delete the single-select model entirely

- **Approach:** Rip out `selectedCommunityId`, `selectedCommunity`,
  `effectiveCommunityIds`, `enabledCommunities`, `selectCommunity`,
  `switchTo`, and the SharedPreferences keys. Replace
  `enabledCommunities` call sites with direct reads of `communities`.
  Delete the rejoin screen's selection call.
- **Pros:** Removes the dead code path that produced the bug.
  Eliminates the entire class of "client surfaces disagree about which
  communities exist" bugs. Clean codebase post-#1895.
- **Cons:** Larger diff (~15-25 call sites to migrate). Higher risk of
  unrelated breakage. Worth doing but probably not in the same PR as
  the user-facing fix.
- **Effort:** Medium.

### Option C: Restore the per-community filter UI (revert #1895 partially)

- **Approach:** Bring back a working `selectCommunity()` so that the
  top switcher actually filters and the persisted selection is
  meaningful again.
- **Pros:** None for the user — she didn't ask for the filter to come
  back, she's just confused why one community is gone.
- **Cons:** Reverts a product decision; doesn't fix #1968 (the share
  picker would still narrow to the current filter).
- **Effort:** Medium, and the wrong direction.

## Recommendation

**Ship Option A immediately, follow up with Option B in a separate PR.**

Option A is the minimal, surgical change that resolves both the user's
problem and #1968 with one diff:

1. Force `isAllCircles` to always be true (one-line change to the
   getter on `community_providers.dart:132`), **or** equivalently
   make `setCommunities()` always set `selectedCommunityId: null`
   regardless of persisted state, **and** delete both
   SharedPreferences keys.
2. Verify with a unit test that `enabledCommunities == communities`
   for any non-empty input.
3. Add an integration test reproducing the regression: persist a
   single-community ID, call `setCommunities` with two communities,
   assert all three surfaces see both.

This recovers stuck users on relaunch without requiring a server-side
patch. Option B (full removal of the dead state) can land afterwards
when there's time for a careful migration of the 15-25 call sites.

While we're here, also fix the inconsistency that started this whole
mess by aligning the share picker:

4. In `community_picker_sheet.dart:57`, change
   `ref.watch(selectedCommunityProvider).enabledCommunities` →
   `.communities`. The share-target picker should *always* offer the
   user's full set of memberships regardless of any current view
   filter — Option A makes the two equivalent, but the explicit
   `.communities` read documents the intent and would also have
   prevented this bug on its own.

The server-side `ListCommunities` `Deleted` leak is real but out of
scope; tracked separately.

## Open Questions

- **Is anything else still legitimately writing to
  `selected_community_id` in SharedPreferences post-#1895?** —
  Recommendation: grep confirms only `setCommunities()` (lines 239,
  243) writes the new key. Both writes are in the auto-pick / migration
  branches that Option A removes. The new key has no other writers, so
  the one-shot cleanup can simply `prefs.remove(_selectedCommunityIdKey)`
  in setCommunities for one release cycle, then drop the cleanup line.
- **Should the rejoin flow's `selectCommunity(match)` call
  (`rejoin_community_screen.dart:156`) be deleted as part of this PR?**
  — Recommendation: yes, drop it. `selectCommunity` is already a
  no-op in release builds and asserts in debug; removing the caller
  prevents a debug crash for anyone exercising the rejoin path and
  matches the spirit of "the single-select model is gone."
- **Does the user need a server-side cleanup to recover, or will the
  next app launch fix her?** — Recommendation: next launch fixes her,
  because Option A clears the persisted key. No prod data is
  corrupted; the symptom is entirely in client SharedPreferences. We
  should reply to the issue confirming this once the fix ships.
- **Should we still fix the server-side `Deleted` filter leak in
  `ListCommunities`?** — Recommendation: yes, file as a follow-up.
  It's a latent bug — a user who left a community could see a ghost
  entry, and a user whose community was soft-deleted would see the
  shell. Not the cause here but easy and worth doing. Owner:
  `server/services/community/membership.go:145` add
  `WHERE deleted IS NULL` semantics, and have the response loop also
  skip `isSoftDeleted(community)`.

## Implementation Plan

### Phase 1: Stop persisting and restoring single-community selection

**Goal:** Ship a client-only fix that recovers all stuck prod users on
next app launch and unblocks the share picker.

**Tasks:**

- [ ] In `app/lib/services/providers/community_providers.dart`:
  - Change `setCommunities()` to always emit
    `selectedCommunityId: null`. Remove the new-key restore branch
    (lines 224-233) and the auto-pick branch (lines 241-244).
  - Keep the one-shot cleanup: in `setCommunities()`,
    `prefs.remove(_selectedCommunityIdKey)` and
    `prefs.remove(_legacyEnabledCommunityIdsKey)` so users who already
    have stale entries get them flushed.
- [ ] In `app/lib/presentation/widgets/sharing/community_picker_sheet.dart:57`:
  change `.enabledCommunities` → `.communities`.
- [ ] In `app/lib/presentation/screens/settings/rejoin_community_screen.dart:156`:
  delete the `selectCommunity(match)` call.
- [ ] **Rewrite tests in
  `app/test/services/selected_community_test.dart`.** Five tests in
  the `setCommunities` group currently codify the broken behavior and
  will fail after the code change. Replace them with tests that
  assert the post-fix invariants:
  - `auto-pick removed for sole community` — given an empty prefs
    map and `setCommunities([c1])`, expect `selectedCommunityId ==
    null`, `isAllCircles == true`, and that `selected_community_id`
    is **not** present in `SharedPreferences`.
  - `persisted selected_community_id is ignored and cleared` —
    seed `SharedPreferences.setMockInitialValues({'selected_community_id': 'c2'})`,
    call `setCommunities([c1, c2])`, expect
    `selectedCommunityId == null` and
    `prefs.getString('selected_community_id') == null` after the
    call (one-shot cleanup).
  - `legacy enabled_community_ids is ignored and cleared` — seed
    `{'enabled_community_ids': ['c2']}`, expect All Circles and that
    the legacy key has been removed.
  - `unknown persisted ID still resolves to All Circles` —
    keep the existing case structurally but assert that the legacy
    key clear runs even on the no-match path.
  - `enabledCommunities equals communities for any non-empty input` —
    new test in a separate `group('SelectedCommunityState getters')`
    block, no provider container needed: construct
    `SelectedCommunityState(communities: [c1, c2])` directly and
    assert `enabledCommunities` is the same list and
    `isAllCircles == true`.
  - The existing `SelectedCommunityNotifier.selectCommunity (neutered)`
    group stays as-is — the no-op-with-debug-assert contract is
    unchanged.
- [ ] `npm run test:app` green. Also run
  `npm run lint:dart:ephemeral-comments` and
  `npm run lint:dart:size` for the touched files (none should be over
  the 1000-line gate; they're well under).

**Workshop title note:** `workshop_screen.dart:66-69` currently passes
the FloatingHeader title through
`CommunityHelper.selectedCommunitiesTitle(selectedCommunityProvider)`.
With this fix, `enabledCommunities == communities` always, so a user
with two communities will see "2 Communities" instead of a single
community name. That is the intended post-#1895 portfolio-wide
framing, but it changes what stuck users see on relaunch (Mike's
header flips from "Oakwood Heights Neighbors" to "2 Communities").
That is correct and we do not change it in this PR. If product
prefers the header to honour the Workshop carousel pin
(`workshopCommunityProvider`) and show the pinned community's name,
file that as a separate follow-up — the data is already there.

**Validates:** the user (and any other prod user in the same state)
recovers on relaunch; share picker shows the full membership list; no
regression in the rest of the app.

### Phase 2: Reply to the issue, monitor, ship

**Goal:** Confirm with the reporter that the fix lands her community
back.

**Tasks:**
- [ ] Comment on #1971 and #1968 referencing the PR, noting that the
  fix activates on app relaunch — no server-side action needed.
- [ ] Include in the release notes for the next app build.
- [ ] After release, watch for any "communities list empty" or
  "stuck on All Circles" reports for a week. None expected.

**Validates:** reporter confirms, no new regressions.

### Phase 3 (follow-up, separate PR): Delete the dead single-select model

**Goal:** Remove the underlying confused-state model so this class of
bug can't recur.

**Tasks:**
- [ ] Remove `selectedCommunityId`, `selectedCommunity`,
  `effectiveCommunityIds`, `enabledCommunityIdList`,
  `enabledCommunities`, `isAllCircles`, `selectCommunity`, `switchTo`,
  and `_persistSelection` from `SelectedCommunityState` /
  `SelectedCommunityNotifier`.
- [ ] Migrate ~15-25 call sites currently reading
  `.enabledCommunities` / `.selectedCommunity` /
  `.effectiveCommunityIds` to `.communities` /
  `.communities.map((c) => c.id).toList()` directly.
- [ ] Delete both SharedPreferences keys' constants once everyone is
  past the one-shot cleanup window (one release).

**Validates:** no remaining surface area for "different views of the
same community list" bugs.

### Phase 4 (follow-up, separate PR): Server-side ListCommunities Deleted filter

**Goal:** Fix the latent server-side bug noticed during investigation.

**Tasks:**
- [ ] In `server/services/community/membership.go:145-191`, after
  fetching memberships and communities, filter out entries where the
  membership is soft-deleted (`membership.Deleted != nil`) OR the
  community is soft-deleted (`isSoftDeleted(community)`).
- [ ] Add a Go test in
  `server/services/community/membership_test.go` covering both
  filters.
- [ ] `go test ./...` and `npm run build` green.

**Validates:** users who left a community or whose community was
soft-deleted no longer see a ghost entry on their ripls tab.

## Repro on a local server (the user's request)

To reproduce against a local server before shipping:

1. Run `npm run start:server` and the Flutter app pointed at it.
2. Sign in as a fresh user, join exactly one community ("CommA").
3. Force-quit and relaunch the app — the auto-pick branch
   (`community_providers.dart:241-244`) writes
   `selected_community_id = CommA.id` to SharedPreferences.
4. Join a second community ("CommB").
5. Force-quit and relaunch.
6. Verify the symptoms:
   - Top switcher shows both CommA and CommB.
   - Tapping CommB does nothing (debug build: assert; release:
     silent no-op).
   - Feed/items/events show only CommA's content.
   - Open the create-gear flow → tap the community selector → the
     `CommunityPickerSheet` shows only CommA.

Apply Phase 1 and confirm all four symptoms resolve on the next
relaunch.

## Related Issues
- #1968 — same user, same root cause; closes together.
- #1895 — PR that neutered `selectCommunity()` without clearing the
  persisted state; the regression originates there.
- #1657 / #1655 — soft-delete / rejoin work that introduced the
  `Deleted` field on `CommunityUser` and `Community`; relevant to
  the Phase 4 follow-up.
