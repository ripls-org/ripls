---
issue: 2001
title: Invite people should not be a toggle
---

# Issue #2001: Invite people should not be a toggle

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2001
**Filed:** 2026-05-17 | **Last activity:** 2026-05-17
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none
**Related plan:** [docs/issues/1976-invite-and-community-selection.md](./1976-invite-and-community-selection.md) — built the unified `CommunitySelectionSheet` whose UI this follow-up corrects.

> **Audit pass (2026-05-18):** Reviewed against [docs/client/architecture.md](../client/architecture.md), [docs/client/caching.md](../client/caching.md), [docs/client/i18n.md](../client/i18n.md), [docs/client/accessibility.md](../client/accessibility.md), [docs/client/testing.md](../client/testing.md), and the proto/server conventions in [CLAUDE.md](../../CLAUDE.md). Updates appear as **\[Audit]** callouts and as a new "Compliance Requirements" section. Server docs were read but do not apply — this is a client-only UI tweak with no proto, RPC, or repository changes.

## Problem Statement

The "Invite people" flow lets the user pick **one** community to invite to (`CommunitySelectionSheet` in `invite` mode, opened from `InviteSheet` via `showForInvite`). But the row UI is a `Switch` (toggle) — the same widget the multi-select modes (`deferred`, `immediate`) use. Two problems:

1. **Toggle semantics imply persistence and multi-state.** A `Switch` reads as "this community is shared / not shared, you can flip multiple of them." For invite mode the truth is "pick one and commit" — there is no off state from the user's perspective, because tapping pops the sheet immediately with that ID.
2. **Same widget for opposite semantics is misleading.** The invite-multiple-communities-to-an-event flow (`deferred` mode, used from `experience_preview_modal`, `gear_preview_modal`, `request_preview_modal`, `unified_preview_card`) and the access-edit flow (`immediate` mode) genuinely toggle a binary state per row — `Switch` is right there. Invite (single-select) should look different so the user can tell at a glance which kind of dialog they're in.

The right native control for "pick one of these" is a radio indicator. The codebase already has the visual pattern: [`time_proposal_tile.dart:75-118`](../../app/lib/presentation/widgets/experience/time_proposal_tile.dart#L75-L118) renders a radio-style indicator with `transferSage`-tinted selected state, and [`poll_results_view.dart:57-96`](../../app/lib/presentation/widgets/experience/poll_results_view.dart#L57-L96) reuses it. That's the design token we want.

## Context from Comments

The issue has **no comments** yet. The body proposes "same dialog but have a multi-select version and single-select version" — explicitly endorsing the two-variant approach. Filing this plan also gives the issue author a place to push back on the implementation shape.

## Codebase Findings

- **Tile widget under test:** `app/lib/presentation/widgets/sharing/community_toggle_tile.dart:103` renders a `Switch` regardless of selection semantics.
- **Modal that selects the tile:** `app/lib/presentation/widgets/sharing/community_selection_sheet.dart:372-392` (`_buildList`) uses `CommunityToggleTile` for both `invite` and `deferred` modes. The `invite` mode at line 196-199 short-circuits `_onTileChanged` to `Navigator.pop([communityId])` — tapping a tile commits.
- **Existing radio pattern to mirror:** `time_proposal_tile.dart` and `poll_results_view.dart` — circle ring outline when unselected, filled checkmark inside `transferSage` ring when selected.
- **Sole caller of invite mode:** `app/lib/presentation/screens/communities/invite_sheet.dart:101` (`showForInvite`). No other code path opens the sheet in single-select mode today.
- **Tests:** `app/test/presentation/widgets/sharing/community_selection_sheet_test.dart:162-260` already has an `invite mode` group with two cases — extend it rather than write fresh scaffolding.
- **i18n:** No new strings required. Existing `communityPickerInviteTitle` / `communityPickerInviteSubtitle` ARB keys are already invite-specific. Only the a11y semantics label on the tile may need an update from "toggle" wording to "radio" wording — check `app/lib/l10n/app_en.arb` for `a11y` prefixes referenced by `community_toggle_tile.dart` (none today; the semantics label is just the community name from the modal — fine).
- **Bulk modal:** `bulk_community_selection_modal.dart` also uses `CommunityToggleTile`, always in multi-select. No change needed — it stays on the switch UI.

## Potential Solutions

### Option A: Parametrize `CommunityToggleTile` with a `selectionStyle` enum
- **Approach:** Add `enum CommunitySelectionStyle { single, multi }` (or a `bool isSingleSelect`) to `CommunityToggleTile`. When `single`, render the existing `time_proposal_tile`-style radio indicator instead of the `Switch`. `CommunitySelectionSheet._buildList` passes `single` when `mode == invite` and `multi` otherwise. `_buildImmediateContent`'s `CommunityListItem` is unaffected (different widget entirely, already correct for multi).
- **Pros:** One widget, one tile API, smallest diff. Easy to reason about. Tile-level unit testing can cover both renders. Matches the issue author's "same dialog but two versions" wording most directly.
- **Cons:** Tile name still says "toggle" — slightly misleading once it can render as a radio. (Easy follow-up rename if it sticks.)
- **Effort:** Small.

### Option B: Split into `CommunityToggleTile` + new `CommunityRadioTile`
- **Approach:** Extract the avatar / name / subtitle layout into a private base widget and build two thin variants on top: `CommunityToggleTile` (Switch) and a new `CommunityRadioTile` (radio). `CommunitySelectionSheet._buildList` switches on `mode` to pick which tile to render.
- **Pros:** Type-level separation; misuse is harder. Tile name accurately describes what it draws.
- **Cons:** More files, more API surface, two test groups, and the base-widget extraction is the bulk of the work. Net not much safer than A given there's exactly one caller per variant.
- **Effort:** Small-to-medium.

### Option C: Visual tweak only — keep `Switch`, change selected affordance
- **Approach:** Don't change semantics, just colorize the `Switch` differently in `invite` mode (e.g. always off-coloured except for the selected row).
- **Pros:** Minimum code change.
- **Cons:** Doesn't address the user's complaint. A "Switch in a single-select list" remains an a11y/affordance mismatch.
- **Effort:** Trivial.

## Recommendation

**Option A.** Smallest viable change, exactly addresses the complaint, follows the existing radio-tile design pattern in `time_proposal_tile.dart`, and ships in one PR. The "tile name says toggle" cosmetic concern can be handled with a one-commit rename later if we want — it doesn't block the fix.

In the single-select render path, the radio indicator is **purely decorative** — a `Container` with a circle border and an inner check on selection, no inner `Radio`/`Switch` widget. The row's `Toggle` wrapper (at `community_selection_sheet.dart:379-389`) stays the sole tap target. **\[Audit]** This sidesteps the existing minor bug in the multi-select render — the inner `Switch`'s `onChanged` is functionally a duplicate tap surface because the outer `Toggle` already calls `_onTileChanged` on row tap — but we are not asked to fix the multi-select path in this PR; that's a separate cleanup.

## Open Questions

- **Should we rename `CommunityToggleTile` → `CommunitySelectionTile` in the same PR?** — Recommendation: **no, not now.** A rename touches every import (`community_selection_sheet.dart`, `bulk_community_selection_modal.dart`) plus the test file, and the user complaint is the visual mismatch, not the type name. File a follow-up if it grates — a 5-minute rename later.
- **Should `immediate` mode also reconsider the Switch UI?** — Recommendation: **keep Switch.** `immediate` is genuinely multi-select with per-row mutations (gear/request/experience sharing). The Switch communicates the "on/off, this row is independent" semantics correctly there. Different widget entirely (`CommunityListItem`) — out of scope.
- **A11y: what should `semanticsLabel` on the radio row announce?** — Recommendation: keep the community name as the semantics label (matches the current `Toggle` wrapper at `community_selection_sheet.dart:381`). The `Toggle` primitive already wraps the row in `Semantics(selected: ...)`, so "selected" / "not selected" is announced today. **\[Audit]** What `Toggle` is **missing** for true radio-button semantics is the `inMutuallyExclusiveGroup` flag — without it, screen readers announce the row as a "button" rather than a "radio button". Two options: (1) thread an optional `inMutuallyExclusiveGroup` bool through `Toggle` (default false; the invite mode passes true), or (2) wrap the single-select tile in a raw `Semantics(inMutuallyExclusiveGroup: true, selected: ...)` inside `community_selection_sheet.dart:_buildList`. Option (1) is the right place — it generalizes for future single-select uses and keeps a11y primitives centralized per [docs/client/accessibility.md](../client/accessibility.md). Land it in Phase 1.
- **What about `bulk_community_selection_modal.dart`?** — Recommendation: leave alone. It's multi-select. Switches are right.
- **Test scope** — Recommendation: extend `community_selection_sheet_test.dart`'s existing `invite mode` group with one new case asserting `find.byType(Switch)` is empty when the sheet is opened in `invite` mode, and one case asserting `find.byType(Switch)` is non-empty in `deferred` mode (the inverse case, to guard against an accidental swap). **\[Audit]** Do **not** add a tile-level widget test of `CommunityToggleTile` itself — per [docs/client/testing.md](../client/testing.md) ("test business logic, not Flutter widgets") the tile is a pure presentation widget; the sheet-level assertions are the right layer. If a tile-level widget test is added later, it should only check the `inMutuallyExclusiveGroup` semantics flag on the rendered row, which is the one non-trivial behavioral knob.

## Compliance Requirements

**\[Audit]** — derived from `docs/client/*` and `CLAUDE.md`. These apply across the implementation phase below.

### Architecture (MVVM + Riverpod)
- **Pure widget change.** No new ViewModel, no state provider, no repository touch. `CommunityToggleTile` stays a `StatelessWidget`; selection style is a build-time parameter, not state. `CommunitySelectionSheet` keeps its existing `_selected: Set<String>` state and `_onTileChanged` flow — invite mode's pop-on-tap behavior at `community_selection_sheet.dart:196-199` is preserved.
- **No new controllers, no manual cache maps.** The plan does not introduce either; flagged for completeness.

### Caching
- **N/A.** No data layer changes. `CommunityRepository.refreshUserCommunities()` already runs on the "Create new community" path (`community_selection_sheet.dart:179-193`) — that flow stays untouched.

### i18n
- **No new ARB keys required.** `communityPickerInviteTitle` and `communityPickerInviteSubtitle` exist (`community_selection_sheet.dart:334-369`) and remain the single-select title/subtitle. The tile's outer `Toggle` already uses `community.name` (a domain value, not a localizable label) as its `semanticsLabel`. **\[Audit]** Verify by running `npm run lint:dart:i18n` after the change — no new literal strings should appear in `community_toggle_tile.dart` or `community_selection_sheet.dart`.
- **Per [docs/client/i18n.md](../client/i18n.md), ViewModels never resolve strings.** Not relevant here — no ViewModel touched.

### Accessibility
- **Use accessibility primitives.** Both render paths route through the existing `Toggle` primitive. No raw `GestureDetector` / `InkWell` introduced.
- **\[Audit] Radio semantics.** Single-select rows must announce as radio buttons, not buttons. Achieve this by threading `inMutuallyExclusiveGroup` through `Toggle` (preferred — keeps primitives centralized per [docs/client/accessibility.md](../client/accessibility.md)) and passing `true` from invite mode. The accompanying ARB-key changes: none — the existing `community.name` as `semanticsLabel` is correct.
- **Lint baseline (`scripts/check_a11y.js`).** `a11y_allowlist.txt` is empty. The implementation must not introduce raw `GestureDetector`/`InkWell` (it won't, since the radio indicator is decorative and the tap target is the existing `Toggle`). `prefer_accessible_modal` already passes (`showAccessibleModal` is used at `community_selection_sheet.dart:90, 117, 141`).
- **Reduce-motion.** No new `AnimationController` or `AnimatedX` — N/A.

### Observability
- **No new analytics events.** `CommunitySelectionOpenedEvent` (`community_selection_sheet.dart:171-175`) already captures `source: 'invite'` so we can verify the radio variant is actually being rendered when invite users open the modal. Per [docs/client/observability.md](../client/observability.md), no UI-only change warrants a new event unless we want to A/B-test confusion rates — out of scope.

### Testing (per [docs/client/testing.md](../client/testing.md))
- **Sheet-level widget tests are the right layer.** `community_selection_sheet_test.dart` already exercises `showForInvite` (line 162-260). Add invite-mode and deferred-mode regression assertions on the presence/absence of `Switch` to lock the visual contract in. Do not add a `CommunityToggleTile`-level widget test — testing.md explicitly steers away from framework-level widget tests when business logic is unchanged.
- **No ViewModel test needed.** No ViewModel is touched.

## Implementation Plan

### Phase 1: Parametrize the tile, wire the modal, update tests
**Goal:** Single-select invite mode renders a radio indicator (with proper radio-button a11y semantics) instead of a Switch; multi-select modes are unchanged.

**Tasks:**
- [ ] Add `enum CommunitySelectionStyle { single, multi }` to `community_toggle_tile.dart`. Default to `multi` for backward compat.
- [ ] When `single`, replace the `Switch` branch with a radio indicator modeled on `time_proposal_tile.dart` (circle outline → filled check on selected, `transferSage` ring color). Indicator is decorative — no inner tap target.
- [ ] In `community_selection_sheet.dart:_buildList`, pass `single` when `widget.mode == CommunitySelectionMode.invite`, `multi` otherwise. `_buildImmediateContent` is untouched (different widget).
- [ ] Add an optional `bool inMutuallyExclusiveGroup = false` parameter to the `Toggle` accessibility primitive (`app/lib/presentation/widgets/accessibility/toggle.dart`) and forward it to `Semantics(...)`. Pass `true` from `community_selection_sheet.dart:_buildList` when `widget.mode == CommunitySelectionMode.invite`.
- [ ] Extend `community_selection_sheet_test.dart`'s `invite mode` group with: (a) regression test asserting `find.byType(Switch)` is empty in invite mode; (b) inverse test asserting `find.byType(Switch)` is non-empty in `deferred` mode to guard against an accidental swap.
- [ ] Run `npm run lint:dart` + `npm run lint:dart:i18n` + `npm run lint:dart:a11y` locally and confirm green.
- [ ] Manual verification on the Pixel: open `InviteSheet` from `manage_members_screen`, confirm radio appearance; open `experience_preview_modal` community picker, confirm Switch appearance is unchanged.
- [ ] Optional manual a11y verification with TalkBack: invite-mode row announces as "radio button, selected"/"radio button, not selected"; deferred-mode row announces as "button" + selected state. If TalkBack run is not feasible, file a follow-up issue rather than block the PR — per [docs/client/accessibility.md](../client/accessibility.md), screen-reader smoke tests are not yet a routine signal.

**Validates:** The user's complaint is closed — visual paradigm differs between single-select (invite) and multi-select (event-sharing) modes. Radio rows also announce correctly to assistive tech. No regression in multi-select callers.

### Phase 2 (optional follow-up): Rename and unify naming
**Goal:** Tile name reflects what it draws.

**Tasks:**
- [ ] File a follow-up issue: rename `CommunityToggleTile` → `CommunitySelectionTile`. Tracked separately; only landed if a reader trips on the name during Phase 1 review.

**Validates:** Type names accurately describe their behavior. Pure ergonomics — not user-facing.

## Related Issues
- #1976 — built the unified `CommunitySelectionSheet` that this fix sits on top of. The original plan's open question #3 explicitly called out the single-vs-multi distinction; this issue is the user-feedback confirmation that the chosen UI didn't make that distinction visible enough.
- #1991 / PR #2014 — recent unrelated fix in the same area (`location_picker_modal`) that surfaced a similar "same widget for two different semantics" complaint; not a dependency, but a useful precedent for "one widget, two modes, distinguished by a typed parameter."
