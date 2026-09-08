# Accessibility primitives

Foundation widgets and utilities that the rest of the app builds on so screen readers, keyboard navigation, reduced motion, and focus management all work by default. New interactive widgets should compose from these — never use raw `GestureDetector`, `InkWell`, `IconButton`, or `showModalBottomSheet` outside this directory.

These primitives are the durable contract Phase 2 lint rules will enforce: anything that bypasses them will fail CI.

## When to use which primitive

| Need | Primitive | Notes |
|------|-----------|-------|
| Tap that performs an action | `Tappable` | Replaces `GestureDetector` / `InkWell`. Required `semanticsLabel`. |
| Tap that toggles a selectable state | `Toggle` | RSVP buttons, filter chips, period togglers, segmented controls. |
| Icon-only button | `IconAction` | Replaces `IconButton`. Required `semanticsLabel` and `tooltip`. |
| Image that conveys meaning | `CachedMediaImage(semanticsLabel:)` | Pass a content-specific label like `'Photo of ${gear.name}'`. Omit for purely decorative images — they're auto-excluded from the semantics tree. |
| Status content (banner, error) that stays on screen | `LiveRegion` | Wraps the visible widget. Re-announces on change. |
| One-shot status announcement | `SemanticAnnouncer.announce(context, msg)` | Use for "Loaded", "Copied to clipboard". Prefer `LiveRegion` when the content stays visible. |
| Animation duration on `AnimationController`, `AnimatedX` | `accessibleDuration(context, dur)` | Returns `Duration.zero` when reduce-motion is on. |
| Showing a bottom sheet | `showAccessibleModal` | Drop-in for `showModalBottomSheet`; restores focus on dismiss. |
| Modal title | `ModalHeader` | Already wraps the title in `Semantics(header: true)`. |

## Writing good `semanticsLabel` text

- **Always pull from `context.l10n`.** Phase 2 lint rejects raw string literals on any `semanticsLabel`, `tooltip`, or `Semantics(label:)` argument. New keys live in `app_en.arb` with the `a11y` prefix (e.g. `a11yClose`, `a11yShowPassword`).
- **Describe the element, not the action.** "Close" is right; "Tap to close" is redundant — screen readers already announce the role ("button"). Reserve verbs for cases where the action is genuinely non-obvious.
- **State first when state matters.** For toggles, set `selected:` instead of embedding "selected" / "not selected" into the label. For password toggles, the label should describe what the *next tap will do*: `a11yShowPassword` when the password is hidden, `a11yHidePassword` when visible.
- **Keep labels short.** Screen readers re-read them often. A label longer than a tweet is too long.

## `semanticsIdentifier` for the e2e harness

`Tappable`, `Toggle`, `IconAction`, and `CachedMediaImage` accept an
optional `semanticsIdentifier:` parameter. When set, it lands on the
primitive's `Semantics` node as the `identifier:` field, which Flutter
Web serializes to the DOM as a `flt-semantics-identifier` attribute.
The Playwright harness (#2162) queries by this attribute.

- Identifiers are **kebab-case, raw strings, not l10n keys**. They are
  not announced by screen readers. A new lint rule
  (`require_kebab_case_for_semantics_identifier`) rejects anything that
  isn't `^[a-z0-9-]+$`.
- Add an identifier **only when a test asks for it**. Don't pre-tag
  widgets speculatively — the identifiers are a stable contract surface
  and unused ones rot.
- Prefer the primitive's `semanticsIdentifier:` parameter over wrapping
  the widget in a standalone `Semantics(identifier: ...)`. One node is
  always cleaner than two.

See [`docs/client/testing/semantics_identifiers.md`](../../../../../../docs/client/testing/semantics_identifiers.md) for the full convention.

## Adding a new primitive

If you find yourself wrapping `Semantics(...)` in feature code, consider whether it belongs here instead. Criteria:

1. The pattern recurs in three or more places.
2. It encodes an accessibility decision the rest of the team should not have to re-make.
3. Its API can require the accessibility-correct path (e.g. non-nullable `semanticsLabel`).

If all three hold, add the widget here, write a test that asserts the Semantics tree shape, and update this README.

## Testing

Each primitive has a widget test under `app/test/presentation/widgets/accessibility/` that asserts the produced `Semantics` node shape — labels, roles (button/link/header), state flags (selected, enabled), and tree exclusions. Use those as templates when adding new primitives.

`Tristate` (from `dart:ui`) is the type returned by some `flagsCollection` properties (`isEnabled`, `isSelected`). Compare to `Tristate.isTrue` / `Tristate.isFalse`, not `true` / `false`.

## See also

- [`docs/ai/accessibility_plan.md`](../../../../../../docs/ai/accessibility_plan.md) — full multi-phase plan, including the Phase 2 lint package and the migration phases that consume these primitives.
- [`app/lib/l10n/README.md`](../../../l10n/README.md) — l10n key conventions and the `a11y` prefix.
