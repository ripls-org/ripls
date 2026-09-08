import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/repositories/feed_repository.dart';
import 'package:ripls/data/repositories/portfolio_repository.dart';
import 'package:ripls/services/feed_service.dart';
import 'package:ripls/services/portfolio_service.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/cache_providers.dart';

/// Provider for FeedService
final feedServiceProvider = Provider<FeedService>((ref) {
  return FeedService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    onUnauthenticated: () async {
      // Clear auth state - the router will automatically redirect to login
      // and preserve the current location in the 'from' query parameter
      await ref.read(authStateProvider.notifier).logout();
    },
  );
});

/// Provider for PortfolioService
final portfolioServiceProvider = Provider<PortfolioService>((ref) {
  return PortfolioService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    onUnauthenticated: () async {
      await ref.read(authStateProvider.notifier).logout();
    },
  );
});

/// Provider for FeedRepository
final feedRepositoryProvider = Provider<FeedRepository>((ref) {
  final cache = ref.watch(cacheManagerProvider);
  final service = ref.watch(feedServiceProvider);

  void onFeedStatusInvalidated() {
    ref.read(feedStatusCacheInvalidationProvider.notifier).notify();
  }

  return FeedRepository(cache, service,
      onFeedStatusInvalidated: onFeedStatusInvalidated);
});

/// Provider for PortfolioRepository
final portfolioRepositoryProvider = Provider<PortfolioRepository>((ref) {
  final cache = ref.watch(cacheManagerProvider);
  final service = ref.watch(portfolioServiceProvider);
  return PortfolioRepository(
    cache,
    service,
    onWatchMutated: () =>
        ref.read(portfolioCacheInvalidationProvider.notifier).notify(),
  );
});
