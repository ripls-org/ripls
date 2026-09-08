import 'package:flutter/material.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityMember;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_avatar.dart';

/// MemberPreviewRow shows overlapping member avatars, a comma-separated
/// first-name preview, and a "+N more" overflow count.
///
/// The row is tappable as a whole — [onTap] is invoked when the user taps
/// anywhere in the row.
///
/// Design mirrors the action-bar avatar strip in the community content view:
/// 24 px avatars, 8 px overlap, up to [maxVisible] avatars rendered, then
/// the remaining count in a pill. [textColor], [borderColor], and
/// [overflowBackgroundColor] are caller-supplied so the widget works on both
/// dark (action-bar) and light (context-band) backgrounds.
class MemberPreviewRow extends StatelessWidget {
  const MemberPreviewRow({
    super.key,
    required this.members,
    required this.totalCount,
    required this.onTap,
    this.maxVisible = 4,
    this.avatarSize = 32.0,
    this.avatarOverlap = 10.0,
    this.namesFontSize = 14.0,
    this.overflowFontSize = 11.0,
    this.textColor,
    this.borderColor,
    this.overflowBackgroundColor,
    this.semanticsLabel,
  });

  final List<CommunityMember> members;

  /// Total member count across all circles (may exceed [members.length]).
  final int totalCount;

  final VoidCallback onTap;

  /// Maximum avatars to render before showing the "+N" overflow.
  final int maxVisible;

  final double avatarSize;
  final double avatarOverlap;
  final double namesFontSize;
  final double overflowFontSize;

  /// Color for the names text and "+" count.  Defaults to [Colors.white].
  final Color? textColor;

  /// Border color rendered around each avatar.  Defaults to transparent black.
  final Color? borderColor;

  /// Background color of the "+N" overflow pill.
  final Color? overflowBackgroundColor;

  final String? semanticsLabel;

  @override
  Widget build(BuildContext context) {
    if (members.isEmpty) return const SizedBox.shrink();

    final visible = members.take(maxVisible).toList();
    final overflow = totalCount - visible.length;
    final stackWidth =
        avatarSize + (visible.length - 1) * (avatarSize - avatarOverlap);
    final effectiveTextColor = textColor ?? Colors.white;
    final effectiveBorderColor =
        borderColor ?? Colors.black.withValues(alpha: 0.4);

    final namesText = visible
        .map((m) => m.hasUser() ? _firstName(m.user.name) : '')
        .where((n) => n.isNotEmpty)
        .join(', ');

    return Tappable(
      semanticsLabel: semanticsLabel ?? namesText,
      onTap: onTap,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          SizedBox(
            width: stackWidth,
            height: avatarSize,
            child: Stack(
              children: [
                for (int i = 0; i < visible.length; i++)
                  Positioned(
                    left: i * (avatarSize - avatarOverlap),
                    child: Container(
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        border: Border.all(
                          color: effectiveBorderColor,
                          width: 1.5,
                        ),
                      ),
                      child: ContentAvatar(
                        user: visible[i].hasUser() ? visible[i].user : User(),
                        size: avatarSize,
                        backgroundColor: const Color(0xFF7A9B8C),
                      ),
                    ),
                  ),
              ],
            ),
          ),
          const SizedBox(width: 8),
          Flexible(
            child: Text(
              namesText,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                fontSize: namesFontSize,
                fontWeight: FontWeight.w600,
                color: effectiveTextColor,
              ),
            ),
          ),
          if (overflow > 0) ...[
            const SizedBox(width: 6),
            _buildOverflowPill(overflow, effectiveTextColor),
          ],
          const SizedBox(width: 4),
          Icon(
            Icons.chevron_right,
            size: 14,
            color: effectiveTextColor.withValues(alpha: 0.6),
          ),
        ],
      ),
    );
  }

  Widget _buildOverflowPill(int overflow, Color textColor) {
    final bg = overflowBackgroundColor ?? Colors.white.withValues(alpha: 0.15);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 1),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(99),
      ),
      child: Text(
        '+$overflow',
        style: TextStyle(
          fontSize: overflowFontSize,
          fontWeight: FontWeight.w700,
          color: textColor,
        ),
      ),
    );
  }

  static String _firstName(String fullName) {
    final parts = fullName.trim().split(' ');
    return parts.isNotEmpty ? parts.first : fullName;
  }
}
