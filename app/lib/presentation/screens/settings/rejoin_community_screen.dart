import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/presentation/viewmodels/rejoin_community_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';
import 'package:ripls/services/providers.dart';

/// RejoinCommunityScreen is the §2.6 confirmation surface for rejoining
/// a community the caller previously left within the 30-day window.
/// Reached from the Settings → Communities recently-left section.
///
/// Utility/settings screen — no `SwipeToCloseMixin`, reached via
/// `pushWithSlide`.
class RejoinCommunityScreen extends ConsumerWidget {
  /// The rejoinable community item (carries id, name, description,
  /// mediaIds, leftAtUnixSec). The screen needs the name for the body
  /// text and snackbar; the rest is incidental.
  final RejoinableCommunityItem community;

  const RejoinCommunityScreen({super.key, required this.community});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    ref.listen<RejoinOutcome>(rejoinCommunityProvider, (prev, next) {
      _onOutcomeChanged(context, ref, next);
    });

    final outcome = ref.watch(rejoinCommunityProvider);
    final inProgress = outcome is RejoinOutcomeInProgress;

    return Scaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        leading: AppBarBackButton(onPressed: () => Navigator.of(context).pop()),
        title: Text(
          context.l10n.communityRejoinTitle,
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
                context.l10n.communityRejoinBody(community.name),
                style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                      color: AppColors.textPrimary(context),
                    ),
              ),
              const Spacer(),
              ElevatedButton(
                onPressed: inProgress
                    ? null
                    : () => ref
                        .read(rejoinCommunityProvider.notifier)
                        .rejoin(community.id),
                child: inProgress
                    ? const SizedBox(
                        height: 20,
                        width: 20,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : Text(context.l10n.communityRejoinCta),
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
    RejoinOutcome outcome,
  ) {
    switch (outcome) {
      case RejoinOutcomeIdle():
      case RejoinOutcomeInProgress():
        return;
      case RejoinOutcomeSuccess():
        _success(context, ref);
      case RejoinOutcomeAlreadyMember():
        // Treat the same as success — community is accessible. Drop
        // the user into it.
        _success(
          context,
          ref,
          messageOverride: context.l10n.communityRejoinAlreadyMember,
        );
      case RejoinOutcomeWindowExpired():
        _completeWith(
          context,
          context.l10n.communityRejoinWindowExpired,
          popDestination: _PopDestination.home,
        );
      case RejoinOutcomeCommunityDeleted():
        // Stale entry — invalidate the local list so the row
        // disappears on the next read.
        ref.read(communityRepositoryProvider).invalidateRejoinableList();
        _completeWith(
          context,
          context.l10n.communityRejoinCommunityDeleted,
          popDestination: _PopDestination.settings,
        );
      case RejoinOutcomeNoMembership():
        ref.read(communityRepositoryProvider).invalidateRejoinableList();
        _completeWith(
          context,
          context.l10n.communityRejoinNoMembership,
          popDestination: _PopDestination.settings,
        );
      case RejoinOutcomeError(:final error):
        _completeWith(
          context,
          RpcErrorHandler.localize(error, context.l10n),
          popDestination: _PopDestination.stay,
        );
    }
  }

  void _success(
    BuildContext context,
    WidgetRef ref, {
    String? messageOverride,
  }) {
    final message =
        messageOverride ?? context.l10n.communityRejoinSuccess(community.name);
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(message)),
    );
    SemanticAnnouncer.announce(context, message);

    // Pop the rejoin screen so we land back on Settings. After #1895 the
    // per-community filter is gone — the rejoined community is already
    // visible portfolio-wide via the reloadCommunities() fan-out the
    // notifier ran before this listener fired, so there is nothing to
    // "select."
    final navigator = Navigator.of(context);
    if (navigator.canPop()) navigator.pop();
  }

  void _completeWith(
    BuildContext context,
    String message, {
    required _PopDestination popDestination,
  }) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(message)),
    );
    SemanticAnnouncer.announce(context, message);

    final navigator = Navigator.of(context);
    switch (popDestination) {
      case _PopDestination.stay:
        return;
      case _PopDestination.settings:
        if (navigator.canPop()) navigator.pop();
      case _PopDestination.home:
        // Pop back through the stack until the first route — that's
        // the home screen when Settings was opened from there.
        // context.go is forbidden for in-app navigation per
        // docs/client/architecture.md.
        navigator.popUntil((route) => route.isFirst);
    }
  }
}

enum _PopDestination { stay, settings, home }
