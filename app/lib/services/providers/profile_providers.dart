import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/repositories/profile_repository.dart';
import 'package:ripls/services/profile_service.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/cache_providers.dart';

/// Provider for the Profile service RPC client.
final profileServiceProvider = Provider<ProfileService>((ref) {
  return ProfileService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    onUnauthenticated: () async {
      await ref.read(authStateProvider.notifier).logout();
    },
  );
});

/// Singleton repository for the viewer-facing user profile surface.
final profileRepositoryProvider = Provider<ProfileRepository>((ref) {
  final cacheManager = ref.watch(cacheManagerProvider);
  final service = ref.watch(profileServiceProvider);
  return ProfileRepository(cacheManager, service);
});
