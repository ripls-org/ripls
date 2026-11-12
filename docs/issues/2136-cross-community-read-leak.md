# Issue #2136: Security: experience.GetExperience and experience.ListExperiences leak event data across community boundaries

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2136
**Filed:** 2026-05-23 | **Last activity:** 2026-05-23
**Labels:** bug, security, server | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

A family of authenticated-but-not-authorized read RPCs returns full
community-scoped entity data to any caller who can supply an entity ID or a
`community_id`. The peer RPCs all gate on `auth.RequireMemberOfActiveCommunity`
or an equivalent participant check; the affected handlers either skip the
membership check entirely or make it conditional on an empty request field
the client controls. Once a UUID is learned through any channel (invite-link
resolution, stale post-leave membership, screenshot, log leak), the attacker
can dump the full feature surface — events, requests, gear, stories, per-user
impact rollups — for communities they have no membership in.

The leak is broader than the issue title suggests. The original report covers
`experience.GetExperience` and `experience.ListExperiences`; the four
follow-up comments add `experience.ListExperienceNeedsAndContributions`,
`feed.ListItemStories`, `request.GetRequest`,
`request.ListRequestNeedsAndContributions`, four `gear` reads
(`GetGear`, `GetGearPeople`, `GetGearStats`, `GetTransferRequestCount`),
and three `impact_metrics` reads (`GetUserImpactMetrics`,
`GetUserCommunityImpactDetail`, `GetCommunityActions`). The community-aggregate
impact handlers (`GetCommunityImpactMetrics`, `GetCommunityMetricDetail`,
`GetCommunityActsDetail`, `GetCommunityUtilization`, `GetTimeToSolveDetail`,
`GetCommunityPercentileDetail`, `GetCommunityLeaderboard`) are borderline —
they gate on `RequireActiveCommunity` only, which may be the intentional
"community discovery / public preview" pattern (`GetCommunity` is the
reference) or a missing check. That decision needs product input.

The shared root cause: every cited handler is a community-scoped read on an
entity (gear / experience / request / story / user-in-community / community
itself) where the membership invariant of "caller is an active member of at
least one community the entity is shared with" is not enforced. The fix shape
is the same across every site, so one helper closes them all.

## Context from Comments

Five comments build out the scope:

- **Comment 1 (briangbrown):** Adds `experience.ListExperienceNeedsAndContributions`
  with the same `RequireAuth`-then-`fetchExperienceForRead` shape. Confirms
  write-side needs/contributions RPCs correctly gate on `requireParticipant`
  (owner-or-RSVP-YES/MAYBE).
- **Comment 2 (briangbrown):** Adds `feed.ListItemStories`. Sibling RPC
  `feed.ListStories` correctly uses `RequireMemberOfActiveCommunity`.
  Notes story payload includes title, description, media IDs, full
  participants, cross-references, impact, and kicker. Suggests the same
  per-item-to-community-set fanout helper.
- **Comment 3 (briangbrown):** Adds three `impact_metrics` handlers
  (`GetUserImpactMetrics`, `GetUserCommunityImpactDetail`, `GetCommunityActions`)
  with clear missing checks, plus seven borderline community-aggregate handlers
  the comment author refuses to file without product input.
- **Comment 4 (briangbrown):** Adds the `request` service mirrors
  (`GetRequest`, `ListRequestNeedsAndContributions`) and four `gear` reads
  including the explicit TODO at `gear/service.go:396`. Names
  `profile.GetUserProfileForViewer` as the reference design for the
  viewer-aware response shape (compute caller's shared-community lens first,
  render the response through that lens).
- **Comment 5 (briangbrown):** `@claude plan` — explicit request for the
  plan path, which is why this doc exists rather than a fast-path PR.

Cross-references to #2132 / #2133 / #2134 (same broader class on the
chat-side). Those issues do not yet have plan docs or open PRs, so this
plan must own the helper design rather than compose with theirs. The
helper should be shaped so #2132/#2133/#2134 can adopt it as well.

## Codebase research

What already exists that the fix should reuse, not reinvent:

- `server/auth/community_active.go`
  - `RequireMemberOfActiveCommunity(ctx, store, communityID, userID)` —
    single-community member check with NotFound / FailedPrecondition /
    PermissionDenied semantics in one round-trip.
  - `RequireActiveCommunity(ctx, store, communityID)` — no-member variant.
  - `FilterActiveMemberCommunities(ctx, store, communityIDs, userID)` —
    batch variant returning the subset of communities the caller is in.
  - `WithCommunityCache(ctx)` — request-scoped memoization of
    `(communityID, userID)` lookups via `sync.Map`; opt-in at the top of
    an RPC. The helper introduced here should compose with this so a
    single RPC that does multiple sub-checks pays for one DB round-trip.
- `server/services/chat/authorization.go`
  - `collectSharedCommunityIDs(ctx, store, conversation)` — fans a chat
    topic out to its community set via `CommunityGear` /
    `CommunityExperience` / `CommunityRequest` (or `topic.GetCommunityId()`
    for community-wide topics).
- `server/services/chat/helpers.go`
  - `requireConversationAccess(ctx, conversationID, userID)` — participant
    bypass, then per-community membership check across the fan-out.
    Returns `PermissionDenied` if `len(sharedCommunityIDs) == 0`.
    This is the closest existing prior art for the helper this plan
    introduces.
- `server/services/experience/needs.go::requireParticipant` and
  `server/services/chat/authorization.go::requireParticipant` — narrower
  participant-only checks already used by the write-side RPCs. The new
  helper's owner-bypass is the cross-entity generalization of the
  owner half of `experience.requireParticipant`.

The pattern this plan recommends:

```
requireAccessToCommunityScopedEntity(
    ctx, store, callerID,
    entityType, entityID, ownerID,
)
```

returning `(sharedCommunityIDs []string, callerCommunityIDs []string, error)`.
Both slices are useful: handlers that need to render a viewer-aware response
(per comment 4's `profile.GetUserProfileForViewer` reference) get
`callerCommunityIDs` to filter `SharedCommunities`/`MemberCount` to the
viewer's lens; handlers that only need the gate ignore the second return.

## Potential Solutions

### Option A: One shared helper in `server/auth`, applied at every call site

- **Approach:** Add `server/auth/community_scoped_access.go` containing
  `RequireAccessToCommunityScopedEntity(ctx, s, callerID, entityKind, entityID, ownerID) ([]string, []string, error)`.
  `entityKind` is an enum (`EntityGear`, `EntityExperience`, `EntityRequest`)
  that selects the `CommunityX` table to fan out. Owner-bypass is in the
  helper, not at the call site, so handlers stay one line:

  ```go
  shared, callerIn, err := auth.RequireAccessToCommunityScopedEntity(
      ctx, s.storage, authInfo.UserID,
      auth.EntityExperience, expStored.Id, expStored.OwnerId,
  )
  if err != nil { return nil, err }
  ```

  Use `callerIn` in `populateExperienceSharedCommunities` /
  `populateRequestSharedCommunities` / `populateGearSharedCommunities`
  to filter the response to only the viewer's lens (the
  `profile.GetUserProfileForViewer` pattern from comment 4).

  For `feed.ListItemStories`, resolve `req.Msg.ItemId` to one of the three
  entity kinds via a small dispatch (try CommunityGear, then
  CommunityExperience, then CommunityRequest; the first match wins). For
  `impact_metrics.GetUserImpactMetrics`, the helper does not fit — that
  one uses `caller == target || share-any-community(caller, target)`,
  which is a different primitive that needs a separate
  `RequireSharedCommunityWithUser(ctx, s, callerID, targetID)` helper.

- **Pros:**
  - One reviewed primitive replaces ten ad-hoc gates. Audit surface
    shrinks; future RPCs in the same shape have a one-liner to adopt.
  - Owner-bypass and viewer-lens computation live in one place — same
    bug can't reappear in a future copy-paste.
  - Composes with `WithCommunityCache` already in `server/auth`.
  - The `RequireSharedCommunityWithUser` companion closes the
    `impact_metrics` per-user reads with the same shape.
- **Cons:**
  - New helper plus enum type to review and test up front. Adds a small
    fan-out cost on every read (one `QueryByField` for the
    `CommunityX` lookup) — already paid today by the `populate*SharedCommunities`
    helpers, so the marginal cost is zero if the call sites are
    refactored to reuse the lookup. Phase plan covers that consolidation.
- **Effort:** Medium. Helper + tests ~1 day; each service phase
  ~0.5 day; total ~4-5 days.

### Option B: Replicate the chat helper into each service

- **Approach:** Copy the `requireConversationAccess` / `collectSharedCommunityIDs`
  pair into `experience/`, `request/`, `gear/`, `feed/` as separate
  `requireExperienceAccess` / `requireRequestAccess` / etc. helpers.
  Each service owns its own.
- **Pros:**
  - No new package dependency. Each service stays self-contained.
  - Minimal review surface per service.
- **Cons:**
  - Four to five near-identical helpers that will drift. The whole
    issue *is* this pattern — replicating it preserves the conditions
    that produced it.
  - Owner-bypass and viewer-lens behavior get re-implemented per
    service; one will get it subtly wrong.
  - Does not address `impact_metrics` (different shape).
- **Effort:** Small per service, large in total. Estimated ~5-6 days
  with substantially more lines of code than Option A.

### Option C: Middleware that enforces a per-RPC-policy declaration

- **Approach:** Annotate every community-scoped read RPC in its proto
  definition with a `ripls.access = ENTITY_MEMBER_OF_ANY_SHARED_COMMUNITY`
  option, generate an interceptor that resolves the access policy from
  the request shape, and centralize enforcement at the transport layer.
- **Pros:**
  - Strongest invariant: a future RPC that forgets the gate gets it
    automatically.
  - Documents the access model in the proto, which is the API contract.
- **Cons:**
  - Large design + generator work. Buf option plumbing, request-shape
    introspection, owner-bypass semantics that vary per entity, and
    test scaffolding for the interceptor are all multi-day items
    individually.
  - Conflicts with current convention of per-RPC dedicated request
    types — the interceptor would need to know which field carries
    the entity ID per RPC, re-introducing the boilerplate the option
    promises to eliminate.
  - Pulls the scope of a security-incident response far past what is
    needed to close this issue.
- **Effort:** Large. Multi-week.

## Recommendation

**Option A.** It matches the prior art (`requireConversationAccess`),
addresses every site cited in the issue and the four comments with one
helper + one companion (per-user variant), and produces a primitive that
#2132/#2133/#2134 can adopt instead of duplicating logic. The marginal
DB cost is zero because the affected handlers already perform the
`CommunityX` fan-out today to populate `SharedCommunities` in the
response — Phase 1 of the implementation plan consolidates the lookup
so the gate and the population share one query.

Option B is on the table only if there is a near-term constraint
against adding to `server/auth`. There isn't one — `server/auth` already
contains the per-request memoization primitive (`WithCommunityCache`)
and the batch primitive (`FilterActiveMemberCommunities`); a per-entity
primitive is the natural sibling.

Option C is over-scoped for an in-flight security incident. The
declarative-policy idea is worth filing as a follow-up if Option A's
adoption pattern proves brittle, but it should not block this fix.

## Open Questions

- **Should the response from `GetExperience` / `GetRequest` / `GetGear`
  expose all communities the entity is shared with, or only the ones
  the caller is a member of?** — **Resolved: return the full set.**
  The initial implementation filtered `shared_communities` and
  `total_distinct_member_count` to the caller's visible communities
  (viewer-scoped). After implementation review (#2143) the owner
  confirmed the UX must be unchanged: the `AccessRing` widget shows
  community icon thumbnails and a "X people" count that must represent
  the item's full reach, not a per-viewer subset. The access gate
  itself is correct and unchanged — a caller who belongs to none of
  the shared communities still receives `PermissionDenied`. But once
  the gate admits the caller, `shared_communities` returns every
  community the entity is shared with (names, media IDs, member
  counts), and `total_distinct_member_count` counts across all of
  them. `callerCommunityIDs` returned by
  `RequireAccessToCommunityScopedEntity` is still available to
  handlers that need the viewer-filtered subset for other purposes
  (e.g. community-switcher dropdowns) but is not used for these
  response fields.

- **Should `impact_metrics.GetUserImpactMetrics(target_user_id)` allow
  any caller who shares a community with the target, or only the
  target themselves?** — Recommendation: "any active-community-shared
  caller, plus the target." This matches the existing app surface
  (member profiles show impact rollups). Tighter than that breaks the
  member-profile flow; looser preserves the IDOR. Add a feature flag
  if product wants to keep the looser behavior temporarily.

- **What is the right answer for the seven borderline
  community-aggregate impact handlers (`GetCommunityImpactMetrics`,
  `GetCommunityMetricDetail`, `GetCommunityActsDetail`,
  `GetCommunityUtilization`, `GetTimeToSolveDetail`,
  `GetCommunityPercentileDetail`, `GetCommunityLeaderboard`)?** —
  Recommendation: treat as out of scope for this issue. Add
  `RequireMemberOfActiveCommunity` opportunistically only if product
  confirms non-members should not see them; otherwise leave the
  current `RequireActiveCommunity` gate in place and document the
  "public-preview" intent with a comment, modeled on `GetCommunity`.
  File a separate issue to resolve.

- **Where should `feed.ListItemStories` dispatch from item ID to
  entity kind?** — Recommendation: a private `feed.resolveItemKind(ctx, itemID) (auth.Entity, string, error)`
  that tries `CommunityGear` → `CommunityExperience` → `CommunityRequest`
  in order and returns the first hit. The `ItemId` in the proto is
  already polymorphic; no schema change needed. Returns NotFound if
  no community-scoped owner row exists. The cost is at most three
  indexed queries per call, all hitting existing indexes.

- **Should `gear.GetGear` retain its silently-conditional community
  context (i.e., `community_id == "" → no community fields populated`)
  or require a community context now?** — Recommendation: keep the
  conditional behavior for the populated fields (some clients fetch
  by gear ID alone to render the share-link landing), but add the
  access gate unconditionally (call the new helper before populating
  anything). Owner-bypass means owners reading their own gear keep
  working; non-owner readers must be in at least one shared community.

- **How does this plan compose with #2132 (chat SendMessage missing
  participant/member check) and #2133 (stale ParticipantIds)?** —
  Recommendation: ship Phase 1 of this plan first (the helper) and
  expose it. Update #2132 and #2133 plans to reuse it for the
  conversation-topic fan-out path. The chat-side helper
  (`requireConversationAccess`) becomes a one-line wrapper that
  delegates to the auth-package primitive plus the participant
  short-circuit. No duplicate behavior, one audit surface.

## Implementation Plan

### Phase 1: Shared helper + unit tests

**Goal:** Land the primitives in `server/auth` so every later phase
becomes a small mechanical change.

**Tasks:**
- [ ] Create `server/auth/community_scoped_access.go` exporting:
  - `EntityKind` enum: `EntityGear`, `EntityExperience`, `EntityRequest`.
  - `RequireAccessToCommunityScopedEntity(ctx, s, callerID, kind, entityID, ownerID) (sharedCommunityIDs []string, callerCommunityIDs []string, err error)`.
    - Owner-bypass: if `callerID == ownerID && ownerID != ""`, returns
      the full `sharedCommunityIDs` and the same value for
      `callerCommunityIDs` (owner sees everything).
    - Loads the appropriate `CommunityGear` / `CommunityExperience` /
      `CommunityRequest` rows via `QueryByField`. If
      `len(sharedCommunityIDs) == 0`, return `PermissionDenied`
      (matches `requireConversationAccess`).
    - Calls `GetCommunitiesWithMembership(ctx, sharedCommunityIDs, callerID)`
      in one batched JOIN. Active communities the caller is a member of
      fill `callerCommunityIDs`. If `len(callerCommunityIDs) == 0` and
      caller is not the owner, return `PermissionDenied`.
  - `RequireSharedCommunityWithUser(ctx, s, callerID, targetID) error`:
    self-bypass when `callerID == targetID`; otherwise load
    `CommunityUser` rows for caller, load `CommunityUser` rows for
    target, intersect, return `PermissionDenied` if disjoint.

  Error handling rules baked into both helpers:
  - **Storage failures** wrap with
    `connecterr.Internal(ctx, "RequireAccessToCommunityScopedEntity", err, "entity_id", entityID, "kind", kind.String())`.
    Bare `connect.NewError(connect.CodeInternal, err)` is gated by
    `scripts/check_codeinternal.js` (per `docs/server/architecture.md`
    §Error wrapping) and will fail CI.
  - **PermissionDenied paths** log at WARN, not ERROR
    (per `docs/server/observability.md` §Appropriate Log Levels —
    "user errors are INFO or WARN, not ERROR"; ERROR routes through
    the "Server Error Logged" alert policy and pages on-call). Use
    `logging.LoggerWithContext(ctx).WarnContext(ctx, "access denied",
    "operation", "RequireAccessToCommunityScopedEntity", "caller_id", callerID,
    "entity_id", entityID, "kind", kind.String(), "reason", "<owner|none|nonmember>")`.
    The existing per-RPC `rpc_errors` log-based metric groups by
    `operation`, so the per-handler WARN naturally feeds the alerting
    pipeline without new metric infrastructure.
  - **Log field naming**: when extending the helper to handle
    `models.Request` topics (Phase 3), use `target_request_id` for the
    entity ID, never `request_id`. `LoggerWithContext` already injects
    `request_id` as the HTTP correlation ID and a collision silently
    overwrites one with the other
    (per `docs/server/observability.md` §Request ID glossary).

- [ ] Add `server/auth/community_scoped_access_test.go`:
  - Owner can read own entity that is shared nowhere.
  - Member of a shared community can read; non-member cannot.
  - Ex-member (soft-deleted membership) cannot read.
  - Entity shared with a soft-deleted community + caller is a member of
    that deleted community → still denied (matches active-only
    invariant).
  - Caller in shared community A but not B → `callerCommunityIDs`
    contains only A.
  - Composes with `WithCommunityCache` (second call in same ctx is one
    fewer query). Use `storage.WithQueryStats(ctx)` /
    `storage.AssertMaxQueries(t, ctx, max, fn)` to lock in the
    query-count expectation (per `CLAUDE.md` §SQL Efficiency).
  - `RequireSharedCommunityWithUser`: self-bypass, shared-community,
    disjoint-membership, target-does-not-exist cases.
- [ ] Run `npm run build` and `go test ./server/auth/...`.

**Validates:** The helper's semantics are nailed down before any
call-site cutover.

### Phase 2: Experience service — close the originally reported leak

**Goal:** Stop the bleed on `experience.GetExperience`, `ListExperiences`,
`ListExperienceNeedsAndContributions`.

**Tasks:**
- [ ] `server/services/experience/get.go::GetExperience` — remove the
  `if req.Msg.CommunityId != ""` gate; replace with
  `auth.RequireAccessToCommunityScopedEntity(ctx, s.storage, authInfo.UserID, auth.EntityExperience, expStored.Id, expStored.OwnerId)`.
  Pass `callerCommunityIDs` into a new
  `populateExperienceSharedCommunitiesForViewer` that filters
  `SharedCommunities` and `TotalDistinctMemberCount` to the
  caller's lens.
- [ ] `server/services/experience/get.go::ListExperiences` — add
  `auth.RequireMemberOfActiveCommunity(ctx, s.storage, req.Msg.CommunityId, authInfo.UserID)`
  at the top, mirroring `ListRequests` / `ListCommunityGear`.
- [ ] `server/services/experience/needs_and_contributions.go::ListExperienceNeedsAndContributions` —
  add the new helper after `fetchExperienceForRead`. Reuse the same
  call shape as `GetExperience`.
- [ ] Add regression tests in `server/services/experience/get_test.go`
  and `needs_and_contributions_test.go`:
  - Non-member calling `GetExperience` with empty `community_id` →
    `connect.CodePermissionDenied`.
  - Non-member calling `ListExperiences(community_id=X)` →
    `connect.CodePermissionDenied`.
  - Ex-member calling either → `connect.CodePermissionDenied`.
  - Owner of an experience shared nowhere can still read it.
  - Member of one of two shared communities sees only one in
    `SharedCommunities`.
- [ ] `npm run build` and `go test -tags=integration -timeout 20m ./server/services/experience/...`.

**Validates:** The pattern works end-to-end in the service that
originated the report.

### Phase 3: Request service mirrors

**Goal:** `request.GetRequest` and `request.ListRequestNeedsAndContributions`.

**Tasks:**
- [ ] `server/services/request/queries.go::GetRequest` — same
  treatment as `GetExperience`. Owner-bypass via `requestStored.RequesterId`.
- [ ] `server/services/request/queries.go::populateRequestSharedCommunities` —
  filter to viewer's lens.
- [ ] `server/services/request/contributions.go::ListRequestNeedsAndContributions` —
  add helper after `GetByID(request)`.
- [ ] Regression tests in `queries_test.go` and `contributions_test.go`
  matching Phase 2 shape.
- [ ] Build + targeted tests.

**Validates:** Helper generalizes cleanly to `Request`.

### Phase 4: Feed service — ListItemStories

**Goal:** Close the polymorphic-item-ID read.

**Tasks:**
- [ ] Add private `feed.resolveItemKind(ctx, store, itemID) (auth.EntityKind, string, error)`
  returning the entity kind and owner-ID; tries
  `CommunityGear` → `CommunityExperience` → `CommunityRequest`,
  first hit wins. NotFound if none.
- [ ] `server/services/feed/stories.go::ListItemStories` — call
  `resolveItemKind`, then the helper, before `ListByItem`.
- [ ] Regression test: non-member calling `ListItemStories` for an
  item in a community they don't belong to → `PermissionDenied`.
  Member call works.
- [ ] Build + targeted tests.

**Validates:** Helper supports the polymorphic item-ID surface.

### Phase 5: Gear service — four reads + the explicit TODO

**Goal:** Close `GetGear`, `GetGearPeople`, `GetGearStats`,
`GetTransferRequestCount`. Remove the explicit TODO at
`gear/service.go:396`.

**Tasks:**
- [ ] `server/services/gear/service.go::GetGear` — replace the TODO
  with a helper call. Keep the existing conditional
  `community_id != "" → populateCommunityFields` block; the gate
  is unconditional, the populated fields are still conditional.
  Filter `populateGearSharedCommunities` to viewer's lens.
- [ ] `server/services/gear/people.go::GetGearPeople` — add helper
  call after `GetByID`.
- [ ] `server/services/gear/stats.go::GetGearStats` — same.
- [ ] `server/services/gear/service.go::GetTransferRequestCount` — same;
  owner-bypass is already implicit in the existing `isOwner` branch,
  but the non-owner branch returns `recipient_id == callerID`
  transfers, which is fine and stays. The helper guards the
  pre-existing `GetByID` lookup.
- [ ] Regression tests covering each handler with non-member,
  ex-member, and owner-of-unshared-gear cases.
- [ ] Build + targeted tests.

**Validates:** Helper works for the gear surface, including the
"owner of unshared gear" case for unpublished items.

### Phase 6: Impact metrics — per-user reads

**Goal:** Close `GetUserImpactMetrics`, `GetUserCommunityImpactDetail`,
`GetCommunityActions`. Leave the seven aggregate handlers alone (see
Open Questions).

**Tasks:**
- [ ] `server/services/impact_metrics/user_metrics.go::getUserImpactMetrics` —
  promote `auth.RequireAuth` to a real error path (currently
  swallowed at line 109), then call
  `auth.RequireSharedCommunityWithUser(ctx, store, authUser.UserID, req.Msg.UserId)`.
- [ ] `server/services/impact_metrics/user_impact_detail.go::getUserCommunityImpactDetail` —
  add `auth.RequireMemberOfActiveCommunity(ctx, store, req.Msg.CommunityId, authInfo.UserID)`
  alongside the existing `RequireActiveCommunity`.
- [ ] `server/services/impact_metrics/actions.go::getCommunityActions` —
  same upgrade from `RequireActiveCommunity` to
  `RequireMemberOfActiveCommunity`.
- [ ] Regression tests: stranger calling with a target UUID →
  `PermissionDenied`; shared-community caller works; self always works.
- [ ] Build + targeted tests.
- [ ] File a follow-up issue for the seven borderline aggregate handlers
  that links back to this issue and requests product input on the
  member-vs-non-member visibility question.

**Validates:** The companion `RequireSharedCommunityWithUser` primitive
works in production-shaped code paths.

### Phase 7: Sweep + post-fix audit

**Goal:** Make sure no other community-scoped read was missed and that
the fix is observable.

**Tasks:**
- [ ] Sweep for the anti-pattern across all RPC handlers in `server/services/`.
  Concrete commands the implementer should run:

  ```bash
  # Handlers that call RequireAuth but never call any access-gate helper.
  rg -lP '^\s*(auth\.)?RequireAuth\b' server/services/ \
    | xargs rg -L 'RequireMemberOfActiveCommunity|RequireActiveCommunity|RequireAccessToCommunityScopedEntity|RequireSharedCommunityWithUser|requireConversationAccess|requireParticipant|requireOwnership|RequireRole'
  ```

  Cross-check each hit by hand. Handlers that scope to the caller
  (e.g. `ListMyExperiences`) are acceptable false positives;
  handlers that accept a caller-controlled ID and load a
  community-scoped model are not. File a fresh issue for each
  legitimate hit, link back to #2136, and add it to the sweep
  follow-up (do not expand the scope of this PR).

- [ ] Verify observability is in place for the new WARN log lines.
  No new Prometheus metric or alert policy is required — the
  existing `rpc_errors_{environment}` log-based metric in
  `terraform/modules/monitoring/main.tf` groups by
  `metric.label.operation` and will automatically aggregate denials
  per RPC. If an unexpected denial spike fires the `rpc_errors`
  alert, the linked log entry surfaces `caller_id`, `entity_id`,
  and `reason` for triage. Confirm by tailing `gcloud logging read
  'jsonPayload.operation:"RequireAccessToCommunityScopedEntity"'`
  in dev after the cutover.
- [ ] Update `server/auth/doc.go` (the canonical place per
  `CLAUDE.md` §Style — every Go package owns its own `doc.go`
  package comment) with one paragraph naming the new primitives
  and the one-line recipe for adopting them in new RPCs. If
  `server/auth/README.md` exists, mirror the recipe there;
  otherwise leave the README work to a future PR (this issue is a
  security fix, not a documentation overhaul).
- [ ] Final `npm run build` + `go test -tags=integration -timeout 20m ./...`.

**Validates:** No regressions across the broader server; future
copy-paste of the broken pattern is a one-line bug.

## Audit notes

This plan has been audited against `docs/server/architecture.md` and
`docs/server/observability.md`. Adjustments made during the audit:

- **Helper error wrapping** — explicit requirement to use
  `connecterr.Internal` rather than bare `connect.NewError(connect.CodeInternal, ...)`.
  Enforced by `scripts/check_codeinternal.js`.
- **PermissionDenied log level** — pinned to WARN rather than ERROR
  to avoid paging on-call for legitimate user-error volume. Aligns
  with the observability doc's "user errors are INFO or WARN, not
  ERROR" rule and with the existing `Server Error Logged` alert
  policy that fires on any ERROR-level line.
- **Field naming for `models.Request` topics** — explicit reminder
  to use `target_request_id` (not `request_id`) per the observability
  doc's Request ID glossary, to avoid collision with the HTTP
  correlation ID injected by `LoggerWithContext`.
- **Metrics instrumentation** — removed the proposed
  `auth_access_denied_total` Prometheus counter from Phase 7.
  Per `docs/server/observability.md`, business metrics are still
  future work; the existing `rpc_errors` log-based metric
  (operation-grouped) already covers per-handler denial trends
  without new infrastructure. Avoids adding metric surface that
  would need its own alert wiring.
- **Sweep step concreteness** — replaced "grep for the
  anti-pattern" with a concrete `rg` command and explicit rule
  that legitimate hits get fresh issues filed (not bundled).
  Keeps the security PR tight.
- **Service-boundary compliance** — confirmed
  `feed.resolveItemKind` reads `CommunityGear` /
  `CommunityExperience` / `CommunityRequest` rows directly via
  storage, not via the gear/experience/request services.
  Aligns with `docs/server/architecture.md` §Service Independence
  ("services should not call or depend on other services").

## Related Issues

- **#2132** — Security: ChatService.SendMessage skips participant/community-member
  authorization check. Same root cause class on the write side of chat;
  the helper introduced here is the primitive that #2132's fix should
  delegate to.
- **#2133** — Security: ex-community-members retain chat access via stale
  ChatConversation.ParticipantIds. Tightly coupled to #2132; both should
  adopt the helper.
- **#2134** — Security: SetCommunityRegionOverride retains write access
  after creator leaves community. Same shape, different RPC; resolves
  to `RequireMemberOfActiveCommunity` on the write path. No helper
  reuse, but the audit sweep in Phase 7 should pick up any related
  region-handler oversights.
