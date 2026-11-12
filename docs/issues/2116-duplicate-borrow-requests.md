# Issue #2116: I'm in line multiple times to borrow an item

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2116
**Filed:** 2026-05-22 | **Last activity:** 2026-05-22
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

A user (user2@example.org, PROD build 7130, app 0.22.0, Pixel 9 Pro / Android 16)
tapped the "Borrow" button twice quickly on a gear detail screen and ended up
in the borrow queue twice — i.e. two separate `Transfer` rows were created
for the same `(gear_id, recipient_id, community_id)` triple, both in
non-terminal states. The user explicitly asks for the duplicate to be
prevented both on the server (idempotent or rejected) and on the client (UI
debounce / disable while in flight).

Today there is no protection on either side:

- **Server:** `Service.ExpressInterest`
  (`server/services/transfer/interest.go:21`) validates the gear and
  community, then unconditionally calls `createNewTransfer`
  (`interest.go:126`) which inserts a fresh `models.Transfer` row. No
  query asks "does this user already have an active transfer for this
  gear?". The existing test
  `TestService_ExpressInterest_ExistingLoan_CreatesMultipleTransfers`
  (`interest_test.go:306`) only covers *different* users borrowing —
  it does not guard against the same user double-requesting.
- **Client:** `GearActionsSection`
  (`app/lib/presentation/screens/gear/widgets/gear_actions_section.dart:134`)
  disables the loan CTA only when `hasRequested` is true (i.e. only after
  the server-confirmed state arrives back through
  `GearTransferContext.userTransfer`). Between the first tap and the state
  update at `gear_view_model.dart:929`, the button stays armed. The same
  is true for giveaways: `GearActionsSection` line 80–90 wires
  `onExpressInterest: onTransferTap` with no `isLoading`/`isSaving` gate.
- The viewmodel's `expressInterest()` (`gear_view_model.dart:880`)
  *sets* `state.isSaving = true` but does **not** early-return if it is
  already true (unlike `saveChanges` at line 474 which uses
  `if (state.isSaving …) return;`). So even at the viewmodel layer a
  second tap can fire a second RPC before the first one's optimistic
  refresh lands.

The result is a server allowing N concurrent or back-to-back inserts and a
UI that doesn't suppress them. Either layer alone could be made bulletproof;
both should be tightened because the server protection is the durable
guarantee and the client protection is the latency-friendly UX.

## Context from Comments

Only one comment on the issue — `@claude` from the maintainer (maintainer)
triaging this to autonomous handling. No clarifying discussion, no
maintainer direction on idempotent-vs-error, and no mention of related
issues. The submitter (user2@example.org) and the maintainer
(maintainer@github) are almost certainly the same person, given the
project context.

The issue body explicitly proposes both layers of fix ("on server and
client (with debounce or UI suspension)"), so we have author endorsement
for the dual-layer approach.

## Potential Solutions

### Option A: Idempotent server + client guard (recommended)

- **Approach:** In `ExpressInterest`, after `validateGearForInterest` and
  `findMatchingCommunityAndType`, query
  `QueryByFields(gear_id, recipient_id, community_id)` on `models.Transfer`.
  If any row exists in a non-terminal state (anything other than
  `TRANSFER_STATE_COMPLETED` or `TRANSFER_STATE_CANCELLED`), build the API
  response from that existing transfer and return it without inserting a
  new row, recording a log line and a `Debug`-level counter for visibility.
  On the client, (1) add a `state.isSaving` early-return guard to
  `GearViewModel.expressInterest()` matching `saveChanges`, and (2) pass
  `isLoading: state.isSaving` (or wire `onAction: null` while saving) into
  the loan `MorphingActionButton` and the giveaway `WorkflowActionButtons.buildGiveaway` factory.
- **Pros:**
  - Idempotent semantics are friendly to retries (flaky network, client
    re-issuing on app foreground, etc.) — the user always lands in the
    same end state.
  - The client doesn't need to special-case a 409-style error response.
  - Cancelled transfers (user withdrew, owner declined) are still
    re-borrowable — those rows are terminal so the query excludes them.
- **Cons:**
  - A new code path. We must remember to NOT record a second community
    event or re-emit a system message when returning an existing
    transfer; otherwise we'd notify the owner twice.
  - The server check is read-then-write, so two truly simultaneous calls
    (same user, same gear, same community) can still race. Acceptable —
    the UI guard removes the realistic case, and a community event
    deduper for `TRANSFER_INTEREST_EXPRESSED`/`RECIPIENT_SELECTED` could
    be a follow-up if it actually shows up.
- **Effort:** Small–medium. ~30 lines server, ~15 lines client, two
  focused tests.

### Option B: Strict server rejection (409 / FailedPrecondition) + client guard

- **Approach:** Same idea, but if an active transfer exists, return
  `connect.NewError(connect.CodeAlreadyExists, …)`. Client surfaces a
  generic toast and refreshes the transfer context so the button morphs
  to "In line".
- **Pros:** Most explicit. Clean separation between create and read.
- **Cons:** Worse retry UX — a benign retry now flashes an error. Forces
  the client to handle the error code specifically (translate to a
  benign "you're already in line" toast or fold it into a silent
  refresh). Adds a `connect.Code` to the public API contract.
- **Effort:** Small–medium. Slightly more client work than Option A.

### Option C: Client-only fix (no server change)

- **Approach:** Just gate the button on `state.isSaving` and add the
  early-return guard in the viewmodel. Skip the server change entirely.
- **Pros:** Smallest diff. Solves the reported case (a single device
  double-tapping).
- **Cons:** Doesn't protect against any non-UI client (web admin tool,
  future bot, retry middleware, mis-built version of the app). The
  server stays an unbounded queue-of-one-user. The fact that the bug
  report itself came from PROD suggests we should not rely on the UI
  alone.
- **Effort:** Small.

## Recommendation

**Option A** — idempotent server + client guard. It's the smallest set of
changes that closes the bug at both the layer that prevents it (UI) and
the layer that *guarantees* it can't happen (server). Idempotency is
strictly nicer than a 409 here: the natural client behaviour on retry is
"land in 'in line' state", which is exactly what returning the existing
transfer produces with no special-case error handling. Option C is
tempting because the diff is tiny, but the bug was filed from PROD where
the client tier is already several builds behind HEAD; the server-side
fix is what protects users on older builds.

## Open Questions

- **What counts as "non-terminal" for the duplicate check?** —
  *Recommendation:* exclude `TRANSFER_STATE_COMPLETED` and
  `TRANSFER_STATE_CANCELLED`. So `INTEREST_EXPRESSED`,
  `RECIPIENT_SELECTED`, and `ACTIVE` all dedupe to the existing transfer.
  This matches the client's own `buildGearTransferContext` filter
  (`context.go:110-111`).
- **Should the idempotent path re-emit the chat system message and
  community event?** — *Recommendation:* No. The duplicate-detection
  branch should return early *before* the `getOrCreateGearConversation`
  / `recordCommunityEventForTransfer` calls. Returning the existing
  transfer in the response is enough for the client to refresh state.
  Owners should not be notified twice for the same interest.
- **Do we cover giveaways too?** — *Recommendation:* Yes. The bug
  report is about loans, but `ExpressInterest` is a shared code path
  for both. The dedupe logic naturally covers both transfer types with
  the same query. The Flutter `buildGiveaway` factory already accepts
  `isLoading`; we just need to pass `state.isSaving` through from
  `GearActionsSection`.
- **Should we also add a partial-unique DB index?** — *Recommendation:*
  Not in this PR. ProtoSQL storage doesn't manage indexes through the
  Go layer in the same way; the right place for that is a separate
  schema change. File a follow-up issue if we see the read-then-write
  race in production.
- **Do we need new ARB strings or accessibility labels?** —
  *Recommendation:* No. The fix disables the existing CTA during
  the in-flight window via `MorphingActionButton.isLoading: true` /
  `onAction: null`; the existing `semanticsLabel: preActionLabel`
  stays valid because the visible label is unchanged. `Tappable`
  with `onTap: null` (set inside `MorphingActionButton._buildCTA`
  when loading) is announced as a disabled button by screen readers,
  which is the correct semantic. No new `app_en.arb` / `app_es.arb`
  keys needed — i18n parity (`npm run lint:dart:l10n-parity`) is not
  affected.
- **CommunityEventType impact?** — *Recommendation:* None. The
  dedupe branch deliberately skips
  `recordCommunityEventForTransfer`, so no new
  `CommunityEventType` enum values are introduced and the
  `TestEveryEventTypeClassified` registry test (per CLAUDE.md
  §"Adding New CommunityEventType Values") is unaffected.
- **What's the right log level when we hit the dedupe path?** —
  *Recommendation:* `Info` with `operation=ExpressInterest`,
  `dedupe=true`, `transfer_id=<existing>`, so the path is observable
  in prod without becoming noisy debug spam. (`transfer_id` is the
  standard entity field per `docs/server/observability.md`; the
  `dedupe` boolean distinguishes the idempotent branch from the
  insert branch in queries.)

## Implementation Plan

### Phase 1: Server-side dedupe in ExpressInterest

**Goal:** A second `ExpressInterest` call from the same user for the same
gear/community is idempotent — it returns the existing transfer instead
of inserting a new row.

**Tasks:**
- [ ] In `server/services/transfer/interest.go`, after
      `findMatchingCommunityAndType` returns `communityID, transferType`
      (around `interest.go:50`), query existing transfers:
      `QueryByFields({"gear_id": gearID, "recipient_id": authInfo.UserID, "community_id": communityID}, &models.Transfer{})`.
- [ ] Iterate the results; if any row has state `INTEREST_EXPRESSED`,
      `RECIPIENT_SELECTED`, or `ACTIVE`, log
      `logger.InfoContext(ctx, "express interest is idempotent; returning existing transfer", "operation", "ExpressInterest", "transfer_id", t.Id, "state", t.State.String(), "dedupe", true)`,
      call `s.buildTransfer(ctx, t)` for the API view, and return the
      response. Do NOT touch watches, conversation membership, system
      messages, or community events on this branch — they were created
      when the original transfer was created. (Audit note: per
      `docs/server/observability.md`, the entity ID field is the
      conventional `transfer_id`; signal dedupe via a separate
      `dedupe=true` field rather than a non-standard
      `existing_transfer_id` key.)
- [ ] Make sure the dedupe block runs **before** `createNewTransfer` so
      the new-row path is unchanged for genuinely new requests.
- [ ] Add `TestService_ExpressInterest_DuplicateCall_LoanReturnsExistingTransfer`
      and `_GiveawayReturnsExistingTransfer` in
      `server/services/transfer/interest_test.go`. Each test calls
      `ExpressInterest` twice with the same context, asserts that the
      second response's `Transfer.Id` equals the first, and asserts that
      `QueryByField(ctx, "gear_id", gearID, &models.Transfer{})` returns
      exactly one row. Use the existing test helpers
      (`setupTestServiceWithNotifications`, `setupTestGear`,
      `setupCommunityAndShareGear`).
- [ ] Add a third test
      `TestService_ExpressInterest_AfterWithdraw_AllowsNewTransfer`:
      call `ExpressInterest`, then `WithdrawInterest` (transitions to
      `CANCELLED`), then `ExpressInterest` again; assert two transfer
      rows exist (one CANCELLED, one new and non-terminal). This locks
      the "cancelled transfers don't block re-borrow" rule into the
      test suite.

**Validates:** Server alone now prevents the duplicate queue entry that
the user reported, even against a stale or unfixed client.

### Phase 2: Client-side guard (viewmodel + button)

**Goal:** A second tap during the first request's in-flight window is a
no-op at the viewmodel layer; the button is visibly disabled (loading
spinner / dimmed) while the request is in flight.

**Tasks:**
- [ ] In `app/lib/presentation/viewmodels/gear_view_model.dart`,
      `expressInterest()` (line 880): add
      `if (state.isSaving) return;` as the first statement after the
      `gearId == null` guard. Matches the pattern in `saveChanges`
      (line 474). Do the same in `withdrawInterest()` (line 938) for
      symmetry, since the same double-tap race exists for withdrawal.
- [ ] In `app/lib/presentation/screens/gear/widgets/gear_actions_section.dart`:
      - Loan path (line 128): pass
        `isLoading: state.isSaving` to the `MorphingActionButton` and
        change the `onAction` predicate from `!hasRequested` to
        `!hasRequested && !state.isSaving` so the button is disabled
        during the in-flight window.
      - Giveaway path (line 80): pass `isLoading: state.isSaving` to
        `WorkflowActionButtons.buildGiveaway`. The factory already
        threads `isLoading` through to `MorphingActionButton`.
- [ ] Add `expressInterest is no-op while isSaving` and
      `withdrawInterest is no-op while isSaving` tests to
      `app/test/presentation/viewmodels/gear_view_model_test.dart` (the
      existing `expressInterest` group starts at line 1611). Use
      `notifier.state = notifier.state.copyWith(isSaving: true)` as a
      precondition (the pattern already in use at line 473) and assert
      with `verifyNever(mockTransferRepository.expressInterest(…))`.
      Per `docs/client/testing.md`, these are pure ViewModel tests on
      `ProviderContainer` with the `transferRepositoryProvider`
      overridden by the generated Mockito mock — no widget tree, no
      l10n delegates, no real services.
- [ ] Run `cd app && flutter test --reporter github` to confirm.

**Validates:** End-to-end, a double-tap on the borrow button no longer
fires two RPCs; the existing visual states still work; cancelled
transfers can still be re-requested.

### Phase 3: Manual smoke + close-out

**Goal:** Confirm the fix in a running app and close the issue.

**Tasks:**
- [ ] Run `npm run generate && npm run build && go test -tags=integration -timeout 20m ./...` from the repo root.
- [ ] Run `cd app && flutter test --reporter github`.
- [ ] Optional manual smoke: launch the app, tap "Borrow" twice in
      quick succession on a community-shared gear item, confirm only
      one entry appears in the borrower list and only one Transfer
      row in the DB.
- [ ] In the PR body, link back to this issue with `Fixes #2116`.

**Validates:** The reported user-visible bug is gone.

## Audit Notes

Ran against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/client/accessibility.md`,
`docs/client/testing.md`, `docs/server/architecture.md`,
`docs/server/observability.md`. Adjustments applied inline:

- **Logging field naming:** changed `existing_transfer_id` → `transfer_id`
  (the canonical entity field) plus a separate `dedupe: true` flag.
- **Accessibility:** explicitly confirmed the disable-while-loading
  approach preserves correct screen-reader semantics via the existing
  `Tappable(onTap: null)` path inside `MorphingActionButton`; no new
  a11y primitives or ARB keys required.
- **i18n:** confirmed no new strings; `npm run lint:dart:l10n-parity`
  is not affected.
- **CommunityEventType registry:** confirmed dedupe path emits no
  new events, so `server/undo/registry.go` and
  `TestEveryEventTypeClassified` are unaffected.
- **Client testing layer:** added an explicit note that the new tests
  follow the ViewModel-first pattern (ProviderContainer + generated
  Mockito mocks), no widget tree.

Other concerns checked and found OK: server/client boundary
(`ViewModel → Repository → Service` preserved, no new cross-service
dependency); cache invalidation in `transfer_repository.dart` is
already triggered on the existing `expressInterest` path and will
continue to fire correctly when the server returns an existing
transfer ID; the optimistic-update fallback at
`gear_view_model.dart:926-928` works unchanged because the existing
transfer's protobuf shape is identical.

## Related Issues

- #1684, #1685 — earlier work on the same `expressInterest` →
  optimistic refresh flow; the comment block at
  `gear_view_model.dart:925` calls these out. Worth a look to confirm
  the early-return guard doesn't regress the optimistic-update
  behaviour those issues introduced.
- #1705 — the multi-community gear `community_id` handling that
  `expressInterest` and `withdrawInterest` both reference; the new
  duplicate check must use the same server-authoritative
  `community_id` from `findMatchingCommunityAndType`, not
  `state.communityId`, for the same reason.
