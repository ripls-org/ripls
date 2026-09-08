import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/repositories/provisional_user_repository.dart';
import 'package:ripls/services/community_service.dart';

import 'provisional_user_repository_test.mocks.dart';

@GenerateMocks([CommunityService])
void main() {
  group('ProvisionalUserRepository', () {
    late ProvisionalUserRepository repository;
    late MockCommunityService mockService;
    late StashCacheManager cacheManager;

    setUp(() async {
      mockService = MockCommunityService();
      cacheManager = StashCacheManager();
      await cacheManager.initialize();
      repository = ProvisionalUserRepository(cacheManager, mockService);
    });

    // ── listProvisionalUsers — caching ──────────────────────────────────────────

    test('listProvisionalUsers returns list from service', () async {
      final expected = [
        ProvisionalUser(id: 'su1', name: 'Alice'),
        ProvisionalUser(id: 'su2', name: 'Bob'),
      ];
      when(
        mockService.listProvisionalUsers(communityId: 'com1'),
      ).thenAnswer((_) async => expected);

      final result = await repository.listProvisionalUsers('com1');

      expect(result, hasLength(2));
      expect(result.first.name, 'Alice');
      verify(mockService.listProvisionalUsers(communityId: 'com1')).called(1);
    });

    test('listProvisionalUsers caches response', () async {
      when(
        mockService.listProvisionalUsers(communityId: 'com1'),
      ).thenAnswer((_) async => [ProvisionalUser(id: 'su1', name: 'Alice')]);

      await repository.listProvisionalUsers('com1');
      await repository.listProvisionalUsers('com1');

      verify(mockService.listProvisionalUsers(communityId: 'com1')).called(1);
    });

    test(
      'listProvisionalUsers uses separate cache for different communities',
      () async {
        when(
          mockService.listProvisionalUsers(communityId: 'com1'),
        ).thenAnswer((_) async => [ProvisionalUser(id: 'su1', name: 'Alice')]);
        when(
          mockService.listProvisionalUsers(communityId: 'com2'),
        ).thenAnswer((_) async => [ProvisionalUser(id: 'su2', name: 'Bob')]);

        await repository.listProvisionalUsers('com1');
        await repository.listProvisionalUsers('com2');

        verify(mockService.listProvisionalUsers(communityId: 'com1')).called(1);
        verify(mockService.listProvisionalUsers(communityId: 'com2')).called(1);
      },
    );

    test('invalidateList forces re-fetch on next call', () async {
      when(
        mockService.listProvisionalUsers(communityId: 'com1'),
      ).thenAnswer((_) async => []);

      await repository.listProvisionalUsers('com1');
      await repository.invalidateList('com1');
      await repository.listProvisionalUsers('com1');

      verify(mockService.listProvisionalUsers(communityId: 'com1')).called(2);
    });

    // ── searchProvisionalUsers — not cached ─────────────────────────────────────

    test(
      'searchProvisionalUsers bypasses cache (every call hits the service)',
      () async {
        when(
          mockService.searchProvisionalUsers(
            communityId: 'com1',
            query: 'alice',
          ),
        ).thenAnswer((_) async => [ProvisionalUser(id: 'su1', name: 'Alice')]);

        await repository.searchProvisionalUsers(
          communityId: 'com1',
          query: 'alice',
        );
        await repository.searchProvisionalUsers(
          communityId: 'com1',
          query: 'alice',
        );

        verify(
          mockService.searchProvisionalUsers(
            communityId: 'com1',
            query: 'alice',
          ),
        ).called(2);
      },
    );

    // ── createProvisionalUser ───────────────────────────────────────────────────

    test('createProvisionalUser returns created provisional user', () async {
      final created = ProvisionalUser(id: 'su-new', name: 'Carol');
      when(
        mockService.createProvisionalUser(communityId: 'com1', name: 'Carol'),
      ).thenAnswer((_) async => created);

      final result = await repository.createProvisionalUser(
        communityId: 'com1',
        name: 'Carol',
      );

      expect(result.name, 'Carol');
    });

    test(
      'createProvisionalUser invalidates list cache so next fetch re-fetches',
      () async {
        final created = ProvisionalUser(id: 'su-new', name: 'Carol');
        when(
          mockService.createProvisionalUser(communityId: 'com1', name: 'Carol'),
        ).thenAnswer((_) async => created);
        when(
          mockService.listProvisionalUsers(communityId: 'com1'),
        ).thenAnswer((_) async => []);

        await repository.listProvisionalUsers('com1');
        await repository.createProvisionalUser(
          communityId: 'com1',
          name: 'Carol',
        );
        await repository.listProvisionalUsers('com1');

        verify(mockService.listProvisionalUsers(communityId: 'com1')).called(2);
      },
    );

    // ── getActivities — not cached ─────────────────────────────────────────

    test(
      'getActivities bypasses cache (every call hits the service)',
      () async {
        when(
          mockService.getProvisionalUserActivities(
            communityId: 'com1',
            provisionalUserId: 'su1',
          ),
        ).thenAnswer((_) async => GetProvisionalUserActivitiesResponse());

        await repository.getActivities(
          communityId: 'com1',
          provisionalUserId: 'su1',
        );
        await repository.getActivities(
          communityId: 'com1',
          provisionalUserId: 'su1',
        );

        verify(
          mockService.getProvisionalUserActivities(
            communityId: 'com1',
            provisionalUserId: 'su1',
          ),
        ).called(2);
      },
    );

    // ── getInviteLink ──────────────────────────────────────────────────────

    test('getInviteLink delegates to service', () async {
      final expected = GetProvisionalUserInviteLinkResponse();
      when(
        mockService.getProvisionalUserInviteLink(
          communityId: 'com1',
          provisionalUserId: 'su1',
        ),
      ).thenAnswer((_) async => expected);

      final result = await repository.getInviteLink(
        communityId: 'com1',
        provisionalUserId: 'su1',
      );

      expect(result, expected);
      verify(
        mockService.getProvisionalUserInviteLink(
          communityId: 'com1',
          provisionalUserId: 'su1',
        ),
      ).called(1);
    });

    // ── error propagation ──────────────────────────────────────────────────

    test('listProvisionalUsers propagates service errors', () {
      when(
        mockService.listProvisionalUsers(communityId: 'com1'),
      ).thenThrow(Exception('network error'));

      expect(() => repository.listProvisionalUsers('com1'), throwsException);
    });
  });
}
