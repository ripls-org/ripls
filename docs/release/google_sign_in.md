---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: How Google Sign-In (OIDC) is configured on Android and iOS — the web/iOS/Android OAuth clients, serverClientId audience, SHA-1 fingerprints, google-services.json / GoogleService-Info.plist fields, and common sign-in error fixes.
  globs: [app/lib/services/oidc_service.dart, app/android/**, app/ios/Runner/GoogleService-Info.plist, app/lib/core/config/environment.dart]
  triggers: [google-sign-in, oidc, oauth, server-client-id, sha-1, google-services, id-token, audience]
  lens: [client, security]
  skills: [issue, audit]
  domain: release
freshness:
  verified_commit: "2ba311708"
  verified_on: "2026-06-09"
---
# OIDC Client Configuration for Google Sign-In

This document explains how Google Sign-In (OIDC) is configured for the Ripls
app on Android and iOS, including the required OAuth clients, keystores, and
common troubleshooting steps.

## Overview

Google Sign-In requires platform-specific OAuth clients plus a shared web
client for server-side token verification:

| Platform | OAuth Client                             | Configuration File         |
| -------- | ---------------------------------------- | -------------------------- |
| Android  | Android OAuth Client (SHA-1 fingerprint) | `google-services.json`     |
| iOS      | iOS OAuth Client (bundle ID)             | `GoogleService-Info.plist` |
| Server   | Web OAuth Client                         | `GOOGLE_CLIENT_ID` env var |

The **Web OAuth Client** is critical - it's used as the `serverClientId` on
both platforms to ensure ID tokens have the correct audience for server-side
verification.

## Architecture

```text
┌─────────────────┐     ┌──────────────────────┐     ┌─────────────────┐
│  Android App    │────▶│  Google Sign-In SDK  │────▶│  Google OAuth   │
│                 │     │                      │     │  Servers        │
│  - Package name │     │  - Uses Android      │     │                 │
│  - SHA-1 finger │     │    OAuth client for  │     │  Returns:       │
│    print        │     │    authentication    │     │  - ID Token     │
│  - serverClient │     │  - Uses serverClient │     │  - User info    │
│    Id (web)     │     │    Id for ID token   │     │                 │
└─────────────────┘     └──────────────────────┘     └─────────────────┘
                                   │
                                   ▼
                        ┌──────────────────────┐
                        │  Ripls Server        │
                        │                      │
                        │  Verifies ID token   │
                        │  using Google's      │
                        │  public keys         │
                        └──────────────────────┘
```

## Required Configuration

### Google Cloud Console

Location: <https://console.cloud.google.com/apis/credentials?project=ripls-dev>

You need these OAuth 2.0 Client IDs:

| Client Type       | Purpose                        | Configuration                               |
| ----------------- | ------------------------------ | ------------------------------------------- |
| Web               | `serverClientId` for ID tokens | Used in `GOOGLE_CLIENT_ID` env var          |
| iOS               | iOS app authentication         | Bundle ID: `org.ripls.app` (release/prod)   |
| Android (Debug)   | Debug/profile build auth       | Package: `org.ripls.app.dev`, Debug SHA-1   |
| Android (Release) | Release build authentication   | Package: `org.ripls.app`, Release SHA-1     |

Debug and profile builds ship under the **`.dev` application-id suffix**
(`applicationIdSuffix = ".dev"` in `app/android/app/build.gradle.kts`), so the
package Google sees for those builds is `org.ripls.app.dev`. The release build
has no suffix and uses `org.ripls.app`. The iOS bundle ID is likewise
`org.ripls.app.dev` for debug/profile and `org.ripls.app` for release — see
[iOS dev/prod flavors](#ios-devprod-flavors) below.

### How Android and iOS Identify the App

**Android** verifies app identity using **SHA-1 certificate fingerprints** plus
the package name. Each keystore (debug vs release) produces a different
fingerprint, so Google needs an Android OAuth client registered for each one.
In practice, only the **release** `google-services.json` carries Android OAuth
clients (`client_type: 1`) — the debug/profile files (package
`org.ripls.app.dev`) ship just the web client, because debug Google Sign-In is
authorized via the debug SHA-1 registered in the `ripls-dev` Firebase project.

**iOS** is verified through the **bundle ID** rather than a certificate
fingerprint, so iOS needs one OAuth client per bundle ID — and the bundle ID
differs by flavor (`org.ripls.app.dev` for debug/profile, `org.ripls.app` for
release). Apple's code signing guarantees the app's authenticity within a
bundle ID regardless of debug-vs-release, so no per-SHA-1 registration is
needed the way Android requires.

### SHA-1 Fingerprints (Android Only)

#### Debug (Shared Keystore)

The project uses a shared debug keystore checked into source control at
`app/android/app/debug.keystore`. This ensures all developers use the same
SHA-1 fingerprint.

```bash
# Get the debug SHA-1 fingerprint
keytool -list -v -keystore app/android/app/debug.keystore \
  -alias androiddebugkey -storepass android | grep SHA1
```

**Current Debug SHA-1:** `E1:8B:48:51:2F:70:AE:D2:80:C8:39:08:70:79:84:97:93:16:74:3A`

#### Release

The release keystore is stored securely (not in source control). The SHA-1 is
registered in Google Cloud Console.

```bash
# Get the release SHA-1 fingerprint (requires access to release keystore)
keytool -list -v -keystore /path/to/upload-keystore.jks \
  -alias upload -storepass <password> | grep SHA1
```

**Current Release SHA-1:** `3B:43:64:A3:CD:BB:3E:B6:EB:02:45:AB:07:C0:32:9E:9B:D9:90:D9`

### Environment Configuration

The web client ID is passed to the app via environment variables, with
**separate values per environment** (dev vs prod Firebase projects):

```json
// env.dev.json
{ "GOOGLE_CLIENT_ID": "937892531321-shvr2m51jmdbqqjaml4f5ueic3esmbr3.apps.googleusercontent.com" }

// env.prod.json
{ "GOOGLE_CLIENT_ID": "507242812633-4v6q4h50h4ksshoui4o6vg5e18jcvsbj.apps.googleusercontent.com" }
```

This is read by `Environment.googleClientId`
(`app/lib/core/config/environment.dart`) and used in
`OIDCService._ensureInitialized()` to configure the `serverClientId` for
obtaining ID tokens.

### google-services.json (Android)

Downloaded from Firebase Console, this file contains:

- `client_type: 3` entry for the Web OAuth client (every build type)
- `client_type: 1` entries for Android OAuth clients, one per registered SHA-1
  — present in the **release** file (the upload-key and Google Play
  app-signing fingerprints); the debug/profile files carry only the web client.

Location: per build type, under
`app/android/app/src/{debug,profile,release}/google-services.json`. The Gradle
build selects the file matching the build type (the `debug`/`profile` variants
register the `org.ripls.app.dev` package in the `ripls-dev` project, the
`release` variant registers the `org.ripls.app` package with the release
SHA-1).

To update:

1. Go to [Firebase Console](https://console.firebase.google.com/project/ripls-dev/settings/general)
2. Download google-services.json
3. Replace the file under the matching `app/android/app/src/<buildType>/` directory

### iOS dev/prod flavors

iOS selects its bundle ID and OAuth client IDs per build flavor through
xcconfig includes (`app/ios/Flutter/`): `Debug.xcconfig` and `Profile.xcconfig`
`#include "Dev.xcconfig"`; `Release.xcconfig` `#include "Prod.xcconfig"`. The
flavor files set:

| Setting                            | Dev (`Dev.xcconfig`) | Prod (`Prod.xcconfig`) |
| ---------------------------------- | -------------------- | ---------------------- |
| `PRODUCT_BUNDLE_IDENTIFIER`        | `org.ripls.app.dev`  | `org.ripls.app`        |
| `GOOGLE_IOS_CLIENT_ID`             | `685679309377-…`     | `507242812633-…`       |
| `GOOGLE_IOS_REVERSED_CLIENT_ID`    | `com.googleusercontent.apps.685679309377-…` | `com.googleusercontent.apps.507242812633-…` |

The committed `app/ios/Runner/GoogleService-Info.plist` is the **dev** file
(`BUNDLE_ID = org.ripls.app.dev`, `CLIENT_ID = 685679309377-…`); the prod plist
is supplied by the release pipeline.

### GoogleService-Info.plist (iOS)

Downloaded from Firebase Console, this file contains these key fields:

| Key                  | Value               | Purpose                                                   |
| -------------------- | ------------------- | --------------------------------------------------------- |
| `CLIENT_ID`          | iOS OAuth client ID | iOS app authentication                                    |
| `REVERSED_CLIENT_ID` | Reversed client ID  | URL scheme for OAuth callback                             |
| `SERVER_CLIENT_ID`   | Web OAuth client ID | **Critical:** Sets token audience for server verification |

Location: `app/ios/Runner/GoogleService-Info.plist`

**Important:** Firebase Console downloads do **not** include `SERVER_CLIENT_ID`,
and the committed plist currently omits it. If iOS sign-in fails server-side
token validation with an audience mismatch, add it manually:

```xml
<key>SERVER_CLIENT_ID</key>
<string>937892531321-shvr2m51jmdbqqjaml4f5ueic3esmbr3.apps.googleusercontent.com</string>
```

The `SERVER_CLIENT_ID` must match the Web OAuth client ID used as
`GOOGLE_CLIENT_ID` in the environment configuration for the same flavor.

### iOS URL Scheme

`app/ios/Runner/Info.plist` registers the OAuth-callback URL scheme through a
build variable that resolves per flavor — it is **not** hardcoded:

```xml
<key>CFBundleURLSchemes</key>
<array>
    <string>$(GOOGLE_IOS_REVERSED_CLIENT_ID)</string>
</array>
```

`GOOGLE_IOS_REVERSED_CLIENT_ID` is defined in `Dev.xcconfig` / `Prod.xcconfig`
(see the flavor table above) and must equal the `REVERSED_CLIENT_ID` of the
matching iOS OAuth client. `Info.plist` also registers a static `ripls` scheme
for the app's own deep links, separate from the Google callback scheme.

## Troubleshooting

### Error: "[16] Account reauth failed"

**Symptom:** User selects Google account, then is silently returned to the
registration screen with no error (or error code 16).

**Cause:** Missing or misconfigured Android OAuth client in Google Cloud
Console. The app's SHA-1 fingerprint doesn't match any registered client.

**Solution:**

1. Get the SHA-1 fingerprint of the keystore being used:

   ```bash
   # For debug builds
   keytool -list -v -keystore app/android/app/debug.keystore \
     -alias androiddebugkey -storepass android | grep SHA1
   ```

2. Go to [Google Cloud Console Credentials](https://console.cloud.google.com/apis/credentials?project=ripls-dev)
3. Check if an Android OAuth client exists with that SHA-1
4. If not, create one:
   - Click **+ CREATE CREDENTIALS** → **OAuth client ID**
   - Application type: **Android**
   - Package name: `org.ripls.app.dev` for debug/profile builds, or
     `org.ripls.app` for release builds (the `.dev` suffix comes from
     `applicationIdSuffix` in `build.gradle.kts`)
   - SHA-1 certificate fingerprint: (paste from step 1)
5. Re-download `google-services.json` from Firebase Console (place it in the
   matching `app/android/app/src/<buildType>/` directory)
6. Rebuild the app

### Error: "Failed to get ID token from Google"

**Symptom:** Google Sign-In appears to work but no ID token is returned.

**Cause:** The `serverClientId` (web client ID) is not configured or doesn't
match what's in Google Cloud Console.

**Solution:**

1. Verify `GOOGLE_CLIENT_ID` is set in your env file
2. Verify the value matches the Web OAuth client ID in Google Cloud Console
3. Rebuild with `--dart-define-from-file=env.dev.json`

### Error: "token validation failed: expected audience X got Y" (iOS)

**Symptom:** iOS Google Sign-In completes but server rejects the token with an
audience mismatch error.

**Cause:** `GoogleService-Info.plist` is missing `SERVER_CLIENT_ID`. Without
this, iOS sends tokens with the iOS client ID as the audience instead of the
web client ID that the server expects.

**Solution:**

1. Open `app/ios/Runner/GoogleService-Info.plist`
2. Add the `SERVER_CLIENT_ID` key with the web client ID:

   ```xml
   <key>SERVER_CLIENT_ID</key>
   <string>937892531321-shvr2m51jmdbqqjaml4f5ueic3esmbr3.apps.googleusercontent.com</string>
   ```

3. Rebuild the iOS app

### Error: "INSTALL_FAILED_UPDATE_INCOMPATIBLE"

**Symptom:** App fails to install with signature mismatch error.

**Cause:** The app is already installed on the device with a different signing
key (e.g., old debug keystore vs new shared debug keystore).

**Solution:** (debug/profile builds install under the `.dev` suffix, so
uninstall `org.ripls.app.dev`)

```bash
adb uninstall org.ripls.app.dev
flutter run --dart-define-from-file=env.dev.json
```

### Google Sign-In works on one machine but not another

**Cause:** Each developer's machine has a different debug keystore at
`~/.android/debug.keystore` with a different SHA-1 fingerprint.

**Solution:** Use the shared debug keystore. The project is configured to use
`app/android/app/debug.keystore` for debug builds. If a developer still has
issues:

1. Verify `app/android/app/build.gradle.kts` has the debug signing config
2. Uninstall the app: `adb uninstall org.ripls.app.dev` (debug/profile builds
   use the `.dev` suffix)
3. Rebuild

## Code References

- **OIDC Service:** `app/lib/services/oidc_service.dart`
  - `_ensureInitialized()` - Calls `GoogleSignIn.instance.initialize(serverClientId:)`
  - `signInWithGoogle()` - Handles sign-in flow; maps `GoogleSignInException`
    code 16 ("reauth failed") to a surfaced configuration error
- **Environment Config:** `app/lib/core/config/environment.dart`
  - `googleClientId` getter (reads `GOOGLE_CLIENT_ID` dart-define)

**Android:**

- **Gradle Config:** `app/android/app/build.gradle.kts` - Signing configs
- **Debug Keystore:** `app/android/app/debug.keystore` (password: `android`)
- **Google Services:** `app/android/app/src/{debug,profile,release}/google-services.json` (per build type)

**iOS:**

- **Google Services:** `app/ios/Runner/GoogleService-Info.plist` (committed file
  is the dev flavor)
- **URL Schemes:** `app/ios/Runner/Info.plist` (`CFBundleURLSchemes` resolves
  `$(GOOGLE_IOS_REVERSED_CLIENT_ID)`)
- **Flavor config:** `app/ios/Flutter/Dev.xcconfig` / `Prod.xcconfig` (bundle ID
  + iOS client IDs), included by `Debug.xcconfig` / `Profile.xcconfig` /
  `Release.xcconfig`

## Adding a New Developer

When a new developer joins:

1. They clone the repo (shared debug keystore is included)
2. No additional configuration needed for debug builds
3. Google Sign-In should work immediately

If they need to test release builds, they need access to the release keystore
(stored in GitHub Secrets as `ANDROID_KEYSTORE_BASE64`).

## Checklist for New Projects

When setting up Google Sign-In for a new project:

**Google Cloud Console:**

- [ ] Create Firebase project
- [ ] Create Web OAuth client (for server-side verification)
- [ ] Create iOS OAuth client (bundle ID)
- [ ] Create Android OAuth client(s) (package name + SHA-1 fingerprints)

**Android:**

- [ ] Download `google-services.json` from Firebase
- [ ] Create shared debug keystore and check into source control
- [ ] Configure gradle to use shared debug keystore
- [ ] Register debug and release SHA-1 fingerprints

**iOS:**

- [ ] Download `GoogleService-Info.plist` from Firebase
- [ ] Add `SERVER_CLIENT_ID` key with web client ID
- [ ] Verify URL scheme in `Info.plist` matches `REVERSED_CLIENT_ID`

**App Configuration:**

- [ ] Set `GOOGLE_CLIENT_ID` environment variable to web client ID
