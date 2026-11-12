# Issue #2255: Can't list correct helpers for request

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2255
**Filed:** 2026-05-30 | **Last activity:** 2026-05-30
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When the requester opens `MarkFulfilledModal` for a multi-community request
(one shared with more than one community), the modal cannot surface the right
helpers. The reporter (maintainer) wrote:

> It seems to have limited the set it will match to a single community but the
> contributions came from another community

Two distinct but related defects produce this symptom:

1. **The helper search/grid is single-community.** `MarkFulfilledModal`
   accepts a single `communityId` (the community the requester was viewing
   the request from) and passes it into `DarkPersonSearch`, which calls
   `searchMembers(communityId: …)` and `searchShadowUsers(communityId: …)`.
   Members of *other* communities the request is shared with — including
   people who actually contributed to the request — never appear in the
   quick-add grid or the search results, so the requester can't confirm
   them as helpers.
2. **Free-form contributors are not pre-populated.** `AddRequestContribution`
   does **not** call `ensureRequestOffer` (compare `ClaimRequestNeed`, which
   does, in `server/services/request/needs.go:439`). A user who adds a
   free-form `PlanningContribution` is therefore absent from
   `request.offerers`. The modal only seeds its "TAP TO CONFIRM HELPERS"
   list from `request.offerers`, so contributors who never offered are
   invisible from the start.

Whichever path the requester tries — pre-populated list or search — helpers
who acted from another shared community (or who only contributed, never
offered) are unreachable.

The `request.offerers` payload itself is multi-community-correct:
`buildRequest` in `server/services/request/helpers.go:33` queries
`RequestOffer` rows by `request_id` only, with no community filter. The bug
is on the modal side and in the `AddRequestContribution`/`ensureRequestOffer`
asymmetry, not in the request fetch.

## Context from Comments

- Only one comment: maintainer pinged `@claude` to triage. No human
  follow-up discussion narrowing scope, so the plan has to make a judgment
  call about whether to fix only the search (the direct quote) or also fix
  the contributor-not-pre-populated issue (the implied cause of "the
  contributions came from another community"). Both are reachable from the
  same UX surface — the modal — and one fix without the other still leaves
  the user frustrated.

## Potential Solutions

### Option A: Fan out searches client-side across `sharedCommunityIds` + pre-populate contributors

- **Approach:** Two parts:
  1. **Client search fan-out (frontend-only for search):**
     - Plumb `request.sharedCommunityIds` from `RequestContentView` through
       `MarkFulfilledModal` into `DarkPersonSearch` as a new
       `Set<String> searchCommunityIds` parameter (alongside the existing
       single `communityId` used for grid header copy and shadow-user
       create).
     - In `DarkPersonSearch._runSearch`, call `searchMembers` and
       `searchShadowUsers` once per community ID in parallel
       (`Future.wait`), deduplicate by user/shadow id, cap by `limit`.
       Preserve existing per-call limit semantics (10 default) — request
       `limit` per community and trim after merge.
  2. **Pre-populate contributors in the modal:**
     - Have `_handleMarkFulfilled` in `request_content_view.dart` read
       contributors from `requestNeedsProvider` (the existing viewmodel
       that already owns this data) and pass them as a new
       `contributors: List<User>` parameter to `MarkFulfilledModal`.
       Screens-talk-to-viewmodels per `docs/client/architecture.md`.
     - In `FulfillModalNotifier.initialize`, merge `offerers ∪ contributors`
       (dedupe by `user.id`) into the displayed helpers list. Default-
       confirm contributors the same way offerers are default-confirmed.
- **Pros:** No proto/server changes. Lands behind no migration. Touches a
  bounded set of files. Solves both the search and the pre-population gap
  the reporter described. The fan-out is N small queries (N = number of
  shared communities, typically 1–3), bounded by the request's
  `sharedCommunityIds`. Search latency stays acceptable because
  `searchMembers` already has a `limit=10` cap.
- **Cons:** N round trips per keystroke (per the existing debounce-less
  `onChanged: (v) => _runSearch(v.trim())` — debounce would be a useful
  follow-up but is out of scope here). One user can legitimately appear in
  multiple communities and needs deduping. Mark-fulfilled flow now depends
  on a successful `listNeedsAndContributions` fetch, which adds a failure
  mode (handle by treating empty contributor list as "no extras to
  pre-populate" — non-fatal).
- **Effort:** Small.

### Option B: New multi-community search RPC

- **Approach:** Add a new RPC `SearchUsersInCommunities` (and a shadow-user
  equivalent) that accepts `repeated string community_ids` and returns
  deduplicated users matching the query. Storage-layer change to
  `SearchCommunityUsersByName` to take a `[]string` and use `WHERE
  cu.community_id = ANY($1)`. Update the client search to call the new RPC
  with `sharedCommunityIds`.
- **Pros:** One round trip per keystroke regardless of community count.
  Cleaner long-term API. Server-side dedup. Makes the same primitive
  available to other completion flows (experiences, gear past-transfers,
  participant pickers) that have the same single-community-search shape.
- **Cons:** Proto change → buf generate, all the proto-convention
  considerations (separate request/response messages, no reuse). Adds a
  new RPC surface that requires auth checks across N communities
  (`RequireMemberOfActiveCommunity` per community, or a new
  `RequireMemberOfAnyCommunity`). Bigger blast radius for a user-visible
  bug fix. The bug is fixable client-side; this is a larger refactor
  arriving with the fix.
- **Effort:** Medium.

### Option C: Server-side — auto-create RequestOffer on AddRequestContribution

- **Approach:** Change `AddRequestContribution` in
  `server/services/request/contributions.go` to call `ensureRequestOffer`
  the same way `ClaimRequestNeed` does. This requires adding `community_id`
  to `AddRequestContributionRequest` (currently absent) so the offer can be
  attributed to a community. Then contributors automatically appear in
  `request.offerers`, fixing the pre-population half of the bug.
- **Pros:** Restores the invariant that "anyone who acted on the request
  has a RequestOffer record." Simplifies downstream code that already
  assumes offerers == all helpers (e.g., notifications, stats).
- **Cons:** Doesn't fix the cross-community search at all — even if every
  contributor has an offer, the requester still can't find them in the
  search if they're in another community. Requires a proto field addition
  (`community_id`) and a client update at every call site of
  `AddRequestContribution`. Doesn't address the reporter's literal
  complaint ("limited to a single community").

## Recommendation

**Option A.** It directly addresses what the reporter saw — the modal can't
find or pre-populate helpers from other shared communities — without proto
churn. The fan-out cost is bounded (`sharedCommunityIds` is typically tiny)
and the change is contained to the Flutter layer plus an existing data
fetch we already use elsewhere on the request screen.

Option C is a legitimate follow-up server-side cleanup (it would also fix
inconsistency between `ClaimRequestNeed` and `AddRequestContribution`), but
filed as a separate issue — it's an orthogonal data-model invariant change
and shouldn't ride with a user-visible bug fix.

Option B is the right long-term shape if more completion flows want the
multi-community search primitive, but is over-investment for this single
bug.

## Open Questions

- **Should the contributor fetch block modal open, or load asynchronously?**
  — Recommendation: fetch contributors *before* opening the modal in
  `_handleMarkFulfilled` (we already have a non-loading entrypoint) but
  fall through to "no contributors" if the fetch fails or times out. The
  modal still functions; pre-population is a nice-to-have, not a hard
  dependency.
- **Should the search grid header still say "ADD FROM YOUR COMMUNITY" when
  fanned out?** — Recommendation: change to "ADD FROM YOUR COMMUNITIES" (or
  drop the possessive: "ADD FROM THE COMMUNITY"/"ADD A HELPER") when more
  than one community is in scope. Minor copy follow-up inside
  `dark_person_search.dart`.
- **Should we add a debounce to `_runSearch`?** — Recommendation: out of
  scope — `searchMembers` is cheap, the modal is short-lived, and adding
  debounce belongs in its own UX polish PR. Document the absence of
  debounce in a follow-up issue if needed.
- **What about the experience completion modal (same pattern, single
  community)?** — Recommendation: out of scope — issue #2255 is request-
  specific. The same fix shape applies to `mark_completed_modal.dart` for
  experiences if/when reported. Note in PR description as future work.
- **Should shadow users be deduped across communities the same way?** —
  Recommendation: yes — dedupe by `shadow.id` after fan-out, identical to
  member dedup. Shadow users are community-scoped but the id namespace is
  global.

## Implementation Plan

### Phase 1: Fan out `DarkPersonSearch` across `sharedCommunityIds`

**Goal:** Helper search and quick-add grid surface members from every
community the request is shared with.

**Tasks:**
- [ ] In `app/lib/presentation/widgets/completion/dark_person_search.dart`,
  add a `Set<String>? searchCommunityIds` constructor parameter. When
  null or empty, fall back to `{communityId}` (preserving experience-
  modal and past-transfer-modal behavior unchanged).
- [ ] Update `_runSearch` to iterate `searchCommunityIds ?? {communityId}`,
  call `searchMembers`/`searchShadowUsers` per community in parallel
  via `Future.wait`, then deduplicate the merged lists by `id`.
  Preserve `repo.searchMembers`/`repo.searchShadowUsers` as the only
  data-access entrypoints — this widget already routes through
  repositories, not services directly (matches `docs/client/architecture.md`
  layer rule "ViewModels are the ONLY layer that calls Repositories";
  this widget is a presentation primitive and the call shape is
  unchanged).
- [ ] Trim each merged list down to `10` after dedup (keep the existing
  cap visible to the grid).
- [ ] Move the existing hardcoded grid header
  (`'ADD FROM YOUR COMMUNITY'`) into ARB. Add
  `completionAddFromYourCommunity` (singular) and
  `completionAddFromYourCommunities` (plural) in both
  `app/lib/l10n/app_en.arb` and `app/lib/l10n/app_es.arb` with `@key`
  metadata. Read via `context.l10n.*` and choose plural form from
  `searchCommunityIds.length > 1`. This both fixes the multi-community
  copy and pays down the pre-existing hardcoded-string violation in
  the file we're already editing (per `docs/client/i18n.md`). Run
  `npm run generate` so `AppLocalizations` regenerates.
- [ ] In `app/lib/presentation/screens/request/mark_fulfilled_modal.dart`,
  add a `Set<String> sharedCommunityIds` constructor parameter (default
  `const {}`). Pass it through to the `DarkPersonSearch` widget as
  `searchCommunityIds`.
- [ ] In `app/lib/presentation/screens/request/request_content_view.dart:684`
  (`_handleMarkFulfilled`), pass
  `sharedCommunityIds: request.sharedCommunityIds.toSet()` to
  `MarkFulfilledModal.show`. The screen reads from `requestProvider`
  (a viewmodel), not directly from a repository — matches the
  MVVM rule in `docs/client/architecture.md`.
- [ ] Add a widget test in
  `app/test/presentation/widgets/completion/dark_person_search_test.dart`
  (or extend the existing one) asserting that providing two community
  IDs causes two `searchMembers` calls and that duplicate users are
  deduped. Widget tests need the `localizationsDelegates` /
  `supportedLocales` setup from `app/test/helpers/l10n_helpers.dart`
  (per `docs/client/i18n.md`).

**Validates:** From a request shared with communities A and B, searching
"alex" from either community returns alex@A and alex@B together; selecting
either one adds them as a helper.

### Phase 2: Pre-populate contributors in the modal

**Goal:** People who contributed via `AddRequestContribution` show up in
"TAP TO CONFIRM HELPERS" without the requester having to search.

**Tasks:**
- [ ] In `_handleMarkFulfilled` in `request_content_view.dart`, after
  reading `requestProvider`, also read the cached
  `requestNeedsProvider(widget.requestId).contributions` (a
  viewmodel — adheres to MVVM "screens only talk to viewmodels" rule
  in `docs/client/architecture.md`). Build a `List<User>` of unique
  contributors (`distinct by contributor.id`, excluding the requester).
- [ ] Pass that list as a new `contributors: List<User>` parameter on
  `MarkFulfilledModal` / its `.show()` static.
- [ ] In `fulfill_modal_view_model.dart` `FulfillModalNotifier.initialize`,
  accept `List<User> contributors` and merge with `offerers` (dedupe by
  `id`). Default-confirm contributors the same way offerers are.
- [ ] Defensive: if `requestNeedsProvider` hasn't loaded yet, kick its
  initial load and proceed without contributors rather than blocking
  the modal open. Document with a one-line comment near the read.
- [ ] Add a unit test in
  `app/test/presentation/viewmodels/fulfill_modal_view_model_test.dart`
  covering: contributors-only (no offerers), contributors + overlapping
  offerers (deduped), contributors-empty (current behavior preserved).
  Test format mirrors the existing
  `fulfill_modal_view_model_test.dart`: `ProviderContainer` +
  `@GenerateMocks` on `RequestRepository`/`ImpactMetricsRepository`,
  per `docs/client/testing.md`.

**Validates:** Opening MarkFulfilledModal on a request where Carol added a
free-form contribution from community B shows Carol in the helpers list
pre-confirmed, even when the requester opened the modal from community A.

### Phase 3: Manual verification + release note

**Goal:** Confirm the fix works on a real multi-community request and
write a short note for the changelog.

**Tasks:**
- [ ] Run `npm run build` and `cd app && flutter test --reporter expanded`
  to confirm the test additions pass and nothing regresses. Also run
  `npm run lint:dart:l10n-parity` since Phase 1 adds ARB keys in both
  locales.
- [ ] Manual: create a request shared with two communities, have a member
  from each community add a contribution, open MarkFulfilledModal from
  the requester's POV in community A; verify the helper from community
  B appears pre-confirmed and is searchable.
- [ ] Mention in the PR body that the same pattern exists in
  `mark_completed_modal.dart` (experiences) and
  `past_transfer_modal.dart` (gear); flag as a follow-up if/when
  reported.

**Validates:** Real device walk-through matches the unit/widget tests.

## Audit Notes (added by ripls-audit)

Cross-checked against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/client/accessibility.md`,
`docs/client/testing.md`, `docs/server/architecture.md`, and
`docs/server/observability.md`. Findings folded inline into the phases above;
captured here for the reviewer's quick scan:

- **MVVM compliance (architecture.md).** Original phases had the screen
  reading "from `requestNeedsProvider` (or the repository directly)" —
  amended to drop "or the repository directly". Screens read viewmodels,
  viewmodels read repositories. The `DarkPersonSearch` presentation
  widget's existing `repo.searchMembers` / `repo.searchShadowUsers`
  call shape stays unchanged.
- **i18n (i18n.md).** `dark_person_search.dart` currently has a
  hardcoded English string (`'ADD FROM YOUR COMMUNITY'`). The
  `npm run lint:dart:i18n` script's grep doesn't catch this pattern, so
  it slipped in — but the convention is unambiguous. The plan now moves
  it (singular + new plural) into both ARB files in the same PR rather
  than introducing a second hardcoded string. `npm run lint:dart:l10n-parity`
  added to Phase 3 to enforce parity.
- **Accessibility (accessibility.md).** No regressions expected —
  `DarkPersonSearch` already composes `Tappable` for the grid and search
  field, results use `DarkSearchResultTile`, and the modal already opens
  via `showAccessibleModal`. The new ARB strings are plain `Text`
  copy, no `semanticsLabel` needed.
- **Caching (caching.md).** No new cache surfaces; the plan reuses the
  existing `requestNeedsProvider` cache for contributors and leaves
  `searchMembers` (uncached on the client by design — it's a live
  query) alone.
- **Testing (testing.md).** Phase 1's widget test is the right level
  for verifying the fan-out call count; Phase 2's viewmodel test is the
  right level for the pre-population dedup logic. Both layers match the
  doc's "test business logic in viewmodels, widget-level only when
  necessary" guidance.
- **Server (architecture.md, observability.md).** Recommended Option A
  is client-only; no server changes, so no new logging/observability
  obligations. Option C (recommended as a follow-up issue, not in
  scope) would touch `AddRequestContribution` and would inherit the
  existing `LoggerWithContext` + `target_request_id` field conventions
  already established in `server/services/request/`.
- **No contradictions found** between the plan and existing
  architecture decisions.

## Related Issues

- None on file at write time. Likely follow-ups (file as new issues during
  PR review if appropriate):
  - **Server-side parity:** Make `AddRequestContribution` call
    `ensureRequestOffer` so contributors always have a `RequestOffer`
    row (Option C above). Requires adding `community_id` to
    `AddRequestContributionRequest`.
  - **Same fix in experience completion modal and past-transfer modal**
    if the pattern is reported there.
  - **Add debounce to `DarkPersonSearch._runSearch`** to reduce per-
    keystroke RPC volume now that fan-out multiplies it by N.
