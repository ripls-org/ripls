import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/provisional_user_repository.dart';
import 'package:ripls/services/community_event_poller.dart';
import 'package:ripls/services/community_event_stream.dart';
import 'package:ripls/services/community_service.dart';
import 'package:ripls/services/event_router.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/cache_providers.dart';
import 'package:ripls/services/providers/feed_providers.dart';
import 'package:ripls/services/providers/profile_providers.dart';
import 'package:ripls/services/providers/search_providers.dart';

part 'community_providers.freezed.dart';

/// Provider for the event router that deduplicates and dispatches community
/// events from any source (poll, stream, push) to cache invalidation providers.
final eventRouterProvider = Provider<EventRouter>((ref) {
  return EventRouter(ref);
});

/// Provider for the event poller that calls ListUserEvents on a single timer
/// and routes results through the EventRouter — the correctness backstop.
final communityEventPollerProvider = Provider<CommunityEventPoller>((ref) {
  return CommunityEventPoller(
    ref.watch(communityServiceProvider),
    ref.watch(eventRouterProvider),
  );
});

/// Provider for the event stream service that opens the user's single gRPC
/// server-stream for sub-second real-time updates across all their communities.
final communityEventStreamProvider = Provider<CommunityEventStreamService>((ref) {
  return CommunityEventStreamService(
    ref.watch(communityServiceProvider),
    ref.watch(eventRouterProvider),
  );
});

/// Provider for CommunityService
final communityServiceProvider = Provider<CommunityService>((ref) {
  return CommunityService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    onUnauthenticated: () async {
      // Clear auth state - the router will automatically redirect to login
      // and preserve the current location in the 'from' query parameter
      await ref.read(authStateProvider.notifier).logout();
    },
  );
});

/// Provider for CommunityRepository
final communityRepositoryProvider = Provider<CommunityRepository>((ref) {
  final cache = ref.watch(cacheManagerProvider);
  final service = ref.watch(communityServiceProvider);
  final searchRepository = ref.watch(searchRepositoryProvider);
  final feedRepository = ref.watch(feedRepositoryProvider);
  void onDeletedListInvalidated() {
    ref.read(deletedCommunitiesCacheInvalidationProvider.notifier).notify();
  }

  void onRejoinableListInvalidated() {
    ref.read(rejoinableCommunitiesCacheInvalidationProvider.notifier).notify();
  }

  void onViewerMembershipChanged() {
    // The viewer's membership set drives the shared-community subset
    // for every cached user profile. Invalidate them all rather than
    // tracking per-target dependencies.
    ref.read(profileRepositoryProvider).invalidateAll();
  }

  return CommunityRepository(
    cache,
    service,
    searchRepository,
    feedRepository,
    onDeletedListInvalidated: onDeletedListInvalidated,
    onRejoinableListInvalidated: onRejoinableListInvalidated,
    onViewerMembershipChanged: onViewerMembershipChanged,
  );
});

/// Provider for ProvisionalUserRepository
final provisionalUserRepositoryProvider = Provider<ProvisionalUserRepository>((ref) {
  final cache = ref.watch(cacheManagerProvider);
  final service = ref.watch(communityServiceProvider);
  return ProvisionalUserRepository(cache, service);
});

/// Family provider for community details - caches community data
final communityProvider =
    FutureProvider.family<GetCommunityResponse, String>((
  ref,
  communityId,
) async {
  final communityRepository = ref.watch(communityRepositoryProvider);
  return communityRepository.get(communityId);
});

/// State for the current user's portfolio of communities.
///
/// Holds the list of communities the authenticated user belongs to, plus
/// the loading/error states for the underlying fetch. Post-#1895 there
/// is no "selected community" — the per-community filter was removed.
@freezed
sealed class CommunitiesState with _$CommunitiesState {
  const factory CommunitiesState({
    @Default([]) List<CommunityItem> communities,
    @Default(false) bool isLoading,
    String? errorMessage,
  }) = _CommunitiesState;

  const CommunitiesState._();

  /// Returns the IDs of every community the user belongs to. Use this
  /// when an RPC needs the full portfolio (the common case post-#1895).
  List<String> get communityIds => communities.map((c) => c.id).toList();
}

/// Manages the current user's community membership list.
///
/// **No `ref.mounted` guards after `await`** — this notifier is global
/// (no `.autoDispose`) and lives for the app's lifetime, so the
/// late-write race that motivates the guard in screen-scoped notifiers
/// does not apply here. See [docs/client/architecture.md "Async Safety
/// in Notifiers"](../../../../docs/client/architecture.md).
class CommunitiesNotifier extends Notifier<CommunitiesState> {
  static final _log = Logger('CommunitiesNotifier');

  @override
  CommunitiesState build() {
    return const CommunitiesState();
  }

  /// Sets the communities list and clears any prior loading/error state.
  Future<void> setCommunities(List<CommunityItem> communities) async {
    _log.info('setCommunities: ${communities.length} communities received');

    state = state.copyWith(
      communities: communities,
      isLoading: false,
      errorMessage: null,
    );
  }

  /// setLoading sets the loading state for community fetching.
  void setLoading(bool isLoading) {
    state = state.copyWith(isLoading: isLoading, errorMessage: null);
  }

  /// setError sets an error message when community loading fails.
  void setError(String errorMessage) {
    state = state.copyWith(
      isLoading: false,
      errorMessage: errorMessage,
    );
  }

  /// clearError clears the error state.
  void clearError() {
    state = state.copyWith(errorMessage: null);
  }

  /// retryLoadCommunities retries loading communities from the repository.
  ///
  /// Sets loading state, fetches communities, and updates state.
  /// Should be called from the home screen retry action.
  Future<void> retryLoadCommunities() async {
    final authState = ref.read(authStateProvider);
    if (!authState.isAuthenticated) return;

    state = state.copyWith(isLoading: true, errorMessage: null);
    try {
      final communities = await ref
          .read(communityRepositoryProvider)
          .listUserCommunities();
      await setCommunities(communities);
    } catch (e) {
      _log.severe('Failed to load communities on retry: $e');
      state = state.copyWith(
        isLoading: false,
        errorMessage: 'Unable to reach the server. Please try again.',
      );
    }
  }

  /// reloadCommunities reloads the communities list from the repository.
  Future<void> reloadCommunities() async {
    final authState = ref.read(authStateProvider);
    if (!authState.isAuthenticated) return;

    try {
      final communities = await ref
          .read(communityRepositoryProvider)
          .listUserCommunities();
      await setCommunities(communities);
    } catch (e) {
      _log.severe('Failed to reload communities: $e');
    }
  }

  /// Resets the portfolio to empty. Called on logout.
  void clear() {
    state = const CommunitiesState();
  }
}

final communitiesProvider =
    NotifierProvider<CommunitiesNotifier, CommunitiesState>(() {
      return CommunitiesNotifier();
    });
