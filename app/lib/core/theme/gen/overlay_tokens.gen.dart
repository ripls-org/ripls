// GENERATED from design/tokens.json by scripts/gen_design_tokens.js — DO NOT EDIT.
// Edit design/tokens.json and run `npm run generate:design-tokens`. CI
// (`npm run lint:design-tokens`) fails if this file drifts from the source.

import 'dart:ui';

/// Media-overlay material — scrims and controls painted onto user photos. (issue #2770).
///
/// A different material from glass: glass floats above content, this is a
/// wash on the media itself. Not per-theme — the photo is the backdrop and
/// does not follow the app theme.
///
/// Consumed by `AppColors`, which keeps the semantic names widgets already
/// call pointing here — widgets should keep using those rather than reaching
/// for these directly.
class OverlayTokens {
  OverlayTokens._();

  /// Top of the hero gradient, behind the status bar and top controls.
  static const Color scrimTop = Color(0x80000000);

  /// Bottom of the hero gradient, behind the title block and metadata.
  static const Color scrimBottom = Color(0xCC000000);

  /// Flat wash over media where no gradient applies. Was 30% in light and 50% in dark; unified, because the media is the backdrop and does not follow the app theme.
  static const Color scrimFlat = Color(0x80000000);

  /// Minimum wash under ANY text on media. The shipped gradients fade to fully transparent between their stops, leaving mid-hero text on bare photo. 55% is not a taste call: white text needs a composite no lighter than grey 119 to clear 4.5:1, and over a white photo that requires at least 53% black. An earlier 35% floor measured 3.07:1 on the gear title. This is the price of assuming the worst photo — luminance-aware material selection is what would let it be conditional.
  static const Color scrimFloor = Color(0x8C000000);

  /// Where a hero's content wash meets the top of the editorial sheet. The wash is sized to the CONTENT, not the frame, so this is the darkest the photo gets at the sheet's upper edge and everything above the run-up stays bare photograph. It has to be reached AT that edge rather than partway down: the title starts within ~16px of it, and the shipped gradient — which put transparent at the edge and 65% a fifth of the way in — left the title on roughly 10% black, measuring 3.07:1. 72% rather than that 65%: on the brightest hero photo a metadata row landed on a near-white patch and measured 3.94:1, and clearing 4.5:1 there needs 11.2% more black than 65% supplies. The threshold is 70%; the extra 2 points are margin. It costs little because everything it darkens is behind the sheet.
  static const Color washTop = Color(0xB8000000);

  /// Bottom of a hero's content wash, under the deepest metadata rows.
  static const Color washBottom = Color(0xF2000000);

  /// Borders for chips, fields and controls on media. Collapses three separately-maintained white 30% values.
  static const Color outline = Color(0x4DFFFFFF);

  /// Metadata and status chips on media.
  static const Color chipFill = Color(0x33FFFFFF);

  /// Floating action controls on media.
  static const Color controlFill = Color(0x29000000);

  /// Text fields shown over media.
  static const Color fieldFill = Color(0x4D000000);

  /// Primary text on media.
  static const Color textPrimary = Color(0xFFFFFFFF);

  /// Secondary text on media. Media text must come from THIS ramp, not the palette's — the palette's neutral steps are validated against `background`/`surface` and measure as low as 2.71:1 out here.
  static const Color textSecondary = Color(0xE6FFFFFF);

  /// De-emphasised text on media. Never body copy.
  static const Color textFaint = Color(0xB3FFFFFF);
}
