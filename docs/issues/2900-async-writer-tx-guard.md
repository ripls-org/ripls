# Issue #2900: Async request writers clobber concurrent state changes (cancel silently reverts)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2900
**Filed:** 2026-08-13 | **Last activity:** 2026-08-14
**Labels:** bug | **Assignees:** unassigned
**Context docs:** docs/server/architecture.md (⚓, goroutine safety), docs/server/conventions.md (⚓, fail-fast + testing), docs/proto_conventions.md (⚓), docs/reviews/error_handling_review_prompt.md, docs/reviews/testing_review_prompt.md

## Problem Statement

`SubmitRequest` spawns two detached goroutines that each perform a
read-modify-write of the **entire** `models.Request` proto:

1. `fetchAndAttachStockImage` (`server/services/request/stock_imagery.go:79-91`) —
   `GetByID` → set `MediaIds` → `UpdateIfNotDeleted`.
2. The suggestion-chips goroutine (`server/services/request/lifecycle.go:207-231`)
   — AI call → `GetByID` (with `IncludeDeleted`) → set `AdditionalAsks` /
   `BreakdownPieces` / `OfferIdeas` → `UpdateIfNotDeleted`.

Because `ProtoSQLStorage.update` (`server/storage/protosql.go:377-519`) writes
the full `binary_proto` on every UPDATE, each goroutine persists its entire
stale snapshot — not just the columns it means to touch. `UpdateIfNotDeleted`
adds only `AND (deleted_deleted_at_unix_sec = 0 OR … IS NULL)` to the WHERE
clause (`protosql.go:472-478`); it does not detect that any other field
changed under the reader.

So this interleaving silently reverts a cancel (or any other Request field
edit landed in the async writer's window — description, media, needed-by,
social context, state transitions from `MarkRequestFulfilled` / accepted
offers):

| t | actor | effect |
|---|---|---|
| 1 | chips goroutine | `GetByID` → snapshot with `State = ACTIVE` |
| 2 | `CancelRequest` | writes `State = CANCELLED` |
| 3 | chips goroutine | `UpdateIfNotDeleted(snapshot)` → `State = ACTIVE` again |

The window is the length of an AI call (`GenerateRequestSuggestions`, up to
the detached 2-minute timeout) or the stock-image fetch + GCS copy. No error
is raised on the losing write — `applied` is true, and the log line says
`persisted async request suggestions`. This surfaced through a CI flake on
`TestService_CancelRequest` at
`server/services/request/lifecycle_cancel_update_test.go:51` (Go Tests run
31656284013, PR #2899), but the underlying bug reverts real user actions in
production whenever a helper cancels or edits a fresh request within the
async writer's window.

Sweep candidates for the same shape: the issue explicitly calls out
gear/experience. `server/services/experience/save.go:141-243` runs its LLM
enrichers on a `sync.WaitGroup` **before** the RPC returns, so no async
window opens; `server/services/gear/gen_fanout.go` similarly runs stock
imagery inside the RPC via the streaming fan-out. Neither service spawns
a detached goroutine that re-reads and updates a live entity — the request
service is the only writer with this shape today.

## Context from Comments

The only comment is `@claude` from maintainer triaging the bug to us.
The issue body carries the entire analysis: names both call sites with line
numbers, walks the interleaving that trips the CI failure, calls out the
2-minute AI timeout as the worst-case race window, notes that the log line
says success on the losing write, and enumerates three options with a
recommendation for **Option 2 — re-read inside a transaction and merge**.
The "Done when" checklist also asks for a deterministic regression test
(not a timing-based flake fix) and a sweep of gear/experience for the same
shape.

## Potential Solutions

### Option A: Wrap each async writer's read-modify-write in `WithTx` with a locked reload

- **Approach:** Introduce a `QueryOptions{ForUpdate: bool}` variant of
  `GetByID` in `server/storage/protosql.go` that appends `FOR UPDATE` to
  the `SELECT binary_proto` when set (only honored inside a
  transaction — asserted in the code path, since `SELECT FOR UPDATE`
  outside a tx acquires a lock that's released immediately and would
  silently do nothing useful). Rewrite both async writers to:
    1. `s.storage.WithTx(asyncCtx, nil, func(tx *storage.ProtoSQLStorage) error { … })`
    2. Inside the tx: `tx.GetByID(asyncCtx, requestID, fresh, storage.QueryOptions{IncludeDeleted: true, ForUpdate: true})`
    3. Guard `fresh.Deleted`; return early (no-op) if soft-deleted.
    4. Copy only the fields this goroutine owns from the pre-tx buffer
       onto `fresh` (stock imagery: `MediaIds`; chips: `AdditionalAsks`,
       `BreakdownPieces`, `OfferIdeas`).
    5. `tx.Update(asyncCtx, fresh)`. Plain `Update` is fine — the row is
       locked and the `Deleted` check inside the tx already fired.
  The lock held by `SELECT FOR UPDATE` blocks any concurrent
  `CancelRequest` / `UpdateRequest` / `MarkRequestFulfilled` / offer-side
  state flip from committing until the async write commits. Anything that
  landed before the lock is visible on the reload and preserved on the
  merged write.
- **Pros:** Smallest correct fix that keeps `binary_proto` as the source
  of truth; reuses `WithTx` (documented, panic-safe, already used by
  `server/jobs/community_purge.go:239-369` for exactly this
  "re-check-under-lock" pattern); adds one narrow `QueryOptions` field
  behind an assertion; the merge lives in the async writer that knows
  which fields it owns; the community-active pre-checks and error paths
  don't change.
- **Cons:** Adds one round-trip (the reload) inside each async writer;
  `CancelRequest` / friends now wait up to the async-writer commit time,
  which is dominated by driver latency and is bounded (the AI call
  happens *before* the tx opens); a new storage primitive (`ForUpdate`)
  must be documented and its "tx-only" invariant enforced. `WithTx`
  does not retry on serialization failures — none should fire under
  `READ COMMITTED` with row-level locking, so no retry loop is needed.
- **Effort:** small — two call-site rewrites, one storage flag, a
  deterministic regression test.

### Option B: Partial (field-scoped) UPDATE that only sets the columns the writer owns

- **Approach:** Add a `PatchFields(ctx, msg, fieldPaths []string)` API to
  `ProtoSQLStorage` that emits `UPDATE <table> SET <col> = $1 [, …]
  WHERE id = $N` for the named proto fields only. Skip `binary_proto`
  entirely — or re-serialize it from a fresh reload done inside the same
  UPDATE round-trip via a returning read. Rewrite both async writers to
  call `PatchFields` with the fields they own.
- **Pros:** No stale-snapshot problem by construction — the goroutine
  writes only what it means to write, and other writers' concurrent
  changes are physically un-touched. No new locking behavior for callers
  to reason about. Structurally the "most correct" fix in the issue's
  taxonomy.
- **Cons:** Large surface-area addition to `protosql.go` (partial-column
  marshalling, keeping the flattened columns and `binary_proto` in sync
  when `binary_proto` is bypassed, wiring array-column denormalizations
  and embedding regeneration to know which fields moved). The
  denormalization code (`hasArrayColumns`, `syncArrayColumns`) assumes
  the whole row was rewritten. Fixing that is a bigger project than
  what #2900 is actually asking for, and rolls a general partial-update
  feature that no other caller currently needs.
- **Effort:** large.

### Option C: Optimistic concurrency (version / updated_at column) with retry

- **Approach:** Add a monotonic `version_int64` (or `updated_at_unix_ms`)
  column to every proto-storage table. Every `Update` bumps it and
  gates on `WHERE id = $N AND version = $prior_version`. On mismatch,
  return a typed `ErrConflict`. Async writers wrap their
  read-modify-write in a small retry loop (bounded, e.g., 3 attempts):
  reload → merge → try to write; on `ErrConflict`, reload and try
  again.
- **Pros:** Fixes the shape everywhere at once — anywhere else in the
  server that does read-modify-write concurrently benefits automatically.
  No new locking; readers stay lock-free.
- **Cons:** Widest blast radius — every table gains a column and every
  `Update` path in `ProtoSQLStorage` grows a version check; every
  caller that does read-modify-write today has to be audited for how
  it wants to handle `ErrConflict`; retry loops add their own bugs
  (stack overflow on infinite conflict, request-scoped
  retries corrupting response bodies, log noise). This is the right
  investment if we discover many more races, but not for a two-writer
  bug we can prove is bounded.
- **Effort:** large.

## Recommendation

**Option A.** The issue explicitly recommends it as "the smallest correct
fix for these two call sites and reuses machinery that already exists,"
and the codebase agrees: `WithTx` is battle-tested and its "re-check
under lock" pattern is already load-bearing for
`server/jobs/community_purge.go` (Safeguard #5 — the community-purge
orchestrator's exact rationale for using this pattern is that a concurrent
`RestoreCommunity` could slip in between candidate selection and cascade;
same shape as us).

The only new storage primitive is `QueryOptions{ForUpdate: bool}` — a
narrow, well-defined addition that pairs with `IncludeDeleted` in the
existing struct (`server/storage/protosql_types.go:82-87`) and enforces
its "tx-only" contract via an early-return assertion (`inTransaction()`
already exists at `server/storage/protosql.go:406`).

Option B pays for a general partial-update API we don't otherwise need
today, and Option C is the right investment only if we accumulate a
third and fourth writer of this shape. If either of those becomes true
in a follow-up, Option A does not preclude either — a future
`PatchFields` or version column can wrap the same call sites without
un-doing this fix.

## Open Questions

- **Should the locked reload use `IncludeDeleted: true` and then skip on
  `fresh.Deleted != nil`, or `IncludeDeleted: false` and treat "not found"
  as a benign skip?** — Recommendation: `IncludeDeleted: true` + explicit
  `Deleted != nil` skip. The chips goroutine already reads that way
  today (`lifecycle.go:208`) so behavior stays identical, and it lets us
  log "skipping async writer — request was deleted" as an info line
  instead of losing that signal in an `ErrRecordNotFound` return path.
  The `UpdateIfNotDeleted` guard is redundant once the reload happens
  under lock, so it collapses back to plain `Update`.

- **Do we need a retry loop on serialization failure?** — Recommendation:
  no. `WithTx` opens with `nil` opts (READ COMMITTED per
  `server/storage/tx.go:19-20`). Row-level locks via `SELECT FOR UPDATE`
  under READ COMMITTED serialize writers on the same row without ever
  raising `serialization_failure` (40001); waiters block, then read
  the fresh committed state. Anywhere that would need a retry (e.g.
  SERIALIZABLE) is out of scope for this fix.

- **Should the community pre-check** (`community.IsActive`,
  `lifecycle.go:218`; `stock_imagery.go:54`) **move inside the tx?**
  — Recommendation: yes, keep it inside the tx but keep it *before* the
  locked reload so we don't hold a row lock across another storage call
  we might exit early from. That preserves the "gate before side effect"
  ordering that the existing comments call out, and it means a
  concurrent community soft-delete still short-circuits the writer
  cleanly.

- **How do we make the regression test deterministic?** —
  Recommendation: wire a **before-write hook** on `stockImageryDone` /
  a new `suggestionChipsBeforeWrite` channel (mirroring the existing
  `stockImageryDone` pattern at `service.go:34, 102`). The test signals
  the goroutine to pause after the "read/prepare" step, fires
  `CancelRequest` on the same request from the test goroutine, then
  releases the goroutine. Without the fix the request re-emerges
  `ACTIVE`; with it the request stays `CANCELLED`. This does not
  require sleeps or `-count=N`.

- **Should we sweep gear/experience anyway?** — Recommendation: no
  additional fix now — Phase 3 grep confirms neither service spawns a
  detached goroutine that re-reads and updates a Gear or Experience.
  Add a one-line comment in each service's doc.go pointing at this
  issue and the tx-guarded pattern so future async writers land the
  right way from the start.

- **Do the array-column and embedding side effects still fire
  correctly under `WithTx`?** — Recommendation: yes; verify in a
  test. `ProtoSQLStorage.update` already handles the in-tx path:
  when `s.inTransaction()`, it routes `syncArrayColumns` through
  `s.exec` (the tx), and `queueEmbeddingGeneration` fires via the
  after-commit hook mechanism landed for #2847
  (`server/storage/tx.go:73-75`). So array denormalization commits
  atomically with the row, and the embedding regen only fires after
  a successful commit — same semantics as today. The Phase 3 test
  should assert one embedding regenerates for the Request per
  successful async writer, and none regenerates when the writer's
  tx aborts (e.g. row already soft-deleted at reload time).

## Implementation Plan

### Phase 1: Add `ForUpdate` storage primitive

**Goal:** Give `GetByID` a way to acquire a row lock inside a
transaction, so async writers can build the merge on top of it.

**Tasks:**
- [ ] Add `ForUpdate bool` to `QueryOptions`
      (`server/storage/protosql_types.go:82-87`) with a doc comment
      that it MUST be used inside `WithTx`.
- [ ] In `ProtoSQLStorage.GetByID`
      (`server/storage/protosql.go:119-157`), when `opt.ForUpdate` is
      set: (a) return a typed error if `!s.inTransaction()`, and
      (b) append the fixed string ` FOR UPDATE` after the existing
      WHERE clause. Match the placement style of `buildDeletedFilter`.
      The suffix is a constant, no bound values — but `check-sql`
      (`server/cmd/check-sql`, see `docs/server/conventions.md` →
      "SQL Safety" #5) flags every raw fragment: annotate the append
      with `// sql-fragment-allow: FOR UPDATE clause is a fixed
      constant, no caller input`.
- [ ] Unit test in `server/storage/protosql_query_ops_test.go`: (i)
      `ForUpdate` outside a tx returns the typed error without
      touching the DB; (ii) two parallel `WithTx` callers on the same
      row serialize — the second one waits for the first to commit
      and reads the fresh committed value; (iii) `ForUpdate: true,
      IncludeDeleted: true` composes correctly. Use
      `storage.SetupTestStorage(t)` per the testing convention
      (`docs/server/conventions.md` → "Server Testing"); no sleeps or
      `-count=N` for the serialization test.
- [ ] Extend the docstring on `WithTx`
      (`server/storage/tx.go:12-34`) with a "locked reload"
      subsection pointing at the new `ForUpdate` and the
      community-purge Safeguard #5 as a live example.

**Validates:** the primitive works, honors its tx-only contract, and
composes with `IncludeDeleted`.

### Phase 2: Rewrite the two async writers to reload under lock

**Goal:** Close the race for both call sites.

**Tasks:**
- [ ] `server/services/request/stock_imagery.go:79-103` — replace the
      final `GetByID` + `UpdateIfNotDeleted` block with a
      `s.storage.WithTx(ctx, nil, …)` that (a) reloads with
      `QueryOptions{IncludeDeleted: true, ForUpdate: true}`, (b)
      skips on `fresh.Deleted != nil` (logs the existing
      "request_deleted_during_job" line), (c) sets `fresh.MediaIds =
      []string{mediaID}`, (d) writes with plain `tx.Update`. Delete
      the now-obsolete `UpdateIfNotDeleted` comment block.
      Keep the existing best-effort pattern on the tx return: log at
      `Warn` with `operation`, `target_request_id`, `duration_ms`,
      `error` (per `docs/server/conventions.md` → "Structured Logging"
      / "Standard field names" — note the `target_request_id` (not
      `request_id`) convention). This is not a Fail-Fast violation —
      the writer is a documented best-effort enricher whose failure
      is safely absorbed by later user actions, mirroring the
      existing warn-on-`UpdateIfNotDeleted`-error branch.
- [ ] `server/services/request/lifecycle.go:203-241` — same shape:
      keep the pre-tx `IncludeDeleted` early-out (`stored.Deleted`
      check at :212) as a cheap escape hatch that avoids opening a tx
      when the request is already gone, but replace the final
      `UpdateIfNotDeleted` with a `WithTx` that (a) re-reloads with
      `ForUpdate`, (b) re-checks `fresh.Deleted`, (c) copies just
      `AdditionalAsks` / `BreakdownPieces` / `OfferIdeas` onto
      `fresh`, (d) writes with plain `tx.Update`. Keep the
      `community.IsActive` gate before the tx. Do not change the
      surrounding `go func` / inline-recover shape — it already
      passes `server/cmd/check-goroutines` (see
      `docs/server/architecture.md` → "Goroutine Safety"); moving
      to `logging.GoSafe` is out of scope here.
- [ ] Add a `suggestionChipsBeforeWrite chan struct{}` (buffered,
      nil in production) alongside `stockImageryDone` on
      `server/services/request/service.go:22-35`, plumb it through
      `NewForTesting` (which today returns
      `(*Service, chan, chan)` — the plan expands this to return
      the two additional signal channels rather than mutating a
      shared config struct, matching the existing convention). Send
      on the channel after the pre-tx early-outs and before entering
      `WithTx`. Do the same for the stock-imagery goroutine
      (`stockImageryBeforeWrite`). This is the deterministic hook
      the regression tests rely on. Production callers keep using
      `New` / `NewWithNotificationSignal` (unchanged), which leave
      the new channels nil — the send is guarded by a nil check
      exactly the way `stockImageryDone` is today.
- [ ] Update the existing docs on `UpdateIfNotDeleted`
      (`server/storage/protosql.go:383-397`) to note that it only
      guards soft-delete, and point at this pattern for callers that
      also need to guard concurrent field changes.

**Validates:** both call sites now read + merge + write atomically
against any concurrent Request mutation.

### Phase 3: Deterministic regression tests + sweep

**Goal:** Prove the race is closed at both sites and confirm no other
service has the same shape today.

**Tasks:**
- [ ] Add
      `TestService_CancelRequest_RaceWithSuggestionChips` and
      `TestService_CancelRequest_RaceWithStockImagery` under
      `server/services/request/`. Each: start `SubmitRequest`, wait on
      the new `*BeforeWrite` channel, invoke `CancelRequest` in the
      same goroutine (it blocks on the row lock until the async
      writer's tx commits), then release the writer. Assert (a) the
      Request's final `State` is `CANCELLED`, (b) the async writer's
      owned field made it (`AdditionalAsks` populated, or `MediaIds`
      set), (c) no `binary_proto` field the writer doesn't own moved
      away from the post-cancel value. Add a parallel
      `TestService_UpdateRequest_RaceWithSuggestionChips` for the
      description/title revert path called out in the issue.
- [ ] Add a `TestService_DeleteRequest_RaceWithSuggestionChips` to
      confirm the existing "skip on Deleted" branch still fires
      (should be green pre- and post-fix — regression guard for the
      soft-delete path we're preserving).
- [ ] Run `go test -tags=integration -timeout 20m
      ./server/services/request/... ./server/storage/...` and confirm
      green. Iterate the deterministic tests with `-race -count=10` to
      confirm no residual flake.
- [ ] Grep sweep to confirm no other detached-goroutine writer of the
      same shape exists today:
      `rg -n 'go func|logging\.GoSafe' server/services | rg -v _test.go`
      and inspect each hit. Add a one-line note in
      `server/services/request/doc.go` describing the tx-guarded
      pattern so future async writers land the right way.

**Validates:** the fix holds under a deterministic interleaving, the
soft-delete path still short-circuits, and there is no other
undiscovered instance of the bug.

## Related Issues

- #2899 — the PR that surfaced the flake in CI (run 31656284013).
- #2847 — sibling race: an async write races the transaction that
  spawned it. Different root cause (Insert-in-tx spawns a background
  writer whose commit hasn't landed), same theme — "detached writer
  interleaving with concurrent state." The after-commit-hook mechanism
  that lands there is orthogonal to this fix.
- #1620 / #1623 — the community-purge safeguards whose "re-check under
  lock" pattern (Safeguard #5) is the direct precedent for the WithTx
  + `ForUpdate` recipe used here.
- #2020 — proto-deprecation TODOs; unrelated code, mentioned only
  because it's the canonical example of `TODO(#NNNN):` format we
  should use for any deferred TODOs the fix leaves behind.

## Audit notes

Applied a `ripls-audit` pass against the loaded standards. Adjustments made:

- Called out the `check-sql` (`server/cmd/check-sql`) requirement:
  the ` FOR UPDATE` suffix needs a `// sql-fragment-allow:` marker
  with a mandatory reason (per `docs/server/conventions.md` → "SQL
  Safety" #5), since it is a raw fragment even though it carries no
  caller input.
- Anchored the async-writer error-return branch in the existing
  best-effort convention (`docs/server/conventions.md` → "Fail
  Fast") and listed the required log fields (`operation`,
  `target_request_id`, `duration_ms`, `error`) with the
  `target_request_id` (not `request_id`) convention explicit — the
  latter is reserved for the HTTP correlation key.
- Called out that `NewForTesting` already returns three values and
  the plan expands that tuple rather than reshuffling into a config
  struct, matching the existing convention.
- Preserved the current `go func` + inline-recover shape at the
  call sites (already passes `check-goroutines`); moving to
  `logging.GoSafe` is a separate refactor and out of scope.
- Added a new Open Question confirming that `syncArrayColumns` and
  `queueEmbeddingGeneration` still fire correctly on the in-tx
  path — the existing after-commit-hook mechanism landed for #2847
  (`server/storage/tx.go:73-75`) handles the embedding side
  effect.

**Docs consulted:**

- `docs/server/architecture.md` (⚓ — Service Independence, File
  Organization, Goroutine Safety)
- `docs/server/conventions.md` (⚓ — Fail Fast, CodeInternal, SQL
  Safety, Structured Logging / Standard field names, Server Testing)
- `docs/proto_conventions.md` (⚓)
- `docs/reviews/error_handling_review_prompt.md` (Areas to Review §1,
  §4 — Go error handling, Logging & Observability)
- `docs/reviews/testing_review_prompt.md` (Areas to Review §2, §4 —
  Go server testing, Test Quality / flakiness)
- `docs/llms.txt` (index; used to select the anchors above)
