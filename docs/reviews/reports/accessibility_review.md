# Accessibility Review - 2026-04-28

## Executive Summary

Since the previous review on 2026-04-24, the team has executed an extraordinarily comprehensive accessibility remediation effort, landing all six phases of the `accessibility_plan.md` (foundation primitives, lint enforcement, auth flow, transfer, impact metrics/charts, and a parallel-agent long-tail sweep). The accessibility surface of the Flutter app has gone from "growing inaccessible" to a codebase-wide accessible-by-default posture in 96 hours: every previously-reported Critical issue is fixed, the Phase 2 lint allowlist is empty (zero violations across all rules), and a strong set of accessibility primitives (`Tappable`, `Toggle`, `IconAction`, `LiveRegion`, `SemanticAnnouncer`, `accessibleDuration`, `showAccessibleModal`, `ModalHeader` with `header: true`) is now mandatory for new interactive UI via `scripts/check_a11y.js` wired into CI.

Concrete progress: `CachedMediaImage` now accepts and applies `semanticsLabel`; `LiveRegion` wraps form errors and the connectivity banner in auth and content shells; password-visibility toggles are migrated to `IconAction` with localized labels; chart widgets accept `semanticsLabel` and `ExcludeSemantics`-wrap their visual; `ConfidenceDot` exposes High/Moderate/Low confidence text; the `MilestoneCard` dismiss target is 48dp; `RSVPToggleButtons` and `InputModeToggle` use `Toggle` with `selected:`. The only residual gap meaningful enough to file is a narrow lint-rule mismatch: `require_l10n_for_semantic_labels` matches the property name `semanticLabel` (Flutter's icon-level spelling) and `Semantics(label:|value:|hint:)`, but does not match the `semanticsLabel:` argument used by the team's own primitives (`Tappable`, `Toggle`, `IconAction`, `CachedMediaImage`). A handful of hardcoded English literals slip through this gap in production code today. The previously-flagged `SectionHeader` 10px low-contrast issue and the `lightTextTertiary` color token remain unchanged but are explicitly deferred per the plan to a later design-token pass.

**Overall Accessibility Posture**: Strong (trend: Sharply Improving) — A high-quality, durably-enforced accessibility floor is now in place. The remaining items are polish and one specific lint sharpening, not structural gaps.

## Changes Since Last Review

**Previous Review Date**: 2026-04-24 (4 days ago)

**Remediated Issues:**

- ✅ **CachedMediaImage now supports `semanticsLabel`** (was 142 days old)
  - Location: `app/lib/presentation/widgets/chat/cached_media_image.dart:32-60`
  - Fixed in: Phase 1 foundation primitives

- ✅ **No live region or dynamic content announcements** (was 142 days old)
  - Location: `app/lib/presentation/widgets/connectivity_banner.dart:40`, `app/lib/presentation/widgets/content/content_error_banner.dart:34`, `app/lib/presentation/screens/auth/login_screen.dart:259`, `app/lib/presentation/screens/auth/register_screen.dart:494`
  - Fixed in: Phase 3 (auth flow & global status); `LiveRegion` primitive added in Phase 1

- ✅ **Form errors not announced** (was 142 days old)
  - Location: `login_screen.dart:259`, `register_screen.dart:494`, `content_error_banner.dart:34`
  - Fixed in: Phase 3

- ✅ **Password visibility toggles missing semantic labels** (was 45 days old)
  - Location: `login_screen.dart:229-234`, `register_screen.dart:404-411`, `register_screen.dart:461-467`, `reset_password_screen.dart:110`, `reset_password_screen.dart:143`
  - Fixed in: Phase 3 (now uses `IconAction` with `context.l10n.a11yShowPassword/a11yHidePassword`)

- ✅ **Impact Metrics zero accessibility coverage** (was new Critical)
  - Location: `app/lib/presentation/widgets/impact/impact_metric_card.dart:28-30` (`Tappable`), `impact_tab_bar.dart:147` (`Toggle`), `period_toggle_bar.dart:77` (`Toggle`), `sharing_impact_card.dart:162` (`Tappable`), `loan_history_card.dart:81-83` (`Semantics` + `Tappable`)
  - Fixed in: Phase 5

- ✅ **ConfidenceDot color-only confidence** (was new High)
  - Location: `app/lib/presentation/widgets/impact/confidence_dot.dart:23-44`
  - Fixed in: Phase 5 (now `Semantics(label: a11yConfidenceHigh/Moderate/Low)`)

- ✅ **Chart widgets have no text alternatives** (was Medium)
  - Location: All 6 widgets in `app/lib/presentation/widgets/charts/` now accept `final String? semanticsLabel`, wrap with `Semantics(label:, child: ExcludeSemantics(...))`; e.g. `ring_progress_widget.dart:22-58`
  - Fixed in: Phase 5

- ✅ **Transfer feature zero accessibility coverage** (was new Critical)
  - Location: `chat_button.dart:27-28` (`Tappable` + `a11yOpenConversation`), `transfer_modal_shell.dart:39` (`Semantics` on loading indicator), `transfer_modal_header.dart:27-29` (`IconAction` + `a11yBack`), `transfer_progress_bar.dart:26` (`Semantics`)
  - Fixed in: Phase 4

- ✅ **MilestoneCard dismiss touch target below 48dp** (was new High)
  - Location: `app/lib/presentation/widgets/impact/community/milestone_card.dart:133-145` (now `BoxConstraints(minWidth: 48, minHeight: 48)`)
  - Fixed in: Phase 5

- ✅ **RSVPToggleButtons and InputModeToggle missing selection semantics** (was High)
  - Location: `rsvp_toggle_buttons.dart:161` (`Toggle`), `input_mode_toggle.dart:78-86` (`Toggle` with `selected:`)
  - Fixed in: Phase 6b (long-tail sweep)

- ✅ **ModalHeader does not use `Semantics(header: true)`** (was Medium, 45 days old)
  - Location: `app/lib/presentation/widgets/modal/modal_header.dart:29` (now wraps title in `Semantics(header: true, child: Text(...))`)
  - Fixed in: Phase 1

- ✅ **No reduced-motion support** (was Medium)
  - Location: `app/lib/presentation/widgets/accessibility/accessible_duration.dart` defines the helper; adopted in `milestone_card.dart:52`, `system_pill.dart:88,120`, `swipe_to_close_mixin.dart:95-96`, `plus_button_modal.dart:242,364,371,391`, `progress_timeline.dart:151,167,282`, `streaming_skeleton.dart:47,130`
  - Fixed in: Phase 6a (mechanical sweep)

- ✅ **Modal focus restoration** (was Medium, 142 days old)
  - Location: `app/lib/presentation/widgets/accessibility/show_accessible_modal.dart` provides drop-in; adopted by all production callers (e.g. `unified_time_modal.dart:49`, `event_organizer_modal.dart:52`, `event_joiner_modal.dart:47`)
  - Fixed in: Phase 6a (only remaining `showModalBottomSheet` references are doc strings in `content_sharing_trigger.dart`)

- ✅ **GestureDetector/InkWell without Semantics on `onTap`** (was 174 files)
  - The 8 surviving raw `GestureDetector` files all use drag-only handlers (`onHorizontalDragEnd`, `onVerticalDragUpdate`, swipe-to-close) or are the `Tappable` primitive itself. No raw `onTap` `GestureDetector` calls remain.
  - Fixed in: Phase 6b

- ✅ **UserAvatar status badge lacks semantic description** (was Medium)
  - Location: `app/lib/presentation/widgets/user_avatar.dart:74-81` (composite label via `_badgeSemanticLabel(context, statusBadge!)`)
  - Fixed in: Phase 6b

**Unaddressed Issues:**

- ⚠️ `SectionHeader` 10px font with low-contrast `lightTextTertiary` (~3.3:1 on white)
  - Location: `app/lib/presentation/widgets/impact/section_header.dart:22-31`, `app/lib/core/theme/app_colors.dart:23,44`
  - Original Priority: Medium
  - Current Priority: Medium (explicitly deferred per `accessibility_plan.md` "Resolved decisions" — design-token review pass)
  - Days Open: 146 days
  - Note: This was deferred deliberately because remediation requires visual-hierarchy decisions outside the accessibility plan's scope.

- ⚠️ Skip-navigation links absent on complex screens
  - Original Priority: Low
  - Current Priority: Low (web accessibility convention; debatable value on a Flutter mobile app)
  - Note: Mobile platforms typically address this via assistive-tech rotor / heading navigation; the new `Semantics(header: true)` adoption substantially mitigates the underlying need.

**New Issues:**

- 🆕 **Lint rule `require_l10n_for_semantic_labels` does not cover `semanticsLabel:` (with the trailing 's')** — the regex matches Flutter's icon-level `semanticLabel:` and `Semantics(label:|value:|hint:)` but not the spelling used by the team's own `Tappable` / `Toggle` / `IconAction` / `CachedMediaImage` parameter. As a result, a small number of hardcoded English literals are passing through enforcement today. See "High Priority" below for specifics.

- 🆕 **A handful of hardcoded English `semanticsLabel` literals** in `pickup_details_card.dart`, `loan_history_card.dart`, `input_mode_toggle.dart`. See High #1.

**Improvements:**

- New `app/lib/presentation/widgets/accessibility/` directory with `Tappable`, `Toggle`, `IconAction`, `LiveRegion`, `SemanticAnnouncer`, `accessibleDuration`, `showAccessibleModal`, plus a `README.md` and per-primitive widget tests.
- `scripts/check_a11y.js` runs in CI with seven rules and a strict ratchet (allowlist may shrink, never grow on `main`). The allowlist is currently **0 entries**.
- New `a11y*` localization key prefix in `app_localizations.arb`, with directory-namespaced sub-prefixes (`a11yExp`, `a11yContent`, `a11yChat`, `a11yPortfolio`, etc.) used during the parallel migration.
- PR template now includes an `## Accessibility` section.
- Developer docs at `docs/client/accessibility.md` (per plan).
- 26 files now use `Semantics(...)` directly (up from 18), and ~311 files compose at least one accessibility primitive.

**Trend**: **Sharply Improving** — Six phases of the accessibility plan landed in 4 days, all five Critical issues from the prior review are remediated, and CI now blocks regressions. The team has gone from "feature velocity outpaces accessibility" (prior review) to "accessibility is the default and enforced." The only meaningful remaining gap is a single lint-regex sharpening; everything else is polish.

---

## WCAG Compliance Summary

| Guideline | Level A | Level AA | Notes |
|-----------|---------|----------|-------|
| Perceivable | Largely yes | Mostly yes | All images, charts, icons, color-only indicators now have text alternatives. `SectionHeader` 10px text contrast is the lone outstanding AA gap (deferred to design-token pass). |
| Operable | Largely yes | Largely yes | `Tappable`/`Toggle`/`IconAction` give every interactive element a name, role, value. Modal focus restoration via `showAccessibleModal`. Touch targets remediated. |
| Understandable | Yes | Yes | Form errors announce via `LiveRegion`; auth flow and connectivity banner covered; status messages remediated. |
| Robust | Yes | Yes | CI lint blocks regressions; primitives required for new code; allowlist empty; widget tests assert Semantics tree shape per primitive. |

## Critical Issues

_None this run._

The five Critical issues from the previous review (CachedMediaImage alt text, no liveRegion, form errors silent, Impact Metrics zero coverage, Transfer zero coverage) are all remediated.

## High Priority

### 1. Lint rule misses `semanticsLabel:` (with trailing s) — hardcoded English literals slipping through

**Location**:
- `scripts/check_a11y.js:264-265` — `literalArgRegex = /\b(tooltip|semanticLabel)\s*:\s*('[_]*'|"[_]*")/`
- Production violations the rule misses today:
  - `app/lib/presentation/widgets/transfer/pickup_details_card.dart:116` — `semanticsLabel: 'Edit pickup details'` (also visible-text literal at line 122)
  - `app/lib/presentation/widgets/impact/loan_history_card.dart:171` — `semanticsLabel: 'How we calculated this'`
  - `app/lib/presentation/widgets/creation/input_mode_toggle.dart:60,79` — `_buildToggleButton(label: 'Image', ...)` then `Toggle(semanticsLabel: label, ...)` (the "Image" English literal reaches a screen-reader label).

**Description**: The lint rule that prevents English literals from being passed as accessibility labels matches the Flutter-built-in `semanticLabel:` (no trailing s, used on `Icon` and `Image`) but does not match the parameter name the team's own primitives expose: `semanticsLabel:` (trailing s, used by `Tappable`, `Toggle`, `IconAction`, and `CachedMediaImage`). Because the bulk of the codebase has been migrated to those primitives, this is now the dominant code path for accessibility labels — and it is unenforced. Three call sites with hardcoded English have already accumulated; without the rule fix they will keep accumulating in non-English locales.

**WCAG Criteria**: 3.1.2 Language of Parts (Level AA) — and breaks the project's own l10n-as-accessibility plan.

**Impact**: Spanish-, French-, and other non-English speakers using a screen reader hear English-language labels embedded in an otherwise localized UI. Severity is moderate today (3 known sites, all short labels) but the regression vector is wide open: any new `Tappable(semanticsLabel: 'Foo')` call ships without warning.

**Recommendation**: Tighten the regex to also match `semanticsLabel`. One-line change:

```js
const literalArgRegex =
  /\b(tooltip|semanticLabel|semanticsLabel)\s*:\s*('[_]*'|"[_]*")/;
```

Then run `node scripts/check_a11y.js` to surface the existing offenders, fix them by introducing `a11y*` keys (`a11yEditPickupDetails`, `a11yHowWeCalculatedThis`, `a11yInputModeImage`), and the allowlist can stay at zero. While you're there, consider also adding `semanticsLabel` to the windowed `Semantics(...)` constructor scan at line 294 for completeness — though that path is mostly already covered by the constructor scan since `Semantics` itself uses `label:`, this is the parameter-name-on-arbitrary-widget path that was missed.

## Medium Priority

### 2. `SectionHeader` 10px low-contrast text — unchanged, deferred per plan

**Location**: `app/lib/presentation/widgets/impact/section_header.dart:22-31`, `app/lib/core/theme/app_colors.dart:23` (`lightTextTertiary = 0xFF9A8D86`).

**Description**: Section headers throughout Impact Metrics still render at `fontSize: 10` with `AppColors.textSecondary` (which resolves to `lightTextTertiary` in light mode at ~3.3:1 contrast on white). At 10px (≈ 7.5pt) this falls under the normal-text 4.5:1 threshold for WCAG 1.4.3 AA.

**WCAG Criteria**: 1.4.3 Contrast (Minimum) — Level AA.

**Status**: Explicitly deferred in the accessibility plan ("Resolved decisions" — design-token review pass). Not a regression, not new — included here only for completeness. Not filed as an issue this run.

**Recommendation**: When the design-token pass happens, either bump font size to 12px (which moves the threshold to large-text 3:1 in some interpretations, and is more legible regardless) or darken `lightTextTertiary` toward `0xFF706560` (~ 5:1 on white).

## Low Priority

### 3. Three legitimate `GestureDetector` exemptions are no longer in the allowlist

**Location**: `app/lib/presentation/screens/portfolio/portfolio_metrics_screen.dart:129`, `app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart:139`, `app/lib/presentation/widgets/swipe_to_close_wrapper.dart:33`, `app/lib/presentation/widgets/keyboard_dismiss_wrapper.dart`, `app/lib/presentation/widgets/discover/discover_bottom_sheet.dart:158`, `app/lib/presentation/widgets/content/content_view_pane_switcher.dart:37`, `app/lib/presentation/widgets/impact/completion/impact_edit_chip.dart:178`.

**Description**: Several `GestureDetector` / `InkWell` usages remain. All inspected cases are drag-only handlers (`onHorizontalDragEnd`, `onVerticalDragUpdate`, swipe-to-close) — exactly the case the rule body documents as legitimate ("Walking the brace depth (not a fixed 30-line window) avoids false positives where an outer drag-only GestureDetector wraps an inner Tappable that has its own onTap"). The lint passes here because the rule only fires on `onTap:` / `onLongPress:`. This is correct behavior, not a finding — surfacing only because the prior report flagged 174 files.

### 4. `mention_chip.dart` passes a literal `'@$displayName'` as `semanticsLabel`

**Location**: `app/lib/presentation/widgets/chat/mention/mention_chip.dart:49`.

**Description**: This is technically a string literal (so could be caught by the sharpened lint above), but the `@` prefix is universal and the substituted `$displayName` carries the user-visible content. Low impact; mention-style labels are reasonable. Worth converting to `context.l10n.a11yMention(displayName)` only if a localized verbalization differs (some screen readers already speak `@` as "at", which may be redundant).

### 5. Visible-text literals near `semanticsLabel` literals

**Location**: `pickup_details_card.dart:122` ("Edit pickup details" again as visible button text), `loan_history_card.dart:175` ("How we calculated this →" as visible link text).

**Description**: These visible-text strings are unrelated to a11y enforcement (they live in the i18n review's domain), but they're co-located with the High #1 finding above and a single fix pass can cover both. Mentioning here so the i18n review owns it but the a11y fix doesn't drop them on the floor.

## Positive Findings

### 1. `widgets/accessibility/` directory — accessible-by-default primitives

`Tappable`, `Toggle`, `IconAction`, `LiveRegion`, `SemanticAnnouncer`, `ModalHeader`, `accessibleDuration`, `showAccessibleModal`. Each has unit tests asserting Semantics tree shape (`app/test/presentation/widgets/accessibility/`). The README documents when to use which. The naming choice (`Tappable` not `AccessibleTappable`) signals that accessibility is the default, not the exception.

### 2. Phase 2 lint enforcement is real and CI-wired

`scripts/check_a11y.js` runs seven rules, and `scripts/a11y_allowlist.txt` is **empty** (only comments). The ratchet ensures the allowlist never grows on `main`. This is the load-bearing infrastructure that prevents future regression.

### 3. Localized accessibility labels everywhere

The `a11y*` l10n key prefix is consistently applied (e.g., `a11yShowPassword`, `a11yHidePassword`, `a11yBack`, `a11yDismiss`, `a11yOpenConversation`, `a11yConfidenceHigh/Moderate/Low`). Directory-namespaced sub-prefixes (`a11yExp`, `a11yContent`, etc.) prevented merge conflicts during parallel-agent migration.

### 4. Reduced-motion support is genuinely adopted

`accessibleDuration(context, ...)` is used at 14+ sites including `milestone_card.dart`, `system_pill.dart`, `swipe_to_close_mixin.dart`, `plus_button_modal.dart`, `progress_timeline.dart`, `streaming_skeleton.dart`. The `swipe_to_close_mixin.dart` pattern of constructing the controller with `Duration.zero` then setting `duration` in `didChangeDependencies` (where `MediaQuery` is available) is correct and worth documenting as canonical.

### 5. Modal focus restoration

`showAccessibleModal<T>` wraps `showModalBottomSheet` and restores focus on dismissal. All production sites have migrated; the only remaining `showModalBottomSheet` references in the source tree are doc strings in `content_sharing_trigger.dart`.

### 6. `CachedMediaImage` correctly handles decorative vs. content-bearing images

When `semanticsLabel` is null, the widget wraps with `ExcludeSemantics` (decorative); when set, it wraps with `Semantics(label:, image: true, child: ExcludeSemantics(child: visual))`. This is the correct double-wrapping pattern that prevents the underlying `CachedNetworkImage`'s default semantics from leaking through.

## Recommendations

### Immediate (this week)

1. **Sharpen `require_l10n_for_semantic_labels`** to also match `semanticsLabel:`. One-line regex update in `scripts/check_a11y.js`. Fix the three known existing offenders (`pickup_details_card.dart`, `loan_history_card.dart`, `input_mode_toggle.dart`) by adding three new `a11y*` ARB keys. Total: ~30-minute change.

### Short-term (next sprint)

2. **Schedule the deferred design-token contrast pass** that was noted in the accessibility plan's Resolved Decisions. `SectionHeader` 10px + `lightTextTertiary` is the canonical example; an audit of all `fontSize: 10`/`fontSize: 11` text against `lightTextTertiary`, `transferTextMuted`, and other muted color tokens would scope the work.

3. **VoiceOver / TalkBack smoke-test follow-up** mentioned as Phase 6c. The static-analysis-driven plan was explicit that live-tester gaps would become follow-up issues, not extensions of Phase 6. Schedule the smoke test now that the foundation is in place.

### Ongoing

4. **Sustain the ratchet.** The plan's success now depends on the allowlist staying at 0 (or shrinking only). The `## Accessibility` PR-template checklist plus the CI ratchet should hold the line, but periodic spot checks of newly-added widget directories are worthwhile.

### Tools and Resources

- `scripts/check_a11y.js` (existing) — run locally with `node scripts/check_a11y.js`; refresh baseline with `--update-baseline`.
- Flutter DevTools accessibility inspector / `showSemanticsDebugger: true`.
- iOS VoiceOver, Android TalkBack — pair with Phase 6c.

---

## Conclusion

This is the strongest single improvement in any single review cycle of this codebase to date. Five Critical issues remediated, six accessibility plan phases shipped in 96 hours, an empty lint allowlist, primitives that make the accessible path the default path, and CI ratchet preventing regression. The accessibility floor is now load-bearing and enforced.

The single remaining concern is narrow and one-line-fixable: the lint rule that enforces "no English literals on accessibility labels" misses the trailing-s spelling that the team's own primitives use. Filing one High issue to close that gap. Everything else in this report is positive findings, deferred-by-plan items, or low-priority polish.

**Next review**: 6–8 weeks. Most useful next pass will be a VoiceOver/TalkBack smoke test (Phase 6c) once the lint sharpening is in.

---

## Issues to File

### [High] a11y lint regex misses `semanticsLabel:` — hardcoded English labels slip through

- **Fingerprint**: `review:accessibility:lint-misses-semanticslabel-spelling`
- **Type**: Bug
- **Why it matters**: `scripts/check_a11y.js`'s `require_l10n_for_semantic_labels` rule matches `tooltip:` and Flutter's icon-level `semanticLabel:` (no trailing s) but not the trailing-s `semanticsLabel:` argument that the team's own primitives (`Tappable`, `Toggle`, `IconAction`, `CachedMediaImage`) expose. Since the codebase migrated nearly all interactive widgets to those primitives in Phase 6, this is now the dominant accessibility-label path — and it is unenforced. Three production sites already pass hardcoded English literals (`'Edit pickup details'`, `'How we calculated this'`, `'Image'`), and any new offender ships without warning. Non-English screen-reader users will hear English fragments mixed into an otherwise localized UI.
- **Locations**: `scripts/check_a11y.js:264-265`, `app/lib/presentation/widgets/transfer/pickup_details_card.dart:116`, `app/lib/presentation/widgets/impact/loan_history_card.dart:171`, `app/lib/presentation/widgets/creation/input_mode_toggle.dart:60,79`
- **Recommendation**: Add `semanticsLabel` to the rule's regex (`/\b(tooltip|semanticLabel|semanticsLabel)\s*:\s*('[_]*'|"[_]*")/`), run `node scripts/check_a11y.js` to surface offenders, add `a11yEditPickupDetails`, `a11yHowWeCalculatedThis`, `a11yInputModeImage` ARB keys, and migrate the three call sites. The allowlist can remain at 0. Total scope: ~30 minutes.
- **Suggested labels**: `accessibility,lint,flutter`
