import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/keyboard_dismiss_wrapper.dart';

void main() {
  group('KeyboardDismissWrapper', () {
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
          home: KeyboardDismissWrapper(
            child: const Text('Test Child', key: testKey),
          ),
        ),
      );

      expect(find.byKey(testKey), findsOneWidget);
      expect(find.text('Test Child'), findsOneWidget);
    });

    testWidgets('unfocuses text field when tapping outside',
        (WidgetTester tester) async {
      final focusNode = FocusNode();

      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: KeyboardDismissWrapper(
            child: Scaffold(
              body: Column(
                children: [
                  TextField(focusNode: focusNode),
                  const Expanded(child: SizedBox()),
                ],
              ),
            ),
          ),
        ),
      );

      // Focus the text field.
      await tester.tap(find.byType(TextField));
      await tester.pump();
      expect(focusNode.hasFocus, isTrue);

      // Tap in the empty area below the text field to dismiss the keyboard.
      await tester.tapAt(const Offset(200, 400));
      await tester.pump();
      expect(focusNode.hasFocus, isFalse);

      focusNode.dispose();
    });

    testWidgets('does not interfere with interactive child widget taps',
        (WidgetTester tester) async {
      bool buttonPressed = false;

      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: KeyboardDismissWrapper(
            child: ElevatedButton(
              onPressed: () => buttonPressed = true,
              child: const Text('Press Me'),
            ),
          ),
        ),
      );

      await tester.tap(find.text('Press Me'));
      await tester.pump();

      expect(buttonPressed, isTrue);
    });

    testWidgets('unfocuses on tap over opaque background area',
        (WidgetTester tester) async {
      final focusNode = FocusNode();

      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: Scaffold(
            body: KeyboardDismissWrapper(
              child: SizedBox(
                width: 400,
                height: 600,
                child: Column(
                  children: [
                    TextField(focusNode: focusNode),
                    // Empty space below the text field — no interactive widget.
                    const Spacer(),
                  ],
                ),
              ),
            ),
          ),
        ),
      );

      // Focus the text field.
      await tester.tap(find.byType(TextField));
      await tester.pump();
      expect(focusNode.hasFocus, isTrue);

      // Tap in the empty area below the text field.
      await tester.tapAt(const Offset(200, 500));
      await tester.pump();
      expect(focusNode.hasFocus, isFalse);

      focusNode.dispose();
    });
  });
}
