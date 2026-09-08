import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/user_service.pb.dart';
import 'package:ripls/services/community_service.dart';

part 'user_profile_state.freezed.dart';

/// State for user profile screen, holding user data and communities.
@freezed
sealed class UserProfileState with _$UserProfileState {
  const factory UserProfileState({
    /// The user profile data.
    GetUserResponse? user,

    /// User's profile media URL (resolved from first mediaId).
    String? mediaUrl,

    /// All media URLs for user's profile images (for carousel).
    @Default([]) List<String> mediaUrls,

    /// User's communities.
    @Default([]) List<CommunityItem> communities,

    /// Recent activity events across all user's communities.
    @Default([]) List<CommunityEventItem> recentActivities,

    /// Currently selected tab index (0=Activity, 1=About).
    @Default(0) int selectedTabIndex,

    /// Loading states for different data sections.
    @Default(true) bool isLoadingUser,
    @Default(true) bool isLoadingCommunities,
    @Default(false) bool isLoadingActivities,

    /// Saving and upload states.
    @Default(false) bool isSaving,
    @Default(false) bool isUploadingMedia,

    /// Errors for different sections.
    UserError? userError,
    UserError? communitiesError,
    UserError? activitiesError,
  }) = _UserProfileState;

  const UserProfileState._();

  /// Returns true if any section has an error.
  bool get hasError =>
      userError != null || communitiesError != null;
  
  
  
}
