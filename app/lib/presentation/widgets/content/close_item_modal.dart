import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';

/// ContentType identifies which kind of content is being closed, so that
/// CloseItemModal can use appropriate labels.
enum CloseItemContentType { item, event, request, loan }

/// CloseItemModal presents the owner with three options for ending a content item.
///
/// Options:
/// - Unshare from community (remove from feed without deleting)
/// - Mark as Cancelled (notify participants)
/// - Delete permanently
///
/// Generalizes [CancelEventModal] to work for gear items, events, and requests.
class CloseItemModal extends StatelessWidget {
  final CloseItemContentType contentType;
  final VoidCallback? onUnshare;
  final VoidCallback? onCancel;
  final VoidCallback? onDelete;

  const CloseItemModal({
    super.key,
    required this.contentType,
    this.onUnshare,
    this.onCancel,
    this.onDelete,
  });

  /// Shows the modal as a bottom sheet.
  static Future<void> show(
    BuildContext context, {
    required CloseItemContentType contentType,
    VoidCallback? onUnshare,
    VoidCallback? onCancel,
    VoidCallback? onDelete,
  }) async {
    await showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => CloseItemModal(
        contentType: contentType,
        onUnshare: onUnshare,
        onCancel: onCancel,
        onDelete: onDelete,
      ),
    );
  }

  String get _noun {
    switch (contentType) {
      case CloseItemContentType.item:
        return 'Item';
      case CloseItemContentType.event:
        return 'Event';
      case CloseItemContentType.request:
        return 'Request';
      case CloseItemContentType.loan:
        return 'Loan';
    }
  }

  @override
  Widget build(BuildContext context) {
    return GlassSheet(
      applyMaxHeight: false,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(4, 0, 4, 16),
            child: Semantics(
              header: true,
              child: Text(
                'Close $_noun',
                style: ModalTheme.headerValueStyle.copyWith(
                  color: AppColors.modalTextPrimary,
                  fontSize: 17,
                ),
              ),
            ),
          ),
          _OptionRow(
            icon: Icons.group_off_outlined,
            label: 'Unshare from this community',
            subtitle: 'Remove from the community feed',
            onTap: onUnshare == null
                ? null
                : () {
                    Navigator.of(context).pop();
                    onUnshare!();
                  },
          ),
          const _OptionDivider(),
          _OptionRow(
            icon: Icons.cancel_outlined,
            label: 'Mark as Cancelled',
            subtitle: 'Notify participants the $_noun is cancelled',
            isDestructive: true,
            onTap: onCancel == null
                ? null
                : () {
                    Navigator.of(context).pop();
                    onCancel!();
                  },
          ),
          const _OptionDivider(),
          _OptionRow(
            icon: Icons.delete_outline,
            label: 'Delete Permanently',
            subtitle: 'Remove this $_noun and all its data',
            isDestructive: true,
            onTap: onDelete == null
                ? null
                : () {
                    Navigator.of(context).pop();
                    onDelete!();
                  },
          ),
        ],
      ),
    );
  }
}

class _OptionDivider extends StatelessWidget {
  const _OptionDivider();

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 1,
      margin: const EdgeInsets.symmetric(horizontal: 12),
      color: AppColors.modalBorderSubtle,
    );
  }
}

class _OptionRow extends StatelessWidget {
  final IconData icon;
  final String label;
  final String subtitle;
  final VoidCallback? onTap;
  final bool isDestructive;

  const _OptionRow({
    required this.icon,
    required this.label,
    required this.subtitle,
    required this.onTap,
    this.isDestructive = false,
  });

  @override
  Widget build(BuildContext context) {
    final disabled = onTap == null;
    final color = isDestructive
        ? AppColors.statusWarningOnDark
        : AppColors.modalTextPrimary;
    final fg = disabled ? AppColors.modalTextMuted : color;

    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(12),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 12),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.center,
          children: [
            Icon(icon, color: fg, size: 22),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    label,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w600,
                      color: fg,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    subtitle,
                    style: TextStyle(
                      fontSize: 13,
                      color: AppColors.modalTextMuted,
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}
