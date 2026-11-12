# Issue #1686: Select final time from poll

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1686
**Filed:** 2026-05-03 | **Last activity:** 2026-05-10
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

A poll organizer who has run an "Ask the group" time poll wants a clear,
discoverable way to **pick a winning time and convert it into the event's
confirmed time**. Today the only owner-side action surfaced inside the
vote modal is "End Poll", which the user reports "seems to just delete
it." The reporter is on Android prod build 0.17.0 (#5625).

The underlying server flow already supports the feature in two distinct
RPCs: `ConfirmTime` (sets `experience.time` to a chosen proposal,
flips `time_poll_active=false`, sets `time_poll_completed=true`,
preserves all proposals and votes), and `CancelTimePoll` (flips the
same flags but does **not** set a time — proposals/votes are preserved
server-side too). The UI also already has a `TimePollConfirmModal`
that calls `lockTime`/`ConfirmTime`, plus a `PollEndedBanner` that
opens it. The bug is therefore not a missing RPC or screen — it is an
**entry-point / discoverability flaw in the vote-modal "End Poll"
flow**:

1. The bottom-bar action for organizers is labeled "End Poll" with red
   destructive styling. There is no peer "Pick winning time" CTA.
2. The confirmation dialog body says **"Existing votes will be lost."**
   even though the server preserves them. This actively misinforms the
   user.
3. After `cancelTimePoll` returns, the modal silently switches to a
   read-only "Poll results · Final results" view whose only action is
   `Close`. There is no in-modal CTA pointing the owner at the next
   step.
4. The only path to `TimePollConfirmModal` is the `PollEndedBanner` on
   the Plan tab's `needs_section`. An owner who closes the modal on a
   different tab, or whose Plan tab has the banner scrolled below the
   fold, never finds it. The banner also disappears once the experience
   already has a time, hiding it from anyone who *thinks* they ended the
   poll without picking a winner and then sets a time the old way.

The fix is to give the organizer a single, obvious "End poll & pick
winning time" path inside the vote modal itself, while keeping a clearly
separated "Cancel poll without picking" escape hatch and correcting the
misleading "votes will be lost" copy.

## Context from Comments

The two issue comments are bare `@claude` triggers from a maintainer
(maintainer) on 2026-05-09 and 2026-05-10 — no additional
clarification or scope guidance. The issue body is the only direction.
Treat the user's words as the authoritative requirement and the
maintainer's silence as license to make a reasonable UX call.

## Potential Solutions

### Option A: Reshape the vote modal's owner bottom bar (recommended)

- **Approach:** Replace the single red "End Poll" button with two
  organizer actions in the active-poll vote modal bottom bar:
  1. **Primary CTA** — "Pick winning time" (sage green, calls
     `TimePollConfirmModal.show`, which already exists and already
     calls `confirmTime` on the server, ending the poll atomically).
  2. **Secondary CTA / overflow** — "Cancel poll" (low-emphasis text
     button or kebab menu), which preserves today's
     `cancelTimePoll` behavior for the case where the owner genuinely
     wants to scrap the poll without picking a winner.
  Also extend the read-only / "poll already ended" branch of the same
  modal: when `timePollCompleted && !hasConfirmedTime && isOwner`,
  show a "Pick winning time" CTA in place of (or alongside) the bare
  "Close" button, so an owner who already ended the poll without
  picking can recover from inside the modal without hunting for a
  banner.
- **Pros:** Smallest behavior change. Reuses every existing
  asset (`TimePollConfirmModal`, `confirmTime` RPC, `PollEndedBanner`,
  the read-only results view). The user's reported confusion goes
  away because the obvious primary action is "pick the winner."
  Backwards-compatible: the banner-driven flow keeps working, the
  destructive cancel keeps working, and unrelated callers
  (`event_joiner_modal`, `rsvp_modal`, `experience_content_view`,
  `inline_conversation_view`, `unified_time_modal`) don't change.
- **Cons:** Adds visual weight to the vote modal's bottom bar.
  Requires non-trivial layout work to fit "Pick winning time",
  voter "Save N", and a "Cancel poll" affordance for the owner
  without crowding the non-owner UI.
- **Effort:** Small.

### Option B: Multi-step "End poll" dialog

- **Approach:** Keep the single "End Poll" button but on tap open a
  bottom sheet that asks "How do you want to end the poll?" with two
  buttons: "Pick winning time" (opens the existing confirm modal) and
  "Cancel without picking" (calls today's `cancelTimePoll`). Drop the
  current `AlertDialog`.
- **Pros:** No layout pressure on the vote modal's bottom bar. Forces
  the owner to make an explicit choice at end-of-poll time.
- **Cons:** Hides the primary action behind one more tap and a confirm
  step. Trains users to miss the picker entirely on first encounter
  (the "End Poll" wording still primes them for destruction). Adds a
  third surface to maintain.
- **Effort:** Small.

### Option C: Auto-transition the vote modal into the confirm modal after `cancelTimePoll`

- **Approach:** When `endTimePoll()` succeeds and at least one
  proposal exists, swap the modal contents in place to the confirm
  modal (`TimePollConfirmModal`-equivalent) with an explicit "Skip,
  don't pick a time" escape. No layout change to the active-poll
  bottom bar.
- **Pros:** Minimal change to active-poll UI. Forces a follow-up
  prompt every time a poll is ended.
- **Cons:** Couples two server states together that weren't designed
  to compose (`cancelTimePoll` + `confirmTime`). Forces the owner
  through a second modal even when they intentionally wanted to
  cancel. Confusing for accessibility (modal contents change without
  navigation).
- **Effort:** Medium.

## Recommendation

Take **Option A**. It is the smallest, most discoverable change, reuses
every existing asset (`TimePollConfirmModal` / `confirmTime`
/ `PollEndedBanner`), and keeps the destructive-cancel path explicitly
available without making it the default. Pair it with a copy fix to the
`timeEndPollWithVotes` localization (which is currently misleading —
votes are preserved by the server) and a layout pass on the bottom bar
so the owner sees three affordances (Pick winning time, Cancel poll,
Save vote) without overloading the non-owner two-button layout.

## Open Questions

- **Should the read-only branch of `TimePollVoteModal` add an owner
  "Pick winning time" CTA, or rely solely on the `PollEndedBanner`?** —
  Recommendation: **add it**. Owners who get to the read-only view
  via the modal flow have already shown they look for next-step CTAs
  there; the banner is a backup for users who closed the modal first.
- **Where should the secondary "Cancel poll" affordance live?** —
  Recommendation: a low-emphasis text button to the left of "Pick
  winning time" with a confirmation dialog. Avoids a kebab menu, which
  the Ripls Flutter UI doesn't otherwise use for time actions, and
  keeps the destructive option visible but de-emphasized.
- **Should the "Pick winning time" CTA be enabled when zero proposals
  have any YES votes?** — Recommendation: **yes**. The organizer can
  still legitimately pick a final time even if nobody voted; the
  confirm modal already lets them select any proposal regardless of
  vote count.
- **Do we update the `timeEndPollWithVotes` copy to say "Voters can
  still see their selections" instead of "Existing votes will be
  lost."?** — Recommendation: **yes**, this is currently a factual
  bug. Update both `app_en.arb` and `app_es.arb` (Spanish has a
  parallel string) and regenerate `app/lib/l10n/app_localizations*.dart`.
- **Should we add a system-message / chat event when an owner picks a
  winning time *via the vote modal* vs. via the banner?** —
  Recommendation: **no**. The server's `ConfirmTime` already emits a
  `CHAT_SYSTEM_ACTION_DETAIL_CHANGED` message — entry point doesn't
  matter.

## Implementation Plan

### Phase 1: Bottom-bar redesign + read-only owner CTA

**Goal:** Surface "Pick winning time" inside the vote modal so a poll
owner has an obvious in-modal path to confirming a final time, both
during an active poll and after the poll has been ended without a
time.

**Tasks:**
- [ ] In `app/lib/presentation/screens/experience/time_poll_vote_modal.dart`,
      `_buildBottomBar` (active-poll branch around line 420): replace
      the current organizer "End Poll" Tappable with two organizer
      affordances stacked horizontally. Use the `Tappable` primitive
      (already used elsewhere in this file) with
      `semanticsLabel: context.l10n.*` for both — never raw
      `GestureDetector`/`InkWell` (lint rule
      `avoid_raw_gesture_detector` will fail otherwise), and never a
      string literal in `semanticsLabel` (lint rule
      `require_l10n_for_semantic_labels`):
      - Primary: "Pick winning time" sage-green Tappable that calls
        `TimePollConfirmModal.show(context, experienceId)` after
        dismissing the current sheet.
      - Secondary: a smaller "Cancel poll" text-button styled
        affordance (kept red but visually de-emphasized) that calls
        the existing `_endPoll(data)` flow.
      Preserve the non-owner "Save N" CTA on the right unchanged.
- [ ] Same file, `_buildBottomBar` read-only branch (around line 392):
      when `data.isOrganizer && data.timePollCompleted &&
      !(data.eventTime?.hasSpecific() ?? false) &&
      !(data.eventTime?.hasRange() ?? false)` (i.e. the owner ended
      the poll but no time is confirmed), show a "Pick winning time"
      `Tappable` (sage green, `semanticsLabel:
      context.l10n.timePollConfirmTitle` or new key) in place of the
      bare "Close" button. Otherwise keep the existing "Close"
      button.
- [ ] Verify the corresponding owner gating in
      `time_poll_confirm_modal.dart` does not need changes — it does
      not, because `lockTime` already calls `confirmTime` which
      enforces owner-only on the server.
- [ ] **Testing.** Per `docs/client/testing.md`, ViewModel tests are
      preferred over widget tests; this change is purely UI wiring
      (no new branching in `TimeModalNotifier`), so add a thin widget
      test in
      `app/test/presentation/screens/experience/time_poll_vote_modal_test.dart`
      using `app/test/helpers/l10n_helpers.dart` for the
      `localizationsDelegates`/`supportedLocales` setup. Cover:
      (a) organizer + active-poll shows both "Pick winning time" and
      "Cancel poll" buttons; (b) tapping "Pick winning time" opens
      `TimePollConfirmModal`; (c) non-organizer + active-poll only
      shows "Save"; (d) read-only owner branch with
      `timePollCompleted=true` and no confirmed time shows
      "Pick winning time"; (e) read-only branch with a confirmed
      time still shows just "Close". Override `timeModalProvider`
      via `ProviderContainer.overrides` to feed deterministic
      `TimeModalData` fixtures rather than mocking the underlying
      services. If the wiring grows any state-dependent logic (e.g.
      a derived "owner can pick winner" getter on `TimeModalData`),
      relocate it to `time_modal_view_model.dart` /
      `time_modal_state.dart` and add a ViewModel test in
      `app/test/presentation/viewmodels/time_modal_view_model_test.dart`.

**Validates:** The reported "End Poll just deletes it" experience is
gone — the primary CTA is obvious and converts a poll into a confirmed
time. Existing destructive cancel still reachable.

### Phase 2: Localization correctness

**Goal:** Stop telling users that ending the poll deletes votes when
the server preserves them, and add the strings the new affordances
need.

**Tasks:**
- [ ] Update `app/lib/l10n/app_en.arb`:
      - `timeEndPollWithVotes` → "Voters can still see results, but
        no winning time will be recorded."
      - Add `timePollConfirmCtaShort` (or reuse `timePollConfirmTitle`
        / `timeEndPoll`) for the in-modal primary button — pick a
        copy that fits the bottom-bar width.
      - Add `timePollCancelPollShort` for the de-emphasized
        secondary button.
- [ ] Mirror the additions in `app/lib/l10n/app_es.arb` (the Spanish
      file has parallel strings — search for the same `@`-keyed
      entries).
- [ ] Regenerate localizations via `npm run generate` (which runs
      `flutter gen-l10n` per `app/l10n.yaml`). **Do not** commit the
      regenerated `app/lib/l10n/app_localizations*.dart` files — they
      are gitignored (see `app/.gitignore`) and rebuilt by CI.

**Validates:** Confirmation dialog no longer lies about data loss; new
buttons have proper i18n.

### Phase 3: Telemetry and analytics (optional, owner discretion)

**Goal:** Confirm in production that the new CTA is being used and
that "End Poll" cancellations drop in volume.

**Tasks:**
- [ ] Optionally fire a tracked event when `lockTime` is invoked from
      `TimePollVoteModal` vs. from the `PollEndedBanner` so we can
      see entry-point usage. Reuse whichever analytics surface the
      Ripls app already uses — do **not** add a new analytics
      framework.

**Validates:** Adoption signal that this fix actually closed the
discoverability gap.

## Related Issues

- None directly linked, but search the open backlog for "time poll" /
  "ask the group" / "experience time" before merging in case a parallel
  redesign is being planned. The maintainer triggering `@claude` here
  is the owner of that area; they will know.

---

### Audit notes (applied 2026-05-10)

- **Architecture (MVVM):** Plan reuses the existing
  `TimeModalNotifier.lockTime` (calls `confirmTime`) and
  `TimePollConfirmModal` — no new business logic in widgets,
  matching `docs/client/architecture.md`. ✅
- **Caching:** No data-layer changes; nothing to audit against
  `docs/client/caching.md`. ✅
- **i18n:** Plan amended to clarify that
  `app/lib/l10n/app_localizations*.dart` are gitignored (per
  `app/.gitignore`) and must NOT be committed; only the `.arb`
  source files are committed and `npm run generate` regenerates the
  Dart code. ✅
- **Accessibility:** Plan amended to require `Tappable` (not raw
  `GestureDetector`/`InkWell`) and `context.l10n.*` for every new
  `semanticsLabel`, per the empty-allowlist policy in
  `docs/client/accessibility.md`. ✅
- **Testing:** Plan amended to flag that this is UI-only wiring with
  no new ViewModel logic, so a thin widget test (using
  `app/test/helpers/l10n_helpers.dart` for the localizations
  delegate setup) is appropriate; if any new derived state ends up
  on `TimeModalData`, the plan calls out moving the test to a
  ViewModel test, matching the "ViewModel-first" principle in
  `docs/client/testing.md`. ✅
- **Server / observability:** No server changes (no new RPCs,
  logging fields, or storage queries) — out of scope for
  `docs/server/architecture.md` and `docs/server/observability.md`.
  ✅
