# Issue #1650: Community chat follow-ups from #1646: All Circles surfacing, media support, attribution info

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1650
**Filed:** 2026-05-01 | **Last activity:** 2026-05-01
**Labels:** none | **Assignees:** unassigned
**Context docs:** none (predecessor plan: [docs/issues/1637-community-wide-chat.md](1637-community-wide-chat.md))

## Problem Statement

PR #1646 shipped the community-wide chat MVP: a Discuss tab on the community content view, an `All Circles`-aware pinned card at the top of the inbox in single-community context, and the server foundations from the #1637 plan. Three rough edges were intentionally deferred and tracked here:

1. **All Circles surfacing.** The pinned community-chat card in [portfolio_inbox_screen.dart:206-209](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart#L206-L209) is conditional on `selectedCommunityProvider.selectedCommunity != null`. In All Circles mode (`selectedCommunityId == null`) the pin is hidden entirely — so a user with multiple communities has no way to see "is anyone talking in any of my circles" without switching the sidebar selection one community at a time.
2. **Media support in `CommunityChatPane`.** [community_chat_pane.dart:48-51](../../app/lib/presentation/screens/communities/widgets/community_chat_pane.dart#L48-L51) hard-codes `mediaItems: const []`, `onAddMedia: () async {}`, `onDeleteMedia: (_) async {}`, `isOwner: false`. The shared `InlineConversationView` is fully wired for media (see [experience_chat_pane.dart:76-103](../../app/lib/presentation/screens/experience/widgets/experience_chat_pane.dart#L76-L103) for the standard pattern); the community pane just isn't passing anything through.
3. **Attribution info button.** The community content view *does* load and pass background attribution to the overflow menu — [community_content_view.dart:121](../../app/lib/presentation/screens/communities/community_content_view.dart#L121) calls `loadBackgroundAttribution`, and [line 547](../../app/lib/presentation/screens/communities/community_content_view.dart#L547) passes `state.backgroundAttribution` into `ContentOverflowMenuConfig`. But the *trigger* for that menu is `onInfoTap: _showOverflowMenu` on `ContentOwnerRow`, gated at [line 296](../../app/lib/presentation/screens/communities/community_content_view.dart#L296) by `widget.actor != null`. When the user lands on the community via the inbox-pin path through [community_screen.dart](../../app/lib/presentation/screens/communities/community_screen.dart) (which does **not** pass `actor`), the entire owner row — and with it the info button — is suppressed. Attribution is loaded but unreachable.

These are the three TODOs in the description, in order. None requires a proto change; all three are Flutter-side.

## Context from Comments

The issue has no comments yet — it was filed today as a deliberate carve-out from #1646. The PR description for #1646 is the source of truth for the three TODO bullets and is reproduced verbatim in the issue body. No reviewer pushback redirected the work.

## Current State Snapshot

Useful baselines from the codebase research, since each TODO depends on what already exists:

- **`selectedCommunityProvider` already exposes the full community list** at [community_providers.dart:105](../../app/lib/services/providers/community_providers.dart#L105) (`communities`), [line 120](../../app/lib/services/providers/community_providers.dart#L120) (`effectiveCommunityIds`), and [line 137](../../app/lib/services/providers/community_providers.dart#L137) (`enabledCommunities`). Iterating "all communities the user belongs to" is `state.communities`; the All Circles flag is `state.isAllCircles` ([line 113](../../app/lib/services/providers/community_providers.dart#L113)).
- **The inbox already uses a sliver-based scroll view** at [portfolio_inbox_screen.dart:200](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart#L200). Adding more pinned cards is just emitting more `SliverToBoxAdapter` entries before the existing item list.
- **`communityConversationProvider(communityId)`** is the per-community data source the existing single pin already uses ([line 225](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart#L225)). It returns the same `ConversationItem` shape (with `unreadCount`, `lastMessageAtUnixSec`, `lastMessageText`, `lastMessageSender`) for every community. Reusing it in a list per community is straightforward — the question is fan-out cost.
- **The shared `InlineConversationView` media contract** is parameter-only ([inline_conversation_view.dart:53-64](../../app/lib/presentation/widgets/content/inline_conversation_view.dart#L53-L64)). `mediaItems` is the parent entity's media (community avatar/gallery), not a separate "chat-only" attachment list. `isOwner` controls full delete/reorder rights; non-owners can still upload and delete their *own* uploads. This matches gear/experience semantics: members upload to the community's gallery, the community creator can curate.
- **Community media flows already exist** — [community_content_view.dart](../../app/lib/presentation/screens/communities/community_content_view.dart) implements `_uploadMedia` at [line 207](../../app/lib/presentation/screens/communities/community_content_view.dart#L207), `_handleDeleteMedia` at [line 590](../../app/lib/presentation/screens/communities/community_content_view.dart#L590), and `_handleReorderMedia` at [line 601](../../app/lib/presentation/screens/communities/community_content_view.dart#L601). They all delegate to `communityEditProvider(communityId).notifier`, whose state already exposes `allMediaItems`. The plumbing exists; it just stops at `CommunityChatPane`'s prop boundary.
- **Attribution loading is already wired.** `loadBackgroundAttribution(mediaId)` populates `editState.backgroundAttribution`, and `showContentOverflowMenu` already renders attribution when present (per the pattern shared with gear/experience). The data path works — only the trigger is missing in actor-less contexts.
- **Standalone-screen entry points have a precedent for top-right action buttons.** [community_screen.dart:69-82](../../app/lib/presentation/screens/communities/community_screen.dart#L69-L82) currently puts only a `BackButtonWidget` in its top row; gear/experience equivalents add additional buttons there. There's space for an info icon in this layout without restructuring.
- **CI lint gates relevant to this work** (per [CLAUDE.md](../../CLAUDE.md)): `lint:dart:i18n` (every user-visible string from `context.l10n`), `lint:dart:a11y` (every tap target uses `Tappable`/`Toggle`/`IconAction`, allowlist is empty), `lint:nav` (no raw `Navigator.push(MaterialPageRoute(...))`), `lint:dart:size` (Dart files under 1000 lines).

## Potential Solutions

Each TODO is independent; this section presents alternatives per TODO, then a unified recommendation.

### TODO 1 — All Circles surfacing

#### Option 1A: One pin per community, always shown (simplest)

- **Approach:** When in All Circles mode, render N `_buildPinnedCommunityChat(...)` instances at the top of the inbox — one for every entry in `state.communities`. When in single-community mode, keep the existing single-pin behavior unchanged.
- **Pros:** Trivially mirrors the existing pattern. No new viewmodel logic; just a `for` loop instead of an `if`. Easy to reason about ordering (e.g. alphabetical, or by community recency).
- **Cons:** A user with 10+ communities would see a wall of pinned cards at the top of the inbox — very noisy, especially when most are silent. Fan-out: N parallel `communityConversationProvider(id)` watchers means N HTTP fetches (the cache mitigates after first load, but the cold-start cost scales linearly).
- **Effort:** Small.

#### Option 1B: Pin only communities with unread messages, in All Circles mode (recommended)

- **Approach:** In All Circles mode, render a pinned card for each community where `communityConversationProvider(communityId).asData?.value.unreadCount > 0`, ordered by `lastMessageAtUnixSec` descending. Communities without unreads stay hidden. In single-community mode, keep the existing pin (always visible regardless of unreads — the user explicitly scoped to that community).
- **Pros:** Surface only meaningful signal — no clutter from silent communities. Ordering by recency makes the most active circle the most discoverable. The "no unreads → no pins" zero-state is a clean signal-of-absence to the user. Bounded fan-out: still N watchers, but the visible row count only grows with activity, not with community count.
- **Cons:** Slightly more complex viewmodel logic. Requires derivation of the visible list (a `Provider` that watches all `communityConversationProvider(id)`s and filters/sorts). Edge case: when zero communities have unreads, the user may not realize community chat exists — mitigated by the existing Discuss tab on each community's content view.
- **Effort:** Small to medium.

#### Option 1C: Single aggregated "Community chats" pin in All Circles mode

- **Approach:** Render one consolidated card showing total unread count across all communities; tapping it opens a list/sheet of communities with their individual unread counts.
- **Pros:** Constant inbox real-estate cost. Scales to arbitrary community counts.
- **Cons:** Adds a new modal/sheet UX layer the rest of the inbox doesn't have. The first tap doesn't take the user to a chat — it takes them to a picker. Worse for the common case of "user wants to read the one community chat that has new messages."
- **Effort:** Medium.

### TODO 2 — Full media support in `CommunityChatPane`

#### Option 2A: Plumb existing community media through the pane (recommended)

- **Approach:** `CommunityContentView` already owns the community-media flows (`_uploadMedia`, `_handleDeleteMedia`, `_handleReorderMedia`) and the `allMediaItems` state. Pass the same plumbing into `CommunityChatPane` that `_showMediaCarousel` already uses, then forward into `InlineConversationView`. `isOwner` resolves to `widget.canEdit`.
- **Pros:** Mechanical; mirrors the gear/experience/request pane pattern exactly. Reuses storage, upload, deletion, and ordering logic the community view already has. No new state surface.
- **Cons:** Couples chat-pane media to the community's broader media gallery — which is the same coupling gear/experience/request have, and is fine.
- **Effort:** Small.

#### Option 2B: Separate per-conversation message attachments

- **Approach:** Treat chat-pane attachments as message-level (not community-level), with their own staging area distinct from the community gallery.
- **Pros:** Conceptually cleaner separation between "community media" (cover, gallery) and "messages with photos."
- **Cons:** Diverges from the rest of the chat stack. Doubles the upload/delete code surface for chat permanently. Not what the existing `InlineConversationView` is built for. The `mediaItems` parameter on the shared widget specifically expects parent-entity media.
- **Effort:** Large.

### TODO 3 — Attribution info button

#### Option 3A: Always-visible info icon in `CommunityScreen`'s top row + decouple in `CommunityContentView` (recommended)

- **Approach:** Add an `InfoIconButton` (using `IconAction` per a11y rules) to [community_screen.dart](../../app/lib/presentation/screens/communities/community_screen.dart)'s existing top row, alongside the back button — visible whenever the screen is open. Tapping invokes `_showOverflowMenu`. In `community_content_view.dart`, also expose the info-tap *outside* the actor-conditional `ContentOwnerRow` so feed-context entries that don't render `ContentOwnerRow` (or render it without an actor) still surface attribution.
- **Pros:** The info button is always reachable, not conditional on entry path. Matches how the back button itself is placed (always at top, regardless of actor presence). Clear, discoverable affordance.
- **Cons:** Two touchpoints (`CommunityScreen` and `CommunityContentView`) need changes; need to be careful not to render the icon *twice* in the feed-header context where `_showOverflowMenu` is already wired into `buildFeedHeader` ([line 311](../../app/lib/presentation/screens/communities/community_content_view.dart#L311)). The `CommunityScreen` button should suppress when `showFeedHeader` is true (but `CommunityScreen` never sets that, so the suppression is structural — not a runtime check).
- **Effort:** Small.

#### Option 3B: Always-render `ContentOwnerRow` even with null actor

- **Approach:** Remove the `widget.actor != null` clause from the conditional and have `ContentOwnerRow` degrade gracefully when no actor is provided.
- **Pros:** Single touchpoint.
- **Cons:** `ContentOwnerRow` is shared with gear/experience; changing its semantics is a wider blast radius. The component's name ("OwnerRow") implies an owner is present. Forcing it to render with no owner is semantically dishonest.
- **Effort:** Small but with collateral risk.

## Recommendation

- **TODO 1:** **integrated-row model** — community chats with unreads appear as regular inbox rows in a single sort with the rest of the inbox, not as pinned cards above it. This supersedes Options 1A/1B/1C above per the resolved decision "don't treat the communities different than the other items in the inbox. There should be just one sort." This change also retires PR #1646's coral-tinted pinned card. See Phase 1 for details.
- **TODO 2: Option 2A** — plumb the existing community-media flows through `CommunityChatPane`. Mirrors the established pattern; the work is wiring, not invention.
- **TODO 3: Option 3A** — add an explicit info icon to `CommunityScreen`'s top-right (parallel to other content views), and decouple the info trigger inside `CommunityContentView` from the actor-conditional row. The data path is already correct; only the trigger is missing.

All three TODOs land cleanly as separate phases of one PR (or as three small PRs if preferred). Net effort: 2-3 days of one engineer's time including tests, lint conformance, and emulator verification. Phase 1's scope expanded slightly versus the original plan because it now retires the #1646 pinned-card UI in addition to handling All Circles surfacing.

## Resolved Decisions

All open questions answered by the issue owner on 2026-05-01. Decisions are recorded here as the binding spec for the implementation phases below; if any need to be revisited, update this section first and then propagate the change into the affected phase.

- **All Circles surfacing model:** **community chats are integrated into the regular inbox row list, not pinned above it.** They appear as inbox entries with the same row treatment as `DailyItem` rows and participate in the same single sort. **Why:** the user directive is "don't treat the communities different than the other items in the inbox. There should be just one sort." This is a behavior change from PR #1646's single-community pinned card — see "Scope note" in Phase 1 below.
- **Sort key:** **`lastMessageAtUnixSec` descending**, same key the rest of the inbox uses. Most-active surfaces first; one combined sort across community-chat rows and DailyItem rows.
- **Unread gate:** community chats appear in the inbox **only when `unreadCount > 0`**. Communities with no unread messages do not appear (matches the "show nothing when zero unreads" empty-state directive — there's no separate empty state because the rows just don't exist).
- **Cap:** **no hard cap**. The unread-only filter naturally bounds the list. Revisit only if a user reports a usability issue with many active community chats.
- **Media support `isOwner` semantics:** **match gear/experience** — `isOwner = widget.canEdit` (creator-only for delete-any and reorder; members can still upload and delete their own uploads, enforced by `InlineConversationView`).
- **Upload permission for chat-pane media:** **any community member can upload** (option b in the original analysis). Carousel stays creator-gated for the curated gallery; chat attachments and curated gallery are distinct UX affordances. Document the divergence in `community_chat_pane.dart`'s class doc-comment.
- **Info icon placement on `CommunityScreen`:** **top-right**, parallel to the overflow-menu placement in other content views. Back button stays top-left.
- **Info icon visibility:** **always visible**, regardless of whether attribution exists. The overflow menu surfaces Edit/Share/Gallery in addition to attribution, so the icon is useful even when attribution is empty. Conditional hiding would create a discoverability problem.

## Implementation Plan

### Phase 1: Integrate community chats into the inbox flat list

**Goal:** Community chats with unread messages appear as regular inbox rows — not pinned cards — interleaved with `DailyItem` rows in a single `lastMessageAtUnixSec`-descending sort. This applies in both single-community and All Circles modes.

**Scope note (behavior change from #1646):** PR #1646 shipped a dedicated pinned card above the inbox list in single-community mode (with a coral background and a "Community" type label) at [portfolio_inbox_screen.dart:206-209](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart#L206-L209) and [_buildPinnedCommunityChat](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart#L221-L366). The resolved decision **replaces** that pinned card with a regular inbox row that participates in the standard sort. The "Community" type label survives (it's just the row's type, parity with "Transfer"/"Request"/etc.); the dedicated pin slot and coral-tinted background go away. After this phase, `_buildPinnedCommunityChat` and its supporting `_buildCommunityPhoto` helper are deleted.

**Tasks:**

- [ ] **Inbox item type extension.** Audit how `inboxFlatItemsProvider` produces its current entries (likely a list of `DailyItem` or a wrapper type). Either (a) extend the existing item type with a community-chat variant (e.g. a `oneof`-style sealed Dart class), or (b) introduce a presentation-layer `InboxRowItem` union that wraps both `DailyItem` and a new `CommunityChatRow` carrying `(community, conversationItem)`. Pick whichever fits the existing shape with the least churn — the inspection in [portfolio_inbox_view_model.dart:332-356](../../app/lib/presentation/viewmodels/portfolio_inbox_view_model.dart#L332-L356) (the existing `inboxFlatItemsProvider`) decides. Document the choice in the PR description.
- [ ] **Producer side — extend `inboxFlatItemsProvider`** so it emits community-chat rows alongside `DailyItem` rows, sorted together by `lastMessageAtUnixSec` descending. For each community in `selectedCommunityProvider.enabledCommunities` (which already returns `[selectedCommunity]` in single-community mode and all communities in All Circles mode — see [community_providers.dart:136-138](../../app/lib/services/providers/community_providers.dart#L136-L138)), watch `communityConversationProvider(community.id)`; emit a row only when the conversation's `unreadCount > 0`. The provider's existing `effectiveCommunityIds` filter for DailyItems already gates the same way ([line 119-121](../../app/lib/services/providers/community_providers.dart#L119-L121)) — community chats inherit the same scoping for free.
- [ ] **Riverpod plumbing.** Watching N `communityConversationProvider(id)` instances inside `inboxFlatItemsProvider` keeps them alive automatically via the dependency graph — no manual lifecycle management. If `inboxFlatItemsProvider` is currently a `Provider`, it stays a `Provider`; if it's a `Notifier`, the new community-watching logic still uses `ref.watch` synchronously inside the read path, no `await`, no `state = ...` after async — so no new `if (!ref.mounted) return;` guards required.
- [ ] **Renderer side — inbox row.** In [portfolio_inbox_screen.dart](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart), `_buildItemList` currently renders `DailyItem` rows. Extend it (or `InboxRow`) to dispatch on the new union/variant: community-chat rows render with the same row geometry as DailyItem rows (72×72 photo, type label, title, message preview, time-ago, unread badge — see the snapshot of `inbox_row.dart:19-50` in the research notes). Reuse the existing photo/preview/badge sub-widgets; the divergence from DailyItem is entirely about *what data is bound*, not *how it's laid out*.
- [ ] **Type label.** The row's type label reads `context.l10n.communityRowType` ("Community") — parity with the existing `"Transfer"`, `"Request"`, etc. labels. The label color is the row's accent color via `AppColors.<semantic>(context)`; pick the same semantic the existing pinned card used (`AppColors.transferCoral`) **only if** that was a deliberate community-chat color in #1646; otherwise pick a neutral or community-themed semantic. If a new `AppColors` semantic is needed, add it before this phase ships. The full coral *background* tint goes away (it was the "treat differently" treatment); only the accent color on the type label remains.
- [ ] **Delete the old pin code.** Remove [portfolio_inbox_screen.dart:206-209](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart#L206-L209) (the pin sliver), [`_buildPinnedCommunityChat`](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart#L221-L366), [`_buildCommunityPhoto`](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart#L368), and any helper exclusive to the pin (e.g. `_formatTimeAgo` if it's not used elsewhere — verify with grep). The accessibility key `a11yCommunityChatPin` introduced by #1646 is no longer needed if no other call site uses it; remove it from both ARB files (or repurpose for the new row's `Tappable` if the row needs a per-row label).
- [ ] **Cache & invalidation.** No new cache keys needed — the data already flows through `communityConversationProvider`, which already participates in `dailyCacheInvalidationProvider` per [docs/client/caching.md](../../docs/client/caching.md) §Pattern 7. Verify that a chat send (which invalidates `dailyCacheInvalidationProvider`) propagates through the extended `inboxFlatItemsProvider` and re-renders the inbox without manual refresh. **Fan-out scale note:** in All Circles mode, this triggers N parallel `communityConversationProvider(id)` cache lookups (N = user's community count). The Stash cache absorbs warm-state cost; cold-start is N parallel HTTP fetches. Acceptable today; revisit with a batched `GetUnreadCountsForCommunities` RPC if observed latency warrants it post-launch.
- [ ] **Accessibility** ([docs/client/accessibility.md](../../docs/client/accessibility.md)): the row's `Tappable` `semanticsLabel` must include the community name and the unread count, so screen readers can distinguish multiple community-chat rows. Add `a11yCommunityChatRow(communityName, unreadCount)` as a parameterized ARB key. Use `IconAction`/`Toggle`/`Tappable` per the primitive table; no raw `GestureDetector`. The unread-count badge (a colored dot/number) falls under the `avoid_color_only_status` rule — wrap in `Semantics` or `Tooltip` announcing the count, parity with the existing inbox-row badge handling.
- [ ] **i18n**: add `communityRowType` ("Community") and `a11yCommunityChatRow` (parameterized) to both [app_en.arb](../../app/lib/l10n/app_en.arb) and [app_es.arb](../../app/lib/l10n/app_es.arb) with `@key` metadata in English. Remove the now-unused `a11yCommunityChatPin` if confirmed orphan (run `npm run lint:dart:i18n` to verify no stale references).
- [ ] **Tests** ([docs/client/testing.md](../../docs/client/testing.md)): extend the existing `inbox_view_model_test.dart` with a `ProviderContainer`. `communityConversationProvider` is a `family`, so override with `overrideWith((ref, communityId) => ...)` (not `overrideWithValue`) to return distinct fake `ConversationItem`s per community ID. Verify (a) single-community mode interleaves the selected community's row (when it has unreads) with DailyItems by `lastMessageAtUnixSec`, (b) single-community mode emits zero community rows when the selected community has zero unreads, (c) All Circles with multiple communities emits one row per community with unreads, sorted with DailyItems by `lastMessageAtUnixSec`, (d) reactivity: when one community transitions from `unreadCount == 0` to `unreadCount > 0`, the row appears in the correct sort position within the same `ProviderContainer` lifecycle. Widget test that the inbox renders the new row geometry correctly and that tapping a community row navigates to the community Discuss tab via the same `NavigationHelpers.pushScreen(...)` call the pin used.
- [ ] `npm run test:app`, `npm run lint:dart:i18n`, `npm run lint:dart:a11y`, `npm run lint:dart:size` clean.

**Validates:** In single-community mode, a community chat with unreads appears as an inbox row interleaved with DailyItems by recency (the old coral-tinted pin is gone). In All Circles mode with 3 communities (2 with unreads), 2 community-chat rows appear in the inbox, mixed and sorted with DailyItems. With zero communities having unreads, no community rows appear (no empty-state widget either).

### Phase 2: Media support in `CommunityChatPane`

**Goal:** Community members can upload, view, delete, and reorder media in the Discuss tab, matching the experience/request chat-pane pattern.

**Tasks:**

- [ ] Extend `CommunityChatPane`'s constructor to accept media plumbing, mirroring [experience_chat_pane.dart:18-34](../../app/lib/presentation/screens/experience/widgets/experience_chat_pane.dart#L18-L34):
  - `final List<MediaItemData> mediaItems`
  - `final Future<void> Function() onAddMedia`
  - `final Future<void> Function(String mediaId) onDeleteMedia`
  - `final Future<void> Function(List<String> mediaIds)? onReorderMedia`
  - `final bool isOwner`
- [ ] Forward all five into the existing `InlineConversationView` call at [community_chat_pane.dart:42-64](../../app/lib/presentation/screens/communities/widgets/community_chat_pane.dart#L42-L64). Replace the `const []` / no-op stubs.
- [ ] **Verify the existing community-media flows are repository-backed** before plumbing them through a new entry point. Per [docs/client/architecture.md](../../docs/client/architecture.md) §"Architectural Rules": viewmodels can only call repositories, never services. Audit `communityEditProvider`'s `uploadMedia`, `deleteMedia`, `reorderMedia`, `loadAllMedia`, `loadBackgroundMedia`, `loadBackgroundAttribution` methods — confirm each calls a repository (likely `CommunityRepository` or `MediaRepository`), not a service directly. If any path calls `*Service` directly, fix in-place in this phase before fan-out — surfacing the same violation through `CommunityChatPane` would multiply the architectural debt.
- [ ] In [community_content_view.dart:383-393](../../app/lib/presentation/screens/communities/community_content_view.dart#L383-L393), pass the existing community-media state and handlers into `CommunityChatPane`:
  - `mediaItems: editState.allMediaItems`
  - `onAddMedia: _showMediaPickerDialog` (the pane will await it; `_showMediaPickerDialog` is already async via the picker helpers)
  - `onDeleteMedia: _handleDeleteMedia`
  - `onReorderMedia: widget.canEdit ? _handleReorderMedia : null`
  - `isOwner: widget.canEdit`
- [ ] **Async safety**: the new callbacks are forwarded directly — no new state mutations introduced inside `CommunityChatPane`. The state mutations happen inside `communityEditProvider`, which the existing media flow already exercises and presumably already has `if (!ref.mounted) return;` guards (verify; if not, fix in this phase since we're now calling those flows from a new entry point).
- [ ] **Image caching** ([docs/client/caching.md](../../docs/client/caching.md) §"Image Cache Keys"): the `MediaItemData` flowing through to `MediaCarousel` and `InlineConversationView` already uses presigned URLs paired with stable IDs. Concretely verify (grep + visual inspection of the rendered widget tree on emulator) that every image bound to community-chat media uses `CachedMediaImage` with `cacheKey: ImageCacheKeys.thumbnail(mediaId)` for chat-strip thumbnails (< 200px) and `ImageCacheKeys.full(mediaId)` for full-size carousel views (>= 200px). Per [CLAUDE.md](../../CLAUDE.md) §"Media Display", any new `Image.network` of a presigned URL is a regression — grep the diff before submitting. Sharing a key between thumbnail and full would serve a 400px asset to a 4000px viewport; correct keying is load-bearing.
- [ ] **Accessibility**: media picker dialogs use `Tappable`/`IconAction` per existing implementation; no new a11y surface in this phase since the underlying widgets (`MediaCarousel`, `MediaPickerDialog`) already conform.
- [ ] **i18n**: no new user-visible strings — the existing media-picker / carousel strings already cover this code path.
- [ ] **Tests** ([docs/client/testing.md](../../docs/client/testing.md)): widget test for `CommunityChatPane` verifying that media items render (use a fake `MediaItemData` list), and that tapping the add-media affordance invokes the supplied `onAddMedia` callback. Update the existing `community_content_view_model` test (if any) to assert that `_showMediaPickerDialog` and `_handleDeleteMedia` paths still work when triggered from the chat-pane entry. Disposal test stays unchanged (no new `Notifier` introduced).
- [ ] **Verify on emulator** per the `ripls-app-logs` skill: open Discuss tab → upload an image → see it in the chat strip → send a message referencing it → verify it appears in the second user's chat. Repeat with a video. Test reorder (creator account) and delete-own-upload (member account).
- [ ] `npm run test:app`, `npm run lint:dart:size`, `npm run lint:dart:a11y` clean.

**Validates:** Community members can attach images and videos to community chat messages; creators can reorder/delete any chat media; non-creator uploads can be deleted by their uploader.

### Phase 3: Attribution info button

**Goal:** The attribution overflow menu is reachable from every entry into the community content view, including the inbox-pin → `CommunityScreen` path that bypasses `ContentOwnerRow`.

**Tasks:**

- [ ] Add an `IconAction` info button to [community_screen.dart:69-82](../../app/lib/presentation/screens/communities/community_screen.dart#L69-L82)'s top row, right-aligned (back button stays left). Use `Icons.info_outline`. Implementation pattern:
  - Lift the overflow-menu invocation into a stateless top-level helper function — **not** a widget, **not** a class with state — at `app/lib/presentation/screens/communities/widgets/community_overflow_menu_helper.dart`. Signature: `void showCommunityOverflowMenu(BuildContext context, WidgetRef ref, {required String communityId, required bool canEdit})`. Inside it: read `communityEditProvider(communityId)` for `backgroundAttribution` + `allMediaItems`, then call `showContentOverflowMenu(...)`. Both `CommunityScreen` (in its top row) and `CommunityContentView._showOverflowMenu` (the actor-row info-tap) call the same helper. **Do not use `GlobalKey<_CommunityCreationFeedViewState>`** — it tightly couples two screens through a private state class and breaks the moment either screen is refactored.
- [ ] In [community_content_view.dart:296](../../app/lib/presentation/screens/communities/community_content_view.dart#L296), the existing actor-conditional path is fine as-is — the new top-right info button on `CommunityScreen` covers the actor-less entry. **Do not** modify the actor-conditional logic; that path is shared with the feed-rendering case where `ContentOwnerRow` is the right surface. The fix is additive at the screen layer, not subtractive at the content-view layer.
- [ ] If `showFeedHeader` is true, `CommunityScreen` is **not** the entry point (the feed renders `CommunityContentView` directly). So the new info button on `CommunityScreen` does not conflict with the feed-header info-tap at [line 311](../../app/lib/presentation/screens/communities/community_content_view.dart#L311). Confirm by audit, not runtime check.
- [ ] **Accessibility** ([docs/client/accessibility.md](../../docs/client/accessibility.md)): use `IconAction` (not raw `IconButton`). Per the doc's "When to reach for which primitive" table, `IconAction` requires **both** `semanticsLabel` and `tooltip` (not "either" — both). The same l10n string can satisfy both, so use `tooltip: context.l10n.communityInfoTooltip` and `semanticsLabel: context.l10n.communityInfoTooltip` (one ARB key, two parameters). The `require_semantic_label_on_icon_button` and `require_l10n_for_semantic_labels` lint rules in `scripts/check_a11y.js` reject either being absent or being a raw string literal.
- [ ] **i18n**: add **one** ARB key `communityInfoTooltip` ("Community info" or equivalent) to both `app_en.arb` and `app_es.arb` with `@key` metadata in English. Per [docs/client/i18n.md](../../docs/client/i18n.md) §"Key Naming Convention", use the `community*` prefix. Per the `a11y` prefix convention, the key would be `a11yCommunityInfoButton` if the label *only* served accessibility — but here it doubles as the visible tooltip for sighted users, so the `community*`-prefixed semantic key is the right home. One key, two parameters (`tooltip:` and `semanticsLabel:`), one ARB entry per locale.
- [ ] **Theming**: icon color from `AppColors.<semantic>(context)`, not a hardcoded color. White-on-dark to match the back button — most likely `AppColors.iconOnDark(context)` or whatever the back button uses today (mirror it).
- [ ] **Tests** ([docs/client/testing.md](../../docs/client/testing.md)): widget test on `CommunityScreen` that (a) the info icon renders, (b) tapping it invokes the overflow menu, (c) when `backgroundAttribution` is populated, the attribution row appears in the menu. Use the `localizationsDelegates` helper from [app/test/helpers/l10n_helpers.dart](../../app/test/helpers/l10n_helpers.dart). Test on emulator: navigate from inbox pin → see info icon → tap → see attribution. Then navigate from feed → confirm `ContentOwnerRow` info-tap still works (regression check on the unchanged path).
- [ ] `npm run test:app`, `npm run lint:dart:i18n`, `npm run lint:dart:a11y`, `npm run lint:nav` clean.

**Validates:** Tapping the community card in inbox → landing on `CommunityScreen` → tapping the info icon → seeing the attribution / Edit / Share / Gallery overflow menu. Feed-context entry still surfaces the same menu via the actor row, unchanged.

### Phase 4: End-to-end verification

**Goal:** The three TODOs work together on a real device, and no regressions slipped into the existing #1646 surfaces.

**Tasks:**

- [ ] On Android emulator + iOS simulator, exercise the matrix:
  - User with 1 community: All Circles is hidden in sidebar (existing #1646 behavior). Inbox shows a community-chat row only when that community has unread messages, interleaved by recency with the user's other inbox items. **No more coral-tinted pinned card** — confirm the #1646 visual is gone.
  - User with 3 communities, 2 with unreads: All Circles mode shows 2 community-chat rows mixed with DailyItem rows in a single recency sort; single-community mode shows the selected community's row when it has unreads (or no community row at all when it doesn't), again interleaved with DailyItems.
  - From any community-chat row, tap → land on `CommunityScreen` → see info icon at top-right; Discuss tab → upload a photo, send message → second user receives it; back to inbox → unread badge cleared and the row drops out of the inbox until a new unread arrives.
  - **Reactive reappearance:** after reading the last unread in a community (the row drops out), have the second user post a new message → the row reappears in the inbox at its correct sort position in real time, without manual refresh. Validates that the extended `inboxFlatItemsProvider` stays subscribed to `communityConversationProvider(id)` and that `dailyCacheInvalidationProvider` propagates end-to-end.
  - **Sort interleaving:** with one community-chat row at `lastMessageAtUnixSec = T1` and a DailyItem at `lastMessageAtUnixSec = T2 > T1`, confirm the DailyItem renders above the community row and vice-versa. Validates the single-sort directive end-to-end.
  - **Non-creator media path:** as a non-creator member, upload media in the Discuss tab — confirm any-member-uploads works (resolved-decision behavior). As a non-creator, also confirm reorder is unavailable and that delete is restricted to your own uploads.
- [ ] Run server side: `npm run build`, `go test ./...` — no server changes expected, but confirm the existing #1637 server tests still pass.
- [ ] Run client side: `npm run test:app`, `npm run lint:dart:i18n`, `npm run lint:dart:a11y`, `npm run lint:nav`, `npm run lint:dart:size` all clean.
- [ ] Update or add system test if any of the three TODOs is in scope for `ripls-update-system-tests` — at minimum, the inbox-pin → media-attach → attribution-view path is a likely candidate.

**Validates:** The follow-up work matches the spec, doesn't regress #1646, and ships clean through CI.

## Related Issues

- **#1637 — Community-wide chat foundation.** Direct prerequisite. Plan: [docs/issues/1637-community-wide-chat.md](1637-community-wide-chat.md). Shipped via PR #1646.
- **#1646 — Add community chat (PR).** The PR whose three TODOs spawned this issue. URL: https://github.com/Type2Fun-Foundation/ripls/pull/1646.
- **#1629 — Sidebar single-select + All Circles.** Provides the `selectedCommunityProvider.isAllCircles` flag that Phase 1's pinned-list filter depends on.
- **#1107 — Per-community notifications.** Not a direct dependency; surfaced because Phase 1 makes "many communities chatting" more visually present in the inbox, which may sharpen the case for per-community mute controls.
- **#1648 — Image-mode community creation: real `StreamGenCommunityFromMedia` RPC.** Adjacent: when image-generated communities ship, the attribution surface (TODO 3) becomes more meaningful since the cover image will commonly come from Pexels/Unsplash and need crediting. Phase 3's info-button work is a load-bearing prerequisite for that flow being usable.

---

## Audit Notes

Reviewed against [docs/client/architecture.md](../../docs/client/architecture.md), [docs/client/caching.md](../../docs/client/caching.md), [docs/client/i18n.md](../../docs/client/i18n.md), [docs/client/accessibility.md](../../docs/client/accessibility.md), [docs/client/testing.md](../../docs/client/testing.md). Server docs ([docs/server/architecture.md](../../docs/server/architecture.md), [docs/server/observability.md](../../docs/server/observability.md)) reviewed and confirmed not applicable — this issue is entirely Flutter-side; no proto, RPC, or server-handler changes.

**Changes made during audit:**

1. **Phase 1 — Provider type clarified.** Original task said "Add a derived provider" without naming the Riverpod type. Tightened to `Provider<List<CommunityItem>>` (a plain `Provider`, not a `Notifier`/`AsyncNotifier`), since the new piece is purely a derivation from existing providers with no internal state, no `await`, and no side effects. Per [docs/client/architecture.md](../../docs/client/architecture.md) §"Providers", `Provider` is the right choice for value derivations; `Notifier` would be over-tooling.

2. **Phase 1 — Family override pattern in tests.** Original test bullet said "with mocked `communityConversationProvider`" without specifying the override mechanism. `communityConversationProvider` is a `family` — `overrideWithValue` doesn't apply. Updated to specify `overrideWith((ref, communityId) => ...)` and added a reactivity test case (transition from `unreadCount == 0` to `unreadCount > 0` should reflect in the derived provider's value within the same `ProviderContainer` lifecycle).

3. **Phase 1 — Fan-out scale note.** Original task glossed over the cost of N parallel `communityConversationProvider(id)` lookups in All Circles mode. Added a "Fan-out scale note" sub-bullet under cache/invalidation: warm-state is fine (Stash cache absorbs it); cold-start is N parallel HTTP fetches. Flagged a future-work option to add a batched `GetUnreadCountsForCommunities` RPC if a user with >20 communities reports load latency post-launch. Not a Phase 1 blocker, but a known signal worth tracking.

4. **Phase 2 — Repository-layer pre-flight.** Per [docs/client/architecture.md](../../docs/client/architecture.md) §"Architectural Rules" #1: viewmodels can only call repositories, never services. The plan was about to plumb existing `communityEditProvider` media flows through a new entry point (`CommunityChatPane`), but did not require an audit of those flows for service-direct calls. Added an explicit pre-flight task to verify that `communityEditProvider`'s `uploadMedia`/`deleteMedia`/`reorderMedia`/`loadAllMedia`/`loadBackgroundMedia`/`loadBackgroundAttribution` each call a repository (likely `CommunityRepository` or `MediaRepository`), and to fix any service-direct calls in-place before fan-out. Surfacing the same violation through a second entry point would multiply architectural debt.

5. **Phase 2 — Image cache keys made concrete.** Original "verify `CachedMediaImage` is used downstream" was vague. Replaced with the concrete rule from [docs/client/caching.md](../../docs/client/caching.md) §"Image Cache Keys": `ImageCacheKeys.thumbnail(mediaId)` for chat-strip thumbnails (< 200px) and `ImageCacheKeys.full(mediaId)` for full-size carousel views (>= 200px), and called out the load-bearing reason (sharing keys serves a 400px asset to a 4000px viewport).

6. **Phase 3 — Helper shape pinned down.** Original suggested either a `GlobalKey<_CommunityCreationFeedViewState>` (with a "avoid if possible" hedge) or "a small helper". Pinned this to a single concrete shape: a stateless top-level function in `community_overflow_menu_helper.dart` with the signature `void showCommunityOverflowMenu(BuildContext context, WidgetRef ref, {required String communityId, required bool canEdit})`. Both `CommunityScreen` and `CommunityContentView` call the same function. Removed the `GlobalKey` option entirely with rationale (couples two screens through a private state class; breaks under refactor).

7. **Phase 3 — `IconAction` requires both `tooltip` and `semanticsLabel`.** Plan said "either equal to the tooltip or via [a separate key]". Per [docs/client/accessibility.md](../../docs/client/accessibility.md) "When to reach for which primitive" table, `IconAction` requires *both*. Updated to specify both parameters use the same l10n key, and called out the two specific lint rules (`require_semantic_label_on_icon_button`, `require_l10n_for_semantic_labels`) that enforce this.

8. **Phase 3 — i18n key consolidated.** Original plan added two ARB keys (`communityInfoTooltip` and `a11yCommunityInfoButton`). Since the same string serves the visible tooltip *and* the screen-reader label, consolidated to one key (`communityInfoTooltip`), with a note explaining that the `a11y` prefix is reserved for labels that *only* serve accessibility.

9. **Phase 4 — Manual test matrix expanded.** Added two test cases that the original matrix missed:
   - **Reactive reappearance:** a pin should reappear in All Circles mode when a community's unread count transitions from 0 → 1+ in real time (validates Phase 1's reactivity end-to-end with `dailyCacheInvalidationProvider`).
   - **Non-creator media path:** validates whichever upload-permission decision is resolved in Open Questions (creator-only vs. any-member upload) and verifies parity with the carousel behavior on the same screen.

10. **Open Questions — Upload permission parity.** The plan did not flag the inconsistency between the existing `_showMediaCarousel` (creator-only `onAddMedia`) and the experience/request chat-pane convention (any-member `onAddMedia`). Added an explicit open question with a recommendation (any-member uploads for chat, creator-only uploads for the curated carousel — with a doc-comment in `community_chat_pane.dart` explaining the divergence). Phase 2's existing plumbing (`onAddMedia: _showMediaPickerDialog` unconditional) already aligns with this recommendation.

**No changes needed:**

- **MVVM layering.** The plan respects all three Architectural Rules: viewmodels-only-call-repositories (no service calls added in viewmodels), repositories-only-call-services (no plumbing changes here), screens-only-interact-with-viewmodels (the new pin layout consumes a provider, not a service or repository directly). The new `inboxPinnedCommunitiesProvider` is a presentation-layer derivation, not a data-layer hop, so the rule about repository-mediated data access doesn't apply.
- **Async safety.** The new derived provider has no `await` and no `state = ...` after async work, so no `if (!ref.mounted) return;` guards or `SafeNotifierMixin` are required for it. Where the plan does cross async gaps (Phase 4 manual tests, Phase 2 callback forwarding), the existing notifiers are already responsible — and the plan doesn't introduce new async surfaces in viewmodels.
- **Theme-aware colors.** Phase 3 already calls out using `AppColors.<semantic>(context)` (matching the back button's color) and forbids hardcoded `Color(0xFF...)`.
- **Navigation.** Phase 1 reuses the existing `NavigationHelpers.pushScreen(...)` call inside `_buildPinnedCommunityChat` ([portfolio_inbox_screen.dart:243](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart#L243)) — no new navigation paths added.
- **File size.** None of the touched Dart files (`portfolio_inbox_screen.dart`, `community_chat_pane.dart`, `community_screen.dart`, `community_content_view.dart`, `portfolio_inbox_view_model.dart`) is anywhere near the 1000-line gate. The new helper file is small. `lint:dart:size` will pass.
- **Repository composition shallow-ness.** No new repositories or repository-to-repository dependencies introduced.
- **Disposal testing.** No new async `Notifier` introduced (the new derived `Provider` doesn't need a disposal test; only stateful `Notifier`s do per [docs/client/testing.md](../../docs/client/testing.md) §"Stream Management"). Phase 2 doesn't add a notifier; Phase 3 doesn't add a notifier. Existing notifiers (`CommunityContentViewModel`, `PortfolioInboxNotifier`) keep their existing disposal tests.
- **Server-side concerns.** No proto changes, no RPC changes, no handler changes, no observability/logging changes — entirely Flutter-side. [docs/server/architecture.md](../../docs/server/architecture.md) and [docs/server/observability.md](../../docs/server/observability.md) reviewed but not applicable.
