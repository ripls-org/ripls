import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/theme/gen/design_tokens.gen.dart';
import 'package:ripls/core/utils/value_helpers.dart';
import 'package:ripls/data/gen/ripls/api/value.pb.dart';

void main() {
  group('ValueHelpers.formatValue', () {
    test('formats small value correctly', () {
      expect(ValueHelpers.formatValue(75), equals('\$75'));
    });

    test('formats value with thousands separator', () {
      expect(ValueHelpers.formatValue(1250), equals('\$1,250'));
    });

    test('formats large value with multiple commas', () {
      expect(ValueHelpers.formatValue(12345678), equals('\$12,345,678'));
    });

    test('returns null for zero value', () {
      expect(ValueHelpers.formatValue(0), isNull);
    });

    test('returns null for negative value', () {
      expect(ValueHelpers.formatValue(-1), isNull);
    });

    test('returns null for null input', () {
      expect(ValueHelpers.formatValue(null), isNull);
    });

    test('formats exact dollar amounts', () {
      expect(ValueHelpers.formatValue(1), equals('\$1'));
      expect(ValueHelpers.formatValue(100), equals('\$100'));
      expect(ValueHelpers.formatValue(1000), equals('\$1,000'));
    });
  });

  group('ValueHelpers.getConfidenceIcon', () {
    test('returns verified icon for high confidence', () {
      expect(ValueHelpers.getConfidenceIcon(0.8), equals(Icons.verified));
      expect(ValueHelpers.getConfidenceIcon(0.9), equals(Icons.verified));
      expect(ValueHelpers.getConfidenceIcon(1), equals(Icons.verified));
    });

    test('returns help icon for medium confidence', () {
      expect(ValueHelpers.getConfidenceIcon(0.5), equals(Icons.help_outline));
      expect(ValueHelpers.getConfidenceIcon(0.7), equals(Icons.help_outline));
      expect(ValueHelpers.getConfidenceIcon(0.79), equals(Icons.help_outline));
    });

    test('returns warning icon for low confidence', () {
      expect(ValueHelpers.getConfidenceIcon(0), equals(Icons.warning_amber));
      expect(ValueHelpers.getConfidenceIcon(0.3), equals(Icons.warning_amber));
      expect(ValueHelpers.getConfidenceIcon(0.49), equals(Icons.warning_amber));
    });
  });

  group('ValueHelpers.getConfidenceColor', () {
    // Now resolves against the ambient theme (#2445), so these need a real
    // element to read a Brightness from. The point of the change is that the
    // SAME confidence yields different colours per theme, so each case is
    // asserted in both — a single-theme assertion is what let the light theme
    // ship dark-theme status colours in the first place.
    // One pump per test. Pumping twice inside a single test did NOT re-resolve
    // the theme — the second capture kept the first theme's value even with a
    // changed key — so a two-in-one test silently asserted light twice and
    // "passed" the dark case without ever entering dark. One tree per test
    // removes the question.
    Future<Color> colorFor(
      WidgetTester tester,
      double confidence,
      Brightness brightness,
    ) async {
      late Color captured;
      await tester.pumpWidget(
        MaterialApp(
          theme: ThemeData(brightness: brightness),
          home: Builder(
            builder: (context) {
              captured = ValueHelpers.getConfidenceColor(context, confidence);
              return const SizedBox.shrink();
            },
          ),
        ),
      );
      return captured;
    }

    testWidgets('high confidence is success (light)', (tester) async {
      expect(await colorFor(tester, 0.85, Brightness.light),
          equals(DesignTokens.lightSuccess));
    });
    testWidgets('high confidence is success (dark)', (tester) async {
      expect(await colorFor(tester, 0.85, Brightness.dark),
          equals(DesignTokens.darkSuccess));
    });

    testWidgets('medium confidence is warning (light)', (tester) async {
      expect(await colorFor(tester, 0.65, Brightness.light),
          equals(DesignTokens.lightWarning));
    });
    testWidgets('medium confidence is warning (dark)', (tester) async {
      expect(await colorFor(tester, 0.65, Brightness.dark),
          equals(DesignTokens.darkWarning));
    });

    testWidgets('low confidence is error (light)', (tester) async {
      expect(await colorFor(tester, 0.3, Brightness.light),
          equals(DesignTokens.lightError));
    });
    testWidgets('low confidence is error (dark)', (tester) async {
      expect(await colorFor(tester, 0.3, Brightness.dark),
          equals(DesignTokens.darkError));
    });
  });

  group('ValueHelpers.getConfidenceLabel', () {
    test('returns correct labels for confidence levels', () {
      expect(ValueHelpers.getConfidenceLabel(0.9), equals('High confidence'));
      expect(ValueHelpers.getConfidenceLabel(0.6), equals('Medium confidence'));
      expect(ValueHelpers.getConfidenceLabel(0.3), equals('Low confidence'));
    });
  });

  group('ValueHelpers.hasValidEstimate', () {
    test('returns false for null estimate', () {
      expect(ValueHelpers.hasValidEstimate(null), isFalse);
    });

    test('returns false for zero value estimate', () {
      final estimate = ValueEstimate()..estimatedValueUsd = 0.0;
      expect(ValueHelpers.hasValidEstimate(estimate), isFalse);
    });

    test('returns true for positive value estimate', () {
      final estimate = ValueEstimate()..estimatedValueUsd = 75.0;
      expect(ValueHelpers.hasValidEstimate(estimate), isTrue);
    });

    test('returns true for large value estimate', () {
      final estimate = ValueEstimate()..estimatedValueUsd = 1250.0;
      expect(ValueHelpers.hasValidEstimate(estimate), isTrue);
    });
  });
}
