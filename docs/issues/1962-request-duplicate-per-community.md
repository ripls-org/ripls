# Issue #1962: Request creation creating multiple copies

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1962
**Filed:** 2026-05-16 | **Last activity:** 2026-05-16
**Labels:** user-feedback | **Assignees:** tescobar
**Context docs:** none

## Audit Notes (2026-05-15)

This plan was audited against
[docs/client/architecture.md](docs/client/architecture.md),
[docs/client/caching.md](docs/client/caching.md),
[docs/client/i18n.md](docs/client/i18n.md),
[docs/client/accessibility.md](docs/client/accessibility.md),
[docs/client/testing.md](docs/client/testing.md), and
[docs/server/architecture.md](docs/server/architecture.md). Three issues
in the original draft were corrected below:

1. **Architecture violation (the most important fix).** The original draft
   said to call `requestService.shareRequest` directly from
   `_saveRequest`. That repeats the pre-existing architectural mistake
   already present at
   [unified_create_save_actions.dart:110-113](app/lib/presentation/viewmodels/unified_create_save_actions.dart#L110-L113),
   which reads `requestServiceProvider` and bypasses the repository
   layer entirely. Per
   [docs/client/architecture.md §Key Architectural Rules](docs/client/architecture.md)
   only repositories may call services, and ViewModels must go through
   repositories. The fix now routes the new call through
   `requestRepositoryProvider` instead, matching the proven pattern in
   [gen_request_view_model.dart:309-339](app/lib/presentation/viewmodels/gen_request_view_model.dart#L309-L339).
2. **Cache-staleness side effect.** Going through the repository is not
   just a stylistic preference: `RequestRepository.submitRequest`
   ([request_repository.dart:193-222](app/lib/data/repositories/request_repository.dart#L193-L222))
   and `RequestRepository.shareRequest`
   ([request_repository.dart:416-441](app/lib/data/repositories/request_repository.dart#L416-L441))
   invalidate `my:*`, `community:<id>:*`, the feed cache, the search
   cache, and (for submit) the daily invalidation signal. The current
   service-direct path in unified-create skips every one of these. The
   updated plan picks that invalidation up "for free" by switching to
   the repository, and an explicit phase task verifies the resulting
   feed/discover refresh behavior.
3. **i18n / accessibility for error surfaces.** The Open Questions
   section mentioned showing a snackbar on `ShareRequest` failure
   without saying where the string comes from. Per
   [docs/client/i18n.md](docs/client/i18n.md), every user-visible
   string lives in `app_en.arb` + `app_es.arb` and is resolved in
   widgets via `context.l10n.*`; viewmodels expose typed state
   (`UserError? error` via `RpcErrorHandler.classify`) and never
   resolve strings. The plan now spells that out and adds the ARB
   keys to Phase 1. The plan also calls out that since this fix is
   ViewModel-only, no new accessibility primitives are needed.

The Implementation Plan, Open Questions, and Recommendation sections
below have been edited in place to reflect these corrections.

## Problem Statement

When a user creates a Request and selects multiple communities to share it
with, the app creates **one Request per community** instead of a single
Request linked to all selected communities. Gear and Experience creation
correctly produce one entity with multiple community associations; Request
should match that pattern.

The bug is reproducible in app version 0.20.1 on iOS (DEV environment) and
is filed by the project owner via the in-app feedback channel.

## Context from Comments

The issue has no comments. The description is unambiguous and consistent
with the codebase evidence (see Problem Location below), so the lack of
discussion is not a blocker.

## Problem Location

There are **two** request-creation paths in the client. One already does the
right thing; the other is the buggy one.

### Path A — buggy: unified-create flow

[app/lib/presentation/viewmodels/unified_create_save_actions.dart:109-128](app/lib/presentation/viewmodels/unified_create_save_actions.dart#L109-L128) — `_saveRequest`
loops over `state.selectedCommunityIds` and calls
`requestService.submitRequest(communityId: cid, …)` once per community.
Each call creates a brand-new `Request` row with its own conversation and
its own `CommunityRequest` junction record. This is the path the unified
create modal (the one the user hit when filing #1962) takes.

For comparison, the same file does it correctly for the other two types:
- [unified_create_save_actions.dart:65-81](app/lib/presentation/viewmodels/unified_create_save_actions.dart#L65-L81) — `_saveEvent` calls `saveExperience` once, then `shareExperience` per community.
- [unified_create_save_actions.dart:83-107](app/lib/presentation/viewmodels/unified_create_save_actions.dart#L83-L107) — `_saveGear` calls `saveGear` once, then `community.shareGear` per community.

The header comment at [unified_create_save_actions.dart:32-33](app/lib/presentation/viewmodels/unified_create_save_actions.dart#L32-L33) actually documents the
broken pattern as intentional: *"Request — `submitRequest()` takes the
communityId up-front, so we call it once per selected community and return
the first id."* That comment encodes the bug.

### Path B — already correct: gen-request flow

[app/lib/presentation/viewmodels/gen_request_view_model.dart:302-339](app/lib/presentation/viewmodels/gen_request_view_model.dart#L302-L339) — `submitRequest(List<String> communityIds)`
calls `_requestRepository.submitRequest(communityId: communityIds.first, …)`
once, then `_requestRepository.shareRequest(requestId, communityIds: rest)`
for any remaining communities. An in-code comment explicitly warns against
the per-community loop pattern that path A uses. This is the pattern path A
should adopt.

### Server side is fine

Both [SubmitRequest](server/services/request/lifecycle.go#L35) and
[ShareRequest](server/services/request/sharing.go#L67) work as designed.
`ShareRequest` already accepts `repeated string community_ids`
([proto/ripls/api/request_service.proto:325-332](proto/ripls/api/request_service.proto#L325-L332)) and creates the
extra `CommunityRequest` junction rows on a single existing Request /
conversation. No server change is required for the minimal fix.

### Schema is fine

The `Request` storage model in [proto/ripls/models/request.proto:29-98](proto/ripls/models/request.proto#L29-L98)
has no `community_id` field — community membership lives entirely in the
`CommunityRequest` junction table
([proto/ripls/models/community.proto:142-168](proto/ripls/models/community.proto#L142-L168)), exactly
matching the Gear / Experience pattern. Nothing about the schema forces
one-row-per-community; the client just happens to do that.

## Potential Solutions

### Option A: Mirror the gen-request pattern in unified-create (minimal client-only fix)

- **Approach:** Rewrite `_saveRequest` in
  [unified_create_save_actions.dart:109-128](app/lib/presentation/viewmodels/unified_create_save_actions.dart#L109-L128)
  to (a) read `requestRepositoryProvider` (not `requestServiceProvider`),
  (b) call `requestRepository.submitRequest(communityId: communityIds.first, …)`
  once, capture the returned `requestId`, and (c) when more than one
  community is selected, call
  `requestRepository.shareRequest(requestId, communityIds: rest)` once
  with the remaining IDs (`rest = selectedCommunityIds.skip(1).toList()`).
  Handle the empty-list case as a `StateError` (the unified-create flow
  already prevents save unless at least one community is selected). Remove
  the misleading header comment at lines 32-33.
- **Pros:** Smallest possible change. No proto change, no server change,
  no migration. The exact same pattern is already working in path B, so
  there is a live reference for behavior, error handling, and analytics
  emission. Going through the repository also picks up the cache
  invalidation listed in the Audit Notes above, fixing the stale-feed
  side effect at the same time. Ships in a single PR.
- **Cons:** Leaves the proto in a state where `SubmitRequest` still takes a
  singular `community_id` while `ShareRequest` takes `repeated` — slightly
  asymmetric API. Two RPCs are made instead of one when there are 2+
  communities.
- **Effort:** Small.

### Option B: Add `repeated string additional_community_ids` to SubmitRequestRequest

- **Approach:** Add an `optional repeated string additional_community_ids`
  field to
  [SubmitRequestRequest](proto/ripls/api/request_service.proto#L105-L124)
  (keeping `community_id` for the primary association). Server creates the
  Request once, then loops to insert `CommunityRequest` junction rows for
  the primary + each additional ID inside the same transaction. Client
  passes all selected community IDs in one call.
- **Pros:** Single RPC at creation time. The new field is wire-compatible
  (optional repeated, default empty). Slight perf win on multi-community
  creation. Aligns the API with how `ShareRequest` already accepts a list.
- **Cons:** Proto + server + client + tests all need to change. The
  server-side handler already does a fair amount in
  [server/services/request/lifecycle.go:35-250](server/services/request/lifecycle.go#L35-L250)
  (location validation, conversation creation, media stitching) and adding
  a junction-loop inside the transaction widens the surface area for the
  fix. More code review, more risk.
- **Effort:** Medium.

### Option C: Make `SubmitRequest` symmetrical with Gear/Experience (drop `community_id`, require a follow-up Share)

- **Approach:** Remove `community_id` from `SubmitRequestRequest` entirely;
  callers always do `SubmitRequest` (no communities) + `ShareRequest`
  (one or more communities). Server-side `SubmitRequest` no longer writes
  any `CommunityRequest` row.
- **Pros:** Most architecturally consistent with Gear and Experience.
- **Cons:** Breaking API change. Requires migrating both call sites
  (unified-create and gen-request) and any existing clients. The Request
  model conceptually requires at least one community (a Request with no
  community has no audience), which would either need to be enforced at
  the client (fragile) or relaxed at the server (creates a new "orphan
  Request" state that the rest of the system would need to handle).
- **Effort:** Large.

## Recommendation

**Option A.** The bug is fully fixable in
[unified_create_save_actions.dart:109-128](app/lib/presentation/viewmodels/unified_create_save_actions.dart#L109-L128)
by mirroring the pattern already proven in
[gen_request_view_model.dart:302-339](app/lib/presentation/viewmodels/gen_request_view_model.dart#L302-L339).
The server, proto, and storage layers are all already correctly designed
for one-Request-many-communities — only the unified-create client path
fails to use them. Options B and C add scope (and risk) without changing
the user-visible behavior beyond what Option A already gives us.

If we later want to consolidate to a single RPC for perf or API
consistency, that's a follow-up worth filing on its own; it shouldn't gate
shipping the user-visible fix.

## Open Questions

Every open question below is resolved by **deferring to how Experience
already handles the same situation**. The canonical reference is
[gen_experience_view_model.dart:680-779](app/lib/presentation/viewmodels/gen_experience_view_model.dart#L680-L779)
(`createAndShareExperience`), which is the closest peer flow and has
been in production without this class of bug. The unified-create path
for Request must match Experience's behavior 1:1 unless we have a
specific reason to diverge — and we don't.

- **Should analytics emit one `RequestCreatedEvent` or one per
  community?** —
  **Defer to Experience.** Experience emits exactly **one**
  `ExperienceCreatedEvent` after all share calls complete, keyed by
  `communityIds.first`
  ([gen_experience_view_model.dart:748-755](app/lib/presentation/viewmodels/gen_experience_view_model.dart#L748-L755)).
  Do the same: one `RequestCreatedEvent` keyed by `communityIds.first`,
  emitted after the share loop. Path B (gen-request) already does this
  at [gen_request_view_model.dart:320-327](app/lib/presentation/viewmodels/gen_request_view_model.dart#L320-L327),
  so both reference flows agree.
- **What happens if `ShareRequest` fails after `SubmitRequest`
  succeeds?** —
  **Defer to Experience.** Experience wraps the create + share-loop in
  a single `try` block. On any thrown error it stores the classified
  error on state — `state.copyWith(error: RpcErrorHandler.classify(e,
  fallback: 'Could not create the event'), errorStep: …, currentStep:
  preview)` — and rewinds the step indicator to the preview state
  ([gen_experience_view_model.dart:766-778](app/lib/presentation/viewmodels/gen_experience_view_model.dart#L766-L778)).
  There is **no rollback**: a partially-shared Experience stays in
  place and the user retries from the preview. Do the same for
  Request: typed `UserError` on state, no rollback, no manual snackbar
  string in the viewmodel. The widget reads `state.error` and
  resolves the localized message via
  `RpcErrorHandler.localize(state.error!, context.l10n)`.
  **i18n key required:** Experience uses the literal fallback string
  `'Could not create the event'` inline at
  [gen_experience_view_model.dart:774](app/lib/presentation/viewmodels/gen_experience_view_model.dart#L774).
  That's a pre-existing i18n violation we should not copy — instead,
  reference a localized fallback from `app_en.arb` /
  `app_es.arb` (e.g. `unifiedCreateRequestFailure`) and let
  `RpcErrorHandler.localize` produce the final string. (Optionally
  file a follow-up to back-fix Experience's hard-coded fallback the
  same way.)
- **Should we also delete the duplicate Requests already created by
  users in prod/dev?** —
  **Defer to Experience.** Experience does not run any backfill for
  partially-shared or duplicated rows; there is no Experience-equivalent
  janitor job. Do the same for Request: no automated cleanup. The
  affected user can manually archive any duplicate Requests from the
  Request detail screen.
- **Should the header doc-comment at
  [unified_create_save_actions.dart:23-33](app/lib/presentation/viewmodels/unified_create_save_actions.dart#L23-L33)
  be updated, or just deleted?** —
  **Defer to Experience.** The bullet currently reads
  *"Event — `saveExperience()` (returns id) then `shareExperience()`
  per community."* Rewrite the Request bullet in the same shape:
  *"Request — `submitRequest()` once for the first community (returns
  id), then `shareRequest()` for the remaining communities."* Same
  voice, same level of detail.

## Implementation Plan

### Phase 1: Fix the client path

**Goal:** Eliminate the duplicate-Request bug end-to-end for unified-create.
**Tasks:**
- [ ] Rewrite `_saveRequest` in
      [unified_create_save_actions.dart:109-128](app/lib/presentation/viewmodels/unified_create_save_actions.dart#L109-L128)
      to (a) read `requestRepositoryProvider` instead of
      `requestServiceProvider`, (b) assert `selectedCommunityIds` is
      non-empty, (c) call
      `requestRepository.submitRequest(communityId: communityIds.first, …)`
      once and capture `requestId`, (d) if `communityIds.length > 1`,
      call `requestRepository.shareRequest(requestId, communityIds: rest)`
      once with the remaining IDs.
- [ ] Update the class-level doc comment at
      [unified_create_save_actions.dart:23-33](app/lib/presentation/viewmodels/unified_create_save_actions.dart#L23-L33)
      so the Request bullet matches the new behavior. Also drop the
      stale "submitRequest takes the communityId up-front" justification.
- [ ] Emit a single `RequestCreatedEvent` (keyed by the first community ID),
      matching path B.
- [ ] Wrap the `submit` + `share` pair in `try` / `catch`; on failure
      call `RpcErrorHandler.classify(e)` and store the resulting
      `UserError` on the unified-create state (see
      [app/lib/core/errors/README.md](app/lib/core/errors/README.md)).
      The screen widget reads that typed state and resolves the user-
      visible string via
      `RpcErrorHandler.localize(state.error!, context.l10n)`. Do **not**
      put raw strings in the viewmodel.
- [ ] Add new ARB keys (`unifiedCreateShareRequestPartialFailure` and
      whatever submit-failure key the existing flow needs) to
      [app/lib/l10n/app_en.arb](app/lib/l10n/app_en.arb) and
      [app/lib/l10n/app_es.arb](app/lib/l10n/app_es.arb) with
      `@key.description` metadata. Run `npm run generate` to update
      `AppLocalizations`.
- [ ] Add a unit test under
      `app/test/presentation/viewmodels/unified_create_save_actions_test.dart`
      that uses `ProviderContainer` with
      `requestRepositoryProvider.overrideWithValue(mockRequestRepository)`
      (mock is generated via `@GenerateMocks([RequestRepository])`),
      covering the three cases: 1 community, 2 communities, 3+
      communities. Assert that `submitRequest` is called exactly once
      with the first community ID and that `shareRequest` is called
      exactly once with the remaining IDs (or not at all in the
      1-community case). Also add an error-path test that throws from
      `submitRequest` and asserts the resulting `UserError` is the
      expected variant. Pattern reference:
      [docs/client/testing.md §1 ViewModel Testing](docs/client/testing.md).
- [ ] Run `flutter test --reporter expanded` (via `npm run test:app`)
      and `npm run build` locally before opening the PR. Note that
      `--reporter expanded` is required per
      [CLAUDE.md §Client Testing](CLAUDE.md) — the default `compact`
      reporter floods the terminal when piped.
**Validates:** Confirms the unified-create modal produces exactly one
Request regardless of how many communities are selected; that the
relevant caches (`my:*`, `community:*:*`, feed, search, daily) are
invalidated as a side effect of routing through the repository; and
that the fix is covered by a regression test.

### Phase 2: Manual QA in DEV

**Goal:** Verify the fix on the same code path that produced the original
bug report (unified-create modal, iOS, DEV).
**Tasks:**
- [ ] On a DEV build, create a Request via the unified-create flow with
      1 community → verify exactly 1 Request appears in the feed, and
      that the feed/discover screens refresh without a pull-to-refresh.
      (This second check confirms the repository's `invalidateFeed()` /
      `invalidateSearchesForCommunity()` calls fired — the existing
      service-direct path skipped them.)
- [ ] Repeat with 2 communities → verify exactly 1 Request, but visible in
      both community feeds, with one shared conversation.
- [ ] Repeat with 3+ communities → same.
- [ ] Trigger a `ShareRequest` failure (e.g. by selecting a community the
      user is no longer a member of, if reproducible) and confirm the
      partial-success snackbar is localized (no fallback English text in
      Spanish locale).
**Validates:** That the user who filed #1962 will no longer see duplicate
Requests in the wild, and that the cache-invalidation side benefit is
working as expected.

### Phase 3 (optional, follow-up issue): Consolidate to a single RPC

**Goal:** Reduce the create-with-N-communities flow to a single round trip
and remove the singular-vs-repeated API asymmetry.
**Tasks:**
- [ ] File a separate follow-up issue for Option B (add
      `repeated string additional_community_ids` to
      `SubmitRequestRequest`).
- [ ] Cross-link it from this issue and from the PR that closes #1962.
**Validates:** Not in scope for #1962. Listed here only so the asymmetry
isn't forgotten.

## Related Issues

- None found in the comments. If a duplicate or precursor issue exists,
  cross-link in the PR description.
