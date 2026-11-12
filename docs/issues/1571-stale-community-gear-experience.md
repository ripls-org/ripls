# Issue #1571: Stale community context: extend #1547 fix to gear and experience flows

**Source:** <https://github.com/Type2Fun-Foundation/ripls/issues/1571>
**Filed:** 2026-04-29 | **Last activity:** 2026-04-29
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** docs/issues/1547-offer-help-stale-community.md

## Problem Statement

The Phase 2 defense-in-depth guard introduced in #1549 (`resolveCommunityId`
validating `state.communityId` against `response.sharedCommunities` before
issuing per-entity RPCs) was applied only in
`request_view_model.dart::loadRequestDetails`. The same unguarded
fallback — `state.communityId ?? communities.first.communityId` — survives
in the gear and experience load paths, so any code path that pre-seeds
`communityId` with a stale value (router params, caller-supplied
`initialCommunityId`, future deep-link surfaces) leaves those flows
exposed even after the #1549 root-cause fix. The bad communityId then
flows into every RPC the screen issues (`GetGearStats`,
`GetGearPeople`, `RSVPToExperience`, …).

The experience flow has a second, independent gap on the server.
`server/services/experience/rsvp.go::RSVPToExperience` accepts
`community_id` and persists the RSVP keyed to it without verifying the
experience is actually shared with that community. By contrast, the
parallel request flow validates `(request_id, community_id)` against
`CommunityRequest` (`server/services/request/offers.go:75-85`) and
returns `CodeNotFound` on mismatch. Without that check the RSVP is
silently recorded under the wrong community: per-community attendance
counts skew, the chat system message and community event fire in the
wrong place, and the user sees no error. This is *worse* than the
original request bug (#1547) because there is no signal — neither user
nor server flags the corruption.

The issue body also notes that loans and giveaways are bug-immune by
accident: `ExpressInterest(gear_id)` does not take a client-supplied
`community_id` (server resolves via `findMatchingCommunityAndType`),
`GetGearTransferContext` requires `community_id` only for input
validation and does not filter on it, and the lifecycle RPCs are keyed
by `transfer_id`. `GetGearStats` / `GetGearPeople` would silently
return wrong numbers under a stale community context, but fixing the
gear viewmodel guard closes this.

## Context from Comments

The only comment is `@claude` from a maintainer (maintainer)
triggering this triage run. The issue body is the maintainer's own
analysis; there's no debate to summarize. The body explicitly
proposes both fixes and confirms the loan/giveaway flows are out of
scope.

## Potential Solutions

### Option A: Extract `resolveCommunityId` to `core/utils/`, apply at gear + experience load sites, add server validation in `RSVPToExperience`

- **Approach:**
  1. Move the existing `resolveCommunityId` top-level function (and its
     `_log` warning) from
     `app/lib/presentation/viewmodels/request_view_model.dart` into a
     new file `app/lib/core/utils/community_resolver.dart`. Re-export
     it from `request_view_model.dart` so its existing test
     (`request_view_model_resolve_community_test.dart`) compiles
     unchanged via the same import.
  2. Replace the unguarded fallback in
     `gear_view_model.dart::loadGearDetails` (lines 307-314) and
     `experience_view_model.dart::loadExperienceDetails` (lines 186-191)
     with a call to `resolveCommunityId(supplied: …, shared: communities)`.
  3. In `server/services/experience/rsvp.go::RSVPToExperience`, before
     touching state, query
     `CommunityExperience` by `(experience_id, community_id)` and
     return `connect.NewError(connect.CodeNotFound, …)` when no row
     exists. Mirror the wording and structure of
     `offers.go:75-85`. Skip the check when
     `req.Msg.CommunityId == ""` (defensive — current behaviour treats
     empty community id as "no community context").
- **Pros:** One fix per gap, no semantic surprises. Helper extraction
  is mechanical and the unit test for the helper already exists. The
  server validation closes the silent-corruption hole and matches the
  existing request-flow pattern. Test coverage is straightforward:
  reuse the existing helper test file by retargeting its import (no
  new tests needed for it), add new viewmodel tests for the gear and
  experience guards, add a server unit test that drives
  `RSVPToExperience` with a non-shared community.
- **Cons:** Three behavioural change sites (two client viewmodels, one
  server RPC) plus one move. The server change adds an extra storage
  query on every RSVP — small (`QueryByFields` with two equality
  filters on an indexed table) but worth measuring.
- **Effort:** small.

### Option B: Push the guard into the repository layer (Gear/ExperienceRepository)

- **Approach:** Put `resolveCommunityId` inside
  `app/lib/data/repositories/gear_repository.dart` and
  `experience_repository.dart` so any caller that fetches the entity
  gets a corrected community context "for free" via a wrapper that
  returns `(response, effectiveCommunityId)`.
- **Pros:** Eliminates the chance that a future viewmodel forgets to
  guard.
- **Cons:** The repository layer per `caching.md` is a passive cache —
  it does not own ViewModel state. The "effective community id" is a
  ViewModel concern (it owns `state.communityId` and the warning log
  context). Changing repository return shapes is more invasive than
  the bug warrants and would not help the server-side gap. Existing
  pattern in `request_view_model.dart` is the contract the issue
  explicitly asks us to extend.
- **Effort:** medium.

### Option C: Make `community_id` optional on `RSVPToExperience` (server) like option C in #1547

- **Approach:** Allow the client to omit `community_id`. If absent,
  server picks a single eligible community (experience is shared
  there + user is a member). If multiple are eligible, return
  `FailedPrecondition` with the list so the client can prompt.
- **Pros:** Cleanest API: the user is RSVPing to an *experience*,
  not a *(experience, community)* tuple.
- **Cons:** Bigger change — proto + server + every client call site +
  tests. Doesn't fix the fact that the experience screen still
  *displays* under the wrong community, and per-community RSVP
  storage is currently a real distinction (max-participants,
  per-community attendance, RSVP visibility under
  `cross_community_test.go`). #1547 explicitly skipped C; the same
  reasoning applies here.
- **Effort:** medium.

## Recommendation

Ship **Option A**. It mirrors the #1549 Phase 2 pattern the issue
explicitly asks us to extend, and it is the smallest change that
actually closes both the client and server gaps. Option B drags
business logic into a passive caching layer; Option C is a bigger
proto/contract change that #1547 already weighed and rejected for
the same reasons.

## Open Questions

- **Where exactly should the shared helper live?** — Recommendation:
  `app/lib/core/utils/community_resolver.dart`. The function is a
  pure, top-level Dart function with no Riverpod or state
  dependencies, so it belongs alongside the other "narrow concern"
  helpers in `core/utils/`. Add a one-line entry to
  `core/utils/README.md`. Re-export from `request_view_model.dart` so
  the existing helper test keeps importing
  `request_view_model.dart` and stays passing.
- **Should the server `RSVPToExperience` validation skip when
  `community_id == ""`?** — Recommendation: yes. Matches the existing
  `RecordAttendance` behaviour where `community_id` is treated as a
  scoping filter; an empty value means "no community context" rather
  than "this empty string". Document the carve-out with a short
  comment.
- **Should we surface the auto-correction in the gear/experience
  UIs?** — Recommendation: silent, same as request flow. Emit a
  `_log.warning` that captures both the original and corrected ids so
  prod logs surface upstream miswiring. The user-facing experience is
  to silently load against the right community; a banner here would
  be confusing and cosmetic.
- **Should we add a server-side warning log on RSVP mismatch (parallel
  to Phase 3 of #1547)?** — Recommendation: yes — log a
  `mismatch=community` warning before the 404 with the supplied
  `community_id`, the experience id, and the list of communities the
  experience is actually shared with. Matches the precedent set in
  `offers.go` (which would also be the place the #1547 Phase 3 task
  lives).

## Implementation Plan

### Phase 1: Extract shared helper

**Goal:** A single source of truth for "validate `state.communityId`
against `response.sharedCommunities`."

**Tasks:**

- [ ] Create `app/lib/core/utils/community_resolver.dart` containing
      the existing `resolveCommunityId({required String? supplied,
      required Iterable<SharedCommunity> shared})` function and its
      private `_log` (`Logger('CommunityResolver')`). Verbatim copy of
      lines 39-57 of `request_view_model.dart` with the logger name
      updated.
- [ ] In `app/lib/presentation/viewmodels/request_view_model.dart`,
      delete the local `resolveCommunityId` and `_log` references for
      it (keep the file's existing `_log` for the notifier). Add an
      `export 'package:ripls/core/utils/community_resolver.dart'
      show resolveCommunityId;` so existing imports of
      `request_view_model.dart` keep compiling unchanged. (This avoids
      churn in
      `request_view_model_resolve_community_test.dart` and any other
      caller that ends up importing the helper transitively.)
- [ ] Add an entry for `community_resolver.dart` to
      `app/lib/core/utils/README.md` ("Validates that a supplied
      community id is in a response's `sharedCommunities`, falling
      back to the first shared community").
- [ ] Run `cd app && flutter test --reporter expanded
      test/presentation/viewmodels/request_view_model_resolve_community_test.dart`
      to confirm the move is behaviour-preserving.

**Validates:** The helper is importable from a non-viewmodel location
and its existing tests still pass with no source edits.

### Phase 2: Apply guard in gear and experience load paths

**Goal:** `GetGearStats`, `GetGearPeople`, `RSVPToExperience`, and any
other RPC that reads `state.communityId` after load gets a corrected id.

**Tasks:**

- [ ] In `app/lib/presentation/viewmodels/gear_view_model.dart`,
      replace the body of the `safeUpdateState` block at lines
      307-314 with:
      ```dart
      final communities = response.sharedCommunities;
      final effectiveCommunityId = resolveCommunityId(
        supplied: s.communityId,
        shared: communities,
      );
      return s.copyWith(
        gearDetails: response,
        sharedCommunities: communities,
        communityId: effectiveCommunityId,
      );
      ```
      Import the helper via
      `import 'package:ripls/core/utils/community_resolver.dart';`.
      Do **not** also call it inside `refreshGearDetails`,
      `refreshGearDetailsSilent`, or `_invalidateAndReloadStatsAndPeople`
      — they all read `state.communityId` after the load, so a single
      correction at the load site covers them (mirroring the
      single-site approach validated in #1549 Phase 2).
- [ ] In `app/lib/presentation/viewmodels/experience_view_model.dart`,
      apply the same change at lines 186-191. Same import.
- [ ] Add a viewmodel test in
      `app/test/presentation/viewmodels/gear_view_model_test.dart`
      using the existing `ProviderContainer` + Mockito harness in
      that file (per `docs/client/testing.md` §1): seed
      `state.communityId = 'wrong'`, stub `gearRepositoryProvider`
      to return a `GetGearResponse` whose `sharedCommunities` is
      `[SharedCommunity(communityId: 'right')]`, call
      `loadGearDetails()`, assert post-load
      `container.read(gearProvider(gearId)).communityId == 'right'`.
      Reuse the existing mocks generated by `@GenerateMocks` rather
      than introducing new ones.
- [ ] Add the parallel viewmodel test in
      `app/test/presentation/viewmodels/experience_view_model_test.dart`,
      using the same patterns and the existing
      `experienceRepositoryProvider` mock.
- [ ] Run `cd app && flutter test --reporter expanded
      test/presentation/viewmodels/gear_view_model_test.dart
      test/presentation/viewmodels/experience_view_model_test.dart
      test/presentation/viewmodels/request_view_model_test.dart`.

**Validates:** A pre-seeded stale `communityId` is silently corrected
on load for both entity types; no other test in the gear/experience
viewmodel suites regresses.

### Phase 3: Server-side validation in `RSVPToExperience`

**Goal:** A client that sends a `community_id` the experience isn't
shared with gets a clear `CodeNotFound`, not silent corruption.

**Tasks:**

- [ ] In `server/services/experience/rsvp.go::RSVPToExperience`, after
      the existing `s.storage.GetByID(ctx, …, expStored)` block but
      before the `MaxParticipants` check, add:
      ```go
      if req.Msg.CommunityId != "" {
          ces, err := s.storage.QueryByFields(ctx, map[string]any{
              "experience_id": req.Msg.ExperienceId,
              "community_id":  req.Msg.CommunityId,
          }, &models.CommunityExperience{})
          if err != nil {
              logger.ErrorContext(ctx, "failed to query community experience for RSVP", "error", err)
              return nil, connect.NewError(connect.CodeInternal, err)
          }
          if len(ces) == 0 {
              logger.WarnContext(ctx, "RSVP community mismatch: experience not shared with community",
                  "mismatch", "community")
              return nil, connect.NewError(connect.CodeNotFound,
                  fmt.Errorf("experience not shared with this community"))
          }
      }
      ```
      Match the pattern in `offers.go:75-85` for naming and error
      wording. Use the `*Context` log methods per
      `docs/server/observability.md` so the request_id and user_id
      already attached to `logger` propagate through Cloud Logging.
      The `WarnContext` call mirrors the Phase 3 diagnostic logging
      recommended in
      `docs/issues/1547-offer-help-stale-community.md`.
- [ ] Add a test in
      `server/services/experience/rsvp_test.go`:
      `TestService_RSVPToExperience_CommunityNotShared`. Set up an
      experience shared with community A, ensure user is a member of
      both A and B, call `RSVPToExperience` with `community_id = B`,
      assert `connect.CodeOf(err) == connect.CodeNotFound`. Verify
      no `ExperienceRSVP` row was inserted (`QueryByFields` returns
      empty).
- [ ] Run `npm run build` and
      `go test -tags=integration -timeout 20m
      ./server/services/experience/...`.

**Validates:** Stale community context on the experience RSVP path
returns a hard error instead of silently corrupting attendance data.

### Phase 4 (optional, bundle if cheap): mirror diagnostic logging in `offers.go`

**Goal:** Close out the Phase 3 follow-up that
`docs/issues/1547-offer-help-stale-community.md` left open.

**Tasks:**

- [ ] In `server/services/request/offers.go`, before the existing
      `len(communityRequests) == 0` 404 (line 84), add the same
      `logger.WarnContext(ctx, "offer community mismatch: request
      not shared with community", "mismatch", "community")` line
      that Phase 3 adds to `rsvp.go`. Pure logging change, no
      behaviour change.

**Validates:** Future occurrences of either bug shape are
diagnosable in one log query rather than multiple cross-referenced
ones. Skip this phase if it would slow down landing Phase 1-3.

## Architecture Compliance Notes

- **Layering (`docs/client/architecture.md`):** `core/utils/` is the
  prescribed home for "narrow concern, no Riverpod dependency" pure
  helpers. `resolveCommunityId` qualifies (pure function, takes a
  proto type, returns a string). It does *not* belong in the
  repository layer per `caching.md` (repositories are a passive
  cache, not a place for ViewModel-state concerns).
- **Caching (`docs/client/caching.md`):** No cache key changes, no
  new repository methods. The fix is purely a guard at the
  ViewModel/load boundary. No invalidation work.
- **i18n (`docs/client/i18n.md`):** No user-facing strings added.
  Existing `Code.notFound` localization in `rpc_error_handler.dart`
  continues to cover the residual NotFound from the new server-side
  validation. No ARB updates required.
- **Accessibility (`docs/client/accessibility.md`):** No UI changes;
  the auto-correction is silent and prevents a hard error from
  reaching the user. No new semantic labels required.
- **Testing (`docs/client/testing.md`):** ViewModel-first tests via
  `ProviderContainer`, with repositories mocked using existing
  `@GenerateMocks` annotations. Server-side test follows the
  testcontainers + `setupTestService(t)` pattern already established
  in `rsvp_test.go`.
- **Server observability (`docs/server/observability.md`):** New log
  lines use `logger.WarnContext(ctx, …)` / `logger.ErrorContext(ctx, …)`
  so the request_id and user_id attached via `LoggerWithContext`
  propagate. Field naming (`mismatch`, `community_id`,
  `experience_id`) matches the standard schema. The mismatch warning
  is `WARN` (handled user-input issue), not `ERROR`, per the
  level-selection guidance.

## Related Issues

- #1547 — Original "can't offer help" bug; this issue extends its
  Phase 2 fix.
- #1549 — PR that landed the #1547 fix and introduced the
  `resolveCommunityId` helper this plan extracts.
