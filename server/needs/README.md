# server/needs

Shared library for the Workshop's "This week need" surface — the
one-line ask of the circle that renders between the Crew strip and the
Hero card stack on hot days.

## Why a shared library

Per `docs/server/architecture.md` Pattern 1 (Standalone Libraries for
Shared Logic), the detection logic lives here so the Workshop service
and future consumers (e.g., a scheduled job that pre-warms Workshop
content) can both call into it without one service injecting another.
Detection runs over the existing `Request` and `CommunityRequest` tables;
no new storage models.

## Files

- `doc.go` — package overview.
- `detector.go` — `DetectThisWeekNeed(ctx, store, communityIDs, now)`
  returns the highest-priority open Request to surface, or nil.
- `detector_test.go` — unit tests for the detection rule (active state,
  community membership, time-pressure window, under-claimed criterion).

## When to add code here

- New per-Request detection rules that the Workshop reads.
- Helpers around `RequestState`, `CommunityRequest`, or claim counts
  that more than one service needs.

When the helper is specific to one service (e.g., RPC handler logic),
keep it in that service's package instead.
