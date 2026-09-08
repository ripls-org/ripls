import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';

void main() {
  group('Glass modal contrast tokens (#1934)', () {
    test('primary button fill and its label clear the large-text threshold', () {
      // Asserts the PROPERTY, not the identity. The original form pinned this
      // to `lightPrimary` with a rationale about white-on-coral from the
      // pre-#2441 palette, so it survived the palette change while its stated
      // reason stopped being true, and it then blocked a strictly better
      // pairing.
      //
      // Glass is an always-dark material, so its action colour is now the light
      // sage with dark ink on it (#2770) rather than a light-theme green with
      // white on it. That inverts fill and foreground, which an identity check
      // cannot express. What actually matters is that the pair is legible.
      final ratio = _contrast(
        AppColors.modalPrimaryButtonText,
        AppColors.modalPrimaryButtonBackground,
      );
      expect(
        ratio,
        greaterThanOrEqualTo(3.0),
        reason:
            'Primary CTA label on its own fill must clear WCAG 1.4.11 (3:1). '
            'Measured ${ratio.toStringAsFixed(2)}:1.',
      );
    });

    test('secondary button background is a dark fill, not white-on-white', () {
      final bg = AppColors.modalSecondaryButtonBackground;
      expect(
        bg.r,
        lessThan(0.1),
        reason:
            'Secondary fill must be near-black so white-on-glass text gets '
            'real contrast. Brightening the pill (10% -> 25% white) makes '
            'the white text harder to read, not easier.',
      );
      expect(bg.a, greaterThan(0.1));
    });

    test('secondary button border is at least 50% white', () {
      expect(
        AppColors.modalSecondaryButtonBorder.a,
        greaterThanOrEqualTo(0.5),
        reason: 'The pill outline needs to read against the new darker fill.',
      );
    });

    test('button text style meets WCAG large-text threshold', () {
      final style = ModalTheme.buttonTextStyle;
      expect(style.fontSize, greaterThanOrEqualTo(14));
      expect(style.fontWeight, FontWeight.w700);
    });
  });

  group('Opaque chip pair (#2798)', () {
    // The selected chip is the one on-glass control with a FULLY OPAQUE fill,
    // which is what makes it assertable from constants at all: everything else
    // on the sheet is translucent, so its true ratio depends on the backdrop
    // and only the composited gate can measure it.
    //
    // #2798 shipped because a bespoke chip picked its own fill (#F2F2EE, the
    // light-THEME surface) and kept an on-glass foreground on top: 1.79:1, then
    // 1.12:1 once #2764's sweep moved the label to the white text ramp. The
    // canonical pair below was correct the whole time.
    test('selected chip fill and its label clear WCAG AA', () {
      final ratio = _contrast(
        AppColors.modalChipTextActive,
        AppColors.modalChipBackgroundActive,
      );
      expect(
        ratio,
        greaterThanOrEqualTo(4.5),
        reason:
            'Selected-chip label on its own fill must clear WCAG AA (4.5:1). '
            'Measured ${ratio.toStringAsFixed(2)}:1.',
      );
    });

    test('the selected chip fill is opaque, so the pair is backdrop-free', () {
      // If this fill ever gains alpha the test above silently starts measuring
      // a colour that is never painted — the assertion would still pass while
      // meaning nothing.
      expect(
        AppColors.modalChipBackgroundActive.a,
        1.0,
        reason:
            'The chip pair is assertable from constants only while the fill is '
            'opaque. A translucent fill belongs to the composited gate '
            '(e2e/scripts/contrast_surfaces.mjs), not to a widget test.',
      );
    });
  });
}

/// WCAG 2.x relative-luminance contrast ratio between two opaque colours.
double _contrast(Color a, Color b) {
  final la = _luminance(a);
  final lb = _luminance(b);
  final hi = la > lb ? la : lb;
  final lo = la > lb ? lb : la;
  return (hi + 0.05) / (lo + 0.05);
}

double _luminance(Color c) {
  double channel(double v) =>
      v <= 0.03928 ? v / 12.92 : math.pow((v + 0.055) / 1.055, 2.4).toDouble();
  return 0.2126 * channel(c.r) + 0.7152 * channel(c.g) + 0.0722 * channel(c.b);
}
