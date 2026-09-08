# Glass modal primitives

Frosted-glass building blocks for bottom-sheet modals. Every in-scope
bottom-sheet modal in the app composes from the widgets in this directory.

## What lives here

| File | Purpose |
|------|---------|
| `glass_surface.dart` | The foundational blur + translucent fill + hairline border primitive. Other widgets stack on top. Set `useBlur: false` to skip the `BackdropFilter` on low-end Android. |
| `glass_sheet.dart` | The full bottom-sheet container — scrim, sheet inset, drag handle, top-rounded corners. **Content-only** — does not call `showModalBottomSheet`. Wrap it inside `showAccessibleModal` / `ModalHelpers.showStandardModal()`. |
| `glass_modal_header.dart` | Icon badge + UPPERCASE kicker label + single-line value. The value is wrapped in `Semantics(header: true)` so it announces as the modal heading. |
| `glass_field_label.dart` | UPPERCASE field label (e.g. "DATE", "START TIME") above a chip row or inline action. |
| `glass_chip.dart` | Two-line selectable chip (primary line + optional secondary). Uses `Toggle` for accessibility and `accessibleDuration` for press-scale feedback. |
| `glass_inline_action.dart` | 44px translucent row with a leading icon, label, and trailing chevron — used for "Pick another date…" / "Custom time…" fall-through actions. |
| `glass_footer_buttons.dart` | Cancel ghost + coral primary action bar, separated from the sheet body by a 1px divider. |
| `glass_search_input.dart` | White-on-glass `TextField` for autocomplete-style modals. |
| `glass_inset_card.dart` | Card shape for content **inside** a `GlassSheet` (transfer-flow shared cards, etc.). Flat `white/8` alpha fill, no inner `BackdropFilter` — avoids the recursive-blur artifact iOS+Metal exhibits when `BackdropFilter`s nest. Presentational by default; composes `Tappable` when `onTap` is passed. |
| `glass.dart` | Barrel file re-exporting every widget. |

## Design contract

- **Colors:** every color comes from `AppColors.modal*` — no `Color()`
  literals in this directory.
- **Geometry & typography:** every magic number comes from `ModalTheme` —
  no hardcoded radii, sizes, or font weights.
- **Light/dark identical:** the glass surface is white-on-translucent in
  both themes by design (the dark scrim handles theme adaptation). This is
  documented in `docs/client/design.md` as an explicit deviation from the
  theme-aware-color rule.
- **Accessibility:** every interactive surface uses `Tappable`, `Toggle`,
  or `IconAction` — no raw `GestureDetector` or `InkWell`. Every animation
  `Duration` is wrapped in `accessibleDuration(context, ...)`. All
  user-visible strings come from `context.l10n`.

## When to add a new widget here

Only when the same shape is needed across **two or more** glass modals.
A modal-specific layout stays in the modal file — don't preemptively
hoist patterns. See `docs/issues/1797-glass-modal-revamp.md` §Phase 1
for the original primitive set.


## Reference

- Visual target: `docs/concepts/modals/time-modal.png`
- Design source: `docs/cowork/App Design/time-modal-standalone.html`
- Plan: `docs/issues/1797-glass-modal-revamp.md`
