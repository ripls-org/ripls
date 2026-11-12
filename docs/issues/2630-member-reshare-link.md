# Issue #2630: Non-owner community members can't reshare an item: ShareItem owner-gates the share sheet's link fetch (experience/gear/request)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2630
**Filed:** 2026-07-03 | **Last activity:** 2026-07-03
**Labels:** bug | **Assignees:** unassigned

**Context docs:**
- Anchors: `docs/server/architecture.md`, `docs/server/conventions.md`, `docs/client/architecture.md`, `docs/client/conventions.md`, `docs/proto_conventions.md`, `docs/users/priorities.md`
- Leaf docs (glob/topic match): `docs/workflows/item_sharing.md`, `docs/ad_hoc_communities.md`, `docs/invitations.md`, `docs/client/sharing.md`
- Review dimensions engaged: security (authz change), error_handling, testing, api_design (one additive response field + RPC comment update), observability, code_quality

## Problem Statement

Any member of a community an item is shared with is supposed to be able to reshare that item via its `/go/{code}` open link — resharing by attendees/members is the growth loop of the phone-first pivot (#2492), and the tiered product priorities lean directly on it (a circle member "gets a ball rolling" without being the host). Today they can't: the share sheet (`ItemShareSheet`) calls the `ShareItem` RPC on open just to resolve the item's per-item community and mint the link, and `ShareItem` gates the **entire** call on item ownership (`server/services/community/share_item.go:91`) before doing anything. A non-host attendee who RSVP'd to an event — and who therefore *is* a member of the event's per-item origin community via the #2548 implicit join — taps **Invite** on the "Who's in" roster and gets `PermissionDenied` ("You don't have permission to perform this action").

The bug applies to all three shareable item kinds (experience, gear, request): they share the same `ItemShareSheet` → `ShareItem` path, and each has invite affordances reachable by non-owners (`experience_pitching_in_screen.dart:241-249` renders Invite unconditionally; `gear_content_view.dart:379` and `request_content_view.dart:736` gate only on the viewer having a shared community).

The codebase already encodes the intended permission model in three places that `ShareItem` contradicts:

1. **`GetOrCreateShareLink` is membership-gated, not owner-gated** (`server/services/community/share_links.go:214`, `auth.RequireMemberOfActiveCommunity`) — and `docs/invitations.md` states "each community member can generate a shareable link." `ShareItem`'s `mintItemShareLink` wraps the same `getOrCreateShareLink` core, which even mints per-inviter links keyed on `inviter_id`, so member-minted links attribute (and revoke) correctly.
2. **The client's invite contract assumes member access suffices**: `CommunityHelper.invitableSharedCommunities` (`app/lib/core/utils/community_helper.dart:28-45`) documents "only members can invite, and the server already guarantees that (#1666)."
3. **A shared server gate for exactly this check already exists**: `auth.RequireAccessToCommunityScopedEntity` (`server/auth/community_scoped_access.go:74`) — "active member of at least one community the entity is shared with, or owner" — supports Gear/Experience/Request and is what `GetExperience` (`server/services/experience/get.go:51`) already uses.

One contradiction cuts the other way: `docs/client/sharing.md` and `docs/workflows/item_sharing.md` describe the SharedWithCard Invite button as **owner-only**. The maintainer's clarified intent (in the issue) resolves this: the **link** is member-shareable; audience *management* (removing people/communities, changing RSVPs) stays host-only. The docs need updating as part of the fix.

## Context from Comments

No comments on the issue yet. The issue body is itself the full diagnosis (filed by the maintainer after an interactive debugging session), and it embeds the product decision: *"Anyone in a community an item is shared with can reshare it to more folks via the shortcode link. Only the host retains audience management."* Treat that as maintainer direction, not an open debate.

The issue also flags a **secondary anomaly** to verify during the fix: `VerifyExperienceOwner` attaches a `LocalizedErrorDetail` (`experience_owner_required_for_share` → "Only the owner can share this event."), and the client maps that code (`rpc_error_handler.dart:359`), yet the reporter saw the bare-`Code.permissionDenied` fallback string. Either the detail is being lost on the wire (a real bug in `_extractLocalizedErrorDetail`'s `detail.type` matching outside unit tests) or the deployed server predates the detail. Worth one end-to-end check while in here.

## Potential Solutions

### Option A: Two-tier authorization inside ShareItem (link fetch = view access; audience mutation = owner)

- **Approach:** Split `ShareItem`'s single `VerifyOwner` gate by what the request actually does:
  - **Link-only calls** (no `invitees`, no `share_to_community_ids` — what the sheet sends on open): require the caller to have *view access* to the item (member of any community it's shared with, or owner), via a new per-kind `ItemSharer` hook implemented with `auth.RequireAccessToCommunityScopedEntity`.
  - **Audience-mutating calls** (any `invitees` or `share_to_community_ids`): keep the existing `VerifyOwner` gate unchanged.
  - Fix provisioning attribution: the provision-if-missing fallback (`share_item.go:130`) must create the per-item community with the **item owner** as host, never a non-owner caller. The new hook returns the item's owner ID so `ProvisionPerItemCommunity` (`server/community/provision.go:214`) gets the right `hostUserID`.
  - Client: hide the host-only rows ("Invite people", "Add Community") in `ItemShareSheet` for non-owners, gated by a new server-derived `can_manage_audience` field on `ShareItemResponse` (see Phase 2 for why the client-side `isOwner` getters are not enough).
- **Pros:** Matches the clarified product intent exactly. Reuses the existing, tested access-gate helper. API change is one additive `optional` response field. Keeps the SMS/email invite fan-out and cross-community sharing owner-only (no new abuse surface). Consistent with `GetOrCreateShareLink`'s existing permission model.
- **Cons:** The `ItemSharer` seam grows a second hook wired in `main.go` for three kinds; the sheet needs an ownership signal threaded in (small).
- **Effort:** Medium (server seam + gate split + attribution fix + tests; small client change; doc refresh).

### Option B: Client falls back to GetOrCreateShareLink for non-owners

- **Approach:** Leave `ShareItem` owner-only. Teach `ItemShareSheet` to detect non-ownership and instead call the already-membership-gated `GetOrCreateShareLink` with the item target plus a community the viewer belongs to (picked from the item's `shared_communities`).
- **Pros:** Zero server authz change; smallest server risk.
- **Cons:** The client can't reliably pick the item's per-item origin community (it would have to guess by empty name), and for gear the per-item community may not exist yet — it's provisioned lazily *inside* `ShareItem`, so the non-owner path would mint a link scoped to a named community instead of the item's own audience, changing what joining the link means. Splits one flow into two divergent RPC paths and leaves `ShareItem`'s contract silently owner-only, contradicting the client's documented invite contract (#1666). Fixes the symptom, not the root cause — contrary to the project's fix-root-cause norm.
- **Effort:** Medium (client complexity), but leaves debt.

### Option C: Membership-gate all of ShareItem

- **Approach:** Replace `VerifyOwner` with the view-access check for the whole RPC — members can also invite individuals and add communities.
- **Pros:** Simplest server diff; one gate.
- **Cons:** Materially widens the abuse surface: any member could trigger off-app email invites and cross-community fan-out for an item they don't own (the SMS/email invite path is explicitly a rate-capped abuse surface — see `maxInviteesPerShare` in `share_item.go`). Contradicts the documented owner-only audience-management model and the host-centric roster. Product hasn't asked for it.
- **Effort:** Small code, large policy/product risk.

## Recommendation

**Option A.** It implements the maintainer's stated intent precisely (member = link resharing; owner = audience management), reuses `auth.RequireAccessToCommunityScopedEntity` rather than inventing a parallel check, keeps the API change to one additive response field, and closes the provisioning-attribution hole that Options B/C leave open or don't touch. The mechanical shape:

- Extend the `ItemSharer` seam (`share_item.go:42`) with one hook: `VerifyViewer func(ctx, itemID, actorUserID string) (ownerID string, err error)` — verifies view access and returns the item's owner ID (needed for provisioning attribution). Implemented as `VerifyExperienceViewer` (experience service), `VerifyRequestViewer` (request service), and `VerifyGearViewer` (community service, next to `VerifyGearOwner`), each a thin wrapper over `auth.RequireAccessToCommunityScopedEntity` with the right `EntityKind`.
- In `ShareItem`: if the request mutates audience (`invitees` or `share_to_community_ids` non-empty) → `VerifyOwner` as today; otherwise → `VerifyViewer`, and pass its returned `ownerID` (not `authInfo.UserID`) to `ProvisionPerItemCommunity` and as the actor for the idempotent origin-community share, so a non-owner triggering the lazy gear/legacy provision never becomes host of the item's origin community.
- Link minting stays per-caller (`getOrCreateShareLink(..., inviterID=caller, ...)`): each resharer gets their own revocable link, consistent with `RevokeShareLink`'s inviter-only authorization.
- The response tells the client what the caller may do: `ShareItemResponse.can_manage_audience` (optional bool, field 3) is set from the check that just ran (`callerID == ownerID`). The server is the authority; the sheet doesn't re-derive permissions from possibly-uninitialized local state.

## Open Questions

- **Should "Invite people" (adding member invitees) also open to non-owner members?** — Recommendation: **no, not in this fix.** Keep audience mutation owner-only, matching the documented model and containing the off-app fan-out surface. If member-driven inviting is wanted later, it's a deliberate product decision (possibly alongside COMM-2 #2592's approval gate), not a bug fix.
- **Should link minting require the caller to be a member of the per-item ad-hoc community specifically, or is view access via any shared community enough?** — Recommendation: **view access is enough.** The RSVP'd attendee is a member of the origin community anyway (#2548); requiring origin membership would still exclude gear/request viewers who see the item only via a named community, which contradicts "anyone in a community it's shared with can reshare." Note this is deliberately looser than `GetOrCreateShareLink`'s community-membership gate — the minted link is scoped to the item's origin community, which the caller may not belong to. The joined audience is the item's audience either way, which is the product intent; flag it explicitly in the PR description for security review.
- **Transfer kind?** — Recommendation: leave as-is. No `ItemSharer` is registered for `ItemKindTransfer` (returns `Unimplemented`, covered by `TestShareItem_UnsupportedItemKind`); don't expand scope.
- **Why did the reporter see the generic permission string instead of the mapped "Only the owner can share this event."?** — Recommendation: verify `LocalizedErrorDetail` propagation end-to-end once during Phase 3 (server on this branch → real client). If the detail's wire `type` doesn't match `'ripls.api.LocalizedErrorDetail'` in `_extractLocalizedErrorDetail` (`rpc_error_handler.dart:302`), file/fix as a separate small bug; if it was deployment version skew, no action.
- **Capacity interaction:** ad-hoc communities currently enforce the 32-member cap on open-link join (`docs/ad_hoc_communities.md`). Member resharing makes hitting the cap more likely. — Recommendation: no change here; the cap exemption for unnamed communities is already tracked as deferred work under #2492 COMM-1.

## Implementation Plan

### Phase 1: Server — split ShareItem authorization

**Goal:** A non-owner with view access can call `ShareItem` (link-only) and get the per-item community + share URL; audience mutation stays owner-only; lazy provisioning attributes to the item owner.

**Tasks:**
- [x] Add `VerifyViewer func(ctx context.Context, itemID, actorUserID string) (string, error)` to `ItemSharer` (`server/services/community/share_item.go:42`), documented as "view access → item owner ID".
- [x] Implement `VerifyExperienceViewer` in `server/services/experience/share.go` (wraps `fetchExperienceForRead` + `auth.RequireAccessToCommunityScopedEntity(EntityExperience, …)`, returns `OwnerId`).
- [x] Implement `VerifyRequestViewer` in `server/services/request/sharing.go` (same shape, `EntityRequest`, returns `RequesterId`).
- [x] Implement `VerifyGearViewer` in `server/services/community/gear_sharing.go` (same shape, `EntityGear`, returns `OwnerId`).
- [x] Wire all three in `server/main.go:1215-1225`.
- [x] Add `optional bool can_manage_audience = 3` to `ShareItemResponse` (`proto/ripls/api/community_service.proto:652`) — "whether the caller may manage this item's audience (invite people, add communities); false for members who can only reshare the open link." Update the `ShareItemRequest`/RPC comments, which currently describe owner-only semantics, to the split model (link fetch = any member of a community the item is shared with; audience mutation = owner). Per `docs/proto_conventions.md`, describe meaning only — no server mechanics. No round-trip test needed (server-derived, response-only field). Run `npm run generate`.
- [x] In `ShareItem`: branch on `len(req.Msg.GetInvitees()) > 0 || len(req.Msg.GetShareToCommunityIds()) > 0` — mutating → `VerifyOwner` (unchanged); link-only → `VerifyViewer`, thread returned `ownerID` into `ProvisionPerItemCommunity` and into `shareItemViaSharer` as the actor for the origin-community share. Set `can_manage_audience` on the response from the caller-is-owner outcome.
- [x] Error semantics: `VerifyViewer` denials stay bare `PermissionDenied` (consistent with `RequireAccessToCommunityScopedEntity` / `GetExperience` — a caller with no view access shouldn't reach the sheet, so no new user-visible ARB string). Keep the existing localized owner-required codes (`*_owner_required_for_share`) on the mutating path.
- [x] Observability: add the caller's role to `ShareItem`'s structured log line (e.g. `"caller_is_owner", bool`) so member-minted links are distinguishable in the audit trail (`docs/server/observability.md` standard-fields conventions).
- [x] Unit tests in `server/services/community/share_item_test.go` (extend `fakeItemSharer` with a `verifyViewer` hook): (1) non-owner member, link-only → success, share URL returned, link `inviter_id` = caller; (2) non-owner with `invitees` → PermissionDenied, viewer check not sufficient; (3) non-owner with `share_to_community_ids` → PermissionDenied; (4) non-owner link-only on an item with **no** per-item community yet → community created with item owner as community owner (assert `Community.owner`/membership), not the caller; (5) owner flows unchanged (existing tests keep passing).
- [x] Per-service viewer-hook tests (experience/request/gear): owner passes, shared-community member passes, non-member fails, returns correct owner ID.
- [x] `npm run build` (lint + compile) and `go test ./...` locally.

**Validates:** The server-side permission model matches intent, with the provisioning-attribution hole closed, before any UI changes.

### Phase 2: Client — non-owner share sheet

**Goal:** A non-owner opening the share sheet sees QR + link + Copy/Share and can reshare; host-only rows are hidden; the roster Invite button works for attendees.

**Tasks:**
- [x] Surface `can_manage_audience` through the client data layer (`CommunityService`/`CommunityRepository.shareItem` result object) and into `ItemShareSheet` (`app/lib/presentation/widgets/sharing/item_share_sheet.dart`); hide `_buildAudienceActions` (both "Invite people" and "Add Community") when it's false. Do **not** gate on the per-type provider `isOwner` getters: the sheet also opens from post-create flows (`home_screen.dart:105`, `blank_create_dispatcher.dart:42`) before the item's detail provider is initialized, and a false-negative there would hide the audience rows from the item's own creator. The server response is authoritative and always present exactly when the sheet needs it.
- [x] Confirm the "Anyone with link can join" copy and share/copy analytics events are correct for the non-owner path (no change expected).
- [x] Widget tests: `can_manage_audience: true` shows both audience rows; `false` shows neither; non-owner sheet renders QR/link from a mocked repository. (Run the FULL `flutter test --reporter expanded` suite, not just the edited files — share-sheet copy changes have broken sibling widget tests before.)
- [x] Manually drive the attendee flow (RSVP → Who's in → Invite) against the local server to verify the end-to-end fix (per `verify` norm: exercise the real flow, not just tests).

**Validates:** The reported repro is fixed in the real UI for events, and gear/request non-owner invite paths stop erroring.

### Phase 3: E2E proof, docs, and error-detail verification

**Goal:** The invitee-reshare loop is proven end-to-end in the real web UI; documentation matches the new permission model; the error-message anomaly is explained or fixed.

**Tasks:**
- [x] Update `docs/client/sharing.md` (SharedWithCard "owner-only Invite button" → non-owners get the reshare sheet without audience rows) and `docs/workflows/item_sharing.md`; touch `docs/invitations.md` if its `ShareItem` framing needs the member-reshare nuance. Advance `freshness:` stamps per `docs/context_map.md`.
- [x] **Playwright e2e (required — maintainer direction):** extend the existing RSVP full-loop spec (`e2e/tests/workflows/phone-rsvp-full-loop.spec.ts`) so that after the guest completes phone-first RSVP and lands in the app, they open the event's Who's-in roster, tap **Invite**, and the share sheet renders a `/go/` share link (assert the URL text and/or QR presence; the audience rows are absent for the non-host guest). Update the mirrored workflow example in `docs/workflows/phone_first_rsvp.md` (steps + postconditions) per the `docs/testing/update_system_tests.md` protocol. Run locally per `e2e/README.md` (secret fetching works locally).
- [x] Verify `LocalizedErrorDetail` propagation — **verified at the wire-parsing layer, no bug found.** The Dart connect lib (connectrpc 1.0.0) yields exactly `ripls.api.LocalizedErrorDetail` as `ErrorDetail.type` on both protocols (Connect-JSON passes the `type` field verbatim, matching connect-go's bare fully-qualified name; the gRPC-web path strips the `type.googleapis.com/` prefix from `typeUrl`), which matches `_extractLocalizedErrorDetail`'s comparison string. The reporter's generic message is therefore deployment version skew (server/app at the time predating the detail or its mapping), not a client extraction bug. No separate issue filed.
- [ ] Close the loop on #2630 with a summary of the shipped permission model.

**Validates:** A real invitee (phone-first RSVP'er) can pull up the share sheet and see the link, guarded by CI; docs stop contradicting behavior; the secondary anomaly is resolved rather than left mysterious.

## Related Issues

- #2492 — phone-first pivot epic; per-item ad-hoc communities and open-link membership are its COMM-1/LINK-1 mechanics, and member resharing is the growth loop this fix unblocks.
- #2548 — events-only implicit join (RSVP'er promoted into origin community); why the reporter is a member of the community they can't get a link for.
- #2526 — `ShareItem` superseding per-type share add-paths; where the blanket owner gate came from.
- #1666 — server guarantees `shared_communities` is caller-membership-scoped; the client invite contract this fix restores consistency with.
- #2592 — COMM-2 approval gate for open-link communities (parked); member-minted links must compose with it when it lands (link → approval queue instead of direct join).

## Audit Notes (`/ripls-audit`, 2026-07-03)

Changes applied to this plan by the audit:

1. **Client ownership signal replaced (correctness/api_design).** The original Phase 2 gated the audience rows on the per-type provider `isOwner` getters. That breaks in the post-create entry points (`home_screen.dart:105`, `blank_create_dispatcher.dart:42`), where the sheet auto-opens before the item's detail provider is initialized — the creator would see the rows hidden. Replaced with a server-derived `ShareItemResponse.can_manage_audience` field set from the authorization check that just ran. This changes Option A from "no proto change" to "one additive optional response field" (wire-compatible; response-only, so no round-trip test per `docs/proto_conventions.md`).
2. **Proto comment task added (api_design).** The `ShareItemRequest`/RPC comments describe owner-only audience semantics; the behavior split must be reflected in the API contract's comments (meaning only, no server mechanics).
3. **Error-semantics task made explicit (error_handling).** `VerifyViewer` denials stay bare `PermissionDenied`, matching `RequireAccessToCommunityScopedEntity` / `GetExperience`; localized `*_owner_required_for_share` codes remain on the mutating path. No new ARB strings → no i18n parity work.
4. **Observability task added.** `ShareItem`'s structured log line gains a caller-role field so member-minted links are auditable (security dimension: audit logging of permission-relevant actions).
5. **Playwright e2e made required (maintainer direction, this session).** Extends `phone-rsvp-full-loop.spec.ts` — invitee completes RSVP, opens Who's-in → Invite, asserts the `/go/` link renders — and updates the mirrored `docs/workflows/phone_first_rsvp.md` example per `docs/testing/update_system_tests.md`.

Docs consulted: `docs/llms.txt`; anchors `docs/server/architecture.md`, `docs/server/conventions.md`, `docs/client/architecture.md`, `docs/client/conventions.md`, `docs/proto_conventions.md`, `docs/users/priorities.md`; leaf docs `docs/workflows/item_sharing.md`, `docs/ad_hoc_communities.md`, `docs/invitations.md`, `docs/client/sharing.md`; review rubrics ("Areas to Review") for security, error_handling, testing, api_design, observability, code_quality.
