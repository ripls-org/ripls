import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/data/repositories/community_repository.dart';
import 'package:ripls/data/repositories/provisional_user_repository.dart';
import 'package:ripls/services/community_service.dart' show CommunityMember;
import 'package:ripls/services/providers.dart';

export 'package:ripls/data/repositories/provisional_user_repository.dart'
    show ProvisionalUser, GetProvisionalUserInviteLinkResponse;
export 'package:ripls/services/community_service.dart' show CommunityMember;

part 'manage_members_view_model.freezed.dart';

/// State for the Manage Members screen.
@freezed
sealed class ManageMembersState with _$ManageMembersState {
  const factory ManageMembersState({
    @Default([]) List<CommunityMember> members,
    @Default([]) List<ProvisionalUser> provisionalUsers,
    @Default(false) bool isLoadingMembers,
    @Default(false) bool isLoadingProvisionalUsers,
    UserError? error,

    /// Set after successfully fetching an invite link for a provisional user.
    GetProvisionalUserInviteLinkResponse? pendingInviteLink,

    /// The provisional user ID for which the invite link was most recently fetched.
    String? inviteLinkProvisionalUserId,
  }) = _ManageMembersState;
}

/// Notifier for the Manage Members screen.
///
/// Loads real members and provisional members, and fetches invite links for
/// provisional users.
///
/// Architecture: ViewModel → Repository → Service
class ManageMembersNotifier extends Notifier<ManageMembersState> {
  @override
  ManageMembersState build() => const ManageMembersState();

  CommunityRepository get _communityRepo =>
      ref.read(communityRepositoryProvider);

  ProvisionalUserRepository get _provisionalRepo =>
      ref.read(provisionalUserRepositoryProvider);

  /// Loads both real members and provisional members for [communityId].
  Future<void> load(String communityId) async {
    state = state.copyWith(
      isLoadingMembers: true,
      isLoadingProvisionalUsers: true,
      error: null,
    );

    await Future.wait([
      _loadMembers(communityId),
      _loadProvisionalUsers(communityId),
    ]);
  }

  Future<void> _loadMembers(String communityId) async {
    try {
      final members = await _communityRepo.listCommunityUsers(communityId);
      state = state.copyWith(members: members, isLoadingMembers: false);
    } catch (e) {
      state = state.copyWith(
        isLoadingMembers: false,
        error: RpcErrorHandler.classify(e),
      );
    }
  }

  Future<void> _loadProvisionalUsers(String communityId) async {
    try {
      final provs = await _provisionalRepo.listProvisionalUsers(communityId);
      state = state.copyWith(
        provisionalUsers: provs,
        isLoadingProvisionalUsers: false,
      );
    } catch (e) {
      state = state.copyWith(
        isLoadingProvisionalUsers: false,
        error: RpcErrorHandler.classify(e),
      );
    }
  }

  /// Fetches (or creates) an invite link for [provisionalUserId] in [communityId].
  ///
  /// The result is stored in [ManageMembersState.pendingInviteLink] so the UI
  /// can display a share sheet.
  Future<void> getInviteLink({
    required String communityId,
    required String provisionalUserId,
  }) async {
    state = state.copyWith(error: null, pendingInviteLink: null);
    try {
      final response = await _provisionalRepo.getInviteLink(
        communityId: communityId,
        provisionalUserId: provisionalUserId,
      );
      state = state.copyWith(
        pendingInviteLink: response,
        inviteLinkProvisionalUserId: provisionalUserId,
      );
    } catch (e) {
      state = state.copyWith(error: RpcErrorHandler.classify(e));
    }
  }

  /// Clears the pending invite link after the share sheet has been shown.
  void clearInviteLink() {
    state = state.copyWith(
      pendingInviteLink: null,
      inviteLinkProvisionalUserId: null,
    );
  }

  /// Clears the error message.
  void clearError() {
    state = state.copyWith(error: null);
  }
}

/// Provider for ManageMembersNotifier.
final manageMembersProvider =
    NotifierProvider.autoDispose<ManageMembersNotifier, ManageMembersState>(
      ManageMembersNotifier.new,
    );
