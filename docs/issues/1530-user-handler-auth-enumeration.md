# Issue #1530: [security] FindUserByEmail is a user-enumeration oracle; GetUser lacks per-handler auth

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1530
**Filed:** 2026-04-28 | **Last activity:** 2026-04-29
**Labels:** auto-filed, review:security | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Two related authorization weaknesses in the `UserService` RPC handlers
(`server/services/user/service.go`):

1. **User-enumeration oracle.** `FindUserByEmail` (lines 184-211) takes any
   email and returns either a user ID or `CodeNotFound`. Any authenticated
   caller can therefore enumerate which emails are registered. There is no
   relationship check (own account, shared community, or otherwise), no rate
   limit, and the success/failure code is distinguishable.

2. **Per-handler auth gap.** `GetUser` (lines 33-63) and `FindUserByEmail`
   rely on the service-wide `authMiddleware.Wrap` mounted in `server/main.go`
   (`mux.Handle(userPath, authMiddleware.Wrap(userHandler))`). Sibling
   handlers `SaveUser` and `DeleteUser` *also* call `auth.RequireAuth(ctx)`
   (or `auth.GetAuthInfo` + manual unauth check) inside the handler.
   This is defense-in-depth and a consistency requirement; the gap means a
   future change that mounts the handler without `authMiddleware`, or a test
   that constructs a context directly, would silently bypass auth on
   `GetUser`/`FindUserByEmail`.

A useful additional finding from the codebase: **`FindUserByEmail` has no
production callers.** `app/lib/services/user_service.dart` exposes a
`findUserByEmail` wrapper, but no Flutter screen, viewmodel, repository, or
test other than auto-generated mocks invokes it. That is not the case for
`GetUser`, which is called heavily.

## Context from Comments

- **Triage (2026-04-28, maintainer):** classified as Bug / P1. Rationale:
  "security High: user-enumeration oracle plus per-handler auth gap.
  Trust-circle infrastructure (priorities #4) requires this tight."
- **2026-04-29:** maintainer pinged `@claude` to drive the fix.

No technical clarifications, no decision on whether to keep, restrict, or
remove `FindUserByEmail`.

## Potential Solutions

### Option A: Remove `FindUserByEmail` entirely

- **Approach:** Drop the RPC from `proto/ripls/api/user_service.proto`,
  delete the server handler and tests, regenerate Go and Dart code, remove
  the `findUserByEmail` wrapper from `app/lib/services/user_service.dart`,
  and add `auth.RequireAuth(ctx)` to `GetUser` for the auth-gap fix.
- **Pros:** Eliminates the oracle by construction. No production caller is
  affected. Smallest ongoing maintenance surface. Matches the codebase
  pattern of "lookup users by ID; discover other members via
  `ListCommunityUsers` / `SearchCommunityUsers`."
- **Cons:** Forecloses on a future "invite user by email" feature. If such a
  feature is in flight (e.g., a community invitation flow), this would have
  to be rebuilt later — but it would be rebuilt with the right scoping
  baked in, not retrofitted onto a wide-open RPC.
- **Effort:** Small. ~150-line deletion plus regeneration. Tests in
  `service_test.go` lines 370-526 are deleted, not rewritten.

### Option B: Restrict `FindUserByEmail` to the caller's own email

- **Approach:** Inside the handler, call `auth.RequireAuth(ctx)`, then
  compare `req.Msg.Email` to `authInfo.Email` (case-insensitive) and return
  `CodePermissionDenied` for any other email — without distinguishing
  registered vs. unregistered. Add `RequireAuth` to `GetUser` as well.
- **Pros:** Removes the oracle. Mechanical change; no schema or proto edit.
- **Cons:** The RPC becomes nearly useless: a caller already knows their
  own user ID via the auth token (`authInfo.UserID`), so a lookup that only
  returns the caller's own ID is redundant. Keeps a dead RPC alive.
- **Effort:** Small.

### Option C: Gate `FindUserByEmail` on shared-community membership

- **Approach:** Inside the handler, call `auth.RequireAuth(ctx)`, look up
  the candidate user by email, then verify the caller and the candidate
  share at least one community via `models.CommunityUser` membership rows.
  Return an *identical* error shape (same `connect.Code`, same message) for
  all three negative cases — no such email, email belongs to a user with
  no shared community, and email belongs to the caller — so the response
  cannot distinguish "registered but not visible to you" from "not
  registered." Use `connect.CodeNotFound` with a generic "user not found"
  message for all three. Add `RequireAuth` to `GetUser`.
- **Pros:** Preserves a "find a friend who's already on Ripls" capability
  while eliminating the oracle (you can only enumerate emails of users you
  already share a community with — i.e., users whose membership you can
  already list via `ListCommunityUsers`). Aligns with the trust-circle
  priority called out in triage.
- **Cons:** Adds a query (memberships of caller intersected with
  memberships of candidate), so it's slower than B. Requires careful
  attention to side-channels: the response time of the "found but not
  shared" branch must match the "not found" branch closely enough that a
  caller can't time-distinguish them. Adds code that has to be maintained
  for an RPC with no current production callers.
- **Effort:** Medium.

## Recommendation

**Option A — remove `FindUserByEmail`.**

Rationale:

- No production code calls it (verified by grepping `app/lib/` and
  `app/test/` for non-mock uses — only `app/lib/services/user_service.dart`
  defines the wrapper).
- Removing dead surface is the safest fix: no oracle by construction, no
  side-channel risk, no maintenance burden.
- If invite-by-email comes back as a real product requirement, it should
  come with its own scoping decision (community-scoped invite token,
  email-blind hash lookup, signed magic link, etc.) — not get retrofitted
  onto a generic lookup RPC.

`GetUser` keeps its current shape but gains an explicit `auth.RequireAuth`
call for defense-in-depth and consistency with `SaveUser`/`DeleteUser`.

If a maintainer overrides this to Option C (because there is an in-flight
invite-by-email feature this plan doesn't know about), the plan section in
"Implementation Plan, Phase 1B (Option C variant)" describes the concrete
work for that path.

## Open Questions

- **Is there an in-flight feature that needs `FindUserByEmail`?** —
  Recommendation: assume no based on the absence of callers and ship
  Option A. If maintainers know of one (e.g., an invite-by-email flow on
  a feature branch), they should redirect to Option C in PR review and the
  plan's Phase 1B describes the concrete work.
- **Should `GetUser` also enforce a relationship constraint** (e.g.,
  caller and target share a community), or is "any authenticated user can
  fetch any user's profile by ID" the intended trust model? — Recommendation:
  out of scope for this issue. `GetUser` returning email/name to any
  authenticated caller is a separate trust-model question that the security
  review didn't flag for this issue. Tracked as a follow-up rather than
  expanded into this PR; we keep the scope to "remove the email-oracle
  RPC and add `RequireAuth` to the two handlers that lack it."
- **Wire compatibility on Option A.** — Recommendation: this is a
  breaking gRPC/Connect change for any out-of-tree client. The Flutter
  app (the only known client) does not call the RPC, so the in-tree blast
  radius is zero. We ship the removal in the same commit as the proto
  change; clients pinned to an older version simply lose access to a
  method that was already a security defect.

## Implementation Plan

### Phase 1: Add `auth.RequireAuth` to `GetUser`

**Goal:** Close the per-handler auth gap on `GetUser` independently of the
`FindUserByEmail` outcome. Even if Option A is rejected, this part stands.

**Tasks:**
- [ ] In `server/services/user/service.go`, at the top of `GetUser`
      (currently line 38 right after the `req.Msg.UserId == ""` check),
      add:
      ```go
      if _, err := auth.RequireAuth(ctx); err != nil {
          return nil, err
      }
      ```
      Match the pattern used by `DeleteUser` (lines 219-222).
- [ ] In `server/services/user/service_test.go`, update the existing
      `TestService_GetUser` subtests to use
      `createAuthenticatedContext(...)` (defined at line 1511) instead of
      the bare `context.Background()`. Add one new subtest
      `unauthenticated_request_returns_unauthenticated` that calls
      `service.GetUser(context.Background(), ...)` and asserts
      `connect.CodeUnauthenticated`.
- [ ] Verify other tests that build `*Service` directly (no HTTP
      middleware) aren't broken by the new check. `TestService_GetUser_*`
      and `TestService_GetUser_MultipleUsers` will need authenticated
      contexts.

**Validates:** `GetUser` rejects unauthenticated callers from the handler
itself, not just from the mounted middleware. Test suite passes.

### Phase 1B (Option C variant — only if maintainer redirects): Restrict `FindUserByEmail` to shared-community members

**Skip this phase if proceeding with Option A.** Documented here so an
implementing agent has a concrete contract if maintainers reject removal.

**Goal:** Remove the user-enumeration oracle while preserving
"find a known user" within trust circles.

**Tasks:**
- [ ] In `server/services/user/service.go` `FindUserByEmail`:
      - Replace `if req.Msg.Email == "" { ... }` ordering so it stays the
        first check.
      - After the empty-email check, call `authInfo, err := auth.RequireAuth(ctx)`.
      - Look up the candidate via `s.userManager.GetUserByEmail`. Treat
        all three negative branches identically (return
        `connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))`):
        a) candidate not found,
        b) candidate is the caller themselves (no useful answer),
        c) candidate exists but caller and candidate share no community.
      - For (c), implement a single-query intersection. Add a helper to
        `server/storage` (or `server/services/community`) of the form
        `UsersShareCommunity(ctx, userIDA, userIDB string) (bool, error)`
        backed by:
        ```sql
        SELECT 1 FROM community_user a
        JOIN community_user b ON a.community_id = b.community_id
        WHERE a.user_id = $1 AND b.user_id = $2
        LIMIT 1;
        ```
        Index `community_user(user_id, community_id)` should already
        cover this; verify with `EXPLAIN`.
      - On success, return only `UserId`.
- [ ] On the timing side-channel: the "found but not shared" branch
      issues two queries (lookup + membership join); the "not found"
      branch issues one. To keep these closer in time, run the membership
      check unconditionally — even when the candidate doesn't exist —
      using the empty string for the candidate ID (which will simply
      return zero rows). Document this rationale with a one-line comment
      explaining the invariant ("query both branches to narrow timing
      side-channel").
- [ ] Update `server/services/user/service_test.go`
      `TestService_FindUserByEmail` subtests to use authenticated
      contexts. Add subtests:
      - `not_found_and_not_visible_return_same_error` — assert that an
        unregistered email and a registered-but-unshared email produce
        identical `connect.Code` and message.
      - `caller_email_returns_not_found` — caller looking up themselves
        gets the same `NotFound` shape.
      - `shared_community_member_returns_user_id` — happy path.

**Validates:** The oracle is collapsed; only emails of users you can
already enumerate via `ListCommunityUsers` are visible.

### Phase 2 (Option A): Remove `FindUserByEmail`

**Goal:** Eliminate the oracle by removing the RPC. Skip this phase if
Phase 1B was taken.

**Tasks:**
- [ ] Delete the `FindUserByEmailRequest`, `FindUserByEmailResponse`
      messages and the `rpc FindUserByEmail(...)` line from
      `proto/ripls/api/user_service.proto` (lines 32-40 and 143).
- [ ] Run `npm run generate` to regenerate Go (`server/gen/...`) and
      Dart (`app/lib/data/gen/...`) bindings. Confirm the generated
      files no longer reference `FindUserByEmail*`.
- [ ] Delete the `FindUserByEmail` handler from
      `server/services/user/service.go` (lines 184-211).
- [ ] Delete `TestService_FindUserByEmail` and
      `TestService_FindUserByEmail_MultipleUsers` from
      `server/services/user/service_test.go` (lines 370-526).
- [ ] Delete the `findUserByEmail` method from
      `app/lib/services/user_service.dart` (lines 69-94).
- [ ] Regenerate Mockito mocks: `cd app && flutter pub run
      build_runner build --delete-conflicting-outputs`. The `.mocks.dart`
      files under `app/test/...` will lose their `findUserByEmail` stubs
      automatically.
- [ ] Run `npm run build` (Go lint + build), `go test
      -tags=integration -timeout 20m ./...`, and `cd app && flutter
      test --reporter expanded` until green.

**Validates:** No reference to `FindUserByEmail` remains in source or
generated code. Server and Flutter test suites pass.

### Phase 3: Verification

**Goal:** Confirm both fixes hold together and don't regress neighbors.

**Tasks:**
- [ ] `grep -r "FindUserByEmail\|findUserByEmail" .` returns only matches
      under `proto/` history, no live source — for Option A. (For
      Option C: matches in `service.go`/`service_test.go` only.)
- [ ] Run `npm run build` and `go build -tags=integration ./...` (per
      CLAUDE.md guidance for integration-tagged builds).
- [ ] Manually verify `GetUser` with no auth context returns
      `CodeUnauthenticated` from the handler (not just the middleware) by
      running the new unauth subtest in isolation.

**Validates:** End-to-end fix is consistent and CI-green.

## Related Issues

- None directly linked. The triage doc
  (`docs/issues/triage-2026-04-28.md`) groups this with other
  trust-circle-priority security items.

## Audit notes

Audited against `docs/server/architecture.md`:

- ✓ Per-handler `auth.RequireAuth(ctx)` matches the inline-authorization
  pattern documented in §"Authorization Patterns / Option 1." `SaveUser`
  (lines 76-86) and `DeleteUser` (lines 219-222) already follow this
  pattern; this brings `GetUser` (and Option C's `FindUserByEmail`) into
  line with sibling handlers.
- ✓ The shared-community check in Phase 1B reuses
  `auth.IsMemberOfCommunity` / `auth.RequireUserIsCommunityMember` from
  `server/auth/community.go` rather than introducing a service-to-service
  dependency.
- ⚠ The `community_user(user_id, community_id)` index referenced in
  Phase 1B is **assumed**, not confirmed.
  `server/storage/protosql_schema.go` registers the `community_user`
  table at line 261 but does not declare a composite index on those
  columns. If Option C is taken, the implementing agent should run
  `EXPLAIN` on the proposed query and add a `CREATE INDEX IF NOT EXISTS`
  to `protosql_schema.go` if the planner does a sequential scan. This
  is a follow-up step inside Phase 1B, not a blocker for the plan.
- ✓ Tests stay at the unit level using `createAuthenticatedContext`
  (defined at `server/services/user/service_test.go:1511`), consistent
  with §"Unit Tests are always preferable to integration tests."
- ✓ No changes required in Flutter caching or i18n layers — Option A
  removes a never-cached RPC; Option C tightens server-side authorization
  without changing the response type.
