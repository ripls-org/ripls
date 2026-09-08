import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/media/attribution_widget.dart';

void main() {
  group('AttributionWidget', () {
    final testAttribution = Attribution(
      provider: StockImageProvider.STOCK_IMAGE_PROVIDER_UNSPLASH,
      originalUrl: 'https://unsplash.com/photos/test123',
      creatorName: 'Jane Smith',
      creatorUsername: 'janesmith',
    );

    testWidgets('displays full attribution with creator name and Unsplash link',
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
            body: AttributionWidget(attribution: testAttribution),
          ),
        ),
      );

      // Verify text elements are present
      expect(find.text('Photo by '), findsOneWidget);
      expect(find.text('Jane Smith'), findsOneWidget);
      expect(find.text(' on '), findsOneWidget);
      expect(find.text('Unsplash'), findsOneWidget);
    });

    testWidgets('displays compact attribution with camera icon', (tester) async {
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
            body: AttributionWidget(
              attribution: testAttribution,
              compact: true,
            ),
          ),
        ),
      );

      // Verify camera icon is present
      expect(find.byIcon(Icons.camera_alt), findsOneWidget);

      // Verify creator name is present
      expect(find.text('Jane Smith'), findsOneWidget);

      // Verify "Photo by" is NOT present in compact mode
      expect(find.text('Photo by '), findsNothing);
    });

    testWidgets('applies custom text style when provided', (tester) async {
      const customStyle = TextStyle(fontSize: 16, color: Colors.red);

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
            body: AttributionWidget(
              attribution: testAttribution,
              textStyle: customStyle,
            ),
          ),
        ),
      );

      await tester.pumpAndSettle();

      // Widget should render without errors with custom style
      expect(find.byType(AttributionWidget), findsOneWidget);
    });

    testWidgets('has proper semantics for accessibility', (tester) async {
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
            body: AttributionWidget(attribution: testAttribution),
          ),
        ),
      );

      // Verify semantic labels are present
      final semantics = tester.getSemantics(find.byType(Wrap));
      expect(
        semantics.label,
        contains('Photo by Jane Smith on Unsplash'),
      );
    });

    testWidgets('creator name is tappable', (tester) async {
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
            body: AttributionWidget(attribution: testAttribution),
          ),
        ),
      );

      // Find the creator name text
      final creatorNameFinder = find.text('Jane Smith');
      expect(creatorNameFinder, findsOneWidget);

      // Verify it's wrapped in a GestureDetector
      final gestureDetector = find.ancestor(
        of: creatorNameFinder,
        matching: find.byType(GestureDetector),
      );
      expect(gestureDetector, findsOneWidget);
    });

    testWidgets('Unsplash text is tappable', (tester) async {
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
            body: AttributionWidget(attribution: testAttribution),
          ),
        ),
      );

      // Find the Unsplash text
      final unsplashFinder = find.text('Unsplash');
      expect(unsplashFinder, findsOneWidget);

      // Verify it's wrapped in a GestureDetector
      final gestureDetector = find.ancestor(
        of: unsplashFinder,
        matching: find.byType(GestureDetector),
      );
      expect(gestureDetector, findsOneWidget);
    });
  });

  group('Pexels attribution', () {
    final pexelsAttribution = Attribution(
      provider: StockImageProvider.STOCK_IMAGE_PROVIDER_PEXELS,
      originalUrl: 'https://www.pexels.com/photo/sunset-beach-12345678/',
      creatorName: 'John Doe',
      creatorUsername: '', // Pexels doesn't use username
      photographerUrl: 'https://www.pexels.com/@johndoe',
    );

    testWidgets('displays full attribution with correct Pexels text',
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
            body: AttributionWidget(attribution: pexelsAttribution),
          ),
        ),
      );

      // Verify text elements are present
      expect(find.text('Photo by '), findsOneWidget);
      expect(find.text('John Doe'), findsOneWidget);
      expect(find.text(' on '), findsOneWidget);
      expect(find.text('Pexels'), findsOneWidget);
    });

    testWidgets('displays compact attribution with camera icon', (tester) async {
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
            body: AttributionWidget(
              attribution: pexelsAttribution,
              compact: true,
            ),
          ),
        ),
      );

      // Verify camera icon is present
      expect(find.byIcon(Icons.camera_alt), findsOneWidget);

      // Verify creator name is present
      expect(find.text('John Doe'), findsOneWidget);

      // Verify "Photo by" is NOT present in compact mode
      expect(find.text('Photo by '), findsNothing);
    });

    testWidgets('has proper semantics for Pexels accessibility', (tester) async {
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
            body: AttributionWidget(attribution: pexelsAttribution),
          ),
        ),
      );

      // Verify semantic labels are present with Pexels
      final semantics = tester.getSemantics(find.byType(Wrap));
      expect(
        semantics.label,
        contains('Photo by John Doe on Pexels'),
      );
    });

    testWidgets('Pexels text is tappable', (tester) async {
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
            body: AttributionWidget(attribution: pexelsAttribution),
          ),
        ),
      );

      // Find the Pexels text
      final pexelsFinder = find.text('Pexels');
      expect(pexelsFinder, findsOneWidget);

      // Verify it's wrapped in a GestureDetector
      final gestureDetector = find.ancestor(
        of: pexelsFinder,
        matching: find.byType(GestureDetector),
      );
      expect(gestureDetector, findsOneWidget);
    });

    testWidgets('creator name is tappable for Pexels', (tester) async {
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
            body: AttributionWidget(attribution: pexelsAttribution),
          ),
        ),
      );

      // Find the creator name text
      final creatorNameFinder = find.text('John Doe');
      expect(creatorNameFinder, findsOneWidget);

      // Verify it's wrapped in a GestureDetector
      final gestureDetector = find.ancestor(
        of: creatorNameFinder,
        matching: find.byType(GestureDetector),
      );
      expect(gestureDetector, findsOneWidget);
    });
  });
}
