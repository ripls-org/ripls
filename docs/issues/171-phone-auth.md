# Issue #171: Passwordless phone number registration and login

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/171
**Filed:** 2025-10-29 | **Last activity:** 2026-04-13
**Labels:** none | **Assignees:** unassigned
**Context docs:** docs/registration_and_login.md

## Problem Statement

Ripls currently supports three auth methods — email/password, Google Sign-In,
and Apple Sign-In. Many users expect phone-based authentication as an
alternative, which is now the industry standard for mobile apps. The lack of
phone auth adds friction to onboarding, particularly for users who don't have
or prefer not to use email for app registration.

A previous attempt at phone-based features focused on sending invitation texts
from Ripls, which was abandoned because it's spammy and difficult to get cleared
with SMS providers. This issue is explicitly scoped to **user-initiated phone
auth** (registration and login), not outbound messaging from Ripls.

## Context from Comments

- **Triage (tescobar, 2026-03-05):** Rated P2. Reduces onboarding friction for
  all user personas. Many users expect phone-based auth.
- **maintainer (2026-04-13):** Previous attempt at phone-based invites was
  abandoned (spammy, provider clearance issues). This time: support
  industry-standard phone sign-up. Research the simplest, least-scammy approach.

Key direction: simplicity, industry-standard patterns, passwordless phone auth.
No outbound SMS from Ripls for invites — only Firebase-managed verification
codes during user-initiated flows. Phone and email auth are separate tracks —
phone users are passwordless, email users reset passwords via email.

## Decision: Firebase Phone Auth (passwordless)

**Provider:** Firebase Phone Auth, using the existing `ripls-dev` and
`ripls-prod` Firebase projects.

**Auth model:** Passwordless. Phone users verify ownership via OTP on every
sign-in — no password stored or needed. This matches the industry standard
(Uber, WhatsApp, Signal, etc.) and is the simplest possible flow.

**Scope:** Phone registration and login only. No cross-pollination between phone
and email auth methods — phone users are passwordless, email users reset
passwords via email. Each auth method is self-contained.

**Why Firebase:** The infrastructure is already in place:
- Firebase projects exist for both dev and prod
- `firebase_core`, `firebase_messaging`, `firebase_analytics`,
  `firebase_crashlytics`, `firebase_performance` are already Flutter dependencies
- Firebase Admin SDK (`firebase.google.com/go/v4`) is already in `go.mod` and
  used by FCM (`server/notifications/fcm/provider.go`)
- Only `firebase_auth` needs to be added to the Flutter app

The pattern maps cleanly to the existing OIDC flow: Firebase verifies phone
ownership and issues an ID token → server validates the ID token and manages the
Ripls user lifecycle (JWT + refresh tokens). The phone number extracted from the
verified Firebase token becomes the user's identity, like email is for
email/password users.

## Open Questions

- **Phone number as unique identifier?** — Recommendation: Yes, enforce
  uniqueness. Like email, one phone number = one account. Store in E.164 format
  (`+15551234567`). Users register with either email or phone, not both required.

- **Country code handling in UI?** — Recommendation: Use a phone input widget
  with country code picker (e.g., `intl_phone_field` or `phone_form_field`
  Flutter package). Default to device locale. Store numbers in E.164 format.

- **What happens if a user loses their phone number?** — Recommendation:
  Standard account recovery flow. If they registered with phone only, they'd
  need to contact support. Document this in the UI ("make sure you can receive
  SMS at this number"). This is the same limitation every phone-auth app has.

- **Should the JWT contain phone_number instead of email?** — Recommendation:
  Add `phone_number` as a JWT claim alongside `email`. One will be empty
  depending on auth method. The `Info` struct in `server/auth/tokens.go` gains
  a `PhoneNumber` field. Auth middleware populates whichever is present.

- **Firebase app instance sharing?** — Recommendation: The FCM code in
  `server/notifications/fcm/provider.go` already initializes a `firebase.App`.
  Extract initialization into a shared helper so both FCM and phone auth use the
  same app instance. Avoid initializing Firebase twice.

## Implementation Plan

### Phase 1: Proto definitions and Firebase client setup

**Goal:** Define the API contract and add `firebase_auth` to the Flutter app.

**Tasks:**
- [x] Add `optional string phone_number = 18` to `proto/ripls/models/user.proto`
      (E.164 format)
- [x] Add `AUTH_METHOD_PHONE = 4` to `AuthMethod` enum in
      `proto/ripls/models/user.proto`
- [x] Define new RPCs in `proto/ripls/api/login_service.proto`:
  - `PhoneRegister(PhoneRegisterRequest) → PhoneRegisterResponse`
    - Fields: `string firebase_id_token = 1`, `string name = 2`,
      `string short_code = 3`, `string gear_id = 4`,
      `optional string simulation_id = 5`
    - Response: `User user = 1`, `Tokens tokens = 2`
  - `PhoneLogin(PhoneLoginRequest) → PhoneLoginResponse`
    - Fields: `string firebase_id_token = 1`
    - Response: `User user = 1`, `Tokens tokens = 2`
- [x] Add `firebase_auth` dependency to `app/pubspec.yaml`
- [x] Run `npm run generate` to generate Go and Dart code
- [x] Verify builds: `npm run build` (server) and `flutter build` (app)

**Design notes:**
- Both registration and login require `firebase_id_token` because phone auth is
  passwordless — every auth event starts with OTP verification on the client,
  producing a Firebase ID token that the server validates.
- No password fields anywhere in the phone auth flow.
- `PhoneRegister` includes `name` because we still need a display name. Phone
  number alone isn't enough for community interactions.
- `PhoneLogin` only needs the Firebase ID token — the server extracts the phone
  number from it and looks up the user.

**Validates:** API contract is defined; Firebase Auth SDK integrates without
breaking existing builds.

### Phase 2: Server — Firebase token validation and shared app initialization

**Goal:** Server can validate Firebase Phone Auth ID tokens.

**Tasks:**
- [x] Extract Firebase app initialization from `server/notifications/fcm/provider.go`
      into a shared package (e.g., `server/firebase/app.go`):
  - `NewApp(ctx, credentialsPath) → (*firebase.App, error)`
  - Reuse from both FCM and phone auth
  - Update FCM provider to use the shared initializer
- [x] Create `server/auth/firebase.go`:
  - `FirebaseAuth` struct wrapping `auth.Client` (from `firebase.google.com/go/v4/auth`)
  - `NewFirebaseAuth(app *firebase.App) → (*FirebaseAuth, error)` — calls
    `app.Auth(ctx)` to get the auth client
  - `VerifyPhoneToken(ctx, idToken) → (phoneNumber string, err error)`:
    - Call `client.VerifyIDToken(ctx, idToken)` to validate signature and claims
    - Extract `phone_number` from token claims
    - Reject tokens where `firebase.sign_in_provider` is not `"phone"` (prevent
      using a Google ID token to impersonate a phone user)
    - Return E.164-formatted phone number
- [x] Write unit tests for `VerifyPhoneToken` (mock the Firebase auth client):
  - Valid phone token → returns phone number
  - Expired token → error
  - Non-phone provider token → error
  - Missing phone_number claim → error
- [x] Verify build: `npm run build`

**Validates:** Server can securely validate Firebase Phone Auth tokens and
extract verified phone numbers.

### Phase 3: Server — phone registration and login RPCs

**Goal:** Users can register and log in with phone number via the server.

**Tasks:**
- [x] Add `phone_number` unique index in storage (allow NULL for email/OIDC
      users). Add user lookup by phone number to storage layer.
- [x] Implement `PhoneRegister` RPC in `server/services/login/registration.go`:
  - Validate Firebase ID token → extract verified phone number via
    `FirebaseAuth.VerifyPhoneToken()`
  - Validate short code (same pattern as `Register`)
  - Create user with `auth_method = PHONE`, `phone_number` set, no
    `password_hash`
  - Call `completeRegistration()` — auto-join community, generate JWT + refresh
    token
  - Include `phone_number` in JWT claims
- [x] Implement `PhoneLogin` RPC in `server/services/login/authentication.go`:
  - Validate Firebase ID token → extract verified phone number
  - Look up user by phone number
  - Verify `auth_method == PHONE`
  - Issue JWT + refresh token (include `phone_number` in claims)
  - If user not found: return clear error ("no account with this phone number")
  - If wrong auth method: return "please login with [method]" (same pattern as
    email/OIDC mismatch)
- [x] Update `getAuthMethodName()` in `helpers.go` to handle `AUTH_METHOD_PHONE`
- [x] Update JWT generation (`server/auth/tokens.go`) to include `phone_number`
      claim when present
- [x] Update `Info` struct and `NewAuthFunc()` to extract `phone_number` from
      JWT
- [x] Initialize `FirebaseAuth` in `server/main.go` using the shared Firebase
      app, inject into login service
- [x] Write integration tests:
  - Phone registration with valid invitation → success
  - Phone registration with invalid/missing token → error
  - Phone login with registered phone → success
  - Phone login with unregistered phone → clear error
  - Phone login attempt by email user → auth method mismatch error
  - Duplicate phone registration → error
- [x] Verify build: `npm run build`

**Validates:** Server handles phone registration and login end-to-end with
proper auth method enforcement.

### Phase 4: Client — phone verification and registration UI

**Goal:** Users can register with phone number in the Flutter app.

**Tasks:**
- [x] Add phone number input widget with country code picker (evaluate
      `phone_form_field` or `intl_phone_field` packages for E.164 output and
      country detection from device locale). All colors must use `AppColors.*`
      helpers (no hardcoded colors — the app supports light and dark modes)
- [x] Create phone verification flow (can be a screen or bottom sheet):
  - Enter phone number → call `FirebaseAuth.verifyPhoneNumber()`:
    - `codeSent` callback: show OTP input
    - `verificationCompleted` callback (Android auto-retrieval): auto-proceed
    - `verificationFailed` callback: show error
  - Enter 6-digit OTP code → `PhoneAuthProvider.credential()` →
    `FirebaseAuth.signInWithCredential()` → get Firebase ID token
  - On success, proceed to registration (name entry) or login
- [x] Add `phoneRegister({firebaseIdToken, name, shortCode})` and
      `phoneLogin({firebaseIdToken})` methods to `AuthService`
- [x] Add phone registration state to `RegisterViewModel` using Freezed
      immutable state (e.g., `phoneVerified`, `firebaseIdToken`, `phoneNumber`
      fields in the Freezed state class). All state updates via `copyWith()`.
      Screen interacts with ViewModel only — no direct service calls from
      screens.
- [x] Update `RegisterScreen` to offer phone registration:
  - Add "Register with Phone" option alongside email and OIDC buttons
  - Phone flow: phone verification → name entry → ViewModel calls
    `AuthService.phoneRegister()` (screen never calls service directly)
  - Wire `short_code` and `gear_id` through the phone flow
- [x] Add i18n strings to both `app_en.arb` and `app_es.arb` for all new
      user-facing text: phone input labels, OTP prompts, "Register with Phone"
      button, verification status messages, error messages. Use `register*`
      prefix for key names per naming convention. ViewModels expose typed state;
      screens resolve to `context.l10n.*` strings in `build()`.
- [x] Test on iOS and Android (Firebase phone auth differs per platform —
      reCAPTCHA on iOS, auto-retrieval on Android). Use `--reporter expanded`
      for Flutter tests.

**Validates:** Users can register via phone in the app and land in their
community.

### Phase 5: Client — phone login UI

**Goal:** Existing phone users can log in.

**Tasks:**
- [x] Create or update a login ViewModel (Freezed state) to handle phone login
      flow — phone verification state, error handling, Firebase token. Screens
      interact with ViewModel only, never call `AuthService` directly.
- [x] Update `LoginScreen` to support phone login:
  - Add "Log in with Phone" option (tab, toggle, or separate button)
  - Phone flow: phone verification (same component as registration) →
    ViewModel calls `PhoneLogin` RPC with Firebase ID token
  - Handle errors: unregistered phone, auth method mismatch
  - All colors via `AppColors.*` helpers
- [x] Update `InvitationLandingScreen` to show phone login option
- [x] Ensure phone login works with invitation deep links (user clicks invite
      link → lands on invitation screen → chooses phone login → verifies →
      gets logged in → joins community)
- [x] Add i18n strings to both `app_en.arb` and `app_es.arb` for login-specific
      text: "Log in with Phone", phone login error messages, etc. Use `login*`
      prefix for key names.
- [x] Test complete flows: phone registration → logout → phone login. Use
      `--reporter expanded` for Flutter tests.

**Validates:** Full phone auth lifecycle works end-to-end in the app.

### Phase 6: Documentation and polish

**Goal:** System is documented, edge cases handled, ready for production.

**Tasks:**
- [x] Update `docs/registration_and_login.md` with phone auth flows
- [x] Add phone auth to security considerations (Firebase token validation,
      phone number uniqueness, E.164 normalization, provider enforcement)
- [x] Handle edge cases:
  - Phone number already registered (clear error message)
  - Firebase verification timeout/failure (retry flow)
  - Invalid/unreachable phone numbers (Firebase handles this, but show clear UI)
  - User changes phone number (carrier reassignment — phone-only users would
    lose access; document this risk)
- [x] Add `AUTH_METHOD_PHONE` handling to all auth-method-aware code paths:
  - Login error messages in `getAuthMethodName()`
  - Account info display in client
  - Any admin or analytics code that switches on auth method

**Validates:** Phone auth is production-ready with documentation and error
handling.

## Audit Notes

*Audited against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/server/architecture.md`.*

**Fixes applied:**

1. **i18n (critical)** — Added ARB string requirements to Phases 4 and 5. All
   user-facing strings must be in both `app_en.arb` and `app_es.arb` with proper
   key prefixes (`register*`, `login*`). ViewModels expose typed state; screens
   resolve to `context.l10n.*`.
2. **ViewModel data access** — Clarified in Phases 4 and 5 that screens never
   call `AuthService` directly. All phone auth logic goes through ViewModels.
   Screens interact via `ref.watch()` / `ref.read().notifier`.
3. **Freezed state** — Added requirement for Freezed immutable state classes in
   phone auth ViewModels, with `copyWith()` for state updates.
4. **Theme-aware colors** — Added `AppColors.*` requirement for all new UI
   components (phone input, OTP screen). No hardcoded colors.
5. **Flutter test reporter** — Added `--reporter expanded` requirement to test
   tasks in Phases 4 and 5.

**No issues found with:**

- Server architecture: `server/auth/firebase.go` and `server/firebase/app.go`
  follow the shared library pattern correctly. Constructor injection into login
  service is the right pattern. No service-to-service dependencies introduced.
- Caching: Auth operations are correctly excluded from repository caching (they
  are one-shot mutations, not cached reads).
- Proto separation: New phone RPCs correctly defined in `api/` protos, phone
  number field in `models/` user proto. No cross-package imports.

## Related Issues

- #757 — Token refresh implementation (refresh tokens already implemented;
  phone auth will use the same token lifecycle)
- Previous phone/SMS attempt (referenced in comments) — abandoned due to
  outbound SMS spam concerns; this issue uses Firebase-managed OTP only
