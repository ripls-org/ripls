---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Server profiling and load-testing stack — per-request SQL counting via InstrumentedDB, dev-mode pprof endpoints, the deterministic simulation framework, and the profile.sh orchestration that ties them into one report; includes AssertMaxQueries for N+1 regressions.
  globs: [server/simulation/**, server/storage/instrumented_db.go, server/middleware/**, scripts/profile.sh, scripts/sql_profile.sh]
  triggers: [profiling, pprof, load-test, simulation, n+1, query-stats, performance, bottleneck, flame-graph]
  lens: [server, performance]
  domain: server
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Server Profiling & Load Testing

This document describes the Ripls server's profiling and load testing
infrastructure: what tools are available, how they work together, and how to
use them to find and fix performance bottlenecks.

## Architecture Overview

The profiling stack has four layers that build on each other:

1. **Query instrumentation** — per-request SQL counting and slow query logging
2. **pprof endpoints** — Go runtime CPU, memory, and goroutine profiling
3. **Simulation framework** — deterministic load generation with realistic
   read/write traffic patterns
4. **Profiling script** — automated orchestration that ties 1–3 together into
   a single command producing a markdown report

## Query Instrumentation

Every database query passes through `InstrumentedDB`
(`server/storage/instrumented_db.go`), a wrapper around `*sql.DB` that counts
queries and measures execution time per request context. The `QueryStats`
middleware (`server/middleware/query_stats.go`) attaches a stats struct to each
request context and logs `db_queries` and `db_duration_ms` as structured fields
when the request completes.

The logging middleware (`server/middleware/logging.go`) adds `rpc_method`
(e.g., `FeedService/GetFeed`) and `response_bytes` to every request log entry,
enabling per-endpoint analysis from structured JSON logs.

### Test Assertions

Use `storage.AssertMaxQueries(t, ctx, max, fn)` in tests to prevent N+1
regressions. This runs `fn`, counts the queries it executed, and fails the test
if the count exceeds `max`. It catches query count regressions at test time
rather than in production.

## pprof Endpoints

When the server runs with `--dev-mode`, Go's `net/http/pprof` handlers are
exposed at `/debug/pprof/`. These provide runtime profiling with zero
application overhead until a profile is actively being captured.

### Available Profiles

| Endpoint | Purpose |
|----------|---------|
| `/debug/pprof/profile?seconds=30` | CPU profile (30-second capture) |
| `/debug/pprof/heap` | Heap allocation snapshot |
| `/debug/pprof/goroutine` | Active goroutines and stack traces |
| `/debug/pprof/mutex` | Mutex contention |
| `/debug/pprof/block` | Blocking operations |
| `/debug/pprof/allocs` | Allocation counts since startup |
| `/debug/pprof/trace?seconds=5` | Execution trace (scheduler, GC) |

### Usage

```bash
# Interactive CPU flame graph (captures 30s, opens browser)
go tool pprof -http=:8081 http://localhost:8080/debug/pprof/profile?seconds=30

# Heap snapshot
go tool pprof -http=:8081 http://localhost:8080/debug/pprof/heap

# Text goroutine dump
curl http://localhost:8080/debug/pprof/goroutine?debug=1
```

pprof profiles are most useful when captured during a simulation run, so the
profiling script automates this (see below).

## Simulation Framework

The simulation framework (`server/simulation/`) generates deterministic,
multi-user workloads against a running server. It models realistic user
behavior with both write flows (sharing gear, making loans, creating
experiences) and read patterns (browsing feeds, searching, checking messages).

### Traffic Composition

**Write flows** model multi-step transaction lifecycles: SaveGear → ShareGear →
ExpressInterest → StartLoan → CompleteLoan, with chat messages injected between
steps. Each simulated user is assigned a persona (e.g., "stuff giver," "time
seeker") that determines their activity mix.

**Read actions** model background browsing behavior — the feed checks, searches,
and gear browsing that dominate real client traffic. Seven read action types are
defined, each executing a realistic chain of RPC calls:

- **CheckFeed** — GetFeed + MarkFeedItemsViewed
- **SearchCommunity** — Search with realistic query terms, follow-up detail views
- **BrowseGear** — ListCommunityGear + GetGear + GetGearPeople
- **ViewProfile** — GetUser + GetUserStats
- **CheckStories** — ListStories
- **CheckConversations** — ListConversations + GetUnreadCounts + GetConversationHistory
- **ViewImpact** — GetCommunityImpactMetrics + GetUserImpactMetrics

Read frequency is controlled per-persona via `ReadBehavior` structs (e.g., a
"stuff seeker" checks feeds 10x/week and searches 5x/week, while a "stuff
giver" checks feeds 5x/week and searches 1x/week). These weights were
calibrated from production Cloud Run log analysis.

**Contextual reads** are injected before write actions to simulate realistic
user journeys. For example, before `ExpressInterest`, there's an 80% chance
the user first calls `GetGear` and `GetGearPeople` (simulating viewing an item
before acting on it).

### Concurrency

The `--concurrency N` flag enables concurrent simulation where each user gets a
dedicated goroutine. A `refCoordinator` ensures causal ordering within
multi-step flows (e.g., StartLoan waits for ExpressInterest to complete) while
allowing independent flows across users to execute in parallel. State access is
protected by `sync.RWMutex` — read actions run lock-free for full parallelism.

### Scenarios

| Scenario | Communities | Members | Purpose |
|----------|-------------|---------|---------|
| `load-test-small` | 1 | 10 | Quick profiling (~30s sequential) |
| `load-test-medium` | 2 | 30 | Realistic load, cross-community queries |
| `load-test-large` | 3 | 75 | Stress test, connection pool pressure |
| `read-heavy` | 1 | 20 | 5x read multiplier, isolates read paths |

## Profiling Script

`scripts/profile.sh` orchestrates a complete profiling run: it builds the
server, starts it in dev mode, runs a simulation scenario, captures pprof
profiles, extracts per-endpoint metrics from structured logs, and produces a
markdown report.

### Prerequisites

- Local PostgreSQL running (`npm run db:start`)
- Generated code exists (`npm run generate`)
- `jq` installed (`brew install jq`)

### Basic Usage

```bash
# Default run (load-test-small, sequential, read traffic off — add --read-traffic to enable)
./scripts/profile.sh

# Save report to a file
./scripts/profile.sh -o docs/ai/profile_baseline.md

# With concurrent users and custom scenario
./scripts/profile.sh --scenario load-test-medium --concurrency 15

# Capture pprof CPU + heap profiles
./scripts/profile.sh --pprof /tmp/profiles -o report.md

# Compare against a previous report (shows deltas)
./scripts/profile.sh --compare docs/ai/profile_baseline.md -o report.md
```

### Report Contents

The generated markdown report includes:

- **Header** — branch, commit, scenario, total requests, total queries, DB
  time, wall-clock duration
- **Per-endpoint latency** — p50, p95, p99, max milliseconds for every RPC
  method, sorted by call count
- **Per-endpoint DB queries** — min, avg, p50, p95, max queries per request
- **Top 10 latency offenders** — ranked by impact score (p95 × call count)
- **Top 10 query offenders** — ranked by total queries across all calls

### Comparison Mode

When `--compare` points to a previous report, the script extracts the previous
total query count and shows the delta (e.g., "43,405 → 42,100, -3.0%"). This
makes it easy to verify that an optimization had the intended effect or that a
refactor didn't regress performance.

## Workflow: Finding and Fixing Bottlenecks

1. **Run the profiling script** to identify endpoints with the highest p95
   latency or query counts.

2. **Diagnose the bottleneck type** from the report:
   - High `db_queries` per request → N+1 pattern → use batch APIs (`GetByIDs`,
     `QueryByFieldIn`)
   - Low query count but high latency → missing index → run `EXPLAIN ANALYZE`
   - Low DB time but high wall-clock → CPU-bound → capture pprof CPU profile

3. **Fix and re-profile** with `--compare` to verify improvement.

4. **Add `AssertMaxQueries` tests** to prevent regression.

### Historical Results

The initial baseline captured 137,621 total SQL queries for the
`load-test-small` scenario. After batch-optimizing the top four endpoints
(ListCommunityGear, GetCommunityImpactMetrics, GetFeed, MarkFeedItemsViewed),
total queries dropped to 43,405 — a 68.4% reduction — and wall-clock time went
from 83s to 36s.

## CLI Reference

| Flag | Default | Description |
|------|---------|-------------|
| `--scenario` | `load-test-small` | Simulation scenario name |
| `--seed` | `42` | PRNG seed for deterministic runs |
| `--concurrency` | `0` (sequential) | Max concurrent user goroutines |
| `--read-traffic` | disabled | Enable read actions (feed, search, browse) |
| `--read-multiplier` | `1.0` | Scale read action frequency |
| `-o, --output` | (stdout) | Write markdown report to file |
| `--pprof` | (none) | Directory for CPU + heap profile files |
| `--compare` | (none) | Previous report for delta comparison |
| `--port` | `8080` | Server port |
| `-v, --verbose` | off | Debug logging |
