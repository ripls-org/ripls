import 'package:flutter/material.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// StatChip displays a translucent pill button for stats and metadata.
///
/// Supports two visual variants:
/// - Light: Translucent white background for dark hero sections
/// - Bordered: Translucent white with border for detail chips
class StatChip extends StatelessWidget {
  final String label;
  final VoidCallback? onTap;
  final bool bordered;

  const StatChip({
    super.key,
    required this.label,
    this.onTap,
    this.bordered = false,
  });

  @override
  Widget build(BuildContext context) {
    final chip = Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.15),
        border: bordered
            ? Border.all(
                color: Colors.white.withValues(alpha: 0.25),
                width: 1,
              )
            : null,
        borderRadius: BorderRadius.circular(16),
      ),
      child: Text(
        label,
        style: TextStyle(
          color: Colors.white.withValues(alpha: 0.9),
          fontSize: 13,
          fontWeight: FontWeight.w500,
        ),
      ),
    );

    if (onTap != null) {
      return Tappable(
        semanticsLabel: label,
        onTap: onTap,
        child: chip,
      );
    }

    return chip;
  }
}
