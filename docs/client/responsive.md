---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Desktop/adaptive layout system (#2912, #2926) — the constraint-first rule, the Responsive measure constants and the reading-vs-gallery measure split, ContentColumn and measureInset, the centralized sheet cap in showAccessibleModal, the NavDock capsule cap, the CalendarPanes two-pane switch, the SSR landing measure, and the desktop testing template (widget landscape matrix + @desktop Playwright specs + desktop walkthrough renders).
  globs: [app/lib/core/utils/responsive.dart, app/lib/presentation/widgets/adaptive/**, app/lib/presentation/widgets/home/calendar/calendar_panes.dart, app/lib/presentation/screens/discover/discover_screen.dart, website/content/css/event.css]
  triggers: [responsive, adaptive, desktop, breakpoint, measure, content-column, reading-measure, gallery-measure, shelf-inset, landscape, viewport, two-pane, letterbox]
  lens: [client, architecture]
  domain: client
freshness:
  verified_commit: "71d5880e6"
  verified_on: "2026-08-15"
---
# Responsive / desktop layout

How the app reads as designed on desktop-wide windows (#2912) while phone
layouts stay untouched. #2908 made the app *correct* at every viewport (the
month grids derive cell size from both axes); this layer is *composition
quality* on top of that — no width cap here may ever be load-bearing for fit.

## The rule: constraint-first, breakpoint-rare

Derive layout from the constraints a widget is actually given
(`LayoutBuilder`, `ConstrainedBox`), not from window classification. Most of
the system is breakpoint-free width caps that simply never bind on phones;
exactly **one** structural threshold exists, and one screen family consumes
it. New code that must read window size uses `MediaQuery.sizeOf(context)`
(rebuilds only on size changes — live browser resizes reflow without rebuild
storms), never `MediaQuery.of(context).size`. Adaptivity keys off available
width, never `kIsWeb` — a resized window, a tablet, and desktop web all get
the same treatment.

## The vocabulary (`core/utils/responsive.dart`)

| Constant | Value | Consumed by |
|---|---|---|
| `Responsive.contentMaxWidth` | 640 | `ContentColumn` — the *reading* measure: text streams (Home, People), the item routes' caption column, morph-panel bodies |
| `Responsive.galleryMaxWidth` | 1200 | `ContentColumn` — the *gallery* measure: the Library's header and shelf insets |
| `Responsive.sheetMaxWidth` | 560 | `showAccessibleModal`'s default `constraints` — every bottom sheet |
| `Responsive.dockMaxWidth` | 500 | `NavDock`'s self-cap on its capsule |
| `Responsive.expandedBreakpoint` | 840 | `CalendarPanes`' stacked ↔ side-by-side switch (via `LayoutBuilder`, not `MediaQuery`) |
| `Responsive.mobileBreakpoint` | 600 | `AuthScreenWrapper` (pre-#2912 auth-only treatment) |
| `Responsive.baseInset` | 20 | the default edge inset for surfaces that are not themselves width-capped |

**Reading measure vs gallery measure (#2926).** The measure a surface holds
must match the shape of its content. 640 is a *line-length* number — right for
prose, rows, and captions. A rail of fixed-size tiles is not bounded by line
length, and #2912's original choice to hold the Library at 640 left a 420px
void down the left of a 1440 window while tiles ran off the right. Pick
`galleryMaxWidth` when the surface lays out tiles, `contentMaxWidth` when it
lays out text. The cost is that the Library's header is wider than Home's and
People's, so it shifts horizontally when switching tabs — accepted: the
content genuinely differs, and matching them would reintroduce the void.

## The pieces

- **`ContentColumn`** (`widgets/adaptive/content_column.dart`) — `Center` +
  `ConstrainedBox(maxWidth:)`, nothing more. Below the measure it is a no-op
  by construction (the phone-safety argument, proven by
  `content_column_test.dart`'s bare-vs-wrapped rect-equality test). Apply per
  surface; it is a composition tool, not a shell clamp. It defaults to the
  reading measure; pass `maxWidth:` for a surface that holds another one.
- **`Responsive.measureInset`** — the leading inset that lines a *full-width*
  surface's content up with a centered column without constraining the
  surface. Used where a `ConstrainedBox` would be wrong: see the Library
  below. Below the measure it returns `baseInset`, which is what makes it a
  no-op on phones (`responsive_test.dart`).
- **Sheet cap** — `showAccessibleModal` defaults
  `constraints: BoxConstraints(maxWidth: Responsive.sheetMaxWidth)`;
  `showModalBottomSheet` centers a constrained sheet natively and keeps the
  tap-outside dismiss barrier intact. **Never** wrap a sheet in
  `Align`/`Center` instead — that breaks the barrier (documented in
  `glass_sheet.dart`). Callers passing their own `constraints` win. (Material
  3 already imposed a 640 default on modal sheets; the explicit constant makes
  the cap deliberate, uniform, and ours to tune.)
- **Dock cap** — `NavDock` centers its own capsule at `dockMaxWidth`; the
  slot math is `LayoutBuilder`-driven, so it simply receives the capped width.
- **`CalendarPanes`** (`widgets/home/calendar/calendar_panes.dart`) — the one
  structural switch: stacked grid-over-detail in compact windows
  (bit-identical to the phone layout — the stacked grid budget is computed by
  the caller exactly as before), grid beside a day-detail rail at
  `expandedBreakpoint` of available width. Adopted by all three calendar
  surfaces (Plans, shared calendar, workshop panel). Child order is
  grid-then-detail in both arms, so semantics traversal and keyboard focus
  order match the visual reading order.

  **Size the grid pane to the grid, then centre the pair (#2926 follow-up).**
  The first cut gave the grid an `Expanded` pane and centred the grid inside
  it. That is wrong for this grid specifically: `monthGridCellSide` caps cells
  at `maxCellSide` (96), so the grid *cannot* fill a wide pane, and `Center`
  split the surplus evenly — stranding 201px in the seam at 1440×810 and
  441px at 1920×1080 while the rail stayed at its 380 minimum and event titles
  wrapped and ellipsized. Now the pane takes `monthGridNaturalWidth(...)`, the
  rail absorbs the surplus up to `detailPaneWidth` (560) and never below
  `minDetailPaneWidth` (380, so the breakpoint is unchanged), and the leftover
  goes to the outer margins. Callers pass `gridRows: monthGridRows(month)` so
  the pane can predict the grid's width before laying out.
- **Caption column** — the item routes' read shells, edit panes, bottom
  panels, and `ContentMorphPanel` bodies wrap in `ContentColumn`
  bottom-centered over the full-bleed hero; `HeroContentWash` sizes to the
  sheet, so the wash follows the column. Heroes, maps (Library map mode),
  and scrims deliberately stay full-bleed.
- **Library shelves** (`screens/discover/discover_screen.dart`) — the one
  surface where the measure lives in scroll padding rather than in a
  `ConstrainedBox`. The header wraps in `ContentColumn(maxWidth:
  galleryMaxWidth)`; each shelf's title `Padding` and its horizontal
  `ListView`'s leading `padding` take `Responsive.measureInset(...,
  measure: galleryMaxWidth)`. The `ListView` keeps a **window-wide
  RenderBox** on purpose — two reasons, both load-bearing: bounding it
  shrinks the clip rect to start flush with the first tile's rounded corner,
  so any drag or overscroll bounce clips through that corner; and scrolling
  tiles off the left edge into the gutter is a wanted affordance, not a bug
  (#2926 — *"you can horizontally scroll things so they leave the screen on
  the left, so that part of the screen is usable"*). Map mode stays
  full-bleed, with only its header holding the same gallery measure so the
  header doesn't jump on toggle.
- **SSR `/go/` landings** — the body and CTA bar center at a 560px measure
  (`--ssr-measure` in `website/content/css/event.css`, shared by all four
  content landings). #2918: the hero title used to anchor to the window's
  left edge instead, so at 1440 it sat 432px left of everything below it. It
  now takes a `padding-inline: max(--space-5, calc((100% - --ssr-measure)/2
  + --space-4))` — above the measure its content box equals `.event-main`'s,
  below it the `max()` picks the original inset and phones render unchanged.
  `web-landing-desktop.spec.ts` asserts both the CTA bounds and the h1's
  left edge against `.event-main`'s content box.

## Testing template

Every adapted layout ships with **both** layers:

1. **Widget tests** at explicit surfaces: `tester.view.physicalSize` +
   `devicePixelRatio = 1` + `addTearDown(tester.view.reset)`; assert
   cap-and-center at `Size(1440, 810)` **and** unchanged geometry at
   `Size(390, 844)` (`content_column_test.dart` is the model). For a surface
   with its own breakpoint behavior, assert across a named-viewport matrix
   instead — the `'landscape viewports (#2908)'` group in
   `home_calendar_screen_test.dart` is the model.

   **Assert the exact edge, and assert paint, not boxes.** Two ways a layout
   assertion here passes without testing anything, both hit in #2926:
   a lower bound (`x >= bandLeft`, `detail.left >= grid.right`) cannot see a
   gutter that is too *large*, which is the whole failure mode — use an
   equality against the computed edge; and a widget's rect is not where it
   paints — under an `Expanded` pane `CalendarMonthGrid`'s box spanned the
   full pane while the grid painted centred inside it, so a seam assertion on
   that box was blind to the 201px gap. Measure the day cells' union instead.
   Verify by reverting the fix and watching the assertion fail.
2. **A `@desktop` Playwright spec** under the `desktop-chromium` project
   (1440×810): forward `test.info().project.use.viewport` into any manual
   context (they inherit nothing — #2908 trap), assert bounding-box geometry
   against a centered measure band (never tap success — `flt-semantics` nodes
   stay clickable while painted off-screen), `exact: true` where substrings
   collide, no `networkidle`. Current specs: `desktop-shell`,
   `streams-desktop`, `plans-calendar-landscape`, `item-routes-desktop`,
   `web-landing-desktop`.

For human judgment beyond geometry, render the desktop walkthrough variants:
`WALKTHROUGH_VIEWPORT=desktop e2e/scripts/run_walkthrough.sh requests` (and
`events`) — 1440×810 reels under `e2e/videos-walkthroughs/<reel>-desktop/`,
local-only, never deployed to the site (see `docs/walkthroughs.md`).

## What stays deliberately out

A global `MaterialApp.builder` shell clamp (#2908's "Option B") — rejected:
it needs a load-bearing `MediaQuery` override across 51 call sites and is a
dead end once real adaptive layouts exist. Multi-column Library/People grids,
an item-route side rail, and a nav rail are deferred until the constrained
compositions prove insufficient in real desktop use (see
`docs/issues/2912-desktop-adaptive.md` → Open Questions, and #2917).

Bounding the Library's shelf viewport to the gallery measure so tiles clip at
the gutter instead of at the window edge — rejected in #2926: the scroll-under
behaviour is the affordance working, and the clip trap above makes it visibly
wrong during drag.
