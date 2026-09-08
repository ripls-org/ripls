import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// GlassInsetCard is the card shape used inside a [GlassSheet] when content
/// needs a "card layer" above the sheet body — e.g. transfer flow's
/// pickup-details cards. It uses a **flat alpha fill** with no inner
/// [BackdropFilter] so it composes correctly inside a parent glass sheet
/// without producing the recursive-blur ring artifact iOS+Metal exhibits
/// when [BackdropFilter]s nest.
///
/// Presentational by default; pass [onTap] (with a [semanticsLabel]) to
/// make the card tappable. The widget composes [Tappable] internally
/// rather than exposing a raw `GestureDetector`, satisfying the
/// `avoid_raw_gesture_detector` lint.
///
/// See `docs/issues/1802-modals-glass-sweep.md`.
class GlassInsetCard extends StatelessWidget {
  /// Card content.
  final Widget child;

  /// Optional padding applied inside the card.
  final EdgeInsetsGeometry? padding;

  /// Corner radius. Defaults to 16.
  final BorderRadius borderRadius;

  /// Optional override for the fill. Defaults to
  /// [AppColors.modalInsetCardBg].
  final Color? fill;

  /// Optional override for the border color. Defaults to
  /// [AppColors.modalInsetCardBorder]. Pass `null` to omit the border.
  final Color? border;

  /// Optional tap callback. When non-null the card is wrapped in a
  /// [Tappable] with [semanticsLabel].
  final VoidCallback? onTap;

  /// Required when [onTap] is non-null — passed to [Tappable] for screen
  /// readers. Ignored when [onTap] is null.
  final String? semanticsLabel;

  const GlassInsetCard({
    super.key,
    required this.child,
    this.padding,
    this.borderRadius = const BorderRadius.all(Radius.circular(16)),
    this.fill,
    this.border,
    this.onTap,
    this.semanticsLabel,
  }) : assert(
          onTap == null || semanticsLabel != null,
          'GlassInsetCard with onTap requires a semanticsLabel for accessibility.',
        );

  @override
  Widget build(BuildContext context) {
    final body = Container(
      decoration: BoxDecoration(
        color: fill ?? AppColors.modalInsetCardBg,
        borderRadius: borderRadius,
        border: border == null && fill != null
            ? null
            : Border.all(
                color: border ?? AppColors.modalInsetCardBorder,
                width: 1,
              ),
      ),
      padding: padding,
      child: child,
    );

    if (onTap == null) {
      return body;
    }

    return Tappable(
      semanticsLabel: semanticsLabel!,
      onTap: onTap,
      child: body,
    );
  }
}
