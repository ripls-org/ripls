# Issue #2912: Make the app look intentional on desktop web aspect ratios

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2912
**Filed:** 2026-08-14 | **Last activity:** 2026-08-14
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** `docs/client/architecture.md` ⚓, `docs/client/conventions.md` ⚓,
`docs/users/priorities.md` ⚓, `docs/users/personas.md` ⚓, `docs/client/design.md`,
`docs/client/content.md`, `docs/client/inbox.md`, `docs/client/calendar.md`,
`docs/client/modals.md`, `docs/walkthroughs.md`, `docs/client/accessibility.md`,
`docs/issues/2908-web-aspect-ratios.md` (parent investigation),
`docs/reviews/accessibility_review_prompt.md`,
`docs/reviews/testing_review_prompt.md`,
`docs/reviews/performance_review_prompt.md`,
`docs/reviews/code_quality_review_prompt.md`,
`docs/reviews/tech_debt_review_prompt.md`

## Problem Statement

#2908 made the app **correct** at desktop aspect ratios — nothing is clipped or
unreachable any more (fixed in PR #2913, merged 2026-08-14). This issue is the
follow-up it explicitly deferred: the app still has exactly **one designed
viewport shape** — a tall phone — and at a 1440×810 browser window every screen
reads as that phone layout stretched sideways:

- **Home** — the zero-state hero card and its "Ask the crew" / "Offer
  something" pills stretch to ~1400px because the card's `Column` uses
  `CrossAxisAlignment.stretch` and nothing above it constrains width; body copy
  runs far past a comfortable reading measure.
- **Item routes** (gear / request / experience) — the bottom read-shell sheet
  (`Positioned(left: 0, right: 0)` + `HeroContentWash`) spans the window, so
  cards like "See the calendar ›" become window-wide pills.
- **Library / People** — single-column lists against a very wide empty field.
- **Plans** — half-fixed: #2908's `CalendarMonthGrid` is now a centered ≤702px
  block, but the day-detail panel below it is still `Alignment.bottomLeft` at
  full window width, so the screen visibly disagrees with itself.
- **Modals** — *(correction during implementation: Material 3's default
  already capped modal sheets at 640, so sheets were never the window-wide
  slabs the pre-implementation static read claimed — the framework default
  was simply invisible to it. The explicit `sheetMaxWidth` cap makes the
  measure deliberate, uniform, and locally owned.)* The Data Consent sheet's
  action row is a confirmed near-miss at 1440×810.
- **Nav dock** — a ~1412px-wide glass capsule with four 22px icons huddled in
  proportionally-stretched slots.

The issue body lays out two directions: **A**, clamp the whole shell to a
phone-width column via the `MaterialApp.router` `builder:`; **B**, genuinely
adaptive layouts. It flags the choice as a product call. **That call has now
been made**: the issue was handed to planning with the explicit direction
*"please actually make the app look natural / truly adaptive for web"* — i.e.
direction B is the destination, and a letterboxed phone column is not an
acceptable end state.

## Context from Comments

The issue has no comments — it was filed directly from the #2908 follow-up
work, and its body already carries the per-screen findings from that
investigation's sweep. The one steer that exists arrived out-of-band with the
planning request itself (quoted above), and it resolves the "clamp vs. adaptive"
product question the body left open. The remaining open choices are inside
direction B (breakpoint values, per-screen compositions) — see Open Questions.

## UI Investigation

**Deliberately not re-run.** The `ripls-ui-investigate` skill already executed
against this exact failure surface under #2908 (verdict, hypothesis table,
width-sweep evidence, and screenshots in
[`2908-web-aspect-ratios.md`](2908-web-aspect-ratios.md) → *UI Investigation*),
and this issue's body **is** the per-screen residue of that sweep. There is no
new misbehavior to bisect — the layer is known (client render, layout quality),
the mechanism is known (no width constraints anywhere outside
`AuthScreenWrapper`), and re-running the harness would reproduce screenshots we
already have. The codebase survey below supplies the equivalent grounding for
the quality work.

## What the codebase actually provides today

Facts the plan builds on (verified 2026-08-14 on `main` post-#2913):

1. **`Responsive` is greenfield.** `app/lib/core/utils/responsive.dart` is 25
   lines — one constant (`mobileBreakpoint = 600`), `isMobile()`, and two
   `MediaQuery` passthroughs — imported by exactly one file,
   `app/lib/presentation/widgets/adaptive/auth_screen_wrapper.dart`. No
   `isDesktop`, no `LayoutBuilder` helpers. A repo-wide grep for
   `breakpoint|isDesktop|isTablet` finds nothing else.
2. **The only width clamp in the product is `AuthScreenWrapper`** — a centered
   `maxWidth: 480` elevated `Card` over a soft gradient, applied at all 7 auth
   screens. This is the existing precedent for "centered surface on desktop."
3. **#2908 already established the winning sizing pattern**:
   `monthGridCellSide` derives the day cell from *both* axes and
   `CalendarMonthGrid` renders `Center(child: SizedBox(width:
   monthGridWidth(side)))`. Constraint-derived layout, no breakpoint, no-ops on
   phones. The generalization of this idea is the core of the plan.
4. **Every bottom sheet has a single centralized lever.**
   `showAccessibleModal` (`app/lib/presentation/widgets/accessibility/show_accessible_modal.dart`)
   forwards a `constraints:` parameter to `showModalBottomSheet`, which centers
   a constrained sheet natively. `glass_sheet.dart:143-147` documents why an
   `Align`/`Center` wrapper must **not** be used (it breaks the tap-to-dismiss
   barrier); the `constraints:` parameter is the correct mechanism.
5. **51 `MediaQuery...size` call sites — 48 read `.height`, 3 read `.width`.**
   Almost all are modal-height caps and overlay anchors. This is the load
   under direction A's `MediaQuery` override; under the per-surface approach
   below they are untouched and stay correct.
6. **Zero `kIsWeb` layout branches** (57 sites, all plugin/capability gating).
   The plan keeps it that way: adaptivity keys off available width, never off
   platform, so a resized window, a tablet, and desktop web all get the same
   treatment for free.
7. **The nav shell is `HomeScreen`'s own `Stack`** — `IndexedStack` body plus a
   `Positioned(left: 0, right: 0, bottom: 0)` floating `NavDock` whose slot
   widths are purely proportional (`nav_dock.dart:184-199`). Not a
   `ShellRoute`; there is one obvious place to cap the capsule.
8. **CI has exactly one desktop assertion** — the `desktop-chromium` Playwright
   project (1440×810, scoped to `@desktop`) with one spec,
   `plans-calendar-landscape.spec.ts`. Both #2908 traps are live: manual
   contexts don't inherit the project viewport (must forward
   `test.info().project.use.viewport`), and `flt-semantics` nodes stay tappable
   while painted off-screen (assert bounding-box geometry, never tap success).
9. **Widget tests have a landscape template**:
   `home_calendar_screen_test.dart`'s `landscape viewports (#2908)` group pumps
   five named `Size`s at `pixelRatio: 1` and asserts geometry. New adaptive
   screens copy this shape.
10. **`main.dart`'s `builder:` is occupied only by a layout-neutral l10n
    seeder**, and the splash path is a *second* `MaterialApp` with no `builder:`
    at all — two more reasons direction A is less "one small change" than it
    looks.

## Potential Solutions

### Option A: Global max-width shell clamp (the issue's direction A)

- **Approach:** Center the whole app in a ~640px column via the
  `MaterialApp.router` `builder:`, overriding `MediaQuery` with
  `.copyWith(size: …)` so the 51 size readers observe the clamp; fill the
  letterbox with a themed surface.
- **Pros:** One change point; every screen "fixed" at once; honest about
  phone-first heritage.
- **Cons:** Explicitly ruled out as the end state by the direction attached to
  this issue — a letterboxed phone column is not "natural / truly adaptive."
  Mechanically riskier than advertised: the `MediaQuery` override is
  load-bearing for 51 call sites (dropping `textScaler`/`padding`/`viewInsets`
  via a fresh `MediaQueryData` would be an app-wide a11y regression), the
  splash `MaterialApp` needs separate handling, and the seeder must be
  preserved in the builder chain. It is also a dead end: none of the work
  survives a later move to real adaptive layouts.
- **Effort:** Small–medium (the clamp is small; verifying 51 sites + overlay
  surfaces is the bulk).

### Option B: Constraint-first per-surface adaptivity (recommended)

- **Approach:** Generalize the pattern #2908 already proved. Two moves, in
  order of leverage:
  1. **A content-measure primitive** — a tiny widget (working name
     `ContentColumn`: `Center` + `ConstrainedBox(maxWidth: measure)` +
     existing horizontal padding) applied *per surface* to the app's stream
     screens (Home, Library, People), the item routes' bottom read shell, and
     — via the centralized `showAccessibleModal` — every bottom sheet, plus a
     width cap on the nav dock capsule. This is breakpoint-free: on any window
     narrower than the measure it is a no-op by construction, so phone layouts
     are provably untouched and there is no platform fork and no `MediaQuery`
     override. It delivers ~90% of what direction A promised, without A's
     risk, and every line of it survives into full adaptivity.
  2. **Structural two-pane compositions only where they earn their place** —
     starting (and possibly ending) with Plans: at expanded widths the month
     grid sits beside a day-detail side panel instead of above a starved
     bottom strip, which is the one screen the issue names as benefiting most
     and the one place a real breakpoint is needed.
- **Pros:** Matches the stated direction; uses the space instead of hiding it;
  incremental and shippable per phase; each phase is independently
  verifiable with the #2908 test machinery; keeps `kIsWeb` out of layout;
  leaves the 51 `MediaQuery` call sites alone; establishes a vocabulary
  (`ContentColumn`, one `expanded` breakpoint) that future screens adopt
  instead of multiplying ad-hoc permutations.
- **Cons:** Touches many files across phases (though each phase is small);
  the two-pane Plans layout adds a real layout permutation that future
  calendar changes must be checked against — mitigated by widget tests at the
  landscape matrix plus a `@desktop` spec.
- **Effort:** Medium overall; small per phase.

### Option C: Full breakpoint system with per-screen desktop compositions everywhere

- **Approach:** A complete window-size-class system (compact/medium/expanded)
  with bespoke desktop compositions on every screen — multi-column
  Library/People grids, side-rail navigation, two-pane item routes with a
  detail rail, adaptive dialogs-instead-of-sheets.
- **Pros:** The maximal reading of "truly adaptive"; desktop-native feel
  everywhere.
- **Cons:** Multiplies layout permutations across ~every screen at once, with
  today's desktop CI coverage at exactly one spec; most of it is speculative —
  web is still primarily the `/go/` entry surface and phone-first registration
  path (see `docs/users/priorities.md`: the wedge personas live on phones),
  so bespoke desktop compositions beyond the high-traffic surfaces have no
  user demand signal yet. Violates the project's minimal-dependencies /
  no-over-engineering instinct.
- **Effort:** Large.

## Recommendation

**Option B.** Ship the constraint-first foundation (measure primitive + modal
constraint + dock cap) first — it is cheap, breakpoint-free, phone-safe by
construction, and immediately makes every modal, the dock, and the stream
screens read as designed rather than stretched. Then do the structural work
screen-by-screen in descending order of visible payoff: Home streams →
Plans two-pane → item routes. Stop before Option C's speculative surfaces
(multi-column grids, nav rails) and file them as follow-ups only if the
constrained compositions still read as wasteful once real desktop usage
exists.

Two principles carry through every phase, both inherited from #2908's
hard-won lessons:

1. **Constraint-first, breakpoint-rare.** Derive layout from the constraints a
   widget is actually given (`LayoutBuilder` / `ConstrainedBox`), not from
   window classification. A breakpoint is introduced only where the *structure*
   changes (Plans two-pane), and there is exactly one new threshold. New code
   that must read window size uses `MediaQuery.sizeOf(context)`, not
   `MediaQuery.of(context).size`, so widgets rebuild only on size changes
   (live browser-window resizes reflow without rebuild storms).
2. **No layout correctness may depend on a width cap.** #2908's grid fix
   guarantees fit; everything in this issue is composition quality. If
   removing every `ContentColumn` would clip content somewhere, that is a
   #2908-class bug to fix at its root, not something this issue's widgets are
   allowed to paper over.

## Open Questions

- **What is the content measure?** — Recommendation: **640 logical px** for
  stream/text surfaces (the issue's own number; comfortably above the 480 auth
  card, close to classic 60–75ch at body sizes) and **560 px** for bottom
  sheets (sheets are focused tasks; the narrower measure keeps footer buttons
  reachable in one glance). Define both once, next to the breakpoint, so a
  later design pass tunes two numbers rather than thirty call sites.
- **Where does the vocabulary live?** — Recommendation: extend
  `app/lib/core/utils/responsive.dart` (constants + `isExpanded(context)`
  reading `MediaQuery.sizeOf`) and put the `ContentColumn` widget in
  `app/lib/presentation/widgets/adaptive/` beside `AuthScreenWrapper` (the
  directory becomes multi-file → gets the mandatory `README.md`). Do not
  create a parallel "responsive2" concept; `Responsive` finally earns its
  name.
- **What is the expanded breakpoint?** — Recommendation: **840 px** (Material
  window-size-class boundary for expanded). Only Plans' two-pane switch
  consumes it initially. The existing 600 stays as-is for
  `AuthScreenWrapper`.
- **Should the modal max-width apply inside `showAccessibleModal` (all sheets)
  or per call site?** — Recommendation: inside `showAccessibleModal`, as a
  default `BoxConstraints(maxWidth: sheetMaxWidth)` that callers can override
  — one change point covers every glass sheet, `SolidSheet`, and (verify) the
  consent dialog's `showStandardModal` path. Callers passing their own
  `constraints` keep them. Must use the `constraints:` parameter, never an
  `Align` wrapper (documented barrier-breaking trap in `glass_sheet.dart`).
- **Item routes: bottom-center caption column, or a side detail rail?** —
  Recommendation: bottom-center the read shell at the 640 measure over the
  full-bleed hero (poster-with-caption-column; an established editorial
  pattern and a small diff to three structurally-identical read shells). A
  side rail is a Option-C-scale redesign of the morph-panel system — defer,
  and let the centered composition prove whether anyone misses it.
- **Multi-column Library / People?** — Recommendation: defer. Constrain both
  to the measure now; file a follow-up for a two-column expanded grid only if
  the constrained single column reads as wasteful in real use. (Library's
  horizontal shelves already use width naturally once centered.)
  **CORRECTION (#2926, 2026-08-15): the parenthetical was wrong.** Centering
  the shelves at the *reading* measure left a 420px void down the left of a
  1440 window while tiles ran off the right, which is exactly the
  "constrained column reads as wasteful" outcome this question gated on.
  Fixed by giving the Library its own wider gallery measure
  (`Responsive.galleryMaxWidth`), not by the multi-column grid — #2917 stays
  open for that. See `docs/issues/2926-2918-desktop-width-seams.md`.
- **Dock: capped capsule or nav rail?** — Recommendation: capped centered
  capsule (~500 px max) — one `ConstrainedBox` around the existing capsule,
  zero navigation-paradigm change. A left rail is Option-C territory.
- **Which walkthrough reels get desktop variants?** — Recommendation:
  `requests` (the appless `/go/`-link guest journey — the product's most
  genuinely desktop scenario) and `events` (broadest in-app surface
  coverage). Local evaluation artifacts only, never deployed to the site's
  walkthrough index; add more reels only if these two leave a changed surface
  unwatched. See *Desktop walkthrough evaluation loop* under Testing
  strategy.

## Status

**All five phases implemented** on branch `desktop_adaptive` (2026-08-14).
What shipped, phase by phase, with deltas from the plan:

- **Phase 1** — `Responsive` constants (four measures; the dead
  `width()`/`height()` passthroughs deleted; `isMobile` moved to
  `MediaQuery.sizeOf`), `ContentColumn` + the `widgets/adaptive/` README,
  the `showAccessibleModal` default sheet cap, the `NavDock` self-cap, and
  `desktop-shell.spec.ts`. The walkthrough pipeline gained
  `WALKTHROUGH_VIEWPORT=desktop` (capture recipe, `-desktop` output dirs, no
  site deploy, a 2× action-timeout budget for the larger screencast), and
  **baseline desktop renders of `requests` + `events` were captured against
  the pre-change layouts** under `e2e/videos-walkthroughs/*-desktop/`.
- **Phase 2** — Home/People wrap their sections in `ContentColumn`; Library
  centers its header and aligns each shelf's *content inset* to the measure
  while the shelf scroll region stays window-wide (constraining the region
  clips tiles during overscroll — the in-code outer-Padding trap
  generalizes); map mode stays full-bleed. `streams-desktop.spec.ts`.
  **Retuned by #2926:** the Library's measure was the wrong one — it now
  holds `galleryMaxWidth` (1200), not `contentMaxWidth` (640). The
  window-wide scroll region and everything else about the treatment stands.
- **Phase 3** — `CalendarPanes` (stacked arm bit-identical via the
  passed-through grid budget; side-by-side at 840 of *available width* with
  a 380px day-detail rail), adopted by all three calendar surfaces in this
  phase rather than a fast-follow. Landscape widget-test matrix updated
  (side-by-side shapes assert beside-not-below; a stacked below-the-grid
  rect assertion proved unsound inside the reversed scroll view and was
  dropped), plus a semantics-order test. Spec extended with the rail
  assertion.
- **Phase 4** — read shells, edit panes, experience/community bottom panels,
  and `ContentMorphPanel` bodies hold the caption column; the signed-out
  web-event placeholder caps its copy. The `/go/` SSR landings turned out to
  already center at a readable measure (site CSS), so
  `web-landing-desktop.spec.ts` locks the existing behavior in instead of
  driving CSS changes; the hero-title/content seam it revealed is #2918.
  Bonus fix: a pre-existing 12px Row overflow in the gear mini-card label at
  390px-wide phones, surfaced by the new phone-geometry test.
- **Phase 5** — `docs/client/responsive.md` (context-mapped, llms.txt
  regenerated), the `web_unsupported.dart` #2157 comment corrected, and the
  follow-ups filed: **#2916** (200% text scaling), **#2917** (deferred
  desktop compositions), **#2918** (SSR title seam).

Corrections to this plan discovered during implementation are annotated
inline (see the Problem Statement's modal bullet).

## Implementation Plan

### Phase 1: Adaptive vocabulary + universal surfaces (dock, modals)

**Goal:** The measure/breakpoint vocabulary exists, and the two surfaces that
appear on *every* screen — the nav dock and bottom sheets — read as designed at
1440×810.
**Tasks:**

- [ ] Extend `app/lib/core/utils/responsive.dart` with the four named
      constants: `contentMaxWidth` (640), `sheetMaxWidth` (560),
      `dockMaxWidth` (~500), `expandedBreakpoint` (840). Do **not** add an
      `isExpanded(context)` helper yet — every planned consumer reads the
      breakpoint through `LayoutBuilder` constraints (constraint-first), so
      a `MediaQuery`-classified helper would land as dead code; add it only
      when a real consumer needs the window-classified form (via
      `MediaQuery.sizeOf`, not `.of(context).size`). Keep
      `mobileBreakpoint`/`isMobile` untouched, and delete the caller-less
      `Responsive.width`/`height` passthroughs while in the file.
- [ ] Add `ContentColumn` to `app/lib/presentation/widgets/adaptive/`
      (`Center` + `ConstrainedBox(maxWidth:)`, optional `padding`), plus the
      now-mandatory `README.md` for the directory (purpose, when to use
      `ContentColumn` vs. `AuthScreenWrapper`, the constraint-first rule).
- [ ] Default `constraints: BoxConstraints(maxWidth: Responsive.sheetMaxWidth)`
      inside `showAccessibleModal` (caller-overridable). Verify the consent
      dialog's `ModalHelpers.showStandardModal` path is covered; if it calls
      `showModalBottomSheet` directly, thread the same constraint there.
      **Use the `constraints:` parameter only — no `Align`/`Center` wrapper**
      (breaks the tap-outside-to-dismiss barrier per `glass_sheet.dart`).
- [ ] Cap the `NavDock` capsule (`home_screen.dart` `_buildNavDock`) at
      `Responsive.dockMaxWidth`, centered; confirm `nav_dock.dart`'s
      `LayoutBuilder` slot math simply receives the smaller `maxWidth` (it
      does — it is constraint-driven).
- [ ] Widget tests: a dedicated test for `ContentColumn` itself (no-op below
      the measure — child rect identical with and without the wrapper at
      390 wide; centered at the measure at 1440 wide); sheet width ≤ cap and
      horizontally centered at `Size(1440, 810)`; dock capsule ≤ cap and
      centered; all unchanged at `Size(390, 844)`. Follow the
      `home_calendar_screen_test.dart` landscape template
      (`tester.view.physicalSize`, `pixelRatio: 1`,
      `addTearDown(tester.view.reset)`).
- [ ] New `@desktop` spec **`desktop-shell.spec.ts`** covering the universal
      surfaces at 1440×810: (a) the Data Consent sheet's action row fully
      inside the viewport with margin — retiring the recorded #2908 near-miss;
      (b) an open glass sheet's bounding box ≤ `sheetMaxWidth` and
      horizontally centered; (c) the nav dock capsule ≤ its cap and centered.
      Forward `test.info().project.use.viewport` into any manual context;
      assert geometry, never tap success; no `networkidle` waits.
- [ ] **Desktop walkthrough capability + baseline renders.** Parameterize the
      walkthrough capture recipe (`e2e/lib/capture.ts` / `openScene`, which
      today pin phone emulation) so a reel can render at 1440×810 desktop
      emulation — either an env knob threaded through
      `run_walkthrough.sh` or a second `walkthrough-desktop` Playwright
      project; pick whichever keeps `capture.ts` the single source of the
      recipe. Then render **baseline desktop passes of the `requests` and
      `events` reels against today's layouts** — the "before" half of the
      evaluation every later phase re-renders against. Desktop variants are
      **local evaluation artifacts only** (`e2e/videos-walkthroughs/`,
      gitignored): do not deploy them to `website/content/walkthroughs/` —
      the walkthrough page frames clips in a phone frame, and these exist to
      evaluate layouts, not to publish.

**Validates:** The vocabulary is right-sized (four constants, one widget),
the highest-leverage single change point (`showAccessibleModal`)
behaves — every sheet in the app is fixed by one diff — and the desktop
evaluation loop (walkthrough renders + `@desktop` specs) exists before any
screen-level work begins.

### Phase 2: Stream surfaces — Home, Library, People

**Goal:** The three list-shaped dock destinations hold a readable centered
measure; the Home hero and its CTAs stop stretching to the window.
**Tasks:**

- [ ] Home (`home_tab_screen.dart`): wrap the `ListView`'s children (greeting
      `DestinationHeader`, Needs-you rows, zero-state card, pulse section) in
      `ContentColumn`. Keep the `ListView` itself full-width so the scrollbar
      stays at the window edge.
- [ ] Zero-state hero (`home_empty_states.dart`): inside the measure, the
      existing `CrossAxisAlignment.stretch` becomes correct (pills span 640,
      not 1400). Check the nudge hero's fixed 320px height still composes at
      640 wide; adjust only if the crop reads badly.
- [ ] Library (`discover_screen.dart`): center the list content at the
      measure. The horizontal shelves keep their edge-to-edge scroll region if
      that reads better — decide visually — but shelf *content* insets align
      to the measure. Map mode stays full-bleed (maps want the space).
- [ ] People (`directory_screen.dart`): same treatment for rows, scope pill,
      and sort/filter sheet content.
- [ ] `DestinationHeader`: constrained by the same `ContentColumn` as its
      siblings on each destination so titles and avatars align with content.
- [ ] Widget tests per screen at `Size(1440, 810)` + `Size(390, 844)`:
      content row width ≤ measure and centered on desktop; geometry unchanged
      on phone (assert a known row's rect equals the pre-change snapshot
      values, not just "no exception").
- [ ] New `@desktop` e2e spec (`streams-desktop.spec.ts`) covering **all
      three** stream destinations: on Home, the "Ask the crew" pill's
      bounding box width ≤ ~680 and horizontally centered, and
      clickable-and-on-screen (a11y-tree presence **and** rect-in-viewport,
      per the #2908 assertion discipline); on Library, a seeded shelf item's
      rect inside the measure band; on People, a seeded member row's rect
      inside the measure band and the `DestinationHeader` aligned with it.
- [ ] **Evaluation checkpoint:** re-render the desktop `events` and
      `requests` walkthrough variants and review `frames/` — Home scenes
      should now read as designed; item-route and landing scenes will still
      look stretched (expected until Phases 3–4).

**Validates:** The measure primitive scales across heterogeneous list screens
without per-screen forks — the pattern the remaining phases repeat.

### Phase 3: Plans two-pane (the structural showcase)

**Goal:** At expanded widths, Plans uses the space: month grid beside a
day-detail side panel, replacing today's centered-grid-over-left-anchored-strip
mismatch.
**Tasks:**

- [ ] In `home_calendar_screen.dart`, branch on the *available width* (the
      existing outer `LayoutBuilder`, threshold `expandedBreakpoint`): wide →
      `Row[ month grid (existing centered block, full height budget) |
      day-detail panel (fixed ~380–420px width, own scroll) ]`; narrow →
      today's `Column` exactly as-is. The grid's `maxHeight` budget in the
      wide arm can rise (no bottom strip to reserve for), letting cells reach
      their 96px cap — reuse `monthGridCellSide` untouched.
- [ ] Keep `CalendarDayDetail` and its `maxHeight` contract intact — the side
      panel passes its own constraints; no changes inside the detail widget
      beyond what composition requires.
- [ ] Decide (visually) whether `shared_calendar_screen.dart` and
      `workshop_library_calendar_panel.dart` — same `Column` shape, same
      shared widgets — adopt the two-pane arm in this phase or a fast-follow;
      they share the widgets either way, so the branch should live in a shared
      layout helper, not be copy-pasted three times.
- [ ] Accessibility: semantics traversal order in the `Row` arm must remain
      grid → detail (visual order = reading order); verify with the widget
      test's semantics tree. **Keyboard focus traversal too** — desktop web
      users tab through the page, and Flutter's default traversal follows
      widget-tree order, so the `Row` arm's child order is load-bearing:
      confirm Tab reaches the day cells before the detail entries and that
      focus is not trapped in either pane. Backdrop crossfade behavior
      (reduce-motion aware) is unaffected.
- [ ] Extend the `landscape viewports (#2908)` widget-test group: at
      1440×810 assert the detail panel's rect is *beside* the grid
      (`detail.left > grid.right`), both fully on-surface; at 390×844 assert
      the stacked layout is byte-identical in geometry to today.
- [ ] Extend `plans-calendar-landscape.spec.ts`: the day-detail entry
      ("Extension Ladder"-style seeded item) must be present in the a11y tree
      **and** its bounding box in the right-hand region of the viewport,
      *beside* the grid's box rather than below it. `exact: true` on role
      names (substring matches day cells). If the shared/workshop calendars
      adopt the two-pane arm in this phase, each gets the same assertion pair
      in this spec.
- [ ] **Evaluation checkpoint:** re-render the desktop walkthrough variants
      whose scenes touch Plans/calendar surfaces and review `frames/`.

**Validates:** The one real breakpoint in the plan pays for itself — the
flagship screen the issue names is genuinely better on desktop, not just
narrower — and the two-pane arm is held up by both test layers.

### Phase 4: Item routes — gear / request / experience read shells

**Goal:** Content routes read as a full-bleed hero with an intentional caption
column instead of window-wide sheets and pills.
**Tasks:**

- [ ] In the three read shells (`gear_read_shell.dart`,
      `request_read_shell.dart`, `experience_read_shell.dart` — structurally
      identical `Positioned(left:0,right:0,bottom:0) → HeroContentWash`),
      constrain the sheet to the 640 measure, bottom-center. `HeroContentWash`
      inherits the sheet's width, so the wash follows the caption column
      automatically — exactly its content-sized design intent
      (`docs/client/design.md` → Media Overlay Material).
- [ ] Same treatment for the edit panes and the `*BottomContent` legacy panels
      (Experience/Request tabs, Community) so edit mode and the community view
      don't snap back to full width.
- [ ] Morph-reveal panels (`ContentMorphPanel` children — conversation,
      location, who's-using, details): constrain the panel *content* to the
      measure over the full-bleed scrim; the morph animation (grows from the
      card's rect) needs no change since the source cards now live inside the
      measure.
- [ ] Check `ContentGradientOverlay` (frame-anchored top strip) still covers
      the top-bar controls at wide sizes — it is frame-anchored, so yes, but
      verify the back chevron/overflow positions still align with the
      constrained content or deliberately stay at frame corners (recommend:
      frame corners, like a video player's chrome).
- [ ] The `/go/` web landing screens (`web_experience_screen.dart`,
      `web_community_screen.dart`, `web_item_action_screen.dart`) get the same
      measure treatment — they are the *most* desktop-visited screens in the
      product (guests open invite links on laptops) and today have no width
      handling at all.
- [ ] Widget tests: each read shell at 1440×810 asserts sheet width ≤ measure,
      horizontally centered, workflow CTA on-surface; phone geometry
      unchanged.
- [ ] New `@desktop` e2e spec (`item-routes-desktop.spec.ts`) covering **all
      three read shells plus a morph panel**: on a seeded gear item, the "See
      the calendar" card's bounding box ≤ ~680 wide and centered; the RSVP
      pill on a seeded event and the "I'll Help" CTA on a seeded request
      likewise; then open one morph panel (e.g. the gear conversation) and
      assert its content column's rect sits inside the measure band.
- [ ] New `@desktop` e2e spec (`web-landing-desktop.spec.ts`): the signed-out
      `/go/` event, community, and item-action landings at 1440×810 — primary
      CTA inside the measure band, on-screen, and clickable (these pages have
      no auth dance, so the spec is cheap).
- [ ] **Evaluation checkpoint:** re-render both desktop walkthrough variants
      end-to-end and review `frames/` against the Phase-1 baselines — this is
      the before/after pass that judges the whole issue. The `requests` reel's
      guest scene exercises the `/go/` landing + phone-OTP handoff on desktop,
      which no phone reel ever showed.

**Validates:** The measure works over full-bleed media (the hardest visual
context), and the highest-traffic guest-facing web screens are covered.

### Phase 5: Docs, follow-ups, and scope boundary

**Goal:** The vocabulary is documented so future screens adopt it, and
deliberately-deferred work is tracked rather than implicit.
**Tasks:**

- [ ] Write `docs/client/responsive.md` (or a section in `design.md` §
      Responsive Design, replacing its aspirational bullet list): the
      constraint-first rule, the four constants, `ContentColumn`, the Plans
      two-pane precedent, and the testing template. Add the `context:`
      frontmatter block and regenerate `docs/llms.txt`
      (`npm run generate:context-map`).
- [ ] File the deferred follow-ups from Open Questions, each with the evidence
      bar that would activate it: multi-column Library/People grid; item-route
      side rail; nav rail.
- [ ] File the **200% text-scaling coverage issue** that #2908's Phase 4
      called for and that was never filed (grep of open issues 2026-08-14
      confirms) — same starvation mechanism family, and the new width-measure
      work is the natural place to prove text-scale survival.
- [ ] Correct the misleading `#2157` reference in `web_unsupported.dart`
      (carried from #2908 Phase 4, still outstanding).
- [ ] Confirm CI cost: the `desktop-chromium` project now runs ~5 spec files
      instead of 1 (see the coverage matrix) — still scoped by `@desktop`,
      still far under the #2738 artefact lessons (no `video: 'on'` anywhere
      new; desktop walkthrough renders are local-only and never run in CI).

**Validates:** The scope boundary between "adaptive now" and "desktop-native
someday" is explicit, tracked, and testable — per the project's TODO/issue
hygiene rules.

## Testing strategy (cross-phase)

**Coverage rule: every layout a phase changes lands with a `@desktop`
Playwright assertion in the same PR.** No adaptive change ships on widget
tests alone — the #2908 lesson is that only the real CanvasKit bundle at a
real landscape viewport catches this class. The full matrix:

| Adapted layout | Spec | Phase |
| --- | --- | --- |
| Glass/solid bottom sheets (via `showAccessibleModal`) | `desktop-shell.spec.ts` | 1 |
| Data Consent sheet (near-miss regression) | `desktop-shell.spec.ts` | 1 |
| Nav dock capsule | `desktop-shell.spec.ts` | 1 |
| Home stream + zero-state hero/CTAs | `streams-desktop.spec.ts` | 2 |
| Library list/shelves | `streams-desktop.spec.ts` | 2 |
| People list + `DestinationHeader` alignment | `streams-desktop.spec.ts` | 2 |
| Plans two-pane (grid ∥ day detail) | `plans-calendar-landscape.spec.ts` (extended) | 3 |
| Shared / workshop calendars (if adopted in 3) | `plans-calendar-landscape.spec.ts` | 3 |
| Gear / request / experience read shells | `item-routes-desktop.spec.ts` | 4 |
| Morph-reveal panel content | `item-routes-desktop.spec.ts` | 4 |
| `/go/` web landings (event, community, item) | `web-landing-desktop.spec.ts` | 4 |

All run under the existing `desktop-chromium` project (1440×810, `@desktop`
grep) — roughly five spec files at completion, well inside the #2738 cost
bounds since the project greps to `@desktop` only.

Carried from #2908, because both traps *will* recur:

1. **Playwright**: every `@desktop` spec that builds its own context must
   forward `test.info().project.use.viewport` or it silently runs at
   1280×720. Assert on bounding-box geometry vs. the viewport rect **and**
   a11y-tree presence — `flt-semantics` nodes stay clickable while painted
   off-screen, and either assertion alone passes in one of the two observed
   failure modes. `exact: true` on role names. No `networkidle` (long-lived
   stream). Wrap geometry assertions in `expect(...).toPass()` (Flutter
   rebuilds its semantics tree continuously).
2. **Widget tests**: the `tester.view.physicalSize` + `pixelRatio: 1` +
   `addTearDown(tester.view.reset)` idiom, with the named-viewport-matrix
   group shape from `home_calendar_screen_test.dart`. Phone-shape assertions
   are as important as desktop ones — the measure must be a provable no-op at
   390 wide.
3. **Phone regression**: full `flutter test --reporter expanded` per phase;
   the existing phone-viewport e2e projects (`mobile-chromium`, `webkit`) and
   walkthrough baselines must pass unchanged — any diff there means the
   measure was not the no-op it claims to be.

### Desktop walkthrough evaluation loop

Alongside the geometry specs, **two reels get desktop-aspect variants** as
the human-judgment channel the specs can't provide (specs prove "≤ 680px and
centered"; only watching the journey shows whether it *looks natural*):

- **`requests`** — the most desktop-relevant journey in the product: an
  appless guest opens a `/go/` link on a laptop, offers via the SSR landing,
  and registers by phone OTP. Also exercises create, sheets, the request
  route, and impact views.
- **`events`** — the broadest in-app coverage: Home, unified create, the
  experience route, chat, needs claims, wrap-up.

Mechanics (Phase 1 builds this): the capture recipe in `e2e/lib/capture.ts` /
`openScene` currently pins phone emulation; it gets a desktop mode
(1440×810) selectable per render. Same specs, same fixture packs, same
scene-by-scene `frames/` review — a baseline render against today's layouts
first, then re-renders at each phase's evaluation checkpoint. Desktop
variants stay **local artifacts** (`e2e/videos-walkthroughs/`, gitignored)
and are never deployed to `website/content/walkthroughs/`. Standard
walkthrough cautions apply: one reel at a time (single fixed port), rebuild
via `npm run build:e2e` before rendering (embedded-bundle staleness guard),
and expect a handful of desktop-conditional scene tweaks — locators are
role/label-based and viewport-agnostic, but pacing and what's-on-screen can
shift.

## Related Issues

- #2908 — the parent: correctness fix (merged as PR #2913), investigation
  sweep, and the `desktop-chromium` CI project this plan builds on. Its plan
  doc's Phase 3 (shell clamp) is **superseded** by this plan's Option B
  decision; its Phase 4 follow-ups are absorbed into Phase 5 here.
- #2157 — served the whole app on web; origin of the failure class.
- #2634 — the converged nav dock and embedded Plans tab this plan restyles.
- #2514 — the month × backdrop calendar that Phase 3 recomposes.
- #2738 — Playwright artefact-cost lessons bounding the CI additions.
- #2684 — the walkthrough pipeline the desktop evaluation variants extend.

## Audit notes

Audited with `ripls-audit` on 2026-08-14. Engaged dimensions: accessibility,
testing, performance, code_quality, tech_debt. (Not engaged: security,
api_design — no proto/RPC or auth-surface changes, layout only;
error_handling, observability — no new failure or logging paths; dependency —
no new packages; i18n — no new user-visible strings, so no ARB/l10n parity
work.)

Changes the audit made to this plan:

1. **Removed planned-but-consumerless API from Phase 1** (code-quality: dead
   code). `isExpanded(context)` had no consumer — every planned breakpoint
   read goes through `LayoutBuilder` constraints — so the phase now ships
   constants only and adds the helper when a consumer exists. The same task
   now also deletes `Responsive.width`/`height`, which are caller-less
   today.
2. **Named the dock cap** (`Responsive.dockMaxWidth`) instead of an inline
   "~500 px" (code-quality: magic values live next to their siblings).
3. **Added a dedicated `ContentColumn` widget test** to Phase 1 — the
   primitive's no-op-below-measure claim is the plan's central phone-safety
   argument and deserved its own proof, not just screen-level coverage
   (testing rubric: critical-path coverage).
4. **Added keyboard focus-traversal verification to Phase 3** — desktop web
   users tab through the page, and the two-pane `Row` makes widget-tree
   order load-bearing for focus order (accessibility rubric §5, a gap the
   plan's semantics-order task alone did not cover).
5. **Markdown lint fixes** (MD032 blank lines around task lists, MD060 table
   separator style) flagged by diagnostics on write.

Compliance checks that passed without changes: no proto changes (API/models
separation not engaged); no new dependencies; no TODOs without tracking
issues (the plan *files* issues instead — Phase 5); `ContentColumn` composes
existing framework primitives and introduces no raw `GestureDetector` /
`showModalBottomSheet` (the modal change rides `showAccessibleModal`, the
a11y-lint-approved entry point, and uses the `constraints:` parameter rather
than the barrier-breaking `Align` wrapper the glass-sheet docs warn about);
the new `widgets/adaptive/` README satisfies the multi-file-directory rule;
all touched Dart files stay under the 1,000-line gate (largest:
`discover_screen.dart` at 840 — wrapping adds single-digit lines); the
desktop walkthrough variants respect the walkthrough pipeline's constraints
(one reel at a time, `build:e2e` staleness guard, local-only artifacts, no
consent implications — both chosen packs are fully fabricated); `MediaQuery`
guidance follows the performance rubric (`sizeOf` over `.of(context).size`,
`LayoutBuilder` over window classification); and the plan contradicts no
documented decision — it supersedes #2908's Phase 3 explicitly rather than
silently.

Docs consulted: `client/architecture.md` ⚓, `client/conventions.md` ⚓,
`users/priorities.md` ⚓, `users/personas.md` ⚓, `client/design.md`,
`client/content.md`, `client/inbox.md`, `client/calendar.md`,
`client/modals.md`, `client/accessibility.md`, `walkthroughs.md`,
`issues/2908-web-aspect-ratios.md`, and the accessibility / testing /
performance / code-quality / tech-debt review prompts' "Areas to Review"
sections.
