import 'dart:math' as math;
import 'dart:ui';

import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart' show Attribution;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/presentation/widgets/content/photo_attribution_line.dart';
import 'package:ripls/presentation/widgets/nudge/nudge_presentation.dart';
import 'package:ripls/services/feed_service.dart' show NudgePayload, NudgeStat;

/// Fraction of a card's content box the headline may claim before the
/// description gets its turn.
///
/// The headline is not [Flexible]: it takes exactly the height its own line
/// count needs, and the description — the sole [Flexible] — then absorbs
/// everything left, including whatever a short headline didn't use. Without a
/// ceiling, a three-line headline eats an embedded card whole (#2801).
const double _headlineHeightShare = 0.45;

/// Inset from the card's edges for overlay chrome (the photo credit).
const double _chromeInset = 12;
const double _chromeSideInset = 16;

/// Extra top padding the content column takes when the photo credit is on the
/// card, so a centered headline can't run under it.
///
/// The credit is [_chromeInset] from the top and ~25 px tall (a 10.5 px line in
/// a pill with 5 px of vertical padding, plus the widget's own 4 px bottom
/// inset); this clears it with a few pixels to spare. Embedded cards only —
/// a full-screen card has room above its centered content already.
const double _attributionReserve = 24;

/// _CardMetrics is the geometry one nudge variant uses in one presentation.
///
/// Not every variant reads every field — variant 2 keeps its CTA in the outer
/// column and so uses [ctaRowPadding] rather than [ctaGap].
class _CardMetrics {
  /// Inset around the card's content column.
  final EdgeInsets padding;

  /// Gap between the headline and the description.
  final double headlineGap;

  /// Gap between the description and the CTA.
  final double ctaGap;

  /// Inset around variant 2's separate CTA row.
  final EdgeInsets ctaRowPadding;

  /// Whether the CTA renders at its compact size.
  final bool compactCta;

  /// Headline type. Embedded cards step down a size — 32 px spends 40 % of a
  /// 280 px card on three headline lines.
  final TextStyle headlineStyle;

  const _CardMetrics({
    required this.padding,
    required this.headlineGap,
    required this.ctaGap,
    required this.compactCta,
    required this.headlineStyle,
    this.ctaRowPadding = EdgeInsets.zero,
  });
}

/// _NudgeCTAButton renders the primary coral CTA button.
class _NudgeCTAButton extends StatelessWidget {
  final String label;
  final VoidCallback onTap;
  final bool small;

  const _NudgeCTAButton({required this.label, required this.onTap, this.small = false});

  @override
  Widget build(BuildContext context) {
    // The label must pair with the FILL, not with the card behind it. The fill
    // is AppColors.primary(context), which is the light sage #9DBFA8 in dark
    // theme — white on it measures 2.01:1. Light theme was fine at 7.61:1,
    // which is why this only looked broken in one theme.
    final textStyle = Theme.of(
      context,
    ).textTheme.labelLarge!.copyWith(color: AppColors.onPrimary(context));
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      child: Container(
        padding: EdgeInsets.symmetric(horizontal: small ? 20 : 24, vertical: small ? 12 : 14),
        decoration: BoxDecoration(
          color: AppColors.primary(context),
          borderRadius: BorderRadius.circular(30),
        ),
        child: Text(label, style: textStyle),
      ),
    );
  }
}

/// _NudgeGhostButton renders a secondary ghost CTA button.
class _NudgeGhostButton extends StatelessWidget {
  final String label;
  final VoidCallback onTap;

  const _NudgeGhostButton({required this.label, required this.onTap});

  @override
  Widget build(BuildContext context) {
    final textStyle = Theme.of(
      context,
    ).textTheme.labelLarge!.copyWith(color: GlassTokens.textPrimary);
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 12),
        decoration: BoxDecoration(
          color: GlassTokens.fillSubtle,
          borderRadius: BorderRadius.circular(30),
          border: Border.all(color: GlassTokens.borderSoft),
        ),
        child: Text(label, style: textStyle),
      ),
    );
  }
}

/// _DarkGlassPill renders a dark frosted-glass pill badge.
class _DarkGlassPill extends StatelessWidget {
  final String label;
  final IconData? icon;

  const _DarkGlassPill({required this.label, this.icon});

  @override
  Widget build(BuildContext context) {
    final textStyle = AppTheme.heroMetricLabelStyle.copyWith(
      color: GlassTokens.textSecondary,
      letterSpacing: 0.2,
    );
    return ClipRRect(
      borderRadius: BorderRadius.circular(20),
      child: BackdropFilter(
        filter: ImageFilter.blur(sigmaX: 20, sigmaY: 20),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 6),
          decoration: BoxDecoration(
            color: OverlayTokens.fieldFill,
            borderRadius: BorderRadius.circular(20),
            border: Border.all(color: GlassTokens.hairline),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (icon != null) ...[
                Icon(icon, color: GlassTokens.textSecondary, size: 11),
                const SizedBox(width: 5),
              ],
              Text(label, style: textStyle),
            ],
          ),
        ),
      ),
    );
  }
}

/// _StatTile renders a single value+label stat for variant 2.
class _StatTile extends StatelessWidget {
  final NudgeStat stat;

  const _StatTile({required this.stat});

  @override
  Widget build(BuildContext context) {
    final labelStyle = AppTheme.heroMetricLabelStyle.copyWith(color: GlassTokens.textMuted);
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(stat.value, style: AppTheme.nudgeStatValueStyle),
        const SizedBox(height: 2),
        Text(stat.label.toUpperCase(), style: labelStyle),
      ],
    );
  }
}

/// The number of whole lines of [style] that fit in [height], capped at [cap].
///
/// The line height is text-scaled, so the budget stays right when the reader
/// has bumped their system font size.
int _fittedLines(BuildContext context, TextStyle style, double height, int cap) {
  if (!height.isFinite) return cap;
  final lineHeight =
      MediaQuery.textScalerOf(context).scale(style.fontSize ?? 14) * (style.height ?? 1.2);
  if (lineHeight <= 0) return cap;
  return math.min(cap, height ~/ lineHeight);
}

/// _FittedText renders [text] with the most lines that fit the height it is
/// allowed, and renders nothing when not even one line does.
///
/// `TextOverflow.ellipsis` only ellipsizes at the `maxLines` boundary. When the
/// binding constraint is height rather than line count, `RenderParagraph` clips
/// the raster instead — which is how a nudge in a 280 px host ended up sliced
/// through the middle of a glyph (#2801). Fitting the line count to the height
/// moves the truncation back onto a line boundary.
///
/// The allowance is [budget] when given, and otherwise the incoming
/// `maxHeight`. The description passes nothing: as the sole [Flexible] its
/// `maxHeight` already *is* what its fixed siblings left over, so it needs no
/// arithmetic about the CTA or the gaps. The headline passes a share of the
/// content box, because it is not flexible and would otherwise take every line
/// it wanted.
class _FittedText extends StatelessWidget {
  final String text;
  final TextStyle style;
  final int maxLines;
  final double? budget;

  const _FittedText({required this.text, required this.style, required this.maxLines, this.budget});

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final lines = _fittedLines(context, style, budget ?? constraints.maxHeight, maxLines);
        if (lines < 1) return const SizedBox.shrink();
        return Text(
          text,
          textAlign: TextAlign.center,
          maxLines: lines,
          overflow: TextOverflow.ellipsis,
          style: style,
        );
      },
    );
  }
}

/// NudgeCardVariants builds one of the 3 nudge card layouts.
///
/// All variant builders take the same arguments so the dispatcher in
/// NudgeContentView can call them uniformly. Background imagery is provided
/// as a pre-resolved [mediaUrl]/[mediaId] pair from the ViewModel (widgets never
/// access repositories directly). All text styles derive from [AppTheme] constants.
///
/// [presentation] selects the geometry: full-screen cards own the screen edges,
/// embedded ones sit in a host-bounded box. See [NudgePresentation].
class NudgeCardVariants {
  NudgeCardVariants._();

  /// Headline line ceiling, before the height budget narrows it further.
  static const int _headlineMaxLines = 3;

  /// Description line ceiling, before the height budget narrows it further.
  static const int _descriptionMaxLines = 4;

  static const _CardMetrics _v1Full = _CardMetrics(
    padding: EdgeInsets.symmetric(horizontal: 28, vertical: 32),
    headlineGap: 14,
    ctaGap: 28,
    compactCta: false,
    headlineStyle: AppTheme.nudgeHeadlineLargeStyle,
  );

  static const _CardMetrics _v1Embedded = _CardMetrics(
    padding: EdgeInsets.symmetric(horizontal: 20, vertical: 18),
    headlineGap: 10,
    ctaGap: 16,
    compactCta: true,
    headlineStyle: AppTheme.nudgeHeadlineMediumStyle,
  );

  static const _CardMetrics _v2Full = _CardMetrics(
    padding: EdgeInsets.symmetric(horizontal: 24, vertical: 24),
    headlineGap: 10,
    ctaGap: 0,
    ctaRowPadding: EdgeInsets.fromLTRB(22, 14, 22, 22),
    compactCta: true,
    headlineStyle: AppTheme.nudgeHeadlineMediumStyle,
  );

  static const _CardMetrics _v2Embedded = _CardMetrics(
    padding: EdgeInsets.symmetric(horizontal: 18, vertical: 14),
    headlineGap: 8,
    ctaGap: 0,
    ctaRowPadding: EdgeInsets.fromLTRB(16, 10, 16, 14),
    compactCta: true,
    headlineStyle: AppTheme.nudgeHeadlineMediumStyle,
  );

  static const _CardMetrics _v3Full = _CardMetrics(
    padding: EdgeInsets.symmetric(horizontal: 32, vertical: 48),
    headlineGap: 14,
    ctaGap: 28,
    compactCta: true,
    headlineStyle: AppTheme.nudgeHeadlineMediumStyle,
  );

  static const _CardMetrics _v3Embedded = _CardMetrics(
    padding: EdgeInsets.symmetric(horizontal: 20, vertical: 18),
    headlineGap: 10,
    ctaGap: 16,
    compactCta: true,
    headlineStyle: AppTheme.nudgeHeadlineMediumStyle,
  );

  /// Variant 1: Magazine Spread — aspirational editorial layout.
  ///
  /// Centered composition with dark radial overlay, serif headline, italic
  /// description, optional location pill, and a single coral CTA.
  static Widget buildVariant1({
    required BuildContext context,
    required NudgePayload nudge,
    required String? mediaUrl,
    required String? mediaId,
    required VoidCallback onCtaTap,
    Attribution? attribution,
    NudgePresentation presentation = NudgePresentation.fullScreen,
  }) {
    final locationHint = nudge.hasLocationHint() && nudge.locationHint.isNotEmpty
        ? nudge.locationHint
        : null;
    final m = presentation == NudgePresentation.embedded ? _v1Embedded : _v1Full;
    final bodyStyle = AppTheme.nudgeBodyStyle.copyWith(
      color: GlassTokens.textSecondary,
      fontStyle: FontStyle.italic,
    );

    return Stack(
      fit: StackFit.expand,
      children: [
        _buildBackground(mediaUrl, mediaId),
        _buildRadialOverlay(strength: 0.7),
        _safeArea(
          presentation,
          Padding(
            padding: _contentPadding(m.padding, presentation, attribution),
            child: LayoutBuilder(
              builder: (context, box) => Column(
                mainAxisAlignment: MainAxisAlignment.center,
                crossAxisAlignment: CrossAxisAlignment.center,
                children: [
                  if (locationHint != null) ...[
                    _DarkGlassPill(label: locationHint, icon: Icons.location_on_outlined),
                    const SizedBox(height: 20),
                  ],
                  // The headline sizes to its content (up to the height budget)
                  // rather than competing with the description for an equal
                  // share of the free space — otherwise, in a height-constrained
                  // host (e.g. the calendar's fixed-height card) the 50/50 flex
                  // split starves the multi-line headline while the shorter
                  // description leaves a blank gap. The description is the sole
                  // Flexible, so it takes whatever the headline left.
                  _FittedText(
                    text: nudge.headline,
                    style: m.headlineStyle,
                    maxLines: _headlineMaxLines,
                    budget: box.maxHeight * _headlineHeightShare,
                  ),
                  SizedBox(height: m.headlineGap),
                  Flexible(
                    child: _FittedText(
                      text: nudge.description,
                      style: bodyStyle,
                      maxLines: _descriptionMaxLines,
                    ),
                  ),
                  SizedBox(height: m.ctaGap),
                  _NudgeCTAButton(label: nudge.ctaLabel, onTap: onCtaTap, small: m.compactCta),
                ],
              ),
            ),
          ),
        ),
        ..._attributionChrome(context, presentation, attribution),
      ],
    );
  }

  /// Variant 2: Ticker Tape — celebrates activity with a stats bar.
  ///
  /// Centered headline and description, a bottom bar with up to 3 stats,
  /// and primary + optional secondary CTA buttons.
  static Widget buildVariant2({
    required BuildContext context,
    required NudgePayload nudge,
    required String? mediaUrl,
    required String? mediaId,
    required VoidCallback onCtaTap,
    Attribution? attribution,
    NudgePresentation presentation = NudgePresentation.fullScreen,
  }) {
    final hasSecondaryCta =
        nudge.hasSecondaryCtaLabel() &&
        nudge.secondaryCtaLabel.isNotEmpty &&
        nudge.hasSecondaryCtaAction() &&
        nudge.secondaryCtaAction.isNotEmpty;
    final m = presentation == NudgePresentation.embedded ? _v2Embedded : _v2Full;
    final bodyStyle = AppTheme.nudgeBodyStyle.copyWith(color: GlassTokens.textSecondary);

    return Stack(
      fit: StackFit.expand,
      children: [
        _buildBackground(mediaUrl, mediaId),
        _buildRadialOverlay(strength: 0.75),
        _safeArea(
          presentation,
          Column(
            children: [
              // Main content — centered in the available space.
              Expanded(
                child: Padding(
                  padding: _contentPadding(m.padding, presentation, attribution),
                  child: LayoutBuilder(
                    builder: (context, box) => Column(
                      mainAxisAlignment: MainAxisAlignment.center,
                      crossAxisAlignment: CrossAxisAlignment.center,
                      children: [
                        // Headline sizes to content; the description is the
                        // sole Flexible that yields space — see buildVariant1.
                        _FittedText(
                          text: nudge.headline,
                          style: m.headlineStyle,
                          maxLines: _headlineMaxLines,
                          budget: box.maxHeight * _headlineHeightShare,
                        ),
                        SizedBox(height: m.headlineGap),
                        Flexible(
                          child: _FittedText(
                            text: nudge.description,
                            style: bodyStyle,
                            maxLines: _descriptionMaxLines,
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
              // Stats bar.
              if (nudge.stats.isNotEmpty)
                Container(
                  decoration: BoxDecoration(
                    color: OverlayTokens.fieldFill,
                    border: Border(top: BorderSide(color: GlassTokens.hairline)),
                  ),
                  padding: const EdgeInsets.symmetric(horizontal: 28, vertical: 16),
                  child: Row(
                    mainAxisAlignment: MainAxisAlignment.spaceAround,
                    children: nudge.stats.take(3).map((s) => _StatTile(stat: s)).toList(),
                  ),
                ),
              // CTA row.
              Padding(
                padding: m.ctaRowPadding,
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    _NudgeCTAButton(label: nudge.ctaLabel, onTap: onCtaTap, small: true),
                    if (hasSecondaryCta) ...[
                      const SizedBox(width: 10),
                      _NudgeGhostButton(label: nudge.secondaryCtaLabel, onTap: onCtaTap),
                    ],
                  ],
                ),
              ),
            ],
          ),
        ),
        ..._attributionChrome(context, presentation, attribution),
      ],
    );
  }

  /// Variant 3: Headline Card — simple centered terminator layout.
  ///
  /// Used as the feed terminator ("You're all caught up"). No badge or
  /// pill — just headline, description, and a small CTA.
  static Widget buildVariant3({
    required BuildContext context,
    required NudgePayload nudge,
    required String? mediaUrl,
    required String? mediaId,
    required VoidCallback onCtaTap,
    Attribution? attribution,
    NudgePresentation presentation = NudgePresentation.fullScreen,
  }) {
    final m = presentation == NudgePresentation.embedded ? _v3Embedded : _v3Full;
    final bodyStyle = AppTheme.nudgeBodyStyle.copyWith(color: GlassTokens.textSecondary);

    return Stack(
      fit: StackFit.expand,
      children: [
        _buildBackground(mediaUrl, mediaId),
        _buildRadialOverlay(strength: 0.65),
        _safeArea(
          presentation,
          Padding(
            padding: _contentPadding(m.padding, presentation, attribution),
            child: LayoutBuilder(
              builder: (context, box) => Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  // Headline sizes to content; the description is the sole
                  // Flexible that yields space — see buildVariant1.
                  _FittedText(
                    text: nudge.headline,
                    style: m.headlineStyle,
                    maxLines: _headlineMaxLines,
                    budget: box.maxHeight * _headlineHeightShare,
                  ),
                  SizedBox(height: m.headlineGap),
                  Flexible(
                    child: _FittedText(
                      text: nudge.description,
                      style: bodyStyle,
                      maxLines: _descriptionMaxLines,
                    ),
                  ),
                  SizedBox(height: m.ctaGap),
                  _NudgeCTAButton(label: nudge.ctaLabel, onTap: onCtaTap, small: true),
                ],
              ),
            ),
          ),
        ),
        ..._attributionChrome(context, presentation, attribution),
      ],
    );
  }

  // ---------------------------------------------------------------------------
  // Private helpers
  // ---------------------------------------------------------------------------

  /// A full-screen card owns the window's insets; an embedded one is already
  /// inside its host's safe area, where a second [SafeArea] would only eat
  /// height the copy needs.
  static Widget _safeArea(NudgePresentation presentation, Widget child) =>
      presentation == NudgePresentation.fullScreen ? SafeArea(child: child) : child;

  /// [base], plus room for the photo credit when an embedded card carries one.
  /// The credit is a `Positioned` overlay, so nothing else moves the centered
  /// content out of its way.
  static EdgeInsets _contentPadding(
    EdgeInsets base,
    NudgePresentation presentation,
    Attribution? attribution,
  ) => attribution != null && presentation == NudgePresentation.embedded
      ? base + const EdgeInsets.only(top: _attributionReserve)
      : base;

  /// The photo credit, top-right over the imagery.
  ///
  /// Stock imagery from Unsplash *requires* visible attribution, so the credit
  /// is on the card rather than behind an info button that opened a one-line
  /// sheet at the far bottom of the screen (#2801). Last in the Stack, so the
  /// CTA still precedes it in reading order.
  static List<Widget> _attributionChrome(
    BuildContext context,
    NudgePresentation presentation,
    Attribution? attribution,
  ) {
    if (attribution == null) return const [];
    final topInset = presentation == NudgePresentation.fullScreen
        ? MediaQuery.of(context).padding.top
        : 0.0;
    return [
      Positioned(
        top: topInset + _chromeInset,
        left: _chromeSideInset,
        right: _chromeSideInset,
        child: PhotoAttributionLine(attribution: attribution, boxed: true),
      ),
    ];
  }

  static Widget _buildBackground(String? mediaUrl, String? mediaId) {
    if (mediaUrl != null && mediaUrl.isNotEmpty) {
      return CachedMediaImage(
        // Decorative; the surrounding card carries the semantic label.
        semanticsLabel: null,
        imageUrl: mediaUrl,
        cacheKey: mediaId,
        fit: BoxFit.cover,
      );
    }
    return Container(color: Colors.black87);
  }

  /// Radial dark overlay — darker at edges, lighter at center.
  static Widget _buildRadialOverlay({double strength = 0.7}) {
    return Container(
      decoration: BoxDecoration(
        gradient: RadialGradient(
          center: Alignment.center,
          radius: 1.1,
          colors: [
            Colors.black.withValues(alpha: strength * 0.65),
            Colors.black.withValues(alpha: strength),
          ],
        ),
      ),
    );
  }
}
