import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/viewmodels/experience_needs_view_model.dart'
    show ExperienceContributionResponse;
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/experience/needs/_sheet_chrome.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_footer_buttons.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

// ── ViewContributionSheet ───────────────────────────────────────────────────
//
// Read-only modal opened when a viewer taps a contribution they didn't
// post themselves. The title is just the item name; the body shows
// every contributor bringing that item; the footer offers an "I'll
// bring it too" affordance for joiners.

/// Modal for viewing a contribution and optionally adding the current
/// user as a co-bringer.
///
/// Takes the full list of contributions [contributions] that share the
/// same title — the sheet collapses them into a single roster so two
/// people offering the same item show as two names on one entry.
class ViewContributionSheet extends StatelessWidget {
  const ViewContributionSheet({
    super.key,
    required this.title,
    required this.contributions,
    required this.currentUserId,
    required this.experienceOwnerId,
    this.onAddMe,
    this.onRemoveMine,
  });

  /// Item name — used as the modal title.
  final String title;

  /// All contributions matching [title]. At least one is expected.
  final List<ExperienceContributionResponse> contributions;

  /// Current user id; used to decide whether to show "I'll bring it too"
  /// vs "Remove mine".
  final String currentUserId;

  /// Experience owner id; used to label owner-posted offers.
  final String experienceOwnerId;

  /// Called when the user taps "I'll bring it too". Pass null to hide
  /// the button (e.g., terminal experiences).
  final VoidCallback? onAddMe;

  /// Called when the user taps "Remove mine". Only used when the
  /// current user is in the contributor list; the caller is
  /// responsible for unclaiming linked contributions vs deleting
  /// freeform offers. Pass null to hide the affordance.
  final VoidCallback? onRemoveMine;

  /// Shows the sheet.
  static Future<void> show(
    BuildContext context, {
    required String title,
    required List<ExperienceContributionResponse> contributions,
    required String currentUserId,
    required String experienceOwnerId,
    VoidCallback? onAddMe,
    VoidCallback? onRemoveMine,
  }) {
    return showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => ViewContributionSheet(
        title: title,
        contributions: contributions,
        currentUserId: currentUserId,
        experienceOwnerId: experienceOwnerId,
        onAddMe: onAddMe,
        onRemoveMine: onRemoveMine,
      ),
    );
  }

  bool get _currentUserInList =>
      contributions.any((c) => c.contributor.id == currentUserId);

  @override
  Widget build(BuildContext context) {
    final isClaimed = contributions
        .any((c) => c.hasFromNeedId() && c.fromNeedId.isNotEmpty);
    final isOwnerOnly = contributions.length == 1 &&
        contributions.first.contributor.id == experienceOwnerId;

    final Color dotColor;
    final String statusLabel;
    if (isOwnerOnly) {
      dotColor = AppColors.experienceSageGreen;
      statusLabel = 'Handling themselves';
    } else if (isClaimed) {
      dotColor = AppColors.experienceSageGreen;
      statusLabel = contributions.length == 1
          ? 'Claimed from the request'
          : '${contributions.length} bringing this';
    } else {
      dotColor = AppColors.transferCoral;
      statusLabel = contributions.length == 1
          ? 'Freely offered'
          : '${contributions.length} bringing this';
    }

    return buildSheetContainer(
      context: context,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          buildDragHandle(context),
          const SizedBox(height: 8),
          buildStatusBadge(
            context: context,
            dotColor: dotColor,
            label: statusLabel,
          ),
          const SizedBox(height: 20),
          buildSerifTitle(context, primary: title),
          const SizedBox(height: 20),
          Divider(color: AppColors.modalFooterDivider, height: 1),
          const SizedBox(height: 12),
          for (var i = 0; i < contributions.length; i++) ...[
            _ContributorRow(contribution: contributions[i]),
            if (i < contributions.length - 1) const SizedBox(height: 10),
          ],
          if (onAddMe != null && !_currentUserInList) ...[
            const SizedBox(height: 20),
            GlassFooterButtons(
              primaryLabel: "I'll bring it too",
              primaryEnabled: true,
              onPrimary: () {
                Navigator.of(context).pop();
                onAddMe!();
              },
              showSecondary: false,
            ),
          ],
          if (onRemoveMine != null && _currentUserInList) ...[
            const SizedBox(height: 20),
            GlassFooterButtons(
              primaryLabel: 'Remove mine',
              primaryEnabled: true,
              onPrimary: () {
                Navigator.of(context).pop();
                onRemoveMine!();
              },
              showSecondary: false,
            ),
          ],
        ],
      ),
    );
  }
}

/// Single contributor row inside the modal: avatar + name (with optional
/// note shown beneath as muted secondary text).
class _ContributorRow extends StatelessWidget {
  const _ContributorRow({required this.contribution});

  final ExperienceContributionResponse contribution;

  @override
  Widget build(BuildContext context) {
    final hasNote = contribution.hasDescription() &&
        contribution.description.isNotEmpty;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        UserAvatar(user: contribution.contributor, radius: 16),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                contribution.contributor.name,
                style: const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                  color: AppColors.modalTextPrimary,
                ),
              ),
              if (hasNote) ...[
                const SizedBox(height: 2),
                Text(
                  contribution.description,
                  style: TextStyle(
                    fontSize: 13,
                    color: AppColors.modalTextSecondary,
                    height: 1.4,
                  ),
                ),
              ],
            ],
          ),
        ),
      ],
    );
  }
}
