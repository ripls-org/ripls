---
issue: 1976
title: Unify community selection + make inviting and community creation more accessible
---

# Issue #1976: Unify community selection + make inviting and community creation more accessible

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1976
**Filed:** 2026-05-16 | **Last activity:** 2026-05-19
**Labels:** none | **Assignees:** briangbrown
**Context docs:** none

> **Audit pass (2026-05-16):** Reviewed against [docs/client/architecture.md](../client/architecture.md), [docs/client/caching.md](../client/caching.md), [docs/client/i18n.md](../client/i18n.md), [docs/client/accessibility.md](../client/accessibility.md), [docs/client/testing.md](../client/testing.md), and the proto/server conventions in [CLAUDE.md](../../CLAUDE.md). Updates appear as **\[Audit]** callouts in each section and as a new "Compliance Requirements" section before the implementation plan. Server docs ([observability](../server/observability.md), [architecture](../server/architecture.md)) were read but do not apply — this issue is client-only with no proto, RPC, or server changes.

> **Re-audit pass (2026-05-19):** Re-reviewed after the bulk of the work shipped in [#1979](https://github.com/Type2Fun-Foundation/ripls/pull/1979). Spot-checked the committed [`CommunitySelectionSheet`](../../app/lib/presentation/widgets/sharing/community_selection_sheet.dart) for raw `GestureDetector` / `InkWell` / `IconButton` / `showModalBottomSheet` / `Duration(...)` / hex `Color(0x…)` — **none found**. 16 `context.l10n.*` references; no hardcoded user-visible strings. Re-read [docs/client/modals.md](../client/modals.md) (not in the original audit set — it codifies the glass-modal shape that Phase 5 must hit) and [docs/client/sharing.md](../client/sharing.md). Findings folded into the Status section and Phase 5 below as **\[Re-audit]** callouts.

## Status (2026-05-19)

Most of the work has already shipped — none of the merged PRs cross-referenced #1976, so the issue still reads as open scope.

**Shipped:**

- **[PR #1979](https://github.com/Type2Fun-Foundation/ripls/pull/1979) — "Initial re-addition of invite user"** (merged 2026-05-16, Thomas) — the big one. Added [`CommunitySelectionSheet`](../../app/lib/presentation/widgets/sharing/community_selection_sheet.dart) with `deferred` / `invite` / `immediate` modes, deleted legacy `community_picker_sheet.dart` and `community_selection_modal.dart`, wired the workshop plus icon → `InviteSheet` (with empty-community fallback to `CommunityCreationModal`), added the typed `CommunitySelectionOpenedEvent` analytics event at [events.dart:561](../../app/lib/core/observability/events.dart#L561), added en/es ARB strings, and ships widget tests at [community_selection_sheet_test.dart](../../app/test/presentation/widgets/sharing/community_selection_sheet_test.dart). Covers **Phase 1, Phase 2, Phase 3, Phase 4** in full.
- **[PR #2022](https://github.com/Type2Fun-Foundation/ripls/pull/2022) — "fix(communities): radio indicator for single-select invite picker (#2001)"** (merged 2026-05-18) — invite-mode rows render a decorative radio instead of a `Switch`; renamed `CommunityToggleTile` → [`CommunitySelectionTile`](../../app/lib/presentation/widgets/sharing/community_selection_tile.dart). Polish on top of Phase 1's invite mode. Cross-references [#2001](https://github.com/Type2Fun-Foundation/ripls/issues/2001), not #1976.
- **[PR #2021](https://github.com/Type2Fun-Foundation/ripls/pull/2021) — "Fix #2000: restore Manage Members invite FAB"** (merged 2026-05-19) — adjacent fallout from the post-#1895 `selectedCommunityProvider` removal; the Manage Members FAB invocation of `InviteSheet` is one of the call sites this issue ultimately rationalizes. Cross-references [#2000](https://github.com/Type2Fun-Foundation/ripls/issues/2000), not #1976.
- **PR #2023 family** (commits `62969236e`, `1b155613c`, `2f91f2cb5`, `c0bd4e566`, `a7bf75bc5` — renamed `selectedCommunityProvider` → `communitiesProvider` and removed dead single-select APIs that fed the legacy pickers). Cleans up after this issue's deletions.

**Outstanding (Phases 5 / 7 / 8 + #1979's own TODO list):**

- **Phase 5 — `CommunityCreationModal` glass migration is NOT done.** The modal still wraps its content in an `AppColors.experienceModalBackground(context)` container at [community_creation_modal.dart:198](../../app/lib/presentation/screens/communities/community_creation_modal.dart#L198) rather than `GlassSheet`. The acceptance criterion "Create community modal renders in the new clear modal design" is unmet.
- **Phase 7 — `CommunityPreviewModal` glass migration is NOT started.** Phase 5 only covers the *creation* modal that kicks off AI generation. Immediately after, `_handleGenerate` opens [`CommunityPreviewModal`](../../app/lib/presentation/screens/communities/community_preview_modal.dart) via `showDialog` — a `Dialog`-style full-screen overlay with bespoke styling. The gear / request / experience preview modals already use the modern glass preview-page design (`BackgroundMediaImage` + `ContentEditableField` + `ContentActionButton` + `ContentEditingMixin`, see [`gear_preview_modal.dart`](../../app/lib/presentation/screens/gear/gear_preview_modal.dart)). Phase 7 brings the community preview onto that same baseline.
- **Phase 8 — "New Community" carousel affordance is NOT started.** Today the only single-purpose path to create a community is the "Create new community" footer inside `CommunitySelectionSheet` — three taps deep from Workshop (plus icon → invite → community picker → footer). Phase 8 adds a dedicated tile at the trailing end of the [Workshop community carousel](../../app/lib/presentation/widgets/workshop/workshop_community_carousel.dart) (circular glass `+` affordance + "New Community" caption) that launches `CommunityCreationModal.show()` in one tap.
- **#1979 TODOs that did not get filed separately:**
  - "Clean up some remaining cream colors on the modal" — pairs with Phase 5 above.
  - "Consolidate the community creation modal — there's still two: one with basic text support and one with text or image." (Worth a follow-up issue if not folded into Phase 5.)
  - "The ACL modal for an item requires a user to use 'Add Community' to also remove existing ones — not intuitive." (Separate UX concern; file as its own issue.)
- **Stale references to deleted files** (not blocking, but should be cleaned up alongside Phase 5):
  - [`app/lib/presentation/widgets/sharing/README.md`](../../app/lib/presentation/widgets/sharing/README.md) — lines 11-16 still describe `community_picker_sheet.dart`, `community_selection_modal.dart`, and `community_toggle_tile.dart`, none of which exist anymore.
  - [`app/lib/presentation/screens/request/request_creation_modal.dart:25`](../../app/lib/presentation/screens/request/request_creation_modal.dart#L25) — comment references `CommunityPickerSheet` (deleted).
  - **\[Re-audit]** [`docs/client/sharing.md`](../client/sharing.md) — lines 45 and 62 still document `CommunitySelectionModal` and `CommunityPickerSheet` as the entry points for "Add Community" and item-creation flows. Both widgets were deleted in [#1979](https://github.com/Type2Fun-Foundation/ripls/pull/1979). Update to reference `CommunitySelectionSheet` with its three modes (`showForDeferred`, `showForInvite`, `showForImmediate`) and the `CommunitySelectionSource` constants.
- **Lint gates** from Phase 6 (`lint:dart:a11y`, `lint:dart:i18n`, `lint:dart:size`, `lint:nav`) — CI is presumed green since the PRs merged, but worth re-running before closing.

**Bottom line:** Phases 1-4 are merged. **Phases 5, 7, and 8 are the remaining work** — Phase 5 swaps the creation surface, Phase 7 swaps the preview/edit surface that immediately follows it, and Phase 8 makes creation reachable in one tap from Workshop. The two non-Phase-5 #1979 TODOs (consolidate-two-create-modals, ACL "Add Community" UX) should be filed as their own issues rather than expanding the scope of #1976.

### Re-audit deltas (2026-05-19)

1. **No regressions in shipped code.** Spot-checked [`CommunitySelectionSheet`](../../app/lib/presentation/widgets/sharing/community_selection_sheet.dart) / [`CommunitySelectionTile`](../../app/lib/presentation/widgets/sharing/community_selection_tile.dart): zero raw `GestureDetector` / `InkWell` / `IconButton` / `showModalBottomSheet` / `Duration(milliseconds:…)` / hex color literals; all user-visible strings flow through `context.l10n.*`; `refreshUserCommunities()` is called at line 183 after community creation as Phase 1 prescribed; the typed `CommunitySelectionOpenedEvent` exists at [events.dart:561](../../app/lib/core/observability/events.dart#L561).
2. **Phase 5 picked up an extra constraint** — [docs/client/modals.md](../client/modals.md) (not in the original audit set) codifies the **standard glass-modal shape**: 12px horizontal inset, 28px top radius, 0.92 max-height fraction, `showAccessibleModal` + `GlassSheet` entry. `ModalHelpers.showStandardModal` is explicitly off-limits for new glass modals. Phase 5 now calls this out.
3. **Drag-handle layout decision added** — image-mode needs `dragHandleOverlay: true` + `padding: EdgeInsets.zero` (the location-picker precedent), text-mode stays on the default. Captured in Phase 5.
4. **Doc drift discovered.** [`docs/client/sharing.md`](../client/sharing.md) still describes `CommunityPickerSheet` (line 62) and `CommunitySelectionModal` (line 45) as the canonical pickers — both deleted in #1979. Added to the Outstanding list above; should be updated alongside Phase 5 to describe `CommunitySelectionSheet` and its three `showFor*` entry points.

## Problem Statement

Three changes that share a theme — making inviting people and managing communities feel more first-class on the workshop screen, and unifying the community-selection UX so it works the same way everywhere:

1. **Workshop plus-group icon redirects to the invite-user flow.** Today the icon at [workshop_screen.dart:66-71](../../app/lib/presentation/screens/workshop/workshop_screen.dart#L66-L71) opens [`CommunityCreationModal`](../../app/lib/presentation/screens/communities/community_creation_modal.dart). The issue moves the primary action to "invite a user" — the existing QR-code-bearing [`InviteSheet`](../../app/lib/presentation/screens/communities/invite_sheet.dart) — and pushes community creation one layer deeper (reached from the new selection modal's "Create new community" entry).
2. **Unify the community selection modal.** Today we ship **two** community pickers with different commit semantics:
   - [`CommunityPickerSheet`](../../app/lib/presentation/widgets/sharing/community_picker_sheet.dart) — used during item *creation*; deferred-commit, returns `List<String>` on confirm.
   - [`CommunitySelectionModal`](../../app/lib/presentation/widgets/sharing/community_selection_modal.dart) — used to change access on *existing* items; immediate-commit, fires API calls on each toggle.
   The invite flow ([`InviteSheet`](../../app/lib/presentation/screens/communities/invite_sheet.dart)) uses a third surface — an inline [`CommunityPickerButton`](../../app/lib/presentation/widgets/sharing/community_picker_button.dart) dropdown. The issue asks all three callers to share the **same** selection modal.
3. **"Create new community" entry point + glass-design migration.** The unified selection modal must include a "Create new community" button that opens the existing [`CommunityCreationModal`](../../app/lib/presentation/screens/communities/community_creation_modal.dart), migrated to the [`GlassSheet`](../../app/lib/presentation/widgets/modal/glass/glass.dart)-based design that [`InviteSheet`](../../app/lib/presentation/screens/communities/invite_sheet.dart), [`CommunityPickerSheet`](../../app/lib/presentation/widgets/sharing/community_picker_sheet.dart), and [`CommunitySelectionModal`](../../app/lib/presentation/widgets/sharing/community_selection_modal.dart) already use.

Tensions worth flagging up front:

- **Commit-semantics mismatch.** Unifying picker-sheet and selection-modal is not just a visual merge — the post-create flow needs to fire API mutations on each toggle, while pre-create needs to defer until confirm. A single widget needs a mode/callback contract that supports both.
- **Workshop "create community" discoverability regresses.** Today, one tap on the workshop plus icon starts community creation. After this change, the same icon starts user invitation, and creating a community requires: tap plus → open invite modal → tap community picker → tap "Create new community". For users who don't yet belong to any community, the invite modal isn't a useful entry point. We need to decide what to show in that empty state.
- **Pre-selected community when opening invite from workshop.** Workshop has a selected-community context ([`selectedCommunityProvider`](../../app/lib/services/providers/community_providers.dart)). The invite modal should respect it — but workshop's "All circles" mode has no single ID; we need a defined fallback (first enabled community? or open the picker first?).

## Context from Comments

The issue has **no comments** yet. All requirements come from the description. This is worth flagging — at minimum, the open questions in this plan should be reviewed by the issue author before implementation begins.

## Codebase Findings

What exists today, anchored to actual files:

- **Workshop top-nav icon** → currently opens `CommunityCreationModal.show(context)` at [workshop_screen.dart:69](../../app/lib/presentation/screens/workshop/workshop_screen.dart#L69).
- **Invite sheet** at [invite_sheet.dart](../../app/lib/presentation/screens/communities/invite_sheet.dart) already accepts a `List<CommunityItem> communities` parameter and renders [`CommunityPickerButton`](../../app/lib/presentation/widgets/sharing/community_picker_button.dart) when more than one is passed. It already uses `GlassSheet` and re-loads the invite link via `_loadInviteLink()` whenever the user switches community. Invocation points today: [manage_members_screen.dart:245-274](../../app/lib/presentation/screens/communities/manage_members_screen.dart#L245-L274) and [community_content_view.dart:605-615](../../app/lib/presentation/screens/communities/community_content_view.dart#L605-L615).
- **Item-creation picker (deferred-commit)** at [community_picker_sheet.dart](../../app/lib/presentation/widgets/sharing/community_picker_sheet.dart). Watches `selectedCommunityProvider.enabledCommunities`; returns selected IDs via `Navigator.pop`. Used by `gear_preview_modal` and `experience_preview_modal`.
- **Access-edit picker (immediate-commit)** at [community_selection_modal.dart](../../app/lib/presentation/widgets/sharing/community_selection_modal.dart). Branches on `itemType` ('gear' / 'request' / 'experience'), reads from `gearSharingProvider` / `requestSharingProvider` / `experienceSharingNotifierProvider`, and tiles use [`CommunityListItem`](../../app/lib/presentation/widgets/sharing/community_list_item.dart) that fire API calls on toggle.
- **Create-community modal** at [community_creation_modal.dart](../../app/lib/presentation/screens/communities/community_creation_modal.dart) — uses `AppColors.experienceModalBackground` directly and a custom bottom-sheet rounded container instead of `GlassSheet`. The widget doc says "Clean modal design matching RSVP modal (no header, warm beige background)" — close to glass design but **not** the same surface. The other three modals (Invite/PickerSheet/SelectionModal) all wrap in `GlassSheet`.
- **Glass design primitives** at [presentation/widgets/modal/glass/](../../app/lib/presentation/widgets/modal/glass/): `GlassSheet`, `GlassModalHeader`, `GlassFooterButtons`, `GlassFieldLabel`, etc. Modal entry via [`showAccessibleModal`](../../app/lib/presentation/widgets/accessibility/show_accessible_modal.dart) with `barrierColor: AppColors.modalBackdrop`.
- **CommunityRepository** at [community_repository.dart](../../app/lib/data/repositories/community_repository.dart) — uses [CacheService](../../app/lib/data/cache/cache_service.dart) namespace `'community'`. After creating a community we must call `refreshUserCommunities()` (line 110) so any open picker / workshop carousel sees the new community without restart. `selectedCommunityProvider` is downstream of this repository.
- **Typed analytics events** at [core/observability/events.dart](../../app/lib/core/observability/events.dart) — `CommunityInviteSharedEvent` (L530) and `CommunityInviteCopiedEvent` (L543) follow the `AnalyticsEvent` interface. New events ride the same `observabilityServiceProvider.logAnalyticsEvent(...)` API.
- **`CommunityPickerButton` is referenced from two places**, not one: [invite_sheet.dart:233](../../app/lib/presentation/screens/communities/invite_sheet.dart#L233) and [item_share_sheet.dart:313](../../app/lib/presentation/widgets/sharing/item_share_sheet.dart#L313). Any deletion plan must address both call sites or leave the widget in place.

## Potential Solutions

### Option A: One unified `CommunitySelectionSheet` widget with a "mode" contract
- **Approach:** Build a single new `CommunitySelectionSheet` widget in [presentation/widgets/sharing/](../../app/lib/presentation/widgets/sharing/) that supports three usage modes via a strategy/callback contract:
  - **Deferred mode** — initial selection in, `onConfirm(List<String> ids)` callback, pops result on confirm. Replaces `CommunityPickerSheet`.
  - **Immediate mode** — `itemId` + `itemType` + sharing-provider hookup; toggles call API immediately. Replaces `CommunitySelectionModal`.
  - **Invite mode** — single-select (radio, not checkbox); fires `onCommunityChanged(String id)`. Used by `InviteSheet` instead of the inline `CommunityPickerButton`.
  All three render the same `GlassSheet` + tile list, with a shared **"Create new community"** trailing entry that pushes the (newly glass-migrated) `CommunityCreationModal`. Delete the old `CommunityPickerSheet` and `CommunitySelectionModal` after migration.
- **Pros:** Real unification — one widget, one bug surface, one place to evolve. Matches the spirit of the issue exactly. Single visual baseline.
- **Cons:** The widest blast radius. Touches gear/experience/request preview modals + access-sheet helper + invite sheet. Single/multi-select divergence (invite is single-select; the others are multi-select) is real and the widget must carry a mode flag. ViewModel wiring for immediate-commit cases stays complex.
- **Effort:** Large.

### Option B: Keep both modals but extract a shared inner `CommunitySelectionList` widget
- **Approach:** Leave [`CommunityPickerSheet`](../../app/lib/presentation/widgets/sharing/community_picker_sheet.dart) and [`CommunitySelectionModal`](../../app/lib/presentation/widgets/sharing/community_selection_modal.dart) as separate entry points but extract the shared list-of-toggles UI (the section between header and footer) into a reusable widget. Add the "Create new community" button at the bottom of both. Migrate `CommunityCreationModal` to `GlassSheet`. For the invite modal, replace the inline `CommunityPickerButton` with a tap-to-open `CommunityPickerSheet` call (single-select variant).
- **Pros:** Smaller blast radius. Visual unification + shared tile/empty-state code without touching the two providers' state machines. Lower regression risk for in-flight access changes (gear/request/experience all keep current behavior).
- **Cons:** Falls short of the issue's literal ask ("the **same** community selection modal"). Future drift between the two surfaces is likely. Adds a new single-select mode to `CommunityPickerSheet` (currently multi-select only) which is awkward.
- **Effort:** Medium.

### Option C: Build a thin shell + provider-driven content
- **Approach:** Introduce `CommunitySelectionSheet` as a thin shell that handles glass chrome, header, "Create new community" footer, and barrier behavior. The content is supplied by the caller via a `body` slot. Existing picker-sheet and selection-modal become thin wrappers that hand their tile content to the shell. Invite flow opens the shell with single-select content.
- **Pros:** Visual unification is real and centralized. Each caller keeps its own commit semantics without a mode enum proliferation. Adding "Create new community" lands in one place.
- **Cons:** Looks more like consistency-by-composition than the "single modal" the issue describes. Slot-based contracts can be harder for new readers to follow than a typed mode enum.
- **Effort:** Medium.

## Recommendation

**Option A** — the unified `CommunitySelectionSheet` with a typed mode contract — is the right end state. The issue is explicitly asking for one selection modal, and the upside of having one source of truth for community-selection UX is large enough to justify the migration cost.

That said, ship Option A **incrementally** so we don't try to flip every caller in one PR:

1. Build the unified widget with `deferred` and `invite` modes first (these are the new uses and the simpler ones).
2. Migrate `CommunityPickerSheet`'s callers to the deferred mode; delete `CommunityPickerSheet`.
3. Wire workshop plus icon → InviteSheet → unified picker (invite mode).
4. Add `immediate` mode + migrate `CommunitySelectionModal` callers; delete `CommunitySelectionModal`.
5. Migrate `CommunityCreationModal` to `GlassSheet` and wire the "Create new community" entry point.

If during Phase 1–2 the `immediate` mode looks like it's going to fight the providers more than expected, fall back to Option B's "shared inner list widget" for the access-edit case while keeping the unified widget for the other two — and document that retreat in this file.

## Open Questions

- **What does the workshop plus icon do for a user with zero communities?** — Recommendation: open `CommunityCreationModal` directly (current behavior). Inviting someone to nothing makes no sense, so detect `enabledCommunities.isEmpty` in the tap handler and short-circuit to community creation. After they create one, they can re-tap to invite.
- **Which community is pre-selected when invite opens from workshop?** — Recommendation: if workshop is filtered to a single community via `selectedCommunityProvider`, pre-select that. If workshop is in "All circles" mode, pre-select the user's first (most recently active) enabled community and rely on the community picker dropdown for switching. Don't open the picker eagerly — the QR code should be visible immediately.
- **Single-select vs multi-select in invite mode** — Recommendation: invite mode is single-select (one QR code at a time). The unified widget should expose a `selectionMode: { single, multi }` enum independent of commit semantics.
- **Where does the "Create new community" button live in the modal?** — Recommendation: a persistent footer row above the primary action button (sticky, not in the scrolling tile list), so it's reachable regardless of how many communities you have. Use `GlassInlineAction` + a leading `+` icon for consistency with other modal action affordances.
- **Should the existing `CommunityPickerButton` inline dropdown in `InviteSheet` be removed?** — Recommendation: yes, replace it with a tap-to-open call into the unified modal. Reduces surface area, removes a third selection UI, and makes "Create new community" reachable from inside the invite flow.
- **Telemetry** — Recommendation: emit a new `community_selection_opened` analytics event with `source: { invite | gear_create | gear_access | experience_create | experience_access | request_create | request_access | workshop_create }` so we can verify the unification is actually being used.
- **Localization** — Recommendation: keep `communityPickerTitle` / `communityPickerSubtitle` ARB keys and add new ones for the "Create new community" affordance and the invite-mode title. Audit existing keys before adding new ones; some may be reusable.

## Compliance Requirements

**\[Audit]** — derived from `docs/client/*`, `CLAUDE.md`, and the proto/accessibility/i18n lint scripts. These apply across every phase below.

### Architecture (MVVM + Riverpod)
- **Widgets never call services or repositories directly.** Any state the unified sheet needs comes from existing providers (`selectedCommunityProvider`, `gearSharingProvider`, `requestSharingProvider`, `experienceSharingNotifierProvider`). If a new ViewModel is required (e.g. to combine "create community → pre-select it" into one transactional flow), it lives in [presentation/viewmodels/](../../app/lib/presentation/viewmodels/) as a `Notifier` with Freezed state — never as a `StatefulWidget`-managed `Map`.
- **No new manual cache maps.** `CommunityPickerSheet` today uses a local `Set<String> _selected` for *deferred-commit pending UI state*, which is fine (it is ephemeral selection state, not cached data). Do not introduce a cross-screen cache outside `CommunityRepository`.
- **Use `Notifier.autoDispose` for screen-scoped state** if a new ViewModel is added. Guard `state =` assignments after any `await` with `if (!ref.mounted) return;` (or migrate via [`SafeNotifierMixin`](../../app/lib/core/utils/safe_notifier.dart)). See [client/architecture.md "Async Safety in Notifiers"](../client/architecture.md).
- **No "controllers."** New helpers go under `core/utils/` or `presentation/widgets/`; not `*Controller`.

### Caching invalidation
- **After `CommunityCreationModal` succeeds from inside the unified picker, call `CommunityRepository.refreshUserCommunities()`** (currently at [community_repository.dart:110](../../app/lib/data/repositories/community_repository.dart#L110)) so the picker, workshop carousel, and other downstream surfaces see the new community without restart. This is the pull-to-refresh-style invalidation pattern from [client/caching.md "Pattern 2: Mutations with Auto-Invalidation"](../client/caching.md).
- **Do not access `StashCacheManager` directly** from the new widget. All invalidation routes through `CommunityRepository`'s `refresh*()` methods.

### i18n
- **Every user-visible string in the new widget comes from `context.l10n.*`.** Raw `'Select Communities'` and `'Choose which of your Ripls communities to share this with.'` literals already exist in [`CommunitySelectionModal`](../../app/lib/presentation/widgets/sharing/community_selection_modal.dart#L54-L66) and **must be migrated to ARB keys as part of this work** (current code violates `lint:dart:i18n`).
- **Add every new key to both `app_en.arb` AND `app_es.arb`** with the appropriate `@key` metadata. Strings under the `a11y` prefix go on `tooltip:` / `semanticsLabel:` / `Semantics(...)` arguments.
- **ViewModels expose typed state, not localized text.** If a new sharing ViewModel emits errors, expose `UserError? error` and let widgets call `RpcErrorHandler.localize(state.error!, context.l10n)`. See [client/i18n.md](../client/i18n.md) and [`app/lib/core/errors/README.md`](../../app/lib/core/errors/README.md).

### Accessibility
- **Use the existing primitives** ([client/accessibility.md table](../client/accessibility.md)) — no raw `GestureDetector` / `InkWell` / `IconButton` / `showModalBottomSheet` / `Duration(...)` inside the new code:
  - `Tappable` for any tap surface (e.g. the "Create new community" row).
  - `IconAction` for icon-only buttons (already used at [workshop_screen.dart:66](../../app/lib/presentation/screens/workshop/workshop_screen.dart#L66); preserve when changing `semanticsLabel`).
  - `showAccessibleModal` (not `showModalBottomSheet`) for opening the unified sheet.
  - `accessibleDuration(context, ...)` for any animation `duration:` in the new widget.
  - `ModalHeader` (or `Semantics(header: true)`) on the modal title to mark it as a heading.
  - For toggle-style tiles, use `Toggle` and set `selected:` on `Semantics` rather than embedding "selected" / "not selected" in the label text.
- **`a11y*` prefix** for every label key. The plan's `a11yWorkshopInvitePeople` is the right shape; also reserve `a11yCommunitySelectionCreateNew` (or similar) for the "Create new community" affordance.
- **Run `npm run lint:dart:a11y` before merging** — the [allowlist](../../scripts/a11y_allowlist.txt) is empty and CI enforces no new violations.

### Testing
- **ViewModel-first.** If any sharing ViewModel (`gearSharingProvider` / `requestSharingProvider` / `experienceSharingNotifierProvider`) changes shape, add or update tests under [`app/test/presentation/viewmodels/`](../../app/test/presentation/viewmodels/) using `ProviderContainer` + `Mockito`. Coverage target: 90%+ for ViewModels (per [client/testing.md](../client/testing.md)).
- **Widget tests for `CommunitySelectionSheet`** belong under [`app/test/presentation/widgets/sharing/`](../../app/test/presentation/widgets/sharing/) and should mock `CommunityRepository` + the sharing notifiers. Test the contract (deferred returns IDs on confirm; invite fires callback; immediate triggers provider mutations; "Create new community" round-trips into the new community being auto-selected).
- **Disposal tests** are required for any new `autoDispose` `Notifier` (see [client/architecture.md "Disposal Testing"](../client/architecture.md)).
- **`MaterialApp` test wrapper** must include `localizationsDelegates` and `supportedLocales` — see [`app/test/helpers/l10n_helpers.dart`](../../app/test/helpers/l10n_helpers.dart).
- **Run command:** `flutter test --reporter expanded` (the project mandates `--reporter expanded`, not `compact`).

### Telemetry
- **New events follow the `AnalyticsEvent` typed class pattern** at [core/observability/events.dart](../../app/lib/core/observability/events.dart) — e.g. `class CommunitySelectionOpenedEvent implements AnalyticsEvent { final String source; ... }`. **No raw event-name strings** passed to `logAnalyticsEvent`.
- **`source` enum** should match the call sites: `invite`, `gearCreate`, `gearAccess`, `experienceCreate`, `experienceAccess`, `requestCreate`, `requestAccess`, `workshopInvite`.

### Theming
- **No `Colors.white`, `Colors.grey[*]`, or hex `Color(0xFF...)` literals** in new code. Use `AppColors.cardBackground(context)`, `AppColors.modalTextPrimary`, etc. (per [client/architecture.md "Colors and Theming"](../client/architecture.md) and the user's standing memory: reuse existing theme tokens; don't invent per-section coral/cream constants).

### Navigation
- The unified sheet is a modal, not a route. `showAccessibleModal` is correct. If "Create new community" succeeds and we want to deep-link to the new community detail screen, use `NavigationHelpers.pushScreen(useRootNavigator: true, ...)` (per [client/architecture.md "Pushing screens"](../client/architecture.md)) — never `Navigator.push(MaterialPageRoute(...))` or `context.push()`.

### File size
- The new widget should land at ~250–400 lines. If it grows past 1000 lines (the `npm run lint:dart:size` gate, [scripts/check_file_size.js](../../scripts/check_file_size.js)), extract `CommunitySelectionListTile`, `CommunitySelectionFooter`, etc. into siblings rather than adding the `// dart-line-count-allow` hatch.

## Implementation Plan

### Phase 1: New unified `CommunitySelectionSheet` (deferred + invite modes) — ✅ SHIPPED ([#1979](https://github.com/Type2Fun-Foundation/ripls/pull/1979))
**Goal:** Have a single GlassSheet-based community picker that supports the two new uses (deferred-commit item creation and single-select invite) without breaking any existing flow.
**Tasks:**
- [ ] Create `app/lib/presentation/widgets/sharing/community_selection_sheet.dart` with a `mode: { deferred, invite }` enum, a `selectionMode: { single, multi }` enum, an `initialSelection`, and an `onResult` callback. Wraps in `GlassSheet`; sources communities from `selectedCommunityProvider.enabledCommunities`. **\[Audit]** Open via `showAccessibleModal` (not `showModalBottomSheet`); use `Tappable` for any tap surfaces; pull every label from `context.l10n`; use `AppColors.*` helpers (no hex / `Colors.white`).
- [ ] Add a persistent "Create new community" affordance in the footer that calls `CommunityCreationModal.show()` and, on success, **\[Audit] calls `ref.read(communityRepositoryProvider).refreshUserCommunities()` before auto-selecting** the new community — otherwise downstream surfaces (workshop carousel, other open pickers) will still display the stale list until next TTL. Cache key namespace is `community:*`.
- [ ] **\[Audit]** Define a typed `CommunitySelectionOpenedEvent implements AnalyticsEvent` in [core/observability/events.dart](../../app/lib/core/observability/events.dart) with a `source` field (one of `invite | gearCreate | gearAccess | experienceCreate | experienceAccess | requestCreate | requestAccess | workshopInvite`). Emit from `show()` via `ref.read(observabilityServiceProvider).logAnalyticsEvent(...)`. No raw event-name strings.
- [ ] Add ARB entries to **both** `app_en.arb` and `app_es.arb`: at minimum `communitySelectionCreateNew`, `communitySelectionTitleInvite`, `communitySelectionSubtitleInvite`, `a11yCommunitySelectionCreateNew`, `a11yCommunitySelectionClose`. Re-use existing `communityPickerTitle` / `communityPickerSubtitle` / `communityPickerEmpty` when the meaning matches.
- [ ] Add unit/widget tests under `app/test/presentation/widgets/sharing/community_selection_sheet_test.dart` covering: deferred-multi confirm, deferred-multi cancel, invite-single change callback, empty-state, create-new-community auto-selection + `refreshUserCommunities` invocation. **\[Audit]** Use `ProviderContainer` overrides + `@GenerateMocks([CommunityRepository, ...])`; wrap test `MaterialApp` with `localizationsDelegates` + `supportedLocales` per [`app/test/helpers/l10n_helpers.dart`](../../app/test/helpers/l10n_helpers.dart). Run with `flutter test --reporter expanded`.
- [ ] Wire `selectedCommunityProvider` reads to handle the "All circles" sentinel correctly.
**Validates:** The widget contract is sound before any callers migrate. New tests provide a regression net.

### Phase 2: Migrate item-creation flows to the new sheet — ✅ SHIPPED ([#1979](https://github.com/Type2Fun-Foundation/ripls/pull/1979))
**Goal:** Replace `CommunityPickerSheet` callers (`gear_preview_modal`, `experience_preview_modal`, and the request-create path) with the new sheet in `deferred` + `multi` mode.
**Tasks:**
- [ ] Update each caller of `CommunityPickerSheet.show()` to call `CommunitySelectionSheet.show(mode: deferred, selectionMode: multi, ...)`.
- [ ] Verify selection round-trips correctly through each preview modal's existing state.
- [ ] Delete `community_picker_sheet.dart` and its test (if any).
- [ ] Run `flutter test --reporter expanded` and `flutter analyze`.
**Validates:** The deferred mode works in production-shaped contexts; the unified widget can fully replace one of the three legacy surfaces.

### Phase 3: Workshop plus icon → invite + invite-modal refactor — ✅ SHIPPED ([#1979](https://github.com/Type2Fun-Foundation/ripls/pull/1979); radio polish in [#2022](https://github.com/Type2Fun-Foundation/ripls/pull/2022))
**Goal:** Replace the workshop plus icon's destination and consolidate `InviteSheet`'s community switcher onto the unified sheet.
**Tasks:**
- [ ] In `workshop_screen.dart`, change the `IconAction.onPressed` to:
  - If `enabledCommunities.isEmpty` → `CommunityCreationModal.show()` (preserved fallback).
  - Otherwise → open `InviteSheet` with the workshop-selected community pre-filled (or first enabled community in "All circles" mode) and pass `communities: enabledCommunities`.
- [ ] Update `IconAction.semanticsLabel` from `a11yWorkshopCreateCommunity` to a new `a11yWorkshopInvitePeople` (and add the ARB entry).
- [ ] Refactor `InviteSheet._buildCommunitySelector` to push `CommunitySelectionSheet.show(mode: invite, selectionMode: single, initialSelection: [_selectedCommunityId])` on tap instead of rendering `CommunityPickerButton` inline. On result, call `_selectCommunity` and refresh the QR.
- [ ] **\[Audit]** `CommunityPickerButton` is referenced from **two** call sites: [invite_sheet.dart:233](../../app/lib/presentation/screens/communities/invite_sheet.dart#L233) (removed in this phase) and [item_share_sheet.dart:313](../../app/lib/presentation/widgets/sharing/item_share_sheet.dart#L313) (still active). Decision: **keep** `community_picker_button.dart` until `item_share_sheet.dart` is also migrated, then delete in a follow-up. Do not add a `// removed` comment or other back-compat shim — per [CLAUDE.md](../../CLAUDE.md), if the widget is still used, leave it as-is.
- [ ] Add a widget test verifying that the workshop plus icon opens `InviteSheet` (and that empty-community state opens `CommunityCreationModal` instead).
**Validates:** The visible UX change ships independently of the access-edit refactor — and reviewers can poke the new flow end-to-end.

### Phase 4: Migrate access-edit flow + delete legacy modal — ✅ SHIPPED ([#1979](https://github.com/Type2Fun-Foundation/ripls/pull/1979))
**Goal:** Replace `CommunitySelectionModal` callers with the unified sheet in `immediate` mode.
**Tasks:**
- [ ] Add `immediate` mode to `CommunitySelectionSheet`. Mode takes an `itemId` + `itemType` + the existing sharing-provider hookup, and uses [`CommunityListItem`](../../app/lib/presentation/widgets/sharing/community_list_item.dart)'s on-tap mutation behavior.
- [ ] Replace `CommunitySelectionModal` invocations in the gear-access helper ([`GearAccessSheetHelper`](../../app/lib/presentation/widgets/sharing/gear_access_sheet_helper.dart) — verify path during impl) and the corresponding request/experience access helpers.
- [ ] Delete `community_selection_modal.dart`.
- [ ] Verify error states, empty states, and loading states render identically to the old modal.
**Validates:** All three legacy selection surfaces are gone; one widget owns the UX.

### Phase 5: Migrate `CommunityCreationModal` to glass design — ✅ DONE (this branch)
**Goal:** Bring the community-creation entry point onto the same visual baseline as the rest of the family.
**Tasks:**
- [ ] Refactor `CommunityCreationModal` to wrap its content in `GlassSheet` (replace the bespoke `AppColors.experienceModalBackground` container).
- [ ] **\[Audit risk]** Image mode currently fills the screen edge-to-edge with zero padding and `BorderRadius.zero` ([community_creation_modal.dart:152-170](../../app/lib/presentation/screens/communities/community_creation_modal.dart#L152-L170)). `GlassSheet` is a rounded bottom-sheet container — naively wrapping the camera viewport will break full-screen behavior. **Preferred fix:** extend `GlassSheet` with a `fullscreen: bool` (or `variant: { sheet, fullscreen }`) parameter so all glass modals share a single API. **Fallback:** branch on `inputMode` and render the bespoke fullscreen container only in image mode, keeping text mode on `GlassSheet`. Do not fork the glass-design widgets per-feature.
- [ ] **\[Re-audit]** Hit the **standard glass-modal shape** mandated by [docs/client/modals.md](../client/modals.md): `ModalTheme.sheetHorizontalInset` (12px), `ModalTheme.sheetTopRadius` (28px), and `ModalTheme.sheetMaxHeightFraction` (0.92). Do **not** thread this through `ModalHelpers.showStandardModal` — that path imposes a 20px ClipRRect and 75/90% height that conflicts with the canonical shape. `showAccessibleModal` + `GlassSheet` is the only entry pattern.
- [ ] **\[Re-audit]** Pick the right drag-handle layout: image-mode (full-bleed camera viewport) → `dragHandleOverlay: true` + `padding: EdgeInsets.zero`, matching the location-picker precedent in [modals.md "Drag handle"](../client/modals.md). Text-mode → default `dragHandleOverlay: false`.
- [ ] **\[Audit]** Replace any raw `Duration(milliseconds: 300)` on `AnimatedContainer` (currently at [community_creation_modal.dart:162](../../app/lib/presentation/screens/communities/community_creation_modal.dart#L162) — note: already uses `accessibleDuration`, preserve this when refactoring).
- [ ] **\[Audit]** Migrate the SnackBar at [community_creation_modal.dart:100-102](../../app/lib/presentation/screens/communities/community_creation_modal.dart#L100-L102) to `ToastHelper.showError` to match the invite flow and avoid hardcoded strings.
- [ ] **\[Audit]** Audit the modal for raw string literals (e.g. `'Briefly describe your community...'` at line 252, `'Capture something about your community'` at line 191) and migrate to `context.l10n.*` with paired en/es ARB entries. The `lint:dart:i18n` check will catch what's missed.
- [ ] Update the existing usages from `workshop_screen.dart` (fallback) and from `CommunitySelectionSheet`'s "Create new community" button.
- [ ] Add a golden / widget test capturing the new design.
**Validates:** The "Create new community" button from inside the unified picker lands on a visually consistent modal.

### Phase 6: Telemetry + polish — 🟡 PARTIAL (events landed in #1979; lint re-run + stale-ref cleanup remain)
**Goal:** Make sure we can observe the unification working and catch regressions.
**Tasks:**
- [ ] **\[Audit]** Emit the `CommunitySelectionOpenedEvent` typed analytics event (defined in Phase 1) from every entry point, with the correct `source` value. No raw event-name strings.
- [ ] Run `npm run build` (Go-side: should be a no-op for app-only changes, but verify nothing in `proto/` is touched).
- [ ] **\[Audit]** Run the full Flutter lint suite, not just size:
  - `npm run lint:dart:size` (1000-line gate)
  - `npm run lint:dart:a11y` (a11y rules; allowlist must not grow — currently empty)
  - `npm run lint:dart:i18n` (no hardcoded user-visible strings)
  - `npm run lint:nav` (no raw `Navigator.push(MaterialPageRoute(...))` or `context.push()`)
- [ ] Run `flutter analyze` and `flutter test --reporter expanded`.
- [ ] **\[Audit]** Confirm `ProviderContainer.dispose()` is called in every new test's `tearDown` and that `Mockito` mocks are `reset()` between tests (per [client/testing.md "Common Testing Pitfalls"](../client/testing.md)).
- [ ] Self-review the invite flow on Android + iOS via [ripls-app-logs](../../.claude/skills/ripls-app-logs/) to confirm QR refresh works on community switch and "Create new community" round-trips cleanly. **\[Audit]** Also smoke-test with VoiceOver/TalkBack: the new sheet's title is announced as a heading, the "Create new community" tap is reachable, and the close button has a meaningful label.
**Validates:** End-to-end manual confirmation + lint/test gates pass before shipping.

### Phase 7: Migrate `CommunityPreviewModal` to the glass preview-page design — 🟡 REMAINING
**Goal:** Bring the AI-generated community preview/edit step onto the same modern glass surface used by [`GearPreviewModal`](../../app/lib/presentation/screens/gear/gear_preview_modal.dart), [`RequestPreviewModal`](../../app/lib/presentation/screens/request/request_preview_modal.dart), and [`ExperiencePreviewModal`](../../app/lib/presentation/screens/experience/experience_preview_modal.dart) — full-bleed hero media background, glass content card, `ContentEditableField`s for name/description, `ContentActionButton` primary CTA, error banner, streaming skeleton while AI fields populate. The creation surface (Phase 5) and the preview surface (Phase 7) must land together for the create-community flow to feel consistent end-to-end.

**Tasks:**
- [ ] Read [`gear_preview_modal.dart`](../../app/lib/presentation/screens/gear/gear_preview_modal.dart) end-to-end as the reference. Note the structure: `BackgroundMediaImage` (hero) → glass `Card` content with `ContentEditableField` (title) + `ContentEditableField` (description) + `MediaPickerButton` (replace media) + `ContentActionButton` (primary "Save & Share") + `ContentErrorBanner`. Uses [`ContentEditingMixin`](../../app/lib/presentation/widgets/content/content_editing_mixin.dart) for the editing-buffer state machine.
- [ ] Rewrite [`community_preview_modal.dart`](../../app/lib/presentation/screens/communities/community_preview_modal.dart) to follow that structure. Replace the bespoke editing-buffer fields (`_editingName`, `_editingDescription`) with `ContentEditingMixin`. Replace any opaque container backgrounds with the glass widget set.
- [ ] **\[Audit]** Use [`showAccessibleModal`](../../app/lib/presentation/widgets/accessibility/show_accessible_modal.dart) (not `showDialog`) so the preview surface is consistent with the rest of the modal family and routes properly through the accessibility focus restoration. Wire from `CommunityCreationModal._handleGenerate` (currently calls `showDialog<String>(builder: CommunityPreviewModal())`).
- [ ] **\[Audit]** Keep `GenCommunityViewModel` streaming wiring intact — the modal should still pick up `name` / `description` / `mediaIds` from `genCommunityProvider` as streaming events arrive. The race-seed fix in [`community_preview_modal.dart:43-56`](../../app/lib/presentation/screens/communities/community_preview_modal.dart#L43-L56) must survive the rewrite.
- [ ] **\[Audit]** Migrate any raw user-visible strings to ARB (en + es). Reuse existing keys where possible (e.g. the gear preview's "Save & Share" → community equivalent if such a key exists, otherwise add `communityPreview*`).
- [ ] **\[Audit]** No new raw `GestureDetector` / `InkWell` / `IconButton` / `showModalBottomSheet` / `Duration(...)` / hex `Color(0x…)` literals — keep the a11y allowlist at zero. Every label flows through `context.l10n.*`. Run `npm run lint:dart:{a11y,i18n,size,l10n-parity}` before merge.
- [ ] **\[Audit]** Add a widget test under [`app/test/presentation/screens/communities/community_preview_modal_test.dart`](../../app/test/presentation/screens/communities/community_preview_modal_test.dart): seed `genCommunityProvider` with streaming AI output, mount the modal, verify the editable name + description fields populate, the primary action is enabled when fields are non-empty, and tapping it invokes the create RPC. Wrap `MaterialApp` with `localizationsDelegates` + `supportedLocales` per [`app/test/helpers/l10n_helpers.dart`](../../app/test/helpers/l10n_helpers.dart).
- [ ] Manual smoke on Android + iOS: open the modal from (a) workshop plus icon with zero communities (creation fallback) and (b) the "Create new community" footer inside `CommunitySelectionSheet`. Type a prompt, hit Generate, verify the preview opens on glass and reflects the streaming AI output. Edit name/description, hit Save & Share, confirm the new community lands in `communitiesProvider` (carousel updates without restart).

**Risks / open questions to resolve during implementation:**
- **Two community-creation paths still exist.** #1979's TODO list calls out "there's still two: one with basic text support and one with text or image" — Phase 7 should either consolidate to one (preferred) or document why two exist after the migration. If consolidation is in scope, the second creation modal should also adopt the glass design here; if not, file a follow-up.
- **`ContentEditingMixin` was built for "title + description" content types.** Community has `name` + `description`. Confirm the mixin's contract maps cleanly (name → title slot, description → description slot) without forcing a "gear-shaped" data model on community state.
- **Streaming skeleton states.** The gear preview shows `StreamingSkeleton` while AI fields are populating. Make sure `genCommunityProvider` exposes an equivalent loading signal — if not, add one as part of this phase rather than landing a half-streaming preview.

**Validates:** End-to-end create-community flow renders consistently on glass: creation modal (Phase 5) → preview modal (Phase 7) → community lands in `communitiesProvider` and downstream surfaces. Closes the "Create community modal renders in the new clear modal design" acceptance criterion in full.

### Phase 8: "New Community" carousel affordance on Workshop — 🟡 REMAINING
**Goal:** Make community creation reachable in one tap from the Workshop screen. After Phase 1-7 the only entry point to creation is the "Create new community" footer inside `CommunitySelectionSheet` (reached from the workshop plus icon → invite → community picker → footer), which is three taps deep for users who aren't trying to invite anyone. Phase 8 adds a dedicated "New Community" tile at the trailing end of the [Workshop community carousel](../../app/lib/presentation/widgets/workshop/workshop_community_carousel.dart) that launches `CommunityCreationModal.show()` directly — bypassing the invite flow entirely.

**Tasks:**
- [ ] In [`workshop_community_carousel.dart`](../../app/lib/presentation/widgets/workshop/workshop_community_carousel.dart), append a trailing "+ New Community" tile after the last community in the `ListView.builder` (`itemBuilder` at line 122). The tile mirrors the existing community-avatar tile dimensions for visual rhythm (same width, same vertical alignment) but renders:
  - A circular glass surface (or theme-token "add" affordance) with an icon — `Icons.add_circle_outline` or `Icons.group_add_outlined` (decide during design; the latter is already used at [workshop_screen.dart:124](../../app/lib/presentation/screens/workshop/workshop_screen.dart#L124) for the invite icon, which would *collide visually with the plus-group icon* — `Icons.add_circle_outline` is the safer pick).
  - A "New Community" caption below the icon, matching the typography of the community-name labels under existing avatars.
- [ ] Tap handler calls `CommunityCreationModal.show(context)` directly (not through `CommunitySelectionSheet`). On success — `CommunityCreationModal.show()` resolves with the new community ID — pin the new community in the carousel via `ref.read(workshopCommunityProvider.notifier).set(newId)` so the user lands on it immediately.
- [ ] **\[Audit]** Wrap the tile in `Tappable` (not raw `GestureDetector` / `InkWell` / `IconButton`) with `semanticsLabel: context.l10n.a11yWorkshopNewCommunity`. Decorative icon → `excludeChildSemantics: true`.
- [ ] **\[Audit]** Add ARB keys to **both** `app_en.arb` and `app_es.arb`:
  - `workshopCarouselNewCommunity` — the visible "New Community" label.
  - `a11yWorkshopNewCommunity` — the accessibility tap-label (e.g. "Create a new community").
- [ ] **\[Audit]** Emit a typed analytics event when the tile is tapped. Either reuse the existing `CommunitySelectionOpenedEvent` with a new `source: workshopCarousel` value (preferred — minimizes event-name proliferation) or add a dedicated `CommunityCreationLaunchedEvent` if the source set proves too coarse for the desired funnel analysis.
- [ ] Add a widget test under [`app/test/presentation/widgets/workshop/`](../../app/test/presentation/widgets/workshop/) verifying the tile renders after the last community, the tap fires `CommunityCreationModal.show()`, and the analytics event lands with the right source value.
- [ ] Manual smoke on Android + iOS: scroll the carousel to the end, confirm the "New Community" tile renders with the correct icon + label, tap it, confirm the creation modal opens (text mode), generate a community, confirm it appears in the carousel and is auto-pinned.

**Open questions to resolve during implementation:**
- **Icon choice.** `Icons.add_circle_outline` vs `Icons.group_add_outlined` vs a custom glass `+` chip. Recommend `Icons.add_circle_outline` to avoid colliding with the plus-group invite icon next to the title on the same screen ([workshop_screen.dart:122-127](../../app/lib/presentation/screens/workshop/workshop_screen.dart#L122-L127)).
- **Position.** Trailing end of the carousel (after the last community) keeps the tile out of the user's primary scan path but discoverable on scroll. Alternative is *leading* end (most visible) — but that pushes the user's own communities right, hurting glanceability for the common case. Recommend trailing.
- **Empty-state collision.** When the user has zero communities, the workshop plus icon already short-circuits to `CommunityCreationModal.show()` ([workshop_screen.dart:65-68](../../app/lib/presentation/screens/workshop/workshop_screen.dart#L65-L68)). With Phase 8 the carousel itself is empty in that case — render *only* the "+ New Community" tile so the carousel never appears blank. Confirm that's actually the visual we want, or hide the carousel entirely when communities is empty.
- **Carousel "Everything" tile.** Today the leading tile is "Everything" (the cross-community aggregate). Decide whether the "+ New Community" tile sits *after* per-community tiles only or after "Everything" too — recommend after per-community (so it's clearly a creation affordance, not part of the filter set).

**Risks / open questions:**
- **Discoverability vs visual noise.** A persistent tile at the end of the carousel takes up some glanceable real estate that would otherwise display the next-shown community on small screens. Worth a manual scan on iPhone SE to confirm the trade-off is acceptable.
- **Empty-community fallback divergence.** The workshop plus icon's "no communities → CreationModal" fallback ([workshop_screen.dart:65-68](../../app/lib/presentation/screens/workshop/workshop_screen.dart#L65-L68)) is now redundant with the carousel tile. Decide whether to leave both (plus-icon = invite-or-create-fallback; carousel = always-create) or simplify the plus icon to always open invite. Recommend leaving both — they're not redundant: the plus icon is still the primary invite affordance, and its empty-state fallback is a graceful degrade rather than a discoverable creation entry point.

**Validates:** Community creation is reachable in **one tap** from Workshop without going through the invite/selection chain. Empty-state communities is no longer a dead end — the carousel itself surfaces the next action.

## Related Issues
- None linked yet. Worth checking the open backlog for any recent UX-rework issues that touch `community_picker_sheet.dart` or `community_selection_modal.dart` before starting Phase 1; if such an issue exists, consolidate.

## Audit Summary (2026-05-16)

**Reviewed against:** [client/architecture.md](../client/architecture.md), [client/caching.md](../client/caching.md), [client/i18n.md](../client/i18n.md), [client/accessibility.md](../client/accessibility.md), [client/testing.md](../client/testing.md), [server/observability.md](../server/observability.md), [server/architecture.md](../server/architecture.md), [CLAUDE.md](../../CLAUDE.md).

**Material changes from initial draft:**

1. Added a **Compliance Requirements** section codifying MVVM/Riverpod rules, cache invalidation, i18n (both en/es ARB), accessibility primitives, ViewModel-first testing, typed analytics events, theme tokens, navigation helpers, and file-size limits.
2. **Cache invalidation gap fixed:** Phase 1 now explicitly calls `CommunityRepository.refreshUserCommunities()` after "Create new community" succeeds. Without this, downstream surfaces would see stale lists until the global TTL expires.
3. **Typed analytics events:** the originally-vague `community_selection_opened` event is now a `CommunitySelectionOpenedEvent implements AnalyticsEvent` typed class to match the existing pattern at [events.dart:530](../../app/lib/core/observability/events.dart#L530).
4. **CommunityPickerButton fate clarified:** the widget has **two** call sites (invite_sheet + item_share_sheet), not one. The plan now keeps it during this issue and defers deletion to a follow-up once `item_share_sheet.dart` is migrated.
5. **Glass full-screen risk:** Phase 5's camera-mode concern was expanded with a concrete preferred fix (extend `GlassSheet` with a `fullscreen` variant) and a fallback (branch on `inputMode`). Avoids per-feature forks of the glass design.
6. **i18n migration debt called out:** the existing `CommunitySelectionModal` already has hardcoded English strings ("Select Communities", "Choose which of your Ripls communities…") — this work surfaces them and must move them to ARB.
7. **Accessibility primitives enumerated:** every new tap surface uses `Tappable`/`IconAction`; every modal uses `showAccessibleModal`; every duration uses `accessibleDuration`; every label uses `context.l10n.a11y*`.
8. **Lint gates listed:** Phase 6 now runs `lint:dart:a11y`, `lint:dart:i18n`, `lint:dart:size`, and `lint:nav` — not just the size check.
9. **Test conventions enforced:** ViewModel-first under `app/test/presentation/viewmodels/`, widget tests under `app/test/presentation/widgets/sharing/`, `ProviderContainer` + `Mockito` with proper `tearDown`, `--reporter expanded` for runs.
10. **Server-side noted as out of scope:** no proto, RPC, or server changes — server docs were reviewed but do not apply.

**No contradictions found** between the recommended approach (Option A, unified widget with mode/selectionMode enums) and the established architecture. The MVVM separation, async-first repository pattern, and Riverpod 3.0 conventions all accommodate the proposed design cleanly.
