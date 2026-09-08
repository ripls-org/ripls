import 'package:connectrpc/connect.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/repositories/story_repository.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('LeaveCommunityViewModel');

/// User-facing result of attempting to leave a community. Widgets switch
/// on it to pick the right snackbar copy without inspecting raw RPC
/// codes or parsing message strings themselves.
///
/// Covers both branches of #1719:
/// - **Non-owner Leave** only emits
///   [LeaveOutcomeIdle] / [LeaveOutcomeInProgress] /
///   [LeaveOutcomeSuccess] / [LeaveOutcomeError].
/// - **Owner-leave-with-handoff** additionally emits the
///   four typed [Code.invalidArgument] / [Code.failedPrecondition]
///   reasons the server returns from the race-safe
///   `ClaimCommunityOwnerHandoff` path.
sealed class LeaveOutcome {
  const LeaveOutcome();
}

/// Initial / not-yet-attempted state.
final class LeaveOutcomeIdle extends LeaveOutcome {
  const LeaveOutcomeIdle();
}

/// The leave RPC is in flight. Widgets disable the CTA.
final class LeaveOutcomeInProgress extends LeaveOutcome {
  const LeaveOutcomeInProgress();
}

/// The caller's CommunityUser was soft-deleted (and ownership was
/// handed off if the caller was the owner). Widgets pop the screen,
/// route the user away from the now-left community, and show the
/// success snackbar.
final class LeaveOutcomeSuccess extends LeaveOutcome {
  const LeaveOutcomeSuccess();
}

/// Server returned `INVALID_ARGUMENT: missing_new_owner_user_id` —
/// the owner-leave call was made without a candidate. Defensive: the
/// picker should always populate this. Widgets show
/// `communityLeaveMissingNewOwner` and stay on screen.
final class LeaveOutcomeMissingNewOwner extends LeaveOutcome {
  const LeaveOutcomeMissingNewOwner();
}

/// Server returned `INVALID_ARGUMENT: candidate_is_caller` — the
/// caller picked themselves as the new owner. Widgets show
/// `communityLeaveCandidateIsCaller` and return to the picker.
final class LeaveOutcomeCandidateIsCaller extends LeaveOutcome {
  const LeaveOutcomeCandidateIsCaller();
}

/// Server returned `FAILED_PRECONDITION: candidate_not_member` — the
/// candidate is no longer a member of the community. Widgets show
/// `communityLeaveCandidateNotMember` and route back to the picker
/// so the user can pick again from the fresh member list.
final class LeaveOutcomeCandidateNotMember extends LeaveOutcome {
  const LeaveOutcomeCandidateNotMember();
}

/// Server returned `FAILED_PRECONDITION: ownership_changed` —
/// ownership transferred since the screen was opened. Widgets show
/// `communityLeaveOwnershipChanged` and dismiss.
final class LeaveOutcomeOwnershipChanged extends LeaveOutcome {
  const LeaveOutcomeOwnershipChanged();
}

/// Server returned `FAILED_PRECONDITION: community_deleted` — the
/// community is in deleted state. Widgets show
/// `communityLeaveCommunityDeleted` and pop back to Settings.
final class LeaveOutcomeCommunityDeleted extends LeaveOutcome {
  const LeaveOutcomeCommunityDeleted();
}

/// Server returned `Code.notFound` — the caller no longer has an
/// active membership in the community (likely the membership row was
/// purged or hard-deleted between screen open and the leave call).
/// Widgets show `communityLeaveNotFound` and pop.
final class LeaveOutcomeNotFound extends LeaveOutcome {
  const LeaveOutcomeNotFound();
}

/// Any other failure. The widget surfaces the localized message via
/// `RpcErrorHandler.localize(error, context.l10n)`.
final class LeaveOutcomeError extends LeaveOutcome {
  final UserError error;
  const LeaveOutcomeError(this.error);
}

/// Notifier for the Leave confirmation screen. Owns the
/// `LeaveCommunity` mutation plus the post-success cache fan-out
/// described in `docs/community_delete_and_leave.md` §7.5.
///
/// Wider fan-out than the rejoin VM because the leaver's own gear
/// shares, requests, and active transfers were just soft-deleted by
/// the server-side leave cascade — the corresponding caches must
/// drop them.
///
/// `Code.unauthenticated` is handled upstream by `RpcUtils.executeRpc`
/// via the `_onUnauthenticated` callback wired in `CommunityService`'s
/// constructor; the outcome-mapping below never sees it.
class LeaveCommunityNotifier extends Notifier<LeaveOutcome>
    with SafeNotifierMixin<LeaveOutcome> {
  @override
  LeaveOutcome build() => const LeaveOutcomeIdle();

  /// Attempts to leave the community. When the caller is the owner,
  /// pass [newOwnerUserId] to identify the active member who will
  /// take over. Pass null for the non-owner branch.
  Future<void> leave(
    String communityId, {
    String? newOwnerUserId,
  }) async {
    if (state is LeaveOutcomeInProgress) return;
    safeSetState(const LeaveOutcomeInProgress());

    try {
      await ref.read(communityRepositoryProvider).leaveCommunity(
            communityId,
            newOwnerUserId: newOwnerUserId,
          );
    } on ServiceException catch (e) {
      final mapped = _mapTypedFailure(e, communityId);
      safeUpdateState((_) => mapped);
      return;
    } catch (e, st) {
      _log.severe('Leave $communityId failed unexpectedly', e, st);
      safeUpdateState((_) => LeaveOutcomeError(
            RpcErrorHandler.classify(e),
          ));
      return;
    }

    // RPC succeeded. Best-effort cache fan-out.
    try {
      await _invalidateRelatedCaches(communityId);
    } catch (e, st) {
      _log.warning(
        'Leave $communityId: post-success cache fan-out failed',
        e,
        st,
      );
    }
    _log.info('Community $communityId left successfully');
    safeUpdateState((_) => const LeaveOutcomeSuccess());
  }

  /// Server discriminates the typed reasons via a suffix on the error
  /// message for `Code.invalidArgument` and `Code.failedPrecondition`,
  /// or returns `Code.notFound` directly when the membership row is
  /// gone. Parse here; unrecognised suffixes fall through to a
  /// generic UserError.
  LeaveOutcome _mapTypedFailure(ServiceException e, String communityId) {
    // notFound is code-only — no message discriminator. Branch first.
    if (e.code == Code.notFound) {
      _log.info('Leave $communityId rejected: not_found');
      return const LeaveOutcomeNotFound();
    }
    final msg = e.message;
    if (e.code == Code.invalidArgument) {
      if (msg.contains('missing_new_owner_user_id')) {
        _log.info(
          'Leave $communityId rejected: missing_new_owner_user_id',
        );
        return const LeaveOutcomeMissingNewOwner();
      }
      if (msg.contains('candidate_is_caller')) {
        _log.info('Leave $communityId rejected: candidate_is_caller');
        return const LeaveOutcomeCandidateIsCaller();
      }
    }
    if (e.code == Code.failedPrecondition) {
      if (msg.contains('candidate_not_member')) {
        _log.info('Leave $communityId rejected: candidate_not_member');
        return const LeaveOutcomeCandidateNotMember();
      }
      if (msg.contains('ownership_changed')) {
        _log.info('Leave $communityId rejected: ownership_changed');
        return const LeaveOutcomeOwnershipChanged();
      }
      if (msg.contains('community_deleted')) {
        _log.info('Leave $communityId rejected: community_deleted');
        return const LeaveOutcomeCommunityDeleted();
      }
    }
    _log.warning('Leave $communityId failed: ${e.code} ${e.message}');
    return LeaveOutcomeError(RpcErrorHandler.classify(e));
  }

  Future<void> _invalidateRelatedCaches(String communityId) async {
    // CommunityRepository.leaveCommunity already invalidates the
    // community row, the rejoinable-list (so the freshly-soft-deleted
    // membership surfaces in recently-left), and refreshes
    // user-communities. Cover the §7.5 cross-repo fan-out.
    await ref.read(communityRepositoryProvider).refreshAll(communityId);
    await ref.read(gearRepositoryProvider).invalidateForCommunity(communityId);
    await ref
        .read(requestRepositoryProvider)
        .invalidateForCommunity(communityId);
    await ref
        .read(experienceRepositoryProvider)
        .invalidateForCommunity(communityId);
    await ref
        .read(transferRepositoryProvider)
        .invalidateForCommunity(communityId);
    await ref
        .read(searchRepositoryProvider)
        .invalidateSearchesForCommunity(communityId);
    await ref.read(feedRepositoryProvider).invalidateFeed();
    await ref.read(storyRepositoryProvider).invalidateCommunity(communityId);
    await ref.read(chatRepositoryProvider).invalidateAll();
    // Push the refreshed user-communities list into
    // CommunitiesNotifier so the side pane / "who can see"
    // pickers drop the now-left community immediately.
    await ref.read(communitiesProvider.notifier).reloadCommunities();
  }
}

/// Auto-disposes when the Leave screen leaves the tree.
final leaveCommunityProvider =
    NotifierProvider.autoDispose<LeaveCommunityNotifier, LeaveOutcome>(
  LeaveCommunityNotifier.new,
);
