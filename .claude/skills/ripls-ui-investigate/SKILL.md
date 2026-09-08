---
name: ripls-ui-investigate
description: Autonomously reproduce and investigate a UI-observable bug in a hermetic web environment — boot the e2e stack standalone, seed a world, drive the real Flutter Web bundle with throwaway Playwright probe scripts, read the screenshots, and bisect the failure to a layer (client render / client state / server / data / UX gap). Use when the user types /ripls-ui-investigate <issue#>, asks to "reproduce issue #X in the UI", "investigate the UI bug", "click around and figure out why X happens", or when the ripls-issue skill hits an issue describing misbehavior a user saw on a screen. Takes an issue number or a freeform description of the behavior to investigate.
---

# UI Investigation Skill

Reproduce a reported UI misbehavior against a hermetic local stack, then run
a hypothesis-driven probe loop until the failure is pinned to a layer. The
deliverable is a **UI Investigation report** in `docs/issues/` (and, when
warranted, a distilled e2e spec) — not the pile of probe scripts, which are
throwaway.

This skill drives the real production Flutter Web bundle. The app is one
codebase across mobile and web, so most screen-level bugs reported from the
phone apps reproduce here. **Out of scope:** behavior that only exists in
platform-native layers (push notifications, camera/EXIF capture, APNs/FCM,
app lifecycle) — say so and stop rather than simulating something the web
can't exhibit.

## Input

An issue number (`/ripls-ui-investigate 2638`) or a freeform description.
With an issue number, `gh issue view` for body + comments, and **download any
screenshot URLs in the issue body** into the workspace (`curl -L -o …`) and
read them — the reporter's screenshot usually disambiguates what "it" is.

## The workspace

Everything one investigation produces lives in `e2e/investigations/<slug>/`
(gitignored; slug = issue number or a short kebab name):

```
e2e/investigations/2638/
├── env.json          # written by env:start; consumed by probes and env:stop
├── server.log        # server slog stream (grep by request-id prefix)
├── auth-emulator.log
├── issue-assets/     # downloaded reporter screenshots
├── seed.mts           # world-seeding script → world.json
├── world.json        # seeded ids + tokens, reused by every probe
├── probe-01-*.mts …   # one probe per question
├── shots/            # numbered screenshots (READ these)
└── logs/             # per-role console/RPC captures + aria snapshots
```

## Phase 0 — Preflight

1. **Understand the report.** Issue body, comments, screenshots. Then read
   the relevant code: locate the screen/widget (`app/lib/presentation/…`),
   its viewmodel, and the server RPCs it calls. Check `git log` for recent
   fixes in the same area — the bug may be version-skew (reporter's app
   version predates a fix). Load docs per `docs/llms.txt` for the touched
   area (`client/architecture.md` + the matching leaf docs).
2. **Write the hypothesis list.** Before any browser starts, enumerate
   H1…Hn — each a falsifiable statement naming the layer it blames and the
   observation that would confirm/refute it. The probe loop exists to kill
   these one by one; without the list it degenerates into clicking around.
3. **Build the stack** (skip pieces that are demonstrably fresh):
   ```bash
   npm run db:start                      # idempotent Postgres provision
   npm run generate                      # only if server/gen or e2e/gen missing
   npm run build:e2e                     # bundle + tmp/server, in the right order
   cd e2e && npm install && npm run install:browsers   # first time only
   ```
   `build:e2e` = `build:web:e2e` + `go build -o tmp/server ./server` — the
   bundle is embedded into the Go binary at build time, so the order is
   load-bearing. The env boot (`e2e/lib/server.ts`) refuses a `tmp/server`
   older than the bundle, so a stale binary fails fast instead of serving
   an old app.

## Phase 1 — Environment up

```bash
cd e2e && npm run env:start -- --slug <slug>    # default port 8100
```

Boots Auth Emulator (reuses a live one) + ephemeral Postgres DB + detached
server, and writes `env.json`. Idempotent per slug; `env:status` checks it.
Probes find the URL via `env.json` — never hardcode. **Never point a probe
at dev or prod.**

## Phase 2 — Seed the world

Write `seed.mts` using `e2e/lib/seed/*` (registerUser, seedSharedGear,
seedRequest, createCommunity, registerUserViaInvite, …) to build the minimal
preconditions the report implies — typically an owner, another member, and
the entity on screen. Print/persist ids + tokens to `world.json` so every
probe reuses the same world:

```ts
// investigations/2638/seed.mts  (run: npx tsx investigations/2638/seed.mts)
import { writeFileSync } from 'node:fs';
import { readEnvState } from '../../lib/investigation-state.js';
import { registerUser } from '../../lib/seed/users.js';
import { seedSharedGear } from '../../lib/seed/gear.js';

const baseUrl = readEnvState('2638')!.baseUrl;
const owner = await registerUser({ baseUrl, specSlug: '2638-seed', name: 'Olive Owner' });
const gear = await seedSharedGear({ baseUrl, specSlug: '2638-seed', accessToken: owner.accessToken });
writeFileSync('investigations/2638/world.json', JSON.stringify({ owner, gear }, null, 2));
```

Seeding via RPC is correct here (same rule as specs' preconditions).

## Phase 3 — The probe loop

One probe answers **one question**, named for it:
`probe-01-does-double-tap-create-two-transfers.mts`. Run with
`npx tsx investigations/<slug>/probe-01-….mts` from `e2e/`.

```ts
import { openProbe } from '../../lib/probe.js';
import { createTestClient } from '../../lib/connect.js';
// world.json → users/entities; openProbe injects auth pre-navigation.

const probe = await openProbe({ slug: '2638', role: 'borrower', user: world.borrower });
await probe.page.goto(`${probe.baseUrl}/gear/${world.gear.gearId}`);
await probe.snap('gear-screen-initial');          // → shots/borrower-01-….png
await probe.page.getByRole('button', { name: /borrow/i }).tap();
await probe.snap('after-first-tap');
console.log(await probe.aria('after-first-tap')); // text view of CanvasKit UI
await probe.close();
```

**Observation channels, in the order to reach for them:**

1. `probe.snap(name)` — screenshots at 2× DPR. **Actually read the PNGs**
   (Read tool); that is the ground truth for "what did the user see".
2. `probe.aria(name)` — ARIA snapshot (YAML): the greppable text of what
   CanvasKit rendered. CanvasKit paints to canvas, so **DOM text assertions
   don't work** — role/label locators and aria snapshots are the text
   channel (details: `e2e/README.md` § What's reliably reachable).
3. Direct RPC reads via `createTestClient` — server-state truth (counts,
   states, lists).
4. `psql "$(jq -r .dbUrl env.json)" -c '…'` — raw rows when RPC responses
   already aggregate away what you need.
5. `logs/<role>-console.log` — page errors, failed requests, non-2xx RPCs.
6. `server.log` grepped by `e2e-<slug>-<role>` request-id prefix — pair a
   browser gesture with the exact server lines it produced.

**Bisect between layers.** Probes are investigation tools, not specs — the
"state changes only through the UI" rule binds `e2e/tests/`, **not**
investigations. Mixing gestures and RPCs deliberately is the method:

- *Client vs server:* perform the action by UI gesture, then again by
  calling the same RPC directly; compare resulting server state. Server
  misbehaves on direct RPC → server bug. Server behaves but the UI produced
  different calls (check `server.log` for what the browser actually sent) →
  client bug.
- *State vs render:* RPC read says X while the screenshot shows Y → client
  render/state bug; both wrong → server/data.
- *Version skew:* reproducible on HEAD at all? If not, diff reporter's app
  version against the fix history from Phase 0.

**Iterate.** After each probe: which hypotheses died, which survived, what's
the next single question? Add hypotheses when evidence surprises you. Two
users interacting = two `openProbe` calls with different `role`s. If a probe
can't answer its question after ~3 attempts (locator won't resolve, flow
unreachable), switch observation channel or route (deep-link URL instead of
tap navigation) rather than retrying harder.

## Phase 4 — Verdict and report

Classify the root cause: **client-render / client-state / server-logic /
data / UX-affordance-gap / not-reproducible** (several can co-occur; say
which is primary). Then write the report.

**Report location:** `docs/issues/<number>-<slug>.md`. If a plan doc for the
issue exists, append a `## UI Investigation` section; otherwise create the
file with just header + that section (a later `/ripls-issue` run fills in
the plan around it).

```markdown
## UI Investigation

**Investigated:** <date> on <branch>@<short-sha> | **Environment:** hermetic e2e stack (`ripls-ui-investigate`)
**Verdict:** <root-cause layer(s), one sentence>

### Reproduction
<the minimal world + gesture sequence that shows the bug; note if NOT reproducible on HEAD and why>

### Hypotheses tested
| # | Hypothesis | Probe | Observation | Verdict |
|---|---|---|---|---|
| H1 | … | probe-01 | … | refuted/confirmed |

### Evidence
<the 2-4 observations that carry the verdict: screenshot descriptions, RPC/DB
state, server-log lines. Reference workspace paths for provenance but make the
text self-sufficient — the workspace is gitignored and ephemeral.>

### Fix direction
<concrete, layer-appropriate suggestion(s); feed into the plan's Potential Solutions>
```

**Distill an e2e spec when it earns its place.** If the confirmed repro is a
UI failure class per `e2e/README.md` § Testing philosophy (cross-user
propagation, browser-only path, route landing, cross-context invariant),
convert the decisive probe into a proper spec under `e2e/tests/` —
UI-gestures-only in the body, seed/assert via RPC, workflow-doc header. Ship
it in the fix PR where it flips green; if the fix is deferred, land it as
`test.fixme(…)` with the issue reference. A repro that a ViewModel or server
integration test can catch belongs there instead — don't add browser cost.

## Phase 5 — Teardown

```bash
cd e2e && npm run env:stop -- --slug <slug>
```

Always stop the env when the investigation concludes (kills server, drops
the ephemeral DB, leaves logs/shots on disk). Keep it up only if the user
says so or the fix work continues immediately — then stop it when done. The
workspace is scratch; the durable outputs are the report and any spec.

## Guardrails

- **Hermetic only.** Probes take `baseUrl` from `env.json`. Never aim at
  any deployed environment.
- **Don't leak infrastructure.** `env:stop` before finishing; if a start
  failed halfway, `env:stop` also reaps stale pids/DBs from `env.json`.
- **Don't commit the workspace.** `e2e/investigations/` is gitignored;
  reports carry the evidence in prose.
- **Respect concurrent test runs.** The env reuses a live Auth Emulator and
  binds port 8100+ precisely so `npm test` can run alongside — don't kill
  ports 8080-8090/9099 to "clean up".
- **Timebox.** If after ~5 probes no hypothesis is confirmed, write up what
  was ruled out (that's a real result) and stop rather than wandering.

## When invoked from ripls-issue

Run Phases 0–5 during that skill's Phase 2 (codebase research). The
investigation's Verdict/Evidence feed the plan's Problem Statement and
Potential Solutions, and the report section lands in the same
`docs/issues/` doc the plan uses.
