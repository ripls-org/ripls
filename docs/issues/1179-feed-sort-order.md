# Issue #1179: Feed items not sorted chronologically -- new items appear below older ones

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1179
**Filed:** 2026-04-18 | **Last activity:** 2026-04-18
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When a user creates a new item (e.g., a request), they expect it to appear at
the top of the feed. Instead, it can appear several positions down. The root
cause is the feed's multi-tier sort strategy, which prioritizes unread status
and action-required status over chronological order, combined with conversation
activity inflating the sort timestamp of older items.

The feed sort in `server/services/feed/service.go:135-150` works as follows:

1. **Unread items first** -- sorted by `UnreadAtUnixSec` desc
2. **Read non-action-required** -- sorted by `LastActivityAtUnixSec` desc
3. **Read action-required** -- sorted by `LastActivityAtUnixSec` desc

Within the unread tier, `UnreadAtUnixSec` is set to `LastActivityAtUnixSec`,
which is `max(event.OccurredAtUnixSec, latestNonViewerConversationMessage)`.
This means an older item with recent conversation activity from another user
will sort above a brand-new item whose only activity is its creation event.

A secondary issue exists in the cross-community deduplication
(`service.go:116-129`): when an item is shared with multiple communities, the
first occurrence by community iteration order is kept, without comparing
timestamps. The kept item's `LastActivityAtUnixSec` depends on which
community's event happens to be processed first.

## Context from Comments

No comments on the issue. The issue was filed based on a direct user report
where a newly created request appeared "several down" in the feed instead of
at the top.

## Potential Solutions

### Option A: Pure chronological sort by LastActivityAtUnixSec

- **Approach:** Replace the tiered sort with a single `LastActivityAtUnixSec`
  desc sort. Keep `is_unread` and `is_action_required` as visual indicators
  only (badges, bold text) without affecting sort position.
- **Pros:** Simplest change. Most predictable for users -- newest activity is
  always at the top. Matches the user's mental model ("I just created this, it
  should be at the top").
- **Cons:** An item with a new comment from someone else would sort above an
  item the user just created if the comment arrived 1 second later. Users lose
  the "unread items surfaced to the top" behavior, which helps catch up on
  missed activity. Could cause users to miss important unread items if they
  don't scroll down.
- **Effort:** Small.

### Option B: Use event creation time as primary sort, conversation activity for unread badge only

- **Approach:** Sort all feed items by `OccurredAtUnixSec` (the event creation
  timestamp) desc. Continue using conversation activity to determine `is_unread`
  status for badge display, but do not let it affect sort order. This decouples
  "when the item was created/shared" from "when the last chat message arrived."
- **Pros:** Newly created items always appear at the top. Conversation activity
  still triggers unread badges. Feed is stable -- items don't jump around as
  conversations happen.
- **Cons:** An item with active new conversation activity won't rise to the top,
  which means users may not notice a new comment on an older item unless they
  scroll or check badges. However, this is mitigated by the inbox/portfolio
  view which already handles conversation-based unread tracking.
- **Effort:** Small.

### Option C: Hybrid -- chronological with a short "unread boost" window

- **Approach:** Sort primarily by `LastActivityAtUnixSec` desc, but give newly
  created items (items where `OccurredAtUnixSec` == `LastActivityAtUnixSec`,
  i.e., no subsequent conversation activity) a small timestamp boost (e.g.,
  +60 seconds) so they reliably sort above items whose conversation activity
  happened at a similar time. Alternatively, use a two-level sort: first by
  `OccurredAtUnixSec` within a 5-minute bucket, then by `LastActivityAtUnixSec`.
- **Pros:** Newly created items almost always appear at the top. Conversation
  activity on older items still surfaces them, just slightly below brand-new
  items.
- **Cons:** More complex logic. Edge cases around the boost window. Still
  doesn't fully solve the problem if conversation activity on another item
  happens minutes after the new item is created.
- **Effort:** Medium.

## Recommendation

**Option B: Use event creation time as primary sort, conversation activity for
unread badge only.**

This is the cleanest separation of concerns. The feed becomes a chronological
timeline of "what happened in the community" sorted by when items were
created/shared. Conversation activity continues to drive unread badges and the
inbox/portfolio view. Users get the predictable "newest item at the top"
behavior they expect.

The `is_unread` and `is_action_required` fields remain on `FeedItem` and the
client can still render visual indicators -- we just stop using them as sort
keys.

For the cross-community dedup issue, when an item appears in multiple
communities, we should use the **earliest** `OccurredAtUnixSec` across all
community events for that item, since that represents when it first appeared.

## Open Questions

- **Should conversation activity still bump items in the feed at all?** --
  Recommendation: No. The feed is for discovery ("what's new in the
  community"). The inbox/portfolio is for tracking conversations. Mixing these
  concerns is what caused the current bug. If we want to surface chat activity,
  add a separate "recent activity" indicator rather than re-sorting.

- **Do the existing sort-order tests need to change?** -- Recommendation: Yes.
  `TestGetFeed_SortOrder_UnreadBeforeRead` (line 3050) and
  `TestGetFeed_SortOrder_ReadBeforeActionRequired` (line 3158) explicitly
  assert the tiered sort behavior. These need to be updated to assert
  chronological order instead.

- **Should `LastActivityAtUnixSec` still be computed and sent to the client?**
  -- Recommendation: Yes, keep computing and sending it. The client passes it
  through to content views (`feedLastActivityAtUnixSec` on
  `GearContentView`, `RequestContentView`, `ExperienceContentView`) for
  display purposes such as "last activity 2h ago" and for the `MarkFeedItemsViewed`
  timestamp tracking. Just stop using it as the sort key.

- **Does `GetFeedStatus` (sidebar "New" badge) need updating?** --
  Recommendation: No. `GetFeedStatus` in `feed_status.go` uses
  `isItemUnread()` which checks whether activity is newer than the user's
  last view -- this is independent of sort order and should continue using
  `LastActivityAtUnixSec` (including conversation activity) to determine
  whether new activity exists. The badge answers "is there something new?"
  which is a different question from "what order should items appear in?"

- **Should cross-community dedup use earliest or latest sharing timestamp?** --
  Recommendation: Use the earliest `OccurredAtUnixSec`. This represents when
  the item first appeared in the feed. Using the latest would cause a
  re-shared item to jump to the top, which could be confusing.

## Implementation Plan

### Phase 1: Change feed sort to chronological

**Goal:** Feed items sorted by event creation time, newest first.

**Tasks:**

- [x] In `service.go`, replace the tiered sort (lines 135-150) with a single
  sort by `OccurredAtUnixSec` desc. Keep `is_unread`, `unread_at_unix_sec`,
  and `is_action_required` fields populated for client-side badge display.
- [x] Verify that `GetFeedStatus` (`feed_status.go`) does NOT need changes --
  it uses `isItemUnread()` which is independent of sort order and should
  continue using `LastActivityAtUnixSec` to detect new activity for the
  sidebar badge.
- [x] Verify that `isItemFresh()` continues to use `LastActivityAtUnixSec`
  for the freshness/expiry filter -- this controls whether an item appears in
  the feed at all (not its position), and should still consider conversation
  activity so items with new comments don't expire prematurely.
- [x] Update `TestGetFeed_SortsByTimestamp` (line 229) to confirm the new
  behavior.
- [x] Update `TestGetFeed_SortOrder_UnreadBeforeRead` (line 3050) to assert
  chronological order: items sorted by `OccurredAtUnixSec` desc regardless of
  read status.
- [x] Update `TestGetFeed_SortOrder_ReadBeforeActionRequired` (line 3158) to
  assert chronological order regardless of action-required status.
- [x] Add a new test: create an item, add conversation activity to an older
  item, verify the new item still appears first.
- [x] Run `npm run build` to verify lint and compilation pass.

**Validates:** Newly created items always appear at the top of the feed.

### Phase 2: Fix cross-community dedup timestamp

**Goal:** When an item appears in multiple communities, use the earliest event
timestamp for sort position.

**Tasks:**

- [x] In the cross-community dedup loop (lines 116-129), when a duplicate
  entity key is found, compare `OccurredAtUnixSec` and keep the item with the
  earliest timestamp (since that's when the item first appeared).
- [x] Add a test: share an item with two communities at different times, verify
  the feed uses the earlier timestamp and the item appears only once.
- [x] Run `npm run build` to verify lint and compilation pass.

**Validates:** Items shared with multiple communities sort consistently by
their earliest sharing time.

## Audit Notes

**Changes made during audit (2026-04-17):**

1. **Added `GetFeedStatus` question and task.** The sidebar "New" badge uses
   `isItemUnread()` with `LastActivityAtUnixSec`. This is correct and should NOT
   change -- it answers "is there new activity?" independently of sort order.
   Added explicit verification task to Phase 1.

2. **Added `isItemFresh()` verification task.** The freshness filter also uses
   `LastActivityAtUnixSec` to decide whether an item should appear at all.
   Conversation activity should still keep items fresh (prevent expiry), even
   though it no longer affects sort position. Added explicit verification task.

3. **Added `npm run build` tasks.** Per CLAUDE.md, Go changes must pass lint
   (`golangci-lint`) and compilation. Added to both phases.

4. **Added `LastActivityAtUnixSec` client usage detail.** The client passes
   this value through to content views for display. Confirmed it should
   continue to be computed and sent.

5. **No client-side changes required.** The client does not re-sort feed items
   and does not use `isUnread` or `isActionRequired` for visual sorting --
   only the feed viewmodel uses `isUnread` to optimistically clear the badge
   after `MarkFeedItemsViewed`. No architectural violations found.

6. **No i18n impact.** No user-facing strings are added or changed.

7. **No caching impact.** Feed data is not cached in the client repository
   layer; it's fetched fresh on each load via the feed viewmodel.

## Related Issues

- None identified.
