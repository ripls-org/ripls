# Issue #1340: [error_handling] Wrap `connect.NewError(CodeInternal, err)` to stop leaking raw internals

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1340
**Filed:** 2026-04-25 | **Last activity:** 2026-04-29
**Labels:** auto-filed, review:error_handling | **Assignees:** unassigned
**Context docs:** docs/reviews/reports/error_handling_review.md

## Problem Statement

Roughly 207 call sites under `server/services/` (and ~290 more across the wider
`server/`) construct an internal-error response with `connect.NewError(connect.CodeInternal, err)`
where `err` is a raw error returned from storage, a third-party SDK, or
`json.Unmarshal`. Connect serializes that error's `Error()` string into the
response body, so clients see whatever the underlying library produced —
Postgres constraint violations (`pq: duplicate key value violates unique
constraint "users_email_key"`), JSON decoder messages with column offsets,
third-party API bodies, and so on.

This has two concrete impacts:

1. **Information disclosure.** Users see schema names, table names, and other
   server-internal hints. In some cases the raw text contains values from the
   request itself (e.g. emails, IDs in constraint messages).
2. **Crash-reporting noise.** Flutter's `RpcErrorHandler` treats
   `Code.internal` as reportable and forwards the message into our crash
   analytics. The high-cardinality, low-signal SQL/JSON text drowns out real
   crashes.

The review names ~206 sites; a fresh count finds **207** sites of the
literal `connect.NewError(connect.CodeInternal, err)` shape under
`server/services/` (490+ across `server/` if you also count
`fmt.Errorf("…: %w", err)` wrappers, which leak the same raw text through
the `%w`). Both shapes are in scope.

There is no existing helper for this — `server/logging/` provides logging
primitives but no error-wrapping utilities, and there is no `connecterr` or
`rpcerr` package in the tree.

## Context from Comments

The only substantive comment is from the triage pass on 2026-04-28
(maintainer) which set this to **Type: Task / Priority: P1** with the
rationale: "error_handling High durable pattern: ~206 sites leak raw
`connect.NewError(CodeInternal, err)` to clients."

The issue body itself is the canonical reference. The recommendation it
inherits from `docs/reviews/reports/error_handling_review.md` is:

> Add `connecterr.Internal(ctx, logger, "operation", err)` helper that logs
> the wrapped error server-side and returns a generic-message `CodeInternal`
> to the client. Migrate sites incrementally, starting with streaming-gen
> files. Add a ruleguard/revive rule forbidding bare
> `connect.NewError(connect.CodeInternal, err)` outside the helper.

No counter-arguments or scope-narrowing comments have been posted.

## Potential Solutions

### Option A: New `server/connecterr` package, one helper that logs + wraps

- **Approach:** Create `server/connecterr/` with a single helper:
  ```go
  // Internal logs err at Error level using the context logger, then returns
  // a connect.CodeInternal error whose public Error() is a fixed generic
  // string. The original err is never serialized to the wire.
  func Internal(ctx context.Context, op string, err error, kv ...any) error
  ```
  Internally: `logging.LoggerWithContext(ctx).ErrorContext(ctx, op, "error", err, kv...)`,
  then `return connect.NewError(connect.CodeInternal, errors.New("internal server error"))`.
  Migrate the 207 service sites in batches by sub-package; remove redundant
  `logger.ErrorContext(ctx, "msg", "error", err)` calls that immediately
  precede the wrap (the helper now owns that log line). Add a CI grep-gate
  script (`scripts/check_codeinternal.sh`) wired into the existing Go CI job
  that fails on `connect\.NewError\(connect\.CodeInternal,` outside the new
  package, with a per-file allowlist that drops to zero by the end of the
  migration.
- **Pros:** Single call replaces a logger-line + error-wrap pair, so net
  line count goes *down*. Logger lookup is centralised, so adding fields
  (e.g. `code: "internal"`) later is a one-line change. Generic public
  message ("internal server error") is what every other production gRPC
  service ships, and removes the security/PII concern entirely. CI gate
  with allowlist is mechanical and incremental. Mirrors the
  `logging.GoSafe` / `logging.MaskEmail` precedent — small leaf packages
  that own one cross-cutting concern.
- **Cons:** New top-level package adds one more import. Helper "swallows"
  the original `err` (it never reaches the client), which is a minor
  behaviour change for any client that was relying on the message contents
  — none should be (Code is the contract, not the message), but worth
  calling out.
- **Effort:** Large (helper + tests + ~207 site migration + CI gate). Most
  of the work is mechanical site-by-site migration, splittable into
  per-sub-package commits.

### Option B: Extend `server/logging` with an error-returning helper

- **Approach:** Add `logging.InternalError(ctx, op, err, kv...)` to the
  existing `server/logging/` package, returning a `connect.CodeInternal`
  error. No new package.
- **Pros:** Zero new package; reuses an already-imported one (almost every
  service file already imports `logging`).
- **Cons:** Forces `server/logging` to depend on
  `connectrpc.com/connect`, coupling a generic logging package to a
  specific RPC framework. The `logging` package's `README.md` and
  `doc.go` describe it as a structured-logging utility, and adding
  RPC-specific helpers blurs that. Also makes future per-code helpers
  (`InvalidArgument`, `NotFound`) feel awkwardly placed in a logging
  package.
- **Effort:** Same migration cost as Option A, slightly less initial
  scaffolding.

### Option C: Custom `analysis.Analyzer` plugin via `golangci-lint custom`

- **Approach:** Same helper as Option A, but instead of a grep-gate, write
  a Go static-analysis pass that checks `*ssa.CallCommon` for
  `connect.NewError(connect.CodeInternal, …)` outside an allowlist of
  packages, then ship it as a custom `golangci-lint` plugin via the
  `custom` linter mechanism.
- **Pros:** Type-aware, robust against renamed imports (`cn "connectrpc.com/connect"`).
  No false positives from comments or strings.
- **Cons:** Custom plugins require building a `golangci-lint` binary at the
  exact version used by CI; this is a known maintenance burden in the Go
  ecosystem. The grep-gate from Option A catches the same cases at >99%
  precision because the codebase consistently uses `connect.` as the
  alias. Not worth the tooling complexity for a one-off rule.
- **Effort:** Medium-large just for the analyzer + CI wiring, on top of
  Option A's migration cost.

## Recommendation

**Option A.** A new leaf package `server/connecterr/` is the right home for
this — it owns one cross-cutting concern (Connect error construction), keeps
the existing `server/logging` package focused on logging, and matches the
shape of other small leaf packages in the tree (`server/clock`,
`server/timezone`).

Public message: a single fixed string `"internal server error"`. Per-site
context belongs in the *log line*, not in the wire response. If a future
caller needs to surface a *user-friendly* generic message ("Something went
wrong while saving — try again"), that's a different code path
(`CodeUnavailable` or a new helper, not this one) and out of scope.

CI enforcement: a shell script (`scripts/check_codeinternal.sh`) wired
into `.github/workflows/test_go.yaml`, modelled on the **ratchet**
pattern that `scripts/check_a11y.js` already uses for accessibility
regressions (header comment: *"The allowlist can SHRINK but never
GROW on main"*). Each line in `scripts/codeinternal_allowlist.txt`
is `<relative-path> <count>` — the number of bare
`connect.NewError(connect.CodeInternal, …)` occurrences in that file
at baseline-bake time. The script greps the tree per file and:

- **Fails** if any allowlisted file's actual count exceeds its
  allowlisted count (catches new violations sneaking into an
  already-violating file).
- **Fails** if any non-allowlisted file contains the pattern at all
  (catches new files introducing the pattern).
- **Warns** (does not fail) when a file's actual count is *below* its
  allowlisted count, or when a listed file has zero actual matches —
  these are stale entries the next migration commit should
  tighten.

This is the programmatic guard against future bare call sites: a
plain file-exclusion allowlist would let allowlisted files silently
accrue new violations — the per-file count closes that gap. The
script also supports `--update-baseline` (mirroring
`check_a11y.js --update-baseline`) to regenerate the allowlist from
current state, used only when intentionally adopting a new
baseline. The final migration phase deletes the allowlist file and
the `--allowlist` flag, leaving an unconditional grep-gate.

## Open Questions

- **Should the helper accept a logger explicitly, or read it from `ctx`?**
  Recommendation: read from `ctx` via `logging.LoggerWithContext(ctx)`.
  Every existing site already constructs its logger from context, and
  passing the logger through adds noise without benefit. Sites that have
  already enriched the logger with `.With(...)` fields can pass those
  fields as the trailing `kv ...any` argument or, more simply, just put
  the per-call fields in the `kv` and drop the local `logger.With(...)`
  altogether. This is consistent with how `logging.GoSafe(ctx, name, fn)`
  was designed — context-driven, no logger argument.

- **What should the helper do with `fmt.Errorf("…: %w", err)` wrapped
  errors? Should the migration unwrap them?**
  Recommendation: pass through — the helper already discards the message
  by replacing it with the fixed public string. Migration commits should
  delete the `fmt.Errorf` wrap when it's pure boilerplate (`"failed to X:
  %w"`) and fold that string into the `op` argument so it shows up in the
  log. Wraps that add real structured context (sentinel errors for
  `errors.Is` checks) stay as-is.

- **Should we also wrap `connect.CodeUnknown` / other "internal-ish"
  codes?** Recommendation: out of scope for this issue. The review
  explicitly scopes to `CodeInternal`; expanding the helper later is
  cheap. Keep the surface area minimal for now.

- **How do we handle the ~290 non-services sites (jobs, middleware, ai,
  etc.)?** Recommendation: in-scope but in a later phase. Streaming-gen
  and services are user-facing; jobs and middleware mostly produce errors
  that never reach a client. Phase the migration: services first
  (user-impact), then the rest, then turn on enforcement everywhere.

- **What about the public Error message — should it include the request
  ID?** Recommendation: keep the message bare ("internal server error")
  for now. The `X-Request-ID` header already round-trips (set by
  `server/middleware/request_id.go`, surfaced to clients), so the
  request ID is correlatable without baking it into the error message.
  Including it in the message duplicates information the client
  already has and risks leaking the format if anything ever changes
  about how request IDs are generated. If a future need arises (e.g.
  Flutter wants to display "Request ID: …" in an error toast), adding
  it later is a one-line change in `connecterr.Internal` — keep
  options open by not committing to a wire format.

- **What if the caller passes `nil` for `err`?** Recommendation: treat
  it as a programming bug. The helper should log a `Warn`-level entry
  (`"connecterr.Internal called with nil error"`, with stack trace via
  `runtime/debug.Stack` so the offending site is findable) and still
  return a valid `CodeInternal` error so the caller's contract holds.
  Don't panic — a nil-err programming bug at the error-return path is
  exactly the moment when crashing the request handler would be worst.

## Implementation Plan

### Phase 1: Helper package and tests

**Goal:** Land the `connecterr` helper, fully tested, with no migration yet.
This is a no-op for callers; it just makes the helper available.

**Tasks:**
- [ ] Create `server/connecterr/` with:
  - `doc.go` — package comment per `revive.package-comments`
    (the build will fail without it).
  - `internal.go` — the `Internal(ctx, op, err, kv...) error` function.
    All exported symbols must have doc comments starting with the
    symbol name (`revive.exported` is `severity: error`). Use
    `logging.LoggerWithContext(ctx).ErrorContext(ctx, op, "error", err, kv...)`
    so that `operation` is the `op` parameter (matches
    `docs/server/observability.md` field-name conventions). Public
    `Error()` returns the bare string `"internal server error"`.
  - `internal_test.go` — table-driven tests covering: returned error
    has `connect.CodeOf(err) == connect.CodeInternal`; public `Error()`
    is exactly `"internal server error"`; log line is emitted at
    `LevelError` with `operation`, `error`, and any `kv` fields, using
    a captured `slog.Handler` to assert structure; nil `err` produces
    a `Warn`-level "called with nil error" log entry plus a valid
    `CodeInternal` return value (does not panic).
  - `README.md` — purpose, key types, when to add code here vs.
    `server/logging`. Do **not** name vendors here per the
    proto-comment ban; this is a non-proto README so vendor names are
    fine to mention as orientation if needed.
- [ ] Add `server/connecterr` to the package list expected by the
  test-coverage gate (`.github/workflows/test_go.yaml` step "Require
  tests on every Go package"). It must ship with tests, so it should
  *not* go into `.github/coverage-allowlist.txt`.
- [ ] Confirm `npm run build` passes; no callers yet.

**Validates:** The helper compiles cleanly, tests pass, log+return
behaviour matches the spec, and it imports nothing from `server/services`
(no cycles). `revive.exported` and `revive.package-comments` both
satisfied.

### Phase 2: CI ratchet gate (allowlist-seeded)

**Goal:** Programmatically prevent any future bare call site —
whether in a brand-new file or smuggled into an existing
already-violating one — while migration is in flight. Mirror the
ratchet pattern that `scripts/check_a11y.js` already enforces in
this codebase.

**Tasks:**
- [ ] Add `scripts/check_codeinternal.sh` modelled on the ratchet
  in `scripts/check_a11y.js`. Behaviour:
  - Greps for `connect\.NewError\(connect\.CodeInternal,` under
    `server/`, excluding `server/connecterr/`, `server/gen/`, and
    `_test.go` files (tests legitimately construct internal
    errors for assertions on `connect.CodeOf`).
  - Reads `scripts/codeinternal_allowlist.txt`, a table of
    `<relative-path> <count>` lines naming every currently-violating
    file with its current occurrence count.
  - **Fails** if any allowlisted file's actual count exceeds its
    listed count (catches new violations smuggled into already-
    violating files — the gap a plain file-exclusion allowlist
    would leave open).
  - **Fails** if any non-allowlisted file contains the pattern
    even once (catches new files introducing the pattern).
  - **Warns** (does not fail) when a file's actual count is below
    its listed count, or when an allowlist entry has zero matches
    — these are stale entries the next migration commit should
    tighten.
  - Supports `--update-baseline` to regenerate the allowlist from
    current HEAD, mirroring `check_a11y.js --update-baseline`.
    This is intentionally not run in CI; it exists only to bake a
    new baseline when a migration commit lands.
- [ ] Generate the initial allowlist with the new script's
  `--update-baseline` mode and check it in. Add a header comment
  mirroring `scripts/a11y_allowlist.txt`'s:
  *"Each entry is `<path> <count>`. The allowlist can SHRINK but
  never GROW on main — the count for any given file may only
  decrease, and adding a new file requires removing one. CI
  enforces this ratchet."*
- [ ] Wire the script into `.github/workflows/test_go.yaml` as a
  step in the Go job (alongside the existing build/lint steps).
  Locally, expose it as `npm run lint:go:codeinternal`.
- [ ] Add a one-paragraph `## Error wrapping` section to
  `docs/server/architecture.md` (or the appropriate server doc)
  pointing at the helper and the gate, and explicitly noting
  that the ratchet means **new violations fail CI even in
  files that already had violations** — the allowlist is a
  count, not a per-file blanket exemption.

**Validates:** CI fails on any new violation — both in
brand-new files and as growth in already-violating files. Existing
207 sites continue to compile. The allowlist enumerates exactly
the files (and their counts) that still need migration; that
total is the migration progress meter.

### Phase 3: Migrate streaming-gen files

**Goal:** Cover the highest-risk paths first (long-running streams have
the worst leak amplification — every chunk error reaches the client).

**Tasks:**
- [ ] Migrate every `server/services/*/gen_streaming.go` file to use
  `connecterr.Internal`.
- [ ] For each migrated file: delete the redundant `logger.ErrorContext`
  call that immediately precedes the wrap (the helper now owns it); fold
  the message into the `op` argument.
- [ ] Update each migrated file's entry in
  `scripts/codeinternal_allowlist.txt` to its new count (drop to zero
  → remove the line; partial migration → decrement). The ratchet
  rejects the PR if you forget to tighten the allowlist after
  removing call sites — the script reports stale entries as warnings,
  but the migrating PR should also remove them.
- [ ] Verify `npm run build`,
  `npm run lint:go:codeinternal`, and
  `go test -tags=integration -timeout 20m ./server/services/...` pass.

**Validates:** The streaming hot paths no longer leak raw errors and the
gate continues to pass with a smaller allowlist.

### Phase 4: Migrate remaining `server/services/` files

**Goal:** Drive the `server/services/` allowlist entries to zero.

**Tasks:**
- [ ] Migrate the rest of `server/services/` in per-sub-package commits
  (chat, community, experience, gear, request, transfer, …). Each
  sub-package commit:
  - Replaces `connect.NewError(connect.CodeInternal, …)` with
    `connecterr.Internal(ctx, op, err, kv...)`.
  - Removes the now-redundant log line directly above each call.
  - Updates the migrated files' lines in the allowlist (decrement
    or delete) so the ratchet stays tight.
- [ ] **Test fallout to expect.** Some existing tests assert on the
  raw error message (e.g. `assert.Contains(t, err.Error(), "duplicate
  key")`) or on a wrapped `fmt.Errorf` substring. Those assertions
  must shift to checking `connect.CodeOf(err) == connect.CodeInternal`
  instead. Tests asserting on `Code()` only are unaffected. Plan to
  audit each sub-package's tests before migration, not after — a
  green test run that hides a behavioural regression is worse than a
  test that fails loudly.
- [ ] Run `npm run build` and `go test -tags=integration -timeout 20m ./...`
  after each sub-package commit.
- [ ] When every `server/services/` entry in the allowlist has
  count 0, delete those lines entirely.

**Validates:** All user-facing RPCs return generic CodeInternal messages
and emit a structured log line per error.

### Phase 5: Migrate non-services packages and remove allowlist

**Goal:** Finish the migration and harden the gate.

**Tasks:**
- [ ] Migrate remaining sites under `server/` outside `services/`
  (`server/jobs/`, `server/middleware/`, `server/cmd/`, etc.).
- [ ] Delete `scripts/codeinternal_allowlist.txt` and the
  `--allowlist` flag from `scripts/check_codeinternal.sh`. The script
  now hard-fails on any violation.
- [ ] Update `CLAUDE.md` with a one-liner under
  "Error Handling and Robustness": *"Use `connecterr.Internal(ctx, op,
  err, kv...)` to return `connect.CodeInternal` errors. Bare
  `connect.NewError(connect.CodeInternal, err)` is gated by
  `scripts/check_codeinternal.sh` and will fail CI."*

**Validates:** Zero call sites of the bare pattern remain; the gate is
unconditional and self-enforcing.

## Related Issues

- #1340 — this issue.
- The companion review finding (Flutter side, "Raw exception messages
  exposed to users in ViewModels") is being tracked under the
  `review:error_handling` label; it shares motivation (don't show raw
  internals to users) but lives entirely in `app/lib/presentation/viewmodels/`
  and is out of scope here.
- Prior review reports under `docs/reviews/reports/` consistently flag
  the `CodeInternal` raw-err pattern; this is the durable High that has
  not moved across multiple review cycles.
