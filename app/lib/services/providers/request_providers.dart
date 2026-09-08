import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/cache_providers.dart';
import 'package:ripls/services/providers/chat_providers.dart';
import 'package:ripls/services/providers/feed_providers.dart';
import 'package:ripls/services/providers/profile_providers.dart';
import 'package:ripls/services/providers/search_providers.dart';
import 'package:ripls/services/request_service.dart';

/// Provider for RequestService
final requestServiceProvider = Provider<RequestService>((ref) {
  return RequestService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    onUnauthenticated: () async {
      // Clear auth state - the router will automatically redirect to login
      // and preserve the current location in the 'from' query parameter
      await ref.read(authStateProvider.notifier).logout();
    },
  );
});

/// Provider for RequestRepository
final requestRepositoryProvider = Provider<RequestRepository>((ref) {
  final cache = ref.watch(cacheManagerProvider);
  final service = ref.watch(requestServiceProvider);
  final chatRepository = ref.watch(chatRepositoryProvider);
  final feedRepository = ref.watch(feedRepositoryProvider);
  final searchRepository = ref.watch(searchRepositoryProvider);

  void onDailyInvalidated() {
    ref.read(portfolioCacheInvalidationProvider.notifier).notify();
  }

  void onProfileInvalidated() {
    ref.read(profileRepositoryProvider).invalidateAll();
  }

  return RequestRepository(
    cache,
    service,
    chatRepository,
    feedRepository,
    searchRepository,
    onDailyInvalidated: onDailyInvalidated,
    onProfileInvalidated: onProfileInvalidated,
  );
});
