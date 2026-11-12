# Issue #2162: Flutter Web bundle as an E2E UI test harness

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2162
**Filed:** 2026-05-25 | **Last activity:** 2026-05-29
**Labels:** (none) | **Assignees:** unassigned
**Context docs:** none (issue body + author's expanded-scope comment)

## Status (2026-05-29)

**Phase 0 + Phase 1 shipped** in PR
[#2243](https://github.com/Type2Fun-Foundation/ripls/pull/2243) on
branch `web_automation`. 25 commits. All 5 specs pass on
mobile-chromium locally. CI exercise pending merge. Phases 2 and 3
not started.

What's live:

- `e2e/` Playwright Test package (top-level npm)
- Connect-ES v2 TS proto codegen → `e2e/gen/`
- TS-side seed primitives (users, communities, experiences) via
  public Connect-Web RPCs
- `scripts/start_e2e_server.sh` + `e2e/lib/server.ts` — hermetic
  server boot (matches `server/integration_tests` pattern; no
  Secret Manager / gcloud)
- `e2e/lib/connect.ts` — Connect client with `X-Request-ID`
  propagation (`e2e-{spec-slug}-{seq}` for seed RPCs,
  `e2e-{spec-slug}-browser-{seq}` for bundle traffic)
- `e2e/lib/auth.ts` — pre-navigation auth-state injection
  (`access_token`, `refresh_token`, `user`, `server_url` — all
  four required, all JSON-stringified, all un-prefixed keys
  matching `SharedPreferencesAsync`)
- Five specs:
  - `tests/video-on-event-hero.spec.ts` — #2155 regression
  - `tests/guest-rsvp-confirm.spec.ts` — share-link RSVP handoff
  - `tests/ported-smoke.spec.ts` — 3 chromedp-equivalent checks
- `.github/workflows/test_web_e2e.yaml` — CI workflow (LFS,
  testcontainer Postgres, Chromium + WebKit, artifact upload on
  failure)
- Phase-0 a11y groundwork: `ensureSemantics()` at app start,
  `semanticsIdentifier:` on `Tappable`/`Toggle`/`IconAction`/
  `CachedMediaImage`, `web-event-screen` anchor on
  `WebExperienceScreen` (with the
  `explicitChildNodes: true, container: true` fix that's required
  for the identifier to actually reach DOM),
  `require_kebab_case_for_semantics_identifier` lint rule,
  `docs/client/testing/semantics_identifiers.md` convention doc
- `server/cmd/web-smoke/` (chromedp), `scripts/web_smoke.sh`,
  `.github/workflows/test_web_smoke.yaml` — **deleted** in the
  same PR (the planned two-week coexistence soak collapsed because
  the Playwright suite is a strict superset of the chromedp
  checks on first local run)

What deviated from the plan:

- `forbid_static_key_in_auth_screens` lint rule was drafted but
  **dropped from the PR**: it grandfathered 3 auth files into the
  allowlist, which the existing CI ratchet (`a11y_allowlist.txt`
  may only shrink on main) correctly rejected. Its real value
  materializes when migrating those widgets; deferred to a future
  PR alongside specs that need them.
- The Phase-1 widget-migration footprint shrunk to just the
  `web-event-screen` anchor. The plan's longer list (login +
  register field migrations, RSVP/email-capture identifiers) was
  over-eager — the Phase-1 specs land tokens via Connect-Web seed
  RPCs and inject them directly, so the UI auth forms aren't on
  the path. Identifiers for the rest of the widgets will land
  alongside Phase-2 specs that actually exercise them, per the
  "only when a test asks for it" rule in the convention doc.
- Six bugs were caught and fixed by running the suite locally
  before merging: missing `--jwt-signing-secret`, mobile-chromium
  project launching WebKit, raw localStorage encoding,
  `SharedPreferencesAsync` un-prefixed key shape, four-entry auth
  injection requirement (not just tokens), and the Semantics-
  anchor `explicitChildNodes` fix.

Open items / not done:

- WebKit project results not yet observed locally (specs only
  exercised on mobile-chromium); first CI run on this PR is the
  real first WebKit signal
- #2229 (`ensureSemantics()` Web first-paint perf audit) — open
- Phase 2 (multi-client interactive scenarios) — not started
- Phase 3 (walkthrough artefacts) — DELIVERED via #2684 (scene-based
  walkthrough pipeline: `e2e/tests/walkthroughs/`, `e2e/lib/walkthrough.ts`,
  `e2e/scripts/run_walkthrough.sh`; see `docs/walkthroughs.md`)
- Auth-screen patrol-era key migration + the deferred
  `forbid_static_key_in_auth_screens` lint rule — not started

Resolved follow-ups:

- #2241 (delete `server/cmd/web-smoke/`) — closed by this PR.

## Problem Statement

We have no automated way to catch client-side regressions that depend on the
real Dart widget tree rendering through a real browser. End-to-end coverage
today is server-driven (RPC-only integration tests) plus a 210-line `chromedp`
smoke (`server/cmd/web-smoke/`) that confirms the Flutter Web bundle boots,
does not throw, and lands on the expected URL. Anything richer — "the
`<video>` element on the event hero actually plays", "a guest's RSVP from a
share link actually confirms" — is currently manual.

The motivating example is #2155: a Web-only video playback bug whose unit
tests covered classification logic and compile-time platform-symbol layout,
but where no automated coverage exercised the production `createCachedVideo­
Controller` code path through a real browser. The same shape will keep
recurring as the Web rollout (#1195) lands more features.

The issue author also expanded the scope in a follow-up comment (2026-05-28)
to include:

1. Multi-client interactive simulation — several browsers running at once,
   driving the same world, interacting through the server. Mix of UI-driven
   and RPC-driven users.
2. Walkthrough-video generation — screen recordings of common journeys that
   re-render whenever the app changes.
3. Use of `server/simulation/` as the data-seeding primitive so test worlds
   are realistic (multi-user, populated, time-evolved).
4. Mobile-browser-first emulation; desktop secondary; OK to start with
   Chrome but extensible.

This document plans the harness that covers all three. It is the
re-introduction of "client-side E2E" that `docs/client/testing.md` Pitfall §4
currently says is out of scope post-Patrol-removal (#1887) — but on a
single-platform (Web) substrate that sidesteps the iOS+Android matrix
instability that doomed Patrol.

## Context from Comments

The author's 2026-05-28 comment significantly expands the issue's stated
goals. Key points:

- E2E test infrastructure should cover the client side, not just the server.
- Multi-client orchestration is in scope — "actually manipulating UI
  automatically, on multiple clients running at once and interacting with
  each other".
- Generating walkthrough videos that auto-update with the app is a use case.
- The existing `server/simulation/` library should be reused/extended to
  feed realistic data into test backends.
- A "broad and deep review of web-based UI automation" is part of the
  exercise.
- Primary target is mobile browsers; desktop-sized is extensible; Chrome-only
  start is fine.

No other comments. The plan therefore reflects an interactive design
dialogue (captured in §"Recommendation" and §"Open Questions") rather than
revised maintainer guidance.

## Landscape Survey

The author asked for a broad+deep review of web UI automation before
committing. The shortlist:

### Browser drivers (the layer that talks to the browser process)

| Tool | Language | Engines | Auto-wait locators | Mobile emulation | Video record | Trace viewer | Multi-context |
|---|---|---|---|---|---|---|---|
| **Playwright** | TS/JS/Py/Java/.NET | Chromium + WebKit + Firefox | Yes | Device presets | Per-context built-in | Yes | First-class |
| **Puppeteer** | JS | Chromium | Limited | Manual | 3rd-party | No | Manual |
| **chromedp** (status quo) | Go | Chromium | No | Manual flags | DIY (ffmpeg) | No | Multiple `NewContext`s |
| **Selenium / WebDriverIO** | All major | All | Limited | Manual | DIY | No | Manual |
| **Cypress** | TS/JS | Chromium + Electron + WebKit | Yes | Limited | Built-in | Yes | Single-tab |

**Playwright wins** for this project. Built-in mobile device presets cover
the comment's mobile-first ask without any manual UA/viewport plumbing.
Per-context `recordVideo` is the walkthrough pipeline. `browser.new­
Context()` is the multi-client primitive. WebKit support catches Safari-
specific regressions that Chromium-only tools miss. Auto-wait locators
eliminate the `chromedp.Sleep(12s)` flake. The Playwright Trace Viewer is a
direct upgrade for CI failure triage.

### Test runners (the layer that organises specs, fixtures, retries, reports)

| Tool | Best for | Limits |
|---|---|---|
| **Playwright Test (TS)** | First-class Playwright integration. Watch mode, fixtures, retries, parallelism, sharding, reporters, codegen, trace viewer, screenshot diff. | TS-only. New language for this repo. |
| **Jest / Mocha + Playwright** | If we already have one of them. | More plumbing. We don't. |
| **Go test + playwright-go** | Stays in Go ecosystem. | Bindings less polished than TS; some Playwright features lag the TS API. |
| **Dart `integration_test`** | Native widget-tree access. | Awkward for multi-client; weak orchestration; slow boot. |

**Playwright Test (TS)** wins, conditional on the author's tolerance for a
new top-level language. The author explicitly green-lit TypeScript in the
design dialogue and accepted "rewriting simulation primitives in TS if
needed".

### Flutter-aware drivers (the layer that knows about widgets)

| Tool | Status |
|---|---|
| **Patrol** | Removed in #1887; cross-platform mobile UI automation. Out of scope. |
| **`flutter_driver` + `integration_test`** | Dart-side widget tree access. Considered and rejected — multi-client orchestration too awkward and Phase-3 walkthrough pipeline isn't a natural fit. |
| **Semantics-tree-as-DOM** | Flutter's accessibility tree is serialised to ARIA-labelled DOM nodes on Web when `SemanticsBinding.instance.ensureSemantics()` is called. Stable cross-renderer. *Chosen targeting strategy.* |

### Recording & visual diffing (Phase 3 walkthrough)

- **Playwright `recordVideo`** — built-in, webm out-of-the-box per context.
  *Choice for Phase 3.*
- **Playwright `toHaveScreenshot`** — built-in pixel-diff with snapshot
  update mode. Out of scope for Phase 1; available for later.
- **Percy / Chromatic / Applitools** — paid hosted visual coverage. Out of
  scope.

### Mobile fidelity (the engine question)

- **Chromium with mobile presets** — Playwright's `devices['iPhone 14']`
  bundles viewport + UA + `hasTouch` + `isMobile`. Catches ~95% of mobile
  bugs; runs on the existing self-hosted Chromium binary.
- **Playwright WebKit** — Playwright bundles a patched WebKit that is much
  closer to Safari than Chromium-with-Safari-UA. Catches CSS / JS / layout
  bugs that only Safari exhibits. *Included in Phase 1.*
- **Real device cloud (BrowserStack/Sauce)** — Out of scope for now; revisit
  when WebKit-emulation false negatives become visible.

### What we already have that we keep

- `clock.SimulationTimestamp` middleware (dev-mode header → server clock).
  Time-travel for free.
- `EmailRegister` accepts empty `short_code` in dev-mode. User bootstrap
  for free.
- `AdminService` (dev-mode) for destructive cleanup.
- `server/simulation/` Go library — gear catalogs, personas, scenario
  timelines. Stays as the Go-side load-test driver; the TS harness
  re-implements the slice it needs (a small one) rather than calling into
  Go.
- `server/cmd/web-smoke/` chromedp binary — kept during the
  migration window; deleted in the same Phase-1 PR once the
  Playwright suite proved a superset of its three checks (which it
  did on first local run, so the two-week coexistence window was
  collapsed). The original plan called for the soak; the actual
  delivery skipped it.

## Existing accessibility footprint — and how the harness builds on it

This was the single biggest design question after the framework choice:
*how do Playwright tests find Flutter widgets when CanvasKit draws to a
single `<canvas>`?* A walk of the codebase changed the answer materially.
The harness should not invent a new tagging discipline; it should extend
the existing one.

### What's already there

| Signal | Count | Where |
|---|---|---|
| `Semantics(...)` wrappers | 105 (across 57 files) | All using `label: context.l10n.a11y…` |
| `semanticLabel:` on Icon/Image | 19 | Same l10n pattern |
| `Tappable` / `Toggle` / `IconAction` primitives | (used app-wide) | Required `semanticsLabel`, lint-enforced |
| `ValueKey('name')` / `Key('name')` | 74 | Concentrated on auth screens + form modals; Patrol-era leftovers |
| `Semantics.identifier:` | 0 | Field is unused today — the test-only ID slot is free |
| `SemanticsBinding.ensureSemantics()` | 0 | Tree-build not forced; only enabled when AT detects |

CI enforcement is strict: `scripts/check_a11y.js` has an empty allowlist
(every file passes), and `require_l10n_for_semantic_labels` rejects any
raw string literal on `tooltip:`, `semanticLabel:`, or `label/value/hint/
increasedValue/decreasedValue` inside `Semantics(...)`. The rule
deliberately omits `identifier:` — verified by reading the regex on
lines 252-280 of `scripts/check_a11y.js`. So test identifiers can stay
raw kebab strings while accessibility labels stay l10n-bound. That's the
narrow doorway the harness needs.

### Flutter Web DOM serialization (the bridge that makes Playwright work)

When `SemanticsBinding.instance.ensureSemantics()` is called at app
startup (or assistive tech is detected), Flutter Web renders a parallel
`<flt-semantics>` DOM tree alongside the canvas. Each semantics node
becomes a DOM element with:

- `aria-label="<the label>"` — from `Semantics(label: ...)`. Localized
  in our codebase, so unreliable as a stable selector across locales.
- `flt-semantics-identifier="<the identifier>"` — from `Semantics(
  identifier: ...)`. Not spoken by screen readers. Stable.
  ([Flutter source: SemanticsObject.updateIdentifier in
  `flutter/lib/web_ui/lib/src/engine/semantics/semantics.dart`.])
- ARIA role attributes, `aria-checked`, `aria-pressed`, etc., as
  appropriate.

Playwright queries it with `page.locator('[flt-semantics-identifier="
event-hero-video-player"]')`. Fast, cross-browser, locale-agnostic.

`ValueKey` does NOT appear in the DOM. It's only reachable through the
Flutter `integration_test` driver bridge, which the author's design
choice (Playwright over Flutter `integration_test`) rules out. The 74
patrol-era `Key('…')` instances are therefore dead weight for the
Playwright path.

### Strategy: extend the existing primitives

Rather than scatter standalone `Semantics(identifier: ...)` wrappers
across the codebase, **add an optional `semanticsIdentifier:` parameter
to `Tappable`, `Toggle`, `IconAction`, and `CachedMediaImage`.** When
set, the primitive's existing `Semantics(...)` wrapper gains the
identifier; when null (default), nothing changes. One-line opt-in for any
widget the e2e suite needs to target:

```dart
Tappable(
  semanticsLabel: context.l10n.a11yRsvpYes,   // existing, l10n-bound
  semanticsIdentifier: 'event-hero-rsvp-yes', // new, raw kebab
  onTap: _handleRsvp,
  child: ...
)
```

Bare `TextField`s (the auth + form-modal patrol-era hotspots) get
wrapped in `Semantics(identifier: 'login-email-field', child: ...)` once;
the existing `Key('login_email_field')` is deleted in the same diff as
dead weight.

This means the Phase-0 footprint is smaller than originally drafted:
**no new tagging discipline, no new convention doc beyond a one-pager,
~10-15 widgets touched for Phase 1 — the auth screens are already
half-tagged via the patrol-era keys we're replacing.**

### Lint surface

Two small additions to `scripts/check_a11y.js`:

1. `require_kebab_case_for_semantics_identifier` — `Semantics(identifier:
   '…')` and the new primitive parameters must use lowercase letters,
   digits, and hyphens only. No l10n key. No spaces. Prevents drift
   like `Semantics(identifier: 'Event Hero Video Player')`.
2. `forbid_value_key_in_auth_and_form_modal` — `ValueKey('…')` / `Key(
   '…')` in `app/lib/presentation/screens/auth/` and any file ending in
   `*_modal.dart` triggers a violation. The patrol-era keys we delete
   in Phase 0 don't return as muscle-memory copy-paste later.

Both rules ship with empty allowlists (ratchet pattern, like the
existing rules).

## Potential Solutions

### Option A: TypeScript Playwright Test in a top-level `e2e/` package *(Recommended)*

- **Approach:** New top-level `e2e/` npm package with Playwright Test as the
  runner. Generated TS Connect-Web clients (`@connectrpc/connect-web` +
  `@bufbuild/protobuf`) under `e2e/gen/`. Tests target the Flutter
  Semantics-tree DOM serialisation via ARIA-label locators. A small TS
  module reimplements the slice of `server/simulation/` we need (persona
  names, gear catalog, scenario flows). The existing dev-mode RPCs
  (EmailRegister, the simulation-clock header, AdminService cleanup) are
  the only privileged surface — **no new SimulationService**. Mobile
  Chromium + WebKit are both Phase-1 targets. Walkthrough-video is `record­
  Video` artefacts only in Phase 3.
- **Pros:** Industry-standard tool with the most batteries for our scope.
  No new server surface to maintain. Re-uses generated proto contract — TS
  tests stay in sync with the wire format automatically. Mobile + WebKit
  coverage that the Chromium-only baseline doesn't have. Per-context video
  is one config flag away. Trace viewer dramatically shortens CI-failure
  triage.
- **Cons:** TypeScript is a third top-level language alongside Go and Dart.
  Adds `protoc-gen-es` + `protoc-gen-connect-es` to codegen. ~300MB of
  browser binaries on the CI runner (`npx playwright install`). TS-side
  simulation primitives are duplicated from Go; minimal duplication today
  (personas + catalogs are small) but will grow.
- **Effort:** Large for Phase 1 (~2 weeks for a single developer);
  Phases 2-3 medium each.

### Option B: Grow the existing `server/cmd/web-smoke/` chromedp binary into a Go-side harness

- **Approach:** Extend the 210-line chromedp binary into a library:
  `server/web_e2e/` with a `Driver`, `Locator`, `MobileViewport`,
  `Recorder`. Tests live as Go integration tests under
  `server/integration_tests/web/`. Mobile-viewport plumbing is hand-rolled
  on top of CDP. Video recording goes through headed Chrome + an ffmpeg
  capture sidecar. WebKit support requires a second tool (or skipping).
- **Pros:** Stays in Go. No new language. Direct in-process access to the
  Go simulation library. No browser-binary install on the runner.
- **Cons:** We end up hand-rolling ~70% of what Playwright gives us free.
  Mobile emulation, video record, locator auto-wait, trace viewer all need
  implementation. WebKit is effectively unavailable (no maintained
  Go-CDP-for-WebKit driver). The walkthrough pipeline becomes an
  ongoing engineering project instead of a `recordVideo: { dir: …}` option.
- **Effort:** Large for Phase 1 (~3-4 weeks); Phases 2-3 large each.

### Option C: Dart `integration_test` + `flutter drive` against Chrome

- **Approach:** Use Flutter's first-party `package:integration_test` to
  drive a `flutter drive` run against headless Chrome. Tests are Dart
  files that import the app, find widgets via `find.byValueKey(...)`, and
  assert against the widget tree.
- **Pros:** Tests author in the same language as the app code; widget-tree
  access is native, no Semantics tagging required for assertions. Already
  in the Flutter ecosystem.
- **Cons:** Multi-client orchestration is awkward — `flutter drive` is one
  process per run. Multi-browser-engine (WebKit) is not supported. Video
  recording requires DIY. Seeding from `server/simulation/` requires an
  out-of-process RPC anyway. Slower bundle boot than a pre-built profile
  bundle.
- **Effort:** Medium for Phase 1; Phases 2-3 medium-to-large.

## Recommendation

**Option A.** This was a strong recommendation across all four design forks
in dialogue with the author, and the author confirmed it. The remaining
fork — whether to ship a dev-mode `SimulationService` for cross-process
seeding — was resolved during dialogue in favour of "use the existing
public RPCs + existing dev-mode hooks", which is materially less code and
no new privileged server surface.

The phased shape:

1. **Phase 1 — Regression catch.** Two scenarios (video-on-event-hero,
   guest RSVP confirm) + mobile-Chromium and WebKit projects, both on the
   self-hosted runner. `server/cmd/web-smoke/` was deleted in the same
   PR — the Playwright suite proved a superset of its three checks
   (`/event/{fake-id}`, unauth `/feed`, authed `/feed`) on first local
   run, so the planned two-week coexistence window was collapsed.
2. **Phase 2 — Multi-client.** Tests spawn multiple browser contexts +
   RPC-only background users (via the same TS seed library) in the same
   scenario, orchestrated by Playwright. No premature helper class — tests
   handcraft until ~3 multi-client scenarios exist, then we refactor.
3. **Phase 3 — Walkthrough-video artefacts.** Per-context `recordVideo`
   webm files uploaded as CI artefacts; no post-processing (no phone-
   frame composite, no captions, no voiceover). Stitching/branding is
   walkthrough-team workflow, out of this issue's scope.

## Open Questions

- **TS proto codegen pipeline placement.** `protoc-gen-es` and
  `protoc-gen-connect-es` need adding to a new `buf.gen.ts.yaml`, output
  directory under `e2e/gen/`, and `npm run generate` extended to call it.
  *Recommendation:* keep the per-language codegen file pattern (`buf.gen.
  go.yaml`, `buf.gen.dart.yaml`, new `buf.gen.ts.yaml`); add a
  `lint:ts-protos` script later if generated TS drifts from API protos.

- **Semantics-identifier naming convention.** Identifiers are raw kebab
  (not l10n keys) and pass through the new lint rule
  `require_kebab_case_for_semantics_identifier`. *Recommendation:*
  flat kebab without project prefix (`event-hero-video-player`, not
  `ripls:event-hero-video-player`). The Semantics tree is already
  namespaced by ancestor route; a synthetic prefix is noise. The lint
  rule enforces the kebab shape.

- **Should `Tappable`/`Toggle`/`IconAction`/`CachedMediaImage` gain
  `semanticsIdentifier:`?** Required for the primitive-extension
  strategy described above. *Recommendation:* yes, ship as Phase-0
  task. Optional parameter; default null; pass-through to the
  internal `Semantics(...)` wrapper. Update each primitive's README in
  `app/lib/presentation/widgets/accessibility/`. Test-author guidance
  in `docs/client/testing.md` directs people to the parameter, not
  standalone `Semantics(...)` wrappers.

- **Patrol-era `ValueKey`/`Key` migration.** 74 instances; ~40 of them
  on auth + form-modal `TextField`s; the rest scattered. *Recommendation:*
  Phase 0 migrates only the Phase-1-needed ones (~6-8 keys on the auth +
  email-capture flow); Phase 1 ships with a lint rule
  (`forbid_value_key_in_auth_and_form_modal`) so the pattern doesn't
  re-grow; an "audit & delete remaining patrol-era keys" follow-up issue
  is filed at end of Phase 1.

- **Bundle build mode for the e2e suite.** Current Web Smoke uses `build:
  web:local` = `--profile`. *Recommendation:* keep profile mode for e2e
  too — fast enough to be tolerable, observable enough to triage from
  trace + console. Production mode adds minification that wrecks
  stack-trace readability without catching new bugs.

- **`ensureSemantics()` gate.** We need `SemanticsBinding.instance.ensure­
  Semantics()` enabled in the e2e bundle but not necessarily on every
  production load. *Recommendation:* enable unconditionally on Web —
  enabling Semantics is a real accessibility improvement (and the
  existing a11y plan in `docs/ai/accessibility_plan.md` argues for this
  end-state anyway), not a test-only hack. If perf measurably regresses,
  gate by dart-define `RIPLS_ENABLE_SEMANTICS=true` set by the e2e build
  script. Perf cost tracked in #2229.

- **CI cost target.** Adding WebKit to every PR roughly doubles e2e
  runtime per scenario. *Recommendation:* run Chromium-mobile on every
  PR; run Chromium + WebKit on push to main and on `workflow_dispatch`.
  Sharding via Playwright's `--shard=N/M` once we have >6 scenarios.

- **Failure artefact retention.** Playwright produces trace.zip, video.webm,
  screenshot.png, and `console.log` per failed test. *Recommendation:* 7-
  day retention on GitHub Actions artefacts (default), trace.zip only on
  failure (not success). Open a follow-up if storage cost grows.

- **Phase-2 deferral marker.** The author chose "defer the helper to Phase
  2.5". *Recommendation:* hard-code that decision in
  `e2e/MULTI_CLIENT.md` and the Phase-2 phase doc — three scenarios first,
  then refactor.

- **`server/cmd/web-smoke-db/` future.** That binary already exists to spin
  up a testcontainer Postgres for the web-smoke job. *Recommendation:*
  re-use as-is for the e2e suite. No changes needed for Phase 1.

- **Test-server `--log-format` for CI.** The current `test_web_smoke.yaml`
  starts the server with `--log-format=text` (human-friendly stream into
  `/tmp/web-smoke-server.log`). `docs/server/observability.md` is explicit
  that production uses JSON for automatic indexing and that text is for
  dev. The e2e workflow sits between the two — humans triage failures from
  it, but the structured fields (`request_id`, `operation`, `user_id`)
  matter for correlating a Playwright failure to the server slog line that
  caused it. *Recommendation:* run the e2e server with `--log-format=json`
  in CI; have `e2e/lib/server.ts` capture the log file path and surface it
  in failure artefacts so a triager can `jq` the file by `request_id`.
  Local dev keeps `text` for readability.

- **TS-side request-ID propagation.** `docs/server/observability.md`
  requires every request to carry an `X-Request-ID`; the server's
  `middleware.RequestID` auto-generates one if missing, but a test-scoped
  ID lets us correlate Playwright failures to specific server log lines.
  *Recommendation:* the seed library wraps Connect-Web client construction
  in `e2e/lib/connect.ts`, which attaches an `X-Request-ID` header per
  call using the format `e2e-{spec-slug}-{seq}` (e.g.
  `e2e-video-on-event-hero-7`). The Playwright `page.route` handler does
  the same for browser-side RPC traffic so the bundle's `RequestId­
  Generator` is overridden in test runs. Failures upload the captured
  server log alongside `playwright-report/`; triagers grep by request-ID
  prefix to find the corresponding slog entries.

- **e2e tests as a complement, not a replacement.** `docs/client/testing.
  md` is emphatic that "we test business logic, not Flutter widgets" —
  ViewModel coverage stays the primary signal for product correctness.
  The Phase-1+ e2e suite covers a different layer: the integration
  boundary between Flutter Web, the bundle bootstrap, the Semantics tree,
  and the Connect-Web wire format. ViewModel tests will keep catching
  every business-logic regression they catch today. *Recommendation:*
  the Phase-1 `docs/client/testing.md` update explicitly frames e2e
  tests as "tests of the integration boundary, not a replacement for
  ViewModel coverage", and the structure tree gains a top-level `e2e/`
  entry beside `test/`.

## Implementation Plan

### Phase 0 — Codegen + Semantics foundations

**Status:** ✅ Shipped in PR #2243. One deviation: the
`forbid_static_key_in_auth_screens` lint rule was drafted but
dropped from the PR (allowlist-ratchet rejected the 3 grandfathered
entries; rule re-lands alongside specs that need those widgets).
The Phase-1-needed-widgets migration footprint also shrunk to just
the `web-event-screen` anchor — see the Status section at the top
for the rationale.

**Goal:** Make the TS test process able to call Connect RPCs, make the
Flutter Web bundle queryable via the existing accessibility primitives,
and lay down the lint rules that keep the discipline from drifting. No
tests yet — just the scaffolding the test author needs to write any test
at all.

**Tasks:**

- [ ] Add `buf.gen.ts.yaml` with `protoc-gen-es` + `protoc-gen-connect-es`,
  output to `e2e/gen/`. Add to `npm run generate`.
- [ ] Add `e2e/package.json` with `@playwright/test`, `@connectrpc/
  connect-web`, `@bufbuild/protobuf`. Pin versions.
- [ ] Add `e2e/README.md` following the project convention (every
  multi-file directory has a README): purpose, key files, how to run
  locally, how to write a new spec, where Semantics identifiers come
  from, and the dependency-upgrade policy. Style models: `server/ai/
  README.md` for large packages, `app/lib/presentation/widgets/
  creation/README.md` for small ones.
- [ ] Add `e2e/playwright.config.ts` with two `projects`: `mobile-
  chromium` (`devices['iPhone 14']`) and `webkit` (`devices['iPhone 14
  Safari']`). `recordVideo: { mode: 'retain-on-failure', dir: 'test-
  results/videos' }` (Phase-1 default; Phase-3 promotes to `'on'`).
- [ ] Add `SemanticsBinding.instance.ensureSemantics()` call at the
  start of `app/lib/main.dart` (kIsWeb branch). Document in
  `app/lib/README.md`. File a perf-audit follow-up issue to measure
  first-paint cost on Web; block Phase 1 only if it regresses >5%.
- [ ] **Extend the existing accessibility primitives** under `app/lib/
  presentation/widgets/accessibility/` with an optional
  `semanticsIdentifier:` parameter:
  - `Tappable` — pass through to internal `Semantics(identifier: ...)`.
  - `Toggle` — same.
  - `IconAction` — same.
  - `CachedMediaImage` — same (under `app/lib/presentation/widgets/
    chat/cached_media_image.dart`).
  Update each primitive's README. Default null; behaviour unchanged
  when the parameter isn't passed.
- [ ] Two new lint rules in `scripts/check_a11y.js`:
  - `require_kebab_case_for_semantics_identifier` — `Semantics(
    identifier: '…')` and the new primitive parameters must match
    `/^[a-z0-9-]+$/`. Catches drift like `'Event Hero Video Player'`.
  - `forbid_value_key_in_auth_and_form_modal` — `ValueKey('…')` and
    `Key('…')` in `app/lib/presentation/screens/auth/` and any file
    ending in `*_modal.dart` produce a violation. Allowlist starts
    populated with the existing patrol-era keys; Phase-0 deletes the
    Phase-1-needed ones from the allowlist (and the source) in the
    same diff; remaining entries are cleaned up by the end-of-Phase-1
    follow-up issue.
- [ ] Add a one-pager `docs/client/testing/semantics_identifiers.md`:
  the kebab convention, when to add an identifier (test author asks
  for one — not speculatively), how the primitive pass-through works,
  and a pointer to the lint rule.
- [ ] Migrate the Phase-1-needed widgets in-place. ~10-15 touches:
  - `app/lib/presentation/screens/auth/login_screen.dart` — replace
    `Key('login_email_field')` / `Key('login_password_field')` with
    `Semantics(identifier: 'login-email-field', ...)` (wrapping the
    `TextField`) and same for password. Delete the keys.
  - `app/lib/presentation/screens/auth/register_screen.dart` — same
    pattern for email + name + password.
  - `app/lib/presentation/screens/experience/web_experience_screen.dart`
    — the existing top-level `Semantics(liveRegion: true, ...)` wrapper
    on the hero gains `identifier: 'event-hero'`; the video child gets
    a nested `Semantics(identifier: 'event-hero-video-player')`.
  - The RSVP yes/no buttons (existing `Tappable`s on the event page) gain
    `semanticsIdentifier: 'event-hero-rsvp-yes'` / `'…rsvp-no'`.
  - Email-capture submit `Tappable` on the unauth event landing gains
    `semanticsIdentifier: 'email-capture-submit-button'`.

**Validates:** Running `npm run generate` produces TS clients without
errors. Running `npm run lint:dart:a11y` passes (no regressions). A dev
build opened in Chrome shows the expected `flt-semantics-identifier`
attributes on the migrated widgets — verified by
`document.querySelectorAll('[flt-semantics-identifier]')` in DevTools.
The Phase-1 anchor scenario can then find each tagged widget via
`page.locator('[flt-semantics-identifier="…"]')`.

### Phase 1 — Regression catch

**Status:** ✅ Shipped in PR #2243. All 5 specs pass on
mobile-chromium locally; WebKit results pending first CI run.
The chromedp removal landed in the same PR (planned two-week soak
collapsed). Six bugs caught during local exercise are documented
in the commit log.

**Goal:** Two scenarios live in CI on mobile Chromium and WebKit, both
catching the kind of bug #2155 was — and at end of phase, delete `server/
cmd/web-smoke/` because the new suite covers its three checks.

**Tasks:**

- [ ] `e2e/lib/server.ts` — boots the test server in a child process,
  polls `/health`, returns `{baseUrl, logPath, kill}`. **Delegates the
  flag set to a thin shell helper**, not duplicating it in TS — the
  existing `scripts/run_server_with_sm_secrets.sh` + `scripts/web_smoke.
  sh` pair already owns ~10 startup flags; factor the e2e-relevant
  subset into a new `scripts/start_e2e_server.sh` that the TS helper
  shells out to. Avoids drift when a future server change adds a
  required flag. CI uses `--log-format=json` (per the open-question
  recommendation); local dev uses `--log-format=text`. The captured
  `logPath` is surfaced in failure artefacts.
- [ ] `e2e/lib/connect.ts` — wraps Connect-Web client construction with
  a per-test `X-Request-ID` header (`e2e-{spec-slug}-{seq}`) so seed
  RPC failures correlate to server slog. Also installs a Playwright
  `page.route` handler that injects the same prefix on browser-side
  RPC traffic, so the Flutter `RequestIdGenerator`'s output is
  overridden in test runs.
- [ ] `e2e/lib/seed/users.ts` — `registerUser(serverUrl, email, name)`
  using Connect-Web against `LoginService.EmailRegister`. Returns
  `{accessToken, refreshToken, userId}`.
- [ ] `e2e/lib/seed/communities.ts` — `createCommunity(...)`,
  `inviteUser(...)` using `CommunityService` RPCs.
- [ ] `e2e/lib/seed/experiences.ts` — `createExperienceWithVideo(...)`
  using `ExperienceService` + `MediaService`. Uploads a 522 KB mp4
  fixture from `e2e/fixtures/`.
- [ ] `e2e/lib/auth.ts` — `injectTokens(page, accessToken, refreshToken)`
  via Playwright's `context.addInitScript` writing
  `flutter.access_token` and `flutter.refresh_token` to
  `localStorage` before any bundle script runs. Direct port of the
  chromedp `addScriptToEvaluateOnNewDocument` pattern.
- [ ] `e2e/fixtures/video.mp4` — copy of
  `website/content/crew/screens/chat-in-context.mp4` (522 KB). Track via
  Git LFS to keep the diff small.
- [ ] `e2e/tests/video-on-event-hero.spec.ts` — Phase-1 anchor:
    1. Seed: a user, a community, a hero-video experience.
    2. Auth: inject tokens.
    3. Navigate to `/event/{seeded-id}`.
    4. Assert: `getByLabel('event-hero-video-player')` becomes visible.
    5. Assert via `page.evaluate(...)`: the underlying `<video>` element
       reaches `readyState >= 2` within 10s.
    6. Assert: `video.error === null`.
    7. Take a per-element screenshot to catch the "video element exists
       but the frame is black" failure mode.
- [ ] `e2e/tests/guest-rsvp-confirm.spec.ts` — Phase-1 second scenario:
    1. Seed: a host, a community, an experience with `?rsvp=yes&code=…`
       capture enabled.
    2. Navigate (unauth) to `/event/{seeded-id}?rsvp=yes&code=…`.
    3. Fill the email-capture field; click submit.
    4. Assert: final URL path is `/event/{seeded-id}` (auth landed) and
       the RSVP state visible on the page reflects "confirmed".
    5. Assert via Connect RPC from the test: the seeded experience now
       has an RSVP from the captured email.
- [ ] `e2e/tests/ported-smoke.spec.ts` — port of the three existing
  `server/cmd/web-smoke/` checks (`/event/{fake-id}`, unauth `/feed`
  redirect, authed `/feed` render) so we can delete the Go binary.
- [ ] `.github/workflows/test_web_e2e.yaml` — new workflow. Mirrors the
  existing `test_web_smoke.yaml` setup (LFS, gcloud, Chrome install,
  testcontainer Postgres, server startup, flutter build). Replaces the
  chromedp navigation steps with `cd e2e && npx playwright install
  --with-deps chromium webkit && npx playwright test`.
- [ ] On-failure artefact upload: `e2e/playwright-report/` (HTML) +
  `e2e/test-results/` (video, trace, screenshot). 7-day retention.
- [ ] Update `docs/client/testing.md`:
  - Pitfall §4 / E2E Testing section now points at `e2e/` as the
    canonical client-side E2E path.
  - The "Test Structure" tree gains a top-level `e2e/` entry alongside
    `test/`, with a short note that `e2e/` runs Playwright Test against
    the built Flutter Web bundle and is complementary to (not a
    replacement for) ViewModel coverage.
  - The "Testing Philosophy" section gains one sentence: "ViewModel
    tests remain the primary signal for business-logic correctness;
    the e2e suite covers the integration boundary between Flutter
    Web, the bundle bootstrap, and the wire format."
  - The "Integration Testing" section's "previous Patrol attempt was
    removed (#1887)" paragraph is rewritten to reflect the web-only
    scope choice and to point at the new harness.
  - iOS+Android mobile-native coverage is still explicitly out of
    scope; no plan to reintroduce Patrol.
- [x] Delete `server/cmd/web-smoke/` (binary + its workflow) — done
  in the same PR; the Playwright suite is a superset on first local
  run. #2241 closed as fixed.

**Validates:** The video-on-event-hero spec fails when the #2155 fix is
reverted; passes today. The guest RSVP spec fails when the email-capture
flow regresses; passes today. The Semantics-identifier approach is proven
to be queryable from Playwright in both Chromium and WebKit.

### Phase 2 — Multi-client interactive scenarios

**Status:** ⏳ Not started.

**Goal:** Tests can spawn N browser contexts + M RPC-only background users
in the same scenario and assert on cross-user state propagation. No helper
class — handcraft until 3 scenarios exist (deferred per author's choice).

**Tasks:**

- [ ] `e2e/lib/seed/background.ts` — `spawnRpcUser(serverUrl, persona)`
  returns an authenticated Connect client and an `act()` method that
  takes timeline-style actions ("SaveGear", "SendMessage", …) — a thin
  TS port of `server/simulation/executor.go`'s public surface. ~150
  lines.
- [ ] Scenario A: `e2e/tests/multi-client-rsvp.spec.ts` — Alice (mobile
  Chromium browser) RSVPs to Bob's experience; Bob (mobile Chromium
  browser) observes the attendee-count update within 5s. Three RPC-only
  decoy users in the same community.
- [ ] Scenario B: `e2e/tests/multi-client-loan.spec.ts` — Owner (browser)
  shares gear; Borrower (browser) requests it; Owner confirms in their
  inbox. RPC-only background creates 5 other gear items so feed-position
  assertions are realistic.
- [ ] Scenario C: `e2e/tests/multi-client-chat.spec.ts` — Two browsers
  exchange messages in a perpetual conversation; assertions cover
  message ordering, read-receipts, and the unread badge in the third
  party's inbox.
- [ ] After Scenario C lands, refactor the common multi-context plumbing
  into `e2e/lib/multi-client.ts` (deferred-helper extraction).
- [ ] `docs/client/testing.md` updated with a multi-client section.

**Validates:** The harness handles N browser contexts × M RPC users
without timeouts or flakes; the dev-mode `clock.SimulationTimestamp`
header is used at least once for a time-dependent assertion (e.g., RSVP
deadline expiry); the deferred-helper refactor produces a cleaner Scenario
B+C diff than the pre-refactor versions.

### Phase 3 — Walkthrough-video artefacts

**Status:** ⏳ Not started.

**Goal:** Per-scenario webm files exported on every run, suitable as raw
material for the walkthrough team's editor. No stitching, framing, or
narration in this phase.

**Tasks:**

- [ ] `e2e/playwright.config.ts` — promote `recordVideo` from `retain-
  on-failure` to `on` for a new `walkthrough` project. The project
  filters to specs tagged `@walkthrough` (Playwright supports tag-based
  project filtering since v1.35).
- [ ] Three Phase-3 specs, each tagged `@walkthrough`:
    1. `e2e/tests/walkthroughs/share-event.spec.ts` — host creates an
       experience, copies the share link, "sends" it (via clipboard
       assertion).
    2. `e2e/tests/walkthroughs/borrow-gear.spec.ts` — owner shares a power
       drill, borrower discovers it in feed, taps to express interest,
       the request lands in the owner's inbox.
    3. `e2e/tests/walkthroughs/rsvp-and-attend.spec.ts` — guest follows a
       share link to an experience, RSVPs, sees confirmation.
- [ ] `docs/walkthroughs.md` — how to run the walkthrough project locally
  (`npx playwright test --project=walkthrough`), where the webm
  files land, the cadence at which they should be regenerated, and the
  intentional scope-limit ("we ship raw webm; the editor is downstream").
- [ ] CI: `walkthrough` runs on `workflow_dispatch` only (not on
  PR or push) to keep PR latency stable. Artefacts uploaded with 30-day
  retention so the walkthrough team can fetch them on demand.

**Validates:** Walkthrough team can download a fresh per-scenario webm from
the most recent `workflow_dispatch` run without engineer involvement. The
Phase-1 + Phase-2 specs continue to run on mobile Chromium + WebKit; the
`walkthrough` project is additive, not blocking.

## Related Issues

- #2155 — original web video playback bug; the motivating example.
  Phase-1 anchor spec asserts the fix.
- #2050 — Web Smoke pipeline + `/app/event/{id}` route that this harness
  inherits and supersedes.
- #1887 — Patrol removal. This issue's "single-platform Web" framing is
  the direct response to the mobile-matrix instability that doomed Patrol.
- #1195 — broader Web rollout; this harness becomes increasingly load-
  bearing as the Web bundle covers more product surface.
- #2157 — drop the legacy `/app/` prefix; routes assumed by Phase-1
  specs are the apex-domain catch-all routes from that issue.
- #2229 — perf audit of `ensureSemantics()` on Web first-paint. Block
  Phase-1 only if it regresses >5%.
- #2241 — delete `server/cmd/web-smoke/` and remove
  `test_web_smoke.yaml`. Resolved in the same PR.

---

## Audit Notes (`/ripls-audit`, 2026-05-28)

Audit pass against `docs/client/{architecture,caching,i18n,accessibility,
testing}.md` and `docs/server/{architecture,observability}.md`. Findings
and the targeted fixes applied to this plan:

1. **Request-ID propagation in TS tests.** `docs/server/observability.md`
   makes the HTTP correlation `request_id` a first-class invariant: every
   RPC carries one, services log under it, alerts group by it. The plan
   originally said nothing about how the TS test process would tag its
   RPCs. *Fix:* added a new Open Question and a Phase-1 task for
   `e2e/lib/connect.ts` — wraps Connect-Web client construction with an
   `X-Request-ID` header of the form `e2e-{spec-slug}-{seq}`, and installs
   a Playwright `page.route` handler that overrides the bundle's own
   `RequestIdGenerator` on browser-side RPC traffic. Failure artefacts
   include the server log path so triagers can `jq` by request-ID prefix.

2. **Test-server boot delegates to a shared shell helper.** The existing
   web-smoke workflow boots the server via `scripts/run_server_with_sm_
   secrets.sh` with ~10 flags. Duplicating that flag list inside
   `e2e/lib/server.ts` would drift the moment a new required flag lands
   server-side. *Fix:* the Phase-1 task for `e2e/lib/server.ts` now
   explicitly delegates to a new `scripts/start_e2e_server.sh` that owns
   the flag set; TS just shells out and waits on `/health`.

3. **Log format for CI: `json`, not `text`.** Per
   `docs/server/observability.md`, JSON is the production format because
   `request_id` / `operation` / `user_id` become indexable. The current
   web-smoke uses `text` (human-readable) because the only consumer is
   eye-grep at PR-failure time. With the request-ID propagation in (1),
   triagers can now grep structured fields when a Playwright test fails.
   *Fix:* added an Open Question pinning `--log-format=json` for CI
   server boot and `text` for local dev.

4. **e2e tests positioned as complement, not replacement.**
   `docs/client/testing.md` opens with "Test business logic, not Flutter
   widgets" — ViewModel coverage is the primary signal. The plan needs
   to avoid the (easy) misreading that "now we have e2e tests, so we
   can skip ViewModel tests for new features". *Fix:* added an Open
   Question framing e2e as the integration-boundary layer and
   strengthened the Phase-1 `docs/client/testing.md` update task to
   carry that framing into the doc — including a new sentence in the
   "Testing Philosophy" section and a top-level `e2e/` entry in the
   "Test Structure" tree.

5. **Project convention: every multi-file directory has a README.**
   `CLAUDE.md` requires this for `server/` and `app/lib/`. `e2e/` is
   top-level so the rule doesn't technically apply — but the conventions
   for naming, layout, and README discipline should still propagate to
   a new top-level package. *Fix:* the Phase-0 task for `e2e/README.md`
   is now explicit about being a directory README (purpose, key files,
   how to run, how to write a new spec) rather than a "freeze
   dependencies" notes file. Style models cited.

Items audited and found compliant (no changes):

- **MVVM / Repository / no business logic in widgets** — the plan only
  touches widget-level Semantics tagging; no ViewModel, Repository, or
  Service code is added or modified.
- **Stash-based caching** — the harness never touches caches. Bundle uses
  its existing repositories; TS tests call public RPCs unguarded.
- **`require_l10n_for_semantic_labels` lint rule** — verified
  `scripts/check_a11y.js` already omits `identifier:` from its raw-string
  check; the new `semanticsIdentifier:` parameter pattern doesn't
  collide. The two new lint rules
  (`require_kebab_case_for_semantics_identifier`,
  `forbid_value_key_in_auth_and_form_modal`) follow the existing ratchet
  pattern.
- **Server architecture / service-independence** — the plan introduces
  **no new Go service** (rejected `SimulationService` in design dialogue
  in favour of public RPCs + existing dev-mode hooks). No new server
  dependencies, no goroutines, no error-wrapping changes.
- **Phase-marker / TODO discipline** — all forward-looking follow-ups are
  listed in `Related Issues` as "To file" items with stable conditions,
  not as bare TODOs in source. The plan-level guidance to pre-file the
  cleanup issue at the start of Phase 0 matches `feedback_backfill_
  cleanup_issue.md` (the existing project rule that backfill / cleanup
  jobs need a tracking issue before they land).
