# Issue #1137: With realtime update, feed view flashes/spins unnecessarily on minor updates

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1137
**Filed:** 2026-04-15 | **Last activity:** 2026-04-25
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/realtime_updates.md

## Problem Statement

When a real-time community event fires for a detail-level change (e.g., someone RSVPs to an experience, going from 1 to 2 attendees), the feed view briefly flashes and shows a loading spinner. This happens because `FeedNotifier` listens to `contentCacheInvalidationProvider` and calls `_refreshInPlace()` on every content invalidation — including detail-level events like RSVPs that do not change what items appear in the feed, only their internal state.

`_refreshInPlace()` makes a network call to refetch the feed, then replaces `state.items` with the fresh list. If the server returns the feed reordered (e.g., the RSVPed experience has a newer `lastActivityAtUnixSec` and moves to the top), the `PageView.builder` renders a different item at the user's current page index. Because items are keyed by `ValueKey(item.id)`, Flutter destroys the old `ExperienceContentView` widget and creates a fresh one — which starts with `isLoading: true` and shows a loading spinner until `initialize()` completes.

The design intent, documented in `docs/realtime_updates.md`, is clear: detail-level events should fire only the content provider, "avoiding unnecessary feed and search reloads." The code violates this intent.

## Context from Comments

- The triage comment (2026-04-25) escalated this to P1, citing "high-impact issue for key personas tied to top-tier priorities (story activation, core loops, or growth use cases)."
- No technical comments or workarounds. This is a clean bug report with a video repro.

## Potential Solutions

### Option A: Add a dedicated `feedListingCacheInvalidationProvider`

Introduce a new `feedListingCacheInvalidationProvider` that fires only for events that structurally change the feed listing (items added or removed: `GEAR_SHARED`, `GEAR_UNSHARED`, `REQUEST_CREATED`, `REQUEST_CANCELLED`, `EXPERIENCE_CREATED`, `REQUEST_FULFILLED`). The `FeedNotifier` listens to this new provider instead of `contentCacheInvalidationProvider`. Detail-level events continue firing only `contentCacheInvalidationProvider`, which `ExperienceContentView` / `GearContentView` / `RequestContentView` already use to silently refresh their own state.

- **Approach:** New `_InvalidationTarget.feedListing` in `EventRouter`; new `feedListingCacheInvalidationProvider` in `providers.dart`; `FeedNotifier.build()` listens to the new provider.
- **Pros:** Precisely maps to the documented two-level granularity. Surgical: only listing-level events trigger a feed network call. Test-visible: the existing `event_router_test.dart` pattern extends naturally.
- **Cons:** Adds a new provider, which adds a small amount of boilerplate.
- **Effort:** Small.

### Option B: Stop refreshing the feed in-place on content events entirely

Remove the `contentCacheInvalidationProvider` listener from `FeedNotifier` and rely solely on the 30-second poll backstop (via `feedStatusCacheInvalidationProvider`) for new-item discovery, plus the explicit user pull-to-refresh.

- **Approach:** Delete the `ref.listen(contentCacheInvalidationProvider, ...)` block from `FeedNotifier.build()`.
- **Pros:** Zero flashing — no in-place refresh at all.
- **Cons:** New feed items (GEAR_SHARED, REQUEST_CREATED) no longer appear in real time; user waits up to 30 seconds. Violates the real-time update design goal.
- **Effort:** Trivial (deletion), but wrong product outcome.

### Option C: Debounce longer and suppress reorders

Keep the existing listener but increase the debounce delay and, after fetching fresh data, merge items preserving their current order (only appending genuinely new items, never reordering). This avoids the widget-destruction case.

- **Approach:** Longer debounce (e.g., 1500ms); custom merge logic in `_refreshInPlace()` that preserves position of existing items and only inserts/removes items at stable positions.
- **Pros:** No new provider needed.
- **Cons:** Complex, fragile merge logic. Still makes an unnecessary network call on RSVP events. Masks the root cause rather than fixing it.
- **Effort:** Medium, with ongoing maintenance burden.

## Recommendation

**Option A** — add `feedListingCacheInvalidationProvider`. It directly implements the documented intent (two-level granularity), is minimal in scope, and follows the existing invalidation provider pattern established by `transferCacheInvalidationProvider`, `portfolioCacheInvalidationProvider`, etc. It completely eliminates unnecessary network calls and widget destruction for detail-level events.

## Open Questions

- **Should `REQUEST_FULFILLED` and `REQUEST_CANCELLED` trigger a feed refresh?** These archive the item, which the server already filters out of the feed response (`CommunityRequest.archived`). Without a feed refresh, the item would linger in the feed until the next explicit refresh or poll cycle. Recommendation: yes, keep them as listing-level events so the feed stays accurate in real time.
- **Should `GEAR_UNSHARED` remain a listing-level event?** Similarly, it archives the item. Recommendation: yes.
- **Are there other event types not yet classified?** The current enum has 23 values. All are handled in the router's switch. No new events need to be classified for this fix.

## Implementation Plan

### Phase 1: Introduce `feedListingCacheInvalidationProvider` and wire it up

**Goal:** Eliminate unnecessary feed refreshes triggered by detail-level events (RSVP, offer, etc.).

**Tasks:**
- [ ] Add `feedListingCacheInvalidationProvider` and `FeedListingCacheInvalidationNotifier` to `app/lib/services/providers.dart`, following the same pattern as `contentCacheInvalidationProvider`.
- [ ] Add `feedListing` to `_InvalidationTarget` enum in `app/lib/services/event_router.dart`.
- [ ] In `EventRouter._notifyTargets()`, wire `feedListing` to call `feedListingCacheInvalidationProvider.notifier.notify()`.
- [ ] In `EventRouter._processEvent()`, add `_InvalidationTarget.feedListing` to the target sets for listing-level events: `GEAR_SHARED`, `GEAR_UNSHARED`, `REQUEST_CREATED`, `REQUEST_CANCELLED`, `EXPERIENCE_CREATED`, `REQUEST_FULFILLED`.
- [ ] In `FeedNotifier.build()`, change `ref.listen(contentCacheInvalidationProvider, ...)` to `ref.listen(feedListingCacheInvalidationProvider, ...)`.
- [ ] Update tests in `app/test/services/event_router_test.dart`: assert listing-level events fire `feedListingCacheInvalidationProvider` and detail-level events do NOT.
- [ ] Update tests in `app/test/presentation/viewmodels/feed_view_model_test.dart`: assert that firing `contentCacheInvalidationProvider` alone does NOT trigger `_refreshInPlace()`, and that firing `feedListingCacheInvalidationProvider` DOES.
- [ ] Update the routing table in `docs/realtime_updates.md` to add a `feedListing` column and split the current "feed" target into two rows: `feedStatus` (sidebar indicator) and `feedListing` (in-place feed refresh). Update the "Content listing changes" and "Request fulfillment" rows to show both `feedStatus` and `feedListing` as separate providers.

**Validates:** No spinner or flash on RSVP; feed still updates in real time on gear share / request creation.

---

## Architecture Audit Notes

*Audit performed 2026-04-25 against `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, and `docs/server/architecture.md`.*

**Result: Plan is compliant. One gap found and addressed (task added above).**

### Compliant

- **Repository/ViewModel boundary.** The fix operates entirely within the client event routing layer (`EventRouter`, `FeedNotifier`, `providers.dart`). No repository logic is modified. `FeedNotifier` continues to call `_repository` for data and never accesses `StashCacheManager` directly — consistent with the caching architecture rule that repositories own caching and ViewModels never access `CacheManager` directly.
- **Invalidation provider pattern.** Adding `feedListingCacheInvalidationProvider` as a `NotifierProvider<..., int>` with a `notify()` method exactly follows the established pattern used by `contentCacheInvalidationProvider`, `feedStatusCacheInvalidationProvider`, `transferCacheInvalidationProvider`, `portfolioCacheInvalidationProvider`, `searchCacheInvalidationProvider`, and `impactCacheInvalidationProvider`.
- **No manual cache maps in ViewModels.** The fix does not introduce any `Map<String, T>` caches in `FeedNotifier`.
- **Async-first.** `_refreshInPlace()` remains async; no sync cache access is introduced.
- **Service independence (server).** This is a pure client fix. No server changes are needed or proposed.
- **i18n.** No new user-visible strings are introduced. No ARB file changes needed.
- **No hardcoded colors or navigation violations.** The fix touches no UI widgets.

### Gap Found

**`docs/realtime_updates.md` routing table is not updated in the plan.** The doc at `docs/realtime_updates.md` (lines 141–149) contains a routing table with a "feed" column. After this fix the "feed" target will be split into two distinct providers: `feedStatus` (sidebar new-items indicator, `FeedStatusCacheInvalidationNotifier`) and `feedListing` (in-place feed refresh, `FeedListingCacheInvalidationNotifier`). The existing table would become misleading — it currently shows a single "feed" provider for listing-level events, but post-fix there will be two separate providers with different purposes. The implementation task list above has been updated to include updating this doc.

### No Other Issues

The `docs/client/i18n.md`, `docs/server/architecture.md`, and `docs/client/architecture.md` documents have no bearing on this fix and introduce no constraints that the plan violates.

## Related Issues

- #999 — original real-time update feature request that introduced the current architecture
