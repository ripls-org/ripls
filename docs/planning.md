---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Needs & Contributions — the shared planning layer for Experiences and Requests where a planner posts needs with slot counts and participants claim or freely volunteer; covers scopes, permissions, slot claiming, and soft-delete history.
  globs: [server/services/experience/needs.go, server/services/experience/contributions.go, server/services/request/needs.go, server/services/request/contributions.go, app/lib/presentation/widgets/needs/**, app/lib/presentation/viewmodels/needs_scope.dart]
  triggers: [needs, contributions, slots, claim, volunteer, plan-tab, i-got-this]
  lens: [domain, client, server]
  domain: planning
freshness:
  verified_commit: "11d9d200b"
  verified_on: "2026-07-11"
---
# Planning: Needs & Contributions

## Overview

Needs and Contributions are the collaborative planning layer shared by **Experiences** and **help Requests** — the "who's bringing what / who's helping with what" feature. A planner posts things a group still needs; participants volunteer to cover them. Together these form a live, shared list that helps people self-organize without any central planner.

The same planning system is used in two scopes:

| Scope | Context | Permissions |
|-------|---------|-------------|
| **Experience** | "Who's bringing what to the event" — attached to a single Experience | Owner or YES/MAYBE RSVP can post Needs or add Contributions |
| **Request** | "Who's helping with this request" — attached to a standalone help Request | Any community member can offer; only the requester can post Needs |

Both scopes share the same UI widgets (`NeedsSection`, `NeedsActions`), the same modal chrome, and the same chat integration pattern. Scope-specific behavior — permission checks, auto-RSVP, API endpoints — is handled internally by the `NeedsScope` sealed class.

## Architecture Principles

**Scoped to a single entity:** Every Need and Contribution belongs to exactly one Experience or Request. They cannot be transferred, shared, or referenced outside it.

**Slots enable concurrent claiming:** A Need carries a slot count (1–20). Each claim atomically decrements the remaining count. Multiple participants can claim the same Need simultaneously up to its slot capacity.

**Contributions are bidirectional:** A Contribution can be a direct response to a Need (claimed) or a freestanding volunteer that nobody asked for. Both appear in the same list. When a Contribution is created from a Need, the Need's name and note are snapshotted onto it at claim time so the audit trail is preserved even if the Need is later removed.

**Soft deletes only:** Removed Needs and Contributions are soft-deleted. Records are retained for history and the chat message record.

**Mutations drive cache invalidation:** After any write, the server's latest state is force-fetched, bypassing the client cache. The client never applies optimistic local state.

**Terminal entities are immutable:** Once a scope reaches a terminal state (COMPLETED/CANCELLED for Experiences, FULFILLED/CANCELLED for Requests), no new Needs or Contributions can be added, claimed, or removed.

## User Flows

### Posting a Need

The requester/owner opens the Plan tab and taps "Request Something" (Experience) or "Break it down" (Request). They enter a name (required), an optional note, and a slot count. The Need appears in the "Still Needed" list. If a note was provided it posts to chat as a user message (attributed to the actor); otherwise a system message is posted.

Suggestion chips from the entity's AI-generated context appear when the list is empty, giving a one-tap shortcut to common needs.

### Claiming a Need ("I Got This" / "Offer something")

A participant taps a Need to view its detail sheet. If slots remain and they are eligible, they can claim it. This opens a sheet showing the Need's name, the original note in a framed NOTE section, and an optional note field. On confirmation the slot count is atomically decremented and a linked Contribution is created.

For Experience scope: participants without an RSVP are automatically RSVPed YES (or MAYBE if they toggle it) before the claim is recorded.

### Adding a Freestanding Contribution

A participant taps "Offer Something" and enters a title and optional description. The Contribution is added directly without consuming any Need slot.

### Editing a Contribution

Only the contributor who created a Contribution can edit it. Tapping their own Contribution chip (from the Plan tab or a chat pill) opens an edit sheet. From there they can modify the title/description, release a claimed slot back to the Need, or remove the Contribution.

### Removing a Need

Only the participant who posted a Need can remove it. Removal is soft — existing Contributions retain their snapshot of the original name and note.

## Permission Model

| Scope | Who can post a Need | Who can add a Contribution |
|-------|---------------------|---------------------------|
| Experience | Owner or YES/MAYBE RSVP | Owner or YES/MAYBE RSVP |
| Request | Requester only | Any community member |

Beyond these base checks, certain operations are further restricted across both scopes:

| Operation | Restriction |
|-----------|-------------|
| Remove a Need | Only the original proposer |
| Edit a Contribution | Only the original contributor |
| Remove a Contribution | Only the original contributor |
| Unclaim a slot | Only the original contributor |

All mutations are blocked when the parent entity is in a terminal state.

## Data Model

### Needs

A Need holds the thing a group still requires. Key properties:

- **Name** — what is needed (required)
- **Note** — optional context for whoever claims it
- **Slots / Slots Remaining** — total capacity and how many remain; decremented atomically on claim, restored on unclaim
- **Proposer** — the participant who posted the Need

### Contributions

A Contribution records what a participant is providing. Key properties:

- **Title** — what they are bringing/doing (required)
- **Description** — optional detail
- **Contributor** — the participant who created the Contribution
- **Source Need** — optional link to the Need this Contribution fulfills
- **Original Need Name / Note** — snapshot at claim time; preserved even if the Need is later removed
- **Transfer link** (Request scope, #2702) — a gear-linked contribution can
  escalate into a real loan/giveaway offer; `transfer_id` records the child
  transfer, and `accepted_at_unix_sec` records the requester's "this one works"
  acceptance. Cancelling the transfer pre-handoff removes the contribution
  and reopens the need slot (server-side, bus-driven).

### API Responses

The API enriches both models before sending them to the client. Bare user IDs are resolved to full user objects in a single batch fetch. The client receives display-ready data and never makes separate user lookups.

## Atomic Slot Management

Slot claiming uses a conditional update: `SET slots_remaining = slots_remaining - 1 WHERE slots_remaining > 0`. The database reports whether a row was updated. If `slots_remaining` was already zero the claim fails immediately rather than going negative.

Unclaiming uses an unconditional increment: `SET slots_remaining = slots_remaining + 1`. If the source Need has already been removed, the increment is a no-op.

## Chat Integration

Mutations post messages into the entity's conversation. When a note or description is provided the text posts as a **user message** (attributed to the actor) so participants can react. Bare adds without text fall back to a system message.

| Action | Note provided? | Chat output |
|--------|---------------|-------------|
| Need added | Yes | User message with the note text; need pill set |
| Need added | No | System message: "[Name] added a need: [name]" |
| Need removed | — | System message: "[Name] removed the need: [name]" |
| Need claimed | Yes | User message with the note text; contribution pill set |
| Need claimed | No | System message: "[Name] is bringing/helping with: [name]" |
| Contribution added | Yes | User message with the description; contribution pill set |
| Contribution added | No | System message: "[Name] is bringing/offering: [title]" |
| Contribution removed | — | System message: "[Name] removed their contribution: [title]" |

**Per-entity coalesce keys:** every system message insert passes the relevant `need_id` or `contribution_id` as its coalesce key so that batch adds produce separate chat cards and add-then-remove of the same entity coalesces in place.

## Server Implementation

| Component | Purpose |
|-----------|---------|
| Experience needs service | RPC handlers for Experience-scoped mutations; enforces RSVP and terminal-state checks |
| Request needs service | RPC handlers for Request-scoped mutations; enforces requester-only and terminal-state checks |
| List handlers | `ListExperienceNeedsAndContributions` / `ListRequestNeedsAndContributions`; batch-fetch all user records in one query |
| Storage helpers | Atomic slot decrement/increment; soft-delete helpers |
| `SystemMessageWriter` | Shared library in `server/chat`; provides `InsertSystemMessage` and `InsertUserMessageOnBehalfOf` |

## Client Implementation

### `NeedsScope` sealed class

`NeedsScope` is a Dart Freezed sealed class with two variants:

- **`ExperienceNeedsScope`** — carries `experienceId`, `communityId`, `currentUserId`, `ownerId`, `experienceName`, `isTerminal`, `isRsvped`, `isRsvpedMaybe`
- **`RequestNeedsScope`** — carries `requestId`, `communityId`, `currentUserId`, `requestOwnerId`, `isTerminal`, `isOwner`

Passing a `NeedsScope` to `NeedsSection` or `NeedsActions` is all that is required to route the correct provider, permission checks, and sheets.

### Widget and action classes

| Component | Purpose |
|-----------|---------|
| `NeedsSection` | `ConsumerWidget` that renders the full planning UI (Still Needed list, Who's Bringing/Offering list, suggestion chips, action buttons) for either scope |
| `NeedsActions` | Shared handler class used by both the Plan tab and the chat pills; tapping a Need or Contribution from either surface opens the same sheet with the same permissions |
| `experienceNeedsProvider` | Riverpod `autoDispose.family` notifier scoped by experience ID |
| `requestNeedsProvider` | Riverpod `autoDispose.family` notifier scoped by request ID |
| Shared sheets (`app/lib/presentation/widgets/needs/`) | Used by `NeedsActions` for **both** scopes: `NeedsProposeSheet`, `NeedsPickerModal`, `NeedsVolunteerSheet`, `NeedsClaimConfirmSheet`, `NeedsAddToListSheet`, `NeedsArchivedSheet`, and the `showNeedsManageMenu` entry point — see [`docs/client/needs.md`](client/needs.md) for the full surface map |
| Experience-specific sheets (`app/lib/presentation/widgets/experience/needs/`) | `ViewNeedSheet`, `AddContributionSheet`, `BatchAddNeedSheet`, `BatchAddContributionSheet`, `EditContributionSheet`, `ViewContributionSheet` |

### Unified modal design

The client-side modal layer is documented in
[`docs/client/needs.md`](client/needs.md) — surfaces, lifecycle, scope
dispatch, and the parity contract between Experience and Request scopes.
Composed on `GlassSheet` + `NeedsSheetChrome`; shared primitives
(`NeedsClaimRow`, `NeedsSuggestionGrid`) live under
[`app/lib/presentation/widgets/needs/`](../app/lib/presentation/widgets/needs/).

### Caching

Reads go through the project's standard cache layer keyed on entity ID. After any mutation the cache entry is force-invalidated and fresh server state is fetched before the UI updates.

### Loading Deduplication

Each view model schedules the initial data load during its build phase. If the UI calls `refresh()` before that load completes, the notifier awaits the already-in-flight request instead of issuing a duplicate.

### RSVP Auto-enrollment (Experience scope only)

If a participant attempts to claim a Need or add a Contribution in an Experience without an existing RSVP, the client automatically submits a YES RSVP before sending the mutation. If the RSVP fails the mutation is aborted and the error surfaced.

## Trade-offs

- **Force-refresh after mutations:** The client always re-fetches from the server after a write rather than applying optimistic local changes. Guarantees accurate slot counts but adds a round-trip after every action.
- **Snapshot on claim:** Snapshotting the Need name/note onto the Contribution at claim time means the contributor always sees the original context even if the Need is edited or removed. The trade-off is that post-claim edits to a Need name are not reflected on existing Contributions.
- **Request scope: requester-only Needs posting.** Only the person who created a Request can post Needs on it. Any community member can offer. This matches the asymmetric ownership of a help Request.
- **Experience scope: participant-only mutations.** Requiring a YES or MAYBE RSVP before posting keeps the planning list grounded in confirmed participants.

---

**Last Updated:** 2026-04-21
**Status:** Production-ready — Experience and Request scopes both live; unified `NeedsSection`/`NeedsActions` widgets; chat-pill and Plan-tab tap consolidation complete
