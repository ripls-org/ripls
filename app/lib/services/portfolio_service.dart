import 'package:connectrpc/connect.dart' as connect;
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';

export 'package:ripls/data/gen/ripls/api/portfolio.pb.dart'
    show
        GetPortfolioMetricsRequest,
        GetPortfolioMetricsResponse,
        GetDirectoryPeopleRequest,
        GetDirectoryPeopleResponse,
        GetHomeViewRequest,
        GetHomeViewResponse,
        HomeDecision,
        HomeUpNextEntry,
        HomeAsk,
        HomeOwnedEvent,
        HomeGearItem,
        HomeGearCounts,
        HomeActivityEntry,
        SetWeeklyGoalsRequest,
        SetWeeklyGoalsResponse,
        PortfolioMetricsSection,
        DailyPerson;

export 'package:ripls/data/gen/ripls/api/portfolio.pb.dart'
    show
        MarkInboxItemReadRequest,
        MarkInboxItemReadResponse,
        WatchItemRequest,
        WatchItemResponse,
        DismissInboxItemRequest,
        DismissInboxItemResponse,
        IsWatchedRequest,
        IsWatchedResponse;
export 'package:ripls/data/gen/ripls/api/portfolio.pbenum.dart'
    show
        DailyItemType,
        PortfolioPerspective,
        HomeDecisionKind,
        HomeTransferAction,
        HomeUpNextKind,
        HomeGearCategory,
        HomeGearPillStyle;

/// PortfolioService handles personal cross-community portfolio operations.
class PortfolioService {
  final PortfolioServiceClient _client;
  final String? Function() _getAccessToken;
  final RpcErrorHandler _errorHandler;
  final Future<void> Function()? _onUnauthenticated;

  PortfolioService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
  })  : _client = PortfolioServiceClient(transport),
        _getAccessToken = getAccessToken,
        _errorHandler = errorHandler ?? RpcErrorHandler(),
        _onUnauthenticated = onUnauthenticated;

  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// getPortfolioMetrics retrieves three-perspective lifetime impact metrics
  /// for the authenticated user, optionally scoped to [communityIds].
  ///
  /// Returns saved_self, saved_others, and communities_total sections.
  /// Throws [ServiceException] on failure.
  Future<GetPortfolioMetricsResponse> getPortfolioMetrics({
    List<String> communityIds = const [],
  }) async {
    return RpcUtils.executeRpc(
      () async {
        return _client.getPortfolioMetrics(
          GetPortfolioMetricsRequest(communityIds: communityIds),
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetPortfolioMetrics',
    );
  }

  /// getDirectoryPeople retrieves the Directory address book's people rows:
  /// everyone the viewer shares a community with who has something shared.
  ///
  /// Throws [ServiceException] on failure.
  Future<GetDirectoryPeopleResponse> getDirectoryPeople() async {
    return RpcUtils.executeRpc(
      () async {
        return _client.getDirectoryPeople(
          GetDirectoryPeopleRequest(),
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetDirectoryPeople',
    );
  }

  /// getHomeView retrieves the sectioned Home tab view (#2435): Needs-you
  /// decisions, the Up-next agenda, the viewer's requests, gear rows, and
  /// recent activity, across all of the user's communities.
  ///
  /// Throws [ServiceException] on failure.
  Future<GetHomeViewResponse> getHomeView(String timezone) async {
    return RpcUtils.executeRpc(
      () async {
        final request = GetHomeViewRequest(timezone: timezone);
        return _client.getHomeView(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetHomeView',
    );
  }

  /// setWeeklyGoals saves the user's personal weekly impact goals and returns
  /// the effective values after saving.
  ///
  /// Throws [ServiceException] on failure.
  Future<SetWeeklyGoalsResponse> setWeeklyGoals(
    int socialTimeMinutesGoal,
    int moneySavedCentsGoal,
    int co2AvoidedGramsGoal,
  ) async {
    return RpcUtils.executeRpc(
      () async {
        final request = SetWeeklyGoalsRequest(
          socialTimeMinutesGoal: socialTimeMinutesGoal,
          moneySavedCentsGoal: moneySavedCentsGoal,
          co2AvoidedGramsGoal: co2AvoidedGramsGoal,
        );
        return _client.setWeeklyGoals(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'SetWeeklyGoals',
    );
  }

  /// markInboxItemRead marks a watched item as read for the authenticated user.
  ///
  /// Throws [ServiceException] on failure.
  Future<MarkInboxItemReadResponse> markInboxItemRead({
    required DailyItemType itemType,
    required String itemId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = MarkInboxItemReadRequest(
          itemType: itemType,
          itemId: itemId,
        );
        return _client.markInboxItemRead(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'MarkInboxItemRead',
    );
  }

  /// watchItem manually adds an item to the authenticated user's watch list.
  /// If the item was previously dismissed, it is reactivated.
  ///
  /// Throws [ServiceException] on failure.
  Future<WatchItemResponse> watchItem({
    required DailyItemType itemType,
    required String itemId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = WatchItemRequest(itemType: itemType, itemId: itemId);
        return _client.watchItem(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'WatchItem',
    );
  }

  /// dismissInboxItem removes an item from the authenticated user's inbox by
  /// soft-deleting the watch entry.
  ///
  /// Throws [ServiceException] on failure.
  Future<DismissInboxItemResponse> dismissInboxItem({
    required DailyItemType itemType,
    required String itemId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request =
            DismissInboxItemRequest(itemType: itemType, itemId: itemId);
        return _client.dismissInboxItem(
            request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'DismissInboxItem',
    );
  }

  /// isWatched returns whether the authenticated user is currently watching
  /// the specified item.
  ///
  /// Throws [ServiceException] on failure.
  Future<IsWatchedResponse> isWatched({
    required DailyItemType itemType,
    required String itemId,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = IsWatchedRequest(itemType: itemType, itemId: itemId);
        return _client.isWatched(request, headers: _buildHeaders());
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'IsWatched',
    );
  }

}
