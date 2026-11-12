# Issue #1726: Close Request

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1726
**Filed:** 2026-05-05 | **Last activity:** 2026-05-05
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When a request owner taps "Close Request" in the request action menu, the
`CloseItemModal` bottom sheet appears with three rows: "Unshare from this
community", "Mark as Cancelled", and "Delete Permanently". Only "Delete
Permanently" is tappable — the other two render as disabled (greyed-out)
rows that do nothing. The reporter on iOS 0.19.0 (build 6338) tried to
mark the request cancelled or unshare it and could not.

The cause is purely a UI plumbing gap. `CloseItemModal` enables each row
only when the matching callback (`onUnshare`, `onCancel`, `onDelete`) is
non-null. Every call site in the app passes only `onDelete`, leaving the
other two callbacks null and therefore the rows disabled.

The bug affects every place `CloseItemModal.show` is invoked:

- `app/lib/presentation/screens/request/widgets/request_request_pane.dart:291`
  (Close Request — the reported case)
- `app/lib/presentation/screens/experience/widgets/experience_event_pane.dart:553`
  (Close Event)
- `app/lib/presentation/screens/gear/widgets/gear_menu_items.dart:126`
  (Close Giveaway — recipient-selected phase)
- `app/lib/presentation/screens/gear/widgets/gear_menu_items.dart:163`
  (Close Giveaway — pre-recipient phase)
- `app/lib/presentation/screens/gear/widgets/gear_menu_items.dart:381`
  (Close Lending)

This regression was introduced by commit `cef6d186b` ("Simplify menus",
2026-03-27), which removed the standalone "Cancel Request" / "Cancel
Giveaway" / "Cancel Loan" dropdown items and the "Communities" dropdown
items in favour of routing them through the new `CloseItemModal`. The
old handlers (`_handleCancelRequest`, `_handleCancelGiveawayFromDropdown`,
`_handleCancelLoanFromDropdown`, `_showRequestCommunitiesModal`,
`_showGearCommunitiesModal`) and their underlying viewmodel/repository
methods (`RequestNotifier.cancelRequest`, `ExperienceNotifier.cancelExperience`,
`TransferRepository.cancelTransfer`, `RequestSharingNotifier.unshareFromCommunity`,
`ExperienceSharingNotifier.unshareFromCommunity`,
`CommunityRepository.unshareGear`) all still exist — only the UI glue from
the new modal back to those methods is missing.

The original triage scoped this issue to requests since that was the user's
report; the maintainer comment on the plan PR (#1729) asked us to expand
the fix to the other item types, since they all manifest the same bug
through the same modal. This document is now the single plan covering all
five call sites.

## Context from Comments

- Issue #1726 itself has only `@claude` from `maintainer` triggering this
  triage run. There is no further clarification on the issue, no
  maintainer direction on scope beyond the reported request flow, and no
  debate about behaviour. The reporter's description is the full available
  signal.
- Plan PR #1729 received the directive `@claude please expand the plan to
  fix the other item types` from `maintainer` (`COLLABORATOR`) on
  2026-05-05. That comment is the reason this revision now covers events
  and gear in addition to requests.

## Potential Solutions

### Option A: Wire all three callbacks at every call site, contextual unshare (recommended)

- **Approach:**
  Add per-type confirmation handlers that mirror the existing per-type
  `_handleDelete*` handlers, plumb them through the relevant pane widget
  as new optional `VoidCallback` fields, and pass them to
  `CloseItemModal.show` as `onCancel` and `onUnshare`. For unshare, target
  the *current* community context (`state.communityId`), matching the
  modal's existing copy "Unshare from **this** community".

  Concretely:

  - **Requests** (`request_request_pane.dart` / `request_content_view.dart`):
    cancel via `RequestNotifier.cancelRequest()`; unshare via
    `RequestSharingNotifier.unshareFromCommunity(requestId, communityId)`.
  - **Events** (`experience_event_pane.dart` / `experience_content_view.dart`):
    cancel via `ExperienceNotifier.cancelExperience()`; unshare via
    `ExperienceSharingNotifier.unshareFromCommunity(experienceId, communityId)`.
  - **Gear giveaway/lending** (`gear_menu_items.dart` /
    `gear_content_view.dart` + `gear_transfer_handlers_mixin.dart`):
    cancel via `TransferRepository.cancelTransfer(transferId)` against the
    active transfer (the recipient-selected giveaway transfer or the
    active loan transfer). Unshare via
    `CommunityRepository.unshareGear(gearId, communityId)` followed by a
    `refreshAfterTransferAction()` so the access ring updates. When
    no active transfer exists (giveaway pre-recipient), pass null for
    `onCancel` so the row stays disabled — the modal contract already
    handles that gracefully.

  In every case, when `state.communityId` is null (rare deep-link case),
  pass a null `onUnshare` callback so just the unshare row stays disabled.

- **Pros:**
  - Smallest, most surgical fix per call site.
  - Matches the modal's existing copy ("this community", singular).
  - Reuses every existing viewmodel/repository method — no service or
    proto changes.
  - Symmetric with how `_handleDeleteRequest`, `_handleDeleteExperience`,
    and `handleDeleteGear` are already structured.
  - Restores the user-reported flow on the request and the parallel
    flows on event/gear that would have produced the same complaint.
- **Cons:**
  - Doesn't restore the broader "manage shared communities" experience
    that the deleted "Communities" dropdown items provided. Users who
    want to add/remove multiple communities at once still have to use
    the `AccessSheet` entry point on each item type.
  - One-off contextual unshare can confuse a user whose mental model is
    "the item is mine, not tied to one community". Mitigation: the modal
    copy already names "this community", so the action is unambiguous.
- **Effort:** small per call site; medium overall (three screens, four
  panes, plus tests).

### Option B: Wire cancel + delete only; route unshare through the multi-community picker

- **Approach:**
  Same as Option A for cancel and delete, but `onUnshare` opens the
  existing `CommunitySelectionModal` (the same one each item type's
  AccessSheet uses) so the user can toggle off any community. This
  restores the deleted "Communities" dropdown affordance via the modal.

- **Pros:**
  - Gives the user control when the item spans multiple communities.
  - Reuses an existing, tested modal.
- **Cons:**
  - Mismatch with the `CloseItemModal` row copy ("Unshare from *this*
    community"). Either the row label changes to something like
    "Manage shared communities", or the action surprises the user.
  - Two stacked modals on a flow that is supposed to be terminal-style
    ("close this thing") add friction.
  - `onUnshare` then doesn't actually unshare on its own — the
    semantics drift; the user can leave the picker without unsharing.
- **Effort:** small-medium per call site (also touches modal copy /
  behaviour).

### Option C: Hide the unwired rows entirely

- **Approach:**
  Change `CloseItemModal._buildOption` to return `SizedBox.shrink()`
  when the callback is null, instead of rendering a disabled row.
  Don't add cancel/unshare handlers — just stop showing options that
  don't work.

- **Pros:**
  - One-line change. Cosmetic only.
- **Cons:**
  - Strips functionality the user explicitly asked for.
  - Leaves the bug in place: the simplification commit's intent was
    that all three options work, not that two of them quietly
    disappear.
  - Hides the same partial wiring on every call site rather than fixing
    it anywhere.
- **Effort:** trivial, but does not fix the bug.

## Recommendation

**Option A, applied across all five call sites.** The viewmodel /
repository plumbing already exists for every item type, the modal copy
already implies a single contextual community, and the fix is the
symmetric counterpart to the per-type `_handleDelete*` handlers already
working in each file. It restores the user's reported missing
functionality on the request and proactively fixes the latent bug on
events and gear (which would surface as the next user complaint). No
architectural change.

The "manage many communities" use case that Option B serves is already
available via each item type's `AccessSheet`, so we are not regressing
that path. If a reviewer later wants the multi-community picker exposed
from the close modal too, that's a follow-up change at any single call
site.

## Open Questions

- **Should unshare ask the user to confirm?**
  Recommendation: yes, on every item type. A small `AlertDialog` with
  localised title "Remove from this community?" and confirm button
  "Remove", matching the cancel/delete dialogs already in each content
  view. Unsharing is reversible (re-share via AccessSheet) but the
  user is in a "Close X" flow and a wrong tap should not silently
  detach the post from a community feed.

- **What should the cancel confirmation dialog say per type?**
  Recommendation: keep the exact copy from each pre-`cef6d186b`
  baseline where one existed, and synthesise from precedent where
  none did:
  - Request: "Cancel Request?" / "Are you sure you want to cancel
    this request?" / confirm "Cancel Request" (red).
  - Event: "Cancel Event?" / "Are you sure you want to cancel this
    event? Participants will be notified." / confirm "Cancel Event"
    (red). (Mirrors the unused `cancel_event_modal.dart` precedent.)
  - Giveaway: "Cancel Giveaway?" / "Are you sure you want to cancel
    this giveaway? This cannot be undone." / confirm "Cancel
    Giveaway" (red). (Verbatim from the deleted
    `_handleCancelGiveawayFromDropdown` in `cef6d186b`.)
  - Loan: "Cancel Loan?" / "Are you sure you want to cancel this
    loan?" / confirm "Cancel Loan" (red). (Verbatim from the deleted
    `_handleCancelLoanFromDropdown` in `cef6d186b`.)
  All copy goes through `app_en.arb` + `app_es.arb`.

- **What happens after cancel succeeds — close the screen, or stay?**
  Recommendation: stay, on every type. Cancelling flips state on the
  underlying entity so the menu collapses to a terminal layout
  automatically (request: `RequestState.REQUEST_STATE_CANCELLED`
  collapses `menuItems` at `request_request_pane.dart:236`; event:
  `ExperienceState.EXPERIENCE_STATE_CANCELLED` similar; giveaway:
  `GiveawayPhase.GIVEAWAY_PHASE_CANCELLED` returns `const []` at
  `gear_menu_items.dart:54`; loan: cancelled transfer simply removes
  the active borrower section). The user can navigate back themselves.
  Matches what each `_handleDelete*` does *not* do — delete pops via
  the screen's `onDeleted` callback, but cancel is a state transition,
  not a removal.

- **What if `state.communityId` is null at the moment the modal is
  shown?**
  Recommendation: pass a null `onUnshare` callback in that case so the
  row stays disabled. Don't surface a misleading toast. This preserves
  the existing modal contract (callback null → row disabled) and keeps
  the fix narrow.

- **Gear: what if there is no active transfer at the moment the modal
  is shown?**
  Recommendation: pass a null `onCancel` callback so the row stays
  disabled. Specifically: for the giveaway pre-recipient call site
  (`gear_menu_items.dart:163`), there is no recipient transfer yet, so
  there is nothing to cancel — only the unshare and delete rows are
  meaningful. For the recipient-selected and active-loan call sites,
  the transfer always exists.

- **Should the modal copy itself be localised?**
  Out of scope. The modal currently uses hard-coded English strings
  ("Close Request", "Unshare from this community", etc.) at
  `close_item_modal.dart:91, 102, 120, 139`. Localising the modal
  itself is a worthwhile follow-up issue but not required to fix the
  reported bug. We will, however, localise all *new* dialog and toast
  copy added by this fix.

- **Should we modernise the icon color of the disabled row?**
  Recommendation: no. The disabled colouring is already implemented
  at `close_item_modal.dart:165` and works correctly — the issue is
  that callbacks are null, not that disabled styling is broken. Out
  of scope.

## Implementation Plan

### Phase 1: Add ARB strings (both locales)

**Goal:** All new user-visible copy lives in `app_en.arb` and
`app_es.arb` so the i18n lint stays green and Spanish users get
matching translations. Per `docs/client/i18n.md`, every user-visible
string must be reached through `context.l10n.*`.

**Tasks:**
- [ ] Add the following keys to `app/lib/l10n/app_en.arb`. Use
  `lowerCamelCase` and a per-type scope prefix (`request*`, `experience*`,
  `gear*`) for cancel-related copy plus a shared `commonUnshare*` block
  for the unshare dialog (same copy across all four item types since the
  modal copy is already shared). Each new key needs an `@key` description
  block.

  Per-type cancel copy:
  - `requestCancelDialogTitle` = `"Cancel Request?"`
  - `requestCancelDialogBody` = `"Are you sure you want to cancel this request?"`
  - `requestCancelConfirm` = `"Cancel Request"`
  - `requestCancelSuccess` = `"Request cancelled"`
  - `experienceCancelDialogTitle` = `"Cancel Event?"`
  - `experienceCancelDialogBody` = `"Are you sure you want to cancel this event? Participants will be notified."`
  - `experienceCancelConfirm` = `"Cancel Event"`
  - `experienceCancelSuccess` = `"Event cancelled"`
  - `giveawayCancelDialogTitle` = `"Cancel Giveaway?"`
  - `giveawayCancelDialogBody` = `"Are you sure you want to cancel this giveaway? This cannot be undone."`
  - `giveawayCancelConfirm` = `"Cancel Giveaway"`
  - `giveawayCancelSuccess` = `"Giveaway cancelled"`
  - `loanCancelDialogTitle` = `"Cancel Loan?"`
  - `loanCancelDialogBody` = `"Are you sure you want to cancel this loan?"`
  - `loanCancelConfirm` = `"Cancel Loan"`
  - `loanCancelSuccess` = `"Loan cancelled"`

  Shared unshare copy (used identically by all four item types):
  - `commonUnshareDialogTitle` = `"Remove from this community?"`
  - `commonUnshareDialogBody` = `"This will no longer appear in this community's feed. You can re-share it later."`
  - `commonUnshareConfirm` = `"Remove"`
  - `commonUnshareSuccess` = `"Removed from community"`
- [ ] Mirror every key into `app/lib/l10n/app_es.arb` with a Spanish
  translation (matching the conventions used by neighbouring keys
  like `requestStatusCancelled`, `experienceStatusCancelled`).
- [ ] Run `npm run generate` so the typed `context.l10n.*` getters
  are regenerated.

**Validates:** `npm run lint:dart:i18n` passes; both locale files
contain the new keys; generated `AppLocalizations` exposes the new
getters.

### Phase 2: Wire cancel + unshare for requests

**Goal:** The reporter's flow works — tapping Close Request shows three
enabled options, each performs the documented action with confirmation
and toast.

**Tasks:**
- [ ] In
  `app/lib/presentation/screens/request/widgets/request_request_pane.dart`,
  add two new fields to `RequestRequestPane`: `final VoidCallback?
  onCancelRequest;` and `final VoidCallback? onUnshareRequest;`. Update
  the constructor. Pass them as `onCancel` and `onUnshare` to
  `CloseItemModal.show` at line 291. Mark them optional (nullable) so
  call sites that don't supply them gracefully degrade.
- [ ] In
  `app/lib/presentation/screens/request/request_content_view.dart`,
  add `_handleCancelRequest()` modelled after `_handleDeleteRequest`
  (lines 726–777). Show an `AlertDialog` whose title, body, and
  confirm button text come from
  `context.l10n.requestCancelDialogTitle`,
  `context.l10n.requestCancelDialogBody`, and
  `context.l10n.requestCancelConfirm` (red). On confirm, call
  `ref.read(requestProvider(widget.requestId).notifier).cancelRequest()`.
  On success, show `context.l10n.requestCancelSuccess` via
  `ToastHelper.showSuccess`. On error, surface the error via
  `ToastHelper.showError` using the existing `RpcErrorHandler.localize`
  pattern. Do not pop the screen.
- [ ] In the same file, add `_handleUnshareRequest()`. Read
  `state.communityId`. If null, return (defensive — the menu won't
  call this since we'll pass null). Show an `AlertDialog` whose
  title and body come from `context.l10n.commonUnshareDialogTitle`
  and `context.l10n.commonUnshareDialogBody`, with a red confirm
  button labelled `context.l10n.commonUnshareConfirm`. On confirm,
  call `ref.read(requestSharingProvider.notifier).unshareFromCommunity(
  widget.requestId, state.communityId!)`. On success, show
  `context.l10n.commonUnshareSuccess` and call
  `refreshRequestDetails()` so `sharedCommunities` and the access
  ring update. On error (including the "Cannot remove from the
  last community" exception thrown by `unshareFromCommunity`),
  surface the error via `ToastHelper.showError` and
  `RpcErrorHandler.localize`.
- [ ] In `_buildBottomContent`, pass
  `onCancelRequest: _handleCancelRequest` and
  `onUnshareRequest: state.communityId == null ? null :
  _handleUnshareRequest` to `RequestRequestPane`.

**Validates:** All three rows tappable in the request close modal in a
community context; cancel + unshare both round-trip through the server.

### Phase 3: Wire cancel + unshare for events

**Goal:** Tapping Close Event shows three enabled options for the
organizer, each performs the documented action with confirmation and
toast.

**Tasks:**
- [ ] In
  `app/lib/presentation/screens/experience/widgets/experience_event_pane.dart`,
  add two new fields on `ExperienceEventPane`:
  `final VoidCallback? onCancelExperience;` and
  `final VoidCallback? onUnshareExperience;`. Update the constructor.
  Pass them as `onCancel` and `onUnshare` to `CloseItemModal.show` at
  line 553.
- [ ] In
  `app/lib/presentation/screens/experience/experience_content_view.dart`,
  add `_handleCancelExperience()` modelled after `_handleDeleteExperience`
  (lines 900–946). Confirmation dialog uses
  `context.l10n.experienceCancelDialogTitle`,
  `experienceCancelDialogBody`, and `experienceCancelConfirm` (red).
  On confirm, call
  `ref.read(experienceProvider(widget.experienceId).notifier).cancelExperience()`.
  On success show `context.l10n.experienceCancelSuccess` and refresh
  details so the menu collapses; on error use the existing toast +
  RpcErrorHandler pattern. Do not pop the screen.
- [ ] Add `_handleUnshareExperience()` reading
  `ref.read(experienceProvider(widget.experienceId)).communityId`.
  Confirmation dialog uses the shared `commonUnshare*` keys. On
  confirm, call
  `ref.read(experienceSharingNotifierProvider.notifier).unshareFromCommunity(widget.experienceId, communityId)`.
  On success, show `commonUnshareSuccess` and call
  `refreshExperienceDetails(communityId: communityId)` so the access
  ring updates.
- [ ] In the existing `ExperienceEventPane(...)` construction at
  `experience_content_view.dart:589`, pass
  `onCancelExperience: _handleCancelExperience` and
  `onUnshareExperience: state.communityId == null ? null :
  _handleUnshareExperience`.

**Validates:** All three rows tappable in the event close modal for the
organizer; cancel flips state to CANCELLED and the menu collapses;
unshare drops the community from the access ring.

### Phase 4: Wire cancel + unshare for gear (giveaway and lending)

**Goal:** Tapping Close Giveaway or Close Lending shows three enabled
options for the owner. The cancel row targets the active transfer; the
unshare row targets the current community.

**Tasks:**
- [ ] In
  `app/lib/presentation/screens/gear/widgets/gear_menu_items.dart`,
  add two new optional callback parameters to
  `buildGiveawayOwnerItems` and `buildLoanOwnerItems`:
  `Future<void> Function(String transferId)? onCancelTransfer` and
  `Future<void> Function()? onUnshareFromCommunity`. Pass them through
  to the three `CloseItemModal.show` invocations at lines 126, 163, and
  381. For each invocation, compute `onCancel` from the optional
  callback bound to the active transfer's id (or null if no active
  transfer):
  - Line 126 (giveaway, recipient-selected phase): bind to
    `transfer.id` (the recipient transfer is non-null in this branch).
  - Line 163 (giveaway, pre-recipient phase): there is no transfer to
    cancel, so pass `onCancel: null` regardless of whether the caller
    supplied `onCancelTransfer`.
  - Line 381 (loan owner): bind to `transfer.id` if a borrower transfer
    is active; otherwise pass `onCancel: null`.
  For each `onUnshare`, pass the optional callback directly (or null if
  the caller didn't supply one).
- [ ] In `app/lib/presentation/screens/gear/widgets/gear_transfer_handlers_mixin.dart`,
  update `handleCancelLoanFromDropdown` to use
  `context.l10n.loanCancelDialogTitle`, `loanCancelDialogBody`,
  `loanCancelConfirm`, and surface `loanCancelSuccess` on success.
  Add a parallel `handleCancelGiveawayFromDropdown(String transferId)`
  using `giveawayCancelDialogTitle`, `giveawayCancelDialogBody`,
  `giveawayCancelConfirm`, and `giveawayCancelSuccess` (the deleted
  `_handleCancelGiveawayFromDropdown` from commit `cef6d186b` is the
  reference implementation — re-introduce it here, calling
  `transferRepository.cancelTransfer` then `refreshAfterTransferAction`).
  Rename both methods to drop the misleading "FromDropdown" suffix
  while we're here (`handleCancelGiveaway`, `handleCancelLoan`) — the
  dropdown is gone; they're now invoked from the close modal.
- [ ] In the same mixin, add `handleUnshareGearFromCommunity()` which:
  reads the current `state.communityId` from `gearProvider(gearId)`,
  shows a confirmation dialog using the shared `commonUnshare*` keys,
  on confirm calls
  `ref.read(communityRepositoryProvider).unshareGear(gearId: gearId, communityId: communityId)`,
  shows `commonUnshareSuccess`, and calls `refreshAfterTransferAction()`
  so the access ring updates. Use null guard for `state.communityId`
  even though we filter at the call site.
- [ ] In `app/lib/presentation/screens/gear/gear_content_view.dart`,
  pass the new handlers to both `buildGiveawayOwnerItems` (line 581)
  and `buildLoanOwnerItems` (line 616): `onCancelTransfer:
  handleCancelGiveaway` (or `handleCancelLoan` for the loan owner
  builder), and `onUnshareFromCommunity: state.communityId == null ?
  null : handleUnshareGearFromCommunity`.
- [ ] If the existing `buildGiveawayOwnerItems` no-recipient branch
  (line 136 onward) does not have access to a transfer id, ensure the
  `onCancel` callback passed to `CloseItemModal.show` at line 163 is
  `null` — handled by the per-call-site computation in the first task.

**Validates:** All three rows tappable in the close modal for both
giveaway phases (where applicable) and lending; cancel cancels the
active transfer; unshare removes the gear from the current community
without deleting it. The pre-recipient giveaway case still shows the
unshare and delete rows enabled and the cancel row disabled.

### Phase 5: Tests

**Goal:** Lock in the wiring so the next "simplify" pass doesn't
silently regress it again.

`docs/client/testing.md` is explicit that ViewModels are the primary
test target and widget tests are reserved for high-value flows.
However, the bug here is *purely* widget-layer wiring (callbacks not
passed to a child) — a ViewModel test would not catch it. So we add
targeted widget tests at each call site plus rely on the existing
ViewModel coverage of `cancelRequest`, `cancelExperience`,
`unshareFromCommunity`, and `cancelTransfer`.

**Tasks:**
- [ ] Add a widget test at
  `app/test/presentation/screens/request/widgets/request_request_pane_test.dart`
  that pumps a `RequestRequestPane` with a non-cancelled, non-fulfilled
  state, an `isOwner: true` flag, and a non-null `communityId`. Provide
  mock callbacks for `onDeleteRequest`, `onCancelRequest`,
  `onUnshareRequest`. Open the workflow overflow → "Close Request",
  then introspect the three `ListTile` widgets in `CloseItemModal` and
  assert `onTap != null` on each. (Don't tap through end-to-end —
  confirmation dialogs would require deeper context.) Add a second
  variant where `communityId` is null and assert the unshare row is
  disabled (`onTap == null`) while the other two remain enabled.
- [ ] Add a parallel test at
  `app/test/presentation/screens/experience/widgets/experience_event_pane_test.dart`
  pumping `ExperienceEventPane` for an organizer, opening "Close
  Event" via the workflow overflow, and asserting all three rows
  enabled. Variant: `communityId == null` → unshare disabled.
- [ ] Add a parallel test at
  `app/test/presentation/screens/gear/widgets/gear_menu_items_test.dart`
  exercising both `buildGiveawayOwnerItems` (recipient-selected and
  pre-recipient branches) and `buildLoanOwnerItems`. For each, build
  the dropdown items with mock callbacks, locate the "Close X" item,
  trigger its `onTap`, and assert the resulting `CloseItemModal`'s row
  states:
  - Recipient-selected giveaway: all three enabled.
  - Pre-recipient giveaway: cancel disabled, unshare + delete enabled.
  - Loan with active transfer: all three enabled.
  - Loan with no active transfer: cancel disabled, unshare + delete
    enabled.
- [ ] Use `app/test/helpers/l10n_helpers.dart` for the i18n delegate
  setup in every new widget test (per `docs/client/i18n.md`).
- [ ] Do NOT add new ViewModel tests in this PR. `cancelRequest`,
  `cancelExperience`, `cancelTransfer`, and `unshareFromCommunity` are
  already exercised through the existing notifier tests. If those
  tests don't yet exist, that's a separate gap and out of scope.
- [ ] Run `cd app && flutter test --reporter expanded
  test/presentation/screens/request/widgets/request_request_pane_test.dart
  test/presentation/screens/experience/widgets/experience_event_pane_test.dart
  test/presentation/screens/gear/widgets/gear_menu_items_test.dart`
  to validate locally, then `cd app && flutter test --reporter github`
  for the full suite. Iterate until green.

**Validates:** A future commit that drops `onCancel` or `onUnshare`
from any of the five call sites will fail CI.

### Phase 6: Cleanup and commit

**Goal:** Land the fix.

**Tasks:**
- [ ] Run `npm run lint:dart:size` (each touched file should remain
  under 1000 lines; `experience_content_view.dart` is the closest to
  the cap and worth checking).
- [ ] Run `npm run lint:dart:i18n` to confirm no hardcoded user-facing
  strings were introduced.
- [ ] Run `npm run lint:dart:a11y` to confirm no new accessibility
  violations (the new dialogs use stock `AlertDialog` which is fine).
- [ ] Run `npm run lint:ephemeral-comments` to catch any phase markers
  introduced by accident.
- [ ] Commit with a message describing the user-visible bug and the
  root cause, naming all five call sites. Open PR.

**Validates:** Lint is green; PR description accurately describes the
fix.

## Audit notes

The audit step (`/ripls-audit`) flagged two issues with the original
draft of this plan, which have been addressed inline:

1. **i18n compliance.** The first draft proposed inline English
   strings for the new dialog title, body, confirm button, and toasts.
   `docs/client/i18n.md` requires every user-visible string to live
   in `app_en.arb` + `app_es.arb` and to be reached via
   `context.l10n.*`; the CI lint `npm run lint:dart:i18n` would have
   rejected the implementation. Phase 1 now explicitly enumerates
   the new ARB keys (per-type cancel copy plus shared unshare copy)
   and instructs the implementer to add Spanish translations.
2. **Testing layer.** The first draft hand-waved the test approach
   ("add a widget test"). `docs/client/testing.md` is explicit that
   ViewModels are the primary test target and widget tests must be
   justified. The bug here is genuinely widget-layer (callback
   plumbing), so widget tests are correct — but the plan now
   spells out exactly what to assert (enabled `ListTile.onTap`
   non-null per row), uses the documented `l10n_helpers.dart` setup,
   adds parallel coverage at every call site (not just request), and
   explicitly defers ViewModel tests as out of scope (the underlying
   methods already exist with their own coverage).

No other architectural concerns — the plan stays inside the
established MVVM boundaries (screen → notifier → repository), reuses
existing viewmodel/repository methods rather than adding new ones, and
does not touch the cache layer.

## Related Issues

- Commit `cef6d186b` ("Simplify menus", 2026-03-27) is the regression
  source. Not a GitHub issue, but worth referencing in the fix PR
  description.
- The unused `app/lib/presentation/screens/experience/cancel_event_modal.dart`
  is dead code (no callers) since `CloseItemModal` superseded it.
  Out of scope for this fix; flag for a separate cleanup PR.
- The hardcoded English strings inside `CloseItemModal` itself
  (`close_item_modal.dart:91, 102, 120, 139`) are a pre-existing i18n
  gap. Out of scope for this fix; flag for a separate i18n PR. The
  new dialog/toast copy this fix introduces is fully localised.
