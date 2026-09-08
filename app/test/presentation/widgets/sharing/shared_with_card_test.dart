import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/sharing/shared_with_card.dart';

Widget _host(Widget child) => ProviderScope(
      child: MaterialApp(
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate,
        ],
        supportedLocales: AppLocalizations.supportedLocales,
        home: Scaffold(body: child),
      ),
    );

void main() {
  group('SharedWithCard', () {
    // The ContentEdgesCard header is uppercased, so the visible count copy is
    // e.g. "SHARED WITH 12 PEOPLE".
    testWidgets('shows the people count and an owner Share button',
        (tester) async {
      var accessTaps = 0;
      var inviteTaps = 0;
      await tester.pumpWidget(_host(SharedWithCard(
        totalPeople: 12,
        invitedIndividuals: [User(id: 'a', name: 'Ada Lovelace')],
        onTap: () => accessTaps++,
        onInvite: () => inviteTaps++,
      )));
      await tester.pump();

      expect(find.textContaining('12 PEOPLE'), findsOneWidget);
      expect(find.text('Share'), findsOneWidget);

      await tester.tap(find.text('Share'));
      expect(inviteTaps, 1, reason: 'Share button calls onInvite');
      expect(accessTaps, 0, reason: 'tapping Share must not open access');
    });

    testWidgets('hides the Share button when onInvite is null',
        (tester) async {
      var accessTaps = 0;
      await tester.pumpWidget(_host(SharedWithCard(
        totalPeople: 3,
        invitedIndividuals: const [],
        onTap: () => accessTaps++,
      )));
      await tester.pump();

      expect(find.textContaining('3 PEOPLE'), findsOneWidget);
      expect(find.text('Share'), findsNothing);

      // The whole card opens the access sheet.
      await tester.tap(find.textContaining('3 PEOPLE'));
      expect(accessTaps, 1);
    });

    testWidgets('uses singular copy for a single person', (tester) async {
      await tester.pumpWidget(_host(SharedWithCard(
        totalPeople: 1,
        invitedIndividuals: const [],
        onTap: () {},
      )));
      await tester.pump();

      expect(find.textContaining('1 PERSON'), findsOneWidget);
    });

    testWidgets(
        'facepile shows the counted people: invitee faces + a "+N" overflow '
        'disk for the rest (#2724)', (tester) async {
      await tester.pumpWidget(_host(SharedWithCard(
        totalPeople: 4,
        invitedIndividuals: [
          User(id: 'a', name: 'Ada Lovelace'),
          User(id: 'b', name: 'Bea Ortiz'),
        ],
        onTap: () {},
      )));
      await tester.pump();

      expect(find.textContaining('4 PEOPLE'), findsOneWidget);
      // Two invitee faces render as initials avatars…
      expect(find.text('AL'), findsOneWidget);
      expect(find.text('BO'), findsOneWidget);
      // …and the 2 counted-but-unlisted community members fold into "+2".
      expect(find.text('+2'), findsOneWidget);
    });

    testWidgets('facepile caps at five faces then overflows', (tester) async {
      await tester.pumpWidget(_host(SharedWithCard(
        totalPeople: 9,
        invitedIndividuals: [
          for (var i = 0; i < 7; i++) User(id: 'u$i', name: 'User $i'),
        ],
        onTap: () {},
      )));
      await tester.pump();

      // 5 faces + "+4" (9 counted − 5 shown).
      expect(find.text('+4'), findsOneWidget);
    });

    testWidgets('no facepile row (and no stray overflow) without invitees',
        (tester) async {
      await tester.pumpWidget(_host(SharedWithCard(
        totalPeople: 6,
        invitedIndividuals: const [],
        onTap: () {},
      )));
      await tester.pump();

      expect(find.textContaining('6 PEOPLE'), findsOneWidget);
      expect(find.textContaining('+'), findsNothing);
    });

    test('shareeCount excludes the owner from the audience count (#2724)', () {
      // The server's total_distinct_member_count counts the owner as a member
      // of the shared communities; the card counts who it's shared WITH.
      expect(SharedWithCard.shareeCount(4), 3);
      expect(SharedWithCard.shareeCount(1), 0); // owner-only origin community
      expect(SharedWithCard.shareeCount(0), 0); // not shared yet
    });
  });
}
