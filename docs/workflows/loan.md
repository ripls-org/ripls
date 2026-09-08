---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: End-to-end loan workflow — temporary gear lending through the full five-state transfer machine (interest, recipient selection, active loan, return), handoff/return handling, and perpetual gear conversations.
  globs: [server/services/transfer/**, server/services/gear/**, app/lib/data/repositories/transfer_repository.dart, app/lib/data/repositories/gear_repository.dart, app/lib/presentation/screens/gear/**, app/lib/presentation/viewmodels/gear_sharing_view_model.dart]
  triggers: [loan, lending, transfer, gear, recipient-selection, start-loan, return, interest, state-machine]
  lens: [workflow, domain]
freshness:
  verified_commit: "3be6b87fc"
  verified_on: "2026-07-21"
---
# Loan Workflow

This document explains the architecture, operations, state transitions, and workflow logic for gear lending in the Ripls application. Loans are temporary transfers where the item will be returned to the owner.

## Architecture Overview

The loan workflow enables gear owners to temporarily lend items to other community members, with a structured handoff and return process.

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
  - `state_machine.go`: State transition validation and gear state updates
  - `lifecycle.go`: SelectRecipient, StartLoan, CompleteTransfer, CancelTransfer operations
  - `interest.go`: ExpressInterest operation
  - `queries.go`: Listing and retrieval operations

**Client (Flutter):**

- `app/lib/data/repositories/gear_repository.dart`: Cached gear data access
- `app/lib/data/repositories/transfer_repository.dart`: Cached transfer data access
- `app/lib/presentation/viewmodels/gear_sharing_view_model.dart`: Sharing/transfer UI logic
- `app/lib/presentation/screens/gear/`: Gear UI screens

## Loan States

Loans follow a strict state machine with five states:

```protobuf
enum TransferState {
    TRANSFER_STATE_UNSPECIFIED = 0;
    TRANSFER_STATE_INTEREST_EXPRESSED = 1;  // Borrower has shown interest
    TRANSFER_STATE_RECIPIENT_SELECTED = 2;  // Owner approved the loan request
    TRANSFER_STATE_ACTIVE = 3;              // Item handed over, loan in progress
    TRANSFER_STATE_COMPLETED = 4;           // Loan completed (item returned)
    TRANSFER_STATE_CANCELLED = 5;           // Loan cancelled
}
```

### Valid State Transitions

The state machine is enforced in `server/services/transfer/state_machine.go`:

**From INTEREST_EXPRESSED:**

- → RECIPIENT_SELECTED (owner only, via SelectRecipient)
- → CANCELLED (either party can cancel, or borrower can withdraw via `WithdrawInterest`)

**From RECIPIENT_SELECTED:**

- → ACTIVE (owner only, via StartLoan)
- → CANCELLED (either party can cancel)

**From ACTIVE:**

- → COMPLETED (either party, via CompleteTransfer - item returned)
- → CANCELLED (either party, via CancelTransfer)

**From COMPLETED:**

- No transitions allowed (terminal state)

**From CANCELLED:**

- No transitions allowed (terminal state)

### Gear State Side Effects

When loan state changes, gear state is automatically updated (`state_machine.go`):

- **Loan → ACTIVE**: Gear becomes `GEAR_STATE_UNAVAILABLE`
- **Loan → COMPLETED** (from ACTIVE): Gear becomes `GEAR_STATE_AVAILABLE` (item returned to owner)
- **Loan → CANCELLED** (from ACTIVE): Gear becomes `GEAR_STATE_AVAILABLE` (loan cancelled, item returned)

### Multi-Community Sharing and Global Gear State

**Important**: Gear state is **global across all communities**, not per-community. This is intentional design because physical items can only be in one place at a time.

**How it works:**

- A gear item can be shared with multiple communities (via `CommunityGear` junction table)
- Each community has its own `CommunityGear` record with community-specific availability (`FOR_LOAN` or `FOR_GIVEAWAY`)
- Each community has its own perpetual conversation for the gear
- **However**, the gear's `state` field (`AVAILABLE`/`UNAVAILABLE`/`GIVEN_AWAY`) is global

**Implications:**

- If a gear item is loaned out in Community A, it automatically shows as `UNAVAILABLE` in Community B
- Community B members can see that the item is unavailable, but they cannot see who borrowed it or which community it's loaned to (for privacy)
- When the loan completes in Community A, the gear becomes `AVAILABLE` again in all communities
- This prevents double-booking of physical items across communities

**Example scenario:**

```
1. Alice shares her drill with both "Tech Tools" and "Home Improvement" communities
2. Bob (from Tech Tools) borrows the drill → Gear state becomes UNAVAILABLE globally
3. Carol (from Home Improvement) can see the drill but it shows as UNAVAILABLE
4. Carol cannot tell that Bob borrowed it or that it's loaned to Tech Tools
5. When Bob returns the drill, it becomes AVAILABLE in both communities again
```

This design ensures physical items cannot be double-booked while maintaining privacy about cross-community activity.

## Conversation Model

Loans use a community-wide perpetual conversation model where all discussion happens in a single shared conversation per gear item:

**Conversation Flow:**

1. The perpetual gear conversation is created when the gear item is created — `SaveGear` provisions the gear's per-item community and shares the gear into it (#2492)
2. The gear owner is automatically added as the initial participant
3. When anyone expresses interest in borrowing, they are added to the shared gear conversation
4. All community members can view and participate in the conversation (public discussion about the item)
5. System messages automatically track key events (interest expressed, loan approved, loan started, loan completed)
6. The conversation persists across all loan cycles - it's not tied to individual transfers

**Conversation Storage:**

- Conversation ID is stored in `Gear.conversation_id` field (migrated off the deprecated `CommunityGear.conversation_id`)
- Conversations reference gear via `ChatConversation.topic` — a `ConversationTopic` oneof set to `gear_id` (not `transfer_id`)
- The same conversation is used for all loans of this gear in this community
- Creates a perpetual discussion space about the gear item itself

**System Messages:**
System messages are automatically posted for major state transitions:

- **Interest expressed**: "[User] requested to borrow" (using `RequestedToBorrowText()`)
- **Loan approved**: "[User] was selected as the recipient" (using `ApprovedText()`)
- **Loan started**: "The loan has started" (using `StartedText()`)
- **Loan completed**: "This transfer has been completed" (using `CompletedText()`)
- **Loan cancelled**: "This request was cancelled" (using `CancelledText()`)

**Transfer Status Badges:**
In the conversation screen (NOT in the transfer management modal), users see status badges next to participant names based on their transfer state:

- **Requested** (blue): User has expressed interest (INTEREST_EXPRESSED)
- **Selected** (green): Owner has selected the user as recipient (RECIPIENT_SELECTED)
- **Borrowing** (purple): Loan is active (ACTIVE state)
- **Completed** (grey): Loan finished (COMPLETED or CANCELLED)

These badges are populated from the `GearTransferContext` included in conversation metadata and only appear in conversation screens, not in the transfer management modal.

**Key Benefits:**

- Community members can ask questions about gear without expressing interest first
- Past borrower experiences are visible to future borrowers (social proof)
- Transparent - everyone can see who's interested and the item's history
- Gear becomes a conversation starter for building community connections
- Owner can answer questions once for all interested parties

## User Operations

### For Gear Owners (Lenders)

**1. Share Gear for Loan**

- **Location**: `CommunityService.ShareItem` (server) via `CommunityService.ShareGearToCommunity`, `GearSharingViewModel.setCommunityAvailability` (app)
- **Options**: Availability is **item-wide**, not per-community (#2492/#2687). Gear is
  born `AVAILABILITY_FOR_LOAN` unless `SaveGearRequest.availability` says otherwise at
  creation, and `CommunityService.SetGearAvailability` changes it everywhere at once.
  `ShareItem` inherits whatever the gear already carries.
- **Effect**: Creates `CommunityGear` relationship with the gear's loan availability

**2. View Loan Requests**

- **Location**: `TransferService.ListMyTransfers` (server), `TransferRepository.listMyTransfers` (app)
- **Shows**: All transfers where user is owner
- **Filtering**: By transfer type (LOAN), state

**3. Select Borrower (Approve)**

- **Location**: `TransferService.SelectRecipient` (server), `TransferRepository.selectRecipient` (app)
- **State**: INTEREST_EXPRESSED → RECIPIENT_SELECTED
- **Validation**: Owner only
- **Side Effects**: Posts "approved" system message to gear conversation
- **Implementation**: `lifecycle.go:24-150`

**4. Start Loan**

- **Location**: `TransferService.StartLoan` (server), `TransferRepository.startLoan` (app)
- **State**: RECIPIENT_SELECTED → ACTIVE
- **Effect**: Gear becomes `GEAR_STATE_UNAVAILABLE`
- **Validation**: Owner only
- **Implementation**: `lifecycle.go:221-322`

**5. Complete Loan (Item Returned)**

- **Location**: `TransferService.CompleteTransfer` (server), `TransferRepository.completeTransfer` (app)
- **State**: ACTIVE → COMPLETED
- **Effect**: Gear becomes `GEAR_STATE_AVAILABLE`
- **Validation**: Either owner or borrower can mark complete
- **Implementation**: `lifecycle.go`

**6. Cancel Loan**

- **Location**: `TransferService.CancelTransfer` (server), `TransferRepository.cancelTransfer` (app)
- **State**: Any non-terminal → CANCELLED
- **Validation**: Either party can cancel
- **Implementation**: `lifecycle.go`

**7. Switch to Giveaway (Change Sharing Mode)**

- **Location**: `CommunityService.SetGearAvailability` (server), `GearRepository.setGearAvailability` / `GearNotifier.setAvailability` (app)
- **Effect**: Flips the item's availability (loan vs giveaway) across every community it's shared with in one call — unlike `UpdateGearSharing`, which only changes one community's setting
- **Validation**: Owner only; rejected (`FailedPrecondition`) while the gear has any in-progress transfer (interest expressed, recipient selected, or an active loan)
- **Side Effects**: Writes the giveaway-shared system message once to the gear's conversation; emits no `CommunityEvent` (a re-mode of an already-shared item shouldn't trigger a "shared an item" push)
- **UI**: Lend/Give segmented control in the gear edit pane (`GearEditPane`)

### For Borrowers

**1. Browse Available Gear**

- **Location**: `CommunityService.ListCommunityGear` (server)
- **Shows**: Gear shared with communities user belongs to
- **Filtering**: By community, availability (FOR_LOAN)

**2. Express Interest**

- **Location**: `TransferService.ExpressInterest` (server), `TransferRepository.expressInterest` (app)
- **Effect**:
  - Creates new transfer in INTEREST_EXPRESSED state
  - Adds user to existing gear conversation (shared with owner and all other interested borrowers)
  - Posts "requested to borrow" system message to gear conversation
- **Returns**: Full Transfer object with conversation ID
- **Validation**:
  - Cannot express interest in own gear
  - Gear must be in AVAILABLE state
  - User must be member of community where gear is shared
- **Implementation**: `interest.go:21-130`

**3. View My Loan Requests**

- **Location**: `TransferService.ListReceivedTransfers` (server), `TransferRepository.listReceivedTransfers` (app)
- **Shows**: All transfers where user is recipient
- **Filtering**: By transfer type (LOAN), state

**4. Withdraw Interest**

- **Location**: `TransferService.WithdrawInterest` (server), `TransferRepository.withdrawInterest` (app)
- **State**: INTEREST_EXPRESSED → CANCELLED
- **Effect**:
  - Cancels the borrower's loan request
  - User remains in the gear conversation (perpetual community discussion)
  - Records `COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_WITHDRAWN` event
  - **UI**: Transfer management modal automatically closes after successful withdrawal
- **Validation**:
  - User must be the recipient of the transfer
  - Transfer must be in INTEREST_EXPRESSED state
- **Implementation**: `interest.go`
- **Note**: Unlike leaving a conversation, withdrawal only cancels the transfer - users stay in the conversation for future interactions

**5. Cancel Request**

- **Location**: `TransferService.CancelTransfer` (server), `TransferRepository.cancelTransfer` (app)
- **State**: Any non-terminal → CANCELLED

**6. Return Item (Complete Loan)**

- **Location**: `TransferService.CompleteTransfer` (server), `TransferRepository.completeTransfer` (app)
- **State**: ACTIVE → COMPLETED
- **Effect**: Gear becomes available again

## Workflow Examples

These examples describe complete user journeys that should be covered by integration tests. See [update_system_tests.md](../testing/update_system_tests.md) for test implementation guidelines.

### 1. Sharing and Borrowing Gear (Happy Path)

#### Preconditions

- A test community exists
- Three users exist: an owner, borrower A, and borrower B, all members of the community
- The owner has gear that is not yet shared

#### Steps

1. **Owner**: Shares gear with community → `AVAILABILITY_FOR_LOAN`
2. **System**: Creates perpetual conversation for the gear, adds owner as participant
3. **Borrower A**: Sends message to gear conversation asking about the item
4. **Owner**: Responds with a message in the conversation
5. **Borrower A**: Expresses interest → Transfer A auto-approved to `RECIPIENT_SELECTED` state
6. **System**: Posts "[Borrower A] requested to borrow" system message
7. **Borrower B**: Also expresses interest → Transfer B auto-approved to `RECIPIENT_SELECTED` state
8. **System**: Posts "[Borrower B] requested to borrow" system message
9. **Owner**: Opens gear conversation, sees both borrowers listed
10. **Owner**: Starts loan with Borrower A → Transfer A moves to `ACTIVE`, gear becomes `UNAVAILABLE`
11. **Borrower A**: Returns item, marks loan complete → Transfer A moves to `COMPLETED`

#### Postconditions

- The gear is still shared with the community
- The gear is available (`GEAR_STATE_AVAILABLE`) for the next person to borrow
- Transfer B remains in `RECIPIENT_SELECTED` state (auto-approved, not affected by Transfer A's completion)
- The conversation contains system messages for each state transition
- **Notifications sent:**
  - When the owner shares the gear with the community: every other community member receives a "New Gear Shared" notification (`GEAR_SHARED`, gated by the per-community `notify_gear_shared` toggle).
  - When the owner starts the loan with Borrower A: Borrower A receives a "Loan Started" notification (`TRANSFER_ACTIVE`, gated by `notify_transfer_updates`).
  - The auto-approval (`RECIPIENT_SELECTED`) self-notification is suppressed because actor == recipient.
- **Impact estimation:**
  - CompleteTransfer response includes ImpactEstimate with money_saved (mean > 0, stddev > 0), emissions_prevented (manufacture_avoided_carbon + waste_reduced_carbon, both with mean > 0), and time_saved (mean > 0, stddev > 0)
  - Gear stats cumulative impact equals the single transfer's impact (timesLoaned=1): money_saved, carbon, and time means match within 1%
  - Community metrics cost_savings_usd, carbon_savings_grams, and time_banked_minutes all have positive means matching the transfer's impact
  - User savings for Borrower A are populated (cost_saved_usd > 0, co2_saved_kg > 0, time_saved_hours > 0)

### 2. Borrower Withdraws Interest

#### Preconditions

- A test community exists
- Two users exist: an owner and a borrower
- The owner has gear shared for loan with the community
- The borrower has expressed interest in the gear

#### Steps

1. **Borrower**: Withdraws interest via `WithdrawInterest`
2. **System**: Transfer moves to `CANCELLED`, posts withdrawal system message

#### Postconditions

- The transfer is in `CANCELLED` state
- The borrower remains in the gear conversation
- The gear remains available (`GEAR_STATE_AVAILABLE`) for other requests
- **Notifications sent:**
  - Owner receives an "Interest withdrawn" notification (`TRANSFER_INTEREST_WITHDRAWN`, gated by `notify_transfer_updates`).
- **Impact estimation:** Gear stats impact is zero (no completed loans). Community impact metrics savings are zero.

### 3. Owner Declines Request

#### Preconditions

- A test community exists
- Two users exist: an owner and a borrower
- The owner has gear shared for loan with the community
- The borrower has expressed interest in the gear

#### Steps

1. **Owner**: Reviews request, decides not to lend
2. **Owner**: Cancels transfer → Transfer moves to `CANCELLED`
3. **System**: Posts cancellation system message

#### Postconditions

- The transfer is in `CANCELLED` state
- The borrower remains in the conversation
- **CRITICAL REGRESSION TEST**: The gear remains available (`GEAR_STATE_AVAILABLE`) for other requests. Verify this explicitly to ensure cancellation doesn't incorrectly affect gear state.
- **Notifications sent:**
  - Borrower received "Loan Cancelled" notification (owner cancelled the transfer)
- **Impact estimation:** Gear stats impact is zero (no completed loans). Community impact metrics savings are zero.

### 4. Loan Cancelled After Approval

#### Preconditions

- A test community exists
- Two users exist: an owner and a borrower
- The owner has gear shared for loan
- A loan has been approved (transfer in `RECIPIENT_SELECTED` state)

#### Steps

1. **Either party**: Cancels the transfer
2. **System**: Transfer moves to `CANCELLED`

#### Postconditions

- The transfer is in `CANCELLED` state
- The gear remains available (`GEAR_STATE_AVAILABLE`)
- **Notifications sent:**
  - The other party receives a "Loan Cancelled" notification when transfer is cancelled (`TRANSFER_CANCELLED`, gated by `notify_transfer_updates`).
  - For auto-approved loans the borrower also received an earlier "Loan Started" notification when interest was first expressed (`TRANSFER_ACTIVE`).
- **Impact estimation:** Gear stats impact is zero (no completed loans). Community impact metrics savings are zero.

### 5. Multiple Completed Loans (Cumulative Impact)

#### Preconditions

- A test community exists
- Three users exist: an owner, borrower A, and borrower B, all members of the community
- The owner has gear shared for loan with the community (gear has value, material, and weight metadata)

#### Steps

1. **Borrower A**: Expresses interest → Transfer A auto-approved (`RECIPIENT_SELECTED`)
2. **Owner**: Starts loan A → Transfer A `ACTIVE`, gear `UNAVAILABLE`
3. **Borrower A**: Returns item → Transfer A `COMPLETED`, gear `AVAILABLE`
4. **Borrower B**: Expresses interest → Transfer B auto-approved (`RECIPIENT_SELECTED`)
5. **Owner**: Starts loan B → Transfer B `ACTIVE`, gear `UNAVAILABLE`
6. **Borrower B**: Returns item → Transfer B `COMPLETED`, gear `AVAILABLE`

#### Postconditions

- Transfer A and Transfer B are both in `COMPLETED` state
- The gear is available (`GEAR_STATE_AVAILABLE`)
- **Notifications sent:**
  - When the owner shares the gear: each member except the owner receives a "New Gear Shared" notification (`GEAR_SHARED`).
  - For each loan cycle: the borrower receives a "Loan Started" notification (`TRANSFER_ACTIVE`) when the owner starts the loan.
- **Impact estimation (cumulative scaling):**
  - CompleteTransfer A and B each return ImpactEstimate with identical values (same gear, same config)
  - Gear stats cumulative impact = 2× single transfer impact (timesLoaned=2)
  - Community metrics cost_savings_usd, carbon_savings_grams, time_banked_minutes each equal the sum of Transfer A + Transfer B impacts
  - Owner has savings from both loans
  - Each borrower has savings from their respective loan only

## Data Flow

### Server → Client

**Transfer Data:**

- Server: `TransferService` (state machine operations)
- Client: `TransferRepository` (cached access)
- Caching:
  - `'my:*'` - User's loans as lender
  - `'received:*'` - User's loans as borrower
  - `'{transferId}'` - Single transfer by ID

**Cache Invalidation:**

- Repositories automatically invalidate relevant caches after mutations
- Pattern: `await repository.operation(); await repository.refresh();`
- Single-item cache enables efficient real-time updates from push notifications

### Client State Management

**GearSharingViewModel** manages:

- Gear details and ownership
- Loan request lists (sent/received)
- Community sharing configuration
- Loading and error states

**Key ViewModel Operations:**

- `initialize`: Loads gear and user-specific transfer data
- `expressInterest`: Creates loan request, sends initial message
- `selectRecipient`: Approves loan request (owner only)
- `cancelTransferRequest`: Cancels active request
- `loadUserCommunities`: Loads communities for sharing management

## Business Rules

### Validation Rules

1. **Gear Ownership**: Only owners can share, update, or delete their gear
2. **Self-Interest**: Users cannot express interest in their own gear
3. **Multiple Concurrent Requests**: Multiple borrowers can request the same item
4. **Owner Controls Selection**: Only the owner can select which borrower to approve
5. **State Transitions**: Enforced by state machine
6. **Actor Authorization**: Only owner or borrower can modify a transfer
7. **Role-Based Actions**:
   - Only owner can select recipient
   - Only owner can start loan
   - Either party can complete or cancel
8. **UI Button Visibility**: In the transfer management modal, action buttons (including chat) are only visible to the owner and the requestor (recipient/interested party), not to other viewers. This ensures only parties directly involved in the transfer can take actions or communicate about it.

### Side Effects

1. **Gear State Updates**: Automatic when transfer state changes (ACTIVE makes unavailable, COMPLETED/CANCELLED from ACTIVE makes available)
2. **Conversation Creation**: Automatic at gear creation (`SaveGear` provisions the per-item community and shares the gear in, #2492)
3. **Conversation Archiving**: Implicit when item is done (COMPLETED/CANCELLED) and no unread messages
4. **Notifications**: See [Notifications](#notifications) section below
5. **Community Events**: Recorded for activity feed

### Notifications

Push notifications are sent for specific loan events to keep users informed. See [push_notifications.md](../push_notifications.md) for architecture details.

All notifications are gated per recipient by their per-(user, community) preferences row (see [push_notifications.md](../push_notifications.md)). Unset toggles resolve to "on", so default behavior is to notify.

| Event                      | Category                  | Recipient                   | Notification Content                                                       |
| -------------------------- | ------------------------- | --------------------------- | -------------------------------------------------------------------------- |
| Gear shared                | `notify_gear_shared`      | All community members       | "New Gear Shared" / "[user] shared [gear name]"                            |
| Interest expressed         | `notify_transfer_updates` | Gear owner                  | "Interest in [gear name]" / "[user] wants to borrow your [gear]"           |
| Interest withdrawn         | `notify_transfer_updates` | Gear owner                  | "Interest withdrawn for [gear name]" / "[user] no longer wants to borrow your [gear]" |
| Recipient selected         | `notify_transfer_updates` | Selected recipient          | "Request Approved" / "Your request for [gear name] was approved"           |
| Loan started (ACTIVE)      | `notify_transfer_updates` | Other party                 | "Loan Started" / "Your loan of [gear] is now active"                       |
| Loan cancelled             | `notify_transfer_updates` | Other party                 | "Loan Cancelled" / "The loan of [gear] was cancelled by [user]"            |
| Pickup proposed            | `notify_transfer_updates` | Other party                 | "Pickup Proposed" / "[user] proposed a pickup time for [gear]"             |

**Events that do NOT trigger notifications:**

- Loan completed - no notification currently sent

**Deep linking:** All notifications include `conversation_id` for direct navigation to the gear conversation.

## Error Handling

**Server:**

- Uses Connect-style error codes (NotFound, PermissionDenied, FailedPrecondition, InvalidArgument)
- Validates state transitions and returns descriptive errors
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
- Interest expression tests (`interest_test.go`)
- Query tests (`queries_test.go`)

**Client:**

- Repository tests with mocked services
- ViewModel tests for business logic
- Widget tests for UI interactions

## Gear-Backed Request Offers (#2702)

A loan (or giveaway) can be born as the supply side of a community request:
`TransferService.OfferTransfer` (`server/services/transfer/offer.go`) creates
a transfer directly in `RECIPIENT_SELECTED` with the **requester** as the
recipient — the owner commits by offering, the requester already signaled by
asking. Such transfers carry the ask as their origin
(`Transfer.origin_request_id`) and couple the two lifecycles over the
community-event bus: the handoff (`StartLoan` → `ACTIVE`, or giveaway
completion) auto-fulfills a single-need request, a fulfilled request stands down its
open sibling offers, and cancelling the offer pre-handoff unwinds the claim it
rode on. Everything downstream of `RECIPIENT_SELECTED` — handoff, return,
reminders, the perpetual gear conversation (the requester is added as a
participant at offer time) — is the ordinary loan machinery described above.
See `docs/workflows/request.md` § Gear-backed offers for the request side.

## Gear Bookings (Calendar Reservations)

A **gear booking** is a loan Transfer carrying a reserved date window
(`estimated_pickup_unix_sec` .. `expected_return_unix_sec`) — bookings flow
through the existing loan/transfer model, not a parallel concept. The
`GearService` exposes the booking RPCs (implemented in
`server/services/gear/bookings.go`):

- **`ListGearBookings`**: the who-has-it-which-days schedule — every live loan
  transfer (`RECIPIENT_SELECTED`/`ACTIVE`) that carries a reserved date window,
  plus the selected recipient of a giveaway (so owner and recipient can
  coordinate the pickup hand-off).
- **`ClaimGearDays`**: claims an inclusive range of days for the caller, creating
  a loan Transfer in `RECIPIENT_SELECTED`. Adjacent/overlapping own-days merge
  into one transfer; overlap with another recipient is rejected. The owner can
  also reserve on someone's behalf, block days (recipient = owner), or mint a
  "pending" hold behind an accept-link token.
- **`AcceptGearBooking`**: claims a pending owner-created hold via its link
  token, assigning the accepter as the recipient.
- **`ReleaseGearBooking`**: cancels a booking the caller owns, while it's
  still `INTEREST_EXPRESSED` or `RECIPIENT_SELECTED`. Once the loan is
  `ACTIVE` the item is in the borrower's hands, so the RPC rejects the
  release (`FailedPrecondition`, `gear_booking_not_droppable`) — the loan
  must be completed (marked returned) instead (#2638).
- **`UpdateGearBookingHandoff`**: sets or clears the pickup / drop-off location
  (`Transfer.pickup_location_id` / `dropoff_location_id`) and hand-off times.

## Future Considerations

1. **Reminders**: No automated reminders for loan returns
2. **Ratings/Reviews**: No feedback system after loans
3. **Deposit/Insurance**: No security deposit or insurance mechanism
4. **Photo Evidence**: No photo verification of item condition
5. **Dispute Resolution**: No formal dispute mechanism
6. **Extension Requests**: No formal way to request loan extension
