# Issue #1641: ListMyTransfers 500s when a referenced gear is soft-deleted

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1641
**Filed:** 2026-05-01 | **Last activity:** 2026-05-01
**Labels:** none | **Assignees:** unassigned
**Context docs:** none
**Audited against:** `docs/server/architecture.md`, `docs/server/observability.md`, `docs/client/accessibility.md`, `docs/client/i18n.md`, `docs/client/testing.md`, `docs/client/caching.md`

## Problem Statement

Four transfer-listing RPCs — `ListMyTransfers`, `ListReceivedTransfers`,
`GetGearTransfers`, and `ListTransfers` — return `connect.CodeInternal` ("gear
`<id>` not found") whenever any of the user's transfers references a gear that
has been soft-deleted. Because the failure happens inside the batch enrichment
step (`buildTransfers`), a single orphaned transfer poisons the whole list:
the user cannot load **any** transfers until the bad row is gone.

Two prod alerts within two minutes for the same user/gear pair on
2026-04-30 confirm this is a real, hit-in-the-wild bug, not just a theoretical
edge case. The user retried (or the client auto-retried) and the second
request also failed — so this is a sticky outage for affected users, not a
transient.

`GetTransfer` (single-row fetch) has the same root cause via `buildTransfer`
→ `buildTransfers`, so any direct deep link into a transfer with a deleted
gear is also broken.

`GetUserTransferStatus` also calls `buildTransfer`, but only on
non-terminal transfers (`!IsTerminalState(transfer.State)` guards at
`queries.go:274,310`). Since `DeleteGear` cancels every active transfer
before soft-deleting the gear (terminal state `CANCELLED`), this RPC is
effectively immune in practice — but the fix in `buildTransfers` covers
it for free, with no special-casing needed.

## Context from Comments

The issue has no comments yet — I filed it during this session immediately
after diagnosis. The body already captures the root-cause analysis,
recommended fix, and alternatives.

## Codebase Findings

**The bug:**

- `server/services/gear/service.go:823-842` (`DeleteGear`): cancels active
  transfers (sets state to `CANCELLED`) but does **not** soft-delete the
  `Transfer` rows. Completed transfers are also left untouched.
- `server/services/transfer/service.go:192` (`buildTransfers`): calls
  `storage.GetByIDs[*models.Gear](...)`, which excludes soft-deleted rows by
  default per `server/storage/protosql_batch.go:24`.
- `server/services/transfer/service.go:248-253`: when the gear lookup misses,
  returns `connecterr.Internal(ctx, "buildTransfers", fmt.Errorf("gear %s not
  found", t.GearId))`. There is no fallback path.

**Established pattern for the fix.** Other services that enrich historical
records already pass `IncludeDeleted: true` to gear lookups, exactly because
they have to render rows whose gear may have been deleted:

- `server/services/community/gear_sharing.go:67`
- `server/services/impact_metrics/user_impact_detail.go:81`
- `server/services/impact_metrics/utilization.go:57`
- `server/impact_metrics/calculator.go:69, 132, 157, 270, 302, 333, 726, 770`

Transfers were apparently overlooked when this pattern was rolled out. The
fix is conventional, not novel.

**Adjacent code that's already correct:**

- `FetchAPIUsersBatch` (`server/services/user_util.go:67`) silently omits
  missing/soft-deleted users. `buildTransfers` reads the result with
  `userMap[t.OwnerId]` and tolerates `nil` — owner/recipient deletion does
  not cause a 500. So users-side is already permissive; only the gear-side is
  strict.
- The conversation lookup at `service.go:238` already logs-and-continues on
  failure (`convMap = nil`). So the 500 path is uniquely the gear lookup.

**Test infrastructure exists.** `server/services/transfer/queries_test.go`
(`TestService_ListMyTransfers`, `TestService_ListReceivedTransfers`,
`TestService_ListTransfers`) and `build_transfer_test.go` already use the
same harness (`setupTestServiceWithNotifications`,
`setupCommunityAndShareGear`, `ExpressInterest`) we'd need to drive the
regression test. Per `docs/server/architecture.md` §Testing, unit/local
tests are the preferred level — this bug is unit-testable without lifting
the full integration harness.

**Wire-format note (relevant to Phase 3 below).** `api.Transfer` carries
`gear_name` and `gear_media_id` denormalized directly on the transfer —
it does not nest an `api.Gear`. Neither `api.Gear` nor `api.Transfer`
exposes a `deleted` field today (only `models.Gear` does, at field 20).
So Option A surfaces nothing new to the client: enriched transfers for
deleted gear look identical on the wire. A "this gear was deleted"
affordance therefore needs a proto change — not just a client edit —
which is why we're deferring it.

## Potential Solutions

### Option A: Pass `IncludeDeleted: true` to gear `GetByIDs` in `buildTransfers`

- **Approach:** One-line change at `service.go:192`. Optionally surface a
  `gear_deleted` boolean (or just trust the existing `Deleted` metadata on
  the gear proto) so clients can render a "this gear was removed" affordance.
  Mirror the existing pattern in `community/gear_sharing.go` and
  `impact_metrics/`.
- **Pros:** Smallest change. Matches the established convention. Preserves
  history visibility for both owner and recipient. Fixes all four RPCs and
  `GetTransfer` in a single line. No proto changes, no migration.
- **Cons:** The client cannot distinguish a transfer with deleted gear
  from one with live gear, because `api.Transfer` only carries
  denormalized `gear_name` / `gear_media_id` and no `deleted` flag (and
  `api.Gear` has no `deleted` field either). For Phase 1/2 this is a
  *feature*, not a defect — the response just looks normal to the
  client. A "removed" pill would require a proto addition (deferred).
- **Effort:** Small (≈1 line + 1 regression test + optional client polish).

### Option B: Filter orphaned transfers out in `buildTransfers`

- **Approach:** When a transfer's gear is missing from `gearMap`, skip it
  with a `WarnContext` log and continue building the rest.
- **Pros:** Restores list functionality without exposing deleted gear to the
  client at all. No need to think about deleted-gear UI states.
- **Cons:** Silently hides legitimate history from the user. A completed
  loan they once made disappears from their list when the gear is deleted —
  surprising and probably wrong. Also masks future bugs where a transfer
  legitimately points at a missing gear (e.g. a non-soft-delete bug).
  Diverges from the convention used elsewhere.
- **Effort:** Small.

### Option C: Cascade-soft-delete transfers in `DeleteGear`

- **Approach:** In `DeleteGear`, after cancelling active transfers, also set
  `Deleted` metadata on every transfer for the gear. Then the listing RPCs
  naturally exclude them.
- **Pros:** Keeps the listing RPCs strict (every gear they reference is
  guaranteed live). Conceptually clean: "if the gear is gone, so is its
  transfer history."
- **Cons:** Hides history from the recipient/owner of a transfer they
  participated in — they didn't delete the gear, but their record of having
  borrowed/given it disappears. Bigger blast radius (touches `DeleteGear`,
  not just the read path) and we'd want a backfill job for the orphans
  already in prod (otherwise affected users stay broken until someone
  deletes the gear again, which won't happen). Diverges from the
  history-preserving convention used by impact metrics, etc.
- **Effort:** Medium (DeleteGear change + backfill job + tests).

## Recommendation

**Option A.** It's the conventional fix already used in three or four sibling
services for the same scenario, it's surgical, it preserves history (which
matches user intuition: "I borrowed that lawnmower last summer" should still
show up even if the lender later deleted the listing), and it fixes all
affected RPCs in one place. Options B and C both lose history that the user
expects to keep.

The only real concern with Option A is client rendering: if the gear was
deleted, should the UI show a "removed" pill? Defer to a follow-up issue —
it requires a proto change (`api.Transfer` has no `gear_deleted` field
today) plus client work, and is out of scope for the prod-fire fix.

## Open Questions

- **Should the client render a "this gear was deleted" affordance for
  orphaned transfers?** — Recommendation: defer to a follow-up. Server fix
  ships first to stop the 500s. Implementing the affordance requires (a)
  adding `bool gear_deleted` (or similar) to `api.Transfer`, (b) populating
  it in `buildTransfers` from `gear.Deleted != nil`, and (c) Flutter work
  using accessibility-compliant primitives per `docs/client/accessibility.md`
  — i.e. any tappable affordance uses `Tappable` / `IconAction` with a
  `semanticsLabel` pulled from `context.l10n.a11y*`, not raw
  `GestureDetector` / string literals (`scripts/check_a11y.js` will
  enforce). The "Removed" badge text itself must be an l10n key per
  `docs/client/i18n.md`. None of that is needed for the prod fix.
- **Should we also pass `IncludeDeleted: true` for the conversation lookup
  at `service.go:238`?** — Recommendation: no. Conversations are already
  cascade-soft-deleted with the gear (`gear/service.go:858`,
  `CascadeDeleteConversationsByTopic`), so the existing log-and-continue
  fallback (`convMap = nil`) handles them correctly. Adding `IncludeDeleted`
  there would *resurrect* deleted chat threads in the response, which is
  worse than the current behavior.
- **Do we need a one-time data fix or backfill?** — Recommendation: no.
  Once Option A ships, the orphaned-but-already-existing transfer rows just
  start working again. No migration needed.
- **Should `DeleteGear` start soft-deleting transfers anyway, as a
  belt-and-suspenders measure?** — Recommendation: no. It's redundant with
  Option A and would conflict with the "preserve history" semantics we're
  picking. Pick one model and stick with it.

## Implementation Plan

### Phase 1: Server fix + regression test

**Goal:** Stop the 500s on all four transfer-listing RPCs and `GetTransfer`.

**Tasks:**

- [ ] In `server/services/transfer/service.go`, change `buildTransfers` to
      pass `storage.QueryOptions{IncludeDeleted: true}` to
      `storage.GetByIDs[*models.Gear](...)`. Leave the `FetchAPIUsersBatch`
      and conversation lookups alone — users are already nil-tolerant
      downstream, and conversations are intentionally excluded when their
      cascade-deleted parent gear goes away.
- [ ] Audit the existing error-wrap shape at `service.go:252`. It already
      uses `connecterr.Internal(ctx, "buildTransfers", fmt.Errorf("gear %s
      not found", t.GearId))`, which conforms to the wrapping convention
      in `docs/server/architecture.md` §"Error wrapping" (the wrapped
      error is logged, not serialized). After the fix this branch becomes
      genuinely unreachable in normal flow, but keep it as a defensive
      assert — a future bug that *truly* leaves an orphaned transfer
      (e.g. hard-deleted gear, manual DB intervention) should still surface
      loudly per `CLAUDE.md` "Fail Fast" guidance.
- [ ] Add a regression test in `server/services/transfer/queries_test.go`
      (or a new `orphaned_gear_test.go` next to it). Per
      `docs/server/architecture.md` §Testing, prefer the unit-test layer:
  - Create owner, recipient, gear; share gear in a community; recipient
    `ExpressInterest`; complete or cancel the transfer so it survives.
  - Soft-delete the gear directly against storage: set
    `gear.Deleted = &models.DeletedMetadata{...}` and call
    `s.storage.Update(ctx, gear)`. (Going through storage rather than
    `DeleteGear` isolates the read path from the cascade and avoids
    pulling the gear service into the transfer test.)
  - Assert `ListMyTransfers(ownerCtx)`, `ListReceivedTransfers(recipientCtx)`,
    `GetGearTransfers(ownerCtx, gearID)`, `ListTransfers(ownerCtx,
    communityID)`, and `GetTransfer(ownerCtx, transferID)` all succeed and
    return a transfer whose `GearName` is populated.
  - Use existing harness helpers (`setupTestServiceWithNotifications`,
    `setupCommunityAndShareGear`) from `common_test.go`. Use
    `services.WaitForNotification` where async fan-out is involved
    (matches the established `TestService_ListMyTransfers` pattern).
- [ ] Add a second test variant that goes through `DeleteGear` end-to-end
      (active transfer at deletion time → cancelled by `DeleteGear` →
      still listable afterwards) to cover the realistic prod scenario,
      including the cascade.
- [ ] Run `npm run build` (golangci-lint + `go build ./...`) and
      `go test ./server/services/transfer/...` locally. Also run
      `go build -tags=integration ./...` per `CLAUDE.md` if signatures
      change (they shouldn't here — this is a one-line internal change).

**Validates:** The exact prod-observed failure mode is fixed and stays
fixed. Before the fix, the new tests fail with `gear ... not found`; after,
they pass.

### Phase 2: Verify and ship

**Goal:** Confidence the fix is complete with no surprises.

**Tasks:**

- [ ] Audit `buildTransfer`/`buildTransfers` callers (`queries.go:61, 117,
      217, 428` plus `lifecycle.go` / `interest.go` if they call it after
      mutations) and confirm none of them care about the deleted-gear
      distinction; if any do, branch on `gear.Deleted != nil`.
- [ ] Grep for other call sites that follow the same anti-pattern (a
      historical-record enrichment doing `GetByIDs[*models.Gear]` without
      `IncludeDeleted: true`). The likely audit list:
  `server/services/feed/...`, `server/services/search/...`,
  `server/services/loan/...`, `server/services/request/...`,
  `server/services/experience/...`. Fix in this PR if trivially the same
  bug; file a follow-up issue if scope creeps.
- [ ] Run the full server test suite (`npm run build && go test ./...`).
- [ ] Open PR referencing #1641. Production verification: after deploy,
      confirm the alert fires no further occurrences and (if logs allow)
      that the affected user's transfer list now loads.

**Validates:** No silent regressions in adjacent code paths; bug is fixed
in production.

### Phase 3 (optional follow-up, NOT in this PR): client rendering

**Goal:** Surface "this gear was deleted" in the UI.

**Tasks (file as a separate issue, not in this PR):**

- [ ] Add `bool gear_deleted` to `api.Transfer` (new field at the next
      free number — do not reuse). Per `CLAUDE.md` proto conventions,
      use `optional` if there is any doubt about presence semantics.
- [ ] Populate it in `buildTransfers` from `gear.Deleted != nil`.
- [ ] In the Flutter transfer list / detail widgets, render a "Removed"
      pill and disable any "view gear" affordance when `gearDeleted` is
      true. Per `docs/client/accessibility.md`:
  - Tappable affordances must use `Tappable` / `IconAction` (not
    `GestureDetector` / `InkWell`) with a `semanticsLabel` from
    `context.l10n.a11y*` — `scripts/check_a11y.js` enforces this.
  - The pill text itself comes from `context.l10n.*` per
    `docs/client/i18n.md`.
- [ ] No new caching layer needed; per `docs/client/caching.md`, the
      transfer list already flows through repositories, and the
      `gear_deleted` bit is just another scalar on the cached transfer
      record.

**Validates:** Recipient/owner can clearly tell which entries refer to
gear that no longer exists.

## Related Issues

- None known. The branch name `gear_missing_bug` and the matching prod alert
  suggest this is a freshly-discovered bug. If a search of closed issues
  surfaces a duplicate during PR review, link it then.

---

## Audit Notes (2026-05-01)

Reviewed against `docs/server/architecture.md`, `docs/server/observability.md`,
`docs/client/accessibility.md`, `docs/client/i18n.md`,
`docs/client/testing.md`, and `docs/client/caching.md`. Changes applied to
the plan in this revision:

1. **Added `GetUserTransferStatus` to the affected-RPC list** with the
   note that it's effectively immune in practice (terminal-state guard)
   but is fixed for free.
2. **Corrected the proto-availability claim.** `api.Transfer` carries
   denormalized `gear_name` / `gear_media_id` and has no `deleted` flag,
   and `api.Gear` likewise has no `deleted` field. Only `models.Gear` does.
   The original plan implied the client could read deletion state today
   without a server change — that's wrong, and the deferred Phase 3 now
   reflects the proto-addition requirement.
3. **Reinforced the existing `connecterr.Internal` usage.** The
   gear-not-found branch at `service.go:252` already uses the wrapping
   helper required by `docs/server/architecture.md` §"Error wrapping" —
   no change needed, but the plan now explicitly justifies keeping the
   defensive branch (per `CLAUDE.md` "Fail Fast").
4. **Tightened the regression-test recipe** to name the existing harness
   helpers (`setupTestServiceWithNotifications`,
   `setupCommunityAndShareGear`, `services.WaitForNotification`) and the
   exact storage-level soft-delete pattern, matching conventions in
   `queries_test.go` / `common_test.go`.
5. **Added build-verification steps** consistent with the `CLAUDE.md`
   "Go Server Build Verification" guidance (`npm run build` plus the
   `-tags=integration` build when signatures change — flagged that this
   change shouldn't trigger that, but the plan calls it out so the
   author thinks about it).
6. **Made the deferred Phase 3 client work concrete and a11y/i18n-compliant.**
   Listed the `Tappable` / `IconAction` primitive choice, the
   `context.l10n.a11y*` label requirement, and the pill-text l10n key —
   so when this Phase 3 follow-up is filed, it inherits compliant
   tasks rather than re-inventing them.

No client-side caching, observability/logging, or terminology
("Event" vs "Experience") concerns surfaced — Phase 1/2 don't touch
client code or user-facing strings, and Phase 3 is appropriately
scoped to a separate issue.
