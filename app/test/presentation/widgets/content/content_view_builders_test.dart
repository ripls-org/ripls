import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/content/content_view_builders.dart';

void main() {
  group('ContentViewBuilders', () {
    testWidgets('buildLoadingView shows loading indicator',
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
            body: ContentViewBuilders.buildLoadingView(),
          ),
        ),
      );

      expect(find.byType(CircularProgressIndicator), findsOneWidget);
      expect(find.byType(Container), findsOneWidget);
    });

    testWidgets('buildErrorView shows error message and retry button',
        (WidgetTester tester) async {
      var retryPressed = false;

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
            body: ContentViewBuilders.buildErrorView(
              title: 'Test Error',
              errorMessage: 'Something went wrong',
              onRetry: () => retryPressed = true,
            ),
          ),
        ),
      );

      expect(find.text('Test Error'), findsOneWidget);
      expect(find.text('Something went wrong'), findsOneWidget);

      // Tap retry button
      await tester.tap(find.text('Retry'));
      await tester.pump();

      expect(retryPressed, isTrue);
    });

    testWidgets('buildExpandableDescription toggles expansion',
        (WidgetTester tester) async {
      var isExpanded = false;
      const testDescription = 'This is a test description';

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
            body: StatefulBuilder(
              builder: (context, setState) {
                return ContentViewBuilders.buildExpandableDescription(
                  context: context,
                  description: testDescription,
                  isExpanded: isExpanded,
                  onTap: () => setState(() => isExpanded = !isExpanded),
                );
              },
            ),
          ),
        ),
      );

      expect(find.text(testDescription), findsOneWidget);

      // Initially should have maxLines = 3 (collapsed)
      final textWidget = tester.widget<Text>(find.text(testDescription));
      expect(textWidget.maxLines, equals(3));
      expect(textWidget.overflow, equals(TextOverflow.ellipsis));

      // Tap to expand
      await tester.tap(find.text(testDescription));
      await tester.pumpAndSettle();

      // After expansion, maxLines should be null
      final expandedTextWidget =
          tester.widget<Text>(find.text(testDescription));
      expect(expandedTextWidget.maxLines, isNull);
      expect(expandedTextWidget.overflow, isNull);
    });

    testWidgets('buildEditableTitle creates editable field',
        (WidgetTester tester) async {
      var currentValue = 'Initial Title';

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
                body: ContentViewBuilders.buildEditableTitle(
                  value: currentValue,
                  onChanged: (newValue) =>
                      setState(() => currentValue = newValue),
                  enabled: true,
                  label: 'Test Title',
                  hintText: 'Enter title',
                ),
              ),
            );
          },
        ),
      );

      expect(find.text('Initial Title'), findsOneWidget);
    });

    testWidgets('buildEditableDescription creates multiline field',
        (WidgetTester tester) async {
      var currentValue = 'Initial Description';

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
                body: ContentViewBuilders.buildEditableDescription(
                  value: currentValue,
                  onChanged: (newValue) =>
                      setState(() => currentValue = newValue),
                  enabled: true,
                ),
              ),
            );
          },
        ),
      );

      expect(find.text('Initial Description'), findsOneWidget);
    });

    testWidgets('buildTitle shows read-only title',
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
              builder: (context) {
                return ContentViewBuilders.buildTitle(
                  context: context,
                  title: 'Test Title',
                );
              },
            ),
          ),
        ),
      );

      expect(find.text('Test Title'), findsOneWidget);
      final textWidget = tester.widget<Text>(find.text('Test Title'));
      expect(textWidget.style?.fontWeight, equals(FontWeight.bold));
    });

    testWidgets('buildFeedHeader shows actor info and action text',
        (WidgetTester tester) async {
      // Create a test user
      final testUser = User(
        id: 'user123',
        name: 'Test User',
        mediaId: '',
      );

      // Use a timestamp from 1 hour ago
      final oneHourAgo = DateTime.now()
          .subtract(const Duration(hours: 1))
          .millisecondsSinceEpoch ~/
          1000;

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
              builder: (context) {
                return Stack(
                  children: [
                    ContentViewBuilders.buildFeedHeader(
                      context: context,
                      actor: testUser,
                      occurredAtUnixSec: oneHourAgo,
                      actionText: 'Shared for loan',
                    ),
                  ],
                );
              },
            ),
          ),
        ),
      );

      // Verify actor name is displayed
      expect(find.text('Test User'), findsOneWidget);

      // Verify action text is displayed (with timeago)
      expect(find.textContaining('Shared for loan'), findsOneWidget);

      // Verify gradient background exists
      expect(find.byType(Container), findsWidgets);

      // Verify avatar is present
      expect(find.byType(GestureDetector), findsOneWidget);
    });

    testWidgets('buildFeedHeader displays timeago correctly',
        (WidgetTester tester) async {
      final testUser = User(
        id: 'user456',
        name: 'Another User',
        mediaId: '',
      );

      // Use a timestamp from just now
      final now = DateTime.now().millisecondsSinceEpoch ~/ 1000;

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
              builder: (context) {
                return Stack(
                  children: [
                    ContentViewBuilders.buildFeedHeader(
                      context: context,
                      actor: testUser,
                      occurredAtUnixSec: now,
                      actionText: 'Posted request',
                    ),
                  ],
                );
              },
            ),
          ),
        ),
      );

      // Timeago should show something like "a moment ago" or "just now"
      expect(find.textContaining('Posted request'), findsOneWidget);
      expect(find.text('Another User'), findsOneWidget);
    });

    testWidgets('buildFeedHeader has proper layout structure',
        (WidgetTester tester) async {
      final testUser = User(
        id: 'user789',
        name: 'Layout Test User',
        mediaId: '',
      );

      final timestamp = DateTime.now().millisecondsSinceEpoch ~/ 1000;

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
              builder: (context) {
                return Stack(
                  children: [
                    ContentViewBuilders.buildFeedHeader(
                      context: context,
                      actor: testUser,
                      occurredAtUnixSec: timestamp,
                      actionText: 'Created community',
                    ),
                  ],
                );
              },
            ),
          ),
        ),
      );

      // Verify Stack structure - buildFeedHeader returns a Stack with 2 Positioned widgets
      expect(find.byType(Stack), findsWidgets);

      // Verify Positioned widgets exist (for gradient + header)
      expect(find.byType(Positioned), findsNWidgets(2)); // gradient + header overlay

      // Verify SafeArea is used
      expect(find.byType(SafeArea), findsOneWidget);

      // Verify Row layout for avatar + text
      expect(find.byType(Row), findsOneWidget);

      // Verify gradient background with IgnorePointer exists
      expect(find.byType(IgnorePointer), findsWidgets);
    });

    group('buildLocationChip', () {
      testWidgets('shows location text', (tester) async {
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
              body: ContentViewBuilders.buildLocationChip(
                text: '123 Main St, Springfield',
                onTap: () {},
              ),
            ),
          ),
        );

        expect(find.text('123 Main St, Springfield'), findsOneWidget);
      });

      testWidgets('shows location icon', (tester) async {
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
              body: ContentViewBuilders.buildLocationChip(
                text: 'Address',
                onTap: () {},
              ),
            ),
          ),
        );

        expect(find.byIcon(Icons.location_on), findsOneWidget);
      });

      testWidgets('has no trailing chevron', (tester) async {
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
              body: ContentViewBuilders.buildLocationChip(
                text: 'Address',
                onTap: () {},
              ),
            ),
          ),
        );

        expect(find.byIcon(Icons.chevron_right), findsNothing);
      });

      testWidgets('calls onTap when tapped', (tester) async {
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
              body: ContentViewBuilders.buildLocationChip(
                text: 'Address',
                onTap: () => tapped = true,
              ),
            ),
          ),
        );

        await tester.tap(find.text('Address'));
        await tester.pump();

        expect(tapped, isTrue);
      });

      testWidgets('has rounded chip decoration', (tester) async {
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
              body: ContentViewBuilders.buildLocationChip(
                text: 'Address',
                onTap: () {},
              ),
            ),
          ),
        );

        final container = tester.widget<Container>(find.byType(Container));
        final decoration = container.decoration! as BoxDecoration;
        expect(decoration.borderRadius, equals(BorderRadius.circular(10)));
      });
    });

    group('buildTimeChip', () {
      testWidgets('shows time text', (tester) async {
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
              body: ContentViewBuilders.buildTimeChip(
                text: 'Saturday, March 15 at 2:00 PM',
                onTap: () {},
              ),
            ),
          ),
        );

        expect(find.text('Saturday, March 15 at 2:00 PM'), findsOneWidget);
      });

      testWidgets('shows time icon', (tester) async {
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
              body: ContentViewBuilders.buildTimeChip(
                text: 'Time',
                onTap: () {},
              ),
            ),
          ),
        );

        expect(find.byIcon(Icons.access_time), findsOneWidget);
      });

      testWidgets('has no trailing chevron', (tester) async {
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
              body: ContentViewBuilders.buildTimeChip(
                text: 'Time',
                onTap: () {},
              ),
            ),
          ),
        );

        expect(find.byIcon(Icons.chevron_right), findsNothing);
      });

      testWidgets('calls onTap when tapped', (tester) async {
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
              body: ContentViewBuilders.buildTimeChip(
                text: 'Time',
                onTap: () => tapped = true,
              ),
            ),
          ),
        );

        await tester.tap(find.text('Time'));
        await tester.pump();

        expect(tapped, isTrue);
      });

      testWidgets('has rounded chip decoration', (tester) async {
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
              body: ContentViewBuilders.buildTimeChip(
                text: 'Time',
                onTap: () {},
              ),
            ),
          ),
        );

        final container = tester.widget<Container>(find.byType(Container));
        final decoration = container.decoration! as BoxDecoration;
        expect(decoration.borderRadius, equals(BorderRadius.circular(10)));
      });
    });

    group('buildAgeChip', () {
      testWidgets('shows schedule icon', (tester) async {
        final postedAt = DateTime.now().subtract(const Duration(hours: 2));

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
              body: ContentViewBuilders.buildAgeChip(
                postedAt: postedAt,
                onTap: () {},
              ),
            ),
          ),
        );

        expect(find.byIcon(Icons.schedule), findsOneWidget);
      });

      testWidgets('shows "Opened just now" for very recent posts', (tester) async {
        final postedAt = DateTime.now().subtract(const Duration(seconds: 30));

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
              body: ContentViewBuilders.buildAgeChip(
                postedAt: postedAt,
                onTap: () {},
              ),
            ),
          ),
        );

        expect(find.text('Opened just now'), findsOneWidget);
      });

      testWidgets('shows minutes for posts under an hour old', (tester) async {
        final postedAt = DateTime.now().subtract(const Duration(minutes: 15));

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
              body: ContentViewBuilders.buildAgeChip(
                postedAt: postedAt,
                onTap: () {},
              ),
            ),
          ),
        );

        expect(find.text('Opened 15 min ago'), findsOneWidget);
      });

      testWidgets('shows hours for posts under a day old', (tester) async {
        final postedAt = DateTime.now().subtract(const Duration(hours: 5));

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
              body: ContentViewBuilders.buildAgeChip(
                postedAt: postedAt,
                onTap: () {},
              ),
            ),
          ),
        );

        expect(find.text('Opened 5 hrs ago'), findsOneWidget);
      });

      testWidgets('shows days for posts under a week old', (tester) async {
        final postedAt = DateTime.now().subtract(const Duration(days: 3));

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
              body: ContentViewBuilders.buildAgeChip(
                postedAt: postedAt,
                onTap: () {},
              ),
            ),
          ),
        );

        expect(find.text('Opened 3 days ago'), findsOneWidget);
      });

      testWidgets('shows formatted date for posts a week or older', (tester) async {
        // Use a fixed date so the formatted output is predictable
        final postedAt = DateTime(2026, 2, 13); // Friday, Feb 13

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
              body: ContentViewBuilders.buildAgeChip(
                postedAt: postedAt,
                onTap: () {},
              ),
            ),
          ),
        );

        expect(find.text('Opened Fri, Feb 13'), findsOneWidget);
      });

      testWidgets('has no trailing chevron', (tester) async {
        final postedAt = DateTime.now().subtract(const Duration(hours: 1));

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
              body: ContentViewBuilders.buildAgeChip(
                postedAt: postedAt,
                onTap: () {},
              ),
            ),
          ),
        );

        expect(find.byIcon(Icons.chevron_right), findsNothing);
      });

      testWidgets('calls onTap when tapped', (tester) async {
        bool tapped = false;
        final postedAt = DateTime.now().subtract(const Duration(hours: 1));

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
              body: ContentViewBuilders.buildAgeChip(
                postedAt: postedAt,
                onTap: () => tapped = true,
              ),
            ),
          ),
        );

        await tester.tap(find.byIcon(Icons.schedule));
        await tester.pump();

        expect(tapped, isTrue);
      });

      testWidgets('has rounded chip decoration', (tester) async {
        final postedAt = DateTime.now().subtract(const Duration(hours: 1));

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
              body: ContentViewBuilders.buildAgeChip(
                postedAt: postedAt,
                onTap: () {},
              ),
            ),
          ),
        );

        final container = tester.widget<Container>(find.byType(Container));
        final decoration = container.decoration! as BoxDecoration;
        expect(decoration.borderRadius, equals(BorderRadius.circular(10)));
      });
    });

    group('buildUrlChip', () {
      testWidgets('shows url text', (tester) async {
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
              body: ContentViewBuilders.buildUrlChip(
                text: 'https://example.com',
                onTap: () {},
              ),
            ),
          ),
        );

        expect(find.text('https://example.com'), findsOneWidget);
      });

      testWidgets('shows link icon', (tester) async {
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
              body: ContentViewBuilders.buildUrlChip(
                text: 'URL',
                onTap: () {},
              ),
            ),
          ),
        );

        expect(find.byIcon(Icons.link), findsOneWidget);
      });

      testWidgets('has no trailing chevron', (tester) async {
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
              body: ContentViewBuilders.buildUrlChip(
                text: 'URL',
                onTap: () {},
              ),
            ),
          ),
        );

        expect(find.byIcon(Icons.chevron_right), findsNothing);
      });

      testWidgets('calls onTap when tapped', (tester) async {
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
              body: ContentViewBuilders.buildUrlChip(
                text: 'URL',
                onTap: () => tapped = true,
              ),
            ),
          ),
        );

        await tester.tap(find.text('URL'));
        await tester.pump();

        expect(tapped, isTrue);
      });

      testWidgets('has rounded chip decoration', (tester) async {
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
              body: ContentViewBuilders.buildUrlChip(
                text: 'URL',
                onTap: () {},
              ),
            ),
          ),
        );

        final container = tester.widget<Container>(find.byType(Container));
        final decoration = container.decoration! as BoxDecoration;
        expect(decoration.borderRadius, equals(BorderRadius.circular(10)));
      });
    });
  });
}
