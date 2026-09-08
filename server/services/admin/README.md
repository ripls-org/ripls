# services/admin

The `admin` service implements the AdminService RPC interface, which exposes dev-mode-only operations for simulation management and database cleanup. All RPCs in this service are gated behind a `devMode` flag and return `PermissionDenied` in production.

## Key files

- `admin_service.go` — `AdminService` struct and constructor. Holds a storage reference and the dev-mode flag.
- `cleanup.go` — `ListSimulations`, `CleanupSimulation`: query and remove simulation-created entities from the database by simulation ID.

## When to add code here vs. elsewhere

An RPC belongs here when it is exclusively for development or testing support and must never be enabled in production. Simulation execution logic belongs in `server/simulation`. Production admin operations (if any are ever needed) should be a separate, properly authenticated service.
