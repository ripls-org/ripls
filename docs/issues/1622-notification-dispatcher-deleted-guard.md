# Issue #1622: Notification dispatcher: skip deleted communities at the source

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1622
**Filed:** 2026-04-30 | **Last activity:** 2026-04-30
**Labels:** none | **Assignees:** briangbrown
**Context docs:** [`docs/community_delete_and_leave.md`](../community_delete_and_leave.md), [`docs/issues/1621-deleted-community-read-filter.md`](./1621-deleted-community-read-filter.md)

> **Helper renamed** (during #1623 implementation, 2026-04-30) — the
> helper introduced by this issue was originally named
> `MaySendNotification`. While starting #1623 we noticed the new
> `MayWriteToCommunity` would be byte-for-byte identical except for
> log strings, so we consolidated to a single
> `community.IsActive(ctx, s, communityID) bool` helper used by both
> the notification dispatchers (this issue) and the async-write
> guard (#1623). Throughout the rest of this plan, mentally
> substitute `MaySendNotification` → `IsActive`. The behaviour and
> wiring are unchanged; only the name is generalized. The CI Rule C
> grep was updated to match `community.IsActive`.
>
> **Audit applied** (`/ripls-audit`, 2026-04-30) — three refinements vs.
> `docs/server/architecture.md` and `docs/server/observability.md`:
> 1. The dispatcher goroutine runs in a fresh `notifyCtx` derived from
>    `context.Background()` (see `server/community/events.go:77`), not
>    the request context. So `logging.LoggerWithContext(ctx)` inside
>    the helper does NOT inherit `request_id` / `user_id` from the
>    originating RPC. Phase 1 must explicitly attach `community_id`
>    and any other fields needed for log correlation.
> 2. Phase 2's mock-call assertion uses
>    `notifications.NewMockService()` (`server/notifications/mock.go:25`)
>    + `m.GetCalls()` (line 53). Plan now names them.
> 3. Phase 3's CI grep gate **extends** the existing
>    `scripts/check_active_community_gate.js` from #1621 — it is
>    not a new script. Adding a third rule to that gate is one diff,
>    one file, and stays in the same `lint:go:active-community-gate`
>    npm step.

## Problem Statement

After a community is soft-deleted, no community-scoped push notifications
should fire for it — including notifications triggered by writes that
happen during the deletion window (a chat message that lands before the
community-scoped chat is fully sealed off, an in-flight async LLM job
that completes after delete, a `CommunityEvent` recorded just before the
delete commit). The user-facing promise from the design doc is that
deleted communities are invisible everywhere except Settings → Communities.

The guard must live at the **dispatcher**, not at every caller. A new
`CommunityEventType` added in the future, or a new dispatcher path, must
not be able to leak push notifications by forgetting to add a
`Community.Deleted` check.

This issue is **Phase 1 scaffolding for #1619** — it ships as a no-op
because no community is in the deleted state today (verified by #1624).
Tests force-flip `Community.Deleted` to prove the guard fires.

## Context from Comments

No comments — this issue was filed as part of the #1619 decomposition,
along with #1621 and #1623. The design doc and the merged work in #1621
provide the surrounding context.

What's worth knowing from the codebase audit:

- There are **two** dispatchers, not one. The issue mentions
  `server/community/notifications.go`'s `notifyTargetedUsers` (the
  community-event dispatcher), but there's also
  `server/services/chat/notifications.go`'s `notifyParticipants` (the
  chat-message dispatcher) at line 163. Both call
  `notificationService.NotifyUser(ctx, userID, notification)` and both
  scope their work by `conversation.CommunityId`. The guard must apply
  to both.
- The community-event dispatcher already has a `shouldNotify` global
  allow-list and a per-recipient `CategoryEnabled` filter. The new
  deleted-community guard sits **above** both — events in the
  allow-list with category enabled still get suppressed when the
  community is deleted.
- `RecordCommunityEventAndNotify` records the event row first, then
  fans out to `notifyTargetedUsers` in a `logging.GoSafe` goroutine.
  The event row itself is **always preserved** (it's the audit trail);
  only the user-facing push is suppressed.

## Potential Solutions

### Option A: Inline `Community.Deleted` check in each dispatcher

- **Approach:** At the top of `notifyTargetedUsers` and at the top of
  `notifyParticipants`, do a `s.storage.GetByID(ctx, communityID,
  &models.Community{}, storage.QueryOptions{IncludeDeleted: true})`,
  check `community.Deleted`, return early if set.
- **Pros:** Smallest possible diff. Each dispatcher's check is local
  and easy to read.
- **Cons:** Two copies of the same logic. A third dispatcher (e.g., a
  future "community-scoped email" path) could miss the check. The
  copy-paste is exactly the failure mode the issue calls out.
- **Effort:** Small.

### Option B: Shared `MaySendNotification` helper, called by every dispatcher

- **Approach:** Add `community.MaySendNotification(ctx, s,
  communityID) bool` as a single chokepoint. Returns `false` if the
  community is soft-deleted, returns `true` otherwise (also returns
  `true` if the lookup fails — see "Open Questions" below for the
  fail-open vs. fail-closed decision). Both dispatchers call it before
  the FCM `NotifyUser` call.
- **Pros:** One implementation, one logging convention, one test
  surface, one place to extend if more state ever has to gate
  notifications (e.g., user-level "do not disturb"). Follows the
  shared-library pattern already established in
  `server/community/notifications.go` (`shouldNotify`, `CategoryFor`,
  `CategoryEnabled` are all peers).
- **Cons:** New helper to introduce. Slight risk that a future
  dispatcher author forgets to call it — same risk as Option A but
  documented in one place.
- **Effort:** Small-medium. The helper itself is ~20 lines; the wiring
  is one call per dispatcher.

### Option C: Push the check into `notifications.Service.NotifyUser`

- **Approach:** Make `NotifyUser` itself look up the community and
  refuse to send. The guard is at the FCM-provider boundary; no
  dispatcher can bypass it.
- **Pros:** Strongest enforcement. Compile-time impossible to leak
  pushes.
- **Cons:** `notifications.Service` is a generic FCM abstraction —
  it doesn't know about communities. Pushing community-aware logic
  into it pollutes a clean library interface and breaks the package
  layering (`server/notifications/` would have to import
  `server/community/` or a model type). Also: not every notification
  is community-scoped (waitlist notifications use `emailService`,
  password resets are user-scoped via `Notification`). Coupling the
  generic provider to community state is the wrong place.
- **Effort:** Medium-large, with collateral changes to
  `notifications.Service`.

## Recommendation

**Option B** — shared `MaySendNotification` helper. Rationale:

1. Two dispatchers exist today (community events, chat); a third is
   plausible (e.g., the day-before-purge reminder in #1619 §8 will
   need its own dispatch path that **bypasses** this guard, which
   makes the helper-based design easy: that path simply doesn't call
   the helper).
2. The codebase already follows this shape — `shouldNotify`,
   `CategoryFor`, `CategoryEnabled` are sibling helpers in the same
   file. `MaySendNotification` slots in cleanly.
3. Doesn't pollute the generic `notifications.Service` interface with
   community semantics. Layering stays clean.
4. The CI grep gate (already shipped in #1621) will be extended with
   one more check: any file in `server/services/` or `server/community/`
   or `server/services/chat/` that calls `notifications.Service.NotifyUser`
   must also reference `MaySendNotification`. That gives compile-time-style
   enforcement via lint.

## Open Questions

- **Fail open or fail closed when the `Community` lookup itself
  errors?** The existing pattern in `notifyTargetedUsers` (when the
  `FetchPreferencesForUsers` call fails) is fail-open: log a warning
  and proceed. Rationale recorded inline: "missed pushes are a worse
  failure mode than over-delivering during the brief degraded window."
  **Recommendation:** match that — fail open. A transient DB hiccup on
  the deleted-state check shouldn't silently mute pushes for active
  communities. Soft-deletes are rare; pushes are frequent. Match the
  existing precedent and document it.
- **Where does the helper live — `server/community/`, or a new
  `server/notifications/community_guard.go`?** Both have downsides:
  `community/` couples to community semantics (already does);
  `notifications/` would have to import `community/` (creates a
  layering question).
  **Recommendation:** `server/community/notifications.go`, alongside
  `shouldNotify` / `CategoryFor` / `CategoryEnabled`. They're all
  community-aware notification gates. Adding a fourth peer is the
  obvious shape.
- **Should the day-before-purge reminder (`#1619 §8`) bypass the
  helper, or should the helper take a "force" option?** The reminder
  needs to fire **for** deleted communities — that's the whole point.
  **Recommendation:** the reminder calls a different code path (it
  iterates `Community.deleted_snapshot.member_user_ids` and dispatches
  push directly via `notifications.Service`). It will not call
  `MaySendNotification`. No special-case in the helper. Documented
  here so future readers know it's intentional.
- **Should `COMMUNITY_RESTORED` notifications be specially handled?**
  The restore RPC clears `Community.Deleted` **before** firing the
  notification, so by the time `notifyTargetedUsers` runs, the lookup
  sees `Deleted == nil` and the notification proceeds.
  **Recommendation:** no special case needed; a test covers the
  ordering invariant in #1619 Phase 2.
- **Do we cache the `Community` lookup across calls in the same
  dispatch goroutine?** Each invocation of the dispatcher fetches the
  community once and uses it for one event's worth of notifications.
  **Recommendation:** no caching needed. The dispatcher already runs
  in a fresh `notifyCtx` (see
  `server/community/events.go:77-78`) — there's no cross-call state
  to share. One `GetByID` per dispatch is acceptable.
- **What about historical events fired during the deletion window
  that are queued to deliver later (e.g., scheduled push)?** Today
  the dispatcher is synchronous-within-the-goroutine — there's no
  scheduled-delivery surface. If one is added, it will need to
  re-check deleted state at delivery time, not enqueue time. Not in
  scope for this issue.

## Implementation Plan

### Phase 1: Add `MaySendNotification` helper

**Goal:** Land the helper in `server/community/notifications.go`
with full unit-test coverage. No callers wired yet — the system is
unchanged in production behavior.

**Tasks:**
- [ ] Add `MaySendNotification(ctx context.Context, s
      *storage.ProtoSQLStorage, communityID string) bool` in
      `server/community/notifications.go`. Implementation: `GetByID`
      with `IncludeDeleted: true`, check `Deleted != nil &&
      DeletedAtUnixSec > 0`, return `false` if so. On lookup error,
      log a warning and return `true` (fail open) per the open-question
      decision above. Logger:
      `logging.LoggerWithContext(ctx).With("operation",
      "MaySendNotification", "community_id", communityID)`. **Note:**
      callers run inside a fresh `notifyCtx` derived from
      `context.Background()` (see `server/community/events.go:77`), so
      `request_id` / `user_id` from the originating RPC are NOT
      automatically propagated — log fields are limited to whatever
      this helper attaches plus whatever the caller adds before
      invoking. That's acceptable; the helper's role is the
      gate-result log, not full request correlation.
- [ ] Unit tests in `server/community/notifications_test.go`:
      active community returns true; soft-deleted community returns
      false; missing community returns true (fail open) with a logged
      warning; storage error returns true (fail open) with a logged
      warning.

**Validates:** The helper is correct in isolation. We can flip a
test community to deleted and the helper returns `false`.

### Phase 2: Wire the helper into the two dispatchers

**Goal:** Both `notifyTargetedUsers` and `notifyParticipants` call
`MaySendNotification` before sending. Tests prove the guard fires.

**Tasks:**
- [ ] In `server/community/notifications.go::notifyTargetedUsers`, add
      `if !MaySendNotification(ctx, s, event.CommunityId) { logger.InfoContext(ctx,
      "skipping dispatch — community deleted", "reason", "community_deleted",
      "event_id", event.Id, "event_type", event.EventType); return }`
      at the top of the function, after the logger is built but before
      `getNotificationRecipients`.
- [ ] In `server/services/chat/notifications.go::notifyParticipants`,
      add the same guard at the top, keyed on
      `conversation.CommunityId`. Skip the guard when
      `conversation.CommunityId == ""` (private DM-style conversations
      have no community context).
- [ ] Integration-style test in `server/community/notifications_test.go`:
      seed community + members + recipient prefs, force-flip
      `Community.Deleted`, fire a `CommunityEvent` of every type that
      passes `shouldNotify`, assert the mock notification service got
      0 `NotifyUser` calls. Use `notifications.NewMockService()`
      (`server/notifications/mock.go:25`) and inspect captured calls
      via `m.GetCalls()` (line 53). The mock returns a `[]NotificationCall`
      slice; assert `len(calls) == 0` after the dispatch goroutine
      drains (drain via the `notificationDone` channel that
      `RecordCommunityEventAndNotify` already accepts for testing —
      see `server/community/events.go:50`).
- [ ] Test in `server/services/chat/notifications_test.go`:
      seed conversation + community + participant, force-flip
      `Community.Deleted`, write a chat message that triggers
      `notifyParticipants`, assert the mock got 0 calls.
- [ ] Test in `server/community/notifications_test.go`: `CommunityEvent`
      recorded for an active community still notifies (guard does not
      false-positive).

**Validates:** Push notifications stop the moment a community is
soft-deleted, in both the community-event and chat-message paths.
The audit trail (`CommunityEvent` rows) is preserved.

### Phase 3: CI grep gate extension

**Goal:** A new dispatcher path can't be added without the guard.

**Tasks:**
- [ ] **Extend** the existing `scripts/check_active_community_gate.js`
      from #1621 with Rule C: any file under `server/community/` or
      `server/services/` that calls `.NotifyUser(` must also reference
      `community.MaySendNotification` (qualified — strict on purpose
      so a typo'd import alias can't silently pass). The helper
      definition file (`server/community/notifications.go`) and test
      files are exempt.
- [ ] Confirm zero violations after Phases 1 and 2.

**Sequencing note:** This branch is rebased on top of
`1621-deleted-community-read-filter` so the gate script (introduced
in #1621) is available to extend. The PR base on GitHub is
`1621-deleted-community-read-filter`, not `main`, so the diff
review only shows this issue's commits.

**Validates:** A future RPC that adds a third notification dispatch
path is rejected at lint time unless it explicitly calls the guard.

## Related Issues

- #1619 — parent issue (community soft-delete with 30-day restore
  window). This issue is one of three Phase 1 prerequisites.
- #1621 — sibling Phase 1 issue (read-path filter). Currently in
  review (PR #1630). The pattern of "shared helper + CI grep gate"
  established there is reused here.
- #1623 — sibling Phase 1 issue (background-job write guard). Same
  failure-mode family: writes fired against deleted communities. This
  issue covers push; #1623 covers persistence.
- #1340 — closed; established the `connecterr.Internal` pattern that
  the helper's error wrapping follows.
