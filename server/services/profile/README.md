# `server/services/profile`

Viewer-facing user profile service. Implements `ProfileService` from
[`proto/ripls/api/profile_service.proto`](../../../proto/ripls/api/profile_service.proto).

## Purpose

Renders one user's profile (the target) through another user's lens
(the viewer). The screen is anchored on the `(viewer, target)`
relationship: the same target produces different output for different
viewers because the shared-community subset and the postcard set vary.

## Key files

| File | Purpose |
|------|---------|
| [`service.go`](service.go) | Service struct, constructor, interface assertion |
| [`profile.go`](profile.go) | `GetUserProfileForViewer` handler — identity, shared communities, aggregate numbers, postcards |
| [`postcards.go`](postcards.go) | Deterministic gear / experience / help postcard generators scoped to communities the viewer shares with the target |
| [`profile_test.go`](profile_test.go) | Unit tests covering target-scope, leakage invariants, auth, and self-call rejection |
| [`postcards_test.go`](postcards_test.go) | Unit tests for the three postcard generators, including the privacy boundary |

For end-to-end documentation of the surface and its privacy invariants
see [`docs/profile.md`](../../../docs/profile.md).

## When to add code here

- Yes: new viewer-facing surfaces on another user's profile
  (cross-user relationship reads).
- Yes: privacy filtering between a viewer and a target.
- No: own-profile metrics — those go through `server/services/impact_metrics`
  (`GetUserImpactMetrics`) and the client renders them in `PortfolioMetricsScreen`.
- No: user CRUD (name / bio / media) — those live in `server/services/user`.
- No: community membership reads — those live in `server/services/community`.

## Privacy invariant

Non-shared community identifiers — IDs, names, and any per-row entity
context that would identify them — must never appear in the response
or in log fields. Per-record entries from non-shared communities,
when surfaced inside the detail screens, are anonymized server-side
(stripped of community-identifying fields). Aggregate totals across
the target's full activity are fine to surface; the protected
information is the per-record visibility into communities the viewer
isn't a member of.
