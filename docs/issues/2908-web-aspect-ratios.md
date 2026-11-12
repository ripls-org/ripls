# Issue #2908: UI is unusable in standard web aspect ratio

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2908
**Filed:** 2026-08-13 | **Last activity:** 2026-08-13
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** `docs/client/architecture.md` ⚓, `docs/client/conventions.md` ⚓,
`docs/client/calendar.md`, `docs/client/inbox.md`, `docs/client/testing.md`,
`docs/client/testing/semantics_identifiers.md`, `docs/client/accessibility.md`,
`docs/client/design.md`, `docs/reviews/accessibility_review_prompt.md`,
`docs/reviews/testing_review_prompt.md`, `docs/reviews/performance_review_prompt.md`

## Problem Statement

On Flutter Web the app is authored for one viewport shape — a tall phone
portrait — and degrades badly in every landscape browser window. The reporter's
screenshot shows the Plans (calendar) tab with the month's last row clipped
behind the bottom dock and the day-detail panel absent entirely.

The investigation below (Playwright, as the issue suggests) confirms the report
and sharpens it considerably. **The failure condition is an aspect-ratio
threshold, exactly as the issue title says**: the day-detail panel dies once
**width ≳ 1.1 × height**, which is every landscape window there is (16:9 = 1.78,
16:10 = 1.6, 4:3 = 1.33). The app only works in portrait-ish windows, and no
desktop browser is portrait by default.

The *mechanism* behind that threshold is width-driven, which is what makes the
fix a sizing change rather than a scrolling one: `CalendarMonthGrid` renders its
7-column month with `GridView.count`'s default `childAspectRatio: 1.0`, so every
day cell is a **square of side ≈ width/7** and the grid's height grows linearly
with the window's *width*. Its parent is a plain non-scrolling `Column`, so once
header + grid exceeds the window height, the sibling `Expanded` holding the day
detail is starved to zero and the month's last rows clip away with **no scroll
affordance to recover them**. Because the chrome around the grid is nearly
constant, "grid height ≈ 0.857 × width" turns into the near-scale-invariant
ratio threshold above.

Two scope corrections worth stating up front, because they change what the fix
has to cover:

1. **The calendar is the only screen that loses content permanently.** Home,
   Library and the gear/request/experience routes are scrollable, so at
   landscape sizes they are stretched and awkward (full-bleed heroes, very long
   line lengths, controls stranded at the far left) but nothing becomes
   unreachable. The calendar is uniquely broken because its overflowing child
   sits in a `Column` with no scrollable ancestor.
2. **This is also an accessibility defect, not only a visual one.** At
   1440×810 the day-detail content is not merely painted off-screen — it is
   absent from the accessibility tree altogether, so a screen-reader user at a
   normal laptop window has no route to the day's events either.

The reason this shipped is a coverage gap: **every** automated surface renders
at phone portrait. Playwright's `mobile-chromium` and `webkit` projects both use
390×844 / iPhone 14, and the widget tests explicitly force phone-shaped
surfaces. No test in the repo has ever instantiated a landscape viewport.

## Context from Comments

**The issue has no comments** — it is a raw in-app feedback submission
(`user-feedback` label, auto-filed from the app's feedback sheet with device
context and screenshot attached). There is no maintainer discussion to draw
direction from, so the product call in Open Questions below is genuinely open
rather than merely unrecorded.

The body does carry one explicit steer from the reporter: *"we can use playwright
to test alternative aspect ratios and see which actions and workflows are broken
or work."* The investigation below follows that instruction, and the plan
promotes it to a permanent CI gate rather than a one-off sweep.

## UI Investigation

**Investigated:** 2026-08-13 on `web_aspects`@845d1d649 | **Environment:** hermetic e2e stack (`ripls-ui-investigate`)
**Verdict:** **client-render.** `CalendarMonthGrid` derives cell height from
viewport *width* (square cells at width/7) inside a non-scrolling `Column`;
past ~1.1 : 1 width-to-height the grid's height starves the sibling `Expanded`
day-detail panel to zero and clips the month's final rows, with no scroll path
to recover either. Secondary, lower-severity finding: the other tabs are
stretched but scrollable, so they are ugly rather than unusable.

### Reproduction

Minimal world: one registered user, one community, six events spread across the
current month, one shared gear item, one request. Sign in, dismiss the Data
Consent sheet, open the **Plans** tab. At 390×844 the month renders through day
31 and the day-detail panel below it reads `THURSDAY · AUG 13 · 2 events` with
both seeded items. At 1440×810 the same screen shows day cells ballooned to
~205 px squares, rows 24–31 clipped below the dock, and no day-detail panel at
all.

### Hypotheses tested

| # | Hypothesis | Probe | Observation | Verdict |
|---|---|---|---|---|
| H1 | The grid's square cells make its height a function of viewport width, starving the day detail | probe-01, probe-03 | Detail present ≤800 px wide at h=810; absent ≥900 px. Threshold moves with height exactly as `0.857·W + chrome > H` predicts | **confirmed** |
| H2 | The failure is app-wide, not calendar-specific | probe-01 | Home/Library/gear/request/experience all render and scroll at 1440×810 — stretched, never unreachable | **refuted** (as stated); real but lower-severity layout-quality issue |
| H3 | The bottom dock is itself pushed off-screen at landscape sizes | probe-01 | Dock bounding box `belowFold=0` at all six viewports incl. 1920×1080 | **refuted** |
| H4 | Short height alone triggers it, independent of width | probe-01 | 1440×1400 (wide **and** tall): detail still not painted. 390×620 (short, narrow): detail renders fine | **refuted** — width is the driving variable, and since the surrounding chrome is near-constant the failure condition reduces to the ratio threshold W ≳ 1.1·H |
| H5 | The clipped content is merely scrolled out of view and recoverable | probe-02 | Wheel ×6, drag, `End`, `PageDown` all no-op; content never appears | **refuted** — permanently unreachable |
| H6 | No automated surface could have caught this | playwright.config.ts, app/test | Both Playwright projects are 390×844 / iPhone 14; widget tests force phone surfaces | **confirmed** |

### Evidence

**1 — Width sweep pins the threshold and the mechanism** (probe-03; `Extension
Ladder` is a seeded day-detail entry, `August 31` the month's last cell):

| viewport | detail in a11y tree | detail visible | day 31 visible |
|---|---|---|---|
| 800×810 | yes | yes | yes |
| **900×810** | **no** | **no** | yes |
| 1100×810 | no | no | **no** |
| 1100×1080 | yes | yes | yes |
| **1200×1080** | **no** | **no** | yes |
| 1440×1080 | no | no | **no** |

The breakpoint moves with height, and both thresholds fit
`gridHeight ≈ 6·((W − 66)/7) + 30` — i.e. the grid's height is set by width.
Failure begins at roughly **W > 1.1·H**, so every landscape window fails; a
second, wider threshold (**W ≳ 1.33·H**) additionally clips the month's last row.

**2 — The content is not rendered, not merely off-screen.** ARIA snapshot at
390×844 contains `text: THURSDAY · AUG 13 · 2 events`, `button "Need a canopy
for Saturday"`, `button "Extension Ladder"`. At 1440×810 none of the three
appear anywhere in the tree, while all 31 day-cell buttons still do. At
1440×1400 the detail group returns to the tree but has no room to paint — the
distinction matters for the fix and for the accessibility claim.

**3 — No gesture recovers it** (probe-02, 1440×810): mouse wheel ×6, a 500 px
drag, `End` and `PageDown` each leave `inA11yTree=false` for both seeded
entries and for day 31. Consistent with the static reading — `HomeCalendarScreen`
builds `Column [header, grid, Expanded(detail)]` with no scrollable ancestor
(`app/lib/presentation/screens/portfolio/home_calendar_screen.dart:183`).

**4 — Methodological note that shaped the probes.** Playwright drives Flutter's
`flt-semantics` DOM nodes, not the painted CanvasKit pixels, so a node stays
DOM-"visible" and tappable while its pixels sit past the viewport edge. Probes
therefore assert on **bounding-box geometry vs. the viewport rect**, not on
tap success — a `.tap()` that succeeds proves nothing about human reachability.
The 1440×1400 case is exactly this trap: tappable in the tree, invisible on
screen. Release web builds also omit Flutter's yellow-black overflow banner
(debug-only assertion), so the clipping is entirely silent.

**5 — A second instance of the same class**, worth noting though not the
report: at 1440×810 the first-launch **Data Consent** sheet's Accept All / Save
row sits flush against the bottom edge, on the verge of clipping.

Provenance: `e2e/investigations/2908/` (gitignored) — `probe-01…03`, `shots/`,
`logs/*.aria.yaml`, `measurements.json`.

### Fix direction

Decouple day-cell height from viewport width (a height-aware
`childAspectRatio`, or cap the grid's height outright), and/or stop the app
from ever handing screens a viewport wider than they were designed for. The
same latent defect exists in `gear_booking_grid.dart`, which is the same
`GridView.count(crossAxisCount: 7)`-with-default-aspect-ratio shape.

## Potential Solutions

### Option A: Fix the width-driven grids

- **Approach:** In `CalendarMonthGrid`, wrap the grid in a `LayoutBuilder` and
  compute `childAspectRatio` from the height budget rather than accepting the
  default 1.0 — e.g. cap the grid at a fraction of available height so six rows
  always fit and the day detail keeps its share. Apply the same to
  `gear_booking_grid.dart` (identical shape, identical latent bug).
- **Pros:** Surgical and directly at the root cause; no visual change on phones;
  makes the calendar correct at *any* size rather than merely at sizes we
  happen to allow; keeps the design's full-bleed intent on wide screens.
- **Cons:** Fixes this instance of the class, not the class. Every future screen
  can reintroduce a width-driven height. Leaves the other tabs stretched and
  awkward at desktop widths (unusable line lengths, stranded controls).
- **Effort:** Small.

### Option B: Global max-width shell

- **Approach:** Clamp the app to a phone-like column on wide viewports via the
  `builder:` hook already present on `MaterialApp.router`
  (`app/lib/main.dart:785`) — a centered `ConstrainedBox` at ~640–840 logical
  px, plus a `MediaQuery` override so descendants observe the clamped size, with
  the letterbox filled by a themed surface.
- **Pros:** One small change fixes every screen at once and permanently retires
  this whole failure class — no screen can be handed a viewport it was not
  designed for. Honest about what the product is (phone-first; web is the
  entry surface for `/go/` links). Standard, well-understood Flutter Web
  treatment.
- **Cons:** A product/visual decision, not purely a bug fix — desktop users get
  a narrow centered column and a lot of letterboxing. Masks rather than fixes
  the grid defect (which resurfaces if the clamp is ever loosened). The
  `MediaQuery` override is load-bearing and easy to get wrong: **51 call sites**
  read `MediaQuery...size` today and would silently read the window instead of
  the clamp if it is omitted.
- **Effort:** Small to medium (the clamp is small; verifying the 51 sites and
  the modal/overlay/snackbar surfaces is the bulk).

### Option C: Genuinely adaptive desktop layouts

- **Approach:** Real breakpoints and per-screen desktop compositions — the
  calendar as a month grid beside a day-detail side panel, multi-column
  library/people, constrained reading measures on content routes. The
  `Responsive` helper (`app/lib/core/utils/responsive.dart`) already exists but
  is imported by exactly **one** file, so this is close to greenfield.
- **Pros:** The only option that makes desktop genuinely good rather than
  merely not-broken; uses the space instead of hiding it.
- **Cons:** Large, touches nearly every screen, and multiplies the layout
  permutations every future change must be checked against — with no automated
  desktop coverage today to hold it up. Far beyond what this issue reports.
- **Effort:** Large.

## Recommendation

**Ship A as the fix, B as a follow-on layout-quality improvement, and treat C as
separate work.**

An earlier draft of this plan recommended B first, on the reasoning that one
shell-level clamp retires the whole failure class. Checking that against the
probe-03 data disproves it, and the correction is worth stating plainly because
it inverts the order of work:

A width clamp cannot fix this bug at any width worth having. The grid needs
`≈ 0.857·W + 60` px of height before the day detail gets anything at all, and a
*useful* detail panel needs ~180 px more. So the minimum viewport height a clamp
still requires is:

| clamp width | grid + header | + usable detail (~180 px) |
|---|---|---|
| 600 px | 573 px | **753 px** |
| 640 px | 607 px | **787 px** |
| 840 px | 778 px | **958 px** |

(The model reproduces both measured thresholds in probe-03 exactly.) A 1440×900
laptop gives roughly 760 px of *viewport* height after browser chrome, and a
1280×720 screen gives about 600 px. So the 840 px clamp the earlier draft
proposed would still be broken on a standard laptop, and even a 640 px clamp
fails on a 720p screen or any window the user has dragged short. **B masks the
bug on some machines and leaves it on others** — which is worse than not fixing
it, because it makes the failure look intermittent.

A fixes it unconditionally: once cell height is derived from the available
height budget rather than from width, the calendar fits at every viewport,
including short ones, with or without a clamp. It is also the smaller change,
and it repairs `gear_booking_grid.dart`'s identical latent defect at the same
time.

B remains worth doing, but on its own merits — the other tabs are stretched and
awkward at desktop widths (unusable line lengths, controls stranded at the far
left) even though they stay scrollable, and a clamp is the cheapest broad
remedy. It should not be sold as the fix for #2908, and it should land *after*
A so the calendar is correct independent of it.

C is the right end state but is not what this issue asks for, and attempting it
now would mean authoring desktop layouts with no automated desktop coverage to
hold them up. Separate issue, sequenced after the Phase-1 gate exists.

## Open Questions

- **Is a letterboxed phone-width column acceptable on desktop, or does web
  warrant real desktop layouts?** — Recommendation: accept the clamp, but note
  this decision no longer gates the bug fix (Phase 1–2 stand alone). Web is
  currently the *entry* surface for `/go/` links and phone-first registration,
  not the primary product surface, so the return on full desktop layouts is low
  relative to its cost and ongoing maintenance. Revisit if web engagement data
  says otherwise. This is a product call the plan should not make silently.
- **What max width, if B ships?** — Recommendation: ~640 logical px, and only
  *after* A. Per the table in Recommendation, wider clamps demand viewport
  heights that common laptops do not have. Do not treat any clamp width as
  load-bearing for correctness — that is A's job.
- **What fills the letterbox?** — Recommendation: a flat themed surface colour
  from the existing tokens. Note the trap recorded on the website work: grounding
  a surface in `--color-surface`-style tokens can invert unexpectedly in dark
  mode, so pick the token deliberately and check both themes.
- **Should the clamp apply on mobile web too?** — Recommendation: no-op there by
  construction (phone viewports are already under the cap), so no branching on
  `kIsWeb` is needed; the clamp can apply uniformly and simply never binds on
  phones. Avoids a platform fork in layout code.
- **Does the clamp belong above or below the router?** — Recommendation: in the
  existing `builder:` on `MaterialApp.router`, which wraps the Navigator and so
  captures modals, bottom sheets and snackbars too. Verify the Data Consent
  sheet specifically, since it is a confirmed near-miss at 1440×810.

## Status

**Phases 1 and 2 shipped** (`7dedb4ca0`, branch `web_aspects`) — the app is
unbroken in landscape windows. Phase 3 (the shell clamp) and Phase 4 (adaptive
desktop layouts) are deliberately not done — both are tracked as **#2912,
"Make the app look intentional on desktop web aspect ratios"**, which carries
the per-screen findings from this investigation's sweep.

What landed:

- `month_grid_metrics.dart` — `monthGridCellSide` derives the day cell from
  **both** axes and takes the smaller, so the month fits any viewport and
  becomes a centred block once height binds. Width still binds on phones, so
  that layout is unchanged.
- Applied to **all four** screens that stack a month over other content — the
  investigation found the Plans calendar, plus `shared_calendar_screen.dart` and
  `workshop_library_calendar_panel.dart`, which have the identical
  `Column [header, grid, Expanded]` shape and therefore the identical bug — and
  to `gear_booking_grid.dart`, which had the same sizing inside a scroll view
  (ugly rather than broken; capped so the month stays readable).
- `desktop-chromium` Playwright project at 1440×810, scoped to `@desktop` specs.
- Regression coverage at both layers, each verified to **fail without the fix**:
  `month_grid_metrics_test.dart`, a landscape group in
  `home_calendar_screen_test.dart` across five viewport shapes, and
  `plans-calendar-landscape.spec.ts`.

Two things worth carrying forward, both discovered while writing the spec:

1. **`browser.newContext()` does not inherit the project's `use.viewport`.**
   Every workflow spec builds its own context (to inject auth before first
   navigation), so the desktop project's viewport silently did not apply and the
   spec ran at Playwright's default 1280×720. Any future spec that must run at a
   specific shape has to forward `test.info().project.use.viewport` explicitly.
2. **Substring role locators can match a day cell as well as its detail entry.**
   `getByRole('button', {name: /Ladder Return Party/})` matches both the grid
   cell (`"Friday, August 14, Ladder Return Party"`) and the detail entry;
   `.first()` picked the cell, which is on screen either way, so the assertion
   would have passed on the broken build. The spec uses `exact: true`.

## Implementation Plan

### Phase 1: Make the failure visible in CI
**Goal:** A landscape viewport exists in automated coverage and fails today, so
every later phase is provable rather than asserted.
**Tasks:**
- [ ] Add a `desktop-chromium` Playwright project (e.g. 1440×810) to
      `e2e/playwright.config.ts`, scoped to a small landscape-sensitive spec set
      rather than the whole suite, to bound CI cost (the #2738 lesson).
- [ ] Write an e2e spec that opens Plans and asserts the day-detail panel is
      **(a)** present in the accessibility tree and **(b)** has a bounding box
      inside the viewport rect. Both assertions are needed and neither implies
      the other: at 1440×810 it fails (a), at 1440×1400 it passes (a) and fails
      (b). Do **not** assert via tap success — `flt-semantics` nodes stay
      tappable while painted off-screen.
- [ ] Add a Flutter widget test that pumps `HomeCalendarScreen` at a landscape
      surface and asserts the day detail renders with no `RenderFlex` overflow —
      widget tests surface overflow, unlike release web. Note existing tests
      deliberately force phone-shaped surfaces (see
      `app/test/.../experience_time_panel_test.dart`), so this needs its own
      surface setup rather than the house default.
- [ ] Confirm both fail on `main` for the right reason before any fix lands.
- [ ] Avoid `networkidle` waits in the new spec — the app holds a long-lived
      stream, so it never settles.
**Validates:** That the repro is mechanised and the coverage gap (H6) is closed
at its root — not just this bug patched.

### Phase 2: Fix the width-driven grids (the actual fix)
**Goal:** The calendar fits at every viewport, including short ones, with no
dependence on a shell clamp.
**Tasks:**
- [ ] Make `CalendarMonthGrid`'s `childAspectRatio` height-aware via
      `LayoutBuilder` — derive cell height from the available height budget so
      six rows plus a usable day detail always fit, instead of accepting the
      default 1.0.
- [ ] Apply the same fix to `gear_booking_grid.dart` (identical latent defect,
      unreported).
- [ ] Add widget tests across a spread of surface sizes — wide-and-short,
      wide-and-tall, narrow-and-short — asserting the grid never exceeds its
      height budget and the detail keeps a usable minimum.
- [ ] Re-run the probe-03 width sweep (or its distilled spec) and confirm the
      day detail survives at every width/height pair in the matrix.
**Validates:** That #2908 is fixed at the root and unconditionally — this is the
phase that closes the issue.

### Phase 3: Global max-width shell (layout quality)
**Goal:** The remaining tabs stop being stretched and awkward at desktop widths.
Not required for the bug fix; ships only if the product call in Open Questions
lands on "clamp".
**Tasks:**
- [ ] Add the centered `ConstrainedBox` clamp in the `MaterialApp.router`
      `builder:` at `app/lib/main.dart:785`.
- [ ] Override `MediaQuery` so the 51 existing `MediaQuery...size` call sites
      observe the clamp. **Use `MediaQuery.of(context).copyWith(size: …)`, never
      a freshly constructed `MediaQueryData`** — a fresh instance silently drops
      `textScaler`, `padding`, `viewInsets`, `platformBrightness`,
      `disableAnimations` and `accessibleNavigation`, which would regress text
      scaling, safe-area insets, keyboard avoidance, dark mode and reduce-motion
      app-wide in one line.
- [ ] Fill the letterbox with a themed surface; verify light **and** dark (note
      the recorded trap that grounding a surface in a `--color-surface`-style
      token can invert in dark mode).
- [ ] Verify overlay surfaces inherit the clamp — modals, bottom sheets,
      snackbars, and the Data Consent sheet specifically (a confirmed near-miss
      at 1440×810).
- [ ] Confirm phone-viewport specs and walkthrough baselines are unchanged.
**Validates:** That desktop layout quality improves without disturbing the phone
layout or the accessibility surface.

### Phase 4: Follow-ups (separate issues)
**Goal:** Record the deliberately-deferred work rather than leaving it implicit.
**Tasks:**
- [ ] File an issue for adaptive desktop layouts (Option C), noting that
      `Responsive` has one importer and is effectively unused.
- [ ] File an issue for the layout-quality problems the clamp merely hides on
      content routes (line length, stranded controls) if the clamp is ever
      loosened.
- [ ] Consider a lint or review-checklist item against `GridView.count` with a
      default `childAspectRatio` inside a non-scrolling parent.
- [ ] Correct the misleading `#2157` reference in `web_unsupported.dart` (see
      Related Issues).
- [ ] File an issue for text-scaling coverage: the accessibility rubric requires
      the UI to survive 200% text, and nothing in `docs/client/accessibility.md`
      or the a11y lint rules covers it today. It is the same starvation
      mechanism as this bug (fixed-height chrome vs. growing content), so the
      Phase-2 height-budget work is the natural place to prove it.
**Validates:** That the scope boundary drawn here is explicit and tracked, per
the project's TODO/issue-hygiene rules.

## Related Issues

- #2157 — "Serve the whole Ripls app on web at ripls.app / dev.ripls.app", the
  change that put the phone-shaped app in a desktop browser in the first place;
  the direct origin of this failure class. (Note: `web_unsupported.dart` cites
  #2157 as "the broader plugin audit", which does not match that issue's actual
  title — the comment is misleading and worth correcting while in the area.)
- #2634 — introduced the converged dock and the embedded Plans tab whose
  `bottomContentInset` reserves the clearance this bug overruns.
- #2514 — introduced `HomeCalendarScreen`, the screen in the reporter's
  screenshot.
- #2738 — prior Playwright-config work on video/artefact cost; the same
  cost-control judgement applies to scoping the new desktop project in Phase 1.

## Audit notes

Audited with `ripls-audit` on 2026-08-13. Engaged dimensions: accessibility,
testing, code_quality, performance, tech_debt. (Not engaged: security,
api_design — no proto or RPC surface changes; dependency — no new packages;
error_handling, observability — no new failure or logging paths.)

Changes the audit made to this plan:

1. **Reversed the recommendation (A before B, not B before A).** The original
   draft recommended the shell clamp as the primary fix. Modelling the grid
   height against the probe-03 measurements showed no clamp width worth having
   survives a standard laptop viewport — an 840 px clamp needs ~958 px of
   viewport height for a usable day detail, and a 1440×900 laptop supplies about
   760 px. B would have masked the bug on tall monitors and left it on laptops,
   presenting as an intermittent failure. A is now the fix; B is layout quality.
2. **Split the Phase-1 e2e assertion into semantics-presence *and* geometry.**
   Either alone passes in one of the two observed failure modes.
3. **Added the `MediaQuery.copyWith` requirement to Phase 3** — a freshly
   constructed `MediaQueryData` drops `textScaler`, `padding`, `viewInsets`,
   `platformBrightness`, `disableAnimations` and `accessibleNavigation`, which
   would be an app-wide accessibility regression from a one-line change.
4. **Recorded that no existing coverage addresses 200% text scaling**, which the
   accessibility rubric requires and which fails by the same starvation
   mechanism as this bug. Filed as a Phase-4 follow-up rather than scope creep.
5. **Added the `networkidle` caution** to Phase 1 (long-lived stream never
   settles) and the phone-forced-surface note (existing widget tests pin phone
   dimensions, so a landscape test needs its own setup).
6. **Corrected the #2157 description** after `gh issue view` showed the title
   does not match what `web_unsupported.dart`'s comment claims.

Docs consulted: `client/architecture.md` ⚓, `client/conventions.md` ⚓,
`client/accessibility.md`, `client/calendar.md`, `client/inbox.md`,
`client/testing.md`, `client/testing/semantics_identifiers.md`,
`client/design.md`, `reviews/accessibility_review_prompt.md`,
`reviews/testing_review_prompt.md`, `reviews/code_quality_review_prompt.md`,
`reviews/performance_review_prompt.md`, `reviews/tech_debt_review_prompt.md`,
`e2e/README.md` § What's reliably reachable.

Compliance checks that passed: no proto changes (API/models separation and the
no-shared-RPC-types rule are not engaged); no new dependencies; no TODOs
introduced without tracking issues; all touched files remain well under the
1,000-line gate (`home_calendar_screen.dart` 377, `calendar_month_grid.dart` 122,
`nav_dock.dart` 529); accessibility primitives (`Tappable`, `IconAction`) and
`context.l10n` label sourcing are unaffected by the proposed changes; the plan
adds no user-visible strings, so no ARB/l10n parity work is required.
