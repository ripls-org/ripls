# Issue #2170: GetMedia enforcement (#1529) would block loan/transfer inbox gear thumbnails

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2170
**Filed:** 2026-05-25 | **Last activity:** 2026-05-25
**Labels:** bug, security, server, privacy | **Assignees:** unassigned
**Context docs:** `docs/issues/1529-getmedia-auth.md` (the access-rule design); related #1529

## Problem Statement

The per-resource `GetMedia` access rule from #1529 (`canUserAccessMedia` in
`server/services/media/authorization.go`) is running in **log-only** mode. When
it is flipped to enforce (`requireMediaReadAccess`), it will wrongly return
`PermissionDenied` for **gear thumbnails in the loan/transfer surfaces**,
because the rule has no "transfer participant" path.

Root cause is now pinned precisely: **soft-deleted gear.**

- `GetGearTransferContext` loads gear with `IncludeDeleted: true`
  (`server/services/transfer/service.go:195`) and populates the API
  `Transfer.gear_media_id` from a soft-deleted gear's `MediaIds[0]`
  (`transfer/service.go:270-279`). The client then resolves that media id
  through the `GetMedia` RPC (`inbox_row.dart:160` / `gear_info_card.dart:115`
  → `media_repository.dart:28`).
- But the rule's `gearAllows` uses `QueryByArrayContains` which **excludes**
  soft-deleted rows by default (`storage/array_columns.go:191` +
  `buildDeletedFilter`, `storage/protosql_schema.go:243`). So for soft-deleted
  gear it returns zero hits → `deny:no-gear-link` → the universal fallthrough
  `deny:not-owner-or-avatar`.

That is exactly the prod signature: 9/10 log-only denials carry
`deny:not-owner-or-avatar` and fire inside `GetGearTransferContext` /
`GetPortfolioInboxFeed` sessions. The denied callers are real users who
borrowed/lent gear that was later deleted — not attackers.

The deliverable for **this** issue is to add a transfer-participant surface to
the access rule so the loan history keeps working. The actual enforcement flip
remains #1529's deliverable, gated on this.

## Context from Comments

No comments — I authored the issue after a prod-log investigation. The body,
the prod telemetry, and the codebase research below are the full context. The
fix shape (Option A) is therefore my recommendation, not a maintainer decision;
flag for review.

## Codebase research findings

1. **The access rule's surface helpers** (`authorization.go:75-428`) all share
   one shape: `QueryByArrayContains("media_ids", id, &models.X{})` (or a direct
   query) → per-row membership/participation check → a specific deny reason
   (`deny:no-X-link` when the probe is empty, `deny:no-X-membership` when rows
   exist but the caller fails the check). `mostSpecificDeny` (`:189`) upgrades
   the rollup only on a membership-deny; `isMembershipDeny` (`:196`) enumerates
   those. A transfer surface slots into this pattern directly.

2. **`models.Transfer`** (`proto/ripls/models/transfer.proto`) has **no media
   field** — only `gear_id`, `owner_id`, `recipient_id`, `shadow_recipient_id`,
   `state`, `community_id`, `deleted`. So the only stored media↔transfer link is
   media → `Gear.media_ids` → `Transfer.gear_id`. The soft-deleted gear row
   persists (gear delete is soft — `server/services/gear/service.go:889-898`),
   so its `media_ids` are still queryable **if** the probe opts into
   `IncludeDeleted`.

3. **`QueryByArrayContains` accepts `opts ...QueryOptions`**
   (`storage/array_columns.go:170-176`), so `storage.QueryOptions{IncludeDeleted:
   true}` is available — this is the key that lets a transfer probe see the
   soft-deleted gear that severed the `gearAllows` link. `GetByID`,
   `QueryByField`, and `GetByIDs` all default to excluding soft-deleted and all
   accept the same option.

4. **The two surfaces differ in deleted-gear handling:**
   `GetGearTransferContext` (`transfer/service.go:195`) deliberately includes
   deleted gear and surfaces its media id; the portfolio inbox feed
   (`portfolio/fetch.go:563-572`) **skips** deleted gear (`bgMediaID` stays
   empty), so the deleted-gear thumbnail leak is the transfer-context path.
   Both appeared in the prod sessions; the GetMedia denials track the
   transfer-context path.

5. **What is NOT affected** (verified, keep out of scope): the invite/share
   preview builds presigned URLs directly from the bucket
   (`web/item_preview.go:236`, `login/registration.go:560`) and never calls
   `GetMedia`; unauthenticated callers hit `RequireAuth` (`media/service.go:401`)
   and get `Unauthenticated`, never `PermissionDenied`.

6. **Existing tests** live in `server/services/media/authorization_test.go`
   (per-surface unit tests calling `canUserAccessMedia` directly with
   `SetupTestStorage`) and `service_test.go` (RPC-level, already exercises
   soft-delete via `storage.QueryOptions{IncludeDeleted: true}`). The new
   surface gets unit tests in the former.

## Potential Solutions

### Option A (recommended): add a transfer-participant surface to `canUserAccessMedia`

- **Approach:** New helper `transferAllows(ctx, s, userID, mediaID)`:
  1. `gears := s.QueryByArrayContains(ctx, "media_ids", mediaID, &models.Gear{}, storage.QueryOptions{IncludeDeleted: true})` — **include soft-deleted** (the crux: this is the link `gearAllows` can't see).
  2. empty → `deny:no-transfer-link`.
  3. for each gear: `transfers := s.QueryByField(ctx, "gear_id", gear.Id, &models.Transfer{}, storage.QueryOptions{IncludeDeleted: true})`; allow (`reason="transfer"`) if any has `OwnerId == userID || RecipientId == userID`.
  4. else `deny:no-transfer-participation` (add to `isMembershipDeny`).
  Wire it as a new probe in `canUserAccessMedia` after `gearAllows`, add the
  three reason constants, leave the handler in **log-only**.
- **Pros:** fixes the authorization model centrally and correctly — *any*
  `GetMedia` caller who is a transfer participant is allowed, not just the two
  inbox RPCs. Survives soft-delete and community-departure. Mirrors the existing
  surface pattern exactly; small, testable diff. No proto/client change.
- **Cons:** authorizes against soft-deleted gear (intentional, but unusual for
  an authz rule — must be called out). Adds ~2 indexed queries on the deny path
  (cheap; short-circuited after owner/avatar). Grants a *historical* participant
  durable read access to that gear's media (see Open Questions — this is the
  intended loan-history UX).
- **Effort:** Small.

### Option B: return presigned thumbnail URLs directly from the transfer/inbox RPCs

- **Approach:** `GetGearTransferContext` (and the inbox feed) already load the
  gear (incl. deleted) and have the media id + owner id; have them presign the
  thumbnail URL directly (the `item_preview.go:236` pattern) so the client never
  calls `GetMedia` for these surfaces.
- **Pros:** removes the `GetMedia` dependency for the exact broken surfaces; no
  access-rule change; naturally handles deleted gear.
- **Cons:** proto + client changes across two RPCs; sidesteps rather than fixes
  the rule, so any *other* current/future surface that renders transfer/deleted
  gear media via `GetMedia` still breaks; pushes presigned-URL TTL/caching
  concerns into these responses (`media/service.go:448` notes the 35-min expiry
  vs client cache TTL coupling).
- **Effort:** Medium.

### Option C: enforce anyway, accept the breakage

- **Rejected.** Breaks a core legitimate flow; prod telemetry shows the denials
  are live loan-history traffic, not abuse.

## Recommendation

**Option A.** It fixes the authorization model where the gap actually is —
`canUserAccessMedia` doesn't know transfer participants are legitimate readers —
so the fix holds for every `GetMedia` caller, not just today's two inbox RPCs.
The diff is small and matches the established surface-helper pattern, and the
one unusual aspect (querying soft-deleted gear) is exactly what the bug
requires: the durable authorization is the *transfer relationship*, not the
gear's current community sharing, so the rule must look past the soft-deleted
gear row to the transfer.

Option B is a reasonable complement if we later want transfer thumbnails to
stop depending on `GetMedia` for performance reasons, but on its own it leaves
the rule wrong.

## Open Questions

- **Should a *historical/completed* transfer grant durable media access?** —
  Recommendation: **yes.** The user legitimately interacted with the gear and
  still sees it in loan history; revoking on completion would re-break the
  exact view this fixes. Match on any transfer state.
- **Include soft-deleted *transfers* (not just gear) in the probe?** —
  Recommendation: **include the gear lookup's `IncludeDeleted` unconditionally**
  (essential), and **also include deleted transfers** — a torn-down transfer is
  precisely the history case, and the cost is nil. Low stakes; default-exclude
  is acceptable if a reviewer prefers it.
- **`shadow_recipient_id` (non-registered recipient)?** — Recommendation: no
  special handling; there's no authenticated `userID` to match, and shadow
  recipients don't call `GetMedia`. Match only `owner_id` / `recipient_id`.
- **Does the owner case need this at all?** — Recommendation: the gear owner is
  usually already allowed via the `owner` surface (`media.UserId == userID`);
  the transfer surface primarily rescues the **recipient/borrower**. Match both
  roles anyway for correctness against ownership edge cases.
- **Is authorizing against soft-deleted rows acceptable to the security
  reviewer?** — Recommendation: yes, with the rationale documented in the helper
  comment; the transfer row is the durable grant. Call it out explicitly in the
  PR for #1529 sign-off.

## Implementation Plan

### Phase 1: Add the transfer-participant surface (log-only)
**Goal:** Close the rule gap so transfer participants are allowed, without yet
enforcing.
**Tasks:**
- [ ] Add reason constants in `authorization.go`: `accessReasonTransfer =
      "transfer"`, `accessReasonDenyNoTransferLink = "deny:no-transfer-link"`,
      `accessReasonDenyNoTransferParticipation = "deny:no-transfer-participation"`;
      add the participation reason to `isMembershipDeny`.
- [ ] Implement `transferAllows(ctx, s, userID, mediaID)` per Option A, using
      `storage.QueryOptions{IncludeDeleted: true}` on the gear (and transfer)
      lookups; document the soft-delete rationale in the doc comment.
- [ ] Wire it into `canUserAccessMedia` as a new probe after `gearAllows`
      (`authorization.go:144`), following the existing `ok/reason/err` +
      `mostSpecificDeny` shape. Note the ordering effect: placing it after gear
      means a true non-participant who is also a non-member surfaces
      `deny:no-transfer-participation` (last membership-deny wins) rather than
      `deny:no-gear-community-membership` — acceptable, but call it out so the
      telemetry shift is expected.
- [ ] **SQL efficiency (CLAUDE.md SQL rules):** the probe loops gears and runs
      one `QueryByField` per gear — identical in shape to `gearAllows`
      (`authorization.go:299-310`) and bounded (a given media id is normally on
      one gear). Confirm `Transfer.gear_id` is indexed (the transfer service
      already filters transfers by `gear_id`, so it should be); add the index if
      missing (SQL rule #6). Do **not** query inside a per-transfer loop — the
      owner/recipient check is in-memory on the already-fetched rows.
- [ ] Keep `GetMedia` on `canUserAccessMedia` (log-only). No handler/enforcement
      change in this issue. (When #1529 flips to `requireMediaReadAccess`, the
      transfer probe's storage errors must surface as `CodeInternal` via
      `connecterr.Internal` at the handler — that wiring is #1529's, but the
      probe returns wrapped errors today so it's ready.)
**Validates:** Transfer participants now resolve `reason="transfer"` (allow)
instead of a would-deny; rule covers the gap.

### Phase 2: Tests
**Goal:** Lock the new surface and prove it doesn't over-grant.
**Tasks:**
- [ ] Unit tests in `authorization_test.go` (pattern: `SetupTestStorage` +
      direct `canUserAccessMedia` calls):
  - [ ] **soft-deleted gear, recipient allowed:** owner `A` uploads gear+media,
        soft-delete the gear, create a `Transfer{gear_id, owner_id:A,
        recipient_id:B}`; assert `canUserAccessMedia(B, media)` → allowed,
        `reason == "transfer"`.
  - [ ] **live gear, non-member borrower allowed:** gear not shared to any
        community `B` is in, but `B` is the transfer recipient → allowed via
        transfer (proves it also rescues the community-departure case).
  - [ ] **true non-participant denied:** stranger `C` (not owner/recipient, not a
        community member) → denied; reason `deny:no-transfer-participation` when a
        transfer exists for the gear, else the universal fallthrough.
  - [ ] **owner still allowed** (via the owner surface) — regression guard.
- [ ] **N+1 guard:** wrap one allow-path assertion in
      `storage.AssertMaxQueries(t, ctx, N, fn)` (CLAUDE.md SQL rule #8) so a
      future refactor can't turn the gear/transfer probe into a per-row query
      storm. Pin `N` to the expected count for the single-gear case.
- [ ] Run `npm run build` and `go test ./server/services/media/...`.
**Validates:** Participants allowed, non-participants still denied, reason codes
correct, query count bounded.

### Phase 3: Re-soak telemetry, then hand the enforcement flip back to #1529
**Goal:** Confirm in prod the gap is closed before anyone enforces.
**Tasks:**
- [ ] After Phase 1+2 deploy, re-query `ripls-prod` for `would deny media read`
      over a fresh window; confirm the `deny:not-owner-or-avatar` /
      transfer-session denials drop to ~0 (the new path now logs
      `media access allowed reason=transfer`).
- [ ] Comment the re-soak result on #1529; the `canUserAccessMedia` →
      `requireMediaReadAccess` flip is **#1529's** deliverable and proceeds only
      after this confirmation.
**Validates:** Enforcement can be turned on without 403-ing loan history.

## Related Issues
- #1529 — per-resource `GetMedia` auth + the enforcement flip this unblocks.
  This issue must merge and re-soak clean first.
- `docs/issues/1529-getmedia-auth.md` — the access-rule design + audit; update
  it to list the transfer surface once Phase 1 lands.

## Audit Notes (architecture compliance)

Reviewed against `docs/server/architecture.md`, `docs/server/observability.md`,
and the client docs. Changes folded into the plan above; rationale here.

**Server architecture — compliant.**
- *Service independence:* `transferAllows` reads `models.Transfer` rows directly
  through the shared `storage.ProtoSQLStorage`, not the transfer *service* — the
  same pattern `gearAllows`/`chatAllows` already use (reading `models.Gear` /
  `models.ChatConversation`). No service-to-service dependency introduced.
- *SQL efficiency (Phase 1):* added an explicit note that the gear loop +
  per-gear `QueryByField` mirrors `gearAllows` and is bounded, a task to confirm
  the `Transfer.gear_id` index, and a guard against querying inside the
  per-transfer loop.
- *Error wrapping:* flagged that the eventual enforcement site (#1529) must route
  the probe's storage error through `connecterr.Internal`, not a bare
  `connect.CodeInternal` (CI-gated by `scripts/check_codeinternal.js`).

**Observability — compliant.** No new log lines are needed: `canUserAccessMedia`
returns the `reason`, which the existing log-only branch
(`media/service.go:436-445`) already emits. The new reason codes (`transfer`,
`deny:no-transfer-link`, `deny:no-transfer-participation`) carry no PII, so no
masking applies. Phase 3's re-soak reuses the existing `would deny media read` /
`media access allowed` telemetry.

**Testing — compliant + strengthened.** Unit tests in `authorization_test.go`
with `SetupTestStorage` match the established per-surface pattern and the arch
guide's "unit tests are always preferable." Added an `AssertMaxQueries` N+1
regression guard (Phase 2).

**Client docs — N/A for the recommended option.** Option A is server-only,
wire-compatible (no proto/ARB/widget/a11y change). The client caching
conventions (`docs/client/caching.md`: repository-owned, async, namespaced
keys) matter only to **Option B**, where presigned thumbnail URLs returned in
the transfer/inbox responses would be cached with those responses — the plan
already flags the presigned-URL TTL vs client-cache-TTL coupling
(`media/service.go:448`) as an Option-B cost.
