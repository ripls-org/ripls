# Issue #2441: Visual unification: minimal color/font token set as single source of truth for app + websites + marketing

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2441
**Filed:** 2026-06-11 | **Last activity:** 2026-06-11
**Labels:** none | **Assignees:** unassigned
**Context docs:** `docs/llms.txt`, `docs/client/design.md`, `docs/client/conventions.md`, `docs/client/architecture.md` (colors/theming section), `docs/reviews/accessibility_review_prompt.md` (contrast areas); plus codebase research: `app/lib/core/theme/`, `website/content/css/{tokens,marketing,shared}.css`, `scripts/check_no_hardcoded_accent.js`, `scripts/gen_context_map.js`, PR #2440's palette explorer (`website/content/colors/index.html` on `website_style_restage`)

## Problem Statement

Brand colors and fonts are defined independently in three places, with no programmatic link and only manual discipline holding them together:

1. **Flutter app** — `app/lib/core/theme/app_colors.dart` (~34 KB of `Color(0xFF...)` constants with theme-aware getters) + `app_theme.dart` (Libre Baskerville headings bundled as assets, system sans body). This is the de-facto source of truth; `npm run lint:dart:colors` ratchets accent-hex discipline *within* the app only.
2. **Server-rendered web pages** (event landing, invite deep-link, privacy) — `website/content/css/tokens.css`, whose own header comment says it is *hand-mirrored* from `app_colors.dart` and flags the drift risk. A codegen follow-up was already promised in the #2049 plan and never built.
3. **Marketing pages** (`/`, /moms/, /kids/, /simple/, /crew/, /green/, /builders/, /social/) — `marketing.css` defines a third, independent system: Newsreader + Public Sans from Google Fonts CDN, its own `:root` sage variables, cream sections (`#F5ECE1`), and dark photo overlays. `shared.css` still carries the legacy coral `#E07A5F`.

That's three serif choices in flight (Libre Baskerville in app + tokens.css, Newsreader on marketing, more candidates in the PR #2440 explorations) and two-and-a-half color systems. On top of the drift problem, the issue declares an aesthetic direction change: move off the "Claude house style" (warm cream `~#F4F1EA`-family backgrounds, conventional serif display, italic `<em>` word-accents, terracotta) — which explicitly puts shipped surfaces (marketing cream sections, possibly the app's warm-brown dark neutrals) in scope for change once a palette is chosen.

The goal is one minimal canonical token set — machine-readable and human-viewable — consumed by all three surfaces, with CI gates that make drift impossible rather than merely discouraged.

The issue is unusually well-specified (Bryan authored it with a 4-phase plan, a proposed token inventory, and explicit anti-constraints). This plan grounds that outline in the codebase, resolves the engineering forks the issue leaves open, and sequences the work.

## Context from Comments

No comments. Normally a flag, but the issue body is effectively its own design discussion (proposed token set "to be confirmed during design", named palette favorites, structural model for the choice sheet). The one genuinely open product decision — which palette + font pairing — is deliberately deferred to the Phase 1 choice sheet, which exists to extract that decision from Bryan.

## Codebase Findings That Shape the Plan

- **Precedent for the drift gate already exists**: `docs/llms.txt` is a *generated, checked-in* artifact with a regenerate-and-diff CI gate (`npm run lint:context-map`, `scripts/gen_context_map.js`, `.github/workflows/check_context_map.yaml`). The token pipeline should clone this pattern, not the proto pattern (proto codegen output is *not* checked in — but the website is static nginx-served with no build step, so generated CSS **must** be checked in; consistency then argues for checking in the generated Dart too, with the same diff gate).
- **The color ratchet is reusable**: `scripts/check_no_hardcoded_accent.js` + `scripts/color_allowlist.txt` (currently empty) already implement shrink-only allowlisting for Dart. The web-side hex lint in Phase 4 is the same shape pointed at `website/content/**/*.{css,html}`.
- **PR #2440's 20 palettes are single-theme** (`html[data-pal=...]` CSS variable blocks). The three favorites — `inksage` (light), `deepriver` (dark), `plum` (dark) — each need a synthesized counterpart theme before they can be compared as full light+dark token sets. The choice sheet must do this synthesis; it is the main creative work of Phase 1.
- **The explorer's token vocabulary** (`--bg/--bg-2/--card/--ink/--ink-2/--brand/--accent/--accent-hover/--on-accent/--band/...`) maps cleanly onto the issue's proposed minimal set (background ramp, text ramp, primary/on-primary, accent) — missing only border/divider and the semantic status colors, which the issue proposes adding.
- **Fonts**: the app bundles Libre Baskerville TTFs as assets; marketing loads Newsreader/Public Sans from Google Fonts CDN. Whatever pairing is chosen, the loading strategy should unify (see Open Questions).
- **Glass-modal tokens are out of scope by design**: `docs/client/design.md` documents the glass material as intentionally theme-invariant (white-at-alpha on translucent sheet). Those `AppColors.modal*` alpha tokens derive from no palette and stay app-owned, consistent with the issue's "deliberately excluded" list.

## Potential Solutions

The palette/font *selection* is Phase 1's output, not an engineering fork. The real forks are the SSOT format and the codegen tooling.

### Option A: W3C Design Tokens JSON + hand-rolled Node generator (llms.txt pattern)

- **Approach:** `design/tokens.json` in W3C Design Tokens Community Group format (`$value`/`$type`/`$description`, group nesting for `light`/`dark`/`semantic`/`font`). A single dependency-free Node script `scripts/gen_design_tokens.js` reads it and emits: `app/lib/core/theme/gen/design_tokens.g.dart` (raw `Color` constants only), `website/content/css/gen/tokens.gen.css` (custom-property blocks for light + `prefers-color-scheme: dark`), and the HTML guidelines page. Wired as `npm run generate:design-tokens` + `npm run lint:design-tokens` (regenerate-and-diff), mirroring `generate:context-map`/`lint:context-map` exactly.
- **Pros:** Zero new dependencies (matches the project's minimal-deps rule); the W3C format is a real spec so future tooling (Figma plugins, Style Dictionary if ever wanted) can consume the same file; the generator is small enough to also run contrast checks at generation time; the CI pattern is already proven in this repo.
- **Cons:** ~300–400 lines of bespoke generator to write and unit-test; W3C format is slightly more ceremonious than a flat map.
- **Effort:** Medium.

### Option B: Style Dictionary (off-the-shelf token build)

- **Approach:** Adopt Amazon's `style-dictionary` npm package; author tokens in its format; configure transforms/formats for Dart, CSS, and HTML outputs.
- **Pros:** Battle-tested multi-platform output; formats and transform plumbing for free; docs-site generators exist in its ecosystem.
- **Cons:** A real dependency (plus its transitive tree) for what is ultimately ~30 tokens and 3 output targets; its Dart output needs a custom format anyway (no first-class Flutter format matching `AppColors` conventions); custom HTML guidelines page is custom work regardless; contradicts the "minimal dependencies" project rule for little leverage at this scale.
- **Effort:** Medium (less generator code, more configuration + dependency surface).

### Option C: textproto SSOT (estimator-config pattern)

- **Approach:** Define a `DesignTokens` proto and a `design/tokens.textproto`, following `server/impact/estimator/config.textproto`; generate Dart/CSS from it via a Go or Node tool.
- **Pros:** Schema validation for free; consistent with how the server holds config.
- **Cons:** Wrong audience — this file is edited by a human making design decisions and consumed by web/Flutter tooling, not the Go server; proto toolchain involvement for a pure front-end artifact adds friction; no ecosystem benefit (design tools speak JSON, not textproto).
- **Effort:** Medium-large.

## Decision (2026-06-12)

**Palette: Ink & Sage** (light theme verbatim from PR #2440's explorer; dark theme as synthesized on the choice sheet). **Fonts: Libre Baskerville + Public Sans.** Selected by Bryan from the Phase 1 choice sheet. Consequences locked in with the selection:

- **Status colors are per-theme** in `tokens.json` (the choice-sheet contrast badges proved shared values can't pass on light): light gets a new set (`#2E7041 / #8A6200 / #B5492B / #3E6471`); dark keeps success/warning and lightens error→`#DB7F63`, info→`#7FA0A9`.
- **The app's warm-brown dark neutrals are replaced** by the Ink & Sage dark ramp (`#141714 / #1F2421 / #323832`) at Phase 3 adoption.
- **Naming change from the original plan:** the generated Dart file is `design_tokens.gen.dart`, not `*.g.dart` — `npm run clean` deletes all `*.g.dart` files as build_runner outputs, which would wipe a checked-in generated file.

## Recommendation

**Option A.** The repo already has the exact CI pattern this needs (`gen_context_map.js` + checked-in generated artifact + regenerate-and-diff gate), the token count is small enough that a bespoke generator is less total surface than a dependency, and W3C-format JSON keeps the door open to standard tooling later. Style Dictionary earns its keep at hundreds of tokens across many platforms; at ~30 tokens × 3 outputs it's dependency weight without leverage.

Within Option A, the key structural decisions:

- **Generated outputs are checked in** and gated by regenerate-and-diff (llms.txt precedent). This is forced for the website (static nginx serving, no build step) and adopted for Dart for consistency. The Dart output goes in `app/lib/core/theme/gen/` and is excluded from the line-count and color lints the way `gen/` already is; `app_colors.dart` keeps the semantic layer (`lightPrimary = DesignTokens.lightPrimary`, theme-aware getters, content-type colors, glass alphas) and loses its raw brand/neutral/semantic hex literals.
- **The generator enforces contrast at generation time**: declared pairs (text ramp on background ramp, on-primary on primary, on-accent on accent, status colors on both backgrounds) are checked against WCAG thresholds and generation fails below them. This turns the #1934-style contrast rationale from a PR comment into a permanent gate, and directly serves the accessibility review dimension this change engages.
- **Token inventory** = exactly the issue's proposed minimal set: per-theme background ramp (page, raised), text ramp (primary, secondary, faint), border/divider, primary + on-primary, accent + on-accent; shared semantic status (success/warning/error/info); one serif + one sans with weights and fallback stacks. Nothing else — content-type colors, chat bubbles, spacing, radii, and glass material remain app-level concerns in `app_colors.dart`/`app_theme.dart`/`design.md`, re-pointed at the tokens for any raw values they derive from.

## Open Questions

- **How do single-theme explorer palettes become light+dark candidates?** The three favorites are one-theme each (`inksage` light; `deepriver`/`plum` dark). — Recommendation: the choice sheet synthesizes the counterpart theme for each candidate (e.g. an `inksage` dark built from its band/ink values; a `deepriver` light keeping sea-sage as primary on near-white), and presents only complete light+dark pairs. This is Phase 1's core design work and should be labeled as synthesized so Bryan knows what's original vs. derived.
- **Font loading on the web: self-host or Google Fonts CDN?** Marketing currently hits the CDN; the app bundles TTFs. — Recommendation: self-host woff2 under `website/content/fonts/` for the chosen pair. It removes a third-party request (GDPR/privacy noise, render-blocking variance), makes the font-family lint clean (no CDN URL exceptions), and matches the app's bundled-asset approach. The font files become part of the generated/managed token surface.
- **Does the app's warm-brown dark neutral ramp change in this issue?** The issue says "possibly... in scope to change once the palette is chosen." — Recommendation: yes, decide it on the choice sheet. The dark neutral ramp is part of every palette candidate's dark theme, so the selection implicitly answers it; Phase 3 app adoption then lands whatever was chosen. Avoid a half-state where web adopts new dark neutrals and the app keeps `#1A1512`.
- **What happens to the #2049-promised tokens.css codegen follow-up?** — Recommendation: this issue subsumes it. Note that in `tokens.css`'s replacement header and in the #2049 plan doc when Phase 3 lands.
- **Do semantic status colors get per-theme variants?** The issue says shared across themes; current app practice agrees, but `#E8A661` warning on a near-white `inksage` background vs. on `plum` dark may need luminance tweaks. — Recommendation: start shared (4 values) per the issue; let the generator's contrast gate prove whether shared values pass on both themes — if a value fails, that's the evidence for promoting status colors to per-theme, and the schema should tolerate that without breaking consumers.
- **Should `lint:dart:colors` eventually block *all* raw hex outside `theme/`, not just accent-family hex?** The architecture doc already prohibits it in prose. — Recommendation: yes, as the Phase 4 ratchet extension — seed the allowlist with the current violation set, shrink-only from there. Keep it a ratchet, not a flag-day.

## Implementation Plan

### Phase 1: Choice sheet (decision artifact for Bryan)
**Goal:** A single HTML page from which Bryan can pick one palette (as a full light+dark pair) and one serif+sans pairing.
**Tasks:**
- [x] Extract the candidate palette definitions from `origin/website_style_restage` (`inksage`, `deepriver`, `plum`) plus 1–2 strong outliers from the remaining 17 for contrast — chose `glacier` (cool light) and `evergreen` (tonal dark); a sixth candidate, Evergreen & Amber, added on request — a scoped exception allowing a gold/honey second (hue ≈41°, distinct from coral ≈13°)
- [x] Synthesize the missing theme for each single-theme candidate so every candidate is a complete light+dark token set (background/text/border ramps, primary/on-primary, accent/on-accent, status colors)
- [x] Pick 3–4 serif+sans pairings satisfying the anti-cliché constraint — Libre Baskerville + Public Sans (app incumbent), Newsreader + Public Sans (marketing incumbent), Spectral + IBM Plex Sans, Besley + Schibsted Grotesk; each carries a re-justification note in the sheet
- [x] Build `docs/concepts/theme/unification-choice-sheet.html` modeled on `design-languages-sage.html`: each candidate rendered in context (marketing hero sample, app-style card, primary/secondary buttons, body text, status banners) in both themes, with every font pairing toggleable per palette
- [x] Add the "non-goals strip": the current Claude-style look (cream `#F5ECE1` section, italic `<em>` accent, terracotta) for visible contrast
- [x] Annotate each candidate with computed WCAG contrast ratios for the load-bearing pairs, so a candidate that can't pass is visibly disqualified before selection — all 10 theme panels pass every core pair; the shared status set fails warning (<3:1) on every light theme (pre-existing in the shipped app), evidence for per-theme status variants
**Validates:** That at least one candidate produces a complete, contrast-passing light+dark system; extracts the palette + font decision that everything downstream depends on.

### Phase 2: Canonical tokens + generator + guidelines
**Goal:** `design/tokens.json` exists, encodes the selection, and is the only place brand/neutral/semantic hex and font families are defined; generated artifacts cannot drift from it.
**Tasks:**
- [x] Author `design/tokens.json` (W3C Design Tokens format) with the selected palette + fonts, per-theme color groups + `font` with `$description` usage notes per token
- [x] Write `scripts/gen_design_tokens.js` (dependency-free, modeled on `gen_context_map.js`): emits `app/lib/core/theme/gen/design_tokens.gen.dart` (`.gen.dart`, not `.g.dart` — see Decision), `website/content/css/gen/tokens.gen.css`, and `design/guidelines.html`
- [x] Generator computes WCAG contrast for declared token pairs and fails generation below thresholds (4.5:1 normal text, 3:1 large text/UI) — 23 pairs per theme set in `CONTRAST_PAIRS`
- [x] Generated guidelines page: every swatch with hex + usage note, type specimens at display/body sizes, light/dark side by side, contrast table, plus prose from `design/guidelines.md`
- [x] `design/README.md` covering purpose, the file inventory (`tokens.json`, generated outputs), and the change workflow (edit JSON → `npm run generate:design-tokens` → commit JSON + generated files together); plus `app/lib/core/theme/gen/README.md`
- [x] `npm run generate:design-tokens` + `npm run lint:design-tokens` (regenerate-and-diff); wired into `npm run lint` and a dedicated `.github/workflows/check_design_tokens.yaml` (intended as a required check, like the context-map gate)
- [x] Unit tests for the generator (`scripts/gen_design_tokens.test.js`, runs under `npm run test:scripts`): contrast math, validation failures, gate enforcement, Dart/CSS emission, determinism, on-disk freshness
**Validates:** The SSOT round-trips to all three output forms deterministically; the contrast gate works; CI fails on a hand-edit to any generated file.

### Phase 3: Adoption
**Goal:** App, tokens.css consumers, and marketing pages all render from generated values; the Claude-style elements are gone from shipped surfaces.
**Tasks:**
- [x] **App:** `app_colors.dart` brand/neutral/semantic constants re-pointed at `design_tokens.gen.dart`; theme-aware `statusSuccess/Warning/Error/Info` getters added (legacy shared consts → dark token values; call-site migration tracked in #2445); `app_theme.dart` headingFont reads `DesignTokens.serifFamily`; body realizes the sans token via platform fallback (Public Sans not bundled in-app); component-level colors stay hand-written; 11 `fontFamily:` literals (10× Georgia — banned family — + 1 LB) replaced with `AppTheme.headingFont`
- [x] **Server-rendered web:** `tokens.css` is now a thin derived layer importing the generated `gen/tokens.gen.css` (legacy variable names preserved as aliases; item-type accents and type/spacing scales stay as derived extras); stale cream/brown literals in `event.css`/`deeplink.css` replaced with token vars/color-mix; the generator additionally emits fixed-theme `--light-*`/`--dark-*` variables for surfaces that don't follow the OS scheme
- [x] **Marketing:** `marketing.css` `:root` aliases the fixed-theme generated variables (photo overlays always dark); cream `#F5ECE1` sections → token surface; serif display italics → normal; headline/lede `<em>` accents → sage color + underline; Newsreader → Libre Baskerville; weight-300 → 400 (self-hosted files carry 400–700); `shared.css` coral/cream swept onto tokens
- [x] Fonts self-hosted: woff2 under `website/content/fonts/` + `css/fonts.css` `@font-face`; OFL license files shipped alongside; every Google Fonts CDN link replaced across marketing/release-notes/legal/invite pages and server event/invite templates (choice sheet keeps CDN for multi-family preview)
- [x] Visual pass: homescreen hero + light/closing sections rendered and screenshot-verified (LB, sage `#3E5A47`, surface `#F2F2EE`, underline accents); guidelines page verified; event/invite pages static-checked (legacy var names preserved)
- [x] **Docs:** `docs/client/design.md` palette/typography slimmed to defer to `design/tokens.json` + guidelines; freshness stamp advanced; #2049 hand-mirroring caveat resolved; `docs/llms.txt` regenerated
**Validates:** Acceptance criteria #4 and #6 — no shipped surface defines its own brand hex; the anti-constraint items (cream sections, italic accents) are demonstrably gone.

### Phase 4: Enforcement
**Goal:** Drift is a CI failure, not a code-review catch.
**Tasks:**
- [x] Web hex lint: `scripts/check_web_colors.js` (`npm run lint:web:colors`) — raw hex/rgb/hsl in `website/content/**/*.{css,html}` (CSS files, `<style>` blocks, inline `style=`, SVG `fill=`/`stroke=`) outside generated files; pure black/white at any alpha exempt (overlays are functional); ratchet allowlist `scripts/web_style_allowlist.txt` seeded with 12 file-level entries, shrink-only (clones the `check_no_hardcoded_accent.js` mechanics)
- [x] `lint:dart:colors` extended with 14 Ink & Sage token hex values (brand, neutral ramp, per-theme status), excluding `app/lib/core/theme/`
- [x] Font-family lint: web side folded into `check_web_colors.js` (`no_off_token_font` — token families + fallbacks + monospace generics only); app side folded into `check_no_hardcoded_accent.js` (`no_font_family_literal` — any `fontFamily:` string literal outside theme/; all 11 pre-existing violations fixed rather than allowlisted)
- [x] Unit tests: `scripts/check_web_colors.test.js` (8 tests — exemptions, style-region extraction, font rule, line numbers, repo-clean gate) + extended inline self-tests in `check_no_hardcoded_accent.js`
- [x] All gates wired into `npm run lint`; `check_design_tokens.yaml` workflow runs `lint:design-tokens` + `lint:web:colors` + the unit tests, dependency-free (pure Node, no npm install)
- [x] `docs/client/design.md` and `design/README.md` describe the enforcement workflow; `design.md` `context:` block valid (`lint:context-map` green)
**Validates:** Acceptance criterion #5 — introduce a deliberate drift (hand-edit generated CSS, add a raw hex to a marketing page, add a rogue font-family) and confirm each fails CI.

## Related Issues
- #2440 — Draft PR: 20-palette + 6-direction style explorations (`website_style_restage`); source of the palette candidates. Stays a parallel exploration artifact; the choice sheet extracts from it.
- #2438 — Merged PR: design-language/accent concept sheets in `docs/concepts/theme/`; structural model for the Phase 1 choice sheet.
- #1946 — Heritage Sage accent migration; established the `lint:dart:colors` ratchet this plan extends.
- #1934 — Contrast rationale for white-on-sage; the precedent the generator's contrast gate generalizes.
- #2049 — Web event landing plan that first promised tokens.css codegen; subsumed by Phase 2–3 here.
- #2439/#2432 — The accidental merge + revert of earlier style explorations; cautionary context for keeping exploration branches (#2440) clearly separated from the canonical token work.

## Audit Notes (/ripls-audit, 2026-06-11)

Changes applied by the audit:

- **Generated guidelines page moved** from `docs/concepts/theme/guidelines.html` to `design/guidelines.html` — generated artifacts shouldn't mingle with the hand-authored exploration sheets in `docs/concepts/theme/`; co-locating with `tokens.json` keeps the generated surface one directory.
- **`design/README.md` made a firm Phase 2 task** (was a "possibly" in Phase 4) — new multi-file directory, and the change-workflow documentation belongs with the artifact from day one.
- **Font-license verification task added** to Phase 3 — self-hosting woff2 requires confirming OFL (or equivalent) terms and shipping license files with the assets.
- **Unit tests for the new/extended lint scripts added** to Phase 4 — the plan tested the generator but not the enforcement scripts; CLAUDE.md's always-test rule applies to both.
- **Phase 4 docs task tightened** to include keeping `docs/client/design.md`'s `context:` frontmatter valid for `lint:context-map` when its palette sections are slimmed.

Checked and found compliant (no change needed): no new dependencies (Option A explicitly rejects Style Dictionary, satisfying the minimal-deps rule); checked-in generated outputs justified against the proto convention via the `docs/llms.txt` regenerate-and-diff precedent and the static-nginx forcing function; `AppColors` semantic layer preserved per `client/architecture.md`'s theming rules; WCAG contrast gates at generation time plus choice-sheet annotation satisfy the accessibility rubric's contrast areas; glass-modal theme-invariant exception respected; `*.g.dart` naming keeps the generated Dart inside existing lint exclusions; plan doc lives in `docs/issues/` (excluded from the context map, no frontmatter needed).

Docs consulted: `docs/llms.txt`; anchors `docs/client/architecture.md` (colors/theming), `docs/client/conventions.md`; leaf `docs/client/design.md`; rubrics `docs/reviews/accessibility_review_prompt.md`, `code_quality_review_prompt.md`, `testing_review_prompt.md`, `architecture_review_prompt.md`, `dependency_review_prompt.md`; plus `CLAUDE.md` (generated-code, minimal-deps, README, testing, TODO rules).
