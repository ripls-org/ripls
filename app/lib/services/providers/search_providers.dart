import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/repositories/search_repository.dart';
import 'package:ripls/data/repositories/search_suggestions_repository.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/cache_providers.dart';
import 'package:ripls/services/search_service.dart';

/// Provider for SearchService
final searchServiceProvider = Provider<SearchService>((ref) {
  return SearchService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    onUnauthenticated: () async {
      // Clear auth state - the router will automatically redirect to login
      // and preserve the current location in the 'from' query parameter
      await ref.read(authStateProvider.notifier).logout();
    },
  );
});

/// Provider for SearchRepository
final searchRepositoryProvider = Provider<SearchRepository>((ref) {
  final cache = ref.watch(cacheManagerProvider);
  final service = ref.watch(searchServiceProvider);

  // Callback to notify SearchViewModel when cache is invalidated
  void onCacheInvalidated() {
    ref.read(searchCacheInvalidationProvider.notifier).notify();
  }

  return SearchRepository(cache, service, onCacheInvalidated);
});

/// Provider for SearchSuggestionsRepository — backs the idle search
/// panel's personalized chips (top known-for categories + top
/// communities). Shares the SearchService client with the unified
/// search RPC, separate cache namespace.
final searchSuggestionsRepositoryProvider =
    Provider<SearchSuggestionsRepository>((ref) {
  final cache = ref.watch(cacheManagerProvider);
  final service = ref.watch(searchServiceProvider);
  return SearchSuggestionsRepository(cache, service);
});

/// Whether the SearchNearbyPill is currently in its "active"
/// (text-input + suggestions panel) state. The Feed's
/// [FloatingHeader] watches this to hide the trailing avatar so the
/// active search bar reads as the only foreground surface.
class SearchPillActiveNotifier extends Notifier<bool> {
  @override
  bool build() => false;

  void set(bool value) => state = value;
}

final searchPillActiveProvider =
    NotifierProvider<SearchPillActiveNotifier, bool>(
  SearchPillActiveNotifier.new,
);
