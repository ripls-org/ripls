import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/governance/delete_community_screen.dart';
import 'package:ripls/presentation/screens/governance/sole_member_leave_explainer_screen.dart';

void main() {
  group('SoleMemberLeaveExplainerScreen', () {
    Widget pumpHarness({required Widget child}) {
      return ProviderScope(
        child: MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          home: child,
        ),
      );
    }

    final community = CommunityItem(id: 'comm-1', name: 'Tide Pool');

    testWidgets('renders title, body, Continue, and Cancel', (tester) async {
      await tester.pumpWidget(pumpHarness(
        child: SoleMemberLeaveExplainerScreen(community: community),
      ));
      await tester.pumpAndSettle();

      expect(find.text("You're the only member"), findsAtLeastNWidgets(1));
      expect(
        find.textContaining('Leaving will delete Tide Pool'),
        findsOneWidget,
      );
      expect(find.text('Continue to Delete'), findsOneWidget);
      expect(find.text('Cancel'), findsOneWidget);
    });

    testWidgets('Cancel pops without advancing', (tester) async {
      await tester.pumpWidget(pumpHarness(
        child: Builder(
          builder: (context) => Scaffold(
            body: Center(
              child: ElevatedButton(
                onPressed: () => Navigator.of(context).push(
                  MaterialPageRoute(
                    builder: (_) =>
                        SoleMemberLeaveExplainerScreen(community: community),
                  ),
                ),
                child: const Text('Open'),
              ),
            ),
          ),
        ),
      ));
      await tester.tap(find.text('Open'));
      await tester.pumpAndSettle();

      expect(find.byType(SoleMemberLeaveExplainerScreen), findsOneWidget);

      await tester.tap(find.text('Cancel'));
      await tester.pumpAndSettle();

      expect(find.byType(SoleMemberLeaveExplainerScreen), findsNothing);
      expect(find.text('Open'), findsOneWidget);
    });

    testWidgets(
      'Continue replaces explainer with DeleteCommunityScreen',
      (tester) async {
        await tester.pumpWidget(pumpHarness(
          child: SoleMemberLeaveExplainerScreen(community: community),
        ));
        await tester.pumpAndSettle();

        await tester.tap(find.text('Continue to Delete'));
        await tester.pumpAndSettle();

        // pushReplacement: explainer is gone, delete screen is on top.
        expect(find.byType(SoleMemberLeaveExplainerScreen), findsNothing);
        expect(find.byType(DeleteCommunityScreen), findsOneWidget);
      },
    );
  });
}
