import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart'
    show ExperienceContributionResponse;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/experience/needs/view_contribution_sheet.dart';

ExperienceContributionResponse _c({
  required String id,
  required String contributorId,
  required String contributorName,
  String title = 'Chips',
  String description = '',
  String fromNeedId = '',
}) {
  final r = ExperienceContributionResponse(
    id: id,
    title: title,
    contributor: User(id: contributorId, name: contributorName),
  );
  if (description.isNotEmpty) r.description = description;
  if (fromNeedId.isNotEmpty) r.fromNeedId = fromNeedId;
  return r;
}

Widget _wrap(Widget child) {
  return ProviderScope(
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
}

void main() {
  group('ViewContributionSheet rendering', () {
    testWidgets('shows item title and every contributor name', (tester) async {
      await tester.pumpWidget(_wrap(ViewContributionSheet(
        title: 'Chips',
        contributions: [
          _c(id: 'c1', contributorId: 'u1', contributorName: 'Alice'),
          _c(id: 'c2', contributorId: 'u2', contributorName: 'Bob'),
        ],
        currentUserId: 'viewer',
        experienceOwnerId: 'owner',
      )));

      expect(find.text('Chips'), findsOneWidget);
      expect(find.text('Alice'), findsOneWidget);
      expect(find.text('Bob'), findsOneWidget);
    });

    testWidgets('renders contribution description as secondary line',
        (tester) async {
      await tester.pumpWidget(_wrap(ViewContributionSheet(
        title: 'Dessert',
        contributions: [
          _c(
            id: 'c1',
            contributorId: 'u1',
            contributorName: 'Alice',
            description: 'gluten-free',
          ),
        ],
        currentUserId: 'viewer',
        experienceOwnerId: 'owner',
      )));

      expect(find.text('gluten-free'), findsOneWidget);
    });

    testWidgets('owner-only contribution shows "Handling themselves" badge',
        (tester) async {
      await tester.pumpWidget(_wrap(ViewContributionSheet(
        title: 'Music',
        contributions: [
          _c(id: 'c1', contributorId: 'owner', contributorName: 'Owen'),
        ],
        currentUserId: 'viewer',
        experienceOwnerId: 'owner',
      )));

      expect(find.text('Handling themselves'), findsOneWidget);
    });

    testWidgets('claimed contribution (linked to a need) shows claimed status',
        (tester) async {
      await tester.pumpWidget(_wrap(ViewContributionSheet(
        title: 'Drinks',
        contributions: [
          _c(
            id: 'c1',
            contributorId: 'u1',
            contributorName: 'Alice',
            fromNeedId: 'need-1',
          ),
        ],
        currentUserId: 'viewer',
        experienceOwnerId: 'owner',
      )));

      expect(find.text('Claimed from the request'), findsOneWidget);
    });

    testWidgets('single freeform offer shows "Freely offered"',
        (tester) async {
      await tester.pumpWidget(_wrap(ViewContributionSheet(
        title: 'Plates',
        contributions: [
          _c(id: 'c1', contributorId: 'u1', contributorName: 'Alice'),
        ],
        currentUserId: 'viewer',
        experienceOwnerId: 'owner',
      )));

      expect(find.text('Freely offered'), findsOneWidget);
    });

    testWidgets('multiple contributors shows "N bringing this"',
        (tester) async {
      await tester.pumpWidget(_wrap(ViewContributionSheet(
        title: 'Chips',
        contributions: [
          _c(id: 'c1', contributorId: 'u1', contributorName: 'Alice'),
          _c(id: 'c2', contributorId: 'u2', contributorName: 'Bob'),
          _c(id: 'c3', contributorId: 'u3', contributorName: 'Carol'),
        ],
        currentUserId: 'viewer',
        experienceOwnerId: 'owner',
      )));

      expect(find.text('3 bringing this'), findsOneWidget);
    });
  });

  group('ViewContributionSheet — Add-me / Remove-mine branching', () {
    testWidgets("shows 'I'll bring it too' when viewer is not in list",
        (tester) async {
      await tester.pumpWidget(_wrap(ViewContributionSheet(
        title: 'Chips',
        contributions: [
          _c(id: 'c1', contributorId: 'u1', contributorName: 'Alice'),
        ],
        currentUserId: 'viewer',
        experienceOwnerId: 'owner',
        onAddMe: () {},
        onRemoveMine: () {},
      )));

      expect(find.text("I'll bring it too"), findsOneWidget);
      expect(find.text('Remove mine'), findsNothing);
    });

    testWidgets("shows 'Remove mine' when viewer IS in list", (tester) async {
      await tester.pumpWidget(_wrap(ViewContributionSheet(
        title: 'Chips',
        contributions: [
          _c(id: 'c1', contributorId: 'viewer', contributorName: 'Viewer'),
          _c(id: 'c2', contributorId: 'u2', contributorName: 'Bob'),
        ],
        currentUserId: 'viewer',
        experienceOwnerId: 'owner',
        onAddMe: () {},
        onRemoveMine: () {},
      )));

      expect(find.text('Remove mine'), findsOneWidget);
      expect(find.text("I'll bring it too"), findsNothing);
    });

    testWidgets("hides 'I'll bring it too' when onAddMe is null",
        (tester) async {
      await tester.pumpWidget(_wrap(ViewContributionSheet(
        title: 'Chips',
        contributions: [
          _c(id: 'c1', contributorId: 'u1', contributorName: 'Alice'),
        ],
        currentUserId: 'viewer',
        experienceOwnerId: 'owner',
      )));

      expect(find.text("I'll bring it too"), findsNothing);
    });

    testWidgets("hides 'Remove mine' when onRemoveMine is null",
        (tester) async {
      await tester.pumpWidget(_wrap(ViewContributionSheet(
        title: 'Chips',
        contributions: [
          _c(id: 'c1', contributorId: 'viewer', contributorName: 'Viewer'),
        ],
        currentUserId: 'viewer',
        experienceOwnerId: 'owner',
      )));

      expect(find.text('Remove mine'), findsNothing);
    });

    testWidgets('tapping "I\'ll bring it too" calls onAddMe and pops modal',
        (tester) async {
      var called = 0;
      await tester.pumpWidget(_wrap(_SheetLauncher(
        builder: (ctx) => ViewContributionSheet.show(
          ctx,
          title: 'Chips',
          contributions: [
            _c(id: 'c1', contributorId: 'u1', contributorName: 'Alice'),
          ],
          currentUserId: 'viewer',
          experienceOwnerId: 'owner',
          onAddMe: () => called++,
        ),
      )));
      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      await tester.tap(find.text("I'll bring it too"));
      await tester.pumpAndSettle();

      expect(called, 1);
      expect(find.text('Chips'), findsNothing);
    });

    testWidgets('tapping "Remove mine" calls onRemoveMine and pops modal',
        (tester) async {
      var called = 0;
      await tester.pumpWidget(_wrap(_SheetLauncher(
        builder: (ctx) => ViewContributionSheet.show(
          ctx,
          title: 'Chips',
          contributions: [
            _c(id: 'c1', contributorId: 'viewer', contributorName: 'Viewer'),
          ],
          currentUserId: 'viewer',
          experienceOwnerId: 'owner',
          onRemoveMine: () => called++,
        ),
      )));
      await tester.tap(find.text('open'));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Remove mine'));
      await tester.pumpAndSettle();

      expect(called, 1);
      expect(find.text('Chips'), findsNothing);
    });
  });
}

/// Helper widget that exposes a button to launch the sheet so we can
/// verify modal-close behaviour through Navigator.pop.
class _SheetLauncher extends StatelessWidget {
  const _SheetLauncher({required this.builder});

  final Future<void> Function(BuildContext) builder;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Builder(
        builder: (ctx) => TextButton(
          onPressed: () => builder(ctx),
          child: const Text('open'),
        ),
      ),
    );
  }
}
