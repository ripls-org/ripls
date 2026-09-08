// Flutter Web bundle embed surface.
//
// `npm run build:web` runs `flutter build web --release` and copies
// the output (index.html, main.dart.js, manifest, asset/font/icon
// trees, etc.) into `app_assets/`. The build artifacts in this
// directory are .gitignored; only `placeholder.html` is checked in
// so `//go:embed` finds at least one file before any Flutter Web
// build has run on a fresh checkout.

package web

import "embed"

// AppAssets holds the Flutter Web release bundle copied here by
// `npm run build:web`. Served as the catch-all root by `ServeApp`,
// with SPA-style fallback to `index.html` so client-side routing
// handles deep links like `/event/{experience_id}`, `/feed`,
// `/community/{id}`, etc.
//
//go:embed app_assets
var AppAssets embed.FS
