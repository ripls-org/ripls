import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/utils/logout_diagnostics.dart';
import 'package:ripls/data/repositories/experience_repository.dart';
import 'package:ripls/services/experience_service.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/cache_providers.dart';
import 'package:ripls/services/providers/chat_providers.dart';
import 'package:ripls/services/providers/feed_providers.dart';
import 'package:ripls/services/providers/profile_providers.dart';
import 'package:ripls/services/providers/search_providers.dart';

/// Provider for ExperienceService
final experienceServiceProvider = Provider<ExperienceService>((ref) {
  return ExperienceService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    onUnauthenticated: () async {
      // Clear auth state - the router will automatically redirect to login
      // and preserve the current location in the 'from' query parameter
      await ref.read(authStateProvider.notifier).logout();
    },
  );
});

/// Provider for ExperienceRepository
final experienceRepositoryProvider = Provider<ExperienceRepository>((ref) {
  final cache = ref.watch(cacheManagerProvider);
  final service = ref.watch(experienceServiceProvider);
  final feedRepository = ref.watch(feedRepositoryProvider);
  final chatRepository = ref.watch(chatRepositoryProvider);
  final searchRepository = ref.watch(searchRepositoryProvider);

  void onDailyInvalidated() {
    LogoutDiagnostics.trace('EXP_REPO_ON_DAILY_INVALIDATED');
    ref.read(portfolioCacheInvalidationProvider.notifier).notify();
  }

  void onProfileInvalidated() {
    ref.read(profileRepositoryProvider).invalidateAll();
  }

  void onContentInvalidated() {
    LogoutDiagnostics.trace('EXP_REPO_ON_CONTENT_INVALIDATED');
    ref.read(contentCacheInvalidationProvider.notifier).notify();
  }

  void onImpactInvalidated() {
    LogoutDiagnostics.trace('EXP_REPO_ON_IMPACT_INVALIDATED');
    ref.read(impactCacheInvalidationProvider.notifier).notify();
  }

  return ExperienceRepository(
    cache,
    service,
    feedRepository,
    chatRepository,
    searchRepository,
    onDailyInvalidated: onDailyInvalidated,
    onProfileInvalidated: onProfileInvalidated,
    onContentInvalidated: onContentInvalidated,
    onImpactInvalidated: onImpactInvalidated,
  );
});
