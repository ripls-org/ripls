import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/data/gen/ripls/api/user_service.pb.dart';

part 'profile_metrics_data.freezed.dart';

/// Immutable data model for ProfileMetricsView.
///
/// Contains all data needed to render the profile metrics screen:
/// - User profile information
/// - Aggregate statistics (communities, items, loans, savings, etc.)
/// - Profile image URL (full resolution for background)
/// - Location name (user's primary residence)
@freezed
sealed class ProfileMetricsData with _$ProfileMetricsData {
  const factory ProfileMetricsData({
    required GetUserResponse user,
    required GetUserStatsResponse stats,
    String? profileImageUrl,
    String? locationName,
  }) = _ProfileMetricsData;
}
