# Issue #967: Experiences catch-all for closed beta

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/967
**Filed:** 2026-03-31 | **Last activity:** 2026-03-31
**Labels:** P1 (High: significant pain for key personas) | **Assignees:** tescobar
**Context docs:** docs/client/content.md, docs/client/workflow.md

## Problem Statement

A collection of 10 experience (event) bugs found during closed beta testing. The
issues span notifications, UI, workflow hangs, and the wrap-up flow. Several are
blocking the organizer workflow — confirm time hangs make events unmanageable,
and wrap-up issues prevent clean event completion.

## Context from Comments

No comments on this issue. All requirements come from the issue description. The
bug list reads like a tester walkthrough of the full organizer lifecycle: create
event → receive notification → manage → confirm time → wrap up → view result.

## Bug Inventory

| # | Bug | Category | Severity |
|---|-----|----------|----------|
| 1 | Creator gets notification for own event creation | Server / notifications | Low |
| 2 | RSVP notification links to chat that has no non-system messages | Server / notifications | Medium |
| 3 | Website shown under title (already on Details tab) | Client / UI | Low |
| 4 | Manage → Confirm Time hangs | Server / client | **High** |
| 5 | Any time change leads to a hang | Server / client | **High** |
| 6 | Wrap up shows summary entry (should be removed) | Client / UI | Low |
| 7 | Can't select "did not attend" in wrap up | Client / UI | Medium |
| 8 | Everyone should default to "attended" in wrap up | Client / logic | Medium |
| 9 | Wrap up should show predicted impact | Client / UI | Medium |
| 10 | Detail page still says "Manage" after completing | Client / cache | Medium |

## Codebase Analysis

### Bug 1: Creator receives own creation notification

**Location:** `server/services/community/notifications.go:148-180`

`getNotificationRecipients()` returns all community members for
`EXPERIENCE_CREATED` events. There is an actor-skip check at line 177-179, but
the creator is still included in the results from `getCommunityMemberIDs()`.
The actor exclusion logic may not be firing correctly for this event type.

**Fix:** Verify and ensure the actor exclusion at line 177-179 correctly filters
out the creator for `EXPERIENCE_CREATED` events. If the check is present but not
working, the bug is in how `event.ActorId` is populated during creation.

### Bug 2: RSVP notification deep-links to empty chat

**Location:** `server/services/community/notifications.go:291-388`

`getConversationID()` (lines 377-386) can return an empty string if no
conversation exists yet. The RSVP notification links the organizer to a chat
that has no user messages — only system messages.

**Fix:** Change the RSVP notification deep link to open the experience content
view with the Chat tab active (tab index 2). This gives the organizer the
conversation context while being inside the full experience view, rather than
linking to a standalone chat that may have no user messages.

### Bug 3: Website shown under title

**Location:** `app/lib/presentation/screens/experience/experience_content_view.dart:411-414`

The `sourceUrl` chip is rendered in the type tab (pane 0) under the title AND
on the details tab. The issue asks to remove the duplicate from the title area.

**Fix:** Remove lines 411-414 that display the URL chip in the type tab header.

### Bugs 4-5: Confirm time / time changes hang

**Root cause identified — both server and client contribute.**

**Server side:** `server/services/experience/time_proposals.go:200-322`

`ConfirmTime()` has two N+1 query loops:
1. **Lines 273-289:** Fetches ALL proposals, then updates each non-confirmed one
   individually in a loop.
2. **Lines 294-319:** Fetches ALL community experiences, then calls
   `InsertSystemMessage()` for each conversation sequentially. If the chat
   service is slow, the entire request blocks.

**Client side:** `app/lib/data/repositories/experience_repository.dart:666-678`

`confirmTime()` does NOT call `_onContentInvalidated?.call()` after the RPC
completes. Compare to `rsvp()` (line 285) which does. This means:
- The `ExperienceContentView` listener never fires `refreshExperienceDetails()`
- UI state remains stale even after the RPC eventually completes
- Subsequent time changes also appear hung because experience data is out of sync

The hang the user sees is likely the server taking too long on system message
writes, combined with the client never refreshing state even when the RPC
does complete.

**Fix (server):** Add structured logging with timing to `ConfirmTime()`. Batch
the proposal updates and system message inserts to eliminate N+1 queries.

**Fix (client):** Add `_onContentInvalidated?.call()` to
`ExperienceRepository.confirmTime()` so the content view refreshes after the
RPC completes.

> **Audit note (SQL efficiency, CLAUDE.md):** Two confirmed N+1 patterns in
> `ConfirmTime()`. Use batch UPDATE for proposals and batch INSERT for system
> messages. Add `storage.AssertMaxQueries(t, ctx, max, fn)` in tests to
> prevent regression.

### Bug 6: Remove summary entry from wrap up

**Location:** `app/lib/presentation/screens/experience/mark_completed_modal.dart:157-174`

The wrap-up phase shows a "SUMMARY" section with a TextField. This should be
removed per the issue.

**Fix:** Remove the summary section label and TextField from the wrap-up phase
of the modal.

### Bug 7: Can't select "did not attend"

**Location:** `app/lib/presentation/screens/experience/mark_completed_modal.dart:227-239`

The attendance UI shows check/X buttons, but the toggle logic doesn't correctly
handle marking someone as "did not attend."

**Fix:** Debug the toggle logic to ensure tapping the X button correctly sets
attendance to false and persists the state. The toggle callback in the widget
should call the ViewModel method (e.g., `setAttendance(userId, false)`) — the
widget must not mutate attendance state directly.

> **Audit note (architecture.md compliance):** Business logic (attendance state
> mutation) belongs in the ViewModel, not in the widget. Verify the toggle
> callback flows through the ViewModel notifier, not `setState()`.

### Bug 8: Default everyone to "attended"

**Location:** `app/lib/presentation/viewmodels/event_modal_view_model.dart:456-463`

The codebase research indicates this is already implemented — when transitioning
to wrap-up, all YES RSVPs are automatically marked as attended. If the bug is
still visible, the default-marking logic may not be firing (e.g., race condition
with state initialization).

**Fix:** Verify the default-attendance logic fires correctly during the
wrap-up transition. Check if there's a timing issue where the UI renders before
the defaults are applied.

### Bug 9: Show predicted impact in wrap up

**Location:** `app/lib/presentation/screens/experience/mark_completed_modal.dart:306-390`

The impact card is already shown in the "Complete" phase (phase 1) of the modal.
If the user isn't seeing it, the wrap-up flow may not be reaching that phase, or
the impact data may not be loading.

**Fix:** Verify the impact card renders in the correct phase of the modal flow.
Check if the predicted impact data is available before completion.

### Bug 10: "Manage" label after completing (cache invalidation)

**Location:** `app/lib/presentation/viewmodels/conversation_view_model.dart:2281-2288`

`preActionLabel()` returns `'Manage'` for organizers regardless of experience
state. After `completeExperience()` succeeds, the cached experience isn't
refreshed in the conversation view model.

**Fix:** After experience completion, invalidate through the **repository layer**
(e.g., `ExperienceRepository.invalidate(experienceId)`), not by manually
clearing ViewModel state. Use the existing event-driven cache invalidation
pattern (`contentCacheInvalidationProvider` or similar) so that all screens
observing experience state refresh automatically — including the conversation
view. The button status logic in `preActionLabel()` should also check the
experience state and return `completed` status when the experience is completed.

> **Audit note (caching.md compliance):** Repositories own caching — ViewModels
> must not manage cache maps or invalidation directly. Use repository
> `invalidate*()` methods and cross-provider invalidation signals.

## Potential Solutions

### Option A: Fix all bugs in a single PR

- **Approach:** Address all 10 bugs in one branch, organized as separate commits.
- **Pros:** Single review cycle. Changes are related and can be tested together.
- **Cons:** Large PR. If one fix regresses, harder to bisect.
- **Effort:** Medium-large.

### Option B: Split into server vs. client PRs

- **Approach:** One PR for server fixes (bugs 1, 2, 4-5) and one for client
  fixes (bugs 3, 6-10).
- **Pros:** Server and client can be reviewed/deployed independently. Smaller
  PRs are easier to review.
- **Cons:** Two review cycles. Some bugs span both (4-5 need investigation).
- **Effort:** Medium.

### Option C: Prioritize by severity, ship incrementally

- **Approach:** Phase 1: time hang bugs (4-5) as urgent fix. Phase 2: wrap-up
  bugs (6-9). Phase 3: notification and UI polish (1-3, 10).
- **Pros:** Gets the blocking bugs fixed first. Each phase is small and focused.
- **Cons:** Three review cycles. More branch management.
- **Effort:** Medium (spread over time).

## Recommendation

**Option C** — fix incrementally by severity. The time hang (bugs 4-5) is the
most critical since it blocks organizers from managing events at all. The wrap-up
bugs (6-9) block clean event completion. The notification and UI bugs (1-3, 10)
are polish. Shipping in this order unblocks beta testers fastest.

## Open Questions (Resolved)

- **Is the time hang server-side or client-side?** — **Decision:** Both.
  Server has N+1 query loops in `ConfirmTime()` that slow the RPC. Client's
  `ExperienceRepository.confirmTime()` is missing `_onContentInvalidated?.call()`
  so the UI never refreshes even when the RPC completes. Fix both.
- **Should "did not attend" be a toggle or a separate state?** — **Decision:**
  Boolean toggle (attended = true/false). The bug is in the UI toggle logic,
  not the data model.
- **Where should RSVP notifications link?** — **Decision:** Link to the
  experience content view opened to the **Chat tab**. This gives the organizer
  the conversation context while still being inside the experience view.
- **Is the predicted impact card already showing?** — **Decision:** The impact
  card exists in the codebase. Find the existing implementation and reuse it in
  the wrap-up flow rather than building a new one.

## Cross-Cutting Audit Notes

> **i18n (i18n.md):** Any new user-visible strings added during these fixes
> (toast messages, error text, button labels) must use `context.l10n.*` with
> entries in both `app_en.arb` and `app_es.arb`. ViewModels must not resolve
> strings — expose enums/error codes and let widgets resolve to localized text.

> **Server logging (CLAUDE.md):** All new server-side logging must use
> `logging.LoggerWithContext(ctx)` with structured fields. Never use
> `log.Printf` or `fmt.Printf`. Mask any user emails or tokens.

> **Server service independence (server/architecture.md):** Bug 1 and 2 fixes
> are within the community service's notification logic — no cross-service
> calls needed. This is compliant.

## Implementation Plan

### Phase 1: Fix time hang (bugs 4-5)

**Goal:** Unblock organizers from confirming and changing event times.

**Tasks:**
- [ ] **Server:** Add structured logging with timing to `ConfirmTime()` in `server/services/experience/time_proposals.go` (before/after each DB operation loop)
- [ ] **Server:** Batch the proposal updates at lines 273-289 (replace loop of individual UPDATEs with a single batch operation)
- [ ] **Server:** Batch the system message inserts at lines 294-319 (replace sequential `InsertSystemMessage` loop)
- [ ] **Server:** Add `storage.AssertMaxQueries` test covering `ConfirmTime()` to prevent N+1 regression
- [ ] **Client:** Add `_onContentInvalidated?.call()` to `ExperienceRepository.confirmTime()` (line ~678) so content views refresh after the RPC completes
- [ ] **Client:** Verify `time_modal_view_model.dart:lockTime()` properly surfaces errors to the UI (it currently has `rethrow` — confirm the modal catches it)
- [ ] Test confirm time and time changes with active time polls — verify UI updates immediately

**Validates:** Organizers can confirm and change event times without the UI hanging, and the content view reflects the new time immediately.

### Phase 2: Fix wrap-up flow (bugs 6-9)

**Goal:** Make the event completion flow work correctly end-to-end.

**Tasks:**
- [ ] Remove the summary section from `mark_completed_modal.dart` (bug 6)
- [ ] Fix the "did not attend" toggle logic in the attendance UI (bug 7)
- [ ] Verify default-attendance logic fires correctly on wrap-up entry (bug 8)
- [ ] Verify predicted impact card renders in the correct modal phase (bug 9)
- [ ] Test the full wrap-up flow: open modal → see defaults → toggle attendance → see impact → complete

**Validates:** Organizers can complete events cleanly with correct attendance tracking and impact preview.

### Phase 3: Notifications, UI polish, cache (bugs 1-3, 10)

**Goal:** Clean up remaining beta feedback items.

**Tasks:**
- [ ] Filter out creator from `EXPERIENCE_CREATED` notification recipients in `notifications.go` (bug 1)
- [ ] Change RSVP notification deep link target from chat to experience content view (bug 2)
- [ ] Remove `sourceUrl` chip from type tab header in `experience_content_view.dart` (bug 3)
- [ ] Fix cache invalidation after experience completion: use `ExperienceRepository.invalidate()` + cross-provider invalidation signal so button updates from "Manage" to completed state (bug 10)
- [ ] Fix `preActionLabel()` in conversation view model to check experience state before returning 'Manage'
- [ ] Test notification behavior for event creation and RSVPs
- [ ] Test that the manage button updates after completing an event without manual refresh
- [ ] Ensure any new user-visible strings use `context.l10n.*` (i18n compliance)

**Validates:** Notification behavior is correct, UI shows no duplicate info, and state updates propagate without manual refresh.

## Audit Summary

This plan was audited against:
- `docs/client/architecture.md` (MVVM, Riverpod, ViewModel boundaries)
- `docs/client/caching.md` (repository-owned caching, invalidation patterns)
- `docs/client/i18n.md` (localized strings, ViewModel string resolution)
- `docs/server/architecture.md` (service independence, shared libraries)

**Changes made during audit:**
1. **Bug 10 fix:** Rewrote to use repository-layer invalidation and cross-provider signals instead of direct ViewModel cache manipulation (caching.md)
2. **Bug 7 fix:** Added note that attendance toggle must flow through ViewModel notifier, not widget `setState()` (architecture.md)
3. **Bugs 4-5 fix:** Added N+1 query check and `AssertMaxQueries` test requirement (SQL efficiency / CLAUDE.md)
4. **Phase 1 tasks:** Added ViewModel loading state requirement for client-side fix
5. **Phase 3 tasks:** Added `preActionLabel()` fix task and i18n compliance task
6. **Cross-cutting notes:** Added section for i18n, structured logging, and service independence compliance

**No architectural violations found in the proposed approach.** The plan correctly keeps server fixes within the community service boundary, and client fixes within the ViewModel/widget layer separation.

## Related Issues

- None referenced in the issue or comments.
