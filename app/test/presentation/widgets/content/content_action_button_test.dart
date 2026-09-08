import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/content/content_action_button.dart';

void main() {
  group('ContentActionButton', () {
    testWidgets('renders with label', (WidgetTester tester) async {
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
            body: ContentActionButton(
              label: 'Lend',
              backgroundColor: Colors.blue,
              onPressed: () {},
            ),
          ),
        ),
      );

      // Assert
      expect(find.text('Lend'), findsOneWidget);
      expect(find.byType(ElevatedButton), findsOneWidget);
    });

    testWidgets('calls onPressed when tapped', (WidgetTester tester) async {
      // Arrange
      bool tapped = false;

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
            body: ContentActionButton(
              label: 'Submit',
              backgroundColor: Colors.green,
              onPressed: () => tapped = true,
            ),
          ),
        ),
      );

      // Act
      await tester.tap(find.text('Submit'));
      await tester.pump();

      // Assert
      expect(tapped, isTrue);
    });

    testWidgets('shows loading spinner when isLoading is true',
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
            body: ContentActionButton(
              label: 'Loading',
              backgroundColor: Colors.blue,
              isLoading: true,
              onPressed: () {},
            ),
          ),
        ),
      );

      // Assert
      expect(find.byType(CircularProgressIndicator), findsOneWidget);
      expect(find.text('Loading'), findsNothing);
    });

    testWidgets('is disabled when isLoading is true',
        (WidgetTester tester) async {
      // Arrange
      bool tapped = false;

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
            body: ContentActionButton(
              label: 'Submit',
              backgroundColor: Colors.blue,
              isLoading: true,
              onPressed: () => tapped = true,
            ),
          ),
        ),
      );

      // Act - try to tap the button
      await tester.tap(find.byType(ElevatedButton));
      await tester.pump();

      // Assert - callback should not have been called
      expect(tapped, isFalse);
    });

    testWidgets('is disabled when onPressed is null',
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
            body: ContentActionButton(
              label: 'Disabled',
              backgroundColor: Colors.grey,
              onPressed: null,
            ),
          ),
        ),
      );

      // Assert - find the ElevatedButton and check it's disabled
      final button = tester.widget<ElevatedButton>(
        find.byType(ElevatedButton),
      );
      expect(button.onPressed, isNull);
    });

    testWidgets('applies custom background color',
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
            body: ContentActionButton(
              label: 'Orange',
              backgroundColor: Colors.orange,
              onPressed: () {},
            ),
          ),
        ),
      );

      // Assert
      final button = tester.widget<ElevatedButton>(
        find.byType(ElevatedButton),
      );
      expect(
        button.style?.backgroundColor?.resolve({}),
        equals(Colors.orange),
      );
    });

    testWidgets('applies custom foreground color',
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
            body: ContentActionButton(
              label: 'Black Text',
              backgroundColor: Colors.white,
              foregroundColor: Colors.black,
              onPressed: () {},
            ),
          ),
        ),
      );

      // Assert
      final button = tester.widget<ElevatedButton>(
        find.byType(ElevatedButton),
      );
      expect(
        button.style?.foregroundColor?.resolve({}),
        equals(Colors.black),
      );
    });

    testWidgets('uses default white foreground color',
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
            body: ContentActionButton(
              label: 'Default',
              backgroundColor: Colors.blue,
              onPressed: () {},
            ),
          ),
        ),
      );

      // Assert
      final button = tester.widget<ElevatedButton>(
        find.byType(ElevatedButton),
      );
      expect(
        button.style?.foregroundColor?.resolve({}),
        equals(Colors.white),
      );
    });

    testWidgets('renders with icon when provided',
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
            body: ContentActionButton(
              label: 'Approve',
              backgroundColor: Colors.green,
              icon: Icons.check,
              onPressed: () {},
            ),
          ),
        ),
      );

      // Assert - ElevatedButton.icon is still a widget of type ElevatedButton
      expect(find.byWidgetPredicate(
        (widget) => widget is ElevatedButton,
      ), findsOneWidget);
      expect(find.byIcon(Icons.check), findsOneWidget);
      expect(find.text('Approve'), findsOneWidget);
    });

    testWidgets('hides icon when loading', (WidgetTester tester) async {
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
            body: ContentActionButton(
              label: 'Loading',
              backgroundColor: Colors.blue,
              icon: Icons.check,
              isLoading: true,
              onPressed: () {},
            ),
          ),
        ),
      );

      // Assert
      expect(find.byIcon(Icons.check), findsNothing);
      expect(find.byType(CircularProgressIndicator), findsOneWidget);
    });

    testWidgets('applies custom elevation', (WidgetTester tester) async {
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
            body: ContentActionButton(
              label: 'High Elevation',
              backgroundColor: Colors.blue,
              elevation: 8,
              onPressed: () {},
            ),
          ),
        ),
      );

      // Assert
      final button = tester.widget<ElevatedButton>(
        find.byType(ElevatedButton),
      );
      expect(button.style?.elevation?.resolve({}), equals(8));
    });

    testWidgets('applies custom border radius', (WidgetTester tester) async {
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
            body: ContentActionButton(
              label: 'Rounded',
              backgroundColor: Colors.blue,
              borderRadius: 20,
              onPressed: () {},
            ),
          ),
        ),
      );

      // Assert
      final button = tester.widget<ElevatedButton>(
        find.byType(ElevatedButton),
      );
      final shape = button.style?.shape?.resolve({}) as RoundedRectangleBorder?;
      expect(
        shape?.borderRadius,
        equals(BorderRadius.circular(20)),
      );
    });

    testWidgets('applies custom vertical padding', (WidgetTester tester) async {
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
            body: ContentActionButton(
              label: 'Padded',
              backgroundColor: Colors.blue,
              verticalPadding: 20,
              onPressed: () {},
            ),
          ),
        ),
      );

      // Assert
      final button = tester.widget<ElevatedButton>(
        find.byType(ElevatedButton),
      );
      final padding = button.style?.padding?.resolve({});
      expect(padding, equals(const EdgeInsets.symmetric(vertical: 20)));
    });

    testWidgets('uses default values when not specified',
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
            body: ContentActionButton(
              label: 'Default',
              backgroundColor: Colors.blue,
              onPressed: () {},
            ),
          ),
        ),
      );

      // Assert
      final button = tester.widget<ElevatedButton>(
        find.byType(ElevatedButton),
      );

      // Check defaults
      expect(button.style?.elevation?.resolve({}), equals(4));
      expect(button.style?.foregroundColor?.resolve({}), equals(Colors.white));

      final shape = button.style?.shape?.resolve({}) as RoundedRectangleBorder?;
      expect(shape?.borderRadius, equals(BorderRadius.circular(12)));

      final padding = button.style?.padding?.resolve({});
      expect(padding, equals(const EdgeInsets.symmetric(vertical: 12)));
    });
  });
}
