import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/repositories/esm_repository.dart';
import 'package:ripls/services/esm_service.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/cache_providers.dart';

/// Provider for the ESM RPC service client.
final esmServiceProvider = Provider<EsmService>((ref) {
  return EsmService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    onUnauthenticated: () async {
      await ref.read(authStateProvider.notifier).logout();
    },
  );
});

/// Singleton repository for ESM card interactions.
final esmRepositoryProvider = Provider<EsmRepository>((ref) {
  return EsmRepository(
    ref.watch(cacheManagerProvider),
    ref.watch(esmServiceProvider),
    onFeedListingInvalidated: () {
      ref.read(feedListingCacheInvalidationProvider.notifier).notify();
    },
    onEsmInvalidated: () {
      ref.read(esmCacheInvalidationProvider.notifier).notify();
    },
  );
});
