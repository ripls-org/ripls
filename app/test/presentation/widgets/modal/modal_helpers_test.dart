import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/modal/modal_helpers.dart';

void main() {
  Widget wrapWithMaterialApp(Widget child) {
    return MaterialApp(
      theme: AppTheme.lightTheme,
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: AppLocalizations.supportedLocales,

      home: Scaffold(
        body: child,
      ),
    );
  }

  group('ModalHelpers.showStandardModal', () {
    testWidgets('shows modal with default height', (tester) async {
      await tester.pumpWidget(
        wrapWithMaterialApp(
          Builder(
            builder: (context) {
              return ElevatedButton(
                onPressed: () {
                  ModalHelpers.showStandardModal(
                    context,
                    builder: (context) => const Text('Modal Content'),
                  );
                },
                child: const Text('Show Modal'),
              );
            },
          ),
        ),
      );

      await tester.tap(find.text('Show Modal'));
      await tester.pumpAndSettle();

      expect(find.text('Modal Content'), findsOneWidget);
    });

    testWidgets('modal is dismissible by default', (tester) async {
      await tester.pumpWidget(
        wrapWithMaterialApp(
          Builder(
            builder: (context) {
              return ElevatedButton(
                onPressed: () {
                  ModalHelpers.showStandardModal(
                    context,
                    builder: (context) => const Text('Modal Content'),
                  );
                },
                child: const Text('Show Modal'),
              );
            },
          ),
        ),
      );

      await tester.tap(find.text('Show Modal'));
      await tester.pumpAndSettle();

      // Tap outside to dismiss
      await tester.tapAt(const Offset(10, 10));
      await tester.pumpAndSettle();

      expect(find.text('Modal Content'), findsNothing);
    });

    testWidgets('can customize height factor', (tester) async {
      await tester.pumpWidget(
        wrapWithMaterialApp(
          Builder(
            builder: (context) {
              return ElevatedButton(
                onPressed: () {
                  ModalHelpers.showStandardModal(
                    context,
                    builder: (context) => const Text('Modal Content'),
                    heightFactor: 0.5,
                  );
                },
                child: const Text('Show Modal'),
              );
            },
          ),
        ),
      );

      await tester.tap(find.text('Show Modal'));
      await tester.pumpAndSettle();

      expect(find.text('Modal Content'), findsOneWidget);
    });

    testWidgets('can disable dismissal', (tester) async {
      await tester.pumpWidget(
        wrapWithMaterialApp(
          Builder(
            builder: (context) {
              return ElevatedButton(
                onPressed: () {
                  ModalHelpers.showStandardModal(
                    context,
                    builder: (context) => const Text('Modal Content'),
                    isDismissible: false,
                  );
                },
                child: const Text('Show Modal'),
              );
            },
          ),
        ),
      );

      await tester.tap(find.text('Show Modal'));
      await tester.pumpAndSettle();

      // Try to tap outside
      await tester.tapAt(const Offset(10, 10));
      await tester.pumpAndSettle();

      // Modal should still be visible
      expect(find.text('Modal Content'), findsOneWidget);
    });

    testWidgets('modal uses animated container', (tester) async {
      await tester.pumpWidget(
        wrapWithMaterialApp(
          Builder(
            builder: (context) {
              return ElevatedButton(
                onPressed: () {
                  ModalHelpers.showStandardModal(
                    context,
                    builder: (context) => const Text('Modal Content'),
                  );
                },
                child: const Text('Show Modal'),
              );
            },
          ),
        ),
      );

      await tester.tap(find.text('Show Modal'));
      await tester.pumpAndSettle();

      expect(find.byType(AnimatedContainer), findsOneWidget);
    });
  });

  group('ModalHelpers.handleModalClose', () {
    testWidgets('closes immediately when no unsaved changes', (tester) async {
      bool closed = false;

      await tester.pumpWidget(
        wrapWithMaterialApp(
          Builder(
            builder: (context) {
              return ElevatedButton(
                onPressed: () {
                  ModalHelpers.handleModalClose(
                    context,
                    hasUnsavedChanges: false,
                    onConfirmClose: () => closed = true,
                  );
                },
                child: const Text('Close'),
              );
            },
          ),
        ),
      );

      await tester.tap(find.text('Close'));
      await tester.pump();

      expect(closed, true);
    });

    testWidgets('shows dialog when has unsaved changes', (tester) async {
      await tester.pumpWidget(
        wrapWithMaterialApp(
          Builder(
            builder: (context) {
              return ElevatedButton(
                onPressed: () {
                  ModalHelpers.handleModalClose(
                    context,
                    hasUnsavedChanges: true,
                    onConfirmClose: () {},
                  );
                },
                child: const Text('Close'),
              );
            },
          ),
        ),
      );

      await tester.tap(find.text('Close'));
      await tester.pumpAndSettle();

      expect(find.text('Unsaved Changes'), findsOneWidget);
      expect(
        find.text('You have unsaved changes. Are you sure you want to close?'),
        findsOneWidget,
      );
      expect(find.text('Cancel'), findsOneWidget);
      expect(find.text('Discard'), findsOneWidget);
    });

    testWidgets('cancels close when user taps Cancel', (tester) async {
      bool closed = false;

      await tester.pumpWidget(
        wrapWithMaterialApp(
          Builder(
            builder: (context) {
              return ElevatedButton(
                onPressed: () {
                  ModalHelpers.handleModalClose(
                    context,
                    hasUnsavedChanges: true,
                    onConfirmClose: () => closed = true,
                  );
                },
                child: const Text('Close'),
              );
            },
          ),
        ),
      );

      await tester.tap(find.text('Close'));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();

      expect(closed, false);
      expect(find.text('Unsaved Changes'), findsNothing);
    });

    testWidgets('confirms close when user taps Discard', (tester) async {
      bool closed = false;

      await tester.pumpWidget(
        wrapWithMaterialApp(
          Builder(
            builder: (context) {
              return ElevatedButton(
                onPressed: () {
                  ModalHelpers.handleModalClose(
                    context,
                    hasUnsavedChanges: true,
                    onConfirmClose: () => closed = true,
                  );
                },
                child: const Text('Close'),
              );
            },
          ),
        ),
      );

      await tester.tap(find.text('Close'));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Discard'));
      await tester.pumpAndSettle();

      expect(closed, true);
    });

    testWidgets('pops navigator when no callback provided', (tester) async {
      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.lightTheme,
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,

          home: Scaffold(
            body: Builder(
              builder: (context) {
                return ElevatedButton(
                  onPressed: () {
                    Navigator.push(
                      context,
                      MaterialPageRoute(
                        builder: (context) => Scaffold(
                          body: ElevatedButton(
                            onPressed: () {
                              ModalHelpers.handleModalClose(
                                context,
                                hasUnsavedChanges: false,
                              );
                            },
                            child: const Text('Close Page'),
                          ),
                        ),
                      ),
                    );
                  },
                  child: const Text('Open Page'),
                );
              },
            ),
          ),
        ),
      );

      await tester.tap(find.text('Open Page'));
      await tester.pumpAndSettle();

      expect(find.text('Close Page'), findsOneWidget);

      await tester.tap(find.text('Close Page'));
      await tester.pumpAndSettle();

      expect(find.text('Close Page'), findsNothing);
      expect(find.text('Open Page'), findsOneWidget);
    });
  });

  group('ModalHelpers.dismissKeyboard', () {
    testWidgets('unfocuses when called', (tester) async {
      final focusNode = FocusNode();

      await tester.pumpWidget(
        wrapWithMaterialApp(
          Column(
            children: [
              TextField(focusNode: focusNode),
              Builder(
                builder: (context) {
                  return ElevatedButton(
                    onPressed: () {
                      ModalHelpers.dismissKeyboard(context);
                    },
                    child: const Text('Dismiss'),
                  );
                },
              ),
            ],
          ),
        ),
      );

      // Focus the text field
      focusNode.requestFocus();
      await tester.pump();

      expect(focusNode.hasFocus, true);

      // Dismiss keyboard
      await tester.tap(find.text('Dismiss'));
      await tester.pump();

      expect(focusNode.hasFocus, false);
    });

    testWidgets('works when no focus node is active', (tester) async {
      await tester.pumpWidget(
        wrapWithMaterialApp(
          Builder(
            builder: (context) {
              return ElevatedButton(
                onPressed: () {
                  ModalHelpers.dismissKeyboard(context);
                },
                child: const Text('Dismiss'),
              );
            },
          ),
        ),
      );

      // Should not throw
      await tester.tap(find.text('Dismiss'));
      await tester.pump();
    });
  });
}
