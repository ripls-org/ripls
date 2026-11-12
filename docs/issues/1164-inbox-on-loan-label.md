# Issue #1164: Inbox shows items "on loan" but only interest has been expressed

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1164
**Filed:** 2026-04-17 | **Last activity:** 2026-04-17
**Labels:** bug, user-feedback | **Assignees:** tescobar
**Context docs:** docs/client/inbox.md

## Problem Statement

The inbox displays misleading status information for transfers that are still in
the early `INTEREST_EXPRESSED` state. Two separate labeling issues combine to
create confusion:

1. **Title suffix is always "Lending"/"Borrowing"** regardless of transfer
   state. The `assembleTransfer` function in `assembly.go:489-502` hard-codes
   the title as `"{GearName} — Lending"` (owner) or `"{GearName} — Borrowing"`
   (recipient) for all non-giveaway transfers, even when only interest has been
   expressed and no loan is active.

2. **`RECIPIENT_SELECTED` is labeled "On loan"** prematurely. The
   `transferStateLabels` function in `assembly.go:561-562` returns
   `stateLabel="On loan"` for both `RECIPIENT_SELECTED` and `ACTIVE` states,
   but `RECIPIENT_SELECTED` means the owner has accepted the request — the
   physical handover hasn't happened yet.

The user-reported bug ("Inbox shows items 'on loan' but only interest has been
expressed") could stem from issue 1 (the title says "Lending" in
`INTEREST_EXPRESSED` state) or from both issues together if transfers are
advancing to `RECIPIENT_SELECTED` too quickly.

## Context from Comments

No comments on this issue. The bug was filed via in-app feedback with
screenshots that show the inbox displaying loan-related labels for items where
only interest was expressed. The user is a tester on the dev environment.

## Potential Solutions

### Option A: State-aware titles + separate RECIPIENT_SELECTED label

- **Approach:** Make the title suffix reflect the actual transfer state, and
  distinguish `RECIPIENT_SELECTED` from `ACTIVE` in `transferStateLabels`.
  - `INTEREST_EXPRESSED` owner title: `"{Name}"` (no suffix — the status label
    "Borrow Request" already communicates the state)
  - `INTEREST_EXPRESSED` recipient title: `"{Name}"` (status label shows
    "Loan Requested")
  - `RECIPIENT_SELECTED` title: `"{Name} — Accepted"` (owner) /
    `"{Name} — Accepted"` (recipient)
  - `RECIPIENT_SELECTED` stateLabel: `"Accepted"` instead of `"On loan"`
  - `ACTIVE` keeps `"On loan"` and title `"Lending"`/`"Borrowing"`
- **Pros:** Titles and status labels are consistent and accurate at every state.
  Users see clear progression: no suffix → Accepted → Lending/Borrowing.
- **Cons:** Slightly more complex title logic. Existing users see different
  titles after the fix.
- **Effort:** Small.

### Option B: Remove title suffix entirely, rely on status labels

- **Approach:** Always use just the gear name as the title (no suffix). The
  `inboxStatusLabel` already communicates the role and state ("Borrow Request",
  "On Loan", "Lending", etc.).
- **Pros:** Simplest change. No redundant information between title and status.
- **Cons:** Loses the at-a-glance role indicator for `ACTIVE` transfers where
  "Lending"/"Borrowing" is actually accurate. Giveaway titles also use suffixes
  ("Giving"/"Received") which would need the same treatment for consistency.
- **Effort:** Small.

### Option C: Fix only the RECIPIENT_SELECTED state label

- **Approach:** Only change `transferStateLabels` to return a distinct label for
  `RECIPIENT_SELECTED` (e.g. `"Accepted"`) while leaving titles unchanged.
- **Pros:** Minimal change, fixes the "On Loan" mislabel for accepted-but-not-
  handed-over transfers.
- **Cons:** Doesn't address the title saying "Lending"/"Borrowing" during
  `INTEREST_EXPRESSED`, which is likely what the user is actually seeing.
- **Effort:** Small.

## Recommendation

**Option A** — state-aware titles with a separate `RECIPIENT_SELECTED` label.
This fixes both symptoms: the premature "Lending"/"Borrowing" title in
`INTEREST_EXPRESSED` state and the premature "On Loan" label in
`RECIPIENT_SELECTED` state. The implementation is straightforward and the title
progression (no suffix → Accepted → Lending/Borrowing) maps cleanly to the
user's mental model of the loan lifecycle.

## Open Questions

- **What should the title suffix be for `INTEREST_EXPRESSED`?** —
  Recommendation: No suffix (just the gear name). The `inboxStatusLabel`
  ("Borrow Request" / "Loan Requested") already communicates the state, and
  adding a suffix like "— Requested" would be redundant.

- **Should giveaway titles follow the same pattern?** — Recommendation: Yes,
  for consistency. `INTEREST_EXPRESSED` giveaways should show just the gear
  name (status label already says "Giveaway Request"), and `ACCEPTED` giveaways
  keep "— Giving"/"— Received".

- **Does the `resolveInboxStatusLabel` need a new case for `RECIPIENT_SELECTED`?**
  — Recommendation: Yes. When `stateLabel` changes from `"On loan"` to
  `"Accepted"` for `RECIPIENT_SELECTED`, the existing `case stateLabel == "Accepted"`
  already returns `"Borrowing"` which is appropriate for the non-owner view.
  For the owner view, `resolveInboxStatusLabel` falls through to the default
  `"Lending"` which is acceptable, but a dedicated `"Pickup Pending"` or
  `"Accepted"` label might be clearer. Keep it simple: `"Lending"` for owner,
  `"Borrowing"` for non-owner at `RECIPIENT_SELECTED` state.

## Audit Notes

_Audited against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, and `docs/server/architecture.md`._

- **Server architecture: compliant.** All changes are within a single service
  package (`server/services/portfolio/`). No cross-service dependencies
  introduced.
- **Client architecture: no client changes needed.** `InboxRow` already displays
  whatever `inboxStatusLabel`, `title`, and `actionDescription` the server sends.
  This is a server-only fix.
- **Caching: no changes needed.** Inbox data flows through
  `PortfolioRepository` with `portfolio:inbox_feed` cache key. Label changes
  propagate on the next fetch or pull-to-refresh.
- **i18n: follows existing pattern.** All inbox labels are currently hardcoded
  server-side English strings (`resolveInboxStatusLabel`, `resolveTypeLabel`,
  title assembly). This is technical debt for future i18n, but the plan follows
  the established pattern.
- **Side effect on `resolveActionDescription`:** Changing `RECIPIENT_SELECTED`
  from `stateLabel="On loan"` to `"Accepted"` means the owner-side
  `resolveActionDescription` branch for `stateLabel == "On loan"` (assembly.go:431)
  will no longer fire at `RECIPIENT_SELECTED`. This is **correct** — the item
  isn't borrowed yet — but the plan must verify the description is appropriate
  at each state. Added as an explicit task below.

## Implementation Plan

### Phase 1: Fix transfer state labels ✅
**Goal:** Accurate inbox status labels at every transfer state.
**Tasks:**
- [x] In `transferStateLabels`: split `RECIPIENT_SELECTED` from `ACTIVE`. Return `("Accepted", "")` for `RECIPIENT_SELECTED`. Keep `("On loan", "")` only for `ACTIVE` loans.
- [x] Update `resolveInboxStatusLabel` to handle `"Accepted"` with `isOwner` differentiation.
- [x] Titles are **never modified** to reflect state — always role-based suffix ("— Lending", "— Borrowing", etc.).
- [x] Add unit tests for `transferStateLabels`, `resolveInboxStatusLabel`, and title assembly across all states.
- [x] Run `npm run build` to verify lint and compilation pass.

### Phase 2: Inbox pills mirroring content view ✅
**Goal:** Every inbox row shows a pill communicating why the item is there.
**Tasks:**
- [x] Add `resolveInboxPill` function mirroring content view action buttons (e.g. "{N} interested", "Going", "Helping").
- [x] Group owner transfers by gear — multiple borrowers for the same gear produce ONE inbox row with aggregated interest count, people, and unread counts.
- [x] Add `transferStatePriority` for selecting the primary transfer in a group.
- [x] Apply `resolveInboxPill` to all inbox items (transfers, experiences, requests) in `assembleFeedItems`.
- [x] Client: show the pill in `inbox_row.dart` regardless of whether `actionDescription` is set.
- [x] Add tests for `resolveInboxPill`, `transferStatePriority`, and owner gear grouping.
- [x] Update `docs/client/inbox.md` with title and pill rules.

### Phase 3: Verify end-to-end
**Goal:** Confirm the fix works in the running app.
**Tasks:**
- [ ] Run server tests: `go test ./server/services/portfolio/...`
- [ ] Manually test on dev: create a transfer, verify inbox labels at each state transition (`INTEREST_EXPRESSED` → `RECIPIENT_SELECTED` → `ACTIVE` → `COMPLETED`).
- [ ] Verify giveaway labels follow the same pattern.
- [ ] Verify pills show for experiences (RSVP counts) and requests (Review/Helping).

## Related Issues
- #1013 — Giveaway label fix in portfolio (similar label accuracy concern)
- #987 — Giveaway pill status (related state label work)
