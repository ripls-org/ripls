# End-to-end test harness (Flutter Web)

Playwright Test driving the production Flutter Web bundle. This is the
top-level guide for anyone touching the harness — what it covers, what
it doesn't, the conventions, and the Flutter Web quirks you'll hit if
you try to assert against the wrong thing.

Designed and tracked in
[`docs/issues/2162-web-e2e-harness.md`](../docs/issues/2162-web-e2e-harness.md).

## What this is

A small Playwright suite that runs the built Flutter Web bundle in a
real browser (mobile-Chromium and WebKit) against a hermetic Ripls
server, with auth pre-injected. It exists to catch the class of
regression no other test layer sees:

- Browser-only code paths through `CanvasKit` (the #2155 hero-video
  case).
- Multi-user UI propagation — user A's action surfacing in user B's
  open browser.
- Cross-context UI invariants — same server state rendering correctly
  across distinct community / role / permission scopes.
- Route landings, deep links, and bundle-bootstrap correctness.

It is **complementary** to the existing test layers, not a replacement:

| Layer | Source of truth for | Where |
|---|---|---|
| ViewModel tests | Business-logic correctness | `app/test/presentation/viewmodels/` |
| Server integration tests | State machines, notifications, impact math | `server/integration_tests/` |
| **E2E (this harness)** | UI integration boundary | `e2e/tests/` |

If a regression can be caught by a ViewModel or server integration
test, it should be — those are faster, more deterministic, and easier
to triage. E2E catches the residue.

## Testing philosophy

### State changes during the test body happen through the UI

**This is the hardest rule.** RPCs are used in two well-defined
windows:

1. **Preconditions (before the test body):** seed users,
   communities, gear, experiences, memberships — everything that
   has to exist before the workflow step the spec is actually
   testing. The seed primitives in `lib/seed/` exist for this.
2. **Postconditions (after the UI gestures complete):** read
   server state — counts, list contents, status fields — to
   verify the workflow landed correctly. The seed-side Connect-
   Node clients (`createTestClient`) exist for this.

In between those two windows — the actual test body — **every state
change must come from a UI gesture on the real Flutter Web bundle**:
taps, long-presses, form fills, URL navigations a user could
realistically perform. If you find yourself calling
`hostExpClient.someRpc(…)` to flip state mid-test, stop: the spec is
no longer proving anything the server integration tests don't
already cover, and the browser overhead is pure cost.

The shape Phase-1 anchor specs use the same rule: the share-link
handoff spec drives the URL gesture (`page.goto('/event/{id}?
rsvp=yes&code=…')`), not the underlying `RSVPToExperience` RPC.
That URL is what a real user clicks; the RPC is the server-side
implementation detail.

### Cherry-pick, not 1:1 port

Workflow examples in `docs/workflows/*.md` are the canonical scenarios,
and `server/integration_tests/` covers them 1:1 via direct RPC calls.
The e2e suite **does not** mirror that mapping. Each e2e companion
spec exists only when the UI catches a failure class the RPC test
cannot.

Concrete criteria for adding an e2e spec — full list and rationale in
[`docs/testing/update_system_tests.md` § E2E companion
specs](../docs/testing/update_system_tests.md#e2e-companion-specs):

- **Add when** the spec exercises cross-user UI propagation, a
  browser-only code path, a cross-context UI invariant, or a route /
  deep-link landing.
- **Don't add when** the underlying bug is a server state-machine
  transition, a notification-dispatch issue, impact-metric math, or
  a UI render that no realistic action triggers.

If you can't write a one-sentence "this UI catches what RPC can't"
justification in the spec's header, the spec probably shouldn't
exist. A spec that just re-asserts every workflow step through
`Tappable` taps is a Patrol-shape re-run — and the structural
reasons Patrol failed (#1887) still apply.

### Why this discipline matters

Patrol's failure modes — Android `pumpAndSettle` hangs, iOS injection-
layer fragility, cross-platform matrix flake, slow APK / emulator boot,
and a 1,400-line custom harness — all stemmed from trying to cover
everything through a single brittle substrate. The structural fix in
this harness:

- Single browser, many `BrowserContext`s (one tab per user) instead of
  N parallel emulators.
- Pre-built bundle + ~2 s browser start instead of full-cycle APK
  build + emulator boot.
- WebKit as a real engine target instead of a custom Dart-side
  injection hook.
- Auto-wait locators with explicit timeouts instead of
  `pumpAndTrySettle`.

The cheap shape stays cheap **only if we keep the suite selective**.
A 24-spec full port re-introduces Patrol's cost profile in a different
package.

## Layout

```
e2e/
├── package.json            # Playwright + Connect-Web deps (pinned)
├── tsconfig.json
├── playwright.config.ts    # Two projects: mobile-chromium + webkit
├── global-setup.ts         # Start the SHARED Auth Emulator (servers are per-worker)
├── global-teardown.ts      # Stop the emulator + collect videos
├── lib/                    # Reusable helpers
│   ├── fixtures.ts          # The `test` every spec imports — worker-scoped server + DB
│   ├── server.ts           # Start/stop server (shells to scripts/start_e2e_server.sh)
│   ├── auth.ts             # Inject auth state into a context (multi-client capable)
│   ├── connect.ts          # Typed Connect-Node clients + browser request-ID override
│   └── seed/               # One file per service: registerUser, createCommunity, …
├── tests/                  # *.spec.ts — phase-1 anchors live here
│   ├── workflows/          # Workflow-companion specs (Phase 2+)
│   └── walkthrough/          # Walkthrough specs (#2684) — paced videos, not gates
├── fixtures/               # Binary fixtures (mp4, png) used during seed
└── gen/                    # Generated TS Connect-Web clients (gitignored)
```

Where to add code:

| Need | Goes in |
|---|---|
| New scenario / assertion | `tests/` (anchor) or `tests/workflows/` (workflow-companion) |
| New RPC-seeded entity (user, gear, experience…) | `lib/seed/{entity}.ts`, one file per service |
| New browser helper (token injection, locator shorthand) | `lib/` |
| New required server flag | `scripts/start_e2e_server.sh`, not `lib/server.ts` |
| New walkthrough reel (paced journey video) | `tests/walkthroughs/` — see [`docs/walkthroughs.md`](../docs/walkthroughs.md) |

**Walkthrough reels are showcases, not correctness gates.** The `@walkthrough`
specs under `tests/walkthroughs/` render paced journey videos
(`e2e/scripts/run_walkthrough.sh <reel>`) and are exempt from the
UI-only-state-changes rule below — they deliberately drive off-camera cast
members via RPC *mid-test* so one on-camera actor can watch the world react.
They run only under the `walkthrough` project and never in CI. The
correctness versions of those journeys stay in `tests/workflows/`. Full
pipeline docs: [`docs/walkthroughs.md`](../docs/walkthroughs.md).

## Creating test users

Registration is passwordless since #2571 — an account is created by proving you
can receive mail at an address, not by choosing a password. `registerUser()`
handles that for you, so **nothing in a spec changes**:

```ts
const host = await registerUser({ baseUrl, specSlug: SLUG, name: 'Host' });
```

Under the hood it runs the real three-RPC loop: `RequestEmailCode` →
`VerifyEmailCode` → `EmailRegister` with the resulting proof token. The code
comes back on `RequestEmailCodeResponse.devCode`, which the **server only
populates in dev mode** — that field exists precisely so tests can complete the
flow without a mailbox. If it is ever empty, the server is not in dev mode, and
`registerUser` says so rather than failing obscurely.

`SeededUser` no longer carries a `password`, because there isn't one.

**Driving the flow through the UI** (rather than seeding it) is
`completeEmailRegister()` in `lib/ui/email-register.ts`, which walks the three
on-screen steps: address → 6-digit code → name. It reads the code the same way.
Use it when the registration *itself* is what you're testing; use `registerUser`
when you just need an account to exist.

**Seeding a legacy password account.** `registerUser({ password })` still exists
for one purpose: creating the pre-#2571 account shape, so the login screen's
"Use my password instead" fallback can be exercised
(`login-screen-email-code.spec.ts`). It goes away with the password path (#2864).
Do not reach for it otherwise — a password account is not what real users have
any more.

**By hand, playing with the app.** Against any dev-mode server (a local
`npm run start:server`, or the dev environment) the app **fills the code in for
you** and says so under the field — so registering a made-up address is: type it,
tap "Email me a code", tap "Continue". No mailbox, and no password to invent.
That autofill is driven by the server echoing the code back, which production
never does, so it cannot appear there.

The echo is *additive* — **the email is still sent**. The dev environment has
Mailgun configured, so pointing at it with an address you can actually read
exercises the real delivery path end to end. To test the message itself rather
than the flow, clear the prefilled field and type what arrived.

**Outside e2e**, the same dev-mode echo backs the Go integration helpers
(`proveEmailOwnership` in `server/integration_tests/test_helpers.go`) and the
simulation runner (`proveSimulatedEmail`). Go *unit* tests use a different seam —
`email.MockEmailService` captures the code in-process. And for **production**
(app-store review, manual QA), where the echo is off by construction, there is a
fixed-code allowlist: see `server/services/login/README.md`.

## The multi-client model

The Phase-2 question — "how do we drive multiple distinct users
simultaneously?" — has a one-line answer: distinct `BrowserContext`s
per user, all under the same `browser` fixture.

```ts
test('two-user scenario', async ({ browser }) => {
  // 1. Seed both users via RPC.
  const host  = await registerUser({ baseUrl, specSlug: SLUG, name: 'Host' });
  const guest = await registerUser({ baseUrl, specSlug: SLUG, name: 'Guest' });

  // 2. One BrowserContext per user. Auth pre-injected before
  //    navigation. recordVideo is set per-context — see
  //    "Per-context video recording" below.
  const hostCtx = await browser.newContext({
    recordVideo: { dir: `test-results/videos/${SLUG}/host` },
  });
  await injectAuth(hostCtx, { accessToken: host.accessToken, /* … */ });
  const hostPage = await hostCtx.newPage();
  await installRequestIdOverride(hostPage, `${SLUG}-host`);

  const guestCtx = await browser.newContext({
    recordVideo: { dir: `test-results/videos/${SLUG}/guest` },
  });
  await injectAuth(guestCtx, { accessToken: guest.accessToken, /* … */ });
  const guestPage = await guestCtx.newPage();
  await installRequestIdOverride(guestPage, `${SLUG}-guest`);

  // 3. Drive both pages in parallel.
  let failed = false;
  try {
    await Promise.all([
      hostPage.goto(`${baseUrl}/event/${experienceId}`),
      guestPage.goto(`${baseUrl}/event/${experienceId}?rsvp=yes&code=${code}`),
    ]);
    // 4. Postconditions: server-side via RPC; UI re-render via locator.
    /* … */
  } catch (err) {
    failed = true;
    throw err;
  } finally {
    const hostVideo = hostPage.video();
    const guestVideo = guestPage.video();
    await hostCtx.close();
    await guestCtx.close();
    if (!failed) {
      await hostVideo?.delete().catch(() => {});
      await guestVideo?.delete().catch(() => {});
    }
  }
});
```

Each `BrowserContext` has its own localStorage, cookies, and
network stack — they're isolated. The shared server sees them as
different users with different access tokens, so anything that flows
through the server (RSVPs, messages, transfer state changes)
propagates naturally.

Per-context request-ID prefixes (`installRequestIdOverride(page,
'${SLUG}-host')`) let the triager grep the server log by user when a
failure crosses both browsers.

### Per-context video recording

The project default `video: 'retain-on-failure'` in
`playwright.config.ts` applies to the **fixture** context only —
`browser.newContext()` ignores it. Multi-client specs that want
failure videos uploaded as CI artifacts must:

1. Pass `recordVideo: { dir: 'test-results/videos/<slug>/<role>' }`
   to each `browser.newContext()` call — one subdirectory per user
   so the artifact bundle has clear naming (e.g.
   `videos/experience-rsvp-multi-client/host/` and
   `.../participant/`). The CI `test_web_e2e.yaml` workflow uploads
   `e2e/test-results/` on every outcome, so anything under that
   path lands in the artifact bundle.
2. In a `try / catch / finally`, capture `page.video()` references
   **before** closing the context (closing finalizes the file), close
   the contexts, then call `video?.delete()` only when the test
   passed — keeps artifact bundles small while preserving failure
   evidence.

The skeleton above shows the exact pattern. Copy it; don't reinvent
the cleanup logic per spec.



**Deferred until 3+ multi-client scenarios exist.** Per the Phase-2
plan, no `lib/multi-client.ts` helper class yet. Handcraft each spec
until the third one — at that point, refactor what's actually shared.

## What's reliably reachable on Flutter Web

Flutter Web with CanvasKit doesn't render its widget tree as
conventional DOM. Visual text goes straight to a `<canvas>`. The
parallel accessibility tree (`<flt-semantics>` nodes) is only emitted
when `SemanticsBinding.instance.ensureSemantics()` runs at startup,
and even then the engine aggressively merges and prunes nodes that
look "uninteresting". This shapes what your spec can assert against —
get it wrong and you'll spend a lot of time chasing locators that
look like they should work.

### What works reliably

| Mechanism | When to use it |
|---|---|
| **Scaffold.body-level `Semantics(identifier: …, explicitChildNodes: true, container: true)`** | Top-of-route anchor — "the screen rendered". Pattern: `web-event-screen`. |
| **Playwright role / label locators** (`getByRole`, `getByLabel`) | "This interactive widget exists." Reads the browser's accessibility tree, which has more nodes than the DOM. Works for `Tappable` / `Toggle` / `IconAction` surfaces by their `semanticsLabel`. |
| **Server-side RPC via `createTestClient`** | State truth. The bundle's bound widget tree reads from `GetExperience`, `GetGear`, etc. — assert against the same RPC for the count / state / list contents. |
| **`toHaveScreenshot` golden diffs** | Visual correctness (rendered text, layout, colour). Brittle, so use only when 1–3 above don't cover the failure mode. Out of scope for Phase 2. |

### What does NOT work (and why)

- **`Tappable.semanticsIdentifier` alone.** The parameter exists on the
  primitive, but the engine merges its `Semantics(identifier: …)` into
  the descendant subtree and drops it from DOM serialization unless
  the wrapper has its own non-trivial role. `page.locator('[flt-
  semantics-identifier="…"]')` will not find it.
- **Deeply-nested `Semantics(identifier: …, explicitChildNodes: true,
  container: true)` wrappers.** The `web-event-screen` anchor works
  because it sits at `Scaffold.body` depth — the engine has nothing to
  merge it into. The same pattern further down the route subtree gets
  pruned for being "uninteresting" (no label, no button, no role).
  `liveRegion: true` does **not** save it.
- **DOM text content for CanvasKit-rendered Text widgets.**
  `toContainText('1 attendee')` cannot find a string that lives only
  on the canvas. The accessibility tree has the row's `aria-label`
  ("View attendees"), not the visible count.
- **`document.querySelectorAll('[aria-label]')` for general inspection.**
  Flutter Web exposes accessibility through the browser's a11y API,
  not always via attributes on visible DOM elements. Use Playwright's
  `getByRole` / `getByLabel` / `accessibility.snapshot()` instead of
  raw DOM queries.

### Assertion strategy — in preference order

1. **`web-event-screen`-shaped anchor** on the route's Scaffold.body.
   Anchors are the only `flt-semantics-identifier` form that reliably
   reaches DOM. Use for "the screen rendered".
2. **`getByRole` / `getByLabel`** on the accessibility tree. Use for
   "this interactive widget is present" — works for any `Tappable` /
   `Toggle` / `IconAction` by its localized `semanticsLabel`.
3. **Server RPC via `createTestClient`** for any state question —
   counts, list contents, current status. The UI is bound to these
   values; if the RPC says count=1, the bound widget tree reflects 1.
4. **`toHaveScreenshot`** only when 1–3 cannot cover the failure
   mode (e.g., the assertion is about visible text alignment or
   colour).

If you find yourself writing `toContainText` against CanvasKit text or
adding a third `Semantics(identifier: …)` wrapper deep in a route,
stop and pick a different mechanism above. The right call is almost
always "anchor + role lookup + RPC for state".

## Writing a new workflow spec

Workflow companion specs live under `e2e/tests/workflows/`. Each
spec's file header must include:

1. **Maps to**: `docs/workflows/{workflow}.md` §"Workflow Examples"
   → Example N. If the spec covers only a slice (1 guest instead of
   3), say so and explain why the slice is sufficient.
2. **What this spec proves**: 1–2 sentences naming the failure class.
3. **What this spec does NOT prove**: out-of-scope notes so future
   readers don't add a redundant second spec for the same workflow.
4. **Divergences from workflow.md**: any preconditions or steps that
   differ from the doc, with the rationale.

Skeleton:

```ts
// e2e/tests/workflows/{workflow}-{scenario}.spec.ts
//
// Maps to: docs/workflows/{workflow}.md → Example N
//
// What this spec proves:
//   - <UI-specific failure class>
//
// What this spec does NOT prove:
//   - <out-of-scope explicitly>
//
// Divergences from workflow.md Example N:
//   - <e.g., guest joins via share-link handoff rather than pre-invite>

import { test, expect } from '@playwright/test';
import { createTestClient, installRequestIdOverride } from '../../lib/connect.js';
import { injectAuth } from '../../lib/auth.js';
import { registerUser } from '../../lib/seed/users.js';
// …

const SPEC_SLUG = '{workflow}-{scenario}';

test('<one-line scenario description>', async ({ browser }) => {
  const baseUrl = requireBaseUrl();

  // ── PRECONDITIONS (RPC seed) ───────────────────────────────────
  // Anything the workflow doc lists under "Preconditions" — seeded
  // via lib/seed/*. Never via UI taps.

  // ── BROWSER CONTEXTS + AUTH INJECTION ──────────────────────────
  // One context per user. Inject auth before navigating.

  // ── STEPS ──────────────────────────────────────────────────────
  // Workflow doc's Steps. Use UI gestures only for steps that
  // exercise the UI layer for *this* spec's failure class. Use RPC
  // for everything else.

  // ── POSTCONDITIONS ─────────────────────────────────────────────
  // 1. Server state via RPC (count, status, list contents).
  // 2. UI render correctness via anchor + role locator.
});
```

Also: add the spec to the coverage table in
[`docs/testing/update_system_tests.md` § Current e2e companion
coverage](../docs/testing/update_system_tests.md#current-e2e-companion-coverage)
in the same PR.

## Running locally

> **You can run this locally — including from an AI coding agent.** A working
> `gcloud` login (CLI auth + ADC) is part of the standard local dev setup and is
> **generally always available on a Ripls dev machine**, so the secret fetches in
> `build:web:e2e` (Mapbox / Firebase / Maps) Just Work — don't assume otherwise
> and skip the run. If a fetch *does* fail, the fix is to re-auth
> (`gcloud auth login` + `gcloud auth application-default login`), not to give up
> on running e2e. Verify with `gcloud auth list` and
> `gcloud auth application-default print-access-token`.

Prerequisites:

- Node ≥ 20.
- `gcloud` auth (CLI + ADC) — standard local setup; needed by `build:web:e2e`
  to fetch Mapbox / Firebase / Maps secrets. See the note above.
- Postgres reachable on `localhost:5432`. **`npm run db:start` idempotently
  provisions the e2e DB** (`ripls_e2e_proof`) on the shared local container
  (`scripts/ensure_e2e_db.sh`) — run it once and forget it. CI doesn't use it:
  it spins a throwaway Postgres testcontainer per run that's auto-reaped (Ryuk),
  so the DB lifecycle there is fully automatic.
- Flutter Web bundle built into `server/services/web/app_assets/`
  (`npm run build:web:e2e` — the e2e variant adds the auth-emulator define;
  `build:web:local` is the non-emulator variant).
- Ripls server binary built at `tmp/server` (`go build -o tmp/server
  ./server`).

> **⚠️ Order matters: rebuild the server AFTER the web bundle.** The Flutter
> Web bundle is **embedded into the server binary** at Go build time
> (`//go:embed app_assets` in `server/services/web/app_embed.go`). So any
> **client (Dart) change is invisible until you rebuild `tmp/server`** — the
> server serves the bundle that was embedded when *it* was last built, not
> whatever is currently on disk in `app_assets/`. After a client change, one
> command does both steps in the right order:
>
> ```bash
> npm run build:e2e   # = build:web:e2e + go build -o tmp/server ./server
> ```
>
> Symptom of skipping the Go rebuild: the app behaves like an older build
> (missing your latest widget/logic changes) even though the on-disk
> `app_assets/` bundle is fresh. This trap is now also caught mechanically:
> `e2e/lib/server.ts` (every runner's boot path) and
> `scripts/run_walkthrough.sh` refuse to start a `tmp/server` that is older
> than the newest file in `app_assets/`.

```bash
cd e2e
npm install
npm run install:browsers       # one-time: ~300 MB of Chromium + WebKit

# The e2e DB is provisioned idempotently by `npm run db:start` and specs
# self-isolate by unique seed ids, so you do NOT need to reset it between runs.
# (To force a clean slate anyway: drop + recreate ripls_e2e_proof by hand, or
# `npm run db:reset` then `npm run db:start`.)

E2E_DB_URL='postgres://ripls:ripls_dev@localhost:5432/ripls_e2e_proof?sslmode=disable' \
E2E_PORT=8090 \
  npm test                     # both projects; or one spec:
                               #   npx playwright test tests/workflows/<spec>.ts --project=mobile-chromium
# or:
  npm run test:headed          # visible window
  npm run test:ui              # Playwright UI mode (best dev loop)
  npm run report               # open HTML report from last run
```

`E2E_BASE_URL` (when set) makes every worker reuse an already-running
server instead of starting its own — useful when iterating on spec code
without waiting for server startup each run.

## Standalone investigation environment

The same stack a worker gets — Auth Emulator + ephemeral DB + server — can be
booted **outside the test runner** and left up, for interactive or
agent-driven UI investigation (the `ripls-ui-investigate` skill in
`.claude/skills/`):

```bash
npm run env:start -- --slug 2638      # detached; state → investigations/2638/env.json
npm run env:status -- --slug 2638
npm run env:stop  -- --slug 2638      # kills server, drops the ephemeral DB
```

Against it you run throwaway **probe scripts** (`npx tsx
investigations/<slug>/probe-*.mts`) built on [`lib/probe.ts`](lib/probe.ts) —
auth-injected Chromium sessions with numbered screenshots, ARIA snapshots,
and console/RPC capture — plus the normal `lib/seed/*` primitives.
`investigations/` is gitignored scratch space; the durable output of an
investigation is its report in `docs/issues/`. Probes are **not** specs: they
may mix UI gestures and direct RPCs freely (that contrast is how you bisect
client-vs-server), and none of the testing-philosophy rules above apply to
them. The env binds port 8100 by default and reuses a live Auth Emulator, so
it coexists with a concurrent `npm test`.

**Parallelism.** Specs run across multiple Playwright workers (CI defaults
to 4; override with `E2E_WORKERS`; locally Playwright auto-picks). Each
worker gets its OWN isolated server (bound to `E2E_PORT + parallelIndex`)
and its OWN ephemeral database (created in the shared Postgres container),
wired up by the worker-scoped fixture in [`lib/fixtures.ts`](lib/fixtures.ts)
and torn down when the worker exits. The Auth Emulator is shared (one
process), so phone fixtures must stay unique per spec. This is why every
spec imports `test` from `../lib/fixtures.js` rather than `@playwright/test`.

## Composited-contrast gate

`design/tokens.json` gates contrast at generation time, but only for
token-on-token pairs against `background` and `surface` — two flat opaque
colours. Much of the app renders on translucent glass over arbitrary user
media, where the real background is a blurred composite no token math can
predict. #2764 is the consequence: deep sage `#3E5A47`, validated at 6.8:1 as
a button *fill*, used as a `TextButton` foreground on a dark glass sheet where
it measures **1.64:1**.

This gate measures what was actually painted:

```bash
npm run test:contrast          # full matrix (nightly)
npm run test:contrast:pr       # reduced set (per-PR)
```

Each surface is rendered twice — once from the real tokens, once from a bundle
built by `node scripts/gen_design_tokens.js --sentinel`, which replaces every
token with a distinct hue. Diffing the two passes attributes each painted pixel
to the token that painted it. That indirection is necessary because CanvasKit
paints text to canvas and prunes non-interactive nodes from the accessibility
tree (see *What's reliably reachable* above) — there is no DOM to read colours
or geometry from, so bounding-box approaches miss body copy and status text
entirely.

| File | Role |
| --- | --- |
| [`scripts/contrast_surfaces.mjs`](scripts/contrast_surfaces.mjs) | The evaluation set — surfaces, themes, backdrops. Declarative; adding a surface is a data change. |
| [`scripts/contrast_capture.mts`](scripts/contrast_capture.mts) | One capture pass. Seeds deterministically, since the two passes run against separate environments. |
| [`scripts/contrast_analyzer.mjs`](scripts/contrast_analyzer.mjs) | Pure measurement: classification, connected components, ring background, WCAG ratios. Unit-tested. |
| [`scripts/analyze_contrast.mjs`](scripts/analyze_contrast.mjs) | Pairs the passes, reports, exits non-zero on failure. |
| [`scripts/run_contrast_gate.sh`](scripts/run_contrast_gate.sh) | Orchestrates both builds and restores real tokens on any exit. |

**This is not the only contrast layer, and it is not the cheap one.**
[`app/test/helpers/contrast_helpers.dart`](../app/test/helpers/contrast_helpers.dart)
pairs each `Text`/`Icon` in a pumped tree with its nearest **opaque** ancestor
fill and asserts the WCAG threshold that element actually gets — 4.5:1 for body
text, 3:1 for large text and icons. It runs on every `flutter test`, so it is
the layer that gates `main` today; this gate is not yet wired into CI (#2770
Phase 4).

The split is forced by the material, not by convenience. Widget tests see
*declared* colours, so they can only judge an element that paints its own opaque
fill — which is exactly #2798, where a control painted `#F2F2EE` and then drew
the on-glass white ramp on it at 1.12:1. They cannot judge a translucent fill,
because what it composites against depends on the backdrop. Only this gate can.
Neither layer subsumes the other; a surface that fails here will usually pass
there, and vice versa.

Two details are load-bearing and easy to break:

- **Components are measured separately.** A token is routinely used against more
  than one background — `modalTextPrimary` labels both the bare sheet and the
  inside of a filled pill. Measured as one mask, the median collapses to
  whichever context has more pixels and the other is silently averaged away.
- **The background is a ring that excludes only the token's own pixels.**
  Pixels belonging to a *different* token are kept deliberately: when text sits
  on a filled pill, that fill *is* the background. Sampling the whole bounding
  box instead reports the sheet behind the pill (measured 2.97 against a true
  2.41).

Token values are compile-time constants, so each pass needs its own bundle
(~2.6 min; `flutter build web --wasm` has no meaningful incremental reuse for a
constant change). The sentinel pass rewrites a checked-in generated file, so the
orchestrator restores it via an `EXIT` trap — an interrupted run must never
leave sentinel hues staged. Background plan: `docs/issues/2770-token-contrast-evaluation.md`.

## Anti-patterns

Concrete things to avoid:

1. **Don't manipulate state via RPC during the test body.** RPCs
   are for preconditions (seed) and postconditions (assert) only;
   workflow steps fire through real UI gestures. See § "State
   changes during the test body happen through the UI" above. If
   the UI lacks the affordance, either (a) add semantics so the
   affordance is findable, or (b) cover it in a server integration
   test instead — don't fake a tap with an RPC.
2. **Don't port every workflow example.** The server tests are the
   1:1 mapping; e2e is selective. See § Testing philosophy above.
2. **Don't pre-tag widgets with `Semantics(identifier: …)`
   speculatively.** Identifiers and the spec that queries them land
   in the same PR. Unused identifiers rot.
3. **Don't assert on CanvasKit-rendered text via DOM queries.** The
   text isn't in the DOM. Use role locators + RPC for the state
   truth.
4. **Don't add nested `Semantics(…)` wrappers expecting them to
   serialize.** Anchor + role lookup is the supported path.
5. **Don't share state between specs.** Each spec seeds its own
   world. The global-setup is per-run, not per-spec.
6. **Don't rebuild seed state via UI taps.** RPC seeding is the
   canonical path for everything before the spec's actual
   assertion — including users, communities, gear, experiences.
7. **Don't write a spec without a workflow-doc reference in the
   header.** If you can't name the workflow example and the
   UI-specific failure class, the spec probably belongs in
   `server/integration_tests/` instead.
8. **Don't skip the per-spec request-ID prefix.** When a multi-
   client spec fails, the only way to disambiguate user A's server
   logs from user B's is the `e2e-{slug}-{role}` prefix.

## Dependency upgrade policy

Pin Playwright, Connect-Web, and `@bufbuild/protobuf` to exact
versions in `package.json` (no `^` / `~`). Major-version upgrades
land as their own PR — never bundled with a feature change.
Playwright's behaviour around auto-waits, locators, and the WebKit
bundle changes subtly between minors; a flaky spec post-upgrade
should have a single-commit revert path.

## See also

- [`docs/issues/2162-web-e2e-harness.md`](../docs/issues/2162-web-e2e-harness.md) — full plan and design rationale.
- [`docs/testing/update_system_tests.md`](../docs/testing/update_system_tests.md) — workflow-doc ↔ test mapping, including the e2e cherry-pick rules.
- [`docs/client/testing.md`](../docs/client/testing.md) — broader client testing strategy.
- [`docs/client/testing/semantics_identifiers.md`](../docs/client/testing/semantics_identifiers.md) — the identifier convention.
- [`scripts/start_e2e_server.sh`](../scripts/start_e2e_server.sh) — server-startup helper invoked by `lib/server.ts`.
