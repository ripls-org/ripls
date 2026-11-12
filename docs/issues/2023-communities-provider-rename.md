---
issue: 2023
title: "tech_debt: rename selectedCommunityProvider and remove vestigial single-select API (post-#1895)"
---

# Issue #2023: tech_debt: rename selectedCommunityProvider and remove vestigial single-select API (post-#1895)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2023
**Filed:** 2026-05-18 | **Last activity:** 2026-05-19
**Labels:** none | **Assignees:** briangbrown
**Context docs:** none

> **Audit pass (2026-05-19):** Reviewed against [docs/client/architecture.md](../client/architecture.md), [docs/client/caching.md](../client/caching.md), [docs/client/i18n.md](../client/i18n.md), [docs/client/accessibility.md](../client/accessibility.md), [docs/client/testing.md](../client/testing.md), and the proto/server conventions in [CLAUDE.md](../../CLAUDE.md). Updates appear as **\[Audit]** callouts and as a new "Compliance Requirements" section. Server docs were read but do not apply — this is a client-only rename + cleanup with no proto/RPC/server changes.

## Problem Statement

After #1895 removed the per-community filter, `selectedCommunityProvider` no longer represents a selection — it represents the user's portfolio of communities plus loading/error state. Every code path now forces `selectedCommunityId` back to `null`. The old single-select shape survives as a misleading API surface where the name promises "the community the user picked" but the implementation guarantees "always `null`".

This isn't theoretical: #2000 (PR #2021, merged 2026-05-19) was caused by exactly this trap — `Manage Members` called `communityState.selectedCommunity` expecting the community in view and got `null` on every render, permanently disabling the Invite FAB. The misleading name plus a getter that compiles without warning is a recurring-bug pattern.

The fix is a rename plus a delete pass over the dead API. The user has specified the destination names (see Recommendation).

## Context from Comments

The issue has no GitHub comments, but the request that opened this plan carries a key directive from briangbrown:

> "In terms of the names, I think just `communitiesProvider`, `communitiesState`, `communitiesNotifier` make sense if there are no other naming conflicts. It is implied that they are the user's communities that they are a part of."

I checked: `grep -rn 'communitiesProvider\|communitiesState\|communitiesNotifier'` finds **zero** existing identifiers with those names. The short form is free to use.

## Codebase Findings

What exists today, anchored to real lines:

- `app/lib/services/providers/community_providers.dart:306-309` — `selectedCommunityProvider` declaration, a `NotifierProvider<SelectedCommunityNotifier, SelectedCommunityState>`.
- `app/lib/services/providers/community_providers.dart:120-158` — `SelectedCommunityState`. Live fields: `selectedCommunityId` (always written `null` post-#1895), `communities`, `isLoading`, `errorMessage`. Live getters: `isAllCircles` (always true), `effectiveCommunityIds` (always returns full list), `enabledCommunityIdList` (alias), `selectedCommunity` (always `null` — the #2000 trap), `enabledCommunities` (always returns full list).
- `app/lib/services/providers/community_providers.dart:160-304` — `SelectedCommunityNotifier`. Live: `setCommunities`, `setLoading`, `setError`, `clearError`, `retryLoadCommunities`, `reloadCommunities`, `clearSelection`. Dead/no-op: `selectAllCircles` (sets the always-`null` field to `null`), `selectCommunity` (`assert(false)` + forces `null`), `switchTo` (alias for `selectCommunity`), `refreshSelectedCommunity` (explicit no-op). Also two private members tied to the dead persistence path: `_persistSelection`, `_selectedCommunityIdKey`, `_legacyEnabledCommunityIdsKey` — the SharedPreferences cleanup in `setCommunities()` (L204-206) still removes the legacy keys; that's still useful for one more release.
- `app/lib/core/utils/community_helper.dart:15-26` — `selectedCommunitiesTitle` reads `state.enabledCommunities` to derive a header title. Caller signature uses `SelectedCommunityState` — needs rename + accessor update.
- `app/lib/main.dart:525,536` — last surviving caller of `state.selectedCommunity?.id` and `.switchTo(...)`. Both load-bearing in the FCM deep-link path; needs migration to a route-driven flow.
- `app/lib/core/router/app_router.dart:407` — calls `.selectCommunity(newCommunity)` after community creation. Same migration shape as #2021's fix in Manage Members.
- `app/lib/presentation/viewmodels/gen_experience_view_model.dart:790` — reads `state.selectedCommunity`. Always-null today, so this line is dead code. Confirm in PR.
- `app/lib/presentation/screens/governance/governance_screen.dart:65,407` — `widget.community ?? ref.watch(selectedCommunityProvider).selectedCommunity` and `.selectCommunity(updatedCommunity)`. The fallback to `selectedCommunity` is unreachable; `widget.community` is the real source.
- `app/lib/presentation/widgets/empty_content_state.dart:70` — `state.selectedCommunity`. Likely #2000-class bug already in code; verify in PR.
- `app/lib/presentation/screens/impact_metrics/metrics_screen.dart:44,82` — same pattern.
- `app/lib/presentation/screens/{experience,gear,request}/{*_content_view,*_screen,*_preview_modal}.dart` — wide spread. Most cases use `selectedCommunity?.id` as a filter; the `widget.communityId` (already present on these screens via routing) is the right replacement. PR #2021 used exactly this shape.
- `app/lib/presentation/screens/{discover,feed,leaderboard,story,governance,request,workshop}/...` — additional spread of `selectedCommunity?.id`, `enabledCommunityIdList`, `effectiveCommunityIds`. All boil down to "the user's full set of communities" or "this screen's community from route".
- `app/lib/services/providers/chat_providers.dart:126`, `app/lib/services/fcm_service.dart` — service-layer reads. Same migration.

**Call-site counts** (excluding generated `.freezed.dart` files): `selectedCommunity` appears across **64** `lib/` files and **19** `test/` files. Mechanical rename + accessor migration.

## Potential Solutions

### Option A — Rename + delete dead API in one PR, multi-commit
- **Approach:** Per the issue body and the user's confirmed naming. Three logical commits in one PR: (1) rename file/class/provider with no behavior change, (2) migrate accessor call sites, (3) delete the dead API + legacy prefs cleanup.
- **Pros:** Atomic — main is never half-renamed. Reviewer sees the rename diff cleanly in commit 1, then the call-site migrations in commit 2, then the API deletion in commit 3. Matches the user's preferred "one commit per phase" rhythm from prior PRs.
- **Cons:** Bigger PR review. Risk of merge-conflict churn against unrelated PRs touching these screens during review.
- **Effort:** Medium.

### Option B — Rename PR first, accessor migration PR second, deletion PR third
- **Approach:** Three separate PRs that land sequentially.
- **Pros:** Each PR is smaller, easier to review and roll back.
- **Cons:** Main stays in a half-migrated state between PRs. Stragglers (new code merged between PR 1 and PR 2) re-introduce the old patterns and require sweep passes. Net more churn.
- **Effort:** Medium-high (PR overhead + sweep risk).

### Option C — Delete dead API only; keep `selectedCommunity*` names
- **Approach:** The issue's Option B fallback. Delete `selectedCommunity` getter and methods, leave the provider/class names alone, optionally `@Deprecated` what remains.
- **Pros:** Smallest diff. Fixes the autocomplete trap.
- **Cons:** Name still implies single-select. Future readers searching for "the selected community" find nothing — better than misleading, but the name keeps lying about the shape of the data.
- **Effort:** Small.

## Recommendation

**Option A** with three commits:
1. **Rename only.** `selectedCommunityProvider` → `communitiesProvider`, `SelectedCommunityState` → `CommunitiesState`, `SelectedCommunityNotifier` → `CommunitiesNotifier`. File renamed via `git mv` (the file is named `community_providers.dart` — already plural; no rename needed there). No accessor changes; `selectedCommunity`, `effectiveCommunityIds`, `enabledCommunities`, etc. all keep their current bodies. Every call site updates only the type/provider symbol. Result: build green, no behavior change.
2. **Migrate call sites off dead/misleading accessors.** Replace `state.selectedCommunity` / `state.selectedCommunity?.id` with route-driven `widget.communityId` where the screen has one (the #2021 pattern), or remove the lookup entirely where it was always going to be `null`. Replace `state.effectiveCommunityIds` and `state.enabledCommunityIdList` with a single survivor — propose `state.communityIds` as a new convenience getter that returns `communities.map((c) => c.id).toList()`. Replace `state.enabledCommunities` with `state.communities`. Remove `state.isAllCircles` references (always true → conditional bodies inline). Replace `.selectCommunity(...)` / `.switchTo(...)` with the community-detail navigation appropriate to each call site (most are post-create flows where pushing the community detail screen is the correct UX). Drop `.refreshSelectedCommunity()` calls.
3. **Delete the dead API + legacy prefs cleanup retention decision.** Remove from `CommunitiesState`: `selectedCommunityId`, `isAllCircles`, `effectiveCommunityIds`, `enabledCommunityIdList`, `selectedCommunity`, `enabledCommunities`. Remove from `CommunitiesNotifier`: `selectAllCircles`, `selectCommunity`, `switchTo`, `refreshSelectedCommunity`, `_persistSelection`. Decide on `_selectedCommunityIdKey` / `_legacyEnabledCommunityIdsKey` cleanup retention — recommendation in Open Questions below.

The user has already chosen the names. The three-commit structure matches the prior PR cadence and keeps each diff reviewable. Reject Option B (sequential PRs) — the half-migrated main between PRs is exactly where the next #2000 lands. Reject Option C — the recurring bug is the name, not just the dead getter.

## Open Questions

- **Add `communityIds` as a convenience getter, or inline `communities.map((c) => c.id).toList()` at every call site?** — Recommendation: **add `communityIds`.** ~10 call sites use `effectiveCommunityIds` / `enabledCommunityIdList`. Inlining `state.communities.map((c) => c.id).toList()` everywhere is noisier than necessary, and `.communityIds` exactly captures the intent. Cheap to add, easier to grep for in future.
- **Legacy SharedPreferences keys — keep the cleanup in `setCommunities()` or drop?** — Recommendation: **keep one more release.** The cleanup at `community_providers.dart:204-206` removes `selected_community_id` and `enabled_community_ids` keys on every `setCommunities` call. Users still upgrading from a pre-#1895 build need this; the cost is two prefs reads per app launch. File a follow-up to remove after 4 weeks in prod.
- **Should `clearSelection()` (the logout cleanup) be renamed?** — Recommendation: **rename to `clear()`.** It resets the entire state, not a "selection" — there's no selection to clear. Same migration footprint as the others; rolls into commit 2.
- **`reloadCommunities({String? selectId})` — what happens to the `selectId` parameter?** — Recommendation: **remove the parameter.** Its only effect today is to call the neutered `selectCommunity()`. Callers that pass `selectId` (e.g. `app_router.dart:407` after community creation) should instead navigate to the new community's detail screen — that's the user-visible behavior the parameter was trying to express anyway. Surface this migration explicitly in commit 2.
- **Is there a hidden caller depending on the always-`null` behavior to short-circuit a code path?** — Recommendation: **assume not, verify with `flutter test` + a manual sweep of all screens that read `selectedCommunity` in PR 1 and PR 2.** No such caller is expected — the issue body and the PR #2021 history both treat the always-`null` as a bug, not a contract — but worth a paragraph in the PR description acknowledging the sweep.
- **Naming: prefer `CommunitiesState` (matches the user's directive) over `UserCommunitiesState` (more explicit about whose communities)?** — Recommendation: **`CommunitiesState` per the user's directive.** Implicit "the user's" is fine when the singular provider for the current user is the only one of its kind in the app. The longer name buys clarity nobody currently needs.
- **\[Audit] Should `errorMessage: String?` be migrated to `UserError? error` as part of this PR?** — Recommendation: **no, file a follow-up.** [docs/client/architecture.md](../client/architecture.md) and [docs/client/i18n.md](../client/i18n.md) both say the canonical viewmodel-error shape is `UserError? error`, with widgets calling `RpcErrorHandler.localize(state.error!, context.l10n)` at render time. The current `errorMessage: 'Unable to reach the server. Please try again.'` literal at `community_providers.dart:262` is a pre-existing i18n violation (viewmodel resolving a string), but it's orthogonal to the rename and would balloon this PR. File a follow-up after Phase 3 lands.
- **\[Audit] `ref.mounted` guards after `await` on the surviving notifier methods?** — Recommendation: **not required for this provider, document why.** Per architecture.md §"Async Safety in Notifiers", `ref.mounted` checks matter for `autoDispose` notifiers — this one is global (no `.autoDispose`) and lives for the app's lifetime, so the late-write footgun doesn't apply. Add a one-line comment on the class noting this so future readers don't add the guards out of habit.

## Compliance Requirements

**\[Audit]** — derived from `docs/client/*` and `CLAUDE.md`. These apply across all three phases below.

### Architecture (MVVM + Riverpod)
- **Notifier lives at the right layer.** The renamed `CommunitiesNotifier` stays in `app/lib/services/providers/community_providers.dart`. Per [client/architecture.md](../client/architecture.md) §"Riverpod 3.0 Notifier Pattern", screen-scoped ViewModels live under `presentation/viewmodels/`; this is a global, app-wide provider for the current user's community membership and correctly sits in `services/providers/` alongside `selectedCommunityProvider`'s original home, `chatProviders`, etc. No move needed.
- **No "controllers."** The rename does not introduce any `*Controller`. Compliant.
- **Repository pattern preserved.** `CommunitiesNotifier.reloadCommunities` and `retryLoadCommunities` already call `communityRepositoryProvider.listUserCommunities()`. The notifier never bypasses the repository. Phase 2 must preserve this — no direct `communityServiceProvider` reads from the notifier.
- **State.communities is render state, not a manual cache.** The "no manual cache maps in ViewModels" rule in architecture.md targets in-ViewModel cache `Map<K,V>` structures. `state.communities` is the rendered output of a repository call; the cache lives in `CommunityRepository` (`Stash`-backed). Compliant.

### Caching
- **No new caching code.** Phase 2 introduces `state.communityIds` as a pure derived getter over `state.communities`. No new cache entries, no new invalidation paths.
- **Repository invalidation is unchanged.** `CommunityRepository.refreshUserCommunities()` (called from community-creation flows) still drives the membership refresh. Phase 2's removal of `reloadCommunities(selectId: ...)` only changes the post-refresh navigation behavior, not the cache-invalidation path.

### i18n
- **No new ARB keys.** The rename and accessor cleanup do not introduce user-visible strings. **\[Audit]** The pre-existing `'Unable to reach the server. Please try again.'` literal at `community_providers.dart:262` is an i18n violation but predates this work and is tracked in Open Questions as a follow-up.
- **`npm run lint:dart:i18n` after Phase 1 and Phase 3** to confirm no regression.

### Accessibility
- **No tap surfaces, semantics, or modal entry points touched.** Phase 2's `state.selectedCommunity` → `widget.communityId` migrations are inside `ConsumerWidget.build()` bodies that already use accessibility primitives where applicable. No new `GestureDetector` / `InkWell` introduced. `npm run lint:dart:a11y` must stay green; the `a11y_allowlist.txt` is empty and must not grow.

### Testing (per [docs/client/testing.md](../client/testing.md))
- **ViewModel-first.** Existing tests for the notifier should continue to pass after Phase 1's rename and through Phase 3's API deletion. Tests under `app/test/services/selected_community_test.dart` (~ filename mirrors the type name; verify path in PR) and 18 other test files reference `selectedCommunity*`; treat them as part of the rename diff in Phase 1.
- **No new widget tests required.** Phase 2's call-site migrations are mechanical accessor substitutions; the behavior under test in the touched screens (Manage Members, Experience, Gear, Request, Workshop) does not change.
- **`ProviderContainer` with overrides for any new test scaffolding.** Per testing.md, the canonical setup is `ProviderContainer(overrides: [...])` + mocked repository — not widget pumps. The notifier test should already use this; rename it in place.
- **Mark new ViewModel tests `@GenerateMocks` per testing.md if any are added.** Likely not needed for this PR — file a follow-up if coverage gaps surface during the rename.

## Implementation Plan

### Phase 1: Rename without behavior change

**Goal:** `selectedCommunityProvider`, `SelectedCommunityState`, `SelectedCommunityNotifier` no longer exist by those names anywhere in `app/lib/` or `app/test/`. Build green.

**Tasks:**
- [x] In `app/lib/services/providers/community_providers.dart`: rename class `SelectedCommunityState` → `CommunitiesState`, class `SelectedCommunityNotifier` → `CommunitiesNotifier`, provider `selectedCommunityProvider` → `communitiesProvider`. Update the `part 'community_providers.freezed.dart';` directive's generated symbols by re-running `npm run generate`. The file itself stays at the same path (already plural, no `git mv` needed).
- [x] Update every import and reference in `app/lib/**` and `app/test/**`. There are no naming conflicts — confirmed via `grep -rn 'communitiesProvider\|CommunitiesState\|CommunitiesNotifier'`.
- [x] Do not change accessor names, method names, or behavior. `selectedCommunity`, `selectCommunity`, `switchTo`, `refreshSelectedCommunity`, etc. stay in place.
- [x] Run `npm run generate`, `flutter analyze`, `flutter test`. Build green.

**Validates:** Build still passes after a pure rename — confirms no callers were depending on the old type-name string anywhere outside of the import system (e.g. reflection, generated code).

### Phase 2: Migrate call sites off the dead/misleading API

**Goal:** Every call site that reads `selectedCommunity`, `effectiveCommunityIds`, `enabledCommunityIdList`, `enabledCommunities`, `isAllCircles`, or calls `selectCommunity` / `switchTo` / `selectAllCircles` / `refreshSelectedCommunity` is replaced with the correct shape — either route-driven `widget.communityId`, or the new `state.communityIds` / `state.communities`, or community-detail navigation.

**Tasks:**
- [x] Add `List<String> get communityIds => communities.map((c) => c.id).toList();` to `CommunitiesState`. Single survivor for what `effectiveCommunityIds` and `enabledCommunityIdList` both used to mean.
- [x] Migrate `state.selectedCommunity` and `state.selectedCommunity?.id` per call site:
  - Where the screen has a `widget.communityId` route arg (`experience_screen.dart`, `gear_content_view.dart`, `request_content_view.dart`, `governance_screen.dart`, etc.), use that.
  - Where the lookup was always going to be `null` (e.g. `gen_experience_view_model.dart:790`), delete the dead branch and confirm with `flutter test`.
  - Where the screen exists outside of a per-community route (`empty_content_state.dart`, `impact_metrics/metrics_screen.dart`, the FCM deep-link in `main.dart`), use route context or remove the conditional entirely.
- [x] Replace `state.effectiveCommunityIds` / `state.enabledCommunityIdList` with `state.communityIds`. ~10 call sites.
- [x] Replace `state.enabledCommunities` with `state.communities`. ~3 call sites.
- [x] Remove `state.isAllCircles` references. Always-true → inline the conditional consequent.
- [x] Replace `.selectCommunity(community)` / `.switchTo(community)` with `NavigationHelpers.push*` (or equivalent) to the community detail screen. `app/lib/main.dart:536` and `app/lib/core/router/app_router.dart:407` are the two real call sites; `governance_screen.dart:407` is a third.
- [x] Drop `.refreshSelectedCommunity()` calls — it's a documented no-op.
- [x] Rename `clearSelection()` → `clear()` and update the (single) logout caller.
- [x] Remove the `selectId` parameter from `reloadCommunities`; migrate the one caller (`app_router.dart:407`) to navigate after the reload.
- [x] Update `community_helper.dart:15-26` `selectedCommunitiesTitle` signature: takes `CommunitiesState`, reads `state.communities` directly. **\[Audit] Consider renaming the helper to `communitiesTitle`** in the same pass — its name still implies a selection. Low-cost; falls out of `flutter analyze`.
- [x] `flutter test` green after this commit. Manual sweep of every touched screen in dev build.

**Validates:** No remaining caller relies on a dead method or a misleading getter. The build still passes with the API surface unchanged but the consumers cleaned up.

### Phase 3: Delete the dead API

**Goal:** `CommunitiesState` exposes only `communities`, `isLoading`, `errorMessage`, and `communityIds`. `CommunitiesNotifier` exposes only `setCommunities`, `setLoading`, `setError`, `clearError`, `retryLoadCommunities`, `reloadCommunities` (no `selectId`), `clear`.

**Tasks:**
- [x] Delete from `CommunitiesState`: `selectedCommunityId` field, `isAllCircles`, `effectiveCommunityIds`, `enabledCommunityIdList`, `selectedCommunity`, `enabledCommunities`.
- [x] Delete from `CommunitiesNotifier`: `selectAllCircles`, `selectCommunity`, `switchTo`, `refreshSelectedCommunity`, `_persistSelection`.
- [x] Retain `_selectedCommunityIdKey` / `_legacyEnabledCommunityIdsKey` and the cleanup-on-`setCommunities` for one more release per the open-questions decision. Follow-up issue **#2045** filed to remove these (and the SharedPreferences cleanup) after 4 weeks of prod soak.
- [x] **\[Audit]** Add a one-line comment on `CommunitiesNotifier` documenting why it intentionally does **not** use `ref.mounted` guards after `await`: the provider is global (no `.autoDispose`), so the late-write race doesn't apply. Prevents a future reviewer from adding the guards out of habit.
- [x] Regenerate Freezed (`npm run generate`).
- [x] `flutter analyze`, `flutter test`, `npm run lint:dart:i18n`, `npm run lint:dart:a11y`, `npm run lint:dart:size`. Build green; the state and notifier classes are now noticeably smaller.

**Validates:** The trap is gone. `flutter analyze` after deletion will fail at any straggler caller, which would be a Phase 2 miss.

## Related Issues

- #1895 — the nav restructure that removed the per-community filter and triggered this debt.
- #1919 — closed polish issue that listed a generic "dead-code cleanup" bullet but did not enumerate this surface.
- #2000 / PR #2021 — the bug this debt directly caused; the migration pattern at `manage_members_screen.dart` is the template for Phase 2's call-site sweep.
- Follow-up (to be filed during Phase 3): remove legacy SharedPreferences cleanup after 4 weeks in prod.
- Follow-up (to be filed during Phase 3): migrate `CommunitiesState.errorMessage: String?` to `UserError? error` per [docs/client/architecture.md](../client/architecture.md) — out of scope for this PR but a known compliance gap.
