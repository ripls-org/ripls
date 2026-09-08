import 'package:fixnum/fixnum.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/presentation/viewmodels/deleted_communities_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'deleted_communities_view_model_test.mocks.dart';

@GenerateMocks([CommunityRepository])
void main() {
  group('DeletedCommunitiesNotifier', () {
    late MockCommunityRepository mockRepo;
    late ProviderContainer container;

    DeletedCommunityItem item({
      required String id,
      required String name,
      int deletedAtUnixSec = 0,
    }) {
      return DeletedCommunityItem(
        id: id,
        name: name,
        description: '',
        deletedAtUnixSec: Int64.fromInts(0, deletedAtUnixSec),
      );
    }

    setUp(() {
      mockRepo = MockCommunityRepository();
      container = ProviderContainer(
        overrides: [
          communityRepositoryProvider.overrideWithValue(mockRepo),
        ],
      );
    });

    tearDown(() {
      container.dispose();
      reset(mockRepo);
    });

    test('build loads the list from the repository', () async {
      final items = [
        item(id: 'a', name: 'Alpha', deletedAtUnixSec: 100),
        item(id: 'b', name: 'Beta', deletedAtUnixSec: 200),
      ];
      when(mockRepo.listDeletedCommunitiesForRestore(
        refresh: anyNamed('refresh'),
      )).thenAnswer((_) async => items);

      final result =
          await container.read(deletedCommunitiesProvider.future);

      expect(result, items);
      verify(mockRepo.listDeletedCommunitiesForRestore()).called(1);
    });

    test('build returns empty list when caller has no eligible communities',
        () async {
      when(mockRepo.listDeletedCommunitiesForRestore(
        refresh: anyNamed('refresh'),
      )).thenAnswer((_) async => []);

      final result =
          await container.read(deletedCommunitiesProvider.future);

      expect(result, isEmpty);
    });

    test('preserves the server-provided ascending-by-deleted-at ordering',
        () async {
      // Server contract: oldest deletion first. The ViewModel must
      // not reorder.
      final items = [
        item(id: 'old', name: 'Old', deletedAtUnixSec: 100),
        item(id: 'mid', name: 'Mid', deletedAtUnixSec: 200),
        item(id: 'new', name: 'New', deletedAtUnixSec: 300),
      ];
      when(mockRepo.listDeletedCommunitiesForRestore(
        refresh: anyNamed('refresh'),
      )).thenAnswer((_) async => items);

      final result =
          await container.read(deletedCommunitiesProvider.future);

      expect(result.map((i) => i.id).toList(), ['old', 'mid', 'new']);
    });

    test('refresh forces refresh=true on the repository', () async {
      when(mockRepo.listDeletedCommunitiesForRestore(
        refresh: anyNamed('refresh'),
      )).thenAnswer((_) async => []);

      await container.read(deletedCommunitiesProvider.future);

      final notifier = container.read(deletedCommunitiesProvider.notifier);
      await notifier.refresh();

      verify(mockRepo.listDeletedCommunitiesForRestore()).called(1);
      verify(mockRepo.listDeletedCommunitiesForRestore(refresh: true))
          .called(1);
    });

    test('refresh failure surfaces as AsyncError', () async {
      when(mockRepo.listDeletedCommunitiesForRestore(
        refresh: anyNamed('refresh'),
      )).thenAnswer((_) async => []);

      await container.read(deletedCommunitiesProvider.future);

      when(mockRepo.listDeletedCommunitiesForRestore(refresh: true))
          .thenThrow(Exception('boom'));

      final notifier = container.read(deletedCommunitiesProvider.notifier);
      await notifier.refresh();

      final value = container.read(deletedCommunitiesProvider);
      expect(value.hasError, true);
      expect(value.error.toString(), contains('boom'));
    });

    test('rebuilds when the cache-invalidation provider bumps', () async {
      final initial = [item(id: 'a', name: 'Alpha', deletedAtUnixSec: 100)];
      final after = [
        item(id: 'a', name: 'Alpha', deletedAtUnixSec: 100),
        item(id: 'b', name: 'Beta', deletedAtUnixSec: 200),
      ];

      var call = 0;
      when(mockRepo.listDeletedCommunitiesForRestore(
        refresh: anyNamed('refresh'),
      )).thenAnswer((_) async {
        call++;
        return call == 1 ? initial : after;
      });

      // Initial build.
      final first =
          await container.read(deletedCommunitiesProvider.future);
      expect(first.map((i) => i.id).toList(), ['a']);

      // Simulate the cache being invalidated (e.g. another screen
      // soft-deleted a community); the provider must refetch
      // immediately rather than wait for autoDispose.
      container
          .read(deletedCommunitiesCacheInvalidationProvider.notifier)
          .notify();

      final second =
          await container.read(deletedCommunitiesProvider.future);
      expect(second.map((i) => i.id).toList(), ['a', 'b']);
      verify(mockRepo.listDeletedCommunitiesForRestore()).called(2);
    });

    test('handles disposal during the initial fetch', () async {
      when(mockRepo.listDeletedCommunitiesForRestore(
        refresh: anyNamed('refresh'),
      )).thenAnswer((_) async {
        await Future.delayed(const Duration(milliseconds: 50));
        return [];
      });

      final future = container.read(deletedCommunitiesProvider.future);
      container.dispose();

      await expectLater(future, completes);
    });
  });
}
