---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: End-to-end experience (event) workflow — five-state lifecycle, RSVP and conversation creation, time-proposal voting, cross-community sharing, and attendance tracking across server and Flutter client.
  globs: [server/services/experience/**, app/lib/data/repositories/experience_repository.dart, app/lib/presentation/screens/experience/**, app/lib/presentation/widgets/experience/**, app/lib/presentation/widgets/sharing/**, app/lib/presentation/viewmodels/experience_view_model.dart]
  triggers: [experience, event, rsvp, time-proposal, attendance, lifecycle, in-process, sharing]
  lens: [workflow, domain]
freshness:
  verified_commit: "82c3518d0"
  verified_on: "2026-08-17"
---
# Experience Workflow

This document explains the architecture, operations, state transitions, and workflow logic for community experiences (events and activities) in the Ripls application. Experiences allow community members to create, share, and coordinate gatherings, activities, and events.

## Architecture Overview

The experience workflow enables users to create community events and activities, share them with their communities, and coordinate attendance through an RSVP system with integrated group chat.

### Core Components

**Protocol Definitions:**
- `proto/ripls/models/experience.proto`: Storage model for experiences, RSVPs, and time data
- `proto/ripls/api/experience_service.proto`: API types for experience operations
- `proto/ripls/api/experience.proto`: API types for Experience and ExperienceState
- `proto/ripls/api/time.proto`: Time representation (ExperienceTime, TimeConfidence)

**Server Services:**
- `server/services/experience/`: Experience service implementation
  - `service.go`: Service initialization and helper functions
  - `save.go`: Experience creation and update operations
  - `share.go`: Community sharing logic
  - `rsvp.go`: RSVP management and conversation creation
  - `lifecycle.go`: State transitions (MarkInProcess, Complete, Cancel)
  - `time_proposals.go`: Time proposal voting, confirmation (lock/unlock)
  - `delete.go`: Experience deletion
  - `get.go`: Listing and retrieval operations (GetExperience, ListExperiences, ListMyExperiences)

**Client (Flutter):**
- `app/lib/data/repositories/experience_repository.dart`: Cached experience data access
- `app/lib/presentation/viewmodels/experience_view_model.dart`: Experience UI logic
- `app/lib/presentation/viewmodels/experience_sharing_view_model.dart`: Cross-community sharing state
- `app/lib/presentation/viewmodels/time_modal_view_model.dart`: Time Modal state management (AsyncNotifier)
- `app/lib/presentation/screens/experience/`: Experience UI screens
  - `experience_content_view.dart`: Full experience detail view with editing and sharing
  - `time_poll_sheet.dart`: Single morphing sheet for the time-poll flow (propose/vote/finalize)
- `app/lib/presentation/widgets/experience/`: Experience-specific widgets
  - `time_proposal_tile.dart`: Individual time proposal tile (vote, select, lock)
  - `propose_time_form.dart`: Date/time/duration picker for proposing times
- `app/lib/presentation/widgets/sharing/`: Reusable sharing UI components
  - `community_selection_sheet.dart`: Sheet for selecting communities (used by gear, requests, and experiences)
  - `community_list_item.dart`: Community checkbox item with share/unshare toggle
  - `item_share_sheet.dart`: The per-item share sheet (QR + open link, Invite people, Add Community) opened after Save and from "Who's In"

## Two-Phase Creation & the Per-Item Community (#2492)

Creating an event and inviting people are **two separate phases** (the phone-first
pivot, epic #2492). This shapes both the server contract and the client UX.

**Creation provisions a per-item community.** `SaveExperience` (on insert) calls the
shared `community` library's `ProvisionPerItemCommunity` to create a **host-only,
nameless "ad-hoc" community** keyed to the new event (`Community.origin_experience_id`),
then shares the event into it via `shareExperienceToCommunity`. So **every event is
born already shared** to its own per-item community — the conversation is created, the
owner is auto-RSVP'd YES, and the `EXPERIENCE_CREATED` event fires, all at creation
time (not on a later explicit share). `SaveExperienceResponse.item_community_id` returns
that community's id. A nameless community is the event's default audience; it becomes a
"real" persistent community if it is later given a name.

**The client creation flow is Save-only.** The unified-create modal's primary button
reads **"Save Event"** (not "Share"); it collects no audience. On a successful save the
modal pops and the caller (`home_screen` / `blank_create_dispatcher`) runs the
post-creation flow and then **auto-opens the share sheet** (`ItemShareSheet.show`).

**Inviting is a separate action via the share sheet.** `ItemShareSheet`, on open, calls
`CommunityService.ShareItem(experienceId)` — which finds the event's per-item community
(provisioned at creation) and returns its open `/go/{code}` link + QR. From there the
host can Copy/Share the link, **Invite people** (phone/email → provisional members +
host-relay SMS, via `ShareItem` invitees), or **Add Community** (share to existing
communities via `ShareItem.share_to_community_ids`). `ShareItem` is the only add-path:
the per-type `ShareExperience` RPC was removed in #2526.

## Experience States

Experiences follow a state machine with five states:

```protobuf
enum ExperienceState {
    EXPERIENCE_STATE_UNSPECIFIED = 0;
    EXPERIENCE_STATE_ACTIVE = 1;       // Created & available for RSVPs
    EXPERIENCE_STATE_JOINED = 2;       // Someone besides owner RSVPed Yes/Maybe
    EXPERIENCE_STATE_IN_PROCESS = 3;   // Experience is happening now
    EXPERIENCE_STATE_COMPLETED = 4;    // Experience finished successfully
    EXPERIENCE_STATE_CANCELLED = 5;    // Experience called off
}
```

### Valid State Transitions

The state machine is enforced in `server/services/experience/lifecycle.go`:

**From ACTIVE:**
- → JOINED (automatic when first RSVP Yes/Maybe is received)
- → IN_PROCESS (owner only, via MarkExperienceInProcess)
- → COMPLETED (owner only, via CompleteExperience — skips IN_PROCESS for past/informal events)
- → CANCELLED (owner only, via CancelExperience)

**From JOINED:**
- → IN_PROCESS (owner only, via MarkExperienceInProcess)
- → COMPLETED (owner only, via CompleteExperience — skips IN_PROCESS for past/informal events)
- → CANCELLED (owner only, via CancelExperience)

**From IN_PROCESS:**
- → COMPLETED (owner only, via CompleteExperience)
- → CANCELLED (owner only, via CancelExperience)

**From COMPLETED:**
- No transitions allowed (terminal state)

**From CANCELLED:**
- No transitions allowed (terminal state)

### Experience Time Model

Experiences support flexible time specifications:

```protobuf
message ExperienceTime {
  oneof time_type {
    SpecificTime specific = 1;     // Precise date/time with timezone
    TimeRange range = 2;            // Flexible window with start/end, timezone
    TimeTBD tbd = 3;                // To be determined
  }
  string informal_description = 4;  // User-friendly text (e.g., "next Friday evening")
  TimeConfidence confidence = 5;    // AI confidence level (EXPLICIT, INFERRED, UNKNOWN)
}
```

**Time Confidence Levels:**
- `EXPLICIT`: User gave specific time (e.g., "3pm", "2:30")
- `INFERRED`: User gave fuzzy time (e.g., "morning", "afternoon")
- `UNKNOWN`: No time mentioned

## RSVP System

Users can indicate their attendance intention with three options:

```protobuf
enum RSVPIntention {
  RSVP_INTENTION_YES = 1;      // Planning to attend
  RSVP_INTENTION_MAYBE = 2;    // Interested/tentative
  RSVP_INTENTION_NO = 3;       // Not attending
}
```

**RSVP Features:**
- Users can change their RSVP at any time before the experience
- Owner receives notification when users RSVP
- RSVP counts are tracked for YES and MAYBE intentions
- After experience completion, owner can record actual attendance

**Attendance Tracking:**
```protobuf
enum AttendedStatus {
  ATTENDED_STATUS_UNKNOWN = 1;   // Not yet recorded
  ATTENDED_STATUS_YES = 2;       // Attended
  ATTENDED_STATUS_NO = 3;        // Did not attend
}
```

## Time Proposals & Scheduling

Time coordination uses a dedicated Time Modal that allows participants to propose, vote on, and finalize meeting times. See [time.md](../client/time.md) for detailed client-side business logic.

### Time Proposal Flow

1. **Organizer creates experience** with an initial event time (or TBD)
2. **Participants propose alternative times** via the Time Modal
3. **All participants vote** by tapping proposal cards (tap-to-toggle yes vote)
4. **Organizer selects and finalizes** a time (two-step: Select → Finalize)
5. **Main modal updates** — `WhenRow` shows locked time with sage green styling

### Time Modal Architecture

The Time Modal is a self-contained bottom sheet with its own `TimeModalNotifier` (AsyncNotifier pattern). It communicates state changes back to the main modal via repository cache invalidation — the Time Modal calls repository methods, which invalidate the cache, and the main modal's `ref.watch` triggers a rebuild.

**Modes:** default (card list with voting), propose (date/time/duration form), confirmed (locked time display)

**Key operations:**
- `ProposeTime`: Any participant can propose a new time
- `VoteOnTime`: Tap a card to toggle a yes vote (no explicit "No" vote)
- `ConfirmTime`: Organizer locks a selected time proposal
- `UnlockTime`: Organizer unlocks a previously confirmed time
- `SaveExperience`: When organizer selects a proposal, the experience's root time updates to match

### WhenRow Summary Card

The `WhenRow` appears in both organizer and joiner main modals as a tappable summary:
- **Unlocked**: Shows leading candidate time, vote counts, coral calendar icon
- **Locked**: Shows confirmed time, duration, lock icon, sage green styling
- **Tap action**: Opens the Time Modal

## Conversation Model

Each experience has **one** group conversation, scoped to the experience itself and reused across every community it is shared with:

**Conversation Architecture:**
- An experience can be shared with multiple communities
- All shares reuse a single experience-scoped conversation (stored in `Experience.conversation_id`, field 10 of the `Experience` message)
- The conversation is created when the event is first shared into a community — which now happens **at creation**, when `SaveExperience` shares the event into its per-item community (#2492) — then reused on every subsequent share
- `CommunityExperience.conversation_id` is **deprecated** and no longer written — it remains on the proto (field 5, marked `deprecated`) for migration reads only

**Conversation Flow:**
1. **At creation (first share)**: `SaveExperience` shares the event into its per-item community, which creates the conversation and persists its ID on `Experience.conversation_id`. Subsequent shares (to additional communities) reuse it (see `share.go:143-171`)
2. **Initial Participant**: Owner is automatically added as a participant
3. **Anchor + Seed**: On first creation only, an `EXPERIENCE_CREATED` anchor system message and the owner's description are posted; reused conversations on later shares do not duplicate the anchor
4. **RSVP Joining**: Users who RSVP Yes/Maybe are automatically added to the experience conversation (`rsvp.go:167-178`)
5. **RSVP Leaving**: Changing from Yes/Maybe to No does **not** remove the user from the conversation — because the conversation is shared across communities, membership is not revoked on a per-community decline (no removal call exists on the No path in `rsvp.go`)
6. **Persistence**: Conversation persists after the experience is completed/cancelled (for history)

**Multi-Community Behavior:**
- Same experience shared to 3 communities = **1** shared conversation, not three
- Members of any community that the experience is shared with see the same conversation and the same RSVP/message history
- RSVPs are still community-specific (user can RSVP differently in different communities), but the chat thread is common

**Conversation Archiving:**
- Conversations are implicitly archived when `is_item_done && unread_count == 0`
- Archived conversations appear in the "Archived" inbox view
- New messages in an archived conversation move it back to the active view

**Key Points:**
- All RSVPed users (Yes/Maybe) across all shared communities share the single experience conversation
- Owner participates in that one conversation regardless of how many communities the experience is shared with
- Transparent coordination between all attendees in a single thread
- Conversation membership is additive on Yes/Maybe RSVP; it is not pruned on No

## User Operations

### For Experience Organizers (Owners)

**1. Create Experience**
- **Location**: `ExperienceService.SaveExperience` (server), `ExperienceRepository.createExperience` (app)
- **Initial State**: `EXPERIENCE_STATE_ACTIVE`
- **Required Fields**: Name, description
- **Optional Fields**: Media IDs, location ID, time, max participants
- **Default Time**: TBD if not specified
- **Per-item community (#2492)**: on insert, provisions the event's host-only per-item
  community and shares the event into it (conversation created, owner auto-RSVP'd YES,
  `EXPERIENCE_CREATED` event fired); returns `item_community_id`. See
  [Two-Phase Creation](#two-phase-creation--the-per-item-community-2492).
- **Client UX**: Save-only ("Save Event"); the share sheet auto-opens after save.
- **Implementation**: `save.go:30-525` (per-item community: the `community.ProvisionPerItemCommunity` + `shareExperienceToCommunity` calls in the insert branch)

**2. Generate AI-Powered Experience**
- **Location**: `ExperienceService.StreamGenExperience` (server), `ExperienceRepository.streamGenExperience` (app)
- **Purpose**: Get AI suggestions from text prompt or uploaded flyer/poster image
- **Returns**: AI-generated name, description, suggested time, location suggestions
- **Notes**: Does NOT save to database; preview only

**3. Update Experience**
- **Location**: `ExperienceService.SaveExperience` (server), `ExperienceRepository.saveExperience` (app)
- **Validation**: Owner only
- **Updatable Fields**: Name, description, media IDs, location ID, time, max participants
- **Notes**: Supports partial updates (only provided fields are modified)
- **Implementation**: `save.go:30-525`

**4. Share with Additional Communities / Invite People**
- **Location**: `CommunityService.ShareItem` (server) via `ExperienceService.ShareExperienceToCommunity`, `CommunityRepository.shareItem` (app)
- **Effect** (every share reuses `shareExperienceToCommunity`):
  - Creates a `CommunityExperience` relationship for each additional community
  - Reuses the experience-scoped conversation created at creation (`Experience.conversation_id`)
  - Adds owner as a participant; auto-RSVPs the owner YES once (already done at creation)
  - Records a community event for the activity feed
- **`ShareItem` additionally**: finds the event's per-item community, mints/returns its
  open link, attaches phone/email **invitees** (provisional members + host-relay), and
  shares to any `share_to_community_ids`. This is what the share sheet drives.
- **Validation**: Owner only, must be a member of each target community
- **Implementation**: `services/community/share_item.go` (`ShareItem`), `share.go:62-70` (`ShareExperienceToCommunity`, the ItemSharer hook), `share.go:78-225` (shared `shareExperienceToCommunity` core)

**4a. Unshare from Community**
- **Location**: `CommunityService.UnshareItem` (server) via `ExperienceService.UnshareExperienceFromCommunity`, `ExperienceRepository.unshareExperience` (app)
- **Effect**:
  - Removes `CommunityExperience` relationship
  - Experience no longer appears in that community's feed
  - Conversation remains accessible (for history)
- **Validation**: Owner only
- **UI**: "Communities" menu item in overflow menu (owner only)
- **Implementation**: `services/community/unshare_item.go` (`UnshareItem`), `unshare.go:20-72` (`UnshareExperienceFromCommunity` core)

**5. Confirm/Unlock Time**
- **Location**: `ExperienceService.ConfirmTime` / `UnlockTime` (server), `ExperienceRepository.confirmTime` / `unlockTime` (app)
- **Validation**: Owner only
- **Effect**: Locks or unlocks a time proposal; updates `WhenRow` in main modal via cache invalidation
- **Implementation**: `time_proposals.go`

**6. Mark Experience In Process**
- **Location**: `ExperienceService.MarkExperienceInProcess` (server), `ExperienceRepository.markInProcess` (app)
- **State**: ACTIVE/JOINED → IN_PROCESS
- **Effect**: Posts a `CHAT_SYSTEM_ACTION_STARTED` system message in the experience conversation; emits an `EXPERIENCE_STARTED` `CommunityEvent` per shared community via the standard notification funnel — recipients are Yes/Maybe RSVPs minus the actor, gated by `notify_experience_rsvps`, with stream-suppression applied
- **Validation**: Owner only
- **Implementation**: `lifecycle.go:25-115`

**7. Complete Experience**
- **Location**: `ExperienceService.CompleteExperience` (server), `ExperienceRepository.completeExperience` (app)
- **State**: ACTIVE / JOINED / IN_PROCESS → COMPLETED
- **Effect**: Posts a `CHAT_SYSTEM_ACTION_COMPLETED` system message; emits an `EXPERIENCE_COMPLETED` `CommunityEvent` per shared community (recipients are Yes/Maybe RSVPs minus the actor — same lifecycle funnel as START/CANCEL, *not* a community broadcast — gated by `notify_experience_completed`); generates a story card per shared community. The notification is RSVP-scoped so completing an event with no other attendees doesn't ping the whole community; the story/recap card still generates regardless of the notification audience.
- **Validation**: Owner only
- **Notes**: Client calls `RecordAttendance` first, then `CompleteExperience`. Story generation happens inside `CompleteExperience` after attendance is already recorded, so all participants are included.
- **Implementation**: `lifecycle.go:120-402`

**8. Cancel Experience**
- **Location**: `ExperienceService.CancelExperience` (server), `ExperienceRepository.cancelExperience` (app)
- **State**: Any non-terminal → CANCELLED
- **Effect**: Posts a `CHAT_SYSTEM_ACTION_CANCELLED` system message; emits an `EXPERIENCE_CANCELLED` `CommunityEvent` per shared community via the standard notification funnel — recipients are Yes/Maybe RSVPs minus the actor, gated by `notify_experience_rsvps`, with stream-suppression applied
- **Validation**: Owner only, cannot cancel if already completed
- **Implementation**: `lifecycle.go:479-591`

**9. Record Attendance**
- **Location**: `ExperienceService.RecordAttendance` (server), `ExperienceRepository.recordAttendance` (app)
- **Validation**: Owner only, experience can be in any valid state
- **Effect**:
  - Updates RSVP attendance status (YES, NO, UNKNOWN)
  - Auto-creates RSVP records for participants added at completion time (no prior RSVP)
- **Cross-community scoping**: attendance for a **registered** attendee is
  resolved **event-scoped** — by `(experience_id, user_id)` across every shared
  community, not by the request's `community_id`. The completion roster
  (`buildRSVPs`) is itself cross-community, so an attendee's RSVP may live in a
  different community than the one the host completes from; the request's
  `community_id` only scopes provisional-user lookups and the community a
  brand-new (never-RSVP'd) participant's RSVP is created in. Scoping the lookup
  to a single community previously 404'd the whole batch for cross-community
  attendees (#2690).
- **Purpose**: Track who actually showed up; called by client **before** `CompleteExperience` so attendance is set when the story is generated
- **Implementation**: `rsvp.go` `RecordAttendance`

**10. Delete Experience**
- **Location**: `ExperienceService.DeleteExperience` (server), `ExperienceRepository.deleteExperience` (app)
- **Validation**: Owner only
- **Effect**: Permanently removes experience and associated data
- **Implementation**: `delete.go`

**11. Manage the "Who's In" Roster (#2492)**
- **Location**: `ExperienceService.SetExperienceMemberRSVP` / `RemoveExperienceMember` (server)
- **Validation**: Owner only
- **Effect**:
  - `SetExperienceMemberRSVP`: host sets an invitee's RSVP on their behalf
    (invited → going / maybe / not going); upserts a single RSVP per
    (experience, user), a new one scoped to the event's per-item (ad-hoc origin)
    community
  - `RemoveExperienceMember`: host uninvites a directly-invited individual —
    soft-deletes their membership in the event's per-item community and clears
    their RSVP. The host cannot remove themselves.
- **UI**: the host roster sheet (`whos_in_manage_sheet.dart`)
- **Implementation**: `member_management.go`

### For Participants (Attendees)

**1. Browse Community Experiences**
- **Location**: `ExperienceService.ListExperiences` (server), `ExperienceRepository.listCommunityExperiences` (app)
- **Shows**: Experiences shared with communities user belongs to
- **Filtering**: Optional by state (ACTIVE, JOINED, IN_PROCESS)

**2. View Experience Details**
- **Location**: `ExperienceService.GetExperience` (server), `ExperienceRepository.getExperienceDetails` (app)
- **Shows**: Full experience details with RSVP list, owner info, conversation ID
- **Validation**: User must be community member

**3. RSVP to Experience**
- **Location**: `ExperienceService.RSVPToExperience` (server), `ExperienceRepository.rsvp` (app)
- **Options**: YES, MAYBE, NO
- **Effect**:
  - Creates or updates RSVP record (community-specific; a user may RSVP to an experience only once across all communities)
  - First RSVP Yes/Maybe transitions experience to JOINED state
  - Adds user to the experience conversation if RSVPing Yes/Maybe
  - Changing to No does NOT remove the user from the conversation (the conversation is shared across communities)
  - Sends notification to owner
- **Validation**:
  - Experience must be in ACTIVE, JOINED, or IN_PROCESS state (users can join an event that is already happening)
  - Caller must be a member of the supplied community and the experience must be shared with it
  - Max participants check (if specified and RSVPing YES)
- **Notes**: RSVPs are community-specific; user can RSVP differently in different communities, but they share one conversation
- **Returns**: Updated experience with the experience conversation ID
- **Implementation**: `rsvp.go:22-329`

**4. Change RSVP**
- **Location**: `ExperienceService.RSVPToExperience` (server), `ExperienceRepository.rsvp` (app)
- **Effect**: Updates existing RSVP record
- **Notes**: User can change intention multiple times before experience

**5. Join Conversation**
- **Access**: Via RSVP chip in UI or conversation ID in experience details
- **Requirement**: Must have RSVPed Yes/Maybe
- **Auto-navigation**: UI automatically navigates to conversation after RSVPing Yes/Maybe

## UI Elements

### Organizer Modal Flow (3 Phases)

The organizer modal uses a simplified 3-phase flow:

| Phase | Name | Description |
|-------|------|-------------|
| 0 | Planning | Title, description, LocationRow, WhenRow (→ Time Modal), RSVP toggles, response list |
| 1 | Wrap Up | Host-written recap (starts empty, no AI draft — #2936), attendance checkoff with check/X buttons |
| 2 | Complete | Summary card, attendee avatar stack, impact metrics |

The "Confirmed" phase was removed — locking a time updates the `WhenRow` inline. The organizer can proceed directly from Planning to Wrap Up once a time is locked.

**Phase-to-State Mapping:**

| ExperienceState | Organizer Phase | Joiner Phase |
|-----------------|----------------|--------------|
| ACTIVE | 0 (Planning) | 0 (RSVP) |
| JOINED | 0 (Planning) | 0 (RSVP) |
| IN_PROCESS | 0 or 1 (Wrap Up) | 0 (RSVP) |
| COMPLETED | 2 (Complete) | 1 (Complete) |
| CANCELLED | Cancelled view | Cancelled view |

### Joiner Modal Flow (2 Phases)

| Phase | Name | Description |
|-------|------|-------------|
| 0 | RSVP | Title, description, organizer info, LocationRow, WhenRow (→ Time Modal), RSVP toggles, response list |
| 1 | Complete | Summary card, attendee avatar stack, impact metrics |

### Experience Content View

**RSVP Status Display**:
- **Location**: Shown inline within the organizer and joiner modals via `RSVPToggleButtons`
- **Purpose**: Shows RSVP status and allows participants to indicate their attendance intention
- **Display**: Three toggle buttons (YES, MAYBE, NO) with current selection highlighted
- **Tap Action**: Updates RSVP intention and navigates to conversation for YES/MAYBE selections

**Experience Modal Flows**:
- **Title**: "RSVP"
- **Header**: Shows attendee summary (e.g., "5 going, 2 maybe")
- **Content**:
  - Three RSVP buttons (YES, MAYBE, NO) with current selection highlighted
  - List of participants grouped by intention (YES, MAYBE)
  - Empty state: "No RSVPs yet. Be the first to RSVP!"
- **Action**: Updates RSVP and navigates to conversation for YES/MAYBE selections
- **Attendance Verification** (owner only, COMPLETED state):
  - Shows unified "Confirmed Attendees" list with all RSVPs
  - Each attendee has inline Yes/No toggle buttons (check/close icons)
  - Tap Yes/No immediately saves attendance status for that user
  - Visual feedback: selected button is highlighted in green (Yes) or red (No)
  - Loading spinner shown while saving individual attendance

**Action Buttons** (for owner only):
- **Visibility**: Shown to experience owner in appropriate states
- **Buttons**:
  - "Mark In Process": Transitions ACTIVE/JOINED → IN_PROCESS (when experience starts)
  - "Complete Experience": Transitions ACTIVE/JOINED/IN_PROCESS → COMPLETED (when experience ends)
  - "Cancel Experience": Transitions ACTIVE/JOINED/IN_PROCESS → CANCELLED
- **Location**: Conversation screen or experience detail overflow menu

## Workflow Examples

These examples describe complete user journeys that should be covered by integration tests. See [update_system_tests.md](../testing/update_system_tests.md) for test implementation guidelines.

> **Per-item community (#2492).** In every example below, "Owner creates experience" also
> provisions the event's host-only per-item community and shares the event into it (owner
> auto-RSVP'd YES; conversation created there). So a step like "shares with Community A" adds
> a *second* community, and an event's `shared_community_ids` / RSVP counts include the
> per-item community. The integration tests assert against this (e.g. they read the
> per-item community from `item_community_id`, or expect community/RSVP counts +1).

### 1. Experience Completed (Happy Path)

#### Preconditions

- A test community exists
- Four users exist: an owner, participant A, participant B, and participant C, all members of the community

#### Steps

1. **Owner**: Creates experience → `ACTIVE` state
2. **Owner**: Shares experience with community → Creates `CommunityExperience` record
3. **System**: Creates conversation, adds owner as initial participant
4. **Participant A**: RSVPs Yes → Experience transitions to `JOINED` state
5. **System**: Adds Participant A to conversation
6. **Participant B**: Also RSVPs Yes
7. **System**: Adds Participant B to conversation
8. **Participant C**: RSVPs Maybe
9. **System**: Adds Participant C to conversation
10. **Owner**: (Optional) Marks experience as in-process → State changes to `IN_PROCESS`
11. **Owner**: Records attendance (YES for A & B, NO for C) — called before CompleteExperience
12. **Owner**: Marks experience complete → State changes to `COMPLETED`
13. **System**: Generates story card for community feed (inside CompleteExperience, after attendance is already set)

#### Postconditions

- The experience state is `COMPLETED`
- A story card appears at the top of the community feed
- The conversation remains accessible for history
- All RSVPed participants (A, B, C) remain in conversation
- **Notifications sent:**
  - All community members (A, B, C) received "New Event" notification when shared (step 2; `EXPERIENCE_CREATED` broadcast, gated by `notify_new_experiences`)
  - Owner received RSVP notification when Participant A RSVPed Yes (step 4)
  - Owner received RSVP notification when Participant B RSVPed Yes (step 6)
  - Owner received RSVP notification when Participant C RSVPed Maybe (step 8)
  - Yes/Maybe RSVPs minus actor (A, B, C) received "is starting now" push when marked in-process (step 10; `EXPERIENCE_STARTED` event via funnel, gated by `notify_experience_rsvps`, stream-suppression applied)
  - Yes/Maybe RSVPs minus actor (A, B, C) received `EXPERIENCE_COMPLETED` push on completion (step 12; same lifecycle funnel as STARTED/CANCELLED, *not* a community broadcast; gated by `notify_experience_completed`)
- **Impact estimation:**
  - CompleteExperience response includes ImpactEstimate with time_saved.mean approximately equal to config default (120 min) × attendee count (2 Yes RSVPs = 240 min)
  - Community metrics time_banked_minutes has positive mean matching the experience's time_saved

### 2. Experience Cancelled

#### Preconditions

- A test community exists
- Three users exist: an owner, participant A, and participant B
- The owner has created and shared an experience
- Participant A and B have RSVPed Yes (experience is in `JOINED` state)

#### Steps

1. **Owner**: Realizes event can't happen
2. **Owner**: Cancels experience → State changes to `CANCELLED`
3. **System**: Records community event

#### Postconditions

- The experience state is `CANCELLED`
- The conversation remains accessible for history
- **Notifications sent:**
  - All community members (A, B) received "New Event" notification when shared (`EXPERIENCE_CREATED` broadcast, gated by `notify_new_experiences`)
  - Owner received RSVP notification when Participant A RSVPed Yes
  - Owner received RSVP notification when Participant B RSVPed Yes
  - Yes/Maybe RSVPs minus actor (A, B) received cancellation push ("[experience name]" / "Event has been cancelled"; `EXPERIENCE_CANCELLED` event via funnel, gated by `notify_experience_rsvps`, stream-suppression applied; actor excluded)
- **Impact estimation:** Community impact metrics savings are zero (no completed experiences).

### 3. Participant Changes RSVP to No

#### Preconditions

- A test community exists
- Three users exist: an owner, participant A, and participant B
- The owner has created and shared an experience
- Participant A and B have RSVPed Yes (experience is in `JOINED` state)

#### Steps

1. **Participant A**: Changes RSVP from Yes to No
2. **System**: Records the No intention; does NOT remove Participant A from the conversation

#### Postconditions

- Participant A's RSVP intention is `NO`
- Participant A remains in the conversation (membership is not pruned on No, since the conversation is shared across communities)
- Participant B remains in the conversation
- The experience remains in `JOINED` state (at least one Yes/Maybe RSVP remains)
- **Notifications sent:** None (RSVP No does not trigger notifications)

### 4. All Participants Decline

#### Preconditions

- A test community exists
- Two users exist: an owner and a participant
- The owner has created and shared an experience
- The participant has RSVPed Yes (experience is in `JOINED` state)

#### Steps

1. **Participant**: Changes RSVP from Yes to No
2. **System**: Records the No intention; does NOT remove the participant from the conversation

#### Postconditions

- The experience remains in `JOINED` state (the owner's auto-YES RSVP from sharing is still active)
- The participant's RSVP intention is `NO`, but they remain in the conversation
- **Notifications sent:** None (RSVP No does not trigger notifications)

### 5. Cross-Community Sharing

#### Preconditions

- Two test communities exist (Community A and Community B)
- An owner is a member of both communities
- Participant A is a member of Community A only
- Participant B is a member of Community B only

#### Steps

1. **Owner**: Creates experience and shares with Community A → Creates `CommunityExperience` for A
2. **System**: Creates the experience conversation, stores its ID on `Experience.conversation_id`, adds owner as participant
3. **Participant A**: RSVPs Yes in Community A
4. **System**: Adds Participant A to the experience conversation
5. **Owner**: Opens Communities menu in experience overflow, shares with Community B
6. **System**: Creates a separate `CommunityExperience` for B but reuses the existing experience conversation
7. **Participant B**: RSVPs Yes in Community B
8. **System**: Adds Participant B to the same experience conversation
9. **Owner**: Views the experience conversation → sees both Participant A and Participant B

#### Postconditions

- Experience appears in both community feeds with `shared_community_ids: ["A", "B"]`
- A single shared conversation exists (`Experience.conversation_id`), not one per community
- Both participants and the owner share that one conversation
- RSVPs remain community-specific, but the chat thread is common to both communities
- **Notifications sent:**
  - Owner received notification when Participant A RSVPed Yes (step 3)
  - Owner received notification when Participant B RSVPed Yes (step 7)

### 6. Unsharing from Community

#### Preconditions

- An experience is shared with Community A and Community B
- Participants have RSVPed in both communities
- The single experience conversation exists, shared across both communities

#### Steps

1. **Owner**: Opens Communities menu in experience overflow
2. **Owner**: Unchecks Community B to unshare
3. **System**: Deletes the `CommunityExperience` record for Community B (`unshare.go`)

#### Postconditions

- Experience no longer appears in Community B's feed
- Experience still appears in Community A's feed
- The experience conversation is untouched (unshare only deletes the `CommunityExperience` row; it does not touch `Experience.conversation_id` or its participants)
- `shared_community_ids` updated to `["A"]`
- **Notifications sent:** None (unsharing does not trigger notifications)

### 7. Recording Attendance

#### Preconditions

- A test community exists
- Four users exist: an owner, participant A, participant B, and participant C
- An experience exists in any non-terminal state (ACTIVE, JOINED, IN_PROCESS)
- Participant A RSVPed Yes, Participant B RSVPed Yes, Participant C RSVPed Maybe

#### Steps

1. **Owner**: Opens completion modal (experience is in JOINED state — participants have RSVPed)
2. **Owner**: Sees unified "Confirmed Attendees" list with all RSVPs
3. **Owner**: Marks Participant A as attended (YES)
4. **Owner**: Marks Participant B as attended (YES)
5. **Owner**: Marks Participant C as NOT attended (NO)
6. **Owner**: Confirms completion — `RecordAttendance` is called first, then `CompleteExperience`

#### Postconditions

- Participant A has `ATTENDED_STATUS_YES`
- Participant B has `ATTENDED_STATUS_YES`
- Participant C has `ATTENDED_STATUS_NO`
- **Notifications sent:** `RecordAttendance` itself sends no notifications. The other operations in this scenario do:
  - All community members (A, B, C) received "New Event" push when shared (precondition; `EXPERIENCE_CREATED` broadcast)
  - Owner received RSVP push from each of A (Yes), B (Yes), C (Maybe) (precondition)
  - Yes/Maybe RSVPs minus actor (A, B, C) received "is starting now" push when marked in-process (precondition; `EXPERIENCE_STARTED` via funnel, gated by `notify_experience_rsvps`)
  - Yes/Maybe RSVPs minus actor (A, B, C) received `EXPERIENCE_COMPLETED` push on completion (step 6; same lifecycle funnel as STARTED/CANCELLED, *not* a community broadcast; gated by `notify_experience_completed`)

### 8. Direct Completion from JOINED State

This exercises the most common completion flow: the owner wraps up an experience
directly from JOINED state without ever marking it IN_PROCESS. The client calls
`RecordAttendance` then `CompleteExperience` in sequence. This was the flow that
failed in issue #1104 when `RecordAttendance` rejected JOINED state.

#### Preconditions

- A test community exists
- Three users exist: an owner, participant A, and participant B, all members of the community

#### Steps

1. **Owner**: Creates experience → `ACTIVE` state
2. **Owner**: Shares experience with community
3. **Participant A**: RSVPs Yes → Experience transitions to `JOINED` state
4. **Participant B**: RSVPs Yes
5. **Owner**: Records attendance (YES for A, NO for B) — experience is in `JOINED` state
6. **Owner**: Completes experience → State changes to `COMPLETED`

#### Postconditions

- The experience state is `COMPLETED`
- Participant A has `ATTENDED_STATUS_YES`
- Participant B has `ATTENDED_STATUS_NO`
- A story card appears in the community feed (story generation uses the recorded attendance)
- `CompleteExperience` response includes `ImpactEstimate` with populated metrics

### 9. Cross-Community Completion

An event shared to more than one community, where an attendee RSVPs via one
community but the host completes with a *different* community context. Attendance
is event-scoped, so completion must succeed regardless of which community lens
the host records from. Before the fix this 404'd the whole batch (#2690).

#### Preconditions

- Two communities exist (A and C), both containing the host
- An experience is shared to **both** A and C
- An attendee (member of C) RSVPed Yes **via community C**

#### Steps

1. **Owner**: Records attendance scoped to community **A** (not C) — confirms the
   attendee YES
2. **Owner**: Completes the experience

#### Postconditions

- `RecordAttendance` **succeeds** (no 404) — the attendee's RSVP is resolved
  event-scoped and marked attended on its existing community-C row (no duplicate
  row is fabricated in A)
- The experience state is `COMPLETED`
- The attendee shows `ATTENDED_STATUS_YES` in the cross-community RSVP roster
- `CompleteExperience` response includes a populated `ImpactEstimate` that counts
  the attendee

## Data Flow

### Server → Client

**Experience Data:**
- Server: `ExperienceService` (CRUD operations, RSVP management, state transitions)
- Client: `ExperienceRepository` (cached access)
- Caching:
  - `'{experienceId}'` - Single experience by ID
  - `'community:{communityId}:list'` - Community experience lists
  - `'user:list'` - User's created experiences

**Cache Invalidation:**
- Repositories automatically invalidate relevant caches after mutations
- Pattern: `await repository.operation(); await repository.refresh();`
- Single-item cache enables efficient real-time updates from push notifications

### Client State Management

**ExperienceViewModel** manages:
- Experience details and ownership
- RSVP state (user's intention, Yes/Maybe counts)
- Edit mode and media uploads
- Location and time information
- Loading and error states

**Key ViewModel Operations:**
- `initialize`: Loads experience and user-specific RSVP data
- `loadExperienceDetails`: Fetches full experience with RSVPs
- `updateRSVP`: Changes user's RSVP intention
- `saveChanges`: Updates experience details (owner only)
- `deleteExperience`: Removes experience (owner only)
- `shareWithCommunity`: Shares experience with community

**TimeModalNotifier** (AsyncNotifier, autoDispose family by experienceId) manages:
- Time proposals list with voting state
- Lock/unlock (confirm/unconfirm) state
- Propose form state (date, time, duration)
- Selected proposal for finalize flow
- Event time hero card data
- Optimistic vote updates with rollback on failure

## Business Rules

### Validation Rules

1. **Experience Ownership**: Only owners can update, share, delete, or change state
2. **RSVP Constraints**:
   - Can only RSVP to experiences in ACTIVE, JOINED, or IN_PROCESS states (late joins allowed for in-progress events)
   - Max participants enforced when RSVPing YES (if specified)
   - User can change RSVP multiple times
   - RSVPs are community-specific (same user can RSVP differently in different communities)
3. **State Transitions**: Enforced by state machine, owner-only operations
4. **Community Membership**: Users must be community members to view/RSVP to that community's experience
5. **Conversation Access**: Users can read/post in the experience conversation regardless of RSVP status
6. **Attendance Recording**: Available in any valid experience state — the client records attendance before calling CompleteExperience
7. **Lifecycle Control**: Owner controls all state transitions
8. **Multi-Community Sharing**: Same experience can be shared to multiple communities; all shares reuse one experience-scoped conversation

### Side Effects

1. **State Transitions**:
   - Automatic ACTIVE → JOINED when first RSVP Yes/Maybe
   - Owner-driven JOINED → IN_PROCESS → COMPLETED/CANCELLED
2. **Conversation Creation**: Automatic on the first share of an experience
   - Creates one experience-scoped conversation
   - Stores conversation ID in `Experience.conversation_id`
   - Adds owner as initial participant
   - Reused (not recreated) on subsequent shares to other communities
3. **Conversation Joining**: Automatic when user RSVPs Yes/Maybe to the experience
4. **Conversation Leaving**: None — changing RSVP from Yes/Maybe to No does not remove the user from the conversation
5. **Conversation Archiving**: Implicit when experience is done (COMPLETED/CANCELLED) and no unread messages
6. **Notifications**: See [Notifications](#notifications) section below
7. **Community Events**: Recorded for activity feed (share, state changes)

### Notifications

Push notifications are sent for specific experience events to keep users informed. See [push_notifications.md](../push_notifications.md) for architecture details.

<!-- Notification counts consolidated in PR #1593 (fixes #1552): lifecycle pings now flow through
     RecordCommunityEventAndNotify with actor exclusion and stream-suppression. -->

| Event | Recipient | Notification Content | Toggle |
|-------|-----------|---------------------|--------|
| Experience shared | All community members minus actor | "New Event" / "[user] created [experience name]" | `notify_new_experiences` |
| RSVP Yes | Experience owner | "RSVP to [name]" / "[user] is attending your event" | — |
| RSVP Maybe | Experience owner | "RSVP to [name]" / "[user] might attend your event" | — |
| Experience started (`EXPERIENCE_STARTED`) | Yes/Maybe RSVPs minus actor | "[experience name]" / "is starting now" | `notify_experience_rsvps` |
| Experience completed (`EXPERIENCE_COMPLETED`) | Yes/Maybe RSVPs minus actor | "Event Completed" / "[experience name] wrapped up" | `notify_experience_completed` |
| Experience cancelled (`EXPERIENCE_CANCELLED`) | Yes/Maybe RSVPs minus actor | "[experience name]" / "Event has been cancelled" | `notify_experience_rsvps` |

All three lifecycle notifications (STARTED, COMPLETED, CANCELLED) go through `RecordCommunityEventAndNotify`, applying stream-suppression and actor exclusion uniformly, and all target Yes/Maybe RSVPs minus the actor. Every toggle category is opt-out (default: on) and is scoped per-(user, community). See `docs/issues/1107-per-community-notifications.md` for the preference design.

**Events that do NOT trigger notifications:**

- RSVP No - no notification (user is declining)

**Deep linking:** All notifications include `conversation_id` for direct navigation to the experience conversation.

## Advanced Features

### AI-Powered Content Generation

**GenExperience Operation:**
- Uses LLM to extract event details from:
  - Natural language prompts (e.g., "yoga class Tuesday morning")
  - Uploaded flyer/poster images (OCR + extraction)
- Generates: Name, description, suggested time, location hints, tags
- Returns confidence levels for time extraction
- Enables quick experience creation with minimal user input

**ConvertInformalTime Operation:**
- Real-time LLM parsing of natural language time descriptions
- Converts fuzzy times to structured date/time data
- Examples:
  - "tomorrow afternoon" → SpecificTime (next day, 2pm inferred)
  - "next weekend" → TimeRange (Saturday-Sunday)
  - "Friday at 6pm" → SpecificTime (explicit 6pm)
- Used by calendar-chips time picker for interactive time selection

### Smart RSVP Navigation

The UI implements intelligent navigation based on RSVP state:
- **Already RSVPed Yes/Maybe + Conversation exists**: Tap RSVP chip → Navigate directly to conversation
- **Not RSVPed or No RSVP**: Tap RSVP chip → Show RSVP modal
- **After RSVPing Yes/Maybe**: Auto-navigate to conversation (if created)

This creates a seamless flow from discovering an experience to coordinating in the group chat.

### Max Participants Enforcement

Experiences can optionally specify a max participant count:
- Enforced server-side when RSVPing YES
- Users can still RSVP MAYBE or NO when full
- Owner's RSVP does not count toward limit
- Returns `ResourceExhausted` error when full

## Error Handling

**Server:**
- Uses Connect-style error codes (NotFound, PermissionDenied, FailedPrecondition, InvalidArgument, ResourceExhausted)
- Validates state transitions and returns descriptive errors
- Validates max participants and community membership
- Logs all operations for debugging

**Client:**
- Wraps service exceptions in user-friendly messages
- ViewModels store error state for UI display
- Repository operations fail gracefully with cache invalidation
- Toast notifications for operation success/failure

## Testing Strategy

**Server:**
- Unit tests for state machine logic (`lifecycle.go`)
- RSVP tests for conversation creation and joining (`rsvp_test.go`)
- Authorization tests (owner-only operations)
- Sharing tests (`share_test.go`)
- Query tests (`get_test.go`)

**Client:**
- Repository tests with mocked services
- ViewModel tests for business logic
- Widget tests for UI interactions
- RSVP flow tests (modal, chip, navigation)

## Future Considerations

1. **Recurring Experiences**: Currently no support for repeating events
2. **Waiting Lists**: No waitlist mechanism when max participants reached
3. **Sub-Events**: No support for multi-day events with sub-activities
4. **Reminder System**: No automated reminders before event time
5. **Post-Event Feedback**: No rating/review system after completion
6. **Expense Tracking**: No built-in cost sharing or expense tracking
7. **Private Experiences**: All experiences are community-wide (no invite-only)
8. **Co-Hosts**: Only single owner, no co-organizer roles
9. **RSVP Deadline**: No deadline cutoff for RSVPs
10. **Calendar Integration**: No export to device calendar (iCal, Google Calendar)
12. **Real-Time Time Modal Sync**: Currently uses optimistic UI; no WebSocket live updates for concurrent voting
