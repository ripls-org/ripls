# Issue #1036: [Bug Report] chat notifications don't deep link to chat tab (v2)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1036
**Filed:** 2026-04-05 | **Last activity:** 2026-04-12
**Labels:** bug, user-feedback | **Assignees:** maintainer
**Context docs:** none
**Previous plan:** `docs/issues/1036-chat-deep-link.md` (v1 — addressed warm-start; implemented in PR #1041)

## Problem Statement

Tapping a push notification opens the app but does not navigate to the target
screen. The user lands on the home screen instead of, e.g.,
`/gear/<id>?tab=chat`. This affects chat notifications and other notification
types (events, requests). Sometimes manifests as landing in the wrong community.
Happens more frequently after extended inactivity (backgrounded for a while).

PR #1041 (shipped in v0.10.0) fixed the warm-start path by replacing a fragile
`go('/') -> addPostFrameCallback -> push(route)` pattern with a single
`router.go(route)`. The bug persists on v0.11.0 (confirmed by Brian, April 12).

## Context from Comments

- **briangbrown (2026-04-12):** Reopened. Reproduced on v0.11.0 (PROD) — chat
  notification on a gear conversation. Confirmed PR #1041's fix shipped in
  v0.10.0 and no v0.11.0 PRs touched notification routing. Provided detailed
  analysis of the cold-start `getInitialMessage()` race.

- **maintainer (2026-04-12):** Also observed with non-chat notifications
  (new events). Sometimes lands in wrong community. Happens more frequently
  after not interacting with the app for a while.

## Root Cause Analysis

### Research findings

Investigated three areas: GoRouter pre-mount behavior, Firebase message delivery,
and the `initialize()` failure cascade.

#### GoRouter preserves pre-mount `go()` — NOT the problem

Verified from go_router 17.1.0 source code:

- `go()` updates `GoRouteInformationProvider._value` immediately
  (`information_provider.dart:201-206`)
- `initialLocation` only seeds the constructor (`information_provider.dart:101`),
  not applied on subsequent mounts
- Flutter's `Router.restoreState()` reads `routeInformationProvider.value` on
  first mount (`router.dart:634-637`), which IS the value from `go()`
- **Conclusion:** The v2 plan's original hypothesis (route overwritten by
  `initialLocation`) is wrong. GoRouter handles this correctly.

#### Firebase message delivery paths

From firebase_messaging 16.1.2 source:

- `onMessageOpenedApp` is a **broadcast stream**
  (`platform_interface_messaging.dart:85-87`) — events before `.listen()` are
  lost
- On Android, `onAttachedToActivity()` (`FlutterFirebaseMessagingPlugin.java:101-111`)
  calls `onNewIntent(mainActivity.getIntent())` for both cold start AND Activity
  recreation, which:
  - Stores message in `initialMessage` for `getInitialMessage()`
  - Fires `Messaging#onMessageOpenedApp` via method channel
- `getInitialMessage()` is a one-shot read that consumes the message
  (`FlutterFirebaseMessagingPlugin.java:277-287`)
- Server sends proper hybrid messages with both `notification` and `data` fields
  (`server/notifications/fcm/provider.go:213-259`), so the receiver stores them
  correctly

#### The actual bugs — `initialize()` failure cascade + missing resume handler

**Bug 1: `_setupMessageHandlers()` gated behind network calls**

```
fcm_service.dart initialize():
  50:  await _requestPermission()         ← async
  57:  await _initializeLocalNotifications()  ← async
  60:  await _registerDeviceToken()       ← NETWORK CALL, can fail/timeout
  63:  _setupMessageHandlers()            ← NEVER RUNS if any above fails
         getInitialMessage()              ← NEVER CALLED
         onMessageOpenedApp.listen()      ← NEVER SET UP
```

`_registerDeviceToken()` (`fcm_service.dart:132-179`) makes a network call to
the server. If it fails (timeout, server error), it rethrows
(`fcm_service.dart:173-179`). This exception propagates through `initialize()`'s
catch block (which also rethrows, line 71), becoming an unhandled Future error
(since `_initializeFCM()` calls `initialize()` without `await`, line 427).

**The result:** `_setupMessageHandlers()` never runs. Neither
`getInitialMessage()` nor `onMessageOpenedApp.listen()` are set up. The
notification tap data is silently lost.

This explains "more frequent after inactivity" — stale network connections are
more likely to fail on the first request after waking up.

**Bug 2: No `getInitialMessage()` on app resume**

When the app is backgrounded and the user taps a notification:
- If the Activity is alive: `onNewIntent()` fires → `onMessageOpenedApp`
  stream → listener handles it. This works IF `_setupMessageHandlers()` ran.
- If the Activity was destroyed (common after extended background): the plugin's
  `onAttachedToActivity()` fires `onNewIntent()` AND stores in
  `initialMessage`. The `onMessageOpenedApp` stream should fire, but the
  method channel delivery depends on engine resume timing. No fallback check
  via `getInitialMessage()` exists — it was only called once during initial
  setup.

**Bug 3: Missing community context switch**

The notification payload includes `community_id` (set by
`server/services/chat/notifications.go:105`) but `routeNotificationPayload()`
(`fcm_service.dart:362-405`) ignores it. The app navigates to the correct item
but with the wrong community selected. GearScreen listens to community changes
(`gear_screen.dart:66-75`) and auto-closes if the community changes after the
screen opens — but it never switches TO the notification's community in the
first place.

## Potential Solutions

### Option A: Reorder `initialize()` + add resume check + community switch

- **Approach:** Three targeted fixes:
  1. Move `_setupMessageHandlers()` before the await chain in `initialize()` so
     `getInitialMessage()` and `onMessageOpenedApp.listen()` run even if device
     token registration fails
  2. Add `getInitialMessage()` check in `didChangeAppLifecycleState(resumed)` to
     catch missed notifications on resume (with message ID deduplication)
  3. Extract `community_id` from the notification payload and switch communities
     before navigating
- **Pros:** Fixes all three bugs. Minimal new state. Each fix is independently
  valuable and testable. No GoRouter hacks or pending-route queues needed.
- **Cons:** Resume check could double-fire with `onMessageOpenedApp` on Android
  warm starts (both fire for the same message). Needs message ID tracking.
- **Effort:** Small-medium.

### Option B: Reorder `initialize()` only

- **Approach:** Just move `_setupMessageHandlers()` before the await chain.
- **Pros:** Simplest change. Fixes the cold-start case.
- **Cons:** Doesn't fix resume/background case. Doesn't fix wrong community.
- **Effort:** Small.

### Option C: Full pending-route queue (original v2 plan)

- **Approach:** Store pending notification route, replay after splash.
- **Pros:** Defensive — works even if GoRouter pre-mount behavior changes.
- **Cons:** Unnecessary complexity. GoRouter already preserves the route
  (verified from source). Doesn't fix the root cause (`initialize()` failure
  cascade). Doesn't fix resume case.
- **Effort:** Small, but solving the wrong problem.

## Recommendation

**Option A: Reorder + resume check + community switch.**

The `initialize()` reorder is the highest-impact fix — it ensures
`getInitialMessage()` runs on every cold start regardless of network conditions.
The resume check is the safety net for background/resume scenarios. The community
switch fixes the "wrong community" symptom.

Option B solves only cold starts. Option C addresses a non-problem (GoRouter
behavior is correct) while missing the actual root cause.

## Open Questions

- **Does `getInitialMessage()` work before requesting permission?** —
  Yes. Permission is needed to receive new notifications, not to read the launch
  intent that already triggered the app. `getInitialMessage()` reads from
  `initialMessage` (set by `onAttachedToActivity()`) or from the persistent
  store — neither requires runtime notification permission.

- **Will `onMessageOpenedApp` and `getInitialMessage()` double-fire on Android
  warm starts?** — Yes. On Android, `onNewIntent()` stores in `initialMessage`
  AND fires `onMessageOpenedApp`. Both paths call `_onMessageOpenedApp()`.
  Deduplicate by tracking the last-handled message ID.

- **Should `_registerDeviceToken()` stop rethrowing?** — No. The rethrow
  is correct for caller visibility. The fix is reordering, not swallowing
  errors. However, `initialize()` should catch the error after
  `_setupMessageHandlers()` runs, so the message handlers are always set up.

- **What about iOS?** — On iOS, `getInitialMessage()` and `onMessageOpenedApp`
  are mutually exclusive (native code checks `_initialNotificationID` match).
  The resume check would call `getInitialMessage()` which returns null if
  `onMessageOpenedApp` already fired. No double-handling risk on iOS.

## Implementation Plan

### Phase 1: Reorder `initialize()` to decouple message handlers from network calls
**Goal:** `getInitialMessage()` and `onMessageOpenedApp.listen()` always run,
even if device token registration fails.
**Tasks:**
- [ ] In `fcm_service.dart:initialize()`, move `_setupMessageHandlers()` call
  (currently line 63) to run BEFORE `_requestPermission()` (before line 50).
  Also move `_messaging.onTokenRefresh.listen(_onTokenRefresh)` (line 66) to
  after `_registerDeviceToken()` in a separate try block
- [ ] Restructure `initialize()` so that `_requestPermission()`,
  `_initializeLocalNotifications()`, and `_registerDeviceToken()` are in their
  own try/catch that logs but doesn't prevent the method from completing.
  `_setupMessageHandlers()` runs unconditionally
- [ ] Add a unit test: mock `FirebaseMessaging` to have `getToken()` throw,
  verify that `getInitialMessage()` still resolves with the expected message
- [ ] Run existing `fcm_navigation_test.dart` to verify no regression
**Validates:** Notification deep links work even when device token registration
fails (stale network, server unavailable).

### Phase 2: Add `getInitialMessage()` check on app resume
**Goal:** Catch notifications missed by `onMessageOpenedApp` during
background/resume transitions.
**Tasks:**
- [ ] Add `String? _lastHandledNotificationMessageId` field to `FCMService`
- [ ] In `_onMessageOpenedApp()` (`fcm_service.dart:235`), record
  `message.messageId` in `_lastHandledNotificationMessageId` and skip if
  it matches (deduplication)
- [ ] Add a `checkForMissedNotification()` method on `FCMService` that calls
  `getInitialMessage()`, deduplicates by message ID, and routes if found
- [ ] Call `fcmService.checkForMissedNotification()` in
  `didChangeAppLifecycleState(AppLifecycleState.resumed)` in `main.dart`
- [ ] Add logging: `_log.info('Resume check: getInitialMessage returned
  messageId=...')` and `_log.info('Resume check: skipping duplicate
  messageId=...')`
- [ ] Add unit tests: verify deduplication (same ID skipped), verify
  new message on resume triggers `onNavigate`
**Validates:** Notification taps work after extended background periods,
regardless of Android Activity lifecycle behavior.

### Phase 3: Community context switching for notification navigation
**Goal:** Notification taps land in the correct community context.
**Tasks:**
- [ ] Modify `onNavigate` callback signature to include an optional
  `communityId` named parameter:
  `Function(String route, {String? communityId})?`. Prefer this over a
  dedicated class — two fields don't justify a new type (CLAUDE.md:
  "only add the minimal required dependencies")
- [ ] In `routeNotificationPayload()` (`fcm_service.dart:362-405`), extract
  `community_id` from the data map and pass it to `onNavigate`
- [ ] In the `onNavigate` handler (`main.dart:417-421`), if `communityId` is
  non-null and differs from the currently selected community, switch via
  `selectedCommunityProvider.notifier.selectCommunity()` before calling
  `router.go(route)`. Find the community in the current communities list
  by ID. This follows the existing pattern in `main.dart` where the app
  entry point directly manages `selectedCommunityProvider` for
  orchestration purposes (see `_loadCommunities`, `_refreshCommunitiesOnResume`)
- [ ] Also apply community switch in the `checkForMissedNotification()`
  resume path (Phase 2)
- [ ] Update `fcm_navigation_test.dart` to verify `community_id` passthrough
- [ ] Manual test: be in community A, receive notification for item in
  community B, tap it, verify community B is selected and item opens
**Validates:** No more "wrong community" symptoms after notification taps.

### Phase 4: Automated tests for resume and cold-start paths
**Goal:** Prevent regression with automated tests covering the full flow.
**Tasks:**
- [ ] Add test: `initialize()` with failing `_registerDeviceToken()` — verify
  `getInitialMessage()` still called and message routed
- [ ] Add test: simulate resume with pending `getInitialMessage()` — verify
  `checkForMissedNotification()` routes correctly
- [ ] Add test: duplicate message ID across `onMessageOpenedApp` and
  `getInitialMessage()` — verify only handled once
- [ ] Add test: community ID extraction and switching in notification routing
**Validates:** All notification delivery paths are covered by automated tests.

## Audit Notes

**Reviewed against:** `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/server/architecture.md`

### Issues found and fixed

1. **Over-engineering: `NotificationRoute` class (architecture.md, CLAUDE.md):**
   Phase 3 originally proposed a dedicated `NotificationRoute` class to carry
   `route` and `communityId`. Two fields don't justify a new type. **Updated to
   use a named parameter on the callback signature instead**, consistent with
   CLAUDE.md's "only add minimal required dependencies" and the existing
   `onNavigate` pattern.

### Verified compliant

- **Architecture layers (architecture.md):** All changes stay within the
  service layer (`fcm_service.dart`) and app entry point (`main.dart`). No
  viewmodel or repository boundary violations. `main.dart` already directly
  manages `selectedCommunityProvider` for app-level orchestration
  (`_loadCommunities`, `_refreshCommunitiesOnResume`) — Phase 3 follows the
  same pattern.

- **Navigation patterns (architecture.md §Navigation):** Deep link navigation
  uses `router.go()`, which is correct for external entry points (notification
  taps). `NavigationHelpers` and `context.push()` rules apply to in-app
  navigation from widgets, not programmatic routing from FCM handlers.

- **Caching (caching.md):** No caching changes. Notification routing is
  stateless.

- **i18n (i18n.md):** No new user-visible strings. Log messages are developer-
  facing, not user-facing.

- **Server architecture (server architecture.md):** No server changes.

- **Proto boundary (CLAUDE.md):** Client reads raw FCM data fields
  (`Map<String, dynamic>`), not generated proto models. No proto boundary
  violations.

- **Testing (architecture.md §Testing):** All proposed tests are pure
  service-layer unit tests using mocked `FirebaseMessaging`. No UI + logic
  mixing.

## Related Issues
- #962 — Chat notifications lead to old chat UI (fixed by PR #978 — added
  `?tab=chat` routing and server-side entity ID resolution)
- #977 — Non-chat notification deep links (separate scope, but will also
  benefit from these fixes)
- #1041 — PR that fixed the warm-start path (go+push → single go)
