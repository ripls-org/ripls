import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'contrast_helpers.dart';

/// Tests for the contrast helper itself (#2798).
///
/// A contrast gate that has never seen the bug it exists to catch is a vacuous
/// assertion — it passes forever and nobody notices. These pin the helper
/// against the exact shape of #2798 and against the cases it must stay silent
/// on, so a future refactor of the walker cannot quietly hollow it out.
void main() {
  Widget wrap(Widget child) => MaterialApp(home: Scaffold(body: child));

  group('detects the #2798 shape', () {
    testWidgets('white label on the light-surface fill is caught',
        (tester) async {
      // Verbatim reproduction: DesignTokens.lightSurface as a fill inside a
      // dark sheet, with the on-glass white ramp on top. 1.12:1.
      await tester.pumpWidget(wrap(
        DecoratedBox(
          decoration: const BoxDecoration(color: Color(0xFFF2F2EE)),
          child: Text(
            'Add Screenshots (optional)',
            style: const TextStyle(color: Color(0xFFFFFFFF), fontSize: 14),
          ),
        ),
      ));

      final pairs = opaqueContrastPairs(tester)
          .where((p) => p.label == 'Add Screenshots (optional)');

      expect(pairs, hasLength(1));
      expect(pairs.first.ratio, lessThan(1.2));
      expect(pairs.first.passes, isFalse);
    });

    testWidgets('sage label on the light-surface fill is caught',
        (tester) async {
      // The pre-af66dadd4 form of the same defect: GlassTokens.primary as a
      // foreground on the same fill. 1.79:1.
      await tester.pumpWidget(wrap(
        DecoratedBox(
          decoration: const BoxDecoration(color: Color(0xFFF2F2EE)),
          child: Text(
            'Bug',
            style: const TextStyle(color: Color(0xFF9DBFA8), fontSize: 13),
          ),
        ),
      ));

      final pairs = opaqueContrastPairs(tester).where((p) => p.label == 'Bug');
      expect(pairs.single.passes, isFalse);
      expect(pairs.single.ratio, closeTo(1.79, 0.02));
    });

    testWidgets('expectOpaqueContrast fails on a bad pair', (tester) async {
      await tester.pumpWidget(wrap(
        const DecoratedBox(
          decoration: BoxDecoration(color: Color(0xFFF2F2EE)),
          child: Text(
            'invisible',
            style: TextStyle(color: Color(0xFFFFFFFF), fontSize: 14),
          ),
        ),
      ));

      expect(() => expectOpaqueContrast(tester), throwsA(isA<TestFailure>()));
    });
  });

  group('stays silent where it cannot know the answer', () {
    testWidgets('a translucent fill is skipped, not guessed', (tester) async {
      // On glass this composites against whatever is behind the sheet. Any
      // ratio computed from the tree would be fiction.
      await tester.pumpWidget(wrap(
        const DecoratedBox(
          decoration: BoxDecoration(color: Color(0x14FFFFFF)),
          child: Text(
            'on glass',
            style: TextStyle(color: Color(0xFFFFFFFF), fontSize: 14),
          ),
        ),
      ));

      expect(
        opaqueContrastPairs(tester).where((p) => p.label == 'on glass'),
        isEmpty,
      );
    });

    testWidgets('a gradient fill is skipped', (tester) async {
      await tester.pumpWidget(wrap(
        const DecoratedBox(
          decoration: BoxDecoration(
            gradient: LinearGradient(colors: [Colors.black, Colors.white]),
          ),
          child: Text(
            'on gradient',
            style: TextStyle(color: Color(0xFFFFFFFF), fontSize: 14),
          ),
        ),
      ));

      expect(
        opaqueContrastPairs(tester).where((p) => p.label == 'on gradient'),
        isEmpty,
      );
    });

    testWidgets('a good pair passes', (tester) async {
      await tester.pumpWidget(wrap(
        const DecoratedBox(
          decoration: BoxDecoration(color: Color(0xFFFFFFFF)),
          child: Text(
            'legible',
            style: TextStyle(color: Color(0xFF1A1A1A), fontSize: 14),
          ),
        ),
      ));

      final pair =
          opaqueContrastPairs(tester).firstWhere((p) => p.label == 'legible');
      expect(pair.passes, isTrue);
      expectOpaqueContrast(tester);
    });
  });

  group('applies the WCAG threshold the element actually gets', () {
    test('body text needs 4.5:1', () {
      expect(thresholdForText(14, FontWeight.w400), kContrastBodyText);
      expect(thresholdForText(null, null), kContrastBodyText);
      expect(thresholdForText(17.9, FontWeight.w400), kContrastBodyText);
    });

    test('large text needs 3:1', () {
      expect(thresholdForText(18, FontWeight.w400), kContrastLargeOrGraphic);
      expect(thresholdForText(14, FontWeight.w700), kContrastLargeOrGraphic);
      expect(thresholdForText(22, FontWeight.w400), kContrastLargeOrGraphic);
    });

    testWidgets('icons are non-text content and need 3:1', (tester) async {
      // The success-state checkmark: sage on a pale sage badge, 3.18:1. It
      // passes 1.4.11 and would fail a flat 4.5:1 rule — the false positive
      // this split exists to prevent.
      await tester.pumpWidget(wrap(
        const DecoratedBox(
          decoration: BoxDecoration(color: Color(0xFFE8F3E9)),
          child: Icon(Icons.check_rounded, color: Color(0xFF6B8F71)),
        ),
      ));

      final pair = opaqueContrastPairs(tester).single;
      expect(pair.requiredRatio, kContrastLargeOrGraphic);
      expect(pair.ratio, closeTo(3.18, 0.05));
      expect(pair.passes, isTrue);
    });
  });

  group('wcagContrast', () {
    test('is symmetric and matches known values', () {
      expect(wcagContrast(Colors.white, Colors.black), closeTo(21.0, 0.01));
      expect(wcagContrast(Colors.black, Colors.white), closeTo(21.0, 0.01));
      expect(wcagContrast(Colors.white, Colors.white), closeTo(1.0, 0.001));
    });
  });
}
