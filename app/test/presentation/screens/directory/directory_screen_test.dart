import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/responsive.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/communities/community_creation_modal.dart';
import 'package:ripls/presentation/screens/directory/directory_screen.dart';
import 'package:ripls/presentation/viewmodels/directory_entry.dart';
import 'package:ripls/presentation/viewmodels/directory_view_model.dart';
import 'package:ripls/presentation/viewmodels/tab_search_scope_provider.dart';
import 'package:ripls/presentation/widgets/search/search_scope_pill.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers.dart' show authStateProvider;
import 'package:shared_preferences/shared_preferences.dart';

import '../../../helpers/l10n_helpers.dart';

/// The viewer, for the owner-gated "Name" chip.
const _viewerId = 'viewer-1';

class _FakeAuthNotifier extends AuthStateNotifier {
  @override
  AuthStateData build() => AuthStateData(
        isLoading: false,
        user: User(id: _viewerId, name: 'Viewer'),
      );
}

const _entries = [
  // Boulder Crew moved most recently; Betty earlier — so the recent order
  // (Crew, Betty, group) differs from A–Z (Betty, Crew, group) and the two
  // sort tests genuinely distinguish the modes.
  DirectoryEntry(
    id: 'c1',
    kind: DirectoryKind.community,
    displayName: 'Boulder Crew',
    memberCount: 3,
    lastActivityUnixSec: 9000,
  ),
  DirectoryEntry(
    id: 'p1',
    kind: DirectoryKind.person,
    displayName: 'Betty',
    sharedCommunityNames: ['Boulder Crew', 'Hiking Crew'],
    lastActivityUnixSec: 1000,
  ),
  DirectoryEntry(
    id: 'g1',
    kind: DirectoryKind.group,
    displayName: 'Devon & Priya',
    subtitle: 'Devon, Priya',
    isNameless: true,
    // Owned by the viewer, so the "Name" chip opens the promote modal.
    ownerUserId: _viewerId,
  ),
];

Widget _wrap() {
  return ProviderScope(
    overrides: [
      directoryEntriesProvider.overrideWithValue(_entries),
      // Hermetic people source: the real provider fetches the inbox view,
      // and the test HttpClient's 400 trips the RPC unauthenticated path,
      // which logs the fake auth user out mid-test.
      directoryPeopleProvider
          .overrideWith((ref) async => const <DirectoryEntry>[]),
      authStateProvider.overrideWith(_FakeAuthNotifier.new),
    ],
    child: localizedApp(const DirectoryScreen()),
  );
}

void main() {
  setUp(() {
    TestWidgetsFlutterBinding.ensureInitialized();
    SharedPreferences.setMockInitialValues({});
  });

  group('DirectoryScreen (liquid-glass Rev 16)', () {
    testWidgets('renders everyone in one flat list — no section headers',
        (tester) async {
      await tester.pumpWidget(_wrap());
      await tester.pumpAndSettle();

      expect(find.text('Boulder Crew'), findsOneWidget);
      expect(find.text('Betty'), findsOneWidget);
      expect(find.text('Devon & Priya'), findsOneWidget);
      // Rev 16: one row type, no photo-card/person sections.
      expect(find.text('YOUR COMMUNITIES'), findsNothing);
      expect(find.text('PEOPLE'), findsNothing);
    });

    testWidgets('title is the Everyone control; subtitle carries the counts',
        (tester) async {
      await tester.pumpWidget(_wrap());
      await tester.pumpAndSettle();

      expect(find.text('Everyone ▾'), findsOneWidget);
      expect(
        find.text('2 groups · 1 person, by recent activity'),
        findsOneWidget,
      );
    });

    testWidgets('list sorts by most recent action regardless of kind',
        (tester) async {
      await tester.pumpWidget(_wrap());
      await tester.pumpAndSettle();

      // Boulder Crew (9000) floats above Betty (1000); the nameless group
      // (no activity time) sorts last.
      final crewY = tester.getTopLeft(find.text('Boulder Crew')).dy;
      final bettyY = tester.getTopLeft(find.text('Betty')).dy;
      final groupY = tester.getTopLeft(find.text('Devon & Priya')).dy;
      expect(crewY, lessThan(bettyY));
      expect(bettyY, lessThan(groupY));
    });

    testWidgets('tapping the title opens the sort & filter sheet',
        (tester) async {
      await tester.pumpWidget(_wrap());
      await tester.pumpAndSettle();

      await tester.tap(find.text('Everyone ▾'));
      await tester.pumpAndSettle();

      expect(find.text('Sort & filter'), findsOneWidget);
      expect(find.text('Recent activity'), findsOneWidget);
      expect(find.text('Name A–Z'), findsOneWidget);
      expect(find.text('People only'), findsOneWidget);
      expect(find.text('Communities only'), findsOneWidget);
    });

    testWidgets('People only filter scopes the one list', (tester) async {
      await tester.pumpWidget(_wrap());
      await tester.pumpAndSettle();

      await tester.tap(find.text('Everyone ▾'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('People only'));
      await tester.pumpAndSettle();

      expect(find.text('Betty'), findsOneWidget);
      expect(find.text('Boulder Crew'), findsNothing);
      expect(find.text('Devon & Priya'), findsNothing);
    });

    testWidgets('title and subtitle state what the filter is showing',
        (tester) async {
      await tester.pumpWidget(_wrap());
      await tester.pumpAndSettle();

      await tester.tap(find.text('Everyone ▾'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('People only'));
      await tester.pumpAndSettle();

      expect(find.text('People ▾'), findsOneWidget);
      expect(find.text('1 person, by recent activity'), findsOneWidget);

      await tester.tap(find.text('People ▾'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Communities only'));
      await tester.pumpAndSettle();

      expect(find.text('Communities ▾'), findsOneWidget);
      expect(find.text('2 groups, by recent activity'), findsOneWidget);
    });

    testWidgets('subtitle ordering clause tracks the active sort',
        (tester) async {
      await tester.pumpWidget(_wrap());
      await tester.pumpAndSettle();

      await tester.tap(find.text('Everyone ▾'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Name A–Z'));
      await tester.pumpAndSettle();

      expect(
        find.text('2 groups · 1 person, by name'),
        findsOneWidget,
      );
    });

    testWidgets('Name A–Z sort reorders the one list', (tester) async {
      await tester.pumpWidget(_wrap());
      await tester.pumpAndSettle();

      await tester.tap(find.text('Everyone ▾'));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Name A–Z'));
      await tester.pumpAndSettle();

      final bettyY = tester.getTopLeft(find.text('Betty')).dy;
      final crewY = tester.getTopLeft(find.text('Boulder Crew')).dy;
      final groupY = tester.getTopLeft(find.text('Devon & Priya')).dy;
      expect(bettyY, lessThan(crewY));
      expect(crewY, lessThan(groupY));
    });

    testWidgets('row middle line: person shows shared communities',
        (tester) async {
      await tester.pumpWidget(_wrap());
      await tester.pumpAndSettle();

      expect(find.text('Boulder Crew, Hiking Crew'), findsOneWidget);
      expect(find.text('3 members'), findsOneWidget);
    });

    testWidgets('nameless group row keeps the name prompt (Decision 5)',
        (tester) async {
      await tester.pumpWidget(_wrap());
      await tester.pumpAndSettle();

      expect(find.text('Name'), findsOneWidget);
    });

    testWidgets(
        'tapping the "Name" chip as the group owner opens the community '
        'modal in promote mode (#2568)', (tester) async {
      await tester.pumpWidget(_wrap());
      await tester.pumpAndSettle();

      await tester.tap(find.text('Name'));
      // Bounded pumps instead of pumpAndSettle: the modal hosts ongoing
      // animation (backdrop blur route transition) that never settles.
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 600));

      // The promote-mode create/AI modal — not the group's profile.
      expect(find.byType(CommunityCreationModal), findsOneWidget);
      expect(
        tester
            .widget<CommunityCreationModal>(
                find.byType(CommunityCreationModal))
            .promoteCommunityId,
        'g1',
      );
    });

    testWidgets(
        'a universal-search scope filters the list; the pill ✕ restores it',
        (tester) async {
      await tester.pumpWidget(_wrap());
      await tester.pumpAndSettle();

      final container = ProviderScope.containerOf(
        tester.element(find.byType(DirectoryScreen)),
      );
      container.read(peopleSearchScopeProvider.notifier).set('betty');
      await tester.pumpAndSettle();

      // Only the match remains, and the scope pill shows the query.
      expect(find.byType(SearchScopePill), findsOneWidget);
      expect(find.text('betty'), findsOneWidget);
      expect(find.text('Betty'), findsOneWidget);
      expect(find.text('Boulder Crew'), findsNothing);
      expect(find.text('Devon & Priya'), findsNothing);

      // Cancelling the search restores the full tab.
      await tester.tap(find.descendant(
        of: find.byType(SearchScopePill),
        matching: find.byIcon(Icons.close),
      ));
      await tester.pumpAndSettle();
      expect(find.byType(SearchScopePill), findsNothing);
      expect(find.text('Boulder Crew'), findsOneWidget);
      expect(find.text('Devon & Priya'), findsOneWidget);
    });
  });

  group('desktop measure (#2912)', () {
    testWidgets('rows hold the centered reading column at 1440x810',
        (tester) async {
      tester.view.physicalSize = const Size(1440, 810);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.reset);

      await tester.pumpWidget(_wrap());
      await tester.pumpAndSettle();

      const bandLeft = (1440 - Responsive.contentMaxWidth) / 2;
      const bandRight = bandLeft + Responsive.contentMaxWidth;
      for (final name in ['Boulder Crew', 'Betty']) {
        final rect = tester.getRect(find.text(name));
        expect(rect.left, greaterThanOrEqualTo(bandLeft),
            reason: '$name row starts left of the measure band');
        expect(rect.right, lessThanOrEqualTo(bandRight),
            reason: '$name row ends right of the measure band');
      }
    });

    testWidgets('phone geometry is unchanged at 390x844', (tester) async {
      tester.view.physicalSize = const Size(390, 844);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.reset);

      await tester.pumpWidget(_wrap());
      await tester.pumpAndSettle();

      // Rows keep their left-anchored phone layout: the row body starts at
      // the 16px list inset plus the row's own leading padding — nowhere
      // near a centered column's left edge.
      final rect = tester.getRect(find.text('Boulder Crew'));
      expect(rect.left, lessThan(120));
    });
  });
}
