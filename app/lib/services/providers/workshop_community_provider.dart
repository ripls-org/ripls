// Workshop-scoped community selection (#1898). Drives the Workshop
// screen's community switcher and the WorkshopNotifier's brief refetch.
// Does NOT propagate to Feed search, Inbox filtering, or any other
// surface — those read communitiesProvider, which stays
// portfolio-wide.
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;

import 'community_providers.dart';

/// State for [workshopCommunityProvider]: the id of the community the
/// Workshop is pinned to, or null before the user has picked one (the
/// screen then falls back to the first community — see
/// [workshopActiveCommunityProvider]).
class WorkshopCommunityNotifier extends Notifier<String?> {
  @override
  String? build() => null;

  /// Pins the Workshop to [communityId].
  void select(String? communityId) {
    state = communityId;
  }
}

/// Workshop-scoped community selection. Independent of
/// [communitiesProvider] so a Workshop selection does not leak into
/// Feed search or inbox filtering.
final workshopCommunityProvider =
    NotifierProvider<WorkshopCommunityNotifier, String?>(
  WorkshopCommunityNotifier.new,
);

/// The community the Workshop currently shows. The Workshop no longer has an
/// "Everything" aggregate scope: it always renders a single community view, so
/// when nothing is explicitly pinned this falls back to the user's first
/// community. Null only when the user belongs to no communities.
final workshopActiveCommunityProvider = Provider<CommunityItem?>((ref) {
  final communities = ref.watch(communitiesProvider).communities;
  if (communities.isEmpty) return null;
  final pinnedId = ref.watch(workshopCommunityProvider);
  if (pinnedId == null) return communities.first;
  return communities.firstWhere(
    (c) => c.id == pinnedId,
    orElse: () => communities.first,
  );
});

/// Derived list of community ids the Workshop screen's brief should query —
/// always the single [workshopActiveCommunityProvider] (or empty when the user
/// has no communities). The membership list comes from [communitiesProvider],
/// the single source of truth for the user's communities.
final workshopEnabledCommunityIdsProvider = Provider<List<String>>((ref) {
  final active = ref.watch(workshopActiveCommunityProvider);
  return active == null ? const <String>[] : [active.id];
});
