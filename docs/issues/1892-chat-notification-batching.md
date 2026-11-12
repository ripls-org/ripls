# Issue #1892: Chat notifications should batch and dedupe

**Status (2026-05-13):** Phases 1–4 shipped via PR #1909, plus an additional commit (`62f27a8db`) adding a `conversation_title` field to the FCM payload so the client can render the topic name as the notification heading. Phases 5 and 6 split into separate follow-up issues:

- **#1910** — iOS Communication Notifications (Notification Service Extension + `INSendMessageIntent` + lock-screen avatar + Focus integration). Plan of record: one-time Xcode UI session to scaffold the target, with the resulting `project.pbxproj` diff committed.
- **#1911** — Inline reply (Android `RemoteInput` + iOS `UNTextInputNotificationAction`). Includes Phase 7 telemetry as a fold-in.

The plan-of-record sections for Phases 5–7 below are preserved as historical context and as the seed material for the follow-up issues. They are no longer the authoritative source — see the issues above.

## Known issues observed during validation (PR #1909, still open)

These are real bugs surfaced when testing the shipped Phases 1–4 + `conversation_title` on Android emulators. Each was investigated during the original session; fixes were attempted and rolled back to keep the PR focused. Reproductions, evidence, and starting points for the next pass are below.

### 1. Android notification heading concatenates `conversationTitle` and `contentTitle`

**Observed:** `dumpsys notification --noredact` on a chat notification shows `android.title=String (Camping Stove: Nancy Park)` instead of either `"Camping Stove"` alone or `"Nancy Park"` alone. `NotificationCompat.MessagingStyle` combines the `conversationTitle` it was given with the notification's `contentTitle` when both are set.

**Cause:** `ChatNotificationManager.renderForChatMessage` sets both `MessagingStyleInformation.conversationTitle = conversation_title` *and* passes `title: senderName` to `_localNotifications.show()`. Android's compat library renders the title slot as `"${conversationTitle}: ${contentTitle}"`.

**Next pass:** set only one. Two equivalent options:
- Pass `show().title = conversation_title.isNotEmpty ? conversation_title : senderName` and drop `MessagingStyleInformation.conversationTitle` entirely. The Person.name prefix on each Message keeps multi-sender distinction in the expanded transcript.
- Keep `MessagingStyleInformation.conversationTitle = conversation_title` and pass `title: null` to `show()` (lets MessagingStyle pick the heading).

Either yields a single clean heading. Reverted attempt lived at commit `cb4aaee4f` (now in the reflog only); it took the first approach.

### 2. Android auto-generated group-summary picks a misleading header

**Observed:** When 2+ chat notifications exist, Android shows a cluster summary at the top with avatars sampled across the underlying notifications and a heuristic title (often a participant's name from a different conversation, not the topic the user is currently looking at). The user reported "Nancy Park" + three small avatars as a summary above a Camping Stove conversation card.

**Cause:** `ChatNotificationManager.renderForChatMessage` sets `groupKey: 'chats'` on every per-conversation notification but never posts an explicit `setAsGroupSummary(true)` notification. Per Android docs, without an explicit summary the system auto-generates one — and its heuristic doesn't know which conversation is "primary," so it picks across the cluster.

**Next pass:** post a dedicated group-summary notification alongside each per-conversation render:
- Stable fixed `id` (use a distinctive constant like `0x52495053` to avoid colliding with `conversationId.hashCode`)
- `setAsGroupSummary: true`, `groupKey: 'chats'`, same `channelId: 'chats'`
- `GroupAlertBehavior.children` + `onlyAlertOnce: true` so the summary stays silent — only the per-conversation children fire sound/vibration
- Title: the localized "Chats" channel name; body: null

Also extend `clearForConversation` to query `getActiveNotifications()` and cancel the summary when no children with the chats `groupKey` remain. Android does NOT auto-remove explicit summaries when their cluster empties.

Reverted attempt lived at commit `ebc66805e` (reflog only); it shipped 5 unit tests covering summary post / refresh / conditional cleanup behavior. Tests are recoverable from the reflog if useful.

### 3. iOS aps-environment mismatch for local debug builds

**Observed:** `Runner.entitlements` declares `aps-environment = production` (set by #1027 to fix TestFlight delivery). That's correct for Release/TestFlight but wrong for Debug — both iOS Simulator and locally-signed Debug builds on physical iPhones reach the **sandbox** APNs gateway (`sandbox.push.apple.com`), so a production-declared app never receives a token from the gateway it actually connects to.

**Next pass:** split into per-configuration entitlements files:
- New `app/ios/Runner/Runner.Debug.entitlements` with `aps-environment = development` and the same `com.apple.developer.associated-domains` as Release.
- Update `project.pbxproj`: change `CODE_SIGN_ENTITLEMENTS` for the **Debug** and **Profile** build configurations (lines around 559 and 759, identifiable by the surrounding `DISPLAY_NAME = "Ripls Dev"` / `INFOPLIST_KEY_CFBundleDisplayName = "Ripls Dev"` strings) to point at the new file. **Release** stays pointing at `Runner.entitlements` — TestFlight delivery unchanged.

Reverted attempt lived at commit `393637a9a` (reflog only); it was a two-line `project.pbxproj` diff plus the new entitlements file.

### 4. iOS APNs token timing on cold start

**Observed:** `firebase_messaging.getToken()` throws `[firebase_messaging/apns-token-not-set]` when called before iOS finishes registering with APNs. That registration is asynchronous and continues after `application:didFinishLaunchingWithOptions:` returns. On cold start, our FCM init calls `getToken()` immediately, the throw bubbles up, and the device never registers with our server — visible as no row in `user_device` and `WARNING: Failed to get device token` in the app log.

**Next pass:** before calling `getToken()` on iOS, poll `getAPNSToken()` until it returns non-null (or a bounded timeout). A reasonable cap is 32 attempts × 250 ms = 8 seconds; the typical case completes sub-second, the cap covers slow simulators, and a timeout logs a warning and skips registration so the next app launch retries naturally. Only run on iOS; Android's `getToken()` doesn't need this wait.

Reverted attempt lived at commit `a561b52df` (reflog only); it added `_awaitApnsToken` to `FCMService` and 4 unit tests (token-ready-immediately / token-after-N-polls / never-arrives-timeout / transient-exception). Tests are recoverable from the reflog.

### 5. iOS path not validated end-to-end

**Observed:** No iOS device — physical or simulator — ever registered with the local dev server during this session, so the iOS rendering path was not exercised end-to-end. The server-side `aps.alert.subtitle` from `conversation_title` is covered by unit tests in `server/notifications/fcm/provider_test.go` and the chat-subscriber tests, but actual lock-screen rendering on a real device is unverified.

**Cause and constraints:**
- **iOS Simulator** can't easily receive real cloud APNs delivery: Flutter's `flutter run --debug` for simulator uses ad-hoc signing (`CODE_SIGN_IDENTITY = -`), which sets `ENTITLEMENTS_ALLOWED = NO` and strips entitlements from the binary. Without `aps-environment` in the binary, iOS doesn't ask APNs for a token. Even with the developer portal, dev cert, and dev provisioning profile correctly in place on the Mac, Flutter doesn't substitute real Apple Development signing for simulator targets. `xcrun simctl push <device-uuid> <bundle-id> <payload.json>` is the supported alternative for testing payload-shape changes (bypasses APNs entirely; doesn't exercise FCM round-trip).
- **Physical iPhone** uses real Apple Development signing via `flutter run` so APNs registration works, but reaching the local server needs (a) `NSAllowsLocalNetworking` in `Info.plist` (since iOS App Transport Security blocks plain HTTP to non-localhost hosts by default), and (b) `--dart-define=SERVER_URL=http://<mac-lan-ip>:8080`.

**Next pass:** wire up physical-iPhone-against-local-dev path as the primary iOS validation route. The work in #1 + #2 + #4 above lands on Android-only behaviors; #3 (entitlements split) is iOS-only and is what unblocks the physical iPhone path. After that's done, the simulator is still useful for rendering tests via `xcrun simctl push` but not for full FCM-round-trip validation.

### 6. (Stretch) iOS Notification Service Extension is the right long-term path for #5

If iOS chat notifications need to render with the same fidelity as Android (sender avatar on lock screen, Communication Notification tier, Focus integration), follow-up issue **#1910** is the work. The path of least surprise is to first land #1–#4 above and verify the basics work on a physical iPhone, then come back for NSE polish.

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1892
**Filed:** 2026-05-12 | **Last activity:** 2026-05-12
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none provided. Adjacent reading consulted:
`docs/push_notifications.md`, `docs/client/chat.md`,
`docs/issues/1027-ios-notifications.md`,
`docs/issues/1107-per-community-notifications.md`,
`docs/issues/1133-stale-chat-notification.md`.

## Problem Statement

Today every chat message produces a separate, independent system notification.
On an active thread this stacks rapidly: a 20-message back-and-forth becomes 20
lock-screen entries that the user must dismiss one at a time. Modern chat apps
collapse messages from a single conversation into one notification that updates
in place, surfacing the most recent message and a count or short transcript of
preceding messages.

The fix is a coordinated client/server change. The server already carries
enough information in the FCM payload to identify a conversation; the
client/server are simply not using any of the platform grouping APIs
(`NotificationCompat.MessagingStyle`, Android `setGroup`, iOS
`thread-identifier`, stable per-conversation notification IDs, conversation
shortcuts). This issue is to adopt the standard chat-app pattern: one
notification per conversation, updated in place, dismissed in one swipe, and
cleared the moment the user reads the thread in the app.

## Context from Comments

No comments — the issue is a single user-feedback submission from the
maintainer (user2@example.org) on Android 16 / Pixel 9 Pro, app v0.20.1, build
6691, PROD. No debate to resolve. The description is short but
unambiguous: "research modern chat app notification management and emulate
this functionality." That instruction is treated as the spec.

## Industry Research

Researched best-in-class chat apps (WhatsApp, Signal, Telegram, iMessage,
Slack, Discord) and the platform docs they depend on (Android conversation
notifications, iOS Communication Notifications, FCM/APNs). The convergent
pattern across every one of them is:

### Android (Android 11+, the current floor for conversation features)

1. **One stable notification ID per conversation.** Re-posting the same ID
   replaces the prior notification — that's how "20 messages → 1 entry"
   works at the system level. Today we use `RemoteNotification.hashCode` per
   message; that gives a new ID per push, so the system can't dedup.
2. **`NotificationCompat.MessagingStyle`** is the chat-specific notification
   style. The app supplies the "self" `Person` once and a list of
   `MessagingStyle.Message(text, timestamp, sender_person)` entries; the
   system renders the multi-bubble UI ("Alice: hi · Alice: are you there"),
   handles long-press for inline reply, and surfaces the conversation in the
   Conversation Space when the device supports it. This is the API every
   first-class messenger uses.
3. **`setGroup(conversation_or_app_key) + a summary notification`** to keep
   multiple conversations visually grouped under the app. Auto-grouping
   exists on Android 7+ but explicit grouping is more deterministic.
4. **Long-lived dynamic shortcut + `setShortcutId()` + `LocusId`** unlocks
   the Conversation Space (Android 11+), inline replies, bubbles, and
   conversation widgets. Optional polish layer; not required to fix the
   user's complaint.
5. **`cancel(notification_id)` on conversation open** so the notification
   disappears the moment the user is reading the thread.
6. **MessagingStyle requires client-side state.** The server cannot render a
   thread of recent messages on its own — only the client knows the prior
   messages still visible on the device. The client must persist a small
   "last N messages per conversation" cache and reconstruct the style every
   time a new message arrives. SharedPreferences is the standard backing
   store because it survives process death and is accessible from the
   Android background isolate.

### iOS

1. **`thread-identifier`** in the APNs payload (set to `conversation_id`)
   makes iOS auto-stack all notifications from the same thread. This alone
   gives 80% of the desired UX on iOS — multiple messages in one thread are
   grouped, expandable, and dismissible as a unit.
2. **`apns-collapse-id`** is a different tool: it *replaces* the prior
   notification with the same id, so the user sees only the latest message.
   Standard chat apps use **thread-identifier (grouping)**, not
   collapse-id (replacement), so each message remains individually visible
   inside the expanded thread. We follow that convention.
3. **`UNNotificationServiceExtension` + Communication Notifications
   (iOS 15+)** elevates chat notifications into the system's Communication
   tier (same priority/affordances as Messages.app), shows the sender's
   avatar inline, and lets users mute or prioritize specific conversations
   via iOS Focus modes. Requires `INSendMessageIntent` donation and a
   small Notification Service Extension target.
4. **Inline reply** is supported on both platforms and is the natural
   completion of the modern chat-notification UX:
   - Android: `RemoteInput` attached to a notification action triggers
     the system inline reply UI (Android 7+). The typed text is
     delivered to the app via a `PendingIntent`, even when the process
     is dead — the system spawns a background isolate to handle it.
   - iOS: `UNTextInputNotificationAction` registered on a
     `UNNotificationCategory` does the same thing (iOS 9+). Combined
     with the Communication Notification category from above, the
     reply UX matches Messages.app.

   Both require server-side knowledge of which payloads are chat
   messages (already true via `data['type'] == 'chat_message'`) and a
   minimal background-isolate path that can authenticate to the chat
   send RPC.
5. **`UNUserNotificationCenter.removeDeliveredNotifications(...)`** to
   clear-on-read. iOS doesn't expose a stable per-thread cancel — we
   identify by the notification's `identifier` (FCM/APNs sets this from the
   payload).

### FCM payload mechanics

1. **`collapse_key` is not the answer.** Per FCM docs, collapse_key only
   coalesces undelivered messages while the device is offline, and is
   ignored for `notification` payloads. It does not help when the device is
   online and receiving messages back-to-back — which is the user's
   scenario.
2. **Data-only payloads on Android.** If we want client-side rendering of
   MessagingStyle, we must drop the top-level `notification` field on
   Android for chat messages and switch to data-only. Otherwise the system
   displays the notification before our code runs in the background and we
   never get a chance to render the styled version. iOS keeps `aps.alert`
   so the system displays immediately while the NSE optionally enriches.
3. **Background isolate constraint.** Flutter's background message handler
   runs in a separate isolate with no access to in-memory state. The
   per-conversation message cache must live in `SharedPreferences` (or
   another isolate-safe store) and be re-read on every notification.

### What the leading chat apps do

- **WhatsApp / Signal / Telegram**: MessagingStyle + per-conversation
  stable IDs + thread-identifier on iOS + Notification Service Extension
  for rich rendering. All cancel-on-read.
- **iMessage**: Apple's reference Communication Notifications target —
  thread-id + INSendMessageIntent + avatar.
- **Slack / Discord**: Same Android pattern; iOS uses thread-identifier
  per channel/DM and grouped lock-screen presentation.

The pattern is industry-standard and well-supported on Flutter via
`flutter_local_notifications` (`MessagingStyleInformation`,
`AndroidNotificationDetails.groupKey`,
`DarwinNotificationDetails.threadIdentifier`) and Firebase Admin Go SDK
(`messaging.AndroidNotification`, `messaging.APNSPayload`).

## Codebase Findings

**Server-side notification dispatch** (`server/notifications/`):
- `server/notifications/chat_subscriber/subscriber.go:83` — `Subscriber.Handle()`
  is the single entry point for chat-message pushes. Resolves recipients,
  gates on preferences and active stream + foreground, dispatches per user
  via `notifications.Service.NotifyUser`. This is the right chokepoint.
- `server/notifications/chat_subscriber/copy.go:14` — `buildNotification()`
  assembles a `models.Notification` with title (sender name), body (preview
  text), and a `ChatMessage` payload. No grouping fields exist on the
  shared `models.Notification` proto today.
- `server/notifications/fcm/provider.go:187` — `buildMessage()` constructs
  the FCM `Message`. Sets `android.priority=high`, `aps.badge=1`,
  `content-available`, `mutable-content`, `apns-push-type=alert`. Sets the
  top-level `messaging.Notification` *and* `android.Notification`, which is
  what blocks client-side rendering on background. **Sets no
  `apns-collapse-id`, no `thread-id`, no Android `tag`, no `collapse_key`**
  (also lacks `apns-priority`, which should be `10` for alerts —
  pre-existing gap, not in scope here).
- `server/notifications/fcm/provider.go:261` — `buildDataPayload()` already
  passes `conversation_id`, `message_id`, `sender_user_id`, `sender_name`,
  `preview_text`, plus optional `community_id`/`experience_id`/`gear_id`/
  `request_id`. We have everything the client needs.

**Client-side notification handling** (`app/lib/services/`):
- `app/lib/services/fcm_service.dart:361` — `_showLocalNotification()` is the
  foreground render path. Posts with `id: notification.hashCode` (per-
  message, not per-conversation), no `groupKey`, no `MessagingStyleInformation`,
  no `threadIdentifier`. This is the line that produces the user's pain.
- `app/lib/services/fcm_service.dart:447` —
  `firebaseMessagingBackgroundHandler` is the background-isolate entry point.
  Currently a logging no-op. When we switch chat messages to data-only on
  Android, this is where the styled notification must be rendered.
- `app/lib/services/fcm_service.dart:185` — Single channel
  `'community_events'` is registered. A second `'chats'` channel is the
  industry norm so users can mute non-chat events while keeping chats.
- `app/lib/presentation/viewmodels/conversation_messages_notifier.dart:135` —
  `loadMessages()` already calls `chatRepository.markMessagesRead(conversationId)`
  on open. This is the natural hook for `cancel(notification_id)` —
  cancel on the same call.
- `app/lib/data/repositories/unread_count_repository.dart` — Already tracks
  unread counts per conversation. Useful as a fallback label
  ("3 new messages") when the MessagingStyle cache is empty (e.g.
  after a fresh install or cache eviction).

**iOS platform config** (`app/ios/Runner/`):
- `app/ios/Runner/AppDelegate.swift` is minimal; no notification service
  extension target exists. Adding one is a per-target Xcode change, not a
  pure Dart edit.
- `app/ios/Runner/Runner.entitlements` — `aps-environment` already
  `production` after #1027 landed. No work here.

**Adjacent work / overlap:**
- **#1027 (iOS notifications)** — Closed; the APNs payload basics are in
  place. We layer thread-id on top.
- **#1133 (stale chat notification)** — Same code path; orthogonal bug
  about message freshness on tap. The cancel-on-open hook we add here
  shares the same "user just opened conversation" trigger as that fix.
- **#1107 (per-community notifications)** — Adds preference-gated
  filtering (already merged based on `subscriber.go:132`). Per-community
  `notify_chats` toggle exists; our work is downstream of it (we only
  render notifications that actually got sent).
- **Notification channel design** — A `chats` channel is a natural
  successor to the current `community_events` channel. Touches #1551
  (native notification categories) which is open; we should coordinate
  but not block.

## Architecture Audit (2026-05-13)

Audited against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/client/accessibility.md`,
`docs/client/testing.md`, `docs/server/architecture.md`, and
`docs/server/observability.md`. Findings folded into the relevant
phases above; summarized here for reviewers.

- **Layer boundary (now in Phase 2).** First draft had
  `ConversationMessagesNotifier` calling `ChatNotificationManager`
  directly. Per `client_architecture.md`: ViewModels call only
  Repositories; Repositories call Services. Corrected: extend
  `ChatRepository.markMessagesRead` to fan out to the manager. The
  ViewModel's existing call is unchanged.
- **Caching layer (now explicit in Phase 2).** Confirmed the
  per-conversation message cache must not live in Stash. Stash is
  memory-only per `client_caching.md` and isolate-scoped; the
  notification cache must survive process death and be readable
  from the Android background isolate. `SharedPreferences` (Android)
  and an App Group container file (iOS) are the right backings.
- **Accessibility (now explicit in Phase 2).** Notification a11y is
  OS-driven; the `accessibility.md` widget primitives don't apply.
  Documented so reviewers don't expect `Tappable`/`Semantics`
  wrappers in the rendering path.
- **i18n (already correct).** Channel names, action labels, and
  reply hints all routed through `app_en.arb` + `app_es.arb` per
  `i18n.md`. The `npm run generate` step is included.
- **Testing (now explicit in Phase 2/6).** Tests use
  `ProviderContainer` + Mockito mocks per `testing.md`. Added an
  explicit ViewModel/Repository test for the cancel-on-read
  fan-out. The reply handler tests cover the parser, the channel
  handler, and the end-to-end Android background flow.
- **Server file-size rule (now noted in Phase 1/3).**
  `server/notifications/fcm/provider.go` is ~290 lines today. The
  Phase 3 data-only Android branch likely pushes it past ~600 with
  the helpers and tests. Documented a split into a new
  `chat_message.go` if it crosses ~700 lines, well under the
  1000-line ceiling from `server_architecture.md`.
- **Server observability (now explicit in Phase 7).** Log fields use
  the standard names from `observability.md`: `operation`,
  `conversation_id`, `external_service`, `user_id`. No ad-hoc field
  names. Message body is never logged — chat content is sender-
  authored user content. Levels follow the existing rubric: INFO
  for routine sends, WARN for handled fallbacks, ERROR for
  delivery failures.
- **Service independence (no violation).** Server changes are
  scoped to `server/notifications/fcm/` (shared library) and the
  chat-subscriber, which already calls into `notifications.Service`
  via interface. No new service-to-service coupling introduced.
- **Connect error handling (no impact).** This work doesn't add
  new RPC handlers, so the `connecterr.Internal(...)` requirement
  from CLAUDE.md doesn't apply. The existing FCM provider already
  returns wrapped errors from `Send`; we don't change that
  signature.

## Potential Solutions

### Option A: Client-side MessagingStyle + per-conversation stable IDs

- **Approach.**
  - Server: switch chat-message FCM payloads to *data-only on Android*
    (drop `messaging.Notification` and `android.Notification` for the
    `ChatMessage` payload variant; keep them for other variants). Add
    `aps.thread-id = conversation_id` to iOS payloads (keep `aps.alert`
    intact). No other server changes.
  - Client (Android): in the foreground handler and the background
    handler, render a local notification via
    `flutter_local_notifications` with:
    - `id = conversationId.hashCode` (stable per conversation)
    - `MessagingStyleInformation` populated from a SharedPreferences-
      backed per-conversation message cache (`Person` for sender,
      add the new message, trim to last ~6)
    - `groupKey = 'chats'`, plus a summary notification, on a new
      `chats` Android channel
    - `setShortcutId` left for a later phase (Option C polish)
  - Client (iOS): rely on the new `thread-identifier` for grouping; the
    foreground render path passes
    `DarwinNotificationDetails.threadIdentifier` so the foreground-
    rendered local notification matches what APNs would have used.
  - Cancel on open: in `ConversationMessagesNotifier.loadMessages()`, after
    `markMessagesRead`, call `_localNotifications.cancel(conversationId.hashCode)`
    via the new `NotificationManager` service.
- **Pros.** Fixes the user's actual complaint. Matches what every modern
  chat app does. All payload data already exists. Flutter plugin support
  is first-class.
- **Cons.** Touches both server and client. The "data-only on Android"
  switch requires the background handler to actually render — which means
  the background isolate has to load the cache and the localizations.
  Background-isolate gotcha is real but well-documented (SharedPreferences
  works across isolates; we need to be careful with l10n in the
  background path — for chat we don't translate user content, so this is
  manageable).
- **Effort.** Medium.

### Option B: Server-side coalescing / collapse only

- **Approach.** Keep the client renderer almost unchanged. On the server,
  add a short debounce window per (recipient, conversation): if a push
  was already sent for this conversation within the last 5–10 seconds,
  replace it with an aggregate ("3 new messages from Alice") instead of
  appending. Add `collapse_key` and `apns-collapse-id` so the *latest*
  push wins when offline.
- **Pros.** Smaller surface change. No client architectural shifts. No
  Android channel split required.
- **Cons.**
  - `collapse_key` is ignored for `notification` payloads (FCM docs); we'd
    need the same data-only switch anyway to get it to work — losing the
    "no client changes" benefit.
  - Server has no view into what's currently visible on the device. Two
    quick messages 2s apart still create two notifications if delivery
    crosses the debounce boundary. The user reports this exact case
    ("threads that are active even a bit").
  - Doesn't deliver the modern feel — no per-thread message preview, no
    inline reply, no grouped lock-screen layout. The user explicitly
    asked us to "emulate" what modern chat apps do, not just reduce the
    count.
- **Effort.** Small-medium. Rejected as too narrow.

### Option C: Full Communication Notifications (Option A plus native polish)

- **Approach.** Everything in Option A, plus:
  - Android: register long-lived `ShortcutInfoCompat` per conversation
    with `Person`, `LocusId`, `setLongLived(true)`, `setIsConversation`;
    call `setShortcutId()` on the notification. Enables Conversation
    Space, conversation widgets, bubbles.
  - iOS: add a `RunnerNotificationService` extension target. On each
    push, donate an `INSendMessageIntent` to elevate to a
    Communication Notification (avatar, system Focus integration).
    Requires App Group entitlement and per-conversation
    `INPerson` resolution.
- **Pros.** Closes the gap to iMessage / WhatsApp on first-class
  surfaces. Better lock-screen presence, system mute affordances.
- **Cons.** Native Android shortcut plumbing is a non-trivial Flutter
  bridge (no first-class plugin support for ShortcutInfoCompat with
  Person + LocusId; needs platform-channel glue). iOS NSE target is a
  separate Xcode target; CI/release plumbing for Codemagic (or wherever
  we build) must learn about it. Sizeable scope.
- **Effort.** Large. Best as a Phase 4 follow-up after Option A lands
  and we've confirmed the foundation behaves.

## Recommendation

**Option C: full Communication Notifications.** Build the Option A
foundation first (it is a strict prerequisite — Conversation Space and
Communication Notifications both require MessagingStyle + thread-id +
stable per-conversation IDs to be in place), then layer the
platform-native polish on top: long-lived dynamic shortcuts with
`Person` + `LocusId` on Android, and a Notification Service Extension
that donates `INSendMessageIntent` on iOS. The deliverable matches
iMessage / WhatsApp on both lock-screen presentation and OS-level
affordances (Conversation Space ranking, Focus modes, system mute,
bubbles, avatars on the lock screen).

Why pay the larger cost up front:
1. **The user's complaint is the visible symptom of a deeper gap.** Once
   we land MessagingStyle the next round of feedback will be "why
   doesn't this look like iMessage" — Communication Notifications is
   what closes that gap.
2. **The two halves share infrastructure.** Both Android shortcut
   registration and iOS NSE need the same `ConversationDirectory` of
   "last-seen conversations the user has" and the same client-side
   message cache. Building them separately re-litigates the cache
   design twice.
3. **Risk is well-bounded.** Each phase below is independently
   shippable. If iOS NSE turns out to be a CI/release blocker we ship
   Phases 1–4 and Android gets the full Option C while iOS stays at
   Option A (thread-id grouping only). That's still a large win.
4. **Cost is mostly one-time.** The NSE target setup, App Group
   entitlement, and platform channel for shortcuts are
   ceremony-heavy but small in line count. They become reusable
   foundation for any future rich-notification work (richer
   community-event notifications, reactions, mentions, etc.).

Reject Option B: `collapse_key` is ignored on notification payloads, so
the simplest server-only fix degrades to the same Android data-only
shift Option A needs anyway, while delivering visibly worse UX.

Reject "Option A only": settling for plain MessagingStyle leaves the
Conversation Space and Communication Notification tiers unused —
those tiers are where modern chat apps live on the lock screen and
how they win user trust on iOS Focus modes. The incremental work to
add them is meaningful but not prohibitive, and the foundation phases
are identical.

## Open Questions

- **Q1: Should we add a separate `chats` notification channel, or reuse
  `community_events`?** — *Recommendation:* add a new `chats` channel
  (low-touch in Phase 1). Industry norm is one channel per category;
  users routinely mute community-event noise on Android while keeping
  chat. The existing channel does not need to be deleted — pre-existing
  installs keep their preferences; new ones receive both channels.
  Document the migration in `docs/push_notifications.md`.

- **Q2: How many recent messages should the MessagingStyle cache hold per
  conversation, and for how long?** — *Recommendation:* last 6
  messages, trimmed to the last hour. Six is enough to fill the
  Android conversation card without being noisy on the lock screen,
  and matches what `MessagingStyle` itself shows by default. The
  hour TTL prevents stale "yesterday" messages from re-surfacing when
  a single new message arrives the next day. Total per-app storage
  bounded at ~50 KB even at 200 active conversations.

- **Q3: Where does the per-conversation message cache live?** —
  *Recommendation:* `SharedPreferences` keyed on
  `chat_notif_cache:<conversation_id>`. Persists across process death,
  accessible from the background isolate without extra init, no schema
  migration. Hive is overkill for this volume and adds isolate-init
  ceremony in the background path. The repository pattern doesn't
  apply here because this data never round-trips to the server — it is
  display-side only. Keep it in a small `NotificationManager` service.

- **Q4: Should the server send `apns-collapse-id` in addition to
  `thread-identifier`?** — *Recommendation:* no. Thread-id groups
  multiple distinct notifications under one expandable thread (the
  desired UX). Collapse-id would replace earlier messages so the user
  only ever sees the latest one — not what modern chat apps do. Stay
  consistent with WhatsApp/iMessage convention: group, don't replace.

- **Q5: What is the notification ID strategy across platforms?** —
  *Recommendation:* on Android, `conversationId.hashCode` is the
  notification ID (stable, replaces in place). On iOS, the APNs
  `apns-collapse-id` would normally serve this role; since we are
  *not* using it, iOS relies on `thread-identifier` for grouping, and
  individual messages keep their own APNs-assigned identifier. For
  foreground iOS rendering via `flutter_local_notifications`, we use a
  per-message ID and set `threadIdentifier = conversationId` so the
  system still groups them.

- **Q6: What happens on a fresh install / cache miss?** —
  *Recommendation:* fall back to single-message rendering ("Alice: hi")
  with `MessagingStyle` containing just that one message. Optionally,
  if the unread-count repository has data, append "(3 unread)" to the
  conversation title via `MessagingStyleInformation.conversationTitle`.
  Never block the notification on a cache fetch — render immediately
  with what we have.

- **Q7: How do we handle community vs. 1:1 conversation rendering in
  MessagingStyle?** — *Recommendation:* set
  `groupConversation: true` and supply the conversation's display name
  as `conversationTitle` when `community_id` is present. Otherwise,
  `groupConversation: false` and let MessagingStyle infer from the
  Person on each Message. The current payload includes
  `community_id`; that's the signal.

- **Q8: Cancel on read — what about messages that arrive during the
  cancel cycle?** — *Recommendation:* the same conversation_id-stable
  notification ID makes this benign: cancel happens, then the next
  message immediately re-posts under the same id. The lock-screen
  briefly empties and refills, which is the correct behavior. No race
  window where the user gets duplicate notifications.

- **Q9: Should the background isolate path be the same as foreground?**
  — *Recommendation:* extract a top-level `renderChatNotification(data)`
  helper that both handlers call. Background isolate needs zero
  Flutter-engine state — just `SharedPreferences` + the local
  notifications plugin (which initializes itself from the manifest/
  Info.plist). Both paths share the cache write so the next render
  sees the freshly added message.

- **Q10: Do we need any proto changes?** — *Recommendation:* none.
  Every field the client needs is already in `ChatMessagePayload`
  (`sender_name`, `preview_text`, `conversation_id`, `community_id`).
  No new RPCs, no migration. Server changes are isolated to
  `server/notifications/fcm/provider.go`.

## Implementation Plan

### Phase 1: Server payload changes (FCM provider) ✅ SHIPPED 2026-05-13

**Goal:** Get the payload shape right so the client has what it needs to
render MessagingStyle, without changing client behavior yet. Behavior on
Android improves immediately as a side effect — `notification.tag`
causes system-level dedup, so background chat pushes now collapse to
"latest message wins" per conversation instead of stacking. iOS gains
thread-identifier grouping (each message still visible, but stacked
under one expandable thread).

**Tasks:**
- [x] In `server/notifications/fcm/provider.go:buildMessage`, when the
      payload is a `Notification_ChatMessage`:
      - Set `apns.aps.ThreadID = chat_message.conversation_id`.
      - Add `apns-priority: "10"` header.
      - Add Android `notification.tag = chat_message.conversation_id`.
      Type-switch lives in helper `chatConversationIDFor()`.
- [x] No file split needed in Phase 1 (`provider.go` is now ~310 lines,
      well below the 700-line trigger). Revisit in Phase 3 when the
      data-only branch lands.
- [x] Added unit tests in `server/notifications/fcm/provider_test.go`:
      `TestBuildMessage_ChatMessageGrouping` (5 cases — Android chat
      sets tag, Android non-chat omits tag, iOS chat sets thread-id +
      apns-priority, iOS non-chat omits both, empty conversation_id
      omits grouping fields on both platforms) and
      `TestChatConversationIDFor` (4 cases — chat returns id, community
      event returns "", no payload returns "", nil ChatMessage returns "").
- [x] `npm run build` clean (gofumpt + golangci-lint + check-goroutines
      + codeinternal + event-terminology + active-community-gate).
- [x] `go test ./server/notifications/...` green across fcm, chat_subscriber,
      community_subscriber, notifications, and noop packages.

**Verification gate (must be green before Phase 2 starts):**
- `go test -v ./server/notifications/fcm/ -run 'TestBuildMessage_ChatMessageGrouping|TestChatConversationIDFor'`
  → 9/9 pass.
- `npm run build` → exit 0.

**Commits:**
- `8a08603d6` — plan doc.
- `b27f7eb40` — `fcm: group chat notifications by conversation_id (#1892 phase 1)`.

**Validates:** iOS notifications visibly group by conversation on the
lock screen the moment a build goes out, and Android background pushes
collapse to latest-message-per-conversation — a directly user-visible
win with zero client risk.

### Phase 2: Client foreground rendering with MessagingStyle (Android + iOS) ✅ SHIPPED 2026-05-13

**Goal:** Replace the per-message hashCode notification with a
conversation-stable, MessagingStyle-rendered one on the foreground
path. Background path still goes through the old behavior — we'll fix
that in Phase 3 once the foreground rendering is proven.

**Tasks:**

*New service:*
- [ ] Add `app/lib/services/chat_notification_manager.dart`. Responsibility:
      - Maintain per-conversation `RecentMessage` cache in
        SharedPreferences under key
        `chat_notif_cache:<conversation_id>` — JSON-encoded list of
        `{senderName, senderUserId, body, timestampMs}`, trimmed to
        last 6 entries and entries older than 60 minutes.
      - Expose `renderForChatMessage(Map<String, dynamic> data)` that:
        - decodes the FCM data fields,
        - appends the new message to the cache,
        - constructs `MessagingStyleInformation` (Person for "me",
          Person per distinct sender, `Message` per cached entry,
          `groupConversation` from `community_id` presence,
          `conversationTitle` from sender_name or a future
          conversation-title lookup),
        - posts via `_localNotifications.show(id: conversationId.hashCode, …)`
          with `groupKey: 'chats'` on Android and
          `threadIdentifier: conversationId` on iOS,
        - emits a one-time `chats` group summary on Android,
        - logs the render at `debug` with conversation_id and
          message_id.
      - Expose `clearForConversation(String conversationId)` that calls
        `_localNotifications.cancel(conversationId.hashCode)`.
- [ ] Add unit tests under
      `app/test/services/chat_notification_manager_test.dart`:
      cache append + trim, TTL eviction, MessagingStyle construction
      for solo and group conversations, empty-cache fallback. Use
      `ProviderContainer` for any Riverpod-resolved deps per
      `docs/client/testing.md`. The
      `flutter_local_notifications` plugin exposes a built-in mock
      surface — use it, don't mock at the OS layer.
- [ ] Add a ViewModel/Repository test for the cancel-on-read fan-out:
      stub `ChatService.markMessagesRead`, assert
      `ChatNotificationManager.clearForConversation` was called
      exactly once with the same `conversationId`.

*New channel + initial wiring:*
- [ ] Register a `'chats'` `AndroidNotificationChannel`
      (`importance: high`, vibration on, description "Chat messages")
      alongside the existing `'community_events'` channel in
      `_initializeLocalNotifications`.
- [ ] In `_onForegroundMessageReceived` (`fcm_service.dart:276`), when
      `data['type'] == 'chat_message'`, delegate to
      `ChatNotificationManager.renderForChatMessage(data)` *instead of*
      `_showLocalNotification`. Keep `_showLocalNotification` for the
      `community_event` path unchanged.

*Cancel on read (via Repository, not direct Service call):*
- [ ] Extend `ChatRepository.markMessagesRead(conversationId)` to fan
      out to `ChatNotificationManager.clearForConversation(conversationId)`
      after the server-side mark-read completes. `markMessagesRead`
      is already the single chokepoint the ViewModel calls; folding
      the notification clear into it preserves the
      `client_architecture.md` rule that ViewModels call only
      Repositories — not Services — and keeps the side-effect
      bundled with its trigger ("when read state changes, clear
      notification").
- [ ] No new call in `ConversationMessagesNotifier.loadMessages` —
      the existing `markMessagesRead` invocation
      (`conversation_messages_notifier.dart:145`) now does both
      jobs.
- [ ] Inject `ChatNotificationManager` into `ChatRepository` via the
      existing repository constructor pattern (Riverpod
      `chatNotificationManagerProvider` → `chatRepositoryProvider`).
- [ ] Same cancel in `ConversationView.dispose()` is not needed —
      cancel happens once on open, not on close.

*i18n (per `docs/client/i18n.md`):*
- [ ] Add ARB keys to `app/lib/l10n/app_en.arb` and `app_es.arb`:
      - `notificationSelfName` ("You" / "Tú") for the self `Person`.
      - `notificationChatsChannelName`,
        `notificationChatsChannelDescription` for the channel.
      No new user-visible body strings — chat content is the sender's
      own message text.

*Architecture audit notes baked in here:*
- `ChatNotificationManager` is a Service (per
  `docs/client/architecture.md` — pure side-effect helper with no
  caching layer needed, since `SharedPreferences` is the
  persistence medium and the data is display-only, never sent to a
  repository).
- The cache is intentionally not a Repository: it doesn't round-trip
  through the server, and the Stash/`CacheManager` namespacing is
  wrong for isolate-shared data. Stash is memory-only per
  `docs/client/caching.md`; the notification cache must persist
  through process death.
- The ViewModel never calls the Service directly — `ChatRepository`
  is the only caller (per `docs/client/architecture.md` rule:
  "Repositories are the ONLY layer that calls Services"). The
  Repository delegates to `ChatNotificationManager` after
  `markMessagesRead` succeeds.
- Screen-reader accessibility for the notification itself is handled
  by the OS (TalkBack on Android, VoiceOver on iOS automatically
  announce notification title/body and MessagingStyle thread
  contents). No `a11y_*` ARB keys or `Tappable`/`Semantics`
  wrappers apply — those rules from `docs/client/accessibility.md`
  govern in-app widgets, not OS notifications. The only string
  content that hits a screen reader is the user-authored message
  text and the localized channel name; both are already covered by
  the i18n step above.

**Verification gate (green before Phase 3):**
- `app/test/services/chat_notification_manager_test.dart` — 15 tests
  covering missing/empty `conversation_id` skip, stable
  `conversationId.hashCode` notification id (replacement behavior),
  cache append + trim past 6 entries, TTL eviction at 60 minutes,
  per-conversation cache isolation, corrupt-cache recovery, localized
  string round-trip via SharedPreferences, `clearForConversation`
  cancel-by-id, no-op on empty id, and English fallback for unseeded
  channel strings. All passing.
- `app/test/data/repositories/chat_repository_test.dart` — 2 new
  tests in a `markMessagesRead → ChatNotificationManager fan-out`
  group: cancel fires once on RPC success, and does NOT fire when
  the RPC throws (preserves the lock-screen reminder). All 22 chat
  repo tests passing.
- `flutter test --reporter expanded` → **4266 tests green, 0 failed**.
- `flutter analyze app/lib app/test` → 0 issues.
- `npm run lint:dart:i18n`, `npm run lint:dart:a11y`,
  `npm run lint:dart:size` → all clean.
- Manual smoke (recommended before broader rollout): 3 messages on
  Android foreground → one notification showing the 3-line
  transcript; open conversation → notification dismissed.

**Notable design points landed:**
- `ChatNotificationManager` constructs `SharedPreferencesAsync` lazily
  on first read, not in the constructor. This is required so the
  Riverpod provider can build cleanly in unit tests that don't mock
  the platform interface (regression caught and fixed during Phase 2).
- `chatNotificationManagerProvider` is independent of
  `fcmServiceProvider`. Each manager owns its own
  `FlutterLocalNotificationsPlugin` instance but the platform channel
  is process-singleton, so a cancel from the repository's manager
  takes down the notification posted by FCMService's manager.
- `clearForConversation` swallows plugin errors (best-effort cancel):
  a `MissingPluginException` in unit tests would otherwise propagate
  through `markMessagesRead` and break unrelated callbacks. The mark-
  read intent has already succeeded on the server; failing here would
  punish the user for our cleanup step.
- Localized strings are seeded via `_ChatNotificationStringSeeder` in
  `main.dart`'s `MaterialApp.router.builder`, which runs once as a
  descendant of `Localizations`. Background isolate (Phase 3) reads
  the same SharedPreferences keys without needing l10n context.
- Android channel name is set at first-install channel creation;
  Android does not allow renaming a channel after creation, so users
  whose locale changes between installs keep the old name until app
  data is cleared. Acceptable per spec.

**Commits:**
- `<TBD>` — `chat: messaging-style foreground notifications + cancel-on-read (#1892 phase 2)`.

**Validates:** A series of messages in a single conversation appears as
one notification on Android that shows the recent transcript; opening
the conversation in-app immediately dismisses it. iOS notifications
already group from Phase 1; the foreground renderer continues that
grouping when the app is alive.

### Phase 3: Background isolate rendering (Android data-only path) ✅ SHIPPED 2026-05-13

**Goal:** Get the same MessagingStyle UX when the app is in the
background or killed. This is the second half of the user fix —
foreground-only solves nothing for the lock-screen case they reported.

**Tasks:**
- [ ] In `server/notifications/fcm/provider.go:buildMessage`, when the
      payload is a `Notification_ChatMessage` AND platform is Android:
      - Drop the top-level `messaging.Notification`.
      - Drop `android.Notification`.
      - Keep `android.Priority = "high"`.
      - Keep all data fields. The data payload becomes the *only*
        carrier; FCM delivers it to `onBackgroundMessage` and the
        client renders.
      - iOS is unchanged: keep `aps.alert` and `thread-id`. iOS
        rendering stays system-side until we add an NSE in Option C.
- [ ] Update `firebaseMessagingBackgroundHandler`
      (`fcm_service.dart:447`) to call
      `ChatNotificationManager.renderForChatMessage(message.data)` when
      `message.data['type'] == 'chat_message'`. Top-level function — no
      injection; the manager is constructed inline from
      `SharedPreferences.getInstance()` and
      `FlutterLocalNotificationsPlugin()`. This is acceptable per
      Flutter's background-isolate constraints.
- [ ] Confirm `flutter_local_notifications` initialization in the
      background isolate works without explicit `initialize()` on the
      Android side (it picks up the channel registered in the main
      isolate via the system). Add a defensive `initialize` call with
      the same settings if testing shows it's needed.
- [ ] Verify iOS: with `mutable-content: 1` and no NSE, iOS displays
      the system alert directly using `thread-id` grouping. Background
      handler is not invoked for visible alerts on iOS — that's
      expected and correct.
- [ ] Server unit test: chat-message Android payload has no
      `Notification` field; non-chat Android payload still has it.
- [ ] Integration test under `app/integration_test/` (or mock-driven
      widget test if integration is heavy): post a fake background
      data-only message, assert the notification was rendered with
      `MessagingStyle`. Use the existing
      `flutter_local_notifications` mocking surface.

*Caveats to document in `docs/push_notifications.md`:*
- Background isolate uses SharedPreferences (works) and no l10n strings
  (chat content is sender-authored, channel name comes from a
  fixed-key SharedPreferences read seeded at app start).
- Per FCM docs, data-only messages at `priority: high` are delivered
  promptly even in Doze; no behavioral regression expected on Android
  battery optimization.

**Verification gate (green before Phase 4):**
- `server/notifications/fcm/provider_test.go` — rewrote the existing
  Android-chat assertion to check the data-only contract:
  `msg.Notification == nil`, `msg.Android.Notification == nil`,
  `Android.Priority == "high"`, and `Data[type] == "chat_message"`.
  Added an explicit test for the empty-conversation-id fallback path
  (degraded payloads still produce a visible notification) and one
  asserting iOS keeps `Notification + aps.alert` for system-side
  display until the iOS NSE phase ships.
- `app/test/services/chat_notification_manager_test.dart` — added a
  `background-isolate handoff` group with 2 new tests: a fresh
  manager instance (simulating the background isolate) sees the
  foreground manager's cache via SharedPreferences and posts under
  the same `conversation_id.hashCode` so the system replaces the
  prior notification; a fresh-install background-only render works
  with an empty cache.
- `flutter test --reporter expanded` → **4268 tests green, 0 failed**.
- `go test ./server/notifications/...` → green across fcm,
  chat_subscriber, community_subscriber, notifications, noop.
- `npm run build` → exit 0 (gofumpt + golangci-lint + all internal
  lints).
- `npm run lint:dart:{i18n,a11y,size}` → clean.
- Manual smoke (recommended before broad rollout): app force-killed →
  send 5 messages from another account → exactly 1 lock-screen
  notification, showing the transcript; tap → app opens to the right
  conversation; notification disappears after mark-read.

**Notable design points:**
- The data-only switch on Android is scoped to chat messages with a
  non-empty `conversation_id`. Malformed chat payloads (empty id) and
  non-chat notifications keep the existing `notification` block so
  FCM still displays them system-side. No quiet drops.
- The background handler initializes its own
  `FlutterLocalNotificationsPlugin` per isolate. The plugin's factory
  returns the same instance within an isolate, so the channel-creation
  state from the main isolate isn't accessible — `.initialize()` and
  channel-creation rerun on first use. Channels are platform-singleton
  so creating the same channel twice is a no-op on Android.
- iOS chat payloads still carry the top-level `notification` block.
  iOS background handlers aren't reliably invoked for alert pushes;
  iOS displays the alert from `aps.alert` and groups by `thread-id`.
  A future phase replaces this with a Notification Service Extension.

**Commits:**
- `<TBD>` — `chat: data-only android payload + background-isolate render (#1892 phase 3)`.

**Validates:** Phone locked → 5 messages from Alice → 1 notification on
the lock screen that updates in place and shows the recent transcript.
Open the app → notification dismisses. This is the user-reported scenario,
end-to-end.

### Phase 4: Android Conversation Space (long-lived shortcuts + Person + LocusId) ✅ SHIPPED 2026-05-13

**Goal:** Promote chat notifications into the Android 11+ Conversation
Space tier: priority placement on the lock screen, conversation
widgets, bubble eligibility, and consistent Person/avatar rendering
inside MessagingStyle. The plumbing is a small Flutter→Android
platform channel plus `shortcutId` on the existing
`AndroidNotificationDetails` (which `flutter_local_notifications`
supports natively).

**Tasks:**

*New Android platform channel for shortcut registration:*
- [ ] Add a Kotlin class
      `app/android/app/src/main/kotlin/.../ConversationShortcutsPlugin.kt`
      registered on a `MethodChannel("ripls/conversation_shortcuts")`.
      Single method `pushDynamicShortcut(args)` that takes
      `{conversation_id, short_label, long_label, person_name,
      person_key, person_icon_bytes?, person_uri?}`, constructs a
      `ShortcutInfoCompat.Builder` with:
      - `setLongLived(true)`
      - `setIsConversation()` (Android 11+, no-op below)
      - `setPerson(Person.Builder().setName(...).setKey(...).build())`
      - `setLocusId(LocusIdCompat(conversation_id))`
      - `setShortLabel(...)`, `setLongLabel(...)`
      - `setIntent(Intent(Intent.ACTION_VIEW)
        .setData(Uri.parse("ripls://conversation/<id>"))
        .setPackage(packageName))`
      - `setCategories(setOf(SHARE_TARGET))` (optional but unlocks
        share-sheet integration for free)
      and calls `ShortcutManagerCompat.pushDynamicShortcut(...)`.
      Returns void / acks. Guard the whole method on
      `Build.VERSION.SDK_INT >= R` and degrade to no-op below
      Android 11.
- [ ] Register the plugin in `MainActivity.kt`'s
      `configureFlutterEngine`.

*Dart shortcut service:*
- [ ] Add `app/lib/services/conversation_shortcut_service.dart`. Thin
      wrapper around the platform channel. Method
      `ensureShortcut(ConversationShortcutSpec)` is idempotent —
      Android's `pushDynamicShortcut` already replaces on duplicate
      id, so we just call it every time we render a notification.
      No iOS-side implementation; method returns immediately on iOS.
- [ ] Add unit tests with `MethodChannelMock` asserting the right
      method+args land on the channel.

*Wire into the chat notification render path:*
- [ ] In `ChatNotificationManager.renderForChatMessage`, before
      posting the notification, call
      `ConversationShortcutService.ensureShortcut(...)` with the
      sender's `Person` data. Then set
      `AndroidNotificationDetails.shortcutId = conversation_id` and
      keep the `MessagingStyleInformation` as before. The plugin
      passes shortcutId through to `setShortcutId()` on the
      notification builder.
- [ ] If the conversation has a known avatar URL
      (`MediaRepository` cache hit), fetch the bytes
      synchronously off the cached file and include them as
      `person_icon_bytes`. On cache miss, skip the icon — never
      block the notification on a network fetch.

*Cleanup & limits:*
- [ ] Android caps dynamic shortcuts at ~15 by default
      (`getMaxShortcutCountPerActivity`). Add an LRU eviction
      strategy in `ConversationShortcutsPlugin`: when a push would
      exceed the cap, remove the oldest non-pinned shortcut via
      `ShortcutManagerCompat.removeLongLivedShortcuts(...)` first.
- [ ] When a conversation is left/deleted client-side (existing
      conversation-leave flow), call a new
      `removeShortcut(conversation_id)` method on the platform
      channel.

*Deep-link routing:*
- [ ] Register `ripls://conversation/<id>` (or the equivalent
      AndroidManifest intent filter — verify existing scheme in
      `AndroidManifest.xml`) so that the shortcut's intent opens the
      correct conversation. Reuse existing deep-link routing in
      `routeNotificationPayload`; the shortcut intent simulates a
      cold-start launch into the right conversation.

*Tests + smoke:*
- [ ] Unit test the LRU eviction logic on the Kotlin side
      (`./gradlew test` from `app/android`).
- [ ] Smoke test on Android 11, 14, and 16:
      - Conversation card appears in the Conversation Space at the
        top of the shade after first message.
      - Long-press on the notification opens the conversation
        priority dialog (Conversation Settings → "Priority" /
        "Favorite" / "Modify").
      - Bubble affordance is available when the user has bubbles
        enabled.

**Verification gate (green before Phase 5):**
- `app/test/services/conversation_shortcut_service_test.dart` — 7
  tests covering: pushDynamicShortcut payload shape, optional fields
  (person_uri, icon bytes), empty-id skip, swallowed platform errors,
  removeShortcut payload, removeShortcut empty-id skip, swallowed
  errors on remove.
- `app/test/services/chat_notification_manager_test.dart` — extended
  with 4 new tests in an `Android Conversation Space shortcut
  integration` group: shortcut pushed on each render, idempotent
  across multiple renders, falls back to conversation_id when sender
  is missing, shortcut failure does not block the notification post.
- `cd app/android && ./gradlew compileDebugKotlin` → BUILD SUCCESSFUL,
  0 warnings, 0 errors.
- `flutter test --reporter expanded` → **4279 tests green, 0 failed**.
- `flutter analyze app/lib app/test` → 0 issues.
- `npm run lint:dart:{i18n,a11y,size}` → clean.
- Manual smoke (deferred to a device test): Android 11/14/16 long-press
  notification opens the Conversation Settings dialog; bubble
  affordance appears; conversation card lands in Conversation Space.

**Notable design points:**
- `ShortcutManagerCompat.pushDynamicShortcut` handles LRU eviction
  internally when the system shortcut cap (~15) is reached, so no
  manual eviction logic is needed.
- Native plugin uses `androidx.core.content.pm.ShortcutInfoCompat` +
  `androidx.core.app.Person` + `androidx.core.content.LocusIdCompat`
  — the compat wrappers are no-ops on Android <11 so a single code
  path serves every supported API level.
- Shortcut intent uses `ripls://conversation/<id>`; added to the
  manifest's existing deep-link filter alongside `ripls://invite`
  and `ripls://reset-password`. The Dart router's existing
  `routeNotificationPayload` resolves conversation_id from the URL.
- Platform guard uses `defaultTargetPlatform != TargetPlatform.android`
  not `Platform.isAndroid` so tests can exercise the channel without
  needing a host-platform override (tests on macOS otherwise skipped
  every call).
- Best-effort: shortcut failures (PlatformException, missing plugin)
  are caught and logged at warn. A failed shortcut push never blocks
  the notification itself; the user still sees the message.

**Validates:** Chat notifications now occupy the Conversation Space
tier on modern Android: separated from the rest of the shade, with
the sender's avatar, available for bubbles, and a working long-press
conversation menu.

**Commits:**
- `<TBD>` — `chat: android conversation space shortcuts (#1892 phase 4)`.

### Phase 5: iOS Communication Notifications (Notification Service Extension + INSendMessageIntent) → tracked in #1910

**Goal:** Elevate iOS chat notifications into the system Communication
tier — same priority and affordances as Messages.app. Sender avatar
on the lock screen, INSendMessageIntent donation so Focus modes and
"Allowed Notifications from People" lists treat our chats correctly.

**Tasks:**

*New Xcode target:*
- [ ] In `app/ios/Runner.xcworkspace`, add a new target
      `RunnerNotificationService` of type
      "Notification Service Extension". Deployment target matches
      the app (currently iOS 14 — confirm and bump to iOS 15 if
      needed; Communication Notifications require iOS 15+).
- [ ] Add the target's `Info.plist`:
      - `NSExtension.NSExtensionPointIdentifier`:
        `com.apple.usernotifications.service`
      - `NSExtension.NSExtensionPrincipalClass`:
        `RunnerNotificationService.NotificationService`
      - `NSExtension.NSExtensionAttributes.IntentsSupported`:
        `[INSendMessageIntent]`
      - `NSUserActivityTypes`: `[INSendMessageIntent]`
- [ ] Add the same `NSUserActivityTypes` array to the main
      `Runner/Info.plist` (Apple requires it on both targets).

*App Group entitlement for shared storage:*
- [ ] Create an App Group ID `group.org.ripls.app` (or whatever
      matches the existing bundle prefix — confirm by reading
      `Runner.entitlements` and the bundle identifier).
- [ ] Add `com.apple.security.application-groups` to both
      `Runner.entitlements` and a new
      `RunnerNotificationService.entitlements`.
- [ ] On the Dart side, store the conversation-cache JSON used by
      the NSE in a file inside the App Group container
      (`FileManager.containerURL(forSecurityApplicationGroupIdentifier:)`).
      Replace SharedPreferences for the NSE-readable cache only —
      the main app keeps SharedPreferences for the Android side.
      Document the dual-backed cache in
      `chat_notification_manager.dart`.

*NotificationService.swift implementation:*
- [ ] Implement `didReceive(_ request:, withContentHandler:)`:
      1. Make a mutable copy of the content via
         `request.content.mutableCopy()`.
      2. Read `conversation_id`, `sender_user_id`, `sender_name`
         from `request.content.userInfo` (set by FCM data fields).
      3. Optionally read a cached avatar path from the App Group
         container.
      4. Build `INPerson` for the sender (with
         `personHandle: INPersonHandle(value: sender_user_id,
         type: .unknown)` and `image: INImage(imageData: ...)`
         if avatar bytes are available).
      5. Build `INSendMessageIntent` with `recipients: [self]`,
         `content: preview_text`, `sender: senderPerson`,
         `speakableGroupName: nil` (or community-derived for group
         chats), `conversationIdentifier: conversation_id`.
      6. Donate via
         `INInteraction(intent: intent, response: nil)
           .direction = .incoming`,
         `interaction.donate { _ in }`.
      7. Call `try? request.content.updating(from: intent)` and
         hand the result to `contentHandler`.
- [ ] Implement `serviceExtensionTimeWillExpire()` to fall back to
      the original (non-enriched) content so the system still
      displays something.

*Server payload — minor additions:*
- [ ] Confirm `mutable-content: 1` is set on chat-message iOS
      payloads (already set per `provider.go:220`). No change.
- [ ] Add `aps-priority: "10"` for chat alerts (already noted in
      Phase 1). Required for Communication Notifications to render
      with full priority.
- [ ] No new data fields needed — `sender_user_id`, `sender_name`,
      `preview_text`, `conversation_id`, `community_id` already
      cover the NSE's needs.

*CI / release plumbing:*
- [ ] Update Codemagic (or whatever ships builds — verify in
      `codemagic.yaml` / release scripts) to compile the
      `RunnerNotificationService` target alongside `Runner`.
- [ ] Add the App Group to the development and distribution
      provisioning profiles. Document the steps in
      `docs/push_notifications.md`.

*Tests + smoke:*
- [ ] Add an iOS unit test target for the extension covering
      INSendMessageIntent construction (use `XCTest` against
      `NotificationService`; no FCM dependency).
- [ ] Smoke test on iOS 16 and 18:
      - Sender avatar renders on the lock screen.
      - Focus mode "Allow Notifications from People" surfaces the
        sender by name.
      - Per-thread mute (long-press notification → "Mute
        conversation") works.
      - Communication Notifications priority badge appears under
        Settings → Notifications → Ripls.

**Validates:** iOS chat notifications match iMessage on lock-screen
presentation: thread-grouped, avatar-tagged, INSendMessageIntent-
donated, Focus-aware.

### Phase 6: Inline reply (Direct Reply on Android, UNTextInputNotificationAction on iOS) → tracked in #1911

**Goal:** Let the user reply to a chat message directly from the
notification without opening the app, matching iMessage / WhatsApp /
Signal. The plumbing layers cleanly on top of Phase 2's MessagingStyle
foundation and Phase 5's Communication Notification category.

**Tasks:**

*Server: add the iOS notification category and (optionally) a hint
field:*
- [ ] In `server/notifications/fcm/provider.go:buildMessage`, when the
      payload is a `Notification_ChatMessage` and platform is iOS,
      set `aps.category = "chat_message"`. iOS uses this string to
      pick the registered `UNNotificationCategory` (and therefore
      the reply action).
- [ ] No new proto fields needed. The existing
      `ChatMessagePayload` already carries everything the reply path
      needs (`conversation_id`, `sender_user_id`,
      `sender_name`).

*Android — RemoteInput action via `flutter_local_notifications`:*
- [ ] In `ChatNotificationManager.renderForChatMessage`, attach an
      `AndroidNotificationAction` to the notification:
      - `id: 'reply_${conversationId}'` (unique per conversation so
        the system reply intent carries it back).
      - `title: l10n.notificationReplyAction` ("Reply").
      - `inputs: [AndroidNotificationActionInput(
          label: l10n.notificationReplyHint, allowFreeFormInput: true)]`
        — triggers a system-level text input directly on the
        notification.
      - `showsUserInterface: false` (no Activity launch — the
        reply happens in the background).
      - `cancelNotification: false` (we'll update it ourselves
        with the user's message appended; cancelling would lose the
        recent transcript).
- [ ] Configure the plugin's `onDidReceiveBackgroundNotificationResponse`
      to route reply payloads. Annotate the handler
      `@pragma('vm:entry-point')` so it survives tree-shaking in
      release builds.
- [ ] Add `ChatNotificationReplyHandler` (top-level function,
      `app/lib/services/chat_notification_reply_handler.dart`) that:
      1. Parses the action id to extract `conversation_id` and the
         user's typed text from the action input map.
      2. Reads the stored auth token from `flutter_secure_storage`
         (already accessible from background isolates per the plugin
         docs — verify in a smoke test).
      3. Calls the existing chat send RPC via a minimal Connect
         client constructed inline (the full app stack is not
         initialized in the background isolate; this is the same
         pattern used by `firebaseMessagingBackgroundHandler`).
      4. On success: appends the just-sent message to the per-
         conversation cache as a "me"-`Person` message, then
         re-renders the notification via
         `ChatNotificationManager.renderForChatMessage` so the
         reply appears inside the MessagingStyle thread.
      5. On failure: posts a brief error notification ("Couldn't
         send reply — tap to retry") with `id =
         conversationId.hashCode ^ 0x1` so it sits alongside the
         original.

*iOS — `UNTextInputNotificationAction` + AppDelegate handler:*
- [ ] In `AppDelegate.swift`'s `application(_:didFinishLaunchingWithOptions:)`:
      - Register a `UNNotificationCategory`
        (`identifier: "chat_message"`,
        `intentIdentifiers: [INSendMessageIntent]`,
        `options: [.allowInCarPlay, .hiddenPreviewsShowTitle]`)
        with one action: `UNTextInputNotificationAction(
          identifier: "reply",
          title: l10n.notificationReplyAction, options: [],
          textInputButtonTitle: l10n.notificationReplyButton,
          textInputPlaceholder: l10n.notificationReplyHint)`.
      - Register the category via
        `UNUserNotificationCenter.current().setNotificationCategories([...])`.
- [ ] Implement
      `userNotificationCenter(_ center:, didReceive response:,
       withCompletionHandler completionHandler:)`:
      - When `response is UNTextInputNotificationResponse` and
        `response.notification.request.content.categoryIdentifier ==
        "chat_message"`:
        - Extract `userText` from
          `(response as! UNTextInputNotificationResponse).userText`.
        - Extract `conversation_id` from
          `response.notification.request.content.userInfo`.
        - Forward to Dart via the existing platform channel (a new
          `MethodChannel("ripls/chat_reply")` method
          `submitReply(conversationId, text)`) — Flutter is alive
          when the iOS extension hands off the action response,
          unlike Android's background isolate.
        - Call `completionHandler()` once the channel call returns.
- [ ] On the Dart side, register a handler on the channel that calls
      the same chat send RPC as Android, then locally appends the
      sent message and re-renders the notification.

*Auth in the background path (cross-platform concern):*
- [ ] Confirm `flutter_secure_storage` reads succeed in the Android
      background isolate. Add a fallback: if the read fails,
      surface the "couldn't send" notification and rely on the user
      tapping to launch the app and retry.
- [ ] Document the constraint in
      `docs/push_notifications.md`: reply requires a non-expired
      auth token. If the token has expired we cannot silently
      refresh from the background — the user gets a "tap to reply"
      fallback. (Token refresh requires the full app stack; not
      worth replicating in the background isolate.)

*i18n:*
- [ ] Add ARB keys to `app_en.arb` and `app_es.arb`:
      `notificationReplyAction` ("Reply" / "Responder"),
      `notificationReplyHint` ("Reply…" / "Responder…"),
      `notificationReplyButton` ("Send" / "Enviar"),
      `notificationReplyError` ("Couldn't send reply" /
      "No se pudo enviar la respuesta").

*Tests + smoke:*
- [ ] Unit test the parser that converts an
      `NotificationResponse` (Android plugin shape) into the
      `(conversation_id, text)` tuple used by the reply handler.
- [ ] Unit test the iOS `submitReply` channel handler in Dart with
      a mocked chat service.
- [ ] Integration test the end-to-end Android flow: simulate a
      reply action, assert the chat RPC was called with the right
      conversation_id and text, assert the local notification got
      re-rendered with the user's message appended.
- [ ] Manual smoke on Android 11/14/16 and iOS 16/18: type a reply
      from the notification, confirm the recipient sees it in <1s
      and the lock-screen notification updates to include the sent
      message inline.

**Validates:** A user reading a chat notification on the lock screen
can reply without ever opening the app, and the reply appears both
in the recipient's app and inline within the sender's own
notification thread. This is the final piece that brings parity with
iMessage / WhatsApp / Signal.

### Phase 7: Telemetry, channels, and rollout → folded into #1911

**Goal:** Add the observability needed to confirm the new path is
healthy in production and finish the small UX touches that don't
need a separate plan.

**Tasks:**
- [ ] Add `info` logs on cache hits, misses, and TTL evictions in
      `ChatNotificationManager` (one per render, with conversation_id
      and cache size).
- [ ] Add a `info` log on shortcut push (Android) and on NSE
      enrichment (iOS, via `os_log` so it shows in Console.app).
- [ ] Server log: extend the existing `provider.go` FCM log line
      via `logging.LoggerWithContext(ctx).With(...)` to include
      `thread_id_set` (bool), `data_only_android` (bool),
      `conversation_id` (string). Use the standard field names from
      `docs/server/observability.md`:
      `operation="fcm.send"`, `conversation_id`, `user_id`,
      `external_service="fcm"`. Do NOT introduce ad-hoc field names
      like `convId` or `convo_id`. ERROR level only for delivery
      failures; INFO for routine sends; never log message body
      (it's user content and not in scope for server logs).
- [ ] Set `MessagingStyleInformation.conversationTitle` from
      `ConversationContextRepository` when cached locally (drops back
      to sender name when we don't).
- [ ] Document the new path end-to-end in
      `docs/push_notifications.md` and link from
      `docs/client/chat.md`. Include the dual-backed cache
      (SharedPreferences on Android, App Group container on iOS) so
      future contributors know where chat-notif state lives.
- [ ] Add a feature flag (lightweight env-var on the server, see
      `server/config/`) so we can roll the data-only Android switch
      and the NSE-enriched iOS path independently in production.
- [ ] Run `npm run build`, full `go test ./...`, `flutter test
      --reporter expanded`, and the integration tests before the
      first PROD rollout.

**Validates:** Production telemetry shows >95% of chat pushes render
through the new path on both platforms; the feature flag is healthy
and reversible if a regression appears.

## Related Issues
- **#1027 (iOS notifications)** — Closed. Provides the APNs payload
  baseline we extend with `thread-id`.
- **#1107 (per-community notifications)** — Already gates per-community
  `notify_chats`; we render only what gets through the gate, no
  interaction.
- **#1133 (stale chat notification)** — Shares the conversation-open
  hook with our cancel-on-read step; coordinate so both fixes land
  cleanly on the same line.
- **#1551 (native notification categories)** — Adjacent: our new
  `chats` channel is a step in the same direction. Document the
  channel as the foundation for that issue.
- **#1124 (multi-photo chat)** — When media-only messages flow through,
  `preview_text` is "(photo)" or similar; MessagingStyle handles
  text-only entries fine. No work needed here, but confirm in Phase 4
  smoke tests.

## Sources

Industry research drew on these references:
- [Android: People and conversations](https://developer.android.com/develop/ui/views/notifications/conversations)
- [Android: NotificationCompat.MessagingStyle](https://developer.android.com/reference/androidx/core/app/NotificationCompat.MessagingStyle)
- [Android: Create a group of notifications](https://developer.android.com/develop/ui/views/notifications/group)
- [Firebase: FCM on Android 2025 best practices](https://firebase.blog/posts/2025/04/fcm-on-android/)
- [Firebase: Collapsible vs non-collapsible messages](https://firebase.google.com/docs/cloud-messaging/customize-messages/collapsible-message-types)
- [Apple: UNNotificationServiceExtension](https://developer.apple.com/documentation/usernotifications/unnotificationserviceextension)
- [Apple: thread-identifier in APNs payload (developer forum reference)](https://developer.apple.com/forums/thread/69520)
- [Connectycube: Push notifications in chat apps best practices](https://connectycube.com/2025/12/18/push-notifications-in-chat-apps-best-practices-for-android-ios/)
- [Signal: notification grouping rollout](https://aboutsignal.com/news/signal-to-group-notifications-in-chats-and-groups/)
- [flutter_local_notifications package](https://pub.dev/packages/flutter_local_notifications)
