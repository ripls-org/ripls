# API Design Review - 2026-04-28

## Executive Summary

The Ripls API continues its disciplined growth trajectory — every RPC has dedicated request/response types, documentation coverage remains effectively 100%, and proto comments stay scoped to wire semantics rather than server internals. Since the previous review the team has made one substantive remediation: `SaveUserRequest` and `SaveGearRequest` now use `optional` scalars for partial-update fields (the long-standing #1 issue), with comments that explicitly call out the "set to clear; omit to leave unchanged" contract. New surface this cycle is small and well-shaped: `StreamGenRequest` mirrors the existing streaming pattern, and `DraftImpactEstimate` / `DraftImpactEstimateWithOverrides` use a `oneof target` to avoid request-type sprawl.

The remaining picture is essentially unchanged from the prior review. List endpoints still ship without pagination, `RequestState` and `GearItemState` still skip enum value 3 with no `reserved` declaration, `UpdateTransferRequest` still silently skips field 2, Delete RPCs still use inconsistent id naming, and `PortfolioService` / `ImpactService` continue to bake view-model formatting into the wire format. None of those carried-forward items rise to "Critical" — they are durable hygiene problems that the previous review already filed as issues. There is no new severe finding this cycle.

## Changes Since Last Review

**Previous Review Date**: 2026-04-24

**Remediated Issues:**
- ✅ Issue #1 (SaveUserRequest/SaveGearRequest partial-update semantics) — fixed via commit `44629003e Fix partial-update semantics on SaveUser/SaveGear via optional scalars`. All updatable scalars on both messages now use `optional`, with comments that explicitly document "set to clear; omit to leave unchanged".
  - Locations: `proto/ripls/api/user_service.proto:46-75`, `proto/ripls/api/gear_service.proto:31-59`.

**Unaddressed Issues:**
- ⚠️ Issue #2 (Unpaginated list endpoints): `ListConversations`, `ListMyTransfers`, `ListReceivedTransfers`, `ListMyRequests`, `ListMyExperiences`, `ListUserGear`, `ListCommunityUsers`, `ListCompletedGiveaways`, `ListShadowUsers`. High, ~147 days open.
- ⚠️ Issue #3 (Pagination parameter inconsistency across the API). Medium, ~147 days open.
- ⚠️ Issue #4 (`RequestState` / `GearItemState` skip value 3 without `reserved`). Medium, ~147 days open.
- ⚠️ Issue #5 (`UpdateTransferRequest` missing `reserved 2`). Medium.
- ⚠️ Issue #6 (Delete RPC id naming inconsistency: `gear_id`/`request_id`/`experience_id` vs `id`). Medium.
- ⚠️ Issue #7 (`PortfolioService` / `ImpactService` pre-formatted display strings). High.
- ⚠️ Issue #8 (`SaveLocationRequest` lacks id for update). Medium.
- ⚠️ Issue #9 (`GenRequest` vs `GenRequestFromMedia` redundancy). Medium.
- ⚠️ Issue #10 (Parallel planning RPCs across `ExperienceService` / `RequestService`). Medium.
- ⚠️ Issue #11 (`CommunityImpactMetrics` `reserved` without explanation). Low.
- ⚠️ Issue #12 (Comment-only deprecations without `option deprecated = true;`). Medium.
- ⚠️ Issue #13 (`GearItem` vs `Gear` vs `CommunityGearItem` triplication). Low.
- ⚠️ Issue #14 (`Experience` field numbering fragmentation). Low.
- ⚠️ Issue #15 ("fulfullment" typo on `MarkRequestFulfilledResponse`). Low.
- ⚠️ Issue #16 (`CheckInvitationResponse` non-sequential field numbering). Low.

**New Issues:**
- 🆕 None this cycle. New surface (`StreamGenRequest`, `DraftImpactEstimate*`) follows existing patterns cleanly.

**Improvements:**
- `optional` scalar partial-update pattern correctly adopted on the two highest-traffic save messages, with crisp inline comments establishing the convention. These messages now serve as the reference style for any future `Save*Request` author.
- `DraftImpactEstimateRequest` uses `oneof target { experience_id | request_id }` rather than introducing two parallel RPCs, in tension with the broader Experience/Request planning duplication pattern Issue #10 highlights — a positive precedent.

**Trend**: Stable — one carried-forward High remediated; new surface is small and well-shaped; no carried-forward items regressed. Trend continues "improving on new surface, declining on hygiene backlog." The four filed High/Medium issues from the previous review remain the right things to drive down.

---

## API Inventory

| Service | RPCs | Documented | Issues |
|---|---|---|---|
| AdminService | 2 | 100% | 0 |
| ChatService | 16 | 100% | 1 (medium, unpaginated) |
| CommunityService | 26 | 100% | 1 (medium, unpaginated) |
| DeviceService | 2 | 100% | 0 |
| ExperienceService | 35 | 100% | 2 (planning duplication, unpaginated) |
| FeedbackService | 1 | 100% | 0 |
| FeedService | 6 | 100% | 1 (low, comment-only deprecation) |
| GearService | 10 | 100% | 2 (enum gap, unpaginated) |
| HealthService | 1 | 100% | 0 |
| ImpactService | 11 | 100% | 1 (high, formatted strings) |
| LeaderboardService | 1 | 100% | 0 |
| LocationService | 15 | 100% | 1 (medium, save-without-id) |
| LoginService | 11 | 100% | 1 (low, field ordering) |
| MediaService | 3 | 100% | 0 |
| PortfolioService | 12 | 100% | 2 (high formatted strings, low deprecation) |
| RequestService | 30 | 100% | 3 (enum gap, planning dup, unpaginated) |
| SearchService | 1 | 100% | 1 (low, comment-only deprecation) |
| TransferService | 20 | 100% | 2 (reserved-2 gap, unpaginated) |
| UserService | 6 | 100% | 0 |
| WaitlistService | 1 | 100% | 0 |
| **Total** | **209** | **~100%** | **16** |

(RPC count: 207 → 209: +1 `StreamGenRequest`, +2 `DraftImpactEstimate*`, no removals.)

## Critical Issues

None.

## High Priority

### 1. List endpoints continue to ship without pagination

**Location**: `proto/ripls/api/chat_service.proto:228-239`, `proto/ripls/api/transfer_service.proto:118-146`, `proto/ripls/api/request_service.proto:232-245`, `proto/ripls/api/experience_service.proto:241-...`, `proto/ripls/api/gear_service.proto:182-189`, `proto/ripls/api/community_service.proto:339-...`.

Carried forward unchanged. Violates CLAUDE.md "Always LIMIT unbounded queries." Memory-exhaustion risk at scale; no incremental UI loading. Already filed as `review:api_design:missing-pagination-list-rpcs`.

### 2. `PortfolioService` / `ImpactService` couple the API to a single-client view model

**Location**: `proto/ripls/api/portfolio.proto` (~829 lines, multiple `formatted_*` / `display_name` / `label` fields at lines 485, 494, 535, 571, 577, 720, 723, 756, 771); `proto/ripls/api/impact_service.proto` (`formatted_total`, `formatted_value`, `UserRankedCommunity.formatted_value` at line 626).

Carried forward unchanged. Pre-formatted display strings on the wire prevent client-side i18n, alternate clients (web/widget/email), and copy A/B without server deploys. Already filed as `review:api_design:portfolio-service-preformatted-strings`.

## Medium Priority

### 3. Enum value gaps without `reserved` declarations

**Location**: `proto/ripls/api/request.proto:13-19` (`RequestState` skips 3); `proto/ripls/api/gear_service.proto:18-23` (`GearItemState` skips 3).

Carried forward unchanged. Skipped value could be reused, causing silent wire divergence with older clients. Already filed as `review:api_design:enum-gaps-request-gear-state`.

### 4. `UpdateTransferRequest` missing `reserved 2`

**Location**: `proto/ripls/api/transfer_service.proto:101-113`.

Fields 1, 3, 4, 5 are declared; field 2 is silently absent with no `reserved 2;`. Same fingerprint as #3 above.

### 5. Parallel planning RPCs across ExperienceService and RequestService

**Location**: `proto/ripls/api/experience_service.proto:394-521`, `proto/ripls/api/request_service.proto:403-515`.

Carried forward unchanged. ~20 RPCs and ~20 message types maintained in lockstep. Already filed as `review:api_design:planning-rpcs-duplicated-experience-request`.

### 6. Delete RPC id naming inconsistency

**Location**: `DeleteGearRequest` uses `gear_id`, `DeleteExperienceRequest` uses `experience_id`, `DeleteRequestRequest` uses `request_id`; `DeleteCommunityRequest`, `DeleteLocationRequest`, `DeleteMediaRequest` all use bare `id`.

Carried forward unchanged.

### 7. Comment-only deprecations

**Location**: `proto/ripls/api/feed_service.proto:47`, `proto/ripls/api/search_service.proto:35`, `proto/ripls/api/portfolio.proto:13`.

The word "Deprecated" appears in comments, but no field or RPC carries `option deprecated = true;`. Tooling (linters, generated docs, IDE warnings) cannot surface deprecation to clients. There are zero `deprecated = true` annotations across all api protos.

### 8. `GenRequest` vs `GenRequestFromMedia` still split

**Location**: `proto/ripls/api/request_service.proto`. `GenExperienceRequest` already uses the `oneof prompt { text | media_id | website_url }` pattern; `GenRequest` should follow.

### 9. `SaveLocationRequest` cannot update an existing location

**Location**: `proto/ripls/api/location_service.proto`.

No `id` field on `SaveLocationRequest`, so all calls are inserts. Carried forward.

## Low Priority

### 10. `CommunityImpactMetrics` `reserved` declarations lack rationale comments

`proto/ripls/api/impact.proto:72` — `reserved 1, 2, 3, 4, 5, 19, 20;` with no comment explaining what was removed and why.

### 11. `GearItem` vs `Gear` vs `CommunityGearItem` message triplication

Carried forward.

### 12. `Experience` field numbering fragmentation

Carried forward; source-order reordering is non-breaking.

### 13. `MarkRequestFulfilledResponse` typo "fulfullment"

`proto/ripls/api/request_service.proto:181`.

### 14. `CheckInvitationResponse` non-sequential field numbering

`proto/ripls/api/login_service.proto:140-149` (1, 2, 3, 6, 7, 4, 5, 8).

## Positive Findings

1. Issue #1 from last review remediated cleanly. The fix went beyond the minimum — comments on `SaveUserRequest` and `SaveGearRequest` now explicitly document the "set to clear; omit to leave unchanged" contract, which is the hardest part of `optional`-scalar APIs to communicate. These two messages now serve as the canonical example for any future `Save*Request` author.
2. `DraftImpactEstimateRequest` uses `oneof target { experience_id | request_id }` rather than splitting into two parallel RPCs — a deliberate counterexample to the planning-RPC duplication problem (Issue #5).
3. `StreamGenRequest` mirrors the established `StreamGenGear` / `StreamGenExperience` shape: `oneof event` with explicit terminal `final` / `error`, non-terminal events documented as unordered, field numbers reflect introduction order.
4. Documentation coverage remains effectively 100%. Every RPC and most fields carry meaningful comments scoped to wire semantics — no vendor names, no transport plumbing, no server-side mechanics in proto comments.
5. Provenance and `TrackedX` types remain first-class data lineage on every metadata field.
6. `Heartbeat {}` in stream protos correctly uses an empty message; no field-number land-grab.
7. Stream RPCs continue to document non-ordering guarantees and terminal-event invariants (`StreamGenGearResponse` comment, lines 415-423).

## Recommendations

### Priority 1 (already filed; need owner + landing)
1. Drive the four open auto-filed High/Medium issues to landed state:
   - `review:api_design:missing-pagination-list-rpcs`
   - `review:api_design:portfolio-service-preformatted-strings`
   - `review:api_design:enum-gaps-request-gear-state`
   - `review:api_design:planning-rpcs-duplicated-experience-request`

### Priority 2 (hygiene; small but valuable)
2. Standardize Delete RPC id naming on bare `id` (or entity-prefixed) — pick one and apply.
3. Add `option deprecated = true;` to the three comment-only deprecations so tooling and downstream clients can surface them.
4. Add `id` to `SaveLocationRequest` so the message can support update, matching the pattern on `SaveGearRequest`.

### Priority 3 (cosmetic)
5. Consolidate `GenRequest` / `GenRequestFromMedia` using the `oneof prompt` pattern from `GenExperienceRequest`.
6. Document `CommunityImpactMetrics.reserved` rationale; reorder `Experience` fields in source order; fix "fulfullment" typo; group `CheckInvitationResponse` fields.

## Issues to File

_None this run._

The four highest-impact findings from the previous review are already filed as
auto-filed issues with stable fingerprints (`partial-update-optional-save-user-gear`
remediated; `missing-pagination-list-rpcs`, `portfolio-service-preformatted-strings`,
`enum-gaps-request-gear-state`, `planning-rpcs-duplicated-experience-request`
remain open). No new finding this cycle clears the bar for filing — the
incremental surface added since the last review is small and well-shaped, and
the carried-forward hygiene items are already represented in the backlog.
Filing them again would create duplicate-issue noise.
