---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Go server architecture — Connect-RPC services, proto-SQL storage, service independence, shared-library patterns, testing strategy.
  globs: [server/**]
  lens: [architecture, server]
  alwaysApply: true
  domain: server
freshness:
  verified_commit: "f9888306d"
  verified_on: "2026-07-08"
---
# Server Architecture Guide

This document describes the architecture, design principles, and best practices for the ripls Go server.

## Table of Contents

1. [Overview](#overview)
2. [Core Design Principles](#core-design-principles)
3. [Service Architecture](#service-architecture)
4. [Shared Functionality Patterns](#shared-functionality-patterns)
5. [File Organization](#file-organization)
6. [Storage Layer](#storage-layer)
7. [Testing Strategy](#testing-strategy)
8. [Observability](#observability)
9. [Current State & Migration Path](#current-state--migration-path)

---

## Overview

The ripls server is a Go-based HTTP/JSON API built on Connect-Go (gRPC-compatible). It provides community building and gear sharing functionality through a collection of independent, loosely-coupled services.

### Architecture Layers

```
┌─────────────────────────────────────────────────────────────┐
│                     API Layer (HTTP/JSON)                   │
│                  Connect-Go Protocol Handlers               │
└────────────────────────┬────────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────────┐
│                   Service Layer (RPC Logic)                 │
│  community, gear, transfer, user, chat, location, media...  │
└────────────────────────┬────────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────────┐
│              Shared Libraries & Utilities                   │
│  storage, notifications, auth, email, location, ai           │
└────────────────────────┬────────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────────┐
│              External Dependencies                          │
│  PostgreSQL, Firebase, GCS, Mapbox, Mailgun, Vertex AI, ... │
└─────────────────────────────────────────────────────────────┘
```

### Key Components

**Protocol Buffers:**

- `proto/ripls/api/`: RPC API types (request/response messages)
- `proto/ripls/models/`: Storage/persistence model types
- Conventions for how these relate (API message shape, naming parity where fields round-trip, conversion location): [`../proto_conventions.md`](../proto_conventions.md)

**Server Services:**

- `server/services/`: RPC service implementations
- Each service implements a single Connect-Go service interface

**Shared Libraries:**

- `server/storage/`: Proto-SQL database abstraction
- `server/auth/`: User management, JWT tokens, OIDC
- `server/notifications/`: Push notifications (FCM) plus off-app channels (email, SMS)
- `server/email/`: Email delivery (Mailgun)
- `server/location/`: Geolocation services (Mapbox)
- `server/ai/`: AI providers for image analysis (Gemini/Vertex AI)
- `server/weather/`: Per-day weather providers for the Home calendar (Open-Meteo)
- `server/jobs/`: Background job scheduling

---

## Core Design Principles

### 1. Service Independence

**Services should not call or depend on other services.**

- ✅ Services implement only the RPC contract defined in API protos
- ✅ Services depend on shared libraries, not other services
- ❌ Services should NOT inject or call other service implementations

**Why:** Service-to-service dependencies create tight coupling, circular dependencies, and make services harder to test and evolve independently.

**Migration Path:**

- Factor out shared functionality into standalone libraries
- Inject only the minimal required abstractions (interfaces, not concrete services)

### 2. Shared Functionality in Libraries

**If two services need shared functionality, factor it into a shared library.**

**Examples of proper shared libraries:**

- `storage.ProtoSQLStorage`: Database access used by all services
- `auth.UserManager`: User CRUD operations
- `notifications.Service`: Push notification delivery
- `location.MapboxClient`: Geocoding and mapping
- `github.Client`: GitHub API operations with App authentication

**Pattern to follow:**

```go
// ✅ Good: Inject shared library
type GearService struct {
    storage    *storage.ProtoSQLStorage
    aiProvider ai.Provider // Optional interface
}

// ❌ Bad: Inject another service
type GearService struct {
    storage         *storage.ProtoSQLStorage
    transferService *transfer.Service // Don't do this
}
```

**Each shared library should:**

- Have a clear, single responsibility
- Include comprehensive unit tests
- Expose minimal public API surface
- Use interfaces for testability

### 3. Short, Focused Files

**Files should be short (< 1000 lines) and focused on related functionality.**

This is **enforced in CI** (#1424): `npm run lint:go:size` — wired into
`.github/workflows/test_go.yaml` — fails when any non-generated Go file under
`server/` exceeds 1,000 lines (excludes `*.pb.go`, `*_connect.go`, and `gen/`).
Split oversized files into sibling files in the same package using the naming
convention below. If a split is genuinely deferred, add a `// go-line-count-allow`
comment near the top of the file with a tracking-issue link; the gate skips files
carrying that comment. Do not add the hatch without an issue.

**Breaking down large services:**

Instead of:

```
services/community/
└── service.go (1,012 lines)
```

Break into:

```
services/community/
├── service.go          # Service struct, constructor (~100 lines)
├── membership.go       # Join, Leave, AddMember, RemoveMember
├── invitations.go      # Invite, AcceptInvite, DeclineInvite
├── gear_sharing.go     # AddCommunityGear, RemoveCommunityGear
├── events.go           # RecordEvent, GetEvents, GetActivity
└── authorization.go    # RequireMembership, IsMember helpers
```

**Benefits:**

- Easier code review and navigation
- Clear separation of concerns
- Simpler to test individual features
- Reduces merge conflicts

**File naming convention:**

- `service.go`: Service struct, constructor, and shared types
- Feature files: Name after functionality they implement (`membership.go`, `invitations.go`)
- Use lowercase with underscores for multi-word names (`gear_sharing.go`)
- Group related RPC handlers in the same file

### 4. Comprehensive Test Coverage

**No new functionality should be added without test coverage.**

**Testing Philosophy:**

- **Unit tests are always preferable to integration tests**
- Test functionality at the most local level possible to reduce interactions
- Mock external dependencies to isolate the code under test
- Integration and E2E tests complement unit tests but don't replace them

**Test levels:**

1. **Unit Tests** (required for all new code)

   - Test individual functions and methods in isolation
   - Use table-driven tests for multiple cases
   - Mock external dependencies (storage, API clients, other libraries)
   - Files: `*_test.go` alongside implementation
   - **Preferred:** Test at this level whenever possible

2. **Integration Tests** (required for major features)

   - Test complete RPC flows with real database
   - Use embedded PostgreSQL or SQLite
   - Verify authorization, validation, error handling
   - Location: `server/integration_tests/` (one `_test.go` file per feature)
   - **Use when:** You need to verify cross-component behavior

3. **End-to-End Tests** (required for critical user journeys)
   - Test against a local server by default, or a deployed server via `E2E_SERVER_URL`
   - Verify complete workflows (registration → create gear → loan → return)
   - File: `server/e2e_test.go`
   - **Use when:** Verifying complete user journeys across services

**Testing shared libraries:**

- Every shared library function should have unit tests
- Test both success and error paths
- Test edge cases and boundary conditions

**Mock Strategy:**

- Use a single, shared mock implementation where possible
- Define mocks in the library package (e.g., `storage/mock.go`)
- Reuse mocks across services rather than creating service-specific mocks
- Keep mocks simple and focused on the interface contract

---

## Service Architecture

### Service Structure

Each service is a Go package in `server/services/[service-name]/` that:

1. **Implements a single Connect-Go service interface** (generated from proto)
2. **Has a constructor that accepts dependencies** via dependency injection
3. **Organizes RPC handlers across multiple focused files**
4. **Maintains no state beyond injected dependencies**

### Standard Service Package Layout

```
services/[service-name]/
├── service.go              # Service struct, constructor, core setup
├── [feature1].go           # Related RPC handlers (e.g., membership.go)
├── [feature2].go           # More RPC handlers (e.g., invitations.go)
├── authorization.go        # (Optional) Service-specific auth helpers
├── service_test.go         # Tests for constructor and core functionality
├── [feature1]_test.go      # Tests for feature1
└── [feature2]_test.go      # Tests for feature2
```

### Service Constructor Pattern

```go
package community

import (
    "library/server/storage"
    "library/server/notifications"
)

// Service implements the CommunityService RPC interface.
type Service struct {
    storage             *storage.ProtoSQLStorage
    notificationService notifications.Service
    // Add only minimal dependencies (not other services)
}

// New creates a new community service instance.
func New(
    storage *storage.ProtoSQLStorage,
    notificationService notifications.Service,
) *Service {
    return &Service{
        storage:             storage,
        notificationService: notificationService,
    }
}

// NewForTesting creates a service instance with test helpers.
func NewForTesting(
    storage *storage.ProtoSQLStorage,
    notificationService notifications.Service,
) (*Service, chan struct{}) {
    notifComplete := make(chan struct{}, 100)
    return &Service{
        storage:             storage,
        notificationService: notificationService,
        testSignal:          notifComplete,
    }, notifComplete
}
```

### RPC Handler Pattern

Each RPC handler:

- Accepts `context.Context` and request message
- Returns response message and error
- Uses `connect.Error` for gRPC-compatible errors
- Validates inputs before processing
- Checks authorization using `auth.UserFromContext()`

```go
// CreateCommunity handles the CreateCommunity RPC.
func (s *Service) CreateCommunity(
    ctx context.Context,
    req *connect.Request[apipb.CreateCommunityRequest],
) (*connect.Response[apipb.CreateCommunityResponse], error) {
    // 1. Get authenticated user
    userInfo, err := auth.UserFromContext(ctx)
    if err != nil {
        return nil, connect.NewError(connect.CodeUnauthenticated, err)
    }

    // 2. Validate request
    if req.Msg.Name == "" {
        return nil, connect.NewError(
            connect.CodeInvalidArgument,
            fmt.Errorf("name is required"),
        )
    }

    // 3. Business logic
    community := &modelspb.Community{
        Id:          uuid.New().String(),
        Name:        req.Msg.Name,
        Description: req.Msg.Description,
        CreatedBy:   userInfo.ID,
        CreatedAt:   timestamppb.Now(),
    }

    // 4. Persist to storage
    if err := s.storage.Insert(ctx, community); err != nil {
        return nil, connect.NewError(connect.CodeInternal, err)
    }

    // 5. Return response
    return connect.NewResponse(&apipb.CreateCommunityResponse{
        Community: convertToAPICommunity(community),
    }), nil
}
```

### Authorization Patterns

**Principle:** Factor shared authentication checks into `server/auth` and reuse them across services. For complex but service-specific authorization logic, use helper functions in `authorization.go` within the service package.

**Common auth patterns in `server/auth`:**

- `UserFromContext(ctx)`: Extract authenticated user from request context
- `RequireRole(ctx, role)`: Verify user has required role
- `RequireUser(ctx, userID)`: Verify user is accessing their own resources

**Option 1: Inline authorization checks**

```go
func (s *Service) UpdateGear(ctx context.Context, req *Request) (*Response, error) {
    user, err := auth.UserFromContext(ctx)
    if err != nil {
        return nil, connect.NewError(connect.CodeUnauthenticated, err)
    }

    gear, err := s.storage.GetByID(ctx, "gear", req.Msg.GearId, &modelspb.Gear{})
    if err != nil {
        return nil, connect.NewError(connect.CodeNotFound, err)
    }

    if gear.(*modelspb.Gear).OwnerId != user.ID {
        return nil, connect.NewError(connect.CodePermissionDenied,
            fmt.Errorf("not authorized"))
    }

    // ... proceed with update
}
```

**Option 2: Helper functions in same package**

```go
// In authorization.go
func requireOwnership(ctx context.Context, storage *storage.ProtoSQLStorage,
    gearID string) (*modelspb.Gear, error) {
    user, err := auth.UserFromContext(ctx)
    if err != nil {
        return nil, connect.NewError(connect.CodeUnauthenticated, err)
    }

    gear, err := storage.GetByID(ctx, "gear", gearID, &modelspb.Gear{})
    if err != nil {
        return nil, connect.NewError(connect.CodeNotFound, err)
    }

    if gear.(*modelspb.Gear).OwnerId != user.ID {
        return nil, connect.NewError(connect.CodePermissionDenied,
            fmt.Errorf("not authorized"))
    }

    return gear.(*modelspb.Gear), nil
}

// In gear_updates.go
func (s *Service) UpdateGear(ctx context.Context, req *Request) (*Response, error) {
    gear, err := requireOwnership(ctx, s.storage, req.Msg.GearId)
    if err != nil {
        return nil, err
    }

    // ... proceed with update
}
```

---

## Shared Functionality Patterns

### Pattern 1: Standalone Libraries for Shared Logic

**Use Case:** Multiple services need impact estimation, but shouldn't depend on another service.

**Current (❌ Bad):**

```go
// Service depends on another service
type TransferService struct {
    impactService *impact.Service
}

func (s *TransferService) CompleteTransfer(...) {
    s.impactService.EstimateImpact(...)
}
```

**Refactored (✅ Good):**

```go
// Factor impact estimation into standalone library
package impact

// Estimator provides impact calculation without service dependencies.
type Estimator struct {
    storage *storage.ProtoSQLStorage
}

func NewEstimator(storage *storage.ProtoSQLStorage) *Estimator {
    return &Estimator{storage: storage}
}

// EstimateImpact calculates impact metrics for a completed transfer.
func (e *Estimator) EstimateImpact(
    ctx context.Context,
    gearID string,
) (*ImpactEstimate, error) {
    // Implementation...
}
```

**Usage in services:**

```go
// Transfer service uses impact estimator
type TransferService struct {
    storage         *storage.ProtoSQLStorage
    impactEstimator *impact.Estimator
}

func (s *TransferService) CompleteTransfer(...) {
    // Use the estimator directly
    s.impactEstimator.EstimateImpact(ctx, gearID)
}
```

**Benefits:**

- Clear separation: library has no RPC handlers
- Testable: unit test estimator independently
- Reusable: multiple services use same logic
- No circular dependencies

### Pattern 2: Interface-Based Dependencies

**Use Case:** Service needs optional functionality (AI, email) that may not be available.

**Pattern:**

```go
// Define interface in shared package
package ai

type Provider interface {
    DetectGearInImage(ctx context.Context, imageURL string) ([]string, error)
}
```

**Service accepts interface:**

```go
type GearService struct {
    storage    *storage.ProtoSQLStorage
    aiProvider ai.Provider // Optional: can be nil
}

func (s *Service) AddGear(ctx context.Context, req *Request) (*Response, error) {
    // Use AI provider if available
    if s.aiProvider != nil && req.Msg.ImageUrl != "" {
        tags, _ := s.aiProvider.DetectGearInImage(ctx, req.Msg.ImageUrl)
        gear.AiTags = tags
    }

    // Continue without AI if not available
    // ...
}
```

**Benefits:**

- Graceful degradation
- Easy to mock in tests
- Optional features don't block core functionality

### Pattern 3: Constructor Injection

**Use Case:** Service needs core dependencies like storage, auth.

**Pattern:** Inject via constructor (current pattern, keep this)

```go
func New(
    storage *storage.ProtoSQLStorage,
    bucket storage.BucketStorage,
) *Service {
    return &Service{
        storage: storage,
        bucket:  bucket,
    }
}
```

**When to use:**

- Storage: Always required
- Auth: Middleware provides context-based auth
- External clients: Mapbox, GCS bucket, email

**Benefits:**

- Explicit dependencies
- Easy to test (inject mocks)
- Idiomatic Go pattern
- Type-safe

### Pattern 4: Domain Event Buses

**Use Case:** One domain action (e.g., a chat message sent) must trigger multiple downstream effects (stream fan-out, push notifications) without the emitting service knowing about them.

Two domain-specific adapters sit on top of `server/pubsub`:

| Package | Topic name | What it carries |
|---------|-----------|-----------------|
| `server/community_event_bus` | `community_events` | `CommunityEvent` rows (also inserts the row) |
| `server/chat_event_bus` | `chat_messages` | `ChatMessage` publishes (row already inserted by caller) |

**Publish side (emitter service):**

```go
// In server/services/chat — after inserting the message row.
s.bus.Publish(ctx, chat_event_bus.KindUserMessage, message, conversation,
    chat_event_bus.WithMentionedUserIDs(mentionedUserIDs))
```

**Subscribe side (wired in the composition root, `server/wiring.go`):**

```go
chatNotifSub := chat_subscriber.New(sqlStorage, notificationService)
chatNotifSub.SetStreamChecker(chatService.HasActiveStream)
chatNotifSub.SetForegroundChecker(chatService.IsUserInForeground)
chatEventBus.Subscribe(chatNotifSub)
chatEventBus.Subscribe(chatService.StreamSubscriber())
```

Subscribers are registered **after** their dependent services are constructed so that late-bound hooks (stream registry, foreground presence) can be wired without circular imports.

---

## File Organization

### File Size Guidelines

**Target:** < 500 lines per file (ideally)
**Maximum:** < 1000 lines per file
**If larger:** Break into smaller files by functionality

**When to split a file:**

- File exceeds 1000 lines
- File contains unrelated RPC handlers
- Testing becomes difficult due to file size
- Code reviews are hard to follow

**How to split:**

1. Identify logical groupings of RPC handlers
2. Create new files for each group (e.g., `membership.go`, `invitations.go`)
3. Keep constructor and struct definition in `service.go`
4. Move service-specific auth helpers to `authorization.go`

### Shared Library Organization

```
server/[library-name]/
├── [library].go          # Main interface and types
├── [impl1].go            # First implementation
├── [impl2].go            # Alternative implementation
├── [library]_test.go     # Tests for public API
├── [impl1]_test.go       # Tests for implementation 1
└── [impl2]_test.go       # Tests for implementation 2
```

**Example: Notifications**

```
server/notifications/
├── service.go            # NotificationService interface
├── fcm.go                # Firebase Cloud Messaging provider
├── noop.go               # No-op provider for testing
├── mock.go               # Mock implementation for tests
├── service_test.go       # Tests
└── fcm_test.go           # FCM-specific tests
```

---

## Storage Layer

### Proto-SQL Pattern

The storage layer uses a generic Proto-SQL abstraction that automatically:

- Creates tables from protobuf message definitions
- Handles type conversions between proto and SQL
- Provides generic CRUD operations
- Stores both flattened fields (for querying) and serialized proto (for integrity)

**Core abstraction:** `storage.ProtoSQLStorage`

### Database Abstraction

```go
// DatabaseSpecifics abstracts SQL dialect differences.
type DatabaseSpecifics interface {
    SQLType(field *descriptor.FieldDescriptorProto) string
    Placeholder(index int) string
    OpenDatabase(connectionString string) (*sql.DB, error)
}
```

**Implementation:**

- `postgresql.go`: PostgreSQL-specific SQL dialect

The server uses PostgreSQL exclusively. Connection strings must use the
`postgres://` or `postgresql://` format.

### Storage Operations

```go
// Generic CRUD operations
storage.Insert(ctx, message)
storage.GetByID(ctx, table, id, message)
storage.Update(ctx, message)
storage.Delete(ctx, table, id)

// Queries
storage.QueryByField(ctx, table, field, value, message)
storage.QueryByFields(ctx, table, fieldMap, message)
```

### Field Flattening

Nested proto messages are flattened to SQL columns:

```protobuf
message Gear {
    string id = 1;
    string name = 2;
    Geolocation location = 3;  // Nested message
}

message Geolocation {
    double latitude_deg = 1;
    double longitude_deg = 2;
}
```

SQL schema:

```sql
CREATE TABLE gear (
    id TEXT PRIMARY KEY,
    name TEXT,
    location_latitude_deg REAL,
    location_longitude_deg REAL,
    proto_data BLOB  -- Full serialized proto
)
```

### Storage Best Practices

1. **Use storage layer for all persistence**

   - Don't write raw SQL in services
   - Use `storage.QueryByField()` for simple queries
   - Add new storage methods for complex queries

2. **Keep storage logic in storage package**

   - Don't leak SQL details to services
   - Add helper methods to `ProtoSQLStorage` for common patterns

3. **Transaction support**
   - Use `storage.BeginTx()` for multi-operation transactions
   - Always defer `tx.Rollback()` and check `tx.Commit()`

4. **SQL safety is CI-enforced**
   - Values are always bound with `dbSpec.Placeholder(n)`; identifiers always
     go through `quoteIdent` (`server/storage/sqlident.go`)
   - `npm run lint:go:sql` (`server/cmd/check-sql`) fails the build on anything
     else, including string concatenation into a query
   - Full rules, and why gosec's G201 cannot cover this package, in
     [`conventions.md`](conventions.md) § SQL Safety

---

## Testing Strategy

### 1. Unit Tests

**Required for:** All new functions, methods, and shared libraries

**Pattern:**

```go
func TestSomething(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"valid input", "test", "TEST", false},
        {"empty input", "", "", true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Something(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("got error %v, wantErr %v", err, tt.wantErr)
            }
            if got != tt.want {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

**Mocking dependencies:**

- Use interfaces for external dependencies
- Create mock implementations in test files
- Inject mocks via constructors

**Test file naming:**

- `[filename]_test.go` alongside implementation
- Keep tests close to the code they test

### 2. Integration Tests

**Required for:** Major features, complete RPC flows

**Location:** `server/integration_tests/` (one `_test.go` file per feature)

**Pattern:**

```go
func TestCommunityFlow(t *testing.T) {
    // Setup: Create test server with embedded database
    tempDir := t.TempDir()
    storage, err := setupTestStorage(tempDir)
    require.NoError(t, err)

    server := setupTestServer(storage)
    defer server.Close()

    client := createTestClient(server.URL)

    // Test: Complete user journey
    t.Run("create community", func(t *testing.T) {
        resp, err := client.CreateCommunity(ctx, &api.CreateCommunityRequest{
            Name: "Test Community",
        })
        require.NoError(t, err)
        assert.NotEmpty(t, resp.Community.Id)
    })

    t.Run("join community", func(t *testing.T) {
        // ... test join flow
    })
}
```

**Use testcontainers:**

Tests use testcontainers-go with the `pgvector/pgvector:pg16` image. Docker must
be running.

**Test isolation:**

- Each test creates a unique database within a shared container
- No shared state between tests
- Cleanup handled automatically by `SetupTestStorage(t)`

### 3. End-to-End Tests

**Required for:** Critical user journeys, cross-service workflows

**Location:** `server/e2e_test.go`

**Pattern:**

```go
func TestEndToEnd_LoanLifecycle(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping e2e test in short mode")
    }

    // Resolve the server URL: a deployed server if E2E_SERVER_URL is set,
    // otherwise a local server started once with a testcontainers database.
    serverURL := getTestServerURL(t)

    client := createClient(serverURL)

    // Test complete user journey
    t.Run("register users", func(t *testing.T) { /* ... */ })
    t.Run("create gear", func(t *testing.T) { /* ... */ })
    t.Run("request loan", func(t *testing.T) { /* ... */ })
    t.Run("approve and pickup", func(t *testing.T) { /* ... */ })
    t.Run("return and rate", func(t *testing.T) { /* ... */ })
}
```

**Run against a server:**

- Default: a local server started once with a testcontainers database
- Point at a deployed server with the `E2E_SERVER_URL` environment variable
- Use longer timeouts: `-timeout 10m`

**Test real integrations:**

- Push notifications (FCM)
- Email delivery (Mailgun)
- Media storage (GCS)
- AI providers (Vertex AI)

### Running Tests

```bash
# All tests (unit + integration, excludes e2e)
go test ./...

# Integration tests only (with embedded database)
go test ./server/integration_tests

# End-to-end tests (local server started automatically)
go test -run TestEndToEnd -v -timeout 10m ./server
# Or against a deployed server
E2E_SERVER_URL=https://custom-server.com go test -run TestEndToEnd -v -timeout 10m ./server
```

---

## Observability

The server implements comprehensive observability through structured logging, health monitoring, and cloud-native alerting. For full details, see `docs/server/observability.md`.

### Structured Logging

All logging uses Go's `log/slog` package with JSON output in production and human-readable text in development. Key features:

- **Request Correlation**: Every HTTP request receives a UUID (from `X-Request-ID` header or auto-generated) that propagates through all log entries via `context.Context`
- **Sensitive Data Masking**: Emails and tokens are never logged in plaintext. Use `logging.MaskEmail()` and `logging.MaskToken()` for automatic redaction
- **Context-Aware Logging**: Services extract loggers via `logging.LoggerWithContext(ctx)` which automatically includes request_id, user_id, and other ambient context

**Standard field names**: `request_id`, `user_id`, `operation`, `duration_ms`, `error`, plus entity-specific fields (`gear_id`, `community_id`, etc.)

**Log levels**:
- `DEBUG`: Verbose operational details
- `INFO`: Normal operations worth recording
- `WARN`: Unexpected but handled situations
- `ERROR`: System failures requiring attention (not user errors)

### Health Monitoring

Two endpoints provide health visibility:

- **`/health` (HTTP GET)**: Simple endpoint for uptime checks. Returns JSON with dependency status and HTTP 200/503
- **`HealthService.CheckHealth` (Connect RPC)**: Full programmatic health check with detailed dependency information

Dependencies register health checkers at startup:
- **Core**: Database (PostgreSQL ping + pool stats), Storage (GCS/local), Email (Mailgun domain verification)
- **Optional**: AI providers (Anthropic, OpenAI, Gemini), Location (Mapbox token validation), Stock imagery (Unsplash, Pexels)

### Cloud Monitoring Integration

Terraform-managed monitoring infrastructure (`terraform/modules/monitoring/`) provides:

- **Uptime Checks**: Cloud Monitoring calls `/health` every 5 minutes
- **Log-Based Metrics**: Custom metrics for per-RPC error counts and health check failures
- **Alert Policies**: Email notifications for uptime failures, elevated error rates, and unhealthy dependencies

---

## Additional Resources

- **Protocol Buffers:** `proto/ripls/api/` and `proto/ripls/models/`
- **Code Generation:** See `CLAUDE.md` for `npm run generate` instructions
- **Storage Layer:** `server/storage/protosql.go` implementation details
- **Deployment:** `terraform/README.md` for infrastructure

---

## Goroutine Safety

Every goroutine spawned in `server/` must be protected against panics. A panic in a bare `go func()` crashes the entire server process because the Connect middleware cannot catch panics in goroutines that outlive the originating request.

### Fire-and-forget goroutines

Use `logging.GoSafe(ctx, "name", fn)` from `server/logging`. It wraps the function in a `defer recover()` and logs the panic at Error level with the goroutine name, panic value, and stack trace.

```go
logging.GoSafe(ctx, "process-background-task", func() {
    doBackgroundWork(context.Background(), payload)
})
```

### Goroutines with terminal channels

When a goroutine must signal a result on a channel (so the consumer doesn't block forever), use an inline recover defer registered **after** any `close(ch)` defers. LIFO ordering means the recover defer runs first on panic, writes a synthetic error to the channel while it is still open, and then the close defers fire.

```go
go func() {
    defer close(final)
    defer func() {
        if r := recover(); r != nil {
            logging.LoggerWithContext(ctx).ErrorContext(ctx, "panic in streaming goroutine",
                "goroutine", "my-operation",
                "panic", r,
                "stack", string(debug.Stack()),
            )
            final <- MyFinalType{Err: fmt.Errorf("panic in my-operation: %v", r)}
        }
    }()
    // ... normal body
}()
```

See `server/streaming/sender.go` for the canonical example.

### CI enforcement

`server/cmd/check-goroutines` walks the `server/` tree and fails if any `go` statement spawns a function literal without a recover defer. It runs as part of `npm run lint:go`. Two files are allowlisted: `server/logging/logger.go` (defines `GoSafe`) and `server/streaming/sender.go` (uses the inline-recover shape). Test files (`_test.go`) are exempt.

## Error wrapping

Connect serializes a returned error's `Error()` string into the response body. A bare `connect.NewError(connect.CodeInternal, err)` therefore leaks raw storage messages, JSON decoder errors, third-party API bodies, and similar low-signal noise to clients (and to client crash analytics, where it drowns out real failures).

Use `server/connecterr.Internal(ctx, op, err, kv...)` instead. It logs the wrapped error server-side via `logging.LoggerWithContext(ctx)` and returns a `connect.CodeInternal` whose public `Error()` is the fixed string `"internal server error"`. The original `err` is never serialized to the wire.

```go
gear, err := s.storage.GetByID(ctx, req.Msg.Id)
if err != nil {
    return nil, connecterr.Internal(ctx, "SaveGear.GetByID", err,
        "gear_id", req.Msg.Id,
    )
}
```

The bare pattern is gated by `scripts/check_codeinternal.js`, run as part of `npm run lint:go` and CI. Any occurrence in non-test, non-generated code outside `server/connecterr/` fails the build.

---

## Document Maintenance

This document should be updated when:

- New architectural patterns are introduced
- Design principles change or evolve
- Major refactoring efforts are completed
- New shared libraries are added

**Last updated:** 2025-11-13
