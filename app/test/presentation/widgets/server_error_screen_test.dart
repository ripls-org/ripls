import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/server_error_screen.dart';

void main() {
  Widget buildWidget({
    String errorMessage = 'Something went wrong',
    required VoidCallback onRetry,
    bool isOffline = false,
    bool isRetrying = false,
  }) {
    return MaterialApp(
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: AppLocalizations.supportedLocales,
      home: ServerErrorScreen(
        errorMessage: errorMessage,
        onRetry: onRetry,
        isOffline: isOffline,
        isRetrying: isRetrying,
      ),
    );
  }

  group('ServerErrorScreen — server error variant', () {
    testWidgets('shows server unavailable title', (tester) async {
      await tester.pumpWidget(buildWidget(onRetry: () {}));

      expect(find.text('Server unavailable'), findsOneWidget);
    });

    testWidgets('shows cloud_off icon', (tester) async {
      await tester.pumpWidget(buildWidget(onRetry: () {}));

      expect(find.byIcon(Icons.cloud_off_rounded), findsOneWidget);
    });

    testWidgets('shows Try Again button', (tester) async {
      await tester.pumpWidget(buildWidget(onRetry: () {}));

      expect(find.text('Try Again'), findsOneWidget);
    });

    testWidgets('fires onRetry callback when Try Again tapped', (tester) async {
      var retryCalled = false;

      await tester.pumpWidget(buildWidget(onRetry: () => retryCalled = true));
      await tester.tap(find.text('Try Again'));
      await tester.pump();

      expect(retryCalled, isTrue);
    });

    testWidgets('shows Report a Problem button', (tester) async {
      await tester.pumpWidget(buildWidget(onRetry: () {}));

      expect(find.text('Report a Problem'), findsOneWidget);
    });
  });

  group('ServerErrorScreen — offline variant', () {
    testWidgets('shows offline title when isOffline=true', (tester) async {
      await tester.pumpWidget(buildWidget(onRetry: () {}, isOffline: true));

      expect(find.text("You're offline"), findsOneWidget);
    });

    testWidgets('shows wifi_off icon when isOffline=true', (tester) async {
      await tester.pumpWidget(buildWidget(onRetry: () {}, isOffline: true));

      expect(find.byIcon(Icons.wifi_off_rounded), findsOneWidget);
    });

    testWidgets('does not show server-error title when offline', (tester) async {
      await tester.pumpWidget(buildWidget(onRetry: () {}, isOffline: true));

      expect(find.text('Server unavailable'), findsNothing);
    });
  });

  group('ServerErrorScreen — retrying state', () {
    testWidgets('does not fire onRetry when already retrying', (tester) async {
      var retryCalled = false;

      await tester.pumpWidget(
        buildWidget(onRetry: () => retryCalled = true, isRetrying: true),
      );
      await tester.tap(find.text('Try Again'), warnIfMissed: false);
      await tester.pump();

      expect(retryCalled, isFalse);
    });

    testWidgets('shows loading indicator when isRetrying=true', (tester) async {
      await tester.pumpWidget(buildWidget(onRetry: () {}, isRetrying: true));

      expect(find.byType(CircularProgressIndicator), findsOneWidget);
    });

    testWidgets('shows refresh icon when not retrying', (tester) async {
      await tester.pumpWidget(buildWidget(onRetry: () {}));

      expect(find.byIcon(Icons.refresh), findsOneWidget);
      expect(find.byType(CircularProgressIndicator), findsNothing);
    });
  });
}
