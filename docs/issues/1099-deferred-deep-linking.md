# Issue #1099: Share/invite link context is lost when recipient has to install the app first

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1099
**Filed:** 2026-04-12 | **Last activity:** 2026-04-13
**Labels:** bug | **Assignees:** briangbrown
**Context docs:** none

## Problem Statement

When a user taps a Ripls share link (community invite or item deep link) and
doesn't have the app installed, they're sent to the App Store or Play Store.
After install, the app launches with no knowledge of the original link — the
invite token or item context is lost. The user lands on a generic first-run
screen instead of the shared content.

This breaks the core sharing loop. New users — the highest-value recipients of
a share — get the worst onboarding experience instead of the best.

The root cause is straightforward: `website/content/invite.html` links to the
stores without any referrer mechanism. The short code lives only in the URL
that brought the user to the landing page; once they leave for the store, it's
gone.

Related: #1071 (Firebase reports `ripls.app/invite.*` and `/reset-password.*`
as misconfigured Android deep links).

## Context from Comments

Three comments, all from the two maintainers:

- **Brian (comment 1 — Android research):** The Google Play Install Referrer
  API is the recommended Android solution. It's deterministic: the landing page
  encodes the short code into a `referrer` query param on the Play Store URL,
  Play Store bakes it into the install, and the app reads it on first launch
  via the Install Referrer library. 90-day retention, no third-party SDK.

- **Brian (comment 2 — iOS research):** Apple has no Install Referrer
  equivalent. The proposed approach is a three-layer fallback:
  1. **Clipboard handoff** (primary): landing page copies `ripls://invite?token=...`
     to clipboard; app reads `UIPasteboard` on first launch; iOS 16+ shows a
     paste permission banner. This is what Branch's NativeLink does under the
     hood.
  2. **Universal Links re-tap** (fallback): the Safari tab is still open after
     install; a "Tap here to open" CTA triggers Universal Links now that the
     app is installed.
  3. **Manual code entry** (last resort): a "Have an invite code?" field on
     the first-run screen.

- **Bryan:** Expressed interest in picking this up if Brian isn't planning to.

## Potential Solutions

### Option A: DIY per-platform (Install Referrer + Clipboard)

- **Approach:** Android uses the Play Install Referrer API. iOS uses clipboard
  handoff as primary, Universal Links re-tap as fallback, manual entry as last
  resort. No third-party attribution SDK.
- **Pros:** No external dependency. Free. Deterministic on Android. Clipboard
  approach on iOS is well-understood (Branch uses it). All three iOS fallback
  layers mean high recovery rate.
- **Cons:** Two completely different implementations to build and maintain. iOS
  clipboard approach requires user to approve paste banner (iOS 16+). If user
  copies something else between tapping the link and opening the app, clipboard
  is lost.
- **Effort:** Medium.

### Option B: Branch.io / third-party attribution

- **Approach:** Use Branch.io (or Adjust, AppsFlyer) for deferred deep linking.
  These services handle the install attribution problem across both platforms
  with a single SDK.
- **Pros:** Battle-tested. Handles edge cases (fingerprint matching, cross-device).
  Single API for both platforms. Dashboard for link analytics.
- **Cons:** Paid dependency (~$0/mo for free tier up to 10K MAU, then pricing
  jumps). Adds a third-party SDK and data dependency. Goes against the project's
  preference for self-hosted, no-vendor-lock-in solutions. Still uses clipboard
  under the hood on iOS.
- **Effort:** Small-medium.

### Option C: Server-side fingerprint matching

- **Approach:** When the landing page loads, record the user's IP + user agent +
  timestamp server-side. On first app launch, send the same fingerprint to the
  server and look for a matching recent visit with a short code.
- **Pros:** Works on both platforms without platform-specific APIs. No clipboard
  permissions needed.
- **Cons:** Non-deterministic — multiple devices on the same network (home WiFi)
  produce false matches. Privacy concerns with IP tracking. Fragile with VPNs
  and carrier-grade NAT. Not reliable enough for a critical onboarding flow.
- **Effort:** Medium.

## Recommendation

**Option A: DIY per-platform.** The research in the issue comments is thorough
and the approaches are well-understood. Android's Install Referrer API is
Google's official replacement for Dynamic Links — it's deterministic, free,
and requires minimal code. iOS clipboard handoff is what Branch itself uses
under the hood, so we get the same mechanism without the SDK dependency. The
three-layer iOS fallback (clipboard → re-tap → manual entry) provides high
recovery rates even when the primary mechanism fails.

Branch.io would be faster to implement but adds a paid dependency for something
we can build in ~200 lines of platform code total.

## Open Questions

- **Should item deep links (gear, experience, request) also use deferred deep
  linking, or just community invites?** — Recommendation: Yes, both. The issue
  explicitly lists both link types in scope, and the mechanism is the same — the
  short code or item ID is what gets preserved through install. The landing page
  already handles both; the deferred path should too.

- **Should the "Have an invite code?" field appear on Android too?** —
  Recommendation: Yes. Even though Install Referrer is reliable, a manual
  fallback costs almost nothing to implement and covers edge cases (sideloaded
  APKs, alternative stores without referrer support).

- **Does the iOS app have Associated Domains configured for Universal Links?**
  — Research shows `Info.plist` has the custom `ripls://` scheme but no
  Associated Domains entitlement was found. This needs to be verified and likely
  set up for the re-tap fallback to work. The entitlement and
  `apple-app-site-association` file on `ripls.app` may already exist (the
  Android `assetlinks.json` equivalent) but should be confirmed.

- **Should we handle the desktop-to-mobile handoff case?** — Recommendation:
  Not in this issue. Desktop users who click a share link see the landing page
  with store buttons; there's no reliable cross-device attribution without a
  third-party service. The manual code entry fallback partially covers this.

- **How does this interact with #1071 (misconfigured deep links)?** —
  Recommendation: Fix #1071 as part of Phase 1 setup, since we'll be touching
  the same configuration files. The misconfigured deep link warnings in Firebase
  likely stem from the same gaps we're addressing here.

## Implementation Plan

### Phase 1: Landing page and shared infrastructure

**Goal:** Update the landing page to preserve context for both platforms, and
add the manual code entry fallback to the app's first-run flow.

**Tasks:**
- [x] Update `website/content/invite.html` to encode the short code in the
  Play Store URL as a referrer parameter:
  `https://play.google.com/store/apps/details?id=org.ripls.app&referrer=utm_source%3Dripls%26utm_content%3D{code}`
- [x] Update `website/content/invite.html` to copy the deep link URL to
  clipboard on iOS via `navigator.clipboard.writeText('ripls://invite?token={code}')`
  before redirecting to the App Store
- [x] Add a visible "Installed? Tap here to open in Ripls" CTA on the landing
  page that links via the Universal Link (`https://ripls.app/go/{code}`) for
  users who return to Safari after install
- [x] Display the short code prominently on the landing page so users can
  copy/type it manually
- [x] Add a "Have an invite code?" input field on the registration screen
  (`app/lib/presentation/screens/auth/register_screen.dart`) — when submitted,
  resolve the code via existing `CheckInvitation` RPC and proceed with the
  normal invite flow
- [x] Add i18n strings for all new user-visible text to both `app_en.arb` and
  `app_es.arb`: "Have an invite code?" field label/placeholder, any validation
  messages, and the landing page "Installed? Tap here" CTA. Use `context.l10n`
  in all widgets — no hardcoded strings
- [x] Verify/fix #1071 (Firebase deep link misconfiguration) while touching
  the Android manifest and iOS config:
  - Fixed `assetlinks.json`: wrong package name (`com.ripls.gear` → `org.ripls.app`),
    added release and debug SHA256 fingerprints, separate entries for prod and
    dev package names
  - Fixed `apple-app-site-association`: wrong bundle ID (`com.ripls.gear` →
    `org.ripls.app`), added real Team ID (`2HL54765FF`)
  - Added `/go/` pathPrefix to Android manifest App Links intent filter

**Validates:** Landing page encodes context for both platforms. Manual fallback
works end-to-end. #1071 warnings cleared.

### Phase 2: Android — Play Install Referrer

**Goal:** Android users who install after tapping a share link are routed to
the shared content on first launch.

**Tasks:**
- [x] Add the `play_install_referrer` package to `app/pubspec.yaml`
  (the `android_play_install_referrer` package is discontinued; its
  replacement is `play_install_referrer`)
- [x] Create a deferred deep link service
  (`app/lib/services/deferred_deep_link_service.dart`) that:
  - On Android first launch, calls the Install Referrer API
  - Parses the `utm_content` parameter from the referrer string
  - Returns the raw extracted context (short code, item IDs) — the service
    only handles platform API interaction. Resolution happens in the ViewModel
    layer via existing flows
- [x] Create Riverpod providers (`deferredDeepLinkServiceProvider`,
  `deferredDeepLinkContextProvider`) for dependency injection and testability
- [x] Wire the service into the app's initialization flow
  (`app/lib/main.dart`) — after auth state is determined but before the
  initial route is selected, for unauthenticated users only
- [x] Update `app/lib/core/router/app_router.dart` to check deferred deep
  link context in the redirect logic — routes unauthenticated users to
  `/invite?token=CODE` instead of `/login` when referrer context is present
- [x] Handle the case where the user needs to register first: the redirect
  sends the user through the existing `/invite` flow, which shows
  `InvitationLandingScreen` for unauthenticated users and routes to
  `/register?token=CODE` on sign-up, preserving the short code
- [x] Add unit tests for referrer string parsing (17 tests covering valid
  referrer strings, missing utm_content, malformed strings, empty input,
  URL-encoded values, non-ripls sources, extra parameters)
- [x] Update landing page `buildPlayStoreUrl()` to encode item IDs
  (gear_id, request_id, experience_id) in the referrer string alongside
  the invite code
- [x] Add tests for the `DeferredDeepLinkContextNotifier` (set/consume/reset)
  and verify integration with the existing `registerProvider` tests (the
  deferred flow feeds into the same `/invite` → `/register?token=CODE` path
  that is already covered by `register_view_model_test.dart`)
- [ ] Test end-to-end: tap link → Play Store → install → open → land on
  shared content

**Validates:** Android deferred deep linking works for both invite and item
links. Referrer string is correctly parsed and resolved.

### Phase 3: iOS — Clipboard handoff and Universal Links re-tap

**Goal:** iOS users who install after tapping a share link are routed to the
shared content on first launch.

**Tasks:**
- [x] Verify that Associated Domains entitlement is configured for
  `applinks:ripls.app` in the iOS project — already configured in
  `app/ios/Runner/Runner.entitlements` and referenced in all build
  configurations (Debug, Profile, Release)
- [x] Extend the deferred deep link service from Phase 2 to handle iOS:
  - Reads clipboard via Flutter's `Clipboard.getData` (which reads
    `UIPasteboard` on iOS) — no method channel or native code needed
  - Parses the `ripls://invite?token=...` URL for token and item IDs
  - Returns the raw extracted context (no RPC calls in the service layer)
  - Clears the clipboard after successful read (good hygiene)
- [x] Handle the iOS paste permission banner (iOS 16+) — the system
  automatically shows "Ripls would like to paste from Safari" when
  `Clipboard.getData` is called. Only triggers on first launch for
  unauthenticated users (gated by SharedPreferences consumed flag and
  auth state check in `main.dart`)
- [x] Verify the Universal Links re-tap path: the landing page CTA links to
  `https://ripls.app/go/{code}`, Associated Domains entitlement has
  `applinks:ripls.app`, and the AASA file is served at
  `https://ripls.app/.well-known/apple-app-site-association` with the
  correct Team ID and paths including `/go/*`
- [x] Add unit tests for clipboard URL parsing (13 tests covering valid URLs,
  wrong scheme/host, missing token, empty input, plain text, whitespace)
- [ ] Test end-to-end: tap link → App Store → install → open → paste banner →
  land on shared content
- [ ] Test fallback: clipboard lost → return to Safari → tap CTA → land on
  shared content
- [ ] Test last resort: both fail → type code on registration screen → join
  community

**Validates:** iOS deferred deep linking works with all three fallback layers.
Paste permission is handled gracefully. Universal Links re-tap works.

## Manual Test Plan

### Prerequisites

- A valid invite short code (create one from an existing community)
- A second device or someone to send you the link (or text yourself)
- Access to `https://ripls.app/go/{CODE}` (prod) or
  `https://server-dev.ripls.org/go/{CODE}` (dev)

### A. Landing Page (both platforms, use mobile browser)

- [x] **A1. Invite code display:** Open `https://ripls.app/go/{CODE}` in
  mobile Safari/Chrome. Verify the invite code is displayed prominently with
  "Your invite code" label and helper text.
- [x] **A2. Platform-specific store buttons:** On iOS only the
  TestFlight/App Store button is visible. On Android only the Play Store
  button is visible.
- [x] **A3. Re-tap CTA:** After ~2 seconds, "Already installed? Tap here to
  open in Ripls" appears. The link points to `https://ripls.app/go/{CODE}`.
- [x] **A4. Play Store referrer (Android):** Long-press the Play Store button
  and copy the link. Verify the URL contains
  `&referrer=` with `utm_source%3Dripls%26utm_content%3D{CODE}`.

### B. Android — Install Referrer Path

> The Install Referrer API only works for Play Store installs, not debug
> APKs. Tests B1 requires uploading to a Play Store testing track. Tests
> B2–B3 can be run with debug builds.

- [x] **B1. Happy path — code recovered via referrer:**
  Uninstall the app. Open `https://ripls.app/go/{CODE}` in Chrome. Tap Play
  Store button. Install from the Play Store. Open the app. **Expected:** App
  launches → invitation landing screen shows community name and inviter →
  tap "Sign Up" → registration screen shows invitation banner.
- [x] **B2. Debug/sideload fallback — code lost, first launch:**
  Uninstall the app (`adb shell pm clear org.ripls.app.dev && adb uninstall
  org.ripls.app.dev`). Install via
  `flutter run --debug --dart-define-from-file=env.local.json`. **Expected:**
  App launches → registration screen (not login) → shows "Have an invite
  code?" with "Tap your invite link again, or enter the code below." Re-tap
  the invite link → app opens via App Links and shows invitation flow. OR
  type the code manually → tap "Apply Code" → invitation banner appears.
- [x] **B3. Second launch — returns to login:**
  After B2, close and reopen the app without registering. **Expected:** App
  launches → login screen (not registration). First-launch redirect only
  fires once.

### C. iOS — Clipboard Handoff Path

- [x] **C1. Happy path — code recovered via clipboard:**
  Uninstall the app (`xcrun simctl uninstall booted org.ripls.app.dev`).
  Open `https://ripls.app/go/{CODE}` in Safari. Tap the App Store button
  (landing page copies deep link to clipboard). Install the app. Open it.
  **Expected (iOS 16+):** System paste banner "Ripls would like to paste
  from Safari" → tap Allow → invitation landing screen shows community name
  and inviter. **If denied:** Falls through to registration screen with
  "Tap your invite link again, or enter the code below."
- [x] **C2. Clipboard overwritten — code lost:**
  Uninstall the app. Open the invite link in Safari. Tap the App Store
  button. **Before opening the app**, copy some other text. Open the app.
  **Expected:** Registration screen with "Tap your invite link again, or
  enter the code below."
- [x] **C3. Universal Links re-tap fallback:**
  After C2, go back to Safari — the landing page tab is still open. Tap
  "Already installed? Tap here to open in Ripls." **Expected:** App opens
  via Universal Links → invitation landing screen with community details.
- [x] **C4. Manual code entry fallback:**
  On the registration screen (no invite context), type the invite code in
  the text field. Tap "Apply Code." **Expected:** Invitation banner appears
  with community name → can proceed to register.
- [x] **C5. Second launch — returns to login:**
  Close and reopen the app without registering. **Expected:** Login screen,
  not registration.

### D. Invalid/Edge Cases (both platforms)

- [x] **D1. Invalid invite code:** On the registration screen, enter a fake
  code like `XXXXXX`. Tap "Apply Code." **Expected:** Error message "Invalid
  invite code. Please check and try again."
- [x] **D2. Revoked invite code:** Create an invite link, revoke it, then use
  the code. **Expected:** Error shown on landing screen or registration
  screen.
- [x] **D3. Existing user with app installed:** An existing user taps an
  invite link. **Expected:** App opens via App Links / Universal Links →
  accepts invitation automatically → routes to home or item.
- [x] **D4. Invite link with item deep link:** Open
  `https://ripls.app/go/{CODE}?gear_id={GEAR_ID}` on a device without the
  app. Install and open. **Expected:** Same invite flow, but after
  registration and invite acceptance, routes to the gear detail screen.

### E. Well-Known Files Verification

- [x] **E1. Android assetlinks.json:** Visit
  `https://digitalassetlinks.googleapis.com/v1/statements:list?source.web.site=https://ripls.app&relation=delegate_permission/common.handle_all_urls`.
  **Expected:** Two entries — `org.ripls.app` (prod) and `org.ripls.app.dev`
  (dev) with correct SHA256 fingerprints.
- [x] **E2. iOS apple-app-site-association:**
  `curl -s https://ripls.app/.well-known/apple-app-site-association`.
  **Expected:** JSON with `appID: "2HL54765FF.org.ripls.app"` and paths
  including `/go/*`.

## Audit Notes

The following changes were made during architectural review:

1. **Architecture layer violation (Phases 2 & 3):** The deferred deep link
   service originally "resolved codes via existing RPCs." Per client
   architecture rules, services must not call repositories or other API
   services. The service now only extracts raw context (short code, item IDs)
   from platform APIs. Resolution happens in the ViewModel layer through the
   standard Repository → Service flow.

2. **Missing Riverpod provider (Phase 2):** Added
   `deferredDeepLinkServiceProvider` for dependency injection. Per architecture
   docs, all services are accessed through Riverpod providers — never
   instantiated directly.

3. **i18n compliance (Phase 1):** Added task to create ARB entries in both
   `app_en.arb` and `app_es.arb` for all new user-visible strings ("Have an
   invite code?" field, validation messages). Per i18n docs, every user-visible
   string must use `context.l10n` and exist in both locales.

4. **Testing gaps (Phase 2):** Added ViewModel-level tests with a mocked
   `DeferredDeepLinkService` to verify resolution and navigation flow. Per
   architecture docs, ViewModels should be tested with fake/mock
   implementations of their dependencies.

5. **No server-side changes needed.** Existing `CheckInvitation` and
   `AcceptInvitationLink` RPCs already handle short-code resolution. The
   deferred deep link feature is purely client-side (landing page + app
   platform code).

6. **No caching implications.** Deferred deep link context is read once on
   first launch and consumed — it doesn't need caching. The subsequent
   invitation resolution uses existing cached repository flows.

## Related Issues

- #1071 — Firebase reports `ripls.app/invite.*` and `/reset-password.*` as
  misconfigured Android deep links; should be resolved in Phase 1
- #135 — Referenced in the issue body as related to item-lookup RPCs for
  rehydrating context from a recovered token
