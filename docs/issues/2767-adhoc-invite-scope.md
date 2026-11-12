# Issue #2767: Share link summary includes spurious community name

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2767
**Filed:** 2026-07-25 | **Last activity:** 2026-07-25
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** `docs/invitations.md`, `docs/ad_hoc_communities.md`, `docs/proto_conventions.md`, ⚓ `docs/server/architecture.md`, ⚓ `docs/server/conventions.md`, ⚓ `docs/client/architecture.md`, ⚓ `docs/client/conventions.md`, ⚓ `docs/server/observability.md`, `docs/workflows/item_sharing.md`, `docs/client/sharing.md`, `docs/client/deep_linking.md`, `server/services/community/share_links.go`, `server/services/community/share_item.go`, `server/notifications/deeplink.go`, `server/community/provision.go`, `docs/reviews/api_design_review_prompt.md` (§4 Backwards Compatibility, §7 API Security), `docs/reviews/error_handling_review_prompt.md` (§1 Go Error Handling, §4 Logging & Observability), `docs/reviews/security_review_prompt.md` (§1 AuthN/AuthZ, §4 API Security), `docs/reviews/testing_review_prompt.md` (§2 Server Testing), prior plan `docs/issues/1711-itemsharesheet-multi-community.md`, closed PR #2768 (first-attempt fix)
**Audit pass:** 2026-07-25 — see [Audit Notes](#audit-notes).

## Problem Statement

An item's `/go/{code}` invite link renders a preview whose description ends
"in {community name}" (e.g. "Dana is lending this in halls of Austin.").
The community name in that preview is whichever `community_id` the link's
underlying `ShareLink` row is scoped to, resolved server-side by the SSR
landing page. When that scoping community is a *named* community that both
the inviter and the joiner happen to share membership in, the preview leaks
that community's name into an out-of-community share — surprising to the
inviter (who did not mean to advertise "halls of Austin" to their friend)
and opaque to the joiner (who has no context for why that community appears).

Bryan's second comment reframes the fix: the mint-time community selection
is the real defect. Because `GetOrCreateShareLink` (and the notification-side
`ShortLinkCodeForEntity`) take an arbitrary `community_id` alongside the
item target, any code path that happens to have a named community in
context — the calendar-export mint on the experience viewmodel, the
off-app SMS/email notification builder, a member re-sharing from a
community-context view — mints a link scoped to that named community and
bakes it into the preview forever. Every item already has an **ad-hoc
per-item community** (see `docs/ad_hoc_communities.md`); the `ItemShareSheet`
share-out on the item detail page already routes through `ShareItem` and
therefore scopes to that ad-hoc community — but the other mint paths do
not, and they are the ones producing the spurious preview.

**Contradiction with the first fix.** The closed PR #2768 tried to fix
this in the SSR templates by stripping the community name from the
`og:description` and `og:title` meta tags across `gear.html` /
`request.html` / `event.html`. Bryan closed it in favor of this
mint-time fix so the on-page kicker and the app's own display can still
show the community name where it makes sense — the leak is in the choice
of community, not in the preview template.

## Context from Comments

- **Original description (Bryan, 2026-07-25 19:54).** Screenshot of a
  share preview ending "in halls of Austin"; app 0.27.0 build 9096
  Android; environment PROD. Bryan hypothesizes it appears because he
  received the invite via his membership in that named community.
- **First direction (Bryan, 2026-07-25 20:41).** "I think we should
  always exclude the community name in the share link preview." Prompted
  the first-pass template fix in PR #2768.
- **Revised direction (Bryan, 2026-07-25 21:45).** "Actually, I realize
  what's really going on here… the community baked into an item invite
  link should always be the item's ad-hoc community, and not another
  named community the invitee happens to be a member of — that's too
  surprising and opaque to the inviter. We should change the way invite
  links are minted / made available to joiners so the invite link
  gotten from the item page is always to join the ad-hoc community."
  This supersedes the first direction and closes PR #2768 as the wrong
  fix layer.

## Codebase inventory

- **`server/services/community/share_links.go`** — `GetOrCreateShareLink`
  RPC (line 205); `ShortLinkCodeForEntity` server-internal seam used by
  the off-app notification channel (line 338); `parseShareLinkTarget`
  variant dispatch (line 63); the `verify` closures currently gate the
  target-community pair on "item is shared with this community" (lines
  95-113, 123-141, 174-192).
- **`server/services/community/share_item.go`** — `ShareItem` RPC and
  `mintItemShareLink` (line 421) already scope to the item's ad-hoc
  community via `communitylib.ProvisionPerItemCommunity` (line 167).
- **`server/community/provision.go`** — `FindAdHocByOrigin` (line 192)
  is the canonical lookup for an item → its ad-hoc community id.
  `Origin` (line 27) is the tagged union of item ids we already thread
  through the create side.
- **`server/notifications/deeplink.go`** — `notificationLink`
  (line 34) resolves an outgoing notification to a `/go/{code}` URL via
  the injected `ShareLinkResolver` closure — currently
  `ShortLinkCodeForEntity(communityID, experienceID, gearID, requestID)`,
  which passes the notification's `community_id` straight through.
- **Client — `app/lib/presentation/widgets/sharing/item_share_sheet.dart`**
  — the on-item-page share sheet; already correct (uses `shareItem()`
  which mints against the ad-hoc community via `ShareItem`).
- **Client — `app/lib/presentation/viewmodels/experience_view_model.dart`
  `exportToCalendar` (line 800-830)** — mints an item-scoped `/go` link
  for the calendar-entry URL via `getOrCreateShareLink(communityId:
  state.communityId, experienceId: experience.id)`. `state.communityId`
  is resolved via `resolveCommunityId` from
  `sharedCommunities ∩ userMemberIds` — so on an experience shared with
  both a named community and its ad-hoc one, it can (and typically
  will) resolve to the named community. **This is one of the two
  primary bug producers.**
- **Client — `app/lib/presentation/screens/communities/invite_sheet.dart`**
  — a community-level invite sheet with an optional `gearId` param
  passed through to `getOrCreateShareLink`. All in-tree callers pass no
  `gearId`, but the parameter still exists; a stale caller could scope
  a gear link to a named community here.
- **Proto — `proto/ripls/api/common.proto:135-140`** —
  `SharedCommunity.is_origin_community` is already denormalized onto
  the item-detail responses. The client already has the ad-hoc
  community id in hand any time it can see the item.

## Potential Solutions

### Option A: Server-side rewrite (recommended)

Make the server the single source of truth for "an item-target `/go`
link is scoped to that item's ad-hoc community." Two touchpoints:

1. **`GetOrCreateShareLink`** — before the target-scoped `verify`
   closure runs, resolve the ad-hoc community id for the requested item
   via `communitylib.FindAdHocByOrigin`. If the row exists (it will for
   every item post-#2492; provisioning on-miss is the fallback
   `ShareItem` already runs), swap it in as the scoping community for
   the ShareLink row. Preserve the caller-membership auth on the
   *supplied* `community_id` — the caller must still be a member of a
   community the item is shared with, so members can reshare while
   randoms can't; only the row's scope changes. Response's
   `community_id` reflects the ad-hoc.
2. **`ShortLinkCodeForEntity`** — mirror the same rewrite: when an item
   id is set, resolve the ad-hoc community via
   `FindAdHocByOrigin(ctx, s.storage, origin)` and use that for the
   `getOrCreateShareLink` call. Fall back to the notification's
   `communityID` only for the entity-less "community_invite" branch.

- **Approach:** two ~10-line rewrites inside `share_links.go`; touches
  the `verify` closures so they check ad-hoc membership post-swap;
  updates the docs in `docs/invitations.md` to describe "item-target
  links are always ad-hoc scoped."
- **Pros:** Backward compatible — every existing client build starts
  producing correct previews the moment the server rolls out; no
  min-version gate; no client changes strictly required for the fix
  to land; consolidates the invariant in one place so future callers
  can't drift.
- **Cons:** Response's `community_id` differs from the caller's
  requested `community_id` — this is a semantic change worth calling
  out in the RPC doc and in `invitations.md`. Client callers that read
  `response.community_id` for their own routing (there are none today
  — `invite_sheet.dart` uses the *pre-call* selected id) would need to
  handle the swap.
- **Effort:** small–medium (server rewrite + tests + doc updates;
  optional client tidy-ups can follow).

### Option B: Deprecate `community_id` from the item-target request

Drop `community_id` from item-target variants entirely (or ignore it),
requiring the server to look up the ad-hoc community and use it
unconditionally.

- **Approach:** mark `community_id` optional-for-item-targets in the
  proto comments; server ignores when set; client stops sending. Follow
  the same server rewrite as Option A but without the pretense of
  "membership on the supplied community."
- **Pros:** Cleanest final shape — the item is the address, ad-hoc is
  the scope, and there's no vestigial community id to fight over.
- **Cons:** Loses the caller-membership auth signal that "you must be
  in a community the item is shared with" — we'd have to reproduce it
  as "you must be able to view the item," which is what `ShareItem`'s
  `VerifyViewer` already does; wiring that here means importing an
  item-service seam into `share_links.go` or adding another
  `ItemSharer` hook. Larger scope than Option A. Also the more
  disruptive proto change of the two.
- **Effort:** medium.

### Option C: Client-only fix

Update every client caller of `getOrCreateShareLink` with an item id to
pass the item's ad-hoc community id (available via
`SharedCommunity.is_origin_community`). Add a server-side assertion
that item-target links are only ever minted against ad-hoc communities,
so future violations fail fast.

- **Approach:** touch `experience_view_model.exportToCalendar`,
  `invite_sheet.dart` (drop the `gearId` param or route via
  `ItemShareSheet`), and audit for any other item-with-community-id
  mints. Add server assertion (`FailedPrecondition` when scoping
  community isn't ad-hoc for the target item) for regression
  protection.
- **Pros:** Explicit at the call site — a reader sees "ad-hoc for
  item links" in the client code.
- **Cons:** Requires a min-app-version gate to enforce (old builds
  still pass named communities and would break on the new assertion);
  every future caller has to remember the rule; misses the
  notification-side `ShortLinkCodeForEntity` code path unless that's
  also addressed. Higher regression risk than Option A.
- **Effort:** medium (many call sites + version gating).

## Recommendation

**Option A.** Server-side rewrite of both `GetOrCreateShareLink` and
`ShortLinkCodeForEntity` to scope item-target ShareLinks to the item's
ad-hoc community, resolved via `communitylib.FindAdHocByOrigin`.
Preserve the caller-membership auth on the caller-supplied
`community_id` so the "must be in a community the item is shared with"
signal is unchanged; only the scoping of the minted row changes.

Rationale:

- The invariant Bryan wants is *global* — every mint path, not just
  the item-page share sheet. Enforcing at the server closes it in one
  place for every past, present, and future caller (including the
  notification-side path that neither the closed PR #2768 nor a
  client-only fix would have caught).
- No min-app-version gate — the fix rolls out with the next server
  deploy and immediately corrects the notification and calendar-export
  previews on every installed build.
- The change respects the existing auth invariant: the item must still
  be visible-to-the-caller through *some* community they're a member
  of; that's what the current `verify` closures already check, just
  now against the ad-hoc row.
- One follow-up (Phase 2 below) then removes the vestigial
  `communityId`/`state.communityId` threading on
  `exportToCalendar` — nice cleanup, not required for correctness.

## Open Questions

- **What happens for legacy items with no `origin_item` row?**
  Every item created after #2492 has one; earlier items may not.
  Recommendation: fall back to the caller-supplied `community_id`
  (the pre-fix behavior) and log a WARN with the item id so we can
  spot how many stragglers exist. `ShareItem` already has a
  `ProvisionPerItemCommunity` fallback path — we could opt to
  provision-on-miss here too, but it's owner-attribution-sensitive
  (see `share_item.go:167` for why `ShareItem` uses `itemOwnerID`),
  so a plain fallback + WARN is safer for this scope. A true DB
  failure inside `FindAdHocByOrigin` (as opposed to a benign miss)
  wraps to `connecterr.Internal(ctx, spec.opLog, err)` per
  `docs/server/conventions.md` §Error Handling — never a silent
  fallback.
- **`AcceptInvitationLink` semantics — does the joiner join a
  different community now?** Yes, and that is the point. Before the
  fix, a `/go/{code}` link scoped to a named community adds the
  joiner to that named community on accept. After the fix, an
  item-target link is always scoped to the ad-hoc, so the joiner
  joins the item's ad-hoc audience — exactly what Bryan's second
  comment asks for ("the invite link gotten from the item page is
  always to join the ad-hoc community"). This is a behavioral shift
  worth calling out in the PR description so anyone tracking join
  counts on named communities knows why the growth pattern moves.
- **Does the response's `community_id` swap break any existing
  client?** Grep says no in-tree — `ItemShareSheet` uses
  `shareItem()` (which returns `adhocCommunityId` explicitly, no
  ambiguity), and `invite_sheet.dart` only uses the selected id
  pre-call to key its cache. Recommendation: land the swap; document
  the semantic in the RPC doc comment and in `docs/invitations.md`.
- **`ShareLink` cache reuse.** The existing "one active link per
  (inviter, target)" record already keys on target, not community, so
  a caller who has previously minted an item-target link against a
  named community will retrieve *that* row (still scoped to the named
  community) on a re-mint. Recommendation: on cache hit, if
  `link.community_id != adhocID`, update the row in-place to the
  ad-hoc scope before returning. Backfill-in-flight; the row's short
  code doesn't change, so already-shared URLs "self-heal."
- **`resolveCommunityId` semantics.** No change needed there — it
  continues to pick an item-viewing community for RPCs that care
  (RSVP, stats, etc.); only the share-link mint stops using it.
- **Client cache keys on the repository layer.** The
  `share:{communityId}:{kind}:{itemId}` cache key in
  `community_repository.dart:_buildShareLinkCacheKey` uses the
  caller-supplied `communityId`. Post-fix, two calls with
  different supplied ids for the same item map to different cache
  entries pointing at the same underlying ShareLink row — mildly
  wasteful, not incorrect. No client cache change required for
  correctness; a follow-up could re-key on the item alone once
  Phase 2 lands.
- **Should PR #2768's SSR-template cleanup be revisited?** No — the
  on-page kicker is a viewer-side surface (the person opening the
  link has context), and the OG description auto-corrects once the
  scoping community is nameless (`communityDisplayName` substitutes
  "a Ripls group"). Leave the templates alone.

## Implementation Plan

### Phase 1: Server rewrite (unblocks the fix)

**Goal:** Every `/go/{code}` link minted with an item target scopes
to that item's ad-hoc origin community, regardless of caller
context; the on-wire preview reads "a Ripls group" (nameless) rather
than a spurious named community.

**Tasks:**
- [ ] In `server/services/community/share_links.go`, extract a small
      helper `resolveScopingCommunity(ctx, spec, callerSuppliedCommunityID)`
      that returns the ad-hoc community id (via
      `communitylib.FindAdHocByOrigin` on the target item's origin)
      for item targets, and the caller-supplied id for the
      `community_invite` variant. On lookup miss for an item target,
      log a WARN and return the caller-supplied id so legacy items
      still work.
- [ ] Rewrite `GetOrCreateShareLink` (line 205): keep the
      `RequireMemberOfActiveCommunity` auth check against the
      caller-supplied `community_id` (that's the "you must be in a
      community the item is shared with" invariant), then call the
      helper, and pass the resolved id into `spec.verify` and
      `getOrCreateShareLink`. Response's `community_id` reflects the
      resolved id.
- [ ] In `getOrCreateShareLink` (line 259): on cache hit, if
      `existing[0].CommunityId != resolvedCommunityID`, update the
      row's `community_id` in place before returning (the short code
      stays stable so already-shared URLs self-heal).
- [ ] Rewrite `ShortLinkCodeForEntity` (line 338) to use the same
      helper for item-set branches; the `community_invite` fallback
      keeps the notification's community id.
- [ ] Extend `spec.verify` variants to check the item is shared with
      *some* community the caller belongs to — currently they check
      it's shared with the caller-supplied community, which is a
      subset of that. This is preserved implicitly by the outer
      `RequireMemberOfActiveCommunity` + the existing
      `CommunityGear`/`CommunityExperience`/`CommunityRequest` join
      row lookup; verify no additional check is needed.
- [ ] Update RPC doc comment on `GetOrCreateShareLink` and on
      `ShortLinkCodeForEntity` to describe the ad-hoc scoping and
      the `community_id`-as-auth-only semantic.
- [ ] Update `docs/invitations.md` §"Link Structure" and §"Nameless
      (ad-hoc) communities" to describe the invariant: "every
      item-target `/go/{code}` link is scoped to the item's ad-hoc
      origin community, so the preview never leaks an unrelated
      named community. The mint-time `community_id` argument is
      used only for caller-membership auth; the row's scope is
      always the ad-hoc."
- [ ] Bump `freshness.verified_commit` / `verified_on` on
      `docs/invitations.md` and (if touched) on
      `docs/ad_hoc_communities.md` and
      `docs/workflows/item_sharing.md` per the freshness protocol
      in `docs/context_map.md` §Prose freshness. Add one sentence
      to `item_sharing.md` explicitly noting the invariant (already
      implicit in `ItemShareSheet`'s use of `ShareItem`; now true
      for every mint path).
- [ ] Structured logging on the rewrite: keep the existing
      per-target `logger.With("operation", spec.opLog,
      "user_id", ..., "community_id", req.Msg.CommunityId,
      spec.targetField, spec.targetID)` fields, and add
      `"resolved_community_id"` (the ad-hoc id) and
      `"used_fallback"` (bool, true when `FindAdHocByOrigin`
      missed and we fell back to the caller-supplied community)
      per `docs/server/observability.md` standard field naming.
- [ ] Add integration tests in
      `server/services/community/share_links_test.go` covering:
      caller in named + ad-hoc → link scoped to ad-hoc; existing
      link in named → self-healed to ad-hoc; legacy item without
      an ad-hoc → falls back to caller community with WARN;
      `ShortLinkCodeForEntity` on a notification community → link
      scoped to ad-hoc; a `FindAdHocByOrigin` storage error →
      `connect.CodeInternal`, not a silent fallback; a joiner
      accepting the rewritten link joins the ad-hoc, not the
      caller's community (guards against a regression on the
      behavioral change called out in Open Questions).

**Validates:** the SSR preview for any freshly shared item link
reads with the nameless-community copy (`communityDisplayName`
falls through to "a Ripls group"), and the calendar-export URL /
push-notification `/go/` link go through the same self-healed row.

### Phase 2: Client cleanup (optional, follow-up)

**Goal:** Remove the dead `community_id` threading from item-scoped
mint callers so a reader can tell from the client code that
item-target ShareLinks aren't community-selected.

**Tasks:**
- [ ] In `app/lib/presentation/viewmodels/experience_view_model.dart`
      `exportToCalendar`, either (a) resolve the ad-hoc community id
      from `state.experienceDetails.sharedCommunities` (find the one
      with `isOriginCommunity == true`) and pass that as
      `communityId`, or (b) switch the mint to
      `communityRepository.shareItem(experienceId: exp.id)` and use
      the returned `shareUrl` (matches `ItemShareSheet`'s pattern).
      Prefer (b) — single mint API; the server's link-only
      `VerifyViewer` branch already permits non-owners.
- [ ] In `app/lib/presentation/screens/communities/invite_sheet.dart`,
      drop the `gearId` parameter — verified there are no in-tree
      callers passing a real gear id, and post-Phase-1 the server
      would rewrite the scope anyway. If we want to keep the shape,
      leave it but add a doc comment pointing to `ItemShareSheet`.
- [ ] Sweep `app/lib` for `getOrCreateShareLink(...gearId:` /
      `requestId:` / `experienceId:` and route survivors through
      `shareItem` where appropriate.

**Validates:** every client mint of an item-target link is
either through `shareItem` or explicitly scoped to the ad-hoc.

## Related Issues

- #2492 — phone-first pivot; introduces per-item ad-hoc communities
  (`docs/ad_hoc_communities.md`) that this plan leans on as the
  canonical scope for item invite links.
- #2526 — convergence onto `ShareItem`/`UnshareItem` (in progress).
  Phase 2 of this plan is one more step in that same convergence.
- #1711 — prior ItemShareSheet multi-community fix. Shipped the
  member-filtered picker; this plan closes the "even if the picker
  is right, the mint is wrong" gap.
- #1705 — umbrella for multi-community scoping bugs; this plan is
  the invite-link half.
- PR #2768 — first-pass template fix (closed). This plan
  supersedes it: the leak is the mint-time community, not the OG
  template.

## Audit Notes

**Docs consulted** (via `docs/llms.txt` map):
⚓ `docs/server/architecture.md`, ⚓ `docs/server/conventions.md`,
⚓ `docs/server/observability.md`, ⚓ `docs/proto_conventions.md`,
⚓ `docs/client/architecture.md`, ⚓ `docs/client/conventions.md`,
`docs/invitations.md`, `docs/ad_hoc_communities.md`,
`docs/workflows/item_sharing.md`, `docs/client/sharing.md`,
`docs/client/deep_linking.md`,
`docs/reviews/api_design_review_prompt.md`,
`docs/reviews/error_handling_review_prompt.md`,
`docs/reviews/security_review_prompt.md`,
`docs/reviews/testing_review_prompt.md`.

**Engaged review dimensions:** `api_design` (RPC semantic change,
no wire change), `error_handling` (fallback discipline for
`FindAdHocByOrigin` miss vs. error), `observability`
(structured-log fields on the rewrite; the `used_fallback` field
is the signal we'd watch to know legacy stragglers are gone),
`security` (auth invariant preservation: caller must still be a
member of the supplied community; row lookup is (inviter, target)
so no cross-caller side effect), `testing` (integration coverage
across the four rewrite paths + a joiner-side assertion).

**Fixes applied during audit:**
- Broadened Context docs to include the observability, review
  prompts, and sharing / workflow docs the plan actually leans on.
- Added an Open Question on `AcceptInvitationLink` semantics —
  the joiner joins the ad-hoc, not the named community, once the
  rewrite lands. Bryan's stated intent, but a real behavioral
  shift worth calling out in the PR.
- Added an Open Question on repository cache keys — the client's
  `share:{communityId}:...` key uses the supplied id, so old
  clients get slightly wasteful cache splitting but correct
  results. No client change required for correctness.
- Added Phase 1 tasks for freshness stamp bumps on
  `docs/invitations.md` (and any other touched doc) per
  `docs/context_map.md` §Prose freshness, and for the
  structured-log fields (`resolved_community_id`, `used_fallback`)
  aligned with `docs/server/observability.md`.
- Added a test case for a joiner accepting the rewritten link,
  guarding against a regression on the behavioral change.
- Added an explicit "true DB failure → `connecterr.Internal`"
  note on `FindAdHocByOrigin` per `docs/server/conventions.md`
  §Error Handling (no silent warn-and-continue on non-benign
  errors).

**No blocking findings.** The plan is compatible with the
architecture (service independence via the `communitylib` shared
library — no cross-service call), the proto conventions (no wire
change; only response semantic is broadened; documented), and the
security posture (auth invariant preserved; row lookup can't
touch another caller's rows).
