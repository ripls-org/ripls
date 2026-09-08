import 'package:flutter/foundation.dart';
import 'package:flutter/painting.dart';
import 'package:flutter/services.dart' show rootBundle;
import 'package:geolocator/geolocator.dart' as geo;
import 'package:google_maps_flutter/google_maps_flutter.dart';
import 'package:ripls/presentation/models/discover_item.dart';

/// Neutral map-style identifiers stored in `SearchState.filters.mapStyle`.
/// The discover screen translates these into [MapType] + optional JSON
/// style. Kept as plain strings rather than an enum so the existing
/// freezed `@Default` works without further codegen.
const String mapStyleStandard = 'standard';
const String mapStyleOutdoor = 'outdoor';
const String mapStyleSatellite = 'satellite';
const String mapStyleDark = 'dark';

/// _darkMapStyleAsset is the JSON style applied via setMapStyle when the
/// user picks the dark variant — the same asset the location modal uses.
const String _darkMapStyleAsset = 'assets/map_styles/google_dark.json';

/// DiscoverMapHelper holds the discover screen's map-side state — the
/// GoogleMapController, the current marker set, the selected item, the
/// active style, the bottom padding driven by the bottom sheet.
///
/// The screen owns the [GoogleMap] widget itself and reads [markers],
/// [mapType], and [padding] from this helper. Marker taps flow back
/// through the [onMarkerTapped] callback the screen wires up.
class DiscoverMapHelper {
  GoogleMapController? _controller;
  Set<Marker> _markers = const {};
  // Tracked so future commits can render a scaled BitmapDescriptor for
  // the selected marker (the Mapbox version's 1.35x scale on selection
  // has no direct Google equivalent; the gallery overlay below the
  // map already communicates selection so the regression is bearable).
  // ignore: unused_field
  String? _selectedItemId;
  String _currentStyle = mapStyleStandard;
  String? _darkStyleJson;
  double _bottomPadding = 0;

  /// onMarkerTapped is invoked when the user taps a marker. The screen
  /// is responsible for setting this before [buildMarkers] runs;
  /// rebuilding the marker set captures the new callback into each
  /// Marker's onTap.
  ValueChanged<DiscoverItem>? onMarkerTapped;

  GoogleMapController? get controller => _controller;
  Set<Marker> get markers => _markers;
  
  /// mapType maps the active style identifier to Google's MapType enum.
  /// The dark style is rendered as MapType.normal with a JSON style
  /// applied via setMapStyle (which the helper drives in onMapCreated
  /// and changeStyle).
  MapType get mapType {
    switch (_currentStyle) {
      case mapStyleOutdoor:
        return MapType.terrain;
      case mapStyleSatellite:
        return MapType.hybrid;
      case mapStyleDark:
      case mapStyleStandard:
      default:
        return MapType.normal;
    }
  }

  /// Bottom padding applied to camera moves so focused items land in
  /// the visible map area above the bottom sheet.
  EdgeInsets get padding => EdgeInsets.only(bottom: _bottomPadding);

  /// setBottomPadding updates the padding applied to all camera moves.
  /// Pass [pixels] equal to `screenHeight * currentSheetFraction`.
  void setBottomPadding(double pixels) {
    _bottomPadding = pixels;
  }

  /// onMapCreated stores the controller and applies the current style
  /// (the dark JSON, when applicable).
  Future<void> onMapCreated(GoogleMapController controller) async {
    _controller = controller;
    await _applyStyle();
  }

  /// changeStyle swaps the active map style at runtime. The screen
  /// should rebuild the [GoogleMap] widget so the new [mapType] flows
  /// through; setMapStyle handles the JSON-style swap for dark.
  Future<void> changeStyle(String style) async {
    _currentStyle = style;
    await _applyStyle();
  }

  Future<void> _applyStyle() async {
    final controller = _controller;
    if (controller == null) return;
    try {
      if (_currentStyle == mapStyleDark) {
        _darkStyleJson ??= await rootBundle.loadString(_darkMapStyleAsset);
        // ignore: deprecated_member_use
        await controller.setMapStyle(_darkStyleJson);
      } else {
        // ignore: deprecated_member_use
        await controller.setMapStyle(null);
      }
    } catch (e) {
      debugPrint('Failed to apply map style "$_currentStyle": $e');
    }
  }

  /// buildMarkers produces a fresh marker set for the supplied items.
  /// Items without coordinates or without a pre-rendered thumbnail are
  /// skipped. The new set is stored as [markers] and also returned for
  /// convenience.
  Set<Marker> buildMarkers({
    required List<DiscoverItem> items,
    required Map<String, Uint8List> thumbnailImages,
  }) {
    final markers = <Marker>{};
    for (final item in items) {
      final (lat, lon) = _coordsOf(item);
      if (lat == 0 && lon == 0) continue;
      final bytes = thumbnailImages[item.id];
      if (bytes == null) continue;

      markers.add(
        Marker(
          markerId: MarkerId(item.id),
          position: LatLng(lat, lon),
          // The thumbnail PNG is rendered at 112 physical px by
          // MapAvatarRenderer; BitmapDescriptor.bytes treats the bitmap as
          // 1:1 logical pixels, which is too large at typical device DPR.
          // Setting width to 40 logical px brings the marker back to a
          // visually reasonable size (about 1/3 of the unscaled render).
          icon: BitmapDescriptor.bytes(bytes, width: 40),
          anchor: const Offset(0.5, 1),
          onTap: () => _handleMarkerTap(item),
        ),
      );
    }
    _markers = markers;
    return markers;
  }

  void _handleMarkerTap(DiscoverItem item) {
    _selectedItemId = item.id;
    onMarkerTapped?.call(item);
  }

  /// clearSelection drops the recorded selection. Visual de-selection
  /// is handled by the gallery overlay below the map — the marker set
  /// itself doesn't currently scale-on-select (a regression from the
  /// Mapbox version, deferred until we render scaled BitmapDescriptors).
  Future<void> clearSelection() async {
    _selectedItemId = null;
  }

  
  /// updateCameraForItems animates the camera to fit all the supplied
  /// items' positions. With no items, centers on [userPosition] when
  /// supplied, otherwise leaves the camera unchanged.
  Future<void> updateCameraForItems(
    List<DiscoverItem> items, {
    geo.Position? userPosition,
  }) async {
    final controller = _controller;
    if (controller == null) return;

    final coords = <LatLng>[];
    for (final item in items) {
      final (lat, lon) = _coordsOf(item);
      if (lat != 0 || lon != 0) coords.add(LatLng(lat, lon));
    }

    try {
      if (coords.isEmpty) {
        if (userPosition == null) return;
        await controller.animateCamera(
          CameraUpdate.newCameraPosition(
            CameraPosition(
              target: LatLng(userPosition.latitude, userPosition.longitude),
              zoom: 12,
              tilt: 45,
            ),
          ),
        );
        return;
      }

      if (coords.length == 1) {
        await controller.animateCamera(
          CameraUpdate.newCameraPosition(
            CameraPosition(target: coords.first, zoom: 14, tilt: 45),
          ),
        );
        return;
      }

      await controller.animateCamera(
        CameraUpdate.newLatLngBounds(_boundsOf(coords), 60),
      );
    } catch (e) {
      debugPrint('Failed to update camera position: $e');
    }
  }

  /// focusOnItem flies to a single item with a smooth animation.
  /// Skipped silently when the controller isn't ready or the item has
  /// no coordinates.
  Future<void> focusOnItem({
    required DiscoverItem item,
    double zoom = 14.0,
  }) async {
    final controller = _controller;
    if (controller == null) return;
    final (lat, lon) = _coordsOf(item);
    if (lat == 0 && lon == 0) {
      debugPrint('Cannot focus on item without location data: ${item.id}');
      return;
    }
    try {
      await controller.animateCamera(
        CameraUpdate.newCameraPosition(
          CameraPosition(
            target: LatLng(lat, lon),
            zoom: zoom,
            tilt: 45,
          ),
        ),
      );
    } catch (e) {
      debugPrint('Failed to focus camera on item: $e');
    }
  }

  /// clearAllMarkers drops every marker from the rendered set. Use when
  /// refreshing data so stale markers don't linger before the new batch
  /// arrives.
  Future<void> clearAllMarkers() async {
    _markers = const {};
    _selectedItemId = null;
  }

  
  
  /// dispose drops the controller reference. Markers are owned by the
  /// widget tree; nothing to free explicitly on the Google side.
  void dispose() {
    _controller = null;
    _markers = const {};
    _selectedItemId = null;
  }

  static (double, double) _coordsOf(DiscoverItem item) => item.when(
        gear: (g) => (g.latitudeDeg, g.longitudeDeg),
        request: (r) => (r.latitudeDeg, r.longitudeDeg),
        experience: (e) => (e.latitudeDeg, e.longitudeDeg),
      );

  /// initialCameraForItems returns a CameraPosition centered on the
  /// supplied items' centroid (or the single item's position) so the
  /// GoogleMap widget can mount already framing the results. Falls
  /// back to null when none of the items carry coordinates — the
  /// caller should then use a user-location or default camera.
  static CameraPosition? initialCameraForItems(List<DiscoverItem> items) {
    final coords = <LatLng>[];
    for (final item in items) {
      final (lat, lon) = _coordsOf(item);
      if (lat != 0 || lon != 0) coords.add(LatLng(lat, lon));
    }
    if (coords.isEmpty) return null;
    if (coords.length == 1) {
      return CameraPosition(target: coords.first, zoom: 14, tilt: 45);
    }
    // Approximate the bounding box center + pick a zoom that fits the
    // spread. The post-mount animateCamera in updateCameraForItems
    // refines with newLatLngBounds (which uses the actual map size);
    // this initial estimate just gets us close enough that the user
    // doesn't see a big NA → zoom flight.
    double minLat = coords.first.latitude;
    double maxLat = coords.first.latitude;
    double minLng = coords.first.longitude;
    double maxLng = coords.first.longitude;
    for (final c in coords) {
      if (c.latitude < minLat) minLat = c.latitude;
      if (c.latitude > maxLat) maxLat = c.latitude;
      if (c.longitude < minLng) minLng = c.longitude;
      if (c.longitude > maxLng) maxLng = c.longitude;
    }
    final centerLat = (minLat + maxLat) / 2;
    final centerLng = (minLng + maxLng) / 2;
    final span = [maxLat - minLat, maxLng - minLng].reduce((a, b) => a > b ? a : b);
    final zoom = _zoomForSpan(span);
    return CameraPosition(
      target: LatLng(centerLat, centerLng),
      zoom: zoom,
      tilt: 45,
    );
  }

  /// _zoomForSpan picks a Google zoom level for a coordinate span. Mirrors
  /// the breakpoints used by the pre-migration Mapbox helper.
  static double _zoomForSpan(double span) {
    if (span > 10) return 5;
    if (span > 5) return 6;
    if (span > 2) return 7;
    if (span > 1) return 8;
    if (span > 0.5) return 9;
    if (span > 0.2) return 10;
    if (span > 0.1) return 11;
    if (span > 0.05) return 12;
    if (span > 0.02) return 13;
    if (span > 0.01) return 14;
    return 15;
  }

  static LatLngBounds _boundsOf(List<LatLng> coords) {
    double minLat = coords.first.latitude;
    double maxLat = coords.first.latitude;
    double minLng = coords.first.longitude;
    double maxLng = coords.first.longitude;
    for (final c in coords) {
      if (c.latitude < minLat) minLat = c.latitude;
      if (c.latitude > maxLat) maxLat = c.latitude;
      if (c.longitude < minLng) minLng = c.longitude;
      if (c.longitude > maxLng) maxLng = c.longitude;
    }
    return LatLngBounds(
      southwest: LatLng(minLat, minLng),
      northeast: LatLng(maxLat, maxLng),
    );
  }
}
