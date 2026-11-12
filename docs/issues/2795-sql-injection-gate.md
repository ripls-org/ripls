# Issue #2795: [security] Replace the repo-wide gosec G201 exclude with audited per-site suppressions

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2795
**Filed:** 2026-07-29 | **Last activity:** 2026-07-29
**Labels:** review:code_quality | **Assignees:** unassigned
**Context docs:** `docs/llms.txt` (index) · `docs/server/architecture.md` (⚓ server anchor — §Storage Layer) · `docs/server/conventions.md` (⚓ server anchor — §SQL Efficiency, §PostgreSQL Only) · `docs/reviews/security_review_prompt.md` §2 (Input Validation & Injection — the engaged dimension) · `server/storage/README.md` · `server/.golangci.yml`. `docs/proto_conventions.md` (the third anchor) was **not** loaded: this change adds no proto surface and touches no generated types.

> **Scope note.** The requester extended the issue beyond its filed text: *"we should
> generally upgrade our SQL query syntax / substitution to be best-practice compliant,
> even if it's more work."* This plan treats #2795's lint question as Stage 1 and the
> substitution upgrade as the substance of Stages 2–5.
>
> **Delivery: one PR.** The requester asked for all of it in a single pull request, so
> the stages below are ordered commits on one branch (`sql_security`), not separate
> shippable increments. See *Implementation Plan* for the review strategy that keeps a
> change this size reviewable.

> **`ripls-ui-investigate` skipped** — deliberately. #2795 describes a lint-config and
> storage-layer construction concern with no UI-observable behavior; there is nothing
> to reproduce against the web bundle.

---

## Problem Statement

`server/.golangci.yml` mutes gosec **G201** ("SQL query construction using format
string") repo-wide, on a comment-stated assumption that every such site is
parameterized. #2795 asks to (1) enable gosec without the exclude, (2) enumerate and
audit the findings, (3) replace the blanket exclude with per-site
`//nolint:gosec // G201: <why>` directives that `nolintlint` will keep honest.

**Two things have changed since the issue was filed, and one of them invalidates the
prescribed remedy.**

**1. The "currently moot" caveat is stale.** The issue says the exclude "is *currently*
moot because `gosec` itself is not enabled until #1611 Phase 2." PR
[#2817](https://github.com/Type2Fun-Foundation/ripls/pull/2817) merged 2026-07-30 and
enables `gosec`, `nolintlint`, and 39 other linters inside `npm run lint:go`, which
runs in the required `Go Tests` job. gosec is live now.

**2. The exclude is a no-op anyway, and the prescribed fix cannot be written.**
Measured, not assumed: standalone `gosec -include=G201,G202` over `./server/...` —
**705 files, 157,085 lines — reports 0 issues.** Not "0 after suppressions." Zero
findings exist to audit or suppress.

Two independently reproduced blind spots in gosec's `sqlStrFormat` rule explain it,
and between them they cover 100% of this repo's SQL:

| Blind spot | Reproduction | What it hides here |
|---|---|---|
| **Interface-typed executor.** G201 fires only when the receiver of `Query*`/`Exec*` resolves to concrete `*sql.DB` / `*sql.Tx` / `*sql.Stmt`. Against an interface it does not fire — even for `fmt.Sprintf("... WHERE name = '%s'", name)`. | probe: identical query flagged on a `*sql.DB` field, silent on an `Exec` interface field | `ProtoSQLStorage` holds `db SQLDB` and `exec SQLExecutor` — **both interfaces** (`server/storage/protosql_types.go:173-176`, `server/storage/instrumented_db.go:24-39`). Every query in `server/storage/` is invisible. |
| **Inline `fmt.Sprintf`.** `db.Query(fmt.Sprintf(...))` is missed; only `q := fmt.Sprintf(...); db.Query(q)` is caught. | probe: `C1` silent, `C2` flagged, same query text | `server/storage/testing.go:648,672,678` — the only concrete-`*sql.DB` call sites in the tree. |

The interface indirection is not incidental: it is what `InstrumentedDB` needs for
per-request query counting (#1613 metrics, `AssertMaxQueries` N+1 gates) and what
`WithTx` needs to route storage calls through an `InstrumentedTx`. It is good design
that happens to defeat this linter.

**Consequence for step 3 of the issue.** `nolintlint` is configured with
`allow-unused: false`. A `//nolint:gosec // G201: ...` directive on a site gosec never
flags is an *unused* directive, and `nolintlint` fails the build on it. Writing the
issue's prescribed suppressions would turn the required check red. The remedy as
specified is not implementable.

**So the real problem is not "the exclude hides findings." It is that
`server/storage/` — the only package in the tree that issues SQL — has no automated
guard against SQL injection at all, and the config comment claiming one is
load-bearing false comfort.** `docs/reviews/security_review_prompt.md` §2 requires
that every `Query*`/`Exec*` call in `server/storage/` be reviewed for string
concatenation. Today nothing enforces it between reviews.

### What the manual audit found

I read all ~81 dynamic-SQL construction sites across the 18 non-test files in
`server/storage/`. Verified first: **no `Query*`/`Exec*` call exists anywhere outside
`server/storage/`** (0 matches in non-test code), so the audit surface is genuinely
one package.

**The good news, stated plainly: there is no SQL injection in this codebase.** Every
value that could carry user input is bound through `s.dbSpec.Placeholder(n)`. I found
no site where a request-derived string reaches the query text.

The problems are all one tier down — the substitution *discipline* is ad hoc, and the
invariant that keeps it safe is unwritten, unenforced, and unevenly applied.

**A. Identifier interpolation has three protection tiers, and the third is "none."**

| Tier | Mechanism | Coverage |
|---|---|---|
| Closed set | `s.allowedTypes[typeName]` map lookup, registered at startup | Table names on the generic CRUD paths |
| Regex-validated | `validateIdentifier` (`server/storage/identifier.go:18`) | ~13 call sites: `DeleteByField`, `DeleteByFieldIn`, `QueryDistinctField`, `MaxInt64FieldByGroup`, `CountByField`, `DecrementFieldIfPositive`, `IncrementField`, `QueryByField`, `QueryByFields`, `QueryByFieldIn`, array-column helpers, `EnsureEmbeddingColumn` |
| **Nothing** | — | See below |

Unprotected identifier interpolation, in descending order of concern:

- `QueryByProximityForCommunity` (`protosql_spatial.go:118-169`) takes
  `locationRefField, ownerField string` from the **caller** and interpolates both
  into the JOIN and WHERE clauses with no validation. Today every caller passes a
  literal (`"location_id"`, `"owner_id"`), so it is safe *by convention only*. This is
  the single site where a future caller could pass a variable and nothing would stop
  it.
- Column lists in `Insert` (`protosql.go:76-81`), `update` (`:439`), `InsertBatch`
  (`protosql_batch.go:270`) and `scheduled_notification.go:76` come from
  `extractFieldValues` / proto descriptors — a closed set by construction, but nothing
  asserts it, and the `update` SET clause emits them **unquoted** (`%s = %s`) while
  the sibling WHERE clause quotes (`"id" = ...`). Works today only because proto field
  names are lowercase snake_case that PostgreSQL folds identically.
- `scheduled_notification.go:225` interpolates `discriminatorColumn` unvalidated.
- `embedding.go` re-derives `colName` / `indexName` from `EmbeddingColumnName(info)`
  and validates it in `EnsureEmbeddingColumn` (`:87-95`) but **not** on the read paths
  (`:225`, `:262`, `:296`, `:313`) or in `protosql_search.go` (`:139,142,150`).
  `embedding.Info` is currently compile-time constant (`"local"` /
  `"ripls-minilm-v1"`, `server/ai/embedding/embedding.go:36-41`), so there is no live
  exposure — the gap is that the validation is not where the interpolation is.
- `chat_message.go:36` interpolates an `ORDER BY ... %s` direction derived from a
  `bool`. Safe, but it is a raw fragment splice with no marker saying so.

**B. Four incompatible identifier-quoting styles coexist.**

| Style | Sites |
|---|---|
| `"%s"` (manual double quotes) | Most of `protosql*.go`, `community.go`, `embedding.go`, `scheduled_notification.go` |
| `%q` (Go quoting) | `array_columns.go:154,191`, `array_column_schema.go:67,81,106,113` |
| bare `%s` | Column lists, `latField`/`lonField`, `columnName` in `array_column_schema.go`, `discriminatorColumn` |
| hand-rolled `` `"` + f + `"` `` | `embedding.go:741` |

`%q` is **not a SQL identifier quoter**: Go escapes an embedded `"` as `\"`, while
PostgreSQL requires it doubled as `""`, and Go escapes backslashes that PostgreSQL
treats literally. `validateIdentifier` runs first at those sites so nothing is
exploitable — but the codebase is one careless copy-paste away from a `%q` that isn't
preceded by a validation call. `lib/pq` is already a direct dependency (`go.mod:25`)
and ships the correct primitive, `pq.QuoteIdentifier`, which is used nowhere.

**C. Value-side gaps that are not injection but are not best practice.**

- **`LIMIT %d` at 11 sites** (`story.go` ×3, `chat_message.go`, `chat_conversation.go`,
  `protosql_search.go` ×2, `activity_digest.go` ×2, `user_activity.go`,
  `protosql.go:163`). `%d` on a Go `int` cannot inject, but PostgreSQL accepts
  `LIMIT $n` and binding it is the correct form. Worse, `StoryStorage.ListByCommunity`
  / `ListByUser` / `ListByItem` and `ProtoSQLStorage.ListAll` accept an **unclamped**
  caller `limit` — only the floor (`<= 0 → 50`) is applied, no ceiling.
  `protosql_search.go` does clamp (to 50), and `chat_*` default to 200. This is
  inconsistent with `docs/server/conventions.md` §SQL Efficiency rule 3 ("Always LIMIT
  unbounded queries… Even 'list all' queries need a reasonable cap").
- **LIKE metacharacters are not escaped.** `executeTextSearch`
  (`protosql_search.go:207`), `SearchCommunityUsersByName` (`:651`) and
  `SearchProvisionalUsersByName` (`:706`) build `"%" + query + "%"` and bind it. The
  value is bound, so this is not injection — but `%`, `_` and `\` in user input are
  treated as wildcards. A member searching `%` matches every row in the community;
  `_` matches any single character. That is a correctness bug and an unbounded-scan
  risk on a user-reachable path.
- `testing.go:672` interpolates a generated `dbName` into `datname = '%s'`. Test
  infrastructure, but it lives in a **non-`_test.go` file** in a production package,
  so it is linted like production code and reads as the pattern to copy.

**D. Binding everything is what keeps values out of the logs.** `recordQuery`
(`instrumented_db.go:76-93`) logs the first 200 characters of the **query text** at
WARN for any query over 100 ms, feeding the `db_slow_query_duration` metric (#1613).
Because every value is bound today, that log line carries no user data. The moment
someone interpolates a value "just this once" — an email, a phone number, a search
term — it lands in Cloud Logging. That is a second, independent reason the
bind-everything invariant needs a gate rather than a convention, and it is worth
stating in the §SQL Safety doc so nobody optimizes the placeholder away.

**E. The invariant is undocumented.** `docs/server/conventions.md` §SQL Efficiency has
eight rules, all about performance. Neither it, `docs/server/architecture.md`
§Storage Layer, nor `server/storage/README.md` states "values are always bound;
identifiers are always validated and quoted." A new contributor has nothing to read
and nothing to fail against.

---

## Context from Comments

**The issue has zero comments.** It was auto-filed by `/ripls-reviews` from the
code_quality review (label `review:code_quality`) and has had no human discussion.

That matters for two reasons, and both cut toward more scrutiny rather than less:

1. Its central factual premise — that removing the exclude will produce findings to
   audit — was never checked by a human and is **false**, as measured above. An
   auto-filed issue's prescription deserves verification before implementation, not
   after.
2. Its stated rationale ("a `//nolint` with a reason is auditable in review and shows
   up in `git blame`") is sound reasoning about suppression hygiene in general. It is
   simply inapplicable when the linter produces nothing to suppress. The *goal* behind
   it — a durable, call-site-local, CI-enforced record of why each SQL construction is
   safe — is exactly right and is what this plan preserves.

The requester's follow-up instruction ("upgrade our SQL query syntax / substitution to
be best-practice compliant, even if it's more work") is the only human input on the
issue and explicitly authorizes scope beyond the lint config.

---

## Potential Solutions

### Option A: Delete the exclude, document the finding, stop there
- **Approach:** Remove `- G201` from `server/.golangci.yml`, replace the TODO with a
  comment recording that G201 is structurally blind to the storage layer's interface
  indirection, close #2795 as "prescribed fix not implementable; exclude was a no-op."
  Leave gosec enabled un-excluded so a future concrete-`*sql.DB` caller is still
  caught.
- **Pros:** Honest, ~30 minutes, zero regression risk, removes a config line that
  actively misleads. Correctly closes the filed issue.
- **Cons:** Coverage after the change is still zero. Every problem in the audit above
  (§A–D) survives. Ignores the requester's explicit extension.
- **Effort:** Small.

### Option B: Safe-by-construction substitution primitives + a repo-specific gate (recommended)
- **Approach:** Four moves, in order.
  1. **One identifier primitive.** Add `server/storage/sqlident.go` exposing
     `quoteIdent(name string) (string, error)` — `validateIdentifier` then
     `pq.QuoteIdentifier` — plus `mustQuoteIdent` for descriptor-derived names that
     cannot fail. Migrate all ~81 sites to it, retiring `"%s"`, `%q`, bare `%s` and
     the hand-rolled `` `"` + f + `"` ``.
  2. **A repo-specific AST gate.** Add `server/cmd/check-sql`, following the exact
     shape of the existing `server/cmd/check-goroutines` (166 lines, already wired
     into `npm run lint:go`). It enforces what gosec cannot see, on interface and
     concrete executors alike:
     - the query argument to any `Query*`/`Exec*` call must be a string literal, a
       `const`, or a `fmt.Sprintf` with a literal format string;
     - within such a `fmt.Sprintf`, every argument must be an *approved* expression —
       `quoteIdent(...)` / `mustQuoteIdent(...)`, `dbSpec.Placeholder(...)`, a
       `strings.Join` of those, a package-level `const`, or a value carrying an
       explicit `// sql-fragment-allow: <reason>` marker;
     - no `+` concatenation into a query string (the G202 gap);
     - no `%q` on a SQL identifier.
  3. **Close the value-side gaps.** Bind `LIMIT` as a placeholder and clamp every
     caller-supplied limit; escape `\`, `%`, `_` in ILIKE inputs with `ESCAPE '\'`;
     validate `locationRefField`/`ownerField`/`discriminatorColumn` and the embedding
     read-path `colName` at entry.
  4. **Write the invariant down.** New §SQL Safety in `docs/server/conventions.md`,
     a matching section in `server/storage/README.md`, and a pointer from
     `docs/server/architecture.md` §Storage Layer.
- **Pros:** Produces the durable, call-site-local, CI-enforced record #2795 actually
  wanted — just built on a gate that can see this codebase. Zero new dependencies
  (`lib/pq` is already direct; the walker is stdlib `go/ast`). Follows a precedent the
  repo already runs. Catches the next contributor, not just today's code. Fixes two
  real defects on the way (LIKE wildcards, unclamped limits).
- **Cons:** Large. Touches ~81 sites in the most load-bearing package in the server.
  A new custom linter is a thing to maintain, and an over-strict one gets escape-hatched
  into uselessness. The `// sql-fragment-allow` marker is a hatch that must be reviewed
  as carefully as a `//nolint`.
- **Effort:** Large (bounded — see phasing).

### Option C: Adopt a query builder (squirrel / goqu) or sqlc
- **Approach:** Replace hand-assembled SQL with a library that owns placeholder
  numbering and identifier quoting.
- **Pros:** Injection becomes structurally impossible; someone else maintains it.
- **Cons:** `server/storage/` is a proto-reflection-driven layer that *generates its
  own schema* from descriptors at startup — sqlc needs static queries it cannot
  supply, and a runtime builder would have to be threaded through `ProtoSQLStorage`,
  `InstrumentedDB`, `WithTx`, the array-column and embedding subsystems, and the
  pgvector/PostGIS fragments the builders do not model. It is a rewrite of the layer,
  not a hardening of it, and it contradicts CLAUDE.md's "Minimal Dependencies:
  only add the minimal required dependencies; avoid over-engineering."
- **Effort:** Very large. **Rejected.**

---

## Recommendation

**Option B, delivered as a single PR** (per the requester's instruction), staged
internally as ordered commits so the diff can be read in passes.

The reasoning: #2795's diagnosis (unverified assumption in a security-relevant lint
config) is right, and its prescription (per-site suppressions) is wrong for a reason
nobody could have known without running the tool — gosec cannot see through the
`SQLDB`/`SQLExecutor` interfaces that `InstrumentedDB` requires. Deleting the exclude
is correct and costs nothing, but shipping only that would close a security issue by
confirming the security check does nothing. That is worse than the status quo, because
it converts an unverified assumption into a verified absence and then walks away.

The tradeoff I am making: **replacing a third-party linter's coverage with our own is
real maintenance cost, and I am accepting it** because the alternative is no coverage.
The mitigating fact is that the repo already runs three custom Go/JS gates of exactly
this shape (`check-goroutines`, `check_codeinternal.js`, `check_active_community_gate.js`),
so this is an established pattern here rather than a novel one — and `check-sql` has an
easier job than `check-goroutines`, because all the SQL lives in one package.

I am **not** recommending Option C. The proto-reflection design is the reason this
storage layer is small and generic; a query builder would fight it at every turn for a
guarantee Option B gets from an 80-line validated-quote helper.

Three things I would flag to the reviewer as the genuine risks:

- **One PR means the config deletion and the ~81-site migration land together.** The
  cheap, obviously-correct part of this change (deleting a no-op exclude) no longer
  gets to merge on its own, so #2795 stays open until the whole thing is reviewed. That
  is the requester's call and it is a reasonable one — a single PR keeps the config
  comment, the gate it points at, and the code that gate protects consistent at every
  commit, instead of leaving a comment referencing a `check-sql` that does not exist
  yet. The mitigation is commit hygiene, not scope reduction: see the review strategy
  below.

- **The `// sql-fragment-allow` hatch is the whole ballgame.** `notDeletedFilter`,
  `extraWhereSQL`, `orderBySQL` and the `communitySearchConfig` fragments are
  legitimately raw SQL and will need it. If that marker gets used reflexively the gate
  degrades to decoration — the same failure mode the G115 comment in
  `server/.golangci.yml` already describes ("carrying 93 `//nolint` directives forever
  would train everyone to add a 94th by reflex"). Stage 3 must land with a hard cap on
  the number of hatches and each one reviewed individually.
- **LIKE escaping is a user-visible behavior change.** A member who types `%` today
  gets every item in the community; after Stage 4 they get items literally containing
  `%`. I believe that is a bug fix, not a regression, but it is a behavior change and
  should be called out in the PR.

---

## Open Questions

- **Should the exclude be deleted, or should `gosec` G201 be turned off entirely
  given it finds nothing?** — Recommendation: **delete the exclude, keep G201 on.**
  It costs nothing to run and it is the correct check for the one shape it *can* see
  — a future caller reaching for a raw `*sql.DB`, which is exactly the case we most
  want caught. Replace the TODO with a comment recording the two measured blind spots,
  so the next person does not re-derive them.

- **Custom `go/ast` walker (`server/cmd/check-sql`) or a `golang.org/x/tools/go/analysis`
  pass?** — Recommendation: **the walker.** It matches `server/cmd/check-goroutines`
  line for line, needs no new dependency, and the checks are syntactic (does this
  expression match an approved shape?) rather than type-directed. If we later need to
  resolve whether a receiver implements `SQLExecutor`, the analysis framework is the
  right upgrade — but not for v1.

- **Should `check-sql` cover `_test.go` files?** — Recommendation: **no for `_test.go`,
  yes for `testing.go`.** The repo already exempts tests from `gosec`/`errcheck`
  (`server/.golangci.yml` `exclude-rules`), and test queries against a throwaway
  container are not a threat. But `server/storage/testing.go` is a non-test file in a
  production package, it is already linted as production code, and its
  `datname = '%s'` splice is precisely the pattern we do not want copied.

- **Bind `LIMIT` as a placeholder, or keep `%d` and only add clamps?** —
  Recommendation: **bind it.** PostgreSQL accepts `LIMIT $n`; binding removes an entire
  category of expression from the format string, which materially simplifies what
  `check-sql` has to reason about. Clamp as well — binding an unbounded limit is still
  an unbounded query.

- **How should LIKE escaping be applied — helper at the storage boundary, or at each
  call site?** — Recommendation: **one unexported helper in `protosql_search.go`**
  (`escapeLikePattern`) used by all three ILIKE paths, emitting `ESCAPE '\'` in the
  query text. Three call sites is not enough to justify a public API, and a single
  helper is what a test can pin.

- **Does anything depend on `%` behaving as a wildcard in search?** —
  Recommendation: **assume not, verify with a grep of the app's search callers before
  Stage 4 lands.** No documented feature exposes wildcard search, and the client
  search boxes are plain text fields. If something does depend on it, the fix is an
  explicit flag, not an unescaped pattern.

- **Should `QueryByProximityForCommunity`'s `locationRefField`/`ownerField` be
  validated, or replaced with a closed enum?** — Recommendation: **validate in Stage 2,
  and consider the enum in a follow-up.** `validateIdentifier` is a one-line fix that
  closes the hole today; narrowing the signature to a typed field constant is better
  design but changes a public API with callers in `server/services/`, which is a
  separate change with its own review surface.

- **What is the acceptable count of `// sql-fragment-allow` markers?** —
  Recommendation: **name a number in the PR and hold to it — I estimate 6–10**, all in
  `protosql_search.go` (the `communitySearchConfig` fragments) and
  `protosql_schema.go` (DDL type strings). If the migration wants more than ~12, the
  gate's approved-shape list is too narrow and should be widened deliberately rather
  than hatched around.

---

## Implementation Plan

**One PR on branch `sql_security`, closing #2795.** The five stages below are the
commit order, not a merge order. Each stage is a self-contained commit that builds and
passes tests on its own, so the PR can be reviewed commit-by-commit rather than as one
~81-file diff — that is the entire mitigation for the size, and it only works if the
commits stay clean, so no stage may be squashed into another.

**Review strategy for the reviewer:**
1. Read Stage 1 and Stage 5 first (~5 files) — the config comment and the docs state
   the invariant the rest of the PR implements.
2. Read Stage 2's `sqlident.go` + tests, then Stage 3's `check-sql` + tests (~4 files).
   These are the only genuinely new logic in the PR.
3. Treat Stage 2's ~81-site migration as mechanical and verify it by the gate rather
   than by eye — Stage 3 fails the build on any site the migration missed, which is a
   stronger guarantee than reading 18 files. Spend the eye-time instead on the
   `// sql-fragment-allow` hatches, which are listed exhaustively in the PR body.
4. Stage 4 is the only behavior change; its tests pin it.

**Ordering constraint:** Stage 3 (the gate) must be committed *after* Stage 2 (the
migration), or the build is red at that commit. Stage 1's comment references
`server/cmd/check-sql`, which does not exist until Stage 3 — acceptable within a single
PR, where no intermediate commit is ever the merged state, and the reason the config
deletion is not being split out.

### Stage 1: Delete the exclude, record what was measured
**Goal:** Remove the misleading config line and replace it with fact, so the reason the
rest of the PR exists is legible from `server/.golangci.yml`.
**Tasks:**
- [ ] Delete `- G201` from `linters-settings.gosec.excludes` in
      `server/.golangci.yml`; keep `gosec` enabled.
- [ ] Replace the `TODO(#2795)` block with a durable comment stating the two measured
      blind spots (interface-typed executor; inline `fmt.Sprintf`), the measurement
      (`gosec -include=G201,G202 ./server/...` → 0 issues over 705 files, 157,085
      lines), and a pointer to `server/cmd/check-sql` as the gate that actually covers
      the storage layer. No `TODO(#NNNN)` is left behind — the work it would point at
      lands in the same PR.
- [ ] Confirm `npm run lint:go` stays green with the exclude removed (it must — there
      are no findings).
**Validates:** That removing the exclude changes nothing, which is the whole point —
and that the record of *why* now lives in the config instead of an unverified claim.

### Stage 2: The identifier primitive, and the migration to it
**Goal:** Exactly one way to put an identifier into a query, correct by construction.
**Tasks:**
- [ ] Add `server/storage/sqlident.go`: `quoteIdent(string) (string, error)` =
      `validateIdentifier` + `pq.QuoteIdentifier` (present in `lib/pq` v1.12.3,
      already a direct dependency — `go.mod:25`). **Error-returning only; no panicking
      `mustQuoteIdent`.** *(audit fix — see A1)*
- [ ] Validate every descriptor-derived column name **once, at registration/schema
      time** — in `DefaultStorageTypes()` / `protosql_schema.go`'s table-creation path
      — and fail server startup on violation. That is fail-fast where the repo already
      fails fast, and it means the per-query paths handle a `quoteIdent` error that
      can no longer occur rather than panicking inside a request. *(audit fix — A1)*
- [ ] Unit tests: valid names, embedded `"` and `\`, empty string, leading digit,
      63-byte-truncated `sanitizeColumnName` output, and a case pinning that
      `quoteIdent` differs from `%q` on a name containing `"`.
- [ ] **Injection test, not just a helper test:** a storage-package test that drives a
      public entry point (`QueryByField`, `DeleteByField`,
      `QueryByProximityForCommunity`) with a hostile identifier
      (`id"; DROP TABLE "user`, `id) OR 1=1 --`) and asserts it is rejected before any
      query is issued. `docs/reviews/testing_review_prompt.md` §6 asks for injection
      tests specifically; a unit test of `quoteIdent` alone does not prove the callers
      route through it. *(audit fix — A5)*
- [ ] Migrate all ~81 sites across the 18 files. Retire `"%s"`, `%q`, bare `%s`, and
      the hand-rolled `` `"` + f + `"` `` in `embedding.go:741`.
- [ ] Add the missing validations: `locationRefField` / `ownerField`
      (`protosql_spatial.go`), `discriminatorColumn` (`scheduled_notification.go`),
      and the embedding read-path `colName` (`embedding.go` `:225,262,296,313`,
      `protosql_search.go` `:139,142,150`).
- [ ] Quote the `update` SET-clause columns (`protosql.go:439`) to match the WHERE
      clause. Verify against a table with a mixed-case column (`"Story"`,
      `"FeedItemView"`) that nothing depended on the unquoted folding behavior.
- [ ] Run the full server suite locally — unit, `-tags=integration`, and the storage
      package's own testcontainer tests — before committing.
**Validates:** That a single primitive covers every real identifier shape in the layer,
including the pgvector, PostGIS, array-column and DDL paths. If it does not, the gate
in Stage 3 is not yet writable and the primitive needs another constructor.

### Stage 3: `server/cmd/check-sql` and CI wiring
**Goal:** Make the invariant fail the build, not just the review.
**Tasks:**
- [ ] Write `server/cmd/check-sql/main.go` following `server/cmd/check-goroutines`:
      walk `server/`, skip `gen/` and `_test.go`, enforce the four rules from Option B.2.
- [ ] Support `// sql-fragment-allow: <reason>` with a mandatory reason string; reject
      a bare marker. **No tracking-issue link is required, and none should be
      demanded.** *(audit fix — A4.)* The repo's sibling hatches
      (`// go-line-count-allow`, `// dart-line-count-allow`) require an issue link
      because they mark *deferred work*. This one marks a permanently legitimate case —
      `notDeletedFilter`, the `communitySearchConfig` `extraWhereSQL`/`orderBySQL`
      fragments, and the DDL type strings in `protosql_schema.go` are raw SQL forever.
      Requiring an issue link by analogy would create a backlog of issues that can
      never be closed. State this in the gate's own help text so the next reviewer
      does not ask for one.
- [ ] Add `lint:go:sql` to `package.json` and chain it into `lint:go` (which
      `.github/workflows/test_go.yaml` already runs as a required check).
- [ ] Table-driven tests for the walker: each rule's violating and conforming shape,
      the inline-`fmt.Sprintf` case gosec misses, the interface-receiver case gosec
      misses, and a marker with and without a reason.
- [ ] Count the hatches the migration actually needed. If > ~12, widen the
      approved-shape list instead of accepting them.
**Validates:** That the gate is precise enough to be worth having — it must flag both
gosec blind spots on a synthetic case, and produce zero false positives on the
migrated tree.

### Stage 4: Value-side hardening
**Goal:** Close the two behavioral gaps the audit found.
**Tasks:**
- [ ] Bind `LIMIT` via `dbSpec.Placeholder` at all 11 sites.
- [ ] Add a ceiling to caller-**supplied** limits: `StoryStorage.ListByCommunity` /
      `ListByUser` / `ListByItem`. Match the existing convention (50 for search, 200
      for chat) rather than inventing a third number.
- [ ] **Do NOT impose a default cap on `ProtoSQLStorage.ListAll`.** *(audit fix — A2.)*
      An earlier draft of this plan listed it alongside the `Story` methods. That is
      wrong and would have been a quiet data-corruption bug: `ListAll` with
      `Limit == 0` emits no LIMIT clause **by design**, and several of its 13 non-test
      callers depend on getting every row — `server/impact_metrics/dataset.go` builds
      the impact dataset from all communities, transfers and junction rows
      (`:274,288,304,320,336,352`), and `server/services/location/places.go` scans all
      users and all gear (`:199,254`). Silently truncating those would skew impact
      metrics with no error anywhere. Clamp a supplied limit; leave the unlimited
      contract alone. The tension with `docs/server/conventions.md` §SQL Efficiency
      rule 3 ("Always LIMIT unbounded queries") is real but **pre-existing and out of
      scope** — fixing it means paginating those callers, which is its own change.
      File that as a separate issue and link it from the PR.
- [ ] Add `escapeLikePattern` in `protosql_search.go`; apply in `executeTextSearch`,
      `SearchCommunityUsersByName`, `SearchProvisionalUsersByName`; emit
      `ESCAPE '\'`.
- [ ] Tests: a search for `%` returns only rows literally containing `%`; `_` likewise;
      a trailing `\` does not produce a malformed pattern.
- [ ] Grep the app's search callers to confirm nothing relied on wildcard behavior;
      call the change out explicitly in the PR body.
**Validates:** That escaping does not break ordinary substring search — the existing
search tests must stay green — while wildcard input stops matching everything.

### Stage 5: Documentation
**Goal:** Give the invariant a written home, so the gate is explicable rather than
mysterious.
**Tasks:**
- [ ] New §SQL Safety in `docs/server/conventions.md`, adjacent to §SQL Efficiency:
      values are always bound; identifiers always go through `quoteIdent`; raw
      fragments require a reviewed `// sql-fragment-allow`; `check-sql` enforces it.
- [ ] Matching section in `server/storage/README.md` under Key concepts.
- [ ] Pointer from `docs/server/architecture.md` §Storage Layer → Storage Best
      Practices (which today lists three rules, none about safety).
- [ ] **`docs/server/semantic_search.md`** *(audit fix — A3.)* Its `globs` cover both
      `server/storage/embedding.go` and `server/storage/protosql_search.go`, the two
      files Stages 2 and 4 change most, and the first draft of this plan omitted it.
      Two edits: (a) §Search Execution's "Text Fallback" must describe the escaped
      pattern and `ESCAPE '\'`; (b) the example on line 160 already shows
      `LOWER(name) LIKE LOWER($pattern)`, which is **wrong twice over** — the code has
      used `ILIKE` since before this issue (`protosql_search.go:208`), and
      `docs/server/conventions.md` §SQL Efficiency rule 7 explicitly forbids the
      `LOWER(...) LIKE LOWER(...)` form because it defeats the index. Fix the example
      while the doc is open.
- [ ] Update the `freshness:` stamps on the touched docs and run
      `npm run generate:context-map`, then `npm run lint:context-map`.
- [ ] Note in `docs/reviews/security_review_prompt.md` §2 that the storage-layer
      injection sweep is now gated by `check-sql`, so the review can focus on the
      hatches rather than re-reading every call.
**Validates:** That a contributor who has never seen this layer can read one section
and know what the gate will reject before they hit CI.

### Ship: whole-PR verification
**Goal:** Everything green on one branch before the PR opens — no stage validated only
in isolation.
**Tasks:**
- [ ] `npm run lint` (full) — includes `lint:go` with the new `lint:go:sql`,
      `lint:go:size` (Stage 2 must not push any `server/storage/` file past 1,000
      lines), and `lint:context-map` for Stage 5's doc edits.
- [ ] `go test ./server/...` and `go test -tags=integration ./server/...` — CI runs the
      integration tag, so a local run without it is not a verification.
- [ ] `go test ./server/storage/` specifically, against the testcontainer database —
      the identifier migration and the `LIMIT`/ILIKE changes are only exercised there.
- [ ] `gosec -include=G201,G202 ./server/...` still reports 0, confirming Stage 1 did
      not silently start producing findings that the PR then ignores.
- [ ] `go run ./server/cmd/check-sql` on a deliberately-broken scratch file, to confirm
      the gate fails rather than passes vacuously (a gate that only ever passes is
      indistinguishable from no gate).
- [ ] PR body lists every `// sql-fragment-allow` hatch with its reason, and states the
      LIKE-escaping behavior change explicitly.
**Validates:** That the PR is green on the checks CI actually runs, and that the new
gate demonstrably rejects something.

### Explicitly out of scope (stated so the PR is not asked to grow)

- **`instrumented_db.go` is untouched.** It is governed by `docs/server/profiling.md`.
  Binding `LIMIT` does not change `queryKind()`'s leading-keyword parse, per-request
  query counts, or any `AssertMaxQueries` assertion — the query count is identical,
  only the argument list grows. No profiling doc needs updating.
- **The `SQLDB`/`SQLExecutor` interfaces stay.** "Fixing" gosec's blind spot by
  reverting to a concrete `*sql.DB` would break `InstrumentedDB` query counting (#1613
  metrics, N+1 gates) and `WithTx` routing. The gate goes to the code, not the code to
  the gate.
- **Unbounded `ListAll`** — separate issue (see Stage 4).
- **Narrowing `QueryByProximityForCommunity`'s field parameters to a typed enum** —
  better design, but it changes a public API with `server/services/` callers. Stage 2
  validates the strings; the signature change is a follow-up.
- **`server/cmd/check-sql` covering `_test.go`** — the repo already exempts tests from
  `gosec`/`errcheck`; `server/storage/testing.go` is covered because it is not a test
  file.

**File-size headroom** (CI gate at 1,000 lines, `npm run lint:go:size`): `protosql.go`
778, `community.go` 761, `protosql_search.go` 725. The migration substitutes calls for
format strings and should be roughly length-neutral, but Stage 4's `escapeLikePattern`
lands in `protosql_search.go` — if it pushes past the gate, split it into a sibling
file per `docs/server/architecture.md` §3 rather than adding the `// go-line-count-allow`
hatch.

---

## Related Issues

- **#1611** — Go lint enforcement (closed; shipped as PR
  [#2817](https://github.com/Type2Fun-Foundation/ripls/pull/2817), merged 2026-07-30).
  Enabled `gosec` and `nolintlint`. Its `allow-unused: false` setting is precisely why
  #2795's prescribed per-site suppressions cannot be written.
- **#2816** — Ratchet `funlen`, `cyclop`, `dupl`, `goconst` (open). Sibling
  lint-hardening work parked out of #1611. `server/storage/` files appear in its
  backlog; Stage 2's migration should avoid growing any of them past the 1,000-line
  gate.
- **#1613** — Log-based SLI/SLO metrics. `InstrumentedDB`'s query counting — the reason
  `ProtoSQLStorage` holds interfaces rather than `*sql.DB`, and therefore the root
  cause of gosec's blind spot — feeds its `db_slow_query_duration` metric. Any attempt
  to "fix" the blind spot by removing the interface would break this.

---

## Audit Notes (`/ripls-audit`, 2026-07-30)

Audited against the documentation context map. **Verdict: compliant after five fixes,
which are applied in-place above and marked `(audit fix — An)`.**

**Scope determined:** server-only (`server/storage/**`, `server/cmd/check-sql`,
`server/.golangci.yml`, `package.json`, docs). No client, proto, or infra surface.
**Engaged dimensions:** security (primary), code_quality, testing, performance,
error_handling, tech_debt, dependency. Not engaged: accessibility, api_design (no proto
change), observability (see A6 — considered and found unaffected).

### Findings and fixes applied

- **A1 — `mustQuoteIdent` panicking at query time (error_handling, code_quality).**
  The draft had descriptor-derived names go through a panicking helper.
  `docs/reviews/code_quality_review_prompt.md` §2 flags "`panic`/`recover` used as
  control flow rather than for genuinely unrecoverable states," and
  `docs/server/architecture.md` §Goroutine Safety treats panics as a process-level
  hazard the middleware cannot always contain. **Fixed:** validate descriptor-derived
  column names once at registration/schema-creation time and fail server startup —
  fail-fast where the repo already fails fast — leaving only an error-returning
  `quoteIdent` on the request path.

- **A2 — capping `ListAll` would have been a silent data bug (performance, correctness).
  The most serious finding.** The draft listed `ProtoSQLStorage.ListAll` among the
  methods to clamp. `ListAll` with `Limit == 0` deliberately emits no LIMIT clause, and
  of its 13 non-test callers, `server/impact_metrics/dataset.go` (6 calls) and
  `server/services/location/places.go` (2 calls) require every row. A default cap would
  have skewed impact metrics with no error raised anywhere.
  `docs/reviews/performance_review_prompt.md` §1 does ask about unbounded queries — but
  the fix there is pagination of the callers, not truncation under them. **Fixed:**
  clamp supplied limits only; `ListAll`'s contract is explicitly preserved; the
  conventions rule-3 tension is recorded as pre-existing and deferred to its own issue.

- **A3 — `docs/server/semantic_search.md` was missing from the doc updates
  (context-map compliance).** Its `globs` cover `server/storage/embedding.go` and
  `server/storage/protosql_search.go` — the two files Stages 2 and 4 change most — so
  the map selects it and Stage 5 omitted it. **Fixed:** added, with a second finding
  attached: its §Search Execution example (line 160) shows
  `LOWER(name) LIKE LOWER($pattern)`, which contradicts the code (`ILIKE`,
  `protosql_search.go:208`) *and* `docs/server/conventions.md` §SQL Efficiency rule 7,
  which forbids that form outright. Pre-existing drift, fixed while the doc is open.

- **A4 — the `// sql-fragment-allow` hatch must not require an issue link
  (tech_debt / TODO hygiene).** The repo's sibling hatches (`go-line-count-allow`,
  `dart-line-count-allow`) require a tracking issue because they mark deferred work.
  This one marks permanently legitimate raw SQL. Requiring a link by analogy would
  manufacture unclosable issues. **Fixed:** stated explicitly, to be repeated in the
  gate's help text.

- **A5 — helper unit tests alone would not prove the invariant (testing).**
  `docs/reviews/testing_review_prompt.md` §6 asks for injection tests specifically, and
  §4 warns against assertions that only prove "no error." Testing `quoteIdent` in
  isolation does not show that callers route through it. **Fixed:** added a
  storage-level test driving hostile identifiers through public entry points.

- **A6 — observability: considered, unaffected, and turned into supporting rationale.**
  `instrumented_db.go` is governed by `docs/server/profiling.md`; binding `LIMIT`
  leaves `queryKind()`, per-request counts and `AssertMaxQueries` identical. But
  `recordQuery` logs 200 characters of **query text** at WARN — which is safe today
  only because every value is bound. Added as §D of the audit findings and as a
  required line in the new §SQL Safety doc.

### Confirmed compliant (no change needed)

- **Minimal Dependencies (CLAUDE.md).** Zero new dependencies: `lib/pq` is already
  direct (`go.mod:25`) and `pq.QuoteIdentifier` verified present in v1.12.3
  (`quote.go:19`); the gate is stdlib `go/ast`. Option C (query builder / sqlc) was
  evaluated and rejected on exactly this rule.
- **Custom-gate precedent.** `server/cmd/check-sql` mirrors `server/cmd/check-goroutines`
  (166 lines, already in `npm run lint:go`), alongside `check_codeinternal.js` and
  `check_active_community_gate.js`. Not a novel pattern here.
- **File-size gate (#1424).** Headroom checked and recorded; splitting is prescribed
  over the `// go-line-count-allow` hatch.
- **TODO hygiene (CLAUDE.md).** No `TODO(#NNNN)` is left behind — the work a TODO would
  point at ships in the same PR.
- **Testing strategy (`docs/server/architecture.md` §7).** Unit tests preferred and
  present; the storage-layer tests run against the existing testcontainer harness;
  `-tags=integration` is in the Ship checklist because CI always uses it.
- **Deletion / cascade docs not engaged.** `deleting_things.md` and
  `community_delete_and_leave.md` glob `cascade_delete.go` / `cascade_leaver.go`, which
  contain no dynamic-SQL construction sites and are untouched.

### Docs consulted

`docs/llms.txt` · `docs/server/architecture.md` (⚓) · `docs/server/conventions.md` (⚓) ·
`docs/reviews/security_review_prompt.md` · `docs/reviews/code_quality_review_prompt.md` ·
`docs/reviews/testing_review_prompt.md` · `docs/reviews/performance_review_prompt.md` ·
`docs/reviews/error_handling_review_prompt.md` · `docs/server/semantic_search.md` ·
`docs/server/profiling.md` (index entry) · `server/storage/README.md` · CLAUDE.md.
Not loaded: `docs/proto_conventions.md` (no proto surface), client and accessibility
docs (no `app/lib/**` surface), `deleting_things.md` / `community_delete_and_leave.md`
(globbed files untouched).

---

## Implementation Record (2026-07-30, branch `sql_security`)

Shipped as one PR, five commits in the planned stage order. Deviations from the
plan and things found on the way, so the plan and the code do not drift:

### Found during implementation (not predicted by the plan)

- **A live bug in the stock_image embedding backfill.** The startup validation
  added in Stage 2 refused to start against `DefaultStorageTypes()`, which was
  the correct answer to a real defect: `EmbeddingFieldConfig.Fields` holds proto
  field *paths* (dotted for nested messages —
  `provider_image.description`), and `extractEmbeddingText` walks them with
  protoreflect, but `getEmbeddingTextByID` quoted the dotted path straight into
  a `SELECT`. The column has never existed — `flattenFields` stores that field
  as `provider_image_description`. `BackfillEmbeddings` swallows per-record
  errors with a warn-and-continue, so `stock_image` embeddings had simply never
  backfilled. Fixed with `embeddingTextColumn`, plus a property test asserting
  that every configured embedding field maps to a column the schema creates.
- **A soundness bug in the first cut of `check-sql`.** Treating any assignment
  as authoritative let `if limit <= 0 { limit = defaultLimit }` launder a
  parameter. Fixed so only `:=` sets safety while `=`/`+=` can weaken it; the
  fix immediately surfaced a site (`chat_message.go`) the first pass had passed.
- **Two hand-rolled reimplementations of `strings.Join`** (`joinSep` in
  `activity_digest.go`, `join` in `feed.go` — the latter round-tripping through
  `[]byte` for no reason). Deleted rather than taught to the checker.
- **`ESCAPE` takes exactly one character.** `ESCAPE '\\'` inside a Go *raw*
  string literal sends two backslashes and PostgreSQL rejects it with
  `invalid escape string (22025)`. Caught by the DB-backed test, not by review.
  The clause now lives in one `likeEscapeClause` constant so the three ILIKE
  sites cannot disagree.

### Deviations from the plan

- **Freshness stamps were not updated.** The plan's Stage 5 said to advance
  them; `docs/context_map.md` says they are written by the
  `ripls-doc-freshness` skill and never by hand, because a hand-edited stamp
  rubber-stamps unverified prose. Left for the nightly ratchet.
- **`quotedTableFor` was added beyond the planned primitives.** The
  lookup-then-quote pair repeated ~20 times; factoring it is what the
  code_quality rubric asks for, and it keeps the closed-set lookup and the
  quoting together.
- **`LIMIT` was bound at const-valued sites too**, not just caller-supplied
  ones, so the rule states as "no substitution except identifiers".

### Hatches used: 5

All predicted (the plan estimated 6–10, capped at 12):

| Site | Reason |
|---|---|
| `protosql_search.go` ×5 | `communitySearchConfig.extraWhereSQL` / `orderBySQL` / `limitSQL` — package-level literals spliced as raw WHERE/ORDER BY fragments |
| `protosql_schema.go` ×3 | `DatabaseSpecifics.SQLType` / `BinaryColumnType` dialect keywords in DDL |

(The `protosql_schema.go` count is 3 markers covering 2 distinct expressions
plus the ALTER builder.) The `communitySearchConfig` type doc now carries the
invariant the markers assert.

### Verification

- `npm run lint:go` (includes the new `lint:go:sql`) — pass
- `npm run lint:go:size` — pass; largest storage file 896 lines
- `npm run lint:context-map` — pass
- `go test ./server/...` — pass
- `go test ./server/storage/` against testcontainers — pass, including the new
  DB-backed LIKE-wildcard and LIMIT-binding tests
- `go test ./server/cmd/check-sql/` — 12 tests, including one per gosec blind
  spot
- `gosec -include=G201,G202 ./server/...` — still 0, confirming Stage 1 did not
  start producing findings the PR then ignores
- `check-sql` run against a deliberately-broken file — fails as intended, so
  the gate is not passing vacuously
