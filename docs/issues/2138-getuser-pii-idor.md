# Issue #2138: Security: user.GetUser discloses every user's email + home-location ID to any authenticated caller

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2138
**Filed:** 2026-05-23 | **Last activity:** 2026-05-23
**Labels:** bug, security, server, privacy | **Assignees:** unassigned
**Context docs:** none (related: #2136 IDOR class, #2137 GetNearbyUsers — fixed & merged in PR #2142)

## Problem Statement

`user.GetUser` (`server/services/user/service.go:36-69`) is an IDOR. It accepts
any `user_id`, gates only on `auth.RequireAuth` (any valid token), and returns
the full storage profile of the target user — including **`Email`** (real email,
the password-reset / account-recovery vector), **`PrimaryResidenceLocationId`**,
and **`OtherLocationIds`** (home-location IDs that chain to `location.GetLocation`
for an address and previously chained to `GetNearbyUsers` for exact lat/lng).

There is no caller-vs-target check and no field-level scoping. User IDs are not
secret in practice — they leak through `gear.GetGear` (`Owner`),
`experience.GetExperience` (`OwnerId`), chat participants, community member
lists, etc. So any authenticated caller can map an enumerable `user_id` to a
`(name, email, home location)` profile for every user. Email enumeration via
user-id IDOR is a standard high-severity finding; the home-location pairing
makes it worse (doxxing / stalking / spear-phishing).

The core issue is **over-disclosure of two PII field groups (email, location
IDs) to non-self callers** — not the existence of the RPC itself.

### Where this plan diverges from the issue's recommended fix

The issue recommends the `GetUserProfileForViewer` "viewer lens": require at
least one **shared active community** for any cross-user fetch, returning
`PermissionDenied` otherwise, and drop `Email` even when permitted.

Codebase research shows the shared-community **gate** is too strict for how
`GetUser` is actually used and would break existing UX (see Context /
Recommendation). The **field-trimming** half of the recommendation is exactly
right and is what closes the high-severity leak. This plan adopts field
trimming as the fix and treats the shared-community gate as a rejected
alternative, with rationale below.

## Context from Comments

The issue has one comment — the author's own `@claude plan` trigger. No
maintainer discussion, no disagreement, no prior art beyond the cross-references
already in the body. That is a signal that the *fix shape* is unreviewed: the
recommended-fix block in the issue is a first-draft suggestion, not a settled
decision. This plan therefore re-derives the fix from the codebase rather than
implementing the issue's suggested signature verbatim.

Relevant external context (not in comments, but load-bearing):

- **#2137 is fixed and merged** (PR #2142). The exact-coordinate chain from
  `PrimaryResidenceLocationId` → `GetNearbyUsers` is already quantized to ~110 m
  and community-scoped. The location-ID leak in #2138 still chains to
  `location.GetLocation` for an address, so it remains live and worth closing.
- The codebase already has the canonical cross-user identity RPC:
  `profile.GetUserProfileForViewer` (`server/services/profile/profile.go:32`),
  which uses `resolveSharedCommunities` and never returns email.

## Codebase research findings

1. **`GetUser` handler** (`server/services/user/service.go:36-69`): confirmed
   exactly as the issue quotes. After `RequireAuth`, it copies every storage
   field — `Email`, `Name`, `PrimaryResidenceLocationId`, `OtherLocationIds`,
   `MediaId`, `Description`, `MediaIds`, `PreferredTimezone` — into the
   response with no caller-vs-target logic.

2. **`api.GetUserResponse`** (`proto/ripls/api/user_service.proto:15-31`)
   carries all of those fields. No proto change is needed for field trimming —
   we simply leave the PII fields empty for non-self callers. (Trimming is a
   pure server-side response-shaping change; the wire contract is unchanged.)

3. **The "Related" sibling-leak concern is already safe.** `services.ToAPIUser`
   (`server/services/user_util.go:27-37`) sets only `Id`, `Name`, `MediaId`.
   `api.User` (`proto/ripls/api/user.proto:11`) **has no `email` field at all**
   and no location fields. So `FetchAPIUser` / `FetchAPIUsersBatch` — the
   denormalized user loader used by `feed.GetFeed`, `chat.GetConversation`, gear
   owner display, etc. — cannot leak email or location. Only `user.GetUser`'s
   own response carries them. No action needed there beyond a guard test.

4. **`GetUser` is heavily used cross-user by the client**, and those callers
   read only display fields:
   - `getUserProfile(userId)` for **gear owners** (`gear_view_model.dart:319`),
     **experience owners** (`experience_view_model.dart:304`), **chat
     participants** (`conversation_view_model.dart:206`), **other users'
     profiles** (`user_profile_view_model.dart:63`), **requests**
     (`request_view_model.dart:203`) — reads `mediaIds` (avatar).
   - `userRepository.get(id)` for **discover results** (`discover_view_model.dart:190`),
     **profile metrics** (`profile_metrics_view_model.dart:54`), **gear owner
     lists** (`profile_gear_view_model.dart:551`), **position resolution**
     (`user_position_resolver.dart:81`).
   None of these share-community-gate today, and **name + avatar are already
   freely readable for any user via `ToAPIUser`** (feed/chat/gear owner). A
   `PermissionDenied`-on-no-shared-community gate would break gear/experience/
   discover/chat owner display for users outside your communities — and would
   be inconsistent with the rest of the app, which already exposes name+avatar
   broadly.

5. **The PII fields are only ever read self-scoped on the client:**
   - `Email`: `feedback_screen.dart:55` (`authState.user!.id`, prefill) — self.
   - `PrimaryResidenceLocationId` / `OtherLocationIds`:
     `profile_locations_view_model.dart:67` (`authState.user?.id`) and
     `user_service.getUserSavedLocations` → only caller is
     `location_autocomplete_field.dart:66` for the editing user's own saved
     locations — self.
   So **trimming email + location IDs from non-self responses breaks no client
   flow.**

6. **Shared-community helper is `profile`-package-private.**
   `resolveSharedCommunities` (`server/services/profile/profile.go:199`) returns
   profile-specific tuples and isn't exported. The reusable membership
   primitives live in `server/auth/` (`IsMemberOfCommunity`,
   `RequireMemberOfActiveCommunity`). The `user` service should not import the
   `profile` service. (Only relevant if we were to add a gate — which we're
   not.)

## Potential Solutions

### Option A — Field trimming: self gets everything, non-self gets display-only (RECOMMENDED)

- **Approach:** In `GetUser`, after `RequireAuth` and loading the target,
  compute `isSelf := authInfo.UserID == target.Id`. Always return the display
  fields (`UserId`, `Name`, `MediaId`, `MediaIds`, `Description`,
  `PreferredTimezone`). Add `Email`, `PrimaryResidenceLocationId`, and
  `OtherLocationIds` **only when `isSelf`**. No proto change.
- **Pros:** Closes the entire high-severity leak (email + home-location IDs)
  with no risk to existing cross-user display UX. **Zero new queries** — we
  already fetch the target; `authInfo.UserID` is already in context.
  Consistent with what `ToAPIUser` already exposes app-wide. Minimal, surgical
  diff. Easy to regression-test.
- **Cons:** Leaves name+avatar+description readable by any authenticated caller
  for any `user_id`. That is the existing app-wide posture (`ToAPIUser`), and
  #2138's severity is specifically about email+location, not name. If the team
  later wants to also gate *display* identity, that's a separate, broader
  decision (#2136 class) that must move `ToAPIUser` too — out of scope here.
- **Effort:** Small.

### Option B — Viewer-lens shared-community gate (the issue's literal recommendation)

- **Approach:** Non-self fetch requires ≥1 shared active community
  (`PermissionDenied` otherwise) **and** drops email/location. Requires a new
  exported `auth.SharesActiveCommunity(ctx, storage, a, b)` helper (two
  `QueryByField[*CommunityUser]` calls + intersection, mirroring
  `resolveSharedCommunities`).
- **Pros:** Matches the issue text and the `GetUserProfileForViewer` precedent.
  Tightest restriction on who can map `user_id` → identity.
- **Cons:** **Breaks existing cross-user display UX.** `getUserProfile` is
  called for gear owners, experience owners, chat participants, discover
  results, and profile views — all of which can involve users you don't share a
  community with, and all of which would start returning `PermissionDenied`.
  Adds 2 membership queries per `GetUser` call. Inconsistent with `ToAPIUser`,
  which already exposes name+avatar with no gate, so the gate would be trivially
  bypassable for name/avatar via feed/chat anyway — paying a UX + perf cost for
  no real confidentiality gain on the display fields.
- **Effort:** Medium (helper + gate + reworking client cross-user callers onto
  `GetUserProfileForViewer`, or accepting broken screens).

### Option C — Make `GetUser` self-only; route all cross-user reads through `GetUserProfileForViewer` / `FetchAPIUser`

- **Approach:** `GetUser` returns `PermissionDenied` for any non-self
  `user_id`. Migrate every cross-user client caller to
  `GetUserProfileForViewer` (display) or rely on already-denormalized
  `api.User` data.
- **Pros:** Cleanest long-term contract — one self RPC, one cross-user RPC.
- **Cons:** Largest blast radius: ~10 client call sites to migrate and re-test,
  plus the cache layer (`user_repository`) keys on `GetUserResponse`. High
  regression risk for a security fix that wants to ship fast.
- **Effort:** Large.

## Recommendation

**Option A (field trimming).** It fully closes the high-severity finding (email
+ home-location-ID disclosure to non-self callers) with a small, zero-query,
zero-proto-change diff and no UX regression. The codebase evidence is decisive:
(1) the PII fields are only read self-scoped on the client, so trimming them
cross-user breaks nothing; (2) name/avatar are already exposed app-wide via
`ToAPIUser`, so the issue's shared-community gate would impose a UX/perf cost
without a matching confidentiality gain and would break gear/experience/
discover/chat owner display.

This honors the issue's core intent ("do not return `Email` cross-user; treat
`PrimaryResidenceLocationId` as PII") while rejecting the `PermissionDenied`
gate that the issue's first-draft signature included. If the team later decides
display identity itself must be community-gated, that is the broader #2136
work and should move `ToAPIUser` in lockstep — file it separately rather than
making `GetUser` an inconsistent outlier.

The follow-up toward Option C's clean contract (deprecate cross-user `GetUser`
in favor of `GetUserProfileForViewer`) is worth tracking but is not a security
prerequisite once Option A lands.

## Open Questions

- **Should `Description` and `PreferredTimezone` be returned cross-user?** —
  Recommendation: **yes for `Description`** (it's a user-authored public bio,
  and `GetUserProfileForViewer` already surfaces bios cross-user), **yes for
  `PreferredTimezone`** (low sensitivity; used to render event times correctly
  and harmless). Keep both in the display set. If the team disagrees on
  timezone, dropping it cross-user is a one-line change with no known client
  impact.
- **`PermissionDenied` vs. trimmed-200 for the email/location fields?** —
  Recommendation: **trimmed-200** (return the display fields, omit the PII).
  Returning an error for non-self fetch would break the documented cross-user
  display callers; a trimmed success response is what they already need.
- **Do we want a follow-up to deprecate cross-user `GetUser`?** —
  Recommendation: **yes, file a separate tracking issue** (Option C) so the
  long-term single-cross-user-RPC contract is captured, but do not block #2138
  on it.
- **Server-side internal callers of the `GetUser` RPC?** — Recommendation:
  none found; server code denormalizes via `FetchAPIUser`, not the RPC. Confirm
  with a quick grep during implementation; if any exist and need email, they
  should call storage directly, not the public RPC.

## Implementation Plan

### Phase 1 — Trim PII fields from non-self GetUser responses
**Goal:** Close the email + home-location-ID disclosure with no UX regression.
**Tasks:**
- [ ] In `server/services/user/service.go` `GetUser`: capture
      `authInfo, err := auth.RequireAuth(ctx)` (currently discarded as `_`).
- [ ] Build the response with display-only fields first (`UserId`, `Name`,
      `MediaId` via `services.PrimaryAvatarMediaID`, `MediaIds`, `Description`,
      `PreferredTimezone`).
- [ ] Add `Email`, `PrimaryResidenceLocationId`, `OtherLocationIds` only when
      `authInfo.UserID == user.Id`.
- [ ] **Logging (observability conventions):** attach a context logger via
      `logging.LoggerWithContext(ctx).With("operation", "GetUser", ...)`. Log
      the requested id under **`target_user_id`** — NOT `user_id`, which
      `LoggerWithContext` already injects as the authenticated caller, and a
      collision would silently overwrite one with the other (see the request-id
      glossary in `docs/server/observability.md`). Add a `self_view` bool field
      (mirrors `GetUserProfileForViewer`). Emit at **DEBUG** (this is a read
      path; user-not-found is INFO/WARN, never ERROR). **Never log the target
      email** — if it's ever logged for debugging, route through
      `logging.MaskEmail`.
- [ ] Keep the empty-`user_id` `InvalidArgument` and not-found `NotFound`
      behavior unchanged. **No new `connect.CodeInternal` path is introduced**
      (field trimming adds no storage calls), so the `connecterr.Internal`
      gate is not triggered. (Only relevant if the rejected Option B were taken:
      its membership-query error MUST use `connecterr.Internal(ctx, op, err,
      ...)`, never bare `connect.NewError(connect.CodeInternal, err)`, which CI
      blocks via `scripts/check_codeinternal.js`.)
**Validates:** The high-severity leak is closed at the only RPC that carried it.

### Phase 2 — Regression tests
**Goal:** Lock the behavior so a future refactor can't reopen the leak.

> **Critical — existing tests currently assert the leak.** In
> `server/services/user/service_test.go`, `TestService_GetUser` authenticates
> as `callerUser` (`authCtx`) but every subtest fetches a *different* `testUser`
> and asserts the PII fields ARE returned:
> - `"successful get user"` asserts `userResp.Email == testUser.Email` (line ~103).
> - `"get user with locations"` asserts `PrimaryResidenceLocationId` /
>   `OtherLocationIds` are returned (lines ~155-168).
> - `"get admin user"` asserts `Email == adminUser.Email` (line ~268).
>
> These cross-user assertions encode the exact vulnerability and **will fail
> under the fix.** They must be rewritten, not just appended to. Also audit
> `TestService_GetUser_MultipleUsers`, `TestService_GetUser_Integration`
> (lines ~363-443), and `roundtrip_test.go:57` — any GetUser call where
> `authCtx`'s user ≠ the requested id and which asserts email/location must be
> updated. Convert "I want to assert PII is present" subtests to **self-fetch**
> (authenticate as the user being fetched).

**Tasks:**
- [ ] Rewrite the cross-user subtests above: either make them self-fetch (when
      asserting PII presence) or flip their assertions to expect empty
      `Email` / `PrimaryResidenceLocationId` / `OtherLocationIds`.
- [ ] Add explicit new subtests to `TestService_GetUser`:
  - [ ] **self-fetch returns PII**: authenticate as `testUser` and fetch own id;
        expect non-empty `Email` and the location fields (when set).
  - [ ] **non-self fetch omits PII**: `callerUser` fetches `testUser`; expect
        `Email == ""`, `PrimaryResidenceLocationId == ""`,
        `len(OtherLocationIds) == 0`, **and** correct `Name` + avatar (proves
        display still works and we didn't just blanket-deny).
  - [ ] keep the existing unauthenticated → `Unauthenticated` case.
- [ ] Prefer unit-level tests with `storage.SetupTestStorage(t)` (the existing
      `setupTestService` pattern) — server arch guidance favors local unit tests
      over integration tests for authorization logic; no new integration test is
      needed.
- [ ] Add a guard assertion that `api.User` exposes no email — a small test in
      `user_util_test.go` documenting that `ToAPIUser` sets only id/name/media_id
      (cheap insurance against a future field add re-introducing the denormalized
      leak through `FetchAPIUser` / `FetchAPIUsersBatch`).
- [ ] Run `npm run build` (golangci-lint + `go build ./...`) and
      `go test ./server/services/user/... ./server/services/...`. Run
      `go build -tags=integration ./...` only if a signature visible to
      integration tests changed (it shouldn't).
**Validates:** Self path intact, cross-user path PII-free, leak-asserting tests
removed, no build/lint break.

### Phase 3 — Follow-up tracking (non-blocking)
**Goal:** Capture the clean long-term contract without blocking the fix.
**Tasks:**
- [ ] File a follow-up issue: "Deprecate cross-user `user.GetUser` in favor of
      `profile.GetUserProfileForViewer`; make `GetUser` self-only" (Option C),
      referencing #2138 and #2136.
- [ ] Reference that issue number in the #2138 PR description as the deferred
      hardening step.
**Validates:** Reviewers see the residual (name/avatar broadly readable) is a
known, separately-tracked decision, not an oversight.

## Related Issues
- #2136 — broader IDOR class; #2138 is its highest-impact instance because of
  the email field. The "gate display identity too" question belongs to #2136,
  not here.
- #2137 — `GetNearbyUsers` exact-coordinate leak; **fixed & merged (PR #2142)**.
  Quantization there already broke the location-ID → exact-lat/lng chain; this
  issue closes the location-ID → address chain via `location.GetLocation`.
- (to be filed) — deprecate cross-user `GetUser` in favor of
  `GetUserProfileForViewer` (Option C follow-up).

## Audit Notes (architecture compliance)

Reviewed against `docs/server/architecture.md`, `docs/server/observability.md`,
and the client docs (`architecture`, `caching`, `i18n`, `accessibility`,
`testing`). Changes folded into the plan above; rationale here.

**Server architecture — compliant.**
- *Service independence:* the fix stays inside `user` and does not import the
  `profile` service. `resolveSharedCommunities` is `profile`-private and stays
  there; we deliberately don't reach across services. (The rejected Option B's
  helper would have correctly gone in the shared `server/auth` library, not a
  service.)
- *Authorization pattern:* a `caller == target` check is the inline-auth idiom
  the arch guide shows for `UpdateGear`-style ownership; no new helper needed.
- *Error wrapping:* added an explicit note that field trimming introduces **no**
  new `connect.CodeInternal` path, so the `connecterr.Internal` gate isn't
  engaged — and flagged the `connecterr.Internal` requirement on the rejected
  Option B path so a reviewer who revives it doesn't trip CI.

**Observability — fixed in Phase 1.** Original draft said "add a `self_view`
log field" without naming conventions. Corrected to: log the requested id as
**`target_user_id`** (not `user_id`, which collides with the caller id injected
by `LoggerWithContext`), emit at **DEBUG** (read path), and never log the target
email unmasked. This matches `GetUserProfileForViewer`'s logging shape and the
request-id glossary.

**Testing — materially strengthened in Phase 2.** The audit surfaced that the
existing `TestService_GetUser` subtests authenticate as one user and fetch a
*different* user while asserting the PII fields are present — i.e. they actively
encode the vulnerability and will fail under the fix. The plan now calls out
rewriting them (self-fetch for PII-presence assertions; flipped assertions for
cross-user) rather than only adding new cases. Unit-level tests with
`SetupTestStorage` are preferred over a new integration test, per the arch
guide's "unit tests are always preferable" principle.

**Client docs — N/A (server-only, wire-compatible).** No proto change, no ARB
strings, no widgets, no accessibility labels, no new RPC — so i18n / a11y /
client-testing conventions don't apply. One **caching** point checked and
cleared: `user_repository.get(userId)` caches `GetUserResponse` keyed on the
*target* `userId` (`app/lib/data/repositories/user_repository.dart:31`). The
client-side cache is per-device, so the only "self" entry is ever the device
user's own id (which legitimately carries full fields); cross-user entries never
carry PII. The key does not need a caller dimension and there is no
cross-user cache-poisoning risk. No client cache-invalidation change required.
