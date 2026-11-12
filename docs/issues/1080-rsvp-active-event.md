# Issue #1080: [Bug Report] Allow RSVP to active event

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1080
**Filed:** 2026-04-09 | **Last activity:** 2026-04-09
**Labels:** bug, user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

A user reports they cannot RSVP to an "active" event. The term "active" from
the user's perspective likely means an event that is currently happening (i.e.
in the `IN_PROCESS` state), not `EXPERIENCE_STATE_ACTIVE` (which is the initial
state before anyone has RSVPed).

The root cause is a mismatch between the client UI and server validation:

1. **Server** (`server/services/experience/rsvp.go:44-49`): The RSVP endpoint
   only allows RSVPs when the experience is in `ACTIVE` or `JOINED` state. It
   rejects `IN_PROCESS` with a `FailedPrecondition` error.

2. **Client** (`app/lib/presentation/viewmodels/event_modal_view_model.dart:230-234`):
   The `_determinePhase` method maps `IN_PROCESS` to phase 0 (RSVP) for
   non-organizer joiners — so the RSVP buttons are shown in the UI, but tapping
   them results in a server error.

3. **Conversation action button** (`app/lib/presentation/widgets/chat/conversation_action_button.dart:114-115`):
   `isTerminal` is only true for `phase == null` (cancelled) or `phase == 3`
   (completed). Phase 2 (`IN_PROCESS`) is not terminal, so the "RSVP" button
   appears in the conversation bar for non-organizer joiners.

The result: a user sees an event that's in progress, sees the RSVP button, taps
it, opens the modal with RSVP toggle buttons, selects an intention, and gets an
error toast because the server rejects it.

## Context from Comments

No comments on this issue. The report came from in-app user feedback with a
screenshot. The user's description is minimal ("Allow me to RSVP to an active
event. I currently can't.") but aligns with the client/server mismatch
described above.

## Potential Solutions

### Option A: Allow RSVP for IN_PROCESS events (server-side fix)

- **Approach:** Add `EXPERIENCE_STATE_IN_PROCESS` to the allowed states in
  `rsvp.go:44-49`. This lets people join an event that's already happening.
- **Pros:** Minimal change. Makes sense from a UX perspective — someone who
  learns about an event while it's happening should be able to join. Fixes the
  bug at the source. No client changes needed since the UI already shows RSVP
  for IN_PROCESS.
- **Cons:** Need to verify that the state transition logic (lines 158-169 that
  promote ACTIVE → JOINED) handles IN_PROCESS correctly (it should be a no-op
  since IN_PROCESS is already past JOINED).
- **Effort:** Small.

### Option B: Hide RSVP UI for IN_PROCESS events (client-side fix)

- **Approach:** Change `_determinePhase` to return a non-RSVP phase for joiners
  when the experience is `IN_PROCESS`. Also update `ConversationActionButtonBuilder`
  to not show the RSVP button for phase 2.
- **Pros:** No server changes. Prevents the error by not offering the action.
- **Cons:** Users genuinely cannot join an in-progress event, which is a poor
  UX. The user filed this as a feature request/bug — they *want* to RSVP to an
  active event.
- **Effort:** Small.

### Option C: Both server and client changes

- **Approach:** Allow RSVP for IN_PROCESS on the server, AND adjust the client
  to show appropriate messaging (e.g. "Event is in progress — join now" instead
  of the standard RSVP copy).
- **Pros:** Best UX — the action works and the user knows the event is already
  happening.
- **Cons:** More work for a v1 fix. The messaging improvement could come later.
- **Effort:** Medium.

## Recommendation

**Option A** — allow RSVPs for `IN_PROCESS` events on the server. This is the
simplest fix and aligns with user intent. The client already handles
`IN_PROCESS` as RSVP-eligible in its phase mapping, so the only change is
adding `IN_PROCESS` to the server-side allowed states.

The state transition logic in `rsvp.go:158-169` only promotes from `ACTIVE` to
`JOINED`, so it will be a no-op for `IN_PROCESS` events — no unintended state
changes.

If we later want to improve the messaging (Option C), that can be a separate
enhancement.

## Open Questions (All Resolved)

- **Is there a scenario where RSVPing to an IN_PROCESS event should NOT be
  allowed?** — **Resolved: No.** The max_participants check still applies.
  If the organizer wants to prevent late joins, they can complete the event.

- **Should the organizer's "Wrap Up" flow be affected?** — **Resolved: No.**
  The only change is letting new people RSVP while the event is happening,
  before the organizer marks it complete.

- **Does the notification flow (line 78 in `lifecycle.go`) need to notify
  late-joining RSVPs?** — **Resolved: No changes needed.** The existing RSVP
  system message (rsvp.go:213-244) already handles this.

## Implementation Plan

### Phase 1: Server-side fix
**Goal:** Allow RSVPs for IN_PROCESS experiences.
**Tasks:**
- [ ] Update `server/services/experience/rsvp.go:44-49` to also allow `EXPERIENCE_STATE_IN_PROCESS`
- [ ] Add a test case in `server/services/experience/rsvp_test.go` that creates an experience, transitions it to IN_PROCESS, and verifies RSVP succeeds
- [ ] Add a test case that verifies RSVP is still rejected for COMPLETED and CANCELLED states
- [ ] Run `npm run build` to verify lint/compile
- [ ] Run `go test ./server/services/experience/...` to verify all tests pass
**Validates:** RSVPs are accepted for in-progress events; existing state guards for COMPLETED/CANCELLED still hold.

### Phase 2: Verify client behavior (no code changes expected)
**Goal:** Confirm the existing client-side phase mapping works correctly with the server fix.
**Tasks:**
- [ ] Verify `_determinePhase` returns phase 0 (RSVP) for IN_PROCESS joiners — already the case at `event_modal_view_model.dart:230-234`
- [ ] Verify `ConversationActionButtonBuilder` shows RSVP button for non-terminal IN_PROCESS — already the case since phase 2 is not terminal
- [ ] Verify the RSVP toggle buttons in `EventJoinerModal` render correctly for IN_PROCESS state — no state-specific guards exist
- [ ] Run `flutter test --reporter expanded` to confirm no regressions
**Validates:** End-to-end flow works: user sees RSVP button → opens modal → selects intention → server accepts.

## Audit Notes

**Audited:** 2026-04-10 | **Result:** Compliant

- **Architecture:** Server-only change in the experience service package. No service-to-service dependencies introduced. Follows the existing RPC handler pattern with validation, storage, and response. No violations of the service independence principle.
- **Caching:** No caching impact. The client-side `ExperienceRepository` cache is already invalidated by `_refreshExperience()` after RSVP submission (`event_modal_view_model.dart:399`). No manual cache maps or new cache keys needed.
- **Server/client boundary:** The fix correctly targets the server validation layer. The client already treats `IN_PROCESS` as RSVP-eligible (phase 0 for joiners), so the boundary is already aligned — only the server guard needs updating.
- **Testing:** Plan includes unit tests for both positive (IN_PROCESS RSVP succeeds) and negative (COMPLETED/CANCELLED rejected) cases. Consistent with the "no new functionality without test coverage" principle.
- **i18n:** No new user-facing strings introduced. The existing RSVP UI copy works for IN_PROCESS events without modification.
- **Note:** The state transition block in `rsvp.go:158-169` promotes `ACTIVE → JOINED`. Verify this block is a safe no-op when `expStored.State` is already `IN_PROCESS` (it should be, since the `if` only triggers for `ACTIVE`). Add a brief comment in the code documenting that IN_PROCESS is intentionally allowed.

## Related Issues
- None identified.
