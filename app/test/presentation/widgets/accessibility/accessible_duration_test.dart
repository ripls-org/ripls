import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';

void main() {
  group('accessibleDuration', () {
    testWidgets('returns the requested duration when reduce-motion is off',
        (tester) async {
      late Duration result;
      await tester.pumpWidget(MediaQuery(
        data: const MediaQueryData(),
        child: Builder(builder: (context) {
          result = accessibleDuration(context, const Duration(milliseconds: 200));
          return const SizedBox();
        }),
      ));

      expect(result, const Duration(milliseconds: 200));
    });

    testWidgets('returns Duration.zero when reduce-motion is on',
        (tester) async {
      late Duration result;
      await tester.pumpWidget(MediaQuery(
        data: const MediaQueryData(disableAnimations: true),
        child: Builder(builder: (context) {
          result = accessibleDuration(context, const Duration(milliseconds: 200));
          return const SizedBox();
        }),
      ));

      expect(result, Duration.zero);
    });
  });
}
