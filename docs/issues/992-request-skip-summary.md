# Issue #992: Requests shouldn't prompt for a summary on conclusion

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/992
**Filed:** 2026-04-01 | **Last activity:** 2026-04-02
**Labels:** none | **Assignees:** maintainer
**Context docs:** none

## Problem Statement

When the owner taps "Mark Fulfilled" on a request, a two-phase modal appears:
Phase 1 asks them to review/edit an AI-generated resolution summary, then
Phase 2 shows impact metrics. The issue says this should skip the summary
prompt entirely and go straight to showing impact after marking fulfilled.

This mirrors the recent experience completion change (commits `30028091`,
`ca22d376`) where the mandatory summary was removed and the modal just
completes and closes.

## Context from Comments

No comments. The requirement is clear: no summary prompt, just complete the
request and show impact.

## Root Cause

`RequestFulfillModal` (`app/lib/presentation/screens/request/request_fulfill_modal.dart`)
implements a two-phase in-modal flow:

1. **Phase 1 (lines 66-82, 151-223):** Calls `markFulfilled()` which generates
   an AI summary via `generateResolutionSummary()`, then shows a text field for
   the user to edit it. User must tap "Mark Fulfilled" to proceed.

2. **Phase 2 (lines 84-97, 225-264):** Calls `saveSummaryAndMarkFulfilled()`
   which sends the RPC, then shows impact metrics inline. The bottom bar
   disappears and the user must swipe to dismiss.

The viewmodel splits this into two stages: `markFulfilled()` (Stage 1: generate
summary) and `saveSummaryAndMarkFulfilled()` (Stage 2: RPC + post summary to
chat). The summary is `optional` in the proto, so it can be omitted.

## Potential Solutions

### Option A: Collapse to single step — mark fulfilled, show impact, close

- **Approach:** Replace the two-phase modal with a single action: call
  `saveSummaryAndMarkFulfilled('')` immediately (empty summary), show a brief
  loading state, then navigate to `ItemMetricsScreen` and close the modal.
  This matches how the conversation screen already handles fulfillment (no
  modal at all) and how experience completion works (just close).
- **Pros:** Simplest fix. Removes the entire summary editing UI. Navigates
  to the dedicated impact screen which is richer than the in-modal metrics.
  Consistent with experience completion pattern.
- **Cons:** Loses the ability to enter a resolution summary. The AI summary
  generation and chat posting are skipped entirely.
- **Effort:** Small.

### Option B: Keep the modal but auto-submit with empty summary

- **Approach:** On `initState`, skip the AI summary generation and immediately
  call `saveSummaryAndMarkFulfilled('')`. Show a loading spinner, then display
  the impact metrics in Phase 2 as today.
- **Pros:** Keeps the in-modal impact display.
- **Cons:** Still shows a modal that the user didn't ask for. The impact
  display is a stripped-down version of `ItemMetricsScreen`.
- **Effort:** Small.

## Recommendation

**Option A.** The issue says "jump immediately to the impact view" — this
means navigate to `ItemMetricsScreen`, not show impact inline in a modal.
This also aligns with the experience completion pattern where the modal
simply closes after success. The summary feature can be revisited later if
needed.

## Open Questions

- **Should the summary still be posted to chat?** — Recommendation: No.
  With no summary prompt, there's nothing to post. The server already sends
  a FULFILLED system message to the conversation. If AI summaries are wanted
  later, that's a separate feature.
- **Should `markFulfilled()` (Stage 1) be removed from the viewmodel?** —
  Recommendation: Yes, it becomes dead code. Consolidate into a single
  `markFulfilled()` that calls the RPC directly without summary generation.

## Implementation Plan

### Phase 1: Skip summary, navigate to impact

**Goal:** "Mark Fulfilled" completes the request immediately and opens the
impact screen.

**Tasks:**
- [ ] In `request_content_view.dart` `_handleMarkFulfilled()`: replace the
      `RequestFulfillModal.show()` call with a direct call to the viewmodel's
      `saveSummaryAndMarkFulfilled('')`, then navigate to `ItemMetricsScreen`
      on success using `NavigationHelpers.pushWithSlide` (matching the existing
      "View Impact" menu item at line 501)
- [ ] In the viewmodel: remove `markFulfilled()` (Stage 1 AI summary
      generation) — it becomes dead code. Keep
      `saveSummaryAndMarkFulfilled()` as-is since it handles the RPC, state
      update, and cache invalidation correctly. Do NOT remove
      `isLoadingSummary`, `isEditingSummary`, `resolutionSummary`, or
      `completionSavings` state fields — they are still used by
      `request_management_card.dart` for the conversation-screen chat pill
- [ ] Remove the summary chat message posting from
      `saveSummaryAndMarkFulfilled()` (the server sends a FULFILLED system
      message already)
- [ ] Delete `request_fulfill_modal.dart` — it's no longer used
- [ ] Verify conversation screen's `_handleMarkRequestFulfilled` still works
      (it calls `conversationViewModel.markRequestFulfilled()` which is a
      separate code path that doesn't use the modal — no changes needed)

**Validates:** Owner taps "Mark Fulfilled" → request completes → impact
screen opens. No summary modal appears.

---

## Audit

**Audited:** 2026-04-02

**Changes made:**

1. **State field retention (architecture.md):** Original plan proposed removing
   `isLoadingSummary`, `isEditingSummary`, `resolutionSummary`, `completionSavings`
   from `RequestState` "if no longer referenced." These ARE still referenced by
   `request_management_card.dart` (the conversation-screen chat pill uses them
   for inline summary display and impact metrics). Updated to keep them.

2. **Navigation pattern (architecture.md):** Original plan said "navigate to
   `ItemMetricsScreen`" without specifying how. Updated to require
   `NavigationHelpers.pushWithSlide`, matching the existing "View Impact" menu
   item in the same file (line 501) and the architecture doc's rule that utility
   screens use `pushWithSlide`.

3. **Scope clarification:** Clarified that `markFulfilled()` (Stage 1) is the
   only viewmodel method to remove; `saveSummaryAndMarkFulfilled()` (Stage 2)
   stays since it handles the RPC correctly.

**No issues found with:**
- Caching (fulfillment invalidation already handled by repository)
- Server/client boundary (no server changes; `resolution_summary` is already
  optional in the proto)
- i18n (no new user-visible strings)

## Related Issues

- #968 — PR that attempted eager state updates via system chat messages
  (related: the summary was posted to chat as part of fulfillment)
