import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// GearOwnerQuoteCard is the gear read-shell comment widget: the owner is the
/// sender of the opening message, so it leads with their avatar + name + role
/// ("Owns this gear" / "Owns this gear · giving it away") above a divider, then
/// the gear description as an editorial italic quote. The whole card morph-opens
/// the conversation (matching docs/cowork/App Design/gear-loan-and-giveaway.html).
class GearOwnerQuoteCard extends StatelessWidget {
  final User owner;

  /// The opening quote (the gear description).
  final String description;

  /// The role line under the owner's name.
  final String roleLine;

  /// Whether to show the unread accent dot next to the chevron.
  final bool hasUnread;

  final Color accentColor;

  /// Opens the conversation, receiving the card's on-screen rect so the caller
  /// can morph the conversation out of the card. Null → not tappable.
  final ValueChanged<Rect>? onTap;

  final String semanticsLabel;

  const GearOwnerQuoteCard({
    super.key,
    required this.owner,
    required this.description,
    required this.roleLine,
    required this.accentColor,
    required this.semanticsLabel,
    this.hasUnread = false,
    this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final card = Container(
      padding: const EdgeInsets.fromLTRB(17, 15, 17, 15),
      decoration: BoxDecoration(
        color: AppColors.darkTextPrimary.withValues(alpha: 0.05),
        borderRadius: BorderRadius.circular(18),
        border: Border.all(color: AppColors.darkBorder),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          _ownerHead(),
          if (description.trim().isNotEmpty) ...[
            const SizedBox(height: 12),
            // Cap the preview at four lines; tapping the card opens the full
            // conversation (where this description is the first comment).
            Text(
              '"${description.trim()}"',
              maxLines: 4,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                fontFamily: AppTheme.headingFont,
                fontStyle: FontStyle.italic,
                fontSize: 16.5,
                height: 1.4,
                color: AppColors.onContentImage,
              ),
            ),
          ],
        ],
      ),
    );

    if (onTap == null) return card;
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: () {
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

  Widget _ownerHead() {
    return Container(
      padding: const EdgeInsets.only(bottom: 12),
      decoration: const BoxDecoration(
        border: Border(bottom: BorderSide(color: AppColors.darkBorder)),
      ),
      child: Row(
        children: [
          // Deliberately the theme default, despite this card sitting on media.
          //
          // Overriding it to the on-dark light sage was tried and measured
          // WORSE: the circle went 2.52:1 to 1.39:1, because the washed hero is
          // a MID-tone and the light sage is closer to it than the deep green
          // is. No brand green clears a mid-tone from either direction — see
          // `glass.primary` in tokens.json. Whatever fixes this avatar is a
          // boundary (a ring) or a different backdrop, not another green.
          UserAvatar(user: owner, radius: 20),
          const SizedBox(width: 11),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  owner.name,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontSize: 15.5,
                    fontWeight: FontWeight.w700,
                    color: AppColors.onContentImage,
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  roleLine,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontSize: 12,
                    color: AppColors.darkTextSecondary,
                  ),
                ),
              ],
            ),
          ),
          if (hasUnread) ...[
            Container(
              width: 7,
              height: 7,
              decoration: BoxDecoration(
                color: accentColor,
                shape: BoxShape.circle,
              ),
            ),
            const SizedBox(width: 8),
          ],
          if (onTap != null)
            const Icon(
              Icons.chevron_right_rounded,
              size: 20,
              // The overlay ramp, not the palette's. tokens.json is explicit:
              // "Media text must come from THIS ramp, not the palette's — the
              // palette's neutral steps are validated against background/surface
              // and measure as low as 2.71:1 out here." This chevron measured
              // 2.81:1 as the palette's faint step.
              color: OverlayTokens.textFaint,
            ),
        ],
      ),
    );
  }
}
