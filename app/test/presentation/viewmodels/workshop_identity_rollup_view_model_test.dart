import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem, CommunityMember;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/presentation/viewmodels/workshop_identity_rollup_view_model.dart';
import 'package:ripls/services/auth_state.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/community_providers.dart';
import 'package:ripls/services/providers/workshop_community_provider.dart';

import 'workshop_identity_rollup_view_model_test.mocks.dart';

@GenerateMocks([CommunityRepository])
void main() {
  CommunityMember member(String id, String name) {
    return CommunityMember(user: User()..id = id..name = name);
  }

  CommunityItem community(String id) =>
      CommunityItem()..id = id..name = 'C $id';

  late MockCommunityRepository mockRepo;
  late ProviderContainer container;

  ProviderContainer makeContainer({
    required List<CommunityItem> communities,
    required String viewerId,
    List<String>? enabledIds,
  }) {
    mockRepo = MockCommunityRepository();
    return ProviderContainer(
      overrides: [
        communityRepositoryProvider.overrideWithValue(mockRepo),
        communitiesProvider.overrideWith(
          () => _SeededCommunitiesNotifier(communities),
        ),
        // The Workshop scope is a single community now; tests that exercise
        // the rollup's cross-community dedup pin the scope explicitly.
        if (enabledIds != null)
          workshopEnabledCommunityIdsProvider.overrideWithValue(enabledIds),
        authStateProvider.overrideWith(
          () => _SeededAuthNotifier(
            AuthStateData(
              user: User()..id = viewerId..name = 'Me',
              isLoading: false,
              accessToken: 't',
            ),
          ),
        ),
      ],
    );
  }

  tearDown(() {
    container.dispose();
  });

  test('returns empty rollup when no communities in scope', () async {
    container = makeContainer(communities: [], viewerId: 'me');
    final rollup =
        await container.read(workshopIdentityRollupProvider.future);
    expect(rollup.firstFewNames, isEmpty);
    expect(rollup.otherCount, 0);
  });

  test('lists first three non-viewer names, folds the viewer into others',
      () async {
    container = makeContainer(
      communities: [community('c1')],
      viewerId: 'me',
    );
    when(mockRepo.getMembers('c1')).thenAnswer((_) async => [
          member('me', 'Me Self'),
          member('mike', 'Mike Davis'),
          member('diego', 'Diego Lopez'),
          member('sarah', 'Sarah Park'),
          member('jen', 'Jen Wu'),
          member('priya', 'Priya R'),
        ]);

    final rollup =
        await container.read(workshopIdentityRollupProvider.future);
    // Viewer is pinned last in the ordered list. With > 3 distinct
    // people we only show the first 3 non-viewer names; the viewer
    // counts toward "others" along with anyone else past slot 3.
    expect(rollup.firstFewNames, ['Mike', 'Diego', 'Sarah']);
    expect(rollup.otherCount, 3); // Jen + Priya + Me
  });

  test('dedupes members across communities by user id', () async {
    container = makeContainer(
      communities: [community('c1'), community('c2')],
      viewerId: 'me',
      enabledIds: ['c1', 'c2'],
    );
    when(mockRepo.getMembers('c1')).thenAnswer((_) async => [
          member('mike', 'Mike D'),
          member('diego', 'Diego L'),
        ]);
    when(mockRepo.getMembers('c2')).thenAnswer((_) async => [
          member('mike', 'Mike D'), // dup
          member('sarah', 'Sarah P'),
        ]);

    final rollup =
        await container.read(workshopIdentityRollupProvider.future);
    // Three distinct non-viewer members + viewer appended last via
    // the auth-state fallback (the viewer isn't in either roster
    // but is still pinned at the end of the ordered list, then
    // folded into the count because the slice is full).
    expect(rollup.firstFewNames, ['Mike', 'Diego', 'Sarah']);
    expect(rollup.otherCount, 1);
  });

  test('shows the viewer last when the scope has ≤ 3 distinct members',
      () async {
    container = makeContainer(
      communities: [community('c1')],
      viewerId: 'me',
    );
    when(mockRepo.getMembers('c1')).thenAnswer((_) async => [
          member('me', 'Me Myself'),
          member('mike', 'Mike D'),
        ]);

    final rollup =
        await container.read(workshopIdentityRollupProvider.future);
    expect(rollup.firstFewNames, ['Mike', 'Me']);
    expect(rollup.otherCount, 0);
  });

  test('renders the viewer alone when they are the only member',
      () async {
    container = makeContainer(
      communities: [community('c1')],
      viewerId: 'me',
    );
    when(mockRepo.getMembers('c1'))
        .thenAnswer((_) async => [member('me', 'Me Myself')]);

    final rollup =
        await container.read(workshopIdentityRollupProvider.future);
    expect(rollup.firstFewNames, ['Me']);
    expect(rollup.otherCount, 0);
  });

  test('handles single-name display gracefully', () async {
    container = makeContainer(
      communities: [community('c1')],
      viewerId: 'me',
    );
    when(mockRepo.getMembers('c1'))
        .thenAnswer((_) async => [member('mononym', 'Cher')]);

    final rollup =
        await container.read(workshopIdentityRollupProvider.future);
    // Viewer isn't in the roster — auth-state fallback name is "Me",
    // appended last. Two distinct people total, both fit in the slice.
    expect(rollup.firstFewNames, ['Cher', 'Me']);
    expect(rollup.otherCount, 0);
  });

  test('pins the viewer last when exactly three non-viewer members are present',
      () async {
    container = makeContainer(
      communities: [community('c1')],
      viewerId: 'me',
    );
    when(mockRepo.getMembers('c1')).thenAnswer((_) async => [
          member('mike', 'Mike D'),
          member('diego', 'Diego L'),
          member('sarah', 'Sarah P'),
          member('me', 'Me Self'),
        ]);

    final rollup =
        await container.read(workshopIdentityRollupProvider.future);
    // Four distinct people: three non-viewers fill the slice, the
    // viewer is the one "+ 1 other" in the count.
    expect(rollup.firstFewNames, ['Mike', 'Diego', 'Sarah']);
    expect(rollup.otherCount, 1);
  });

  test('does not crash when disposed during fetch', () async {
    container = makeContainer(
      communities: [community('c1')],
      viewerId: 'me',
    );
    // Pending future — never completes.
    when(mockRepo.getMembers('c1'))
        .thenAnswer((_) => Future<List<CommunityMember>>.delayed(
              const Duration(seconds: 5),
              () => const [],
            ));

    final future = container.read(workshopIdentityRollupProvider.future);
    container.dispose();
    // Future must complete (with either a value or an error) without
    // crashing the harness. Either outcome is acceptable — the test
    // exists to prove disposal-during-fetch is safe, not to enforce a
    // specific resolution path.
    await expectLater(future, completes);
  });
}

class _SeededCommunitiesNotifier extends CommunitiesNotifier {
  _SeededCommunitiesNotifier(this._communities);
  final List<CommunityItem> _communities;

  @override
  CommunitiesState build() {
    return CommunitiesState(communities: _communities);
  }
}

class _SeededAuthNotifier extends AuthStateNotifier {
  _SeededAuthNotifier(this._seed);
  final AuthStateData _seed;

  @override
  AuthStateData build() => _seed;
}
