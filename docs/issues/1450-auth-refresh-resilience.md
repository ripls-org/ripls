# Issue #1450: Phone re-login forced despite 90-day refresh token (PR #787 follow-up: persistence race, cold-start gap, unbounded refresh storm)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1450
**Filed:** 2026-04-27 | **Last activity:** 2026-04-27
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

After PR #787 added 90-day refresh tokens, users were still being kicked back to the phone login screen long before the 90-day window. The issue documents three distinct client-side bugs in `app/lib/services/auth_state.dart` and `app/lib/core/utils/rpc_utils.dart` that compound to cause forced re-logins:

1. **Persistence race** — `refreshAccessToken` mutates in-memory state *before* awaiting the SharedPreferences and FlutterSecureStorage writes. If the OS kills the process in the ~20–50 ms window between state mutation and disk commit, the new tokens are lost while older flushed values (`server_url`) survive — so the device boots with no `access_token` even though `refresh_token` was supposed to be persisted.
2. **Cold-start recovery gap** — `loadAuthState` (auth_state.dart:248-251) only schedules a proactive refresh when `accessToken != null`. If `access_token` is missing on cold start but a valid `refresh_token` exists in secure storage, the app silently falls through to unauthenticated and routes the user to login.
3. **Unbounded refresh storm** — `RpcUtils.executeRpc` (rpc_utils.dart:112-129) retries any 401 once after refresh, but `refreshAccessToken` itself only coalesces *concurrent* refreshes via `_refreshCompleter`; it does not rate-limit *sequential* refreshes. With a server bug producing repeated 401s after refresh, the client looped at ~150–200 ms per cycle, generating 130+ refresh calls in 18 s.

PR #1455 (merged 2026-04-27) fixed the server-side root cause of the storm seen in production: `server/services/login/refresh.go:76` was building post-refresh access tokens without `phoneNumber`, producing JWTs that the validator rejected. With #1455 deployed the immediate user-blocking incident clears, but the three client-side weaknesses described above are still real defense-in-depth gaps.

## Context from Comments

- **briangbrown (2026-04-27 04:47)** narrowed the repro: a clean force-stop after a stable login does **not** reproduce the missing-token state. The kill must land *during* a refresh write. So Bug 1 is real but the per-kill window is narrow; the storm widened it dramatically because 130 rotations × narrow window each gave high cumulative probability that one rotation never flushed.
- **briangbrown (2026-04-27 16:45)** identified the server-side root cause of the storm and the post-refresh `unauthenticated` retry: `refresh.go:76` was missing the `phoneNumber` argument, producing tokens that failed `(email == "" && phoneNumber == "")` validation in `server/auth/tokens.go:178`. Hotfix shipped as PR #1455. The comment explicitly states the client-side mitigations (rate-limit, persist-before-state, cold-start refresh probe) "are still worth shipping for defense in depth, but with this server fix the user-blocking incident clears immediately."
- **briangbrown (2026-04-27 21:52)** asks Claude to confirm post-#1455 whether the client-side fixes are still needed and, if so, to write a plan. They are: #1455 closes one specific server bug but does nothing for future malformed-token scenarios, persistence races on real device kills, or the cold-start gap (which is independent of any storm). All three remain.

No disagreements among comments; the issue author refined the diagnosis as evidence accumulated. The recommended order in the issue body — Bug 2 first (smallest patch, prevents the most common repro), then Bug 3+1 rate-limit, then Bug 1 persist-before-state — is the order I'd preserve.

## Potential Solutions

### Option A: Ship all three client-side fixes in one PR

- **Approach:** A single Flutter PR that:
  1. **Bug 2 (cold-start refresh probe):** In `loadAuthState`, when `access_token` is null but `_secureStorage.read(_refreshTokenKey)` returns a non-empty value, call `refreshAccessToken()` and only fall through to unauthenticated if it returns false.
  2. **Bug 3+1 (rate-limit):** Track `_lastRefreshAt` in `AuthStateNotifier` and short-circuit `refreshAccessToken` to return `true` if a successful refresh completed within the last 10 s. Cap retry attempts in `RpcUtils.executeRpc` at exactly one (already de facto true; make it explicit and guarded).
  3. **Bug 1 (persist-before-state):** Reorder `refreshAccessToken` to `await _prefs.setString` and `_secureStorage.write` *before* mutating `state = state.copyWith(accessToken: ...)`. Same reorder in `setAuthState`.
- **Pros:** All three protections land together; testers get one round-trip; changes are tightly coupled (the reorder in #3 invalidates some assumptions about #2 timing).
- **Cons:** Larger blast radius — three behavior changes interacting at once, harder to bisect if a regression lands; review surface is bigger.
- **Effort:** Medium.

### Option B: Three small phased PRs (Bug 2 → Bug 3+1 → Bug 1)

- **Approach:** Same end state as Option A, but split into three sequential PRs in the issue's recommended order. Each phase is a shippable increment with its own tests.
- **Pros:** Each PR is small, easy to review, easy to revert. Bug 2 alone would have prevented the production repro and is the lowest-risk patch — shipping it first lets us validate the fix quickly. Each phase teaches us whether the next is still load-bearing.
- **Cons:** Three round-trips for review and CI; some test scaffolding (Riverpod ProviderContainer + mocked SharedPreferences/FlutterSecureStorage for AuthStateNotifier) gets duplicated across PRs unless we extract a shared test helper in phase 1.
- **Effort:** Medium total; small per phase.

### Option C: Bug 2 only, defer the rest

- **Approach:** Ship just the cold-start refresh probe. Wait to see if the persistence race or sequential-refresh storm recur in the wild before adding mitigations.
- **Pros:** Minimum change to fix the most common observed repro path.
- **Cons:** Leaves two known defense-in-depth gaps open. PR #787 added a 90-day refresh token expecting near-zero re-logins; the persistence race and unbounded storm both undermine that contract. The issue author explicitly asks for all three.
- **Effort:** Small.

## Recommendation

**Option B — three phased PRs in the recommended order.** Rationale:

- The bugs are **independent in causality** (a cold-start probe doesn't help with a force-kill mid-write; a rate-limiter doesn't help if cold start never tries refresh; persist-before-state doesn't help if cold start never reads back the persisted state). Each phase delivers user-visible protection on its own.
- The bugs have **different risk profiles**. Bug 2's cold-start probe is purely additive — adds an `await` on a previously-skipped path. Bug 3's rate-limiter is also additive (short-circuits return `true`) but has a tunable window we may need to revise. Bug 1's persist-before-state changes ordering on a hot path that touches every refresh; it's the highest-risk change and should land last after we've validated test coverage with the earlier phases.
- This issue is a **post-incident hardening exercise** with #1455 already plugging the immediate hole; we have time to sequence the changes carefully rather than batch them.

Phase 1 will also extract the AuthStateNotifier test helper (mocked `SharedPreferencesAsync`, `FlutterSecureStorage`, and `AuthService`), which the existing `app/test/services/auth_state_test.dart` doesn't currently cover. That helper is the gating dependency for all three phases — without it, none of the three bugs are testable beyond `AuthStateData`'s plain data class.

## Open Questions

- **What rate-limit window should we pick for Bug 3?** — Recommendation: **10 seconds**, controlled by a single `static const _refreshMinInterval = Duration(seconds: 10)`. The proactive refresh fires 5 minutes before expiry, the 401-retry path is single-shot per RPC, and a typical authenticated RPC takes <2 s end-to-end. 10 s is comfortably above any legitimate back-to-back refresh trigger, well below any user-perceptible session interruption, and an order of magnitude tighter than the 150–200 ms storm cadence. If a server bug produces a stream of 401s on a freshly-refreshed token, the user sees one logout after one retry — not 130 calls in 18 s.
- **Should Bug 2's cold-start refresh be awaited or fire-and-forget?** — Recommendation: **awaited**. `loadAuthState` already runs during app initialization and the splash/loading state is already presented (`isLoading: true` is the default). Awaiting the refresh adds at most one network round-trip to startup for users with a valid refresh token but no access token — far better than routing them to login. If the refresh fails (network down, refresh token revoked) we fall through to unauthenticated as before. The router's existing `authState.isLoading` guard (`docs/client/startup.md` "Authentication-Based Routing") already covers the "still loading" rendering case, so adding an awaited refresh fits the documented startup flow rather than introducing a new gating step.
- **For Bug 1, is there a risk that delaying state mutation until after disk writes causes concurrent RPCs to use the old (about-to-expire) access token?** — Recommendation: **acceptable risk, document in code**. The `_refreshCompleter` already coalesces concurrent refreshes; concurrent reads of `state.accessToken` between the disk write start and state mutation can use the old token at most for the duration of the disk write (~20–50 ms). If the old token is still within its 5-minute pre-expiry buffer (which it almost always is, since proactive refresh fires that early), those concurrent RPCs succeed. If they don't, they hit the 401 retry path which now finds the new token. Net: no worse than today. Add a one-line comment near the persist-before-state ordering explaining the invariant.
- **Should we also rate-limit refreshes on the server?** — Recommendation: **out of scope for this issue**. Server-side rate limiting on `LoginService.RefreshToken` is a sensible follow-up but belongs in a separate issue; this plan stays client-side per the issue's framing.
- **Do we need to handle the case in Bug 2 where the user record (`user` JSON in SharedPreferences) is also null but `refresh_token` is present?** — Recommendation: **fall through to unauthenticated with a logged warning; do not add a server-side user fetch**. The combination is reachable only if some external mechanism wiped `user` while leaving `refresh_token` intact (e.g. a developer hand-editing storage). Adding a `UserService.getUser` call would require parsing the user_id out of the JWT (which the codebase doesn't currently do) and complicates the refresh path. The simpler guard — log a warning, reset state to unauthenticated, let the user log back in — is sufficient for an edge case that doesn't appear in the production repro.
- **Should `_secureStorage` and `_prefs` be injectable, or do we mock them globally?** — Recommendation: **make them injectable on `AuthStateNotifier` via test-only setters or by adding two new Riverpod providers (`secureStorageProvider`, `sharedPreferencesAsyncProvider`) and overriding them in tests.** Today `auth_state.dart:55` and `:58` declare them as `late final` and `static const` respectively, so there's no test seam. Adding Riverpod providers is the architecturally consistent choice — it matches the `cacheManagerProvider`/`authServiceProvider` pattern already used by this Notifier and avoids touching production call sites. The two new providers are trivially small and live next to the existing ones in `app/lib/services/providers.dart`.

### Architectural alignment notes (post-audit)

- `AuthStateNotifier` is a `Notifier<AuthStateData>` that already follows `docs/client/architecture.md`'s Riverpod 3.0 Notifier pattern. Auth state is not a cached domain entity, so it correctly does **not** sit behind a Repository — `docs/client/architecture.md` reserves the repository pattern for "User, Gear, Loan, Community" data, which is distinct from session/auth state. No change to the architectural shape; the three fixes are localized to this Notifier and `RpcUtils`.
- `AuthStateData` is currently a hand-rolled immutable class with `copyWith`, not Freezed. The plan does not introduce new state fields requiring Freezed migration; we add a private `_lastSuccessfulRefreshAt` field to the notifier itself (not the public state). Migrating `AuthStateData` to Freezed is a separate refactor outside this issue's scope.
- The proactive refresh timer (`_scheduleProactiveRefresh`, auth_state.dart:144) is only rescheduled inside the success path of `refreshAccessToken`. With the new rate-limit short-circuit, a no-op return-true does not reschedule the timer — but the timer was already set when the original refresh ran 5 minutes earlier, so it stays valid. This is correct behavior; document it in the rate-limit comment.
- The app-resume refresh path (`_checkAndRefreshOnResume`, auth_state.dart:132) calls `refreshAccessToken()` if the token is within 5 minutes of expiry. With a 10 s rate-limit, a brief background-then-resume cycle that lands inside the rate-limit window will short-circuit — that's the intended behavior, not a regression.

## Implementation Plan

### Phase 1: Cold-start refresh probe + AuthStateNotifier test scaffolding

**Goal:** Eliminate the cold-start gap (Bug 2) and lay the testing foundation for phases 2 and 3.

**Tasks:**
- [ ] **Add injection seams for storage.** In `app/lib/services/providers.dart`, add two new providers next to the existing `authServiceProvider` / `cacheManagerProvider`:
  - `final secureStorageProvider = Provider<FlutterSecureStorage>((ref) => const FlutterSecureStorage());`
  - `final sharedPreferencesAsyncProvider = Provider<SharedPreferencesAsync>((ref) => SharedPreferencesAsync());`
  In `app/lib/services/auth_state.dart`, replace the hardcoded `late final SharedPreferencesAsync _prefs = SharedPreferencesAsync();` (line 55) and `static const _secureStorage = FlutterSecureStorage();` (line 58) with `late final` getters that read from the new providers via `ref.read(...)`. Production behavior is unchanged; tests can override either provider.
- [ ] **Test scaffolding** in `app/test/services/auth_state_notifier_test.dart`:
  - `@GenerateMocks([AuthService, FlutterSecureStorage, CacheManager])` (and any others as needed).
  - `ProviderContainer` with overrides for `authServiceProvider`, `secureStorageProvider`, `cacheManagerProvider`. For `SharedPreferencesAsync`, use `SharedPreferencesAsyncPlatform.instance = InMemorySharedPreferencesAsync.empty()` from `shared_preferences_platform_interface` test utilities, OR an override on `sharedPreferencesAsyncProvider` if simpler.
  - Re-export a `setUp`/`tearDown` helper so phases 2 and 3 reuse the same scaffolding without duplication.
- [ ] **Cold-start refresh probe** in `app/lib/services/auth_state.dart` `loadAuthState`:
  - Where `accessToken == null` is currently a silent path through to `state = AuthStateData.empty`-ish, instead attempt `final stored = await _secureStorage.read(key: _refreshTokenKey);` before deciding.
  - If `stored != null && stored.isNotEmpty`, call `await refreshAccessToken();`. If it returns true, re-read `access_token` from prefs (or use `state.accessToken` since `refreshAccessToken` updates state too). Continue with the loaded `userJson` from earlier in the method.
  - If the user JSON is missing post-refresh (edge case — `setAuthState` always writes them together), log a warning at WARNING level, leave state unauthenticated, and let the router redirect to login. Do NOT add a server fetch for the user record.
  - If refresh returns false, fall through to unauthenticated as today.
- [ ] **Tests:**
  - `loadAuthState recovers when access_token is missing but refresh_token is present` — asserts `AuthService.refreshToken` is called exactly once, prefs receives the new `access_token`, and the resulting state is authenticated with the previously-persisted user record.
  - `loadAuthState falls through to unauthenticated when no refresh_token is present` — regression guard for current behavior.
  - `loadAuthState falls through when refresh_token is present but refresh fails` — error path; mock returns `success: false`.
  - `loadAuthState falls through with warning when refresh succeeds but user JSON is missing` — guard-rail edge case.
- [ ] Run `cd app && flutter test --reporter github test/services/auth_state_notifier_test.dart` and confirm green. Run `npm run lint:dart:size` to confirm `auth_state.dart` is still under 1 000 lines.

**Validates:** The most common repro (cold start with no `access_token` but valid `refresh_token`) no longer forces a re-login. Test scaffolding exists for AuthStateNotifier and the storage dependencies have a clean injection seam for future tests.

### Phase 2: Sequential-refresh rate limiter

**Goal:** Bound the refresh storm class of failures (Bug 3 + Bug 1's storm-mitigation slice).

**Tasks:**
- [ ] In `app/lib/services/auth_state.dart`, add `DateTime? _lastSuccessfulRefreshAt;` and `static const _refreshMinInterval = Duration(seconds: 10);` to `AuthStateNotifier`.
- [ ] At the top of `refreshAccessToken`, after the `_refreshCompleter` check, add: `if (_lastSuccessfulRefreshAt != null && DateTime.now().difference(_lastSuccessfulRefreshAt!) < _refreshMinInterval) { _log.fine('🔄 Skipping refresh: succeeded ${...}ms ago'); return true; }`.
- [ ] On successful refresh (just before `_refreshCompleter!.complete(true)`), set `_lastSuccessfulRefreshAt = DateTime.now();`.
- [ ] On `logout()`, reset `_lastSuccessfulRefreshAt = null;` so the next session starts fresh.
- [ ] In `app/lib/core/utils/rpc_utils.dart` `executeRpc`, the post-refresh retry already only retries once before throwing — make this explicit by adding a one-line comment: `// Single retry. The refresh handler is rate-limited so a second 401 is fatal.` No code change needed if the structure already enforces this.
- [ ] Add tests in `auth_state_notifier_test.dart`:
  - `refreshAccessToken returns cached true within rate-limit window` — call refresh twice 1 s apart, assert `AuthService.refreshToken` is called once.
  - `refreshAccessToken refreshes again after rate-limit window` — use a fake clock or expose `_refreshMinInterval` for test override; second call after 11 s hits the service.
  - `logout resets the rate-limit timestamp` — refresh, logout, login, refresh — service is called both times.
- [ ] Run `cd app && flutter test --reporter github` and `npm run build` (Go side untouched; build-no-op confirms no regression).

**Validates:** A storm of 401s (or a regression of the kind #1455 fixed) cannot translate into more than one refresh call per 10 s window; the user sees a clean logout after the rate-limited single retry rather than a saturating loop.

### Phase 3: Persist-before-state ordering

**Goal:** Close the persistence race (Bug 1) so an OS kill mid-refresh leaves disk consistent.

**Tasks:**
- [ ] In `app/lib/services/auth_state.dart` `refreshAccessToken`, reorder:
  - `await _prefs.setString('access_token', result.accessToken!);`
  - `await _secureStorage.write(key: _refreshTokenKey, value: result.refreshToken!);`
  - `state = state.copyWith(accessToken: result.accessToken);`
  - `_scheduleProactiveRefresh(result.accessToken!);`
  - `_lastSuccessfulRefreshAt = DateTime.now();`
- [ ] In `setAuthState`, reorder so all `_prefs.setString(...)` and `_secureStorage.write(...)` calls complete before `state = AuthStateData(...)`. The cache-clearing call (`_clearUserCaches()`) stays where it is at the top.
- [ ] Add a one-line comment in `refreshAccessToken` explaining the invariant: `// Persist before mutating in-memory state so an OS kill mid-refresh leaves disk consistent.`
- [ ] Add tests:
  - `refreshAccessToken writes prefs and secure storage before mutating state` — order verified by tracking call order on the mocks (Mockito's `verifyInOrder`).
  - `setAuthState writes prefs and secure storage before mutating state` — same pattern.
- [ ] Manually verify on a Pixel emulator: log in, trigger a refresh (e.g. via a debug button or the proactive timer), force-stop the app from Android settings while the refresh is in flight (will require timing or a debug delay), reopen, confirm authenticated. Document the manual repro in the PR description since unit tests alone can't cover the OS-kill race.
- [ ] Run `cd app && flutter test --reporter github` and `npm run build`.

**Validates:** The persistence race window from Brian's narrowed repro is closed. With phases 1–3 together, the issue's recommended fixes are all shipped.

## Related Issues

- #1455 — server-side hotfix that closed the immediate user-blocking incident (malformed JWT after refresh for phone-only users). Merged. This plan handles the client-side defense-in-depth gaps that #1455 explicitly defers.
- #757 — original auth/registration thread referenced by the issue body for prior context.
- PR #787 — added 90-day refresh tokens. This plan delivers the resilience layer that #787 implicitly assumes but doesn't enforce.

---

## Audit Notes (2026-04-27)

Run via `/ripls-audit docs/issues/1450-auth-refresh-resilience.md` against `docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`, `docs/client/startup.md`, `docs/client/testing.md`. Changes applied in-place above:

1. **Test seams clarified.** Original plan said "override the `MethodChannel` directly" for `FlutterSecureStorage`, which neither matches the codebase nor produces a clean test. Replaced with a Riverpod-provider injection pattern that mirrors how `authServiceProvider` and `cacheManagerProvider` are already used inside `AuthStateNotifier`. Two new providers (`secureStorageProvider`, `sharedPreferencesAsyncProvider`) and a getter-based access pattern in the notifier — production behavior identical, tests get a clean override seam.
2. **Architectural alignment confirmed.** Added an "Architectural alignment notes" subsection to Open Questions documenting that `AuthStateNotifier` correctly remains a Notifier (not behind a Repository) per `docs/client/architecture.md` — auth/session state is not a cached domain entity, so the repository pattern doesn't apply. Also confirmed Freezed migration of `AuthStateData` is out of scope.
3. **User-record edge case downgraded.** The "fetch user via UserService if missing" recommendation has been replaced with "log warning and fall through to unauthenticated." Pulling the user record from server requires parsing the user_id from the JWT (not currently done) and adds a code path for an edge case that doesn't appear in the production repro.
4. **Resume + proactive timer interaction documented.** Verified that the rate-limit short-circuit does not break the proactive refresh timer (it was scheduled inside the original successful refresh and stays valid) and does not break app-resume refresh (a short-circuit during a brief resume cycle is intentional). Noted in the alignment subsection.
5. **Startup flow alignment.** Added a one-line note in the cold-start question's recommendation that the awaited refresh fits the documented `authState.isLoading`-gated router behavior in `docs/client/startup.md`, so no new gating step is introduced.
