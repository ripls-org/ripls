import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/theme/gen/design_tokens.gen.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/content/content_error_banner.dart';

void main() {
  group('ContentErrorBanner', () {
    testWidgets('exposes Semantics(liveRegion: true) so screen readers announce errors',
        (WidgetTester tester) async {
      await tester.pumpWidget(const MaterialApp(
        localizationsDelegates: [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        supportedLocales: [Locale('en')],
        home: Scaffold(
          body: ContentErrorBanner(errorMessage: 'Something went wrong'),
        ),
      ));

      expect(find.byType(LiveRegion), findsOneWidget);
      final node = tester.getSemantics(find.text('Something went wrong'));
      expect(node.flagsCollection.isLiveRegion, isTrue);
    });

    testWidgets('renders with error message', (WidgetTester tester) async {
      // Arrange & Act
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
            body: ContentErrorBanner(
              errorMessage: 'Something went wrong',
            ),
          ),
        ),
      );

      // Assert
      expect(find.text('Something went wrong'), findsOneWidget);
    });

    testWidgets('shows default warning icon', (WidgetTester tester) async {
      // Arrange & Act
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
            body: ContentErrorBanner(
              errorMessage: 'Error occurred',
            ),
          ),
        ),
      );

      // Assert
      expect(find.byIcon(Icons.warning_amber_rounded), findsOneWidget);
    });

    testWidgets('shows custom icon when provided', (WidgetTester tester) async {
      // Arrange & Act
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
            body: ContentErrorBanner(
              errorMessage: 'Error occurred',
              icon: Icons.error_outline,
            ),
          ),
        ),
      );

      // Assert
      expect(find.byIcon(Icons.error_outline), findsOneWidget);
      expect(find.byIcon(Icons.warning_amber_rounded), findsNothing);
    });

    testWidgets('hides icon when showIcon is false',
        (WidgetTester tester) async {
      // Arrange & Act
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
            body: ContentErrorBanner(
              errorMessage: 'Error occurred',
              showIcon: false,
            ),
          ),
        ),
      );

      // Assert
      expect(find.byType(Icon), findsNothing);
    });

    testWidgets('applies error color to container', (WidgetTester tester) async {
      // Arrange & Act
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
            body: ContentErrorBanner(
              errorMessage: 'Error occurred',
            ),
          ),
        ),
      );

      // Assert
      final container = tester.widget<Container>(
        find.descendant(
          of: find.byType(ContentErrorBanner),
          matching: find.byType(Container),
        ),
      );

      // The banner resolves its error colour from the ambient theme (#2445).
      // These pumps use a bare MaterialApp, whose default ThemeData is light,
      // so the expected value is the LIGHT error token — the one the app used
      // to skip in favour of the dark one at 2.59:1.
      final decoration = container.decoration! as BoxDecoration;
      expect(decoration.color, equals(DesignTokens.lightError.withAlpha(50)));
      expect(decoration.border, isA<Border>());
      expect(
        (decoration.border! as Border).top.color,
        equals(DesignTokens.lightError),
      );
    });

    testWidgets('applies rounded corners', (WidgetTester tester) async {
      // Arrange & Act
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
            body: ContentErrorBanner(
              errorMessage: 'Error occurred',
            ),
          ),
        ),
      );

      // Assert
      final container = tester.widget<Container>(
        find.descendant(
          of: find.byType(ContentErrorBanner),
          matching: find.byType(Container),
        ),
      );

      final decoration = container.decoration! as BoxDecoration;
      expect(decoration.borderRadius, equals(BorderRadius.circular(8)));
    });

    testWidgets('applies correct text style', (WidgetTester tester) async {
      // Arrange & Act
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
            body: ContentErrorBanner(
              errorMessage: 'Error occurred',
            ),
          ),
        ),
      );

      // Assert
      final textWidget = tester.widget<Text>(find.text('Error occurred'));
      expect(textWidget.style?.color, equals(DesignTokens.lightError));
      expect(textWidget.style?.fontSize, equals(14));
    });

    testWidgets('expands text to fill available width',
        (WidgetTester tester) async {
      // Arrange & Act
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
            body: ContentErrorBanner(
              errorMessage: 'This is a very long error message that should wrap to multiple lines',
            ),
          ),
        ),
      );

      // Assert - verify Expanded widget wraps the text
      expect(
        find.descendant(
          of: find.byType(ContentErrorBanner),
          matching: find.byType(Expanded),
        ),
        findsOneWidget,
      );
    });

    testWidgets('icon and text align at top', (WidgetTester tester) async {
      // Arrange & Act
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
            body: ContentErrorBanner(
              errorMessage: 'Error\nwith\nmultiple\nlines',
            ),
          ),
        ),
      );

      // Assert - Row should have crossAxisAlignment.start
      final row = tester.widget<Row>(
        find.descendant(
          of: find.byType(ContentErrorBanner),
          matching: find.byType(Row),
        ),
      );
      expect(row.crossAxisAlignment, equals(CrossAxisAlignment.start));
    });

    testWidgets('applies consistent padding', (WidgetTester tester) async {
      // Arrange & Act
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
            body: ContentErrorBanner(
              errorMessage: 'Error occurred',
            ),
          ),
        ),
      );

      // Assert
      final container = tester.widget<Container>(
        find.descendant(
          of: find.byType(ContentErrorBanner),
          matching: find.byType(Container),
        ),
      );
      expect(container.padding, equals(const EdgeInsets.all(12)));
    });
  });
}
