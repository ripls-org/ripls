import 'package:geolocator/geolocator.dart' show Position;
import 'package:ripls/core/utils/distance_formatter.dart';
import 'package:ripls/presentation/viewmodels/location_panel_providers.dart';

/// Location-specific helpers for [ExperienceLocationPanel]. The shared poll
/// presentation widgets (option rows, badges, voter stacks, buttons, etc.) live
/// in `widgets/poll/poll_option_widgets.dart` and are used by both the location
/// ("Where") and time ("When") panels.

/// Viewer-relative "~X away" for a resolved spot, or null when the spot has no
/// coordinate or the viewer's position is unknown.
String? locationDistanceLabel(ResolvedLocationProposal r, Position? pos) {
  if (!r.hasCoordinate || pos == null) return null;
  final meters = DistanceFormatter.calculateDistance(
    lat1: pos.latitude,
    lon1: pos.longitude,
    lat2: r.latitude,
    lon2: r.longitude,
  );
  return DistanceFormatter.formatWithAway(meters);
}
