import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart' show ConversationItem;
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';

part 'community_content_state.freezed.dart';

/// State for CommunityContentViewModel.
///
/// Manages the state of community members, events, gear count, and the
/// community-wide chat conversation with separate loading and error states.
@freezed
sealed class CommunityContentState with _$CommunityContentState {
  const factory CommunityContentState({
    /// Active tab index: 0 = Community info, 1 = Discuss.
    @Default(0) int activeTab,

    /// The community-wide conversation, loaded lazily when tab 1 is opened.
    ConversationItem? communityConversation,

    /// Whether the community conversation is being loaded.
    @Default(false) bool isLoadingConversation,

    /// Error for conversation loading, if any.
    UserError? conversationError,

    /// List of members in the community.
    @Default([]) List<CommunityMember> members,

    /// List of events in the community.
    @Default([]) List<CommunityEventItem> events,

    /// Count of gear items shared in the community.
    @Default(0) int gearCount,

    /// Whether members are currently being loaded.
    @Default(false) bool isLoadingMembers,

    /// Whether events are currently being loaded.
    @Default(false) bool isLoadingEvents,

    /// Whether gear count is currently being loaded.
    @Default(false) bool isLoadingGear,

    /// Error for members loading, if any.
    UserError? membersError,

    /// Error for events loading, if any.
    UserError? eventsError,

    /// Error for gear loading, if any.
    UserError? gearError,
  }) = _CommunityContentState;

  const CommunityContentState._();

  /// Returns true if all data sources have finished loading.
  bool get isFullyLoaded =>
      !isLoadingMembers && !isLoadingEvents && !isLoadingGear;

  /// Returns true if any data source is currently loading.
  bool get isLoading => isLoadingMembers || isLoadingEvents || isLoadingGear;

  /// Returns true if any data source has an error.
  bool get hasError =>
      membersError != null || eventsError != null || gearError != null;

  /// Returns true if there are members in the community.
  bool get hasMembers => members.isNotEmpty;

  /// Returns true if there are events in the community.
  bool get hasEvents => events.isNotEmpty;

  /// Returns true if there are any gear items shared.
  bool get hasGear => gearCount > 0;

  /// Returns true if the initial load is complete (no errors and fully loaded).
  bool get isInitialized => isFullyLoaded && !hasError;
}
