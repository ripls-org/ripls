import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pbenum.dart'
    show GiveawayPhase;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// Bottom sheet listing interested parties for a giveaway gear item.
///
/// Shows an "Express Interest" / "Interested" action button for non-owners,
/// followed by "SELECTED" and "INTERESTED" sections.
/// Watches [gearProvider] for live state updates after interest changes.
class GearInterestSheet extends ConsumerStatefulWidget {
  const GearInterestSheet._({required this.gearId});

  final String gearId;

  /// Shows the interest list modal bottom sheet.
  static Future<void> show(BuildContext context, String gearId) {
    return showAccessibleModal(context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (context) => GearInterestSheet._(gearId: gearId),
    );
  }

  @override
  ConsumerState<GearInterestSheet> createState() => _GearInterestSheetState();
}

class _GearInterestSheetState extends ConsumerState<GearInterestSheet> {
  bool _isActing = false;

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(gearProvider(widget.gearId));
    final people = state.gearPeople;
    if (people == null) return const SizedBox.shrink();

    final transferContext = state.transferContext;
    final isOwner = state.isOwner;
    final currentUserId = state.currentUserId;

    // Determine terminal state (completed/cancelled) — hide action button.
    final isTerminal =
        transferContext?.overallPhase == GiveawayPhase.GIVEAWAY_PHASE_COMPLETED ||
        transferContext?.overallPhase == GiveawayPhase.GIVEAWAY_PHASE_CANCELLED;

    // Split into selected (recipient) and interested.
    final recipientId = transferContext?.hasUserTransfer() ?? false
        ? transferContext!.userTransfer.recipient.id
        : null;
    final selected = people.interestedParties
        .where((u) => u.id == recipientId)
        .toList();
    final interested = people.interestedParties
        .where((u) => u.id != recipientId)
        .toList();
    final totalCount = people.interestedParties.length;

    // Check if current user has already expressed interest.
    final hasExpressedInterest = currentUserId != null &&
        people.interestedParties.any((u) => u.id == currentUserId);

    final parts = <String>[];
    if (selected.isNotEmpty) {
      parts.add(context.l10n.gearSelectedCount(selected.length));
    }
    if (interested.isNotEmpty) {
      parts.add(context.l10n.gearInterestedCount(interested.length));
    }
    final subtitle = parts.join(' \u00b7 ');

    return GlassSheet(
      child: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // Header
            Text(
              context.l10n.gearInterestedCountTitle(totalCount),
              style: TextStyle(
                fontSize: 20,
                fontWeight: FontWeight.w700,
                color: AppColors.modalTextPrimary,
              ),
            ),
            if (subtitle.isNotEmpty) ...[
              const SizedBox(height: 4),
              Text(
                subtitle,
                style: TextStyle(
                  fontSize: 14,
                  color: AppColors.modalTextSecondary,
                ),
              ),
            ],
            const SizedBox(height: 20),

            // Action button (non-owner, non-terminal only)
            if (!isOwner && !isTerminal) ...[
              _buildInterestAction(context, hasExpressedInterest),
              const SizedBox(height: 20),
            ],

            // SELECTED section
            if (selected.isNotEmpty) ...[
              _buildSectionHeader(
                context,
                context.l10n.gearSelected,
                selected.length,
              ),
              const SizedBox(height: 8),
              ...selected
                  .map((u) => _buildPersonRow(context, u, opacity: 1)),
              const SizedBox(height: 16),
            ],

            // INTERESTED section
            if (interested.isNotEmpty) ...[
              _buildSectionHeader(
                context,
                context.l10n.gearInterested,
                interested.length,
              ),
              const SizedBox(height: 8),
              ...interested
                  .map((u) => _buildPersonRow(context, u, opacity: 1)),
            ],

            // Empty state
            if (people.interestedParties.isEmpty)
              Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: Text(
                  context.l10n.gearNoInterestedYet,
                  style: TextStyle(
                    fontSize: 14,
                    color: AppColors.modalTextMuted,
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }

  Widget _buildInterestAction(BuildContext context, bool hasExpressed) {
    if (hasExpressed) {
      return _ActionButton(
        label: context.l10n.gearAlreadyInterested,
        icon: Icons.check,
        color: AppColors.transferSage,
        isLoading: _isActing,
        onTap: null,
      );
    }

    return _ActionButton(
      label: context.l10n.gearExpressInterest,
      color: AppColors.transferSage,
      isLoading: _isActing,
      onTap: () => _expressInterest(),
    );
  }

  Widget _buildSectionHeader(BuildContext context, String label, int count) {
    return Row(
      children: [
        Text(
          label,
          style: TextStyle(
            fontSize: 11,
            fontWeight: FontWeight.w700,
            letterSpacing: 0.8,
            color: AppColors.modalTextMuted,
          ),
        ),
        const SizedBox(width: 6),
        Text(
          '($count)',
          style: TextStyle(
            fontSize: 11,
            color: AppColors.modalTextMuted,
          ),
        ),
      ],
    );
  }

  Widget _buildPersonRow(BuildContext context, User user,
      {required double opacity}) {
    return Opacity(
      opacity: opacity,
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 6),
        child: Row(
          children: [
            UserAvatar(user: user, radius: 18),
            const SizedBox(width: 12),
            Text(
              user.name,
              style: TextStyle(
                fontSize: 15,
                fontWeight: FontWeight.w500,
                color: AppColors.modalTextPrimary,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _expressInterest() async {
    if (_isActing) return;
    setState(() => _isActing = true);
    try {
      await ref
          .read(gearProvider(widget.gearId).notifier)
          .expressInterest();
    } finally {
      if (mounted) setState(() => _isActing = false);
    }
  }
}

class _ActionButton extends StatelessWidget {
  const _ActionButton({
    required this.label,
    required this.color,
    required this.isLoading,
    this.icon,
    this.onTap,
  });

  final String label;
  final Color color;
  final bool isLoading;
  final IconData? icon;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: label,
      onTap: isLoading ? null : onTap,
      child: Container(
        height: 44,
        decoration: BoxDecoration(
          color: color.withValues(alpha: 0.15),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: color.withValues(alpha: 0.35)),
        ),
        child: Center(
          child: isLoading
              ? SizedBox(
                  width: 18,
                  height: 18,
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    valueColor: AlwaysStoppedAnimation(color),
                  ),
                )
              : Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    if (icon != null) ...[
                      Icon(icon, size: 16, color: color),
                      const SizedBox(width: 6),
                    ],
                    Text(
                      label,
                      style: TextStyle(
                        fontSize: 14,
                        fontWeight: FontWeight.w600,
                        color: color,
                      ),
                    ),
                  ],
                ),
        ),
      ),
    );
  }
}
