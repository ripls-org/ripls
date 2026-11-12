# Issue #1353: [security] SQL identifier injection latent in protosql Query/Delete helpers

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1353
**Filed:** 2026-04-25 | **Last activity:** 2026-04-26
**Labels:** auto-filed, review:security | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Several public methods on `ProtoSQLStorage` accept `fieldName` and `tableName`
as plain `string` parameters and interpolate them directly into SQL with
`fmt.Sprintf` — without quoting or validation. All current callers pass
compile-time literals, so there is no exploit path today. However, the API
shape invites future callers to pass derived or user-influenced values, which
would reintroduce SQL injection without any change to these files. The same
shape exists for `EmbeddingColumnName`, whose `vendor`/`model` strings are
assembled into DDL identifiers.

Affected symbols (as identified in the security review):

| File | Lines | Symbol |
|------|-------|--------|
| `server/storage/protosql.go` | 198, 251, 449, 464, 488, 511 | `QueryByField`, `QueryByFields`, `DeleteByField`, `DeleteByFieldIn`, `QueryDistinctField`, `CountByField` |
| `server/storage/protosql.go` | 525, 546 | `DecrementFieldIfPositive`, `IncrementField` |
| `server/storage/protosql_batch.go` | 133 | `QueryByFieldIn` |
| `server/storage/embedding.go` | 22–25, 86, 92, 112, 125, 181 | `EmbeddingColumnName` and all callers |

## Context from Comments

Only one comment exists (from `maintainer`), which is an `@claude` ping
requesting the fix. No additional design direction beyond what the issue body
states.

## Potential Solutions

### Option A: Double-quote all identifiers (no validation)

Wrap every interpolated identifier in `"double quotes"` in the SQL string.
PostgreSQL double-quoted identifiers are passed through verbatim, turning
injection attempts into invalid identifier names rather than executable SQL.

- **Approach:** Find every `%s` used as a column/table identifier in
  `fmt.Sprintf` SQL strings and change it to `"%s"`. No new validation helper.
- **Pros:** Minimal diff; no new API; fixes the injection surface immediately.
- **Cons:** A caller passing `"foo\" OR 1=1--"` can still craft a malformed
  query (though PostgreSQL rejects unbalanced quotes, so the query fails rather
  than executes). The API remains permissive — typo'd field names give a DB
  error, not a clear message.
- **Effort:** Small.

### Option B: Validate field names against proto FieldDescriptor + double-quote

For functions that receive a `proto.Message` type argument
(`QueryByField`, `QueryByFields`, `QueryByFieldIn`), look up `fieldName` in the
message's `FieldDescriptor` before building SQL. For low-level helpers that
don't receive a message type (`DeleteByField`, `DeleteByFieldIn`,
`QueryDistinctField`, `CountByField`, `DecrementFieldIfPositive`,
`IncrementField`), add a regex guard `^[a-zA-Z_][a-zA-Z0-9_]*$`. Double-quote
all identifiers. Validate `vendor`/`model` in `EmbeddingColumnName` (or at
`SetEmbedder` time) against `^[a-zA-Z0-9_]+$`.

- **Approach:** New `validateIdentifier(s string) error` helper; descriptor
  lookup in higher-level functions; validation before DDL in embedding code.
- **Pros:** Defence-in-depth: invalid names fail loudly with a clear error
  message rather than a DB error. Makes the API self-documenting. The test
  case `"1=1; --"` produces a clear error.
- **Cons:** Slightly larger diff; adds a new dependency on `regexp` in the
  storage package (not currently used there); proto-descriptor lookup is NOT
  viable for `QueryByField`/`QueryByFields`/`QueryByFieldIn` (see correction in
  Recommendation below); treat them identically to the low-level helpers.
- **Effort:** Small-medium.

### Option C: Allowlist table+field combinations via a registry

Build a static map of all valid (table, field) pairs at registration time (in
`NewProtoSQLStorage`) and validate each lookup against it.

- **Approach:** During `RegisterTypes`, enumerate all field names for each
  registered message and store them in an allowlist. `QueryByField` and friends
  check the allowlist.
- **Pros:** Strongest possible validation; no regex maintenance.
- **Cons:** High complexity, extra memory, couples storage registration to
  query helpers in a non-obvious way. Overkill given that low-level helpers
  don't even know their message type.
- **Effort:** Large.

## Recommendation

**Option B** — validate + double-quote.

Option A alone is insufficient because an attacker who can control the input
can still produce a parse error or probe table structure via error messages. The
double-quoting is necessary in all cases; the validation layer is cheap and
turns a confusing DB error into a clear `fmt.Errorf`. Option C is
over-engineered for a latent risk where all callers are currently literals.

**Correction to original Option B rationale:** The proto descriptor lookup
described for `QueryByField`, `QueryByFields`, and `QueryByFieldIn` is not
viable. The `fieldName` parameter in these functions is a SQL **column name**,
not a proto field name. The storage layer flattens nested messages into SQL
columns (e.g., `address.locality` becomes `address_locality`), so proto
descriptor field names do not match SQL column names for nested fields, as
confirmed by `TestSchemaEvolution` calling
`QueryByField(ctx, "address_locality", ...)`. Use the regex guard
`^[a-zA-Z_][a-zA-Z0-9_]*$` uniformly across all ten functions — both the
higher-level ones that accept a `proto.Message` and the lower-level ones that
do not.

For embedding columns, validate at `SetEmbedder` time so errors surface at
startup rather than at query time.

## Open Questions

- **Should `EmbeddingColumnName` return `(string, error)` or should validation
  happen only at init?** — Recommendation: validate in `SetEmbedder` (and
  `EnsureEmbeddingColumn`). Changing `EmbeddingColumnName`'s signature would
  require updating many call sites and is unnecessary since embedder info is
  fixed at startup.

- **Should the double-quoting also be applied to `tableName` in functions that
  already quote it (e.g. `DeleteByField`)?** — Already double-quoted in most
  cases; ensure consistency. Note: `tableName` is already double-quoted only for
  the higher-level functions whose `tableName` is resolved from `allowedTypes`
  (e.g. `QueryByField`, `QueryByFields`). For the lower-level functions that
  accept `tableName` as a raw string parameter (`DeleteByField`,
  `DeleteByFieldIn`, `QueryDistinctField`, `CountByField`,
  `DecrementFieldIfPositive`, `IncrementField`), `tableName` is NOT validated
  against `allowedTypes` at the call site — it is passed directly by callers.
  Apply `validateIdentifier` to `tableName` in these functions as well as to
  `fieldName`.

- **Should `DecrementFieldIfPositive` and `IncrementField` also validate?** —
  Yes, they follow the same pattern. Apply the same `validateIdentifier` call.

## Implementation Plan

### Phase 1: Add `validateIdentifier` helper and double-quote field names in protosql.go / protosql_batch.go

**Goal:** Harden the DML/query helpers against identifier injection.

**Tasks:**
- [ ] Add `validateIdentifier(name string) error` to a new file
  `server/storage/identifier.go` using `regexp.MustCompile("^[a-zA-Z_][a-zA-Z0-9_]*$")`.
  Note: `regexp` is not currently imported in the storage package; this will be
  a new dependency, but it is a stdlib package.
- [ ] In `QueryByField`: call `validateIdentifier(fieldName)`; double-quote `fieldName` in SQL.
  (Proto descriptor lookup is not viable here — field names are SQL column names, not proto
  field names. Flattened nested fields like `address_locality` would not match the
  proto descriptor for field `locality` inside message `Address`.)
- [ ] In `QueryByFields`: call `validateIdentifier` on each key; double-quote each key in SQL.
  (Same reason — SQL column names, not proto field names.)
- [ ] In `QueryByFieldIn` (batch): call `validateIdentifier(fieldName)`; double-quote in SQL.
  (Same reason.)
- [ ] In `DeleteByField`: call `validateIdentifier(tableName)` and `validateIdentifier(fieldName)`;
  double-quote both in SQL. (`tableName` is a raw caller-supplied string, not looked up from
  `allowedTypes`.)
- [ ] In `DeleteByFieldIn`: call `validateIdentifier(tableName)` and `validateIdentifier(fieldName)`;
  double-quote both in SQL. (Same reason as above.)
- [ ] In `QueryDistinctField`: call `validateIdentifier(tableName)` and `validateIdentifier(fieldName)`;
  double-quote both in SQL.
- [ ] In `CountByField`: call `validateIdentifier(tableName)` and `validateIdentifier(fieldName)`;
  double-quote both in SQL.
- [ ] In `DecrementFieldIfPositive`: call `validateIdentifier(tableName)` and
  `validateIdentifier(fieldName)`; double-quote both in SQL.
- [ ] In `IncrementField`: call `validateIdentifier(tableName)` and
  `validateIdentifier(fieldName)`; double-quote both in SQL.

**Validates:** `go build ./...` passes; no existing tests break.

### Phase 2: Harden embedding.go DDL

**Goal:** Ensure embedding column and index names cannot be injected via vendor/model strings.

**Tasks:**
- [ ] In `SetEmbedder`, validate `info.Vendor` and `info.Model` against
  `^[a-zA-Z0-9_]+$` before proceeding. Return an error if invalid.
- [ ] In `EnsureEmbeddingColumn`, double-quote `colName` and `indexName`
  wherever they appear unquoted in SQL (DDL statements).
- [ ] Apply the same quoting to `DropEmbeddingColumn`,`UpdateEmbedding`,
  `QueryByEmbeddingSimilarity`, `CountWithEmbedding`, `QueryWithoutEmbedding`.

**Validates:** `go build ./...` passes; embedding tests pass.

### Phase 3: Add negative tests

**Goal:** Regression-proof the fix.

**Tasks:**
- [ ] In `protosql_test.go`, add `TestIdentifierInjectionRejected` that calls
  `QueryByField` / `DeleteByField` / `CountByField` with `fieldName = "1=1; --"`
  and asserts that an error is returned (not a DB panic or rows).
- [ ] Add a test for `SetEmbedder` with a malformed vendor/model string
  asserting an error.

**Validates:** Negative tests pass; `go test ./server/storage/...` is green.

## Related Issues

- None identified.
