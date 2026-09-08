import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:geolocator/geolocator.dart' show Position;
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/presentation/screens/communities/community_public_screen.dart';
import 'package:ripls/presentation/viewmodels/community_content_view_model.dart';
import 'package:ripls/services/location_search_service.dart';
import 'package:ripls/services/providers.dart';
import 'package:uuid/uuid.dart';

part 'governance_view_model.freezed.dart';

final _log = Logger('GovernanceViewModel');

/// State for community governance functionality
@freezed
sealed class GovernanceState with _$GovernanceState {
  const factory GovernanceState({
    String? communityId,
    @Default([]) List<CommunityRegionItem> regions,
    @Default(false) bool isLoading,
    UserError? error,
    // Google Places session token bundling autocomplete + the eventual
    // Place Details call into one billable session. Minted in [build] and
    // rotated after every [resolveSuggestion]. Ignored by providers that
    // don't support session tokens (Mapbox).
    String? sessionToken,
  }) = _GovernanceState;

  const GovernanceState._();

  /// Returns whether the state has an error
  bool get hasError => error != null;

  /// Returns whether region data is available
  bool get hasData => regions.isNotEmpty;
}

/// Notifier for managing community governance state
class GovernanceNotifier extends Notifier<GovernanceState>
    with SafeNotifierMixin<GovernanceState> {
  static const Uuid _uuid = Uuid();

  @override
  GovernanceState build() {
    return GovernanceState(sessionToken: _uuid.v4());
  }

  /// Initializes and loads regions for a community
  Future<void> initialize(String communityId) async {
    state = state.copyWith(
      communityId: communityId,
      isLoading: true,
      error: null,
    );
    await _loadRegions();
  }

  /// Loads community regions
  Future<void> _loadRegions() async {
    final communityId = state.communityId;
    if (communityId == null) return;

    _log.info('📥 Loading regions for community: $communityId');

    state = state.copyWith(isLoading: true, error: null);

    try {
      final communityRepository = ref.read(communityRepositoryProvider);
      final regions = await communityRepository.getCommunityRegions(communityId);

      safeUpdateState((s) => s.copyWith(
        regions: regions,
        isLoading: false,
        error: null,
      ));

      _log.info('✅ Regions loaded - ${regions.length} regions');
    } catch (e, stackTrace) {
      _log.severe('❌ Failed to load regions: $e', e, stackTrace);
      safeUpdateState((s) => s.copyWith(
        isLoading: false,
        error: RpcErrorHandler.classify(e),
      ));
    }
  }

  /// Refreshes region data by invalidating cache
  Future<void> refresh() async {
    final communityId = state.communityId;
    if (communityId == null) return;

    _log.info('🔄 Refreshing regions for community: $communityId');

    try {
      final communityRepository = ref.read(communityRepositoryProvider);
      await communityRepository.refreshCommunityRegions(communityId);

      // Reload data
      await _loadRegions();

      _log.info('✅ Regions refreshed');
    } catch (e, stackTrace) {
      _log.severe('❌ Failed to refresh regions: $e', e, stackTrace);
      safeUpdateState((s) => s.copyWith(error: RpcErrorHandler.classify(e)));
    }
  }

  /// Sets a manual region override for the community
  Future<void> setRegionOverride({
    required LocationResult location,
  }) async {
    final communityId = state.communityId;
    if (communityId == null) return;

    _log.info('🔧 Setting region override for community: $communityId');

    state = state.copyWith(isLoading: true, error: null);

    try {
      final communityRepository = ref.read(communityRepositoryProvider);
      final locationService = ref.read(locationServiceProvider);

      // Step 1: Create region from Mapbox place details
      final regionResponse = await locationService.createRegionFromAddress(
        latitudeDeg: location.latitude,
        longitudeDeg: location.longitude,
        regionCode: location.regionCode,
        postalCode: location.postcode,
        locality: location.locality,
        neighborhood: '',
        county: '',
        administrativeArea: location.region,
        preferredRegionType: _determineRegionType(location.type),
      );

      _log.info('✅ Created region: ${regionResponse.regionId} (${regionResponse.regionType})');

      // Step 2: Set community region override
      await communityRepository.setCommunityRegionOverride(
        communityId: communityId,
        regionId: regionResponse.regionId,
      );

      _log.info('✅ Region override set successfully');

      // Step 3: Invalidate related providers so they refetch with updated data
      ref.invalidate(getCommunityProvider(communityId));
      ref.invalidate(communityRegionsProvider(communityId));

      // Step 4: Reload regions to reflect changes
      await _loadRegions();
    } catch (e, stackTrace) {
      _log.severe('❌ Failed to set region override: $e', e, stackTrace);
      safeUpdateState((s) => s.copyWith(
        error: RpcErrorHandler.classify(e),
        isLoading: false,
      ));
      rethrow;
    }
  }

  /// Determines region type from the provider's feature-type taxonomy
  /// (Mapbox today: neighborhood/place/locality/district/region).
  String _determineRegionType(String featureType) {
    switch (featureType) {
      case 'neighborhood':
        return 'neighborhood';
      case 'locality':
      case 'place':
        return 'city';
      case 'district':
        return 'county';
      case 'region':
        return 'state';
      default:
        return 'city';
    }
  }

  /// Searches for geographic areas matching [query]. Used by the
  /// region-override autocomplete field on the governance screen.
  /// Returns an empty list on error so the typeahead degrades
  /// gracefully.
  Future<List<LocationResult>> searchGeographicAreas(
    String query, {
    Position? proximity,
  }) async {
    try {
      final repo = ref.read(locationRepositoryProvider);
      return await repo.searchGeographicAreas(
        query,
        proximity: proximity,
        sessionToken: state.sessionToken,
      );
    } catch (e) {
      _log.warning('searchGeographicAreas failed: $e');
      return const [];
    }
  }

  /// Searches for addresses, POIs, places, and localities matching
  /// [query]. Returns an empty list on error.
  Future<List<LocationResult>> searchAddresses(
    String query, {
    Position? proximity,
  }) async {
    try {
      final repo = ref.read(locationRepositoryProvider);
      return await repo.searchAddresses(
        query,
        proximity: proximity,
        sessionToken: state.sessionToken,
      );
    } catch (e) {
      _log.warning('searchAddresses failed: $e');
      return const [];
    }
  }

  /// Resolves a search [suggestion] (from [searchAddresses] /
  /// [searchGeographicAreas]) to a fully-populated [LocationResult].
  /// Rotates the session token on success so the next search starts a
  /// fresh billing session.
  Future<LocationResult?> resolveSuggestion(LocationResult suggestion) async {
    try {
      final repo = ref.read(locationRepositoryProvider);
      final resolved = await repo.resolveSuggestion(
        suggestion,
        sessionToken: state.sessionToken,
      );
      if (!ref.mounted) {
        return resolved;
      }
      state = state.copyWith(sessionToken: _uuid.v4());
      return resolved;
    } catch (e) {
      _log.warning('resolveSuggestion failed: $e');
      return null;
    }
  }
}

/// Provider for community governance view model
final governanceProvider =
    NotifierProvider.autoDispose<GovernanceNotifier, GovernanceState>(
      GovernanceNotifier.new,
    );
