# Issue #965: Requests catch-all for closed beta

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/965
**Filed:** 2026-03-31 | **Last activity:** 2026-03-31
**Labels:** P1 (High) | **Assignees:** unassigned
**Context docs:** docs/client/content.md, docs/client/workflow.md

## Problem Statement

Three request-related UI issues need fixing before closed beta:

1. **Light mode creation modal contrast:** The request creation modal (`request_creation_modal.dart`) uses `AppColors.experienceModalBackground(context)` which resolves to `Color(0xFFF5F1ED)` (warm beige) in light mode. This is nearly indistinguishable from the light-mode page background behind the semi-transparent modal barrier, making the modal blend into its surroundings with no visible edge.

2. **"Helping" green pill is non-interactive:** When a non-owner has offered to help on a request, the `_OfferButton` widget renders a sage-green "Helping" pill with `onTap: null` (line 460 of `request_content_view.dart`). Tapping it does nothing. The issue suggests linking to chat.

3. **"Mark Fulfilled" icon collision:** The "Mark Fulfilled" menu item in the owner manage menu uses `Icons.check_circle_outline` (line 526 of `request_content_view.dart`). This same icon is used by `checklist_status_icons.dart` for "not yet completed" default states. The visual collision creates ambiguity about whether the item represents a status indicator or an action.

## Context from Comments

No comments on this issue. All three items are clearly described in the issue body and are straightforward UI fixes.

## Potential Solutions

### Option A: Targeted fixes for all three items
- **Approach:** Fix each issue individually in the existing files. Minimal changes, no refactoring.
- **Pros:** Small blast radius, fast to implement and review.
- **Cons:** None significant — these are all isolated UI bugs.
- **Effort:** Small.

### Option B: Refactor non-owner request actions to use ContentActionBar
- **Approach:** Replace the custom `_OfferButton` + `_ShareButton` row with `ContentActionBar` (which already has helper section, chat pill, and action button segments), aligning the request non-owner experience with how other content types work.
- **Pros:** Consistency with experience/giveaway non-owner UX; the chat tap issue is solved by the built-in chat pill.
- **Cons:** Larger change surface; `ContentActionBar` may not fit the request UX perfectly since requests use a different action model (offer vs. RSVP).
- **Effort:** Medium.

## Recommendation

**Option A** — targeted fixes. These are three independent, small bugs. The non-owner request action row already works well; the `_OfferButton` just needs a tap handler added. A refactor to `ContentActionBar` would be a separate improvement to evaluate on its own merits.

## Open Questions

All resolved:

- **What should the "Helping" pill do when tapped?** — **Decided:** Navigate to the Chat tab (tab index 2), matching `onConfirmedTap` on the owner's `MorphingActionButton`.
- **What icon should "Mark Fulfilled" use?** — **Decided:** Keep a checkmark icon. The current `Icons.check_circle_outline` is acceptable; the issue is about reuse, not the checkmark concept itself. Use a distinct checkmark variant if needed to avoid collision with status icons (e.g., keep `Icons.check_circle_outline` but ensure it doesn't duplicate the exact icon used by `checklist_status_icons.dart` default states).

## Implementation Plan

### Phase 1: Fix all three issues
**Goal:** Resolve all three request UI bugs for closed beta.
**Tasks:**
- [ ] **Light mode modal contrast:** In `request_creation_modal.dart`, improve the modal's visual separation from the page background. Do NOT use a hardcoded `Color(0xFF...)` value — per architecture.md theming rules, all colors must use named `AppColors` helpers. Instead, either (a) add a new theme-aware `AppColors` helper for the modal background with better light-mode contrast, or (b) add a theme-aware border/shadow using `AppColors.border(context)` or `AppColors.divider(context)`, or (c) ensure the `showModalBottomSheet` barrier color is sufficiently dark in light mode.
- [ ] **"Helping" pill tap handler:** In `request_content_view.dart` line 460, change `onTap: isCurrentUserHelping ? null : _handleOffer` to provide a chat-navigation callback when `isCurrentUserHelping` is true. Use the existing `_onTabChanged(2, state)` pattern to switch to the Chat tab.
- [ ] **i18n: Replace hardcoded strings in `_OfferButton`:** The `_OfferButton` widget (line 1044) has hardcoded English strings `'Helping'` and `'Offer to Help'`. Per i18n.md, every user-visible string must use `context.l10n.*`. Add ARB keys (e.g., `requestOfferButtonHelping`, `requestOfferButtonOfferToHelp`) to both `app_en.arb` and `app_es.arb`, then replace the hardcoded strings. _(Audit addition: i18n compliance.)_
- [ ] **"Mark Fulfilled" icon:** In `request_content_view.dart` line 526, keep a checkmark but use a distinct variant that doesn't collide with `checklist_status_icons.dart` default states (e.g., `Icons.check_circle` filled, or `Icons.task_alt`).
- [ ] Update the request checklist menu test (`app/test/presentation/widgets/request/request_checklist_menu_test.dart`) to verify the new icon.
- [ ] Add a test for the "Helping" pill tap behavior — verify it triggers tab change to chat.
- [ ] Run `flutter test --reporter expanded` to confirm no regressions.
**Validates:** All three issues are resolved; the request creation modal is visually distinct in light mode; the "Helping" pill navigates to chat; the "Mark Fulfilled" icon is visually distinct from status icons.

## Audit

**Audited:** 2026-03-31 | **Status:** Compliant (with fixes applied)

| Check | Result | Notes |
|-------|--------|-------|
| MVVM architecture | Pass | All changes are in view layer (widgets). No repository or service changes needed. |
| Theming / AppColors | Fixed | Original plan suggested hardcoded `Color(0xFFEDE7E0)`. Updated to require named `AppColors` helper per architecture.md. |
| i18n compliance | Fixed | `_OfferButton` has hardcoded English strings. Added task to replace with ARB keys per i18n.md. |
| Caching | N/A | No data-layer changes. |
| Server/client boundary | N/A | All changes are client-side UI only. |
| Testing | Pass | Plan includes test updates for icon change and new tap behavior. |

## Related Issues
- None identified.
