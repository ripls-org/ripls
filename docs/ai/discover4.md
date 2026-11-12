# Discover Screen Redesign v4

Reference designs: `tmp/discover4/discover.jsx`, `tmp/discover4/discover.png`, `tmp/discover4/discover2.png`

## Problems

### 1. Search Bar Buried in the Sheet
The search bar is currently inside the `DiscoverBottomSheet`, meaning it scrolls away and is only visible when the sheet is expanded. Users must drag the sheet up to search. The reference design shows the search bar as a **floating overlay at the top of the map**, always visible regardless of sheet position.

### 2. Limited Sheet Snap Positions
The current sheet has only three snap positions (30% peek, 55% half, 90% full). This doesn't give users fine-grained control over how much map vs. content they see. The reference design provides **four snap positions** that let users precisely balance map visibility against content browsing.

### 3. Help Requests / Giveaways Lack Visual Impact
Current request rows use a small avatar + text list layout. The reference design uses **large magazine-style photo cards** with gradient overlays, hero images, and typographic hierarchy — making help requests feel urgent and visually compelling.

### 4. Events Displayed as Plain List Rows
Events currently render as simple rows with small thumbnails. The reference design shows events as a **horizontal carousel** of photo cards with date badges, attendee stacks, and distance — a richer browsing experience.

### 5. Gear Section Uses Same Row Layout as Everything Else
The current sharing section looks identical to requests and events. The reference design uses a **2-column photo grid** with label badges (lending/giving), matching the "Available" section in `discover2.png`.

### 6. No Person Filter Chips
The current `DiscoverAvatarRow` shows bare avatars in a horizontal scroll. The reference shows **pill-shaped chips** with avatar + first name, an "All" chip with a star icon, and a selected state with coral highlight. These act as interactive filters for the results below.

## Solutions & Recommendations

### Search Bar → Floating Header
**Recommendation: Integrate search into the existing `FloatingHeader` widget.**

Move the search `TextField` into the `FloatingHeader` using its `content` parameter. The header already has the menu icon, community name, and trailing slot. Replace the community name text with a borderless search input that uses the community name as placeholder text. Keep the notification badge count indicator and my-location button as trailing actions.

The search bar should be subtle — no border, no background on the input area itself. The existing `FloatingHeader` container provides the frosted glass pill background. The `Open/All` toggle can move to a small icon in the trailing area.

### Four-Position Snap Sheet
**Recommendation: Change `DraggableScrollableSheet` snap sizes.**

Update the snap positions from `[0.30, 0.55, 0.90]` to:

| Position | Fraction | What's Visible |
|----------|----------|----------------|
| Short    | 0.20     | Only person filter chips peek above tab bar. Maximum map. |
| Medium   | 0.40     | Person chips + first help request card (hero photo visible). |
| Starting | 0.60     | Default. Map + search bar clearly visible. Shows people, help requests, start of events. |
| Tall     | 0.80     | Nearly full screen. Only floating search bar visible above sheet. |

Initial size: 0.60 (starting). Min size: 0.20 (short).

### Magazine-Style Help Request Cards
**Recommendation: Create a new `DiscoverHelpCard` widget.**

Replace `DiscoverRequestRow` usage in the bottom sheet with large photo cards:
- **Hero card** (first item): ~210px tall, full-width rounded image with gradient overlay. Requester avatar + name at top-left, coral "HELP" badge at top-right, request title in large serif font at bottom.
- **Secondary cards**: ~140px tall, same layout but slightly smaller typography.
- Include both help requests and giveaways in this section (filter by `searchResult.request` and giveaway gear items).
- Section header: pulsing coral dot + "HELP NEEDED" uppercase label + "{N} open" count.

### Event Carousel
**Recommendation: Create a new `DiscoverEventCarousel` widget.**

Replace the events `SliverList` with a horizontal `ListView` carousel:
- Each card: ~230px wide, ~160px tall, rounded corners, photo with gradient overlay.
- Date badge (pill) at top-left over the photo.
- "Experience" label, event title (serif), attendee avatar stack + "{N} going" + distance at bottom.
- Section header: "Upcoming" in serif font + "See all {N}" link.
- Carousel scrolls horizontally with edge-to-edge bleed (negative margin + padding pattern).

### Gear Grid
**Recommendation: Create a new `DiscoverGearGrid` widget.**

Replace the sharing `SliverList` with a 2-column `SliverGrid`:
- Each cell: rounded corners, photo with gradient overlay.
- Label badge at top-left ("LENDING" on translucent white, "GIVING" on sage green).
- Gear title + owner name + distance at bottom over gradient.
- Section header: "Available" in serif font + "See all {N}" link.

### Person Filter Chips
**Recommendation: Redesign `DiscoverAvatarRow` as pill-shaped chips.**

Each chip: rounded pill with avatar (22px) + first name. Selected state: coral background/border. "All" chip with star icon leads the row. Tapping a chip filters all three sections. Tapping an already-selected chip deselects (shows all).

## Open Questions

1. **Giveaways in Help Needed section**: The reference JSX mixes help requests and giveaways into "Help Needed." Should giveaways appear here, in the Gear grid, or both? **Recommendation**: Show giveaways in the Help Needed section since they represent items people are actively offering (similar urgency to help requests). The gear grid shows only lendable items.

2. **Search bar scope**: When the user types in the floating search bar, should results still appear in the three categorized sections, or switch to a unified search results list? **Recommendation**: Keep the categorized layout — the search just filters each section.

3. **Gallery overlay interaction**: The current carousel overlay appears when a map marker is tapped (at 32% from bottom). With the new sheet positions, where should this overlay render? **Recommendation**: Position it just above the sheet's current top edge, adapting dynamically to the current snap position.

4. **Sheet height on initial load**: The reference JSX defaults to "starting" (60%). The current implementation uses 30% peek. Is 60% the right default? **Recommendation**: Yes, 60% — it shows meaningful content immediately while keeping the map and search bar visible.

5. **Section expand/collapse**: Current sections have "See all (N)" / "Show less" behavior. Should this be preserved with the new card layouts? **Recommendation**: Keep it. Show 2 help cards in preview mode, full carousel for events (it's already compact), and 4 gear items (2×2 grid) in preview.

## Implementation Plan

### Phase 0: Fix `_refreshResults()` Architecture Violation
Move repository calls out of `DiscoverScreen` and into the ViewModel layer.

**Problem:** `_refreshResults()` in `discover_screen.dart` calls repositories directly (e.g., `ref.read(communityRepositoryProvider).refreshCommunityGear(...)`), violating architecture.md rule §1: "ViewModels are the ONLY layer that calls Repositories."

**Files to modify:**
- `app/lib/presentation/viewmodels/search_view_model.dart` — Add a `refreshAllCaches()` method to `SearchNotifier` that performs the full cache invalidation sequence currently in `_refreshResults()`: refresh community gear/requests/experiences, invalidate search/gear/request/experience/user/media/location caches, then call `refresh()`.
- `app/lib/presentation/screens/discover/discover_screen.dart` — Replace the body of `_refreshResults()` with a single call: `await ref.read(searchProvider.notifier).refreshAllCaches()`.

### Phase 1: Search Bar Migration
Move the search bar from `DiscoverBottomSheet` into the `FloatingHeader` on `DiscoverScreen`.

**Files to modify:**
- `app/lib/presentation/screens/discover/discover_screen.dart` — Replace community name `FloatingHeader` with search-enabled version using `content` parameter. Add `TextEditingController` for search. Wire `onChanged` to `searchProvider.notifier.searchWithLocation()` via `ref.read()` (event handler pattern — correct per architecture rules). Move Open/All toggle to trailing area.
- `app/lib/presentation/widgets/discover/discover_bottom_sheet.dart` — Remove `_buildSearchBar()` method and its sliver. Remove `_searchController`. Remove `onTap` expand-to-half logic (the search field is now always visible).

**Details:**
- FloatingHeader `content`: borderless `TextField` with community name as hint, no decoration borders, transparent background.
- FloatingHeader `trailing`: Row of [Open/All toggle icon, my-location button].
- Notification badge count moves to a small dot inside the search area (right side), matching the reference design's coral dot + count.

<!-- AUDIT FIX (architecture.md §3 — Screens only interact with ViewModels):
The `TextEditingController` is local UI state, so keeping it in the screen's
`ConsumerStatefulWidget` is correct per architecture rules (setState for local UI
state only). Search triggering via `ref.read(searchProvider.notifier)` is also
correct — screens may call notifier methods through `ref.read()` in event handlers. -->

### Phase 2: Four-Position Sheet Snap
Change the `DraggableScrollableSheet` snap positions and update all related constants.

**Files to modify:**
- `app/lib/presentation/widgets/discover/discover_bottom_sheet.dart` — Update `_peekSize` → 0.20, `_halfSize` → 0.40, add `_startingSize` = 0.60, `_fullSize` → 0.80. Update `snapSizes` list to `[0.20, 0.40, 0.60, 0.80]`. Update `initialChildSize` to 0.60.
- `app/lib/presentation/screens/discover/discover_screen.dart` — Update `_sheetPeekSize` to 0.20. Update gallery overlay bottom position to account for dynamic sheet top. Update `_collapseSheet()` target.

### Phase 3: Person Filter Chips
Redesign the avatar row as pill-shaped filter chips.

**Files to modify:**
- `app/lib/presentation/widgets/discover/discover_avatar_row.dart` — Rebuild as horizontal scroll of pill chips. Each chip: `Container` with `borderRadius: 24`, avatar (22px) + first name text. "All" chip with star icon first. Selected state: coral background + border. Add `AnimatedContainer` for smooth selection transitions. Widget remains a `ConsumerWidget` — it needs `ref` to watch `mediaUrlProvider` for avatar images.

### Phase 4: Magazine-Style Help Cards
Replace request list rows with large photo card layout.

**Files to create:**
- `app/lib/presentation/widgets/discover/discover_help_card.dart` — New `ConsumerWidget`. Hero variant (210px) and secondary variant (140px). Photo background via `CachedMediaImage` with `ImageCacheKeys.full()` for the hero card (210px ≥ 200px threshold) and `ImageCacheKeys.thumbnail()` for secondary cards (140px < 200px). Gradient overlay, avatar + name at top, "HELP" badge, serif title at bottom. Handles both requests and giveaways.

**Files to modify:**
- `app/lib/presentation/widgets/discover/discover_bottom_sheet.dart` — Replace requests section: new section header with pulsing dot + "HELP NEEDED" + open count. Use `DiscoverHelpCard` instead of `DiscoverRequestRow`. First item gets hero treatment, rest get secondary.

<!-- AUDIT FIX (caching.md — Image Cache Keys § Choosing Thumbnail vs Full):
Original plan mentioned `CachedMediaImage` but didn't specify cache key sizing.
Per caching.md, images displayed at ≥ 200px must use `ImageCacheKeys.full()`,
and images < 200px use `ImageCacheKeys.thumbnail()`. Fixed to specify:
- Hero card (210px) → `ImageCacheKeys.full(mediaId)`
- Secondary card (140px) → `ImageCacheKeys.thumbnail(mediaId)` -->

<!-- AUDIT FIX (CLAUDE.md — Media Display):
New widgets displaying user-uploaded media MUST use `CachedMediaImage` (never
`Image.network`). The widget must be a `ConsumerWidget` to access
`mediaUrlProvider` for resolving presigned URLs. Use stable `cacheKey` from
`MediaUrl` objects returned by the repository. -->

### Phase 5: Event Carousel
Replace event list with horizontal photo card carousel.

**Files to create:**
- `app/lib/presentation/widgets/discover/discover_event_carousel.dart` — New `ConsumerWidget`. Horizontal `ListView.builder` of 230px-wide cards. Photo displayed via `CachedMediaImage` with `ImageCacheKeys.thumbnail()` (160px < 200px threshold). Gradient overlay, date badge pill, "Experience" label, title in serif, attendee avatar stack, going count, distance.

**Files to modify:**
- `app/lib/presentation/widgets/discover/discover_bottom_sheet.dart` — Replace events section with `SliverToBoxAdapter` containing the carousel. Section header: "Upcoming" serif + "See all {N}".

<!-- AUDIT FIX (caching.md — Image Cache Keys):
Event carousel cards are 160px tall — below the 200px threshold, so
`ImageCacheKeys.thumbnail()` is correct. Specified explicitly. -->

### Phase 6: Gear Grid
Replace sharing list with 2-column photo grid.

**Files to create:**
- `app/lib/presentation/widgets/discover/discover_gear_grid.dart` — New `ConsumerWidget`. Returns a `SliverGrid` with 2 columns. Each cell: photo via `CachedMediaImage` with `ImageCacheKeys.thumbnail()` (~115px per cell, well below 200px threshold), gradient overlay, label badge (LENDING/GIVING), gear title + owner + distance overlay.

**Files to modify:**
- `app/lib/presentation/widgets/discover/discover_bottom_sheet.dart` — Replace sharing section with gear grid sliver. Section header: "Available" serif + "See all {N}".

<!-- AUDIT FIX (caching.md — Image Cache Keys):
Gear grid cells are ~115px — use thumbnail keys. Specified explicitly. -->

### Phase 7: Polish & Integration
Final adjustments and cleanup.

**Files to modify:**
- `app/lib/presentation/screens/discover/discover_screen.dart` — Adjust gallery overlay positioning for new sheet heights. Ensure search state is properly wired between floating header and sheet content.
- `app/lib/presentation/widgets/discover/discover_bottom_sheet.dart` — Remove `_buildSearchBar` if not already removed. Verify section expand/collapse works with new card layouts. Tune spacing between sections.
- Clean up unused imports and dead code from old row widgets if they're fully replaced (keep `DiscoverRequestRow`, `DiscoverEventRow`, `DiscoverSharingRow` if still used elsewhere; delete if not).

### Phase 8: Tests
Add widget tests for new components.

**Files to create:**
- `app/test/presentation/widgets/discover/discover_help_card_test.dart`
- `app/test/presentation/widgets/discover/discover_event_carousel_test.dart`
- `app/test/presentation/widgets/discover/discover_gear_grid_test.dart`
- `app/test/presentation/widgets/discover/discover_avatar_row_test.dart`

**Test coverage:**
- Help card renders hero and secondary variants, handles missing photos gracefully.
- Event carousel renders cards, horizontal scrolling works, date badge shows correct text.
- Gear grid renders 2 columns, label badges show correct text and color.
- Person filter chips: "All" chip selected by default, tapping person chip filters, tapping again deselects.

Run all tests with `flutter test --reporter expanded` (never use the default compact reporter — it produces enormous output when piped).

<!-- AUDIT FIX (architecture.md §Testing + CLAUDE.md §Testing):
- Added test file for the redesigned avatar row (discover_avatar_row_test.dart)
- Added `--reporter expanded` reminder per CLAUDE.md testing conventions -->

### Phase 9: Simplify Sheet Snap Positions & Section Defaults
Reduce sheet snap positions from four to three, remove "See all" from Upcoming events, and increase the default visible gear count.

**Problem:** Four sheet snap positions (peek/medium/starting/full) create an awkward "medium" state that isn't useful — users either want peek (maximum map), starting (default browse), or full (focused browsing). The Upcoming events section has a "See all" link but it's a horizontal carousel that should just load all events by default. The Available gear section only shows 4 items (2×2) in preview mode, which feels sparse — 20 items is a better default before requiring "See all."

**Changes:**

1. **Three-position sheet** — Remove the `_mediumSize = 0.40` snap position. Keep only:
   | Position | Fraction | What's Visible |
   |----------|----------|----------------|
   | Peek     | 0.20     | Person filter chips peek above tab bar. Maximum map. |
   | Starting | 0.60     | Default. Map + search bar visible. Shows people, help cards, start of events. |
   | Full     | 0.86     | Nearly full screen. Only floating search bar visible above sheet. |

   Update `snapSizes` to `[0.20, 0.60, 0.86]`. Remove the `_mediumSize` constant.

2. **Upcoming events — remove "See all"** — The event carousel is horizontal and compact; it should load all events by default. Remove the "See all {N}" link from `_buildUpcomingSectionHeader()`. The section header becomes just "Upcoming" with no trailing action.

3. **Available gear — show 20, conditional "See all"** — Change the preview display count from `sharing.length.clamp(0, 4)` to `sharing.length.clamp(0, 20)`. Only show the "See all" link when `sharing.length > 20`. When there are 20 or fewer items, hide both "See all" and "Show less" since all items are already visible.

4. **Drag handle + avatar row must drive sheet position** — The fixed header area (drag handle and avatar row) is already wrapped in a `GestureDetector` with `onVerticalDragUpdate` / `onVerticalDragEnd` handlers that move the sheet. When removing `_mediumSize`, update `_onHeaderDragEnd`'s local `snapSizes` list (currently `[_peekSize, _mediumSize, _startingSize, _fullSize]`) to match the new three-position set `[_peekSize, _startingSize, _fullSize]`. Verify that dragging up/down on both the handle and the avatar chip row still smoothly adjusts the sheet height and snaps to the correct positions.

**Files to modify:**
- `app/lib/presentation/widgets/discover/discover_bottom_sheet.dart` — Remove `_mediumSize` constant, update `snapSizes` list in both the `DraggableScrollableSheet` and `_onHeaderDragEnd`, remove "See all" from events header, change gear preview count from 4 → 20, conditionally hide "See all"/"Show less" for gear when count ≤ 20.
- `app/lib/presentation/screens/discover/discover_screen.dart` — Update any references to `_mediumSize` or the four-position snap logic (e.g. `_collapseSheet()` target, gallery overlay positioning).

---

## Audit Notes

**Audited against:** `docs/client/architecture.md`, `docs/client/caching.md`, `docs/server/architecture.md`

### Findings & Fixes Applied

1. **Image cache key sizing (caching.md)** — Original plan mentioned `CachedMediaImage` but never specified whether to use `thumbnail()` or `full()` cache keys. Per caching.md §Choosing Thumbnail vs Full, images ≥ 200px use `full()` and < 200px use `thumbnail()`. Fixed in Phases 4–6 with explicit key selection per display size.

2. **Widget types for provider access (architecture.md §1)** — Original plan didn't specify whether new widgets should be `ConsumerWidget` or plain `StatelessWidget`. All new card/carousel/grid widgets need `ref` to resolve media URLs via `mediaUrlProvider`, so they must be `ConsumerWidget`. Fixed in Phases 3–6.

3. **Missing avatar row test (architecture.md §Testing)** — Phase 8 tested the three new widgets but omitted a test for the redesigned `DiscoverAvatarRow`. Added `discover_avatar_row_test.dart`.

4. **Flutter test reporter (CLAUDE.md §Testing)** — Phase 8 didn't mention `--reporter expanded`. Added reminder.

5. **`ref.read()` vs `ref.watch()` correctness (architecture.md §State Management)** — Phase 1 wires search `onChanged` to `ref.read(searchProvider.notifier)`. Confirmed correct: `ref.read()` is the right pattern for event handlers. Added clarifying comment.

### Confirmed Compliant (No Changes Needed)

- **No server changes** — This is purely a client-side UI redesign. No new RPCs, proto changes, or server modifications needed. Server architecture is unaffected.
- **No new repositories or services** — All data comes from existing `SearchState` (requests, events, sharing getters). No new data layer code.
- **No manual cache maps in ViewModels** — The plan doesn't introduce any ViewModel caching. All data flows through existing `searchProvider` → `SearchRepository` pipeline.
- **Repository pattern respected** — No direct service access from screens or new widgets. Media URLs resolved through `mediaUrlProvider` which wraps `MediaRepository`.
- **`CachedMediaImage` for all user media** — Plan correctly uses `CachedMediaImage` (not `Image.network`) for all user-uploaded photos, per CLAUDE.md media display rules.
- **`TextEditingController` as local UI state** — Keeping it in `ConsumerStatefulWidget.initState()` is correct per architecture rules (setState for local UI state only).
- **Navigation patterns** — Item taps still use `NavigationHelpers.pushScreen()` with `useRootNavigator: true`, matching established navigation conventions.
- **No new architecture layers** — All new code is presentation-layer widgets in `app/lib/presentation/widgets/discover/`, consistent with directory structure.

### Pre-existing Issue (Not Introduced by This Plan)

- **`_refreshResults()` in `discover_screen.dart` calls repositories directly** (e.g., `ref.read(communityRepositoryProvider).refreshCommunityGear(...)`). This violates architecture.md rule §1: "ViewModels are the ONLY layer that calls Repositories." This is pre-existing debt — the plan doesn't worsen it, but a future cleanup should move this logic into the ViewModel layer.
