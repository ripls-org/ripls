import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/l10n/app_localizations.dart';

/// The kind of a Directory row. The Directory is the viewer's private
/// address book (issue #2568) — never a discovery surface — so every
/// entry is something the viewer is already connected to.
enum DirectoryKind {
  /// A named community the viewer belongs to.
  community,

  /// An ad-hoc / nameless (per-item) community the viewer belongs to —
  /// rendered like a group chat, with a "name this group" affordance.
  group,

  /// A person the viewer shares a circle with / has interacted with.
  person,
}

/// How the Directory list is ordered.
enum DirectorySort {
  /// Newest activity first, grouped into time buckets (the default).
  recent,

  /// Alphabetical, with an A–Z scrub rail.
  alphabetical,
}

/// The type filter applied on top of the sort.
enum DirectoryFilter { all, people, groups }

/// Recency grouping for Recent-mode rows.
enum RecentBucket { today, yesterday, older }

/// One row in the Directory address book — a person, a named community,
/// or an ad-hoc group. Assembled client-side from existing reads
/// (community membership + people/activity); see
/// docs/issues/2568-directory-and-profiles.md, Decision 4.
class DirectoryEntry {
  const DirectoryEntry({
    required this.id,
    required this.kind,
    required this.displayName,
    this.mediaId,
    this.memberCount,
    this.lastActivityUnixSec,
    this.subtitle,
    this.sharedCommunityNames = const [],
    this.isNameless = false,
    this.ownerUserId,
    this.memberPreviewFirstNames = const [],
    this.originItemName,
  });

  final String id;
  final DirectoryKind kind;

  /// The shown name. **Empty for a nameless group until [localized] runs** —
  /// its label is a localized member rollup, and the provider that builds these
  /// entries has no `BuildContext` (`docs/client/i18n.md`: viewmodels never
  /// resolve strings).
  final String displayName;

  /// Avatar (person) or cover (community/group) media id, when present.
  final String? mediaId;

  /// Member count for community/group rows.
  final int? memberCount;

  /// Last-activity timestamp used by the Recent sort. May be null until
  /// the server carries a per-entry activity time (Decision 4); null
  /// entries sort last under Recent.
  final int? lastActivityUnixSec;

  /// Optional context line (recent-activity blurb under Recent, impact
  /// stats under A–Z).
  final String? subtitle;

  /// For person entries: display names of the communities this person
  /// shares with the viewer (server-capped, most-recently-active first).
  /// Person rows render these as the subtitle per the #2634 mock.
  final List<String> sharedCommunityNames;

  /// True for ad-hoc communities with no user-set name — these surface a
  /// "name this group" prompt.
  final bool isNameless;

  /// The community/group owner's user id, when the server sent one. The
  /// nameless-group "Name" chip only opens the promote flow for the
  /// owner (mirroring the Workshop switcher rule it replaced).
  final String? ownerUserId;

  /// First names of a few other members, for a nameless group's label and its
  /// avatar cluster. Empty for named communities and people.
  final List<String> memberPreviewFirstNames;

  /// For a nameless group, the item that spawned it — the subtitle line.
  final String? originItemName;

  /// Fills in the localized [displayName] and [subtitle] for a nameless group.
  ///
  /// Call this in `build()` before [DirectoryView.arrange]: arrange sorts,
  /// searches, and A–Z-buckets on `displayName`, so resolving inside the row
  /// widget would leave the ordering keyed on an empty string.
  DirectoryEntry localized(AppLocalizations l10n) {
    if (!isNameless || displayName.isNotEmpty) return this;
    final origin = originItemName?.trim() ?? '';
    return DirectoryEntry(
      id: id,
      kind: kind,
      displayName: communityLabel(
        name: '',
        memberCount: memberCount ?? 0,
        memberPreviewFirstNames: memberPreviewFirstNames,
        l10n: l10n,
      ),
      mediaId: mediaId,
      memberCount: memberCount,
      lastActivityUnixSec: lastActivityUnixSec,
      // The people are the label now, so the second line identifies the group
      // by what spawned it. Null falls through to the member count in the row.
      subtitle: origin.isNotEmpty ? l10n.workshopGroupFromItem(origin) : null,
      sharedCommunityNames: sharedCommunityNames,
      isNameless: isNameless,
      ownerUserId: ownerUserId,
      memberPreviewFirstNames: memberPreviewFirstNames,
      originItemName: originItemName,
    );
  }
}

/// Pure ordering + filtering for the Directory list, kept out of the
/// widget so it is unit-testable.
class DirectoryView {
  /// Applies [filter] then [sort] to [entries], returning a new list.
  static List<DirectoryEntry> arrange(
    List<DirectoryEntry> entries, {
    required DirectorySort sort,
    required DirectoryFilter filter,
    String query = '',
  }) {
    final q = query.trim().toLowerCase();
    final filtered = entries.where((e) {
      switch (filter) {
        case DirectoryFilter.all:
          break;
        case DirectoryFilter.people:
          if (e.kind != DirectoryKind.person) return false;
        case DirectoryFilter.groups:
          if (e.kind != DirectoryKind.community &&
              e.kind != DirectoryKind.group) {
            return false;
          }
      }
      if (q.isEmpty) return true;
      return e.displayName.toLowerCase().contains(q);
    }).toList();

    switch (sort) {
      case DirectorySort.alphabetical:
        filtered.sort((a, b) =>
            a.displayName.toLowerCase().compareTo(b.displayName.toLowerCase()));
      case DirectorySort.recent:
        // Most-recent first; entries with no activity time sort last,
        // then alphabetically for stability.
        filtered.sort((a, b) {
          final at = a.lastActivityUnixSec;
          final bt = b.lastActivityUnixSec;
          if (at == null && bt == null) {
            return a.displayName
                .toLowerCase()
                .compareTo(b.displayName.toLowerCase());
          }
          if (at == null) return 1;
          if (bt == null) return -1;
          return bt.compareTo(at);
        });
    }
    return filtered;
  }

  /// Which recency group an entry's last-activity time falls into,
  /// relative to [now] (local time). Drives the Recent-mode day headers
  /// (Today / Yesterday / a formatted date for older days).
  static RecentBucket recentBucketFor(int unixSec, DateTime now) {
    final d = DateTime.fromMillisecondsSinceEpoch(unixSec * 1000);
    final day = DateTime(d.year, d.month, d.day);
    final today = DateTime(now.year, now.month, now.day);
    final diffDays = today.difference(day).inDays;
    if (diffDays <= 0) return RecentBucket.today;
    if (diffDays == 1) return RecentBucket.yesterday;
    return RecentBucket.older;
  }

  /// A stable per-day key for grouping Recent-mode rows under one header.
  /// Today/Yesterday collapse to fixed keys; older days key by calendar
  /// date so each distinct day gets its own header.
  static String recentGroupKey(int unixSec, DateTime now) {
    final bucket = recentBucketFor(unixSec, now);
    switch (bucket) {
      case RecentBucket.today:
        return 'today';
      case RecentBucket.yesterday:
        return 'yesterday';
      case RecentBucket.older:
        final d = DateTime.fromMillisecondsSinceEpoch(unixSec * 1000);
        return '${d.year}-${d.month}-${d.day}';
    }
  }

  /// The leading uppercase letter used for the A–Z section header / rail.
  /// Non-letters bucket under '#'.
  static String azBucket(DirectoryEntry e) {
    final name = e.displayName.trimLeft();
    if (name.isEmpty) return '#';
    final c = name[0].toUpperCase();
    final code = c.codeUnitAt(0);
    if (code >= 0x41 && code <= 0x5A) return c; // A–Z
    return '#';
  }
}
