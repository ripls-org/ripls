# Issue #873: Remove all traces of community equity

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/873
**Filed:** 2026-03-26 | **Last activity:** 2026-03-26
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/impact_metrics.md, docs/community_equity.md

## Problem Statement

The Community Equity system — a point-based scoring model that assigned equity values to community actions (sharing gear, completing transfers, creating experiences, etc.) — has been superseded by the Impact Metrics system. Dead code for Community Equity still exists across the entire stack: proto definitions, Go server (scorer, service, storage, jobs, admin RPCs, scripts), Flutter client (service, repository, viewmodels, screens, analytics events), database schema, and documentation. This dead code should be removed completely.

## Context from Comments

No comments on this issue. The issue is clear and self-contained — the replacement (Impact Metrics) is already production-ready per `docs/impact_metrics.md`.

## Recommendation

Full removal in one pass across the entire stack. This eliminates all dead code at once with no intermediate state where half the system references deleted protos.

## Open Questions

- **Should `COMMUNITY_EVENT_TYPE_EQUITY_DECAY` be removed from the `CommunityEventType` enum in `community.proto`?** — Recommendation: Yes, remove it. It exists solely for the equity decay job and has no meaning outside the equity system. Mark field number 5 as reserved to prevent accidental reuse.
- **Should the `equity_value` field in `CommunityCreatedFeedEntry` (feed_service.proto:266) be removed?** — Recommendation: Yes, remove it. It only carries equity points and has no meaning after equity is gone. Mark field number 8 as reserved.
- **What about existing equity data in the database?** — Recommendation: Don't add a migration to drop tables now. The schema is auto-created from protosql_schema.go, so removing the entries there prevents table creation in new environments. Orphaned tables can be dropped manually later.

## Implementation Plan

### Phase 1: Remove all community equity code, protos, and documentation

**Goal:** Complete removal of all equity traces across proto definitions, server, client, and docs.

**Proto definitions:**
- [ ] Delete `proto/ripls/models/equity.proto`
- [ ] Delete `proto/ripls/api/equity_service.proto`
- [ ] Remove equity RPCs and types from `proto/ripls/api/admin_service.proto` (RegenerateEquity RPC, RegenerateEquityMode, RegenerateEquityRequest/Response, RegenerationSummary, RegenerationDiff)
- [ ] Reserve and remove `COMMUNITY_EVENT_TYPE_EQUITY_DECAY = 5` in `proto/ripls/models/community.proto`
- [ ] Remove `equity_value` field from `CommunityCreatedFeedEntry` in `proto/ripls/api/feed_service.proto` and reserve field number 8

**Server — delete equity-specific packages:**
- [ ] Delete `server/equity/` directory (scorer.go, constants.go)
- [ ] Delete `server/services/equity/` directory (service.go, service_test.go)
- [ ] Delete `server/storage/equity.go`
- [ ] Delete `server/jobs/equity_decay.go` and `server/jobs/equity_decay_test.go`
- [ ] Delete `server/services/admin/regeneration.go` and `server/services/admin/regeneration_test.go`
- [ ] Delete `server/scripts/regenerate_equity.go`, `server/scripts/regenerate_equity.sh`, `server/scripts/regenerate_equity/` directory

**Server — remove equity from shared code:**
- [ ] Remove equity cleanup from `server/services/admin/cleanup.go`
- [ ] Remove equity schema entries and indexes from `server/storage/protosql_schema.go` (6 table registrations + 6 indexes)
- [ ] Remove equityScorer from `server/main.go`: imports, instantiation, wiring into services, EquityService handler registration
- [ ] Remove equityScorer parameter from `server/services/community/service.go`, `notifications.go`, and all callers (lifecycle.go, membership.go, invitations.go, gear_sharing.go)
- [ ] Remove equityScorer parameter from `server/services/transfer/service.go` and all callers
- [ ] Remove equityScorer parameter from `server/services/request/service.go` and all callers
- [ ] Remove equityScorer parameter from `server/services/experience/service.go` and all callers
- [ ] Remove equityScorer from `server/services/feed/service.go` (import, field, constructor, equity_value population)
- [ ] Update all test files that instantiate equityScorer in community, transfer, request, experience, and feed service tests
- [ ] Update `server/scripts/README.md`

**Client — delete equity-specific files:**
- [ ] Delete `app/lib/services/equity_service.dart`
- [ ] Delete `app/lib/data/repositories/equity_repository.dart`
- [ ] Delete `app/lib/core/models/equity_history_response.dart`
- [ ] Delete `app/test/data/repositories/equity_repository_test.dart`
- [ ] Delete `app/test/core/models/equity_history_response_test.dart`

**Client — remove equity from shared code:**
- [ ] Remove equity providers from `app/lib/services/providers.dart`
- [ ] Remove equity state from `app/lib/presentation/viewmodels/user_profile_state.dart`
- [ ] Remove equity loading from `app/lib/presentation/viewmodels/user_profile_view_model.dart`
- [ ] Remove equity from `app/lib/presentation/viewmodels/community_content_view_model.dart` and `community_content_state.dart`
- [ ] Remove equity modals from screens: `gear_content_view.dart`, `request_content_view.dart`, `experience_content_view.dart`
- [ ] Remove equity from `community_content_view.dart` and `feed_screen.dart`
- [ ] Remove EquityViewedEvent from `events.dart` and `home_screen.dart`
- [ ] Update `app/test/core/observability/events_test.dart`

**Workflow docs — remove equity sections and postconditions:**
- [ ] Remove "Community Equity" section from `docs/workflows/loan.md` (line 482+) and equity postconditions from all loan examples (lines 293–296, 325, 349, 372–373)
- [ ] Remove "Community Equity" section from `docs/workflows/giveaway.md` (line 409+) and equity cross-reference (line 302)
- [ ] Remove "Community Equity" section from `docs/workflows/request.md` (line 575+)
- [ ] Remove "Community Equity" section from `docs/workflows/experience.md` (line 664+)

**Integration tests — remove equity assertions and helpers:**
- [ ] Remove all equity postcondition subtests from `server/integration_tests/loan_test.go` (Postcondition_OwnerEquity, Postcondition_BorrowerAEquity, Postcondition_BorrowerBEquity, Postcondition_GearEquity, Postcondition_BorrowerZeroNetEquity, Postcondition_BorrowerPositiveEquity, Postcondition_OwnerNetEquity) and equity client setup lines
- [ ] Remove `createAuthEquityClient` helper from `server/integration_tests/test_helpers.go`

**Testing docs — remove equity examples:**
- [ ] Remove equity code examples from `docs/testing/update_system_tests.md` (Postcondition_OwnerEquity example at line 108, equity TODO example at lines 174–179)

**Documentation:**
- [ ] Delete `docs/community_equity.md`
- [ ] Remove community_equity.md reference from `docs/impact_metrics.md`
- [ ] Delete `docs/crashlytics/01282026/NotifierProvider.CommunityEquityNotifier.CommunityEquityState.md` and `.txt`
- [ ] Update `docs/client/observability.md`, `docs/ai/feed2.md`, `docs/ai/feed-freshness.md`
- [ ] Update `docs/server/architecture.md`: remove equity from shared-libraries diagram (line 38), remove "Current State" note about equity.Service coupling (line 83), replace equity code examples in Patterns 1–2 (lines 405–458) with a current library example
- [ ] Update `docs/client/architecture.md`: remove stale reference to `community_equity_view_model.dart` (line 478)

**Verification:**
- [ ] Regenerate code with `npm run generate`
- [ ] Run `npm run build` to verify server lint and compilation
- [ ] Run `go test ./...` to verify server tests pass
- [ ] Run `flutter test --reporter expanded` to verify client tests pass

**Validates:** Zero references to community equity remain anywhere in the codebase.

## Audit Notes

The following items were added or modified during architectural review:

1. **Architecture docs reference equity** — `docs/server/architecture.md` uses equity as a code example in Patterns 1 and 2 (lines 38, 83, 87, 114, 405–456) and `docs/client/architecture.md` references a non-existent `community_equity_view_model.dart` (line 478). Both files need updating: remove equity from the server architecture shared-libraries diagram, replace the equity code examples with a current library (e.g., impact metrics), and remove the stale client architecture reference. Added to Documentation tasks.

2. **`RecordCommunityEventAndNotify` signature simplification** — After removing the `equityScorer` parameter, also remove the equity import from `notifications.go` and the equity-processing block (lines 47–54). The function signature becomes simpler since equity was the only optional scorer parameter.

3. **No i18n cleanup needed** — Verified no equity-related keys exist in ARB files (`app/lib/l10n/`).

4. **Proto field reservation is critical** — Removing `COMMUNITY_EVENT_TYPE_EQUITY_DECAY = 5` and `equity_value = 8` without reserving their field numbers could cause silent data corruption if those numbers are reused. The plan correctly calls for reservations. For `admin_service.proto`, also reserve the removed RPC field numbers and message types to prevent wire-format conflicts with any clients still running old code.

6. **Workflow docs and integration tests contain equity accounting** — All four workflow docs (`loan.md`, `giveaway.md`, `request.md`, `experience.md`) have "Community Equity" sections with point tables, and loan workflow examples include equity postconditions (e.g., "owner has +4 equity"). `server/integration_tests/loan_test.go` has 7 equity postcondition subtests and equity client setup. `docs/testing/update_system_tests.md` uses equity postconditions as code examples. All of these must be scrubbed. Added to implementation tasks.

5. **Orphaned community events in the database** — After removal, existing `COMMUNITY_EVENT_TYPE_EQUITY_DECAY` events (value 5) will remain in the `community_event` table but won't match any enum value. This is harmless (proto3 preserves unknown enum values as integers) but feed/event listing code should already handle unknown event types gracefully. Verify during testing that unknown event type 5 doesn't cause panics or errors in feed rendering.

## Related Issues

- No directly related issues. The issue references `docs/impact_metrics.md` as the successor system, which is already implemented.
