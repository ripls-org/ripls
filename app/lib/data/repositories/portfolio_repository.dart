import 'package:flutter/foundation.dart' show VoidCallback;
import 'package:ripls/data/cache/cache_manager.dart';
import 'package:ripls/data/cache/cache_service.dart';
import 'package:ripls/services/portfolio_service.dart';

export 'package:ripls/services/portfolio_service.dart'
    show
        GetDirectoryPeopleResponse,
        GetPortfolioMetricsResponse,
        GetHomeViewResponse,
        HomeDecision,
        HomeUpNextEntry,
        HomeAsk,
        HomeOwnedEvent,
        HomeGearItem,
        HomeGearCounts,
        HomeActivityEntry,
        HomeDecisionKind,
        HomeTransferAction,
        HomeUpNextKind,
        HomeGearCategory,
        HomeGearPillStyle,
        SetWeeklyGoalsResponse,
        PortfolioMetricsSection,
        PortfolioPerspective,
        DailyPerson,
        DailyItemType,
        IsWatchedResponse;

/// PortfolioRepository manages personal cross-community portfolio data with caching.
class PortfolioRepository {
  final PortfolioService _service;
  final CacheService _cache;

  /// Called after any mutation that changes the inbox contents (watch/unwatch).
  /// Allows the inbox view model to refresh without polling.
  final VoidCallback? onWatchMutated;

  PortfolioRepository(
    CacheManager cacheManager,
    this._service, {
    this.onWatchMutated,
  }) : _cache = CacheService(cacheManager, 'portfolio');

  /// getDirectoryPeople retrieves the Directory's people rows with caching.
  ///
  /// Cache key: 'portfolio:directory_people'.
  Future<GetDirectoryPeopleResponse> getDirectoryPeople() {
    return _cache.get(
      key: 'directory_people',
      fetch: () => _service.getDirectoryPeople(),
    );
  }

  /// getPortfolioMetrics retrieves lifetime impact metrics with caching,
  /// scoped to [communityIds] when provided.
  ///
  /// Cache key: 'portfolio:metrics:{sorted community IDs}'.
  Future<GetPortfolioMetricsResponse> getPortfolioMetrics({
    List<String> communityIds = const [],
  }) {
    final sorted = [...communityIds]..sort();
    final key = sorted.isEmpty ? 'metrics' : 'metrics:${sorted.join(',')}';
    return _cache.get(
      key: key,
      fetch: () => _service.getPortfolioMetrics(communityIds: communityIds),
    );
  }

  /// refreshInboxFeed invalidates the caches every inbox mutation affects.
  ///
  /// The inbox-feed and my-stuff entries it also cleared are gone with their
  /// RPCs (#2830); Home is what those mutations actually surface on now. The
  /// name is kept because every mutation path calls it.
  Future<void> refreshInboxFeed() async {
    await _cache.invalidate('home_view');
  }

  /// getHomeView retrieves the sectioned Home tab view (#2435) with caching.
  ///
  /// Cache key: 'portfolio:home_view'.
  Future<GetHomeViewResponse> getHomeView(String timezone) {
    return _cache.get(
      key: 'home_view',
      fetch: () => _service.getHomeView(timezone),
    );
  }

  /// refreshHomeView invalidates the Home view cache so the next call
  /// fetches from the server.
  Future<void> refreshHomeView() async {
    await _cache.invalidate('home_view');
  }

  /// setWeeklyGoals saves the user's personal weekly impact goals.
  ///
  /// After saving, invalidates Home so the next fetch reflects the new goals.
  Future<SetWeeklyGoalsResponse> setWeeklyGoals(
    int socialTimeMinutesGoal,
    int moneySavedCentsGoal,
    int co2AvoidedGramsGoal,
  ) async {
    final response = await _service.setWeeklyGoals(
      socialTimeMinutesGoal,
      moneySavedCentsGoal,
      co2AvoidedGramsGoal,
    );
    await _cache.invalidate('home_view');
    return response;
  }

  /// markInboxItemRead marks a watched item as read and invalidates the inbox
  /// Home view so the next fetch reflects the updated read state.
  Future<void> markInboxItemRead({
    required DailyItemType itemType,
    required String itemId,
  }) async {
    await _service.markInboxItemRead(itemType: itemType, itemId: itemId);
    await refreshInboxFeed();
  }

  /// watchItem manually adds an item to the user's watch list, reactivating
  /// it if previously dismissed. Invalidates the Home view and notifies the
  /// view model to refresh immediately.
  Future<void> watchItem({
    required DailyItemType itemType,
    required String itemId,
  }) async {
    await _service.watchItem(itemType: itemType, itemId: itemId);
    await refreshInboxFeed();
    onWatchMutated?.call();
  }

  /// dismissInboxItem soft-deletes the watch entry, removing the item from
  /// the user's inbox. Invalidates the Home view and notifies the view model
  /// to refresh immediately.
  Future<void> dismissInboxItem({
    required DailyItemType itemType,
    required String itemId,
  }) async {
    await _service.dismissInboxItem(itemType: itemType, itemId: itemId);
    await refreshInboxFeed();
    onWatchMutated?.call();
  }

  /// isWatched returns whether the user has an active watch entry for the item.
  Future<bool> isWatched({
    required DailyItemType itemType,
    required String itemId,
  }) async {
    final response =
        await _service.isWatched(itemType: itemType, itemId: itemId);
    return response.isWatched;
  }

  /// refreshMetrics invalidates the metrics cache so the next call fetches from the server.
  Future<void> refreshMetrics() async {
    await _cache.invalidate('metrics');
  }

}
