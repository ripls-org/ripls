import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:geolocator/geolocator.dart' show Position;
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/utils/user_location_initializer.dart';
import 'package:ripls/data/repositories/location_repository.dart';
import 'package:ripls/services/device_location_service.dart';
import 'package:ripls/services/location_search_service.dart';
import 'package:ripls/services/location_service.dart';
import 'package:ripls/services/providers.dart';
import 'package:uuid/uuid.dart';

part 'location_picker_view_model.freezed.dart';

final _log = Logger('LocationPickerViewModel');

/// State for the Location Picker modal
@freezed
sealed class LocationPickerState with _$LocationPickerState {
  const factory LocationPickerState({
    Location? currentLocation,
    @Default(true) bool isLoadingLocation,
    @Default(false) bool isSaving,
    @Default(false) bool isLoadingCurrentLocation,
    UserError? error,
    // Saved locations for chip selection
    @Default([]) List<Location> savedLocations,
    Location? primaryLocation,
    @Default(true) bool isLoadingSavedLocations,
    // Current device location result (before saving)
    LocationResult? currentLocationResult,
    // Default camera position for initial map display
    double? defaultCameraLatitude,
    double? defaultCameraLongitude,
    @Default(false) bool isLoadingDefaultCamera,
    // Google Places session token bundling autocomplete + the eventual
    // Place Details call into one billable session. Minted in [build] and
    // rotated after every [resolveSuggestion]. Ignored by providers that
    // don't support session tokens (Mapbox).
    String? sessionToken,
  }) = _LocationPickerState;

  const LocationPickerState._();

  /// Whether we have a current location
  bool get hasLocation => currentLocation != null;

  
  
  /// Whether we have a primary location
  bool get hasPrimaryLocation => primaryLocation != null;
}

/// Notifier for managing the Location Picker modal state
class LocationPickerNotifier extends Notifier<LocationPickerState> {
  static const Uuid _uuid = Uuid();

  @override
  LocationPickerState build() {
    return LocationPickerState(sessionToken: _uuid.v4());
  }

  LocationRepository get _locationRepository =>
      ref.read(locationRepositoryProvider);

  /// Loads location by ID directly
  Future<void> loadLocationById(String locationId) async {
    state = state.copyWith(isLoadingLocation: true, error: null);

    try {
      // Fetch location details
      final location = await _locationRepository.getLocation(locationId);

      state = state.copyWith(
        currentLocation: location,
        isLoadingLocation: false,
      );
    } catch (e) {
      _log.severe('Failed to load location by ID: $e');
      state = state.copyWith(
        isLoadingLocation: false,
        error: RpcErrorHandler.classify(e, fallback: 'Could not load location'),
      );
    }
  }

  /// Saves a new location and returns the location ID
  Future<String> saveLocation({
    required LocationResult selectedLocation,
    required String userId,
  }) async {
    // Check mounted before starting
    if (!ref.mounted) throw Exception('ViewModel disposed before save');

    state = state.copyWith(isSaving: true, error: null);

    try {
      // If the user picked from autocomplete and tapped Save before Place
      // Details resolved (Google's lazy-load path), hydrate now so we
      // persist real coordinates rather than 0/0.
      var hydrated = selectedLocation;
      if (selectedLocation.externalPlaceId.isNotEmpty &&
          selectedLocation.latitude == 0.0 &&
          selectedLocation.longitude == 0.0) {
        final resolved = await _locationRepository.resolveSuggestion(
          selectedLocation,
          sessionToken: state.sessionToken,
        );
        if (resolved != null) {
          hydrated = resolved;
        }
      }

      // Save the location - server automatically adds it to user's list
      // (as primary if none exists, or to other locations if primary already set)
      final locationId = await _locationRepository.saveLocation(
        latitudeDeg: hydrated.latitude,
        longitudeDeg: hydrated.longitude,
        regionCode: hydrated.regionCode,
        postalCode: hydrated.postcode,
        locality: hydrated.locality,
        addressLines: hydrated.streetAddress.isNotEmpty
            ? [hydrated.streetAddress]
            : null,
        name: hydrated.name.isNotEmpty ? hydrated.name : null,
        externalPlaceId: hydrated.externalPlaceId.isNotEmpty
            ? hydrated.externalPlaceId
            : null,
        externalPlaceProvider: hydrated.externalPlaceProvider.isNotEmpty
            ? hydrated.externalPlaceProvider
            : null,
        userId: userId,
      );

      // Check mounted after async gap
      if (!ref.mounted) throw Exception('ViewModel disposed after save');

      // NOTE: No need to call saveUser() here - the server's SaveLocation RPC
      // automatically adds the location to the user's list. Calling saveUser()
      // would OVERWRITE the entire location list, defeating the automatic management.

      // Fetch the saved location details
      final savedLocation = await _locationRepository.getLocation(locationId);

      // Check mounted before updating state
      if (!ref.mounted) throw Exception('ViewModel disposed after fetch');

      state = state.copyWith(currentLocation: savedLocation, isSaving: false);

      return locationId;
    } catch (e) {
      _log.severe('Failed to save location: $e');
      if (!ref.mounted) {
        rethrow;
      }
      state = state.copyWith(
        isSaving: false,
        error: RpcErrorHandler.classify(e, fallback: 'Could not save location'),
      );
      rethrow;
    }
  }

  /// Clears any error messages
  void clearError() {
    state = state.copyWith(error: null);
  }

  /// Reverse-geocodes a GPS [Position] (already fetched with permission) and
  /// records the resulting [LocationResult] in state.
  ///
  /// Callers (typically a widget with [BuildContext]) are responsible for
  /// obtaining [position] via the permission-aware helper
  /// `requestLocationWithSettingsFallback` — this method does not prompt for
  /// permission on its own, keeping the view model free of UI/context
  /// dependencies per the MVVM boundary in `docs/client/architecture.md`.
  ///
  /// The result is not saved until the user calls [saveLocation].
  Future<void> applyDevicePosition(Position position) async {
    state = state.copyWith(
      isLoadingCurrentLocation: true,
      error: null,
    );

    try {
      final locationResult =
          await _locationRepository.reverseGeocodeViaSearchService(
        position.latitude,
        position.longitude,
      );

      if (!ref.mounted) return;

      state = state.copyWith(
        currentLocationResult: locationResult,
        isLoadingCurrentLocation: false,
      );
    } catch (e, stackTrace) {
      _log.severe('Failed to reverse-geocode device position', e, stackTrace);

      if (!ref.mounted) return;

      state = state.copyWith(
        isLoadingCurrentLocation: false,
        error: RpcErrorHandler.classify(e),
      );
    }
  }

  /// Reverse-geocodes a tapped map coordinate and returns the result for
  /// callers to consume. Used by the map-tap flow in LocationPickerModal.
  /// Returns null on no-result; throws on transport error.
  Future<LocationResult?> reverseGeocodeForLatLng(
    double latitude,
    double longitude,
  ) async {
    try {
      return await _locationRepository.reverseGeocodeViaSearchService(
        latitude,
        longitude,
      );
    } catch (e) {
      _log.warning('Map-tap reverse geocode failed: $e');
      return null;
    }
  }

  /// Searches for addresses, POIs, places, and localities matching [query].
  /// Used by the autocomplete field. Returns an empty list on error so the
  /// typeahead degrades gracefully. Threads the current session token so
  /// providers that bill per session (Google) can amortize the
  /// autocomplete cost across the eventual Place Details follow-up.
  Future<List<LocationResult>> searchAddresses(
    String query, {
    Position? proximity,
  }) async {
    try {
      return await _locationRepository.searchAddresses(
        query,
        proximity: proximity,
        sessionToken: state.sessionToken,
      );
    } catch (e) {
      _log.warning('searchAddresses failed: $e');
      return const [];
    }
  }

  /// Searches for geographic areas only (place, locality, district,
  /// region, country) matching [query]. Used by the autocomplete field.
  /// Returns an empty list on error.
  Future<List<LocationResult>> searchGeographicAreas(
    String query, {
    Position? proximity,
  }) async {
    try {
      return await _locationRepository.searchGeographicAreas(
        query,
        proximity: proximity,
        sessionToken: state.sessionToken,
      );
    } catch (e) {
      _log.warning('searchGeographicAreas failed: $e');
      return const [];
    }
  }

  /// Resolves a search [suggestion] (typically from [searchAddresses]) to
  /// a fully-populated [LocationResult] with coordinate and address
  /// components. After a successful resolve the session token is rotated
  /// so the next autocomplete + details pair forms its own session.
  ///
  /// Returns the input verbatim for providers that don't lazy-load
  /// details (Mapbox). Returns null when the upstream lookup fails.
  Future<LocationResult?> resolveSuggestion(LocationResult suggestion) async {
    try {
      final resolved = await _locationRepository.resolveSuggestion(
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

  /// Loads user's recent locations (sorted by last_used_at, most recent first)
  Future<void> loadSavedLocations() async {
    state = state.copyWith(isLoadingSavedLocations: true);

    try {
      final userRepository = ref.read(userRepositoryProvider);
      final authState = ref.read(authStateProvider);

      if (authState.user == null) {
        state = state.copyWith(isLoadingSavedLocations: false);
        return;
      }

      // Get full user details to check primary location
      final fullUser = await userRepository.get(authState.user!.id);

      Location? primary;
      if (fullUser.primaryResidenceLocationId.isNotEmpty) {
        try {
          primary = await _locationRepository.getLocation(
            fullUser.primaryResidenceLocationId,
          );
        } catch (e) {
          _log.warning('Failed to load primary location: $e');
        }
      }

      // Get recent locations sorted by last_used_at (limit to 5 for picker)
      final userLocations = await _locationRepository.getUserLocations(limit: 5);

      // Extract the location details from UserLocationWithDetails
      final locations = userLocations.map((ul) => ul.location).toList();

      if (!ref.mounted) return;

      state = state.copyWith(
        savedLocations: locations,
        primaryLocation: primary,
        isLoadingSavedLocations: false,
      );
    } catch (e, stackTrace) {
      _log.severe('Failed to load saved locations', e, stackTrace);
      if (!ref.mounted) return;

      state = state.copyWith(
        isLoadingSavedLocations: false,
        error: RpcErrorHandler.classify(e, fallback: 'Could not load saved locations'),
      );
    }
  }

  /// Initializes default camera position with smart fallback hierarchy.
  ///
  /// Fallback order:
  /// 1. User's primary residence location
  /// 2. Current GPS location (via userLocationProvider, session-cached)
  /// 3. Austin, TX (30.2672, -97.7431)
  ///
  /// Residence-first ordering is intentional: the picker asks "where does
  /// this user live?" which differs from userLocationProvider's resolver,
  /// which prefers GPS over residence ("where is the user now?").
  ///
  /// This method is called by LocationPickerModal when no initial location is provided.
  Future<void> initializeDefaultCamera() async {
    state = state.copyWith(isLoadingDefaultCamera: true);

    try {
      // Try to get user's primary residence location (cheap, side-effect-free).
      try {
        final primaryLocationId =
            await UserLocationInitializer.getPrimaryLocationIfSet(ref);

        if (!ref.mounted) return;

        if (primaryLocationId != null) {
          final primaryLocation =
              await _locationRepository.getLocation(primaryLocationId);

          if (!ref.mounted) return;

          state = state.copyWith(
            defaultCameraLatitude: primaryLocation.latitudeDeg,
            defaultCameraLongitude: primaryLocation.longitudeDeg,
            isLoadingDefaultCamera: false,
          );
          _log.info('Using primary residence for default camera position');
          return;
        }
      } catch (e) {
        _log.warning('Failed to load primary residence for default camera: $e');
        // Continue to GPS fallback
      }

      if (!ref.mounted) return;

      // No primary residence — try GPS via the session-cached provider.
      // Picker camera/pin is user-visible, so use precise intent.
      try {
        final position = await ref.read(
          userLocationProvider(LocationIntent.precisePin).future,
        );

        if (!ref.mounted) return;

        if (position != null) {
          state = state.copyWith(
            defaultCameraLatitude: position.latitude,
            defaultCameraLongitude: position.longitude,
            isLoadingDefaultCamera: false,
          );
          _log.info('Using GPS location for default camera position');
          return;
        }
      } catch (e) {
        _log.warning('Failed to get GPS location for default camera: $e');
        // Continue to Austin fallback
      }

      if (!ref.mounted) return;

      // Fallback to Austin, TX
      state = state.copyWith(
        defaultCameraLatitude: 30.2672,
        defaultCameraLongitude: -97.7431,
        isLoadingDefaultCamera: false,
      );
      _log.info('Using Austin, TX fallback for default camera position');
    } catch (e, stackTrace) {
      _log.severe('Failed to initialize default camera', e, stackTrace);
      if (!ref.mounted) return;

      // Use Austin as ultimate fallback even on error
      state = state.copyWith(
        defaultCameraLatitude: 30.2672,
        defaultCameraLongitude: -97.7431,
        isLoadingDefaultCamera: false,
        error: RpcErrorHandler.classify(e, fallback: 'Could not determine location'),
      );
    }
  }
}

/// Provider for the Location Picker modal state
final locationPickerProvider =
    NotifierProvider.autoDispose<LocationPickerNotifier, LocationPickerState>(
      LocationPickerNotifier.new,
    );
