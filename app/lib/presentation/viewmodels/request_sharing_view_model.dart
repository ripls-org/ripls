import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/services/providers.dart';

part 'request_sharing_view_model.freezed.dart';

final _log = Logger('RequestSharingViewModel');

/// State for request sharing functionality across multiple communities.
@freezed
sealed class RequestSharingState with _$RequestSharingState {
  const factory RequestSharingState({
    // Communities the user belongs to
    @Default([]) List<CommunityItem> userCommunities,
    // Communities where this request is currently shared
    @Default([]) List<String> sharedCommunityIds,
    // Loading state for fetching communities
    @Default(false) bool isLoadingCommunities,
    UserError? communitiesError,
    // Loading state for share/unshare operations
    @Default({}) Set<String> loadingCommunityIds,
    UserError? sharingError,
  }) = _RequestSharingState;

  const RequestSharingState._();

  /// Returns whether a community is currently being shared or unshared.
  bool isLoadingCommunity(String communityId) =>
      loadingCommunityIds.contains(communityId);

  /// Returns whether the request is shared with a specific community.
  bool isSharedWithCommunity(String communityId) =>
      sharedCommunityIds.contains(communityId);
}

/// Notifier for managing request sharing across multiple communities.
class RequestSharingNotifier extends Notifier<RequestSharingState>
    with SafeNotifierMixin<RequestSharingState> {
  @override
  RequestSharingState build() => const RequestSharingState();

  /// Loads the list of communities the user belongs to.
  Future<void> loadUserCommunities() async {
    state = state.copyWith(isLoadingCommunities: true, communitiesError: null);

    try {
      _log.info('Loading user communities for request sharing');

      final communityRepo = ref.read(communityRepositoryProvider);
      final communities = await communityRepo.listUserCommunities();

      safeUpdateState((s) => s.copyWith(
        userCommunities: communities,
        isLoadingCommunities: false,
      ));

      _log.info('✅ Loaded ${communities.length} communities');
    } catch (e, stackTrace) {
      _log.severe('Failed to load communities', e, stackTrace);
      safeUpdateState((s) => s.copyWith(
        communitiesError: RpcErrorHandler.classify(e),
        isLoadingCommunities: false,
      ));
    }
  }

  /// Sets the list of communities where the request is currently shared.
  ///
  /// Call this when initializing the modal with the current request's shared_community_ids.
  void setSharedCommunities(List<String> communityIds) {
    state = state.copyWith(sharedCommunityIds: communityIds);
  }

  /// Shares the request with a specific community.
  Future<void> shareWithCommunity(String requestId, String communityId) async {
    if (state.isSharedWithCommunity(communityId)) {
      _log.info('Request already shared with community $communityId');
      return;
    }

    state = state.copyWith(
      loadingCommunityIds: {...state.loadingCommunityIds, communityId},
      sharingError: null,
    );

    try {
      _log.info('Sharing request $requestId with community $communityId');

      final requestRepo = ref.read(requestRepositoryProvider);
      final updatedRequest = await requestRepo.shareRequest(
        requestId: requestId,
        communityIds: [communityId],
      );

      safeUpdateState((s) => s.copyWith(
        sharedCommunityIds: updatedRequest.sharedCommunityIds,
        loadingCommunityIds: s.loadingCommunityIds
            .where((id) => id != communityId)
            .toSet(),
      ));

      _log.info('✅ Request shared with community $communityId');
    } catch (e, stackTrace) {
      _log.severe('Failed to share request with community', e, stackTrace);

      safeUpdateState((s) => s.copyWith(
        sharingError: RpcErrorHandler.classify(e),
        loadingCommunityIds: s.loadingCommunityIds
            .where((id) => id != communityId)
            .toSet(),
      ));

      rethrow; // Re-throw so UI can show error toast
    }
  }

  /// Unshares the request from a specific community.
  Future<void> unshareFromCommunity(String requestId, String communityId) async {
    if (!state.isSharedWithCommunity(communityId)) {
      _log.info('Request not shared with community $communityId');
      return;
    }

    // Prevent unsharing from the last community
    if (state.sharedCommunityIds.length <= 1) {
      _log.warning('Cannot unshare from last community');
      state = state.copyWith(
        sharingError: const UserError.generic(fallback: 'Cannot remove from the last community'),
      );
      throw Exception('Cannot remove from the last community');
    }

    state = state.copyWith(
      loadingCommunityIds: {...state.loadingCommunityIds, communityId},
      sharingError: null,
    );

    try {
      _log.info('Unsharing request $requestId from community $communityId');

      final requestRepo = ref.read(requestRepositoryProvider);
      await requestRepo.unshareRequest(
        requestId: requestId,
        communityId: communityId,
      );

      safeUpdateState((s) => s.copyWith(
        sharedCommunityIds: s.sharedCommunityIds
            .where((id) => id != communityId)
            .toList(),
        loadingCommunityIds: s.loadingCommunityIds
            .where((id) => id != communityId)
            .toSet(),
      ));

      _log.info('✅ Request unshared from community $communityId');
    } catch (e, stackTrace) {
      _log.severe('Failed to unshare request from community', e, stackTrace);

      safeUpdateState((s) => s.copyWith(
        sharingError: RpcErrorHandler.classify(e),
        loadingCommunityIds: s.loadingCommunityIds
            .where((id) => id != communityId)
            .toSet(),
      ));

      rethrow; // Re-throw so UI can show error toast
    }
  }

  /// Clears any error messages.
  void clearError() {
    state = state.copyWith(sharingError: null, communitiesError: null);
  }
}

/// Provider for the request sharing notifier.
///
/// Note: Not using autoDispose to ensure state persists when the community
/// selection modal is shown — the provider is read (not watched) before the
/// modal opens, so autoDispose would drop the loaded communities before the
/// modal could render them.
final requestSharingProvider =
    NotifierProvider<RequestSharingNotifier, RequestSharingState>(
  RequestSharingNotifier.new,
);
