---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Why Apple Sign In was removed in favor of Google-only — Android web-flow complexity, per-environment bundle IDs, and the core mismatch where Apple omits email/name from the ID token after first authorization.
  globs: [app/lib/services/oidc_service.dart, server/auth/**]
  triggers: [apple-sign-in, oidc, sign-in-with-apple, id-token, authentication]
  lens: [client, security]
  skills: [issue, audit, triage]
  domain: auth
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Apple Sign In Implementation Issues

This document summarizes the complexities encountered when attempting to implement Apple Sign In for the Ripls app. Due to these challenges, Apple Sign In support was removed in favor of Google Sign In only.

## Issues Encountered

### 1. Platform Limitations

**Android requires web authentication flow**: Unlike iOS where Apple Sign In is a native system feature, Android requires implementing a web-based OAuth flow with `webAuthenticationOptions`. This adds significant complexity:
- Requires a service ID configured in Apple Developer Console
- Needs a redirect URL endpoint on the server
- More error-prone than native authentication

**Decision**: Rather than implement the web flow, we chose to hide the Apple Sign In button on Android (iOS only).

### 2. iOS Entitlements and Bundle ID Configuration

Apple Sign In on iOS requires:
- Adding `com.apple.developer.applesignin` capability to `Runner.entitlements`
- Configuring the correct Bundle ID in Apple Developer Console
- The Bundle ID must match exactly between the app and Apple's configuration

**Complication**: We use different Bundle IDs for dev (`org.ripls.app.dev`) and prod (`org.ripls.app`). This requires:
- Separate App IDs in Apple Developer Console for each environment
- Sign in with Apple capability enabled on both App IDs
- Server configuration to validate tokens against the correct Bundle ID per environment

### 3. Server-Side Token Validation

Apple's OIDC implementation requires:
- Registering Apple as an OIDC provider with the correct Bundle ID (not a client ID like Google)
- The Bundle ID used for validation must match the app's Bundle ID exactly

### 4. Apple Only Provides User Info on First Authorization (Critical Issue)

This is the most significant issue and the primary reason for removing Apple Sign In support.

**The Problem**: Apple's ID token (JWT) only contains the user's email and name on the **first** authorization. On subsequent sign-ins:
- The ID token contains only: `sub` (user ID), `aud`, `iss`, `iat`, `exp`, `auth_time`
- No `email` claim
- No `name` claim

**Apple's Expectation**: Apps should store the email and name from the first authorization and look up users by the `sub` (subject) claim on subsequent sign-ins.

**Impact on Our Architecture**:
1. Our JWT tokens include the user's email for authentication
2. Without the email in Apple's token, we can't generate proper JWTs
3. The email IS available in the credential response (not the token) on first sign-in, but this requires:
   - Adding email/name fields to the OIDC registration request
   - Passing these from the client to the server
   - Trusting client-provided email (security concern)

**Token Claims Comparison**:

Google ID Token:
```json
{
  "sub": "google-user-id",
  "email": "user@gmail.com",
  "email_verified": true,
  "name": "User Name",
  "picture": "https://..."
}
```

Apple ID Token (after first auth):
```json
{
  "sub": "001743.xxxxx.xxxx",
  "aud": "org.ripls.app.dev",
  "iss": "https://appleid.apple.com",
  "iat": 1766781952,
  "exp": 1766868352,
  "auth_time": 1766781952,
  "nonce_supported": true
}
```

### 5. Name Handling Complexity

Even when Apple does provide the name (first authorization only):
- Name is NOT in the ID token - only in the credential response
- Name is split into `givenName` and `familyName`
- Either or both may be null/empty (user can decline to share)
- Requires client-side logic to combine name parts
- Requires passing name separately from token to server

## Alternative Approaches (Not Implemented)

### Option A: Store Email on First Auth, Lookup by Subject
- On registration: store email from credential, associate with Apple subject
- On login: lookup user by Apple subject, ignore missing email in token
- **Drawback**: Requires schema changes and different auth flow for Apple vs Google

### Option B: Trust Client-Provided Email
- Add email field to OIDC requests
- Client sends email from credential response
- Server uses client-provided email when token doesn't have it
- **Drawback**: Security concern - client could potentially spoof email

### Option C: Use Apple's Server-to-Server Notifications
- Apple can notify your server when users revoke access
- Requires additional server endpoint and webhook handling
- **Drawback**: Significant additional complexity

## Conclusion

The fundamental mismatch between Apple's "privacy-first" approach (minimal data in tokens) and our authentication architecture (email-based JWT tokens) makes Apple Sign In integration significantly more complex than Google Sign In.

Given that:
1. Google Sign In works reliably on both platforms
2. Apple Sign In would only work on iOS anyway
3. The implementation complexity is high for a single-platform feature

We decided to remove Apple Sign In support entirely.

## References

- [Apple Sign In Documentation](https://developer.apple.com/sign-in-with-apple/)
- [Apple ID Token Claims](https://developer.apple.com/documentation/sign_in_with_apple/sign_in_with_apple_rest_api/authenticating_users_with_sign_in_with_apple)
- [sign_in_with_apple Flutter Package](https://pub.dev/packages/sign_in_with_apple)
