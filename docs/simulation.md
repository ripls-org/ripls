---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Simulation harness — a CLI that drives a dev-mode server with persona-driven, deterministic timelines of loans/giveaways/requests/experiences for demo seeding, load testing, and end-to-end workflow validation.
  globs: [server/simulation/**, server/cmd/simulate/**]
  triggers: [simulation, simulate, load-test, persona, scenario, deterministic-timeline, dev-mode, simulation-clock]
  lens: [server, testing]
  skills: [issue, audit, triage]
  domain: infra
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Simulation System

The simulation system generates realistic community activity against a running
Ripls server. It creates users, communities, and gear, then executes a
deterministic timeline of multi-user interactions: loans, giveaways, requests,
experiences, RSVPs, and cancellations. This is useful for demo environments,
load testing, and validating end-to-end workflows.

## Architecture

### Simulation Clock

The server must run in **dev mode** (`-dev-mode`), which enables the simulation
clock middleware. Each RPC call from the simulator carries a
`X-Simulation-Timestamp` header that overrides `time.Now()` on the server side.
This lets the simulator compress months of activity into minutes of wall-clock
time, with all timestamps, TTLs, and scheduled jobs behaving as if real time
passed.

### Persona-Driven Activity

Each simulated user is assigned a **persona** that governs their behavior:

| Persona | Focus | Behavior |
|---------|-------|----------|
| Alfred | Stuff × Giving | Lots of gear, generous lender |
| Derek | Stuff × Exchanging | Selective, lends & borrows |
| Gary | Stuff × Seeking | Borrows a lot, little to share |
| Betty | Time × Giving | Hosts many experiences |
| Emma | Time × Exchanging | Occasional participant |
| Henry | Time × Seeking | Makes requests for help |

Personas control: how many gear items a user starts with, which activity types
they favor (borrowing, hosting, requesting), how many actions they take per
simulated week, and which gear categories they prefer.

### Deterministic Timeline

The simulator generates a complete timeline of `ActivityStep` structs before
execution begins. Each step has a simulated time, actor, community, action type,
and reference key linking multi-step flows (e.g., express interest → start
loan → complete loan). The timeline is sorted by time and executed sequentially.
A fixed PRNG seed ensures identical runs for the same scenario and seed.

### Chat Messages

After each successful timeline step, the executor probabilistically injects
chat messages into the relevant conversation. Messages are drawn from templates
in `server/simulation/data/chat_messages.json`, organized by transaction type
(loan, giveaway, request, experience) and **phase** (e.g., `interest`,
`owner_reply`, `logistics`).

Each phase maps to a specific sender role so conversations alternate between
participants naturally:

| Flow | Phases (in order) | Speakers |
|------|-------------------|----------|
| Loan (interest) | `interest` → `owner_reply` | borrower → owner |
| Loan (start) | `logistics` → `logistics_confirm` → `thanks` | borrower → owner → borrower |
| Giveaway (interest) | `interest` → `owner_reply` | recipient → owner |
| Giveaway (select) | `logistics` → `thanks` | recipient → recipient |
| Request (offer) | `clarification` → `requester_reply` → `offer` | helper → requester → helper |
| Request (fulfill) | `coordination` → `requester_thanks` | requester → requester |
| Experience (RSVP) | `rsvp_excitement` [→ `question` → `host_reply`] | attendee [→ attendee → host] |
| Experience (share) | `question` → `host_reply` | attendee → host |

Each phase produces exactly one message (randomly selected from ~12 templates).
Messages contain placeholder tokens (`{item_name}`, `{owner_name}`,
`{borrower_name}`, `{duration}`) that are expanded at send time.

To preserve chronological ordering, each message within a batch gets a simulated
timestamp offset by a randomized 5–20 minute gap. This prevents same-second
messages from being sorted randomly by UUID in the database.

### Scenarios

Scenarios are predefined community configurations with different sizes and
activity periods:

| Scenario | Communities | Members | Period | Description |
|----------|-------------|---------|--------|-------------|
| `suburban-neighborhood` | 1 | 17 | 6 months | Neighborhood tool & gear sharing |
| `college-friends` | 1 | 7 | 3 months | Tight-knit friend group |
| `active-community-org` | 2 | 37 | 12 months | Large org with tool library + mutual aid |
| `new-community` | 1 | 4 | 2 weeks | Brand new community getting started |
| `load-test-small` | 1 | 10 | 1 month | Quick profiling run (~2 min) |
| `load-test-medium` | 2 | 30 | 3 months | Medium load, cross-community queries (~10 min) |
| `load-test-large` | 3 | 75 | 6 months | Stress test for connection pool + search indexes |
| `read-heavy` | 1 | 20 | 1 month | Read-heavy profiling (use with `-read-multiplier=5`) |

## Running Locally

### Prerequisites

1. Start the server in dev mode:
   ```bash
   go run ./server/cmd/server -dev-mode
   ```

2. Ensure code generation has been run:
   ```bash
   npm run generate
   ```

### Basic Run

```bash
go run ./server/cmd/simulate \
  -url http://localhost:8080 \
  -assets-dir server/simulation/assets
```

This runs the default `suburban-neighborhood` scenario with seed 42 and uploads
all media assets (gear, experience, request, profile, and community images).

### With Options

```bash
# Specific scenario and seed
go run ./server/cmd/simulate -url http://localhost:8080 -assets-dir server/simulation/assets -scenario college-friends -seed 123

# Generate HTML report
go run ./server/cmd/simulate -url http://localhost:8080 -assets-dir server/simulation/assets -report /tmp/sim-report.html

# Debug logging
go run ./server/cmd/simulate -url http://localhost:8080 -assets-dir server/simulation/assets -v
```

### Against a Remote Server

The simulator works against any server running in dev mode. Point `-url` at the
remote address:

```bash
go run ./server/cmd/simulate -url https://dev.example.com -scenario college-friends
```

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-url` | (required) | Server base URL |
| `-scenario` | `suburban-neighborhood` | Scenario name to run |
| `-seed` | `42` | PRNG seed for reproducibility |
| `-timeout` | `0` | Maximum wall-clock run time (0 = no limit) |
| `-assets-dir` | (required) | Path to simulation image assets |
| `-report` | (none) | Path to write HTML visualization report |
| `-list` | `false` | List available scenarios and exit |
| `-purge` | `false` | Remove all simulation data and exit |
| `-v` | `false` | Enable debug logging |
| `-read-traffic` | `false` | Enable periodic read actions (feed checks, browsing, search) |
| `-read-multiplier` | `1.0` | Scale read frequency (2.0 = double reads) |
| `-concurrency` | `0` | Number of concurrent user goroutines (0 = sequential) |

## Listing Scenarios

```bash
go run ./server/cmd/simulate -list
```

## Cleanup / Purge

Each simulation run automatically cleans up data from any previous run of the
same scenario (keyed by simulation ID `sim-<scenario-name>`). To remove **all**
simulation data across all scenarios:

```bash
go run ./server/cmd/simulate -url http://localhost:8080 -purge
```

This calls `ListSimulations` to find all simulation IDs, then
`CleanupSimulation` for each one. Cleanup removes users, communities, gear,
transfers, requests, experiences, and all related records.

## HTML Report

The `-report` flag generates a self-contained HTML file with no external
dependencies. Open it in any browser to see:

- **Summary cards**: total users, communities, gear, transfers, requests,
  experiences (with completed/cancelled breakdowns)
- **Weekly activity chart**: stacked bar chart showing activity by category
  (Gear Setup, Loans, Giveaways, Requests, Experiences, Cancellations) over
  the simulated period
- **Per-user table**: sortable table showing each user's activity counts by
  type, colored by persona
- **Community breakdown**: per-community totals (shown when multiple
  communities exist)
- **Simulated accounts**: email and persona for each simulated user. They have
  no password — registration is an emailed one-time code (#2571). To sign in as
  one, request a code for its address and read it from `RequestEmailCode`'s
  dev-mode `dev_code` echo; no mail is sent, because the personas use a
  reserved test domain.

The report embeds all data as JSON in a `<script>` tag and renders with vanilla
JS and CSS—no build step or external CDN required.

## Code Organization

All simulation code lives in `server/simulation/`:

| File | Purpose |
|------|---------|
| `types.go` | Core types: Scenario, CommunityDef, MemberDef, PersonaType |
| `scenarios.go` | Built-in scenario definitions |
| `personas.go` | Persona weights and activity selection |
| `activity.go` | Timeline scaffolding: ActivityStep, Action, GenerateTimeline |
| `loan_flow.go` | Loan multi-step flow generation |
| `giveaway_flow.go` | Giveaway multi-step flow generation |
| `request_flow.go` | Request multi-step flow generation |
| `experience_flow.go` | Experience multi-step flow generation |
| `executor.go` | Timeline execution (RPC calls) |
| `executor_reads.go` | Read-action execution (feed checks, browsing, search) |
| `reads.go` | Read behavior model and Poisson read timeline |
| `concurrent.go` | Per-user goroutine concurrency for timeline execution |
| `runner.go` | Orchestration (setup → generate → execute) |
| `state.go` | Runtime state tracking (tokens, IDs, counters) |
| `client.go` | HTTP client with simulation clock headers |
| `client_pool.go` | Per-user client pool for concurrent execution |
| `report.go` | Report data structures and BuildReport() |
| `report_html.go` | HTML template and WriteReportHTML() |
| `gear_catalog.go` | Gear item catalog |
| `catalogs.go` | Experience and request template catalogs |
| `locations.go` | Location/venue data |
| `chat.go` | Chat message building and sending |
| `data/chat_messages.json` | Chat message templates by flow type and phase |
| `media.go` | Media asset upload helpers |
| `media_index.go` | Asset filename mapping and cross-scenario dedup helpers |

The CLI entry point is `server/cmd/simulate/main.go`.
