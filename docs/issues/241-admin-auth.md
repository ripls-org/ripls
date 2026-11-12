# Issue #241: Add auth to admin_service

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/241
**Filed:** 2025-11-08 | **Last activity:** 2026-03-26
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The AdminService is registered without authentication middleware. All other services use `authMiddleware.Wrap()` but AdminService is registered bare:

```go
// Register AdminService without authentication (TODO: add auth middleware for admin-only access)
adminPath, adminHandler := apiconnect.NewAdminServiceHandler(adminService)
mux.Handle(adminPath, adminHandler)
```

After the community equity removal (#873), AdminService exposes two RPCs: `CleanupSimulation` and `ListSimulations`. Both have internal `devMode` guards that return `PermissionDenied` when `devMode=false`, so production is not at risk today. However, when `devMode=true` (development/staging), anyone with network access can call these destructive endpoints — there is no authentication or authorization check.

The broader concern is that AdminService should be behind auth middleware and restricted to admin-role users, following the same patterns as all other services.

## Context from Comments

- **Triage (tescobar, 2026-03-05):** P3 security hardening. Low risk while user base is small and admin access is controlled.
- **Maintainer (maintainer, 2026-03-26):** AdminService should only be available when `devMode` is on. At minimum, move registration into the `devMode` conditional block. Consider other best-practice methods of securing the endpoint.

## Potential Solutions

### Option A: Dev-mode-only registration + auth middleware

- **Approach:** Only register the AdminService handler when `devMode=true`, and wrap it with `authMiddleware.Wrap()`. Add an admin-role check inside each RPC handler (belt-and-suspenders with the existing `devMode` guard).
- **Pros:** AdminService endpoints don't exist at all in production (no attack surface). Dev/staging environments still require valid JWT with admin role. Simple, small change.
- **Cons:** No admin tooling path in production (acceptable — admin ops are dev-only by design).
- **Effort:** Small.

### Option B: Auth middleware only (always registered)

- **Approach:** Keep AdminService always registered but add `authMiddleware.Wrap()` and admin-role checks inside RPCs. Keep `devMode` guards as additional protection.
- **Pros:** Admin endpoints exist in all environments (future-proofs for production admin tooling).
- **Cons:** Unnecessarily exposes the endpoint surface in production where the RPCs always return `PermissionDenied` anyway. No current need for production admin endpoints.
- **Effort:** Small.

### Option C: Separate admin auth middleware with API key

- **Approach:** Create a dedicated admin middleware that validates a separate API key or service account token, distinct from user JWT auth.
- **Pros:** Separates admin auth from user auth. Good for CI/CD automation.
- **Cons:** Over-engineered for current needs. Adds a new auth mechanism to maintain. The simulation tooling that calls AdminService already authenticates as a user with a JWT.
- **Effort:** Medium.

## Recommendation

**Option A: Dev-mode-only registration + auth middleware.** This matches the maintainer's direction ("move into the conditional block that checks if dev_mode is active") and eliminates the attack surface in production entirely. Adding auth middleware on top ensures dev/staging environments require authentication. Adding an admin-role check inside each RPC provides defense-in-depth.

## Open Questions

- **Should we create a shared `RequireAdmin` helper for role checking?** — Recommendation: Yes, add a small helper in the `auth` package (`auth.RequireAdmin(ctx)`) that returns a `connect.Error` with `CodePermissionDenied` if the user isn't an admin. This is reusable if future endpoints need admin restrictions.
- **Should `devMode` guards be removed from individual RPCs now that the entire service is behind the conditional?** — Recommendation: Keep them as defense-in-depth. They're cheap and prevent accidental exposure if someone later moves the registration outside the conditional.

## Implementation Plan

### Phase 1: Secure admin service registration and add role checks

**Goal:** AdminService is only available in dev mode, requires authentication, and requires admin role.

**Tasks:**
- [ ] Add `auth.RequireAdmin(ctx)` helper in `server/auth/authorization.go` that extracts auth info from context and verifies `Role == ROLE_ADMIN`, returning `connect.CodePermissionDenied` on failure
- [ ] Add unit tests for `RequireAdmin` (valid admin, valid non-admin, unauthenticated)
- [ ] Move AdminService handler registration inside the existing `if cfg.devMode` block in `server/main.go`, wrapped with `authMiddleware.Wrap()`
- [ ] Add `auth.RequireAdmin(ctx)` call at the top of `CleanupSimulation` and `ListSimulations` RPCs in `server/services/admin/cleanup.go`
- [ ] Update admin service tests to pass authenticated contexts with admin role
- [ ] Run `npm run build` to verify lint and compilation
- [ ] Run `go test ./...` to verify all tests pass

**Validates:** Admin endpoints are unreachable in production, require valid JWT in dev mode, and require admin role.

## Audit Notes

1. **Aligned with server architecture authorization patterns.** The architecture doc (`docs/server/architecture.md` lines 331-337) specifies that shared auth checks belong in `server/auth` and lists `RequireRole(ctx, role)` as an intended pattern. `RequireAdmin` is a concrete implementation of this pattern. The existing codebase uses `auth.GetAuthInfo(ctx)` for authentication (confirmed in 4 services) — `RequireAdmin` should use the same function internally.

2. **No client impact.** AdminService is not called from the Flutter client — only from simulation tooling and manual scripts. No client-side changes are needed.

3. **Simulation tooling compatibility requires design attention.** The simulation runner calls `CleanupSimulation` *before* any users are registered (`runner.go:49`), so there's no JWT token available at that point. Three options: (a) reorder the simulation to register an admin user first, then clean up; (b) have the server mint a dev-mode bootstrap admin token at startup that the simulation can use; (c) skip JWT auth for AdminService and rely solely on dev-mode gating + conditional registration. Option (c) is simplest and still eliminates the production attack surface — admin endpoints simply don't exist when `devMode=false`. Option (a) changes simulation ordering which may have side effects. Recommend Option (c) for now: conditional registration only, no auth middleware, keeping the internal `devMode` guards as defense-in-depth.

## Related Issues

- #873 — Remove community equity (removed RegenerateEquity RPC from AdminService, reducing attack surface)
