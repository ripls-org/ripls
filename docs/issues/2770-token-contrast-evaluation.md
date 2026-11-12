# Issue #2770: Contrast is gated on flat surfaces but rendered on glass — real in-app token evaluation harness

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2770
**Filed:** 2026-07-25 | **Last activity:** 2026-07-25
**Labels:** none | **Assignees:** unassigned
**Context docs:** `docs/client/conventions.md` ⚓, `docs/client/architecture.md` ⚓,
`docs/client/design.md`, `docs/client/modals.md`, `docs/client/accessibility.md`,
`docs/client/testing.md`, `docs/client/testing/semantics_identifiers.md`,
`docs/testing/architecture.md` ⚓, `docs/walkthroughs.md`,
`docs/reviews/accessibility_review_prompt.md` ⚓,
`docs/reviews/testing_review_prompt.md` ⚓, `design/README.md`, `e2e/README.md`.
`proto_conventions.md` deliberately not loaded — no proto surface changes.

## Problem Statement

#2441 made `design/tokens.json` the canonical token set and gave it a WCAG gate
at generation time. The gate is real, but it is only expressive enough to check
**token-on-token pairs against `background` and `surface`** — two flat, opaque
colors. A large share of the app does not render on those surfaces.

The glass material is a **third surface the gate cannot see**: roughly 40
hand-tuned alpha constants in `app_colors.dart` (`modal*`), a second family for
chat (`pillBg`, `bubbleBg`, `bottomBarBg`), and bespoke materials for the impact
modal (warm paper) and Workshop (parchment). `design/README.md` states outright
that the glass material does *not* belong in the token set, which is precisely
how those constants ended up outside every gate.

The failure this produces is not a bad palette — it is **a token validated on
one surface being used as a foreground on another**, with nothing able to detect
it. `glass_pickers.dart:28` assigns `scheme.primary =
modalPrimaryButtonBackground = lightPrimary` (deep sage `#3E5A47`). The comment
on that token justifies it at "white on #3E5A47 ≈ 6.8:1" — correct for the
*fill*. But Material also uses `scheme.primary` as the **TextButton foreground**,
so Cancel/OK and `todayForegroundColor` paint sage text directly on the dark
glass sheet. Measured: **1.64:1**. That is #2764.

**#2445 is the same class on flat surfaces** — the legacy shared status consts
point at the dark token values and under-deliver on light backgrounds
(`warning` ≈ 2:1), which `docs/client/design.md` already documents.

Two supporting gaps: `docs/client/accessibility.md` contains no contrast section
at all, and `docs/reviews/accessibility_review_prompt.md:71` asks reviewers to
verify 4.5:1 by eye with no mechanism behind it.

**On `ripls-ui-investigate`:** #2764 is UI-observable, so that skill would
normally run here. It was not, because the layer is already pinned harder than a
probe loop would pin it — the reporter's screenshot, the exact line
(`glass_pickers.dart:28`), an analytic prediction of the composite
(`0.85·(0.65·B)+38.25`), and a pixel measurement of **1.64:1** on both the DOM
compositor and CanvasKit all agree. The verdict is *client-render*, and the
Potential Solutions below are grounded in that measurement rather than in a
reading of the issue text.

## Context from Comments

The issue was filed as the umbrella for this work after a design interview, so
the decisions are recorded on it rather than debated in it. Two carry weight:

- **The palette may not be the defect.** Scope was deliberately opened to the
  glass material and the "on-glass content is always white, identical in light
  and dark" rule from `docs/client/design.md`, not just token values.
- **A de-risk prototype reversed one decision.** The plan originally called for
  a runtime token override channel; measurement showed it does not pay for
  itself, and the harness rebuilds instead. See *De-risk results*.

#2764 is a user-feedback report (Pixel 9 Pro, Android 16, app 0.27.0, PROD) with
a screenshot; #2445 is a maintainer-filed migration with no discussion.

## Potential Solutions

### Option A: Re-engineer the glass material, keep Ink & Sage
- **Approach:** Leave the palette alone; change the material — scrim strength,
  blur sigma, sheet alpha — and add a dedicated on-glass foreground ramp.
- **Pros:** No brand change, so no marketing/web/email churn and no reel
  re-rendering. Directly targets where the measurements say the defect is.
- **Cons:** Cannot fix a token that is simply too dark to be a foreground on a
  dark sheet; some palette movement may still be forced.
- **Effort:** Medium.

### Option B: Factored canonical on-glass ramp (recommended)
- **Approach:** Collapse the 50 glass constants to a minimal canonical set
  (~16 + status, see *Appendix — Glass token redundancy*), then add it as a
  third ramp — `glass-primary`, `glass-text-*`, `glass-status-*` — distinct from
  the light/dark ramps and gated against the *composited* backdrop. Move the
  factored material into `design/tokens.json`.
- **Pros:** Makes "deep sage as a foreground on a dark sheet" **impossible to
  express** rather than merely discouraged. Gives the CI gate something concrete
  to check — and something checkable at all, which seven aliases for solid white
  are not. Subsumes Option A: the material is one of the things it tokenizes.
  Catches the chat glass family that #2441 left on the old warm-brown palette.
- **Cons:** Reverses the documented exclusion in `design/README.md`. ~730
  `AppColors.modal*` call sites migrate, and the factoring means many become
  *renames to a different token*, not a mechanical substitution — each aliased
  call site needs a judgement about which canonical token it actually wanted.
- **Effort:** Large.

### Option C: New palette with a dual-surface primary
- **Approach:** Re-open the palette for a mid-tone primary clearing 4.5:1 on both
  white and dark glass.
- **Pros:** One primary that works everywhere; simplest conceptual model.
- **Cons:** Widest blast radius — app, `tokens.css`, marketing, emails, and every
  published walkthrough reel. And the noise-backdrop measurements suggest *no*
  single primary survives a busy mid-tone photo behind glass, so this may not
  actually solve the problem it costs the most to attempt.
- **Effort:** Large.

## Recommendation

**Option B**, with Options A and C carried as *candidate axes* in the evaluation
rather than as alternatives decided up front — which is the whole point of
building the harness first. The measured evidence already favours B: on a
high-frequency-noise backdrop the blur averages to ~109 gray and the entire
non-white ramp collapses (sage 1.46, `info` 1.24, `success` 1.16, `warning` 1.06,
`error` 1.03) **regardless of palette**. That is a material defect, not a palette
defect, and only a dedicated on-glass ramp gated on composited pixels prevents
its recurrence.

The harness is built before the decision so the choice is made on rendered
evidence. Round 1 evaluates 3 palettes × 3 glass ramps as independent axes.

## Open Questions

- **~~Does the CI gate run per-PR or nightly?~~ DECIDED:** minimal per-PR,
  full matrix nightly. Per-PR runs the glass+media core, both themes, worst-case
  synthetic backdrop only.
- **What happens to `modalContentScrim` opt-in?** — Recommendation: if the gate
  proves the base sheet fails over bright media, the scrim should stop being
  opt-in (`GlassSheet.scrim`) and become the default, with opt-*out* for sheets
  demonstrably over dark media. Decide from round-1 renders, not now.
- **Do the bespoke materials (impact paper, Workshop parchment) get tokenized
  too?** — Recommendation: measure them in round 1 but defer tokenizing to a
  follow-up. They are lower traffic and folding them in now widens an already
  large migration.
- **Does `docs/client/design.md`'s "on-glass content is identical in light and
  dark" rule survive?** — Recommendation: expect it to. The scrim genuinely does
  handle theme adaptation; what fails is *brand* tokens used as on-glass
  foregrounds, which the new ramp fixes without making on-glass text
  theme-aware.
- **~~Reel re-rendering trigger.~~ DECIDED:** re-render aggressively on *any*
  visual change, palette or material, so what is published is always recent.
  Re-render is a Phase 3 exit criterion, not a Phase 4 afterthought.

## Implementation Plan

### Phase 1: Harness and gate — **COMPLETE**
**Goal:** Measure composited contrast on the real bundle, for the shipped tokens.

**Result:** the gate reproduced #2764 without being told about it —
`glass-date-picker`, `primary` at **1.01:1** (needs 3:1), fg `[69,88,75]` on bg
`[83,83,83]`, ×4 instances. Foreground and background sit at nearly identical
luminance, i.e. genuinely invisible. Worse than the 1.62 analytic figure because
this sheet composites lighter (bg 83 vs 52), pushing sage closer to its own
background.

Across 8 surfaces: 202 failing instances, worst per token — `text-faint` 1.27:1
(×122), `text-secondary` 2.53:1 (×66), `primary` 1.01:1 (×12), `text-primary`
1.27:1 (×2). `home-feed-light-white` passes clean, so the gate discriminates
rather than failing everything.

**Known coverage limit:** only design tokens are sentineled, so the gate is
blind to the ~50 glass constants in `app_colors.dart` — the date-picker surface
yielded just 5 measurable instances because its day numbers use
`modalTextPrimary`, an app-level constant. Coverage jumps once the glass
material is tokenized, which Phase 2 now does first (see below).

**Tasks:**
- [ ] Sentinel token generation: a mode of `scripts/gen_design_tokens.js` that
      emits a sentinel-hued variant of every token (one pass, distinct hues)
- [ ] Render sweep driving the e2e stack across the surface matrix
      (`e2e/lib/capture.ts` phone shape, both themes, synthetic backdrops)
- [ ] Sentinel-diff analyzer: mask by nearest-hue classification,
      **connected-component segmentation**, ring background excluding only the
      token's own pixels, WCAG ratio per token per component
- [x] Decode PNGs **in process** (`e2e/scripts/png.mjs`, on Node's `zlib`).
      ffmpeg was the first choice — already a project tool — but it is absent
      from the self-hosted runner image, and a full sweep decodes ~876 gate
      images: via ffmpeg that is ~1,750 subprocess spawns and ~4.4GB of
      intermediate raw files (each 960×1800 frame is ~5MB of rgb24). ~110 lines
      with all five PNG filter types under test, and no runner-image change.
- [ ] **Unit-test the analyzer** against synthetic fixtures with analytic ground
      truth — classification, segmentation, ring background, WCAG math. Wire
      into `npm run test:scripts`; precedent is `scripts/gen_design_tokens.test.js`.
      The analyzer is the thing the gate's credibility rests on; an untested
      contrast estimator is a gate that lies confidently.
- [ ] Report the shipped token set's failures — expect #2764 and #2445 to appear
**Validates:** That the gate reproduces the known defects without being told
about them. If #2764 does not surface at ~1.6:1, the harness is wrong.

### Phase 2: Candidate exploration and comparison sheet
**Goal:** Pick a palette and glass material on rendered evidence.

**Ordering correction found in Phase 1:** evaluating *glass-ramp* candidates
requires being able to generate them, and the glass values are 52 hand-written
constants in `app_colors.dart`. So factoring and tokenizing the glass material —
originally filed under Phase 3 — is a prerequisite of authoring candidates, not
a follow-up. It also lifts the gate's coverage limit noted above. The phase
deliverables are unchanged; only the internal order moves.

**Tasks:**
- [x] **Factor and tokenize the glass material first** (see *Appendix — Glass
      token redundancy*): 52 declarations → 19 canonical tokens in
      `design/tokens.json`, emitted to `glass_tokens.gen.dart`. `AppColors`'
      `modal*` names became aliases, so the ~730 call sites needed no change.
- [x] **Reuse one browser across captures.** `openProbe` gained a `browser`
      option; the capture loop launches one per pass.
- [x] **Tokenize the media-overlay material.** Not in the original plan — the
      first sweep showed the glass ramp moving almost nothing, because the bulk
      of failures were on hero/media surfaces governed by a *second*
      untokenized material: four inline `LinearGradient`s plus a dozen alphas in
      `app_colors` methods. 11 canonical tokens in `overlay`. The fix that
      matters is `scrim-floor`: the shipped gradients ran to
      `Colors.transparent`, so hero titles and metadata sat on bare photo.
- [x] **Generalise materials into a registry.** `MATERIALS` in
      `gen_design_tokens.js` drives validation, Dart emission, sentinel
      selection and contrast roles; a second material was the signal to stop
      hand-coding each one.
- [x] **Lint fills used as foregrounds** (`lint:glass-foreground`). Found 8
      violations including the #2764 line and two in `feedback_sheet.dart` that
      inspection had missed. Ratchet allowlist pending the Phase 3 migration.
- [ ] Re-sweep now that the overlay material is tokenized — the previous sweep
      could not discriminate ramps because the dominant failures were on a
      material that was not varying
- [ ] Author 3 palette candidates and 3 glass-ramp candidates as token sets,
      each expressed in the **factored** canonical shape — the sweep is what
      confirms the collapse holds up visually, in particular whether the
      on-glass text ramp needs real differentiated values or collapses to one
- [ ] Sweep all 9 combinations (18 builds ≈ 47 min) plus real-photo renders
- [ ] Generate the comparison sheet in `docs/concepts/theme/` — independent
      palette and glass-ramp pickers, theme/backdrop toggles, glass+media and
      flat groups expanded, bespoke and creation collapsed, measured contrast
      inline with failures marked
- [ ] **Add the LFS pattern to `.gitattributes` *before* committing any render.**
      Today it covers `*.onnx`, `e2e/fixtures/*.mp4`, and
      `website/content/walkthroughs/**/*.mp4` — there is **no PNG rule**, so the
      renders would land as ordinary ~60MB of git blobs and need history surgery
      to extract. Add a path-scoped `*.png` rule for the render directory only.
- [ ] Commit the 126 visual renders to Git LFS; gate renders stay ephemeral
- [ ] Confirm the docs deploy checkout fetches LFS (`lfs: true`) if the sheet is
      published, matching the reel precedent
- [ ] **Review gate: Bryan selects palette + glass material**
**Validates:** Whether the material or the palette dominates the outcome — the
question the whole effort exists to answer.

### Phase 3: Factor, tokenize, and adopt
**Goal:** The selection ships, on a canonical glass token set rather than the
current sprawl.
**Tasks:**
- [ ] **Factor the glass tokens to a minimal canonical set** — see
      *Appendix — Glass token redundancy* for the measured starting point (50
      consts, ~34 distinct values, two incompatible base colors). Target shape
      in that appendix; the collapse is a prerequisite for tokenizing, not a
      follow-up, because tokenizing 50 near-duplicates would just move the mess
      into `tokens.json` and hand the gate 50 things to check.
- [ ] Fold the chat glass family (`pillBg`/`bubbleBg`/`bottomBarBg` and the
      gradient pairs) onto the same base as modal glass, or record why it stays
      separate. It is currently on the **pre-Ink&Sage warm-brown palette** and
      was missed by #2441.
- [ ] Move the factored glass material into `design/tokens.json` (on-glass
      colors, alphas, blur sigma, scrim); extend the generator's Dart/CSS/Go
      outputs
- [ ] Update `design/README.md` — remove the glass exclusion, state the new shape
- [ ] **Put the glass ramp in a sibling file, not `app_colors.dart`.**
      That file is **905 lines** against the 1,000-line cap enforced by
      `scripts/check_dart_file_size.sh` (`npm run lint:dart:size`); ~40 tokens
      with doc comments is ~120+ lines and breaches it. Split per
      `docs/server/architecture.md` §3 naming (e.g. `app_colors_glass.dart`).
      Do not reach for the `// dart-line-count-allow` hatch.
- [ ] Migrate ~730 `AppColors.modal*` sites onto the glass ramp (const-to-const)
- [ ] Keep `scripts/color_allowlist.txt` shrinking, never growing — the
      `lint:dart:colors` ratchet is one-way on main
- [ ] Migrate #2445's ~163 status call sites to the theme-aware getters; delete
      the legacy shared consts
- [x] Fix #2764 specifically: `glass_pickers.dart` stops using a fill token as
      `scheme.primary`. **Verified on the reporter's actual screen** (create →
      draft → Set time → Select a date), measured from the capture rather than
      reasoned about: Cancel 1.75:1 → **4.74:1**, OK 1.75:1 → **4.88:1**, day
      cell 4.67:1, header 7.26:1. The sage survives only as the selected-day
      FILL, with white text on it — the role it was always correct for.
- [ ] **Re-render every published walkthrough reel** (#2684 / #2687 / #2703 /
      #2708) and re-publish. Exit criterion for the phase — no visual change
      ships with stale reels.
**Validates:** That the gate from Phase 1 goes green on surfaces it previously
failed, and that the canonical set is small enough to hold in your head.

### Phase 4: Enforcement and documentation
**Goal:** The class of bug cannot recur silently.
**Tasks:**
- [ ] Wire the gate into CI: **minimal per-PR** (glass+media core, both themes,
      worst-case synthetic backdrop only) and the **full matrix nightly**.
      Budget against the self-hosted runners deliberately — the gate adds a
      second `flutter build web --wasm` (~2.6 min, and Flutter codegen is
      already the dominant RAM consumer on the mimir runners) plus renders.
- [ ] Re-render reels on any visual change. If this is to happen reliably rather
      than by memory, it wants automation — a nightly or post-merge job in the
      same shape as the `claude_refresh_docs` ratchet.
- [ ] Add a contrast section to `docs/client/accessibility.md` pointing at the
      gate; advance its freshness stamp
- [ ] Update `docs/reviews/accessibility_review_prompt.md` so "check 4.5:1"
      names the mechanism instead of asking for an eyeball
- [ ] Update `docs/client/design.md` and `docs/client/modals.md` for the glass
      ramp; advance freshness stamps
- [ ] Re-render published walkthrough reels if the palette moved
**Validates:** A future PR reintroducing a fill token as an on-glass foreground
fails CI.

## Related Issues
- #2445 — same defect class on flat surfaces; closed by Phase 3.
- #2764 — the user-visible instance; closed by Phase 3.
- #2441 — established the token set and the gate this extends.
- #2634 — "liquid glass" nav restructure; shares the glass material and should
  consume whatever Phase 3 lands rather than diverging.
- #1797 — the glass-modal revamp that established the current material.
- #2772 — `env:start` hangs when it boots the Auth Emulator itself; found while
  building the Phase 1 harness, which works around it.

## Appendix — Glass token redundancy

Measured against `app_colors.dart` at the time of planning. The glass surface is
**50 colour constants carrying ~34 distinct values, across two incompatible base
colours.**

### Modal glass: 39 consts → 24 distinct values

| Value | Names | Aliases |
| --- | --- | --- |
| `0xFFFFFFFF` | 7 | `modalTextPrimary`, `modalTextSecondary`, `modalTextTertiary`, `modalTextMuted`, `modalChipBackgroundActive`, `modalChipBorderActive`, `modalPrimaryButtonText` |
| `0x26FFFFFF` | 3 | `modalSurface`, `modalBorderSubtle`, `modalFooterDivider` |
| `0x14FFFFFF` | 3 | `modalInlineActionBackground`, `modalSearchFieldBackground`, `modalInsetCardBg` |
| `0x1FFFFFFF` | 2 | `modalIconBadgeBackground`, `modalInsetCardBorder` |
| `0x40FFFFFF` | 2 | `modalIconBadgeBorder`, `modalChipBorder` |
| `0xE6FFFFFF` | 2 | `modalChipText`, `modalSecondaryButtonText` |
| `0xFF1A1A1A` | 2 | `modalChipTextActive`, `modalSheetInputText` |
| `0x33FFFFFF` | 2 | `modalInlineActionBorder`, `modalSearchFieldBorder` |

Three findings from this, in descending order of how much they matter:

1. **The on-glass text ramp does not exist.** `modalTextPrimary`, `Secondary`,
   `Tertiary`, and `Muted` are all solid white. Four names encode a hierarchy
   that renders as one value — so any widget "de-emphasising" text with
   `modalTextMuted` is not de-emphasising anything. Either the ramp gets real
   values (and the gate enforces 4.5:1 / 3:1 on them) or it collapses to one
   token. The render sweep should decide which.
2. **Dividers are the same value as the surface they divide.** `modalSurface`,
   `modalBorderSubtle`, and `modalFooterDivider` are all `0x26FFFFFF`.
3. **The white-alpha ladder has fourteen steps** — `0x14, 0x1A, 0x1F, 0x26,
   0x33, 0x40, 0x4D, 0x59, 0x73, 0x80, 0x8C, 0xD9, 0xE6, 0xFF`. Adjacent steps
   differ by 2 percentage points and are not perceptually distinguishable, but
   each is independently maintainable — which is the mechanism that produced 39
   constants. Black-alpha adds five more (`0x33, 0x59, 0x66, 0x80, 0xBF`).

Two are stale palette leftovers that #2441 missed: `modalSheetInputBackground`
(`0xFFF4EFE4`, cream) and `modalSheetInputPlaceholder` (`0x8C5A4A3D`, warm
brown).

### Chat glass: 11 more consts, on a different base

`lightPillBg`, `darkPillBg`, `darkBubbleBg`, `darkBottomBarBg`,
`lightInputFieldBg`, `darkTintOverlay`, and the four gradient stops are built on
`0x2A2723` / `0x1C1A17` / `0xFDF8F0` — **pre-Ink&Sage warm browns and cream**.
`darkBottomBarBg` and `darkGradientBottom` are also byte-identical
(`0xF21C1A17`). So the app has two glass materials that share no base colour,
and one of them was never migrated to the current palette.

### Target shape (~16 tokens + 4 status)

To be confirmed by the render sweep, not adopted on paper:

**Material:** `glass-scrim`, `glass-scrim-strong`, `glass-surface`,
`glass-blur-sigma`, `glass-backdrop-blur-sigma`
**Structure:** `glass-border`, `glass-divider` (must differ from
`glass-surface`), `glass-fill-subtle` (collapses `0x14`/`0x1A`/`0x1F`),
`glass-fill-strong`
**Foreground:** `glass-text-primary`, `glass-text-secondary`, `glass-text-faint`,
`glass-primary` (the on-glass action colour #2764 needed and did not have),
`glass-on-primary`, `glass-status-{success,warning,error,info}`

The point is not the count. It is that every token in this set has a distinct
job the gate can check, whereas seven aliases for solid white cannot fail a
contrast gate in any way that tells you which caller was wrong.

## Audit notes

Audited against the context map. Fixes applied above; the findings were:

- **LFS pattern missing.** `.gitattributes` has no PNG rule. Committing renders
  before adding one puts ~60MB into ordinary git history. Ordering is
  load-bearing — added as an explicit pre-step in Phase 2.
- **`app_colors.dart` is 905/1000 lines.** Adding the glass ramp there breaches
  `npm run lint:dart:size`. Phase 3 now specifies a sibling file.
- **The analyzer had no tests.** Per the testing rubric's untested-functions and
  edge-case items, a contrast estimator with no unit tests is the one component
  that must not be trusted on inspection. Phase 1 now specifies fixture-based
  tests wired into `npm run test:scripts`.
- **Dependency decision was implicit.** CLAUDE.md caps dependencies at the
  minimum and the tree has no image library; Phase 1 now pins PNG decoding to
  ffmpeg, already used by two existing scripts.
- **CI cost was unbudgeted** against the self-hosted runners; Phase 4 now says so.
- **Ratchet direction.** `scripts/color_allowlist.txt` may shrink but never grow
  on main — noted in Phase 3.
- **No phase labels in Dart comments.** This plan is phase-structured, but
  `scripts/check_ephemeral_comments.js` (`npm run lint:ephemeral-comments`)
  rejects "Phase 3"-style markers in `*.dart`. Implementation comments cite
  `#2770` / `#2764` / `#2445` instead.
- **Accessibility rubric alignment.** The change is squarely §4 *Visual
  Accessibility* ("Color Contrast", "Dark Mode"). Worth noting the rubric's
  neighbouring item — **"Color Independence: verify info isn't conveyed by
  colour alone"** — is *not* addressed by this work and stays a separate gap.

### Found in the first candidate sweep (Phase 2)

72 captures, 10,405 token instances. The result overturned the plan's premise.

- **The glass ramp moves almost nothing.** Failure counts were byte-identical
  across all three ramps for every token except `glass.primary`
  (`text-faint` 246, `text-secondary` 132, `primary` 83, `text-primary` 6). The
  bulk of failures were never on the glass sheet — they were on hero/media
  surfaces governed by the *content-overlay gradients*, which were not
  tokenized and therefore not varying. Choosing a material on that data would
  have been choosing on noise. Hence the overlay tokenization above.
- **No material can rescue a dark foreground.** `glass.primary` improved only
  1.01 → 1.37 → 1.59 across current/deeper/heaviest, still far from 4.5.
  Darkening the sheet makes a *dark* foreground worse, not better. The
  arithmetic says so independently: on the mid-grey composite the blur produces
  over a busy photo, the theoretical maximum is **4.06:1 with pure black** and
  **5.17:1 with pure white**, and any foreground needs luminance ≥ 0.863 —
  lighter than grey 240 — to clear 4.5:1. So **no palette can make a
  brand-coloured foreground legible on a material**; the fix is not to use one
  there, which is exactly Apple's rule and what the lint now enforces.
- **The shipped material is far too weak over bright media.** Worst-case
  composite is grey **175**, where even pure white reaches only **2.19:1**. A
  70% scrim brings that to 94 → 6.48:1.

### Found while implementing Phase 1

- **`env:start` never exits when it boots the Auth Emulator itself.** The
  emulator is spawned as a non-detached child, so Node's event loop never
  drains: `env.json` is written, the server serves, and the command still hangs.
  It returns promptly only when an emulator is *already* live and it can attach —
  which is why the documented `ripls-ui-investigate` flow appears to work. The
  gate therefore waits on observable readiness (state file + a served response)
  rather than on the command returning. Filed as **#2772** — it hangs any
  non-interactive use of the investigation environment, not just this gate.
- **`.ts` under `e2e/scripts/` transforms as CJS**, so top-level `await` is
  rejected — hence `contrast_capture.mts`, matching the existing probe-script
  convention.
- **`openProbe` launches a browser per capture.** Fine for the per-PR set, but
  at ~7s each (launch + settle + shot) the full 438-capture matrix would run
  ~50 min per pass, against the ~20–25 min the budget above assumes. The capture
  loop needs to reuse one browser and open a fresh context per theme before the
  full matrix is run in anger. Not a correctness issue, so it did not block
  Phase 1.
- **Judging needs per-token roles.** The first full run measured 960 token
  instances and reported 385 failures — but the profile was dominated by tokens
  that are not body text: `text-faint` (which `tokens.json` itself documents as
  clearing 3:1, never body copy), `border` (a hairline divider), and `surface` /
  `background`, which are the measurement *reference* rather than subjects.
  Sentineling the reference surfaces also changed nearly every pixel in the
  frame, producing frame-sized masks. Fixed by excluding `background`/`surface`
  from sentineling, tagging each token with a role
  (`text` / `text-faint` / `ui` / `decorative`), and suppressing findings whose
  measured foreground does not match the token's declared value — a thin glyph
  run is mostly anti-aliased edge, so its median blends toward the background
  and understates contrast. That last check only bites when token and background
  are far apart, which is exactly when a blended reading is most misleading.

**Docs consulted:** `docs/llms.txt`; anchors `client/architecture.md`,
`client/conventions.md`, `testing/architecture.md`,
`reviews/accessibility_review_prompt.md`, `reviews/testing_review_prompt.md`,
`reviews/dependency_review_prompt.md`; leaves `client/design.md`,
`client/modals.md`, `client/accessibility.md`, `client/testing.md`,
`client/testing/semantics_identifiers.md`, `walkthroughs.md`; plus
`design/README.md`, `e2e/README.md`, `CLAUDE.md`, `.gitattributes`,
`scripts/check_dart_file_size.sh`. `proto_conventions.md` and the server anchors
were not loaded — no proto or server surface is touched.

---

# Appendix — Evaluation set specification

The surfaces, states, and axes every candidate token set is rendered against,
and the protocol that decides whether a candidate passes. Phase 1 and Phase 2
above implement this.

## Why an evaluation set exists

`design/tokens.json` already gates contrast, but only for token-on-token pairs
against `background` and `surface` — two flat opaque colors. The surfaces where
the app actually fails are the ones the gate cannot express: translucent glass
over an arbitrary user photo. #2764 is a light-theme token (`lightPrimary`,
deep sage `#3E5A47`, chosen to clear 4.5:1 against white) used as a foreground
on a dark glass sheet, where it measures roughly 1.5:1.

The evaluation set is therefore defined by one rule:

> **Every place a token is composited onto something other than `background`
> or `surface` is in the set.**

Flat surfaces are in the set too, but as a *control group* — they prove a
candidate did not fix glass by wrecking the surfaces that already worked.

## Axes

| Axis | Values | Applies to |
| --- | --- | --- |
| **Palette** | 3 candidates | every surface |
| **Glass ramp** | 3 candidates | glass/media surfaces only — a no-op where there is no glass |
| **Theme** | light, dark | every surface |
| **Backdrop** | 3 synthetic + 1 real | media-bearing surfaces only |

Palette and glass ramp are **independent axes**, navigated by separate pickers
on the comparison sheet rather than flattened into a cross product.

**Backdrops.** The three synthetic backdrops — pure white, pure black, and
high-frequency noise — are the *gate*. They bound the worst case a real photo
can produce, so passing them is a guarantee rather than a sample; a user's snow
photo cannot be worse than pure white. The real stock photo is for visual
judgement only and is not gated.

**Candidate directions** (round 1): a separate on-glass token ramp; Ink & Sage
with a re-engineered glass material; a new palette with a dual-surface primary.

## The surface matrix

Routes are deep-linkable (`app/lib/core/router/app_router.dart`), so the sweep
navigates directly to each surface rather than clicking through flows —
deterministic, and it removes flow flake from a 1,000-render job.

### Group A — glass + media core

The surfaces that carry the bug class. If only one group ships, it is this one.
All four axes apply to every row.

| # | Surface | Entry | Tokens under test |
| --- | --- | --- | --- |
| A1 | Content hero over photo | `/gear/:id` | title, chips, status chip, floating actions on bare media |
| A2 | Glass date/time picker | experience → change time | **#2764 verbatim** — `glass_pickers.dart` scheme, today/selected day, Cancel/OK |
| A3 | Glass transfer sheet | transfer modal | `GlassFooterButtons` primary + secondary, inset cards |
| A4 | Chat | `/experience/:id` chat | `pillBg`, `bubbleBg`, `bottomBarBg` — a second glass token family |
| A5 | Time modal / agenda dock | experience time panel | morph-reveal panel over hero, weather strip, host control bar |
| A6 | Bottom nav dock | `/` | opaque dock over feed media, badges |

### Group B — flat surfaces + status states

The control group, and where #2445 lives. Palette × theme only.

| # | Surface | Tokens under test |
| --- | --- | --- |
| B1 | Settings | text ramp, borders, list dividers |
| B2 | Profile | text ramp on surface |
| B3 | Form with validation errors | `error` on field + helper text |
| B4 | Status banners | `success` / `warning` / `error` / `info`, both themes |
| B5 | Empty state | faint text, illustration accents |

### Group C — bespoke materials

Materials the token set never modeled. C1/C2 are palette × theme; C3 takes all
four axes.

| # | Surface | Tokens under test |
| --- | --- | --- |
| C1 | Impact detail modal | `ImpactModalColors` warm paper |
| C2 | Workshop | parchment + hashed `community_color.dart` palette |
| C3 | Location picker | dark-glass pills over a live map |

### Group D — creation & preview

Palette × theme only. Collapsed on the sheet by default.

| # | Surface | Tokens under test |
| --- | --- | --- |
| D1 | Create item preview | fields, primary CTA |
| D2 | Create event preview | seed-need chips |
| D3 | AI streaming preview | in-flight/skeleton states |

## Render budget

Gate renders use synthetic backdrops only; the real-photo renders are for
visual judgement and need no sentinel pass.

| Bucket | Arithmetic | Renders |
| --- | --- | --- |
| A, gated | 6 × 3 palettes × 3 ramps × 2 themes × 3 synthetic | 324 |
| C3, gated | 1 × 3 × 3 × 2 × 3 | 54 |
| B, gated | 5 × 3 × 2 | 30 |
| C1–C2, gated | 2 × 3 × 2 | 12 |
| D, gated | 3 × 3 × 2 | 18 |
| **Gate baseline** | | **438** |
| **Gate sentinel pass** | ×2 (see below) | **438** |
| Visual only (real photo) | (6 + 1) × 3 × 3 × 2 × 1 | 126 |
| **Total** | | **~1,000** |

At roughly 4s per render with warm contexts across 4 workers, the render half of
a full sweep is ~20–25 minutes.

**Builds.** Token values are compile-time constants, and only the palette ×
glass-ramp axes need a build — theme and backdrop are runtime. A measured
rebuild is **2.6 min flat** (`flutter build web --wasm` does no meaningful
incremental reuse for a constant change: 2:37.93 after a one-constant edit vs
2:32.95 cold). So:

- **Exploration:** 9 combos × 2 passes = 18 builds ≈ **47 min**, unattended.
  Full sweep including renders ≈ 70 min.
- **CI gate:** only the shipped token set is checked — 2 builds plus ~98
  renders ≈ **10 min**, and CI already builds the bundle for e2e, so the
  marginal cost is one extra build.

**Only the 126 visual renders are committed** to Git LFS (~60MB). Gate renders
are ephemeral CI artifacts.

## Measurement protocol — sentinel-diff

CanvasKit paints text to canvas and the engine prunes non-interactive nodes
from the accessibility tree (`e2e/README.md` § What's reliably reachable), so
semantics bounding boxes reach `Tappable`/`Toggle`/`IconAction` but miss body
copy, headings, timestamps, and most status text — precisely where #2445 lives.
Sentinel-diff has no semantics dependency and nothing to keep annotated.

1. Render the surface normally. This is the **baseline**.
2. Re-render with **every token overridden to a distinct sentinel color**,
   hues chosen maximally separated in a perceptual space. Sentineling one token
   per render would cost (1+N)× renders; sentineling all of them at once keeps
   the pass at exactly 2× and still yields a per-token mask.
3. Diff. Each changed pixel is classified to a token by nearest sentinel hue.
   Use only high-confidence core pixels — glyph edges are anti-aliased blends
   of sentinel and background and must not be classified.
4. For each token's mask, foreground = the **baseline** color at those pixels;
   background = the median of unchanged pixels in the surrounding neighborhood,
   which is the true composited backdrop with glass, blur, scrim and photo
   already baked in.
5. Report the contrast ratio per token, per surface, per axis combination.

**Connected-component segmentation is required, not optional.** A token used
against two different backgrounds — `modalTextPrimary` appears both on the bare
sheet and as the label inside a filled pill — yields a single merged mask, and
the median collapses to whichever context has more pixels. Measured on
CanvasKit: the pill instance was averaged away entirely and reported the
sheet's background. Each contiguous glyph run must therefore be segmented and
given its own ring background before ratios are computed.

**Background estimation must use a ring, and must exclude only the token's own
pixels.** Sampling "all unchanged pixels in the bounding box" reports the sheet
when the text actually sits on a fill (measured: 2.97 against a true 2.41). A
ring around each component, read from the *baseline* render and excluding only
pixels belonging to that same token, recovers the fill exactly (2.41).
Neighbouring pixels that belong to a *different* token must be kept — when text
sits on a filled pill, that fill is the background.

### The method's blind spot: sentinel-diff can only see tokens

The pass rate is over **tokenized pixels**, not over the screen. A colour no
token owns is not recoloured in the sentinel pass, so it is absent from both the
numerator and the denominator — it does not fail, it is never examined. Silence
about a pixel is not a pass.

Confirmed on `gear-detail-hero-light-white`, by diffing the two passes
region-by-region rather than by eye:

| element | pixels changed | max delta | seen by the gate |
| --- | --- | --- | --- |
| `WHERE` label, `TBD`, `View details` | 100% | 265–335 | yes |
| title (glyphs only within bbox) | 35% | 256 | yes |
| **"See the calendar" CTA** | 5% | **17** | no — below threshold |
| **"Share" button** | **0%** | **0** | no |

The CTA is `gear_who_card.dart:48-49`, two private literals
(`_rsvp = 0xFFA7C59E`, `_rsvpInk = 0xFF23351C`). `lint:dart:colors`
(`check_no_hardcoded_accent.js`) does not catch them: it blocks an enumerated
list of ~12 coral/sage values, so an arbitrary green is legal and unmonitored.

Scope of the blind spot: **518** raw `Color(0x…)` literals in `app/lib` outside
`core/theme/`, of which **146 across 52 files** are in widgets that also render
glass or media. Whether each is a defect is unknown — that is the point. The
gate cannot answer it.

This does not invalidate the sweep: the token ramps are what the sweep varies,
and those are measured correctly. It bounds the claim. "0.55% of instances fail"
must be read as *of tokenized instances*, and no candidate should be described
as clean on a surface until its untokenized colours are either tokenized or
inspected by eye.

Follow-up: widen `lint:dart:colors` from an enumerated denylist to "no raw
`Color(0x…)` outside `core/theme/`, ratcheted", which converts this from an
invisible hole into a shrinking list.

## De-risk results

Prototyped before planning. Ground truth is analytic: over a flat backdrop `B`
the glass stack composites to `0.85 · (0.65 · B) + 38.25`.

**The measurement is accurate, on both renderers.** Deep sage `#3E5A47` as an
on-glass foreground:

| Backdrop | Analytic | DOM compositor | CanvasKit / wasm |
| --- | --- | --- | --- |
| dark `#1A1A1A` | 1.62 | 1.64 | **1.64** |
| white | 3.64 | 3.63 | **3.63** |

Every token in the set landed within ~1% on both. CanvasKit's `ImageFilter.blur`
and compositing are equivalent to the DOM compositor for this technique, so the
gate is safe to run against the production wasm bundle. **1.64:1 is #2764**, and
it was recovered from pixels alone with no semantics involvement.

**It is strictly better than flat token math.** Alpha-composited foregrounds are
measured as rendered: a 55%-alpha white over the dark composite genuinely paints
as gray 163 (`0.55·255 + 0.45·52.6 = 163.9`) and the estimator returned 163.
Token-level math would have scored it as pure white.

**One sentinel pass covers the whole token set.** Tested to 48 simultaneous
tokens: per-pixel classification degrades to ~79% mean at that count, but the
misclassified pixels are anti-aliased edges, and the strict core filter plus a
robust median recovered all 48 foregrounds exactly (worst error 0). No
multi-pass encoding is needed; 2× renders holds.

**The runtime override channel was measured and rejected.** Converting 12 of the
~40 glass tokens from `static const` to resolved getters produced **118 compile
errors across 31 files** — dropping `const` from an inner expression forces the
whole enclosing const widget tree to become non-const, cascading upward. It also
required an API sentinel (`GlassSurface.useTokenBorder`) because a const
constructor's default value must itself be const. Against a measured 2.6 min
rebuild, the override does not pay for itself; the harness rebuilds instead.

**Incidental finding that supports the chosen direction.** On the
high-frequency-noise backdrop the blur averages to ~109 gray, and the brand and
status ramp collapses: sage 1.46, `info` 1.24, `success` 1.16, `warning` 1.06,
`error` 1.03. Over a busy mid-tone photo, essentially the entire non-white ramp
is invisible on glass regardless of palette — evidence that the material, not
the palette, is the primary defect.

## Pass criteria

- **Gate:** 4.5:1 for normal text, 3:1 for large text and UI component
  boundaries, measured on composited pixels, across all three synthetic
  backdrops in both themes. A candidate fails on its worst case, not its mean.
- **Visual:** the side-by-side sheet decides aesthetics among candidates that
  pass the gate. Measurement never picks the winner on its own — it only
  eliminates.

## Comparison sheet

**Rebuild required (Bryan, 2026-07-26).** The first cut was not reviewable: it
stacks captures vertically, so comparing two candidates means scrolling between
them and holding the difference in memory. Options must sit **side by side**.

- **Grid, not list.** Ramp and palette are the axes — rows and columns — so a
  row reads as "same palette, four materials" and a column as "same material,
  three palettes". That is the comparison the sweep exists to support.
- **Dropdowns for the other three axes**: screen, theme, backdrop. They select
  which grid is shown; they are not additional rows.
- **Real photographs as backdrops, one light and one dark.** The synthetic
  white/black/noise fields bound the worst case and are right for *gating*, but
  under a wash they are flat grey rectangles — they cannot show whether a
  photograph still looks like a photograph, which is the judgement being asked
  for here. Keep synthetic for the gate, use real photos for the sheet.

`docs/concepts/theme/` (precedent: `accent-color-variations.html`,
`unification-choice-sheet.html`), generated from the captures.

- Independent pickers for palette and glass ramp.
- Theme and backdrop as visible toggles.
- Groups A and B expanded; C and D behind disclosure, per the review that rows
  per color set should not overwhelm.
- Measured contrast shown inline per surface, with failures marked.

## Consequences

- The glass material moves into `design/tokens.json` — on-glass colors, alphas,
  blur sigma, scrim. This reverses the exclusion currently stated in
  `design/README.md`, which is why ~40 constants drifted outside every gate.
- `AppColors` constants stay `const`. The ~730 `AppColors.modal*` call sites
  still migrate onto the new glass ramp, but const-to-const substitution keeps
  that a rename rather than the de-const cascade the override would have forced.
- `docs/client/accessibility.md` has no contrast section today, and
  `docs/reviews/accessibility_review_prompt.md:71` asks reviewers to verify
  4.5:1 by eye with no mechanism. Both get pointed at the gate.
- The walkthrough reels published in #2684 / #2687 / #2703 / #2708 are rendered
  with the current palette and need re-rendering if the palette moves.
