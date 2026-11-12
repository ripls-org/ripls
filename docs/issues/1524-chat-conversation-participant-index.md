# Issue #1524: [performance] ListConversations and GetUnreadCounts use unbounded ListAll(ChatConversation)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1524
**Filed:** 2026-04-28 | **Last activity:** 2026-04-29
**Labels:** auto-filed, review:performance | **Assignees:** unassigned
**Priority:** P1 (set by /ripls-triage on 2026-04-28)
**Context docs:** none

## Problem Statement

Both inbox-style RPCs in the chat service fetch every row of the global
`chat_conversation` table on every request, then filter to the caller's
conversations in Go memory:

- `server/services/chat/conversations.go:392` — `ListConversations` calls
  `s.storage.ListAll(ctx, &models.ChatConversation{})` and then walks the
  returned slice to keep only conversations whose `participant_ids` contains
  the authenticated user.
- `server/services/chat/unread_counts.go:35` — `GetUnreadCounts` does
  the same `ListAll` + in-memory filter.

The `participant_ids` repeated field is *not* materialized as a SQL column
by ProtoSQL — `flattenFields` in `server/storage/protosql_schema.go:44-46`
explicitly skips repeated fields. Today the field only lives inside the
serialized `binary_proto` payload, so there is no index to push the filter
down to. Cost therefore scales with the total platform-wide conversation
count multiplied by the active-session refresh rate, not with per-user
activity. The issue body calls this out as the single largest scaling cliff
remaining in the server.

The downstream batch fetches in both handlers (`QueryByFieldIn` for
messages, `GetByIDs` for transfers/requests) are already bounded by the
*user's* conversation count after filtering, so they are fine — only the
initial conversation fetch needs to be replaced. The existing
`AssertMaxQueries` regression tests pass today only because they create a
small number of conversations; they do not actually exercise the row-count
problem.

## Context from Comments

- **2026-04-28 — maintainer (triage):** Issue Type set to **Task**,
  Priority set to **P1**. Rationale: "performance Critical: unbounded
  `ListAll(ChatConversation)` table scan in hot path."
- **2026-04-29 — maintainer:** `@claude` ping requesting work to begin.

No further design discussion in comments — the issue body's
"Recommendation" paragraph is the only design input. It proposes three
shapes:

1. A `chat_conversation_participant` join table queried by `user_id`.
2. A JSONB GIN index on `participant_ids`.
3. A Postgres array column with array-containment (`@>`) and a GIN index.

It also asks for an `AssertMaxQueries` assertion *and* a row-count
assertion to lock in the fix.

## Codebase Research

The repo already has a working template for option 3 in
`server/storage/story.go` and `server/storage/story_schema.go`. The
`Story` proto has a `repeated string participant_ids` field with the same
shape as `ChatConversation.participant_ids`, and the storage layer:

- Adds an `ALTER TABLE "Story" ADD COLUMN participant_ids TEXT[]` column
  on startup (`InitializeStoryParticipantColumn`).
- Backfills the column from `binary_proto` for any rows where it is `NULL`.
- Creates `idx_story_participant_ids USING GIN (participant_ids)`.
- Wraps `ProtoSQLStorage` in a `StoryStorage` type whose `Insert` method
  populates the column manually (since `ProtoSQL.Insert` skips repeated
  fields), and exposes `ListByUser(ctx, userID, limit)` that runs
  `WHERE $1 = ANY(participant_ids)`.

`StoryStorage` is wired up in `server/main.go:709` and consumed by
`server/services/feed/` and `server/services/user/`.

`ChatConversation` is more complex than `Story` because it is *mutated*
after creation, not just inserted:

- `server/chat/conversations.go:113` — `CreateOrGetConversation` (initial
  insert).
- `server/chat/conversations.go:161` — `AddParticipantToConversation` (calls
  `storage.Update`).
- `server/chat/conversations.go:207` — `RemoveParticipantFromConversation`
  (calls `storage.Update`).
- `server/services/chat/conversations.go:81` — `validateAndCleanParticipants`
  (calls `storage.Update`).
- `server/services/chat/conversations.go:148, 178` — `addUserAsParticipant`
  (calls `storage.Update`).
- `server/services/chat/mention_handler.go:102` — `processMentions` (calls
  `storage.Update`).

Every one of these sites currently relies on the generic
`ProtoSQLStorage.Update` (`server/storage/protosql.go:322`), which writes
the entire row including `binary_proto` but does not touch repeated-field
columns. The Story pattern works around this for inserts only; for chat we
need the indexed column to stay in sync on update too.

## Potential Solutions

### Option A: Indexed `TEXT[]` column on `chat_conversation`, sync from `binary_proto`
- **Approach:** Mirror the Story pattern. Add a `participant_ids TEXT[]`
  column to `chat_conversation` with a GIN index, backfill on startup, and
  ensure both `Insert` and `Update` populate the column. Add a
  `ChatConversationStorage` wrapper (parallel to `StoryStorage`) that
  exposes `Insert`, `Update`, and `ListByParticipant(ctx, userID)`. Route
  the existing mutation sites through the wrapper. Replace the
  `ListAll(ChatConversation)` calls in `ListConversations` and
  `GetUnreadCounts` with `ListByParticipant(authInfo.UserID)`.
- **Pros:** Reuses an already-merged, already-tested pattern. Minimal
  schema change — one new column, one GIN index, one startup hook.
  No proto changes; wire-compatible. Single SQL query per inbox call.
  Easy to test (`AssertMaxQueries` plus a row-count assertion can both
  be added without test-harness changes). Soft-delete already works
  correctly because the Update path keeps `participant_ids` in lockstep
  with the proto.
- **Cons:** Requires a wrapper layer and we have to remember to use it
  — direct `s.storage.Update(conversation)` calls would silently let
  the indexed column diverge from the truth in `binary_proto`. We
  mitigate that by collapsing all six mutation sites onto the wrapper
  and adding a unit test that updates a conversation and re-reads via
  `ListByParticipant`.
- **Effort:** Small / medium.

### Option B: Explicit `chat_conversation_participant` join table
- **Approach:** Define a new proto model (per the issue's first
  recommendation) with `(id, conversation_id, participant_id, joined_at_unix_sec)`,
  treat it as the source of truth for membership, and rewrite all
  participant reads/mutations to operate on rows in that table. Replace
  the `ListAll` calls with a join-table query.
- **Pros:** Cleanly normalized, mirrors `CommunityUser` /
  `CommunityGear` / `CommunityExperience`. Could grow per-participant
  metadata (last-read timestamps, notification prefs) in the future.
- **Cons:** Bigger blast radius. `ChatConversation.participant_ids` is
  read in many places (auth checks, notification fan-out, the inbox
  handlers themselves, mention plumbing, story generation). Every read
  becomes a JOIN or a separate query. Mutation paths grow from one
  `Update` to insert/delete row pairs that have to stay in sync with
  `binary_proto` *or* the proto field has to be removed (a bigger
  proto-contract change). Migration is non-trivial: we have to backfill
  the join table from existing `binary_proto` payloads. The issue
  explicitly lists this as one of three options, not the only one.
- **Effort:** Large.

### Option C: JSONB column with GIN index on `participant_ids`
- **Approach:** Add a `participant_ids JSONB` column populated from the
  proto and create a GIN index using `jsonb_path_ops`. Query with
  `participant_ids @> '"<user_id>"'::jsonb`.
- **Pros:** Single column type can accommodate richer participant
  metadata later (status, joined-at, etc.) without further migrations.
- **Cons:** Strictly worse than option A for this exact use case —
  Postgres `TEXT[]` with a GIN index is the canonical shape for
  array-of-strings membership and produces simpler SQL (`= ANY(col)`).
  We already have the working precedent in `StoryStorage`. JSONB is
  warranted when the value is genuinely structured, not when it's a
  flat list of IDs.
- **Effort:** Small / medium.

## Recommendation

**Option A (`TEXT[]` indexed column with a `ChatConversationStorage`
wrapper).** It is a straight port of the existing, working `Story`
pattern, requires no proto change, no data migration beyond a one-pass
backfill on first boot, and reduces the inbox query from a full table
scan to one indexed lookup. The wrapper layer is the natural place to
keep the indexed column in sync with `binary_proto` on both insert and
update, and it lets us collapse all six current mutation sites onto a
single API surface that future authors will pick up automatically.

Option B is structurally cleaner long-term but is a much larger change
than #1524 calls for; we can revisit it if per-participant metadata
becomes a real need. Option C provides no advantage over A for this
field shape.

## Open Questions

- **Should the wrapper wrap `Insert`/`Update` for *all* `ChatConversation`
  writes, or just the ones reached from chat services?** —
  Recommendation: **all writes**. Mention handler, defensive
  participant cleanup, and story generation all touch participants;
  any unwrapped `s.storage.Update(&models.ChatConversation{...})` is a
  latent staleness bug. Keep the search for direct calls in scope of
  Phase 2. (`grep -rn "Update(.*ChatConversation\|Update(.*conversation\b"`
  in `server/`.)
- **Should `ListByParticipant` filter soft-deleted rows in the SQL?** —
  Recommendation: **yes**. Add `AND COALESCE(deleted_deleted_at_unix_sec, 0) = 0`
  to the WHERE clause, matching `StoryStorage.ListByCommunity`. The
  current Go-side filter (`conversation.Deleted != nil && ...`) becomes
  redundant and can be removed in the same change.
- **Should `ListByParticipant` accept a limit?** — Recommendation: yes,
  with a default cap of 200 conversations to satisfy CLAUDE.md's
  "always LIMIT unbounded queries" rule. This is far above any
  realistic per-user inbox; if we ever hit it, paginating the API is
  a separate, larger change.
- **Backfill safety on a hot table.** — Recommendation: the
  `Story` backfill iterates row-by-row and updates each one in its own
  statement. For `chat_conversation` the table is bigger but the same
  pattern is fine because it only runs when the column is `NULL`,
  which is true exactly once per row. Log a final count and elapsed
  time at `Info`. If the operation is unacceptable on prod, the
  fallback is a Postgres `UPDATE ... SET participant_ids = ARRAY(...)`
  computed from a `jsonb_array_elements_text` of the proto — but
  ProtoSQL has no JSON view of `binary_proto`, so we'd need a custom
  decoder. Defer unless backfill latency proves to be a problem.
- **Test budget after the fix.** — Recommendation: tighten
  `AssertMaxQueries` from 10 to 6 in both `TestListConversations_NoPlusOneQueries`
  and `TestGetUnreadCounts_NoPlusOneQueries`. The post-fix budget is
  1 (`ListByParticipant`) + 1 (batch messages) + 1 (batch transfers)
  + 1 (batch requests) + 1 (batch users, only `ListConversations`) = 5.
  Add a *row-count* assertion: create N conversations the user *isn't*
  in alongside M they *are* in, then assert that the response contains
  exactly M and that `binary_proto` was decoded fewer than N+M times
  (proxied via the query count).

## Implementation Plan

> **Conventions to honor in every phase.** Use double-quoted SQL identifiers
> (the table is registered as `chat_conversation` and `protosql_schema.go`
> emits `CREATE TABLE "chat_conversation" (...)`). All logging in new code
> must use `logging.LoggerWithContext(ctx)` with `"operation"` set, and
> `*Context` slog variants (`InfoContext`, `ErrorContext`) — see
> CLAUDE.md "Structured Logging (Go Server)". New exported symbols need
> doc comments that start with the symbol name; `revive.exported`
> enforces this at error severity.

### Phase 1: Storage layer — indexed column, wrapper, query
**Goal:** Make `participant_ids` queryable in O(log N) and provide an
API for it; ground truth still lives in `binary_proto`.
**Tasks:**
- [ ] Add `server/storage/chat_conversation_schema.go`:
  - `(s *ProtoSQLStorage) InitializeChatConversationParticipantColumn(ctx)`,
    modeled directly on `InitializeStoryParticipantColumn` in
    `server/storage/story_schema.go`. The `information_schema.tables`
    existence check uses `WHERE table_name = 'chat_conversation'`
    (lowercase — Postgres folds the unquoted form, and the table was
    created with double-quoted lowercase). The `ALTER TABLE` and
    `CREATE INDEX` statements all reference `"chat_conversation"`
    quoted exactly.
  - `backfillChatConversationParticipantIds(ctx)` — same shape as
    `backfillStoryParticipantIds`. Unmarshal `&models.ChatConversation{}`
    rather than `&models.Story{}`. Log a final summary line at
    `Info` with `"chat_conversation_count"` and `duration_ms`.
- [ ] Add `server/storage/chat_conversation.go`:
  - `type ChatConversationStorage struct { storage *ProtoSQLStorage }`
  - `NewChatConversationStorage(*ProtoSQLStorage) *ChatConversationStorage`.
  - `(cs *ChatConversationStorage) Insert(ctx, *models.ChatConversation) (string, error)` —
    delegates to `storage.Insert`, then `UPDATE "chat_conversation" SET participant_ids = $1 WHERE id = $2`.
  - `(cs *ChatConversationStorage) Update(ctx, *models.ChatConversation) error` —
    delegates to `storage.Update`, then the same `UPDATE participant_ids`.
    **Do not wrap the pair in a transaction** — the existing
    `StoryStorage.Insert` does not, and matching that pattern keeps
    behavior consistent. The `binary_proto` column is authoritative;
    the indexed column is a denormalization. Document this invariant
    in a doc comment on `ChatConversationStorage` so future readers
    don't assume otherwise. Drift, if any, is recoverable via the
    backfill on next startup.
  - `(cs *ChatConversationStorage) ListByParticipant(ctx, userID string, limit int) ([]*models.ChatConversation, error)` —
    `SELECT binary_proto FROM "chat_conversation" WHERE $1 = ANY(participant_ids) AND COALESCE(deleted_deleted_at_unix_sec, 0) = 0 ORDER BY last_message_at_unix_sec DESC LIMIT $2`.
    Default limit `200` when `limit <= 0`.
  - All three methods take `ctx context.Context` first; all three use
    `logging.LoggerWithContext(ctx).With("operation", ...)` for any
    error/debug logging; doc comments start with the symbol name.
- [ ] Wire `InitializeChatConversationParticipantColumn` into
  `InitializePostgreSQLDatabase` in `server/storage/postgresql.go`
  immediately after the existing `InitializeStoryParticipantColumn`
  call (line 99). Because `SetupTestStorage` (in
  `server/storage/testing.go:672`) goes through the same
  `InitializePostgreSQLDatabase`, all existing tests automatically
  get the new column without test-harness edits.
- [ ] Unit tests under `server/storage/chat_conversation_test.go`:
  - Insert via wrapper, then `ListByParticipant` returns the row.
  - Update via wrapper changes `participant_ids` reflected in
    `ListByParticipant`.
  - Direct `storage.Update` (a future-proofing canary) followed by
    `ListByParticipant` returns *stale* data — keep this as a
    deliberate negative test so future readers see why the wrapper
    is required.
  - Soft-deleted conversations are excluded.
  - Result ordering: a conversation with a newer
    `last_message_at_unix_sec` appears first.
  - Default limit kicks in when `limit <= 0`.
**Validates:** The schema, backfill, and query-by-participant all work
end to end against the testcontainer. After this phase the new code
exists but no callers use it yet.

### Phase 2: Route mutations through the wrapper
**Goal:** Every write path that mutates `ChatConversation` populates
the indexed column.
**Tasks:**
- [ ] Plumb `*storage.ChatConversationStorage` into `server/services/chat/Service`
  alongside the existing `*storage.ProtoSQLStorage` (analogous to how
  `feed.Service` holds both). Construct it in `server/main.go`.
- [ ] Replace the following calls (all currently
  `s.storage.Update(conversation)` or `storage.Insert(conversation)`):
  - `server/chat/conversations.go:113` — `CreateOrGetConversation`
    insert.
  - `server/chat/conversations.go:161` — `AddParticipantToConversation`
    update.
  - `server/chat/conversations.go:207` —
    `RemoveParticipantFromConversation` update.
  - `server/services/chat/conversations.go:82` —
    `validateAndCleanParticipants` update.
  - `server/services/chat/conversations.go:149, 179` —
    `addUserAsParticipant` updates.
  - `server/services/chat/mention_handler.go:102` — `processMentions`
    update.
  - Any additional `Update(...ChatConversation...)` discovered via
    `grep -rn "ChatConversation" server/ | grep Update`.
- [ ] For helpers in `server/chat/` that don't currently take a
  service, accept a `*storage.ChatConversationStorage` parameter
  rather than a `*storage.ProtoSQLStorage`. Update callers (the chat
  service methods).
- [ ] Add a unit test in `server/services/chat/` that calls
  `addUserAsParticipant` (or `processMentions`) and then asserts the
  newly added user appears in `ListByParticipant` results.
**Validates:** Every mutation path keeps the indexed column current; a
post-mutation `ListByParticipant` agrees with the proto.

### Phase 3: Replace inbox `ListAll` calls + tighten regression tests
**Goal:** Hot path uses the index. Lock the gain in.
**Tasks:**
- [ ] `server/services/chat/conversations.go:392`: replace
  `s.storage.ListAll(ctx, &models.ChatConversation{})` with
  `s.chatConvStorage.ListByParticipant(ctx, authInfo.UserID, 0)`.
  Drop the in-memory participant filter (`isParticipant` loop) and
  the soft-delete filter — both are now done in SQL.
- [ ] `server/services/chat/unread_counts.go:35`: same swap.
- [ ] Tighten the existing `AssertMaxQueries` budgets:
  - `TestListConversations_NoPlusOneQueries` (currently `max=10` at
    `server/services/chat/conversations_test.go:1029`) → `max=6`.
    Post-fix budget: 1 (`ListByParticipant`) + 1 (batch messages) +
    1 (batch transfers) + 1 (batch requests, may be empty) + 1
    (batch users) = 5.
  - `TestGetUnreadCounts_NoPlusOneQueries` (currently `max=10` at
    `server/services/chat/unread_counts_test.go:593`) → `max=5`.
    Post-fix budget: 1 (`ListByParticipant`) + 1 (batch messages) +
    1 (batch transfers, may be empty) + 1 (batch requests, may be
    empty) = 4.
- [ ] Add a *row-count* assertion test for each handler. Construct N
  conversations the test user is not in alongside M they are in
  (e.g. N=20, M=3), call the handler, and assert that:
  - `len(resp.Conversations)` (or the unread-count map) equals M
    exactly — none of the N "other-user" conversations leak through.
  - `AssertMaxQueries` budget stays at the new tightened value, so
    the query count did **not** scale with N (catches a regression
    where someone reverts to a full scan + Go-side filter).
- [ ] Run `npm run build`, then `go test -tags=integration -timeout
  20m ./server/...`, and iterate until green. Specifically watch
  for: (a) any caller still using the raw `s.storage.Update` for
  `ChatConversation` that we missed in Phase 2 (the negative test
  from Phase 1 will demonstrate this is a bug), and (b) tests that
  pre-seeded conversations directly via `sqlStorage.Insert(...)`
  rather than the wrapper — those rows will have `NULL`
  `participant_ids` until backfill runs. Either route those test
  inserts through the wrapper or call the backfill explicitly in
  test setup. Several existing tests (e.g. the body of
  `TestGetUnreadCounts_NoPlusOneQueries` at lines 564–572) directly
  call `sqlStorage.Insert(ctx, &models.ChatConversation{...})` and
  must be updated.
**Validates:** `ListConversations` and `GetUnreadCounts` query
complexity is now bounded by the *user's* conversation count, not the
platform total, and the regression tests will fail loudly if a future
change reverts that.

## Related Issues
- None directly. The `Story` participant-index pattern this plan
  inherits was added in an earlier change (see
  `server/storage/story_schema.go` history); referencing it here is
  fine but no open issue blocks this work.

## Audit notes (`/ripls-audit`, 2026-04-29)

Reviewed against `docs/server/architecture.md` and
`docs/server/observability.md`. The plan respects the storage-logic-in-storage-package rule
(no raw SQL in services), the constructor-injection pattern (the chat
service receives `*storage.ChatConversationStorage` alongside
`*storage.ProtoSQLStorage`), and the file-size guideline (the new
files are small and single-purpose). The recommended option reuses the
existing, working `Story` template rather than introducing a new
abstraction.

Adjustments applied:

1. **Identifier quoting.** Added an explicit conventions block at the
   top of the implementation plan stating that the table is registered
   as `chat_conversation` and that `protosql_schema.go` quotes it as
   `"chat_conversation"`; spelled out which form to use in
   `information_schema.tables` (lowercase, unquoted) versus
   `ALTER TABLE` / `CREATE INDEX` (double-quoted).
2. **Logging conventions.** Spelled out the CLAUDE.md / observability
   requirements: `logging.LoggerWithContext(ctx).With("operation", ...)`
   plus `*Context` slog variants; backfill logs a final summary at
   `Info` with `chat_conversation_count` and `duration_ms`. Doc
   comments on exported symbols must start with the symbol name
   (`revive.exported`).
3. **Transaction decision pinned.** The wrapper's two-phase write
   (`storage.Insert/Update` → `UPDATE participant_ids`) explicitly
   matches `StoryStorage` in *not* using a transaction; the
   `binary_proto` is authoritative, and any drift recovers via the
   startup backfill. This is documented as an invariant on the
   wrapper type rather than left ambiguous.
4. **Test setup automatically covered.** Confirmed
   `SetupTestStorage(t)` (`server/storage/testing.go:672`) routes
   through `InitializePostgreSQLDatabase`, so wiring the new init
   call there propagates to every test using the testcontainer
   fixture without harness edits.
5. **Existing test inserts flagged.** Tests like
   `TestGetUnreadCounts_NoPlusOneQueries` currently call
   `sqlStorage.Insert(ctx, &models.ChatConversation{...})` directly
   to seed conversations. Phase 3 explicitly calls these out as
   needing to be routed through the new wrapper (or have their seeded
   rows backfilled in test setup); without this fix the new
   index-based `ListByParticipant` would return zero rows and the
   tests would silently pass with `0 == 0`-style assertions.
6. **Tightened query budgets distinguished per RPC.** `GetUnreadCounts`
   has a smaller real budget than `ListConversations` (no user
   batch-fetch). Plan now says `max=5` for `GetUnreadCounts` and
   `max=6` for `ListConversations`, with the breakdown of the five /
   four expected queries stated inline.
