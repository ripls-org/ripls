# Chat Workflow: Giveaway Conversations

## Overview

Integrate smart workflow components directly into giveaway chat conversations,
replacing the current pattern of navigating away to a separate modal for all
workflow actions. The chat becomes the primary coordination surface: gear info
cards, participant status badges, and completion impact cards appear as inline
widget cards within the message timeline. A persistent bottom bar provides phase
awareness and quick access to owner/participant actions without scrolling.

This is the third workflow type to receive this treatment, following the
experience and request workflows. The patterns established there —
`WorkflowWidgetFactory`, `WidgetShell`, `ManagePanel`, phase progress bar —
are reused and extended for giveaways.

Giveaways have a distinctive architecture: a single **group conversation** is
shared among the gear owner and all interested parties, making the conversation
a multi-participant space where transparency is a design goal. This is
fundamentally different from request conversations (typically small, focused)
and experience conversations (RSVP-driven).

## Reference Materials

- **Request chat workflow spec:**
  [docs/ai/chat-wf-req.md](chat-wf-req.md) — Most recent pattern reference,
  all shared infrastructure
- **Experience chat workflow spec:**
  [docs/ai/chat-wf-exp.md](chat-wf-exp.md) — Original pattern, shared
  infrastructure
- **Giveaway workflow spec:**
  [docs/workflows/giveaway.md](../workflows/giveaway.md) — Transfer states,
  user operations, business rules
- **Existing implementation:**
  - [conversation_screen.dart](../../app/lib/presentation/screens/inbox/conversation_screen.dart) — Current conversation screen
  - [conversation_view_model.dart](../../app/lib/presentation/viewmodels/conversation_view_model.dart) — ViewModel (has `cachedTransfer`, `GearTransferContext`)
  - [message_input_bar.dart](../../app/lib/presentation/widgets/chat/message_input_bar.dart) — Input bar (experience + request modes already added)
  - [workflow_widget_factory.dart](../../app/lib/presentation/widgets/chat/workflow_widget_factory.dart) — Widget card factory (experience + request cards)
  - [widget_shell.dart](../../app/lib/presentation/widgets/chat/widget_shell.dart) — Glassmorphic card container
  - [manage_request_panel.dart](../../app/lib/presentation/widgets/chat/manage_request_panel.dart) — Request manage panel (template for giveaway panel)
  - [giveaway_giver_modal.dart](../../app/lib/presentation/screens/gear/giveaway_giver_modal.dart) — Existing owner modal (Select → Pickup → Complete)
  - [conversation.proto](../../proto/ripls/api/conversation.proto) — ConversationContext + GearTransferContext

---

## Problems

### 1. Workflow actions require leaving the conversation

Currently, all giveaway management (selecting a recipient, completing,
cancelling) happens in a separate modal (`GiveawayGiverModal`). Users tap the
contextual action button, wait for the modal to load, perform the action,
dismiss, and return to the group chat. This context switch breaks the
conversational flow — people are discussing the item *in chat* but must *leave
chat* to act on it.

**Impact**: Extra taps, lost scroll position, cognitive overhead. The same
problem solved for experiences and requests.

### 2. No visibility into giveaway phase from the chat

The chat timeline shows only text and bare system messages like "Alex joined the
conversation" and "Sam was selected as the recipient." There is no visual
indicator of the current phase (Open → Recipient Selected → Completed), no count
of who's interested, and no way to see state without opening the gear screen or
a modal.

**Impact**: Participants check the gear screen repeatedly just to understand
where the giveaway stands.

### 3. Interest activity is invisible in message context

When community members express interest, only a system message appears.
There is no card showing the current interest count, participant avatars, or
the gear details that prompted interest. Community members scrolling through the
conversation can't tell at a glance how much interest the item has.

**Impact**: The owner has to mentally track who's interested. New members
joining the conversation must scroll up to understand context.

### 4. Participant status is invisible on message bubbles

Chat messages show no indication of whether the sender has expressed interest,
was selected, or is the owner. When someone says "I could really use that," you
can't see whether they've formally expressed interest or are just chatting.

**Impact**: The owner and other participants must cross-reference messages with
the interests list in the modal. The social context that makes group
coordination effective is missing.

### 5. Completion is disjointed

After a giveaway completes, the impact metrics and conclusion appear only in the
modal, not in the shared conversation where the wrap-up discussion happens.

**Impact**: The giveaway lifecycle ends in a modal rather than in the group
chat where the community celebrated the generosity.

---

## Proposed Solution

### Core Concept: Widget Cards in the Chat Timeline

Reuse the **widget card** infrastructure from experience and request workflows —
interactive, state-aware UI components embedded in the chat message timeline.
Cards render at specific points in the conversation and update based on the
current giveaway state.

Widget cards are **not chat messages**. They are client-rendered components
anchored to system events. The server continues to send standard system messages;
the client enriches certain messages with interactive widget cards.

### Architecture

```
Server                                   Client
──────                                   ──────
SystemMessage
  type: GIVEAWAY_SHARED (new)       →    SystemMessageWidget
                                           + WorkflowWidgetCard (gear info card)

SystemMessage
  type: JOINED                      →    SystemMessageWidget (highlighted, sage)
                                           sender badge: "Interested"

SystemMessage
  type: APPROVED                    →    SystemMessageWidget (highlighted, sage)
                                           sender badge: "Selected"

SystemMessage
  type: COMPLETED                   →    SystemMessageWidget
                                           + WorkflowWidgetCard (completion card)

SystemMessage
  type: CANCELLED                   →    SystemMessageWidget (highlighted, coral)
```

**Server changes required**:
- `GIVEAWAY_SHARED` (new): Emitted when gear is shared with a community for
  giveaway (in `CommunityService.ShareGear`). Anchors the gear info card at
  the correct timeline position — the first thing anyone sees when opening
  the conversation.
- Extend `GearTransferContext` with `selected_recipient` and `overall_phase`
  fields (see Phase 0).

**Existing system message types reused**:
- `CHAT_SYSTEM_ACTION_JOINED` (1): Interest expressed — already emitted
- `CHAT_SYSTEM_ACTION_LEFT` (2): Interest withdrawn — already emitted
- `CHAT_SYSTEM_ACTION_APPROVED` (3): Recipient selected — already emitted
- `CHAT_SYSTEM_ACTION_COMPLETED` (7): Giveaway completed — already emitted
- `CHAT_SYSTEM_ACTION_CANCELLED` (5): Giveaway cancelled — already emitted

### Key Design Decisions

1. **Client-rendered, server-anchored**: Widget cards are rendered client-side
   based on system message types. One new message type (`GIVEAWAY_SHARED`) is
   added. Cards read live state from `GearTransferContext` in the ViewModel,
   so they always show current data.

2. **GearTransferContext is the data source**: Unlike request conversations
   which use a `cachedRequest` object, giveaway conversations already have
   `GearTransferContext` — a purpose-built data structure that includes the
   user's transfer state, available actions, pending requests, and owner flag.
   We extend this rather than introducing parallel state.

3. **Select Recipient is inline** (revised in Phase 5): Choosing a recipient
   from the list of interested users is handled inline in the bottom bar area,
   not via a separate modal. The `ManageGiveawayPanel` transitions to a
   recipient picker view showing `UserAvatar` + name + "Select" button for each
   interested user. This keeps the entire giveaway workflow in the conversation.
   The `GiveawayGiverModal` is retained for access from the gear detail screen
   but is no longer navigated to from the chat.

4. **Both owner and recipient can complete**: Unlike requests, giveaway
   completion can be triggered by either the owner or the selected recipient
   (`CompleteTransfer` validates this server-side). Both users see a
   "Complete Giveaway" action in the manage panel when in the Recipient
   Selected phase.

5. **Two participant badge states**: Giveaway conversations have richer
   participant semantics than requests — participants can be Interested
   (INTEREST_EXPRESSED) or Selected (RECIPIENT_SELECTED). Both states get
   distinct badge styles on message bubbles.

6. **Multi-party group chat**: Giveaway conversations can have many participants.
   The bottom bar participant avatars show interested users from
   `GearTransferContext.pending_requests`. After recipient selection, only the
   selected recipient's avatar is shown alongside the owner's.

---

## Detailed Component Design

### A. Gear Info Card (Widget Card)

Rendered in the timeline after the `GIVEAWAY_SHARED` system message.

- **Container**: Reuse `WidgetShell` — `AppColors.transferBackgroundCream`
  background, `backdropFilter: blur(12)`, rounded 18px, subtle border, shadow.
  When muted: reduced opacity (0.7), no shadow.
- **Content**:
  - **Gear image**: `CachedMediaImage` thumbnail (right-justified), 56×56px
    rounded rect. Use stable cache key from MediaUrl. Omitted if no media.
  - **Title**: Gear name (serif display font, 17pt, bold)
  - **Description**: Gear description (body text, 2-3 lines max with ellipsis)
  - **Action button**: "I'm Interested" for non-owners who haven't expressed
    interest. "Interested ✓" (muted sage) for users who have. Hidden for the
    gear owner. Hidden when phase is terminal (Completed or Cancelled).
  - No icons or interest count on this card — keep it clean and
    text/image-focused. Interest activity is visible via participant badges on
    message bubbles and avatars in the bottom bar.
- **Behavior**:
  - Tapping "I'm Interested" calls `expressInterest()` on the ViewModel, which
    calls `TransferRepository.expressInterest()`. Card immediately shows
    "Interested ✓" (optimistic update).
  - Card becomes muted when the giveaway reaches a terminal state.
  - Tapping the card body navigates to the gear detail screen via
    `NavigationHelpers.pushScreen()`.
- **Source data**: `GearTransferContext` from `ConversationState.
  conversationContext.gearTransferContext`. Gear details from cached gear data
  fetched by the ViewModel on conversation load (already done via
  `fetchGearDetails()`).

### B. Chat Bubbles with Participant Badges

Enhance received message bubbles to show the sender's giveaway status:

- Above each received message bubble (next to the sender's first name): a
  small badge pill showing participation status:
  - **"Interested"** (blue bg/text): Sender is in `pending_requests`
    (INTEREST_EXPRESSED state)
  - **"Selected"** (sage green bg/text): Sender is the `selected_recipient`
    from the extended `GearTransferContext`
- No badge for the gear owner or users with no transfer.
- Badge only shown in giveaway conversations (gear conversations with
  `availability == FOR_GIVEAWAY`).
- Mirrors the RSVP badge pattern from experiences (same widget, different data
  source).
- Badge disappears for terminal states (Completed, Cancelled) — already matched
  by the giveaway.md "Completed (grey)" badge, but since the conversation
  remains open post-completion the badge can also just disappear.

### C. Completion Card (Widget Card)

Rendered after the `COMPLETED` system message.

- **Container**: Same `WidgetShell`, read-only.
- **Content**:
  - **Header**: Check icon (sage) + "Giveaway Complete"
  - **Gear name**: Serif display font, 17pt, bold
  - Horizontal divider
  - **Recipient row**: "Given to [recipient name]" with `UserAvatar` (24px).
    Shown to all participants (transparency is a goal per giveaway.md).
  - **Duration**: "Fulfilled in N days" (from conversation creation to
    completion date)
  - Horizontal divider
  - **Impact metrics**: "Money saved" (sage value), "Emissions prevented"
    (coral value). Adapted from `ImpactMetricsCard` in `GiveawayGiverModal`.
- **Behavior**: Read-only. Tapping → navigates to the gear detail screen via
  `NavigationHelpers.pushScreen()`.
- **Source data**: Impact data from `CompleteTransfer` response, stored as
  an immutable Freezed field in `ConversationState` (`_cachedImpact` — the same
  field already used by experience and request workflows, since a conversation
  is always exactly one workflow type).

### D. Bottom Bar: Phase Progress + Status + Input

Enhance the existing `MessageInputBar` to add a "giveaway mode" parallel to
the existing experience and request modes:

**Section 1: Progress bar** (top edge of the floating card)
- Three thin bar segments (3px height, 3px gap) representing the three
  giveaway phases: Open, Recipient Selected, Completed.
- Color logic: completed = sage green, current = coral, future = light gray.
- Cancelled: all segments grey.

**Section 2: Status row** (below progress bar)
- **Phase pill** (left): Colored dot + phase name + chevron. Tapping opens/
  closes the manage menu panel.
  - Dot color: coral for Open/Recipient Selected, sage for Completed, grey for
    Cancelled.
  - Text: "Open", "Recipient Selected", "Completed", or "Cancelled".
- **Participant avatars** (right): Overlapping `UserAvatar` circles (24px).
  - Phase 0 (Open): Shows interested parties from `pending_requests`.
  - Phase 1 (Selected): Shows selected recipient + owner.
  - Phase 2 (Completed): Empty or shows selected recipient.

**Section 3: Input row** (bottom, unchanged from chat v2)
- Plus button + text field + send button.

**Manage menu panel** (slides up between sections 2 and 3):
- Same slide-up animation as experience/request panels (300ms cubic bezier).
- Frosted-glass background, tap-outside-to-close.
- **Owner menu** (Open phase):
  - **Select Recipient**: Transitions the manage panel to an inline recipient
    picker (see Phase 5). No modal navigation.
  - **Cancel Giveaway**: Coral text → calls `CancelTransfer`, closes panel.
- **Owner menu** (Recipient Selected phase):
  - **Complete Giveaway**: Sage highlight → calls `CompleteTransfer`, closes
    panel, shows completion card.
  - **Cancel Giveaway**: Coral text → calls `CancelTransfer`.
- **Owner menu** (terminal phase):
  - Read-only: "This giveaway has been completed." or "This giveaway has been
    cancelled."
- **Interested party menu** (Open phase):
  - **Withdraw Interest**: Coral text → calls `WithdrawInterest`, removes user
    from conversation participant list.
- **Selected Recipient menu** (Recipient Selected phase):
  - **Complete Giveaway**: Sage highlight → calls `CompleteTransfer`.
- **Non-selected party menu** (Recipient Selected phase):
  - Read-only: "Another recipient has been selected."
- **Non-participant menu** (Open phase):
  - **Express Interest**: Sage highlight → calls `ExpressInterest`, adds user
    to group conversation.
- All terminal states for non-owners: read-only "This giveaway is [completed/
  cancelled]."

### E. Phase-to-State Mapping

The three UI phases map to transfer state:

| Transfer State | UI Phase | GiveawayPhase Enum |
|---|---|---|
| (no transfer / all INTEREST_EXPRESSED) | Open | `GIVEAWAY_PHASE_OPEN` |
| RECIPIENT_SELECTED | Recipient Selected | `GIVEAWAY_PHASE_RECIPIENT_SELECTED` |
| COMPLETED | Completed | `GIVEAWAY_PHASE_COMPLETED` |
| CANCELLED | Cancelled | `GIVEAWAY_PHASE_CANCELLED` |

Phase is computed from `GearTransferContext.overall_phase` (a `GiveawayPhase`
enum — see Phase 0 server changes). The server computes this from the transfer
state, since clients cannot always determine the overall state from their own
`user_transfer` alone (e.g., a non-selected participant in Phase 1 only knows
their own transfer is still INTEREST_EXPRESSED). `GIVEAWAY_PHASE_UNSPECIFIED`
(the zero value) means the field is not populated — the client should not
render giveaway workflow UI in this case.

### F. System Messages (Enhanced)

Certain system messages get special treatment in giveaway conversations:

| System Message Type | Server Status | Enhancement |
|---|---|---|
| Gear shared for giveaway | **New** | Renders gear info card below it |
| User expressed interest (JOINED) | Existing | Highlight: sage bg, "Interested" badge |
| User withdrew interest (LEFT) | Existing | Dimmed system message (no highlight) |
| Recipient selected (APPROVED) | Existing | Highlight: sage bg, "Selected" badge |
| Giveaway completed (COMPLETED) | Existing | Highlight + renders completion card |
| Giveaway cancelled (CANCELLED) | Existing | Highlight: coral bg, cancelled icon |

---

## Recommendations

### 1. Extend GearTransferContext rather than adding parallel state

Unlike requests (which introduced a `cachedRequest` object), giveaway
conversations already have `GearTransferContext` — a rich, purpose-built
structure. The right pattern is to extend it with two new fields:

```protobuf
// Phase of a giveaway lifecycle for the UI progress bar.
// Server-computed from the active transfer's state so that all participants
// (including non-selected parties) see the correct phase.
enum GiveawayPhase {
  GIVEAWAY_PHASE_UNSPECIFIED = 0;
  GIVEAWAY_PHASE_OPEN = 1;              // All transfers are INTEREST_EXPRESSED
  GIVEAWAY_PHASE_RECIPIENT_SELECTED = 2; // A recipient has been chosen
  GIVEAWAY_PHASE_COMPLETED = 3;          // Giveaway completed
  GIVEAWAY_PHASE_CANCELLED = 4;          // Giveaway cancelled
}

message GearTransferContext {
  // ... existing fields ...

  // The selected recipient (if any). Set when a recipient has been chosen
  // (transfer in RECIPIENT_SELECTED state). Visible to all participants.
  TransferRequest selected_recipient = 5;

  // Overall phase for the UI progress bar. Server-computed from the active
  // transfer's state (not the current user's user_transfer).
  GiveawayPhase overall_phase = 6;
}
```

Using an enum rather than a raw `int32` avoids protobuf3 zero-value ambiguity
(an unset `int32` defaults to 0, which would be indistinguishable from "Open")
and makes both server and client code more self-documenting. The server computes
`overall_phase` so that all participants — including non-selected parties who
only see their own `user_transfer` in INTEREST_EXPRESSED — get the correct
phase.

### 2. Reuse existing infrastructure

All shared infrastructure from experience and request workflows is reused:

- **`WidgetShell`** → Same glassmorphic card container
- **`WorkflowWidgetFactory`** → Extended with `buildGiveawayCard()`
- **`ManagePanel` pattern** → New `ManageGiveawayPanel` widget following the
  same structure as `ManageRequestPanel`
- **Phase progress bar** → Same three-segment structure as request workflow
- **System message highlighting** → Extend `shouldHighlightSystemMessage()`
  with `isGiveawayConversation` parameter
- **Participant badges** → Mirrors RSVP badges and helper badges (same widget,
  different data source)
- **`_cachedImpact`** → Same Freezed field in `ConversationState`, reused since
  a conversation is always exactly one workflow type

### 3. Select Recipient is inline (revised in Phase 5)

Recipient selection is handled inline in the `ManageGiveawayPanel`. The panel
transitions from the default action menu to a recipient picker view showing
each interested user with an avatar, name, and "Select" button. This is
feasible because the interested users are already available in
`GearTransferContext.pending_requests` and the list is typically small (a
handful of people). The inline picker avoids the context-switch of navigating
to `GiveawayGiverModal` and keeps the entire giveaway workflow in the chat.
The `GiveawayGiverModal` remains accessible from the gear detail screen.

### 4. Widget card rendering strategy

Extend `WorkflowWidgetFactory` with giveaway-specific methods:

```dart
// In WorkflowWidgetFactory
static Widget? buildGiveawayCard({
  required ChatMessage message,
  required ConversationState state,
  required VoidCallback? onExpressInterest,
  required VoidCallback onTap,
}) {
  final gearCtx = state.conversationContext?.gearTransferContext;
  if (message.systemAction == ChatSystemAction.GIVEAWAY_SHARED) {
    return GearInfoCard(
      gear: state.cachedGear,           // already fetched by ViewModel
      gearTransferContext: gearCtx,
      muted: _isGiveawayTerminal(gearCtx),
      onExpressInterest: onExpressInterest,
      onTap: onTap,
    );
  }
  if (message.systemAction == ChatSystemAction.COMPLETED) {
    return GiveawayCompletionCard(
      gear: state.cachedGear,
      gearTransferContext: gearCtx,
      impactMetrics: state.cachedImpact,
      onTap: onTap,
    );
  }
  return null;
}
```

### 5. Giveaway detection in ConversationScreen

Giveaway conversations are identified by `conversationContext.hasGearId() &&
conversationContext.availability == Availability.AVAILABILITY_FOR_GIVEAWAY`.
Loan conversations share the same gear conversation structure but have a
different availability type — the `isGiveawayConversation` boolean must check
both conditions.

### 6. Cross-provider invalidation

After mutations (`expressInterest`, `completeTransfer`, `cancelTransfer`,
`withdrawInterest`), invalidate:
1. `gearProvider(gearId)` — so `GearScreen` sees fresh state
2. Transfer list caches via `TransferRepository.refresh*()` methods
3. `GearTransferContext` is refreshed by calling the ViewModel's
   `refreshGearTransferContext()` method (already done when system messages
   arrive via stream)

---

## Design Decisions (Resolved)

### 1. GearTransferContext as the single state source

The ViewModel already populates `GearTransferContext` for gear conversations.
Rather than introducing a parallel `cachedGiveaway` or `cachedTransfer`-based
state tree (which would duplicate data), the giveaway workflow reads directly
from the extended `GearTransferContext`. This keeps the data flow consistent
with the existing gear conversation architecture.

### 2. Inline recipient selection (revised in Phase 5)

Recipient selection is handled inline within the `ManageGiveawayPanel`. The
interested users list from `GearTransferContext.pending_requests` is typically
small, making a compact scrollable list with `UserAvatar` + name + "Select"
button per row a natural fit for the bottom bar area. This avoids the
context-switch of a full-screen modal and keeps the entire giveaway workflow
in the conversation. See Phase 5 for full design.

### 3. Completion card shows recipient name to all

The recipient's name is shown in the completion card ("Given to [name]").
Community transparency is an explicit design goal of the giveaway feature (per
giveaway.md). This is also consistent with `APPROVED` system messages which
already name the recipient.

### 4. GIVEAWAY_SHARED anchor, not first JOINED

The gear info card is anchored to `GIVEAWAY_SHARED` (emitted when gear is
shared), not to the first `JOINED` message (when the first person expresses
interest). This means the card appears at the correct chronological point in
the timeline — before anyone has expressed interest — giving new viewers the
gear details as context before they see participation activity.

For conversations created before this feature ships (no `GIVEAWAY_SHARED`
message exists), the client falls back to prepending the gear info card before
the first message (same fallback pattern used by request conversations before
`REQUEST_CREATED` existed).

### 5. Participant avatars in bottom bar: interested parties

In Phase 0 (Open), the bottom bar shows avatars of users from
`pending_requests`. In Phase 1 (Recipient Selected), it shows only the selected
recipient's avatar alongside the owner. This gives all participants visual
context about where the giveaway stands.

### 6. Server-computed phase (differs from request workflow)

The request workflow computes `requestPhase` **client-side** from
`cachedRequest.state` (a single request object owned by the viewing user). This
works because the request's state directly reflects the overall phase.

Giveaways require **server-computed phase** because a non-selected participant's
own transfer is still INTEREST_EXPRESSED even after a recipient has been chosen.
Only the server can inspect all transfers and determine the overall phase. This
is an intentional divergence — not an inconsistency to resolve retroactively.
If request conversations eventually need multi-party phase awareness, they
should migrate to the same server-computed enum pattern.

### 7. Cancelled state: grey but not read-only for owner

When a giveaway is cancelled, the conversation remains open (per giveaway.md).
The progress bar goes full grey, the phase pill shows "Cancelled", and the gear
info card mutes. The manage panel shows a read-only status message — no further
actions (the owner cannot un-cancel from chat; they'd need to re-share the gear
as a new giveaway).

---

## Implementation Plan

### Phase 0: Server — New System Message Type + GearTransferContext Extension

**Goal**: Anchor giveaway creation in the chat timeline; provide client with
sufficient state for phase computation and recipient display.

**Changes**:
- Add `CHAT_SYSTEM_ACTION_GIVEAWAY_SHARED = 17` to `ChatSystemAction` enum in
  **both** `proto/ripls/models/chat.proto` (storage model) and
  `proto/ripls/api/chat_service.proto` (API). Both enums must stay in sync —
  the client uses the API-generated Dart code and the server uses the model
  enum for storage.
- Emit `GIVEAWAY_SHARED` in `CommunityService.ShareGear` (in
  `server/services/community/`) when availability = `FOR_GIVEAWAY`, after the
  conversation is created. Message description: "[Owner] listed [gear name] for
  giveaway." Use the `systemMessageWriter` interface. If `CommunityService`
  does not yet have `systemMessageWriter` injected, inject it at construction
  following the same pattern as `RequestService`.
- Add `GiveawayPhase` enum to `proto/ripls/api/transfer.proto` (see
  Recommendation 1 for full definition).
- Extend `GearTransferContext` in `proto/ripls/api/transfer.proto` with:
  - `selected_recipient` (`TransferRequest`, field 5): Populated server-side
    when a recipient has been selected.
  - `overall_phase` (`GiveawayPhase`, field 6): Server computes this from the
    active transfer's state. `UNSPECIFIED` means not populated (non-giveaway
    conversation). Client must not derive phase from `user_transfer.state`
    alone.
- Update `server/services/community/` (or wherever `GetConversationContext` is
  assembled) to populate the new `GearTransferContext` fields.

**Files modified**:
- `proto/ripls/models/chat.proto` (add enum value 17)
- `proto/ripls/api/chat_service.proto` (add enum value 17 — must stay in sync)
- `proto/ripls/api/transfer.proto` (add `GiveawayPhase` enum + 2 fields to
  GearTransferContext)
- `server/services/community/lifecycle.go` or `ShareGear` handler (emit system
  message, inject systemMessageWriter if needed)
- `server/services/chat/context.go` (populate new GearTransferContext fields)

**Tests** (use `storage.SetupTestStorage(t)` with testcontainers, consistent
with existing service test patterns):
- Table-driven test: `ShareGear` with `AVAILABILITY_FOR_GIVEAWAY` emits
  `GIVEAWAY_SHARED` system message with correct description and conversation ID.
- Table-driven test: `GetConversationContext` for a gear conversation in
  RECIPIENT_SELECTED phase returns `GIVEAWAY_PHASE_RECIPIENT_SELECTED` and
  non-nil `selected_recipient`.
- Table-driven test: `GetConversationContext` for a completed giveaway returns
  `GIVEAWAY_PHASE_COMPLETED`.

**Estimated scope**: ~40-60 lines server changes + ~60-80 lines test code

---

### Phase 1: Gear Info Card + Participant Badges

**Goal**: First interactive widget card in the giveaway timeline, plus
participant status on message bubbles.

**Changes**:
- **Gear info card**: New widget `GearInfoCard` rendered after `GIVEAWAY_SHARED`
  system message. Shows gear image (CachedMediaImage), name, description
  snippet, and "I'm Interested" / "Interested ✓" / hidden for owner. No icons
  or interest count on the card. Uses `WidgetShell`. Mutes when terminal.
- **Participant badges on bubbles**: Add `giveawayStatus` parameter to message
  bubble (parallel to request's `helperBadge`). Render "Interested" pill (blue)
  or "Selected" pill (sage) next to sender name. Status computed from
  `GearTransferContext.pending_requests` and `selected_recipient`.
- **WorkflowWidgetFactory**: Extend with `buildGiveawayCard()` method.
- **System message highlighting**: Extend `shouldHighlightSystemMessage()` with
  `isGiveawayConversation` parameter. Highlight JOINED (sage), APPROVED (sage),
  COMPLETED (sage/info), CANCELLED (coral).
- **State refresh on system message**: When JOINED, LEFT, APPROVED, COMPLETED,
  or CANCELLED arrives via stream, call `refreshGearTransferContext()` to update
  widget card data.
- **Express interest action**: Add `expressInterest()` method to
  `ConversationNotifier`. Calls `TransferRepository.expressInterest()` with the
  gear ID and community ID from conversation context. Refreshes
  GearTransferContext after.

**Files modified**:
- `app/lib/presentation/widgets/chat/workflow_widget_factory.dart` (extend with giveaway)
- `app/lib/presentation/widgets/chat/message_bubble.dart` (participant badge)
- `app/lib/presentation/viewmodels/conversation_view_model.dart` (expressInterest, refresh, isGiveawayConversation getter)
- `app/lib/presentation/screens/inbox/conversation_screen.dart` (wire giveaway card builder)
- New: `app/lib/presentation/widgets/chat/gear_info_card.dart`

**Tests**:
- `WorkflowWidgetFactory`: Returns gear info card for `GIVEAWAY_SHARED` in
  giveaway conversation; returns `null` for non-giveaway.
- Widget test: Gear info card renders name, description, image.
- Widget test: "I'm Interested" visible for non-owner non-participant; "Interested ✓"
  for participant; hidden for owner.
- Widget test: Card mutes when `overall_phase` is `COMPLETED` or `CANCELLED`.
- Widget test: "Interested" badge on bubbles for pending_requests members; "Selected"
  badge for selected_recipient.
- ViewModel: `expressInterest()` calls repository, refreshes GearTransferContext.
- ViewModel: Disposal test — async `expressInterest` handles disposal gracefully.

**Estimated scope**: ~350-450 lines added + ~140-180 lines test code

---

### Phase 2: Bottom Bar Phase Progress + Giveaway Manage Panel

**Goal**: Phase-aware bottom bar with manage menu for giveaway conversations.

**Changes**:
- **Bottom bar — giveaway layout**: Add giveaway mode to `MessageInputBar`:
  - Three thin bar segments (Open, Recipient Selected, Completed)
  - Phase pill button + participant avatars (right-justified)
  - Cancelled: all segments grey
- **Phase computation**: Read `overall_phase` (`GiveawayPhase` enum) from
  `GearTransferContext`. Add `giveawayPhase` getter to ViewModel.
- **Manage menu panel**: New `ManageGiveawayPanel` widget following the same
  structure as `ManageRequestPanel`. Role-based rows:
  - **Owner (Open)**: "Select Recipient" (opens modal), "Cancel Giveaway"
  - **Owner (Recipient Selected)**: "Complete Giveaway", "Cancel Giveaway"
  - **Owner (terminal)**: Read-only status message
  - **Interested party (Open)**: "Withdraw Interest"
  - **Selected Recipient (Recipient Selected)**: "Complete Giveaway"
  - **Non-selected party (Recipient Selected)**: Read-only "Another recipient
    has been selected"
  - **Non-participant (Open)**: "Express Interest"
  - **All (terminal)**: Read-only
- **Action callbacks**: Wire manage menu to ViewModel methods
  (`completeTransfer`, `cancelTransfer`, `withdrawInterest`, `expressInterest`,
  and navigation to `GiveawayGiverModal` for select recipient).

**Files modified**:
- `app/lib/presentation/widgets/chat/message_input_bar.dart` (giveaway layout)
- `app/lib/presentation/viewmodels/conversation_view_model.dart` (giveawayPhase getter, action methods)
- `app/lib/presentation/screens/inbox/conversation_screen.dart` (wire giveaway action handlers)
- New: `app/lib/presentation/widgets/chat/manage_giveaway_panel.dart`

**Tests**:
- ViewModel: `giveawayPhase` getter returns correct phase for each `GiveawayPhase` enum value.
- Widget test: Bottom bar renders 3 progress segments for giveaway conversations.
- Widget test: Phase pill shows correct label per phase.
- ManageGiveawayPanel: Shows owner actions for owner; interest actions for participant; offer button for non-participant.
- ManageGiveawayPanel: Terminal state shows read-only message.
- ManageGiveawayPanel: Selected recipient sees "Complete Giveaway"; non-selected sees read-only.
- ViewModel: Disposal tests — `completeTransfer`, `cancelTransfer`, `withdrawInterest` handle disposal gracefully.

**Estimated scope**: ~350-450 lines added + ~140-180 lines test code

---

### Phase 3: Completion Card + Impact Metrics

**Goal**: Complete the giveaway lifecycle in the group chat with a fulfillment
summary visible to all participants.

**Changes**:
- **Completion card**: New widget `GiveawayCompletionCard` rendered after the
  `COMPLETED` system message. Shows gear name, "Given to [recipient]" with
  avatar, days-open duration, and impact metrics (money saved, emissions
  prevented). Uses `WidgetShell`. Read-only. Tappable to open gear screen.
- **Impact data**: `CompleteTransfer` RPC already returns `ImpactEstimate` in
  the response. Store as `_cachedImpact` in `ConversationState` (same Freezed
  field reused by experience and request workflows — a conversation is always
  exactly one type). For other participants (owner if recipient completes, or
  vice versa) who receive the `COMPLETED` stream message without having called
  `CompleteTransfer`, fetch stats via
  `TransferRepository.getTransferImpact(transferId)` if available, or fall
  back to fetching the transfer's stored `ImpactEstimate`.
- **WorkflowWidgetFactory**: Return `GiveawayCompletionCard` for `COMPLETED`
  messages in giveaway conversations.

**Files modified**:
- `app/lib/presentation/widgets/chat/workflow_widget_factory.dart` (completion card dispatch)
- `app/lib/presentation/viewmodels/conversation_view_model.dart` (store impact, fetch for non-completing party)
- New: `app/lib/presentation/widgets/chat/giveaway_completion_card.dart`

**Tests**:
- `WorkflowWidgetFactory`: Returns completion card for `COMPLETED` in giveaway; `null` otherwise.
- Widget test: Completion card renders gear name, recipient name with avatar, duration, impact metrics.
- Widget test: Completion card is read-only (no interactive elements).
- ViewModel: Impact data stored in Freezed state after `completeTransfer`.
- ViewModel: Impact fetched for non-completing party when `COMPLETED` arrives via stream.
- ViewModel: Disposal test — async `completeTransfer` handles disposal gracefully.

**Estimated scope**: ~200-280 lines added + ~80-120 lines test code

---

### Phase 4: Cross-Provider Invalidation + Polish

**Goal**: Ensure giveaway state stays consistent across screens and polish edge
cases.

**Changes**:
- **Cross-provider invalidation**: After each mutation (`expressInterest`,
  `completeTransfer`, `cancelTransfer`, `withdrawInterest`), invalidate:
  1. `gearProvider(gearId)` — so `GearScreen` sees updated gear state
     (GIVEN_AWAY badge after completion)
  2. Transfer list providers via `TransferRepository.refresh*()` methods
  3. GearTransferContext refreshed via `refreshGearTransferContext()` (already
     called on stream events)
- **Fallback for legacy conversations**: Gear conversations that predate
  `GIVEAWAY_SHARED` have no anchor message for the gear info card. In this
  case, the client prepends the gear info card before the first message in the
  list (same fallback pattern as experience workflow's experience card).
- **Cancelled state**: All progress segments grey, coral system message
  highlight, gear info card muted, phase pill shows "Cancelled" with grey dot.
  Manage panel shows read-only message.
- **Empty conversation** (no interest yet): Gear info card shows with "I'm
  Interested" button. No participant avatars in bottom bar. Interest count hidden
  (or shown as "0 interested").
- **Select Recipient is inline**: Recipient selection no longer navigates to
  `GiveawayGiverModal` from the chat (see Phase 5). The manage panel handles
  selection inline, and `refreshGearTransferContext()` is called after the
  `selectRecipient` API call completes.
- **Keyboard / scroll**: Verify manage menu panel and progress bar move correctly
  above keyboard. Auto-scroll to new widget cards. "New activity" pill when
  scrolled up.
- **Loading states**: Shimmer for widget cards while gear data loads. Spinner
  on action buttons during API calls.
- **Error handling**: Toast on failed express interest / complete / cancel /
  withdraw. Roll back optimistic updates.

**Files modified**:
- `app/lib/presentation/viewmodels/conversation_view_model.dart` (cross-provider invalidation, fallback card logic)
- `app/lib/presentation/widgets/chat/gear_info_card.dart` (loading, error states)
- `app/lib/presentation/widgets/chat/manage_giveaway_panel.dart` (cancelled state)
- `app/lib/presentation/screens/inbox/conversation_screen.dart` (GiveawayGiverModal return refresh, scroll, keyboard)

**Tests**:
- ViewModel: After `expressInterest`, `gearProvider` is invalidated and transfer
  list caches are refreshed via repository.
- ViewModel: Disposal safety tests — all async methods handle disposal.
- Cancelled state: Grey progress bar, muted gear info card, "Cancelled" pill.
- Legacy fallback: No `GIVEAWAY_SHARED` message → gear info card prepended.
- Empty state (no interest): Info card shows with offer button, no avatars.
- Error handling: Toast on failed API calls; optimistic updates roll back.

**Estimated scope**: ~200-300 lines changed/added + ~100-140 lines test code

---

### Phase 5: Inline Recipient Selection (Replace Modal)

**Goal**: Move recipient selection out of the `GiveawayGiverModal` and into the
bottom bar area, consistent with how other giveaway actions (express interest,
withdraw, complete) are handled inline. Selecting a recipient sends a system
message to the group chat announcing the selection.

**Problem with current approach**: The Phase 2 "Select Recipient" manage panel
row opens `GiveawayGiverModal` via `NavigationHelpers.pushScreen()`. This is the
same context-switch problem the entire chat workflow effort is solving — the
owner leaves the conversation, picks a person in a modal, and returns. For a
list of interested users that's already available in `GearTransferContext
.pending_requests`, a full-screen modal is unnecessary.

**Changes**:

- **Inline recipient picker in bottom bar**: When the owner taps "Select
  Recipient" in the `ManageGiveawayPanel`, instead of navigating to
  `GiveawayGiverModal`, the manage panel transitions to a **recipient picker
  view** — a scrollable list of interested users rendered inline in the same
  bottom bar area, between the status row and the input row. This uses the same
  `AnimatedSize` slide-up pattern already established by the manage panels.

- **Recipient picker UI**: A compact, scrollable list of users from
  `GearTransferContext.pending_requests`:
  - Each row: `UserAvatar` (32px) + user's display name + a "Select" button
    (sage pill, right-justified).
  - Header: "Choose a recipient" label + close (×) button to return to the
    normal manage panel.
  - If `pending_requests` is empty: "No one has expressed interest yet." read-
    only message.
  - Max visible height: ~3-4 rows before scrolling (constrained by the same
    `maxHeight` used by other manage panels). `ListView` with shrinkWrap for
    short lists.

- **Selection confirmation**: Tapping the "Select" button on a user row
  triggers a brief inline confirmation: the row highlights, the button changes
  to a spinner, and the ViewModel calls `selectRecipient(transferId)` (which
  calls the existing server-side `SelectRecipient` RPC / transfer state
  transition). No separate confirmation dialog — the action is easily
  reversible (owner can cancel and re-select).

- **System message on selection**: The server already emits a
  `CHAT_SYSTEM_ACTION_APPROVED` system message when a recipient is selected
  (e.g., "Sam was selected as the recipient"). This existing system message
  is sufficient — no new message type is needed. The message is visible to all
  group chat participants, providing the transparency that was previously hidden
  inside the modal.

- **Post-selection state update**: After `selectRecipient` succeeds:
  1. `refreshGearTransferContext()` picks up the new `overall_phase`
     (`GIVEAWAY_PHASE_RECIPIENT_SELECTED`) and `selected_recipient`.
  2. The manage panel auto-closes (or transitions to the Recipient Selected
     owner menu showing "Complete Giveaway" / "Cancel Giveaway").
  3. The progress bar advances to segment 2 (Recipient Selected).
  4. The `APPROVED` system message appears in the chat timeline with the
     "Selected" badge highlight.

- **ManageGiveawayPanel changes**: Replace the current "Select Recipient" row
  (`onSelectRecipient` callback → modal navigation) with a two-state panel:
  - **Default state** (Open phase, owner): Shows "Select Recipient" row as
    before, but tapping it sets a local `_showRecipientPicker` boolean to
    `true`, which swaps the panel content to the recipient picker list.
  - **Picker state**: Shows the recipient list inline. Close (×) returns to
    the default manage menu.

- **MessageInputBar changes**: Increase `maxHeight` constraint slightly when
  the recipient picker is active (the picker may need more vertical space than
  the standard 2-row manage panel). Pass a new `isRecipientPickerOpen` flag
  or just let `ManageGiveawayPanel` manage its own height within the existing
  `AnimatedSize`.

- **Remove modal navigation**: Remove `onSelectRecipient` callback that
  navigates to `GiveawayGiverModal`. The `GiveawayGiverModal` itself is NOT
  deleted — it may still be used from the gear detail screen or transfer list.
  Only the chat conversation's path to it is removed.

- **ViewModel**: Add `selectRecipient(String transferId)` method to
  `ConversationNotifier`. Calls the existing `TransferRepository
  .selectRecipient()` (or equivalent RPC). Refreshes `GearTransferContext`
  after success. Checks `mounted` after each `await`.

**Files modified**:
- `app/lib/presentation/widgets/chat/manage_giveaway_panel.dart` (add inline
  recipient picker state, remove modal navigation trigger)
- `app/lib/presentation/widgets/chat/message_input_bar.dart` (adjust maxHeight
  if needed, remove `onSelectRecipient` modal callback)
- `app/lib/presentation/viewmodels/conversation_view_model.dart` (add
  `selectRecipient()` method)
- `app/lib/presentation/screens/inbox/conversation_screen.dart` (remove
  `_handleOpenSelectRecipient` modal navigation, wire new `selectRecipient`
  callback)

**Tests**:
- ManageGiveawayPanel: Tapping "Select Recipient" shows inline picker with user
  rows from `pending_requests`.
- ManageGiveawayPanel: Empty `pending_requests` shows "No one has expressed
  interest" message.
- ManageGiveawayPanel: Tapping "Select" on a user row calls
  `onSelectRecipient(transferId)` callback.
- ManageGiveawayPanel: Close (×) on picker returns to default manage menu.
- ViewModel: `selectRecipient()` calls repository, refreshes
  GearTransferContext, transitions phase.
- ViewModel: Disposal test — async `selectRecipient` handles disposal
  gracefully.
- Integration: Selecting a recipient closes the picker, advances progress bar,
  and the `APPROVED` system message appears in the timeline.

**Estimated scope**: ~200-280 lines changed/added + ~80-120 lines test code

---

### Phase 6: Giveaway Chat Card, Non-Owner Entry Points & Status Check

**Goal**: Make the chat the primary surface for non-owners interacting with a
giveaway. A prominent gear card at the top of the chat lets participants request
the item directly. All non-owner entry points funnel into the chat. A "Check
Status" menu option gives non-owners a quick read-only view of where their
request stands.

**Changes**:

- **Gear card at top of chat**: When a giveaway conversation is opened, a card
  is pinned to the top of the chat (above the message timeline, not scrollable
  away) showing the gear's name and description. This is distinct from the
  inline `GearInfoCard` widget message — it is a persistent header card visible
  at all times regardless of scroll position.
  - Displays: gear image thumbnail (via `CachedMediaImage`), gear name, gear
    description (truncated to ~2 lines with ellipsis).
  - **Request button**: For non-owners who have NOT yet expressed interest, the
    card includes a primary "Request" button (sage filled pill). Tapping it
    calls `expressInterest` via the ViewModel, then updates the button state.
  - **Requested state**: For non-owners who HAVE expressed interest (determined
    by `hasCurrentUserExpressedInterest` on `GearTransferContext`), the button
    is replaced with a muted "Requested" label (grey pill, non-interactive)
    confirming their request is submitted.
  - **Owner view**: For the gear owner, the card shows the gear info but no
    request button (they own the item).
  - Uses `WidgetShell` for consistent card styling. Not tappable for navigation
    (the user is already in the conversation).

- **Non-owner action bar in GearContentView always opens chat**: When a
  non-owner taps the action bar button in `GearContentView` (the gear detail
  screen), instead of the current behavior, it should always navigate directly
  to the giveaway group conversation. The action bar label should reflect this
  (e.g., "Open Chat" or "View Giveaway").

- **Request button on GearContentView submits then opens chat**: If the
  non-owner taps a "Request" button on the gear detail screen (action bar or
  dedicated button), the app should:
  1. Call `expressInterest` to submit the request.
  2. Update the button to show "Requested" (same muted state as the chat card).
  3. Navigate to the giveaway group conversation (chat screen).
  This ensures the non-owner lands in the chat after expressing interest, making
  the chat the central coordination surface.

- **"Check Status" menu option in conversation view**: In the conversation
  screen's overflow menu (kebab / three-dot menu), non-owners see a "Check
  Status" option. Tapping it opens the giveaway non-owner modal
  (`GiveawayRecipientModal` or equivalent) showing the **first screen** of the
  giveaway request flow:
  - "Waiting on [Owner Name]" header with the owner's avatar.
  - "What Happens Next" section explaining the giveaway process (same content
    as the existing giveaway request modal's initial screen).
  - Read-only — no action buttons. The modal is purely informational, giving
    the non-owner confidence that their request is in progress.
  - If the non-owner has been selected as recipient, the modal reflects that
    state (e.g., "You've been selected!" with next-step instructions).
  - If the giveaway is completed or cancelled, the modal shows the terminal
    state accordingly.

**Files modified**:
- New: `app/lib/presentation/widgets/chat/giveaway_chat_header_card.dart`
  (persistent top-of-chat gear card with request/requested button)
- `app/lib/presentation/screens/inbox/conversation_screen.dart` (mount header
  card above message list for giveaway conversations, add "Check Status" menu
  item for non-owners)
- `app/lib/presentation/viewmodels/conversation_view_model.dart` (wire
  `expressInterest` from header card, expose check-status modal data)
- `app/lib/presentation/widgets/gear/gear_content_view.dart` (non-owner action
  bar always opens chat, request button submits + navigates to chat)
- `app/lib/presentation/screens/gear/giveaway_status_modal.dart` (new or
  adapted read-only modal for non-owner status check)

**Tests**:
- Header card: Non-owner without interest sees "Request" button; tapping calls
  `expressInterest`.
- Header card: Non-owner with interest sees "Requested" label (non-interactive).
- Header card: Owner sees gear info without request button.
- Header card: Gear name and description render correctly, description truncates.
- GearContentView: Non-owner action bar navigates to giveaway conversation.
- GearContentView: Request button calls `expressInterest`, updates to
  "Requested", then navigates to conversation.
- Conversation menu: Non-owner sees "Check Status" option; owner does not.
- Check Status modal: Shows "Waiting on [Owner]" with owner name and avatar.
- Check Status modal: Shows selected state when non-owner is chosen recipient.
- Check Status modal: Shows terminal state for completed/cancelled giveaways.
- ViewModel: Disposal safety for `expressInterest` triggered from header card.

**Estimated scope**: ~300-400 lines added + ~120-160 lines test code

---

### Phase 7: Selection Card Replaces Completion Card

**Goal**: When a recipient is selected, show a rich card in the chat that
celebrates the selection and thanks the community — and make this the final
lifecycle card. No separate completion card is shown when the item is picked up;
the selection card serves as the giveaway's conclusion in the chat timeline.

**Changes**:

- **Selection card in chat timeline**: When the `APPROVED` system message
  arrives (recipient selected), render a `GiveawaySelectionCard` widget card
  below it. This card contains:
  - **Recipient row**: Recipient's `UserAvatar` (40px) + display name,
    prominently styled (serif display font, sage accent).
  - **Impact metrics**: Money saved and emissions prevented — the same impact
    data currently shown in the Phase 3 completion card. Impact is computed at
    selection time (the giveaway's value is known once the item is committed to
    a recipient). Displayed with the same `ImpactMetricsCard`-derived layout.
  - **Thank-you message**: "Thanks to everyone who expressed interest!" — a
    warm, community-facing acknowledgment visible to all group chat participants.
    Uses body text style, muted color, below a horizontal divider.
  - Uses `WidgetShell` with sage accent styling. Read-only (no interactive
    buttons). Tappable to navigate to the gear detail screen.

- **Remove completion card**: The Phase 3 `GiveawayCompletionCard` is no longer
  rendered. When the `COMPLETED` system message arrives (item picked up), no
  widget card is attached — only the standard system message text appears
  (e.g., "Giveaway completed"). The selection card already contains the
  impact metrics and recipient info, so a second card would be redundant.
  - Remove `GiveawayCompletionCard` from `WorkflowWidgetFactory`'s
    `buildGiveawayCard()` dispatch for `COMPLETED` messages.
  - The `GiveawayCompletionCard` widget file can be deleted or retained for
    potential reuse elsewhere — but it is no longer rendered in the chat
    timeline.

- **Impact data at selection time**: The `SelectRecipient` RPC (or the
  server-side state transition) should return or make available the
  `ImpactEstimate` for the giveaway. If not currently returned, extend the
  `SelectRecipient` response to include it (or compute it client-side from
  gear metadata — retail value, weight, category). Store as `_cachedImpact`
  in `ConversationState` at selection time rather than at completion time.

- **WorkflowWidgetFactory update**: Return `GiveawaySelectionCard` for
  `APPROVED` messages in giveaway conversations. Return `null` for `COMPLETED`
  messages (no card).

- **Fallback for non-selecting participants**: Participants who are online when
  the `APPROVED` stream message arrives see the card immediately. Participants
  who open the conversation later see the card anchored to the `APPROVED`
  system message in the timeline (same anchoring pattern as other widget cards).
  Impact data is fetched via `TransferRepository.getTransferImpact(transferId)`
  if not already cached.

**Files modified**:
- New: `app/lib/presentation/widgets/chat/giveaway_selection_card.dart`
  (recipient avatar/name, impact metrics, thank-you message)
- `app/lib/presentation/widgets/chat/workflow_widget_factory.dart` (return
  selection card for `APPROVED`, remove completion card for `COMPLETED`)
- `app/lib/presentation/viewmodels/conversation_view_model.dart` (store impact
  at selection time, fetch for late-joining participants)
- Delete or deprecate: `app/lib/presentation/widgets/chat/giveaway_completion_card.dart`

**Tests**:
- `WorkflowWidgetFactory`: Returns selection card for `APPROVED` in giveaway
  conversation; returns `null` for `COMPLETED`.
- Widget test: Selection card renders recipient avatar, display name, impact
  metrics, and thank-you message.
- Widget test: Selection card is read-only (no interactive elements).
- Widget test: Selection card tappable → navigates to gear detail screen.
- ViewModel: Impact data stored in state after `selectRecipient`.
- ViewModel: Impact fetched for late-joining participants when `APPROVED`
  message is in timeline but no cached impact exists.
- ViewModel: Disposal test — async impact fetch handles disposal gracefully.
- Integration: `COMPLETED` system message appears as text only, no widget card.

**Estimated scope**: ~200-280 lines added/changed + ~80-120 lines test code

---

### Phase 8: Pickup Confirmation + Interest Card with Participant Avatars

**Goal**: Two improvements: (1) After a recipient is selected, both the winner
and the owner can mark the item as "picked up," which completes the giveaway.
(2) Redesign the gear info card (rendered after `GIVEAWAY_SHARED`) to show an
RSVP-style interest button with participant avatars and names as people express
interest — matching the visual language of experience RSVP cards.

**Changes**:

#### A. Pickup Confirmation ("Mark as Picked Up")

Once a recipient is selected (`GIVEAWAY_PHASE_RECIPIENT_SELECTED`), the
giveaway is not yet complete — the physical handoff still needs to happen.
Both the owner and the selected recipient should be able to confirm the
pickup, which transitions the giveaway to `COMPLETED`.

- **ManageGiveawayPanel — Recipient Selected phase**: Replace the current
  owner "Manage giveaway" row and participant "You have been selected!" read-
  only message with a prominent **"Mark as Picked Up"** action row for both
  the owner and the selected recipient. Uses a check-circle icon with sage
  green styling (consistent with completion). Tapping calls
  `completeTransfer()` on the ViewModel, which calls the existing
  `CompleteTransfer` RPC (already validates that either owner or recipient
  can complete).

- **ManageGiveawayPanel — Owner (Recipient Selected)**: Shows:
  1. "Mark as Picked Up" (sage highlight) — calls `completeTransfer`.
  2. "Cancel Giveaway" (coral text) — calls `cancelTransfer`.

- **ManageGiveawayPanel — Selected Recipient (Recipient Selected)**: Shows:
  1. "Mark as Picked Up" (sage highlight) — calls `completeTransfer`.
  2. The recipient can also see "Check Status" as before.

- **ManageGiveawayPanel — Non-selected participant (Recipient Selected)**:
  Read-only: "Another recipient has been selected." (unchanged).

- **ViewModel**: `completeTransfer()` already exists on `ConversationNotifier`
  (added in Phase 3). No new method needed — just wire it as the callback
  for the "Mark as Picked Up" row in the manage panel.

- **Post-pickup state update**: After `completeTransfer` succeeds:
  1. `refreshGearTransferContext()` picks up `GIVEAWAY_PHASE_COMPLETED`.
  2. The manage panel auto-closes.
  3. The progress bar advances to segment 3 (Completed).
  4. The `COMPLETED` system message appears in the timeline.
  5. Cross-provider invalidation: `gearProvider(gearId)` invalidated (gear
     becomes `GIVEN_AWAY`), transfer caches refreshed.

#### B. Gear Info Card Redesign — RSVP-Style Interest with Avatars

Redesign the `GearInfoCard` to match the visual language of the experience
`RsvpInviteCard` and `AttendanceCard`. Instead of a plain "Express Interest"
button, the card shows a structured interest section with participant avatars
and names that grows as people express interest.

- **Card layout** (replaces current `GearInfoCard` content):
  - **Header row**: Gift icon (coral background, 36×36 rounded rect) +
    "Free Giveaway" label (coral, 12pt semibold) + gear name below (15pt
    bold). Same pattern as `RsvpInviteCard`'s calendar-icon header row.
  - **Description**: Gear description (unchanged, 2-3 lines max).
  - **Interest section** (below description, separated by SizedBox):
    - **Section label**: "Interested?" (13pt, secondary text) — mirrors
      `RsvpInviteCard`'s "Can you make it?" label.
    - **Action button**: Full-width rounded pill button:
      - **Not yet interested**: Coral border + coral text "I'm Interested"
        (same styling as current `_buildExpressInterestButton` but using the
        `AnimatedContainer` pattern from RSVP buttons for smooth transitions).
      - **Already interested**: Sage green fill + white check icon + "Interested"
        text. Tapping opens a confirmation to withdraw (same as current
        `_buildInterestedButton` but with the RSVP selected-state styling).
      - **Owner**: Button hidden (owner doesn't express interest in own item).
      - **Terminal phase**: Section hidden entirely.
    - **Participant list** (below button, shown when ≥1 person has expressed
      interest): A compact vertical list of interested users from
      `GearTransferContext.pending_requests`. Each row:
      - `UserAvatar` (16px radius) + display name (13pt, w500) + timestamp
        ("2h ago", "1d ago") right-justified in secondary text.
      - Rows styled with the same `AnimatedContainer` + rounded rect pattern
        as `AttendanceCard` attendee rows — subtle sage background with sage
        border for visual consistency.
      - Max 4 rows visible; if more, show "+N more" link at the bottom.
      - When a recipient is selected, the selected user's row gets a
        "Selected" pill badge (sage green, right-justified) and appears first.
    - **Empty state** (no interest yet, non-owner view): Just the "I'm
      Interested" button with no participant list. After the user taps it,
      their avatar/name appears in the list (optimistic update).

- **Data source**: `GearTransferContext.pending_requests` (already contains
  `TransferRequest` with `borrower` (User) and `requested_at_unix_sec`).
  Selected recipient from `GearTransferContext.selected_recipient`.

- **New parameter on GearInfoCard**: `List<TransferRequest> pendingRequests`
  (passed from the ViewModel via `GearTransferContext.pending_requests`).
  Also `TransferRequest? selectedRecipient` for highlighting. The card
  already receives `GearTransferContext` — these can be read directly from
  `transferContext.pendingRequests` and `transferContext.selectedRecipient`.

- **Timestamp formatting**: Reuse `DateTimeFormatter` or add a simple
  relative-time helper (e.g., `_formatRelativeTime(int unixSec)`) within
  the card. Keep it simple: "Xm ago", "Xh ago", "Xd ago".

**Files modified**:
- `app/lib/presentation/widgets/chat/gear_info_card.dart` (full redesign:
  header row with icon, interest section with participant list, RSVP-style
  button)
- `app/lib/presentation/widgets/chat/manage_giveaway_panel.dart` (add "Mark
  as Picked Up" action for owner and selected recipient in
  RECIPIENT_SELECTED phase; add `onCompleteTransfer` callback; add
  `onCancelTransfer` callback)
- `app/lib/presentation/screens/inbox/conversation_screen.dart` (wire
  `completeTransfer` and `cancelTransfer` callbacks to manage panel)
- `app/lib/presentation/viewmodels/conversation_view_model.dart` (wire
  `cancelTransfer` method if not already present for giveaway context)

**Tests**:

*Gear Info Card:*
- Card renders gift icon header row with "Free Giveaway" label and gear name.
- Non-owner without interest sees "Interested?" section with "I'm Interested"
  button.
- Non-owner with interest sees "Interested" button (sage, selected state).
- Owner sees gear info without interest section.
- Terminal phase hides interest section entirely.
- Participant list renders avatars and names from `pending_requests`.
- Participant list shows "+N more" when more than 4 interested users.
- Selected recipient appears first in list with "Selected" badge.
- Empty `pending_requests` shows only the button, no participant list.

*Manage Giveaway Panel:*
- Owner in RECIPIENT_SELECTED phase sees "Mark as Picked Up" and "Cancel
  Giveaway" rows.
- Selected recipient in RECIPIENT_SELECTED phase sees "Mark as Picked Up"
  row.
- Tapping "Mark as Picked Up" calls `onCompleteTransfer` callback.
- Non-selected participant sees read-only message (unchanged).
- Terminal states show read-only message (unchanged).

*ViewModel:*
- `completeTransfer` calls repository, refreshes GearTransferContext,
  invalidates gearProvider.
- Disposal test — async `completeTransfer` handles disposal gracefully.

**Estimated scope**: ~250-350 lines changed/added + ~100-140 lines test code

---

### Phase 9: Modal Pickup Action + Phase Pill Bug Fix

**Goal**: Two bug fixes. (1) `GiveawayGiverModal` needs a clear "Mark as Picked
Up" action for both the owner and the winner — the current modal shows a
loan-style pickup-scheduling flow that does not match the simplified giveaway
state machine. (2) The chat manage panel phase pill incorrectly shows "Open"
after certain mutations because `overall_phase` comes back as
`GIVEAWAY_PHASE_UNSPECIFIED` from the server and the client does not guard
against it.

#### A. Modal "Mark as Picked Up" for Owner and Winner

**Context**: `GiveawayGiverModal` is accessed from the gear detail screen (not
from the chat — that path was replaced in Phase 5/8). It currently implements
four phases: `selectRecipient`, `pickupProposed`, `waitingForPickup`, and
`complete`. The `pickupProposed` / `waitingForPickup` phases model a
pickup-time-scheduling flow (`estimatedPickupUnixSec`, `approvePickup` RPC)
that is inherited from the loan workflow and is not part of the giveaway state
machine (which only has `INTEREST_EXPRESSED → RECIPIENT_SELECTED → COMPLETED`).

**Problems**:
- The modal's `pickupProposed` and `waitingForPickup` phases can never be
  reached for giveaways because giveaways skip `ACTIVE` state — there is no
  server-side concept of a pickup-time proposal for giveaways.
- The winner (selected recipient) has no path to "Mark as Picked Up" through
  the modal. Only the owner can currently complete the giveaway via the modal.
- "Mark as Picked Up" in the chat manage panel (added in Phase 8) calls
  `completeTransfer()` correctly, but the modal does not expose the equivalent
  to the recipient.

**Changes**:
- **`GiveawayGiverModal` — Recipient Selected phase for the owner**: Replace
  the `pickupProposed` and `waitingForPickup` phase bodies (which are
  unreachable) with a single clean screen showing:
  - The selected recipient's `UserAvatar` (28px) + name + "Selected as
    Recipient" subtitle.
  - A "Mark as Picked Up" primary button that calls
    `giveawayGiverProvider.notifier.completeGiveaway(transfer.id)`.
  - A "Cancel Giveaway" secondary destructive button (existing logic
    unchanged).
  - The "Pickup Proposed" and "Waiting for Pickup" step labels in the
    progress header should be collapsed to just "Handoff" — or the header
    simplified to two steps: "Select" and "Complete".

- **Winner (selected recipient) view in the modal**: The modal currently has
  no `GiveawayReceiverModal` equivalent that lets the winner confirm pickup.
  Add a thin receiver path: when the viewing user is the selected recipient
  (not the owner), the `GiveawayGiverModal` (or a sibling widget
  `GiveawayReceiverModal` if it already exists) should show:
  - "You've been selected!" confirmation card with gear name and owner name.
  - A "Mark as Picked Up" primary button calling the same `completeTransfer`
    RPC via `TransferRepository`.
  - On success: navigate to the complete phase / show impact card.
  - This does NOT require a separate ViewModel provider — the recipient's
    `userTransfer` (available in `GearTransferContext`) contains the transfer
    ID needed to call `CompleteTransfer`.

- **`GiveawayGiverState` / `GiveawayGiverViewModel`**: The `pickupProposed`
  and `waitingForPickup` phases are unreachable for giveaways. Remove them
  from the phase enum (or mark them as deprecated/unused) to prevent future
  confusion. The relevant phase logic simplifies to:
  - `selectRecipient` — no recipient chosen yet.
  - `recipientSelected` (rename of former `waitingForPickup`) — recipient
    chosen, waiting for handoff.
  - `complete` — giveaway done.

- **No server changes required**: `CompleteTransfer` already accepts both
  owner and recipient as callers.

**Files modified**:
- `app/lib/presentation/screens/gear/giveaway_giver_modal.dart` (simplify
  phases: remove pickup-time-scheduling flow, add recipient-selected body
  with "Mark as Picked Up", simplify progress header to 2 steps)
- `app/lib/presentation/viewmodels/giveaway_giver_state.dart` (simplify
  `GiveawayGiverPhase` enum: remove `pickupProposed` / `waitingForPickup`,
  add `recipientSelected`)
- `app/lib/presentation/viewmodels/giveaway_giver_view_model.dart` (update
  phase mapping logic; `recipientSelected` phase is entered when transfer
  state is `RECIPIENT_SELECTED`)
- `app/lib/presentation/screens/gear/giveaway_status_modal.dart` (if this is
  the winner's read-only modal from Phase 6, add "Mark as Picked Up" button
  when user is the selected recipient in RECIPIENT_SELECTED phase)

#### B. Phase Pill Shows "Open" After Mutations (Bug Fix)

**Root cause**: The `giveawayPhase` getter in `ConversationNotifier` returns
`ctx.gearTransferContext.overallPhase` directly. In protobuf3, unset enum
fields default to their zero value — `GIVEAWAY_PHASE_UNSPECIFIED = 0`. When
the server's `GetConversationContext` populates `GearTransferContext` for a
completed or just-mutated giveaway and does not explicitly set `overall_phase`,
the client receives `GIVEAWAY_PHASE_UNSPECIFIED`. The getter returns this
non-null value, and the `MessageInputBar` progress-bar/pill switch has no case
for `UNSPECIFIED`, so it falls through to the default — which ends up rendering
"Open" (phase index 0) instead of the correct state.

There are two sources of the bug:

1. **Client-side**: `giveawayPhase` must treat `GIVEAWAY_PHASE_UNSPECIFIED`
   as `null` (i.e., "no phase data available"). It currently only guards for
   `GIVEAWAY_PHASE_CANCELLED`.

2. **Server-side**: `GetConversationContext` (or its `GearTransferContext`
   assembly) must populate `overall_phase` for all non-unspecified states,
   including `GIVEAWAY_PHASE_COMPLETED`. Currently it may be omitting the
   field for terminal states.

**Client fix** (`conversation_view_model.dart`):

```dart
GiveawayPhase? get giveawayPhase {
  final ctx = state.conversationContext;
  if (ctx == null || !ctx.hasGearTransferContext()) return null;
  final phase = ctx.gearTransferContext.overallPhase;
  // Treat unspecified (protobuf3 zero default) and cancelled as null —
  // the progress bar renders a greyed-out terminal state for null.
  if (phase == GiveawayPhase.GIVEAWAY_PHASE_UNSPECIFIED ||
      phase == GiveawayPhase.GIVEAWAY_PHASE_CANCELLED) {
    return null;
  }
  return phase;
}
```

**Server fix** (`server/services/chat/context.go` or equivalent):

Ensure `overall_phase` is set in `GearTransferContext` for ALL giveaway
states, including `COMPLETED` and `CANCELLED`. The current server code may
only set the field for `INTEREST_EXPRESSED` / `RECIPIENT_SELECTED`. Extend
the switch/if block to explicitly write:
- `TRANSFER_STATE_COMPLETED` → `GIVEAWAY_PHASE_COMPLETED`
- `TRANSFER_STATE_CANCELLED` → `GIVEAWAY_PHASE_CANCELLED`

This ensures the client always receives an explicit value rather than
relying on the protobuf3 zero default.

**Additionally**, the `MessageInputBar._giveawayPhaseInfo` switch should have
an explicit `null` / default case that returns a sensible fallback (e.g.,
grey "Unknown" or nothing) rather than silently falling through to "Open".
This is defensive — the server fix is the primary remedy.

**Files modified**:
- `app/lib/presentation/viewmodels/conversation_view_model.dart` (guard
  `GIVEAWAY_PHASE_UNSPECIFIED` in `giveawayPhase` getter)
- `app/lib/presentation/widgets/chat/message_input_bar.dart` (add explicit
  null/default case in `_giveawayPhaseInfo` and `_buildGiveawayProgressBar`
  switches)
- `server/services/chat/context.go` (or equivalent assembly file — populate
  `overall_phase` for `COMPLETED` and `CANCELLED` transfer states)

**Tests**:

*Phase pill bug:*
- ViewModel: `giveawayPhase` returns `null` for `GIVEAWAY_PHASE_UNSPECIFIED`.
- ViewModel: `giveawayPhase` returns `GIVEAWAY_PHASE_COMPLETED` when server
  returns that value.
- Widget test: Phase pill shows "Completed" (not "Open") after
  `completeTransfer` when context returns `GIVEAWAY_PHASE_COMPLETED`.
- Widget test: Phase pill renders grey/no-op when `giveawayPhase` is null.
- Server: `GetConversationContext` for a completed giveaway populates
  `overall_phase = GIVEAWAY_PHASE_COMPLETED` in `GearTransferContext`.
- Server: `GetConversationContext` for a cancelled giveaway populates
  `overall_phase = GIVEAWAY_PHASE_CANCELLED`.

*Modal pickup:*
- Modal: Owner in RECIPIENT_SELECTED phase sees "Mark as Picked Up" button.
- Modal: Tapping "Mark as Picked Up" calls `completeGiveaway(transferId)`.
- Modal: After completion, transitions to complete phase (impact screen).
- Modal: Winner (selected recipient) sees "Mark as Picked Up" button when
  their transfer is in RECIPIENT_SELECTED state.
- Modal: `pickupProposed` and `waitingForPickup` phases no longer reachable.

**Estimated scope**: ~150-200 lines changed + ~60-80 lines test code

---

## Architecture Compliance Notes

### Compliant

- **Async safety**: All new ViewModel async methods must check `ref.mounted`
  after each `await` before updating state (per
  [architecture.md](../client/architecture.md#async-safety)).
- **Disposal tests**: All new async ViewModel methods require disposal safety
  tests (per [architecture.md](../client/architecture.md#disposal-testing)).
- **ViewModel-only repository access**: `expressInterest`, `completeTransfer`,
  `cancelTransfer`, `withdrawInterest` all go through the ViewModel. Widget
  cards receive callbacks wired by the screen — cards never access repositories
  directly.
- **No manual cache maps**: Impact metrics stored as immutable Freezed field
  (`_cachedImpact`) in `ConversationState`. Gear data read from existing cached
  gear state, not a new Map.
- **Navigation**: Gear info card tap, completion card tap, and "Select Recipient"
  panel action all use `NavigationHelpers` methods.
- **Computed getters**: `isGiveawayConversation`, `giveawayPhase`,
  `isGiveawayOwner`, `hasCurrentUserExpressedInterest` are computed getters on
  ViewModel or `ConversationState`, derived from `GearTransferContext`.
- **Minimal server changes**: One new system message type (`GIVEAWAY_SHARED`)
  + two new fields on an existing proto message. No new RPCs.
- **Server service independence**: System message emission in `ShareGear` uses
  the `systemMessageWriter` interface — no new service-to-service dependency.
- **Proto separation**: New message type in `models/chat.proto`; new
  `GearTransferContext` fields in `api/transfer.proto`. No cross-package imports.
- **Structured logging**: New server-side emission uses `logging.
  LoggerWithContext(ctx)` with `operation`, `gear_id`, `community_id` fields.
- **Widget reuse**: Completion card adapts `ImpactMetricsCard` from
  `GiveawayGiverModal`. Badge widget reuses experience/request badge pattern.
- **File size**: New widgets are separate files (< 1000 lines each).
- **CachedMediaImage**: Gear image in info card uses `CachedMediaImage` with
  stable cache key from `MediaUrl.cacheKey`, never `Image.network`.

### Risk: GearTransferContext server population

The `GearTransferContext` is populated by the chat context service. Adding
`selected_recipient` and `overall_phase` requires the context service to query
the active transfer state. This is an additional DB read per conversation
context load. Profile to ensure it doesn't regress conversation list
performance.

**Mitigation**: Add the fields to the existing `GearTransferContext` population
query (a single additional transfer lookup) rather than a separate RPC.

### Risk: CommunityService doesn't have systemMessageWriter

If `CommunityService` was not built with the `systemMessageWriter` interface,
injecting it is a constructor change that also requires updating the server
assembly in `main.go`.

**Mitigation**: Verify first. If the interface is absent, follow the same
injection pattern as `RequestService` — no new abstraction needed.

### Risk: conversation_view_model.dart growth

The ViewModel already handles experience, request, and gear (loan) workflows.
Giveaway adds another set of methods. Evaluate after Phase 2 whether to extract
a `GiveawayWorkflowMixin` or split the ViewModel.

**Mitigation**: Giveaway actions (`expressInterest`, `completeTransfer`,
`cancelTransfer`, `withdrawInterest`) are simpler than experience actions
(no RSVP voting, time proposals). The growth should be modest.

### Risk: MessageInputBar growing complexity

`MessageInputBar` already branches on `isExperienceConversation` and
`isRequestConversation`. Adding `isGiveawayConversation` introduces a third
workflow-specific layout path. The file may approach the 1000-line guideline.

**Mitigation**: After Phase 2, measure `MessageInputBar` line count. If it
exceeds ~800 lines, extract phase-specific builders (progress bar, status row,
manage panel trigger) into separate widget classes — e.g.,
`GiveawayStatusBar`, `RequestStatusBar` — and have `MessageInputBar` delegate
to them.

---

**Last Updated:** 2026-03-07 — Phase 9 complete
