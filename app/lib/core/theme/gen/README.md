# theme/gen/ — generated design-token constants

`design_tokens.gen.dart` is **generated** from `design/tokens.json` by
`npm run generate:design-tokens` and checked in; CI
(`npm run lint:design-tokens`) fails if it drifts from the source. Do not
edit it — edit `design/tokens.json` and regenerate.

This directory holds raw token values only (colors, font families). The
semantic, theme-aware layer on top — `AppColors` getters, content-type
colors, glass modal material — stays hand-written in the parent
`app/lib/core/theme/` directory; widgets should consume that layer, not
these constants directly.

The file is named `*.gen.dart` (not `*.g.dart`) deliberately: `npm run clean`
deletes all `*.g.dart` files as build_runner outputs, and this file is not
one — it must survive a clean.
