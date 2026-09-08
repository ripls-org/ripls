import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/settings/timezone_picker.dart';
import 'package:timezone/data/latest.dart' as tz;

void main() {
  // Initialize timezone database once for all tests
  setUpAll(() {
    tz.initializeTimeZones();
  });

  group('TimezonePicker', () {
    testWidgets('displays timezone list', (WidgetTester tester) async {
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: TimezonePicker(
            currentTimezone: null,
            onTimezoneSelected: (_) {},
          ),
        ),
      );

      // Verify AppBar title
      expect(find.text('Timezone'), findsOneWidget);

      // Verify search field exists
      expect(find.byType(TextField), findsOneWidget);

      // Verify at least some common timezones are displayed
      expect(find.byType(ListTile), findsWidgets);
    });

    testWidgets('displays current timezone with check icon',
        (WidgetTester tester) async {
      const currentTz = 'Etc/UTC';

      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: TimezonePicker(
            currentTimezone: currentTz,
            onTimezoneSelected: (_) {},
          ),
        ),
      );

      await tester.pumpAndSettle();

      // Search for UTC to make it visible
      await tester.enterText(find.byType(TextField), 'UTC');
      await tester.pumpAndSettle();

      // Verify check icon exists for selected timezone
      expect(find.byIcon(Icons.check), findsOneWidget);
    });

    testWidgets('filters timezones based on search query',
        (WidgetTester tester) async {
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: TimezonePicker(
            currentTimezone: null,
            onTimezoneSelected: (_) {},
          ),
        ),
      );

      await tester.pumpAndSettle();

      // Enter specific search query
      await tester.enterText(find.byType(TextField), 'UTC');
      await tester.pumpAndSettle();

      // Verify specific timezone is shown in the list
      // Note: find.text('UTC') will find 2 widgets - one in TextField, one in ListTile
      expect(find.text('UTC'), findsWidgets);

      // Verify only a few results (not the full list)
      final filteredCount = tester.widgetList(find.byType(ListTile)).length;
      expect(filteredCount, lessThan(10)); // UTC is a short list
    });

    testWidgets('clears search when clear button is tapped',
        (WidgetTester tester) async {
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: TimezonePicker(
            currentTimezone: null,
            onTimezoneSelected: (_) {},
          ),
        ),
      );

      // Enter search query
      await tester.enterText(find.byType(TextField), 'tokyo');
      await tester.pumpAndSettle();

      // Tap clear button
      await tester.tap(find.byIcon(Icons.clear));
      await tester.pumpAndSettle();

      // Verify search field is empty
      final textField = tester.widget<TextField>(find.byType(TextField));
      expect(textField.controller?.text, isEmpty);
    });

    testWidgets('calls onTimezoneSelected when timezone is tapped',
        (WidgetTester tester) async {
      String? selectedTimezone;

      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: TimezonePicker(
            currentTimezone: null,
            onTimezoneSelected: (tz) => selectedTimezone = tz,
          ),
        ),
      );

      await tester.pumpAndSettle();

      // Search for a specific timezone
      await tester.enterText(find.byType(TextField), 'GMT');
      await tester.pumpAndSettle();

      // Tap on the ListTile (not the text to avoid ambiguity)
      await tester.tap(find.byType(ListTile).first);
      await tester.pumpAndSettle();

      // Verify callback was called
      expect(selectedTimezone, isNotNull);
    });

    testWidgets('closes screen after timezone selection',
        (WidgetTester tester) async {
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
            body: Builder(
              builder: (context) => ElevatedButton(
                onPressed: () {
                  Navigator.push(
                    context,
                    MaterialPageRoute(
                      builder: (context) => TimezonePicker(
                        currentTimezone: null,
                        onTimezoneSelected: (_) {},
                      ),
                    ),
                  );
                },
                child: const Text('Open'),
              ),
            ),
          ),
        ),
      );

      // Open TimezonePicker
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      // Verify TimezonePicker is shown
      expect(find.text('Timezone'), findsOneWidget);

      // Search and select a timezone - tap on ListTile instead of text
      await tester.enterText(find.byType(TextField), 'GMT');
      await tester.pumpAndSettle();

      // Find and tap the first ListTile
      await tester.tap(find.byType(ListTile).first);
      await tester.pumpAndSettle();

      // Verify TimezonePicker is closed (back to original screen)
      expect(find.text('Timezone'), findsNothing);
      expect(find.text('Open'), findsOneWidget);
    });

    testWidgets('formats timezone names correctly',
        (WidgetTester tester) async {
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: TimezonePicker(
            currentTimezone: null,
            onTimezoneSelected: (_) {},
          ),
        ),
      );

      await tester.pumpAndSettle();

      // Search for timezone with underscores
      await tester.enterText(find.byType(TextField), 'los_angeles');
      await tester.pumpAndSettle();

      // Verify underscores are replaced with spaces in display
      expect(find.text('America/Los Angeles'), findsWidgets);
    });

    testWidgets('search is case insensitive', (WidgetTester tester) async {
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: TimezonePicker(
            currentTimezone: null,
            onTimezoneSelected: (_) {},
          ),
        ),
      );

      await tester.pumpAndSettle();

      // Search with uppercase
      await tester.enterText(find.byType(TextField), 'TOKYO');
      await tester.pumpAndSettle();

      // Verify results are found (list is not empty)
      expect(find.byType(ListTile), findsWidgets);
    });

    testWidgets('displays all timezones when search is empty',
        (WidgetTester tester) async {
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: TimezonePicker(
            currentTimezone: null,
            onTimezoneSelected: (_) {},
          ),
        ),
      );

      await tester.pumpAndSettle();

      // Get initial count of list tiles
      final initialCount = tester.widgetList(find.byType(ListTile)).length;

      // Enter and clear search
      await tester.enterText(find.byType(TextField), 'test');
      await tester.pumpAndSettle();
      await tester.tap(find.byIcon(Icons.clear));
      await tester.pumpAndSettle();

      // Verify all timezones are shown again
      final finalCount = tester.widgetList(find.byType(ListTile)).length;
      expect(finalCount, equals(initialCount));
    });

    testWidgets('handles no search results gracefully',
        (WidgetTester tester) async {
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: TimezonePicker(
            currentTimezone: null,
            onTimezoneSelected: (_) {},
          ),
        ),
      );

      await tester.pumpAndSettle();

      // Search for non-existent timezone
      await tester.enterText(
        find.byType(TextField),
        'nonexistenttiemzone12345',
      );
      await tester.pumpAndSettle();

      // Verify no list tiles are shown
      expect(find.byType(ListTile), findsNothing);
    });
  });
}
