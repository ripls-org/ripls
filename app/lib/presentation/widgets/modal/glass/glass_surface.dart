import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// GlassSurface is the foundational frosted-glass primitive used by every
/// glass-modal widget. It wraps [child] with a translucent fill, optional
/// backdrop blur, and a hairline border, all clipped to [borderRadius].
///
/// Set [useBlur] to `false` to skip the [BackdropFilter]. Backdrop filters
/// are noticeably expensive on older Android hardware; the fallback paints a
/// solid translucent rectangle that reads similarly enough that the
/// migration does not regress without blur.
///
/// See docs/issues/1797-glass-modal-revamp.md for the design rationale.
class GlassSurface extends StatelessWidget {
  /// Optional override for the surface fill. Defaults to [AppColors.modalSurface].
  final Color? fill;

  /// Optional override for the border color. Pass `null` to omit the border.
  /// Defaults to [AppColors.modalBorder].
  final Color? border;

  /// Border thickness when [border] is non-null.
  final double borderWidth;

  /// Sigma value for the [BackdropFilter]. Ignored when [useBlur] is false.
  final double blurSigma;

  /// Whether to render the [BackdropFilter]. Disable on low-end Android or
  /// when an outer surface already applied a blur.
  final bool useBlur;

  /// Corner radius applied to the clip and border.
  final BorderRadius borderRadius;

  /// Optional padding applied inside the surface.
  final EdgeInsetsGeometry? padding;

  /// Optional opaque-ish darkening layer painted between the backdrop blur and
  /// the [fill]+[child]. Use over bright backgrounds (e.g.
  /// [AppColors.modalContentScrim]) so the blurred-through content does not
  /// wash out on-glass text. When null (default) no scrim is painted.
  final Color? scrim;

  /// Child rendered on top of the glass.
  final Widget child;

  const GlassSurface({
    super.key,
    required this.child,
    this.fill,
    this.border = AppColors.modalBorder,
    this.borderWidth = 1,
    this.blurSigma = AppColors.modalSurfaceBlurSigma,
    this.useBlur = true,
    this.borderRadius = const BorderRadius.all(Radius.circular(16)),
    this.padding,
    this.scrim,
  });

  @override
  Widget build(BuildContext context) {
    final fillColor = fill ?? AppColors.modalSurface;

    Widget surface = Container(
      decoration: BoxDecoration(
        color: fillColor,
        borderRadius: borderRadius,
        border: border == null
            ? null
            : Border.all(color: border!, width: borderWidth),
      ),
      padding: padding,
      child: child,
    );

    if (scrim != null) {
      // The scrim sits behind the translucent [fill] so it darkens the
      // blurred content, then the fill+child paint on top.
      surface = DecoratedBox(
        decoration: BoxDecoration(color: scrim, borderRadius: borderRadius),
        child: surface,
      );
    }

    if (!useBlur) {
      return ClipRRect(borderRadius: borderRadius, child: surface);
    }

    return ClipRRect(
      borderRadius: borderRadius,
      child: BackdropFilter(
        filter: ui.ImageFilter.blur(sigmaX: blurSigma, sigmaY: blurSigma),
        child: surface,
      ),
    );
  }
}
