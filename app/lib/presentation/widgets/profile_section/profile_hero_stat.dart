import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// The profile's one editorial stat moment (v11 synthesis): an accent
/// kicker beside a big serif number, a plain-language subtext line
/// under it, and the serif equivalence sentence — the ledger rows
/// below it stay quiet by contrast. Tappable (with a chevron) only
/// when the host wires a drill-down.
class ProfileHeroStat extends StatelessWidget {
  const ProfileHeroStat({
    super.key,
    required this.kicker,
    required this.value,
    this.unit,
    required this.subtext,
    this.equivalence,
    this.onTapRect,
    this.semanticsLabel,
  });

  /// Uppercase accent kicker, e.g. "TIME TOGETHER".
  final String kicker;

  /// The serif display number, pre-formatted.
  final String value;

  /// Small unit tail beside the number ("hrs").
  final String? unit;

  /// Plain-language line under the number.
  final String subtext;

  /// Serif equivalence sentence ("Worth about five extra years…").
  final String? equivalence;

  /// Receives the block's own footprint — the source rect the
  /// morph-reveal drill-down grows from (`openContentMorphPanel`).
  final ValueChanged<Rect>? onTapRect;

  final String? semanticsLabel;

  @override
  Widget build(BuildContext context) {
    final interactive = onTapRect != null;
    final block = Container(
      padding: const EdgeInsets.fromLTRB(22, 14, 22, 16),
      decoration: BoxDecoration(
        border: Border(
          bottom: BorderSide(color: Colors.white.withValues(alpha: 0.16)),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.center,
            children: [
              Expanded(
                child: Text(
                  kicker.toUpperCase(),
                  style: TextStyle(
                    fontSize: 10.5,
                    fontWeight: FontWeight.w700,
                    letterSpacing: 1.4,
                    color: AppColors.primary(context),
                  ),
                ),
              ),
              Text.rich(
                TextSpan(
                  text: value,
                  style: const TextStyle(
                    fontFamily: AppTheme.headingFont,
                    fontWeight: FontWeight.w600,
                    fontSize: 34,
                    letterSpacing: -0.3,
                    color: Colors.white,
                  ),
                  children: [
                    if (unit != null)
                      TextSpan(
                        text: ' ${unit!}',
                        style: TextStyle(
                          fontFamily: AppTheme.bodyFont,
                          fontWeight: FontWeight.w500,
                          fontSize: 15,
                          color: Colors.white.withValues(alpha: 0.72),
                        ),
                      ),
                  ],
                ),
              ),
              if (interactive) ...[
                const SizedBox(width: 6),
                Icon(
                  Icons.chevron_right,
                  size: 17,
                  color: Colors.white.withValues(alpha: 0.52),
                ),
              ],
            ],
          ),
          const SizedBox(height: 2),
          Text(
            subtext,
            style: TextStyle(
              fontSize: 11.5,
              color: Colors.white.withValues(alpha: 0.52),
            ),
          ),
          if (equivalence != null && equivalence!.isNotEmpty) ...[
            const SizedBox(height: 8),
            Text(
              equivalence!,
              style: const TextStyle(
                fontFamily: AppTheme.headingFont,
                fontSize: 16.5,
                height: 1.3,
                color: Colors.white,
              ),
            ),
          ],
        ],
      ),
    );
    if (!interactive) return block;
    return Builder(
      builder: (ctx) => Tappable(
        semanticsLabel: semanticsLabel ?? '$value ${unit ?? ''} $kicker'.trim(),
        onTap: () => onTapRect!(_boundsOf(ctx)),
        child: block,
      ),
    );
  }
}

/// The tapped block's own on-screen footprint — the source rect a
/// morph-reveal panel grows from (`openContentMorphPanel`). Mirrors the
/// content views' per-file `_rectOf` helpers; [context] must belong to
/// an already-laid-out element (safe inside a tap handler).
Rect _boundsOf(BuildContext context) {
  final box = context.findRenderObject();
  return box is RenderBox && box.hasSize
      ? box.localToGlobal(Offset.zero) & box.size
      : Rect.zero;
}
