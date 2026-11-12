# Issue #2102: dead code: remove community leaderboard code (from server and client)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2102
**Filed:** 2026-05-21 | **Last activity:** 2026-05-26
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The repository contains a substantial cross-community "leaderboard" feature —
a map-based view that ranks communities by impact metrics (money saved,
CO₂ saved, time banked). The implementation spans proto, server, and client,
but it is wired to nothing reachable: neither `LeaderboardScreen` (the
map-based screen at `app/lib/presentation/screens/leaderboard/leaderboard_screen.dart`)
nor `CommunityLeaderboardScreen` (the per-community top-contributors screen at
`app/lib/presentation/screens/impact_metrics/community_leaderboard_screen.dart`)
is pushed by any router, navigator, or other screen. Per Thomas (issue
reporter, project owner), this code is dead and should be removed.

The work is to delete the dead leaderboard surface end-to-end while
preserving adjacent code that *is* alive — notably the per-community
top-contributors *data* (`GetCommunityLeaderboard` RPC + `LeaderboardMember`
type) which is still used by `community_metrics_breakdown.dart`, and the
`LeaderboardEntry`/`CommunityComparison` types in `impact_service.proto`
used by `community_compare_v2.dart`. Those stay.

## Context from Comments

The issue body is one line — "Per Thomas it's dead" — and two `@claude`
comments triggering this run. No design discussion, alternatives, or
constraints have been recorded. The reporter is a project collaborator
and frequent maintainer; this is a straightforward dead-code cleanup
request, not a design question.

The only nuance worth flagging: the word "leaderboard" appears in two
distinct subsystems (cross-community map + per-community top contributors),
and only the first is dead. The plan must not accidentally tear out the
second.

## Potential Solutions

### Option A: Remove only the cross-community `LeaderboardService` and its client (recommended)
- **Approach:** Delete the cross-community leaderboard end-to-end:
  `proto/ripls/api/leaderboard.proto`, `server/services/leaderboard/`,
  `server/main.go` wiring, the `app/lib/services/leaderboard_service.dart`
  client, `app/lib/data/repositories/leaderboard_repository.dart`,
  `app/lib/presentation/viewmodels/leaderboard_view_model.dart`,
  `app/lib/presentation/screens/leaderboard/` (whole dir),
  `app/lib/presentation/widgets/leaderboard/` (whole dir),
  `app/lib/presentation/models/leaderboard_item.dart`, dead l10n strings,
  related tests. Also delete the orphaned
  `app/lib/presentation/screens/impact_metrics/community_leaderboard_screen.dart`
  (no router pushes it). Remove the two `leaderboardRepositoryProvider`
  cache-invalidation calls from `delete_community_view_model` /
  `restore_community_view_model` and their test overrides. Preserve
  `GetCommunityLeaderboard` RPC, `LeaderboardMember`, `LeaderboardEntry`
  (CommunityComparison), and `community_metrics_breakdown.dart`.
- **Pros:** Removes ~30+ files of dead code and an unused RPC service.
  Single coherent deletion. Reduces server query surface and client
  bundle size. Removes a misleadingly-named codepath (the file
  `leaderboard.proto` is *not* the same as the `LeaderboardMember`
  type in `impact_service.proto` — confusing today).
- **Cons:** Touches proto (requires regen), server `main.go` wiring,
  several client viewmodels and providers, plus ARB l10n files.
  Must be careful not to remove the still-used `LeaderboardEntry` /
  `LeaderboardMember` types in the *other* proto file.
- **Effort:** Medium (lots of files, but mechanical once scope is locked).

### Option B: Remove only the unreachable screens, leave proto/server intact
- **Approach:** Delete `LeaderboardScreen`, `CommunityLeaderboardScreen`,
  `leaderboard_view_model.dart`, and their tests; keep proto/server/client
  service code in case it's resurrected.
- **Pros:** Smaller diff. No proto regen.
- **Cons:** Leaves a dead RPC, a dead repository, dead l10n, and dead
  providers on the wire. Doesn't satisfy "remove from server and client"
  in the issue title. Future engineers grep for "leaderboard," find the
  service, and assume it's live.
- **Effort:** Small.

### Option C: Remove client only, leave server `LeaderboardService` running
- **Approach:** Delete the client side but leave the server endpoint up.
- **Pros:** Smallest server diff.
- **Cons:** Same as B — leaves dead server code. The issue explicitly
  says "from server and client."
- **Effort:** Small-medium.

## Recommendation

**Option A.** The issue title is explicit ("from server and client"),
the project owner has authorized the deletion, and the dead-code surface
is large enough that a partial cleanup leaves more confusion than it
resolves. Option B/C would just defer the work. The proto-regen step is
routine and gated by `npm run generate` + CI.

## Open Questions

- **Should we keep the `LeaderboardService` proto definition for possible
  future use, or delete it outright?** — Recommendation: **delete**.
  The current implementation is a placeholder with `TODO:` stubs and
  hard-coded coordinates (San Francisco) and metrics ($1000 / 50kg / 10h)
  for every community. If a real cross-community discovery feature is
  built later, it will need a different schema anyway (real geo bounds,
  real metrics, pagination). Keeping the dead proto wastes maintenance.
- **Are the dead l10n keys `leaderboardTitle` and `communityLeaderboard`
  used anywhere outside the generated files?** — Recommendation: **yes,
  delete both** from `app/lib/l10n/app_en.arb` (and any other language
  ARB files). Confirmed via grep — only the generated
  `app_localizations*.dart` files reference them, and those regenerate
  from the ARBs.
- **The dead view models `delete_community_view_model` /
  `restore_community_view_model` call `leaderboardRepositoryProvider.
  invalidateLeaderboard()` — just drop the calls?** — Recommendation:
  **yes**. The leaderboard cache won't exist anymore; the calls are
  no-ops in waiting.
- **Should the `claude-plan-pr` PR also regenerate the proto-derived
  Dart/Go files, or leave that to the implementation agent?** —
  Recommendation: **implementation agent does the regen** as part of
  the normal `npm run generate` step. Generated code isn't checked in
  (per CLAUDE.md), so no diff lands.

## Implementation Plan

### Phase 1: Proto + server deletion
**Goal:** Remove the dead `LeaderboardService` from the schema and Go server.
**Tasks:**
- [ ] Delete `proto/ripls/api/leaderboard.proto`.
- [ ] Delete `server/services/leaderboard/` (whole package — `doc.go`,
      `leaderboard.go`, `leaderboard_test.go`).
- [ ] In `server/main.go`: remove the `"type2fun.org/ripls/server/services/leaderboard"`
      import, the `leaderboardService := leaderboard.New(sqlStorage)` line,
      and the `NewLeaderboardServiceHandler` registration + mux.Handle
      lines.
- [ ] Run `npm run generate` to refresh generated Go/Dart code.
- [ ] Run `npm run build` and `go test -tags=integration -timeout 20m ./...`
      and resolve any compile errors (only expected breakage is in
      client code, handled in Phase 2).
**Validates:** Server compiles cleanly without the dead RPC.

### Phase 2: Client deletion
**Goal:** Remove the unreachable Dart code that depended on the deleted
proto/service plus the orphan `CommunityLeaderboardScreen`.
**Tasks:**
- [ ] Delete files:
  - `app/lib/services/leaderboard_service.dart`
  - `app/lib/data/repositories/leaderboard_repository.dart`
  - `app/lib/presentation/viewmodels/leaderboard_view_model.dart`
  - `app/lib/presentation/screens/leaderboard/` (entire directory:
    `README.md`, `leaderboard_screen.dart`, `leaderboard_list_item.dart`)
  - `app/lib/presentation/widgets/leaderboard/` (entire directory — all
    files: `README.md`, `community_detail_modal.dart`,
    `leaderboard_header.dart`, `leaderboard_map_helper.dart`,
    `my_communities_summary_bar.dart`)
  - `app/lib/presentation/screens/impact_metrics/community_leaderboard_screen.dart`
  - `app/lib/presentation/models/leaderboard_item.dart`
- [ ] In `app/lib/services/providers/impact_providers.dart`: remove the
      `import` of leaderboard_service.dart + leaderboard_repository.dart,
      delete `leaderboardServiceProvider` and `leaderboardRepositoryProvider`.
- [ ] In `app/lib/services/providers.dart`: remove the `leaderboardServiceProvider`
      and `leaderboardRepositoryProvider` exports, and remove the
      "Re-export leaderboardProvider for convenience" block. Update the
      `// Impact, leaderboard, feedback` comment to drop "leaderboard".
- [ ] In `app/lib/presentation/viewmodels/delete_community_view_model.dart`
      (line 139) and `restore_community_view_model.dart` (line 141):
      delete the `ref.read(leaderboardRepositoryProvider).invalidateLeaderboard()`
      call.
- [ ] Verify (via grep) no other `leaderboard*Provider` / `LeaderboardRepository`
      / `LeaderboardService` (the *cross-community* one) references remain.
**Validates:** `flutter analyze` (via `npm run lint:app` or
`cd app && dart analyze`) reports no broken imports or unresolved
identifiers in client code.

### Phase 3: Tests + l10n cleanup
**Goal:** Remove tests covering deleted code and dead l10n strings.
**Tasks:**
- [ ] Delete test files:
  - `app/test/data/repositories/leaderboard_repository_test.dart`
  - `app/test/data/repositories/leaderboard_repository_test.mocks.dart`
  - `app/test/presentation/viewmodels/leaderboard_view_model_test.dart`
  - `app/test/presentation/viewmodels/leaderboard_view_model_test.mocks.dart`
  - `app/test/presentation/screens/leaderboard/` (entire dir)
- [ ] In `app/test/presentation/viewmodels/delete_community_view_model_test.dart`
      and `restore_community_view_model_test.dart`: remove the
      `import 'package:ripls/data/repositories/leaderboard_repository.dart'`
      and the `leaderboardRepositoryProvider.overrideWithValue(mockLeaderboard)`
      override lines + the `mockLeaderboard` setup.
- [ ] In `app/test/presentation/screens/settings/restore_community_screen_test.dart`:
      same cleanup as above.
- [ ] Search for any remaining mock leaderboard generation (e.g.,
      `@GenerateMocks` annotations referencing `LeaderboardRepository`
      or `LeaderboardService`) and remove. Regenerate mocks with
      `cd app && flutter pub run build_runner build --delete-conflicting-outputs`
      if any annotation changed.
- [ ] In `app/lib/l10n/app_en.arb`: remove the `leaderboardTitle` key
      (line 1345-1346) and the `communityLeaderboard` key (line ~962
      and its `@communityLeaderboard` description). Check
      `app/lib/l10n/app_es.arb` and any other locale ARBs for the same
      keys and remove them too. Regenerate localizations via the
      project-standard `npm run generate` (covers both proto and l10n
      generation in one pass — do not call `flutter gen-l10n` directly
      per CLAUDE.md).
- [ ] **Do NOT remove** the body string at `app_en.arb` line ~4261
      (`portfolioDetailRankingDescription`: "...The leaderboard below
      shows the top contributors on Ripls...") or the description at
      line ~5313 (a11y label for the impact-metrics top-contributors
      row). Both describe the *kept* per-community top-contributors
      feature in `community_metrics_breakdown.dart`, not the deleted
      cross-community service. Mirror the same preservation in
      `app_es.arb`.
- [ ] Run `npm run test:app` (which invokes `flutter test --reporter
      expanded` per CLAUDE.md) and resolve any remaining failures.
- [ ] Run `npm run lint:dart:size` and any other CI lints touched by
      the diff (`npm run lint:ephemeral-comments`,
      `npm run lint:codeinternal` if changed).
**Validates:** Full Flutter and Go test suites pass; no broken
imports; no dead l10n strings flagged.

### Phase 3.5: Stale doc cleanup
**Goal:** Remove dead doc reference uncovered during audit.
**Tasks:**
- [ ] In `docs/client/architecture.md` line 345, the
      `NavigationHelpers.pushScreen()` row lists "leaderboard" as a
      primary content screen. Drop "leaderboard" from that
      comma-separated list (other items stay).
**Validates:** Docs no longer reference a deleted screen.

### Phase 4: Final verification
**Goal:** Confirm nothing dangling remains and the build is clean.
**Tasks:**
- [ ] `grep -ri "LeaderboardService\b\|LeaderboardRepository\b\|leaderboardProvider\b\|leaderboardServiceProvider\b\|leaderboardRepositoryProvider\b\|LeaderboardNotifier\b\|LeaderboardScreen\b\|CommunityLeaderboardScreen\b\|leaderboard_screen\b\|community_leaderboard_screen\b\|leaderboard_view_model\b\|leaderboard_repository\b\|leaderboard_service\b\|leaderboard_item\b\|leaderboard.proto\b\|widgets/leaderboard\b"`
      across the repo should return zero hits outside of `docs/issues/`,
      `docs/reviews/`, and `docs/ai/` (historical plan docs).
- [ ] Grep for `CommunityLeaderboardEntry`, `SearchCommunitiesRequest`,
      `SearchCommunitiesResponse`, `LeaderboardMetric` — all should be
      gone (these came from the deleted proto).
- [ ] Confirm the following all still exist and compile — these are
      *kept* because they belong to the per-community top-contributors
      feature, not the deleted cross-community service:
      `GetCommunityLeaderboard` RPC, `LeaderboardMember`,
      `LeaderboardEntry` (the `CommunityComparison` one in
      `impact_service.proto`), `community_metrics_breakdown.dart`,
      `community_compare_v2.dart`, and especially
      `app/lib/presentation/viewmodels/community_metric_detail_view_model.dart`
      (which keeps identifiers like `moneyLeaderboard`, `co2Leaderboard`,
      `qualityTimeLeaderboard`, `_withLeaderboard`, `_getCachedLeaderboard`
      and calls `_repository.getCommunityLeaderboard(...)` — do not
      remove these on a naive grep).
- [ ] `npm run build` clean.
- [ ] `npm run test:app` clean.
- [ ] `go test -tags=integration -timeout 20m ./...` clean.
**Validates:** End-to-end build / test green; dead code is gone;
live code is preserved.

## Related Issues

- #1281 — "leaderboard N+1" — this earlier perf issue was filed
  against the now-dead cross-community leaderboard's N+1 member-count
  query. Once #2102 lands, #1281 is moot and should be closed as
  "resolved by deletion." The implementation agent should comment on
  #1281 and close it.
