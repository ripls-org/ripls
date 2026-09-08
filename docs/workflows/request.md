---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: End-to-end request workflow — community members post needs and others offer to fulfill; covers the ACTIVE↔OFFERS_RECEIVED↔FULFILLED state machine with reversion, offer/conversation management, cross-community sharing, and AI generation.
  globs: [server/services/request/**, app/lib/data/repositories/request_repository.dart, app/lib/services/request_service.dart, app/lib/presentation/screens/request/**, app/lib/presentation/viewmodels/gen_request_view_model.dart]
  triggers: [request, offer, fulfill, offers-received, state-reversion, sharing, gen-request, need]
  lens: [workflow, domain]
freshness:
  verified_commit: "e78336400"
  verified_on: "2026-07-26"
---
# Request Workflow

This document explains the architecture, operations, state transitions, and workflow logic for community requests in the Ripls application. Requests allow community members to ask for items they need, while others can offer to fulfill those requests.

## Architecture Overview

The request workflow enables a community-driven model where users can express needs and community members can offer to help fulfill them.

### Core Components

**Protocol Definitions:**
- `proto/ripls/models/request.proto`: Storage model for requests
- `proto/ripls/api/request_service.proto`: API types for request operations
- `proto/ripls/api/request.proto`: API types Request and RequestState

**Server Services:**
- `server/services/request/`: Request service implementation
  - `lifecycle.go`: Request creation, updates, deletion, and state transitions
  - `offers.go`: Offer-to-fulfill logic and conversation management
  - `queries.go`: Request listing and retrieval operations
  - `sharing.go`: Cross-community sharing and unsharing operations
  - `gen.go`: AI-powered request generation
  - `stock_imagery.go`: Stock image fetching from a stock-imagery provider (Pexels/Unsplash)
  - `events.go`: Community event recording
  - `helpers.go`: Data transformation utilities

**Client (Flutter):**
- `app/lib/data/repositories/request_repository.dart`: Cached request data access
- `app/lib/services/request_service.dart`: Request API client
- `app/lib/presentation/screens/request/`: Request UI screens
- `app/lib/presentation/viewmodels/`: Request-related viewmodels

## Request States

Requests follow a linear state progression:

```protobuf
enum RequestState {
    REQUEST_STATE_UNSPECIFIED = 0;
    REQUEST_STATE_ACTIVE = 1;           // Publicly visible, awaiting offers
    REQUEST_STATE_OFFERS_RECEIVED = 2;  // One or more offers received
    REQUEST_STATE_FULFILLED = 4;        // Successfully fulfilled
    REQUEST_STATE_CANCELLED = 5;        // Cancelled by requester
}
```

**State Transitions:**
- New requests start as `ACTIVE` (set during creation in `lifecycle.go:70`)
- Becomes `OFFERS_RECEIVED` when first offer is made
- Moves to `FULFILLED` when requester marks it fulfilled — or automatically
  when a gear-backed offer's handoff covers a single-need request (see
  [Gear-backed offers](#gear-backed-offers-request--loan-2702))
- Moves to `CANCELLED` when requester cancels

## Request State Machine

### Valid State Transitions

**From ACTIVE:**
- → OFFERS_RECEIVED (when first offer is made, automatic)
- → CANCELLED (requester only)
- → FULFILLED (requester only)

**From OFFERS_RECEIVED:**
- → ACTIVE (automatic, when last offerer withdraws)
- → FULFILLED (requester only)
- → CANCELLED (requester only)

**From FULFILLED:**
- No transitions allowed (terminal state)

**From CANCELLED:**
- No transitions allowed (terminal state)

## User Operations

### For Requesters (Users Asking for Help)

**1. Generate AI-Powered Request**
- **Location**: `RequestService.StreamGenRequest` (server), `RequestRepository.streamGenRequest` (app)
- **Purpose**: Get AI suggestions for title, description, tags, and stock image
- **Notes**: Does NOT save to database; preview only
- **Returns**: `StreamGenRequestResponse` events with AI-generated suggestions

**2. Submit Request**
- **Location**: `RequestService.SubmitRequest` (server), `RequestRepository.submitRequest` (app)
- **Initial State**: `REQUEST_STATE_ACTIVE`
- **Per-item community (#2492)**: every request is born in its own host-only
  per-item community, provisioned via `community.ProvisionPerItemCommunity` and
  then shared into via `shareRequestToCommunity` (creating the conversation,
  CommunityRequest junction, `REQUEST_CREATED` anchor + description seed, and the
  community event). `SubmitRequestResponse.item_community_id` returns its id; the
  client opens the share/invite sheet against it.
- **Requirements**: `SubmitRequest` takes no community (#2529); the request is
  created in its own per-item community only. To also post it to existing
  communities, follow up with `CommunityService.ShareItem` (the uniform per-item
  sharing path).
- **Optional Fields**: `needed_by_unix_sec` sets the request's needed-by date at
  creation (a positive value schedules it for the Home calendar / Up-next agenda;
  omitted or zero leaves it undated)
- **Side Effects**:
  - Creates the request-scoped conversation (stored on `Request.conversation_id`)
    and adds the requester as participant
  - Creates a CommunityRequest junction record for the per-item community
  - Creates community event(s)
  - Asynchronously fetches a stock image if no media_id provided
  - Records in community feed
- **Implementation**: `lifecycle.go:38-253` (`community.ProvisionPerItemCommunity` + `shareRequestToCommunity` for the per-item community)

**3. Update Request**
- **Location**: `RequestService.UpdateRequest` (server), `RequestRepository.updateRequest` (app)
- **Validation**: Only requester can update
- **State Requirements**: Must be in ACTIVE or OFFERS_RECEIVED state
- **Updatable Fields**: Title, description, media_id, location_id, needed-by date (`needed_by_unix_sec`)
- **Notes**: Supports partial updates (only provided fields are modified). An
  explicit zero `needed_by_unix_sec` clears the needed-by date; an unset value
  leaves it unchanged.
- **Implementation**: `lifecycle.go:592-699`

**4. View My Requests**
- **Location**: `RequestService.ListMyRequests` (server), `RequestRepository.listMyRequests` (app)
- **Shows**: All requests created by the user
- **Filtering**: Optional by community
- **Implementation**: `queries.go:275-347`

**5. Mark Request Fulfilled**
- **Location**: `RequestService.MarkRequestFulfilled` (server), `RequestRepository.markRequestFulfilled` (app)
- **State**: ACTIVE/OFFERS_RECEIVED → FULFILLED
- **Validation**: Only requester can mark fulfilled
- **UI**: Available via action buttons in conversation screen (ACTIVE/OFFERS_RECEIVED states only)
- **Side Effects**:
  - Records community event
  - Updates community feed
  - Conversation remains for history
- **Implementation**: `lifecycle.go:255-481`

**6. Cancel Request**
- **Location**: `RequestService.CancelRequest` (server), `RequestRepository.cancelRequest` (app)
- **State**: ACTIVE/OFFERS_RECEIVED → CANCELLED
- **Validation**: Only requester can cancel
- **UI**: Available via action buttons in conversation screen (ACTIVE/OFFERS_RECEIVED states only)
- **Restrictions**: Cannot cancel if already fulfilled or cancelled
- **Side Effects**:
  - Records community event
  - Conversation remains for history
- **Implementation**: `lifecycle.go:484-590`

**7. Delete Request**
- **Location**: `RequestService.DeleteRequest` (server), `RequestRepository.deleteRequest` (app)
- **Validation**: Only requester can delete
- **Restrictions**: Can delete from any state
- **Side Effects**:
  - Soft-deletes the request (sets `deleted` metadata with timestamp)
  - Cascades deletion to associated media files
  - Request excluded from all listings and search results
  - Conversation context returns NOT_FOUND
- **Implementation**: `lifecycle.go:701-793`

**8. Share Request with Additional Communities**
- **Location**: `CommunityService.ShareItem` (server) via `RequestService.ShareRequestToCommunity`, `RequestRepository.shareRequest` (app)
- **Purpose**: Share an existing request with additional communities
- **Validation**: Only requester can share; user must be member of target communities
- **Side Effects** (each community goes through the shared `shareRequestToCommunity` core):
  - Creates a new CommunityRequest junction record for each community
  - **Reuses** the single request-scoped conversation (`Request.conversation_id`),
    creating it only on the first share if it does not yet exist
  - Adds requester as a participant
  - Records a community event in each community
  - Skips communities where the request is already shared (idempotent)
- **Implementation**: `services/community/share_item.go` (`ShareItem`), `sharing.go:54-66` (`ShareRequestToCommunity`, the ItemSharer hook), `sharing.go:68-147` (shared `shareRequestToCommunity` core)

**9. Unshare Request from Community**
- **Location**: `CommunityService.UnshareItem` (server) via `RequestService.UnshareRequestFromCommunity`, `RequestRepository.unshareRequest` (app)
- **Purpose**: Remove a request from a specific community
- **Validation**: Only requester can unshare
- **Restrictions**: Cannot unshare from the last community (at least one must remain)
- **Side Effects**:
  - Archives the CommunityRequest record (soft delete)
  - Request no longer visible in that community's feed
  - Conversation remains accessible for history
- **Implementation**: `services/community/unshare_item.go` (`UnshareItem`), `sharing.go:149-211` (`UnshareRequestFromCommunity` core)

### For Helpers (Users Offering to Fulfill)

**1. Browse Community Requests**
- **Location**: `RequestService.ListRequests` (server), `RequestRepository.listRequests` (app)
- **Shows**: Requests in communities user belongs to
- **Default Filter**: Active and offered requests (excludes fulfilled/cancelled)
- **Optional Filter**: By specific state
- **Implementation**: `queries.go:176-272`

**2. View Request Details**
- **Location**: `RequestService.GetRequest` (server), `RequestRepository.getRequest` (app)
- **Shows**: Full request details with requester info, conversation participants, message count
- **Validation**: User must be community member
- **Data Included**:
  - Request details (title, description, location, etc.)
  - Requester profile
  - List of users who have posted messages in the conversation (excluding requester)
  - Total message count
- **Implementation**: `queries.go:18-68`, `helpers.go:22-228` (`buildRequest`)

**3. Offer to Fulfill**
- **Location**: `RequestService.OfferToFulfill` (server), `RequestRepository.offerToFulfill` (app)
- **UI**: Click "OFFER HELP" chip or navigate to conversation
- **Effect**:
  - Adds user to conversation participants (if not already participating)
  - Creates group conversation (if first participant)
  - Adds user to existing conversation (if conversation exists)
  - Updates state to OFFERS_RECEIVED (if first participant)
- **Validation**:
  - Cannot offer on own request
  - Request must be in ACTIVE or OFFERS_RECEIVED state
  - User must be community member
- **Returns**: Full Request object with conversation ID
- **Side Effects**:
  - Records community event for state transition (first participant only)
  - Records offer made event (every offer)
- **Implementation**: `offers.go:23-216`

**4. Leave Conversation**
- **Location**: Participants can simply leave the conversation via conversation screen
- **UI**: No dedicated "Leave Chat" button in request conversation
- **Effect**: User removed from conversation participants
- **Notes**: Request conversations are open to all community members

## UI Elements

### Request Content View

**Helpers Chip** (`_buildHelpersChip` in `request_content_view.dart`):
- **Location**: Top of request detail screen, next to location chip
- **Purpose**: Shows number of users offering help and provides access to helper list
- **Display Logic**:
  - Terminal states (FULFILLED/CANCELLED): Shows "COMPLETED" or "CANCELLED" with disabled tap
  - Active states: Shows "OFFER HELP (n)" where n = number of conversation participants
  - Icon: check_circle when current user has offered, circle_outlined otherwise
- **Tap Action**: Opens modal showing list of helpers (disabled for terminal states)
- **Color**: Always uses primary color (coral) for consistency

**Action Buttons** (for requester only):
- **Location**: Conversation screen bottom action bar
- **Visibility**: Only shown to requester in ACTIVE or OFFERS_RECEIVED states
- **Buttons**:
  - "Mark Fulfilled": Transitions request to FULFILLED state
  - "Cancel Request": Transitions request to CANCELLED state
- **Removed**: "Leave Chat" button (helpers can simply leave via conversation controls)

**Request Participant Modal**:
- **Title**: "Offering Help"
- **Header**: Shows count (e.g., "3 people are offering help")
- **Content**: List of users who have posted messages in the conversation
- **Notes**: Excludes the requester from the participant list
- **Empty State**: "No one has offered help yet. Offer to help to start the conversation!"

## Gear-backed offers: request → loan (#2702)

A helper can satisfy a request with their **actual item** — the request morphs
into a real loan or giveaway instead of staying a chat coordination. Design
and decision log: `docs/issues/2702-request-to-loan.md`.

**A request is born with the needs its text plainly names.** `SubmitRequest`
seeds one one-slot need per entry in `seed_need_names` (the gen call's
`seed_needs` extraction — a single "Lawn mower", or a whole supply list "Picture
books", "Whiteboard", …), trimmed, case-insensitively de-duped, and capped at 8
server-side (`seed_need.go`/`lifecycle.go`, via `InsertBatch`). When the text
names nothing concrete the list is empty and the request is born with **no**
needs — the compose sheet ("What would help?") exists to fill exactly that
empty row (#2731). There is no title-derived fallback (a request titled
"Back-to-school supplies" must not restate its own headline as a need nobody
would claim, #2724).

**The helper side (claim-first).** Claiming a need with linked gear on the
request claim sheet surfaces a Lend/Give choice (default lend). Confirming
runs two client-orchestrated RPCs: the claim
(`ClaimRequestNeed`/`AddRequestContribution`, which auto-creates the
`RequestOffer`) then `TransferService.OfferTransfer` — a transfer born in
`RECIPIENT_SELECTED` with the requester as recipient, carrying
`origin_request_id` and linked back onto the contribution
(`PlanningContribution.transfer_id`). The gear is auto-shared into the
community with the offer's availability (the toggle wins over an existing
`CommunityGear` availability). If the second call fails the claim survives as
a display-only gear link; re-confirming the claim retries the escalation.

**The requester side.** `GetRequest` carries `gear_offers`
(`RequestGearOffer`: transfer + gear + helper + linked need/contribution +
acceptance). Rows render a Lending/Giving tag; the requester's row wears an
**"Offered"** pill they can tap to accept — it flips to **"Accepted ✓"**
(`AcceptRequestOffer`, toggle semantics, stored as
`PlanningContribution.accepted_at_unix_sec`) — the first real emitter of
`COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED`, pushing "Offer Selected" to
the chosen helper. Acceptance is optional; handoff drives fulfillment.

**Lifecycle coupling (bus-driven, services stay independent):**
- **Auto-fulfill at handoff** — loan → `ACTIVE` or giveaway → `COMPLETED` on
  an origin-linked transfer fulfills a request with **at most one need**
  (`fulfill.go:autoFulfillFromTransfer`, actor = the requester so they own
  the undo). Multi-need requests stay open (each handoff covers its need; the
  requester closes via Mark Fulfilled).
- **Siblings stand down** — a fulfilled request cancels its still-open
  (pre-handoff) gear offers with an "Offer closed" push; a handoff covering a
  single-slot need of a multi-need request cancels that need's other offers
  (multi-slot needs keep theirs — three wanted rakes aren't covered by one).
- **Cancel unwinds** — cancelling an origin-linked offer pre-handoff removes
  the escalated contribution, reopens the need slot, and withdraws the
  helper's `RequestOffer` unless another live contribution of theirs remains;
  the existing last-offer reversion then applies.
- **Undo unwinds** — undoing a handoff-driven fulfillment restores the
  request AND cancels the transfer that drove it
  (`RequestFulfillmentUndo.fulfilled_by_transfer_id`). The reverse (the owner
  undoing StartLoan) does **not** un-fulfill the request — a known v1
  asymmetry; the requester's undo covers it.

**Impact.** A transfer-linked fulfillment rolls up **every** handed-off
child transfer's item-based money/emissions/time into the request's
displayed estimate (`impact_metrics.SumTransferImpactDimensions` — means
add, uncertainties combine in quadrature; fixes the zero-value "backdoor
loan", #2275) and stamps `Request.impact_adopted_from_transfer_id` so
aggregation counts those dimensions once per child, on the transfers
(`impact_metrics.MaskTransferAdoptedRequestDimensions`); Quality Time
remains the request's own contribution.

## Conversation Integration

**Request Conversations:**
- Open to all community members (not restricted to offerers)
- **One** request-scoped conversation, created when the request is born into its
  per-item community (#2492) and **reused** across every community it is later
  shared with (no longer one conversation per community)
- The conversation references its topic via `ChatConversation.topic` (a `ConversationTopic` oneof set to `request_id`); the conversation ID is stored on `Request.conversation_id`. `CommunityRequest.conversation_id` is deprecated and no longer written.
- Requester is added as initial participant
- Additional helpers are added when they offer to help
- Participants tracked by who has posted messages (not just joined)

**Conversation Lifecycle:**
- Created on the request's first share — which now happens **at creation**, when
  `SubmitRequest` shares the request into its per-item community
- Reused (not recreated) when the request is shared with additional communities
- Requester is added as initial participant
- Helpers join when they offer to fulfill
- Subsequent offerers join the same shared conversation
- Conversations persist even after request is fulfilled, cancelled, or deleted (for history)
- No explicit "leave" action needed - participants can simply navigate away

## Workflow Examples

These examples describe complete user journeys that should be covered by integration tests. See [update_system_tests.md](../testing/update_system_tests.md) for test implementation guidelines.

### 1. Request Fulfilled (Happy Path)

#### Preconditions

- A test community exists
- Four users exist: a requester, helper A, helper B, and helper C, all members of the community

#### Steps

1. **Requester**: Creates request → `REQUEST_STATE_ACTIVE`
2. **System**: Records community event, appears in community feed
3. **Helper A**: Offers to help → State changes to `OFFERS_RECEIVED`
4. **System**: Creates group conversation with requester and Helper A
5. **Helper B**: Also offers to help
6. **System**: Adds Helper B to existing group conversation
7. **Helper C**: Also offers to help
8. **System**: Adds Helper C to existing group conversation
9. **Requester**: Coordinates with helpers in group chat
10. **Requester**: Marks request as fulfilled → State changes to `FULFILLED`

#### Postconditions

- The request state is `FULFILLED`
- The request is no longer visible in community request feed
- The requester can still see the request in "My Requests"
- The conversation remains accessible for history
- All three helpers remain as conversation participants
- **Notifications sent:**
  - When the request is created (step 1): every other community member receives a "New Request" notification (`REQUEST_CREATED`, gated by `notify_new_requests`).
  - Requester received "Help Offered" notifications when Helper A, B, and C each offered (steps 3, 5, 7) (`REQUEST_OFFER_MADE`, gated by `notify_request_updates`).
  - Helper A, B, and C each received a "Request Fulfilled" notification when the request was fulfilled (step 10) (`REQUEST_FULFILLED`, gated by `notify_request_updates`).
- **Impact estimation:**
  - MarkRequestFulfilled response includes ImpactEstimate with emissions_prevented (flat default from config) and time_saved (mean > 0, stddev > 0)
  - Community metrics time_banked_minutes has positive mean matching the request's time_saved

### 2. Request Cancelled

#### Preconditions

- A test community exists
- Two users exist: a requester and a helper
- The requester has created a request
- The helper has offered to help (request is in `OFFERS_RECEIVED` state)

#### Steps

1. **Requester**: Realizes they no longer need the item
2. **Requester**: Cancels request → State changes to `CANCELLED`
3. **System**: Records community event

#### Postconditions

- The request state is `CANCELLED`
- The request is no longer visible in community request feed
- The conversation remains accessible for history
- **Notifications sent:**
  - When the request was created in the precondition: each member except the requester received a "New Request" notification (`REQUEST_CREATED`).
  - When the helper offered in the precondition: requester received a "Help Offered" notification (`REQUEST_OFFER_MADE`).
  - On cancellation: helper receives a "Request Cancelled" notification (`REQUEST_CANCELLED`, gated by `notify_request_updates`).
- **Impact estimation:** Community impact metrics savings are zero (no fulfilled requests).

### 3. Helper Withdraws Offer

#### Preconditions

- A test community exists
- Three users exist: a requester, helper A, and helper B
- The requester has created a request
- Helper A and Helper B have both offered to help

#### Steps

1. **Helper A**: Leaves the conversation (withdraws offer)
2. **System**: Removes Helper A from participant list

#### Postconditions

- Helper A is no longer a conversation participant
- Helper B remains in the conversation
- The request remains in `OFFERS_RECEIVED` state (at least one helper remains)
- **Notifications sent:**
  - When the request was created in the precondition: each member except the requester received a "New Request" notification (`REQUEST_CREATED`).
  - When Helper A and Helper B each offered in the precondition: requester received "Help Offered" notifications (`REQUEST_OFFER_MADE`).
  - On withdrawal: requester receives an "Offer Withdrawn" notification (`REQUEST_OFFER_WITHDRAWN`, gated by `notify_request_updates`).

### 4. Last Helper Withdraws

#### Preconditions

- A test community exists
- Two users exist: a requester and a helper
- The requester has created a request
- The helper has offered to help (request is in `OFFERS_RECEIVED` state)

#### Steps

1. **Helper**: Leaves the conversation (withdraws offer)
2. **System**: Removes helper from participant list

#### Postconditions

- The request reverts to `ACTIVE` state (no more offerers)
- The request is visible in community request feed again
- **Notifications sent:**
  - When the request was created in the precondition: each member except the requester received a "New Request" notification (`REQUEST_CREATED`).
  - When the helper offered in the precondition: requester received a "Help Offered" notification (`REQUEST_OFFER_MADE`).
  - On withdrawal: requester receives an "Offer Withdrawn" notification (`REQUEST_OFFER_WITHDRAWN`).

### 5. Request Deleted

#### Preconditions

- A test community exists
- Two users exist: a requester and a helper
- The requester has created a request with a unique searchable title
- The helper has offered to help (conversation exists)

#### Steps

1. **Requester**: Deletes the request via the overflow menu
2. **System**: Soft-deletes the request (sets `deleted` metadata with timestamp)

#### Postconditions

- The request returns `NOT_FOUND` when accessed directly
- The request is excluded from community request listings
- The request is excluded from "My Requests" list
- The request is excluded from search results
- The conversation context returns `NOT_FOUND` (topic is deleted)
- Non-owners cannot delete the request (returns `PERMISSION_DENIED`)
- **Notifications sent:**
  - When the request was created in the precondition: each member except the requester received a "New Request" notification (`REQUEST_CREATED`).
  - When the helper offered in the precondition: requester received a "Help Offered" notification (`REQUEST_OFFER_MADE`).
  - Deletion itself does not trigger a notification.

**Note**: This is a soft-delete operation. The request data is retained in the database with deletion metadata for potential audit/recovery purposes.

### 6. Cross-Community Sharing

#### Preconditions

- Two test communities exist (Community A and Community B)
- A requester is a member of both communities
- The requester has created a request in Community A
- Helper X is a member of Community A only
- Helper Y is a member of Community B only

#### Steps

1. **Requester**: Creates request in Community A → `REQUEST_STATE_ACTIVE`
2. **System**: Creates conversation for Community A, adds requester as participant
3. **Helper X**: Offers to help in Community A
4. **System**: Adds Helper X to Community A conversation
5. **Requester**: Shares the same request with Community B
6. **System**:
   - Creates CommunityRequest junction for Community B
   - Reuses the existing request-scoped conversation (not a new one)
   - Records community event in Community B
7. **Helper Y**: Offers to help in Community B
8. **System**: Adds Helper Y to the shared request conversation
9. **Requester**: Coordinates with both helpers in the one shared conversation
10. **Requester**: Marks request as fulfilled → State changes to `FULFILLED` (affects both communities)

#### Postconditions

- The request state is `FULFILLED` in both communities
- A single shared conversation exists (`Request.conversation_id`), not one per community
- Helper X, Helper Y, and the requester all share that one conversation
- Request is no longer visible in either community's active feed
- **Notifications sent:**
  - Step 1 (request created in Community A): each Community-A member except the requester receives a "New Request" notification (`REQUEST_CREATED`, gated by `notify_new_requests`).
  - Step 5 (request shared with Community B): no `REQUEST_CREATED` is emitted — sharing an existing request does not re-broadcast its creation. Community B members learn of the request via the feed.
  - Steps 3 and 7: requester received "Help Offered" notifications when Helper X and Helper Y offered (one per community, both gated by `notify_request_updates`).
  - Step 10 (fulfillment): both Helper X and Helper Y receive **1 "Request Fulfilled" notification** each — the per-community fulfillment events are deduplicated into a single push per offerer (#2088).

**Note on cross-community fulfillment notifications**: When a request shared with multiple communities is fulfilled, each community still records its own `REQUEST_FULFILLED` event (the per-community feed/story depends on it). The recipient set for each event is "all offerers minus the actor," computed across the request as a whole. Because all of these events originate from the single `MarkRequestFulfilled` call, they share that request's correlation id, and the notification layer deduplicates per offerer so each offerer receives exactly **one** "Request Fulfilled" push regardless of how many communities the request was shared with (#2088).

### 7. Unsharing from Community

#### Preconditions

- Two test communities exist (Community A and Community B)
- A requester is a member of both communities
- The requester has created a request shared with both communities
- Helper A has offered in Community A
- Helper B has offered in Community B

#### Steps

1. **Requester**: Unshares request from Community B
2. **System**: Archives the CommunityRequest record for Community B

#### Postconditions

- Request is no longer visible in Community B feed
- Request remains visible in Community A feed
- The shared request conversation is untouched (unshare only archives the
  CommunityRequest row; it does not touch `Request.conversation_id`)
- Helper B can still access the conversation history
- Request state unchanged
- Cannot unshare from Community A (would be the last community)
- **Notifications sent:**
  - When the request was created in Community A: each Community-A member except the requester received a "New Request" notification (`REQUEST_CREATED`).
  - The subsequent share with Community B does **not** emit `REQUEST_CREATED` (sharing an existing request does not re-broadcast its creation).
  - When Helper A and Helper B each offered: requester received "Help Offered" notifications.
  - Unsharing itself does not trigger a notification.

## AI-Powered Request Generation

The request service includes AI assistance for creating requests:

**GenRequest Operation:**
- **Location**: `gen.go`
- **Input**: User's natural language prompt (e.g., "I need a drill for weekend project")
- **AI Processing**:
  - Generates clear title (e.g., "Power Drill")
  - Enhances description with helpful details
  - Extracts relevant tags/categories
  - Suggests appropriate location (user's primary or community center)
- **Returns**: Preview data (NOT saved to database)
- **Client Flow**: User reviews AI suggestions, can edit before submitting

## Unsplash Integration

Requests automatically fetch stock images if no media is provided:

**Async Image Fetching:**
- **Location**: `stock_imagery.go` (`fetchAndAttachStockImage`, via the `stockImageryProvider` abstraction — Pexels/Unsplash)
- **Trigger**: When `SubmitRequest` called without `media_id`
- **Process**:
  1. Request is created and returned immediately
  2. Background goroutine searches the stock-imagery provider using description
  3. Downloads first matching photo
  4. Uploads to bucket storage
  5. Updates request with media_id
- **Notes**: Non-blocking; request is usable before image arrives

## Data Flow

### Server → Client

**Request Data:**
- Server: `RequestService` (CRUD operations, offers)
- Client: `RequestRepository` (cached access)
- Caching: Individual requests by ID, community request lists, user's requests

**Cache Invalidation:**
- Repositories automatically invalidate relevant caches after mutations
- Pattern: `await repository.operation(); await repository.refresh();`
- Cache keys follow namespace pattern: `'request:item:id'`, `'request:list:community:id'`, `'request:list:my'`

### Client State Management

Request-related ViewModels manage:
- Request lists (community requests, my requests)
- Request details and state
- Offer submission
- Loading and error states
- AI generation previews

**Key ViewModel Operations:**
- `loadCommunityRequests`: Fetches requests for a community
- `loadMyRequests`: Fetches user's own requests
- `submitRequest`: Creates new request (with or without AI generation)
- `offerToFulfill`: Makes an offer on a request
- `withdrawOffer`: Withdraws an offer from a request (offerer only)
- `markFulfilled`: Marks request as fulfilled (requester only)
- `cancelRequest`: Cancels request (requester only)
- `updateRequest`: Updates request details (requester only)
- `deleteRequest`: Deletes request permanently (requester only)
- `shareRequest`: Shares request with additional communities (requester only)
- `unshareRequest`: Removes request from a specific community (requester only)

## Business Rules

### Validation Rules

1. **Community Membership**: Operations on an existing community require the user to be a member of it. Submitting a request no longer requires (or accepts) an existing community — every request is born in its own host-only per-item community (#2492); posting it into existing communities goes through `ShareItem`, which requires active membership of each target.
2. **Requester Ownership**: Only requester can update, fulfill, cancel, delete, share, or unshare their request
3. **Self-Offer Prevention**: Users cannot offer to fulfill their own requests
4. **State Requirements**:
   - Updates only allowed in ACTIVE or OFFERS_RECEIVED states
   - Fulfillment only allowed in ACTIVE or OFFERS_RECEIVED states
   - Cannot cancel already fulfilled or cancelled requests
   - Deletion allowed from any state
5. **Multiple Offers Allowed**: Same user can't offer twice, but system prevents duplicates
6. **Offer State Requirements**: Can only offer on ACTIVE or OFFERS_RECEIVED requests
7. **Sharing Requirements**:
   - Requester must be member of all target communities when sharing
   - Cannot unshare from the last community (at least one must remain)
   - Duplicate shares are silently skipped

### Side Effects

1. **State Transitions**: Automatic when first offer is made (ACTIVE → OFFERS_RECEIVED)
2. **State Reversion**: Automatic when last offerer withdraws (OFFERS_RECEIVED → ACTIVE)
3. **Conversation Creation**:
   - Automatic when the request is created — one request-scoped conversation,
     created as the request is shared into its per-item community (#2492)
   - Reused (not recreated) when the request is shared with additional communities
4. **Conversation Joining**: Automatic when offers are made
5. **Conversation Removal**: Automatic when offerer withdraws
6. **Unsplash Image Fetching**: Async when no media_id provided
7. **Community Events**: Recorded for all state changes, offers, and sharing actions
8. **Feed Updates**: Requests appear in community feed for each community they're shared with
9. **Media Cascade Deletion**: When request is deleted, associated media files are also deleted
10. **Notifications**: See [Notifications](#notifications) section below

### Notifications

Push notifications are sent for specific request events to keep users informed. See [push_notifications.md](../push_notifications.md) for architecture details.

All notifications are gated per recipient by their per-(user, community) preferences row (see [push_notifications.md](../push_notifications.md)). Unset toggles resolve to "on", so default behavior is to notify.

| Event             | Category                | Recipient                       | Notification Content                                                  |
| ----------------- | ----------------------- | ------------------------------- | --------------------------------------------------------------------- |
| Request created   | `notify_new_requests`   | All other community members     | "New Request" / "[user] posted '[request title]'" *(emitted on the original `SubmitRequest` only, not on subsequent `ShareItem` calls)* |
| Offer made        | `notify_request_updates`| Request creator                 | "Help Offered" / "[user] offered to help with '[request title]'"      |
| Offer selected    | `notify_request_updates`| Selected offerer                | "Offer Selected" / "[user] chose your offer for '[request title]'" *(emitted by `AcceptRequestOffer` on gear-backed offers, #2702)* |
| Offer withdrawn   | `notify_request_updates`| Request creator                 | "Offer Withdrawn" / "[user] withdrew an offer for '[request title]'"  |
| Request fulfilled | `notify_request_updates`| All offerers (except actor)     | "Request Fulfilled" / "'[request title]' was marked as fulfilled"     |
| Request cancelled | `notify_request_updates`| All offerers (except actor)     | "Request Cancelled" / "'[request title]' was cancelled"               |
| Gear offer stood down | `notify_transfer_updates` | The helper (gear owner)     | "Offer closed" / "[requester]'s request no longer needs [gear] — thanks for offering" *(a `TRANSFER_CANCELLED` on an origin-linked offer, #2702)* |

**Cross-community fan-out:** Cross-community sharing emits one event per community for `REQUEST_CREATED` and one per community for `REQUEST_FULFILLED` / `REQUEST_CANCELLED` (each community's feed and story depend on its own event). Push notifications, however, are **deduplicated per recipient**: all of the per-community events for a single fulfillment/cancellation originate from one RPC and share its correlation id, so each offerer receives exactly **one** push for that action regardless of how many communities the request was shared with (#2088).

**Deep linking:** All notifications include `conversation_id` for direct navigation to the request conversation.

## Error Handling

**Server:**
- Uses Connect-style error codes (NotFound, PermissionDenied, FailedPrecondition, InvalidArgument)
- Validates state requirements and returns descriptive errors
- Logs all operations for debugging

**Client:**
- Wraps service exceptions in `ServiceException` with user-friendly messages
- ViewModels store error state for UI display
- Repository operations fail gracefully with cache invalidation

## Testing Strategy

**Server:**
- Lifecycle tests for creation, update, fulfillment, cancellation, deletion (`lifecycle_test.go`)
- Offer tests for offer-to-fulfill flow, withdrawal, and conversation management (`offers_test.go`)
- Sharing tests for cross-community sharing and unsharing (`sharing_test.go`)
- Query tests for listing and filtering (`queries_test.go`)
- AI generation tests (`gen_test.go`)
- Stock-imagery integration tests (`stock_imagery_test.go`)

**Client:**
- Repository tests with mocked services
- ViewModel tests for business logic
- Widget tests for UI interactions

## Future Considerations

1. **Helper Selection**: Gear-backed offers have an optional accept (`AcceptRequestOffer`, #2702); plain chat offers still coordinate informally
2. **Offer Limits**: No limit on number of offerers
3. **Request Expiration**: No automatic expiration or cleanup of old requests
4. **Ratings/Reviews**: No feedback system after fulfillment
5. **Request Types**: Could categorize requests (borrow vs. gift vs. service)
6. **Priority/Urgency**: No urgency flag or prioritization
7. **Request Templates**: Could provide common request templates
8. **Location Matching**: Could suggest requests based on proximity
9. **Notification Preferences**: Could allow users to opt in/out of request notifications
10. **Request Analytics**: Could track fulfillment rates, response times, etc.
