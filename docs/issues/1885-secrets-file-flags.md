# Issue #1885: security: pass server credentials via file paths instead of cmdline flags

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1885
**Filed:** 2026-05-12 | **Last activity:** 2026-05-12
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

After #1884 landed, the server binary is hermetic — Secret Manager fetches happen in `server/entrypoint.sh` (Cloud Run) and `scripts/run_server_with_sm_secrets.sh` (local dev), then the values are passed to the server via `--<name>=<value>` flags. Cloud Run service config no longer carries plaintext values.

The residual surface: each credential value sits in the server process's `argv`, visible to anything inside the container that can read `/proc/<pid>/cmdline`. In Cloud Run with gVisor isolation this is a low-severity exposure (only the server itself reads `/proc` inside its own container; container breakout is rare). But the upgrade path is cheap and there are two collateral wins worth folding in:

1. **Local benchmark/eval scripts** (e.g., from #1939 Phase 5) pass `-anthropic-api-key=$(cat /tmp/...)` directly to `go test`, exposing the value in `ps` on shared dev hosts.
2. **Database password** is currently embedded in the `-db=postgres://user:PASS@host/db` URL — same `/proc/cmdline` exposure, and arguably worse since it's the highest-value credential of the ten.

Goal: add `-<name>-file=<path>` companion flags for every credential and migrate the entrypoint/wrapper scripts to write fetched values to a tmpfs and pass file paths instead.

## Context from Comments

No comments on the issue. The issue body is self-contained and gives a clear technical proposal with cost estimate and a clean before/after table. The lack of discussion suggests this is a low-priority follow-up that was filed and parked rather than debated — proceed without ambiguity, but flag this as Open Question 1 in case a maintainer wants to redirect scope.

## Potential Solutions

### Option A: Per-flag companion `-file` variants (recommended)

- **Approach:** For each of the 10 credential flags (plus the DB URL — see Open Q3), add a sibling `-<name>-file=<path>`. Add a single helper `flagOrFile(name, value, path string) (string, error)` that returns the trimmed file contents when `path` is set, the raw value when `value` is set, and an error on both-or-neither. Validate after `flag.Parse()`. Update the entrypoint scripts to write fetched values to a tmpfs (`/tmp/secrets/`) with mode `0400` and exec the server with `-<name>-file=...` paths. The helper must never log secret contents; surface errors via the standard `logging.LoggerWithContext` patterns (per `docs/server/observability.md` §1), and on file-read failure include the path and `error` field but not the bytes. Treat each successful resolution at INFO with only the flag name (no value), and any precedence/conflict at ERROR.
- **Pros:** Surgical. Each credential's two flags stay grouped in `main.go`. Test surface is the helper itself plus a smoke test that the file path round-trips. Backward-compatible — value flags still work, so integration tests and any external runners don't need updates immediately.
- **Cons:** Roughly 20 LoC of per-credential plumbing in `main.go` (10 credentials × 2 lines). The helper-call duplication is mild; a small slice-driven loop would dedupe but reduce locality.
- **Effort:** small — half-day, mostly mechanical.

### Option B: Unified `-secrets-dir=<path>` convention

- **Approach:** A single new flag `-secrets-dir=<path>`. The server reads each credential from `<path>/<name>` (matching the Secret Manager secret names: `openai-api-key`, `anthropic-api-key`, etc.). Value-flag fallback stays for back-compat. Entrypoint writes the same `/tmp/secrets/` tree the server expects.
- **Pros:** Single flag for the operator to set. No per-credential plumbing growth — adding an 11th secret needs one line in the entrypoint, zero lines in main.go (just one more `loadSecret("foo")` call). Maps cleanly to k8s secret mounts, which already produce this exact layout (`subPath` per key).
- **Cons:** Less explicit at the flag-parse boundary — a typo in a filename inside the directory only surfaces when the credential is consumed. Less back-compat: a partial migration mixing dir-based and value-flag credentials is awkward. Diverges from the issue's stated apache/nginx-style `-<name>-file=<path>` proposal.
- **Effort:** small — but the divergence from the issue's proposal needs a maintainer signoff before we pick it.

### Option C: Defer

- **Approach:** Close as won't-fix or move to a long-tail "if we ever need it" bucket. The issue itself acknowledges the post-#1884 surface "is fine."
- **Pros:** Zero effort.
- **Cons:** Doesn't address the local-eval-script exposure surfaced in #1939, which is the operationally relevant case today (shared dev workstations / Claude Code agent runs). Leaves DB password in `ps` output. No upgrade path when we eventually want to lock down further.

## Recommendation

**Option A.** It's exactly what the issue proposes, fits the existing flag-per-credential model in `main.go`, and the cost is genuinely small (the issue's "half-day of work" estimate is accurate based on the current `main.go:226-275` flag block). The dedupe gain from Option B is real but small — we only have 10 secrets, the per-credential lines in `main.go` are 2 lines each, and the explicitness of per-flag validation is worth the boilerplate.

Make sure the helper handles the trimming question deterministically (trailing newline from `printf '%s' "$VALUE" > /tmp/secrets/foo` vs. multi-line PEM-encoded GitHub App key). Recommendation: read the file verbatim, trim only the trailing `\n` if present, and document that secrets must not contain leading whitespace.

## Open Questions

- **Should this work be picked up now or stay parked as a low-priority follow-up?** Recommendation: pick up now. The original issue framed it as "not blocking," but the local-eval-script exposure surfaced in PR #2015 (#1939 Phase 5) — where benchmark scripts passed `-anthropic-api-key=...` directly on the cmdline — is an active source of leaks on shared dev hosts. Doing this now closes both surfaces in one PR.

- **Does the DB URL get the same treatment?** Recommendation: yes, via a slight variant. The password is embedded inside a connection URL (`-db=postgres://user:PASS@host/db`), so a literal `-db-file=` would force the operator to put the full URL in a file. Cleanest split: keep `-db=postgres://USER:HOST/DB` for the URL (no password), and add a sibling `-db-password-file=<path>` that the server uses to construct the final URL at startup. Otherwise we end up with the worst of both worlds — flag still in `ps`, password still in URL.

- **Do we also remove the value flags as a follow-up, or keep them indefinitely for tests?** Recommendation: keep them for now. Integration tests (`server/services/*_test.go`) and any harness that boots a server in-process pass values directly. Removing the value flag means migrating all of those to write tmpfiles, which is real churn for no production benefit. Revisit when/if we lock down with a `-disallow-value-flags` build mode.

- **Where do the tmpfiles live in Cloud Run?** Recommendation: `/tmp/secrets/<name>`. Cloud Run's `/tmp` is already an in-memory tmpfs (per the Cloud Run runtime contract). No persistent-storage risk, no additional volume config needed. Set mode `0400` after write.

- **Local-dev: how do we delete the tmpfiles?** Recommendation: write them under `mktemp -d` (already done in `run_server_with_sm_secrets.sh`), and have the *server* delete them once it has loaded the value. This is a small departure from "operator owns the files" but matches the local-dev one-shot lifecycle and avoids leaving secret files in `/tmp` if the server crashes before the wrapper's `EXIT` trap fires. In Cloud Run the container disappears on revision teardown so the rm is decorative — still cheap, do it consistently.

## Implementation Plan

### Phase 1: Helper + flag pairs

**Goal:** Wire the `flagOrFile` helper and every credential's `-file` companion flag, with values still flowing through value flags. Pure additive change; production behavior unchanged.

**Tasks:**
- [ ] Add `flagOrFile(value, path string) (string, error)` helper in `server/secretsflag/` (new package, flat under `server/` to match the repo's convention — see `server/connecterr/`, `server/clock/`, `server/logging/`; there is no existing `server/internal/` tree) with unit tests covering: value-only, file-only, both-set (error), neither-set (returns empty string — required-vs-optional is the caller's call), trailing-newline trim, missing-file error. Add a `doc.go` per CLAUDE.md (`revive.package-comments`) and a `README.md` covering purpose and when to add code here vs. an adjacent directory.
- [ ] Add 10 `*-file` companion flags in `server/main.go` parallel to lines 238-265. Order them next to their value-flag counterparts.
- [ ] Replace `*openAIAPIKey` etc. consumption with a small block right after `flag.Parse()` that resolves each to a single local var via `flagOrFile`.
- [ ] Add 11th case for DB password (see Open Q3): add `-db-password-file` and rewrite `-db` parsing to inject the password into the URL when the file flag is set. Reject `-db` containing a password when `-db-password-file` is also set.
- [ ] `go test ./server/...` + `npm run build` + `go build -tags=integration ./...` (per CLAUDE.md). `npm run build` runs `golangci-lint` (gofumpt/godot/revive) plus `go build ./...`; the integration-tagged build is required because adding `*-file` flag plumbing in `server/main.go` is a cross-package signature ripple that integration tests may import.

**Validates:** Server still boots and integration tests still pass with value flags. New `-file` flags are recognized but unused in production paths.

### Phase 2: Migrate the two entrypoint scripts

**Goal:** Both scripts write fetched values to tmpfiles and pass `-file` paths to the server.

**Tasks:**
- [ ] `server/entrypoint.sh`: mkdir `/tmp/secrets`, chmod `0700` on the dir, write each fetched secret to `/tmp/secrets/<name>` with `umask 077` before the write (the file inherits `0600`; immediately `chmod 0400`). Replace each `-<name>=$VAR` exec arg with `-<name>-file=/tmp/secrets/<name>`. Drop the now-unused `OPENAI=$(fetch_secret openai-api-key)` etc. shell vars.
- [ ] `scripts/run_server_with_sm_secrets.sh`: keep the existing `mktemp -d` (already used for the parallel-fetch step), just stop reading into shell vars (`OPENAI=$(<...)` etc.) and instead pass `--<name>-file="$TMPDIR/<name>"`. Drop the pre-exec `rm -rf "$TMPDIR"` so the files survive the exec; have the server delete them itself (or accept the lifetime-of-tmpdir leak — see Open Q5).
- [ ] Add a small integration test under `server/integration_tests/` (the existing path — `server/integration/` does not exist; see `ls server/`) that boots the server pointing at a tmpdir of fake credentials and confirms it reads them. Gate behind `//go:build integration` per CLAUDE.md so it runs only under `go test -tags=integration ./...` and stays out of the fast local loop.
- [ ] Verify on Cloud Run dev revision: `/proc/<pid>/cmdline` shows paths, not values.
- [ ] Verify locally: `ps aux | grep server` shows paths, not values.

**Validates:** End-to-end: secrets reach the server via paths only; `cmdline` is clean. Dev & prod parity confirmed by manual inspection.

### Phase 3: Eval-script migration (collateral cleanup)

**Goal:** Stop passing API keys via `-go-test -anthropic-api-key=$(cat /tmp/...)` in eval/benchmark runs.

**Tasks:**
- [ ] Sweep the test corpus for `-anthropic-api-key=`, `-openai-api-key=`, `-gemini-*-key=` patterns in `go test` invocations or `*_test.go` files. Likely candidates: `server/ai/eval/`, `server/jobs/*_integration_test.go`, `server/services/experience/*_integration_test.go`, `server/ai/provider_*_streaming_test.go`.
- [ ] Add a test helper `eval.LoadProviderCreds()` (or extend the existing `provider_setup_test.go`) to read each credential from `${RIPLS_SECRETS_DIR:-$HOME/.config/ripls/secrets}/<name>` when no env var override is set, with the env-var path still supported for CI. CI keeps using env vars; local dev uses files.
- [ ] Document the local-dev convention in `docs/server/testing.md` or a new `docs/server/local_secrets.md`.

**Validates:** Local benchmark/eval runs (e.g., `go test -tags=benchmark ./server/ai/eval/`) no longer expose keys in `ps`. CI behavior unchanged.

## Audit Notes

Reviewed against `docs/server/architecture.md` and `docs/server/observability.md`; client docs (`docs/client/*`) confirmed not applicable — this is a server-only flag-parsing change.

Changes made:

1. **Package location.** Moved the new package from `server/internal/secretsflag/` to `server/secretsflag/`. The repo has no `server/internal/` tree today; every shared library sits flat under `server/` (`server/connecterr/`, `server/auth/`, `server/clock/`, `server/logging/`). Followed the shared-library template in `architecture.md` §4 and `server/notifications/`.
2. **Required scaffolding.** Called out the `doc.go` (per `revive.package-comments` in CLAUDE.md) and `README.md` (per the multi-file-directory rule in CLAUDE.md).
3. **Observability alignment.** Added a note that the helper must use `logging.LoggerWithContext` and the standard field set (`operation`, `error`), and must never log secret bytes — straight from `docs/server/observability.md` §1.4 (Sensitive Data Protection).
4. **Build verification.** Expanded the Phase 1 build-step bullet to spell out what `npm run build` covers and why the integration-tagged build matters here (per CLAUDE.md "Go Server Build Verification").
5. **Integration test location.** Corrected `server/integration/` → `server/integration_tests/` (the directory that actually exists) and required the `//go:build integration` tag so it stays out of the default test run.

No other changes. The flag-parsing approach itself is consistent with the existing `server/main.go` pattern (lines 226-275), the secret-handling guidance in CLAUDE.md ("Don't land secrets in chat"), and the established observability conventions.

## Related Issues
- #1766 — security: secrets cleanup and prevention (umbrella) — parent
- #1884 — SM-fetch in entrypoint (merged) — direct prerequisite
- #1767 — rotate leaked credentials — independent, not blocking
- #1772 — DIY gitleaks stack (merged) — prevents net-new commits of secrets; this issue addresses runtime exposure
- #1883 — post-cutover docs and cleanup after SM-fetch lands
- #1891 — Add shellcheck to CI — would catch entrypoint.sh regressions during Phase 2
- #1939 / PR #2015 — surfaced the local-eval-script exposure that motivates Phase 3
