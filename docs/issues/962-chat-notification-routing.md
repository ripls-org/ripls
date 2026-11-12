# Issue #962: Chat notifications lead to old chat UI

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/962
**Filed:** 2026-03-31 | **Last activity:** 2026-04-01
**Labels:** P1 | **Assignees:** maintainer
**Context docs:** none

## Problem Statement

When a user taps an FCM push notification for a chat message, the app navigates
to the conversation via the GoRouter `/conversations/:id` route. This route
uses a `FutureBuilder` that fetches an `InboxItem` (listing all conversations,
finding the match, fetching transfer details) and renders `ConversationScreen`
inside GoRouter's page stack.

The new portfolio inbox (`PortfolioInboxScreen`) navigates to conversations
differently: it pushes `ConversationScreen` directly via
`NavigationHelpers.pushWithSlide()` with `useRootNavigator: true`, passing the
`onMessagesMarkedAsRead` callback. This is the intended chat entry point since
mid-March 2026.

The two approaches differ in:
1. **Navigation stack** -- the old route pushes onto GoRouter's stack; the new
   pattern pushes onto the root `Navigator` with a slide transition.
2. **Data fetching** -- the old route calls `fetchConversationData()` which
   lists all conversations + fetches transfer details to build an `InboxItem`.
   The new pattern passes `conversationId` directly without pre-fetching an
   `InboxItem`; the `ConversationScreen` loads its own data.
3. **Unread count reset** -- the old route creates the mark-as-read callback
   from the fetched `InboxItem`. The new pattern creates the callback inline
   using the `communityId` from the `DailyItem`.

## Context from Comments

One comment from the issue author (maintainer) clarifies: "when there are
FCM notifications for chat messages, they link into an older chat UI, rather
than the new one added in the last week or so. They should link to the new one."

This confirms the fix is about changing the notification navigation path, not
about removing the old UI entirely (the GoRouter route may still be needed for
other deep links or web).

## Potential Solutions

### Option A: Update FCM handler to push ConversationScreen directly (like portfolio inbox)

- **Approach:** Change `handleConversationNotification()` in
  `conversation_helper.dart` to push `ConversationScreen` via
  `NavigationHelpers.pushWithSlide()` with `useRootNavigator: true`, matching
  the portfolio inbox pattern. The FCM handler in `main.dart` would need a
  `BuildContext` (or root `NavigatorState`) to call `Navigator.of()`.
- **Pros:** Exactly matches the new chat entry pattern. No GoRouter involvement
  for conversation navigation. Consistent UX.
- **Cons:** The FCM callback in `main.dart` currently passes a route string; it
  would need refactoring to pass context or a navigator key. Also needs a way to
  get `communityId` for the unread count callback without pre-fetching an
  `InboxItem`.
- **Effort:** Medium.

### Option B: Update the GoRouter `/conversations/:id` route to render the new UI

- **Approach:** Modify the existing `/conversations/:id` GoRoute builder so it
  renders `ConversationScreen` in the new style (with slide transition, proper
  callbacks), or simply simplify it to just pass `conversationId` without the
  `FutureBuilder`/`InboxItem` pre-fetch since `ConversationScreen` can accept
  `inboxItem` as optional.
- **Pros:** Minimal changes -- the FCM handler already navigates to
  `/conversations/:id`, so fixing the route fixes notifications. Also fixes any
  other deep links using that route.
- **Cons:** Still uses GoRouter navigation stack rather than root navigator
  push. May not get the slide-from-right transition. Navigation back behavior
  might differ from portfolio inbox path.
- **Effort:** Small.

### Option C: Refactor to use a global navigator key for all notification navigation

- **Approach:** Add a `rootNavigatorKey` (already likely available from
  GoRouter) and use it in the FCM handler to push screens directly onto the root
  navigator, bypassing GoRouter entirely for notification-triggered navigation.
- **Pros:** Clean separation between deep-link routing (GoRouter) and
  notification-push navigation (root navigator). Most flexible for future
  notification types.
- **Cons:** Larger refactor. Introduces a second navigation mechanism.
- **Effort:** Medium-large.

## Recommendation

**Option A** is the right approach. The portfolio inbox's
`_navigateToConversation()` is the canonical way to open a conversation now, and
FCM notifications should match it exactly. The key insight is that
`ConversationScreen` already accepts `inboxItem` as optional (`this.inboxItem`),
so the notification handler doesn't need to pre-fetch an `InboxItem` -- it can
just pass `conversationId` and let the screen load its own data.

Note: `architecture.md` specifies that conversation is a primary content screen
that should use `NavigationHelpers.pushScreen()` + `SwipeToCloseMixin`, not
`pushWithSlide()`. The portfolio inbox currently uses `pushWithSlide()` which is
a deviation -- both the notification handler and portfolio inbox should be
aligned to use `pushScreen()`.

The main challenge is getting a `NavigatorState` in the FCM callback. GoRouter's
`navigatorKey` provides access to the root navigator, which is exactly what
`useRootNavigator: true` resolves to.

For the unread count callback, the notification handler can create a simpler
callback that fetches `communityId` from the conversation after it loads, or
pass it through from the notification payload (the server already looks up
`conversation_id` and could include `community_id`).

## Open Questions

- **Should the GoRouter `/conversations/:id` route be removed?** --
  Yes, remove it. The new navigation pattern (direct push via
  `NavigationHelpers.pushWithSlide()`) is the only entry point going forward.

- **Should `community_id` be added to the chat message notification payload?** --
  Recommendation: Yes. Adding `community_id` to `ChatMessagePayload` on the
  server is trivial (the conversation already has it), and it enables the client
  to create the `onMessagesMarkedAsRead` callback without an extra fetch.

- **What about the `onReturnFromScreen` callback that portfolio inbox uses?** --
  Recommendation: The FCM handler doesn't need this -- the portfolio inbox uses
  it to refresh its list when returning from a conversation, but notification
  navigation goes home first anyway.

## Implementation Plan

### Phase 1: Add community_id to chat notification payload
**Goal:** Server sends `community_id` alongside `conversation_id` in chat
message notifications so the client can reset unread counts without extra
fetches.
**Tasks:**
- [ ] Add `community_id` field to `ChatMessagePayload` in
  `proto/ripls/models/notification.proto`
- [ ] Set `community_id` from `conversation.CommunityId` in
  `server/services/chat/notifications.go:notifyParticipants()`
- [ ] Add `community_id` to the FCM data payload in
  `server/notifications/fcm/provider.go:buildDataPayload()` for chat messages
- [ ] Run `npm run generate` and verify build with `npm run build`
**Validates:** Server correctly includes community_id in all chat notifications.

### Phase 2: Update client notification navigation
**Goal:** Chat notification taps navigate to `ConversationScreen` using the same
pattern as portfolio inbox.
**Tasks:**
- [ ] In `fcm_service.dart`, extract `community_id` from chat message data
  payloads and pass it through the `onNavigate` callback (or add a dedicated
  chat navigation callback)
- [ ] In `main.dart` `_initializeFCM()`, update the conversation notification
  handler to push `ConversationScreen` directly via
  `NavigationHelpers.pushScreen()` (not `pushWithSlide()` — conversation is a
  primary content screen with `SwipeToCloseMixin` per `architecture.md`) using
  GoRouter's root navigator key, passing `conversationId` and
  `onMessagesMarkedAsRead` callback with the `community_id` from the payload
- [ ] Update `handleConversationNotification()` in `conversation_helper.dart`
  to accept `communityId` parameter and use the new navigation pattern, or
  replace it entirely
- [ ] Update `_onLocalNotificationTapped()` in `fcm_service.dart` to also
  extract and pass `community_id`
- [ ] Also update portfolio inbox `_navigateToConversation()` to use
  `pushScreen()` instead of `pushWithSlide()` for consistency with
  `architecture.md` navigation standards
- [ ] Manual test: tap chat notification from background -> lands on new
  conversation UI with swipe-to-close
- [ ] Manual test: tap chat notification from terminated -> same behavior
- [ ] Manual test: tap foreground local notification -> same behavior
- [ ] Unit test: verify `_handleNotificationNavigation()` extracts
  `community_id` from chat message payloads and invokes the correct callback
- [ ] Unit test: verify local notification tap handler parses `community_id`
  from JSON payload
**Validates:** All three notification entry points (background, terminated,
foreground) navigate to the conversation using the new UI pattern. Unit tests
cover payload parsing.

### Phase 3: Remove old route and dead code
**Goal:** Delete the GoRouter `/conversations/:id` route and related dead code.
**Tasks:**
- [ ] Remove the `/conversations/:id` GoRoute from `app_router.dart`
  (lines 468-529)
- [ ] Remove `handleConversationNotification()` from `conversation_helper.dart`
  if fully replaced by the new navigation in Phase 2
- [ ] Remove the unused `navigateToConversation()` function in
  `conversation_helper.dart` (lines 98-144)
- [ ] Remove unused `inboxNavigatorKeyProvider` from `home_view_model.dart` if
  confirmed unused
- [ ] Remove `fetchConversationData()` from `conversation_helper.dart` if no
  longer referenced anywhere
- [ ] Clean up any now-unused imports
**Validates:** No dead code left behind. Grep for `/conversations/` confirms no
remaining references to the old route.

## Audit Notes

**Reviewed against:** `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/server/architecture.md`

### Issues found and fixed

1. **Navigation helper mismatch (architecture.md compliance):**
   `architecture.md` lists "conversation" as a primary content screen that must
   use `NavigationHelpers.pushScreen()` + `SwipeToCloseMixin`, not
   `pushWithSlide()`. The unused `navigateToConversation()` in
   `conversation_helper.dart` already uses the correct identity-transition +
   zero-duration pattern for SwipeToCloseMixin screens. The portfolio inbox's
   use of `pushWithSlide()` for conversations is an existing deviation from the
   documented standard. **The plan's Phase 2 tasks were updated to use
   `pushScreen()` instead of `pushWithSlide()`.**

2. **Missing test coverage (architecture.md: "No new functionality without
   tests"):** The plan only listed manual testing. **Added unit test tasks to
   Phase 2.**

### No issues

- **Proto package boundary:** The plan correctly modifies `models` proto
  (server-only) for the notification payload. The client reads raw FCM data
  fields, not generated models code, so the client/server boundary is respected.
- **Caching:** Unread count reset uses the proper
  `unreadCountProvider.notifier.resetUnreadCount()` pattern, consistent with the
  optimistic delta pattern in `caching.md`.
- **i18n:** No new user-visible strings introduced.
- **Server architecture:** Adding a field to the notification payload and
  populating it in `notifyParticipants()` follows the existing patterns.

## Related Issues
- No directly related issues referenced.
