# Issue #1889: Redesign user screen to mirror Workshop layout, scoped to shared communities

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1889
**Filed:** 2026-05-12 | **Last activity:** 2026-05-12
**Labels:** none | **Assignees:** unassigned
**Context docs:** [docs/workshop.md](../workshop.md), [docs/impact_metrics.md](../impact_metrics.md), [docs/cowork/App Design/profile-refined.html](../cowork/App%20Design/profile-refined.html)

## Decisions Resolved (2026-05-12)

Author confirmed via chat:

- **"Workbench" did mean Workshop** — proceed against [docs/workshop.md](../workshop.md). Note resolved.
- **RPC home**: `ProfileService` (new service in `server/services/profile/`).
- **Hero/ticker scope inverted from earlier draft.** Numbers shown are the *target user's totals across all of their communities* — not the (viewer ∩ target) intersection. The privacy boundary moves out of the aggregate numbers and into two narrower places:
  1. The identity-block subtitle: names only **shared** communities; non-shared communities collapse to `"+N other communities"` (count only).
  2. The detail screens: aggregated chart and totals render unchanged for everyone, but per-record entries (transactions, events) are **anonymized or omitted when they belong to a community the viewer is not in**.
  3. The postcard set stays viewer-scoped (postcards are actionable, so the viewer must be able to see the underlying entity).
  This dramatically simplifies the server: no per-community calculator iteration; reuse the same aggregation `GetUserImpactMetrics` already performs.
- **Postcards in v1**: ship the three deterministic templated cards described below (gear / experience / help). Tap dispatcher uses entity-type routing. `WorkshopPostcard` gets an optional `onTap` so the existing Workshop tab is unaffected.
- **No shared communities case**: the default profile screen still renders identity + hero + ticker (target's full numbers). The postcard section is hidden. Detail screens still render the chart and totals but show no per-record specifics.
- **`UserMetricsScreen` is removed entirely.** All of `app/lib/presentation/screens/users/user_metrics_screen.dart`, `app/lib/presentation/viewmodels/user_impact_view_model.dart` (including `UserImpactMetricsNotifier`, `userImpactProvider`, `UserImpactData`), and `app/lib/presentation/models/impact_metrics_content_adapters.dart#userImpactToContentData` are dead after this work — grep confirms they have no other consumers. Own-impact viewing keeps using `PortfolioMetricsScreen` ([app/lib/presentation/screens/portfolio/portfolio_metrics_screen.dart](../../app/lib/presentation/screens/portfolio/portfolio_metrics_screen.dart)) reachable from `ProfileSettingsScreen`.
- **Viewer == target redirect**: routes to `PortfolioMetricsScreen` (not the new `UserScreen`). The new `UserScreen` is exclusively the viewer-of-another-user surface.
- **Hero copy ambiguity (TBD, lightweight)**: the mock reads `"TOGETHER SINCE APRIL 2026"` / `"hours together"`. With the inverted scope, "together" is a soft framing — "since the target was first active in any community" rather than "since you two overlapped." Acceptable for v1; if a content reviewer pushes back, fall back to `"ACTIVE SINCE …"` / `"hours contributed"`. Not blocking.

## Audit Revisions (2026-05-12)

Audited against [docs/client/architecture.md](../client/architecture.md), [docs/client/caching.md](../client/caching.md), [docs/client/i18n.md](../client/i18n.md), [docs/client/accessibility.md](../client/accessibility.md), [docs/client/testing.md](../client/testing.md), [docs/server/architecture.md](../server/architecture.md), [docs/server/observability.md](../server/observability.md). Changes vs. the initial draft:

- **Cache namespace renamed** `'user_profile'` → `'profile'` to match the single-token convention in [client/caching.md](../client/caching.md) (sibling namespaces: `'user'`, `'gear'`, `'loan'`, `'community'`). Cache key format `'profile:<target_user_id>'`. *Why:* the doc explicitly defines `'namespace:identifier'` and the namespace is one token.
- **ViewModel typing tightened** to `AsyncNotifierProvider.autoDispose.family<UserProfileNotifier, UserProfileState, String>` (matching [user_impact_view_model.dart](../../app/lib/presentation/viewmodels/user_impact_view_model.dart)). Added the `ref.mounted` guards after each `await` in any non-`build()` method ([client/architecture.md §Async Safety](../client/architecture.md#async-safety-in-notifiers)). *Why:* autoDispose family is the established shape for per-target-user state; the original plan said "AsyncNotifier" generically.
- **i18n requirements made concrete**: every new ARB key must land in both `app_en.arb` AND `app_es.arb` with `@key` metadata before `npm run generate`; "+N other communities" uses ICU plural per [client/i18n.md](../client/i18n.md). Viewmodels expose typed `SharedCommunityRef` lists, never pre-formatted strings. *Why:* the original plan named keys but didn't bind them to the bilingual + ICU rules.
- **Accessibility primitives named explicitly**. Community-name links use `Tappable` with `semanticsLabel: context.l10n.a11yOpenCommunity(name)` (new `a11y*` key); `IconAction` for any icon-only control; `accessibleDuration(context, ...)` for any animation. No `GestureDetector`/`InkWell` outside `widgets/accessibility/`. *Why:* the original plan said "tap target" without binding to the project's a11y primitives; the lint rules in [scripts/check_a11y.js](../../scripts/check_a11y.js) would have caught this in PR review anyway.
- **Server errors must use `connecterr.Internal(ctx, op, err, kv...)`** — bare `connect.NewError(connect.CodeInternal, ...)` fails the build via [scripts/check_codeinternal.js](../../scripts/check_codeinternal.js). Caller-input errors stay as `connect.NewError(connect.CodeInvalidArgument, ...)`. *Why:* the original plan didn't name the helper; this is enforced and would be caught at build time.
- **Goroutine safety reminder**: v1 does not spawn background goroutines, so `logging.GoSafe` is not needed. Flagged anyway so a future "kick off materialization" iteration doesn't slip a bare `go func()` past CI ([server/architecture.md §Goroutine Safety](../server/architecture.md#goroutine-safety)).
- **N+1 prevention pinned with tests** via `storage.AssertMaxQueries(t, ctx, max, fn)` — the calculator iterates over shared communities and could regress easily ([CLAUDE.md §SQL Efficiency](../../CLAUDE.md)).
- **Observability fixed**: the viewer's `user_id` is *automatically* injected by `logging.LoggerWithContext(ctx)` from auth — the original plan incorrectly said to "mask" it. The target user gets a dedicated `target_user_id` field (mirroring the `target_request_id` convention in [server/observability.md §Request ID glossary](../server/observability.md#request-id-glossary)). Add a privacy invariant: log entries must never carry non-shared community IDs, since logs are searchable.
- **Proto: `optional` for unset fields** per [CLAUDE.md §Proto Field Conventions](../../CLAUDE.md). `target_description`, `target_media_id`, and `payload` (empty when no shared communities) must be `optional`. The `repeated SharedCommunityRef shared_communities` stays as a normal repeated field (no `optional` for repeateds).
- **Wire-type reuse clarified**: the plan reuses `BriefPayload` as a *nested field type*. That is not the same as reusing an RPC's request/response message — the "never reuse" rule in [CLAUDE.md](../../CLAUDE.md) applies to top-level RPC messages only. Called out explicitly so a reviewer doesn't flag it.
- **New gap surfaced — detail-screen scoping.** `WorkshopActsDetailScreen` / `WorkshopMoneyDetailScreen` / `WorkshopCo2DetailScreen` / `WorkshopTimeDetailScreen` currently aggregate the *caller's* activity inside the passed `communityIds`. Tapping a target user's ticker would surface viewer-scoped numbers, not target-scoped — a silent correctness bug. Added as Open Question + new Phase 3.5 with three options (parameterize, fork, or drop drill-down for v1). The original plan glossed over this.
- **Routing**: the new `UserScreen` must adopt `SwipeToCloseMixin` and `buildSwipeableScaffold` because it's pushed via `NavigationHelpers.pushScreen()` ([client/architecture.md §Navigation & Gestures](../client/architecture.md#navigation--gestures)).
- **Membership-mutation invalidation**: joining/leaving a community changes the shared set, so `JoinCommunity` / `LeaveCommunity` / `AcceptInvitationLink` / `RestoreCommunity` / `RejoinCommunity` repositories must invalidate `profile:*` after mutation. Listed as a Phase 2 task — easy to miss otherwise.
- **CI gate matrix added to Phase 5** (`lint:go`, `lint:dart:i18n`, `lint:dart:a11y`, `lint:dart:size`, `lint:nav`, `lint:ephemeral-comments`, `lint:codeinternal`, `lint:check-goroutines`, `lint:check-active-community-gate`, plus `go build -tags=integration ./...` since `Calculator` is consumed by integration tests).
- **Test layering aligned to MVVM**: ViewModel tests use `ProviderContainer` + Mockito on `ProfileRepository` (not the service); disposal-safety tests required for the AsyncNotifier per [client/architecture.md §Disposal Testing](../client/architecture.md#disposal-testing); widget tests on the identity block need `localizationsDelegates` per [client/i18n.md](../client/i18n.md).

## Problem Statement

Tapping another user's name anywhere in the app today routes to [app/lib/presentation/screens/users/user_screen.dart](../../app/lib/presentation/screens/users/user_screen.dart), which is a thin wrapper over [user_metrics_screen.dart](../../app/lib/presentation/screens/users/user_metrics_screen.dart) — the read-only Impact 2.0 portfolio layout. That layout was a generic "user impact" view (issue #1009 landed it). The new direction in the mock at [docs/cowork/App Design/profile-refined.html](../cowork/App%20Design/profile-refined.html) is community-scoped: the *viewer* should see the *target* user through the lens of communities they share — avatar + name + inline links to each shared community, then the same hero / ticker / postcards pattern the Workshop tab uses, but with every number aggregated only across the communities both users belong to. For communities the target is in but the viewer isn't, the name is hidden and only the count surfaces.

Concrete shifts from today:

- **Layout source-of-truth changes** from `ImpactMetricsContent` (in [app/lib/presentation/widgets/impact/](../../app/lib/presentation/widgets/impact/)) to the Workshop widgets in [app/lib/presentation/widgets/workshop/workshop_home_layout.dart](../../app/lib/presentation/widgets/workshop/workshop_home_layout.dart): `WorkshopHero`, `WorkshopTicker`, `WorkshopSectionHeader`, `WorkshopShowMoreLink`, `WorkshopPostcard`. The Workshop's `WorkshopTopStrip` is replaced by a new identity block (avatar + name + inline community subtitle).
- **Numeric scope** for the hero and ticker stays the **target user's full totals across every community they belong to** — same aggregation that today's `GetUserImpactMetrics` already performs. The privacy boundary is *not* in the aggregate numbers; it lives in three narrower places:
  1. The identity-block subtitle, which names only shared communities and collapses non-shared ones to `"+N other communities"` (count only — no IDs, no names).
  2. The Workshop detail screens (acts / money / CO₂ / time), which still render the full chart and aggregate totals but **anonymize or omit per-record entries that belong to communities the viewer isn't in**. Aggregate is fine; specifics are scoped.
  3. The postcard set, which references actionable entities (gear, experiences, help offers) and must therefore be limited to data the viewer can see — i.e., entities in shared communities.
- **Postcards** become viewer-facing ways to "lean on" the target ("Borrow his pressure washer", "Join Saturday's hike") rather than the host-facing Workshop nudges. The detector chain in [server/services/workshop/momentum/](../../server/services/workshop/momentum/) was designed for the host's own home and emits owner-scoped nudges (active quest, calendar gap, etc.); it cannot be reused as-is.
- **`UserMetricsScreen` is removed.** Today's `UserScreen` wraps [user_metrics_screen.dart](../../app/lib/presentation/screens/users/user_metrics_screen.dart) (a read-only Impact 2.0 portfolio layout). After this work the file is deleted, along with `userImpactProvider`, `UserImpactMetricsNotifier`, `UserImpactData`, and the `userImpactToContentData` adapter — grep confirms these have no other consumers. Own-impact viewing continues via `PortfolioMetricsScreen` reachable from `ProfileSettingsScreen`.

The issue title says "scoped to shared communities," but the resolved scope is narrower: only the *subtitle naming*, *detail-screen per-record visibility*, and *postcard surface* are intersection-scoped. The aggregate numbers themselves render the target user's full activity.

## Context from Comments

The issue has no GitHub comments yet — it was filed minutes before this plan was drafted, by the same conversation that produced both. Worth flagging because:

- The author has not yet validated the assumption that "workbench" in the original prompt meant `docs/workshop.md`. The issue body calls this out explicitly; the recommendation here proceeds on that assumption.
- No one has weighed in on the postcard content for a viewer-facing profile, which is the biggest open product question (see Open Questions). Spec'd here is a deterministic v1 set; richer detection is deferred.
- The prior user-screen redesign ([docs/issues/1009-user-screen-redesign.md](./1009-user-screen-redesign.md)) is the closest precedent. It chose Option A (extract a shared `ImpactMetricsContent` widget) for the same DRY reason this issue asks for Workshop-widget reuse. The pattern should mirror that — extend widget APIs rather than fork — but the *data* and *scope* are different, so the same shared-content trick won't apply directly.

## Potential Solutions

### Option A: New viewer-scoped server RPC + thin client adapter

- **Approach:** Add a new RPC, e.g. `GetUserProfileForViewer(target_user_id)`, on a sensible service (likely [user_service.proto](../../proto/ripls/api/user_service.proto) or a new `profile_service.proto` to keep `UserService` focused; see Open Questions). The handler:
  1. Authenticates the viewer.
  2. Computes `shared_community_ids` = intersection of viewer's active memberships ∩ target's active memberships.
  3. Computes the non-shared remainder count.
  4. For the shared set, calls `impact_metrics.Calculator.CalculateUserImpactInCommunity` per community per dimension (using the existing helper at [server/impact_metrics/calculator.go:897](../../server/impact_metrics/calculator.go#L897)), aggregates with quadrature for the ticker.
  5. Computes `since_unix_sec` = min over shared communities of `max(viewer.joined_at, target.joined_at)`.
  6. Computes `hours_together` from QT minutes on transactions where both viewer and target participated, within shared communities (see Open Questions on the exact formula).
  7. Emits 0–3 templated postcards (see Phase 4) — borrow-friendly gear, next upcoming event the target hosts in a shared community, recent help offered. Postcards reuse the existing `BriefCTARow` wire shape so client widgets render unchanged.
  8. Returns target identity (name, primary media id, description), shared-community names + ids, non-shared count, and a `BriefPayload`-shaped block for the hero/ticker/postcards.
- **Client:** New `UserProfileViewModel` (mirrors `WorkshopNotifier` shape — `AsyncNotifier<UserProfileState>` with a `BriefPayload`-shaped field). Replace `UserScreen`'s body with a new layout that reuses `WorkshopHero` / `WorkshopTicker` / `WorkshopSectionHeader` / `WorkshopShowMoreLink` / `WorkshopPostcard` verbatim. Build one new widget for the identity block (avatar + name + subtitle with linked community names + "+N other communities" tail).
- **Pros:** Single round trip. Clean privacy boundary — server is the only place that knows the full intersection logic. Workshop widgets reused without modification. Wire shape (`BriefPayload`) already designed to render hero+ticker+postcards.
- **Cons:** Requires a net-new RPC and proto messages. Some server compute overlap with `GetUserImpactMetrics` (same per-community user impact calculator under the hood, different filter set). Postcards for a viewer profile need a separate code path — they're *not* the same as Workshop nudges.
- **Effort:** Medium-large.

### Option B: Extend GetUserImpactMetrics with a viewer-scoped mode

- **Approach:** Add `bool viewer_scoped = N` and `repeated string shared_community_ids_override = N` to [`GetUserImpactMetricsRequest`](../../proto/ripls/api/impact_service.proto). When set, the server scopes all aggregations to the intersection of viewer and target memberships, omits ranked-community names the viewer can't see, and adds the count. Reuse the same response — most fields already line up with what the new screen needs (`metrics`, `user_name`, `user_media_id`, `ranked_communities`).
- **Client:** Same as Option A but reads from `userImpactProvider` with new args. Build a translation layer that maps `UserImpactMetrics` → the fields the Workshop widgets expect (`BriefPayload.acts_count`, `replaced_cost_usd`, `co2_avoided_pounds`, `hours_together`, `since_unix_sec`, `cta_rows`).
- **Pros:** No new RPC. The pre-existing `is_own_profile` field already partitions own/other behavior. Touches less of the proto surface.
- **Cons:** Violates the project rule "Never reuse RPC request or response message types across multiple RPCs" — same response would serve two semantically different surfaces (existing impact-metrics screen + new Workshop-style screen). The shape needs `BriefPayload` fields it doesn't currently have (e.g. `since_unix_sec`, `hours_together`, `cta_rows`); bolting them onto `UserImpactMetrics` muddies that proto. Also pulls postcard generation into the impact-metrics service, which doesn't own nudge data.
- **Effort:** Medium.

### Option C: Pure-client aggregation

- **Approach:** Call `ListCommunities` (viewer's communities, exists today) + `GetUser` (target identity) + a new tiny RPC like `GetUserCommunityIds(target_user_id)` that returns *just* the target's community IDs. Compute the intersection client-side; then call `GetUserCommunityImpactDetail` (exists at [server/services/impact_metrics/user_impact_detail.go](../../server/services/impact_metrics/user_impact_detail.go)) once per shared community and aggregate the numbers on the client.
- **Pros:** Smallest server change. Reuses existing detail RPCs.
- **Cons:** N RPCs instead of 1. Worse, **leaks the full target community ID list to the client**, which defeats the privacy goal: a malicious viewer can read off the target's non-shared community IDs even if the UI hides their names. The whole point of the issue's "+N other communities" requirement is that the viewer shouldn't see what those communities *are*. This option fails that requirement.
- **Effort:** Small.

## Recommendation

**Option A.** It's the only option that holds the privacy boundary cleanly: the server is the single place that knows both users' full community sets and only emits the intersection plus a count. Option B would require denormalizing two unrelated surfaces into the same proto and violate the "no reused message types" rule. Option C leaks the target's full membership list to the client. The net-new RPC cost in Option A is real but bounded — the calculator helpers already exist, and the wire shape borrows `BriefPayload` so the client widgets render unchanged.

Decision dependencies to confirm before implementation:
- The exact home of the new RPC (UserService vs new ProfileService — see Open Questions).
- The "hours together" formula. The candidates have meaningfully different compute costs (see Open Questions).
- The v1 postcard set. The mock shows three card types that don't map 1:1 to existing detectors.

## Open Questions

> Decisions confirmed by the author are captured in the **Decisions Resolved** section above. The remaining questions below are minor and can be settled during implementation.

- **Hero copy wording given the inverted scope** — the mock reads `"TOGETHER SINCE <Month Year>"` / `"hours together"`. With the target's full-scope numbers (not the intersection), "together" is soft framing rather than literal. Recommendation: keep the mock copy in v1 and re-interpret "since" as "since the target first joined any community" (`min(target.joined_at)` over their active memberships). If a content reviewer pushes back, the fallback is `"ACTIVE SINCE <Month Year>"` / `"hours contributed"`. Pin the eyebrow ARB key as `userProfileHeroEyebrow` and the unit ARB key as `userProfileHeroUnit` so the copy is a one-line ARB swap.
- **Anonymization strategy in detail screens** — Phase 3.5 must hide per-record entries from non-shared communities. Two reasonable shapes:
  - **Omit non-visible rows entirely** — the totals/chart still match because they came from aggregate queries; the row list is just shorter. Simple, but a 1000-act target with 2 shared rows looks misleadingly sparse.
  - **Render anonymized stub rows** — e.g., `"An event in another community"` or `"A shared item in another community"` with a generic icon and no link. Preserves the sense of magnitude.
  - Recommendation: **anonymized stubs**, capped at a small number per detail screen (e.g., 5 stub rows, then "+N more"). Better matches the user's intent ("show the metrics and charts but no specifics").
- **What does tapping a postcard do?** — Open the underlying entity, not the host's draft flow. `WorkshopLeverDispatcher.openPostcard` (in [workshop_lever_dispatcher.dart](../../app/lib/presentation/widgets/workshop/workshop_lever_dispatcher.dart)) seeds an experience draft for the host — wrong vocabulary for a viewer. The viewer profile needs a sibling dispatcher (`UserProfileLeverDispatcher.openPostcard`) that routes by entity type: gear → gear detail screen, experience → experience detail, help → request detail. Build it once; pass it into `WorkshopPostcard` via the new optional `onTap` callback.
- **Does `WorkshopPostcard` need a new tap API?** — Yes. Today the tap is hard-wired to `WorkshopLeverDispatcher.openPostcard`. Add an optional `VoidCallback? onTap` to `WorkshopPostcard`; when null it falls back to the existing host dispatcher (Workshop tab unaffected).
- **Wire-shape backwards compatibility** — N/A: this is a new RPC, no old clients.

## Implementation Plan

### Phase 1: Server — viewer-aware profile RPC, shape only

**Goal:** Ship `GetUserProfileForViewer` returning identity, shared-community list, non-shared count, and the target's **full** hero/ticker numbers. No postcards yet.

**Tasks:**
- [ ] Add `proto/ripls/api/profile_service.proto` with `ProfileService` containing `GetUserProfileForViewer(GetUserProfileForViewerRequest) returns (GetUserProfileForViewerResponse)`. Define dedicated request/response messages (no reuse of *top-level* RPC types) per [docs/proto_conventions.md](../proto_conventions.md). Nested wire types like `BriefPayload` and `SharedCommunityRef` are field types, not RPC contracts — reuse is fine.
- [ ] Response fields:
  - `string target_user_id`
  - `string target_name`
  - `optional string target_description` — empty for users with no bio; `optional` per [CLAUDE.md §Proto Field Conventions](../../CLAUDE.md)
  - `optional string target_media_id` — same reasoning
  - `repeated SharedCommunityRef shared_communities` (each `{string id, string name}`) — communities the viewer and target both belong to; repeated fields never take `optional`
  - `int32 other_community_count` — count of communities the target is in that the viewer is *not* in; **never accompanied by their IDs anywhere in the response or in log fields**
  - `BriefPayload payload` — hero / ticker / (empty for now) postcards. Numbers are the **target's totals across all of their communities**, not the intersection.
- [ ] Proto field comments must avoid vendor names, transport plumbing, and project-convention references per [CLAUDE.md §Proto Comments](../../CLAUDE.md). Describe what each field *means*, not how the server produces it.
- [ ] Service skeleton in `server/services/profile/` with `doc.go` (1–3 sentences, satisfies `revive.package-comments`) + `README.md` (purpose, key files, when to add code here vs. an adjacent service) per [CLAUDE.md §Style Guidelines](../../CLAUDE.md). File layout: `service.go` (struct + constructor) + `profile.go` (RPC handler). Keep each file < 1000 lines per [server/architecture.md §File Size Guidelines](../server/architecture.md#file-size-guidelines).
- [ ] Constructor signature: `profile.New(storage *storage.ProtoSQLStorage, calculator *impact_metrics.Calculator) *Service`. **No cross-service dependencies** — `Calculator` is a shared library at `server/impact_metrics/`, not a service ([server/architecture.md §Service Independence](../server/architecture.md#1-service-independence)).
- [ ] **Membership work** (drives identity + postcard scope, *not* the hero/ticker totals):
  - Load viewer's active `CommunityUser` rows in one query
  - Load target's active `CommunityUser` rows in one query
  - Intersect in Go → `shared_community_ids` (resolve names for the response — one batched `GetByIDs` against `Community`)
  - `other_community_count = len(target_active_communities) - len(shared_community_ids)`
  - **Privacy invariant:** non-shared community IDs must never appear in the response, never appear as log fields, never appear in error messages bubbled to the wire. Pin this with a leakage test (see below).
- [ ] **Aggregate numbers** (hero + ticker) — computed across **all** of the target's active communities, not the intersection. Two implementation choices, equivalent for v1:
  - **Reuse** the existing aggregator in [server/services/impact_metrics/user_metrics.go](../../server/services/impact_metrics/user_metrics.go) (the same path `GetUserImpactMetrics` already exercises). Factor the per-user totals computation into a shared helper on `impact_metrics.Calculator` so the new `ProfileService` doesn't depend on `impact_metrics.Service` (the calculator is the shared library; the service is not — preserves the Service Independence rule).
  - Or, alternately, iterate the target's communities and sum via `Calculator.CalculateUserImpactInCommunity` (already exists per [server/impact_metrics/calculator.go:897](../../server/impact_metrics/calculator.go#L897)). Same result; the first option is fewer queries.
  - Populate `BriefPayload.replaced_cost_usd`, `co2_avoided_pounds`, `acts_count`, `hours_together` from these totals.
- [ ] `BriefPayload.since_unix_sec` = `min over target.active_communities of target.joined_at` — the earliest moment the target was active in *any* community. (Not the intersection.) Drives the "TOGETHER SINCE …" eyebrow per the Decisions Resolved section.
- [ ] Leave `cta_rows` empty in this phase — postcards land in Phase 4.
- [ ] **Out of scope for Phase 1**: any new "both viewer and target participated" filter. That framing belongs to the *old* intersection-scoped model and is no longer needed.
- [ ] Auth: `auth.RequireAuth(ctx)`; reject empty `target_user_id` with `connect.NewError(connect.CodeInvalidArgument, ...)`; reject viewer == target with the same code (client should redirect to `PortfolioMetricsScreen` first, but server is the contract owner).
- [ ] **Error wrapping:** every internal failure (storage, calculator, etc.) must use `connecterr.Internal(ctx, "GetUserProfileForViewer.<sub-op>", err, "target_user_id", req.Msg.TargetUserId, ...)`. Bare `connect.NewError(connect.CodeInternal, err)` is forbidden — `scripts/check_codeinternal.js` fails the build ([server/architecture.md §Error wrapping](../server/architecture.md#error-wrapping)).
- [ ] **Logging:** `logging.LoggerWithContext(ctx).With("operation", "GetUserProfileForViewer", "target_user_id", req.Msg.TargetUserId, "shared_community_count", len(shared), "other_community_count", otherCount)`. The viewer's `user_id` is injected automatically — do not re-add it. INFO on success; DEBUG inside hot loops; WARN on best-effort partial failures; ERROR only via `connecterr.Internal`. No `log.Printf` / `fmt.Printf` ([server/observability.md §Standard Field Names](../server/observability.md#standard-field-names)).
- [ ] **Goroutines:** v1 does not spawn background goroutines. If a future iteration adds one, it must use `logging.GoSafe(ctx, "name", fn)` — bare `go func()` fails `server/cmd/check-goroutines` ([server/architecture.md §Goroutine Safety](../server/architecture.md#goroutine-safety)).
- [ ] Tests in `service_test.go` and `profile_test.go` (unit tests alongside implementation), all using `storage.SetupTestStorage(t)` (testcontainers `pgvector/pgvector:pg16`):
  - **Numbers are target-scoped, not intersection-scoped** — seed the target with activity in two communities A (shared with viewer) and B (not shared). Assert the response's hero/ticker numbers equal target totals across **A + B**, not just A.
  - Identity block: `shared_communities` contains A only; `other_community_count == 1`.
  - **Leakage test**: when `other_community_count > 0`, assert the response proto's serialized bytes contain **none** of the non-shared community IDs, **none** of their names, and that captured log lines contain neither.
  - Empty-overlap case: `shared_communities` empty, `other_community_count > 0`, hero/ticker still populated with the target's full totals.
  - Self-call rejected with `connect.CodeInvalidArgument`.
  - Empty `target_user_id` rejected.
  - Unauthenticated rejected with `connect.CodeUnauthenticated`.
  - `storage.AssertMaxQueries(t, ctx, max, func)` pinning the query budget. Without per-community iteration the bound is small: ~2 membership queries + 1 user lookup + the calculator's existing query plan (which already has its own bounds in [server/services/impact_metrics/user_metrics.go](../../server/services/impact_metrics/user_metrics.go)). Pick a constant cap, fail any future N+1 regression.
- [ ] Wire into `server/main.go` startup and the apiconnect router. The connect handler is registered via `apiconnect.NewProfileServiceHandler` once the proto is generated.
- [ ] Run **`go build -tags=integration ./...`** in addition to `npm run build` — if the calculator helper is factored, integration tests under `server/services/experience/` and `server/jobs/` may pick up the new signature ([CLAUDE.md §Go Server Build Verification](../../CLAUDE.md)).

**Validates:** that the hero/ticker reflect the target's full activity, that the identity block lists only shared communities by name, that the non-shared count is present but never accompanied by IDs in response or logs.

### Phase 2: Client — new screen and view model, identity + hero + ticker

**Goal:** Replace `UserScreen`'s body with the new layout. Identity block, hero, ticker — no postcards yet. Wire to the new RPC.

**Tasks:**
- [ ] Generate Dart from the new proto (`npm run generate`).
- [ ] Add `app/lib/services/profile_service.dart` mirroring [workshop_service.dart](../../app/lib/services/workshop_service.dart) (the canonical thin service: `connect.Transport`, `RpcErrorHandler`, `RpcUtils.executeRpc`, no caching).
- [ ] Add `app/lib/data/repositories/profile_repository.dart` with the `'profile'` cache namespace ([client/caching.md §Cache Key Naming Convention](../client/caching.md#cache-key-naming-convention)). Methods: `Future<GetUserProfileForViewerResponse> getProfile(String targetUserId)` (keyed `targetUserId`, cache miss calls service); `Future<void> invalidate(String targetUserId)`; `Future<void> invalidateAll()`. ViewModels never touch `CacheManager`/`CacheService` — only the repo does ([client/architecture.md §Repository Pattern](../client/architecture.md#4-repository-pattern-with-stash-based-caching)).
- [ ] **Membership-mutation invalidation hooks**: in `CommunityRepository`, after each of `join`, `leave`, `restore`, `rejoin`, `acceptInvitationLink` (any call that changes the viewer's active membership set), invalidate `profile:*` via the shared `CacheManager`. The viewer's set changing changes the shared set for *every* profile they might open, so a single-key invalidation isn't enough — `_profileCache.invalidatePattern('profile:*')` is the right hammer. Document the dependency in `community_repository.dart`'s class docstring.
- [ ] Add `app/lib/presentation/viewmodels/user_profile_view_model.dart` — `AsyncNotifierProvider.autoDispose.family<UserProfileNotifier, UserProfileState, String>(UserProfileNotifier.new)` keyed by `targetUserId`. Mirrors the shape of [user_impact_view_model.dart](../../app/lib/presentation/viewmodels/user_impact_view_model.dart). State is a `@freezed` class exposing typed fields:
  - `String targetName`
  - `String? targetDescription`, `String? targetMediaUrl`
  - `List<SharedCommunityRef> sharedCommunities`
  - `int otherCommunityCount`
  - `BriefPayload? payload`
  - `bool get hasSharedCommunities => sharedCommunities.isNotEmpty`
  - Viewmodel exposes only typed state, **never pre-formatted strings** ([client/i18n.md §Best Practices](../client/i18n.md)). All copy resolved in the widget via `context.l10n`.
- [ ] **`build()` reads the repo; `refresh()` re-runs it**. Per [client/architecture.md §Async Safety](../client/architecture.md#async-safety-in-notifiers), `build()` is disposal-safe automatically; non-build methods (`refresh`) must check `ref.mounted` after every `await` before assigning to `state`. Use `AsyncValue.guard` to wrap re-loads.
- [ ] Add ARB keys to **both** `app/lib/l10n/app_en.arb` and `app/lib/l10n/app_es.arb` (with `@key` metadata + ICU plurals) per [client/i18n.md](../client/i18n.md). New keys:
  - `userProfileSharedLeadIn` → `"With "` / `"Con "` (leads into the linked community names)
  - `userProfileOtherCommunities`, ICU plural over `count`:
    - `=0{}` (empty when no extras)
    - `=1{ and one other community}` / `=1{ y otra comunidad}`
    - `other{ and {count} other communities}` / `other{ y {count} comunidades más}`
  - `userProfileNoSharedCommunities` → `"No communities in common yet"` / `"Aún no hay comunidades en común"`
  - `userProfilePostcardSectionHeader(name)` → `"Three ways to lean on {name}"` / `"Tres maneras de contar con {name}"` (used in Phase 4)
  - `a11yOpenCommunity(name)` → `"Open community {name}"` / `"Abrir comunidad {name}"` (a11y prefix per [client/accessibility.md §Writing good accessibility labels](../client/accessibility.md#writing-good-accessibility-labels))
- [ ] Build new identity block widget `app/lib/presentation/widgets/user_profile/user_profile_identity.dart` (avatar + name + subtitle). Subtitle composed via `Text.rich` with:
  - `TextSpan` for the lead-in (`context.l10n.userProfileSharedLeadIn`)
  - One `WidgetSpan` per shared community wrapping a `Tappable` (from `widgets/accessibility/`) around the community name; `semanticsLabel: context.l10n.a11yOpenCommunity(community.name)`; tap navigates via `NavigationHelpers.pushScreen` to the community screen
  - `TextSpan` joiners (commas, "and") — pull from a small `community_name_joiner.dart` helper if non-trivial
  - `TextSpan` for the `userProfileOtherCommunities` ICU plural tail
  - Avatar uses `CachedMediaImage(semanticsLabel: 'Photo of $targetName')` per the a11y lint rule for `CachedMediaImage`
  - All colors via `AppColors.*` helpers; non-tappable count text uses `AppColors.textTertiary(context)` ([client/architecture.md §Colors and Theming](../client/architecture.md#colors-and-theming))
- [ ] Rewrite `app/lib/presentation/screens/users/user_screen.dart` as a `ConsumerStatefulWidget` with `SingleTickerProviderStateMixin` and `SwipeToCloseMixin` ([client/architecture.md §Navigation & Gestures](../client/architecture.md#navigation--gestures)). Use `buildSwipeableScaffold` + `AppBarBackButton(onPressed: handleClose)`. Body is a `CustomScrollView` of: identity block → `WorkshopHero` → `WorkshopTicker` → empty postcard section. Reuse `WorkshopHero` / `WorkshopTicker` directly — no widget changes in this phase.
- [ ] Wire `WorkshopHero.onTap` → `WorkshopTimeDetailScreen.push(...)` and ticker cells → existing detail screens, **but** see Phase 3.5: the existing detail screens (a) aggregate the *caller's* activity, and (b) show per-record lists that may leak non-shared community data once the queries are switched to the target. For Phase 2 only, leave the tap callbacks unset (the widgets render inert without an `onTap`). Phase 3.5 wires them once the screens are parameterized **and** the anonymization layer for per-record rows is in place.
- [ ] Redirect on viewer == target inside `ContentViewHelpers.openUserScreen` ([content_view_helpers.dart:123](../../app/lib/presentation/widgets/content/content_view_helpers.dart#L123)): when the id matches `ref.read(authStateProvider).user?.id`, push `PortfolioMetricsScreen` via `NavigationHelpers.pushScreen` (it already mixes in `SwipeToCloseMixin`). No `Navigator.push(MaterialPageRoute(...))` per the lint rule. **`UserMetricsScreen` no longer exists after this work, so the redirect target is unambiguously `PortfolioMetricsScreen`.**
- [ ] Empty state: when `sharedCommunities` is empty, render the identity block with the `userProfileNoSharedCommunities` subtitle **plus** the hero and ticker (the target's full numbers still render — the privacy boundary is in the subtitle, not the aggregate). Skip the postcard section (no actionable entities the viewer can see). Detail-screen taps stay inert in this state (Phase 3.5 will render the chart + total but no per-record rows).
- [ ] **Tests** ([client/testing.md](../client/testing.md)):
  - **Primary focus — ViewModel tests** in `app/test/presentation/viewmodels/user_profile_view_model_test.dart`: use `ProviderContainer` with `profileRepositoryProvider.overrideWithValue(MockProfileRepository())`. `@GenerateMocks([ProfileRepository])` and run `flutter pub run build_runner build`. Cover: initial-load success, error path (`AsyncError`), refresh path with `ref.mounted` exit, empty-shared-communities branch, viewer-as-target redirect helper.
  - **Disposal-safety test** per [client/architecture.md §Disposal Testing](../client/architecture.md#disposal-testing): start `loadProfile`, dispose the container, expect no throw.
  - **Repository test** in `app/test/data/repositories/profile_repository_test.dart`: cache-hit (service not called), cache-miss (service called), invalidate after membership mutation (pattern-based clear), `invalidatePattern('profile:*')` reaches every key.
  - **Widget test** for the identity block in `app/test/presentation/widgets/user_profile/user_profile_identity_test.dart` — wraps a test `MaterialApp` with `localizationsDelegates` and `supportedLocales` (per [client/i18n.md](../client/i18n.md)); verifies (a) link tap dispatches navigation, (b) ICU plural renders correctly for 0/1/many other communities, (c) avatar has a non-empty `semanticsLabel`, (d) no `GestureDetector`/`InkWell` appears (a11y compliance).
  - Run via `flutter test --reporter expanded` (the `compact` default produces enormous output when piped; mandated by [CLAUDE.md §Testing](../../CLAUDE.md)).

**Validates:** that all Workshop widgets render unchanged against the new payload — i.e., the DRY contract from the issue's "Acceptance" section — and that the cache, i18n, and a11y conventions hold.

### Phase 3: Postcard dispatcher and `WorkshopPostcard` tap-extension hook

**Goal:** Prepare the postcard surface to accept viewer-facing taps without forking the widget.

**Tasks:**
- [ ] Add an optional `VoidCallback? onTap` to [`WorkshopPostcard`](../../app/lib/presentation/widgets/workshop/workshop_home_layout.dart). When non-null, replace the call to `WorkshopLeverDispatcher.openPostcard` with the provided callback. Default behavior unchanged on the Workshop tab (the Workshop screen continues passing no callback, so it falls through to the existing dispatcher).
- [ ] Add `app/lib/presentation/widgets/user_profile/user_profile_lever_dispatcher.dart` with `openPostcard(BuildContext, WidgetRef, {required String contextId, required PostcardKind kind})`. Routes: `gear` → gear detail, `experience` → experience detail, `help` → request detail.
- [ ] Update `WorkshopPostcard` signature to carry an optional `postcardKind` field on `BriefCTARow` (already on the wire? — no; add it: see below) or carry it via a wrapper struct. Prefer a wrapper struct on the client to avoid a proto change for the discriminator.
- [ ] No `BriefCTARow` proto changes in this phase — `cta_rows` stays empty until Phase 4 lands postcards.
- [ ] Widget tests for the new tap path.

**Validates:** the existing Workshop tab is unaffected; the new tap seam is available for Phase 4.

### Phase 3.5: Parameterize Workshop detail screens with `targetUserId` and add per-record anonymization

> **Status (2026-05-12):** **Deferred for v1** — option 3 of the three originally outlined in the audit ("drop drill-down for v1") was chosen. The hero and ticker on the new `UserScreen` render the target's full aggregate numbers (Phase 1 & 2 already deliver this), but `WorkshopHero.onTap` / ticker cells are left unwired. The drill-down screens stay host-scoped and reachable only from the Workshop tab. Re-opening this phase requires the proto change to `GetCommunityMetricDetailRequest` + server-side stripping outlined below.

**Goal:** Make the hero and ticker drill-downs render the target user's full aggregate numbers (chart + total) on the user profile, host-scoped numbers on the Workshop tab — same screens, one new optional parameter. Per-record rows are visible only when they belong to a shared community.

**Tasks:**
- [ ] Add an optional `String? targetUserId` parameter to each of `WorkshopTimeDetailScreen.push`, `WorkshopActsDetailScreen.push`, `WorkshopMoneyDetailScreen.push`, `WorkshopCo2DetailScreen.push` ([app/lib/presentation/screens/workshop/](../../app/lib/presentation/screens/workshop/)). Default `null` preserves existing host-scoped behavior; non-null switches to target-scoped aggregation across **all** of the target's communities.
- [ ] **Aggregate path**: when `targetUserId != null`, the chart and total reflect the target's full activity (matches the ticker on the profile). Server adds a `target_user_id` field to the existing detail-RPC requests (this is OK — these are *existing* messages, but the field is purely additive and doesn't violate proto reuse rules since the RPC contract evolves rather than being shared with a new RPC). The aggregation runs across the target's full active-community set.
- [ ] **Per-record anonymization (the privacy layer for this phase)**: the per-record rows underneath the chart must be filtered through a visibility check before rendering:
  - If the row's `community_id` ∈ viewer's active communities → render with full detail (name, photo, link).
  - If not → render an anonymized stub row: italic, no link, no photo, copy like `"An event in another community"` / `"A shared item in another community"` (new ARB keys `userProfileAnonymizedEvent`, `userProfileAnonymizedItem`, etc. — both `app_en.arb` and `app_es.arb`).
  - Cap anonymized stubs at a small number (e.g. 5) per dimension, then `"+N more"` follow-up. Aggregate totals are unchanged.
  - Implementation home: the anonymization layer can live in either the server response (server marks a row `is_visible` based on viewer membership and strips identifying fields when false) or the client (server returns viewer's membership set, client filters). **Preferred: server-side stripping** — keeps the privacy boundary in one place, mirrors the Phase 1 invariant, and matches the `is_own_profile` pattern already used in [server/services/impact_metrics/user_metrics.go:110](../../server/services/impact_metrics/user_metrics.go#L110).
- [ ] Wire `UserScreen` (Phase 2) to pass `targetUserId: widget.targetUserId` into all four detail screens; remove the inert `onTap: null` placeholders from Phase 2. Hero + ticker taps now open the target-scoped detail screens.
- [ ] **Privacy invariant**: when `targetUserId` is set, the response and any logs must never name a non-shared community or reveal individually identifiable records from one. Validate with a per-screen leakage test mirroring Phase 1's.
- [ ] **Empty-shared case**: when `sharedCommunities` is empty, the detail screen still renders the chart and total (target's full numbers); the per-record list shows only anonymized stubs (capped + truncated). Matches the author's direction: "show the metrics and charts on the detailed page but no specifics."
- [ ] Tests:
  - Aggregate consistency: detail total equals the ticker total for the same dimension.
  - Visible row rendered with full detail when its community is in `shared_communities`.
  - Non-shared row rendered as an anonymized stub with no identifying fields in the wire response.
  - Empty-shared profile: chart + total render, per-record list contains only stubs.
  - Leakage test per detail screen, paralleling Phase 1.

**Validates:** the user profile drill-down lands you at the right aggregate numbers, surfaces the records you're authorized to see, and anonymizes the rest without compromising totals.

### Phase 4: Server postcards and viewer-facing nudges

**Goal:** Ship the three deterministic postcards from the mock.

**Tasks:**
- [ ] In the `ProfileService` handler, after computing shared communities, run three small queries:
  - **Gear card:** pick one `Gear` owned by target, joined to any shared community via `CommunityGear`, with no `Transfer` currently in `ACTIVE` state. Order by loan-count desc, take first. Build `BriefCTARow{ headline: "Borrow his {gear_name}", atmosphere_line: "Already lent N times", context_id: gear_id, thumbnail_media_id: gear.media_ids[0] }`.
  - **Experience card:** earliest upcoming `Experience` owned by target inside any shared community. `BriefCTARow{ headline: "Join {name}", atmosphere_line: "He's led this N times" or "N going so far", context_id: experience_id }`.
  - **Help card:** count of `Request` rows fulfilled by target (in `confirmed_helper_ids`) within shared communities. `BriefCTARow{ headline: "Ask him for a hand", atmosphere_line: "Offered help N times across your crews", context_id: "" (no entity target — link to a help-request creation flow scoped to those communities)}`.
- [ ] Each card is emitted only when its underlying data exists; if all three are empty, return an empty `cta_rows` and the client hides the section.
- [ ] Tests for each card's fire-and-no-fire cases. Also test that **no** card is emitted referencing data outside the shared-communities set — the privacy boundary holds on postcards too.
- [ ] Client: pass the postcard kind through `UserProfileLeverDispatcher` so taps route correctly.
- [ ] Update the `WorkshopSectionHeader` copy to "Three ways to lean on {first_name}" (new ARB string `userProfilePostcardSectionHeader(name)`).

**Validates:** visual parity with [profile-refined.html](../cowork/App%20Design/profile-refined.html).

### Phase 5: Polish, CI gates, and documentation

**Goal:** Ship-readiness.

**Tasks:**
- [ ] Patrol test covering "open user from `manage_members_screen` → see new layout with shared communities". Patrol tests live in `app/patrol_test/`.
- [ ] **Delete `UserMetricsScreen` and its dead dependencies** (grep-confirmed: no other consumers):
  - `app/lib/presentation/screens/users/user_metrics_screen.dart`
  - `app/lib/presentation/viewmodels/user_impact_view_model.dart` (and its `.freezed.dart` generated sibling)
  - The `userImpactToContentData(...)` adapter in `app/lib/presentation/models/impact_metrics_content_adapters.dart` (if nothing else uses it after removal — re-grep)
  - Any imports of the above across the app
  - Any ARB keys that exclusively backed `UserMetricsScreen`
  - Run `npm run lint:dart:i18n` and `flutter analyze` to confirm no dangling references remain.
  - Use `git mv` is **not** appropriate here — these files are being deleted, not moved. Use `git rm`.
- [ ] Server logging review — confirm every log site emits the standard fields ([server/observability.md §Standard Field Names](../server/observability.md#standard-field-names)):
  - `request_id` (HTTP correlation — injected automatically by `LoggerWithContext`; do NOT reuse this key for entity IDs)
  - `user_id` (viewer — injected automatically from auth)
  - `target_user_id` (the user being viewed — mirrors the `target_request_id` convention)
  - `operation`, `shared_community_count`, `other_community_count`, `duration_ms`
  - **Never** log non-shared community IDs. **Never** use `log.Printf`/`fmt.Printf` (CLAUDE.md rule). Emails/tokens never appear in this flow, but if they do, mask via `logging.MaskEmail` / `logging.MaskToken`.
- [ ] **Run the full CI gate matrix before declaring done:**
  - `npm run build` — runs `golangci-lint` (gofumpt, godot, revive — including `revive.exported` and `revive.package-comments` at severity `error`) + `go build ./...` ([CLAUDE.md §Go Server Build Verification](../../CLAUDE.md))
  - `go build -tags=integration ./...` — `impact_metrics.Calculator` is consumed by integration tests, so signature changes need the integration-tag build to pass too ([CLAUDE.md](../../CLAUDE.md))
  - `npm run lint:go` — bundles `scripts/check_codeinternal.js` (no bare `connect.CodeInternal`), `server/cmd/check-goroutines` (no bare `go func()`), `scripts/check_active_community_gate.js` (active-community check Rule D)
  - `npm run lint:ephemeral-comments` — fails on `Phase 3a`, `pre-refactor`, etc. in Go/Dart comments (CLAUDE.md)
  - `npm run lint:dart:i18n` — catches hardcoded strings outside ARB
  - `npm run lint:dart:a11y` — catches `GestureDetector`/`InkWell`, missing `semanticsLabel` on `CachedMediaImage`, raw `showModalBottomSheet`, raw `Duration` in animations, etc. ([client/accessibility.md](../client/accessibility.md))
  - `npm run lint:dart:size` — every Dart file under `app/lib/` ≤ 1000 lines (CLAUDE.md)
  - `npm run lint:nav` — no `Navigator.push(MaterialPageRoute(...))` or `context.push()` outside `login_screen.dart` ([client/architecture.md §Navigation & Gestures](../client/architecture.md#navigation--gestures))
  - `flutter test --reporter expanded` — never `compact` (CLAUDE.md)
  - `go test ./...` (unit) and `go test -tags=integration ./...` (integration with testcontainers Postgres)
- [ ] Manual QA on iOS simulator (use the `ripls-app-logs` skill to capture logs while clicking through to a few users with varying overlap — zero shared, one shared, many shared with extras).
- [ ] Documentation:
  - Update [docs/workshop.md](../workshop.md) "Related references" to point back here
  - Add a short sibling doc `docs/users/profile.md` describing the new surface, the (viewer, target) scoping, the cache namespace + invalidation hooks, and the privacy invariants — future contributors need to know that the intersection logic and the "no non-shared community IDs anywhere" rule are non-negotiable
  - Update the server `server/services/profile/README.md` to point to that doc

**Validates:** the issue's full Acceptance checklist plus every architecture-doc convention.

## Related Issues
- [#1009 — User screen redesign](./1009-user-screen-redesign.md) — the prior plan that pulled `UserScreen` into the Impact 2.0 layout via the shared `ImpactMetricsContent` widget. This issue supersedes that direction for the viewer-facing flow. The own-profile path through `ProfileSettingsScreen` → `UserMetricsScreen` stays as #1009 left it.
- [#1579 — Workshop tab](./1579-workshop-tab.md) — established the cascade detector chain that produces the host's Workshop nudges. Not reused on the viewer profile (different perspective), but the wire shape (`BriefPayload`) is.
- [#1618 — Workshop daily brief](./1618-workshop-daily-brief.md) — defined the postcard + hero + ticker rendering layer that this issue reuses verbatim.
