---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Push-notification-only architecture for community updates — provider-agnostic NotificationService (FCM/APNs/test), strongly-typed proto payloads, targeted recipients, device-token management, deep linking with tap→navigate recovery, and the Flutter FCM handler.
  globs: [server/notifications/**, server/services/device/**, server/services/community/notification_prefs.go, app/lib/services/fcm_service.dart, app/lib/services/notification_route_replayer.dart]
  triggers: [push-notification, fcm, apns, device-token, deep-link, deep-link-recovery, notify, notification-payload]
  lens: [domain, client, server]
  domain: notifications
freshness:
  verified_commit: "5f4eaab27"
  verified_on: "2026-09-20"
---
# Push Notifications for Real-Time Community Updates

## Overview

For low-frequency community events (a few times per day), we use a push-notification-only architecture instead of maintaining persistent streaming connections. This is simpler, more efficient, and better suited for the actual usage pattern.

### Design Principles

- **Event-driven**: Push notifications trigger data fetches, not streaming
- **Battery efficient**: No persistent connections to maintain
- **Simple**: Standard request/response RPC pattern throughout
- **Provider-agnostic**: Abstraction allows swapping FCM/APNs/webhooks/etc.
- **Bandwidth efficient**: Notifications contain minimal data, full data fetched on-demand

## Architecture

### High-Level Flow

```
Event occurs → Server records event → NotificationService sends push
                                    ↓
Client receives push → Calls GetCommunityEventsSince RPC → Updates UI
```

### Key Components

1. **Server-side notification abstraction** - Provider-agnostic notification service
2. **Provider implementations** - Concrete implementations (FCM, APNs, test/no-op for testing)
3. **Device token management** - Track user devices and tokens
4. **Community service integration** - Send notifications when events occur
5. **Client notification handler** - Process incoming notifications and fetch data

### Server Components

**Proto Definitions:**
- [`proto/ripls/models/notification.proto`](../proto/ripls/models/notification.proto) - Notification model with strongly-typed payloads
- [`proto/ripls/models/user_device.proto`](../proto/ripls/models/user_device.proto) - Device registration model
- [`proto/ripls/api/device_service.proto`](../proto/ripls/api/device_service.proto) - Device registration API

**Notification Service:**
- [`server/notifications/service.go`](../server/notifications/service.go) - Provider-agnostic notification service
  - Interface: `NotifyUser()`, `HasDevices()`, `UnregisterDevice()`, `SendPhoneOptInWelcome()` (one-time opt-in confirmation SMS/RCS, sent on first phone registration)
  - Multi-provider support with platform-specific routing
  - Automatic cleanup of invalid device tokens

**Providers:**
- [`server/notifications/fcm/provider.go`](../server/notifications/fcm/provider.go) - FCM provider for production
- [`server/notifications/noop/provider.go`](../server/notifications/noop/provider.go) - No-op provider for development and testing (logs notifications without sending; selected by `--notification-provider=test`)
- [`server/notifications/mock.go`](../server/notifications/mock.go) - `MockService` that captures `NotifyUser` calls for test verification

**Device Service:**
- [`server/services/device/service.go`](../server/services/device/service.go) - Device token management
  - RPCs: `RegisterDeviceToken()`, `UnregisterDeviceToken()`
  - Idempotent registration; backfills the Firebase installation ID onto an
    already-registered device if a later registration reports one it didn't
    have before
  - Handle token migration between users

**Addressing (FCM installation ID vs. device token, #3043):**
- `RegisterDeviceTokenRequest`/`UserDevice` carry both `device_token` and an
  optional `installation_id` (the Firebase installation ID). FCM deprecated
  addressing by registration token in favor of the installation ID, but the
  two are different identifiers and are not interchangeable.
- [`server/notifications/fcm/provider.go`](../server/notifications/fcm/provider.go)
  sends to the installation ID (`Target.InstallationID`) when the client
  reported one, falling back to `Target.DeviceToken` otherwise — so devices
  registered before the client sent an installation ID keep working
  unchanged.
- Client-side, `FCMService` reads the ID via the `firebase_app_installations`
  package; a read failure is non-fatal and simply leaves the device on the
  token-only path.

**Community Event Bus Integration:**
- Push dispatch is an event-bus subscriber, not in-place service code. The
  community service publishes each `CommunityEvent` to the in-memory
  `community_event_bus`; the [`server/notifications/community_subscriber/`](../server/notifications/community_subscriber/)
  package consumes it asynchronously.
  - `Subscriber.Handle()` ([`subscriber.go`](../server/notifications/community_subscriber/subscriber.go)) resolves recipients, gates by preferences/active-stream/soft-delete/dedup, builds copy, and calls `notificationService.NotifyUser()`
  - Helpers: `ShouldNotify()`, `resolveRecipients()` / `getNotificationRecipients()` ([`recipients.go`](../server/notifications/community_subscriber/recipients.go)), `buildNotification()` ([`copy.go`](../server/notifications/community_subscriber/copy.go))
  - **Member-broadcast audiences are pinned at publish time**: for the "all
    community members minus actor" event types (`EXPERIENCE_CREATED`,
    `GEAR_SHARED`, `REQUEST_CREATED`, `INVITATION_LINK_USED`,
    `COMMUNITY_RESTORED`), `InProcessBus.Publish` captures the member IDs
    onto `PublishedEvent.MemberIDsAtPublish` (see
    `cebus.SnapshotsMembers`), and recipient resolution prefers that
    snapshot. Resolving from live membership at dispatch time raced joins
    landing between publish and the async dispatch — a user who joined in
    that window was pushed an event that predated their membership
    (#2657). A user's membership start is the notification cutover: events
    before it reach them through the feed and item surfaces, not push. An
    absent snapshot (failed capture, non-bus publisher) falls back to the
    live lookup with a WARN.
  - Gating helpers live in [`server/community/notifications.go`](../server/community/notifications.go): `CategoryFor()`, `CategoryEnabled()`, `FiresForDeletedCommunity()`, `IsActive()`, `FetchPreferencesForUsers()`
  - Chat pushes are handled by a sibling [`server/notifications/chat_subscriber/`](../server/notifications/chat_subscriber/) subscriber
  - Targeted notifications: Only users directly involved in transactions receive notifications (not all community members)
  - Deep linking: Notifications include entity IDs (`gear_id`, `experience_id`, `request_id`) for navigation to the item detail screen

### Client Components (Flutter)

**Dependencies** (in `pubspec.yaml`):
- `firebase_core`
- `firebase_messaging`
- `firebase_app_installations`
- `flutter_local_notifications`

**FCM Service** (`app/lib/services/fcm_service.dart`):
- Request notification permissions
- Get and register device tokens (plus the Firebase installation ID, when
  readable) with server
- Handle token refresh
- Display local notifications for foreground messages

**App State Handling:**
- `FirebaseMessaging.onMessage` - foreground messages
- `FirebaseMessaging.onMessageOpenedApp` - background/terminated taps
- `FirebaseMessaging.onBackgroundMessage` - background handler

## Key Design Decisions

### Strongly-Typed Notification Payloads

Instead of weakly-typed key-value pairs, notifications use proto `oneof` for type safety:

```protobuf
message Notification {
  string title = 1;
  string body = 2;

  oneof payload {
    CommunityEventPayload community_event = 10;
    ChatMessagePayload chat_message = 12;
  }
}
```

### Provider Map Architecture

Providers are stored in a map keyed by platform for O(1) lookup:

```go
providers map[models.DevicePlatform]Provider
```

### Off-App Channels (Email)

`NotifyUser` historically suppressed silently when a recipient had no active app
device (`len(devices) == 0`). EMAIL-1 (epic #2492) turns that seam into the
**off-app channel**: a deviceless recipient is reached by email instead.

- **Where:** the no-device branch in `NotifyUser` (`service.go`) now calls
  `dispatchOffApp`, which resolves the recipient's `User.email` + locale +
  opt-out and sends via an `EmailSender` (`server/email`'s `SendNotification`).
- **Surface-specific copy:** community-event emails re-render in the recipient's
  locale via `notification_content.RenderEmail` (`off_app_email.go`) rather than
  reusing the push `Title`/`Body`; chat falls back to the already-localized
  `Title`/`Body`. Re-rendering does **not** mean a second sentence: push and
  off-app share one `notif.offapp.{kind}.message`, and each surface adds only
  its own adornment — a category title for push
  (`notif.community_event.{kind}.title`, via `Content.PushCopy`), a CTA verb for
  off-app. What re-rendering buys is the recipient's locale and a link CTA, not
  different words (#2896).
  Community-event emails are further enriched with a themed item card — inline
  hero image + per-entity summary — built best-effort by `buildEmailExtras`
  (`off_app_summary.go`); any miss degrades to the plain layout. Either way the
  email is wrapped in the branded shell with a CAN-SPAM footer + one-click
  unsubscribe.
- **Short-link CTA:** both off-app channels land the recipient on a short
  `/go/{code}` share link to the entity rather than a long entity-ID URL.
  `notificationLink` (`deeplink.go`) resolves it best-effort via the injected
  `ShareLinkResolver` (`WithShareLinkResolver`, satisfied by the community
  service's `ShortLinkCodeForEntity`); any failure or non-linkable notification
  degrades to the channel's bare `AppBaseURL`.
- **Neither off-app channel fits the provider map** (no `UserDevice` row), so
  each is a separate field on the service — email via `WithOffAppEmail`, SMS via
  `WithSMS` — not a `providers[platform]` entry. The no-device branch routes by
  handle in the same `dispatchOffApp`: SMS for a phone-bearing recipient, email
  otherwise (SMS internals, including its own flag gate and email fallback, live in
  [`sms_notifications.md`](sms_notifications.md)).
- **Best-effort & flag-gated:** a disabled channel, missing email, opt-out, or
  send failure is logged with structured `channel` + `outcome` fields (the
  observable for Cloud Logging queries) and never fails the originating
  `NotifyUser`. Off by default
  (`--off-app-email-enabled`); enabling it for all deviceless users is a
  deliberate ops decision.
- **Opt-out (CAN-SPAM):** `User.off_app_email_opted_out` is the durable
  suppression record, set by the signed one-click unsubscribe link
  (`GET /email/unsubscribe`, verified via `auth.VerifyUnsubscribeToken`).

### Shared Unregister Logic

Both device service and notification service use the same `UnregisterDevice()` method to ensure consistency when removing invalid tokens.

### Asynchronous Notification Sending

The community service records the event synchronously, then publishes it to the
in-memory `community_event_bus`. Each subscriber (push, stream broadcast) runs in
the bus's own dispatch, so notification sending never blocks the originating RPC.
The push subscriber short-circuits early on events that never notify:

```go
func (s *Subscriber) Handle(ctx context.Context, evt *cebus.PublishedEvent) error {
    if !ShouldNotify(evt.Event.EventType) {
        return nil
    }
    // resolve recipients, gate, build copy, NotifyUser...
}
```

### Targeted Notifications (Not Broadcast)

Notifications are sent only to users directly involved in a transaction, not all community members:

| Event Type | Recipients |
|------------|------------|
| Interest Expressed | Gear owner |
| Recipient Selected | Selected recipient |
| Transfer Cancelled | Other party (owner or recipient) |
| Pickup Proposed | Other party (owner or recipient) |
| Offer Made | Request creator |
| Experience RSVP | Experience host |
| Experience Created | All community members (except creator), as of publish time |

For the member-broadcast rows, "all community members" means membership **as
of the event's publish** (the `MemberIDsAtPublish` snapshot) — someone who
joins moments later is not notified about it (#2657).

### Notification Deduplication

To avoid duplicate notifications, system messages (chat) do NOT send push notifications. All push notifications flow through the community event system. This prevents users from receiving multiple notifications for the same action (e.g., when someone offers to help with a request).

### Deep Linking

Notifications include entity IDs (`gear_id`, `experience_id`, `request_id`) in the payload, enabling the client to navigate directly to the item detail screen when tapped. Chat message notifications also include `conversation_id`, which causes the client to open the Chat tab of the item detail screen.

#### Tap → navigate pipeline and recovery (#2636)

The "notification doesn't take me to the item" bug recurred repeatedly (#1036,
#2117, #2636) because a dropped deep link is invisible — it's not a crash, and
the path was INFO-logged only. The residual drop lives at the Android
FCM/platform boundary (`getInitialMessage()` returning null / `onMessageOpenedApp`
not firing on terminated/background taps), which **cannot be reproduced in a
Dart test**. The client-side pipeline is now built to **catch, report, and
recover** so the next real drop is diagnosable and, where the cause is
client-side, self-healing:

- **Report** — `routeNotificationPayload` (`app/lib/services/fcm_service.dart`)
  emits a `notification_deeplink` analytics event (`NotificationDeepLinkEvent`
  in `app/lib/core/observability/events.dart`) at the route decision and again
  at the navigation result. `phase: route` carries `source`
  (`opened|initial|missed|local`), the resolved `entity`/`route`, and the drop
  causes (`no_entity_id`, `on_navigate_null`); `phase: nav_result` carries the
  router's actual location one frame after `go()` (`landed`), the drain
  `trigger`, and `recovered`. Drop branches are logged at WARNING so they also
  become Crashlytics breadcrumbs. Params are redacted (entity IDs and data-map
  **keys** only — never titles/bodies) and inherit the observability consent
  no-op. Analytics is aggregate and per-incident-queryable only via the
  BigQuery export, and a WARNING breadcrumb surfaces only on a crash — so a
  **permanent** drop (the replay also failed) is additionally reported as a
  **Crashlytics non-fatal** (`NotificationDeepLinkDroppedException`, wired from
  `main.dart`), which *is* visible per-incident from prod user devices and can
  drive the alert→issue pipeline. A first-attempt miss that later recovers does
  **not** fire the non-fatal.
- **Catch** — the moment a payload resolves to a route, it is written to
  `PendingNotificationRouteHolder` (a static hand-off slot mirroring
  `DeferredDeepLinkContextHolder`) **independent of the imperative `onNavigate`
  callback**. So a not-yet-mounted router or a rebuilt FCM service with an
  unwired callback can no longer *drop* the link — worst case it *defers*.
  `fcmServiceProvider` is also `keepAlive` so the service (and its callback) is
  never rebuilt, and the resume `checkForMissedNotification()` serializes behind
  the startup `getInitialMessage()` consume so a cold-start tap isn't
  double-handled.
- **Recover** — `NotificationRouteReplayer`
  (`app/lib/services/notification_route_replayer.dart`) is the single executor.
  It drains the holder from three vectors — the live-tap poke (`onNavigate`),
  splash→ready (a terminated-state tap captured before the router mounted), and
  app-resume — does `go()`, verifies it stuck a frame later, and **retries once**
  (via a timer, since an idle app schedules no frames) before giving up. A
  recovered replay reports `recovered: true`; a permanent client-side drop
  reports `recovered: false` (the queryable "gave up" signal). There is **no
  fallback UI** — a route that even the replay can't land is reported, not
  surfaced on Home.

The **platform-layer** drop (Android never delivers the message, so there is no
route to replay) is out of the recovery mechanism's reach by construction; the
Phase-1 telemetry above exists to measure how often that actually happens.

## Event Types that Trigger Notifications

Recipient resolution and the `ShouldNotify` allow-list live in
`server/notifications/community_subscriber/recipients.go`; the per-category
gating helpers (`CategoryFor`, `CategoryEnabled`) live in
`server/community/notifications.go`. Every notification is gated per recipient
by the `CommunityNotificationPreferences` row for the (user, community) pair.
Unset toggles resolve to "on", so default behavior is to notify; users opt out
via the per-community Manage Notifications screen.

**Transfer Events** (gated by `notify_transfer_updates`):
- `TRANSFER_INTEREST_EXPRESSED` / `TRANSFER_INTEREST_WITHDRAWN` — gear owner
- `TRANSFER_RECIPIENT_SELECTED` — selected recipient
- `TRANSFER_ACTIVE`, `TRANSFER_CANCELLED`, `TRANSFER_PICKUP_PROPOSED` — other party

**Request Events** (gated by `notify_request_updates`, except CREATED):
- `REQUEST_OFFER_MADE` — request creator
- `REQUEST_OFFER_WITHDRAWN` — request creator
- `REQUEST_OFFER_SELECTED` — the chosen helper, named in `object_user_id` at emit
  time ("your offer was picked"). It gained its first emitter with gear-backed
  offers (#2702); before that it was documented here as never notifying.
- `REQUEST_CANCELLED` — all other offerers
- `REQUEST_CREATED` — broadcast to community minus actor (gated by `notify_new_requests`)

`REQUEST_FULFILLED` intentionally does **not** notify here (product decision —
its pushes flow through the publish-time member snapshot; see `ShouldNotify` in
`recipients.go`).

**Experience Events:**
- `EXPERIENCE_RSVP_YES` / `EXPERIENCE_RSVP_MAYBE` — host (gated by `notify_experience_rsvps`)
- `EXPERIENCE_CREATED` — community minus actor (gated by `notify_new_experiences`)
- `EXPERIENCE_COMPLETED` — Yes/Maybe RSVPs minus actor (gated by `notify_experience_completed`). A lifecycle ping like START/CANCEL, not a community broadcast — only people who said they were coming are told the event wrapped (the host is the actor and is filtered out). The per-community story/recap card is generated separately and is unaffected.

**Gear Events:**
- `GEAR_SHARED` — community minus actor (gated by `notify_gear_shared`)

**Direct Share:**
- `ITEM_SHARED_WITH_USER` — the one person named in `object_user_id`, **ungated**
  (no `NotificationCategory`, so `CategoryFor` returns UNSPECIFIED and it passes
  through unfiltered). One event per person the share *newly* added; re-sharing to
  widen the audience does not re-notify existing members, and sharing with yourself
  notifies nobody. Applies to gear, experiences and requests alike — the copy kind
  splits three ways on what was handed over (`item_shared_with_user_{gear,experience,request}`),
  gear being the default because gear travels in its own payload field rather than
  the topic oneof. The categories gate community broadcasts (volume); a direct share
  is one person handing something to one named person, so it is not toggleable (#3106).
  `GEAR_SHARED` cannot serve this: it is published before the invitee is a member and
  its audience is the publish-time member snapshot, so it misses exactly the person
  the share was for.

**Membership Events:**
- `INVITATION_LINK_USED` — community minus the joiner (gated by
  `notify_new_members`). Title: "New member" / Body: "[user] joined the
  community". Tap deep-links to the community **discussion**
  (`/group/{community_id}?tab=discuss`) — the off-app copy says "Say hi at …",
  so anywhere else breaks that promise (#2876). Every other community-scoped
  event with no inner entity (`COMMUNITY_DELETED`, `COMMUNITY_RESTORED`,
  `OWNERSHIP_TRANSFERRED`) lands on `/group/{community_id}` itself: a deletion
  notice does not belong in a chat pane.

**Planning Events** (gated by `notify_planning_updates`, recipients are
RSVP=YES/MAYBE on the experience):
- `PLANNING_NEED_ADDED`
- `PLANNING_NEED_CLAIMED`
- `PLANNING_CONTRIBUTION_ADDED`

**Experience Lifecycle** — all notify Yes/Maybe RSVPs minus the actor via the
community-event funnel:
- `EXPERIENCE_STARTED` (mark in-process), `EXPERIENCE_CANCELLED`, and
  `EXPERIENCE_UPDATED`; gated by `notify_experience_rsvps`.
- `EXPERIENCE_COMPLETED`; gated by `notify_experience_completed`. Was previously
  a full-community broadcast — narrowed to RSVPs so completing an event with no
  other attendees no longer pings the whole community. The per-community
  story/recap card still generates regardless.

**Chat Messages** (gated by `notify_chats`):
- Chat message in any conversation - Notifies participants not actively
  streaming. Includes topic entity ID for deep linking.

**Events that do NOT trigger notifications:**
- `EXPERIENCE_RSVP_NO`, `MEMBER_LEFT`, `GEAR_UNSHARED`
- System chat messages - Handled by community events to avoid duplicates

## Benefits Over Streaming

| Aspect | Streaming | Push Notifications |
|--------|-----------|-------------------|
| **Latency** | <100ms | 1-5 seconds |
| **Battery** | Moderate drain | Minimal impact |
| **Complexity** | High (reconnection, keepalive) | Low (platform handles) |
| **When app closed** | Doesn't work | Works perfectly |
| **Scalability** | Server manages connections | FCM infrastructure |
| **Best for** | High-frequency (chat, games) | Low-frequency (alerts, feeds) |
| **Our use case** | Overkill | Perfect fit |

## Configuration

### Server Configuration

#### Development (Test Provider)

```bash
# Use test provider for testing/development (default)
./server --notification-provider=test
```

#### Production with FCM

The server uses **Application Default Credentials** (ADC) for FCM. There is
no `--fcm-credentials` flag — the explicit-key-file path was removed in #1769.

```bash
# Local dev: authenticate once, then ADC takes over.
gcloud auth application-default login

./server --notification-provider=fcm --firebase-project="$(scripts/gcp_project.sh dev)"
```

The `--firebase-project` flag is required locally — the SDK can't infer
the project ID under user ADC.

```bash
# Alternative for local dev: point ADC at a key file outside the repo.
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/firebase-credentials.json

./server --notification-provider=fcm
```

**That key-file path is the one dev and prod use** (#3078): the deployment
sets `GOOGLE_APPLICATION_CREDENTIALS` to a bind-mounted service-account key
and `NOTIFICATION_PROVIDER=fcm`, and sets no `--firebase-project` — the Admin
SDK reads `project_id` from the key file itself. That service account must
hold the Firebase Admin / Cloud Messaging IAM roles.

Off GCP there is no metadata server, so the project is not inferred from the
environment; it comes from the key file or the flag.

### Firebase Project Setup

1. **Create Firebase project**: https://console.firebase.google.com/
2. **Add iOS app**: Get `GoogleService-Info.plist`
3. **Add Android app**: Get `google-services.json`
4. **Configure APNs for iOS Push Notifications**:

   Firebase uses Apple Push Notification service (APNs) to deliver notifications to iOS devices. You must configure APNs credentials in Firebase:

   **Step 4a: Get APNs Authentication Key from Apple**
   - Go to [Apple Developer Portal](https://developer.apple.com/account/resources/authkeys/list)
   - Sign in with your Apple Developer account
   - Click the "+" button to create a new key (or use an existing one)
   - Check "Apple Push Notifications service (APNs)"
   - Give it a name (e.g., "Ripls Push Notifications")
   - Click Continue → Register
   - Download the .p8 file (you can only download it once - save it securely!)
   - Note the Key ID (shown on the download page)
   - Note your Team ID (found at the top right of the Apple Developer portal)

   **Step 4b: Upload APNs Key to Firebase**
   - Go to Firebase Console → Select your project
   - Click the gear icon → Project Settings
   - Go to the "Cloud Messaging" tab
   - Scroll down to "Apple app configuration"
   - Under "APNs authentication key", click "Upload"
   - Upload your .p8 file
   - Enter your Key ID (from Step 4a)
   - Enter your Team ID (from Step 4a)
   - Click "Upload"

5. **Download service account key for server**:
   - Project Settings → Service Accounts
   - Click "Generate New Private Key"
   - Save as `firebase-credentials.json`
   - Keep this file secure - it grants admin access to your Firebase project

6. **Enable Cloud Messaging API**: Automatically enabled for new projects

**Important Configuration Notes:**
- The iOS app and server must use the **same Firebase project**
- Verify the `PROJECT_ID` in `GoogleService-Info.plist` matches the `project_id` in your server's Firebase credentials JSON
- Common error: "SenderId mismatch" indicates the app and server are using different Firebase projects
- Common error: "Auth error from APNS" indicates APNs credentials are not configured in Firebase (Step 4)

### Client Configuration (Flutter)

1. **Create or use existing Firebase project** from server setup above

2. **Add Android App** (if not already added):
   - Go to Project Settings → Add App → Android
   - Package name: `org.ripls.app`
   - Download `google-services.json`
   - Place in `app/android/app/google-services.json`

3. **Add iOS App** (if not already added):
   - Go to Project Settings → Add App → iOS
   - Bundle ID: `org.ripls.gear-app`
   - Download `GoogleService-Info.plist`
   - Place in `app/ios/Runner/GoogleService-Info.plist`
   - Add to Xcode project (drag into Xcode, check "Copy items if needed")

4. **iOS Capabilities** (Xcode):
   - Open `app/ios/Runner.xcworkspace` in Xcode
   - Select Runner → Signing & Capabilities
   - Add "Push Notifications" capability
   - Add "Background Modes" capability
   - Enable "Background fetch" and "Remote notifications"

5. **Run the app**:
   ```bash
   cd app
   flutter pub get
   flutter run
   ```

The FCM service will automatically:
- Request notification permissions on first launch
- Register device token with the server (if authenticated)
- Handle incoming notifications in all app states
- Display local notifications for foreground messages

## Manual Testing

### Prerequisites

Before testing, ensure you have:
- Firebase project created with Android and iOS apps
- `google-services.json` in `app/android/app/`
- `GoogleService-Info.plist` in `app/ios/Runner/`
- Firebase service account JSON for server
- iOS capabilities configured (Push Notifications + Background Modes)

### Step 1: Start the Server with FCM

```bash
npm run start:server
```

**Expected output:**
```
Initializing FCM notification providers...
Initialized FCM provider for DEVICE_PLATFORM_IOS...
Initialized FCM provider for DEVICE_PLATFORM_ANDROID...
✓ FCM notification providers initialized
Starting server on 0.0.0.0:8080
```

### Step 2: Run the Flutter App

List available devices:
```bash
flutter devices
```

Run on your device:
```bash
cd app
flutter run -d <device-id>
```

**Note:** iOS push notifications require a real device (not simulator).

### Step 3: Login and Verify Token Registration

1. **Launch the app** and login/register with any email (dev auth mode)

2. **Watch server logs** for device token registration:
   ```
   User test@example.com (user-id) registering device token for platform DEVICE_PLATFORM_ANDROID
   Registered device with ID <device-id>
   ```

3. **Watch Flutter logs** for FCM initialization:
   ```
   [FCMService] Initializing FCM service...
   [FCMService] Permission status: authorized
   [FCMService] Got device token: <token>...
   [FCMService] Device registered with ID: <device-id>
   [FCMService] FCM service initialized successfully
   ```

### Step 4: Test Notification Delivery

**Create a Community:**
1. Navigate to Communities in the app
2. Create a new community (e.g., "Test Community")
3. Invite another user (use a different email)

**Trigger Notification:**
1. Login as the invited user (second device or account)
2. Accept the invitation

**Server logs should show:**
```
User user2@example.com (user-id-2) accepting invitation: <invitation-id>
Successfully sent FCM notification to token=<token>..., message_id=<message-id>
```

**App should receive notification:**
- **Foreground**: Local notification appears at top of screen
- **Background/Terminated**: System notification appears, tap opens app

### Step 5: Verify App States

**Foreground Notifications:**
1. Keep app open and visible
2. Trigger a notification event
3. Local notification appears, `onForegroundMessage` fires

**Background Notifications:**
1. Send app to background (home button)
2. Trigger a notification event
3. System notification appears
4. Tap notification → app opens, `onNotificationTapped` fires

**Terminated Notifications:**
1. Force quit the app
2. Trigger a notification event
3. System notification appears
4. Tap notification → app launches

### Step 6: Deep-Link Recovery Release Gate (#2636)

Run this matrix on **physical devices** before shipping any change to the
notification tap → navigate pipeline (`fcm_service.dart`, `main.dart`'s FCM
wiring, `notification_route_replayer.dart`, or the router redirect). Emulators
and simulators do **not** reproduce the OEM battery/process-kill quirks that
cause the field drop — the original report was a physical Pixel 9 Pro, so at
minimum cover one physical Android device plus one physical iPhone.

**Payload to send** (RSVP is the reported case): an event RSVP notification, so
the tap should land on `/experience/<id>`. Use a real RSVP from a second account
or an FCM test-send of the `experience_id` data payload (see Step 4).

For **each** cell below — {Android, iOS} × {foreground, background (process
alive), terminated} — tap the notification and confirm:

1. The app lands on the event detail screen (not Home).
2. Firebase **DebugView** shows a `notification_deeplink` event with
   `phase: nav_result` and `landed: true` (enable DebugView per
   `firebase.google.com/docs/analytics/debugview`; on Android
   `adb shell setprop debug.firebase.analytics.app <package>`).
3. If it did **not** land on the first try, confirm a follow-up event with
   `trigger: replay` and `recovered: true` appears (the retry rescued it), and
   the screen ends on the event. `recovered: false` or a `nav_result` with
   `landed: false` and no recovery is a **gate failure** — capture the `source`
   and `trigger` and file against #2636.

| State | Android landed | iOS landed | Recovered if missed | Notes |
|-------|:--------------:|:----------:|:-------------------:|-------|
| Foreground (`source: local`) | ☐ | ☐ | ☐ | Local notification tap |
| Background, process alive (`source: opened` / `missed`) | ☐ | ☐ | ☐ | Home button, then tap |
| Terminated (`source: initial`) | ☐ | ☐ | ☐ | Force-quit, then tap |

Cross-check the `source` field matches the state you exercised — a mismatch
(e.g. a terminated tap arriving as `missed` rather than `initial`) is itself a
finding worth noting on the issue.

DebugView here works because your device is in debug mode. **In the field** (real
prod devices you can't put in DebugView), a permanent drop instead surfaces as a
`NotificationDeepLinkDroppedException` **Crashlytics non-fatal** — that, plus the
BigQuery analytics export, is how a user's dropped tap becomes diagnosable
without their device on your bench.

### Debugging

**Check Server Logs:**
```bash
# Grep for FCM sends
grep "FCM notification" server.log

# Watch live logs
tail -f server.log | grep FCM
```

**Check Flutter Logs:**
```bash
# Run with verbose logging
flutter run --verbose

# Filter FCM logs
flutter logs | grep FCM
```

**Common Issues:**

| Issue | Solution |
|-------|----------|
| **"Failed to register device token: Unauthenticated"** | Ensure you're logged in. FCM initializes after authentication. |
| **"No device token"** | iOS: Enable capabilities in Xcode<br>Android: Verify `google-services.json` location<br>Check notification permissions granted |
| **"Notification not received"** | Check server logs for FCM send confirmation<br>Verify device token registered on server<br>Check Firebase console app configuration<br>iOS: Must use real device, not simulator |
| **"Permission denied"** | Grant notification permission when prompted<br>Reset: Settings → App → Notifications |
| **"SenderId mismatch"** | App and server are using different Firebase projects<br>**Fix**: Verify `PROJECT_ID` in `GoogleService-Info.plist` matches `project_id` in server's Firebase credentials JSON<br>Download matching credentials from the same Firebase project |
| **"Auth error from APNS or Web Push Service"** | APNs authentication not configured in Firebase (iOS only)<br>**Fix**: Upload APNs authentication key (.p8) or certificate (.p12) to Firebase Console → Project Settings → Cloud Messaging → Apple app configuration<br>See "Configure APNs for iOS Push Notifications" section above |
| **"Skipping notification - no registered devices"** | Device token not registered with server<br>**Fix**: Ensure `ENABLE_NOTIFICATIONS=true` in app environment config<br>Check Flutter logs for successful device registration<br>Verify notifications permission granted on device |

### Success Criteria

- Server starts with FCM provider initialized
- App requests and receives notification permission
- Device token registered with server on login
- Server logs show FCM message sent
- Notification appears on device
- Tapping notification opens app
- Works in foreground, background, and terminated states

## Future Enhancements

### UI Integration
- Fetch latest community events when notification received
- Show custom in-app notification UI

### Testing & Polish
- Write unit tests for notification service
- Write integration tests for device service
- Add metrics and logging
- Load testing
- FCM analytics: Monitor delivery metrics in Firebase console

### Advanced Features
- **Rich notifications**: Include images, action buttons
- **Notification grouping**: Batch multiple events into single notification
- **User preferences**: Per-event-type notification settings
- **Badge counts**: Show unread count on app icon
- **Custom sounds**: Platform-specific notification sounds
- **Web Push**: Support for browser-based clients
- **Email fallback**: Send email digests if push notifications fail repeatedly
- **Notification history**: Track delivery for debugging
- **Multi-device sync**: Mark notifications as read across devices
