import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/content/content_editable_field.dart';

void main() {
  group('ContentEditableField', () {
    testWidgets('renders with label and value', (WidgetTester tester) async {
      // Arrange
      const testValue = 'Test content';
      const testLabel = 'Title';

      // Act
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: ContentEditableField(
              value: testValue,
              label: testLabel,
              onChanged: (_) {},
            ),
          ),
        ),
      );

      // Assert - label appears as separate Text widget
      expect(find.text(testLabel), findsWidgets);
      // Value appears in the TextField
      final textField = tester.widget<TextField>(find.byType(TextField));
      expect(textField.controller?.text, equals(testValue));
    });

    testWidgets('calls onChanged when text changes', (WidgetTester tester) async {
      // Arrange
      String? changedValue;
      const newText = 'New text';

      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: ContentEditableField(
              value: '',
              label: 'Title',
              onChanged: (value) => changedValue = value,
            ),
          ),
        ),
      );

      // Act
      await tester.enterText(find.byType(TextField), newText);

      // Assert
      expect(changedValue, equals(newText));
    });

    testWidgets('respects enabled state', (WidgetTester tester) async {
      // Arrange & Act
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: ContentEditableField(
              value: 'Test',
              label: 'Title',
              onChanged: (_) {},
              enabled: false,
            ),
          ),
        ),
      );

      // Assert
      final textField = tester.widget<TextField>(find.byType(TextField));
      expect(textField.enabled, isFalse);
    });

    testWidgets('respects maxLines constraint', (WidgetTester tester) async {
      // Arrange & Act
      const maxLines = 3;
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: ContentEditableField(
              value: 'Test',
              label: 'Description',
              onChanged: (_) {},
              maxLines: maxLines,
            ),
          ),
        ),
      );

      // Assert
      final textField = tester.widget<TextField>(find.byType(TextField));
      expect(textField.maxLines, equals(maxLines));
    });

    testWidgets('respects minLines constraint', (WidgetTester tester) async {
      // Arrange & Act
      const minLines = 2;
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: ContentEditableField(
              value: 'Test',
              label: 'Description',
              onChanged: (_) {},
              minLines: minLines,
            ),
          ),
        ),
      );

      // Assert
      final textField = tester.widget<TextField>(find.byType(TextField));
      expect(textField.minLines, equals(minLines));
    });

    testWidgets('uses custom hint text when provided', (WidgetTester tester) async {
      // Arrange
      const customHint = 'Enter your title here';

      // Act
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: ContentEditableField(
              value: '',
              label: 'Title',
              onChanged: (_) {},
              hintText: customHint,
            ),
          ),
        ),
      );

      // Assert
      final textField = tester.widget<TextField>(find.byType(TextField));
      expect(textField.decoration?.hintText, equals(customHint));
    });

    testWidgets('uses label as hint when no custom hint provided', (WidgetTester tester) async {
      // Arrange
      const label = 'Title';

      // Act
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: ContentEditableField(
              value: '',
              label: label,
              onChanged: (_) {},
            ),
          ),
        ),
      );

      // Assert
      final textField = tester.widget<TextField>(find.byType(TextField));
      expect(textField.decoration?.hintText, equals(label));
    });

    testWidgets('applies correct styling in light theme', (WidgetTester tester) async {
      // Arrange & Act
      await tester.pumpWidget(
        MaterialApp(
          theme: ThemeData.light(),
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,

          home: Scaffold(
            body: ContentEditableField(
              value: 'Test',
              label: 'Title',
              onChanged: (_) {},
            ),
          ),
        ),
      );

      // Assert
      final textField = tester.widget<TextField>(find.byType(TextField));
      final decoration = textField.decoration!;

      // Verify filled and fillColor are set
      expect(decoration.filled, isTrue);
      expect(decoration.fillColor, isNotNull);
    });

    testWidgets('applies correct styling in dark theme', (WidgetTester tester) async {
      // Arrange & Act
      await tester.pumpWidget(
        MaterialApp(
          theme: ThemeData.dark(),
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,

          home: Scaffold(
            body: ContentEditableField(
              value: 'Test',
              label: 'Title',
              onChanged: (_) {},
            ),
          ),
        ),
      );

      // Assert
      final textField = tester.widget<TextField>(find.byType(TextField));
      final decoration = textField.decoration!;

      // Verify filled and fillColor are set
      expect(decoration.filled, isTrue);
      expect(decoration.fillColor, isNotNull);
    });

    testWidgets('applies overlay style when useOverlayStyle is true', (WidgetTester tester) async {
      // Arrange & Act
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: ContentEditableField(
              value: 'Test',
              label: 'Title',
              onChanged: (_) {},
              useOverlayStyle: true,
            ),
          ),
        ),
      );

      // Assert
      final textField = tester.widget<TextField>(find.byType(TextField));
      final decoration = textField.decoration!;

      // Verify overlay styling
      expect(decoration.filled, isTrue);
      expect(decoration.fillColor, equals(OverlayTokens.fieldFill));

      // Verify border styling (overlay uses 12px radius and 2px width)
      final border = decoration.border! as OutlineInputBorder;
      expect(border.borderRadius, equals(BorderRadius.circular(12)));
      expect(border.borderSide.width, equals(2.0));
      expect(border.borderSide.color, equals(GlassTokens.border));

      // Verify focused border is white
      final focusedBorder = decoration.focusedBorder! as OutlineInputBorder;
      expect(focusedBorder.borderSide.color, equals(Colors.white));
    });

    testWidgets('hides label when useOverlayStyle is true', (WidgetTester tester) async {
      // Arrange
      const testLabel = 'Title';

      // Act
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: ContentEditableField(
              value: 'Test',
              label: testLabel,
              onChanged: (_) {},
              useOverlayStyle: true,
            ),
          ),
        ),
      );

      // Assert - label should not appear as separate Text widget (only as hint)
      expect(find.text(testLabel), findsOneWidget); // Only in hint, not as label
      expect(find.widgetWithText(Padding, testLabel), findsNothing); // No label Padding
    });

    testWidgets('applies white text color when useOverlayStyle is true', (WidgetTester tester) async {
      // Arrange & Act
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: ContentEditableField(
              value: 'Test',
              label: 'Title',
              onChanged: (_) {},
              useOverlayStyle: true,
            ),
          ),
        ),
      );

      // Assert
      final textField = tester.widget<TextField>(find.byType(TextField));
      expect(textField.style?.color, equals(Colors.white));

      final decoration = textField.decoration!;
      expect(decoration.hintStyle?.color, equals(OverlayTokens.textFaint));
    });
  });
}
