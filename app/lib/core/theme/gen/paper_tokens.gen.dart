// GENERATED from design/tokens.json by scripts/gen_design_tokens.js — DO NOT EDIT.
// Edit design/tokens.json and run `npm run generate:design-tokens`. CI
// (`npm run lint:design-tokens`) fails if this file drifts from the source.

import 'dart:ui';

/// Impact-paper material — the warm printed-report surface of the impact and chart views. (issue #2770).
///
/// Opaque and light-only, unlike glass and overlay: the impact views have no
/// dark treatment today. The category-* ramp is a SEMANTIC scale, not decoration
/// — the same category must read as the same colour on every screen.
///
/// Consumed by `AppColors`, which keeps the semantic names widgets already
/// call pointing here — widgets should keep using those rather than reaching
/// for these directly.
class PaperTokens {
  PaperTokens._();

  /// The page itself — warm cream. Collapses #FFF9F0 and #FAF8F3, which differ from it by 2-5 and were never distinguishable.
  static const Color surface = Color(0xFFFDF8F0);

  /// Inset cards and table rows on the page. Collapses #F0ECE3.
  static const Color surfaceRaised = Color(0xFFF5F2EC);

  /// Standard rule around cards and tables.
  static const Color border = Color(0xFFE4DFD4);

  /// Internal dividers, lighter than `border`.
  static const Color borderSubtle = Color(0xFFEDE8E0);

  /// Emphasised edge — section boundaries and chart axes.
  static const Color borderStrong = Color(0xFFE0D4BE);

  /// Body and figures. Collapses #3D3A36.
  static const Color textPrimary = Color(0xFF2D2A26);

  /// Supporting copy and labels.
  static const Color textSecondary = Color(0xFF665B46);

  /// De-emphasised labels, units, captions. Collapses #7A756D (distance 17) and #9E9280 — three names that read as one weight.
  static const Color textMuted = Color(0xFF8A7968);

  /// Disabled and placeholder text. Clears 3:1 on `surface`, not 4.5:1 — never body copy.
  static const Color textFaint = Color(0xFFA9A49C);

  /// Deepest ink, for headline figures. Collapses #261F14.
  static const Color ink = Color(0xFF1C1810);

  /// Chart/accent scale.
  static const Color accentCoral = Color(0xFFE67E50);

  /// Chart/accent scale.
  static const Color accentSage = Color(0xFF9DB88A);

  /// Chart/accent scale.
  static const Color accentGold = Color(0xFFD4A574);

  /// Chart/accent scale — deeper than sage. Collapses #6B8758, 26 away.
  static const Color accentGreen = Color(0xFF6B8F71);

  /// Chart/accent scale — the emissions metric's identity, a saturated leaf green. NOT folded into `accent-green`: it sits 41 away, and `impact_tab_bar_test.dart` pins it as a distinct identity by name ('forest').
  static const Color accentForest = Color(0xFF4C8A4A);

  /// Light end of the forest gradient; `accent-forest` is the dark end. Gradients need both ends named or half the gradient silently stops following the theme.
  static const Color accentForestLight = Color(0xFF6AAD68);

  /// Chart/accent scale. Collapses #3D9A8B.
  static const Color accentTeal = Color(0xFF268080);

  /// Light end of the teal gradient; `accent-teal` is the dark end.
  static const Color accentTealLight = Color(0xFF3AA0A0);

  /// Chart/accent scale. Collapses #7DB8D4.
  static const Color accentBlue = Color(0xFF5B8FB9);

  /// Chart/accent scale.
  static const Color accentBronze = Color(0xFF786A50);

  /// Metric-card accent. This is Material green 800 verbatim and sits 41 from `accent-forest` — it reads as a stray framework default rather than a chosen member of this palette, and is a candidate for folding into `accent-forest`. Tokenised at its shipped value so that stays a design decision instead of a side effect.
  static const Color accentGreenStrong = Color(0xFF2E7D32);

  /// Metric-card accent. Material blue 800 verbatim, 82 from `accent-blue` — same caveat as `accent-green-strong`.
  static const Color accentBlueStrong = Color(0xFF1565C0);

  /// Pale amber wash behind a highlighted metric card. A tint, not an accent — it is a background and never carries text contrast on its own.
  static const Color tintAmber = Color(0xFFFFF8E1);

  /// Pale teal wash behind a highlighted metric card.
  static const Color tintTeal = Color(0xFFE4F3F3);

  /// Celebration and milestone emphasis. Collapses the #C49A2A gold.
  static const Color highlight = Color(0xFFE8A040);

  /// Warning-weight emphasis on paper — deliberately distinct from `highlight`, which is positive.
  static const Color ember = Color(0xFFD07030);
}
