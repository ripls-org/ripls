# Issue #1027: [Bug Report] No notifications on iOS

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1027
**Filed:** 2026-04-04 | **Last activity:** 2026-04-04
**Labels:** bug, user-feedback | **Assignees:** tescobar
**Context docs:** docs/push_notifications.md, docs/client/chat.md

## Problem Statement

iOS users are not receiving push notifications. The report was filed by
user2@example.org from an Android device (Pixel 9 Pro, app v0.9.2), indicating
he observed that iOS users in his community are not getting notified. Android
notifications appear to be working normally.

The push notification pipeline has multiple iOS-specific configuration points
that could independently cause this failure. Investigation reveals several
concrete issues in the server APNs payload, the iOS entitlements file, and
missing APNs configuration in Firebase.

## Context from Comments

No comments on this issue. The report is brief — "iOS folks seem not to be
getting notifications." This needs confirmation from actual iOS users about
their experience (never received any? stopped receiving? intermittent?).

## Root Cause Analysis

Investigation of the full notification pipeline (server → FCM → APNs → iOS
device) revealed the following issues:

### Finding 1: `aps-environment` hardcoded to `development`

**File:** `app/ios/Runner/Runner.entitlements:5-6`

The entitlements file sets `aps-environment` to `development`. For TestFlight
and App Store distribution, the APNs environment must be `production`. While
the distribution provisioning profile can override this during signing, having
the wrong value in the entitlements file is a known source of token mismatch
issues — the app requests a development APNs token, but FCM sends via the
production APNs gateway (or vice versa), causing silent delivery failures.

### Finding 2: Missing APNs `badge` count in server payload

**File:** `server/notifications/fcm/provider.go:90-101`

The iOS APNs config sets `Alert` and `Sound` but omits the `Badge` field.
While this alone shouldn't prevent notification display, some iOS versions
and notification configurations behave inconsistently without a badge count.

### Finding 3: No `content-available` flag for background delivery

**File:** `server/notifications/fcm/provider.go:90-101`

The APNs payload lacks `ContentAvailable: true`. Without this, iOS won't wake
the app for background processing when a notification arrives. This primarily
affects background data refresh, not notification display itself, but is
relevant for the chat system's real-time updates.

### Finding 4: APNs authentication key may not be configured in Firebase

**File:** `docs/push_notifications.md` (Step 4 in Firebase Project Setup)

The documentation describes the APNs key upload as a required step, and the
troubleshooting table lists "Auth error from APNS" as a known issue when this
isn't done. There's no way to verify this from code — it requires checking the
Firebase Console under Project Settings → Cloud Messaging → Apple app
configuration. If the .p8 key is not uploaded, FCM cannot deliver to any iOS
device.

### Finding 5: Background handler is a no-op

**File:** `app/lib/services/fcm_service.dart:334-339`

The `firebaseMessagingBackgroundHandler` only logs and returns. For iOS
background notifications, this means no local state updates or data fetching
occurs when a notification arrives while the app is suspended. This is less
critical for notification display (APNs handles that) but affects the user
experience when opening the app after tapping a notification.

## Potential Solutions

### Option A: Fix APNs configuration (server + client + Firebase Console)

- **Approach:** Address all five findings: fix `aps-environment` to `production`
  for release builds, add `Badge` and `ContentAvailable` to server APNs
  payload, verify APNs key in Firebase Console, and improve background handler.
- **Pros:** Comprehensive fix that addresses root cause and improves reliability.
- **Cons:** Requires Firebase Console access for verification (not just code changes).
- **Effort:** Small — most changes are a few lines each.

### Option B: Minimal fix — verify Firebase APNs key + fix entitlements

- **Approach:** Focus on the two most likely root causes: verify the APNs
  authentication key is uploaded in Firebase Console, and fix the
  `aps-environment` entitlement. Skip the server payload improvements.
- **Pros:** Fastest path to a fix if the APNs key is the root cause.
- **Cons:** Leaves the server payload incomplete, which could cause
  intermittent issues.
- **Effort:** Small.

### Option C: Add server-side iOS notification logging

- **Approach:** Before making any fixes, add detailed logging to capture the
  FCM response for iOS sends. This would confirm whether FCM is accepting
  the messages (and APNs is rejecting them) or whether the messages never
  reach FCM.
- **Pros:** Data-driven — confirms root cause before making changes.
- **Cons:** Requires a server deploy and waiting for iOS users to trigger
  notifications. Delays the fix.
- **Effort:** Small for logging; additional time for observation.

## Recommendation

**Option A** — fix everything. The individual changes are small and
well-understood. The `aps-environment` fix and APNs key verification are the
most likely root causes, but the server payload improvements (badge count,
content-available) are low-risk and improve iOS notification reliability. The
Firebase Console verification is a manual step that should be done first —
if the APNs key is missing, no code change will help.

## Open Questions

- **Has the APNs authentication key (.p8) been uploaded to Firebase Console?**
  — Recommendation: Check Firebase Console → Project Settings → Cloud
  Messaging → Apple app configuration immediately. This is the single most
  likely cause. If it's missing, upload it following the steps in
  `docs/push_notifications.md` Step 4.

- **Did iOS notifications ever work, or has this been broken since launch?**
  — Recommendation: Check server logs for any successful FCM sends to iOS
  platform devices. If there are zero successful iOS sends in the logs, this
  points to a Firebase/APNs configuration issue rather than a regression.

- **Are iOS users seeing the permission prompt and granting it?**
  — Recommendation: The permission request in `fcm_service.dart:77-85` looks
  correct. Ask Bryan to have an iOS user check Settings → Ripls → Notifications
  to confirm notifications are enabled.

- **Should `aps-environment` use separate entitlements files for debug vs release?**
  — Recommendation: No. Use a single entitlements file with `production`. The
  development provisioning profile will override to `development` during debug
  builds. This is simpler and avoids the current mismatch.

## Implementation Plan

### Phase 1: Verify Firebase APNs configuration (manual)

**Goal:** Confirm or rule out the most likely root cause.

**Tasks:**
- [ ] Check Firebase Console → Project Settings → Cloud Messaging → Apple app configuration
- [ ] Verify APNs authentication key (.p8) is uploaded with correct Key ID and Team ID
- [ ] If missing, upload following `docs/push_notifications.md` Step 4a-4b
- [ ] Verify the Firebase project ID matches `GoogleService-Info.plist` in `app/ios/Runner/`

**Validates:** Whether the APNs gateway is properly configured to receive messages from FCM.

### Phase 2: Fix entitlements and server APNs payload

**Goal:** Fix the code-level issues that could cause iOS notification failures.

**Tasks:**
- [ ] Change `aps-environment` from `development` to `production` in `app/ios/Runner/Runner.entitlements`
- [ ] Add `Badge` field (value `1`) to APNs config in `server/notifications/fcm/provider.go`
- [ ] Add `ContentAvailable: true` to APNs config for background delivery support
- [ ] Add `MutableContent: true` to enable notification service extensions (future-proofing for rich notifications)
- [ ] Update the APNs config to set `apns-push-type` header to `alert` explicitly
- [ ] Add unit test for iOS-specific APNs payload fields in FCM provider
- [ ] Run `npm run build` to verify server builds

**Validates:** That the server sends correctly-formed APNs payloads and the app uses the correct APNs environment.

### Phase 3: Test end-to-end on iOS device

**Goal:** Confirm notifications work on a real iOS device.

**Tasks:**
- [ ] Build the app with production entitlements and deploy to a real iOS device (TestFlight or direct)
- [ ] Verify device token registration in server logs (should show `DEVICE_PLATFORM_IOS`)
- [ ] Trigger a notification (e.g., express interest in a giveaway from another account)
- [ ] Verify notification appears on the iOS device in foreground, background, and terminated states
- [ ] Verify notification tap navigates to the correct item detail screen
- [ ] Check server logs for successful FCM send confirmation with iOS platform

**Validates:** End-to-end notification delivery on iOS.

## Architecture Audit

**Status:** Pass — no violations found.

**Checked against:** `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/server/architecture.md`

- **Server changes** are scoped to the `server/notifications/fcm/` shared
  library, not a service — correct per server architecture Pattern 2
  (interface-based dependencies). No service-to-service coupling introduced.
- **No client architecture impact.** Changes are to iOS platform config
  (entitlements) and server-side payload construction. No Flutter MVVM,
  repository, caching, or state management patterns affected.
- **No i18n impact.** No user-visible strings added or changed.
- **Testing:** Phase 2 includes a unit test for APNs payload fields. Use
  table-driven tests per server architecture guidelines (test both iOS and
  Android platform cases, verify badge/content-available/mutable-content
  fields are set correctly for iOS and absent for Android).
- **Logging:** The FCM provider already uses `logging.LoggerWithContext(ctx)`
  with structured fields — no changes needed to logging patterns.

## Related Issues

- #1004 — FCM log levels (improved notification logging)
