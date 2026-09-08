import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/viewmodels/leave_community_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';

/// OwnerLeaveConfirmScreen is §2.3 step 3 / §7.3 owner branch — the
/// final leave confirmation, dispatched after the member picker and
/// handoff confirm. Calls
/// `LeaveCommunity(communityId, newOwnerUserId: candidate.id)`,
/// exercising the server's race-safe `ClaimCommunityOwnerHandoff`
/// path, then runs the §7.5 cache fan-out.
///
/// Reuses [LeaveCommunityNotifier] from the non-owner Leave flow, but
/// reacts to the four typed `FailedPrecondition` /
/// `InvalidArgument` reasons that only fire on the owner-handoff
/// path:
///   - `candidate_not_member` → pop back to the picker so the user
///     can pick again from the fresh member list.
///   - `ownership_changed` → pop all the way out (ownership has
///     transferred since the screen was opened).
///   - `missing_new_owner_user_id` / `candidate_is_caller` →
///     defensive: shouldn't fire from this path, surface as an error
///     and stay on screen.
class OwnerLeaveConfirmScreen extends ConsumerWidget {
  final CommunityItem community;
  final User candidate;

  const OwnerLeaveConfirmScreen({
    super.key,
    required this.community,
    required this.candidate,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    ref.listen<LeaveOutcome>(leaveCommunityProvider, (prev, next) {
      _onOutcomeChanged(context, next);
    });

    final outcome = ref.watch(leaveCommunityProvider);
    final inProgress = outcome is LeaveOutcomeInProgress;

    return Scaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        leading: AppBarBackButton(onPressed: () => Navigator.of(context).pop()),
        title: Text(
          context.l10n.communityLeaveTitle,
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
      ),
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const SizedBox(height: 16),
              Text(
                context.l10n.communityLeaveBody(
                  communityDisplayName(community, context.l10n),
                ),
                style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                      color: AppColors.textPrimary(context),
                    ),
              ),
              const Spacer(),
              ElevatedButton(
                style: ElevatedButton.styleFrom(
                  backgroundColor: AppColors.statusWarning(context),
                  foregroundColor: Colors.white,
                ),
                onPressed: inProgress
                    ? null
                    : () => ref
                        .read(leaveCommunityProvider.notifier)
                        .leave(community.id, newOwnerUserId: candidate.id),
                child: inProgress
                    ? const SizedBox(
                        height: 20,
                        width: 20,
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          color: Colors.white,
                        ),
                      )
                    : Text(context.l10n.communityLeaveCta),
              ),
              const SizedBox(height: 12),
              TextButton(
                onPressed: inProgress
                    ? null
                    : () => Navigator.of(context).pop(),
                child: Text(context.l10n.commonCancel),
              ),
            ],
          ),
        ),
      ),
    );
  }

  void _onOutcomeChanged(BuildContext context, LeaveOutcome outcome) {
    switch (outcome) {
      case LeaveOutcomeIdle():
      case LeaveOutcomeInProgress():
        return;
      case LeaveOutcomeSuccess():
        // Pop the four screens we pushed (this confirm, handoff
        // confirm, picker) plus the underlying GovernanceScreen,
        // landing back on Settings hub. The pushWithSlide stack from
        // governance → picker → handoff → this screen has 4 routes
        // to peel off.
        _completeWith(
          context,
          context.l10n.communityLeaveOwnerHandoffSuccess(
            communityDisplayName(community, context.l10n),
            candidate.name,
          ),
          popCount: 4,
        );
      case LeaveOutcomeCandidateNotMember():
        // Pop just this screen + the handoff confirm — back to the
        // picker so the user can pick a different member from a
        // freshly-rebuilt member list. The picker uses initState's
        // future, so re-mounting it on the next push gets fresh data.
        _completeWith(
          context,
          context.l10n.communityLeaveCandidateNotMember,
          popCount: 2,
        );
      case LeaveOutcomeOwnershipChanged():
        // Pop all the way back to Settings — ownership transferred,
        // the user is no longer the owner of this community and
        // shouldn't be in any of the owner-leave screens.
        _completeWith(
          context,
          context.l10n.communityLeaveOwnershipChanged,
          popCount: 4,
        );
      case LeaveOutcomeMissingNewOwner():
      case LeaveOutcomeCandidateIsCaller():
        // Defensive — these shouldn't fire from the picker-driven
        // flow because the picker filters out the caller and always
        // populates newOwnerUserId. Surface as an error and stay on
        // screen so the user can retry.
        _completeWith(
          context,
          outcome is LeaveOutcomeMissingNewOwner
              ? context.l10n.communityLeaveMissingNewOwner
              : context.l10n.communityLeaveCandidateIsCaller,
          popCount: 0,
        );
      case LeaveOutcomeCommunityDeleted():
        // The community got deleted by another path while the
        // owner was navigating the picker → handoff → confirm
        // sequence. Pop all the way out to Settings (4 routes).
        _completeWith(
          context,
          context.l10n.communityLeaveCommunityDeleted,
          popCount: 4,
        );
      case LeaveOutcomeNotFound():
        // Owner's membership row is gone (e.g. the picked candidate
        // already became owner via another path and the original
        // caller's row was purged). Pop all the way out.
        _completeWith(
          context,
          context.l10n.communityLeaveNotFound,
          popCount: 4,
        );
      case LeaveOutcomeError(:final error):
        _completeWith(
          context,
          RpcErrorHandler.localize(error, context.l10n),
          popCount: 0,
        );
    }
  }

  void _completeWith(
    BuildContext context,
    String message, {
    required int popCount,
  }) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(message)),
    );
    SemanticAnnouncer.announce(context, message);
    final navigator = Navigator.of(context);
    for (var i = 0; i < popCount; i++) {
      if (!navigator.canPop()) break;
      navigator.pop();
    }
  }
}
