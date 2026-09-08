---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Go server coding conventions — comments, fail-fast error handling, CodeInternal, SSRF-safe outbound HTTP, SQL efficiency, structured logging, build verification, PostgreSQL, server testing, and the CommunityEventType undo-registry gate.
  globs: [server/**]
  triggers: [go, golang, slog, ssrf, n+1, codeinternal, fail-fast, undo-registry]
  lens: [server, conventions]
  alwaysApply: true
  domain: server
freshness:
  verified_commit: "50e6fce55"
  verified_on: "2026-08-26"
---
# Server Coding Conventions (Go)

Conventions every change under `server/` is expected to follow. This is the
coding-rules companion to [`server/architecture.md`](architecture.md) (which
covers system structure) and [`proto_conventions.md`](../proto_conventions.md)
(which covers the proto/API contract).

## Go Code Comments

- Function-level comments are required and should follow Go convention: start
  with the function name followed by imperative description (e.g., "WriteFile
  writes a file..." not "Writes a file...").
- Comments should be complete sentences (end with punctuation).
- Omit trivial comments that just repeat what the code does.
- Focus on explaining why, not what, unless the what is non-obvious.
- **Do not reference implementation phases in source comments.** Comments describe
  durable invariants of the code — not the rollout phase, task, or branch
  that motivated a change. Terms like "Phase 3a", "pre-refactor",
  "new in this PR" rot the moment the surrounding work lands and leave
  readers chasing dead links. That context belongs in the commit message,
  PR description, or plan/eval docs under `docs/`. Issue and PR numbers
  (e.g. `#1157`) are allowed — they provide stable, retrievable context.
  Same rule applies to file-level package comments.
- **Phase markers in `*.go`/`*.dart` comments fail the build.** Enforced by `scripts/check_ephemeral_comments.js` via `npm run lint:ephemeral-comments`.
- **Exported symbols must (a) have a doc comment starting with the symbol name and (b) avoid stuttering with the package name** (e.g. `bytes.Buffer`, not `bytes.BytesBuffer`; `admin.Service`, not `admin.AdminService`). Both are enforced by `revive.exported` at `severity: error` — violations fail the build.

## Error Handling and Robustness

- **Fail Fast**: NEVER silently ignore errors or use "warn and continue" patterns
  unless there is a clear, documented reason why the failure is expected and safe
  to ignore. Soft failures mask bugs and lead to cascading errors that are much
  harder to debug. Every error should either:
  - Be returned to the caller (preferred)
  - Have an explicit comment explaining why it's safe to ignore
  - Be part of a documented fallback/retry strategy

  BAD: `if err != nil { logger.WarnContext(ctx, "operation failed"); /* continue */ }`
  GOOD: `if err != nil { return fmt.Errorf("operation failed: %w", err) }`

- **Connect CodeInternal errors**: Use `connecterr.Internal(ctx, op, err, kv...)`
  to return `connect.CodeInternal` errors. It logs `err` server-side and returns a
  generic public message so raw storage / SDK / decoder text never reaches the
  wire. Bare `connect.NewError(connect.CodeInternal, err)` is gated by
  `scripts/check_codeinternal.js` and will fail CI.

## Security: Outbound HTTP

Any server code that fetches a URL whose host is even partly attacker-controlled
(user-submitted gear URLs, `og:image` entries from fetched pages, generic media
imports) **must** use `server/safehttp.NewClient`. Using a bare `&http.Client{}`
on these paths enables SSRF: an attacker can direct the server to fetch
`http://169.254.169.254/` (cloud metadata), RFC1918 addresses, or loopback
services.

Trusted egress to known, hard-coded endpoints (Mapbox, Pexels/Unsplash/Pixabay
APIs, Gemini, GitHub) may use a plain `http.Client` — the SSRF guard adds
overhead and can false-positive on CDNs with private relay hops.

For context on why this matters and how the guard works, see
`server/safehttp/README.md`.

## SQL Efficiency

1. **Never query in a loop.** Every `for` loop containing a storage call is an N+1 bug. Use batch APIs (`GetByIDs`, `QueryByFieldIn`) or JOINs instead.
2. **Push filtering to SQL.** Use WHERE clauses, not post-query Go filtering. Every row fetched and discarded wastes I/O.
3. **Always LIMIT unbounded queries.** Any SELECT without LIMIT is a memory exhaustion risk. Even "list all" queries need a reasonable cap.
4. **Batch writes.** Use multi-row INSERT (`InsertBatch`) instead of looping single inserts.
5. **Use JOINs for related data.** Fetch parent + children in one query rather than fetching parents then looping for children.
6. **Index what you filter on.** Every WHERE/JOIN/ORDER BY column used in production queries should have an appropriate index.
7. **Use `ILIKE` for case-insensitive matching.** Never use `LOWER(col) LIKE LOWER($1)` — it prevents index use.
8. **Measure before and after.** Use `storage.WithQueryStats(ctx)` / `storage.GetQueryStats(ctx)` for per-request counting and `storage.AssertMaxQueries(t, ctx, max, fn)` in tests to prevent N+1 regressions.

## SQL Safety

All SQL in this server lives in `server/storage/`; no other package issues a
query. Inside it, exactly two things may enter a query string, and
`npm run lint:go:sql` (`server/cmd/check-sql`) fails the build on anything else.

1. **Values are always bound.** Use `s.dbSpec.Placeholder(n)` and pass the value
   as an argument. This includes `LIMIT` and `OFFSET` — PostgreSQL accepts
   `LIMIT $1`, and `fmt.Sprintf("LIMIT %d", n)` is not an exception to the rule.
   Beyond injection, this is what keeps user data out of the logs:
   `InstrumentedDB.recordQuery` writes the first 200 characters of the *query
   text* to Cloud Logging for any query over 100 ms, so an interpolated email or
   search term ends up in the log stream.
2. **Identifiers always go through `quoteIdent`.** `quoteIdent` /
   `quoteIdents` / `qualify` / `quotedTableFor` in `server/storage/sqlident.go`
   validate against `^[a-zA-Z_][a-zA-Z0-9_]*$` and quote with
   `pq.QuoteIdentifier`. Do not hand-roll `"` + name + `"`, and do not use `%q`:
   Go escapes an embedded quote as `\"` where PostgreSQL requires `""`, so `%q`
   is only ever accidentally correct.
3. **Descriptor-derived identifiers are validated once, at startup.**
   `validateSchemaIdentifiers` runs from `initializeDatabase` and refuses to
   start the server if any registered table name or flattened column name is
   unusable. That is why the per-query paths return a `quoteIdent` error rather
   than panicking mid-request.
4. **Escape LIKE metacharacters in user input.** `%`, `_` and `\` are wildcards.
   Use `likeContains` (which wraps `escapeLikePattern`) and emit
   `likeEscapeClause` alongside the pattern. Binding the term is not enough — an
   unescaped `%` matches every row.
5. **Raw SQL fragments need a marker.** A genuinely fixed fragment that the
   checker cannot prove — a `communitySearchConfig` WHERE clause, a
   `DatabaseSpecifics` type keyword — carries
   `// sql-fragment-allow: <why this cannot carry caller input>`. The reason is
   mandatory; a tracking issue is **not** (unlike `go-line-count-allow`, this
   marks permanently-legitimate code, not deferred work). Every marker is a
   claim a reviewer has to check, so keep them few.

**Why a repo-specific checker rather than gosec.** gosec's G201/G202 stay
enabled but find nothing here: they only fire when the receiver of
`Query*`/`Exec*` resolves to a concrete `*sql.DB`, and `ProtoSQLStorage` holds
the `SQLDB` / `SQLExecutor` *interfaces* that `InstrumentedDB` needs for
per-request query counting. `check-sql` matches on the call name instead, so
the indirection does not hide anything from it. See #2795.

## Structured Logging

The server uses structured JSON logging via Go's `log/slog` package. See
[`server/observability.md`](observability.md) for the full treatment.

**IMPORTANT:** Never use `log.Printf`, `log.Println`, or `fmt.Printf` for logging. Always use the structured logging package.

**Key patterns:**
- Use `logging.LoggerWithContext(ctx)` in all RPC handlers and functions with context
- Use `logging.Default()` for startup/initialization logging without context
- Always mask sensitive data: `logging.MaskEmail(email)`, `logging.MaskToken(token)`

**Log levels:**
- `Debug`: Verbose details (cache hits, query timing, read operations)
- `Info`: Normal operations (user actions, successful completions)
- `Warn`: Unexpected but handled (retries, fallbacks, non-critical failures)
- `Error`: System failures requiring attention (database errors, external API failures)

**Standard field names:**
- `request_id` (HTTP correlation ID — reserved for the X-Request-ID header value injected by `LoggerWithContext`; do NOT use for entity IDs), `user_id`, `user_email` (masked), `operation`, `duration_ms`, `error`
- Entity IDs: `gear_id`, `community_id`, `transfer_id`, `location_id`, `experience_id`, `target_request_id` (use `target_request_id`, not `request_id`, for `models.Request` entity IDs to avoid collision with the HTTP correlation key)
- External services: `external_service`, `provider`, `model`

**Example:**
```go
logger := logging.LoggerWithContext(ctx).With(
    "operation", "SaveGear",
    "gear_id", gearID,
)
logger.InfoContext(ctx, "gear saved successfully")
```

## PostgreSQL Only

The server uses PostgreSQL exclusively. Connection strings must start with
`postgres://` or `postgresql://`. The default is
`postgres://ripls:ripls_dev@localhost:5432/ripls?sslmode=disable`.

## Server Testing

- Use `go test ./...` for fast local runs (~60-90s). Tests that make live AI API
  calls are gated behind the `integration` build tag and skipped by default. Use
  `go test -tags=integration ./...` to include them (CI always uses this). When
  adding new tests that call external AI APIs, add `//go:build integration` as the
  first line of the test file.
- **Test Isolation**: Each test should be completely isolated with its own
  temporary directories and unique ports.
- **Database Testing**: Tests use testcontainers with the `pgvector/pgvector:pg16`
  image for PostgreSQL. Use `storage.SetupTestStorage(t)` for unit tests. The
  container is shared across tests within a package for performance.
- **Testcontainers**: testcontainers-go containers are automatically cleaned up
  after tests complete.
- **Resource Management**: Always use proper cleanup patterns with defer
  functions and `t.TempDir()` for test isolation.

## Go Server Build Verification

After making any Go server changes, always run `npm run build` to verify the build passes lint and compiles successfully:

```bash
npm run build
```

This runs `npm run lint:go` followed by `go build ./...`. Fix any lint errors before considering the task complete.

`npm run lint:go` is the whole Go gate, and it is the **same command CI runs** (`.github/workflows/test_go.yaml`, inside the required `Go Tests` job — a lint failure blocks the PR). It chains:

1. `lint:go:fmt` — `gofumpt -l .` must report nothing.
2. `lint:go:vet` — `golangci-lint run --config server/.golangci.yml ./server/...`.
3. `lint:go:vet:benchmark` — `go vet -tags=benchmark ./server/...`, so files behind the `benchmark` build tag (e.g. `server/services/experience/gen_ai_fanout_benchmark_test.go`) still get vetted even though they never compile into the default build.
4. `go run ./server/cmd/check-goroutines` — goroutine-safety checker.
5. The custom gates: `lint:go:codeinternal`, `lint:go:event-terminology`, `lint:go:active-community-gate`, `lint:go:l10n-parity`, `lint:go:no-inline-user-strings`.

**Never invoke `golangci-lint run ./...` from the repo root.** golangci-lint derives its config search directory from the first positional argument, so `./...` resolves to the repo root, finds no `.golangci.yml`, and silently falls back to the default linter set — `server/.golangci.yml` is skipped entirely. That is not hypothetical: it is how this repo ran between #1605 and #1611, during which the config was never once applied. Always go through `npm run lint:go` (or pass `--config server/.golangci.yml` explicitly).

The enabled linter set is deliberately narrower than the file's history suggests; `server/.golangci.yml` lists the linters parked for later phases of #1611 and what un-parks each. Enable one only when its findings are **fixed**, not suppressed — a red required check gets muted, not repaired.

`server/.golangci.yml` uses the **v2 config schema** (#3008): `linters.settings` rather than `linters-settings`, `linters.exclusions.paths`/`.rules` rather than `issues.exclude-dirs`/`exclude-rules`, and a separate top-level `formatters:` block for gofmt/gofumpt/goimports. v1 and v2 refuse to read each other's config, so a v1-shaped edit fails closed rather than being silently ignored. `golangci-lint migrate` converts the schema but **strips every comment**, and in that file most of the content is the reasoning — restore it from git if you ever rerun it.

The pinned version lives in `versions.env` (`GOLANGCI_LINT_VERSION`) and is baked into the runner image, but unlike other baked tools it is *also* honoured pre-merge: `.github/actions/setup-toolchain`'s `go-lint` input installs the pinned version whenever it differs from what is on PATH. That is what lets a lint-toolchain bump be validated by the PR that makes it.

**Also build with the `integration` tag when changing exported APIs.** `npm run build` does NOT compile files behind `//go:build integration` (CI does, via `go test -tags=integration ./...`). Any change to a function signature that an integration test calls — for example, anything in `server/ai/` consumed by `server/services/experience/*_integration_test.go` or `server/services/unified_create/integration_test.go` — needs:

```bash
go build -tags=integration ./...
```

Run this before pushing any cross-package signature change. CI will catch it otherwise, but the round-trip costs minutes.

## Adding New CommunityEventType Values

Every new `CommunityEventType` enum value must be classified in **two**
registries before merge, each with its own CI gate:

1. **Undo registry** — `server/undo/registry.go`, enforced by
   `TestEveryEventTypeClassified`: how (or whether) the action can be
   reversed. Classifications below.
2. **Digest registry** — `server/activity_digest/registry.go`, enforced
   by `TestEveryDigestEventTypeClassified`: which ops-digest reporting
   category the event belongs to and which `email.ActivityTotals`
   counter it increments, or an explicit exclusion
   (`DispositionExcludedRetraction` for UNDONE rows,
   `DispositionExcludedInternal` for bookkeeping signals that are not
   user actions). This keeps the daily/weekly activity digest (#2665)
   reconciling exactly — a new action can never silently vanish from
   the ops email.

If you add an enum value without an entry in either registry, the
server build fails.

When classifying, pick one of:

- `ClassificationClientOnly` — a natural inverse RPC or UI path already
  covers the undo need; no server retraction plumbing needed. Include a
  one-sentence rationale naming the inverse.
- `ClassificationServerSnackbar` — undo via a dedicated `Undo*` RPC
  from a transient snackbar. Populate `Entry.UndoRPC` with the RPC name.
- `ClassificationServerStory` — same as server-snackbar, plus the
  action generates a story whose fetch response surfaces an `UndoableAction`
  to the actor. Populate `Entry.UndoRPC`.
- `ClassificationRetentionRestore` — creation action whose inverse is
  soft-delete + restore (uses `DeletedMetadata`, not an `Undo*` RPC).
- `ClassificationIrreversible` — flag with a confirmation dialog at
  action time.
- `ClassificationInternal` — server-emitted retraction events written
  by `Undo*` RPCs themselves; not user-triggered.

For server-authoritative classifications, also define the
`UndoData` variant in `proto/ripls/models/undo.proto` and populate
`Entry.ValidateUndoData` when the corresponding `Undo*` RPC ships —
`TestEveryServerUndoableActionHasUndoDataValidator` surfaces the
TODOs.

When in doubt, default to `ClassificationClientOnly` with a rationale
explaining why no server-side undo is needed. See
`docs/ai/undo_plan.md` §10.6 for the broader design.
