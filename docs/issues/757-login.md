# Issue #757: Login Required Too Frequently

**Priority:** P0 — Fix Now
**Status:** Open
**Reported by:** user2@example.org (Android 16, Pixel 9 Pro, v0.4.0)
**Confirmed on:** Android and iOS

## Problem Statement

Users are forced to re-authenticate at least once daily, even without app
updates. This creates significant friction for all user personas, especially
casual users with low tolerance for overhead.

## Root Cause Analysis

The authentication system uses **24-hour JWT access tokens with no refresh
mechanism**. When a token expires, the next API call triggers an automatic
logout and redirect to the login screen.

### Current Token Lifecycle

1. User logs in → server issues JWT with 24-hour expiration
   (`server/main.go:392`)
2. Token stored in `SharedPreferencesAsync`
   (`app/lib/services/auth_state.dart:139`)
3. Every RPC call includes the token in the Authorization header
4. After 24 hours, server returns `Code.unauthenticated`
   (`server/auth/tokens.go:59-96`)
5. Client catches this, calls `logout()`, clears all state, redirects to login
   (`app/lib/core/utils/rpc_utils.dart:107-111`)

### Why This Is a Problem

- **No refresh tokens**: The system has no mechanism to extend a session without
  full re-authentication.
- **Hard 24-hour wall**: Even active users mid-session get logged out when the
  token expires.
- **Full logout on expiry**: All caches are cleared and the user loses their
  navigation context.
- **Every service triggers logout**: All service providers have
  `onUnauthenticated` callbacks that call `logout()`
  (`app/lib/services/providers.dart`).

### Key Files

| Component | File | Purpose |
|-----------|------|---------|
| Token generation | `server/auth/tokens.go:21-57` | `GenerateToken()` with configurable expiration |
| Token config | `server/main.go:390-392` | Sets 24-hour expiration |
| Token validation | `server/auth/tokens.go:59-96` | `ValidateToken()` — rejects expired tokens |
| Login RPCs | `server/services/login/authentication.go` | Login, OIDCLogin handlers |
| Proto API | `proto/ripls/api/login_service.proto` | Response includes `access_token`, `expires_in` |
| Client auth state | `app/lib/services/auth_state.dart` | Token storage, logout, state management |
| Expiry handling | `app/lib/core/utils/rpc_utils.dart:107-111` | Auto-logout on unauthenticated error |
| Service providers | `app/lib/services/providers.dart` | `onUnauthenticated` callbacks on all services |

## Potential Solutions

### Option A: Refresh Tokens (Recommended)

Introduce a long-lived refresh token alongside the short-lived access token.
The client uses the refresh token to obtain new access tokens without user
interaction.

**Pros:**
- Industry standard (OAuth 2.0 pattern)
- Short-lived access tokens maintain security
- Users stay logged in for weeks/months
- Transparent to the user — no re-authentication unless refresh token expires

**Cons:**
- Requires server-side refresh token storage (database)
- Need token rotation and revocation logic
- More complex implementation

### Option B: Longer Access Token Expiration

Simply increase the JWT expiration from 24 hours to 30+ days.

**Pros:**
- Minimal code changes (single line in `server/main.go`)
- Immediately fixes the symptom

**Cons:**
- Security risk: long-lived tokens can't be revoked without a blocklist
- No granular session management
- If a token is stolen, it's valid for the full duration
- Doesn't solve the underlying architecture gap

### Option C: Silent Re-authentication

Store credentials locally and automatically re-authenticate when the token
expires.

**Pros:**
- Simple client-side change for OIDC users (can silently re-auth with cached
  provider tokens)

**Cons:**
- Doesn't work for email/password users (can't store passwords)
- Depends on OIDC provider token freshness
- Unreliable — provider sessions also expire

## Recommendation

**Option A: Refresh Tokens** is the recommended approach.

This is the standard solution used by virtually all mobile apps. It provides
the best balance of security and user experience:

- Access tokens remain short-lived (1 hour) for security
- Refresh tokens are long-lived (90 days) for persistence
- Rotating refresh tokens: each use issues a new token and revokes the old one
- Token theft detection: reuse of a revoked token triggers full revocation

## Decisions

1. **Refresh token lifetime**: **90 days.** Maximizes convenience for casual
   users. Token rotation (see #2) mitigates the exposure window.
2. **Token rotation**: **Yes — use rotating refresh tokens.** Each refresh
   issues a new refresh token and revokes the old one. This is recommended
   because it limits the damage of a leaked refresh token: a stolen token can
   only be used once before the legitimate user's next refresh invalidates it.
   If the server sees a revoked refresh token being reused, it should revoke
   all tokens for that user (token theft detection per RFC 6819).
3. **Concurrent sessions**: **Yes — support multiple devices.** Each
   login/registration creates an independent refresh token. Multiple devices
   each maintain their own token pair. No session limit enforced.
4. **Revocation scope**: **No "log out everywhere" in initial release.** Phase
   4 (session management) is deferred. Individual logout revokes only the
   device's own refresh token.
5. **Migration**: **Force a one-time re-login.** Existing users without a
   refresh token will be logged out on the first request after deployment.
   The normal `Code.unauthenticated` → logout flow handles this naturally.
   No migration code needed.
6. **Access token duration**: **Reduce to 1 hour** once refresh tokens are in
   place. The current 24-hour window is unnecessarily long for an access token
   and limits the effectiveness of token revocation.

## Phased Implementation Plan

### Phase 1: Server-Side Refresh Token Infrastructure

**Goal:** Add refresh token generation, storage, validation, and a new
`RefreshToken` RPC.

**Changes:**

1. **Proto** — `proto/ripls/models/user.proto`
   - Add `RefreshToken` message: `id`, `user_id`, `token_hash` (SHA-256),
     `expires_at_unix_sec`, `created_at_unix_sec`, `is_revoked`
   - This is a persistence model — belongs in `models/`, not `api/`

2. **Proto** — `proto/ripls/api/login_service.proto`
   - Add `refresh_token` string field to `LoginResponse`, `RegisterResponse`,
     `OIDCLoginResponse`, `OIDCRegisterResponse` (inline string field, no
     cross-import of models/)
   - Add `RefreshToken` RPC with dedicated `RefreshTokenRequest`
     (`refresh_token` string) and `RefreshTokenResponse` (`access_token`,
     `refresh_token`, `token_type`, `expires_in`) — each RPC gets its own
     request/response types per project conventions

3. **Server** — `server/auth/tokens.go`
   - Add `GenerateRefreshToken()` — generates a cryptographically random token
     (e.g., 32-byte base64url)
   - Add `HashRefreshToken()` — SHA-256 hash for storage (never store raw
     tokens)
   - These are shared library functions, not service logic — correct placement
     in `server/auth/`

4. **Server** — `server/services/login/`
   - Update `Login`, `Register`, `OIDCLogin`, `OIDCRegister` to generate and
     return a refresh token alongside the access token
   - Store hashed refresh token in database
   - Implement `RefreshToken` RPC handler in a new file
     `server/services/login/refresh.go` (keep files short and focused per
     server architecture guidelines)
   - `RefreshToken` RPC logic:
     - Look up token by hash
     - Validate not expired and not revoked
     - Generate new access token + new refresh token (rotation)
     - Revoke old refresh token
     - Return new tokens
   - Add unit tests in `server/services/login/refresh_test.go`

5. **Server** — Auth middleware bypass for `RefreshToken` RPC
   - The `RefreshToken` endpoint must work **without a valid access token**
     (the user's access token is expired — that's why they're refreshing).
     The auth middleware in `server/main.go` must exclude the `RefreshToken`
     RPC path from JWT validation, similar to how `Login`/`Register` are
     handled. The RPC authenticates via the refresh token itself, not the
     Authorization header.

6. **Server** — `server/main.go`
   - Reduce access token expiration to 1 hour
   - Configure refresh token expiration (90 days)
   - Register `RefreshToken` RPC path as unauthenticated (no JWT required)

7. **Tests** — `server/services/login/refresh_test.go`
   - Refresh token generation and storage
   - Token refresh happy path
   - Expired refresh token rejection
   - Revoked refresh token rejection
   - Token rotation (old token invalidated after refresh)
   - Reuse of revoked refresh token is rejected (detect token theft)

### Phase 2: Client-Side Token Refresh

**Goal:** Client automatically refreshes access tokens before expiration and
on unauthenticated errors.

**Changes:**

1. **Client** — `app/lib/services/auth_state.dart`
   - Store `refresh_token` in `flutter_secure_storage` (not
     `SharedPreferencesAsync`). Refresh tokens are long-lived credentials
     (30+ days) and must use encrypted platform storage (Keychain on iOS,
     EncryptedSharedPreferences on Android). The short-lived access token
     can remain in SharedPreferences.
   - Add `refreshAccessToken()` method that calls `RefreshToken` RPC via
     `AuthService` (auth_state calls auth_service — this is infrastructure-
     level, not domain data, so the repository pattern does not apply)
   - Update `setAuthState()` to persist refresh token to secure storage
   - Update `logout()` to clear refresh token from secure storage
   - Add a mutex/`Completer` guard so that concurrent 401 responses trigger
     only **one** refresh attempt. All concurrent callers await the same
     in-flight refresh and receive the result. This prevents token rotation
     race conditions where multiple refreshes invalidate each other's tokens.

2. **Client** — `app/lib/core/utils/rpc_utils.dart`
   - Centralize the refresh-then-retry logic here instead of updating each
     service provider's `onUnauthenticated` callback individually. The RPC
     utility layer is the right place for a cross-cutting interceptor.
   - On `Code.unauthenticated`:
     1. Attempt token refresh (via `AuthStateNotifier.refreshAccessToken()`)
     2. If refresh succeeds, retry the original request with the new token
     3. If refresh fails (expired/revoked refresh token), then call `logout()`
   - This replaces the current pattern where every service provider has its
     own `onUnauthenticated: () => logout()` callback

3. **Client** — `app/lib/services/auth_service.dart`
   - Add `refreshToken(String refreshToken)` method calling the `RefreshToken`
     RPC. This is the service layer — correct placement.

4. **Client** — `app/lib/services/providers.dart`
   - Simplify `onUnauthenticated` callbacks: they no longer need to call
     `logout()` directly since `rpc_utils.dart` now handles refresh-then-
     retry centrally. Remove or simplify the per-service callbacks.

5. **Dependencies** — `app/pubspec.yaml`
   - Add `flutter_secure_storage` package for encrypted credential storage

6. **Tests** — `app/test/`
   - Token refresh on 401
   - Retry after successful refresh
   - Logout on refresh failure
   - Concurrent 401s trigger only one refresh (mutex/Completer guard)
   - Secure storage read/write for refresh tokens

### Phase 3: Proactive Token Refresh

**Goal:** Refresh tokens before they expire to avoid any user-visible
interruption.

**Changes:**

1. **Client** — `app/lib/services/auth_state.dart`
   - Decode JWT to read `exp` claim (JWTs are base64url-encoded — parse the
     payload segment directly without adding a JWT library dependency)
   - Schedule proactive refresh when token has < 5 minutes remaining
   - Refresh on app resume (from background) if token is expired or near-expiry

2. **Client** — Background refresh on app lifecycle events
   - `WidgetsBindingObserver.didChangeAppLifecycleState` — refresh on resume
   - This should live in `auth_state.dart` (infrastructure) not in a screen
     or widget

3. **Tests**
   - Proactive refresh scheduling
   - App resume refresh behavior
   - JWT expiration parsing

### Future: Session Management and Revocation (Deferred)

"Log out everywhere" and session listing are deferred. Individual device
logout (revoking the device's own refresh token) is handled in Phase 2 as
part of the `logout()` flow. Session management can be revisited if users
request it.

---

## Audit Notes

The following changes were made during architectural compliance review against
`docs/client/architecture.md`, `docs/client/caching.md`, and
`docs/server/architecture.md`:

1. **[Phase 1] Added auth middleware bypass** — The `RefreshToken` RPC must
   work without a valid access token. The original plan omitted this, which
   would have caused a deadlock: users can't refresh because their token is
   expired, but the endpoint requires a valid token.

2. **[Phase 1] Added file organization** — Server architecture requires short,
   focused files (< 1000 lines). Added `refresh.go` and `refresh_test.go` as
   separate files instead of adding to the existing `authentication.go`.

3. **[Phase 1] Clarified proto separation** — Confirmed `RefreshToken` model
   goes in `models/` (storage type) and API fields use inline strings (no
   cross-import between `api/` and `models/` packages). Added explicit note
   about dedicated request/response types per RPC.

4. **[Phase 1] Clarified shared library placement** — `GenerateRefreshToken()`
   and `HashRefreshToken()` belong in `server/auth/` (shared library), not in
   the login service, per the server architecture principle that shared
   functionality lives in libraries.

5. **[Phase 2] Secure storage for refresh tokens** — Changed from
   `SharedPreferencesAsync` (plaintext XML on Android) to
   `flutter_secure_storage` (Keychain/EncryptedSharedPreferences). A 90-day
   credential in plaintext storage is a security risk.

6. **[Phase 2] Centralized refresh interceptor** — Moved refresh-then-retry
   logic to `rpc_utils.dart` as a cross-cutting interceptor instead of
   updating each service provider's `onUnauthenticated` callback individually.
   This aligns with the existing pattern where `rpc_utils.dart` already handles
   unauthenticated errors centrally.

7. **[Phase 2] Added concurrent refresh guard** — Multiple simultaneous 401
   responses must trigger only one refresh attempt. Without this, token
   rotation causes race conditions where the second refresh invalidates the
   first refresh's new token. Added mutex/Completer pattern.

8. **[Phase 2] Auth infrastructure exemption from repository pattern** — The
   client architecture mandates "Repositories are the ONLY layer that calls
   Services." Auth token refresh is infrastructure-level plumbing, not domain
   data access, so it correctly bypasses the repository layer (same as the
   existing `auth_state.dart` → `auth_service.dart` pattern).

9. **[Phase 3] JWT parsing without library** — Specified that JWT `exp` claim
   can be extracted by base64url-decoding the payload segment directly, without
   adding a JWT library dependency.

10. **[Phase 4] Deferred** — Session management ("log out everywhere", session
    listing) deferred per decision. Individual logout handled in Phase 2.
