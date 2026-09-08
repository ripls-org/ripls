import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/plus_button_modal.dart';

Widget _buildModal({
  VoidCallback? onShareGear,
  VoidCallback? onInviteExperience,
  VoidCallback? onRequestSomething,
  VoidCallback? onInviteUser,
  VoidCallback? onClose,
}) {
  return MaterialApp(
    localizationsDelegates: const [
      AppLocalizations.delegate,
      GlobalMaterialLocalizations.delegate,
      GlobalWidgetsLocalizations.delegate,
      GlobalCupertinoLocalizations.delegate,
    ],
    supportedLocales: AppLocalizations.supportedLocales,
    home: Scaffold(
      body: PlusButtonModal(
        onShareGear: onShareGear ?? () {},
        onInviteExperience: onInviteExperience ?? () {},
        onRequestSomething: onRequestSomething ?? () {},
        onInviteUser: onInviteUser ?? () {},
        onClose: onClose ?? () {},
      ),
    ),
  );
}

void main() {
  group('PlusButtonModal', () {
    testWidgets('default tab is Ask', (WidgetTester tester) async {
      await tester.pumpWidget(_buildModal());
      await tester.pump();

      // Ask tab is default; its items should be rendered
      expect(find.text('Ask'), findsOneWidget);
      expect(find.text('Help'), findsOneWidget);
    });

    testWidgets('renders title', (WidgetTester tester) async {
      await tester.pumpWidget(_buildModal());
      await tester.pump();

      expect(find.text("I'd like to..."), findsOneWidget);
    });

    testWidgets('renders all three tab labels', (WidgetTester tester) async {
      await tester.pumpWidget(_buildModal());
      await tester.pump();

      expect(find.text('Share'), findsOneWidget);
      expect(find.text('Do'), findsOneWidget);
      expect(find.text('Ask'), findsOneWidget);
      expect(find.text('Grow'), findsNothing);
    });

    testWidgets('Share tab shows 8 items', (WidgetTester tester) async {
      await tester.pumpWidget(_buildModal());
      await tester.pump();

      await tester.tap(find.text('Share'));
      await tester.pumpAndSettle();

      expect(find.text('Tools'), findsOneWidget);
      expect(find.text('Clothes'), findsOneWidget);
      expect(find.text('Books'), findsOneWidget);
      expect(find.text('Furniture'), findsOneWidget);
      expect(find.text('Games'), findsOneWidget);
      expect(find.text('Kids stuff'), findsOneWidget);
      expect(find.text('Food'), findsOneWidget);
      expect(find.text('Other'), findsOneWidget);
    });

    testWidgets('Do tab shows 8 items', (WidgetTester tester) async {
      await tester.pumpWidget(_buildModal());
      await tester.pump();

      await tester.tap(find.text('Do'));
      await tester.pumpAndSettle();

      expect(find.text('Eat'), findsOneWidget);
      expect(find.text('Hike'), findsOneWidget);
      expect(find.text('Carpool'), findsOneWidget);
      expect(find.text('Adventure'), findsOneWidget);
      expect(find.text('Dance'), findsOneWidget);
      expect(find.text('Study'), findsOneWidget);
      expect(find.text('Garden'), findsOneWidget);
      expect(find.text('Other'), findsOneWidget);
      expect(find.text('Exercise'), findsNothing);
      expect(find.text('Create'), findsNothing);
    });

    testWidgets('Ask tab shows 8 items', (WidgetTester tester) async {
      await tester.pumpWidget(_buildModal());
      await tester.pump();

      expect(find.text('Help'), findsOneWidget);
      expect(find.text('Babysit'), findsOneWidget);
      expect(find.text('Donation'), findsOneWidget);
      expect(find.text('Pet-sit'), findsOneWidget);
      expect(find.text('Ride'), findsOneWidget);
      expect(find.text('Tutor'), findsOneWidget);
      expect(find.text('Repair'), findsOneWidget);
      expect(find.text('Other'), findsOneWidget);
      expect(find.text('Errand'), findsNothing);
    });

    testWidgets('tapping a Share item calls onShareGear and onClose', (
      WidgetTester tester,
    ) async {
      bool shareGearCalled = false;
      bool closeCalled = false;

      await tester.pumpWidget(
        _buildModal(
          onShareGear: () => shareGearCalled = true,
          onClose: () => closeCalled = true,
        ),
      );
      await tester.pump();

      await tester.tap(find.text('Share'));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Tools'));
      await tester.pump();

      expect(shareGearCalled, isTrue);
      expect(closeCalled, isTrue);
    });

    testWidgets('tapping a Do item calls onInviteExperience and onClose', (
      WidgetTester tester,
    ) async {
      bool inviteExperienceCalled = false;
      bool closeCalled = false;

      await tester.pumpWidget(
        _buildModal(
          onInviteExperience: () => inviteExperienceCalled = true,
          onClose: () => closeCalled = true,
        ),
      );
      await tester.pump();

      await tester.tap(find.text('Do'));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Eat'));
      await tester.pump();

      expect(inviteExperienceCalled, isTrue);
      expect(closeCalled, isTrue);
    });

    testWidgets('tapping an Ask item calls onRequestSomething and onClose', (
      WidgetTester tester,
    ) async {
      bool requestCalled = false;
      bool closeCalled = false;

      await tester.pumpWidget(
        _buildModal(
          onRequestSomething: () => requestCalled = true,
          onClose: () => closeCalled = true,
        ),
      );
      await tester.pump();

      await tester.tap(find.text('Ask'));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Help'));
      await tester.pump();

      expect(requestCalled, isTrue);
      expect(closeCalled, isTrue);
    });

    testWidgets('tapping invite link calls onInviteUser and onClose', (
      WidgetTester tester,
    ) async {
      bool inviteUserCalled = false;
      bool closeCalled = false;

      await tester.pumpWidget(
        _buildModal(
          onInviteUser: () => inviteUserCalled = true,
          onClose: () => closeCalled = true,
        ),
      );
      await tester.pump();

      await tester.ensureVisible(find.text('Invite Someone'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Invite Someone'));
      await tester.pump();

      expect(inviteUserCalled, isTrue);
      expect(closeCalled, isTrue);
    });

    testWidgets('segmented control indicator updates on tab switch', (
      WidgetTester tester,
    ) async {
      await tester.pumpWidget(_buildModal());
      await tester.pump();

      // Ask tab items are visible initially
      expect(find.text('Help'), findsOneWidget);

      // Switch to Do tab
      await tester.tap(find.text('Do'));
      await tester.pumpAndSettle();

      // Do tab items are now visible, Ask items gone
      expect(find.text('Eat'), findsOneWidget);
      expect(find.text('Help'), findsNothing);
    });

    testWidgets('feedback button is present', (WidgetTester tester) async {
      await tester.pumpWidget(_buildModal());
      await tester.pump();

      expect(find.text('Feedback'), findsOneWidget);
    });
  });
}
