import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/services/event_router.dart';
import 'package:ripls/services/providers.dart';

import 'event_router_test.mocks.dart';

@GenerateMocks([CommunityRepository])
void main() {
  late ProviderContainer container;
  late EventRouter router;
  late MockCommunityRepository mockCommunityRepository;

  setUp(() {
    mockCommunityRepository = MockCommunityRepository();
    when(mockCommunityRepository.refreshUserCommunities())
        .thenAnswer((_) async => <CommunityItem>[]);
    when(mockCommunityRepository.invalidateDeletedList())
        .thenAnswer((_) async {});

    container = ProviderContainer(overrides: [
      communityRepositoryProvider.overrideWithValue(mockCommunityRepository),
    ]);
    router = container.read(eventRouterProvider);
  });

  tearDown(() {
    container.dispose();
  });

  CommunityEventItem makeEvent(String id, CommunityEventType type) {
    return CommunityEventItem(
      id: id,
      communityId: 'community-1',
      eventType: type,
    );
  }

  group('EventRouter', () {
    test('transfer completed notifies transfer + portfolio + impact', () {
      final transferBefore =
          container.read(transferCacheInvalidationProvider);
      final portfolioBefore =
          container.read(portfolioCacheInvalidationProvider);
      final impactBefore =
          container.read(impactCacheInvalidationProvider);

      router.routeCommunityEvent(makeEvent(
        'e1',
        CommunityEventType.COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED,
      ));

      expect(container.read(transferCacheInvalidationProvider),
          transferBefore + 1);
      expect(container.read(portfolioCacheInvalidationProvider),
          portfolioBefore + 1);
      expect(container.read(impactCacheInvalidationProvider),
          impactBefore + 1);
    });

    test('transfer state changes notify transfer + portfolio', () {
      final transferBefore =
          container.read(transferCacheInvalidationProvider);
      final portfolioBefore =
          container.read(portfolioCacheInvalidationProvider);

      router.routeCommunityEvent(makeEvent(
        'e1b',
        CommunityEventType.COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE,
      ));

      expect(container.read(transferCacheInvalidationProvider),
          transferBefore + 1);
      expect(container.read(portfolioCacheInvalidationProvider),
          portfolioBefore + 1);
    });

    test('transfer detail events notify transfer only, not portfolio', () {
      final transferBefore =
          container.read(transferCacheInvalidationProvider);
      final portfolioBefore =
          container.read(portfolioCacheInvalidationProvider);

      router.routeCommunityEvent(makeEvent(
        'e1c',
        CommunityEventType.COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
      ));

      expect(container.read(transferCacheInvalidationProvider),
          transferBefore + 1);
      expect(container.read(portfolioCacheInvalidationProvider),
          portfolioBefore);
    });

    test('content listing event notifies content + search + feedStatus + feedListing', () {
      final contentBefore =
          container.read(contentCacheInvalidationProvider);
      final searchBefore =
          container.read(searchCacheInvalidationProvider);
      final feedStatusBefore =
          container.read(feedStatusCacheInvalidationProvider);
      final feedListingBefore =
          container.read(feedListingCacheInvalidationProvider);

      router.routeCommunityEvent(makeEvent(
        'e2',
        CommunityEventType.COMMUNITY_EVENT_TYPE_GEAR_SHARED,
      ));

      expect(container.read(contentCacheInvalidationProvider),
          contentBefore + 1);
      expect(container.read(searchCacheInvalidationProvider),
          searchBefore + 1);
      expect(container.read(feedStatusCacheInvalidationProvider),
          feedStatusBefore + 1);
      expect(container.read(feedListingCacheInvalidationProvider),
          feedListingBefore + 1);
    });

    test('request fulfilled notifies content + search + feedStatus + feedListing + impact', () {
      final impactBefore =
          container.read(impactCacheInvalidationProvider);
      final contentBefore =
          container.read(contentCacheInvalidationProvider);
      final feedListingBefore =
          container.read(feedListingCacheInvalidationProvider);

      router.routeCommunityEvent(makeEvent(
        'e2b',
        CommunityEventType.COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED,
      ));

      expect(container.read(contentCacheInvalidationProvider),
          contentBefore + 1);
      expect(container.read(impactCacheInvalidationProvider),
          impactBefore + 1);
      expect(container.read(feedListingCacheInvalidationProvider),
          feedListingBefore + 1);
    });

    test('detail-level events notify content only, not feedListing or search', () {
      final contentBefore =
          container.read(contentCacheInvalidationProvider);
      final searchBefore =
          container.read(searchCacheInvalidationProvider);
      final feedStatusBefore =
          container.read(feedStatusCacheInvalidationProvider);
      final feedListingBefore =
          container.read(feedListingCacheInvalidationProvider);

      router.routeCommunityEvent(makeEvent(
        'e3a',
        CommunityEventType.COMMUNITY_EVENT_TYPE_REQUEST_OFFER_WITHDRAWN,
      ));

      expect(container.read(contentCacheInvalidationProvider),
          contentBefore + 1);
      expect(container.read(searchCacheInvalidationProvider), searchBefore);
      expect(container.read(feedStatusCacheInvalidationProvider), feedStatusBefore);
      expect(container.read(feedListingCacheInvalidationProvider), feedListingBefore);
    });

    test('RSVP event notifies content + portfolio (Up next), not feedListing', () {
      final contentBefore =
          container.read(contentCacheInvalidationProvider);
      final portfolioBefore =
          container.read(portfolioCacheInvalidationProvider);
      final searchBefore =
          container.read(searchCacheInvalidationProvider);
      final feedListingBefore =
          container.read(feedListingCacheInvalidationProvider);

      router.routeCommunityEvent(makeEvent(
        'e3b',
        CommunityEventType.COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES,
      ));

      expect(container.read(contentCacheInvalidationProvider),
          contentBefore + 1);
      // RSVP refreshes Home "Up next" subtitle (#2492).
      expect(container.read(portfolioCacheInvalidationProvider),
          portfolioBefore + 1);
      expect(container.read(searchCacheInvalidationProvider), searchBefore);
      expect(container.read(feedListingCacheInvalidationProvider), feedListingBefore);
    });

    test('planning need/contribution events notify content only (#2724)', () {
      const planningTypes = [
        CommunityEventType.COMMUNITY_EVENT_TYPE_PLANNING_NEED_ADDED,
        CommunityEventType.COMMUNITY_EVENT_TYPE_PLANNING_NEED_REMOVED,
        CommunityEventType.COMMUNITY_EVENT_TYPE_PLANNING_NEED_CLAIMED,
        CommunityEventType.COMMUNITY_EVENT_TYPE_PLANNING_NEED_UPDATED,
        CommunityEventType.COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_ADDED,
        CommunityEventType.COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_REMOVED,
      ];
      final contentBefore = container.read(contentCacheInvalidationProvider);
      final searchBefore = container.read(searchCacheInvalidationProvider);
      final portfolioBefore =
          container.read(portfolioCacheInvalidationProvider);
      final feedListingBefore =
          container.read(feedListingCacheInvalidationProvider);

      for (var i = 0; i < planningTypes.length; i++) {
        router.routeCommunityEvent(makeEvent('e-plan-$i', planningTypes[i]));
      }

      // Each planning event refreshes open content views (the roster's needs
      // strip + claim pills) without touching listings.
      expect(container.read(contentCacheInvalidationProvider),
          contentBefore + planningTypes.length);
      expect(container.read(searchCacheInvalidationProvider), searchBefore);
      expect(container.read(portfolioCacheInvalidationProvider),
          portfolioBefore);
      expect(container.read(feedListingCacheInvalidationProvider),
          feedListingBefore);
    });

    test('roster-changed event notifies content + portfolio, not feedListing', () {
      final contentBefore =
          container.read(contentCacheInvalidationProvider);
      final portfolioBefore =
          container.read(portfolioCacheInvalidationProvider);
      final feedListingBefore =
          container.read(feedListingCacheInvalidationProvider);

      router.routeCommunityEvent(makeEvent(
        'e3c',
        CommunityEventType.COMMUNITY_EVENT_TYPE_EXPERIENCE_ROSTER_CHANGED,
      ));

      expect(container.read(contentCacheInvalidationProvider),
          contentBefore + 1);
      expect(container.read(portfolioCacheInvalidationProvider),
          portfolioBefore + 1);
      expect(container.read(feedListingCacheInvalidationProvider), feedListingBefore);
    });

    test('membership event notifies portfolio provider only', () {
      final portfolioBefore =
          container.read(portfolioCacheInvalidationProvider);
      final transferBefore =
          container.read(transferCacheInvalidationProvider);

      router.routeCommunityEvent(makeEvent(
        'e4',
        CommunityEventType.COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED,
      ));

      expect(container.read(portfolioCacheInvalidationProvider),
          portfolioBefore + 1);
      expect(
          container.read(transferCacheInvalidationProvider), transferBefore);
    });

    test('batch routing notifies each provider at most once', () {
      final contentBefore =
          container.read(contentCacheInvalidationProvider);
      final searchBefore =
          container.read(searchCacheInvalidationProvider);
      final transferBefore =
          container.read(transferCacheInvalidationProvider);
      final feedListingBefore =
          container.read(feedListingCacheInvalidationProvider);

      // Three content listing events + one transfer event in a single batch.
      router.routeCommunityEvents([
        makeEvent('b1', CommunityEventType.COMMUNITY_EVENT_TYPE_GEAR_SHARED),
        makeEvent('b2', CommunityEventType.COMMUNITY_EVENT_TYPE_REQUEST_CREATED),
        makeEvent('b3', CommunityEventType.COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED),
        makeEvent('b4', CommunityEventType.COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE),
      ]);

      // Content, search, feedStatus, feedListing each notified once (not three times).
      expect(container.read(contentCacheInvalidationProvider),
          contentBefore + 1);
      expect(container.read(searchCacheInvalidationProvider),
          searchBefore + 1);
      expect(container.read(feedListingCacheInvalidationProvider),
          feedListingBefore + 1);
      // Transfer notified once.
      expect(container.read(transferCacheInvalidationProvider),
          transferBefore + 1);
    });

    test('routeFcmEvent routes from FCM data payload', () {
      final transferBefore =
          container.read(transferCacheInvalidationProvider);

      router.routeFcmEvent({
        'type': 'community_event',
        'event_id': 'fcm-1',
        'event_type': 'COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED',
        'community_id': 'c1',
        'gear_id': 'g1',
      });

      expect(container.read(transferCacheInvalidationProvider),
          transferBefore + 1);
    });

    test('routeFcmEvent deduplicates with stream events', () {
      final transferBefore =
          container.read(transferCacheInvalidationProvider);

      // Stream delivers the event first.
      router.routeCommunityEvent(makeEvent(
        'shared-id',
        CommunityEventType.COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED,
      ));

      // FCM arrives with the same event ID — should be deduped.
      router.routeFcmEvent({
        'event_id': 'shared-id',
        'event_type': 'COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED',
        'community_id': 'c1',
      });

      // Should only increment once.
      expect(container.read(transferCacheInvalidationProvider),
          transferBefore + 1);
    });

    test('routeFcmEvent ignores unknown event types', () {
      final before = container.read(transferCacheInvalidationProvider);

      router.routeFcmEvent({
        'event_id': 'fcm-bad',
        'event_type': 'UNKNOWN_TYPE',
        'community_id': 'c1',
      });

      expect(container.read(transferCacheInvalidationProvider), before);
    });

    test('routeFcmEvent ignores missing event_id', () {
      final before = container.read(transferCacheInvalidationProvider);

      router.routeFcmEvent({
        'event_type': 'COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED',
        'community_id': 'c1',
      });

      expect(container.read(transferCacheInvalidationProvider), before);
    });

    test('deduplicates by event ID', () {
      final transferBefore =
          container.read(transferCacheInvalidationProvider);

      router.routeCommunityEvent(makeEvent(
        'dup-1',
        CommunityEventType.COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED,
      ));
      router.routeCommunityEvent(makeEvent(
        'dup-1',
        CommunityEventType.COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED,
      ));

      expect(container.read(transferCacheInvalidationProvider),
          transferBefore + 1);
    });

    test('ignores events with empty ID', () {
      final transferBefore =
          container.read(transferCacheInvalidationProvider);

      router.routeCommunityEvent(makeEvent(
        '',
        CommunityEventType.COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED,
      ));

      expect(
          container.read(transferCacheInvalidationProvider), transferBefore);
    });

    test('LRU eviction allows reprocessing of old event IDs', () {
      // Route 501 unique events to exceed the 500-event LRU limit.
      for (var i = 0; i < 501; i++) {
        router.routeCommunityEvent(makeEvent(
          'evict-$i',
          CommunityEventType.COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED,
        ));
      }

      // The first event ID should have been evicted from the dedup set.
      final afterBulk = container.read(transferCacheInvalidationProvider);
      router.routeCommunityEvent(makeEvent(
        'evict-0',
        CommunityEventType.COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED,
      ));

      expect(
          container.read(transferCacheInvalidationProvider), afterBulk + 1);
    });

    // The community-lifecycle family was unrouted until #2869: every one of
    // these fell through to the `unhandled event type` default and invalidated
    // nothing. COMMUNITY_DELETED is the load-bearing one — the per-community
    // stream terminating used to be what got a viewer off a deleted community,
    // and a per-user stream cannot terminate without dropping every other
    // community's realtime, so the event has to be handled instead.
    group('community lifecycle', () {
      test('community deleted clears the community and deleted lists',
          () async {
        final portfolioBefore =
            container.read(portfolioCacheInvalidationProvider);
        final feedListingBefore =
            container.read(feedListingCacheInvalidationProvider);
        final contentBefore =
            container.read(contentCacheInvalidationProvider);

        router.routeCommunityEvent(makeEvent(
          'deleted-1',
          CommunityEventType.COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED,
        ));
        // Repository invalidation is fire-and-forget off the routing path.
        await pumpEventQueue();

        // Clearing the repository cache is the point: bumping the notifier
        // alone would rebuild the view models onto the same stale list.
        verify(mockCommunityRepository.refreshUserCommunities()).called(1);
        verify(mockCommunityRepository.invalidateDeletedList()).called(1);
        expect(container.read(portfolioCacheInvalidationProvider),
            portfolioBefore + 1);
        expect(container.read(feedListingCacheInvalidationProvider),
            feedListingBefore + 1);
        expect(container.read(contentCacheInvalidationProvider),
            contentBefore + 1);
      });

      test('community restored clears the same caches as delete', () async {
        router.routeCommunityEvent(makeEvent(
          'restored-1',
          CommunityEventType.COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED,
        ));
        await pumpEventQueue();

        verify(mockCommunityRepository.refreshUserCommunities()).called(1);
        verify(mockCommunityRepository.invalidateDeletedList()).called(1);
      });

      test('community named refreshes the list but not the deleted list',
          () async {
        router.routeCommunityEvent(makeEvent(
          'named-1',
          CommunityEventType.COMMUNITY_EVENT_TYPE_COMMUNITY_NAMED,
        ));
        await pumpEventQueue();

        verify(mockCommunityRepository.refreshUserCommunities()).called(1);
        verifyNever(mockCommunityRepository.invalidateDeletedList());
      });

      test('ownership transferred refreshes the list and open content',
          () async {
        final contentBefore =
            container.read(contentCacheInvalidationProvider);

        router.routeCommunityEvent(makeEvent(
          'owner-1',
          CommunityEventType.COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED,
        ));
        await pumpEventQueue();

        verify(mockCommunityRepository.refreshUserCommunities()).called(1);
        expect(container.read(contentCacheInvalidationProvider),
            contentBefore + 1);
      });

      test('member rejoined refreshes the community list', () async {
        router.routeCommunityEvent(makeEvent(
          'rejoin-1',
          CommunityEventType
              .COMMUNITY_EVENT_TYPE_MEMBER_REJOINED_WITHIN_WINDOW,
        ));
        await pumpEventQueue();

        verify(mockCommunityRepository.refreshUserCommunities()).called(1);
      });

      // A batch that touches the same target twice must invalidate once, not
      // once per event — otherwise a poll returning a delete and a restore
      // would fire two list refetches back to back.
      test('a batch invalidates each target exactly once', () async {
        router.routeCommunityEvents([
          makeEvent('batch-1',
              CommunityEventType.COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED),
          makeEvent('batch-2',
              CommunityEventType.COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED),
          makeEvent('batch-3',
              CommunityEventType.COMMUNITY_EVENT_TYPE_COMMUNITY_NAMED),
        ]);
        await pumpEventQueue();

        verify(mockCommunityRepository.refreshUserCommunities()).called(1);
        verify(mockCommunityRepository.invalidateDeletedList()).called(1);
      });

      // The routing path is synchronous and fire-and-forget from the stream's
      // onData; an exception escaping would tear down the subscription over a
      // stale list.
      test('a failing repository invalidation does not throw', () async {
        when(mockCommunityRepository.refreshUserCommunities())
            .thenThrow(Exception('network down'));

        router.routeCommunityEvent(makeEvent(
          'boom-1',
          CommunityEventType.COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED,
        ));
        await pumpEventQueue();

        // The notifier-based targets still fired despite the repository throw.
        verify(mockCommunityRepository.invalidateDeletedList()).called(1);
      });
    });
  });
}
