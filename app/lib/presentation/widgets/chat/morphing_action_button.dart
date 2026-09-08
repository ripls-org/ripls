import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/action_dropdown_menu.dart';

/// ActionButtonStatus describes the visual + interactive state of a
/// [MorphingActionButton].
enum ActionButtonStatus {
  /// Pre-action: coral CTA pill. Tapping calls [onAction].
  preAction,

  /// Post-action: sage confirmed pill with dot + label + chevron.
  /// Tapping opens the [menuItems] dropdown.
  postAction,

  /// Terminal: the workflow was cancelled. Button is visually muted and not
  /// tappable.
  terminal,

  /// Completed: the workflow finished successfully. Button is green with a
  /// checkmark icon and is not tappable.
  completed,
}

/// MorphingActionButton mirrors the action bar pattern used in content view
/// screens (gear, experience, request) — a coral CTA morphs to a sage
/// confirmed pill after the user takes action.
///
/// In [ActionButtonStatus.postAction] the button opens an upward-anchored
/// [ActionDropdownMenu] on tap. The menu is dismissed when an item is tapped
/// or the user taps outside.
///
/// This is a StatefulWidget because dropdown open/close state is purely local
/// UI state with no business logic — per architecture.md: "only use setState()
/// for local UI state like text field focus."
class MorphingActionButton extends StatefulWidget {
  /// Label shown in the coral CTA state.
  final String preActionLabel;

  /// Label shown in the sage confirmed state.
  final String postActionLabel;

  /// Current status controlling the visual appearance and interaction.
  final ActionButtonStatus status;

  /// Called when the CTA is tapped in [ActionButtonStatus.preAction] state.
  /// When null the CTA is rendered as disabled.
  final VoidCallback? onAction;

  /// Items shown in the dropdown when in [ActionButtonStatus.postAction] state.
  final List<ActionDropdownItem> menuItems;

  /// Called when the confirmed pill is tapped and [menuItems] is empty.
  ///
  /// Content views use this to navigate to chat without a dropdown. When
  /// [menuItems] is non-empty, the dropdown is opened instead.
  final VoidCallback? onConfirmedTap;

  /// When true, shows a loading spinner in place of the label.
  final bool isLoading;

  /// When true, uses the flat action bar style: solid fill with no border
  /// radius (parent [ClipRRect] handles corners), bold text (w700), fills
  /// the entire segment. Used in content view action bars (gear, request,
  /// experience) where the button sits inside a segmented bar.
  ///
  /// When false (default), uses the pill style with radius 16 and w600
  /// text — used in the conversation screen bottom bar.
  final bool flat;

  /// Optional color override for the post-action (confirmed) pill.
  /// Defaults to [AppColors.transferSage] when null.
  final Color? postActionColor;

  const MorphingActionButton({
    super.key,
    required this.preActionLabel,
    required this.postActionLabel,
    required this.status,
    this.onAction,
    this.menuItems = const [],
    this.onConfirmedTap,
    this.isLoading = false,
    this.flat = false,
    this.postActionColor,
  });

  @override
  State<MorphingActionButton> createState() => _MorphingActionButtonState();
}

class _MorphingActionButtonState extends State<MorphingActionButton> {
  final GlobalKey _anchorKey = GlobalKey();
  final Object _dropdownGroupId = Object();
  ActionDropdownController? _dropdownController;

  @override
  void dispose() {
    _dropdownController?.remove();
    super.dispose();
  }

  void _toggleDropdown() {
    if (_dropdownController?.isShowing ?? false) {
      _dropdownController!.remove();
      setState(() => _dropdownController = null);
      return;
    }

    if (widget.menuItems.isEmpty) {
      widget.onConfirmedTap?.call();
      return;
    }

    setState(() {
      _dropdownController = ActionDropdownMenu.show(
        context: context,
        anchorKey: _anchorKey,
        items: widget.menuItems,
        groupId: _dropdownGroupId,
      );
    });
  }

  @override
  Widget build(BuildContext context) {
    switch (widget.status) {
      case ActionButtonStatus.preAction:
        return _buildCTA(context);
      case ActionButtonStatus.postAction:
        return _buildConfirmedPill(context);
      case ActionButtonStatus.terminal:
        return _buildTerminal(context);
      case ActionButtonStatus.completed:
        return _buildCompleted(context);
    }
  }

  /// Coral CTA: solid coral background, white text.
  ///
  /// Flat mode (content view action bar): no border radius (parent ClipRRect
  /// handles corners), fills entire segment, bold text (w700).
  /// Pill mode (conversation bottom bar): radius 16, normal padding, w600.
  Widget _buildCTA(BuildContext context) {
    if (widget.flat) {
      return _buildFlatButton(
        color: widget.onAction != null && !widget.isLoading
            ? AppColors.transferCoral
            : AppColors.transferCoral.withValues(alpha: 0.5),
        label: widget.preActionLabel,
        textColor: AppColors.darkBackground,
        onTap: widget.isLoading ? null : widget.onAction,
      );
    }

    return Tappable(
      semanticsLabel: widget.preActionLabel,
      onTap: widget.isLoading ? null : widget.onAction,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 9),
        decoration: BoxDecoration(
          color: widget.onAction != null && !widget.isLoading
              ? AppColors.transferCoral
              : AppColors.transferCoral.withValues(alpha: 0.5),
          borderRadius: BorderRadius.circular(12),
        ),
        child: Center(
          child: widget.isLoading
              ? const SizedBox(
                  width: 18,
                  height: 18,
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    valueColor: AlwaysStoppedAnimation<Color>(
                      AppColors.darkBackground,
                    ),
                  ),
                )
              : Text(
                  widget.preActionLabel,
                  style: const TextStyle(
                    color: AppColors.darkBackground,
                    fontSize: 13,
                    fontWeight: FontWeight.w700,
                  ),
                ),
        ),
      ),
    );
  }

  /// Sage confirmed state.
  ///
  /// Flat mode (content view action bar): fills segment, no border radius,
  /// w700, no chevron — tapping calls [onConfirmedTap] to navigate to chat.
  /// Pill mode (conversation bottom bar): solid sage, radius 16, w600, with
  /// chevron that opens the manage dropdown.
  Widget _buildConfirmedPill(BuildContext context) {
    final pillColor = widget.postActionColor ?? AppColors.transferSage;

    if (widget.flat) {
      return TapRegion(
        groupId: _dropdownGroupId,
        child: _buildFlatButton(
          key: _anchorKey,
          color: pillColor,
          label: widget.postActionLabel,
          onTap: _toggleDropdown,
        ),
      );
    }

    final isOpen = _dropdownController?.isShowing ?? false;

    return TapRegion(
      groupId: _dropdownGroupId,
      child: Tappable(
        key: _anchorKey,
        semanticsLabel: widget.postActionLabel,
        onTap: _toggleDropdown,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 9),
          decoration: BoxDecoration(
            color: pillColor,
            borderRadius: BorderRadius.circular(12),
          ),
          child: Center(
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  widget.postActionLabel,
                  style: const TextStyle(
                    color: Colors.white,
                    fontSize: 13,
                    fontWeight: FontWeight.w700,
                  ),
                ),
                if (widget.menuItems.isNotEmpty) ...[
                  const SizedBox(width: 4),
                  AnimatedRotation(
                    turns: isOpen ? 0.5 : 0.0,
                    duration: accessibleDuration(
                      context,
                      const Duration(milliseconds: 200),
                    ),
                    child: const Icon(
                      Icons.keyboard_arrow_down,
                      size: 16,
                      color: Colors.white,
                    ),
                  ),
                ],
              ],
            ),
          ),
        ),
      ),
    );
  }

  /// Successfully completed state: green background, white checkmark + label,
  /// not tappable.
  Widget _buildCompleted(BuildContext context) {
    const color = AppColors.transferSage;
    if (widget.flat) {
      return Container(
        constraints: const BoxConstraints(minWidth: 90),
        color: color,
        alignment: Alignment.center,
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        child: const Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.check, color: Colors.white, size: 14),
            SizedBox(width: 4),
            Text(
              'Completed',
              style: TextStyle(
                color: Colors.white,
                fontSize: 13,
                fontWeight: FontWeight.w700,
              ),
            ),
          ],
        ),
      );
    }

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      decoration: BoxDecoration(
        color: color,
        borderRadius: BorderRadius.circular(16),
      ),
      child: const Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.check, color: Colors.white, size: 14),
          SizedBox(width: 4),
          Text(
            'Completed',
            style: TextStyle(
              color: Colors.white,
              fontSize: 13,
              fontWeight: FontWeight.w600,
            ),
          ),
        ],
      ),
    );
  }

  /// Disabled terminal state: muted background, muted text, not tappable.
  Widget _buildTerminal(BuildContext context) {
    if (widget.flat) {
      return _buildFlatButton(
        color: AppColors.textTertiary(context).withValues(alpha: 0.15),
        label: widget.postActionLabel,
        textColor: AppColors.textTertiary(context),
      );
    }

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      decoration: BoxDecoration(
        color: AppColors.textTertiary(context).withValues(alpha: 0.15),
        borderRadius: BorderRadius.circular(16),
      ),
      child: Text(
        widget.postActionLabel,
        style: TextStyle(
          color: AppColors.textTertiary(context),
          fontSize: 13,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }

  /// Flat button style for content view action bars: fills the entire segment
  /// with no border radius (parent [ClipRRect] handles corners), centered
  /// white bold text.
  Widget _buildFlatButton({
    Key? key,
    required Color color,
    required String label,
    VoidCallback? onTap,
    Color textColor = Colors.white,
  }) {
    return Tappable(
      key: key,
      semanticsLabel: label,
      onTap: onTap,
      child: Container(
        constraints: const BoxConstraints(minWidth: 90),
        color: color,
        alignment: Alignment.center,
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        child: widget.isLoading
            ? const SizedBox(
                width: 16,
                height: 16,
                child: CircularProgressIndicator(
                  strokeWidth: 2,
                  valueColor: AlwaysStoppedAnimation<Color>(Colors.white),
                ),
              )
            : Text(
                label,
                style: TextStyle(
                  color: textColor,
                  fontSize: 13,
                  fontWeight: FontWeight.w700,
                ),
              ),
      ),
    );
  }
}
