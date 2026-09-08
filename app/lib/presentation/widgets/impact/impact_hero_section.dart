import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/paper_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

import 'stat_chip.dart';

/// ImpactMetricsHeroSection displays a dark gradient hero with name, metadata,
/// description, and stat chips.
///
/// Supports three variants: item, user, and community impact views.
/// Can display a background image with gradient overlay.
/// Description is tappable to expand/collapse.
class ImpactMetricsHeroSection extends StatefulWidget {
  final String? title;
  final String? subtitle; // For item view: owner name in coral
  final String?
  metadata; // Metadata line (members/communities · location · since)
  final String? description;
  final List<StatChipData> chips;
  final bool bordered; // Use bordered variant for detail chips
  final Widget? trailing; // Optional trailing widget (e.g., settings icon)
  final VoidCallback? onBack;
  final String? backgroundImageUrl; // Background image for item view

  /// Optional cache key paired with [backgroundImageUrl]. When set, the
  /// background renders through `CachedNetworkImageProvider` so the
  /// image bytes share the disk cache with `CachedMediaImage` /
  /// `CachedNetworkImage` elsewhere in the app — important so the
  /// hero doesn't re-download bytes that a list-row thumbnail already
  /// fetched (and vice versa). Pass `mediaUrl.cacheKey` from
  /// `heroMediaUrlProvider` so thumbnail vs full keys stay distinct.
  ///
  /// When null, falls back to raw `NetworkImage` (no shared cache —
  /// used by callers that don't have a media-id to key on).
  final String? backgroundImageCacheKey;
  final bool extendToTop; // Extend background to top of screen (behind AppBar)
  final double? customTopPadding; // Custom top padding when extendToTop is true
  final Widget? metricSelector; // Optional metric selector (for community metrics)
  final Widget? bottomContent; // Optional bottom content (e.g., hero stats row for item metrics)
  final bool showBottomDivider; // Show a divider above bottomContent (default true)

  const ImpactMetricsHeroSection({
    super.key,
    this.title,
    this.subtitle,
    this.metadata,
    this.description,
    this.chips = const [],
    this.bordered = false,
    this.trailing,
    this.onBack,
    this.backgroundImageUrl,
    this.backgroundImageCacheKey,
    this.extendToTop = false,
    this.customTopPadding,
    this.metricSelector,
    this.bottomContent,
    this.showBottomDivider = true,
  });

  @override
  State<ImpactMetricsHeroSection> createState() => _ImpactMetricsHeroSectionState();
}

class _ImpactMetricsHeroSectionState extends State<ImpactMetricsHeroSection> {
  bool _isDescriptionExpanded = false;

  @override
  Widget build(BuildContext context) {
    // Calculate top padding
    final double topPadding;
    if (widget.customTopPadding != null) {
      // Use custom padding if provided
      topPadding = widget.customTopPadding!;
    } else if (widget.extendToTop) {
      // When extending to top, add safe area padding
      topPadding = MediaQuery.of(context).padding.top;
    } else {
      // Default padding
      topPadding = 20.0;
    }

    return Container(
      width: double.infinity,
      decoration: BoxDecoration(
        image: widget.backgroundImageUrl != null
            ? DecorationImage(
                // Prefer CachedNetworkImageProvider so the bytes share
                // the flutter_cache_manager disk cache with the rest of
                // the app (avatars, list rows, carousel cards). Falls
                // back to NetworkImage when no cache key is supplied —
                // some callers don't have a media-id to key on.
                image: widget.backgroundImageCacheKey != null
                    ? CachedNetworkImageProvider(
                        widget.backgroundImageUrl!,
                        cacheKey: widget.backgroundImageCacheKey,
                      )
                    : NetworkImage(widget.backgroundImageUrl!),
                fit: BoxFit.cover,
              )
            : null,
        gradient: const LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            PaperTokens.textPrimary, // text - dark base
            PaperTokens.textPrimary, // Slightly lighter
          ],
        ),
      ),
      child: Container(
        // Gradient overlay for readability when image is present
        decoration: widget.backgroundImageUrl != null
            ? BoxDecoration(
                gradient: LinearGradient(
                  begin: Alignment.topCenter,
                  end: Alignment.bottomCenter,
                  colors: [
                    Colors.black.withValues(alpha: 0.7),
                    Colors.black.withValues(alpha: 0.8),
                  ],
                ),
              )
            : null,
        child: SafeArea(
          top: !widget.extendToTop,
          bottom: false,
          child: Padding(
            padding: EdgeInsets.only(
              left: 20,
              right: 20,
              top: topPadding,
              bottom: 20,
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                // Floating nav (back arrow + title + optional trailing)
                if (widget.onBack != null) ...[
                  Row(
                    children: [
                      IconAction(
                        icon: Icons.arrow_back,
                        color: Colors.white,
                        semanticsLabel: context.l10n.a11yBack,
                        onPressed: widget.onBack,
                        padding: EdgeInsets.zero,
                        constraints: const BoxConstraints(),
                      ),
                      if (widget.title != null) ...[
                        const SizedBox(width: 12),
                        Expanded(
                          child: Text(
                            widget.title!,
                            style: const TextStyle(
                              color: Colors.white,
                              fontSize: 20,
                              fontWeight: FontWeight.w400,
                                                          ),
                          ),
                        ),
                      ],
                      if (widget.trailing != null) widget.trailing!,
                    ],
                  ),
                  const SizedBox(height: 16),
                ],
                // Subtitle (for item view: owner name)
                if (widget.subtitle != null) ...[
                  Text(
                    widget.subtitle!,
                    style: const TextStyle(
                      color: AppColors.transferCoralSoft,
                      fontSize: 11,
                      fontWeight: FontWeight.w600,
                      letterSpacing: 1,
                    ),
                  ),
                  const SizedBox(height: 8),
                ],
                // Title (if no onBack - item name for item view)
                if (widget.onBack == null && widget.title != null)
                  Text(
                    widget.title!,
                    style: const TextStyle(
                      color: Colors.white,
                      fontSize: 28,
                      fontWeight: FontWeight.w400,
                                            height: 1.2,
                    ),
                  ),
                // Metadata line
                if (widget.metadata != null && widget.metadata!.isNotEmpty) ...[
                  const SizedBox(height: 8),
                  Text(
                    widget.metadata!,
                    style: const TextStyle(
                      color: AppColors.transferCoralSoft,
                      fontSize: 11,
                      fontWeight: FontWeight.w600,
                      letterSpacing: 1,
                    ),
                  ),
                ],
                // Description (tappable to expand)
                if (widget.description != null) ...[
                  const SizedBox(height: 12),
                  Tappable(
                    semanticsLabel: _isDescriptionExpanded
                        ? context.l10n.a11yHideDetails
                        : context.l10n.a11yShowDetails,
                    onTap: () {
                      setState(() {
                        _isDescriptionExpanded = !_isDescriptionExpanded;
                      });
                    },
                    child: Text(
                      widget.description!,
                      maxLines: _isDescriptionExpanded ? null : 3,
                      overflow: _isDescriptionExpanded
                          ? TextOverflow.visible
                          : TextOverflow.ellipsis,
                      style: TextStyle(
                        color: Colors.white.withValues(alpha: 0.7),
                        fontSize: 14,
                        height: 1.5,
                      ),
                    ),
                  ),
                ],
                // Metric selector (for community metrics) OR Stat chips (for other views)
                if (widget.metricSelector != null) ...[
                  const SizedBox(height: 16),
                  Container(
                    padding: const EdgeInsets.only(top: 16),
                    decoration: BoxDecoration(
                      border: Border(
                        top: BorderSide(
                          color: Colors.white.withValues(alpha: 0.08),
                          width: 1,
                        ),
                      ),
                    ),
                    child: widget.metricSelector!,
                  ),
                ] else if (widget.chips.isNotEmpty) ...[
                  const SizedBox(height: 16),
                  Wrap(
                    spacing: 8,
                    runSpacing: 8,
                    children: widget.chips.map((chip) {
                      return StatChip(
                        label: chip.label,
                        onTap: chip.onTap,
                        bordered: widget.bordered,
                      );
                    }).toList(),
                  ),
                ],
                // Bottom content (e.g., hero stats row for item metrics)
                if (widget.bottomContent != null) ...[
                  const SizedBox(height: 16),
                  if (widget.showBottomDivider)
                    Container(
                      padding: const EdgeInsets.only(top: 16),
                      decoration: BoxDecoration(
                        border: Border(
                          top: BorderSide(
                            color: Colors.white.withValues(alpha: 0.12),
                            width: 1,
                          ),
                        ),
                      ),
                      child: widget.bottomContent!,
                    )
                  else
                    widget.bottomContent!,
                ],
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// StatChipData represents data for a single stat chip.
class StatChipData {
  final String label;
  final VoidCallback? onTap;

  const StatChipData({required this.label, this.onTap});
}
