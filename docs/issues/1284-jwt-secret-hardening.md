# Issue #1284: [security] JWT secret is hardcoded; any code-reader can forge tokens

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1284
**Filed:** 2026-04-25 | **Last activity:** 2026-04-27
**Labels:** auto-filed, review:security | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The server's JWT signing secret is a literal string baked into `server/main.go`:

```go
authTokenConfig := &auth.TokenConfig{
    Secret:     []byte("dev-secret-key"), // TODO: Use environment variable in production
    Expiration: 1 * time.Hour,
}
```

Every binary built from `HEAD` — including production deployments — signs and
verifies access tokens with the same value. Anyone who can read this repo (or
who has compromised the supply chain) can mint a valid HS256 access token for
any `user_id` and any `role`, bypassing Firebase phone verification, OIDC,
bcrypt password hashing, and the refresh-token store entirely. The forged
token is indistinguishable from a legitimate one because the signing key
matches what the server uses to verify.

The issue body cites `server/main.go:415`; the actual line is `server/main.go:525`
(the file shifted since filing). The vulnerable site is the same.

Note: refresh tokens are not affected — those are random 32-byte values hashed
with SHA-256 and stored in the database (`server/auth/tokens.go:22`). The
attack surface is access tokens only, but those carry `role` claims that gate
admin paths, so the impact is full account takeover.

## Context from Comments

- 2026-04-25 (maintainer): duplicate #1286 was closed in favor of this issue.
- 2026-04-25 (triage): set Type=Bug, Priority=P1. Rationale: critical security
  vulnerability on engineering grounds; 142 days open. P1 assigned even
  without external user impact data.
- 2026-04-27 (maintainer): pinged `@claude` — the request to plan/fix
  this work.

No technical disagreement in the thread. The fix direction (env var, refuse
non-dev startup if unset/short, rotate everywhere it has shipped) is already
endorsed in the issue body.

## Resolved Design

The shape of the fix is decided (confirmed 2026-04-29):

- **Flag only — no environment variable fallback.** The signing secret is
  passed via `--jwt-signing-secret`. We do *not* read `JWT_SIGNING_SECRET`
  from the environment. This matches the convention used by every other
  secret-bearing flag in `package.json`'s `start:server` line
  (`--openai-api-key`, `--mailgun-api-key`, `--mapbox-access-token`, etc.):
  one source of truth (the flag), wired in by whoever launches the binary.
- **Dev GCP reads from Secret Manager, same as prod.** The dev Cloud Run
  service mounts a per-environment GCP secret (`jwt-signing-secret-dev`)
  and passes it through as a flag value at container start; prod uses
  `jwt-signing-secret-prod`. Both environments are equally hardened.
- **Local development uses a hardcoded value in `package.json`.** The
  `scripts.start:server` line in `package.json` already carries other
  long-lived dev secrets as literal flag values; we add
  `--jwt-signing-secret=<32+ byte literal>` alongside them. Contributors
  inherit it automatically with `npm run start:server`.
- **The startup check is unconditional, not gated on `--dev-mode`.** Any
  binary, in any mode, refuses to start if `--jwt-signing-secret` is
  unset or shorter than 32 bytes. There is no ephemeral-random fallback
  and no dev-mode lenience — the "you must have a real secret" rule is
  the same everywhere. (No separate guard against the leaked literal
  `"dev-secret-key"` is needed: it is 14 bytes, so the length check
  rejects it.)

### Alternatives considered and rejected
- **Env var fallback.** Rejected — splits the source of the value across
  two places and complicates audit. The flag-only convention is project
  policy.
- **Dev-mode ephemeral random secret.** Rejected — the local dev value in
  `package.json` does the same job (frictionless `npm run start:server`)
  while keeping behavior consistent with dev/prod GCP.
- **Asymmetric signing (RS256/EdDSA) with KMS.** Out of scope for this
  issue — large blast radius, not needed for the current single-binary
  architecture. Reconsider if we ever split signing and verification
  across services.

## Open Questions

- **Should we rotate the existing prod secret as part of this change, or
  document rotation as a separate operational step?** — Recommendation:
  rotate as part of merging. Anyone who pulled the repo while
  `dev-secret-key` was live can forge tokens against any environment that
  has ever booted with that literal. Rotation happens at deploy time (new
  secret in Secret Manager → restart Cloud Run revision) and invalidates
  all outstanding access tokens. Refresh tokens survive (different code
  path), so users will not be logged out — they'll just need to refresh
  once.
- **Minimum secret length: 32 bytes?** — Recommendation: yes, 32 bytes.
  HS256 keys should be at least as long as the hash output (SHA-256 → 32
  bytes); shorter keys weaken the construction. Enforce at startup
  (`len(secret) >= 32`). Document that operators should generate via
  `openssl rand -base64 32`.
- **Where do we put the Secret Manager entries?** — Recommendation:
  alongside the others in `terraform/environments/{dev,prod}/main.tf`,
  named `jwt-signing-secret-dev` and `jwt-signing-secret-prod` to match
  the existing pattern (`gear-db-password`, `mailgun-api-key`, etc.).
  Use a `random_password` resource + `google_secret_manager_secret_version`
  so distinct values are provisioned per environment without committing
  them.
- **What value goes in `package.json`?** — Recommendation: a 32+ byte
  base64 literal generated once via `openssl rand -base64 32`, pasted
  verbatim into the `--jwt-signing-secret=` flag in `scripts.start:server`.
  Treat it the same as the other long-lived dev keys already in that
  line — not sensitive (only valid against a localhost server), but
  long enough to satisfy the startup gate.
- **How do we handle CI / e2e tests that boot the server binary?** —
  Recommendation: pass `--jwt-signing-secret=<dev literal>` (or reuse the
  same literal that lives in `package.json`) in the workflow / e2e
  harness command. Server unit tests construct `auth.TokenConfig`
  directly with `test-secret-key` and never go through `main()`, so they
  are unaffected. Audit `.github/workflows/` and `npm run test:server:e2e*`
  for callers that boot the binary.

## Architecture Audit (2026-04-29)

The plan was reviewed against `docs/server/architecture.md` and
`docs/server/observability.md`. Findings, all folded into the phases
below:

- **Validation helper belongs in `server/auth`, not `server/main.go`.**
  The architecture doc's "Shared Functionality in Libraries" principle
  calls for putting reusable logic next to the abstraction it serves.
  `auth.TokenConfig` consumes the secret, so add
  `auth.ValidateSigningSecret([]byte) error` (or similar) to
  `server/auth/tokens.go` and call it from `main()`. Tests then live in
  `server/auth/tokens_test.go` alongside the existing
  `TestTokenConfig_*` tests, not in `server/main_test.go`.
- **Update `server/auth/README.md`.** Per project policy ("every
  multi-file directory under `server/` must have a README"), and the
  existing `server/auth/README.md` already enumerates `tokens.go`'s key
  exports. Add a one-line entry for the new validation helper so future
  contributors find it.
- **Closest precedent for the startup gate is the
  `--invite-link-hostname` check at `server/main.go:462-465`**, not the
  CORS check. Both are "required flag, fail-closed at startup, no
  dev-mode lenience" — exactly the shape we want. Match the error
  format (`"error: --jwt-signing-secret is required ..."` to stderr,
  `os.Exit(1)`).
- **Use `fmt.Fprintln(os.Stderr, ...)` + `os.Exit(1)`, not
  structured logging, for the gate failure.** `docs/server/observability.md`
  prescribes structured `slog` for runtime logs, but at the point where
  this check runs in `main()`, the structured logger has not yet been
  initialized (`logging.NewLogger(...)` is called after the existing
  early-flag checks). Stay consistent with the existing precedents in
  `main.go`.
- **Never log the secret value, even masked.** Add a single
  `logger.Info("JWT signing secret configured")` (no length, no prefix,
  no `MaskToken` of the secret itself) *after* the structured logger is
  initialized later in `main()`, or skip the log entirely. The existing
  `MaskToken` calls in `tokens.go` mask user-issued JWTs, not the
  signing key — different concern, don't conflate them.
- **Drop the `dev-secret-key` regression guard in Phase 3.** The
  unconditional 32-byte minimum rejects the literal `dev-secret-key`
  (14 bytes) automatically. The "short secret rejected" unit test
  subsumes a separate string check; an extra grep/CI gate is dead
  weight.
- **Server-binary launchers beyond `package.json`.** `scripts/profile.sh`
  and `scripts/sql_profile.sh` also `go build -o tmp/server` and run
  the binary; both must be updated with `--jwt-signing-secret=<dev
  literal>`. (The same dev literal that lives in `package.json` is
  fine.) Phase 1 calls these out explicitly.
- **Client docs (architecture / caching / i18n / a11y / testing) do
  not apply.** This is a server-only, build-time / startup-time change.
  No proto schema changes, no API surface, no client code path. The
  client will continue to receive valid JWTs; only the signing key on
  the server side changes.

## Implementation Plan

### Phase 1: Server-side gate and local dev wiring
**Goal:** Remove the hardcoded literal, require a flag-supplied 32+ byte
secret in every mode, and keep `npm run start:server` working.
**Tasks:**
- [ ] Add `ValidateSigningSecret(secret []byte) error` to
  `server/auth/tokens.go` (next to the existing `TokenConfig` type).
  Returns a descriptive error if `len(secret) < 32`. This is the
  shared-library home for the rule per `docs/server/architecture.md`
  ("Shared Functionality in Libraries").
- [ ] Add a unit test for the helper in `server/auth/tokens_test.go`
  (table-driven: empty / 1 byte / 31 bytes / 32 bytes / 64 bytes).
  Tests live next to the helper, not in `server/main_test.go`.
- [ ] Update `server/auth/README.md` — add a one-line entry for
  `ValidateSigningSecret` under "Key files" / `tokens.go`.
- [ ] Add `--jwt-signing-secret` flag (string, default empty) to
  `parseFlags` in `server/main.go`. **No env-var fallback.**
- [ ] Add `jwtSigningSecret` field to `serverConfig`.
- [ ] In `main()`, in the same early-flag-validation block as the
  existing `--invite-link-hostname` check (`server/main.go:462-465`),
  call `auth.ValidateSigningSecret([]byte(cfg.jwtSigningSecret))`. On
  error, write `fmt.Fprintln(os.Stderr, "error: --jwt-signing-secret is
  required and must be at least 32 bytes")` and `os.Exit(1)`. The check
  is unconditional — no `cfg.devMode` branch. Match the
  `--invite-link-hostname` precedent: same idiom, same lifecycle phase
  (before the structured logger is initialized).
- [ ] Replace `[]byte("dev-secret-key")` at `server/main.go:525` with
  `[]byte(cfg.jwtSigningSecret)`. Remove the `// TODO: Use environment
  variable in production` comment.
- [ ] After the structured logger is initialized
  (`server/main.go:487-492`), optionally emit a single
  `logger.Info("JWT signing secret configured")` — *no* length, *no*
  prefix, *no* `MaskToken` of the secret. The secret itself never
  enters logs.
- [ ] Generate a 32-byte literal via `openssl rand -base64 32` and add
  `--jwt-signing-secret=<literal>` to the `scripts.start:server`
  command in `package.json`, alongside the other `--*-key` flags
  already there.
- [ ] Update the other server-binary launchers found in the repo to
  pass the same dev flag literal:
  - `scripts/profile.sh` (line ~108: `./tmp/server \\` invocation)
  - `scripts/sql_profile.sh` (line ~55: `./tmp/server \\` invocation)
- [ ] Run `npm run build` and `go build -tags=integration ./...` per
  CLAUDE.md.

**Validates:** A binary built from `HEAD` cannot start without a flag-
supplied 32+ byte secret in any mode, and `npm run start:server`,
`scripts/profile.sh`, and `scripts/sql_profile.sh` still boot cleanly
for local contributors.

### Phase 2: GCP Secret Manager + Cloud Run wiring (dev and prod)
**Goal:** Provision per-environment secrets in Secret Manager and surface
them to the Cloud Run binary as the `--jwt-signing-secret` flag.
**Tasks:**
- [ ] In `terraform/environments/dev/main.tf`, add a `random_password`
  resource (length ≥ 32) and a `google_secret_manager_secret` +
  `google_secret_manager_secret_version` pair named
  `jwt-signing-secret-dev`. Match the conventions used for
  `gear-db-password` and the other Secret Manager entries already in
  the file.
- [ ] Mirror in `terraform/environments/prod/main.tf` as
  `jwt-signing-secret-prod`.
- [ ] Wire the secret into the Cloud Run service so it lands as a
  `--jwt-signing-secret=<value>` flag on the container command — same
  mechanism used for the existing secret-bearing flags
  (`--mapbox-access-token`, `--mailgun-api-key`, etc.). Inspect
  `terraform/modules/containers/` to confirm whether to add a new
  container arg or pass through an env-var-substituted arg, and
  follow the existing pattern.
- [ ] Apply Terraform in dev first; verify the dev Cloud Run service
  comes up and serves `/health` cleanly with the new flag in place.
- [ ] Apply in prod after a successful dev deploy.
- [ ] Document the secret and rotation procedure in `docs/server/`
  (add a short `docs/server/secrets.md` if no equivalent ops runbook
  exists).

**Validates:** Both environments boot from a per-environment, randomly-
generated secret stored in Secret Manager and passed via flag.

### Phase 3: Rotation and cleanup
**Goal:** Invalidate any tokens minted under the leaked literal.
**Tasks:**
- [ ] After Phase 2 deploys, restart the prod Cloud Run revision so the
  new secret is in effect. Outstanding access tokens signed with
  `dev-secret-key` fail validation; clients refresh transparently
  using their unaffected refresh tokens.
- [ ] Grep the repo for stray `dev-secret-key` occurrences outside
  test fixtures and remove them. (Tests use `"test-secret-key"`,
  a different literal — those `TokenConfig` instances never run
  through `main()` and are fine.)
- [ ] Close issue #1284 with a link to the merged PR.

**No bespoke regression guard is needed.** The unconditional 32-byte
minimum in Phase 1 rejects `dev-secret-key` (14 bytes) automatically;
the unit test covering "short secret rejected" subsumes a separate
string check. Don't add a grep-based CI gate — it's dead weight.

**Validates:** Any tokens forged with the leaked literal are no longer
honored anywhere we deploy.

## Related Issues
- #1286 — duplicate, closed in favor of this issue.
