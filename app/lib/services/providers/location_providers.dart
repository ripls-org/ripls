import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/config/environment.dart';
import 'package:ripls/data/repositories/location_repository.dart';
import 'package:ripls/services/google_location_search_service.dart';
import 'package:ripls/services/location_search_service.dart';
import 'package:ripls/services/location_service.dart';
import 'package:ripls/services/mapbox_location_service.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/cache_providers.dart';

/// Provider for LocationService (RPC client for server-side location storage).
final locationServiceProvider = Provider<LocationService>((ref) {
  return LocationService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    onUnauthenticated: () async {
      // Clear auth state - the router will automatically redirect to login
      // and preserve the current location in the 'from' query parameter
      await ref.read(authStateProvider.notifier).logout();
    },
  );
});

/// Provider for LocationSearchService (geocoding + place-search against a
/// third-party provider). Selected at compile time via
/// `Environment.mapProvider` (`MAP_PROVIDER` --dart-define): 'mapbox'
/// (default) or 'google' (#2188).
///
/// Consumed only by LocationRepository — viewmodels and widgets access
/// search via repository methods, not via this provider directly
/// (`docs/client/architecture.md`).
final locationSearchServiceProvider = Provider<LocationSearchService>((ref) {
  switch (Environment.mapProvider) {
    case 'google':
      return GoogleLocationSearchService();
    case 'mapbox':
    default:
      return MapboxLocationService();
  }
});

/// Provider for LocationRepository
final locationRepositoryProvider = Provider<LocationRepository>((ref) {
  final cache = ref.watch(cacheManagerProvider);
  final service = ref.watch(locationServiceProvider);
  final searchService = ref.watch(locationSearchServiceProvider);
  return LocationRepository(cache, service, searchService);
});

/// Resolves a locationId to a short display string (street > locality +
/// region > region > location name). Returns null when the location has
/// no usable display fields.
final locationDisplayProvider =
    FutureProvider.autoDispose.family<String?, String>((ref, locationId) async {
  final repo = ref.watch(locationRepositoryProvider);
  final loc = await repo.getLocation(locationId);
  if (loc.addressLines.isNotEmpty) return loc.addressLines.first;
  if (loc.locality.isNotEmpty) {
    return loc.regionCode.isNotEmpty
        ? '${loc.locality}, ${loc.regionCode}'
        : loc.locality;
  }
  if (loc.regionCode.isNotEmpty) return loc.regionCode;
  if (loc.name.isNotEmpty) return loc.name;
  return null;
});
