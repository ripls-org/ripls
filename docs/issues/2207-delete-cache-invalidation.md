# Issue #2207: Delete doesn't seem to delete

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2207
**Filed:** 2026-05-27 | **Last activity:** 2026-06-10
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** [docs/client/architecture.md], [docs/client/conventions.md], [docs/client/caching.md], [docs/deleting_things.md], [docs/server/architecture.md], [docs/proto_conventions.md], [docs/users/profile.md]

## Problem Statement

A user (iOS, prod, app 0.22.0 build 7130) selected "delete permanently" on a
gear item ("Makita leaf blower"). The server delete succeeded, but the item
kept showing up on "different profile pages" within the same app session. The
screenshots show the gear surfacing on profile-scoped surfaces after the user
returned from the delete confirmation.

The root cause is **client-side cache staleness**, not server-side filtering:

- `Service.DeleteGear` (`server/services/gear/service.go:947`) correctly
  soft-deletes the gear row, cascades to conversations, `community_gear`
  joins, share links, and media. All server list/get queries filter
  `deleted IS NULL` by default, and `available_now.gearQualifies`
  (`server/available_now/available_now.go:297`) and
  `ListCommunityGear` (`server/services/community/gear_sharing.go:71`)
  explicitly skip gear with non-nil `Deleted` metadata.
- The Flutter client's `GearRepository.deleteGear`
  (`app/lib/data/repositories/gear_repository.dart:154`) invalidates only:
  the gear item cache, the user-gear list (`gear:user:list`), the feed
  cache, the search cache, the conversations cache, and the portfolio
  inbox (via `onDailyInvalidated`).
- It does **not** invalidate the **profile cache** (`profile:*` in
  `ProfileRepository` —
  `app/lib/data/repositories/profile_repository.dart:21`) or the
  **per-community gear lists** (`{communityId}:gear` in
  `CommunityRepository` —
  `app/lib/data/repositories/community_repository.dart:136`).

The profile cache is the load-bearing one for the reported screenshots: the
user-profile screen (`UserScreen` →
`viewerProfileProvider(targetUserId)`) renders the **Available Now rail**
straight from the cached `GetUserProfileForViewerResponse.availableNowItems`,
which contains the deleted gear until the entry's 30-minute (dev) /
60-minute (prod) TTL expires or the user manually pull-to-refreshes. Any
other profile page the user had already visited that session (their own,
other community members') keeps showing the gear from the same stale cache
namespace.

The same gap exists in `RequestRepository.deleteRequest`
(`app/lib/data/repositories/request_repository.dart:365`) and
`ExperienceRepository.deleteExperience`
(`app/lib/data/repositories/experience_repository.dart:397`) — neither
invalidates `profile:*` or `{communityId}:gear`-equivalent caches —
so a comprehensive fix should address all three.

## Context from Comments

The only comment on the issue is `@claude` from a maintainer, asking this
agent to triage. No clarifying questions or workarounds were provided.
The issue body, three screenshots, and device context (iOS prod) are the
complete signal. The screenshots confirm the gear persists on profile
surfaces after a successful delete — exactly what stale `profile:*`
cache entries would produce.

## Potential Solutions

### Option A: Callback-based profile invalidation (recommended)
- **Approach:** Add an `onProfileInvalidated` VoidCallback parameter to
  `GearRepository`, `RequestRepository`, and `ExperienceRepository` —
  mirroring the existing `onDailyInvalidated` callback. Wire it up in
  each repository's provider
  (`app/lib/services/providers/gear_providers.dart`,
  `request_providers.dart`, `experience_providers.dart`) to call
  `ref.read(profileRepositoryProvider).invalidateAll()`. Call the
  callback from `deleteGear`, `deleteRequest`, `deleteExperience`. Also
  invalidate the per-community gear cache pattern
  (`{communityId}:gear`) for every community the gear was shared with
  (the gear object exposes `sharedCommunityIds` server-side; we can
  fetch the gear via the existing cached `get(gearId)` call before
  deleting, then iterate its `sharedCommunityIds` and invalidate each).
- **Pros:** Matches the existing decoupling pattern
  (`onDailyInvalidated`, `onCommunityJoined`, etc.); ProfileRepository
  stays out of the gear/request/experience repos' constructors;
  consistent with how `CommunityRepository` already calls
  `profileRepository.invalidateAll()` from
  `onViewerMembershipChanged`. The "nuclear" `invalidateAll()` matches
  the doc's stated "over-invalidate but guarantee correctness"
  trade-off for cross-cutting profile/feed/search caches (see
  `docs/client/caching.md` Pattern 5).
- **Cons:** Slight over-invalidation — every cached profile entry gets
  dropped on every delete, including profiles unaffected by the
  deleted gear. Acceptable because deletes are infrequent, the cache
  TTL is short, and per-target dependency tracking would be far more
  complex.
- **Effort:** Small.

### Option B: Direct constructor injection of ProfileRepository
- **Approach:** Inject `ProfileRepository` as a constructor dependency
  in `GearRepository`, `RequestRepository`, `ExperienceRepository`.
  Call `_profileRepository.invalidateAll()` directly from the delete
  methods.
- **Pros:** Most explicit — the dependency is visible in the type.
- **Cons:** Tightens the dep graph and breaks symmetry with the
  existing `onDailyInvalidated` callback pattern. Would require a
  larger change to the three repositories' constructors and every
  call site (including tests that build repositories manually).
- **Effort:** Small-medium.

### Option C: Broadcast invalidation via a new notifier provider
- **Approach:** Add a `profileCacheInvalidationProvider`
  (`NotifierProvider<int>` counter) analogous to
  `portfolioCacheInvalidationProvider` and
  `searchCacheInvalidationProvider`. Repositories call `.notify()` on
  mutations; the `ViewerProfileNotifier` listens in its `build()` and
  triggers an in-place refresh on signal, matching the portfolio
  inbox pattern (`docs/client/caching.md` Pattern 7).
- **Pros:** Most idiomatic with the existing event-driven
  invalidation patterns; gives the same in-place refresh UX that
  portfolio inbox enjoys (no spinner on the active profile screen).
- **Cons:** Significantly more wiring (new provider, listener in
  view-model, integration with refresh-in-place state); overkill for
  the reported bug. Worth doing as a follow-up when more surfaces
  need live profile updates, but heavyweight for a delete-correctness
  fix.
- **Effort:** Medium.

## Recommendation

Implement **Option A**: callback-based `onProfileInvalidated` plus
broad community-gear-list invalidation. Rationale:

1. Smallest change that fixes the user-visible bug across all three
   delete paths.
2. Matches the established `onDailyInvalidated` callback pattern in
   the same repositories, keeping the architecture coherent.
3. The "nuclear" `profileRepository.invalidateAll()` matches the
   over-invalidate-for-correctness trade-off the codebase already
   accepts for pull-to-refresh and viewer-membership changes
   (see `docs/client/caching.md` Pattern 5).
4. Option C is the right long-term destination but is a larger
   refactor — file as a follow-up.

**No proto, server, or RPC changes are required.** The server's
`Service.DeleteGear` already cascades correctly; this is purely a
client-side cache invalidation gap.

## Open Questions

- **Should we also invalidate the workshop synthesis/brief caches
  (`workshop:synthesis:*`, `workshop_brief:brief:*`)?** —
  **Recommendation: yes, but narrow it.** The workshop brief is
  AI-generated and may reference the deleted gear in postcards. The
  workshop already auto-disposes per scope; a single
  `workshopRepository.invalidateAll()` call from the same
  `onProfileInvalidated`-equivalent callback (or fold both into a
  single `onContentMutated`) is cheap and prevents stale brief copy.
  Open to consolidating as `onContentMutated` during implementation
  if it reads cleaner.

- **Should we cancel the gear-delete server change?** —
  **Recommendation: no.** The server is correct; only the client
  caching is buggy. Adding more server-side cascades would not fix
  the symptom because the symptom is stale client cache, not a
  server-side leak.

- **Should we also invalidate per-community gear lists for requests
  and experiences?** — **Recommendation: yes — symmetric.**
  `CommunityRepository` caches `{communityId}:gear`,
  `{communityId}:requests`-equivalent, and
  `{communityId}:experiences`-equivalent lists used by community
  detail screens. The delete path should sweep all three.

- **Should we add a similar pull-to-refresh in the profile screen?**
  — **Recommendation: it already exists** (`UserScreen.RefreshIndicator`
  → `ViewerProfileNotifier.refresh()` invalidates the per-target
  entry). The fix here is for the implicit "I just deleted it, why
  is it still there" expectation, not the explicit refresh path.

## Implementation Plan

### Phase 1: Add profile + community-list invalidation to gear delete
**Goal:** Fix the load-bearing user-reported case (gear delete) so the
profile and community gear-list caches drop the deleted item.
**Tasks:**
- [ ] In `app/lib/data/repositories/gear_repository.dart`, add a
  `VoidCallback? onProfileInvalidated` constructor parameter
  alongside the existing `onDailyInvalidated`, and add a
  `CommunityRepository _communityRepository` dependency in the
  constructor (no circular import — `community_repository.dart` does
  not import `gear_repository.dart`; see also the existing pattern
  where `transfer_repository.dart` already takes `CommunityRepository`
  as a constructor arg). Inside `deleteGear`, after the server
  delete and the existing invalidations:
  1. Call the new `onProfileInvalidated` callback (does
     `profileRepository.invalidateAll()`).
  2. Call `_communityRepository.invalidateAllCommunityGearLists()` — a
     new helper that does `_cache.invalidatePattern('*:gear')` on the
     community namespace. The nuclear pattern is preferred over
     per-community iteration because: (a) it's symmetric with the
     existing `searchRepository.invalidateSearches()` /
     `feedRepository.invalidateAllFeeds()` calls already in
     `deleteGear`, (b) it avoids needing to fetch the gear before
     deletion to read `sharedCommunityIds`, (c) the community gear
     list isn't dense and re-fetch is cheap.
- [ ] In `app/lib/services/providers/gear_providers.dart`, wire
  `onProfileInvalidated` to
  `ref.read(profileRepositoryProvider).invalidateAll()` and pass
  `ref.watch(communityRepositoryProvider)` to the constructor.
- [ ] In `app/lib/data/repositories/community_repository.dart`, add
  the `invalidateAllCommunityGearLists()` helper (one-liner wrapping
  `_cache.invalidatePattern('*:gear')`). Document it as the
  delete-side counterpart to `refreshCommunityGear(communityId)`.
- [ ] In `app/test/data/repositories/gear_repository_test.dart`,
  extend the existing `group('deleteGear')` block with two tests:
  one that verifies `profileRepository.invalidateAll()` is called
  once, one that verifies
  `communityRepository.invalidateAllCommunityGearLists()` is called
  once. Update the test setup to include `MockProfileRepository` and
  `MockCommunityRepository` in the `@GenerateMocks(...)` annotation
  at the top of the file. Regenerate mocks via
  `dart run build_runner build --delete-conflicting-outputs` from
  `app/`.
- [ ] In `app/test/data/repositories/community_repository_test.dart`,
  add a test that `invalidateAllCommunityGearLists()` clears
  matching keys.
**Validates:** The reported bug — a freshly-deleted gear no longer
appears on the user's own profile, on other shared-community profiles
the user has visited that session, or in cached community gear lists.

### Phase 2: Mirror the fix in request and experience delete paths
**Goal:** Close the same gap for requests and experiences.
**Tasks:**
- [ ] Repeat Phase 1's repository + provider + test changes in
  `app/lib/data/repositories/request_repository.dart` (`deleteRequest`
  at line 365) and
  `app/lib/data/repositories/experience_repository.dart`
  (`deleteExperience` at line 397). Add `onProfileInvalidated`
  callbacks and call `profileRepository.invalidateAll()` from each.
  (The community-gear list helper applies only to gear; requests and
  experiences already invalidate `request:community:*` / equivalent
  patterns inside their delete methods.)
- [ ] Update `app/test/data/repositories/request_repository_test.dart`
  and `app/test/data/repositories/experience_repository_test.dart`
  with the same `profileRepository.invalidateAll()` verification
  pattern. Regenerate Mockito mocks for both.
- [ ] Wire `onProfileInvalidated` callbacks in
  `app/lib/services/providers/request_providers.dart` and
  `app/lib/services/providers/experience_providers.dart` (mirroring
  the `gear_providers.dart` wiring from Phase 1).
**Validates:** Symmetric behavior across the three deletable content
types — no path leaves a stale profile cache.

### Phase 3: Optional — workshop cache and consolidation
**Goal:** Stop the AI-generated workshop brief from referencing
deleted gear in its 15-minute cache window.
**Tasks:**
- [ ] Wire `WorkshopRepository.invalidateAll()` into the same
  `onProfileInvalidated` callback (rename to `onContentMutated` if
  it now covers profile + workshop). Add a single regression test
  per repository asserting the workshop cache is touched.
**Validates:** Postcard/brief copy regenerates on next open after a
delete, instead of carrying a 15-minute reference to a gone item.

## Related Issues

- #1701 — DeleteGear tombstone path; gear delete server flow already
  documented and tested. Confirms the server side is correct.
- #1064 — Prior "request cache stale" bug, same family of problems.
- #1996 — Self-view profile carve-out; the Available Now rail render
  path that this bug surfaces through.
- #1929 — Profile V4 redesign that introduced the
  `viewerProfileProvider` and `ProfileRepository` cache layer.

---

### Audit notes

Docs consulted: `docs/client/architecture.md` (anchor),
`docs/client/conventions.md` (anchor),
`docs/client/caching.md` (caching patterns, Pattern 5/7),
`docs/deleting_things.md` (soft-delete cascade rules),
`docs/server/architecture.md` (anchor — confirmed server flow is
correct and no server change is needed),
`docs/proto_conventions.md` (anchor — confirmed no proto change),
`docs/users/profile.md` (profile surface),
`docs/reviews/code_quality_review_prompt.md` (Areas to Review:
DRY, typed errors, no boolean-flag overuse — plan uses an additive
callback, not a new bool),
`docs/reviews/testing_review_prompt.md` (Areas to Review: repository
tests, mutation paths covered, mocks regenerated).

Changes during audit:
- Made the per-community invalidation strategy concrete: use
  `_cache.invalidatePattern('*:gear')` on the community namespace
  rather than iterating `sharedCommunityIds`. Matches the existing
  "search/feed invalidate-all" symmetry and avoids a pre-delete read
  that would otherwise be needed.
- Clarified that no proto / server / RPC changes are needed.
- Specified file paths and test files explicitly, including the
  Mockito `@GenerateMocks` update and the correct codegen command
  (`dart run build_runner build --delete-conflicting-outputs`).
- Confirmed no circular import risk (community_repository.dart does
  not import gear_repository.dart; transfer_repository.dart already
  takes CommunityRepository as a constructor arg, precedent
  exists).
