import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/content/content_overflow_menu.dart';

void main() {
  group('ContentOverflowMenu', () {
    testWidgets('shows edit option when showEdit is true',
        (WidgetTester tester) async {
      bool editTapped = false;

      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.lightTheme,
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
                onPressed: () => showContentOverflowMenu(
                  context: context,
                  config: ContentOverflowMenuConfig(
                    showEdit: true,
                    onEdit: () => editTapped = true,
                  ),
                ),
                child: const Text('Open Menu'),
              ),
            ),
          ),
        ),
      );

      // Tap to open menu
      await tester.tap(find.text('Open Menu'));
      await tester.pumpAndSettle();

      // Verify edit option is shown
      expect(find.text('Edit'), findsOneWidget);
      expect(find.byIcon(Icons.edit), findsOneWidget);

      // Tap edit option
      await tester.tap(find.text('Edit'));
      await tester.pumpAndSettle();

      // Verify callback was invoked
      expect(editTapped, true);
    });

    testWidgets('shows share option when showShare is true',
        (WidgetTester tester) async {
      bool shareTapped = false;

      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.lightTheme,
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
                onPressed: () => showContentOverflowMenu(
                  context: context,
                  config: ContentOverflowMenuConfig(
                    showShare: true,
                    onShare: () => shareTapped = true,
                  ),
                ),
                child: const Text('Open Menu'),
              ),
            ),
          ),
        ),
      );

      await tester.tap(find.text('Open Menu'));
      await tester.pumpAndSettle();

      expect(find.text('Share Link'), findsOneWidget);
      expect(find.byIcon(Icons.share), findsOneWidget);

      await tester.tap(find.text('Share Link'));
      await tester.pumpAndSettle();

      expect(shareTapped, true);
    });

    testWidgets('shows delete option when showDelete is true',
        (WidgetTester tester) async {
      bool deleteTapped = false;

      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.lightTheme,
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
                onPressed: () => showContentOverflowMenu(
                  context: context,
                  config: ContentOverflowMenuConfig(
                    showDelete: true,
                    onDelete: () => deleteTapped = true,
                  ),
                ),
                child: const Text('Open Menu'),
              ),
            ),
          ),
        ),
      );

      await tester.tap(find.text('Open Menu'));
      await tester.pumpAndSettle();

      expect(find.text('Delete'), findsOneWidget);
      expect(find.byIcon(Icons.delete_outline), findsOneWidget);

      await tester.tap(find.text('Delete'));
      await tester.pumpAndSettle();

      expect(deleteTapped, true);
    });

    testWidgets('shows report option when showReport is true',
        (WidgetTester tester) async {
      bool reportTapped = false;

      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.lightTheme,
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
                onPressed: () => showContentOverflowMenu(
                  context: context,
                  config: ContentOverflowMenuConfig(
                    showReport: true,
                    onReport: () => reportTapped = true,
                  ),
                ),
                child: const Text('Open Menu'),
              ),
            ),
          ),
        ),
      );

      await tester.tap(find.text('Open Menu'));
      await tester.pumpAndSettle();

      expect(find.text('Report'), findsOneWidget);
      expect(find.byIcon(Icons.flag_outlined), findsOneWidget);

      await tester.tap(find.text('Report'));
      await tester.pumpAndSettle();

      expect(reportTapped, true);
    });

    testWidgets('hides options when corresponding show flags are false',
        (WidgetTester tester) async {
      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.lightTheme,
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
                onPressed: () => showContentOverflowMenu(
                  context: context,
                  config: const ContentOverflowMenuConfig(
                    showEdit: false,
                    showShare: false,
                    showDelete: false,
                    showReport: false,
                  ),
                ),
                child: const Text('Open Menu'),
              ),
            ),
          ),
        ),
      );

      await tester.tap(find.text('Open Menu'));
      await tester.pumpAndSettle();

      // Verify no menu items are shown
      expect(find.text('Edit'), findsNothing);
      expect(find.text('Share Link'), findsNothing);
      expect(find.text('Delete'), findsNothing);
      expect(find.text('Report'), findsNothing);
    });

    testWidgets('shows custom menu items', (WidgetTester tester) async {
      bool customTapped = false;

      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.lightTheme,
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
                onPressed: () => showContentOverflowMenu(
                  context: context,
                  config: ContentOverflowMenuConfig(
                    customItems: [
                      MenuItemConfig(
                        label: 'Custom Action',
                        icon: Icons.star,
                        onTap: () => customTapped = true,
                      ),
                    ],
                  ),
                ),
                child: const Text('Open Menu'),
              ),
            ),
          ),
        ),
      );

      await tester.tap(find.text('Open Menu'));
      await tester.pumpAndSettle();

      expect(find.text('Custom Action'), findsOneWidget);
      expect(find.byIcon(Icons.star), findsOneWidget);

      await tester.tap(find.text('Custom Action'));
      await tester.pumpAndSettle();

      expect(customTapped, true);
    });

    testWidgets('shows multiple menu items in correct order',
        (WidgetTester tester) async {
      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.lightTheme,
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
                onPressed: () => showContentOverflowMenu(
                  context: context,
                  config: ContentOverflowMenuConfig(
                    showEdit: true,
                    showShare: true,
                    showDelete: true,
                    showReport: true,
                    onEdit: () {},
                    onShare: () {},
                    onDelete: () {},
                    onReport: () {},
                  ),
                ),
                child: const Text('Open Menu'),
              ),
            ),
          ),
        ),
      );

      await tester.tap(find.text('Open Menu'));
      await tester.pumpAndSettle();

      // Verify all items are shown
      expect(find.text('Edit'), findsOneWidget);
      expect(find.text('Share Link'), findsOneWidget);
      expect(find.text('Delete'), findsOneWidget);
      expect(find.text('Report'), findsOneWidget);

      // Verify order (Edit, Share, Report, Delete — delete is last as most
      // destructive). After the glass-modal migration the menu rows are no
      // longer ListTiles; collect Text widgets in tree order and filter to the
      // four labels we expect.
      const expected = ['Edit', 'Share Link', 'Report', 'Delete'];
      final allTexts = tester
          .widgetList<Text>(find.byType(Text))
          .map((t) => t.data)
          .where((t) => t != null && expected.contains(t))
          .toList();
      expect(allTexts, expected);
    });

    testWidgets('closes menu after tapping an item',
        (WidgetTester tester) async {
      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.lightTheme,
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
                onPressed: () => showContentOverflowMenu(
                  context: context,
                  config: ContentOverflowMenuConfig(
                    showEdit: true,
                    onEdit: () {},
                  ),
                ),
                child: const Text('Open Menu'),
              ),
            ),
          ),
        ),
      );

      await tester.tap(find.text('Open Menu'));
      await tester.pumpAndSettle();

      // Verify menu is open
      expect(find.text('Edit'), findsOneWidget);

      // Tap edit
      await tester.tap(find.text('Edit'));
      await tester.pumpAndSettle();

      // Verify menu is closed
      expect(find.text('Edit'), findsNothing);
    });

    testWidgets('shows handle bar at top', (WidgetTester tester) async {
      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.lightTheme,
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
                onPressed: () => showContentOverflowMenu(
                  context: context,
                  config: ContentOverflowMenuConfig(
                    showEdit: true,
                    onEdit: () {},
                  ),
                ),
                child: const Text('Open Menu'),
              ),
            ),
          ),
        ),
      );

      await tester.tap(find.text('Open Menu'));
      await tester.pumpAndSettle();

      // Find the handle bar (Container with specific dimensions)
      final handleBars = tester.widgetList<Container>(
        find.byWidgetPredicate(
          (widget) =>
              widget is Container &&
              widget.constraints?.maxWidth == 40 &&
              widget.constraints?.maxHeight == 4,
        ),
      );

      expect(handleBars.isNotEmpty, true);
    });

    testWidgets('does not invoke callback if callback is null',
        (WidgetTester tester) async {
      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.lightTheme,
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
                onPressed: () => showContentOverflowMenu(
                  context: context,
                  config: const ContentOverflowMenuConfig(
                    showEdit: true,
                    onEdit: null, // Callback is null
                  ),
                ),
                child: const Text('Open Menu'),
              ),
            ),
          ),
        ),
      );

      await tester.tap(find.text('Open Menu'));
      await tester.pumpAndSettle();

      // Edit should not be shown if callback is null
      expect(find.text('Edit'), findsNothing);
    });

    testWidgets('supports dark theme', (WidgetTester tester) async {
      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.darkTheme,
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
                onPressed: () => showContentOverflowMenu(
                  context: context,
                  config: ContentOverflowMenuConfig(
                    showEdit: true,
                    onEdit: () {},
                  ),
                ),
                child: const Text('Open Menu'),
              ),
            ),
          ),
        ),
      );

      await tester.tap(find.text('Open Menu'));
      await tester.pumpAndSettle();

      // Verify menu renders (no errors in dark theme)
      expect(find.text('Edit'), findsOneWidget);
    });

    testWidgets('custom items appear after standard items',
        (WidgetTester tester) async {
      await tester.pumpWidget(
        MaterialApp(
          theme: AppTheme.lightTheme,
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
                onPressed: () => showContentOverflowMenu(
                  context: context,
                  config: ContentOverflowMenuConfig(
                    showEdit: true,
                    showShare: true,
                    onEdit: () {},
                    onShare: () {},
                    customItems: [
                      MenuItemConfig(
                        label: 'Custom 1',
                        icon: Icons.star,
                        onTap: () {},
                      ),
                      MenuItemConfig(
                        label: 'Custom 2',
                        icon: Icons.favorite,
                        onTap: () {},
                      ),
                    ],
                  ),
                ),
                child: const Text('Open Menu'),
              ),
            ),
          ),
        ),
      );

      await tester.tap(find.text('Open Menu'));
      await tester.pumpAndSettle();

      // Verify order: Edit, Share Link, Custom 1, Custom 2 — see the
      // ordering note in the earlier test for why we filter Text widgets.
      const expected = ['Edit', 'Share Link', 'Custom 1', 'Custom 2'];
      final allTexts = tester
          .widgetList<Text>(find.byType(Text))
          .map((t) => t.data)
          .where((t) => t != null && expected.contains(t))
          .toList();
      expect(allTexts, expected);
    });
  });
}
