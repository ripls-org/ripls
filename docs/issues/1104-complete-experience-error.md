# Issue #1104: [Bug Report] Failed to complete experience

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1104
**Filed:** 2026-04-13 | **Last activity:** 2026-04-13
**Labels:** bug, user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

A user on Android (Pixel 9 Pro, app v0.11.0) attempted to mark an experience
as completed and received an error. The most likely root cause is a state
validation gap in `RecordAttendance`: when `CompleteExperience` was updated
(commit `ca22d376`) to accept JOINED state as a valid starting state, the
corresponding `RecordAttendance` handler was not updated to match.

The client's `wrapUp()` flow calls `recordAttendance()` **before**
`completeExperience()`. If the experience is in JOINED state:

1. `recordAttendance` → fails with `CodeFailedPrecondition` ("attendance can
   only be recorded for active, in-process, completed, or cancelled
   experiences")
2. The error propagates to the client, which displays "Failed to complete
   experience: ..."

The user never reaches `completeExperience` because the preceding
`recordAttendance` call rejects JOINED state.

## Context from Comments

No comments on the issue. The report was auto-generated via in-app feedback
with a screenshot of the error. No additional context from the reporter.

## Relevant Code

**Server — RecordAttendance state check** (`server/services/experience/rsvp.go:334-343`):
```go
// Missing EXPERIENCE_STATE_JOINED in allowed states
if expStored.State != models.ExperienceState_EXPERIENCE_STATE_ACTIVE &&
    expStored.State != models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS &&
    expStored.State != models.ExperienceState_EXPERIENCE_STATE_COMPLETED &&
    expStored.State != models.ExperienceState_EXPERIENCE_STATE_CANCELLED {
    return nil, connect.NewError(connect.CodeFailedPrecondition, ...)
}
```

**Server — CompleteExperience state check** (`server/services/experience/lifecycle.go:127-135`):
```go
// Correctly includes JOINED
switch expStored.State {
case models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
    models.ExperienceState_EXPERIENCE_STATE_JOINED,
    models.ExperienceState_EXPERIENCE_STATE_IN_PROCESS:
    // Valid source states for completion.
```

**Client — wrapUp flow** (`app/lib/presentation/viewmodels/event_modal_view_model.dart:726-796`):
```dart
// Calls recordAttendance BEFORE completeExperience
await _repository.recordAttendance(...);
// If above fails, completeExperience is never reached
final response = await _repository.completeExperience(...);
```

**Commit introducing the gap**: `ca22d376` — "Allow direct completion from
ACTIVE/JOINED state" updated `CompleteExperience` but not `RecordAttendance`.

## Potential Solutions

### Option A: Add JOINED to RecordAttendance allowed states

- **Approach:** Add `EXPERIENCE_STATE_JOINED` to the state check in
  `RecordAttendance` (rsvp.go:337), matching the states allowed by
  `CompleteExperience`.
- **Pros:** Minimal change, directly fixes the root cause, consistent with the
  intent of `ca22d376`.
- **Cons:** None significant. JOINED is a superset of ACTIVE (someone has
  RSVPed), so allowing attendance recording is logically sound.
- **Effort:** Small.

### Option B: Make client resilient to recordAttendance failure

- **Approach:** In `wrapUp()`, catch errors from `recordAttendance` and
  continue to `completeExperience` anyway, relying on the server's
  `autoMarkAttendanceIfNeeded` fallback.
- **Pros:** More resilient client regardless of server state validation.
- **Cons:** Masks server bugs. The attendance the user explicitly recorded
  would be silently lost. The `autoMarkAttendanceIfNeeded` fallback only
  auto-marks YES RSVPs — it doesn't capture NO attendance. This is a kludge,
  not a fix.
- **Effort:** Small.

### Option C: Reorder client calls (complete first, then record attendance)

- **Approach:** Call `completeExperience` first, then `recordAttendance`.
  Since `RecordAttendance` already allows COMPLETED state, this would work.
- **Pros:** Sidesteps the state check issue entirely.
- **Cons:** `CompleteExperience` counts attended RSVPs for impact estimation.
  If attendance hasn't been recorded yet, the count will be wrong (relying on
  `autoMarkAttendanceIfNeeded` which only auto-marks YES RSVPs). This defeats
  the purpose of recording attendance first.
- **Effort:** Small, but introduces data quality regression.

## Recommendation

**Option A** — add JOINED to RecordAttendance's allowed states. This is the
correct fix: JOINED means people have RSVPed, which is exactly when you'd want
to record attendance. The server state check was simply missed when
`CompleteExperience` was updated. Options B and C work around the problem
rather than fixing it.

## Open Questions

- **Was the experience actually in JOINED state?** — Recommendation: Very
  likely, given the error path analysis. Could confirm via production logs
  filtering by the reporter's user ID and the `RecordAttendance` operation,
  but the code analysis is sufficient to identify and fix the bug regardless.
- **Are there other RPCs with similar JOINED-state gaps?** — Recommendation:
  Check `MarkExperienceInProcess` and time proposal RPCs. Quick grep shows
  `MarkExperienceInProcess` already accepts JOINED (lifecycle.go:51), and time
  proposals accept ACTIVE and JOINED (time_proposals.go:46). Only
  `RecordAttendance` is missing JOINED.

## Implementation Plan

### Phase 1: Fix RecordAttendance state validation

**Goal:** Allow attendance recording for JOINED experiences.

**Tasks:**
- [ ] Add `EXPERIENCE_STATE_JOINED` to the state check in `RecordAttendance`
  (`server/services/experience/rsvp.go:337`)
- [ ] Update the error message comment to list all allowed states including
  JOINED
- [ ] Add test case: record attendance on a JOINED experience (rsvp_test.go)
- [ ] Add test case: complete experience from JOINED state with attendance
  (lifecycle_test.go) — end-to-end flow matching the client's wrapUp sequence
- [ ] Run `npm run build` to verify lint + compile
- [ ] Run `go test ./server/services/experience/...` to verify all tests pass

**Validates:** The exact user flow that triggered the bug report now succeeds
end-to-end.

## Related Issues

- None referenced in the issue. The root cause commit (`ca22d376`) was a
  standalone improvement to the experience lifecycle flow.

---

## Audit

**Status:** Pass — no architectural issues found.

- Server-only fix within a single service package; no cross-service coupling.
- No client changes required (client already sends the correct request).
- No caching, i18n, or navigation concerns.
- Testing plan covers both the isolated state check and the full client-mimicking
  flow (recordAttendance → completeExperience from JOINED state).
- Recommendation (Option A) aligns with project principle: fix the root cause at
  the server validation layer, don't add client-side workarounds.
