import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/creation/input_mode_toggle.dart';

void main() {
  group('InputModeToggle', () {
    testWidgets('renders both Text and Image buttons', (WidgetTester tester) async {
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
            body: InputModeToggle(
              currentMode: CreationInputMode.text,
              onModeChanged: (_) {},
              isLoading: false,
            ),
          ),
        ),
      );

      // Assert
      expect(find.text('Text'), findsOneWidget);
      expect(find.text('Image'), findsOneWidget);
    });

    testWidgets('highlights selected mode with primary color', (WidgetTester tester) async {
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
            body: InputModeToggle(
              currentMode: CreationInputMode.text,
              onModeChanged: (_) {},
              isLoading: false,
            ),
          ),
        ),
      );

      // Assert - find the Text button container
      final textContainer = tester.widget<Container>(
        find.descendant(
          of: find.ancestor(
            of: find.text('Text'),
            matching: find.byType(GestureDetector),
          ),
          matching: find.byType(Container),
        ).first,
      );

      // Text mode is selected, so it should have primary color background
      final decoration = textContainer.decoration! as BoxDecoration;
      expect(decoration.color, isNot(Colors.transparent));
    });

    testWidgets('calls onModeChanged when tapping inactive mode', (WidgetTester tester) async {
      // Arrange
      CreationInputMode? changedMode;

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
            body: InputModeToggle(
              currentMode: CreationInputMode.text,
              onModeChanged: (mode) => changedMode = mode,
              isLoading: false,
            ),
          ),
        ),
      );

      // Act - tap on Image mode
      await tester.tap(find.text('Image'));
      await tester.pump();

      // Assert
      expect(changedMode, equals(CreationInputMode.image));
    });

    testWidgets('calls onModeChanged when tapping Text mode', (WidgetTester tester) async {
      // Arrange
      CreationInputMode? changedMode;

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
            body: InputModeToggle(
              currentMode: CreationInputMode.image,
              onModeChanged: (mode) => changedMode = mode,
              isLoading: false,
            ),
          ),
        ),
      );

      // Act - tap on Text mode
      await tester.tap(find.text('Text'));
      await tester.pump();

      // Assert
      expect(changedMode, equals(CreationInputMode.text));
    });

    testWidgets('disables interaction when isLoading is true', (WidgetTester tester) async {
      // Arrange
      CreationInputMode? changedMode;

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
            body: InputModeToggle(
              currentMode: CreationInputMode.text,
              onModeChanged: (mode) => changedMode = mode,
              isLoading: true,
            ),
          ),
        ),
      );

      // Act - try to tap Image mode
      await tester.tap(find.text('Image'));
      await tester.pump();

      // Assert - should not call callback
      expect(changedMode, isNull);
    });

    testWidgets('switches from text to image mode', (WidgetTester tester) async {
      // Arrange
      CreationInputMode currentMode = CreationInputMode.text;

      await tester.pumpWidget(
        StatefulBuilder(
          builder: (context, setState) {
            return MaterialApp(
              localizationsDelegates: const [
                AppLocalizations.delegate,
                GlobalMaterialLocalizations.delegate,
                GlobalWidgetsLocalizations.delegate,
                GlobalCupertinoLocalizations.delegate,
              ],
              supportedLocales: AppLocalizations.supportedLocales,
              home: Scaffold(
                body: InputModeToggle(
                  currentMode: currentMode,
                  onModeChanged: (mode) {
                    setState(() {
                      currentMode = mode;
                    });
                  },
                  isLoading: false,
                ),
              ),
            );
          },
        ),
      );

      // Initial state
      expect(currentMode, equals(CreationInputMode.text));

      // Act - tap Image button
      await tester.tap(find.text('Image'));
      await tester.pump();

      // Assert
      expect(currentMode, equals(CreationInputMode.image));
    });

    testWidgets('switches from image to text mode', (WidgetTester tester) async {
      // Arrange
      CreationInputMode currentMode = CreationInputMode.image;

      await tester.pumpWidget(
        StatefulBuilder(
          builder: (context, setState) {
            return MaterialApp(
              localizationsDelegates: const [
                AppLocalizations.delegate,
                GlobalMaterialLocalizations.delegate,
                GlobalWidgetsLocalizations.delegate,
                GlobalCupertinoLocalizations.delegate,
              ],
              supportedLocales: AppLocalizations.supportedLocales,
              home: Scaffold(
                body: InputModeToggle(
                  currentMode: currentMode,
                  onModeChanged: (mode) {
                    setState(() {
                      currentMode = mode;
                    });
                  },
                  isLoading: false,
                ),
              ),
            );
          },
        ),
      );

      // Initial state
      expect(currentMode, equals(CreationInputMode.image));

      // Act - tap Text button
      await tester.tap(find.text('Text'));
      await tester.pump();

      // Assert
      expect(currentMode, equals(CreationInputMode.text));
    });

    testWidgets('has correct height and border radius', (WidgetTester tester) async {
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
            body: InputModeToggle(
              currentMode: CreationInputMode.text,
              onModeChanged: (_) {},
              isLoading: false,
            ),
          ),
        ),
      );

      // Assert
      final container = tester.widget<Container>(
        find.descendant(
          of: find.byType(InputModeToggle),
          matching: find.byType(Container),
        ).first,
      );

      expect(container.constraints?.maxHeight, equals(40));

      final decoration = container.decoration! as BoxDecoration;
      expect(decoration.borderRadius, equals(BorderRadius.circular(20)));
    });
  });
}
