import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_surface.dart';

/// Shared palette + glass-card primitive for the Workshop overview (#2447).
///
/// The Workshop overview adopts the content-view visual language: a full-bleed
/// community photo with a dark scrim and glass-morphic cards floating over it.
/// Because every surface here sits on the always-dark photo, on-glass content
/// is white-on-translucent regardless of the app theme — the same principle as
/// the chat/modal glass material.
///
/// This file is the single home for those on-photo values so section widgets
/// never sprinkle ad-hoc white-alpha constants of their own. The accent reuses
/// the established on-media sage ([AppColors.transferCoral]); only the
/// white-derived text/fill opacities (which are theme-independent on a dark
/// photo) live here.
class WorkshopOverviewPalette {
  const WorkshopOverviewPalette._();

  /// On-media accent (sage). Used for tiny eyebrow labels and category dots.
  static const Color accent = AppColors.transferCoral;

  /// Softer on-media accent for secondary accented text.
  static const Color accentSoft = AppColors.transferCoralSoft;

  /// Primary on-photo text — solid white.
  static const Color onPhoto = OverlayTokens.textPrimary;

  /// Dimmed on-photo text (white @ 74%) — subtitles, secondary copy.
  static const Color onPhotoDim = Color(0xBDFFFFFF);

  /// Faint on-photo text (white @ 52%) — captions, chevrons, units.
  static const Color onPhotoFaint = Color(0x85FFFFFF);

  /// Glass-card fill over the photo.
  static const Color cardFill = GlassTokens.fillSubtle;

  /// Glass-card hairline border.
  static const Color cardBorder = GlassTokens.borderSoft;

  /// Faint per-card darkening behind the [cardFill] (black @ 14%). Following
  /// the content-view approach (experienceContentView), the *page scrim* — not
  /// the cards — carries the text contrast, so this stays light enough that the
  /// cards recede into the photo rather than reading as dark contrast blocks.
  static const Color cardScrim = Color(0x24000000);

  /// Fill for small no-blur pills (category chips) — kept subtle to match the
  /// recessed glass cards.
  static const Color chipFill = GlassTokens.scrimTint;

  /// Corner radius shared by overview glass cards.
  static const double cardRadius = 18;

  /// Backdrop blur for overview glass cards — lighter than the modal sheet so a
  /// scrolling page of cards stays cheap on mid-range hardware.
  static const double cardBlurSigma = 10;

  /// Page scrim between the photo and the content — this is where the text
  /// contrast comes from (the content-view / experienceContentView approach):
  /// 50% at the top so the photo still reads behind the header, ramping to a
  /// near-black floor at the bottom where the content sits.
  static const List<Color> scrimColors = [
    Color(0x80070907), // rgba(7,9,7,0.50)
    Color(0xB3070907), // rgba(7,9,7,0.70)
    Color(0xE0070907), // rgba(7,9,7,0.88)
    Color(0xF7070907), // rgba(7,9,7,0.97) — near-black floor
  ];

  static const List<double> scrimStops = [0.0, 0.4, 0.75, 1.0];
}

/// Global footprint rect of [context]'s render object, for morph-reveal
/// expansion (a destination panel grows from the tapped widget's rect). Falls
/// back to [Rect.zero] when the box isn't laid out yet.
Rect workshopRectOf(BuildContext context) {
  final box = context.findRenderObject();
  return box is RenderBox && box.hasSize
      ? box.localToGlobal(Offset.zero) & box.size
      : Rect.zero;
}

/// A frosted glass card sized for the Workshop overview, floating over the
/// community photo. Thin wrapper over [GlassSurface] with the overview palette
/// applied, plus an optional [onTap] that keeps a ≥48 dp hit target.
class WorkshopGlassCard extends StatelessWidget {
  final Widget child;
  final EdgeInsetsGeometry padding;
  final EdgeInsetsGeometry margin;
  final VoidCallback? onTap;

  /// Morph-reveal tap: reports the card's own global footprint rect so a
  /// destination panel can grow from it. Mutually exclusive with [onTap].
  final ValueChanged<Rect>? onExpand;
  final String? semanticsLabel;

  const WorkshopGlassCard({
    super.key,
    required this.child,
    this.padding = const EdgeInsets.fromLTRB(16, 14, 16, 14),
    this.margin = const EdgeInsets.fromLTRB(16, 12, 16, 0),
    this.onTap,
    this.onExpand,
    this.semanticsLabel,
  });

  @override
  Widget build(BuildContext context) {
    final radius = BorderRadius.circular(WorkshopOverviewPalette.cardRadius);
    Widget surface = GlassSurface(
      fill: WorkshopOverviewPalette.cardFill,
      border: WorkshopOverviewPalette.cardBorder,
      blurSigma: WorkshopOverviewPalette.cardBlurSigma,
      // The dark scrim is what gives on-card text its contrast, letting the
      // page scrim stay light enough to show the photo.
      scrim: WorkshopOverviewPalette.cardScrim,
      borderRadius: radius,
      padding: padding is EdgeInsets ? padding as EdgeInsets : null,
      child: child,
    );

    if (onExpand != null) {
      // Capture the built surface in a separate final so the Builder closure
      // doesn't reference the reassigned `surface` variable (which would point
      // at the Builder itself → infinite recursion).
      final inner = surface;
      // Builder so findRenderObject resolves the card's own box for the rect.
      surface = Builder(
        builder: (ctx) => Tappable(
          semanticsLabel: semanticsLabel ?? '',
          onTap: () => onExpand!(workshopRectOf(ctx)),
          inkBorderRadius: radius,
          child: inner,
        ),
      );
    } else if (onTap != null) {
      surface = Tappable(
        semanticsLabel: semanticsLabel ?? '',
        onTap: onTap!,
        inkBorderRadius: radius,
        child: surface,
      );
    }

    return Padding(padding: margin, child: surface);
  }
}

/// A short uppercase eyebrow label (e.g. "HOW YOU CAN HELP") rendered in the
/// on-media accent, matching the spec's `.hk` / `.sec-h` styling.
class WorkshopSectionLabel extends StatelessWidget {
  final String text;
  const WorkshopSectionLabel(this.text, {super.key});

  @override
  Widget build(BuildContext context) {
    return Text(
      text,
      style: const TextStyle(
        fontSize: 10,
        fontWeight: FontWeight.w700,
        letterSpacing: 1.4,
        color: WorkshopOverviewPalette.accentSoft,
      ),
    );
  }
}
