import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';

void main() {
  group('ContentViewHelpers', () {
    testWidgets('showDeleteConfirmation shows dialog with content type',
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
          home: Builder(
            builder: (context) {
              return Scaffold(
                body: ElevatedButton(
                  onPressed: () async {
                    final confirmed =
                        await ContentViewHelpers.showDeleteConfirmation(
                      context: context,
                      contentType: 'Gear',
                    );
                    ScaffoldMessenger.of(context).showSnackBar(
                      SnackBar(
                        content: Text(confirmed ? 'Confirmed' : 'Cancelled'),
                      ),
                    );
                  },
                  child: const Text('Show Dialog'),
                ),
              );
            },
          ),
        ),
      );

      // Tap button to show dialog
      await tester.tap(find.text('Show Dialog'));
      await tester.pumpAndSettle();

      // Verify dialog content
      expect(find.text('Delete Gear'), findsOneWidget);
      expect(
        find.textContaining('Are you sure you want to delete this Gear?'),
        findsOneWidget,
      );
      expect(find.text('Cancel'), findsOneWidget);
      expect(find.text('Delete'), findsOneWidget);

      // Tap cancel
      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();

      // Verify snackbar shows cancelled
      expect(find.text('Cancelled'), findsOneWidget);
    });

    testWidgets('showDeleteConfirmation returns true when confirmed',
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
          home: Builder(
            builder: (context) {
              return Scaffold(
                body: ElevatedButton(
                  onPressed: () async {
                    final confirmed =
                        await ContentViewHelpers.showDeleteConfirmation(
                      context: context,
                      contentType: 'Request',
                      contentName: 'Test Request',
                    );
                    ScaffoldMessenger.of(context).showSnackBar(
                      SnackBar(
                        content: Text(confirmed ? 'Confirmed' : 'Cancelled'),
                      ),
                    );
                  },
                  child: const Text('Show Dialog'),
                ),
              );
            },
          ),
        ),
      );

      // Tap button to show dialog
      await tester.tap(find.text('Show Dialog'));
      await tester.pumpAndSettle();

      // Verify dialog shows content name
      expect(
        find.textContaining('delete "Test Request"'),
        findsOneWidget,
      );

      // Tap delete
      await tester.tap(find.text('Delete'));
      await tester.pumpAndSettle();

      // Verify snackbar shows confirmed
      expect(find.text('Confirmed'), findsOneWidget);
    });

    testWidgets('handleSave calls save function',
        (WidgetTester tester) async {
      var saveCalled = false;

      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: Builder(
            builder: (context) {
              return Scaffold(
                body: ElevatedButton(
                  onPressed: () async {
                    await ContentViewHelpers.handleSave(
                      context: context,
                      saveFunction: () async {
                        saveCalled = true;
                      },
                    );
                  },
                  child: const Text('Save'),
                ),
              );
            },
          ),
        ),
      );

      // Tap save button
      await tester.tap(find.text('Save'));
      await tester.pumpAndSettle();

      expect(saveCalled, isTrue);
    });

    testWidgets('handleSave shows error message on failure',
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
          home: Builder(
            builder: (context) {
              return Scaffold(
                body: ElevatedButton(
                  onPressed: () async {
                    await ContentViewHelpers.handleSave(
                      context: context,
                      saveFunction: () async {
                        throw Exception('Save failed');
                      },
                    );
                  },
                  child: const Text('Save'),
                ),
              );
            },
          ),
        ),
      );

      // Tap save button
      await tester.tap(find.text('Save'));
      await tester.pumpAndSettle();

      expect(
        find.textContaining('Failed to save'),
        findsOneWidget,
      );
    });

    testWidgets('handleDelete calls delete function and callback',
        (WidgetTester tester) async {
      var deleteCalled = false;
      var callbackCalled = false;

      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: Builder(
            builder: (context) {
              return Scaffold(
                body: ElevatedButton(
                  onPressed: () async {
                    await ContentViewHelpers.handleDelete(
                      context: context,
                      deleteFunction: () async {
                        deleteCalled = true;
                      },
                      onDeleted: () => callbackCalled = true,
                      successMessage: 'Item deleted!',
                    );
                  },
                  child: const Text('Delete'),
                ),
              );
            },
          ),
        ),
      );

      // Tap delete button
      await tester.tap(find.text('Delete'));
      await tester.pumpAndSettle();

      expect(deleteCalled, isTrue);
      expect(callbackCalled, isTrue);
      expect(find.text('Item deleted!'), findsOneWidget);
    });
  });
}
