# Issue #2154: Redesign experienceContentView to match event-item-4.html mockup

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2154
**Filed:** 2026-05-24 | **Last activity:** 2026-05-24
**Labels:** none | **Assignees:** unassigned
**Context docs:** [docs/cowork/App Design/event-item-4.html](../cowork/App%20Design/event-item-4.html)

> **Audit pass (2026-05-24):** Reviewed against `docs/client/{architecture,caching,i18n,accessibility,testing,content,modals}.md` and `docs/server/{architecture,observability}.md`. Corrections folded in below. Notable: `ContentViewTabBar` is **not** variable-arity at the wrapper level (it always emits three `ContentTab`s — see `app/lib/presentation/widgets/content/content_view_tab_bar.dart:80-88`), so removing a tab requires either bypassing the wrapper or extending it. The underlying `ContentTabBar` does take a `List<ContentTab>`. Several manage-menu strings already exist and should be reused rather than added.

## Problem Statement

The Event tab on the experience content view has accumulated chrome that has become redundant or visually inconsistent now that the inline rows themselves expose all the planning surface area. Specifically:

- The second tab (labeled "Plan" in EN, `experienceTabWhyGo`) duplicates information the Event tab already shows inline (location/time polls, needs, source URL). The dedicated Plan pane is no longer carrying its weight.
- The `AccessRing` "circles" affordance at the trailing edge of the tab bar uses a stacked-community-bubbles widget that competes visually with the third tab. The mockup replaces it with a single people icon + count badge that sits inside the tab strip.
- The action row at the bottom of the Event pane (the `WorkflowActionButtons.buildExperience` morphing pill plus the attendee row) is a third visual treatment, distinct from both the row pills (`_NeedsHelpPill`, `_LocationVotePill`, `_TimeChoicePill`) and from anything else in the pane. The mockup folds it into a fifth `InfoRow` (RSVP icon · attendee stack · "N going" · RSVP pill button).
- The owner row uses a `ContentAvatar` (initials chip / photo) while every other row uses a stroked icon glyph. The mockup unifies it with a `PersonIcon` so all five rows have the same leading column.
- Owner management lives behind that bottom morphing button's overflow menu. The mockup pulls it out to a pencil icon next to the title that opens a frosted bottom sheet — same surface treatment the location poll's Manage button uses today (`PollManageMenuSheet` / `GlassSheet`). The menu shrinks to three items: **Edit Details**, **Mark Completed**, **Close Event**.

Source of truth files:
- [app/lib/presentation/screens/experience/experience_content_view.dart](../../app/lib/presentation/screens/experience/experience_content_view.dart)
- [app/lib/presentation/screens/experience/widgets/experience_event_pane.dart](../../app/lib/presentation/screens/experience/widgets/experience_event_pane.dart) (920 lines — already at the soft-limit; this redesign should leave it smaller, not larger)
- [app/lib/presentation/screens/experience/widgets/experience_bottom_content.dart](../../app/lib/presentation/screens/experience/widgets/experience_bottom_content.dart)
- [app/lib/presentation/screens/experience/widgets/experience_details_pane.dart](../../app/lib/presentation/screens/experience/widgets/experience_details_pane.dart) (the pane that goes away)
- [app/lib/presentation/widgets/content/content_view_tab_bar.dart](../../app/lib/presentation/widgets/content/content_view_tab_bar.dart)

## Context from Comments

The issue has no comments. The mockup itself ([event-item-4.html](../cowork/App%20Design/event-item-4.html)) is the authoritative spec — every ambiguous decision should be resolved by looking at it.

One ambiguity the mockup resolves but the issue does not state: in the completed/cancelled state the existing code swaps the Plan tab for a "Results" tab via `experienceTabResults` and a separate `ExperienceResultsPane`. The mockup has nothing to say about the terminal state. **This plan assumes the Results tab is preserved for `EXPERIENCE_STATE_COMPLETED` only** — the new design removes the Plan/WhyGo tab but the impact summary that the Results pane shows after completion is not redundant and is worth keeping. Flagged as an open question below.

## Potential Solutions

### Option A: One PR, full redesign in place

- **Approach:** Modify `ExperienceEventPane`, `ExperienceBottomContent`, and `ExperienceContentView` together. Drop the Details/WhyGo tab from the tab bar entirely, rebuild the RSVP/action row as an `InfoRow`, swap the owner avatar for a `PersonIcon` row, replace `AccessRing` with a new compact people-pill, add the edit pencil + frosted Manage sheet next to the title, and delete `ExperienceDetailsPane` (preserve `ExperienceResultsPane` for completed events).
- **Pros:** Single coherent change, mockup ships intact. No intermediate UI states for users to learn twice. Net code reduction (delete details pane, simplify owner-overflow menu).
- **Cons:** Big surface area in one PR. Higher review burden. If something needs to revert it all reverts together.
- **Effort:** Medium.

### Option B: Phased — chrome first, then tab removal

- **Approach:** Phase 1 ships the within-row changes (RSVP-as-row, owner-as-row, edit pencil + frosted manage sheet, AccessRing → people pill). Phase 2 removes the Plan tab and deletes `ExperienceDetailsPane`. Phase 3 removes any dead code uncovered.
- **Pros:** Each phase is small enough to review and revert independently. Phase 1 has no behavior loss — only visual unification. Phase 2 is the riskier "are we sure no one needs that pane" call, made on its own.
- **Cons:** Two-step rollout means a brief period where the tab bar shows a redundant Plan tab next to the new in-row Manage affordance — users have two ways to get to the same actions. Some logic in `ExperienceEventPane._buildEventActions` may need temporary scaffolding to coexist with the old morphing pill, then get deleted in phase 2.
- **Effort:** Medium-large (more total work than A because of intermediate scaffolding).

### Option C: Mockup-first prototype, then graft in

- **Approach:** Build the new layout as a parallel pane (`ExperienceEventPaneV2`), behind a feature flag or `kDebugMode` toggle, validate on device, then delete the old pane and the flag.
- **Pros:** Easy to A/B against the existing pane on the same device. No risk of breaking the live pane mid-build.
- **Cons:** Codebase memory explicitly bans `v2/` suffixed directories (see `feedback_theme_reuse.md`). Project convention treats parallel-rewrite paths as a smell — we delete and replace, we don't fork. Also doubles the file count temporarily and adds a flag to plumb through.
- **Effort:** Large.

## Recommendation

**Option A — single PR, full redesign in place.** The mockup is concrete enough to build straight to, the change is mostly cosmetic-with-deletion (no new RPCs, no new state), and there is no production cost to bundling the chrome unification with the tab removal — they are the same design intent. Option B's intermediate "tab bar with both a Plan tab and a Manage pencil" state is genuinely confusing rather than safer.

The two pieces that are worth pre-validating with the user before opening a PR (because they affect either the user-visible info architecture or the design fidelity):

1. Whether the Results tab survives for completed events.
2. Whether "Edit Details" should open the existing in-place edit-mode flow (`onToggleEditMode` — the current behavior, edits the title/description/source-URL/cover-photo inline against the same background) or open a new dedicated sheet that combines title + description + background change into one bottom-sheet flow.

Both have recommendations in Open Questions below.

## Open Questions

- **Is the Results tab preserved for completed events?** — Recommendation: **Yes, keep it.** The impact summary that `ExperienceResultsPane` shows post-completion is genuinely different content from the live Event pane, and the mockup is silent about terminal states (so there is no design directive to remove it). Net effect: the tab bar shows one tab (Event + Chat) for active events, two tabs (Event + Results + Chat) for completed events. Wait — that's three. Confirm whether the bottom tab bar collapses to two for active and stays at three for completed, or whether the third tab (Chat) is also retained throughout. The current code has three tabs always; this plan assumes Event + Chat for active (drop Plan), Event + Results + Chat for completed.
- **Does "Edit Details" open inline edit mode or a dedicated sheet?** — Recommendation: **Open the existing inline edit mode** (`onToggleEditMode`). The current edit-mode flow already covers title, description, source URL, and background replacement via the cover-photo media picker; building a dedicated sheet would duplicate that surface. The mockup's three-item menu (Edit Details, Mark Completed, Close Event) suggests collapsing the current overflow menu — not building new edit surfaces. The "Change Background" option in the HTML mockup's manage menu is rolled into "Edit Details" per the issue body.
- **What replaces the current "Confirm Location" / "Confirm Time" / "RSVP" / "View Impact" overflow menu items?** The issue strips the manage menu to three items. The location/time confirmation and RSVP entries are already reachable via the inline rows (tapping the row opens the picker; the RSVP row exposes RSVP). Recommendation: **Delete the redundant menu items** since the rows cover them. For the `View Impact` item that appears in the completed-state menu, recommend folding it into the new Results pane (or keep it as a fourth menu item that's only present when `isCompleted`).
- **What happens to non-owner overflow actions?** Non-owners today see the morphing pill but no menu items. Recommendation: **No edit pencil for non-owners** — the pencil + sheet appear only when `state.isOwner` is true. Non-owners still see the title/description/rows; their only action surface is the RSVP pill in the new RSVP row.
- **AccessRing replacement — does the new people-pill open the same `AccessSheet`?** Recommendation: **Yes.** The community-membership detail surface (`AccessSheet`) is unchanged; only its trigger affordance shrinks from a stacked-bubbles ring to an icon+count pill. The pill belongs in the same trailing slot of `ContentViewTabBar` as today's ring.
- **Row "subtle buttons for voting and helping" — pill shape vs completed-state styling.** **Confirmed with the user:** the existing pills (`_NeedsHelpPill`, `_LocationVotePill`, `_TimeChoicePill`) are correct in shape and CTA state, but the **completed** (`chosen` / `helped == true`) rendering must lose its solid-fill background. The mockup's `StatusPill` uses a translucent fill + matching border + pale text (in Tailwind: `bg-[#87A083]/22 border border-[#87A083]/35 text-[#C8D6BF]`). The pre-action ("Help" / "Choose") state keeps its existing coral solid fill — it's the CTA, the completed state is passive confirmation. The Directions/CalendarPlus inline icon-buttons stay as inline icon-buttons; this rule applies only to the three completed-state pills.

## Implementation Plan

### Phase 1: Tab bar + trailing pill
**Goal:** Drop the Plan/WhyGo tab from active events; replace `AccessRing` with a compact icon-count pill while preserving the `AccessSheet` entry-point.
**Tasks:**
- [ ] **Tab-bar plumbing.** `ContentViewTabBar` is a hard-coded three-tab wrapper used by gear and request as well; we can't just drop a tab. Two acceptable approaches — pick at implementation time:
  - **(a)** Add an optional `secondTabLabel` nullable + a boolean `showSecondTab` (or simpler: treat `secondTabLabel == null` as "omit"), and have the wrapper emit either 2 or 3 `ContentTab`s. Keeps the live-unread badge logic in one place.
  - **(b)** Bypass `ContentViewTabBar` from the experience view and compose `ContentTabBar` (which already accepts `List<ContentTab>`) directly, lifting the `unreadCountProvider` watch into `ExperienceBottomContent` or a small helper. Slight code duplication but no impact on gear/request.
  Recommend (a) — single change, no duplication.
- [ ] In [experience_bottom_content.dart](../../app/lib/presentation/screens/experience/widgets/experience_bottom_content.dart), render Event + Chat for active events, Event + Results + Chat for completed events using the extended wrapper.
- [ ] In [experience_content_view.dart](../../app/lib/presentation/screens/experience/experience_content_view.dart), reindex `_onTabChanged` and `setActiveTab` so Chat is index 1 for active events (today it is 2). Replace literal `2` checks (e.g., `_buildBottomContent`'s nav-lock listener at lines 166-176, `_onTabChanged`'s `lockNav` call at 235) with a named accessor on `ExperienceState` like `state.chatTabIndex` (computed `2` if completed else `1`). Update `widget.initialTab` documentation. Bounds-check the persisted `activeTab` when a completed event's state changes back (defensive, but cheap).
- [ ] Replace the `AccessRing` widget in `_buildAccessRing` with a new compact `_PeoplePill` widget (people icon glyph + tabular-num count). Build the tap surface with the [`Tappable`](../../app/lib/presentation/widgets/accessibility/tappable.dart) primitive (not a bare `GestureDetector`) and pass a localized `semanticsLabel` describing both action and count — `npm run lint:dart:a11y` will fail otherwise. Tap target stays the same — opens `_showAccessSheet`. Reuse `AppColors` tokens per `feedback_theme_reuse.md`; do not introduce new palette entries.
- [ ] Delete [experience_details_pane.dart](../../app/lib/presentation/screens/experience/widgets/experience_details_pane.dart) and its `_buildDetailsPane` caller. Keep `ExperienceResultsPane` for completed events. Confirm no other view imports it (a quick grep — gear/request views compose their own details panes, but verify).
- [ ] **l10n.** Delete the `experienceTabWhyGo` ARB string from **both** `app/lib/l10n/app_en.arb` **and** `app/lib/l10n/app_es.arb` (i18n.md: keys must exist in both before regenerating). Also remove the corresponding `@experienceTabWhyGo` description entry. Run `npm run generate` (project convention from CLAUDE.md — not `flutter gen-l10n` directly) so the generated `app_localizations*.dart` files update. Sweep for stragglers with `grep -rn experienceTabWhyGo app/lib app/test`.
- [ ] Add/update widget tests in [app/test/presentation/screens/experience/](../../app/test/presentation/screens/experience/) per `docs/client/testing.md`: drive via `ProviderContainer` with a mocked experience repository; assert the tab bar renders 2 tabs when `experience.state != COMPLETED` and 3 tabs when it does; assert tapping the people-pill opens the access sheet (use a `pumpWidget` + tap test).

**Validates:** Tab info architecture is correct, no orphan code, the people-pill renders and opens the access sheet, completed events still surface Results.

### Phase 2: Rows — owner, RSVP, Manage pencil
**Goal:** Unify the bottom of the pane on the `ContentInfoRow` shape, replace the bottom action bar with a fifth row, replace the morphing-manage menu with an edit pencil + frosted bottom sheet.
**Tasks:**
- [ ] In `_buildOwnerRow` ([experience_event_pane.dart](../../app/lib/presentation/screens/experience/widgets/experience_event_pane.dart)), swap the `ContentAvatar` leading widget for `Icons.person_outline` rendered through `ContentInfoRow`'s `icon` slot. Owner name stays in `value`; "shared 1d ago" stays in `trailingText`. (If the avatar is still wanted on tap → `openUserScreen`, that's already wired on `onTap`.)
- [ ] Replace `_buildEventActions`'s `WorkflowActionButtons.buildExperience` + attendee-row block with a new `_buildRsvpRow` that returns a `ContentInfoRow` shaped like the mockup: `Icons.event_available_outlined` leading, attendee-stack + "N going" as the `value` (the stack will need to render through the `leading` slot since `ContentInfoRow`'s `value` is a String — confirm by reading `app/lib/presentation/widgets/content/content_info_row.dart` and extend the row's contract if needed rather than fork it), trailing `ExperienceRsvpButton` pill (already exists). For non-owners this row replaces the entire bottom action block. Tapping the row or the pill opens `_showAttendeeSheet`.
- [ ] In the title row, add an edit-pencil affordance to the right of the title for `state.isOwner && !isTerminal`. Build it with [`IconAction`](../../app/lib/presentation/widgets/accessibility/) (not a bare `IconButton`) so `semanticsLabel` + `tooltip` are mandatory — `accessibility.md` requires this and `npm run lint:dart:a11y` enforces it. Wire it to a new `_showManageSheet(context)` method.
- [ ] Implement `_showManageSheet` using `showAccessibleModal` + a new `ExperienceManageMenuSheet` widget composed over [`GlassSheet`](../../app/lib/presentation/widgets/modal/glass/glass.dart) — mirrors `LocationPollManageMenuSheet`'s pattern via `PollManageMenuSheet`. Conform to `docs/client/modals.md`: pop result back to the caller (don't pass callbacks down), respect the 12px inset / 28px top corners / 92% max-height glass conventions, and use `GlassFooterButtons` if the sheet ever needs a footer. Items:
  - **Edit Details** — caller dispatches to `onToggleEditMode` (existing inline edit-mode flow covers title/description/source URL + background via the cover-photo media picker).
  - **Mark Completed** — caller dispatches to `MarkCompletedModal.show(context, exp.id, communityId)`.
  - **Close Event** — caller dispatches to `CloseItemModal.show(...)` with the existing unshare/cancel/delete plumbing. Mark this row as `destructive: true` so it picks up the red `PollManageMenuSheet` styling.
  Modeling the sheet as a generic `PollManageMenuSheet<ExperienceManageAction>` (or hand-rolled if `PollManageMenuSheet` is too poll-flavored to reuse) keeps the row chrome consistent with the location/time manage sheets.
- [ ] Delete `WorkflowActionButtons.buildExperience` if no other caller remains (grep first); otherwise leave it. Delete the dropdown `ownerMenuItems` list — it is fully replaced by the three-item sheet. The owner-RSVP "Confirm Time / Confirm Location" overflow items are dropped because the rows themselves expose those.
- [ ] If a completed-event "View Impact" entry is desired, add it as an extra row in `ExperienceManageMenuSheet` gated on `isCompleted` — reuse the existing `experienceMenuViewImpact` ARB key (already in both locales).
- [ ] **Restyle the completed-state of the three row pills.** Update `_NeedsHelpPill`, `_LocationVotePill`, and `_TimeChoicePill` ([experience_event_pane.dart:766-920](../../app/lib/presentation/screens/experience/widgets/experience_event_pane.dart)) so that when `chosen` / `helped == true` they render as a translucent confirmation chip instead of a solid sage fill: low-alpha sage background, matching-but-stronger sage border, pale sage text/check glyph. The mockup uses `bg-[#87A083]/22 border border-[#87A083]/35 text-[#C8D6BF]` — translate to existing `AppColors.experienceSageGreen.withValues(alpha: 0.22)` for fill and `0.35` for the border; for the text/check use the existing pale-sage token if one exists, otherwise add a single `experienceSageGreenSoftText` token to `app/lib/core/theme/app_colors.dart` (do **not** invent per-section constants — see `feedback_theme_reuse.md`). Pre-action ("Help" / "Choose" / coral solid) state is unchanged. Since all three pills share the same shape and only differ in label, factor the chip into one private widget (e.g., `_RowCompletionPill({required label, required chosen, onTap})`) to remove the ~60 lines of duplication while we're touching the file — helps the 1,000-line ceiling in Phase 3.
- [ ] **l10n.** Three of the four strings needed already exist — **reuse them** rather than minting new keys:
  - "Edit Details" → reuse `experienceMenuUpdateDetails` (already in en/es). If the copy genuinely diverges from the existing menu wording, mint `experienceManageEditDetails`; otherwise reuse.
  - "Mark Completed" → reuse `experienceMenuMarkCompleted`.
  - "Close Event" → reuse `experienceMenuCloseEvent`.
  - The pencil's accessibility label is new — add `a11yExpOpenManageSheet` (and its `@key` description) to both `app_en.arb` and `app_es.arb`. Per `docs/client/i18n.md`: keys must exist in **both** ARB files before code generation, otherwise the build breaks for the missing locale.
  Run `npm run generate` after editing the ARB files.
- [ ] Per `docs/client/architecture.md`: widgets call `context.l10n.*` in `build()`. Do **not** resolve strings in viewmodels or in the manage-action enum's `label` getter — keep the menu definition in the widget layer.
- [ ] Update / add tests in [app/test/presentation/screens/experience/widgets/](../../app/test/presentation/screens/experience/widgets/) per `docs/client/testing.md`: drive via `ProviderContainer` with mocked repositories, not via shortcut helpers. For the manage sheet, assert it renders three items, that destructive styling lands on Close Event, and that each tap pops the expected enum value. For the new RSVP row, assert owner sees stack + RSVP pill; non-owner sees pill state reflecting their `currentUserRsvp`.

**Validates:** Owner can still edit, mark completed, and close the event; non-owner can still RSVP and see attendees; the visual rhythm of the five rows matches the mockup.

### Phase 3: Cleanup
**Goal:** Remove dead code, tighten the file, leave the codebase smaller than we found it.
**Tasks:**
- [ ] Audit [experience_event_pane.dart](../../app/lib/presentation/screens/experience/widgets/experience_event_pane.dart) for unused imports and helper methods (e.g., `_buildRsvpStatus`, `_buildTimeStatus`, `checklist.*` icons) that the menu sheet no longer needs. Delete what's unused. **Hard constraint:** keep this file under the 1,000-line CI gate (`npm run lint:dart:size`, wired into `.github/workflows/test_flutter.yaml`) — it's at 920 today, so we cannot add net lines. If we're trending over, split per `app/lib/presentation/widgets/creation/README.md` patterns (extract row builders into a sibling file). Do **not** add a `// dart-line-count-allow` hatch.
- [ ] If dropping the Plan tab leaves any `setActiveTab(1)` callers that meant "go to Plan", reroute them through the new named `chatTabIndex` accessor.
- [ ] Verify `experience_content_view.dart` still compiles after the action-bar simplification; clean up any unused imports (the `ActionDropdownItem`, `checklist`, `WorkflowActionButtons` imports likely become dead).
- [ ] **Cache invalidation.** Per `docs/client/caching.md`: this redesign is presentation-only — there are no new mutations, so no new `refresh*()` / `invalidate*()` calls are needed. Confirm by reading the new manage-sheet handlers — `onToggleEditMode`, `MarkCompletedModal`, `CloseItemModal` each already trigger the appropriate repository invalidations through the existing viewmodel paths. **Do not add a manual cache wipe** in the new sheet code.
- [ ] **No server changes.** No proto, RPC, storage, or observability work — this issue is pure client. The `docs/server/observability.md` / `docs/server/architecture.md` rules don't apply here. (Flagged because the audit checklist asks; called out so the implementer doesn't go hunting.)
- [ ] Run `npm run generate` and `flutter test --reporter expanded`. Run the CI lints explicitly: `npm run lint:dart:a11y`, `npm run lint:dart:size`, `npm run lint:ephemeral-comments`. Manually verify on iOS simulator and Android emulator via `/ripls-app-logs` that the new sheet opens, the pencil is tappable, RSVP works, the people-pill opens the access sheet, and that the Plan tab is gone for active events while Results stays for completed ones.

**Validates:** No regressions, file-size and a11y CI gates stay green, the codebase is leaner.

## Related Issues
- #1012 — Manage button social proof (relevant context for how the existing Manage menu was thought about).
- The location-poll Manage menu (no issue, but `LocationPollManageMenuSheet` + `PollManageMenuSheet` are the canonical pattern this work should mirror).
