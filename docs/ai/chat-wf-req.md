# Chat Workflow: Request Conversations

## Overview

Integrate smart workflow components directly into request chat conversations,
replacing the current pattern of navigating away to a separate modal for all
workflow actions. The chat becomes the primary coordination surface: offer cards,
helper coordination, and completion impact cards appear as inline widget cards
within the message timeline. A persistent bottom bar provides phase
awareness and quick access to requester/helper actions without scrolling.

This is the second workflow type to receive this treatment, following the
experience workflow. The patterns established there — `WorkflowWidgetFactory`,
`WidgetShell`, `ManagePanel`, phase progress bar, floating top bar — are reused
and extended for requests.

## Reference Materials

- **Experience chat workflow spec:**
  [docs/ai/chat-wf-exp.md](chat-wf-exp.md) — Pattern reference, all shared
  infrastructure
- **Request workflow spec:**
  [docs/workflows/request.md](../workflows/request.md) — Request states, user
  operations, business rules
- **Existing docs:**
  - [docs/client/chat.md](../client/chat.md) — Chat system architecture
  - [docs/workflows/experience.md](../workflows/experience.md) — Experience
    workflow (for comparison)
- **Existing implementation:**
  - [conversation_screen.dart](../../app/lib/presentation/screens/inbox/conversation_screen.dart) — Current conversation screen
  - [conversation_view_model.dart](../../app/lib/presentation/viewmodels/conversation_view_model.dart) — ViewModel
  - [message_input_bar.dart](../../app/lib/presentation/widgets/chat/message_input_bar.dart) — Current input bar
  - [workflow_widget_factory.dart](../../app/lib/presentation/widgets/chat/workflow_widget_factory.dart) — Widget card factory (experience-only today)
  - [widget_shell.dart](../../app/lib/presentation/widgets/chat/widget_shell.dart) — Glassmorphic card container
  - [request_owner_modal.dart](../../app/lib/presentation/screens/request/request_owner_modal.dart) — Owner modal (862 lines)
  - [request_helper_modal.dart](../../app/lib/presentation/screens/request/request_helper_modal.dart) — Helper modal
  - [request_content_view.dart](../../app/lib/presentation/screens/request/request_content_view.dart) — Request detail screen

---

## Problems

### 1. Workflow actions require leaving the conversation

Currently, all request management (viewing offers, marking fulfilled, cancelling)
happens in a separate modal. Users must tap "Manage" in the
input dock, wait for the modal to load, perform the action, dismiss the modal,
and return to the chat. This context switch breaks the conversational flow —
users are discussing logistics *in chat* but must *leave chat* to act on those
logistics.

**Impact**: Extra taps, lost scroll position, cognitive overhead. The same
problem that existed for experiences before the chat workflow migration.

### 2. No visibility into request state from the chat

The chat timeline shows only text messages and bare system messages like "Thomas
offered to help." There is no visual indicator of the current request phase
(Active → Offers Received → Fulfilled), no inline representation of who has
offered, and no way to see the request status without opening the modal.

**Impact**: Participants must navigate to the modal repeatedly just to check who
has offered and what the current state is.

### 3. Offer activity is invisible in message context

When helpers offer to fulfill a request, the only evidence in chat is a system
message "Thomas offered to help." There is no card showing the current offer
count, who has offered, or providing the requester with a quick way to review
and coordinate with helpers.

**Impact**: The requester must mentally track offers or leave the chat to check
the modal. Community members browsing the conversation can't see at a glance
whether the request has enough helpers.

### 4. Helper status is invisible on message bubbles

Chat messages from other participants show no indication of whether they've
offered to help. When someone says "I have one you can borrow," you can't see
at a glance whether they've formally offered or are just chatting.

**Impact**: The requester must cross-reference messages with the offer list in
the modal. The social context that makes coordination effective is missing.

### 5. Fulfillment and completion are disjointed

After a request is fulfilled, the requester must open the modal to see impact
metrics. This final step has no presence in the chat where the wrap-up
conversation ("Thanks so much!") is happening.

**Impact**: The lifecycle feels incomplete — the request ends in a modal rather
than in the shared conversation space.

---

## Proposed Solution

### Core Concept: Widget Cards in the Chat Timeline

Reuse the **widget card** infrastructure from the experience workflow — interactive,
state-aware UI components embedded in the chat message timeline. These cards render
at specific points in the conversation and update their appearance based on the
current request state.

Widget cards are **not chat messages**. They are client-rendered components
anchored to system events in the message history. The server continues to send
standard system messages (e.g., "Thomas offered to help"); the client enriches
certain system messages with interactive widget cards based on message type and
conversation context.

### Architecture

```
Server                                Client
──────                                ──────
SystemMessage
  type: REQUEST_CREATED (new)    →    SystemMessageWidget
                                        + WorkflowWidgetCard (request info card)
SystemMessage
  type: OFFERED                  →    SystemMessageWidget (enhanced)
                                        shows "Helping" badge on user's messages
SystemMessage
  type: FULFILLED                →    SystemMessageWidget
                                        + WorkflowWidgetCard (completion card)
SystemMessage
  type: CANCELLED                →    SystemMessageWidget (highlighted, coral)
```

**Server changes required**: Add one new system message type:
- `REQUEST_CREATED`: Sent when a request is submitted. This anchors the request
  info card in the timeline at the correct position. Currently request creation
  does NOT emit a system message — it only creates the conversation. This new
  type gives the client an anchor point for the request info widget card.

**Reused existing system message types**:
- `CHAT_SYSTEM_ACTION_OFFERED` (8): Already emitted when a user offers to help
- `CHAT_SYSTEM_ACTION_FULFILLED` (9): Already emitted when request is fulfilled
- `CHAT_SYSTEM_ACTION_CANCELLED` (5): Already emitted when request is cancelled

### Key Design Decisions

1. **Client-rendered, server-anchored**: Widget cards are rendered by the client
   based on system message types. One new message type (`REQUEST_CREATED`) is
   added to the server. Cards read live state from the ViewModel (not from the
   message payload), so they always show current data.

2. **Widget cards become read-only history**: Once a workflow phase advances
   past a card's purpose (e.g., request is fulfilled), the card renders in a
   "muted" state — still visible for context but no longer interactive. The
   active controls move to the bottom bar.

3. **Bottom bar is the persistent control surface**: The bottom bar (below the
   message input) shows the current phase, a phase-specific status pill, and
   provides quick access to the manage menu. This mirrors the experience chat
   workflow's bottom bar.

4. **Floating top bar already exists**: The floating top bar (from the
   experience workflow, Phase 1) already renders for all conversation types,
   showing the request title and participant avatars for request conversations.
   No changes needed.

5. **Simpler than experiences**: Requests have fewer phases and no time
   coordination. The request workflow is simpler — no RSVP buttons, no time
   voting, no attendance confirmation. This means fewer widget cards and a
   simpler manage menu.

---

## Detailed Component Design

### A. Request Info Card (Widget Card)

Rendered in the timeline after the "request created" system message (new
`REQUEST_CREATED` type).

- **Container**: Reuse `WidgetShell` — `AppColors.transferBackgroundCream`
  background, `backdropFilter: blur(12)`, rounded 18px, subtle border, shadow.
  When muted: reduced opacity (0.7), no shadow.
- **Content**:
  - **Title**: Request title (serif display font, 17pt, bold)
  - **Description**: Request description (body text, 2-3 lines max with
    ellipsis overflow)
  - **Location row**: Icon (coral map pin in coral-bg circle) + location name.
    Matches existing `LocationRow` pattern. Non-interactive.
  - **Offer button**: "I'll Help" button for non-owners who haven't offered.
    "Helping" (muted, with check icon) for users who have already offered.
    Hidden for the requester (owner). Tapping invokes a ViewModel method
    (e.g., `ConversationViewModel.offerToFulfill()`) which calls the
    repository internally. Per
    [architecture.md](../client/architecture.md) — widgets never access
    repositories directly; all actions go through the ViewModel.
- **Behavior**:
  - Tapping "I'll Help" calls the offer RPC and immediately updates the card
    to show "Helping" (optimistic update).
  - Card becomes muted when the request reaches a terminal state (FULFILLED or
    CANCELLED).
  - The offer button disappears for terminal states.
- **Source data**: Request details from `ConversationState.cachedRequest`.
  Offer state from the request's participant/offer list.

### B. Chat Bubbles with Helper Badges

Enhance received message bubbles to show the sender's offer status:

- Above each received message bubble (next to the sender's first name): a
  small badge pill showing "Helping" (sage green bg/text).
- Badge only shown for request conversations where the sender has offered to
  help (is in the offer list).
- No badge for the requester or users who haven't offered.
- Mirrors the RSVP badge pattern from experiences.

### C. Completion Card (Widget Card)

Rendered after the "request fulfilled" system message (`FULFILLED` type).

- **Container**: Same `WidgetShell`, read-only (not mutable).
- **Content**:
  - Request title (serif display font, 17pt, bold)
  - Horizontal divider
  - Helper avatar stack + "N helped" + days-open string (e.g., "Fulfilled in
    3 days")
  - Horizontal divider
  - Impact metrics: "Time saved" (sage value), "Emissions prevented" (coral
    value). Adapted from `ImpactMetricsCard` in the request owner modal.
  - **No summary text** — the chat conversation itself is the record of how
    the request was fulfilled (see Decision 2).
- **Behavior**: Read-only. Tappable → navigates to the request detail view
  via `NavigationHelpers.pushToItemScreen()` (per
  [architecture.md](../client/architecture.md#navigation--gestures) — all
  in-app navigation must use `NavigationHelpers`).
- **Source data**: Helper list and impact metrics from
  `MarkRequestFulfilled` response stored as a Freezed field in
  `ConversationState`. This is immutable ViewModel state, **not** a manual
  cache map — per [caching.md](../client/caching.md) repositories own
  caching, not ViewModels.

### D. Bottom Bar: Phase Progress + Status + Input

Enhance the existing `MessageInputBar` to add phase awareness for request
conversations. Mirrors the experience workflow's bottom bar structure:

**Section 1: Progress bar** (top edge of the floating card)
- Three thin bar segments (3px height, 3px gap between segments) representing
  the three request phases: Active, Offered, Fulfilled.
- Color logic matches experience workflow: completed = sage green,
  current = coral, future = light gray.
- Compact — just the bars, no labels.

**Section 2: Status row** (below progress bar)
- **Phase pill** (left): Tappable button showing a colored dot + phase name +
  chevron. Tapping opens/closes the manage menu panel.
  - Dot color: coral for Active/Offered, sage for Fulfilled.
  - Text: "Active", "Offers Received", "Fulfilled", or "Cancelled".
- **Participant avatars** (right): Right-justified overlapping `UserAvatar`
  circles (24px) for helpers who have offered. Mirrors the experience bottom
  bar pattern.

**Section 3: Input row** (bottom, unchanged from chat v2)
- Plus button + text field + send button.
- The existing "Manage" action button is replaced by the phase pill's manage
  menu.

**Manage menu panel** (slides up between sections 2 and 3):
- Triggered by tapping the phase pill.
- Semi-transparent frosted-glass background, matching the experience manage
  menu pattern.
- Tap-outside-to-close via dismiss barrier.
- Shows role-appropriate actions as a list of tappable rows.
- **Requester menu** (Active / Offers Received phases):
  - **Edit request**: Opens request edit screen via
    `NavigationHelpers.pushScreen()` (per
    [architecture.md](../client/architecture.md#navigation--gestures))
  - **Share with communities**: Opens community sharing sheet
  - **Mark fulfilled**: Highlighted sage row → transitions to FULFILLED
  - **Cancel request**: Coral text → transitions to CANCELLED
- **Requester menu** (Fulfilled phase):
  - Read-only "This request has been fulfilled." message
- **Helper menu** (Active / Offers Received phases):
  - **Withdraw offer**: Coral text → withdraws the user's offer
- **Helper menu** (Fulfilled / Cancelled phase):
  - Read-only "This request is [fulfilled/cancelled]." message
- **Non-participant menu** (Active / Offers Received):
  - **Offer to help**: Highlighted sage row → creates offer
- Panel has same animation as experience (300ms cubic bezier slide-up).
- Each row: icon in colored circle + label + subtitle + chevron.

### E. Phase-to-State Mapping

The three UI phases map to request state:

| Request State | UI Phase | Phase Index |
|---|---|---|
| ACTIVE | Active | 0 |
| OFFERS_RECEIVED | Offers Received | 1 |
| FULFILLED | Fulfilled | 2 |
| CANCELLED | (special) | - |

This is simpler than experiences (which have 4 phases). The request workflow
does not have a "Confirmed" or "Wrap Up" phase — it goes directly from
offers received to fulfilled.

### F. System Messages (Enhanced)

Certain system messages get special treatment in request conversations:

| System Message Type | Server Status | Enhancement |
|---|---|---|
| Request created | **New** | Renders request info card below it |
| User offered help | Existing | Highlight pill: "Thomas offered to help" (coral bg) |
| Request fulfilled | Existing | Highlight pill + renders completion card below it |
| Request cancelled | Existing | Highlight pill: coral bg, cancelled icon |

Highlighted system messages use the same coral semi-transparent background as
experience system messages (matching the experience chat workflow pattern).

---

## Recommendations

### 1. Minimal server changes, client-heavy implementation

One new system message type (`REQUEST_CREATED`) is needed on the server to
anchor the request info card at the correct timeline position. Beyond that,
no new RPCs or proto fields are required — all existing request RPCs
(`OfferToFulfill`, `MarkRequestFulfilled`, `CancelRequest`, `WithdrawOffer`,
`UpdateRequest`) are reused.

Widget cards pull their interactive state from
`ConversationState.cachedRequest` (already populated by
`fetchRequestDetails()`), not from message payloads. This means cards always
show current state rather than the state at message creation time.

### 2. Reuse existing infrastructure from experience workflow

The request chat workflow should reuse — not duplicate — the shared
infrastructure built for experiences:

- **`WidgetShell`** → Same glassmorphic card container
- **`WorkflowWidgetFactory`** → Extended with request-specific methods
- **`ManagePanel`** → Same slide-up panel infrastructure, request-specific
  menu items
- **Phase progress bar** → Same `TransferProgressBar` rendering, 3 segments
  instead of 4
- **System message highlighting** → Same `shouldHighlightSystemMessage()`
  extended for request types
- **Helper badges on bubbles** → Mirrors RSVP badges (same widget, different
  data source)
- **Floating top bar** → Already works for requests (no changes needed)

### 3. Reuse existing widget code from request modals

The widget cards should adapt — not duplicate — existing modal components:

- **Impact metrics** → adapt from `ImpactMetricsCard` in request owner modal
- **Helper list / avatars** → adapt from `HelperOfferCard`
- **Progress bar segments** → reuse from `TransferProgressBar`

### 4. Keep modals as fallback

The existing request owner and helper modals should remain accessible (via the
manage menu's "View full details" action or from the request content view).
Some users may prefer the modal for complex edits (e.g., updating description,
changing media). The chat workflow cards handle the 80% case; modals handle
the 20%.

### 5. Widget card rendering strategy

Extend `WorkflowWidgetFactory` with request-specific methods:

```dart
// In WorkflowWidgetFactory
static Widget? buildRequestCard({
  required ChatMessage message,
  required ConversationState state,
  required bool isOwner,
  required VoidCallback? onOffer,  // Callback wired by screen to ViewModel
}) {
  if (message.systemAction == ChatSystemAction.REQUEST_CREATED) {
    return RequestInfoCard(
      request: state.cachedRequest,
      isOwner: isOwner,
      // Computed getter on ConversationState — derived from cachedRequest
      // offer list + current user ID, not a separate stored field.
      hasOffered: state.hasCurrentUserOffered,
      muted: state.isRequestTerminal,
      onOffer: onOffer,  // Screen wires this to ViewModel method
    );
  }
  if (message.systemAction == ChatSystemAction.FULFILLED) {
    return RequestCompletionCard(
      request: state.cachedRequest,
      impactMetrics: state.requestImpactMetrics,
    );
  }
  return null;
}
```

**Note on `hasCurrentUserOffered`**: This should be a **computed getter** on
the Freezed `ConversationState` class (derived from `cachedRequest.offers`
+ `currentUserId`), not a separately stored field. This avoids stale state
and keeps the single source of truth in `cachedRequest`.

### 6. Phase pill in the input bar

The request-specific phase progress and manage menu should be added to
`MessageInputBar` as a "request mode" that mirrors the experience mode. The
input bar already supports an experience workflow layout; this adds a parallel
request layout. The same three-section structure (progress bar, status row,
input row) is used, with request-specific phase labels and manage menu items.

### 7. Muting of stale widget cards

When a workflow advances past a card's purpose, the card renders with reduced
opacity (0.7) and no interactive elements, matching the experience pattern:

- Request info card: muted when request is FULFILLED or CANCELLED
- Completion card: always read-only (no muting needed)

---

## Design Decisions (Resolved)

### 1. Offer visibility: Show to all participants

The request info card shows the current offer count and helper avatars to **all
participants**, not just the requester. This creates social proof ("3 people are
already helping") and encourages coordination. Matches the experience pattern
where everyone can see who has RSVP'd.

### 2. No summary field — the chat is the summary

The request owner modal currently has a summary editing step (via
`SummaryEditCard`) with AI-generated summaries. With the chat-based workflow,
**summaries are removed entirely** — both the AI generation step during the
workflow and the summary field on the completion card. The chat conversation
itself serves as the record of how the request was fulfilled. The completion
card shows only impact metrics (time saved, emissions prevented), helper
avatars, and duration — no summary text.

This also means the `SummaryEditCard` and related AI summary generation code
in the request owner modal can be removed or simplified in a future cleanup
pass once the chat workflow is the primary interface.

### 3. No "helper confirmed" state

Requests go directly from OFFERS_RECEIVED to FULFILLED without a formal helper
selection step. The requester coordinates in chat and marks fulfilled when done.
Adding an intermediate state would increase complexity and create friction for
a workflow that typically involves 1-2 helpers. If helper confirmation becomes
important later, it can be added as a separate enhancement.

### 4. Request info card visible to all community members

Community members who haven't offered see the request info card with an "I'll
Help" button. This turns the conversation into a recruitment surface — browsing
the chat naturally leads to offering help.

### 5. DETAIL_CHANGED for request edits: Yes, but deferred

When the requester updates the request title, description, or location, a
`DETAIL_CHANGED` system message should be emitted (same type used by
experiences). This is deferred to a future polish phase — request edits are
less common than experience time/location changes, and the current
`UpdateRequest` flow works fine without inline notifications.

---

## Implementation Plan

### Phase 0: Server — New System Message Type

**Goal**: Anchor request creation in the chat timeline.

**Changes**:
- Add `REQUEST_CREATED` system message type to chat proto enum.
- Emit `REQUEST_CREATED` in `SubmitRequest` (`lifecycle.go`) after the
  conversation is created. Message description: "[User] created a request:
  '[request title]'".
- The request service already has a `systemMessageWriter` interface injected
  at construction (used for existing OFFERED, FULFILLED, CANCELLED messages).
  The new `REQUEST_CREATED` emission uses this same interface — **no new
  service-to-service dependency** is introduced. Per
  [server architecture.md](../server/architecture.md#service-independence),
  services must not call other services directly; the `systemMessageWriter`
  interface is the approved abstraction for emitting system messages.

**Files modified**:
- `proto/ripls/models/chat.proto` (add enum value)
- `server/services/request/lifecycle.go` (emit REQUEST_CREATED via existing
  `systemMessageWriter` after conversation creation)

**Tests**:
- Table-driven unit test: `SubmitRequest` emits `REQUEST_CREATED` system
  message with correct description and conversation ID (per
  [server architecture.md](../server/architecture.md#testing-strategy) —
  use table-driven tests for multiple cases).
- Verify message is stored and retrievable via `ListMessages`.

**Estimated scope**: ~30-50 lines changed + ~40-60 lines test code

---

### Phase 1: Request Info Card + Helper Badges

**Goal**: First interactive widget card in the request timeline, plus helper
status on message bubbles.

**Changes**:
- **Request info card**: New widget `RequestInfoCard` rendered after
  `REQUEST_CREATED` system message. Shows request title, description snippet,
  location, and "I'll Help" / "Helping" button. Uses `WidgetShell` container.
  Mutes when request is terminal.
- **Helper badges on bubbles**: Add `offerStatus` parameter to message bubble
  (mirrors RSVP badge). Render "Helping" pill badge (sage green) next to sender
  name when the sender has offered. Thread offer status from ViewModel through
  `MessageList` to individual bubbles.
- **WorkflowWidgetFactory**: Extend with `buildRequestCard()` method that
  returns `RequestInfoCard` for `REQUEST_CREATED` messages in request
  conversations.
- **System message highlighting**: Extend `shouldHighlightSystemMessage()` to
  highlight `OFFERED` messages in request conversations (coral background).
- **State refresh on system message**: When a request-relevant system message
  arrives via the stream (OFFERED, FULFILLED, CANCELLED), call
  `fetchRequestDetails()` to refresh widget card data.

**Files modified**:
- `app/lib/presentation/widgets/chat/workflow_widget_factory.dart` (extend with request methods)
- `app/lib/presentation/widgets/chat/message_bubble.dart` (helper badge, mirrors RSVP badge)
- `app/lib/presentation/widgets/chat/system_message_widget.dart` (highlight for request events)
- `app/lib/presentation/viewmodels/conversation_view_model.dart` (offer action, refresh on system messages)
- New: `app/lib/presentation/widgets/chat/request_info_card.dart`

**Tests**:
- `WorkflowWidgetFactory`: Returns request info card for `REQUEST_CREATED`
  system message in request conversations; returns `null` for non-request.
- Widget test: Request info card renders title, description, location.
- Widget test: "I'll Help" button visible for non-owner; "Helping" for offerer;
  hidden for owner.
- Widget test: Card mutes when request is FULFILLED or CANCELLED.
- Widget test: Helper badge renders "Helping" for offerers; no badge for
  non-offerers.
- ViewModel: Offer action calls `offerToFulfill()` via repository and refreshes
  request details.
- ViewModel: Disposal test — async `offerToFulfill` handles disposal gracefully.

**Estimated scope**: ~300-400 lines added + ~130-170 lines test code

---

### Phase 2: Bottom Bar Phase Progress + Request Manage Menu

**Goal**: Phase-aware bottom bar with manage menu for request conversations.

**Changes**:
- **Bottom bar — request layout**: Add request-specific mode to
  `MessageInputBar`:
  - Three thin bar segments (Active, Offered, Fulfilled)
  - Phase pill button (colored dot + label + chevron) + participant avatars
    (right-justified)
  - No item title row (title in floating top bar)
- **Phase computation**: Add `requestPhase` getter to ViewModel that maps
  request state → phase index (0-2).
- **Manage menu panel**: Reuse `ManagePanel` infrastructure from experience
  workflow. Populate with request-specific action rows:
  - **Requester**: Edit request, Share with communities, Mark fulfilled,
    Cancel request (phase-gated)
  - **Helper**: Withdraw offer (Active/Offered phases only)
  - **Non-participant**: Offer to help (Active/Offered phases only)
- **Action callbacks**: Wire manage menu actions to existing ViewModel methods
  (`markFulfilled`, `cancelRequest`, `withdrawOffer`, `offerToFulfill`).

**Files modified**:
- `app/lib/presentation/widgets/chat/message_input_bar.dart` (request layout, phase progress)
- `app/lib/presentation/viewmodels/conversation_view_model.dart` (phase getter, action methods)
- `app/lib/presentation/screens/inbox/conversation_screen.dart` (wire request action handlers)
- New: `app/lib/presentation/widgets/chat/manage_request_panel.dart`

**Tests**:
- ViewModel: `requestPhase` getter returns correct phase for each request state.
- Widget test: Bottom bar renders 3 progress segments for request conversations.
- Widget test: Phase pill shows correct label per state.
- ManageRequestPanel: Shows requester actions for owner; helper actions for
  offerer; offer button for non-participant.
- ManageRequestPanel: "Mark fulfilled" hidden in terminal states.
- ManageRequestPanel: "Withdraw offer" hidden for non-offerers and terminal states.
- ViewModel: Disposal tests — `markFulfilled`, `cancelRequest`, `withdrawOffer`
  handle disposal gracefully.

**Estimated scope**: ~350-450 lines added + ~140-180 lines test code

---

### Phase 3: Completion Card + Impact Metrics

**Goal**: Complete the request lifecycle in chat with a fulfillment summary.

**Changes**:
- **Completion card**: New widget `RequestCompletionCard` rendered after the
  `FULFILLED` system message. Shows request title, helper avatars + count,
  days-open duration, and impact metrics (time saved, emissions prevented).
  Uses `WidgetShell` container. Read-only. Tappable to open request detail view.
- **Impact data**: The `MarkRequestFulfilled` RPC already returns
  `ImpactEstimate` in the response. Store this as an immutable Freezed field
  in `ConversationState` when the ViewModel processes the fulfillment action.
  For other participants who receive the `FULFILLED` system message, the
  ViewModel should fetch stats via `requestRepository.getRequestStats()`
  (per [architecture.md](../client/architecture.md) — ViewModels call
  repositories, never services directly).
- **WorkflowWidgetFactory**: Extend to return `RequestCompletionCard` for
  `FULFILLED` system messages in request conversations.

**Files modified**:
- `app/lib/presentation/widgets/chat/workflow_widget_factory.dart` (extend for completion)
- `app/lib/presentation/viewmodels/conversation_view_model.dart` (store impact data in Freezed state, stats fetch via repository)
- New: `app/lib/presentation/widgets/chat/request_completion_card.dart`

**Tests**:
- `WorkflowWidgetFactory`: Returns completion card for `FULFILLED` message in
  request conversation.
- Widget test: Completion card renders title, helper avatars, duration, impact
  metrics.
- Widget test: Completion card is read-only (no interactive elements).
- ViewModel: Impact data stored in Freezed state after `markFulfilled` action.
- ViewModel: `fetchRequestDetails` after `FULFILLED` system message fetches
  stats via repository.
- ViewModel: Disposal test — async `markFulfilled` handles disposal gracefully
  (per [architecture.md](../client/architecture.md#async-safety-in-notifiers)
  — all async Notifiers must have disposal tests).

**Estimated scope**: ~200-300 lines added + ~80-120 lines test code

---

### Phase 4: Cross-Provider Invalidation + Polish

**Goal**: Ensure request state stays consistent across screens, handle edge
cases, and polish the implementation.

**Changes**:
- **Cross-provider invalidation**: After each mutation in the
  `ConversationViewModel` that calls a request repository method
  (`offerToFulfill`, `markFulfilled`, `cancelRequest`, `withdrawOffer`,
  `updateRequest`), invalidate:
  1. `requestProvider(requestId)` — so `RequestContentView` sees fresh data
  2. Related list providers (community request lists, "my requests") via
     repository `refresh*()` methods — per
     [caching.md](../client/caching.md) pattern: "Call repository
     `refresh*()` or `invalidate*()` methods after mutations"
  Use `ref.invalidate(requestProvider(requestId))` — same pattern as
  experience workflow (Phase 7).
- **Cancelled state**: Grey progress bar (all segments grey), coral highlighted
  system message, no active cards, phase pill shows "Cancelled" with grey dot.
- **Empty request** (no offers yet): Request info card shows normally with
  "I'll Help" button. No helper avatars in bottom bar.
- **Keyboard interaction**: Verify manage menu panel + progress bar + input all
  move correctly above keyboard.
- **Scroll behavior**: Auto-scroll to new widget cards when at bottom. "New
  activity" pill when scrolled up.
- **Loading states**: Shimmer/skeleton for widget cards while request data
  loads. Spinner on action buttons during API calls.
- **Error handling**: Toast on failed offer/fulfill/cancel. Rollback optimistic
  updates.
- **Menu dismiss behavior**: Focusing the text input or scrolling the message
  list closes the manage menu (matching experience pattern).

**Files modified**:
- `app/lib/presentation/viewmodels/conversation_view_model.dart` (cross-provider invalidation)
- `app/lib/presentation/widgets/chat/request_info_card.dart` (loading, error states)
- `app/lib/presentation/widgets/chat/manage_request_panel.dart` (cancelled state)
- `app/lib/presentation/screens/inbox/conversation_screen.dart` (scroll, keyboard)

**Tests**:
- ViewModel: After offer action, `requestProvider` is invalidated and related
  list caches are refreshed via repository.
- ViewModel: Disposal safety tests — all new async methods (`offerToFulfill`,
  `markFulfilled`, `cancelRequest`, `withdrawOffer`, `fetchRequestDetails`)
  handle disposal gracefully without throwing (per
  [architecture.md](../client/architecture.md#disposal-testing) — all async
  Notifiers must have disposal tests).
- Cancelled state: Grey progress bar, no active cards, "Cancelled" pill.
- Empty request (no offers): Info card shows with offer button, no helper avatars.
- Error handling: Toast on failed API calls; optimistic updates roll back.
- Menu dismiss: Focusing text input closes manage menu.

**Estimated scope**: ~200-300 lines changed/added + ~100-140 lines test code

---

## Architecture Compliance Notes

### Compliant

- **Async safety**: All new ViewModel async methods (offer, fulfill, cancel,
  withdraw, `fetchRequestDetails()` refresh) must check `ref.mounted` after
  each `await` before updating state (per
  [architecture.md](../client/architecture.md#async-safety) — use
  `SafeNotifierMixin` or explicit `if (!ref.mounted) return` guards).
- **Disposal tests**: All new async ViewModel methods must have disposal safety
  tests that verify the notifier handles disposal during in-flight async
  operations without throwing (per
  [architecture.md](../client/architecture.md#disposal-testing)).
- **ViewModel-only repository access**: All new actions (offer, fulfill, cancel,
  withdraw, fetch stats) go through the ViewModel, not the screen. Widget cards
  receive callbacks wired by the screen to ViewModel methods — cards never
  access repositories directly (per
  [architecture.md](../client/architecture.md) — "ViewModels are the ONLY
  layer that calls Repositories").
- **No manual cache maps in ViewModels**: Impact metrics and request data are
  stored as immutable Freezed fields in `ConversationState`, not manual
  `Map<String, T>` caches. Caching is owned by repositories (per
  [caching.md](../client/caching.md) — "Repositories own caching - ViewModels
  don't").
- **Navigation**: Completion card tap-to-detail and manage menu "Edit request"
  both use `NavigationHelpers` methods (per
  [architecture.md](../client/architecture.md#navigation--gestures) — "All
  in-app forward navigation must use `NavigationHelpers`").
- **Computed getters over stored fields**: `hasCurrentUserOffered` and
  `isRequestTerminal` are computed getters on `ConversationState`, derived from
  `cachedRequest` data. This avoids stale duplicated state.
- **Minimal server changes**: One new system message type (`REQUEST_CREATED`)
  added in Phase 0. No new RPCs — all existing request RPCs are reused.
- **Server service independence**: The new system message emission uses the
  existing `systemMessageWriter` interface already injected into the request
  service — no new service-to-service dependency (per
  [server architecture.md](../server/architecture.md#service-independence)).
- **Server test patterns**: Server-side tests use table-driven test patterns
  for multiple cases and `storage.SetupTestStorage(t)` for database tests (per
  [server architecture.md](../server/architecture.md#testing-strategy)).
- **Proto separation maintained**: New message type added to the models proto
  (`chat.proto` enum), not to the API package.
- **Structured logging**: New server-side code (system message emission in
  `SubmitRequest`) uses `logging.LoggerWithContext(ctx)` with structured fields
  (`operation`, `request_id`, `user_id`).
- **Widget reuse**: Cards adapt existing components rather than duplicating.
- **Immutable state**: New fields added to `ConversationState` (Freezed).
- **Cache invalidation**: After mutations, repositories refresh caches
  (including list caches). Cross-provider invalidation ensures
  `RequestContentView` and list screens see fresh data. Widget cards re-render
  via `ref.watch()`.
- **File size**: New widgets are separate files (< 1000 lines each).

### Risk: conversation_view_model.dart growth

The ViewModel is already large and growing with experience workflow methods.
Adding request workflow actions will increase it further. The request actions
are simpler than experiences (no RSVP, time voting, attendance), but still add
methods for offer, fulfill, cancel, withdraw, and request details refresh.

**Mitigation**: After Phase 2, evaluate whether to extract a
`RequestWorkflowMixin` from the ViewModel (parallel to any future
`ExperienceWorkflowMixin`). Don't pre-optimize — wait until the full shape is
visible.

### Risk: WorkflowWidgetFactory complexity

The factory currently handles only experiences. Adding request support doubles
its surface area. The factory should dispatch based on conversation type early
and delegate to type-specific methods.

**Mitigation**: Use a clean switch on `ConversationContext.type` at the top
of each factory method, delegating to `_buildExperienceCard()` and
`_buildRequestCard()` private methods. Keep the public API surface small.

---

**Last Updated:** 2026-02-26
