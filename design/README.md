# design/ — canonical design tokens

The single source of truth for Ripls brand/neutral/semantic colors and font
families across the Flutter app, the server-rendered web pages, and the
marketing site (issue #2441). Palette: **Ink & Sage**; fonts: **Libre
Baskerville** (display) + **Public Sans** (body/UI).

## Files

| File | Role |
| --- | --- |
| `tokens.json` | **The source of truth** (W3C Design Tokens format). The only place hex values and font families are defined. |
| `guidelines.md` | Short prose guidelines, inlined into the generated HTML page. |
| `guidelines.html` | **Generated** — human-viewable guidelines: swatches, type specimens, light/dark side by side, contrast table. Do not edit. |

Generated consumers elsewhere in the tree (also do-not-edit):

- `app/lib/core/theme/gen/design_tokens.gen.dart` — raw constants for `AppColors`/`AppTheme`
- `app/lib/core/theme/gen/glass_tokens.gen.dart` — the frosted-glass material
- `app/lib/core/theme/gen/overlay_tokens.gen.dart` — the media-overlay material
- `website/content/css/gen/tokens.gen.css` — CSS custom properties for all web surfaces

## Changing the tokens

1. Edit `tokens.json` (and `guidelines.md` if the prose changes).
2. `npm run generate:design-tokens`
3. Commit the JSON **together with** the regenerated outputs.

CI runs `npm run lint:design-tokens` (regenerate-and-diff): any drift between
`tokens.json` and a generated file fails the build. Generation itself fails if
a declared contrast pair (see `CONTRAST_PAIRS` in
`scripts/gen_design_tokens.js`) drops below WCAG thresholds — the palette
cannot regress below AA by construction.

## Materials (#2770)

Beyond the palette, `tokens.json` defines the **materials** — translucent values
painted over content the palette cannot predict:

| Block | Generated to | What it is |
| --- | --- | --- |
| `glass` | `app/lib/core/theme/gen/glass_tokens.gen.dart` | Frosted sheets and modals floating above content |
| `overlay` | `app/lib/core/theme/gen/overlay_tokens.gen.dart` | Scrims and controls painted onto user photos |

Two things make them unlike the palette: **alpha is load-bearing** (values are
`#AARRGGBB`), and they are **not per-theme** — the backdrop, not the app theme,
decides what is behind them.

They are here because the #2441 contrast gate structurally could not see them.
It checks token-on-token pairs against `background` and `surface`, two flat
opaque colours, while these surfaces composite over arbitrary media. Left
outside the token set they accumulated **52 hand-tuned glass constants** (~34
distinct values, seven names for solid white, a divider byte-identical to the
surface it divided) and **four inline gradients** for media that faded to fully
transparent exactly where the text sat. #2764 — deep sage measured at 1.01:1 as
an on-glass foreground — is what that produced.

Adding a material means adding an entry to `MATERIALS` in
`scripts/gen_design_tokens.js`; validation, Dart emission, sentinel selection
and contrast roles all derive from that table.

## What does NOT belong here

Content-type colors, chat bubbles, spacing, radii, and component specs are
app-level concerns derived from these tokens — they live in
`app/lib/core/theme/` and are documented in `docs/client/design.md`. The token
set stays minimal on purpose; materials earned their place by being measurable
surfaces the gate must check, not by being visual detail.

## Provenance

Palette and font pairing were selected from the Phase 1 choice sheet
(`docs/concepts/theme/unification-choice-sheet.html`); the Ink & Sage light
theme comes verbatim from PR #2440's palette explorer, the dark theme was
synthesized during #2441. Plan: `docs/issues/2441-visual-token-unification.md`.
