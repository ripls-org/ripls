import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';

/// One row in a [PollManageMenuSheet]. The caller resolves the icon, label,
/// and description through `context.l10n` so this widget stays
/// poll-kind-agnostic and ARB strings never live inside generic primitives.
class PollManageMenuItem<T> {
  const PollManageMenuItem({
    required this.icon,
    required this.label,
    required this.action,
    this.description,
    this.destructive = false,
  });

  /// Leading icon for the row.
  final IconData icon;

  /// Title shown on the row. Already localized.
  final String label;

  /// Optional one-line subtitle shown under the title. Already localized.
  /// When null the row collapses to label-only — used by the experience
  /// manage sheet where the action names are self-explanatory.
  final String? description;

  /// Value popped from the navigator when this row is tapped.
  final T action;

  /// When true the row renders with the error color (red), used for
  /// destructive actions like "Cancel poll".
  final bool destructive;
}

/// Generic organizer "Manage" bottom-sheet shared by the location- and
/// time-poll flows.
///
/// The caller builds a list of [PollManageMenuItem]s parameterized over an
/// action enum and pops the resulting enum value via
/// [Navigator.pop] when a row is tapped. The sheet itself does not know
/// what the actions mean — that's the calling modal's job.
class PollManageMenuSheet<T> extends StatelessWidget {
  const PollManageMenuSheet({super.key, required this.items});

  /// Ordered list of action rows. The caller decides which rows to include
  /// based on poll state (e.g. hide "Add option" when locked).
  final List<PollManageMenuItem<T>> items;

  @override
  Widget build(BuildContext context) {
    return GlassSheet(
      padding: EdgeInsets.zero,
      child: SafeArea(
        top: false,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const SizedBox(height: 8),
            for (final item in items)
              _PollManageRow(
                item: item,
                onTap: () => Navigator.of(context).pop(item.action),
              ),
            const SizedBox(height: 16),
          ],
        ),
      ),
    );
  }
}

class _PollManageRow<T> extends StatelessWidget {
  const _PollManageRow({required this.item, required this.onTap});

  final PollManageMenuItem<T> item;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final color =
        item.destructive ? AppColors.statusErrorOnDark : AppColors.modalTextPrimary;
    return Tappable(
      semanticsLabel: item.label,
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 12),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Padding(
              padding: const EdgeInsets.only(top: 2),
              child: Icon(item.icon, size: 18, color: color),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    item.label,
                    style: TextStyle(
                      color: color,
                      fontSize: 15,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                  if (item.description != null) ...[
                    const SizedBox(height: 2),
                    Text(
                      item.description!,
                      style: TextStyle(
                        color: AppColors.modalTextSecondary,
                        fontSize: 12,
                      ),
                    ),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}
