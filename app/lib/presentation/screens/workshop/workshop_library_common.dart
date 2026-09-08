import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart'
    show DayForecast, HomeUpNextEntry, OpenDaySuggestion;
import 'package:ripls/data/gen/ripls/api/search_service.pb.dart'
    show SearchItemType, SearchResultItem;
import 'package:ripls/presentation/models/discover_item.dart';
import 'package:ripls/presentation/providers/user_timezone_provider.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_overview_theme.dart';
import 'package:ripls/services/providers/feed_providers.dart';
import 'package:ripls/services/providers/search_providers.dart';

/// Family key for [workshopLibraryLocatedItemsProvider] — the in-scope
/// communities (comma-joined) plus an optional category [query] (the
/// "Common Ground" filter; empty = everything).
typedef WorkshopLocatedKey = ({String idsKey, String query});

/// Located community-library entries — the gear / requests / experiences that
/// carry coordinates — for the real "Where" map. The brief's `Item` rows have
/// no coordinates, so the map sources its pins from the unified search scoped
/// to the in-scope communities, mirroring how Discover plots a community's
/// located items. The optional [query] applies the Common-Ground category
/// filter (empty = browse everything). Cached by the SearchRepository.
final workshopLibraryLocatedItemsProvider = FutureProvider.autoDispose
    .family<List<SearchResultItem>, WorkshopLocatedKey>((ref, key) async {
  final ids = key.idsKey.split(',').where((s) => s.isNotEmpty).toList();
  if (ids.isEmpty) return const <SearchResultItem>[];
  return ref.read(searchRepositoryProvider).search(
    query: key.query,
    communityIds: ids,
    latitudeDeg: 0,
    longitudeDeg: 0,
    itemTypes: const [
      SearchItemType.SEARCH_ITEM_TYPE_GEAR,
      SearchItemType.SEARCH_ITEM_TYPE_REQUEST,
      SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE,
    ],
  );
});

/// Joins community ids into the comma-separated key used by the workshop
/// library providers (located items, calendar).
String workshopLibraryItemsKey(List<String> communityIds) =>
    communityIds.join(',');

/// The community calendar data — the **same** server-assembled `GetHomeView`
/// the inbox calendar uses (events on their scheduled date; requests on their due /
/// first-shared date; gear on its first-shared date; loan pickups & due-backs),
/// with the timeline filtered to the in-scope communities, plus the per-day
/// weather and open-day suggestions (which are the viewer's, not
/// community-scoped). The only difference from the inbox calendar is the entry
/// filter — see `docs/client/calendar.md`.
///
/// Reuses the cached `GetHomeView` read so it stays in lock-step with the inbox
/// (no separate fetch, no re-implemented date logic). Keyed by the comma-joined
/// community ids.
typedef WorkshopCalendarData = ({
  List<HomeUpNextEntry> entries,
  List<DayForecast> forecast,
  List<OpenDaySuggestion> suggestions,
});

final workshopCommunityCalendarProvider = FutureProvider.autoDispose
    .family<WorkshopCalendarData, String>((ref, idsKey) async {
  final ids = idsKey.split(',').where((s) => s.isNotEmpty).toSet();
  final tz = await ref.watch(resolvedTimezoneProvider.future);
  final view = await ref.watch(portfolioRepositoryProvider).getHomeView(tz);
  // An item can belong to several communities (its real community plus its
  // nameless ad-hoc per-item community, #2492). Match on the entry's full
  // community_ids set so an item shared into this community appears here even
  // when its display community_id is a different one (#2675). Fall back to the
  // single community_id for entries from an older server that omits the set.
  final entries = ids.isEmpty
      ? const <HomeUpNextEntry>[]
      : [
          for (final e in view.calendar)
            if (e.communityIds.isNotEmpty
                ? e.communityIds.any(ids.contains)
                : ids.contains(e.communityId))
              e,
        ];
  return (
    entries: entries,
    forecast: view.forecast,
    suggestions: view.openDaySuggestions,
  );
});

/// Owner first name for a located entry, or null when unresolvable.
String? workshopEntryOwner(SearchResultItem? located) {
  if (located == null) return null;
  final name = _firstName(SearchDiscoverItem(located).user.name);
  return name.isEmpty ? null : name;
}

/// The located entry's location id, for resolving a short location name via
/// `locationDisplayProvider`. Null when the entry has no location.
String? workshopEntryLocationId(SearchResultItem? located) {
  if (located == null) return null;
  switch (located.itemType) {
    case SearchItemType.SEARCH_ITEM_TYPE_GEAR:
      return located.gear.locationId;
    case SearchItemType.SEARCH_ITEM_TYPE_REQUEST:
      return located.request.locationId;
    case SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE:
      return located.experience.locationId;
    default:
      return null;
  }
}

/// Joins the non-empty meta parts with " · ", or null when all are empty.
String? workshopJoinMeta(List<String?> parts) {
  final kept = [
    for (final p in parts)
      if (p != null && p.isNotEmpty) p,
  ];
  return kept.isEmpty ? null : kept.join(' · ');
}

/// First whitespace-delimited token of a display name, trimmed.
String _firstName(String fullName) {
  final trimmed = fullName.trim();
  final space = trimmed.indexOf(RegExp(r'\s'));
  return space < 0 ? trimmed : trimmed.substring(0, space);
}

/// A vertically-centered scroll for the library destinations: centers its
/// [children] when they're shorter than the viewport, scrolls when taller —
/// so a short list sits centered, matching the overview's editorial feel.
class WorkshopCenteredList extends StatelessWidget {
  final List<Widget> children;
  final double bottomPadding;

  const WorkshopCenteredList({
    super.key,
    required this.children,
    this.bottomPadding = 0,
  });

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (ctx, c) {
        final minHeight =
            (c.maxHeight - bottomPadding).clamp(0.0, double.infinity);
        return SingleChildScrollView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: EdgeInsets.only(bottom: bottomPadding),
          child: ConstrainedBox(
            constraints: BoxConstraints(minHeight: minHeight),
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: children,
            ),
          ),
        );
      },
    );
  }
}

/// Shared empty / loading / error states for the library destinations.
class WorkshopLibraryStates {
  const WorkshopLibraryStates._();

  static Widget loading() => const Center(
        child: CircularProgressIndicator(
          color: WorkshopOverviewPalette.onPhoto,
        ),
      );

  static Widget error(Object e) => Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Text(
            e.toString(),
            textAlign: TextAlign.center,
            style: const TextStyle(color: WorkshopOverviewPalette.onPhotoDim),
          ),
        ),
      );

  static Widget empty(BuildContext context) => Center(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                context.l10n.workshopLibraryEmptyTitle,
                style: const TextStyle(
                  fontFamily: AppTheme.headingFont,
                  fontSize: 18,
                  fontWeight: FontWeight.w600,
                  color: WorkshopOverviewPalette.onPhoto,
                ),
              ),
              const SizedBox(height: 10),
              Text(
                context.l10n.workshopLibraryEmptySubtitle,
                textAlign: TextAlign.center,
                style: const TextStyle(
                  fontSize: 13,
                  height: 1.45,
                  color: WorkshopOverviewPalette.onPhotoDim,
                ),
              ),
            ],
          ),
        ),
      );
}
