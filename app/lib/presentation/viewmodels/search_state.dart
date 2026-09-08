import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/presentation/models/discover_item.dart';

part 'search_state.freezed.dart';

/// Meters per mile conversion constant.
const _metersPerMile = 1609.34;

/// Sort order for discover search results.
enum DiscoverSortBy { relevancy, nearest, newest }

/// Scope chip state for grouped search results (#2435).
///
/// [none] is the default: results render grouped — the viewer's own items
/// pinned first ("Yours"), then everything else ("From your communities").
/// [yours] filters to the viewer's items only; [everything] renders the
/// explicit ungrouped flat list.
enum SearchScope { none, yours, everything }

/// Returns the creation timestamp (Unix seconds) for [item], used for newest sort.
int _itemCreatedAt(DiscoverItem item) {
  if (item is! SearchDiscoverItem) return 0;
  switch (item.searchResult.itemType) {
    case SearchItemType.SEARCH_ITEM_TYPE_REQUEST:
      return item.searchResult.request.createdAtUnixSec.toInt();
    case SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE:
      return item.searchResult.experience.createdAtUnixSec.toInt();
    default:
      return 0;
  }
}

/// Filter state for discover search.
@freezed
sealed class DiscoverFilters with _$DiscoverFilters {
  const factory DiscoverFilters({
    /// When true, include completed/fulfilled/cancelled items.
    @Default(false) bool includeCompleted,

    /// When set, show only items belonging to this user.
    String? selectedPersonId,

    /// When false, hide request items from results.
    @Default(true) bool showRequests,

    /// When false, hide event (experience) items from results.
    @Default(true) bool showEvents,

    /// When false, hide loan-available gear items from results.
    @Default(true) bool showSharing,

    /// When false, hide giveaway gear items from results.
    @Default(true) bool showGiving,

    /// Maximum distance in miles; null means "Any" (no distance filter).
    double? maxDistanceMiles,

    /// Sort order for results.
    @Default(DiscoverSortBy.relevancy) DiscoverSortBy sortBy,

    /// Map style identifier; one of the mapStyle* constants in
    /// discover_map_helper.dart (standard / outdoor / satellite / dark).
    /// Defaults to 'standard'.
    @Default('standard') String mapStyle,
  }) = _DiscoverFilters;

  const DiscoverFilters._();

  /// Returns the number of active non-default filters.
  ///
  /// Counts deselected categories (0–4), plus one each for includeCompleted,
  /// a distance cap, and a non-relevancy sort order.
  int get activeFilterCount =>
      (!showRequests ? 1 : 0) +
      (!showEvents ? 1 : 0) +
      (!showSharing ? 1 : 0) +
      (!showGiving ? 1 : 0) +
      (includeCompleted ? 1 : 0) +
      (maxDistanceMiles != null ? 1 : 0) +
      (sortBy != DiscoverSortBy.relevancy ? 1 : 0);
}

/// A community member extracted from search results for the avatar row.
///
/// Holds the raw [User] proto so that [UserAvatar] can resolve the media URL
/// directly from [user.mediaId] without any field extraction that could lose data.
class CommunityMember {
  final User user;

  const CommunityMember({required this.user});

  String get userId => user.id;
  String get displayName => user.name;
}

/// State for the search functionality.
@freezed
sealed class SearchState with _$SearchState {
  const factory SearchState({
    @Default([]) List<DiscoverItem> results,
    @Default(false) bool isLoading,
    UserError? error,
    @Default('') String currentQuery,
    @Default(<String>[]) List<String> currentCommunityIds,
    double? currentLatitude,
    double? currentLongitude,
    @Default(0) int refreshCount,
    @Default(DiscoverFilters()) DiscoverFilters filters,
    /// Restricts the server search to a subset of item types. Empty
    /// means "all default types" (gear + requests + experiences +
    /// users). Set non-empty by browse-mode entry points such as the
    /// People in Circles suggestion chip; typing a new keyword query
    /// resets this back to empty.
    @Default(<SearchItemType>[]) List<SearchItemType> currentItemTypes,
    /// When non-null, identifies the community the caller is currently
    /// "browsing" (community carousel tap). Drives the results-header
    /// copy: "Results for {communityName}" instead of the empty-query
    /// fallback. Cleared by typed keyword searches and the other
    /// browse-mode entry points.
    String? currentBrowseCommunityId,

    /// Active results scope (#2435). Default none = grouped "Yours" pinned
    /// then "From your communities". Every new search resets this — quick
    /// search chips run canned queries and clear any active scope.
    @Default(SearchScope.none) SearchScope scope,
  }) = _SearchState;

  const SearchState._();

  /// Returns whether the state has an error.
  bool get hasError => error != null;

  /// Returns whether the list is empty (and not loading).
  bool get isEmpty => results.isEmpty && !isLoading;

  /// Returns only items that have valid locations for map display.
  ///
  /// Uses [sortedResults] so map markers respect active filters.
  List<DiscoverItem> get itemsWithLocations {
    return sortedResults.where((item) {
      if (item is SearchDiscoverItem) {
        return item.latitudeDeg != 0 && item.longitudeDeg != 0;
      } else if (item is GearDiscoverItem) {
        return item.gear.latitudeDeg != 0 && item.gear.longitudeDeg != 0;
      } else if (item is RequestDiscoverItem) {
        return item.latitudeDeg != 0 && item.longitudeDeg != 0;
      } else if (item is ExperienceDiscoverItem) {
        return item.latitudeDeg != 0 && item.longitudeDeg != 0;
      }
      return false;
    }).toList();
  }

  /// Returns results filtered by selected person (if any).
  List<DiscoverItem> get _personFiltered {
    final personId = filters.selectedPersonId;
    if (personId == null) return results;
    return results.where((item) {
      if (item is SearchDiscoverItem) {
        return item.user.id == personId;
      }
      return false;
    }).toList();
  }

  /// Returns results filtered by person, category, and distance.
  ///
  /// Category filters use [DiscoverFilters.showRequests], [showEvents],
  /// [showSharing], and [showGiving].  Sharing maps to
  /// [Availability.AVAILABILITY_FOR_LOAN]; Giving maps to
  /// [Availability.AVAILABILITY_FOR_GIVEAWAY].  Distance is capped at
  /// [DiscoverFilters.maxDistanceMiles] when set.
  List<DiscoverItem> get categoryFilteredResults {
    final f = filters;
    return _personFiltered.where((item) {
      if (item is! SearchDiscoverItem) return true;
      switch (item.searchResult.itemType) {
        case SearchItemType.SEARCH_ITEM_TYPE_REQUEST:
          if (!f.showRequests) return false;
        case SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE:
          if (!f.showEvents) return false;
        case SearchItemType.SEARCH_ITEM_TYPE_GEAR:
          final avail = item.searchResult.gear.availability;
          if (avail == Availability.AVAILABILITY_FOR_GIVEAWAY) {
            if (!f.showGiving) return false;
          } else {
            if (!f.showSharing) return false;
          }
        default:
          break;
      }
      final maxMiles = f.maxDistanceMiles;
      if (maxMiles != null) {
        final dist = item.distanceMeters;
        if (dist != null && dist > maxMiles * _metersPerMile) return false;
      }
      return true;
    }).toList();
  }

  /// Returns [categoryFilteredResults] sorted by [DiscoverFilters.sortBy].
  ///
  /// Relevancy preserves server order (composite score descending).
  /// Nearest sorts by [SearchDiscoverItem.distanceMeters] ascending.
  /// Newest sorts by item creation timestamp descending.
  List<DiscoverItem> get sortedResults {
    final items = List<DiscoverItem>.from(categoryFilteredResults);
    switch (filters.sortBy) {
      case DiscoverSortBy.relevancy:
        return items;
      case DiscoverSortBy.nearest:
        items.sort((a, b) {
          final da = a is SearchDiscoverItem
              ? (a.distanceMeters ?? double.infinity)
              : double.infinity;
          final db = b is SearchDiscoverItem
              ? (b.distanceMeters ?? double.infinity)
              : double.infinity;
          return da.compareTo(db);
        });
        return items;
      case DiscoverSortBy.newest:
        items.sort((a, b) => _itemCreatedAt(b).compareTo(_itemCreatedAt(a)));
        return items;
    }
  }

  /// Returns all filtered and sorted results for map markers and list display.
  List<DiscoverItem> get filteredResults => sortedResults;

  /// Returns request items, filtered and sorted.
  List<SearchDiscoverItem> get requests {
    return sortedResults
        .whereType<SearchDiscoverItem>()
        .where(
          (item) =>
              item.searchResult.itemType ==
              SearchItemType.SEARCH_ITEM_TYPE_REQUEST,
        )
        .toList();
  }

  /// Returns event items, filtered and sorted.
  List<SearchDiscoverItem> get events {
    return sortedResults
        .whereType<SearchDiscoverItem>()
        .where(
          (item) =>
              item.searchResult.itemType ==
              SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE,
        )
        .toList();
  }

  /// Returns gear items, filtered and sorted.
  List<SearchDiscoverItem> get sharing {
    return sortedResults
        .whereType<SearchDiscoverItem>()
        .where(
          (item) =>
              item.searchResult.itemType ==
              SearchItemType.SEARCH_ITEM_TYPE_GEAR,
        )
        .toList();
  }

  
  
  
  /// Returns deduplicated community members extracted from search results.
  ///
  /// Places the current user at index 0 (if present in results), so they can
  /// tap their own avatar to filter to their own items. Other members follow
  /// in order of first appearance (most recently shared item first).
  List<CommunityMember> communityMembers({String? currentUserId}) {
    final seen = <String>{};
    CommunityMember? currentMember;
    final others = <CommunityMember>[];
    for (final item in results) {
      if (item is SearchDiscoverItem) {
        final user = item.user;
        if (user.id.isEmpty) continue;
        if (seen.contains(user.id)) continue;
        seen.add(user.id);
        final member = CommunityMember(user: user);
        if (user.id == currentUserId) {
          currentMember = member;
        } else {
          others.add(member);
        }
      }
    }
    return [?currentMember, ...others];
  }
}
