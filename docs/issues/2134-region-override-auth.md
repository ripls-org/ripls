# Issue #2134: Security: SetCommunityRegionOverride retains write access after creator leaves community

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2134
**Filed:** 2026-05-23 | **Last activity:** 2026-05-23
**Labels:** bug, security, server | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`community.SetCommunityRegionOverride` (`server/services/community/regions.go:67-137`) authorizes writes by comparing `community.CreatorId` to the caller's user ID. `CreatorId` is an **immutable** "first member" marker — it is set once during `CreateCommunity` (`server/services/community/lifecycle.go:43`) and is never updated by ownership handoff in `LeaveCommunity`. The handler does **not** verify the caller is currently a member, nor that they are the current `OwnerUserId`.

Consequence: a user who created a community, transferred ownership via `LeaveCommunity`'s owner-with-handoff branch, and then left the community retains the ability to mutate the community's region override indefinitely (for the lifetime of their access/refresh token chain — up to ~90 days per `main.go:707`). The region override determines which geographic searches surface the community, so an ex-creator can re-target a community to a different state/locality, make it disappear from legitimate member searches, or repeatedly flip it as the only privileged write they retain.

This is the same broad pattern as #2132 (`SendMessage` missing authorization) and #2133 (chat read paths retain access via stale `ParticipantIds`): the underlying privilege source (membership/ownership) was correctly revoked by `LeaveCommunity`, but a particular RPC was never updated to re-check it.

`SetCommunityRegionOverride` is the only privileged community write keyed on immutable `CreatorId`. Reference patterns already in the codebase:

- `DeleteCommunity` (`lifecycle.go:188-190`) — gates on **current** `OwnerUserId`.
- `UpdateCommunity` (`lifecycle.go:124`) — gates on **active membership** via `auth.RequireMemberOfActiveCommunity`.
- `RestoreCommunity` (`lifecycle.go:323-337`) — gates on caller appearing in `deleted_snapshot.member_user_ids`.

The fix is mechanically small but requires a product call: who should be allowed to set a region override — current owner only, any current member, or current creator (with membership)? See Open Questions.

## Context from Comments

One comment from `@briangbrown` (collaborator): `@claude plan`. No other discussion. The issue body is detailed and self-contained — including a recommended fix sketch and a regression-test outline — so the planning work is mostly about confirming the product intent and locking down the exact membership/ownership shape.

## Potential Solutions

### Option A: Owner-only (current owner, with implicit active-membership)

- **Approach:** Replace the `CreatorId` check with a single call to `auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID)` (single round-trip, returns the `Community` so we don't refetch), then check `community.OwnerUserId != authInfo.UserID → PermissionDenied`. Matches `DeleteCommunity`'s ownership gate plus `UpdateCommunity`'s membership gate, in one round-trip. The membership check is essentially defense-in-depth on top of the owner check — owners are always members today — but it's free (single round-trip via the join helper) and protects against any future bug that leaves stale `OwnerUserId` on a community whose ownership transfer half-failed.
- **Pros:** Strictest privilege model; aligns with how the system already treats "destructive admin actions" (delete is owner-only). Region overrides change the community's discoverability for every member — owner-grade matches the blast radius. No new product surface (no UI required for non-owners to ever invoke this).
- **Cons:** Slightly more restrictive than the status quo for *current* creators who are still members but have handed off ownership; they lose access. Given that handoff is a deliberate "I'm stepping back" action, this is almost certainly the intended outcome, but it is a behavior change.
- **Effort:** Small — handler change + regression test + log-message update. No proto change. No client change (the screen that calls this RPC is already owner-gated in the app; we should verify).

### Option B: Member-only (any current active member)

- **Approach:** Replace the `CreatorId` check with `auth.RequireMemberOfActiveCommunity` only — drop the ownership clause. Any active member can set the override.
- **Pros:** Matches `UpdateCommunity`'s gating exactly. Simplest to reason about. Smallest possible privilege change relative to status quo (still requires being in the community).
- **Cons:** Too permissive for an action that silently re-targets discoverability. A single rogue or compromised member could move the community out of its members' search surface. Inconsistent with `DeleteCommunity` (owner-only) for an action that is similarly destructive in the discoverability dimension.
- **Effort:** Small — same shape as Option A minus the `OwnerUserId` line.

### Option C: Creator-with-membership (preserve "creator-forever" intent, plug the membership hole)

- **Approach:** Keep the `CreatorId` check, but require the caller to *also* be an active member via `auth.RequireMemberOfActiveCommunity`. Plugs the exploit (Mallory leaves → no longer a member → check fails) without changing the product story of "creator can always tweak this."
- **Pros:** Smallest semantic change from the status quo if the original design intent really was "creator forever." Plugs the exploit completely.
- **Cons:** Anomalous in the codebase — no other privileged RPC keys on `CreatorId`. After ownership handoff, the original creator can still mutate region overrides even though they are otherwise a normal member; that asymmetry is surprising and is the kind of thing future audits will flag again. Also: if the creator subsequently leaves and rejoins, they regain a privilege the new owner does not have, which is bizarre.
- **Effort:** Small.

## Recommendation

**Option A (Owner-only + active membership)** via a single `RequireMemberOfActiveCommunity` call followed by an `OwnerUserId` check.

Reasoning: setting a region override is a discoverability-altering admin write — it's in the same category as `DeleteCommunity`, which is already owner-only. Aligning the two is the right design and it kills the exploit cleanly. The asymmetry in Option C ("creator forever, even after handoff, but only on this one RPC") is a code smell that future security passes will keep re-discovering. Option B (any member) is too permissive for a write that silently re-targets discoverability.

There is a small product-behavior change: a creator who has handed off ownership but is still a member loses the ability to override regions. We should verify with maintainers, but this is almost certainly the right outcome given handoff semantics.

## Open Questions

- **Should the original creator retain region-override rights after handing off ownership?** — **Recommendation:** No. Handoff is the explicit "I'm stepping back" action; the new owner is the one accountable for community-wide settings going forward. If maintainers say "yes, creator forever" we fall back to **Option C** and the implementation cost is trivial (drop the `OwnerUserId` line from Option A's diff, keep the `CreatorId` check). Either way the membership requirement is non-negotiable — that's the actual security bug.
- **Should this emit a `CommunityEvent` for audit?** — **Recommendation:** Out of scope for this issue (the issue is "ex-member can write," not "writes are unaudited") but worth filing a follow-up. There is currently no `COMMUNITY_EVENT_TYPE_*` for region-override changes; the existing handler doesn't publish one. Adding one would require a new enum value classified per the undo registry rules in `CLAUDE.md` ("Adding New CommunityEventType Values") — best done in its own PR.
- **Does the Flutter client already owner-gate this RPC?** — **Recommendation:** Verify before merging Option A. If the client's region-override screen is reachable by non-owner members today, switching to owner-only will surface as a confusing `PermissionDenied` instead of a hidden button. If the client is already owner-gated, no Flutter work is needed. (Quick `grep -r setCommunityRegionOverride app/lib` will confirm.)
- **Where should the regression test live?** — **Recommendation:** New file `server/services/community/regions_test.go` (no test file currently exists for `regions.go`). Mirror `lifecycle_test.go`'s table-style for `TestService_DeleteCommunity`: cover (1) owner can set, (2) non-owner member gets `PermissionDenied`, (3) non-member gets `PermissionDenied`, (4) the issue's exploit — original creator who handed off and left gets `PermissionDenied`.

## Implementation Plan

### Phase 1: Verify client behavior and fix the handler

**Goal:** Plug the exploit, align with existing owner-gated RPCs, and ship a regression test.
**Tasks:**
- [ ] Run `grep -rn "setCommunityRegionOverride\|SetCommunityRegionOverride" app/lib` to confirm the Flutter client only invokes this RPC from an owner-gated surface. If a non-owner code path exists, file a follow-up Flutter ticket and gate the UI in the same PR (small).
- [ ] Edit `server/services/community/regions.go:67-137` (`SetCommunityRegionOverride`):
  - Replace the `auth.RequireActiveCommunity` call (line 85) with `community, _, err := auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID)` — single round-trip, returns the same `*models.Community` the rest of the handler uses.
  - Replace the `community.CreatorId != authInfo.UserID` block (lines 90-93) with `if community.OwnerUserId != authInfo.UserID { return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("only the current community owner can set region override")) }`.
  - Update the `logger.WarnContext` message to match the new gate ("user is not community owner").
- [ ] Add `server/services/community/regions_test.go` with `TestService_SetCommunityRegionOverride` and four subtests:
  - `owner_can_set` — happy path, verify a `CommunityRegion` row is written with `IsOverride=true`.
  - `non_owner_member_denied` — second member of the community (non-owner) gets `connect.CodePermissionDenied`.
  - `non_member_denied` — user with no membership gets `connect.CodePermissionDenied`. (Confirmed against `evaluateActiveMembership` in `server/auth/community_active.go:215-230`: for an active community, a non-member returns `PermissionDenied`; for a soft-deleted community, a non-member returns `NotFound`. The `RequireActiveCommunity` call earlier in the handler already rules out the soft-deleted branch, so `PermissionDenied` is the only outcome here.)
  - `original_creator_after_handoff_denied` — the exploit scenario from the issue. Create community as user A, invite user B, call `LeaveCommunity` with `new_owner_user_id=B`, then call `SetCommunityRegionOverride` as A. Expect `connect.CodePermissionDenied`. This is the regression test the issue specifically asks for.
  - Use the existing helpers in `common_test.go` (`setupTestStorage`, `setupTestService`, `setupTestUser`, `createAuthenticatedContext`, `addUserToCommunity`) following the `TestService_UpdateCommunity` / `TestService_DeleteCommunity` style.
- [ ] Seed a `Region` row in each test (the handler does `s.storage.GetByID(req.Msg.RegionId, &models.Region{})` — see `regions.go:96-100`).
- [ ] Run `npm run build` (lint + `go build`). Then `go test ./server/services/community/... -timeout 5m` to confirm both the existing community tests still pass and the new test covers the exploit.

**Validates:** The exploit described in the issue is closed (regression test fails on `main`, passes after the handler fix); existing owner/member/non-member behavior matches the rest of the community service.

### Phase 2 (follow-up, optional, separate issue): Audit-log the region-override write

**Goal:** Make region-override writes observable in the community event stream so future tampering attempts are visible without grepping logs.
**Tasks:**
- [ ] File a follow-up issue: add `COMMUNITY_EVENT_TYPE_REGION_OVERRIDE_SET` to `proto/ripls/models/community_event.proto`.
- [ ] Classify the new enum value in `server/undo/registry.go` per `CLAUDE.md` (likely `ClassificationClientOnly` — the inverse is "set a different override" or "let the auto-recompute take over," both already reachable).
- [ ] Publish the event from `SetCommunityRegionOverride` at the end of the handler.

**Validates:** Region-override changes appear in the existing community event surface so they're auditable end-to-end. Not required to close #2134.

## Related Issues

- #2132 — `SendMessage` missing authorization. Same broader pattern (privilege artifact not re-checked after revocation).
- #2133 — chat read paths retain access via stale `ParticipantIds`. Same broader pattern.
