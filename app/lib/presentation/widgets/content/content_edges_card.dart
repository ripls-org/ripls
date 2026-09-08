import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_edge_data.dart';

/// ContentEdgesCard renders the contribution graph ("who's bringing what") on
/// the redesigned content sheet (see
/// docs/issues/2278-experience-content-redesign.md): an accent-tinted card with
/// a tappable header, a divided list of participant [rows], and an optional
/// [footer] slot for the viewer's "you-gap" action row.
///
/// Role-agnostic and always-on-dark. The caller supplies resolved header copy
/// and builds the rows (typically [ContentEdgeRow]s from [EdgeViewData]).
class ContentEdgesCard extends StatelessWidget {
  /// Section label, e.g. "Who's pitching in?" (caller-resolved).
  final String headerLabel;

  /// Optional widget shown before the header label, e.g. a participant avatar
  /// stack. Decorative; the surrounding card carries the semantic label.
  final Widget? headerLeading;

  /// Optional trailing summary shown at the right of the header row, e.g.
  /// "3 going".
  final String? headerTrailing;

  /// Optional tap on the whole card (e.g. expand the "Who's pitching in"
  /// roster). Receives the card's current on-screen rect (global coordinates)
  /// so the caller can morph/grow from the card's footprint. When set,
  /// [semanticsLabel] is required.
  final ValueChanged<Rect>? onTap;

  /// Screen-reader label for the card tap target (caller-resolved).
  final String? semanticsLabel;

  /// Participant rows (typically [ContentEdgeRow]).
  final List<Widget> rows;

  /// Optional trailing slot below the rows.
  final Widget? footer;

  /// The view's accent.
  final Color accentColor;

  /// Optional override for the header label color. Defaults to [accentColor];
  /// pass a neutral token to match the surrounding fact labels.
  final Color? headerColor;

  const ContentEdgesCard({
    super.key,
    required this.headerLabel,
    required this.rows,
    required this.accentColor,
    this.headerLeading,
    this.headerTrailing,
    this.headerColor,
    this.onTap,
    this.semanticsLabel,
    this.footer,
  }) : assert(
         onTap == null || semanticsLabel != null,
         'semanticsLabel is required when onTap is set',
       );

  Widget _header() {
    return Row(
      children: [
        if (headerLeading != null) ...[
          headerLeading!,
          const SizedBox(width: 8),
        ],
        Expanded(
          child: Text(
            headerLabel.toUpperCase(),
            overflow: TextOverflow.ellipsis,
            style: TextStyle(
              color: headerColor ?? accentColor,
              fontSize: 10,
              fontWeight: FontWeight.w700,
              letterSpacing: 1.2,
            ),
          ),
        ),
        if (headerTrailing != null)
          Text(
            headerTrailing!,
            style: const TextStyle(
              color: AppColors.darkTextSecondary,
              fontSize: 11,
              fontWeight: FontWeight.w600,
            ),
          ),
      ],
    );
  }

  @override
  Widget build(BuildContext context) {
    final card = Container(
      padding: const EdgeInsets.fromLTRB(14, 12, 14, 10),
      decoration: BoxDecoration(
        color: accentColor.withValues(alpha: 0.05),
        borderRadius: BorderRadius.circular(18),
        border: Border.all(color: accentColor.withValues(alpha: 0.18)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          _header(),
          const SizedBox(height: 8),
          for (var i = 0; i < rows.length; i++) ...[
            if (i > 0)
              const Divider(
                height: 1,
                thickness: 1,
                color: AppColors.darkDivider,
              ),
            rows[i],
          ],
          if (footer != null) ...[const SizedBox(height: 6), footer!],
        ],
      ),
    );

    if (onTap == null) return card;
    return Tappable(
      semanticsLabel: semanticsLabel!,
      onTap: () {
        // Capture the card's own footprint so the caller can grow from it.
        final box = context.findRenderObject();
        final rect = box is RenderBox && box.hasSize
            ? box.localToGlobal(Offset.zero) & box.size
            : Rect.zero;
        onTap!(rect);
      },
      excludeChildSemantics: false,
      child: card,
    );
  }
}

/// ContentEdgeRow renders a single participant in a [ContentEdgesCard]: avatar,
/// name, contribution summary, and a status pill derived from [EdgeViewData.status].
/// Pure and always-on-dark — the caller resolves [statusLabel] (l10n) and may
/// pass a photo [avatar]; otherwise an initials avatar is drawn.
class ContentEdgeRow extends StatelessWidget {
  final EdgeViewData edge;

  /// Resolved status pill text (caller-provided from `context.l10n`).
  final String statusLabel;

  final Color accentColor;

  /// Optional photo avatar (e.g. a `CachedMediaImage`); falls back to initials.
  final Widget? avatar;

  /// Optional trailing widget that replaces the status pill — e.g. an inline
  /// RSVP CTA on the viewer's own pinned row.
  final Widget? trailing;

  /// Whether to render the leading avatar. When false the row shows just the
  /// name (and contribution / status). Defaults to true.
  final bool showAvatar;

  /// Whether to render the trailing status pill. When false the status is
  /// conveyed by the avatar marker instead (the collapsed "faces" treatment),
  /// and the row ends after the contribution. A [trailing] widget still wins.
  /// Defaults to true.
  final bool showStatusPill;

  const ContentEdgeRow({
    super.key,
    required this.edge,
    required this.statusLabel,
    required this.accentColor,
    this.avatar,
    this.trailing,
    this.showAvatar = true,
    this.showStatusPill = true,
  });

  ({Color fg, Color bg}) _statusColors() {
    switch (edge.status) {
      case EdgeStatus.host:
        return (
          fg: AppColors.darkTextSecondary,
          bg: AppColors.darkTextPrimary.withValues(alpha: 0.06),
        );
      case EdgeStatus.going:
        return (fg: accentColor, bg: accentColor.withValues(alpha: 0.18));
      case EdgeStatus.maybe:
        return (
          fg: AppColors.statusWarningOnDark,
          bg: AppColors.statusWarningOnDark.withValues(alpha: 0.18),
        );
      case EdgeStatus.wrapped:
        return (
          fg: AppColors.statusInfoOnDark,
          bg: AppColors.statusInfoOnDark.withValues(alpha: 0.18),
        );
      case EdgeStatus.invited:
        return (
          fg: AppColors.darkTextTertiary,
          bg: AppColors.darkTextPrimary.withValues(alpha: 0.04),
        );
    }
  }

  Widget _avatar() {
    final Widget base = avatar != null
        ? ClipOval(child: SizedBox(width: 28, height: 28, child: avatar))
        : Container(
            width: 28,
            height: 28,
            alignment: Alignment.center,
            decoration: BoxDecoration(
              color: edge.isYou ? accentColor : AppColors.darkSurface,
              shape: BoxShape.circle,
            ),
            child: Text(
              edge.initials,
              style: TextStyle(
                color: edge.isYou
                    ? AppColors.darkBackground
                    : AppColors.darkTextPrimary,
                fontSize: 10,
                fontWeight: FontWeight.w700,
              ),
            ),
          );

    // Status as the exception, not the rule: "going"/"host" faces are bare, a
    // "maybe" face gets an amber badge, and a non-responder face is dimmed. The
    // badge carries the (caller-resolved) status as its semantic label so the
    // marker is not color-only.
    final bool isFaded = edge.status == EdgeStatus.invited;
    final Widget face = isFaded ? Opacity(opacity: 0.5, child: base) : base;
    if (edge.status != EdgeStatus.maybe) return face;
    return Stack(
      clipBehavior: Clip.none,
      children: [
        face,
        Positioned(
          right: -2,
          bottom: -2,
          child: Semantics(
            label: statusLabel,
            child: Container(
              width: 14,
              height: 14,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: AppColors.statusWarningOnDark,
                shape: BoxShape.circle,
                border: Border.all(color: AppColors.darkBackground, width: 2),
              ),
              child: const Text(
                '?',
                style: TextStyle(
                  color: AppColors.darkBackground,
                  fontSize: 8,
                  fontWeight: FontWeight.w800,
                  height: 1,
                ),
              ),
            ),
          ),
        ),
      ],
    );
  }

  @override
  Widget build(BuildContext context) {
    final colors = _statusColors();
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 9),
      child: Row(
        children: [
          if (showAvatar) ...[_avatar(), const SizedBox(width: 10)],
          Flexible(
            child: Text(
              edge.name,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                color: AppColors.onContentImage,
                fontSize: 13,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
          if (edge.contribution.isNotEmpty) ...[
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                edge.contribution,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  color: AppColors.onContentImage,
                  fontSize: 13,
                ),
              ),
            ),
          ] else
            const Spacer(),
          const SizedBox(width: 8),
          if (trailing != null)
            trailing!
          else if (showStatusPill)
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
              decoration: BoxDecoration(
                color: colors.bg,
                borderRadius: BorderRadius.circular(999),
              ),
              child: Text(
                statusLabel.toUpperCase(),
                style: TextStyle(
                  color: colors.fg,
                  fontSize: 9,
                  fontWeight: FontWeight.w700,
                  letterSpacing: 0.8,
                ),
              ),
            ),
        ],
      ),
    );
  }
}
