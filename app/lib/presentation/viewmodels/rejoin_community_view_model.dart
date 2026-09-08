import 'package:connectrpc/connect.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/repositories/story_repository.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('RejoinCommunityViewModel');

/// User-facing result of attempting to rejoin a community. Widgets switch
/// on it to pick the right snackbar copy without inspecting raw RPC codes
/// or parsing message strings themselves.
sealed class RejoinOutcome {
  const RejoinOutcome();
}

/// Initial / not-yet-attempted state.
final class RejoinOutcomeIdle extends RejoinOutcome {
  const RejoinOutcomeIdle();
}

/// The rejoin RPC is in flight. Widgets disable the CTA.
final class RejoinOutcomeInProgress extends RejoinOutcome {
  const RejoinOutcomeInProgress();
}

/// The caller's CommunityUser was un-soft-deleted. Widgets pop the
/// screen, select the rejoined community, navigate into it, show the
/// `communityRejoinSuccess` snackbar, and announce via SemanticAnnouncer.
final class RejoinOutcomeSuccess extends RejoinOutcome {
  const RejoinOutcomeSuccess();
}

/// Server returned `FAILED_PRECONDITION: rejoin_window_expired` — the
/// 30-day window has elapsed. Widgets show
/// `communityRejoinWindowExpired` and pop back to home.
final class RejoinOutcomeWindowExpired extends RejoinOutcome {
  const RejoinOutcomeWindowExpired();
}

/// Server returned `FAILED_PRECONDITION: community_deleted` — the
/// community is in deleted state (§2.7 case). Widgets show
/// `communityRejoinCommunityDeleted`, pop, and invalidate the
/// recently-left list locally.
final class RejoinOutcomeCommunityDeleted extends RejoinOutcome {
  const RejoinOutcomeCommunityDeleted();
}

/// Server returned `FAILED_PRECONDITION: already_member` — caller
/// already has an active CommunityUser row. Widgets treat as success
/// (community is accessible) and drop the user into it.
final class RejoinOutcomeAlreadyMember extends RejoinOutcome {
  const RejoinOutcomeAlreadyMember();
}

/// Server returned `FAILED_PRECONDITION: no_membership_to_rejoin` —
/// caller never had a row in this community. Likely a stale entry in
/// the recently-left list. Widgets show `communityRejoinNoMembership`,
/// pop, and invalidate the list.
final class RejoinOutcomeNoMembership extends RejoinOutcome {
  const RejoinOutcomeNoMembership();
}

/// Any other failure. The widget surfaces the localized message via
/// `RpcErrorHandler.localize(error, context.l10n)`.
final class RejoinOutcomeError extends RejoinOutcome {
  final UserError error;
  const RejoinOutcomeError(this.error);
}

/// Notifier for the Rejoin confirmation screen. Owns the
/// `RejoinCommunity` mutation plus the post-success cache fan-out
/// described in `docs/community_delete_and_leave.md` §7.5 — narrower
/// than the Restore VM's fan-out because the leaver's prior gear /
/// requests / experiences / transfers were soft-deleted by the leave
/// cascade and are NOT restored on rejoin.
///
/// `Code.unauthenticated` is handled upstream by `RpcUtils.executeRpc`
/// via the `_onUnauthenticated` callback wired in `CommunityService`'s
/// constructor (logout + redirect to login). The outcome-mapping below
/// never sees that code.
class RejoinCommunityNotifier extends Notifier<RejoinOutcome>
    with SafeNotifierMixin<RejoinOutcome> {
  @override
  RejoinOutcome build() => const RejoinOutcomeIdle();

  /// Attempts to rejoin the community.
  ///
  /// On success, fans out cache invalidations narrow per §7.5:
  /// community row, recently-left list, user-communities list,
  /// `communitiesProvider.reloadCommunities`, conversation/chat,
  /// feed, stories. Skips gear/request/experience/transfer
  /// `invalidateForCommunity` because the rejoiner "starts fresh" —
  /// their pre-leave content remains soft-deleted.
  ///
  /// The fan-out is best-effort: a stale repo doesn't make the rejoin
  /// "failed" from the user's perspective. The source of truth is the
  /// server's response.
  Future<void> rejoin(String communityId) async {
    if (state is RejoinOutcomeInProgress) return;
    safeSetState(const RejoinOutcomeInProgress());

    try {
      await ref.read(communityRepositoryProvider).rejoinCommunity(communityId);
    } on ServiceException catch (e) {
      final mapped = _mapFailedPrecondition(e, communityId);
      safeUpdateState((_) => mapped);
      return;
    } catch (e, st) {
      _log.severe('Rejoin $communityId failed unexpectedly', e, st);
      safeUpdateState((_) => RejoinOutcomeError(
            RpcErrorHandler.classify(e),
          ));
      return;
    }

    // RPC succeeded. Best-effort cache fan-out.
    try {
      await _invalidateRelatedCaches(communityId);
    } catch (e, st) {
      _log.warning(
        'Rejoin $communityId: post-success cache fan-out failed',
        e,
        st,
      );
    }
    _log.info('Community $communityId rejoined');
    safeUpdateState((_) => const RejoinOutcomeSuccess());
  }

  /// Server discriminates the four FailedPrecondition reasons via a
  /// suffix on the error message (`": no_membership_to_rejoin"`,
  /// `": already_member"`, `": rejoin_window_expired"`,
  /// `": community_deleted"`). Parse here; unrecognised suffixes
  /// fall through to a generic UserError.
  RejoinOutcome _mapFailedPrecondition(
    ServiceException e,
    String communityId,
  ) {
    if (e.code != Code.failedPrecondition) {
      _log.warning('Rejoin $communityId failed: ${e.code} ${e.message}');
      return RejoinOutcomeError(RpcErrorHandler.classify(e));
    }
    final msg = e.message;
    if (msg.contains('no_membership_to_rejoin')) {
      _log.info('Rejoin $communityId rejected: no_membership_to_rejoin');
      return const RejoinOutcomeNoMembership();
    }
    if (msg.contains('already_member')) {
      _log.info('Rejoin $communityId rejected: already_member');
      return const RejoinOutcomeAlreadyMember();
    }
    if (msg.contains('rejoin_window_expired')) {
      _log.info('Rejoin $communityId rejected: rejoin_window_expired');
      return const RejoinOutcomeWindowExpired();
    }
    if (msg.contains('community_deleted')) {
      _log.info('Rejoin $communityId rejected: community_deleted');
      return const RejoinOutcomeCommunityDeleted();
    }
    _log.warning(
      'Rejoin $communityId: unrecognised FailedPrecondition reason: $msg',
    );
    return RejoinOutcomeError(RpcErrorHandler.classify(e));
  }

  Future<void> _invalidateRelatedCaches(String communityId) async {
    // CommunityRepository.rejoinCommunity already invalidates the
    // community row, the rejoinable-list, and refreshes
    // user-communities. Cover the remaining §7.5 fan-out.
    // Note: gear / request / experience / transfer are intentionally
    // skipped — the leaver's prior content was soft-deleted by the
    // leave cascade and is NOT restored on rejoin.
    final chatRepo = ref.read(chatRepositoryProvider);
    await chatRepo.invalidateAll();
    await ref.read(feedRepositoryProvider).invalidateFeed();
    await ref.read(storyRepositoryProvider).invalidateCommunity(communityId);
    // Push the refreshed user-communities list into CommunitiesNotifier
    // so the side pane / "who can see" pickers pick up the rejoined
    // community immediately.
    await ref.read(communitiesProvider.notifier).reloadCommunities();
  }
}

/// Auto-disposes when the Rejoin screen leaves the tree. The screen is
/// push-and-pop; there's no need to retain state across navigations.
final rejoinCommunityProvider =
    NotifierProvider.autoDispose<RejoinCommunityNotifier, RejoinOutcome>(
  RejoinCommunityNotifier.new,
);
