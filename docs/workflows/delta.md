---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Side-by-side differences between the four workflows (loans, giveaways, requests, experiences) — state machines, conversation models, control/selection, and per-workflow unique features.
  globs: [server/services/transfer/**, server/services/request/**, server/services/experience/**, app/lib/data/repositories/**, app/lib/presentation/screens/gear/**, app/lib/presentation/screens/request/**, app/lib/presentation/screens/experience/**]
  triggers: [workflow, loan, giveaway, request, experience, state-machine, conversation, rsvp, transfer]
  lens: [workflow, domain]
freshness:
  verified_commit: "11d9d200b"
  verified_on: "2026-07-11"
---
# Workflow Comparison: Loans, Giveaways, Requests, and Experiences

This document focuses exclusively on the **differences** between the four main workflows in Ripls. For common patterns shared across all workflows, see the individual workflow documentation.

## Quick Reference

| Aspect | Loans | Giveaways | Requests | Experiences |
|--------|-------|-----------|----------|-------------|
| **Direction** | Owner → Borrower (temporary) | Owner → Recipient (permanent) | Requester ← Helpers | Owner → Attendees (event) |
| **Who initiates** | Borrower expresses interest | User expresses interest | Requester posts need | Owner creates & shares |
| **Conversation model** | Perpetual gear conversation (community-wide) | Perpetual gear conversation (community-wide) | Single request-scoped chat, all offerers (reused across communities) | Single experience-scoped chat (reused across communities) |
| **Conversation storage** | `Gear.conversation_id` | `Gear.conversation_id` | `Request.conversation_id` | `Experience.conversation_id` |
| **State sequence** | INTEREST → SELECTED → ACTIVE → COMPLETED/CANCELLED | INTEREST → SELECTED → COMPLETED/CANCELLED | ACTIVE → OFFERS_RECEIVED → FULFILLED/CANCELLED | ACTIVE → JOINED → IN_PROCESS → COMPLETED/CANCELLED |
| **Item tracking** | Gear with state changes | Gear with ownership transfer | No item of its own — but gear-backed offers spawn child transfers (#2702) | No item (event/activity) |
| **Participation** | Multiple borrowers (one selected) | Multiple interested (one selected) | Multiple helpers (no selection) | Multiple attendees (RSVPs) |
| **Withdrawal behavior** | Cancels transfer, user stays in conversation | Handled by `WithdrawInterest` | `WithdrawOffer` removes from conversation | Change RSVP to NO removes from conversation |
| **Selection** | Owner selects recipient | Owner selects recipient | Requester marks fulfilled; optional accept on gear-backed offers (#2702) | No selection (open RSVPs) |
| **Control** | Either party can cancel/complete | Either party can cancel/complete | Only requester can cancel/fulfill | Only owner can change state |
| **Chip pattern** | INQUIRIES (n) with check/outline icon | INQUIRIES (n) with check/outline icon | OFFER HELP (n) with check/outline icon | RSVP (n) with check/outline icon |
| **Status badges** | Conversation screen only (not modal) | Conversation screen only (not modal) | Conversation screen only (if applicable) | Conversation screen only (not modal) |

## Conversation Models - Key Differences

**Loans vs Giveaways**: Both use perpetual community-wide conversations
- **Loans**: Borrowers remain in conversation after withdrawal; conversation persists across all loan cycles
- **Giveaways**: Conversation archived when giveaway completes (CommunityGear deleted)

**Requests**: Single request-scoped conversation (group chat for all offerers)
- One conversation, created when the request is born into its per-item community (#2492) and **reused** across every community it's later shared with (stored on `Request.conversation_id`; the `CommunityRequest.conversation_id` column is deprecated)
- Offerers can withdraw and leave conversation (unlike loans where users stay)

**Experiences**: Single experience-scoped conversation
- One conversation is created on first share and **reused** when the experience is shared to additional communities (stored on `Experience.conversation_id`; the `CommunityExperience.conversation_id` column is deprecated)
- Like loans/giveaways, which also share one conversation per item across communities (the per-community-isolated model was reversed)

## State Machines - Key Differences

```
Loans:        INTEREST_EXPRESSED → RECIPIENT_SELECTED → ACTIVE → COMPLETED
Giveaways:    INTEREST_EXPRESSED → RECIPIENT_SELECTED → COMPLETED
Requests:     ACTIVE ↔ OFFERS_RECEIVED → FULFILLED
Experiences:  ACTIVE → JOINED → IN_PROCESS → COMPLETED
```

**Unique characteristics:**
- **Loans only**: ACTIVE state (item is currently out on loan)
- **Requests only**: Bidirectional transition (ACTIVE ↔ OFFERS_RECEIVED) for state reversion when last offer withdrawn
- **Experiences only**: IN_PROCESS state (event happening now)
- **Terminal state naming**: Requests use FULFILLED vs COMPLETED for others

## Conversation Creation Timing - Differences

| Workflow | Conversation Created When |
|----------|---------------------------|
| **Loans & Giveaways** | At creation — `SaveGear` provisions the gear's per-item community and shares the gear into it (#2492), creating the gear-scoped conversation; reused when shared with further communities |
| **Requests** | At creation — `SubmitRequest` shares the request into its per-item community (#2492); reused for later shares (single request-scoped conversation) |
| **Experiences** | At creation — `SaveExperience` shares the event into its per-item community (#2492); reused for later shares (single experience-scoped conversation) |

**Key difference**: All workflows now create the conversation upfront. Offers/RSVPs/interest only add participants to the existing conversation. For requests, experiences, and gear this happens at creation, when the item is born into its host-only per-item community (#2492).

## Control Models - Key Differences

| Aspect | Loans & Giveaways | Requests | Experiences |
|--------|-------------------|----------|-------------|
| **Control** | Bilateral (either party) | Unilateral (requester only) | Unilateral (owner only) |
| **Selection** | Required (owner selects) | None (informal) | None (open RSVPs) |
| **Item** | Physical gear | No item (need) | No item (event) |
| **Participation** | 1:1 (selected recipient) | Many:1 (multiple helpers) | Many:1 (multiple attendees) |

## Advanced Features - Unique to Each Workflow

| Feature | Unique To | Notes |
|---------|-----------|-------|
| **AI Generation** | Requests, Experiences | GenRequest, GenExperience from prompts |
| **Image Integration** | Requests, Experiences | Unsplash for Requests; Flyer OCR for Experiences |
| **Time Tracking** | Experiences only | Flexible time model with AI parsing |
| **Max Participants** | Experiences only | Optional attendance limit |
| **Attendance Tracking** | Experiences only | Post-event recording |
| **State Reversion** | Requests only | OFFERS_RECEIVED → ACTIVE when last offer withdrawn |
| **Perpetual Conversations** | Loans only | Users stay after withdrawal; persists across loan cycles |
| **Ownership Transfer** | Giveaways only | Permanent transfer tracked |

## Requests × Transfers: Gear-Backed Offers (#2702)

The one place two workflows deliberately couple: a helper can satisfy a
request with their actual item. The claim escalates into a **child transfer**
(`Transfer.origin_request_id`, born `RECIPIENT_SELECTED`, requester =
recipient), and the two state machines drive each other over the
community-event bus — the handoff auto-fulfills a single-need request, a
fulfilled request stands down its open sibling offers, and cancelling the offer
unwinds the claim. See `workflows/request.md` § Gear-backed offers (the
request side) and `workflows/loan.md` § Gear-Backed Request Offers (the
transfer side).

## Design Rationale - Why These Differences Exist

### State Reversion
- **Requests only**: Broadcast model expects offers to come and go
- **Others**: Once interest/RSVP is expressed, the workflow progresses forward

### Cancellation Permissions
- **Loans/Giveaways**: Bilateral (both parties invested in physical item)
- **Requests**: Requester only (they own the need)
- **Experiences**: Owner only (they organize the event)

### Lifecycle State Granularity
- **Experiences**: Rich lifecycle needed (IN_PROCESS for events happening now)
- **Loans**: Medium lifecycle (ACTIVE for items currently out)
- **Giveaways/Requests**: Simple lifecycle (instant transfer/fulfillment)

## Workflow-Specific Patterns (Differences Only)

**Loans:**
- Users remain in conversation after withdrawal (unique to loans)
- Gear state synchronization (AVAILABLE/UNAVAILABLE)
- Conversation persists across all loan cycles

**Giveaways:**
- Ownership transfer tracking
- Conversation archived when CommunityGear deleted (loans keep conversation)

**Requests:**
- Dynamic conversation participation (users can leave, unlike loans)
- State reversion on withdrawal (unique to requests)

**Experiences:**
- Single experience-scoped conversation reused across communities (like loans/giveaways)
- RSVP intention tracking (YES/MAYBE/NO)
- Attendance recording (post-event)
- Flexible time model with AI parsing

## UI Design Patterns - Differences

### Status Chip Labels
- **Loans/Giveaways**: "INQUIRIES (n)"
- **Requests**: "OFFER HELP (n)"
- **Experiences**: "RSVP (n)"

### Status Badges (Conversation Screens)
- **Loans only**: Includes "Borrowing" (purple) badge for ACTIVE state
- **Giveaways**: No "Borrowing" badge (no ACTIVE state)
- **Requests**: No status badges (no formal selection)
- **Experiences**: RSVP-based badges (YES/MAYBE/NO)

## Summary - Core Distinctions

1. **Loans** = Temporary (ACTIVE state, users stay in conversation after withdrawal)
2. **Giveaways** = Permanent (no ACTIVE state, conversation archived on completion)
3. **Requests** = Broadcast (state reversion, dynamic participation, no selection)
4. **Experiences** = Events (single conversation shared across communities, RSVP system, time tracking)
