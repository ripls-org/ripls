# Flutter Web

The Ripls Flutter app compiles to a web bundle that the Go server
embeds and serves at the deployment's apex domain (and its dev equivalent). This directory holds the web-only
entry-point files (`index.html`, manifest, icons) plus the bundle-
size baseline used by the CI gate.

## Pipeline

```
app/web/index.html            (source — Flutter entry point)
  + app/lib/**.dart           (shared Dart sources)
        │
        ▼
  flutter build web --wasm --release (WASM + JS fallback)
        │
        ▼
app/build/web/                (build output — gitignored)
        │
        ▼  npm run build:web copies here
server/services/web/app_assets/
        │
        ▼  //go:embed app_assets (server/services/web/app_embed.go)
        ▼
Served as catch-all root (server/services/web/app_handler.go)
```

`build:web` does the full pipeline in one step. After it runs, the
next `go build` picks up the new bundle through `//go:embed`.

### Deploy-time bundling (CI)

The same pipeline runs in CI before the container image is built.
`.github/workflows/build_and_deploy_container.yaml` (invoked by
`deploy_to_dev_on_push.yaml`, `deploy_manual.yaml`, and
`release_finalize.yaml`) authenticates to GCP, runs
`npm run build:web:dev` or `:prod` depending on the target
environment, and only then hands off to `docker/build-push-action`.
The `Dockerfile` itself does **not** install Flutter or build the
bundle — it just `COPY server ./server`s the staged
`server/services/web/app_assets/` tree that the workflow populated.

This is why the bundle build step lives in the workflow, not the
`Dockerfile`: it lets us reuse the self-hosted runner's pre-installed
Flutter SDK + pub cache, and lets `scripts/fetch_secret.sh` pull the
matching Mapbox token and Firebase web API key from Secret Manager
under the deploy SA's existing credentials. The image stays
cloud-agnostic.

### Variants by target environment

Three variants mirror the mobile build scripts. Each passes the
matching `env.{env}.json` via `--dart-define-from-file` and fetches
the matching Mapbox token from Secret Manager. Local + dev builds
use `--profile` (keeps Dart symbol names in stack traces for
debuggability); prod uses `--release` (full obfuscation +
optimization).

| Script | env file | Mapbox secret project | Build mode | Use when |
|---|---|---|---|---|
| `npm run build:web:local` (= default `build:web`) | `env.local.json` | dev project | `--profile` | Building against a local server on `localhost:8080` |
| `npm run build:web:dev` | `env.dev.json` | dev project | `--profile` | Deploying to dev |
| `npm run build:web:prod` | `env.prod.json` | prod project | `--release` | Deploying to prod |

The bare `build:web` alias points at `:local` because the only thing
that runs it is a human at a terminal about to `npm run start:server`
— a bundle pinned to the dev server URL silently ignores the
local server you just started, and the symptom (stale behavior, a
login that hangs on old server code) looks like an app bug rather
than a wrong target. CI never uses the alias: the deploy workflow
names `build:web:dev` / `build:web:prod` explicitly, so retargeting
the default cannot affect a deploy.

`:local` resolves the API host from the page origin at runtime
(`Environment.getServer`, `ENVIRONMENT=local` → `Uri.base.origin`),
so the same bundle works on `localhost:8080`, a LAN IP for a real
device, or `10.0.2.2:8080` from Chrome inside the Android emulator.
To point a local build at a *remote* server, pass an explicit
override rather than switching env files:
`--dart-define=SERVER_URL=https://<your-dev-server>`.

The mode split matches the mobile build scripts: `build:app:*:dev`
uses `--profile`, `build:app:*:prod` uses `--release`. The bundle-
size gate (`scripts/check_web_bundle_size.sh`) measures the
release build separately so the baseline reflects the prod
target, not the dev one.

A web bundle built without these dart-defines will boot but
null-check-explode during `main()` — `Environment.getServer` reads
the server URL from the `ENVIRONMENT` dart-define, and downstream
auth / config initialization assumes the values are populated.

## Routing at the apex domain

The bundle is built with `flutter build web --wasm --release --base-href=/`
(set in the `build:web` npm script) and served by the Go embed
handler as a catch-all root (`mux.Handle("/", webService.ServeApp())`
in `server/main.go`). Because the base href is `/`, the browser URL
and the GoRouter path are identical — there's no base-href stripping
to reason about. `context.go('/event/...')` produces the browser URL
`https://<app-host>/event/...`.

**Server-side dispatch.** The catch-all is mounted last but Go's
`http.ServeMux` uses longest-prefix-wins, so more-specific handlers
claim their paths first:

- `/go/*` → SSR invite landing (the universal-link bridge for
  installed apps)
- `/.well-known/*` → AASA + assetlinks (Universal Link allowlist)
- `/css/*`, `/assets/*` → marketing static assets
- `/ripls.api.*` (Connect RPC paths) → service handlers
- `/health`, `/readyz`, `/metrics` → ops endpoints
- everything else (`/`, `/feed`, `/community/{id}`, `/event/{id}`,
  …) → Flutter Web bundle (SPA fallback to `index.html` for
  client-route-shaped paths; 404 for asset-shaped paths that don't
  exist in the embed).

**Auth persistence across page reloads.** Web auth uses the same
Firebase ID-token mechanism as mobile — there's no separate session
cookie. `services/auth_state.dart`'s `_readToken` / `_writeToken`
helpers wrap `flutter_secure_storage` with a `SharedPreferences`
fallback that's the primary storage on web (the secure-storage
plugin throws on insecure-context origins like
`http://10.0.2.2:8080`, and the catch-block routes the read/write
through prefs instead). On page reload, `loadAuthState()` reads the
token from prefs, hands it to the Riverpod `authStateProvider`, and
the existing Connect-RPC interceptor attaches it as
`Authorization: Bearer ...` on every RPC. No special web-session
wiring required, and no `EstablishWebSession` RPC exists.

**Universal-link safety.** The AASA `applinks.details[0].paths` list
in `server/services/web/app_assets/.well-known/apple-app-site-association`
(and the equivalent `assetlinks.json`) is **deliberately narrow** —
only `/go/*`, `/invite*`, and `/reset-password*` are claimed by the
native app. Tapping `https://<app-host>/feed` from iOS Messages with
the app installed opens the *browser*, not the app. Widening that
path list would steal every link from the installed app; the test
`TestAASA_PathsAreExact` in `server/services/web/service_test.go`
pins the exact allowlist so a regression fails CI.

## Web-unsupported features

Several mobile plugins the app depends on have no Flutter Web
implementation. Rather than guard every plugin call deep in the
modal stack, the strategy is to **guard at entry points** —
short-circuit the user-facing action with a notice that points the
visitor at the mobile app, where the flow works as designed.

The shared helper lives at
`app/lib/presentation/widgets/web/web_unsupported.dart`:

| Helper | Used when | ARB key |
|---|---|---|
| `WebUnsupported.showCreateNotice(context)` | User taps the `+` FAB on HomeScreen (any create flow — gear, event, request) | `webUnsupportedCreate` |
| `WebUnsupported.showCalendarNotice(context)` | User taps Add-to-Calendar on an event view | `webUnsupportedCalendar` |
| `WebUnsupported.showPhotoUploadNotice(context)` | User taps a photo-upload control (profile, community, media save) | `webUnsupportedPhotoUpload` |

Each helper is `kIsWeb`-gated and returns `bool` — `true` when the
notice was shown — so callers short-circuit cleanly:

```dart
if (WebUnsupported.showCreateNotice(context)) return;
// …mobile create flow that depends on camera / image_picker / gal …
```

### Guarded entry points (as of #2157)

- **Unified-create Image tab (live camera)** —
  `unified_create_camera_layer.dart`'s `build` returns a gallery-
  only fallback on web. The Text and URL tabs in the same modal
  work as on mobile; the Image tab renders a "Pick a photo from
  gallery" button + a small note that the live camera is mobile-
  only. The `+` FAB itself is **not** blocked on web — gallery /
  text / URL creation flows all work.
- **Add to Calendar** — `experience_content_view._addToCalendar` and
  `time_poll_finalized_modal._exportToCalendar`. (Plugin:
  `add_2_calendar`.)
- **Save / download media to gallery** — `media_carousel.
  _saveCurrentMedia` and `_handleDownloadAll`. (Plugins: `gal` plus
  `dart:io File` for the temp-file dance — the browser's own
  download UI is the natural web-side replacement.)
- **Chat compose attachments** —
  `inline_conversation_view._pickAndStageMedia`. The staged-preview
  renderer in `inline_conversation_attachments.dart` uses
  `Image.file(File(path))` from `dart:io`, and the
  `pendingAttachments` state field is `List<String>` of paths.
  Refactoring the chat-attachment pipeline to flow `XFile` /
  `XFile.readAsBytes` end-to-end is a separate follow-up.

### What now works on web (XFile refactor — #2157)

Photo-upload flows that go through `MediaRepository.addMedia(file:
XFile)` work on every platform because `XFile.readAsBytes()` reads
from the filesystem on mobile and the blob URL on web:

- **Item creation (gear, event, request)** via the unified-create
  modal — Text and URL tabs work as on mobile; the Image tab uses
  the gallery picker on web (camera viewfinder is mobile-only).
  Once the photo is picked, the same `_uploadAndStartStream` flow
  runs the AI stream and lands the user on the preview card.
- **Profile photo edit** —
  `UserProfileViewModel.pickImageFromGallery` / `pickImageFromCamera`.
- **Community photo edit (carousel append + background replace)** —
  `CommunityEditViewModel.uploadMediaAndAppend`,
  `CommunityViewModel.pickImage*` / `pickVideoFromGallery`.
- **Carousel media add on gear/request/experience screens** —
  `*MediaActions` mixins pass XFile through end-to-end.
- **Stock-imagery candidate import (Replace Media)** —
  `UnifiedCreateViewModel.useCandidate`. This path was already
  web-safe (uses `addMediaFromURL` server-side fetch).
- **Feedback screenshot attach** —
  `feedback_sheet` and `feedback_screen` photo picker. The picker
  returns an XFile, and the screenshot path stored in state is
  used downstream by the feedback upload (mobile only — the
  blob URL on web wouldn't read through the upload path until the
  feedback flow follows the same XFile pattern).

### Defense-in-depth `dart:io` guards

For services that *do* still use `dart:io` directly, `kIsWeb` no-op
returns prevent runtime crashes if a future change makes them
reachable from a web route:

- `AppBadgeService.updateBadge` / `removeBadge` — `kIsWeb` ordered
  *before* the `dart:io Platform.isAndroid` check (Platform throws
  on web).
- `_GearAppState._initializeFCM` / `_checkForMissedNotification` —
  skip FCM on web (no service worker, and `fcm_service` uses
  `dart:io Platform` internally).

### Plugins not currently guarded

Some plugins in `pubspec.yaml` are reachable from screens the web
bundle exposes but haven't been individually audited:

- `flutter_local_notifications` — has a web implementation, but
  notifications fire only through `fcm_service` which is web-gated.
- `share_plus` — imported but unused at the time of #2157;
  imports may stay as dead code without runtime cost.
- `audio_session` — initialized in `main.dart` inside a `try/catch`,
  failure is non-fatal.

If a new screen / button reaches a plugin not on either list above
and crashes on web, the fix is to add a guard at the screen-level
entry point and a `kIsWeb` no-op at the plugin-helper level — same
pattern as everything above. Add a row to the tables here.

### What works on web

- Read flows for every primary screen — feed, gear detail, request
  detail, experience detail, profile, communities, chat.
- RPC-driven actions — RSVP, chat send/receive, accept/start loan,
  read shared communities, profile reads, etc.
- Authentication — email/password, Firebase phone (with reCAPTCHA),
  OIDC sign-in. Tokens persist across reloads via
  `SharedPreferences` (see "Auth persistence" above).
- **Photo edits on existing entities** — see the "What now works on
  web" table above. Profile photo and community photo flows are
  fully web-safe via the XFile upload pipeline.

The user-visible compromise on web is roughly: *the only thing
mobile-only is the in-app camera viewfinder and chat photo
attachments — everything else works.*

## Observability on web

**Server side is fully covered** by the existing logging middleware
(`server/middleware/request_id.go` + `server/middleware/logging.go`).
Every HTTP request — including the bundle-asset fetches at `/`,
`/main.dart.js`, etc. and every Connect RPC — gets:

- An `X-Request-ID` header (auto-generated when absent), threaded
  through `logging.LoggerWithContext(ctx)` into every log line.
- Sensitive-data masking (emails, tokens) via the
  `slog.LogValuer` redaction types.
- Coverage by the four production alert policies in
  `terraform/modules/monitoring/main.tf` —
  `Uptime Check Failed`, `RPC Errors Elevated`,
  `Health Check Unhealthy`, `Server Error Logged`. Each one already
  groups by `metric.label.operation`, so any new operation that
  shows up in logs after the apex-domain move is covered without
  Terraform changes.

See `docs/server/observability.md` for the full design.

**Client-side errors on web are a known gap.** On mobile,
`FlutterError.onError` and `PlatformDispatcher.instance.onError` in
`app/lib/main.dart` route through `_log.severe` *and*
`FirebaseCrashlytics.instance.recordError`. On web, the Crashlytics
calls are gated behind `!kIsWeb` because Crashlytics-web is still
in beta — so web errors only reach the browser console, with no
server-side or Crashlytics-dashboard sink.

The decision (per the #2157 plan): **accept the gap** until
Crashlytics-web reaches GA. The volume of web traffic in the
near term is small enough that the absence of dashboarded client
errors isn't a P0 problem, and an interim
`/internal/client-error` Connect RPC would add plumbing for a
window that closes itself.

If the gap becomes a real operational risk before Crashlytics-web
GA's, the easiest follow-up is:

- Add a thin Connect RPC `LogClientError(message, stack,
  request_id)` in a new `server/services/observability/` package.
- In `main.dart`'s zone error handler, post to it from web only
  (`if (kIsWeb) await _logClientError(...)`).
- The server logs at `ERROR` level, which is already covered by
  the `Server Error Logged` alert policy.

## Pinned Flutter version

This bundle is pinned to **Flutter 3.44.0 (stable)**, Dart 3.11.x.

The HTML renderer was removed in Flutter 3.27+; the `--web-renderer`
flag is gone, so renderer choice is expressed only by the `--wasm` build
flag.

## Renderer: WebAssembly with a JS fallback (#2119)

We build with `flutter build web --wasm` (all `build:web:*` variants).
That emits a **dual bundle**:

- `main.dart.wasm` + `main.dart.mjs` — the WASM path, loaded by modern
  browsers (Chrome/Edge 119+, Safari 16.4+, Firefox 120+).
- `main.dart.js` — a JS fallback that older browsers load instead,
  selected automatically by `flutter_bootstrap.js`. **No feature loss on
  older browsers**, only slower Dart execution — there is no hard browser
  cut-off to document.

**Single-threaded skwasm, no cross-origin isolation.** The `--wasm` build
ships two Skia renderers: `skwasm.wasm` (single-threaded) and
`skwasm_heavy.wasm` (multithreaded). `flutter_bootstrap.js` uses the
multithreaded one only when the page is cross-origin-isolated. We
deliberately send **no** `COOP`/`COEP` headers, so it uses the
single-threaded renderer — which works fully. Enabling
cross-origin isolation (`COEP: require-corp`) would break every
cross-origin subresource that doesn't send CORP/CORS headers — Mapbox
tiles, Firebase, the media CDN, Google Maps — so the multithreaded
renderer is a separate, deliberately-deferred optimization (tracked in
#2671).

Enabling `--wasm` was originally blocked on two dependencies
(`flutter_keyboard_visibility_web`'s `dart:html` import;
`flutter_timezone_web`'s JS-interop lint violations). Both cleared via
Renovate version bumps before the migration landed: `flutter_typeahead`
6.0.0 dropped the `flutter_keyboard_visibility` transitive dep, and
`flutter_timezone` 5.1.0 rewrote its web interop with typed
`dart:js_interop`.

## Compression

The `//go:embed` bundle is served **precompressed**. Flutter emits no
compressed assets and the Go handler doesn't compress on the fly, so
`scripts/precompress_web_bundle.js` runs at build time (inside every
`build:web:*`, before the copy into `app_assets/`) and writes a
`<file>.br` (brotli q11) and `<file>.gz` (gzip -9) next to each
compressible asset. `ServeApp` (`app_handler.go`) then negotiates on
`Accept-Encoding` — **brotli → gzip → identity** — setting
`Content-Encoding`, `Vary: Accept-Encoding`, and the *canonical*
`Content-Type` (so `main.dart.wasm.br` still serves `application/wasm`).
Direct requests for a `.br`/`.gz` URL 404; the siblings are reachable
only through negotiation. Net effect: a cold WASM visit downloads
~2.25 MB brotli instead of the 9.6 MB raw `main.dart.wasm`. We ship both
brotli and gzip so `br`-stripping proxies and the older-browser
JS-fallback tier still get compressed bytes.

## Bundle-size gate

`scripts/check_web_bundle_size.sh` sums the gzipped size of the WASM
initial chunk (`flutter_bootstrap.js` + `main.dart.mjs` +
`main.dart.wasm` — what a modern browser downloads; the `main.dart.js`
fallback isn't counted) and compares against the baseline in
`app/web/bundle_baseline.txt`. The initial-chunk gate fails when
measured > baseline × 1.10; a second baseline gates the total bundle at
× 1.20. The measured numbers are gzipped build artifacts (a size
*budget*); production serves the bytes brotli/gzip-precompressed (see
Compression above).

A baseline of `0` disables that gate; the script prints the current
measurement so a PR can commit a number. Baselines are currently set
(measured on the first `--wasm` build, #2119); reset them via the script
after an intentional bundle-size jump.

CI workflow: `.github/workflows/test_flutter_web.yaml`, triggered on
changes under `app/lib/**`, `app/web/**`, `app/pubspec.{yaml,lock}`,
`proto/**`, the script itself, or the workflow file.

## Platform.is\* / dart:io audit

The plan called for a `Platform.is*` / `dart:io` audit of the widget
tree rooted at `ExperienceContentView` (what the web shell will
compose in Step 7) plus the new `web_auth/` OTP widgets (Step 8).
Result as of the first successful `flutter build web --release`:

- **One transitive `dart:io` import**:
  `app/lib/presentation/widgets/content/inline_conversation_attachments.dart`
  imports `dart:io` for `Image.file(File(...))` to preview staged-
  but-not-yet-sent local chat attachments. Reached from
  `ExperienceContentView` via the chat pane.
- **Build outcome**: the web target compiles cleanly — verified on
  both the original JS build and the current `--wasm` build. `dart:io`
  is importable on web at compile time; runtime use of
  `File`/`Process`/etc. would throw, but `Image.file(...)` on the
  staged-attachment path is unreachable for guest users (we don't
  expose the chat compose UI on the web guest flow).
- **Conclusion**: no `kIsWeb` guard needed for v1. Document the
  reachability assumption here so that if a future change exposes
  chat compose on web, the audit catches the runtime risk.

The wider `Platform.is*` / `dart:io` set (~18 files outside the
`core/theme/` tree) is **not** in the audit scope. Per the plan
those files don't reach the guest bundle's initial chunk via the
route graph; Step 8's deferred-route configuration keeps them out.

## Firebase web initialization

Mobile auto-detects Firebase config from `google-services.json`
(Android) and `GoogleService-Info.plist` (iOS), so
`Firebase.initializeApp()` with no args works on those platforms.
Web has no equivalent auto-detection — `main.dart` passes explicit
`FirebaseOptions` via `webFirebaseOptions()` from
`app/lib/core/config/firebase_options_web.dart`.

The project-identifying fields (`authDomain`, `projectId`,
`storageBucket`, `messagingSenderId`, `appId`) are inline in
`firebase_options_web.dart`. The `apiKey` is **sourced from GCP
Secret Manager** (`firebase-web-api-key` in the matching project)
and injected at build time via `--dart-define=FIREBASE_WEB_API_KEY=…`,
matching how Mapbox tokens flow through the build:app:* / build:web:*
scripts. This keeps the rotation story consistent: rotate the secret
in Secret Manager, rebuild, no source change.

**One-time setup per Firebase project**:

```bash
# dev project (required before `npm run build:web:local` or
# `npm run build:web:dev`):
gcloud secrets create firebase-web-api-key \
  --replication-policy=automatic \
  --project="$DEV_PROJECT"
# Then add the secret value (from Firebase Console → Project
# settings → Your apps → Web → Config snippet → apiKey):
echo -n 'AIza…' | gcloud secrets versions add firebase-web-api-key \
  --data-file=- --project="$DEV_PROJECT"

# prod project (required before `npm run build:web:prod`): same as
# above against the prod project, with the prod apiKey.
```

The Mapbox token's `fetch_secret.sh ripls-{env} mapbox-access-token`
plumbing in package.json gives the model for what the build scripts
do here — same pattern, different secret name.

## Firebase App Check

App Check initialization with the reCAPTCHA v3 web provider lands
in Step 8 (web auth + RSVP actions). The site key is provisioned in
the Firebase Console and pushed to GCP Secret Manager as
`firebase-app-check-recaptcha-site-key-{dev,prod}`. The web entry
point reads it via `--dart-define=FIREBASE_APP_CHECK_SITE_KEY=...`
at build time.

Until Step 8, App Check is not initialized on the web build — the
bundle ships without anti-abuse on its Firebase Auth surface. This
is acceptable for the Step 6/7 placeholder shell because no live
auth surface is exposed.

## Build-time variables

`build:web` accepts the same `--dart-define` flags as native
builds. For dev:

```bash
flutter build web --wasm --release \
  --dart-define-from-file=env.dev.json \
  --dart-define=MAPBOX_ACCESS_TOKEN=$(scripts/fetch_secret.sh "$DEV_PROJECT" mapbox-access-token)
```

The `build:web:*` npm scripts wire these in — each fetches the matching
Mapbox / Firebase / Maps secrets from Secret Manager and passes them as
`--dart-define`s (see the "Variants by target environment" table above).
