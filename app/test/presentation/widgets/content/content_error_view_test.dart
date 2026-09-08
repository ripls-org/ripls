import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/content/content_error_view.dart';

void main() {
  group('ContentErrorView', () {
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
            body: ContentErrorView(
              errorMessage: 'Network connection failed',
            ),
          ),
        ),
      );

      // Assert
      expect(find.text('Network connection failed'), findsOneWidget);
      expect(find.byIcon(Icons.error_outline), findsOneWidget);
    });

    testWidgets('renders with default title', (WidgetTester tester) async {
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
            body: ContentErrorView(
              errorMessage: 'Error occurred',
            ),
          ),
        ),
      );

      // Assert
      expect(find.text('Failed to load'), findsOneWidget);
    });

    testWidgets('renders with custom title', (WidgetTester tester) async {
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
            body: ContentErrorView(
              errorMessage: 'Error occurred',
              title: 'Failed to load gear details',
            ),
          ),
        ),
      );

      // Assert
      expect(find.text('Failed to load gear details'), findsOneWidget);
      expect(find.text('Failed to load'), findsNothing);
    });

    testWidgets('shows retry button when onRetry provided', (WidgetTester tester) async {
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
            body: ContentErrorView(
              errorMessage: 'Error occurred',
              onRetry: () {},
            ),
          ),
        ),
      );

      // Assert
      expect(find.text('Retry'), findsOneWidget);
      expect(find.byType(ElevatedButton), findsOneWidget);
    });

    testWidgets('hides retry button when onRetry is null', (WidgetTester tester) async {
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
            body: ContentErrorView(
              errorMessage: 'Error occurred',
            ),
          ),
        ),
      );

      // Assert
      expect(find.text('Retry'), findsNothing);
      expect(find.byType(ElevatedButton), findsNothing);
    });

    testWidgets('calls onRetry when retry button tapped', (WidgetTester tester) async {
      // Arrange
      bool retryCalled = false;

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
            body: ContentErrorView(
              errorMessage: 'Error occurred',
              onRetry: () => retryCalled = true,
            ),
          ),
        ),
      );

      // Act
      await tester.tap(find.text('Retry'));
      await tester.pump();

      // Assert
      expect(retryCalled, isTrue);
    });

    testWidgets('uses custom retry label', (WidgetTester tester) async {
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
            body: ContentErrorView(
              errorMessage: 'Error occurred',
              retryLabel: 'Try Again',
              onRetry: () {},
            ),
          ),
        ),
      );

      // Assert
      expect(find.text('Try Again'), findsOneWidget);
      expect(find.text('Retry'), findsNothing);
    });

    testWidgets('uses custom icon', (WidgetTester tester) async {
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
            body: ContentErrorView(
              errorMessage: 'Error occurred',
              icon: Icons.warning,
            ),
          ),
        ),
      );

      // Assert
      expect(find.byIcon(Icons.warning), findsOneWidget);
      expect(find.byIcon(Icons.error_outline), findsNothing);
    });

    testWidgets('uses custom icon size', (WidgetTester tester) async {
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
            body: ContentErrorView(
              errorMessage: 'Error occurred',
              iconSize: 64,
            ),
          ),
        ),
      );

      // Assert
      final icon = tester.widget<Icon>(find.byIcon(Icons.error_outline));
      expect(icon.size, equals(64));
    });

    testWidgets('has black background', (WidgetTester tester) async {
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
            body: ContentErrorView(
              errorMessage: 'Error occurred',
            ),
          ),
        ),
      );

      // Assert
      final container = tester.widget<Container>(
        find.descendant(
          of: find.byType(ContentErrorView),
          matching: find.byType(Container),
        ),
      );
      expect(container.color, equals(Colors.black));
    });

    testWidgets('centers content with Column alignment', (WidgetTester tester) async {
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
            body: ContentErrorView(
              errorMessage: 'Error occurred',
            ),
          ),
        ),
      );

      // Assert - verify Column exists with correct alignment
      final columnFinder = find.descendant(
        of: find.byType(ContentErrorView),
        matching: find.byType(Column),
      );
      expect(columnFinder, findsWidgets);

      final column = tester.widgetList<Column>(columnFinder).first;
      expect(column.mainAxisAlignment, equals(MainAxisAlignment.center));
    });

    testWidgets('applies correct text styles', (WidgetTester tester) async {
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
            body: ContentErrorView(
              errorMessage: 'Error occurred',
              title: 'Custom Title',
            ),
          ),
        ),
      );

      // Assert - title style
      final titleText = tester.widget<Text>(find.text('Custom Title'));
      expect(titleText.style?.color, equals(Colors.white));
      expect(titleText.style?.fontSize, equals(18));
      expect(titleText.style?.fontWeight, equals(FontWeight.bold));

      // Assert - error message style
      final errorText = tester.widget<Text>(find.text('Error occurred'));
      expect(errorText.style?.color, equals(Colors.white70));
      expect(errorText.style?.fontSize, equals(14));
      expect(errorText.textAlign, equals(TextAlign.center));
    });

    testWidgets('wraps error message with padding', (WidgetTester tester) async {
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
            body: ContentErrorView(
              errorMessage: 'This is a very long error message that should be wrapped with padding',
            ),
          ),
        ),
      );

      // Assert - find the Padding widget containing the error message
      final paddingFinder = find.ancestor(
        of: find.text('This is a very long error message that should be wrapped with padding'),
        matching: find.byType(Padding),
      );
      expect(paddingFinder, findsOneWidget);

      final padding = tester.widget<Padding>(paddingFinder);
      expect(padding.padding, equals(const EdgeInsets.symmetric(horizontal: 24)));
    });
  });
}
