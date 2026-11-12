# Tech Debt Review - 2026-04-28

## Executive Summary

Since the last review on 2026-04-24, the team has paid down two long-standing items: the permissive CORS policy is now an explicit allowlist via the `--cors-allowed-origins` flag with a hard fail-closed in production mode (closing a finding 147 days open), and several "stats hardcoded to 0" issues are gone — `HelpOfferedCount`, `EventsHostedCount`, experience `LatitudeDeg`/`LongitudeDeg`, and `RsvpYesCount`/`RsvpMaybeCount` now read real values from storage. The stale `TODO(carbon_v1)` is also gone. Large-file growth in the presentation layer was reversed dramatically — every non-generated Dart file under `app/lib/` is now ≤ 992 lines, and there are zero `dart-line-count-allow` exemptions outstanding. TODO/FIXME/HACK count dropped from 48 to **37** in non-generated Go/Dart sources.

The most concerning remaining item is the hardcoded `dev-secret-key` JWT signing secret at `server/main.go:524` — now 215 days old by this file's blame. The leaderboard service in `server/services/leaderboard/leaderboard.go` still ships placeholder San Francisco coordinates, fabricated impact metrics, and an N+1 per-community member-count query; the file has not been touched since 2026-02-09. The experience workflow test still papers over a suspected production bug (spurious notifications on `MarkExperienceInProcess` / `CompleteExperience`) with a permissive `>= 6` assertion instead of asserting the documented `== 6`.

## Changes Since Last Review

> Previous Review Date: 2026-04-24

**Paid Down:**
- Permissive CORS allowing `*` origins replaced with explicit `--cors-allowed-origins` allowlist; production mode fails to start without it and rejects wildcard
  - Location: `server/main.go:466-481, 967`
  - Resolved in: `538a4d9e9` "fix(cors): restrict CORS to explicit origin allowlist in production", wired via Dockerfile/Terraform in `ed6bd242f`
- `HelpOfferedCount` and `EventsHostedCount` now read real values
  - Location: `server/services/user/stats.go:120-121`
- Experience `LatitudeDeg`/`LongitudeDeg` resolved from `location_id`; `RsvpYesCount`/`RsvpMaybeCount` computed from `ExperienceRSVP` with per-user dedup
  - Location: `server/services/experience/service.go:215-247`
- Stale `TODO(carbon_v1)` reference removed from `server/services/request/stats.go`
- N+1 in `GetCommunityLeaderboard` (a separate calculator path in `server/impact_metrics/calculator.go`) eliminated via `CalculateUserImpactFromPreloadedData`
  - Resolved in: `878ec07b3` "fix(impact_metrics): eliminate N+1 queries in GetCommunityLeaderboard"
- Large presentation-layer files split — `conversation_view_model.dart` 3,092 → 846, `gear_content_view.dart` 2,465 → 893, `experience_content_view.dart` 2,100 → 907, `experience_needs_sheets.dart` 2,032 → split. Largest remaining non-generated Dart file is 992 lines.
  - Resolved in: `84c6a859a` "Split inline_conversation_view + location_picker_modal under 1000 lines" and predecessors
- Deprecated `community_id` API fields cleaned out of `feed_service.proto` and `search_service.proto` (no `[deprecated = true]` markers remain in `proto/ripls/api/`)

**Still Outstanding:**
- Hard-coded `dev-secret-key` JWT signing secret (`server/main.go:524`) — CRITICAL, 215 days open by file blame
- Leaderboard placeholder data — 7 `TODO(phase1)` markers (`server/services/leaderboard/leaderboard.go`) — HIGH, file unmodified since 2026-02-09 (78 days)
- Leaderboard service N+1 `getMemberCount` (`server/services/leaderboard/leaderboard.go:147-152`) — MEDIUM
- Community cascade soft-delete (`server/services/community/lifecycle.go:188`) — MEDIUM
- Chat array-field query (`server/services/chat/conversations.go:390`) — MEDIUM
- Experience integration test papers over spurious-notification bug (`server/integration_tests/experience_test.go:1644-1655`) — MEDIUM
- Three `Category: "" // TODO: Add category detection to AI provider` sites (`server/services/request/gen.go:160,275`, `server/services/experience/gen_ai.go:345`) — LOW
- Distance formatter library evaluation (`app/lib/core/utils/distance_formatter.dart:4,21`) — LOW
- Cache service/manager merge (`app/lib/data/cache/cache_service.dart:20`) — LOW
- Legacy `InboxItem.fromConversation` factory still has 2 callers (`conversation_view_model.dart:516`, `inbox_view_model.dart:276`) — LOW
- Deprecated `listCommunityUsers()` method on `CommunityRepository` — LOW
- Deprecated `RequestManagementModal` widget retained with no callers — LOW
- Deprecated `conversation_id` fields in `proto/ripls/models/community.proto:94,119,151` — LOW (storage-side; supports forward reads of pre-migration rows)
- Deprecated `GetPortfolioInboxView` RPC retained alongside `GetPortfolioInboxWeek`/`GetPortfolioInboxFeed` (`proto/ripls/api/portfolio.proto:13`) — LOW
- 5 experience workflow integration test gaps (`server/integration_tests/experience_test.go:315, 494, 666, 1638, 1648`) plus 1 giveaway test gap (`giveaway_test.go:693`) — MEDIUM
- Item-metrics fulfillment/session history not wired (`app/lib/presentation/screens/item/item_metrics_screen.dart:314,320`) — LOW
- Estimator phase-8 LLM classification TODOs (3 in proto, 4 in `server/impact_metrics/`) — LOW (planned)
- Governance "delete region overrides" API endpoint (`app/lib/presentation/screens/governance/governance_screen.dart:576`) — LOW
- Event-modal community sharing (`app/lib/presentation/viewmodels/event_modal_view_model.dart:764`) — LOW
- Leaderboard distance calc on client (`app/lib/presentation/viewmodels/leaderboard_view_model.dart:92`) — LOW (blocked on server-side location fix)
- Community `genCommunityFromMedia` RPC TODO (`app/lib/data/repositories/community_repository.dart:55`) — LOW
- 3 i18n TODOs in `app/lib/core/errors/rpc_error_handler.dart` from prior review — APPEAR TO BE GONE (file scanned, no TODOs found at lines 146/203/245 — see "New" note below)

**New Tech Debt:**
- None substantive. The review found no significant new debt accumulated in the 4-day window since the last review. Activity has been overwhelmingly accessibility migrations, large-file splits, lint promotions (`revive.exported` → error), and CI hygiene.

**Trend**: **Improving.** This is a tight window since the prior review (4 days), but the right items moved: a 147-day-old CRITICAL was closed (CORS), three of the prior "HIGH visible-zeros bugs" were fixed (user-stats + experience-mapper), the large-file growth crisis was reversed below the 1,000-line gate, and `revive.exported` was promoted to error severity. The JWT secret and the leaderboard service remain outstanding and need a forcing function (CI assertion + production gate).

---

## Tech Debt Inventory

### TODOs/FIXMEs by Category

| Category | Count | High Priority | Key Files |
|----------|-------|---------------|-----------|
| Production Config / Security | 1 | 1 | main.go |
| Leaderboard Placeholders | 7 | 7 | leaderboard.go |
| Data Model / Feature Gaps (Server) | 6 | 1 | experience, gear, user, chat, community, location, request |
| Integration Test Gaps | 6 | 1 | experience_test.go, giveaway_test.go |
| Estimator Phase Gating | 7 | 0 | savings.go, estimator_config.proto, metric_detail_calculator.go |
| Missing Features (Client) | 7 | 0 | item_metrics, inbox, event modal, leaderboard, community_repository, governance |
| Library Consideration | 3 | 0 | distance_formatter, cache_service |
| Deprecated / Legacy | 2 | 0 | inbox_item, community_repository |

**Total: 37 TODOs across ~22 files** (down from 48 across 28 files).

### Full TODO List

#### Server TODOs (Go) — 25 Total

**Production Configuration (CRITICAL)**
1. `server/main.go:524` — Hard-coded `dev-secret-key` JWT signing secret

**Leaderboard Service — Placeholder (HIGH)**
2. `server/services/leaderboard/leaderboard.go:41` — Geographic bounding box not implemented
3. `server/services/leaderboard/leaderboard.go:63` — Geographic filter not applied
4. `server/services/leaderboard/leaderboard.go:74` — Pagination not implemented
5. `server/services/leaderboard/leaderboard.go:90` — `NextPageToken` always empty
6. `server/services/leaderboard/leaderboard.go:124` — Community lat/lng hardcoded to SF
7. `server/services/leaderboard/leaderboard.go:129` — Impact metrics fabricated
8. `server/services/leaderboard/leaderboard.go:163` — `calculateImpactMetrics` is mock
   (Plus: per-community N+1 `getMemberCount` at line 147-152, no TODO marker but identified in prior review.)

**Data Model / Feature Gaps (HIGH/MEDIUM/LOW)**
9. `server/services/experience/get.go:41` — Community membership check (LOW)
10. `server/services/experience/gen_ai.go:345` — Category detection missing in AI provider (LOW)
11. `server/services/request/gen.go:160` — Category detection missing (LOW)
12. `server/services/request/gen.go:275` — Category detection missing (LOW)
13. `server/services/gear/service.go:385` — Ownership/privacy controls (LOW)
14. `server/services/chat/conversations.go:390` — Array field matching query needed (MEDIUM)
15. `server/services/community/lifecycle.go:188` — Cascade soft-delete on community delete (MEDIUM)
16. `server/services/location/regions.go:326` — State name → code lookup table

**Estimator Phase Gating (LOW — planned)**
17. `server/impact_metrics/estimator/savings.go:90,91` — LLM-assisted estimation for requests/experiences
18. `server/impact_metrics/estimator/savings.go:180` — LLM-assisted estimation per request type
19. `server/impact_metrics/metric_detail_calculator.go:457` — Add completion timestamp to Request model
20. `proto/ripls/models/estimator_config.proto:74,78,215` — Phase-8 LLM classification

**Integration Test Gaps (MEDIUM/LOW)**
21. `server/integration_tests/giveaway_test.go:693` — Giveaway Example 3 postconditions
22. `server/integration_tests/experience_test.go:315, 494, 666, 1638, 1648` — 5 experience-workflow assertion gaps (incl. papered-over notification bug at 1648)

**Undo Registry Phase-Gating**
23. `server/undo/registry_test.go:60` — Logs (not fails) missing `ValidateUndoData` until Phase 4+ (intentional)

#### Client TODOs (Dart/Flutter) — 12 Total

24. `app/lib/core/utils/distance_formatter.dart:4, 21` — Library replacement consideration
25. `app/lib/data/cache/cache_service.dart:20` — Service/manager merge
26. `app/lib/data/repositories/community_repository.dart:55` — `genCommunityFromMedia` RPC needed
27. `app/lib/presentation/viewmodels/event_modal_view_model.dart:764` — Community sharing
28. `app/lib/presentation/viewmodels/leaderboard_view_model.dart:92` — Distance calculation
29. `app/lib/presentation/models/inbox_item.dart:117` — Legacy `fromConversation` factory removal
30. `app/lib/presentation/screens/item/item_metrics_screen.dart:314, 320` — Fulfillment/session history wiring
31. `app/lib/presentation/screens/governance/governance_screen.dart:576` — Delete region overrides API

---

## Critical Tech Debt

### 1. Hard-Coded JWT Signing Secret (CRITICAL — 215 days by file blame)

**Location**: `server/main.go:524`

**Description**: `Secret: []byte("dev-secret-key"), // TODO: Use environment variable in production`. Initial blame is 2025-09-26. Two reviews have flagged this as critical; the CORS sibling finding was just closed in PR `538a4d9e9` but the JWT secret was not.

**Impact**: Anyone with access to the public repo or compiled binary string table can forge HS256 JWTs for any user. There is no environment override and no production-mode check. Combined with the recently-closed CORS issue, the trust boundary around session tokens still rests on this one literal.

**Effort**: 30 minutes for the code change; pair with a deploy-side secret-injection update.

**Recommendation**: Read the secret from a `JWT_SECRET` env var; refuse to start in production mode if unset or equal to `dev-secret-key`. Add a CI check that greps `server/main.go` for the literal `"dev-secret-key"` and fails any build that retains it on `main`. Document the rotation plan in `docs/server/`. The CORS fix in `538a4d9e9` is a good template — same pattern (env/flag, fail-closed in production, validator that rejects insecure defaults).

---

## High Priority

### 1. Leaderboard Service Ships Fabricated Data (HIGH)

**Location**: `server/services/leaderboard/leaderboard.go` (7 TODOs at 41/63/74/90/124/129/163)

**Description**: `buildLeaderboardEntry` hardcodes every community to latitude 37.7749, longitude -122.4194; `calculateImpactMetrics` returns mock numbers proportional to community age rather than aggregating real transfers/loans; pagination is a no-op (`NextPageToken: ""`); geographic bounding-box filtering is skipped. The file has not been modified since its initial commit on 2026-02-09 — 78 days.

**Impact**: Clients see plausible but wrong leaderboard data. Map-based UIs would render every community on top of one another in San Francisco. The `impact_metrics` calculator has a real implementation (`CalculateUserImpactInCommunity` and the new `CalculateUserImpactFromPreloadedData` from `878ec07b3`) — the leaderboard service has not been wired to use it.

**Effort**: 2–3 days to wire to real data and add cursor pagination; 30 minutes if the decision is to feature-flag the RPC off in production until then.

**Recommendation**: Pick one of:
- (a) Wire `calculateImpactMetrics` to the existing `impact_metrics` aggregation path; resolve community location from a primary-region lookup or add `latitude_deg`/`longitude_deg` to the `Community` model; add cursor pagination.
- (b) Feature-flag `SearchCommunities` off (or behind admin-only) in production until (a) ships.

In the same pass, replace the per-community `getMemberCount` (`leaderboard.go:147-152`) with `QueryByFieldIn` over all community IDs.

### 2. Leaderboard Service N+1 `getMemberCount` (HIGH-leaning-MEDIUM)

**Location**: `server/services/leaderboard/leaderboard.go:147-152`

**Description**: `buildLeaderboardEntry` is called inside the per-community loop and invokes `getMemberCount`, which itself runs a `QueryByField` against `CommunityUser`. With N communities this is N+1 round trips against the same table. The recently-shipped `CalculateUserImpactFromPreloadedData` pattern (commit `878ec07b3`) is the correct shape: pre-fetch once, filter in memory. The leaderboard service didn't get that treatment.

**Impact**: At even modest community counts (50+), this dominates `SearchCommunities` latency.

**Effort**: 1–2 hours.

**Recommendation**: Pre-fetch all `CommunityUser` rows for the candidate community IDs once via `QueryByFieldIn("community_id", ids)`, then group in Go; or add a `GROUP BY community_id` count helper. Add an `AssertMaxQueries` guard around `SearchCommunities` to prevent regression.

---

## Medium Priority

### 1. Experience Integration Test Papers Over Spurious-Notification Bug

**Location**: `server/integration_tests/experience_test.go:1644-1655` (test `Postcondition_NoNotificationsForAttendanceRecording`)

**Description**: Per `docs/workflows/experience.md`, `MarkExperienceInProcess` and `CompleteExperience` should send no notifications. The test comment explicitly states the server is sending 14 (3 share + 3 RSVPs + 4 in-process + 4 complete) instead of the documented 6, and the assertion was relaxed to `>= 6` to keep the test green.

**Impact**: Documents and tolerates a real production bug — every experience completion currently fans out 8 unnecessary notifications. This affects user notification volume directly.

**Effort**: 4–6 hours: trace the source of the extra notifications in `experience` service handlers, suppress them, tighten the test back to `== 6`.

**Recommendation**: Treat the test comment at line 1641-1643 as a bug spec. Once fixed, fold the assertion back to `len(notifLogs) == 6` so future regressions surface immediately.

### 2. Community Soft-Delete Does Not Cascade

**Location**: `server/services/community/lifecycle.go:188`

**Description**: When a community is soft-deleted, related `CommunityUser`, `CommunityGear`, and `CommunityInvitation` rows are not cascaded. The existing `DeletedMetadata` proto pattern is in place on those rows; the cascade just isn't implemented.

**Impact**: Queries that don't opt into deleted rows will still surface memberships/sharings to deleted communities until manual cleanup.

**Effort**: 3–4 hours.

**Recommendation**: In `DeleteCommunity`, after marking the community deleted, batch-update `deleted` on the three join tables in a single transaction.

### 3. Chat Conversation Array-Field Lookup Comment Implies Workaround

**Location**: `server/services/chat/conversations.go:390`

**Description**: `// TODO: This requires a database query that can handle array field matching`. The comment implies that a server-side filter or pagination-by-fetch is being used in place of a SQL `=` over an array column.

**Impact**: Possible scan/N-row-fetch in chat lookups; magnitude depends on the surrounding code.

**Effort**: 2–3 hours.

**Recommendation**: Add a Postgres array-contains predicate to the relevant `protosql` helper and replace the in-Go filter with a SQL `WHERE` clause.

### 4. Five Experience Workflow Integration-Test Gaps

**Location**: `server/integration_tests/experience_test.go:315, 494, 666, 1638` (plus the papered-over `1648` covered above)

**Description**: Each TODO references a postcondition in `docs/workflows/experience.md` that is not yet asserted (cancellation notifications, participant removal from conversation, etc.). One gap (`giveaway_test.go:693`) has the same shape on the giveaway side.

**Effort**: 4–6 hours total.

**Recommendation**: Use `/ripls-update-system-tests` to realign the suite with the workflow docs.

---

## Low Priority

### 1. AI Category Detection Missing in 3 Sites

**Location**: `server/services/request/gen.go:160`, `server/services/request/gen.go:275`, `server/services/experience/gen_ai.go:345`

**Description**: All three sites set `Category: ""` with the same TODO. Carried unchanged from prior review.

**Effort**: 4 hours: add `category` to the AI generation schema, propagate through each site.

### 2. Legacy `InboxItem.fromConversation` Factory Still Has Live Callers

**Location**: `app/lib/presentation/models/inbox_item.dart:119` (factory), called from `app/lib/presentation/viewmodels/conversation_view_model.dart:516` and `app/lib/presentation/viewmodels/inbox_view_model.dart:276`

**Description**: TODO says "Remove this factory after GetConversationContext migration is complete." Two callers still exist; the comment has been carried across multiple review windows.

**Effort**: 2–3 hours: migrate the two callers to `fromConversationContext`, delete the factory.

### 3. Deprecated `listCommunityUsers` and `RequestManagementModal` Retained

**Location**: `app/lib/data/repositories/community_repository.dart:189-192`, `app/lib/presentation/screens/request/request_management_modal.dart`

**Description**: `listCommunityUsers` is now a single-line delegate to `getMembers` (still has one internal caller at `community_repository.dart:178`). `RequestManagementModal` has zero non-self call sites in `app/lib`.

**Effort**: 30 min each.

### 4. Distance Formatter Library, Cache Service/Manager Merge

**Location**: `app/lib/core/utils/distance_formatter.dart:4,21`; `app/lib/data/cache/cache_service.dart:20`

**Description**: Carried-forward exploratory TODOs. No urgency.

### 5. Item-Metrics Fulfillment/Session History

**Location**: `app/lib/presentation/screens/item/item_metrics_screen.dart:314, 320`

### 6. Governance Region-Override Delete API

**Location**: `app/lib/presentation/screens/governance/governance_screen.dart:576`

### 7. Event Modal Community Sharing

**Location**: `app/lib/presentation/viewmodels/event_modal_view_model.dart:764`

### 8. Estimator Phase-8 LLM Classification

**Location**: `proto/ripls/models/estimator_config.proto:74, 78, 215`; `server/impact_metrics/estimator/savings.go:90, 91, 180`; `server/impact_metrics/metric_detail_calculator.go:457`

**Description**: Intentional phase gating; tracked in the impact improvements plan.

### 9. State-Code Lookup Table

**Location**: `server/services/location/regions.go:326`

**Description**: TODO marker remains; code is functional with the lookup added in the prior review window. Comment is stale.

**Effort**: 5 minutes — delete the TODO if the lookup is complete.

### 10. Deprecated Storage-Side `conversation_id` Fields

**Location**: `proto/ripls/models/community.proto:94, 119, 151`

**Description**: All three carry `[deprecated = true]` and a "data migration reads only" comment. `server/storage/migrations.go` actively reads these for backward compatibility (`migrateGearConversationIDs`, `migrateRequestConversationIDs`, `migrateExperienceConversationIDs`), so they are intentionally retained until production data is verified migrated.

**Effort**: 2–3 hours after migration confirmation.

### 11. Deprecated `GetPortfolioInboxView` RPC

**Location**: `proto/ripls/api/portfolio.proto:13`

**Description**: Comment says "Deprecated: use GetPortfolioInboxWeek instead." The replacement RPCs (`GetPortfolioInboxWeek`, `GetPortfolioInboxFeed`) are present.

**Effort**: 1–2 hours: confirm no clients call it and remove.

---

## Architecture Violations

### Ongoing Good Patterns
- Proto api/models separation maintained (no cross-imports detected).
- Repository pattern followed on the client.
- The `CalculateUserImpactFromPreloadedData` refactor in `878ec07b3` is a good model of the pre-fetch-once pattern called out in the SQL-efficiency CLAUDE.md guidance.
- File-size discipline (every non-generated Dart file under `app/lib/` ≤ 992 lines, 0 exemption hatches outstanding).

### Concerns Carried Forward

**Leaderboard service N+1** — covered in High Priority above. Same anti-pattern that the impact_metrics calculator already moved past.

---

## Test Debt

### Skipped Tests (Appropriate)
- 68 `t.Skip(...)` call-sites in server tests, predominantly integration-gated tests requiring external credentials, Docker, or model files. Sampled call-sites all use legitimate guards (env-var checks, `testing.Short()`, missing model files).

### Integration Test TODOs
6 open in integration tests (5 experience + 1 giveaway). One (`experience_test.go:1648`) explicitly tolerates a known bug — see Medium Priority #1.

---

## Dependency Debt

### Server (Go)
No outdated dependencies identified.

### Client (Flutter)
No outdated dependencies identified.

---

## Quick Wins

| # | Item | Effort | Impact |
|---|------|--------|--------|
| 1 | JWT secret from env var with prod fail-closed + CI grep gate | 1 hour | CRITICAL security fix |
| 2 | Feature-flag leaderboard `SearchCommunities` off in prod until real data | 30 min | Stop shipping fabricated data |
| 3 | Fix leaderboard N+1 `getMemberCount` via `QueryByFieldIn` + `AssertMaxQueries` | 1–2 hours | Perf |
| 4 | Migrate 2 `InboxItem.fromConversation` callers, delete factory | 2 hours | Clean migration |
| 5 | Delete deprecated `listCommunityUsers` and `RequestManagementModal` | 30 min | API cleanup |
| 6 | Remove stale `state name → code` TODO in `regions.go` (lookup is implemented) | 5 min | Comment hygiene |

**Total effort: ~6 hours for 6 improvements**

---

## Recommendations

### Immediate Actions (Before Next Production Deployment)

1. **JWT secret hardening** (1 hour): env var, prod fail-closed, CI grep check on `dev-secret-key`. The CORS fix in `538a4d9e9` is a working template — apply the same shape.
2. **Stop shipping fabricated leaderboard data** (30 min via feature flag, 2–3 days to fix properly): pick one path; this has been open 78 days.

### Near-Term Actions (Next 2–4 Weeks)

3. Fix experience spurious-notification bug; tighten `experience_test.go:1644` assertion to `==`.
4. Cascade community soft-delete to `CommunityUser`/`CommunityGear`/`CommunityInvitation`.
5. Wire chat array-field SQL query to replace in-Go filter.
6. Close the 5 experience workflow + 1 giveaway integration-test gaps via `/ripls-update-system-tests`.

### Medium-Term Actions (Next 1–2 Months)

7. AI category detection (3 sites) — 4 hours.
8. Estimator Phase-8 LLM classification per plan.
9. Confirm storage migration done; drop deprecated `conversation_id` fields and `GetPortfolioInboxView` RPC.

---

## Overall Assessment

### Strengths
- Architecture remains sound; proto separation, repository pattern, and dependency hygiene all healthy.
- Strong execution this window: CORS fixed, three "visible zeros" stat bugs fixed, large-file growth fully reversed below the 1,000-line gate, `revive.exported` lint promoted to error severity.
- The `CalculateUserImpactFromPreloadedData` refactor sets a good template for further N+1 elimination.

### Areas for Improvement
- The JWT secret has now outlived multiple review cycles. It needs a CI gate, not another comment.
- The leaderboard service has been shipping placeholder data for 78 days untouched. Either fix it or feature-flag it off.
- An integration test still tolerates a documented production bug instead of asserting correctness.

### Trend
**Improving.** Real top-of-list items closed in the 4-day window since the last review. The remaining critical/high items are well-understood and individually small (1 hour, 30 minutes for the feature-flag fallback). Next review should expect to close at least the JWT and one of the two leaderboard items.

---

## Document Metadata

**Review Date**: 2026-04-28
**Previous Review Date**: 2026-04-24
**Reviewer**: Claude (AI Assistant)
**Scope**: Full codebase (server + client)
**Files Analyzed**:
- Server: 367 non-generated, non-test Go files
- Client: 800 non-generated Dart files
- Proto definitions: full `proto/ripls/{api,models}/`
- Documentation: `docs/` tree

**Next Review**: Recommended in 1–2 weeks or immediately after addressing the JWT secret.

---

## Issues to File

### [Critical] Hard-coded JWT signing secret in main.go

- **Fingerprint**: `review:tech_debt:jwt-secret-hardcoded`
- **Type**: Bug
- **Why it matters**: `server/main.go:524` uses the literal string `"dev-secret-key"` as the JWT signing secret for every build, allowing anyone with access to source or binary to forge sessions for any user. Open 215 days by file blame and across multiple reviews. Sibling CORS finding was closed this window via `538a4d9e9` — same pattern can be applied here.
- **Locations**: `server/main.go:524`
- **Recommendation**: Read from a `JWT_SECRET` env var; fail startup in production mode if unset or equal to `dev-secret-key`. Add a CI grep check that rejects the literal in any production build path. Mirror the production fail-closed style of the CORS fix.
- **Suggested labels**: `security`

### [High] Leaderboard ships fabricated data for every community

- **Fingerprint**: `review:tech_debt:leaderboard-placeholder-data`
- **Type**: Bug
- **Why it matters**: `server/services/leaderboard/leaderboard.go` hardcodes every community's coordinates to San Francisco (37.7749, -122.4194) and returns mock impact metrics from `calculateImpactMetrics`. Pagination and geo-bounds filters are no-ops. Users see plausible but wrong leaderboard data; map UIs stack every community on the same pin. File untouched since 2026-02-09 (78 days); the real impact aggregation now exists in `server/impact_metrics/calculator.go` after `878ec07b3`.
- **Locations**: `server/services/leaderboard/leaderboard.go:41`, `server/services/leaderboard/leaderboard.go:124`, `server/services/leaderboard/leaderboard.go:129`, `server/services/leaderboard/leaderboard.go:163`
- **Recommendation**: Wire `calculateImpactMetrics` to `impact_metrics` aggregation; resolve community lat/lng (add columns or use primary-region lookup); implement cursor pagination. If timeline doesn't allow, feature-flag the RPC off in production for now. Fix the per-community N+1 `getMemberCount` at `leaderboard.go:147-152` in the same pass.
- **Suggested labels**: `bug`

### [Medium] Experience integration test asserts known-bad notification count

- **Fingerprint**: `review:tech_debt:experience-test-spurious-notifications`
- **Type**: Bug
- **Why it matters**: `server/integration_tests/experience_test.go:1644-1655` documents that the server sends 14 notifications during the test (3 share + 3 RSVPs + 4 in-process + 4 complete) where `docs/workflows/experience.md` specifies 6. The assertion was relaxed to `>= 6` to keep the test green, papering over an 8-notification fan-out bug per experience completion. Carries forward unchanged from the prior review.
- **Locations**: `server/integration_tests/experience_test.go:1638`, `server/integration_tests/experience_test.go:1648`
- **Recommendation**: Trace why `MarkExperienceInProcess` and `CompleteExperience` each emit 4 extra notifications; suppress them at the source; tighten the assertion to `len(notifLogs) == 6` so regressions surface.
- **Suggested labels**: `bug`

### [Medium] Leaderboard service N+1 member-count query

- **Fingerprint**: `review:tech_debt:leaderboard-n-plus-1-member-count`
- **Type**: Bug
- **Why it matters**: `getMemberCount` at `server/services/leaderboard/leaderboard.go:147-152` runs one `QueryByField` per community in the leaderboard loop. With N communities this is N+1 round-trips, directly violating the project's SQL-efficiency guideline. Flagged in two prior reviews; the analogous N+1 in `server/impact_metrics/calculator.go` was fixed this window via `CalculateUserImpactFromPreloadedData` (`878ec07b3`), so the template exists.
- **Locations**: `server/services/leaderboard/leaderboard.go:113`, `server/services/leaderboard/leaderboard.go:147`
- **Recommendation**: Pre-fetch `CommunityUser` rows once with `QueryByFieldIn` over all candidate community IDs and group in Go (or add a `GROUP BY community_id` count helper). Wrap `SearchCommunities` in `AssertMaxQueries` to prevent regression.
- **Suggested labels**: `performance`
