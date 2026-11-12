# Issue #1797: design: revamp all modals to unified glass aesthetic

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1797
**Filed:** 2026-05-09 | **Last activity:** 2026-05-09
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** [docs/client/modals.md](../client/modals.md), [docs/client/design.md](../client/design.md), [docs/cowork/App Design/time-modal-standalone.html](../cowork/App%20Design/time-modal-standalone.html), [docs/concepts/modals/time-modal.png](../concepts/modals/time-modal.png)

## Problem Statement

Bottom-sheet modals currently use multiple inconsistent surface treatments — orange-gradient headers from `ModalBuilders.buildGradientHeader`, cream `transferCardBackground` + sage/coral accents for transfer flows, and ad-hoc styles in one-off action sheets. Even within a single modal type the primitives diverge: `ModalBuilders.buildChip` (orange-on-white pill) versus `ModalActionButtons` (coral primary on cream) versus the chat-glass `pillBg`/`bubbleBg` system.

The new direction is to unify every bottom-sheet modal around the **frosted-glass** material shown in [time-modal-standalone.html](../cowork/App%20Design/time-modal-standalone.html) and the visual target in [time-modal.png](../concepts/modals/time-modal.png) — a translucent white sheet over a blurred dark scrim, with white-on-glass typography, two-line chips, an UPPERCASE kicker header, and a coral primary CTA against a `white/10` secondary. The change is **purely visual**: no modal behavior, navigation, validation, state machine, or keyboard handling moves.

### Scope: "modal" means **bottom-sheet only**

The codebase historically uses the word "modal" for at least three different patterns. Only the first is in scope for this work:

| Pattern | Example | In scope? |
|---|---|---|
| **Bottom-sheet modals** (slide up from the bottom over a scrim) | time modal, location picker, RSVP modal, transfer modals, plus-button, action sheets | ✅ **YES — this plan** |
| **Right-side detail screens** (slide in from the right via `pushScreen` / `SwipeToCloseMixin`) | metric detail screens, content views (gear/request/experience), user profile, community public view | ❌ Not modals; not in scope |
| **Creation/preview screens** (full-screen with camera or media behind) | `create_gear_modal`, `experience_creation_modal`, `gear_preview_modal`, `experience_preview_modal`, `community_preview_modal`, `request_preview_modal` | ❌ Being redesigned in a separate effort |
| **Centered alert dialogs** (`AlertDialog` / `showDialog`) | `consent_dialog`, transfer `confirmation_dialog` | ❌ Not bottom sheets; out of scope |
| **Impact-detail modals** (warm-paper `ImpactModalColors` aesthetic) | `co2_detail_modal`, `value_detail_modal`, `quality_time_detail_modal` | ❌ Out of scope (deliberate distinct aesthetic) |

The scope as written in the issue body lists ~17 modals; codebase research found ~70 modal call sites. After excluding the categories above, **all remaining bottom-sheet modals are in scope** and migrate as part of this work. As part of the sweep we also identify and delete unused (dead) modal files.

## Context from Comments

The issue has no comments yet — this plan is being written immediately after creation. Two implications:

1. **No external clarification yet.** The design intent and migration order both come from the issue body alone.
2. **The issue body is unusually thorough.** Tokens, components, waves, out-of-scope, and acceptance criteria are all specified. The role of this plan is to pressure-test the proposed shape against the actual codebase, not to refill missing requirements.

## Architecture compliance audit

These findings come from auditing the plan against [docs/client/architecture.md](../client/architecture.md), [docs/client/accessibility.md](../client/accessibility.md), [docs/client/i18n.md](../client/i18n.md), and [docs/client/testing.md](../client/testing.md). They have been folded into the phases and acceptance criteria below — listed here so reviewers can verify each one.

1. **Theme-aware colors rule has one documented exception.** [architecture.md](../client/architecture.md) states "All colors must be theme-aware." This plan deliberately makes the modal-glass surface and on-glass text **not** theme-aware: white text at varying alphas on a translucent white sheet over a dark scrim, identical in light and dark modes (the scrim handles the theme). This mirrors how the existing chat-glass tokens (`pillBg`, `bubbleBg`) are theme-aware on the *fill* but the on-pill content is rendered against a dark gradient. The deviation must be documented in [docs/client/design.md](../client/design.md) at migration time — otherwise a future contributor will think it's a bug.
2. **`prefer_accessible_modal` lint rule.** Raw `showModalBottomSheet` outside [show_accessible_modal.dart](../../app/lib/presentation/widgets/accessibility/show_accessible_modal.dart) is forbidden. `GlassSheet` is **content only** — it must never call `showModalBottomSheet` directly. Callers wrap their `GlassSheet(...)` inside the builder passed to `showAccessibleModal` (or `showStandardModal`).
3. **`prefer_accessible_duration` lint rule.** Every `Duration(...)` literal in an animation must be wrapped in `accessibleDuration(context, ...)`, which returns `Duration.zero` when the user has reduce-motion enabled. Applies to: the press-scale animation on `GlassChip` / `GlassFooterButtons` (`active:scale-[0.97]` from the reference), the backdrop fade-in, the sheet slide-up, and the height-expand-on-keyboard animation already in `ModalHelpers`.
4. **`avoid_raw_gesture_detector` lint rule.** `GlassChip` must use `Toggle` (it's a selectable state), `GlassFooterButtons` and `GlassInlineAction` must use `Tappable`, and the icon badge in `GlassModalHeader` (if interactive) must use `IconAction`. No raw `GestureDetector` / `InkWell`.
5. **`require_semantic_label_on_icon_button` lint rule.** The reference's circular icon badge in the header (the "🕘" position) is decorative in the time modal but may be tappable in other modals. If interactive, it needs `tooltip:` and `semanticsLabel:` from `context.l10n`. If decorative, render it as a plain `Icon` inside a `Container`, not an `IconAction`.
6. **`require_l10n_for_semantic_labels` lint rule.** Every user-visible string and every `semanticsLabel` / `tooltip` must come from `context.l10n`. New ARB keys required for the kicker labels and field labels: `a11yModalKickerWhen`, `a11yModalFieldDate`, `a11yModalFieldStartTime`, `a11yModalFieldDuration`, `a11yModalInlineActionPickAnotherDate`, `a11yModalInlineActionCustomTime`, etc. Some, like `commonCancel` and `commonSave`, already exist in [app_en.arb](../../app/lib/l10n/app_en.arb) and should be reused.
7. **Both locales required.** Every new ARB key lands in `app_en.arb` AND `app_es.arb` in the same PR. Run `npm run generate` after.
8. **Modal header semantics.** [accessibility.md](../client/accessibility.md) calls out that `ModalHeader` "wraps the title in `Semantics(header: true)`." `GlassModalHeader` must preserve this — wrap the value-text region in `Semantics(header: true, child: Text(...))`.
9. **No business-logic relocation.** This is a widget-layer-only refactor. No `Notifier`, no `@freezed` state, no repository, no provider changes. ViewModels stay where they are (e.g., `time_modal_view_model.dart`, `location_picker_view_model.dart`). The plan is consistent with this; the audit confirms it.
10. **No cache, no service, no proto changes.** Confirmed against [caching.md](../client/caching.md) and the architecture diagram. Out of scope for this plan.
11. **Golden tests are the right primary signal.** [testing.md](../client/testing.md) explicitly calls out "No Golden Tests" as a current gap. A pure visual refactor is exactly the case golden tests exist for. Add golden tests for `GlassSheet`, `GlassChip` (active + inactive), `GlassModalHeader`, `GlassFooterButtons`, `GlassInlineAction` in Phase 1. Per-modal screenshots in the PR description supplement these but don't replace them.
12. **Disposal safety.** No new `Notifier` / `autoDispose` providers are introduced. Existing ViewModels (`time_modal_view_model.dart`, `location_picker_view_model.dart`, `experience_rsvp_view_model.dart`) keep their existing `ref.mounted` patterns. The plan does not touch them.
13. **Server observability and server architecture.** Reviewed; not applicable. No proto changes, no RPC changes, no server-side logging changes.

## Codebase findings (from Phase 2 research)

These are the facts that shape the recommendation. They are not in the issue body:

1. **The current modal dispatch surface is multi-headed, not single-headed.** `ModalHelpers.showStandardModal()` is a thin wrapper over `showAccessibleModal()` ([show_accessible_modal.dart](../../app/lib/presentation/widgets/accessibility/show_accessible_modal.dart)) which is itself a wrapper over `showModalBottomSheet`. A `grep` for `showAccessibleModal\|showModalBottomSheet` yields ~70 files; a `grep` for `showStandardModal` yields fewer than ten. **Most modals do not route through `showStandardModal`.** If we re-skin only `showStandardModal` (as the issue's Wave 0 implies), the visual fix lands on the minority of modals.
2. **`ModalBuilders.buildGradientHeader` is the dominant header pattern**, not `ModalHelpers`. It bakes in the orange gradient + white-text + circular close button look. Roughly half the picker/action modals build through it. Migrating away from `buildGradientHeader` is the actual unit of work for Wave 1, not just calling a different show function.
3. **A `ChipData` change is required**, not optional. Today `ChipData { label, icon, value, isPrimary }`. The reference design needs `primary` (e.g. "Today") + `secondary` (e.g. "May 8") on the same chip, with `icon` no longer required. Either evolve `ChipData` (preferred — call sites already pass a list of chips) or introduce a parallel `GlassChipData` and migrate gradually.
4. **Filename drift between docs and code.** The issue body references `calendar_chips_time_picker_modal.dart`, which no longer exists; it was renamed to [unified_time_modal.dart](../../app/lib/presentation/screens/experience/unified_time_modal.dart). That modal calls `showAccessibleModal` directly (not `showStandardModal`) and renders `time_set_view.dart` / `time_read_view.dart` — re-skinning the time modal means re-skinning those view widgets, not just the show entry point.
5. **`AppColors` already has a glass-surface vocabulary for chat** (`lightPillBg`, `darkPillBg`, `lightBubbleBg`, `darkBubbleBg`, `pillBg(context)`, `bubbleBg(context)`, `tintOverlay(context)`). The new modal-glass tokens should follow the **same naming and structure** to keep `AppColors` coherent. This is a stronger cue than the issue body provided.
6. **Impact-detail modals stay on `ImpactModalColors`.** [app_colors.dart:570](../../app/lib/core/theme/app_colors.dart#L570) defines a separate warm-paper palette used by `co2_detail_modal.dart`, `value_detail_modal.dart`, and `quality_time_detail_modal.dart`. These are intentionally a distinct aesthetic and **not migrated** in this work (decided — see Resolved Decisions).
7. **Transfer flows are atypical and the technically hardest target.** `TransferModalShell` produces a cream-card sheet at 90% height, not a 75/90% picker sheet. The transfer-modal ecosystem (`PickupDetailsCard`, `ImpactMetricsCard`, `CountdownDisplay`, `WaitingOnUserCard`, `WhatHappensNextCard`, `RecipientSelectionCard`, etc.) was built specifically against `transferCardBackground`. Re-skinning this means re-skinning ~14 shared widgets, several of which are nested cards-within-cards — the glass material doesn't compose with itself naturally (a `white/15` card inside a `white/15` sheet vanishes). Phase 3 (Transfer) may land as multiple PRs.
8. **`TextField` and search-input styling assumes opaque cream backgrounds.** `ModalBuilders.buildSearchInput` uses `AppColors.cardBackground` as the field fill. On a glass sheet, that input pops out as a flat white block. The glass-input variant needs `bg-white/8` + `border-white/20` (matching the `GlassInlineAction` row style).

## Potential Solutions

### Option A: Wrapper-first migration (what the issue body proposes)

- **Approach:** Build `GlassSheet` + companion glass widgets. Update `ModalHelpers.showStandardModal()` to use `GlassSheet` internally. Migrate modals wave-by-wave by replacing per-modal layout code with the new shared widgets.
- **Pros:** Minimal Wave-0 footprint. Test surface is small. New widgets are introduced cleanly under `widgets/modal/`. Existing `showStandardModal` call sites benefit immediately.
- **Cons:** Only addresses the minority of modals routed through `showStandardModal`. Most modals (transfer, creation, preview, action sheets) bypass it. Wave 1 ends up rewriting modals to use `GlassSheet` directly anyway, which makes the `showStandardModal` rewire largely cosmetic. Risks two parallel surface treatments coexisting for the duration of the migration.
- **Effort:** Medium overall, but front-loaded into Waves 1–4 rather than the foundation.

### Option B: Glass primitives as the foundation, migration via direct adoption

- **Approach:** Same primitives (`GlassSheet`, `GlassModalHeader`, `GlassFieldLabel`, `GlassChip`, `GlassInlineAction`, `GlassFooterButtons`, `GlassSearchInput`, `GlassListTile`). Do **not** rewire `showStandardModal` to inject the glass surface implicitly. Instead, modals that migrate explicitly opt in by wrapping their content in `GlassSheet`. Deprecate `ModalBuilders.buildGradientHeader` immediately (mark `@Deprecated`) so new code can't reach for it.
- **Pros:** Migrated and unmigrated modals are visually distinct, which makes it easy to track progress and screenshot for review. No "half-glass" surfaces caused by partial wrapping. Works uniformly for `showStandardModal`, `showAccessibleModal`, and direct `showModalBottomSheet` callers — the surface is a widget choice, not a show-function choice.
- **Cons:** Slightly more boilerplate at each migrated modal call site (one `GlassSheet(...)` wrap). Doesn't give existing `showStandardModal` callers a free upgrade.
- **Effort:** Same total cost as Option A but distributed more evenly across waves.

### Option C: Big-bang migration

- **Approach:** Single PR that introduces tokens, primitives, AND migrates all modals at once.
- **Pros:** No half-state in the codebase. One review, one screenshot pass.
- **Cons:** Unreviewable. Mixes ~70 modal sites across creation, preview, transfer, picker, action, and impact in a single diff. Almost guaranteed to ship a regression. Explicitly counter to the issue body's stated wave-by-wave preference.
- **Effort:** Large, and dangerous.

## Recommendation

**Option B**, with one amendment to the issue body:

1. **Don't rewire `ModalHelpers.showStandardModal()` in Wave 0.** Leave its API and surface untouched. Adoption is via `GlassSheet`, regardless of show function. This avoids the "did the migration land or not" ambiguity at half-migrated state.

Why Option B over A: the deciding factor is finding (1) — most modals don't go through `showStandardModal`, so rewiring it as the migration vehicle gives a smaller win than the issue body implies. Making `GlassSheet` the unit of opt-in works for every show function in the codebase.

The issue's original wave list (creation modals, preview modals) is descoped — those screens are being redesigned in a separate effort and are explicitly out of scope here. The impact-detail modals are also out of scope. The work that remains: **(Phase 1)** glass primitives + tokens, **(Phase 2)** all in-scope picker/action bottom sheets including the long tail not enumerated in the issue body, **(Phase 3)** transfer modals, **(Phase 4)** cleanup.

## Resolved Decisions

All open questions were resolved by the issue author on 2026-05-09. Recording here so reviewers can see the chosen direction without re-litigating:

- **Creation modals and preview modals are NOT in scope.** They are being redesigned in a separate effort. Phase 3 (Creation) and Phase 4 (Preview) from the original plan are removed.
- **Right-side detail screens were never modals.** The metric detail screens (and any other `pushScreen`-based detail surface) are full screens, not bottom sheets. They are not in scope and never were.
- **Centered `AlertDialog`-style dialogs are not in scope.** Only widgets that slide up from the bottom are migrated. `consent_dialog`, the transfer `confirmation_dialog`, and any other `showDialog`-based widget stay on their current styling.
- **Impact-detail modals (`co2_detail_modal`, `value_detail_modal`, `quality_time_detail_modal`) are NOT in scope.** `ImpactModalColors` and its warm-paper aesthetic stay. No follow-up issue needed.
- **Migrate the long tail of bottom-sheet modals not enumerated in the issue body.** `cancel_event_modal`, `event_joiner_modal`, `event_organizer_modal`, `mark_completed_modal`, `mark_fulfilled_modal`, `arrange_pickup_modal`, `time_poll_*` modals, the `needs/*` sheets, `attendee_list_sheet`, `gear_metadata_sheet`, `feedback_sheet`, `discover_filter_sheet`, `community_picker_sheet`, `portfolio_filter_sheet`, `weekly_goal_editor`, `gear_borrower_sheet`, `gear_interest_sheet`, `helper_list_sheet`, `request_needs_sheets`, `shadow_user_profile_sheet`, `category_material_editor`, `gear_access_sheet_helper`, etc. — all in scope.
- **Identify and delete dead modals as part of the sweep.** Any modal file in [app/lib/presentation/](../../app/lib/presentation/) with no remaining call sites gets deleted, not migrated. This is a Phase 2 task and is enumerated below.
- **`ChipData` is extended in place.** Add optional `String? secondary` and make `IconData? icon` nullable. No parallel `GlassChipData` class.
- **`experienceModalBackground` and `transferCardBackground` get cleaned up.** Both are surface tokens that should not survive the migration. `experienceModalBackground` is unused after creation modals are out of scope, so it can be deleted in Phase 1 (after grep-confirming no remaining call sites — creation modals own all of them today, but they're being redesigned independently and won't keep the warm-beige). `transferCardBackground` is deleted as part of Phase 3 once the transfer flow is on glass. `transferSage` / `transferCoral` (semantic accents, not surface) survive.
- **Glass surface renders identically in light and dark modes.** White-on-translucent on a dark scrim, no theme-aware switching on the sheet itself. This is documented as an explicit deviation from the theme-aware-color rule in [docs/client/design.md](../client/design.md).
- **`BackdropFilter` ships with a graceful fallback.** `GlassSurface` takes a `useBlur: bool` defaulting to `true`. One app-level kill-switch can disable it entirely (e.g., for low-end Android). Fallback renders a flat `white/15` rectangle — visually close enough that the migration doesn't fail without blur.
- **Screen-reader and accessibility expectations are fully preserved.** All glass primitives use `Tappable` / `Toggle` / `IconAction`; the modal title region uses `Semantics(header: true)`; all user-visible strings come from `context.l10n`; `accessibleDuration(context, ...)` wraps every animation `Duration`. The a11y allowlist (`scripts/a11y_allowlist.txt`) must remain at zero entries throughout the migration.

## Implementation Plan

### Phase 1: Foundation (Wave 0 — single PR)

**Goal:** Glass tokens and primitives exist, are documented, and are tested. No modal is migrated yet.

**Tasks:**
- [ ] Add `AppColors.modal*` tokens to [app_colors.dart](../../app/lib/core/theme/app_colors.dart), modeling the naming after the chat-glass section (`lightPillBg` / `darkPillBg` / `pillBg(context)`). Cover: backdrop, surface, surface blur sigma, border, drag handle, text-primary/secondary/tertiary, kicker, chip background/border/text (active + inactive), inline-action background/border/text, footer divider, secondary-button background/border/text, primary-button background/text.
- [ ] Add `ModalTheme` constants block to [app_theme.dart](../../app/lib/core/theme/app_theme.dart): top radius (28), horizontal inset (12), max height fraction (0.92), chip radius (16), inline-action radius (16) + height (44), button height (44), kicker text style, field-label text style, header-value text style, chip primary/secondary text styles, press scale (0.97).
- [ ] Implement `GlassSurface` (the blur + fill + border primitive that everything else builds on, with `useBlur` toggle).
- [ ] Implement `GlassSheet` (**content widget only — must NOT call `showModalBottomSheet`**; wraps content with drag handle, top-rounded corners, scroll, and the dark scrim above it). Use `accessibleDuration(context, ...)` for any built-in transitions.
- [ ] Implement `GlassModalHeader` (icon badge + UPPERCASE kicker + value). Wrap the value-text region in `Semantics(header: true, child: ...)` to mirror the existing `ModalHeader` accessibility contract.
- [ ] Implement `GlassFieldLabel`.
- [ ] Implement `GlassChip` (primary/secondary text, active/inactive states, press feedback) — backed by `ChipData` with new optional `secondary` field. Use `Toggle` (not `GestureDetector`) for selection. Wrap the press-scale animation in `accessibleDuration(context, ...)`.
- [ ] Implement `GlassInlineAction` (44px row pattern for "Pick another date…"). Use `Tappable` (not `GestureDetector`) and pull `semanticsLabel` from `context.l10n`.
- [ ] Implement `GlassFooterButtons` (cancel ghost + coral primary, separated by `modalFooterDivider`). Use `Tappable` for the buttons; reuse `commonCancel` / `commonSave` ARB keys.
- [ ] Implement `GlassSearchInput` (white-on-glass variant of `ModalBuilders.buildSearchInput`).
- [ ] Extend `ChipData` with `String? secondary` and make `IconData icon` nullable. Update existing call sites that currently rely on `icon` being non-null (no-op for them).
- [ ] Mark `ModalBuilders.buildGradientHeader` as `@Deprecated('Use GlassModalHeader inside a GlassSheet')`. Don't delete yet.
- [ ] Grep-confirm `AppColors.experienceModalBackground` and `AppColors.lightExperienceModalBackground` are referenced only by creation modals (which are out of scope for this work and being redesigned separately). If creation modals are not actively retaining the warm-beige, delete the tokens in this phase. Otherwise leave them and revisit in Phase 4 (Cleanup).
- [ ] Add new ARB keys to [app_en.arb](../../app/lib/l10n/app_en.arb) AND [app_es.arb](../../app/lib/l10n/app_es.arb) for any new generic glass-primitive strings (e.g. `a11yModalDragHandle`). Per-modal kicker/field labels are added in Phase 2 alongside the modal that needs them. Reuse `commonCancel`, `commonSave`, `a11yClose` where possible. Run `npm run generate` after.
- [ ] Add **widget tests** under `app/test/presentation/widgets/modal/` covering: `GlassSheet` renders drag handle and content, `GlassChip` toggles active state, `GlassFooterButtons` triggers correct callbacks, `GlassModalHeader` renders kicker and value with `Semantics(header: true)`, `GlassChip` renders primary+secondary correctly.
- [ ] Add **golden tests** for `GlassSheet`, `GlassChip` (active + inactive), `GlassModalHeader`, `GlassFooterButtons`, `GlassInlineAction` against a representative dark-scrim backdrop. This is the primary visual-regression signal for later phases.
- [ ] Add an a11y smoke test verifying that `Semantics` labels survive the rewrap (find by semanticsLabel in widget tests).
- [ ] Verify `npm run lint:dart:a11y` and `npm run lint:dart:i18n` pass; the a11y allowlist (`scripts/a11y_allowlist.txt`) should remain at zero entries.
- [ ] Update [docs/client/modals.md](../client/modals.md) and [docs/client/design.md](../client/design.md) with the new tokens, components, and reference snippet. **Document the white-on-glass exception to the theme-aware-color rule** — explain that the modal-glass surface and on-glass text are intentionally identical in light and dark modes because they sit on a dark scrim. Note the deprecation of `buildGradientHeader` and `experienceModalBackground`.

**Validates:** The primitives compose. The glass surface looks right against a backdrop image. Tests pass. `npm run build`, `npm run lint:dart:size`, `npm run lint:dart:a11y`, and `npm run lint:dart:i18n` all pass. No production modal has changed yet.

### Phase 2: Bottom-sheet modal sweep + dead-code cleanup

**Goal:** Migrate every in-scope bottom-sheet modal to the glass surface, AND delete unused modal files discovered along the way. This phase replaces the original Wave-1 list with the full long tail.

**Inventory and triage (do first):**
- [ ] Build a CSV of every modal-shaped widget under `app/lib/presentation/screens/` and `app/lib/presentation/widgets/` whose name contains `_modal`, `_sheet`, or `_dialog`, plus every file that calls `showAccessibleModal` or `showStandardModal`. For each row record: file path, show function used, presentation type (`bottom-sheet` / `dialog` / `unknown`), call-site count (via grep for `static show(` or the class name).
- [ ] Triage each row into one of: **MIGRATE** (in-scope bottom sheet), **DEAD** (zero call sites — delete the file), **OUT-OF-SCOPE: dialog** (centered `AlertDialog` / `showDialog` — leaves on current styling), **OUT-OF-SCOPE: creation** (creation/preview modals, redesigned separately), **OUT-OF-SCOPE: impact** (impact-detail modals, intentionally distinct), **OUT-OF-SCOPE: not-a-modal** (anything that turns out to be a right-side detail screen).
- [ ] Commit the triage CSV under `docs/ai/modal_glass_migration.md` so progress is trackable across PRs.
- [ ] Delete every file marked **DEAD** in a dedicated PR before any visual migration starts. Run `npm run build` and `flutter test` to confirm nothing imports them.

**Migrate (high-priority, direct match for reference):**
- [ ] [unified_time_modal.dart](../../app/lib/presentation/screens/experience/unified_time_modal.dart) (and its `time_set_view.dart` / `time_read_view.dart`). Direct match for the reference image — date / start-time / duration chip layout maps 1:1.
- [ ] [location_picker_modal.dart](../../app/lib/presentation/widgets/location/location_picker_modal.dart). Replace `buildGradientHeader` + `buildSearchInput` + `buildChipsRow` with `GlassModalHeader` + `GlassSearchInput` + `GlassChip` row.
- [ ] [rsvp_modal.dart](../../app/lib/presentation/screens/experience/rsvp_modal.dart) (the file the issue refers to as `experience_rsvp_modal.dart`).
- [ ] [plus_button_modal.dart](../../app/lib/presentation/widgets/plus_button_modal.dart).
- [ ] [media_picker_dialog.dart](../../app/lib/presentation/widgets/media/media_picker_dialog.dart) — verify during triage that this is a bottom sheet, not a true `Dialog`. If it's a `Dialog`, mark out-of-scope.

**Migrate (event/experience action sheets):**
- [ ] `cancel_event_modal`, `mark_completed_modal`, `event_joiner_modal`, `event_organizer_modal`, `time_poll_propose_modal`, `time_poll_vote_modal`, `time_poll_confirm_modal`, `attendee_list_sheet`, `experience_participants_header` (its sheet), the `needs/*` sheets (`add_contribution_sheet`, `batch_add_contribution_sheet`, `batch_add_need_sheet`, `edit_contribution_sheet`, `view_contribution_sheet`, `view_need_sheet`).

**Migrate (gear/request/loan action sheets — non-transfer):**
- [ ] `mark_fulfilled_modal`, `arrange_pickup_modal` (verify whether this is part of the transfer family or a standalone sheet — if transfer, defer to Phase 3), `loan_list_modal`, `past_transfer_modal`, `gear_borrower_sheet`, `gear_interest_sheet`, `gear_metadata_sheet`, `helper_list_sheet`, `request_needs_sheets`, `request_management_modal`, `request_help_modal`, `gear_access_sheet_helper`, `category_material_editor`.

**Migrate (community / portfolio / discover / settings sheets):**
- [ ] `community_picker_sheet`, `discover_filter_sheet`, `portfolio_filter_sheet`, `weekly_goal_editor`, `feedback_sheet`, `danger_zone_choice_modal`, `shadow_user_profile_sheet`, `access_sheet`, `close_item_modal`, `content_info_modal`, `content_overflow_menu`, `media_pick_from_carousel`, `qt_attributes_card` (its sheet entry point if any), `empty_content_state` (its sheet entry point if any).

**Out-of-scope reminders (do NOT migrate, but verify during triage):**
- Centered alert dialogs: `consent_dialog`, transfer `confirmation_dialog`.
- Creation modals: `create_gear_modal`, `experience_creation_modal`, `community_creation_modal`, `request_creation_modal`.
- Preview modals: `gear_preview_modal`, `request_preview_modal`, `experience_preview_modal`, `community_preview_modal`.
- Impact-detail modals: `co2_detail_modal`, `value_detail_modal`, `quality_time_detail_modal`, anything in `widgets/impact/completion/` driven by `ImpactModalColors`.
- Right-side detail screens: any `*_content_view`, `*_screen`, or `*_metric_*_screen` that uses `pushScreen` + `SwipeToCloseMixin`.

**Other tasks:**
- [ ] As each modal migrates, add per-modal kicker/field-label ARB keys to both locales. Run `npm run generate`.
- [ ] Each migrated modal: before/after screenshot in PR description.
- [ ] After all migrations land, delete `AppColors.experienceModalBackground` / `lightExperienceModalBackground` (creation modals are out of scope and aren't keeping warm-beige; if creation modals retain the token until their own redesign lands, defer this delete to Phase 4).

**Validates:** The glass primitives compose against real-world content (loading state, empty state, scrolling content, search dropdowns, autocomplete, time-poll voting UI, multi-step needs flows). The codebase has no dead modal files. After this phase, every in-scope bottom-sheet modal reads as the same material; the only remaining visual variance is the transfer family.

### Phase 3: Transfer modals

**Goal:** Re-skin the largest and most internally-nested modal family. May land as multiple PRs.

**Tasks:**
- [ ] Decide upfront: do we keep `transferCardBackground` (cream sheet) or move to glass? Recommendation: switch the *outer* sheet to glass; for cards-within-the-sheet (`PickupDetailsCard`, `ImpactMetricsCard`, etc.), use a `GlassInsetCard` variant — slightly different alpha (e.g. `white/8`) so it reads as a layer above the sheet without disappearing.
- [ ] Re-skin `TransferModalShell`, `TransferModalHeader`, `TransferModalTitle`, `TransferPhaseBottomBar`, `TransferProgressBar`, `TransferErrorDisplay`.
- [ ] Re-skin the shared cards: `PickupDetailsCard`, `ImpactMetricsCard`, `ImpactMetricRow`, `CountdownDisplay`, `WaitingOnUserCard`, `WhatHappensNextCard`, `WhatTheySeeCard`, `OtherPartySees`, `RecipientSelectionCard`, `QuickDateChips`, `QuickTimeChips`, `DurationChips`, `StepChecklistItem`, `TransferTimeline`, `ConfirmationDialog`.
- [ ] Migrate the four implementations: [loan_borrower_modal.dart](../../app/lib/presentation/screens/gear/loan_borrower_modal.dart), [loan_owner_modal.dart](../../app/lib/presentation/screens/gear/loan_owner_modal.dart), [giveaway_receiver_modal.dart](../../app/lib/presentation/screens/gear/giveaway_receiver_modal.dart), [giveaway_giver_modal.dart](../../app/lib/presentation/screens/gear/giveaway_giver_modal.dart).
- [ ] Keep `transferSage` / `transferCoral` as semantic accents on action buttons and timeline dots — do not retire them.
- [ ] Decide whether to delete `transferBackgroundCream`, `transferCardBackground`, `transferTextPrimary`, `transferTextSecondary`, `transferTextMuted`, `transferBorder`, `transferBorderLight`, `transferHighlight`. Recommendation: delete the surface tokens (`*Background*`, `*Card*`); keep the text tokens until we've audited that every remaining call site is on a glass surface.
- [ ] Before/after screenshot per phase per modal (transfer phases differ by state machine — at minimum capture one screenshot per phase).

**Validates:** Cards-in-cards render correctly on glass. Transfer phase transitions don't flicker. The end-to-end loan and giveaway flows look like the same material as the Phase 2 picker/action sheets.

### Phase 4: Cleanup and consolidation

**Goal:** Close out the migration, retire deprecated APIs, update docs.

**Tasks:**
- [ ] Delete `ModalBuilders.buildGradientHeader` and any of its companion methods (`buildOwnerEditSection`, `buildHeaderActionButtons`) that no longer have call sites.
- [ ] Delete `ModalActionButtons` if no longer referenced (the new `GlassFooterButtons` replaces it).
- [ ] Delete `AppColors.experienceModalBackground` / `lightExperienceModalBackground` if Phase 2 deferred it (creation modals are out of scope and don't retain it after their own redesign).
- [ ] Audit any remaining `showModalBottomSheet` call sites that bypassed `showAccessibleModal`. Route them through `showAccessibleModal` and wrap content in `GlassSheet`.
- [ ] Remove the "Modal Heights" inconsistency note from [docs/client/design.md](../client/design.md).
- [ ] Update [docs/client/modals.md](../client/modals.md) "Last Updated" and remove references to legacy modal background color tokens. Add a note that creation/preview modals and impact-detail modals are intentionally on different surfaces (separate aesthetics, separate redesigns).
- [ ] Verify `npm run lint:dart:size`, `npm run lint:dart:a11y`, `npm run lint:dart:i18n`, `flutter test`, `npm run build` all pass.
- [ ] Verify `scripts/a11y_allowlist.txt` is still empty after the migration (no new exemptions added).
- [ ] Confirm the dead-modal CSV from Phase 2 has zero remaining MIGRATE rows; archive `docs/ai/modal_glass_migration.md` or move it to `docs/ai/archive/`.

**Validates:** Acceptance criteria from the issue are met; every in-scope bottom-sheet modal renders on the glass material; dead code is gone.

## Related Issues

None known. The creation and preview modals are being redesigned in a parallel effort outside this issue. The impact-detail modals (`ImpactModalColors`) are intentionally retained on their warm-paper aesthetic. The "Modal Fragmentation" weakness called out in [docs/client/modals.md](../client/modals.md) is being directly retired by this work for bottom-sheet modals.
