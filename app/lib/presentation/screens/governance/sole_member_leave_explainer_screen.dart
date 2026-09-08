import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/presentation/screens/governance/delete_community_screen.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';

/// SoleMemberLeaveExplainerScreen is §2.4 step 1 — the educational
/// surface shown when the only member of a community taps Leave.
/// Reframes the action as a 30-day delete (a sole-member leave and
/// an owner-initiated delete produce the same server outcome: the
/// community is soft-deleted and the caller appears on the
/// eligible-restorer snapshot).
///
/// Continue swaps this screen for [DeleteCommunityScreen] via
/// `pushReplacement` so DeleteCommunityScreen's pop-2-on-success
/// lands the user back on the Settings hub. Cancel pops back to
/// the governance screen.
class SoleMemberLeaveExplainerScreen extends StatelessWidget {
  /// The community the caller is the sole member of.
  final CommunityItem community;

  const SoleMemberLeaveExplainerScreen({super.key, required this.community});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        leading: AppBarBackButton(onPressed: () => Navigator.of(context).pop()),
        title: Text(
          context.l10n.communityLeaveSoleMemberTitle,
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
                context.l10n.communityLeaveSoleMemberExplainerBody(
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
                onPressed: () => Navigator.of(context).pushReplacement(
                  PageRouteBuilder(
                    pageBuilder: (_, _, _) =>
                        DeleteCommunityScreen(community: community),
                    transitionsBuilder:
                        NavigationHelpers.slideFromRightTransition,
                    transitionDuration: const Duration(milliseconds: 250),
                    reverseTransitionDuration:
                        const Duration(milliseconds: 200),
                  ),
                ),
                child: Text(
                  context.l10n.communityLeaveSoleMemberContinueCta,
                ),
              ),
              const SizedBox(height: 12),
              TextButton(
                onPressed: () => Navigator.of(context).pop(),
                child: Text(context.l10n.commonCancel),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
