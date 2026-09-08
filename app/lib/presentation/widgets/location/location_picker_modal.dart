import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:geolocator/geolocator.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/location_formatter.dart';
import 'package:ripls/core/utils/location_permission_helper.dart';
import 'package:ripls/core/utils/location_picker_helper.dart' show hasSavedLocationId;
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart'
    show Location, GeocodedLocation;
import 'package:ripls/presentation/viewmodels/location_picker_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/location/location_autocomplete_field.dart';
import 'package:ripls/presentation/widgets/location/location_modal_widgets.dart';
import 'package:ripls/presentation/widgets/location/location_picker_helpers.dart';
import 'package:ripls/presentation/widgets/modal/chip_data.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_footer_buttons.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_sheet.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_surface.dart';
import 'package:ripls/services/location_search_service.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('LocationPickerModal');

/// LocationPickerView is a domain-agnostic, surface-agnostic location picker
/// for selecting or editing a location: header + search + saved-location chips
/// + a tap/drag map. It renders its own content (no sheet chrome) so it can be
/// embedded inline on a screen *or* wrapped in a bottom sheet via
/// [LocationPickerModal.show].
///
/// Accepts either:
/// - initialLocationId: for saved locations (e.g., from request modal)
/// - initialGeocodedLocation: for AI-geocoded locations not yet saved (e.g., from experience modal)
///
/// Reports the saved location ID via the [onSaved] callback (instead of
/// `Navigator.pop`) so inline hosts can act on it without a route.
class LocationPickerView extends ConsumerStatefulWidget {
  final String? initialLocationId;
  final GeocodedLocation? initialGeocodedLocation;
  final Future<bool> Function()? checkIsOwner;
  final bool showDirections;
  final bool allowNonOwnerEdit;

  /// Called with the saved location id when the user taps Save (or keeps an
  /// unchanged existing location). The host decides what to do with it — pop a
  /// modal, propose a poll spot, set the confirmed location, etc.
  final ValueChanged<String> onSaved;

  const LocationPickerView({
    super.key,
    required this.onSaved,
    this.initialLocationId,
    this.initialGeocodedLocation,
    this.checkIsOwner,
    this.showDirections = true,
    this.allowNonOwnerEdit = true,
  });

  @override
  ConsumerState<LocationPickerView> createState() =>
      _LocationPickerViewState();
}

/// Bottom-sheet entry point for [LocationPickerView] — the canonical glass
/// modal used across creation/edit flows. Returns the saved location id via
/// `Navigator.pop`. Inline hosts (e.g. the experience location panel) embed
/// [LocationPickerView] directly instead of going through this.
class LocationPickerModal {
  const LocationPickerModal._();

  static Future<String?> show(
    BuildContext context, {
    String? initialLocationId,
    GeocodedLocation? initialGeocodedLocation,
    Future<bool> Function()? checkIsOwner,
    bool showDirections = true,
    bool allowNonOwnerEdit = true,
  }) {
    // Canonical glass-modal entry pattern. barrierColor is the system
    // modal barrier (rendered outside the sheet's animation hierarchy)
    // so the dark scrim stays put when the sheet is dragged.
    return showAccessibleModal<String>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (sheetContext) => GlassSheet(
        padding: const EdgeInsets.fromLTRB(0, 8, 0, 24),
        child: LocationPickerView(
          initialLocationId: initialLocationId,
          initialGeocodedLocation: initialGeocodedLocation,
          checkIsOwner: checkIsOwner,
          showDirections: showDirections,
          allowNonOwnerEdit: allowNonOwnerEdit,
          onSaved: (id) => Navigator.of(sheetContext).pop(id),
        ),
      ),
    );
  }
}

class _LocationPickerViewState extends ConsumerState<LocationPickerView> {
  late TextEditingController _locationController;
  LocationResult? _selectedLocation;
  String? _selectedChipId; // Track which chip was explicitly selected by user
  bool _isOwner = false;
  bool _isSaving = false; // Guard against concurrent save operations
  bool _hasUserMadeChanges = false; // Track if user has made any changes
  double? _mapCenterLat; // Track map center for search proximity bias
  double? _mapCenterLng;

  @override
  void initState() {
    super.initState();
    _locationController = TextEditingController();

    // Priority 1: Set initialGeocodedLocation synchronously (before first build)
    // This ensures the map shows the correct location immediately
    if (widget.initialGeocodedLocation != null) {
      // Determine the name field with priority: place name → street address → locality
      String nameField = 'Location';
      if (widget.initialGeocodedLocation!.name.isNotEmpty) {
        nameField = widget.initialGeocodedLocation!.name;
      } else if (widget.initialGeocodedLocation!.addressLines.isNotEmpty) {
        nameField = widget.initialGeocodedLocation!.addressLines.first;
      } else if (widget.initialGeocodedLocation!.locality.isNotEmpty) {
        nameField = widget.initialGeocodedLocation!.locality;
      }

      _selectedLocation = LocationResult(
        name: nameField,
        fullName: formatGeocodedLocationDetailed(
          widget.initialGeocodedLocation!,
        ),
        type: 'place',
        latitude: widget.initialGeocodedLocation!.latitudeDeg,
        longitude: widget.initialGeocodedLocation!.longitudeDeg,
        streetAddress: widget.initialGeocodedLocation!.addressLines.isNotEmpty
            ? widget.initialGeocodedLocation!.addressLines.first
            : '',
        locality: widget.initialGeocodedLocation!.locality,
        region: '',
        postcode: widget.initialGeocodedLocation!.postalCode,
        regionCode: widget.initialGeocodedLocation!.regionCode,
      );

      _isOwner = widget.allowNonOwnerEdit;
    }

    WidgetsBinding.instance.addPostFrameCallback((_) async {
      unawaited(ref.read(locationPickerProvider.notifier).loadSavedLocations());

      // Initialize default camera position if no initial location provided.
      // This ensures the map shows the user's area instead of the Mapbox
      // global default (downtown Austin). Treat an empty initialLocationId
      // the same as null — proto3's default for unset IDs (e.g. an
      // experience whose location is TBD) is "", not null. See #1991.
      if (!hasSavedLocationId(widget.initialLocationId) &&
          widget.initialGeocodedLocation == null) {
        unawaited(
          ref.read(locationPickerProvider.notifier).initializeDefaultCamera(),
        );
      }

      // Priority 2: Load saved location by ID (async)
      if (hasSavedLocationId(widget.initialLocationId)) {
        await ref
            .read(locationPickerProvider.notifier)
            .loadLocationById(widget.initialLocationId!);

        // Pre-select the loaded location (but don't fill the search input)
        if (mounted) {
          final state = ref.read(locationPickerProvider);

          if (state.currentLocation != null) {
            setState(() {
              // Determine the name field with priority: place name → street address → locality
              String nameField = 'Location';
              if (state.currentLocation!.name.isNotEmpty) {
                nameField = state.currentLocation!.name;
              } else if (state.currentLocation!.addressLines.isNotEmpty) {
                nameField = state.currentLocation!.addressLines.first;
              } else if (state.currentLocation!.locality.isNotEmpty) {
                nameField = state.currentLocation!.locality;
              }

              _selectedLocation = LocationResult(
                name: nameField,
                // Use formatLocationDetailed for consistency with Mapbox's full_address
                fullName: LocationFormatter.formatLocationDetailed(
                  state.currentLocation!,
                ),
                type: 'place',
                latitude: state.currentLocation!.latitudeDeg,
                longitude: state.currentLocation!.longitudeDeg,
                streetAddress: state.currentLocation!.addressLines.isNotEmpty
                    ? state.currentLocation!.addressLines.first
                    : '',
                locality: state.currentLocation!.locality,
                region: '',
                postcode: state.currentLocation!.postalCode,
                regionCode: state.currentLocation!.regionCode,
              );
              // Don't prefill search input - keep it clean for typing
              // The selected location is shown in the header and chips
            });
          }
        }

        // Check ownership for initialLocationId path
        if (widget.checkIsOwner != null) {
          try {
            final isOwner = await widget.checkIsOwner!();
            if (mounted) {
              setState(() {
                _isOwner = isOwner;
              });
            }
          } catch (e) {
            if (mounted) {
              setState(() {
                _isOwner = false;
              });
            }
          }
        } else {
          _isOwner = widget.allowNonOwnerEdit;
        }
      }
      // Note: initialGeocodedLocation is now handled synchronously in initState()
      // For both initialGeocodedLocation and no-initial-location paths,
      // check ownership if callback is provided. The Priority-2 branch
      // above handles its own ownership check; this branch covers every
      // other case (no saved location, or an inline geocoded location).
      if (!hasSavedLocationId(widget.initialLocationId) ||
          widget.initialGeocodedLocation != null) {
        if (widget.checkIsOwner != null) {
          try {
            final isOwner = await widget.checkIsOwner!();
            if (mounted) {
              setState(() {
                _isOwner = isOwner;
              });
            }
          } catch (e) {
            if (mounted) {
              setState(() {
                _isOwner = false;
              });
            }
          }
        } else if (widget.initialGeocodedLocation == null) {
          // Only set default for no-initial-location case
          if (mounted) {
            setState(() {
              _isOwner = widget.allowNonOwnerEdit;
            });
          }
        }
      }
    });
  }

  @override
  void dispose() {
    _locationController.dispose();
    super.dispose();
  }

  Future<void> _onLocationSelected(LocationResult locationResult) async {
    // Show the picked suggestion immediately (Google Autocomplete returns
    // only displayName + placeId — no coords yet), then resolve to a
    // hydrated LocationResult and update once Place Details returns.
    // Mapbox returns full results inline, so resolveSuggestion is a
    // no-op pass-through there.
    setState(() {
      _selectedLocation = locationResult;
      _selectedChipId = null;
      _locationController.clear();
      _hasUserMadeChanges = true;
    });

    final resolved =
        await ref.read(locationPickerProvider.notifier).resolveSuggestion(
              locationResult,
            );
    if (!mounted || resolved == null) {
      return;
    }
    // Guard against a second pick racing with the first resolve — only
    // apply the hydration if the user hasn't picked something else in
    // the meantime.
    if (_selectedLocation?.externalPlaceId != locationResult.externalPlaceId) {
      return;
    }
    setState(() {
      _selectedLocation = resolved;
    });
  }

  void _onMapCameraChanged(double lat, double lng) {
    setState(() {
      _mapCenterLat = lat;
      _mapCenterLng = lng;
    });
  }

  void _onSavedLocationSelected(Location? location) {
    setState(() {
      if (location == null) {
        _selectedLocation = null;
        _selectedChipId = null;
        _locationController.clear();
      } else {
        // Use location.name if available, otherwise fall back to locality
        final displayName = location.name.isNotEmpty
            ? location.name
            : (location.locality.isNotEmpty ? location.locality : 'Location');

        _selectedLocation = LocationResult(
          name: displayName,
          // Use formatLocationDetailed for consistency with Mapbox's full_address
          fullName: LocationFormatter.formatLocationDetailed(location),
          type: 'place',
          latitude: location.latitudeDeg,
          longitude: location.longitudeDeg,
          streetAddress: location.addressLines.isNotEmpty
              ? location.addressLines.first
              : '',
          locality: location.locality,
          region: '',
          postcode: location.postalCode,
          regionCode: location.regionCode,
        );
        // Track which chip was selected
        _selectedChipId = location.id;
        // Clear search input after selection - location shows in header and chips
        _locationController.clear();
        // Mark that user has made changes
        _hasUserMadeChanges = true;
      }
    });
  }

  /// Lazy-load current location when user taps the "Use Current Location" chip
  Future<void> _useCurrentLocation() async {
    // Set chip as selected while loading
    setState(() {
      _selectedChipId = 'USE_CURRENT_LOCATION';
    });

    try {
      // Prompt for location permission if needed; the helper shows the
      // "Open Settings" dialog on a permanent denial so the user always has
      // a path forward.
      final position = await requestLocationWithSettingsFallback(context, ref);
      if (!mounted) return;
      if (position == null) {
        setState(() {
          _selectedChipId = null;
        });
        return;
      }

      // Hand the resolved position to the view model for reverse-geocoding.
      await ref
          .read(locationPickerProvider.notifier)
          .applyDevicePosition(position);

      // Check if successful
      final state = ref.read(locationPickerProvider);

      if (!mounted) return;

      if (state.error != null) {
        // Error occurred
        setState(() {
          _selectedChipId = null;
        });
        ToastHelper.showError(
          context,
          RpcErrorHandler.localize(state.error!, context.l10n),
        );
      } else if (state.currentLocationResult != null) {
        // Success - update local state
        setState(() {
          _selectedLocation = state.currentLocationResult;
          _locationController.text = state.currentLocationResult!.fullName;
          // Mark that user has made changes
          _hasUserMadeChanges = true;
        });
      } else {
        // Unexpected state
        setState(() {
          _selectedChipId = null;
        });
        ToastHelper.showError(context, 'Unable to get your location.');
      }
    } catch (e) {
      _log.severe('Error getting current location', e);
      if (mounted) {
        setState(() {
          _selectedChipId = null;
        });
        ToastHelper.showError(context, 'Error getting current location: $e');
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(locationPickerProvider);
    final canEdit = _isOwner || widget.allowNonOwnerEdit;
    final shouldEnableSave = canEdit && _hasUserMadeChanges;

    // Layout matches docs/cowork/App Design/where-modal-standalone.html:
    // header → search → chips → map band (edge-to-edge) → footer.
    // Horizontal padding lives on each non-edge child so the map and chip
    // carousel can bleed to the host's edges. The host supplies the surface
    // (GlassSheet for the modal, the panel scrim for the inline embed).
    return Tappable(
      semanticsLabel: context.l10n.a11yMiscDismissKeyboard,
      onTap: () => FocusScope.of(context).unfocus(),
      excludeChildSemantics: false,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
            child: _buildHeader(state),
          ),
          if (canEdit) ...[
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 20),
              child: _buildSearchField(),
            ),
            const SizedBox(height: 12),
            _buildChipsRow(state),
            const SizedBox(height: 12),
          ],
          _buildMapBand(state),
          if (canEdit || widget.showDirections) ...[
            const SizedBox(height: 16),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 20),
              child: _buildFooterRow(state, shouldEnableSave, canEdit),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildHeader(LocationPickerState state) {
    final display = _resolveAddressDisplay(state);
    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        Container(
          width: 40,
          height: 40,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: AppColors.modalChipBackground,
            border: Border.all(color: AppColors.modalChipBorder),
          ),
          child: const Icon(
            Icons.place,
            size: 18,
            color: AppColors.modalTextSecondary,
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                'WHERE',
                style: TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.w700,
                  letterSpacing: 1.6,
                  color: AppColors.modalTextMuted,
                ),
              ),
              const SizedBox(height: 2),
              Text(
                display.title,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  fontSize: 17,
                  fontWeight: FontWeight.w600,
                  height: 1.15,
                  color: AppColors.modalTextPrimary,
                ),
              ),
              if (display.sub != null) ...[
                const SizedBox(height: 2),
                Text(
                  display.sub!,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w500,
                    height: 1.2,
                    color: AppColors.modalTextSecondary,
                  ),
                ),
              ],
            ],
          ),
        ),
      ],
    );
  }

  Widget _buildSearchField() {
    return GlassSurface(
      fill: AppColors.modalChipBackground,
      border: AppColors.modalChipBorder,
      borderRadius: BorderRadius.circular(16),
      child: LocationAutocompleteField(
        controller: _locationController,
        onLocationSelected: _onLocationSelected,
        search: ref.read(locationPickerProvider.notifier).searchAddresses,
        hintText: 'Search for an address or place...',
        proximityBias: _mapCenterLat != null && _mapCenterLng != null
            ? Position(
                latitude: _mapCenterLat!,
                longitude: _mapCenterLng!,
                timestamp: DateTime.now(),
                accuracy: 0,
                altitude: 0,
                altitudeAccuracy: 0,
                heading: 0,
                headingAccuracy: 0,
                speed: 0,
                speedAccuracy: 0,
              )
            : null,
      ),
    );
  }

  /// Edge-to-edge map band sandwiched between the chips and the footer,
  /// per the where-modal design — opaque map in the middle of the sheet
  /// with thin top + bottom borders separating it from the glass chrome.
  Widget _buildMapBand(LocationPickerState state) {
    return Container(
      height: 320,
      decoration: BoxDecoration(
        border: Border(
          top: BorderSide(color: AppColors.modalChipBorder),
        ),
      ),
      child: _buildMapPreview(state),
    );
  }

  /// Footer using the canonical [GlassFooterButtons] primitive. Edit mode
  /// shows Reset (secondary) + Save (primary). Read-only viewers with
  /// [LocationPickerModal.showDirections] get a single Get Directions
  /// primary instead.
  Widget _buildFooterRow(
    LocationPickerState state,
    bool enableSaveButton,
    bool canEdit,
  ) {
    if (!canEdit && widget.showDirections) {
      return GlassFooterButtons(
        showSecondary: false,
        primaryEnabled: true,
        primaryLabel: context.l10n.locationPickerGetDirections,
        onPrimary: () => openDirections(context, state.currentLocation),
      );
    }
    return GlassFooterButtons(
      primaryEnabled: enableSaveButton,
      primaryLabel: context.l10n.commonSave,
      onPrimary: enableSaveButton ? _handleSave : null,
      secondaryLabel: context.l10n.commonReset,
      secondaryEnabled: enableSaveButton,
      onSecondary: enableSaveButton ? _handleReset : null,
    );
  }

  /// Resolves the header's title + optional subtitle from the current
  /// selection. Falls back to the saved location, then a placeholder.
  ({String title, String? sub}) _resolveAddressDisplay(
      LocationPickerState state) {
    if (_selectedLocation != null) {
      final detailed = LocationFormatter.formatLocationResultDetailed(
        _selectedLocation!.name,
        _selectedLocation!.streetAddress,
        _selectedLocation!.locality,
        _selectedLocation!.regionCode,
        _selectedLocation!.postcode,
      );
      final title = _selectedLocation!.name.isNotEmpty
          ? _selectedLocation!.name
          : detailed;
      final sub = _stripTitlePrefix(detailed, title);
      return (title: title, sub: sub.isEmpty ? null : sub);
    }
    final current = state.currentLocation;
    if (current != null) {
      final title = current.name.isNotEmpty
          ? current.name
          : (current.locality.isNotEmpty ? current.locality : 'Location');
      final detailed = LocationFormatter.formatLocationDetailed(current);
      final sub = _stripTitlePrefix(detailed, title);
      return (title: title, sub: sub.isEmpty ? null : sub);
    }
    return (title: 'Pick a location', sub: null);
  }

  static String _stripTitlePrefix(String full, String title) {
    if (title.isEmpty) return full;
    if (full.startsWith('$title, ')) return full.substring(title.length + 2);
    if (full.startsWith(title)) return full.substring(title.length).trimLeft();
    return full;
  }

  /// Handle map tap - select location at tapped coordinates with reverse geocoding
  Future<void> _onMapTap(double lat, double lng) async {
    try {
      // Try reverse geocoding first to get address/POI data. Routed
      // through the LocationPicker viewmodel so the modal never touches
      // the geocoding service directly (`docs/client/architecture.md`).
      final geocoded = await ref
          .read(locationPickerProvider.notifier)
          .reverseGeocodeForLatLng(lat, lng);

      if (!mounted) return;

      if (geocoded != null) {
        // Got address/POI data - use it
        unawaited(_onLocationSelected(geocoded));
      } else {
        // Fallback to raw coordinates if geocoding fails or returns no results
        final latFormatted = lat.toStringAsFixed(6);
        final lngFormatted = lng.toStringAsFixed(6);
        final coordinatesString = '$latFormatted°, $lngFormatted°';

        final result = LocationResult(
          name: coordinatesString,
          fullName: coordinatesString,
          type: 'coordinate',
          latitude: lat,
          longitude: lng,
          streetAddress: '',
          locality: '',
          region: '',
          postcode: '',
          regionCode: '',
        );

        unawaited(_onLocationSelected(result));
      }
    } catch (e) {
      _log.warning('Reverse geocoding failed: $e, falling back to coordinates');

      if (!mounted) return;

      // Fallback to raw coordinates on error
      final latFormatted = lat.toStringAsFixed(6);
      final lngFormatted = lng.toStringAsFixed(6);
      final coordinatesString = '$latFormatted°, $lngFormatted°';

      final result = LocationResult(
        name: coordinatesString,
        fullName: coordinatesString,
        type: 'coordinate',
        latitude: lat,
        longitude: lng,
        streetAddress: '',
        locality: '',
        region: '',
        postcode: '',
        regionCode: '',
      );

      unawaited(_onLocationSelected(result));
    }
  }

  /// Handle reset button - clears selection and resets map
  void _handleReset() {
    setState(() {
      _selectedLocation = null;
      _locationController.clear();
      _selectedChipId = null;
      // Clear the changes flag
      _hasUserMadeChanges = false;
    });
    FocusScope.of(context).unfocus();
  }

  /// Handle save button tap - save location changes and close modal
  Future<void> _handleSave() async {
    if (_isSaving) return;
    final state = ref.read(locationPickerProvider);

    if (!_isOwner && !widget.allowNonOwnerEdit) return;

    if (_selectedLocation == null && state.currentLocation != null) {
      if (mounted) {
        setState(() => _hasUserMadeChanges = false);
        widget.onSaved(state.currentLocation!.id);
      }
      return;
    }
    if (_selectedLocation == null) return;

    final userId = ref.read(authStateProvider).user?.id ?? '';
    setState(() => _isSaving = true);

    try {
      final savedLocationId =
          await ref.read(locationPickerProvider.notifier).saveLocation(
                selectedLocation: _selectedLocation!,
                userId: userId,
              );
      if (!mounted) return;
      setState(() => _hasUserMadeChanges = false);
      widget.onSaved(savedLocationId);
    } catch (e) {
      _log.severe('Failed to save location: $e');
      if (mounted) {
        setState(() => _isSaving = false);
        ToastHelper.showError(context, 'Failed to save location: $e');
      }
    }
  }

  /// Horizontal scrollable chips for saved locations
  /// Build chips row with new floating style
  Widget _buildChipsRow(LocationPickerState state) {
    // Helper to truncate chip labels at 20 characters
    String truncateLabel(String label) {
      if (label.length <= 20) return label;
      return '${label.substring(0, 17)}...';
    }

    // Build chip data list (icon-only current location, filter "Unknown")
    final chips = <ChipData>[
      // Use Current Location chip (icon only - no label)
      ChipData(
        label: '', // No label, icon only
        icon: Icons.my_location,
        value: 'USE_CURRENT_LOCATION',
        isPrimary: false,
      ),
      // Primary location (if valid name)
      if (state.hasPrimaryLocation &&
          LocationFormatter.formatLocationNameShort(state.primaryLocation!) !=
              'Unknown')
        ChipData(
          label: truncateLabel(
            LocationFormatter.formatLocationNameShort(state.primaryLocation!),
          ),
          icon: Icons.home,
          value: state.primaryLocation!.id,
          isPrimary: false,
        ),
      // Saved locations (filter out "Unknown")
      ...state.savedLocations
          .where((loc) => loc.id != state.primaryLocation?.id)
          .where(
            (loc) =>
                LocationFormatter.formatLocationNameShort(loc) != 'Unknown',
          )
          .take(20)
          .map(
            (loc) => ChipData(
              label: truncateLabel(
                LocationFormatter.formatLocationNameShort(loc),
              ),
              icon: Icons.place,
              value: loc.id,
            ),
          ),
    ];

    // Use explicitly selected chip ID
    final selectedValue = _selectedChipId;

    // Show loading state
    if (state.isLoadingSavedLocations) {
      return Container(
        height: 44,
        alignment: Alignment.centerLeft,
        child: const CircularProgressIndicator(),
      );
    }

    // Each chip is its own pill so the carousel scrolls edge-to-edge.
    return SizedBox(
      height: 36,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        padding: const EdgeInsets.symmetric(horizontal: 20),
        itemCount: chips.length,
        separatorBuilder: (context, index) => const SizedBox(width: 8),
        itemBuilder: (context, index) {
          final chip = chips[index];
          final isSelected = chip.value == selectedValue;
          final isIconOnly = chip.label.isEmpty;

          final chipPadding = isIconOnly
              ? const EdgeInsets.symmetric(horizontal: 10)
              : const EdgeInsets.symmetric(horizontal: 12);
          final chipContent = Center(
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(
                  chip.icon,
                  size: 14,
                  color: isSelected
                      ? AppColors.modalChipTextActive
                      : AppColors.modalChipTextSecondary,
                ),
                if (chip.label.isNotEmpty) ...[
                  const SizedBox(width: 6),
                  Text(
                    chip.label,
                    style: TextStyle(
                      fontSize: 13,
                      fontWeight: FontWeight.w500,
                      color: isSelected
                          ? AppColors.modalChipTextActive
                          : AppColors.modalChipText,
                    ),
                  ),
                ],
              ],
            ),
          );

          // Chips sit on the glass header band, not over the map, so plain
          // glass surfaces (no scrim) match the design's `bg-white/10` /
          // `bg-white` pills directly.
          final Widget pill = GlassSurface(
            useBlur: false,
            fill: isSelected
                ? AppColors.modalChipBackgroundActive
                : AppColors.modalChipBackground,
            border: isSelected
                ? AppColors.modalChipBorderActive
                : AppColors.modalChipBorder,
            borderRadius: BorderRadius.circular(18),
            padding: chipPadding,
            child: chipContent,
          );

          return Tappable(
            semanticsLabel: chip.label.isEmpty
                ? context.l10n.a11yMiscUseCurrentLocation
                : chip.label,
            onTap: () {
              // Handle current location chip tap
              if (chip.value == 'USE_CURRENT_LOCATION') {
                _useCurrentLocation();
                return;
              }

              // Handle saved location chip tap
              final loc = state.savedLocations.firstWhere(
                (loc) => loc.id == chip.value,
                orElse: () => state.primaryLocation?.id == chip.value
                    ? state.primaryLocation!
                    : Location(),
              );
              if (loc.id.isNotEmpty) {
                _onSavedLocationSelected(loc);
              }
            },
            child: pill,
          );
        },
      ),
    );
  }


  Widget _buildMapPreview(LocationPickerState state) {
    final currentLocation = state.currentLocation;
    final primaryLocation = state.primaryLocation;

    // Priority: 1) Selected location, 2) Current location, 3) Primary location,
    // 4) Default camera (smart fallback: primary residence → GPS → Austin)
    final lat = _selectedLocation?.latitude ??
        currentLocation?.latitudeDeg ??
        primaryLocation?.latitudeDeg ??
        state.defaultCameraLatitude ??
        30.2672; // Ultimate fallback (should rarely be used)
    final lng = _selectedLocation?.longitude ??
        currentLocation?.longitudeDeg ??
        primaryLocation?.longitudeDeg ??
        state.defaultCameraLongitude ??
        -97.7431; // Ultimate fallback (should rarely be used)

    // LocationMapPreview handles coordinate changes via didUpdateWidget,
    // which smoothly animates the camera using flyTo() (500ms animation).
    // No key is needed - widget updates in place following the discover_screen
    // pattern for flicker-free map transitions.
    return LocationMapPreview(
      latitude: lat,
      longitude: lng,
      showUserLocationMarker: _selectedLocation == null && currentLocation == null,
      height: double.infinity, // Fill available space
      showBorder: false, // No border
      showRoundedCorners: false, // No rounded corners
      onMapTap: _onMapTap, // Enable tap-to-select
      onCameraChanged: _onMapCameraChanged, // Track map center for search proximity
      interactive: true, // Pan/zoom must win over a host that competes for drags
    );
  }

}
