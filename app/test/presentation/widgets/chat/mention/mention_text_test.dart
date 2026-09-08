import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/chat/mention/mention_text.dart';
import 'package:ripls/presentation/widgets/chat/mention/mention_types.dart';

void main() {
  group('MentionText', () {
    testWidgets('renders plain text without mentions', (tester) async {
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
            body: MentionText(text: 'Hello world!'),
          ),
        ),
      );

      expect(find.text('Hello world!'), findsOneWidget);
    });

    testWidgets('renders user mention as chip', (tester) async {
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
            body: MentionText(text: 'Hello @[user:123:John Doe]!'),
          ),
        ),
      );

      // Should find the display name with @ prefix in a chip
      expect(find.text('@John Doe'), findsOneWidget);
      // Should find the surrounding text
      expect(find.textContaining('Hello'), findsOneWidget);
    });

    testWidgets('renders multiple mentions', (tester) async {
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
            body: MentionText(
              text: 'Hey @[user:1:Alice] and @[user:2:Bob]!',
            ),
          ),
        ),
      );

      expect(find.text('@Alice'), findsOneWidget);
      expect(find.text('@Bob'), findsOneWidget);
    });

    testWidgets('renders different mention types', (tester) async {
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
            body: MentionText(
              text: 'Check @[loan:1:Tent] and @[request:2:Ladder]',
            ),
          ),
        ),
      );

      expect(find.text('@Tent'), findsOneWidget);
      expect(find.text('@Ladder'), findsOneWidget);
    });

    testWidgets('calls onMentionTap with correct type and id', (tester) async {
      MentionType? tappedType;
      String? tappedId;

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
            body: MentionText(
              text: 'Hello @[user:user123:John Doe]!',
              onMentionTap: (type, id) {
                tappedType = type;
                tappedId = id;
              },
            ),
          ),
        ),
      );

      // Find and tap the mention chip
      await tester.tap(find.text('@John Doe'));
      await tester.pumpAndSettle();

      expect(tappedType, MentionType.user);
      expect(tappedId, 'user123');
    });

    testWidgets('applies custom text style', (tester) async {
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
            body: MentionText(
              text: 'Hello @[user:123:John]!',
              style: TextStyle(fontSize: 20, color: Colors.red),
            ),
          ),
        ),
      );

      // Find mentions and text rendered together
      expect(find.text('@John'), findsOneWidget);
      expect(find.textContaining('Hello'), findsOneWidget);
    });

    testWidgets('respects onDarkBackground for chip styling', (tester) async {
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
            body: MentionText(
              text: '@[user:123:John]',
              onDarkBackground: true,
            ),
          ),
        ),
      );

      // Chip should be rendered (test passes if no errors)
      expect(find.text('@John'), findsOneWidget);
    });

    testWidgets('handles text with no mentions efficiently', (tester) async {
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
            body: MentionText(text: 'Simple text without any mentions'),
          ),
        ),
      );

      // Should use simple Text widget, not RichText
      expect(find.byType(Text), findsOneWidget);
      expect(find.text('Simple text without any mentions'), findsOneWidget);
    });

    testWidgets('handles empty text', (tester) async {
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
            body: MentionText(text: ''),
          ),
        ),
      );

      // Should render empty text without error
      expect(find.byType(Text), findsOneWidget);
    });

    testWidgets('handles text with only mention', (tester) async {
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
            body: MentionText(text: '@[user:123:John Doe]'),
          ),
        ),
      );

      expect(find.text('@John Doe'), findsOneWidget);
    });

    testWidgets('renders loan mention', (tester) async {
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
            body: MentionText(text: 'Borrow @[loan:t1:Camping Tent]'),
          ),
        ),
      );

      expect(find.text('@Camping Tent'), findsOneWidget);
    });

    testWidgets('renders giveaway mention', (tester) async {
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
            body: MentionText(text: 'Free @[giveaway:t2:Old Bike]'),
          ),
        ),
      );

      expect(find.text('@Old Bike'), findsOneWidget);
    });

    testWidgets('renders request mention', (tester) async {
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
            body: MentionText(text: 'Help with @[request:r1:Need a ladder]'),
          ),
        ),
      );

      expect(find.text('@Need a ladder'), findsOneWidget);
    });

    testWidgets('renders experience mention', (tester) async {
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
            body: MentionText(text: 'Join @[experience:e1:Weekend Hike]'),
          ),
        ),
      );

      expect(find.text('@Weekend Hike'), findsOneWidget);
    });

    testWidgets('taps on different mention types', (tester) async {
      final tappedMentions = <(MentionType, String)>[];

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
            body: MentionText(
              text: '@[user:u1:User] @[loan:l1:Loan] @[request:r1:Request]',
              onMentionTap: (type, id) {
                tappedMentions.add((type, id));
              },
            ),
          ),
        ),
      );

      // Tap user mention
      await tester.tap(find.text('@User'));
      await tester.pumpAndSettle();

      // Tap loan mention
      await tester.tap(find.text('@Loan'));
      await tester.pumpAndSettle();

      // Tap request mention
      await tester.tap(find.text('@Request'));
      await tester.pumpAndSettle();

      expect(tappedMentions, hasLength(3));
      expect(tappedMentions[0], (MentionType.user, 'u1'));
      expect(tappedMentions[1], (MentionType.loan, 'l1'));
      expect(tappedMentions[2], (MentionType.request, 'r1'));
    });
  });
}
