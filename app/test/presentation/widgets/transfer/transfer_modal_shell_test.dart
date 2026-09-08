import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_sheet.dart';
import 'package:ripls/presentation/widgets/transfer/transfer_modal_shell.dart';

void main() {
  group('TransferModalShell', () {
    testWidgets('renders content when not loading and no error', (tester) async {
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
            body: TransferModalShell(
              content: const Text('Test Content'),
            ),
          ),
        ),
      );

      expect(find.text('Test Content'), findsOneWidget);
      expect(find.byType(CircularProgressIndicator), findsNothing);
      expect(find.text('Error'), findsNothing);
    });

    testWidgets('shows loading indicator when isLoading is true', (tester) async {
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
            body: TransferModalShell(
              content: const Text('Test Content'),
              isLoading: true,
            ),
          ),
        ),
      );

      expect(find.byType(CircularProgressIndicator), findsOneWidget);
      expect(find.text('Test Content'), findsNothing);
    });

    testWidgets('shows error message when errorMessage is provided', (tester) async {
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
            body: TransferModalShell(
              content: const Text('Test Content'),
              errorMessage: 'Something went wrong',
            ),
          ),
        ),
      );

      expect(find.text('Error'), findsOneWidget);
      expect(find.text('Something went wrong'), findsOneWidget);
      expect(find.text('Test Content'), findsNothing);
    });

    testWidgets('shows retry button when onRetry is provided', (tester) async {
      bool retryPressed = false;

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
            body: TransferModalShell(
              content: const Text('Test Content'),
              errorMessage: 'Network error',
              onRetry: () => retryPressed = true,
            ),
          ),
        ),
      );

      expect(find.text('Retry'), findsOneWidget);

      await tester.tap(find.text('Retry'));
      expect(retryPressed, true);
    });

    testWidgets('renders header when provided', (tester) async {
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
            body: TransferModalShell(
              header: const Text('Header'),
              content: const Text('Test Content'),
            ),
          ),
        ),
      );

      expect(find.text('Header'), findsOneWidget);
      expect(find.text('Test Content'), findsOneWidget);
    });

    testWidgets('renders bottom bar when provided', (tester) async {
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
            body: TransferModalShell(
              content: const Text('Test Content'),
              bottomBar: const Text('Bottom'),
            ),
          ),
        ),
      );

      expect(find.text('Bottom'), findsOneWidget);
      expect(find.text('Test Content'), findsOneWidget);
    });

    // Regression: shell must use GlassSheet, not the legacy cream Container.
    // Anyone who reverts to a Container surface fails this test.
    testWidgets('wraps content in a GlassSheet (glass migration)',
        (tester) async {
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
            body: TransferModalShell(
              content: const Text('Glass body'),
            ),
          ),
        ),
      );

      expect(find.byType(GlassSheet), findsOneWidget);
    });
  });
}
