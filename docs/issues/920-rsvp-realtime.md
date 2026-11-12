# Issue #920: [Bug Report] RSVPs need to be more real time

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/920
**Filed:** 2026-03-27 | **Last activity:** 2026-03-27
**Labels:** bug, user-feedback | **Assignees:** unassigned
**Context docs:** docs/client/chat.md, docs/push_notifications.md

## Problem Statement

When a participant RSVPs to an experience, the experience host sees the RSVP system message appear in the chat thread immediately (via the real-time streaming connection). However, the **Details tab** on the experience screen — which shows the attendee list — does not update to include the new RSVP. The host must close and reopen the experience view, or navigate away and back, to see the updated list.

The bug is a cache invalidation gap: when an RSVP arrives via the chat message stream, `ConversationViewModel` calls `fetchExperienceDetails()` to refresh in-chat state (RSVP status badges on message bubbles), but it does **not** invalidate the `ExperienceRepository` cache, and does **not** notify the `ExperienceViewModel` (which powers the Details tab). Both the chat ViewModel and the experience ViewModel read from the same repository cache, so without invalidation the Details tab stays stale until the TTL expires.

## Context from Comments

No comments on this issue. It was filed by the maintainer directly from the app's feedback flow (device: iPhone, app version 0.8.0, environment: DEV). The screenshots are not accessible in this environment but the description is unambiguous.

## Potential Solutions

### Option A: Invalidate experience cache + notify ExperienceViewModel on incoming RSVP system messages

When `ConversationViewModel._handleNewMessage()` processes an RSVP system message, it currently calls `fetchExperienceDetails()` (fire-and-forget). The fix is to:
1. Await the `ExperienceRepository` cache invalidation for the experience ID before fetching (chained via `.then()`).
2. Also call `_invalidateExperienceViewModelIfActive()` so `ExperienceNotifier.refresh()` runs with a cold cache and the Details tab re-renders.

An exact analogue already exists in this file: `_invalidateAndRefreshRequest()` (around line 951) chains `invalidate().then(() => fetchRequestDetails())` for the same reason. The RSVP path should follow that pattern with a new `_invalidateAndRefreshExperience()` helper.

- **Approach:** Add `_invalidateAndRefreshExperience()` helper to `conversation_view_model.dart`; replace the bare `fetchExperienceDetails()` call in the RSVP/lifecycle handler with it.
- **Pros:** Minimal, targeted fix; exactly mirrors the existing `_invalidateAndRefreshRequest` pattern; guarantees invalidation completes before the subsequent fetch; no new APIs or RPCs needed.
- **Cons:** Slightly more lines than a direct call; the invalidation and both fetches are still fire-and-forget from the stream handler's perspective (which is intentional — stream handlers cannot await).
- **Effort:** Small.

### Option B: Make `fetchExperienceDetails()` always bypass the cache

Modify the repository call inside `fetchExperienceDetails()` to always invalidate before fetching when called from a stream-triggered context (via a `forceRefresh` flag or a separate method).

- **Approach:** Add `forceRefresh: bool = false` parameter to `fetchExperienceDetails()`; when true, call `experienceRepository.invalidate(experienceId)` before `getExperienceDetails()`. Pass `forceRefresh: true` from the RSVP system message handler.
- **Pros:** Makes the intent explicit; `forceRefresh` can be reused for other stream-triggered refreshes; eliminates the async race in Option A.
- **Cons:** Slightly more code; `fetchExperienceDetails()` already has a comment saying "subsequent calls always fetch from repository for fresh data" that is misleading — this clarifies that but requires updating the comment too.
- **Effort:** Small.

### Option C: Push-notification-triggered refresh

The server already sends `EXPERIENCE_RSVP_YES`/`EXPERIENCE_RSVP_MAYBE` push notifications to the experience host. The client could handle these notifications (when the app is foregrounded) to invalidate and reload the experience.

- **Approach:** In the FCM message handler, when a notification with an experience RSVP payload arrives, call `experienceRepository.invalidate(experienceId)` and trigger `ExperienceNotifier.refresh()`.
- **Pros:** Works even if the user is not currently in the chat thread.
- **Cons:** Per `docs/client/chat.md` Pattern 4 ("Smart Notification Routing"), push notifications are **skipped** for users with an active chat stream — so this would not fire when the host is in the chat, which is exactly the scenario reported. This approach alone doesn't solve the bug; it could be layered on top of Option A/B as a defense-in-depth enhancement.
- **Effort:** Medium (requires FCM handler changes), and doesn't address the core case.

## Recommendation

**Option A** is the right fix, implemented via a chained `.then()` pattern like `_invalidateAndRefreshRequest()`. The key architectural point: `StashCacheManager.invalidate()` is **async** (Stash is async-only per `docs/client/caching.md`), so the implementation must chain the fetch after the invalidation completes rather than calling both independently. Fire-and-forgetiing the invalidate and then calling the fetch in parallel would introduce a real — if low-probability — race condition, since the fetch could hit the cache before the remove microtask runs.

After fixing the stream-triggered path, it's worth noting that `ExperienceNotifier.refresh()` (called by `_invalidateExperienceViewModelIfActive()`) also reads from the repository without invalidating first. This means the ExperienceViewModel refresh triggered by local mutations (RSVP submit, time lock, etc.) also benefits from the cache having been already invalidated by the caller — the existing mutation paths in `ConversationViewModel` do call `experienceRepository.invalidate()` before `_invalidateExperienceViewModelIfActive()`, so those paths are already correct. Only the incoming stream path is missing this step.

## Open Questions

- **Should lifecycle events (CANCELLED, STARTED, COMPLETED) also get the invalidation treatment?** — **Decided: Yes.** Apply to all actions in the existing `if` block (lines 417–426). All of these can change what the Details tab shows (participant list, state banner, completion metrics).
- **Should the fix also cover the `DETAIL_CHANGED` and `TIME_PROPOSED` actions?** — **Decided: Yes.** The Details tab renders event time and location, so remote time proposals and detail edits must also invalidate and refresh.

## Implementation Plan

### Phase 1: Fix cache invalidation in the RSVP/lifecycle stream handler

**Goal:** Details tab shows up-to-date attendee list in real time whenever any RSVP or lifecycle system message arrives.

**Tasks:**

- [ ] In `conversation_view_model.dart`, add a new `_invalidateAndRefreshExperience()` private method modelled on the existing `_invalidateAndRefreshRequest()` method. The method should:
  - Resolve the experience ID from `state.inboxItem.conversation?.topic.experienceId` (or `state.cachedExperience?.experience.id` as fallback).
  - Return early if the ID is empty.
  - Call `experienceRepository.invalidate(expId)`, chain `.then((_) { if (ref.mounted) { fetchExperienceDetails(); _invalidateExperienceViewModelIfActive(); } })`, and handle errors with `.catchError()` logging.
  - For example:
    ```dart
    void _invalidateAndRefreshExperience() {
      final expId = state.inboxItem.conversation?.topic.experienceId.isNotEmpty == true
          ? state.inboxItem.conversation!.topic.experienceId
          : (state.cachedExperience?.experience.id ?? '');
      if (expId.isEmpty) return;
      ref.read(experienceRepositoryProvider).invalidate(expId).then((_) {
        if (!ref.mounted) return;
        fetchExperienceDetails();
        _invalidateExperienceViewModelIfActive();
      }).catchError((Object error) {
        _log.warning('Failed to invalidate experience cache: $error');
      });
    }
    ```
- [ ] In `_handleNewMessage`, replace the bare `fetchExperienceDetails()` call in the RSVP/lifecycle handler (lines 417–426) with `_invalidateAndRefreshExperience()`.
- [ ] Run `npm run build` to verify no lint or compile errors.
- [ ] Run `flutter test --reporter expanded` to verify existing tests pass.

**Validates:** After the change, when a remote RSVP arrives via the stream, the cache invalidation completes before the subsequent fetch, both `ConversationViewModel` and `ExperienceViewModel` receive fresh server data, and the Details tab re-renders with the new attendee.

### Phase 2: Add a regression test

**Goal:** Prevent the stale-cache path from regressing silently.

**Tasks:**

- [ ] In `app/test/presentation/viewmodels/conversation_view_model_test.dart`, add a test scenario: given an active experience conversation, when the stream delivers a `CHAT_SYSTEM_ACTION_RSVP_YES` system message, verify that `MockExperienceRepository.invalidate()` is called with the correct experience ID (using the existing Mockito setup) and that a subsequent `getExperienceDetails()` call is made. This confirms the chained `.then()` executes.
- [ ] Also add a test for `RSVP_MAYBE` and `RSVP_NO` for completeness.
- [ ] Add a disposal safety test: verify that if the notifier is disposed between the `invalidate()` completing and the `.then()` body running, no state mutation or crash occurs (the `ref.mounted` guard handles this).

**Validates:** CI will catch any future regression where the invalidation step is removed.

## Related Issues

- No directly related issues found. The same cache invalidation pattern is already implemented correctly for giveaway/transfer lifecycle events and request mutations.

---

## Audit Notes

_Audited against `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md` — 2026-03-27_

**Corrections applied to this plan:**

1. **Stash is async-only — removed incorrect "synchronous removal" claim.** The original plan stated that `HybridCacheManager` removes cache entries synchronously, making the race between `invalidate()` and the subsequent fetch "theoretical." This is wrong: `caching.md` explicitly states "All cache operations are async" and that `getSync()` is deprecated and always returns null. The plan now correctly describes `invalidate()` as async and uses `.then()` chaining to serialize the operations.

2. **`_invalidateAndRefreshRequest()` is the canonical pattern — implementation updated to follow it.** The original Phase 1 snippet called `invalidate()` fire-and-forget and then called `fetchExperienceDetails()` and `_invalidateExperienceViewModelIfActive()` in the same synchronous frame — meaning the fetch could race the cache clear. The updated plan adds `_invalidateAndRefreshExperience()` that chains via `.then()`, exactly like `_invalidateAndRefreshRequest()` already does. This is the established pattern in this codebase for stream-handler-triggered cache invalidation.

3. **`ref.mounted` guard added to the `.then()` body.** The architecture doc notes that `autoDispose` providers can be disposed between an async gap and a state mutation. The `.then()` body must check `ref.mounted` before calling `fetchExperienceDetails()` or `_invalidateExperienceViewModelIfActive()`. A disposal safety test is added to Phase 2 to enforce this.

4. **No i18n changes needed.** The fix involves no user-visible strings; ARB files are not touched.

5. **No server changes needed.** The bug is entirely on the client side. The server correctly emits RSVP system messages and sends push notifications; the gap is in how the client handles those messages when the experience Details tab is open.
