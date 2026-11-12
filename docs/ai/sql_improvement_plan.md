# Storage Layer Refactor & Optimization Plan

## Context

The storage layer (`server/storage/`) is the core database abstraction for the ripls server. It works well but has grown organically to 2,476 lines in `protosql.go` alone (2,676 in its test file), with significant code duplication in search methods and no batch read APIs. The service layer compounds this by calling storage methods inside loops, creating N+1 query patterns that multiply database round-trips. This plan addresses modularity, maintainability, and SQL efficiency.

---

## Part 1: SQL Efficiency Principles

Document these in `docs/server/sql_efficiency.md` as a living reference:

1. **Never query in a loop.** Every `for` loop containing a storage call is an N+1 bug. Use batch APIs (`GetByIDs`, `QueryByFieldIn`) or JOINs instead.
2. **Push filtering to SQL.** Use WHERE clauses, not post-query Go filtering. Every row fetched and discarded wastes I/O.
3. **Always LIMIT unbounded queries.** Any SELECT without LIMIT is a memory exhaustion risk. Even "list all" queries need a reasonable cap.
4. **Batch writes.** Use multi-row INSERT (`INSERT INTO t (a,b) VALUES ($1,$2),($3,$4),...`) instead of looping single inserts.
5. **Use JOINs for related data.** Fetch parent + children in one query rather than fetching parents then looping for children.
6. **Index what you filter on.** Every WHERE/JOIN/ORDER BY column used in production queries should have an appropriate index.
7. **Avoid function calls in WHERE.** `LOWER(col) LIKE LOWER($1)` prevents index use. Use `ILIKE` (PostgreSQL) or citext columns instead.
8. **Measure before and after.** Use query counting and EXPLAIN ANALYZE to validate that changes actually reduce load.

---

## Part 2: Profiling & Observability

### 2a. Query Counter Middleware

Add a per-request query counter by wrapping `*sql.DB` with an instrumented driver.

**New file: `server/storage/instrumented_db.go` (~80 lines)**

```go
type InstrumentedDB struct {
    *sql.DB
    queryCount *atomic.Int64  // per-request counter stored in context
}

// Context key for query stats
type queryStatsKey struct{}
type QueryStats struct {
    Count    atomic.Int64
    TotalMs  atomic.Int64
}

func WithQueryStats(ctx context.Context) context.Context  // attach stats to ctx
func GetQueryStats(ctx context.Context) *QueryStats       // retrieve stats
```

- Every `QueryContext`/`ExecContext` call increments the counter and records duration
- Log query count + total DB time at end of each RPC via existing logging middleware
- Log individual slow queries (>100ms) at WARN level

### 2b. N+1 Detection in Tests

**New file: `server/storage/query_assert.go` (~30 lines)**

```go
func AssertMaxQueries(t *testing.T, ctx context.Context, max int, fn func())
```

- Wraps a test function, counts queries executed, fails if count exceeds `max`
- Use in integration tests for critical list endpoints to prevent regressions

### 2c. Connection Pool Monitoring

Already in `CheckHealth()` — extend to export Prometheus-style metrics via the existing health endpoint (pool stats: open, in_use, idle, wait_count, wait_duration).

---

## Part 3: File Restructuring

Split `protosql.go` (2,476 lines) into focused modules. No behavior changes — pure code movement.

### Target Structure

| New File | Contents | Est. Lines |
|----------|----------|------------|
| `protosql.go` | ProtoSQLStorage struct, Insert, GetByID, ListAll, QueryByField, QueryByFields, Update, Delete, DeleteByField, DeleteByFieldIn, QueryDistinctField, CountByField, HasAnyUsers, Close, CheckHealth | ~550 |
| `protosql_types.go` | Types (TypeConfig, QueryOptions, EmbeddingFieldConfig, columnDefinition, flattenedField, SpatialQueryResult, GearSearchResult, RequestSearchResult, UnifiedSearchResult), constants, computeCompositeScore | ~150 |
| `protosql_schema.go` | flattenFields, extractFieldValues, getExpectedColumns, getMissingColumns, generateCreateTableSQL, generateAlterTableAddColumnSQL, sanitizeColumnName, detectGeospatialFields, generateGeospatialIndexSQL, initializeDatabase, DefaultStorageTypes, buildDeletedFilter, hasDeletedField | ~350 |
| `protosql_search.go` | QueryGearByCommunitySearch, semanticGearSearch, textGearSearch, QueryRequestByCommunitySearch, semanticRequestSearch, textRequestSearch, IsGearInCommunity | ~600 |
| `protosql_unified_search.go` | QueryCommunitySearch, semanticUnifiedSearch, textUnifiedSearch, searchGearSemantic, searchRequestSemantic, searchExperienceSemantic, searchGearText, searchRequestText, searchExperienceText | ~500 |
| `protosql_spatial.go` | QueryByProximity, QueryByProximityWithJoin | ~200 |

Split test files to match: `protosql_test.go`, `protosql_schema_test.go`, `protosql_search_test.go`, `protosql_unified_search_test.go` (this one already exists), `protosql_spatial_test.go`.

**Key rule:** All files in same package (`storage`), all methods on `*ProtoSQLStorage`. Just file boundaries change.

---

## Part 4: Batch APIs

### 4a. GetByIDs — Batch Read

**Add to `protosql.go`:**

```go
func (s *ProtoSQLStorage) GetByIDs(ctx context.Context, ids []string, msgType proto.Message, opts ...QueryOptions) (map[string]proto.Message, error)
```

- Generates: `SELECT id, binary_proto FROM "table" WHERE id IN ($1,$2,...) AND <deleted_filter>`
- Returns map[id]→message for easy lookup
- Handles empty `ids` gracefully (returns empty map)
- Chunks into batches of 500 if needed (PostgreSQL parameter limit)

### 4b. QueryByFieldIn — Batch Field Query

**Add to `protosql.go`:**

```go
func (s *ProtoSQLStorage) QueryByFieldIn(ctx context.Context, fieldName string, values []string, msgType proto.Message, opts ...QueryOptions) ([]proto.Message, error)
```

- Generates: `SELECT binary_proto FROM "table" WHERE field IN ($1,$2,...) AND <deleted_filter>`
- Similar chunking for large value lists

### 4c. InsertBatch — Bulk Insert

**Add to `protosql.go`:**

```go
func (s *ProtoSQLStorage) InsertBatch(ctx context.Context, msgs []proto.Message) ([]string, error)
```

- Generates multi-row INSERT: `INSERT INTO "table" (cols...) VALUES ($1,...),($n+1,...)`
- All messages must be same type
- Chunks into batches of 100 rows
- Returns slice of generated IDs

### 4d. FetchAPIUsersBatch

**Modify `server/services/user_util.go`:**

```go
func FetchAPIUsersBatch(ctx context.Context, s *storage.ProtoSQLStorage, userIDs []string) (map[string]*api.User, error) {
    // Deduplicate IDs
    // Call s.GetByIDs(ctx, uniqueIDs, &models.User{})
    // Convert to map[id]*api.User
}
```

This is the single highest-leverage change — `FetchAPIUser` is the common denominator in nearly every N+1 loop across all services.

---

## Part 5: Search Code Deduplication

The 12 search functions (6 per-type semantic/text + 6 unified semantic/text) follow identical patterns differing only in: table names, join column, filter clauses, and result type assignment. Deduplicate with a config-driven generic function.

**New file: `protosql_search_generic.go` (~200 lines)**

```go
type communitySearchConfig struct {
    itemType       string                          // "gear", "request", "experience"
    entityTable    string                          // main table
    joinTable      string                          // community junction table
    joinColumn     string                          // "gear_id", "request_id", "experience_id"
    locationTable  string                          // location table
    extraFilters   string                          // e.g., "AND cr.archived = false AND r.state IN ('1','2')"
    searchColumns  []string                        // ["name","description"] or ["title","description"]
    extraSelectCols string                         // e.g., ", cg.availability"
    orderByColumn  string                          // for text search recency, e.g., "cg.created_at_unix_sec"
    textLimit      int                             // 0 = unlimited, 100 for gear
    buildResult    func(proto, loc, extra) UnifiedSearchResult  // result builder
}
```

- `executeSemanticSearch(ctx, cfg, colName, vectorStr, communityID, userLat, userLon) ([]UnifiedSearchResult, error)`
- `executeTextSearch(ctx, cfg, searchQuery, communityID, userLat, userLon) ([]UnifiedSearchResult, error)`

The per-type `QueryGearByCommunitySearch` / `QueryRequestByCommunitySearch` methods become thin wrappers calling the generic functions. The 6 unified `searchXxxSemantic`/`searchXxxText` helpers reduce to 3 config structs passed to the generic functions.

**Estimated reduction:** ~800 lines removed, ~200 lines added = net -600 lines.

---

## Part 6: Equity Batch Optimization

**Modify `server/storage/equity.go`:**

1. **True batch inserts:** Replace the 6 `BatchInsertXxxEquity` functions (which loop calling `Insert()`) with actual multi-row INSERT using `InsertBatch` from Phase 4c.

2. **Single COUNT query for CountEquityRecords:** Replace 6 sequential COUNT queries with:
   ```sql
   SELECT
     (SELECT COUNT(*) FROM community_equity WHERE community_id = $1) +
     (SELECT COUNT(*) FROM user_equity WHERE community_id = $1) + ...
   ```

---

## Part 7: Service-Layer N+1 Elimination

Refactor the worst N+1 offenders using the batch APIs from Part 4. Listed by impact.

### 7a. FetchAPIUser Loops (all services)

**Files:** `search/service.go`, `transfer/queries.go`, `request/helpers.go`, `experience/service.go`, `feed/service.go`, `chat/messages.go`

**Pattern:** Collect all user IDs upfront → single `FetchAPIUsersBatch` call → lookup from returned map.

Example in `transfer/queries.go`:
```go
// Before: per-transfer GetByID
for _, t := range transfers { buildTransfer(ctx, t) } // 3 queries each

// After: batch prefetch
userIDs := collectUserIDs(transfers) // owner + recipient IDs
users, _ := services.FetchAPIUsersBatch(ctx, s.storage, userIDs)
for _, t := range transfers { buildTransferFromCache(ctx, t, users) }
```

### 7b. Search Service Exact-Match Methods

**File:** `server/services/search/service.go`

`searchGearExact`, `searchRequestsExact`, `searchExperiencesExact` each loop with `GetByID` + `FetchAPIUser` per item.

**Fix:** For each, collect all entity IDs from the junction table query, then `GetByIDs` in one call, then `FetchAPIUsersBatch` for all owner IDs.

### 7c. Request/Experience List Methods

**Files:** `request/queries.go`, `experience/get.go`

`ListRequests`, `ListMyRequests`, `ListExperiences` fetch each entity individually.

**Fix:** Same pattern — batch prefetch entities + users + locations.

### 7d. Feed Generation

**File:** `feed/service.go`

`generateFeedItems` calls `eventToFeedItem` per event (each does multiple GetByID calls).

**Fix:** Pre-collect all referenced entity IDs from events, batch fetch, then build items from cache.

---

## Implementation Phases

### Phase 1: Restructure + Profiling (no behavior change) ✅
- [x] Split `protosql.go` into 6 files
- [x] Add `instrumented_db.go` and `query_assert.go`
- [x] Wire query counting into RPC logging via `QueryStats` middleware
- [x] Write `docs/server/sql_efficiency.md`
- [x] Define `SQLDB` interface, wrap `*sql.DB` with `InstrumentedDB` for automatic per-request query counting
- **Verify:** `go test ./server/storage/...` passes, all existing tests unchanged ✅

### Phase 2: Batch APIs ✅
- [x] Add `GetByIDs`, `QueryByFieldIn`, `InsertBatch` to storage (`protosql_batch.go`)
- [x] Unit tests for batch methods (`protosql_batch_test.go`: 15 tests)
- [x] Add `FetchAPIUsersBatch` to services (`user_util.go`: single batch query, dedup, map return; `user_util_test.go`: 11 tests)
- [x] Refactor `FetchAPIUsers` to use `FetchAPIUsersBatch` internally (preserves order + duplicates)
- **Verify:** New tests pass, existing tests unchanged (no callers modified yet) ✅

### Phase 2.5: Performance Baseline via Simulation

Establish query-count baselines **before** any N+1 optimization, using the
simulation infrastructure (see `docs/simulation.md`). The `QueryStats` middleware
already logs `db_queries` and `db_duration_ms` per request.

**Script:** `scripts/sql_profile.sh`

Automates the full cycle: builds the server, starts it with JSON logging and
`--dev-mode`, runs the `college-friends` simulation scenario (deterministic,
seed=42), extracts per-endpoint query stats from the server log, and prints a
summary table.

```bash
# Print summary to terminal
./scripts/sql_profile.sh

# Also write a markdown report (for checking in or comparing)
./scripts/sql_profile.sh -o docs/ai/sql_profile.md
```

After each optimization phase, re-run the script and compare results.

**Key endpoints to watch:**
- `ListConversations` — FetchAPIUsers N+1 on participants
- `Search` / `SearchCommunity` — per-result GetByID + FetchAPIUser
- `GetFeed` — per-event entity + user lookups
- `ListCommunityGear` — per-item owner lookup
- `ListRequests` / `ListMyRequests` — per-request requester lookup
- `ListExperiences` — per-experience owner lookup
- `GetGearPeople` — per-transfer recipient lookup

### Phase 3: Search Deduplication ✅
- [x] Define `communitySearchConfig` struct and 5 config definitions (gear, request, unified gear/request/experience)
- [x] Implement generic `executeSemanticSearch` and `executeTextSearch` executors with `scanSearchRow` helper
- [x] Refactor per-type methods (`QueryGearByCommunitySearch`, `QueryRequestByCommunitySearch`) via `searchWithFallback`
- [x] Refactor unified methods (`QueryCommunitySearch`, `semanticUnifiedSearch`, `textUnifiedSearch`) to loop configs
- [x] Merge `protosql_unified_search.go` into `protosql_search.go` (1,237 → 429 lines, net -808 lines)
- **Verify:** All storage + search service tests pass with identical behavior ✅

### Phase 4: N+1 Elimination (highest impact) ✅
- [x] Add slow query logging (>100ms at WARN) in `InstrumentedDB` (Part 2a gap)
- [x] Refactor search service exact-match methods (collect IDs → `GetByIDs` + `FetchAPIUsersBatch`)
- [x] Refactor transfer `buildTransfer` → `buildTransfers` batch method
- [x] Refactor request list methods (`ListRequests`, `ListMyRequests` → `GetByIDs` batch)
- [x] Refactor experience list methods (`ListExperiences` → `GetByIDs` batch, `buildRSVPs` → `FetchAPIUsersBatch`)
- [x] Refactor feed generation (pre-fetch users, gear, requests, experiences via `feedLookups` struct)
- [x] Refactor chat `ListConversations` (pre-fetch all participant users with `FetchAPIUsersBatch`)
- [x] Refactor gear `GetGearPeople` (batch fetch recipient users)
- **Verify:** All service tests pass ✅

### Phase 5: Equity & Polish ✅
- [x] True batch inserts in equity (6 BatchInsertXxx functions → InsertBatch multi-row INSERT)
- [x] Single COUNT query for `CountEquityRecords` (5 queries → 1 subquery expression)
- [x] Add LIMIT to `ListAll` (optional `QueryOptions.Limit` parameter)
- [x] Replace `LOWER(col) LIKE LOWER($1)` with `ILIKE` in text search
- [x] Add missing indexes (equity tables: 6 indexes, story table: 1 index) via schema init
- [x] Extend `CheckHealth` pool stats with `wait_count`, `wait_duration_ms` (Part 2c gap)
- **Verify:** All storage, equity, admin, and search tests pass ✅

### Phase 6: Mutation Path Optimization ✅

#### Profiling Summary

| Milestone | Total Queries | Delta |
|-----------|--------------|-------|
| Baseline (Phase 2.5) | 18,422 | — |
| Phase 4 (N+1 elimination) | 18,318 | -0.6% |
| Phase 5 (equity & polish) | 18,315 | -0.6% |
| Phase 6 (mutation optimization) | 17,517 | -4.4% |

Phases 4–5 primarily improved read/list endpoints. The simulation is mutation-heavy (~90% of queries), so the total barely moved. The top 7 mutation endpoints account for **14,897 of 18,315 queries (81%)**:

| Endpoint | Calls | Total Q | Avg/Call |
|----------|------:|--------:|--------:|
| RSVPToExperience | 116 | 3,441 | 29.7 |
| CompleteTransfer | 56 | 2,501 | 44.7 |
| StartLoan | 49 | 1,788 | 36.5 |
| ExpressInterest | 65 | 1,622 | 25.0 |
| CompleteExperience | 36 | 1,372 | 38.1 |
| OfferToFulfill | 38 | 1,172 | 30.8 |
| MarkRequestFulfilled | 18 | 876 | 48.7 |

Profile reports: `docs/ai/sql_profile_phase5.md` (current), `docs/ai/sql_profile_phase4.md`, `docs/ai/sql_baseline.md`

#### Hotspot Analysis

Each mutation workflow is a pipeline of: **state transition → equity scoring → impact estimation → story generation → system messages → response building**. The redundancy patterns fall into 4 categories:

**1. Redundant entity re-fetches (all mutation endpoints, ~4-6 wasted queries/call)**

The state machine pattern is: caller fetches entity → `transitionTransferState` re-fetches via GetByID → modifies → Update → caller re-fetches again for impact/response. The same entity gets loaded 2-3×. Similarly, Gear is fetched separately by the state machine (`markGearState`), impact estimation, and story generation.

- CompleteTransfer: Transfer fetched 3×, Gear fetched 3×
- StartLoan: Transfer fetched 2×, Gear fetched 2×, CommunityGear looked up 2×
- CompleteExperience: CommunityExperience queried 3-4× (buildAPIExperience, impact, story, archival), ExperienceRSVP queried 2-3×
- MarkRequestFulfilled: CommunityRequest fetched 2-3×
- RSVPToExperience: CommunityExperience fetched 2-3×

**Fix:** Pass already-fetched entities through the call chain. State machine functions accept the pre-loaded entity + return the updated version. Impact/story functions receive pre-fetched gear/users/communities.

**2. Unbatched user fetches in story generation (~6-10 wasted queries/call)**

Story generation helpers (`generateStoryForCompletedTransfer`, `generateStoryForFulfilledRequest`, `generateStoryForCompletedExperience`) fetch users one-by-one with GetByID loops instead of FetchAPIUsersBatch. For experiences with N attendees or requests with N helpers, this is O(N) individual queries.

**Fix:** Collect all user IDs upfront → single FetchAPIUsersBatch → pass user map to story builder.

**3. Per-community fan-out (RSVPToExperience, OfferToFulfill, ~2-4 wasted queries/call)**

When an entity is shared in multiple communities, operations like AddParticipantToConversation and InsertSystemMessage run per-community, each doing their own GetByID+Update on ChatConversation.

**Fix:** Batch conversation updates when adding participants to multiple conversations simultaneously.

**4. buildAPIExperience/buildRequest/buildTransfer in mutation responses (~4-8 queries/call)**

After mutations, the response builder re-fetches related entities (communities, RSVPs, users, message counts) that were already loaded during the mutation itself. The mutation and response paths share no context.

**Fix:** Have mutation functions return already-fetched context that the response builder can reuse, or cache fetched entities in a per-request context map.

#### Implementation Plan

- [x] Audit and fix entity re-fetch chains in state machine functions (category 1)
- [x] Batch user fetches in all story generation helpers (category 2)
- [ ] Batch conversation updates for multi-community fan-out (category 3) — deferred
- [ ] Thread pre-fetched entities from mutation path into response builders (category 4) — deferred
- [x] Re-profile and compare to 18,315 baseline

#### Phase 6 Results (17,517 total, -4.4%)

| Endpoint | Phase 5 Avg | Phase 6 Avg | Change |
|----------|------------|------------|--------|
| CompleteTransfer | 44.7 | 39.8 | -11% |
| CompleteExperience | 38.1 | 32.9 | -14% |
| StartLoan | 36.5 | 33.5 | -8% |
| ExpressInterest | 25.0 | 24.0 | -4% |
| RSVPToExperience | 29.7 | 28.9 | -3% |
| MarkRequestFulfilled | 48.7 | 47.1 | -3% |

Changes made:
1. State machine pass-through: `transitionTransferState` accepts pre-loaded transfer, returns updated transfer+gear
2. Batch user fetches in story generation (transfer, request, experience)
3. CommunityExperience dedup + getUserDisplayName hoisting
4. Batch participant fetches in story listings (feed + user)
5. ExpressInterest: `createNewTransfer` returns entity instead of re-fetching
6. Merged two Update calls into one in CompleteExperience

Profile report: `docs/ai/sql_profile_phase6.md`

---

## Files Modified/Created Summary

**New files:**
- `server/storage/protosql_types.go`
- `server/storage/protosql_schema.go`
- `server/storage/protosql_search.go` (merged: per-type + unified + generic config-driven search)
- `server/storage/protosql_spatial.go`
- `server/storage/instrumented_db.go`
- `server/storage/query_assert.go`
- `server/storage/protosql_schema_test.go`
- `server/storage/protosql_search_test.go`
- `server/storage/protosql_spatial_test.go`
- `docs/server/sql_efficiency.md`

**Modified files:**
- `server/storage/protosql.go` (shrinks from ~2,476 to ~550 lines)
- `server/storage/protosql_test.go` (shrinks from ~2,676 to ~800 lines)
- `server/storage/equity.go` (batch insert + count optimization)
- `server/services/user_util.go` (add FetchAPIUsersBatch)
- `server/services/search/service.go` (eliminate N+1)
- `server/services/transfer/queries.go` (eliminate N+1)
- `server/services/request/queries.go` + `request/helpers.go` (eliminate N+1)
- `server/services/experience/get.go` + `experience/service.go` (eliminate N+1)
- `server/services/feed/service.go` (eliminate N+1)

## Verification

After each phase:
1. `go test ./server/storage/...` — all storage tests pass
2. `go test ./server/services/...` — all service tests pass
3. `go test ./server/jobs/...` — all job tests pass
4. After Phase 1: verify query count logging appears in RPC logs
5. After Phase 4: verify query counts dropped (use AssertMaxQueries in tests)
