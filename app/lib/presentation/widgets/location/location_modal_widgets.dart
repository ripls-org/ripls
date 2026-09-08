import 'dart:async';

import 'package:flutter/foundation.dart' show Factory;
import 'package:flutter/gestures.dart'
    show EagerGestureRecognizer, OneSequenceGestureRecognizer;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart' show rootBundle;
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';
import 'package:logging/logging.dart';
import 'package:ripls/services/device_location_service.dart';
import 'package:ripls/services/providers.dart';
import '../../../core/theme/app_colors.dart';

final _log = Logger('LocationModalWidgets');

/// _darkMapStyleAsset is the asset path Google's setMapStyle reads when the
/// app is in dark mode. Same JSON shipped with the bundle in pubspec.yaml.
const String _darkMapStyleAsset = 'assets/map_styles/google_dark.json';

/// LocationMapPreview displays a Google Maps preview centered on the
/// supplied coordinates with a marker at the same point.
///
/// The marker is a default Google Marker; a custom BitmapDescriptor for
/// the ripple-circle aesthetic is deferred because Google's Circle is
/// geographic, not pixel-sized, and scales with zoom in a way the
/// design didn't expect. See #2188.
class LocationMapPreview extends ConsumerStatefulWidget {
  final double latitude;
  final double longitude;
  final double height;
  final double zoom;
  final bool showUserLocationMarker;
  final bool showBorder;
  final bool showRoundedCorners;
  final Function(double lat, double lng)? onMapTap;
  final Function(double lat, double lng)? onCameraChanged;

  /// Optional explicit marker set. When non-null it replaces the default
  /// single marker at ([latitude], [longitude]) — used to plot every
  /// proposal in a location poll. The blue user dot is still rendered
  /// separately via `myLocationEnabled`.
  final Set<Marker>? markers;

  /// When true and [markers] holds 2+ points, the camera fits all markers'
  /// bounds (with padding) on map creation and whenever the set changes,
  /// instead of centering on [latitude]/[longitude].
  final bool fitToMarkers;

  /// When true the map claims pan / pinch-zoom gestures eagerly (via an
  /// [EagerGestureRecognizer]) so it stays interactive even inside a parent
  /// that competes for drags — e.g. the location panel's swipe-to-close morph
  /// surface. Leave false for read-only previews embedded in scrolling pages,
  /// where the page should win the drag instead.
  final bool interactive;

  const LocationMapPreview({
    super.key,
    required this.latitude,
    required this.longitude,
    this.height = 200,
    this.zoom = 16.0,
    this.showUserLocationMarker = false,
    this.showBorder = true,
    this.showRoundedCorners = true,
    this.onMapTap,
    this.onCameraChanged,
    this.markers,
    this.fitToMarkers = false,
    this.interactive = false,
  });

  @override
  ConsumerState<LocationMapPreview> createState() => _LocationMapPreviewState();
}

class _LocationMapPreviewState extends ConsumerState<LocationMapPreview> {
  GoogleMapController? _controller;
  String? _darkStyleJson;
  bool _styleApplied = false;

  @override
  void initState() {
    super.initState();
    _loadDarkStyle();
  }

  Future<void> _loadDarkStyle() async {
    try {
      _darkStyleJson = await rootBundle.loadString(_darkMapStyleAsset);
    } catch (e) {
      _log.warning('Failed to load dark map style: $e');
    }
  }

  @override
  void didUpdateWidget(LocationMapPreview oldWidget) {
    super.didUpdateWidget(oldWidget);

    // Multi-marker poll mode: re-fit bounds when the marker set changes.
    if (_shouldFitBounds &&
        !_sameMarkerPositions(oldWidget.markers, widget.markers)) {
      _fitMarkerBounds();
      return;
    }

    if (_controller != null &&
        (oldWidget.latitude != widget.latitude ||
            oldWidget.longitude != widget.longitude)) {
      _animateToPosition(widget.latitude, widget.longitude);
    } else if (oldWidget.latitude != widget.latitude ||
        oldWidget.longitude != widget.longitude) {
      // Controller not ready yet; onMapCreated will pick up the new
      // coordinates from widget.latitude/widget.longitude.
      _log.fine('Map coordinates changed before controller ready — will replay in onMapCreated');
    }
  }

  /// Whether the camera should frame the whole marker set rather than a single
  /// point: requested via [LocationMapPreview.fitToMarkers] with 2+ markers.
  bool get _shouldFitBounds =>
      widget.fitToMarkers && (widget.markers?.length ?? 0) >= 2;

  bool _sameMarkerPositions(Set<Marker>? a, Set<Marker>? b) {
    final pa = (a ?? const <Marker>{}).map((m) => m.position).toSet();
    final pb = (b ?? const <Marker>{}).map((m) => m.position).toSet();
    return pa.length == pb.length && pa.containsAll(pb);
  }

  /// Animates the camera so every marker is visible, with edge padding.
  Future<void> _fitMarkerBounds() async {
    final controller = _controller;
    final markers = widget.markers;
    if (controller == null || markers == null || markers.length < 2) return;
    final lats = markers.map((m) => m.position.latitude);
    final lngs = markers.map((m) => m.position.longitude);
    final bounds = LatLngBounds(
      southwest: LatLng(lats.reduce((a, b) => a < b ? a : b),
          lngs.reduce((a, b) => a < b ? a : b)),
      northeast: LatLng(lats.reduce((a, b) => a > b ? a : b),
          lngs.reduce((a, b) => a > b ? a : b)),
    );
    try {
      await controller.animateCamera(CameraUpdate.newLatLngBounds(bounds, 56));
    } catch (e, stackTrace) {
      _log.severe('Error fitting marker bounds: $e', e, stackTrace);
    }
  }

  Future<void> _animateToPosition(double lat, double lng) async {
    final controller = _controller;
    if (controller == null) return;
    try {
      await controller.animateCamera(
        CameraUpdate.newCameraPosition(
          CameraPosition(target: LatLng(lat, lng), zoom: widget.zoom),
        ),
      );
      unawaited(_notifyCameraPosition());
    } catch (e, stackTrace) {
      _log.severe('Error in animateCamera: $e', e, stackTrace);
    }
  }

  Future<void> _onMapCreated(GoogleMapController controller) async {
    setState(() {
      _controller = controller;
    });
    await _maybeApplyDarkStyle();
    if (!mounted) return;
    if (_shouldFitBounds) {
      // Poll mode: frame all proposals rather than the centroid.
      await _fitMarkerBounds();
    } else {
      // Replay current coords in case didUpdateWidget hopped them before
      // the controller arrived (same race the old Mapbox path guarded).
      await _animateToPosition(widget.latitude, widget.longitude);
    }

    if (widget.onCameraChanged != null) {
      unawaited(_notifyCameraPosition());
    }
  }

  Future<void> _maybeApplyDarkStyle() async {
    final controller = _controller;
    if (controller == null || _styleApplied) return;
    if (!mounted) return;
    final isDark = Theme.of(context).brightness == Brightness.dark;
    if (!isDark || _darkStyleJson == null) {
      return;
    }
    try {
      // setMapStyle is deprecated in newer plugin versions in favor of the
      // GoogleMap.style parameter; keep the runtime call until we move to
      // that parameter, since the deprecation hasn't shipped a removal.
      // ignore: deprecated_member_use
      await controller.setMapStyle(_darkStyleJson);
      _styleApplied = true;
    } catch (e) {
      _log.warning('Failed to apply dark map style: $e');
    }
  }

  Future<void> _notifyCameraPosition() async {
    // A pending camera-idle / animate callback can fire after the map surface
    // is torn down (e.g. the panel closed): the controller is then disposed and
    // getVisibleRegion throws. Bail before touching it, and treat a dispose
    // mid-call as benign rather than logging it as an error.
    if (!mounted) return;
    final controller = _controller;
    if (controller == null || widget.onCameraChanged == null) return;
    try {
      final visible = await controller.getVisibleRegion();
      if (!mounted) return;
      final center = LatLng(
        (visible.northeast.latitude + visible.southwest.latitude) / 2,
        (visible.northeast.longitude + visible.southwest.longitude) / 2,
      );
      widget.onCameraChanged!(center.latitude, center.longitude);
    } catch (e) {
      _log.fine('Skipped camera position (map unavailable or disposed): $e');
    }
  }

  Set<Marker> _markers(BuildContext context) {
    // Explicit set (poll mode) wins — plot every supplied proposal marker.
    if (widget.markers != null) return widget.markers!;
    if (widget.showUserLocationMarker) {
      // The blue dot is rendered by GoogleMap.myLocationEnabled; no
      // discrete marker needed here.
      return const {};
    }
    final color = AppColors.primary(context);
    return {
      Marker(
        markerId: const MarkerId('selected_location'),
        position: LatLng(widget.latitude, widget.longitude),
        icon: BitmapDescriptor.defaultMarkerWithHue(_hueForColor(color)),
        consumeTapEvents: false,
      ),
    };
  }

  // Google's BitmapDescriptor.defaultMarkerWithHue takes a 0-360 hue;
  // approximate the brand color by mapping to the closest preset hue.
  // For the Ripls primary blue this lands on hueAzure.
  double _hueForColor(Color color) {
    final hsl = HSLColor.fromColor(color);
    return hsl.hue;
  }

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final actualHeight = widget.height == double.infinity
            ? constraints.maxHeight
            : widget.height;

        if (actualHeight.isNaN || actualHeight.isInfinite) {
          _log.severe('Invalid map height detected: $actualHeight');
          return Container(
            height: 400,
            color: Colors.grey.shade300,
            child: const Center(child: Text('Map sizing error - using fallback')),
          );
        }

        // Re-apply dark style if theme toggled while map was already
        // mounted. Cheap call when nothing has changed.
        WidgetsBinding.instance.addPostFrameCallback((_) => _maybeApplyDarkStyle());

        return SizedBox(
          width: double.infinity,
          height: actualHeight,
          child: Container(
            decoration: BoxDecoration(
              borderRadius: widget.showRoundedCorners
                  ? BorderRadius.circular(12)
                  : null,
              border: widget.showBorder
                  ? Border.all(
                      color: AppColors.primary(context).withAlpha(100),
                      width: 2,
                    )
                  : null,
            ),
            clipBehavior: widget.showRoundedCorners ? Clip.antiAlias : Clip.none,
            child: GoogleMap(
              key: const ValueKey('location_picker_map'),
              initialCameraPosition: CameraPosition(
                target: LatLng(widget.latitude, widget.longitude),
                zoom: widget.zoom,
              ),
              onMapCreated: _onMapCreated,
              onTap: widget.onMapTap == null
                  ? null
                  : (LatLng pos) =>
                      widget.onMapTap!(pos.latitude, pos.longitude),
              onCameraIdle: widget.onCameraChanged == null
                  ? null
                  : _notifyCameraPosition,
              myLocationEnabled: ref
                      .watch(userLocationProvider(LocationIntent.precisePin))
                      .asData
                      ?.value !=
                  null,
              myLocationButtonEnabled: false,
              compassEnabled: false,
              zoomControlsEnabled: false,
              markers: _markers(context),
              // Win pan / pinch gestures over a competing parent (the panel's
              // swipe-to-close) so the map is draggable and zoomable.
              gestureRecognizers: widget.interactive
                  ? <Factory<OneSequenceGestureRecognizer>>{
                      Factory<OneSequenceGestureRecognizer>(
                        EagerGestureRecognizer.new,
                      ),
                    }
                  : const <Factory<OneSequenceGestureRecognizer>>{},
            ),
          ),
        );
      },
    );
  }
}
