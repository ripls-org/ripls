# Gear Details Tab Redesign

## Problem Statement

The Details tab on `GearContentView` currently shows a minimal layout that does not match the redesigned experience and request Details tabs or the reference designs:

1. **Side-by-side detail cards**: Location and time are rendered as two custom `_DetailCard` widgets in an `IntrinsicHeight` row — the same pattern that experience and request views had before their redesigns. The reference design shows a single glass card with location + optional website rows separated by a divider (matching the shared `ContentInfoCard`).

2. **Flat metadata rows**: Brand, value, material, and weight are rendered as plain `ContentDetailRows` key/value text. The reference design shows these in a separate glass card with the same row/divider treatment, but the values are right-aligned and bold — a more structured "specs card" look.

3. **No borrower/people section**: The Details tab shows no information about who has borrowed the item. The reference design shows an avatar stack with "X borrowing · X returned" summary that opens a borrower list modal (analogous to the attendee/helper rows on experience/request).

4. **No impact metrics**: The Details tab shows no impact data. The reference design includes "POTENTIAL IMPACT" tiles (Money Saved, CO₂ Avoided, Quality Time) identical to the experience and request Details tabs, with tappable formula bottom sheets. The `GetGearStats` RPC already returns `impact` and `potential_impact` — this data just isn't loaded or displayed.

5. **No giveaway-specific modal**: For giveaway gear, the people modal should show "interested" parties rather than "borrowers", with a button to express interest. This is analogous to the borrower modal but adapted for the giveaway workflow.

---

## Current State

### Details tab layout (`gear_content_view.dart:576-617`)

```
┌─────────────────────────────────┐
│ LOCATION card │ TIME card       │  ← IntrinsicHeight row (_DetailCard × 2)
├─────────────────────────────────┤
│ Brand         Arc'teryx         │
│ Value         $225              │  ← ContentDetailRows (flat key/value)
│ Material      Fabric            │
│ Weight        290 g             │
│ Website       arcteryx.com      │
├─────────────────────────────────┤
│ [thumb] [thumb] [+]             │  ← ContentMediaCarousel
└─────────────────────────────────┘
```

### Data already available

- **Gear details**: `state.gearDetails` contains `metadata` (brand, material, weight_grams, carbon_kg), `value_estimate` (value_usd), `source_url`, `location_id`, `times_loaned`
- **Location**: `state.locationName`, `state.locationDistanceMeters` — already loaded
- **Transfer context**: `state.transferContext` — has `GearTransferContext` with owner/active loan info

### Data NOT yet loaded

- **Stats**: `GetGearStats` RPC returns `times_loaned`, `people_helped`, `value_shared_usd`, `interest_count`, `impact`, `potential_impact`. Repository methods `getStats()` and `invalidateStats()` already exist on `GearRepository` — just not called from the view model
- **People**: `GetGearPeople` RPC returns `owner`, `current_borrower`, `past_borrowers`, `interested_parties`, `total_count`. Repository methods `getPeople()` and `invalidatePeople()` already exist — just not called from the view model

### Shared widgets already available

All four shared content widgets exist in `app/lib/presentation/widgets/content/`:

| Widget | Status | Notes |
|--------|--------|-------|
| `ContentInfoCard` | Ready | Glass card with icon rows (location, time, website). Gear needs location + optional website rows |
| `ContentPeopleRow` | Ready | Avatar stack + summary + chevron. Gear needs "X borrowing · X returned" summary |
| `ContentImpactTiles` | Ready | Three tappable metric tiles. Takes `ImpactEstimate` — same type returned by `GetGearStats` |
| `ContentMetricSheet` | Ready | Formula breakdown bottom sheet. Already generic |

---

## Reference Design Analysis

### Gear Details (gear-details.png)

From top to bottom within the Details pane:

1. **Location Card** — Glass card with two rows separated by a divider:
   - Location: pin icon box + "2957 11th Street" (bold serif) + "10 ft" distance subtitle
   - Website (conditional): link icon box + "arcteryx.com" (tappable)

2. **Specs Card** — Separate glass card with four rows separated by dividers:
   - Brand: label left, "Arc'teryx" right (bold)
   - Value: label left, "$225" right (bold)
   - Material: label left, "Fabric" right (bold)
   - Weight: label left, "290 g" right (bold)

3. **Borrower Summary Row** — Single tappable glass card row:
   - Avatar stack (overlapping circles, max 4-5) + "1 borrowing · 3 returned" summary + chevron
   - Tapping opens the borrower list modal

4. **Impact Tiles** — "POTENTIAL IMPACT" label + horizontal row of three equal-width tappable tiles:
   - Money Saved: `$` icon, `$900`, coral accent
   - CO₂ Avoided: `🌿` icon, `48 kg`, green accent
   - Quality Time: `⏱` icon, `78 min`, sage accent

5. **Media Carousel** — Horizontal strip of photo thumbnails + "Add" button

### Borrower Modal (borrower-modal.png)

- Bottom sheet with drag handle, dark background
- Header: "4 Borrowers" title (serif) + "1 active · 3 returned" subtitle
- Two sections: CURRENTLY BORROWING (full opacity) and PAST BORROWERS (0.7 opacity)
- Each section: uppercase label + count on right
- Each row: avatar circle + name, separated by thin dividers
- No action buttons for loans (borrowing is managed through the Loan tab / chat workflow)

### Giveaway Interest Modal (not shown — derived from borrower modal pattern)

- Same layout as borrower modal, adapted for giveaway workflow
- Header: "X Interested" title + "X selected · X interested" subtitle
- **Action button**: "Express Interest" (sage glass, full width) for non-owners who haven't expressed interest; "Interested" (sage, check icon) for those who have
- Sections: SELECTED (if any) and INTERESTED
- Owner sees the list but manages selection through the Giveaway tab / chat workflow

### Metric Formula Modal

- Identical to the experience/request metric formula sheet (same `ContentMetricSheet` component)

---

## Proposed Solution

### Approach

Redesign the Details tab to match the reference layout by:

1. Replacing the side-by-side `_DetailCard` pair with `ContentInfoCard` for the location/website rows
2. Keeping the specs (brand, value, material, weight) as a separate glass card with the existing `ContentDetailRows` pattern — the reference design shows them as a distinct card, not merged into the info card
3. Adding `ContentPeopleRow` for borrowers/interested parties with a modal
4. Adding `ContentImpactTiles` + `ContentMetricSheet` for impact metrics
5. Loading stats and people data from the view model on init

### Architecture

**No new RPCs needed.** `GetGearStats` and `GetGearPeople` already exist with all required fields. The repository already has cached `getStats()`, `getPeople()`, `invalidateStats()`, and `invalidatePeople()` methods.

**No proto changes needed.** All required fields exist.

### New widgets

1. `app/lib/presentation/widgets/gear/gear_borrower_sheet.dart` — Borrower list bottom sheet for loan gear
2. `app/lib/presentation/widgets/gear/gear_interest_sheet.dart` — Interest list bottom sheet for giveaway gear

### State changes (`gear_view_model.dart`)

- Add `GetGearStatsResponse? gearStats` to `GearState`
- Add `GetGearPeopleResponse? gearPeople` to `GearState`
- Add `loadStats()` and `loadPeople()` methods to `GearNotifier`
- Call both in `loadGearDetails()` (fire-and-forget, non-blocking)
- After loan/giveaway state changes: invalidate and reload stats + people

---

## Decisions

1. **Location + Website as ContentInfoCard, specs as separate card**: The reference design clearly separates these into two glass cards. The location card uses icon rows (pin + link icons). The specs card uses plain label/value rows. This matches the existing `ContentInfoCard` for the first card, and a styled `ContentDetailRows` (or equivalent) for the second.

2. **Borrower modal for loans, Interest modal for giveaways**: The people row and its modal adapt based on the gear's sharing type. For loans: shows borrowers (current + past), no action button. For giveaways: shows interested parties, with an "Express Interest" button for non-owners. Both use the same `ContentPeopleRow` with different summary text and different modals.

3. **Impact loading**: Load `gearStats` eagerly alongside gear details (same pattern as experience/request). Show "POTENTIAL IMPACT" when `times_loaned == 0`, "IMPACT" when `times_loaned > 0`. Display `–` for metrics with no value.

4. **People loading**: Load `gearPeople` eagerly alongside gear details. The people row shows a summary; the modal shows the full grouped list. Both refresh after loan/giveaway state changes.

5. **No "Request to Borrow" in borrower modal**: The reference design's borrower modal is read-only for loans. Borrowing is initiated through the Loan tab's action button or the Chat tab. The giveaway interest modal does have an action button ("Express Interest") since that's the primary non-owner action for giveaways.

6. **Specs card retains ContentDetailRows**: The existing `ContentDetailRows` widget already renders key/value pairs with dividers. The gear specs card matches this pattern — just needs glass card wrapping. No new widget needed for the specs section.

---

## Resolved Questions

1. **"Listed X ago" subtitle**: Not included. The location subtitle shows only the distance (e.g., "10 ft"), not "Listed X ago".

2. **Website row placement**: Show only location + optional website in the info card. Drop the "Flexible" / availability card entirely — gear doesn't have a scheduled time.

3. **Giveaway interest modal action**: Use the existing `expressInterest()` method on the gear view model — it already sends the interest RPC and updates the transfer context.

---

## Cross-Cutting Concerns

### i18n

All user-visible strings must use `context.l10n.*`. New ARB keys needed (prefix `gear*`):

- `gearBorrowersCount` — "{count} Borrowers" (ICU plural)
- `gearInterestedCount` — "{count} Interested" (ICU plural)
- `gearBorrowingCount` — "{count} borrowing" (ICU plural)
- `gearReturnedCount` — "{count} returned" (ICU plural)
- `gearActiveCount` — "{count} active" (ICU plural)
- `gearSelectedCount` — "{count} selected" (ICU plural)
- `gearCurrentlyBorrowing` — "CURRENTLY BORROWING"
- `gearPastBorrowers` — "PAST BORROWERS"
- `gearSelected` — "SELECTED"
- `gearInterested` — "INTERESTED"
- `gearExpressInterest` — "Express Interest"
- `gearAlreadyInterested` — "Interested"

Shared impact metric keys (`impactPotentialImpact`, `impactImpact`, `impactMoneySaved`, `impactCo2Avoided`, `impactQualityTime`) already exist from the experience/request redesign work.

### Async Safety

`GearNotifier` uses autoDispose. All new async methods (`loadStats()`, `loadPeople()`) must check `if (!ref.mounted) return;` after each `await` before setting state.

---

## Phased Implementation Plan

### Phase 1: State Layer — Add Stats and People to GearState

**Goal**: Load impact stats and people data alongside gear details so the Details tab has data to display.

**Files modified:**
- `app/lib/presentation/viewmodels/gear_view_model.dart`

**Changes:**

**New state fields (Freezed):**
- `GetGearStatsResponse? gearStats` — impact and usage data for the details tab
- `GetGearPeopleResponse? gearPeople` — owner, borrowers, interested parties

**New methods:**
- `loadStats()`:
  - Calls `ref.read(gearRepositoryProvider).getStats(gearId, communityId: communityId)`
  - Stores result in `state.gearStats`
  - Guard with `if (!ref.mounted) return;` after the await
- `loadPeople()`:
  - Calls `ref.read(gearRepositoryProvider).getPeople(gearId, communityId: communityId)`
  - Stores result in `state.gearPeople`
  - Guard with `if (!ref.mounted) return;` after the await

**Wiring:**
- Call `loadStats()` and `loadPeople()` at end of `loadGearDetails()` (fire-and-forget, non-blocking)
- After successful loan operations (return, pickup, etc.): call `gearRepository.invalidateStats()` + `gearRepository.invalidatePeople()` then `loadStats()` + `loadPeople()`
- After `expressInterest()`: same invalidate-then-reload pattern for both stats and people (interest count affects both `GetGearStatsResponse.interest_count` and `GetGearPeopleResponse.interested_parties`)

**Tests:**
- Unit test: `loadStats()` populates `gearStats`
- Unit test: `loadPeople()` populates `gearPeople`
- Unit test: stats/people remain null on fetch error (no crash)
- Disposal safety test: `loadStats()` and `loadPeople()` handle disposal during async

---

### Phase 2: Details Tab Redesign — Info Card + Specs Card

**Goal**: Replace the side-by-side `_DetailCard` pair and flat `ContentDetailRows` with the new two-card layout matching the reference design.

**Files modified:**
- `app/lib/presentation/screens/gear/gear_content_view.dart`

**Changes to `_buildDetailsPane()`:**

Replace the current `IntrinsicHeight` row (`_DetailCard` × 2) + flat `ContentDetailRows` with:

1. **`ContentInfoCard`** — Location row (pin icon + location name + distance subtitle) + optional Website row (link icon + domain name, tappable). No time row — gear doesn't have a scheduled time.

2. **Specs card** — A glass card wrapping `ContentDetailRows` with the four metadata fields: Brand, Value, Material, Weight. The existing `ContentDetailRows` already renders key/value pairs with dividers. Wrap it in a glass container matching the reference (same glass bg/border as `ContentInfoCard`). In edit mode, these become editable text fields (preserving existing edit buffer behavior).

**Deleted code:**
- `_DetailCard` widget class (replaced by `ContentInfoCard`)
- The `_buildLocationTimeRow()` method or equivalent (absorbed into `ContentInfoCard` usage)

**Tests:**
- Widget test: Info card renders location row with distance subtitle
- Widget test: Info card hides website row when `source_url` is null
- Widget test: Specs card renders all four metadata rows
- Widget test: Edit mode shows editable fields for specs

---

### Phase 3: People Row + Borrower Modal (Loans)

**Goal**: Add borrower information to the Details tab for loan gear, with a modal showing the full borrower list.

**Files created:**
- `app/lib/presentation/widgets/gear/gear_borrower_sheet.dart`

**Widget: `GearBorrowerSheet`**
- Static `show(context, gearId)` method using `showModalBottomSheet` with `DraggableScrollableSheet`
- `ConsumerWidget` that watches `gearProvider(gearId)` for live state (rebuilds when people change)
- Layout matches reference (borrower-modal.png):
  - Drag handle
  - Header: "X Borrowers" title (serif) + "X active · X returned" subtitle
  - **CURRENTLY BORROWING section**: Uppercase label + count + avatar/name rows (full opacity)
  - **PAST BORROWERS section**: Uppercase label + count + avatar/name rows (0.7 opacity)
  - No action buttons — borrowing is managed through the Loan tab
- Theme-aware: `AppColors.cardBackground(context)` etc.
- i18n: All strings via `context.l10n.*`

**Files modified:**
- `app/lib/presentation/screens/gear/gear_content_view.dart` — Add `ContentPeopleRow` between specs card and impact tiles. Summary text: "{currentBorrowers} borrowing · {pastBorrowers} returned". `onTap` opens `GearBorrowerSheet.show()`. Only shown when `gearPeople` is loaded and total > 0.

**ARB keys added in this phase (both `app_en.arb` and `app_es.arb`):**
- `gearBorrowersCount`, `gearBorrowingCount`, `gearReturnedCount`, `gearActiveCount`
- `gearCurrentlyBorrowing`, `gearPastBorrowers`

**Tests:**
- Widget test: People row shows correct borrower counts
- Widget test: People row shows avatar stack
- Widget test: Tapping opens `GearBorrowerSheet`
- Widget test: Borrower sheet renders grouped sections with correct counts
- Widget test: Past borrowers at 0.7 opacity
- Widget test: Empty state (no borrowers) — people row hidden

---

### Phase 4: Interest Modal (Giveaways)

**Goal**: Create the giveaway-specific interest modal for gear being given away.

**Files created:**
- `app/lib/presentation/widgets/gear/gear_interest_sheet.dart`

**Widget: `GearInterestSheet`**
- Static `show(context, gearId)` method
- `ConsumerWidget` watching `gearProvider(gearId)` for live state
- Layout (derived from borrower modal + helper modal patterns):
  - Drag handle
  - Header: "X Interested" title (serif) + "X selected · X interested" subtitle
  - **Action button** (non-owner, non-terminal state):
    - Not interested: "Express Interest" button (sage glass, full width)
    - Already interested: "Interested" button (sage glass, check icon)
  - **SELECTED section** (if any): Uppercase label + count + avatar/name rows
  - **INTERESTED section**: Uppercase label + count + avatar/name rows
- Action calls `ref.read(gearProvider(id).notifier).expressInterest()` — existing method handles the RPC
- After interest change: notifier invalidates + reloads people

**Files modified:**
- `app/lib/presentation/screens/gear/gear_content_view.dart` — When gear is giveaway type, `ContentPeopleRow` shows "{selected} selected · {interested} interested" and opens `GearInterestSheet` instead of `GearBorrowerSheet`

**ARB keys added in this phase (both `app_en.arb` and `app_es.arb`):**
- `gearInterestedCount`, `gearSelectedCount`
- `gearSelected`, `gearInterested`
- `gearExpressInterest`, `gearAlreadyInterested`

**Tests:**
- Widget test: renders "Express Interest" for non-interested non-owner
- Widget test: renders "Interested" for already-interested user
- Widget test: hides action button for owner
- Widget test: tapping "Express Interest" calls notifier
- Widget test: grouped sections show correct counts

---

### Phase 5: Impact Tiles

**Goal**: Add impact metrics to the Details tab using the shared `ContentImpactTiles` and `ContentMetricSheet` widgets.

**Files modified:**
- `app/lib/presentation/screens/gear/gear_content_view.dart`

**Changes:**
- Add `ContentImpactTiles` between the people row and media carousel
- Pass `gearStats.impact` (when `times_loaned > 0`) or `gearStats.potentialImpact` (when `times_loaned == 0`)
- `isCompleted` = `times_loaned > 0` (gear impact is cumulative, not per-loan)
- Label: "IMPACT" when loaned at least once, "POTENTIAL IMPACT" when never loaned
- Tapping opens `ContentMetricSheet.show()` with the metric breakdown
- Hidden while `gearStats` is null (still loading)

**Tests:**
- Widget test: Impact tiles appear when stats loaded
- Widget test: Hidden when stats null
- Widget test: Shows "POTENTIAL IMPACT" when `times_loaned == 0`
- Widget test: Shows "IMPACT" when `times_loaned > 0`
- Widget test: Tapping tile opens `ContentMetricSheet`
- Widget test: Displays `–` for zero-value metrics

---

### Phase 6: Dynamic Recalculation & Integration

**Goal**: Ensure the full reactive chain works — loan returns, giveaway interest changes, etc. update the borrower/interest list, the people summary row, and the impact tiles.

**Reactive chain (loan example):**
1. Borrower returns item → loan state changes
2. `GearNotifier` calls `invalidateStats()` + `invalidatePeople()` then `loadStats()` + `loadPeople()`
3. `GearState` updates → Riverpod rebuilds:
   - `ContentPeopleRow` sees updated borrower counts
   - `ContentImpactTiles` sees updated impact values
   - `GearBorrowerSheet` (if open) sees updated borrower list

**Reactive chain (giveaway example):**
1. User taps "Express Interest" in `GearInterestSheet` → calls `expressInterest()`
2. `expressInterest()` sends RPC, then invalidates + reloads people
3. `GearState` updates → Riverpod rebuilds:
   - `GearInterestSheet` sees updated interested parties list
   - `ContentPeopleRow` sees updated interest counts

**Changes:**
- Audit all loan/giveaway state-changing methods in `GearNotifier` to ensure they call `invalidateStats()` + `invalidatePeople()` followed by `loadStats()` + `loadPeople()`
- Ensure `switchCommunity()` reloads stats and people for the new community context

**Tests:**
- Integration test: loan state change triggers stats + people reload
- Integration test: express interest triggers people reload
- Integration test: community switch reloads stats + people
- Widget test: full Details tab renders all sections in correct order (info card → specs card → people row → impact tiles → media carousel)

---

### Phase 7: Verification + Documentation Update

**Goal**: Verify all i18n keys (added in Phases 3-4) are correct and update documentation.

**Note**: ARB keys are added in the phase where their widgets are created (Phase 3 for borrower keys, Phase 4 for giveaway keys). This phase verifies completeness and updates docs.

**Files modified:**
- `docs/client/content.md` — Update "Content Type Differences" table to reflect gear's new Details tab layout (now uses `ContentInfoCard`, `ContentPeopleRow`, `ContentImpactTiles`)

**Verification:**
- Run `npm run generate` to regenerate `AppLocalizations`
- Run `npm run lint:dart:i18n` to verify no broken i18n references
- Run `flutter test --reporter expanded` to verify all tests pass

---

## File Summary

### New files (2)
| File | Purpose |
|------|---------|
| `app/lib/presentation/widgets/gear/gear_borrower_sheet.dart` | Borrower list modal for loan gear |
| `app/lib/presentation/widgets/gear/gear_interest_sheet.dart` | Interest list modal for giveaway gear |

### Modified files
| File | Change |
|------|--------|
| `gear_view_model.dart` | Add `gearStats`, `gearPeople` state fields + `loadStats()`, `loadPeople()` methods |
| `gear_content_view.dart` | Rebuild `_buildDetailsPane()` with `ContentInfoCard`, specs card, `ContentPeopleRow`, `ContentImpactTiles`; remove `_DetailCard` |
| `app_en.arb` / `app_es.arb` | Add `gear*` ARB keys |
| `docs/client/content.md` | Update gear Details tab documentation |

### Deleted code
- `_DetailCard` widget class in `gear_content_view.dart` (replaced by `ContentInfoCard`)
- `_buildLocationTimeRow()` or equivalent method (absorbed into `ContentInfoCard` usage)
- Flat `ContentDetailRows` usage for all metadata in one block (restructured into separate info card + specs card)

---

## Design Tokens

Same as experience-data.md and request-data.md — the gear Details tab renders over a media background using dark glass styling.

**Content view (over media background — always dark glass):**

| Token | Value |
|-------|-------|
| Glass background | `Colors.white.withValues(alpha: 0.06)` |
| Glass border | `Colors.white.withValues(alpha: 0.10)` |
| Row divider | `Colors.white.withValues(alpha: 0.08)` |
| Muted text | `Colors.white.withValues(alpha: 0.40)` |
| Secondary text | `Colors.white.withValues(alpha: 0.65)` |
| Icon box coral tint | `AppColors.transferCoral.withValues(alpha: 0.12)` |

**Bottom sheets (theme-aware):**

| Element | AppColors getter |
|---------|-----------------|
| Sheet background | `AppColors.cardBackground(context)` |
| Primary text | `AppColors.textPrimary(context)` |
| Secondary text | `AppColors.textSecondary(context)` |
| Muted text | `AppColors.textTertiary(context)` |
| Section background | `AppColors.surface(context)` |
| Borders | `AppColors.border(context)` |

---

## Audit Log

**Audited 2026-03-30** against `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md`. Changes made:

1. **[i18n] ARB keys deferred to Phase 7.** Original plan created widgets with user-visible strings in Phases 3-4 but added all ARB keys in Phase 7. This violates the i18n rule "Strings belong in ARB files, not Dart code" — `npm run lint:dart:i18n` would fail for every intermediate phase. Moved ARB key additions into the phases where their consuming widgets are created (Phase 3 for borrower keys, Phase 4 for giveaway keys). Phase 7 reduced to verification + docs only.

2. **[caching] `expressInterest()` stats invalidation missing.** Phase 1 only invalidated people cache after `expressInterest()`, not stats. `GetGearStatsResponse` has an `interest_count` field that would show stale data without stats invalidation. Added `invalidateStats()` + `loadStats()` to the `expressInterest()` invalidation chain alongside people.

---

**Last Updated:** 2026-03-30
