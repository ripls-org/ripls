# Issue #984: Organizer can't conclude / finish experiences

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/984
**Filed:** 2026-04-01 | **Last activity:** 2026-04-01
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When an experience organizer taps "Mark Completed" in the management dropdown
and interacts with the completion modal, nothing happens — the experience is
never completed. The organizer sees the modal briefly, but the completion API
call never fires.

## Context from Comments

No comments on this issue. The description is concise with a screenshot showing
the management dropdown with "Mark Completed" as a checklist item.

## Root Cause Analysis

The bug is in `mark_completed_modal.dart:512-515`. When the user taps the
primary button in phase 0 (Wrap Up), the handler **pops the modal before
awaiting `wrapUp()`**:

```dart
Future<void> _handlePrimary() async {
  if (_phase == 0) {
    Navigator.of(context).pop();   // closes modal immediately
    ref.read(eventModalProvider(widget.experienceId).notifier).wrapUp(); // NOT awaited
  } else {
    await ref
        .read(eventModalProvider(widget.experienceId).notifier)
        .shareWithGroup();
    if (!mounted) return;
    Navigator.of(context).pop();
  }
}
```

This triggers a chain of failures:

1. `Navigator.of(context).pop()` removes the modal widget from the tree
2. The modal was the only listener on the `autoDispose` family provider
   `eventModalProvider(experienceId)`, so the notifier is **auto-disposed**
3. Inside `wrapUp()`, after `await markInProcess()` yields to the event loop,
   `ref.mounted` is `false` → early return at line 595
4. `completeExperience()` is **never called**

The `markInProcess()` API call may still succeed (setting state to IN_PROCESS),
leaving the experience stuck in an intermediate state with no way to advance.

**Compare with phase 1** (lines 517-522), which correctly awaits the async work
before popping:

```dart
await ref.read(...).shareWithGroup();
if (!mounted) return;
Navigator.of(context).pop();
```

## Potential Solutions

### Option A: Await wrapUp, advance phase locally

Match the phase 1 pattern: await `wrapUp()`, check mounted, then advance the
local `_phase` to show the impact/share view. Only pop after phase 1 completes.

- **Approach:** Fix `_handlePrimary` to await `wrapUp()`, then `setState(() =>
  _phase = 1)` on success. The modal stays open during the operation, showing
  the loading spinner (already implemented via `state.isLoading`). On error,
  the modal stays on phase 0 and the error message displays.
- **Pros:** Minimal change (3-5 lines). Matches existing phase 1 pattern.
  Loading/error states already wired up.
- **Cons:** None significant.
- **Effort:** Small.

### Option B: Keep modal alive with keepAlive on the provider

Use `ref.keepAlive()` to prevent auto-disposal while `wrapUp()` is in flight.
Still pop the modal immediately.

- **Approach:** Add `keepAlive` link management to the notifier, release after
  async work completes.
- **Pros:** Modal closes immediately (snappier UX).
- **Cons:** More complex. Fire-and-forget pattern — user gets no feedback on
  success/failure. If `wrapUp()` fails, user doesn't know. Also, the impact
  sharing phase (phase 1) would never display, breaking the intended two-phase
  UX.
- **Effort:** Medium.

### Option C: Move completion logic out of the viewmodel

Handle `markInProcess` + `completeExperience` + `recordAttendance` in a
standalone service/use-case class that doesn't depend on provider lifecycle.

- **Approach:** Create an `ExperienceCompletionUseCase` that wraps the three
  API calls. Modal triggers it and observes results.
- **Pros:** Decouples lifecycle from UI.
- **Cons:** Over-engineered for the bug at hand. Adds a new abstraction for a
  one-shot operation. Doesn't solve the missing phase 1 UX.
- **Effort:** Large.

## Recommendation

**Option A.** The fix is a 3-line change that matches the existing pattern used
by phase 1. The loading spinner and error display are already wired up. The
two-phase UX (wrap up → share impact) works as designed once the modal stays
open through the async operation.

## Open Questions

- **Should `markInProcess` be idempotent?** If a user triggers "Mark Completed"
  on an experience that's already IN_PROCESS (from a previous failed attempt),
  the server rejects it with `CodeFailedPrecondition`. —
  **Recommendation:** Yes, make `MarkExperienceInProcess` a no-op (return
  success) when the experience is already IN_PROCESS. This prevents stuck
  experiences from blocking completion. This is a separate improvement and not
  required for the immediate fix.

## Implementation Plan

### Phase 1: Fix the modal handler and supporting bugs
**Goal:** Make "Mark Completed" actually complete the experience.
**Tasks:**
- [ ] Fix `_handlePrimary` in `mark_completed_modal.dart:512-523`: await
  `wrapUp()`, check `mounted`, advance `_phase` to 1 on success (no error in
  state)
- [ ] **[AUDIT]** Fix `wrapUp()` success path in
  `event_modal_view_model.dart:631-634`: add `isLoading: false` to the
  success `safeUpdateState`. Currently `isLoading` is set to `true` at
  line 589 but only reset in the `catch` block (line 641). Without this fix,
  the modal will show a spinner forever after successful completion.
- [ ] **[AUDIT]** Add error display to `mark_completed_modal.dart`. The modal
  currently has **no reference to `state.errorMessage`** — if `wrapUp()` fails,
  the user sees a spinner revert to the form but no explanation. Add a
  `Text(state.errorMessage)` widget (styled as error) in `_buildWrapUpPhase`
  or `_buildBottomBar`.
- [ ] Verify the loading spinner shows during the operation (already
  implemented — `state.isLoading` drives the spinner at line 78)
**Validates:** Tapping "Complete" in the wrap-up phase triggers
`markInProcess` → `completeExperience` → `recordAttendance`, transitions
to the impact sharing phase, and displays errors if any step fails.

### Phase 2: Idempotent markInProcess (optional improvement)
**Goal:** Prevent stuck experiences from blocking completion.
**Tasks:**
- [ ] In `server/services/experience/lifecycle.go:51-55`, allow
  `MarkExperienceInProcess` when state is already `IN_PROCESS` (return
  current experience, skip update)
- [ ] Add test case for idempotent behavior
**Validates:** Experiences stuck in IN_PROCESS (from prior bug) can still be
completed via the UI.

## Related Issues

None identified.
