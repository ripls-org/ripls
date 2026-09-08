---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Client time proposals and scheduling — one morphing TimePollSheet that cross-fades a TBD/propose, voting, and finalized body, all sharing one TimeModalNotifier; the propose/vote/confirm data flow with cache invalidation; and per-poll scoping by poll_id.
  globs: [app/lib/presentation/screens/experience/**, app/lib/services/experience/**, app/lib/presentation/viewmodels/time_modal_view_model.dart]
  triggers: [time, scheduling, time-poll, propose-time, poll-id, time-modal, finalization]
  lens: [client, domain]
  domain: client
freshness:
  verified_commit: "27f296427"
  verified_on: "2026-06-30"
---
# Time Proposals & Scheduling

Business logic for time proposals, voting, and finalization in the experience
time flow.

## Overview

The time-poll flow is a single **morphing entry sheet**,
`TimePollSheet`
([time_poll_sheet.dart](../../app/lib/presentation/screens/experience/time_poll_sheet.dart)).
One `GlassSheet` stays open and its body cross-fades between three status
bodies as the experience's poll state changes — so "Ask the group" or "Set the
final time" morph the sheet in place instead of closing it and pushing a
sibling sheet. There is no longer a `UnifiedTimeModal` or a tabbed surface.

The morph is driven off the coarse `TimeModalData.pollStatus` getter:

| `pollStatus` | When | Body (rendered `embedded: true`) | File |
|--------------|------|----------------------------------|------|
| `tbd` | No confirmed time and no active poll (default) | `TimePollProposeModal` | [time_poll_propose_modal.dart](../../app/lib/presentation/screens/experience/time_poll_propose_modal.dart) |
| `poll` | A poll is active (`time_poll_active`) | `TimePollVoteModal` | [time_poll_vote_modal.dart](../../app/lib/presentation/screens/experience/time_poll_vote_modal.dart) |
| `set` | A time is confirmed and no poll is running | `TimePollFinalizedModal` | [time_poll_finalized_modal.dart](../../app/lib/presentation/screens/experience/time_poll_finalized_modal.dart) |

Each body takes an `embedded` flag. In `embedded` mode it renders chrome-less
(no `GlassSheet`, no self-dismiss on state transitions) so `TimePollSheet` can
host it; the same widgets keep their standalone `show()` entry points for
non-morphing callers (e.g. opening the finalized view from a chat banner). The
owner-only `TimePollConfirmModal`
([time_poll_confirm_modal.dart](../../app/lib/presentation/screens/experience/time_poll_confirm_modal.dart))
is reached from the vote modal's Manage menu — it is not one of the three
morph bodies.

State is managed by `TimeModalNotifier`
([time_modal_view_model.dart](../../app/lib/presentation/viewmodels/time_modal_view_model.dart)),
an `AsyncNotifier` family keyed by `experienceId`. Every surface reads
`timeModalProvider(experienceId)` via `ref.watch()`. `TimePollSheet` watches
only `pollStatus` (`.select(...)`) so votes and other intra-state changes
don't restart the cross-fade — each body watches the provider itself for its
own content.

## Data Flow

```
Sheet/body → TimeModalNotifier → ExperienceRepository → Server RPCs
                  ↓ (cache invalidation)
   experienceProvider ref.watch → ExperienceContentView / event-pane time row / PollBanner update
```

When a mutation runs (vote, propose, end poll, confirm time), the notifier
calls `ref.invalidateSelf()` which re-runs `build()` and invalidates the
experience repository cache. `build()` always calls `_repository.invalidate()`
before fetching so it picks up votes cast via other paths (e.g. the inline
chat voting card). Most mutations are **optimistic**: the method writes the
projected state into `state`, awaits the RPC, then `invalidateSelf()`s, and
rolls back to the previous `AsyncData` on failure.

## Per-Poll Scoping

Each poll run is uniquely identified by `poll_id`
([time.proto:88](../../proto/ripls/api/time.proto)), stamped on every
`TimeProposal` created during that run. The experience tracks the most recent
poll's id in `current_poll_id`
([experience.proto:140](../../proto/ripls/api/experience.proto)).

- When `ProposeTime` is called and `time_poll_active` is **false**, the server
  mints a fresh id, sets it on the experience as `current_poll_id`, marks
  `time_poll_active = true`, clears `time_poll_completed`, seeds a default
  reply-by deadline, and stamps the new proposal with that id.
- Subsequent `ProposeTime` calls during the same poll reuse the existing
  `current_poll_id`.
- `CancelTimePoll` clears `time_poll_active` and sets `time_poll_completed =
  true`. **Proposals are preserved** so the read-only results view still has
  data.
- A new poll started after one ends mints a brand-new `poll_id`. Historical
  polls' proposals stay in storage tagged with their original ids, so the
  client can scope each surface to one poll.

`TimeModalData.currentPollProposals` filters `proposals` to those whose
`poll_id` matches `currentPollId` (falling back to all proposals when no poll
id is resolvable, for legacy data). The vote modal, the Plan-tab `PollBanner`,
and the first-tab pill color all scope their counts and "voted" checks to
`currentPollId` so a vote on a previous poll never inflates a new poll's
totals or flashes a false "voted" state.

## TimePollProposeModal (TBD / propose body)

The `tbd` body — `TimePollProposeModal`. The user stages one or more candidate
times before committing. Its title/subtitle/CTA shift on the visible count:

- **empty**: "PICK A TIME" eyebrow + a big "Add your first time" card and a
  `PollTbdCard` hint. The primary button is "Keep it TBD for now" (a no-op
  dismiss — friends can RSVP against a TBD time and the owner sets it later).
- **one**: "Lock this in?" with the staged row + an "Add another time"
  affordance. Primary CTA "Set the time" (`_ctaLabel` for count ≤ 1), with a
  softer "or float it to the group" link below.
- **many**: "Looks good." Primary CTA "Ask the group".

Times are staged via the system date+time picker (`DateTimePickerModal`), not
day/period chips. (`TimeModalNotifier.extractTimeCandidates`, which routes
free-form text through the `ExtractTimeCandidates` RPC for an LLM parse, exists
on the notifier and repository but is not currently wired into this modal's
UI.)

Commit paths:

- **One staged time** → `setSingleTime()`: calls `SaveExperience` directly
  (the same path as the explicit "edit time" flow) — not `ProposeTime + ConfirmTime`
  — so no transient poll is opened and no spurious "Poll ended" chat banner is
  written (#2598). The "or float it to the group" link instead calls
  `_floatToGroup()` → `createTimePoll()`.
- **Multiple staged times** → `createTimePoll()`: options are submitted
  **sequentially** to avoid the race where parallel `ProposeTime` calls each
  mint a new "poll opened" system message.
- When a poll is already live ("Edit Choices" path), existing proposals render
  from server state and each add fires immediately via
  `addTimesToActivePoll()` — no staged copy, no "Ask the Group" submit button.

A `_DeadlineStrip` lets the owner set/change the poll's reply-by deadline
(`SetTimePollDeadline`); until the server seeds the real deadline on first
proposal it shows a 24h-from-now default (matching the server's
`defaultTimePollDeadline`).

## TimePollVoteModal (voting / results body)

The `poll` body — handles both voting and read-only results depending on the
poll being viewed.

### Filtering by Poll

The constructor accepts an optional `pollId`. The modal computes:

- `effectivePollId = widget.pollId ?? data.currentPollId`.
- `visibleProposals` = proposals whose `pollId` matches the effective id
  (falls back to all proposals when no poll id is resolvable, for legacy data).
- `readOnly` = `!data.timePollActive` **OR** `effectivePollId !=
  data.currentPollId` (i.e. you're viewing an older poll's snapshot, not the
  live one).

### Live mode

- Title: "Cast your vote." / sub-label "TAP ALL THAT WORK".
- Each option is a tappable `TimeProposalTile` showing an avatar stack and YES
  vote count.
- **Immediate per-tap voting**: each tile tap commits a YES vote right away via
  `TimeModalNotifier.voteOnTime` (toggle — tap again sends `UNSPECIFIED` to
  clear). There is **no batch "Save" step**; the notifier owns optimistic state
  and rollback, and the widget disables a row while its RPC is in flight.
- A `PollFlexibleVoteRow` ("any time works") is shown to non-organizers. A
  flexible vote is stored as `FLEXIBLE` on a single representative proposal and
  folded into every option's effective tally
  (`TimeModalData.effectiveVoteCount`).
- An add-option row (hidden once proposals are locked) opens
  `TimePollProposeModal` for the live poll.
- Bottom bar: organizers see a single **Manage** button opening
  `TimePollManageMenuSheet` (add option, set final time, lock/unlock proposals,
  nudge unreplied, cancel poll); non-organizers see no bottom bar.

### Read-only / results mode

- Title: "Poll results." / sub-label "FINAL RESULTS".
- Tiles are non-interactive; the current-user filter is skipped so the
  historical voter list shows exactly as the server recorded it.
- The add-option row is hidden.
- Bottom bar collapses to "Close" — except that an owner who ended a poll
  without picking a time gets a "Pick the winning time" CTA that opens
  `TimePollConfirmModal`.

### Confirming a winner

The Manage menu's "set final time" and the read-only owner CTA both call
`_pickWinningTime()`, which opens `TimePollConfirmModal` (owner-only,
radio-select cards with a per-proposal vote breakdown). On a successful
confirm the embedded sheet morphs to the "It's Set" body on its own; the
standalone path pops and pushes `TimePollFinalizedModal`.

## TimePollFinalizedModal (the "It's Set" body)

The `set` body — read-only "It's a plan." view shown once a time is confirmed.
Surfaces the winning time on a sage-tinted `_WinnerCard` with an "N of M chose
this time" footer, a row of non-winning proposals deduped under "Other Times",
and an **Add to calendar** primary CTA (`CalendarHelper.addToCalendar`). The
winner resolves to the confirmed (`is_confirmed`) proposal, falling back to
whichever proposal matches the experience's `eventTime`; when the time was set
directly with no underlying proposal, `_WinnerCard.fromEventTime` renders from
`eventTime` with no vote footer.

Owners also get a **Change** action opening a `PollManageMenuSheet` with two
choices: **swap** the time directly (`DateTimePickerModal` →
`experienceProvider.updateTime`), or open a **fresh poll** (push
`TimePollProposeModal`; embedded, the sheet morphs underneath once a poll
opens).

## TimeModalNotifier mutations

Mutation methods on `TimeModalNotifier` and the RPC each drives:

| Method | RPC | Notes |
|--------|-----|-------|
| `voteOnTime(proposalId)` | `VoteOnTime` | Optimistic toggle YES ↔ UNSPECIFIED. |
| `setFlexibleOnTime(flexible)` | `VoteOnTime` | Records `FLEXIBLE` on the first current-poll proposal. |
| `proposeTime()` / `createTimePoll(options)` / `addTimesToActivePoll(options)` | `ProposeTime` | Sequential when proposing multiple options. |
| `setSingleTime(dt)` | `SaveExperience` | Direct-set path (one staged time → "Set the time" CTA); bypasses ProposeTime + ConfirmTime so no transient poll is opened and no spurious "Poll ended" chat banner is written (#2598). |
| `lockTime(proposalId)` / `finalizeEventTime()` / `selectProposal(proposalId)` | `ConfirmTime` (+ `SaveExperience`) | Confirms a proposal as the event time. |
| `unlockTime()` / `reopenVoting()` | `UnlockTime` | Clears the confirmed proposal. |
| `endTimePoll()` | `CancelTimePoll` | Proposals preserved for the results view. |
| `deleteTimeProposal(proposalId)` | `DeleteTimeProposal` | Owner deletes any; participants only their own. |
| `lockTimeProposals(locked)` | `LockTimeProposals` | Hides the add-option affordance when locked. |
| `setTimePollDeadline(unixSec)` | `SetTimePollDeadline` | `0` clears the deadline. |
| `nudgeTimePollVoters()` | `NudgeTimePollVoters` | Pings unreplied RSVPs; returns the count notified. |
| `extractTimeCandidates(text)` | `ExtractTimeCandidates` | LLM parse of free-form text into `ExperienceTime`s. |

All async methods that hand-set `state` guard with `if (!ref.mounted) return;`
after awaits (`build()` itself needs none — `AsyncNotifier` handles disposal).

## Entry Points

`TimePollSheet.show()` is the single opener, invoked from the experience
content view
([experience_content_view.dart](../../app/lib/presentation/screens/experience/experience_content_view.dart)).
The Plan-tab time row in
[experience_event_pane.dart](../../app/lib/presentation/screens/experience/widgets/experience_event_pane.dart)
routes its tap (and its `RowCallToActionPill`) to that opener, showing
"Choose a time" / a chosen-or-not pill while a poll is live.

## Plan-Tab & Chat Banners

- **`PollBanner`** ([poll_banner.dart](../../app/lib/presentation/widgets/planning/poll_banner.dart))
  — coral "Poll is live" banner with vote count and CTA, rendered in the event
  pane (and inline in chat). Flips to sage "Voted ✓" once the current user has
  cast at least one YES vote on the active poll. **Scoped to `currentPollId`**
  so historical poll data doesn't inflate counts or trigger a false "voted"
  state. Tapping opens the vote sheet.

### Inline Chat Banner

`ChatSystemAction.CHAT_SYSTEM_ACTION_TIME_PROPOSED` system messages render as
interactive cards in the chat tab via `PollBannerOrLabel`
([poll_banner_or_label.dart](../../app/lib/presentation/widgets/chat/message_list/poll_banner_or_label.dart)),
used by [message_list.dart](../../app/lib/presentation/widgets/chat/message_list.dart):

- **Poll active** → renders `PollBanner` inline. Tap opens the vote sheet.
- **Poll ended** → renders a sage "Poll ended · See results →" card
  (`_PollEndedChatBanner`). Tap opens the read-only results view.
- **No poll context** → falls back to the static system text label.

Each chat card carries the `poll_id` stamped on its message; a stale `poll_id`
(a newer poll exists) renders the card as ended even if the experience still
has an active poll. The "poll opened" system message is emitted by the server
**only the first time** `ProposeTime` flips `time_poll_active` from false to
true within a poll run; the client batches its options sequentially so parallel
requests can't race and produce duplicate messages.

## First-Tab Pill Color

The experience content view's first-tab active-pill color (`_firstTabColor` in
[experience_content_view.dart](../../app/lib/presentation/screens/experience/experience_content_view.dart))
is coral when the viewer still has pending actions (no RSVP, or an active poll
without a vote) and sage-green when everything is done. The `hasVoted` check is
**scoped to `currentPollId`** so a vote on a previous poll doesn't make the
pill go green for a newly-opened poll. (Owners and terminal experiences keep
the default accent.)

## Voting Mechanics

- **Tap to vote**: tapping a tile toggles a YES vote.
- **No explicit "No"**: tapping again removes the vote (sends `UNSPECIFIED`).
  The `TimeVoteStatus` enum still carries `NO` / `NONE_WORK` values for wire
  compatibility, but the client only writes `YES`, `FLEXIBLE`, and
  `UNSPECIFIED`.
- **Flexible**: a non-organizer can mark "any time works", recorded as
  `FLEXIBLE` on one representative proposal and folded into every option's
  effective tally.
- **Optimistic UI**: vote updates locally before the server round-trip and
  roll back on failure.

## Timezone Handling

All time creation and display uses the user's resolved timezone, never
hardcoded `'UTC'`.

### Resolution

`resolvedTimezoneProvider`
([user_timezone_provider.dart](../../app/lib/presentation/providers/user_timezone_provider.dart))
returns:

1. The user's preferred timezone (IANA, e.g. "America/New_York") if set in
   Settings.
2. The device's system timezone as the default fallback.

Every path that stamps a timezone on a `SpecificTime` proto reads from this
provider — `TimeModalNotifier`, `TimePollFinalizedModal`'s swap path, the
experience preview time picker, and `PortfolioInboxNotifier`.

### Display

`DateTimeFormatter.formatRelativeEventTimeWithTimezone()` converts event times
from the stored event timezone to the user's timezone for display.
`CalendarHelper.addToCalendar()` reads the timezone from the stored
`SpecificTime` and converts appropriately.

## RPCs

Defined on `ExperienceService`
([experience_service.proto](../../proto/ripls/api/experience_service.proto)).

| RPC | Purpose | Auth | Side effects |
|-----|---------|------|--------------|
| `ProposeTime` | Add a new time proposal | Any participant | Mints/uses `poll_id`. On the first option of a fresh poll: sets `current_poll_id`, sets `time_poll_active`, clears `time_poll_completed`, seeds a default deadline, emits a "poll opened" system chat message. |
| `VoteOnTime` | Cast or clear a vote (`YES`, `FLEXIBLE`, `UNSPECIFIED`) | Any participant | None |
| `ConfirmTime` | Lock a proposal as the event time | Owner only | Sets `is_confirmed` on the chosen proposal, clears `time_poll_active`. |
| `UnlockTime` | Unlock a previously confirmed time | Owner only | Clears the confirmed proposal. |
| `CancelTimePoll` | End the current poll | Owner only | Clears `time_poll_active`, sets `time_poll_completed`. **Proposals are preserved.** |
| `DeleteTimeProposal` | Remove a single proposal from the active poll | Owner (any) / participant (own only) | None |
| `SetTimePollDeadline` | Set/clear the reply-by deadline | Owner only | Sets `time_poll_deadline_unix_sec` (`0` clears). |
| `LockTimeProposals` | Freeze the option list | Owner only | Sets `time_proposals_locked`; hides the add-option affordance. |
| `NudgeTimePollVoters` | Push a reminder to unreplied RSVPs | Owner only | Returns the count notified. |
| `ExtractTimeCandidates` | LLM-parse free-form text into candidate times | Any participant | None (read-only); reachable via `TimeModalNotifier` but not yet wired into the propose UI. |

## Key Implementation Notes

- `TimeModalNotifier` is an `AsyncNotifier` family — one instance per
  `experienceId`.
- `build()` bails to an empty `TimeModalData` before any async work if the user
  is unauthenticated, to avoid writing state into a logout frame.
- All hand-`state`-setting async methods guard with `if (!ref.mounted) return;`
  after awaits.
- `ref.invalidateSelf()` refreshes from server after mutations and also
  invalidates the experience repository cache.
- Modal sizing: `ConstrainedBox(maxHeight: screen * 0.85)` + `Column(min)` +
  `Flexible(SingleChildScrollView(...))` so the body scrolls internally while
  the chrome stays pinned.
- `TimePollSheet` keys the cross-fade subtree off `pollStatus` so a vote
  doesn't restart the animation.
- The propose modal uses `StatefulWidget` for its local staged-times list
  (documented exception per architecture).
