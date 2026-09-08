import 'package:connectrpc/connect.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/utils/safe_notifier.dart';
import 'package:ripls/data/repositories/story_repository.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('DeleteCommunityViewModel');

/// User-facing result of attempting to delete a community. Widgets switch
/// on it to pick the right snackbar copy without inspecting raw RPC codes.
sealed class DeleteOutcome {
  const DeleteOutcome();
}

/// Initial / not-yet-attempted state.
final class DeleteOutcomeIdle extends DeleteOutcome {
  const DeleteOutcomeIdle();
}

/// The delete RPC is in flight. Widgets disable the CTA and show a spinner.
final class DeleteOutcomeInProgress extends DeleteOutcome {
  const DeleteOutcomeInProgress();
}

/// The community was soft-deleted. Widgets pop back to Settings, show
/// `communityDeleteSuccess`, and announce via SemanticAnnouncer.
final class DeleteOutcomeSuccess extends DeleteOutcome {
  const DeleteOutcomeSuccess();
}

/// The server returned `FAILED_PRECONDITION` — another path already
/// soft-deleted the community. Widgets show
/// `communityDeleteAlreadyDeleted` and pop.
final class DeleteOutcomeAlreadyDeleted extends DeleteOutcome {
  const DeleteOutcomeAlreadyDeleted();
}

/// The server returned `PERMISSION_DENIED` — the caller is no longer the
/// owner. Rare; can happen if ownership was transferred since the screen
/// was opened, or if the auth token is stale. Widgets show
/// `communityDeleteNotOwner` and pop.
final class DeleteOutcomeNotOwner extends DeleteOutcome {
  const DeleteOutcomeNotOwner();
}

/// Any other failure. The widget surfaces the localized message via
/// `RpcErrorHandler.localize(error, context.l10n)`.
final class DeleteOutcomeError extends DeleteOutcome {
  final UserError error;
  const DeleteOutcomeError(this.error);
}

/// Notifier for the Delete confirmation screen. Owns the
/// `DeleteCommunity` mutation plus the post-success cache fan-out
/// described in `docs/community_delete_and_leave.md` §7.5 — same shape
/// as `RestoreCommunityNotifier` so reviewers can read the two side
/// by side.
class DeleteCommunityNotifier extends Notifier<DeleteOutcome>
    with SafeNotifierMixin<DeleteOutcome> {
  @override
  DeleteOutcome build() => const DeleteOutcomeIdle();

  /// Attempts to soft-delete the community. On success, fans out cache
  /// invalidations across every repo whose data is keyed by community,
  /// and pushes the refreshed user-communities list into
  /// `communitiesProvider` so the side pane and "who can see"
  /// pickers drop the deleted community immediately.
  ///
  /// The fan-out is best-effort: a stale repo doesn't make the delete
  /// "failed" from the user's perspective. The source of truth is the
  /// server's response — the rest is cache hygiene.
  Future<void> delete(String communityId) async {
    if (state is DeleteOutcomeInProgress) return;
    safeSetState(const DeleteOutcomeInProgress());

    try {
      await ref.read(communityRepositoryProvider).deleteCommunity(communityId);
    } on ServiceException catch (e) {
      switch (e.code) {
        case Code.failedPrecondition:
          _log.info('Delete $communityId rejected: already deleted');
          safeUpdateState((_) => const DeleteOutcomeAlreadyDeleted());
        case Code.permissionDenied:
          _log.info('Delete $communityId rejected: caller is not the owner');
          safeUpdateState((_) => const DeleteOutcomeNotOwner());
        default:
          _log.warning('Delete $communityId failed: ${e.code} ${e.message}');
          safeUpdateState((_) => DeleteOutcomeError(
                RpcErrorHandler.classify(e),
              ));
      }
      return;
    } catch (e, st) {
      _log.severe('Delete $communityId failed unexpectedly', e, st);
      safeUpdateState((_) => DeleteOutcomeError(
            RpcErrorHandler.classify(e),
          ));
      return;
    }

    // RPC succeeded. Best-effort cache fan-out.
    try {
      await _invalidateRelatedCaches(communityId);
    } catch (e, st) {
      _log.warning(
        'Delete $communityId: post-success cache fan-out failed',
        e,
        st,
      );
    }
    _log.info('Community $communityId deleted');
    safeUpdateState((_) => const DeleteOutcomeSuccess());
  }

  Future<void> _invalidateRelatedCaches(String communityId) async {
    // CommunityRepository.deleteCommunity already invalidates the
    // community row, the deleted-list, and refreshes user-communities.
    // Cover the cross-repo fan-out called out in §7.5.
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
    await ref.read(impactMetricsRepositoryProvider).invalidateAll(communityId);
    // Push the refreshed user-communities list into CommunitiesNotifier
    // so the side pane / "who can see" pickers drop the deleted community
    // immediately. Without this surfaces holding cached lists keep showing
    // the community until a manual refresh.
    await ref.read(communitiesProvider.notifier).reloadCommunities();
  }
}

/// Auto-disposes when the Delete screen leaves the tree. The screen is
/// push-and-pop; there's no need to retain state across navigations.
final deleteCommunityProvider =
    NotifierProvider.autoDispose<DeleteCommunityNotifier, DeleteOutcome>(
  DeleteCommunityNotifier.new,
);
