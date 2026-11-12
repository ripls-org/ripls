# Issue #2273: Revoke doesn't seem to do anything

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2273
**Filed:** 2026-06-01 | **Last activity:** 2026-06-09
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** `docs/invitations.md`, `docs/client/sharing.md` (via globs match for `app/lib/presentation/widgets/sharing/**`), `docs/proto_conventions.md` (anchor), `docs/server/architecture.md` (anchor), `docs/server/conventions.md` (anchor), `docs/client/architecture.md` (anchor), `docs/client/conventions.md` (anchor), `docs/reviews/api_design_review_prompt.md`, `docs/reviews/error_handling_review_prompt.md`, `docs/reviews/security_review_prompt.md`, `docs/reviews/testing_review_prompt.md`

## Problem Statement

The user taps **Revoke** on a QR-code share sheet (community invite or item share) and observes:

> "after hitting revoke the background of the QR code turns red but remains the same."

They explicitly ask "Am I misunderstanding what 'revoke' does?" — i.e. they expect tapping Revoke to invalidate the visible QR code and replace it with a new one. That is the documented behavior in `docs/invitations.md`:

> Members can revoke their invitation links … Revoked links return an error when validated. The member can generate a new link afterward.

Two separate bugs together produce the user-visible "nothing happens" outcome:

1. **Server: `RevokeInviteLink` never revokes item-specific share links.** The handler in `server/services/community/invitations.go:91-138` hardcodes the lookup to `community_invite_id = community_id`, ignoring the `gear_id` / `request_id` / `experience_id` fields on the request even when those are set. The proto comment at `proto/ripls/api/community_service.proto:316-322` says "the server already ignores these fields" but the UI in `ItemShareSheet` (`app/lib/presentation/widgets/sharing/item_share_sheet.dart:185-248`) actively passes them. When a user revokes from `ItemShareSheet` and has no community-invite share link, the server returns `404 "no active invitation link found"` and the client shows the **red error toast** "Failed to revoke link. Please try again." — matching the user's "QR background turns red" description while the QR remains visible (the sheet does not close on failure).
2. **Client: `revokeInviteLink` invalidates the wrong cache namespace.** `app/lib/data/repositories/community_repository.dart:671-692` calls `_buildInviteLinkCacheKey` (namespace `invite:...`) after revoke, but every live sheet now reads via `getOrCreateShareLink` which uses the parallel `share:...` namespace (`_buildShareLinkCacheKey`, `community_repository.dart:746-766`). Result: even when the server *does* succeed (community-level revoke from `InviteSheet`), the next `_loadInviteLink()` call returns the cached pre-revoke short code, so the QR is byte-identical to what was just revoked. This produces the user's "remains the same" symptom for the community-invite path.

The combination — error on item-link revoke, stale cache on community-invite revoke — means **revoke appears broken on every surface that shows a QR code**.

## Context from Comments

The issue has one comment (`@claude`) triggering this run; no clarifying discussion from the reporter or maintainers. The submitter (`user13@example.org`) is on iOS 26.5, app 0.22.0 build 7130, PROD. The three attached screenshots match the toast-error + sheet-stays-visible flow described above (we cannot inspect them directly, but the verbal description maps cleanly to the `_buildErrorState` / error-toast color and the `widget.onClose()`-skipped path in `ItemShareSheet._revokeInviteLink`).

On the plan PR (#2428), the maintainer (`maintainer`) followed up with:

> "@claude retarget option B, removing the current invite link revocation RPC all in the same PR, since it's mostly broken now anyway."

That comment is the source of this revision. The previous draft recommended Option A (extend `RevokeInviteLink` to honor item targets, keep its wire shape). The maintainer's directive is to instead ship the typed-shortcode revoke RPC the proto comments and `share_links.go` already point at, and to drop the legacy RPC at the same time rather than carrying a deprecation tail — because the legacy RPC is already silently broken on the item-link surface, so it's not protecting a working production flow.

## Codebase context (from investigation)

- **Server entry point (to delete):** `server/services/community/invitations.go:90-138` (`RevokeInviteLink`). The storage query at line 113-118 keys on `community_invite_id = community_id` and never inspects `req.Msg.GearId` / `RequestId` / `ExperienceId`. This handler is the only one removed by Option B; `GetOrCreateInviteLink` (the URL-decoration shim, also TODO(#2058)) stays for now — it's a separate cleanup tracked by its own TODO.
- **Server pattern to mirror:** the polymorphic `ShareLink` storage shape (`proto/ripls/models/share_link.proto`) already has per-variant target columns. `lookupShareLink(short_code)` (`server/services/community/share_links.go:21-31`) is the canonical short-code → row resolver and is already used by `AcceptInvitationLink` at `invitations.go:158`. The new `RevokeShareLink(short_code)` slots into this file next to its `GetOrCreate…` sibling.
- **Proto:** `proto/ripls/api/community_service.proto:309-329, 967-968` carries `RevokeInviteLinkRequest` / `RevokeInviteLinkResponse` / `rpc RevokeInviteLink`. All three go away in this PR. The new `RevokeShareLinkRequest { string short_code = 1; }` / `RevokeShareLinkResponse {}` / `rpc RevokeShareLink` land in the same neighborhood, immediately following `GetOrCreateShareLink` at line 956.
- **Existing test:** `server/services/community/invitations_test.go:491-636` covers `RevokeInviteLink` for community-only links. The other indirect call site is `invitations_test.go:145-173` ("cannot accept revoked invitation link") which uses revoke to set up the AcceptInvitationLink negative case. Both rewrite to use the new RPC. No item-link revoke coverage exists today, which is consistent with that path being non-functional.
- **Client service:** `app/lib/services/community_service.dart:348-375` (`revokeInviteLink`). Method removed; replaced with `revokeShareLink({required String shortCode})` calling the generated `_client.revokeShareLink`. The legacy Connect client method (`_client.revokeInviteLink`) disappears with the proto removal — any stale call site is a build-time error rather than a silent runtime regression.
- **Client repository:** `app/lib/data/repositories/community_repository.dart:608-633` (`getOrCreateShareLink`) caches the `GetOrCreateShareLinkResponse` (including `shortCode`) under `share:...`. `community_repository.dart:671-692` (`revokeInviteLink`) calls the bare service then invalidates `invite:...` (wrong namespace). Replaced with `revokeShareLink({required String communityId, String? gearId, String? transferId, String? requestId, String? experienceId})` that (a) reads the cached share-link to recover `shortCode`, (b) calls the new RPC, (c) invalidates the `share:...` key for the same `(communityId, target)`. The legacy `invite:...` namespace and `_buildInviteLinkCacheKey` stay in place for now — they belong to the still-live legacy `getOrCreateInviteLink` getter which the maintainer's directive did not retarget.
- **Client UI A — community invite:** `app/lib/presentation/screens/communities/invite_sheet.dart:159-209` calls `revokeInviteLink(communityId)` then `await _loadInviteLink()`. After the rewrite, this calls `revokeShareLink(communityId: …)` (no item IDs — the sheet reads via `getOrCreateShareLink` with no target on the community-only path). The repository handles `shortCode` lookup against the cache internally, so the call site stays one-line.
- **Client UI B — item share:** `app/lib/presentation/screens/widgets/sharing/item_share_sheet.dart:185-248` calls `revokeInviteLink(communityId, gearId/requestId/experienceId)` then `widget.onClose()`. After the rewrite, this calls `revokeShareLink(communityId: …, gearId: …)` (etc.) — same target dispatch, just a different repository method name. The `widget.onClose()` flow on success is preserved.
- **Tests touched:** `app/test/presentation/widgets/sharing/item_share_sheet_test.dart:326-410` mocks `revokeInviteLink`; rewrites to `revokeShareLink`. New `community_repository_test.dart` and `invite_sheet_test.dart` per the lock-in section below.
- **Doc surface:** `docs/invitations.md:36, 153-161, 297-300` (component list, §6 Revocation, related-files table). `docs/registration_and_login.md:140, 144` references. All update to `RevokeShareLink` and the new flow.

## Potential Solutions

The two bugs are independent in cause but have to be fixed together to make Revoke work end-to-end. The interesting fork is on the **server side**: how should revoke be expressed?

### Option A: Extend `RevokeInviteLink` to honor the typed item targets *(declined)*

Keep the existing RPC. Branch on which target field is set, build the per-variant storage query, revoke the matching row.

- Smallest wire surface change, mirrors `shareLinkTargetSpec`, no deprecation tail.
- But: cements the legacy RPC's awkward shape (`community_id` + four dead-or-load-bearing target fields), and the proto comments (already aspirational about typed-shortcode revoke) stay misleading. The maintainer has explicitly rejected this path in favor of B — see Context from Comments.

### Option B: Add a new `RevokeShareLink(short_code)` RPC and remove the legacy `RevokeInviteLink` in the same PR *(recommended)*

Introduce the typed-shortcode RPC the proto comments describe — `RevokeShareLink(short_code) → RevokeShareLinkResponse{}`. The client passes the `shortCode` it already has cached on every share-link response. Server looks up the row by short code, gates on `inviter_id == caller`, flips `is_revoked`. The legacy `RevokeInviteLink` RPC, request/response messages, server handler, client service method, and tests are deleted in the same PR. The legacy URL-decoration shim `GetOrCreateInviteLink` is **not** affected — its TODO(#2058) cleanup is a separate change.

- **Pros:**
  - Cleaner contract — revoke is a property of a specific link (identified by its short code), not a `(caller, target)` pair. Matches how `AcceptInvitationLink` and `CheckInvitation` already work.
  - Drops the misleading proto comments about "ignored" target fields.
  - No long-term deprecation tail. The legacy RPC is silently broken on the item-link surface today (see Problem Statement); removing it doesn't take away any production-working behavior on that surface.
  - Server handler shrinks substantially: no per-variant dispatch, just one `lookupShareLink` + ownership check + `Update`.
  - Sets up `proto/ripls/api/community_service.proto:316-322`'s TODO for retirement when `GetOrCreateInviteLink` later goes away — the remaining `gear_id` / `request_id` / `experience_id` legacy fields then only live on the URL-decoration request.
- **Cons:**
  - **Breaking change for the in-the-wild 0.22.0 build (the reporter's build).** Users on the old build whose app calls `RevokeInviteLink` will get `Unimplemented` from the server and a generic "Failed to revoke link" toast on the **community-invite** path that today actually succeeds server-side. The item-link path was already broken (404 → red toast), so behavior there is unchanged. Mitigation: force-upgrade gate plus a release note. See Q1.
  - Larger wire change: new RPC, new proto messages, regenerated client/server, more test churn than Option A. The blast radius is bounded because the legacy RPC has only one client service method and two UI call sites.
- **Effort:** Medium.

### Option C: Client-side workaround — issue `getOrCreateShareLink` after revoke to mint a fresh one *(declined)*

Leave the server alone. In the client, after `revokeInviteLink` returns success, force a `getOrCreateShareLink` (bypassing cache). Doesn't fix the server-ignores-item-targets bug — item revoke still 404s.

## Recommendation

**Option B + the client cache-namespace fix + a doc and test backfill.** Ship the new `RevokeShareLink(short_code)` RPC, delete the legacy `RevokeInviteLink` (server handler, proto messages, RPC method, client service method, legacy tests) in the same PR, migrate both UI surfaces to call the new repository method.

Rationale:
- Aligns with the maintainer's directive on PR #2428: switch to the typed-shortcode design and remove the legacy RPC at the same time.
- The legacy `RevokeInviteLink` is silently broken on the item-link surface today, so the removal doesn't subtract a working production behavior on that path. The community-invite path *does* succeed server-side today, so we explicitly call out the breaking-change risk for unupgraded clients and pair it with a force-upgrade gate (Q1).
- The client cache-namespace fix is structural — it's a property of the cache layout, not the RPC, and it has to land regardless of which RPC the client calls.
- The new RPC's authorization model mirrors `RequireAuth` on the call + `link.inviter_id == authInfo.UserID` on the row (Q2). No community-membership re-check is needed because ownership of the row is the binding constraint; this also gracefully handles "user revokes a link they minted after leaving the community."

For the proto:
- Remove `RevokeInviteLinkRequest`, `RevokeInviteLinkResponse`, and `rpc RevokeInviteLink`.
- Add `RevokeShareLinkRequest { string short_code = 1; }` (single field, matches `AcceptInvitationLinkRequest`'s shape), `RevokeShareLinkResponse {}`, and `rpc RevokeShareLink(...)` next to `rpc GetOrCreateShareLink(...)` at the bottom of the service.
- `proto/ripls/api/community_service.proto:316-322`'s TODO(#2058) comment block disappears with the request message it sits on.

For the server:
- Delete `RevokeInviteLink` from `invitations.go`.
- Add `RevokeShareLink` to `share_links.go` (it conceptually belongs to the share-link surface, not the legacy invitations file).
- Handler logic: `RequireAuth` → `lookupShareLink(req.Msg.ShortCode)` → if empty, return `NotFound("no active share link found")` → ownership check (`link.InviterId == authInfo.UserID`, else `PermissionDenied`) → if already revoked, return idempotent success → flip `is_revoked`, `s.storage.Update(...)`, log with `MaskToken(short_code)` and `target_kind`.

For the client:
- `community_service.dart`: remove `revokeInviteLink`; add `revokeShareLink({required String shortCode})` calling `_client.revokeShareLink`.
- `community_repository.dart#revokeShareLink`:
  - Resolve `shortCode` by reading the cached `GetOrCreateShareLinkResponse` for the `(communityId, target)` cache key. If absent, fetch via `getOrCreateShareLink` first (the user can't have tapped Revoke without the QR rendering, so the cache should be warm in practice — but treat the cold case as a one-RPC fetch, not an error).
  - Call the new RPC.
  - Invalidate the `share:...` cache key for the same `(communityId, target)`. The next `getOrCreateShareLink` then mints a fresh row and the QR re-renders with the new code.
- `invite_sheet.dart` and `item_share_sheet.dart`: swap the call to `revokeShareLink(...)`; no flow changes. `InviteSheet`'s post-revoke `_loadInviteLink()` already does the round-trip.

For docs:
- `docs/invitations.md`: rewrite §6 to describe `RevokeShareLink(short_code)`. Replace the component-list and related-files entries.
- `docs/registration_and_login.md:140, 144`: replace `RevokeInviteLink` references with `RevokeShareLink`.
- Advance the `freshness:` stamp on both docs per `docs/context_map.md`.

For tests:
- Server: replace `TestService_RevokeInviteLink` with `TestService_RevokeShareLink` covering: successful community-invite revoke, successful gear/request/experience revoke per variant, ownership-only-revoke (user2 can't revoke user1's link → `PermissionDenied`), unknown-shortcode (`NotFound`), idempotent re-revoke (already-revoked returns success without re-updating). Update `AcceptInvitationLink "cannot accept revoked invitation link"` (lines 145-173) to revoke via the new RPC.
- Client: `community_repository_test.dart` (new) asserting `revokeShareLink` invalidates the right `share:...` key per target. `item_share_sheet_test.dart` — rewrite the mock from `revokeInviteLink` to `revokeShareLink`, keep the "sheet closes on success" assertion. `invite_sheet_test.dart` (new) — after mocked revoke, the second `_loadInviteLink()` fetch returns a different short code and the rendered URL updates.

## Open Questions

- **Q1: How do we handle in-the-wild 0.22.0 clients whose `RevokeInviteLink` call will now `Unimplemented`?** — Recommendation: gate the RPC removal behind a min-supported-build bump. Add the bump to `app/lib/core/config/min_supported_build.dart` (or the equivalent — check the `enforce_minimum_version` interceptor's source of truth) in the same PR so the upgrade prompt fires before the broken-revoke call lands. The release notes for the PR's app build should call out "Revoke now requires app build ≥ N." On unupgraded clients, the user-visible effect on the community-invite surface goes from "revoke succeeds server-side but QR doesn't change" (today) to "revoke fails with an error toast" — strictly more honest, though arguably worse UX in the moment. The force-upgrade gate prevents that gap. If product would rather not force the upgrade, hold the legacy `RevokeInviteLink` server handler as a temporary no-op shim until the bump can ship — but the typed-shortcode plan still applies on the canonical path.
- **Q2: What's the right authorization model for `RevokeShareLink`?** — Recommendation: `RequireAuth` + ownership check (`link.InviterId == authInfo.UserID`); reject anyone else with `PermissionDenied`. Don't require `RequireMemberOfActiveCommunity`: a user who has left the community should still be able to revoke a link they minted before leaving (otherwise their minted QR stays valid forever from their perspective). This is a small but deliberate loosening from today's `RevokeInviteLink` behavior, and is consistent with the "revoke is a property of a specific link" framing the new RPC adopts.
- **Q3: Should already-revoked links return success or error?** — Recommendation: idempotent success. If `link.IsRevoked` is already true on the row the caller's short code resolves to, return `RevokeShareLinkResponse{}` without rewriting. Avoids spurious "Failed to revoke" toasts on retry/network-flap; matches how the client treats revoke as a state-set rather than a state-transition.
- **Q4: Where does the short code come from on the client?** — Recommendation: from the cached `GetOrCreateShareLinkResponse.shortCode`. The repository's `revokeShareLink` looks up the cache entry for the matching `(communityId, target)` and reads `shortCode` from it. If the entry is cold (rare — user opened the sheet and tapped Revoke, so the QR rendered, so the cache must have been written), the repository performs a one-RPC `getOrCreateShareLink` to recover the short code before calling revoke. Sheet code stays free of short-code threading.
- **Q5: Does the new RPC need a `community_id` field for any reason (logging context, multi-region routing, etc.)?** — Recommendation: no. The lookup is by short code (globally unique, indexed); the `community_id` is recoverable from the loaded `ShareLink` row for logging. Adding it back as a "hint" would re-introduce the ambiguity the new RPC is meant to eliminate.
- **Q6: Should `RevokeShareLink` accept a `community_invite` short code AND item-share short codes uniformly?** — Recommendation: yes — same handler, no per-variant branching. The `ShareLink` row's `Target` oneof tells us which variant we're revoking; we just need its `target_kind` for the log line. This is the whole point of the typed-shortcode design.

## Implementation Plan

### Phase 1: Proto — define new RPC, remove legacy

**Goal:** Wire surface reflects the typed-shortcode design.

**Tasks:**
- [ ] In `proto/ripls/api/community_service.proto`:
  - Delete `RevokeInviteLinkRequest` (lines 309-326), `RevokeInviteLinkResponse` (line 329), and `rpc RevokeInviteLink(...)` (line 968).
  - Add `RevokeShareLinkRequest { string short_code = 1; }` and `RevokeShareLinkResponse {}` next to `GetOrCreateShareLinkRequest`/`Response` (after line 396). Follow `docs/proto_conventions.md` for naming and comment style; the new request mirrors `AcceptInvitationLinkRequest`'s single-`short_code`-field shape.
  - Add `rpc RevokeShareLink(RevokeShareLinkRequest) returns (RevokeShareLinkResponse);` immediately after `rpc GetOrCreateShareLink(...)` (line 956), with a doc comment: "Revoke a shareable link. Caller must be the link's inviter. Idempotent — re-revoking an already-revoked link returns success."
- [ ] Run `npm run generate` and ensure the Go and Dart generated files reflect the change. (Generated code is not checked in; verify locally before tests.)
- [ ] Run `npm run lint:proto` and `buf breaking` if configured — this is an explicitly breaking change; the breaking-change check should be allow-listed for this PR or the gate temporarily bypassed with a tracking note in the PR body.

**Validates:** New RPC is callable from a regenerated client; the old RPC no longer exists in the generated stubs (so any stale Go or Dart call site is a build-time error rather than a silent runtime regression).

### Phase 2: Server — implement `RevokeShareLink`, delete legacy handler

**Goal:** New RPC works end-to-end; old handler and its tests are gone.

**Tasks:**
- [ ] In `server/services/community/share_links.go`, add `RevokeShareLink` next to `GetOrCreateShareLink`. Pseudocode:
  ```go
  authInfo, err := auth.RequireAuth(ctx)
  if err != nil { return nil, err }
  links, err := s.lookupShareLink(ctx, req.Msg.ShortCode)
  if err != nil { return nil, connecterr.Internal(ctx, "RevokeShareLink", err) }
  if len(links) == 0 {
      return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no active share link found"))
  }
  link := links[0]
  if link.InviterId != authInfo.UserID {
      return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("not the link's inviter"))
  }
  if link.IsRevoked {
      return connect.NewResponse(&api.RevokeShareLinkResponse{}), nil   // idempotent
  }
  link.IsRevoked = true
  if err := s.storage.Update(ctx, link); err != nil {
      return nil, connecterr.Internal(ctx, "RevokeShareLink", err)
  }
  logger.InfoContext(ctx, "revoked share link",
      "user_id", authInfo.UserID,
      "community_id", link.CommunityId,
      "target_kind", targetKindForLog(link),
      "short_code", logging.MaskToken(link.ShortCode))
  return connect.NewResponse(&api.RevokeShareLinkResponse{}), nil
  ```
  - Define a small `targetKindForLog(link *models.ShareLink) string` helper that returns `"community_invite"` / `"gear"` / `"transfer"` / `"request"` / `"experience"` by switching on `link.Target`. Place it next to `shareLinkTargetSpec` in the same file.
  - **Authorization invariant:** `link.InviterId == authInfo.UserID` is the only authorization gate. No community-membership re-check (see Q2). The ownership check carries the security property.
  - Error wrapping: `connecterr.Internal(ctx, "RevokeShareLink", err)` — bare `connect.NewError(connect.CodeInternal, err)` is gated by `scripts/check_codeinternal.js` per `docs/server/conventions.md` → "Connect CodeInternal errors".
- [ ] Delete `RevokeInviteLink` (`server/services/community/invitations.go:90-138`) and its `// RevokeInviteLink revokes…` doc comment.
- [ ] In `server/services/community/invitations_test.go`:
  - Delete `TestService_RevokeInviteLink` (lines 491-636).
  - Rewrite the `"cannot accept revoked invitation link"` sub-test at lines 145-173 to revoke via `service.RevokeShareLink(ctx1, …)` instead. The new request takes the `inviteLinkResp.Msg.ShortCode` already in scope.
- [ ] Add `server/services/community/share_links_test.go` (or extend the existing test file if present) with `TestService_RevokeShareLink`:
  - `community_invite` revoke succeeds, row flips `is_revoked=true`.
  - `gear` / `transfer` / `request` / `experience` revoke each succeed against rows minted by `GetOrCreateShareLink` with the matching variant — proves the handler is target-agnostic.
  - Ownership: user1 creates a link, user2 calls `RevokeShareLink` with that short code → `PermissionDenied`. (No `RequireMemberOfActiveCommunity` precondition is asserted, matching Q2.)
  - Unknown short code → `NotFound`.
  - Idempotent re-revoke: revoke the same short code twice, second call returns success without state change.
- [ ] `npm run generate`, `npm run build`, `go test -tags=integration -timeout 20m ./server/services/community/...` until green.

**Validates:** Server-side revoke is correct for every share-link variant; the legacy handler is gone; one ownership-only gate replaces the legacy "member of community" gate.

### Phase 3: Client — service, repository, UI

**Goal:** Both share sheets call the new RPC, the cache invalidates correctly, both surfaces show a fresh QR after revoke.

**Tasks:**
- [ ] In `app/lib/services/community_service.dart`:
  - Delete `revokeInviteLink` (lines 348-375).
  - Add `revokeShareLink({required String shortCode})` mirroring the deleted method's `RpcUtils.executeRpc` pattern, with `operationName: 'RevokeShareLink'`.
- [ ] In `app/lib/data/repositories/community_repository.dart`:
  - Replace `revokeInviteLink` (lines 671-692) with `revokeShareLink({required String communityId, String? gearId, String? transferId, String? requestId, String? experienceId})`. Body:
    - Compute `cacheKey = _buildShareLinkCacheKey(communityId, gearId: …, …)`.
    - Resolve the short code: read the cached `GetOrCreateShareLinkResponse` from the cache by `cacheKey`; if cold, call `getOrCreateShareLink` first to fetch one (it caches as a side effect). Read `.shortCode` from the response.
    - Call `_service.revokeShareLink(shortCode: shortCode)`.
    - `await _cache.invalidate(cacheKey)` on success.
  - Leave `_buildInviteLinkCacheKey` and the `invite:...` namespace alone — they remain in use by the still-live legacy `getOrCreateInviteLink` getter. Update the comment at line 743-746 to drop the now-stale "during the deprecation window" framing for revoke specifically; the namespace split now only exists for the two GetOrCreate variants.
- [ ] In `app/lib/presentation/widgets/sharing/item_share_sheet.dart:226-231`, swap `revokeInviteLink` for `revokeShareLink`. Argument shape is identical (`communityId` + the populated item ID).
- [ ] In `app/lib/presentation/screens/communities/invite_sheet.dart:194-196`, swap `revokeInviteLink(communityId: …)` for `revokeShareLink(communityId: …)`. The post-revoke `_loadInviteLink()` re-fetch (already in place at line 200) does the rest once the cache is cleared.
- [ ] Bump the minimum supported app build (Q1). Audit `app/lib/core/config/` and/or `app/lib/core/network/` for the version-gate interceptor's source of truth; raise to the next build number. Verify by running the app pinned to an older build and confirming the upgrade prompt fires before any revoke call would have hit the network. **If product chooses not to bump the gate**, do not delete the server `RevokeInviteLink` handler — leave a temporary thin shim that returns success unconditionally (or `Unimplemented` with a clear message) until the gate can ship. Surface this fork in the PR description.

**Validates:** Both share sheets call the new RPC; `getOrCreateShareLink` on the next read mints a fresh row; the QR re-renders.

### Phase 4: Client tests — lock in the revoke flow

**Goal:** Prevent regression to "QR remains the same" on either surface.

**Tasks:**
- [ ] In `app/test/data/repositories/community_repository_test.dart` (create if absent — pattern-match other repository tests using `MockCacheService` / `MockCommunityService`), add:
  - `revokeShareLink(communityId: c)` calls `_service.revokeShareLink(shortCode: …)` with the short code from the cached community-invite `GetOrCreateShareLinkResponse`, then invalidates `share:c`.
  - `revokeShareLink(communityId: c, gearId: g)` resolves the short code from `share:c:gear:g` cache, then invalidates that same key.
  - Cold cache: with the cache empty for the target, `revokeShareLink` first calls `getOrCreateShareLink` then `revokeShareLink` with the resulting short code.
- [ ] In `app/test/presentation/widgets/sharing/item_share_sheet_test.dart`:
  - Replace `mockCommunityRepository.revokeInviteLink(...)` setups at lines 340, 383, 391, 407 with `revokeShareLink(...)`.
  - Add: after mocked revoke succeeds, `widget.onClose` is invoked (guard against regression on the close-on-success path).
- [ ] In `app/test/presentation/screens/communities/invite_sheet_test.dart` (create if absent — pattern-match `item_share_sheet_test.dart`), add: after mocked revoke succeeds, `getOrCreateShareLink` is called again (`_loadInviteLink()` re-fetches) and the second response is rendered in the QR. Verify by stubbing the second call to return a different short code and asserting the URL display updates.
- [ ] `cd app && flutter test --reporter github test/data/repositories test/presentation/widgets/sharing test/presentation/screens/communities` until green.

**Validates:** Explicit regression coverage on both UI surfaces plus the repository contract for cache invalidation.

### Phase 5: Documentation refresh

**Goal:** `docs/invitations.md` and `docs/registration_and_login.md` describe the new RPC; the lint and freshness gates pass.

**Tasks:**
- [ ] In `docs/invitations.md`:
  - Line 36 (component list): change `**RevokeInviteLink** - Invalidates an invitation link` to `**RevokeShareLink** - Invalidates a share link by short code (caller must be the inviter)`.
  - §6 "Link Revocation" (lines 153-161): rewrite to describe the typed-shortcode RPC. Pseudocode becomes `RevokeShareLink(short_code) → Sets is_revoked=true on the row owned by the caller`. Note ownership-only authorization and idempotency.
  - Related Files table at line 297-300: update `server/services/community/invitations.go` row to drop `RevokeInviteLink` from the "Purpose" cell, and update `server/services/community/share_links.go` row to include `RevokeShareLink`.
- [ ] In `docs/registration_and_login.md`:
  - Line 140: `Revoke Link via RevokeInviteLink RPC` → `RevokeShareLink RPC`.
  - Line 144: change the server file pointer from `server/services/community/invitations.go (RevokeInviteLink RPC)` to `server/services/community/share_links.go (RevokeShareLink RPC)`.
- [ ] Advance the `freshness:` stamp on both docs per `docs/context_map.md` → "Prose freshness".
- [ ] `npm run lint:context-map` and `npm run lint` pass.

**Validates:** Doc map and prose freshness gates are clean; the `ripls-doc-freshness` ratchet has nothing to refresh.

## Related Issues

- #2058 — Legacy RPC cleanup tracker. This PR closes the `RevokeInviteLink` portion of the tracker by removing the legacy RPC. Remaining scope: `GetOrCreateInviteLink` (URL-decoration shim) and the deprecated `gear_id` field on `AcceptInvitationLinkRequest`. Update #2058 with a note when this PR merges so the remaining checklist is accurate.
- #2048 — `ShareLink` polymorphic-table migration. The Phase-2 handler is a clean consumer of the polymorphic shape that migration introduced; the migration's "RevokeInviteLink reads/writes ShareLink underneath" task is superseded by this PR's full replacement RPC.
- #2050 / #2237 — Event share / RSVP work that uses the `experience_id` variant of `GetOrCreateShareLink`. The new `RevokeShareLink` handler is target-agnostic and covers their revoke path with no additional work.
