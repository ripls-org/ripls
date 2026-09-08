import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// Bottom sheet listing borrowers for a loan gear item.
///
/// Shows "CURRENTLY BORROWING" (full opacity) and "PAST BORROWERS" (0.7 opacity)
/// sections. Watches [gearProvider] for live state updates.
class GearBorrowerSheet extends ConsumerWidget {
  const GearBorrowerSheet._({required this.gearId});

  final String gearId;

  /// Shows the borrower list modal bottom sheet.
  static Future<void> show(BuildContext context, String gearId) {
    return showAccessibleModal(context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (context) => GearBorrowerSheet._(gearId: gearId),
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(gearProvider(gearId));
    final people = state.gearPeople;
    if (people == null) return const SizedBox.shrink();

    final hasCurrent = people.hasCurrentBorrower();
    final currentBorrower = hasCurrent ? people.currentBorrower : null;
    final pastBorrowers = people.pastBorrowers;
    final queuedUsers = (state.transferContext?.pendingRequests ?? [])
        .where((r) => r.hasBorrower())
        .map((r) => r.borrower)
        .toList();
    final totalCount =
        (hasCurrent ? 1 : 0) + pastBorrowers.length + queuedUsers.length;

    final parts = <String>[];
    if (hasCurrent) {
      parts.add(context.l10n.gearActiveCount(1));
    }
    if (queuedUsers.isNotEmpty) {
      parts.add('${queuedUsers.length} queued');
    }
    if (pastBorrowers.isNotEmpty) {
      parts.add(context.l10n.gearReturnedCount(pastBorrowers.length));
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
              context.l10n.gearBorrowersCount(totalCount),
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

            // Empty state
            if (currentBorrower == null &&
                queuedUsers.isEmpty &&
                pastBorrowers.isEmpty)
              Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: Text(
                  context.l10n.gearNoBorrowersYet,
                  style: TextStyle(
                    fontSize: 14,
                    color: AppColors.modalTextMuted,
                  ),
                ),
              ),

            // CURRENTLY BORROWING section
            if (currentBorrower != null) ...[
              _buildSectionHeader(
                context,
                context.l10n.gearCurrentlyBorrowing,
                1,
              ),
              const SizedBox(height: 8),
              _buildPersonRow(context, currentBorrower, opacity: 1),
              const SizedBox(height: 16),
            ],

            // QUEUE section
            if (queuedUsers.isNotEmpty) ...[
              _buildSectionHeader(context, 'QUEUE', queuedUsers.length),
              const SizedBox(height: 8),
              ...queuedUsers
                  .map((u) => _buildPersonRow(context, u, opacity: 1)),
              const SizedBox(height: 16),
            ],

            // PAST BORROWERS section
            if (pastBorrowers.isNotEmpty) ...[
              _buildSectionHeader(
                context,
                context.l10n.gearPastBorrowers,
                pastBorrowers.length,
              ),
              const SizedBox(height: 8),
              ...pastBorrowers
                  .map((u) => _buildPersonRow(context, u, opacity: 0.7)),
            ],
          ],
        ),
      ),
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
}
