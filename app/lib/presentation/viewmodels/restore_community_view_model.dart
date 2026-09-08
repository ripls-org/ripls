import 'package:connectrpc/connect.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/repositories/story_repository.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('RestoreCommunityViewModel');

/// RestoreOutcome is the user-facing result of attempting to restore a
/// community. The ViewModel exposes this directly as its state so widgets
/// can switch on it to pick the right snackbar copy without having to
/// inspect raw RPC codes.
sealed class RestoreOutcome {
  const RestoreOutcome();
}

/// Initial / not-yet-attempted state.
final class RestoreOutcomeIdle extends RestoreOutcome {
  const RestoreOutcomeIdle();
}

/// The restore RPC is in flight. Widgets should disable the CTA.
final class RestoreOutcomeInProgress extends RestoreOutcome {
  const RestoreOutcomeInProgress();
}

/// The community was restored. Widgets pop the screen, show
/// `communityRestoreSuccess`, and announce via SemanticAnnouncer.
final class RestoreOutcomeSuccess extends RestoreOutcome {
  const RestoreOutcomeSuccess();
}

/// The server returned `FAILED_PRECONDITION` — another eligible member
/// already restored the community. Widgets show
/// `communityRestoreAlreadyRestored` and pop.
final class RestoreOutcomeAlreadyRestored extends RestoreOutcome {
  const RestoreOutcomeAlreadyRestored();
}

/// The server returned `PERMISSION_DENIED` — the caller is not in the
/// eligible-restorer snapshot (rare; can happen if the snapshot was
/// recomputed since the screen was opened). Widgets show
/// `communityRestoreNoLongerEligible` and pop.
final class RestoreOutcomeNoLongerEligible extends RestoreOutcome {
  const RestoreOutcomeNoLongerEligible();
}

/// Any other failure. The widget surfaces the localized message via
/// `RpcErrorHandler.localize(error, context.l10n)`.
final class RestoreOutcomeError extends RestoreOutcome {
  final UserError error;
  const RestoreOutcomeError(this.error);
}

/// Notifier for the Restore confirmation screen. Owns one mutation —
/// `restore(communityId)` — plus the post-success cache fan-out
/// described in `docs/community_delete_and_leave.md` §7.5.
class RestoreCommunityNotifier extends Notifier<RestoreOutcome>
    with SafeNotifierMixin<RestoreOutcome> {
  @override
  RestoreOutcome build() => const RestoreOutcomeIdle();

  /// Attempts to restore the soft-deleted community.
  ///
  /// On success, fans out cache invalidations across every repo whose
  /// data is keyed by community so the user does not see a stale
  /// "deleted" view of the restored community on the next read.
  Future<void> restore(String communityId) async {
    if (state is RestoreOutcomeInProgress) return;
    safeSetState(const RestoreOutcomeInProgress());

    try {
      await ref.read(communityRepositoryProvider).restoreCommunity(communityId);
    } on ServiceException catch (e) {
      switch (e.code) {
        case Code.failedPrecondition:
          _log.info('Restore $communityId rejected: already restored');
          safeUpdateState((_) => const RestoreOutcomeAlreadyRestored());
        case Code.permissionDenied:
          _log.info('Restore $communityId rejected: not in restorer snapshot');
          safeUpdateState((_) => const RestoreOutcomeNoLongerEligible());
        default:
          _log.warning('Restore $communityId failed: ${e.code} ${e.message}');
          safeUpdateState((_) => RestoreOutcomeError(
                RpcErrorHandler.classify(e),
              ));
      }
      return;
    } catch (e, st) {
      _log.severe('Restore $communityId failed unexpectedly', e, st);
      safeUpdateState((_) => RestoreOutcomeError(
            RpcErrorHandler.classify(e),
          ));
      return;
    }

    // RPC succeeded. Cache fan-out is best-effort — a stale repo
    // doesn't make the restore "failed" from the user's perspective.
    try {
      await _invalidateRelatedCaches(communityId);
    } catch (e, st) {
      _log.warning(
        'Restore $communityId: post-success cache fan-out failed',
        e,
        st,
      );
    }
    _log.info('Community $communityId restored');
    safeUpdateState((_) => const RestoreOutcomeSuccess());
  }

  Future<void> _invalidateRelatedCaches(String communityId) async {
    // CommunityRepository.restoreCommunity already invalidates the
    // community row, the deleted-list, and refreshes user-communities.
    // Here we cover the cross-repo fan-out called out in §7.5.
    await ref.read(communityRepositoryProvider).refreshAll(communityId);
    // Push the refreshed user-communities list into CommunitiesNotifier
    // so the side pane, "who can see" pickers, and any other surface
    // reading from communitiesProvider.communities update
    // immediately. Mirrors the post-create pattern in sidebar.dart.
    await ref.read(communitiesProvider.notifier).reloadCommunities();
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
    await ref.read(impactMetricsRepositoryProvider).invalidateAll(communityId);
  }
}

/// Auto-disposes when the Restore screen leaves the tree. The screen
/// is push-and-pop; there's no need to retain state across navigations.
final restoreCommunityProvider =
    NotifierProvider.autoDispose<RestoreCommunityNotifier, RestoreOutcome>(
  RestoreCommunityNotifier.new,
);
