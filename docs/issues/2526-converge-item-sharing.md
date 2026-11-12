# Issue #2526: Converge item sharing onto ShareItem; remove ShareExperience/ShareGear/ShareRequest add-paths

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2526
**Filed:** 2026-06-20 | **Last activity:** 2026-06-20
**Labels:** none | **Assignees:** unassigned
**Context docs:** `proto_conventions.md` (anchor), `docs/server/architecture.md` + `docs/server/conventions.md` (server anchors), `docs/client/architecture.md` + `docs/client/conventions.md` (client anchors), `docs/workflows/item_sharing.md`, `docs/client/sharing.md`, `docs/issues/2492-phone-first-pivot.md` (CREATE-1 origin of `ShareItem`)

> **Scope note from the requester (this PR):** Do the **`UnshareItem`** side (marked "out of scope, eventual" in the issue) **in the same PR**. Mark **every** superseded per-type RPC — the three `Share*` **and** the three `Unshare*` — `deprecated = true` with a `TODO(#NNNN)`, roll out the new unified RPCs, migrate the clients, and **defer deletion** of the defunct RPCs to a follow-up. Nothing is removed in this PR.

> **Implementation status (branch `share_rpc`):** Phases 1–3 **done**.
> - **Server:** `UnshareItem` added (`server/services/community/unshare_item.go`), `ItemSharer.UnshareFromCommunity` hook wired for experience/gear/request; each per-type unshare handler refactored into a behavior-preserving core (`Unshare*FromCommunity`). All six per-type `Share*`/`Unshare*` RPCs marked `deprecated = true` with `TODO(#2526)`. Unit + integration sharing tests green.
> - **Client:** the per-type service wrappers now call `ShareItem`/`UnshareItem` (a `CommunityServiceClient` is built from each service's transport); repos/viewmodels/UI/bulk untouched. Gear share-time availability dropped (Open Question 1). `flutter analyze` clean; no hand-written Dart calls a deprecated per-type RPC.
> - **Housekeeping:** issue #2526 broadened (Open Question 2); `client/sharing.md` + `workflows/item_sharing.md` corrected. e2e seed helpers still call the deprecated RPCs deliberately (they migrate at removal time; the gear seed's availability-pinning moves to `SetGearAvailability` then).
> - **Deferred (not this PR):** deletion of the six deprecated RPCs — buf-breaking, app-min-version-gated (#2526 step 3).

## Problem Statement

Item sharing has fragmented into per-type RPCs that all do the same thing. The phone-first pivot (CREATE-1, #2492) already introduced **`CommunityService.ShareItem`** — one RPC that, given an item (`experience_id` / `gear_id` / `request_id` / `transfer_id`), (a) shares it with any number of existing communities (`share_to_community_ids`), and (b) invites individuals (members + off-app phone/email), provisioning the item's per-item ad-hoc community, minting the open `/go/` link, sending email invites, and persisting host consent. It is already the **add**-path for three UI surfaces.

That makes the per-type **community add-paths** redundant — each is exactly `ShareItem` with only `share_to_community_ids`, one community at a time:

- `ShareExperience` — `experience_service.proto` (**already** `deprecated = true`, `TODO(#2526)`)
- `ShareGear` — `community_service.proto`
- `ShareRequest` — `request_service.proto`

The issue's original plan: (1) deprecate each `Share*`, (2) migrate client callers, (3) remove once dead. It explicitly scoped **out** a uniform `UnshareItem`, leaving the per-type `Unshare*` RPCs in place because "`ShareItem` only **adds** audience."

**This plan expands that per the requester:** add the symmetric **`UnshareItem`** now, so the audience-management surface is uniform in both directions, and deprecate the per-type `Unshare*` RPCs (`UnshareExperience`, `UnshareGear`, `UnshareRequest`) alongside the `Share*` ones. All six get a `TODO(#2526)` and stay callable until a follow-up deletes them.

### Ambiguities / contradictions

- The issue body deliberately defers `UnshareItem`; the requester overrides that for this PR. The issue's **removal** step (delete the RPCs) stays deferred either way — this PR only *adds + deprecates + migrates*.
- `gen_request_view_model.dart:396` has a stale comment claiming request-creation sharing already goes "via ShareItem (#2529)", but the code still calls `shareRequest`. Migration must fix the code, not trust the comment.

## Context from Comments

**The issue has no comments** — the body is the sole spec, plus the requester's PR-scope note above. That is a small risk: the design of `UnshareItem` (field shape, whether to unify the three divergent per-type unshare semantics) has had no prior discussion, so the **Open Questions** below carry more weight than usual. Everything else is well-anchored by the already-merged `ShareItem` (a working reference implementation to mirror).

## Codebase findings (research)

**Server — `ShareItem` is registry-driven** (`server/services/community/share_item.go`). The `community` service holds an `itemSharers map[ItemKind]ItemSharer`, injected in `server/wiring.go:357-371` via `SetItemSharer` so it never imports the item-service packages. Each `ItemSharer` supplies method values:

```go
type ItemSharer struct {
    VerifyOwner      func(ctx, itemID, actorUserID string) error
    VerifyViewer     func(ctx, itemID, actorUserID string) (ownerID string, err error)   // link-only reshare gate (#2630)
    ShareToCommunity func(ctx, itemID, communityID, actorUserID string) error
}
```

Registered for `ItemKindExperience` (on `experienceService`), `ItemKindGear` (on `communityService`), `ItemKindRequest` (on `requestService`). **No transfer sharer is wired** → `ShareItem` with `transfer_id` already returns `CodeUnimplemented`. `ShareItem` enforces tiered permissions (#2630): a mutating call (invitees or `share_to_community_ids`) requires `VerifyOwner`; a link-only call is allowed for any viewer.

**Server — the three per-type `Unshare*` are NOT uniform.** `UnshareItem` must preserve each type's behavior (interface-first, behavior-preserving refactor):

| RPC | Removal mechanism | Extra behavior |
|---|---|---|
| `UnshareExperience` (`experience/unshare.go`) | **hard delete** `CommunityExperience` | rejects if target community soft-deleted; `NotFound` if not shared |
| `UnshareGear` (`community/gear_sharing.go:547`) | **hard delete** `CommunityGear` | **idempotent** no-op if not shared; **emits `GEAR_UNSHARED`** community event |
| `UnshareRequest` (`request/sharing.go:200`) | **soft archive** (`CommunityRequest.Archived = true`) | **blocks unsharing the last community** (`FailedPrecondition`); `NotFound` if not shared; no event |

All three are **owner-only**. Each already exposes an owner-check as a registry method value (`VerifyExperienceOwner` / `VerifyGearOwner` / `VerifyRequestOwner`) reused by `ShareItem`.

**Client — `ShareItem` exists and is wired for 3 surfaces; everything else is still per-type; there is no unified unshare at all.** Full call-site map (from a thorough sweep):

- **Already on `ShareItem`** (no change): experience creation (`gen_experience_view_model.dart:779` → `createAndShareExperience`), the share-sheet target/link (`item_share_sheet.dart:134,252`), member invites (`invite_members_sheet.dart:110`). The client `shareItem` (`community_service.dart:545`) already accepts `shareToCommunityIds`.
- **Per-type `Share*` still primary** (migrate → `ShareItem`):
  - Per-community toggle: `community_list_item.dart` → `experience_sharing_view_model.dart:58` / `request_sharing_view_model.dart:95` / `gear_sharing_audience_actions.dart:157`.
  - Creation flows: `gen_gear_view_model.dart:513` (gear), `gen_request_view_model.dart:423` (request).
  - Bulk: `bulk_community_selection_modal.dart` → `shareMultipleWithCommunity` on the gear/request/experience repos.
- **All `Unshare*` still per-type** (migrate → `UnshareItem`):
  - Toggle notifiers: `experience_sharing_view_model.dart:78` / `request_sharing_view_model.dart:147` / `gear_sharing_audience_actions.dart:132`.
  - Detail-screen confirm-remove handlers: `experience_close_handlers_mixin.dart:122`, `gear_transfer_handlers_mixin.dart:234` (direct repo), `request_content_view.dart:892`.
  - Direct repo bypass: `whos_in_manage_sheet.dart:247`.
  - Bulk: `bulk_community_selection_modal.dart` → `unshareMultipleFromCommunity` on all three repos.

**Migration watch-outs surfaced by research:**

1. **Gear carries `availability` (loan/giveaway) at share-time; `ShareItemRequest` does not.** Server-side, `ShareGearToCommunity` (the gear `ItemSharer` hook) already **inherits** the gear's existing availability (`existingGearAvailability`, default `FOR_LOAN`), and #2492/#2687 made availability **item-wide** (`SetGearAvailability`), with the create-time choice plumbed through `SaveGearRequest.availability`. So per-community share-time availability is legacy. See Open Question 1.
2. **Request last-community guard** is enforced both server-side (`UnshareRequest`) and client-side (`request_sharing_view_model.dart:130`). `UnshareItem` must keep the server guard; the client guard should keep working against the new path. **Doc contradiction to reconcile:** `docs/client/sharing.md:104` states "there is no minimum community requirement — owners can remove all communities after the fact," which is true for gear/experience but **false for requests** (the server blocks the last one). Whichever way Open Question 5 lands, the doc must be corrected so it stops asserting a uniform "remove all" that the request path doesn't honor.
3. **Stale comment** at `gen_request_view_model.dart:396`.
4. **Possibly-dead** `experience_view_model.dart:860` `shareWithCommunity` — verify no live caller before touching.
5. **Dart deprecation warnings.** `ShareExperience` is *already* `deprecated = true` and the client *still* calls `shareExperience` on `main` (green), so protobuf-dart service-method deprecation is evidently **not** analyzer-fatal here. Still, migrate fully so we don't accumulate deprecated calls; verify `flutter analyze` after deprecating the rest.
6. **Proto-comment wording (`proto_conventions.md` §9).** §9 keeps "project conventions" and server mechanics out of proto comments, but a *deprecation* marker is a legitimate client-facing contract signal (it tells clients which RPC to use instead), and CLAUDE.md's TODO policy **requires** the `TODO(#NNNN)` issue reference in the comment — proto is explicitly in scope for that rule. The already-merged `ShareExperience` marker (`experience_service.proto:942`) is the precedent to match: say *what supersedes it* (`ShareItem` / `UnshareItem`) and carry the `TODO(#2526)`, but don't describe server-side removal mechanics or fan-out. This is the one place proto comments name an issue by design.

## Potential Solutions

The core decision is **how to shape and implement `UnshareItem`** (deprecating the redundant paths is mechanical once it lands). Three approaches:

### Option A: Mirror `ShareItem` exactly — registry hook + single `community_id`, behavior-preserving

- **Approach:** Add `UnshareItemRequest { oneof item {experience_id, gear_id, request_id, transfer_id}; string community_id }` and empty `UnshareItemResponse`. Extend `ItemSharer` with `UnshareFromCommunity func(ctx, itemID, communityID, actorUserID) error`. Refactor each existing per-type `Unshare*` handler to delegate to a shared **removal core** (`unshareXFromCommunity`, owner-check hoisted to the caller); register that core as the `UnshareFromCommunity` hook. `UnshareItem` handler: `RequireAuth` → parse target → `VerifyOwner` → `UnshareFromCommunity`. Each type's quirks (hard-delete vs archive, last-community guard, `GEAR_UNSHARED` event, active-community check) stay **inside** its core, unchanged.
- **Pros:** Symmetric with the proven `ShareItem` design; zero behavior change (safe); the three per-type RPCs become one-line delegators to the same cores, which is exactly how `ShareToCommunity` was factored. `transfer` naturally returns `Unimplemented` (no hook), matching `ShareItem`.
- **Cons:** Preserves three divergent semantics behind one RPC (a caller of `UnshareItem` gets soft-archive for requests, hard-delete for gear/experience). Doesn't *fix* the inconsistency — just unifies the entry point.
- **Effort:** Medium.

### Option B: Unify the semantics while converging (normalize delete/archive + guards)

- **Approach:** Option A, plus reconcile the three behaviors — e.g. make all three soft-delete, add the "last community" guard to all types (or drop it), emit a uniform `*_UNSHARED` event for each.
- **Pros:** Leaves a genuinely uniform audience model behind.
- **Cons:** Expands scope well beyond the issue; changes observed behavior (event emission drives the feed/digests; soft-vs-hard-delete affects re-share and history); needs its own testing + product sign-off. High regression risk folded into a convergence PR.
- **Effort:** Large.

### Option C: Client-only convergence, no `UnshareItem` (defer per the original issue)

- **Approach:** Do only what the issue's body says — deprecate/migrate the `Share*` add-paths; leave `Unshare*` alone.
- **Pros:** Smallest change; matches the issue as written.
- **Cons:** **Directly contradicts the requester's PR-scope note** ("do the UnShare as part of the same PR"). Leaves the unshare surface fragmented and a second PR outstanding.
- **Effort:** Small.

## Recommendation

**Option A.** It satisfies the requester's expanded scope (`UnshareItem` in this PR), mirrors the already-merged `ShareItem` so the design is low-risk and reviewer-familiar, and stays behavior-preserving by keeping each type's quirks inside its removal core. Explicitly **do not** unify the divergent unshare semantics in this PR (Option B) — that is a separable behavioral change that would put feed/digest and re-share regressions into what should be a mechanical convergence. If uniform semantics are wanted, file a follow-up.

Deletion of the six deprecated RPCs stays **out of this PR**, tracked by #2526 (see Open Question 2 for keeping that tracker accurate).

## Open Questions

- **1. Gear share-time `availability` — drop it from the converged path?** — Recommendation: **yes, drop per-community share-time availability; converge gear community-share onto `ShareItem` (which inherits the item-wide availability, default `FOR_LOAN`).** Availability is item-wide since #2492 and set at creation (#2687, `SaveGearRequest.availability`) or via `SetGearAvailability`; the bulk modal already hardcodes `FOR_LOAN`. The toggle's "share as giveaway" intent, if it must survive, should route through `SetGearAvailability` separately rather than through the share call. *Fallback:* if a real product need for share-time-per-community availability surfaces, add `optional Availability gear_availability` to `ShareItemRequest` (gear-only field, ignored for other kinds) — but don't do this speculatively (per the "no speculative API fields" convention).
- **2. Which issue do the `Unshare*` deprecation `TODO`s point at?** — Recommendation: **point all six `TODO`s at #2526 and broaden #2526** (edit its title/body) to "Converge item sharing onto ShareItem **and UnshareItem**; remove the per-type `Share*`/`Unshare*` add/remove-paths." #2526 is already the removal tracker for the `Share*` set and stays open after this PR; folding `Unshare*` in keeps one umbrella issue and every `TODO(#2526)` accurate. *Fallback:* file a sibling "remove per-type `Unshare*`" issue and point the three unshare `TODO`s there — only if we want the two removals tracked separately.
- **3. `UnshareItemRequest` field shape: single `community_id` vs `repeated community_ids`?** — Recommendation: **single `community_id`.** Every existing per-type unshare removes one community, the toggle/detail-remove UIs act one community at a time, and the bulk path loops per id today. A `repeated` variant can be added later without a break. (Note the asymmetry with `ShareItem.share_to_community_ids` being repeated — acceptable; add is naturally batchy, remove is a deliberate single act.)
- **4. Transfer support in `UnshareItem`?** — Recommendation: **leave it `Unimplemented`** (register no transfer unshare hook), matching `ShareItem` today. No per-type `UnshareTransfer` exists to converge.
- **5. Keep per-type divergent unshare semantics (soft-archive request / hard-delete gear+experience / last-community guard / `GEAR_UNSHARED` event)?** — Recommendation: **preserve exactly** in this PR (Option A). If unification is desired, file a follow-up issue rather than expanding this one.
- **6. Remove the now-dead hand-written client wrappers (`shareGear`/`shareExperience`/`shareRequest`/`unshare*` service+repo methods)?** — Recommendation: **remove the ones whose only callers were migrated** (keeps the client honest and prevents regrow onto deprecated paths); leave the generated `_client.*` stubs (they exist until the proto RPCs are deleted). Any wrapper still referenced by a test-only or genuinely-live caller stays until its caller migrates. Verify `experience_view_model.dart:860` and the `whos_in_manage_sheet.dart:247` direct-repo bypass are re-pointed, not orphaned.

## Implementation Plan

One PR on the existing `share_rpc` branch. Phases are ordered so the server contract lands first, then clients migrate onto it, then docs/tests/e2e catch up. **Removal of the deprecated RPCs is explicitly not in this PR.**

### Phase 1: Server — add `UnshareItem`, deprecate the six per-type RPCs
**Goal:** A working unified `UnshareItem` mirroring `ShareItem`, plus every superseded per-type RPC marked `deprecated = true` with `TODO(#2526)`, with zero behavior change to the per-type paths.
**Tasks:**
- [ ] Proto (`community_service.proto`): add `UnshareItemRequest { oneof item {experience_id=1, gear_id=2, request_id=3, transfer_id=4}; string community_id=5 }`, empty `UnshareItemResponse`, and `rpc UnshareItem(...)`. Mirror the `ShareItem` doc-comment style; note owner-only + per-type semantics preserved.
- [ ] Proto deprecations with `option deprecated = true` + a `TODO(#2526):` block (state what the RPC does, that `ShareItem`/`UnshareItem` supersede it, and that it's kept until clients migrate):
  - [ ] `ShareGear` (`community_service.proto:1131`), `ShareRequest` (`request_service.proto:822`). (`ShareExperience` already done.)
  - [ ] `UnshareExperience`, `UnshareGear`, `UnshareRequest`.
- [ ] Extend `ItemSharer` with `UnshareFromCommunity func(ctx, itemID, communityID, actorUserID) error` (`share_item.go`).
- [ ] Refactor each per-type `Unshare*` handler → delegate to a shared **removal core** (`unshareExperienceFromCommunity` / `unshareGearFromCommunity` / `unshareRequestFromCommunity`), owner-check hoisted to the caller; register the core as the sharer's `UnshareFromCommunity` in `wiring.go`. Keep every quirk (hard-delete/archive, last-community guard, `GEAR_UNSHARED` event, active-community check) inside the core.
- [ ] Implement `UnshareItem` in `server/services/community/` (new file, e.g. `unshare_item.go`, mirroring `share_item.go` per `server/architecture.md` §3 file-naming): `RequireAuth` → `parseUnshareItemTarget` → `VerifyOwner` → `UnshareFromCommunity`; `Unimplemented` for unregistered kinds. Follow `server/conventions.md` fail-fast + error handling: `connecterr.Internal` for storage failures, `connecterr.UserVisible` for owner/last-community rejections (reuse the existing per-type error keys so client copy is unchanged), and a structured `slog` line with `operation`/`user_id`/`item_kind`/`item_id`/`community_id`.
- [ ] Register `UnshareFromCommunity` for experience/gear/request in `wiring.go`.
- [ ] Tests (`server/services/community/unshare_item_test.go` + integration): per type — owner-only rejection; gear idempotent-when-not-shared + `GEAR_UNSHARED` emitted; request last-community `FailedPrecondition` + soft-archive; experience hard-delete + soft-deleted-community rejection; unregistered kind → `Unimplemented`. Keep the existing per-type unshare tests green — the delegation refactor must not change behavior: `server/services/experience/share_test.go`, `server/services/community/gear_sharing_unshare_test.go`, `server/services/request/sharing_test.go`, and the integration suites `experience_sharing_test.go` / `request_sharing_test.go`.
**Validates:** `UnshareItem` is a faithful union of the three per-type removers; the per-type RPCs still behave identically; the deprecation markers compile. Run `npm run generate` then `go test ./...` (unit + `-tags=integration`) locally.

### Phase 2: Client — migrate all callers onto `ShareItem` / `UnshareItem`
**Goal:** No hand-written client code calls a deprecated per-type share/unshare; add/remove for all three item types flows through `shareItem` / a new `unshareItem`.
**Tasks:**
- [ ] Add `CommunityService.unshareItem({experienceId?, gearId?, requestId?, communityId})` (`community_service.dart`, mirror `shareItem`) and `CommunityRepository.unshareItem(...)` with the same cache-invalidation as `shareItem`.
- [ ] Migrate the **toggle notifiers** (`community_list_item.dart` drivers): `experience_sharing_view_model.dart`, `request_sharing_view_model.dart`, `gear_sharing_audience_actions.dart` — share → `shareItem(shareToCommunityIds: [id])`, unshare → `unshareItem(communityId: id)`. Preserve the request last-community client guard.
- [ ] Migrate **creation flows**: `gen_gear_view_model.dart:513`, `gen_request_view_model.dart:423` (and fix the stale `:396` comment).
- [ ] Migrate **detail-screen confirm-remove** handlers: `experience_close_handlers_mixin.dart:122`, `gear_transfer_handlers_mixin.dart:234`, `request_content_view.dart:892`, and the `whos_in_manage_sheet.dart:247` direct-repo bypass.
- [ ] Migrate **bulk** paths: `shareMultipleWithCommunity` / `unshareMultipleFromCommunity` on the gear/request/experience repos → `ShareItem`/`UnshareItem` (`bulk_community_selection_modal.dart`).
- [ ] Resolve gear availability per Open Question 1 (drop share-time availability; rely on item-wide availability).
- [ ] Remove now-dead hand-written wrappers (Open Question 6); verify `experience_view_model.dart:860` isn't left orphaned.
- [ ] Update Dart unit tests + Mockito mocks for the rewired repos/services/viewmodels.
**Validates:** the whole sharing UI (toggle, creation, detail-remove, bulk) works through the unified RPCs. Run `npm run generate`, `flutter analyze` (confirm no new fatal deprecation warnings), `flutter test --reporter expanded`.

### Phase 3: Docs, e2e, and issue housekeeping
**Goal:** Durable docs and e2e describe the unified add **and** remove paths; the #2526 tracker reflects the expanded scope.
**Tasks:**
- [ ] Update `docs/workflows/item_sharing.md` and `docs/client/sharing.md`: community add/remove goes through `ShareItem` / `UnshareItem`; per-type `Share*`/`Unshare*` are deprecated pending removal (#2526). Advance freshness stamps.
- [ ] Update e2e specs (`gear-authoring-share.spec.ts`, `request-authoring-share.spec.ts`, and any unshare/`community-picker` specs) that drive or assert per-type unshare, so they exercise the unified path. Run the Playwright workflow suite.
- [ ] **Broaden issue #2526** (Open Question 2): edit title/body to include `UnshareItem` convergence + `Unshare*` removal; confirm all six `TODO(#2526)` markers match. Keep #2526 **open** as the deletion tracker.
- [ ] `npm run lint` (incl. `lint:context-map`, `lint:proto`, `lint:go:size`, `lint:dart:size`).
**Validates:** the convergence is documented and e2e-covered; #2526 is an accurate deletion tracker for the deferred removal.

### Deferred (follow-up, NOT this PR) — tracked by #2526
- Delete the six deprecated RPCs and their request/response message types once no client calls them. This is a **buf-breaking** change (removing wire surface), unlike this PR's additive `option deprecated = true` — so it must be gated on a minimum app version, because older installed builds still call the per-type RPCs even after the store release migrates. Confirm zero live callers via client telemetry + a repo grep before removal. This is issue #2526's remaining step-3 after this PR lands.

## Related Issues
- **#2492** — phone-first pivot; CREATE-1 introduced `ShareItem`, the reference design this converges onto (`docs/issues/2492-phone-first-pivot.md`).
- **#2630** — tiered `ShareItem` permissions (any member may reshare the open link; audience mutation is owner-only); `UnshareItem` is always audience-mutating → owner-only.
- **#2687** — item-wide gear availability plumbed through creation; underpins Open Question 1 (dropping share-time availability).
- **#2529** — referenced in a stale `gen_request_view_model.dart:396` comment; verify relationship during the request-creation migration.

---

## Audit notes (ripls-audit)

Audited against the documentation context map. **Engaged dimensions:** `api_design` (new RPC + deprecations), `error_handling`, `observability`, `testing`, `code_quality`, `security` (owner-only auth), `tech_debt` (deprecation lifecycle). Both sides (server + client).

**Docs consulted:** `docs/proto_conventions.md` (anchor — separation, message shape, `optional`, naming parity, proto-comment rules §9, round-trip), `docs/reviews/api_design_review_prompt.md` (Areas to Review — naming/field-numbering/deprecation/backwards-compat), `docs/workflows/item_sharing.md`, `docs/client/sharing.md` (surfaced the last-community doc contradiction), `docs/server/architecture.md` (§3 file-naming, cited for `unshare_item.go`) + `docs/server/conventions.md` (fail-fast, `connecterr`, structured logging) as server anchors, `docs/client/architecture.md` + `docs/client/conventions.md` as client anchors, `CLAUDE.md` (TODO-issue policy, file-size gates).

**Fixes applied after the first draft:**
- Added the `client/sharing.md:104` ↔ `UnshareRequest` last-community **doc contradiction** as an explicit reconcile item (watch-out 2 + Phase 3 doc update).
- Added proto-comment-wording guidance (watch-out 6): deprecation `TODO(#2526)` is the sanctioned exception to §9's "no project conventions/issues in proto comments," matching the merged `ShareExperience` precedent; keep the prose client-facing, no server mechanics.
- Strengthened the `UnshareItem` server task with explicit `connecterr`/`slog` error-handling + observability expectations and reuse of existing error keys.
- Named the specific per-type + integration tests that must stay green through the delegation refactor.
- Clarified the deferred deletion is **buf-breaking** and must be app-min-version gated.

**No blocking compliance issues found.** The design mirrors the already-merged, convention-conformant `ShareItem` (registry pattern keeps `community` from importing item services; dedicated request/response types; `optional` presence; owner-only audience mutation). Open Question 1 (gear `availability`) is the one item that could otherwise drift from the "no speculative API fields" rule — the recommendation (don't add `gear_availability` speculatively) keeps it compliant.
