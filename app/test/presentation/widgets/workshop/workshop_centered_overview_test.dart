import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/viewmodels/workshop_identity_rollup_view_model.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_centered_overview.dart';

void main() {
  // Pumps the overview with the identity rollup stubbed empty (no members hero)
  // and an empty communityId (skips the conversation-glimpse provider). [problems]
  // defaults to 1 so the impact sentence renders — the case where the invite
  // pill used to disappear.
  Future<void> pump(
    WidgetTester tester, {
    required String communityName,
    String originItemName = '',
    VoidCallback? onNameGroup,
    VoidCallback? onInvite,
    int problems = 1,
  }) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          workshopIdentityRollupProvider
              .overrideWith((ref) async => WorkshopIdentityRollup.empty),
        ],
        child: MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: Scaffold(
            body: WorkshopCenteredOverview(
              communityId: '',
              communityName: communityName,
              originItemName: originItemName,
              hours: 0,
              dollars: 0,
              co2Kg: 0,
              problems: problems,
              problemsPotential: problems,
              memberCount: 3,
              items: const [],
              specialties: const ['Hiking'],
              onTapMembers: (_) {},
              onTapTime: (_) {},
              onTapMoney: (_) {},
              onTapCo2: (_) {},
              onTapProblems: (_) {},
              onTapConversation: (_) {},
              onTapCalendar: (_) {},
              onTapCommonGround: (_, _) {},
              onInvite: onInvite ?? () {},
              onNameGroup: onNameGroup,
            ),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
  }

  testWidgets(
    'a named, active community still shows an Invite pill (no Name-this-group)',
    (tester) async {
      await pump(tester, communityName: 'Trail Crew');

      // The invite pill persists even though the group has impact (the bug).
      expect(find.text('Invite to Trail Crew'), findsOneWidget);
      // No promote pill for a named community.
      expect(find.text('Name this group'), findsNothing);
    },
  );

  testWidgets(
    'a nameless community the viewer owns shows both Name-this-group and Invite',
    (tester) async {
      await pump(tester, communityName: '', onNameGroup: () {});

      expect(find.text('Name this group'), findsOneWidget);
      // Nameless -> the generic invite label.
      expect(find.text('Invite people'), findsOneWidget);
    },
  );

  testWidgets('the action pills fire their callbacks', (tester) async {
    var named = 0;
    var invited = 0;
    await pump(
      tester,
      communityName: '',
      onNameGroup: () => named++,
      onInvite: () => invited++,
    );

    await tester.tap(find.text('Name this group'));
    await tester.pumpAndSettle();
    expect(named, 1);

    await tester.tap(find.text('Invite people'));
    await tester.pumpAndSettle();
    expect(invited, 1);
  });

  testWidgets(
    'a nameless community the viewer does NOT own shows only Invite',
    (tester) async {
      await pump(tester, communityName: ''); // onNameGroup null

      expect(find.text('Name this group'), findsNothing);
      expect(find.text('Invite people'), findsOneWidget);
    },
  );

  testWidgets(
    'a nameless community eyebrow identifies the spawning item',
    (tester) async {
      await pump(tester, communityName: '', originItemName: 'Dinner at Este');

      // The eyebrow is uppercased: "FROM DINNER AT ESTE · 3 MEMBERS".
      expect(find.textContaining('FROM DINNER AT ESTE'), findsOneWidget);
    },
  );
}
