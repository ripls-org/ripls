import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// The profile identity header's tag chips (v11 synthesis): a group's
/// interests or a person's signature tags. Clamped to [clampCount]
/// chips with a "+N more" chip that expands the rest **in place**
/// (toggling back with "show less") — no sheet, no navigation.
/// `SizedBox.shrink()` when [tags] is empty.
class ProfileChips extends StatefulWidget {
  const ProfileChips({super.key, required this.tags, this.clampCount = 5});

  final List<String> tags;

  /// Chips shown while collapsed; the rest hide behind "+N more".
  final int clampCount;

  @override
  State<ProfileChips> createState() => _ProfileChipsState();
}

class _ProfileChipsState extends State<ProfileChips> {
  bool _open = false;

  @override
  Widget build(BuildContext context) {
    if (widget.tags.isEmpty) return const SizedBox.shrink();
    final overflow = widget.tags.length - widget.clampCount;
    final shown = _open || overflow <= 0
        ? widget.tags
        : widget.tags.take(widget.clampCount).toList(growable: false);
    return Wrap(
      spacing: 6,
      runSpacing: 6,
      children: [
        for (final tag in shown) _chip(context, tag),
        if (overflow > 0) _moreChip(context, overflow),
      ],
    );
  }

  Widget _chip(BuildContext context, String tag) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.10),
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: Colors.white.withValues(alpha: 0.12)),
      ),
      child: Text(
        tag,
        style: TextStyle(
          fontSize: 11,
          fontWeight: FontWeight.w500,
          color: Colors.white.withValues(alpha: 0.72),
        ),
      ),
    );
  }

  Widget _moreChip(BuildContext context, int overflow) {
    final label = _open
        ? context.l10n.profileChipsLess
        : context.l10n.profileChipsMore(overflow);
    final accent = AppColors.primary(context);
    return Tappable(
      semanticsLabel: label,
      onTap: () => setState(() => _open = !_open),
      inkBorderRadius: BorderRadius.circular(999),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
        decoration: BoxDecoration(
          color: accent.withValues(alpha: 0.10),
          borderRadius: BorderRadius.circular(999),
          border: Border.all(color: accent.withValues(alpha: 0.4)),
        ),
        child: Text(
          label,
          style: TextStyle(
            fontSize: 11,
            fontWeight: FontWeight.w600,
            color: accent,
          ),
        ),
      ),
    );
  }
}
