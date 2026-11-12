# Issues #2926 + #2918: desktop width seams — the Library's dead left gutter and the SSR hero-title seam

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2926 ·
https://github.com/Type2Fun-Foundation/ripls/issues/2918
**Filed:** 2026-08-15 (#2926) · 2026-08-14 (#2918) | **Last activity:** 2026-08-15
**Labels:** `user-feedback` (#2926) · `enhancement` (#2918) | **Assignees:** unassigned
**Scope:** one PR covering both (Bryan's instruction)
**Context docs:** `docs/llms.txt` (map) · `docs/client/responsive.md` ⚓area ·
`docs/client/architecture.md` ⚓ · `docs/client/conventions.md` ⚓ ·
`docs/client/testing.md` · `docs/client/testing/semantics_identifiers.md` ·
`docs/server/architecture.md` ⚓ · `docs/invitations.md` ·
`docs/walkthroughs.md` · `docs/issues/2912-desktop-adaptive.md` ·
`docs/reviews/accessibility_review_prompt.md` ·
`docs/reviews/testing_review_prompt.md` ·
`docs/reviews/code_quality_review_prompt.md`

## Problem Statement

Both issues are the same defect class, on the two halves of the product's
desktop surface: **#2912 picked one measure — the 640px *reading* measure —
and applied it to content that isn't reading-shaped.** The result is a page
whose halves disagree about where the column is.

**#2926 (Flutter app, Library/"Near me").** The Library's category shelves
inset their content to the 640 reading column's left edge while the shelf's
scroll region stays window-wide. On a 1440-wide window that is a **420px empty
left gutter** with tiles running off the right edge — 29% of the window is
void. Bryan's second sentence is the tell: *"If you scroll items can move left
into the empty space."* That gutter is not a margin; it is scroll padding, so
content visibly invades it. The layout announces that it is not really using
the width it has.

**#2918 (SSR `/go/` landings).** The hero title block is pinned to the window's
left edge (`position:absolute; left:0; right:0; padding: var(--space-5)`) while
the meta pill, description, host row and CTA bar all center at a 560px measure.
At 1440 the h1 starts at x≈24 and the body at x≈456 — a 432px seam down the
middle of the page. Cosmetic, not functional; the CTAs are already centered and
bounded.

Neither is a regression in the sense of "worked before #2912" — both are
composition choices that #2912 made and that real desktop viewing has now
falsified. #2912's own Open Questions predicted the wrong outcome for the
Library: *"(Library's horizontal shelves already use width naturally once
centered.)"* — #2926 is the evidence that they do not.

## Context from Comments

**Neither issue has comments.** #2926 arrived through the in-app feedback
pipeline (`docs/user_feedback.md`) with a PROD screenshot and device context
(Web, MacIntel, Chrome 151, app 0.30.0) and no discussion; #2918 was filed by
the #2912 implementation as a deliberate scope split, and its body already
names the root cause and the files. So the "context" that matters lives in
`docs/issues/2912-desktop-adaptive.md` rather than in comment threads:

- The **constraint-first, breakpoint-rare** house rule: derive layout from the
  constraints a widget is given; width caps must be no-ops below the cap so
  phone layouts are untouched by construction.
- Bryan rejected the letterbox/shell-clamp direction during #2912 —
  *"actually make it natural / truly adaptive"*. #2926 is the same instinct
  applied to a surface that #2912 over-clamped.
- #2917 deferred a **multi-column Library/People** composition behind an
  evidence bar: *"Activate if the constrained column reads as wasteful once
  desktop usage exists."* #2926 is exactly that evidence — but it asks for
  *more width*, not for a grid redesign.
- #2912 Phase 4 found the `/go/` landings "already centered" and locked that
  in with `web-landing-desktop.spec.ts` rather than changing CSS; the seam it
  noticed on the way past became #2918.

## UI Investigation

I did **not** run the `ripls-ui-investigate` skill. The skill's job is to
reproduce the report and bisect it to a layer; for both issues the
reproduction artifacts already exist on disk and the layer is a single call
site, verified by arithmetic rather than by guessing:

**#2926 — layer: client render.** One call site,
`app/lib/presentation/screens/discover/discover_screen.dart:747-750`:

```dart
final shelfInset = math.max(
  20,
  (MediaQuery.sizeOf(context).width - Responsive.contentMaxWidth) / 2 + 20,
).toDouble();
```

applied as the shelf title's `Padding` and the horizontal `ListView`'s leading
`padding` (`:820`, `:837`). Reconciling that formula against Bryan's PROD
screenshot: the screenshot's content edge sits at 385/1632 px, which solves to
a **1136 CSS-px window with a 268px inset** — exactly
`(1136 − 640)/2 + 20`. The screenshot *is* the reproduction, and the formula
predicts it to the pixel. There is no server, data, or state component: the
same gear renders correctly on a phone.

**#2918 — layer: static CSS.** Already rendered, on disk:
`e2e/videos-walkthroughs/requests-desktop/frames/frame_028.png` (1440×810 @2×)
shows "WANTED / A lawn mower for the weekend" flush left while "1 item · 1
still open", the description, the host row and "Offer to help" all center at
560. The rules are `website/content/css/event.css` `.event-hero-inner` (:56-65,
full-bleed) vs `.event-main` (:85-89) and `.event-cta-bar-inner` (:250-255,
both `max-width: 560px; margin: 0 auto`).

What a hermetic run would add is a *judgment* artifact — how much width the
Library should take — and that is Phase 4's desktop walkthrough render, where
it is cheaper and more honest (a real reel, not a probe screenshot).

## What the code actually does today

**App (#2926).** `Responsive` (`app/lib/core/utils/responsive.dart`) publishes
four measures: `contentMaxWidth` 640, `sheetMaxWidth` 560, `dockMaxWidth` 500,
`expandedBreakpoint` 840. `ContentColumn` = `Center` + `ConstrainedBox`, with a
`maxWidth` parameter that **no caller currently overrides**. The Library
(`discover_screen.dart`) uses it three times — map-mode header (:461),
shelves-mode header (:782), empty-state header (:758) — all at the default 640,
and derives `shelfInset` from the same constant. Shelf tiles are 150×144 with
a 10px gap (`library_shelf_tile.dart:39-40`). There is **no widget test** for
the Library's desktop geometry; the only coverage is
`e2e/tests/workflows/streams-desktop.spec.ts:135-143`, which asserts the shelf
tile's left edge is at or right of the 640 band — an assertion that this change
must update, not merely re-run.

**SSR (#2918).** All four landings (`community.html`, `event.html`,
`gear.html`, `request.html`) share the same three classes, so one CSS file
fixes all four; `invite.html` uses `deeplink.css` and is already centered
(`.deeplink-card`, max-width 600). `website/content/css` is embedded into the
Go binary (`website/embed.go`, served via `fs.Sub` in
`server/services/web/service.go:160-166`), so this ships with the server, not
with a site deploy. The templates themselves need no edit.

## Potential Solutions

### Option A: Give the Library its own (wider) gallery measure — recommended

- **Approach:** Add one constant — `Responsive.galleryMaxWidth = 1200` — for
  *gallery* surfaces (fixed-size tiles), distinct from the 640 *reading*
  measure for text streams. The Library's header (all three call sites) passes
  it to `ContentColumn(maxWidth:)`; the shelf inset derives from the same
  constant. Extract the inset arithmetic into a tested static on `Responsive`
  so the number and the formula live in one place. The shelf's scroll region
  stays window-wide (the documented overscroll-clip trap is untouched). For
  #2918, align `.event-hero-inner` to the SSR measure in CSS.
  At 1440 the left gutter goes **420 → 140**; below ~1240 the shelves are
  edge-to-edge at the plain 20px and nothing changes; phones are untouched by
  construction.
- **Pros:** Names the actual distinction (reading measure vs gallery measure)
  instead of stretching one number over two jobs — the same move #2912 made
  for sheets and the dock. Header and shelves agree, which is the symmetry
  Bryan asked for. Small, reversible diff; the value is tunable in one place.
  Ultrawide monitors stay sane.
- **Cons:** The Library header becomes wider than Home's and People's, so the
  header shifts horizontally when switching tabs — and `DestinationHeader`'s
  doc comment currently promises "identical geometry on every tab" (that
  comment needs updating, or the promise needs narrowing to *internal*
  geometry). Introduces a fifth measure to the vocabulary.
- **Effort:** Small (app ~40 lines + one CSS rule; the test updates are the
  bulk).

### Option B: Drop the Library's cap entirely — fixed gutter, full width

- **Approach:** Remove the measure from the Library altogether: header and
  shelves both take a flat 20–32px gutter at every width, matching Map mode
  (which is deliberately full-bleed). Same CSS fix for #2918.
- **Pros:** Smallest possible diff (delete the `shelfInset` arithmetic and the
  header's `ContentColumn`). Perfectly symmetric at every width. Maximally
  "uses the width", and makes List mode agree with its Map-mode sibling.
- **Cons:** On a 27" monitor a shelf row is ~2500px of 150px tiles and the
  header's title and toggle sit in opposite corners — the "stretched phone"
  reading #2912 was created to kill, reintroduced on one surface. Loses the
  ability to tune later without re-adding a constant.
- **Effort:** Small.

### Option C: Multi-column Library grid (activate #2917)

- **Approach:** Replace horizontal shelves with a responsive tile grid at
  expanded widths — the composition #2917 deferred.
- **Pros:** Genuinely uses the space; removes horizontal scrolling on desktop
  entirely, which is the real ergonomic complaint behind "items move into the
  empty space".
- **Cons:** A redesign of the Library's information architecture (category
  shelves are how "biggest shelf first" is expressed), a structural breakpoint
  where #2912 deliberately has one, and far more than #2926 asked for. Bryan
  asked for width, not a new layout.
- **Effort:** Large.

### #2918 sub-options (independent of A/B/C)

- **A′ (recommended): one padding rule.** Replace `.event-hero-inner`'s
  horizontal padding with
  `padding-inline: max(var(--space-5), calc((100% - var(--ssr-measure)) / 2 + var(--space-4)))`,
  hoisting the 560 into a `--ssr-measure` custom property that `.event-main`
  and `.event-cta-bar-inner` also consume. At 1440 that resolves to 456px —
  pixel-identical to `.event-main`'s text edge. **Below 576px the `max()`
  picks `--space-5` and the phone rendering is byte-identical**, so no reel,
  screenshot, or phone landing changes.
  Two things the rule depends on, both worth a comment in the CSS because a
  later edit could silently break them: (1) `.event-hero-inner` is
  absolutely positioned, so `100%` resolves against its containing block —
  `.event-hero`'s padding box, which is window-wide because `.event-hero` is
  the `position: relative` full-bleed banner; constraining `.event-hero`
  later would change what this rule means. (2) The resulting hero content
  width is `--ssr-measure − 2 × --space-4` = 528px at every width above the
  measure, which is exactly `.event-main`'s content box — that equality *is*
  the invariant, and it is what the Phase 1 spec asserts.
- **B′: `max-width` + `margin-inline: auto`.** Equivalent result via the
  absolutely-positioned-with-auto-margins centering rule, but needs
  `max-width: 576px` (560 + the 8px-per-side padding delta) to land on the
  same text edge — a magic number that invites drift.
- **C′: template change** — wrap the hero text in an inner measure div. More
  markup for no gain; the issue is purely a CSS one.

## Recommendation

**Option A + A′, in one PR.**

The through-line that justifies pairing them: *the measure a surface holds must
match the shape of its content.* 640 is a text measure — right for Home, People,
captions, sheets; wrong for a rail of 150px tiles. 560 is the SSR body measure —
and the hero title is body content that simply never opted in. Both fixes are
"apply the right measure", which is why they read as one change and one docs
update rather than two unrelated patches.

A over B because the cap is what makes the treatment *designed* rather than
*absent* — B's ultrawide behaviour is the exact failure #2912 existed to fix,
and re-adding a constant later costs more than parameterising now. A over C
because C is a redesign #2926 did not ask for and #2917 gates behind a design
pass; this PR should comment on #2917 with #2926 as partial evidence, not
close it.

A′ over B′ because `max()` makes the phone no-op structural instead of
arithmetic — the same "no-op below the cap by construction" property that
`ContentColumn` gets from `Center` + `ConstrainedBox`.

The one thing I would not do is ship the app fix without the walkthrough render
in Phase 4. 1200 is a defensible first number, not a proven one, and the render
is how #2912 settled every other measure.

## Open Questions

- **Is 1200 the right gallery measure?** — **DECIDED (2026-08-15): 1200.**
  It gives 140px gutters at 1440 (a normal page margin, not a void), ~7.5
  tiles per row, and sits at Material's extra-large window boundary; the
  marketing site's `.container` is 1100, so it is in family. Judge it in
  Phase 4's desktop reel and tune the one constant if it reads wrong.
- **Should the shelf's scroll viewport also be bounded to the measure, so
  tiles clip at 140 instead of at the window edge?** — **DECIDED
  (2026-08-15): no — keep the window-wide scroll region.** Bryan, on what his
  second sentence meant: *"you can horizontally scroll things so they leave
  the screen on the left, so that part of the screen is usable — it's just
  over-justified to start."* The scroll-under-the-inset behaviour is **not the
  bug**; it is the affordance working. The bug is only the resting inset. So
  the fix is the measure, and the `ListView` keeps its window-wide RenderBox —
  which also keeps the documented overscroll-clip trap
  (`discover_screen.dart:783-791`) untouched.
- **Should the Library header widen with the shelves, accepting a
  horizontal jump when switching tabs?** — Recommendation: yes. The alternative
  (header at 640, shelves at 1200) reproduces the same disagreement #2926
  reports, one layer down. Update `DestinationHeader`'s doc comment to promise
  identical *internal* geometry rather than identical width.
- **Do Home and People want the gallery measure too?** — Recommendation: no.
  Their content is text rows and cards; 640 is correct for them, and the
  divergence is the point of naming a second measure.
- **Does the SSR h1 keep `max-width: 18ch` inside the new measure?** —
  Recommendation: yes, unchanged. It binds on phones today and is a deliberate
  ragged-title device; widening it is a typographic decision, not a bug fix,
  and it would change the phone rendering that A′ otherwise preserves exactly.
- **Should #2917 be closed?** — Recommendation: no. Comment on it that #2926
  supplied the "constrained column reads as wasteful" evidence for the Library
  and that Option A discharges it *without* the multi-column grid; leave the
  grid, the item-route side rail, and the nav rail open.
- **Does a 1200-wide header hurt low-vision users?** — Recommendation: accept
  it, but check it rather than assume. Widening the Library header puts ~1000px
  between the "Near me" title and the avatar/search/toggle cluster, which is a
  real cost under screen magnification (the two ends of one header stop being
  co-visible) and interacts with #2916's untested 200% text scale. Semantics
  traversal order is unaffected — `ContentColumn` adds no reordering — so the
  cost is visual, not structural. Phase 2 should eyeball the header at 200%
  text scale; if it breaks, that is #2916's bug surfacing early, not a reason
  to abandon the measure.
- **Where does `--ssr-measure` live — `event.css` or `tokens.css`?** —
  Recommendation: `event.css`. `tokens.css` does claim "the type/spacing/radius
  scales" and the aliases consumed by `event.css`, so a reviewer will ask; the
  answer is that all three consumers are in `event.css` and the number is a
  property of that page family, not a site-wide scale. Never hand-edit
  `website/content/css/gen/tokens.gen.css` — it is generated by
  `npm run generate:design-tokens` and CI fails on drift (#2441).

## Status

**All five phases implemented** on branch `ui_fixes` (2026-08-15). Both
`@desktop` specs pass against the hermetic stack at 1440×810, and both fixes
were eyeballed on real renders (see Phase 4 below). Deltas from the plan, all
discovered while implementing:

- **`ContentColumn.maxWidth` was already tested** —
  `content_column_test.dart:86` has a "honors a custom maxWidth" case. The
  plan's claim that this PR is its first caller was wrong about the *test*,
  right about the caller. No duplicate test added; the new coverage went to
  `responsive_test.dart` where the new logic is.
- **The #2918 spec asserts the h1 against `.event-main`, not the CTA.** A
  throwaway Playwright probe over a static copy of the markup measured the
  CTA pill 16px left of the body text at every width — correct by design (the
  button spans the column; text is inset by `.event-main`'s padding), so the
  originally-planned h1-vs-CTA assertion would have encoded a false invariant.
  Probe results, hero-title vs body-text left edge: 1440 → 456/456, 1024 →
  248/248, 768 → 120/120, 600 → 36/36, **390 → 24/16 (unchanged)**.
- **The Library header title cannot be found by its visible text.**
  `DestinationHeader`'s `titleSemanticsLabel` goes to `Tappable`, whose
  `semanticsLabel` *replaces* the child's text in the accessible name — so the
  node reads "Change the distance anchor", not "Near me ▾". The spec matches
  on the a11y label.
- **`DestinationHeader`'s 20px padding equals `Responsive.baseInset`**, which
  is why the header title and the first shelf tile land on exactly the same
  x and the new e2e assertion can be an equality rather than a band.
- **The first draft of the Library e2e assertions was vacuous.** Both the
  band check (`x >= bandLeft`, `rightBounded: false`) and the
  title-vs-tile equality pass against the *buggy* layout: #2926's inset was
  420px, which is both "right of" the gallery band's 120px edge and equal to
  where the 640-measure header put its title. #2926 is an inset that is too
  *large*, and a lower bound cannot see that. Replaced with an equality
  against `bandLeft(gallery) + baseInset` (140 at 1440, 20 on a phone — the
  invariant holds at every viewport). The old spec's `x >= 398` assertion
  passing in CI is the evidence that the old tile sat at 420, so the
  equality would reject it.
- **The Phase 6 seam assertion was vacuous on its first cut too — same trap,
  new disguise.** `detail.left - grid.right <= 44` passed against the buggy
  layout, because under `Expanded` the `CalendarMonthGrid` *box* spanned the
  whole pane while the grid *painted* centred inside it. Rewritten against the
  union of the `CalendarDayCell` rects (real paint geometry), it reports 201px
  at 1440×810 and 441px at 1920×1080 on the old layout. Lesson for this file:
  after writing a geometry assertion, revert the fix and watch it fail — twice
  now the green test proved nothing.
- **Phase 4 done with a throwaway probe, not the walkthrough reel.** Two
  ~15s `@desktop` probe specs against the already-built stack produced the
  two artifacts the reel would have — the Library at 1440 (header, shelf
  title and first tile all at x=140; the right cluster on the band's right
  edge; tiles bleeding right) and the real `/go/` request landing at 1440
  (hero title at 456, aligned with the description, host row and CTA, over
  an unchanged full-bleed photo). The reel would have cost ~15 minutes and
  destroyed the untracked "before" frames; the probes cost neither. The
  playwright-recorded videos are useless for this — the config forces a
  780×1688 phone video size regardless of viewport.

## Phase 6: the calendar's stranded seam (added 2026-08-15)

Found by Bryan while inspecting this PR's build on the local stack, and folded
in at his direction: *"the second column has too much space between it and the
calendar, and the event names are cut off on the right."*

Same defect family, third instance — and the mirror image of #2926. Where the
Library held a measure that was too *narrow*, `CalendarPanes` gave the month
grid a pane it was incapable of *filling*: `monthGridCellSide` caps cells at
`maxCellSide` (96), so the grid tops out at `monthGridWidth(96)` = 702px no
matter how wide the pane, and the `Expanded` + `Center` split the surplus
evenly — **201px stranded in the seam at 1440×810, 441px at 1920×1080** —
while the rail that needed width sat at 380 (336 content, ~276 for a title
after the 60px time column, `maxLines: 2` then ellipsis). Both of Bryan's
symptoms, one cause.

**Shipped:** `monthGridRows(month)` and `monthGridNaturalWidth(...)` in
`month_grid_metrics.dart` (shared so the pane predicts exactly what the grid
will do; `CalendarMonthGrid` asserts the row counts agree). `CalendarPanes`
takes `gridRows`, sizes the grid pane to the grid's natural width, clamps the
rail to `[380, 560]` of the surplus, and centres the pair so the leftover
lands in the outer margins. All three calendar surfaces get it.

**Rail floor matters:** a flat 560 would starve the grid at the 840
breakpoint (cells collapse to ~30px). The floor keeps the breakpoint
bit-identical to before; the rail only reaches 560 once there is surplus to
give it. Measured at 1440: rail 560, seam 40 (18 pane inset + 22 scroll
padding), outer margins ~78 each.

**Tests:** the `'landscape viewports (#2908)'` matrix gains seam, rail-width
and centring assertions, plus a breakpoint test pinning the floor. All four
were verified to fail against the reverted layout — the first cut of the seam
assertion passed on the bug (see Status).

## Implementation Plan

### Phase 1: SSR hero-title seam (#2918)

**Goal:** All four `/go/` landings align the hero title with the body measure
at desktop widths, with phone rendering unchanged.
**Tasks:**

- [ ] In `website/content/css/event.css`, add `--ssr-measure: 560px` and route
      `.event-main` and `.event-cta-bar-inner` through it.
- [ ] Change `.event-hero-inner`'s `padding` to the `max()`/`calc()` form, with
      a comment stating the invariant (its text edge equals `.event-main`'s)
      and why `max()` keeps phones a no-op.
- [ ] Extend `e2e/tests/workflows/web-landing-desktop.spec.ts`: for each of the
      four landings assert the `<h1>`'s left edge is within a few px of the
      primary CTA's left edge. Keep the existing centered-and-bounded
      assertions.
- [ ] Verify no Go test in `server/services/web` asserts hero geometry (it
      should not — they cover strings and status codes).
- [ ] Note in the PR description that this CSS ships **inside the Go binary**
      (`website/embed.go` → `fs.Sub` in `server/services/web/service.go:160`),
      so it reaches prod on a **server deploy**, not a website deploy. `/go/`
      is Go-served, so the mimir marketing site's copy of `website/content` is
      not a second surface to update.

**Validates:** The whole class of "hero disagrees with body" is gone on the
guest-facing pages, and the fix is locked by a spec that would have caught it.
This phase is independently shippable and touches no Dart.

### Phase 2: The gallery measure (#2926)

**Goal:** `Responsive` distinguishes reading from gallery surfaces, and the
Library consumes it.
**Tasks:**

- [ ] Add `Responsive.galleryMaxWidth = 1200` with a doc comment stating what
      makes a surface a gallery (fixed-size tiles, not prose).
- [ ] Add a pure static for the measure-aligned inset (window width, measure,
      base inset → inset) so the arithmetic is testable without pumping a
      widget, with the base inset as a named default rather than the `20`
      currently repeated across the formula and `_buildShelf`'s parameter.
      New test file `app/test/core/utils/responsive_test.dart` (mirrors
      `app/lib/core/utils/responsive.dart`): below the measure it returns the
      base; above, the centered gutter plus the base.
- [ ] `discover_screen.dart`: all three `ContentColumn`s take
      `maxWidth: Responsive.galleryMaxWidth`; `shelfInset` calls the new static.
      Update the two explanatory comment blocks (`:741-750`, `:809-812`) — they
      currently name the *reading* column.
- [ ] Update `DestinationHeader`'s class doc comment (identical *internal*
      geometry, not identical width).
- [ ] Add a `ContentColumn` widget test for a non-default `maxWidth` at
      `Size(1440, 810)` and unchanged geometry at `Size(390, 844)`, following
      `content_column_test.dart`'s bare-vs-wrapped model — this is the first
      caller to override `maxWidth`, so the parameter is currently untested.
- [ ] Eyeball the Library header at 200% text scale on a 1440 window (see the
      a11y open question). Do not fix #2916 here; note what breaks.

**Validates:** The measure vocabulary carries the distinction, and the Library
picks it up, with the arithmetic covered by a test that does not need the
screen's provider graph.

**Deliberate coverage boundary:** `DiscoverScreen` itself gets no widget test.
It sits on the map, search, communities and home providers, and standing that
graph up to assert two x-coordinates would be a large, brittle fixture for a
layout constant. The split instead is: the arithmetic in a pure unit test, the
primitive in `content_column_test.dart`, and the composed result in Phase 3's
`@desktop` spec — the same three-layer split `docs/client/responsive.md`'s
testing template prescribes. Call this out in the PR so the missing screen test
reads as a decision rather than an omission.

### Phase 3: e2e realignment

**Goal:** The desktop specs assert the new invariant rather than the old
number.
**Tasks:**

- [ ] `streams-desktop.spec.ts`: the Library assertion currently uses the 640
      band and will fail. Parameterise the band per surface (Home/People at
      640, Library at 1200) and mirror the constant with a comment pointing at
      `responsive.dart`, as the file already does.
- [ ] Add the assertion the issue actually wants: the Library header's title
      and the first shelf tile share a left edge. Confirm the "Near me" title
      node keeps its own paint rect before relying on it — the merged-row
      sliver-width trap documented in the People section of that spec, and in
      `docs/client/testing/semantics_identifiers.md`, applies to any node whose
      content merges into one semantics button.
- [ ] **`npm run build:e2e` before running anything.** Phase 2 changed Dart,
      and the specs drive the real Flutter Web bundle — the stale-bundle trap
      (`docs/client/testing.md` → "three traps") otherwise validates the old
      layout and passes. Then `npm run lint:e2e:types` and the `@desktop`
      project.

**Validates:** The regression that produced #2926 now fails a test, and the
alignment invariant is stated once in machine-checkable form.

### Phase 4: Judge it on a real desktop render

**Goal:** Confirm 1200 reads as intentional before the PR goes up.
**Tasks:**

- [ ] `WALKTHROUGH_VIEWPORT=desktop e2e/scripts/run_walkthrough.sh requests` —
      it exercises both fixes in one reel (the `/go/` landing *and* the app).
      **Copy `e2e/videos-walkthroughs/requests-desktop/frames/` somewhere
      outside the repo first.** The export `rm -rf`s its output directory, and
      `e2e/.gitignore:5` ignores `videos-walkthroughs/` — those frames are
      untracked local state, so once overwritten the "before" evidence this
      plan cites (`frame_028.png`) is unrecoverable from git.
- [ ] Read the frames at the Library and the landing; tune `galleryMaxWidth`
      if the gutter reads wrong. Upload before/after stills to the PR by hand
      (they cannot be referenced by repo path).

**Validates:** A human-judgeable artifact for a human-judgement change, and
the same evaluation loop #2912 used to settle its measures.

### Phase 5: Docs and follow-ups

**Goal:** The written system matches the shipped one.
**Tasks:**

- [ ] `docs/client/responsive.md`: add `galleryMaxWidth` to the vocabulary
      table, rewrite the Library bullet under "The pieces" (it currently
      asserts the reading-measure inset as the design), replace the "SSR `/go/`
      landings — already centered" bullet with what the CSS now does, and
      advance the `freshness:` stamp. If the `context:` description changes,
      run `npm run generate:context-map`.
- [ ] `app/lib/presentation/widgets/adaptive/README.md`: its "Key files" entry
      calls `ContentColumn` "the reading-measure primitive: centers its child
      at `Responsive.contentMaxWidth` (640)" — after this change it is the
      measure primitive with two measures. The mandatory-README rule
      (CLAUDE.md → Style Guidelines) makes this part of the change, not a
      nicety.
- [ ] `docs/issues/2912-desktop-adaptive.md`: annotate the Phase 2 status
      bullet and the "(Library's horizontal shelves already use width
      naturally once centered)" Open Question with the #2926 correction —
      the plan's stated assumption was wrong and the record should say so.
- [ ] Comment on #2917 with the partial-evidence note; do not close it.
- [ ] `npm run lint` (context-map, `lint:dart:size` — `discover_screen.dart`
      is at 860 of its 1,000-line budget, `lint:dart:a11y`, `lint:e2e:types`)
      and the Flutter suite with `--reporter expanded`.

**Validates:** The next person reading `responsive.md` learns the reading-vs-
gallery distinction instead of rediscovering #2926.

## Related Issues

- **#2912** — shipped the measure system that both defects come from; its plan
  doc is the primary context and needs two corrections (Phase 5).
- **#2917** — deferred multi-column Library/People behind an evidence bar that
  #2926 partially meets; this PR discharges the Library half with width rather
  than with a grid.
- **#2916** — 200% text scaling, still uncovered; a wider gallery measure does
  not help or hurt it (shelf tiles are fixed-size).
- **#2908** — the correctness layer beneath all of this; nothing here may
  become load-bearing for fit.

## Audit notes

Audited with `/ripls-audit` after drafting. Scope: **client** (`app/lib/**`) +
an embedded static asset served by the server + e2e + docs. Engaged
dimensions: `accessibility`, `testing`, `code_quality`, `tech_debt`. Not
engaged: `api_design` (no proto change), `security`, `error_handling`,
`observability`, `dependency`, `performance` (the new code is a pure static;
`MediaQuery.sizeOf` is already the rebuild-cheap accessor the responsive doc
mandates).

Eight fixes applied to the draft:

1. **Missing README update** — `widgets/adaptive/README.md` names
   `ContentColumn` "the reading-measure primitive… (640)". The change makes it
   two-measure; CLAUDE.md's mandatory-README rule puts that in scope. Added to
   Phase 5.
2. **Stale-bundle trap** — Phase 3 runs Playwright against the Flutter Web
   bundle *after* Phase 2 changed Dart. `npm run build:e2e` was only in
   Phase 4; without it the specs would validate the old layout and pass.
   Moved into Phase 3.
3. **The cited evidence is gitignored** — `e2e/.gitignore:5` ignores
   `videos-walkthroughs/`, so `frame_028.png` is untracked and the Phase 4
   export's `rm -rf` destroys it irrecoverably. Strengthened the warning and
   corrected "attach stills" (no repo path to reference).
4. **Design-token discipline (#2441)** — `tokens.css` claims the spacing/type
   scales and the `event.css` aliases, so `--ssr-measure`'s placement needs a
   stated reason. Added it as an open question with the answer, plus the hard
   guard against hand-editing generated `gen/tokens.gen.css`.
5. **Unstated CSS dependency** — the `100%` in the `padding-inline` rule
   resolves against `.event-hero`'s window-wide padding box; that and the
   derived 528px content-width equality are the load-bearing facts, now
   written down as the invariant the Phase 1 spec asserts.
6. **Accessibility dimension was unaddressed** — a 1200-wide header separates
   the title from its controls by ~1000px (screen-magnifier cost) and meets
   #2916's untested 200% text scale. Added an open question, a Phase 2 check,
   and `lint:dart:a11y` to Phase 5. Semantics traversal is unaffected.
7. **Deployment shape** — the #2918 fix is CSS but ships in the Go binary via
   `website/embed.go`, so it needs a server deploy, not a website deploy.
   Added to Phase 1.
8. **Coverage boundary made explicit** — `DiscoverScreen` gets no widget test
   (heavy provider graph); the three-layer split that replaces it is now
   stated so a reviewer reads it as a decision. Also folded the repeated
   magic `20` into a named default.

Docs consulted: `docs/llms.txt`; anchors `client/architecture.md`,
`client/conventions.md`, `testing/architecture.md`; review rubrics
`accessibility_review_prompt.md`, `testing_review_prompt.md`,
`code_quality_review_prompt.md`, `tech_debt_review_prompt.md`; leaf docs
`client/responsive.md`, `client/testing.md`,
`client/testing/semantics_identifiers.md`, `client/design.md`,
`walkthroughs.md`, `invitations.md`, `user_feedback.md`; plus
`docs/issues/2912-desktop-adaptive.md`, `CLAUDE.md`,
`app/lib/presentation/widgets/adaptive/README.md`,
`website/content/css/tokens.css` and `website/embed.go` headers.
`proto_conventions.md` was skipped deliberately — no proto surface is touched.
