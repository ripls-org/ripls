---
issue: 1720
title: 'Phase 3 of #1619: Sole-member-leave explainer + non-owner Leave polish'
---

# Issue #1720: Phase 3 of #1619: Sole-member-leave explainer + non-owner Leave polish

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1720
**Filed:** 2026-05-04 | **Last activity:** 2026-05-04
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/community_delete_and_leave.md (§§ 2.4, 2.6, 4, 7.3)

## Audit findings (2026-05-05)

Reviewed against `docs/client/{architecture,caching,i18n,accessibility,testing}.md`
and `docs/server/{architecture,observability}.md`. Most of the plan tracks
the conventions established in #1717/#1718/#1719/#1741. Changes applied
below:

1. **Cache fan-out for sole-member-leave was incomplete.** Original plan
   reused `LeaveCommunityNotifier`'s existing fan-out, which invalidates
   the **rejoinable-list** but **not the deleted-list**. When a sole
   member leaves, the server cascades the membership soft-delete into a
   *community* soft-delete (`leaveAsSoleMember` →
   `deleteCommunityInternal`). The caller is in the eligible-restorer
   snapshot, so the just-left community must appear in **RECENTLY
   DELETED**, not RECENTLY LEFT (§2.7's "rejoin requires the community
   to be active" gates the rejoin path entirely). Without invalidating
   `deletedCommunitiesCacheInvalidationProvider`, the deleted-list
   section stays stale until it auto-disposes. **Fix:** extend
   `CommunityRepository.leaveCommunity` to invalidate **both**
   list caches (cheap — `invalidateDeletedList` and
   `invalidateRejoinableList` are O(cache map ops)) so sole-member and
   non-sole-member callers both land on a fresh hub on next render.
   Belt-and-suspenders: defensible regardless of which branch the
   server takes.

2. **Eager-refresh-on-mount is a no-op without Riverpod-level
   invalidation.** Original plan said
   "call `repo.invalidate(communityId)` on governance-screen mount" to
   keep `numMembers` fresh. Per **#1740** (filed yesterday), invalidating
   the Stash cache layer does NOT propagate to the
   `communityProvider(id)` `FutureProvider.family` entry — the family
   holds its already-resolved `AsyncData(GetCommunityResponse)` until
   it autoDisposes. **Fix:** the eager refresh has to invalidate at
   *both* layers — `repo.invalidate(communityId)` AND
   `ref.invalidate(communityProvider(community.id))`. Marked the task
   accordingly. Once #1740 ships the second call becomes redundant
   (the new signal will refresh the family on event), but for now both
   are required.

3. **`Code.notFound` mapping order.** Original plan implied adding
   `Code.notFound` matching alongside the existing `_mapTypedFailure`
   suffix parser. The current parser only inspects messages when
   `e.code == Code.invalidArgument` or `Code.failedPrecondition`.
   `Code.notFound` is a different RPC code entirely; it needs its own
   branch *before* (or independent of) the suffix parsing — the
   message is not a discriminator. Reflagged in Phase 1 tasks.

4. **`Code.unauthenticated` is handled upstream.** Same as in
   #1721/#1719's VMs — `RpcUtils.executeRpc` routes unauthenticated to
   the `_onUnauthenticated` callback registered on
   `CommunityService`. The outcome-mapping never sees it. Documented as
   a comment in the VM (matches existing convention).

5. **A11y primitives explicit.** Sole-member screens use
   `ElevatedButton` (warning-coloured CTA), `TextButton` (Cancel),
   and `AppBarBackButton` (top-left). All three are accessible by
   default — Material widgets carry button-role semantics. No
   `Tappable` wrapping needed because there are no non-text gesture
   surfaces on these screens. SemanticAnnouncer fires alongside each
   snackbar (success path on the sole-member confirm; the
   community-deleted/notFound paths on the existing
   LeaveCommunityScreen and OwnerLeaveConfirmScreen).

6. **Picker fallback message refinement.** Once this issue ships, the
   sole-member empty state in `OwnerLeaveMemberPickerScreen` becomes
   defence-in-depth (only fires under cache mismatch). Updated copy
   recommendation in Open Questions: rename
   `communityOwnerLeaveSoleMember` to
   `communityOwnerLeaveSoleMemberFallback` and reword to "Looks like
   you're the only member — close this and try Leave again to use
   the right flow." Marks the path as a recovery rather than a
   first-class destination.

7. **Repository-test coverage** for the new repo-side invalidations
   (deleted-list + rejoinable-list both fire) added to Phase 1 tasks
   — mirroring the test that pinned the rejoinable-list signal in
   #1741 phase 1.

## Problem Statement

The current in-app **Leave Community** entry (in PR #1741, just opened)
branches only on caller-vs-owner. It misses the §2.4 case the server
already handles: a sole member leaving morphs into a 30-day soft-delete
of the community itself. Per spec the branch detection at tap time is
three-way:

1. `activeMemberCount == 1` (sole member, regardless of role) →
   explainer + confirm; success snackbar reframes the action as
   "deleted X" and routes to the deleted-list.
2. Else if owner → choice modal (#1718) → picker (#1719).
3. Else → non-owner Leave confirm (#1719 phase 2, in #1741).

#1741's picker has a defensive sole-member empty state that points
the user at #1720 but stops short of actually doing anything; this
issue replaces that stop-gap with the real flow.

The non-owner Leave path also needs two small polishing items per
§7.3:

- Snackbar copy includes the rejoin window hint
  ("You left X. You can rejoin within 30 days.") instead of the bare
  current "You left X".
- Two typed outcomes the server returns from #1656 that #1741's VM
  doesn't surface: `Code.failedPrecondition: community_deleted` and
  `Code.notFound`.

## Context from Comments

No comments. Body is detailed enough to plan from. The interesting
ambiguity to flag in Open Questions: where does branch detection
read `activeMemberCount` from, and how do we keep it fresh enough
that we don't route the wrong way under cache staleness (cf. the
sidebar staleness bug filed as #1740 — same root cause).

## Codebase reconnaissance

- **Current Leave entry**
  (`governance_screen.dart:160-184`, post-#1741): the membership-section
  Leave button branches only on `isOwner`. Owner → push
  `OwnerLeaveMemberPickerScreen`. Non-owner → push
  `LeaveCommunityScreen`. No member-count check. Both screens use the
  same `LeaveCommunityNotifier` from
  `presentation/viewmodels/leave_community_view_model.dart`.

- **Picker's sole-member fallback**
  (`owner_leave_member_picker_screen.dart:111-135`): after fetching
  `ListCommunityUsers`, if the filtered list is empty the screen
  renders a stop-gap card pointing at #1720. This issue's
  sole-member-leave flow takes over before the picker is reached, so
  the fallback becomes purely defensive (it'd only fire under
  staleness or if branching mis-classified).

- **`numMembers` source**: `GetCommunityResponse.numMembers`
  populated server-side by `communitylib.GetNumCommunityMembers`
  (`server/community/membership.go`) which counts active
  `CommunityUser` rows scoped to the community. Surfaced on the
  client via the `communityProvider(communityId)` `FutureProvider.family`
  in `services/providers/community_providers.dart:86-92`. Cache
  staleness on this is exactly the bug filed as **#1740**; sidebar
  shows '1 member' when the truth is 2 because the family entry's
  AsyncData persists across membership-change events.

- **`LeaveCommunityNotifier`**
  (`leave_community_view_model.dart`, post-#1741): sealed
  `LeaveOutcome` already covers idle / inProgress / success /
  missingNewOwner / candidateIsCaller / candidateNotMember /
  ownershipChanged / error. Two outcomes still missing:
  `communityDeleted` and `notFound`. Pattern is identical to the
  rejoin VM's discriminator parsing (`': community_deleted'` suffix);
  notFound maps from `Code.notFound`.

- **`LeaveCommunityScreen`** (non-owner branch): renders body
  `communityLeaveBody`, success snackbar `communityLeaveSuccess`,
  pops 2 routes back to Settings. Spec says success snackbar should
  read "You left {name}. You can rejoin within 30 days." — current
  ARB value is just "You left {name}". One ARB key tweak.

- **Settings → Communities deleted-list** (#1717 / merged): lives
  on the same Settings hub screen, below the active list. The
  sole-member-leave success path should land the user on Settings
  hub so they immediately see the just-deleted community in
  RECENTLY DELETED.

- **Existing tests to mirror**: `LeaveCommunityNotifier`'s 10 VM
  tests cover each typed outcome; add 2 (community_deleted / notFound)
  alongside.

## Potential Solutions

The shape is mostly fixed by spec. The interesting decisions are
where to do branch detection and how many new screens.

### Option A: branch detection via cached `numMembers`, two new screens

- **Approach:**
  - In `governance_screen.dart`, `ref.watch(communityProvider(id))`
    to get `GetCommunityResponse.numMembers`. Compute
    `isSoleMember = (numMembers ?? 0) <= 1` (treat null as the
    not-yet-loaded edge case → fall through to non-sole flow,
    which is safer than blocking on an explainer that turns out to
    be wrong).
  - Three-way branch on tap:
    - `isSoleMember` → push `SoleMemberLeaveExplainerScreen`.
    - Else `isOwner` → push `OwnerLeaveMemberPickerScreen` (existing).
    - Else → push `LeaveCommunityScreen` (existing).
  - **Two new screens:**
    1. `SoleMemberLeaveExplainerScreen` — title "You're the only
       member.", body, **Continue to leave** / **Cancel**. Continue
       pushes screen 2.
    2. `SoleMemberLeaveConfirmScreen` — body adapted for the delete
       framing ("On leave, {name} will be soft-deleted for 30 days.
       Cancel to keep the community."), warning-coloured **Leave
       Community** CTA + Cancel. On confirm dispatches the same
       `LeaveCommunityNotifier.leave(communityId)` (no
       `newOwnerUserId` — server detects sole-member and converts
       to soft-delete).
  - **VM tweaks:**
    - Add `LeaveOutcomeCommunityDeleted` and `LeaveOutcomeNotFound`
      to the sealed class.
    - Map `Code.failedPrecondition` with `: community_deleted`
      suffix → `communityDeleted`.
    - Map `Code.notFound` → `notFound`.
    - The two existing widget reactions (`LeaveCommunityScreen`
      and `OwnerLeaveConfirmScreen`) get switch arms for the new
      outcomes; the sole-member confirm gets its own success
      handling that lands on Settings hub instead of Governance.
- **Pros:** matches spec verbatim; reuses the existing VM and cache
  fan-out without copy-paste; #1741's defensive picker fallback
  becomes a true defence-in-depth measure rather than the
  user-visible path.
- **Cons:** depends on `numMembers` being reasonably fresh.
  Under #1740's staleness, an owner whose community just had a
  second member join might briefly route to the sole-member path
  (numMembers stuck at 1). The server would correctly
  soft-delete-with-snapshot but the user would have been routed
  through the wrong UX. Mitigations:
  - **Eager refresh on tap.** Call
    `repo.invalidate(communityId)` + re-resolve
    `communityProvider(id)` *before* branching. Costs one extra
    `GetCommunity` round-trip on every Leave tap.
  - **Trust + recover.** Branch on cached numMembers; if the
    server later returns a different reality the existing
    typed-outcome handling absorbs it.
  - **Wait for #1740.** Once that fix lands, async events
    refresh `numMembers` and staleness is much rarer.
- **Effort:** medium (~250-350 LOC: 2 screens + 2 outcome
  variants + ARB + 4-6 tests).

### Option B: branch detection via fresh `ListCommunityUsers` on tap

- **Approach:** at tap time, fetch `ListCommunityUsers(communityId)`
  unconditionally. Caller is the only entry → sole-member path.
  Else → existing branches. The picker fetches the same data, so
  pass the prefetched list down to avoid duplicate round-trips.
- **Pros:** branching is always correct (server is source of truth
  for member count).
- **Cons:** extra round-trip on every Leave tap (negligible —
  members list is small). Plumbing the prefetched list through to
  the picker is awkward (picker currently fetches in initState);
  reviewers will ask why. More invasive than (A).
- **Effort:** small-medium but with more wiring.

### Option C: server-side sole-member confirmation

- **Approach:** call `LeaveCommunity` unconditionally; let the
  server return a typed outcome distinguishing
  `would_convert_to_delete` (or use a `dry_run` flag). Client
  shows the explainer based on the response.
- **Pros:** server is single source of truth; no client-side
  branch logic.
- **Cons:** changes the server contract for a UX nicety; the
  user experiences the explainer **after** committing to Leave,
  which violates §1 "Bail out, always" — they need to confirm
  again. Awkward.
- **Effort:** medium (server + client + proto round-trip).

## Recommendation

**Option A.** Matches the spec, reuses the existing VM, and is
small. The staleness risk is real but bounded: the only way the
sole-member path fires incorrectly is if a second member joined
*and* the cached `numMembers` hasn't updated *and* the user hits
Leave between those two states. Even then the server's
`LeaveCommunity` does the right thing — it'd run the normal
non-owner-leave or owner-handoff path because there *is* a
co-member; the user just sees the wrong copy momentarily before
the snackbar / navigation completes. Once #1740 ships, the
window narrows further.

Couple Option A with a small belt-and-suspenders:
**eagerly call `repo.invalidate(communityId)` + re-watch on
governance-screen mount** so the freshest `numMembers` is read
once when the user lands on the screen — no per-tap round-trip.

## Open Questions

- **Where to read `activeMemberCount` from?** Recommendation:
  `ref.watch(communityProvider(community.id)).asData?.value.numMembers`
  in `governance_screen.dart`, with a null-treated-as-not-sole
  default. Fresh enough for the tap-time branch given the screen
  was just opened (which triggered the fetch). Once #1740 lands
  this gets even fresher. The sole-member empty state in
  `OwnerLeaveMemberPickerScreen` remains as defence-in-depth.

- **One screen or two for the sole-member flow?**
  Recommendation: **two screens** (explainer + confirm), per spec
  §2.4 "one extra screen before the leave confirm". The explainer
  carries the educational framing ("leaving deletes X for 30
  days"); the confirm carries the destructive CTA. Splitting
  matches the spec's mental model and keeps the §1 "Bail out,
  always" clear at each step. Cancel exits at either step.

- **Sole-member success destination.** Recommendation: pop all
  the way out to Settings hub (so the user lands on the
  RECENTLY DELETED section that #1717 ships). Spec says
  "Navigate to Settings → Communities". With the way the
  navigation stack works (Settings → Governance → Sole-member
  explainer → Sole-member confirm), `popUntil(route.isFirst)`
  pops back through to home. Then we want to push the user
  *into* Settings so they see the deleted-list. Implementation:
  pop until first route, then re-push Settings via
  `NavigationHelpers.pushScreen(SettingsHubScreen())`.

  Alternative: keep the user on Settings hub by popping just
  the screens we pushed (sole-member confirm + explainer +
  governance screen), landing on Settings hub directly (3 pops).
  Cleaner. Recommended.

- **Snackbar copy for sole-member success.** Recommendation:
  spec verbatim — `'{name} deleted. You can restore it from
  Settings → Communities for 30 days.'`. New ARB key
  `communityLeaveSoleMemberSuccess`.

- **Non-owner Leave snackbar.** Recommendation: append the
  rejoin window hint to the existing `communityLeaveSuccess`:
  `'You left {name}. You can rejoin within 30 days.'`. The
  current bare 'You left {name}' loses the discoverability of the
  recently-left section. Owner-handoff success snackbar
  (`communityLeaveOwnerHandoffSuccess`) keeps its current copy
  since the rejoin window applies but the user just gave up
  ownership — the rejoin hint isn't the leading signal.

- **Two new typed outcomes (`community_deleted`, `notFound`).**
  Recommendation: add to the sealed `LeaveOutcome`. Snackbar
  keys: `communityLeaveCommunityDeleted` ("Community has been
  deleted"), `communityLeaveNotFound` ("You are no longer a
  member"). Both pop back to Settings (community is no longer
  meaningfully accessible).

- **Branch detection lives in governance screen or in the VM?**
  Recommendation: in the screen. The VM is RPC-shaped and
  doesn't know which UI flow to surface; it just performs the
  leave. The screen reads `numMembers` + `isOwner` and routes.
  Same pattern as the existing branching (#1741 phase 2/3).

## Implementation Plan

### Phase 1: VM polish + repo cache fan-out for sole-member leave

**Goal:** `LeaveCommunityNotifier` returns
`LeaveOutcomeCommunityDeleted` and `LeaveOutcomeNotFound` for
the corresponding server responses, and
`CommunityRepository.leaveCommunity` invalidates BOTH the
rejoinable-list AND the deleted-list so sole-member-leave's
deleted-list surface refreshes immediately.

**Tasks:**
- [ ] Extend `LeaveOutcome` sealed class with
  `LeaveOutcomeCommunityDeleted` and `LeaveOutcomeNotFound`
  variants. Document `Code.unauthenticated` is handled upstream
  in a VM comment.
- [ ] Update `_mapTypedFailure` in
  `leave_community_view_model.dart` so it:
  - **First** branches on `Code.notFound` regardless of message
    (no suffix discriminator) → `LeaveOutcomeNotFound`.
  - Then matches `Code.failedPrecondition` with
    `: community_deleted` suffix →
    `LeaveOutcomeCommunityDeleted`.
  - Existing branches (`missing_new_owner_user_id`,
    `candidate_is_caller`, `candidate_not_member`,
    `ownership_changed`) remain unchanged.
  - Unrecognised → `LeaveOutcomeError(UserError)` as before.
- [ ] Extend `CommunityRepository.leaveCommunity` to invalidate
  both `invalidateDeletedList()` and the existing
  `invalidateRejoinableList()` after the RPC succeeds. The
  caller is in the deleted-snapshot when the server cascades
  to community-soft-delete; the deleted-list section needs to
  show the just-left community on next read.
- [ ] Update `LeaveCommunityScreen._onOutcomeChanged` to handle
  both new outcomes — both pop 2 routes back to Settings with
  the appropriate snackbar.
- [ ] Update `OwnerLeaveConfirmScreen._onOutcomeChanged`
  similarly (community-deleted from inside an owner-leave is
  weird but legal — community got deleted by another path while
  the owner was on the screen; pop 4 routes).
- [ ] ARB additions: `communityLeaveCommunityDeleted`
  ("Community has been deleted"), `communityLeaveNotFound`
  ("You are no longer a member"). English + Spanish + `@key`
  metadata.
- [ ] Refresh `communityLeaveSuccess` to include the rejoin
  window hint: 'You left {name}. You can rejoin within 30
  days.' Update Spanish equivalent.
- [ ] 2 new VM tests pinning each typed mapping
  (`Code.notFound` and `community_deleted` suffix).
- [ ] 1 new repo test pinning that `leaveCommunity` invalidates
  the deleted-list cache (mirrors the existing rejoinable-list
  test from #1741 phase 1).

**Validates:** the VM handles every documented server outcome
end-to-end; the repo's cache fan-out covers the sole-member
case before any UI is added; #1741's existing screens get the
new outcome surfacing without changing branch detection.

### Phase 2: Sole-member detection + explainer + confirm

**Goal:** sole-member-leave path is reachable from the Leave
button when `numMembers == 1`. Owner of a sole-member community
is routed here too (regardless of ownership).

**Tasks:**
- [ ] `governance_screen.dart`: add a `ref.watch` of
  `communityProvider(community.id)` to read `numMembers`. Add
  `isSoleMember = (numMembers ?? 0) <= 1`. Three-way branch:
  sole-member → explainer; else owner → picker; else →
  non-owner Leave. Defence-in-depth: keep the picker's
  sole-member empty state (rephrased per audit finding 6 to
  make it clear it's a recovery path, not a destination).
- [ ] **Eager refresh on mount.** In
  `_GovernanceScreenState.initState` (post-frame callback), if
  `_community != null`, call **both**:
  - `ref.read(communityRepositoryProvider).invalidate(community.id)` — clears the Stash cache key.
  - `ref.invalidate(communityProvider(community.id))` —
    invalidates the Riverpod family entry so the next watch
    re-fetches.
  Both are required per #1740's analysis (the Stash invalidation
  alone is a no-op on the family entry). This single mount-time
  refresh keeps the tap-time branch on a fresh `numMembers`
  without an extra round-trip per tap.
- [ ] `governance_screen.dart`: also gate the Danger Zone modal's
  Just-leave branch behind the same sole-member check —
  routing to the explainer instead of the picker if the
  community has dropped to one member since the screen opened.
- [ ] New screen
  `presentation/screens/governance/sole_member_leave_explainer_screen.dart`.
  Title: "You're the only member.". Body:
  "Leaving deletes {name} for 30 days. You can restore it from
  Settings → Communities during that time." Two buttons:
  warning-coloured **Continue to leave** (pushes confirm) and
  **Cancel** (TextButton, pops). No `SwipeToCloseMixin`.
- [ ] New screen
  `presentation/screens/governance/sole_member_leave_confirm_screen.dart`.
  Body: "On leave, {name} will be soft-deleted for 30 days.
  Cancel to keep the community." Warning-coloured **Leave
  Community** CTA dispatching
  `LeaveCommunityNotifier.leave(community.id)` (no
  newOwnerUserId — server detects sole-member and converts to
  soft-delete). Cancel TextButton.
- [ ] On `LeaveOutcomeSuccess` from the sole-member confirm,
  show snackbar `communityLeaveSoleMemberSuccess`
  ("{name} deleted. You can restore it from Settings →
  Communities for 30 days."), `SemanticAnnouncer.announce`,
  pop 3 routes (confirm + explainer + governance) so the user
  lands on Settings hub where the deleted-list lives.
- [ ] ARB additions: `communityLeaveSoleMemberTitle`,
  `communityLeaveSoleMemberExplainerBody` (with `{name}`),
  `communityLeaveSoleMemberContinueCta`,
  `communityLeaveSoleMemberConfirmBody` (with `{name}`),
  `communityLeaveSoleMemberSuccess` (with `{name}`). English
  + Spanish + `@key` metadata.
- [ ] Widget test: explainer renders correctly at boundary
  (`numMembers == 1`), Cancel at either step pops without
  firing the RPC, Continue advances to confirm.
- [ ] ViewModel-level test asserting branch detection picks the
  right path for each combination of (`isOwner` ×
  `numMembers ∈ {1, 2}`). Since branching is in the screen, this
  is a screen-level widget test that mounts the governance
  screen with overridden `communityProvider` data and asserts
  which screen the Leave tap pushes onto the navigator.

**Validates:** the four (ownership × sole-member) combinations
land on the correct screen; the explainer reads properly for an
owner of a sole-member community as well as a non-owner of a
sole-member community (the rare case where a non-owner is the
only member because the owner already left through the picker
and is still in their 30-day rejoin window — server allowed
that path because the leaving owner-handoff requires another
member, but the picker would have shown that other member, then
that other member could still leave).

### Phase 3: Manual verification + small polish (optional)

**Goal:** confirm the sole-member-leave path against a local
server; clean up any remaining UX papercuts.

**Tasks:**
- [ ] Manual run against local server with a single-member
  community. Tap Leave → explainer → continue → confirm →
  leave. Verify snackbar copy, navigation lands on Settings
  hub, RECENTLY DELETED section now shows the just-deleted
  community.
- [ ] Same path as the owner of a sole-member community.
- [ ] Tap Leave on a community where the second member just
  joined (verify branch detection picks non-sole path even if
  the cached numMembers is briefly stale; if not, file a fresh
  bug or wait for #1740).
- [ ] Verify the picker's sole-member empty state is no longer
  user-reachable through the normal branching, but still fires
  defensively if branching mis-classifies.

**Validates:** end-to-end correctness; any residual issues get
filed.

## Related Issues

- #1619 — parent.
- #1741 / open — combined Rejoin + owner-leave PR; #1720 closes
  the remaining sole-member case it left as a stop-gap.
- #1717 / merged — deleted-list + Restore screen; sole-member
  success deep-links here so the user discovers their just-
  deleted community is recoverable.
- #1718 / merged — Danger Zone choice modal; this PR also
  routes its Just-leave branch through the sole-member check
  before reaching the picker.
- #1656 / PR #1709 — server `LeaveCommunity` rewrite. Phase 4
  of #1656 (`leaveAsSoleMember` → `deleteCommunityInternal`)
  is what makes the sole-member soft-delete actually work.
- #1740 / open — sidebar staleness; same root cause as the
  branch-detection staleness risk this PR mitigates with
  defence-in-depth.
