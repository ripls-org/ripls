import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/utils/distance_formatter.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/gen/ripls/api/item.pb.dart';
import 'package:ripls/data/gen/ripls/api/search_service.pb.dart'
    show SearchItemType, SearchResultItem;
import 'package:ripls/presentation/models/discover_item.dart';
import 'package:ripls/presentation/screens/workshop/workshop_library_common.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/floating_header.dart';
import 'package:ripls/presentation/widgets/location/location_modal_widgets.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_morph.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_overview_theme.dart';
import 'package:ripls/services/device_location_service.dart' show LocationIntent;
import 'package:ripls/services/providers/location_providers.dart';
import 'package:ripls/services/providers/user_location_provider.dart';

/// The Community Library plotted on a real, interactive map — the "Where"
/// destination (#2447). A Google map up top, fit to the bounds of every
/// in-scope located entry (gear / requests / experiences), then the list of
/// those entries below.
///
/// Opened from the overview's "Common Ground" section: [category] is the
/// tapped category (null = "All"), applied as the search filter via
/// [workshopLibraryLocatedItemsProvider]. Because a Google map is a platform
/// view screen readers can't introspect, the list below stays the accessible
/// source of truth.
class WorkshopLibraryMapPanel extends ConsumerWidget {
  final List<String> communityIds;

  /// Active "Common Ground" category filter; null shows everything ("All").
  final String? category;

  const WorkshopLibraryMapPanel({
    super.key,
    required this.communityIds,
    this.category,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final locatedAsync = ref.watch(
      workshopLibraryLocatedItemsProvider((
        idsKey: workshopLibraryItemsKey(communityIds),
        query: category ?? '',
      )),
    );
    return WorkshopMorphPanel(
      title: context.l10n.workshopLibraryMapTitle,
      eyebrow: category,
      child: locatedAsync.when(
        loading: WorkshopLibraryStates.loading,
        error: (e, _) => WorkshopLibraryStates.error(e),
        data: (items) {
          if (items.isEmpty) return WorkshopLibraryStates.empty(context);
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              _LibraryMap(items: items),
              Expanded(
                child: WorkshopCenteredList(
                  bottomPadding: FloatingHeader.contentBottom(context) + 16,
                  children: [
                    for (var i = 0; i < items.length; i++)
                      _MapRow(index: i + 1, located: items[i]),
                  ],
                ),
              ),
            ],
          );
        },
      ),
    );
  }
}

/// Opens the underlying gear / request / experience for a located entry,
/// reusing [NavigationHelpers.pushToItem] via a synthesized [Item].
void _openLocated(BuildContext context, SearchDiscoverItem item) {
  final kind = switch (item.itemType) {
    SearchItemType.SEARCH_ITEM_TYPE_GEAR => ItemKind.ITEM_KIND_GEAR,
    SearchItemType.SEARCH_ITEM_TYPE_REQUEST => ItemKind.ITEM_KIND_REQUEST,
    SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE => ItemKind.ITEM_KIND_EXPERIENCE,
    _ => ItemKind.ITEM_KIND_UNSPECIFIED,
  };
  if (item.id.isEmpty || kind == ItemKind.ITEM_KIND_UNSPECIFIED) return;
  NavigationHelpers.pushToItem(
    context: context,
    item: Item()
      ..contextId = item.id
      ..kind = kind
      ..title = item.name,
  );
}

/// The real interactive map: plots every located community entry and fits the
/// camera to their bounds. Centers on the contiguous-US fallback when no entry
/// carries coordinates yet (mirrors Discover's empty-state camera).
class _LibraryMap extends StatelessWidget {
  final List<SearchResultItem> items;
  const _LibraryMap({required this.items});

  static const double _height = 280;
  // Contiguous-US fallback (matches Discover) for the no-coordinates case.
  static const double _fallbackLat = 39.5;
  static const double _fallbackLng = -98.5;

  @override
  Widget build(BuildContext context) {
    final markers = <Marker>{};
    final seen = <String>{};
    for (final result in items) {
      final item = SearchDiscoverItem(result);
      final lat = item.latitudeDeg;
      final lng = item.longitudeDeg;
      if ((lat == 0 && lng == 0) || item.id.isEmpty || !seen.add(item.id)) {
        continue;
      }
      markers.add(
        Marker(
          markerId: MarkerId(item.id),
          position: LatLng(lat, lng),
          icon: BitmapDescriptor.defaultMarkerWithHue(
            BitmapDescriptor.hueGreen,
          ),
          onTap: () => _openLocated(context, item),
        ),
      );
    }

    final center = markers.isNotEmpty
        ? markers.first.position
        : const LatLng(_fallbackLat, _fallbackLng);
    final zoom = markers.isEmpty
        ? 3.0
        : markers.length == 1
            ? 14.0
            : 12.0;

    // The morph panel runs under [AppTheme.darkTheme], which would push the
    // map into its dark style. Force the app's light theme around the map so it
    // renders the standard Google style, like Discover and the location modals.
    return Theme(
      data: AppTheme.lightTheme,
      child: LocationMapPreview(
        latitude: center.latitude,
        longitude: center.longitude,
        zoom: zoom,
        height: _height,
        markers: markers,
        fitToMarkers: markers.length >= 2,
        interactive: true,
        showBorder: false,
        showRoundedCorners: false,
        showUserLocationMarker: true,
      ),
    );
  }
}

/// A numbered library list row — numbered circle + title + "owner · location ·
/// distance" line, tappable to open the entry.
class _MapRow extends ConsumerWidget {
  final int index;
  final SearchResultItem located;

  const _MapRow({required this.index, required this.located});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final di = SearchDiscoverItem(located);
    final detail = _detail(ref) ?? '';
    return Tappable(
      semanticsLabel: di.name,
      onTap: () => _openLocated(context, di),
      child: Container(
        decoration: const BoxDecoration(
          border: Border(bottom: BorderSide(color: GlassTokens.hairline)),
        ),
        padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 11),
        child: Row(
          children: [
            Container(
              width: 22,
              height: 22,
              alignment: Alignment.center,
              decoration: const BoxDecoration(
                shape: BoxShape.circle,
                color: WorkshopOverviewPalette.accent,
              ),
              child: Text(
                '$index',
                style: const TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w800,
                  color: Color(0xFF15281A),
                ),
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    di.name,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      fontFamily: AppTheme.headingFont,
                      fontSize: 14,
                      color: WorkshopOverviewPalette.onPhoto,
                    ),
                  ),
                  if (detail.isNotEmpty)
                    Padding(
                      padding: const EdgeInsets.only(top: 2),
                      child: Text(
                        detail,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          fontSize: 11,
                          color: WorkshopOverviewPalette.onPhotoDim,
                        ),
                      ),
                    ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  /// Owner · short location · distance-from-me — sourced from the located
  /// entry. Each part is dropped when unavailable (e.g. no GPS fix, or a
  /// location whose name hasn't resolved yet).
  String? _detail(WidgetRef ref) {
    final locationId = workshopEntryLocationId(located);
    final locationName = locationId != null && locationId.isNotEmpty
        ? ref.watch(locationDisplayProvider(locationId)).asData?.value
        : null;
    return workshopJoinMeta([
      workshopEntryOwner(located),
      locationName,
      _distance(ref),
    ]);
  }

  /// Haversine distance from the user's current position to the entry,
  /// formatted; null when either coordinate is unknown.
  String? _distance(WidgetRef ref) {
    final di = SearchDiscoverItem(located);
    if (di.latitudeDeg == 0 && di.longitudeDeg == 0) return null;
    final pos = ref
        .watch(userLocationProvider(LocationIntent.proximityBias))
        .asData
        ?.value;
    if (pos == null) return null;
    return DistanceFormatter.format(
      DistanceFormatter.calculateDistance(
        lat1: pos.latitude,
        lon1: pos.longitude,
        lat2: di.latitudeDeg,
        lon2: di.longitudeDeg,
      ),
    );
  }
}
