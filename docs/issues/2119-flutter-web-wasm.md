# Issue #2119: Enable Flutter Web --wasm by fixing two incompatible deps

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2119
**Filed:** 2026-05-22 | **Last activity:** 2026-05-22
**Labels:** none | **Assignees:** unassigned | **Priority:** P3 (per issue body)
**Context docs:** `app/web/README.md` (primary — read in full), `docs/client/architecture.md` + `docs/client/conventions.md` (client anchors), `docs/dependency_automation.md` (why the blockers cleared), `docs/client/testing.md` (Playwright e2e boundary), `docs/reviews/performance_review_prompt.md` + `docs/reviews/dependency_review_prompt.md` (engaged dimensions)

## Problem Statement

The Flutter Web bundle currently ships the default CanvasKit-on-JS target
(`flutter build web --release`, compiled via `dart2js`). The issue asks to
switch to the WebAssembly target (`flutter build web --wasm`) for faster Dart
execution and smoother animation, and names two upstream packages that blocked
the WASM dry-run when the issue was filed (2026-05-22):

- `flutter_keyboard_visibility_web` — used a deprecated `dart:html` import.
- `flutter_timezone_web` (`4.1.1`) — 8 `invalid_runtime_check_with_js_interop_types`
  lint violations from casting `JSFunction?` to Dart function types.

**Both blockers are already resolved in the current dependency tree**, by
version bumps that landed between the issue being filed and today. These are
`pub` (Flutter app) deps, which Renovate routes to **human review** rather than
auto-merge (per `docs/dependency_automation.md` §4), so the bumps were reviewed
Renovate PRs, not silent auto-merges. I verified the WASM impact end-to-end
rather than trusting the lockfile:

- `flutter build web --wasm --release` **compiles cleanly today** (~53 s
  compile; `✓ Built build/web`, zero errors, zero WASM lint failures).
- `flutter_keyboard_visibility` is **gone from `app/pubspec.lock` entirely**.
  It was a transitive dependency of `flutter_typeahead`; the locked
  `flutter_typeahead` is now `6.0.0`, which dropped the
  `flutter_keyboard_visibility` dependency (6.x switched its overlay to
  `OverlayPortal`). The `dart:html` package is no longer anywhere in the tree.
- `flutter_timezone` is now locked at `5.1.0` (issue referenced `4.1.1`). The
  `5.1.0` web source (`flutter_timezone_web.dart`) was **rewritten** to use
  typed `dart:js_interop` extensions (`@JS` + `@staticInterop` with typed
  `external` accessors) instead of the `JSFunction?` casts — the exact lint
  violation the issue describes no longer exists.
- No Renovate pin or held-back rule touches `flutter_timezone`,
  `flutter_typeahead`, or `flutter_keyboard_visibility` (checked `renovate.json`
  + `renovate/`), so this isn't a fragile resolution someone will bump back.

So the issue's genuinely hard part — patching or forking upstream packages — is
**already done for us**. What remains is the mechanical migration checklist the
issue lists, plus one adaptation the issue under-specified: the bundle-size gate
script hard-codes JS output filenames and must be taught the WASM output shape.

The scope of this plan is therefore: **verify, flip the build flag, adapt the
size gate + reset the baseline, extend one server MIME test, and update the
docs** — no upstream patching, no fork. **Plus** (added at the maintainer's
request) a fix for a pre-existing, WASM-sharpened perf gap the audit surfaced:
the server serves the whole web bundle **uncompressed** (Current State #6).
Because the WASM bundle's largest asset is a 9.6 MB `main.dart.wasm`, shipping it
raw is the dominant transfer cost, so compression is now in scope here rather
than deferred. That adds one build-time precompression step and one negotiating
branch to the app handler.

## Current State (verified this session)

Facts established by inspecting the real tree and a real WASM build. These
drive the solutions below.

**1. `--wasm` emits a dual bundle with automatic runtime fallback.** The build
produced *both*:
- WASM path: `main.dart.wasm` (9.6 MB raw / **3.03 MB gzipped**) + `main.dart.mjs` (13.7 KB gz).
- JS fallback: `main.dart.js` (11.9 MB raw / **2.86 MB gzipped**) — same as the
  current renderer output.

`flutter_bootstrap.js` selects at runtime based on browser capability. This
**directly answers the issue's "verify graceful fallback for older browsers"
checkbox**: Flutter does it automatically — old browsers (below Chrome 119 /
Safari 16.4 / Firefox 120) transparently get the JS bundle. There is no
correctness cut-off to document, only a performance one.

**2. No server change is needed to serve WASM.** The app handler
(`server/services/web/app_handler.go`) serves assets through
`http.FileServer` → Go's `mime.TypeByExtension`. On this Go toolchain that
resolves `.wasm → application/wasm` and `.mjs → text/javascript; charset=utf-8`
(verified by running `mime.TypeByExtension`). `application/wasm` is exactly what
streaming WASM compilation requires, and `.mjs` gets a JS content-type that
survives the `X-Content-Type-Options: nosniff` guard. The existing cache-header
logic (`isHashedAsset`) already treats `main.dart.wasm`/`.mjs` correctly:
fixed-name → `no-store`, same as `main.dart.js` today.

**3. No cross-origin-isolation headers are needed (and adding them is risky).**
The WASM build ships two Skia renderers: `skwasm.wasm` (single-threaded) and
`skwasm_heavy.wasm` (multithreaded). `flutter_bootstrap.js` branches on
`crossOriginIsolated`. Because the server sends **no** COOP/COEP headers today,
the page is not cross-origin-isolated, so Flutter uses single-threaded
`skwasm.wasm` — which works fully. Turning on multithreaded `skwasm_heavy`
would require `COOP: same-origin` + `COEP: require-corp` (or `credentialless`),
and `COEP: require-corp` breaks every cross-origin subresource that doesn't send
CORP/CORS headers — Mapbox tiles, Firebase, the media CDN, Google Maps. That is
a separate optimization with its own risk surface and should **not** ride along
in this migration.

**4. The bundle-size gate needs adapting; it's currently disabled anyway.**
`scripts/check_web_bundle_size.sh` measures the initial chunk as
`flutter_bootstrap.js` + `main.dart.js`. Under `--wasm` that measures the JS
*fallback*, not the WASM path a modern browser actually loads. Both baselines in
`app/web/bundle_baseline.txt` are currently `0` (gates disabled, informational
only), so nothing breaks — but the migration is the natural moment to teach the
script the WASM shape and commit real numbers. Measured now (release):
- WASM initial chunk (`flutter_bootstrap.js` + `main.dart.mjs` + `main.dart.wasm`): **~3.05 MB gz**.
- Total bundle (gzip-sum of every file, both renderers + symbols + canvaskit + chromium): **~27 MB gz** (vs. a JS-only total that shipped one renderer). The dual renderer roughly doubles disk/CDN weight; the `+20%` total tolerance cannot absorb this, so the total baseline *must* be reset, not nudged.

**5. e2e already exercises whatever the build scripts emit.** The Playwright web
suite builds via `npm run build:web:e2e` (a `--profile` variant). If `--wasm`
goes into the profile scripts, the e2e suite runs against the WASM renderer for
free — giving us WASM-runtime coverage rather than only testing a renderer prod
won't use.

**6. The server serves the bundle *uncompressed* — a real, pre-existing perf gap
that WASM sharpens (now in scope).** There is **no HTTP response-compression
middleware** in the Go server (grep of `server/**/*.go` finds only unrelated
proto-descriptor gzip in generated code), the Flutter build emits **no
precompressed `.gz`/`.br` files**, and no fronting-proxy/CDN compression exists
in `terraform/` (Cloud Run's GFE does not auto-gzip; the app must). So
`main.dart.js` ships today at its **raw** ~11.9 MB, not the 2.86 MB gzipped the
size gate measures — the gate's gzipped baseline is aspirational, describing bytes
that never reach the browser. Under WASM, `main.dart.wasm` is **9.6 MB raw**
(3.03 MB gzip / **~2.25 MB brotli-11**, both measured this session).

The migration's real prize (faster Dart *execution* under WASM) is independent of
transfer compression, so this gap does **not** block or undercut #2119 — but
serving a 9.6 MB wasm raw makes it the dominant cost of a cold web visit, so we
fix it here. Design basis I verified for the fix (see Phase 3):
- **Assets are immutable and embedded** (`//go:embed app_assets`, populated by a
  single `cp -R app/build/web/. app_assets/` at the tail of every `build:web:*`).
  For byte-identical, cache-immutable static files, **precompressing once at build
  time** and serving the precompressed variant beats on-the-fly compression: best
  ratio (offline brotli quality 11), zero per-request CPU, and no new *runtime*
  dependency.
- **No new dependency of any kind is required.** Node's built-in `zlib` has both
  `brotliCompressSync` and `gzipSync` (verified on the repo's Node), so a small
  `scripts/precompress_web_bundle.*` invoked from the npm build does the
  compression — no `brotli` system binary, no Go library. The `.br`/`.gz`
  siblings land in `app_assets/` via the existing `cp -R` and embed automatically.
- **The handler change is one branch.** `app_handler.go` already resolves
  `Content-Type` correctly per extension; the negotiating branch picks
  `br`/`gzip`/identity from `Accept-Encoding`, sets `Content-Encoding` +
  `Vary: Accept-Encoding`, and keeps `Content-Type` from the *canonical*
  extension (so `main.dart.wasm.br` still serves `application/wasm`) and the
  existing cache headers. Full design in Phase 3.

## Context from Comments

**The issue has no comments.** For a P3 optimization filed as a deliberate
carve-out from #2050 this is expected, but it means one thing worth flagging:
the issue is 7 weeks old and the dependency landscape moved underneath it. The
"what to do" section (fork/patch two packages) is now stale — anyone picking
this up from the issue text alone would start forking packages that no longer
need forking. This plan supersedes that section; the first implementation step
is to *confirm* the deps rather than *fix* them.

## Potential Solutions

The dep-fixing options in the issue (fork vs. wait for upstream) are both moot.
Two real decisions remain: **(1) how broadly to enable `--wasm`** (Options A–C),
and **(2) how to compress the served bundle** (Options D–E).

### Decision 1 — WASM build breadth

### Option A: Enable WASM everywhere (all `build:web:*` variants), adapt the gate, reset baselines
- **Approach:** Add `--wasm` to every `flutter build web` invocation in
  `package.json` (`build:web:local`, `:dev`, `:e2e`, `:prod`) and in
  `check_web_bundle_size.sh`. Teach the gate script to detect `main.dart.wasm`
  and measure the WASM initial chunk as the primary axis. Commit fresh
  baselines. Extend the server MIME test to pin `.wasm`/`.mjs`. Update
  `app/web/README.md`.
- **Pros:** Dev, e2e, and prod all run the same renderer — the e2e Playwright
  suite exercises the exact bundle prod ships, so a WASM-only runtime regression
  surfaces in CI, not in production. One renderer to reason about. Matches the
  issue's intent (WASM is the go-forward target).
- **Cons:** Dev/e2e builds get marginally slower and larger. If some WASM-only
  runtime quirk exists, it now blocks dev iteration too (mitigated: the JS
  fallback still exists in the same bundle).
- **Effort:** Small–Medium.

### Option B: Enable WASM for prod only (`--release`), keep dev/e2e on JS (`--profile`)
- **Approach:** Add `--wasm` only to `build:web:prod` and the release build in
  `check_web_bundle_size.sh`. Leave the `--profile` variants on JS.
- **Pros:** Dev iteration unchanged. Smallest blast radius.
- **Cons:** **e2e would test a renderer prod doesn't use** — the highest-value
  reason to do this migration carefully (catching WASM runtime regressions) is
  lost. Two renderers to keep working. `--profile` retains readable Dart symbol
  names, but the JS fallback in a `--wasm` build already preserves debuggability
  for the cases that matter. This trades away CI signal for a small dev-speed
  win.
- **Effort:** Small.

### Option C: Minimal flip — prod `--wasm` + README, defer the gate entirely
- **Approach:** Add `--wasm` to `build:web:prod`, update the README, leave
  `check_web_bundle_size.sh` and the (already-zeroed) baselines untouched.
- **Pros:** Smallest possible change; ships the perf win immediately.
- **Cons:** Leaves the gate silently measuring the wrong thing — when someone
  later sets a baseline, they'd baseline the JS fallback, not the WASM path a
  browser loads, so the gate would never see WASM bundle growth. Kicks a
  half-configured gate down the road. Skips the MIME regression test.
- **Effort:** Small.

### Decision 2 — Compression approach

### Option D: Build-time precompression (brotli + gzip) + a negotiating handler
- **Approach:** A small `scripts/precompress_web_bundle.*` (Node `zlib`, no new
  dep) walks `app/build/web/`, and for each compressible file above a small size
  floor writes `<file>.br` (brotli q11) and `<file>.gz` (gzip -9), keeping the
  variant only if smaller. Wired into every `build:web:*` after `flutter build
  web`, before the `cp -R` into `app_assets/` (so the variants embed for free).
  `app_handler.go` gains one branch: pick `br`/`gzip`/identity from
  `Accept-Encoding`, serve the precompressed sibling with `Content-Encoding` +
  `Vary: Accept-Encoding` + canonical-extension `Content-Type` + the existing
  cache headers.
- **Pros:** Best ratio (offline brotli-11), **zero per-request CPU** (matters on
  Cloud Run cold starts / scale-to-zero), no new *runtime* dependency, brotli
  *and* gzip so both modern and proxy-stripped clients win. Serves immutable
  bytes — the natural model for a `//go:embed` static bundle.
- **Cons:** Adds a build step and ~a few MB to the embedded binary (canonical +
  `.br` + `.gz` for the big files). Slightly more handler code + tests.
- **Effort:** Medium (the bulk of the added scope).

### Option E: On-the-fly gzip middleware
- **Approach:** Wrap only the app-bundle handler in a gzip `ResponseWriter`
  (e.g. `klauspost/compress/gzhttp`), leaving the build untouched.
- **Pros:** ~10 lines, no build change, no embed-size growth.
- **Cons:** Recompresses on every cache-miss request (CPU per cold serve),
  gzip-only in practice (no brotli → ~15% worse and misses the wasm win),
  and adds a **runtime** Go dependency for something that could be done once
  offline. For immutable static assets this is the weaker engineering choice.
- **Effort:** Small.

## Recommendation

**Decision 1 → Option A** (enable WASM across all `build:web:*` variants, adapt
the gate, commit fresh baselines), and **Decision 2 → Option D** (build-time
brotli+gzip precompression with a negotiating handler). Cross-origin isolation
stays explicitly **out of scope** (single-threaded skwasm, no COOP/COEP).

Rationale:
- **Option A:** The dep blockers are gone, so the cost side of the ledger is
  small. Renderer parity between e2e and prod is the single most valuable safety
  property available — WASM is a new-ish target (Flutter still prints an
  "understand the details before deploying to production" banner), and the way you
  de-risk it is to make CI's e2e suite run the same bytes prod runs. Option B
  throws that away for a marginal dev-speed gain. The gate must be adapted
  regardless (Current State #4).
- **Option D:** For immutable, embedded static assets, precompress-once is the
  textbook-correct choice over per-request compression, and Node `zlib` means it
  costs no dependency. It gets brotli (which the on-the-fly path realistically
  won't), which is exactly what tames the 9.6 MB wasm. The cost is a contained
  build step + handler branch — acceptable given the maintainer pulled this into
  scope.

Explicitly deferred, with a follow-up issue: multithreaded `skwasm_heavy` via
cross-origin isolation. It's a real further perf win but its risk (breaking
Mapbox/Firebase/media cross-origin loads under `COEP: require-corp`) is
disproportionate here, and it deserves its own plan.

## Open Questions

- **WASM everywhere vs. prod-only?** — ✅ **DECIDED: everywhere (Option A).** All
  four `build:web:*` variants get `--wasm`, so e2e tests the exact renderer prod
  ships; the dual bundle keeps a JS fallback in every build anyway.
- **Does `--wasm` compose with `--profile`?** — Recommendation: assume yes but
  *verify in Phase 1* by actually running `build:web:e2e`/`build:web:dev` with
  the flag. `flutter build web --wasm --profile` is supported by Flutter 3.44;
  the release path is already confirmed. If a profile+wasm combination misbehaves
  on the runners, fall back to Option B for the profile variants only and note
  it in the README.
- **What should the gate measure under WASM — the WASM chunk, the JS fallback,
  or both?** — Recommendation: make the **WASM initial chunk** (`bootstrap` +
  `main.dart.mjs` + `main.dart.wasm`) the primary initial-chunk gate (that's what
  a modern browser loads), and keep the **total-bundle** gate summing *all*
  files (both renderers). Optionally also print the JS-fallback initial chunk as
  informational. Detect the WASM build by presence of `main.dart.wasm` so the
  script still works if someone runs a non-wasm build locally.
- **What baseline numbers to commit?** — Recommendation: run
  `bash scripts/check_web_bundle_size.sh` on the prod-target build after the flag
  lands and commit the emitted "current measurement" numbers (initial ≈ 3.05 MB
  gz, total ≈ 27 MB gz as measured this session — but let the script's own
  numbers from the CI-matching build be authoritative, not these local ones).
- **Should the `~27 MB` total alarm us?** — Recommendation: no, for a P3, but
  note it. It's disk/CDN cost, not per-visit cost (a browser downloads one
  renderer path, ~3 MB). Two refinements are possible follow-ups: (a) stop
  shipping/serving the `*.symbols` files (~3 MB of the total) if we don't use
  them for deobfuscation on web; (b) revisit whether both renderer families need
  to ship. Neither blocks this issue.
- **Browser support cut-off copy for the README?** — Recommendation: state
  "WASM path requires Chrome/Edge 119+, Safari 16.4+, Firefox 120+; older
  browsers automatically fall back to the JS bundle — no feature loss, slower
  execution." Frame it as automatic fallback, not a hard requirement.
- **Compression: precompress vs on-the-fly?** — Recommendation: precompress
  (Option D). Immutable embedded assets + free Node `zlib` + brotli make it the
  clear winner over an on-the-fly middleware (Option E). Now **in scope** for
  #2119 at the maintainer's request (was previously a defer).
- **Ship both `.br` and `.gz`, or brotli only?** — ✅ **DECIDED: both.** Brotli is
  the primary win, `.gz` is cheap insurance for proxies that strip `br` from
  `Accept-Encoding` and for the older-browser JS-fallback tier. The extra embed
  weight is accepted; brotli-only stays available as a clean later trim if it ever
  matters. State the choice in the precompress script's header.
- **Which files to precompress, and a size floor?** — Recommendation: an
  *extension allowlist* of compressible types (`.js .mjs .wasm .json .html .css
  .otf .ttf .map .symbols`), skipping already-compressed formats
  (`.png/.jpg/.webp/.woff2/.ico`) and files below ~1 KB (compression overhead
  isn't worth it). Keep a variant only when it's actually smaller than the source.
- **Does the size gate double-count the new `.br`/`.gz` files?** — Recommendation:
  no, but make it defensive. The gate runs its *own* `flutter build web` (not the
  npm `build:web`), so its build dir won't contain precompressed siblings — but
  add an explicit `-not -name '*.br' -not -name '*.gz'` to the gate's `find` so it
  can never gzip-a-gzip if someone runs precompression first. Note in the gate
  comment that, with precompression live, the gzipped measurement now finally
  matches (for gzip; brotli is a bit smaller) what's actually on the wire.
- **`Vary: Accept-Encoding` on every asset response?** — Recommendation: yes, set
  it on both compressed and identity asset responses so a shared/browser cache
  keyed by URL can't hand a brotli body to a client that only sent `gzip`. Cheap
  correctness.

## Implementation Plan

### Phase 1: Confirm the deps and flip the build flag
**Goal:** Prove the WASM build is green in a CI-shaped environment and switch the
build scripts over.
**Tasks:**
- [ ] Re-run `flutter build web --wasm --release` from a clean `flutter pub get`
      to confirm the current lockfile compiles under WASM (already verified
      locally this session; re-confirm on a runner-matching toolchain — Flutter
      3.44.0 per `versions.env`).
- [ ] Verify `--wasm` composes with `--profile` by running the dev/e2e build
      commands with the flag added (answers the open question).
- [ ] Add `--wasm` to every `flutter build web` invocation in `package.json`:
      `build:web:local`, `build:web:dev`, `build:web:e2e`, `build:web:prod`.
      (Leave the `sed`/copy/`app_assets` tail of each script unchanged.)
- [ ] Sanity-check the output tree has `main.dart.wasm` + `main.dart.mjs` +
      `main.dart.js` (fallback) + `canvaskit/skwasm*.wasm`.
- [ ] Run the Playwright e2e suite once against the WASM `build:web:e2e` bundle
      (`npm run test:web:e2e` or the workflow-equivalent) and confirm it stays
      green — this is the WASM-runtime regression check the parity argument buys.
      A WASM-only runtime failure (e.g. a JS-interop path that behaves
      differently under `dart2wasm`) surfaces here, not in prod.
**Validates:** The whole premise — that the tree builds *and runs* under WASM
with no upstream patching — and that dev/e2e/prod all build with the flag.

### Phase 2: Adapt the size gate + reset baselines + pin MIME types
**Goal:** Make the CI gates and server tests correct for the WASM output shape.
**Tasks:**
- [ ] Update `scripts/check_web_bundle_size.sh`: detect `main.dart.wasm`; when
      present, compute the initial chunk as `flutter_bootstrap.js` +
      `main.dart.mjs` + `main.dart.wasm` (keep the JS-only path as a fallback so
      the script still works for a non-wasm build). Keep the total-bundle gate
      summing all files. Update the header comments (they currently assert
      "CanvasKit is now the only renderer" and name `main.dart.js` as the initial
      chunk).
- [ ] Add `-not -name '*.br' -not -name '*.gz'` to the gate's `find` so the
      total-bundle sum can never gzip-a-gzip once precompression (Phase 3) exists.
- [ ] Run the script against the prod-target build and commit the emitted
      current-measurement numbers into `app/web/bundle_baseline.txt` (line 1 =
      initial chunk, line 2 = total). Update that file's comment block, which
      still references the pre-WASM baseline-picking process — and note there that
      the numbers are *gzipped build artifacts* (a size budget); once Phase 3's
      precompression ships, gzipped ≈ the actual gzip wire size (brotli a bit
      smaller).
- [ ] Extend `TestServeApp_BundleAssetsServeWithJSMime` in
      `server/services/web/app_handler_test.go` to also assert `/main.dart.wasm`
      returns `application/wasm` and `/main.dart.mjs` returns a JS content-type —
      pinning the MIME behavior that WASM instantiation depends on so a future Go
      or embed change can't silently break it.
- [ ] Confirm `go test ./server/services/web/...` and
      `bash scripts/check_web_bundle_size.sh` both pass.
**Validates:** CI now gates the bytes a real browser downloads, and the
server's WASM/`.mjs` content types are regression-protected.

### Phase 3: Serve the bundle compressed (brotli + gzip)
**Goal:** Stop shipping the ~9.6 MB `main.dart.wasm` (and the JS fallback) raw;
serve precompressed variants with content negotiation. (Current State #6.)
**Tasks:**
- [ ] Add `scripts/precompress_web_bundle.mjs` (Node `zlib`, no new dep): walk a
      target dir (arg-defaulted to `app/build/web`), and for each file whose
      extension is in the compressible allowlist (`.js .mjs .wasm .json .html
      .css .otf .ttf .map .symbols`) and size ≥ ~1 KB, write `<file>.br`
      (`brotliCompressSync`, quality 11) and `<file>.gz` (`gzipSync`, level 9),
      keeping each variant only when smaller than the source. Skip files already
      ending in `.br`/`.gz` (idempotent). Print a short summary (bytes saved).
- [ ] Add a unit test for the script (node:test) covering: allowlist filtering,
      the size floor, the smaller-only guard, and idempotency.
- [ ] Wire it into every `build:web:*` variant in `package.json` — run it on
      `app/build/web` **after** `flutter build web …` and **before** the existing
      `cp -R app/build/web/. server/services/web/app_assets/` so the `.br`/`.gz`
      siblings embed via `//go:embed` with no other change.
- [ ] Add the negotiating branch to `ServeApp` in
      `server/services/web/app_handler.go`: in the asset path (after the
      `fs.Stat(subFS, cleaned)` success, before `fileServer.ServeHTTP`), choose
      `br` if the request's `Accept-Encoding` includes it **and** `cleaned+".br"`
      exists in the embed FS; else `gzip` if accepted and `cleaned+".gz"` exists;
      else identity. For a compressed pick: set `Content-Encoding`,
      `Vary: Accept-Encoding`, `Content-Type` from `mime.TypeByExtension` of the
      **canonical** extension (not `.br`/`.gz`), reuse `setAppCacheHeaders(w,
      cleaned)` (cache semantics keyed on the canonical name are unchanged), then
      write the sibling's bytes (`fs.ReadFile`) directly — do **not** route
      compressed bytes through `http.FileServer` (it would derive headers from the
      `.br` name). Identity keeps today's path, plus `Vary: Accept-Encoding`.
      Treat a direct request for a `.br`/`.gz` URL as not-an-asset (404 via the
      existing `looksLikeAssetPath` handling) so siblings are reachable only
      through negotiation.
- [ ] Extend `app_handler_test.go`: for `main.dart.wasm` and `main.dart.js`,
      assert (a) `Accept-Encoding: br` → `Content-Encoding: br` **and**
      `Content-Type: application/wasm` / `…javascript` (type survives
      compression) + `Vary: Accept-Encoding`; (b) `Accept-Encoding: gzip` (no br)
      → `Content-Encoding: gzip`; (c) no `Accept-Encoding` → identity (no
      `Content-Encoding`), and the immutable/`no-store` cache header is unchanged
      across all three. These require a built bundle, so gate on the existing
      `bundleAvailable()` skip like the sibling test.
- [ ] Confirm `go test ./server/services/web/...`, `npm test` (script unit test),
      and a manual `curl -H 'Accept-Encoding: br' …/main.dart.wasm -I` against a
      locally-run server show `content-encoding: br` + `content-type:
      application/wasm`.
**Validates:** A cold web visit downloads **~2.25 MB brotli**, not 9.6 MB raw
wasm (a ~4× reduction) — the actual point of pulling compression into this effort
— with `Content-Type` and caching intact and old/proxy clients degrading to gzip
then identity.

### Phase 4: Docs, browser-compat note, and close-out
**Goal:** Bring `app/web/README.md` in line with the shipped renderer and record
the support story + deferred work.
**Tasks:**
- [ ] Update `app/web/README.md`: the "Pinned Flutter version" section (currently
      says CanvasKit-on-JS is what we ship and that `--wasm` is blocked on the two
      deps / tracked as #2119), the pipeline diagram comment (`dart2js`); add a
      short "Renderer: WASM with JS fallback" note covering the dual bundle, the
      automatic browser fallback + version cut-off, and the single-threaded-skwasm
      (no cross-origin isolation) decision with the Mapbox/Firebase/media-CDN
      reason; and add a "Compression" note documenting the build-time precompress
      step + the handler's `Accept-Encoding` negotiation (br → gzip → identity,
      `Vary: Accept-Encoding`). (This is an app-level README, not a `docs/` durable
      doc — no `context:`/`freshness:` frontmatter, outside the freshness ratchet.)
- [ ] File a follow-up issue for multithreaded skwasm via COOP/COEP
      cross-origin isolation (the deferred perf win + its cross-origin-load
      risk), and reference it from the README note.
- [ ] (Optional) a follow-up for trimming the `*.symbols` files (~3 MB of the
      total) from the served bundle if we don't deobfuscate web stack traces —
      note these now also carry `.br`/`.gz` siblings, so the trim saves 3×.
- [ ] Close #2119, noting in the closing comment that the two dep blockers
      self-resolved via Renovate bumps (`flutter_typeahead` 6.0.0 dropped
      `flutter_keyboard_visibility`; `flutter_timezone` 5.1.0 rewrote its web
      interop), so the work became the build/gate migration **plus** the
      brotli/gzip serving fix pulled in from the audit.
**Validates:** The docs match reality, the deferred optimization is tracked, and
the issue's ledger is closed honestly.

## Related Issues
- **#2050** — shipped the Flutter Web foundation (guest RSVP) on CanvasKit-on-JS;
  this issue is its explicitly-deferred renderer-migration follow-up.
- **#1195** — the parent web epic (#2050 was sub-issue 3).
- **#2157** — apex-domain move; established the bundle-size gate and the
  baseline-picking process this plan resets. Also the source of the
  `Platform.is*`/`dart:io` web audit in `app/web/README.md`.
- **Dependency automation** (`docs/dependency_automation.md`, Renovate on mimir)
  — the mechanism that silently cleared both blockers between filing and now; a
  live example of why a plan re-verifies dep state rather than trusting an aging
  issue body.
- **(to be filed)** — multithreaded skwasm via cross-origin isolation
  (COOP/COEP); deferred perf follow-up carved out of this migration.

---

## Audit notes (ripls-audit)

Audited against the documentation context map. Findings folded into the plan
above:

1. **Performance / compression (added — Current State #6, Open Questions, and a
   dedicated Phase 3).** Verified the Go server ships the bundle uncompressed
   (no response-compression middleware in `server/**/*.go`, no precompressed
   assets from the build, no CDN gzip in `terraform/`); the size gate measures
   gzipped bytes that never reach the wire. Originally flagged as a separate
   follow-up; **the maintainer pulled it into this effort**, so the plan now
   carries a full compression design (build-time brotli+gzip precompression via
   Node `zlib` — no new dep — plus an `Accept-Encoding`-negotiating branch in the
   app handler) as first-class scope.
2. **Testing (strengthened — Phase 1).** Added an explicit "run the Playwright
   e2e suite under WASM and confirm green" task so the renderer-parity argument
   is actually cashed in as a regression check, not just asserted.
3. **Dependency accuracy (corrected — Problem Statement).** Noted `pub` deps are
   human-reviewed by Renovate (not auto-merged) per `dependency_automation.md` §4;
   cross-checked the *Held-back dependencies* list and confirmed none of
   `flutter_timezone` / `flutter_typeahead` / `flutter_keyboard_visibility` is
   capped, so the WASM-enabling resolution is stable.
4. **Doc-map correctness (corrected — Phase 3).** `app/web/README.md` is an
   app-level README, not a `docs/` durable doc, so it has no `context:`/
   `freshness:` frontmatter and is outside the freshness ratchet; removed the
   conditional frontmatter task.

**Compliance:** No architecture/convention violations. No `app/lib/**` Dart
change (so client conventions, i18n, accessibility don't engage); no proto/RPC
change (api_design n/a); no new secrets or auth surface. With compression now in
scope there **is** a real server code change — the `ServeApp` negotiating branch
in `app_handler.go` — but it stays inside the existing web-serving package, adds
no dependency (Node `zlib` at build time; standard-library `mime`/`fs` at
runtime), and follows the package's existing header-setting patterns; the security
surface is limited to `Content-Type`/`Content-Encoding`/`Vary` correctness, which
Phase 3's tests pin (the same `nosniff`/MIME concern the original test guards).
Cross-origin isolation correctly kept out of scope with a documented reason
(Mapbox/Firebase/media cross-origin loads under `COEP: require-corp`).

**Docs consulted:** `docs/reviews/dependency_review_prompt.md`,
`docs/reviews/performance_review_prompt.md`, `docs/dependency_automation.md`,
`app/web/README.md` (read in full), `docs/llms.txt` (map), plus the
client/server/testing anchors referenced in the header. Verified against the live
tree: a real `flutter build web --wasm --release`, `app/pubspec.lock`, the
`flutter_timezone-5.1.0` web source, `server/services/web/app_handler.go` +
`app_handler_test.go`, `renovate.json`, and a compression grep of `server/` +
`terraform/`.
