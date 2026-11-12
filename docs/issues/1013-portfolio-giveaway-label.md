# Issue #1013: Profile screen shows active giveaway as a loan

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1013
**Filed:** 2026-04-03 | **Last activity:** 2026-04-03
**Labels:** none | **Assignees:** tescobar
**Context docs:** none

## Problem Statement

When a user has an active giveaway, the portfolio/profile screen mislabels it as a loan. The status badge says "On Loan", the filter chip buckets it under "On Loan", and there is no "Giveaway" filter chip. Tapping the item opens the gear screen, but the portfolio card itself gives the wrong impression of the item's state.

The root cause is that three components in the portfolio display layer only check `Transfer.state` and never check `Transfer.transferType`:

1. **`PortfolioItemCard._gearStatus()`** ([portfolio_item_card.dart:419-440](app/lib/presentation/widgets/portfolio/portfolio_item_card.dart#L419-L440)) — returns "On Loan" for any `TRANSFER_STATE_ACTIVE` transfer regardless of type.
2. **`ProfileGearState._gearChipFilter()`** ([profile_gear_view_model.dart:308-320](app/lib/presentation/viewmodels/profile_gear_view_model.dart#L308-L320)) — routes active giveaways to `ChipFilter.onLoan`.
3. **`PortfolioItemsScreen` filter chips** ([portfolio_items_screen.dart:292-296](app/lib/presentation/screens/portfolio/portfolio_items_screen.dart#L292-L296)) — only offers "All", "Available", "On Loan", "Pending" with no giveaway category.

The `TransferType` information is already available — `Transfer.transferType` is populated in `ProfileGearState.transfersByGearId` — it's just not being used in the display logic.

## Context from Comments

No comments on this issue. The issue description is detailed and self-contained with a clear root cause analysis. No additional clarification needed.

## Potential Solutions

### Option A: Add transfer-type-aware labels (no new chip)

- **Approach:** Update `_gearStatus()` in `PortfolioItemCard` and `_gearChipFilter()` in `ProfileGearState` to check `Transfer.transferType`. When the transfer is a giveaway, display "Giveaway" instead of "On Loan" in the badge, but keep giveaways bucketed under the existing "On Loan" chip (rename it to "Active" to be type-neutral).
- **Pros:** Minimal UI change — just rename one chip and update badge text. Users see correct labels immediately. No new filter state to manage.
- **Cons:** Loans and giveaways are still lumped into one filter bucket, so users with both can't filter them independently.
- **Effort:** Small.

### Option B: Rename "On Loan" chip to "Active" and add type-aware badges

- **Approach:** Same as Option A but also update the badge colors to visually differentiate giveaways from loans (e.g., a distinct color for "Giveaway" vs "On Loan" badges). The "Active" chip encompasses both transfer types.
- **Pros:** Clear visual distinction between loans and giveaways. The "Active" label is accurate for both types. Simple chip model — no new enum values.
- **Cons:** Still can't filter loans vs giveaways independently. Adds a new color to the badge palette.
- **Effort:** Small.

### Option C: Add a dedicated "Giveaway" chip

- **Approach:** Add a new `ChipFilter.giveaway` enum value and a "Giveaway" chip alongside "On Loan". Update `_gearChipFilter()` to route giveaway transfers to the new chip. Update badge text accordingly.
- **Pros:** Full filtering capability — users can view only giveaways or only loans.
- **Cons:** More chips means more horizontal scrolling. The chip bar is already at 4 items; adding a 5th may feel crowded. Also requires handling the chip count logic for the new filter. The portfolio also shows requests and experiences — adding granular transfer-type chips only for gear items may feel inconsistent.
- **Effort:** Medium.

## Recommendation

**Option B** — rename "On Loan" to "Active" and add type-aware badges with distinct colors. This fixes the mislabeling bug with minimal UI disruption. The "Active" chip is semantically correct for both loans and giveaways (both represent items currently in a transfer). The distinct badge colors give users the visual signal they need without adding chip bar complexity.

If users later request the ability to filter loans vs giveaways independently, Option C can be layered on top.

## Open Questions (Resolved)

- **What color should the "Giveaway" badge use?** — **Decision:** Use `AppColors.giveawayColor` (sage green `#7A9B76`) with appropriate alpha for the badge background. This color already exists in the theme and is used for giveaway availability indicators elsewhere. The loan badge should use `AppColors.loanColor` (stone blue `#5A7A82`) for consistency, replacing the current `AppColors.primary(context).withValues(alpha: 0.85)`. Both colors are already defined as shared semantic constants in [app_colors.dart:72-73](app/lib/core/theme/app_colors.dart#L72-L73).
- **Should "Pending" also differentiate between loan-pending and giveaway-pending?** — **Decision:** Yes. Use the same giveaway/loan color distinction for pending badges: giveaway-pending gets sage green, loan-pending gets stone blue (or the existing gold). Badge text should also reflect the type.
- **Should the "Requested" state also be differentiated?** — **Decision:** Keep as-is. "Requested" is neutral language that works for both transfer types.

## Audit Notes

> **[AUDIT — i18n]** All user-visible strings ("Giveaway", "On Loan", "Active", "Pending", etc.) **must** use ARB localization via `context.l10n.*`, not hardcoded Dart strings. Add new keys to both `app_en.arb` and `app_es.arb` before running `npm run generate`. The existing `_gearStatus()` method already hardcodes English strings ("On Loan", "Pending", "Requested", etc.) — this is a pre-existing i18n violation. Fix it as part of this work: add `portfolio*` prefixed keys for all badge labels. See `docs/client/i18n.md`.
>
> **[AUDIT — theming]** Badge colors in `_gearStatus()` currently use hardcoded hex values (`Color(0xD9B8860B)`, `Color(0x99FFFFFF)`, etc.). Per `docs/client/architecture.md`, use `AppColors` semantic helpers for theme-aware colors. Add new semantic colors (e.g., `AppColors.giveawayBadge(context)`) if needed, rather than introducing more hardcoded hex values. The one exception is `AppColors.primary(context).withValues(alpha: 0.85)` which is already theme-aware.
>
> **[AUDIT — viewmodel strings]** Per i18n architecture, viewmodels must not resolve display strings. `_gearStatus()` lives in the widget layer (`PortfolioItemCard`), which is correct — it has `BuildContext` access. However, ensure the chip label mapping in `PortfolioItemsScreen._buildFilterChips()` also uses `context.l10n` rather than hardcoded strings in the `chips` list.

## Implementation Plan

### Phase 1: Add i18n keys for badge and chip labels
**Goal:** All portfolio badge and chip labels are localizable.
**Tasks:**
- [ ] Add ARB keys to `app_en.arb` and `app_es.arb` for: "On Loan", "Giveaway", "Active", "Pending", "Requested", "Available", "Not Shared" (badge labels) and "All", "Available", "Active", "Pending" (chip labels). Use `portfolio*` prefix per naming convention (e.g., `portfolioBadgeOnLoan`, `portfolioBadgeGiveaway`, `portfolioChipActive`).
- [ ] Run `npm run generate` to regenerate `AppLocalizations`
- [ ] Update `PortfolioItemCard._gearStatus()` to use `context.l10n.*` instead of hardcoded strings
- [ ] Update `PortfolioItemsScreen._buildFilterChips()` chip labels to use `context.l10n.*`
**Validates:** All user-visible portfolio strings are localized. No hardcoded English strings remain in badge/chip code.

### Phase 2: Fix badge labels, colors, and chip rename
**Goal:** Active giveaways display localized "Giveaway" (not "On Loan") with a theme-aware distinct badge color; chip renamed to "Active".
**Tasks:**
- [ ] Update `PortfolioItemCard._gearStatus()` to check `activeTransfer.transferType` and return giveaway-specific localized label + theme-aware color when type is `TRANSFER_TYPE_GIVEAWAY`
- [ ] Replace hardcoded hex colors in `_gearStatus()` with `AppColors` semantic helpers (add new helpers if needed, e.g., `AppColors.giveawayBadge(context)`)
- [ ] Update the "Pending" state to also check transfer type and return giveaway-pending label when applicable
- [ ] Rename the chip label from "On Loan" to "Active" in `PortfolioItemsScreen._buildFilterChips()` (line 295) — use the new `context.l10n.portfolioChipActive` key
- [ ] Optionally rename `ChipFilter.onLoan` enum value to `ChipFilter.active` for code clarity (update all references)
- [ ] Verify that `activeTransfer` passed to `PortfolioItemCard` includes `transferType` (it does — comes from `ProfileGearState.transfersByGearId`)
- [ ] Verify chip counts still work correctly (giveaways + loans both counted under the renamed chip)
**Validates:** Giveaway items show correct badge text and color in the portfolio grid. All colors are theme-aware. Chip label is accurate for both transfer types.

### Phase 3: Add unit tests
**Goal:** Prevent regression — ensure badge and chip logic handles both transfer types.
**Tasks:**
- [ ] Add unit tests for `_gearChipFilter()` covering loan and giveaway transfers routing to the correct chip bucket
- [ ] Verify existing portfolio tests still pass
**Validates:** Transfer-type-aware logic is covered by tests.

## Related Issues
- #972 — Active giveaways sometimes disappear from the top of the feed (the feed bug that surfaces this secondary symptom)
