import 'dart:async';

import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/screens/search/universal_search_screen.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/profile_menu_avatar.dart';

/// DestinationHeader is the shared header of the four dock destinations
/// (#2634 v2): a left-aligned serif title with an optional subtitle, and
/// the avatar chip with the universal-search chip to its right, with
/// identical *internal* geometry on every tab (the dock carries no search —
/// the NavVariations-v3 liquid-glass bar is places + Create only).
/// Destination-specific controls (view toggles, month arrows) slot in
/// between via [trailing].
///
/// The header's outer *width* is the caller's business and is not uniform:
/// each destination wraps this in the `ContentColumn` measure that suits its
/// content, so Library (a gallery of tiles) is wider than Home and People
/// (text streams) on a desktop-wide window. See `docs/client/responsive.md`.
///
/// On scrolling destinations, place this as the scroll view's first
/// child so it scrolls off with the content — it is not an overlay.
/// Non-scrolling destinations (the map, the calendar) may pin it.
class DestinationHeader extends StatelessWidget {
  /// Serif title; may contain a newline (the Home greeting). Titles are
  /// dynamic (#2634 v2 Rev 14): the dock names the tab, so the title
  /// answers — Home says when-of-day, Plans says when, Library says
  /// where, People says who.
  final String title;

  /// Makes the title tappable (e.g. Library's location anchor opens the
  /// location sheet). Requires [titleSemanticsLabel].
  final VoidCallback? onTitleTap;
  final String? titleSemanticsLabel;

  final String? subtitle;

  /// Makes the subtitle tappable (e.g. the Plans month opens a date
  /// picker). Requires [subtitleSemanticsLabel].
  final VoidCallback? onSubtitleTap;
  final String? subtitleSemanticsLabel;

  /// Controls rendered between the title block and the avatar chip.
  final List<Widget> trailing;

  /// Hidden when a pushed variant carries a close affordance instead.
  final bool showAvatar;

  /// Overrides for headers over photo backdrops (e.g. the calendar).
  final Color? foreground;
  final Color? subtitleColor;

  const DestinationHeader({
    super.key,
    required this.title,
    this.onTitleTap,
    this.titleSemanticsLabel,
    this.subtitle,
    this.onSubtitleTap,
    this.subtitleSemanticsLabel,
    this.trailing = const [],
    this.showAvatar = true,
    this.foreground,
    this.subtitleColor,
  });

  @override
  Widget build(BuildContext context) {
    final titleColor = foreground ?? AppColors.textPrimary(context);
    final metaColor =
        subtitleColor ?? foreground ?? AppColors.textSecondary(context);

    Widget? subtitleText;
    if (subtitle != null && subtitle!.isNotEmpty) {
      subtitleText = Text(
        subtitle!,
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
        style: TextStyle(fontSize: 12, color: metaColor),
      );
      if (onSubtitleTap != null) {
        subtitleText = Tappable(
          semanticsLabel: subtitleSemanticsLabel ?? subtitle!,
          onTap: onSubtitleTap,
          inkBorderRadius: BorderRadius.circular(8),
          child: subtitleText,
        );
      }
    }

    Widget titleText = Text(
      title,
      style: TextStyle(
        fontFamily: AppTheme.headingFont,
        fontSize: 26,
        fontWeight: FontWeight.w600,
        height: 1.1,
        letterSpacing: -0.4,
        color: titleColor,
      ),
    );
    if (onTitleTap != null) {
      titleText = Tappable(
        semanticsLabel: titleSemanticsLabel ?? title,
        onTap: onTitleTap,
        inkBorderRadius: BorderRadius.circular(10),
        child: titleText,
      );
    }

    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 12, 20, 14),
      child: Row(
        // Top-aligned (not bottom, and no nested centering row): every
        // child's top sits at the same y regardless of its siblings'
        // heights. Bottom-alignment (or centering trailing controls
        // together with the avatar in their own row) both make the
        // avatar's position depend on a sibling's height — Home's
        // two-line greeting, a subtitle, or a tall trailing control all
        // shifted it. Direct top-aligned siblings have no such coupling.
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                titleText,
                if (subtitleText != null) ...[
                  const SizedBox(height: 2),
                  subtitleText,
                ],
              ],
            ),
          ),
          // A small constant nudge (matches the mock's trailing-group
          // margin-top) — a fixed offset, not a sibling-relative one, so
          // it doesn't reintroduce height-dependent shifting.
          for (final control in trailing) ...[
            Padding(
              padding: const EdgeInsets.only(top: 2),
              child: control,
            ),
            const SizedBox(width: 10),
          ],
          if (showAvatar) ...[
            const Padding(
              padding: EdgeInsets.only(top: 2),
              child: ProfileMenuAvatar(),
            ),
            const SizedBox(width: 10),
            // Universal search, to the right of the avatar — moved up
            // from the dock (NavVariations-v3: no search in the bar).
            Padding(
              padding: const EdgeInsets.only(top: 2),
              child: _SearchChip(foreground: foreground),
            ),
          ],
        ],
      ),
    );
  }
}

/// The header's universal-search entry: a circular chip matching the
/// avatar's scale that opens [UniversalSearchScreen].
class _SearchChip extends StatelessWidget {
  const _SearchChip({this.foreground});

  /// Override for headers over photo backdrops (matches the title).
  final Color? foreground;

  @override
  Widget build(BuildContext context) {
    final color = foreground ?? AppColors.textPrimary(context);
    return Tappable(
      semanticsLabel: context.l10n.a11yNavDockSearch,
      onTap: () => unawaited(UniversalSearchScreen.show(context)),
      inkBorderRadius: BorderRadius.circular(14),
      child: Container(
        width: 28,
        height: 28,
        decoration: BoxDecoration(
          shape: BoxShape.circle,
          border: Border.all(color: color.withValues(alpha: 0.35)),
        ),
        child: Icon(Icons.search, size: 16, color: color),
      ),
    );
  }
}
