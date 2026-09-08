import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/core/observability/events.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart' show Request;
import 'package:ripls/data/repositories/location_repository.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/presentation/viewmodels/gen_request_view_model.dart';
import 'package:ripls/services/providers.dart';

import '../../core/observability/analytics_test_helper.dart';
import 'gen_request_view_model_test.mocks.dart';

@GenerateMocks([RequestRepository, MediaRepository, LocationRepository])
void main() {
  late MockRequestRepository mockRequestRepository;
  late MockMediaRepository mockMediaRepository;
  late MockLocationRepository mockLocationRepository;
  late MockObservabilityService mockObservability;
  late ProviderContainer container;

  setUp(() {
    mockRequestRepository = MockRequestRepository();
    mockMediaRepository = MockMediaRepository();
    mockLocationRepository = MockLocationRepository();
    mockObservability = MockObservabilityService();

    // SubmitRequest no longer takes a community (#2529); the viewmodel shares
    // the new request into the selected communities via shareRequest. Stub it
    // so the share step in submitRequest tests doesn't hit MissingStubError.
    when(mockRequestRepository.shareRequest(
      requestId: anyNamed('requestId'),
      communityIds: anyNamed('communityIds'),
    )).thenAnswer((_) async => Request());

    container = ProviderContainer(
      overrides: [
        requestRepositoryProvider.overrideWithValue(mockRequestRepository),
        mediaRepositoryProvider.overrideWithValue(mockMediaRepository),
        locationRepositoryProvider.overrideWithValue(mockLocationRepository),
        observabilityServiceProvider.overrideWithValue(mockObservability),
      ],
    );
  });

  tearDown(() {
    container.dispose();
  });

  group('GenRequestState', () {
    test('initial state is correct', () {
      final state = container.read(genRequestProvider);

      expect(state.currentStep, GenRequestStep.prompt);
      expect(state.completedSteps, isEmpty);
      expect(state.generatedTitle, isNull);
      expect(state.generatedDescription, isNull);
      expect(state.isLoading, isFalse);
    });
  });

  group('submitRequest', () {
    test('successfully submits request and logs analytics', () async {
      const testCommunityId = 'community-123';
      const testRequestId = 'request-456';
      const testLocationId = 'location-789';

      when(mockRequestRepository.submitRequest(
        title: anyNamed('title'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
        locationId: anyNamed('locationId'),
      )).thenAnswer((_) async => testRequestId);

      final notifier = container.read(genRequestProvider.notifier);

      // Set up state with title and description
      notifier.state = notifier.state.copyWith(
        generatedTitle: 'Test Request Title',
        generatedDescription: 'Test request description',
        selectedLocationId: testLocationId,
        currentStep: GenRequestStep.preview,
      );

      await notifier.submitRequest([testCommunityId]);

      final state = container.read(genRequestProvider);
      expect(state.createdRequestId, testRequestId);
      expect(state.currentStep, GenRequestStep.completed);
      expect(state.isLoading, isFalse);

      verify(mockRequestRepository.submitRequest(
        title: 'Test Request Title',
        description: 'Test request description',
        mediaIds: null,
        locationId: testLocationId,
      )).called(1);

      // The selected community is now shared via shareRequest (#2529).
      verify(mockRequestRepository.shareRequest(
        requestId: testRequestId,
        communityIds: [testCommunityId],
      )).called(1);

      // Verify analytics event was logged
      expect(mockObservability.hasEventOfType<RequestCreatedEvent>(), isTrue);
      final event = mockObservability.lastEventOfType<RequestCreatedEvent>();
      expect(event?.parameters['community_id'], testCommunityId);
      expect(event?.parameters['has_location'], isTrue);
    });

    test('logs analytics with hasLocation false when no location', () async {
      const testCommunityId = 'community-123';
      const testRequestId = 'request-456';

      when(mockRequestRepository.submitRequest(
        title: anyNamed('title'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
        locationId: anyNamed('locationId'),
      )).thenAnswer((_) async => testRequestId);

      final notifier = container.read(genRequestProvider.notifier);

      // Set up state without location
      notifier.state = notifier.state.copyWith(
        generatedTitle: 'Test Request Title',
        generatedDescription: 'Test request description',
        selectedLocationId: null,
        currentStep: GenRequestStep.preview,
      );

      await notifier.submitRequest([testCommunityId]);

      // Verify analytics event with hasLocation = false
      expect(mockObservability.hasEventOfType<RequestCreatedEvent>(), isTrue);
      final event = mockObservability.lastEventOfType<RequestCreatedEvent>();
      expect(event?.parameters['has_location'], isFalse);
    });

    test('does not log analytics on submission error', () async {
      const testCommunityId = 'community-123';

      when(mockRequestRepository.submitRequest(
        title: anyNamed('title'),
        description: anyNamed('description'),
        mediaIds: anyNamed('mediaIds'),
        locationId: anyNamed('locationId'),
      )).thenThrow(Exception('Network error'));

      final notifier = container.read(genRequestProvider.notifier);

      notifier.state = notifier.state.copyWith(
        generatedTitle: 'Test Request Title',
        generatedDescription: 'Test request description',
        currentStep: GenRequestStep.preview,
      );

      await notifier.submitRequest([testCommunityId]);

      final state = container.read(genRequestProvider);
      expect(state.hasError, isTrue);
      expect(state.error, isNotNull);

      // Analytics should NOT be logged on error
      expect(mockObservability.hasEventOfType<RequestCreatedEvent>(), isFalse);
    });

    test('returns early with error when title is empty', () async {
      final notifier = container.read(genRequestProvider.notifier);

      notifier.state = notifier.state.copyWith(
        generatedTitle: '',
        generatedDescription: 'Test description',
        currentStep: GenRequestStep.preview,
      );

      await notifier.submitRequest(['community-123']);

      final state = container.read(genRequestProvider);
      expect(state.hasError, isTrue);
      expect(state.error, isNotNull);

      // Analytics should NOT be logged
      expect(mockObservability.hasEventOfType<RequestCreatedEvent>(), isFalse);
    });
  });

  // Regression coverage for #1961: the request preview's location row used
  // to mirror state into a local field that drifted from the ViewModel.
  // updateLocation now stages selectedLocationId synchronously, then resolves
  // the Location proto into state.resolvedLocation in a second copyWith.
  group('updateLocation', () {
    Location buildLocation(String id, String name) {
      return Location()
        ..id = id
        ..name = name;
    }

    test('stages selectedLocationId immediately, then resolves Location',
        () async {
      final completer = Completer<Location>();
      when(mockLocationRepository.getLocation('loc-A'))
          .thenAnswer((_) => completer.future);

      final notifier = container.read(genRequestProvider.notifier);
      // Seed a prior AI suggestion so we can confirm it's cleared on pick.
      notifier.state = notifier.state.copyWith(
        geocodedLocation: GeocodedLocation()..name = 'Central Park',
      );

      final pending = notifier.updateLocation('loc-A');

      // First copyWith: selectedLocationId set, resolvedLocation cleared,
      // geocodedLocation cleared.
      var state = container.read(genRequestProvider);
      expect(state.selectedLocationId, 'loc-A');
      expect(state.resolvedLocation, isNull);
      expect(state.geocodedLocation, isNull);

      completer.complete(buildLocation('loc-A', 'Times Square'));
      await pending;

      state = container.read(genRequestProvider);
      expect(state.selectedLocationId, 'loc-A');
      expect(state.resolvedLocation, isNotNull);
      expect(state.resolvedLocation!.id, 'loc-A');
      expect(state.resolvedLocation!.name, 'Times Square');
    });

    test('replaces resolvedLocation on a second pick', () async {
      when(mockLocationRepository.getLocation('loc-A'))
          .thenAnswer((_) async => buildLocation('loc-A', 'First'));
      when(mockLocationRepository.getLocation('loc-B'))
          .thenAnswer((_) async => buildLocation('loc-B', 'Second'));

      final notifier = container.read(genRequestProvider.notifier);
      await notifier.updateLocation('loc-A');
      expect(container.read(genRequestProvider).resolvedLocation!.id, 'loc-A');

      await notifier.updateLocation('loc-B');
      final state = container.read(genRequestProvider);
      expect(state.selectedLocationId, 'loc-B');
      expect(state.resolvedLocation!.id, 'loc-B');
      expect(state.resolvedLocation!.name, 'Second');
    });

    test('ignores stale fetch when the user picks again mid-flight',
        () async {
      final firstFetch = Completer<Location>();
      when(mockLocationRepository.getLocation('loc-A'))
          .thenAnswer((_) => firstFetch.future);
      when(mockLocationRepository.getLocation('loc-B'))
          .thenAnswer((_) async => buildLocation('loc-B', 'Second'));

      final notifier = container.read(genRequestProvider.notifier);

      // Start first pick (fetch hangs).
      final firstCall = notifier.updateLocation('loc-A');
      // User immediately picks something else; this completes its fetch.
      await notifier.updateLocation('loc-B');

      // Now the stale fetch resolves — it must NOT clobber loc-B.
      firstFetch.complete(buildLocation('loc-A', 'First'));
      await firstCall;

      final state = container.read(genRequestProvider);
      expect(state.selectedLocationId, 'loc-B');
      expect(state.resolvedLocation!.id, 'loc-B');
    });

    test('preserves selection if the repository fetch fails', () async {
      when(mockLocationRepository.getLocation('loc-A'))
          .thenThrow(Exception('network down'));

      final notifier = container.read(genRequestProvider.notifier);
      await notifier.updateLocation('loc-A');

      final state = container.read(genRequestProvider);
      // ID stands so the row can show a loading affordance and the request
      // submission still has the locationId to send. resolvedLocation
      // remains null — the widget falls back to its loading text.
      expect(state.selectedLocationId, 'loc-A');
      expect(state.resolvedLocation, isNull);
    });

    test('handles disposal mid-fetch gracefully', () async {
      final completer = Completer<Location>();
      when(mockLocationRepository.getLocation('loc-A'))
          .thenAnswer((_) => completer.future);

      final notifier = container.read(genRequestProvider.notifier);
      final pending = notifier.updateLocation('loc-A');

      // Dispose before the fetch resolves; subsequent state assignment must
      // not throw (architecture.md §"Disposal Testing").
      container.dispose();
      completer.complete(buildLocation('loc-A', 'Times Square'));

      await expectLater(pending, completes);
    });
  });

  group('ensureLocationResolved', () {
    Location buildLocation(String id, String name) {
      return Location()
        ..id = id
        ..name = name;
    }

    test('is a no-op when resolvedLocation is already set', () async {
      final notifier = container.read(genRequestProvider.notifier);
      notifier.state = notifier.state.copyWith(
        selectedLocationId: 'loc-A',
        resolvedLocation: buildLocation('loc-A', 'Cached'),
      );

      await notifier.ensureLocationResolved();

      verifyNever(mockLocationRepository.getLocation(any));
    });

    test('resolves selectedLocationId when present', () async {
      when(mockLocationRepository.getLocation('loc-A'))
          .thenAnswer((_) async => buildLocation('loc-A', 'Resolved'));

      final notifier = container.read(genRequestProvider.notifier);
      notifier.state = notifier.state.copyWith(selectedLocationId: 'loc-A');

      await notifier.ensureLocationResolved();

      expect(
        container.read(genRequestProvider).resolvedLocation!.id,
        'loc-A',
      );
    });

    test('falls back to generatedLocationId when no selection', () async {
      when(mockLocationRepository.getLocation('loc-G'))
          .thenAnswer((_) async => buildLocation('loc-G', 'Home'));

      final notifier = container.read(genRequestProvider.notifier);
      notifier.state = notifier.state.copyWith(generatedLocationId: 'loc-G');

      await notifier.ensureLocationResolved();

      expect(
        container.read(genRequestProvider).resolvedLocation!.id,
        'loc-G',
      );
    });

    test('does nothing when no location IDs are set', () async {
      final notifier = container.read(genRequestProvider.notifier);
      await notifier.ensureLocationResolved();

      verifyNever(mockLocationRepository.getLocation(any));
      expect(container.read(genRequestProvider).resolvedLocation, isNull);
    });
  });
}
