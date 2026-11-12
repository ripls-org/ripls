# Issue #964: Generalized Undo functionality for major actions

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/964
**Filed:** 2026-03-31 | **Last activity:** 2026-04-20
**Labels:** (none) | **Assignees:** unassigned
**Context docs:** none passed; issue references `docs/users/village-blueprint.md` and `docs/users/growth-use-cases.md`

> **Working design doc:** `docs/ai/undo_plan.md` (authored in Phase 1; Phases 2-7 below are the initial sketch and will be revised and moved into that plan as the design is built). **After implementation completes, a concise summary lives at `docs/undo.md`.**

## Problem Statement

State-changing actions in Ripls — starting a loan, marking a loan returned, completing a giveaway, RSVPing to an experience, submitting or fulfilling a request, creating/destroying gear/communities, leaving a community — are either irreversible today or can only be "undone" by rebuilding state from scratch (cancelling and re-creating). This creates a psychological tax on the exact people we most want to act: the frequent, high-velocity users (Crisis Coordinators, Potluck Hosts) who make many decisions per session and are most exposed to misclicks.

The originally filed scope ("reopen owner management pill for accidentally closed items") is one narrow case. The owner comment on 2026-04-20 reframes this as a platform-wide primitive: a **shared undo mechanism** spanning snackbar-level quick reverts and historical reopen affordances, with first-class server-side reversal rather than user-visible rebuilding.

Two undo patterns are bundled in the issue but have very different blast radii: snackbar undo is mostly a client-UX problem sitting on top of existing mutations; reopen of terminal states requires loosened state machines, retraction of system messages, and (for cascading actions like giveaways) a reversal log.

## Context from Comments

One comment from maintainer (2026-04-20, from triage) enumerates four product decisions that must land **before implementation begins** and explicitly blocks execution on a design doc. All four are resolved in [Resolved Product Decisions](#resolved-product-decisions) below.

No other comments. This issue has had exactly one round of in-thread discussion, all from the owner — treat this as in design phase, not execution phase.

## Codebase Reality Check

- **Client-side undo primitive already exists.** [docs/client/undo.md](../client/undo.md), `ToastHelper.showUndo` in `app/lib/core/utils/toast_helper.dart`. Used today for swipe-to-unwatch on the portfolio inbox. Pattern: optimistic state change, background RPC, SnackBar with `Undo` button, reverse callback re-issues the inverse RPC. Works where a natural inverse RPC already exists.
- **State machines are centralized per domain.** `server/services/transfer/state_machine.go` (`isValidTransferTransition`) treats `COMPLETED` and `CANCELLED` as hard-terminal — no transitions allowed out. Experience and Request have analogous state machines. Reopening a terminal state means loosening these predicates conditionally (within window, actor-only).
- **Impact metrics are derived, not materialized.** `server/services/impact_metrics/actions.go` loads completed transfers/requests/experiences on demand and sums `ImpactEstimate` values at read time. Undo that flips a transaction out of its terminal state automatically excludes it from metrics — no explicit rollback needed. `ImpactEstimate` is stored on the transaction; undo should null it out too so direct reads stay consistent.
- **CommunityEvent is a near-complete action log already.** `proto/ripls/models/community.proto:190-263` — every state-changing community action already emits a `CommunityEvent` with `actor_id`, `object_user_id`, `gear_id`, a `topic` oneof (transfer/request/experience), `occurred_at_unix_sec`, `event_type`, and `transfer_type`. Callers live in `transfer/*`, `request/*`, `community/*`, `login/registration.go`. This is the natural substrate for a reversal log.
- **`models` vs `api` proto separation.** Per `CLAUDE.md`, storage-model protos never leak to clients. Reversal metadata we add lives only on `models.CommunityEvent`; the `api.CommunityEvent` RPC shape stays untouched. Client payloads do not grow.
- **Community events and chat system messages are append-only today.** Neither `CommunityEvent` nor `ChatMessage` has `DeletedMetadata`. Retracting the "Alice marked returned" system message after undo requires adding soft-delete to `ChatMessage` and a retraction event type to `CommunityEventType`.
- **Giveaway completion is the nastiest reversal case.** `server/services/transfer/state_machine.go:248-342` (`completeGiveaway`) marks gear `GIVEN_AWAY`, cancels every other pending transfer for the gear, archives every `CommunityGear` row. Undoing requires replaying the cascade inverse — which is why we need to record the cascade in reversal metadata.
- **Soft-delete is already a pattern.** `DeletedMetadata` in `proto/ripls/models/common.proto:9` is present on Gear, Transfer, Request, Experience, Community, User, Media, Location, and more. Create-then-delete undo is close to implementable as retention-window reactivation with no new storage.

## Resolved Product Decisions

All 2026-04-20 triage blockers resolved in thread:

| # | Question | Decision |
|---|---|---|
| Q1 | Undo window length | **Snackbar-display duration only** (a few seconds). No longer than the snackbar is visible. One tier. This dramatically shrinks the cascade-reversal surface — within a few seconds, nearly everything is still safely reversible. |
| Q2 | Notification interaction | **Send a correction ("undo") notification** to the same audience as the original. Don't try to suppress — FCM retraction is unreliable. |
| Q3 | Payment irreversibility | **Out of scope.** Ripls doesn't process payments today. Revisit when that changes; it'll likely be an "issue refund" flow, not generalized undo. |
| Q4 | Impact-metric rollback | **No explicit rollback.** Metrics derive from transaction state + stored `ImpactEstimate` at read time. Undo clears `ImpactEstimate` + reverts state; next metrics query excludes naturally. Integration test asserts round-trip. |
| Q5 | System message retraction | **Add `DeletedMetadata` to `ChatMessage`.** Mark the original "marked returned" message deleted, emit a new `CHAT_SYSTEM_ACTION_UNDONE` inline. **Open a follow-up issue** for a future manual "delete my chat message" UI that reuses the same soft-delete field. |
| Q6 | `reversal_data` shape | **Strongly-typed oneof** containing the minimal set of shapes the in-scope undo actions require (e.g., `giveaway_cascade`, `loan_completion`). No generic bytes. |
| Q7 | Does every in-scope action emit a `CommunityEvent`? | **Audit required in Phase 2.** Actions missing emission either get wired before inclusion or stay client-only-snackbar. |
| Q8 | Admin vs. actor undo | **Actor only.** Admin-level reopen is a separate feature. |
| Q9 | "Leave community" | **Snackbar only.** No long-window reopen — rejoining after that is via invitation. |
| +  | Low-stakes actions (added) | Some actions (e.g., RSVP flip, toggle watch) only need **client-side snackbar undo** via existing inverse RPCs — no server-side retraction plumbing needed. **Phase 2 audit classifies each action** into one of: (a) client-only snackbar, (b) server-side short-window undo, (c) create/destroy retention-window reopen, (d) flagged irreversible. |

## Recommendation

**Extend `models.CommunityEvent` with typed reversal metadata, and use it (server-side only) as the reversal log.** It already records actor + topic + timestamp for every state-changing community action — which is most of what a reversal log needs. We add:

1. A typed `ReversalData` oneof on **`models.CommunityEvent`** (storage-only; not copied to `api.CommunityEvent`) holding the cascade snapshot where needed.
2. Retraction event types (`COMMUNITY_EVENT_TYPE_*_UNDONE`) and `CHAT_SYSTEM_ACTION_UNDONE`.
3. `DeletedMetadata` on `ChatMessage`.

Undo flow for the server-side tier: look up the most recent `CommunityEvent` for the entity (by `transfer_id` / `request_id` / `experience_id` in the topic oneof), validate the window (a few seconds), apply the inverse transition using `reversal_data` to replay cascades, soft-delete the original system chat message, emit a retraction event, send a correction push notification.

Low-stakes / naturally-inverse actions stay client-only. Create/destroy undo uses the existing `DeletedMetadata` retention pattern — no `reversal_data` needed.

**Because the window is only a few seconds, cascade reversal stays simple in practice:** a giveaway-cascade undo within 3-5 seconds of completion is vanishingly unlikely to collide with a parallel mutation from another user. We still validate and fail cleanly on collision, but we're not designing for minutes of divergent-state tolerance.

## Open Questions

All product-level questions resolved in [Resolved Product Decisions](#resolved-product-decisions). Remaining design work (in Phase 1) is implementation-level, to be worked through in `docs/ai/undo_plan.md` **with the owner consulted at each step**:

- Exact classification per action (which of the 4 tiers above).
- Exact shape of each `ReversalData` oneof variant per action class.
- Notification-correction wording and timing.
- Whether undo is a single generic RPC (`UndoAction(community_event_id)`) or per-domain.

## Implementation Plan (initial sketch — supersedes into `docs/ai/undo_plan.md` after Phase 1)

### Phase 1: Design doc — `docs/ai/undo_plan.md`
**Goal:** Land the complete design in writing with owner sign-off before any proto or state-machine changes.
**Tasks:**
- [ ] Author `docs/ai/undo_plan.md` containing: tier classification per action, exact `ReversalData` variants, system-message retraction model, notification-correction policy, metric-correctness invariant, irreversibility UX, final choice on generic vs per-domain undo RPC.
- [ ] Consult owner **iteratively** while drafting — do not merge until each section is explicitly signed off.
- [ ] Move and revise the phases below into `docs/ai/undo_plan.md` as part of this phase; they become the authoritative plan once it lands.
- [ ] Open follow-up issue for "manual delete my chat message" UI that reuses the `ChatMessage.deleted` field added in Phase 2.

**Validates:** scope and design are agreed before code changes.

### Phase 2: Foundation — reversal log substrate on `models.CommunityEvent`
**Goal:** Land shared infrastructure. Everything after depends on this.
**Tasks:**
- [ ] **2a:** Audit every in-scope action for `CommunityEvent` emission (grep callers of `recordCommunityEvent*` in `server/services/{transfer,request,experience,community,login}`); wire emission for any that's missing. Classify each action into one of the four tiers from the `+` decision row.
- [ ] Add optional `ReversalData` message on **`models.CommunityEvent`** only (not `api.CommunityEvent`) with a typed oneof per cascade shape (`giveaway_cascade`, `loan_completion`, etc. — filled in as phases land).
- [ ] Add `*_UNDONE` variants to `CommunityEventType` for each undoable action class.
- [ ] Add `ChatSystemAction.CHAT_SYSTEM_ACTION_UNDONE` and `DeletedMetadata` on `ChatMessage` in `proto/ripls/models/chat.proto`.
- [ ] Update chat list endpoints to filter out soft-deleted messages; render retraction events inline.
- [ ] Server helper: `findRecentReversibleEvent(ctx, entityType, entityID, actorID, windowSec)` — the common lookup every undo RPC uses.

**Validates:** the substrate compiles, round-trips, and doesn't regress existing event consumers.

### Phase 3: Undo for simple reversible actions (RSVP, withdraw interest, withdraw offer)
**Goal:** Prove the pattern on actions that already round-trip through existing state machines.
**Tasks:**
- [ ] Per action: either client-only snackbar (if inverse RPC exists and retraction-in-chat isn't required) or a thin server undo RPC that calls `findRecentReversibleEvent`, applies the inverse, soft-deletes the original system chat message, records a retraction event, sends a correction notification.
- [ ] Client: extend `ToastHelper.showUndo` callers to cover these actions.
- [ ] Tests: perform action, undo within window, assert (a) entity state restored, (b) chat message marked deleted, (c) retraction `CommunityEvent` emitted, (d) community metrics unchanged pre/post.

**Validates:** retraction plumbing, window contract, metric round-trip.

### Phase 4: Undo for loan/experience completion
**Goal:** Apply the primitive to the highest-value actions from `growth-use-cases.md` (Betty and Emma live here).
**Tasks:**
- [ ] Loosen `isValidTransferTransition` to allow `COMPLETED → ACTIVE` within window, actor-only, gated on a recent `CommunityEvent` lookup.
- [ ] Add `UndoCompleteTransfer` / `UndoCompleteExperience` RPCs.
- [ ] On undo: revert state, clear `ActualReturnUnixSec`, re-mark gear `UNAVAILABLE`, clear stored `ImpactEstimate`. Experience: revert state, clear attendee-count-based `ImpactEstimate`.
- [ ] Soft-delete the `CHAT_SYSTEM_ACTION_COMPLETED` message, emit `_UNDONE` message.
- [ ] Send correction push notification to the same audience.
- [ ] Tests: happy path, window expired, non-actor attempt, impact-metric round-trip.

**Validates:** state-machine loosening is safe under window + actor checks; metrics are correct via derivation.

### Phase 5: Undo for giveaway completion (the cascade case)
**Goal:** Close the gap on the most cascading action using typed `ReversalData.giveaway_cascade`.
**Tasks:**
- [ ] Define `GiveawayCascadeSnapshot` submessage: list of sibling `transfer_id`s auto-cancelled with prior states, list of `community_gear_id`s archived.
- [ ] `completeGiveaway` writes the snapshot into the emitted `CommunityEvent.reversal_data` before returning.
- [ ] `UndoCompleteTransfer` giveaway branch: reads `ReversalData`, restores gear `GEAR_STATE_AVAILABLE`, un-archives listed community-gears, revives listed sibling transfers.
- [ ] Tests: full cascade reversal; reject after window; legible error on parallel-mutation collision (rare at few-seconds window but must be handled).

**Validates:** the typed-snapshot approach; informs whether it generalizes to future cascades.

### Phase 6: Create/destroy undo via `DeletedMetadata` retention
**Goal:** Cover "I accidentally deleted my gear / community / experience" — originally filed scope of #964.
**Tasks:**
- [ ] Retention windows per entity defined in `docs/ai/undo_plan.md` (proposal: 30 days user-owned; 7 days community-owned — confirm in Phase 1).
- [ ] `RestoreGear`, `RestoreExperience`, `RestoreRequest`, `RestoreCommunity` RPCs validate retention window, clear `DeletedMetadata`, emit a "restored" `CommunityEvent`.
- [ ] "Recently deleted" UI surface under Profile / Settings.
- [ ] Background sweep job hard-deletes past retention (explicit, tested; no behavior change from today).
- [ ] Tests: restore within window, reject outside, community-feed propagation.

**Validates:** long-window model on the easiest case (soft-delete reactivation), no `reversal_data` needed.

### Phase 7: Irreversibility UX and polish
**Goal:** Make the asymmetry honest.
**Tasks:**
- [ ] Audit action confirmations in `app/lib/presentation/screens/` for accurate "cannot be undone" framing.
- [ ] `ConfirmIrreversibleActionDialog` widget for terminal actions past window.
- [ ] Move the post-implementation summary to `docs/undo.md` (client + server), updating/superseding `docs/client/undo.md` as appropriate.
- [ ] End-to-end patrol test covering snackbar undo on loan-completion.

**Validates:** the system-wide UX story is coherent.

## Related Issues

- #1057 (`1057-start-first-loan-action.md`) — start-first-loan UX touches the same state transitions; undo design should align.
- #1104 (`1104-complete-experience-error.md`) — experience-completion errors today; undo depends on that path being stable first.
- #1133 (`1133-stale-chat-notification.md`) — chat-notification retraction surface is shared with undo's push-correction plan.
- The original "reopen owner management pill" scope is subsumed by Phase 6.
- **To be opened in Phase 1:** follow-up issue for manual "delete my chat message" UI, reusing the `ChatMessage.deleted` field added in Phase 2.
