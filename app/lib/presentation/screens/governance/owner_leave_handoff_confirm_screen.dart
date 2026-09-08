import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/screens/governance/owner_leave_confirm_screen.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';

/// OwnerLeaveHandoffConfirmScreen is §2.3 step 2 — the confirmation
/// before the leave step. Shows "Make {picked-user} the new owner?"
/// with Confirm/Cancel. Cancel pops back to the picker.
///
/// Confirm pushes the leave-confirm screen which dispatches the actual
/// `LeaveCommunity(communityId, newOwnerUserId)` RPC.
class OwnerLeaveHandoffConfirmScreen extends ConsumerWidget {
  final CommunityItem community;
  final User candidate;

  const OwnerLeaveHandoffConfirmScreen({
    super.key,
    required this.community,
    required this.candidate,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Scaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.appBarBackground(context),
        elevation: 0,
        leading: AppBarBackButton(onPressed: () => Navigator.of(context).pop()),
        title: Text(
          context.l10n.communityOwnerLeaveHandoffTitle(candidate.name),
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
                context.l10n.communityOwnerLeaveHandoffBody(candidate.name),
                style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                      color: AppColors.textPrimary(context),
                    ),
              ),
              const Spacer(),
              ElevatedButton(
                onPressed: () => _onConfirm(context),
                child: Text(context.l10n.communityOwnerLeaveHandoffCta),
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

  void _onConfirm(BuildContext context) {
    NavigationHelpers.pushWithSlide(
      context: context,
      screen: OwnerLeaveConfirmScreen(
        community: community,
        candidate: candidate,
      ),
      routeName: 'owner_leave_confirm',
    );
  }
}
