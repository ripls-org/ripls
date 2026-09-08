# core/theme

App-wide visual design tokens: colors and text styles.

## Key types

- **`AppColors`** (`app_colors.dart`) — semantic color accessors. Provides static methods (e.g., `AppColors.textPrimary(context)`) that resolve to the correct light- or dark-mode value. Use these instead of hardcoded `Color` literals anywhere in the UI.
- **`AppTheme`** (`app_theme.dart`) — named `TextStyle` constants and font family references. Also contains the `ThemeData` factory used in `main.dart`.

## When to add code here vs. elsewhere

- New **color** needed in multiple widgets → add a semantic accessor to `AppColors`.
- New **text style** shared across screens → add a constant to `AppTheme`.
- Color or style used in exactly one widget → define it locally in that widget file.
- Spacing, padding, or layout constants are not centralized here; define them at the usage site or in the relevant widget.

## Brand accent color

The app's brand accent is **Deep Sage `#3E5A47`** (`AppColors.lightPrimary`, the Ink & Sage palette — see `design/tokens.json`), with a lighter sage `#9DBFA8` for `lightAccent` and the dark-mode inversions (dark theme swaps to the light sage as primary). Several legacy tokens carry the historical name `transferCoral` / `experienceCoral` / `ImpactModalColors.coral` — these now hold sage values and are scheduled for a rename in a follow-up issue. Always reach for the named token; treat the token name as opaque.

## CI enforcement

`scripts/check_no_hardcoded_accent.js` (wired as `npm run lint:dart:colors`) blocks PRs that introduce `Colors.orange` / `Colors.deepOrange` / `Colors.amber` or hardcoded coral / sage hex literals outside this directory. A ratchet allowlist at `scripts/color_allowlist.txt` may shrink but never grow on main. See #1946 for the migration that established this rule.
