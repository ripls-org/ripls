# Server Profiling, Load Testing & Performance Optimization Plan

## Context

The Ripls server has solid query-level instrumentation from the SQL refactor
(Phase 1–6), but lacks runtime profiling and realistic load testing. The
simulation framework only exercises write-heavy mutation flows — it doesn't
simulate the read-heavy browsing, searching, and feed-checking patterns that
dominate real-world client traffic. This plan adds:

1. **Server profiling endpoints** (pprof) to identify CPU, memory, and latency
   bottlenecks
2. **Comprehensive load testing** by extending the simulation framework with
   realistic read/write patterns and concurrent user simulation
3. **A systematic methodology** for using 1 + 2 to find and fix performance
   issues

### What Already Exists

| Component | Location | What It Does |
|-----------|----------|-------------|
| `InstrumentedDB` | `server/storage/instrumented_db.go` | Per-request query counting + slow query logging (>100ms) |
| `QueryStats` middleware | `server/middleware/query_stats.go` | Attaches query stats to context, logs `db_queries`/`db_duration_ms` per request |
| `AssertMaxQueries` | `server/storage/query_assert.go` | Test helper to prevent N+1 regressions |
| `sql_profile.sh` | `scripts/sql_profile.sh` | Runs simulation, extracts per-endpoint query stats from logs |
| Simulation framework | `server/simulation/` | Deterministic timeline of write-heavy multi-user workflows |
| Health endpoint | `/health` + `HealthService/CheckHealth` | DB pool stats, dependency health checks |

### What's Missing

- **No CPU/memory profiling** — can't see where compute time or allocations go
- **No read traffic simulation** — Search, GetFeed, ListStories, ListCommunityGear, GetUser, etc. never called
- **No concurrent load** — simulation runs sequentially, can't test contention or pool exhaustion
- **No trend tracking** — each profiling run is a snapshot with manual comparison
- **No production traffic data** — read/write distribution assumptions are guesses, not measured

---

## Part 0: Production Traffic Analysis

Before designing simulation read weights, analyze real traffic from Cloud Run
logs to understand actual client behavior patterns.

### 0a. Extract Cloud Run Request Logs

Pull structured logs from `ripls-dev` and `ripls-prod` Cloud Run services to
see which RPC methods are called, how often, and their latency distribution.

```bash
# Export last 7 days of request logs from dev (adjust date range as needed)
gcloud logging read \
  'resource.type="cloud_run_revision" AND resource.labels.service_name="ripls-server" AND jsonPayload.method!=""' \
  --project=ripls-dev \
  --format=json \
  --freshness=7d \
  --limit=50000 \
  > /tmp/cloud-run-dev-logs.json

# Same for prod
gcloud logging read \
  'resource.type="cloud_run_revision" AND resource.labels.service_name="ripls-server" AND jsonPayload.method!=""' \
  --project=ripls-prod \
  --format=json \
  --freshness=7d \
  --limit=50000 \
  > /tmp/cloud-run-prod-logs.json
```

The exact filter will depend on what fields the structured logger emits — we
may need to adjust to match the `path` field from the logging middleware (e.g.,
`httpRequest.requestUrl` or `jsonPayload.path`).

### 0b. Analyze Traffic Distribution

Write a script (`scripts/analyze_cloud_logs.sh`) that processes the exported
logs and produces:

1. **RPC method frequency** — calls per method, sorted by count
2. **Read vs write ratio** — classify each RPC as read/write, compute ratio
3. **Per-method latency percentiles** — p50, p95, p99 from Cloud Run's
   `httpRequest.latency` or the server's `duration_ms` field
4. **Peak concurrency** — requests per second over time (to calibrate
   concurrent simulation)
5. **User session patterns** — for a given `user_id`, what's a typical
   sequence of RPC calls? (helps design contextual reads)
6. **Error rates** — which endpoints fail most often?

**Example output:**

```
RPC Method                  Calls   %Total  Read/Write  p50ms  p95ms
─────────────────────────── ─────── ─────── ────────── ────── ──────
GetFeed                     1,247   18.2%   READ       45     210
ListConversations             891   13.0%   READ       32     150
ListCommunityGear             634    9.3%   READ       28     120
Search                        412    6.0%   READ       180    890
GetUnreadCounts               398    5.8%   READ       12      45
GetGear                       312    4.6%   READ       22      95
SendMessage                   289    4.2%   WRITE      18      65
ExpressInterest               187    2.7%   WRITE      85     340
...
Read/Write ratio: 72% / 28%
Peak RPS: 12.4 (Tuesday 6pm)
```

### 0c. Calibrate Simulation Read Weights

Use the production traffic analysis to set realistic persona read weights:

- **Overall read/write ratio** informs the `ReadBehavior` frequencies vs.
  existing write activity rates
- **Per-endpoint frequency** tells us which read actions need highest weight
  (likely GetFeed > ListConversations > ListCommunityGear > Search)
- **Session sequences** tell us which contextual reads to pair with writes
- **Peak RPS** tells us what concurrency level to target in `load-test-medium`

This replaces the initial guess-based weights in Part 2b with data-driven
values.

### 0d. Deliverables

- [ ] `scripts/analyze_cloud_logs.sh` — Log analysis script
- [ ] `docs/ai/production_traffic_analysis.md` — Traffic distribution report
- [ ] Updated persona read weights in Part 2b based on findings

---

## Part 1: Server Profiling & Observability

### 1a. pprof Endpoints (Dev-Mode Only)

Go's `net/http/pprof` package provides CPU, memory, goroutine, mutex, and block
profiling with zero application changes. We gate it behind `--dev-mode`.

**Modify `server/main.go`:**

```go
import _ "net/http/pprof"  // registers /debug/pprof/* handlers

// In server setup, when devMode is true:
if cfg.devMode {
    mux.Handle("/debug/pprof/", http.DefaultServeMux)
}
```

This exposes:

| Endpoint | Purpose |
|----------|---------|
| `/debug/pprof/profile?seconds=30` | CPU profile (flame graph) |
| `/debug/pprof/heap` | Heap allocations |
| `/debug/pprof/goroutine` | Active goroutines + stack traces |
| `/debug/pprof/mutex` | Mutex contention |
| `/debug/pprof/block` | Blocking operations |
| `/debug/pprof/allocs` | Allocation counts (since start) |
| `/debug/pprof/trace?seconds=5` | Execution trace (scheduler, GC, etc.) |

**Usage:**
```bash
# CPU flame graph (captures 30s, opens in browser)
go tool pprof -http=:8081 http://localhost:8080/debug/pprof/profile?seconds=30

# Heap snapshot
go tool pprof -http=:8081 http://localhost:8080/debug/pprof/heap

# Goroutine dump (text)
curl http://localhost:8080/debug/pprof/goroutine?debug=1
```

**Estimated effort:** ~10 lines in `main.go`.

### 1b. Enhanced Query Stats Logging

Extend the existing `QueryStats` middleware to also log:

- **Request wall-clock duration** (already logged, but add to structured fields)
- **RPC method name** as a structured field (currently logs only URL path)
- **Response size** (bytes written)

This improves the log-based analysis in `sql_profile.sh` and provides per-
endpoint latency data without additional dependencies.

**Estimated effort:** ~20 lines in `middleware/logging.go`.

### Future Work: Prometheus Metrics

When deeper continuous monitoring is needed, add `prometheus/client_golang` to
expose request latency histograms, error rate counters, and DB query gauges at
a `/metrics` endpoint. This can be scraped by:

- **Local dev:** Prometheus on a home NAS
- **Cloud Run:** Google Cloud Managed Prometheus (GMP) sidecar, which forwards
  custom metrics to Cloud Monitoring for PromQL querying and Grafana dashboards

The server code change is the same regardless of environment — just expose
`/metrics`. The scraping infrastructure is a deployment concern. This is
deferred until pprof + log analysis proves insufficient.

---

## Part 2: Simulation Framework Extensions

### 2a. Read Action Types

Add new `Activity` constants for read-heavy operations that real users perform.

**New actions in `server/simulation/types.go`:**

```go
// Read-heavy actions (browsing, searching, checking)
ActionCheckFeed          Activity = "check_feed"
ActionSearchCommunity    Activity = "search_community"
ActionViewGearDetail     Activity = "view_gear_detail"
ActionViewExperience     Activity = "view_experience_detail"
ActionViewRequest        Activity = "view_request_detail"
ActionViewUserProfile    Activity = "view_user_profile"
ActionListCommunityGear  Activity = "list_community_gear"
ActionListTransfers      Activity = "list_transfers"
ActionCheckUnread        Activity = "check_unread"
ActionViewConversation   Activity = "view_conversation"
ActionListStories        Activity = "list_stories"
ActionViewImpact         Activity = "view_impact"
```

### 2b. Periodic Read Patterns

Model background browsing behavior — things users do independent of any
transaction flow. These fire on a per-user schedule based on persona.

**New file: `server/simulation/reads.go` (~200 lines)**

```go
// ReadBehavior defines how often a persona performs read actions per simulated week.
type ReadBehavior struct {
    FeedChecksPerWeek       float64
    SearchesPerWeek         float64
    GearBrowsesPerWeek      float64
    ProfileViewsPerWeek     float64
    StoryViewsPerWeek       float64
    ConversationChecksPerWeek float64
    ImpactViewsPerWeek      float64
}
```

**Persona-based read weights:**

| Persona | Feed | Search | Browse | Profiles | Stories | Chat | Impact |
|---------|------|--------|--------|----------|---------|------|--------|
| Alfred (Stuff×Giving) | 5 | 1 | 3 | 1 | 2 | 3 | 1 |
| Derek (Stuff×Exchange) | 7 | 3 | 5 | 2 | 3 | 4 | 1 |
| Gary (Stuff×Seeking) | 10 | 5 | 8 | 2 | 3 | 5 | 0.5 |
| Betty (Time×Giving) | 5 | 1 | 2 | 1 | 4 | 3 | 2 |
| Emma (Time×Exchange) | 7 | 2 | 3 | 2 | 3 | 4 | 1 |
| Henry (Time×Seeking) | 10 | 4 | 5 | 1 | 3 | 5 | 0.5 |

Read actions are generated into the timeline alongside write actions using the
same Poisson sampling approach. Each read action gets a jittered timestamp
within its week.

**Feed check** calls:
1. `GetFeed` (with pagination — page 1, then 20% chance page 2)
2. `MarkFeedItemsViewed` (for items returned)

**Search** calls:
1. `Search` with a query term drawn from a pool of realistic search terms
   (gear names from catalog, category keywords, experience titles)
2. 30% chance: follow up with `GetGear`/`GetRequest`/`GetExperience` on a
   result (simulates tapping a search result)

**Gear browse** calls:
1. `ListCommunityGear` (random community the user belongs to)
2. 40% chance: `GetGear` on a random item from the list
3. 20% chance: `GetGearPeople` on the same item

**Profile view** calls:
1. `GetUser` (random community member)
2. 30% chance: `GetUserStats` on same user
3. 20% chance: `ListUserStories` on same user

**Story check** calls:
1. `ListStories` (random community)
2. 30% chance: `ListItemStories` on a story's item

**Conversation check** calls:
1. `ListConversations`
2. `GetUnreadCounts`
3. If unread > 0: `GetConversationHistory` on first unread conversation

**Impact view** calls:
1. `GetCommunityImpactMetrics` (random community)
2. 30% chance: `GetCommunityMetricDetail` on a metric
3. 20% chance: `GetUserImpactMetrics`

### 2c. Contextual Reads

Weave read actions into existing write flows to simulate realistic user
journeys. Users don't jump straight to "express interest" — they browse, search,
then act.

**Modify `server/simulation/executor.go`:**

Before executing a write action, optionally prepend contextual reads:

| Write Action | Contextual Reads (probability) |
|-------------|-------------------------------|
| `ExpressInterest` | 80%: `GetGear` → `GetGearPeople` (user views item before acting) |
| `OfferToFulfill` | 80%: `GetRequest` → `GetRequestPeople` |
| `RSVPToExperience` | 80%: `GetExperience` → `GetExperiencePeople` |
| `StartLoan` | 60%: `GetTransfer` → `GetConversationForTransfer` → `GetConversationHistory` |
| `CompleteLoan` | 60%: `GetTransfer` → `GetGear` |
| `FulfillRequest` | 60%: `GetRequest` |
| `CompleteExperience` | 60%: `GetExperience` → `GetExperiencePeople` |

These reads happen with the acting user's auth token at the same simulated
timestamp (or a few minutes before) the write action.

### 2d. Concurrent User Simulation

Replace sequential timeline execution with a concurrent model where multiple
simulated users act simultaneously.

**New file: `server/simulation/concurrent.go` (~250 lines)**

**Architecture:**

```
┌─────────────────────────────────────────────────┐
│                  Orchestrator                     │
│  (groups timeline steps by simulated time window) │
└──────────────┬──────────────────┬────────────────┘
               │                  │
        ┌──────▼──────┐   ┌──────▼──────┐
        │  User Pool  │   │  User Pool  │  ...
        │  (worker)   │   │  (worker)   │
        │  goroutine  │   │  goroutine  │
        └─────────────┘   └─────────────┘
```

**Design:**

1. **Time-window grouping:** The sorted timeline is split into time windows
   (e.g., 1-minute buckets). Steps within the same window can execute
   concurrently.

2. **Per-user worker goroutines:** Each simulated user gets a dedicated
   goroutine with its own HTTP client (auth token, simulation timestamp). This
   avoids auth token contention and models real concurrent users.

3. **Concurrency control:**
   - `--concurrency N` flag controls max parallel goroutines (default: number
     of simulated users in the scenario)
   - `sync.WaitGroup` per time window — all steps in a window must complete
     before advancing to the next window
   - Shared `State` struct protected by `sync.RWMutex` for entity ID lookups

4. **Flow ordering preserved:** Steps within the same multi-step flow (e.g.,
   ExpressInterest → StartLoan → CompleteLoan) maintain causal ordering via
   the existing `ref` key system. Steps for different flows/users within the
   same time window execute concurrently.

5. **Error isolation:** A failure in one user's goroutine marks that flow as
   failed but doesn't block other users.

**CLI flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--concurrency` | `0` (= number of users) | Max concurrent user goroutines |
| `--read-traffic` | `true` | Enable periodic + contextual read patterns |
| `--read-multiplier` | `1.0` | Scale read frequency (2.0 = double reads) |

### 2e. Scenario Extensions

Add load-testing-focused scenarios optimized for profiling.

**New scenarios in `server/simulation/scenarios.go`:**

| Scenario | Communities | Members | Period | Purpose |
|----------|-------------|---------|--------|---------|
| `load-test-small` | 1 | 10 | 1 month | Quick profiling run (~2min), balanced read/write |
| `load-test-medium` | 2 | 30 | 3 months | Medium load (~10min), tests cross-community queries |
| `load-test-large` | 3 | 75 | 6 months | Heavy load (~30min), stress tests connection pool and search |
| `read-heavy` | 1 | 20 | 1 month | 5x read multiplier, minimal writes. Isolates read path performance |

### 2f. Enhanced Profiling Script

Replace `scripts/sql_profile.sh` with `scripts/profile.sh` that captures SQL
stats, request latency, and pprof profiles.

**`scripts/profile.sh` workflow:**

1. Build server with race detector optional (`--race` flag)
2. Start server in dev mode with JSON logging
3. Wait for health check
4. Start pprof CPU capture in background (`curl .../profile?seconds=<duration>`)
5. Run simulation with specified scenario and concurrency
6. Stop pprof capture, save `.prof` file
7. Capture heap profile snapshot
8. Extract per-endpoint stats from logs (existing `jq` pipeline, enhanced with
   latency percentiles from the logging middleware's `duration_ms` field)
9. Generate combined report:
   - **Request metrics:** latency p50/p95/p99 per endpoint (from structured logs)
   - **DB metrics:** queries per request, DB time per request (from structured logs)
   - **Top offenders:** sorted by total latency, total queries, error count
   - **pprof artifacts:** paths to saved CPU and heap profiles
   - **Comparison:** if a previous report exists, show deltas

**Output formats:**

```bash
# Terminal summary + markdown report
./scripts/profile.sh -o docs/ai/profile_baseline.md

# With pprof capture
./scripts/profile.sh --pprof /tmp/cpu.prof -o docs/ai/profile_baseline.md

# Specific scenario and concurrency
./scripts/profile.sh --scenario load-test-medium --concurrency 15

# Compare to previous run
./scripts/profile.sh --compare docs/ai/profile_baseline.md
```

---

## Part 3: Performance Analysis Methodology

### 3a. Baseline Capture

Before any optimization, capture baselines across multiple scenarios:

1. **Run `load-test-small`** with `--concurrency 10` (quick iteration cycle)
2. **Run `load-test-medium`** with `--concurrency 30` (realistic load)
3. **Capture pprof profiles** during both runs

**Baseline metrics to record:**

- Per-endpoint: p50, p95, p99 latency; avg queries/request; error rate
- Global: total requests, total queries, peak goroutines, peak heap, pool wait count
- pprof: top 10 CPU-hottest functions, top 10 heap allocators

### 3b. Bottleneck Identification Workflow

```
1. Run profile.sh → identify endpoints with highest p95 latency
       │
2. Check logs → is latency from DB time or Go compute?
       │
       ├── DB-heavy → check db_queries / db_duration_ms in logs
       │       │
       │       ├── High query count → N+1 pattern → batch API fix
       │       └── Low count but slow → missing index / expensive query → EXPLAIN ANALYZE
       │
       └── Compute-heavy → capture pprof CPU profile during load
               │
               ├── Proto marshal/unmarshal → reduce response size or cache
               ├── JSON encoding → optimize serialization
               ├── String operations → reduce allocations
               └── Mutex contention → pprof mutex profile → redesign locking
```

### 3c. Optimization Targets

Based on the RPC inventory (158 RPCs, 58% reads) and known usage patterns,
these are the highest-impact endpoints to optimize:

**Tier 1 — Called on every app open / session:**

| Endpoint | Expected Issues |
|----------|----------------|
| `GetFeed` | Multiple JOINs per feed item, user lookups, pagination |
| `ListConversations` | Participant user lookups, unread counting |
| `GetUnreadCounts` | Cross-conversation aggregation |
| `ListCommunities` | Member counts, recent activity |

**Tier 2 — Called frequently during browsing:**

| Endpoint | Expected Issues |
|----------|----------------|
| `Search` | Embedding computation + vector similarity + text fallback |
| `ListCommunityGear` | Per-item owner lookup, location JOIN |
| `ListExperiences` | RSVP counts, host info, time proposals |
| `ListRequests` | Offer counts, requester info |
| `ListStories` | Story generation, participant info |

**Tier 3 — Called on detail views:**

| Endpoint | Expected Issues |
|----------|----------------|
| `GetGear` + `GetGearPeople` | Transfer history, interested users |
| `GetExperience` + `GetExperiencePeople` | RSVP list, attendance |
| `GetUser` + `GetUserStats` | Aggregate stats across communities |
| `GetCommunityImpactMetrics` | Multiple aggregation queries |

### 3d. Common Optimization Patterns

Based on the SQL refactor experience and the codebase architecture, these
are the optimization patterns most likely to apply:

1. **Batch user fetches** — `FetchAPIUsersBatch` already exists; ensure all
   list/detail endpoints use it instead of per-item `FetchAPIUser`

2. **Query result caching** — For read-heavy endpoints called many times with
   the same parameters (e.g., `GetCommunity` for the same community across
   multiple users), consider per-request or short-TTL in-process caching

3. **Reduce response building cost** — Detail endpoints like `GetGear` often
   re-fetch entities that were already loaded. Pass pre-fetched context through
   the call chain

4. **Pagination enforcement** — Ensure all list endpoints respect LIMIT and
   don't fetch more data than the client displays

5. **Index coverage** — Use `EXPLAIN ANALYZE` on slow queries identified via
   profiling to verify index usage

6. **Connection pool tuning** — Use health endpoint pool stats and pprof
   goroutine dumps to right-size `MaxOpenConns`, `MaxIdleConns`,
   `ConnMaxLifetime`

---

## Implementation Phases

### Phase 0: Production Traffic Analysis — DONE

- [x] Pull Cloud Run request logs from `ripls-dev` and `ripls-prod` via `gcloud logging read`
- [x] Create `scripts/analyze_cloud_logs.sh` to aggregate by RPC method
- [x] Produce `docs/ai/production_traffic_analysis.md` with traffic distribution, read/write ratio, latency percentiles
- [x] Use findings to calibrate persona read weights for Phase 2

**Notes:** Cloud Run service names are `server-dev` (ripls-dev project) and
`server` (ripls-prod project). Dev is 85% writes (simulation-dominated). Prod
has minimal traffic (~55 legitimate RPCs in 3 days) so read weights were
UX-flow-informed rather than purely data-driven. Latency hotspots identified:
CompleteTransfer p95=3.2s, CompleteExperience p95=2.8s.

### Phase 1: pprof Endpoints + Enhanced Logging — DONE

- [x] Add pprof endpoints (gated behind `--dev-mode`) to `server/main.go`
- [x] Enhance logging middleware to include RPC method name as structured field
- [x] Add response size to structured log fields
- [x] Verify: server starts, `curl localhost:8080/debug/pprof/` returns index

**Notes:** `extractRPCMethod()` parses Connect-Go paths (`/ripls.api.ServiceName/Method`
→ `ServiceName/Method`), skips non-RPC paths. Tests in `logging_test.go`.

### Phase 2: Simulation Read Patterns — DONE

- [x] Define read action constants in `activity.go` (7 actions: CheckFeed, SearchCommunity, BrowseGear, ViewProfile, CheckStories, CheckConversations, ViewImpact)
- [x] Define `ReadBehavior` struct and per-persona read weights (calibrated from Phase 0)
- [x] Implement `generateReadTimeline()` in new `reads.go` — generates periodic read steps via Poisson sampling
- [x] Implement read action execution in `executor_reads.go` (7 execute functions with realistic RPC chains)
- [x] Add contextual reads before write actions in `executor_reads.go` (`injectContextualReads()`)
- [x] Add `--read-traffic` and `--read-multiplier` flags to CLI
- [x] Add search query pool (realistic search terms from gear catalog + common keywords)
- [x] Add `Feed()` accessor to `Client` struct, `ReadsExecuted` counter to `State`
- [x] Add 4 load test scenarios: `load-test-small`, `load-test-medium`, `load-test-large`, `read-heavy`
- [x] Verify: simulation builds, all tests pass, read timeline generates 4.3:1 read/write ratio, 2x multiplier works

**Notes:** Read actions are non-critical (failures don't poison ref flows).
Contextual reads are probability-gated (80% for ExpressInterest/RSVP, 60% for
StartLoan/Complete). `executor_reads.go` is a new file (not in `executor.go`)
to keep file sizes manageable. Load test scenarios were pulled forward from
Phase 4 since they're needed for Phase 3 testing.

### Phase 3: Concurrent Simulation (~1.5 days)

- [x] Implement `concurrent.go` — per-user goroutines with ref-based ordering (not time-window grouping)
- [x] Add `sync.RWMutex` to `State` struct with `rlock`/`wlock` helpers
- [x] Add `--concurrency` flag to CLI (default 0 = sequential)
- [x] Handle flow ordering with `refCoordinator` (per-step channels, causal ordering)
- [x] Add per-user error isolation (failed refs tracked, non-critical actions isolated)
- [x] Refactor `getClient` pattern for cross-user operations (chat, giveaway re-share)
- [x] Read actions run lock-free for full parallelism; write actions hold wlock
- [x] 6 unit tests passing with `-race` (coordinator, pool, chains, state access)
- Note: concurrent execution is non-deterministic — same seed produces similar but not identical results

### Phase 4: Profiling Script — DONE

- [x] Add `load-test-small`, `load-test-medium`, `load-test-large`, `read-heavy` scenarios *(done in Phase 2)*
- [x] Create `scripts/profile.sh` replacing `sql_profile.sh`
- [x] Add pprof capture integration (`--pprof DIR` captures CPU + heap profiles)
- [x] Add comparison mode (`--compare FILE` extracts deltas from previous markdown report)
- [x] Supports `--scenario`, `--concurrency`, `--read-traffic`, `--read-multiplier`, `-o`, `--pprof`, `--compare`
- [x] Per-endpoint latency percentiles (p50/p95/p99/max) from `rpc_method` + `duration_ms` structured log fields
- [x] Per-endpoint DB query stats (min/avg/p50/p95/max) from `db_queries` structured log fields
- [x] Top 10 latency offenders (p95 x calls impact score) + top 10 query offenders

### Phase 5: Baseline Capture & First Optimizations — DONE

- [x] Run baseline for `load-test-small` scenario (3,373 requests, 137,621 total queries, 83s wall-clock)
- [x] Identify top 4 endpoints by DB queries/request: ListCommunityGear (217 avg), GetCommunityImpactMetrics (139 avg), GetFeed (96 avg), MarkFeedItemsViewed (41 avg)
- [x] Optimize ListCommunityGear: batch GetByIDs for gear, locations, owners, active loans (217→8 queries/call, -96%)
- [x] Optimize GetFeed: batch CommunityGear/CommunityRequest/CommunityExperience, locations, offer/RSVP counts (96→17 queries/call, -82%)
- [x] Optimize MarkFeedItemsViewed: IncrementViewCountBatch with batch fetch + batch insert (41→21 queries/call, -48%)
- [x] Optimize GetCommunityImpactMetrics: batch GetByIDs in CalculateActivityCounts, CalculateImpactSavings, CalculateTotalValue (139→18 queries/call, -87%)
- [x] Re-profile and compare: **137,621 → 43,405 total queries (-68.4%)**, wall-clock 83s → 36s

**Reports:** `docs/ai/profile_baseline.md`, `docs/ai/profile_optimized.md`

---

## Files Modified/Created Summary

**New files (Phases 0–2, done):**
- `scripts/analyze_cloud_logs.sh` — Cloud Run log analysis script (Phase 0)
- `docs/ai/production_traffic_analysis.md` — Real traffic distribution report (Phase 0)
- `server/simulation/reads.go` — ReadBehavior, generateReadTimeline(), SearchQueryPool() (Phase 2)
- `server/simulation/executor_reads.go` — 7 read execute functions + injectContextualReads() (Phase 2)

**Modified files (Phases 0–2, done):**
- `server/main.go` — pprof endpoint registration (dev-mode only) (Phase 1)
- `server/middleware/logging.go` — RPC method name + response_bytes structured fields (Phase 1)
- `server/middleware/logging_test.go` — Tests for extractRPCMethod + response bytes (Phase 1)
- `server/simulation/activity.go` — 7 read action constants, TimelineOptions, GenerateTimelineWithOptions() (Phase 2)
- `server/simulation/executor.go` — Read action switch cases, contextual reads call, queryPool (Phase 2)
- `server/simulation/client.go` — Feed() accessor + feedSvc field (Phase 2)
- `server/simulation/state.go` — ReadsExecuted counter (Phase 2)
- `server/simulation/runner.go` — ReadTraffic/ReadMultiplier wiring (Phase 2)
- `server/simulation/scenarios.go` — 4 load-test scenarios + generateMembers() (Phase 2)
- `server/simulation/scenarios_test.go` — Read timeline + load test scenario tests (Phase 2)
- `server/simulation/types.go` — ReadTraffic/ReadMultiplier on RunConfig (Phase 2)
- `server/cmd/simulate/main.go` — --read-traffic, --read-multiplier flags + read count output (Phase 2)

**New files (Phase 3, done):**

- `server/simulation/concurrent.go` — refCoordinator, clientPool, executeConcurrent, executeReadAction, executeWriteAction
- `server/simulation/concurrent_test.go` — 6 tests for coordinator, pool, chains, state safety

**Modified files (Phase 3, done):**

- `server/simulation/state.go` — `sync.RWMutex`, `rlock`/`wlock` helpers
- `server/simulation/types.go` — `Concurrency int` on RunConfig
- `server/simulation/executor.go` — `getClient` param on `executeExpressInterest`, `injectChat`; `isReadAction` helper; sequential `getClient` resolver
- `server/simulation/chat.go` — `getClient` param on `sendChatMessages`, all 4 chat senders
- `server/simulation/executor_reads.go` — counter increments moved to caller
- `server/simulation/runner.go` — branch to `executeConcurrent` when `Concurrency > 0`
- `server/cmd/simulate/main.go` — `--concurrency` flag

**Planned new files (Phases 4–5, not started):**

- `scripts/profile.sh` — Enhanced profiling script (Phase 4, replaces `sql_profile.sh`)

**Deprecated:**
- `scripts/sql_profile.sh` — will be replaced by `scripts/profile.sh` (Phase 4)

---

## Implementation Reference

### Server Architecture

**Main entry point:** `server/main.go`

- Uses `http.NewServeMux()` for routing
- 20+ services registered via `path, handler := apiconnect.NewXxxServiceHandler(service)` then `mux.Handle(path, authMiddleware.Wrap(handler))`
- Middleware chain order: RequestID → Logging → QueryStats → CORS → [SimulationClock (dev only)] → h2c → ServeMux
- Auth via `connectrpc.com/authn` middleware wrapping individual service handlers
- Dev mode flag: `cfg.devMode` (bool from `--dev-mode` CLI flag)

**Middleware files:**

| File | Purpose |
|------|---------|
| `server/middleware/request_id.go` | UUID generation, context storage |
| `server/middleware/logging.go` | Request logging: method, path, status, duration, user_id |
| `server/middleware/query_stats.go` | Attaches `*storage.QueryStats` to context, logs `db_queries`/`db_duration_ms` |
| `server/middleware/cors.go` | CORS handling (gorilla/handlers) |

**pprof wiring pattern:**

`import _ "net/http/pprof"` registers handlers on `http.DefaultServeMux`. Route `/debug/pprof/` from the app mux to `http.DefaultServeMux`, gated behind `cfg.devMode`.

**Existing instrumentation:**

- `server/storage/instrumented_db.go` — `SQLDB` interface, `InstrumentedDB` wrapper, `QueryStats` struct (atomic counters for `Count` and `TotalNs`), slow query logging (>100ms at WARN)
- `server/storage/query_assert.go` — `AssertMaxQueries(t, ctx, max, fn)` test helper
- Health endpoint at `/health` and `HealthService/CheckHealth` with DB pool stats (`open_connections`, `in_use`, `idle`, `wait_count`, `wait_duration_ms`)

### Simulation Framework Internals

**Key files:**

| File | Purpose |
|------|---------|
| `server/simulation/types.go` | `Activity` type (string), `Scenario`, `CommunityDef`, `MemberDef`, `PersonaType` |
| `server/simulation/activity.go` | `GenerateTimeline()`, Poisson sampling, multi-step flow generation |
| `server/simulation/executor.go` | `executeTimeline()`, per-step 30s timeout, flow state tracking, chat injection |
| `server/simulation/runner.go` | `RunSimulation()` orchestrates: setup → media → timeline gen → execute |
| `server/simulation/state.go` | `State` struct with auth tokens, entity IDs, flow tracking |
| `server/simulation/client.go` | HTTP client with `X-Simulation-Timestamp` and auth header injection |
| `server/simulation/scenarios.go` | 4 built-in scenarios |
| `server/simulation/personas.go` | Persona weights and activity selection |
| `server/cmd/simulate/main.go` | CLI entry point |

**Current Activity constants (all writes):**

- Gear: `ActionSaveGear`, `ActionShareGear`
- Loans: `ActionExpressInterest`, `ActionSelectRecipient`, `ActionStartLoan`, `ActionCompleteLoan`
- Giveaways: `ActionExpressInterestGiveaway`, `ActionSelectGiveaway`, `ActionCompleteGiveaway`
- Requests: `ActionSubmitRequest`, `ActionOfferToFulfill`, `ActionFulfillRequest`
- Experiences: `ActionSaveExperience`, `ActionShareExperience`, `ActionRSVP`, `ActionRSVPNo`, `ActionStartExperience`, `ActionCompleteExperience`
- Cancellations: `ActionWithdrawInterest`, `ActionCancelTransfer`, `ActionCancelRequest`, `ActionWithdrawOffer`, `ActionCancelExperience`

**Client services** (lazy-initialized on `simulation.Client`): Login, Gear, Community, Transfer, Request, Experience, Chat, Media, Admin, User, Equity, Search, Impact, Location

**Executor pattern:**

- Steps sorted by time, executed sequentially
- `CompletedRefs`/`FailedRefs` maps skip steps for finished/failed flows
- 50–70% chat injection probability after successful steps
- Progress logged every 100 steps

**State struct fields** (need `sync.RWMutex` for Phase 3):

`UserTokens`, `UserIDs`, `UserNames`, `CommunityIDs`, `CommunityMembers`, `GearIDs`, `GearOwners`, `GearTemplates`, `LocationIDs`, `TransferIDs`, `RequestIDs`, `ExperienceIDs`, `InterestedUsers`, `HelperUsers`, `RequestOwners`, `ExperienceOwners`, `ConversationIDs`, `MediaIDs`, `CompletedRefs`, `FailedRefs`, 14 activity counters

### RPC Inventory (158 total: 92 reads, 66 writes)

**Read-heavy services for Phase 2 simulation:**

| Service | Read RPCs | Key Methods |
|---------|-----------|------------|
| FeedService (75% read) | `GetFeed`, `ListStories`, `ListItemStories` | + `MarkFeedItemsViewed` (write) |
| SearchService (100% read) | `Search` | Unified semantic/exact search |
| ChatService (79% read) | `ListConversations`, `GetConversationHistory`, `GetUnreadCounts`, `GetConversation`, `GetConversationForTransfer/Request/Experience` | |
| GearService (75% read) | `GetGear`, `ListUserGear`, `GetGearPeople`, `GetGearStats`, `GetTransferRequestCount` | |
| CommunityService (52% read) | `ListCommunityGear`, `ListCommunityUsers`, `ListCommunities`, `GetCommunity`, `ListCommunityEvents` | |
| UserService (67% read) | `GetUser`, `GetUserStats`, `ListUserStories`, `FindUserByEmail` | |
| ExperienceService (50% read) | `GetExperience`, `ListExperiences`, `ListMyExperiences`, `GetExperiencePeople`, `GetExperienceStats` | |
| RequestService (44% read) | `GetRequest`, `ListRequests`, `ListMyRequests`, `GetRequestPeople`, `GetRequestStats` | |
| ImpactService (100% read) | `GetCommunityImpactMetrics`, `GetUserImpactMetrics`, `GetCommunityMetricDetail`, `GetImpactChain` | |
| TransferService (46% read) | `ListTransfers`, `ListMyTransfers`, `ListReceivedTransfers`, `GetTransfer`, `GetGearTransfers` | |
| EquityService (100% read) | `GetCommunityEquitySummary`, `ListTopContributors`, `GetUserAggregatedEquity` | |

### Cloud Run Log Access

- Dev project: `ripls-dev`, service name: `ripls-server`
- Prod project: `ripls-prod`, service name: `ripls-server`
- Log filter may need adjustment — check whether structured log fields appear under `jsonPayload.path` or `httpRequest.requestUrl`

---

## Verification

After each phase:

1. `go test ./server/...` — all existing tests pass
2. `go build ./server/...` — server builds cleanly
3. After Phase 0: `docs/ai/production_traffic_analysis.md` exists with traffic data
4. After Phase 1: `curl localhost:8080/debug/pprof/` returns index (dev mode only)
5. After Phase 2: simulation with `--read-traffic` completes without errors
6. After Phase 3: concurrent simulation produces consistent results across runs (same seed)
7. After Phase 4: `./scripts/profile.sh` produces a report with latency + query + pprof metrics
8. After Phase 5: measurable improvement in identified bottleneck endpoints
