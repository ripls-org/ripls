# Issue #1711: ItemShareSheet "Failed to load invite link" for multi-community items when initial / picker selection is a non-member community

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1711
**Filed:** 2026-05-04 | **Last activity:** 2026-05-04
**Labels:** none | **Assignees:** unassigned
**Context docs:** none
**Audit pass:** 2026-05-04 — see [Audit Notes](#audit-notes) for what changed and why.
**Status:** picker-filter half **already shipped in PR #1694** (`CommunityHelper.invitableSharedCommunities`, merged earlier 2026-05-04, before this plan was written). Trace captured this morning was on a build that pre-dated #1694. The remaining work is the typed-error upgrade in ItemShareSheet — see [Scope after rediscovery](#scope-after-rediscovery).

## Problem Statement

`ItemShareSheet` is the QR + invite-link bottom sheet shown when sharing a gear, experience, or request. The sheet calls `getOrCreateInviteLink(communityId: _selectedCommunityId, …)` on initState, where `_selectedCommunityId` is seeded from `widget.communityId` (which the caller sets to the item's `sharedCommunities[0]`). For multi-community items shared into communities the viewing user is *not* a member of, that initial id can be a non-member community → server correctly rejects with `permission_denied` → user sees a generic "Failed to load invite link. Please try again." with no recovery path. The picker also enumerates the full `sharedCommunities`, so even if the initial pick were correct the user can manually pick a non-member community and trip the same failure — and in the process the picker leaks the names of communities the user isn't in.

This is a sibling instance of the multi-community scoping pattern tracked under umbrella #1705. Phase 1 (#1707) and Phase 4-gear-page-load (#1708) shipped fixes for the gear-borrow path; Phase 4-members (#1710) shipped the AccessSheet redaction. This issue is the share-link path.

The bug is on the client side: the server's strict membership check is correct. The fix is to filter `sharedCommunities` against the user's actual community membership before passing to ItemShareSheet, so the sheet only ever operates on communities the user is allowed to invite into.

## Context from Comments

No comments yet — issue was filed by me earlier today (2026-05-04 20:57 UTC) from a Phase 4-members repro session. The repro trace is included in the issue body:

```
14:46:48.755 RpcUtils: RPC null failed after 96ms: permission_denied
14:46:48.757 ItemShareSheet: Failed to load invite link
14:46:48.780 ServiceException: Failed to load invite link
```

Issue body already proposes three ranked options and recommends Option A (filter at call sites). This plan formalizes that recommendation.

## Codebase Inventory

**ItemShareSheet** (`app/lib/presentation/widgets/sharing/item_share_sheet.dart`):
- Constructor takes `communityId` (initial pick), `communityName`, and `communities: List<SharedCommunity>` (picker options).
- `_loadInviteLink` (lines 87-127) catches errors and stores raw string `_error` (not typed `UserError`).
- `_buildCommunitySelector` (line 309) passes `widget.communities` straight through to `CommunityPickerButton.options`.
- Picker is hidden when `widget.communities.length <= 1` (line 688).

**Three call sites** (all identical shape — `final first = communities.first;` then pass `first.communityId` + `communities` through):
- `app/lib/presentation/screens/gear/gear_content_view.dart:874-892` — `_showShareInviteLink(List<SharedCommunity>)`
- `app/lib/presentation/screens/experience/experience_content_view.dart:877-898` — same
- `app/lib/presentation/screens/request/request_content_view.dart:703-723` — same

**Existing helpers worth reusing**:
- `app/lib/core/utils/community_resolver.dart` — already has `resolveCommunityId({supplied, shared, userMemberIds})` from Phase 4-gear-page-load. This is the right home for a sibling `filterToMemberCommunities` helper.
- `app/lib/services/providers/community_providers.dart:101-138` — `selectedCommunityProvider.communities` is the user's own community list (already used by AccessSheet in #1710 for the same intersection).
- `app/lib/core/errors/user_error.dart` and `rpc_error_handler.dart` — typed-error pattern already used in viewmodels; not yet adopted by ItemShareSheet.

## Potential Solutions

### Option A: Filter the picker to member-only at the call sites (recommended)

Add a small `filterToMemberCommunities` helper alongside `resolveCommunityId` in `community_resolver.dart`. Each of the three call sites intersects `sharedCommunities` with the user's communities before constructing ItemShareSheet:

- Initial `communityId` = first member-community in the (preserved-order) intersection.
- `communities` arg = only the member-communities subset.
- If intersection is empty (rare; user got to the screen for an item they share zero member-communities with): show a localized snackbar and don't open the sheet.

- **Approach:** New helper, three call-site updates, snackbar fallback for the empty-intersection case.
- **Pros:** No new RPC behavior. User can't accidentally pick a non-member community in the first place. Closes the data-leak half (the picker no longer exposes non-member community names). Pattern matches AccessSheet fix in #1710. Helper is reusable for future Phase 4-other surfaces.
- **Cons:** Three call sites to touch. Edge case (empty intersection) needs a small i18n string for the snackbar.
- **Effort:** Small.

### Option B: Defensive UI inside ItemShareSheet

Catch `permission_denied` in `_loadInviteLink`, fall back to the user's first member community in `widget.communities`, retry once. Upgrade `_error` from raw string to typed `UserError` with `RpcErrorHandler.classify` for the genuine-failure case.

- **Approach:** Single-place change in ItemShareSheet only.
- **Pros:** Single-place fix. Doesn't require touching the three callers.
- **Cons:** UX flicker (initial spinner → permission_denied → retry → success). Doesn't address the data-leak half: the picker still enumerates non-member communities. Encodes recovery logic for a state that should never have been reachable in the first place.
- **Effort:** Small.

### Option C: Server-side relaxation of `getOrCreateInviteLink`

Treat the endpoint as "any community the item is shared with where the user is a member" — same shape as the proposed any-community auth helper in #1705 Phase 3. Server accepts any communityId in `sharedCommunities ∩ userMemberCommunities`.

- **Approach:** Server change in `community_repository.dart` invite-link path; security review; protocol unchanged.
- **Pros:** Pattern-level. Future-proof for any client that hands a non-member community id.
- **Cons:** Larger; security review gate. Doesn't fix the picker exposing non-member community names — that's a client-side leak independent of server behavior. Out of scope for a single-PR fix.
- **Effort:** Medium.

## Recommendation

**Option A.** Combines well with the #1705 Phase 4 family already shipped, closes both halves (the failure AND the picker name-leak), and the helper extends `community_resolver.dart`'s existing membership-intersection theme. Keep the typed-`UserError` upgrade from Option B as a small follow-on within the same PR (orthogonal to scoping; no extra cost given we're already touching the file).

Option C is the right long-term answer for the broader umbrella but should ride along with the Phase 3 audit work, not block this fix.

## Open Questions

- **Q1: Empty-intersection behavior — snackbar-and-don't-open, or open-with-error?** — Recommendation: **snackbar-and-don't-open**. The ItemShareSheet has no useful state to show with zero member communities — the QR can't be generated, the picker has nothing to pick, and the user already saw a Share button so we should give actionable feedback at the entry point instead of opening an empty sheet. The snackbar string can be: "You're not a member of any community this item is shared with."
- **Q2: Should the helper live in `community_resolver.dart` or a new file?** — Recommendation: **same file**. Already houses `resolveCommunityId`, which has the exact same theme (intersect `shared` with `userMemberIds`); a sibling `filterToMemberCommunities` keeps related logic together. Splitting now would be premature.
- **Q3: Do we upgrade ItemShareSheet's raw-string `_error` to typed `UserError` in this PR?** — Recommendation: **yes, but minimal scope**. Convert `_error` from `String?` to `UserError?`, render via `RpcErrorHandler.localize(error, context.l10n)` at build time. This adds ~5 lines. The current raw string is hardcoded English and bypasses the i18n lint allowlist; cleaning it up is a small win adjacent to the scoping fix.
- **Q4: How are the screens getting an item shared into communities the user isn't in?** — Recommendation: **don't try to fix here**. Likely paths are the AccessSheet Add-Community modal (sharing into another community) and gear/experience portfolio listings of items still visible after a community leave. The fix in this PR makes the share path safe regardless of how the user got there; tracking down the upstream paths can happen as part of the Phase 3 audit.

## Scope after rediscovery

Mid-implementation, while reviewing the call sites, I discovered that **PR #1694 (commit `ec24cdc92`, merged 2026-05-04 earlier in the day) already implemented the picker-filter fix** via `CommunityHelper.invitableSharedCommunities`, applied at all three `_showShareInviteLink` call sites in `gear_content_view.dart`, `experience_content_view.dart`, and `request_content_view.dart`. The AccessSheet's "Invite Person" callback is gated on the filtered list (`invitable.isEmpty ? null : ...`), and the snackbar/disabled-action UX is already in place.

After #1694, the bug as described in the issue body is no longer reachable through the standard flow. The trace I captured this morning (gear `c2dfd843-...`) was on a build that pre-dated #1694's merge.

What's left in scope for **this** PR (#1713):

- **Typed-error upgrade in ItemShareSheet.** `_error` was `String?` with a hardcoded English `'Failed to load invite link. Please try again.'`, which bypasses i18n. Convert to `UserError?` classified via `RpcErrorHandler.classify` and rendered via `RpcErrorHandler.localize`. Closes a real i18n violation; orthogonal to the scoping fix #1694 already landed.

What's **not** in scope (already done by #1694 — do not duplicate):

- `CommunityHelper.invitableSharedCommunities` helper.
- Filtering `sharedCommunities` at the three `_showShareInviteLink` call sites.
- Empty-intersection snackbar/disabled-action handling.

The original Implementation Plan below was written before this rediscovery and reflects the larger scope. Phase 1 task list has been trimmed accordingly; the original options A/B/C in [Potential Solutions](#potential-solutions) remain as historical context for why Option A was the recommendation when it was first written.

## Implementation Plan

### Phase 1: Typed-error upgrade in ItemShareSheet
**Goal:** Close the i18n violation in ItemShareSheet's error rendering. Picker-filter half is already shipped in #1694.
**Tasks:**
- [x] In `item_share_sheet.dart`: change `_error` from `String?` to `UserError?`; replace `_error = 'Failed to load invite link. Please try again.'` with `_error = RpcErrorHandler.classify(e)`; update render path to use `RpcErrorHandler.localize(error, context.l10n)`. Remove the hardcoded English string. Preserve the existing `if (!mounted) return;` guards across each await in `_loadInviteLink` — async safety is not optional even when the surrounding state is widget-local.
- [x] Update existing `app/test/presentation/widgets/sharing/item_share_sheet_test.dart` error-state test to assert the new localized text (`'Something went wrong. Please try again.'` from `rpcErrorGeneric`) and that the old hardcoded string is gone.
- [x] Lint + build pass: `flutter analyze`, `npm run lint:dart:i18n`, `npm run lint:dart:a11y`, `npm run lint:ephemeral-comments`, `flutter test`.

**Validates:** ItemShareSheet's error rendering now goes through `RpcErrorHandler.localize`, removing the hardcoded English string. Picker-filter behavior is unchanged (already correct via #1694).

## Related Issues
- #1705 — Umbrella for the multi-community client scoping pattern. This issue is a sibling Phase 4 instance.
- #1707 — Phase 1 of #1705 (gear borrow flow). Same pattern, mutation+refresh axis.
- #1708 — Phase 4-gear-page-load. Introduced `resolveCommunityId({…, userMemberIds})`. This PR adds the sibling `filterToMemberCommunities` helper alongside it.
- #1710 — Phase 4-members (AccessSheet, ManageMembers). Same membership-intersection pattern at a different surface.
- #1665 — Phase 4-chat (open). Adjacent multi-community surface; not affected by this fix but watch for similar picker-leak shape if there's a chat-share path.

## Audit Notes
2026-05-04 — audit pass against `docs/client/{architecture,caching,i18n,accessibility,testing,sharing}.md` and `docs/server/{architecture,observability}.md`. Changes folded into the plan above; rationale below for traceability.

### Q1: Helper placement (community_resolver.dart) — confirmed
The plan puts `filterToMemberCommunities` next to the existing `resolveCommunityId` in `app/lib/core/utils/community_resolver.dart`. Aligns with `docs/client/architecture.md` §1 ("ViewModels are the only layer that calls Repositories" — utility helpers under `core/utils/` are explicitly fine). No "controllers" introduced (architecture.md §5).

### Q2: `selectedCommunityProvider.communities` vs `enabledCommunities`
`docs/client/sharing.md` describes the existing `InviteSheet` (the home-screen invite) as picking a community from "the user's enabled communities." The semantics there: `enabledCommunities` returns the *currently-viewed* set (`isAllCircles ? communities : [selected]`) — i.e., it's narrowed to whatever the sidebar selection is.

For `ItemShareSheet`, that narrowing would be too restrictive: the user is sharing an existing item that has its own `sharedCommunities`, and the right intersection is "communities the item is shared into ∩ communities the user is a member of," **regardless of sidebar selection**. The bug being fixed is `permission_denied` from membership, not from sidebar state. Plan stays on `selectedCommunityProvider.communities` (full member set).

This is a meaningful divergence from the existing `InviteSheet` precedent — flagged here so reviewers see the intentional choice. If product wants `enabledCommunities` for consistency with InviteSheet, the fix is one-line.

### Q3: ItemShareSheet is a `ConsumerStatefulWidget`, not a Notifier
`docs/client/architecture.md` §5 ("No Controllers") and the testing.md guidance prefer business logic in Notifiers. ItemShareSheet currently owns `_isLoading` / `_error` / `_inviteUrl` / `_numMembers` directly in widget state — a pre-existing departure from the architecture that this PR does *not* introduce. The plan upgrades the error storage to typed `UserError?` (improving i18n compliance per `docs/client/i18n.md`) but does not migrate the widget to a Notifier. A migration would belong in a follow-up; flagging here so it doesn't get lost. The widget catching `RpcErrorHandler.classify(e)` directly in `_loadInviteLink` is the documented "widget async exception" shape (architecture.md §"Widget Async Exceptions") — acceptable for a stateful widget that owns its own state.

### Q4: Async-safety guards (architecture.md §"Async Safety in Notifiers")
ItemShareSheet's existing `_loadInviteLink` already has `if (!mounted) return;` after each await. The plan must not regress these. Added an explicit reminder in the implementation tasks. Even though this is a widget (not a Notifier), the same disposal-window risk applies.

### Q5: i18n / a11y compliance
- New string `itemShareSheetNoMemberCommunitiesError` is a regular user-facing string (snackbar text), **not a semantic label** — no `a11y` prefix needed (per `docs/client/accessibility.md` §"Writing good accessibility labels": `a11y` prefix is for semantic action labels, not visible UI text).
- Both `app_en.arb` and `app_es.arb` get the key (per i18n.md §"Each new string needs both locales").
- Hardcoded English `'Failed to load invite link. Please try again.'` is removed in this PR — closes a long-standing i18n violation that the lint allowlist may have been tolerating.
- No new ICU plurals or placeholders, so no metadata block beyond `description`.

### Q6: Testing structure (testing.md)
- Resolver tests live in `app/test/presentation/viewmodels/request_view_model_resolve_community_test.dart`. The filename predates this PR — it's where the existing `community_resolver.dart` tests live despite the misleading "request_view_model" prefix. Adding sibling tests keeps the module's tests co-located. Renaming is a separate cleanup not in scope here.
- Caller-screen and ItemShareSheet widget tests need `localizationsDelegates` / `supportedLocales` on their `MaterialApp` wrapper (per i18n.md §"Test localization"). Use `app/test/helpers/l10n_helpers.dart` if a helper exists; otherwise inline the four standard delegates as `access_sheet_test.dart` does.

### Q7: Caching (caching.md) — N/A
This PR adds no new repository methods, no new cache keys, no manual cache maps. `selectedCommunityProvider.communities` is read directly (already cached upstream by `CommunityRepository.listUserCommunities`).

### Q8: Server boundary — N/A
Pure client-side fix. No server-side changes; no new RPCs; no auth helper changes (Option C from the issue body explicitly deferred to #1705 Phase 3). Server observability conventions (`docs/server/observability.md`) do not apply.
