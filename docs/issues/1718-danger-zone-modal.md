---
issue: 1718
title: 'Phase 3 of #1619: Danger Zone — Delete-vs-Leave-with-handoff choice modal'
---

# Issue #1718: Phase 3 of #1619: Danger Zone — Delete-vs-Leave-with-handoff choice modal

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1718
**Filed:** 2026-05-04 | **Last activity:** 2026-05-04
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/community_delete_and_leave.md (§§ 2.1, 2.3, 7.2)

## Audit findings (2026-05-05)

Reviewed against `docs/client/{architecture,caching,i18n,accessibility,testing,deep_linking}.md`
and `docs/server/{architecture,observability}.md`. Changes applied below:

1. **Riverpod read vs. watch.** Original Phase 2 said
   `ref.read(authStateProvider)` to compute the ownership gate, but the
   gate is computed during `build()` (it determines what's rendered).
   Per `docs/client/architecture.md` "Use `ref.watch()` in build methods,
   `ref.read()` in event handlers." Changed to `ref.watch`.

2. **Modal title primitive.** Added an explicit `ModalHeader` reference —
   `docs/client/accessibility.md` lists it as the canonical modal title
   wrapper (auto-applies `Semantics(header: true)`).

3. **Choice type.** Replaced "sealed class `DangerZoneChoice`" with a
   plain `enum DangerZoneChoice { delete, leaveWithHandoff, cancel }` —
   sealed class is overkill for three flat tagged values; the modal
   returns one of three constants via `Navigator.pop<DangerZoneChoice>`.

4. **Phase 3: extract a Notifier, don't inline state.** Original plan
   said "ViewModel test (DeleteCommunityNotifier if extracted, or
   inline state in screen)" — `docs/client/architecture.md` is clear
   that all business logic (RPC call, error mapping, cache fan-out)
   lives in a Notifier, not in `setState`. Removed the inline-state
   option; `DeleteCommunityNotifier` is required, mirroring
   `RestoreCommunityNotifier` from #1717.

5. **Phase 3 cache fan-out: explicit list, not "follow Restore VM."**
   Reviewers shouldn't have to read another file to know what
   invalidates. Expanded the fan-out into the explicit ten-repository
   list `RestoreCommunityNotifier._invalidateRelatedCaches` uses
   today, plus `selectedCommunityProvider.notifier.reloadCommunities()`
   so the side pane and "who can see" pickers update in lockstep
   (the second bug we caught while testing #1717).

6. **Phase 3 error mapping: add `PermissionDenied`.** Original mapped
   only `FailedPrecondition`. Server's `DeleteCommunity` enforces
   owner-only at the RPC layer; the client gate makes this rare, but
   stale-auth or post-transfer races can still trigger it. Added a
   third mapped outcome with its own snackbar key.

7. **Phase 3 logging.** Added `Logger('DeleteCommunityViewModel')`
   plumbing matching `RestoreCommunityViewModel`. Per
   `docs/server/observability.md` the request gets a server-side
   correlation ID via `X-Request-ID` (already emitted by
   `RpcUtils.buildHeaders` — no client change needed).

8. **Cancel-from-modal accessibility.** `showAccessibleModal`'s drag
   handle and barrier-tap already provide a Cancel path. The
   spec's required Cancel affordance is satisfied by the drag
   handle + an explicit `Cancel` row inside the sheet (belt and
   suspenders). Documented this so reviewers don't ask why both
   exist.

9. **Server prereq scope clarification.** The model proto already
   carries `owner_user_id` (#1619 Phase 1). The prereq is *only* an
   API-side surfacing — copy the existing field into the API
   responses in the service handlers. Marked as a one-line change per
   handler so it's not over-scoped.

## Problem Statement

Today the community-settings screen renders a single **Delete Community** entry
in the Danger Zone, visible to every member, that opens a "Coming Soon" stub
(`_DeleteCommunityScreen` in `governance_screen.dart`). Per
`docs/community_delete_and_leave.md` §2.1 / §7.2 it must:

1. Render the entry **only for the community owner** (currently shown to
   everyone).
2. On tap, present a three-way choice modal — *Delete for everyone*, *Just
   leave (transfer ownership first)*, *Cancel* — instead of jumping
   straight into a destructive flow.
3. Route the *Delete* branch to the existing confirmation, and the
   *Leave-with-handoff* branch to the member-picker shipped by sibling
   issue #1719.

The Cancel affordance is required by §1's "Bail out, always" principle. No
irreversible action should land from a single tap.

## Context from Comments

No comments on the issue. The body is detailed enough to plan from, with
one ambiguity to resolve up front (see Open Questions): the API surface
doesn't currently carry the `owner_user_id` field the issue tells the
client to gate on.

## Codebase reconnaissance

- **Danger Zone today** lives at `governance_screen.dart:88–155`. The
  `_buildDangerSection` always renders the *Delete Community* row
  (`_GovernanceAction` at line 149–154); there is no ownership gate.
  `_DeleteCommunityScreen` is the "Coming Soon" stub at line 337.
- **Existing leave entry** (`_LeaveCommunityScreen` at line 284) is also
  a stub — it'll need work in #1719/#1720 but Cancel-from-modal must be
  able to dismiss without touching it.
- **Ownership data on the wire is missing.** API
  `proto/ripls/api/community_service.proto`:
  - `CommunityItem` (lines 85–97) has `id`, `name`, `description`,
    `media_ids`. **No `owner_user_id`.**
  - `GetCommunityResponse` (lines 46–64) has `id`, `name`, `description`,
    `media_ids`, `num_members`, `max_members`. **No `owner_user_id`.**
  The model proto (`proto/ripls/models/community.proto`) does carry
  `owner_user_id` (#1619 Phase 1), but it never reaches the client.
  This is a **server prerequisite** that has to land first — same
  shape as the #1722 / #1723 prerequisites for #1717.
- **Modal primitives.** `showAccessibleModal` in
  `widgets/accessibility/show_accessible_modal.dart` is the codebase
  default for bottom sheets — restores focus on dismiss. The choice
  modal should use it. Per `docs/client/accessibility.md`, options
  inside the modal use `Tappable` with `semanticsLabel:` from
  `context.l10n`, and `ModalHeader` for the title (already wraps in
  `Semantics(header: true)`).
- **`auth.user.id`** for the gating comparison comes from
  `authStateProvider` (used in `settings_hub_screen.dart:33`).
- **i18n.** New ARB keys for the choice modal title, three options, and
  an a11y-prefixed semantics label per option. English + Spanish in the
  same PR per `docs/client/i18n.md`.
- **Tests.** ViewModel-first / `ProviderContainer` pattern; widget tests
  use the `localizedApp` helper from `test/helpers/l10n_helpers.dart`.

## Potential Solutions

The bulk of the work shape is fixed by the issue body. The decisions worth
calling out are how to surface ownership and how much the choice modal
should orchestrate vs forward.

### Option A: file the server prereq, then implement modal as a stateless three-row sheet

- **Approach:**
  1. **Server-side prereq (separate small issue, must land first):**
     add `owner_user_id` to `CommunityItem` and `GetCommunityResponse` in
     `proto/ripls/api/community_service.proto`; populate from the model
     in the service handlers. Mirrors the existing `id`/`name`/`description`
     plumbing.
  2. **Client gate:** in `_buildActions`, compute
     `final isOwner = community.ownerUserId == authState.user?.id;`,
     render the *Delete Community* `_GovernanceAction` only when
     `isOwner`. (`_LeaveCommunityScreen` stub stays visible — every
     member can leave.)
  3. **Choice modal:** replace `_DeleteCommunityScreen` (the stub) with
     a `showAccessibleModal` invocation that returns one of three sealed
     `DangerZoneChoice` values: `delete | leaveWithHandoff | cancel`.
     The action `onTap` opens the modal, switches on the result, and
     forwards to either the existing-but-stubbed Delete confirmation
     screen or the picker from #1719. Cancel is a no-op.
  4. **Delete confirmation:** ship a real Delete confirmation flow in
     this PR — it's the natural pair to the choice modal and small
     enough to bundle. (Replaces the dev-only hack we used to test
     #1717.) Calls `CommunityRepository.deleteCommunity(id)` (already
     exists from #1717's Phase 1) and reloads
     `selectedCommunityProvider`.
- **Pros:** matches the spec verbatim; clean server boundary; shipped
  end-to-end so the next merge actually unblocks owner-driven deletion.
- **Cons:** depends on the server prereq landing first → coordination
  cost. Bundling the Delete confirmation expands the PR vs. strictly
  the modal — but doing it any other way leaves a stub in place we'd
  immediately replace next ticket.
- **Effort:** medium (server prereq is small, modal + confirmation +
  a11y + i18n + tests is the bulk).

### Option B: ship the modal alone, leave both branches as stubs

- **Approach:** identical to A but stop after the modal renders — the
  *Delete* branch still opens the existing "Coming Soon" stub, the
  *Leave-with-handoff* branch shows a placeholder "needs #1719" toast.
- **Pros:** smallest PR; lets reviewers focus on just the modal +
  ownership gate.
- **Cons:** modal can't be tested end-to-end; we'd have to reintroduce
  the dev-only hack we just removed; ships no actual user-visible
  improvement (still no real delete path).
- **Effort:** small.

### Option C: skip the server prereq by pulling ownership from elsewhere

- **Approach:** instead of adding `owner_user_id` to the API, look it
  up via `ListCommunityUsers` (already returns the member set with
  some attributes) or via a dedicated `GetCommunityOwnership` RPC.
  Cache locally.
- **Pros:** no proto change.
- **Cons:** every Danger Zone render fires an extra RPC; ownership
  belongs on the community proto by design (it's a permanent attribute,
  not a query); adds a new RPC the spec doesn't ask for. Worse than A.
- **Effort:** medium (more code, more cache plumbing, no architectural
  benefit).

## Recommendation

**Option A**, with the server prereq filed as a separate small issue and
landed first — same playbook that worked for #1717's #1722/#1723
prerequisites.

Bundling the Delete confirmation into this PR (rather than gating it
behind another sibling) is the right call: it's small, it pairs
naturally with the choice modal, and it removes the temptation to keep
the dev-only hack around for "just one more sibling." It also gives
reviewers a complete owner-delete flow to manually test.

## Open Questions

- **API field placement.** Recommendation: add `owner_user_id` to *both*
  `CommunityItem` (for `ListCommunities`-driven surfaces like the side
  pane and the settings hub) and `GetCommunityResponse` (for governance
  detail screens). Each surface calls one or the other; not all of them
  fetch the full community. Cost is one optional string per response
  instance — negligible.

- **Where to gate the Delete entry.** Recommendation: pass the
  current user's id into `_buildActions` and gate on
  `community.ownerUserId == userId`. Don't gate inside
  `_buildDangerSection` itself — the section header should still
  render even if the only entry is hidden, so the layout doesn't
  jump (the header announces "Danger Zone — Leave is the riskiest
  thing here"). Owner-only entries hide cleanly in the row list.
  Edge case: if non-owners have no Danger Zone entries to render, fall
  back to suppressing the whole section — the existing
  `_LeaveCommunityScreen` row keeps it populated for them, so this
  shouldn't happen in practice.

- **Modal style.** Recommendation: bottom sheet (`showAccessibleModal`
  default), three rows + Cancel button at the bottom. Follows the
  Material guidance and matches the codebase's existing patterns for
  multi-action choices.

- **Copy.** Recommendation: lift §2.1 step 2 wording verbatim and
  drop into ARB keys —
  - `dangerZoneDeleteForEveryone` → `'Delete {name} for everyone'`
  - `dangerZoneJustLeave` → `'Just leave — let someone else take
     over'`
  - `commonCancel` → reuse if it exists, else add.
  Plus a11y semantic labels:
  - `a11yDangerZoneDeleteForEveryone` → `'Delete {name} for everyone'`
    (same — already imperative).
  - `a11yDangerZoneJustLeave` → `'Leave and pick a new owner'` (more
    specific than the visible chip; assistive tech benefits from
    clarifying the *outcome*).

- **What if the server prereq slips?** Recommendation: don't ship
  this issue as Option B (modal-only) just to unblock — the value
  is in the end-to-end Delete path, not the modal in isolation. If
  the prereq slips, ship #1719 first (which also needs ownership
  data, but in a different shape — picker is over `ListCommunityUsers`
  which is independently queryable) and let this issue wait.

- **Owner-leave deep-link from this PR?** Recommendation: **no**, not
  in this PR. Per spec §2.3, the owner-leave path forks into the
  member-picker. That picker is #1719's scope. This PR's "Just leave"
  branch should `Navigator.push` into the picker screen (filed in
  #1719) and stop there. If #1719 isn't merged when we're ready to
  land, gate this PR's "Just leave" branch behind a `_log.warning`
  + Cancel-style snackbar until it does.

## Implementation Plan

### Phase 1: server prereq — `owner_user_id` on API

**Goal:** the client can read who owns a community without an extra
query. File as a separate small issue (mirrors #1722 / #1723).

**Tasks:**
- [ ] File new server-side issue: "Surface `owner_user_id` on
  `CommunityItem` and `GetCommunityResponse`." Reference #1718 as
  consumer.
- [ ] Add `optional string owner_user_id = N;` to both messages in
  `proto/ripls/api/community_service.proto`.
- [ ] Populate from the model in the relevant service handlers
  (`server/services/community/membership.go` for `ListCommunities`;
  the `GetCommunity` handler for `GetCommunityResponse`).
- [ ] Round-trip test: server returns the field; client proto can read
  it after `npm run generate`.
- [ ] Merge before this PR.

**Validates:** every client surface that has a `CommunityItem` or
`GetCommunityResponse` in hand can identify the owner without an
extra round-trip.

### Phase 2: Danger Zone gate + choice modal

**Goal:** owner sees a three-way modal; non-owner sees no Delete
Community entry. No real Delete-confirmation flow yet — *Delete*
branch still goes to the existing stub.

**Tasks:**
- [ ] `governance_screen.dart`: read the current user via
  `ref.watch(authStateProvider).user?.id` inside `build()`, thread
  it into `_buildActions`, gate the *Delete Community*
  `_GovernanceAction` on `selectedCommunity.ownerUserId == userId`.
  (`ref.watch` not `ref.read` — gate runs during build, per
  `docs/client/architecture.md`.)
- [ ] New `danger_zone_choice_modal.dart` widget under
  `presentation/widgets/settings/`. Wraps the title in `ModalHeader`,
  renders three `Tappable` rows with localised semantics labels via
  `showAccessibleModal`. Returns a
  `enum DangerZoneChoice { delete, leaveWithHandoff, cancel }` via
  `Navigator.pop<DangerZoneChoice>`. The bottom sheet's drag handle
  + barrier tap also dismiss as Cancel; the explicit *Cancel* row is
  belt-and-suspenders for users who don't reach for the drag handle.
- [ ] Replace `_DeleteCommunityScreen` invocation in `_buildActions`
  with an `onTap` that opens the modal and switches on the result:
  - `delete` → open the existing `_DeleteCommunityScreen` stub for
    now (Phase 3 replaces it).
  - `leaveWithHandoff` → open the existing `_LeaveCommunityScreen`
    stub (or a new picker stub if #1719 hasn't merged).
  - `cancel` → no-op.
- [ ] ARB additions (English + Spanish, with `@key` metadata):
  `dangerZoneChoiceTitle`, `dangerZoneDeleteForEveryone` (with
  `{name}`), `dangerZoneJustLeave`, `commonCancel` (reuse if exists),
  `a11yDangerZoneDeleteForEveryone` (with `{name}`),
  `a11yDangerZoneJustLeave`.
- [ ] Widget tests:
  - non-owner does NOT see *Delete Community* entry.
  - owner taps *Delete Community* → modal renders three rows.
  - tapping *Delete for everyone* opens the existing stub.
  - tapping *Just leave* opens the leave stub.
  - tapping Cancel dismisses modal without side effects.

**Validates:** ownership gate works; modal correctly forwards to
each branch; cancel-from-modal is safe.

### Phase 3: real Delete confirmation flow

**Goal:** the *Delete for everyone* branch reaches a real
confirmation that calls `DeleteCommunity` and lands the user back in
Settings with the community removed.

**Tasks:**
- [ ] Replace the `_DeleteCommunityScreen` stub with a real
  `DeleteCommunityScreen` (public, parameter `community`). Body
  matches §2.1 step 3: name, member count, category-level impact
  summary ("gear shared, requests posted, events, chats become
  inaccessible"), 30-day restore reminder. One destructive CTA
  *Delete Community* (red), one Cancel via `AppBarBackButton`. No
  `SwipeToCloseMixin` (utility/settings screen, reached via
  `pushWithSlide`).
- [ ] New `DeleteCommunityNotifier` (`Notifier<DeleteOutcome>` with
  `SafeNotifierMixin`) under `presentation/viewmodels/`, sealed
  outcome class with `idle | inProgress | success | alreadyDeleted
  | notOwner | error(UserError)` — same shape as
  `RestoreCommunityNotifier`. Business logic lives here, not in
  the screen's `setState` — per `docs/client/architecture.md`
  "all business logic lives in Notifiers." Add
  `Logger('DeleteCommunityViewModel')` plumbing and `_log.info`
  / `.warning` lines mirroring the Restore VM. Server-side
  correlation flows via `X-Request-ID` already emitted by
  `RpcUtils.buildHeaders`.
- [ ] Wire CTA to `ref.read(deleteCommunityProvider.notifier)
  .delete(community.id, community.name)`. The notifier:
  1. Calls `CommunityRepository.deleteCommunity(id)` (already
     provided by #1717's Phase 1; already invalidates the
     deleted-list and refreshes user-communities).
  2. On success, runs the explicit cache fan-out (best-effort —
     a fan-out failure must NOT flip the outcome to error, same
     refactor we landed for the Restore VM):
     - `communityRepository.refreshAll(id)` (community-scoped:
       users, events, gear, completed_giveaways).
     - `gearRepository.invalidateForCommunity(id)`.
     - `requestRepository.invalidateForCommunity(id)`.
     - `experienceRepository.invalidateForCommunity(id)`.
     - `transferRepository.invalidateForCommunity(id)`.
     - `searchRepository.invalidateSearchesForCommunity(id)`.
     - `feedRepository.invalidateFeed()`.
     - `storyRepository.invalidateCommunity(id)`.
     - `impactMetricsRepository.invalidateAll(id)`.
     - `leaderboardRepository.invalidateLeaderboard()`.
     - `selectedCommunityProvider.notifier.reloadCommunities()`
       — pushes the refreshed list into the active-list state so
       the side pane and "who can see" pickers drop the deleted
       community immediately. Without this we re-introduce the
       second bug we caught while testing #1717.
  3. Maps `Code.failedPrecondition` → `DeleteOutcomeAlreadyDeleted`
     (race: another path already deleted).
  4. Maps `Code.permissionDenied` → `DeleteOutcomeNotOwner` (rare —
     the client gate makes this unlikely, but stale auth or
     post-transfer races can still hit it).
  5. Anything else → `DeleteOutcomeError(UserError)` via
     `RpcErrorHandler.classify`.
- [ ] Widget reaction:
  - `success` → snackbar `communityDeleteSuccess` → "Deleted {name}",
    `SemanticAnnouncer.announce(...)` the same string, pop back to
    Settings.
  - `alreadyDeleted` → snackbar `communityDeleteAlreadyDeleted` →
    "Already deleted", pop.
  - `notOwner` → snackbar `communityDeleteNotOwner` → "You're no
    longer the owner of this community", pop.
  - `error` → snackbar via `RpcErrorHandler.localize(error,
    context.l10n)`, stay on screen.
- [ ] ARB additions (English + Spanish, with `@key` metadata):
  `communityDeleteTitle`, `communityDeleteBody` (with `{name}`,
  member-count plural), `communityDeleteCta`, `communityDeleteSuccess`
  (with `{name}`), `communityDeleteAlreadyDeleted`,
  `communityDeleteNotOwner`.
- [ ] ViewModel test (`ProviderContainer` + Mockito, mirrors the
  Restore VM test): success path verifies the full fan-out fired;
  `FailedPrecondition` → `alreadyDeleted` (no fan-out);
  `PermissionDenied` → `notOwner` (no fan-out); generic error;
  disposal during async; `TestWidgetsFlutterBinding.ensureInitialized()`
  + `SharedPreferences.setMockInitialValues({})` so the
  `reloadCommunities` call doesn't blow up on
  `WidgetsBinding`/prefs.
- [ ] Widget test: cancel via `AppBarBackButton` does not call
  delete.
- [ ] Manual run against local server: navigate to community → Danger
  Zone → Delete Community → confirm modal → Delete for everyone →
  confirmation screen → Delete → community appears in Settings'
  Recently Deleted section (proves end-to-end with #1717).

**Validates:** owner can complete the entire delete flow in-app
without the dev-only hack; the §1 "Bail out, always" path works at
both modal and confirmation steps; the cache fan-out keeps the side
pane / "who can see" pickers honest.

## Related Issues

- #1619 — parent (community soft-delete with 30-day restore window).
- #1717 / PR #1728 — sibling client issue, deleted-list + Restore screen.
  This PR's Phase 3 confirmation flow is the natural Delete pair to
  #1717's Restore confirmation.
- #1719 — sibling client issue, owner-leave member picker. The "Just
  leave — let someone else take over" branch routes here. Same author
  may want to coordinate landing order.
- #1720 — sibling, sole-member-leave explainer + non-owner Leave polish.
- #1656 / PR #1709 — server `LeaveCommunity` rewrite (already merged).
- New issue (to file in Phase 1): "Surface `owner_user_id` on
  `CommunityItem` and `GetCommunityResponse`."
