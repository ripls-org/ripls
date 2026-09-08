import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/services/providers/community_providers.dart';
import 'package:ripls/services/providers/workshop_community_provider.dart';

class _TestCommunitiesNotifier extends CommunitiesNotifier {
  _TestCommunitiesNotifier(this._communities);
  final List<CommunityItem> _communities;

  @override
  CommunitiesState build() {
    return CommunitiesState(communities: _communities);
  }
}

CommunityItem _community(String id, String name) {
  return CommunityItem()
    ..id = id
    ..name = name;
}

void main() {
  group('WorkshopCommunityNotifier', () {
    late ProviderContainer container;

    setUp(() {
      container = ProviderContainer();
    });

    tearDown(() {
      container.dispose();
    });

    test('initial state is null (Everything)', () {
      expect(container.read(workshopCommunityProvider), isNull);
    });

    test('select(communityId) updates state', () {
      container.read(workshopCommunityProvider.notifier).select('c-1');
      expect(container.read(workshopCommunityProvider), 'c-1');
    });

    test('select(null) returns to Everything', () {
      final notifier = container.read(workshopCommunityProvider.notifier);
      notifier.select('c-1');
      notifier.select(null);
      expect(container.read(workshopCommunityProvider), isNull);
    });
  });

  group('workshopEnabledCommunityIdsProvider', () {
    ProviderContainer makeContainer(List<CommunityItem> memberships) {
      return ProviderContainer(
        overrides: [
          communitiesProvider.overrideWith(
            () => _TestCommunitiesNotifier(memberships),
          ),
        ],
      );
    }

    test('no explicit selection defaults to the first community (no All scope)',
        () {
      final container = makeContainer([
        _community('c-1', 'A'),
        _community('c-2', 'B'),
        _community('c-3', 'C'),
      ]);
      addTearDown(container.dispose);

      // The Workshop no longer has an "Everything" aggregate — an unpinned
      // scope falls back to just the first community.
      final ids = container.read(workshopEnabledCommunityIdsProvider);
      expect(ids, ['c-1']);
    });

    test('workshopActiveCommunityProvider falls back to the first community',
        () {
      final container = makeContainer([
        _community('c-1', 'A'),
        _community('c-2', 'B'),
      ]);
      addTearDown(container.dispose);

      expect(container.read(workshopActiveCommunityProvider)?.id, 'c-1');

      container.read(workshopCommunityProvider.notifier).select('c-2');
      expect(container.read(workshopActiveCommunityProvider)?.id, 'c-2');
    });

    test('non-null selection returns just that id', () {
      final container = makeContainer([
        _community('c-1', 'A'),
        _community('c-2', 'B'),
      ]);
      addTearDown(container.dispose);

      container.read(workshopCommunityProvider.notifier).select('c-2');
      expect(container.read(workshopEnabledCommunityIdsProvider), ['c-2']);
    });

    test('empty memberships + null selection returns empty list', () {
      final container = makeContainer(const []);
      addTearDown(container.dispose);

      expect(container.read(workshopEnabledCommunityIdsProvider), isEmpty);
    });
  });

  group('cross-surface isolation', () {
    test(
        'workshop selection does NOT propagate to communitiesProvider.communityIds',
        () {
      final container = ProviderContainer(
        overrides: [
          communitiesProvider.overrideWith(
            () => _TestCommunitiesNotifier([
              _community('c-1', 'A'),
              _community('c-2', 'B'),
            ]),
          ),
        ],
      );
      addTearDown(container.dispose);

      // Set a specific Workshop scope.
      container.read(workshopCommunityProvider.notifier).select('c-1');

      // The Workshop provider reports the narrowed scope.
      expect(container.read(workshopEnabledCommunityIdsProvider), ['c-1']);

      // But the portfolio-wide communitiesProvider remains
      // untouched — Feed search (#1896) and Inbox filtering (#1897)
      // continue to read every membership.
      final portfolio =
          container.read(communitiesProvider).communityIds;
      expect(portfolio, ['c-1', 'c-2']);
    });
  });
}
