import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/creation/text_input_area.dart';

void main() {
  group('TextInputArea', () {
    testWidgets('renders with provided hint text', (WidgetTester tester) async {
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
            body: TextInputArea(
              controller: TextEditingController(),
              hintText: 'Enter your prompt here',
            ),
          ),
        ),
      );

      // Assert
      expect(find.text('Enter your prompt here'), findsOneWidget);
    });

    testWidgets('accepts text input', (WidgetTester tester) async {
      // Arrange
      final controller = TextEditingController();

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
            body: TextInputArea(
              controller: controller,
              hintText: 'Test hint',
            ),
          ),
        ),
      );

      // Act
      await tester.enterText(find.byType(TextField), 'Test input text');

      // Assert
      expect(controller.text, equals('Test input text'));
    });

    testWidgets('calls onChanged callback when text changes', (WidgetTester tester) async {
      // Arrange
      String? changedText;

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
            body: TextInputArea(
              controller: TextEditingController(),
              hintText: 'Test hint',
              onChanged: (value) => changedText = value,
            ),
          ),
        ),
      );

      // Act
      await tester.enterText(find.byType(TextField), 'New text');

      // Assert
      expect(changedText, equals('New text'));
    });

    testWidgets('uses default line counts when not specified', (WidgetTester tester) async {
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
            body: TextInputArea(
              controller: TextEditingController(),
              hintText: 'Test hint',
            ),
          ),
        ),
      );

      // Assert
      final textField = tester.widget<TextField>(find.byType(TextField));
      expect(textField.minLines, equals(4));
      expect(textField.maxLines, equals(4));
    });

    testWidgets('respects custom minLines and maxLines', (WidgetTester tester) async {
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
            body: TextInputArea(
              controller: TextEditingController(),
              hintText: 'Test hint',
              minLines: 2,
              maxLines: 8,
            ),
          ),
        ),
      );

      // Assert
      final textField = tester.widget<TextField>(find.byType(TextField));
      expect(textField.minLines, equals(2));
      expect(textField.maxLines, equals(8));
    });

    testWidgets('uses TextInputAction.done', (WidgetTester tester) async {
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
            body: TextInputArea(
              controller: TextEditingController(),
              hintText: 'Test hint',
            ),
          ),
        ),
      );

      // Assert
      final textField = tester.widget<TextField>(find.byType(TextField));
      expect(textField.textInputAction, equals(TextInputAction.done));
    });

    testWidgets('uses multiline keyboard type', (WidgetTester tester) async {
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
            body: TextInputArea(
              controller: TextEditingController(),
              hintText: 'Test hint',
            ),
          ),
        ),
      );

      // Assert
      final textField = tester.widget<TextField>(find.byType(TextField));
      expect(textField.keyboardType, equals(TextInputType.multiline));
    });

    testWidgets('supports text capitalization', (WidgetTester tester) async {
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
            body: TextInputArea(
              controller: TextEditingController(),
              hintText: 'Test hint',
            ),
          ),
        ),
      );

      // Assert
      final textField = tester.widget<TextField>(find.byType(TextField));
      expect(textField.textCapitalization, equals(TextCapitalization.sentences));
    });

    testWidgets('clears text when controller is cleared', (WidgetTester tester) async {
      // Arrange
      final controller = TextEditingController();

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
            body: TextInputArea(
              controller: controller,
              hintText: 'Test hint',
            ),
          ),
        ),
      );

      await tester.enterText(find.byType(TextField), 'Test text');
      expect(controller.text, equals('Test text'));

      // Act
      controller.clear();
      await tester.pump();

      // Assert
      expect(controller.text, isEmpty);
    });

    testWidgets('preserves text when widget rebuilds', (WidgetTester tester) async {
      // Arrange
      final controller = TextEditingController(text: 'Initial text');

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
            body: TextInputArea(
              controller: controller,
              hintText: 'Test hint',
            ),
          ),
        ),
      );

      // Act - rebuild widget
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
            body: TextInputArea(
              controller: controller,
              hintText: 'Test hint',
            ),
          ),
        ),
      );

      // Assert
      expect(controller.text, equals('Initial text'));
    });
  });
}
