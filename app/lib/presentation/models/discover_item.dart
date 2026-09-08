import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart' show Experience;
import 'package:ripls/data/gen/ripls/api/request.pb.dart' show Request;
import 'package:ripls/data/gen/ripls/api/search_service.pb.dart'
    show SearchItemType, SearchResultItem;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;

/// Base class for items displayed in the discover view.
///
/// This is a sealed class (via factory pattern) that represents a
/// gear item, request item, or experience item. Using a sealed class
/// provides type safety and exhaustive pattern matching instead of
/// using `dynamic`.
sealed class DiscoverItem {
  const DiscoverItem();

  /// Gets the unique ID for this item
  String get id;

  /// Gets the media ID for this item (may be empty)
  String get primaryMediaId;

  /// Gets the media ID to use for thumbnail display (may be empty)
  /// This is the same as primaryMediaId but named for clarity in UI code
  String get thumbnailMediaId;

  /// Gets the display name for this item (gear name or request title)
  String get name;

  /// Gets the owner/requester user for this item
  User get user;

  /// Gets the name of the owner/requester
  String get ownerName;

  /// Pattern matching helper - execute different code based on item type.
  /// `user` is optional so existing callers that only handle the three
  /// item types stay source-compatible; it is required only when the
  /// concrete item is a [SearchDiscoverItem] carrying a USER result.
  T when<T>({
    required T Function(CommunityGearItem gear) gear,
    required T Function(Request request) request,
    required T Function(Experience experience) experience,
    T Function(User user)? user,
  });
}

/// A gear item in the discover view
class GearDiscoverItem extends DiscoverItem {
  final CommunityGearItem gear;

  const GearDiscoverItem(this.gear);

  @override
  String get id => gear.id;

  @override
  String get primaryMediaId =>
      gear.mediaIds.isNotEmpty ? gear.mediaIds.first : '';

  @override
  String get thumbnailMediaId => primaryMediaId;

  @override
  String get name => gear.name;

  @override
  User get user => gear.owner;

  @override
  String get ownerName => gear.owner.name;

  @override
  T when<T>({
    required T Function(CommunityGearItem gear) gear,
    required T Function(Request request) request,
    required T Function(Experience experience) experience,
    T Function(User user)? user,
  }) {
    return gear(this.gear);
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is GearDiscoverItem &&
          runtimeType == other.runtimeType &&
          gear.id == other.gear.id;

  @override
  int get hashCode => gear.id.hashCode;
}

/// A request item in the discover view
class RequestDiscoverItem extends DiscoverItem {
  final Request request;

  const RequestDiscoverItem(this.request);

  @override
  String get id => request.id;

  @override
  String get primaryMediaId =>
      request.mediaIds.isNotEmpty ? request.mediaIds.first : '';

  @override
  String get thumbnailMediaId => primaryMediaId;

  @override
  String get name => request.title;

  @override
  User get user => request.requester;

  @override
  String get ownerName => request.requester.name;

  /// Gets the latitude for map display
  double get latitudeDeg => request.latitudeDeg;

  /// Gets the longitude for map display
  double get longitudeDeg => request.longitudeDeg;

  @override
  T when<T>({
    required T Function(CommunityGearItem gear) gear,
    required T Function(Request request) request,
    required T Function(Experience experience) experience,
    T Function(User user)? user,
  }) {
    return request(this.request);
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is RequestDiscoverItem &&
          runtimeType == other.runtimeType &&
          request.id == other.request.id;

  @override
  int get hashCode => request.id.hashCode;
}

/// An experience item in the discover view
class ExperienceDiscoverItem extends DiscoverItem {
  final Experience experience;

  const ExperienceDiscoverItem(this.experience);

  @override
  String get id => experience.id;

  @override
  String get primaryMediaId =>
      experience.mediaIds.isNotEmpty ? experience.mediaIds.first : '';

  @override
  String get thumbnailMediaId => primaryMediaId;

  @override
  String get name => experience.name;

  @override
  User get user => experience.owner;

  @override
  String get ownerName => experience.owner.name;

  /// Gets the latitude for map display
  double get latitudeDeg => experience.latitudeDeg;

  /// Gets the longitude for map display
  double get longitudeDeg => experience.longitudeDeg;

  @override
  T when<T>({
    required T Function(CommunityGearItem gear) gear,
    required T Function(Request request) request,
    required T Function(Experience experience) experience,
    T Function(User user)? user,
  }) {
    return experience(this.experience);
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is ExperienceDiscoverItem &&
          runtimeType == other.runtimeType &&
          experience.id == other.experience.id;

  @override
  int get hashCode => experience.id.hashCode;
}

/// A search result item in the discover view.
///
/// This wraps SearchResultItem from the unified search API and provides
/// access to the underlying gear, request, or experience data along with
/// search scores (composite score, semantic similarity, distance).
class SearchDiscoverItem extends DiscoverItem {
  final SearchResultItem searchResult;

  const SearchDiscoverItem(this.searchResult);

  @override
  String get id {
    switch (searchResult.itemType) {
      case SearchItemType.SEARCH_ITEM_TYPE_GEAR:
        return searchResult.gear.id;
      case SearchItemType.SEARCH_ITEM_TYPE_REQUEST:
        return searchResult.request.id;
      case SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE:
        return searchResult.experience.id;
      case SearchItemType.SEARCH_ITEM_TYPE_USER:
        return searchResult.user.id;
      default:
        return '';
    }
  }

  @override
  String get primaryMediaId {
    switch (searchResult.itemType) {
      case SearchItemType.SEARCH_ITEM_TYPE_GEAR:
        final mediaIds = searchResult.gear.mediaIds;
        return mediaIds.isNotEmpty ? mediaIds.first : '';
      case SearchItemType.SEARCH_ITEM_TYPE_REQUEST:
        final mediaIds = searchResult.request.mediaIds;
        return mediaIds.isNotEmpty ? mediaIds.first : '';
      case SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE:
        final mediaIds = searchResult.experience.mediaIds;
        return mediaIds.isNotEmpty ? mediaIds.first : '';
      case SearchItemType.SEARCH_ITEM_TYPE_USER:
        return searchResult.user.mediaId;
      default:
        return '';
    }
  }

  @override
  String get thumbnailMediaId => primaryMediaId;

  @override
  String get name {
    switch (searchResult.itemType) {
      case SearchItemType.SEARCH_ITEM_TYPE_GEAR:
        return searchResult.gear.name;
      case SearchItemType.SEARCH_ITEM_TYPE_REQUEST:
        return searchResult.request.title;
      case SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE:
        return searchResult.experience.name;
      case SearchItemType.SEARCH_ITEM_TYPE_USER:
        return searchResult.user.name;
      default:
        return '';
    }
  }

  @override
  User get user {
    switch (searchResult.itemType) {
      case SearchItemType.SEARCH_ITEM_TYPE_GEAR:
        return searchResult.gear.owner;
      case SearchItemType.SEARCH_ITEM_TYPE_REQUEST:
        return searchResult.request.requester;
      case SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE:
        return searchResult.experience.owner;
      case SearchItemType.SEARCH_ITEM_TYPE_USER:
        return searchResult.user;
      default:
        return User();
    }
  }

  @override
  String get ownerName => user.name;

  /// Gets the latitude for map display
  double get latitudeDeg {
    switch (searchResult.itemType) {
      case SearchItemType.SEARCH_ITEM_TYPE_GEAR:
        return searchResult.gear.latitudeDeg;
      case SearchItemType.SEARCH_ITEM_TYPE_REQUEST:
        return searchResult.request.latitudeDeg;
      case SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE:
        return searchResult.experience.latitudeDeg;
      default:
        return 0;
    }
  }

  /// Gets the longitude for map display
  double get longitudeDeg {
    switch (searchResult.itemType) {
      case SearchItemType.SEARCH_ITEM_TYPE_GEAR:
        return searchResult.gear.longitudeDeg;
      case SearchItemType.SEARCH_ITEM_TYPE_REQUEST:
        return searchResult.request.longitudeDeg;
      case SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE:
        return searchResult.experience.longitudeDeg;
      default:
        return 0;
    }
  }

  /// Gets the distance in meters from the user's location
  double? get distanceMeters =>
      searchResult.distanceMeters > 0 ? searchResult.distanceMeters : null;

  /// Gets the composite relevance score (0.0 to 1.0)
  double get compositeScore => searchResult.compositeScore;

  /// Gets the semantic similarity score (0.0 to 1.0)
  double get semanticSimilarity => searchResult.semanticSimilarity;

  /// Gets the item type for filtering
  SearchItemType get itemType => searchResult.itemType;

  
  /// Returns true if this is a request item
  bool get isRequest =>
      searchResult.itemType == SearchItemType.SEARCH_ITEM_TYPE_REQUEST;

  /// Returns true if this is an experience item
  bool get isExperience =>
      searchResult.itemType == SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE;

  
  @override
  T when<T>({
    required T Function(CommunityGearItem gear) gear,
    required T Function(Request request) request,
    required T Function(Experience experience) experience,
    T Function(User user)? user,
  }) {
    switch (searchResult.itemType) {
      case SearchItemType.SEARCH_ITEM_TYPE_GEAR:
        // Convert Gear to CommunityGearItem for the callback
        final communityGear = CommunityGearItem(
          id: searchResult.gear.id,
          name: searchResult.gear.name,
          description: searchResult.gear.description,
          owner: searchResult.gear.owner,
          availability: searchResult.gear.availability,
          mediaIds: searchResult.gear.mediaIds,
          locationId: searchResult.gear.locationId,
          latitudeDeg: searchResult.gear.latitudeDeg,
          longitudeDeg: searchResult.gear.longitudeDeg,
        );
        return gear(communityGear);
      case SearchItemType.SEARCH_ITEM_TYPE_REQUEST:
        return request(searchResult.request);
      case SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE:
        return experience(searchResult.experience);
      case SearchItemType.SEARCH_ITEM_TYPE_USER:
        if (user == null) {
          throw StateError(
            'SearchDiscoverItem.when called on USER result without a `user` callback',
          );
        }
        return user(searchResult.user);
      default:
        throw StateError('Unknown search item type: ${searchResult.itemType}');
    }
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is SearchDiscoverItem &&
          runtimeType == other.runtimeType &&
          id == other.id &&
          searchResult.itemType == other.searchResult.itemType;

  @override
  int get hashCode => Object.hash(id, searchResult.itemType);
}
