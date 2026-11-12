# Access Sheet / Community Selection Desync

## Problem

The access sheet and the community selection modal get out of sync when the
user makes quick repeated changes. Here is the exact scenario that triggers it:

1. User opens the access sheet for an experience.
2. User taps **Add Communities**, toggling on Community X.
   - The `CommunitySelectionModal` fires an API call; `experienceSharingNotifierProvider.sharedCommunityIds` now includes X.
3. User closes the modal. `_showAddCommunity()` reads the notifier's optimistic
   state and returns an updated `AccessGroup` list that includes X.
   `AccessSheet._currentGroups` is updated to show X.
4. User taps **Add Communities** again.
   - `_showAddCommunity()` is called a second time.
   - It calls `notifier.setSharedCommunities(experience.experience.sharedCommunityIds)`, seeding the notifier from the **server-fetched experience detail** — which still does **not** include X, because the server refresh is deferred until the outer access sheet is dismissed.
   - The modal opens showing X as **OFF** (stale) even though the access sheet above it shows X as **ON** (optimistic).
5. The user, confused, toggles X "off" — or the toggle fires in the wrong direction entirely.
6. The access sheet and the modal now reflect different truths, and subsequent dismissal/refresh may or may not reconcile them depending on which API call landed last.

---

## Root Causes

### 1. `setSharedCommunities` always reseeds from stale server state

In [`experience_content_view.dart:319`](../../app/lib/presentation/screens/experience/experience_content_view.dart#L319):

```dart
notifier.setSharedCommunities(experience.experience.sharedCommunityIds);
```

Every call to `_showAddCommunity()` unconditionally resets the sharing notifier
to `experience.experience.sharedCommunityIds`. This field comes from the
`experienceProvider` state, which is only refreshed after the **outer access
sheet** is dismissed. Accumulated optimistic changes from previous modal trips
are silently discarded on each subsequent open.

### 2. Two independent sources of truth diverge

`AccessSheet._currentGroups` is updated from the return value of
`_showAddCommunity()` (optimistic). `experience.experience.sharedCommunityIds`
is not updated until the server refresh runs. These two sources diverge the
moment any toggle fires.

### 3. Single `isSharing` flag blocks all communities

[`experience_sharing_view_model.dart`](../../app/lib/presentation/viewmodels/experience_sharing_view_model.dart)
uses a single `isSharing: bool` for all in-flight toggles. If one toggle is in
flight, every community row appears disabled. Gear and request sharing use
per-community `loadingCommunityIds: Set<String>` which is the correct pattern.
This also means rapid toggles on different communities serialize incorrectly.

---

## Open Questions

1. **Should the community selection modal support multi-select with a deferred
   commit, or remain immediate (fire-on-toggle)?**
   The current design fires API calls on every toggle. A "confirm" button would
   eliminate the interleaving problem but changes the UX contract.

2. **Should `experienceProvider` be updated optimistically on each toggle?**
   If so, `experience.experience.sharedCommunityIds` would stay in sync with
   `experienceSharingNotifierProvider`, eliminating the two-source problem
   entirely.

3. **Scope: experience only, or all item types?**
   Gear uses per-community `loadingCommunityIds` but the same stale-reseed
   pattern likely exists. Requests are the same. Is this fix experience-only or
   should it be applied uniformly?

---

## Recommended Implementation

### Fix 1: Preserve notifier state across repeated modal opens (minimal, high-confidence)

Add item-ID tracking to `ExperienceSharingState` so `_showAddCommunity()` can
detect whether the notifier is already initialized for the current item and skip
the destructive `setSharedCommunities` call.

**`experience_sharing_view_model.dart`** — add `initializedForItemId`:

```dart
@freezed
sealed class ExperienceSharingState with _$ExperienceSharingState {
  const factory ExperienceSharingState({
    String? initializedForItemId,          // NEW
    @Default([]) List<CommunityItem> userCommunities,
    @Default([]) List<String> sharedCommunityIds,
    @Default(<String>{}) Set<String> loadingCommunityIds,  // per-community (Fix 2)
    @Default(false) bool isLoadingCommunities,
    String? communitiesError,
    String? sharingError,
  }) = _ExperienceSharingState;
}
```

Update `setSharedCommunities` to track the item:

```dart
void setSharedCommunities(String itemId, List<String> communityIds) {
  state = state.copyWith(
    initializedForItemId: itemId,
    sharedCommunityIds: communityIds,
  );
}
```

**`experience_content_view.dart` — `_showAddCommunity()`**: only seed if not
yet initialized for this item:

```dart
final sharingState = ref.read(experienceSharingNotifierProvider);
if (sharingState.initializedForItemId != experience.experience.id) {
  notifier.setSharedCommunities(
    experience.experience.id,
    experience.experience.sharedCommunityIds,
  );
}
await notifier.loadUserCommunities();
```

This makes the notifier's accumulated optimistic state survive repeated modal
openings within the same access sheet session. When the access sheet is
dismissed and the server refresh runs, the next `_showAddCommunity()` call will
find `initializedForItemId` pointing to the same item but will accept the
freshly-fetched `sharedCommunityIds` — which is fine because the server state
is now authoritative.

> **`autoDispose` constraint:** `experienceSharingNotifierProvider` must remain
> **non-autoDispose**. Fix 1 relies on the notifier surviving between the modal
> close and the `_showAddCommunity()` return-value read. An autoDispose provider
> would be disposed the moment the modal loses its last watcher, destroying the
> optimistic state before the screen can read it.

> **Clearing `initializedForItemId`:** After `AccessSheet.show()` awaits (i.e.,
> the sheet is dismissed), `_showAccessSheet()` already calls
> `refreshExperienceDetails()`. At that same point, also clear the notifier:
>
> ```dart
> // In _showAccessSheet(), after AccessSheet.show() returns:
> if (mounted) {
>   ref.read(experienceSharingNotifierProvider.notifier).clearInitialization();
>   ref.read(experienceProvider(widget.experienceId).notifier)
>       .refreshExperienceDetails(communityId: communityId).ignore();
> }
> ```
>
> Add `clearInitialization()` to `ExperienceSharingNotifier`:
>
> ```dart
> void clearInitialization() {
>   state = state.copyWith(initializedForItemId: null);
> }
> ```
>
> This ensures the next access-sheet session (even for the same item) re-seeds
> from the freshly-fetched server state.

### Fix 2: Per-community loading state for experiences (correctness)

Replace `isSharing: bool` with `loadingCommunityIds: Set<String>` in
`ExperienceSharingState` (already shown above). Update `shareWithCommunity` and
`unshareFromCommunity` to add/remove the specific community ID from the set
instead of flipping the single flag.

Update `CommunityListItem._isLoading` for `'experience'`:

```dart
final state = ref.watch(experienceSharingNotifierProvider);
return state.loadingCommunityIds.contains(community.id);
```

This matches the gear and request patterns and allows concurrent toggles on
different communities.

### Fix 3: Reconcile experienceProvider on toggle (optional, stronger guarantee)

After a successful share/unshare, update `experienceProvider`'s
`sharedCommunityIds` so both sources stay consistent.

**Architecture constraint:** Do NOT do this inside `ExperienceSharingNotifier`.
The architecture rule is that ViewModels must not depend on other ViewModels'
providers — adding a `ref.read(experienceProvider(...))` call inside the sharing
notifier would create a cross-ViewModel dependency that mirrors the
service-to-service coupling the server architecture explicitly forbids.

Instead, handle reconciliation in the screen layer (`experience_content_view.dart`):
after each successful toggle callback returns, call a lightweight update on
`experienceProvider` to patch `sharedCommunityIds` in-place. This keeps the
sharing notifier self-contained and the reconciliation visible in the one place
(the screen) that already orchestrates both providers.

Only adopt this if Fixes 1 and 2 prove insufficient.

---

## Files to Change

| File | Change |
|------|--------|
| [`app/lib/presentation/viewmodels/experience_sharing_view_model.dart`](../../app/lib/presentation/viewmodels/experience_sharing_view_model.dart) | Add `initializedForItemId`, replace `isSharing` with `loadingCommunityIds: Set<String>` |
| [`app/lib/presentation/screens/experience/experience_content_view.dart`](../../app/lib/presentation/screens/experience/experience_content_view.dart) | Guard `setSharedCommunities` with item-ID check in `_showAddCommunity()` |
| [`app/lib/presentation/widgets/sharing/community_list_item.dart`](../../app/lib/presentation/widgets/sharing/community_list_item.dart) | Update `_isLoading` for `'experience'` to use `loadingCommunityIds` |
| [`app/lib/presentation/viewmodels/experience_sharing_view_model.freezed.dart`](../../app/lib/presentation/viewmodels/experience_sharing_view_model.freezed.dart) | Regenerate after model change (`flutter pub run build_runner build`) |

---

## Feed Duplication After Multi-Community Sharing

### Problem

When an item is shared with a second community via the access sheet, it appears
**twice** in the feed. Items must never appear more than once regardless of how
many communities they are shared with.

**Root cause — `GetFeed` deduplicates by event ID, not entity ID:**

[`server/services/feed/service.go:112-127`](../../server/services/feed/service.go#L112):

```go
seenItemIDs := make(map[string]bool)
for _, cid := range communityIDs {
    items, genErr := s.generateFeedItems(ctx, authInfo.UserID, cid, communities[cid])
    if genErr != nil {
        return nil, connect.NewError(connect.CodeInternal, genErr)
    }
    for _, item := range items {
        if !seenItemIDs[item.Id] {   // ← BUG: item.Id is CommunityEvent UUID
            seenItemIDs[item.Id] = true
            feedItems = append(feedItems, item)
        }
    }
}
```

`FeedItem.Id` is always `event.Id` — the `CommunityEvent` UUID. Each community
produces its own `CommunityEvent` record for the same underlying entity. Sharing
gear X with Community A and Community B creates two events:

| Community | `event.Id` | `GearId` in payload |
|-----------|------------|---------------------|
| A | `"event-uuid-A"` | `"gear-X-uuid"` |
| B | `"event-uuid-B"` | `"gear-X-uuid"` |

The dedup map sees two distinct keys, so both items survive — gear X appears
twice.

**Why within-community deduplication does not help:**

`generateFeedItems` already collapses multiple events for the same item within
one community using `itemKeyForEvent` (e.g., `"gear:gear-X-uuid"`). But that
key is discarded before the cross-community merge: the assembled `FeedItem`
carries only `event.Id`, not the entity key.

### Proposed Solution

Add a helper `feedItemEntityKey` that derives the same `"type:entity-id"` key
from the assembled `FeedItem` payload, and use it for the cross-community
deduplication map.

```go
// feedItemEntityKey returns a stable entity-scoped key for cross-community
// deduplication. This mirrors itemKeyForEvent but operates on the assembled
// FeedItem payload rather than the raw CommunityEvent.
func feedItemEntityKey(item *api.FeedItem) string {
    switch p := item.Payload.(type) {
    case *api.FeedItem_GearShared:
        return "gear:" + p.GearShared.GearId
    case *api.FeedItem_RequestCreated:
        return "request:" + p.RequestCreated.RequestId
    case *api.FeedItem_ExperienceCreated:
        return "experience:" + p.ExperienceCreated.ExperienceId
    case *api.FeedItem_CommunityCreated:
        return "community:" + p.CommunityCreated.CommunityId
    default:
        // Stories and nudges have no shared entity ID; item.Id is already unique.
        return item.Id
    }
}
```

Change the dedup loop in `GetFeed`:

```go
// After:
entityKey := feedItemEntityKey(item)
if !seenItemIDs[entityKey] {
    seenItemIDs[entityKey] = true
    feedItems = append(feedItems, item)
}
```

No proto changes, no client changes, no new RPCs required.

### Open Questions

1. **Which community's version wins?** The loop processes communities in
   `communityIds` order (client-provided). The first community's `FeedItem`
   wins, including its `is_unread` computation (scoped to that community's
   `FeedItemView`). An item the user has seen in Community B but not A will
   appear unread if A is processed first. Is this acceptable for the initial
   fix, or should the "most-unread wins" merge strategy be implemented?

2. **`MarkFeedItemsViewed` is single-community.** The client calls
   `markItemsViewed(currentCommunityIds.first, [item.id])` for every viewed
   item ([`feed_view_model.dart:135`](../../app/lib/presentation/viewmodels/feed_view_model.dart#L135)).
   Only the winning community's view record is written. Should the server
   fanout `MarkFeedItemsViewed` to all communities that share the entity?

3. **Story deduplication.** Stories are generated per community. If an
   experience is shared with two communities, could two completion stories
   appear in the same multi-community feed? Stories fall back to `item.Id`
   in `feedItemEntityKey`, so they would both appear. Is that intentional?

### Recommended Implementation

Implement `feedItemEntityKey` and the dedup loop change. Add a regression test
in `service_test.go` asserting a multi-community feed request returns each item
exactly once when it is shared with multiple communities.

`service.go` is already 1,240 lines (over the 1,000-line guideline). Do not add
`feedItemEntityKey` there. Instead, create `server/services/feed/feed_dedup.go`
and move the related helpers into it:

- `deduplicateEventsByItemKey` (currently in `service.go`)
- `itemKeyForEvent` (currently in `service.go`)
- `feedItemEntityKey` (new)

### Files to Change

| File | Change |
|------|--------|
| [`server/services/feed/feed_dedup.go`](../../server/services/feed/feed_dedup.go) | **New file.** Move `deduplicateEventsByItemKey` and `itemKeyForEvent` from `service.go`; add new `feedItemEntityKey()` |
| [`server/services/feed/service.go`](../../server/services/feed/service.go) | Remove moved functions; switch cross-community dedup loop from `item.Id` to `feedItemEntityKey(item)` |
| [`server/services/feed/service_test.go`](../../server/services/feed/service_test.go) | Add regression test for cross-community item deduplication |

---

## Discover Screen Deduplication Across Communities

### Problem

The same invariant that applies to the feed applies to discover: **an item
shared with multiple communities must appear at most once in discover results.**
A user with Communities A and B enabled who shares gear X with both must see
gear X once on the map and once in the carousel, not twice.

### Current State — Already Handled (with gaps)

Unlike the feed (which had a dedup-by-event-ID bug), the search service already
deduplicates by **entity ID**. In
[`server/services/search/service.go:88-125`](../../server/services/search/service.go#L88):

```go
seenIDs := make(map[string]bool)
for _, cid := range communityIDs {
    results, err = s.semanticSearch(ctx, perCommunityReq, itemTypes)
    for _, r := range results {
        itemID := extractSearchResultID(r)
        if itemID != "" && seenIDs[itemID] {
            continue
        }
        if itemID != "" {
            seenIDs[itemID] = true
        }
        apiResults = append(apiResults, r)
    }
}
```

`extractSearchResultID`
([`service.go:148-161`](../../server/services/search/service.go#L148)) extracts
the actual entity ID (`gear.Id`, `request.Id`, `experience.Id`, `user.Id`) from
the search result payload — not an event or join-table ID. This means the dedup
key is correct and the feed-style bug does not apply here.

**Gaps identified:**

1. **No type prefix on the dedup key.** `extractSearchResultID` returns the bare
   UUID (e.g., `"abc-123"`), not a typed key like `"gear:abc-123"`. If a gear
   and an experience share the same UUID, the second would be silently dropped.
   With random UUIDs this is astronomically unlikely, but the feed's
   `feedItemEntityKey` uses type prefixes and this should be consistent.

2. **No regression test.** There is no test in
   [`server/services/search/service_test.go`](../../server/services/search/service_test.go)
   that asserts cross-community deduplication. The single existing test
   (`TestSearch_EmptyCommunityID`) only validates the error case. A regression
   test — sharing an item with two communities and searching across both —
   should be added.

3. **"First community wins" ordering.** The loop processes communities in client
   order. If community A returns gear X at rank 5 and community B returns it at
   rank 1, community A's version (and position in the result list) wins because
   A is processed first. For discover, this is less impactful because the client
   re-sorts by distance/relevancy anyway, but the item payload (e.g.,
   `distance_meters`) may differ between communities.

### Open Questions

1. **Should `extractSearchResultID` use type prefixes?** Adding
   `"gear:" + id`, `"request:" + id`, etc. would make the dedup key consistent
   with `feedItemEntityKey` and eliminate the theoretical type-collision risk.
   The change is trivial but touches a working codepath.

2. **Should `maxResults` apply per-community or globally?** Currently,
   `maxResults` is forwarded to each per-community query (each community
   returns up to `maxResults`), and the merged result is truncated to
   `maxResults` after dedup. With many communities, this means the first
   community dominates results. Should the per-community limit be
   `maxResults / len(communityIDs)` to give each community a fair share?

### Recommended Implementation

1. **Add a regression test** in `service_test.go` asserting that a
   multi-community search request returns each item exactly once when it is
   shared with both communities. This is the highest-priority change — the
   dedup logic is correct today but has no test protecting it.

2. **Add type prefixes to `extractSearchResultID`** (optional, low risk). Rename
   to `searchResultEntityKey` for consistency with `feedItemEntityKey`, and
   return `"gear:" + id` etc. instead of bare IDs.

### Files to Change

| File | Change |
|------|--------|
| [`server/services/search/service_test.go`](../../server/services/search/service_test.go) | Add regression test for cross-community search deduplication |
| [`server/services/search/service.go`](../../server/services/search/service.go) | (Optional) Add type prefixes to `extractSearchResultID` for consistency with feed dedup |

---

## Existing Violations (Out of Scope)

These pre-existing issues were noticed during the audit and should be tracked separately:

- **i18n:** `CommunityListItem` uses hardcoded English strings for toast messages
  (`'Shared with ${community.name}'`, `'Removed from ${community.name}'`). Per
  [docs/client/i18n.md](../client/i18n.md), all user-visible strings must live in
  ARB files and be accessed via `context.l10n.*`.
