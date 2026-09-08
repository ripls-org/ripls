// GENERATED from design/tokens.json by scripts/gen_design_tokens.js — DO NOT EDIT.
// Edit design/tokens.json and run `npm run generate:design-tokens`. CI
// (`npm run lint:design-tokens`) fails if this file drifts from the source.

import 'dart:ui';

/// Raw design-token constants (issue #2441). Consumed by `AppColors` /
/// `AppTheme`, which own the semantic, theme-aware layer on top — widgets
/// should keep using those, not reach for these directly.
class DesignTokens {
  DesignTokens._();

  /// Display/heading font family.
  static const String serifFamily = 'Libre Baskerville';

  /// Body/UI font family.
  static const String sansFamily = 'Public Sans';

  /// Fallback stack for [serifFamily].
  static const List<String> serifFallbacks = [
    'Baskerville',
    'Times New Roman',
    'serif',
  ];

  /// Fallback stack for [sansFamily].
  static const List<String> sansFallbacks = [
    '-apple-system',
    'BlinkMacSystemFont',
    'Segoe UI',
    'Roboto',
    'sans-serif',
  ];

  // Light theme.

  /// Page background.
  static const Color lightBackground = Color(0xFFFFFFFF);

  /// Raised surface: cards, sheets, alternating sections.
  static const Color lightSurface = Color(0xFFF2F2EE);

  /// Borders and dividers; cards are bordered, not shadowed.
  static const Color lightBorder = Color(0xFFDDDDD4);

  /// Primary text.
  static const Color lightTextPrimary = Color(0xFF191C19);

  /// Secondary text: body copy, descriptions.
  static const Color lightTextSecondary = Color(0xFF4A524C);

  /// Faint text: placeholders, footnotes, timestamps. Clears 3:1, not 4.5:1 — never for body copy.
  static const Color lightTextFaint = Color(0xFF7C837C);

  /// Deep sage. The only fill for buttons and primary CTAs; also passes as link/text on background.
  static const Color lightPrimary = Color(0xFF3E5A47);

  /// Text and icons on primary fills.
  static const Color lightOnPrimary = Color(0xFFF4F7F1);

  /// Hover/pressed deepening of primary.
  static const Color lightPrimaryHover = Color(0xFF2F4636);

  /// Ink. Emphasis accent: headline accent words (color + underline, never italics), marketing band sections.
  static const Color lightAccent = Color(0xFF1F2421);

  /// Text and icons on accent fills (e.g. marketing bands).
  static const Color lightOnAccent = Color(0xFFF4F4EF);

  /// Success status. Clear green, deliberately distinct from the sage primary.
  static const Color lightSuccess = Color(0xFF2E7041);

  /// Warning status. Amber-brown, distinguishable from both sage and error.
  static const Color lightWarning = Color(0xFF8A6200);

  /// Error status: form errors, destructive actions, failure banners.
  static const Color lightError = Color(0xFFB5492B);

  /// Info status. Stone blue.
  static const Color lightInfo = Color(0xFF3E6471);

  // Dark theme.

  /// Page background.
  static const Color darkBackground = Color(0xFF141714);

  /// Raised surface: cards, sheets, alternating sections.
  static const Color darkSurface = Color(0xFF1F2421);

  /// Borders and dividers; cards are bordered, not shadowed.
  static const Color darkBorder = Color(0xFF323832);

  /// Primary text.
  static const Color darkTextPrimary = Color(0xFFF4F4EF);

  /// Secondary text: body copy, descriptions.
  static const Color darkTextSecondary = Color(0xFFABB3AB);

  /// Faint text: placeholders, footnotes, timestamps. Clears 3:1, not 4.5:1 — never for body copy.
  static const Color darkTextFaint = Color(0xFF7B837B);

  /// Light sage. The only fill for buttons and primary CTAs; also passes as link/text on background.
  static const Color darkPrimary = Color(0xFF9DBFA8);

  /// Text and icons on primary fills.
  static const Color darkOnPrimary = Color(0xFF14241A);

  /// Hover/pressed lightening of primary.
  static const Color darkPrimaryHover = Color(0xFFB1CFBA);

  /// Chalk — the dark-theme counterpart of the ink accent. Headline accent words (color + underline, never italics).
  static const Color darkAccent = Color(0xFFE6E8E1);

  /// Text and icons on accent fills.
  static const Color darkOnAccent = Color(0xFF191C19);

  /// Success status.
  static const Color darkSuccess = Color(0xFF7A9B76);

  /// Warning status.
  static const Color darkWarning = Color(0xFFE8A661);

  /// Error status: lightened from the legacy #D16B4E to clear 4.5:1 on dark surfaces.
  static const Color darkError = Color(0xFFDB7F63);

  /// Info status: lightened from the legacy #5A7A82 to clear 4.5:1 on dark surfaces.
  static const Color darkInfo = Color(0xFF7FA0A9);
}
