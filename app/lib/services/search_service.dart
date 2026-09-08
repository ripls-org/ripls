import 'package:connectrpc/connect.dart' as connect;
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/utils/rpc_utils.dart';
import 'package:ripls/data/gen/ripls/api/search_service.connect.client.dart';
import 'package:ripls/data/gen/ripls/api/search_service.pb.dart';

export 'package:ripls/core/errors/rpc_error_handler.dart' show ServiceException;
export 'package:ripls/data/gen/ripls/api/profile_service.pb.dart'
    show SharedCommunityRef;
export 'package:ripls/data/gen/ripls/api/search_service.pb.dart'
    show
        GetSearchSuggestionsResponse,
        SearchResultItem,
        SearchItemType,
        SearchResponse,
        SearchStrategy,
        UniversalSearchResponse;

/// SearchService handles unified search operations using the SearchService API.
class SearchService {
  final SearchServiceClient _client;
  final String? Function() _getAccessToken;
  final RpcErrorHandler _errorHandler;
  final Future<void> Function()? _onUnauthenticated;

  SearchService({
    required connect.Transport transport,
    required String? Function() getAccessToken,
    RpcErrorHandler? errorHandler,
    Future<void> Function()? onUnauthenticated,
  })  : _client = SearchServiceClient(transport),
        _getAccessToken = getAccessToken,
        _errorHandler = errorHandler ?? RpcErrorHandler(),
        _onUnauthenticated = onUnauthenticated;

  connect.Headers _buildHeaders() {
    return RpcUtils.buildHeaders(
      _getAccessToken,
      onUnauthenticated: _onUnauthenticated,
    );
  }

  /// Search searches for gear, requests, and experiences within a community.
  ///
  /// Parameters:
  /// - [query]: The search query string (searches name/title and description)
  /// - [communityId]: The unique identifier of the community to search within
  /// - [latitudeDeg]: User's current latitude in degrees
  /// - [longitudeDeg]: User's current longitude in degrees
  /// - [maxResults]: Maximum number of results to return (default: 50 on server)
  /// - [itemTypes]: Optional list of item types to filter results. If null,
  ///   defaults to gear, requests, experiences, and users.
  /// - [strategy]: Optional search strategy. Defaults to semantic search.
  ///   Use EXACT for @-mention autocomplete.
  /// - [includeCompleted]: When true, includes completed/fulfilled/cancelled
  ///   items in results. Defaults to false (active items only).
  ///
  /// Returns a list of search results sorted by composite score (descending).
  /// Each result contains the item type, scores, and the actual item data.
  /// Throws [ServiceException] with user-friendly message on failure.
  Future<List<SearchResultItem>> search({
    required String query,
    required List<String> communityIds,
    required double latitudeDeg,
    required double longitudeDeg,
    int? maxResults,
    List<SearchItemType>? itemTypes,
    SearchStrategy? strategy,
    bool includeCompleted = false,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = SearchRequest()
          ..query = query
          ..communityIds.addAll(communityIds)
          ..latitudeDeg = latitudeDeg
          ..longitudeDeg = longitudeDeg
          ..includeCompleted = includeCompleted;

        if (maxResults != null) {
          request.maxResults = maxResults;
        }

        if (strategy != null) {
          request.strategy = strategy;
        }

        if (itemTypes != null) {
          request.itemTypes.addAll(itemTypes);
        } else {
          request.itemTypes.addAll([
            SearchItemType.SEARCH_ITEM_TYPE_GEAR,
            SearchItemType.SEARCH_ITEM_TYPE_REQUEST,
            SearchItemType.SEARCH_ITEM_TYPE_EXPERIENCE,
            SearchItemType.SEARCH_ITEM_TYPE_USER,
          ]);
        }

        final response = await _client.search(
          request,
          headers: _buildHeaders(),
        );

        return response.results;
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'Search',
    );
  }

  /// Universal search (#2634): one keyword index over everything in the
  /// caller's communities, returned as fixed groups (library / plans /
  /// people). The server derives the scope from the caller's own
  /// memberships — no community IDs are sent.
  Future<UniversalSearchResponse> universalSearch({
    required String query,
    int? maxResultsPerGroup,
  }) async {
    return RpcUtils.executeRpc(
      () async {
        final request = UniversalSearchRequest()..query = query;
        if (maxResultsPerGroup != null) {
          request.maxResultsPerGroup = maxResultsPerGroup;
        }
        return _client.universalSearch(
          request,
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'UniversalSearch',
    );
  }

  /// Returns personalized suggestions to seed the idle search panel:
  /// the caller's top "known for" categories and a short list of
  /// their communities, most-recently-joined first. Server caps each
  /// list at 5 entries. Takes no parameters — scope is the
  /// authenticated user's full active membership set.
  Future<GetSearchSuggestionsResponse> getSuggestions() async {
    return RpcUtils.executeRpc(
      () async {
        return _client.getSearchSuggestions(
          GetSearchSuggestionsRequest(),
          headers: _buildHeaders(),
        );
      },
      _errorHandler,
      onUnauthenticated: _onUnauthenticated,
      operationName: 'GetSearchSuggestions',
    );
  }
}
