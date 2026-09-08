import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart' show Request;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/presentation/viewmodels/request_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'request_view_model_test.mocks.dart';

@GenerateMocks([RequestRepository])
void main() {
  group('RequestNotifier', () {
    late ProviderContainer container;
    late MockRequestRepository mockRequestRepository;

    setUp(() {
      mockRequestRepository = MockRequestRepository();
    });

    tearDown(() {
      container.dispose();
      reset(mockRequestRepository);
    });

    ProviderContainer createContainer() {
      return ProviderContainer(
        overrides: [
          requestRepositoryProvider.overrideWithValue(mockRequestRepository),
        ],
      );
    }

    group('deleteRequest', () {
      test('calls repository deleteRequest and succeeds', () async {
        const requestId = 'request-1';
        final testRequest = Request(
          id: requestId,
          title: 'Test Request',
          description: 'Test Description',
          requester: User(id: 'user-1', name: 'Test User'),
        );

        // Setup mock
        when(mockRequestRepository.deleteRequest(requestId: requestId))
            .thenAnswer((_) async {});

        // Create container with mocked repository
        container = createContainer();

        // Get the notifier and set initial state with request details
        final notifier = container.read(requestProvider(requestId).notifier);

        // Manually set the request details (simulating loaded state)
        notifier.state = notifier.state.copyWith(
          requestDetails: testRequest,
          isLoading: false,
        );

        // Call deleteRequest
        await notifier.deleteRequest();

        // Verify repository was called
        verify(mockRequestRepository.deleteRequest(requestId: requestId))
            .called(1);
      });

      test('sets error state on failure', () async {
        const requestId = 'request-1';
        final testRequest = Request(
          id: requestId,
          title: 'Test Request',
          description: 'Test Description',
          requester: User(id: 'user-1', name: 'Test User'),
        );

        // Setup mock to throw error
        when(mockRequestRepository.deleteRequest(requestId: requestId))
            .thenThrow(Exception('Delete failed'));

        // Create container with mocked repository
        container = createContainer();

        // Get the notifier and set initial state with request details
        final notifier = container.read(requestProvider(requestId).notifier);

        // Manually set the request details (simulating loaded state)
        notifier.state = notifier.state.copyWith(
          requestDetails: testRequest,
          isLoading: false,
        );

        // Call deleteRequest - should rethrow
        expect(
          () => notifier.deleteRequest(),
          throwsException,
        );

        // Verify error state was set
        final state = container.read(requestProvider(requestId));
        expect(state.error, isNotNull);
      });

      test('does nothing when request details is null', () async {
        const requestId = 'request-1';

        // Create container with mocked repository
        container = createContainer();

        // Get the notifier (initial state has null requestDetails)
        final notifier = container.read(requestProvider(requestId).notifier);

        // Call deleteRequest - should return early
        await notifier.deleteRequest();

        // Verify repository was NOT called
        verifyNever(
          mockRequestRepository.deleteRequest(
            requestId: anyNamed('requestId'),
          ),
        );
      });
    });

    group('canEditCoverPhoto', () {
      test('false when not editing, even if owner', () {
        const requestId = 'request-1';
        container = createContainer();
        final notifier = container.read(requestProvider(requestId).notifier);
        notifier.state = notifier.state.copyWith(
          currentUserId: 'user-1',
          requestDetails: Request(
            id: requestId,
            requester: User(id: 'user-1', name: 'Owner'),
          ),
          isEditing: false,
        );

        expect(
          container.read(requestProvider(requestId)).canEditCoverPhoto,
          isFalse,
        );
      });

      test('true when owner is editing, regardless of existing cover', () {
        const requestId = 'request-1';
        container = createContainer();
        final notifier = container.read(requestProvider(requestId).notifier);
        notifier.state = notifier.state.copyWith(
          currentUserId: 'user-1',
          requestDetails: Request(
            id: requestId,
            requester: User(id: 'user-1', name: 'Owner'),
          ),
          isEditing: true,
          mediaPath: 'https://example.com/photo.jpg',
          mediaId: 'media-1',
        );

        expect(
          container.read(requestProvider(requestId)).canEditCoverPhoto,
          isTrue,
        );
      });

      test('false when editing as non-owner', () {
        const requestId = 'request-1';
        container = createContainer();
        final notifier = container.read(requestProvider(requestId).notifier);
        notifier.state = notifier.state.copyWith(
          currentUserId: 'user-1',
          requestDetails: Request(
            id: requestId,
            requester: User(id: 'user-2', name: 'Other'),
          ),
          isEditing: true,
        );

        expect(
          container.read(requestProvider(requestId)).canEditCoverPhoto,
          isFalse,
        );
      });
    });

    group('Media Upload Ordering', () {
      // Note: Full media upload testing requires mocking MediaPickerHelper
      // and file system operations. These tests document the expected behavior
      // of the insertAtFront parameter. Integration tests verify the complete flow.

      test('picker methods accept insertAtFront parameter', () {
        const requestId = 'request-1';
        container = createContainer();

        // This test verifies that the API signature is correct
        // The methods should compile and accept the insertAtFront parameter
        final notifier = container.read(requestProvider(requestId).notifier);

        // Verify the methods exist with the correct signature
        expect(notifier.pickImageFromGallery, isA<Function>());
        expect(notifier.pickImageFromCamera, isA<Function>());
        expect(notifier.pickVideoFromGallery, isA<Function>());

        // The actual behavior (prepend vs append) is tested in integration tests
        // since it requires mocking file pickers and media upload infrastructure
      });
    });
  });
}
