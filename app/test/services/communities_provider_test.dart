import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/services/providers.dart';

void main() {
  group('CommunitiesNotifier.setCommunities', () {
    late ProviderContainer container;

    setUp(() {
      container = ProviderContainer();
    });

    tearDown(() {
      container.dispose();
    });

    test('emits exactly one state change when communities is non-empty',
        () async {
      final notifier =
          container.read(communitiesProvider.notifier);

      final emitted = <CommunitiesState>[];
      container.listen(communitiesProvider, (_, next) {
        emitted.add(next);
      });

      final communities = [
        CommunityItem(id: 'c1', name: 'Community One'),
        CommunityItem(id: 'c2', name: 'Community Two'),
      ];

      await notifier.setCommunities(communities);

      // Exactly one state change.
      expect(emitted.length, 1,
          reason:
              'setCommunities must emit exactly one state change, not two');
    });

    test('lists the communities passed in', () async {
      final notifier =
          container.read(communitiesProvider.notifier);

      CommunitiesState? capturedState;
      container.listen(communitiesProvider, (_, next) {
        capturedState ??= next;
      });

      final community = CommunityItem(id: 'c1', name: 'Community One');
      await notifier.setCommunities([community]);

      expect(capturedState?.communities.length, 1);
    });

    test('emits one state change with isLoading: false and no error',
        () async {
      final notifier =
          container.read(communitiesProvider.notifier);

      notifier.setLoading(true);

      CommunitiesState? capturedState;
      container.listen(communitiesProvider, (_, next) {
        capturedState ??= next;
      });

      await notifier.setCommunities([
        CommunityItem(id: 'c1', name: 'Community One'),
      ]);

      expect(capturedState?.isLoading, isFalse);
      expect(capturedState?.errorMessage, isNull);
    });

    test('emits single state change for empty communities list', () async {
      final notifier =
          container.read(communitiesProvider.notifier);

      notifier.setLoading(true);

      final emitted = <CommunitiesState>[];
      container.listen(communitiesProvider, (_, next) {
        emitted.add(next);
      });

      await notifier.setCommunities([]);

      expect(emitted.length, 1,
          reason: 'Empty communities must emit exactly one state change');
      expect(emitted.first.communities, isEmpty);
      expect(emitted.first.isLoading, isFalse);
    });
  });

  group('CommunitiesState getters', () {
    test('communityIds exposes the IDs of every community', () {
      final c1 = CommunityItem(id: 'c1', name: 'Community One');
      final c2 = CommunityItem(id: 'c2', name: 'Community Two');

      const empty = CommunitiesState();
      final filled = empty.copyWith(communities: [c1, c2]);

      expect(filled.communities, equals([c1, c2]));
      expect(filled.communityIds, equals(['c1', 'c2']));
    });
  });
}
