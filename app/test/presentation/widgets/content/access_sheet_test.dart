import 'dart:async' show Completer;

import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem, CommunityMember;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/community_avatar.dart';
import 'package:ripls/presentation/widgets/content/access_sheet.dart';
import 'package:ripls/presentation/widgets/group_avatar.dart';
import 'package:ripls/services/providers.dart'
    show communityRepositoryProvider, communitiesProvider;
import 'package:ripls/services/providers/community_providers.dart'
    show CommunitiesNotifier, CommunitiesState;

import 'access_sheet_test.mocks.dart';

@GenerateMocks([CommunityRepository])
void main() {
  late MockCommunityRepository mockCommunityRepo;

  setUp(() {
    mockCommunityRepo = MockCommunityRepository();
  });

  group('AccessSheet member count', () {
    testWidgets(
        'shows fetched member count when members load successfully',
        (tester) async {
      // The group starts with memberCount=0 (optimistic state after
      // adding a community).
      final group = makeGroup(communityId: 'com-1', memberCount: 0);

      // The repository returns 2 real members.
      when(mockCommunityRepo.getMembers('com-1')).thenAnswer(
        (_) async => [makeMember('u1'), makeMember('u2')],
      );

      await tester.pumpWidget(buildSheet(mockCommunityRepo, groups: [group]));
      // Let the async getMembers call complete and the widget rebuild.
      await tester.pumpAndSettle();

      // The subtitle should say "2 members", not "0 members".
      expect(find.textContaining('2 member'), findsOneWidget);
      expect(find.textContaining('0 member'), findsNothing);
    });

    testWidgets(
        'falls back to group.memberCount while fetch is pending',
        (tester) async {
      // The group has a non-zero proto-supplied memberCount.
      final group = makeGroup(communityId: 'com-2', memberCount: 3);

      // The future never resolves during this test so the sheet stays in
      // the loading state. Using a Completer avoids leaving a pending timer.
      when(mockCommunityRepo.getMembers('com-2')).thenAnswer(
        (_) => Completer<List<CommunityMember>>().future,
      );

      await tester.pumpWidget(buildSheet(mockCommunityRepo, groups: [group]));
      // Only one frame so the async fetch does NOT complete.
      await tester.pump();

      // The subtitle should fall back to the proto-supplied memberCount.
      expect(find.textContaining('3 member'), findsOneWidget);
      expect(find.textContaining('0 member'), findsNothing);
    });

    testWidgets(
        'shows singular "1 member" for a community with exactly one member',
        (tester) async {
      final group = makeGroup(communityId: 'com-3', memberCount: 0);

      when(mockCommunityRepo.getMembers('com-3')).thenAnswer(
        (_) async => [makeMember('solo-user')],
      );

      await tester.pumpWidget(buildSheet(mockCommunityRepo, groups: [group]));
      await tester.pumpAndSettle();

      expect(find.textContaining('1 member'), findsOneWidget);
      // Verify plural form is absent.
      expect(find.textContaining('1 members'), findsNothing);
    });

    // Phase 4 of #1705: non-member communities are collapsed into a single
    // aggregate row that does NOT leak their names or per-community share
    // metadata, and never triggers getMembers() (which would 403 anyway).
    testWidgets(
        'communities the caller cannot see are summarized by '
        'otherCommunityCount, not leaked as rows',
        (tester) async {
      final memberGroup = makeGroup(
        communityId: 'mine',
        communityName: 'My Bike Club',
        memberCount: 3,
      );
      when(mockCommunityRepo.getMembers('mine')).thenAnswer(
        (_) async => [makeMember('u1'), makeMember('u2'), makeMember('u3')],
      );

      // The server keeps non-visible communities out of `groups` (which is
      // already scoped to communities the caller is a member of) and reports
      // only their count via otherCommunityCount.
      await tester.pumpWidget(buildSheet(
        mockCommunityRepo,
        groups: [memberGroup],
        otherCommunityCount: 2,
      ));
      await tester.pumpAndSettle();

      // The visible community renders its real name + members.
      expect(find.text('My Bike Club'), findsOneWidget);
      verify(mockCommunityRepo.getMembers('mine')).called(1);
      // The rest collapse into a single count-only aggregate row.
      expect(find.text('And 2 other communities'), findsOneWidget);
    });

    testWidgets(
        'singular: "And 1 other community" when otherCommunityCount is 1',
        (tester) async {
      final memberGroup = makeGroup(communityId: 'mine', memberCount: 1);
      when(mockCommunityRepo.getMembers('mine'))
          .thenAnswer((_) async => [makeMember('u1')]);

      await tester.pumpWidget(buildSheet(
        mockCommunityRepo,
        groups: [memberGroup],
        otherCommunityCount: 1,
      ));
      await tester.pumpAndSettle();

      expect(find.text('And 1 other community'), findsOneWidget);
    });

    testWidgets(
        'aggregate row is non-interactive: no chevron, tap is a no-op',
        (tester) async {
      // No visible communities, just a count the caller can't see into.
      await tester.pumpWidget(buildSheet(
        mockCommunityRepo,
        groups: const [],
        otherCommunityCount: 1,
      ));
      await tester.pumpAndSettle();

      // No chevron — nothing expandable.
      expect(find.byIcon(Icons.keyboard_arrow_down), findsNothing);
      // Tapping the aggregate row fetches no members.
      await tester.tap(find.text('And 1 other community'));
      await tester.pumpAndSettle();
      verifyNever(mockCommunityRepo.getMembers(any));
    });

    testWidgets(
        'aggregate-only: count row renders with no visible communities and no '
        'indefinite spinner',
        (tester) async {
      await tester.pumpWidget(buildSheet(
        mockCommunityRepo,
        groups: const [],
        otherCommunityCount: 2,
      ));
      await tester.pumpAndSettle();

      // Single aggregate row, no individual rows.
      expect(find.text('And 2 other communities'), findsOneWidget);
      expect(find.text('Bike Club'), findsNothing);
      // No infinite spinner from the summary banner.
      expect(find.byType(CircularProgressIndicator), findsNothing);
    });

    testWidgets(
        'visible community rows are listed before the aggregate count row',
        (tester) async {
      final memberGroup = makeGroup(
        communityId: 'mine',
        communityName: 'My Bike Club',
        memberCount: 3,
      );
      when(mockCommunityRepo.getMembers('mine'))
          .thenAnswer((_) async => [makeMember('u1')]);

      await tester.pumpWidget(buildSheet(
        mockCommunityRepo,
        groups: [memberGroup],
        otherCommunityCount: 1,
      ));
      await tester.pumpAndSettle();

      final memberRect = tester.getTopLeft(find.text('My Bike Club'));
      final aggregateRect =
          tester.getTopLeft(find.text('And 1 other community'));
      expect(memberRect.dy, lessThan(aggregateRect.dy));
    });

    testWidgets(
        'subtitle includes sharedByName and sharedTimeAgo',
        (tester) async {
      final group = AccessGroup(
        communityId: 'com-4',
        communityName: 'Trail Runners',
        sharedByName: 'Bob',
        sharedByInitials: 'B',
        sharedTimeAgo: '1 hour ago',
        sharedAtUnixSec: 1700000000,
        memberCount: 4,
      );

      when(mockCommunityRepo.getMembers('com-4')).thenAnswer(
        (_) async => [
          makeMember('u1'),
          makeMember('u2'),
          makeMember('u3'),
          makeMember('u4'),
        ],
      );

      await tester.pumpWidget(buildSheet(mockCommunityRepo, groups: [group]));
      await tester.pumpAndSettle();

      // The localized subtitle must contain the sharer name, time, and count.
      expect(find.textContaining('Bob'), findsWidgets);
      expect(find.textContaining('1 hour ago'), findsOneWidget);
      expect(find.textContaining('4 member'), findsOneWidget);
    });
  });

  // DISPLAY-1 (#2492): a nameless ad-hoc / per-item community has no name to
  // show, so the sheet renders it group-text style ("You and …") and leads with
  // its members' faces instead of a blank community avatar.
  group('AccessSheet nameless (group-text) community', () {
    testWidgets(
        'renders the group-text label and member faces, not a blank name',
        (tester) async {
      final group = makeGroup(
        communityId: 'adhoc',
        communityName: '', // nameless ad-hoc community
        memberCount: 4,
      );
      when(mockCommunityRepo.getMembers('adhoc')).thenAnswer(
        (_) async => [
          makeMember('viewer'),
          makeMember('ada'),
          makeMember('sam'),
          makeMember('cy'),
        ],
      );

      await tester.pumpWidget(buildSheet(
        mockCommunityRepo,
        groups: [group],
        // The loaded CommunityItem is also nameless and carries the server's
        // viewer-excluded member preview names.
        communities: [
          CommunityItem(
            id: 'adhoc',
            name: '',
            memberCount: 4,
            memberPreviewFirstNames: ['Ada', 'Sam'],
          ),
        ],
      ));
      await tester.pumpAndSettle();

      // Group-text label from the viewer's perspective: memberCount 4 = viewer
      // + 3 others, 2 shown -> ", and 1 other".
      expect(find.text('You, Ada, Sam, and 1 other'), findsOneWidget);
      // The leading slot is a group-avatar cluster of member faces, NOT a
      // community avatar with a blank/placeholder initial.
      expect(find.byType(GroupAvatar), findsOneWidget);
      expect(find.byType(CommunityAvatar), findsNothing);
    });

    testWidgets(
        'a named community still renders its name and a community avatar',
        (tester) async {
      final group = makeGroup(
        communityId: 'named',
        communityName: 'Trail Crew',
        memberCount: 3,
      );
      when(mockCommunityRepo.getMembers('named')).thenAnswer(
        (_) async => [makeMember('u1'), makeMember('u2'), makeMember('u3')],
      );

      await tester.pumpWidget(buildSheet(
        mockCommunityRepo,
        groups: [group],
        communities: [
          CommunityItem(id: 'named', name: 'Trail Crew', memberCount: 3),
        ],
      ));
      await tester.pumpAndSettle();

      expect(find.text('Trail Crew'), findsOneWidget);
      // Named communities keep the community avatar (no faces substitution).
      expect(find.byType(CommunityAvatar), findsOneWidget);
    });
  });
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

/// Wraps [AccessSheet] in a ProviderScope that stubs the community repository
/// so no real network calls are made during tests. Pass [memberCommunityIds]
/// to seed the user-membership set used by the sheet to detect non-member
/// communities (#1705 / #1676). Defaults to "user is a member of every
/// community in [groups]" so existing tests don't need to change.
Widget buildSheet(
  MockCommunityRepository repo, {
  required List<AccessGroup> groups,
  int totalPeople = 5,
  int otherCommunityCount = 0,
  Set<String>? memberCommunityIds,
  List<CommunityItem>? communities,
}) {
  final memberIds = memberCommunityIds ?? {for (final g in groups) g.communityId};
  // By default the viewer is a member of every group's community, each with a
  // placeholder name. Pass [communities] to control the loaded CommunityItems
  // directly (e.g. a nameless community carrying memberPreviewFirstNames).
  final selectedCommunities = communities ??
      [for (final id in memberIds) CommunityItem(id: id, name: 'Community $id')];
  return ProviderScope(
    overrides: [
      communityRepositoryProvider.overrideWithValue(repo),
      communitiesProvider.overrideWith(
        () => _StubCommunitiesNotifier(
          CommunitiesState(communities: selectedCommunities),
        ),
      ),
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
        body: AccessSheet(
          creator: const AccessCreator(
            name: 'Alice',
            initials: 'A',
            timeAgo: '2 days ago',
          ),
          groups: groups,
          totalPeople: totalPeople,
          otherCommunityCount: otherCommunityCount,
        ),
      ),
    ),
  );
}

/// Returns a minimal [AccessGroup] with the given [communityId] and
/// [memberCount] (the proto-supplied fallback value).
AccessGroup makeGroup({
  String communityId = 'com-1',
  String communityName = 'Bike Club',
  int memberCount = 0,
}) {
  return AccessGroup(
    communityId: communityId,
    communityName: communityName,
    sharedByName: 'Alice',
    sharedByInitials: 'A',
    sharedTimeAgo: '3 days ago',
    sharedAtUnixSec: 1700000000,
    memberCount: memberCount,
  );
}

/// Returns a minimal [CommunityMember] for testing.
CommunityMember makeMember(String userId) {
  return CommunityMember(user: User(id: userId));
}

/// Stub notifier that returns a fixed [CommunitiesState] so tests can
/// control which communities the viewing user is treated as a member of.
class _StubCommunitiesNotifier extends CommunitiesNotifier {
  _StubCommunitiesNotifier(this._initial);

  final CommunitiesState _initial;

  @override
  CommunitiesState build() => _initial;
}
