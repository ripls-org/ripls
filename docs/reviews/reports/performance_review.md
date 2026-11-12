# Performance Review - 2026-04-28

## Executive Summary

Significant remediation since the last review four days ago: **eight of the ten Critical/High issues filed last run were resolved**, including the two new Critical N+1 patterns in `ListConversations`/`GetUnreadCounts` and `GetCommunityLeaderboard`, the longest-aging items (`ListCommunityEvents`, `GetUserTransferStatus`, `buildTransfers` redundant `GetByID`, missing HTTP server timeouts), and the `ListMyCommunities`/`getSharedCommunityMap`/portfolio `IsItemDone` regressions. The remaining unaddressed items are largely the same medium/low scale findings that have been carrying over (background goroutine cancellation, Flutter `MessageList` eager rendering, unbounded `QueryByField`, bubble sort in spatial query, presence map leak).

The most important newly-recognized issue is that **`ListConversations` and `GetUnreadCounts` still call `ListAll(&ChatConversation{})`** — the per-conversation N+1 is gone but every inbox/unread refresh still fetches every conversation in the database and filters in memory. This is the highest-impact scaling cliff in the codebase right now and should be the headline action. Two earlier findings (`ref.listen` in `build()`, `time.After` in broadcast loops) remain; the `ref.listen` pattern has additionally spread to community and request content views.

This review surfaces exactly four unfilable findings (one Critical, two High, one Medium-as-pattern). All are filed below.

## Changes Since Last Review

**Previous Review Date**: 2026-04-24

**Remediated Issues:**

- [Resolved] Critical: N+1 in ListConversations / GetUnreadCounts per-conversation message and item-done queries
  - Location: `server/services/chat/conversations.go:443-489`, `server/services/chat/unread_counts.go:81-110`
  - `QueryByFieldIn("conversation_id", convIDs, ...)` batches messages; `IsItemDoneFromMaps` consults pre-fetched transfer/request maps
- [Resolved] Critical: N+1 in GetCommunityLeaderboard per-member community-wide queries
  - Location: `server/services/impact_metrics/leaderboard.go:59-98`
  - Pre-loads transfers + community requests; `CalculateUserImpactFromPreloadedData` works in memory
- [Resolved] Critical: N+1 in ListCommunityEvents per-event user and gear queries (45+ days aging)
  - Location: `server/services/community/events.go:90-175`
  - Uses `FetchAPIUsersBatch` and `storage.GetByIDs` with pre-collected ID sets
- [Resolved] Critical: Missing HTTP server timeouts (45+ days aging)
  - Location: `server/main.go:993-1000`
  - `ReadHeaderTimeout: 10s`, `ReadTimeout: 30s`, `WriteTimeout: 120s`, `IdleTimeout: 120s`
- [Resolved] Critical: N+1 in GetUserTransferStatus per-transfer conversation query (45+ days aging)
  - Location: `server/services/transfer/queries.go:305-337`
  - Collects non-terminal transfer IDs and uses `QueryByFieldIn("topic_transfer_id", transferIDs, ...)` once
- [Resolved] High: buildTransfers redundant GetByID loop ignores pre-fetched gearMap
  - Location: `server/services/transfer/service.go:212-218`
  - Now reads `gearStored, ok := gearMap[k.gearID]` from the batch
- [Resolved] High: N+1 in ListUserGear via getSharedCommunityMap
  - Location: `server/services/gear/service.go:764-789`
  - Single `QueryByFieldIn("gear_id", gearIDs, ...)` then in-memory grouping
- [Resolved] High: N+1 in ListMyCommunities GetByID per community
  - Location: `server/services/community/membership.go:223-233`
  - Single `storage.GetByIDs[*models.Community]` after collecting IDs
- [Resolved] High: Portfolio buildConvUnreadCounts IsItemDone GetByID per conversation
  - Location: `server/services/portfolio/fetch.go:632-651`
  - Now takes `transferMap` and `reqMap` and uses `IsItemDoneFromMaps`
- [Partial] High: Notification background goroutines without timeout
  - Location: `server/community/events.go:77-85` — `notifyTargetedUsers` now wraps `context.WithTimeout(context.Background(), notificationTimeout)` with `logging.GoSafe`. Backfills (impact + community cascade) now use the root `ctx` cancelled on shutdown. Embedding backfill (`server/main.go:567`) is the only remaining offender.

**Unaddressed Issues:**

- High: Embedding backfill goroutine still uses `context.Background()`
  - Original Priority: High
  - Current Priority: High
  - Location: `server/main.go:567`
- High: `buildAPIExperience` per-experience queries inside ListExperiences / ListMyExperiences loop
  - Original Priority: Low (re-evaluated up — see New Issues section; this is genuinely a hot path)
  - Location: `server/services/experience/service.go:156-307`, called in loop at `get.go:183`, `get.go:225`
- High: Unbounded `QueryByField` / `QueryByFields` (no Limit support, despite ListAll having it) (50+ days aging)
  - Original Priority: High
  - Current Priority: High
  - Location: `server/storage/protosql.go:198-249`, `server/storage/protosql.go:255-319`
- Medium: `ref.listen` inside `build()` in content views (now four sites, not two)
  - Original Priority: Medium
  - Current Priority: Medium
  - Location: `app/lib/presentation/screens/gear/gear_content_view.dart:310`, `experience/experience_content_view.dart:414`, `communities/community_content_view.dart:222`, `request/request_content_view.dart:370`
- Medium: `time.After` inside broadcast loop (50+ days aging)
  - Original Priority: Medium
  - Current Priority: Medium
  - Location: `server/services/chat/streaming.go:215`, `server/services/community/streaming.go:270`
- Medium: MessageList still uses eager `ListView` instead of `ListView.builder`
  - Location: `app/lib/presentation/widgets/chat/message_list.dart:217`
- Medium: RecordAttendance multi-query and per-record Update in loop
  - Location: `server/services/experience/rsvp.go:346-440`
- Medium: Stream channel buffer size 10 (chat + community)
  - Location: `server/services/chat/streaming.go:53`, `server/services/community/streaming.go:52`
- Medium: IncrementViewCountBatch per-item Update inside loop
  - Location: `server/storage/feed.go:163`
- Low: Bubble sort in QueryByProximity (50+ days aging)
  - Location: `server/storage/protosql_spatial.go:95-101`
- Low: User presence map sets to `false` instead of deleting
  - Location: `server/services/chat/streaming.go:182`

**New Issues:**

- Critical: `ListAll(ChatConversation{})` in ListConversations and GetUnreadCounts loads every conversation in the database on every request
- Medium: `ref.listen` pattern has spread from 2 → 4 content views, indicating an emerging anti-pattern not caught by lint

**Performance Improvements:**

- Eight of ten previously-filed Critical/High issues fully remediated within four days — strong remediation velocity
- Chat conversation pre-fetching now uses `IsItemDoneFromMaps` (a clean pattern that should be replicated wherever `IsItemDone` callers can supply pre-loaded maps)
- All major feed read paths use `QueryByFieldIn` consistently (see `server/services/feed/service.go:320-449`)

**Trend**: Improving — the post-review remediation cycle is fast and the new Critical surfaced this run is a *narrower* scope than what was filed last week (single load pattern in two handlers vs. systemic N+1 fan-out). Net trajectory is positive.

---

## Critical Issues

### 1. ListConversations and GetUnreadCounts Use Unbounded `ListAll(ChatConversation{})`

**Location**: `server/services/chat/conversations.go:392`, `server/services/chat/unread_counts.go:35`

**Description**: Both inbox endpoints fetch the *entire* `chat_conversation` table on every call and filter to the caller's conversations in Go. The previous-review N+1 fix made the per-conversation work efficient, but the upstream load was not addressed:

```go
// conversations.go:392
allConversations, err := s.storage.ListAll(ctx, &models.ChatConversation{})
// then in-memory: for _, conversation := range allConversations { if isParticipant ... }
```

A `// TODO: This requires a database query that can handle array field matching` comment acknowledges the gap, but the simple fix (query `chat_conversation_user` join table or add a participant index) has not been done. `ListAll` *does* support `QueryOptions{Limit:N}`, but neither caller passes one.

**Impact**: Every inbox load and every unread-count poll scans the full conversations table. With 10K conversations across all users, every poll fetches 10K rows; with 100K, 100K rows. `GetUnreadCounts` is typically called on app foreground / push refresh, so the request rate is at least once per active session. This dominates DB CPU as conversations grow and is a classic O(total_conversations × active_users) hot path.

**Recommendation**: Either (a) add a `chat_conversation_participant` join table (or denormalize `participant_ids` into a normalized side table) and query by `user_id`, or (b) add a JSONB GIN index on `participant_ids` and use array containment (`participant_ids @> ARRAY[$1]`). Add `AssertMaxQueries(t, ctx, 6, ...)` test plus a row-count assertion to lock the fix in place.

---

## High Priority

### 2. ListExperiences / ListMyExperiences Call `buildAPIExperience` In a Loop With Per-Experience Queries

**Location**: `server/services/experience/service.go:156-307`, called from `server/services/experience/get.go:183`, `server/services/experience/get.go:225`

**Description**: `buildAPIExperience` issues per-experience queries:
- `services.FetchAPIUser(ctx, s.storage, exp.OwnerId)` — owner lookup
- `chat.GetConversationMessageCount(...)` — message count query
- `s.storage.QueryByField("experience_id", ...)` — community experiences
- `s.storage.GetByID(ctx, exp.LocationId, loc)` — location
- `storage.QueryByFields(... "experience_id", exp.Id, ...)` — RSVPs
- `s.buildTimeProposals(ctx, exp.Id)` — proposals
- `conversation.EnrichWithCommentPreview(...)` — preview

That's roughly 7 queries per experience. `ListMyExperiences` calls this in a loop (`get.go:222`) for every experience the user owns; `ListExperiences` calls it for every experience in a community page. A user with 20 experiences = ~140 queries.

**Impact**: Both endpoints are user-facing list views. Users with active community participation (>10 experiences) routinely incur 70+ queries per call. The portfolio `fetchAll` shows the right pattern: pre-collect IDs, batch-fetch owners/locations/RSVPs/community-experiences/conversations once, then build response from the maps. This was last review's Issue 15 (Low) but the impact was under-rated; treating it as High here.

**Recommendation**: Add `buildAPIExperiences` (plural) that pre-fetches all owners, locations, community-experience records, RSVPs, and conversation message counts using `FetchAPIUsersBatch` / `GetByIDs` / `QueryByFieldIn`. Wire `ListExperiences` and `ListMyExperiences` to it, and add an `AssertMaxQueries` test (target ≤8 for any list size).

### 3. Embedding Backfill Goroutine Uses `context.Background()`

**Location**: `server/main.go:567`

**Description**:
```go
go sqlStorage.BackfillEmbeddings(context.Background())
```

The two other backfill goroutines (impact + community cascade) were converted to use the root `ctx` so they cancel on shutdown. Embedding backfill was not converted in the same pass. `BackfillEmbeddings` is a long-running job that calls an external embedding model — on shutdown, in-flight requests will continue past the 10s shutdown drain and may produce work the server can no longer consume.

**Impact**: Goroutine outlives shutdown. If the embedder API call stalls, the process can't terminate cleanly. Cloud Run / orchestrators escalate to SIGKILL after the grace window.

**Recommendation**: One-line change — pass `ctx` (the root cancel context) instead of `context.Background()`. Internal loop already needs to honor `ctx.Done()` — verify this and add an early-exit check.

---

## Medium Priority

### 4. `ref.listen` Inside `build()` Has Spread to Four Content Views

**Location**: `app/lib/presentation/screens/gear/gear_content_view.dart:310`, `experience/experience_content_view.dart:414`, `communities/community_content_view.dart:222`, `request/request_content_view.dart:370`

**Description**: Last review flagged this in two screens; it has now spread to four. Each `build()` invocation registers a fresh `ref.listen` callback, so the nav-lock/unlock side effect fires multiple times per state change as listeners stack with each rebuild. The fact that `ref.listenManual` is used elsewhere in the same files (with proper `initState` registration) suggests the team knows the right pattern but hasn't backported it to the showFeedHeader branch.

**Impact**: Visible during animations — rapid nav lock/unlock toggling. Memory grows with rebuild count for the screen lifetime. Bug pattern is silent (no dev-mode warning) and easy to ship.

**Recommendation**: Move all four `ref.listen` calls to `initState` using `ref.listenManual` (already imported and used in these files for other listeners). Add a custom_lint rule banning `ref.listen` inside `build()` to prevent recurrence.

### 5. Other Aging Items (Carry-Over)

The following remain from prior reviews and are not separately re-described:
- `time.After` inside per-subscriber broadcast `select` loops (chat + community streaming) — pre-allocate `time.NewTimer`
- MessageList eager `ListView` — switch to `ListView.builder`
- `RecordAttendance` multi-QueryByFields + per-record Insert/Update in loop — pre-collect, batch-fetch RSVPs once, batch-update
- `IncrementViewCountBatch` per-item `Update` inside loop — accumulate updates, then single `UpdateBatch` (does not exist yet; would unlock other call sites)
- Streaming channel buffer of 10 (chat + community) — sized for short bursts only; sustained slow consumers fall back to the 1s timeout

## Low Priority

- `QueryByProximity` bubble sort (`server/storage/protosql_spatial.go:95-101`) — replace with `sort.Slice`
- User presence map sets `false` instead of `delete` (`server/services/chat/streaming.go:182`) — single-line fix

## Positive Findings

### Fast Remediation Velocity

Eight of ten previously-filed Critical/High issues were closed within four days. The fixes are clean and use the existing batch primitives (`QueryByFieldIn`, `GetByIDs`, `FetchAPIUsersBatch`) consistently — no new ad-hoc patterns introduced.

### `IsItemDoneFromMaps` Pattern

`server/conversation/state.go` and its callers (`conversations.go:489`, `unread_counts.go:110`, `portfolio/fetch.go:647`) demonstrate the right way to refactor an `IsItemDone(ctx, store, conv)` API into a `IsItemDoneFromMaps(conv, transferMap, reqMap)` API. This pattern should be replicated for any helper that does a hidden `GetByID` and is called inside a loop.

### Portfolio fetchAll, Feed Pagination, Fanout — All Holding Up

The exemplars from last review (`server/services/portfolio/fetch.go`, `server/services/feed/service.go`, `server/services/fanout/fanout.go`) remain the gold-standard patterns and have not regressed.

### HTTP Server Timeouts

Now configured (`ReadHeaderTimeout: 10s`, `ReadTimeout: 30s`, `WriteTimeout: 120s`, `IdleTimeout: 120s`) — protects against slowloris and bounds connection lifetime.

---

## Recommendations

### Immediate (This Sprint)

1. **Fix `ListAll(ChatConversation{})` in inbox handlers** — The single highest-impact perf fix available. Add a participant index or join table; cap with `AssertMaxQueries`. (Critical)
2. **Wire embedding backfill to root `ctx`** — One-line fix for shutdown cleanliness. (High)

### Short Term (Next Sprint)

3. **Build `buildAPIExperiences` (plural) batch path** — Convert `ListExperiences` and `ListMyExperiences`. (High)
4. **Consolidate `ref.listen`-in-build()** — Move all four sites to `initState`/`listenManual` and add a lint rule. (Medium, escalates if it spreads further)
5. **Add Limit option to `QueryByField` / `QueryByFields`** — Mirror `ListAll`'s existing support; apply at the highest-volume call sites. (High, 50+ days aging)

### Medium Term (Next Quarter)

6. Pre-allocated `time.NewTimer` in broadcast loops; `ListView.builder` for `MessageList`; batch `RecordAttendance`; bounded `IncrementViewCountBatch`. All medium-priority cleanups.

---

## Metrics to Monitor

1. **Conversations table row count vs. inbox endpoint latency** — these will be linked until issue #1 lands. Track full-table-scan plan in `pg_stat_statements` for `chat_conversation`.
2. **Per-request query count** for `ListExperiences`, `ListMyExperiences` — should drop from O(n) to O(1) after batch refactor.
3. **Goroutine count at server shutdown** — confirms embedding backfill goroutine respects cancellation.

---

## Issues to File

### [Critical] ListConversations and GetUnreadCounts use unbounded ListAll(ChatConversation)

- **Fingerprint**: `review:performance:listall-chat-conversations`
- **Type**: Bug
- **Why it matters**: Both inbox handlers fetch every row of the global `chat_conversation` table on every request, then filter to the caller's conversations in memory. Cost scales with total platform conversations × active session refresh rate, not per-user activity. This is the single largest scaling cliff remaining in the server.
- **Locations**: `server/services/chat/conversations.go:392`, `server/services/chat/unread_counts.go:35`
- **Recommendation**: Add a `chat_conversation_participant` join table (or denormalize) and query by `user_id`; alternatively add a JSONB GIN index on `participant_ids` and switch to array containment. Add `AssertMaxQueries` and a row-count assertion to lock in. Both handlers share the fix.
- **Suggested labels**: `performance`

### [High] buildAPIExperience runs per-experience queries inside list endpoints

- **Fingerprint**: `review:performance:n+1-build-api-experience`
- **Type**: Bug
- **Why it matters**: `buildAPIExperience` issues ~7 queries (owner, conversation message count, community experiences, location, RSVPs, time proposals, comment preview) and is called in a loop by `ListExperiences` and `ListMyExperiences`. A user with 20 experiences triggers ~140 queries per list call.
- **Locations**: `server/services/experience/service.go:156-307`, `server/services/experience/get.go:183`, `server/services/experience/get.go:225`
- **Recommendation**: Introduce a `buildAPIExperiences` (plural) batch path mirroring portfolio `fetchAll`: pre-collect owner/location IDs and experience IDs, batch-fetch all dependencies once, then build responses from the maps. Add `AssertMaxQueries(t, ctx, 8, ...)` to lock it.
- **Suggested labels**: `performance`

### [High] Embedding backfill goroutine ignores shutdown cancellation

- **Fingerprint**: `review:performance:embedding-backfill-context-background`
- **Type**: Task
- **Why it matters**: While the impact and community-cascade backfill goroutines were migrated to use the root `ctx`, `BackfillEmbeddings` still uses `context.Background()`. On shutdown, in-flight embedder API calls survive past the 10s drain and risk SIGKILL escalation; also leaks goroutines during dev hot-restart.
- **Locations**: `server/main.go:567`
- **Recommendation**: Pass the root `ctx` (already declared at `server/main.go:496`) instead of `context.Background()`. Verify `BackfillEmbeddings` honors `ctx.Done()` in its loop.
- **Suggested labels**: `performance`

### [Medium] ref.listen inside build() spreading across content views

- **Fingerprint**: `review:performance:ref-listen-in-build-content-views`
- **Type**: Bug
- **Why it matters**: Each rebuild registers a new listener, stacking callbacks and causing the nav-lock side effect to fire multiple times per state change. Filed previously with two sites; now four (gear, experience, community, request). Pattern is silent — no dev-mode warning — and easy to copy into new screens.
- **Locations**: `app/lib/presentation/screens/gear/gear_content_view.dart:310`, `app/lib/presentation/screens/experience/experience_content_view.dart:414`, `app/lib/presentation/screens/communities/community_content_view.dart:222`, `app/lib/presentation/screens/request/request_content_view.dart:370`
- **Recommendation**: Migrate all four to `ref.listenManual` registered in `initState` (the same files already use `listenManual` for other listeners — use that as the template). Add a custom_lint rule banning `ref.listen` calls inside `build()` so the pattern can't spread further.
- **Suggested labels**: `performance`
