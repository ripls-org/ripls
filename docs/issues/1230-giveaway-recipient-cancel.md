# Issue #1230: Giveaway cancels entirely when the selected recipient withdraws — should revert to open for others

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1230
**Filed:** 2026-04-22 | **Last activity:** 2026-04-29
**Labels:** (none — Bug/P1 set via project field per triage on 2026-04-24)
**Assignees:** unassigned
**Context docs:** none

> **Audit notes (2026-04-29):** Reviewed against `docs/server/architecture.md`,
> `docs/client/architecture.md`, `docs/client/caching.md`, and
> `docs/client/i18n.md`. Plan is compliant.
>
> - **Server architecture:** The recommended fix lives entirely inside
>   `server/services/transfer/` and only adjusts the conditions under which
>   `archiveCommunityGearForGiveaway` (state_machine.go:458) runs. No
>   service-to-service dependencies, no new shared library, no proto changes.
>   Tests land in the standard `*_test.go` location alongside the existing
>   `TestCancelGiveaway_*` suite.
> - **Client caching:** No client cache plumbing is needed. The existing
>   `TransferRepository.cancelTransfer` (`app/lib/data/repositories/transfer_repository.dart:233-260`)
>   already calls `invalidatePattern('my:*' / 'received:*' / 'community:*')`
>   plus `invalidateGearTransfers(gearId)` and
>   `invalidateUserTransferStatus(gearId)`, so on the next render User C's
>   gear card refetches `gearTransferContext` from the server with the
>   corrected `overall_phase`. Phase 2's manual smoke is the validator that
>   this is sufficient.
> - **Client i18n:** No new user-visible strings; the chat audit message
>   ("X cancelled the transfer") and the existing
>   `WithdrewInterestText` / `CancelledText` strings already cover the two
>   surfaces. If Option B is later pursued, the new "withdraw recipient"
>   verb would need an `app_en.arb` entry, but that's out of scope here.
> - **Proto conventions:** Recommended Option A makes no proto changes.
>   If Option B is later pursued, each new RPC requires its own dedicated
>   request/response message types per CLAUDE.md, and the new
>   `CommunityEventType` value needs a classification in
>   `server/undo/registry.go` (`TestEveryEventTypeClassified` enforces this).

## Problem Statement

A giveaway in `TRANSFER_STATE_RECIPIENT_SELECTED` is being treated as a single
binary commitment between two people. When the selected recipient backs out,
the server runs the same cancellation cascade that would fire if the owner
had cancelled the whole giveaway: the recipient's transfer is moved to
`TRANSFER_STATE_CANCELLED` and the per-community `CommunityGear` row is
archived, which causes `computeGiveawayPhase` (or the gear card's read of it)
to surface the gear as `GIVEAWAY_PHASE_CANCELLED`. The owner is left with a
greyed-out "managing" pill and no way to pick a different recipient; other
interested users (whose `INTEREST_EXPRESSED` transfers still exist in the
database) see a disabled "interested" pill even though their server-returned
`available_actions` is non-empty.

The intent of giveaway cancellation in the per-recipient transfer model
should be: **only the owner can decide the giveaway is over**. A recipient
backing out is one bid being withdrawn, not the end of the giveaway. The
giveaway phase should revert to `OPEN` (or `INTEREST_EXPRESSED`) so siblings
remain in the running and the owner can re-select.

A secondary client bug is mentioned in the issue body: the gear card renders
based on aggregate `overall_phase` rather than honouring the per-user
`available_actions`. Fixing the server makes the symptom go away in the
specific scenario, but the client lens-of-truth bug is real and worth
addressing in the same change so we don't regress this class of issue.

## Context from Comments

There are two comments:
- **Triage (2026-04-24, maintainer):** Set Bug + P1 via the
  `/ripls-triage` workflow. Rationale: "Wrong state machine transition
  prematurely terminates giveaways… data-loss-adjacent; product lens —
  breaks the stuff loop (story #1, #5) for Alfred and Gary."
- **2026-04-29 maintainer:** `@claude` ping that opened this run.

The issue body itself is detailed and offers two fix directions:
1. **Minimal:** Distinguish the actor (recipient vs. owner) in the
   `RECIPIENT_SELECTED → CANCELLED` giveaway path. Skip the community-gear
   archive when the recipient is the actor; leave sibling transfers alone.
2. **Broader:** Introduce a separate "withdraw recipient" / "un-select"
   action distinct from cancellation.

The issue also explicitly clarifies that this is not a regression from
PR #1228 (generalized undo, `undo-impl` branch); it predates that work and
is reproducible on `main`.

## Codebase Findings

- **Cancellation cascade lives in `server/services/transfer/state_machine.go`.**
  `transitionTransferState` (state_machine.go:25) routes `CANCELLED`
  transitions for giveaways through
  `archiveCommunityGearForGiveaway(gearID, communityID)`
  (state_machine.go:139–143, helper at state_machine.go:458). Archive happens
  unconditionally for any actor as long as `transferType == GIVEAWAY` and
  `targetState == CANCELLED`.
- **The state-machine validator already accepts the bad transition.**
  `isValidTransferTransition` (state_machine.go:265) returns `true` for
  "either party can cancel" from `RECIPIENT_SELECTED` (line 285–288). The
  validator is fine; the side-effects are wrong.
- **Phase computation is correct in isolation.** `computeGiveawayPhase`
  (`server/services/transfer/context.go:268` — duplicated at
  `server/services/chat/context.go:436`) returns `OPEN` when
  `pendingCount > 0` regardless of how many cancelled siblings exist, so the
  surviving `INTEREST_EXPRESSED` transfer for User C *should* leave the phase
  at `OPEN`. The `CANCELLED` surfacing the issue describes therefore comes
  from one of two places we need to verify during implementation:
  (a) the `archived=true` `CommunityGear` row being filtered out of the read
  path so the gear no longer resolves in that community context, or
  (b) the gear-card / chat-card client widgets short-circuiting to
  `GIVEAWAY_PHASE_CANCELLED` based on `Gear.state` or owner-side
  `UserTransfer` being absent. Either way, suppressing the archive is the
  load-bearing server change.
- **The `WithdrawInterest` path is *not* the broken path.**
  `withdrawFromGiveaway` (interest.go:565) already does the right thing for
  giveaways: removes the user from the conversation, calls
  `reassignGiveawayRecipient` if the withdrawer is the current recipient, and
  records a `TRANSFER_INTEREST_WITHDRAWN` event without ever touching
  `CommunityGear`. The bug is reachable through `CancelTransfer` only.
- **The chat workflow does invoke `cancelTransfer` post-selection.**
  `app/lib/presentation/viewmodels/conversation_transfer_actions.dart:255`
  defines `cancelTransfer()` for "from the chat workflow" and
  `declineTransferRequest()` (line 351) routes through the same RPC. The
  recipient-side action surfaces from chat as a "Cancel" affordance, which
  is why the bug is reachable in normal user flows.
- **Per-recipient transfers exist for both giveaways and loans.**
  `createNewTransfer` (interest.go:126) creates one transfer per interested
  user. `SelectRecipient` (lifecycle.go:25) only mutates the chosen
  transfer; siblings stay in `INTEREST_EXPRESSED`. There is no "primary"
  giveaway transfer owned by the owner; the owner's only handle on the
  giveaway is via individual recipient transfers.
- **Existing tests cover the *owner*-cancels case:**
  `TestCancelGiveaway_ArchivesCommunityGear` and
  `TestCancelGiveaway_FromRecipientSelected_ArchivesCommunityGear`
  (state_machine_test.go:385 and :471) both have the **owner** call
  `CancelTransfer`. Neither exercises the recipient-cancels path. The new
  behaviour requires adding a recipient-actor variant.
- **Undo plumbing already discriminates.** `transitionTransferState` builds
  a `TransferCancelUndo` populating `PriorState = RECIPIENT_SELECTED` so
  `UndoCancelTransfer` can restore the recipient_id. The undo entry for
  `TRANSFER_CANCELLED` in `server/undo/registry.go:207` is
  `ServerSnackbar` with `UndoCancelTransfer` as the inverse. We need to
  ensure undo of a recipient-side cancel still restores the transfer
  cleanly without re-archiving anything.

## Potential Solutions

### Option A: Actor-aware cascade in `transitionTransferState` *(recommended)*

- **Approach:** In `transitionTransferState`, when transitioning a giveaway
  to `CANCELLED`, gate the call to `archiveCommunityGearForGiveaway` on
  `actorID == transfer.OwnerId`. The recipient's transfer still moves to
  `CANCELLED`; siblings are untouched; the `CommunityGear` row stays
  unarchived so the gear remains in the feed and `overall_phase` reverts to
  `OPEN` via the existing `computeGiveawayPhase` rules. Add an explicit
  test that asserts the recipient-actor case leaves the community gear
  unarchived and another sibling transfer in `INTEREST_EXPRESSED`. Verify
  the chat / gear-card client widgets render the post-cancel gear as
  available again — the surface bug the issue reports.
- **Pros:** Minimal, surgical, no schema or RPC changes. Matches the
  intuitive product semantics described in the issue ("only owner
  cancellations end the giveaway"). Existing undo flow keeps working
  unchanged because `archiveCommunityGearForGiveaway` was the only
  side-effect skipped. Easy to test and easy to reason about.
- **Cons:** Doesn't address the secondary client bug the issue mentions
  (UI honoring `overall_phase` over `available_actions`); but that's
  cleanly scoped and can be done in the same PR if cheap.
- **Effort:** Small.

### Option B: Add a `WithdrawRecipientSelection` (un-select) RPC

- **Approach:** Introduce a new RPC for the recipient-side
  "I changed my mind" path: takes the transfer, validates the actor is
  the recipient and state is `RECIPIENT_SELECTED`, transitions the
  transfer back to `INTEREST_EXPRESSED` (or `CANCELLED` for that user
  while leaving siblings), and emits a dedicated event type.
  `CancelTransfer` then reverts to its semantic role: "the owner is
  ending the whole giveaway" — and we tighten its authorisation
  to require `actorID == OwnerId` for giveaways from
  `RECIPIENT_SELECTED`.
- **Pros:** Cleanest API contract — the verb matches the user intent.
  Allows analytics / notification flows to distinguish the two cases.
  Forward-compatible with future "owner unselects without rejecting"
  semantics.
- **Cons:** Adds proto surface area, a new event-type that needs an
  `undo.Registry` classification per CLAUDE.md, client-side wiring for
  a new action, and a backward-compatibility shim for older clients
  that still call `CancelTransfer` from chat — they'd need to keep
  working until everyone upgrades. Larger blast radius for what is
  effectively a one-line policy change.
- **Effort:** Medium.

### Option C: Defer archive to "all transfers terminal"

- **Approach:** Stop archiving the community gear in the per-transfer
  cancellation path entirely. Instead, archive `CommunityGear` only when
  the *gear* transitions to a terminal state (e.g. owner deletes it,
  someone completes a giveaway, or owner explicitly cancels via a
  forthcoming action). Practically: in `transitionTransferState`, after
  cancelling, query siblings and archive only if no non-terminal sibling
  remains; or move the archive to a higher-level "owner cancels giveaway"
  RPC.
- **Pros:** Symmetric — never archives prematurely. Sets up for Option B
  in the future.
- **Cons:** Changes owner-cancel semantics: if the owner cancels User B's
  transfer expecting "the giveaway ends," and User C still has interest,
  the gear stays in the feed. That's arguably wrong product behaviour
  (the owner explicitly said "cancel"). Without a separate
  `CancelGiveaway` action, owners lose the ability to end the whole
  giveaway. Effectively requires Option B to be useful.
- **Effort:** Medium-large (only makes sense alongside B).

## Recommendation

**Option A.** It is the smallest change that fixes the user-visible bug,
has zero proto/schema impact, and matches the policy the issue's
"Minimal" suggestion describes. The two-line policy change in
`transitionTransferState` is easy to test in isolation, easy to revert,
and doesn't foreclose Option B if we later decide we want a dedicated
"un-select" verb. If we ship A and the product team later wants to
distinguish "owner cancels the giveaway" from "owner rejects this
applicant," we can layer Option B on top without re-doing this work.

The secondary client behaviour the issue calls out — the gear card
greying out User C's interest pill — should also be verified in the
implementation. If suppressing the server archive cleanly restores the
correct phase, the client may already render correctly. If not, a small
follow-up at `gear_actions_section.dart:64-68` and
`gear_info_card.dart:53-55` can prefer `available_actions` non-empty
over `overall_phase == CANCELLED`. Decide based on what the manual
repro shows after the server fix.

## Open Questions

- **Should the recipient's cancelled transfer auto-revert to
  `INTEREST_EXPRESSED` instead of staying `CANCELLED`?** —
  Recommendation: **leave it `CANCELLED`**. The recipient explicitly
  said "no" to this giveaway (they hit the cancel/withdraw affordance);
  reverting to interest would put them back in the running against
  their will and would also re-display them in the owner's pending
  list. If they want back in, `ExpressInterest` already creates a fresh
  transfer.
- **Do we also need to suppress the cancel system chat message in this
  case?** — Recommendation: **keep it, but use existing wording.** The
  chat audit trail is valuable and the message ("X cancelled the
  transfer") is accurate. We're only changing whether the *gear*
  becomes unavailable, not whether anything visible to participants
  changed.
- **Does the owner still need a way to "end the whole giveaway"?** —
  Recommendation: **yes, but out of scope here.** Today the owner can
  still effectively end it by cancelling — under this fix, the owner's
  cancel of any specific transfer continues to archive the community
  gear (and per existing behaviour, only that one transfer flips). A
  future PR (Option B follow-up) can introduce a dedicated
  `CancelGiveaway` RPC that explicitly cancels all sibling transfers
  in one step. File as a follow-up, do not block.
- **Should the secondary client UI bug (honour `available_actions`
  before `overall_phase`) be fixed in the same PR?** —
  Recommendation: **fix only if the manual repro post-server-change
  still shows User C with a disabled pill.** If the server fix flips
  the phase back to `OPEN`/`INTEREST_EXPRESSED`, the existing client
  rendering is correct and no client change is needed.

## Implementation Plan

### Phase 1: Server fix + tests

**Goal:** Stop the recipient-side cancel from killing the giveaway for
everyone else.

**Tasks:**
- [ ] In `server/services/transfer/state_machine.go`
  `transitionTransferState`, gate
  `archiveCommunityGearForGiveaway(...)` on
  `actorID == transfer.OwnerId`. Add a 1-line comment explaining the
  policy ("only the owner cancelling ends the giveaway; recipient
  cancellation is one bid being withdrawn").
- [ ] Update or add a test in
  `server/services/transfer/state_machine_test.go`:
  - [ ] `TestCancelGiveaway_FromRecipientSelected_RecipientActor_DoesNotArchiveCommunityGear`
    — express interest as User C and User B; owner selects B; B
    cancels; assert (a) B's transfer is `CANCELLED`, (b) C's
    transfer remains `INTEREST_EXPRESSED`, (c) the
    `CommunityGear` row is *not* archived, (d) gear state is
    still `AVAILABLE`.
  - [ ] Verify existing `TestCancelGiveaway_FromRecipientSelected_ArchivesCommunityGear`
    still passes — it has the *owner* doing the cancel, which
    must continue to archive.
- [ ] Add a test that exercises the phase end-to-end: after the
  recipient cancels, fetch the gear transfer context for User C and
  assert `overall_phase == GIVEAWAY_PHASE_OPEN` and
  `available_actions` is non-empty.
- [ ] Verify undo: existing
  `TestUndoCancelTransfer_*` (or equivalent in
  `undo_test.go`) — make sure undoing a recipient-side cancel
  restores `RECIPIENT_SELECTED` cleanly without needing to
  un-archive anything (since we never archived).

**Validates:** Recipient withdrawal no longer takes the gear off the
feed; siblings retain their interest; owner sees the gear as still in
play and can `SelectRecipient` again.

### Phase 2: Manual smoke + optional client polish

**Goal:** Confirm the user-visible symptom is gone end-to-end and
decide whether the client needs a follow-up.

**Tasks:**
- [ ] Run the issue's repro on a local build (User A owns gear; B and
  C express interest; A selects B; B cancels). Verify the gear card
  for A shows actionable state ("select recipient" available) and the
  card for C shows their interest pill as actionable, not greyed out.
- [ ] If C's pill still renders as `GIVEAWAY_PHASE_CANCELLED` despite
  the server returning `OPEN` — investigate
  `app/lib/presentation/widgets/chat/gear_info_card.dart:53-55` and
  `app/lib/presentation/screens/gear/widgets/gear_actions_section.dart:64-68`
  for cached / stale phase reads. Otherwise no client change.
- [ ] Re-run `npm run build` and
  `cd app && flutter test --reporter github` for the touched areas.

**Validates:** End-to-end product behaviour matches the issue's
"Expected behavior" section.

### Phase 3 (optional, separate PR): Add `CancelGiveaway` for explicit owner intent

**Goal:** Give owners a single action to end the whole giveaway,
independent of cancelling individual recipient transfers.

**Tasks:**
- [ ] Design + ship per Option B above. Out of scope for #1230 unless
  product asks; file as a follow-up issue.

**Validates:** Owners no longer rely on a per-transfer side effect to
end a giveaway.

## Related Issues

- **#1228 (`undo-impl` branch)** — The issue notes this bug is *not* a
  regression from the generalized-undo work, but is in the same code
  area; coordinate the patch with whichever lands first to avoid merge
  conflicts in `state_machine.go`.
- **#964** — Explicitly called out in the issue as *not related*; the
  `completeGiveaway`-style cascade is not involved here.
