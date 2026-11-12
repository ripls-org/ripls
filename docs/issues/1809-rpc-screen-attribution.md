# Issue #1809: perf+observability: thread operationName + real screen names through every executeRpc call site (#1581 F9)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1809
**Filed:** 2026-05-09 | **Last activity:** 2026-05-09
**Labels:** _none_ | **Assignees:** unassigned
**Context docs:** `docs/client/observability.md`, `docs/client/architecture.md` (Navigation & Gestures), `docs/client/testing.md`

> **Audit (2026-05-09):** Reviewed against client architecture, observability,
> and testing docs. Findings folded inline:
> - `docs/client/observability.md` already documents `operationName` /
>   `performanceMonitor` as optional `executeRpc` parameters — making it
>   required (Phase 4) is consistent with the doc's intent and the doc
>   should be updated alongside the change.
> - `_sanitizeScreenName` in `analytics_route_observer.dart:101` already
>   normalises to lowercased + `_`-joined output (path `/foo/bar` → `foo_bar`;
>   long alphanumerics → `id`). The recommended snake_case convention
>   matches the sanitiser's output exactly, so no observer changes are
>   needed beyond the test update.
> - `docs/client/architecture.md` Navigation & Gestures says all in-app
>   forward navigation must use `NavigationHelpers.pushScreen` /
>   `pushWithSlide`, enforced by `npm run lint:nav` (exempts
>   `app_router.dart`, `login_screen.dart`, `home_screen.dart`). Phase 3's
>   approach (require a `routeName` parameter on those helpers) is the
>   right enforcement boundary.
> - `docs/client/testing.md` ViewModel-first style does not affect this
>   work — there are no ViewModel-layer changes; service-level changes
>   are mechanical and the only behavioural test is the existing
>   `analytics_route_observer_test.dart`.
> - **Side discovery, not in scope:** `register_screen.dart:620–631`
>   pushes `PhoneAuthScreen` via raw `MaterialPageRoute`, which the
>   `lint:nav` rule's exemption list does not cover. Treat as a
>   pre-existing gap; flag in PR description but do not bundle a fix.

## Problem Statement

Client-side `RPC_AUDIT` log lines emitted by `RpcUtils.executeRpc` and the
screen-view events emitted by `AnalyticsRouteObserver` are almost entirely
unattributed. Across two captured walkthroughs (619 + 491 lines):

- Only **3 of 190** `RpcUtils.executeRpc` call sites pass `operationName:`
  (verified: `fcm_service.dart:232`, `fcm_service.dart:438`, and
  `experience/rsvp_time_methods.dart:47`). Every other call logs
  `rpc=unknown`, so the audit cannot group RPC traffic by method without
  joining server logs by `request_id`.
- Roughly half of audit lines log `screen=unknown` (fired before any route is
  pushed during login bootstrap), and a further ~38 % log
  `screen=Builder<dynamic>` because `GoRoute(builder: ...)` declarations in
  `app/lib/core/router/app_router.dart` don't set `name:`. The remaining ~12 %
  are `Dialog<…>`/`ModalBottomSheet<…>` from `showDialog` /
  `showModalBottomSheet` call sites that never pass `RouteSettings(name: ...)`.

The same fallback path also degrades Firebase Analytics screen-view
attribution and Crashlytics navigation breadcrumbs.

The fix is two parallel mechanical sweeps plus CI guards so this can't
regress: thread `operationName` through every `executeRpc` call site, and
name every navigator destination (GoRoutes, the `NavigationHelpers` push
helpers, and dialog/modal call sites). Once the sweep lands, make
`operationName` non-optional so future call sites can't omit it.

## Context from Comments

The only comment is a maintainer `@claude` ping requesting the plan; there
is no further discussion. The body itself is detailed and prescriptive,
including the **Fix shape** breakdown into Part A (operationName) and
Part B (route names) and a **Test plan** that explicitly asks for two CI
guards (executeRpc operationName presence; GoRoute name presence) and an
observer test asserting `settings.name` wins over runtime type. Treat
those as soft acceptance criteria from the issue author rather than
optional add-ons.

The issue body says **~30 GoRoutes** but the current
`app_router.dart` only declares **11** — the count appears to include
imperative pushes through `NavigationHelpers.pushWithSlide` /
`pushScreen` (which surface as anonymous `PageRouteBuilder` routes) and
`showDialog` / `showModalBottomSheet` call sites. The plan therefore
scopes Part B to all three navigation mechanisms, not just GoRoutes.

## Potential Solutions

### Option A: Sweep call sites, leave `operationName` optional, ratchet via CI script

- **Approach:** Add `operationName:` to every `RpcUtils.executeRpc` call site
  in `app/lib/services/`. Add `name:` to every `GoRoute` in
  `app_router.dart` and a `routeName:` parameter to
  `NavigationHelpers.pushWithSlide` / `pushScreen` that flows through to
  `RouteSettings(name: ...)`. Update `showDialog` / `showModalBottomSheet`
  call sites to pass `routeSettings: RouteSettings(name: ...)`. Add two
  Node.js check scripts (`scripts/check_rpc_operation_names.js`,
  `scripts/check_route_names.js`) wired into `npm run lint:dart` and the
  Flutter tests workflow. Update the `AnalyticsRouteObserver` test to
  cover the `settings.name`-wins case.
- **Pros:** No public-API signature change to `executeRpc`; the migration is
  reviewable as discrete service-by-service commits; Node regex checks are
  consistent with the existing `scripts/check_*.js` style.
- **Cons:** A reviewer who ignores the regex error can still merge an
  un-tagged call site if the lint job is skipped or the regex misses an
  exotic call shape (e.g. `RpcUtils.executeRpc<T>(...)` formatted across
  many lines).
- **Effort:** medium (~190 mechanical edits + 2 CI scripts + 1 helper
  signature change + a handful of dialog/modal edits).

### Option B: Make `operationName` a required positional parameter

- **Approach:** Same as Option A, but after the sweep is complete change
  `executeRpc<T>(rpcCall, errorHandler, {…, String? operationName, …})` to
  `executeRpc<T>(operationName, rpcCall, errorHandler, {…})` (or keep it
  named but `required`). The Dart compiler then enforces presence; no
  regex needed.
- **Pros:** Strongest guarantee — impossible to merge a missing
  `operationName`. No ongoing maintenance cost for a custom lint script.
- **Cons:** Single huge diff that must land atomically (every call site
  has to change in the same PR). Conflicts loudly with anything in flight
  that touches services. The Node check is still useful as a pre-flight
  during the sweep, so this is really Option A **plus** a follow-up
  signature change.
- **Effort:** large for the atomic flip; medium if treated as a follow-up
  after Option A lands.

### Option C: Auto-derive `operationName` from the Connect client method

- **Approach:** Wrap each Connect client (e.g. `CommunityServiceClient`) in
  a tiny adapter that captures the method name as it dispatches and
  threads it into `executeRpc` automatically (e.g. via `Zone` or a
  closure that introspects the call). No call-site edits needed.
- **Pros:** Zero per-call-site work; impossible to forget by construction.
- **Cons:** Connect-Go's Dart bindings don't expose method names through
  any reflective hook, so this requires hand-rolling a wrapper for every
  Connect client (≈15 services × N methods each). That is strictly more
  work than Option A and introduces a layer that obscures what is going
  on at the call site. Method-level attribution still requires a name —
  the only question is *where* the name lives.
- **Effort:** large, with worse readability ergonomics than Option A.

## Recommendation

**Option A now, Option B as an immediate follow-up after the sweep
lands.** Option A is a tractable, reviewable migration (one PR per
service grouping if desired) that closes the audit gap immediately. Once
zero call sites are missing `operationName:`, flipping the parameter to
`required` is a one-line `executeRpc` change plus a no-op rename
everywhere — the heavy lifting is done.

For Part B, name every `GoRoute` (using the path's last meaningful
segment, e.g. `name: 'login'` for `/login`, `name: 'gear_detail'` for
`/gear/:id`), thread a `routeName` parameter through
`NavigationHelpers.push*` so PageRouteBuilders carry `RouteSettings(name:
...)`, and require `routeSettings:` on every `showDialog` /
`showModalBottomSheet` call site. The CI guard for `name:` presence on
GoRoutes is the easy 80 %; the harder Navigation-helper / dialog
checks are best handled by mandating that the helpers' new `routeName`
parameter be required (Dart-level enforcement, no regex).

## Open Questions

- **Should `operationName` end up required-positional, required-named, or
  remain optional with a CI script?** — Recommendation: **required-named**
  in the follow-up. Keeps the existing call-site shape (named params for
  readability) and lets the Dart compiler enforce presence. No regex
  needed long-term, but ship the regex first so the sweep PR doesn't
  collide with in-flight work.
- **What casing convention for screen names?** — Recommendation:
  **snake_case**, matching what `_sanitizeScreenName` already produces
  for path-derived names (`/settings/communities/restore` →
  `settings_communities_restore`). Document this once in
  `app_router.dart` so reviewers can spot omissions ("name should match
  the final path segment, lowercased and underscored").
- **Parameterized routes — `name: 'gear'` or `name: 'gear_detail'`?** —
  Recommendation: **conceptual name, not path token** — `gear_detail`,
  `request_detail`, `experience_detail`, `restore_community`. The path
  segment is `:id`-bearing and uninformative; the audit's
  `screen=gear_detail` reads better than `screen=gear`.
- **Should imperative `Navigator.push` and `MaterialPageRoute` call sites
  be in scope, or only GoRouter?** — Recommendation: **in scope, narrowly**.
  Update `NavigationHelpers.pushWithSlide` / `pushScreen` to require a
  `routeName` parameter (the two existing direct `MaterialPageRoute`
  call sites in `login_screen.dart:336` and `register_screen.dart:622`
  also get `RouteSettings(name: ...)`). This is bounded — `npm run
  lint:nav` already restricts the use of these primitives.
- **Dialog / modal naming scope.** — Recommendation: include the
  ~58 `showDialog` / `showModalBottomSheet` call sites in this sweep but
  ship them in the same PR as the route renames so the audit's
  Dialog-class fallback also clears. Use the conceptual sheet name
  (`gear_action_sheet`, `confirm_leave_community_dialog`).
- **Should we add a runtime-type screen-name override for known
  builder-only routes via the observer, instead of editing every call
  site?** — Recommendation: **no**. The observer-side override would
  hardcode a route → name map duplicated from the router and rot
  silently when a screen is renamed. The "name on the
  destination" approach colocates intent with the source.
- **Where should the new CI scripts run?** — Recommendation: add to
  `npm run lint:dart` (alongside the other Dart-side checks) and a new
  step in `.github/workflows/test_flutter.yaml`, mirroring how
  `lint:dart:size`, `lint:dart:a11y`, etc. are wired.

## Implementation Plan

### Phase 1: CI guards + observer test

**Goal:** Land the regression-prevention scaffolding before changing call
sites, so the sweep can be reviewed against a green-when-complete signal.
**Tasks:**
- [ ] Add `scripts/check_rpc_operation_names.js`. Walk
      `app/lib/services/**/*.dart`. Match
      `RpcUtils.executeRpc(` and verify the matching argument list
      (parenthesis-balanced) contains `operationName:`. Allow
      a documented exemption file `scripts/rpc_operation_names_allowlist.txt`
      that *starts populated with the existing 187 missing call sites*,
      so Phase 1 can ship without breaking CI. The allowlist may shrink
      but never grow on `main` (mirror the pattern in
      `.github/workflows/test_flutter.yaml`'s a11y allowlist verifier).
- [ ] Add `scripts/check_route_names.js`. Walk
      `app/lib/**/*.dart`. Fail when a `GoRoute(` opening is found
      without a `name:` argument inside its parenthesised body. Same
      shrink-only allowlist pattern (`scripts/route_names_allowlist.txt`)
      seeded with the 11 existing GoRoutes.
- [ ] Wire both scripts into `package.json`
      (`lint:dart:rpc-operation-names`, `lint:dart:route-names`) and
      into `npm run lint`. Add matching steps + allowlist-shrink
      verifiers to `.github/workflows/test_flutter.yaml`.
- [ ] Update `app/test/core/observability/analytics_route_observer_test.dart`
      to assert `settings.name` is preferred over `route.runtimeType`,
      and to assert sanitisation of an `id`-bearing name
      (`gear_detail/abc123` → `gear_detail_id`). Use the existing
      `MockObservabilityService` testing pattern documented in
      `docs/client/observability.md` (Testing section) so the assertion
      checks `provider.logScreenView` was called with the expected
      `screenName`/`screenClass`.
**Validates:** CI fails the moment a service file or router gains a new
un-named call site, even before Phase 2 lands. The observer test pins the
contract the rest of the work depends on.

### Phase 2: Thread `operationName` through every `executeRpc` call site

**Goal:** Drive the `check_rpc_operation_names.js` allowlist to zero.
**Tasks:**
- [ ] Sweep `app/lib/services/community_service.dart`,
      `request_service.dart`, `gear_service.dart`,
      `experience/{fetch,gen,lifecycle,needs,rsvp_time}_methods.dart`,
      `experience_service.dart`, `chat_service.dart`,
      `feed_service.dart`, `portfolio_service.dart`,
      `transfer_service.dart`, `user_service.dart`,
      `media_service.dart`, `search_service.dart`,
      `location_service.dart`, `mapbox_location_service.dart`,
      `feedback_service.dart`, `leaderboard_service.dart`,
      `workshop_service.dart`, `impact_service.dart`,
      `esm_service.dart`. For each call site, set `operationName:` to
      the Connect method name in PascalCase (e.g.
      `_client.listCommunities(...)` → `'ListCommunities'`). Remove
      each file from `scripts/rpc_operation_names_allowlist.txt` as it
      lands.
- [ ] Where `performanceMonitor:` is currently absent but the service
      already injects one, thread it through too — this is the
      "belt-and-suspenders" the issue calls out and lets the
      `rpc_<operationName>` performance traces actually fire.
- [ ] Run `npm run lint:dart:rpc-operation-names`. Allowlist must be
      empty.
- [ ] Verify by running `cd app && flutter test --reporter expanded`
      (no behaviour change expected, but a small set of service tests
      assert log output and may need updating).
**Validates:** Audit lines emit `rpc=ListCommunities` etc. instead of
`rpc=unknown`. The allowlist file can be deleted in this same PR or in
Phase 4 cleanup.

### Phase 3: Name every navigator destination

**Goal:** Drive the `check_route_names.js` allowlist to zero and
eliminate `screen=Builder<dynamic>` / `screen=Dialog<…>` /
`screen=ModalBottomSheet<…>` from the audit.
**Tasks:**
- [ ] Add `name:` to every `GoRoute` in
      `app/lib/core/router/app_router.dart` using the conceptual-name
      convention from Open Questions (e.g. `name: 'gear_detail'` for
      `/gear/:id`).
- [ ] Add a required `routeName` parameter to
      `NavigationHelpers.pushWithSlide` and
      `NavigationHelpers.pushScreen` in
      `app/lib/core/utils/navigation_helpers.dart`. Pass the value
      through as `RouteSettings(name: routeName)` on the
      `PageRouteBuilder`. Update every caller (governance, profile,
      settings, impact_metrics, daily, workshop, etc. — see grep in
      research notes) to pass an explicit name. The Dart compiler
      enforces presence — no regex needed.
- [ ] Update `NavigationHelpers.pushToItemScreen` to derive a name
      from `itemType` (`gear` → `gear_detail`, etc.) and forward it.
- [ ] Add `RouteSettings(name: ...)` to the two direct
      `MaterialPageRoute` call sites in
      `app/lib/presentation/screens/auth/login_screen.dart:336` and
      `app/lib/presentation/screens/auth/register_screen.dart:622`.
- [ ] Update every `showDialog` / `showModalBottomSheet` call site
      (~58) to pass `routeSettings: RouteSettings(name: ...)`. Use
      the dialog/sheet's conceptual name (e.g.
      `confirm_leave_community_dialog`,
      `gear_action_sheet`).
- [ ] Drain `scripts/route_names_allowlist.txt` and delete it.
**Validates:** A re-run of the audit produces meaningful
`screen=` values and the route-names CI check passes with no
allowlist.

### Phase 4: Make `operationName` required and clean up

**Goal:** Lock in the invariant in the type system and remove the
transitional CI scaffolding.
**Tasks:**
- [ ] Change the signature of `RpcUtils.executeRpc` so
      `operationName` is `required String` (named, not optional).
- [ ] Delete `scripts/check_rpc_operation_names.js` and
      `scripts/rpc_operation_names_allowlist.txt`; remove the
      `lint:dart:rpc-operation-names` script and workflow step. The
      type system now enforces what the script enforced.
- [ ] If `route_names_allowlist.txt` is empty, delete it too (the
      `check_route_names.js` script remains — new `GoRoute`s outside
      `app_router.dart` would still be a hazard).
- [ ] Update `docs/client/observability.md` (Performance Monitoring →
      Manual Traces section currently calls `operationName` optional;
      change to required) and add a one-paragraph note to
      `app/lib/core/router/app_router.dart` top-of-file comment: every
      navigator destination must have
      `name` / `routeName` / `routeSettings.name`, and every
      `executeRpc` call must pass `operationName:`. Cite #1581 and #1809.
**Validates:** A merge that omits `operationName:` no longer compiles;
the audit's `rpc=` and `screen=` columns are usable end-to-end.

## Related Issues

- #1581 — RPC traffic audit (parent epic; this is finding F9).
- `docs/ai/rpc_audit/findings-2026-05-09-pass2.md` — captured the
  symptom and motivated this fix.
