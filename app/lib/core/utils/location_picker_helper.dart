import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart'
    show GeocodedLocation;
import 'package:ripls/presentation/widgets/location/location_picker_modal.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('LocationPickerHelper');

/// Returns true when [id] names a real saved location.
///
/// Treats both `null` and the empty string as "no saved location". Proto3
/// string fields default to `""` for unset values, so callers reading a
/// `locationId` off a proto message (e.g. an experience whose location is
/// still TBD) get `""` rather than `null`. Use this helper at every
/// presence check to avoid the trap of an `== null` test that silently
/// falls through for empty strings — see issue #1991.
bool hasSavedLocationId(String? id) => id != null && id.isNotEmpty;

/// LocationPickerHelper provides utilities for opening the location picker modal
/// with pre-loaded location data to avoid showing the Austin fallback.
class LocationPickerHelper {
  LocationPickerHelper._(); // Private constructor to prevent instantiation

  /// Opens the location picker modal with pre-loaded location data.
  ///
  /// This method pre-loads the location from the repository before opening
  /// the modal, allowing the map to display the correct location immediately
  /// instead of showing the Austin, TX fallback briefly.
  ///
  /// Parameters:
  /// - [context]: BuildContext for showing the modal
  /// - [ref]: WidgetRef for accessing providers
  /// - [locationId]: Optional location ID to pre-load
  /// - [geocodedLocation]: Optional geocoded location (from AI, not yet saved)
  /// - [checkIsOwner]: Optional callback to check if user is the owner
  /// - [showDirections]: Whether to show the directions button
  /// - [allowNonOwnerEdit]: Whether non-owners can edit the location
  ///
  /// If both locationId and geocodedLocation are provided, geocodedLocation takes priority.
  ///
  /// Returns the new location ID if changed, null otherwise.
  static Future<String?> showLocationPicker({
    required BuildContext context,
    required WidgetRef ref,
    String? locationId,
    GeocodedLocation? geocodedLocation,
    Future<bool> Function()? checkIsOwner,
    bool showDirections = true,
    bool allowNonOwnerEdit = false,
  }) async {
    _log.info('🔧 LocationPickerHelper.showLocationPicker called');
    _log.info('📍 Received locationId: $locationId');
    _log.info('📍 Received geocodedLocation: ${geocodedLocation?.name ?? "null"}');

    // Pre-load location if we have a locationId (but not if geocodedLocation is provided)
    // This allows us to pass the location synchronously instead of async loading
    GeocodedLocation? finalGeocodedLocation = geocodedLocation;

    if (finalGeocodedLocation == null && locationId != null && locationId.isNotEmpty) {
      _log.info('🔄 No geocodedLocation provided, attempting to load locationId: $locationId');

      try {
        final location = await ref
            .read(locationRepositoryProvider)
            .getLocation(locationId);
        // Convert Location to GeocodedLocation
        finalGeocodedLocation = GeocodedLocation(
          name: location.name,
          latitudeDeg: location.latitudeDeg,
          longitudeDeg: location.longitudeDeg,
          locality: location.locality,
          regionCode: location.regionCode,
          postalCode: location.postalCode,
          addressLines: location.addressLines,
        );
        _log.info('✅ Loaded location: ${location.name}');
      } catch (e) {
        _log.warning('⚠️ Failed to load locationId: $e');
        // Fall back to async loading via initialLocationId
      }
    }

    _log.info('🚀 Opening LocationPickerModal with:');
    _log.info('📍 initialGeocodedLocation: ${finalGeocodedLocation?.name ?? "null"}');
    _log.info('📍 initialLocationId (fallback): ${finalGeocodedLocation == null ? locationId : "null (have geocodedLocation)"}');

    // Check if widget is still mounted before using context
    if (!context.mounted) return null;

    // Open the modal with pre-loaded location or fallback to async loading
    return LocationPickerModal.show(
      context,
      // Prefer passing the location directly (synchronous) over locationId (async)
      initialGeocodedLocation: finalGeocodedLocation,
      // Only pass initialLocationId as fallback if we couldn't pre-load
      initialLocationId: finalGeocodedLocation == null ? locationId : null,
      checkIsOwner: checkIsOwner,
      showDirections: showDirections,
      allowNonOwnerEdit: allowNonOwnerEdit,
    );
  }
}
