// GENERATED from design/tokens.json by scripts/gen_design_tokens.js — DO NOT EDIT.
// Edit design/tokens.json and run `npm run generate:design-tokens`. CI
// (`npm run lint:design-tokens`) fails if this file drifts from the source.

import 'dart:ui';

/// Completion/fulfillment sheet brand colours. (issue #2770).
///
/// Small on purpose: these sheets render on always-dark glass (#2492), so
/// their surfaces, text and borders come from GlassTokens. Only the sheet
/// base and the three brand hues are their own.
///
/// Consumed by `AppColors`, which keeps the semantic names widgets already
/// call pointing here — widgets should keep using those rather than reaching
/// for these directly.
class CompletionTokens {
  CompletionTokens._();

  /// Opaque base of the completion sheet, under the glass layers.
  static const Color sheet = Color(0xFF1A1210);

  /// The confirm/success brand green for these sheets. Deliberately not the palette's `success` — that one is validated against a white page, this sits on a dark sheet.
  static const Color green = Color(0xFF5B8C5A);

  /// Lighter partner to `green`, for gradients and hover states.
  static const Color greenLight = Color(0xFF7FB87E);

  /// Warm accent for celebratory moments in the completion flow.
  static const Color accentLight = Color(0xFFF4A97D);
}
