# Simulation

The `simulation` package is the load-testing and data-seeding library. It drives a live Ripls server through a deterministic sequence of multi-user activity — registering users, creating communities, listing gear, sending messages, starting loans, and more — to generate realistic data for development, performance profiling, and demo environments.

## Architecture

```
simulation/
├── runner.go        # RunSimulation: top-level entry point
├── scenarios.go     # Scenario definitions (suburban-neighborhood, college-friends, …)
├── executor.go      # Executor: maps abstract actions to RPC calls
├── executor_reads.go# Read-only actions (GetFeed, ListGear, …)
├── concurrent.go    # Per-user goroutines, semaphore concurrency limiting, refCoordinator
├── client.go        # NewClient: creates authenticated service clients per user
├── client_pool.go   # Pool of pre-authenticated clients
├── state.go         # State: tracks auth tokens, IDs, and timelines for a run
├── reads.go         # ReadBehavior and Poisson-scheduled background read traffic
├── types.go         # Scenario, Timeline, Action, RunConfig types
├── personas.go      # Persona definitions (name sets, gear catalogs per persona)
├── catalogs.go      # Gear catalog data for seeding
├── locations.go     # Pre-defined locations used in scenarios
├── activity.go      # Activity timeline builder
├── media.go         # Media upload helpers
├── report.go        # Text summary report
├── report_html.go   # HTML report with charts
└── assets/          # Test images used during gear seeding
```

## When to add code here vs. elsewhere

A new scenario or action belongs here. The binary that invokes the simulation belongs in `server/cmd/simulate`. Background job logic that runs on the real server belongs in `server/jobs`.
