# Unified Time Modal

Merge the time-setting, poll-creation, and poll-voting modals into a single
two-tab modal that adapts its UI based on whether the viewer is the owner or a
non-owner, and whether a poll is active.

## Reference Designs

- `tmp/time3/unified-time.jsx` — Interactive React prototype of all states
- `tmp/time3/time-owner.png` — Owner "Set Time" tab (date/time/duration picker
  with live summary card)
- `tmp/time3/time-owner-create.png` — Owner "Alternatives" tab (poll builder
  with slot chips)
- `tmp/time3/time-owner-poll.png` — Owner "Alternatives" tab with live poll
  (vote progress, "End Poll" button)
- `tmp/time3/time-owner-pick.png` — Owner "Alternatives" tab after poll ends
  (radio-select winner, "Confirm Time")
- `tmp/time3/time-nonowner.png` — Non-owner single-tab view (read-only time
  card + GOING list, no tabs when no poll)
- `tmp/time3/time-nonowner-options.png` — Non-owner with poll active (tabs
  appear, coral "Vote on N time options →" CTA)
- `tmp/time3/time-nonowner-vote.png` — Non-owner "Alternatives" tab (vote
  cards + "None of these work for me" + "Suggest a different time")

---

## Problems

### 1. Fragmented modals for a single workflow

Time scheduling currently spans four separate surfaces:

| Surface | File | Purpose |
|---------|------|---------|
| TimeModal | `time_modal.dart` (1,320 lines) | Set time, propose, vote, finalize |
| VoteCard | `vote_card.dart` | Non-owner voting on active polls |
| FindBestTimeModal | `find_best_time_modal.dart` | Legacy poll creation / results |
| TimeVotingCard | `time_voting_card.dart` | Inline chat voting |

Users must navigate between different modals depending on role and poll state.
The mental model is unclear: "where do I go to vote?" vs "where do I go to set
a time?" should be the same place.

### 2. TimeModal is too large and mode-heavy

At 1,320 lines, `time_modal.dart` has accumulated multiple mode branches
(`default`, `propose`, `confirmed`, `edit`, `poll`, `askingGroup`) that
conditionally show different widget trees inside a single build method. Each
new state adds more branching.

### 3. Role-based UI is scattered

Owner vs non-owner behavior is split across separate modals (EventOrganizerModal
→ TimeModal vs EventJoinerModal → VoteCard) rather than being handled inside
one component with role-aware rendering.

### 4. Redundant voting UI

Voting logic appears in three places: TimeModal (proposal tiles), VoteCard
(full-screen voting), and TimeVotingCard (inline chat). The first two have
divergent layouts for the same conceptual action.

### 5. No unified poll lifecycle view for owners

Owners currently move between "Ask the Group" builder mode → default list
(with `timePollActive` flag) → selection → finalization. The transitions
between these states are implicit mode toggles inside one monolithic widget
rather than clear, tab-separated views.

---

## Potential Solutions

### Option A: Two-Tab Modal (recommended)

Replace the existing TimeModal, VoteCard, and FindBestTimeModal with a single
`UnifiedTimeModal` containing two tabs:

- **"Set Time" tab** — Owner: date/time/duration picker with live summary.
  Non-owner: read-only time display + GOING attendee list.
- **"Alternatives" tab** — Owner: poll builder → live poll → pick winner.
  Non-owner: vote on options + "Suggest a different time".

Tab visibility adapts: non-owners see tabs only when a poll is active. Owners
always see both tabs.

**Pros:** Matches the reference designs exactly. All time operations in one
place. Clear separation of "direct set" vs "collaborative polling". Reuses
existing widgets (ProposeTimeForm, PollOptionsBuilder, TimeProposalTile).

**Cons:** Requires restructuring `time_modal.dart` and deprecating VoteCard /
FindBestTimeModal. TimeModalNotifier needs minor state additions for tab
tracking.

### Option B: Keep separate modals, add navigation links

Keep TimeModal, VoteCard, and FindBestTimeModal as-is but add cross-links
("Vote on poll →", "Set time directly →") between them.

**Pros:** Minimal code changes.

**Cons:** Doesn't solve the fragmentation problem. User still navigates
between separate surfaces. Doesn't match reference designs.

### Option C: Single-tab progressive disclosure

One tab that progressively reveals sections (set time, then poll, then voting)
as the user scrolls.

**Pros:** No tab navigation.

**Cons:** Long scrollable form. Unclear separation of owner actions vs
collaborative polling. Doesn't match reference designs.

---

## Recommendation

**Option A: Two-Tab Modal.** It directly matches the reference designs, unifies
the user experience, and most of the widget work already exists. The primary
effort is restructuring the container (TimeModal → UnifiedTimeModal with tab
controller) and adapting existing sub-widgets to render inside the correct tab.

---

## Resolved Questions

1. **TimeVotingCard in chat** — **Remove TimeVotingCard entirely.** Instead,
   when a poll is live, show a poll button to the left of the share button on
   `ExperienceContentView`. The poll button (existing `_PollButton` widget)
   already appears there conditionally — this just becomes the sole entry point
   for poll voting, opening `UnifiedTimeModal` to the Alternatives tab.
   Files affected: delete `time_voting_card.dart`, remove from
   `workflow_widget_factory.dart` (`buildTimeVotingCard`), remove from
   `conversation_screen.dart` (the `timeCard` variable and `_handleVoteOnTime`).

2. **"Suggest a different time" for non-owners** — **Reuse ProposeTimeForm.**
   The full form (date/time/duration chips + calendar + time picker) provides a
   better UX than a bare date+time input. Wrap it in an expandable section
   triggered by the "+ Suggest a different time" button.

3. **Export to calendar** — **Wire up the existing
   `CalendarHelper.addToCalendar(experience)` method** (in
   `app/lib/core/utils/calendar_helper.dart`). The helper already handles
   specific times, time ranges, all-day events, and timezone conversion via the
   `add_2_calendar` package. The calendar icon button in the summary card calls
   this method directly (widget-level async exception — platform calendar UI).

4. **Poll minimum options** — **Minimum is 1 option** (not 2). Update
   `PollOptionsBuilder` validation and the bottom bar "Send Poll" enable
   condition accordingly.

5. **"None of these work for me"** — **Add server support.** The current
   `TimeVoteStatus` enum in `proto/ripls/models/time.proto` and
   `proto/ripls/api/time.proto` has `UNSPECIFIED`, `YES`, and `NO`. Add a new
   value `TIME_VOTE_STATUS_NONE_WORK = 3` to represent "none of these work for
   me." The existing `VoteOnTime` RPC already accepts a `status` field — no new
   RPC needed. When a user selects "None of these," the client sends
   `VoteOnTime` with `status = NONE_WORK` for every active proposal (or a
   single sentinel vote — TBD in implementation). The server handler in
   `server/services/experience/time_proposals.go` already stores arbitrary
   `TimeVoteStatus` values, so the main change is:
   - Proto: add enum value to both `api/time.proto` and `models/time.proto`
   - Server: optionally surface "none" voters in poll status / metrics
   - Client: `TimeModalNotifier.voteNoneWork()` method that votes NONE_WORK
     on all proposals and sets `cantMakeIt = true`

6. **Non-owner "Set Time" tab content** — When time is not yet set (TBD), show
   a centered `context.l10n.timeNotSetYet` message with a muted calendar icon.

---

## Implementation Plan

### Phase 0: Extract sub-views from TimeModal

**Goal:** Break `time_modal.dart` into composable sub-widgets so they can be
placed into tabs without copy-pasting.

**Files to create:**

- `app/lib/presentation/widgets/experience/time_set_view.dart` —
  Owner time-picker content (date/time/duration chips + inputs + live summary
  card). Extracted from TimeModal's "set time" mode build logic.
  `ConsumerWidget` that reads `timeModalProvider` via `ref.watch()` and
  triggers mutations via `ref.read(timeModalProvider.notifier)`.
  <!-- AUDIT: Sub-views must be ConsumerWidgets (not plain StatelessWidgets
       receiving TimeModalData + callbacks). Architecture rule: "Screens only
       interact with ViewModels via ref.watch/ref.read". Passing ViewModel data
       and callbacks through constructor args bypasses Riverpod reactivity and
       creates a parallel state-passing channel. -->
- `app/lib/presentation/widgets/experience/time_read_view.dart` —
  Non-owner read-only time display (hero card with date, time, duration +
  calendar export icon + GOING attendee list). `ConsumerWidget` reading
  `timeModalProvider`. New widget based on reference `time-nonowner.png`.
- `app/lib/presentation/widgets/experience/poll_live_view.dart` —
  Owner view of an active poll (status banner + VoteCard-style proposal list +
  vote progress). `ConsumerWidget` reading `timeModalProvider`. Extracted from
  TimeModal's poll-active branch.
- `app/lib/presentation/widgets/experience/poll_results_view.dart` —
  Owner view after poll ends (radio-select proposal list + "Confirm Time"
  action). `ConsumerWidget` reading `timeModalProvider`. Extracted from
  TimeModal's selection logic.
- `app/lib/presentation/widgets/experience/poll_vote_view.dart` —
  Non-owner voting view (proposal tiles + "None of these work for me" +
  "Suggest a different time" expandable section embedding ProposeTimeForm).
  `ConsumerWidget` reading `timeModalProvider`. Adapted from VoteCard.

**Files to modify:**

- `time_modal.dart` — Replace inline build branches with the new sub-widgets.
  Verify existing behavior is preserved before proceeding to Phase 1.

**Validation:** All existing time modal tests pass. Manual smoke test of owner
set-time, propose, vote, finalize, and non-owner voting flows.

---

### Phase 1: Proto + server support for "None of these work"

**Goal:** Add `TIME_VOTE_STATUS_NONE_WORK` to the proto enum and update the
server to store and surface it.

**Files to modify:**

- `proto/ripls/api/time.proto` — Add `TIME_VOTE_STATUS_NONE_WORK = 3` to the
  `TimeVoteStatus` enum.
- `proto/ripls/models/time.proto` — Add matching `TIME_VOTE_STATUS_NONE_WORK = 3`
  to the model-side `TimeVoteStatus` enum.
- Run `npm run generate` to regenerate Go and Dart code.

**Server changes:**

- `server/services/experience/time_proposals.go` — The `VoteOnTime` handler
  already accepts any `TimeVoteStatus` value and stores it, so no handler logic
  changes are needed. Optionally add a helper to surface "none work" voters
  in poll status (e.g., count of participants who voted NONE_WORK on all
  proposals, useful for the owner's poll live view).

**Validation:** `npm run build` passes (lint + compile). Existing vote tests
still pass. Add a unit test in `time_proposals_test.go` that votes with
`NONE_WORK` status and verifies it's stored and retrievable.

---

### Phase 2: Create UnifiedTimeModal shell with tabs

**Goal:** New `UnifiedTimeModal` widget with a `TabController` that renders the
"Set Time" and "Alternatives" tabs.

**Files to create:**

- `app/lib/presentation/screens/experience/unified_time_modal.dart` —
  `ConsumerStatefulWidget` with `TabController` (requires
  `SingleTickerProviderStateMixin` for the `TabController` — this is a
  documented widget-level exception for Flutter animation infrastructure, not
  business logic). Two tabs: "Set Time" and "Alternatives". Tab bar uses
  underline style matching reference (coral active indicator, using
  `AppColors.coral(context)` and `AppColors.textMuted(context)` — no hardcoded
  hex values). Content area is a `TabBarView` with the sub-views from Phase 0.
  Static `show()` method opens as bottom sheet (~90% height).
  <!-- AUDIT: TabController is a Flutter SDK animation concern, not business
       logic — StatefulWidget + SingleTickerProviderStateMixin is the correct
       pattern per architecture.md "Widget Async Exceptions" section. Tab INDEX
       is local UI state (which tab is visible), not app state — keep it in the
       widget, not the ViewModel. -->

**Tab visibility rules:**

| Role | Poll state | Tabs shown |
|------|-----------|------------|
| Owner | Any | Both tabs always |
| Non-owner | No poll | No tabs (Set Time content only) |
| Non-owner | Poll active | Both tabs |
| Non-owner | Poll ended | Both tabs (results read-only) |

**Bottom bar rules (per reference JSX):**

| Tab | Role | State | Left button | Right button |
|-----|------|-------|-------------|-------------|
| Set Time | Owner | — | — | "Update" (sage) |
| Set Time | Non-owner | — | — | — (no actions) |
| Alternatives | Owner | Building poll | — | "Send Poll" (coral, enabled ≥ 1 option) |
| Alternatives | Owner | Poll live | "End Poll" | "Waiting for votes…" (disabled) |
| Alternatives | Owner | Poll ended | — | "Confirm Time" / "Pick a time" (disabled until selection) |
| Alternatives | Non-owner | Voting | — | "Tap times that work" (disabled) / "Submit" (sage) |
| Alternatives | Non-owner | Voted "none" | — | "Can't Make It" (sage) |

<!-- AUDIT: Removed `activeTab` field from TimeModalData. Tab index is local
     UI state owned by the widget's TabController, not business state in the
     ViewModel. The ViewModel should not know or care which tab is displayed.
     The `initialTab` parameter on `show()` is a widget constructor arg that
     sets the TabController's initial index. -->

**Files to modify:**

- `time_modal_view_model.dart` — No tab-tracking changes needed. The ViewModel
  already exposes `timePollActive`, `isOrganizer`, and proposal data — the
  widget derives tab visibility from these.

**Validation:** UnifiedTimeModal opens with correct tab layout. Tab switching
works. Content areas render the Phase 0 sub-views.

---

### Phase 3: Wire owner flows

**Goal:** Owner can set time (tab 1) and create/manage/end polls (tab 2)
inside the unified modal.

**"Set Time" tab (owner):**

- Live summary card at top: calendar icon + "Friday, March 27 · 8:00 AM" +
  calendar-export icon button. Updates as chips are selected. Use
  `AppColors.surface(context)` for card background and
  `AppColors.border(context)` for the card border — no hardcoded `#fff` or
  `Color(0xFF...)`.
  <!-- AUDIT: Architecture requires theme-aware colors via AppColors helpers.
       The reference JSX uses hardcoded #FFFDF9 / #EDE8E0 — these must be
       mapped to AppColors equivalents for dark mode support. -->
- DATE section: 3 smart-default date chips + date input field with calendar icon.
- START TIME section: 3 time chips + time input field with clock icon.
- DURATION section: 3 duration chips.
- Bottom bar: "Update" button (sage, calls
  `ref.read(timeModalProvider(experienceId).notifier).updateEventTime()`).
- Reuses ProposeTimeForm chip logic and smart defaults.

**"Alternatives" tab (owner) — no poll yet:**

- Instruction text: `context.l10n.timeAlternativesInstructions`
  <!-- AUDIT: i18n.md requires all user-visible strings use context.l10n.
       All new strings in this feature must be added to app_en.arb AND app_es.arb
       before running npm run generate. Viewmodels expose data/enums, widgets
       resolve to localized text in build(). -->
- Existing PollSlotChip list of added options (numbered, removable).
- Date chips (coral accent when selected) + time chips (toggle add with ✓/+
  prefix).
- "+ Add a specific date & time" button for custom entry.
- Helper text: `context.l10n.timePollHelperText(pollSlots.length)`
  (ICU plural format).
- Bottom bar: "Send Poll" (coral, enabled when ≥ 1 option).
- Reuses PollOptionsBuilder (update its minimum validation from 2 → 1).

**"Alternatives" tab (owner) — poll live:**

- Status banner: coral dot + `context.l10n.timePollLiveStatus(voted, total)` in
  `highlight` background.
- Instruction text: `context.l10n.timePollWaitingInstructions`
- VoteCard-style proposal list (read-only for owner, shows voter avatars).
- Bottom bar: "End Poll" (outlined) + `context.l10n.timeWaitingForVotes`
  (disabled sage).
- "End Poll" calls `endTimePoll()`, transitions to poll-ended state.

**"Alternatives" tab (owner) — poll ended / all voted:**

- Status banner (sage): `context.l10n.timePollEveryoneVoted` or
  `context.l10n.timePollEndedPickWinner`
- Radio-select proposal list (tap to select, sage highlight + checkmark).
- Bottom bar: `context.l10n.commonConfirmTime` (sage, enabled when selection
  made) / `context.l10n.timePickATime` (disabled).
- "Confirm Time" calls `selectProposal()` → updates event time → calls
  `lockTime()` → auto-switches to Set Time tab showing confirmed state.
  Tab switch is done via the widget's `TabController.animateTo(0)` after the
  notifier method completes.

**Files to modify:**

- `time_set_view.dart` — Add live summary card with calendar icon. Wire chip
  selections to `ref.read(timeModalProvider(id).notifier).updateProposeDate/
  Time/Duration`.
- `poll_options_builder.dart` — Minor: ensure `onOptionsChanged` callback
  signature works with the new bottom bar wiring.
- `poll_live_view.dart` — Wire "End Poll" to `endTimePoll()`.
- `poll_results_view.dart` — Wire radio selection to `selectProposal()` and
  "Confirm Time" to `lockTime()`.
- `unified_time_modal.dart` — Bottom bar logic per the table above.
- `time_modal_view_model.dart` — Add `pollPhase` computed getter
  (`building` | `live` | `ended`) derived from existing `timePollActive` +
  proposals state. This simplifies the modal's conditional rendering. Keep as
  a getter on `TimeModalData`, not a stored field, to avoid state duplication.
  <!-- AUDIT: pollPhase is derivable from timePollActive + proposal count.
       Freezed state should not store computable values — use a const getter
       in the Freezed class body instead. -->
- `app/lib/l10n/app_en.arb` — Add new i18n keys for all user-visible strings
  introduced in this phase (see i18n note above).
- `app/lib/l10n/app_es.arb` — Add Spanish translations for all new keys.

**Validation:** Owner can: (1) set time directly and tap "Update", (2) switch
to Alternatives and build a poll with ≥ 1 option, (3) send poll, (4) see live
poll status, (5) end poll early, (6) pick winning time, (7) confirm time.

---

### Phase 4: Wire non-owner flows

**Goal:** Non-owner sees read-only time info and can vote on polls.

**"Set Time" tab (non-owner, or single view when no poll):**

- Hero time card: `AppColors.sageBg(context)` background, large date text
  (display font from `Theme.of(context).textTheme.headlineSmall`),
  time + duration below, calendar-export icon button.
  <!-- AUDIT: Reference uses Georgia serif — map to the app's display
       TextTheme, not a hardcoded FontFamily string. Use AppColors for the
       sage background, not Color(0xFF...). -->
- GOING section: attendee list (avatar + name + role badge:
  `context.l10n.experienceOrganizer` / `context.l10n.experienceGoing`).
- When poll is active: coral CTA button at bottom —
  `context.l10n.timeVoteOnOptions(pollData.length)` that switches to
  Alternatives tab via `TabController.animateTo(1)`.
- No bottom bar actions.

**"Alternatives" tab (non-owner):**

- Proposal list using TimeProposalTile (tappable toggle vote, checkmark
  circle, voter avatar chips).
- `context.l10n.timeNoneWork` row at bottom (exclusive — clears other votes).
- `context.l10n.timeSuggestDifferent` expandable section: dashed coral border
  button → expands to reveal a `ProposeTimeForm` (reused from the owner flow)
  with Cancel/Propose buttons below. ProposeTimeForm is already a
  `StatefulWidget` for local form input state — documented exception per
  architecture.md.
- Bottom bar: `context.l10n.timeTapTimesWork` (disabled) →
  `context.l10n.commonSubmit` (sage, enabled when any votes cast) /
  `context.l10n.timeCantMakeIt` (sage, enabled when "none" selected).

**Files to create:**

- (None — all sub-views created in Phase 0.)

**Files to modify:**

- `time_read_view.dart` — Build hero card, GOING list, and poll CTA per
  reference. Takes `experienceId` as constructor arg, reads
  `timeModalProvider(experienceId)` internally via `ref.watch()`.
  <!-- AUDIT: Sub-views take the provider family key (experienceId), not raw
       data. This preserves Riverpod reactivity — the widget rebuilds when
       the notifier's state changes. -->
- `poll_vote_view.dart` — Wire vote toggles to `voteOnTime()`. Add "None of
  these" toggle (calls `voteNoneWork()` on the notifier, which sends
  `TIME_VOTE_STATUS_NONE_WORK` to server). Add "Suggest a different time"
  expandable section embedding `ProposeTimeForm` + Cancel/Propose buttons,
  calling `proposeTime()` on submit.
- `unified_time_modal.dart` — Tab visibility logic: hide TabBar when non-owner
  and no poll active.
- `time_modal_view_model.dart` — Add `cantMakeIt` boolean to `TimeModalData`
  (derived from whether the user's votes are all `NONE_WORK`). Add
  `voteNoneWork()` method: sends `VoteOnTime` with
  `TIME_VOTE_STATUS_NONE_WORK` for all active proposals, then refreshes. Add
  `suggestTime(date, time)` convenience method wrapping `proposeTime()`.
- `app/lib/l10n/app_en.arb` — Add non-owner i18n keys.
- `app/lib/l10n/app_es.arb` — Add Spanish translations.

**Validation:** Non-owner can: (1) see read-only time when no poll, (2) see
tabs when poll active, (3) vote on multiple options, (4) select "none of
these", (5) suggest a different time, (6) submit votes.

---

### Phase 5: Update entry points

**Goal:** All callers that previously opened TimeModal, VoteCard, or
FindBestTimeModal now open UnifiedTimeModal.

**Entry point mapping:**

| Caller | Old target | New target | Initial tab |
|--------|-----------|------------|-------------|
| EventOrganizerModal WhenRow `onTap` | `TimeModal.show(startInSetTimeMode: true)` | `UnifiedTimeModal.show()` | Set Time |
| EventJoinerModal WhenRow `onTap` | `TimeModal.show()` (read-only) | `UnifiedTimeModal.show()` | Set Time |
| EventJoinerModal WhenRow `onVote` | `VoteCard.show()` | `UnifiedTimeModal.show(initialTab: 1)` | Alternatives |
| ExperienceContentView menu "Change time" | `TimeModal.show()` | `UnifiedTimeModal.show()` | Set Time |
| ExperienceContentView menu "Vote" | `VoteCard.show()` | `UnifiedTimeModal.show(initialTab: 1)` | Alternatives |
| ExperienceContentView `_PollButton` | `VoteCard.show()` | `UnifiedTimeModal.show(initialTab: 1)` | Alternatives |
| FindBestTimeModal callers | `FindBestTimeModal.show()` | `UnifiedTimeModal.show(initialTab: 1)` | Alternatives |

The `UnifiedTimeModal.show()` method opens via `showModalBottomSheet` (not
`NavigationHelpers.pushScreen`). Bottom sheets are standard Flutter modal
presentation — `NavigationHelpers` is for full-screen push navigation.
<!-- AUDIT: Architecture.md NavigationHelpers section applies to "in-app
     forward navigation" (pushScreen, pushWithSlide). Bottom sheets use
     showModalBottomSheet which is a documented widget-level async exception.
     The existing TimeModal.show() already uses this pattern. -->

**Files to modify:**

- `event_organizer_modal.dart` — Replace `TimeModal.show()` with
  `UnifiedTimeModal.show()`.
- `event_joiner_modal.dart` — Replace `TimeModal.show()` and `VoteCard.show()`
  with `UnifiedTimeModal.show()` (different `initialTab` values).
- `experience_content_view.dart` — Replace menu action targets.
- `when_row.dart` — Simplify: single `onTap` callback (remove separate
  `onVote`). The unified modal handles role detection internally.

**Validation:** All entry points open the correct tab. No dead references to
old modal `show()` methods.

---

### Phase 6: Delete deprecated files

**Goal:** Remove the old modals that are fully replaced.

**Files to delete:**

- `app/lib/presentation/screens/experience/time_modal.dart` — Replaced by
  UnifiedTimeModal + sub-views.
- `app/lib/presentation/widgets/experience/vote_card.dart` — Voting now in
  `poll_vote_view.dart` inside the unified modal.
- `app/lib/presentation/screens/experience/find_best_time_modal.dart` — Poll
  creation now in Alternatives tab.
- `app/lib/presentation/widgets/chat/time_voting_card.dart` — Inline chat
  voting removed; poll button on content view replaces it.

**Files to modify (cleanup):**

- `app/lib/presentation/widgets/chat/workflow_widget_factory.dart` — Remove
  `buildTimeVotingCard()` method and `time_voting_card.dart` import.
- `app/lib/presentation/screens/chat/conversation_screen.dart` — Remove
  `timeCard` variable, `_handleVoteOnTime`, and the widget insertion point
  that prepends the voting card to the message list.

**Files to keep:**

- `time_modal_view_model.dart` — Reused by the unified modal (rename provider
  if desired, but not required since the family key is `experienceId`).
- `propose_time_form.dart` — Still reusable.
- `poll_options_builder.dart` — Still reusable.
- `time_proposal_tile.dart` — Still reusable.
- `time_slot_card.dart` — Evaluate if still needed; may be replaced by simpler
  tile in the unified layout.

**Validation:** `flutter analyze` clean. No dead imports. All tests updated.
Remove any orphaned i18n keys from `app_en.arb` / `app_es.arb` that were only
used by deleted files.

---

### Phase 7: Tests

**Goal:** Unit and widget tests for the unified modal.

**Test file:**

- `app/test/presentation/screens/experience/unified_time_modal_test.dart`

**Test setup:** Widget tests must wrap with `MaterialApp` including
`localizationsDelegates` and `supportedLocales` (per i18n.md test pattern and
`app/test/helpers/l10n_helpers.dart`). Mock `TimeModalNotifier` via
`ProviderContainer` overrides — do not use real services or repositories in
widget tests.
<!-- AUDIT: Architecture.md testing rules: "Test ViewModels with fake
     implementations", "Use ProviderContainer for unit tests", "Don't test UI
     and logic together". Widget tests mock the ViewModel; ViewModel tests
     mock the repository. i18n.md: widget tests need localization delegates. -->

**Test cases:**

1. **Tab visibility:** Owner always sees 2 tabs. Non-owner sees 0 tabs when no
   poll, 2 tabs when poll active.
2. **Owner Set Time tab:** Date/time/duration chips render. Tapping chip
   updates summary card. "Update" button calls `updateEventTime()`.
3. **Owner Alternatives — build poll:** Adding options shows PollSlotChips.
   "Send Poll" disabled when 0 options, enabled when ≥ 1. Sending calls
   `createTimePoll()`.
4. **Owner Alternatives — live poll:** Status banner shows vote progress.
   "End Poll" calls `endTimePoll()`. "Waiting for votes…" is disabled.
5. **Owner Alternatives — pick winner:** Radio selection highlights card.
   "Confirm Time" enabled after selection. Calls `selectProposal()` +
   `lockTime()`.
6. **Non-owner Set Time tab:** Hero card shows date/time/duration. GOING list
   renders attendees. Poll CTA appears when poll active.
7. **Non-owner Alternatives — voting:** Tapping proposal toggles vote.
   "None of these" clears other votes. "Submit" enabled when votes cast.
8. **Non-owner Alternatives — suggest time:** "+ Suggest" expands form.
   "Propose" calls `proposeTime()` when date+time filled.
9. **Bottom bar transitions:** Correct buttons per tab/role/state combination.
10. **Tab auto-navigation:** Non-owner with active poll opens to Alternatives.

**Additional ViewModel tests** (in a separate file
`app/test/presentation/viewmodels/time_modal_view_model_test.dart` if not
already present):

11. **`pollPhase` getter:** Returns `building` when no poll, `live` when
    `timePollActive`, `ended` when proposals exist but poll not active.
12. **`voteNoneWork()`:** Sends `NONE_WORK` status for all active proposals.
    Sets `cantMakeIt = true` in state. Toggling back clears NONE_WORK votes.
13. **`suggestTime`:** Delegates to `proposeTime()` with correct args.
14. **Disposal safety:** Disposing during async vote/lock completes without
    throwing (per architecture.md disposal testing requirement).

**Validation:** `flutter test --reporter expanded` — all tests pass.

---

### Phase 8: Polish and edge cases

**Goal:** Handle remaining edge cases and visual refinements.

- **Empty state (no time set, non-owner):** Centered
  `context.l10n.timeNotSetYet` with muted calendar icon using
  `AppColors.textMuted(context)`.
- **Loading states:** Shimmer placeholders while `TimeModalNotifier` loads.
  Use `ref.watch(timeModalProvider(id))` with `.when(data:, loading:, error:)`
  pattern if the notifier is an `AsyncNotifier`, or check `state.isLoading`
  for `Notifier<TimeModalData>`.
- **Error handling:** Snackbar on vote/propose/lock failures (already exists in
  notifier, verify propagation).
- **Animation:** Tab switch uses default `TabBarView` physics. Bottom bar
  button transitions use `AnimatedSwitcher`.
- **Calendar export icon:** Place icon button in summary cards (owner +
  non-owner). Wire to `CalendarHelper.addToCalendar(experience)` — the helper
  already exists in `app/lib/core/utils/calendar_helper.dart` and handles
  specific times, ranges, all-day events, and timezones. Disable the button
  (muted icon, no onTap) when time is TBD (the helper returns `false` for TBD).
  This is a widget-level async exception (platform calendar UI).
- **Accessibility:** Ensure tab bar has proper semantics. Vote cards have
  tap target ≥ 48px.
- **Cache invalidation:** After `lockTime()` and `endTimePoll()`, the notifier
  already calls `ref.invalidateSelf()` to refresh from server. Verify that
  `experienceRepository` cache is also invalidated so that `WhenRow` and
  `ExperienceContentView` reflect the updated time/poll state when the modal
  closes. This follows the existing repository cache invalidation pattern
  (caching.md Pattern 3).
  <!-- AUDIT: Mutations must invalidate all affected caches. Locking time
       changes the experience's time fields — the experience cache must be
       invalidated so WhenRow sees the update via ref.watch. -->

**Validation:** Full manual walkthrough of all 5 reference scenarios (owner
creates, owner ends poll, owner picks, friend with poll, friend without poll).
Verify that chat conversations no longer show the inline TimeVotingCard. Verify
that the poll button on ExperienceContentView opens the unified modal.

---

### Phase 9: Fix entry point and pollPhase bugs

**Goal:** Fix three issues with how the modal opens and what it shows on
initial load.

#### Bug 1: Owner "Confirm Time" menu item always opens to Alternatives tab

**Problem:** The owner menu item in `experience_content_view.dart` (line ~675)
hardcodes `initialTab: 1`:

```dart
onTap: () => UnifiedTimeModal.show(
  context,
  exp.id,
  initialTab: 1,  // ← always Alternatives
  ...
),
```

When the owner taps "Confirm Time" on a fresh experience with no poll, the
modal opens to the Alternatives tab instead of the Set Time tab.

**Fix:** Make `initialTab` conditional on whether a poll is live:

```dart
onTap: () => UnifiedTimeModal.show(
  context,
  exp.id,
  initialTab: exp.timePollActive ? 1 : 0,
  ...
),
```

When no poll is running, the owner lands on Set Time (tab 0) where they can
set the time directly. When a poll is running, they land on Alternatives
(tab 1) to manage the poll.

#### Bug 2: Alternatives tab shows "poll ended" when no poll was ever created

**Problem:** The `pollPhase` getter in `time_modal_view_model.dart` returns
`'ended'` whenever `proposals.isNotEmpty && !hasLockedTime`:

```dart
String get pollPhase {
  if (timePollActive) return 'live';
  if (proposals.isNotEmpty && !hasLockedTime) return 'ended';
  return 'building';
}
```

When the owner directly sets a time (without creating a poll), the experience
has a time proposal (the event time itself appears as a proposal in the
proposals list). This makes `proposals.isNotEmpty` true, and since
`timePollActive` is false and `hasLockedTime` is false, `pollPhase` returns
`'ended'` — showing "Poll ended · Pick the winning time" even though no poll
was ever created.

**Fix:** Add a `timePollEverCreated` flag (or equivalent) to distinguish
"proposals from direct time setting" from "proposals from a poll." The server
already tracks whether a poll was created via the `time_poll_active` flag on
the experience, but once a poll ends that flag is cleared. Options:

- **Option A (preferred):** Track poll history on the server. Add an
  `optional bool time_poll_ever_created` field to the experience proto (or
  derive it: if `CancelTimePoll` was ever called or all participants voted,
  the poll existed). The `pollPhase` getter becomes:

  ```dart
  String get pollPhase {
    if (timePollActive) return 'live';
    if (timePollEverCreated && proposals.isNotEmpty && !hasLockedTime) {
      return 'ended';
    }
    return 'building';
  }
  ```

- **Option B (simpler, client-only):** Consider proposals as poll-originated
  only if there are ≥ 2 proposals from different users (the owner setting time
  creates exactly 1 proposal attributed to the owner). This is a heuristic
  but covers the common case.

- **Option C (simplest):** Use the existing `canSuggest` or another server
  flag. Check if any proposal has votes from non-owners — if so, a poll
  existed. If all proposals only have the owner's implicit vote, no poll was
  created.

**Recommendation:** Option A. Add `optional bool time_poll_completed` to the
experience API proto (set to true when `CancelTimePoll` is called or the poll
ends naturally). The `pollPhase` getter checks this field to distinguish
"proposals from direct time set" vs "proposals from a completed poll."

**Proto change:**

- `proto/ripls/api/experience.proto` — Add
  `optional bool time_poll_completed = <next_field_number>;` to the
  `Experience` message.
- Server: Set `time_poll_completed = true` in `CancelTimePoll` handler and
  when all participants have voted (alongside clearing `time_poll_active`).
- Run `npm run generate`.

**ViewModel change:**

- `time_modal_view_model.dart` — Read `timePollCompleted` from the experience
  data. Update `pollPhase` getter:

  ```dart
  String get pollPhase {
    if (timePollActive) return 'live';
    if (timePollCompleted && proposals.isNotEmpty && !hasLockedTime) {
      return 'ended';
    }
    return 'building';
  }
  ```

#### Bug 3: Non-owner time card is not tappable when no poll is active

**Problem:** In `experience_content_view.dart` (line ~800), the time card's
`onTap` for non-owners is guarded by `exp.timePollActive`:

```dart
onTap: state.isOwner
    ? _showTimeModal
    : (exp.timePollActive ? _showVoteCard : null),
```

When no poll is active, the non-owner cannot tap the time card at all. Per the
reference design (`time-nonowner.png`), non-owners should always be able to tap
the time to see the read-only view (date, time, duration, GOING list).

**Fix:** Make the time card always tappable for non-owners. Open the unified
modal with `initialTab: 0` (read-only Set Time view) by default, or
`initialTab: 1` when a poll is active:

```dart
onTap: state.isOwner
    ? _showTimeModal
    : () => _showTimeModalNonOwner(exp.timePollActive),
```

Add the new method:

```dart
void _showTimeModalNonOwner(bool pollActive) async {
  ref.invalidate(timeModalProvider(widget.experienceId));

  await UnifiedTimeModal.show(
    context,
    widget.experienceId,
    initialTab: pollActive ? 1 : 0,
  );

  if (mounted) {
    await ref
        .read(experienceProvider(widget.experienceId).notifier)
        .loadExperienceDetails();
  }
}
```

This replaces the current `_showVoteCard()` method which can be removed (it
was only used for non-owner time card taps and is superseded by
`_showTimeModalNonOwner`).

#### Files to modify

- `app/lib/presentation/screens/experience/experience_content_view.dart`:
  - Owner menu item: conditional `initialTab` based on `exp.timePollActive`.
  - Non-owner time card: always tappable, calls `_showTimeModalNonOwner()`.
  - Remove `_showVoteCard()` method (replaced by `_showTimeModalNonOwner`).
- `app/lib/presentation/viewmodels/time_modal_view_model.dart`:
  - Update `pollPhase` getter to check `timePollCompleted`.
  - Add `timePollCompleted` field to `TimeModalData` (populated from
    experience proto during `build()`).
- `proto/ripls/api/experience.proto`:
  - Add `optional bool time_poll_completed` field.
- Server `time_proposals.go` or `experience` service:
  - Set `time_poll_completed = true` when poll ends.
- Run `npm run generate`.

#### Validation

1. **Owner, fresh experience, no poll:** Tapping "Confirm Time" menu opens
   Set Time tab (tab 0). Alternatives tab shows poll builder (per
   `time-owner-create.png`), not "poll ended."
2. **Owner, poll running:** Tapping "Confirm Time" menu opens Alternatives
   tab (tab 1) showing live poll status.
3. **Owner, poll ended:** Tapping "Confirm Time" menu opens Alternatives
   tab (tab 1) showing pick-winner view.
4. **Non-owner, no poll:** Tapping time card opens modal with no tabs,
   showing read-only view (per `time-nonowner.png`).
5. **Non-owner, poll active:** Tapping time card opens modal with 2 tabs,
   Alternatives tab selected, showing voting view.

---

## Audit Log

Changes made during architectural compliance review:

1. **Sub-view widget type** (Phase 0): Changed from "Takes `TimeModalData` +
   callbacks" to `ConsumerWidget` reading `timeModalProvider` via `ref.watch()`.
   **Why:** Architecture rule — screens/widgets interact with ViewModels through
   Riverpod, not by receiving ViewModel data as constructor args.

2. **Tab state ownership** (Phase 1): Removed `activeTab` field from
   `TimeModalData` and `setActiveTab()` from the ViewModel. Tab index stays in
   the widget's `TabController`.
   **Why:** Tab selection is local UI state (like scroll position), not business
   state. Adding it to the ViewModel violates the principle that ViewModels hold
   business logic, not UI chrome state.

3. **Theme-aware colors** (Phases 2, 3): Added explicit `AppColors.*` calls
   instead of references to hardcoded hex values from the JSX reference.
   **Why:** Architecture requires all colors use `AppColors.xxx(context)` for
   dark mode support. Hardcoded colors break the dark theme.

4. **i18n compliance** (Phases 2, 3, 5, 7): All user-visible strings now
   reference `context.l10n.*` keys. Added `app_en.arb` and `app_es.arb` to
   modified files lists. Added orphaned key cleanup in Phase 5.
   **Why:** i18n.md requires every user-visible string use `context.l10n.*`.
   ViewModels must not resolve strings — widgets do it in `build()`.

5. **`pollPhase` as getter** (Phase 2): Changed from "Add `pollPhase` enum"
   (stored field) to "computed getter on `TimeModalData`".
   **Why:** `pollPhase` is derivable from `timePollActive` + proposal count.
   Freezed state should not store computable values — use a const getter.

6. **Test setup** (Phase 6): Added localization delegate requirement, mock
   pattern clarification, ViewModel-specific test cases, and disposal safety
   test.
   **Why:** Architecture.md requires disposal tests for async notifiers. i18n.md
   requires localization delegates in widget tests. Testing rules require
   separating UI tests from logic tests.

7. **Cache invalidation** (Phase 7): Added explicit note about invalidating
   `experienceRepository` cache after mutations.
   **Why:** Caching.md Pattern 3 — mutations must invalidate all affected caches.
   Locking time changes the experience, so the experience cache must be
   invalidated for WhenRow and other watchers to see the update.

8. **Navigation pattern** (Phase 5): Clarified that `show()` uses
   `showModalBottomSheet`, not `NavigationHelpers`.
   **Why:** Architecture.md NavigationHelpers is for push navigation, not
   bottom sheets. The existing pattern is correct — just made it explicit.

### Post-Audit Updates (Resolved Questions)

9. **TimeVotingCard removal** (Phase 6): Changed from "keep TimeVotingCard" to
   delete it entirely. Chat voting card removed; poll button on content view
   becomes the sole non-modal entry point. Added cleanup for
   `workflow_widget_factory.dart` and `conversation_screen.dart`.

10. **ProposeTimeForm reuse** (Phase 4): Changed "Suggest a different time"
    from a simple inline date+time form to reusing the existing
    `ProposeTimeForm` widget. Provides a richer UX (chips, calendar, duration).

11. **Calendar export wired** (Phase 8): Changed from "no-op with TODO" to
    wiring up the existing `CalendarHelper.addToCalendar(experience)` method.
    The helper already handles all time formats and timezone conversion.

12. **Poll minimum reduced** (Phase 3): Changed from ≥ 2 to ≥ 1 minimum
    options for "Send Poll" activation. Updated PollOptionsBuilder validation,
    bottom bar enable condition, and test case.

13. **"None of these" server support** (new Phase 1): Added proto + server
    phase for `TIME_VOTE_STATUS_NONE_WORK = 3` enum value. The existing
    `VoteOnTime` RPC already accepts arbitrary status values — just need the
    new enum value in both `api/time.proto` and `models/time.proto`. Client
    `voteNoneWork()` method sends NONE_WORK for all active proposals.

14. **Phase renumbering**: Inserted new Phase 1 (proto/server), renumbered
    all subsequent phases (old 1→2, 2→3, 3→4, 4→5, 5→6, 6→7, 7→8).
