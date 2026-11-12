# Server Observability Implementation Plan

**Design Document:** `docs/server/observability.md`
**Status:** In Progress
**Started:** 2025-01-15

## Overview

Implement health monitoring for the Ripls server with minimal custom infrastructure:

1. **Health endpoint** (`/health`) - Checks all dependencies, returns structured response
2. **Cloud Monitoring** - Uptime checks, log-based metrics, alerts, email notifications
3. **Authentication** - Cloud Run IAM at ingress (no app-level auth code)

## Architecture

```
Cloud Monitoring Uptime Check (hourly)
         │
         │ OIDC token (validated by Cloud Run IAM)
         ▼
    Cloud Run
         │
         ▼
    /health endpoint
         │
         ├── Database (PostgreSQL)
         ├── Storage (GCS/Local)
         ├── Email (Mailgun)
         ├── Location (Mapbox)
         ├── Images (Unsplash, Pexels)
         └── AI (OpenAI, Anthropic, Vertex)
         │
         ▼
    Structured log: "health_check_complete"
         │
         ▼
    Cloud Logging → Log-based metrics → Alert policies → Email
```

## Progress Legend

- `[ ]` Not started
- `[~]` In progress
- `[x]` Completed

---

## Phase 1: Proto Definitions

**Commit:** `feat(proto): add health check API types`

### 1.1 Create Health Proto

Status: `[x]`

**File:** `proto/ripls/api/health_service.proto`

**Tasks:**
- [x] 1.1.1 Create health_service.proto with `HealthCheckResponse` and `DependencyStatus` messages
- [x] 1.1.2 Run `npm run generate`
- [x] 1.1.3 Verify generated code
- [x] 1.1.4 Commit

---

## Phase 2: Health Check Package

**Commit:** `feat(health): add health check endpoint`

### 2.1 Add CheckHealth to Existing Interfaces

Status: `[x]`

**Tasks:**
- [x] 2.1.1 Add `CheckHealth(ctx) error` to `storage.BucketStorage` interface
- [x] 2.1.2 Implement for `GCSBucketStorage` (bucket attrs check)
- [x] 2.1.3 Implement for `LocalBucketStorage` (directory access check)
- [x] 2.1.4 Add `CheckHealth(ctx) error` to `email.Service` interface
- [x] 2.1.5 Implement for `MailgunService` (GetDomain API)
- [x] 2.1.6 Implement for `MockEmailService` (return nil)
- [x] 2.1.7 Tests for each

### 2.2 Create Health Package

Status: `[x]`

**Files:**
- `server/services/health/service.go` - HealthService RPC implementation
- `server/health/status.go` - Health status types

**Dependencies checked:**
- Database: `ProtoSQLStorage.CheckHealth()`
- Storage: `BucketStorage.CheckHealth()`
- Email: `email.Service.CheckHealth()`
- Mapbox: `MapboxClient.CheckHealth()`
- Stock Imagery: `StockImageryProvider.CheckHealth()` (Unsplash, Pexels)
- AI providers: `ai.Provider.CheckHealth()`

**Tasks:**
- [x] 2.2.1 Create `server/services/health/service.go` with HealthService
- [x] 2.2.2 Individual check functions via RegisterFunc pattern
- [x] 2.2.3 Add caching for external API checks (5 min TTL) - Handled by individual providers
- [x] 2.2.4 Implement HTTP handler returning proto as JSON (`HealthHandler`)
- [x] 2.2.5 Log `health_check_complete` with all results
- [x] 2.2.6 Unit tests with mocked dependencies
- [x] 2.2.7 Integration test (`server/integration_tests/health_test.go`)

### 2.3 Register in main.go

Status: `[x]`

**Tasks:**
- [x] 2.3.1 Create HealthService after services initialized
- [x] 2.3.2 Register `/health` handler (no auth middleware - Cloud Run handles it)
- [x] 2.3.3 Test locally
- [x] 2.3.4 Commit

---

## Phase 3: Terraform Monitoring

**Commit:** `feat(terraform): add Cloud Monitoring for health checks`

### 3.1 Create Monitoring Module

Status: `[x]`

**Files:**
- `terraform/modules/monitoring/main.tf`
- `terraform/modules/monitoring/variables.tf`
- `terraform/modules/monitoring/outputs.tf`

**Resources:**
- Service account for uptime checks
- IAM binding: service account → Cloud Run invoker
- Uptime check (every 5 minutes)
- Log-based metric for per-RPC errors (with operation label)
- Log-based metric for health check failures
- Log-based metric for all errors (severity >= ERROR)
- Alert policy for uptime check failure (immediate)
- Alert policy for per-RPC error rate
- Alert policy for health check unhealthy
- Alert policy for any error logged (aggressive alerting with log content)
- Notification channel (email)

**Tasks:**
- [x] 3.1.1 Create module with service account
- [x] 3.1.2 Create uptime check
- [x] 3.1.3 Create log-based metrics (per-RPC errors, health check failures)
- [x] 3.1.4 Create alert policies (uptime, RPC errors, health unhealthy)
- [x] 3.1.5 Create notification channel
- [x] 3.1.6 Define variables and outputs

### 3.2 Configure Environments

Status: `[x]`

**Tasks:**
- [x] 3.2.1 Add module to `terraform/environments/dev/main.tf`
- [x] 3.2.2 Configure dev alert recipients
- [x] 3.2.3 Add module to `terraform/environments/prod/main.tf`
- [x] 3.2.4 Configure prod alert recipients
- [x] 3.2.5 Test terraform plan for both
- [x] 3.2.6 Commit

---

## Phase 4: Deploy and Validate

**Commit:** `docs: update observability documentation`

### 4.1 Dev Deployment

Status: `[x]`

**Tasks:**
- [x] 4.1.1 Deploy server to dev (health endpoint already deployed)
- [x] 4.1.2 Apply Terraform to dev
- [ ] 4.1.3 Verify uptime check runs
- [ ] 4.1.4 Verify health logs in Cloud Logging
- [ ] 4.1.5 Test alert by simulating failure

### 4.2 Prod Deployment

Status: `[x]`

**Tasks:**
- [x] 4.2.1 Deploy server to prod (health endpoint already deployed)
- [x] 4.2.2 Apply Terraform to prod
- [ ] 4.2.3 Verify uptime check runs
- [ ] 4.2.4 Monitor initial results

### 4.3 Documentation

Status: `[ ]`

**Tasks:**
- [ ] 4.3.1 Update `docs/server/observability.md`
- [ ] 4.3.2 Document alert investigation process
- [ ] 4.3.3 Commit

---

## File Summary

**Phase 1-2 files (completed):**
- `proto/ripls/api/health_service.proto` - Health check proto definitions
- `server/health/status.go` - Health status types
- `server/services/health/service.go` - HealthService RPC implementation
- `server/services/health/service_test.go` - Unit tests
- `server/integration_tests/health_test.go` - Integration tests
- `server/storage/bucket.go` - Added `CheckHealth()` to BucketStorage interface
- `server/storage/protosql.go` - Added `CheckHealth()` to ProtoSQLStorage
- `server/email/mailgun.go` - Added `CheckHealth()` to MailgunService
- `server/email/mock.go` - Added `CheckHealth()` to MockEmailService
- `server/location/mapbox.go` - Added `CheckHealth()` to MapboxClient
- `server/media/unsplash_provider.go` - Added `CheckHealth()` to UnsplashProvider
- `server/media/pexels_provider.go` - Added `CheckHealth()` to PexelsProvider
- `server/ai/provider_*.go` - Added `CheckHealth()` to all AI providers
- `server/main.go` - Register `/health` and health checkers

**Phase 3 files (completed):**
- `terraform/modules/monitoring/main.tf` - Monitoring resources (uptime check, metrics, alerts)
- `terraform/modules/monitoring/variables.tf` - Module input variables
- `terraform/modules/monitoring/outputs.tf` - Module outputs
- `terraform/environments/dev/main.tf` - Added monitoring module
- `terraform/environments/prod/main.tf` - Added monitoring module

---

## Change Log

| Date | Change |
|------|--------|
| 2025-01-15 | Initial plan |
| 2025-01-15 | Completed Phase 1 and Phase 2 - health endpoint fully implemented |
| 2025-01-15 | Added integration test for health checks with live dependencies |
| 2025-01-16 | Completed Phase 3 - Terraform monitoring module with uptime checks, per-RPC error metrics, and alerts applied to dev and prod |
| 2025-01-17 | Added aggressive error alerting - emails on every ERROR+ log with extracted error details |
