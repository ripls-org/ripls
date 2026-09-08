---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: How deep linking and routing work in the Flutter app — custom myapp:// and https://app.example.com App/Universal Links, invite and password-reset entry points, app routes, and router redirect/auth logic.
  globs: [app/lib/core/router/**]
  triggers: [deep-link, routing, universal-link, app-link, invite-link, redirect, go-router]
  lens: [client, domain]
  domain: client
freshness:
  verified_commit: "041b93570"
  verified_on: "2026-08-09"
---
# Deep Linking and Routing Design

This document describes how deep linking and routing works in the Ripls Flutter app.

## URL Schemes Supported

The app supports two URL schemes for deep links:

1. **Custom scheme**: `myapp://` - handled natively by the app
2. **Web URLs**: `https://app.example.com/` - App Links (Android) / Universal Links (iOS)

## Deep Link Entry Points

### Invite Links

**Purpose**: Invite users to a community, optionally with a specific item
(gear / request / experience) to view.

Invitations are identified by an opaque **short code** (not a UUID). The
canonical web link is the `/go/<code>` short link; both `/go/<code>` and
`/invite?token=<code>` resolve to the same `/invite` handler (the `/go/:code`
route redirects to `/invite` with the code carried as the `token` query
param — see [App Routes](#app-routes)).

**URL formats**:
- Custom: `myapp://invite?token=<CODE>`
- Web (short link): `https://app.example.com/go/<CODE>`
- Web (canonical handler): `https://app.example.com/invite?token=<CODE>`

**Query parameters**:
- `token` (required): the invitation **short code** for joining the community.

The deep-link target (a specific gear / request / experience) lives on the
`ShareLink` row, resolved via `CheckInvitation` (`target_kind` / `target_id`);
the `/go/:code` redirect and the `/invite` handler carry only the `token`. Legacy
`gear_id` / `request_id` / `experience_id` query params (from the removed
`GetOrCreateInviteLink` shim) are no longer honored on the invite path (#2562).
Residual parsing on the custom-scheme, deferred-deep-link, and registration paths
is tracked by `TODO(#2644)`.

### Password Reset Links

**URL formats**:
- Custom: `myapp://reset-password?token=<TOKEN>`
- Web: `https://app.example.com/reset-password?token=<TOKEN>`

**Query parameters**:
- `token` (required): Password reset token

## App Routes

All routes are defined in `app/lib/core/router/app_router.dart`.

| Path | Screen | Auth Required | Parameters |
|------|--------|---------------|------------|
| `/` | HomeScreen | Yes | - |
| `/login` | LoginScreen | No | `from` (query, redirect destination) |
| `/register` | RegisterScreen (new-user register only) | No | `token` (short code, query) |
| `/verify-phone` | PhoneAuthScreen (phone-first guest register) | No | `token`, `gear_id` / `request_id` / `experience_id`, `rsvp`, `n`, `img` (query) |
| `/forgot-password` | ForgotPasswordScreen | No | - |
| `/reset-password` | ResetPasswordScreen | No | `token` (query) |
| `/go/:code` | — (redirect only) | No | `code` (path) → redirects to `/invite?token=<code>` |
| `/invite` | RegisterScreen (unauthenticated) or a `FutureBuilder` resolving the share link (authenticated) | Conditional | `token` (short code, query) |
| `/gear/:id` | GearScreen | Yes | `id` (path), `tab` (query) |
| `/request/:id` | RequestScreen | Yes | `id` (path), `tab` (query) |
| `/experience/:id` | ExperienceScreen | Yes | `id` (path), `tab` (query) |
| `/settings/communities/:id/restore` | RestoreCommunityScreenById | Yes | `id` (path) |
| `/event/:experienceId` | WebExperienceScreen (Flutter Web only) | No | `experienceId` (path), `rsvp`, `code` (query) |
| `/item/:gearId` | WebItemActionScreen (gear, Flutter Web only) | No | `gearId` (path), `intent`, `code`, `n`, `img` (query) |
| `/need/:requestId` | WebItemActionScreen (request, Flutter Web only) | No | `requestId` (path), `intent`, `code`, `n`, `img` (query) |

## Router Redirect Logic

The router `redirect` function in `app_router.dart` runs before every route and
handles, in order:

### 1. Firebase Auth callback URLs (mobile only)
On mobile, phone-auth reCAPTCHA redirects arrive via the `REVERSED_CLIENT_ID`
scheme (`com.googleusercontent.apps.*`), or as a `firebaseauth`/`/link` URL.
These are redirected to `/login` so the home screen doesn't fire RPCs without
auth tokens. This whole check is skipped on web (`!kIsWeb`): the web reCAPTCHA
verifier runs inline (no custom-scheme deep link) and Firebase handles its own
callback under `/__/auth/*`, which GoRouter never sees — so the mobile-specific
`firebaseauth` substring match would only risk mis-catching legitimate web
navigations.

### 2. Custom Scheme Deep Links
For `myapp://invite` URLs:
- Extracts the `token` (short code) and redirects to `/invite?token=<CODE>`.
  The deep-link target resolves from the share-link row via CheckInvitation;
  legacy item-id query params are no longer forwarded (#2562). The `to`
  destination hint IS forwarded (#2876) — it names a surface, not a target, and
  dropping it would strand an app user on the community profile while the same
  link takes a browser user to the discussion.

For `myapp://reset-password` URLs:
- Extracts `token` and redirects to `/reset-password?token=<TOKEN>`.

### 3. Authentication Check
If user is NOT authenticated and NOT going to an auth screen (`/login`,
`/register`, `/forgot-password`, `/reset-password`, `/invite` or `/go/*`, or a
`/event/*`, `/item/*`, `/need/*`, or `/group/*` web landing):
- If a deferred deep-link context exists (e.g. from the Play Install Referrer),
  route to `/invite?token=<shortCode>` instead of generic login.
- On first launch, route to `/register` (new installs need an invite code).
- Otherwise redirect to `/login?from=<encoded_original_url>`; the `from`
  parameter preserves the intended destination including query params.

### 4. Auth Screen Redirect
If user IS authenticated and going to `/login`, `/register`, or `/verify-phone`:
- On Flutter Web, if the URL carries an `experience_id`, `gear_id`,
  `request_id`, or `community_id` (the phone-first `/verify-phone` handoff),
  redirect to the matching view — `/event/<id>` (RSVP), `/item/<id>` (gear
  ExpressInterest), `/need/<id>` (request OfferToFulfill), or `/group/<id>`
  (community join, #2875) — so the post-auth handoff fires
  (`postRegistrationDestination`). The community arm is checked last: a share
  link carrying an item always also carries that item's community, and the item
  is the more specific destination.
- Otherwise redirect to `/`.

On the phone-first guest screen (`/verify-phone`), the below-the-fold "other
ways to sign in" options — Google and email — are handled **inline** on
`PhoneAuthScreen` (the email path via the shared `EmailRegisterForm`). They no
longer route-hop to `/register`, so the guest never leaves the phone-first flow
and the post-auth item handoff still fires via `_navigateToHome` (#2595).

There is no onboarding route or onboarding redirect.

## Deep Link Flows

### Flow 1: New User (Not Registered)

The `/invite` route builds `RegisterScreen` directly for unauthenticated
visitors — the register screen now hosts the invitation hero, every auth method,
and an "I already have an account" link in one place (there is no separate
landing screen). The code is passed as `shortCode`.

```
User clicks: https://app.example.com/go/<CODE>   (or /invite?token=<CODE>)
                    │
                    ▼
         /go/<CODE> redirects to /invite?token=<CODE>
                    │
                    ▼
         User not authenticated
                    │
                    ▼
         /invite route builds RegisterScreen(shortCode: CODE)
                    │
                    ▼
         RegisterScreen (invitation hero + Password / Google / Phone,
                         plus "I already have an account")
                    │
                    ▼
         On success: registration joins the community server-side
         (target resolved from the ShareLink row); client navigates to /
                    │
                    ▼
         HomeScreen displays
```

### Flow 2: Existing User, Not in Community

```
User clicks: https://app.example.com/go/<CODE>   (or /invite?token=<CODE>)
                    │
                    ▼
         /invite route builds RegisterScreen(shortCode: CODE)
                    │
                    ▼
         User taps "I already have an account" → logs in
                    │
                    ▼
         User IS authenticated now
                    │
                    ▼
         /invite route builds a FutureBuilder<InvitationCheckResult>
         (resolves the share link via shareLinkRepository.describe(token))
                    │
                    ├─ describe.isEvent → EventInviteJoinGate
                    │                     (joins community, then shows event)
                    │
                    └─ otherwise → acceptInvitationLink(shortCode)
                    │
                    ▼
         Success: postFrameCallback runs
                    │
                    ▼
         1. Refresh communities
         2. setCommunities(...)
         3. Navigate to / (home) — typed item links already routed here (#2562)
                    │
                    ▼
         HomeScreen displays
```

### Flow 3: Existing User, Already in Community

```
User clicks: https://app.example.com/invite?token=T
                    │
                    ▼
         Router receives /invite?token=T
                    │
                    ▼
         User IS authenticated (was already logged in)
                    │
                    ▼
         /invite route builds FutureBuilder<InvitationCheckResult>
                    │
                    ▼
         describe(token) → event-shaped link uses EventInviteJoinGate;
         otherwise acceptInvitationLink(shortCode)
         (Server may return "already member" or success)
                    │
                    ▼
         Success: postFrameCallback runs
                    │
                    ▼
         1. Refresh communities
         2. setCommunities(...)
         3. Navigate to / (home) — typed item links already routed here (#2562)
                    │
                    ▼
         HomeScreen displays
```

## Platform Configuration

### Android (AndroidManifest.xml)

Quoted verbatim from the shipped manifest, so the scheme and host below are this
deployment's actual values rather than placeholders. They become build-time
parameters in #2963; until then, a fork changes them here.

```xml
<!-- Custom scheme -->
<intent-filter>
    <data android:scheme="ripls" android:host="invite" />
    <data android:scheme="ripls" android:host="reset-password" />
    <data android:scheme="ripls" android:host="conversation" />
</intent-filter>

<!-- App Links (https). android:host is this deployment's apex domain. -->
<intent-filter android:autoVerify="true">
    <data android:scheme="https" android:host="example.app" android:pathPrefix="/invite" />
    <data android:scheme="https" android:host="example.app" android:pathPrefix="/go/" />
    <data android:scheme="https" android:host="example.app" android:pathPrefix="/reset-password" />
</intent-filter>
```

### iOS (Runner.entitlements)

App Links are configured via Associated Domains for `ripls.app`.

## Testing Deep Links

### Android (ADB)
```bash
# Test invite link
adb shell am start -a android.intent.action.VIEW \
  -d "https://app.example.com/invite?token=TEST"

# Test custom scheme
adb shell am start -a android.intent.action.VIEW \
  -d "myapp://invite?token=TEST"
```

### iOS (Simulator)
```bash
xcrun simctl openurl booted "https://app.example.com/invite?token=TEST"
```

## Related Files

- `app/lib/core/router/app_router.dart` - Router and route definitions
- `app/lib/presentation/screens/auth/login_screen.dart` - Login flow
- `app/lib/presentation/screens/auth/register_screen.dart` - Registration flow; also the unauthenticated `/invite` landing (hosts the invitation hero and all auth methods)
- `app/lib/presentation/screens/experience/event_invite_join_gate.dart` - Authenticated event-link join gate (EventInviteJoinGate)
- `app/lib/data/repositories/share_link_repository.dart` - Resolves a share link (`describe`) to branch by target kind
- `app/lib/services/deferred_deep_link_service.dart` - Deferred deep-link context (e.g. Play Install Referrer)
- `app/lib/services/providers.dart` - Community selection state
- `app/android/app/src/main/AndroidManifest.xml` - Android deep link config
- `app/ios/Runner/Runner.entitlements` - iOS deep link config
