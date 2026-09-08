import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';

/// AvatarStatusBadge describes the status icon and color overlaid on a user avatar.
///
/// Badges are shown as small circles (42% of avatar diameter) positioned at the
/// bottom-right corner of the avatar. Each badge has a background color and a
/// single icon conveying the user's role or status in the conversation workflow.
enum AvatarStatusBadge {
  /// Giveaway: the user has expressed interest in receiving the item.
  interested,

  /// Giveaway: the user has been selected as the recipient.
  selected,

  /// Giveaway / Request / Experience: the user owns / created the item.
  owner,

  /// Request: the user has offered to help with the request.
  helping,

  /// Request: the user submitted the request.
  requester,

  /// Experience: the user has RSVP'd as going.
  going,

  /// Experience: the user has RSVP'd as maybe.
  maybe,

  /// Experience: the user organized the experience.
  organizer,
}

/// badgeColor returns the background color for the given badge type.
Color badgeColor(AvatarStatusBadge badge, BuildContext context) {
  switch (badge) {
    case AvatarStatusBadge.interested:
      return AppColors.transferCoral;
    case AvatarStatusBadge.selected:
    case AvatarStatusBadge.helping:
    case AvatarStatusBadge.going:
      return AppColors.transferSage;
    case AvatarStatusBadge.maybe:
      return AppColors.statusWarning(context);
    case AvatarStatusBadge.owner:
    case AvatarStatusBadge.requester:
    case AvatarStatusBadge.organizer:
      return AppColors.textTertiary(context);
  }
}

/// badgeIcon returns the icon for the given badge type.
IconData badgeIcon(AvatarStatusBadge badge) {
  switch (badge) {
    case AvatarStatusBadge.interested:
      return Icons.pan_tool_outlined;
    case AvatarStatusBadge.selected:
    case AvatarStatusBadge.helping:
    case AvatarStatusBadge.going:
      return Icons.check;
    case AvatarStatusBadge.maybe:
      return Icons.question_mark;
    case AvatarStatusBadge.owner:
    case AvatarStatusBadge.requester:
    case AvatarStatusBadge.organizer:
      return Icons.star;
  }
}
