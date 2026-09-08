import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/viewmodels/experience_needs_view_model.dart'
    show ExperienceNeedResponse;
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/experience/needs/_sheet_chrome.dart';

// ── ViewNeedSheet ───────────────────────────────────────────────────────────
//
// Read-only view of a need with optional claim/remove actions for participants
// and proposers. Shares the same editorial chrome as the claim/receipt sheets.

/// Read-only bottom sheet showing a need with optional claim/remove actions.
class ViewNeedSheet extends StatelessWidget {
  const ViewNeedSheet({
    super.key,
    required this.need,
    required this.canClaim,
    required this.canRemove,
    required this.isRsvped,
    required this.onClaim,
    required this.onRemove,
  });

  final ExperienceNeedResponse need;
  final bool canClaim;
  final bool canRemove;

  /// Whether the current user already has a yes/maybe RSVP. When false and
  /// [canClaim] is true, a notice is shown that claiming will auto-RSVP them.
  final bool isRsvped;

  final VoidCallback onClaim;
  final VoidCallback onRemove;

  /// Shows the sheet and invokes [onClaim] or [onRemove] as appropriate.
  static Future<void> show(
    BuildContext context, {
    required ExperienceNeedResponse need,
    required bool canClaim,
    required bool canRemove,
    required bool isRsvped,
    required VoidCallback onClaim,
    required VoidCallback onRemove,
  }) {
    return showAccessibleModal<void>(context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => ViewNeedSheet(
        need: need,
        canClaim: canClaim,
        canRemove: canRemove,
        isRsvped: isRsvped,
        onClaim: onClaim,
        onRemove: onRemove,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final hasNote = need.hasNote() && need.note.isNotEmpty;
    final hasSlots = need.slotsRemaining > 0;

    final statusLabel = hasSlots
        ? [
            'Open',
            context.l10n.experienceNeedsSlotCount(need.slotsRemaining),
            if (need.proposer.name.isNotEmpty)
              'asked by ${need.proposer.name.split(' ').first}',
          ].join(' · ')
        : 'All claimed';

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
            dotColor: hasSlots
                ? AppColors.transferCoral
                : AppColors.modalTextMuted,
            label: statusLabel,
          ),
          const SizedBox(height: 20),
          buildSerifTitle(context, primary: need.name),
          const SizedBox(height: 20),
          if (hasNote) ...[
            buildNoteSection(context, need.note),
            const SizedBox(height: 16),
          ],
          if (canClaim && !isRsvped) ...[
            Row(
              children: [
                Icon(Icons.info_outline,
                    size: 14, color: AppColors.modalTextMuted),
                const SizedBox(width: 6),
                Expanded(
                  child: Text(
                    "You'll automatically be marked as going when you claim this.",
                    style: TextStyle(
                      fontSize: 12,
                      color: AppColors.modalTextMuted,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 12),
          ],
          if (canClaim) ...[
            buildPrimaryButton(
              context: context,
              label: "I'll bring it",
              onPressed: () {
                Navigator.of(context).pop();
                onClaim();
              },
            ),
          ],
          if (canRemove)
            buildSecondaryLink(
              context: context,
              label: context.l10n.commonRemove,
              onPressed: () {
                Navigator.of(context).pop();
                onRemove();
              },
              color: AppColors.statusErrorOnDark,
            ),
        ],
      ),
    );
  }
}
