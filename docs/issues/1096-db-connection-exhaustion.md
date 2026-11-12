# Issue #1096: Database connections exhausted after realtime updates

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1096
**Filed:** 2026-04-12 | **Last activity:** 2026-04-12
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

After merging PR #1089 (real-time streaming via `StreamCommunityEvents`), the
production server is hitting PostgreSQL connection limits with error code 53300:
"remaining connection slots are reserved for non-replication superuser
connections". The errors appear on stock image queries and gear queries,
suggesting general pool exhaustion rather than a streaming-specific query.

The Terraform alerting policy for connection exhaustion is configured but not
firing, which is a secondary bug worth investigating.

## Context from Comments

No comments on the issue. The issue author suspects new persistent streams from
PR #1089 may be holding open database connections. Dashboard confirms Cloud Run
instances spiked to 5 at one point.

## Analysis

### Root cause: streaming triggers unnecessary Cloud Run autoscaling

Cloud Run v2 scales based on **concurrent request count**. The container module
(`modules/containers/main.tf`) does not set `max_instance_request_concurrency`,
so it defaults to 80. Each open `StreamCommunityEvents` gRPC stream counts as
one concurrent request, even though these streams are nearly idle — they block
on an in-memory Go channel consuming no CPU.

Each foregrounded client opens **one stream per community** it belongs to (the
client's `CommunityEventStreamService.connectAll()` iterates all community IDs).
Users can be in 1-5 communities, so a handful of active users can easily
generate 10-20+ concurrent streaming "requests." Cloud Run sees this as load
and scales up.

The cascade:
1. Streaming connections count as active requests → Cloud Run scales to 5
   instances
2. Each instance opens its own connection pool (`SetMaxOpenConns(10)` in
   `protosql_schema.go:326`)
3. 5 instances × 10 max connections = 50 potential DB connections
4. The `db-f1-micro` Cloud SQL tier has a hard limit of ~25 connections
5. Connections are refused with PostgreSQL error 53300

### The streams themselves are not the problem

The `StreamCommunityEvents` RPC (`streaming.go`) does NOT hold a persistent
database connection. After an initial catch-up query, streams block on an
in-memory Go channel with zero DB interaction. The streams consume negligible
CPU and memory — they should not trigger scaling.

### Why the alert isn't firing

The alert (`modules/monitoring/main.tf:467`) monitors
`cloudsql.googleapis.com/database/postgresql/num_backends` with a threshold of
`floor(25 * 80 / 100) = 20` connections sustained for 60 seconds. The
exhaustion is likely transient — connections spike above 25 (causing 53300
errors for rejected connections), but rejected connections don't register as
backends, so the metric may never sustain at 20+ for a full minute.

### Connection budget

| Consumer | Connections | Notes |
|----------|-------------|-------|
| App server pool (per instance) | up to 10 | `SetMaxOpenConns(10)` |
| Cloud Run instances (observed) | up to 5 | Scaled due to streaming |
| **Worst-case app total** | **50** | 2× the 25-connection limit |
| Metabase (admin + reader) | ~4-10 | Separate users on same instance |
| PostgreSQL system | ~3 | Background workers |

## Potential Solutions

### Option A: Increase max_instance_request_concurrency

- **Approach:** Set `max_instance_request_concurrency` to 250+ in the Cloud Run
  service template. This tells Cloud Run that a single instance can handle many
  concurrent requests (including idle streaming connections) without needing to
  scale out. With a Go server handling streaming via goroutines, a single
  instance can easily handle hundreds of concurrent streams.
- **Pros:** Directly addresses the root cause (unnecessary scaling). No cost
  increase. No database tier change needed. Single Terraform change.
- **Cons:** Doesn't increase the DB connection limit, so if the server
  legitimately needs >1 instance for CPU-bound work, the problem returns.
- **Effort:** Small.

### Option B: Increase concurrency + reduce max_instances

- **Approach:** Set `max_instance_request_concurrency` to 250 AND reduce
  `max_instances` from 5 to 2. This ensures the connection math always works:
  2 instances × 10 connections = 20, well within the 25-connection budget.
- **Pros:** Belt-and-suspenders. Prevents scaling from ever exhausting DB
  connections. With streaming, there's no need for many instances — the Go
  server handles concurrency via goroutines, not processes.
- **Cons:** Limits horizontal scaling if CPU-bound work actually needs it.
- **Effort:** Small.

### Option C: Full fix — concurrency + max_instances + alerting

- **Approach:** Three changes:
  1. Set `max_instance_request_concurrency = 250` so streaming doesn't trigger
     scaling
  2. Reduce `max_instances` from 5 to 2 as a safety cap so the invariant
     `max_instances × MaxOpenConns ≤ database_max_connections` always holds
  3. Fix the alerting threshold to account for the actual connection budget
     (document the invariant in Terraform)
- **Pros:** Addresses root cause, prevents recurrence, improves observability.
- **Cons:** Multiple changes (but all small Terraform tweaks).
- **Effort:** Small-medium.

## Recommendation

**Option C** — the full fix. The primary change is setting
`max_instance_request_concurrency` high enough that idle streaming connections
don't trigger autoscaling. Reducing `max_instances` to 2 adds a safety cap.
And documenting the connection invariant prevents future regressions.

Additionally, upgrading from `db-f1-micro` to `db-g1-small` provides more
connection headroom and eliminates the tight coupling between instance count and
connection exhaustion.

## Open Questions

- **What value for max_instance_request_concurrency?** — Recommendation: 250.
  The Go server handles concurrency via goroutines, not threads. Each streaming
  connection is a lightweight goroutine blocked on a channel. 250 is
  conservative — Go can handle thousands of goroutines. The default of 80 is
  designed for thread-per-request runtimes like Node.js or Python.

- **Should max_instances stay at 2 or go lower?** — Recommendation: 2. Keeping
  min=1 and max=2 gives some scaling headroom for genuine CPU load while
  guaranteeing the connection math works (2 × 10 = 20 < 25). Going to max=1
  removes all horizontal scaling, which could be a problem during backfill jobs
  or AI-heavy requests.

- **Is the `db-f1-micro` tier adequate long-term?** — Decision: No. Upgrading
  to `db-g1-small` for more connection headroom. This provides a larger safety
  margin and avoids the tight constraint where even 2 instances + Metabase
  approach the 25-connection limit.

- **Why isn't the alert firing?** — Recommendation: The alert is probably
  configured correctly but the metric (`num_backends`) only counts established
  connections, not rejected ones. When all 25 slots are taken, new connection
  attempts fail immediately without ever becoming a backend. Consider also
  adding an alert on Cloud Run error rate (53300 errors in logs) as a
  complementary signal.

## Implementation Plan

### Phase 1: Fix Cloud Run scaling behavior and upgrade DB tier
**Goal:** Stop Cloud Run from scaling up due to idle streaming connections, and
increase connection headroom.
**Tasks:**
- [x] Add `max_instance_request_concurrency` variable to the containers module
      (`modules/containers/variables.tf`) with default 250
- [x] Set `max_instance_request_concurrency` on the `template` block in
      `google_cloud_run_v2_service` (`modules/containers/main.tf`)
- [x] Reduce `max_instances` from 5 to 2 in both `prod/main.tf` and
      `dev/main.tf`
- [x] Upgrade Cloud SQL tier from `db-f1-micro` to `db-g1-small` in both
      `prod/main.tf` and `dev/main.tf`
- [x] Update `database_max_connections` in monitoring module calls to match
      `db-g1-small` actual limit
- [x] Add a comment documenting the invariant:
      `max_instances × DB_MAX_OPEN_CONNS (10) must be < database max_connections`
- [x] Apply to dev, verify instance count stays at 1 with streaming active
- [x] Apply to prod, verify 53300 errors stop
**Validates:** A single Cloud Run instance handles all streaming connections
without triggering autoscaling, and DB connections stay well within the upgraded
tier's limits.

### Phase 2: Improve alerting
**Goal:** Detect connection pressure before it causes user-facing errors.
**Tasks:**
- [ ] Verify the `num_backends` alert fires correctly by reviewing Cloud
      Monitoring metrics for the outage window
- [ ] Consider adding a Cloud Run log-based alert for PostgreSQL error 53300
      as a complementary signal that catches rejected connections
- [ ] Review whether `database_connection_threshold_percent` (80%) is
      appropriate given the new instance cap
**Validates:** Future connection exhaustion scenarios are detected and alerted.

## Related Issues
- PR #1089 — Real-time streaming implementation that triggered the scaling
- #999 — Original real-time updates tracking issue
- #1073 — Always-on server (set min_instances=1 and cpu_idle=false)
