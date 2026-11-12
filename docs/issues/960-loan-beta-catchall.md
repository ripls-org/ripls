# Issue #960: Loan catch-all issues for closed beta

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/960
**Filed:** 2026-03-31 | **Last activity:** 2026-04-01
**Labels:** P1 (High) | **Assignees:** maintainer
**Context docs:** none

## Problem Statement

The loan flow has multiple UX bugs and gaps that degrade the closed beta
experience. The issue covers 13 distinct items spanning the borrower pill
label, owner notifications, managing menu structure, completed loan
visibility, and modal cruft. These need to be addressed together as a P1.

Key themes:
1. **Borrower pill stuck on "requested"**: The action pill shows "Requested"
   regardless of the actual transfer state (picked up, returned). The label
   is hardcoded in `gear_content_view.dart:533` and only checks
   `hasUserTransfer()` — which is true for all non-terminal states.
2. **Missing/wrong menu items**: Upcoming borrowers see "confirm pickup" and
   "confirm return" when they shouldn't. Completed loans still show the
   borrower in the owner's menu. Owner doesn't see "arrange pickup" for the
   next borrower.
3. **Completed loans invisible**: Completed loans don't appear anywhere for
   either party — no history, no impact review.
4. **Modal cruft**: Loan management modal has tabs that are no longer needed;
   the issue says there shouldn't even be a management modal (actions should
   live on the managing menu like giveaways).
5. **Missing notification**: Owner doesn't receive a notification when someone
   requests to borrow.
6. **Borrower impact**: Borrower should automatically see impact on loan
   conclusion.

## Context from Comments

No comments on this issue.

## Sub-issues Breakdown

| # | Item | Area |
|---|------|------|
| 1 | Recipient button should say "you've requested" | Client: pill label |
| 2 | Owner should receive notification on borrow request | Server: notifications |
| 3 | Owner button should show number of people interested | Client: pill label |
| 4 | Owner management menu needs "arrange pickup" for next borrower | Client: managing menu |
| 5 | Upcoming borrowers shouldn't show "confirm pickup" or "confirm return" | Client: managing menu |
| 6 | Upcoming borrowers can plan pickup | Client: managing menu |
| 7 | Borrower pill still says "requested" after marked as picked up | Client: pill label |
| 8 | Borrower pill still says "requested" after marked as returned | Client: pill label |
| 9 | No tabs at the top of the management modal — old cruft | Client: loan modal |
| 10 | Actually shouldn't even be a management modal | Client: loan modal |
| 11 | Borrower should automatically see impact on conclusion | Client: completion flow |
| 12 | Completed transfers shouldn't show borrower in owner's menu | Client/Server: menu filtering |
| 13 | Completed loans don't show up for anyone | Client: loan history |

## Codebase Analysis

### Borrower Pill Label (Items 1, 7, 8)

**File:** `app/lib/presentation/screens/gear/gear_content_view.dart:513-545`

The pill label is hardcoded as `'Requested'` with `postActionLabel: 'Requested'`.
It checks `hasUserTransfer()` to determine if the user has any transfer, but
doesn't inspect the transfer's actual state. The label should change based on
state: "Requested" → "Pickup Scheduled" → "Borrowed" → "Returned".

### Owner Notification (Item 2)

**File:** `server/services/community/notifications.go`

The `shouldNotify` function already returns `true` for
`TRANSFER_INTEREST_EXPRESSED`. The `getNotificationRecipients` function returns
the gear owner for this event type. **This should already work** — loans
auto-approve on interest expression (INTEREST_EXPRESSED → RECIPIENT_SELECTED),
and the RECIPIENT_SELECTED event notifies the recipient. The owner IS notified
via the INTEREST_EXPRESSED event. Need to verify the notification title/body
make sense for loans vs giveaways.

### Owner Managing Menu (Items 4, 5, 6, 12)

**File:** `app/lib/presentation/screens/gear/gear_content_view.dart:1540-1630`

The owner's loan menu shows borrower sections built from `activeLoan`,
`userTransfer`, and `pendingRequests`. Issues:
- Upcoming borrowers (INTEREST_EXPRESSED) show the same pickup/return
  sub-items as the active borrower — they shouldn't.
- No "arrange pickup" action for the next borrower in queue.
- Completed transfers appear in `pendingRequests` because the server's
  `GetGearTransferContext` includes them.

### Loan Management Modal (Items 9, 10)

**File:** `app/lib/presentation/screens/gear/loan_manage_modal.dart`

Has a 3-tab structure (Arrange Pickup, Picked Up, Returned). The issue says
this should be eliminated entirely — actions should live on the managing menu
checklist like giveaways now do. The `ArrangePickupModal` is already available
separately for pickup scheduling.

### Completed Loan Visibility (Item 13)

**File:** `server/services/transfer/context.go:105-106`

For non-owners, `GetGearTransferContext` explicitly filters out COMPLETED and
CANCELLED transfers from `userTransfer`. This means the borrower has no way to
see their completed loan. The owner's context includes COMPLETED transfers but
they're mixed with active ones.

### Borrower Impact (Item 11)

When `CompleteTransfer` is called for a loan return, the response includes
impact metrics. But the UI doesn't display them — the borrower just sees the
pill revert to a stale state.

## Potential Solutions

### Option A: Incremental fixes within current architecture

- **Approach:** Fix each item individually: dynamic pill labels, filter
  completed transfers from menu, remove modal tabs, add impact display.
- **Pros:** Low risk per change; targeted fixes.
- **Cons:** The loan management modal removal (item 10) is a larger refactor
  that touches the TransferModalRouter flow.
- **Effort:** Medium.

### Option B: Align loans with the giveaway pattern (recommended)

- **Approach:** Apply the same simplification done for giveaways in #959:
  remove the loan management modal entirely, move all actions to the managing
  menu checklist, make pill labels state-aware, and fix server filtering.
- **Pros:** Consistent UX between loans and giveaways; eliminates modal cruft;
  follows the pattern already established and tested in #959.
- **Cons:** Larger scope; the loan flow is more complex than giveaways (has
  ACTIVE state, return step, multiple borrowers).
- **Effort:** Medium-large.

### Option C: Keep modal but fix bugs only

- **Approach:** Fix the pill label, notification, and filtering bugs. Keep the
  modal but remove tabs and simplify.
- **Pros:** Smallest scope; minimal risk of breaking existing flows.
- **Cons:** Doesn't address the fundamental UX issue (item 10 says "shouldn't
  even be a management modal").
- **Effort:** Small-medium.

## Recommendation

**Option B** — align loans with the giveaway pattern. The giveaway
simplification in #959 already proved the pattern works: actions on the
managing menu, no workflow modal, state-aware pill labels. Applying the same
approach to loans gives users a consistent experience and eliminates the
confusing tabbed modal.

The loan flow adds complexity (ACTIVE state, return step, loan queue) but the
managing menu already handles these via checklist items — the modal is
redundant.

## Open Questions

- **Q1: Should completed loans be visible somewhere?** *Resolved*: No new
  mechanism needed. Completed loans should show impact cards as they already
  do — no "Past Loans" section or special history view.

- **Q2: What should the borrower pill say in each state?**
  Recommendation:
  - INTEREST_EXPRESSED / RECIPIENT_SELECTED: "Pickup Scheduled" (loans
    auto-approve, so "requested" is misleading)
  - ACTIVE: "Borrowed"
  - COMPLETED: "Returned" (if visible) or revert to "Borrow" (if not)

- **Q3: What should the owner pill show for interest count?** *Resolved*:
  Show count (e.g. "3 people interested") on the owner pill. Also update
  the giveaway pill to show the same interest count.

- **Q4: Should the LoanBorrowerModal and LoanOwnerModal be kept?**
  Recommendation: Remove them. The TransferModalRouter should no longer open
  loan modals. For borrowers, tapping the pill on an item they haven't
  requested should just call expressInterest (like giveaways). For owners,
  the managing menu handles everything.

## Audit Notes

Changes made during `/ripls-audit` review:

- **[i18n]** Phase 1: All new pill labels must be l10n strings in both
  `app_en.arb` and `app_es.arb`. ViewModel should expose transfer state
  as typed enum; screens resolve to `context.l10n.*` in `build()`.
- **[caching]** Phases 2, 4: Completed-transfer filtering changes on the
  server must be paired with client cache invalidation. The existing
  `transferCacheInvalidationProvider` (from #959) handles real-time
  updates via FCM; pull-to-refresh uses `refreshGearDetails()`. No new
  invalidation mechanism needed, but ensure `GetGearTransferContext`
  response changes don't break cached assumptions.
- **[architecture]** Phase 3: Removing loan modals means the
  `TransferModalRouter` loan path should match giveaway behavior:
  express interest → return `true` (no modal). Owner never gets a modal.
  Navigation must use `NavigationHelpers.pushScreen()` if any detail
  screens are pushed, never raw `Navigator.push`.
- **[server]** Phase 4: Adding a new proto field for completed transfers
  in `GetGearTransferContextResponse` follows the existing API pattern
  (separate fields, not overloaded lists). Keep API and models protos
  independent per CLAUDE.md rules.
- **[testing]** All phases: Server changes need table-driven Go tests.
  Client ViewModels need `ProviderContainer` + mock repository tests per
  `docs/client/testing.md`. Pill label state logic is prime candidate for
  a unit-tested helper function.
- **[theming]** Phase 1: Pill colors must use `AppColors` theme-aware
  helpers, not hardcoded `Colors.*`.

## Implementation Plan

### Phase 1: Fix borrower pill labels

**Goal:** Pill label reflects actual transfer state.

**Tasks:**

- [ ] Replace hardcoded "Requested" label with state-aware labels:
  RECIPIENT_SELECTED → "Pickup Scheduled", ACTIVE → "Borrowed",
  COMPLETED → "Returned"
- [ ] For loans, auto-expressing interest should update the pill without
  showing a modal (matching giveaway behavior from #959)
- [ ] Add l10n entries for all new pill labels in both `app_en.arb` and
  `app_es.arb`; ViewModel exposes transfer state enum, screen resolves
  to `context.l10n.*` in `build()`
- [ ] Use `AppColors` theme-aware helpers for any pill color changes
- [ ] Unit test the state-to-label mapping logic

**Validates:** Borrower always knows their current loan status from the pill.

### Phase 2: Fix managing menu for loans

**Goal:** Owner and borrower see correct actions per state.

**Tasks:**

- [ ] Upcoming borrowers (INTEREST_EXPRESSED / RECIPIENT_SELECTED before
  active loan): show only "Arrange Pickup", not "Confirm Pickup" or
  "Confirm Return"
- [ ] Add "Arrange Pickup" action for next borrower in queue
- [ ] Filter completed transfers out of the borrower sections in the owner's
  managing menu
- [ ] Show interest count on the owner's pill or managing button
- [ ] Ensure `transferCacheInvalidationProvider` fires on loan state changes
  so the other party's menu updates in real time (already wired from #959)

**Validates:** Owner sees actionable items only; no stale completed borrowers.

### Phase 3: Remove loan management modal

**Goal:** Eliminate the tabbed loan modal; all actions on managing menu.

**Tasks:**

- [ ] Remove `LoanManageModal` and its tab structure
- [ ] Update `TransferModalRouter` loan path to match giveaway pattern:
  borrower taps pill → express interest → return `true` (no modal).
  Owner never gets a modal from the pill.
- [ ] Remove `LoanOwnerModal` and `LoanBorrowerModal` if no longer needed
- [ ] Ensure `ArrangePickupModal` is accessible from the managing menu for
  both owner and borrower (already wired for giveaways; verify loan
  callers pass correct params)
- [ ] Any navigation to detail screens must use
  `NavigationHelpers.pushScreen()`, not raw `Navigator.push`

**Validates:** No modal opens for loan management; consistent with giveaway UX.

### Phase 4: Completed loan visibility and impact

**Goal:** Both parties can see completed loans and impact.

**Tasks:**

- [ ] Update server `GetGearTransferContext` to include completed transfers
  in a separate field (not mixed with active) — new proto field on the
  API response, keeping API and models protos independent
- [ ] Show completed loans in owner's managing menu as "Past Loans" section
- [ ] Borrower sees impact metrics after loan return (either auto-navigate to
  impact view or show inline on gear detail)
- [ ] Verify owner notification on borrow request works correctly (may
  already work — test and fix title/body if needed)
- [ ] Server tests: table-driven Go tests for completed transfer filtering
- [ ] Client tests: ViewModel tests with `ProviderContainer` and mock
  repositories for completed loan display logic

**Validates:** Complete loan lifecycle is visible to both parties.

## Related Issues

- #959 — Giveaway catch-all (same pattern of simplification applied there
  first; loans should follow the same approach)
