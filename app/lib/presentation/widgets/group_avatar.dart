import 'package:flutter/material.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// GroupAvatar renders a nameless (ad-hoc / group-text) community as a cluster
/// of up to four member circles — like a group-chat icon — instead of a single
/// community photo. Each circle shows the member's avatar, falling back to a
/// colored initial when they have no photo (e.g. a provisional or preview-only
/// member synthesized from a first name).
///
/// Pass the members in display order (typically the viewer first, then the
/// member-preview names) so even a two-person group reads as a group. Members
/// beyond the fourth are dropped — the count is carried by the accompanying
/// "You and … + N others" label, not the avatar.
class GroupAvatar extends StatelessWidget {
  /// Members to depict, in order. Only the first four are drawn.
  final List<User> members;

  /// Overall diameter of the composite avatar (it occupies the same slot a
  /// single [radius]-sized community avatar would: diameter = radius * 2).
  final double diameter;

  /// Color of the thin ring separating overlapping circles — set to the
  /// surface background so the circles read as distinct.
  final Color ringColor;

  const GroupAvatar({
    super.key,
    required this.members,
    required this.diameter,
    required this.ringColor,
  });

  @override
  Widget build(BuildContext context) {
    final show = members.take(4).toList();
    if (show.isEmpty) {
      return SizedBox(width: diameter, height: diameter);
    }
    if (show.length == 1) {
      return _circle(show.first, diameter);
    }

    // Per-count layout (fractions of [diameter]): face size + top-left offsets.
    // Mirrors a group-chat icon — 2 on a diagonal, 3 as a triangle, 4 in a grid.
    final double f; // face size as a fraction of diameter
    final List<Offset> offsets; // top-left of each face, in fractions
    switch (show.length) {
      case 2:
        f = 0.62;
        offsets = const [Offset(0, 0), Offset(0.38, 0.38)];
        break;
      case 3:
        f = 0.56;
        offsets = const [Offset(0.22, 0), Offset(0, 0.44), Offset(0.44, 0.44)];
        break;
      default: // 4
        f = 0.48;
        offsets = const [
          Offset(0, 0),
          Offset(0.52, 0),
          Offset(0, 0.52),
          Offset(0.52, 0.52),
        ];
    }

    final faceSize = diameter * f;
    return SizedBox(
      width: diameter,
      height: diameter,
      child: Stack(
        clipBehavior: Clip.none,
        children: [
          for (int i = 0; i < show.length; i++)
            Positioned(
              left: offsets[i].dx * diameter,
              top: offsets[i].dy * diameter,
              child: _circle(show[i], faceSize),
            ),
        ],
      ),
    );
  }

  /// One ringed member circle of the given [size].
  Widget _circle(User user, double size) {
    return Container(
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        border: Border.all(color: ringColor, width: 1.5),
      ),
      child: UserAvatar(user: user, radius: size / 2 - 1.5),
    );
  }
}

/// Builds the member list for a nameless community's [GroupAvatar]: the viewer
/// first (so a two-person group still shows two circles), then preview-only
/// members synthesized from their first names. [viewer] may be null when the
/// current user isn't resolved; preview names that are blank are skipped.
List<User> groupAvatarMembers({
  required User? viewer,
  required List<String> memberPreviewFirstNames,
}) {
  return [
    ?viewer,
    for (final name in memberPreviewFirstNames)
      if (name.trim().isNotEmpty) User(name: name.trim()),
  ];
}

/// Group-avatar members for a [community], or an empty list when it has a name
/// (named communities keep their single community avatar). Convenience over
/// [groupAvatarMembers] for the community pickers, which hold a [CommunityItem].
List<User> communityGroupAvatarMembers({
  required CommunityItem community,
  required User? viewer,
}) {
  if (community.name.trim().isNotEmpty) return const [];
  return groupAvatarMembers(
    viewer: viewer,
    memberPreviewFirstNames: community.memberPreviewFirstNames,
  );
}
