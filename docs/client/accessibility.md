---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: How the Flutter app stays accessible by default — accessibility primitives (Tappable, Toggle, IconAction, LiveRegion), l10n-sourced semantic labels, and CI-enforced a11y lint rules in scripts/check_a11y.js.
  globs: [app/lib/presentation/widgets/accessibility/**, scripts/check_a11y.js]
  triggers: [accessibility, a11y, semantics, screen-reader, voiceover, talkback, reduce-motion, semantic-label]
  lens: [client, accessibility]
  domain: client
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Accessibility

How the Flutter app stays accessible by default. This document is the contributor reference; the multi-phase plan that produced these mechanisms lives at [`docs/ai/accessibility_plan.md`](../ai/accessibility_plan.md).

The short version: every new interactive widget composes from the primitives under `app/lib/presentation/widgets/accessibility/`, every accessibility-visible string comes from `context.l10n.*`, and CI rejects PRs that introduce new violations.

## When to reach for which primitive

| Need | Primitive | Notes |
|------|-----------|-------|
| Tap that performs an action | `Tappable` | Replaces `GestureDetector` / `InkWell`. Required `semanticsLabel`. |
| Tap that toggles a selectable state | `Toggle` | RSVP buttons, filter chips, period togglers, segmented controls. |
| Icon-only button | `IconAction` | Replaces `IconButton`. Required `semanticsLabel` and `tooltip`. |
| Image that conveys meaning | `CachedMediaImage(semanticsLabel:)` | Pass a content-specific label like `'Photo of ${gear.name}'`. Omit for purely decorative images. |
| Status content (banner, error) that stays on screen | `LiveRegion` | Wraps the visible widget. Re-announces on change. |
| One-shot status announcement | `SemanticAnnouncer.announce(context, msg)` | Use for "Loaded", "Copied to clipboard". |
| Animation duration on `AnimationController`, `AnimatedX` | `accessibleDuration(context, dur)` | Returns `Duration.zero` when reduce-motion is on. |
| Showing a bottom sheet | `showAccessibleModal` | Drop-in for `showModalBottomSheet`; restores focus on dismiss. |
| Modal title | `ModalHeader` | Already wraps the title in `Semantics(header: true)`. |

The widgets live in [`app/lib/presentation/widgets/accessibility/`](../../app/lib/presentation/widgets/accessibility/) — see that directory's README for API details.

## Writing good accessibility labels

- **Always pull from `context.l10n`.** The `require_l10n_for_semantic_labels` lint rule rejects raw string literals on `tooltip:`, `semanticLabel:`, or arguments inside `Semantics(...)`. New keys go in `app_en.arb` with the `a11y` prefix (e.g. `a11yClose`, `a11yShowPassword`).
- **Describe the element, not the action.** "Close" is right; "Tap to close" is redundant — screen readers already announce the role ("button"). Reserve verbs for cases where the action is genuinely non-obvious.
- **State first when state matters.** For toggles, set `selected:` instead of embedding "selected" / "not selected" into the label. For password toggles, the label should describe what the *next tap will do*: `a11yShowPassword` when the password is hidden, `a11yHidePassword` when visible.
- **Keep labels short.** Screen readers re-read them often. A label longer than a tweet is too long.

## Lint rules (`scripts/check_a11y.js`)

`npm run lint:dart:a11y` walks `app/lib/presentation/` and applies these rules:

| Rule | Catches |
|------|---------|
| `avoid_raw_gesture_detector` | `GestureDetector` / `InkWell` with `onTap` outside `widgets/accessibility/`. |
| `require_semantic_label_on_icon_button` | `IconButton(...)` without a `tooltip:` argument. |
| `require_l10n_for_semantic_labels` | Raw string literal on `tooltip:`, `semanticLabel:`, or `label/value/hint` inside a `Semantics(...)` constructor. |
| `require_semantic_label_on_cached_media_image` | `CachedMediaImage(...)` without `semanticsLabel:`. |
| `prefer_accessible_modal` | Raw `showModalBottomSheet` outside `widgets/accessibility/show_accessible_modal.dart`. |
| `prefer_accessible_duration` | `duration: Duration(...)` argument not wrapped in `accessibleDuration(context, ...)`. |
| `avoid_color_only_status` | A class named `*Dot`, `*Indicator`, or `*Badge` that contains no `Semantics(` or `Tooltip(`. |

CI runs the script on every PR via `.github/workflows/test_flutter.yaml`.

## The allowlist

`scripts/a11y_allowlist.txt` is currently **empty** — every accessibility rule passes across the entire `app/lib/presentation/` tree. The allowlist exists as a CI-enforced ratchet: it may shrink but never grow on `main`. Every PR that introduces a violation must either fix it or remove a different existing entry to make room. Since the file is empty, in practice every PR must fix any new violation.

Each line (when present) is `<rule_name> <relative_path_to_file>` — a single file/rule pair, not a blanket exemption.

If a hotfix legitimately needs to add an exemption:

1. Make the change.
2. Refresh the allowlist with `node scripts/check_a11y.js --update-baseline`.
3. Remove an existing entry in the same PR (any file you also remediate works). With an empty allowlist this is functionally "fix the violation in the same PR".

CI will only pass if the net entry count stayed flat or shrank.

## Adding a new lint rule

Rules live in `scripts/check_a11y.js`. Each rule is a function `(filePath, src) => violations[]`. Add the function, append it to the `RULES` array, and write a smoke test by introducing a file the rule should catch and confirming `node scripts/check_a11y.js` exits non-zero.

The script uses a small custom comment-and-string stripper before applying regexes, so rules can pattern-match on identifiers without worrying about coincidences inside strings or doc comments.

## Manual screen-reader testing

We don't currently drive the app with VoiceOver / TalkBack on a routine basis — the static checks above are the primary signal. A smoke-test pass is tracked as a follow-up issue. If you want to drive it yourself:

- iOS VoiceOver: Settings → Accessibility → VoiceOver, then triple-tap the side button to toggle.
- Android TalkBack: Settings → Accessibility → TalkBack.
- Flutter's `showSemanticsDebugger: true` in `MaterialApp` adds a visual overlay of the semantics tree.

## See also

- [`docs/ai/accessibility_plan.md`](../ai/accessibility_plan.md) — full multi-phase plan.
- [`app/lib/presentation/widgets/accessibility/README.md`](../../app/lib/presentation/widgets/accessibility/README.md) — primitive API details.
- [`app/lib/l10n/README.md`](../../app/lib/l10n/README.md) — l10n key conventions, including the `a11y` prefix.
