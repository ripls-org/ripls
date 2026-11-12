# Simulation System Plan

## Context

Testing the Ripls app currently relies on documented workflow integration tests (which test specific paths) and manual data entry by logging in as various users. Neither approach generates the volume or variety of data needed to evaluate the app experience with realistic communities. This plan creates a simulation system that populates a server instance with multiple communities of varying sizes, persona-driven user activity, realistic timelines, and actual gear/event images — all via standard RPC calls against a running server.

## Design Overview

### Components

1. **Dev mode flag** — A single `-dev-mode` flag (replacing both `-dev-auth` and new simulation capabilities) gates all development/simulation features
2. **Simulation clock** — HTTP middleware that reads `X-Simulation-Timestamp` header from requests and injects it into Go context; a `clock.Now(ctx)` helper replaces `time.Now()` at business-timestamp call sites
3. **CleanupSimulation admin RPC** — Selectively deletes all data tagged with a given simulation_id, leaving real data intact
4. **Simulation package** (`server/simulation/`) — Go code defining scenarios, personas, gear catalogs, and a runner that executes activity sequences via RPC calls
5. **Media assets** — Curated set of images for gear and experiences, uploaded via MediaService

### Architecture Diagram

```
┌─────────────────────────────────────────────┐
│          server/simulation/                  │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐ │
│  │ Scenarios │  │ Personas │  │ Catalogs  │ │
│  └────┬─────┘  └────┬─────┘  └─────┬─────┘ │
│       └──────┬───────┘              │       │
│         ┌────▼──────────────────────▼──┐    │
│         │         Runner               │    │
│         │  (RPC client + auth + clock) │    │
│         └──────────┬───────────────────┘    │
└────────────────────┼────────────────────────┘
                     │ HTTP + X-Simulation-Timestamp header
                     ▼
┌─────────────────────────────────────────────┐
│         Live Server (-dev-mode)              │
│  ┌─────────────────────────────────────┐    │
│  │ SimulationClock middleware           │    │
│  │  (reads header → context value)     │    │
│  └─────────────┬───────────────────────┘    │
│  ┌─────────────▼───────────────────────┐    │
│  │ RPC Services                         │    │
│  │  (call clock.Now(ctx) for timestamps)│    │
│  └─────────────────────────────────────┘    │
│  ┌─────────────────────────────────────┐    │
│  │ AdminService.CleanupSimulation        │    │
│  │  (cascade delete by simulation_id)  │    │
│  └─────────────────────────────────────┘    │
└─────────────────────────────────────────────┘
```

---

## Design Details

### 1. Dev Mode Flag (consolidating `-dev-auth` → `-dev-mode`)

Replace the existing `-dev-auth` flag with a single `-dev-mode` flag in `server/main.go`:

```go
devMode := flag.Bool("dev-mode", false,
    "Enable development mode (dev authentication, simulation clock, database reset)")
```

When enabled, this flag controls:
- Development authentication endpoints (currently gated by `-dev-auth`)
- Simulation clock HTTP middleware (new)
- `ResetDatabase` admin RPC (new)
- Startup warning: "DEV MODE ENABLED — do not use in production"

**Migration**: All existing uses of `-dev-auth` in code, tests, scripts, Dockerfile, package.json, and terraform change to `-dev-mode`.

**Files to modify:**
- `server/main.go` — rename flag, wire up simulation clock + reset
- `server/integration_tests/test_helpers.go` — update flag name in `startTestServer()`
- `server/e2e_test.go` — update flag name
- `server/cmd/patrol-test-server/main.go` — update flag name
- `Dockerfile` — `ENABLE_DEV_AUTH` → `ENABLE_DEV_MODE`
- `package.json` — `--dev-auth` → `--dev-mode`
- `terraform/modules/containers/variables.tf` — `enable_dev_auth` → `enable_dev_mode`
- `terraform/modules/containers/main.tf` — env var rename
- `terraform/environments/dev/main.tf` — variable rename
- `README.md`, `server/README.md` — documentation updates

### 2. Simulation Clock

**New package: `server/clock/`**

`server/clock/clock.go`:
```go
package clock

// Now returns the simulation timestamp from context if present, otherwise time.Now().
func Now(ctx context.Context) time.Time { ... }

// UnixSec returns Now(ctx).Unix() for convenience.
func UnixSec(ctx context.Context) int64 { ... }
```

`server/clock/middleware.go`:
```go
// SimulationTimestamp returns HTTP middleware that reads
// X-Simulation-Timestamp header and injects it into the request context.
func SimulationTimestamp(next http.Handler) http.Handler { ... }
```

The middleware reads `X-Simulation-Timestamp` header (int64 unix seconds), parses it, and stores it in the context via a private context key. When the header is absent, `clock.Now(ctx)` falls back to `time.Now()`.

**Call sites to update** (replace `time.Now().Unix()` with `clock.UnixSec(ctx)`):

These are the business-logic timestamps that affect user-visible data. Logging, performance timing, and AI timing remain unchanged.

| File | Current call | Purpose |
|------|-------------|---------|
| `server/services/transfer/state_machine.go` | `time.Now().Unix()` | actual_pickup, actual_return |
| `server/services/transfer/interest.go` | `time.Now().Unix()` | latest_request timestamp |
| `server/services/request/lifecycle.go` | `time.Now().Unix()` | request shared_at, deleted_at |
| `server/services/request/offers.go` | `time.Now().Unix()` | offer created_at |
| `server/services/request/sharing.go` | `time.Now().Unix()` | request shared_at (multi-community) |
| `server/services/request/stats.go` | `time.Now().Unix()` | days open calculation |
| `server/services/experience/lifecycle.go` | `time.Now().Unix()` | experience started/completed |
| `server/services/experience/rsvp.go` | `time.Now().Unix()` | RSVP timestamps |
| `server/services/experience/share.go` | `time.Now().Unix()` | experience shared_at, event occurred_at |
| `server/services/experience/time_proposals.go` | `time.Now().Unix()` | proposal timestamps |
| `server/services/experience/delete.go` | `time.Now().Unix()` | soft delete timestamp |
| `server/services/community/lifecycle.go` | `time.Now().Unix()` | community created_at, updated_at, deleted_at |
| `server/services/community/notifications.go` | `time.Now().Unix()` | event occurred_at |
| `server/services/community/gear_sharing.go` | `time.Now().Unix()` | gear sharing created_at |
| `server/services/community/invitations.go` | `time.Now().Unix()` | invitation created_at, membership created_at |
| `server/services/community/regions.go` | `time.Now().Unix()` | region timestamps |
| `server/services/login/registration.go` | `time.Now().Unix()` | user created_at, community event |
| `server/services/login/password_reset.go` | `time.Now().Unix()` | reset token timestamps |
| `server/services/chat/messages.go` | `time.Now().Unix()` | message created_at |
| `server/services/chat/mention_handler.go` | `time.Now().Unix()` | mention timestamp |

### 3. CleanupSimulation Admin RPC & Simulation ID

Each simulation run generates a human-readable `simulation_id` formatted as `sim-scenario-name` (e.g., `sim-suburban-neighborhood`). The ID is date-independent so that rerunning the same scenario overwrites existing data rather than orphaning it. This ID is stored on two root entity types:

- `User.simulation_id` (optional string, field 17 in models proto)
- `Community.simulation_id` (optional string, field 8 in models proto)

The `CleanupSimulation` RPC takes a `simulation_id` and cascades deletion through all associated records, leaving manually-created data untouched. This allows simulation data to coexist with real community data on the same server instance.

**Proto addition** to `proto/ripls/api/admin_service.proto`:

```protobuf
message CleanupSimulationRequest {
  string simulation_id = 1;
}

message CleanupSimulationResponse {
  int64 users_deleted = 1;
  int64 communities_deleted = 2;
  int64 records_deleted = 3;
  int64 duration_milliseconds = 4;
}

rpc CleanupSimulation(CleanupSimulationRequest)
    returns (CleanupSimulationResponse) {}
```

**Cascade deletion order** (respects foreign key dependencies):

1. Find all user IDs where `simulation_id` matches
2. Find all community IDs where `simulation_id` matches
3. Delete records that reference these users or communities:
   - EquityTransaction, CommunityEquity, UserEquity, GearEquity, ExperienceEquity, RequestEquity (equity records)
   - CommunityEvent (events referencing these communities)
   - ExperienceRSVP, ExperienceTimeProposal, TimeVote (experience participation)
   - RequestOffer (request participation)
   - ChatMessage, ChatConversation (conversations in these communities)
   - Transfer (transfers by these users or in these communities)
   - CommunityGear, CommunityRequest, CommunityExperience (junction tables)
   - Experience (owned by these users)
   - Request (by these users)
   - Gear (owned by these users)
   - CommunityRegion, CommunityInvitationLink (community metadata)
   - CommunityUser (memberships)
   - FeedItemView, UserDevice, PendingPasswordReset, UserLocation (user metadata)
   - Media (uploaded by these users)
   - Story (in these communities)
   - Community (the communities themselves)
   - User (the users themselves)

**Implementation notes:**
- Gated behind `devMode` flag on AdminService
- Uses SQL `DELETE FROM table WHERE field IN (...)` for each table
- Bucket storage media files are NOT cleared (they have unique IDs and are harmless)
- Returns counts of users, communities, and total records deleted

**Files to modify:**
- `proto/ripls/models/user.proto` — add `optional string simulation_id` field
- `proto/ripls/models/community.proto` — add `optional string simulation_id` field
- `proto/ripls/api/admin_service.proto` — add CleanupSimulation RPC + messages
- `server/services/admin/admin_service.go` — add `devMode bool` field, update constructor
- `server/services/admin/cleanup.go` — new file, implements CleanupSimulation
- `server/main.go` — pass devMode to admin service constructor

### 4. Simulation Package (`server/simulation/`)

#### Package Structure

```
server/simulation/
├── runner.go             # Main simulation runner
├── runner_test.go        # Runner tests (against test server)
├── client.go             # Authenticated RPC client wrapper with timestamp support
├── types.go              # Core types (Scenario, CommunityDef, MemberDef, etc.)
├── scenarios.go          # Built-in scenario definitions
├── personas.go           # Persona types and behavior weights
├── activity.go           # Activity generation engine
├── activity_test.go      # Activity generation tests
├── state.go              # Simulation state tracker (available gear, active transfers, etc.)
├── gear_catalog.go       # Curated gear items by category
├── experience_catalog.go # Curated experience templates
├── request_catalog.go    # Curated request templates
├── media.go              # Media upload utilities
└── assets/               # Bundled images for simulation
    ├── gear/             # Gear images (30-50 photos)
    └── experiences/      # Experience images (10-20 photos)
```

#### Core Types

```go
// Scenario defines a complete community simulation.
type Scenario struct {
    Name        string
    Description string
    Communities []CommunityDef
    StartTime   time.Time // Simulated start (e.g., 6 months ago)
    EndTime     time.Time // Simulated end (e.g., now)
}

// CommunityDef defines a community and its members.
type CommunityDef struct {
    Name        string
    Description string
    Region      string        // e.g., "Portland, OR"
    Members     []MemberDef
}

// MemberDef defines a user and their persona.
type MemberDef struct {
    Name    string
    Email   string
    Persona PersonaType
}

// PersonaType maps to the persona framework in docs/users/personas.md.
type PersonaType int
const (
    PersonaAlfred  PersonaType = iota // Stuff x Giving — lots of gear, generous lender
    PersonaDerek                      // Stuff x Exchanging — selective, lends & borrows
    PersonaGary                       // Stuff x Seeking — borrows a lot, little to share
    PersonaBetty                      // Time x Giving — hosts many experiences
    PersonaEmma                       // Time x Exchanging — occasional participant
    PersonaHenry                      // Time x Seeking — makes requests for help
    PersonaCarmen                     // Money x Giving — generous (future: fundraising)
    PersonaFiona                      // Money x Exchanging — balanced (future)
    PersonaIan                        // Money x Seeking — needs support (future)
)
```

#### Multi-User Activity Modeling

Realistic community activity involves multiple users interacting on the same items. The simulation supports these multi-user patterns:

**1. Contested Gear (multiple borrowers expressing interest)**
When a gear item is popular, multiple users may express interest before the owner selects:
```
Activity flow:
  T+0h: Gary expresses interest in Alfred's drill
  T+2h: Derek expresses interest in Alfred's drill
  T+4h: Emma expresses interest in Alfred's drill
  T+8h: Alfred selects Gary as recipient
  T+8h: Derek's and Emma's interest is implicitly declined
  T+9h: Alfred starts loan to Gary
  T+14d: Gary returns drill -> CompleteTransfer
```

**2. Sequential Loans (same item loaned multiple times)**
Popular items get loaned repeatedly over the simulation period:
```
Activity flow:
  Month 1: Alfred loans drill to Gary (2 weeks)
  Month 2: Alfred loans drill to Emma (1 week)
  Month 4: Alfred loans drill to Derek (3 weeks)
```
The state tracker marks gear as UNAVAILABLE during active loans.

**3. Giveaways with Multiple Requesters**
Similar to contested gear, but the transfer completes immediately after recipient selection.

**4. Requests with Multiple Helpers**
A request may receive offers from several community members:
```
Activity flow:
  T+0h: Henry submits request "Need help moving furniture this Saturday"
  T+2h: Alfred offers to help
  T+4h: Betty offers to help
  T+6h: Emma offers to help
  T+48h: Henry marks request fulfilled (confirms Alfred and Betty as helpers)
```

**5. Experiences with Many RSVPs**
Experiences (especially Betty-hosted ones) attract many attendees:
```
Activity flow:
  T+0h: Betty creates "Community BBQ" experience
  T+1d: Alfred RSVPs YES
  T+1d: Gary RSVPs YES
  T+2d: Derek RSVPs MAYBE
  T+2d: Emma RSVPs YES
  T+3d: Henry RSVPs YES
  T+7d: Betty marks experience IN_PROCESS
  T+7d+3h: Betty completes experience (records attendance)
```

#### State Tracking (`state.go`)

The simulation maintains a `SimulationState` struct that tracks:
```go
type SimulationState struct {
    // User state
    UserTokens    map[string]string // email -> auth token
    UserIDs       map[string]string // email -> user ID

    // Community state
    CommunityIDs  map[string]string // community name -> ID

    // Gear state (per community)
    GearByOwner   map[string][]GearState // owner email -> their gear items
    AvailableGear []GearState            // gear currently available for borrowing

    // Active entities
    ActiveTransfers   []TransferState   // loans/giveaways in progress
    ActiveRequests    []RequestState    // unfulfilled requests
    ActiveExperiences []ExperienceState // upcoming/in-progress experiences

    // Media
    MediaIDs map[string]string // asset filename -> uploaded media ID
}
```

#### Persona Behavior Weights

Each persona has weights that determine their activity mix. Activities are generated per-period (e.g., per simulated week):

| Persona | Share Gear | Borrow | Give Away | Host Experience | Attend Experience | Make Request | Offer Help |
|---------|-----------|--------|-----------|-----------------|-------------------|--------------|------------|
| Alfred  | 0.35      | 0.05   | 0.05      | 0.05            | 0.15              | 0.05         | 0.10       |
| Derek   | 0.15      | 0.20   | 0.05      | 0.05            | 0.15              | 0.10         | 0.10       |
| Gary    | 0.05      | 0.35   | 0.00      | 0.00            | 0.20              | 0.20         | 0.05       |
| Betty   | 0.05      | 0.05   | 0.00      | 0.35            | 0.15              | 0.05         | 0.20       |
| Emma    | 0.05      | 0.10   | 0.00      | 0.10            | 0.30              | 0.10         | 0.15       |
| Henry   | 0.02      | 0.10   | 0.00      | 0.03            | 0.10              | 0.40         | 0.05       |

A "base activity rate" per persona controls volume (Alfred might do 4 activities/week, Gary 2).

Additionally, personas have a **contention probability** that determines how likely they are to express interest in already-requested items (higher for Gary, lower for Alfred).

#### Activity Generation (Deterministic)

Activities are generated using a seeded PRNG (`math/rand` with fixed seed) for reproducibility:

1. **Setup phase** (at StartTime):
   - Register users, create communities, upload media
   - Each persona shares their initial gear inventory

2. **Activity generation** — for each simulated week in `[StartTime, EndTime]`:
   a. For each community member:
      - Roll dice against persona weights to determine activities for this week
      - Distribute activities randomly within the week
   b. For multi-step activities, generate the full step sequence with realistic delays:
      - Loan: interest(+0h) -> additional_interest(+2-24h each) -> select(+8-48h) -> start(+1-3d) -> complete(+7-30d)
      - Giveaway: interest(+0h) -> additional_interest(+2-24h each) -> select(+8-48h) -> complete(+0h)
      - Request: submit(+0h) -> offers(+2-48h each) -> fulfill(+1-7d)
      - Experience: create(+0h) -> RSVPs(+1-5d) -> start(+7-30d) -> complete(+2-4h)

3. **Merge and sort** all activities across all users/communities by timestamp

4. **Execute** sequentially, updating SimulationState after each step

#### Gear Catalog

Pre-defined gear items organized by category:

```go
var ToolsGear = []GearTemplate{...}     // 10-15 items (drill, saw, sander, etc.)
var KitchenGear = []GearTemplate{...}   // 10-15 items (stand mixer, food processor, etc.)
var OutdoorGear = []GearTemplate{...}   // 10-15 items (tent, kayak, bike rack, etc.)
var GardenGear  = []GearTemplate{...}   // 8-10 items (mower, chainsaw, etc.)
var PartyGear   = []GearTemplate{...}   // 8-10 items (projector, speakers, etc.)
```

Personas draw from catalogs based on type:
- Alfred: 15-25 items from Tools + Outdoor + Garden
- Derek: 5-10 items from Kitchen + Outdoor (selective)
- Gary: 1-3 items (whatever he has)
- Betty: 2-5 items from Party + Kitchen (for event hosting)

#### Experience & Request Templates

- Experience templates: "Community BBQ", "Board Game Night", "Trail Run", "Neighborhood Cleanup", "Movie Night", "Workshop: Basic Bike Repair", etc.
- Request templates: "Need help moving furniture", "Looking for a ladder", "Can someone help with yard work?", "Need a ride to the airport", etc.

### 5. Media Assets

**Phase 1 (bundled placeholders):** Include 30-50 royalty-free/CC0 images in `server/simulation/assets/`. The runner uploads them via `MediaService.AddMedia` at the start of each simulation run, caching the returned media IDs for use in gear/experience creation.

**Phase 2 (harvest from existing instances):** Write a small utility that downloads media from dev/prod via presigned URLs and saves to `server/simulation/assets/`.

### 6. Built-In Scenarios

**Scenario 1: "Suburban Neighborhood" (medium, 17 members)**
- Mix: 3 Alfreds, 4 Dereks, 3 Garys, 2 Bettys, 3 Emmas, 2 Henrys
- Heavy on gear sharing (tools, garden, outdoor)
- Some neighborhood events (BBQs, cleanups)
- Simulated period: 6 months

**Scenario 2: "College Friends" (small, 7 members)**
- Mix: 1 Alfred, 2 Dereks, 2 Garys, 1 Betty, 1 Emma
- Lots of borrowing, frequent events
- Kitchen/party/outdoor gear
- Simulated period: 3 months

**Scenario 3: "Active Community Org" (large, 37 members across 2 communities of 20+17)**
- Two communities: "Eastside Tool Library" (20) + "Westside Mutual Aid" (17), each under 32-member limit
- Mix: 5 Alfreds, 8 Dereks, 6 Garys, 5 Bettys, 8 Emmas, 5 Henrys
- Lots of experiences and requests
- Diverse gear
- Simulated period: 12 months

**Scenario 4: "New Community" (tiny, 4 members)**
- Just getting started, minimal activity
- Useful for testing empty/sparse states
- Simulated period: 2 weeks

### 7. Runner Interface

```go
// RunSimulation executes a scenario against a running server.
func RunSimulation(cfg RunConfig) error {
    // 1. Clean up any previous run with same simulation_id (via CleanupSimulation)
    // 2. Upload media assets
    // 3. Register users with simulation_id (first user creates community, others join via invite)
    // 4. Create gear for each user based on persona
    // 5. Generate and execute activity timeline
    // 6. Return summary stats
}

type RunConfig struct {
    ServerURL    string
    Scenario     Scenario
    Seed         int64  // PRNG seed for reproducibility
    SimulationID string // e.g., "sim-suburban-neighborhood"
}
```

Runnable as a test or against a remote server.

---

## Phased Implementation Plan

### Phase 1: Server Infrastructure
_Goal: Enable dev mode, timestamp overrides, and database reset. Run tests after each sub-step._

- [x] **1a. Create `server/clock/` package + tests**
  - `clock.go`: `Now(ctx)`, `UnixSec(ctx)` functions with context-based override
  - `middleware.go`: `SimulationTimestamp` HTTP middleware
  - `clock_test.go`: Unit tests — clock without context returns time.Now(), clock with context returns override, middleware sets context from header, middleware without header passes through
  - **Verify**: `go test ./server/clock/...`
  - Files: new `server/clock/clock.go`, `server/clock/middleware.go`, `server/clock/clock_test.go`

- [x] **1b. Consolidate `-dev-auth` -> `-dev-mode` and register simulation middleware**
  - Rename flag in `server/main.go`
  - Update all references: `server/integration_tests/test_helpers.go`, `server/e2e_test.go`, `server/cmd/patrol-test-server/main.go`, `Dockerfile`, `package.json`, terraform configs, READMEs
  - Conditionally register `clock.SimulationTimestamp` middleware in middleware chain when dev mode is on
  - Log warning when dev mode is enabled
  - **Verify**: `go test ./server/integration_tests/...` — existing tests still pass with renamed flag
  - Files: `server/main.go`, `server/integration_tests/test_helpers.go`, `server/e2e_test.go`, `server/cmd/patrol-test-server/main.go`, `Dockerfile`, `package.json`, `terraform/modules/containers/variables.tf`, `terraform/modules/containers/main.tf`, `terraform/environments/dev/main.tf`, `README.md`, `server/README.md`

- [x] **1c. Replace business `time.Now()` calls with `clock.UnixSec(ctx)`**
  - Update ~20 service files listed in the call sites table above
  - Keep non-business timing calls (logging, performance) unchanged
  - **Verify**: `go test ./server/services/...` — all existing tests pass (clock falls back to time.Now() without header)
  - Files: all files listed in the call sites table

- [x] **1d. Add `CleanupSimulation` admin RPC + tests**
  - Add `optional string simulation_id` to User (field 17) and Community (field 8) model protos
  - Add CleanupSimulation RPC, request, and response messages to `proto/ripls/api/admin_service.proto`
  - Run `npm run generate`
  - Implement cascade deletion in new `server/services/admin/cleanup.go`
  - Gate behind `devMode` flag on `AdminService`
  - Update `NewAdminService` to accept `devMode bool`
  - Update `server/main.go` to pass dev mode flag
  - Write `server/services/admin/cleanup_test.go`: test cleanup removes simulation data, test cleanup preserves non-simulation data, test cleanup fails when dev mode off
  - **Verify**: `go test ./server/services/admin/...`
  - Files: `proto/ripls/models/user.proto`, `proto/ripls/models/community.proto`, `proto/ripls/api/admin_service.proto`, `server/services/admin/admin_service.go`, new `server/services/admin/cleanup.go`, new `server/services/admin/cleanup_test.go`, `server/main.go`

### Phase 2: Simulation Framework
_Goal: Build the core runner that can register users, create communities, and execute basic RPCs_

- [x] **2a. Define core types + tests**
  - Scenario, CommunityDef, MemberDef, PersonaType, GearTemplate, RunConfig
  - Basic validation tests (scenario must have communities, communities must have members, etc.)
  - **Verify**: `go test ./server/simulation/...`
  - Files: new `server/simulation/types.go`, new `server/simulation/types_test.go`

- [x] **2b. Create simulation client wrapper + tests**
  - Authenticated HTTP client with per-request simulation timestamp injection
  - Helper methods: `SetTimestamp(t time.Time)`, `AsUser(email string)` to switch auth tokens
  - Wraps all RPC service clients (gear, transfer, request, experience, community, chat, login, media, admin)
  - Unit test: client sets X-Simulation-Timestamp header correctly
  - **Verify**: `go test ./server/simulation/...`
  - Files: new `server/simulation/client.go`, new `server/simulation/client_test.go`

- [x] **2c. Create simulation runner skeleton + integration test**
  - `RunSimulation(cfg)` function implementing: reset -> register users -> create community -> invite members
  - Integration test: start test server in dev mode, run minimal scenario (1 community, 3 users), verify users registered and community populated via ListCommunityUsers
  - **Verify**: `go test ./server/simulation/... -run TestRunner`
  - Files: new `server/simulation/runner.go`, new `server/simulation/runner_test.go`

- [x] **2d. Create SimulationState tracker + tests** (simplified: auth tokens + entity IDs only; all other state queried from server via RPCs)
  - Track user tokens/IDs, community IDs, gear availability, active transfers/requests/experiences
  - Methods: `MarkGearUnavailable()`, `MarkGearAvailable()`, `AddActiveTransfer()`, etc.
  - Unit tests for state transitions
  - **Verify**: `go test ./server/simulation/...`
  - Files: new `server/simulation/state.go`, new `server/simulation/state_test.go`

### Phase 3: Gear & Content Catalogs
_Goal: Populate communities with realistic gear, experiences, and requests_

- [x] **3a. Build gear catalog**
  - 56 gear items across 6 categories (tools, kitchen, outdoor, garden, party, electronics)
  - Each item: name, description, category, brand, model, material, weight, value
  - Files: `server/simulation/gear_catalog.go`

- [x] **3b. Build experience & request catalogs + location/venue data**
  - 100 experience templates with venue categories + 98 request templates (JSON data files)
  - 403 venue records from OpenStreetMap across 6 cities (Austin, Boulder, Minneapolis, Baton Rouge, Portland, Seattle)
  - City definitions with residential address generation
  - Loading functions via Go embed
  - Files: `server/simulation/catalogs.go`, `server/simulation/catalogs_test.go`, `server/simulation/locations.go`, `server/simulation/locations_test.go`, `server/simulation/data/experiences.json`, `server/simulation/data/requests.json`, `server/simulation/data/venues.json`

- [ ] **3c. Media asset collection & upload helper** (moved to Phase 7)

- [x] **3d. Wire gear creation into runner + test**
  - Persona-based gear assignment via `GearForPersona()` + executor creates gear with full metadata
  - SaveGear + ShareGear RPCs with simulation timestamps and venue-based locations
  - Integration test verifies gear creation and full activity execution
  - Files: `server/simulation/executor.go`, `server/simulation/runner.go`, `server/integration_tests/simulation_test.go`

### Phase 4: Persona Engine & Activity Generation
_Goal: Generate realistic, time-distributed community activity with multi-user interactions_

- [x] **4a. Define persona behavior weights + tests**
  - Activity probabilities per persona type (6 persona types)
  - Base activity rates, contention probability, gear category preferences
  - Deterministic activity selection with fixed iteration order
  - Files: `server/simulation/personas.go`, `server/simulation/personas_test.go`

- [x] **4b. Activity generation engine + tests**
  - Deterministic PRNG with configurable seed
  - Multi-step activity flows: contested loans, giveaways, requests with helpers, experiences with RSVPs
  - **Negative activity paths**: WithdrawInterest (10%), CancelTransfer (8%), RSVPNo (15%), CancelRequest (10%), WithdrawOffer (10%), CancelExperience (7%)
  - Poisson-distributed activity counts per persona per week
  - Tests: determinism, time bounds, action type coverage, multi-user patterns
  - Files: `server/simulation/activity.go`, `server/simulation/activity_test.go`

- [x] **4c. Activity execution in runner + integration test**
  - 21 action types executed via RPC calls with simulation timestamps
  - Auth token switching for multi-user interactions
  - State tracking for entity IDs, interested users, helpers
  - Graceful handling of state conflicts (warns and continues)
  - Files: `server/simulation/executor.go`, `server/simulation/runner.go`, `server/simulation/state.go`

### Phase 5: Scenario Definitions & End-to-End Testing
_Goal: Define complete scenarios and validate they work end-to-end_

- [x] **5a. Define built-in scenarios**
  - SuburbanNeighborhood (17 members, 6 months, Austin)
  - CollegeFriends (7 members, 3 months, Boulder)
  - ActiveCommunityOrg (37 members across 2 communities of 20+17, 12 months, Minneapolis)
  - NewCommunity (4 members, 2 weeks, Baton Rouge)
  - MaxCommunityMembers=32 enforced in validation
  - Files: `server/simulation/scenarios.go`, `server/simulation/scenarios_test.go`

- [x] **5b. End-to-end simulation tests**
  - Integration test runs 3-member scenario with full activity execution
  - Verifies user creation, community setup, gear creation, and activity counters
  - Scenario-level unit tests verify validation, unique emails, sizes, timeline generation
  - Files: `server/integration_tests/simulation_test.go`, `server/simulation/scenarios_test.go`

- [x] **5c. Summary reporting**
  - Structured logging with all counters: gear, transfers (started/completed/cancelled), requests (submitted/fulfilled/cancelled), experiences (created/completed/cancelled), RSVPs (yes/no), withdrawn interest/offers
  - Files: `server/simulation/runner.go`

### Phase 6: Polish & Dev Server Support
_Goal: Make simulation easy to run against dev environments_

- [x] **6a. CLI entry point for remote servers**
  - `cmd/simulate/main.go` with `-url`, `-scenario`, `-seed`, `-timeout`, `-v`, `-list`, `-assets-dir` flags
  - Files: `server/cmd/simulate/main.go`

- [x] **6b. Idempotent simulation IDs + purge all**
  - `SimulationID()` changed from `sim-YYYY-MM-DD-name` to `sim-name` (date-independent)
  - `ListSimulations` RPC: queries distinct simulation_ids from user+community tables, returns counts
  - `QueryDistinctField()` and `CountByField()` storage methods
  - `-purge` CLI flag: lists all simulations, calls CleanupSimulation for each, prints summary
  - Tests: `TestListSimulations_ReturnsAllSimulationIDs`, `TestListSimulations_Empty`, `TestListSimulations_FailsWithoutDevMode`, `testQueryDistinctField`, `testCountByField`
  - Files: `server/simulation/types.go`, `proto/ripls/api/admin_service.proto`, `server/storage/protosql.go`, `server/services/admin/cleanup.go`, `server/cmd/simulate/main.go`, `server/services/admin/cleanup_test.go`, `server/storage/protosql_test.go`

- [ ] **6c. Documentation**
  - Usage instructions and how to run locally vs. against dev

### Phase 7: Media Assets
_Goal: Associate realistic images with gear, experiences, and people_

- [x] **7a. Source stock imagery**
  - `cmd/generate_simulation_data images` fetches 1:1 stock photos from Pexels
  - Gear items queried by name+category, experiences by name, requests by title, profiles by "friendly person portrait"
  - Organized in `server/simulation/assets/{gear,experiences,requests,profiles}/`
  - Files: `server/cmd/generate_simulation_data/images.go`

- [x] **7b. Build media upload helper**
  - `UploadMediaDir()` uploads images via MediaService.AddMedia, caches in State.MediaIDs
  - `uploadAssetsForRun()` in runner.go uploads all subdirectories
  - `media_index.go`: `GearAssetFilename()`, `ExperienceAssetFilename()`, `RequestAssetFilename()`, `ProfileAssetFilename()`
  - Files: `server/simulation/media.go`, `server/simulation/media_index.go`

- [x] **7c. Wire media into executor**
  - `executeSaveGear()` attaches gear image media ID via `state.MediaIDs["gear/"+assetFile]`
  - `executeSaveExperience()` attaches experience image media ID via `state.MediaIDs["experiences/"+assetFile]`
  - Profile photos: deferred to future work (requires SetProfilePhoto RPC)
  - Files: `server/simulation/executor.go`

### Phase 8: Simulation Visualization
_Goal: Provide a way to summarize and visualize simulation results_

- [ ] **8a. Generate simulation report**
  - Produce a structured summary: timeline span, activity counts by type, per-user activity breakdown, per-community metrics
  - Output as JSON for downstream tooling
  - Files: new `server/simulation/report.go`

- [ ] **8b. Activity-over-time visualization**
  - Generate an HTML visualization showing community activity over time
  - Options: Gantt chart (per-ref timelines), stacked area chart (activity types per week), or swimlane diagram (per-user activity)
  - Self-contained HTML with embedded data (no external dependencies)
  - Files: new `server/simulation/visualize.go`, embedded HTML template

---

## Verification

After each phase, run the test command specified in the sub-step. Additionally:

1. **Phase 1**: `go test ./server/...` — ALL existing tests pass (clock changes are transparent). Start server with `-dev-mode`, curl an RPC with `X-Simulation-Timestamp: 1700000000` header, verify response timestamps use that time.

2. **Phase 2**: `go test ./server/simulation/...` — basic runner creates users and community.

3. **Phase 3**: Runner test populates gear. Verify via ListCommunityGear.

4. **Phase 4**: Full activity generation creates multi-user scenarios. Verify via ListTransfers, ListRequests, ListExperiences.

5. **Phase 5**: All 4 scenarios run successfully with expected data volumes and multi-user patterns.

6. **Phase 6**: Works against both local test server and remote dev server.

---

## Key Files Reference

| File | Purpose |
|------|---------|
| `server/main.go` | Server startup, flag registration, middleware chain |
| `server/clock/clock.go` | Simulation-aware time functions |
| `server/clock/middleware.go` | HTTP middleware for timestamp header |
| `server/middleware/request_id.go` | Existing middleware pattern to follow |
| `server/services/admin/admin_service.go` | Admin service (add dev mode + CleanupSimulation) |
| `server/services/transfer/state_machine.go` | Transfer timestamps |
| `server/services/transfer/interest.go` | Interest timestamps |
| `server/services/experience/lifecycle.go` | Experience timestamps |
| `server/services/request/lifecycle.go` | Request timestamps |
| `server/services/community/notifications.go` | Community event timestamps |
| `server/services/chat/messages.go` | Chat message timestamps |
| `server/services/login/registration.go` | User registration timestamps |
| `server/storage/protosql.go:352-406` | `DefaultStorageTypes()` — all table/entity types for cascade deletion |
| `server/integration_tests/test_helpers.go` | Test patterns to reuse (auth, client creation, server startup) |
| `docs/users/personas.md` | Persona definitions (source for PersonaType) |
| `docs/workflows/*.md` | Workflow specs (source for activity RPC sequences) |
