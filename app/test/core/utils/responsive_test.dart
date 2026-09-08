import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/responsive.dart';

void main() {
  group('Responsive.measureInset (#2926)', () {
    test('is the plain base inset at phone widths', () {
      // The phone-safety property, the same one ContentColumn gets from
      // Center + ConstrainedBox: below the measure the arithmetic must not
      // bind at all, at any of the widths the app actually ships on.
      for (final width in [320.0, 390.0, 430.0, 600.0]) {
        expect(
          Responsive.measureInset(width, measure: Responsive.galleryMaxWidth),
          Responsive.baseInset,
          reason: 'gallery inset should not bind at ${width}px',
        );
        expect(
          Responsive.measureInset(width),
          Responsive.baseInset,
          reason: 'reading inset should not bind at ${width}px',
        );
      }
    });

    test('holds the base inset right up to the measure', () {
      expect(
        Responsive.measureInset(Responsive.galleryMaxWidth,
            measure: Responsive.galleryMaxWidth),
        Responsive.baseInset,
      );
    });

    test('lines content up with the centered column above the measure', () {
      // 1440 is the desktop reference window. The inset must land on the
      // column's left edge plus the base inset — i.e. content inside a
      // window-wide scroll region paints exactly where a ContentColumn of
      // the same measure would put it.
      const window = 1440.0;
      const gutter = (window - Responsive.galleryMaxWidth) / 2;
      expect(
        Responsive.measureInset(window, measure: Responsive.galleryMaxWidth),
        gutter + Responsive.baseInset,
      );

      // The regression #2926 reported: at the *reading* measure the same
      // window yields a 420px inset — a void down the left of the Library.
      // Kept as a named contrast so a future edit that quietly re-points the
      // shelves at contentMaxWidth has to change this line too.
      expect(Responsive.measureInset(window), 420);
      expect(
        Responsive.measureInset(window, measure: Responsive.galleryMaxWidth),
        lessThan(Responsive.measureInset(window)),
      );
    });

    test('honors a caller-supplied base inset', () {
      expect(
        Responsive.measureInset(1440, measure: 640, inset: 0),
        400,
      );
    });
  });
}
