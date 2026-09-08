---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Server observability — structured slog logging, request IDs, standard field names, sensitive-data masking, log levels, health checks, and the log-based SLI/SLO metrics, dashboards, and alerting stack in terraform/modules/monitoring.
  globs: [server/logging/**, server/middleware/**, server/statslog/**, terraform/modules/monitoring/**]
  triggers: [logging, slog, observability, metrics, request_id, masking, log-level, dashboard, alert, alerting, sli, slo, log-based-metrics]
  lens: [observability, server]
  alwaysApply: true
  domain: server
freshness:
  verified_commit: "e78336400"
  verified_on: "2026-07-26"
---
# Server Observability Infrastructure

This document describes the observability infrastructure for the Ripls Go server, including structured logging, health monitoring, automated reporting, and the roadmap for comprehensive observability.

## Overview

The server's observability strategy is built on four pillars:

1. **Structured Logging** - JSON-formatted logs with request correlation and sensitive data masking
2. **Health Monitoring** - Endpoint-based health checks with dependency validation
3. **Automated Reporting** - Daily digest emails with health status and error analysis
4. **Metrics & Alerting** - Cloud Monitoring integration with log-based metrics and alerts

## Current Implementation Status

| Component | Status | Coverage |
|-----------|--------|----------|
| Structured Logging | Implemented | 95% |
| Request ID Propagation | Implemented | 95% |
| Sensitive Data Masking | Implemented | 95% |
| Health Check Endpoints | **Implemented** | 100% |
| Uptime Monitoring | **Implemented** | 100% |
| Log-Based Metrics | **Implemented** | 100% |
| Alerting | **Implemented** | 100% |
| Daily Digest Emails | **Planned** | 0% |

---

## Part 1: Structured Logging (Implemented)

The server uses structured JSON logging built on Go's standard `log/slog` package. Every log entry is machine-parseable, includes request correlation IDs, masks sensitive data, and follows consistent field naming conventions. In production on Cloud Run, these logs are automatically indexed by Google Cloud Logging.

### Design Principles

**1. Machine-First, Human-Readable**

Logs are JSON-formatted in production for automatic parsing and indexing, but switch to human-readable text format in development. The text format includes emoji prefixes for quick visual scanning:

- DEBUG
- INFO
- WARN
- ERROR

The format is controlled via `--log-format` flag (json/text).

**2. Request Correlation**

Every HTTP request receives a unique request ID (UUID v4), either from the `X-Request-ID` header or auto-generated. This ID propagates through all log entries for that request, enabling end-to-end tracing. The Flutter client generates and sends request IDs with every API call.

**3. Context Propagation**

Logging context flows through Go's `context.Context`. Services extract a context-aware logger via `logging.LoggerWithContext(ctx)`, which automatically includes request_id, user_id (if authenticated), and other ambient context.

**4. Sensitive Data Protection**

Emails, tokens, and phone numbers are never logged in plaintext. Use `logging.MaskEmail()` to transform `alice@example.com` to `al***@example.com`, `logging.MaskToken()` to show only the first 8 characters, and `logging.MaskPhone()` to keep only the last 4 digits (`+15551234567` → `+1******4567`). Custom `slog.LogValuer` types (`RedactedEmail`, `RedactedToken`, `RedactedPhone`) provide automatic masking.

**5. Appropriate Log Levels**

- `DEBUG`: Verbose operational details (cache hits, query timing, internal state)
- `INFO`: Normal operations worth recording (user actions, successful operations)
- `WARN`: Unexpected but handled situations (retries, fallbacks, slow queries)
- `ERROR`: Failures requiring attention (database errors, external API failures)

User errors (invalid input, not found) are INFO or WARN, not ERROR. ERROR is reserved for system failures.

**Background-job transient errors.** Long-running background-job
goroutines (the scheduled-notifications dispatcher / reconciler, the
community-purge jobs, the daily and weekly activity digests) log
transient infrastructure failures — connection resets on pooled DB
connections, upstream 5xx, gRPC `UNAVAILABLE`, DNS blips — at WARN
for the first `errs.TransientPageAfter` (5 minutes). If the streak
of transient failures persists past that window, or if a
non-transient error fires, the log escalates to ERROR (which trips
the "Server Error Logged" alert policy and pages on-call). A
successful tick resets the streak so the next isolated transient
starts fresh. Each goroutine owns its own `errs.TransientStreak`
and routes its error log through `errs.LogJobError`; new background
jobs adopt the policy with one constructor call. The substring
classifier is in `server/errs/transient.go` — add to it if a new
transient pattern shows up in production. Change
`errs.TransientPageAfter` to shift the SLO globally.

**Leaf provider + fallback wrapper (AI providers).** The AI provider
layer (`server/ai/`) composes leaf providers (Gemini, Anthropic,
OpenAI) under `FallbackProvider` and `LoadBalancedProvider`. Within
this hierarchy:

- **Leaf provider methods log WARN on upstream API errors — never
  ERROR.** A leaf failure is handled by the fallback chain above it;
  it is the same class of recoverable situation as a background-job
  transient error. The returned `error` is still non-nil so callers
  can inspect it; only the log level reflects that recovery is
  expected.
- **`FallbackProvider.withFallback` owns ERROR escalation.** When
  every provider in the chain has failed, `withFallback` emits
  `logger.Error("all providers failed", ...)` — this is the
  "streak exhausted" surface that justifies alerting. Do not add a
  second ERROR log in leaf code; doing so fires the alert before
  the fallback has had a chance to recover.
- **Ad-hoc callers outside the ten canonical Gen\*/streaming RPC
  handlers** — nudges, imagery fetches — may still log at their own
  preferred level based on operation semantics: best-effort operations
  use WARN; user-critical operations may use ERROR if the RPC itself
  will return 5xx. The canonical service-handler call sites use the
  `ai.LogAIError` discipline below instead.
- **`LoadBalancedProvider` follows the same rule.** Even though the
  load balancer selects a single provider without its own fallback,
  it may be wrapped by a `FallbackProvider` at a higher level — so
  its per-call failures stay at WARN for the same reason.
- **Service callers of `s.aiProvider.<method>`** classify the
  returned `error` with `ai.IsTransientError` before logging.
  Transient (429 / RESOURCE_EXHAUSTED / 5xx / connection reset) →
  WARN with `transient=true`. Terminal (malformed response, internal
  contract violation, misconfig) → ERROR. Use `ai.LogAIError` so
  the classification is consistent across handlers. This mirrors the
  leaf/wrapper rule one layer up and keeps the `all_errors` alert
  focused on true system failures.

This mirrors the `errs.TransientStreak` doctrine above: the fallback
chain IS the streak, and `FallbackProvider.withFallback` is the
escalation surface.

### Logging Components

#### Logger Package (`server/logging/`)

The core logging package provides:

- `Logger` struct wrapping `*slog.Logger` with convenience methods
- `NewLogger(opts Options)` constructor supporting level and format configuration
- `LoggerWithContext(ctx)` for context-aware logging with automatic field extraction
- `RequestIDFromContext(ctx)` / `WithRequestID(ctx, id)` for request ID management
- `MaskEmail()` / `MaskToken()` / `MaskPhone()` redaction functions
- `RedactedEmail` / `RedactedToken` / `RedactedPhone` types implementing `slog.LogValuer`

#### Request ID Middleware (`server/middleware/request_id.go`)

Extracts or generates request IDs for every HTTP request:

1. Checks for `X-Request-ID` header from client
2. Generates UUID v4 if not provided
3. Stores in request context
4. Adds to response header for client correlation

#### Logging Middleware (`server/middleware/logging.go`)

Logs every HTTP request with:

- Method, path, status code, duration
- Request ID and user ID (if authenticated)
- Remote address
- Automatic level selection based on status code (5xx=ERROR, 4xx=WARN, else INFO)

Implements `http.Flusher` for SSE/streaming compatibility.

### Standard Field Names

Consistent field naming enables effective log queries:

| Field                | Description                                                           | Example                |
| -------------------- | --------------------------------------------------------------------- | ---------------------- |
| `request_id`         | HTTP correlation ID (X-Request-ID header or auto-generated UUID)      | `"a1b2c3d4-..."`       |
| `target_request_id`  | ID of a `models.Request` entity that an RPC is operating on          | `"req_abc123"`         |
| `user_id`            | Authenticated user ID                                                 | `"usr_xyz789"`         |
| `user_email`         | Masked email                                                          | `"al***@example.com"`  |
| `operation`          | Function/RPC name                                                     | `"SaveGear"`           |
| `duration_ms`        | Operation duration                                                    | `45`                   |
| `error`              | Error message                                                         | `"connection refused"` |

Entity-specific fields follow patterns: `gear_id`, `community_id`, `transfer_id`, `location_id`, `media_id`,
`target_request_id`. Note that `models.Request` entity IDs use `target_request_id` (not `request_id`) to
avoid collision with the HTTP correlation key injected by `LoggerWithContext`.

External service calls include: `external_service` (e.g., "mapbox", "openai"), `provider`, `model`.

### Request ID glossary

The codebase has two distinct identifiers that both contain the words
"request" and "id". Keep them straight in code, log fields, and prose:

| Concept | Where it lives in code | Wire / log surface | Lifetime |
| --- | --- | --- | --- |
| **HTTP correlation request ID** | Server: `middleware.RequestID`, `middleware.RequestIDHeader`, `logging.WithRequestID`, `logging.RequestIDFromContext`. Client (Dart): `RequestIdGenerator`, scoped per RPC via `Zone.current[#riplsRequestId]`. | `X-Request-ID` HTTP header, `request_id` log field. | One HTTP request (or one RPC stream). |
| **`models.Request` entity ID** | The primary key of a Request entity (a community member asking for help). Many proto fields named `request_id`; Go field `RequestId`; Dart `requestId`. | `target_request_id` log field. The proto field name itself is still `request_id` for wire compatibility. | Lifetime of the row. |

Rules of thumb:

- When you log the entity ID, the field MUST be `target_request_id`.
  `LoggerWithContext` injects the HTTP correlation ID under `request_id`,
  and a collision silently overwrites one with the other.
- Don't introduce new symbol names for the HTTP correlation ID. Reuse
  the existing helpers — they already handle propagation, headers,
  and zone scoping.
- Don't rename the `request_id` proto fields on `Request`-bearing
  messages. That would be a wire break for every existing client and
  stored proto.

#### Streams and per-event correlation

A long-lived stream (`StreamMessages`, `StreamUserEvents`, etc.)
opens once and emits many events over its lifetime. The stream's
`request_id` scopes the *whole* stream — every log line a stream
handler emits is tagged with that single ID. To slice further inside
a stream, use the **entity ID** of the event being processed:

- Chat streams use `message_id` on per-event log lines, so
  `request_id=<stream-id> message_id=<msg-id>` filters to one
  message's processing path.
- Community-event streams use `community_event_id`, gear streams
  use `gear_id`, etc.

When fanning out an event to multiple subscribers (e.g.
`SendMessage` → broadcast to N stream handlers), pass the originating
RPC's `ctx` into the broadcaster so the fan-out log lines carry the
*originator's* `request_id`. That's how a chat message's full
trajectory — write, fanout, per-recipient delivery attempts — stays
joinable to the SendMessage call that produced it. See
`server/services/chat/streaming.go#broadcastMessage` for the
canonical shape: `BroadcastMessage(ctx, conversationID, msg)`.

### Logging Configuration

Command-line flags control logging behavior:

| Flag | Values | Default | Description |
|------|--------|---------|-------------|
| `--log-level` | debug, info, warn, error | info | Minimum log level to output |
| `--log-format` | json, text | json | Output format (text includes emoji prefixes) |
| `--log-source-location` | (boolean flag) | false | Include source file:line in log entries |

```bash
# Development (with source locations for debugging)
go run ./server --log-level=debug --log-format=text --log-source-location

# Production (JSON for Cloud Logging)
go run ./server --log-level=info --log-format=json
```

Example text output with source location:
```
INFO  2025-12-04 10:15:30 INFO  [service.go:64] creating gear user_id=abc123 gear_name=Tent
WARN  2025-12-04 10:15:31 WARN  [protosql.go:120] slow query duration_ms=1500
ERROR 2025-12-04 10:15:32 ERROR [email.go:45] failed to send email error="connection refused"
```

Note: `--log-source-location` adds ~10-15% overhead from `runtime.Caller()`. Use in development for debugging, disable in production.

### Logging Usage Pattern

Services use a consistent pattern for structured logging:

```go
func (s *Service) DoOperation(ctx context.Context, req *Request) (*Response, error) {
    logger := logging.LoggerWithContext(ctx).With(
        "operation", "DoOperation",
        "entity_id", req.EntityID,
    )

    logger.InfoContext(ctx, "starting operation")

    result, err := s.storage.Query(ctx, req.EntityID)
    if err != nil {
        logger.ErrorContext(ctx, "query failed", "error", err)
        return nil, err
    }

    logger.InfoContext(ctx, "operation completed", "result_count", len(result))
    return result, nil
}
```

---

## Part 2: Health Monitoring Infrastructure (Implemented)

This section describes the health monitoring system that provides continuous visibility into server and dependency health for both dev and production environments.

### Architecture Overview

```
+-------------------+     +--------------------+     +-------------------+
| Cloud Monitoring  |---->| /health           |---->| Health Service    |
| Uptime Check      |     | (HTTP GET endpoint)|     | (HealthService RPC)|
+-------------------+     +--------------------+     +-------------------+
                                   |                         |
                                   v                         v
                          +------------------+     +-------------------+
                          | Cloud Logging    |     | Dependency Checks |
                          | (Health events)  |     | - Database (ping) |
                          +------------------+     | - Storage (GCS)   |
                                                   | - Email (Mailgun) |
                                                   +-------------------+
```

### Health Check Endpoints

Two endpoints provide health monitoring:

#### `/health` - HTTP GET Health Check

Simple HTTP GET endpoint for Cloud Monitoring uptime checks. Returns JSON response with dependency health status:

```json
{
  "timestampMs": 1705347600000,
  "version": "dev",
  "healthy": true,
  "uptimeMs": 3600000,
  "dependencies": [
    {
      "name": "database",
      "backend": "postgresql",
      "healthy": true,
      "configured": true,
      "latencyMs": 12,
      "details": {
        "open_connections": "5",
        "in_use": "1",
        "idle": "4"
      }
    },
    {
      "name": "storage",
      "backend": "gcs",
      "healthy": true,
      "configured": true,
      "latencyMs": 45,
      "details": {
        "bucket": "ripls-media"
      }
    },
    {
      "name": "email",
      "backend": "mailgun",
      "healthy": true,
      "configured": true,
      "latencyMs": 89,
      "details": {
        "domain": "mail.example.com"
      }
    }
  ]
}
```

Always returns HTTP 200 to indicate the server is reachable. The `healthy` field in the response body indicates whether all dependencies are healthy (true) or if any failed (false). This allows uptime checks to distinguish between "server is down" (connection failure) and "server is up but a dependency is failing" (200 with healthy: false).

#### `HealthService.CheckHealth` - Connect RPC

Full RPC endpoint for programmatic health checks. Protocol buffer definition:

```protobuf
// proto/ripls/api/health_service.proto
service HealthService {
  rpc CheckHealth(CheckHealthRequest) returns (CheckHealthResponse);
}

message CheckHealthResponse {
  int64 timestamp_ms = 1;
  string version = 2;
  bool healthy = 3;
  int64 uptime_ms = 4;
  repeated DependencyStatus dependencies = 5;
}

message DependencyStatus {
  string name = 1;
  string backend = 2;
  bool healthy = 3;
  bool configured = 4;
  int64 latency_ms = 5;
  string error = 6;
  map<string, string> details = 7;
}
```

### Dependency Health Checks

The health service validates dependencies via the `Checker` interface:

```go
// server/services/health/service.go
type Checker interface {
    Name() string
    // Returns a slice so composite checkers can report multiple leaf statuses.
    CheckHealth(ctx context.Context) ([]*health.Status, error)
}

// health.Status contains backend info and metadata
type Status struct {
    Backend  string
    Metadata map[string]string
}
```

Dependencies register their check functions at startup. External providers
register **TTL-cached** (`RegisterFuncCached`, 15-minute
`DefaultExternalTTL`): uptime checks hit `/health` continuously, and an
uncached paid probe turns monitoring frequency directly into provider spend —
the Google Maps checker's billable geocode was 45% of the GCP bill before the
cache (#2809), and the Pexels probe burned its 200 req/hour quota the same way
(#929, fixed then by deregistering). Free local checks (database, storage)
stay uncached so `/readyz` always reflects current state. Cached results are
re-served with a `cached_age_ms` detail; an unhealthy cached result still
reports unhealthy on every probe, so log-based alerting is unaffected.

```go
// server/main.go (core resources) and server/wiring.go (services/providers)
healthService := healthsvc.New(logger)

// Core dependencies — free, local, gate /readyz: never cached.
healthService.RegisterFunc("database", sqlStorage.CheckHealth)
healthService.RegisterFunc("storage", bucketStorage.CheckHealth)

// External providers — paid or quota-bound probes: TTL-cached.
healthService.RegisterFuncCached("email", healthsvc.DefaultExternalTTL, emailService.CheckHealth)
if aiProvider != nil {
    healthService.RegisterFuncCached("ai", healthsvc.DefaultExternalTTL, aiProvider.CheckHealth)
}
if cfg.mapboxAccessToken != "" {
    healthService.RegisterFuncCached("mapbox", healthsvc.DefaultExternalTTL, mapboxClient.CheckHealth)
}
// Note: stock imagery is intentionally NOT registered. Its CheckHealth
// hits the Pexels API on every probe and would exhaust the 200 req/hour
// quota (#929). The providers still implement CheckHealth; only the
// registration was removed.
```

#### Current Implementations

**Database (PostgreSQL)** - `server/storage/protosql.go`:
- Pings database connection
- Returns connection pool stats (open, in_use, idle)

**Storage (GCS/Local)** - `server/storage/bucket.go`:
- GCS: Fetches bucket attributes
- Local: Verifies directory accessibility
- Returns backend type and bucket/path info

**Email (Mailgun/Mock)** - `server/email/mailgun.go`:
- Mailgun: Calls GetDomain to verify API access
- Mock: Always healthy (for development)
- Returns domain info

**AI Providers** - `server/ai/provider_*.go`:
- Anthropic: Lists models via free /v1/models endpoint
- OpenAI: Lists models via free /v1/models endpoint
- Gemini: CountTokens with a minimal payload (free endpoint)
- LoadBalanced: Returns a single composite status; healthy if at least one weighted provider is healthy. Per-leaf failures emit WARN-level `dependency_leaf_unhealthy` logs without triggering the health alert.
- Fallback: Returns a single composite status; healthy if the primary or any fallback is healthy (at-least-one-healthy rule). Per-leaf failures emit WARN-level `dependency_leaf_unhealthy` logs without triggering the health alert.
- Mock: Always healthy (for testing)

**Location (Mapbox / Google Maps)** - `server/location/{mapbox,google}.go`:
- Mapbox: Validates access token via free /tokens/v2 endpoint
- Google Maps: Forward-geocodes a fixed known-good address — a **billable**
  Geocoding API call, which is why the checker registers TTL-cached (#2809)
- Returns backend type

**Stock Imagery** - `server/media/*.go` (CheckHealth implemented but NOT
registered — see #929; a probe would call Pexels and burn the 200 req/hour quota):
- Unsplash: Minimal search with per_page=1
- Pexels: Minimal search with per_page=1
- Fallback: Checks all underlying providers
- Cached: Delegates to primary provider
- Fake: Always healthy (for testing)

---

## Part 3: Automated Daily Digest Reports (Planned)

A daily email report will be sent summarizing server health, errors, and key metrics.

### Report Generation Architecture

```
Cloud Scheduler (Daily 8:00 AM UTC)
         |
         v
POST /internal/reports/daily-digest
         |
         v
+-------------------+
| Report Generator  |
+-------------------+
         |
    +----+----+
    |         |
    v         v
Cloud      Historical
Logging    Health Data
API        (24h window)
    |         |
    +----+----+
         |
         v
+-------------------+
| Email Formatter   |
| (HTML + Text)     |
+-------------------+
         |
         v
+-------------------+
| Mailgun Send      |
+-------------------+
```

### Daily Digest Content

The daily digest email will include:

#### 1. Health Summary

```
RIPLS SERVER HEALTH DIGEST - 2025-01-14
Environment: Production

OVERALL STATUS: HEALTHY

Dependency Health (Last 24h):
  Database:  100% uptime, avg latency 12ms
  GCS:       100% uptime, avg latency 45ms
  Mailgun:   100% uptime, avg latency 89ms
  Mapbox:    99.8% uptime, avg latency 156ms (2 timeouts)
```

#### 2. Error Analysis

```
ERROR SUMMARY (Last 24h):

Total Errors: 23
  - 5xx Errors: 3
  - 4xx Errors: 20 (client errors, informational)

Top Error Types:
  1. connect.CodeNotFound (12) - Normal: users requesting non-existent resources
  2. connect.CodeUnauthenticated (5) - Expired tokens
  3. connect.CodeInternal (3) - INVESTIGATE: Database timeout
  4. connect.CodeInvalidArgument (3) - Invalid request data

Errors Requiring Attention:
  [ERROR] 2025-01-14 03:42:15 - Database connection timeout
    request_id: abc-123
    operation: SaveGear
    error: "context deadline exceeded"

  [ERROR] 2025-01-14 07:15:33 - Database connection timeout
    request_id: def-456
    operation: GetCommunityMembers
    error: "context deadline exceeded"
```

#### 3. Traffic Overview

```
TRAFFIC SUMMARY (Last 24h):

Total Requests: 15,234
  - Successful (2xx): 14,891 (97.7%)
  - Client Errors (4xx): 320 (2.1%)
  - Server Errors (5xx): 23 (0.2%)

Peak Traffic: 3:00 PM UTC (892 requests/hour)
Lowest Traffic: 4:00 AM UTC (89 requests/hour)

Top Endpoints by Traffic:
  1. /ripls.api.GearService/ListGear (4,521 calls)
  2. /ripls.api.UserService/GetUser (3,892 calls)
  3. /ripls.api.CommunityService/GetCommunity (2,156 calls)

Slowest Endpoints (p95 latency):
  1. /ripls.api.SearchService/Search (890ms)
  2. /ripls.api.MediaService/UploadMedia (567ms)
  3. /ripls.api.GearService/SaveGear (234ms)
```

#### 4. Recommendations

```
RECOMMENDATIONS:

[HIGH] Database timeouts detected (3 occurrences)
  - Consider increasing connection pool size
  - Review slow queries during peak hours
  - Current pool: 10 connections, consider 20

[MEDIUM] Mapbox latency elevated (avg 156ms vs baseline 100ms)
  - Monitor for continued degradation
  - No action required yet

[LOW] 5 authentication failures from same IP
  - Potential credential stuffing attempt
  - IP: 192.168.x.x
  - Consider rate limiting
```

### Report Implementation

```go
// server/reports/daily_digest.go

type DailyDigestGenerator struct {
    logger      *logging.Logger
    storage     *storage.ProtoSQLStorage
    mailgun     *email.MailgunService
    healthCheck *health.HealthChecker
    cloudLogging *logging.CloudLoggingClient
}

type DailyDigestReport struct {
    GeneratedAt     time.Time
    Environment     string
    Period          ReportPeriod
    HealthSummary   HealthSummary
    ErrorAnalysis   ErrorAnalysis
    TrafficOverview TrafficOverview
    Recommendations []Recommendation
}

func (g *DailyDigestGenerator) Generate(ctx context.Context) (*DailyDigestReport, error) {
    period := ReportPeriod{
        Start: time.Now().UTC().Add(-24 * time.Hour),
        End:   time.Now().UTC(),
    }

    // Gather data from multiple sources
    healthSummary, err := g.gatherHealthSummary(ctx, period)
    if err != nil {
        return nil, fmt.Errorf("gathering health summary: %w", err)
    }

    errorAnalysis, err := g.analyzeErrors(ctx, period)
    if err != nil {
        return nil, fmt.Errorf("analyzing errors: %w", err)
    }

    trafficOverview, err := g.gatherTrafficMetrics(ctx, period)
    if err != nil {
        return nil, fmt.Errorf("gathering traffic metrics: %w", err)
    }

    recommendations := g.generateRecommendations(healthSummary, errorAnalysis, trafficOverview)

    return &DailyDigestReport{
        GeneratedAt:     time.Now().UTC(),
        Environment:     g.environment,
        Period:          period,
        HealthSummary:   healthSummary,
        ErrorAnalysis:   errorAnalysis,
        TrafficOverview: trafficOverview,
        Recommendations: recommendations,
    }, nil
}

func (g *DailyDigestGenerator) SendReport(ctx context.Context, report *DailyDigestReport, recipients []string) error {
    // Generate HTML and text versions
    htmlContent := g.renderHTML(report)
    textContent := g.renderText(report)

    subject := fmt.Sprintf("[%s] Ripls Server Health Digest - %s",
        strings.ToUpper(report.Environment),
        report.GeneratedAt.Format("2006-01-02"),
    )

    return g.mailgun.SendEmail(ctx, email.EmailRequest{
        To:       recipients,
        Subject:  subject,
        HTMLBody: htmlContent,
        TextBody: textContent,
    })
}
```

### Cloud Scheduler Configuration

```hcl
# terraform/modules/monitoring/main.tf

resource "google_cloud_scheduler_job" "daily_health_digest" {
  name        = "ripls-daily-health-digest"
  description = "Generate and send daily health digest report"
  schedule    = "0 8 * * *"  # 8:00 AM UTC daily
  time_zone   = "UTC"

  http_target {
    http_method = "POST"
    uri         = "${var.cloud_run_url}/internal/reports/daily-digest"

    oidc_token {
      service_account_email = var.scheduler_service_account
    }
  }

  retry_config {
    retry_count = 3
  }
}

resource "google_cloud_scheduler_job" "hourly_health_check" {
  name        = "ripls-hourly-health-check"
  description = "Hourly health check heartbeat"
  schedule    = "0 * * * *"  # Every hour
  time_zone   = "UTC"

  http_target {
    http_method = "GET"
    uri         = "${var.cloud_run_url}/internal/health"

    oidc_token {
      service_account_email = var.scheduler_service_account
    }
  }

  retry_config {
    retry_count = 2
  }
}
```

### Report Recipients Configuration

Report recipients will be configured via environment variable:

```go
// Configuration
var reportRecipients = strings.Split(os.Getenv("HEALTH_REPORT_RECIPIENTS"), ",")
// Example: "alerts@example.com,oncall@example.com"
```

---

## Part 4: Cloud Monitoring & Alerting (Implemented)

The monitoring infrastructure is implemented in Terraform and deployed to both dev and prod environments.

**Source:** `terraform/modules/monitoring/`

### Architecture

```
Cloud Monitoring Uptime Check (every 5 minutes)
         │
         ▼
    /health endpoint
         │
         ├── Returns HTTP 200 if healthy
         └── Returns HTTP 503 if unhealthy
         │
         ▼
    Alert Policies
         │
         ├── Uptime Check Failed (CRITICAL)           ← /health unreachable (symptom)
         ├── Server Process Crash (CRITICAL)          ← system log: non-zero exit / OOM (cause)
         ├── Server Serving-Loop Failure (CRITICAL)   ← app log: serving goroutine died
         ├── RPC Error Rate High (ERROR)
         └── Health Check Unhealthy (ERROR)
         │
         ▼
    Email Notifications
```

The crash alerts (`Server Process Crash`, `Server Serving-Loop Failure`) are fed
by log-based filters, not the `/health` path drawn above — see
[Crash & Process-Exit Detection](#crash--process-exit-detection-issue-2490).

### Uptime Checks

Cloud Monitoring uptime checks call the `/health` endpoint from Google's
infrastructure **in prod only** (gated by the monitoring module's
`enable_uptime_check` variable):

- **Frequency:** Every 5 minutes (the USA region group probes from 3
  locations, so `/health` is hit roughly every 100 seconds — the reason
  external health checkers are TTL-cached, see Part 2 and #2809)
- **Timeout:** 30 seconds
- **Regions:** USA (configurable)
- **Success criteria:** HTTP 2xx response

Dev has no uptime check: continuous probing would keep the scale-to-zero dev
service warm 24/7 (#2809). Dev relies on the log-based alerts below, which are
silent while dev is idle and active whenever it is exercised.

### Log-Based Metrics

Custom metrics are created from Cloud Run logs. The main ones:

**1. Health Check Failures** (`health_check_failures_{environment}`)
- Counts health check completions where `healthy=false`
- Triggers when the server is running but dependencies are failing

**2. Container Crashes** (`container_crashes_{environment}`)
- Counts container deaths from the Cloud Run *system* log stream
- See [Crash & Process-Exit Detection](#crash--process-exit-detection-issue-2490)

**3. Serving Failures** (`serving_failures_{environment}`)
- Counts HTTP serving-loop failures from the *application* log stream
- See [Crash & Process-Exit Detection](#crash--process-exit-detection-issue-2490)

(The legacy `rpc_errors_{environment}` count-of-ERROR-logs-per-`operation`
metric and its "RPC Errors Elevated" alert were retired in #2623: the
fraction-based `rpc_error_rate` alert covers failing responses, and any
single ERROR log already trips "Server Error Logged".)

### SLI/SLO Log-Based Metrics (issue #1613)

Latency, traffic, DB, and Go-runtime time series extracted from existing
structured log lines. This replaced the abandoned Prometheus pipeline
(PR #1612): Cloud Monitoring has no Prometheus remote-write endpoint
reachable from Cloud Run, and every needed value already rides on a log
line. Defined in `terraform/modules/monitoring/log_metrics.tf`:

| Metric | Source log line | Kind | Labels |
|---|---|---|---|
| `rpc_requests_{env}` | `http request` (logging middleware) | counter | `rpc_method`, `status` |
| `rpc_request_duration_{env}` | `http request` | distribution (ms) | `rpc_method` |
| `db_request_duration_{env}` | `request db stats` (query-stats middleware) | distribution (ms) | `rpc_method` |
| `db_slow_queries_{env}` | `slow query` (storage layer) | counter | `query_kind` |
| `db_slow_query_duration_{env}` | `slow query` | distribution (ms) | `query_kind` |
| `db_pool_{open,in_use,idle}_{env}` | `db pool stats` (15s ticker) | distribution | — |
| `db_pool_utilization_{env}` | `db pool stats` | distribution (0..1) | — |
| `go_goroutines_{env}` / `go_heap_inuse_bytes_{env}` / `go_gc_pause_ms_{env}` | `go runtime stats` (30s ticker) | distribution | — |
| `email_code_age_{env}` | `email code verified` | distribution (s) | — |
| `email_code_failures_{env}` | email sign-in code failures | counter | — |
| `email_signins_by_credential_{env}` | successful email sign-in | counter | `login_method` |
| `email_delivery_latency_{env}` | `email delivered` (Mailgun webhook) | distribution (ms) | `email_type` |
| `email_deliveries_{env}` | `email delivered` (same filter as the latency metric) | counter | `email_type` |
| `email_delivery_failures_{env}` | `email delivery failed` / `email marked as spam by recipient` | counter | `email_type`, `severity_kind` |
| `sms_delivery_outcomes_{env}` | `sms delivered` / `sms delivery failed` (Twilio status callback) | counter | `message_status`, `twilio_code` |

Interpretation notes:

- **Delivery metrics are not symmetric between the two channels, on purpose.**
  Email gets a latency distribution because the send stamps its own accept time
  into a Mailgun custom variable that the webhook reads back
  (`server/email.stampDeliveryVariables`), so true accepted→delivered latency is
  computable without a correlation table. Twilio reports a lifecycle with no
  such round-trip, so SMS has outcomes only. Counting SMS successes and failures
  in one metric gives its failure-rate alert a denominator that always joins;
  email's failure counter has no matching denominator, so its alert is
  count-based.

- `db_request_duration` is the **uncensored** DB latency signal (total DB
  time per request, from the QueryStats middleware). The `db_slow_query_*`
  metrics only see queries over the 100ms slow-query log threshold — they
  answer "how many slow queries, and how slow", **not** "overall DB P95".
- Log-based metrics cannot be gauges, so the pool and runtime metrics are
  distributions of sampled values. Chart and alert with percentile
  aligners (P99 ≈ worst Cloud Run instance).
- The periodic stats lines are emitted at INFO by the `server/statslog`
  ticker goroutines (`StartPoolStatsLogger`, every 15s; `StartRuntimeStatsLogger`,
  every 30s), started from `server/main.go` at boot. The message strings,
  field names, and intervals are a contract with the Terraform metric
  extractors.
- The 5xx error fraction is derived from `rpc_requests` alone (filtering
  the `status` label), so numerator and denominator labels always join.
- `rpc_method` only ever contains real `ripls.api` services
  (`Service/Method`): the logging middleware extracts it exclusively from
  `/ripls.api.*` paths. (Before #2622 a looser heuristic let internet
  scanner probes mint junk label values like `git/config` — those series
  stopped accruing points and age out of dashboard windows.)

#### Dashboards — where they live and which one answers what

Four dashboards are defined in `terraform/modules/monitoring/dashboards.tf`
and exist in every environment the module is applied to (Cloud console →
Monitoring → Dashboards). They are named `<Dashboard> - <service>`. SLO targets are pinned in widget titles from the same Terraform
variables that drive the alert thresholds, so dashboards and alerts cannot
drift apart.

| You're asking… | Open | Key widgets |
|---|---|---|
| "Is the server slow? Which RPC?" | **RPC Latency** | P95 by RPC (SLO in title), P50/P99 |
| "Are requests failing? Client or server errors?" | **RPC Traffic & Errors** | 5xx fraction (SLO in title), request rate, 4xx vs 5xx split |
| "Is the DB the bottleneck?" | **DB Health** | Per-request DB time P95 (uncensored), pool utilization, slow-query rate/P95 by kind |
| "Leak or memory pressure?" | **Go Runtime** | Goroutines (climb = leak), heap vs Cloud Run limit, GC pause |

Dashboard URLs are per-deployment — the UUIDs belong to the project the
module was applied to, and change if a dashboard resource is destroyed and
recreated. Look yours up from the console list above, or:

```bash
terraform state show module.monitoring.google_monitoring_dashboard.<name>
```

**Querying the metrics directly** (agents/CLI — no console needed): the
metric types are `logging.googleapis.com/user/<name>_{dev,prod}` via the
Monitoring API (`timeSeries.list` with an OAuth token from
`gcloud auth print-access-token`), or
`logging_googleapis_com:user_<name>_{env}` in PromQL. Distributions
aggregate with `ALIGN_DELTA` + `REDUCE_PERCENTILE_XX`; counters with
`ALIGN_RATE` + `REDUCE_SUM`. Data exists only from metric creation onward
(2026-07-01) — log-based metrics have no backfill. Caveats when reading:
`Stream*` RPC "latency" is stream lifetime, not response time, and
`db_slow_query_*` is censored below the 100ms slow-query log threshold
(use `db_request_duration` for overall DB latency).

### Alert Policies

Alert policies with email notifications (plus a Pub/Sub channel in prod):

| Alert | Severity | Trigger | Behavior |
|-------|----------|---------|----------|
| Uptime Check Failed | CRITICAL | Any uptime check failure (immediate) | Immediate notification |
| Server Process Crash | CRITICAL | Container non-zero exit, OOM kill, or signal kill | Rate-limited to 1 per 5 min; re-firing = crash loop |
| Server Serving-Loop Failure | CRITICAL | HTTP serving goroutine panic / fatal serve error / bind failure | Rate-limited to 1 per 5 min |
| RPC Error Rate High | ERROR | >5% of an RPC's responses are 5xx, sustained 10 min (min ~1 error/min) | Per-rpc_method fraction from `rpc_requests_{env}` (#1613, `alerts.tf`) |
| RPC P95 Latency High | WARNING | Interactive RPC P95 > 2s, or slow-class RPC P95 > 8s, sustained 15 min | Per-rpc_method; `Stream*` excluded (duration = stream lifetime); slow class = GenGear, AddMedia, AddMediaFromURL, SubmitFeedback (#2622, `rpc_p95_slow_class_regex`) |
| Availability SLO Burn Rate | WARNING | Error-budget burn > 14.4× (1h window) or > 6× (6h window) | Multi-window burn rate on the availability SLO (#2624, `slos.tf`) |
| Latency SLO Burn Rate | WARNING | Error-budget burn > 14.4× (1h window) or > 6× (6h window) | Multi-window burn rate on the latency SLO (#2624, `slos.tf`) |
| Database Pool Saturation | ERROR | Pool in_use/open > 90%, sustained 10 min | P99 across instances ≈ worst instance |
| Health Check Unhealthy | ERROR | 2 consecutive dependency health check failures | State-change only (per backend) |
| Server Error Logged | ERROR | Any ERROR-level log (excluding health checks) | Rate-limited to 1 per 5 min |
| SMS Delivery Failures High | WARNING | >10% of terminal SMS outcomes are `failed`/`undelivered` over 30 min, sustained 15 min (min ~2 failures/30 min) | Fraction from `sms_delivery_outcomes_{env}` (#2569, `alerts.tf`); triage by the `twilio_code` label |
| Email Sign-in Code Delivery Slow | WARNING | P95 accepted→delivered latency > 10s **and** ≥6 delivered codes in the same 10-minute window, sustained 5 min | P95 over `email_delivery_latency_{env}`, floor over `email_deliveries_{env}` (#2862, `alerts.tf`). Two-condition combiner-AND: P95 threshold + volume floor of `email_code_delivered_min_count` (default 5, so ≥6 required). The floor needs the separate counter because no aggregation yields a scalar sample count from a DELTA DISTRIBUTION. Floor added in #2923 after two low-volume firings; alert is silent by design until volume exceeds the floor |

The legacy count-based "RPC Errors Elevated" alert (>5 ERROR logs per
`operation` in 5 min) and its `rpc_errors_{env}` metric were **retired in
#2623** after the #1613 overlap soak: zero incidents fired on either side
during the overlap, and the legacy alert's unique signal — ERROR logs on
requests that still return non-5xx — is a subset of what "Server Error
Logged" already catches on the first ERROR log.

### SLOs & Error Budgets (issue #2624)

`terraform/modules/monitoring/slos.tf` defines a Cloud Monitoring custom
service (`ripls-server-{env}`) with two request-based SLOs over the
log-based metrics, both on a 28-day rolling window (console: Monitoring →
Services → error-budget view):

| SLO | SLI | Target |
|---|---|---|
| Availability | non-5xx fraction of `rpc_requests_{env}` | 99.5% |
| Latency | fraction of non-streaming, non-slow-class requests in `rpc_request_duration_{env}` under 2000ms | 99% |

Each SLO carries a multi-window burn-rate alert policy (fast: 1h lookback,
burn > 14.4×; slow: 6h lookback, burn > 6× — standard SRE-workbook starting
values). Both are WARNING severity and warn-classified by the GitHub
auto-filer until they prove signal; promote fast-burn to page later if
warranted.

**Layering:** the static threshold alerts stay. Their min-error-rate guard
(`rpc_error_rate_min_errors_per_second`) has no burn-rate equivalent, which
matters at low traffic where a single 5xx among a handful of requests is a
huge instantaneous burn rate. Statics are the guarded floor; SLOs are the
error-budget layer.

**Caveats:** the availability denominator is dominated by high-frequency
polling RPCs (`ListCommunityEvents` is ~80%+ of traffic today), so a failure
confined to a low-traffic RPC barely dents the budget — the per-RPC static
error alert is the compensating control. Targets were set one comfortable
notch below observed first-week prod performance (zero 5xx; well under 1% of
interactive requests over 2s); recalibrate targets and burn thresholds when
prod traffic ramps.

**Health Check Unhealthy** requires 2 consecutive failures (configurable via `dependency_failure_threshold` variable) to avoid alerting on transient issues. It uses state-change alerting with per-backend granularity: you'll receive one notification when a specific backend starts failing (e.g., "anthropic" not just "ai") and another when it recovers (auto-closes after 1 hour of no failures). The alert notification includes:
- **dependency**: The category (e.g., "ai", "storage", "email")
- **backend**: The specific implementation (e.g., "anthropic", "gemini", "postgresql")
- **error**: The error message from the health check

This prevents alert fatigue during extended outages while providing specific details about which backend is failing.

**Composite providers (FallbackProvider, LoadBalancedProvider) suppress per-leaf pages by design.** These providers report a single composite status whose `IsHealthy()` is `true` as long as at least one underlying provider is healthy — matching the at-least-one-healthy guarantee of the request path. A single failing leaf (e.g., the OpenAI tertiary fallback) does not trigger `dependency_health_check_failed` and therefore does not page. Individual-leaf health detail is available as WARN-level `dependency_leaf_unhealthy` log entries in Cloud Logging (filterable on `jsonPayload.message="dependency_leaf_unhealthy"`); this signal is planned for the daily digest (see "Part 3: Automated Daily Digest Reports" below).

### Crash & Process-Exit Detection (issue #2490)

Every log-based metric and alert in the sections above filters the **application**
log stream (`resource.type="cloud_run_revision"` + `jsonPayload`). That stream is
silent when the process dies *hard* — an OOM kill, a non-zero exit from a fatal
startup `os.Exit(1)`, or an unrecovered panic (Go exits with code 2) all happen
without the server writing an application error log. Before #2490 the only signal
for those was the 5-minute **uptime check**, which reports *"unreachable"* with no
crash context and also trips during ordinary deploy / instance-recycle drains — so
a real crash looked identical to a routine restart.

Two CRITICAL alerts close that gap by reading the crash signal directly. They are
always on in both environments (a crash is never routine) and route to the same
channels as every other alert.

**1. `Server Process Crash`** — reads the Cloud Run **system** log stream
(`run.googleapis.com/varlog/system`), the ground truth that the container actually
died. It matches:

- `Container called exit(N)` where **N ≠ 0** (a graceful SIGTERM shutdown exits 0
  and is excluded, so deploys and min-instance recycles do **not** page)
- `Memory limit of … exceeded` (OOM)
- `Container terminated on signal …`

Because a fatal startup `os.Exit(1)` surfaces as `exit(1)` and an unrecovered panic
as `exit(2)`, this single alert covers OOM, startup fatals, and hard panics. A
re-firing alert here is a **crash loop**, not a one-off.

**2. `Server Serving-Loop Failure`** — reads the **application** stream for the
specific messages the server logs when its serving loop dies but the process stays
up: `panic in HTTP server goroutine` (the goroutine recovers and logs, so the
container does *not* exit — the system stream can't see it), `server error`, and
`server failed to bind listener` (`server/main.go`). It deliberately does **not**
match the request-scoped `panic recovered` log from `middleware.PanicRecovery`
(`server/middleware/recovery.go`), which is a handled panic that crashes nothing.

> **Validation note:** the Cloud Run system-message strings (`Container called
> exit(…)`, `Memory limit of …`, `Container terminated on signal …`) are the
> documented/known forms; there was no captured crash sample to template against.
> On the first real crash, confirm `Server Process Crash` fired and tighten the
> filter in `terraform/modules/monitoring/main.tf` if the strings have drifted.

**Source:** `google_logging_metric.container_crashes` / `serving_failures` and
`google_monitoring_alert_policy.container_crash` / `serving_failure` in
`terraform/modules/monitoring/main.tf`.

### Notification Channels

Email notifications go to the addresses each environment passes as
`alert_email_addresses`; the recipients are deployment-specific.

In addition, **prod** routes every alert to a Pub/Sub channel
(`gcp-alerts-to-github`) consumed by the `ripls-alerts` Cloud Function, which
auto-files GitHub issues (gated by `enable_github_pubsub_channel`, prod only).
Email stays attached in parallel as a backup signal.

**Adding an alert policy means adding a classifier pattern.**
`cloud_functions/src/gcpAlertsHandler.ts` decides page-vs-warn by
substring-matching the **condition** display name set in Terraform. A name that
matches nothing classifies as `unknown`, which still files an issue but routes
as a per-incident page-style lifecycle with no claude-fix — the silently
demoted state #2622 found two policies sitting in. So a new policy needs three
things in the same change: the condition name, a matching pattern in
`PAGE_PATTERNS`/`WARN_PATTERNS`, and that exact string pinned in
`cloud_functions/src/__tests__/gcpAlertsHandler.test.ts` so a later rename
breaks CI instead of the routing. The `/delivery\s*fail/i` warn pattern covers
the SMS delivery policy and is the intended landing spot for the email one.

### Terraform Configuration

The monitoring module is included in each environment:

```hcl
# terraform/environments/{dev,prod}/main.tf

module "monitoring" {
  source = "../../modules/monitoring"

  project_id   = var.project_id
  region       = var.region
  environment  = var.environment
  service_name = module.containers.service_name
  service_url  = module.containers.service_url

  alert_email_addresses = ["alerts@example.com"]

  # prod only: route alerts to the auto-file-to-GitHub Pub/Sub channel.
  # enable_github_pubsub_channel = true
}
```

### Alert Email Format

Alert emails include:
- Severity level (CRITICAL/ERROR) in subject
- Service name and environment
- Direct links to Cloud Run logs and metrics
- Troubleshooting steps in documentation

### Unified-create stream (`StreamGenUnifiedCreate`)

The unified-create flow (`docs/design/unified-create.md`) emits the
following structured log fields. These ride on the standard logging
middleware so they appear in Cloud Logging and are covered by the
standard alerting surface (any ERROR log trips "Server Error Logged";
5xx responses count toward the per-`rpc_method` error-rate alert)
without further configuration.

Classifier-stage log lines (`operation=UnifiedCreateClassify`) also
carry the standard `provider` and `model` fields emitted by every
method on `ai.Provider`. `FallbackProvider.withFallback` rewrites
`provider` to the implementation that actually served the response,
so a row with `provider=anthropic` after a Gemini outage indicates
fallback engaged on that call. See `docs/issues/1907-multi-provider-classifier.md`
for the migration that put the classifier on `ai.Provider`.

| Field | Source | Notes |
|---|---|---|
| `operation` | stream_gen.go | `"StreamGenUnifiedCreate"` on the outer stream; `"UnifiedCreateClassify"` on classifier-stage lines |
| `prompt_version` | stream_gen.go, provider_*.go | Bumped when the classifier prompt changes; correlates eval runs to prompt revisions |
| `provider` | provider_*.go | `"anthropic"` / `"gemini"` / `"openai"` on classifier-stage lines; on `FallbackProvider`, names the implementation that served the response (not the primary that may have failed first) |
| `model` | provider_*.go | The model name configured for the serving provider (e.g., `"claude-haiku-4-5"`, `"vertexai/gemini-3.1-flash-lite"`) |
| `user_id` | auth | |
| `user_email` | auth | Masked via `logging.MaskEmail` |
| `input_mode` | stream_gen.go | `"text"` / `"image"` / `"url"` |
| `media_id` | stream_gen.go | Entity ID of the user's uploaded media on image-mode streams; emitted on the `ResolveClassifierMedia` span and any classifier / per-type generation log lines that resulted from it. Never log the presigned URL (carries time-limited credentials in its query string) — only the entity ID. |
| `force_type_used` | stream_gen.go | `true` when the caller supplied `force_type` (a user type-flip re-stream); `false` when the classifier ran |
| `detected_type` | stream_gen.go | Final emitted type (`GEAR` / `EVENT` / `REQUEST`) |
| `classifier_duration_ms` | stream_gen.go | Stage timing |
| `total_duration_ms` | stream_gen.go | End-to-end stream duration |

**Existing alerts that cover this operation today:**

- **`all_errors`** ("Server Error Logged", `terraform/modules/monitoring/main.tf`)
  — fires on any ERROR-level log, so error logs from
  `StreamGenUnifiedCreate` alert automatically. (Note the RPC itself is
  `Stream*`-named, so it is deliberately outside the P95 latency alert and
  the latency SLO — its duration is the stream lifetime.)

**Follow-up (P2.12 / P2.15):** a dedicated classifier-latency alert
(p95 > 1.5s) and a per-stage latency dashboard land as separate Phase 2
work — they require new logging metrics on `classifier_duration_ms` and
`total_duration_ms` plus a dashboard JSON.

### Auto-filed GitHub Issues from Alerts (Phase 1: Crashlytics)

Alongside the email notification channel, production observability signals
auto-file GitHub issues so triage moves out of inboxes and into the issue
tracker. Implemented as a Firebase Cloud Functions codebase under
`cloud_functions/` (TypeScript, Node 24). See
[`cloud_functions/README.md`](../../cloud_functions/README.md) for layout and
[`docs/issues/1148-1149-alert-to-issue.md`](../issues/1148-1149-alert-to-issue.md)
for the full design.

#### What is wired today

| Signal | Source | Trigger | Issue action |
|---|---|---|---|
| Crashlytics velocity alert | Firebase | `onVelocityAlertPublished` | Create / comment, dedup label `crashlytics:<id>` |
| Crashlytics regression alert | Firebase | `onRegressionAlertPublished` | Create / comment / **reopen**, same dedup label |
| Crashlytics stability digest | Firebase | `onStabilityDigestPublished` | Weekly rolling issue, dedup label `crashlytics-digest-week:<isoMonday>`. First digest of the week creates the issue with `claude-fix`; subsequent days within the same week append comments. New week → new issue. |
| Cloud Monitoring incident — page severity | Cloud Monitoring → Pub/Sub → Cloud Function | `onMessagePublished('gcp-alerts-to-github')` | Create / comment / **close on resolve**, dedup label `gcp-incident:<id>`. Page-severity = `uptime_failure`, `health_check_unhealthy`, `database_*`. Always opts into `claude-fix`. |
| Cloud Monitoring incident — warn severity | Cloud Monitoring → Pub/Sub → Cloud Function | `onMessagePublished('gcp-alerts-to-github')` | Daily aggregation: dedup label `gcp-policy:<slug>-<yyyymmdd>`. Warn-severity = `rpc_error_rate`, `all_errors`, `rpc_p95_latency` (both tiers), SLO burn-rate policies. First firing of the day creates with `claude-fix`; subsequent same-day firings append comments. `state=closed` events dropped (closed by daily sweep, not per-incident). |
| Cloud Monitoring — daily sweep | Cloud Scheduler → Cloud Function | `onSchedule('every day 09:00 UTC')` | Closes open `gcp-policy:*` issues > 24h old. Cleans up the warn-severity aggregator from the previous day. |

Cloud Monitoring (Part 4 alert policies) is **also** routed to auto-filing
via a Pub/Sub notification channel `gcp-alerts-to-github`, attached to
every alert policy in the monitoring module via
`local.notification_channel_ids` (gated by
`var.enable_github_pubsub_channel = true` in
`terraform/environments/prod/main.tf`). Email channels stay attached in
parallel; the GitHub pipeline does not replace email.

#### Dedup labels (label-based, no external state)

Each auto-filed issue carries a unique label that identifies the underlying
incident. The Cloud Function searches by that label before creating, so
repeat firings comment instead of creating duplicates.

- `crashlytics:<crashlytics-issue-id>` — one GitHub issue per Crashlytics issue.
- `crashlytics-digest-week:<isoMonday>` — weekly rolling issue for the
  daily Crashlytics stability digest.
- `gcp-incident:<id>` — one issue per Cloud Monitoring incident
  (page-severity policies, full open/close lifecycle).
- `gcp-policy:<policy-slug>-<yyyymmdd>` — daily aggregation key for
  high-volume warn-severity policies (`rpc_error_rate`, `all_errors`,
  `rpc_p95_latency`, SLO burn rates). Closed by the daily sweep at
  09:00 UTC.

Every auto-filed issue also carries `auto-filed` (so humans and queries can
distinguish from human-filed) and a severity-derived label (e.g., `crash`,
`regression`, `platform:android`).

#### Claude-trigger gating

The auto-filer optionally adds the `claude-fix` label, which fires the
existing
[`claude_plan_issue.yaml`](../../.github/workflows/claude_plan_issue.yaml)
workflow. Adding it costs Claude-API time, so we gate carefully:

| Signal | Adds `claude-fix`? |
|---|---|
| Crashlytics velocity alert with `crashPercentage >= 1%` | Yes |
| Crashlytics velocity alert below 1% | No (humans triage as a batch) |
| Crashlytics regression alert | Yes (always — prior fix history is useful context) |
| Crashlytics stability digest, **first digest of the week** | Yes (one plan per weekly tracking issue) |
| Crashlytics stability digest, subsequent days same week | No (appended as comment to existing issue, no new plan) |
| Cloud Monitoring page-severity (uptime / health / database) | Yes (page = wake someone up; Claude triages while humans respond) |
| Cloud Monitoring warn-severity, **first firing of the day** | Yes (one plan per daily aggregator) |
| Cloud Monitoring warn-severity, subsequent same-day firings | No (appended as comment, no new plan) |
| Cloud Monitoring **unknown** severity (policy didn't match a classifier pattern) | No — file the issue, let humans triage manually so we don't burn Claude-API on misclassifications |

Threshold is the constant `CLAUDE_FIX_VELOCITY_THRESHOLD_PCT` in
`cloud_functions/src/crashlyticsHandler.ts`. Tune in Phase 3 based on observed
noise.

#### Velocity alert threshold

Configured in the Firebase Console (not in code). Initial setting:
**affects ≥ 1% of sessions in 1 hour**. This is the threshold at which a
Crashlytics velocity alert fires; the `claude-fix` gating above is a
*separate* threshold applied after the alert reaches us.

To change: Firebase Console → Crashlytics → Settings → Velocity Alerts.
Document the new threshold and rationale here on change.

#### GitHub authentication: shared App with the feedback service

The Cloud Function reuses the same GitHub App that
[`server/services/feedback/`](../../server/services/feedback/) authenticates
with (set up by `tescobar` in 2025; see commits `abc4d4e2b`,
`ca02b2a3c`). Coordinates from `terraform/environments/{dev,prod}/main.tf`:

| Field           | Where it comes from                              |
|-----------------|--------------------------------------------------|
| App ID          | `github_app_id` in `terraform/environments/{dev,prod}/main.tf` |
| Installation ID | `github_installation_id`, same file              |
| Private key     | Secret Manager: `github-app-private-key-base64`  |

Leaving any of the three unset disables the integration rather than failing the
service — see `AppConfig.IsComplete` in `server/github/client.go`.

The Cloud Function reads the same Secret Manager secret via
`defineSecret('github-app-private-key-base64')`. Octokit's `createAppAuth`
mints short-lived installation tokens at invocation time — no long-lived
credentials at runtime.

One rotation surface for both runtimes. To rotate the App private key:

```bash
# 1. Regenerate the private key in GitHub: App settings → Generate a
#    private key. Downloads a .pem.
# 2. Base64-encode (matches the Go server's expected format):
base64 -i downloaded-key.pem -o /tmp/key.b64

# 3. Push a new version to Secret Manager in both projects:
gcloud secrets versions add github-app-private-key-base64 \
  --data-file=/tmp/key.b64 --project="$PROD_PROJECT"
gcloud secrets versions add github-app-private-key-base64 \
  --data-file=/tmp/key.b64 --project="$DEV_PROJECT"

# 4. Redeploy / restart so the new version binds:
#    - Cloud Function: firebase deploy --only functions:ripls-alerts --project prod
#    - Cloud Run server: next deploy picks it up automatically.

# 5. Revoke the old key in the GitHub App settings.
```

The Cloud Functions runtime SA (default
`<project-number>-compute@developer.gserviceaccount.com` unless overridden)
needs `roles/secretmanager.secretAccessor` on this secret in every project
where the alert function runs.

#### Emergency disable

If the auto-filer is creating noise or filing bad issues:

```bash
firebase functions:delete crashlyticsVelocityAlert --region=us-central1 --project prod
firebase functions:delete crashlyticsRegressionAlert --region=us-central1 --project prod
```

Email notifications continue uninterrupted because the email channel is
attached to alert policies in parallel. To re-enable, redeploy via
`.github/workflows/deploy_cloud_functions.yaml` (manual `workflow_dispatch` is
fine).

#### Cloud Monitoring severity classification

Page vs warn is derived in `cloud_functions/src/gcpAlertsHandler.ts`
by substring-matching the alert's `incident.condition.displayName`.
**The patterns must match the CONDITION display name, not the policy
name** — #2622 found two policies filing as "unknown" because their
patterns had been written against policy names. The classifier's unit
tests (`cloud_functions/src/__tests__/gcpAlertsHandler.test.ts`) pin the
actual Terraform condition strings, so a rename that stops matching now
breaks CI instead of silently demoting a policy. Patterns:

| Pattern | Severity |
|---|---|
| `/uptime\s*check/i` | page |
| `/health\s*check/i` | page |
| `/database/i` | page |
| `/server\s*error\s*logged/i` | warn (defensive; see next row) |
| `/service[-\s]?level\s*error/i` | warn — matches `all_errors`' actual condition "Service-level error logged (…)" |
| `/rpc\s*errors/i` | warn — matches `rpc_error_rate`'s "RPC errors exceed N% of requests" |
| `/latency/i` | warn — matches both `rpc_p95_latency` tiers (#2622) |
| `/burn\s*rate/i` | warn — matches the SLO burn-rate conditions (#2624) |
| (no match) | unknown — files but doesn't trigger Claude |

When you add a new alert policy in `terraform/modules/monitoring/main.tf`,
either: (a) name its condition so an existing pattern matches (e.g.
include "Database" in display_name for a new DB metric), or (b) add a
new pattern in `gcpAlertsHandler.ts` and ship a Cloud Function deploy.
The "unknown" bucket is the safety net — issues still get filed,
they just stay quiet (no Claude, no severity-specific labels).

#### Bot identity vs `claude_plan_issue.yaml` self-trigger guard

The existing Claude planning workflow skips events where
`github.actor == 'claude[bot]'` so it doesn't re-fire on its own follow-up
comments. The auto-filer's GitHub identity is the existing GitHub App's
bot user (whatever that App is named) — distinct from `claude[bot]` — so
the guard correctly lets the auto-filer through. Do not rename the App
such that its actor string collides.

---

## Part 5: Implementation Roadmap

### Phase 1: Health Check Endpoints (Complete)

**Completed:**
- [x] Create `server/health/status.go` with Status struct
- [x] Create `server/services/health/service.go` with HealthService RPC
- [x] Implement `/health` GET endpoint wrapping the RPC
- [x] Implement `HealthService.CheckHealth` Connect RPC
- [x] Add dependency checks: Database, Storage, Email
- [x] Register endpoints in `server/routes.go`
- [x] Add unit tests for health service

**Files created:**
- `proto/ripls/api/health_service.proto` - Health check proto definitions
- `server/health/status.go` - Status struct for check results
- `server/services/health/service.go` - Health service implementation
- `server/services/health/service_test.go` - Health service tests

**Files modified:**
- `server/main.go` - Register health service and dependency checkers
- `server/storage/bucket.go` - Add CheckHealth to BucketStorage interface
- `server/storage/protosql.go` - Add CheckHealth to ProtoSQLStorage
- `server/email/mailgun.go` - Add CheckHealth to email.Service interface

### Phase 2: External Service Health Checks (Complete)

**Completed - Core Dependencies:**
- [x] Database health check with pool stats (ping + connection stats)
- [x] GCS health check (bucket attributes)
- [x] Local storage health check (directory access)
- [x] Mailgun health check (domain verification)
- [x] Mock implementations for testing

**Completed - AI Providers:**
- [x] Anthropic health check (free /v1/models list endpoint)
- [x] OpenAI health check (free /v1/models list endpoint)
- [x] Gemini health check (minimal generate with MaxOutputTokens=1)
- [x] LoadBalancedProvider health check (checks all underlying providers)
- [x] FallbackProvider health check (checks primary + fallbacks)
- [x] MockProvider health check (returns healthy)

**Completed - Location Services:**
- [x] Mapbox health check (free token validation endpoint)

**Completed - Stock Imagery Providers:**
- [x] Unsplash health check (minimal search with per_page=1)
- [x] Pexels health check (minimal search with per_page=1)
- [x] FallbackStockImageryProvider health check (checks all underlying providers)
- [x] CachedStockImageryProvider health check (delegates to primary)
- [x] FakeProvider health check (returns healthy)

**Files modified:**
- `server/ai/provider.go` - Added CheckHealth to Provider interface
- `server/ai/provider_anthropic.go` - Implemented CheckHealth
- `server/ai/provider_openai.go` - Implemented CheckHealth
- `server/ai/provider_gemini.go` - Implemented CheckHealth
- `server/ai/provider_loadbalanced.go` - Implemented CheckHealth
- `server/ai/provider_fallback.go` - Implemented CheckHealth
- `server/ai/provider_mock.go` - Implemented CheckHealth
- `server/location/mapbox.go` - Added CheckHealth method
- `server/media/stock_imagery_provider.go` - Added CheckHealth to interface
- `server/media/unsplash_provider.go` - Implemented CheckHealth
- `server/media/pexels_provider.go` - Implemented CheckHealth
- `server/media/stock_imagery_fallback.go` - Implemented CheckHealth
- `server/media/cached_stock_imagery.go` - Implemented CheckHealth
- `server/media/fake_provider.go` - Implemented CheckHealth
- `server/main.go` - Register AI, Mapbox, and stock imagery health checkers

### Phase 3: Heartbeat Infrastructure (Planned)

**Deliverables:**
- [ ] Create Terraform module for Cloud Scheduler jobs
- [ ] Configure hourly health check scheduler
- [ ] Add service account for scheduler authentication
- [ ] Implement internal endpoint authentication
- [ ] Deploy to dev environment and validate

**Files to create:**
- `terraform/modules/monitoring/main.tf`
- `terraform/modules/monitoring/variables.tf`
- `terraform/modules/monitoring/outputs.tf`

**Files to modify:**
- `terraform/environments/dev/main.tf`
- `terraform/environments/prod/main.tf`

### Phase 4: Daily Digest Reports (Week 4)

**Deliverables:**
- [ ] Create `server/reports/daily_digest.go`
- [ ] Implement Cloud Logging query for error analysis
- [ ] Create HTML email template for daily digest
- [ ] Create text email template for daily digest
- [ ] Implement `/internal/reports/daily-digest` endpoint
- [ ] Configure Cloud Scheduler for daily report (8 AM UTC)
- [ ] Add report recipient configuration via environment variable
- [ ] Deploy to dev and send test reports

**Files to create:**
- `server/reports/daily_digest.go`
- `server/reports/templates/daily_digest.html`
- `server/reports/templates/daily_digest.txt`

**Files to modify:**
- `server/main.go`
- `terraform/modules/monitoring/main.tf`

### Phase 5: Log-Based Metrics and Alerting (Complete — issue #1613)

**Completed:**
- [x] Create log-based metrics in Terraform (`log_metrics.tf`)
- [x] Configure alerting policies for error rate (fraction-based, `alerts.tf`)
- [x] Configure alerting policies for health check failures (`main.tf`)
- [x] Set up notification channels (email + prod Pub/Sub→GitHub)
- [x] Create Cloud Monitoring dashboards (`dashboards.tf` — RPC latency,
      traffic/errors, DB health, Go runtime)
- [ ] Document alerting runbooks (triage steps currently live inline in each
      alert policy's `documentation` block; standalone runbooks not yet split
      out)

**Files created:**
- `terraform/modules/monitoring/log_metrics.tf`
- `terraform/modules/monitoring/alerts.tf`
- `terraform/modules/monitoring/dashboards.tf`
- `server/statslog/` (periodic pool/runtime stats log lines)

### Phase 6: Production Deployment and Validation (Week 6)

**Deliverables:**
- [ ] Deploy monitoring infrastructure to production
- [ ] Validate health checks in production
- [ ] Verify daily digest emails are received
- [ ] Confirm alerting triggers correctly
- [ ] Document operational procedures
- [ ] Create troubleshooting guide

---

## Future Enhancements

Beyond the planned implementation, these enhancements are recommended for mature observability:

### Prometheus Metrics (fully removed)

The Prometheus stack is gone (#1613/#1614): the `/metrics` endpoint, its
bearer-token gate, the `server/metrics` registry, the HTTP/Connect metrics
middleware, and the `prometheus/client_golang` dependency. Nothing ever
scraped it in production, and the SLI/SLO pipeline is log-based (see
Part 4). Every signal the registry carried has a structured-log
equivalent: rate-limit rejections log `rate_limited` with
procedure/key_type/mode, off-app notification outcomes log
channel/outcome, and tests assert on those log lines.

Future business metrics (e.g. "active transfers right now") should be
designed as **log-based metrics** on structured log lines, not new
Prometheus instruments.

### Distributed Tracing (Future)

OpenTelemetry integration for cross-service tracing:

```go
tracer := otel.Tracer("ripls-server")
ctx, span := tracer.Start(ctx, "SaveGear")
defer span.End()

span.SetAttributes(
    attribute.String("user_id", authInfo.UserID),
    attribute.String("gear_name", req.Msg.Name),
)
```

### Synthetic Monitoring (Future)

External uptime monitoring from multiple geographic locations to validate end-user accessibility.

### Business Metrics Dashboard (Future)

Track business KPIs alongside technical metrics:
- Daily/weekly active users
- Gear sharing rates
- Loan completion rates
- Community growth

---

## Migration Notes

The logging infrastructure was implemented December 2025, converting 500+ unstructured `log.Printf` statements to structured logging. See `server/logging/README.md` for the migration guide with before/after examples.

This document supersedes `docs/server/logging.md` (renamed to `docs/server/observability.md` January 2025) to reflect the expanded scope of observability infrastructure.
