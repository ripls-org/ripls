import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/repositories/user_repository.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/cache_providers.dart';
import 'package:ripls/services/providers/location_providers.dart';
import 'package:ripls/services/providers/media_providers.dart';
import 'package:ripls/services/user_service.dart';

/// Provider for UserService
final userServiceProvider = Provider<UserService>((ref) {
  return UserService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    onUnauthenticated: () async {
      // Clear auth state - the router will automatically redirect to login
      // and preserve the current location in the 'from' query parameter
      await ref.read(authStateProvider.notifier).logout();
    },
    locationService: ref.watch(locationServiceProvider),
  );
});

/// Provider for UserRepository
final userRepositoryProvider = Provider<UserRepository>((ref) {
  final cache = ref.watch(cacheManagerProvider);
  final service = ref.watch(userServiceProvider);
  final mediaRepo = ref.watch(mediaRepositoryProvider);
  return UserRepository(cache, service, mediaRepo);
});

/// Family provider for user profiles - caches user data with media URLs
/// Now uses UserRepository for transparent caching
final userProfileProvider = FutureProvider.family<UserProfile, String>((
  ref,
  userId,
) async {
  final userRepository = ref.watch(userRepositoryProvider);
  return userRepository.getUserProfile(userId);
});
