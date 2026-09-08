import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/services/providers.dart';

part 'experience_sharing_view_model.freezed.dart';

/// State for experience sharing functionality across multiple communities.
@freezed
sealed class ExperienceSharingState with _$ExperienceSharingState {
  const factory ExperienceSharingState({
    // Item ID this state was initialized for. Used to avoid re-seeding the
    // shared community list from stale server data on repeated modal opens
    // within the same access sheet session.
    String? initializedForItemId,
    // Communities the user belongs to
    @Default([]) List<CommunityItem> userCommunities,
    // Communities where this experience is currently shared
    @Default([]) List<String> sharedCommunityIds,
    // Loading state for fetching communities
    @Default(false) bool isLoadingCommunities,
    UserError? communitiesError,
    // Loading state for share/unshare operations
    @Default(false) bool isSharing,
    UserError? sharingError,
  }) = _ExperienceSharingState;
}

/// Notifier for managing experience sharing state.
class ExperienceSharingNotifier extends Notifier<ExperienceSharingState> {
  @override
  ExperienceSharingState build() => const ExperienceSharingState();

  /// Loads all communities the user is a member of.
  Future<void> loadUserCommunities() async {
    state = state.copyWith(isLoadingCommunities: true, communitiesError: null);
    try {
      final communityRepo = ref.read(communityRepositoryProvider);
      final communities = await communityRepo.listUserCommunities();
      state = state.copyWith(
        userCommunities: communities,
        isLoadingCommunities: false,
      );
    } catch (e) {
      state = state.copyWith(
        communitiesError: RpcErrorHandler.classify(e),
        isLoadingCommunities: false,
      );
    }
  }

  /// Shares an experience with a specific community.
  Future<void> shareWithCommunity(String experienceId, String communityId) async {
    state = state.copyWith(isSharing: true, sharingError: null);
    try {
      await ref.read(experienceRepositoryProvider).shareExperience(
        experienceId: experienceId,
        communityId: communityId,
      );
      state = state.copyWith(
        sharedCommunityIds: [...state.sharedCommunityIds, communityId],
        isSharing: false,
      );
    } catch (e) {
      state = state.copyWith(
        sharingError: RpcErrorHandler.classify(e),
        isSharing: false,
      );
    }
  }

  /// Unshares an experience from a specific community.
  Future<void> unshareFromCommunity(String experienceId, String communityId) async {
    state = state.copyWith(isSharing: true, sharingError: null);
    try {
      await ref.read(experienceRepositoryProvider).unshareExperience(
        experienceId: experienceId,
        communityId: communityId,
      );
      state = state.copyWith(
        sharedCommunityIds: state.sharedCommunityIds
            .where((id) => id != communityId)
            .toList(),
        isSharing: false,
      );
    } catch (e) {
      state = state.copyWith(
        sharingError: RpcErrorHandler.classify(e),
        isSharing: false,
      );
    }
  }

  /// Sets the initial shared communities for the experience and records the
  /// item ID so subsequent modal opens for the same item skip the reseed.
  void setSharedCommunities(String itemId, List<String> communityIds) {
    state = state.copyWith(
      initializedForItemId: itemId,
      sharedCommunityIds: communityIds,
    );
  }

  /// Clears the initialization marker so the next access-sheet session
  /// re-seeds from the freshly-fetched server state.
  void clearInitialization() {
    state = state.copyWith(initializedForItemId: null);
  }
}

/// Provider for the experience sharing notifier.
/// Note: Not using autoDispose to ensure state persists when modal is shown.
final experienceSharingNotifierProvider =
    NotifierProvider<ExperienceSharingNotifier, ExperienceSharingState>(
  ExperienceSharingNotifier.new,
);
