import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/repositories/gear_repository.dart';
import 'package:ripls/data/repositories/transfer_repository.dart';
import 'package:ripls/services/gear_service.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/cache_providers.dart';
import 'package:ripls/services/providers/chat_providers.dart';
import 'package:ripls/services/providers/community_providers.dart';
import 'package:ripls/services/providers/feed_providers.dart';
import 'package:ripls/services/providers/profile_providers.dart';
import 'package:ripls/services/providers/search_providers.dart';
import 'package:ripls/services/providers/user_providers.dart';
import 'package:ripls/services/transfer_service.dart';

/// Provider for GearService
final gearServiceProvider = Provider<GearService>((ref) {
  return GearService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    errorHandler: ref.watch(rpcErrorHandlerProvider),
    onUnauthenticated: () async {
      // Clear auth state - the router will automatically redirect to login
      // and preserve the current location in the 'from' query parameter
      await ref.read(authStateProvider.notifier).logout();
    },
  );
});

/// Provider for TransferService
final transferServiceProvider = Provider<TransferService>((ref) {
  return TransferService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    errorHandler: ref.watch(rpcErrorHandlerProvider),
    onUnauthenticated: () async {
      // Clear auth state - the router will automatically redirect to login
      // and preserve the current location in the 'from' query parameter
      await ref.read(authStateProvider.notifier).logout();
    },
  );
});

/// Provider for GearRepository
final gearRepositoryProvider = Provider<GearRepository>((ref) {
  final cache = ref.watch(cacheManagerProvider);
  final service = ref.watch(gearServiceProvider);
  final communityService = ref.watch(communityServiceProvider);
  final searchRepository = ref.watch(searchRepositoryProvider);
  final feedRepository = ref.watch(feedRepositoryProvider);
  final chatRepository = ref.watch(chatRepositoryProvider);
  final communityRepository = ref.watch(communityRepositoryProvider);

  void onDailyInvalidated() {
    ref.read(portfolioCacheInvalidationProvider.notifier).notify();
  }

  void onProfileInvalidated() {
    ref.read(profileRepositoryProvider).invalidateAll();
  }

  return GearRepository(
    cache,
    service,
    communityService,
    searchRepository,
    feedRepository,
    chatRepository,
    communityRepository,
    onDailyInvalidated: onDailyInvalidated,
    onProfileInvalidated: onProfileInvalidated,
  );
});

/// Provider for TransferRepository
final transferRepositoryProvider = Provider<TransferRepository>((ref) {
  final cache = ref.watch(cacheManagerProvider);
  final service = ref.watch(transferServiceProvider);
  final chatRepository = ref.watch(chatRepositoryProvider);
  final gearRepository = ref.watch(gearRepositoryProvider);
  final communityRepository = ref.watch(communityRepositoryProvider);
  final userRepository = ref.watch(userRepositoryProvider);

  void onDailyInvalidated() {
    ref.read(portfolioCacheInvalidationProvider.notifier).notify();
  }

  return TransferRepository(
    cache,
    service,
    chatRepository,
    gearRepository,
    communityRepository,
    userRepository,
    onDailyInvalidated: onDailyInvalidated,
  );
});
