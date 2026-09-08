import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/provisional_user_repository.dart';
import 'package:ripls/presentation/widgets/completion/dark_person_search.dart';
import 'package:ripls/services/providers.dart';

import '../../../helpers/l10n_helpers.dart';
import 'dark_person_search_test.mocks.dart';

@GenerateMocks([CommunityRepository, ProvisionalUserRepository])
void main() {
  late MockCommunityRepository mockCommunityRepo;
  late MockProvisionalUserRepository mockProvisionalRepo;

  User makeUser(String id, String name) => User(id: id, name: name);

  setUp(() {
    mockCommunityRepo = MockCommunityRepository();
    mockProvisionalRepo = MockProvisionalUserRepository();

    // Default stub: return empty lists for any search.
    when(
      mockCommunityRepo.searchMembers(
        communityId: anyNamed('communityId'),
        query: anyNamed('query'),
        limit: anyNamed('limit'),
      ),
    ).thenAnswer((_) async => []);
    when(
      mockProvisionalRepo.searchProvisionalUsers(
        communityId: anyNamed('communityId'),
        query: anyNamed('query'),
        limit: anyNamed('limit'),
      ),
    ).thenAnswer((_) async => []);
  });

  Widget buildWidget({
    required String communityId,
    Set<String>? searchCommunityIds,
  }) {
    return ProviderScope(
      overrides: [
        communityRepositoryProvider.overrideWithValue(mockCommunityRepo),
        provisionalUserRepositoryProvider.overrideWithValue(
          mockProvisionalRepo,
        ),
      ],
      child: localizedApp(
        Scaffold(
          body: DarkPersonSearch(
            communityId: communityId,
            searchCommunityIds: searchCommunityIds,
            excludedMemberIds: const {},
            excludedProvisionalIds: const {},
            onMemberSelected: (_) {},
            onProvisionalSelected: (_) {},
            onNewProvisionalRequested: (_) {},
          ),
        ),
      ),
    );
  }

  group('DarkPersonSearch — single community', () {
    testWidgets('issues one searchMembers call for the communityId', (
      tester,
    ) async {
      await tester.pumpWidget(buildWidget(communityId: 'comm-a'));
      await tester.pump(); // post-frame callback
      await tester.pumpAndSettle();

      verify(
        mockCommunityRepo.searchMembers(
          communityId: 'comm-a',
          query: anyNamed('query'),
          limit: anyNamed('limit'),
        ),
      ).called(1);
    });
  });

  group('DarkPersonSearch — multi-community fan-out', () {
    testWidgets('issues one searchMembers call per community ID', (
      tester,
    ) async {
      await tester.pumpWidget(
        buildWidget(
          communityId: 'comm-a',
          searchCommunityIds: {'comm-a', 'comm-b'},
        ),
      );
      await tester.pump();
      await tester.pumpAndSettle();

      verify(
        mockCommunityRepo.searchMembers(
          communityId: 'comm-a',
          query: anyNamed('query'),
          limit: anyNamed('limit'),
        ),
      ).called(1);
      verify(
        mockCommunityRepo.searchMembers(
          communityId: 'comm-b',
          query: anyNamed('query'),
          limit: anyNamed('limit'),
        ),
      ).called(1);
    });

    testWidgets('deduplicates users that appear in multiple communities', (
      tester,
    ) async {
      // alex appears in both communities; ben is only in comm-b.
      final alex = makeUser('user-alex', 'Alex');
      final ben = makeUser('user-ben', 'Ben');

      when(
        mockCommunityRepo.searchMembers(
          communityId: 'comm-a',
          query: anyNamed('query'),
          limit: anyNamed('limit'),
        ),
      ).thenAnswer((_) async => [alex]);
      when(
        mockCommunityRepo.searchMembers(
          communityId: 'comm-b',
          query: anyNamed('query'),
          limit: anyNamed('limit'),
        ),
      ).thenAnswer((_) async => [alex, ben]);

      int memberSelectedCount = 0;
      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            communityRepositoryProvider.overrideWithValue(mockCommunityRepo),
            provisionalUserRepositoryProvider.overrideWithValue(
              mockProvisionalRepo,
            ),
          ],
          child: localizedApp(
            Scaffold(
              body: DarkPersonSearch(
                communityId: 'comm-a',
                searchCommunityIds: {'comm-a', 'comm-b'},
                excludedMemberIds: const {},
                excludedProvisionalIds: const {},
                onMemberSelected: (_) => memberSelectedCount++,
                onProvisionalSelected: (_) {},
                onNewProvisionalRequested: (_) {},
              ),
            ),
          ),
        ),
      );
      await tester.pump();
      await tester.pumpAndSettle();

      // Grid shows Alex once and Ben once (alex is deduped, not shown twice).
      expect(find.text('Alex'), findsOneWidget);
      expect(find.text('Ben'), findsOneWidget);
    });

    testWidgets('shows plural header when two communities are in scope', (
      tester,
    ) async {
      // Stub must be set before pump so the grid preload returns a user and
      // the header row renders.
      when(
        mockCommunityRepo.searchMembers(
          communityId: anyNamed('communityId'),
          query: anyNamed('query'),
          limit: anyNamed('limit'),
        ),
      ).thenAnswer((_) async => [makeUser('u1', 'Alice')]);

      await tester.pumpWidget(
        buildWidget(
          communityId: 'comm-a',
          searchCommunityIds: {'comm-a', 'comm-b'},
        ),
      );
      await tester.pump();
      await tester.pumpAndSettle();

      expect(find.text('ADD FROM YOUR COMMUNITIES'), findsOneWidget);
      expect(find.text('ADD FROM YOUR COMMUNITY'), findsNothing);
    });
  });
}
