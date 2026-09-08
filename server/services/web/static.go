package web

import "embed"

// staticContent embeds the assets the SSR landing pages serve from this
// binary: shared stylesheets, the self-hosted webfonts, the logo, and the
// two domain-association files.
//
// These used to live in the marketing site's tree and be embedded by a
// `website` package, which made the server unbuildable without the marketing
// site (#2953, OSS-3 #2955). They are the *server's* assets — it is the
// origin that serves them at /css/, /fonts/ and /.well-known/ — so they live
// here, and the site keeps working through directory symlinks in
// website/content/ that its deploy dereferences (`rsync --copy-dirlinks`).
//
// css and fonts are embedded as whole directories rather than a
// hand-maintained file list so a landing page can't silently lose a
// stylesheet, the generated color tokens (css/gen/tokens.gen.css), or a
// @font-face .woff2 — a per-file list previously drifted and shipped the
// /go/{code} pages with no colors or fonts. assets stays an explicit
// allowlist so the binary doesn't pick up large image directories.
//
// Paths within the FS are rooted at "static/" — use fs.Sub to strip it. The
// prefix is not incidental: `//go:embed` rejects a pattern whose *leading*
// element begins with `.`, so `.well-known/…` is only reachable underneath a
// normal directory name.
//
// The two association files are deployment identity, not product code: they
// carry an Apple Team ID and Android signing-certificate fingerprints. A fork
// must replace both with its own — see the README in that directory.
//
//go:embed static/css static/fonts static/assets/logo.png static/.well-known/apple-app-site-association static/.well-known/assetlinks.json
var staticContent embed.FS
