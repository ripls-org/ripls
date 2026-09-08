# Ripls visual guidelines

One palette, one font pair, everywhere. The app, the server-rendered web
pages, and the marketing site all draw from the same token set — **Ink &
Sage**: white and near-black neutrals with a faint green cast, deep sage
reserved for actions, and ink as the emphasis accent. Selected in
issue #2441 from the Phase 1 choice sheet.

## Color

- **Sage owns actions.** `primary` is the only fill for buttons and primary
  CTAs; `on-primary` is the only text/icon color on it. Don't use sage for
  decoration — if everything is sage, nothing is.
- **Ink is the emphasis accent.** Headline accent words, marketing band
  sections, and highlights use `accent` — rendered as color plus underline,
  **never italics**.
- **Neutrals do the work.** Page on `background`, cards on `surface` with
  `border` — no shadows-as-borders. Text steps down `text-primary` →
  `text-secondary` → `text-faint`; faint is for placeholders and footnotes
  only (it clears 3:1, not 4.5:1).
- **Status colors are per-theme.** Success/warning/error/info have different
  values in light and dark — use the theme's set, never hard-code one. All
  pass 4.5:1 as text on both `background` and `surface`.
- **Not allowed:** warm cream/off-white fields (the `#F4F1EA` family),
  terracotta/amber brand accents, and any raw hex that isn't in
  `tokens.json`. Content-type colors, chat bubbles, and the glass modal
  material are app-level concerns layered on top — see
  `docs/client/design.md`.

## Type

- **Libre Baskerville** for display and headings, weights 400 and 700 only.
  No italic display type.
- **Public Sans** for body, labels, and UI, weights 400–700, falling back to
  the platform system sans.
- Body line-height stays at 1.7; headings at 1.1–1.3.

## Changing the tokens

Edit `design/tokens.json` — nothing else. Then run
`npm run generate:design-tokens` and commit the JSON together with the
regenerated outputs (Dart, CSS, this page's HTML). CI
(`npm run lint:design-tokens`) fails on any drift between them, and
generation itself fails if any declared contrast pair drops below WCAG
thresholds.
