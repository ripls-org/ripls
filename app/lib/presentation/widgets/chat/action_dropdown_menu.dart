import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// ActionDropdownItem is a single row in the [ActionDropdownMenu].
class ActionDropdownItem {
  /// Icon shown in the circular icon container on the left.
  final IconData icon;

  /// Primary label text for the action.
  final String label;

  /// Optional secondary text shown below the label.
  final String? subtitle;

  /// When true, the label is rendered in coral (destructive action).
  final bool isDestructive;

  /// When true, shows a green checkmark on the trailing edge and disables tap.
  /// Used to indicate that this workflow step has already been completed.
  final bool isCompleted;

  /// Optional widget that replaces the default circular icon container.
  /// When set, the default icon/background circle is not rendered.
  final Widget? leadingWidget;

  /// Optional child items rendered indented below this item.
  /// When set, this item renders as a non-tappable header row (avatar + name)
  /// followed by each child with additional left padding.
  final List<ActionDropdownItem>? children;

  /// Called when the row is tapped. When null the row is rendered as disabled.
  final VoidCallback? onTap;

  const ActionDropdownItem({
    required this.icon,
    required this.label,
    this.subtitle,
    this.isDestructive = false,
    this.isCompleted = false,
    this.leadingWidget,
    this.children,
    this.onTap,
  });
}

/// ActionDropdownMenu is an upward-opening dropdown panel anchored to the
/// bottom-left of a target widget.
///
/// Rendered as an [OverlayEntry] positioned using [RenderBox.localToGlobal].
/// Dismiss by tapping outside the menu (via [TapRegion]).
///
/// Usage — show via [ActionDropdownMenu.show], dismiss via the returned
/// [ActionDropdownController].
class ActionDropdownController {
  OverlayEntry? _entry;

  /// remove dismisses the dropdown if it is currently visible.
  void remove() {
    _entry?.remove();
    _entry = null;
  }

  bool get isShowing => _entry != null;
}

class ActionDropdownMenu extends StatelessWidget {
  final List<ActionDropdownItem> items;

  const ActionDropdownMenu({super.key, required this.items});

  /// show inserts the dropdown menu as an [OverlayEntry] above [anchorKey]
  /// and returns a controller that can dismiss it.
  ///
  /// [groupId] is used with [TapRegion.groupId] so that tapping a widget in
  /// the same group (e.g. the button that opened the menu) does not trigger
  /// [TapRegion.onTapOutside], allowing the button to handle toggle itself.
  static ActionDropdownController show({
    required BuildContext context,
    required GlobalKey anchorKey,
    required List<ActionDropdownItem> items,
    Object? groupId,
  }) {
    final controller = ActionDropdownController();

    final renderBox =
        anchorKey.currentContext?.findRenderObject() as RenderBox?;
    if (renderBox == null) return controller;

    final offset = renderBox.localToGlobal(Offset.zero);
    final anchorRight = offset.dx + renderBox.size.width;

    OverlayEntry? entry;
    entry = OverlayEntry(
      builder: (context) {
        final screenWidth = MediaQuery.of(context).size.width;
        // Right-align the panel to the button's right edge when it would
        // otherwise overflow the screen.
        const menuMaxWidth = 320.0;
        final overflowsRight = offset.dx + menuMaxWidth > screenWidth - 8;
        return TapRegion(
          groupId: groupId,
          onTapOutside: (_) => controller.remove(),
          child: Stack(
            children: [
              Positioned(
                left: overflowsRight ? null : offset.dx,
                right: overflowsRight ? (screenWidth - anchorRight) : null,
                bottom: MediaQuery.of(context).size.height -
                    offset.dy +
                    4, // 4px gap above button
                child: _ActionDropdownMenuPanel(
                  items: items,
                  onItemTap: () => controller.remove(),
                ),
              ),
            ],
          ),
        );
      },
    );

    controller._entry = entry;
    Overlay.of(context).insert(entry);
    return controller;
  }

  @override
  Widget build(BuildContext context) {
    return _ActionDropdownMenuPanel(items: items, onItemTap: () {});
  }
}

class _ActionDropdownMenuPanel extends StatelessWidget {
  final List<ActionDropdownItem> items;
  final VoidCallback onItemTap;

  const _ActionDropdownMenuPanel({
    required this.items,
    required this.onItemTap,
  });

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.transparent,
      child: Container(
        constraints: const BoxConstraints(minWidth: 240, maxWidth: 320),
        decoration: BoxDecoration(
          color: Theme.of(context).brightness == Brightness.light
              ? Colors.white
              : AppColors.darkCardBackground,
          borderRadius: BorderRadius.circular(14),
          boxShadow: [
            BoxShadow(
              color: Colors.black.withValues(alpha: 0.15),
              blurRadius: 16,
              offset: const Offset(0, 4),
            ),
          ],
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: _buildAllRows(context),
        ),
      ),
    );
  }

  List<Widget> _buildAllRows(BuildContext context) {
    final widgets = <Widget>[];
    for (var i = 0; i < items.length; i++) {
      final item = items[i];
      final hasChildren = item.children != null && item.children!.isNotEmpty;

      // Divider before grouped sections (items with children).
      if (hasChildren && i > 0) {
        widgets.add(Divider(
          height: 1,
          thickness: 1,
          color: AppColors.divider(context).withValues(alpha: 0.5),
          indent: 16,
          endIndent: 16,
        ));
      }

      // Render the item itself.
      widgets.add(_buildRow(context, item, isHeader: hasChildren));

      if (hasChildren) {
        // Render each child with indentation.
        for (final child in item.children!) {
          widgets.add(_buildRow(context, child, indented: true));
        }
        // Divider after grouped section (if not last item).
        if (i < items.length - 1) {
          widgets.add(Divider(
            height: 1,
            thickness: 1,
            color: AppColors.divider(context).withValues(alpha: 0.5),
            indent: 16,
            endIndent: 16,
          ));
        }
      } else if (i < items.length - 1 &&
          !(items[i + 1].children != null &&
              items[i + 1].children!.isNotEmpty)) {
        // Regular divider between non-grouped items.
        widgets.add(Divider(
          height: 1,
          thickness: 1,
          color: AppColors.divider(context).withValues(alpha: 0.5),
          indent: 16,
          endIndent: 16,
        ));
      }
    }
    return widgets;
  }

  Widget _buildRow(
    BuildContext context,
    ActionDropdownItem item, {
    bool isHeader = false,
    bool indented = false,
  }) {
    final isCompleted = item.isCompleted;
    final labelColor = isCompleted
        ? AppColors.textSecondary(context)
        : item.isDestructive
            ? AppColors.transferCoral
            : AppColors.textPrimary(context);
    final iconColor = isCompleted
        ? AppColors.textSecondary(context)
        : item.isDestructive
            ? AppColors.transferCoral
            : AppColors.textSecondary(context);
    final iconBg = isCompleted
        ? AppColors.surface(context)
        : item.isDestructive
            ? AppColors.transferCoral.withValues(alpha: 0.1)
            : AppColors.surface(context);

    // Headers (items with children) are non-tappable.
    final isTappable = !isHeader && !isCompleted && item.onTap != null;
    final leftPadding = indented ? 32.0 : 16.0;

    final isDisabled = !isHeader && !isCompleted && item.onTap == null;
    return Opacity(
      opacity: isCompleted || isDisabled ? 0.4 : 1.0,
      child: Tappable(
        semanticsLabel: item.label,
        onTap: isTappable
            ? () {
                onItemTap();
                item.onTap!();
              }
            : null,
        inkBorderRadius: BorderRadius.circular(14),
        child: Padding(
          padding: EdgeInsets.only(
            left: leftPadding,
            right: 16,
            top: 12,
            bottom: 12,
          ),
          child: Row(
            children: [
              if (item.leadingWidget != null)
                SizedBox(width: 36, height: 36, child: item.leadingWidget!)
              else
                Container(
                  width: 36,
                  height: 36,
                  decoration: BoxDecoration(
                    color: iconBg,
                    shape: BoxShape.circle,
                  ),
                  child: Icon(item.icon, size: 18, color: iconColor),
                ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      item.label,
                      style: TextStyle(
                        fontSize: 14,
                        fontWeight: FontWeight.w600,
                        color: labelColor,
                      ),
                    ),
                    if (item.subtitle != null) ...[
                      const SizedBox(height: 2),
                      Text(
                        item.subtitle!,
                        style: TextStyle(
                          fontSize: 12,
                          color: AppColors.textSecondary(context),
                        ),
                      ),
                    ],
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
