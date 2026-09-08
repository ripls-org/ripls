import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/presentation/viewmodels/directory_entry.dart';
import 'package:ripls/services/providers/community_providers.dart';
import 'package:ripls/services/providers/feed_providers.dart'
    show portfolioRepositoryProvider;

/// Builds the Directory address-book *community/group* entries from
/// [communitiesProvider] (issue #2568, Decision 4 — reuse the same
/// membership list the Workshop switcher uses; no new RPC). People are a
/// separate async source ([directoryPeopleProvider]) the screen merges in.
///
/// The list is membership-scoped by construction — it only ever contains
/// communities the viewer belongs to — which keeps the Directory a
/// connections surface, never a discovery one.
final directoryEntriesProvider = Provider<List<DirectoryEntry>>((ref) {
  final communities = ref.watch(communitiesProvider).communities;
  return communities.map(_entryForCommunity).toList(growable: false);
});

/// The Directory's **people** rows — the set of people the viewer is
/// actively connected to. Connection-scoped by construction: a person the
/// viewer shares no community with never appears.
///
/// This used to read `GetPortfolioInboxView.people`, pulling a whole inbox
/// assembly to get one list. `GetDirectoryPeople` is the narrow replacement
/// (#2830); the inbox RPC is gone.
final directoryPeopleProvider = FutureProvider<List<DirectoryEntry>>((ref) async {
  final resp = await ref.watch(portfolioRepositoryProvider).getDirectoryPeople();
  return resp.people.map(_entryForPerson).toList(growable: false);
});

DirectoryEntry _entryForPerson(DailyPerson p) {
  return DirectoryEntry(
    id: p.userId,
    kind: DirectoryKind.person,
    displayName: p.displayName,
    mediaId: p.mediaId.isNotEmpty ? p.mediaId : null,
    lastActivityUnixSec:
        p.hasLastActivityUnixSec() ? p.lastActivityUnixSec.toInt() : null,
    sharedCommunityNames: p.sharedCommunityNames,
  );
}

DirectoryEntry _entryForCommunity(CommunityItem c) {
  final nameless = c.name.trim().isEmpty;
  return DirectoryEntry(
    id: c.id,
    kind: nameless ? DirectoryKind.group : DirectoryKind.community,
    // A nameless group's label is a localized member rollup, which this
    // provider can't build — it has no BuildContext, and viewmodels never
    // resolve strings. It arrives empty and DirectoryScreen.build() fills it
    // via DirectoryEntry.localized before sorting.
    displayName: nameless ? '' : c.name,
    mediaId: c.mediaIds.isNotEmpty ? c.mediaIds.first : null,
    memberCount: c.memberCount,
    // Latest community-event time, surfaced by ListCommunities (#2568) so
    // Recent mode can order and day-group by activity. Absent when the
    // community has no recorded activity yet → sorts last under Recent.
    lastActivityUnixSec: c.hasLastActivityUnixSec()
        ? c.lastActivityUnixSec.toInt()
        : null,
    isNameless: nameless,
    ownerUserId: c.ownerUserId.isEmpty ? null : c.ownerUserId,
    memberPreviewFirstNames: c.memberPreviewFirstNames,
    originItemName: c.originItemName.isEmpty ? null : c.originItemName,
  );
}
