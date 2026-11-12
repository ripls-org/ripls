# Issue #979: Community creation

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/979
**Filed:** 2026-04-01 | **Last activity:** 2026-04-01
**Labels:** none | **Assignees:** tescobar
**Context docs:** docs/client/content.md

## Problem Statement

Three related changes are needed for the community creation experience:

1. **Post-creation navigation:** After creating a new community, users are currently taken to the feed (via `PostCreationService.handlePostCreation()`). They should instead be taken to the community metrics screen, which is the natural next step for a newly created community with no content yet.

2. **Community feed card migration:** The `CommunityContentView` currently uses an older, simpler layout (gradient + title + description + action bar) that predates the new content view architecture. It needs to be migrated to match the new design pattern used by `GearContentView`, `ExperienceContentView`, and `RequestContentView` — the `Stack`-based layout with `ContentGradientOverlay`, `ContentOwnerRow`, `ContentEditBar`, and feed header support. Unlike other content types, the community view has no tabs (no workflow, no details, no chat) — it's a single-pane view.

3. **Terminator nudge bug:** The terminator nudge (the "you're all caught up" card at the end of the feed, `is_terminator = true`, Variant 3) is not appearing. This is a bug that needs to be investigated and fixed.

## Context from Comments

No comments on this issue. The requirements are clear from the description: navigate to metrics after community creation, and migrate the community content view to the new design.

## Potential Solutions

### Option A: Incremental migration — navigation first, then content view

- **Approach:** First fix the post-creation navigation to go to community metrics. Then separately migrate `CommunityContentView` to the tabbed design pattern.
- **Pros:** Lower risk per change, easier to review, can ship navigation fix quickly.
- **Cons:** Two separate PRs, community card is temporarily inconsistent with other content cards.
- **Effort:** Medium (navigation: small, content view migration: medium).

### Option B: Full rewrite of CommunityContentView + navigation in one PR

- **Approach:** Rewrite `CommunityContentView` to match the tabbed pattern and fix navigation in a single PR.
- **Pros:** Single coherent change, no intermediate inconsistent state.
- **Cons:** Larger PR to review, higher risk of regressions.
- **Effort:** Medium-large.

### Option C: Minimal fixes only

- **Approach:** Fix navigation, add basic styling fixes to the existing community card without full migration.
- **Pros:** Fastest to ship.
- **Cons:** Accumulates more design debt, community card stays inconsistent with other content types.
- **Effort:** Small.

## Recommendation

**Option A: Incremental migration.** The navigation fix is straightforward and high-value. The content view migration is a separate concern that benefits from focused review. Two phases keeps each change reviewable and testable.

## Open Questions (Resolved)

- **What tabs should the community content view have?** **Answer:** No Details or Chat tabs. The community content view is a single-pane view — just the community info (name, description, invite, media). No tab bar needed. This is simpler than other content types since communities have no workflow or conversation.

- **What does "termination cards in the feed" mean?** **Answer:** The terminator nudge (`is_terminator = true`, Variant 3) is not appearing in the feed. This is a bug — the nudge should render at the bottom of the feed but currently doesn't. Phase 3 is a bug fix, not a new feature.

- **Should the community metrics screen be the selected community's metrics, or a specific community's metrics?** **Answer:** Navigate to the newly created community's metrics. The `CommunityMetricsScreen` already exists — we just need to open it for the user after closing the community creation modal. Set the new community as the selected community, then navigate to the Metrics tab (index 2).

## Implementation Plan

### Phase 1: Post-creation navigation to community metrics

**Goal:** After creating a community, navigate the user to the community metrics screen instead of the feed.

**Tasks:**
- [ ] Modify `GenCommunityNotifier.createCommunity()` in `gen_community_view_model.dart` to not call `_postCreationService.handlePostCreation()` (which navigates to feed)
- [ ] Instead, after creation: refresh the communities list via `CommunityRepository`, set the new community as selected via `selectedCommunityProvider`, refresh the feed via `FeedRepository.invalidateFeed()` + `feedProvider.notifier.refresh()`, and navigate to the Metrics tab (index 2) via `homeProvider.notifier.navigateToTab(2)`
- [ ] **Architecture note:** `GenCommunityNotifier` already accesses `communityRepositoryProvider` and `homeProvider` — follow the existing pattern. All data access must go through repositories, not services directly (per `docs/client/architecture.md` rule 1-2).
- [ ] Verify the `MetricsScreen` correctly picks up the newly selected community and shows `CommunityMetricsScreen`
- [ ] Test the flow end-to-end: create community -> lands on metrics for that community

**Validates:** Users land on the metrics screen after creating a community, giving them a clear next-step experience.

### Phase 2: Migrate CommunityContentView to new design

**Goal:** Make the community feed card consistent with gear/experience/request content views.

The community view is a **single-pane view** (no tabs) since communities have no workflow, details metadata, or conversation. The migration focuses on adopting the shared `Stack`-based layout and shared widgets.

**Tasks:**
- [ ] Audit the current `CommunityContentView` constructor and state to identify what maps to the new pattern
- [ ] Adopt the `Stack`-based layout: `ContentGradientOverlay`, `ContentOwnerRow`/`ContentEditBar`, feed header support. Use `GearContentView` as the reference template but omit `ContentViewTabBar` and tab panes — the community view is single-pane (name, description, invite button, media).
- [ ] **i18n compliance:** All button text and user-visible strings must use `context.l10n.*` keys from ARB files (per `docs/client/i18n.md`). Add new keys with `community*` prefix to both `app_en.arb` and `app_es.arb`. ViewModels must not resolve strings — only expose typed state (enums/codes).
- [ ] **Theme compliance:** Replace all hardcoded colors in the current `CommunityContentView` (e.g., `Colors.blue`, `Colors.grey.shade700`, `Colors.black.withValues(alpha: 0.8)`) with theme-aware `AppColors.*` helpers (per `docs/client/architecture.md` Colors and Theming section). Use `AppColors.cardBackground(context)`, `AppColors.surface(context)`, etc.
- [ ] Update `CommunityEditViewModel` to include `isOwner` and other state fields expected by the shared content widgets. Use Freezed for immutable state and the Riverpod Notifier pattern (per `docs/client/architecture.md`).
- [ ] Update the feed screen's `FEED_ITEM_TYPE_COMMUNITY_CREATED` case to pass the correct parameters to the migrated view
- [ ] Ensure edit mode works: `ContentEditBar` shows Cancel/Editing/Save, editing buffers work for name/description
- [ ] **Cache invalidation:** After saving community edits, invalidate relevant caches via repository methods (`CommunityRepository.invalidate*()`, `FeedRepository.invalidateFeed()`), not manual cache clearing (per `docs/client/caching.md`). The existing `_saveChanges()` pattern in the current view already does this correctly — preserve it.
- [ ] **Navigation:** If the community content view is pushed as a standalone screen (not in feed), use `NavigationHelpers.pushScreen()` + `SwipeToCloseMixin` (per `docs/client/architecture.md` Navigation section). Do NOT use `Navigator.push(MaterialPageRoute(...))`.
- [ ] Test the community card in the feed and as a standalone screen

**Validates:** Community cards in the feed look and behave consistently with other content types, using shared components.

### Phase 3: Fix terminator nudge not appearing

**Goal:** The terminator nudge ("you're all caught up" card) is not showing at the bottom of the feed. Investigate and fix.

**Tasks:**
- [ ] Investigate why the terminator nudge (`is_terminator = true`, Variant 3) is not rendering in the feed. Check: Is the server sending it? Is the feed screen handling `FEED_ITEM_TYPE_NUDGE` correctly? Is the nudge being filtered out or skipped?
- [ ] Check `NudgeContentView` rendering for terminator nudges — verify `nudge_card_variants.dart` Variant 3 (Headline Card) path is reachable
- [ ] Fix the bug — likely in the feed screen's item builder or in the nudge data pipeline
- [ ] Manual QA: scroll through the entire feed and verify the terminator nudge appears at the bottom

**Validates:** The feed has a clean end-state with the terminator nudge visible at the bottom.

## Audit Notes

The following compliance issues were identified and addressed in this plan:

1. **Architecture layering (docs/client/architecture.md):** Phase 1 tasks now explicitly call out that all data access must go through repositories, and navigation uses established provider patterns. ViewModels are the only layer calling repositories.
2. **i18n compliance (docs/client/i18n.md):** Phase 2 now requires all user-visible strings to use `context.l10n.*` from ARB files with `community*` prefix, and that ViewModels never resolve strings.
3. **Theme compliance (docs/client/architecture.md):** Phase 2 now flags the hardcoded colors in the current `CommunityContentView` (`Colors.blue`, `Colors.grey.shade700`) that must be replaced with `AppColors.*` theme-aware helpers.
4. **Cache invalidation (docs/client/caching.md):** Phase 2 explicitly requires cache invalidation via repository methods after mutations, not manual clearing.
5. **Navigation patterns (docs/client/architecture.md):** Phase 2 requires `NavigationHelpers.pushScreen()` + `SwipeToCloseMixin` for standalone screen navigation.
6. **No server changes required:** This issue is entirely client-side. The feed API already returns `FEED_ITEM_TYPE_COMMUNITY_CREATED` items, and the community metrics screen already exists.

## Related Issues

- None referenced in the issue.
