# WEB-2 milestone: full phone-first pivot loop + Playwright e2e (Auth Emulator)

**Companion to** [`2492-phone-first-pivot.md`](./2492-phone-first-pivot.md). This is the
build/verify plan for the **milestone that proves the pivot**: a Playwright e2e that drives the
entire loop through the real web UI — *invite link → web event view → enter phone → OTP → verified →
RSVP auto-fires → becomes a real member*. Bundles WEB-1, the deferred LINK-1 client carrier, WEB-2
wiring, the Auth-Emulator e2e foundation, new `docs/workflows/` examples, and the e2e spec onto one
branch (`web2-pivot-loop-e2e`, stacked on `link1-adhoc-open-links`/#2517). **Decisions locked
(Bryan):** Firebase **Auth Emulator** for deterministic OTP; **hold** the PR until the full-loop e2e
is green (no partial-e2e landing).

## Why the Auth Emulator (not test numbers)

Both the Flutter web **client** and the Go **server** must agree on the Firebase token. The e2e
server runs `--dev-mode` with **no `--firebase-project`**, so `phoneAuth` is nil and
`PhoneRegister`/`PhoneLogin` return `Unimplemented` today. Test phone numbers make only the *client*
deterministic; the server would then need real `ripls-dev` Admin credentials in CI (a secret +
network flakiness, breaking the credential-free hermetic harness). The Auth Emulator is hermetic:
client (`useAuthEmulator`) and server (`FIREBASE_AUTH_EMULATOR_HOST`) both point at a local emulator,
no secrets, no reCAPTCHA, real Firebase SDK code paths, deterministic test codes.

## Phased build plan

- [x] **WEB-1 — client hardening** (`web2-pivot-loop-e2e` HEAD). `kIsWeb`-guard the iOS-only
  `getAPNSToken()` in `phone_auth_screen.dart`; make the Firebase-callback redirect in
  `app_router.dart` mobile-only. `flutter analyze` clean.
- [x] **EMU — Auth Emulator e2e foundation** (server + harness + client). *Done; full-loop run pends
  the web bundle + spec.*
  - [x] `firebase.json` with the `auth` emulator on `127.0.0.1:9099` + demo-project mode.
  - [x] `scripts/start_e2e_server.sh`: passes `--firebase-project=demo-ripls`; when the harness
    exports `FIREBASE_AUTH_EMULATOR_HOST`, the Admin SDK validates emulator tokens. **`NewApp`
    works in emulator mode with NO ADC — confirmed, no shim needed.** (`initializeFirebaseApp` is
    unconditional and already wires `PhoneAuth`, main.go:1157 — the prior gap was token validation.)
  - [x] **Verified:** `server/auth/firebase_emulator_test.go` mints a phone token via the emulator
    REST API and validates it through `NewApp→NewFirebaseAuth→VerifyPhoneToken` — green under
    `firebase emulators:exec --only auth --project demo-ripls`; skips otherwise.
  - [x] `e2e/lib/emulator.ts` + `global-setup`/`global-teardown`: boot the emulator (pinned
    `firebase-tools@15` via `node_modules/.bin/firebase emulators:start --only auth`) before the
    server, export `FIREBASE_AUTH_EMULATOR_HOST`, reap the detached process group on teardown.
    Readiness probe (root → 200 `{ready:true}`) verified.
  - [x] Client: `Environment.authEmulatorHost` (`AUTH_EMULATOR_HOST` dart-define) +
    `FirebaseAuth.instance.useAuthEmulator(host, port)` in `main.dart` (kIsWeb-guarded, no-op in real
    builds). `flutter analyze` clean. **Remaining:** the e2e web build must pass
    `--dart-define=AUTH_EMULATOR_HOST=127.0.0.1:9099` (+ `ENVIRONMENT=local`) — wire into the e2e
    bundle build.
- [x] **LINK-1 client carrier + WEB-2 loop wiring** — **no new wiring needed.** Recon confirmed the
  web event-RSVP handoff (`WebExperienceScreen` + `WebEventRsvpHandoffNotifier`) is
  **auth-method-agnostic**: it captures the pre-verify RSVP intention and auto-fires it after auth +
  community join, identically for phone registration. `post_registration_destination` already threads
  `experienceId`/`rsvp`/`code` for phone. So WEB-1 + ID-2 + LINK-1-server + the emulator close the
  gaps; the only client add was a route-level `phone-auth-screen` Semantics anchor for the spec.
- [x] **Workflow doc** — `docs/workflows/phone_first_rsvp.md` (indexed in llms.txt).
- [x] **Full-loop Playwright spec — GREEN** (`e2e/tests/workflows/phone-rsvp-full-loop.spec.ts`,
  ~8s): seeds host+community+event+share link + a phone-keyed provisional user; lands on `/go/{code}`;
  clicks the SSR "I'm in"; registers via **real phone OTP through the emulator** (reads the
  deterministic code from the emulator's `verificationCodes` endpoint); asserts the RSVP auto-fired
  (`rsvpYesCount > 0`). Two non-obvious lessons baked in: **(1)** the Flutter bundle is `//go:embed`'d,
  so `tmp/server` MUST be rebuilt after `build:web:e2e` (else stale); **(2)** Flutter-web text fields
  ignore `.fill()` — use `click` + `pressSequentially`. Buttons resolve via `getByRole` (a11y tree).
  reCAPTCHA is skipped via `appVerificationDisabledForTesting` set on the emulator path.

### Run recipe (local, on this box)

```bash
npm run db:start                              # Postgres; idempotently provisions the ripls_e2e_proof DB (scripts/ensure_e2e_db.sh)
npm run build:web:e2e                         # web bundle w/ AUTH_EMULATOR_HOST define (~5 min; needs gcloud)
go build -o tmp/server ./server
cd e2e && npm install                         # firebase-tools@15 + browsers (chromium installed)
E2E_DB_URL='postgres://ripls:ripls_dev@localhost:5432/ripls_e2e_proof?sslmode=disable' E2E_PORT=8090 \
  npx playwright test tests/workflows/phone-rsvp-full-loop.spec.ts --project=mobile-chromium
```

The harness `global-setup` boots the Auth Emulator + server automatically. webkit isn't installed
locally — use `--project=mobile-chromium`.

### Existing specs + the emulator (Bryan asked)

**Decision: keep token injection** for the existing specs (`guest-rsvp-confirm`, `ported-smoke`,
`video-on-event-hero`, `experience-rsvp-multi-client`). Their subject isn't auth, and their injected
user is already a real registered user (via `EmailRegister` RPC) — injection only skips the login
*UI*. Driving real emulator auth UI there adds runtime + flakiness without improving their
assertions. The emulator's realism payoff is where auth *is* the unit under test (the phone-first
spec). Revisit only if a specific spec's value depends on the real auth path.

**The emulator must be wired LAZILY, not at startup.** The first CI run of the full suite timed out
`experience-rsvp-multi-client` (the heaviest spec: two browser contexts + video + a real-time
`StreamCommunityEvents` propagation assertion) on *both* Chromium and WebKit, while the phone-first
spec itself passed. Root cause: the e2e bundle's `AUTH_EMULATOR_HOST` define originally called
`FirebaseAuth.useAuthEmulator` at **app startup** (`main.dart`), and `useAuthEmulator` does a network
round-trip to the emulator. Paying that on *every* web page load slowed every spec; the 2-context
multi-client spec tipped over its 60s budget. Proof it was the bundle and not the runner: a no-commit
re-run of `main`'s Web E2E on the *same* runner the same day was green (`12 passed, 58s`) — `main`
doesn't carry the define. **Fix:** the app wires the emulator lazily on the phone-register screen
(`phone_auth_screen.dart`, right before `verifyPhoneNumber`, guarded to run once), so the define is
now *genuinely* inert for every non-phone-auth flow — nothing touches `FirebaseAuth` before that
screen. `main.dart` no longer references Firebase Auth at all.

## Prerequisites / blockers (gate the *green* e2e run)

1. **Runnable `firebase-tools`.** ✅ **RESOLVED.** The *installed* firebase-tools (13.29.2) crashes
   under node 24/25 (`buffer-equal-constant-time` reads `SlowBuffer.prototype`, removed in node 24+).
   **firebase-tools `15.22.0` supports node 24/25 and is verified working under this box's node 25** —
   the Auth Emulator boots and its identitytoolkit REST API serves (`emulators:exec --only auth
   --project demo-ripls`). The harness invokes the emulator via a pinned `firebase-tools@15` (npx or a
   devDependency); CI's runner (node 24) gets the same pin. `firebase.json` (root) configures the auth
   emulator on port 9099. Node ≤22 (e.g. the box's `node@22`) also works with the old CLI if needed.
2. **Web build secrets.** `npm run build:web:local` needs `gcloud` auth for Mapbox/Firebase/Maps
   secrets (per `e2e/README.md`). Required to produce the bundle the Playwright run serves.
3. **CI runner tooling.** The self-hosted runner image must gain the **Firebase CLI + Java** (the
   emulator is a Java process) to run this e2e in CI — a `versions.env` + runner-rebuild change.

## Status — ✅ GREEN (2026-06-19)

The full phone-first loop passes end-to-end on this box, **reliably back-to-back** (~8s):
SSR `/go/{code}` → "I'm in" → Flutter web → "Sign in to RSVP" → "Register with Phone" → **real phone
OTP via the Auth Emulator** → `PhoneRegister` (promotes the seeded provisional user) → back to the
event → auto-RSVP → `rsvpYesCount > 0`. All on `web2-pivot-loop-e2e`.

**The two traps that cost the most (now codified):**
1. **`//go:embed` staleness** — the Flutter bundle is embedded in `tmp/server` at Go build time, so a
   client change is invisible until `tmp/server` is rebuilt *after* `build:web:e2e`. Documented in
   `e2e/README.md`.
2. **Flutter-web text input** — `.fill()` sets the a11y-proxy value but doesn't reach Flutter's
   `TextEditingController`; use `click` + `pressSequentially`. Buttons are fine via `getByRole`.

**Selectors that worked:** SSR CTA `getByRole('link', {name: /rsvp yes/i})` (the aria-label, not the
visible text); `getByRole('button', {name})` for "Sign in to RSVP", "Register with Phone", "Send
Code", "Verify", "Create Account". reCAPTCHA skipped via `appVerificationDisabledForTesting` on the
emulator path (set in `main.dart` + right before `verifyPhoneNumber`).

**Exit code — FIXED.** Earlier the runner exited 143/137: `global-setup` derived the server pid via
`lsof -ti:PORT`, but the runner's own `/health` connections to that port made lsof return the
*runner's* pid, so `global-teardown` SIGTERMed itself (and orphaned the emulator). Now `lib/server.ts`
exposes the spawned server's real pid (the script `exec`s the binary, so `child.pid` IS the server)
and `global-setup` persists that — no lsof. Runs exit **0** with clean teardown + no orphans.
`global-setup` also pre-reaps 9099/4400/E2E_PORT defensively.

**CI — WIRED** (`.github/workflows/test_web_e2e.yaml`): set up Java 17 (the emulator is a Java
process); build with **`build:web:e2e`** (adds `AUTH_EMULATOR_HOST`; inert for other specs); pre-fetch
the emulator JAR; rebuild `tmp/server` after the bundle (the `//go:embed` ordering). The harness boots
the emulator. The phone spec is **chromium-only** (`test.skip` on webkit). The full e2e (incl. the
phone-first loop) runs on PRs targeting `main` — i.e. the integration PR #2520.

**Prereqs (handled):** firebase-tools@15 pinned (node 25/24 + CI node 24); `ripls_e2e_proof` DB
(auto-provisioned by `npm run db:start` via `scripts/ensure_e2e_db.sh` locally; a throwaway
testcontainer via `server/cmd/web-smoke-db` in CI); Playwright chromium; gcloud /
GCP_SERVICE_ACCOUNT_KEY_DEV for the web build secrets.
