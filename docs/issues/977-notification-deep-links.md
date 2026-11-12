# Issue #977: All notifications except for chat messages should deep link into the item detail screen

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/977
**Filed:** 2026-04-01 | **Last activity:** 2026-04-01
**Labels:** none | **Assignees:** maintainer
**Context docs:** none

## Problem Statement

All notification types should deep link to the item detail screen (gear,
experience, or request). Chat message notifications were fixed in PR #978 to
navigate to the Chat tab of the item detail screen. This issue covers the
remaining notification types: community events (transfer interest, recipient
selected, pickup proposed, cancelled, request offer, request fulfilled,
experience RSVP, experience created) and the experience lifecycle notifications
(in progress, completed, cancelled).

Currently, some community event notifications route correctly to the item
screen (those with `gear_id` or `experience_id`), but **request event
notifications** fall through to the old `conversation_id`-based routing because
`CommunityEventPayload` has no `request_id` field. Experience lifecycle
notifications (`notifyRSVPedUsers`) have **no payload at all** — tapping them
does nothing.

In addition, the documentation in `docs/push_notifications.md` is stale: it
still describes deep linking as "navigate to the relevant chat" and doesn't
list the newer event types (TRANSFER_CANCELLED, TRANSFER_PICKUP_PROPOSED,
EXPERIENCE_CREATED). The `LoanEventPayload` proto type is dead code — no
server code creates it.

## Context from Comments

No comments on this issue. The issue body references PR #978 (chat message
notification fix) as prior work.

## Current State: Notification Routing Audit

### Notification types and their current deep link behavior

**Community Event notifications** (from `community/notifications.go`):

| Event Type | Entity IDs in payload | Client routing | Correct? |
|---|---|---|---|
| TRANSFER_INTEREST_EXPRESSED | `gear_id`, `conversation_id` | `/gear/:id` | Yes |
| TRANSFER_RECIPIENT_SELECTED | `gear_id`, `conversation_id` | `/gear/:id` | Yes |
| TRANSFER_CANCELLED | `gear_id`, `conversation_id` | `/gear/:id` | Yes |
| TRANSFER_PICKUP_PROPOSED | `gear_id`, `conversation_id` | `/gear/:id` | Yes |
| REQUEST_OFFER_MADE | `conversation_id` only | `onConversationNavigate` → old UI | **No** |
| REQUEST_FULFILLED | `conversation_id` only | `onConversationNavigate` → old UI | **No** |
| EXPERIENCE_RSVP_YES | `experience_id`, `conversation_id` | `/experience/:id` | Yes |
| EXPERIENCE_RSVP_MAYBE | `experience_id`, `conversation_id` | `/experience/:id` | Yes |
| EXPERIENCE_CREATED | `experience_id`, `conversation_id` | `/experience/:id` | Yes |

**Experience lifecycle notifications** (from `experience/lifecycle.go`):

| Event | Entity IDs in payload | Client routing | Correct? |
|---|---|---|---|
| Experience in progress | none (bare notification) | nowhere | **No** |
| Experience completed | none (bare notification) | nowhere | **No** |
| Experience cancelled | none (bare notification) | nowhere | **No** |

**Chat message notifications** (from `chat/notifications.go`):

| Topic type | Entity IDs in payload | Client routing | Correct? |
|---|---|---|---|
| Experience | `experience_id`, `conversation_id` | `/experience/:id?tab=chat` | Yes (PR #978) |
| Gear | `gear_id`, `conversation_id` | `/gear/:id?tab=chat` | Yes (PR #978) |
| Request | `request_id`, `conversation_id` | `/request/:id?tab=chat` | Yes (PR #978) |
| Transfer | `gear_id`, `conversation_id` | `/gear/:id?tab=chat` | Yes (PR #978) |

### Problems identified

1. **Request community events missing `request_id`**: `CommunityEventPayload`
   has no `request_id` field. Request events (OFFER_MADE, FULFILLED) only
   include `conversation_id`, so they fall through to the old ConversationScreen
   routing.

2. **Experience lifecycle notifications missing payload**: `notifyRSVPedUsers()`
   creates a bare `Notification` with just title/body — no payload with
   `experience_id`. Tapping these does nothing because `routeNotificationPayload`
   has no data to route on.

3. **Community event notifications include `conversation_id`**: This causes the
   `chatTab` variable in `routeNotificationPayload` to be set, appending
   `?tab=chat` to item routes. But non-chat notifications should open on the
   default tab, not the Chat tab.

4. **`LoanEventPayload` is dead code**: Defined in proto but never created by
   any server code. It's only used in test helpers.

5. **`docs/push_notifications.md` is stale**: The deep linking section still
   says notifications navigate to "relevant chat". Missing newer event types.
   Doesn't reflect the item-detail-screen routing pattern.

6. **`onConversationNavigate` / `navigateToConversationFromNotification` is
   dead code**: After PR #978, chat message notifications route via entity IDs
   to item detail screens. The only remaining path that uses
   `onConversationNavigate` is the fallback for community events with only a
   `conversation_id` — which is the bug we're fixing. Once request events
   include `request_id`, no notification path will use it.

## Potential Solutions

### Option A: Incremental — add missing entity IDs to payloads

- **Approach:** Add `request_id` to `CommunityEventPayload` proto. Populate it
  in `buildNotification()`. Add `experience_id` payload to
  `notifyRSVPedUsers()`. Fix the `chatTab` logic to only apply for
  `chat_message` type. Remove dead code.
- **Pros:** Small, targeted changes. Each fix is independent and testable.
  Follows the existing pattern of including entity IDs in payloads.
- **Cons:** None significant — this is the natural completion of the pattern
  established in PR #978.
- **Effort:** Small.

### Option B: Refactor notification payloads to use a unified item payload

- **Approach:** Create a single `ItemNotificationPayload` with
  `oneof item_id { gear_id, experience_id, request_id }` and use it across all
  notification types instead of separate CommunityEvent/LoanEvent/ChatMessage
  payloads.
- **Pros:** Cleaner long-term. Single routing path on client.
- **Cons:** Larger change. Breaks wire compatibility with existing notifications
  in-flight. Requires updating all notification creation code and tests.
- **Effort:** Medium-large.

## Recommendation

**Option A.** The existing pattern works well — each notification type includes
the entity IDs needed for the client to route to the correct item detail
screen. We just need to fill the gaps: add `request_id` to community event
payloads, add an experience payload to lifecycle notifications, and fix the
`chatTab` logic so only chat message notifications open on the Chat tab.

## Open Questions

- **Should `LoanEventPayload` be removed?** — Recommendation: Yes. It's dead
  code — no server code creates `Notification_LoanRequest`. Remove the proto
  message, the `buildDataPayload` case, and the test helper. Keep the proto
  field number reserved to avoid wire conflicts.

- **Should `onConversationNavigate` be removed from `fcm_service.dart`?** —
  Recommendation: Yes. After this fix, no notification path will use the
  conversation-only routing. The standalone `ConversationScreen` is no longer a
  notification target.

- **Should `conversation_id` still be included in community event payloads?** —
  Recommendation: Remove it from `buildNotification()` and
  `getConversationID()`. It's no longer needed for routing since all events now
  include the primary entity ID. This also fixes the `chatTab` bug (problem #3)
  without needing a type-based check.

## Implementation Plan

### Phase 1: Add request_id to community event notifications
**Goal:** Request events (OFFER_MADE, FULFILLED) deep link to the request
detail screen.
**Tasks:**
- [ ] Add `request_id` field (field 9) to `CommunityEventPayload` in
  `proto/ripls/models/notification.proto`
- [ ] In `buildNotification()` in `community/notifications.go`, populate
  `request_id` from `event.GetRequestId()` when present
- [ ] In `buildDataPayload()` in `fcm/provider.go`, include `request_id` in
  the data map for community events when present
- [ ] Run `npm run generate` and verify with `npm run build`
- [ ] Add test case in `provider_test.go` for community event with `request_id`
**Validates:** Tapping a "Help Offered" or "Request Fulfilled" notification
opens the request detail screen.

### Phase 2: Fix experience lifecycle notifications
**Goal:** Experience lifecycle notifications (in progress, completed, cancelled)
deep link to the experience detail screen.
**Tasks:**
- [ ] In `notifyRSVPedUsers()` in `experience/lifecycle.go`, add a
  `CommunityEventPayload` (or minimal payload) with `experience_id` so the
  client can route to the experience screen
- [ ] Add test for experience lifecycle notification payload
**Validates:** Tapping "Experience is now in progress" opens the experience
detail screen.

### Phase 3: Stop appending ?tab=chat for non-chat notifications
**Goal:** Non-chat notifications open the item on the default tab (not Chat).
**Tasks:**
- [ ] Remove `conversation_id` field from `CommunityEventPayload` in
  `notification.proto` and reserve field number 4
- [ ] Remove `getConversationID()` call and `ConversationId` assignment from
  `buildNotification()` in `community/notifications.go`
- [ ] Remove the `getConversationID()` helper function (now unused)
- [ ] Remove `conversation_id` from the community event `buildDataPayload()`
  case in `provider.go`
- [ ] Run `npm run generate` and verify with `npm run build`
- [ ] Verify the `chatTab` variable in `routeNotificationPayload` correctly
  produces empty string for community events (since `conversation_id` will no
  longer be present)
- [ ] Update client tests to verify community events no longer include
  `?tab=chat`
- [ ] Update server tests
**Validates:** Tapping a transfer or experience notification opens the item
detail screen on the default tab.

### Phase 4: Remove dead code and update docs
**Goal:** Clean up unused code and update documentation.
**Tasks:**
- [ ] Remove `LoanEventPayload` from `notification.proto` (reserve field 11)
- [ ] Remove `Notification_LoanRequest` case from `buildDataPayload()`
- [ ] Remove `LoanEventPayload` from `notification_type()` in
  `notifications/service.go`
- [ ] Remove `onConversationNavigate` callback from `fcm_service.dart`
- [ ] Remove `navigateToConversationFromNotification()` from
  `conversation_helper.dart` and its wiring in `main.dart`
- [ ] Remove dead test code referencing `LoanEventPayload`
- [ ] Update `docs/push_notifications.md`:
  - Update deep linking section to describe item-detail-screen routing
  - Add TRANSFER_CANCELLED, TRANSFER_PICKUP_PROPOSED, EXPERIENCE_CREATED to
    the event type table
  - Update the proto example to show `ChatMessagePayload` (not just
    `LoanEventPayload`)
  - Remove references to "navigate to relevant chat" — replace with
    "navigate to item detail screen"
- [ ] Run `npm run generate` and `npm run build`
**Validates:** No dead code. Documentation matches the actual behavior. Grep
for `LoanEventPayload`, `onConversationNavigate`, `getConversationID` returns
zero results.

## Audit Notes

**Reviewed against:** `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/server/architecture.md`

### Issues found and fixed

1. **Existing chatTab bug on transfer/experience community events
   (architecture.md compliance):** Transfer community events (e.g.,
   TRANSFER_INTEREST_EXPRESSED) currently include both `gear_id` and
   `conversation_id` in the FCM payload. The client's `routeNotificationPayload`
   appends `?tab=chat` whenever `conversation_id` is present, so these
   notifications incorrectly open the gear screen on the Chat tab instead of the
   default tab. The same applies to experience RSVP/created events. **Phase 3
   fixes this by removing `conversation_id` from community event payloads.** Note:
   between Phase 1 and Phase 3 completion, this existing bug persists for
   transfer/experience events — consider merging Phases 1-3 into one commit to
   avoid the intermediate state.

2. **Phase 2 underspecified:** `notifyRSVPedUsers()` needs a concrete payload
   type. **Updated: use `CommunityEventPayload` with `experience_id` set,
   matching the pattern in `buildNotification()`.** The function only has
   `experienceID` and `experienceName` available, so the payload will be minimal
   (no `community_id`, no `event_type` — just enough for client routing).

3. **Remove unused proto fields:** Phase 3 stops populating `conversation_id`
   on `CommunityEventPayload`. Since it will no longer be used, **remove the
   field and reserve field number 4** to prevent future reuse conflicts. Same
   pattern as `LoanEventPayload` removal (reserve field 11). **Updated Phase 3
   and Phase 4 tasks accordingly.**

### No issues

- **Navigation pattern:** Notification deep links use GoRouter routes
  (`/gear/:id`, `/experience/:id`, `/request/:id`), which is correct for
  external entry points. The go-home-then-push pattern in `main.dart` ensures
  proper back-button behavior (architecture.md §Navigation & Gestures).
- **Server architecture:** Adding `request_id` to `CommunityEventPayload` and
  populating in `buildNotification()` follows existing patterns. No
  cross-service dependencies introduced.
- **Proto boundary:** All proto changes are in `models` package (server-only).
  Client reads raw FCM data fields, not generated models code.
- **Caching:** No caching changes needed. Notification routing is stateless.
- **i18n:** No new user-visible strings. Notification titles/bodies are
  server-generated.
- **Testing:** Each phase includes test tasks.

## Related Issues
- #962 — Chat notifications lead to old chat UI (fixed in PR #978, parent work)
