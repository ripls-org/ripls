import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/gen/ripls/api/user_service.pb.dart';
import 'package:ripls/presentation/viewmodels/profile_metrics_data.dart';
import 'package:ripls/services/providers.dart';

/// Provider for profile metrics data using AsyncNotifier pattern.
///
/// Manages loading and refreshing of all profile metrics data including:
/// - User profile information
/// - Aggregate statistics (communities, items, loans, savings)
///
/// Usage:
/// ```dart
/// ref.watch(profileImpactMetricsProvider(userId)).when(
///   data: (data) => ProfileMetricsContent(data: data),
///   loading: () => const ProfileMetricsSkeleton(),
///   error: (e, st) => ProfileMetricsError(error: e.toString()),
/// );
/// ```
final profileImpactMetricsProvider = AsyncNotifierProvider.autoDispose
    .family<ProfileImpactMetricsViewModel, ProfileMetricsData, String>(
  ProfileImpactMetricsViewModel.new,
);

/// ViewModel for ProfileMetricsView screen.
///
/// Loads user profile data, statistics, communities, and stories in parallel.
/// Provides manual refresh functionality with proper disposal safety.
class ProfileImpactMetricsViewModel extends AsyncNotifier<ProfileMetricsData> {
  /// Constructor receives the userId from the family provider
  ProfileImpactMetricsViewModel(this.userId);

  /// The userId for this notifier instance
  final String userId;

  @override
  Future<ProfileMetricsData> build() async {
    // Initial load - Riverpod handles disposal automatically in build()
    return _loadAllData(userId);
  }

  /// Loads all profile metrics data in parallel for performance.
  ///
  /// Fetches:
  /// - User profile (name, description, media)
  /// - User statistics (communities, items, loans, savings)
  /// - Profile image URL (if user has media)
  /// - Location name (if user has primary residence)
  Future<ProfileMetricsData> _loadAllData(String userId) async {
    final userRepo = ref.read(userRepositoryProvider);

    // Load data in parallel for performance
    final results = await Future.wait<dynamic>([
      userRepo.get(userId),
      userRepo.getUserStats(userId),
    ]);

    final user = results[0] as GetUserResponse;
    final stats = results[1] as GetUserStatsResponse;

    // Load optional profile image URL and location name
    String? profileImageUrl;
    String? locationName;

    if (user.mediaId.isNotEmpty) {
      // CRITICAL: Check mounted after async gap
      if (!ref.mounted) throw StateError('Provider disposed');
      final mediaRepo = ref.read(mediaRepositoryProvider);
      final mediaUrl = await mediaRepo.getFullMediaUrl(user.mediaId);
      profileImageUrl = mediaUrl.url;
    }

    if (user.primaryResidenceLocationId.isNotEmpty) {
      // CRITICAL: Check mounted after async gap
      if (!ref.mounted) throw StateError('Provider disposed');
      final locationRepo = ref.read(locationRepositoryProvider);
      final location = await locationRepo.get(user.primaryResidenceLocationId);
      locationName = location.name;
    }

    return ProfileMetricsData(
      user: user,
      stats: stats,
      profileImageUrl: profileImageUrl,
      locationName: locationName,
    );
  }

  /// Manual refresh of all profile metrics data.
  ///
  /// CRITICAL: Checks ref.mounted after async gaps to prevent state updates
  /// after disposal.
  ///
  /// Usage:
  /// ```dart
  /// await ref.read(profileImpactMetricsProvider(userId).notifier).refresh();
  /// ```
  Future<void> refresh() async {
    // CRITICAL: Check mounted before state assignment
    if (!ref.mounted) return;
    state = const AsyncLoading();

    final result = await AsyncValue.guard(() => _loadAllData(userId));

    // CRITICAL: Check mounted after async gap
    if (!ref.mounted) return;
    state = result;
  }
}
