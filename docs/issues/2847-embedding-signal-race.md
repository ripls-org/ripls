# Issue #2847: [flake] Embedding tests assert against rows whose embedding write silently failed

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2847
**Filed:** 2026-08-01 | **Last activity:** 2026-08-01
**Labels:** review:code_quality | **Assignees:** unassigned
**Context docs:** docs/server/semantic_search.md, docs/server/architecture.md (goroutine-safety anchor), docs/server/conventions.md (Fail Fast / Error Handling), docs/testing/architecture.md, docs/reviews/error_handling_review_prompt.md, docs/reviews/testing_review_prompt.md

## Problem Statement

Two Go integration tests flake on loaded CI runners with the same shape:
`chat_param_fallbacks` at `server/storage/semantic_search_test.go:214` (table
`gear`) and `chat_system_params` a day earlier (table `request`). Both fail as
"expected N results, got N-1" from a semantic-search query that runs *after*
draining the `embeddingDone` channel, and both leave a `"embedding storage
failed" … "record not found with id …: storage: record not found"` line in the
test log for the missing row. `-count=3` locally is green — this is a timing
race, not a logic bug in search.

Two separable defects are compounding:

1. **The `embeddingDone` signal lies.** `queueEmbeddingGeneration`
   (`server/storage/embedding.go:555–563`) fires the channel unconditionally
   *after* the generate-and-store attempt returns, and `generateEmbedding`
   (same file, `506–535`) only *warns* on failure — the storage-write branch
   at `526` swallows `ErrRecordNotFound` and returns normally. So the channel
   means "attempt finished," not "an embedding exists," and every test gated
   on it can proceed against a row that has no vector.

2. **The background write races the insert becoming visible.** `Insert`
   (`server/storage/protosql.go:22–113`) wraps in `WithTx` whenever the proto
   has an array-column registration (`hasArrayColumns && !inTransaction`, line
   28). Every type the failing tests use (`Gear`, `Request`, `Experience`,
   `User`, `Community`, `Story`) has a `media_ids` registration via
   `RegisterMediaIdsArrayColumns`, so the goroutine spawned at line 111 by
   `queueEmbeddingGeneration` is launched *inside* the transaction callback,
   and then the outer `WithTx` calls `Commit()`. Under load the goroutine's
   `s.updateEmbedding` — which routes through `s.db` (`embedding.go:279`), the
   connection pool, not the tx — wins the race against the commit becoming
   visible, so the `UPDATE … WHERE id = $2` matches 0 rows and returns
   `ErrRecordNotFound`.

The user-facing impact is bounded (a single row without an embedding until the
nightly `BackfillEmbeddings` fills it in), so this is a CI-signal-quality bug
first, and a "we would hide a real embedding regression if one landed" bug
second.

## Context from Comments

The only comment is `@claude` from maintainer triaging the flake to us —
no other reviewer discussion. The issue body itself carries the entire
analysis: separates the two defects, points at both `:539` (the unconditional
signal, now line 561 after `variantText` batching) and the `:515`/`:526`
warn-and-return paths, quotes both failing test snippets, and explicitly warns
against papering over the race with a sleep. It also flags
`SetEmbeddingDoneChannel` as the surface to grep for other callers.

Grep confirms the callers: `server/storage/semantic_search_test.go`,
`server/storage/unified_search_test.go`,
`server/storage/semantic_search_request_test.go`,
`server/storage/embedding_group_test.go`, and via the
`setupTestEmbedder`/`waitForEmbedding` helpers in
`server/media/cached_stock_imagery_test.go:277-306`, also
`server/media/stock_video_provider_cached_test.go` and
`server/media/cached_stock_imagery_test.go` (7+ tests).

## Potential Solutions

### Option A: Run the embedding goroutine after the transaction commits (after-commit hook)

- **Approach:** Introduce a per-tx "after-commit" queue in `ProtoSQLStorage`.
  `queueEmbeddingGeneration` detects `inTransaction()` and appends its work
  to the queue rather than spawning immediately; `WithTx.Commit()` drains
  the queue *after* a successful commit and spawns each item via
  `logging.GoSafe`. When not in a tx (still possible for types with no
  array-column registration), fall through to today's immediate spawn.
  Simultaneously, change the completion signal to carry outcome:
  `embeddingDone chan error` (nil on success, non-nil on any variant
  failure). Tests keep their existing "drain N signals" loop but read the
  value and `t.Fatal` on a non-nil error so a silently failed write turns
  into a legible test failure instead of "expected 1, got 0."
- **Pros:** Fixes the root cause — the write can never race the commit
  because it's scheduled from the commit path itself. Preserves the
  storage-layer encapsulation the semantic-search doc calls out as a design
  principle. `chan error` in a testing-only surface is a small, honest
  signature change; the affected callers are ~10 test files. Panic-recovery
  story stays intact (`logging.GoSafe`), which the goroutine-safety anchor
  (`docs/server/architecture.md` §Goroutine safety) enforces via
  `server/cmd/check-goroutines`.
- **Cons:** Touches `WithTx` (`server/storage/tx.go`), the storage struct
  (`server/storage/protosql_types.go`), and the two call sites in
  `Insert`/`Update` (`server/storage/protosql.go:111,522`). If the
  after-commit hook mechanism grows past this use case it needs its own
  short doc note; if it doesn't, it stays private (unexported) and is
  scoped to embeddings only.
- **Effort:** medium.

### Option B: Retry the update with bounded backoff on `ErrRecordNotFound`

- **Approach:** Leave `queueEmbeddingGeneration` structurally as-is but
  make `updateEmbedding` (or a wrapper called from `generateEmbedding`)
  retry a small handful of times with exponential backoff (e.g. 5ms, 20ms,
  80ms, then give up) specifically on `ErrRecordNotFound`. Fix defect (1)
  the same way as Option A: `chan error`, tests fail loudly on write
  failure.
- **Pros:** Minimal surgery — one new helper, no changes to `WithTx`. The
  retry also masks unrelated timing edges (a soft-deleted-then-restored
  row, cross-replica lag if we ever add read replicas).
- **Cons:** Doesn't fix the design — the goroutine still has no guarantee
  the row exists, we're just retrying long enough that the commit usually
  wins. Under a truly slow commit (contested lock, saturated I/O) the retry
  budget can still expire and we're back to the same "silent failure ->
  loud test failure" outcome, which is strictly better than today but
  worse than Option A. Retrying inside a warn-and-continue path also
  extends the goroutine's lifetime past the caller in ways that might
  surprise; not a correctness issue but worth noting.
- **Effort:** small.

### Option C: Make embedding generation synchronous

- **Approach:** Drop the goroutine entirely. `queueEmbeddingGeneration`
  becomes `generateEmbeddings` and runs inline in `Insert`/`Update`,
  returning any error. The `embeddingDone` channel goes away; tests just
  read the row.
- **Pros:** Eliminates the entire class of "we're testing an async signal"
  bugs. Simplifies the mental model.
- **Cons:** Rejected by the design principle in `docs/server/semantic_search.md`
  §"Storage-Layer Encapsulation" and by write-path latency. Local MiniLM
  inference is ~10–30ms per call; making it synchronous adds that latency
  to every Insert/Update of `Gear`/`Request`/`Experience` on the hot path
  of user actions. Also blocks writes on model init issues that today
  gracefully degrade.
- **Effort:** small-to-medium (mostly deleting code), but wrong tradeoff.

## Recommendation

**Option A**, combined with the `chan error` signal fix. The issue explicitly
calls out that these are two separable defects — (1) is the "carry the outcome
so a regression fails loudly" fix and (2) is the "understand why the row is
invisible before choosing a fix" ask, with the specific guidance "if it is
transaction visibility, the queue should run after commit." That's exactly
what the analysis above concludes: the row is invisible because we spawn
inside the tx and race the commit, so the queue should run after commit.

Option B is the tempting shortcut, but it leaves the design mismatch in
place. Option A costs one small `WithTx` extension and pays for itself the
next time we add an async post-write side effect (search-suggest cache
priming, activity feed fan-out, etc.) — this codebase already has a pattern
gap here.

## Open Questions

- **Should the after-commit hook be a general storage primitive (public,
  documented) or private to embeddings for now?** Recommendation: private
  for now — an unexported `deferAfterCommit(func())` on `ProtoSQLStorage`
  scoped to the tx clone via `withExecutor`, only used by
  `queueEmbeddingGeneration`. Promote when a second caller arrives; premature
  generalization would violate CLAUDE.md's "don't design for hypothetical
  future requirements."

- **When `Insert` is *not* wrapped in `WithTx` (proto without array-column
  registration — none of the embedded types today, but architecturally
  possible), does the goroutine still need after-commit ordering?**
  Recommendation: no — `s.exec == s.db` at the top level, so the insert
  is already visible on any pool connection when `ExecContext` returns.
  Fall through to the current immediate-spawn code path. Document the
  branch in a one-line comment.

- **Should `chan error` be blocking (unbuffered) or buffered?**
  Recommendation: buffered, matching today's `make(chan struct{}, 10)`
  pattern. If a test forgets to drain, the goroutine shouldn't block
  forever — it should drop the signal and log a warn, same as today's
  behavior modulo the type change.

- **Are there production callers of `SetEmbeddingDoneChannel`?**
  Recommendation: verified — no. Only test files. Safe to change the
  channel element type.

- **Do the media tests need the same "read outcome, fail loud" treatment?**
  Recommendation: yes — `waitForEmbedding` in
  `server/media/cached_stock_imagery_test.go:298` should read and check
  the error, and any silent-write flake in that suite is masked today by
  the same bug. Update as part of the same PR.

## Implementation Plan

### Phase 1: Fix defect (1) — carry the outcome on `embeddingDone`

**Goal:** Failed embedding writes fail their gating test loudly instead of
turning into a silent 0-vs-1 mismatch downstream.

**Tasks:**

- [ ] In `server/storage/protosql_types.go` (`ProtoSQLStorage.embeddingDone`
      field, line 185) change type from `chan struct{}` to `chan error`.
- [ ] In `server/storage/embedding.go`, change
      `SetEmbeddingDoneChannel(done chan struct{})` (line 415) to
      `SetEmbeddingDoneChannel(done chan error)`; keep the "testing only"
      doc comment.
- [ ] In `queueEmbeddingGeneration` (`server/storage/embedding.go:543`)
      accumulate variant errors into a single joined error via
      `errors.Join` and send it on `embeddingDone` (nil on all-success).
      Update the block comment on `generateEmbedding` (line 502) to reflect
      that the per-row signal now carries outcome.
- [ ] Change `generateEmbedding` (line 506) to return `error` and update
      the loop in `queueEmbeddingGeneration` to collect them via
      `errors.Join` (Go 1.20+; the repo is on 1.25.1). Keep the
      `WarnContext` calls — the log lines stay useful for post-hoc triage
      and match the `BackfillEmbeddings` symmetry (`:668`, `:749`, `:780`).
      **Fail-Fast alignment:** `docs/server/conventions.md` §Error Handling
      forbids "warn and continue" without a documented fallback. Keep an
      explicit one-line comment above the `WarnContext` calls stating the
      fallback is the nightly `BackfillEmbeddings`, so the pattern is
      documented not silent.
- [ ] Update all 4 storage-package test files and both media-package test
      files (list in Context from Comments) to `make(chan error, 10)` and
      to `t.Fatalf("embedding generation failed: %v", err)` when the drained
      value is non-nil. The media helper
      `setupTestEmbedder`/`waitForEmbedding`
      (`server/media/cached_stock_imagery_test.go:277–306`) is the only
      helper wrapping this pattern today — updating it once fans out to all
      7+ media tests. Do not extract a new cross-package helper in this PR;
      the storage-side tests each inline the pattern with a `time.After`
      that matches today's shape, and premature helper extraction would
      violate CLAUDE.md's "don't design for hypothetical future
      requirements." Revisit in a follow-up if a third package grows a
      copy.
- [ ] Add a targeted unit test in `server/storage/embedding_test.go` (new
      file if one doesn't exist) that: (a) configures a real embedder,
      (b) inserts a row and asserts `<-embeddingDone == nil`, (c) directly
      calls `generateEmbedding` on a non-existent id and asserts the joined
      error is non-nil and wraps `ErrRecordNotFound`.

**Validates:** After Phase 1, if a real embedding regression lands the
tests fail with a clear "embedding storage failed: record not found …"
message instead of an opaque "expected 2 results, got 1". The flake
described in the issue is still possible but now surfaces with the
diagnostic that names the real cause.

### Phase 2: Fix defect (2) — schedule the goroutine after commit

**Goal:** Eliminate the visibility race so the goroutine's `UPDATE` can
never fire before the row exists on the connection pool.

**Tasks:**

- [ ] Add an unexported `afterCommit []func()` slice to `ProtoSQLStorage`
      (`server/storage/protosql_types.go`), only ever populated on the tx
      clone returned by `withExecutor`.
- [ ] Add a private method `func (s *ProtoSQLStorage) deferAfterCommit(fn
      func())` in `server/storage/tx.go`. If `inTransaction()` is false it
      calls `fn()` immediately (preserves today's non-tx code path).
      Otherwise it appends to `afterCommit`.
- [ ] In `WithTx` (`server/storage/tx.go:35`), after the successful
      `rawTx.Commit()` at line 66, iterate `txStorage.afterCommit` and
      invoke each function. Do this *before* returning nil. Add a
      comment noting that panic recovery is the caller's responsibility
      (each hook is expected to wrap itself in `logging.GoSafe`, matching
      the goroutine-safety convention in `docs/server/architecture.md`
      §"Goroutine safety" enforced by `server/cmd/check-goroutines`).
- [ ] In `queueEmbeddingGeneration` (`server/storage/embedding.go:543`),
      replace the direct `logging.GoSafe(...)` call with
      `s.deferAfterCommit(func() { logging.GoSafe(ctx, "storage-generate-embedding", func() { ... }) })`.
      The variant-text extraction stays synchronous (as today) so it
      still reads this row's state, not a later mutation.
- [ ] Add a test in `server/storage/tx_test.go` (extend if it exists,
      create if not) that: (a) opens a `WithTx`, (b) calls
      `deferAfterCommit(fn)`, (c) inside the callback asserts `fn` has
      NOT yet run, (d) after `WithTx` returns asserts `fn` HAS run,
      (e) asserts that a rollback (fn returns an error) does NOT run
      the deferred hook.
- [ ] Add a **deterministic** ordering regression test in
      `server/storage/embedding_test.go`. Do **not** use timing/sleep — per
      `docs/reviews/testing_review_prompt.md` §"Test Quality" flaky-tests
      guidance, timing-based tests re-introduce the same class of bug we're
      fixing. Instead: wrap the deferred hook call site to record a "was
      commit visible when the hook fired?" flag by having the test's stub
      embedder synchronously `GetByID` its own row inside `Generate` and
      assert `err == nil`. If the hook fires before commit, `GetByID` will
      return `ErrRecordNotFound` and the test fails. Combined with the
      `tx_test.go` ordering test above, this locks the guarantee at both
      the mechanism level (afterCommit ordering) and the integration level
      (the embedding goroutine specifically sees its own row).

**Validates:** After Phase 2, running the semantic-search suite with
`-count=100 -race` on a loaded machine (or under `stress`) no longer
produces the "record not found" warn line. The regression test locks
the ordering into CI without depending on timing.

**Also validates:** `logging.GoSafe` is still the goroutine entry point,
so the panic-recovery guarantee in
`docs/server/architecture.md` §"Goroutine safety" is preserved and
`server/cmd/check-goroutines` continues to pass. The new
`WithTx`-post-commit iteration itself runs on the caller's goroutine,
not a spawned one, so it's not subject to that check.

### Phase 3: Documentation touch-up

**Goal:** Keep `docs/server/semantic_search.md` accurate about the
generation model.

**Tasks:**

- [ ] Update `docs/server/semantic_search.md` §"Embedding Generation"
      (line 133–137) to note that on-insert embedding work is scheduled
      *after* transaction commit when the caller is inside `WithTx`, so
      the goroutine can always read its own write. One or two sentences.
- [ ] Bump the doc's `freshness.verified_commit` and `verified_on`
      after the change lands, per the `docs/context_map.md` §"Prose
      freshness" protocol.

**Validates:** Doc-drift nightly ratchet stays green.

## Related Issues

- #2072 — Original migration that introduced the `RegisterArrayColumn`
  pattern that now causes `Insert` to wrap in `WithTx`, which in turn
  creates the visibility race with the embedding goroutine. Context for
  why the wrapping exists.
- #2694 — Experience name-variant embedding that established the
  "one `embeddingDone` signal per row after all variants" contract this
  plan preserves.
- #2795 — Prior embedding-path bug (dotted-path column translation) whose
  fix left the same warn-and-continue behavior in place; this plan
  addresses the "silent failure" pattern more broadly for embeddings.
