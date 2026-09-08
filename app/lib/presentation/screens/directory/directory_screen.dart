import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/date_time_formatter.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/screens/communities/community_creation_modal.dart';
import 'package:ripls/presentation/screens/communities/community_public_screen.dart';
import 'package:ripls/presentation/screens/users/user_screen.dart';
import 'package:ripls/presentation/viewmodels/directory_entry.dart';
import 'package:ripls/presentation/viewmodels/directory_view_model.dart';
import 'package:ripls/presentation/viewmodels/tab_search_scope_provider.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/adaptive/content_column.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/presentation/widgets/destination_header.dart';
import 'package:ripls/presentation/widgets/group_avatar.dart';
import 'package:ripls/presentation/widgets/navigation/nav_dock.dart';
import 'package:ripls/presentation/widgets/search/search_scope_pill.dart';
import 'package:ripls/services/providers/auth_providers.dart'
    show authStateProvider;
import 'package:ripls/services/providers/media_providers.dart'
    show mediaUrlProvider;

/// The People tab (#2634), per the liquid-glass mock Rev 16: groups and
/// individuals share **one** row type — no photo cards, no sections —
/// because both are just "who moved recently." Every row reads
/// name → last action → time-ago on the right, and the whole list sorts
/// by most recent action, so what's live floats up regardless of whether
/// it's a person or a place. Nameless ad-hoc groups show a two-member
/// cluster and list first names, in the same row shape.
///
/// The title is a control ("Everyone ▾") — tapping it opens the sort &
/// filter sheet: **Sort by** reorders the one list (Recent activity is
/// the default), **Show** scopes it, but people and communities always
/// stay in the same list shape.
///
/// Never a discovery surface: only circles, ad-hoc groups, and people the
/// viewer is already connected to. Finding a specific name is the
/// universal search's job.
class DirectoryScreen extends ConsumerStatefulWidget {
  const DirectoryScreen({super.key});

  @override
  ConsumerState<DirectoryScreen> createState() => _DirectoryScreenState();
}

class _DirectoryScreenState extends ConsumerState<DirectoryScreen> {
  DirectorySort _sort = DirectorySort.recent;
  DirectoryFilter _filter = DirectoryFilter.all;

  @override
  Widget build(BuildContext context) {
    final people = ref.watch(directoryPeopleProvider).asData?.value ?? const [];
    // Nameless groups arrive without a label (the provider has no l10n) — fill
    // it in here, before arrange, which orders and buckets by displayName.
    final entries = [...ref.watch(directoryEntriesProvider), ...people]
        .map((e) => e.localized(context.l10n))
        .toList(growable: false);
    // A universal-search escape scopes the tab to its query; the pill
    // under the header cancels it.
    final searchScope = ref.watch(peopleSearchScopeProvider);
    final arranged = DirectoryView.arrange(
      entries,
      sort: _sort,
      filter: _filter,
      query: searchScope ?? '',
    );
    // The header counts state the honest totals, not the filtered view.
    final groupCount = entries
        .where((e) => e.kind != DirectoryKind.person)
        .length;
    final personCount = entries.length - groupCount;

    Widget? scopePill;
    if (searchScope != null) {
      scopePill = Padding(
        padding: const EdgeInsets.fromLTRB(16, 0, 16, 10),
        child: SearchScopePill(
          query: searchScope,
          onClear: () =>
              ref.read(peopleSearchScopeProvider.notifier).clear(),
        ),
      );
    }

    return Scaffold(
      backgroundColor: AppColors.background(context),
      body: SafeArea(
        bottom: false,
        // Header, scope pill, and rows all hold the reading measure (#2912) —
        // ContentColumn is a no-op at phone widths.
        child: arranged.isEmpty
            ? Column(
                children: [
                  ContentColumn(child: _header(context, groupCount, personCount)),
                  if (scopePill != null) ContentColumn(child: scopePill),
                  Expanded(child: _empty(context)),
                ],
              )
            : ListView(
                padding: const EdgeInsets.fromLTRB(
                    0, 0, 0, NavDock.bottomContentInset + 12),
                children: [
                  // First scroll child, so it scrolls off with the content.
                  ContentColumn(child: _header(context, groupCount, personCount)),
                  if (scopePill != null) ContentColumn(child: scopePill),
                  ContentColumn(
                    child: Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 16),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          for (final e in arranged)
                            Padding(
                              padding: const EdgeInsets.only(bottom: 9),
                              child: _EveryoneRow(
                                entry: e,
                                onTap: () => _openEntry(context, e),
                                onNameTap: e.isNameless
                                    ? () => _nameGroup(context, e)
                                    : null,
                              ),
                            ),
                        ],
                      ),
                    ),
                  ),
                ],
              ),
      ),
    );
  }

  /// Rev 14/16 dynamic title: People answers "who" — the title is the
  /// scope control and states what is actually showing (Everyone /
  /// People / Communities, per the active Show filter); the subtitle
  /// carries the scoped counts + the active ordering. Tapping the title
  /// opens the sort & filter sheet.
  Widget _header(BuildContext context, int groupCount, int personCount) {
    final l10n = context.l10n;
    final title = switch (_filter) {
      DirectoryFilter.all => l10n.directoryTitleEveryone,
      DirectoryFilter.people => l10n.directoryTitlePeople,
      DirectoryFilter.groups => l10n.directoryTitleCommunities,
    };
    final counts = switch (_filter) {
      DirectoryFilter.all => l10n.directoryCountsBoth(
          l10n.directoryCountGroups(groupCount),
          l10n.directoryCountPeople(personCount),
        ),
      DirectoryFilter.people => l10n.directoryCountPeople(personCount),
      DirectoryFilter.groups => l10n.directoryCountGroups(groupCount),
    };
    final order = _sort == DirectorySort.recent
        ? l10n.directoryOrderRecent
        : l10n.directoryOrderName;
    return DestinationHeader(
      title: '$title ▾',
      onTitleTap: _showSortFilterSheet,
      titleSemanticsLabel: l10n.directorySortFilterTitle,
      subtitle: l10n.directorySubtitlePattern(counts, order),
    );
  }

  Widget _empty(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Text(
          context.l10n.directoryEmpty,
          textAlign: TextAlign.center,
          style: TextStyle(
            fontFamily: AppTheme.headingFont,
            fontSize: 17,
            color: AppColors.textSecondary(context),
          ),
        ),
      ),
    );
  }

  /// The sort & filter bottom sheet (Library location-sheet grammar):
  /// grabber, serif title, merow-style rows with a leading glyph and the
  /// active choice wearing the "On" pill. Sort reorders the one list;
  /// Show scopes it.
  Future<void> _showSortFilterSheet() async {
    final l10n = context.l10n;
    await showAccessibleModal<void>(
      context,
      backgroundColor: Colors.transparent,
      isScrollControlled: true,
      builder: (sheetContext) => _SortFilterSheet(
        sort: _sort,
        filter: _filter,
        onSort: (s) {
          setState(() => _sort = s);
          Navigator.of(sheetContext).pop();
        },
        onFilter: (f) {
          setState(() => _filter = f);
          Navigator.of(sheetContext).pop();
        },
        l10n: l10n,
      ),
    );
  }

  void _openEntry(BuildContext context, DirectoryEntry e) {
    switch (e.kind) {
      case DirectoryKind.person:
        NavigationHelpers.pushScreen(
          context: context,
          screen: UserScreen(userId: e.id),
          routeName: 'profile',
          useRootNavigator: true,
        );
      case DirectoryKind.community:
      case DirectoryKind.group:
        NavigationHelpers.pushScreen(
          context: context,
          screen: CommunityPublicScreen(communityId: e.id),
          routeName: 'community',
        );
    }
  }

  /// The nameless row's "Name" chip (#2568 Decision 5): for the group's
  /// owner it opens the community create/AI modal in *promote* mode —
  /// the same UpdateCommunity flow the retired Workshop switcher used —
  /// and the row re-renders named when the repository refreshes the
  /// membership list. Non-owners can't promote (the Workshop rule), so
  /// the chip falls back to opening the group like the rest of the row.
  void _nameGroup(BuildContext context, DirectoryEntry e) {
    final viewerId = ref.read(authStateProvider).user?.id;
    final ownsGroup = e.ownerUserId != null && e.ownerUserId == viewerId;
    if (!ownsGroup) {
      _openEntry(context, e);
      return;
    }
    unawaited(CommunityCreationModal.show(context, promoteCommunityId: e.id));
  }
}

/// The one row shape everyone shares (mock `.personrow`): a 44px circular
/// image (a two-member cluster for nameless groups), name over the last
/// action, the time-ago top-right. Nameless groups additionally keep the
/// "name this group" prompt chip (ratified #2568 Decision 5 — the mock
/// omits it, the product decision keeps it).
class _EveryoneRow extends ConsumerWidget {
  const _EveryoneRow({
    required this.entry,
    required this.onTap,
    this.onNameTap,
  });

  final DirectoryEntry entry;
  final VoidCallback onTap;

  /// The nameless-group "Name" chip's own tap (the promote flow). Null
  /// hides the chip's separate target (named rows never show the chip).
  final VoidCallback? onNameTap;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = context.l10n;
    final name = entry.displayName.isEmpty
        ? l10n.directoryFilterGroups
        : entry.displayName;
    final action = _actionLine(context);
    final ts = entry.lastActivityUnixSec;
    final ago = ts != null ? DateTimeFormatter.formatTimeAgo(ts) : null;

    // The "Name" chip is a SIBLING of the row's Tappable, never a child:
    // a Tappable excludes descendant semantics, so a nested chip would
    // have no semantics node of its own — untappable on Flutter Web and
    // invisible to screen readers (the HomeEditorialRow pill / pulse-card
    // who-strip failure class).
    return Container(
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context),
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: AppColors.border(context)),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          Expanded(
            child: Tappable(
              semanticsLabel: name,
              onTap: onTap,
              inkBorderRadius: BorderRadius.circular(16),
              child: Padding(
                padding: const EdgeInsets.fromLTRB(13, 10, 0, 10),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.center,
                  children: [
                    _leading(context, ref),
                    const SizedBox(width: 11),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            name,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: TextStyle(
                              fontSize: 14.5,
                              fontWeight: FontWeight.w600,
                              color: AppColors.textPrimary(context),
                            ),
                          ),
                          if (action != null) ...[
                            const SizedBox(height: 2),
                            Text(
                              action,
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                              style: TextStyle(
                                fontSize: 12,
                                color: AppColors.textSecondary(context),
                              ),
                            ),
                          ],
                        ],
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
          if (entry.isNameless && onNameTap != null)
            Tappable(
              semanticsLabel: l10n.directoryNameGroup,
              semanticsIdentifier: 'directory-name-group-chip',
              onTap: onNameTap,
              inkBorderRadius: BorderRadius.circular(999),
              child: Padding(
                padding: const EdgeInsets.fromLTRB(8, 10, 0, 10),
                child: _namePrompt(context),
              ),
            ),
          if (ago != null)
            Padding(
              padding: const EdgeInsets.fromLTRB(8, 12, 13, 10),
              child: Text(
                ago,
                style: TextStyle(
                  fontSize: 11.5,
                  fontWeight: FontWeight.w500,
                  color: AppColors.textTertiary(context),
                ),
              ),
            )
          else
            const SizedBox(width: 13),
        ],
      ),
    );
  }

  /// The middle line. The mock shows the entity's last action ("Carmen
  /// asked for a stand mixer") — a per-entry action description is not on
  /// the wire yet, so this renders the best available context: groups →
  /// member preview / member count; people → the shared communities.
  String? _actionLine(BuildContext context) {
    if (entry.kind == DirectoryKind.person) {
      final communities = entry.sharedCommunityNames.join(', ');
      if (communities.isNotEmpty) return communities;
      return entry.subtitle;
    }
    if (entry.subtitle != null) return entry.subtitle;
    if (entry.memberCount != null) {
      return context.l10n.directoryMembers(entry.memberCount!);
    }
    return null;
  }

  Widget _leading(BuildContext context, WidgetRef ref) {
    // Nameless groups have no photo of their own — a member cluster stands in
    // (mock `.grpav`), built from the preview first names. Shared with the
    // Workshop switcher and the settings hub so all three agree.
    if (entry.isNameless) {
      final members = groupAvatarMembers(
        viewer: ref.watch(authStateProvider.select((s) => s.user)),
        memberPreviewFirstNames: entry.memberPreviewFirstNames,
      );
      if (members.isNotEmpty) {
        return GroupAvatar(
          members: members,
          diameter: 44,
          ringColor: AppColors.cardBackground(context),
        );
      }
    }
    final mediaId = entry.mediaId;
    if (mediaId != null) {
      final url = ref.watch(mediaUrlProvider(mediaId)).asData?.value;
      if (url != null && url.isNotEmpty) {
        return ClipOval(
          child: CachedMediaImage(
            imageUrl: url,
            cacheKey: mediaId,
            width: 44,
            height: 44,
            fit: BoxFit.cover,
            // Decorative: the row's Tappable announces the entry name.
            semanticsLabel: null,
          ),
        );
      }
    }
    return _monogram(context, entry.displayName, 44, 16);
  }


  Widget _monogram(
      BuildContext context, String name, double size, double fontSize) {
    final trimmed = name.trim();
    final initial = trimmed.isEmpty ? '#' : trimmed[0].toUpperCase();
    return Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: AppColors.primary(context).withValues(alpha: 0.18),
        shape: BoxShape.circle,
        border: Border.all(color: AppColors.background(context), width: 1.5),
      ),
      child: Text(
        initial,
        style: TextStyle(
          fontFamily: AppTheme.headingFont,
          fontSize: fontSize,
          fontWeight: FontWeight.w600,
          color: AppColors.primary(context),
        ),
      ),
    );
  }

  /// The chip's visual only — the host wraps it in its own [Tappable]
  /// (see [onNameTap]) that opens the promote-mode community modal for
  /// the group's owner.
  Widget _namePrompt(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
      decoration: BoxDecoration(
        color: AppColors.primary(context),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        context.l10n.directoryNameGroup,
        style: TextStyle(
          fontSize: 11,
          fontWeight: FontWeight.w700,
          color: AppColors.background(context),
        ),
      ),
    );
  }
}

/// The sort & filter sheet body: Sort by (Recent activity / Name A–Z) and
/// Show (Everyone / People only / Communities only). The active option in
/// each section wears the "On" pill; the rest carry a chevron.
class _SortFilterSheet extends StatelessWidget {
  const _SortFilterSheet({
    required this.sort,
    required this.filter,
    required this.onSort,
    required this.onFilter,
    required this.l10n,
  });

  final DirectorySort sort;
  final DirectoryFilter filter;
  final ValueChanged<DirectorySort> onSort;
  final ValueChanged<DirectoryFilter> onFilter;
  final AppLocalizations l10n;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.fromLTRB(20, 12, 20, 30),
      decoration: BoxDecoration(
        color: AppColors.background(context),
        borderRadius: const BorderRadius.vertical(top: Radius.circular(30)),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Center(
            child: Container(
              width: 38,
              height: 4.5,
              margin: const EdgeInsets.only(bottom: 14),
              decoration: BoxDecoration(
                color: AppColors.textTertiary(context).withValues(alpha: 0.5),
                borderRadius: BorderRadius.circular(3),
              ),
            ),
          ),
          Text(
            l10n.directorySortFilterTitle,
            style: TextStyle(
              fontFamily: AppTheme.headingFont,
              fontSize: 20,
              fontWeight: FontWeight.w600,
              color: AppColors.textPrimary(context),
            ),
          ),
          _sectionHeader(context, l10n.directorySortByHeader),
          _row(
            context,
            icon: Icons.schedule,
            title: l10n.directorySortRecentActivity,
            subtitle: l10n.directorySortRecentActivitySub,
            selected: sort == DirectorySort.recent,
            onTap: () => onSort(DirectorySort.recent),
          ),
          _row(
            context,
            icon: Icons.sort_by_alpha,
            title: l10n.directorySortNameAz,
            selected: sort == DirectorySort.alphabetical,
            onTap: () => onSort(DirectorySort.alphabetical),
          ),
          _sectionHeader(context, l10n.directoryShowHeader),
          _row(
            context,
            icon: Icons.people_outline,
            title: l10n.directoryShowEveryone,
            subtitle: l10n.directoryShowEveryoneSub,
            selected: filter == DirectoryFilter.all,
            onTap: () => onFilter(DirectoryFilter.all),
          ),
          _row(
            context,
            icon: Icons.person_outline,
            title: l10n.directoryShowPeopleOnly,
            selected: filter == DirectoryFilter.people,
            onTap: () => onFilter(DirectoryFilter.people),
          ),
          _row(
            context,
            icon: Icons.home_outlined,
            title: l10n.directoryShowCommunitiesOnly,
            selected: filter == DirectoryFilter.groups,
            onTap: () => onFilter(DirectoryFilter.groups),
          ),
        ],
      ),
    );
  }

  Widget _sectionHeader(BuildContext context, String text) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(2, 14, 2, 6),
      child: Text(
        text.toUpperCase(),
        style: TextStyle(
          fontSize: 10,
          fontWeight: FontWeight.w700,
          letterSpacing: 1.6,
          color: AppColors.textSecondary(context),
        ),
      ),
    );
  }

  Widget _row(
    BuildContext context, {
    required IconData icon,
    required String title,
    String? subtitle,
    required bool selected,
    required VoidCallback onTap,
  }) {
    return Tappable(
      semanticsLabel: title,
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 10, horizontal: 4),
        decoration: BoxDecoration(
          border: Border(
            bottom: BorderSide(
              color: AppColors.divider(context).withValues(alpha: 0.5),
            ),
          ),
        ),
        child: Row(
          children: [
            Container(
              width: 34,
              height: 34,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: AppColors.primary(context).withValues(alpha: 0.14),
                borderRadius: BorderRadius.circular(11),
              ),
              child:
                  Icon(icon, size: 17, color: AppColors.primary(context)),
            ),
            const SizedBox(width: 11),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    style: TextStyle(
                      fontSize: 13.5,
                      fontWeight: FontWeight.w600,
                      color: AppColors.textPrimary(context),
                    ),
                  ),
                  if (subtitle != null)
                    Text(
                      subtitle,
                      style: TextStyle(
                        fontSize: 11.5,
                        color: AppColors.textSecondary(context),
                      ),
                    ),
                ],
              ),
            ),
            if (selected)
              Container(
                padding:
                    const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
                decoration: BoxDecoration(
                  color: AppColors.primary(context).withValues(alpha: 0.14),
                  borderRadius: BorderRadius.circular(9),
                ),
                child: Text(
                  l10n.directorySortOn,
                  style: TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w600,
                    color: AppColors.primary(context),
                  ),
                ),
              )
            else
              Icon(Icons.chevron_right,
                  size: 18, color: AppColors.textTertiary(context)),
          ],
        ),
      ),
    );
  }
}
