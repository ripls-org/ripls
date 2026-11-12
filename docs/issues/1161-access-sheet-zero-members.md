# Issue #1161: Community with 2 members listed as 0

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1161
**Filed:** 2026-04-17 | **Last activity:** 2026-04-17
**Labels:** bug, user-feedback | **Assignees:** tescobar
**Context docs:** docs/client/sharing.md

## Problem Statement

In the Access Sheet (the bottom sheet showing which communities can see an item),
a community with 2 actual members shows "0 members" in its accordion row. The
reporter suspects deduplication as the root cause.

## Context from Comments

No comments on the issue. The report came via in-app user feedback from DEV
environment (v0.15.0, Android 16, Pixel 9 Pro).

## Root Cause

The bug is in the **optimistic state** path used after the "Add Community" modal
closes inside the Access Sheet.

All three content views (`gear_content_view.dart`, `request_content_view.dart`,
`experience_content_view.dart`) have a `_showAddCommunity()` method that rebuilds
`AccessGroup` objects from the sharing viewmodel's optimistic state. This code
builds groups for **all** shared community IDs — both previously-shared and
newly-added — and falls back to `memberCount: 0` when the server-side
`SharedCommunity` proto hasn't been refreshed yet:

```dart
// gear_content_view.dart:405 (same pattern at request:356, experience:395)
final memberCount = existing?.memberCount ?? 0;
```

`existing` comes from `currentState.sharedCommunities` (the last server response).
For a **newly added** community, there is no entry in that map, so
`existing` is null and `memberCount` falls back to 0. The access sheet then
displays "0 members" on that community's row.

The server-side refresh is intentionally **deferred** until the access sheet is
dismissed (to batch network calls), so the 0 persists for the entire access sheet
session after adding a community.

### Files affected

| File | Line | Pattern |
|------|------|---------|
| `app/lib/presentation/screens/gear/gear_content_view.dart` | 405 | `existing?.memberCount ?? 0` |
| `app/lib/presentation/screens/request/request_content_view.dart` | 356 | `existing?.memberCount ?? 0` |
| `app/lib/presentation/screens/experience/experience_content_view.dart` | 395 | `existing?.memberCount ?? 0` |

### Why dedup was suspected

The `totalDistinctMemberCount` (shown in the ring and the "N can see this"
header) is a related dedup feature added recently. However, the per-community
`memberCount` shown in the accordion row is separate — it counts members for a
single community, no dedup involved. The confusion likely arose because both
numbers can show 0 in the optimistic state.

## Potential Solutions

### Option A: Use fetched members count in AccessSheet (recommended)

- **Approach:** In `AccessSheet._buildAccordionRow()`, prefer the actual fetched
  member list length over `group.memberCount` when the list is loaded:
  `final displayCount = members?.length ?? group.memberCount;`
  The AccessSheet already eagerly fetches members for all groups via
  `_fetchAllMembers()` in `initState()` and calls `_fetchMembers()` for
  newly-added groups in `_handleAddCommunity()`. Once fetched, the real count
  is available in `_membersCache`. This is a single-line fix in one file.
- **Pros:** Simplest fix — one file, one line. Doesn't require touching any
  content views. Self-corrects as soon as members load (typically <1s). No
  architecture violations. Works for all content types (gear, request,
  experience) since they all share the same `AccessSheet`.
- **Cons:** Brief flash of "0 members" before members load (sub-second). The
  underlying `group.memberCount` data is still 0 for newly-added communities.
- **Effort:** Small.

### Option B: Look up member count from communityProvider

- **Approach:** When `existing` is null (newly added community), read the
  member count from `communityProvider(id)` (a `FutureProvider.family` that
  calls `GetCommunity` and returns `num_members`).
- **Pros:** Accurate count from the server without a new network call if cached.
- **Cons:** `communityProvider` is keyed per-community and is **not guaranteed
  to be cached**. The sidebar loads communities via `ListCommunities` (the
  `selectedCommunityProvider`), not via `GetCommunity`. So
  `ref.read(communityProvider(id)).value` may return null, falling back to 0
  anyway. Requires touching all three content view files. <!-- AUDIT: corrected
  from original analysis which incorrectly assumed communityProvider would be
  cached from sidebar usage -->
- **Effort:** Small, but less reliable than Option A.

### Option C: Add `member_count` to `CommunityItem` proto

- **Approach:** Add a `member_count` field to `CommunityItem` in the
  `ListCommunities` response. Then `sharingState.userCommunities` would carry
  member counts, and the `toGroup()` fallback could use it.
- **Pros:** Clean long-term solution. `CommunityItem` is already used in
  multiple places that could benefit from this field.
- **Cons:** Requires proto change, server-side population, and code generation.
  Slightly larger scope.
- **Effort:** Medium.

### Option D: Trigger server refresh before returning groups

- **Approach:** After the sharing modal closes, call `refreshGearDetails()`
  (or equivalent) and wait for it, then rebuild groups from the refreshed
  server state.
- **Pros:** 100% accurate — all data comes from the server.
- **Cons:** Adds latency (network round-trip) before the access sheet updates.
  The current design intentionally defers the refresh to dismissal for
  performance. Defeats the purpose of optimistic state.
- **Effort:** Small, but trades UX responsiveness for accuracy.

## Recommendation

**Option A** — use the actual fetched member list length in `AccessSheet`.
It's the smallest change (one line, one file), doesn't violate any
architecture rules, and self-corrects as soon as the member fetch completes.
The AccessSheet already does the member fetch eagerly, so the data is
typically available within a fraction of a second.

<!-- AUDIT: Changed from original recommendation (communityProvider lookup)
because communityProvider is NOT guaranteed to be cached — the sidebar uses
ListCommunities (selectedCommunityProvider), not GetCommunity. Reading an
unresolved FutureProvider synchronously returns null, so the fallback to 0
would still occur. Option A avoids this entirely by using data the AccessSheet
already fetches. -->

## Open Questions

- **Is the initial display also affected?** — Recommendation: No. The initial
  `_buildAccessGroups()` method reads directly from server-side
  `SharedCommunity.memberCount`, which is populated correctly by the server. The
  bug only manifests after adding a community via the in-sheet modal. Verify by
  checking if the reporter had just added a community.

- **Should `totalDistinctMemberCount` also update optimistically?** —
  Recommendation: Not in this fix. The header "N can see this" already comes
  from the initial server response and doesn't update during the session. A
  separate enhancement could recalculate it client-side, but that's out of scope.

## Implementation Plan

### Phase 1: Fix member count display in AccessSheet

**Goal:** Newly added communities show correct member count in the access sheet.

**Tasks:**
- [ ] In `access_sheet.dart` `_buildAccordionRow()`, change the member count
  displayed in the subtitle from `group.memberCount` to
  `members?.length ?? group.memberCount`. This uses the actual fetched member
  list when available and falls back to the proto-supplied count otherwise.
- [ ] Verify fix on device: add a community via the access sheet, confirm member
  count appears correctly after the brief member fetch.

**Validates:** The "0 members" display no longer appears for newly added
communities in the access sheet.

### Phase 2: i18n compliance for member count string (recommended alongside Phase 1)

**Goal:** Replace the hardcoded English pluralization in the accordion subtitle.

The current string at `access_sheet.dart:446` is:
```dart
'Shared by ${group.sharedByName} · ${group.sharedTimeAgo} · ${group.memberCount} member${group.memberCount == 1 ? '' : 's'}'
```

This violates the i18n rule: "Every user-visible string should use
`context.l10n.*`" (docs/client/i18n.md). The member count portion should use
the existing `context.l10n.contentMemberCount()` ICU plural, and the full
subtitle should be composed via a localized format string.
<!-- AUDIT: flagged pre-existing i18n violation since we're already editing
this line -->

**Tasks:**
- [ ] Add a localized string key for the access sheet subtitle in
  `app_en.arb` and `app_es.arb` (e.g., `accessSheetSharedBy`).
- [ ] Replace the hardcoded string with `context.l10n` usage.
- [ ] Run `npm run generate` to regenerate localizations.

**Validates:** No hardcoded English in the access sheet subtitle.

### Phase 3: Consider adding `member_count` to `CommunityItem` (optional follow-up)

**Goal:** Make member count available wherever `CommunityItem` is used.

**Tasks:**
- [ ] Add `int32 member_count = 7;` to `CommunityItem` in `community_service.proto`.
- [ ] Populate the field server-side in `ListCommunities` handler.
- [ ] Run code generation.
- [ ] Update content views' `toGroup()` fallback to use
  `CommunityItem.memberCount` instead of defaulting to 0.

**Validates:** Member count is available without relying on the access sheet's
member fetch.

## Related Issues

- No directly related issues found. The dedup logic for `totalDistinctMemberCount`
  was added recently and is working correctly on the server side — the bug is
  purely in the client-side optimistic state fallback.
