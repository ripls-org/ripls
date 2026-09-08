import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:google_maps_flutter/google_maps_flutter.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/discover_map_helper.dart';
import 'package:ripls/core/utils/location_permission_helper.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/core/utils/responsive.dart';
import 'package:ripls/data/gen/ripls/api/search_service.pb.dart'
    show SearchItemType;
import 'package:ripls/presentation/models/discover_item.dart';
import 'package:ripls/presentation/screens/create/unified_create_modal.dart';
import 'package:ripls/presentation/screens/experience/experience_screen.dart';
import 'package:ripls/presentation/screens/gear/gear_screen.dart';
import 'package:ripls/presentation/screens/request/request_screen.dart';
import 'package:ripls/presentation/viewmodels/discover_view_model.dart';
import 'package:ripls/presentation/viewmodels/home_view_model.dart';
import 'package:ripls/presentation/viewmodels/search_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/adaptive/content_column.dart';
import 'package:ripls/presentation/widgets/destination_header.dart';
import 'package:ripls/presentation/widgets/discover/discover_gallery_overlay.dart';
import 'package:ripls/presentation/widgets/discover/library_location_sheet.dart';
import 'package:ripls/presentation/widgets/discover/library_shelf_tile.dart';
import 'package:ripls/presentation/widgets/discover/map_style_sheet.dart';
import 'package:ripls/presentation/widgets/floating_header.dart';
import 'package:ripls/presentation/widgets/navigation/nav_dock.dart';
import 'package:ripls/presentation/widgets/search/close_search_button.dart';
import 'package:ripls/presentation/widgets/search/search_scope_pill.dart';
import 'package:ripls/services/device_location_service.dart';
import 'package:ripls/services/location_search_service.dart'
    show LocationResult;
import 'package:ripls/services/providers.dart';

/// DiscoverScreen is the Library tab (#2634 v2): gear shared across the
/// user's communities, map-first.
///
/// The map fills the screen under the Library header (title, counts,
/// Map/List toggle). Results float over the map as the item-card rail —
/// there is no results sheet — and the List toggle swaps in the
/// category-shelves view. There is no in-screen search: the dock's
/// universal search cap is the one search entry, and its Library rows
/// deep-link back to the items here.
class DiscoverScreen extends ConsumerStatefulWidget {
  const DiscoverScreen({super.key});

  @override
  ConsumerState<DiscoverScreen> createState() => _DiscoverScreenState();
}

class _DiscoverScreenState extends ConsumerState<DiscoverScreen> {
  late DiscoverMapHelper _mapHelper;

  /// Library view toggle (#2634 v2): true = the category-shelves list
  /// (the default), false = the map. The map stays mounted underneath
  /// the list so toggling never recreates it.
  bool _showListView = true;

  /// The distance anchor (#2634 v2 Rev 14): null means "Near me" (the
  /// user's position); otherwise the place chosen in the location sheet.
  /// The dynamic title states it out loud, and distances measure from it.
  LocationResult? _anchorPlace;

  /// Set when the anchor is one of the user's community areas, so the
  /// location sheet can mark that row Current.
  String? _anchorCommunityId;

  // Initial camera target: contiguous US, used when we have no user
  // position. Picked so an empty result set still reads as a map and
  // cues the user to center on themselves.
  static const CameraPosition _defaultUSCamera = CameraPosition(
    target: LatLng(39.5, -98.5),
    zoom: 2.5,
  );

  @override
  void initState() {
    super.initState();
    _mapHelper = DiscoverMapHelper();
    _mapHelper.onMarkerTapped = (item) {
      if (!mounted) return;
      ref.read(discoverProvider.notifier).selectItem(item);
    };
    // Location permission is NOT requested on mount — see #1174. The first
    // user gesture that needs GPS (tapping "center on me" or a
    // "use current location" control) is the prompt site.
  }

  /// Camera clearance for the chrome floating over the map's bottom edge:
  /// the dock plus the item-card rail (#2634 v2 — no results sheet).
  double _mapBottomClearance() {
    return MediaQuery.of(context).padding.bottom +
        NavDock.bottomContentInset +
        160;
  }

  /// _centerOnUserLocation animates the map camera to the user's current position.
  ///
  /// Prompts for location permission if not yet granted. This is the primary
  /// user-gesture entry point for location permission on the discover screen.
  Future<void> _centerOnUserLocation() async {
    if (!mounted) return;
    final controller = _mapHelper.controller;
    if (controller == null) return;

    final position = await requestLocationWithSettingsFallback(context, ref);
    if (!mounted) return;
    if (position == null) return;

    await controller.animateCamera(
      CameraUpdate.newCameraPosition(
        CameraPosition(
          target: LatLng(position.latitude, position.longitude),
          zoom: 14,
          tilt: 45,
        ),
      ),
    );
  }

  @override
  void dispose() {
    _mapHelper.dispose();
    super.dispose();
  }

  /// _refreshResults invalidates all caches and re-runs the current search.
  Future<void> _refreshResults() async {
    if (!mounted) return;
    await ref.read(searchProvider.notifier).refreshAllCaches();
  }

  void _openGearScreen(String gearId, {double? distanceMeters}) async {
    await NavigationHelpers.pushScreen(
      context: context,
      screen: GearScreen(
        gearId: gearId,
        initialDistanceMeters: distanceMeters,
      ),
      useRootNavigator: true,
      routeName: 'gear_detail',
    );
    unawaited(_refreshResults());
  }

  void _openRequestScreen(String requestId, {double? distanceMeters}) async {
    await NavigationHelpers.pushScreen(
      context: context,
      screen: RequestScreen(
        requestId: requestId,
        initialDistanceMeters: distanceMeters,
      ),
      useRootNavigator: true,
      routeName: 'request_detail',
    );
    unawaited(_refreshResults());
  }

  void _openExperienceScreen(String experienceId, {double? distanceMeters}) async {
    await NavigationHelpers.pushScreen(
      context: context,
      screen: ExperienceScreen(
        experienceId: experienceId,
        initialDistanceMeters: distanceMeters,
      ),
      useRootNavigator: true,
      routeName: 'experience_detail',
    );
    unawaited(_refreshResults());
  }

  /// _getCurrentItemsWithLocations returns search result items that have
  /// valid coordinates for map marker display.
  List<DiscoverItem> _getCurrentItemsWithLocations() {
    final searchState = ref.read(searchProvider).value;
    if (searchState == null) return [];
    return searchState.filteredResults.where((item) {
      if (item is SearchDiscoverItem) {
        return item.latitudeDeg != 0 && item.longitudeDeg != 0;
      }
      return false;
    }).toList();
  }

  Widget _buildLocationButton() {
    return _mapControlButton(
      icon: Icons.my_location,
      semanticsLabel: context.l10n.a11yDiscoverCenterLocation,
      onTap: _centerOnUserLocation,
    );
  }

  /// Opens the standalone map-style picker — base map styling lives on its
  /// own affordance now that it's no longer bundled with result filters.
  Widget _buildMapStyleButton() {
    return _mapControlButton(
      icon: Icons.layers_outlined,
      semanticsLabel: context.l10n.a11yDiscoverMapStyleButton,
      onTap: () => unawaited(MapStyleSheet.show(context)),
    );
  }

  /// A floating 40px circular control button on the map (center-on-me,
  /// map style).
  Widget _mapControlButton({
    required IconData icon,
    required String semanticsLabel,
    required VoidCallback onTap,
  }) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      child: Container(
        width: 40,
        height: 40,
        decoration: BoxDecoration(
          color: AppColors.surface(context),
          shape: BoxShape.circle,
          boxShadow: [
            BoxShadow(
              color: Colors.black.withAlpha(25),
              blurRadius: 8,
              offset: const Offset(0, 2),
            ),
          ],
        ),
        child: Icon(
          icon,
          size: 20,
          color: AppColors.textPrimary(context),
        ),
      ),
    );
  }

  Future<void> _updateMapMarkers({bool repositionCamera = true}) async {
    if (!mounted) return;
    final currentItems = _getCurrentItemsWithLocations();
    final thumbnailImages = await ref
        .read(discoverProvider.notifier)
        .resolveItemThumbnails(currentItems, context);
    if (!mounted) return;
    setState(() {
      _mapHelper.buildMarkers(
        items: currentItems,
        thumbnailImages: thumbnailImages,
      );
    });
    if (!repositionCamera) return;
    await _mapHelper.updateCameraForItems(
      currentItems,
      userPosition: ref.read(userLocationProvider(LocationIntent.precisePin)).asData?.value,
    );
  }

  Widget _buildNoCommunityState(BuildContext context, bool isNavVisible) {
    return Stack(
      children: [
        Center(
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 32),
            child: Text(
              'Select a community from the sidebar to discover nearby items',
              style: TextStyle(
                fontSize: 15,
                color: AppColors.textSecondary(context),
              ),
              textAlign: TextAlign.center,
            ),
          ),
        ),
        Positioned(
          top: MediaQuery.of(context).padding.top,
          left: 0,
          right: 0,
          child: AnimatedSlide(
            duration: accessibleDuration(context, const Duration(milliseconds: 200)),
            offset: isNavVisible ? Offset.zero : const Offset(0, -3),
            child: FloatingHeader(
              content: TextField(
                enabled: false,
                decoration: InputDecoration(
                  hintText: 'Select a community to search',
                  hintStyle: TextStyle(
                    fontSize: 14,
                    color: AppColors.textSecondary(context),
                  ),
                  filled: false,
                  fillColor: Colors.transparent,
                  border: InputBorder.none,
                  enabledBorder: InputBorder.none,
                  focusedBorder: InputBorder.none,
                  isDense: true,
                  contentPadding: const EdgeInsets.symmetric(vertical: 12),
                ),
              ),
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildMapView() {
    // Source priority for the initial camera:
    //   1. Already-loaded items — compute their centroid so we mount
    //      pre-framed and skip the NA → zoom flight.
    //   2. Session-cached user position — neighborhood-level view.
    //   3. Contiguous US fallback — cues "tap center-on-me to start."
    //
    // Mostly we hit case 1 because the search-nearby pill seeds
    // SearchNotifier before pushing the screen.
    final userPosition = ref
        .watch(userLocationProvider(LocationIntent.precisePin))
        .asData
        ?.value;
    final items = _getCurrentItemsWithLocations();
    final initialCamera = DiscoverMapHelper.initialCameraForItems(items) ??
        (userPosition != null
            ? CameraPosition(
                target: LatLng(userPosition.latitude, userPosition.longitude),
                zoom: 12,
                tilt: 45,
              )
            : _defaultUSCamera);

    return GoogleMap(
      key: const ValueKey('discover_map'),
      initialCameraPosition: initialCamera,
      mapType: _mapHelper.mapType,
      markers: _mapHelper.markers,
      padding: _mapHelper.padding,
      myLocationEnabled: userPosition != null,
      myLocationButtonEnabled: false,
      compassEnabled: false,
      zoomControlsEnabled: false,
      onMapCreated: _onMapCreated,
      onTap: (LatLng pos) {
        // The dock is persistent chrome (#2634) — tapping the map no
        // longer hides it (that predates the converged dock, from when
        // this screen had its own collapsible header/pill instead).
        if (ref.read(discoverProvider).selectedItem != null) {
          ref.read(discoverProvider.notifier).clearSelectedItem();
          _mapHelper.clearSelection();
        }
      },
    );
  }

  Future<void> _onMapCreated(GoogleMapController controller) async {
    await _mapHelper.onMapCreated(controller);
    if (!mounted) return;

    setState(() {
      _mapHelper.setBottomPadding(_mapBottomClearance());
    });

    await _updateMapMarkers();
  }

  @override
  Widget build(BuildContext context) {
    ref.watch(communitiesProvider);

    // Listen for search state changes to update map markers and style.
    ref.listen<AsyncValue<SearchState>>(searchProvider, (previous, next) {
      final nextState = next.value;
      if (nextState == null) return;

      final resultsChanged = nextState.results != previous?.value?.results;
      final filtersChanged = nextState.filters != previous?.value?.filters;
      final mapStyleChanged =
          nextState.filters.mapStyle != previous?.value?.filters.mapStyle;

      if (_mapHelper.controller != null && mapStyleChanged) {
        // changeStyle handles the JSON-style swap for dark; rebuilding
        // the widget below picks up the new mapType. Skip the camera
        // reposition so the user's view doesn't jump on a style change.
        _mapHelper.changeStyle(nextState.filters.mapStyle).then((_) {
          if (mounted) setState(() {});
        });
        _updateMapMarkers(repositionCamera: false);
      } else if (_mapHelper.controller != null &&
          (resultsChanged || filtersChanged)) {
        if (nextState.results.isEmpty) {
          _mapHelper.clearAllMarkers().then((_) {
            if (mounted) setState(() {});
          });
        } else {
          _updateMapMarkers();
        }
      }
    });

    // Listen for item selection changes to refocus map.
    ref.listen<DiscoverState>(discoverProvider, (previous, next) {
      if (next.selectedItem != null &&
          previous?.selectedItem?.id != next.selectedItem?.id &&
          _mapHelper.controller != null) {
        _mapHelper.focusOnItem(item: next.selectedItem!);
      }
    });

    final selectedItem = ref.watch(discoverProvider).selectedItem;
    final allItems = _getCurrentItemsWithLocations();
    final isNavVisible = ref.watch(homeProvider).isNavVisible;
    final communityState = ref.watch(communitiesProvider);

    if (communityState.communityIds.isEmpty) {
      return Scaffold(
        backgroundColor: AppColors.background(context),
        body: _buildNoCommunityState(context, isNavVisible),
      );
    }

    final searchState = ref.watch(searchProvider).value ?? const SearchState();

    return Scaffold(
      backgroundColor: AppColors.background(context),
      body: Stack(
        children: [
          _buildMapView(),
        // Shelves list view (#2634 v2): painted over the map (which stays
        // mounted so toggling back is instant). It owns its own header as
        // the first scroll child, so the header scrolls off with content.
        if (_showListView)
          Positioned.fill(
            child: _buildShelvesView(context, searchState),
          ),
        if (!_showListView) ...[
          Positioned(
            top: FloatingHeader.contentTop(context) + 44,
            right: 16,
            child: AnimatedSlide(
              duration: accessibleDuration(
                  context, const Duration(milliseconds: 200)),
              offset: isNavVisible ? Offset.zero : const Offset(0, -6),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  _buildMapStyleButton(),
                  const SizedBox(height: 10),
                  _buildLocationButton(),
                ],
              ),
            ),
          ),
          // The map doesn't scroll, so its header pins at the shared
          // destination-header position. The map itself stays full-bleed
          // (#2912 — maps want the space); only the header holds the measure,
          // and it holds the same gallery measure as List mode so the header
          // doesn't jump when toggling between the two.
          Positioned(
            top: MediaQuery.of(context).padding.top,
            left: 0,
            right: 0,
            child: ContentColumn(
              maxWidth: Responsive.galleryMaxWidth,
              child: _buildLibraryHeader(context),
            ),
          ),
        ],
        // The item-card rail floats over the map whenever there are
        // results (#2634 v2 — the results sheet is gone; these cards
        // are the list). Swiping the rail selects and focuses pins.
        if (allItems.isNotEmpty && !_showListView)
          Positioned(
            left: 0,
            right: 0,
            bottom: MediaQuery.of(context).padding.bottom +
                NavDock.bottomContentInset +
                8,
            child: DiscoverGalleryOverlay(
              items: allItems,
              initialIndex: allItems.indexWhere(
                (i) => i.id == selectedItem?.id,
              ).clamp(0, allItems.isEmpty ? 0 : allItems.length - 1),
              distanceMap: {
                for (final i in allItems)
                  if (i is SearchDiscoverItem) i.id: i.distanceMeters,
              },
              onItemTap: (item) {
                final dist = item is SearchDiscoverItem
                    ? item.distanceMeters
                    : null;
                item.when(
                  gear: (g) =>
                      _openGearScreen(g.id, distanceMeters: dist),
                  request: (r) =>
                      _openRequestScreen(r.id, distanceMeters: dist),
                  experience: (e) =>
                      _openExperienceScreen(e.id, distanceMeters: dist),
                );
              },
              onPageChanged: (item, _) =>
                  ref.read(discoverProvider.notifier).selectItem(item),
            ),
          ),
        ],
      ),
    );
  }

  // ─── Library header (#2634 v2): title only (the location), Map/List toggle ───

  Widget _buildLibraryHeader(BuildContext context) {
    final l10n = context.l10n;
    // As a pushed overlay this closes back to the feed; as the Library
    // tab root (#2634) there is nothing to pop, so the slot carries the
    // standard avatar chip instead.
    final canPop = Navigator.of(context).canPop();
    final searchState = ref.watch(searchProvider).value ?? const SearchState();
    final header = DestinationHeader(
      // Dynamic title (Rev 14): Library answers "where" — the distance
      // anchor is the sole headline; ▾ marks it tappable, opening the
      // location sheet. The subtitle summarizes the active result filters
      // (re-homed onto that same sheet) so they're visible at a glance.
      title: _anchorPlace == null
          ? l10n.libraryNearMe
          : l10n.libraryNearPlace(_anchorPlace!.name),
      onTitleTap: () => unawaited(_openLocationSheet()),
      titleSemanticsLabel: l10n.a11yLibraryLocationTitle,
      subtitle: _filterSubtitle(context, searchState),
      onSubtitleTap: () => unawaited(_openLocationSheet()),
      subtitleSemanticsLabel: l10n.a11yLibraryFilterSummary,
      trailing: [
        _buildViewToggle(context),
        if (canPop)
          CloseSearchButton(
            onTap: () => Navigator.of(context).pop(),
            semanticsLabel: context.l10n.commonClose,
          ),
      ],
      showAvatar: !canPop,
    );
    // While a universal-search escape has the tab scoped to a query, a
    // search pill under the header shows what's filtering the map /
    // shelves; its ✕ re-runs the empty-query search (the full library).
    final currentQuery = searchState.currentQuery.trim();
    if (currentQuery.isEmpty) return header;
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        header,
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 0, 20, 10),
          child: SearchScopePill(
            query: currentQuery,
            onClear: _clearSearchScope,
          ),
        ),
      ],
    );
  }

  /// A one-line summary of the active result filters, shown as the
  /// Library header's subtitle so applied filters are visible without
  /// opening the location sheet. Returns null when nothing is filtered
  /// (the default view), so the header stays title-only.
  String? _filterSubtitle(BuildContext context, SearchState state) {
    final l10n = context.l10n;
    final f = state.filters;
    final parts = <String>[];

    // A person filter leads the summary — it's the strongest narrowing.
    final personId = f.selectedPersonId;
    if (personId != null) {
      final name = _selectedPersonName(state, personId);
      if (name != null && name.isNotEmpty) {
        parts.add(l10n.libraryFilterFromPerson(name));
      }
    }

    // Categories only appear when the set is narrowed from the full four.
    final shown = <String>[
      if (f.showRequests) l10n.discoverFilterRequests,
      if (f.showEvents) l10n.discoverFilterEvents,
      if (f.showSharing) l10n.discoverFilterSharing,
      if (f.showGiving) l10n.discoverFilterGiving,
    ];
    if (shown.length < 4) {
      parts.add(
        shown.isEmpty ? l10n.libraryFilterNothingShown : shown.join(', '),
      );
    }

    if (f.includeCompleted) parts.add(l10n.libraryFilterIncludingCompleted);

    if (parts.isEmpty) return null;
    return parts.join(' · ');
  }

  /// First name of the person the results are filtered to, or null when
  /// that member is no longer present in the current results.
  String? _selectedPersonName(SearchState state, String personId) {
    for (final member in state.communityMembers()) {
      if (member.userId == personId) {
        final name = member.displayName.trim();
        if (name.isEmpty) return null;
        return name.split(' ').first;
      }
    }
    return null;
  }

  /// Cancels an active search scope: re-runs the tab's default
  /// empty-query search so the full library returns.
  void _clearSearchScope() {
    final communityIds = ref.read(communitiesProvider).communityIds;
    if (communityIds.isEmpty) return;
    ref.read(searchProvider.notifier).searchWithLocation(
          query: '',
          communityIds: communityIds,
        );
  }

  /// Opens the location + filters sheet and applies the chosen anchor:
  /// the map animates to it, the title updates, and the search re-runs
  /// with the new coordinates so every distance is measured from what
  /// the title says.
  Future<void> _openLocationSheet() async {
    final result = await LibraryLocationSheet.show(
      context,
      currentCommunityId: _anchorCommunityId,
    );
    if (result == null || !mounted) return;

    double? latitude;
    double? longitude;
    if (result.place != null) {
      latitude = result.place!.latitude;
      longitude = result.place!.longitude;
    } else {
      // Back to Near me — the primary user-gesture entry point for
      // location permission alongside the center-on-me button.
      final position = await requestLocationWithSettingsFallback(context, ref);
      if (!mounted) return;
      latitude = position?.latitude;
      longitude = position?.longitude;
    }

    setState(() {
      _anchorPlace = result.place;
      _anchorCommunityId = result.communityId;
    });
    if (latitude == null || longitude == null) return;

    await _mapHelper.controller?.animateCamera(
      CameraUpdate.newCameraPosition(
        CameraPosition(target: LatLng(latitude, longitude), zoom: 13),
      ),
    );
    if (!mounted) return;
    final communityIds = ref.read(communitiesProvider).communityIds;
    if (communityIds.isEmpty) return;
    final currentQuery =
        ref.read(searchProvider).value?.currentQuery ?? '';
    ref.read(searchProvider.notifier).search(
          query: currentQuery,
          communityIds: communityIds,
          latitudeDeg: latitude,
          longitudeDeg: longitude,
        );
  }

  Widget _buildViewToggle(BuildContext context) {
    Widget segment(String label, {required bool listMode}) {
      final active = _showListView == listMode;
      return Toggle(
        semanticsLabel: label,
        selected: active,
        onTap: () => setState(() => _showListView = listMode),
        inkBorderRadius: BorderRadius.circular(999),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 13, vertical: 8),
          decoration: BoxDecoration(
            color: active ? AppColors.primary(context) : Colors.transparent,
            borderRadius: BorderRadius.circular(999),
          ),
          child: Text(
            label,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w600,
              color: active
                  ? Theme.of(context).colorScheme.onPrimary
                  : AppColors.textSecondary(context),
            ),
          ),
        ),
      );
    }

    return Container(
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context),
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: AppColors.border(context)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          segment(context.l10n.libraryToggleList, listMode: true),
          segment(context.l10n.libraryToggleMap, listMode: false),
        ],
      ),
    );
  }

  // ─── Shelves list view (#2634 v2, Candidate B anatomy) ───

  /// Gear grouped into category shelves: biggest shelf first, the
  /// unclassified bucket last. Requests/experiences stay map- and
  /// search-only — the Library's shelves hold things.
  Widget _buildShelvesView(BuildContext context, SearchState searchState) {
    final gearItems = searchState.filteredResults
        .whereType<SearchDiscoverItem>()
        .where((i) =>
            i.searchResult.itemType == SearchItemType.SEARCH_ITEM_TYPE_GEAR)
        .toList();

    final groups = <String, List<SearchDiscoverItem>>{};
    for (final item in gearItems) {
      final category = item.searchResult.gear.category.trim();
      groups.putIfAbsent(category, () => []).add(item);
    }
    final categories = groups.keys.toList()
      ..sort((a, b) {
        if (a.isEmpty != b.isEmpty) return a.isEmpty ? 1 : -1;
        final byCount = groups[b]!.length.compareTo(groups[a]!.length);
        return byCount != 0 ? byCount : a.compareTo(b);
      });

    final topSafe = MediaQuery.of(context).padding.top;
    final bottomClearance =
        MediaQuery.of(context).padding.bottom + NavDock.bottomContentInset + 12;

    final header = _buildLibraryHeader(context);

    // Desktop measure (#2912, retuned by #2926): the header holds the gallery
    // column and each shelf's CONTENT inset aligns with that column's left
    // edge, while the shelf's scroll region stays window-wide — an outer width
    // constraint would shrink the horizontal ListView's RenderBox and clip
    // tiles at the measure edge during drag/overscroll (the same trap as the
    // outer Padding, documented below), and scrolling tiles off the left edge
    // into the gutter is wanted, not a bug. The gallery measure, not the
    // reading measure: shelves lay out 150px tiles, so line length is not what
    // should bound them, and 640 left a void down the left of a wide window.
    // At phone widths this is the plain 20px.
    final shelfInset = Responsive.measureInset(
      MediaQuery.sizeOf(context).width,
      measure: Responsive.galleryMaxWidth,
    );

    return ColoredBox(
      color: AppColors.background(context),
      child: gearItems.isEmpty
          ? Column(
              children: [
                SizedBox(height: topSafe),
                ContentColumn(
                  maxWidth: Responsive.galleryMaxWidth,
                  child: header,
                ),
                Expanded(
                  child: Center(
                    child: Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 32),
                      child: Text(
                        context.l10n.discoverNothingHereYet,
                        textAlign: TextAlign.center,
                        style: TextStyle(
                          fontFamily: AppTheme.headingFont,
                          fontSize: 17,
                          color: AppColors.textSecondary(context),
                        ),
                      ),
                    ),
                  ),
                ),
              ],
            )
          : ListView(
              padding: EdgeInsets.fromLTRB(0, topSafe, 0, bottomClearance),
              children: [
                // First scroll child: the header scrolls off with the
                // shelves rather than pinning above them. It holds the
                // gallery measure so its title lines up with the shelf
                // titles below it (#2926).
                ContentColumn(
                  maxWidth: Responsive.galleryMaxWidth,
                  child: header,
                ),
                // No outer Padding here: the 20px inset is applied via
                // each shelf's own leading padding instead (title and
                // the horizontal ListView's `padding`). An external
                // Padding would shrink the shelf ListView's RenderBox
                // (and thus its clip rect) to start flush with the
                // first tile's rounded corner, so any drag or
                // iOS-style overscroll bounce clips straight through
                // that corner — the tile visibly ducks under the
                // margin instead of sliding freely above it.
                Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    for (final category in categories)
                      _buildShelf(context, category, groups[category]!,
                          inset: shelfInset),
                  ],
                ),
              ],
            ),
    );
  }

  Widget _buildShelf(
    BuildContext context,
    String category,
    List<SearchDiscoverItem> items, {
    // The measure-aligned content inset (#2912, #2926) — 20 at phone widths;
    // on a desktop-wide window it grows so titles and first tiles line up
    // with the header's gallery column while the shelf scrolls the full
    // window.
    double inset = Responsive.baseInset,
  }) {
    final title =
        category.isEmpty ? context.l10n.libraryShelfMore : category;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: EdgeInsets.fromLTRB(inset, 14, inset, 9),
          child: Text(
            '${title.toUpperCase()} · ${items.length}',
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w700,
              letterSpacing: 1.1,
              color: AppColors.textSecondary(context),
            ),
          ),
        ),
        SizedBox(
          height: 144,
          child: ListView(
            scrollDirection: Axis.horizontal,
            // Left inset lives here, not in an ancestor Padding — see
            // the comment in `_buildShelvesView`.
            padding: EdgeInsets.only(left: inset, right: inset),
            children: [
              for (final item in items) ...[
                LibraryShelfTile(
                  item: item,
                  onTap: () => _openGearScreen(
                    item.id,
                    distanceMeters: item.distanceMeters,
                  ),
                ),
                const SizedBox(width: 10),
              ],
              LibraryShelfGhostTile(
                onTap: () =>
                    unawaited(UnifiedCreateModal.show(context, ref)),
              ),
            ],
          ),
        ),
      ],
    );
  }

}
