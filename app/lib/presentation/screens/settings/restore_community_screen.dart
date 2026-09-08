import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/viewmodels/deleted_communities_view_model.dart';
import 'package:ripls/presentation/viewmodels/restore_community_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';

/// RestoreCommunityScreen is the confirmation surface for restoring a
/// previously soft-deleted community. It is a settings/utility screen
/// reached via `NavigationHelpers.pushWithSlide` from the deleted-list
/// row, or directly via the `/settings/communities/<id>/restore`
/// deep-link target hit by the day-before-purge push notification.
///
/// Per `docs/community_delete_and_leave.md` §7.4 the screen carries
/// one CTA — Restore Community — and a Cancel back-arrow. On
/// success the screen pops and a snackbar announces the result. Error
/// branches map RPC codes to outcome-specific snackbar copy without
/// requiring widgets to inspect raw `Code` values.
class RestoreCommunityScreen extends ConsumerWidget {
  /// The community that will be restored. Required.
  final String communityId;

  /// The community's display name. Used for the body text and the
  /// success snackbar — stored on the route arguments so the screen
  /// can render before any network fetch resolves.
  final String communityName;

  const RestoreCommunityScreen({
    super.key,
    required this.communityId,
    required this.communityName,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    ref.listen<RestoreOutcome>(restoreCommunityProvider, (prev, next) {
      _onOutcomeChanged(context, ref, next);
    });

    final outcome = ref.watch(restoreCommunityProvider);
    final inProgress = outcome is RestoreOutcomeInProgress;

    return Scaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        leading: AppBarBackButton(onPressed: () => Navigator.of(context).pop()),
        title: Text(
          context.l10n.communityRestoreTitle,
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
                context.l10n.communityRestoreBody(communityName),
                style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                      color: AppColors.textPrimary(context),
                    ),
              ),
              const Spacer(),
              ElevatedButton(
                onPressed: inProgress
                    ? null
                    : () => ref
                        .read(restoreCommunityProvider.notifier)
                        .restore(communityId),
                child: inProgress
                    ? const SizedBox(
                        height: 20,
                        width: 20,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : Text(context.l10n.communityRestoreCta),
              ),
            ],
          ),
        ),
      ),
    );
  }

  void _onOutcomeChanged(
    BuildContext context,
    WidgetRef ref,
    RestoreOutcome outcome,
  ) {
    switch (outcome) {
      case RestoreOutcomeIdle():
      case RestoreOutcomeInProgress():
        return;
      case RestoreOutcomeSuccess():
        _completeWith(
          context,
          ref,
          context.l10n.communityRestoreSuccess(communityName),
        );
      case RestoreOutcomeAlreadyRestored():
        _completeWith(
          context,
          ref,
          context.l10n.communityRestoreAlreadyRestored,
        );
      case RestoreOutcomeNoLongerEligible():
        _completeWith(
          context,
          ref,
          context.l10n.communityRestoreNoLongerEligible,
        );
      case RestoreOutcomeError(:final error):
        _completeWith(
          context,
          ref,
          RpcErrorHandler.localize(error, context.l10n),
        );
    }
  }

  void _completeWith(BuildContext context, WidgetRef ref, String message) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(message)),
    );
    SemanticAnnouncer.announce(context, message);
    if (Navigator.of(context).canPop()) {
      Navigator.of(context).pop();
    }
  }
}

/// Resolves the community name from the deleted-list and forwards to
/// [RestoreCommunityScreen]. Used by the deep-link entry where the
/// push notification carries only the community ID, not the name.
class RestoreCommunityScreenById extends ConsumerWidget {
  final String communityId;

  const RestoreCommunityScreenById({super.key, required this.communityId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final asyncList = ref.watch(deletedCommunitiesProvider);
    return asyncList.when(
      data: (items) {
        final match = items.where((i) => i.id == communityId).firstOrNull;
        if (match == null) {
          return _NotEligibleScaffold(communityId: communityId);
        }
        return RestoreCommunityScreen(
          communityId: communityId,
          communityName: match.name,
        );
      },
      loading: () => const Scaffold(
        body: Center(child: CircularProgressIndicator()),
      ),
      error: (_, _) => _NotEligibleScaffold(communityId: communityId),
    );
  }
}

class _NotEligibleScaffold extends StatelessWidget {
  final String communityId;

  const _NotEligibleScaffold({required this.communityId});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        leading: AppBarBackButton(onPressed: () => Navigator.of(context).pop()),
        title: Text(
          context.l10n.communityRestoreTitle,
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
      ),
      body: Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Text(
            context.l10n.communityRestoreNoLongerEligible,
            textAlign: TextAlign.center,
            style: TextStyle(color: AppColors.textPrimary(context)),
          ),
        ),
      ),
    );
  }
}
