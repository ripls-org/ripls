# Generalized Undo — Design Plan

**Issue:** #964 — Generalized Undo functionality for major actions
**Issue plan:** [docs/issues/964-generalized-undo.md](../issues/964-generalized-undo.md)
**Status:** DRAFT — being built iteratively with owner consultation.
**After implementation:** concise summary moves to `docs/undo.md`; this file is archived.

---

## 1. Goals and Non-Goals

### Goals

- A shared undo primitive for state-changing actions that is **server-authoritative where the action has server-side side effects** (chat messages, impact metrics, cascaded mutations) and **client-only where a natural inverse RPC or UI already covers the need**.
- Durable undo on the **story screen** for completion actions that generate a story — so the actor can come back later, look at the story of a loan/giveaway/request/experience they just completed, and undo it from there.
- Snackbar undo on the acting device immediately after action, for server-authoritative actions that don't generate a story.
- System messages corresponding to undone actions are retracted, not left misleading.
- Impact metrics are correct without explicit rollback (derived at read time from current state + stored `ImpactEstimate`).
- Clear UX indication when an action cannot be reversed because of drift.

### Non-Goals (this phase)

- Money / refund handling.
- Admin-authority undo of other users' actions.
- Generic entity-detail undo affordance. Durable surface is the **story screen only**.
- Hard-delete retention windows / sweep jobs. Soft-deleted entities continue to be recoverable as they are today; we are not adding new deletion policy.
- Correction push notifications.

---

## 2. Resolved Product Decisions

- **UX surfaces: two.**
    1. A transient **snackbar** on the acting device immediately after acting. Used by server-authoritative actions that do not generate a story.
    2. A **small, centered "Undo" text link on the story screen** for actions that generate a story. Actor-only. Subordinate to primary story content. The four completion actions (loan, giveaway, request fulfilled, experience) get this surface.
- **No time window.** Undo is allowed any time. The server's drift check (§6.4) is the sole gate — if the entity is still in the post-action state with cascades intact, undo succeeds; otherwise the server returns a typed error explaining why. This removes countdowns, expiry fields, and window-tuning entirely.
- **Drift handling:** if undo can't reverse at attempt time (entity mutated, cascade no longer reversible, already undone), server returns a typed error and the client surfaces a human-readable explanation. No best-effort partial undo.
- **Notifications:** no correction notifications sent on undo. The chat retraction message is the only user-facing signal.
- **Impact metrics:** no explicit rollback — undo clears `ImpactEstimate` + reverts state; metrics re-derive.
- **System messages:** add `DeletedMetadata` to `models.ChatMessage` (storage-only; not exposed on `api.ChatMessage`). Server list/get endpoints filter deleted messages out of results. Emit a new `CHAT_SYSTEM_ACTION_UNDONE` inline. Open a follow-up issue for a future manual-delete UI that reuses the same storage field.
- **`ReversalData` shape:** strongly-typed oneof holding only the shapes the in-scope actions require.
- **Authority:** actor-only.
- **Post-undo story screen:** navigate back to the now-active item screen, which will show the same state it had before the undone action.
- **Join via invite:** client-only. A welcome story exists but does NOT show an undo link.
- **RSVP:** **no undo affordance**. The RSVP selector on the experience screen is the natural way to change RSVP; existing dedup handles repeated system chat messages.
- **Audit results (Phase 2 wiring):** `CompleteExperience`, `SelectOffer`, and experience collab-list actions (add/remove need, claim need, add/remove contribution) do not emit `CommunityEvent` today. All will be wired in Phase 2.
- **Story lookup:** `Story` gains an optional `community_event_id` (storage-only) at creation time, so the story-screen Undo link is a direct read. Server checks `actor_id` match before populating `UndoableAction` on the api response — non-actors never see the undo link.
- **Per-action undo RPCs:** each undoable action has its own specific RPC (`UndoCompleteLoan`, `UndoStartLoan`, etc.). Not a single generic dispatcher. All take `community_event_id` for the actual undo work.

---

## 3. Action Inventory and Classification

### 3.1 UX surfaces

- **Snackbar (transient).** Shown on the acting device immediately after a state-changing action. Fades after ~4 seconds. Used for server-authoritative actions that don't produce a story.
- **Story screen "Undo" link (persistent).** Small centered text link ("↶ Undo"), positioned subordinate to the primary story content. Visible only to the actor, and only while the server would actually accept an undo (drift check would pass and no retraction already exists). Tapping it calls the same undo RPC as the snackbar and either succeeds (navigate back to the now-active item screen) or surfaces a drift error inline.

### 3.2 Classification

> Classifications:
>
> - **Client-only** — existing inverse RPC or UI already covers the semantics; no server undo plumbing. May or may not show a snackbar.
> - **Server-snackbar** — server-authoritative undo via a specific undo RPC; retracts system chat message and emits a retraction `CommunityEvent`. Snackbar is the only UX surface.
> - **Server-story** — same server mechanism as `Server-snackbar`, plus the action generates a story where we add a small "Undo" link visible to the actor.
> - **Retention-restore** — soft-deleted create/destroy action with an existing restore path; UI surface under Settings. No new hard-delete policy in this phase.
> - **Irreversible** — flagged at action time with `ConfirmIrreversibleActionDialog`; no undo path.
>
> *Emits `CommunityEvent` today?* — confirmed in audit.

#### 3.2.1 Transfer / Loan lifecycle

| Action | Emits `CommunityEvent`? | Cascade? | Story? | Classification | Notes |
| --- | --- | --- | --- | --- | --- |
| Express interest | Yes (`TRANSFER_INTEREST_EXPRESSED`) | No | No | **Client-only** | Inverse is `CancelTransfer` from INTEREST_EXPRESSED. Chat pair `JOINED`/`LEFT` is acceptable "changed their mind" framing. |
| Withdraw interest | Yes (`TRANSFER_INTEREST_WITHDRAWN`) | No | No | **Client-only** | Inverse of above. |
| Select recipient | Yes (`TRANSFER_RECIPIENT_SELECTED`) | No | No | **Server-snackbar** | State-machine loosening + retraction. |
| Start loan (ACTIVE) | Yes (`TRANSFER_ACTIVE`) | Yes (gear → UNAVAILABLE) | No | **Server-snackbar** | `loan_start` variant. |
| Mark returned / complete loan | Yes (`TRANSFER_COMPLETED`) | Yes (gear → AVAILABLE, clears `ImpactEstimate`) | **Yes** | **Server-story** | `loan_completion` variant. |
| Complete giveaway | Yes (`TRANSFER_COMPLETED`) | Yes (full cascade) | **Yes** | **Server-story** | `giveaway_cascade` variant. |
| Cancel transfer | Yes (`TRANSFER_CANCELLED`) | Varies | No | **Server-snackbar** | `transfer_cancel` variant. |
| Propose pickup time | No emission | No | No | **Client-only** | Re-edit the proposed time. |

#### 3.2.2 Request lifecycle

| Action | Emits `CommunityEvent`? | Cascade? | Story? | Classification | Notes |
| --- | --- | --- | --- | --- | --- |
| Submit request | Yes (`REQUEST_CREATED`) | No | No | **Retention-restore** | Soft-delete + existing restore path. |
| Offer help | Yes (`REQUEST_OFFER_MADE`) | No | No | **Client-only** | `WithdrawOffer` inverse. |
| Withdraw offer | Yes (`REQUEST_OFFER_WITHDRAWN`) | No | No | **Client-only** | Re-offer. |
| Select offer | **Wire in Phase 2** | No | No | **Server-snackbar** | Emission not found in audit; add alongside the experience-completion event. |
| Mark fulfilled | Yes (`REQUEST_FULFILLED`) | Yes (clears `ImpactEstimate`) | **Yes** | **Server-story** | `request_fulfillment` variant. |
| Cancel request | Yes (`REQUEST_CANCELLED`) | No | No | **Server-snackbar** | `request_cancel` variant. |

#### 3.2.3 Experience lifecycle

| Action | Emits `CommunityEvent`? | Cascade? | Story? | Classification | Notes |
| --- | --- | --- | --- | --- | --- |
| Create experience | Yes (`EXPERIENCE_CREATED`) | No | No | **Retention-restore** | Soft-delete + existing restore path. |
| RSVP Yes / No / Maybe | Yes | No | No | **Client-only** (no snackbar) | The RSVP selector is always visible on the experience screen — no undo affordance needed. Existing system-message dedup handles repeated flips. |
| Add need / remove need / claim need / add/remove contribution | **Wire in Phase 2** | No | No | **Client-only** | Audit confirmed no emission today. Emission added for activity-feed hygiene, not for undo. Inverses exist in the UI. |
| Complete experience | **Wire in Phase 2** (new `COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED`) | Yes (clears `ImpactEstimate`, attendee count) | **Yes** | **Server-story** | `experience_completion` variant. |

#### 3.2.4 Gear lifecycle

| Action | Emits `CommunityEvent`? | Cascade? | Story? | Classification | Notes |
| --- | --- | --- | --- | --- | --- |
| Create gear | No (private until shared) | No | No | **Retention-restore** | Existing soft-delete / restore. |
| Share to community | Yes (`GEAR_SHARED`) | No | No | **Client-only** | `UnshareGear` inverse. |
| Unshare from community | Yes (`GEAR_UNSHARED`) | No | No | **Client-only** | Inverse. |
| Delete gear | N/A | No (soft-delete) | No | **Retention-restore** | Existing restore. |

#### 3.2.5 Community / membership

| Action | Emits `CommunityEvent`? | Cascade? | Story? | Classification | Notes |
| --- | --- | --- | --- | --- | --- |
| Create community | Yes (`COMMUNITY_CREATED`) | No | No | **Retention-restore** | Existing restore. |
| Accept invite (join) | Yes (`INVITATION_LINK_USED`) | No | Yes (welcome) | **Client-only** | Welcome story shows no undo link — "Leave community" is the inverse path. |
| Leave community | Yes (`MEMBER_LEFT`) | No | No | **Client-only** | Rejoin via invite. |
| Remove member (admin) | — | — | — | Out of scope | No admin-remove UI exists today. Flag if that changes. |

#### 3.2.6 Profile

| Action | Emits `CommunityEvent`? | Cascade? | Story? | Classification | Notes |
| --- | --- | --- | --- | --- | --- |
| Edit profile field | No | No | No | **Client-only** | Re-edit. |
| Upload profile photo | No | No | No | **Client-only** | Re-upload. |
| Delete account | N/A | Yes | No | **Irreversible** | Always flagged irreversible. |

### 3.3 Server-authoritative action summary

**Server-story** (snackbar + story-screen link — 4 actions):

1. Mark returned / complete loan
2. Complete giveaway
3. Mark request fulfilled
4. Complete experience

**Server-snackbar** (snackbar only — 5 actions):

1. Select recipient (transfer)
2. Start loan
3. Cancel transfer
4. Select offer (request)
5. Cancel request

All nine share the same server-side reversal mechanism; only the UX surface differs. Each has its own dedicated undo RPC (§5).

---

## 4. Schema Changes

### 4.1 `proto/ripls/models/community.proto` — `ReversalData`

Add to `models.CommunityEvent`. **Storage-only — never surfaced in the `api` proto.**

```proto
// ReversalData records the minimal prior-state snapshot needed to reverse
// a state-changing action. Storage-only; never returned from RPCs.
message ReversalData {
  oneof variant {
    LoanStartReversal loan_start = 1;
    LoanCompletionReversal loan_completion = 2;
    GiveawayCascadeReversal giveaway_cascade = 3;
    TransferCancelReversal transfer_cancel = 4;
    RequestFulfillmentReversal request_fulfillment = 5;
    RequestCancelReversal request_cancel = 6;
    ExperienceCompletionReversal experience_completion = 7;
    // select-recipient and select-offer need no cascade data — prior
    // state is implied by the event type. No variant required.
  }
}

message LoanStartReversal {
  GearState prior_gear_state = 1;
  optional int64 prior_actual_pickup_unix_sec = 2;
}

message LoanCompletionReversal {
  GearState prior_gear_state = 1;
  optional int64 prior_actual_return_unix_sec = 2;
  optional ImpactEstimate prior_impact_estimate = 3;
}

message GiveawayCascadeReversal {
  GearState prior_gear_state = 1;
  repeated CancelledSiblingTransfer cancelled_siblings = 2;
  repeated string archived_community_gear_ids = 3;
  optional ImpactEstimate prior_impact_estimate = 4;

  message CancelledSiblingTransfer {
    string transfer_id = 1;
    TransferState prior_state = 2;
  }
}

message TransferCancelReversal {
  TransferState prior_state = 1;
  optional GearState prior_gear_state = 2;
}

message RequestFulfillmentReversal {
  RequestState prior_state = 1;
  optional ImpactEstimate prior_impact_estimate = 2;
}

message RequestCancelReversal {
  RequestState prior_state = 1;
}

message ExperienceCompletionReversal {
  ExperienceState prior_state = 1;
  int32 prior_attendee_count = 2;
  optional ImpactEstimate prior_impact_estimate = 3;
}

message CommunityEvent {
  // ... existing fields ...

  // Storage-only reversal metadata. Never populated on api.CommunityEvent.
  // Present on reversible events until consumed; absent on retraction events.
  optional ReversalData reversal_data = 13;

  // Storage-only reference to the system chat message emitted for this
  // action (if any), so undo can soft-delete it directly rather than via
  // time-range search. Never populated on api.CommunityEvent.
  optional string system_chat_message_id = 14;
}
```

### 4.2 New event types

```proto
COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED = 24;  // Audit revealed this is missing today.
COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED = 25; // Audit revealed this may be missing; wire in Phase 2.

// Retraction events
COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED_UNDONE = 26;
COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE_UNDONE = 27;
COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED_UNDONE = 28;
COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED_UNDONE = 29;
COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED_UNDONE = 30;
COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED_UNDONE = 31;
COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED_UNDONE = 32;
COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED_UNDONE = 33;

// Hygiene emissions for experience collab-list actions (client-only; no retraction variants needed)
COMMUNITY_EVENT_TYPE_EXPERIENCE_NEED_ADDED = 34;
COMMUNITY_EVENT_TYPE_EXPERIENCE_NEED_REMOVED = 35;
COMMUNITY_EVENT_TYPE_EXPERIENCE_NEED_CLAIMED = 36;
COMMUNITY_EVENT_TYPE_EXPERIENCE_CONTRIBUTION_ADDED = 37;
COMMUNITY_EVENT_TYPE_EXPERIENCE_CONTRIBUTION_REMOVED = 38;
```

(Exact numeric assignments reconciled against current proto before Phase 2 land.)

### 4.3 `proto/ripls/models/chat.proto`

```proto
message ChatMessage {
  // ... existing fields ...
  // Storage-only. Server list/get endpoints filter deleted messages out
  // of results; api.ChatMessage does not expose this field.
  optional DeletedMetadata deleted = 12;
}
```

```proto
CHAT_SYSTEM_ACTION_UNDONE = 27;  // Replaces a retracted system message inline.
```

```proto
message SystemChatMessage {
  optional string actor_id = 1;
  ChatSystemAction action = 2;
  string description = 3;
  // When action == CHAT_SYSTEM_ACTION_UNDONE, identifies the retracted action
  // so the client can render "Alex undid 'marked returned'".
  optional ChatSystemAction undone_action = 4;
}
```

**Client cache invariant (§7.1):** when an undo occurs, the server emits a new `CHAT_SYSTEM_ACTION_UNDONE` message and soft-deletes the original. Clients receive the new message via the existing conversation stream and refresh the conversation — which refetches through the filtered list endpoint and naturally drops the soft-deleted original. No separate deletion signal is required. Verify during Phase 2 that no client code path reads the cached message directly without going through the refresh.

### 4.4 `proto/ripls/models/story.proto` — `community_event_id` on Story

Storage-only addition:

```proto
message Story {
  // ... existing fields ...
  // For Server-story actions, the CommunityEvent that recorded the action this
  // story is about. **Storage-only** — enables the story-screen Undo link
  // to be wired via direct read (no table join at fetch time). Never
  // exposed on api.Story; clients receive the actionable reference via
  // UndoableAction.community_event_id on the story response instead.
  // Absent for stories not tied to a reversible action (e.g.,
  // welcome-new-member stories).
  optional string community_event_id = N;
}
```

Populated at story creation in `generateStoryForCompletedTransfer`, `generateStoryForFulfilledRequest`, and the experience completion handler.

### 4.5 API proto changes

1. **No `ReversalData` on `api.CommunityEvent`.** Unchanged.
2. **`api.ChatMessage` unchanged.** No `deleted` field — server filters soft-deleted rows before returning.
3. **Story API response** gains an optional `UndoableAction`, populated only for the actor on a story whose `community_event_id` points at a still-reversible event:

    ```proto
    message UndoableAction {
      // The CommunityEvent id to pass back on undo.
      string community_event_id = 1;
      // Label (e.g., "Undo mark returned"). Server-rendered so client
      // doesn't mirror every action-type string.
      string label = 2;
    }
    ```

    No expiry field — undo has no time window.

4. **Error detail proto** (client-facing, so lives under `api/`). Placement: `proto/ripls/api/undo.proto` (new file) so the enum and message are co-located and importable by each service's proto:

    ```proto
    syntax = "proto3";
    package ripls.api;

    enum UndoFailureReason {
      UNDO_FAILURE_REASON_UNSPECIFIED = 0;
      UNDO_FAILURE_REASON_ALREADY_UNDONE = 1;
      UNDO_FAILURE_REASON_ENTITY_CHANGED = 2;
      UNDO_FAILURE_REASON_CASCADE_UNRECOVERABLE = 3;
      UNDO_FAILURE_REASON_NOT_ACTOR = 4;
      UNDO_FAILURE_REASON_WRONG_EVENT_TYPE = 5;
    }

    message UndoErrorDetail {
      UndoFailureReason reason = 1;
      // Human-readable explanation, server-rendered.
      string user_message = 2;
    }
    ```

    Server attaches this as a Connect error detail on undo RPC failure. Client unpacks via the generated error-detail API and surfaces `user_message` to the user.

5. **Server-authoritative action responses** return `community_event_id` (for immediate snackbar wiring):
    - Transfer: `StartLoanResponse`, `CompleteTransferResponse`, `CancelTransferResponse`, `SelectRecipientResponse`.
    - Request: `MarkRequestFulfilledResponse`, `CancelRequestResponse`, `SelectRequestOfferResponse`.
    - Experience: `CompleteExperienceResponse`.

All additive; no wire-break risk.

---

## 5. RPC Shape — One Per Undoable Action

Nine specific undo RPCs, one per server-authoritative action. All take `community_event_id`.

```proto
// transfer_service.proto
rpc UndoSelectRecipient(UndoSelectRecipientRequest) returns (UndoSelectRecipientResponse);
rpc UndoStartLoan(UndoStartLoanRequest) returns (UndoStartLoanResponse);
rpc UndoCompleteLoan(UndoCompleteLoanRequest) returns (UndoCompleteLoanResponse);
rpc UndoCompleteGiveaway(UndoCompleteGiveawayRequest) returns (UndoCompleteGiveawayResponse);
rpc UndoCancelTransfer(UndoCancelTransferRequest) returns (UndoCancelTransferResponse);

// request_service.proto
rpc UndoSelectOffer(UndoSelectOfferRequest) returns (UndoSelectOfferResponse);
rpc UndoMarkRequestFulfilled(UndoMarkRequestFulfilledRequest) returns (UndoMarkRequestFulfilledResponse);
rpc UndoCancelRequest(UndoCancelRequestRequest) returns (UndoCancelRequestResponse);

// experience_service.proto
rpc UndoCompleteExperience(UndoCompleteExperienceRequest) returns (UndoCompleteExperienceResponse);

// All requests have the same single-field shape:
message UndoCompleteLoanRequest { string community_event_id = 1; }
// (analogous for the other eight)
```

Rationale: specific RPCs make the API surface self-documenting, each one's authorization / drift logic lives next to the forward action it reverses, metrics and logs are grouped naturally, and future auditing of "who undid what" is trivial.

Retention-restore restore endpoints stay as whatever restore mechanism already exists per entity.

---

## 6. Server Flow

For a server-authoritative action:

1. Action executes: service emits `CommunityEvent` with the appropriate `ReversalData` variant populated **and** the emitted `system_chat_message_id` recorded on the event. Chat system message is sent normally.
2. Client shows snackbar on the acting device.
3. For Server-story actions, the story is created with `community_event_id` set; the story fetch response populates `UndoableAction` when the requester is the actor and a drift check would still pass.
4. Undo can be triggered from either surface. Client calls the specific `Undo*` RPC with `community_event_id`.
5. Server:
    a. Loads the `CommunityEvent`. Validates — in this order, failing fast with a typed reason:
        - **Exists.** If not, `NotFound`.
        - **Actor match.** `event.actor_id == authInfo.UserID`; else `UNDO_FAILURE_REASON_NOT_ACTOR`.
        - **Event type matches RPC.** Each `Undo*` RPC is bound to a specific `CommunityEventType` (e.g., `UndoCompleteLoan` only accepts `TRANSFER_COMPLETED` where `transfer_type == LOAN`; `UndoCompleteGiveaway` only accepts `TRANSFER_COMPLETED` where `transfer_type == GIVEAWAY`; `UndoSelectRecipient` only accepts `TRANSFER_RECIPIENT_SELECTED`; etc.). Mismatch → `UNDO_FAILURE_REASON_WRONG_EVENT_TYPE`. This check runs **before** any entity load to prevent cross-action confusion.
        - **Not already undone.** No retraction event already exists referencing this `community_event_id`; else `UNDO_FAILURE_REASON_ALREADY_UNDONE`.
    b. Loads the target entity.
    c. **Drift check:** validates the entity is still in the post-action state implied by the event. If not, returns `FailedPrecondition` with a typed reason (§6.2). No partial undo.
    d. Applies the inverse transition using `reversal_data`.
    e. Clears stored `ImpactEstimate` where the variant restores a prior one.
    f. Soft-deletes the referenced `ChatMessage`.
    g. Emits a new `*_UNDONE` `CommunityEvent` (same topic; actor = undoer; no `reversal_data`).
    h. Emits a new `SystemChatMessage` with `CHAT_SYSTEM_ACTION_UNDONE` and `undone_action` set.

### 6.1 Drift error reporting

Server returns `connect.CodeFailedPrecondition` with `UndoErrorDetail` as a Connect error detail. The `UndoFailureReason` enum and `UndoErrorDetail` message are defined in `proto/ripls/api/undo.proto` (see §4.5 item 4).

Client maps to a toast (snackbar path) or inline error (story-screen path) using `user_message` verbatim.

### 6.2 State-machine loosening

A separate internal `undoTransitionTransferState` (and analogs) bypasses the public validator. Keeps `isValidTransferTransition` readable and prevents accidental loosening elsewhere. Request has no formal state machine today; we add a small `undoTransitionRequestState` helper in Phase 2.

### 6.3 Looking up the event

- **By event id** (undo RPC takes `community_event_id`): direct `storage.GetByID`.
- **From a story** (server populating `UndoableAction` on story response): direct read via `Story.community_event_id`. Server then verifies:
    1. The requesting user matches `CommunityEvent.actor_id` (authorization — non-actors never see the undo link).
    2. No retraction event exists referencing this `community_event_id`.
    3. A drift check against the current entity state would pass (if it wouldn't, we don't offer the button — better to hide the affordance than let the user tap and get an error).

    Only when all three hold, `UndoableAction` is populated. Otherwise the field is absent and the client hides the link.

### 6.4 Concurrency

Drift check uses optimistic compare-then-write: load entity, verify it matches the post-action snapshot for this event, apply inverse in the same transaction. Parallel mutation between load and apply causes the `Update` to fail on version mismatch (existing pattern); we return `UNDO_FAILURE_REASON_ENTITY_CHANGED`. No new locking.

### 6.5 Logging

Each undo RPC uses `logging.LoggerWithContext(ctx)` per project conventions. Baseline log fields:

- `operation` = `"undo_" + snake_case_rpc_name` (e.g., `"undo_complete_loan"`).
- `actor_id` = `authInfo.UserID`.
- `community_event_id`.
- Entity id (`transfer_id` / `request_id` / `experience_id`) depending on service.
- `drift_check` = `"pass"` or the `UndoFailureReason` value when it fails.
- `outcome` = `"success"` / `"rejected"` / `"error"`.

User email (if referenced at all) must be masked via `logging.MaskEmail`. Success logs at Info; drift rejections at Info (expected path, not a warning); internal failures at Error.

---

## 7. Chat Message Retraction

On undo:

1. Look up target message by `CommunityEvent.system_chat_message_id`. Direct read.
2. Soft-delete: set `ChatMessage.deleted = DeletedMetadata{...}`.
3. Insert new system message at `now` with `action = CHAT_SYSTEM_ACTION_UNDONE` and `undone_action` set. Chronology stays honest.
4. Client: the new message arrives on the existing chat stream; the client's standard refresh re-queries the (now-filtered) message list and the deleted original disappears naturally. Undo system messages render with a distinct treatment (e.g., "↶ Alex undid 'marked returned'").

### 7.1 Server-side filter invariant

**Load-bearing invariant, verified before Phase 3 ships.** Every server code path that returns `ChatMessage` to a client must filter `deleted IS NULL` at fetch time. A miss in any single path leaks a soft-deleted message to clients.

Verification (completes in Phase 2, gates Phase 3):

1. Enumerate every caller that produces an `api.ChatMessage` — search the server for `api.ChatMessage` in response builders, notification payload composers, chat list endpoints, conversation fetch, and any streaming handlers.
2. Confirm each applies `WHERE deleted IS NULL` at the storage layer (or equivalent post-query filter, preferring storage-level for index use).
3. Add a per-endpoint unit test that inserts a soft-deleted message and asserts it is absent from the response.
4. Document the full endpoint list in the Phase 2 PR description so future reviewers can spot new callers that need the same treatment.

---

## 8. Impact Metrics — Correctness Invariant

**Invariant:** for any sequence [perform-action, undo-action], post-undo metrics equal pre-action metrics.

Enforced by:

- `ImpactEstimate` is cleared on the transaction during undo.
- Metrics filter by current transaction state — undone transactions are excluded naturally.

**Test:** integration test that computes M1, performs action, computes M2 (≠ M1), undoes, computes M3, asserts `M3 == M1`. Replicate for giveaway, request-fulfillment, experience-completion.

---

## 9. Client Plumbing

### 9.1 Shared snackbar helper

Extend `app/lib/core/utils/toast_helper.dart` with a server-undo variant. Location rationale: toast helpers are cross-screen infrastructure, so they live in `core/utils/` (per client architecture "Utility classes (NOT controllers)") rather than `presentation/utils/`. The helper accepts a `BuildContext` so it can reach `ScaffoldMessenger` — callers are screens/widgets, not viewmodels.

```dart
ToastHelper.showServerUndo({
  required String message,
  required String undoLabel,
  required Future<void> Function() onUndo,   // Undo*Action RPC
  required VoidCallback optimisticRevert,    // UI state revert
  required void Function(UndoFailureReason, String) onUndoFailed,
  Duration duration = const Duration(seconds: 4),
});
```

On Undo tap: call `optimisticRevert()` first, then `await onUndo()`. On failure, surface `user_message` from the error detail and re-apply the forward state (no half-reverted UI).

### 9.2 Per-screen snackbar wiring

For each server-authoritative action, the screen that triggered the action:

1. Fires the action RPC.
2. Captures returned `community_event_id`.
3. Shows `ToastHelper.showServerUndo` with `onUndo` = the specific `Undo*` RPC.

### 9.3 Story-screen undo link

On the story screen for Server-story actions (complete loan, complete giveaway, mark request fulfilled, complete experience):

- When `UndoableAction` is populated on the story response, render a small centered text link ("↶ Undo") below the primary story content. Actor-only.
- Label: use `UndoableAction.label` verbatim (server-rendered).
- No countdown display.
- Tap → specific `Undo*` RPC → on success, **navigate back to the now-active item screen** (transfer detail in ACTIVE state, request detail pre-fulfill, experience detail pre-complete) — the screen shows the same state it had before the undone action.
- On error: inline error banner on the story screen with `user_message`; then invalidate and refetch the story via the story repository (`storyRepository.refresh(storyId)` or equivalent per caching conventions — the story response will no longer populate `UndoableAction` since the drift check would now fail, so the stale undo link disappears). Do not mutate the cached story in place; go through the repository so cache consumers update.
- Visibility: only the actor. Server populates `UndoableAction` only when requesting user matches `actor_id` and drift check would pass.

### 9.4 Error surfaces

- **Snackbar path:** secondary toast with `user_message`.
- **Story-screen path:** inline error banner ("Can't undo: the request was already fulfilled by someone else") and silent story refresh so the stale link disappears.

### 9.5 Cache invalidation on successful undo

On success, every repository whose cached state was derived from the reverted transaction must be invalidated, per project caching conventions. The responsible caller (the screen or viewmodel that issued the `Undo*` RPC) triggers the invalidation — not the network layer — so downstream consumers re-fetch through the repository as usual.

Invalidations per action class:

- **Complete loan / start loan / select recipient / cancel transfer:** refresh the affected transfer (`transferRepository`), the affected gear (`gearRepository` — state changes), the community impact metrics (`impactMetricsRepository`), and the conversation the action belonged to (`chatRepository` — retraction message + filtered original).
- **Complete giveaway:** above plus all sibling transfers that were auto-cancelled (revive) and all community-gear records that were archived (un-archive) — use pattern invalidation (`invalidatePattern('gear:*')` style) rather than per-id, since the set is known only from `reversal_data` on the server.
- **Mark request fulfilled / cancel request / select offer:** refresh the request (`requestRepository`), impact metrics, and the conversation.
- **Complete experience:** refresh the experience (`experienceRepository`), impact metrics, and the conversation.
- **Any Server-story success:** additionally, invalidate the story itself (`storyRepository.invalidate(storyId)`) — the story is conceptually gone once its underlying action is reverted.

For chat specifically, the existing chat refresh on new-message arrival (triggered by the retraction `CHAT_SYSTEM_ACTION_UNDONE` landing on the conversation stream) already picks up the filtered message list. No separate chat invalidation is required **provided** the chat repository routes list/get through the server rather than serving from a long-lived local cache. Phase 2 verifies this when auditing chat endpoints (§7.1).

---

## 10. Testing Strategy

### 10.1 Per-action unit tests

For each of the 9 server-authoritative actions: table test covering (a) successful undo, (b) reject by non-actor with `NOT_ACTOR`, (c) reject if already undone with `ALREADY_UNDONE`, (d) reject on wrong event type with `WRONG_EVENT_TYPE`, (e) reject on interleaving mutation with `ENTITY_CHANGED`, (f) reject on cascade divergence (giveaway only) with `CASCADE_UNRECOVERABLE`.

### 10.2 Integration tests

- Metric round-trip per server-authoritative action (§8).
- Chat retraction: original system message has `deleted != nil`, new `_UNDONE` message exists and links back via `undone_action`. Chat list/get endpoints don't return the deleted message.
- Giveaway cascade reversal: sibling transfers revive to correct prior states; community-gears un-archived.
- `UndoableAction` visibility on story response: appears only on actor's view; disappears after undo; disappears after another actor mutates the entity.
- **Query-count assertions** (`storage.AssertMaxQueries`) for each undo path. Rough targets to validate — tighten during implementation:
    - Simple undo (e.g., `UndoSelectRecipient`): ≤ 6 queries (load event, load entity, update entity, load chat msg, update chat msg, insert retraction event + retraction chat msg).
    - Loan/experience completion undo: ≤ 8 queries (adds gear update + impact clear).
    - Giveaway cascade undo: ≤ 12 + 2 × cascade_size queries. Prevents N+1 regression on sibling-transfer revival.

### 10.3 Concurrency

Parallel mutation collision: start a loan, begin undo + another user marks returned concurrently. Only one wins; the loser gets `UNDO_FAILURE_REASON_ENTITY_CHANGED`.

### 10.4 Client

- Widget test for `ToastHelper.showServerUndo` covering show → tap undo → RPC success → optimistic revert; show → no tap → expire; RPC fail → surface `user_message` and restore forward state.
- Widget test for story-screen "Undo" link: visible when `UndoableAction` set, hidden when null, navigates back on success, shows inline error on failure.
- Server end-to-end (Go): complete a loan, invoke the matching `Undo*` RPC, verify the loan returns to active state and the system chat message reflects the retraction. Separately: complete a loan, fetch the resulting story, verify `UndoableAction` is populated, invoke `Undo*`, verify the transfer is back in ACTIVE state.

### 10.5 Roundtrip test harness

**The load-bearing correctness guarantee** for every server-authoritative action is the A→B→A roundtrip. A single reusable helper in `server/undo/testing/roundtrip.go` encodes it so every undoable action reuses the same structure:

```go
// RoundTrip asserts that mutating an entity from state A to state B via
// the action under test, then undoing, returns the entity (and all
// affected side-effect state: gear, impact metrics, chat, sibling
// transfers, etc.) byte-equal to its state A snapshot.
func RoundTrip[E proto.Message](t *testing.T, s Setup, tc RoundTripCase[E]) {
    stateA := tc.SnapshotBefore(t, s)           // whole pre-action world
    eventID := tc.Mutate(t, s)                  // perform the forward action; return the CommunityEvent id
    stateB := tc.SnapshotAfter(t, s)
    requireDifferent(t, stateA, stateB)         // guard against a vacuous test
    tc.Undo(t, s, eventID)                      // call the specific Undo* RPC
    stateAPrime := tc.SnapshotBefore(t, s)      // re-snapshot the same surface as stateA
    requireEqual(t, stateA, stateAPrime)        // byte-equal modulo event-log append
}

type RoundTripCase[E proto.Message] struct {
    Name           string
    SnapshotBefore func(t *testing.T, s Setup) proto.Message  // returns a composite snapshot
    Mutate         func(t *testing.T, s Setup) (communityEventID string)
    Undo           func(t *testing.T, s Setup, communityEventID string)
    SnapshotAfter  func(t *testing.T, s Setup) proto.Message
}
```

Each of the nine server-authoritative actions ships a table entry under `server/services/{transfer,request,experience}/undo_roundtrip_test.go` using this helper. The snapshot composes:

- The mutated entity itself.
- The gear record (for transfer actions).
- The `ImpactEstimate` on the entity.
- The chat messages for the relevant conversation (filtered through the normal endpoint, so we catch soft-delete leaks).
- The community impact metrics (so we exercise §8's invariant in the same test).
- For giveaway cascade: every sibling transfer and community-gear row touched by the cascade.

"Byte-equal modulo event-log append" means: the `CommunityEvent` table legitimately differs (two new events — the forward and the retraction) and is excluded from the diff. Everything else must equal.

### 10.6 Action catalog and enforcement

**Brian's review (PR #1207) called out that the classification tables in §3 will go stale as new state-changing RPCs and event types are added.** We treat this as a first-class concern with three layers of enforcement — code registry, documentation, and CI test.

**Layer 1 — central registry (source of truth).** A single Go registry enumerates every `CommunityEventType` and classifies it:

```go
// server/undo/registry.go
package undo

type Classification int
const (
    ClassificationUnspecified Classification = iota
    ClassificationClientOnly       // client-only; inverse exists in UI/RPCs
    ClassificationServerSnackbar  // server undo via snackbar only
    ClassificationServerStory  // server undo via snackbar + story-screen link
    ClassificationRetentionRestore       // retention restore
    ClassificationIrreversible       // irreversible; flagged in UX
    ClassificationInternal    // server-side only; not user-triggered (e.g., system retractions)
)

type Entry struct {
    EventType    models.CommunityEventType
    Class        Classification
    // For server-authoritative: the Undo* RPC name this event is reversed by.
    UndoRPC      string
    // Non-empty rationale for client-only / irreversible / retention-restore
    // classifications; machine-readable short tag plus a human sentence.
    Rationale    string
    // For server-authoritative: validates that the ReversalData variant attached to
    // this event type has the fields required to reverse without loss.
    // Returns a descriptive error when the variant is insufficient.
    ValidateReversalData func(*models.ReversalData) error
}

var Registry = map[models.CommunityEventType]Entry{ /* … */ }
```

**Layer 2 — documentation.** `docs/undo_catalog.md` is the human-readable view. It is **generated from the registry** via a small codegen step (`go run ./cmd/gen-undo-catalog` wired into `npm run generate`) so the docs cannot drift from the code. The classification tables in §3.2 of this design doc freeze as the Phase 1 classification; the ongoing canonical list is the generated `docs/undo_catalog.md`.

**Layer 3 — CI test (the trip-wire).** One test, in `server/undo/registry_test.go`:

```go
// TestEveryEventTypeClassified fails when any CommunityEventType enum
// value lacks an entry in the Registry. Adding a new event type or a
// new state-changing RPC without classifying its undo behavior is a
// contributor's-attention-required failure, not a merge.
func TestEveryEventTypeClassified(t *testing.T) {
    for _, ev := range models.CommunityEventType_name {
        if _, ok := Registry[ev]; !ok {
            t.Errorf("event type %s has no undo classification — add an "+
                "entry to server/undo/registry.go (see Classification "+
                "values) before merging", ev)
        }
    }
}
```

Plus a companion test `TestEveryServerUndoableActionHasReversalDataValidator` that, for each `ClassificationServerSnackbar`/`ClassificationServerStory` entry, asserts `ValidateReversalData` is non-nil and that an example event produced by the forward RPC passes it. This is the storage-level invariant Brian flagged — the `ReversalData` variant on any given event must carry the fields needed to undo, and new write-path fields that can't be reversed without more data will fail this test.

**Layer 4 — contributor guidance (CLAUDE.md update in Phase 2).** Add a short section to `CLAUDE.md`:

> **Adding new state-changing RPCs.** Every new `CommunityEventType` enum value must be classified in `server/undo/registry.go` before merge — the CI test `TestEveryEventTypeClassified` enforces this. For server-authoritative classifications, also define the `ReversalData` variant and validator. `docs/undo_catalog.md` regenerates from the registry via `npm run generate`; never edit it by hand. When in doubt, default to `ClassificationClientOnly` with a rationale explaining why no server-side undo is needed.

This makes the right behavior the path of least resistance: Claude (and human contributors) will see a failing CI test before they finish the feature, with an error message pointing directly at the fix.

---

## 11. Phased Delivery

| Phase | Scope | Ships |
| --- | --- | --- |
| **1** (this doc) | Design + owner sign-off | `docs/ai/undo_plan.md` approved |
| **2** | Proto & storage foundation: `ReversalData`, `ChatMessage.deleted` (storage only), `*_UNDONE` enums, `system_chat_message_id` on `CommunityEvent`, `UndoableAction` submessage, `UndoFailureReason` + error detail, `community_event_id` on `models.Story`. Wire missing emissions: `CompleteExperience` + `SelectOffer` + experience collab-list actions. Audit all chat list/get endpoints to confirm soft-delete filter invariant (§7.1). Confirm story-fetch RPC wiring. Open follow-up issue for manual chat-delete UI. | Infra ready, no user-visible change |
| **3** | Server-snackbar without cascades: `UndoSelectRecipient`, `UndoSelectOffer`. Validates retraction plumbing end-to-end on simplest cases. | Two undoable actions live |
| **4** | Server-story: `UndoCompleteLoan`, `UndoCompleteExperience`. Story-screen link wiring. Metric round-trip tests. | Biggest user-value unlock |
| **5** | Server-story: `UndoCompleteGiveaway`. `giveaway_cascade` variant + cascade-reversal + drift handling. | Cascade case closed |
| **6** | Remaining server-authoritative: `UndoStartLoan`, `UndoMarkRequestFulfilled`, `UndoCancelTransfer`, `UndoCancelRequest`. | Server-authoritative complete |
| **7** | **Client wiring** — the only thing between the server work of Phases 2-6 and a usable feature. Extend `ToastHelper.showServerUndo` usage across every screen/viewmodel that fires a server-authoritative action: capture `community_event_id` from the response, show the snackbar, call the matching `Undo*` RPC on tap, surface `UndoErrorDetail.user_message` on failure, navigate back to the now-active item screen on success. Render the small centered "Undo" link on `StoryScreen` whenever `StoryPayload.undoable_action` is populated, wired to the same set of RPCs. Server-side Go end-to-end coverage for both surfaces. **Out of scope:** a "Recently deleted" profile surface — soft-delete/restore of long-lived entities is a separate feature and is not required for undo of user actions. | Feature usable end-to-end |
| **8** | Irreversibility polish + registry enforcement + docs close-out. Three distinct streams of work: (a) `ConfirmIrreversibleActionDialog` on the one or two action paths that are truly one-way (delete account etc.). (b) **Finish section 10.6 enforcement:** promote `TestEveryServerUndoableActionHasUndoDataValidator` from TODO-logging to a hard failure now that every server-authoritative action has a validator; add a companion test that asserts every `ClassificationServerSnackbar`/`ClassificationServerStory` entry in `server/undo/registry.go` has a corresponding `TestUndo*_RoundTrip` test in the owning service package — so a new undoable action can't land without round-trip coverage. Generate `docs/undo_catalog.md` from the registry and wire codegen into `npm run generate`. (c) Author summary doc `docs/undo.md` (the durable reference). **Scrub implementation-plan references:** grep the repo for every mention of `docs/ai/undo_plan.md` (CLAUDE.md guidance, Go/Dart comments, test comments, README / proto comments, issue-plan docs) and rewrite each to point at `docs/undo.md` instead. The plan doc is an implementation artifact; only the summary is durable. Archive `docs/ai/undo_plan.md` once the last reference is rewritten. | Design closed out |

Shipping order validates riskiest plumbing (chat retraction, metric invariant, story-screen link) on lowest-complexity cases (Phase 3) first.

---

## 12. Open Design Questions

Resolved via owner consultation. Remaining items are confirmations at build time:

- **Story-screen "Undo" link visual** (Phase 4): small centered text link below primary story content. Owner visual sign-off on the first one we ship (loan completion).
- **Retraction message rendering** (Phase 4): confirm visual treatment for `CHAT_SYSTEM_ACTION_UNDONE` system messages (suggested: "↶ Alex undid 'marked returned'").
- **Irreversible-action confirmation dialog copy** (Phase 8): finalize wording per action class.

---

## 13. Out of Scope (explicit)

- Money / refund handling.
- Admin-authority undo of other users' actions.
- Multi-level undo (undo of an undo). Second undo = re-do the action manually.
- Generic entity-detail undo affordance beyond the story screen.
- Correction push notifications for undone actions.
- Undo time windows and countdowns.
- Hard-delete retention policies and sweep jobs. Soft-deleted entities continue to be recoverable via existing mechanisms.
- Snackbar undo for RSVP changes (the experience screen's always-visible RSVP selector is the inverse path).
- Admin-remove-member flows (no UI today).
