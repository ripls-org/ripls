import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/location_service.pb.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/presentation/viewmodels/governance_view_model.dart';
import 'package:ripls/services/location_service.dart';
import 'package:ripls/services/mapbox_location_service.dart';
import 'package:ripls/services/providers.dart';

@GenerateMocks([CommunityRepository, LocationService])
import 'governance_view_model_test.mocks.dart';

void main() {
  group('GovernanceViewModel', () {
    late MockCommunityRepository mockCommunityRepository;
    late MockLocationService mockLocationService;
    late ProviderContainer container;

    setUp(() {
      mockCommunityRepository = MockCommunityRepository();
      mockLocationService = MockLocationService();
      container = ProviderContainer(
        overrides: [
          communityRepositoryProvider.overrideWithValue(mockCommunityRepository),
          locationServiceProvider.overrideWithValue(mockLocationService),
        ],
      );
    });

    tearDown(() {
      container.dispose();
    });

    test('initial state is empty', () {
      final state = container.read(governanceProvider);

      expect(state.communityId, isNull);
      expect(state.regions, isEmpty);
      expect(state.isLoading, isFalse);
      expect(state.error, isNull);
    });

    test('initialize loads regions successfully', () async {
      const communityId = 'community123';
      final mockRegions = [
        CommunityRegionItem(
          regionId: 'region1',
          memberPercentage: 0.75,
          isOverride: false,
        ),
        CommunityRegionItem(
          regionId: 'region2',
          memberPercentage: 1,
          isOverride: false,
        ),
      ];

      when(mockCommunityRepository.getCommunityRegions(communityId))
          .thenAnswer((_) async => mockRegions);

      // Initialize the ViewModel
      await container.read(governanceProvider.notifier).initialize(communityId);

      final state = container.read(governanceProvider);

      expect(state.communityId, communityId);
      expect(state.regions, mockRegions);
      expect(state.isLoading, isFalse);
      expect(state.error, isNull);
      verify(mockCommunityRepository.getCommunityRegions(communityId)).called(1);
    });

    test('initialize handles errors gracefully', () async {
      const communityId = 'community123';

      when(mockCommunityRepository.getCommunityRegions(communityId))
          .thenThrow(Exception('Failed to load regions'));

      await container.read(governanceProvider.notifier).initialize(communityId);

      final state = container.read(governanceProvider);

      expect(state.communityId, communityId);
      expect(state.regions, isEmpty);
      expect(state.isLoading, isFalse);
      expect(state.error, isNotNull);
    });

    test('refresh reloads regions', () async {
      const communityId = 'community123';
      final mockRegions = [
        CommunityRegionItem(
          regionId: 'region1',
          memberPercentage: 0.75,
          isOverride: false,
        ),
      ];

      when(mockCommunityRepository.getCommunityRegions(communityId))
          .thenAnswer((_) async => mockRegions);
      when(mockCommunityRepository.refreshCommunityRegions(communityId))
          .thenAnswer((_) async => mockRegions);

      // Initialize first
      await container.read(governanceProvider.notifier).initialize(communityId);

      // Then refresh
      await container.read(governanceProvider.notifier).refresh();

      final state = container.read(governanceProvider);

      expect(state.regions, mockRegions);
      expect(state.isLoading, isFalse);
      verify(mockCommunityRepository.refreshCommunityRegions(communityId))
          .called(1);
      // Called twice: once for initialize, once for refresh
      verify(mockCommunityRepository.getCommunityRegions(communityId)).called(2);
    });

    test('setRegionOverride creates region and sets override', () async {
      const communityId = 'community123';
      final mockRegions = [
        CommunityRegionItem(
          regionId: 'region1',
          memberPercentage: 0,
          isOverride: true,
        ),
      ];

      final location = LocationResult(
        name: 'Boulder, Colorado',
        fullName: 'Boulder, Colorado, United States',
        latitude: 40.015,
        longitude: -105.27,
        type: 'place',
        streetAddress: '',
        locality: 'Boulder',
        region: 'Colorado',
        regionCode: 'US',
        postcode: '80301',
      );

      final regionResponse = CreateRegionFromAddressResponse(
        regionId: 'region1',
        regionType: 'city',
        regionName: 'Boulder, CO',
      );

      when(mockLocationService.createRegionFromAddress(
        latitudeDeg: anyNamed('latitudeDeg'),
        longitudeDeg: anyNamed('longitudeDeg'),
        regionCode: anyNamed('regionCode'),
        postalCode: anyNamed('postalCode'),
        locality: anyNamed('locality'),
        neighborhood: anyNamed('neighborhood'),
        county: anyNamed('county'),
        administrativeArea: anyNamed('administrativeArea'),
        preferredRegionType: anyNamed('preferredRegionType'),
      )).thenAnswer((_) async => regionResponse);

      when(mockCommunityRepository.setCommunityRegionOverride(
        communityId: anyNamed('communityId'),
        regionId: anyNamed('regionId'),
      )).thenAnswer((_) async => {});

      when(mockCommunityRepository.getCommunityRegions(communityId))
          .thenAnswer((_) async => mockRegions);

      // Initialize first
      await container.read(governanceProvider.notifier).initialize(communityId);

      // Set override
      await container
          .read(governanceProvider.notifier)
          .setRegionOverride(location: location);

      final state = container.read(governanceProvider);

      expect(state.regions, mockRegions);
      expect(state.isLoading, isFalse);
      verify(mockLocationService.createRegionFromAddress(
        latitudeDeg: location.latitude,
        longitudeDeg: location.longitude,
        regionCode: location.regionCode,
        postalCode: location.postcode,
        locality: location.locality,
        neighborhood: '',
        county: '',
        administrativeArea: location.region,
        preferredRegionType: 'city',
      )).called(1);
      verify(mockCommunityRepository.setCommunityRegionOverride(
        communityId: communityId,
        regionId: 'region1',
      )).called(1);
    });

    test('hasError returns true when errorMessage is set', () async {
      const communityId = 'community123';

      when(mockCommunityRepository.getCommunityRegions(communityId))
          .thenThrow(Exception('Error'));

      await container.read(governanceProvider.notifier).initialize(communityId);

      final state = container.read(governanceProvider);

      expect(state.hasError, isTrue);
    });

    test('hasData returns true when regions are loaded', () async {
      const communityId = 'community123';
      final mockRegions = [
        CommunityRegionItem(
          regionId: 'region1',
          memberPercentage: 0.75,
          isOverride: false,
        ),
      ];

      when(mockCommunityRepository.getCommunityRegions(communityId))
          .thenAnswer((_) async => mockRegions);

      await container.read(governanceProvider.notifier).initialize(communityId);

      final state = container.read(governanceProvider);

      expect(state.hasData, isTrue);
    });
  });
}
