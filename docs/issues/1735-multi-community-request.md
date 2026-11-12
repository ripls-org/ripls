# Issue #1735: Create Request should support multi-community share like Gear and Events

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1735
**Filed:** 2026-05-05 | **Last activity:** 2026-05-05
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The Create Request CTA dead-ends with the snackbar "Please select a community
first" in two distinct, very common situations:

1. **New single-community user** — after signing up and accepting one invite,
   `SelectedCommunityNotifier.setCommunities` resolves to All Circles
   (`selectedCommunityId == null`) because no selection has been persisted yet.
   The user has exactly one community to post to, but the gate fires anyway.
2. **Multi-community user in All Circles** — the natural state after deleting
   the previously-selected community (or restoring one). The user sees their
   feed across every community, then taps Create Request and is told to "select
   one first."

Create Gear and Create Event don't have this problem: they open into a preview
modal that initializes `_selectedCommunityIds = enabledCommunityIdList` and let
the user adjust via `CommunityPickerSheet` before saving.

The fix is mostly already in place. `RequestPreviewModal` (the second-stage
preview that follows `RequestCreationModal`) **already** has the
`_selectedCommunityIds` list, the `CommunityPickerSheet` integration, and
the `submitRequest` + `shareRequest` fan-out wired through
`gen_request_view_model.dart`. Server-side `ShareRequest` exists and works.
The only thing keeping the bug alive is the pre-modal gate in two call sites
(`home_screen.dart`, `empty_content_state.dart`) — and the fact that
`RequestCreationModal` still requires a `communityId` constructor argument
that nothing in its body ever reads.

## Context from Comments

The only comment on the issue is `@claude` from a maintainer (maintainer)
asking for triage. The issue body is a thorough first-author write-up that
identifies four ranked fix options (in-modal multi-picker, auto-pick when
exactly one community, fall back to `enabledCommunities.first`, better
snackbar copy) and recommends shipping options 1 + 2 together. There are no
revised requirements or maintainer counter-proposals to reconcile.

The codebase research adds one finding the issue body did not have:
**option 1 is already 90% implemented.** The preview modal's
multi-community machinery is complete; the work is removing the gate, not
building a new picker.

## Potential Solutions

### Option A: Remove the gate, drop the dead `communityId` arg, auto-pick on single-community

**Approach:**
1. **`request_creation_modal.dart`** — drop the `communityId` constructor
   parameter entirely. Search the file: `widget.communityId` is never read.
   It only exists in the constructor and in the `show()` factory. The
   modal proceeds straight to `RequestPreviewModal`, which has its own
   community picker.
2. **`home_screen.dart:103-124`** and
   **`empty_content_state.dart:53-72`** — remove the `community == null`
   gate and the snackbar. Replace with a check on
   `selectedCommunityProvider.communities.isEmpty` (genuinely zero
   communities) and a clearer copy: "Join a community to post a request"
   (new ARB key, `requestCreateNoCommunities`).
3. **`community_metrics_new_stage.dart:187,408`** and
   **`nudge_content_view.dart:96`** — update the two remaining call sites
   to `RequestCreationModal.show(context)` (no positional/named arg).
4. **`community_providers.dart:217-227`
   (`SelectedCommunityNotifier.setCommunities`)** — when the new key is
   absent and the legacy migration didn't fire, but `validIds.length == 1`,
   treat the single community as the resolved selection and persist it.
   Two-line change inside the `else` branch. This independently fixes the
   "new user with one community lands in All Circles" case for every
   surface that reads `selectedCommunity`, not just request creation.

- **Pros:** Smallest possible change for the largest user-visible win.
  Closes both bug paths. Leaves the proven multi-community submit pipeline
  (`submitRequest` + `shareRequest`) untouched. Removes dead code.
  No proto changes, no server changes, no migration. The auto-pick
  semantics are intuitive — a user with one community has only one
  meaningful selection.
- **Cons:** The auto-pick changes the persisted-preferences shape for
  single-community users (we now write the new key on first load). This
  is forward-only — no rollback story for "actually I wanted All Circles
  with one community." Deemed acceptable because for a single-community
  user, "All Circles" and "that one community" are the same view.
- **Effort:** Small. ~80 lines of changes across 4-5 files. ~3 unit tests.

### Option B: Just remove the gate, no auto-pick

**Approach:** Steps 1, 2, 3 from option A. Skip step 4
(`setCommunities` auto-pick).

- **Pros:** Even smaller diff. No SharedPreferences write-on-first-load
  behavior change. The new-user-with-one-community path still works
  because the gate is gone — the modal opens and the picker pre-selects
  the only community via `enabledCommunityIdList`.
- **Cons:** Misses the broader "All Circles is wrong default for a
  single-community user" theme that the issue calls out. Other surfaces
  that read `selectedCommunity` (analytics events, nudges, invite flows)
  still see the unhelpful null. The issue body explicitly recommends
  shipping 1 + 2 together for this reason.
- **Effort:** Small minus. ~50 lines.

### Option C: Fall back to `enabledCommunities.first` in the gate (the issue's option 3)

**Approach:** Leave `RequestCreationModal` and `setCommunities` alone.
At the gate sites, when `selectedCommunity == null` but
`enabledCommunities.isNotEmpty`, pick `enabledCommunities.first.id` and
proceed.

- **Pros:** Smallest possible diff (~10 lines).
- **Cons:** Picks an arbitrary community for the user, which is the wrong
  default for a multi-share post. The user wanted to share to all and
  silently gets one. The issue body explicitly rejects this as a
  long-term solution. Doesn't help the dead-arg problem.
- **Effort:** Small minus minus.

## Recommendation

**Ship option A.** The issue author already did the architectural research
and recommended this combination; the codebase research above confirms
that the heavy lifting (multi-community picker, fan-out submit, server
support) is already done and option A is mostly cleanup. The only judgment
call is the auto-pick step, and it's independently correct: a user with
exactly one community has nothing to gain from the All Circles distinction.

Option B is a viable scoped-down fallback if the auto-pick raises any
concern during review (e.g., interaction with future onboarding flows).
Option C is rejected for the reasons in the issue body.

## Open Questions

- **Should `requestCreateNoCommunities` snackbar route the user anywhere
  (e.g., open `CommunityCreationModal` or a discover screen) instead of
  just showing copy?** — Recommendation: keep it as a snackbar in this PR.
  Routing logic lives outside the request flow's concern and a user with
  zero communities is already on a screen that exposes the
  `_onCreateCommunityTap` button (`empty_content_state.dart:151-164`).
  Don't expand scope. Track follow-up if product wants the deeper
  handhold.
- **Does removing the constructor `communityId` from
  `RequestCreationModal` need a deprecation period for any external
  callers?** — Recommendation: no. Repo grep shows 5 internal call sites
  and no external publishers; this is a private app surface.
- **Should the auto-pick in `setCommunities` also fire when
  `validIds.length > 1` and the user has *never* selected a community
  (true cold-start)?** — Recommendation: no. Multi-community users
  legitimately default to All Circles; auto-picking arbitrary first
  community would regress the multi-community-user-in-All-Circles use
  case the issue is trying to fix. Single-community-only is the
  surgical signal.
- **The `RequestPreviewModal` already validates
  `_selectedCommunityIds.isNotEmpty` and shows
  `communitySelectRequired` if not — but the *creation* modal currently
  doesn't get there if there are no communities. After option A's gate
  changes, can a user reach the preview with zero communities?** —
  Recommendation: no. The `requestCreateNoCommunities` check happens
  before `RequestCreationModal.show`; if it fires, we never open the
  modal. Preview's existing validation remains as defense in depth.

## Implementation Plan

### Phase 1: Gate removal + dead-arg cleanup

**Goal:** Land the user-visible fix end-to-end without changing
`setCommunities` behavior. After this phase, both bug paths from the issue
are closed by virtue of `RequestPreviewModal`'s existing multi-community
picker.

**Tasks:**
- [ ] `app/lib/presentation/screens/request/request_creation_modal.dart`:
  remove the `communityId` field, remove it from the constructor, remove
  it from the `show()` factory signature, remove it from the docstring.
- [ ] `app/lib/presentation/screens/home/home_screen.dart` (~lines
  103-124): replace the `selectedCommunity == null` gate with a check on
  `selectedCommunityProvider.communities.isEmpty`. New snackbar copy:
  use a new ARB key `requestCreateNoCommunities` ("Join a community to
  post a request" — final wording during ARB review). Drop the
  `communityId:` named arg from the `RequestCreationModal.show` call.
- [ ] `app/lib/presentation/widgets/empty_content_state.dart` (~lines
  53-72): same change — empty-communities check, new snackbar copy
  via the same ARB key, drop `communityId:` from
  `RequestCreationModal.show`.
- [ ] `app/lib/presentation/screens/impact_metrics/widgets/community_metrics_new_stage.dart`
  (lines 187 and 408): drop `communityId: communityId` from both
  `RequestCreationModal.show` calls. The metrics screen already scopes to
  a known community; the picker inside the preview modal will pre-select
  `enabledCommunityIdList`. **This is an intentional scope change**: a
  user opening request creation from the community-metrics card now
  sees the same multi-community picker behavior as from the home screen.
  If product prefers to retain the single-community pre-selection on
  this surface, defer this call site to a follow-up — but do *not*
  reintroduce the constructor param to keep both behaviors compatible.
- [ ] `app/lib/presentation/widgets/nudge/nudge_content_view.dart`
  (~line 96): drop the `feedProvider.currentCommunityIds.first` lookup
  that was only there to feed the dead arg; switch to
  `RequestCreationModal.show(context)`. The fallback `else` branch that
  currently routes to `ExperienceCreationModal` becomes unreachable
  for the `ask_for_help` action — remove it (the modal handles
  zero-community state).
- [ ] `app/lib/l10n/app_en.arb` and every sibling ARB: add
  `requestCreateNoCommunities` with description and value. Run the l10n
  generator (`flutter gen-l10n`) so `app_localizations.dart` updates.
- [ ] Add a widget test for `EmptyContentState` (new file
  `app/test/presentation/widgets/empty_content_state_test.dart`) covering:
  (a) zero communities → tapping Share opens plus button modal that
  shows the new "Join a community to post a request" snackbar when
  Request is tapped; (b) one or more communities → no snackbar fires.
- [ ] Add a unit test in
  `app/test/presentation/screens/request/request_creation_modal_test.dart`
  (new file) verifying `RequestCreationModal.show(context)` compiles and
  opens without a `communityId` argument.

**Validates:** Both gate paths the issue describes are closed; the dead
constructor argument is gone; tests prove the new behavior.

### Phase 2: Auto-pick the only community in `setCommunities`

**Goal:** Promote the broader UX fix from the issue's option 2 — a
single-community user lands directly on that community instead of in
All Circles.

**Tasks:**
- [ ] `app/lib/services/providers/community_providers.dart` (~lines
  217-232): inside the `else` branch that handles "new key absent",
  after the legacy-migration check, add a `validIds.length == 1`
  branch that sets `resolvedId = validIds.first` and persists via
  `prefs.setString(_selectedCommunityIdKey, resolvedId)`. Update the
  surrounding log line.
- [ ] Update `app/test/services/selected_community_test.dart`: add a
  test "setCommunities auto-picks when there is exactly one community
  and no persisted selection" that asserts both `selectedCommunity?.id`
  and the persisted `selected_community_id` value.
- [ ] Update the existing `'sets selectedCommunity in the same emission
  as communities'` test (lines 48-64): the current assertion is
  `expect(capturedState?.isAllCircles, isTrue)` for a single-community
  setup with no persisted ID — that assertion now flips to `isFalse`
  with `selectedCommunity?.id == 'c1'`. Rename the test to reflect the
  new contract.

**Validates:** Brand-new single-community user no longer transiently
sees All Circles. Multi-community users (the test on lines 130-140)
still fall back to All Circles when a persisted ID is unknown — that
test stays green.

### Phase 3: Manual smoke + release notes

**Goal:** Validate the test plan from the issue body on a real device
before the PR moves out of draft.

**Tasks:**
- [ ] Brand-new user, single community, lands on home → community is
  auto-selected; tap Create Request → modal opens with picker
  pre-selecting that community.
- [ ] Multi-community user with All Circles selected → tap Create
  Request → modal opens, picker pre-selects all communities.
- [ ] Multi-community user → deselect all but one in picker → save →
  request appears only in that community (verify in DB / feed).
- [ ] Multi-community user → keep all selected → save → fan-out via
  `shareRequest` produces one CommunityRequest per selected community.
- [ ] Empty-state CTA in All Circles → same behavior.
- [ ] User with zero communities → snackbar reads "Join a community to
  post a request" (or final approved copy).

**Validates:** End-to-end UX matches issue's test plan.

## Related Issues

- **#1619 / #1717 / #1718** — community soft-delete work. Deleting the
  active community drops the user into All Circles; option A makes that
  state no longer brick request creation.
- **#1666 / #1674** — 0.19.1 fixes for the share-existing-item All-Circles
  bugs. Different code path
  (`gear_access_sheet_helper.dart`,
  `gear_sharing_audience_actions.dart`) but the same "All Circles
  shouldn't be a dead end" theme.

## Audit Notes

Reviewed against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/client/accessibility.md`,
`docs/client/testing.md`, `docs/client/create.md`,
`docs/client/sharing.md`, and `docs/server/architecture.md` /
`docs/server/observability.md`:

- **i18n.** The plan replaces a hardcoded string
  (`'Please select a community first'`) with
  `context.l10n.requestCreateNoCommunities` and adds the key to both
  `app_en.arb` and `app_es.arb` per
  `docs/client/i18n.md` ("Each new string needs both locales"). The
  existing hardcoded snackbar already violates the pattern and
  `npm run lint:dart:i18n` would catch a new one — formalize the new
  key with `@requestCreateNoCommunities` description metadata.
- **Accessibility.** No new tappable surfaces, icons, or modals are
  introduced. The existing `ScaffoldMessenger.showSnackBar` path is
  unchanged. No `a11y*` keys required for this fix. The
  `RequestPreviewModal._buildCommunitySelector` already uses
  `Tappable` with `context.l10n.a11yReqChooseCommunities` and is
  unchanged.
- **MVVM testing.** `docs/client/testing.md` says "Test ViewModels,
  not widgets." Phase 2's new test in
  `selected_community_test.dart` follows that rule (it tests
  `SelectedCommunityNotifier` via `ProviderContainer`). Phase 1's
  widget tests are an exception because the gate logic lives in a
  widget callback (`onRequestSomething`) that doesn't have an obvious
  viewmodel home — extracting it solely for testability would be
  scope creep. Keep the widget test scope narrow (one happy path,
  one zero-community path) and rely on the existing viewmodel tests
  for the deeper community-state semantics.
- **Caching.** No cache touches. `RequestRepository` already
  invalidates `request:*`, `community:*`, and feed/search caches on
  `submitRequest` and `shareRequest` (see
  `app/lib/data/repositories/request_repository.dart:412-441`).
  The shared-with-multiple-communities path inherits that
  invalidation path unchanged — no plan deltas required.
- **Server boundary.** No proto, RPC, or server-side change. The
  multi-community fan-out already runs through existing
  `RequestService.SubmitRequest` + `RequestService.ShareRequest`
  (`server/services/request/sharing.go`). The plan touches client
  code only.
- **Server observability.**
  `gen_request_view_model.dart:320-327` emits
  `RequestCreatedEvent` for `communityIds.first` only, even when the
  fan-out shares with N communities. This pre-dates this issue and is
  mildly under-counted analytics, not a correctness bug — call it out
  as a follow-up rather than an in-scope fix to keep the diff small.
- **Style.** Per CLAUDE.md "Phase markers in `*.go`/`*.dart` comments
  fail the build." The implementation must not write
  "Phase 1 of #1735" (or similar) into Dart source comments. PR
  description / commit body is the right home for that context.
- **Sharing pattern.** `docs/client/sharing.md` documents the canonical
  three-step pattern that gear/request/experience preview modals all
  share (initialise from `enabledCommunityIdList`, picker via
  `CommunityPickerSheet`, at-least-one-community submit guard).
  `RequestPreviewModal` already complies — confirmed during research.
  This plan removes a deviation rather than introducing one.

No blocking findings. Plan stands as written.
