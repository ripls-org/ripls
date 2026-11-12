# Issue #2131: Migrate time-poll UX to match location-poll flow

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2131
**Filed:** 2026-05-23 | **Last activity:** 2026-05-23
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** [`docs/client/polls.md`](../client/polls.md)
**Audited against:** `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/client/accessibility.md`,
`docs/client/testing.md`, `docs/server/architecture.md`,
`docs/server/observability.md` (2026-05-23).

## Problem Statement

Experience polls have two kinds — location and time — that cover the same RPC
shape (propose / vote / confirm / lock / nudge / cancel on the location side)
but the time-poll client UX has drifted from the more mature location-poll
flow. The migration is client-side and UX-only. The data model and the
existing time-poll RPCs (`ProposeTime`, `VoteOnTime`, `ConfirmTime`,
`UnlockTime`, `CancelTimePoll`) are not changing.

The drift to close, in order of user-visible impact:

1. **Vote submission ergonomics.** Time votes batch behind a "Save votes"
   button via `timePollVoteSelectionProvider`. Location auto-submits per tap.
   The batched UX feels heavier and is the most-noticed difference in the
   field.
2. **No organizer Manage sheet for time.** Time exposes only inline
   "Cancel poll" / "Pick winning time" buttons. Location has a five-action
   Manage menu (Add, Lock/Unlock, Nudge, Set Final, Cancel).
3. **No `TimePollFinalizedModal`.** Once confirmed, time falls back to a
   read-only branch of the vote modal. Location renders a dedicated
   "It's a plan." card with the winning option, loser pills, primary CTA
   (Get directions), and an owner Change menu.
4. **Entry routing is inconsistent.** `UnifiedTimeModal` is organized around
   the owner's date/time picker, with poll modals launched ad-hoc from
   `needs_section.dart`, `inline_conversation_view.dart`, and
   `experience_content_view.dart`. `UnifiedLocationModal` lists per-poll
   history rows and dispatches into the right sub-modal — the time chip
   should follow the same shape.

**Voting is binary, not tri-state.** Earlier drafts of this plan
treated time votes as `YES / NO_WORK / MAYBE`, mirroring the proto
enum. That was wrong. The tri-state at the experience level is **RSVP
intention** (Yes / Maybe / No to attending) — that lives on `RSVP`,
not on time votes. *Voting on a proposed time* is a binary signal
("does this slot work for me?"), exactly like voting on a proposed
location. The migration aligns the two: a tap commits a YES vote; a
second tap clears it. `TimeVoteStatus` values other than YES become
unused on the client and are not surfaced in the UI.

In addition to the four deltas above, two adjacent surfaces need to
move together with this migration so the time/location parity is real
end-to-end, not just inside the poll modals:

- The **Event pane** (first tab in `ExperienceContentView`) already
  shows a `_PollVotePill` for active time polls, but the time row's
  *value* still reads as a confirmed time / `TBD` / `Choose a time`.
  Location's row reads `"Choose from N spots"` when its poll is
  active. Time should mirror that: when a time poll is active and
  there are multiple proposals, the row's value reads
  `"Choose from N times"` and the trailing pill behaves like the
  location Choose/Chosen pill.
- The **Details pane** (second tab) currently surfaces time-poll
  affordances via `NeedsSection`: a `PollBanner` while the poll is
  active, a `PollEndedBanner` for owners after a poll ends, and a
  "Choose time" suggestion chip in the zero-state. Location is
  intentionally absent from this surface (see the comment at
  [`needs_section.dart:104-108`](../../app/lib/presentation/widgets/shared/needs_section.dart)).
  Time should match: remove the entire time-poll surface from the
  Details tab — banners, suggestion chip, and the propose/vote/confirm
  call sites that live in `NeedsSection`.

## Context from Comments

The issue has no comments yet — it was filed just before this plan was
written. The issue body itself is the only source of truth right now. That
means the plan needs to surface decisions the author hasn't made explicitly
(e.g. the tri-state gesture, whether to ship without server-side lock/nudge
RPCs) so a reviewer can stamp them before code starts.

## Potential Solutions

### Option A: Lift-and-shift (mirror the location modals, copy then extract)

- **Approach:** Ship the missing server RPCs first
  (`LockTimeProposals`, `NudgeTimePollVoters`,
  `DeleteTimeProposal`, `SetTimePollDeadline`), then build
  `TimePollManageMenuSheet`, `TimePollFinalizedModal`, and a
  `UnifiedTimeModal` history layout by copying the location
  equivalents and tweaking them for time. Convert the vote modal to
  auto-submit. Once both flows work, extract the duplicated chrome
  into shared widgets (`PollManageMenuSheet<T>`, finalized-card
  scaffold, unified-history list).
- **Pros:** Fastest path to user-visible parity. Each phase is independently
  shippable. The "extract" step is informed by two real implementations,
  which produces a better abstraction than designing the generic up front.
- **Cons:** Temporary duplication; if extraction is deferred, the two flows
  can drift again. Discipline required to actually do phase 5
  (extraction).
- **Effort:** Medium overall; small for each phase.

### Option B: Abstract-first (build shared widgets, then plug both in)

- **Approach:** Design `PollManageMenuSheet<T>`, a generic finalized-card
  scaffold, and a generic vote row up front, then refactor location to
  consume them and build time on the same primitives in one pass.
- **Pros:** No duplication. Forces a clean kind-agnostic seam.
- **Cons:** Riskier — designing the abstraction without two real consumers
  tends to produce widgets that fit one and pinch the other. Bigger
  single PR. The location flow is stable; refactoring it for abstraction's
  sake adds regression risk on a working flow.
- **Effort:** Large; single landing.

### Option C: Minimum viable parity (auto-submit + finalized only)

- **Approach:** Ship the two highest-impact changes — auto-submit-on-tap and
  `TimePollFinalizedModal` — and stop. Leave the Manage menu and unified
  routing as future work.
- **Pros:** Smallest patch; closes the loudest UX gaps.
- **Cons:** Leaves the organizer experience and entry-point inconsistency
  unresolved, which is two of the four deltas the issue explicitly calls
  out. Future contributors will keep needing to read this plan to know
  what's missing.
- **Effort:** Small.

## Recommendation

**Option A**, with extraction (now phase 5) treated as part of the
issue rather than a follow-up. The location flow is the wrong place
to start an abstraction — the abstraction should be informed by a
real second consumer (time). Build time by mirroring location, ship
the parity, then extract together. That order produces the best
shared widgets and keeps each PR small enough to review.

The server-side Lock / Nudge / Delete / Deadline RPCs are built as
part of this work (now phase 1, landing before any client work). The
plan is therefore fully self-contained: no companion issues, no
greyed-out menu items.

## Decisions

The questions raised by the audit were resolved by the issue owner
during plan review (2026-05-23). Recording each one as a decision
so the implementation phases below have no ambiguity to navigate.

- **Voting is binary, not tri-state — DECIDED.** RSVP intention is
  tri-state (Yes / Maybe / No) but it lives on `RSVP`, not on time
  votes. A vote on a *proposed time* is the same shape as a vote on
  a *proposed location*: YES or no-vote. A single tap commits YES;
  a second tap clears. `TimeVoteStatus` values other than YES are
  not surfaced. This eliminates the long-press / segmented-control
  question entirely — the time row uses the same `Toggle` shape as
  the location row, and the shared `PollVoteRow` in phase 5 has no
  "tri-state vs. binary" axis to bridge.
- **Server-side Lock / Nudge / Delete / Deadline RPCs ship in this
  plan — DECIDED.** No separate companion issue. The missing RPCs
  are built as part of this work; the phase ordering below places
  the server work *first* (phase 1) so all client phases can rely on
  it. New `CommunityEventType` values are classified in
  `server/undo/registry.go` per CLAUDE.md.
- **Optimistic vote state lives in `TimeModalNotifier` — DECIDED.**
  Mirrors `LocationModalNotifier`. `timePollVoteSelectionProvider`
  goes away with the batching it was built for.
- **`UnifiedTimeModal` mirrors `UnifiedLocationModal` — DECIDED.**
  Confirmed-time row at top, per-poll history rows below, no
  "single owner picker" surface. Owner taps the confirmed-time row
  to swap (opens propose modal in single-option / edit mode).
- **Finalized card primary CTA is "Add to calendar" — DECIDED.**
  Mirrors location's "Get directions". The finalized modal is the
  destination, not a stop on the way.

## Adjacent surfaces that move with this work

These two changes were called out by the issue owner during plan
review. They aren't internal to the poll modals, but the migration
isn't complete without them:

1. **First tab (Event pane) — surface poll options on the time row.**
   The location row in
   [`experience_event_pane.dart`](../../app/lib/presentation/screens/experience/widgets/experience_event_pane.dart)
   already reads `"Choose from N spots"` when `locationPollActive`
   and renders a Choose/Chosen pill (`_LocationVotePill`, lines 168–214).
   The time row currently falls back to `context.l10n.planChipChooseTime`
   ("Choose a time") with a Vote/Voted pill. Update the time row to
   mirror location: when `timePollActive` and there are ≥2 proposals,
   the row's value reads `"Choose from N times"` and the trailing pill
   becomes Choose/Chosen with the same gesture semantics as the
   location pill (tap opens the vote modal directly). When the poll
   has only one proposal, keep the existing single-time copy — there's
   nothing to choose between.
2. **Second tab (Details pane / `NeedsSection`) — remove the time poll
   surface entirely.** Strip:
   - `PollBanner` for active time polls
     ([`needs_section.dart:110-115`](../../app/lib/presentation/widgets/shared/needs_section.dart))
   - `PollEndedBanner` for owners after a poll ends
     ([`needs_section.dart:116-121`](../../app/lib/presentation/widgets/shared/needs_section.dart))
   - The "Choose time" suggestion chip in the zero-state
     ([`needs_section.dart:164-174`](../../app/lib/presentation/widgets/shared/needs_section.dart))
   - The `onChooseTimeTap` plumbing through `NeedsActions`
     ([`needs_section.dart:388,426`](../../app/lib/presentation/widgets/shared/needs_section.dart)
     and the corresponding parameter in `needs_actions.dart`)
   - The `_openVoteModal`, `_openProposeModal`, `_openConfirmModal`
     helpers and their imports of `time_poll_*_modal.dart`
   - The `showChooseTimeChip` / `showPollEndedBanner` flags and the
     `timePollActive` / `timePollCompleted` / `hasConfirmedTime`
     reads that feed only these surfaces
   This matches the comment at
   [`needs_section.dart:104-108`](../../app/lib/presentation/widgets/shared/needs_section.dart)
   that already declares location-poll banners intentionally absent
   from this surface.

Both changes land in phase 4 — the same client phase that adds the
finalized modal and unified routing — so the first tab and Details
tab move in lockstep with the rest of the poll-shape migration.

## Architecture & Compliance Notes

These rules from the reference docs apply across every phase and were
folded into the task lists below during the audit. Calling them out
once here so reviewers don't have to spot-check each phase.

- **MVVM + Riverpod 3.0** ([`docs/client/architecture.md`](../client/architecture.md)).
  Business logic lives in `TimeModalNotifier`; screens only `ref.watch` /
  `ref.read(.notifier)`. The existing `TimeModalNotifier` calls service
  methods directly — that mirrors `LocationModalNotifier` and is the
  established pattern for the poll modals. Do not introduce a new
  controller class; the rule from `architecture.md` is explicit ("there
  are no 'controllers' in the presentation layer").
- **`AsyncNotifier` is preferred** for async state. `TimeModalNotifier`
  is already `AsyncNotifier<TimeModalData>` — keep it that way. Methods
  that mutate state after an `await` must use `if (!ref.mounted) return;`
  guards before assigning to `state` (architecture.md §"Async Safety").
  All new mutation methods added in phases 2 and 3 inherit this rule.
- **No manual caches in ViewModels** ([`docs/client/caching.md`](../client/caching.md)).
  Poll data is loaded on-demand into the AsyncNotifier and is not
  Stash-cached today (it's per-experience and short-lived). Don't add a
  manual `Map<String, …>` to track optimistic state outside the
  notifier — the in-flight set + optimistic overlay belong in the
  notifier state itself. This is the explicit reason
  `timePollVoteSelectionProvider` is being deleted in phase 2.
- **All user-visible strings via `context.l10n.*`**
  ([`docs/client/i18n.md`](../client/i18n.md)). Every new label —
  Manage menu items, finalized card copy, header switch, "Add to
  calendar" CTA, change-menu actions — needs a key in both
  `app_en.arb` and `app_es.arb` (`npm run lint:dart:l10n-parity`
  enforces parity) and a `@key` metadata block in `app_en.arb`.
  Reuse `locationPollManage*` key shapes for the time equivalents
  (e.g. `timePollManageAddOption`, `timePollManageAddOptionDesc`).
  ViewModels never resolve strings — expose typed enums and let the
  widget call `context.l10n` in `build()`.
- **Accessibility primitives are mandatory** ([`docs/client/accessibility.md`](../client/accessibility.md);
  `scripts/check_a11y.js` allowlist is empty, so any new violation
  fails CI).
  - Vote rows use `Toggle` (selectable state), not `Tappable` — the
    selected/unselected state is part of the widget's semantic role.
    With binary voting (see Decisions), no extra long-press gesture
    or "more" affordance is needed.
  - The Manage button is `IconAction` with `tooltip` + `semanticsLabel`
    from `context.l10n`.
  - Manage menu items use `Tappable` (already true in
    `LocationPollManageMenuSheet`).
  - Open all sheets via `showAccessibleModal` (already the project
    standard; `prefer_accessible_modal` lint enforces it).
  - On successful vote commit, fire
    `SemanticAnnouncer.announce(context, context.l10n.timePollVoteRecorded)`
    so screen-reader users hear confirmation — auto-submit removes the
    "Save votes" button that previously served as audible feedback.
  - Any new animation passes its duration through
    `accessibleDuration(context, …)` so reduce-motion is honoured.
- **Testing is ViewModel-first** ([`docs/client/testing.md`](../client/testing.md)).
  Use `ProviderContainer` with Mockito-generated mocks
  (`@GenerateMocks([ExperienceService, ...])`) for every new
  `TimeModalNotifier` path. Dispose containers and reset mocks in
  `tearDown`. Add a disposal test for any new method that does work
  after an `await`. Coverage target: 90%+ on the notifier; widget
  tests for state-based UI branches only.
- **Modal chrome via `widgets/modal/glass/`** ([`docs/client/modals.md`](../client/modals.md)).
  `GlassSheet`, `GlassModalHeader`, `GlassFooterButtons`,
  `GlassChip`, etc. New finalized modal, Manage menu, and unified
  modal all compose from these — no raw `Color()` literals, no
  hardcoded radii. Per the standard glass-modal shape, the sheet
  inset / corner / max-height come from `ModalTheme`.
- **No `// removed` / `// new in this PR` / phase-marker comments**
  in any `*.dart` or `*.go` file (CLAUDE.md, enforced by
  `npm run lint:ephemeral-comments`).
- **Server work (phase 1)** must use
  `logging.LoggerWithContext(ctx)`, mask sensitive fields, propagate
  the request_id, and pick INFO/WARN/ERROR per
  [`docs/server/observability.md`](../server/observability.md). New
  `CommunityEventType` values must be classified in
  [`server/undo/registry.go`](../../server/undo/registry.go) before
  merge (CLAUDE.md §"Adding New CommunityEventType Values"). Each
  new RPC gets its own dedicated request/response messages.

## Implementation Plan

Five phases, in this order. **Phase 1 is server-only** and lands
first so every client phase can rely on the full RPC surface.
Client phases 2–4 each ship independently; phase 5 (extraction)
lands last with both flows already running on duplicated code so
the shared widgets are designed against two real consumers, not
one.

### Phase 1: Server — Time-poll Manage RPCs

**Goal:** Mirror the four location-side RPCs that don't yet exist for
time polls. Built as part of this work (not a separate issue), so the
client phases that follow have a complete server surface to call.

**Tasks:**
- [ ] Verify what already exists in
      [`proto/ripls/api/experience_service.proto`](../../proto/ripls/api/experience_service.proto)
      (lines 773–785 declare `ProposeTime`, `VoteOnTime`,
      `ConfirmTime`, `UnlockTime`, `CancelTimePoll`). Then declare
      the missing four to mirror the location set:
      `LockTimeProposals`, `NudgeTimePollVoters`,
      `DeleteTimeProposal`, `SetTimePollDeadline`. `UnlockTime`
      already exists; add a matching `LockTimeProposals` rather than
      renaming, to keep wire compatibility.
- [ ] Each RPC gets its own dedicated request/response message
      (CLAUDE.md §"Never reuse RPC request or response message
      types"). Match the shapes of the location equivalents in the
      same proto file.
- [ ] Implement handlers in [`server/services/experience/`](../../server/services/experience/)
      mirroring the location ones. Use
      `logging.LoggerWithContext(ctx)` with the operation name and
      relevant entity IDs (`experience_id`, `user_id`,
      `time_proposal_id`). Mask user identifiers via
      `logging.MaskEmail` / `logging.MaskToken`. Pick
      INFO/WARN/ERROR per
      [`docs/server/observability.md`](../server/observability.md)
      §"Appropriate Log Levels" (user-input errors → INFO/WARN;
      system failures → ERROR).
- [ ] Return Connect errors via `connecterr.Internal(ctx, op, err, kv...)`
      for internal failures (bare `connect.NewError(connect.CodeInternal, err)`
      is gated by `scripts/check_codeinternal.js`).
- [ ] Add corresponding `CommunityEventType` values and classify
      each in [`server/undo/registry.go`](../../server/undo/registry.go)
      per CLAUDE.md §"Adding New CommunityEventType Values":
      `Nudge` → `ClassificationClientOnly` (no inverse needed);
      `Lock` / `Unlock` → `ClassificationServerSnackbar` with
      `UndoLockTimeProposals` / `UndoUnlockTimeProposals` RPCs;
      `DeleteTimeProposal` → `ClassificationRetentionRestore`
      (soft-delete + restore via `DeletedMetadata`).
      Fill in matching `UndoData` variants in
      [`proto/ripls/models/undo.proto`](../../proto/ripls/models/undo.proto)
      and populate `Entry.ValidateUndoData`.
- [ ] Connect-Go integration tests for each new RPC behind the
      `integration` build tag (`//go:build integration` first line).
      Cover happy path, permission denied (non-organizer caller),
      poll-not-active errors, and the undo flows for Lock / Unlock.
- [ ] Run `npm run build` (golangci-lint + `go build ./...`) and
      `go build -tags=integration ./...` before pushing — required
      on any cross-package signature change.
- [ ] Generate Dart bindings via `npm run generate` so phase 2 has
      typed client methods to call.

**Validates:** Full RPC parity with location polls. Unblocks the
Manage menu in phase 3 (everything else in phases 2 and 4 already
works with the pre-existing five RPCs).

### Phase 2: Auto-submit on tap (binary)

**Goal:** Tapping a proposal in `TimePollVoteModal` fires the RPC
immediately, with optimistic UI. Single tap commits YES; second tap
clears. Identical gesture model to `LocationPollVoteModal`.

**Tasks:**
- [ ] Move optimistic vote state from `timePollVoteSelectionProvider`
      (a local batch buffer) into `TimeModalNotifier` — mirror
      `LocationModalNotifier`'s pattern with an in-flight set +
      optimistic overlay set. The state lives in the notifier, not
      in a side-provider (caching.md §"No manual caches in
      ViewModels"). The viewer-state shape changes from
      `Map<String, TimeVoteStatus>` to `Set<String>` of proposal
      IDs voted YES on (binary; matches `LocationModalData`).
- [ ] Add `voteOnTime(proposalId)` (singular, binary) to the
      notifier. Internally it sends a YES on first call and clears
      on second. Add `if (!ref.mounted) return;` guards after each
      `await` before assigning to `state`
      (architecture.md §"Async Safety in Notifiers"). Keep
      `voteOnTimes(Set<String>)` deprecated for one cycle if other
      callers exist; remove once unused.
- [ ] Rewrite `time_poll_vote_modal.dart` to drop "Save votes" — tile
      tap calls the singular notifier method directly. The bottom bar
      collapses to just the organizer controls (still inline in this
      phase; phase 3 replaces them with the Manage menu).
- [ ] Use `Toggle` (not `Tappable`) as the row primitive so the
      selected/unselected state is exposed semantically. Required
      `semanticsLabel` pulled from `context.l10n` (e.g.
      `l10n.timePollVoteRow`).
- [ ] Add `_inFlight` set in the modal to disable the row's tap target
      while the RPC is mid-flight, matching
      [`location_poll_vote_modal.dart:68`](../../app/lib/presentation/screens/experience/location_poll_vote_modal.dart#L68).
- [ ] Update header copy: `l10n.timePollVoteHeader` →
      `l10n.timePollVoteHeaderVoted` ("YOU'RE IN") once the viewer
      has any committed vote (mirror location header switch). New
      ARB keys in both `app_en.arb` and `app_es.arb`; `@key`
      metadata in `app_en.arb`.
- [ ] On successful commit fire
      `SemanticAnnouncer.announce(context, context.l10n.timePollVoteRecorded)`
      so screen-reader users hear confirmation now that the visible
      "Save votes" button is gone.
- [ ] Delete `timePollVoteSelectionProvider` and its notifier.
- [ ] Confirm `TimeVoteStatus` non-YES values are no longer
      surfaced anywhere on the client. Keep the proto enum
      untouched (wire compatibility; server still writes
      `STATUS_YES` only).
- [ ] Add ViewModel unit tests via `ProviderContainer` + Mockito
      mocks (testing.md §"ViewModel Testing"): single commit, second
      tap clears, optimistic reconcile on success, rollback on
      error, and a disposal test (start RPC, dispose container
      before completion, expect no crash).
- [ ] Widget test: tile tap toggles state without a Save button
      present; Save button widget is gone from the tree.
- [ ] Run `npm run lint:dart:a11y`, `npm run lint:dart:i18n`, and
      `npm run lint:dart:l10n-parity` locally before pushing.

**Validates:** Binary auto-submit voting works end-to-end with
optimistic UI and survives notifier disposal mid-RPC.

### Phase 3: TimePollManageMenuSheet (organizer parity)

**Goal:** Replace the inline "Cancel poll" / "Pick winning time" footer with
a Manage affordance that opens a bottom-sheet menu matching
`LocationPollManageMenuSheet`. Builds on phase 1's server RPCs.

**Tasks:**
- [ ] Add `TimePollManageAction` enum: `addOption, setFinal, nudge,`
      `toggleLock, cancel`.
- [ ] Build `TimePollManageMenuSheet` as a near-copy of
      `LocationPollManageMenuSheet` (icon + label + description rows,
      destructive flag for cancel, hide `addOption` when locked).
      Use `Tappable` (with `semanticsLabel` from `context.l10n`) for
      each row, exactly as `_ManageMenuItem` does today. Open via
      `showAccessibleModal` so focus restores on dismiss.
- [ ] Add new ARB keys for every Manage label and description in both
      `app_en.arb` and `app_es.arb`. Mirror the
      `locationPollManage*` key names: `timePollManageAddOption`,
      `timePollManageAddOptionDesc`, `timePollManageLock`,
      `timePollManageLockDesc`, `timePollManageUnlock`,
      `timePollManageUnlockDesc`, `timePollManageNudge`,
      `timePollManageNudgeDesc`, `timePollManageSetFinal`,
      `timePollManageSetFinalDesc`, `timePollManageCancelPoll`,
      `timePollManageCancelDesc`. `@key` metadata in `app_en.arb`.
- [ ] Wire the Manage button into `TimePollVoteModal` — replace the
      inline bottom-bar buttons with a single Manage button (same shape as
      location's organizer affordance). Implement as `IconAction` with
      `tooltip: l10n.timePollManageTooltip` and
      `semanticsLabel: l10n.timePollManageButton`.
- [ ] Add notifier methods: `lockTimeProposals` / `unlockTimeProposals` /
      `nudgeTimePollVoters` / `setTimePollDeadline` calling the new
      RPCs from phase 1. Each method follows the async-safety
      pattern: capture inputs, await,
      `if (!ref.mounted) return;`, then `state = …`.
- [ ] Surface a confirmation toast on each action via the existing
      `toast_helper.dart` using localized strings. Also call
      `SemanticAnnouncer.announce` so the action result is read out.
- [ ] ViewModel unit tests via `ProviderContainer` for each new
      notifier method, plus a disposal test on `nudgeTimePollVoters`
      (mirrors the RPC pattern most likely to outlive the modal).
- [ ] Widget test: organizer sees Manage button; voter does not. Each
      action dispatches the right notifier method. Verify the Cancel
      row renders with `AppColors.error` (destructive).

**Validates:** Organizer surface parity with location. Exercises the
phase-1 server endpoints end-to-end.

### Phase 4: Finalized modal, unified routing, and adjacent surfaces

**Goal:** Ship the user-visible parity end-to-end. Adds the dedicated
finalized modal, overhauls `UnifiedTimeModal` into a history-list
shape, updates routing in `experience_content_view.dart`, surfaces
poll options on the Event pane's time row, and removes the time-poll
surface from the Details tab. After this phase the two flows look
and feel the same to users.

**Tasks (poll modals + routing):**
- [ ] Build `TimePollFinalizedModal`: sage-tinted card with the winning
      time + "N of M chose this time" footer, faint pill row of losing
      proposals, primary CTA = **Add to calendar** (existing calendar
      export plumbing in `UnifiedTimeModal`), owner-only Change menu
      (swap directly via `TimePollProposeModal` in edit mode, or open a
      fresh poll). Compose from `GlassSheet` / `GlassModalHeader` /
      `GlassFooterButtons`; no `Color()` literals or hardcoded radii
      (modals.md §"Standard glass-modal shape").
- [ ] All copy on the finalized card via `context.l10n` — new keys in
      both ARB files: `timePollFinalizedTitle`,
      `timePollFinalizedFooter` (ICU plural for "N of M chose this
      time"), `timePollFinalizedAddToCalendar`,
      `timePollFinalizedChange`, `timePollFinalizedChangeMenuSwap`,
      `timePollFinalizedChangeMenuFreshPoll`. Use ICU plural format
      for the footer count.
- [ ] Replace the read-only branch in `TimePollVoteModal` (lines 411–451)
      with a redirect: if poll is finalized when the modal opens, pop and
      open `TimePollFinalizedModal` instead. Keep historical-poll viewing
      via the unified modal's history rows.
- [ ] Restructure
      [`UnifiedTimeModal`](../../app/lib/presentation/screens/experience/unified_time_modal.dart)
      to mirror
      [`UnifiedLocationModal`](../../app/lib/presentation/screens/experience/unified_location_modal.dart):
      confirmed-time top row + per-poll history list + dispatch on row
      tap. Move the owner's `TimeSetView` flow into a "Set time" entry
      row (opens `TimePollProposeModal` in single-option / edit mode).
      The notifier stays `AsyncNotifier`-shaped; the widget renders
      `.when(data:, loading:, error:)` directly.
- [ ] Update the time-chip dispatch in
      [`experience_content_view.dart`](../../app/lib/presentation/screens/experience/experience_content_view.dart)
      (`_showTimeModal`, `_showTimeModalNonOwner` around lines
      844–883) to match the location-chip pattern at lines 813–835:
      inspect poll state and route to `TimePollFinalizedModal.show` /
      `TimePollVoteModal.show` / `TimePollProposeModal.show`
      accordingly.

**Tasks (first tab — Event pane time row):**
- [ ] In
      [`experience_event_pane.dart`](../../app/lib/presentation/screens/experience/widgets/experience_event_pane.dart)
      `_buildReadContent` (lines 100–162), compute the active
      proposal count the same way the location row does
      (`activeProposals.length` from `exp.timeProposals` filtered
      to `exp.currentPollId`).
- [ ] When `timePollActive && !isCompleted` **and** there are ≥2
      proposals, change the time row's `value` from "Choose a time"
      to `context.l10n.timePollEventRowChoose(count)` —
      mirroring `locationPollEventRowChoose` (line 189). New ARB
      keys in both `app_en.arb` and `app_es.arb` with ICU plural
      handling.
- [ ] Replace `_PollVotePill` (Vote / Voted) with a Choose/Chosen
      pill modelled on `_LocationVotePill` (line 209–214). Same
      gesture: tap opens the vote modal directly. Keep
      `_PollVotePill` available for any remaining single-proposal
      poll state — the choose-from-N copy only applies when there
      is a real choice.
- [ ] Verify the row stays visually balanced: the trailing pill
      takes over the slot (no Add-to-Calendar icon while a poll is
      active, just like the location row drops the directions
      icon).

**Tasks (second tab — strip time poll from `NeedsSection`):**
- [ ] In
      [`needs_section.dart`](../../app/lib/presentation/widgets/shared/needs_section.dart),
      remove the time-poll banners block (lines 110–122 — both
      `PollBanner` and `PollEndedBanner`).
- [ ] Remove the "Choose time" suggestion chip and surrounding
      `showChooseTimeChip` plumbing (lines 72–73, 140, 146–176).
      The zero-state suggestions area keeps the LLM-generated need
      suggestions but no longer carries a poll affordance.
- [ ] Remove `showPollEndedBanner`, `hasConfirmedTime`,
      `timePollActive`, `timePollCompleted` reads from
      `_buildExperience` if nothing else consumes them after the
      banners/chips are gone.
- [ ] Remove the `_openVoteModal` / `_openProposeModal` /
      `_openConfirmModal` helpers (lines 231–265) and their
      imports of `time_poll_*_modal.dart`.
- [ ] Remove `_resolveChooseTimeTap` (lines 275–287) and the two
      `onChooseTimeTap:` parameters it feeds (lines 388, 426),
      then drop the corresponding parameter from
      [`needs_actions.dart`](../../app/lib/presentation/widgets/shared/needs_actions.dart)
      and all the action-button builders that pass it through.
- [ ] Drop the now-unused ARB keys (`planPollSuggestionTime`,
      `planSuggestionPollChip` if not used elsewhere). Run
      `grep -r` across `app/lib/` before deleting each key; if a
      string is used elsewhere it stays. Update both `app_en.arb`
      and `app_es.arb` in lockstep.
- [ ] Update the comment at
      [`needs_section.dart:104-108`](../../app/lib/presentation/widgets/shared/needs_section.dart)
      so it covers time *and* location absence from the Details
      tab, or delete it if the section becomes self-evident.

**Tasks (cross-cutting):**
- [ ] Use `NavigationHelpers` for any new full-screen navigation
      (none expected — all surfaces are bottom-sheet modals). Open all
      sheets via `showAccessibleModal` so focus is restored on
      dismiss. Wrap any new animations' `duration:` in
      `accessibleDuration(context, ...)` so reduce-motion is honoured.
- [ ] Update remaining ad-hoc call sites:
      [`inline_conversation_view.dart:624`](../../app/lib/presentation/widgets/content/inline_conversation_view.dart#L624).
      Direct sub-modal launches stay only where they make sense
      (notification deep links).
- [ ] ViewModel unit tests for the dispatch helper that picks
      finalized vs. vote vs. propose based on poll state. Widget
      tests for finalized modal rendering, change menu actions, the
      Event-pane time row's "Choose from N times" branch, and the
      removal of time-poll banners from the Details tab.
- [ ] Run `npm run lint:dart:a11y`, `npm run lint:dart:i18n`,
      `npm run lint:dart:l10n-parity`, `npm run lint:dart:size`,
      and `npm run lint:ephemeral-comments` before pushing.

**Validates:** End-to-end UX parity with location, with time-poll
affordances anchored on the Event pane (where time lives) rather
than scattered across the Details tab.

### Phase 5: Extract shared widgets

**Goal:** Pull the duplicated chrome from phases 2–4 into kind-agnostic
widgets under `app/lib/presentation/widgets/poll/` so future poll kinds
(or future drift between location and time) is impossible.

**Tasks:**
- [ ] Extract `PollManageMenuSheet<T>` — generic over the action enum;
      icon + label + description rows with destructive flag. Refactor
      both `LocationPollManageMenuSheet` and `TimePollManageMenuSheet`
      to compose it. Generic widgets accept ARB-resolved strings as
      parameters (i18n.md §"ViewModels never resolve strings" — the
      widget that constructs the generic resolves `context.l10n.*`
      and passes strings down).
- [ ] Extract `PollVoteRow` — slot for option body (place card vs.
      date+time chip), shared voter-avatar stack, shared YES/no-vote
      toggle. Both kinds are now binary (see Decisions), so the
      shared row has no status-axis variation. Use `Toggle` as the
      row primitive; required `semanticsLabel` is passed in by the
      caller.
- [ ] Extract `PollFinalizedCard` — sage-tinted card with name slot,
      footer "N of M chose this {option}", primary CTA slot, owner
      Change-menu slot. Caller provides auxiliary content (map vs.
      calendar widget) below the card. Use `AppColors` helpers for
      every surface color (no `Color()` literals; no `Colors.grey[…]`
      — architecture.md §"Colors and Theming").
- [ ] Extract `PollHistoryRow` and the grouping logic from
      `unified_location_modal.dart`'s `_groupByPoll` into a shared
      helper so `UnifiedTimeModal` consumes the same list pattern.
- [ ] Extract a shared `PollEventRowValue` helper (or equivalent)
      so the "Choose from N {options}" Event-pane row copy is the
      same shape for time and location and only the option-noun
      ARB key changes.
- [ ] Verify no regressions on location flow — full widget-test pass
      plus the existing location ViewModel test suite.
- [ ] Keep kind-specific pieces (the option body, the propose input,
      the finalized auxiliary content) on the kind-specific modals.
- [ ] Run the full lint suite: `npm run lint:dart:a11y`,
      `npm run lint:dart:i18n`, `npm run lint:dart:l10n-parity`,
      `npm run lint:dart:size`, `npm run lint:ephemeral-comments`,
      `npm run lint:nav`.

**Validates:** That the abstraction is the right shape — informed by
two real implementations rather than one. After this phase, adding a
third poll kind (e.g. budget poll, dress-code poll) is a small effort.

## Related Issues
- _None linked yet._ This plan is fully self-contained — the server
  work that was previously deferred to a companion issue now lives in
  phase 1.

---

## Audit Log (2026-05-23)

Audited against
[`docs/client/architecture.md`](../client/architecture.md),
[`docs/client/caching.md`](../client/caching.md),
[`docs/client/i18n.md`](../client/i18n.md),
[`docs/client/accessibility.md`](../client/accessibility.md),
[`docs/client/testing.md`](../client/testing.md),
[`docs/server/architecture.md`](../server/architecture.md),
[`docs/server/observability.md`](../server/observability.md).

Findings folded into the plan:

1. **Architecture & async safety.** Added an "Architecture & Compliance
   Notes" section before the plan and called out `AsyncNotifier` /
   `if (!ref.mounted) return;` guards on every new `await` →
   `state = …` site (phases 1, 2). The original plan recommended
   moving optimistic state into the notifier but didn't pin down the
   async-safety contract.
2. **Caching boundary.** Explicit reason for deleting
   `timePollVoteSelectionProvider` — `caching.md` forbids manual
   caches in ViewModels, so the staging provider can't morph into an
   optimistic-overlay store; that state belongs in `TimeModalNotifier`.
3. **i18n.** Every new user-visible string is now wired to
   `context.l10n.*` with new ARB keys listed by name (mirror the
   `locationPollManage*` keys, plus `timePollFinalized*` for phase 4).
   Both `app_en.arb` and `app_es.arb` must be updated for
   `npm run lint:dart:l10n-parity`; `@key` metadata in `app_en.arb`.
   ICU plural format flagged for the "N of M chose this time" footer.
4. **Accessibility.** Allowlist is empty, so any new violation fails
   CI. Updates: `Toggle` (not `Tappable`) for vote rows, `IconAction`
   with `tooltip` for the Manage button, `showAccessibleModal` for
   every sheet, `SemanticAnnouncer.announce` on vote commit (replaces
   the audible feedback the deleted Save button used to provide), and
   `accessibleDuration(context, ...)` on any new animation. (Note:
   the audit also added a long-press "more" affordance for screen
   readers to reach tri-state vote values; the post-review revision
   below dropped this when voting became binary.)
5. **Testing layout.** Aligned task lists to `testing.md`:
   ViewModel-first via `ProviderContainer` + Mockito-generated mocks,
   disposal tests for any new method that does work after an `await`,
   coverage target 90%+ on the notifier, widget tests reserved for
   state-based UI branches. Reset mocks in `tearDown`.
6. **Server / observability (phase 1, after revision).** Specified
   `logging.LoggerWithContext(ctx)`, `MaskEmail` / `MaskToken`,
   INFO/WARN/ERROR per `observability.md`, `connecterr.Internal` for
   internal errors, dedicated request/response messages per RPC (per
   CLAUDE.md), `CommunityEventType` classification in
   `server/undo/registry.go` (with first-pass guesses at each new
   event's classification bucket), and the `integration` build tag for
   the connect tests.
7. **Modal compliance.** Pointed the finalized modal and Manage sheet
   at the `widgets/modal/glass/` primitives and `ModalTheme` constants
   (per `docs/client/modals.md`) so they conform to the standard
   glass-modal shape without per-modal overrides.
8. **Lint gates.** Added explicit `npm run lint:*` commands at the
   end of phases 1, 3, and 4 so the agent doesn't push uncaught
   violations: `lint:dart:a11y`, `lint:dart:i18n`,
   `lint:dart:l10n-parity`, `lint:dart:size`,
   `lint:ephemeral-comments`, `lint:nav`.

Open questions and the lifecycle / phasing decisions were not changed
during the audit — the architectural rules don't speak to the gesture
choice (Question 1), the server-coupling decision (Question 2), or the
unified-time layout (Question 3). Those remain for the human reviewer.

## Revision Log (2026-05-23, post-review)

The issue owner reviewed the plan and supplied four directional
changes. All five open questions are now resolved (see "Decisions"
section). Plan changes:

1. **Voting is binary.** The tri-state premise was wrong — RSVP is
   tri-state, but voting on a proposed time is binary, just like
   voting on a proposed location. Removed the long-press / segmented
   / cycle-on-tap question entirely. Phase 2 task list simplified:
   `voteOnTime(proposalId)` (no status arg), viewer state is
   `Set<String>` (matches location), single tap commits / second tap
   clears. Phase 5's shared `PollVoteRow` no longer needs to bridge
   tri-state vs. binary.
2. **Server work is in-scope.** Old phase 5 ("server, separate
   issue") was moved to **phase 1** so all client phases can rely
   on a complete RPC surface. No separate issue is filed. Phase 3
   (Manage menu) lost its "gate on server work" task; it now
   straightforwardly calls the new RPCs.
3. **First-tab time row mirrors location.** Added a block of tasks
   to phase 4 that updates the Event pane's time row to render
   "Choose from N times" + Choose/Chosen pill when a time poll is
   active with ≥2 proposals, mirroring `locationPollEventRowChoose`
   at [`experience_event_pane.dart:189`](../../app/lib/presentation/screens/experience/widgets/experience_event_pane.dart#L189).
4. **Second-tab time poll removed.** Added a block of tasks to
   phase 4 that strips every time-poll surface from
   [`needs_section.dart`](../../app/lib/presentation/widgets/shared/needs_section.dart):
   `PollBanner`, `PollEndedBanner`, the "Choose time" suggestion
   chip, the `_openVoteModal` / `_openProposeModal` /
   `_openConfirmModal` helpers, the `_resolveChooseTimeTap` /
   `onChooseTimeTap` plumbing, and the unused ARB keys that fall
   out. Matches the location pattern (location-poll banners are
   intentionally absent from the Details tab).
5. **Phase numbering updated:**
   1 (server RPCs) → 2 (auto-submit binary) → 3 (Manage menu) →
   4 (finalized modal + unified routing + Event pane + strip
   Details tab) → 5 (extract shared widgets).
