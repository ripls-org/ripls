# Issue #987: Giveaway manage pill for owner should switch to 'ready for pickup' after selecting recipient

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/987
**Filed:** 2026-04-01 | **Last activity:** 2026-04-14
**Labels:** none | **Assignees:** briangbrown
**Context docs:** none

## Problem Statement

When a giveaway owner selects a recipient, the status pill on the gear detail
screen stays stuck on "X people interested" instead of updating to reflect the
new state (e.g., "Ready for pickup"). The owner has completed the selection
step but the UI doesn't signal what to do next.

Two distinct issues:

1. **Stale pill text**: After `selectRecipient()`, the gear detail view's
   `transferContext` is not refreshed, so the pill keeps showing the old
   pending-requests count.
2. **Missing state label**: Even after a refresh, the pill logic in
   `gear_content_view.dart` only sets a label for the "interested" state —
   there's no label for the `RECIPIENT_SELECTED` phase.

## Context from Comments

No comments on this issue. The description is clear: the pill should update
after recipient selection.

## Potential Solutions

### Option A: Fix cache invalidation + add recipient-selected label

- **Approach:** After `selectRecipient()` in the transfer repository, trigger
  `transferCacheInvalidationProvider` so the gear view refreshes. Add a pill
  label for the `GIVEAWAY_PHASE_RECIPIENT_SELECTED` state showing the
  recipient's name (e.g., "Ready for {name} to pick up").
- **Pros:** Fixes both the staleness bug and the missing label. Follows
  existing cache invalidation patterns. Small, focused change.
- **Cons:** None significant.
- **Effort:** Small.

### Option B: Add a real-time event for recipient selection

- **Approach:** In addition to Option A, emit a community event when a
  recipient is selected, so the pill updates via the event stream without
  needing a manual refresh.
- **Pros:** Real-time update, no polling needed.
- **Cons:** Over-engineered for this case — the owner just performed the
  action, so a local refresh is sufficient. The event stream is for notifying
  *other* users.
- **Effort:** Medium.

## Recommendation

**Option A.** The owner just tapped "Select Recipient" — they're already on
the screen. A local cache invalidation + refresh is the right pattern. No
need for real-time events since the actor and the viewer are the same person.

## Open Questions

- **What should the pill say?** — Recommendation: "Ready for {recipient name}
  to pick up" (truncate if too long). This matches the giveaway flow where
  the next action is a physical handoff.
- **Should there be a pill for the completed state?** — Recommendation: No,
  the completed state already shows a different UI (impact metrics). The pill
  is only needed for actionable intermediate states.

## Implementation Plan

### Phase 1: Fix cache invalidation and add label

**Goal:** After selecting a recipient, the pill updates immediately to show
the next action.

**Tasks:**
- [x] In `giveaway_giver_view_model.dart`, after `selectRecipient()`
  completes, call
  `ref.read(transferCacheInvalidationProvider.notifier).notify()` to trigger
  the gear view refresh (follows the same pattern as `arrange_pickup_modal`
  and `conversation_view_model`)
- [x] In `gear_content_view.dart`, update the `ownerPostLabel` logic for
  giveaways to check the giveaway phase: when
  `GIVEAWAY_PHASE_RECIPIENT_SELECTED`, show "Ready for {name} to pick up"
  using `transferContext?.selectedRecipient?.borrower?.name`
- [x] Add i18n string `gearPillReadyForPickup(String name)` to both
  `app_en.arb` and `app_es.arb`
- [ ] Verify the pill clears or updates when the giveaway is completed or
  cancelled

**Validates:** Owner selects recipient → pill immediately shows "Ready for
{name} to pick up" → owner completes giveaway → pill disappears.

## Audit Notes

1. **Cache invalidation location:** The plan proposes triggering
   `transferCacheInvalidationProvider.notify()` from the transfer repository
   after `selectRecipient()`. Currently, this provider is notified from
   presentation-layer code (event router, ViewModels, widgets). Notifying
   from the repository is actually more correct per caching docs —
   repositories own cache invalidation. However, since
   `transferCacheInvalidationProvider` is a Riverpod `NotifierProvider` in
   `providers.dart`, the repository would need a callback to trigger it (the
   repository doesn't hold a `ref`). Follow the existing pattern: the
   `GiveawayGiverNotifier` ViewModel should call
   `ref.read(transferCacheInvalidationProvider.notifier).notify()` after
   `selectRecipient()` completes — this matches how `arrange_pickup_modal`
   and `conversation_view_model` already trigger it.

2. **Pill label data source:** The `ownerPostLabel` is computed from
   `transferContext?.pendingRequests.length` (line 513-514 in
   `gear_content_view.dart`). After recipient selection, the active
   transfer's recipient name is available from
   `transferContext?.activeTransfer`. The new label should read this field.
   No additional API call needed — the data is already in the transfer
   context after refresh.

3. **i18n compliance:** The plan correctly includes ARB entries for both
   locales. Use a parameterized string like
   `gearPillReadyForPickup(name)` to interpolate the recipient's first name.

4. **No server changes needed.** The giveaway phase is already computed
   server-side and returned in `GetGearTransferContext`. The client just
   needs to read it and display the right label.

## Related Issues

- #1099 — Unrelated but touched the same codebase area during recent work
