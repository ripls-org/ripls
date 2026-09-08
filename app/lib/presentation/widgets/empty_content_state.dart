import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/screens/communities/community_creation_modal.dart';
import 'package:ripls/presentation/screens/communities/invite_sheet.dart';
import 'package:ripls/presentation/screens/create/blank_create_dispatcher.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/feedback/feedback_sheet.dart';
import 'package:ripls/presentation/widgets/plus_button_modal.dart';
import 'package:ripls/services/post_creation_service.dart';
import 'package:ripls/services/providers.dart';

/// A reusable empty state widget that displays when there's no content.
/// Shows an icon, title, message, and action buttons (refresh and share).
class EmptyContentState extends ConsumerWidget {
  final IconData icon;
  final String title;
  final String message;
  final VoidCallback? onRefresh;
  final Color? backgroundColor;

  const EmptyContentState({
    super.key,
    required this.icon,
    required this.title,
    required this.message,
    this.onRefresh,
    this.backgroundColor,
  });

  Future<void> _onCreateCommunityTap(BuildContext context, WidgetRef ref) async {
    // Post-create sequence (reloadCommunities, invalidate feed, navigate to
    // Feed tab) is owned by gen_community_view_model.createCommunity, which
    // the preview modal invokes during creation. Callers just await dismissal.
    await CommunityCreationModal.show(context);
  }

  void _showPlusButtonModal(BuildContext context, WidgetRef ref) {
    showAccessibleModal(context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (context) => PlusButtonModal(
        onShareGear: () async {
          await openBlankCreateGear(context, ref);
        },
        onInviteExperience: () async {
          await openBlankCreate(context, ref);
        },
        onRequestSomething: () async {
          final communityState = ref.read(communitiesProvider);

          if (communityState.communities.isEmpty) {
            ScaffoldMessenger.of(context).showSnackBar(
              SnackBar(content: Text(context.l10n.requestCreateNoCommunities)),
            );
            return;
          }

          final requestId = await openBlankCreateRequest(context, ref);

          if (requestId != null && context.mounted) {
            await ref.read(postCreationServiceProvider).handlePostCreation();
          }
        },
        onInviteUser: () async {
          final communityState = ref.read(communitiesProvider);
          final communities = communityState.communities;

          if (communities.isEmpty) {
            ScaffoldMessenger.of(context).showSnackBar(
              const SnackBar(content: Text('Join a community to invite someone')),
            );
            return;
          }

          await showAccessibleModal(context,
            isScrollControlled: true,
            backgroundColor: Colors.transparent,
            builder: (context) => InviteSheet(
              communityId: communities.first.id,
              communityName: communities.first.name,
              communities: communities,
              onClose: () => Navigator.of(context).pop(),
            ),
          );
        },
        onClose: () => Navigator.of(context).pop(),
      ),
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final bgColor = backgroundColor ?? AppColors.background(context);

    return Container(
      color: bgColor,
      child: Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(icon, color: AppColors.textSecondary(context), size: 64),
            const SizedBox(height: 16),
            Text(
              title,
              style: TextStyle(
                color: AppColors.textPrimary(context),
                fontSize: 18,
                fontWeight: FontWeight.bold,
              ),
            ),
            const SizedBox(height: 8),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 32),
              child: Text(
                message,
                style: TextStyle(
                  color: AppColors.textSecondary(context),
                  fontSize: 14,
                ),
                textAlign: TextAlign.center,
              ),
            ),
            const SizedBox(height: 24),
            Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: _buildPrimaryActions(context, ref),
            ),
            const SizedBox(height: 8),
            TextButton.icon(
              onPressed: () => FeedbackSheet.show(context),
              icon: Icon(
                Icons.chat_bubble_outline,
                size: 16,
                color: AppColors.textSecondary(context),
              ),
              label: Text(
                context.l10n.emptyStateFeedbackLink,
                style: TextStyle(
                  color: AppColors.textSecondary(context),
                  fontSize: 13,
                  fontWeight: FontWeight.w500,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  List<Widget> _buildPrimaryActions(BuildContext context, WidgetRef ref) {
    return [
      if (onRefresh != null) ...[
        ElevatedButton.icon(
          onPressed: onRefresh,
          icon: const Icon(Icons.refresh, size: 20),
          label: const Text('Refresh'),
          style: ElevatedButton.styleFrom(
            backgroundColor: AppColors.primary(context),
            foregroundColor: AppColors.onPrimary(context),
            padding: const EdgeInsets.symmetric(
              horizontal: 20,
              vertical: 12,
            ),
          ),
        ),
        const SizedBox(width: 12),
      ],
      if (ref.watch(communitiesProvider).communities.isEmpty)
        ElevatedButton.icon(
          onPressed: () => _onCreateCommunityTap(context, ref),
          icon: const Icon(Icons.add, size: 20),
          label: const Text('Community'),
          style: ElevatedButton.styleFrom(
            backgroundColor: AppColors.primary(context),
            foregroundColor: AppColors.onPrimary(context),
            padding: const EdgeInsets.symmetric(
              horizontal: 20,
              vertical: 12,
            ),
          ),
        )
      else
        ElevatedButton.icon(
          onPressed: () => _showPlusButtonModal(context, ref),
          icon: const Icon(Icons.add, size: 20),
          label: const Text('Share'),
          style: ElevatedButton.styleFrom(
            backgroundColor: AppColors.primary(context),
            foregroundColor: AppColors.onPrimary(context),
            padding: const EdgeInsets.symmetric(
              horizontal: 20,
              vertical: 12,
            ),
          ),
        ),
    ];
  }
}
