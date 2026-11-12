# Issue #679: Correctly use 'optional' proto3 fields throughout codebase

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/679
**Filed:** 2026-02-03 | **Last activity:** 2026-03-05
**Labels:** none | **Assignees:** unassigned
**Priority:** P3 (triage comment)
**Context docs:** none

## Problem Statement

Proto3 reintroduced the `optional` keyword in protobuf v3.15, giving explicit
field presence tracking. Without `optional`, proto3 fields have "implicit
presence" — you can't distinguish between a field explicitly set to its default
value (0, "", false) and one that was never set. With `optional`, generated code
provides `Has*()` methods and fields serialize even when set to zero values.

The ripls codebase currently uses `optional` on only ~6 fields across 55 proto
files. Many fields have comments describing them as optional or conditionally
present, but the proto schema doesn't enforce this. This leads to brittle
workarounds: `!= 0` checks for timestamps, `.isEmpty` checks for strings, and
`!= nil` checks for messages — all of which are ambiguous and error-prone.

## Context from Comments

- **Triage (tescobar, 2026-03-05):** Rated P3 — code quality improvement with
  low immediate user-facing impact but important for correctness. Acknowledged
  as a large refactor.

No further discussion. The issue is straightforward in intent but broad in scope.

## Current State

**Fields already using `optional` (6 total):**
- `ripls/models/experience.proto`: `started_at_unix_sec`, `completed_at_unix_sec`, `completion_summary`, `source_url`
- `ripls/models/request.proto`: `fulfilled_at_unix_sec`
- `ripls/api/experience.proto`: `source_url`
- `ripls/api/gear.proto`: `source_url`

**Workaround patterns in code:**
- Go: `if transfer.ActualPickupUnixSec != 0` — can't distinguish "never set" from epoch
- Go: `if conversation.Deleted != nil` — nil checks on message fields
- Dart: `if (detected.sourceUrl.isNotEmpty)` — string emptiness instead of `hasSourceUrl()`
- Comments: "Only populated when...", "Optional — may be empty"

## Potential Solutions

### Option A: Big-bang refactor

- **Approach:** Audit all 55 proto files, add `optional` everywhere appropriate, update all Go and Dart code in one pass.
- **Pros:** Done in one shot. Consistent codebase immediately.
- **Cons:** Massive PR, high review burden, risk of regressions across many services. Hard to test comprehensively.
- **Effort:** Large (multiple days of work, touches almost everything).

### Option B: Phased by domain area

- **Approach:** Add `optional` incrementally by service domain (transfers first, then gear, then requests, etc.), updating Go+Dart code for each domain before moving on.
- **Pros:** Manageable PRs, testable in isolation, can prioritize high-value fields first. Each phase is shippable.
- **Cons:** Takes longer overall. Codebase is inconsistent during transition.
- **Effort:** Medium per phase, large total.

### Option C: Phased by field category (timestamps → messages → strings)

- **Approach:** Group changes by field type: first make all conditional timestamps optional (highest bug risk), then conditional message fields, then optional strings/metadata.
- **Pros:** Addresses highest-risk fields first (timestamp ambiguity). Each category has a consistent transformation pattern. Cross-cutting but focused changes.
- **Cons:** Each phase touches many files across domains. Still broad PRs.
- **Effort:** Medium per phase, large total.

## Recommendation

**Option C — phased by field category.** The timestamp ambiguity is the only
category that can cause actual bugs (0 vs absent). Message fields and strings
are less urgent since nil/empty checks mostly work. This approach front-loads
the highest value work and each phase follows a mechanical pattern.

## Open Questions

- **Should `DeletedMetadata deleted` fields become optional?** — Recommendation:
  Yes. These are the classic present-or-absent pattern. However, this touches
  20+ model files and their storage layer, so it should be its own phase.

- **Wire compatibility — do we care about existing stored protos?** —
  Recommendation: Adding `optional` to proto3 is wire-compatible (the encoding
  doesn't change for non-default values). For stored protos in the database,
  existing zero-value fields will still deserialize as "not present" (which is
  correct — they weren't set). No migration needed.

- **Should we update Dart code to use `has*()` instead of emptiness checks?** —
  Recommendation: Yes, but only for fields we make optional. Don't refactor Dart
  code for fields that remain implicit-presence.

- **How to handle the Go pointer type change?** — Recommendation: When a field
  becomes `optional` in proto3, the Go generated type changes from `int64` to
  `*int64` (etc.). This means every access site must be updated. Use the
  compiler — `go build` will catch every broken site. Generate code, fix
  compilation errors, then run tests.

- **protosql impact for optional scalar fields?** — Recommendation: The
  Proto-SQL storage layer flattens proto fields to SQL columns. Verify that
  `protosql` correctly handles `optional` scalars (Go pointer types like
  `*int64`) for both reads and writes. If the flattening logic assumes non-pointer
  types, it will need updates. Test this in Phase 1 with timestamps before
  scaling to other phases.

## Implementation Plan

### Phase 1: Conditional timestamps
**Goal:** Eliminate timestamp ambiguity — the only category that causes real bugs.
**Tasks:**
- [x] Add `optional` to transfer model timestamps: `estimated_pickup_unix_sec`, `actual_pickup_unix_sec`, `actual_return_unix_sec`, `loan_duration_days`
- [x] Add `optional` to transfer API timestamps: same four fields
- [x] Add `optional` to experience API timestamps: `started_at_unix_sec`, `completed_at_unix_sec`, `completion_summary`
- [x] Add `optional` to request API: `fulfilled_at_unix_sec`
- [x] Add `optional` to gear API: `ActiveLoan.due_date_unix_sec`
- [x] Add `optional` to time API: `TimeRange.start_unix_sec`, `TimeRange.end_unix_sec`
- [x] Run `npm run generate` and fix all Go compilation errors (pointer derefs, nil checks)
- [x] Verify `protosql` storage layer handles optional scalar types (`*int64`) correctly for read/write/query operations — works transparently
- [x] Update Dart code to use `has*()` methods where applicable
- [x] Remove redundant "optional" / "may be empty" comments on fields now marked `optional`
- [x] Update tests (lifecycle_test.go, build_transfer_test.go, daily_test.go, impact_metrics tests) to use proper presence checks
- [x] Run `npm run build` to verify lint + compilation
- [x] Run full test suites: `flutter test --reporter expanded` (2917 passed)
- [x] Run full test suites: `go test ./...` (all passed)
**Validates:** The mechanical pattern works. Wire compatibility confirmed. protosql handles optional scalars. Test coverage is sufficient.

### Phase 2: Conditional message fields
**Goal:** Make message-typed fields that are conditionally present explicit in the schema.
**Tasks:**
- [x] Add `optional` to `ripls/api/transfer.proto`: `recipient` (User), `user_transfer` (UserTransferStatus), `selected_recipient` (User)
- [x] Add `optional` to `ripls/api/gear.proto`: `active_loan` (ActiveLoan)
- [x] Add `optional` to `ripls/api/gear_service.proto`: `GetGearResponse.active_loan`
- [x] Add `optional` to `ripls/api/community_service.proto`: `CommunityGearItem.active_loan`
- [x] Add `optional` to `ripls/api/transfer_service.proto`: `GetUserTransferStatusResponse.active_transfer`
- [x] Generate, fix compilation, update tests
- [x] Remove redundant optionality comments on updated fields
- [x] Run `npm run build` to verify lint + compilation
- [x] Note: message fields in proto3 are already nullable in Go (`*MessageType`), so the Go impact was minimal — schema documentation and Dart `has*()` methods are the main benefit
**Validates:** Message field optionality works cleanly in both Go and Dart.

### Phase 3: Conditional string and metadata fields
**Goal:** Make optional strings and metadata fields explicit.
**Tasks:**
- [x] Add `optional` to conversation_id fields in transfer, request, experience APIs (conditionally set)
- [x] Add `optional` to user API: `description`, `preferred_timezone`
- [x] Add `optional` to location API: `name`, `neighborhood`, `county`, `administrative_area`
- [x] Add `optional` to request API: `resolution_summary`
- [x] Add `optional` to experience API: `completion_summary` (already optional in models)
- [x] Remove redundant optionality comments on updated fields
- [x] Generate, fix compilation, update Dart to use `has*()`, update tests
- [x] Run `npm run build` to verify lint + compilation
**Validates:** String optionality works. Dart migration pattern is clean.

### Phase 4: DeletedMetadata fields
**Goal:** Make soft-delete presence explicit in the schema.
**Tasks:**
- [x] Add `optional` to all `DeletedMetadata deleted` fields across model protos (user, chat, location, gear, experience, story, request, community, media)
- [x] Fix Go storage layer code that checks `deleted != nil` — no changes needed (message fields already nullable in Go)
- [x] Verify protosql serialization handles optional message fields correctly — works transparently
- [x] Remove redundant optionality comments on updated fields
- [x] Generate, fix compilation, update tests
- [x] Run `npm run build` to verify lint + compilation
**Validates:** Storage-layer optional messages work with protosql.

### Phase 5: Sweep and cleanup
**Goal:** Catch any remaining fields missed in earlier phases.
**Tasks:**
- [x] Audit all proto files for remaining fields that should be optional
- [x] Add `optional` to model proto fields: location (name, neighborhood, county, administrative_area, region_code, parent_region_id, mapbox_feature_id), media (filename, description, source_stock_image_id, thumbnail_storage_url), request (resolution_summary), chat (SystemChatMessage.actor_id)
- [x] Fix Go service and test code for *string pointer types
- [x] Verify no redundant optionality comments remain
- [x] Run `npm run build` to verify lint + compilation
- [x] Final full test run: `go test ./...` (all passed)

## Audit Notes

**Changes made during audit:**

1. **Added protosql open question:** The Proto-SQL storage layer flattens proto
   fields to SQL columns. Optional scalars change Go types from value to pointer
   (`int64` → `*int64`), which may break the flattening/reconstruction logic in
   `server/storage/protosql.go`. This must be verified in Phase 1 before scaling.

2. **Added protosql verification task to Phase 1:** Explicitly verify that the
   storage layer handles optional scalar types for read/write/query operations.

3. **Added `npm run build` to all phases:** Per CLAUDE.md, all Go changes must
   pass `npm run build` (golangci-lint + go build). Each phase now includes this
   verification step.

4. **No architectural violations found:** The plan correctly keeps API and model
   proto packages independent (changes are parallel, not cross-importing). The
   client-side Dart changes correctly target `has*()` methods through the
   repository/viewmodel layers, not direct service access. The phased approach
   aligns with the project's preference for focused, testable increments.

## Related Issues
- None identified — this is a standalone code quality improvement.
