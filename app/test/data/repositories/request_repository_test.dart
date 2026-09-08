import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/cache/stash_cache_manager.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart'
    show Request, RequestState;
import 'package:ripls/data/gen/ripls/api/request_service.pb.dart'
    show MarkRequestFulfilledResponse;
import 'package:ripls/data/repositories/chat_repository.dart';
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/data/repositories/profile_repository.dart';
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/services/request_service.dart';

import 'request_repository_test.mocks.dart';

@GenerateMocks([RequestService, ChatRepository, FeedRepository, SearchRepository, ProfileRepository])
void main() {
  group('RequestRepository', () {
    late RequestRepository repository;
    late MockRequestService mockService;
    late MockChatRepository mockChatRepository;
    late MockFeedRepository mockFeedRepository;
    late MockSearchRepository mockSearchRepository;
    late MockProfileRepository mockProfileRepository;
    late StashCacheManager cacheManager;

    setUp(() async {
      mockService = MockRequestService();
      mockChatRepository = MockChatRepository();
      mockFeedRepository = MockFeedRepository();
      mockSearchRepository = MockSearchRepository();
      mockProfileRepository = MockProfileRepository();
      cacheManager = StashCacheManager();
      await cacheManager.initialize();
      when(mockProfileRepository.invalidateAll()).thenAnswer((_) async {});
      repository = RequestRepository(
        cacheManager,
        mockService,
        mockChatRepository,
        mockFeedRepository,
        mockSearchRepository,
        onProfileInvalidated: () => mockProfileRepository.invalidateAll(),
      );
    });

    group('listRequests', () {
      test('fetches requests from service', () async {
        final mockRequests = [
          Request(id: 'request1', description: 'Need camping gear'),
          Request(id: 'request2', description: 'Looking for kayak'),
        ];

        when(
          mockService.listRequests(communityId: 'community1'),
        ).thenAnswer((_) async => mockRequests);

        final result = await repository.listRequests(communityId: 'community1');

        expect(result.length, 2);
        expect(result[0].id, 'request1');
        expect(result[1].id, 'request2');
        verify(mockService.listRequests(communityId: 'community1')).called(1);
      });

      test('caches requests', () async {
        final mockRequests = [
          Request(id: 'request1', description: 'Need camping gear'),
        ];

        when(
          mockService.listRequests(communityId: 'community1'),
        ).thenAnswer((_) async => mockRequests);

        // First call - should fetch
        await repository.listRequests(communityId: 'community1');

        // Second call - should use cache
        await repository.listRequests(communityId: 'community1');

        // Service should only be called once
        verify(mockService.listRequests(communityId: 'community1')).called(1);
      });

      test('filters by state when provided', () async {
        final mockRequests = [
          Request(
            id: 'request1',
            description: 'Need camping gear',
            state: RequestState.REQUEST_STATE_ACTIVE,
          ),
        ];

        when(
          mockService.listRequests(
            communityId: 'community1',
            state: RequestState.REQUEST_STATE_ACTIVE,
          ),
        ).thenAnswer((_) async => mockRequests);

        final result = await repository.listRequests(
          communityId: 'community1',
          state: RequestState.REQUEST_STATE_ACTIVE,
        );

        expect(result.length, 1);
        expect(result[0].state, RequestState.REQUEST_STATE_ACTIVE);
        verify(
          mockService.listRequests(
            communityId: 'community1',
            state: RequestState.REQUEST_STATE_ACTIVE,
          ),
        ).called(1);
      });

      test('caches separately by community and state', () async {
        final allRequests = [
          Request(id: 'request1', description: 'Need camping gear'),
          Request(id: 'request2', description: 'Looking for kayak'),
        ];
        final activeRequests = [
          Request(
            id: 'request1',
            description: 'Need camping gear',
            state: RequestState.REQUEST_STATE_ACTIVE,
          ),
        ];

        when(
          mockService.listRequests(communityId: 'community1'),
        ).thenAnswer((_) async => allRequests);
        when(
          mockService.listRequests(
            communityId: 'community1',
            state: RequestState.REQUEST_STATE_ACTIVE,
          ),
        ).thenAnswer((_) async => activeRequests);

        // Fetch all requests
        final all = await repository.listRequests(communityId: 'community1');
        expect(all.length, 2);

        // Fetch active-specific requests (should be separate cache)
        final active = await repository.listRequests(
          communityId: 'community1',
          state: RequestState.REQUEST_STATE_ACTIVE,
        );
        expect(active.length, 1);

        // Each should have been called once
        verify(mockService.listRequests(communityId: 'community1')).called(1);
        verify(
          mockService.listRequests(
            communityId: 'community1',
            state: RequestState.REQUEST_STATE_ACTIVE,
          ),
        ).called(1);
      });
    });

    group('listMyRequests', () {
      test('fetches my requests from service', () async {
        final mockRequests = [
          Request(id: 'request1', description: 'Need camping gear'),
          Request(id: 'request2', description: 'Looking for kayak'),
        ];

        when(
          mockService.listMyRequests(),
        ).thenAnswer((_) async => mockRequests);

        final result = await repository.listMyRequests();

        expect(result.length, 2);
        expect(result[0].id, 'request1');
        expect(result[1].id, 'request2');
        verify(mockService.listMyRequests()).called(1);
      });

      test('caches my requests', () async {
        final mockRequests = [
          Request(id: 'request1', description: 'Need camping gear'),
        ];

        when(
          mockService.listMyRequests(),
        ).thenAnswer((_) async => mockRequests);

        // First call - should fetch
        await repository.listMyRequests();

        // Second call - should use cache
        await repository.listMyRequests();

        // Service should only be called once
        verify(mockService.listMyRequests()).called(1);
      });

      test('filters by communityId when provided', () async {
        final mockRequests = [
          Request(id: 'request1', description: 'Need camping gear'),
        ];

        when(
          mockService.listMyRequests(communityId: 'community1'),
        ).thenAnswer((_) async => mockRequests);

        final result = await repository.listMyRequests(
          communityId: 'community1',
        );

        expect(result.length, 1);
        verify(mockService.listMyRequests(communityId: 'community1')).called(1);
      });
    });

    group('getRequest', () {
      test('fetches request from service', () async {
        final mockRequest = Request(
          id: 'request1',
          description: 'Need camping gear',
        );

        when(
          mockService.getRequest(requestId: 'request1'),
        ).thenAnswer((_) async => mockRequest);

        final result = await repository.getRequest(requestId: 'request1');

        expect(result.id, 'request1');
        expect(result.description, 'Need camping gear');
        verify(mockService.getRequest(requestId: 'request1')).called(1);
      });

      test('caches request', () async {
        final mockRequest = Request(
          id: 'request1',
          description: 'Need camping gear',
        );

        when(
          mockService.getRequest(requestId: 'request1'),
        ).thenAnswer((_) async => mockRequest);

        // First call - should fetch
        await repository.getRequest(requestId: 'request1');

        // Second call - should use cache
        await repository.getRequest(requestId: 'request1');

        // Service should only be called once
        verify(mockService.getRequest(requestId: 'request1')).called(1);
      });
    });

    group('submitRequest', () {
      test('submits request and invalidates cache', () async {
        when(
          mockService.submitRequest(
            title: 'Camping Gear',
            description: 'Need camping gear',
          ),
        ).thenAnswer((_) async => 'request1');

        final requestId = await repository.submitRequest(
          title: 'Camping Gear',
          description: 'Need camping gear',
        );

        expect(requestId, 'request1');
        verify(
          mockService.submitRequest(
            title: 'Camping Gear',
            description: 'Need camping gear',
          ),
        ).called(1);
      });

      // Per-community search-cache invalidation on submit moved to shareRequest
      // (#2529): SubmitRequest no longer takes a community, so the request lands
      // only in its freshly-created per-item community here.
      test('invalidates feed cache after submitting request', () async {
        when(
          mockService.submitRequest(
            title: 'Camping Gear',
            description: 'Need camping gear',
          ),
        ).thenAnswer((_) async => 'request1');

        await repository.submitRequest(
          title: 'Camping Gear',
          description: 'Need camping gear',
        );

        // Verify feed cache was invalidated
        verify(mockFeedRepository.invalidateFeed()).called(1);
      });
    });

    group('acceptRequestOffer', () {
      test('toggles acceptance and returns the updated Request', () async {
        const requestId = 'request1';
        const contributionId = 'contribution1';

        final request = Request(
          id: requestId,
          title: 'Test request',
          description: 'Test description',
        );

        when(
          mockService.acceptRequestOffer(
            requestId: requestId,
            contributionId: contributionId,
          ),
        ).thenAnswer((_) async => request);

        final result = await repository.acceptRequestOffer(
          requestId: requestId,
          contributionId: contributionId,
        );

        expect(result.id, requestId);
        verify(
          mockService.acceptRequestOffer(
            requestId: requestId,
            contributionId: contributionId,
          ),
        ).called(1);
      });
    });

    group('offerToFulfill', () {
      test('offers to fulfill and returns Request', () async {
        const requestId = 'request1';
        const communityId = 'community1';
        const conversationId = 'conversation1';

        final request = Request(
          id: requestId,
          conversationId: conversationId,
          title: 'Test request',
          description: 'Test description',
        );

        when(
          mockService.offerToFulfill(
            requestId: requestId,
            communityId: communityId,
          ),
        ).thenAnswer((_) async => request);

        final result = await repository.offerToFulfill(
          requestId: requestId,
          communityId: communityId,
        );

        expect(result.id, requestId);
        expect(result.conversationId, conversationId);
        verify(
          mockService.offerToFulfill(
            requestId: requestId,
            communityId: communityId,
          ),
        ).called(1);
      });

      test('passes communityId to service', () async {
        const requestId = 'request1';
        const communityId = 'community-456';

        final request = Request(
          id: requestId,
          conversationId: 'conversation1',
          title: 'Test request',
          description: 'Test description',
        );

        when(
          mockService.offerToFulfill(
            requestId: requestId,
            communityId: communityId,
          ),
        ).thenAnswer((_) async => request);

        await repository.offerToFulfill(
          requestId: requestId,
          communityId: communityId,
        );

        verify(
          mockService.offerToFulfill(
            requestId: requestId,
            communityId: communityId,
          ),
        ).called(1);
      });

      test('invalidates conversation cache when offering to fulfill', () async {
        const requestId = 'request1';
        const communityId = 'community1';
        const conversationId = 'conversation1';

        final request = Request(
          id: requestId,
          conversationId: conversationId,
          title: 'Test request',
          description: 'Test description',
        );

        when(
          mockService.offerToFulfill(
            requestId: requestId,
            communityId: communityId,
          ),
        ).thenAnswer((_) async => request);
        when(
          mockChatRepository.refreshConversations(),
        ).thenAnswer((_) async {});

        await repository.offerToFulfill(
          requestId: requestId,
          communityId: communityId,
        );

        // Verify that conversation cache was invalidated
        verify(mockChatRepository.refreshConversations()).called(1);
      });
    });

    group('markRequestFulfilled', () {
      test('marks request as fulfilled', () async {
        final mockRequest = Request(
          id: 'request1',
          description: 'Test request',
          state: RequestState.REQUEST_STATE_FULFILLED,
        );

        final mockResponse = MarkRequestFulfilledResponse(
          request: mockRequest,
        );

        when(
          mockService.markRequestFulfilled(
            requestId: 'request1',
            resolutionSummary: null,
          ),
        ).thenAnswer((_) async => mockResponse);

        final result = await repository.markRequestFulfilled(requestId: 'request1');

        expect(result.request.id, 'request1');
        expect(result.request.state, RequestState.REQUEST_STATE_FULFILLED);
        verify(
          mockService.markRequestFulfilled(
            requestId: 'request1',
            resolutionSummary: null,
          ),
        ).called(1);
      });

      test('marks request as fulfilled with resolution summary', () async {
        final mockRequest = Request(
          id: 'request1',
          description: 'Test request',
          state: RequestState.REQUEST_STATE_FULFILLED,
          resolutionSummary: 'Alice provided tools',
        );

        final mockResponse = MarkRequestFulfilledResponse(
          request: mockRequest,
        );

        when(
          mockService.markRequestFulfilled(
            requestId: 'request1',
            resolutionSummary: 'Alice provided tools',
          ),
        ).thenAnswer((_) async => mockResponse);

        final result = await repository.markRequestFulfilled(
          requestId: 'request1',
          resolutionSummary: 'Alice provided tools',
        );

        expect(result.request.id, 'request1');
        expect(result.request.resolutionSummary, 'Alice provided tools');
        verify(
          mockService.markRequestFulfilled(
            requestId: 'request1',
            resolutionSummary: 'Alice provided tools',
          ),
        ).called(1);
      });
    });

    group('cancelRequest', () {
      test('cancels request', () async {
        when(
          mockService.cancelRequest(requestId: 'request1'),
        ).thenAnswer((_) async => 'event-id');

        await repository.cancelRequest(requestId: 'request1');

        verify(mockService.cancelRequest(requestId: 'request1')).called(1);
      });
    });

    group('deleteRequest', () {
      test('deletes request', () async {
        when(
          mockService.deleteRequest(requestId: 'request1'),
        ).thenAnswer((_) async {});

        await repository.deleteRequest(requestId: 'request1');

        verify(mockService.deleteRequest(requestId: 'request1')).called(1);
      });

      test('invalidates feed cache after deleting request', () async {
        when(
          mockService.deleteRequest(requestId: 'request1'),
        ).thenAnswer((_) async {});

        await repository.deleteRequest(requestId: 'request1');

        // Verify all feed caches were invalidated
        verify(mockFeedRepository.invalidateAllFeeds()).called(1);
      });

      test('invalidates search cache after deleting request', () async {
        when(
          mockService.deleteRequest(requestId: 'request1'),
        ).thenAnswer((_) async {});

        await repository.deleteRequest(requestId: 'request1');

        // Verify search cache was invalidated
        verify(mockSearchRepository.invalidateSearches()).called(1);
      });

      test('invalidates request cache after deleting', () async {
        final mockRequest = Request(
          id: 'request1',
          title: 'Test Request',
          description: 'Test description',
        );

        when(mockService.getRequest(requestId: 'request1'))
            .thenAnswer((_) async => mockRequest);
        when(
          mockService.deleteRequest(requestId: 'request1'),
        ).thenAnswer((_) async {});

        // Populate cache
        await repository.getRequest(requestId: 'request1');

        // Delete request
        await repository.deleteRequest(requestId: 'request1');

        // Next get should re-fetch (cache was invalidated)
        await repository.getRequest(requestId: 'request1');

        // getRequest should be called twice (initial + after delete)
        verify(mockService.getRequest(requestId: 'request1')).called(2);
      });

      test('invalidates conversation cache after deleting (cascade deletion)',
          () async {
        when(
          mockService.deleteRequest(requestId: 'request1'),
        ).thenAnswer((_) async {});

        await repository.deleteRequest(requestId: 'request1');

        // Verify conversations cache was refreshed (cascade deletion removes conversation)
        verify(mockChatRepository.refreshConversations()).called(1);
      });

      test('invalidates profile cache after deleting request', () async {
        when(
          mockService.deleteRequest(requestId: 'request1'),
        ).thenAnswer((_) async {});

        await repository.deleteRequest(requestId: 'request1');

        verify(mockProfileRepository.invalidateAll()).called(1);
      });
    });

    group('updateRequest', () {
      test('updates request description', () async {
        when(
          mockService.updateRequest(
            requestId: 'request1',
            description: 'Updated description',
          ),
        ).thenAnswer((_) async {});

        await repository.updateRequest(
          requestId: 'request1',
          description: 'Updated description',
        );

        verify(
          mockService.updateRequest(
            requestId: 'request1',
            description: 'Updated description',
          ),
        ).called(1);
      });

      // Phase 1.1: Update Operation Cache Invalidation Tests
      test('invalidates search cache after updating request', () async {
        when(
          mockService.updateRequest(
            requestId: 'request1',
            title: 'Updated Title',
            description: 'Updated description',
          ),
        ).thenAnswer((_) async {});

        await repository.updateRequest(
          requestId: 'request1',
          title: 'Updated Title',
          description: 'Updated description',
        );

        // Verify search cache was invalidated
        verify(mockSearchRepository.invalidateSearches()).called(1);
      });

      test('invalidates request cache after updating', () async {
        final mockRequest = Request(
          id: 'request1',
          title: 'Original Title',
          description: 'Original description',
        );

        when(mockService.getRequest(requestId: 'request1'))
            .thenAnswer((_) async => mockRequest);
        when(
          mockService.updateRequest(
            requestId: 'request1',
            description: 'Updated description',
          ),
        ).thenAnswer((_) async {});

        // Populate cache
        await repository.getRequest(requestId: 'request1');

        // Update request
        await repository.updateRequest(
          requestId: 'request1',
          description: 'Updated description',
        );

        // Next get should re-fetch (cache was invalidated)
        await repository.getRequest(requestId: 'request1');

        // getRequest should be called twice (initial + after update)
        verify(mockService.getRequest(requestId: 'request1')).called(2);
      });
    });

    group('refresh methods', () {
      test('refreshRequests returns data', () async {
        final mockRequests = [
          Request(id: 'request1', description: 'Need camping gear'),
        ];

        when(
          mockService.listRequests(
            communityId: anyNamed('communityId'),
            state: anyNamed('state'),
          ),
        ).thenAnswer((_) async => mockRequests);

        // Refresh should return data
        final result = await repository.refreshRequests(
          communityId: 'community1',
        );

        expect(result.length, 1);
        expect(result[0].id, 'request1');
      });

      test('refreshMyRequests returns data', () async {
        final mockRequests = [
          Request(id: 'request1', description: 'Need camping gear'),
        ];

        when(
          mockService.listMyRequests(communityId: anyNamed('communityId')),
        ).thenAnswer((_) async => mockRequests);

        // Refresh should return data
        final result = await repository.refreshMyRequests();

        expect(result.length, 1);
        expect(result[0].id, 'request1');
      });

      test('refreshRequest returns data', () async {
        final mockRequest = Request(
          id: 'request1',
          description: 'Need camping gear',
        );

        when(
          mockService.getRequest(requestId: 'request1'),
        ).thenAnswer((_) async => mockRequest);

        // Refresh should return data
        final result = await repository.refreshRequest(requestId: 'request1');

        expect(result.id, 'request1');
        expect(result.description, 'Need camping gear');
      });
    });
  });
}
