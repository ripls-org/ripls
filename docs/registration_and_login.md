---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Invitation-based registration and authentication flows — passwordless email one-time code, Google/Apple OIDC, phone OTP, additive credential-based sign-in, adding a phone to an existing account, JWT and refresh tokens, bootstrap first-user exception, and community auto-join.
  globs: [server/services/login/**, server/services/user/add_phone.go, server/auth/**, app/lib/presentation/screens/auth/**, app/lib/presentation/widgets/auth/**, app/lib/services/auth_service.dart, app/lib/services/auth_state.dart]
  triggers: [registration, login, authentication, oidc, jwt, refresh-token, phone-otp, email-otp, email-code, email-verified, bootstrap, password, add-phone]
  lens: [domain, security, client, server]
  domain: auth
freshness:
  verified_commit: "5e33d2173"
  verified_on: "2026-08-14"
---
# User Registration and Invitation System

This document describes the user registration, authentication, and community invitation flows in Ripls.

## Overview

Ripls uses an invitation-based registration system where users must be invited to join the platform. The system supports:

- **Multiple Authentication Methods**: Email (passwordless one-time code), Google Sign-In (OIDC), Apple Sign-In (OIDC), and Phone (passwordless OTP via Firebase). Email/password is legacy and being removed (#2571).
- **Credential-Based Sign-In**: an account can sign in by any credential it actually holds; `auth_method` records how it was *created*, not how it may sign in
- **Shareable Invitation Links**: Reusable invitation links for joining communities
- **Community Auto-Join**: Users are automatically added to the community they were invited to
- **Bootstrap Exception**: The first user can register without an invitation to bootstrap the system
- **Gear Deep Linking**: Invitation links can include a specific gear item ID for direct navigation

### Key Components

**Backend:**
- `proto/ripls/api/login_service.proto` - Authentication API definitions
- `proto/ripls/models/user.proto` - User and refresh token data models
- `proto/ripls/models/community.proto` - Community and invitation link data models
- `server/services/login/authentication.go` - Login RPC handlers
- `server/services/login/registration.go` - Registration RPC handlers
- `server/services/login/refresh.go` - Refresh token RPC handler and helpers
- `server/services/community/service.go` - Community and invitation management
- `server/services/login/email_otp.go` - Email one-time-code issue/verify and the ownership-proof token
- `server/auth/otp_code.go` - One-time-code hashing (bcrypt, cost 12)
- `server/auth/email_proof.go` - Email-ownership proof token (mint + validate)
- `server/auth/password.go` - Password hashing (bcrypt, cost 12); legacy, leaves with the password path
- `server/auth/tokens.go` - JWT generation/validation, refresh token generation and hashing
- `server/auth/firebase.go` - Firebase Phone Auth token validation
- `server/firebase/app.go` - Shared Firebase app initialization
- `server/services/login/phone_auth.go` - Phone registration and login RPC handlers

**Frontend:**
- `app/lib/presentation/screens/auth/register_screen.dart` - Registration UI (email + OIDC + phone)
- `app/lib/presentation/screens/auth/login_screen.dart` - Login UI (email + OIDC + phone)
- `app/lib/presentation/screens/auth/phone_auth_screen.dart` - Phone OTP verification flow; on the phone-first guest screen it also hosts inline Google + email sign-up below the fold
- `app/lib/presentation/screens/auth/email_auth_screen.dart` - Email one-time-code flow (address → code → name), hosted by `RegisterScreen`, `LoginScreen`, and `PhoneAuthScreen`
- `app/lib/presentation/widgets/auth/otp_code_field.dart` - The 6-digit code field shared by the phone and email flows
- `app/lib/presentation/screens/gear/gear_screen.dart` - Gear detail view for deep linking
- `app/lib/services/auth_service.dart` - Authentication API client
- `app/lib/services/auth_state.dart` - Token storage, refresh, and session lifecycle management
- `app/lib/core/utils/rpc_utils.dart` - RPC execution with transparent token refresh and retry
- `app/lib/core/router/app_router.dart` - Deep link routing and navigation

---

## Phase 0: Bootstrapping the First User and Community

The first user in the system requires special handling since there's no one to invite them.

### Server: Bootstrap User Registration

**File:** `server/services/login/registration.go` (`EmailRegister` method)

When the database has zero users, the registration flow allows registration without an invitation short code:

1. **User Check**: Server calls `storage.HasAnyUsers(ctx)` to determine if this is the first user
2. **Bootstrap Registration**: If no users exist, invitation short-code validation is skipped
3. **User Creation**: User is created with `auth_method` set based on registration type (EMAIL_PASSWORD, GOOGLE, or APPLE)
4. **JWT Token**: Access token is generated and returned

**Note:** Bootstrap users are created with `ROLE_USER` (not admin). The first user has no special privileges.

### Client: Bootstrap Registration Flow

**File:** `app/lib/presentation/screens/auth/register_screen.dart`

1. User navigates to registration screen (no invitation short code)
2. For email: User completes the one-time-code flow (address → code → name,
   see *Email Registration* below) — a password cannot create an account (#2864)
3. For OIDC: User clicks "Sign in with Google/Apple" and completes OAuth flow
4. Client calls `LoginService.EmailRegister` or `LoginService.OIDCRegister` with empty `short_code`
5. On success, user is authenticated and redirected to home screen

### Creating the First Community

After registration, the first user must manually create a community:

**Server:** `server/services/community/service.go` (`CreateCommunity` RPC)
**Client:** Community creation UI (implementation details in app code)

The user who creates a community becomes its `creator_id` and is automatically added as a member via `CommunityUser` record.

---

## Phase 1: Creating and Sharing Invitation Links

Existing community members can create shareable invitation links that can be used by anyone to join the community.

### Server: Creating Shareable Invitation Links

**File:** `server/services/community/share_links.go` (`GetOrCreateShareLink` RPC)

When a user wants to invite others to their community:

1. **Membership Verification**: Verify user is a member of the community
2. **Token Lookup**: Check if user already has an active (non-revoked) invitation link for this community
3. **Code Reuse or Creation**:
   - If active link exists: Return existing short code
   - If no active link: Create new `ShareLink` with:
     - `inviter_id`: ID of user creating the link
     - `community_id` (via the `community_invite_id` target): Community being invited to
     - `short_code`: 8-char alphanumeric code (crypto/rand, confusable chars excluded)
     - `is_revoked`: false
     - `created_at_unix_sec`: Current timestamp
4. **URL Generation**: Build full invitation URL (`<hostname>/go/<short_code>`)

**Invitation Link Format:**
```
https://ripls.app/go/<short_code>
```

`GetOrCreateShareLink` produces bare `/go/<short_code>` URLs; the deep-link
target (gear/request/experience) lives on the `ShareLink` row and is read back
via `CheckInvitation`. The legacy `?gear_id=<gear_uuid>` query-param form was
minted by the removed `GetOrCreateInviteLink` shim (#2058); already-sent URLs
carrying it still resolve through a web-page fallback.

See [`docs/invitations.md`](invitations.md) for the full short-link system
(server-rendered `/go/` landing pages, OG previews, capacity limits).

**Key Features:**
- **One Link Per User-Community**: Each user has at most one active link per community
- **Reusable**: Links can be used multiple times (not consumed on use)
- **No Expiration**: Links remain valid until manually revoked
- **No Email Binding**: Anyone with the link can use it (not tied to specific email address)

### Client: Managing Invitation Links

**File:** `app/lib/presentation/screens/communities/invite_sheet.dart`

Users can:
- **Create/Get Link**: Call `GetOrCreateShareLink` RPC to get their shareable link
- **Copy to Clipboard**: Copy the invitation URL for sharing via any channel
- **Share**: Use native share sheet to send link via messaging apps, email, etc.
- **Revoke Link**: Invalidate their current link via `RevokeShareLink` RPC

### Revoking Invitation Links

**Server:** `server/services/community/share_links.go` (`RevokeShareLink` RPC)

When a user wants to revoke their share link:

1. **Lookup by short code**: Resolve the `ShareLink` row via `lookupShareLink(short_code)`
2. **Ownership check**: Verify `link.InviterId == authInfo.UserID`; reject with `PermissionDenied` otherwise
3. **Mark as Revoked**: Update `is_revoked` field to true (idempotent if already revoked)

After revocation, the user can create a new link with a fresh short code by calling `GetOrCreateShareLink` again.

---

## Phase 2: New User Invitation Acceptance and App Onboarding

New users receive shareable invitation links via any communication channel (messaging apps, email, SMS, etc.) and register via deep links that open the mobile app.

### Deep Link Infrastructure

**Platform Configuration:**
- iOS: `app/ios/Runner/Info.plist` - Custom scheme (`ripls://`) and Universal Links (`https://ripls.app`)
- Android: `app/android/app/src/main/AndroidManifest.xml` - Deep links and App Links
- Domain verification: `.well-known/apple-app-site-association` and `.well-known/assetlinks.json` (served from ripls.app)

**Web Fallback:**
- `invite.html` - Landing page for desktop/mobile web with app store badges

### Client: Deep Link Routing

**File:** `app/lib/core/router/app_router.dart`

GoRouter handles both custom scheme (`ripls://`) and Universal Links (`https://ripls.app`) with authentication-aware routing:

1. **Custom Scheme**: `ripls://invite?token=<short_code>&gear_id=yyy` → redirects to `/invite?token=<short_code>&gear_id=yyy`
2. **Universal/App Link**: `https://ripls.app/go/<short_code>` → the `/go/:code` route redirects into `/invite?token=<short_code>`

The `/invite` route implements smart routing based on authentication state:
- **Not Authenticated**: Shows `RegisterScreen` (the invitation hero is hosted there directly — see below)
- **Authenticated**: Resolves the share link (`ShareLinkRepository.describe`), then either joins+shows the event via `EventInviteJoinGate` (event-target links) or calls `AcceptInvitationLink` RPC and navigates home (`/`) — gear/request-target links no longer navigate to the item on accept; that in-app query-param routing was removed (#2562)

### Client: Unauthenticated Invitee Landing

On the **web**, an unauthenticated visitor following a `/go/{code}` community
invite never reaches `RegisterScreen` at all: the SSR community landing's
"Join the group" CTA sends them to `/group/{communityId}?intent=join&code=…`,
which routes them phone-first to `/verify-phone` and returns them to the
community once `PhoneRegister` has joined them (#2875). That mirrors the
event/gear/request guest flows. `RegisterScreen` remains the surface for every
*other* way into `/invite` — the custom-scheme deep link, the deferred install
referrer, and manual code entry — and for mobile.

There is no longer a separate `InvitationLandingScreen` with a Yes/No
login-vs-register choice. When an unauthenticated user opens an invitation
link, the `/invite` route renders `RegisterScreen` directly with the
`shortCode`. `RegisterScreen` is a shortCode-only new-user surface (#2644) —
it no longer takes `gearId`/`requestId`/`experienceId`; any deep-link target
resolves from the share-link row via `CheckInvitation`, not from query params:

**File:** `app/lib/presentation/screens/auth/register_screen.dart`

The register screen now hosts the invitation hero, all auth methods
(email/OIDC/phone), and an "I already have an account" link in one place, so
existing-account users and new users branch from a single screen rather than a
dedicated landing screen.

### Client: Invitation Short-Code Verification

**File:** `app/lib/presentation/screens/auth/register_screen.dart` (`checkShortCode` on `registerProvider.notifier`)

When the registration screen receives a `shortCode`:

1. **Automatic Validation**: Calls `LoginService.CheckInvitation` RPC to verify the short code
2. **Loading State**: Shows "Verifying invitation..." banner
3. **Success**: Displays "Invitation verified!" banner showing community name and inviter
4. **Failure**: Shows error message (invalid or revoked)

**Server:** `server/services/login/registration.go` (`CheckInvitation` RPC, request field `short_code`)

Validates the short code and returns:
- `is_valid`: Boolean indicating if the code is valid and not revoked
- `community_id`: ID of community being joined
- `community_name`: Name of community for display
- `inviter_name`: Name of user who created the link
- `error_message`: Human-readable error if invalid

### Client: Registration with Invitation

**File:** `app/lib/presentation/screens/auth/register_screen.dart`

User completes registration using one of three methods:

#### Email Registration (passwordless one-time code)

Email sign-up is a mailed 6-digit code, not a password (#2571). The flow
deliberately mirrors phone: obtain a portable ownership proof first, then hand it
to whichever of register/login applies.

1. User enters an email address; client calls `LoginService.RequestEmailCode`
2. Server mails a 6-digit code (bcrypt-hashed at rest, 10-minute TTL, single-use,
   5 attempts per issued code); re-requesting retires the previous code
3. User enters the code; client calls `LoginService.VerifyEmailCode`, which
   returns a short-lived **email proof token** (`server/auth/email_proof.go`)
4. Client calls `EmailRegister` with `email_proof_token` in place of a password;
   on `alreadyExists` it falls back to `EmailLogin` with the same proof, treating
   the person as a returning member (the same pattern `PhoneAuthScreen` uses)

The proof token carries a `purpose` claim and no `user_id`. It is **not** a
session token: `NewAuthFunc` rejects any token carrying a `purpose`, and
`ValidateEmailProofToken` rejects any token without one, so the two cannot be
substituted in either direction even though both are signed with the same secret.

`RequestEmailCode` responds identically whether or not an account exists — it is
the entry point for both sign-up and sign-in, so distinguishing them would make it
an account-existence oracle. Unlike `RequestPasswordReset`, which swallows send
errors for that same reason, a **send failure here is returned**: the response is
already identical either way, so a transport failure leaks nothing, and hiding it
would strand the user at a code-entry screen.

**Two test seams, for different jobs.** `RequestEmailCodeResponse.dev_code`
echoes the code back on dev-mode servers only (used by the integration helpers,
the e2e harness, and the simulation runner, which mints thousands of unique
personas and cannot be enumerated). `EMAIL_OTP_TEST_ACCOUNTS` configures fixed
`address:code` pairs that work **in production**, for app-store review and manual
QA. The allowlist grants the bypass, not the code: an address must be listed *and*
its code must match, since a code accepted for any address would be a universal
backdoor.

##### Password Registration (retired, #2864)

`EmailRegisterRequest.password` remains on the wire (`[deprecated = true]`,
`TODO(#2864)`) only so a build released before the code flow keeps working
without crashing when it sends the field; the server no longer reads it.
`EmailRegister` unconditionally requires `email_proof_token` now — sending a
password without a proof token, or no credential at all, fails with
`InvalidArgument` ("a verification code is required") in every environment,
dev included. This closes the population of password-holding accounts at its
current size: it can only shrink as those accounts move to the code flow,
never grow. The field is dropped (and tag 3 reserved) once a release carrying
the code flow has shipped and soaked.

Accounts created before this ratchet still hold a `password_hash` and keep
signing in via the legacy password path on `EmailLogin` (see *Email Login*
below). Tests that need that pre-#2864 account shape build it out of band with
`server/cmd/seed-legacy-user`, which writes the row through the normal storage
layer rather than exercising a branch production no longer has.

**Server:** `server/services/login/registration.go` (`EmailRegister`)

1. Validates the invitation short code via `validateShortCode`:
   - Looks up `ShareLink` by `short_code`
   - Verifies link is not revoked (no expiration check, no email check)
2. Creates `User` record with:
   - `auth_method = EMAIL_PASSWORD` (records how the account was created, not a
     credential — see *Auth Method Enforcement* below)
   - `invited_by_id`: From invitation link
   - no `password_hash` — the account is passwordless from birth
3. Calls `completeRegistration` to:
   - Insert user
   - Add user to community via `CommunityUser` record
   - Record `COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED` event (with token and optional gear_id)
   - Generate JWT access token and refresh token
   - **Note**: Invitation link is NOT deleted (remains reusable)
4. Promotes provisional placeholders seeded for this email (`promoteProvisionalByEmail`): claims every unclaimed email-keyed provisional user across **all** communities, joining the new account to each and merging its activity history (the email analog of `provisional.PromoteByPhone`). Idempotent and best-effort. **Gated on `User.email_verified_at_unix_sec` being set** (#2571) — a registration that did not prove ownership of the address promotes nothing, which is what closes the former "email shadowing" risk. The gate reads the stamp off the account rather than trusting a caller-supplied flag. See [single_player_mode.md](single_player_mode.md) → *Claiming*.

#### Google Sign-In (OIDC) Registration

1. User clicks "Sign in with Google"
2. Client initiates Google OAuth flow via `OIDCService`
3. Google returns ID token
4. Client calls `LoginService.OIDCRegister` with `id_token`, `short_code`, and optional `gear_id`
5. Same completion flow as email/password

**Server:** `server/services/login/registration.go` (`OIDCRegister`)

1. Validates Google ID token with Google's servers
2. Extracts user info (email, name, subject ID)
3. Validates the invitation short code (checks link is not revoked)
4. Creates `User` with:
   - `auth_method = GOOGLE`
   - `oidc_provider_subject`: Google's user ID
5. Attempts to fetch and store user's avatar from Google profile picture
6. Completes registration:
   - Insert user
   - Add user to community
   - Record `COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED` event
   - Generate JWT access token and refresh token
   - **Note**: Invitation link is NOT deleted (remains reusable)
7. Promotes provisional placeholders seeded for this email (`promoteProvisionalByEmail`), the same cross-community claim as email/password registration. An OIDC email is provider-verified, so this is the true analog of phone-OTP promotion.

#### Apple Sign-In (OIDC) Registration

Similar to Google, but with `auth_method = APPLE` and Apple's OIDC provider.

#### Phone (Passwordless) Registration

**Client:** `app/lib/presentation/screens/auth/phone_auth_screen.dart`
**Server:** `server/services/login/phone_auth.go` (`PhoneRegister`)

1. User clicks "Register with Phone" on register screen or invitation landing
2. `PhoneAuthScreen` shown with phone number input
3. Client calls `FirebaseAuth.verifyPhoneNumber()` → Firebase verifies phone ownership
4. User enters 6-digit OTP → Client creates `PhoneAuthCredential` → `signInWithCredential()` → gets Firebase ID token
5. Client shows name input (phone users have no email or password)
6. Client calls `LoginService.PhoneRegister` with `firebase_id_token`, `name`, `short_code`
7. Same completion flow as email/password

**Returning-member fallback:** If `PhoneRegister` fails with `alreadyExists` (the
phone is already on a Ripls account), `PhoneAuthScreen._handlePhoneRegister`
treats the invitee as a returning member and logs them in via `PhoneLogin` with
the same verified Firebase token rather than surfacing a duplicate-account error.
Any post-auth handoff (auto-RSVP / interest / offer) then fires exactly as it
does after a fresh registration (#2492).

**Phone Number Normalization:**

The client normalizes phone input before sending to Firebase:
- Bare digits (`5551234567`) → prepends `+1` (assumes US)
- Already has `+` prefix (`+445551234567`) → kept as-is (international)
- Non-digit characters (parens, dashes, spaces) are stripped

Numbers are stored server-side in E.164 format (`+15551234567`).

**Server Validation:**

1. Validates Firebase ID token via `FirebaseAuth.VerifyPhoneToken()`:
   - Checks token signature and expiry with Firebase Admin SDK
   - Verifies `firebase.sign_in_provider == "phone"` (rejects Google/Apple tokens to prevent impersonation)
   - Extracts phone number from `phone_number` claim (E.164 format)
2. Checks phone number uniqueness (no existing user with same phone)
3. Validates invitation short code (same as email registration)
4. Creates `User` with:
   - `auth_method = PHONE`
   - `phone_number`: E.164 format (e.g., "+15551234567")
   - No `password_hash` (passwordless)
5. Completes registration (insert user, join community, generate JWT + refresh token)
6. Promotes provisional placeholders for the verified phone (`provisional.PromoteByPhone`): claims every unclaimed provisional user seeded for that E.164 number across **all** communities, joining the new account to each and merging its activity history. Idempotent and best-effort — see [single_player_mode.md](single_player_mode.md) → *Claiming*.
7. Sends the one-time opt-in confirmation ("welcome") SMS/RCS to the verified number, best-effort, via `notificationService.SendPhoneOptInWelcome` (right after `PromoteByPhone`). A failure never blocks registration, and it fires on **first** opt-in only — `PhoneLogin` does not re-send it. See [push_notifications.md](push_notifications.md) (`SendPhoneOptInWelcome`) (#2492).
8. JWT includes `phone_number` claim (email claim is empty for phone users)

#### Firebase Phone Verification: Platform Behavior

Firebase Phone Auth uses different verification mechanisms per platform. The client code is largely identical — Firebase handles platform selection automatically — with a few explicit `kIsWeb` branches for Flutter Web (see the **Web** entry below).

**iOS (real device):** Silent APNs push notification. Firebase sends an invisible push to the device, verifies it was received, and confirms phone ownership — no user interaction required beyond entering the phone number. The app registers an APNs token with Firebase before calling `verifyPhoneNumber()` (via `FirebaseMessaging.instance.getAPNSToken()`). Silent notifications do not require user notification permission (iOS 8.0+).

**iOS (simulator):** APNs is unavailable on simulators, so Firebase falls back to **reCAPTCHA verification**. This opens a web page ("Verifying you're not a robot"), then redirects back to the app via the `REVERSED_CLIENT_ID` URL scheme. The reCAPTCHA callback URL is handled by:
1. `AppDelegate.application(_:open:options:)` calls `Auth.auth().canHandle(url)` to let Firebase process the verification token
2. GoRouter's redirect function detects Firebase callback URLs (by scheme prefix `com.googleusercontent.apps.` or path `/link`) and redirects to `/login` to prevent routing errors

**Android (release build):** Play Integrity verification. Firebase uses the Play Integrity API for invisible app attestation, then sends the OTP via SMS. The Android SMS Retriever API reads it automatically — the user may not even need to type the code. Play Integrity API must be enabled in the GCP project (managed via Terraform: `google_project_service.playintegrity`). The app's release signing SHA-1 fingerprint must be registered in Firebase Console > Project Settings > Android app.

**Android (debug build):** The debug keystore SHA-1 is not registered in Firebase, so Play Integrity attestation fails and Firebase falls back to **reCAPTCHA verification** (same as iOS simulator). To enable invisible verification in debug builds, add the debug keystore SHA-1 to Firebase Console.

**Web (Flutter Web):** There is no APNs or Play Integrity on web, so Firebase always uses **reCAPTCHA verification** — the verifier runs inline rather than redirecting through a custom URL scheme, and Firebase handles its own callback under `/__/auth/*`. The client has two `kIsWeb` branches for this: the phone screen skips APNs token registration (`getAPNSToken()` is iOS-only and throws on web — `phone_auth_screen.dart`), and GoRouter skips the Firebase callback-URL redirect (the `com.googleusercontent.apps.`/`firebaseauth`/`/link` match is mobile-only — `app_router.dart`). See [phone_first_rsvp.md](workflows/phone_first_rsvp.md) for the full web RSVP loop.

**Android troubleshooting:** Firebase silently falls back to reCAPTCHA whenever Play Integrity can't verify the app — there is no error, warning, or log entry. The only observable symptom is a browser popup during verification. Common causes: missing SHA-256 fingerprint in Firebase Console (SHA-1 alone is not sufficient for Play Integrity), Play Integrity API not enabled in GCP, or app not installed from Google Play. Firebase does not expose which verification path was used to the app code.

**Test phone numbers:** Firebase Console allows configuring test phone numbers that bypass real SMS delivery. These work on all platforms including simulators (Authentication → Sign-in method → Phone → Phone numbers for testing).

#### Firebase Infrastructure

Phone auth uses the same two Firebase projects (dev and prod) that are already configured for FCM push notifications, analytics, and crashlytics — no separate project. `scripts/gcp_project.sh` resolves either name to its id.

**Shared Firebase app initialization:** The server initializes a single `firebase.App` instance (`server/firebase/app.go`) that is shared between FCM notifications and phone auth token validation. Both use the Cloud Run service account's Application Default Credentials. Firebase auth initialization is fatal — the server refuses to start if Firebase is unavailable (no silent degradation).

**Play Integrity API:** Enabled via Terraform (`google_project_service.playintegrity`) in both dev and prod GCP projects. Required for invisible phone verification on Android release builds. Without it, Android falls back to reCAPTCHA.

**Server-side token validation:** `server/auth/firebase.go` wraps the Firebase Admin SDK's `auth.Client` to validate Firebase ID tokens. It enforces that the token's `sign_in_provider` is `"phone"` to prevent a user from using a Google or Apple Firebase token to register as a phone user.

**Client-side Firebase config:** The build system selects the correct Firebase config per environment automatically:
- iOS: `app/ios/scripts/copy_firebase_config.sh` copies `config/dev/` or `config/release/` `GoogleService-Info.plist` based on Xcode build configuration
- Android: Gradle build types select `src/debug/`, `src/profile/`, or `src/release/` `google-services.json`

No manual config switching is needed for dev vs prod — it follows the existing build flavor setup.

### Post-Registration Onboarding

After successful registration:

1. User is authenticated with JWT token stored locally
2. Router redirects to home screen (`/`)
3. User sees welcome/landing page
4. User is already a member of the community they were invited to (auto-joined during registration)

**Note:** Registering by one-time code *is* the email verification step — the address is proven before the account exists, and `email_verified_at_unix_sec` is stamped at creation. A legacy password registration leaves the address unverified; such an account acquires the stamp the first time it signs in by code.

---

## Phase 3: Existing Users Joining Additional Communities

Existing users can use the same shareable invitation links to join additional communities. The system automatically detects authenticated users and handles them appropriately.

### Client: Accepting Invitation Links While Logged In

**File:** `app/lib/core/router/app_router.dart` (`/invite` route)

When an authenticated user clicks an invitation link:

1. **Authentication Detection**: Router checks if user is logged in
2. **Target Resolution**: Router resolves the share link (`ShareLinkRepository.describe`) to check its target kind
3. **Event-Target Links**: Routed through `EventInviteJoinGate`, which joins the community (so an authenticated non-member isn't 403'd by the community-scoped access gate) and shows the event
4. **All Other Links**: Directly calls `AcceptInvitationLink` RPC, joins the community, then navigates home (`/`) with the new community selected — gear/request-target links no longer navigate to the item on accept; that in-app query-param routing was removed (#2562)

### Gear Deep Linking

**File:** `app/lib/core/router/app_router.dart` (`/gear/:id` route)

The router includes a dedicated route for viewing specific gear items:

```
/gear/{gear_id}
```

This route:
- Displays `GearScreen` with the specified gear item
- Works for both new users (after registration) and existing users (after login/acceptance)
- Provides a "back" action that returns to the home screen

Note: accepting a gear-target invitation link no longer navigates here
automatically (#2562) — that in-app query-param routing was removed and the
post-accept destination is always home. The live path to this screen for a
gear-linked guest is the phone-first web flow (`/verify-phone` →
`postRegistrationDestination`, see *Registration with Invitation* above), plus
direct sharing and notification deep links.

**Use Cases:**
1. **Direct Sharing**: Users can share links to specific gear items in the community
2. **Notifications**: Push notifications can deep link to specific gear items

**Benefits:**
- Users see the specific item that prompted the invitation
- Maintains context when navigating back to the community

### Server: Accepting Invitation Links

**File:** `server/services/community/invitations.go` (`AcceptInvitationLink` RPC)

When an authenticated user accepts an invitation link:

1. **Code Validation**: Look up the `ShareLink` by `short_code`
2. **Revocation Check**: Verify link is not revoked
3. **Community Lookup**: Get community details
4. **Duplicate Check**: Verify user is not already a member (handled by unique constraint)
5. **Create Membership**: Add `CommunityUser` record with:
   - `user_id`: Authenticated user's ID
   - `inviter_id`: From invitation link
   - `community_id`: From invitation link
6. **Record Event**: Log `COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED` with token and optional gear_id
7. **Response**: Return community ID and name for navigation

**Note**: The same invitation link works for both new user registration and existing user community joins. The link is never consumed or deleted.

---

## Authentication and Login

### Email Login (one-time code)

**Client:** `app/lib/presentation/screens/auth/email_auth_screen.dart`, hosted by
`login_screen.dart` (which leads with the code flow) and `register_screen.dart`
**Server:** `server/services/login/authentication.go` (`EmailLogin`),
`email_otp.go` (the code lifecycle)

1. User enters an email address → code → `VerifyEmailCode` → proof token
2. Client calls `LoginService.EmailLogin` with `email_proof_token`
3. Server accepts **any account holding a verified email** and returns a token pair

**No `auth_method` gate on the verified path**, matching what #2596 did for
`PhoneLogin`. A Google-registered account can therefore sign in by code: its
address is provider-verified, Google's own recovery already runs through that same
inbox, and the alternative was a "please login with Google" dead end with no way
back in. Signing in this way also stamps `email_verified_at_unix_sec` on accounts
that predate the field, so no backfill job is needed.

The legacy password form remains reachable on the login screen behind an explicit
"Use my password instead", for the accounts that still hold one. It keeps the
original `auth_method == EMAIL_PASSWORD` gate and leaves with the password path.

`login_method` in the sign-in log distinguishes `email_code` from
`email_password`; watching the latter fall to zero is what gates the removal.

### OIDC Login (Google/Apple)

**Client:** OIDC sign-in buttons
**Server:** `server/services/login/authentication.go` (`OIDCLogin`)

1. User clicks "Sign in with Google/Apple"
2. Client initiates OAuth flow, receives ID token
3. Client calls `LoginService.OIDCLogin` with provider and token
4. Server:
   - Validates ID token with provider
   - Looks up user by email
   - Verifies `auth_method` matches provider (GOOGLE or APPLE)
   - Returns a short-lived JWT access token and a long-lived refresh token
5. If user registered with different method, returns error

### Phone Login (Passwordless)

**Client:** `app/lib/presentation/screens/auth/phone_auth_screen.dart`
**Server:** `server/services/login/phone_auth.go` (`PhoneLogin`)

1. User clicks "Log in with Phone" on login screen
2. `PhoneAuthScreen` shown → phone number → OTP verification → Firebase ID token
3. Client calls `LoginService.PhoneLogin` with `firebase_id_token`
4. Server:
   - Validates Firebase ID token and extracts phone number
   - Looks up user by phone number
   - Returns a short-lived JWT access token and a long-lived refresh token
5. If no account exists with that phone number, returns "not found" error

**Any account holding a verified phone can log in by phone** — it does not have to
be a phone-*registered* account. Auth methods are additive (see *Auth Method
Enforcement* below): an email/OIDC user who attached a phone via `AddPhoneNumber`
logs in here too. `PhoneLogin` does **not** check `auth_method` — the lookup is by
phone number, so finding the account row is itself proof of the phone credential.

**Note:** Phone login requires OTP verification on every login (passwordless — no stored password). This matches the industry standard for phone auth (Uber, WhatsApp, etc.).

### Token Refresh

**Client:** `app/lib/services/auth_state.dart`, `app/lib/core/utils/rpc_utils.dart`
**Server:** `server/services/login/refresh.go` (`RefreshToken`)

Access tokens are short-lived (1 hour). When they expire, the client obtains a new access token using the refresh token without requiring the user to log in again.

**Client-side refresh happens in three ways:**

1. **Transparent retry**: When any RPC call returns an `unauthenticated` error, `RpcUtils.executeRpc` automatically calls `refreshAccessToken()`, then retries the original call. All concurrent 401 errors await the same in-flight refresh to prevent rotation races. If the refresh itself fails, the user is logged out.

2. **Proactive refresh**: After login or token refresh, a timer fires 5 minutes before the access token expires. This avoids any mid-session expiry under normal usage.

3. **App resume**: When the app returns to the foreground (`didChangeAppLifecycleState`), the token expiry is checked. If it expires within 5 minutes, a refresh is triggered immediately.

**Server-side token rotation:**

Each `RefreshToken` RPC call:
1. Looks up the stored token hash (tokens are never stored raw — only their SHA-256 hash)
2. Checks revocation status — a revoked token being presented signals token theft: all of the user's refresh tokens are immediately revoked, forcing a full re-login
3. Verifies the token has not expired (90-day lifetime)
4. Revokes the presented token
5. Issues a new access token (1 hour) and a new refresh token (90 days)
6. Returns both tokens

**Token origin stamping (#2665):** every stored `RefreshToken` row
carries a `RefreshTokenOrigin` (`origin` field, server-only): the
interactive flows (email/password login, OIDC login, phone-OTP login,
and all registration paths) mint `INTERACTIVE` tokens, while the
`RefreshToken` RPC mints `ROTATION` tokens. Metrics (e.g. the ops
activity digest's "signed in interactively" count) use this to
distinguish people signing in from automatic credential refresh. Rows
predating the field read as `UNSPECIFIED` and are treated as unknown
until they age out with token expiry.

**Client-side token storage:**

Refresh tokens are stored in encrypted platform storage via `flutter_secure_storage` (`app/lib/services/auth_state.dart`): Keychain on iOS, EncryptedSharedPreferences on Android. Access tokens are stored in `SharedPreferences`.

### Auth Method Enforcement

Authentication methods are **additive**, not mutually exclusive. An account's
`auth_method` field records the *original* method it was created with (used for
analytics, display, and the registration path) — it is no longer an exclusive
lock on how the account signs in. The credentials live in independent columns
(`password_hash`, `oidc_provider_subject`, `phone_number`) that can coexist on
one row, so an account can hold more than one.

Each login RPC accepts a sign-in **when the account holds that method's
credential**:

- `EmailLogin` on the **code path** accepts any account with a verified email —
  no `auth_method` check (#2571). On the legacy **password path** it still
  requires `auth_method == EMAIL_PASSWORD`, as does `RequestPasswordReset`; both
  leave with the password path.
- `OIDCLogin` requires `auth_method` to match the provider.
- `PhoneLogin` accepts **any** account with a verified `phone_number` — it does
  not check `auth_method` (the lookup is by phone number, so finding the row is
  proof of the credential).

Phone (#2596) and email (#2571) are both credential-based now. Only the OIDC gate
still matches on `auth_method`; relaxing it (to enable add-OIDC to a phone-first
account) is the remaining symmetric work, along with `AddEmailAddress`, the
inverse of `AddPhoneNumber`.

**Security invariant.** Every credential column is written only by an
ownership-proving path:

- `User.phone_number` — `PhoneRegister` and `AddPhoneNumber`, both gated by a
  verified Firebase phone-OTP token; `SaveUser` cannot set a phone.
- `User.email_verified_at_unix_sec` — a completed email one-time code, or an
  identity provider's `email_verified` claim. Profile edits never set it.

That is what makes the credential-based `PhoneLogin` and `EmailLogin` gates safe —
a stored phone, or a verified-email stamp, is always one the user proved they own.
A regression test (`SaveUser` cannot set a phone) guards the phone half.

### Adding a Phone Number to an Existing Account

**Client:** `app/lib/presentation/screens/auth/phone_auth_screen.dart`
(`PhoneAuthMode.attach`), entered from `ProfileEditScreen`
**Server:** `server/services/user/add_phone.go` (`UserService.AddPhoneNumber`)

An email/OIDC user can attach a verified phone number to their account as an
*additional* sign-in method — the inverse of phone-first promote-on-verify
([single_player_mode.md](single_player_mode.md) → *Claiming*). The flow:

1. From the profile-edit screen, the user taps "Add phone number".
2. `PhoneAuthScreen` runs the same Firebase phone-OTP flow as registration/login,
   in `attach` mode, obtaining a Firebase phone ID token.
3. Client calls `UserService.AddPhoneNumber` with the token. This RPC is
   **authenticated** — it lives on `UserService` (auth-wrapped), not the public
   `LoginService` — so it always targets the calling account. There is no
   `user_id` field, so a caller cannot add a phone to anyone else (no IDOR).
4. Server validates the token, then:
   - If the number already belongs to a **different** full account → reject with
     `AlreadyExists`. A full account↔account merge is out of scope (tracked in
     #2602).
   - Otherwise set `user.phone_number` (leaving `auth_method` unchanged) and
     reconcile any phone-keyed provisional placeholders for that number across
     all communities into the account, via the shared
     `provisional.PromoteByPhone` — the same path `PhoneRegister` uses.
5. The RPC returns the verified number; the client refreshes its access token so
   the next token carries the `phone_number` claim. There is no session change —
   the account and its original sign-in method are untouched; phone OTP simply
   becomes another way in, and the account is now reachable by off-app SMS.

`phone_number` is returned on `GetUser` only for a self-fetch (the same
PII-self-only rule as `email`), so the settings UI can show whether a phone is
already attached.

---

## Data Models

### User Model

**File:** `proto/ripls/models/user.proto`

Key fields for authentication:
- `auth_method`: Enum (EMAIL_PASSWORD, GOOGLE, APPLE, PHONE)
- `password_hash`: Bcrypt hash (only for EMAIL_PASSWORD users)
- `phone_number`: E.164 format string (only for PHONE users, e.g., "+15551234567")
- `oidc_provider_subject`: Provider's user ID (only for OIDC users)
- `invited_by_id`: User ID who invited this user (empty for bootstrap user)

### RefreshToken Model

**File:** `proto/ripls/models/user.proto`

Fields:
- `id`: Unique identifier
- `user_id`: The user this token belongs to
- `token_hash`: SHA-256 hex digest of the raw token (raw token never stored)
- `expires_at_unix_sec`: Expiry timestamp (90 days from issuance)
- `created_at_unix_sec`: Creation timestamp
- `is_revoked`: Whether the token has been revoked

**Key Characteristics:**
1. **Rotating**: Each use issues a new token and revokes the old one
2. **Hashed storage**: Only the hash is persisted; the raw token exists only in-transit
3. **Theft detection**: Presenting a revoked token immediately revokes all tokens for that user
4. **Multi-device**: Each login creates a separate refresh token, supporting concurrent sessions

### ShareLink Model

**File:** `proto/ripls/models/share_link.proto`

The live invitation flow persists `ShareLink` rows (a polymorphic short-link
model; the legacy `CommunityInvitationLink` message in `community.proto` still
exists but its UUID `invitation_token` field was replaced by `short_code`).
Key `ShareLink` fields:

- `id`: Unique identifier for the share link
- `short_code`: 8-char alphanumeric code (reusable)
- `inviter_id`: User who created this link
- `community_id`: Community the link is scoped to (always set)
- `target` (oneof): the discriminator — `community_invite_id`, `gear_id`, `transfer_id`, `request_id`, or `experience_id`
- `is_revoked`: Whether the link has been manually revoked
- `created_at_unix_sec`: Creation timestamp

**Key Characteristics:**
1. **One Per User-Community**: Each user can have at most one active link per community
2. **Reusable**: Token can be used by multiple people
3. **No Expiration**: Link remains valid until revoked
4. **No Email Binding**: Not tied to any specific invitee
5. **Revocable**: Can be invalidated by setting `is_revoked = true`

---

## Security Considerations

### Password Security

Applies only to the closed set of accounts created before #2864 —
`EmailRegister` no longer accepts a password, so nothing can add to it (see
*Password Registration (retired, #2864)* above).

- Bcrypt hashing with cost factor 12
- Minimum password length: 8 characters (enforced by `auth.HashPassword`,
  used by `server/cmd/seed-legacy-user` to build test fixtures)
- No password strength requirements (weak passwords allowed)

### Invitation Short Codes
- 8-char codes generated with `crypto/rand` over a 54-char confusable-free charset (`server/services/community/invitations.go`), checked for DB uniqueness before use
- No expiration (requires manual revocation if compromised)
- Reusable by design (not deleted on use)
- Server-side validation checks revocation status
- **Security Trade-off**: Links remain valid indefinitely unless revoked, making link management and revocation important

### OIDC Token Validation
- ID tokens validated server-side with provider (Google/Apple)
- Provider's public keys used for signature verification
- Subject ID stored to link account to provider

### Firebase Phone Auth
- Firebase handles SMS delivery, rate limiting, and toll fraud prevention
- Server validates Firebase ID tokens via Admin SDK (`server/auth/firebase.go`)
- Server rejects tokens where `firebase.sign_in_provider` is not `"phone"` — prevents using a Google Firebase token to impersonate a phone user
- Phone numbers stored in E.164 format with uniqueness enforced (one phone = one account)
- Phone users have no password — authentication requires OTP verification on every login
- **Account recovery limitation**: Phone-only users who lose access to their phone number must contact support. This is the same limitation as all phone-auth apps (WhatsApp, Signal, etc.)

### JWT Access Tokens and Refresh Tokens

Access tokens:
- Short-lived (1 hour), signed with server secret
- Contain `user_id`, `email` (or empty for phone users), `phone_number` (for phone users), and `role`
- Stored in `SharedPreferences` on the client

Refresh tokens:
- Long-lived (90 days), cryptographically random (32 bytes, base64url-encoded)
- Only the SHA-256 hash is stored server-side; the raw token is only ever in-transit
- Stored in encrypted platform storage (Keychain / EncryptedSharedPreferences) via `flutter_secure_storage`
- Rotating: each use issues a fresh pair and revokes the previous refresh token
- Theft detection: presenting a revoked token triggers immediate revocation of all tokens for that user, forcing re-login on all devices

See `server/auth/tokens.go` for token generation and hashing, `server/services/login/refresh.go` for the refresh RPC, and `app/lib/services/auth_state.dart` for client-side lifecycle management.

---

## Open Items and Enhancements


### Future Enhancements

1. ~~**Email Verification**~~ — done (#2571). Email sign-up/sign-in is a mailed one-time code, and `email_verified_at_unix_sec` gates cross-community provisional promotion.
2. **Multi-Factor Authentication**: Add optional 2FA
3. **Log Out Everywhere**: Revoke all refresh tokens across all devices (currently each device session is independent)
4. **Account Linking**: Multiple auth methods per account. Partially shipped — auth methods are now additive and a phone can be attached to an email/OIDC account (`AddPhoneNumber`, #2596). Remaining: adding email/OIDC to a phone-first account (relax the email/OIDC login gates to credential-presence), and merging two pre-existing full accounts (#2602).
5. **Invitation Analytics**: Track invitation link usage, acceptance rates, conversion metrics
6. **Advanced Link Options**:
   - Optional expiration dates
   - Usage limits (max number of uses)
   - Email domain restrictions
   - Multiple active links per user-community with different settings
7. **Email Invitations**: Optional email delivery system for invitation links
8. **Custom Landing Pages**: Allow customization of invitation landing page with community info/branding