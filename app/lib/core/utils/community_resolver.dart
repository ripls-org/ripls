import 'package:logging/logging.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show SharedCommunity;

final _log = Logger('CommunityResolver');

/// resolveCommunityId returns a communityId to use for per-entity RPCs given
/// the [supplied] id (typically from screen state) and the [shared] communities
/// the entity is actually exposed in.
///
/// When [userMemberIds] is provided and non-empty, the resolver enforces
/// membership: the returned id is guaranteed to be one the user is a member of
/// (an intersection of [shared] and [userMemberIds]). This avoids
/// `permission_denied` from server endpoints that gate on
/// `RequireMemberOfActiveCommunity` for entities shared into multiple
/// communities where the user belongs to only some of them (#1705).
///
/// Resolution order:
///  1. [supplied] if it is in both [shared] and [userMemberIds] (when
///     [userMemberIds] is supplied), or in [shared] alone (when not).
///  2. First id in [shared] ∩ [userMemberIds] (when [userMemberIds] is
///     supplied and the intersection is non-empty).
///  3. First id in [shared].
///  4. null if [shared] is empty.
///
/// Logs a warning whenever [supplied] had to be corrected, capturing the
/// original, the correction, and the membership context so we can detect
/// upstream bugs that hand a stale community context to a screen.
String? resolveCommunityId({
  required String? supplied,
  required Iterable<SharedCommunity> shared,
  Iterable<String>? userMemberIds,
}) {
  final sharedIds = <String>{
    for (final c in shared) c.communityId,
  };
  // Treat an empty userMemberIds the same as null ("membership unknown") so
  // that a failed listUserCommunities fetch degrades to the pre-membership
  // behavior instead of rejecting every supplied id.
  final memberIds = (userMemberIds == null)
      ? null
      : <String>{...userMemberIds};
  final effectiveMemberIds =
      (memberIds == null || memberIds.isEmpty) ? null : memberIds;

  bool isAcceptable(String id) {
    if (!sharedIds.contains(id)) return false;
    if (effectiveMemberIds == null) return true;
    return effectiveMemberIds.contains(id);
  }

  if (supplied != null && isAcceptable(supplied)) {
    return supplied;
  }

  String? fallback;
  if (effectiveMemberIds != null) {
    for (final id in sharedIds) {
      if (effectiveMemberIds.contains(id)) {
        fallback = id;
        break;
      }
    }
  }
  fallback ??= sharedIds.isNotEmpty ? sharedIds.first : null;

  if (supplied != null && supplied.isNotEmpty && supplied != fallback) {
    _log.warning(
      'communityId not acceptable; correcting. '
      'original=$supplied corrected=$fallback shared=$sharedIds '
      'userMemberIds=${effectiveMemberIds ?? '<not supplied>'}',
    );
  }
  return fallback;
}
