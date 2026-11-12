# Issue #1280: [performance] N+1 in ListConversations — per-conversation message and item-done queries

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1280
**Filed:** 2026-04-25 | **Last activity:** 2026-04-25
**Labels:** auto-filed, review:performance | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`ListConversations` and `GetUnreadCounts` both execute N+1 database queries because they
call `QueryByField("conversation_id", ...)` and `convstate.IsItemDone` inside a loop over
the user's conversations. For a user with N conversations, `ListConversations` fires 3×N
queries (2 per conversation for messages — once to count unread and once inside
`populateLastMessage` — plus 1 per conversation for the transfer/request terminal-state
check). `GetUnreadCounts` fires 2×N queries (1 for messages + 1 for the item-done check).

A user with 15 conversations triggers 45+ queries per inbox load, which is a critical
performance regression on a frequently-viewed screen.

## Context from Comments

One triage comment from a collaborator (2026-04-24) confirms this was filed by the
automated performance review, triaged as P1 Bug, and is part of the current sprint cycle.
No design debate or workarounds have been proposed — the fix is agreed to be batch-fetching.

## Potential Solutions

### Option A: Batch messages + batch topic items before the loop (recommended)

- **Approach:** After filtering to the user's conversations, collect all conversation IDs
  and batch-fetch all messages in one `QueryByFieldIn("conversation_id", ids, ...)` call.
  Group results into `map[string][]*models.ChatMessage`. Then collect all transfer IDs and
  request IDs from conversation topics, batch-fetch them with `GetByIDs`, and build lookup
  maps. Introduce `convstate.IsItemDoneFromMaps(conv, transferMap, requestMap)` that reads
  from those pre-fetched maps instead of hitting the DB. Introduce
  `populateLastMessageFromMessages(item, messages, userMap)` that accepts pre-fetched
  messages AND the already-fetched `map[string]*api.User` to resolve the last-message
  sender — eliminating the per-conversation `FetchAPIUsers` call inside
  `populateLastMessage` (line 822) that is a fourth N+1 not called out in the issue but
  fixed by the same batch approach. Apply the same message/item batch pattern to
  `GetUnreadCounts`.
- **Pros:** Reduces ListConversations from 4N to O(1) extra queries (3 batch queries total:
  messages, transfers, requests; users already batched). Reduces GetUnreadCounts from 2N to
  O(1). Zero semantic change — all filtering, archiving, and unread-count logic is
  preserved. Follows existing patterns (`GetByIDs`, `QueryByFieldIn`, `FetchAPIUsersBatch`).
- **Cons:** Slightly increases memory usage (all conversation messages in RAM at once).
  For users with very high message volume this could be large, but the existing per-loop
  approach already loads all messages — just one at a time.
- **Effort:** Medium — 3 file changes, 1 new helper function, tests for query count.

### Option B: Add a conversation_id index and per-conversation LIMIT query

- **Approach:** Keep the per-conversation query loop but add a `LIMIT`-ed query that only
  fetches the most-recent message and the unread count via a SQL aggregate, avoiding the
  full message scan per conversation.
- **Pros:** Reduces data transferred per conversation; doesn't require restructuring the
  loop.
- **Cons:** Still O(N) queries — doesn't fix the N+1 problem. Requires a new SQL query
  path not currently supported by the `ProtoSQLStorage` abstraction, which would require
  significant new infrastructure. Does not address the `IsItemDone` N+1.
- **Effort:** Large — requires new SQL query helpers and still leaves N+1.

### Option C: Cache conversation state in the `ChatConversation` proto

- **Approach:** Denormalize — store `last_message_text`, `unread_count_by_user`, and
  `item_done` fields directly on `ChatConversation` and update them on write (message send,
  transfer state change).
- **Pros:** Makes inbox reads O(1) once conversations are loaded.
- **Cons:** Requires cache invalidation logic across multiple write paths (SendMessage,
  transfer state changes, request state changes). High risk of stale data. Much larger
  scope than the perf fix warrants.
- **Effort:** Large.

## Recommendation

**Option A** — batch-fetch before the loop. It directly follows the SQL efficiency
guidelines in CLAUDE.md ("Never query in a loop", "Use batch APIs"), uses existing
storage primitives (`QueryByFieldIn`, `GetByIDs`), and has zero semantic risk. The scope
is well-contained: 3 Go files + tests. Option B doesn't fix N+1. Option C is high-risk
over-engineering for this issue.

## Open Questions

- **Should `populateLastMessage` keep its current signature for single-conversation use
  cases (e.g., `GetConversation`)?**
  Recommendation: Yes — keep the existing `populateLastMessage(ctx, item)` method for
  single-conversation paths. Add a new private helper
  `populateLastMessageFromMessages(item, messages)` that takes pre-fetched messages; the
  old method calls the new one after fetching. This avoids breaking other callers.

- **Should `IsItemDoneFromMaps` be exported from the `conversation` package?**
  Recommendation: Export it (`IsItemDoneFromMaps`) so both `ListConversations` and
  `GetUnreadCounts` can share it without duplication, and so it is testable in isolation.

- **Should we add an `AssertMaxQueries` regression test?**
  Recommendation: Yes — add a test that sets up N=3 conversations and asserts the query
  count stays bounded (≤10 regardless of N) using `storage.AssertMaxQueries`.

## Implementation Plan

### Phase 1: Add `IsItemDoneFromMaps` to the conversation package
**Goal:** Provide a batch-friendly terminal-state check that reads from pre-fetched maps.
**Tasks:**
- [ ] Add `IsItemDoneFromMaps(conv *models.ChatConversation, transfers map[string]proto.Message, requests map[string]proto.Message) bool` to `server/conversation/state.go`
- [ ] Add unit tests for `IsItemDoneFromMaps` covering transfer/request terminal states and the default false case
**Validates:** The new helper is correct before we wire it into the RPC handler.

### Phase 2: Refactor `ListConversations` to use batch queries
**Goal:** Eliminate the 3N per-conversation queries in `ListConversations`.
**Tasks:**
- [ ] After the first pass (user conversation filter), collect all conversation IDs
- [ ] Batch-fetch all messages: `s.storage.QueryByFieldIn("conversation_id", convIDs, &models.ChatMessage{})`
- [ ] Group messages into `map[string][]proto.Message` keyed by conversation ID
- [ ] Collect all transfer IDs and request IDs from conversation topics
- [ ] Batch-fetch transfers: `s.storage.GetByIDs(ctx, transferIDs, &models.Transfer{})`
- [ ] Batch-fetch requests: `s.storage.GetByIDs(ctx, requestIDs, &models.Request{})`
- [ ] Replace `s.storage.QueryByField(...)` at line 441 with lookup from message map
- [ ] Replace `convstate.IsItemDone(...)` at line 456 with `convstate.IsItemDoneFromMaps(...)`
- [ ] Add `populateLastMessageFromMessages(item *api.ConversationItem, messages []proto.Message, userMap map[string]*api.User) error` helper that takes pre-fetched messages and user map (eliminating the per-message `FetchAPIUsers` call inside the existing `populateLastMessage`)
- [ ] Make `populateLastMessage` delegate to the new helper (fetches messages and users, then delegates; preserves single-conversation callers)
- [ ] Replace `s.populateLastMessage(ctx, item)` at line 489 with the new helper, passing the pre-fetched message slice and the existing `userMap`
- [ ] Add `AssertMaxQueries` regression test for `ListConversations` with 3 conversations
**Validates:** ListConversations fires ≤ O(1) extra queries (bounded regardless of conversation count).

### Phase 3: Refactor `GetUnreadCounts` to use batch queries
**Goal:** Eliminate the 2N per-conversation queries in `GetUnreadCounts`.
**Tasks:**
- [ ] Apply the same batch-fetch pattern (messages + topic items) before the loop
- [ ] Replace `convstate.IsItemDone` at line 67 with `convstate.IsItemDoneFromMaps`
- [ ] Replace `s.storage.QueryByField(...)` at line 73 with lookup from message map
- [ ] Add `AssertMaxQueries` regression test for `GetUnreadCounts` with 3 conversations
**Validates:** GetUnreadCounts fires ≤ O(1) extra queries regardless of conversation count.

## Related Issues

- #964 — generalized undo — touches transfer/request state changes; N+1 fix should not interfere
- #1096 — db connection exhaustion — same symptom class (too many queries under load)
