# Issue #2798: Contrast and colors bad on feedback screen

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2798
**Filed:** 2026-07-29 | **Last activity:** 2026-07-29
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** `docs/client/architecture.md` ⚓, `docs/client/conventions.md` ⚓,
`docs/client/design.md`, `docs/client/modals.md`, `docs/client/accessibility.md`,
`docs/client/testing.md`, `docs/reviews/accessibility_review_prompt.md`,
`docs/issues/2770-token-contrast-evaluation.md`, `docs/issues/2441-visual-token-unification.md`

> Companion issue: **#2797** renames the profile-menu row that opens this sheet —
> see [`2797-help-feedback-label.md`](2797-help-feedback-label.md).

## Problem Statement

Two controls in `FeedbackSheet` paint a **light-theme surface token as their own
fill** inside the always-dark glass sheet, then draw **on-glass foregrounds** on
top of it. The foreground and the fill are both near-white, so the label is
effectively invisible.

`AppColors.accentButtonBackground` = `DesignTokens.lightSurface` = **`#F2F2EE`**.
It has exactly two call sites in the entire app, and both are in this file:

| Element | Fill | Foreground | Ratio | Needs |
|---|---|---|---|---|
| "Add Screenshots (optional)" label (`feedback_sheet.dart:426-431`) | `accentButtonBackground` `#F2F2EE` | `modalTextPrimary` `#FFFFFF` | **1.12:1** | 4.5:1 |
| its icon (`:416-420`) | `accentButtonBackground` `#F2F2EE` | `modalPrimaryButtonBackground` `#9DBFA8` | **1.79:1** | 3:1 |
| selected type chip, e.g. "Bug" (`:191-204`) | `accentButtonBackground` `#F2F2EE` | `modalPrimaryButtonBackground` `#9DBFA8` | **1.79:1** | 4.5:1 |

These are token-arithmetic values. They were confirmed against the reporter's own
capture: the button fill samples as RGB `(242, 243, 238)` — `#F2F2EE` to the
pixel, modulo JPEG — with the glyph core at `(255, 255, 253)`, and the chip label
measures 2.30:1 there (higher than 1.79 only because antialiasing blends the
glyph toward the fill).

**The rest of the sheet is fine**, and it is worth saying so because the issue
title implicates the whole screen. Measured from the same capture: privacy
footer 8.36:1, field hint text 4.31:1, character counters 5.77:1, and the Submit
CTA passes on the `modalPrimaryButtonText` / `modalPrimaryButtonBackground` pair
that `glass_modal_contrast_test.dart` already asserts. The defect is these two
controls, not the palette.

### How it got worse, not better

`git log -S` on the label line lands on a single commit:

```
af66dadd4  Fix #2764: on-glass foregrounds take the text ramp, not the button fill
-  color: AppColors.modalPrimaryButtonBackground,
+  // #2764 class: the button fill token used as a label colour.
+  color: AppColors.modalTextPrimary,
```

The #2764 remediation took this label from `#9DBFA8` on `#F2F2EE` (1.79:1) to
`#FFFFFF` on `#F2F2EE` (**1.12:1**). It was correct in general and wrong here,
because it assumed the background behind every on-glass foreground *is* the dark
glass sheet. This element paints its own light fill first. The `// #2764 class`
comment is still sitting in the file above the value it made worse.

That is the finding worth carrying forward: **the previous fix's blind spot is
the bug.** Fixing these two controls without fixing the blind spot reproduces the
situation exactly.

### Why every existing guard missed it

Three independent gaps, all verified:

1. **`scripts/check_glass_foreground.js` never looks at the fill.** It detects
   fill tokens in foreground *positions* and stops. Its failure message tells the
   author to "Use an on-glass foreground token (`GlassTokens.textPrimary` /
   `textSecondary` / `textFaint`) instead" — advice that is actively wrong when
   the element paints its own light fill, and is precisely the edit `af66dadd4`
   made.
2. **Local aliasing evades it.** `_buildTypePill` does
   `final primary = AppColors.modalPrimaryButtonBackground;` (line 182) and then
   `color: primary` inside a `TextStyle` (line 203). The scanner matches the
   literal token string, so the chip was **never flagged at all** — not fixed,
   not reported. The icon at line 418 escaped for a different reason: `color:`
   on an `Icon` widget is neither `iconColor:` nor inside a `TextStyle`.
3. **The composited pixel gate does not cover this sheet, and does not run.**
   `e2e/scripts/contrast_surfaces.mjs` declares six surfaces (gear/experience
   heroes, the two date pickers, request detail, home feed); the feedback sheet
   is not one. More importantly, `grep` over `.github/workflows/*.yaml` finds no
   reference to `run_contrast_gate.sh` or `test:contrast` — the gate exists as an
   npm script and gates nothing. That is 2770 Phase 4's first task, still
   unchecked.

`check_design_tokens.yaml` does run in CI, but it only verifies the pairs
*declared* in `design/tokens.json`. An ad-hoc pairing invented at a call site is
outside what it can see.

### Investigation note

The `ripls-ui-investigate` skill was not run. Its job is to bisect a UI report to
a layer; that was already achieved with more precision statically — the layer is
client render, the defect is two constants, the token arithmetic reproduces the
reporter's pixels to within JPEG noise, and the regression is bisected to a named
commit. The hermetic-harness work it would have done is not skipped, only moved:
Phase 3 boots that harness to author the `feedback-sheet` contrast surface and
capture before/after. The sheet is reachable on Flutter Web — no `kIsWeb` guard
in `feedback_sheet.dart` or the profile menu path.

## Context from Comments

**No comments.** The issue body carries the full ask: *"Shows light text on light
button, needs fixing and adding to contrast gating tests."* Both halves are
actionable and the second half is the more valuable one — the reporter is asking
for the class to be gated, not just the instance repaired.

One thing the issue does not say, and the plan should: the "contrast gating
tests" the reporter has in mind do not currently gate anything in CI. Any plan
that says "added to the contrast gate" without saying that would be reporting a
protection the repo does not have.

The attached screenshot is byte-identical to #2797's (md5
`d4276fac495e6a70e5bf5729a2989694`) — one capture reused across both
submissions. It happens to show this sheet, so it evidences this issue correctly.

## Potential Solutions

### Option A: Adopt the glass primitives these controls already bypass
- **Approach:** The sheet hand-rolls controls that already exist as primitives —
  and *the primitives already encode the correct pairing*. `feedback_sheet.dart`
  even imports the barrel (`modal/glass/glass.dart`) and then reimplements
  around it:

  | Hand-rolled | Primitive | The primitive's pairing |
  |---|---|---|
  | `_buildTypePill` (`:174-208`) | **`GlassChip`** | active = `modalChipBackgroundActive` (`fillStrong` `#FFFFFF`) + `modalChipTextActive` (`onFillStrong` `#1A1A1A`) ≈ **17.4:1** |
  | `_buildFieldLabel` (`:293-302`) | **`GlassFieldLabel`** | on-glass text ramp |
  | `_buildErrorBanner` (`:439-466`) | **`GlassInsetCard`** | inset fill + text ramp |
  | `_buildScreenshotButton` (`:381-437`) | **`GlassInlineAction`** (near fit) | `fillSubtle` `0x1AFFFFFF` + `textSecondary` |

  `GlassChip`'s doc comment states the rule outright: *"active chips flip to a
  solid white fill with near-black text."* The hand-rolled pill copied the
  solid-light-fill idea and then paired it with a light foreground — the
  primitive had the answer and the call site diverged from it. Adopt the
  primitives where they fit, borrow their tokens where they do not, and delete
  `accentButtonBackground` (no other call sites; no references in `scripts/`,
  `e2e/`, or `app/test/`, so the deletion is clean).
- **Pros:** Fixes the root cause rather than the symptom — a bespoke control that
  invented its own fill/foreground pairing gets replaced by the one that was
  already validated. Makes the sheet consistent with every other glass modal,
  shrinks the file, and removes the trap instead of leaving it armed. Future
  token moves reach this sheet automatically.
- **Cons:** Larger diff than recolouring two constants, and touches controls the
  issue did not name. `GlassInlineAction` is **not a drop-in**: it hard-codes a
  trailing chevron, which reads as "opens a sub-screen" and is wrong for a
  control that opens the OS picker (see Open Questions). Adopting `GlassChip` /
  `GlassInlineAction` also pulls in their **required `semanticsLabel`**, which
  the project sources from l10n — see the i18n note in Phase 1.
- **Effort:** Small-to-medium.

### Option B: Keep the cream fill, flip the foregrounds to dark ink
- **Approach:** Leave `accentButtonBackground` as the fill; set label and icon to
  `GlassTokens.onPrimary` `#14241A` (**14.43:1** on `#F2F2EE`).
- **Pros:** Smallest possible diff and it clears WCAG comfortably.
- **Cons:** Leaves two full-width cream slabs inside a dark glass sheet — the
  optional-screenshot button would be the visually loudest element on the screen,
  louder than Submit, which inverts the hierarchy the sheet is trying to express.
  It also keeps a light-*theme* surface token load-bearing inside an
  always-dark material, which `glass_tokens.gen.dart` calls "wrong here by
  construction", so the next author has the same trap in front of them.
- **Effort:** Small.

### Option C: Option A plus the three enforcement layers
- **Approach:** Ship A, then (i) a reusable widget-test primitive that pumps a
  subtree and pairs each `Text`/`Icon` colour against its nearest **opaque**
  ancestor `BoxDecoration.color`, asserting WCAG — this is the layer that runs in
  CI today (`test_flutter.yaml:201` runs `flutter test`); (ii) add a
  `feedback-sheet` surface to `contrast_surfaces.mjs` for pixel truth; (iii)
  patch `check_glass_foreground.js` for the two evasions found and correct its
  remedy message.

  Note the division of labour between (i) and (ii), which the glass material
  forces. Widget tests see **declared** colours, so they can only assert the
  opaque case — an element that paints its own solid fill, which is exactly this
  bug. They cannot evaluate a translucent on-glass fill, because what that
  composites against depends on the backdrop. Only the pixel gate can.
- **Pros:** Answers both halves of the reporter's ask. The widget primitive
  catches exactly the defect class — a locally-painted fill under an on-glass
  foreground — which neither the static lint nor the token gate can see, and it
  generalizes to every other glass sheet for free. Fixing the lint's message
  stops it from producing the next `af66dadd4`.
- **Cons:** Meaningfully more work than A, and the tree-walking primitive needs
  care to avoid false alarms on translucent and gradient fills.
- **Effort:** Medium.

## Recommendation

**Option C, staged so Phase 1 is independently shippable.**

Option A alone is not enough, and the evidence for that is in the repo: the last
targeted fix on this exact line made the contrast worse and nothing caught it for
three days until a user did. The reporter asked for the gate for that reason.

Option B is rejected on hierarchy, not on contrast — it passes WCAG and still
produces a worse sheet, with the optional action shouting louder than Submit.

**Do not close this on token arithmetic alone.** Glass composites over whatever
sits behind it, so a single ratio is only true for a single backdrop. The `#333`
sheet sampled from the reporter's capture is one data point, taken over a dark
home feed. `docs/client/modals.md` records that `modalSurface` (white @ 15%) is
already "too transparent to give white-on-glass text the contrast it needs" over
vibrant content — and the thinner inset fills are white @ 8-10%. Quoting a
composited ratio without naming the backdrop is precisely the move that produced
`af66dadd4`. Opaque pairs (the chip's `fillStrong`/`onFillStrong`) are safe to
assert from constants; translucent ones need Phase 3.

The staging matters because of where enforcement actually bites:

- `flutter test` **runs in CI** (`test_flutter.yaml:201`), so the widget-level
  pair test protects `main` the day it lands. This is the highest-value layer and
  it is also the cheapest.
- `run_contrast_gate.sh` **runs nowhere**. Adding the feedback sheet to
  `contrast_surfaces.mjs` is still right — it is how the surface gets captured
  and compared, and it is ready for the moment 2770 Phase 4 wires CI — but it
  should be described as coverage, not protection, until then.

Contribute the surface and the lint patches to 2770's Phase 4 checklist rather
than duplicating that work here; #2798 owns the pixel fix and the widget-test
primitive.

## Open Questions

- **Should the screenshot button read as primary or secondary?** —
  Recommendation: secondary (inset fill). Its own label says "(optional)", and
  the sheet already has a primary action. The current treatment reads as the
  loudest control on the screen, which is part of why the broken contrast is so
  visible.
- **`GlassInlineAction` for the screenshot button, or a local control on its
  tokens?** — It is the right shape (icon + label row on `fillSubtle`) but it
  hard-codes a trailing `chevron_right`, which promises navigation to a
  sub-screen; this control opens the OS image picker and can be disabled at 3
  attachments. Recommendation: **extend the primitive** with an optional trailing
  affordance rather than fork it — one more bespoke glass control is how this
  sheet got here. If that ripples into its existing call sites, fall back to a
  local control built from `modalInlineAction*` tokens and say so in the PR.
- **Which pair for the selected type chip — chip or CTA?** — Recommendation: the
  **chip** pair via `GlassChip` (`fillStrong` `#FFFFFF` + `onFillStrong`
  `#1A1A1A`, ≈17.4:1), not the primary-CTA pair (`primary` `#9DBFA8` +
  `onPrimary` `#14241A`, 8.06:1). Both clear AA; the chip pair is the one the
  material already uses for selected chips everywhere else, and using the CTA
  pair would make three type filters look like three submit buttons.
- **Do the type pills meet the touch-target minimum?** — No. `vertical: 8`
  padding around a 13px label is roughly **32dp** tall, against the 48×48dp
  minimum in `docs/reviews/accessibility_review_prompt.md` §3. Recommendation:
  fix it while in the file — adopting `GlassChip` should resolve it; verify
  rather than assume, since it is a different defect from the reported one and
  will otherwise sit here indefinitely.
- **Does the sheet survive 200% text scale?** — Untested, and the rubric (§4)
  asks for it. The three pills sit in a fixed `Row` with no wrap and the
  screenshot label is single-line. Recommendation: add a large-text-scale pump to
  the Phase 2 widget tests; if it overflows, `Wrap` the pills. Cheap to check
  while the file is open.
- **Delete `accentButtonBackground` or retarget it?** — Recommendation: delete.
  Two call sites, both defects, and a name that says "accent button background"
  attached to a light-theme *surface* token is an invitation to the next misuse.
  The `lint:dart:colors` allowlist ratchet is one-way on main, so removing a
  const is safe; adding one back later is the guarded direction.
- **Tree-walking widget primitive, or a declared list of legal pairs?** —
  Recommendation: the primitive, scoped conservatively — only assert where the
  nearest ancestor fill is **fully opaque** and a flat colour, and skip
  gradients, images, and alpha fills, which need the pixel gate anyway. A
  declared-pair list restates the same mistake in a second place and goes stale;
  the primitive reads the truth out of the tree. Ship it with the feedback sheet
  as its first subject and one other glass sheet to prove it generalizes.
- **Does the widget test belong in `app/test/core/theme/` or beside the sheet?** —
  Recommendation: the helper in `app/test/core/theme/` next to
  `glass_modal_contrast_test.dart`, with per-sheet tests beside the widgets they
  cover, so a new sheet's author finds the pattern where they are already working.
- **Should `check_glass_foreground.js` also flag `Icon(color:)`?** —
  Recommendation: yes. It is why the icon in this button stayed at 1.79:1 while
  the label next to it got "fixed". Expect a handful of legitimate existing
  hits — triage them into the fix or the allowlist with tracking issues rather
  than widening the allowlist wholesale.
- **Does this need a walkthrough-reel re-render?** — Recommendation: check, do
  not assume. 2770 Phase 3 makes re-rendering published reels an exit criterion
  for visual change. The feedback sheet does not appear in the #2684 / #2687 /
  #2703 / #2708 reels, so the answer is very likely no — confirm before closing.

## Implementation Plan

### Phase 1: Put the controls back on the primitives
**Goal:** The reporter's screen is legible, and the bespoke pairings that broke
it are gone rather than re-tuned.
**Tasks:**
- [ ] `_buildTypePill` (`:174-208`) → **`GlassChip`**. This carries the fix
      (`fillStrong` + `onFillStrong`) and drops the `final primary = ...` alias
      (`:182`) that hid the site from `lint:glass-foreground` entirely.
- [ ] `_buildScreenshotButton` (`:381-437`) → `GlassInlineAction` if the chevron
      question resolves that way, otherwise a local control on
      `modalInlineActionBackground` / `modalInlineActionText`. Delete the
      now-false `// #2764 class` comment at `:427`.
- [ ] `_buildFieldLabel` (`:293-302`) → `GlassFieldLabel`; `_buildErrorBanner`
      (`:439-466`) → `GlassInsetCard`. Both are already-correct primitives this
      file reimplemented; adopting them is why the diff shrinks.
- [ ] **i18n dependency:** `GlassChip` and `GlassInlineAction` both **require**
      `semanticsLabel`, sourced from l10n. `a11yMiscAddScreenshot` already exists
      and is already used here; the three type filters need labels. Add keys to
      **both** `app_en.arb` and `app_es.arb` — `check_l10n_parity.js` requires
      an exact key-set match. Do not migrate the sheet's other ~15 hardcoded
      strings here; that is its own issue.
- [ ] `app_colors.dart:141` — delete `accentButtonBackground`. Verified clean:
      no references in `app/lib` outside this sheet, nor in `scripts/`, `e2e/`,
      or `app/test/`.
- [ ] Confirm the pills now clear the 48dp touch-target minimum.
- [ ] Extend `app/test/core/theme/glass_modal_contrast_test.dart` with the
      opaque pairs (chip fill/ink at 4.5:1). Leave the translucent inset fill to
      Phase 3 — a widget test cannot know its backdrop.
- [ ] `npm run lint:dart:colors`, `npm run lint:glass-foreground`,
      `npm run lint:dart:l10n-parity`, `npm run lint:dart:a11y`,
      `npm run lint:dart:size` clean; `cd app && flutter test --reporter expanded`
      (run `npm run generate` first — `app_localizations*.dart` is generated).
**Validates:** Both controls clear WCAG AA on the sheet the reporter
photographed, the light-theme surface token is gone from the glass material, and
the sheet no longer carries private copies of four shared controls.

### Phase 2: A contrast primitive that runs in CI
**Goal:** A locally-painted fill under an on-glass foreground fails
`flutter test`.
**Tasks:**
- [ ] Add a test helper that pumps a widget, walks the element tree, and for each
      `Text`/`Icon` resolves the nearest ancestor with an opaque flat
      `BoxDecoration.color` / `ColoredBox`, then asserts WCAG. Skip translucent,
      gradient, and image fills explicitly and record why in the helper's doc
      comment.
- [ ] Apply it to `FeedbackSheet` (both `_buildForm` and `_buildSuccessState`)
      and to one other glass sheet to prove it generalizes.
- [ ] Verify it **fails** against the Phase 1 parent commit — a contrast test
      that never saw the bug is a vacuous assertion.
**Validates:** That the defect class, not just this instance, is caught by a
suite that already gates `main`.

### Phase 3: Pixel coverage for the sheet
**Goal:** The composited gate can see this surface.
**Tasks:**
- [ ] Add a `feedback-sheet` entry to `e2e/scripts/contrast_surfaces.mjs`
      (group `bespoke`) whose `open` walks avatar → profile menu → the Help row.
      Role/label locators only — CanvasKit paints text to canvas
      (`e2e/README.md`). Use **`SYNTHETIC_BACKDROPS`, not `['none']`**: the sheet
      is translucent and opens over the home feed's media, so the backdrop is the
      axis that matters. Both themes still belong in the matrix, but expect the
      sheet itself to be near-identical across them — glass renders the same in
      light and dark by design (`docs/client/design.md`), so a theme *difference*
      in the sheet body is itself a finding.
- [ ] Run `npm run test:contrast:pr` locally; capture before/after and attach to
      the PR.
- [ ] Add the surface and the CI-wiring dependency to 2770 Phase 4's checklist;
      state plainly in the PR that this is coverage, not a gate, until that lands.
**Validates:** The fix holds at the pixel level over the real composited
material, and the surface is queued for the moment the gate is wired.

### Phase 4: Close the lint's blind spot
**Goal:** `check_glass_foreground.js` stops producing bad fixes.
**Tasks:**
- [ ] Resolve one-hop local aliases (`final x = <FILL_TOKEN>;` then `color: x`)
      so `_buildTypePill`-shaped code is seen.
- [ ] Treat `color:` on `Icon(...)` as a foreground position.
- [ ] Rewrite the failure message: when the element paints its own fill, the fix
      is to pair the foreground against **that fill**, not to reach for the
      on-glass text ramp. Name #2798 as the case that proves it.
- [ ] Extend `scripts/check_glass_foreground.test.js` with a fixture in the
      `feedback_sheet.dart` shape (light fill + on-glass foreground).
- [ ] Triage whatever new hits appear — fix or allowlist with a tracking issue,
      per `CLAUDE.md`'s TODO policy.
**Validates:** That a future `af66dadd4` is told the right thing instead of the
wrong thing.

## Related Issues
- **#2764** — the on-glass foreground defect whose fix (`af66dadd4`) regressed
  this label from 1.79:1 to 1.12:1. Same class, opposite direction.
- **#2770** — the token-contrast evaluation and the composited gate. Phase 4
  ("wire the gate into CI") is the blocker on Phase 3 here becoming a real gate;
  the surface and lint patches belong on its checklist.
- **#2441** — established the token set, and `DesignTokens.lightSurface`, whose
  use inside glass is the root cause.
- **#2445** — the same defect class on flat surfaces.
- **#2797** — renames the profile-menu row that opens this sheet.

## Outcome

Implemented across all four phases. Measured from a real render of the fixed
build (hermetic e2e stack, dark theme, 2× CanvasKit capture):

| Control | Before | After |
|---|---|---|
| Selected type chip ("Bug") | 1.79:1 | **17.40:1** |
| "Add Screenshots (optional)" | 1.12:1 | **11.16:1** |

Deviations from the plan as written, and why:

- **`GlassInlineAction` was a drop-in after all** — no primitive change needed.
  The open question assumed its trailing chevron would read as "navigates to a
  sub-screen", but `media_picker_dialog.dart` already uses it with the chevron
  for Photos/Videos/Camera, i.e. exactly this job. The precedent settled it.
- **The touch-target fix did not land.** The plan said "adopting `GlassChip`
  should resolve it; verify rather than assume" — verified, and it does not.
  `ModalTheme.chipPaddingVertical` (8) around a 13px label is ~33dp for *every*
  glass chip in the app, and neither `Tappable` nor `Toggle` enforces a minimum.
  Fixing it means moving shared geometry across every glass sheet, which is not
  a contrast bugfix. Left for a follow-up.
- **The lint's detection gaps are closed, and its premise corrected** — #2806,
  folded into this PR. The gaps could not be closed on their own: doing so
  surfaced 6 call sites that are all *correct* at ~6.3:1, because the lint's
  premise ("~1:1 as a foreground") described `glass.primary`'s **pre-#2770**
  value, the deep sage `#3E5A47` at 1.66:1. #2770 flipped it to the light sage
  `#9DBFA8`, and the token's own `$description` now calls it "the on-glass action
  colour".

  So the fix was to make the rule true rather than to enforce a false one harder.
  The token list splits by what each token actually is:

  - **fill-only** (`fillStrong`, `modalChipBackgroundActive`, both solid white) —
    banned in every foreground position. Solid white as a foreground is
    `textPrimary`'s job.
  - **dual-role** (`glass.primary` and its alias) — banned only in `ColorScheme`
    slots. Painting it as an accent foreground on glass is correct.

  The `ColorScheme` ban does **not** relax with the value change, and that is
  the part worth keeping straight: one slot becomes a TextButton foreground *and*
  a selected-day fill at once, so the author picks neither. That is #2764's real
  mechanism, and with today's light sage the day cell is light sage under white
  text at 2.01:1.

  With the rule corrected, the two gaps close cleanly — one-hop local aliases
  (how #2798's chip evaded the scanner entirely) and `Icon(color:)` (why the icon
  beside the broken label kept its fill token while the label got "fixed").
  `docs/client/design.md` carried the same stale "1.01:1" claim and is corrected
  too.
- **The 200% text-scale check found a real problem**, so `_buildTypeSelector`
  moved from `Row` to `Wrap`.

## Audit notes

Audited against the context map. Findings applied in place:

- **The recommendation changed.** The first draft hand-picked replacement
  colours. `feedback_sheet.dart` already imports the glass barrel and then
  reimplements four controls that exist as primitives — and `GlassChip`'s doc
  comment already states the rule the hand-rolled pill broke ("active chips flip
  to a solid white fill with near-black text"). Option A is now *adopt the
  primitives*, which is the root-cause fix rather than a second round of the
  hand-tuning that produced `af66dadd4`.
- **Corrected an unsound number.** The draft quoted "10.7:1" for white on a
  translucent inset fill as though it were universal. Glass composites over its
  backdrop; `docs/client/modals.md` records white @ 15% already failing over
  vibrant content. Replaced with the opaque chip pair (assertable from
  constants) and an explicit statement that translucent pairs need Phase 3 —
  quoting a backdrop-free ratio is the exact error this issue is about.
- **Added the accessibility-rubric items the draft missed** (§3 touch targets:
  the pills are ~32dp against a 48dp minimum; §4 text scaling to 200% on a fixed
  three-pill `Row`), both in code this change already edits.
- **Added the i18n dependency to Phase 1.** `GlassChip` / `GlassInlineAction`
  require `semanticsLabel` from l10n, and `check_l10n_parity.js` demands an
  exact key-set match across `app_en.arb` / `app_es.arb` — an ambush if
  discovered mid-implementation.
- **Corrected the Phase 3 surface spec** from `backdrops: ['none']` to
  `SYNTHETIC_BACKDROPS`, and noted that the glass material renders identically
  across themes by design, so a theme difference in the sheet body would be a
  finding rather than expected variation.
- **Verified the token deletion is clean** — `accentButtonBackground` has no
  references outside this sheet in `app/lib`, `scripts/`, `e2e/`, or `app/test/`.
- No security, api_design, observability, or performance dimension is engaged:
  client-only, no proto/RPC change, no new dependency, no data flow. The file is
  546 lines, under the 1,000-line gate, and primitive adoption reduces it.

**Docs consulted:** `docs/llms.txt`; anchors `client/architecture.md`,
`client/conventions.md`, `reviews/accessibility_review_prompt.md`; leaf docs
`client/design.md`, `client/modals.md`, `client/accessibility.md`,
`client/i18n.md`, `client/testing.md`,
`docs/issues/2770-token-contrast-evaluation.md`.
