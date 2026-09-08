import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/viewmodels/community_content_view_model.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/workshop_community_provider.dart';

/// Roll-up of community-member identity context used by the workshop
/// identity block.
///
/// `firstFewNames` holds up to [WorkshopIdentityRollup.maxNamed]
/// member names, with the viewer always pinned to the end of the
/// underlying ordered list before the slice. `otherCount` is the
/// number of remaining members beyond the named ones.
///
/// Pinning the viewer last means they appear by name only when the
/// scope is small enough to render every member (≤ 3 distinct people);
/// in larger scopes the viewer is folded into `otherCount`.
class WorkshopIdentityRollup {
  const WorkshopIdentityRollup({
    required this.firstFewNames,
    required this.otherCount,
  });

  static const int maxNamed = 3;

  /// Empty rollup — rendered as no member subtitle.
  static const WorkshopIdentityRollup empty = WorkshopIdentityRollup(
    firstFewNames: <String>[],
    otherCount: 0,
  );

  final List<String> firstFewNames;
  final int otherCount;
}

/// Async-derived identity rollup for the workshop screen. Fetches the
/// member list of every community in
/// [workshopEnabledCommunityIdsProvider] (so the rollup follows the
/// carousel scope), dedupes by user id, orders the names with the
/// viewer pinned to the end, and slices into
/// [WorkshopIdentityRollup.maxNamed] named slots + an `otherCount`
/// remainder.
///
/// AsyncNotifier disposal is handled automatically by Riverpod for the
/// build() path (see docs/client/architecture.md §"Async Safety in
/// Notifiers"). No `ref.mounted` checks are needed because all member
/// fetches happen in the synchronous-from-Riverpod's-perspective build
/// flow.
final workshopIdentityRollupProvider =
    FutureProvider.autoDispose<WorkshopIdentityRollup>((ref) async {
  final communityIds = ref.watch(workshopEnabledCommunityIdsProvider);
  if (communityIds.isEmpty) {
    return WorkshopIdentityRollup.empty;
  }

  final viewer = ref.watch(authStateProvider).user;
  final viewerId = viewer?.id ?? '';
  final viewerFallbackName = viewer?.name ?? '';

  final memberLists = await Future.wait(
    communityIds.map(
      (id) => ref.watch(communityMembersProvider(id).future),
    ),
  );

  // Dedupe by user id while preserving first-seen order. The viewer
  // is held aside so they can be appended at the end of the ordered
  // list regardless of which community lists them first — that way a
  // small scope still shows the viewer by name, while a larger scope
  // folds them into the "+ N others" tail.
  final seen = <String>{};
  final otherNames = <String>[];
  String? viewerFirstName;
  for (final list in memberLists) {
    for (final m in list) {
      final user = m.hasUser() ? m.user : null;
      if (user == null) continue;
      if (!seen.add(user.id)) continue;
      if (user.id == viewerId) {
        viewerFirstName = _firstName(user.name);
        continue;
      }
      otherNames.add(_firstName(user.name));
    }
  }
  // Fallback: if the viewer is authenticated but somehow not present
  // in any roster (membership not yet synced, scope = communities the
  // viewer has not joined), use the auth-state name so they still
  // surface at the end.
  if (viewerFirstName == null &&
      viewerId.isNotEmpty &&
      viewerFallbackName.isNotEmpty) {
    viewerFirstName = _firstName(viewerFallbackName);
  }

  final ordered = <String>[
    ...otherNames,
    ?viewerFirstName,
  ];
  if (ordered.isEmpty) {
    return WorkshopIdentityRollup.empty;
  }

  final named = ordered
      .take(WorkshopIdentityRollup.maxNamed)
      .toList(growable: false);
  final otherCount = ordered.length - named.length;
  return WorkshopIdentityRollup(
    firstFewNames: named,
    otherCount: otherCount,
  );
});

/// A small set of member faces for the workshop people widget, plus the total
/// distinct member count across the carousel-scoped communities.
class WorkshopMemberFaces {
  const WorkshopMemberFaces({required this.faces, required this.total});

  /// Max avatars rendered in the overlapping stack.
  static const int maxFaces = 5;

  static const WorkshopMemberFaces empty =
      WorkshopMemberFaces(faces: <User>[], total: 0);

  final List<User> faces;
  final int total;
}

/// _firstName returns the first whitespace-delimited token of a
/// display name, trimmed. Falls back to the full name when it carries
/// no whitespace.
String _firstName(String fullName) {
  final trimmed = fullName.trim();
  final space = trimmed.indexOf(RegExp(r'\s'));
  if (space < 0) return trimmed;
  return trimmed.substring(0, space);
}
