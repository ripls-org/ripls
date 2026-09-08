import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/repositories/workshop_repository.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/cache_providers.dart';
import 'package:ripls/services/workshop_service.dart';

/// Provider for the Workshop service client.
final workshopServiceProvider = Provider<WorkshopService>((ref) {
  return WorkshopService(
    transport: ref.watch(transportProvider),
    getAccessToken: () => ref.read(authStateProvider).accessToken,
    onUnauthenticated: () async {
      await ref.read(authStateProvider.notifier).logout();
    },
  );
});

/// Singleton repository for the Workshop tab.
final workshopRepositoryProvider = Provider<WorkshopRepository>((ref) {
  final cacheManager = ref.watch(cacheManagerProvider);
  final service = ref.watch(workshopServiceProvider);
  return WorkshopRepository(cacheManager, service);
});
