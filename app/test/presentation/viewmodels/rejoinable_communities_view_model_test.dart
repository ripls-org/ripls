import 'package:fixnum/fixnum.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/presentation/viewmodels/rejoinable_communities_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'rejoinable_communities_view_model_test.mocks.dart';

@GenerateMocks([CommunityRepository])
void main() {
  group('RejoinableCommunitiesNotifier', () {
    late MockCommunityRepository mockRepo;
    late ProviderContainer container;

    RejoinableCommunityItem item({
      required String id,
      required String name,
      int leftAtUnixSec = 0,
    }) {
      return RejoinableCommunityItem(
        id: id,
        name: name,
        description: '',
        leftAtUnixSec: Int64.fromInts(0, leftAtUnixSec),
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
        item(id: 'a', name: 'Alpha', leftAtUnixSec: 200),
        item(id: 'b', name: 'Beta', leftAtUnixSec: 100),
      ];
      when(mockRepo.listRejoinableCommunities(
        refresh: anyNamed('refresh'),
      )).thenAnswer((_) async => items);

      final result =
          await container.read(rejoinableCommunitiesProvider.future);

      expect(result, items);
      verify(mockRepo.listRejoinableCommunities()).called(1);
    });

    test('build returns empty list when caller has no rejoinable memberships',
        () async {
      when(mockRepo.listRejoinableCommunities(
        refresh: anyNamed('refresh'),
      )).thenAnswer((_) async => []);

      final result =
          await container.read(rejoinableCommunitiesProvider.future);

      expect(result, isEmpty);
    });

    test(
      'preserves the server-provided descending-by-left-at ordering',
      () async {
        // Server contract: most-recently-left first. ViewModel must not
        // reorder.
        final items = [
          item(id: 'new', name: 'New', leftAtUnixSec: 300),
          item(id: 'mid', name: 'Mid', leftAtUnixSec: 200),
          item(id: 'old', name: 'Old', leftAtUnixSec: 100),
        ];
        when(mockRepo.listRejoinableCommunities(
          refresh: anyNamed('refresh'),
        )).thenAnswer((_) async => items);

        final result =
            await container.read(rejoinableCommunitiesProvider.future);

        expect(result.map((i) => i.id).toList(), ['new', 'mid', 'old']);
      },
    );

    test('refresh forces refresh=true on the repository', () async {
      when(mockRepo.listRejoinableCommunities(
        refresh: anyNamed('refresh'),
      )).thenAnswer((_) async => []);

      await container.read(rejoinableCommunitiesProvider.future);

      final notifier =
          container.read(rejoinableCommunitiesProvider.notifier);
      await notifier.refresh();

      verify(mockRepo.listRejoinableCommunities()).called(1);
      verify(mockRepo.listRejoinableCommunities(refresh: true)).called(1);
    });

    test('refresh failure surfaces as AsyncError', () async {
      when(mockRepo.listRejoinableCommunities(
        refresh: anyNamed('refresh'),
      )).thenAnswer((_) async => []);

      await container.read(rejoinableCommunitiesProvider.future);

      when(mockRepo.listRejoinableCommunities(refresh: true))
          .thenThrow(Exception('boom'));

      final notifier =
          container.read(rejoinableCommunitiesProvider.notifier);
      await notifier.refresh();

      final value = container.read(rejoinableCommunitiesProvider);
      expect(value.hasError, true);
      expect(value.error.toString(), contains('boom'));
    });

    test(
      'rebuilds when the cache-invalidation provider bumps',
      () async {
        final initial = [item(id: 'a', name: 'Alpha', leftAtUnixSec: 100)];
        final after = [
          item(id: 'a', name: 'Alpha', leftAtUnixSec: 100),
          item(id: 'b', name: 'Beta', leftAtUnixSec: 200),
        ];

        var call = 0;
        when(mockRepo.listRejoinableCommunities(
          refresh: anyNamed('refresh'),
        )).thenAnswer((_) async {
          call++;
          return call == 1 ? initial : after;
        });

        final first =
            await container.read(rejoinableCommunitiesProvider.future);
        expect(first.map((i) => i.id).toList(), ['a']);

        // Simulate cache invalidation from another flow (e.g.
        // RejoinCommunity success in the rejoin VM).
        container
            .read(rejoinableCommunitiesCacheInvalidationProvider.notifier)
            .notify();

        final second =
            await container.read(rejoinableCommunitiesProvider.future);
        expect(second.map((i) => i.id).toList(), ['a', 'b']);
        verify(mockRepo.listRejoinableCommunities()).called(2);
      },
    );

    test('handles disposal during the initial fetch', () async {
      when(mockRepo.listRejoinableCommunities(
        refresh: anyNamed('refresh'),
      )).thenAnswer((_) async {
        await Future.delayed(const Duration(milliseconds: 50));
        return [];
      });

      final future = container.read(rejoinableCommunitiesProvider.future);
      container.dispose();

      await expectLater(future, completes);
    });
  });
}
