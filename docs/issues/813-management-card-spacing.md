# Issue #813: [Bug Report] poor spacing and inconsistent fonts on management card

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/813
**Filed:** 2026-03-09 | **Last activity:** 2026-03-16
**Labels:** bug, user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The loan management modal (shown when an owner taps "Review" on a gear item for loan) has two visual bugs in its empty/no-requests state:

1. **Serif font with too little spacing**: The `TransferModalTitle` widget renders the gear name in Georgia (serif) with only 8px of spacing before the phase body content. This feels cramped compared to the active-loans state.
2. **Inconsistent font families**: The `_buildNoRequests` empty state in `LoanOwnerModal` uses the default sans-serif font at `fontSize: 24` for "No pending requests", while the gear name above uses Georgia serif. This creates a jarring mismatch within the same modal.

The maintainer comment clarifies: the top spacing should be consistent with how the modal looks when there ARE active loans (i.e., the request phase with borrower selection cards).

## Context from Comments

- **maintainer (maintainer)**: Provided a reference screenshot showing the desired spacing when active loans are present. The top area with the gear name and description has more breathing room in the active state. The empty state should match this spacing.

## Potential Solutions

### Option A: Fix spacing and font consistency in `_buildNoRequests`

- **Approach:** Adjust the `_buildNoRequests` empty state to: (1) use the same visual style as the transfer modal's active states (matching `TransferModalTitle` serif font), and (2) increase spacing between `TransferModalTitle` and the phase body from 8px to 16-24px to match active state spacing.
- **Pros:** Minimal, surgical change. Two files touched at most.
- **Cons:** None significant.
- **Effort:** Small.

### Option B: Refactor empty state to reuse `TransferModalTitle` directly

- **Approach:** Instead of a custom empty state widget, reuse `TransferModalTitle` for the gear name (already done) and replace the separate "No pending requests" heading with a simpler body-text message under it, eliminating the duplicate large heading entirely.
- **Pros:** Removes the root cause of the font inconsistency by eliminating the second large heading. Cleaner visual hierarchy.
- **Cons:** Changes the empty state layout more substantially.
- **Effort:** Small.

## Recommendation

**Option A** is the right fix. The `_buildNoRequests` method needs two changes:

1. Change the "No pending requests" title from `fontSize: 24` sans-serif to either match the serif style or reduce it to a secondary heading role (smaller sans-serif, like `titleMedium`).
2. Increase the `SizedBox(height: 8)` between `TransferModalTitle` and `_buildPhaseBody` in the `build` method to match the spacing visible in the active-loans reference screenshot (~16-24px).

The empty state already has the large circular icon + text pattern which works well visually; it just needs the font and spacing to be consistent.

## Open Questions

- **Should the "No pending requests" text use Georgia serif or stay sans-serif at a smaller size?** Recommendation: Use sans-serif at a smaller size (e.g., `titleMedium` / 16-18px) since the gear name in `TransferModalTitle` is the primary heading. Having two large serif headings would also look odd. The empty state title should be secondary to the gear name.
- **Should the spacing fix also apply to giveaway modals?** Recommendation: Yes, check `GiveawayGiverModal` for the same pattern since it likely shares the same `TransferModalShell` + `TransferModalTitle` structure with identical 8px spacing.

## Implementation Plan

### Phase 1: Fix spacing and font consistency

**Goal:** Match the empty state layout to the active-loans reference screenshot.

**Tasks:**
- [ ] In `loan_owner_modal.dart` `build()`: increase `SizedBox(height: 8)` between `TransferModalTitle` and `_buildPhaseBody` to `SizedBox(height: 16)` or `SizedBox(height: 24)`
- [ ] In `loan_owner_modal.dart` `_buildNoRequests()`: change the "No pending requests" title from `fontSize: 24, fontWeight: FontWeight.w700` to a smaller secondary heading (e.g., `fontSize: 18, fontWeight: FontWeight.w600`) to establish clear hierarchy below the serif gear name
- [ ] Check `giveaway_giver_modal.dart` for the same spacing issue and fix if present
- [ ] Verify the fix visually on Android (matching the reference screenshot from the maintainer comment)

**Validates:** The empty state and active-loans state have consistent spacing and no font family mismatch.

## Related Issues

- None identified.

---

## Audit

**Audited:** 2026-03-16 | **Result:** Pass — no changes required.

- **Architecture compliance:** Pure presentation-layer fix; no data layer, caching, or server boundary crossed.
- **Theming:** Existing code in `_buildNoRequests` already uses `AppColors` theme-aware helpers. Proposed changes stay within that pattern.
- **No new dependencies or patterns introduced.**
