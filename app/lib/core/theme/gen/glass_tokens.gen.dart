// GENERATED from design/tokens.json by scripts/gen_design_tokens.js — DO NOT EDIT.
// Edit design/tokens.json and run `npm run generate:design-tokens`. CI
// (`npm run lint:design-tokens`) fails if this file drifts from the source.

import 'dart:ui';

/// Frosted-glass material — sheets and modals floating above content. (issue #2770).
///
/// One set, not per-theme: the sheet sits on a dark scrim that handles theme
/// adaptation, so on-glass content is identical in light and dark.
///
/// Consumed by `AppColors`, which keeps the semantic names widgets already
/// call pointing here — widgets should keep using those rather than reaching
/// for these directly.
class GlassTokens {
  GlassTokens._();

  /// Backdrop behind the sheet, and the darkening layer inside it over bright media. Collapses the shipped 35% and 40% blacks, which were not distinguishable. Raised from 37% to 42%: the sheet composited to grey 127 on the Material date picker, where white text needs a composite no lighter than grey 119 to clear 4.5:1 and measured 3.94-4.49, and the selected-day circle measured 2.78 against a 3:1 requirement. Ten grey levels clears both. Sized from the measurement, not chosen — a model built from the shipped values predicted the circle at 2.73 against 2.78 observed.
  static const Color scrim = Color(0x6A000000);

  /// Full-screen content panels over a hero image, where the sheet scrim is not enough to keep text legible.
  static const Color scrimHeavy = Color(0xBF000000);

  /// Light tint over media that must stay readable through the overlay.
  static const Color scrimTint = Color(0x40000000);

  /// Translucent fill of the glass sheet.
  static const Color surface = Color(0x26FFFFFF);

  /// Outer edge of the sheet.
  static const Color border = Color(0x4DFFFFFF);

  /// The everyday border on glass. `border` (30%) is the emphatic edge; measured across the app, most real borders sit at 18-20%.
  static const Color borderSoft = Color(0x33FFFFFF);

  /// Border of a selected or active control on glass. Rare but real — without it a selection outline has to reach for `fill-strong`, a FILL token, which is how #2764 started.
  static const Color borderActive = Color(0xCCFFFFFF);

  /// Internal rules and control outlines. Deliberately brighter than surface — the shipped divider was byte-identical to the sheet it divided.
  static const Color divider = Color(0x3DFFFFFF);

  /// Dividers and hairline borders on glass. Equal to `fill-subtle` in value today but a DISTINCT ROLE — a fill and a rule that happen to share a weight will not stay in step when either is retuned. This is not the 'seven names for white' problem that motivated the factoring: those seven were all the same role (text) with no distinction to make. Sized from the code rather than taste — 8 of 9 real dividers draw at 8%, while `divider` says 24%, so routing them there would have tripled every hairline.
  static const Color hairline = Color(0x1AFFFFFF);

  /// The lightest inset fill — barely-there panel tints. 29 call sites cluster at 4-6% and would double in weight if folded into `fill-subtle`.
  static const Color fillFaint = Color(0x0FFFFFFF);

  /// Inset cards, chips, search and inline-action fields. Collapses the shipped 8%/10%/12% whites, which differ by 2 percentage points.
  static const Color fillSubtle = Color(0x1AFFFFFF);

  /// Selected/active control fill.
  static const Color fillStrong = Color(0xFFFFFFFF);

  /// Text and icons on a fill-strong control.
  static const Color onFillStrong = Color(0xFF1A1A1A);

  /// The pill at the top of a sheet.
  static const Color dragHandle = Color(0x59FFFFFF);

  /// Primary on-glass text.
  static const Color textPrimary = Color(0xFFFFFFFF);

  /// Secondary on-glass text. The shipped ramp had four names for solid white, so hierarchy existed in the names and never reached a pixel.
  static const Color textSecondary = Color(0xE6FFFFFF);

  /// The step between `text-secondary` and `text-faint`, for supporting copy that is de-emphasised but still readable. It exists because the ramp had a hole: real call sites cluster at 60-75% and the only options were 90% or 50%, so anything in that band had to round to a weight it was never designed for. Matches `overlay.text-faint` in value, which is deliberate — the same copy often moves between a sheet and a photo.
  static const Color textMuted = Color(0xB3FFFFFF);

  /// De-emphasised on-glass text and chevrons. Never body copy.
  static const Color textFaint = Color(0x80FFFFFF);

  /// On-glass action colour — the LIGHT sage, not the light-theme brand green. Glass is an always-dark material: the sheet sits on a dark scrim regardless of app theme, so a light-theme colour is wrong here by construction. It shipped as #3E5A47 (the light-theme primary) and measured 2.38:1 as a foreground on the sheet; the light sage measures 9.01:1 on the same pixels. That mismatch is what #2764 was. Note this does NOT rescue a brand colour over a mid-grey composite — on the washed photo hero the same element measures 1.50:1 light and 2.52:1 dark, because no brand green works against a mid-tone; that one is a scrim problem.
  static const Color primary = Color(0xFF9DBFA8);

  /// Text and icons on a primary fill. Dark, because `primary` is now light: white on the light sage measures 2.01:1, this measures 8.06:1. The pair has to move together — flipping one and not the other is how a fill and its foreground end up illegible.
  static const Color onPrimary = Color(0xFF14241A);

  /// Secondary (cancel) button fill.
  static const Color secondaryFill = Color(0x33000000);

  /// Secondary button outline, brighter so the pill shape stays unambiguous.
  static const Color secondaryBorder = Color(0x80FFFFFF);

  /// BackdropFilter sigma for the sheet.
  static const double blurSigma = 24.0;

  /// BackdropFilter sigma for the scrim behind the sheet.
  static const double backdropBlurSigma = 4.0;
}
