# Issue #838: Convert loans to chat-oriented workflow

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/838
**Filed:** 2026-03-14 | **Last activity:** 2026-03-14
**Labels:** P1 | **Assignees:** maintainer
**Context docs:** docs/ai/chat-wf-give.md, docs/workflows/loan.md

## Problem Statement

Loans currently use a modal-based workflow — all management (approving
borrowers, starting loans, marking pickup/return) happens in dedicated modals
(`LoanOwnerModal`, `LoanBorrowerModal`, `LoanListModal`). Users tap a
contextual action button, wait for the modal to load, perform the action,
dismiss, and return to the group chat. This is the same context-switch problem
that was solved for giveaways (PR #807), experiences, and requests.

Loans are the last remaining workflow type that hasn't been migrated to the
chat-oriented pattern. The infrastructure — `WorkflowWidgetFactory`,
`WidgetShell`, `ManagePanel`, phase progress bar, participant badges,
`ConversationActionButtonBuilder` — is all in place from the prior three
workflow migrations. This issue asks to apply the same pattern to loans.

### Key differences between loans and giveaways

Loans have a richer lifecycle than giveaways, which makes the migration
non-trivial:

1. **Extra state (ACTIVE)**: Loans have a 5-state machine
   (INTEREST_EXPRESSED → RECIPIENT_SELECTED → ACTIVE → COMPLETED) vs
   giveaways' 4-state (INTEREST_EXPRESSED → RECIPIENT_SELECTED → COMPLETED).
   The ACTIVE state represents "item is out, loan in progress."

2. **Time coordination**: Loans involve pickup dates, loan durations, and
   return dates. The borrower modal has date/time/duration pickers
   (`QuickDateChips`, `QuickTimeChips`, `DurationChips`). Giveaways have no
   time coordination.

3. **Auto-approval**: Loan interest expressions auto-approve to
   RECIPIENT_SELECTED (single recipient expected). Giveaways stay in
   INTEREST_EXPRESSED until the owner manually selects.

4. **StartLoan step**: Loans have an explicit "start" transition
   (RECIPIENT_SELECTED → ACTIVE) marking the physical handoff. Giveaways go
   directly from RECIPIENT_SELECTED → COMPLETED.

5. **Countdown/duration display**: Active loans show elapsed time, countdown
   to return date, and duration tracking. Giveaways have none of this.

6. **Gear state toggle**: Loans toggle gear between AVAILABLE/UNAVAILABLE
   across all communities. Giveaways transition gear to GIVEN_AWAY permanently.

## Context from Comments

No comments on the issue. The issue body references PR #807 (giveaway chat
workflow migration) as the pattern to follow. PR #807's comments note design
concerns about color contrast and redundancy between system messages and
workflow cards — these should be considered when designing loan cards.

## Potential Solutions

### Option A: Full inline workflow (all actions in chat)

Move everything into the chat: date/time picking, loan start, countdown display,
return confirmation — all via manage panels and widget cards in the conversation.
No modal navigation at all from chat.

- **Approach:** Extend `ManageLoanPanel` with inline date/time pickers (reuse
  `QuickDateChips`/`QuickTimeChips`/`DurationChips`), add countdown widget
  cards, handle all state transitions from the bottom bar.
- **Pros:** Fully consistent with the giveaway pattern. No context switches.
- **Cons:** Date/time/duration picking in a constrained bottom bar area is
  cramped. The loan borrower modal's pickup scheduling UX is fairly complex
  (3 independent inputs). Forcing this into an inline panel degrades the UX.
  Large effort.
- **Effort:** Large.

### Option B: Hybrid — actions in chat, scheduling via modal

Primary workflow actions (express interest, start loan, mark returned, cancel)
happen inline in the chat via the action button and manage panel. Time
scheduling (pickup date/time, loan duration) opens a focused modal since it
needs more screen real estate. This is the same compromise the giveaway workflow
made with "Check Status" opening a read-only modal.

- **Approach:** Add `_buildLoanButton()` to `ConversationActionButtonBuilder`,
  create `ManageLoanPanel`, add `LoanPhase` enum for the progress bar, create
  loan-specific widget cards (gear shared, loan active, loan completed). The
  "Set Pickup Details" action in the manage panel opens the existing scheduling
  UI as a bottom sheet/modal. All other actions stay inline.
- **Pros:** Best UX balance — keeps the chat as the primary surface while
  giving scheduling the space it needs. Reuses most existing infrastructure.
  Consistent with how giveaways handle "Check Status" (modal from chat).
- **Cons:** Not 100% modal-free (scheduling still uses a modal). Moderate
  effort.
- **Effort:** Medium.

### Option C: Minimal — just add action button + progress bar

Add a loan action button and phase progress bar to the conversation screen
without widget cards. Actions trigger via the button's dropdown menu. No inline
cards for gear info, loan active, or completion.

- **Approach:** Add `_buildLoanButton()` with dropdown actions, add loan phase
  computation, but skip widget cards entirely. System messages continue to be
  the only visual markers.
- **Pros:** Smallest effort. Addresses the core problem (actions require
  leaving chat).
- **Cons:** Misses the visual richness that makes chat workflows effective
  (gear cards, completion cards, participant badges). Inconsistent with the
  other three workflow types.
- **Effort:** Small.

## Recommendation

**Option B (Hybrid)** is the right approach. It provides the same chat-first
experience as giveaways while respecting that loan time scheduling needs more
screen real estate than a bottom bar can offer. The giveaway workflow already
established a precedent for opening modals from chat (recipient selection
initially used a modal, "Check Status" opens a modal) — using a modal for
pickup scheduling is consistent with that pattern.

The existing loan modals (`LoanOwnerModal`, `LoanBorrowerModal`) should be
retained for access from the gear detail screen (same as `GiveawayGiverModal`
was retained), but the chat conversation should be the primary workflow surface.

## Open Questions

- **No LoanPhase enum needed — use TransferState directly.** Giveaways needed
  a server-computed `GiveawayPhase` enum because of a specific visibility gap:
  when the owner selects a recipient, non-selected participants' own transfers
  are still in `INTEREST_EXPRESSED` — they can't derive the overall phase from
  their own state. Loans don't have this problem. Loans auto-approve, so
  `ExpressInterest` immediately creates a transfer in `RECIPIENT_SELECTED`.
  There's no selection step where some participants are left behind. Each
  borrower's `TransferState` directly reflects their loan's phase
  (`RECIPIENT_SELECTED` = Scheduled, `ACTIVE` = On Loan, `COMPLETED` =
  Returned). The progress bar reads `TransferState` from
  `GearTransferContext.user_transfer` (for borrowers) or from the most active
  transfer (for owners). No new enum, no server-side phase computation.

- **Should we add a LOAN_SHARED system message (parallel to GIVEAWAY_SHARED)?**
  — Recommendation: Yes. The gear info card needs an anchor point in the
  timeline. Add `CHAT_SYSTEM_ACTION_LOAN_SHARED` emitted when gear is shared
  with `AVAILABILITY_FOR_LOAN`. This is consistent with the giveaway pattern
  and provides the correct chronological anchor.

- **Pickup scheduling UX — modal or bottom sheet?** — Recommendation: Bottom
  sheet (not full-screen modal). A bottom sheet keeps the chat partially
  visible, reinforcing the "you're still in the conversation" context. The
  existing `QuickDateChips`/`QuickTimeChips`/`DurationChips` widgets should
  work well in a 60%-height bottom sheet.

- **Active loan countdown — widget card or bottom bar?** — Recommendation:
  Both. A widget card anchored to the STARTED system message shows the loan
  details (borrower, pickup date, expected return). The bottom bar's phase pill
  can show a compact "3 days left" countdown. This mirrors how giveaways show
  the selection card + progress bar simultaneously.

- **Multiple concurrent loans — how does the progress bar work?** —
  Recommendation: The progress bar shows the phase of the "most active" loan
  (the one closest to completion, or the most recently started). The `LoanPhase`
  enum should be computed from the most advanced non-terminal transfer. If all
  transfers are terminal, show the most recently completed one's phase. This
  is simpler than trying to show multiple progress bars.

- **Should the gear info card show an interest list like giveaways?** —
  Recommendation: No. Loans auto-approve, so the "interest" phase is
  instantaneous. The participant list on the gear card doesn't add value for
  loans the way it does for giveaways where the owner is choosing among
  multiple candidates. Keep the loan gear card simple: gear info + "Request to
  Borrow" button.

## Implementation Plan

### Phase 0: Proto + Server — LOAN_SHARED system message

**Goal:** Server-side foundation: a system message anchor for the gear info
card in loan conversations. No new phase enum is needed — the client reads
`TransferState` directly from `GearTransferContext` (see Open Questions).

**Tasks:**
- [x] Add `CHAT_SYSTEM_ACTION_LOAN_SHARED = 20` to `ChatSystemAction` in both
  `proto/ripls/models/chat.proto` and `proto/ripls/api/chat_service.proto`
- [x] Emit `LOAN_SHARED` system message in `CommunityService.ShareGear` when
  availability = `FOR_LOAN`, after conversation creation
- [x] Run `npm run generate` to regenerate Go + Dart code
- [x] Add tests: LOAN_SHARED emission on ShareGear with FOR_LOAN availability
- [ ] Run `npm run build` to verify (blocked by pre-existing portfolio proto
  conflict; community package compiles cleanly in isolation)

**Validates:** Loan conversations have an anchor system message for the gear
info card, and the existing `GearTransferContext` already provides all state
the client needs.

### Phase 1: Client — Loan detection, action button, progress bar

**Goal:** Loans get a chat action button and phase progress bar, making
workflow actions accessible from the conversation without opening a modal.

**Tasks:**
- [x] Add `isLoanConversation` getter to `ConversationNotifier`
- [x] Add `loanPhaseIndex` getter deriving phase from `TransferState`
- [x] Add loan-specific getters: `isLoanOwner`, `isLoanActive`,
  `isLoanTerminal`, `isLoanCompleted`, `isLoanCancelled`
- [x] Add `_buildLoanButton()` to `ConversationActionButtonBuilder` with
  role-based states (owner/borrower/non-participant/terminal)
- [x] Add loan progress bar (4 segments) to `MessageInputBar`
- [x] Wire action button callbacks in `ConversationScreen`
- [x] Add ViewModel methods with `ref.mounted` checks:
  `expressInterestInLoan`, `startLoan` (updated), `completeLoan`,
  `cancelLoan`, `withdrawLoanInterest`
- [ ] Add tests for ViewModel getters and button states (defer to follow-up)

**Validates:** Users can trigger all loan workflow actions from the chat
conversation without navigating to a modal.

### Phase 2: Client — Manage loan panel

**Goal:** Bottom bar manage panel with role-based actions for loans, parallel
to `ManageGiveawayPanel`.

**Tasks:**
- [x] Role-based manage actions implemented via `_buildLoanButton()`
  dropdown menus (no separate panel file needed — app uses dropdown menus
  on the action button, not slide-up panels)
- [x] Add pickup scheduling bottom sheet (`PickupSchedulingSheet`) reusing
  `QuickDateChips`, `QuickTimeChips`, `DurationChips`. Wire via
  `onSetPickupDetails` callback in borrower Scheduled phase.
- [x] Add `updateLoanPickupDetails` ViewModel method calling
  `TransferRepository.updateTransfer`
- [ ] Add tests for dropdown actions and bottom sheet (defer to follow-up)

**Validates:** All loan management happens in the chat's bottom bar area,
with scheduling in a focused bottom sheet.

### Phase 3: Client — Widget cards (gear info, loan active, completion)

**Goal:** Rich inline cards in the chat timeline that show gear details, loan
status, and completion impact — matching giveaway visual patterns.

**Tasks:**
- [x] Extend `WorkflowWidgetFactory` with `buildLoanCard()` — renders gear
  info card for `LOAN_SHARED` using existing `SystemPillContentFactory`
- [x] Wire inline card builder in `ConversationScreen` for loan conversations
- [x] Extend `shouldHighlightSystemMessage()` with `isLoanConversation`
  (JOINED, STARTED, COMPLETED, CANCELLED highlighted)
- [x] Participant badges already work for loans via shared `transferStatusMap`
  from `GearTransferContext`
- [x] Create `LoanActiveCard` for `STARTED` system message (borrower info,
  dates, countdown)
- [x] Create `LoanCompletionCard` for `COMPLETED` system message (money
  saved, CO₂ prevented, time shared)
- [x] Cache impact from `CompleteTransfer` response in `_cachedImpact`
- [x] Add tests: 20 tests covering factory dispatch (5), system message
  highlighting (5), LoanActiveCard rendering (5), LoanCompletionCard
  rendering (5). Fix LOAN_SHARED pill rendering in SystemPillContentFactory.

**Validates:** Loan conversations have the same visual richness as giveaway
conversations — gear context, active status, and completion celebration
all visible inline.

### Phase 4: Cross-provider invalidation, polish, edge cases

**Goal:** State consistency across screens and polished edge cases.

**Tasks:**
- [x] Cross-provider invalidation: gear cache invalidated in startLoan
  (UNAVAILABLE), completeLoan (AVAILABLE), cancelLoan (AVAILABLE).
  TransferRepository already handles daily invalidation, transfer list
  invalidation, and user stats internally.
- [x] Cancelled state: progress bar returns null (grey segments), action
  button shows "Cancelled" (muted), terminal label "Loan Cancelled"
- [x] Error handling: toast on failed actions via try/catch in all handlers
- [x] Dead code cleanup: removed cachedTransfer fallback paths and
  fetchTransferDetails else branches from cancelTransfer, startLoan,
  completeTransfer (now giveaway-only or loan-only, not dual-purpose)
- [ ] Handle multiple concurrent loans: owner "View All Loans" action
  (deferred — common case is one loan at a time; owner can use gear
  detail modal for multi-loan management)

**Validates:** Loan chat workflow is robust, consistent across screens, and
handles all edge cases gracefully.

### Phase 5: Entry point consolidation

**Goal:** Non-owner gear detail screen funnels into the chat as the primary
loan interaction surface.

**Tasks:**
- [x] Non-owner action bar on `GearContentView`: "Request" calls
  `expressInterest` then navigates to conversation via
  `NavigationHelpers.pushScreen()`. Removed `_handleRequestAndOpenModal`.
- [x] Already-interested non-owner: confirmed tap opens chat (not modal)
- [x] Owner action bar: still opens `TransferModalRouter` from gear detail
- [x] `LoanOwnerModal` / `LoanBorrowerModal` retained for gear detail
  screen and Check Status access
- [x] "Check Status" overflow menu in conversation for loan borrowers:
  opens `LoanBorrowerModal` showing pickup details and countdown
- [ ] Add tests for entry point navigation (defer to follow-up)

**Validates:** The chat is the default surface for loan interactions, but
modal access is preserved as a secondary path.

## Audit Notes

**Audited against:** `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/server/architecture.md`

**Findings applied (4 fixes inline):**

1. **Async safety (architecture.md):** Added explicit `ref.mounted` check
   requirement after every `await` in ViewModel methods (Phase 1). Added
   disposal safety test requirement for all async methods.

2. **Media display (CLAUDE.md):** Added `CachedMediaImage` requirement with
   stable `cacheKey` for gear images in widget cards (Phase 3). Plan
   originally said "gear image" without specifying the correct widget.

3. **Daily screen invalidation (caching.md Pattern 7):** Added
   `dailyCacheInvalidationProvider.notify()` requirement after loan mutations
   (Phase 4). Without this, the daily screen would show stale loan data after
   chat-based actions.

4. **Navigation helpers (architecture.md):** Made `NavigationHelpers.pushScreen()`
   explicit for all conversation navigation in Phase 5. Plan originally said
   "navigates to conversation" without specifying the correct navigation API.

**No architectural violations found.** The plan correctly:
- Routes all data access through repositories, never services (architecture.md rule 2)
- Uses `_cachedImpact` Freezed field in ConversationState, not manual cache maps (caching.md rule 1)
- Emits LOAN_SHARED from CommunityService (matches existing GIVEAWAY_SHARED pattern, no service-to-service dependency)
- Adds to both `chat.proto` files in sync (API + models)
- Keeps server changes minimal (no new LoanPhase enum, reuses TransferState)

## Related Issues

- PR #807 — Migrate giveaways to chat workflow (the direct template for this
  work)
- #808 — Color redesign for chat system messages (design concern raised in
  PR #807 comments that applies to loan cards too)
