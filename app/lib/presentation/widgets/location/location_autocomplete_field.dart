import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_typeahead/flutter_typeahead.dart';
import 'package:geolocator/geolocator.dart';

import '../../../core/theme/app_colors.dart';
import '../../../services/device_location_service.dart';
import '../../../services/location_search_service.dart';
import '../../../services/location_service.dart';
import '../../../services/providers.dart';
import '../../../services/user_service.dart';

/// Signature for the search callback supplied to
/// [LocationAutocompleteField]. The widget calls this on each keystroke
/// past the threshold; implementations route through their ViewModel +
/// Repository so the widget has no direct knowledge of the geocoding
/// provider.
typedef LocationSearchCallback = Future<List<LocationResult>> Function(
  String query, {
  Position? proximity,
});

/// Helper class to hold display information for a location suggestion
class _DisplayInfo {
  final String primary;
  final String secondary;

  _DisplayInfo({required this.primary, required this.secondary});
}

/// Widget that provides location autocomplete by delegating to a
/// caller-supplied [search] callback. The widget itself has no knowledge
/// of the underlying geocoding provider — consumers route through their
/// ViewModel + Repository.
class LocationAutocompleteField extends ConsumerStatefulWidget {
  final TextEditingController controller;
  final Function(LocationResult) onLocationSelected;

  /// Per-keystroke search callback. Invoked once the typed query reaches
  /// the threshold (or unconditionally when [geographicAreasOnly] is
  /// true). Implementations must not throw; return an empty list on
  /// failure so the typeahead degrades gracefully.
  final LocationSearchCallback search;

  final String hintText;
  final String? Function(String?)? validator;
  final bool geographicAreasOnly;
  final Position? proximityBias;
  final bool showBorder;

  const LocationAutocompleteField({
    super.key,
    required this.controller,
    required this.onLocationSelected,
    required this.search,
    this.hintText = 'Enter address or place...',
    this.validator,
    this.geographicAreasOnly = false,
    this.proximityBias,
    this.showBorder = true,
  });

  @override
  ConsumerState<LocationAutocompleteField> createState() =>
      _LocationAutocompleteFieldState();
}

class _LocationAutocompleteFieldState
    extends ConsumerState<LocationAutocompleteField> {
  List<UserLocationInfo>? _savedLocations;

  @override
  void initState() {
    super.initState();
    _loadSavedLocations();
  }

  Future<void> _loadSavedLocations() async {
    final authState = ref.read(authStateProvider);
    final userId = authState.user?.id;

    if (userId == null || userId.isEmpty) {
      return;
    }

    try {
      final userService = ref.read(userServiceProvider);
      final locations = await userService.getUserSavedLocations(userId);
      if (mounted) {
        setState(() {
          _savedLocations = locations;
        });
      }
    } catch (e) {
      // Silently fail - saved locations are optional
    }
  }

  /// Converts UserLocationInfo to LocationResult for autocomplete
  LocationResult _convertToLocationResult(UserLocationInfo userLocation) {
    final loc = userLocation.locationDetails;
    return LocationResult(
      name: userLocation.isPrimary ? 'Home' : loc.locality,
      fullName: _buildFullAddress(loc),
      type: 'Address',
      latitude: loc.latitudeDeg,
      longitude: loc.longitudeDeg,
      streetAddress: loc.addressLines.isNotEmpty ? loc.addressLines.first : '',
      locality: loc.locality,
      region: loc.regionCode,
      postcode: loc.postalCode,
      regionCode: loc.regionCode,
    );
  }

  String _buildFullAddress(Location loc) {
    final parts = <String>[];
    if (loc.addressLines.isNotEmpty) {
      parts.addAll(loc.addressLines);
    }
    if (loc.locality.isNotEmpty) {
      parts.add(loc.locality);
    }
    if (loc.regionCode.isNotEmpty && loc.postalCode.isNotEmpty) {
      parts.add('${loc.regionCode} ${loc.postalCode}');
    } else if (loc.regionCode.isNotEmpty) {
      parts.add(loc.regionCode);
    } else if (loc.postalCode.isNotEmpty) {
      parts.add(loc.postalCode);
    }
    return parts.join(', ');
  }

  @override
  Widget build(BuildContext context) {
    final screenHeight = MediaQuery.of(context).size.height;
    final maxSuggestionsHeight = screenHeight * 0.3;

    // TypeAheadField for address search
    return TypeAheadField<LocationResult>(
          controller: widget.controller,
          hideOnEmpty: true,
          hideOnLoading: false,
          direction: VerticalDirection.down,
          constraints: BoxConstraints(maxHeight: maxSuggestionsHeight),
          suggestionsCallback: (pattern) async {
            final query = pattern.trim();

            // Show saved locations when query is short (< 4 characters) and not in geographic-only mode
            if (query.length < 4 && !widget.geographicAreasOnly) {
              if (_savedLocations != null && _savedLocations!.isNotEmpty) {
                // Convert saved locations to LocationResult format
                return _savedLocations!
                    .map((loc) => _convertToLocationResult(loc))
                    .toList();
              }
              return [];
            }

            // For queries with 4+ characters, delegate to the caller's
            // search callback. The caller routes through its ViewModel +
            // Repository so the widget never touches a service directly.
            final proximity = widget.proximityBias ??
                ref
                    .read(userLocationProvider(LocationIntent.proximityBias))
                    .asData
                    ?.value;

            return widget.search(query, proximity: proximity);
          },
          builder: (context, textEditingController, focusNode) {
            return TextFormField(
              controller: textEditingController,
              focusNode: focusNode,
              decoration: InputDecoration(
                hintText: widget.hintText,
                border: widget.showBorder
                    ? null
                    : const OutlineInputBorder(
                        borderRadius: BorderRadius.all(Radius.circular(24)),
                        borderSide: BorderSide.none,
                      ),
                enabledBorder: widget.showBorder
                    ? null
                    : const OutlineInputBorder(
                        borderRadius: BorderRadius.all(Radius.circular(24)),
                        borderSide: BorderSide.none,
                      ),
                focusedBorder: widget.showBorder
                    ? null
                    : const OutlineInputBorder(
                        borderRadius: BorderRadius.all(Radius.circular(24)),
                        borderSide: BorderSide.none,
                      ),
                errorBorder: widget.showBorder
                    ? null
                    : const OutlineInputBorder(
                        borderRadius: BorderRadius.all(Radius.circular(24)),
                        borderSide: BorderSide.none,
                      ),
                focusedErrorBorder: widget.showBorder
                    ? null
                    : const OutlineInputBorder(
                        borderRadius: BorderRadius.all(Radius.circular(24)),
                        borderSide: BorderSide.none,
                      ),
                isDense: true,
                contentPadding: const EdgeInsets.symmetric(
                  horizontal: 16,
                  vertical: 14,
                ),
              ),
              autocorrect: false,
              enableSuggestions: false,
              validator: widget.validator,
              textInputAction: TextInputAction.done,
              onTapOutside: (event) {
                // Dismiss autocomplete dropdown when tapping outside
                FocusScope.of(context).unfocus();
              },
            );
          },
          itemBuilder: (context, suggestion) {
            return _buildSuggestionItem(suggestion);
          },
          onSelected: (suggestion) {
            widget.controller.text = suggestion.fullName;
            widget.onLocationSelected(suggestion);
          },
        );
  }

  Widget _buildSuggestionItem(LocationResult suggestion) {
    final displayInfo = _extractDisplayInfo(suggestion);

    return ListTile(
      leading: Icon(
        _getIcon(suggestion.type),
        color: AppColors.textSecondary(context),
        size: 20,
      ),
      title: Text(
        displayInfo.primary,
        style: TextStyle(
          color: AppColors.textPrimary(context),
          fontSize: 14,
          fontWeight: FontWeight.w500,
        ),
      ),
      subtitle: Text(
        displayInfo.secondary,
        style: TextStyle(color: AppColors.textSecondary(context), fontSize: 12),
        maxLines: 2,
        overflow: TextOverflow.ellipsis,
      ),
      tileColor: AppColors.surface(context),
      hoverColor: AppColors.primary(context).withAlpha(30),
    );
  }

  /// Extracts and formats display information without redundancy
  _DisplayInfo _extractDisplayInfo(LocationResult suggestion) {
    // For POI (businesses, landmarks), show name in bold and address in non-bold
    if (suggestion.type.toLowerCase() == 'poi') {
      // Build secondary text with street address (if available) and location context
      final secondaryParts = <String>[];

      if (suggestion.streetAddress.isNotEmpty) {
        secondaryParts.add(suggestion.streetAddress);
      }

      final context = _buildLocationContext(suggestion);
      if (context.isNotEmpty) {
        secondaryParts.add(context);
      }

      return _DisplayInfo(
        primary: suggestion.name,
        secondary: secondaryParts.isEmpty
            ? suggestion.fullName.replaceFirst('${suggestion.name}, ', '')
            : secondaryParts.join(', '),
      );
    }

    // For addresses, extract street address and city/state/zip
    if (suggestion.type.toLowerCase() == 'address') {
      // fullName format: "123 Main St, City, State ZIP, Country"
      // name is typically just the house number or partial address
      // We need to extract the full street address from fullName
      final parts = suggestion.fullName
          .split(',')
          .map((p) => p.trim())
          .toList();

      if (parts.length > 1) {
        // First part is the street address
        final streetAddress = parts[0];
        // Everything else is the context
        final contextParts = parts.sublist(1);
        return _DisplayInfo(
          primary: streetAddress,
          secondary: contextParts.join(', '),
        );
      }

      // Fallback if we can't parse the fullName
      return _DisplayInfo(
        primary: suggestion.name,
        secondary: _buildLocationContext(suggestion),
      );
    }

    // For cities/localities, show the place name and the region/country
    return _DisplayInfo(
      primary: suggestion.name,
      secondary: _buildLocationContext(suggestion),
    );
  }

  /// Builds location context from locality, region, postcode, regionCode
  String _buildLocationContext(LocationResult suggestion) {
    final parts = <String>[];

    if (suggestion.locality.isNotEmpty) {
      parts.add(suggestion.locality);
    }
    if (suggestion.region.isNotEmpty) {
      parts.add(suggestion.region);
    }
    if (suggestion.postcode.isNotEmpty) {
      parts.add(suggestion.postcode);
    }
    if (suggestion.regionCode.isNotEmpty) {
      parts.add(suggestion.regionCode);
    }

    return parts.isEmpty ? '' : parts.join(', ');
  }

  IconData _getIcon(String type) {
    switch (type.toLowerCase()) {
      case 'address':
        return Icons.home;
      case 'poi':
        return Icons.place;
      case 'city':
        return Icons.location_city;
      case 'locality':
        return Icons.location_on;
      default:
        return Icons.location_on;
    }
  }
}
