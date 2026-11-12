# Time Simplification v2

## Overview

Simplify time scheduling so the **owner decides the mode** — either setting a
fixed time directly or asking the group to vote on proposed options. Non-owners
no longer propose times; they only vote. This changes the permission model,
the time modal UX for owners, and the voting card UX for non-owners.

## Reference Materials

- **React mockup:** [tmp/time2/time-modal.jsx](../../tmp/time2/time-modal.jsx)
- **Visual references:**
  - [time-modal.png](../../tmp/time2/time-modal.png) — Owner modal: "Set Time" tab (working, reuse)
  - [time-ask1.png](../../tmp/time2/time-ask1.png) — Owner modal: "Ask the Group" tab, no options yet
  - [time-ask2.png](../../tmp/time2/time-ask2.png) — Owner modal: "Ask the Group" with options added
  - [vote-card.png](../../tmp/time2/vote-card.png) — Non-owner vote card
- **React mockup (vote card):** [tmp/time2/vote-card.jsx](../../tmp/time2/vote-card.jsx)
- **Existing docs:**
  - [docs/client/time.md](../client/time.md) — Current time modal architecture
  - [docs/ai/chat-wf-exp.md](chat-wf-exp.md) — Chat workflow (phases affected by this change)
- **Existing implementation:**
  - [time_modal.dart](../../app/lib/presentation/screens/experience/time_modal.dart) — Time modal (709 lines)
  - [time_modal_view_model.dart](../../app/lib/presentation/viewmodels/time_modal_view_model.dart) — ViewModel
  - [time_slot_card.dart](../../app/lib/presentation/widgets/experience/time_slot_card.dart) — Voting card
  - [when_row.dart](../../app/lib/presentation/widgets/experience/when_row.dart) — Time summary row
  - [server/services/experience/time_proposals.go](../../server/services/experience/time_proposals.go) — ProposeTime RPC

---

## Problems

### 1. Any participant can propose times — roles are unclear

The current system allows any RSVPed participant to call `ProposeTime`. This is
intentionally democratic, but it creates ambiguity: there is no clear
coordinator. In practice, the owner is expected to drive scheduling, but the
UI treats all participants equally. This leads to cluttered proposal lists and
confusion about whose job it is to lock a time.

**Impact**: Multiple competing proposals from different people, no single source
of truth, owner must sort through community submissions.

### 2. No explicit "poll mode" — owner intent is invisible

The current "Open to suggestions" toggle in the modal is the owner's way to
control whether new proposals are accepted. But there is no persistent, visible
signal to other participants that a poll is in progress. Non-owners see the
same proposal-and-vote list regardless of whether the owner deliberately opened
suggestions or just has unresolved proposals from earlier.

**Impact**: Non-owners can't tell whether they should be proposing or just
voting. The WhenRow in the experience content view shows no poll state at all.

### 3. The propose form is the same for owner and non-owners

Both the owner and joiners see the same `ProposeTimeForm` (date picker + time
picker + duration). For the new design, the owner's "Ask the Group" flow uses
a day chip grid and time-of-day buttons (Morning / Afternoon / Evening) to
quickly build poll options — a much faster UX than the full date/time picker.
Non-owners should not see a propose form at all.

**Impact**: The owner's scheduling flow is slower than necessary; non-owners
have access to a form they shouldn't be able to use.

### 4. No "None of these work" escape hatch for voters

The current time slot card is tap-to-toggle yes. There is no way for a voter to
signal that none of the proposed times work for them. The owner has no signal
about impossible voters vs. abstainers.

**Impact**: Owners can't distinguish between "hasn't voted yet" and "nothing
works." Can lead to locking a time that doesn't work for someone who never
responded.

### 5. chat-wf-exp design gives joiners a "Propose a time" action

The Phase 3 manage menu in [chat-wf-exp.md](chat-wf-exp.md) includes "Propose
a time" for the joiner. Phase 9 adds a "Propose another time" button to the
inline time voting card. Both are contradicted by the new design where only the
owner adds poll options.

**Impact**: If chat-wf-exp is implemented before this change, those surfaces
will need to be reworked. If implemented after, those features can be omitted
from the start.

### 6. WhenRow doesn't distinguish poll state from fixed time

The WhenRow in the experience content view shows "Time is fixed" or "N other
proposed times". It has no concept of "a poll is active — tap to vote." The
distinction matters: non-owners should be invited to vote, not just informed
that a time exists.

**Impact**: Non-owners may not realize there is a poll awaiting their vote.

---

## Proposed Solution

### Owner: Two-tab time modal

The owner's time modal gets a segmented toggle at the top: **"Set time"** and
**"Ask the group"**.

**Set time tab** (default): The existing `ProposeTimeForm` (date chips + time
chips + duration chips + pickers) sets the experience time directly. This path
already works — reuse it. CTA: "Send Invites" (create) / "Update Time" (edit).

**Ask the group tab**: Owner builds a poll. The UI uses a day chip row (4
quick-access days) and, once a day is selected, a time-of-day row (Morning /
Afternoon / Evening). Tapping a combination adds it to the poll option list.
An "Add a specific date & time" button below the chips expands an inline date +
time picker for custom options. Options can be removed individually. Maximum 5
options. CTA: "Send Poll (N options)".

The owner opens this modal by tapping the time row in the experience content
view or via the manage menu in the chat input bar.

Once a poll is active, the owner can return to this modal to see vote results
(which option has the most "yes" votes, voter avatars per slot). They confirm
the winning option using the existing `ConfirmTime` RPC. To cancel the poll and
switch modes, they must explicitly cancel — starting fresh.

### Non-owner: Vote card

When a poll is active, non-owners see a **vote card** instead of the time modal.
Entry points: tapping the time row in the experience content view, or accessing
via the manage menu in chat. The vote card is a full-screen bottom sheet.

**Vote card layout** (matches [vote-card.jsx](../../tmp/time2/vote-card.jsx)):
- Top context: owner avatar + "[Owner] is finding a time for [Experience name]"
- Subhead: "Tap times you can make it" + "Pick all that work."
- Slot list: Each option is a tappable row with a checkbox (sage when selected),
  the day + time string, and an avatar stack of other people who have already
  voted for that slot.
- Footer: "None of these work for me" text link (shown before any selection),
  then a "Submit" / "Can't Make It" / "Pick a time" CTA button.

Voting calls the existing `VoteOnTime` RPC for each selected option. "None of
these work" submits a `TIME_VOTE_STATUS_NO` across all options (or calls a new
`DeclineTimePoll` RPC — see Open Questions).

Non-owners who have already voted can return to the vote card to see current
results and change their votes. The card stays interactive until the owner
confirms a time.

### What stays the same

- `ConfirmTime` and `UnlockTime` RPCs — owner confirms/unlocks as before.
- `VoteOnTime` RPC — non-owners vote using the existing call.
- The finalized view (confirmed time with "Change time" button) — no changes.

---

## Recommendations

### 1. Restrict `ProposeTime` to the owner server-side

The simplest server change: add an owner-only permission check to `ProposeTime`.
Any non-owner call returns `PERMISSION_DENIED`. This enforces the new model
without a new RPC. The RPC name stays the same; only the auth check changes.

### 2. Add `time_poll_active` to the experience data model

The experience needs a flag that persists whether the owner has started a poll
(i.e., submitted the "Ask the Group" form). This drives:
- What non-owners see when they tap the time row (vote card vs. read-only)
- What WhenRow shows ("Poll active — tap to vote" vs. fixed time)
- Whether the owner sees the "Set time / Ask the group" toggle or poll results

Add `bool time_poll_active` to the experience storage model and include it in
`GetExperienceDetails` (and `GetExperience`) responses.

The flag is set to `true` when the owner submits the "Ask the Group" form (which
calls `ProposeTime` one or more times). It is cleared when the owner calls
`ConfirmTime` (a time has been selected) or explicitly cancels the poll.

### 3. Reuse `ProposeTime` for owner poll option creation; no new RPC needed

The "Ask the Group" flow calls `ProposeTime` once per poll option as the owner
adds them. The server sets `time_poll_active = true` when the first option is
added (if owner is the caller). No new RPC is needed for adding options.

A `CancelTimePoll` RPC (or extending `UnlockTime`) is needed to clear all
proposals and reset `time_poll_active`. This lets the owner switch back to "Set
time" mode.

### 4. Keep the "Set time" tab working as-is; only the "Ask the group" tab is new UI

The `ProposeTimeForm` widget already implements the "Set time" tab exactly.
Wrap it in the two-tab layout without touching the form itself. The "Ask the
group" tab is a new widget.

### 5. New `PollOptionsBuilder` widget for the "Ask the group" tab

Build a new `PollOptionsBuilder` stateful widget (owner-only) that manages the
day chip + time-of-day grid + custom date/time form. It outputs a list of
`(DateTime date, TimeOfDay time)` pairs that map to `ProposeTime` calls when
the owner taps "Send Poll".

All `ProposeTime` calls are fired as a batch when the owner taps the CTA, not
as individual additions. This avoids partial state on the server if the user
backs out. Consider whether a single `CreateTimePoll` RPC (batching all options
in one call) would be cleaner than multiple `ProposeTime` calls — see Open
Questions.

### 6. New `VoteCard` widget adapts `TimeSlotCard`

The vote card is a new widget (not a mode of `TimeModal`). It shows as a
full-screen bottom sheet (same height as `TimeModal`). Adapt the existing
`TimeSlotCard` styling: checkbox + day + time + voter avatar stack. The "None
of these work" button is new UI.

### 7. Update WhenRow for poll state

WhenRow should show three states:
- **No time set**: TBD with a prompt to set time (owner) or "awaiting time" (non-owner)
- **Poll active**: Coral calendar icon + "Poll active · N options" + "Tap to vote"
  for non-owners; "Poll active · N votes" for owner
- **Fixed time set**: Existing locked/unlocked display

### 8. Update chat-wf-exp manage menu and time voting card

Remove "Propose a time" from the joiner manage menu (Phase 3 of chat-wf-exp).
Remove "Propose another time" button from the inline time voting card (Phase 9
of chat-wf-exp). These were based on the old model. The inline time voting card
in chat should use the new vote card UX (checkbox + voter avatars, no proposal
form).

---

## Open Questions

### Q1: How does "None of these work for me" get represented?

**Option A**: Client-side only — user simply doesn't submit any votes; owner
sees them as non-respondents. Simple, no server change. Downside: no way to
distinguish "hasn't seen the poll" from "none work."

**Option B**: New `TIME_VOTE_STATUS_NONE` enum value. User calls `VoteOnTime`
with `NONE` status on a sentinel proposal, or a new `DeclineTimePoll` RPC. Owner
can see who explicitly declined. More server work but gives the owner actionable
info.

**Recommendation**: Start with Option A (omit "None of these work" from server
tracking). Show the button in the UI — tapping it submits no votes and closes
the card with a confirmation message. Add server tracking in a follow-up if the
need is validated.

### Q2: Can the owner vote on their own poll?

The owner creates the options, so they presumably know their own availability.
Allowing them to vote would add visual signal to the results (the owner's
checkmark is meaningful). But it also feels odd for the organizer to vote
alongside participants.

**Recommendation**: Allow owner voting — the existing `VoteOnTime` RPC is
owner-unrestricted. The vote card (if shown to the owner as a results view) can
include a "Your vote" indicator. Keep it optional: the owner sees results even
without voting.

### Q3: How does the owner cancel a poll?

Two entry points need a cancel path:
1. From the time modal, while the poll is in progress (before anyone has voted):
   owner taps "Cancel poll" → clears all proposals, resets `time_poll_active`.
2. After some votes are in: the design says the owner "must cancel and create a
   new one if they want to change it" — so same UX, but with a confirmation
   dialog warning that existing votes will be lost.

**Recommended RPC**: Add `CancelTimePoll` (or extend `UnlockTime` to handle
this case). Server deletes all proposals for the experience and clears
`time_poll_active`.

### Q4: What does the owner see when they open the time modal with an active poll?

The reference says owners can see poll results from the modal. Should they see:
- **Option A**: A read-only results list (same as the current default mode with
  vote counts and avatars) plus a "Confirm winner" CTA.
- **Option B**: The `VoteCard` with voting ability (if they can vote) plus a
  "Confirm" CTA.
- **Option C**: The "Ask the Group" tab pre-populated with the current options,
  in a results state.

**Recommendation**: Option A — a results view distinct from the voter's vote
card. Show each option with vote count + voter avatar stack, sorted by votes.
The "Finalize" / "Confirm winner" CTA selects the top option. No toggle back to
"Set time" until the owner explicitly cancels the poll.

### Q5: Should "Add a specific date & time" use a separate form or the existing `ProposeTimeForm`?

The "Ask the Group" tab in the mockup has an inline mini-form (date + time side
by side) for custom options. The existing `ProposeTimeForm` is a full-screen
scrollable form with chips and pickers — overkill for one option inside a list.

**Recommendation**: Implement the compact inline form as shown in the mockup
(two `TextFormField`s side by side for date and time). The duration is not
collected per-option in the poll — all options are treated as same-duration
(the experience's default duration) or duration-unspecified. This simplifies
the poll option model.

### Q6: Does the existing data model need to track each option's duration?

Currently `TimeProposal.time` is an `ExperienceTime` which can carry duration
via `SpecificTime.duration_minutes`. Poll options from the "Ask the Group" tab
(using Morning / Afternoon / Evening) may not have a specific duration.

**Recommendation**: For poll options, store only the start time; duration comes
from the experience's base duration when the winning option is confirmed. This
matches how a real "when can you make it" poll works — people vote on a start
time, not on a duration.

---

## Implementation Plan

### Phase 0: Proto + Server Changes

**Goal**: Enforce owner-only proposals and add poll mode tracking.

**Changes**:
- **`time_poll_active` field**: Add `bool time_poll_active` to
  `models.Experience` storage model. Add to `api.ExperienceDetails` (and
  `api.GetExperienceResponse`).
- **`ProposeTime` owner-only**: Add `expStored.OwnerId != authInfo.UserID`
  check in `time_proposals.go` after the experience fetch. Non-owner calls
  return `PERMISSION_DENIED`. Remove the existing "must have RSVP" check (owners
  don't need an RSVP).
- **`time_poll_active` setter**: When `ProposeTime` succeeds for the owner,
  if `time_poll_active` is not already set, set it to `true` and persist.
- **`CancelTimePoll` RPC**: New owner-only RPC in `time_proposals.go`. Deletes
  all `TimeProposal` records for the experience, clears `time_poll_active`,
  and emits a system message ("Alfred cancelled the time poll").
- **Proto**: Add `CancelTimePollRequest` / `CancelTimePollResponse` to
  `proto/ripls/api/experience.proto`. Add `time_poll_active` to the experience
  details response.

**Files modified**:
- `proto/ripls/models/experience.proto` (add `bool time_poll_active`)
- `proto/ripls/api/experience.proto` (add `bool time_poll_active` to
  `ExperienceDetails` response; add `CancelTimePollRequest` /
  `CancelTimePollResponse` message types and RPC)
- `server/services/experience/time_proposals.go` (permission check, poll flag
  setter in `ProposeTime`, new `CancelTimePoll` handler)
- `server/services/experience/assembly.go` (or wherever experience model →
  API response conversion happens): copy `time_poll_active` from storage model
  to API response
- SQL migration: add `time_poll_active boolean NOT NULL DEFAULT false` column
  to experiences table

**Tests**:
- `ProposeTime` by non-owner returns `PERMISSION_DENIED`.
- `ProposeTime` by owner sets `time_poll_active = true`.
- `CancelTimePoll` by owner deletes all proposals, clears flag.
- `CancelTimePoll` by non-owner returns `PERMISSION_DENIED`.

**Estimated scope**: ~80-120 lines server + ~60-80 lines test code + SQL migration

---

### Phase 1: Owner Time Modal — Two-Tab Layout

**Goal**: Owner sees "Set time" and "Ask the group" tabs; "Set time" is
unchanged.

**Changes**:
- **Tab toggle**: Add a segmented control at the top of `TimeModal` (matching
  the `time-modal.jsx` toggle style). Tabs: "Set time" | "Ask the group". State
  managed in `TimeModalData` via a `timeMode` field (`'fixed'` | `'poll'`).
  Initialized based on `isPollActive` from server response.
- **"Set time" tab**: Renders existing `ProposeTimeForm` — no changes to the
  form itself. CTA bottom bar: "Send Invites" (create context) or "Update Time"
  (edit context). Calls existing `updateEventTime()` notifier method.
- **Toggle disabled when poll is active**: If `timePollActive` is true, the
  toggle locks to "Ask the group" — owner must cancel the poll first before
  switching back to "Set time". Show a subtle lock indicator.
- **Edit context banner**: The existing "Editing — your friends will be notified"
  banner appears when `context == 'edit'` (direct time update, not poll).
- **`TimeModalData`**: Add `timePollActive: bool` (from server), `timeMode:
  String` (local tab state, defaults to `'fixed'` unless poll is active).
- **`TimeModalNotifier`**: Populate `timePollActive` from the experience details
  response in `build()`.

**Files modified**:
- `app/lib/presentation/screens/experience/time_modal.dart` (add toggle, tab routing)
- `app/lib/presentation/viewmodels/time_modal_view_model.dart` (add fields, load from API)
- `app/lib/data/repositories/experience_repository.dart` (expose `timePollActive`
  from API response)

**Tests**:
- Toggle shows both tabs; "Set time" tab renders `ProposeTimeForm`.
- Toggle is locked when `timePollActive` is true.
- `TimeModalData.timePollActive` is populated from server response.

**Estimated scope**: ~100-150 lines changed + ~60-80 lines test code

---

### Phase 2: `PollOptionsBuilder` Widget ("Ask the Group" Tab)

**Goal**: Owner can build poll options using day chip + time-of-day grid.

**Changes**:
- **New `PollOptionsBuilder` widget**: `StatefulWidget` (local state exception
  for form inputs, per project convention). Displays:
  - Current options list (numbered cards with remove button, matches
    [time-ask2.png](../../tmp/time2/time-ask2.png)).
  - "Quick add" day chips row (4 days: today, tomorrow, and two contextual
    upcoming dates derived from the experience's event time or current date).
  - Time-of-day buttons (Morning 8 AM / Afternoon 1 PM / Evening 5 PM) —
    appear after a day chip is selected. Each button is a toggle: tap to add
    that day+time combination as an option (shown checked in sage green); tap
    again to remove it. Count badge on day chip shows how many options exist
    for that day.
  - "Add a specific date & time" button (dashed border): expands an inline
    mini-form with separate date and time inputs side by side.
  - At-max state (5 options): "Maximum of 5 options reached" message; quick add
    buttons disabled.
  - "Friends will vote on N options. You'll pick the winner." summary note.
- **Callbacks**: `onOptionsChanged(List<(DateTime, TimeOfDay)>)` passed up to
  the modal. The modal uses this list to enable/disable the CTA.
- **CTA bottom bar**: "Send Poll (N options)" when `mode == 'poll'` and options
  exist. Disabled state: "Add at least one option". Tapping calls
  `notifier.createTimePoll(options)` which fires `ProposeTime` for each option
  in parallel, then refreshes.

**New `TimeModalNotifier` method: `createTimePoll`**:
```dart
Future<void> createTimePoll(List<(DateTime, TimeOfDay)> options) async {
  // Fire all ProposeTime calls in parallel; fail fast if any call fails.
  // Use Future.wait() — if any call throws, the error propagates and is
  // surfaced to the user via SnackBar. Partial proposals may have been
  // saved on the server; the owner can cancel the poll to clean up.
  // (See Open Question Q5 — a single CreateTimePoll RPC would be cleaner
  // and atomic, avoiding partial state.)
  if (!ref.mounted) return;  // Guard before state updates
  state = const AsyncLoading();
  final result = await AsyncValue.guard(() async {
    await Future.wait(options.map((opt) => _repository.proposeTime(...)));
    return await _repository.getExperienceDetails(experienceId);
  });
  if (!ref.mounted) return;  // Guard after async gap
  state = result;
}
```

**Cache invalidation**: `ExperienceRepository.proposeTime()` must invalidate the
`experience:{id}` cache entry and the experience details cache key (e.g.
`experience:details:{id}`) so that subsequent reads from `TimeModalNotifier`,
`ConversationViewModel`, and the daily screen reflect the poll-active state.
The `ExperienceRepository` already owns mutations + invalidation together
(per caching.md Pattern 2) — confirm `proposeTime()` calls the right
`_cache.invalidate(...)` keys. Also notify `dailyCacheInvalidationProvider`
since the experience now has an active poll (if ExperienceRepository doesn't
already do this, add it here).

**Files modified**:
- New: `app/lib/presentation/widgets/experience/poll_options_builder.dart`
- `app/lib/presentation/screens/experience/time_modal.dart` (wire "Ask the group" tab to new widget)
- `app/lib/presentation/viewmodels/time_modal_view_model.dart` (add `createTimePoll`)
- `app/lib/data/repositories/experience_repository.dart` (confirm cache invalidation
  in `proposeTime()` covers experience details key)

**Tests**:
- `PollOptionsBuilder`: adding day chip + time-of-day button adds option to list.
- `PollOptionsBuilder`: at max (5), buttons disable.
- `PollOptionsBuilder`: remove button removes option from list.
- `TimeModalNotifier.createTimePoll`: calls `ProposeTime` N times, sets
  `timePollActive`; any single `ProposeTime` failure surfaces an error and
  does not silently leave partial state.
- `TimeModalNotifier.createTimePoll`: disposal safety — if the provider is
  disposed while `ProposeTime` calls are in flight, the method completes
  without crashing (Riverpod `AsyncValue.guard` + `ref.mounted` checks).

**Estimated scope**: ~300-400 lines new/changed + ~100-120 lines test code

---

### Phase 3: Poll Results View (Owner, Post-Submit)

**Goal**: After submitting a poll, owner sees results and can confirm the winner.

**Changes**:
- **Results list view**: When `timePollActive == true` and user is organizer,
  show a results list (replaces the current unified card list):
  - Each option: day + time string (bold) + voter avatar stack (20px) + vote
    count badge. Sorted by vote count descending.
  - "Everyone can make it" sage green indicator when all voted YES.
  - Organizer can tap "Select" on an option (coral border) then tap "Confirm"
    in the bottom bar to finalize via `ConfirmTime` RPC.
  - Bottom bar: "Cancel poll" (grey outlined, left) + "Confirm" (sage solid,
    right, disabled until selection made).
- **"Cancel poll"** button: calls new `notifier.cancelTimePoll()` →
  `CancelTimePoll` RPC → resets `timePollActive`, clears proposals, returns to
  "Set time" tab (with confirmation dialog if votes exist). The method follows
  the `AsyncNotifier` async-safety pattern: `if (!ref.mounted) return` before
  the loading assignment and again after each await before any state write
  (per architecture requirement for methods that manually set state on
  `AsyncNotifier`).
- **Reuses**: Vote count logic and avatar stack from existing `TimeSlotCard`.
  The "Confirm" path reuses existing `lockTime()` / `ConfirmTime` RPC.

**Cache invalidation**: `ExperienceRepository.cancelTimePoll()` must invalidate
`experience:{id}` and `experience:details:{id}` (same as `proposeTime`) since the
poll state is cleared. Notify `dailyCacheInvalidationProvider` to keep the daily
screen fresh.

**Files modified**:
- `app/lib/presentation/screens/experience/time_modal.dart` (add results view)
- `app/lib/presentation/viewmodels/time_modal_view_model.dart` (add `cancelTimePoll`)
- `app/lib/data/repositories/experience_repository.dart` (add `cancelTimePoll`
  method with cache invalidation)

**Tests**:
- Results view renders when `timePollActive && isOrganizer`.
- "Cancel poll" calls `CancelTimePoll` RPC via repository, invalidates cache.
- "Confirm" with selected option calls `ConfirmTime`.
- `TimeModalNotifier.cancelTimePoll`: disposal safety — provider disposed mid-
  call does not crash (ref.mounted guard before each state assignment).

**Estimated scope**: ~200-250 lines changed + ~80-100 lines test code

---

### Phase 4: `VoteCard` for Non-Owners

**Goal**: Non-owners see a clean vote card when a poll is active.

**Changes**:
- **New `VoteCard` widget**: `ConsumerWidget` (architecture rule: widgets
  never access repositories directly — all actions go through ViewModels).
  Opens as a full-screen bottom sheet (widget async exception: bottom sheet is
  a UI-level concern). Matches [vote-card.jsx](../../tmp/time2/vote-card.jsx)
  and [vote-card.png](../../tmp/time2/vote-card.png):
  - Context row: owner avatar + "[Owner] is finding a time for [Experience]"
  - Heading: "Tap times you can make it" + "Pick all that work."
  - Slot list: checkbox (sage green when selected) + day string + time-of-day +
    voter avatar stack. Tapping toggles the selection.
  - Footer: "None of these work for me" text link (before any selection) +
    CTA button ("Pick a time" disabled / "Submit" active / "Can't Make It" if
    none selected).
  - `VoteCard` reads `timeModalProvider(experienceId)` via `ref.watch()` to
    display current proposals and vote state. Vote actions call
    `ref.read(timeModalProvider(experienceId).notifier).voteOnTime(...)` —
    the existing `TimeModalNotifier.voteOnTime()` method. No new ViewModel
    or repository method is needed.
  - Submission: calls `notifier.voteOnTime(id)` (YES) for each selected slot,
    then closes. "Can't Make It" closes without any server call (per Option A
    from Open Question 1).
  - Post-submission confirmation: inline success state ("You're in!" + sage
    checkmark, or "Maybe next time!" if Can't Make It). Auto-close after 2s.
- **`VoteCard.show(context, experienceId)`**: Static helper opens it as a bottom
  sheet (same pattern as `TimeModal.show()`).
- **Routing logic in `WhenRow` / content view**: If `timePollActive && !isOwner`
  → tap opens `VoteCard`. If `!timePollActive && !isOwner` → no tap target (or
  show the finalized/TBD state read-only).

**Files modified**:
- New: `app/lib/presentation/widgets/experience/vote_card.dart`
- `app/lib/presentation/widgets/experience/when_row.dart` (route to VoteCard vs TimeModal)
- `app/lib/presentation/screens/experience/event_joiner_modal.dart` (open VoteCard from time row)

**Tests**:
- `VoteCard`: tapping slot calls `TimeModalNotifier.voteOnTime` via notifier,
  not repository directly.
- `VoteCard`: "Submit" button disabled until at least one slot selected.
- `VoteCard`: "None of these work" shows "Can't Make It" CTA, submits no votes.
- Routing: `timePollActive && !isOwner` → `VoteCard`; otherwise → `TimeModal` or read-only.

**Estimated scope**: ~350-450 lines new + ~120-150 lines test code

---

### Phase 5: WhenRow Polish + Chat Integration Updates

**Goal**: WhenRow reflects poll state; chat-wf-exp surfaces updated accordingly.

**Changes**:
- **WhenRow — poll state**: When `timePollActive`:
  - Non-owner: coral calendar icon, "Poll active · N options", "Tap to vote"
    subtitle. Taps open `VoteCard`.
  - Owner: same icon, "Poll active · N votes so far", subtitle shows leading
    option if one exists.
- **WhenRow — fixed time**: Keep existing locked/unlocked display (no changes
  to existing logic).
- **Chat manage menu (owner)**: "Change time" in the manage menu opens the
  full `TimeModal` (two-tab, poll results if poll is active). No change to
  the "Change time" action itself.
- **Chat manage menu (joiner)**: Remove "Propose a time" option entirely
  (contradicts new model). Replace with "Vote on times" (visible only when
  `timePollActive`), which opens `VoteCard`. When no poll is active, joiner
  manage menu has only "Change RSVP".
- **Chat time voting card**: Remove "Propose another time" button from the
  inline `TimeVotingCard` in chat. The card continues to show vote results
  with voter avatars and togglable checkboxes, but without a propose action.
  Update to use checkbox UX from `vote-card.jsx` (replace the existing
  `TimeSlotCard` tap-to-toggle-yes style).

**Files modified**:
- `app/lib/presentation/widgets/experience/when_row.dart` (poll state display)
- `app/lib/presentation/widgets/chat/manage_event_panel.dart` (remove "Propose a time", add "Vote on times")
- `app/lib/presentation/widgets/chat/time_voting_card.dart` (remove propose button, update to checkbox UX)

**Tests**:
- WhenRow: `timePollActive` shows "Poll active" text for non-owner with tap routing to VoteCard.
- WhenRow: `timePollActive` for owner shows vote count.
- Manage menu: "Propose a time" not shown for joiner at any phase.
- Manage menu: "Vote on times" shown for joiner only when `timePollActive`.

**Estimated scope**: ~150-200 lines changed + ~60-80 lines test code

---

### Phase 6: Experience Preview — Time Display Overhaul

**Goal**: Replace the old time modal in the experience preview with a
streamlined inline time editor for owners. Polls are only created through an
explicit "Ask the Group" action. Remove "Propose" and "Finalize" buttons —
the time is always editable by the owner without mode ceremony.

**Problems addressed**:
- Tapping the time in the experience preview currently opens the old time
  modal (the full `TimeModal` with propose/finalize workflow). This is
  confusing because the preview context is not the right place for the
  heavyweight modal — the owner just wants to see or adjust the time.
- A poll card appears in the chat as soon as any time proposal exists, even
  when the owner hasn't explicitly asked the group. This creates noise and
  confusion for participants who see a poll they weren't invited to.
- The "Propose" and "Finalize" buttons imply a multi-step ceremony that
  doesn't match the simplified model: the owner sets the time, and it's
  immediately the time. If they want group input, they explicitly create a
  poll via "Ask the group."

**Changes**:
- **Experience preview time tap**: When the owner taps the time in the
  experience preview (content view), instead of opening the old `TimeModal`,
  show an inline time editing form directly in the preview. The form should:
  - Display the current time prominently at the top (formatted date + time +
    duration, matching the existing `WhenRow` display style).
  - Below the current time, show the "Set time" form (reuse the existing
    `ProposeTimeForm` date chips + time chips + duration chips). The owner
    can change the time directly — saving calls the existing time update
    path.
  - Below the form, show an "Ask the group" button that navigates to the
    "Ask the group" tab of the full `TimeModal` (Phase 2's
    `PollOptionsBuilder`). This is the only way to create a poll.
  - No "Propose" button, no "Finalize" button. The time is the time. The
    owner can change it at any point by editing inline.
- **Non-owner experience preview time tap**: Non-owners see the current time
  read-only. If a poll is active, tapping opens the `VoteCard` (Phase 4
  behavior, unchanged).
- **Chat poll card gating**: Do NOT show a time poll card in the chat
  conversation until the owner has explicitly created a poll through "Ask the
  group" (i.e., `time_poll_active == true`). Currently, any `TimeProposal`
  record triggers the poll card — change the rendering condition from
  "proposals exist" to "`time_poll_active == true`". This means:
  - Owner sets/changes time via the inline form → no poll card in chat.
  - Owner explicitly taps "Ask the group" and submits poll options → poll
    card appears in chat for all participants.
- **Remove "Propose" and "Finalize" buttons**: Remove these from the
  experience preview's time section. The old workflow assumed a
  propose → vote → finalize pipeline. The new model is:
  - Owner sets time (immediate, no ceremony).
  - Owner can optionally create a poll ("Ask the group") to solicit
    preferences, then pick a winning time from poll results.
  - There is no "Finalize" step — confirming a poll winner via the results
    view (Phase 3) is the only finalization flow, and it lives in the
    `TimeModal`, not the preview.

**Files modified**:
- `app/lib/presentation/screens/experience/experience_content_view.dart` (or
  equivalent preview screen): replace time modal launch with inline time
  editor for owner; keep `VoteCard` routing for non-owner with active poll
- `app/lib/presentation/widgets/experience/when_row.dart`: remove "Propose"
  and "Finalize" action buttons; owner tap opens inline editor instead of
  `TimeModal`
- `app/lib/presentation/widgets/chat/time_voting_card.dart`: gate rendering
  on `time_poll_active == true` instead of "proposals exist"
- `app/lib/presentation/screens/experience/time_modal.dart`: remove
  "Propose" and "Finalize" buttons if they exist here; ensure the modal is
  only reachable for the "Ask the group" flow or poll results

**Tests**:
- Owner taps time in preview → inline editor shown (not old `TimeModal`).
- Inline editor shows current time at top, `ProposeTimeForm` below.
- "Ask the group" button in inline editor navigates to poll creation flow.
- No "Propose" or "Finalize" buttons rendered in preview or `WhenRow`.
- Chat poll card not shown when `time_poll_active == false`, even if
  `TimeProposal` records exist.
- Chat poll card shown when `time_poll_active == true`.
- Non-owner tap on time in preview opens `VoteCard` when poll active,
  shows read-only time otherwise.

**Estimated scope**: ~200-300 lines changed + ~80-120 lines test code

---

### Phase 7: Time Modal — Two-Button Layout + Chat Planning Mode

**Goal**: Replace the two-tab segmented control in the owner time modal with a
cleaner two-button bottom bar ("Update" and "Ask the Group"). Show the
currently set time at the top of the modal in both modes. Remove the "Lock
time" stage from the chat manage menu, but keep the experience in planning mode
when a poll is open.

**Problems addressed**:
- The segmented toggle at the top of the modal (from Phase 1) adds visual
  overhead before the owner has even decided what they want to do. Tabs
  imply persistent mode switching; in practice the owner usually just wants
  to adjust the time or kick off a poll — two distinct actions better
  expressed as bottom-bar buttons than a top toggle.
- The "currently set time" is not visible at the top of the modal, so the
  owner has no reference point when deciding whether to update the time or
  start a poll.
- The chat manage menu has a "Lock time" stage that conflicts with the
  simplified model (no propose/finalize ceremony). However, when a poll is
  open the experience should still be visibly in a "planning" state so
  participants understand that the time is not yet decided.

**Changes**:
- **Modal layout overhaul**: Replace the segmented toggle with a single
  content area that shows either the time form or the poll options builder.
  The modal always opens showing the current time at the top, followed by
  the time-editing form (reuse `ProposeTimeForm`). Two buttons in the bottom
  bar replace the single CTA:
  - **"Update"** (sage, right): saves the current form values as the
    experience time using the existing time update path. Disabled if the
    form is unchanged.
  - **"Ask the Group"** (outlined, left): switches the modal body to the
    `PollOptionsBuilder` (the "Ask the Group" view from Phase 2). The
    "Update" button is replaced by "Send Poll (N options)" in this mode.
    A back arrow or "← Back" text link returns to the time form.
- **Current time header**: Always show the currently set time (or "No time
  set yet" if TBD) at the top of the modal, above both the time form and
  the poll options builder. Matches the WhenRow locked-time display style:
  formatted date, time, and duration on a single line with a coral calendar
  icon. This gives the owner context in both modes.
- **"Ask the Group" body**: When the owner taps "Ask the Group", the modal
  body swaps to the `PollOptionsBuilder` widget (Phase 2). The design
  matches [time-ask2.png](../../tmp/time2/time-ask2.png) — day chip row,
  time-of-day buttons, option list with remove buttons, and the "Add a
  specific date & time" inline expander. The current time header remains
  visible above the poll builder. CTA changes to "Send Poll (N options)".
- **"Lock time" removal from chat manage menu**: Remove the "Lock time"
  action from the owner's chat manage menu. Under the new model there is no
  separate "lock" step — the owner either sets a time (always effective
  immediately) or confirms a poll winner (from the results view in Phase 3).
  The "Change time" action in the manage menu opens the updated modal
  (time header + form + "Update" / "Ask the Group" buttons).
- **Planning mode gating on `time_poll_active`**: The experience should
  remain in (or enter) "planning" mode in the chat UI when a poll is open,
  so participants see that the time is still being decided. Specifically:
  - If `time_poll_active == true`, treat the experience as "time not yet
    confirmed" for chat UI purposes (same visual state as when no time is
    set). The chat header and any time summary row should indicate the time
    is being voted on.
  - If `time_poll_active == false` and a time is set, the experience is
    past the planning stage — no planning-mode indicators.
  - This replaces the "Lock time" stage: instead of the owner explicitly
    locking, the absence of an active poll is the implicit lock.

**Files modified**:
- `app/lib/presentation/screens/experience/time_modal.dart`: replace
  segmented toggle with two-button bottom bar; add current time header;
  add "Ask the Group" / back navigation; remove tab state
- `app/lib/presentation/viewmodels/time_modal_view_model.dart`: replace
  `timeMode` tab state with a simpler `isAskingGroup: bool` local flag;
  remove toggle-locked logic
- `app/lib/presentation/widgets/chat/manage_event_panel.dart`: remove
  "Lock time" action; keep "Change time" (opens updated modal)
- `app/lib/presentation/widgets/chat/` (chat header / time summary):
  gate planning-mode display on `time_poll_active` rather than a lock flag

**Tests**:
- Modal opens with current time header visible in both "Set time" and
  "Ask the Group" modes.
- "Ask the Group" button switches body to `PollOptionsBuilder`; back
  navigation returns to time form.
- "Update" button is disabled when form is unchanged; enabled after edit.
- CTA in poll mode reads "Send Poll (N options)"; disabled with zero options.
- "Lock time" is not present in owner chat manage menu.
- Chat planning-mode indicator shown when `time_poll_active == true`;
  hidden when `time_poll_active == false` and time is set.

**Estimated scope**: ~200-280 lines changed + ~80-100 lines test code

---

### Phase 8: System Messages, Vote Card Polish, Poll Results Refresh + Menu Updates

**Goal**: Emit system chat messages at the right moments (time changed, poll
opened), align the in-chat vote card to the reference design, auto-refresh
poll results when the owner opens the modal, unify the manage menu "Vote"
entry across roles, and rename "Cancel Poll" → "End Poll" with a graceful
post-confirmation flow.

**Problems addressed**:
- No system message is emitted when the owner changes the time or starts a
  poll — participants have no in-chat signal that something changed.
- The in-chat vote card does not match the reference design
  ([vote-card.png](../../tmp/time2/vote-card.png)): items need a white
  background card, the date and time should be on separate rows within each
  option row, and voter avatars should be right-justified.
- When the owner opens the time modal from the chat manage menu with an
  active poll, the results shown may be stale — the owner has to tap a slot
  to trigger a refresh. The modal should load fresh poll data on open.
- The poll results view (owner-side, Phase 3) has a different layout from
  the vote card. It should mirror the vote card's visual design so both
  views look consistent, minus the voting affordance (owner cannot vote in
  the results view).
- The manage menu "Vote" entry (Phase 5) is only shown for joiners. If the
  owner has an open poll they should also see it in their menu (opening the
  poll results view, not the vote card).
- "Cancel Poll" is a destructive label. "End Poll" is gentler and more
  accurate. After the owner confirms, rather than returning to an empty
  modal state they should land directly on the time edit form with the
  current time prefilled so they can immediately update the time if needed.

**Changes**:

- **System messages — time changed**: When the owner saves a new time via
  "Update" (the inline form), the server emits a system message to the
  experience chat: "[Owner] changed the time to [formatted datetime]." This
  reuses the existing system message infrastructure. One message per save —
  not one per field edit, not one per poll option.
- **System messages — poll opened**: When the owner submits "Send Poll
  (N options)", the server emits a system message: "[Owner] is asking the
  group when to meet. Tap to vote." This is the trigger that causes the poll
  card to appear in chat (alongside `time_poll_active = true` from Phase 0).
  No system message is emitted for individual poll option additions or when
  the owner views/taps results.
- **System messages — poll ended**: When the owner confirms "End Poll" and
  the `CancelTimePoll` RPC fires, the server emits: "[Owner] ended the time
  poll." No message is emitted when the owner merely views results or
  navigates through the modal.
- **System messages — time confirmed from poll**: When the owner confirms a
  winning time from the poll results (calls `ConfirmTime`), the existing
  time-confirmed system message should fire (or be added if absent).

- **Vote card visual update** (in-chat `TimeVotingCard`): Align to
  [vote-card.png](../../tmp/time2/vote-card.png):
  - Each option row sits on a white rounded card (elevated or with a subtle
    border), not a flat row on the sheet background.
  - Date and time are on **separate rows** within the card: date on top
    (e.g. "Thursday, March 6"), time below (e.g. "Morning · 8:00 AM").
  - Voter avatar stack is **right-justified** within the card row (not
    left of the time text). Align avatars to the trailing edge.
  - Checkbox remains on the leading edge (for voters); for the owner results
    view there is no checkbox — replace with a leading vote-count badge or
    simply rely on the avatar stack.
  - Apply the same card layout to both the in-chat `TimeVotingCard` and the
    owner poll results view (Phase 3) so they are visually consistent.

- **Auto-refresh poll results on modal open**: When the owner opens the time
  modal (from the chat manage menu or experience preview) and `time_poll_active
  == true`, `TimeModalNotifier.build()` must always fetch fresh proposal +
  vote data from the repository, bypassing any in-memory cache. Currently
  the notifier may return cached state. Fix: in `build()`, when
  `timePollActive` is true, call `_repository.getExperienceDetails(id,
  forceRefresh: true)` (or invalidate the cache before reading). This
  ensures the owner always sees current vote counts without a manual tap.

- **Manage menu "Vote" for owner**: The "Vote on times" action in the
  manage menu (Phase 5) is currently shown only to joiners. Add it to the
  **owner's** manage menu as well when `time_poll_active == true`, but open
  the **poll results view** (Phase 3) rather than the `VoteCard`. The label
  can remain "Vote on times" or be "See poll results" — either works since
  the modal makes the owner-vs-voter distinction clear internally.

- **"End Poll" rename + post-confirmation flow**: Rename the "Cancel poll"
  button in the owner modal to **"End Poll"**. Tapping shows a confirmation
  dialog ("End the time poll? Existing votes will be lost."). On confirm:
  1. Call `CancelTimePoll` RPC (unchanged server behavior).
  2. Instead of returning to a blank modal, transition the modal body to the
     time edit form (same as the default "Set time" view from Phase 7) with
     the experience's **current time prefilled** in `ProposeTimeForm`. The
     owner can immediately adjust and tap "Update", or dismiss.
  3. The bottom bar shows "Update" and "Ask the Group" (normal Phase 7
     layout), not a disabled state.

**Server changes**:
- Emit system message on `ProposeTime` (owner, first call that sets
  `time_poll_active`) — "asking the group" message.
- Emit system message on time update (existing `UpdateTime` / time-set path)
  — "changed the time" message.
- Emit system message on `CancelTimePoll` — "ended the poll" message.
- Confirm system message exists for `ConfirmTime` — add if absent.
- No system message on subsequent `ProposeTime` calls (options added to an
  already-active poll), on `VoteOnTime`, or on reads.

**Files modified**:
- `server/services/experience/time_proposals.go`: emit system messages at
  the right call sites
- `server/services/experience/` (time update handler): emit system message
  on time change
- `app/lib/presentation/widgets/chat/time_voting_card.dart`: white card per
  option, date+time split rows, right-justified avatars
- `app/lib/presentation/screens/experience/time_modal.dart`: rename "Cancel
  poll" → "End Poll"; post-confirmation flow transitions to prefilled time
  form; auto-refresh on open when poll active; owner manage menu "Vote"
  entry routes to results view
- `app/lib/presentation/viewmodels/time_modal_view_model.dart`: force-
  refresh in `build()` when `timePollActive`; handle "End Poll" →
  prefilled form transition
- `app/lib/presentation/widgets/chat/manage_event_panel.dart`: add "Vote
  on times" / "See poll results" to owner menu when poll active

**Tests**:
- `ProposeTime` (first option, owner) → system message emitted with
  "asking the group" text.
- `ProposeTime` (second option, same poll) → no additional system message.
- Time update → system message emitted with new formatted time.
- `CancelTimePoll` → "ended the poll" system message emitted.
- Vote card: each option renders as a white card with date and time on
  separate rows; avatar stack is right-aligned.
- Modal open with `timePollActive == true` → data fetched fresh (not from
  cache).
- "End Poll" button labeled "End Poll" (not "Cancel poll"); confirmation
  dialog shown before firing RPC.
- After "End Poll" confirmed, modal shows time edit form with current time
  prefilled.
- Owner manage menu includes "Vote on times" / "See poll results" when
  `timePollActive == true`; opens results view, not `VoteCard`.

**Estimated scope**: ~250-350 lines server + client changed + ~100-130 lines test code

---

### Phase 9: Remaining Bugs + Polish

**Goal**: Fix outstanding issues discovered during implementation of earlier
phases: the experience preview still opens the old time modal on time tap,
"Send Poll" carries a redundant option count, the server emits one system
message per poll option instead of one total, the poll card appears above the
system message instead of below it, the vote card layout doesn't fully match
the reference design, and "Change time" in the owner menu doesn't consistently
show the time edit form.

**Problems addressed**:

- **Phase 6 regression still present**: Tapping the time in the experience
  preview still opens the old `TimeModal` instead of the new inline time
  editor (Phase 6 intent). Whatever code path routes the tap needs to be
  found and corrected.
- **"Send Poll" button label**: The CTA reads "Send Poll (N options)".
  Remove the option count — the label should just be "Send Poll". The
  builder UI already shows the option count in the list; repeating it in the
  button is redundant.
- **One system message per poll option instead of one total**: When the owner
  submits a poll with N options, the server currently emits N system messages
  ("asking the group when to meet") — one per `ProposeTime` call. There should
  be exactly **one** system message for the entire poll creation. Fix: emit
  the "asking the group" system message only when `time_poll_active` transitions
  from `false` to `true` (i.e., only on the first `ProposeTime` call that
  sets the flag), not on subsequent calls for the same poll.
- **Poll card ordering in chat**: The poll card currently appears above the
  system message ("asking the group") in the chat thread. It should appear
  **below** the system message — the system message announces the poll, the
  card follows. This is likely a rendering/ordering issue in how the chat
  stream interleaves system messages with experience state cards.
- **Vote card doesn't fully match reference design**: The in-chat vote card
  (and poll results view) still deviates from
  [vote-card.png](../../tmp/time2/vote-card.png). Specifically: the total
  number of votes is missing — the reference shows a vote count next to the
  avatar stack on the right side of each option row. Audit the full reference
  design and close any remaining gaps.
- **"Change time" in owner menu**: "Change time" in the owner's manage menu
  currently routes to the poll results view when a poll is active. It should
  **always** open the time edit form (Phase 7 layout: current time header +
  `ProposeTimeForm` + "Update" / "Ask the Group" buttons), regardless of poll
  state. If the owner wants to see poll results, they use the separate "See
  poll results" / "Vote on times" menu item (Phase 8). "Change time" is
  specifically for editing the time, not reviewing votes.

**Changes**:

- **Fix experience preview time tap**: Identify the tap handler on the time
  row in the experience preview / content view that still calls
  `TimeModal.show()` and redirect it to the inline time editor introduced in
  Phase 6. Ensure this path is hit for owners in both the preview sheet and
  the experience content view.

- **"Send Poll" label**: Change the CTA string from `"Send Poll ($n options)"`
  to `"Send Poll"` in `PollOptionsBuilder` and wherever the string is
  assembled in `time_modal.dart`.

- **Single system message for poll creation**: In `time_proposals.go`, move
  the system message emission so it fires only once — when `time_poll_active`
  changes from `false` to `true`. Subsequent `ProposeTime` calls on an already-
  active poll must not emit another message.

- **Poll card ordering**: Investigate how the chat message list interleaves
  system messages with the experience time proposal card. The poll card should
  be rendered after (below) the system message in the chat stream. Fix the
  ordering logic — likely in the message list assembly in the conversation
  ViewModel or the chat list widget.

- **Vote card total vote count**: Add a total vote count label next to the
  avatar stack on each option row in both the in-chat `TimeVotingCard` and the
  owner poll results view. Match the reference design exactly: avatars
  right-justified, vote count as a small number to the left of or adjacent to
  the avatar stack. Do a full pixel-level audit of
  [vote-card.png](../../tmp/time2/vote-card.png) and close any remaining
  layout gaps (spacing, typography, border radius, checkbox style, etc.).

- **"Change time" always opens time edit form**: In `manage_event_panel.dart`,
  the "Change time" action must unconditionally open the time modal in "Set
  time" mode (Phase 7 default: time header + form + "Update" / "Ask the Group"
  buttons). Remove any conditional that routes to poll results when
  `time_poll_active == true`. Poll results are accessible via the separate
  "See poll results" menu item.

**Files modified**:
- `app/lib/presentation/screens/experience/` (preview / content view): fix
  time tap routing for owner to use inline editor
- `app/lib/presentation/widgets/experience/poll_options_builder.dart`: remove
  option count from "Send Poll" CTA label
- `app/lib/presentation/screens/experience/time_modal.dart`: same label fix
  if assembled here; fix "Change time" routing to always open time edit form
- `server/services/experience/time_proposals.go`: emit system message only
  on `time_poll_active` transition, not per option
- `app/lib/presentation/widgets/chat/` (chat message list or conversation
  view model): fix poll card / system message ordering
- `app/lib/presentation/widgets/chat/time_voting_card.dart`: add total vote
  count per option row; full audit against reference design
- `app/lib/presentation/widgets/chat/manage_event_panel.dart`: "Change time"
  always opens time edit form

**Tests**:
- Owner taps time in experience preview → inline editor opens (not old
  `TimeModal`). Regression test for Phase 6 fix.
- "Send Poll" button label is exactly "Send Poll" with no option count.
- `ProposeTime` with N options (owner, new poll) → exactly one system message
  emitted total.
- `ProposeTime` adding a second option to an existing poll → zero additional
  system messages.
- Chat message list: system message ("asking the group") appears before
  (above) the poll card in rendering order.
- Vote card: each option row shows a vote count adjacent to the right-
  justified avatar stack.
- "Change time" in owner manage menu → time edit form shown; poll results
  view not shown regardless of `time_poll_active`.

**Estimated scope**: ~150-250 lines changed + ~80-100 lines test code

---

## Architecture Compliance Notes

- **Owner-only enforcement is server-side**: Client UI hides the propose form
  from non-owners, but the server `ProposeTime` permission check is the
  authoritative gate. Defense in depth.
- **`PollOptionsBuilder` is a `StatefulWidget`**: Acceptable exception per
  project convention (`ProposeTimeForm` already uses this pattern for local
  form input state; architecture doc lists form input state as a permitted
  use of `StatefulWidget`/`setState()`).
- **`VoteCard` is a `ConsumerWidget` that goes through `TimeModalNotifier`**:
  The architecture rule "ViewModels are the ONLY layer that calls Repositories
  — Screens/Widgets never access repositories directly" means `VoteCard` cannot
  call `ExperienceRepository` directly. It reads `timeModalProvider(experienceId)`
  and calls `ref.read(...notifier).voteOnTime(...)`. No new ViewModel or
  repository method is needed.
- **`VoteCard.show()` is an acceptable widget async exception**: Opening a
  bottom sheet (`showModalBottomSheet`) is explicitly listed in the architecture
  as a widget-level async operation. A single `if (!context.mounted) return`
  guard after the await is sufficient.
- **`createTimePoll` uses `Future.wait()` with fail-fast semantics**: Multiple
  `ProposeTime` calls run in parallel; any failure propagates and is surfaced
  to the user. Partial proposals may be left on the server — acceptable in
  Phase 2 since the owner can cancel the poll. A single atomic `CreateTimePoll`
  RPC (Open Question Q5) would eliminate partial state.
- **New async `TimeModalNotifier` methods require `ref.mounted` guards**: The
  architecture docs note that `build()` auto-handles disposal in `AsyncNotifier`,
  but other methods that manually assign `state =` still need `if (!ref.mounted)
  return` after every `await`. This applies to `createTimePoll` and
  `cancelTimePoll`.
- **Cache invalidation is the repository's responsibility**: Both `proposeTime()`
  and the new `cancelTimePoll()` in `ExperienceRepository` must invalidate the
  experience details cache key and notify `dailyCacheInvalidationProvider`.
  ViewModels call repository methods and rely on the repository to handle
  invalidation — they do not touch the cache directly.
- **Proto API/models separation preserved**: `time_poll_active` is added to
  both `models.Experience` (storage) and `api.ExperienceDetails` (API response)
  as separate fields. The service assembly layer copies the value — the two
  proto packages do not import each other.
- **`CancelTimePoll` requires its own RPC**: Cannot reuse `UnlockTime` because
  unlock is about unconfirming a locked time, while cancel removes all poll
  proposals and resets `time_poll_active`. Different semantics, different server
  behavior.
- **SQL migration is additive**: `time_poll_active boolean NOT NULL DEFAULT false`
  adds a column with a safe default. No data loss, no backfill required.
