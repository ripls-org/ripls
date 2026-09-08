import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/errors/user_error.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/presentation/viewmodels/leave_community_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';

/// LeaveCommunityScreen is the §7.3 confirmation surface for
/// leaving a community. Reached from the membership-section
/// **Leave Community** entry on the governance screen. Non-owner
/// branch only — the owner-leave-with-handoff path (#1719) goes
/// through a separate picker → handoff confirm → leave confirm
/// sequence that ultimately dispatches to this same VM with
/// `newOwnerUserId` set.
///
/// Utility/settings screen — no `SwipeToCloseMixin`, reached via
/// `pushWithSlide`.
class LeaveCommunityScreen extends ConsumerWidget {
  /// The community to leave.
  final CommunityItem community;

  const LeaveCommunityScreen({super.key, required this.community});

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
                        .leave(community.id),
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
        // Pop both the leave screen and the GovernanceScreen for the
        // now-left community, landing the user back on Settings.
        // Subsequent queries against this community would 403 / 404
        // server-side, so don't leave the user sitting on it.
        _completeWith(
          context,
          context.l10n.communityLeaveSuccess(
            communityDisplayName(community, context.l10n),
          ),
          popCount: 2,
        );
      case LeaveOutcomeMissingNewOwner():
      case LeaveOutcomeCandidateIsCaller():
      case LeaveOutcomeCandidateNotMember():
      case LeaveOutcomeOwnershipChanged():
        // The owner-leave-with-handoff path (#1719) handles these
        // outcomes through its own dedicated screens. The non-owner
        // Leave path that this screen serves should never produce
        // them (the server only enforces these against an owner-leave
        // call, which requires newOwnerUserId — non-owners pass null).
        // If one fires here it indicates a server-side regression;
        // surface as a generic error so the user isn't stranded.
        _completeWith(
          context,
          RpcErrorHandler.localize(
            const UserError.generic(),
            context.l10n,
          ),
          popCount: 0,
        );
      case LeaveOutcomeCommunityDeleted():
        // The community got deleted by another path while the user
        // was on this screen. Pop back to Settings — the community
        // is no longer meaningfully accessible.
        _completeWith(
          context,
          context.l10n.communityLeaveCommunityDeleted,
          popCount: 2,
        );
      case LeaveOutcomeNotFound():
        // The caller's membership row is gone entirely (purge or
        // hard-delete). Pop back to Settings.
        _completeWith(
          context,
          context.l10n.communityLeaveNotFound,
          popCount: 2,
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
