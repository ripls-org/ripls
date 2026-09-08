import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/presentation/viewmodels/delete_community_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';
import 'package:ripls/services/providers.dart';

/// DeleteCommunityScreen is the §2.1 step 3 confirmation surface.
/// Reached from the Danger Zone choice modal's Delete-for-everyone
/// branch. Calls `DeleteCommunity` and pops back to Settings on
/// success.
///
/// Utility/settings screen — no `SwipeToCloseMixin`, reached via
/// `pushWithSlide`.
class DeleteCommunityScreen extends ConsumerWidget {
  /// The community being deleted.
  final CommunityItem community;

  const DeleteCommunityScreen({super.key, required this.community});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    ref.listen<DeleteOutcome>(deleteCommunityProvider, (prev, next) {
      _onOutcomeChanged(context, next);
    });

    final outcome = ref.watch(deleteCommunityProvider);
    final inProgress = outcome is DeleteOutcomeInProgress;
    // Read numMembers off the cached community so the body's ICU
    // plural picks the right branch ("1 member" vs. "all N
    // members"). Falling back to 1 if the data hasn't resolved yet
    // is safer than 0 — the latter would render "all 0 members"
    // before the future settles.
    final numMembers = ref
            .watch(communityProvider(community.id))
            .asData
            ?.value
            .numMembers ??
        1;

    return Scaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        leading: AppBarBackButton(onPressed: () => Navigator.of(context).pop()),
        title: Text(
          context.l10n.communityDeleteTitle,
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
                context.l10n.communityDeleteBody(
                  communityDisplayName(community, context.l10n),
                  numMembers,
                ),
                style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                      color: AppColors.textPrimary(context),
                    ),
              ),
              const Spacer(),
              ElevatedButton(
                style: ElevatedButton.styleFrom(
                  backgroundColor: AppColors.statusError(context),
                  foregroundColor: Colors.white,
                ),
                onPressed: inProgress
                    ? null
                    : () => ref
                        .read(deleteCommunityProvider.notifier)
                        .delete(community.id),
                child: inProgress
                    ? const SizedBox(
                        height: 20,
                        width: 20,
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          color: Colors.white,
                        ),
                      )
                    : Text(context.l10n.communityDeleteCta),
              ),
              const SizedBox(height: 12),
              TextButton(
                onPressed: inProgress ? null : () => Navigator.of(context).pop(),
                child: Text(context.l10n.commonCancel),
              ),
            ],
          ),
        ),
      ),
    );
  }

  void _onOutcomeChanged(BuildContext context, DeleteOutcome outcome) {
    switch (outcome) {
      case DeleteOutcomeIdle():
      case DeleteOutcomeInProgress():
        return;
      case DeleteOutcomeSuccess():
        // Community is gone — pop both the confirmation screen and the
        // GovernanceScreen for the now-deleted community, landing the
        // user back on the Settings hub. Without the second pop the
        // user sits on a community-scoped screen whose subsequent
        // queries will 404.
        _completeWith(
          context,
          context.l10n.communityDeleteSuccess(
            communityDisplayName(community, context.l10n),
          ),
          popCount: 2,
        );
      case DeleteOutcomeAlreadyDeleted():
        // Same destination: another path already deleted this
        // community, so the GovernanceScreen behind us is also stale.
        _completeWith(
          context,
          context.l10n.communityDeleteAlreadyDeleted,
          popCount: 2,
        );
      case DeleteOutcomeNotOwner():
        // Community still exists; ownership was transferred since the
        // user opened this screen. Drop them back on GovernanceScreen
        // — the community is still a normal member surface for them.
        _completeWith(
          context,
          context.l10n.communityDeleteNotOwner,
          popCount: 1,
        );
      case DeleteOutcomeError(:final error):
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
