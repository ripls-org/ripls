# Issue #665: [Bug Report] cross community leakage

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/665
**Filed:** 2026-01-30 | **Last activity:** 2026-03-29
**Labels:** bug, user-feedback | **Assignees:** maintainer
**Context docs:** none

## Problem Statement

When viewing an experience that is shared to multiple communities, RSVP and
attendee data leaks across community boundaries. Users in Community A can see
RSVPs from Community B. This is a P0 trust-breaking bug — community boundaries
are the atomic trust unit in ripls, and cross-community data exposure undermines
the safety premise that makes sharing possible.

The root cause is twofold:

1. **Server: conditional community filtering.** Three RPCs (`GetExperiencePeople`,
   `GetExperienceStats`, and `GetExperience` via `buildRSVPs`) query
   `ExperienceRSVP` records. The first two only filter by `community_id` when the
   request field is non-empty — otherwise they return all RSVPs across all
   communities. `buildRSVPs` never filters by community at all.

2. **Client: optional community context.** Several call sites pass `communityId`
   as nullable/optional, meaning the server receives an empty string and falls
   through to the unfiltered path. Example:
   `conversation_view_model.dart:756` — `communityId: communityId.isNotEmpty ? communityId : null`.

## Context from Comments

- **tescobar (maintainer, 2026-03-05):** Confirmed this is full cross-community
  data exposure, not just owner votes. Classified as P0. Trust circles are the
  atomic unit — if community boundaries leak, the product's trust premise fails.
- **maintainer (maintainer, 2026-03-29):** Requested an integration test that
  demonstrates the bug (listing RSVPs for multi-community experiences shows all
  participants regardless of community), then fix the leakage so the test passes.

## Potential Solutions

### Option A: Make community_id required on all RSVP-reading RPCs

- **Approach:** Change `community_id` from optional to required in
  `GetExperiencePeopleRequest`, `GetExperienceStatsRequest`, and
  `GetExperienceRequest`. Server returns `InvalidArgument` when empty. Update
  `buildRSVPs` to accept and filter by community_id. Fix client call sites to
  always pass community context.
- **Pros:** Eliminates the bug class entirely — no code path can accidentally
  skip filtering. Simple to implement and verify. Future RPCs follow the same
  pattern.
- **Cons:** Breaking change for any caller that currently relies on the
  unfiltered behavior (no known legitimate use case). Requires auditing all
  client call sites.
- **Effort:** Medium.

### Option B: Server-side enforcement with fallback to user's community

- **Approach:** When `community_id` is empty, infer it from the authenticated
  user's active community membership or return an error. Keep the proto field
  optional but enforce filtering server-side.
- **Pros:** Backward-compatible with existing client code.
- **Cons:** "Active community" is ambiguous — a user may belong to multiple
  communities. Inference logic adds complexity and could produce unexpected
  results. Masks the real problem (client should know which community it's in).
- **Effort:** Medium-large.

### Option C: Filter server-side, always by community, using request context

- **Approach:** Same as Option A but keep `community_id` optional in the proto.
  When missing, return empty RSVP lists rather than all RSVPs. No client changes
  required for correctness (they already pass communityId in most paths).
- **Pros:** No breaking proto change. Fail-safe — missing context returns
  nothing rather than everything.
- **Cons:** Doesn't surface the missing-context bug to callers; they silently get
  empty results which could confuse debugging. Doesn't force client discipline.
- **Effort:** Small.

## Recommendation

**Option A** — make `community_id` required on RSVP-reading RPCs. The proto
field can remain `optional` (for wire compatibility), but the server should
validate it is non-empty and return `InvalidArgument` otherwise. This forces
every caller to be explicit about community context and eliminates the entire
class of leakage bugs.

The client already passes `communityId` in the main call paths
(`experience_metric_view_model.dart`, `experience_view_model.dart`). The few
places that pass `null` (e.g., `conversation_view_model.dart:756`) need to be
fixed to propagate the community context they already have.

`buildRSVPs` needs a `communityID` parameter added, and its sole caller
(`GetExperience` in `get.go:50`) already has `req.Msg.CommunityId` available.

## Open Questions

- **Should GetExperience return RSVPs at all, or should clients always use
  GetExperiencePeople?** — Recommendation: Keep RSVPs on GetExperienceResponse
  for now (it's the primary detail view), but filter them. Removing would be a
  larger refactor with no immediate benefit.
- **Are there admin/analytics use cases that need cross-community RSVP counts?**
  — Recommendation: No known use case. If one arises, add a separate
  admin-scoped RPC with explicit cross-community semantics rather than weakening
  the isolation on user-facing RPCs.

## Audit Notes

<!-- Added by architecture audit -->
- **Server patterns compliant**: No cross-service dependencies introduced;
  changes stay within the experience service package. Error handling uses
  `connect.NewError` with appropriate codes (per server architecture doc).
- **Client architecture compliant**: Phase 3 correctly audits viewmodel→repository
  call sites. No direct service access from viewmodels. Repository caching layer
  is unchanged.
- **Caching impact: none required.** The experience repository already keys
  people/stats caches by `communityId` when present
  (`people:$experienceId:$communityId`). Since caches are memory-only with
  30-minute TTL, any stale cross-community entries expire naturally after the
  server fix deploys. No explicit cache invalidation migration needed.
- **i18n impact: none.** No new user-facing strings.
- **`GetExperience` scope note:** `get.go` already passes `communityId` to
  `buildAPIExperience` for community-context display. Requiring it for the
  entire RPC (not just the RSVP portion) is appropriate since experiences
  are always viewed within a community context. The
  `populateExperienceSharedCommunities` call doesn't need community_id
  (it returns all shared communities for the switcher dropdown).
- **No proto changes needed.** Fields remain `optional` in proto definitions;
  validation is server-side only. This avoids a wire-breaking change while
  still enforcing the constraint.

## Implementation Plan

### Phase 1: Integration test demonstrating the bug
**Goal:** Prove the leakage exists with a failing test, per Bryan's comment.
**Tasks:**
- [ ] Add `TestGetExperiencePeople_CrossCommunityIsolation` in `people_test.go`:
  create 2 communities, share experience to both, have different users RSVP in
  each community, call `GetExperiencePeople` with community A's ID, assert only
  community A RSVPs are returned
- [ ] Add `TestGetExperience_BuildRSVPs_CrossCommunityIsolation` in `get_test.go`:
  same setup, call `GetExperience` with community A's ID, assert `rsvps` field
  only contains community A RSVPs
- [ ] Add `TestGetExperienceStats_CrossCommunityIsolation` in `stats_test.go`:
  same setup, call `GetExperienceStats` with community A's ID, assert attendee
  counts reflect only community A
- [ ] Verify all three tests FAIL (demonstrating the bug)
**Validates:** The leakage is real and reproducible via server RPCs.

### Phase 2: Server-side fix
**Goal:** Enforce community filtering on all RSVP-reading code paths.
**Tasks:**
- [ ] `people.go`: Change conditional filter to required — return
  `InvalidArgument` when `CommunityId` is empty, always include `community_id`
  in query fields
- [ ] `stats.go`: Same pattern — require `CommunityId`, always filter
- [ ] `service.go` `buildRSVPs`: Add `communityID string` parameter, use
  `QueryByFields` with both `experience_id` and `community_id`
- [ ] `get.go`: Require `CommunityId` on `GetExperience`, pass it to
  `buildRSVPs`. Note: `populateExperienceSharedCommunities` is unaffected (it
  queries by experience_id to build the community-switcher dropdown)
- [ ] Verify Phase 1 tests now PASS
- [ ] Run full `go test ./server/services/experience/...` — fix any existing
  tests that don't pass community_id
- [ ] Run `npm run build` to verify lint + compile
**Validates:** Community isolation is enforced server-side.

### Phase 3: Client call-site audit
**Goal:** Ensure all Flutter code paths pass community context.
**Tasks:**
- [ ] Audit all callers of `experienceRepository.getExperienceDetails`,
  `.getPeople`, `.getStats` in `app/lib/presentation/viewmodels/` — ensure
  `communityId` is always non-null (viewmodels are the only layer that calls
  repositories per architecture rules)
- [ ] Fix `conversation_view_model.dart:756` — pass `communityId` directly
  instead of converting empty to null
- [ ] Fix any other call sites where communityId might be null/empty
- [ ] Run `flutter test --reporter expanded` to verify no regressions
**Validates:** Client always provides community context to the server.

## Related Issues
- None referenced in the issue or comments.
