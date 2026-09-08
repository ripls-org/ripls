import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart' show Attribution;
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/content/photo_attribution_line.dart';

/// Top-right overlay for the gear content view: the third-party photo-credit
/// line (when the hero image is from a stock provider) and, to its right, the
/// owner's overflow menu (`···`) that opens the gear Manage sheet.
///
/// Mirrors `RequestTopBar` / `ExperienceReadShell._topBar` so the content
/// surfaces share one top-bar treatment. The back button is owned by the parent
/// screen (top-left).
class GearTopBar extends StatelessWidget {
  /// Photo credit for the hero image, or null when there's nothing to credit.
  final Attribution? attribution;

  /// Whether to render the owner overflow `···`. Callers gate this on owner +
  /// non-terminal state (the Manage sheet itself decides which rows apply).
  final bool showOverflow;

  /// Opens the gear Manage sheet.
  final VoidCallback onManage;

  const GearTopBar({
    super.key,
    required this.attribution,
    required this.showOverflow,
    required this.onManage,
  });

  @override
  Widget build(BuildContext context) {
    if (attribution == null && !showOverflow) return const SizedBox.shrink();
    final topInset = MediaQuery.of(context).padding.top;
    return Positioned(
      top: topInset + 8,
      left: 12,
      right: 12,
      child: Row(
        mainAxisAlignment: MainAxisAlignment.end,
        children: [
          if (attribution != null)
            Expanded(
              child: PhotoAttributionLine(
                attribution: attribution!,
                boxed: true,
              ),
            ),
          if (showOverflow) ...[
            const SizedBox(width: 8),
            DecoratedBox(
              decoration: BoxDecoration(
                color: OverlayTokens.fieldFill,
                shape: BoxShape.circle,
              ),
              child: IconAction(
                icon: Icons.more_horiz,
                semanticsLabel: context.l10n.a11yGearOpenManageSheet,
                tooltip: context.l10n.a11yGearOpenManageSheet,
                color: AppColors.onContentImage,
                onPressed: onManage,
              ),
            ),
          ],
        ],
      ),
    );
  }
}
