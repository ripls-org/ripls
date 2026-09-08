---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Client logout teardown — AuthStateNotifier.logout() strict ordering that prevents cross-user data leakage and logout-frame races, the token-refresh interlock, reset-on-login pattern, and mutation-viewmodel guards.
  globs: [app/lib/services/auth_state.dart, app/lib/core/utils/logout_diagnostics.dart]
  triggers: [logout, teardown, auth-state, token-refresh, cross-user-leakage, cache-clear, logout-race]
  lens: [client, security]
  domain: client
freshness:
  verified_commit: "e78336400"
  verified_on: "2026-07-26"
---
# Client Logout Teardown

## Overview

Logout is handled by `AuthStateNotifier.logout()` in [auth_state.dart](../../app/lib/services/auth_state.dart). It is guarded against concurrent calls and follows a strict ordering to prevent cross-user data leakage and to avoid the logout-frame races documented below.

## Teardown Sequence

```
AuthStateNotifier.logout()
    ├─ Guard: skip if _isLoggingOut
    ├─ Mark _isLoggingOut = true; open LogoutDiagnostics window
    │
    ├─ 1. Clear in-memory auth state          → router redirects to /login
    ├─ 2. Clear persisted auth (prefs + SecureStorage)
    ├─ 3. Stop event pollers and streams
    ├─ 4. Clear all caches (CacheManager.clear + ref.invalidate(...))
    └─ 5. Clear all SharedPreferences

_isLoggingOut stays true until the next successful setAuthState() (login).
```

## Invariants

- **Auth state clears first** so the router redirects immediately and stops further UI from issuing RPCs.
- **Pollers/streams stop before caches clear** so in-flight events don't repopulate just-cleared caches.
- **Cache clearing is nuclear** — namespace-agnostic. Simpler than pattern-based clearing and impossible to forget a key.
- **`_isLoggingOut` is sticky.** Only `setAuthState()` (next login) resets it. This blocks two races: (a) the cascade where 401s trigger `logout()` → 401 → `logout()`, and (b) the token-refresh rebound described below.

## The token-refresh interlock (#2158)

`AuthStateNotifier.refreshAccessToken()` **must bail when `_isLoggingOut == true`**. Without it: an in-flight RPC fails with 401 in the ~35ms window between auth-state clear and the SecureStorage delete, `RpcUtils` calls `refreshAccessToken()`, the still-present refresh_token mints a new pair, `state.copyWith(accessToken: ...)` flips `isAuthenticated` back to `true`, the auth listener re-enters the app, and the router bounces unauth → auth within a single transition. That bounce trips the `RenderIgnorePointer was mutated in performLayout` assertion on the home IndexedStack.

The guard short-circuits the chain before SecureStorage is read. Anyone touching this method must keep the `_isLoggingOut` check at the top.

## Riverpod state — reset on login, not on logout

`logout()` does **not** reset per-user notifiers ([`communitiesProvider`](../../app/lib/services/providers/community_providers.dart), [`workshopCommunityProvider`](../../app/lib/services/providers/workshop_community_provider.dart), etc.). Two state writes in the logout frame (auth clear + the reset) caused a historical Duplicate-GlobalKey crash.

Cleanup happens at the login entry point — [`_loadCommunities()`](../../app/lib/main.dart) wipes per-user notifiers before fetching the new user's data. This is safe because `/home` hasn't mounted yet.

### Adding new per-user notifiers

1. Add a synchronous `clear*()` / `reset()` that emits the default state.
2. Call it inside `_loadCommunities()` before `setLoading(true)`.
3. Do **not** add it to `logout()`.

## Adding new teardown steps

For new background services that hold user-specific state (streams, timers, subscriptions):

1. Add teardown in `logout()` between step 2 (clear persisted tokens) and step 4 (clear caches).
2. Give the service a synchronous `reset()` / `dispose()`.

## Guarding mutation viewmodels

Notifiers that issue RPCs must guard the logout window in two places — pre-await and post-await:

```dart
if (!ref.mounted || ref.read(authStateProvider).user == null) return;
final response = await _repo.someMutation(...);
if (!ref.mounted || ref.read(authStateProvider).user == null) return;
state = state.copyWith(...);
```

Notifiers whose `build()` runs on an invalidation cascade (e.g. `FeedNotifier` after `_clearUserCaches()` invalidates `feedProvider`) need the same guard at the top of `initialize()` / `refresh()`.

Use `user == null`, not `accessToken == null` — during a refresh, `accessToken` can be transiently non-null with no live session.

## Diagnostics

[`logout_diagnostics.dart`](../../app/lib/core/utils/logout_diagnostics.dart) maintains a 300-entry ring buffer of timestamped events from the logout window. The teardown steps, cache-invalidation notifiers, repository callbacks, and needs-viewmodel mutations all emit traces. `FlutterError.onError` and `PlatformDispatcher.onError` auto-dump the buffer when a crash matches the #2158 signature or lands inside the logout window — search logs for `[LogoutDiag]`.

When adding code that runs during logout, call `LogoutDiagnostics.trace('LABEL', 'details')`. The buffer is bounded, so over-instrumenting is safe.

## Related

- [startup.md](startup.md) — initialization sequence
- [auth_state.dart](../../app/lib/services/auth_state.dart) — implementation
- [caching.md](caching.md) — cache security
- [issue #2158](../issues/2158-logout-crash-mutation-race.md) — logout-rebound race

---

**Last Updated:** 2026-05-25
