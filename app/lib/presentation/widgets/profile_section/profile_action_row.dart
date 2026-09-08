import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// One circular action in a [ProfileActionRow].
class ProfileActionRowItem {
  const ProfileActionRowItem({
    required this.icon,
    required this.label,
    this.isPrimary = false,
    this.enabled = true,
    this.onTapRect,
    this.anchorKey,
  });

  final IconData icon;

  /// Spoken label (the buttons are icon-only).
  final String label;

  /// Primary gets the accent fill; the rest are translucent rings.
  final bool isPrimary;

  final bool enabled;

  /// Receives the button's own footprint — the source rect the
  /// morph-reveal panel grows from (`openContentMorphPanel`).
  final ValueChanged<Rect>? onTapRect;

  /// Optional handle on this button's element, so a caller can open the
  /// action's destination *without* a tap and still animate from the same
  /// footprint — see [profileActionRowItemBounds]. Used when a deep link
  /// arrives already pointing at a destination (#2876): the panel grows from
  /// the button the user would have pressed, rather than from nowhere.
  final GlobalKey? anchorKey;
}

/// The on-screen footprint of the action carrying [anchorKey], or null when it
/// isn't laid out yet. Callers auto-opening a destination should wait a frame
/// and fall back to a plain push rather than passing `Rect.zero` — a panel that
/// grows from the origin reads as a glitch, not an animation.
Rect? profileActionRowItemBounds(GlobalKey anchorKey) {
  final ctx = anchorKey.currentContext;
  if (ctx == null) return null;
  final box = ctx.findRenderObject();
  if (box is! RenderBox || !box.hasSize) return null;
  return box.localToGlobal(Offset.zero) & box.size;
}

/// The identity header's inline action row (v11 synthesis): a few
/// 46px circular icon buttons — Message primary, Plans and Library
/// beside it — replacing the old bottom-anchored card. Every action
/// expands its destination in place via the morph-reveal panel.
class ProfileActionRow extends StatelessWidget {
  const ProfileActionRow({super.key, required this.actions});

  final List<ProfileActionRowItem> actions;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        for (var i = 0; i < actions.length; i++) ...[
          if (i > 0) const SizedBox(width: 10),
          _ActionButton(item: actions[i]),
        ],
      ],
    );
  }
}

class _ActionButton extends StatelessWidget {
  const _ActionButton({required this.item});

  final ProfileActionRowItem item;

  @override
  Widget build(BuildContext context) {
    final enabled = item.enabled && item.onTapRect != null;
    final circle = Container(
      width: 46,
      height: 46,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: item.isPrimary && enabled
            ? AppColors.primary(context)
            : Colors.white.withValues(alpha: enabled ? 0.09 : 0.05),
        border: item.isPrimary && enabled
            ? null
            : Border.all(
                color: Colors.white.withValues(alpha: enabled ? 0.2 : 0.1),
              ),
      ),
      child: Icon(
        item.icon,
        size: 19,
        color: item.isPrimary && enabled
            ? AppColors.background(context)
            : Colors.white.withValues(alpha: enabled ? 0.92 : 0.4),
      ),
    );
    if (!enabled) return KeyedSubtree(key: item.anchorKey, child: circle);
    return KeyedSubtree(
      key: item.anchorKey,
      child: Builder(
        builder: (ctx) => Tappable(
          semanticsLabel: item.label,
          onTap: () => item.onTapRect!(_boundsOf(ctx)),
          inkBorderRadius: BorderRadius.circular(23),
          child: circle,
        ),
      ),
    );
  }
}

/// The tapped button's own on-screen footprint — the source rect a
/// morph-reveal panel grows from (`openContentMorphPanel`). Mirrors the
/// content views' per-file `_rectOf` helpers; [context] must belong to
/// an already-laid-out element (safe inside a tap handler).
Rect _boundsOf(BuildContext context) {
  final box = context.findRenderObject();
  return box is RenderBox && box.hasSize
      ? box.localToGlobal(Offset.zero) & box.size
      : Rect.zero;
}
