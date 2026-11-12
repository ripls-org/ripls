# Issue #1169: Request shared with multiple communities creates duplicate request objects

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1169
**Filed:** 2026-04-17 | **Last activity:** 2026-04-17
**Labels:** none | **Assignees:** tescobar
**Context docs:** none

## Problem Statement

When a user creates a request and selects multiple communities, the app creates
separate `Request` records (one per community) instead of a single request shared
across all selected communities. This happens because `submitRequest()` in the
viewmodel loops over all community IDs and calls the `SubmitRequest` RPC for each
one, where each call creates a brand-new `Request` + `CommunityRequest` pair.

The correct behavior is to call `SubmitRequest` once for the first community, then
call `ShareRequest` for the remaining communities. The `ShareRequest` RPC already
exists and correctly creates additional `CommunityRequest` junction records pointing
to the same `Request.id`.

## Context from Comments

No comments on this issue. The issue description is thorough and includes the
root cause, expected behavior, and a clear fix specification. No clarification
needed.

## Potential Solutions

### Option A: Fix the loop in the viewmodel (recommended)

- **Approach:** Change `gen_request_view_model.dart` to call `submitRequest` once
  for the first community, then call `shareRequest` for the remaining communities.
- **Pros:** Minimal change, uses existing RPCs exactly as designed, matches how
  gear and experiences handle multi-community creation.
- **Cons:** None significant.
- **Effort:** Small.

### Option B: Modify the SubmitRequest RPC to accept multiple community IDs

- **Approach:** Change the `SubmitRequest` proto/RPC to accept a list of community
  IDs and handle the create-then-share logic server-side.
- **Pros:** Atomic operation, single network call.
- **Cons:** Proto change required, breaks the existing API contract, more work
  for a problem that already has a clean client-side fix. Gear and experiences
  don't work this way either, so it would be inconsistent.
- **Effort:** Medium.

## Recommendation

**Option A.** The fix is straightforward — the `ShareRequest` RPC already exists
and handles deduplication. This matches the pattern used by gear and experiences.
No server changes needed.

## Open Questions

- **Should analytics events fire once or per-community?** — Recommendation: Fire
  one `RequestCreatedEvent` for the initial submit, then fire `RequestSharedEvent`
  (or similar) for each additional community. This accurately reflects what
  happened. However, if no `RequestSharedEvent` exists, firing one
  `RequestCreatedEvent` for the first community is acceptable for now.

## Implementation Plan

### Phase 1: Fix the submit loop

**Goal:** Single request creation with multi-community sharing.

**Tasks:**
- [ ] In `gen_request_view_model.dart`, change the loop at lines 368-391:
  1. Call `_requestRepository.submitRequest()` with the **first** community ID
  2. If there are remaining community IDs, call `_requestRepository.shareRequest()`
     with the returned request ID and the remaining community IDs
  3. Log the analytics event for the initial creation, and log share events for
     the remaining communities
- [ ] Verify the `shareRequest` repository method accepts a list of community IDs
  (confirmed: it does, via `List<String> communityIds` parameter)
- [ ] No manual cache invalidation needed — `RequestRepository.shareRequest()` already
  invalidates request, community, feed, and search caches automatically
- [ ] Test manually: create a request with 2+ communities, confirm only one request
  appears in the feed with all communities in the access circle

### Phase 2: Verify no other duplicate patterns exist

**Goal:** Confirm gear and experiences don't have the same bug.

**Tasks:**
- [ ] Check gear submission flow for similar loop patterns
- [ ] Check experience submission flow for similar loop patterns
- [ ] If any similar bugs found, file follow-up issues

## Related Issues

None identified.
