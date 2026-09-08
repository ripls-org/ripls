import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_wrapper.dart';

void main() {
  group('SwipeToCloseWrapper', () {
    testWidgets('renders child widget', (WidgetTester tester) async {
      const testKey = Key('child');
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: SwipeToCloseWrapper(
            onClose: () {},
            child: const Text('Test Child', key: testKey),
          ),
        ),
      );

      expect(find.byKey(testKey), findsOneWidget);
      expect(find.text('Test Child'), findsOneWidget);
    });

    testWidgets('calls onClose when swipe velocity exceeds threshold',
        (WidgetTester tester) async {
      bool closeCalled = false;

      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: SwipeToCloseWrapper(
            onClose: () => closeCalled = true,
            velocityThreshold: 300,
            child: Container(
              width: 400,
              height: 400,
              color: Colors.blue,
            ),
          ),
        ),
      );

      // Simulate a fast swipe to the right
      await tester.fling(
        find.byType(Container),
        const Offset(500, 0), // Large offset to the right
        1000, // High velocity (pixels per second)
      );
      await tester.pumpAndSettle();

      expect(closeCalled, isTrue);
    });

    testWidgets('does not call onClose when swipe velocity is below threshold',
        (WidgetTester tester) async {
      bool closeCalled = false;

      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: SwipeToCloseWrapper(
            onClose: () => closeCalled = true,
            velocityThreshold: 300,
            child: Container(
              width: 400,
              height: 400,
              color: Colors.blue,
            ),
          ),
        ),
      );

      // Simulate a slow swipe to the right
      await tester.fling(
        find.byType(Container),
        const Offset(100, 0), // Small offset
        100, // Low velocity
      );
      await tester.pumpAndSettle();

      expect(closeCalled, isFalse);
    });

    testWidgets('does not call onClose when swipe direction is left',
        (WidgetTester tester) async {
      bool closeCalled = false;

      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: SwipeToCloseWrapper(
            onClose: () => closeCalled = true,
            velocityThreshold: 300,
            child: Container(
              width: 400,
              height: 400,
              color: Colors.blue,
            ),
          ),
        ),
      );

      // Simulate a swipe to the left (negative velocity)
      await tester.fling(
        find.byType(Container),
        const Offset(-500, 0), // Large offset to the left
        1000, // High velocity
      );
      await tester.pumpAndSettle();

      expect(closeCalled, isFalse);
    });

    testWidgets('respects custom velocity threshold',
        (WidgetTester tester) async {
      bool closeCalled = false;

      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: SwipeToCloseWrapper(
            onClose: () => closeCalled = true,
            velocityThreshold: 500, // Higher threshold
            child: Container(
              width: 400,
              height: 400,
              color: Colors.blue,
            ),
          ),
        ),
      );

      // Simulate a swipe with velocity between default (300) and custom (500)
      await tester.fling(
        find.byType(Container),
        const Offset(300, 0),
        400, // 400 px/s - would trigger default but not custom threshold
      );
      await tester.pumpAndSettle();

      expect(closeCalled, isFalse);
    });

    testWidgets('does not call onClose for vertical swipes',
        (WidgetTester tester) async {
      bool closeCalled = false;

      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: SwipeToCloseWrapper(
            onClose: () => closeCalled = true,
            velocityThreshold: 300,
            child: Container(
              width: 400,
              height: 400,
              color: Colors.blue,
            ),
          ),
        ),
      );

      // Simulate a vertical swipe
      await tester.fling(
        find.byType(Container),
        const Offset(0, 500), // Vertical swipe
        1000,
      );
      await tester.pumpAndSettle();

      expect(closeCalled, isFalse);
    });
  });
}
