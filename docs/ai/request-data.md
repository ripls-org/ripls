# Request Details Tab Redesign

## Problem Statement

The Details tab on `RequestContentView` currently shows a basic layout that does not match the experience Details tab or the reference designs:

1. **Side-by-side cards**: Location and Posted/Status are rendered as two `_DetailCard` widgets in an `IntrinsicHeight` row — the same pattern the experience view had before its redesign. The reference design shows a unified glass card with icon rows separated by dividers.

2. **Helpers section is text-only**: Helpers are shown as a plain "2 Helpers: Name, Name" text row. The reference design shows an avatar stack with "X helping · X in conversation" summary that opens a full helper list modal (similar to the experience attendee sheet).

3. **No impact metrics**: The Details tab shows no impact data. The reference design includes "POTENTIAL IMPACT" tiles (Money Saved, CO₂ Avoided, Quality Time) identical to the experience Details tab, with tappable formula bottom sheets.

4. **No shared components**: The experience Details tab was redesigned with `ExperienceInfoCard`, `ExperienceAttendeeRow`, `ExperienceImpactTiles`, and `ExperienceMetricSheet`. These widgets are functionally identical to what the request view needs, but they are experience-specific (hard-coded imports of experience protos, experience view model, experience-specific RSVP types). Both views need the same glass card rows, avatar stacks, impact tiles, and metric sheets.

---

## Current State

### Details tab layout (`request_content_view.dart:562-639`)

```
┌─────────────────────────────────┐
│ LOCATION card │ POSTED card     │  ← IntrinsicHeight row (_DetailCard × 2)
├─────────────────────────────────┤
│ 2 Helpers: Thomas, Betty        │  ← _buildHelpersSection (text-only)
├─────────────────────────────────┤
│ [thumb] [thumb] [+]             │  ← ContentMediaCarousel
└─────────────────────────────────┘
```

### Data already available

- **Helpers/Offerers**: `request.offerers` (List<User>) is already loaded in `RequestState.requestDetails`
- **Impact stats**: `GetRequestStats` RPC exists and returns `potential_impact` + `impact` (identical to `GetExperienceStats`). Repository methods `getStats()` and `invalidateStats()` already exist
- **Completion savings**: `RequestState.completionSavings` stores impact after fulfillment, but stats are not loaded on the details tab for active requests
- **Location**: `state.locationName`, `state.locationDistanceMeters` — already loaded
- **Posted/Status**: Computed from `request.createdAtUnixSec` and `request.state`

### Experience widgets that could be shared

| Experience Widget | What It Does | Request Equivalent Needed |
|---|---|---|
| `ExperienceInfoCard` | Glass card with location + time + website rows | Glass card with location + posted/status rows |
| `ExperienceAttendeeRow` | Avatar stack + "X going · X maybe" + chevron | Avatar stack + "X helping · X in conversation" + chevron |
| `ExperienceImpactTiles` | Three metric tiles (money, CO₂, QT) | Identical three metric tiles |
| `ExperienceMetricSheet` | Formula breakdown bottom sheet | Identical formula breakdown |
| `AttendeeListSheet` | RSVP buttons + grouped attendee list | Help action + grouped helper list |

---

## Reference Design Analysis

### Request Details (request-details.png)

From top to bottom within the Details pane:

1. **Info Card** — Glass card with two rows separated by a divider:
   - Location: pin icon box + "Denver Recovery Group" (bold serif)
   - Posted: clock icon box + "2 days ago" (bold serif) + "Open" subtitle

2. **Helper Summary Row** — Single tappable row (glass card):
   - Avatar stack (overlapping circles, max 4) + "2 helping · 3 in conversation" + chevron
   - Tapping opens the helper list modal

3. **Impact Tiles** — "POTENTIAL IMPACT" label + horizontal row of three equal-width tappable tiles:
   - Money Saved: `$` icon, `$120`, coral accent
   - CO₂ Avoided: `🌿` icon, `22 kg`, green accent
   - Quality Time: `⏱` icon, `1.5 hrs`, sage accent

4. **Media Carousel** — Horizontal strip of photo thumbnails + "Add" button

### Helper Modal (help-modal.png)

- Bottom sheet with drag handle
- Header: "5 People" title + "2 helping · 3 in conversation" subtitle
- **Help action**: "Offer to Help" button (sage glass) or "Helping" (sage glass, checked) + "Withdraw" (muted border)
- Two sections: HELPING (full opacity) and IN CONVERSATION (0.75 opacity)
- Each row: avatar circle + name

### Metric Formula Modal

- Identical to the experience metric formula sheet (same layout, same calculation breakdown)

---

## Proposed Solution

### Approach: Refactor Experience Widgets into Shared Components

Rather than duplicating the experience widgets for requests, extract the shared UI into content-level components that both views can use. The experience-specific types (RSVP, RSVPIntention) become generic parameters.

**Shared widgets** (move to `app/lib/presentation/widgets/content/`):

1. **`ContentInfoCard`** — The glass card with icon rows. Currently `ExperienceInfoCard` — already generic (no experience-specific types). Rename and move.

2. **`ContentPeopleRow`** — The avatar stack + summary + chevron row. Extracted from `ExperienceAttendeeRow`, parameterized with `List<User>` + summary text instead of RSVP-specific types.

3. **`ContentImpactTiles`** — The three metric tiles. Currently `ExperienceImpactTiles` — already generic (takes `ImpactEstimate`). Rename and move.

4. **`ContentMetricSheet`** — The formula bottom sheet. Currently `ExperienceMetricSheet` — already generic (takes `ImpactEstimate`). Rename and move.

**Content-specific widgets** (remain in their respective directories):

5. **`AttendeeListSheet`** — Experience-specific (RSVP buttons + grouped attendee list). Stays in `widgets/experience/`.

6. **`HelperListSheet`** — New request-specific modal. "Offer to Help" / "Helping + Withdraw" buttons + grouped helper list (Helping / In Conversation sections). In `widgets/request/`.

### Architecture

**No new RPCs needed.** `GetRequestStats` already returns `potential_impact` and `impact` with the right scaling. The request repository already has `getStats()` and `invalidateStats()`.

**No proto changes needed.** All required fields exist.

### State changes (`request_view_model.dart`)

- Add `GetRequestStatsResponse? requestStats` to `RequestState`
- Add `loadStats()` method to `RequestNotifier` — calls `requestRepository.getStats()`, stores result
- Call `loadStats()` in `loadRequestDetails()` (fire-and-forget)
- After `offerToFulfill()` / `withdrawOffer()` / `saveSummaryAndMarkFulfilled()`: call `invalidateStats()` then `loadStats()` to recalculate

---

## Decisions

1. **Shared vs. duplicated widgets**: Extract `ExperienceInfoCard`, `ExperienceImpactTiles`, and `ExperienceMetricSheet` into shared `Content*` widgets. These are already type-agnostic. The attendee/helper modals remain content-specific because their action buttons and grouping logic differ significantly (RSVP vs. offer-to-help).

2. **People row abstraction**: The avatar stack + summary text + chevron pattern is identical between attendee row and helper row. Extract a `ContentPeopleRow` that takes `List<User>`, a summary string, and an `onTap` callback. Both `ExperienceContentView` and `RequestContentView` construct the summary text from their respective state (RSVP counts vs. helper/conversation counts).

3. **"Posted" replaces "Time" for requests**: The info card's second row shows "Posted: 2 days ago / Open" instead of "Time: Sat, Mar 30 / 2:00 PM". The shared `ContentInfoCard` already takes generic `timeDisplay` + `timeSubtitle` parameters, so this is just different data passed in.

4. **No website row for requests**: Requests don't have a `sourceUrl` field. The shared info card already hides the website row when `sourceUrl` is null.

5. **Helper modal vs. Attendee modal**: These remain separate widgets. The attendee modal has RSVP buttons (Yes/Maybe/No toggle) and groups by RSVP intention. The helper modal has Offer/Withdraw buttons and groups by helper status (Helping vs. In Conversation). The visual design is similar but the interaction model is different enough to warrant separate implementations.

6. **Impact loading on details tab**: Load `requestStats` eagerly alongside request details (same as experience). Show "POTENTIAL IMPACT" for active requests, "IMPACT" for fulfilled. Show nothing while loading.

---

## Open Questions

1. **"Helping" vs "In Conversation" definition**: "Helping" = active `request.offerers` (users who have offered to fulfill). "In Conversation" = users from `request.conversationParticipants` who are NOT in the offerers list (they've chatted but haven't formally offered). Both lists are already on the `GetRequestResponse` — no additional RPC needed.

2. **Stats refresh after offer/withdraw**: Yes, invalidate and reload stats after `offerToFulfill()` and `withdrawOffer()` for consistency. Even if the server doesn't scale by offerer count today, the pattern prevents stale data.

3. **Helper modal for non-owners**: Show the modal to everyone. Everyone can offer to help. The modal gives non-owners a way to see who else is helping and offer/withdraw without switching tabs.

---

## Cross-Cutting Concerns

### i18n

All user-visible strings must use `context.l10n.*`. New ARB keys needed (prefix `request*`):

- `requestHelpingCount` — "{count} helping" (ICU plural)
- `requestInConversationCount` — "{count} in conversation" (ICU plural)
- `requestPeopleCount` — "{count} People" (ICU plural)
- `requestOfferToHelp` — "Offer to Help"
- `requestHelping` — "Helping"
- `requestWithdraw` — "Withdraw"
- `requestSectionHelping` — "HELPING"
- `requestSectionInConversation` — "IN CONVERSATION"
- `requestPosted` — "Posted"
- `requestStatusOpen` — "Open"
- `requestStatusFulfilled` — "Fulfilled"
- `requestStatusCancelled` — "Cancelled"

Shared metric keys already exist from the experience work (`experienceMoneySaved`, `experienceCo2Avoided`, `experienceQualityTime`, `experiencePotentialImpact`, `experienceImpact`). Rename these to use a `content*` or `impact*` prefix during the refactor so they're not experience-specific:

- `impactPotentialImpact` — "POTENTIAL IMPACT"
- `impactImpact` — "IMPACT"
- `impactMoneySaved` — "MONEY SAVED"
- `impactCo2Avoided` — "CO₂ AVOIDED"
- `impactQualityTime` — "QUALITY TIME"

### Async Safety

`RequestNotifier` uses autoDispose. All new async methods (`loadStats()`) must check `if (!ref.mounted) return;` after each `await` before setting state.

---

## Phased Implementation Plan

### Phase 1: Refactor Experience Widgets into Shared Content Widgets

**Goal**: Move type-agnostic widgets from `widgets/experience/` to `widgets/content/` so both experience and request views can reuse them.

**Files renamed/moved:**
- `widgets/experience/experience_info_card.dart` → `widgets/content/content_info_card.dart` (class: `ContentInfoCard`)
- `widgets/experience/experience_impact_tiles.dart` → `widgets/content/content_impact_tiles.dart` (class: `ContentImpactTiles`)
- `widgets/experience/experience_metric_sheet.dart` → `widgets/content/content_metric_sheet.dart` (class: `ContentMetricSheet`)

**Files created:**
- `widgets/content/content_people_row.dart` — Extract the avatar stack + summary + chevron pattern from `ExperienceAttendeeRow`. Parameters: `List<User> users`, `String summaryText`, `VoidCallback onTap`. The `_AvatarStack` inner widget moves here too.

**Files modified:**
- `widgets/experience/experience_attendee_row.dart` — Simplify to a thin wrapper around `ContentPeopleRow`. It extracts the yes/maybe/no users from RSVP list, builds the summary text using `context.l10n.experienceGoingCount` etc., and passes to `ContentPeopleRow`.
- `screens/experience/experience_content_view.dart` — Update imports from `ExperienceInfoCard` → `ContentInfoCard`, `ExperienceImpactTiles` → `ContentImpactTiles`, `ExperienceMetricSheet` → `ContentMetricSheet`
- `screens/item/item_metrics_screen.dart` — Update ARB key references from `experience*` → `impact*`
- `viewmodels/experience_metric_view_model.dart` — Update ARB key references from `experience*` → `impact*`
- `app_en.arb` / `app_es.arb` — Rename `experience*` metric keys to `impact*` prefix. Add `request*` keys for helper modal strings. Must update both `@key` metadata blocks and the key names themselves.

**ARB key renames:**
- `experiencePotentialImpact` → `impactPotentialImpact`
- `experienceImpact` → `impactImpact`
- `experienceMoneySaved` → `impactMoneySaved`
- `experienceCo2Avoided` → `impactCo2Avoided`
- `experienceQualityTime` → `impactQualityTime`
- `experienceCalculation` → `impactCalculation`

**All files using renamed keys** (must update `context.l10n.*` references):
- `widgets/experience/experience_impact_tiles.dart` (→ now `widgets/content/content_impact_tiles.dart`)
- `widgets/experience/experience_metric_sheet.dart` (→ now `widgets/content/content_metric_sheet.dart`)
- `screens/item/item_metrics_screen.dart`
- `viewmodels/experience_metric_view_model.dart`

Run `npm run generate` after ARB changes to regenerate `AppLocalizations`. Run `npm run lint:dart:i18n` to verify no broken references.

**Tests:**
- Move/rename existing experience widget tests to match new file locations
- Verify experience details tab still renders correctly after the refactor
- Widget test: `ContentPeopleRow` renders avatar stack, summary, chevron
- Widget test: `ContentPeopleRow` onTap fires

---

### Phase 2: State Layer — Add Stats to RequestState

**Goal**: Load impact stats alongside request details so the Details tab has data to display.

**Files modified:**
- `app/lib/presentation/viewmodels/request_view_model.dart`

**Changes:**

**New state field (Freezed):**
- `GetRequestStatsResponse? requestStats` — impact data for the details tab

**Note on `completionSavings` vs `requestStats`:** The existing `completionSavings: ImpactEstimate?` field stores the impact returned inline from the `MarkRequestFulfilled` RPC response (immediate, during the fulfillment modal flow). The new `requestStats` field stores the full `GetRequestStatsResponse` fetched separately (for the Details tab). Both may hold impact data for fulfilled requests — `completionSavings` is authoritative during the fulfillment flow, `requestStats` is authoritative for the Details tab. No need to merge them.

**No separate people RPC needed.** Helper and conversation participant data is already on the `GetRequestResponse`: `request.offerers` (helpers) and `request.conversationParticipants` (all chat participants excluding requester). "In Conversation" = `conversationParticipants` minus `offerers`. This data refreshes automatically when `loadRequestDetails()` reloads the request.

**New method:**
- `loadStats()`:
  - Calls `ref.read(requestRepositoryProvider).getStats(requestId, communityId: communityId)`
  - Stores result in `state.requestStats`
  - Guard with `if (!ref.mounted) return;` after the await

**Wiring:**
- Call `loadStats()` at end of `loadRequestDetails()` (fire-and-forget, non-blocking)
- After `offerToFulfill()`: call `requestRepository.invalidateStats()` then `loadStats()`. The `loadRequestDetails()` call that already follows `offerToFulfill()` will refresh the offerers and conversation participants lists.
- After `withdrawOffer()`: same invalidate-then-reload pattern
- After `saveSummaryAndMarkFulfilled()`: same pattern (already stores `completionSavings`, but now also refreshes `requestStats` for the tiles)

**Tests:**
- Unit test: `loadStats()` populates `requestStats`
- Unit test: `offerToFulfill()` calls `invalidateStats()` then `loadStats()`
- Unit test: stats remain null on fetch error (no crash)
- Disposal safety test: `loadStats()` handles disposal during async

---

### Phase 3: Request Details Tab Redesign

**Goal**: Replace the current Details tab layout with the shared widgets, matching the reference design.

**Files modified:**
- `app/lib/presentation/screens/request/request_content_view.dart`

**Changes to `_buildDetailsPane()`:**
Replace the current `IntrinsicHeight` row + `_buildHelpersSection()` with:

1. **`ContentInfoCard`** — Location row (pin icon + location name + distance subtitle) + Posted row (clock icon + "2 days ago" + "Open"/"Fulfilled"/"Cancelled" subtitle). No website row.

2. **`ContentPeopleRow`** — Computes helpers from `request.offerers` and in-conversation from `request.conversationParticipants` minus offerers (by user ID). Avatar stack shows all people (helpers first). Summary text: "X helping · X in conversation" using `context.l10n.requestHelpingCount` / `requestInConversationCount`. `onTap` opens `HelperListSheet` (Phase 4).

3. **`ContentImpactTiles`** — Uses `requestStats.potentialImpact` for active requests, `requestStats.impact` for fulfilled. Tapping opens `ContentMetricSheet.show()`.

4. **`ContentMediaCarousel`** — Unchanged.

**Deleted code:**
- `_DetailCard` widget class (replaced by `ContentInfoCard`)
- `_buildHelpersSection()` method (replaced by `ContentPeopleRow`)

**Tests:**
- Widget test: Details tab renders info card, people row, impact tiles, media carousel in correct order
- Widget test: Info card shows location + posted time + status
- Widget test: Impact tiles appear when stats loaded, hidden when null
- Widget test: People row shows correct helper counts

---

### Phase 4: Helper List Bottom Sheet

**Goal**: Create the request-specific helper list modal matching the reference design (help-modal.png).

**Files created:**
- `app/lib/presentation/widgets/request/helper_list_sheet.dart`

**Widget: `HelperListSheet`**
- Static `show(context, requestId)` method using `showModalBottomSheet` with `DraggableScrollableSheet`
- `ConsumerStatefulWidget` that watches `requestProvider(requestId)` for live state
- Layout matches reference (help-modal.png):
  - Drag handle
  - Header: "X People" title + "X helping · X in conversation" subtitle
  - **Help action** (non-terminal state only):
    - Not helping: "Offer to Help" button (sage glass, full width)
    - Already helping: "Helping" button (sage glass, check icon) + "Withdraw" button (muted border) side by side
  - **HELPING section**: Uppercase label + count + avatar/name rows (full opacity)
  - **IN CONVERSATION section**: Uppercase label + count + avatar/name rows (0.75 opacity)
- Help action calls `requestProvider.notifier.offerToFulfill()` / `.withdrawOffer()`
- Watches provider so list updates live when offers change

**Tests:**
- Widget test: renders "Offer to Help" for non-helper
- Widget test: renders "Helping" + "Withdraw" for current helper
- Widget test: grouped sections show correct counts
- Widget test: tapping "Offer to Help" calls notifier
- Widget test: tapping "Withdraw" calls notifier
- Widget test: sections render at correct opacity

---

### Phase 5: Dynamic Recalculation & Integration

**Goal**: Ensure the full reactive chain works — offer/withdraw in the helper modal updates the helper list, the people summary row, and the impact tiles.

**Reactive chain:**
1. User taps "Offer to Help" in `HelperListSheet` → calls `requestNotifier.offerToFulfill()`
2. `offerToFulfill()` sends RPC, then calls `loadRequestDetails()` (refreshes offerers + conversationParticipants) and `loadStats()` (refreshes impact)
3. `RequestState` updates → Riverpod rebuilds:
   - `HelperListSheet` (if still open) sees updated offerers/conversationParticipants and current user's offer status
   - `ContentPeopleRow` sees updated helper/in-conversation counts and avatar list
   - `ContentImpactTiles` sees updated `requestStats.potentialImpact`

**Changes:**
- Verify `offerToFulfill()` calls `invalidateStats()` + `loadStats()` after success
- Verify `withdrawOffer()` calls `invalidateStats()` + `loadStats()` after success
- Verify `saveSummaryAndMarkFulfilled()` refreshes stats (already stores `completionSavings`, ensure `requestStats` also updates)

**Tests:**
- Integration test: offer triggers stats reload → impact values update
- Integration test: withdraw triggers stats reload
- Integration test: full Details tab renders all sections in correct order

---

### Phase 6: Documentation Update

**Goal**: Update `docs/client/content.md` to document shared components.

**Changes:**
- Update the "Pane 1 — Details Tab" section to describe the new shared component layout
- Add shared widget entries to the "Shared Components Reference" table
- Update "Content Type Differences" table to reflect the new Details tab
- Update the "Last Updated" date

---

## File Summary

### New files (2)
| File | Purpose |
|------|---------|
| `app/lib/presentation/widgets/content/content_people_row.dart` | Avatar stack + summary text + chevron (shared) |
| `app/lib/presentation/widgets/request/helper_list_sheet.dart` | Helper list bottom sheet with offer/withdraw actions |

### Renamed/moved files (3)
| From | To |
|------|-----|
| `widgets/experience/experience_info_card.dart` | `widgets/content/content_info_card.dart` |
| `widgets/experience/experience_impact_tiles.dart` | `widgets/content/content_impact_tiles.dart` |
| `widgets/experience/experience_metric_sheet.dart` | `widgets/content/content_metric_sheet.dart` |

### Modified files
| File | Change |
|------|--------|
| `request_view_model.dart` | Add `requestStats` state field + `loadStats()` method |
| `request_content_view.dart` | Rebuild `_buildDetailsPane()` with shared widgets; remove `_DetailCard`, `_buildHelpersSection` |
| `experience_content_view.dart` | Update imports to shared widget paths |
| `experience_attendee_row.dart` | Simplify to thin wrapper around `ContentPeopleRow` |
| `item_metrics_screen.dart` | Update `context.l10n.experience*` → `context.l10n.impact*` references |
| `experience_metric_view_model.dart` | Update ARB key references from `experience*` → `impact*` |
| `app_en.arb` / `app_es.arb` | Rename `experience*` metric keys to `impact*`; add `request*` helper keys |
| `docs/client/content.md` | Document shared components, update Details tab section |

### Deleted code
- `_DetailCard` widget in `request_content_view.dart`
- `_buildHelpersSection()` method in `request_content_view.dart`

---

## Design Tokens

Same as experience-data.md — both views render over media backgrounds using dark glass styling.

**Content view (over media background — always dark glass):**

| Token | Value |
|-------|-------|
| Glass background | `Colors.black.withValues(alpha: 0.40)` |
| Glass border | `Colors.white.withValues(alpha: 0.10)` |
| Row divider | `Colors.white.withValues(alpha: 0.08)` |
| Muted text | `Colors.white.withValues(alpha: 0.40)` |
| Secondary text | `Colors.white.withValues(alpha: 0.65)` |
| Icon box coral tint | `AppColors.transferCoral` |
| Icon box sage tint | `AppColors.transferSage` |

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

1. **[i18n] ARB key rename scope incomplete.** Phase 1 listed only 3 files for the `experience*` → `impact*` ARB key rename, but `grep` found 4 consuming files: `experience_impact_tiles.dart`, `experience_metric_sheet.dart`, `item_metrics_screen.dart`, and `experience_metric_view_model.dart`. Added the missing files to Phase 1 and the File Summary. i18n rule: "Each new string needs both locales" — rename applies to both ARB files.

2. **[architecture] `completionSavings` vs `requestStats` clarification.** Phase 2 added `requestStats` alongside existing `completionSavings` without explaining the relationship. Added a note clarifying: `completionSavings` is set inline during the fulfillment modal flow (from `MarkRequestFulfilled` response), while `requestStats` is fetched separately for the Details tab. Both can coexist — they serve different UI flows.

3. **[i18n] Open questions resolved to decisions.** Open questions #1–3 were left as recommendations. Promoted to decisions since the plan cannot be implemented without resolving them.

4. **[architecture] Helper/conversation data source corrected.** Original audit added `GetRequestPeople` RPC + `loadPeople()` method, but user clarified: "Helpers" = `request.offerers`, "In Conversation" = `request.conversationParticipants` minus offerers. Both fields already exist on `GetRequestResponse` — no additional RPC or state field needed. People data refreshes automatically via `loadRequestDetails()`. Removed `requestPeople` state field and `loadPeople()` method from Phase 2.

---

**Last Updated:** 2026-03-30
