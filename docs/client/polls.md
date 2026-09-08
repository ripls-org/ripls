---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Client experience polls — location and time polls letting attendees collectively decide an event detail; parallel lifecycle, propose/vote/manage/confirm/finalized surfaces, Event-tab routing, and the location/time parity contract.
  globs: [app/lib/presentation/screens/experience/**, app/lib/presentation/widgets/poll/**, app/lib/services/experience/**]
  triggers: [poll, location-poll, time-poll, vote, propose, finalize, parity]
  lens: [client, domain]
  domain: client
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Experience Polls (Client)

Experience polls let attendees collectively decide an event detail when the
organizer hasn't pinned it down. The app has two poll kinds:

- **Location polls** — "Where should we meet?"
- **Time polls** — "When should we meet?"

The two flows are intentionally parallel: same lifecycle, same surface
structure (entry / propose / vote / manage / confirm / finalized), same
chrome (sage-tinted "It's Set" cards, glass-modal sheets, Manage menu
rows), same routing logic on the Event-tab chip, and where reasonable the
same shared widgets. **New differences should be treated as bugs** —
parity is the design contract. See [Intentional differences](#intentional-differences)
for the narrow set of divergences that are deliberate, and [Deferred work](#deferred-work)
for the differences that remain only because the migration was bounded.

Both flows live under
[`presentation/screens/experience/`](../../app/lib/presentation/screens/experience/);
poll-kind-agnostic widgets live under
[`presentation/widgets/poll/`](../../app/lib/presentation/widgets/poll/).

---

## Lifecycle

A poll moves through three user-visible states. The same lifecycle applies
to both kinds.

```
  (no poll)
      │  organizer or eligible participant opens propose modal
      ▼
  ACTIVE  ── voters tap to vote (auto-submit YES toggle, optimistic)
      │  ── organizer manages: Edit Choices, Lock/Unlock, Nudge, Set Final, Cancel
      │  ── organizer picks winner via Confirm modal
      ▼
  FINALIZED  → "It's Set" sage card with primary CTA + owner Change menu
      │  ── organizer can swap directly or start a fresh poll
      ▼
  (loops back)
```

The same `experienceId` may host multiple polls over time (e.g. a
finalized poll, then organizer reopens). The Event-tab chip dispatches
to whichever sub-modal matches the current state; historical polls'
proposals stay grouped under their original `poll_id` server-side.

---

## Surfaces

Five glass-modal screens compose each flow. All conform to the standard
glass-modal shape — see [`docs/client/modals.md`](modals.md). There is
intentionally **no "unified history" entry modal**; both flows dispatch
directly from the Event-tab chip into the right sub-modal based on poll
state (see [Routing](#routing) below).

| Surface | Location | Time | Role |
|---|---|---|---|
| Propose | [`location_poll_propose_modal.dart`](../../app/lib/presentation/screens/experience/location_poll_propose_modal.dart) | [`time_poll_propose_modal.dart`](../../app/lib/presentation/screens/experience/time_poll_propose_modal.dart) | Three TBD-first states: **empty** (a `PollTbdCard` + "Keep it TBD for now" no-op dismiss), **one** ("Set the spot/time" + "or float it to the group" link), **many** ("Ask the group"); pre-seeds with the experience's confirmed value when starting a fresh poll |
| Vote | [`location_poll_vote_modal.dart`](../../app/lib/presentation/screens/experience/location_poll_vote_modal.dart) | [`time_poll_vote_modal.dart`](../../app/lib/presentation/screens/experience/time_poll_vote_modal.dart) | Voter taps options; auto-submit YES toggle per tap |
| Manage | [`location_poll_manage_menu.dart`](../../app/lib/presentation/screens/experience/location_poll_manage_menu.dart) | [`time_poll_manage_menu.dart`](../../app/lib/presentation/screens/experience/time_poll_manage_menu.dart) | Organizer action sheet: Edit Choices, Lock/Unlock, Nudge, Set Final, Cancel |
| Confirm | [`location_poll_confirm_modal.dart`](../../app/lib/presentation/screens/experience/location_poll_confirm_modal.dart) | [`time_poll_confirm_modal.dart`](../../app/lib/presentation/screens/experience/time_poll_confirm_modal.dart) | "SET THE FINAL X" eyebrow + sage winner card + per-proposal vote breakdown + Lock in & notify CTA |
| Finalized | [`location_poll_finalized_modal.dart`](../../app/lib/presentation/screens/experience/location_poll_finalized_modal.dart) | [`time_poll_finalized_modal.dart`](../../app/lib/presentation/screens/experience/time_poll_finalized_modal.dart) | Read-only "IT'S SET" sage card + non-winning pill row + primary CTA + owner Change menu |

### Routing

The Event-tab chip tap dispatches state-aware from
[`experience_content_view.dart`](../../app/lib/presentation/screens/experience/experience_content_view.dart):

| State | Sub-modal |
|---|---|
| Poll active | Vote modal |
| Confirmed value, no poll | Finalized modal |
| Poll ended, no winner | Confirm modal (location only — time auto-confirms via Set Final) |
| No value, no poll | Propose modal |

Underlying data:

- Location: [`location_modal_view_model.dart`](../../app/lib/presentation/viewmodels/location_modal_view_model.dart) + [`location_poll_methods.dart`](../../app/lib/services/experience/location_poll_methods.dart).
- Time: [`time_modal_view_model.dart`](../../app/lib/presentation/viewmodels/time_modal_view_model.dart) + the time-poll mixin in [`rsvp_time_methods.dart`](../../app/lib/services/experience/rsvp_time_methods.dart).

Shared, kind-agnostic widgets:

- [`PollManageMenuSheet<T>`](../../app/lib/presentation/widgets/poll/poll_manage_menu_sheet.dart) — generic Manage / Change-menu chrome composed by both flows.

---

## Voter experience

1. Voter taps the location or time chip on the experience screen.
2. The state-aware dispatcher in
   [`experience_content_view.dart`](../../app/lib/presentation/screens/experience/experience_content_view.dart)
   inspects poll state and opens the right sub-modal:
   - Poll active → Vote modal.
   - Confirmed value with no active poll → Finalized modal.
   - Otherwise → Propose modal.
3. In the Vote modal each proposal renders as a row: option body (place /
   date+time) on the left, voter-avatar stack on the right. Two visual states:
   - **idle** — white outline.
   - **voted** — sage fill + dark check (optimistic; reconciles to server
     state).
4. **Auto-submit on tap.** There is no "Save votes" button. Each tap
   fires a `voteOnLocation` / `voteOnTime` RPC immediately and reflects
   optimistically. Voting is binary (YES / no-vote) for both kinds.
5. After any pick lands, the eyebrow switches to "YOU'RE IN" so the
   voter knows their vote was registered.
6. At the bottom of the list (unless proposals are locked), an "Add
   another spot / time" affordance lets the voter propose a new option
   without leaving the modal.

On the Event tab, while a poll is active, the location/time row reads
"N possible spots" / "N possible times" (ICU plural; ≥1) with a
Choose/Chosen pill replacing the directions/calendar icon. See the
implementation in
[`experience_event_pane.dart`](../../app/lib/presentation/screens/experience/widgets/experience_event_pane.dart).

The Details tab intentionally does not surface poll banners or "Choose
spot/time" suggestion chips — both surfaces are anchored on the Event
tab and the in-modal flow.

---

## Organizer experience

The organizer sees the same Vote modal plus a **Manage** affordance that
opens a [`PollManageMenuSheet<T>`](../../app/lib/presentation/widgets/poll/poll_manage_menu_sheet.dart)
as a separate bottom sheet (so the vote modal stays interactive after
dismiss). Actions:

- **Edit Choices** — opens the propose modal in live-edit mode where
  the organizer can add, remove, or change the deadline against the
  active poll.
- **Lock / Unlock proposals** — toggles whether non-organizers can add
  options.
- **Nudge voters** — sends a push notification to anyone who hasn't
  voted yet (Yes/Maybe RSVPs only; owner is excluded).
- **Set Final** — opens the Confirm modal; on successful confirm the
  Vote modal closes and the user lands on the Finalized modal.
- **Cancel poll** — destructive; ends the poll without a winner.

The Confirm modal renders a "SET THE FINAL X" eyebrow, a serif title
that updates as the selection changes ("Confirm Avery Brewing."),
a sage-tinted FINAL X card showing the currently-selected winner, and a
"HOW EVERYONE PICKED" section with a radio-toggle row per proposal
(voter avatar stack + YES count). Cancel + "Lock in & notify group"
pills at the bottom. The default selection is the proposal with the
most YES votes. `show()` returns `Future<bool?>` so callers can chain
to the Finalized modal on `true`.

The Finalized modal renders the "IT'S SET" sage card with the winning
option, "N of M chose this {option}" footer, a faint pill row of the
non-winning proposals for context, and the primary CTA. Owner-only:
**Change** — opens a [`PollManageMenuSheet`](../../app/lib/presentation/widgets/poll/poll_manage_menu_sheet.dart)
with two options: swap directly to a different single value, or open
a fresh poll.

---

## Data shape

| Concept | Location | Time |
|---|---|---|
| Proposal proto | `LocationProposal` ([`location.pb.dart`](../../app/lib/data/gen/ripls/api/location.pb.dart)) | `TimeProposal` ([`time.pb.dart`](../../app/lib/data/gen/ripls/api/time.pb.dart)) |
| Vote proto | `LocationVote { user, status }` | `TimeVote { user, status }` |
| Vote semantics | YES per proposal (`Set<String>` of proposal IDs) plus a poll-wide FLEXIBLE marker folded into all options | YES per proposal (viewer state derived from `proposal.votes`) plus a poll-wide FLEXIBLE marker folded into all options |
| Server "lock" flag on Experience | `location_proposals_locked` | `time_proposals_locked` |
| Server "deadline" field | `location_poll_deadline_unix_sec` | `time_poll_deadline_unix_sec` |

RPCs (parallel sets):

- `ProposeLocation` / `ProposeTime`
- `VoteOnLocation` / `VoteOnTime`
- `ConfirmLocation` / `ConfirmTime`
- `UnlockLocation` / `UnlockTime`
- `CancelLocationPoll` / `CancelTimePoll`
- `LockLocationProposals` / `LockTimeProposals`
- `NudgeLocationPollVoters` / `NudgeTimePollVoters`
- `DeleteLocationProposal` / `DeleteTimeProposal`
- `SetLocationPollDeadline` / `SetTimePollDeadline`

---

## Intentional differences

Parity is the design contract; everything in this section is a
deliberate divergence that follows from the underlying concept being
modelled. Anything *not* in this list that the two flows do differently
should be treated as drift.

1. **Auxiliary content on the Finalized modal.** Location's
   `_FinalSpotCard` is followed by an interactive Mapbox map and the
   primary CTA is **Get directions**. Time's `_WinnerCard` has no map;
   the primary CTA is **Add to calendar**. The supporting action
   reflects the dominant follow-up for the concept.
2. **"Add" affordance opens a different picker.** Location's "Add
   another spot" opens [`LocationPickerModal`](../../app/lib/presentation/widgets/location/location_picker_modal.dart)
   (saved-place search + map + drop-pin). Time's "Add another time"
   opens [`DateTimePickerModal`](../../app/lib/presentation/widgets/modal/glass/date_time_picker_modal.dart)
   (calendar + time wheel).
3. **Geocoded vs. specific proposals.** Location proposals can
   reference a saved `Location` row by id OR carry an inline geocoded
   place. Time proposals only carry a specific `ExperienceTime`; range
   times that come back from `ExtractTimeCandidates` are downgraded to
   start-of-range for staging.
4. **Pre-seed source on Change → fresh poll.** Both flows pre-seed
   the propose modal with the experience's confirmed value, but the
   source differs: location reads `experience.locationId`; time reads
   `experience.time.specific` and translates it to a `DateTime`.
5. **No intentional vote-enum difference.** Both `LocationVoteStatus`
   and `TimeVoteStatus` declare five enum values (UNSPECIFIED, YES, NO,
   NONE_WORK, FLEXIBLE) on the wire, and both flows emit only
   UNSPECIFIED/YES/FLEXIBLE from the client. NO and NONE_WORK exist for
   wire-compatibility and are not rendered. (Listed here for clarity:
   the proto enums look richer than the actual semantics.)

### Flexible votes ("anywhere/anytime works")

Both flows let a voter mark themselves **flexible** — any proposed option
works — via the shared `PollFlexibleVoteRow` at the bottom of the vote list
(shown to voters, not the organizer). Mechanics, identical across kinds:

- A flexible vote is recorded as `*_VOTE_STATUS_FLEXIBLE` on a single
  representative proposal (the first in the current poll), cast through the
  existing `VoteOnLocation` / `VoteOnTime` RPC — no new RPC.
- The client **folds** flexible voters into every option's effective tally
  and into the leader calc. The derived getters live on `LocationModalData` /
  `TimeModalData` (`effectiveVoteCount`, `leadingProposalId`,
  `currentUserIsFlexible`, `repliedUserIds`) and are unit-tested in
  `app/test/presentation/viewmodels/{location,time}_modal_state_test.dart`.
- Server-side, `usersWhoVotedOnCurrentPoll` /
  `usersWhoVotedOnCurrentTimePoll` count a flexible vote as a reply, so a
  flexible voter is excluded from the `Nudge*` recipient set just like an
  explicit YES voter.

Because a poll-wide flexible vote adds equally to every option, it never
changes *which* option leads (the YES ordering is preserved); it raises every
option's count and the "N replied" progress.

---

## Closed parity items

For history: the migration plan called out a handful of "deferred work"
items that have since been closed. They're recorded here so future
contributors don't re-add them speculatively.

1. **Unified entry modals are not the entry point.** Earlier drafts
   built a `UnifiedLocationModal` / `UnifiedTimeModal` per-poll history
   list as the entry surface. Neither was actually wired into the
   chip-tap path on either side, and both were deleted. The Event-tab
   chip dispatches directly to the right sub-modal (see
   [Routing](#routing)). Re-introducing a history list, if ever
   needed, should ship on both flows in lockstep.
2. **The `PollVoteRow` / `PollFinalizedCard` / `PollHistoryRow`
   extractions were skipped intentionally.** The original Phase 5 plan
   named four widget extractions; only the high-value one
   (`PollManageMenuSheet<T>`) was extracted. The other three would have
   produced slot-based widgets that pinch one consumer or the other —
   location uses FutureBuilder lookups against
   `locationRepositoryProvider`, time formats `DateTime` synchronously,
   and the shared chrome is small (≤ 50 lines per kind). Parity is
   enforced by the [Intentional differences](#intentional-differences)
   contract above instead.
3. **`TimeModalData.pollPhase` getter** removed (had no
   `LocationModalData` equivalent and was only exercised by tests of
   the now-deleted `UnifiedTimeModal`).
4. **Accent color aligned to `AppColors.lightAccent`.** Time-poll
   modals had drifted to `AppColors.experienceSageGreen` for finalized
   card chrome, vote-modal CTAs, and the "YOU'RE IN" eyebrow. All such
   sites now use `lightAccent` to match the location flow.
5. **Default 24h reply-by deadline.** Time previously left
   `time_poll_deadline_unix_sec` unset on first proposal. It now seeds
   24h-from-now (`defaultTimePollDeadline` in
   [`time_proposals.go`](../../server/services/experience/time_proposals.go))
   to match `defaultLocationPollDeadline`, and the propose modal
   renders a display default in fresh-poll mode (`displayDeadlineSec`
   pattern, mirroring location).
6. **Multi-candidate LLM extract.** Time previously used the
   single-candidate `ConvertInformalTime` RPC, requiring repeated parses
   to stage multiple options. It now uses
   [`ExtractTimeCandidates`](../../server/services/experience/time_extract.go),
   which splits the input on conjunctions (reusing the location-side
   segmenter) and runs each segment through the AI provider in
   parallel, returning all parsed candidates in one round-trip —
   mirroring `ExtractLocationCandidates`. The propose modal's LLM
   hatch stages every candidate at once.

---

## Adding a new poll kind

When the next poll kind ships (e.g. budget poll, dress-code poll), the
expectation is:

1. Server: new request/response messages per RPC, mirroring the nine
   `*Time` / `*Location` parallel RPCs (one dedicated message pair per
   RPC).
2. Client: a new view-model + state file, a new set of five modal
   files (Propose / Vote / Manage / Confirm / Finalized), and bindings
   on the existing `experience_event_pane.dart` row.
3. Reuse `PollManageMenuSheet<T>` from
   [`widgets/poll/`](../../app/lib/presentation/widgets/poll/).
4. Add an "Intentional differences" entry above for anything the new
   kind genuinely needs to do differently. If nothing fits, the kind
   shouldn't diverge.

---

## See also

- [`docs/issues/2131-time-poll-migration.md`](../issues/2131-time-poll-migration.md) — the migration plan that produced this parallel structure, including the deferred-work list above.
- [`docs/client/modals.md`](modals.md) — the standard glass-modal shape every surface above conforms to.
