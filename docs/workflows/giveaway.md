---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: End-to-end giveaway workflow — permanent gear transfer via a simplified transfer state machine (skips ACTIVE), group interest, owner recipient selection, ownership transfer, and conversation archival on completion.
  globs: [server/services/transfer/**, server/services/gear/**, app/lib/data/repositories/transfer_repository.dart, app/lib/data/repositories/gear_repository.dart, app/lib/presentation/screens/gear/**, app/lib/presentation/viewmodels/gear_sharing_view_model.dart]
  triggers: [giveaway, transfer, gear, ownership-transfer, recipient-selection, interest, state-machine]
  lens: [workflow, domain]
freshness:
  verified_commit: "3be6b87fc"
  verified_on: "2026-07-21"
---
# Giveaway Workflow

This document explains the architecture, operations, state transitions, and workflow logic for gear giveaways in the Ripls application. Giveaways allow gear owners to give items to other community members. 

## Architecture Overview

The giveaway workflow enables gear owners to permanently give items to other community members, with a group-based interest model where multiple users can express interest and the owner selects who receives the item.

### Core Components

**Protocol Definitions:**
- `proto/ripls/models/gear.proto`: Storage model for gear items
- `proto/ripls/models/transfer.proto`: Storage model for transfers
- `proto/ripls/api/gear_service.proto`: API types for gear operations
- `proto/ripls/api/transfer_service.proto`: API types for transfer operations
- `proto/ripls/api/transfer.proto`: API types Transfer, TransferType, TransferState
- `proto/ripls/api/gear.proto`: API types including Availability

**Server Services:**
- `server/services/gear/`: Gear service implementation
- `server/services/transfer/`: Transfer service implementation with state machine
  - `state_machine.go`: State transition validation and gear state management
  - `lifecycle.go`: SelectRecipient, CompleteTransfer, CancelTransfer operations
  - `interest.go`: ExpressInterest operation with group chat joining
  - `queries.go`: Listing and retrieval operations

**Client (Flutter):**
- `app/lib/data/repositories/gear_repository.dart`: Cached gear data access
- `app/lib/data/repositories/community_repository.dart`: Community and past giveaways data access
- `app/lib/data/repositories/transfer_repository.dart`: Cached transfer data access
- `app/lib/presentation/viewmodels/gear_sharing_view_model.dart`: Sharing/transfer UI logic
- `app/lib/presentation/screens/gear/`: Gear UI screens

## State Models

### Transfer States

Giveaways follow a simplified transfer state machine that skips the ACTIVE state:

```protobuf
enum TransferState {
    TRANSFER_STATE_UNSPECIFIED = 0;
    TRANSFER_STATE_INTEREST_EXPRESSED = 1;  // Users have shown interest
    TRANSFER_STATE_RECIPIENT_SELECTED = 2;  // Owner selected who gets the item
    TRANSFER_STATE_ACTIVE = 3;              // Not used for giveaways
    TRANSFER_STATE_COMPLETED = 4;           // Giveaway completed (item given)
    TRANSFER_STATE_CANCELLED = 5;           // Giveaway cancelled
}
```

### Valid State Transitions

The state machine is enforced in `server/services/transfer/state_machine.go`:

**From INTEREST_EXPRESSED:**
- → RECIPIENT_SELECTED (owner only, via SelectRecipient)
- → CANCELLED (owner can cancel the giveaway)
- Participants can withdraw via `WithdrawInterest` (removed from conversation, transfer remains active)

**From RECIPIENT_SELECTED:**
- → COMPLETED (complete giveaway via CompleteTransfer)
- → CANCELLED (cancel via CancelTransfer)
- Note: Cannot transition to ACTIVE (loans only)

**From COMPLETED:**
- No transitions allowed (terminal state)

**From CANCELLED:**
- No transitions allowed (terminal state)

### Gear States

Gear items have independent state tracking separate from transfer states:

```protobuf
enum GearState {
    GEAR_STATE_UNSPECIFIED = 0;
    GEAR_STATE_UNAVAILABLE = 1;    // Temporarily not available (e.g., loaned out)
    GEAR_STATE_AVAILABLE = 2;      // Can be shared/loaned/given away
    GEAR_STATE_GIVEN_AWAY = 4;     // Permanently given to another member
}
```

**Key State Characteristics:**

- **AVAILABLE**: Normal state, can be shared with communities for loans or giveaways
- **UNAVAILABLE**: Temporarily unavailable (e.g., actively loaned to someone)
- **GIVEN_AWAY**: Completed giveaway, remains in original owner's inventory for history

### Gear State Side Effects

When giveaway state changes, gear state is updated but **ownership does NOT transfer** (`state_machine.go`):

- **Transfer → COMPLETED** (from RECIPIENT_SELECTED):
  - Gear state changes to `GEAR_STATE_GIVEN_AWAY`
  - **Gear ownership remains with original owner** (gear.owner_id unchanged)
  - CommunityGear record is marked `archived = true` (not deleted)
  - Gear remains in original owner's inventory for historical tracking
  - Conversation remains open to all community members

### Multi-Community Sharing and Global Gear State

**Important**: Gear state is **global across all communities**, not per-community. This is intentional design because physical items can only be in one place at a time.

**How it works:**
- A gear item can be shared with multiple communities (via `CommunityGear` junction table)
- Each community has its own `CommunityGear` record with community-specific availability (`FOR_LOAN` or `FOR_GIVEAWAY`)
- Each community has its own perpetual conversation for the gear
- **However**, the gear's `state` field (`AVAILABLE`/`UNAVAILABLE`/`GIVEN_AWAY`) is global

**Implications:**
- If a gear item is given away in Community A, it automatically shows as `GIVEN_AWAY` in Community B
- Community B members can see that the item has been given away, but they cannot see who received it or which community it was given to (for privacy)
- Once gear is given away, it cannot be shared again (terminal state)
- This prevents users from accepting interest from multiple communities for the same physical item

**Example scenario:**
```
1. Alice shares her old bike with both "Bike Enthusiasts" and "Sustainable Living" communities
2. Bob (from Bike Enthusiasts) expresses interest, Alice selects him as recipient
3. Alice marks the giveaway as completed → Gear state becomes GIVEN_AWAY globally
4. Carol (from Sustainable Living) can see the bike was given away but cannot tell who got it
5. The gear remains in Alice's inventory for historical tracking but is no longer available
```

This design ensures physical items cannot be given to multiple people while maintaining privacy about cross-community activity.

## Conversation Model

Giveaways use a community-wide perpetual conversation model where all discussion happens in a single shared conversation per gear item:

**Conversation Flow:**
1. The perpetual gear conversation is created when the gear item is created — `SaveGear` provisions the gear's per-item community and shares the gear into it (#2492)
2. The gear owner is automatically added as the initial participant
3. When anyone expresses interest in the giveaway, they are added to the shared gear conversation
4. All community members can view and participate in the conversation (public discussion about the item)
5. System messages automatically track key events (interest joined, interest withdrawn, recipient selected, giveaway completed)
6. **After giveaway completes:** Conversation is archived but remains fully accessible

**Conversation Storage:**
- Conversation ID is stored in `Gear.conversation_id` field (migrated off the deprecated `CommunityGear.conversation_id`)
- Conversations reference gear via `ChatConversation.topic` — a `ConversationTopic` oneof set to `gear_id` (not `transfer_id`)
- The same conversation is used for all interested parties in this community
- After giveaway completion, CommunityGear is marked `archived = true` (not deleted)
- Archived conversations remain viewable and postable through "Past Giveaways" view

**Conversation Access:**
- All community members can read and post messages at any time
- This includes both before and after giveaway completion
- Conversations are never read-only - they remain fully interactive

**System Messages:**
System messages are automatically posted for major state transitions:
- **Interest expressed**: "[User] joined the conversation" (using `JoinedText()`)
- **Interest withdrawn**: "[User] left the conversation" (using `LeftText()`)
- **Recipient selected**: "[User] was selected as the recipient" (using `ApprovedText()`)
- **Giveaway completed**: "This transfer has been completed" (using `CompletedText()`)
- **Giveaway cancelled**: "This request was cancelled" (using `CancelledText()`)

**Transfer Status Badges:**
In the conversation screen (NOT in the transfer management modal), users see status badges next to participant names based on their transfer state:
- **Requested** (blue): User has expressed interest (INTEREST_EXPRESSED)
- **Selected** (green): Owner has selected this user as recipient (RECIPIENT_SELECTED)
- **Completed** (grey): Giveaway finished (COMPLETED or CANCELLED)

These badges are populated from the `GearTransferContext` included in conversation metadata and only appear in conversation screens, not in the transfer management modal.

**Key Benefits:**
- Community members can ask questions about giveaway items before claiming
- Transparent - everyone can see who's interested
- Owner can answer questions once for all interested parties
- Public discussion builds community engagement around generosity
- Conversation remains accessible for history even after item is given away

## User Operations

### For Gear Owners (Givers)

**1. Share Gear for Giveaway**
- **Location**: `CommunityService.ShareItem` (server) via `CommunityService.ShareGearToCommunity`, `GearSharingViewModel.setCommunityAvailability` (app)
- **Options**: Availability is **item-wide**, not per-community (#2492/#2687). Set
  `AVAILABILITY_FOR_GIVEAWAY` at creation via `SaveGearRequest.availability`, or later
  via `CommunityService.SetGearAvailability`, which applies across every community the
  gear is shared with. `ShareItem` inherits it — and the mode must be set **before** the
  share, since the `GEAR_SHARED` community event takes its "New giveaway" copy from it.
- **Effect**: Creates `CommunityGear` relationship with the gear's giveaway availability

**2. View Interested Users**
- **Location**: `TransferService.ListMyTransfers` (server), `TransferRepository.listMyTransfers` (app)
- **Shows**: All transfers where user is owner
- **Filtering**: By transfer type (GIVEAWAY), state
- **Note**: For giveaways, the `participant_count` field shows number of interested users

**3. Select Recipient**
- **Location**: `TransferService.SelectRecipient` (server), `TransferRepository.selectRecipient` (app)
- **State**: INTEREST_EXPRESSED → RECIPIENT_SELECTED
- **Validation**: Owner only
- **Side Effects**: Posts "approved" system message to gear conversation
- **Implementation**: `lifecycle.go:24-150`

**4. Confirm Pickup Time (Optional)**
- **Location**: `TransferService.UpdateTransfer` (server), `ArrangePickupModal` (app)
- **State**: RECIPIENT_SELECTED (no state change — sets `estimated_pickup_unix_sec` field)
- **Effect**: Sets the `estimated_pickup_unix_sec` field on the transfer to coordinate a pickup time.
- **Validation**: Either owner or selected recipient can set the pickup time.
- **UI**: The pickup time appears as a checklist step under the recipient header in both the owner menu (owner view) and the recipient's checklist menu (non-owner view). A green check icon shows once the pickup time is set.

**5. Complete Giveaway**
- **Location**: `TransferService.CompleteTransfer` (server), `TransferRepository.completeTransfer` (app)
- **State**: RECIPIENT_SELECTED → COMPLETED
- **Effect**:
  - **Gear ownership remains with original owner** (gear.owner_id unchanged)
  - Gear state changes to `GEAR_STATE_GIVEN_AWAY`
  - CommunityGear is marked `archived = true` (not deleted, viewable in "Past Giveaways")
  - **All other pending transfers for this gear are automatically cancelled** (INTEREST_EXPRESSED or RECIPIENT_SELECTED states)
  - Posts "completed" system message to gear conversation
- **Validation**: Either owner or recipient can complete
- **Implementation**: `lifecycle.go:325-458`, `state_machine.go:354-463` (`completeGiveaway`)

**6. Cancel Giveaway**
- **Location**: `TransferService.CancelTransfer` (server), `TransferRepository.cancelTransfer` (app)
- **State**: Any → CANCELLED
- **Validation**: Owner can cancel at any point
- **Side Effects**: Posts "cancelled" system message to gear conversation; button shows read-only "Cancelled" terminal state with no menu
- **Implementation**: `lifecycle.go:153-217`

**7. Switch to Loan (Change Sharing Mode)**
- **Location**: `CommunityService.SetGearAvailability` (server), `GearRepository.setGearAvailability` / `GearNotifier.setAvailability` (app)
- **Effect**: Flips the item's availability (loan vs giveaway) across every community it's shared with in one call — unlike `UpdateGearSharing`, which only changes one community's setting
- **Validation**: Owner only; rejected (`FailedPrecondition`) while the gear has any in-progress transfer (interest expressed, recipient selected, or an active loan)
- **Side Effects**: Writes the loan-shared system message once to the gear's conversation; emits no `CommunityEvent` (a re-mode of an already-shared item shouldn't trigger a "shared an item" push)
- **UI**: Lend/Give segmented control in the gear edit pane (`GearEditPane`)

### For Recipients

**1. Browse Available Gear**
- **Location**: `CommunityService.ListCommunityGear` (server)
- **Shows**: Gear shared with communities user belongs to
- **Filtering**: By community, availability (FOR_GIVEAWAY)

**2. Express Interest**
- **Location**: `TransferService.ExpressInterest` (server), `TransferRepository.expressInterest` (app)
- **Effect**:
  - Creates new transfer in INTEREST_EXPRESSED state (first user only) OR updates existing transfer (subsequent users)
  - Adds user to existing gear conversation (shared with owner and all other interested users)
  - Posts "[User] joined the conversation" system message to gear conversation
- **Returns**: Full Transfer object with conversation ID
- **Validation**:
  - Cannot express interest in own gear
  - Gear must be in AVAILABLE state
  - User must be member of community where gear is shared
  - Transfer must be in INTEREST_EXPRESSED state (cannot join after recipient selected)
- **Implementation**: `interest.go:21-130`, `interest.go:172-275` (`createNewTransfer`)

**3. View My Giveaway Interests**
- **Location**: `TransferService.ListReceivedTransfers` (server), `TransferRepository.listReceivedTransfers` (app)
- **Shows**: All transfers where user is recipient or participant
- **Filtering**: By transfer type (GIVEAWAY), state

**4. Withdraw Interest**
- **Location**: `TransferService.WithdrawInterest` (server), `TransferRepository.withdrawInterest` (app)
- **State**: Removes user from gear conversation (transfer remains in INTEREST_EXPRESSED for other users)
- **Effect**:
  - User is removed from conversation participant list
  - Posts "[User] left the conversation" system message
  - If user was the `recipient_id`, it is reassigned to another participant
  - If no participants remain, `recipient_id` is cleared but transfer stays active
  - Records `COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_WITHDRAWN` event
  - **UI**: Transfer management modal automatically closes after successful withdrawal
- **Validation**:
  - User must be a participant (not the owner)
  - Transfer must be in INTEREST_EXPRESSED state
- **Implementation**: `interest.go:498-549` (dispatch), `interest.go:631-733` (`withdrawFromGiveaway`)

**5. Complete Giveaway**
- **Location**: `TransferService.CompleteTransfer` (server), `TransferRepository.completeTransfer` (app)
- **State**: RECIPIENT_SELECTED → COMPLETED
- **Effect**:
  - Transfer marked as completed (recipient receives physical item)
  - Gear state changes to GIVEN_AWAY (but ownership stays with original owner)
  - CommunityGear marked as archived (conversation remains open to all)

## Workflow Examples

These examples describe complete user journeys that should be covered by integration tests. See [update_system_tests.md](../testing/update_system_tests.md) for test implementation guidelines.

### 1. Giving Away an Item (Happy Path)

#### Preconditions

- A test community exists
- Four users exist: an owner, user A, user B, and user C, all members of the community
- The owner has gear that is not yet shared

#### Steps

1. **Owner**: Shares gear with community → `AVAILABILITY_FOR_GIVEAWAY`
2. **System**: Creates perpetual conversation for the gear, adds owner as participant
3. **User A**: Expresses interest → Creates transfer in `INTEREST_EXPRESSED` state
4. **System**: Adds User A to gear conversation, posts join system message
5. **User B**: Expresses interest → Joins existing transfer
6. **System**: Adds User B to gear conversation, posts join system message
7. **User C**: Expresses interest → Joins existing transfer
8. **System**: Adds User C to gear conversation, posts join system message
9. **Owner**: Selects User B as recipient → Transfer moves to `RECIPIENT_SELECTED`
10. **Owner**: Completes giveaway → Transfer moves to `COMPLETED`

#### Postconditions

- The gear **remains owned by the original owner** (ownership does NOT transfer)
- The gear state is `GEAR_STATE_GIVEN_AWAY`
- The gear remains visible in original owner's inventory with GIVEN_AWAY badge
- CommunityGear is marked `archived = true` (viewable in "Past Giveaways")
- The transfer state is `COMPLETED`
- The conversation remains open to all community members for posting
- **User A and User C's transfers are automatically cancelled** when User B's giveaway completes
- User B receives the physical item but **NOT the gear record**
- If User B wants to share the item, they must create a new gear record
- **Notifications sent:**
  - When the owner shares the gear with the community: each member except the owner receives a "New Gear Shared" notification (`GEAR_SHARED`, gated by `notify_gear_shared`).
  - Owner received "Interest in [gear]" notifications when users A, B, and C each expressed interest (`TRANSFER_INTEREST_EXPRESSED`, gated by `notify_transfer_updates`).
  - User B received a "Request Approved" notification when selected as recipient (`TRANSFER_RECIPIENT_SELECTED`).
  - User B received a "Pickup Proposed" notification when the pickup time was proposed (`TRANSFER_PICKUP_PROPOSED`).
- **Impact estimation:**
  - CompleteTransfer response includes ImpactEstimate with money_saved (mean > 0), emissions_prevented (manufacture_avoided_carbon + waste_reduced_carbon), and time_saved (mean > 0)
  - Community metrics cost_savings_usd, carbon_savings_grams, and time_banked_minutes all have positive means matching the transfer's impact

### 2. Owner Cancels Giveaway

#### Preconditions

- A test community exists
- Three users exist: an owner, user A, and user B
- The owner has gear shared for giveaway with the community
- User A and User B have expressed interest

#### Steps

1. **Owner**: Decides not to give away the item
2. **Owner**: Cancels transfer → Transfer moves to `CANCELLED`
3. **System**: Posts cancellation system message

#### Postconditions

- The transfer is in `CANCELLED` state
- The gear remains owned by the original owner
- The gear state remains `GEAR_STATE_AVAILABLE`
- The gear can be re-shared or given away again
- The conversation remains accessible for history
- CommunityGear is NOT archived (giveaway didn't complete)
- **Notifications sent:**
  - Recipient received "Giveaway Cancelled" notification (owner cancelled the transfer)
- **Impact estimation:** Community impact metrics savings are zero (no completed giveaways).

### 3. User Withdraws Interest

#### Preconditions

- A test community exists
- Four users exist: an owner, user A, user B, and user C
- The owner has gear shared for giveaway
- User A, B, and C have all expressed interest

#### Steps

1. **User A**: Changes mind, withdraws interest via `WithdrawInterest`
2. **System**: Removes User A from conversation, posts withdrawal system message

#### Postconditions

- User A is removed from the conversation
- User B and User C remain in the conversation with "Requested" badges
- The transfer remains in `INTEREST_EXPRESSED` state
- If User A was the `recipient_id`, it is reassigned to another participant
- **Notifications sent:**
  - Owner received "Interest withdrawn" notification when User A withdrew (`TRANSFER_INTEREST_WITHDRAWN`, gated by `notify_transfer_updates`), in addition to the earlier interest notifications from each user.
- **Impact estimation:** Community impact metrics savings are zero (no completed giveaways).

### 4. Selected Recipient Cancels — Giveaway Reverts to Open

#### Preconditions

- A test community exists
- Three users exist: an owner, User B, and User C
- The owner has gear shared for giveaway with the community
- User B and User C have both expressed interest
- The owner has selected User B as the recipient (User B's transfer is `RECIPIENT_SELECTED`)

#### Steps

1. **User B**: Changes mind, cancels their transfer via `CancelTransfer`
2. **System**: Posts cancellation system message

#### Postconditions

- User B's transfer is in `CANCELLED` state
- User C's transfer remains in `INTEREST_EXPRESSED` state
- The `CommunityGear` row is **NOT** archived — the gear remains visible in the community feed
- The gear state remains `GEAR_STATE_AVAILABLE`
- The overall giveaway phase reverts to `GIVEAWAY_PHASE_OPEN` for User C and the owner
- The owner has `TRANSFER_ACTION_SELECT_RECIPIENT` as an available action
- **Notifications sent:**
  - Owner receives a "Giveaway Cancelled" notification (recipient cancelled the transfer)
- **Impact estimation:** Community impact metrics savings are zero (no completed giveaways).

## Gear State After Giveaway Completion

**Important:** Gear ownership does NOT transfer to the recipient. The gear record remains with the original owner for historical tracking and community attribution.

When a giveaway completes, the `completeGiveaway` function (`state_machine.go`) performs:

1. **NO Ownership Change**: `gear.owner_id` remains unchanged (stays with original owner)
2. **State Change**: Sets gear state to `GEAR_STATE_GIVEN_AWAY`
3. **Community Archiving**: Marks CommunityGear as `archived = true` (does not delete)
4. **Inventory Display**: Gear remains visible in original owner's inventory with GIVEN_AWAY badge

### GIVEN_AWAY State

The `GEAR_STATE_GIVEN_AWAY` state serves several purposes:

- **Historical Attribution**: Giveaways remain credited to the person who gave, not who received
- **Community Transparency**: Members can see past generosity and community activity
- **Owner's Record**: Given-away items remain in the giver's inventory permanently
- **Semantic Clarity**: Distinguishes permanent giveaway from temporary unavailability

**Key Differences from UNAVAILABLE:**
- UNAVAILABLE: Temporarily not available (e.g., currently loaned out)
- GIVEN_AWAY: Permanently given to another member, remains in giver's history

### What Happens to the Recipient?

The recipient receives the physical item but **does not** receive the gear record:

- The gear record stays with the original owner in GIVEN_AWAY state
- If the recipient wants to share the item with their communities, they must **create a new gear record**
- This keeps gear records independent and traceable to their original owners
- No ownership transfer complexity or edge cases to handle

### Past Giveaways View

Completed giveaways are viewable through the "Past Giveaways" filter/tab in community gear views:

- **Location**: Community gear listing screen
- **Access**: `CommunityService.ListCompletedGiveaways(communityId)`
- **Display**: Shows archived giveaways with original owner's name
- **Purpose**: Community transparency and social proof of generosity

**Default Behavior:**
- Regular gear listings exclude `archived = true` items
- "Past Giveaways" view includes only `archived = true` items
- Owner's inventory shows GIVEN_AWAY gear with special badge

## Data Flow

### Server → Client

**Transfer Data:**
- Server: `TransferService` (state machine operations)
- Client: `TransferRepository` (cached access)
- Caching:
  - `'my:*'` - User's giveaways as giver
  - `'received:*'` - User's giveaways as potential recipient
  - `'{transferId}'` - Single transfer by ID
  - `'gear:{gearId}'` - Transfers for a specific gear

**Cache Invalidation:**
- Repositories automatically invalidate relevant caches after mutations
- Pattern: `await repository.operation(); await repository.refresh();`
- Gear repository is also invalidated when ownership changes

### Client State Management

**GearSharingViewModel** manages:
- Gear details and ownership
- Giveaway interest lists
- Community sharing configuration
- Loading and error states
- Giveaway conversation summaries

**Key ViewModel Operations:**
- `initialize`: Loads gear and user-specific transfer data
- `expressInterest`: Joins giveaway interest group
- `selectRecipient`: Chooses who gets the item (owner only)
- `cancelTransferRequest`: Cancels giveaway

## Business Rules

### Validation Rules

1. **Gear Ownership**: Only owners can share, update, or delete their gear
2. **Self-Interest**: Users cannot express interest in their own gear
3. **Single Transfer Per Giveaway**: Only one transfer exists per giveaway (all users join the same one)
4. **Owner Controls Selection**: Only the owner can select which user receives the item
5. **State Transitions**: Enforced by state machine (no ACTIVE state for giveaways)
6. **Join Window**: Users can only join while transfer is in INTEREST_EXPRESSED state
7. **Post-Giveaway Sharing**: Recipient must explicitly re-share gear if they want it visible
8. **UI Button Visibility**: In the transfer management modal, action buttons (including chat) are only visible to the owner and the requestor (recipient/interested party), not to other viewers. This ensures only parties directly involved in the transfer can take actions or communicate about it.

### UI Differences from Loans

Giveaway transfer management modals have several UI differences compared to loans:

1. **No "Other Interested Users" Label**: In the recipient management view (after a recipient has been selected), giveaways do not show the "Other Interested Users" label above the list of pending requests. The pending requests are still displayed, but without the label, providing a cleaner UI for the multi-party giveaway model.

2. **Simplified State Flow**: Since giveaways skip the ACTIVE state and go directly from RECIPIENT_SELECTED to COMPLETED, the UI reflects this simpler workflow with fewer state transitions to display.

### Side Effects

1. **Gear State Change**: Gear marked as GIVEN_AWAY when giveaway completes (ownership remains with original owner)
2. **Community Archiving**: CommunityGear marked as archived when giveaway completes (not deleted, viewable in "Past Giveaways")
3. **Automatic Transfer Cancellation**: When a giveaway completes, all other pending transfers for that gear are automatically cancelled
4. **Conversation Creation**: Perpetual gear conversation created at gear creation (`SaveGear` provisions the per-item community and shares the gear in, #2492), not on first interest; all participants join the same conversation
5. **Open Conversations**: Conversations remain open to all community members at all times (no read-only restrictions)
6. **Notifications**: See [Notifications](#notifications) section below
7. **Community Events**: Recorded for activity feed

### Notifications

Push notifications are sent for specific giveaway events to keep users informed. See [push_notifications.md](../push_notifications.md) for architecture details.

All notifications are gated per recipient by their per-(user, community) preferences row (see [push_notifications.md](../push_notifications.md)). Unset toggles resolve to "on", so default behavior is to notify.

| Event              | Category                  | Recipient             | Notification Content                                                       |
| ------------------ | ------------------------- | --------------------- | -------------------------------------------------------------------------- |
| Gear shared        | `notify_gear_shared`      | All community members | "New Gear Shared" / "[user] shared [gear name]"                            |
| Interest expressed | `notify_transfer_updates` | Gear owner            | "Interest in [gear name]" / "[user] wants to borrow your [gear]"           |
| Interest withdrawn | `notify_transfer_updates` | Gear owner            | "Interest withdrawn for [gear name]" / "[user] no longer wants to borrow your [gear]" |
| Recipient selected | `notify_transfer_updates` | Selected recipient    | "Request Approved" / "Your request for [gear] was approved"                |
| Pickup proposed    | `notify_transfer_updates` | Other party           | "Pickup Proposed" / "[user] proposed a pickup time for [gear]"             |
| Giveaway cancelled | `notify_transfer_updates` | Other party           | "Giveaway Cancelled" / "The giveaway of [gear] was cancelled by [user]"    |

**Events that do NOT trigger notifications:**

- Giveaway completed - no notification currently sent
- Non-selected users when giveaway completes - no notification currently sent

**Deep linking:** All notifications include `conversation_id` for direct navigation to the gear conversation.

## Error Handling

**Server:**
- Uses Connect-style error codes (NotFound, PermissionDenied, FailedPrecondition, InvalidArgument)
- Validates state transitions and returns descriptive errors
- Handles missing conversations gracefully
- Logs all operations for debugging

**Client:**
- Wraps service exceptions in `ServiceException` with user-friendly messages
- ViewModels store error state for UI display
- Repository operations fail gracefully with cache invalidation

## Testing Strategy

**Server:**
- Unit tests for state machine logic (`state_machine.go`)
- Integration tests for lifecycle operations (`lifecycle_test.go`)
- Authorization tests (`authorization_test.go`)
- Interest expression tests including group chat joining (`interest_test.go`)
- Query tests (`queries_test.go`)

**Client:**
- Repository tests with mocked services
- ViewModel tests for business logic
- Widget tests for UI interactions

## Hand-off Coordination

Although a giveaway has no return (and therefore no reserved date window), the
selected recipient of a giveaway appears in the gear's "who's using it" schedule
(`GearService.ListGearBookings`, `server/services/gear/bookings.go`) so the owner
and recipient can coordinate the pickup hand-off. The owner or recipient can set
a pickup / drop-off location via `UpdateGearBookingHandoff`
(`Transfer.pickup_location_id` / `dropoff_location_id`). For a **completed**
giveaway, the recipient is exposed to every viewer of the gear (via
`GearTransferContext.SelectedRecipient`), so anyone reading the "who wants it"
card sees who it went to.

## Future Considerations

1. **Priority/Queue**: No first-come-first-served or lottery mechanism
2. **Ratings/Reviews**: No feedback system after giveaways
3. **Photo Evidence**: No photo verification of item handoff
4. **Dispute Resolution**: No formal dispute mechanism
5. **Recipient Criteria**: No way for owner to specify who qualifies
6. **Auto-Selection**: No random or automated recipient selection
7. **Expiration**: No automatic expiration of giveaway offers
