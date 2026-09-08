import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/community_display.dart'
    show communityDisplayNameFromList;
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/community_avatar.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/presentation/widgets/group_avatar.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/services/community_service.dart' show CommunityMember;
import 'package:ripls/services/providers.dart'
    show communityRepositoryProvider, communitiesProvider;

/// A person shown inside a group's people reveal list.
class AccessPerson {
  final String name;
  final String initials;

  const AccessPerson({required this.name, required this.initials});
}

/// Data for a group that has access to an item.
class AccessGroup {
  final String communityId;
  final String communityName;
  final String sharedByName;
  final String sharedByInitials;
  final String sharedTimeAgo;

  /// Unix timestamp (seconds) when the item was shared with this community.
  /// Used for sorting groups oldest-to-newest.
  final int sharedAtUnixSec;

  final int memberCount;
  final bool isRelay;
  final List<AccessPerson> people;

  const AccessGroup({
    required this.communityId,
    required this.communityName,
    required this.sharedByName,
    required this.sharedByInitials,
    required this.sharedTimeAgo,
    required this.sharedAtUnixSec,
    required this.memberCount,
    this.isRelay = false,
    this.people = const [],
  });
}

/// Data for the item creator shown at the top of the chain.
class AccessCreator {
  final String name;
  final String initials;
  final String timeAgo;

  const AccessCreator({
    required this.name,
    required this.initials,
    required this.timeAgo,
  });
}

/// AccessSheet shows "N can see this" with an accordion list of communities
/// and a summary avatar stack.
class AccessSheet extends ConsumerStatefulWidget {
  final AccessCreator creator;
  final List<AccessGroup> groups;

  /// Server-computed deduplicated count of people who can see this item.
  /// Mirrors the value shown in [AccessRing] so both displays are consistent.
  final int totalPeople;

  /// Number of additional communities the item is shared with beyond those in
  /// [groups]. Non-zero when the server returns a viewer-scoped [groups] list
  /// and the item is also shared with communities the viewer is not a member
  /// of. Used to render the "N other communities" aggregate row without
  /// leaking community names or per-community member counts.
  final int otherCommunityCount;

  /// Called when the user taps "Add community". Should open the community
  /// selection modal and return the refreshed group list, or null if nothing
  /// changed.
  final Future<List<AccessGroup>?> Function()? onAddCommunity;

  /// Called when the user taps "Invite someone". Should open the invite/share
  /// sheet, await it, and return the item's refreshed group list (or null if
  /// nothing changed) — so the still-open panel reflects a community/person
  /// added through that flow without being reopened.
  final Future<List<AccessGroup>?> Function()? onInvitePerson;

  const AccessSheet({
    super.key,
    required this.creator,
    required this.groups,
    required this.totalPeople,
    this.otherCommunityCount = 0,
    this.onAddCommunity,
    this.onInvitePerson,
  });

  static Future<void> show(
    BuildContext context, {
    required AccessCreator creator,
    required List<AccessGroup> groups,
    required int totalPeople,
    int otherCommunityCount = 0,
    Future<List<AccessGroup>?> Function()? onAddCommunity,
    Future<List<AccessGroup>?> Function()? onInvitePerson,
  }) {
    return showAccessibleModal<void>(context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => AccessSheet(
        creator: creator,
        groups: groups,
        totalPeople: totalPeople,
        otherCommunityCount: otherCommunityCount,
        onAddCommunity: onAddCommunity,
        onInvitePerson: onInvitePerson,
      ),
    );
  }

  @override
  ConsumerState<AccessSheet> createState() => _AccessSheetState();
}

class _AccessSheetState extends ConsumerState<AccessSheet> {
  late List<AccessGroup> _currentGroups;
  bool _isAddingCommunity = false;

  /// Index of the currently expanded community row (-1 = none).
  int _openIndex = -1;

  /// Members per community, fetched lazily on open.
  final Map<String, List<CommunityMember>> _membersCache = {};
  final Set<String> _fetchingCommunities = {};

  @override
  void initState() {
    super.initState();
    _currentGroups = _sorted(widget.groups);
    _fetchAllMembers();
  }

  List<AccessGroup> _sorted(List<AccessGroup> groups) {
    final copy = List<AccessGroup>.from(groups);
    copy.sort((a, b) => a.sharedAtUnixSec.compareTo(b.sharedAtUnixSec));
    return copy;
  }

  /// All unique users across all fetched member lists.
  List<User> get _uniqueMembers {
    final seen = <String>{};
    final result = <User>[];
    for (final group in _currentGroups) {
      final members = _membersCache[group.communityId];
      if (members == null) continue;
      for (final m in members) {
        if (seen.add(m.user.id)) result.add(m.user);
      }
    }
    return result;
  }

  void _fetchAllMembers() {
    // `groups` is the server's caller-scoped list — only communities the caller
    // is a member of (gear/request/experience all build it from
    // viewerIDs ∩ callerSet); communities the caller can't see are summarized via
    // otherCommunityCount, never sent here. So fetch members for all groups
    // rather than re-filtering against the client community list, which lags a
    // freshly-created per-item ad-hoc community (#1705 #1676 are handled
    // server-side by the scoping). A failed fetch degrades gracefully in
    // [_fetchMembers] (caught, no retry loop).
    for (final group in _currentGroups) {
      _fetchMembers(group.communityId);
    }
  }

  Future<void> _fetchMembers(String communityId) async {
    if (_membersCache.containsKey(communityId)) return;
    if (_fetchingCommunities.contains(communityId)) return;
    if (!mounted) return;

    setState(() => _fetchingCommunities.add(communityId));
    try {
      final members = await ref
          .read(communityRepositoryProvider)
          .getMembers(communityId);
      if (mounted) {
        setState(() {
          _membersCache[communityId] = members;
          _fetchingCommunities.remove(communityId);
        });
      }
    } catch (_) {
      if (mounted) setState(() => _fetchingCommunities.remove(communityId));
    }
  }

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: MediaQuery.of(context).size.height * 0.82,
      child: GlassSheet(
        padding: EdgeInsets.zero,
        child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          // Header
          Padding(
            padding: const EdgeInsets.fromLTRB(22, 0, 22, 16),
            child: Text(
              'Shared with',
              style: TextStyle(
                fontSize: 26,
                fontWeight: FontWeight.w800,
                color: AppColors.modalTextPrimary,
                letterSpacing: -0.3,
              ),
            ),
          ),
          // Scrollable body
          Flexible(
            child: SingleChildScrollView(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  // Summary banner — may be omitted when there's nothing to
                  // show and nothing pending (e.g., all groups are non-member
                  // communities so we don't fetch members for any of them).
                  if (_currentGroups.isNotEmpty)
                    Builder(builder: (ctx) {
                      final banner = _buildSummaryBanner(ctx);
                      if (banner == null) return const SizedBox.shrink();
                      return banner;
                    }),
                  const SizedBox(height: 14),
                  // Community accordion
                  if (_currentGroups.isEmpty &&
                      widget.otherCommunityCount == 0)
                    _buildEmptyState(context)
                  else ...[
                    Builder(builder: (context) {
                      final allCommunities =
                          ref.watch(communitiesProvider).communities;
                      final communityMap = {
                        for (final c in allCommunities) c.id: c,
                      };
                      // `groups` is already the server's caller-scoped list
                      // (only communities the caller can see), so every group
                      // renders as its own expandable row — including a
                      // freshly-created per-item ad-hoc community the client
                      // list may not carry yet. Communities the caller can't
                      // see are summarized by otherCommunityCount, not leaked
                      // here (#1705 / #1676 — scoping is enforced server-side).
                      return Column(
                        children: [
                          for (int i = 0; i < _currentGroups.length; i++)
                            _buildAccordionRow(context, i, communityMap),
                          if (widget.otherCommunityCount > 0)
                            _buildOtherCommunitiesRow(
                              context,
                              widget.otherCommunityCount,
                              0,
                              hasMemberRowsBefore: _currentGroups.isNotEmpty,
                            ),
                        ],
                      );
                    }),
                  ],
                  const SizedBox(height: 8),
                ],
              ),
            ),
          ),
          // Bottom actions
          _buildBottomActions(context),
          SizedBox(height: MediaQuery.of(context).padding.bottom),
        ],
      ),
    ),
    );
  }

  Widget? _buildSummaryBanner(BuildContext context) {
    final unique = _uniqueMembers;
    final hasMembers = unique.isNotEmpty;
    final isFetching = _fetchingCommunities.isNotEmpty;
    // If we have nothing to show and nothing to wait for, skip the banner —
    // otherwise the spinner would render forever (e.g. when every group is a
    // non-member community and we don't fetch their members).
    if (!hasMembers && !isFetching) return null;

    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
      decoration: BoxDecoration(
        color: AppColors.modalInsetCardBg,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: AppColors.modalInsetCardBorder),
      ),
      child: hasMembers
          ? Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                // Row 1: avatar strip
                _buildAvatarRow(context, unique),
                const SizedBox(height: 10),
                // Row 2: names
                _buildNamesRow(context, unique),
              ],
            )
          : Center(
              child: SizedBox(
                width: 20,
                height: 20,
                child: CircularProgressIndicator(
                  strokeWidth: 1.5,
                  color: AppColors.modalTextMuted,
                ),
              ),
            ),
    );
  }

  Widget _buildAvatarRow(BuildContext context, List<User> members) {
    const size = 36.0;
    const overlap = 10.0;
    // Show up to 7 avatars before +N
    const max = 7;
    final show = members.take(max).toList();
    final extra = members.length - max;
    final itemCount = show.length + (extra > 0 ? 1 : 0);
    final totalWidth = size + (itemCount - 1) * (size - overlap);

    return SizedBox(
      height: size,
      width: totalWidth,
      child: Stack(
        children: [
          for (int i = 0; i < show.length; i++)
            Positioned(
              left: i * (size - overlap),
              child: Container(
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  border: Border.all(
                      color: AppColors.modalInsetCardBg, width: 2),
                ),
                child: UserAvatar(user: show[i], radius: size / 2 - 2),
              ),
            ),
          if (extra > 0)
            Positioned(
              left: show.length * (size - overlap),
              child: Container(
                width: size,
                height: size,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: AppColors.modalFooterDivider,
                  border: Border.all(
                      color: AppColors.modalInsetCardBg, width: 2),
                ),
                child: Center(
                  child: Text(
                    '+$extra',
                    style: TextStyle(
                      fontSize: 10,
                      fontWeight: FontWeight.w700,
                      color: AppColors.modalTextSecondary,
                    ),
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }

  Widget _buildNamesRow(BuildContext context, List<User> members) {
    // Show first names for up to 5, then "and N more"
    const maxNames = 5;
    final firstNames =
        members.take(maxNames).map((u) => u.name.split(' ').first).toList();
    final remaining = members.length - maxNames;

    final spans = <TextSpan>[];
    for (int i = 0; i < firstNames.length; i++) {
      spans.add(TextSpan(
        text: firstNames[i],
        style: TextStyle(
          fontWeight: FontWeight.w600,
          color: AppColors.modalTextPrimary,
        ),
      ));
      if (i < firstNames.length - 1 || remaining > 0) {
        final isLast = i == firstNames.length - 1 && remaining > 0;
        spans.add(TextSpan(
          text: isLast ? ', and ' : (i == firstNames.length - 2 && remaining == 0 ? ' and ' : ', '),
          style: TextStyle(color: AppColors.modalTextSecondary),
        ));
      }
    }
    if (remaining > 0) {
      spans.add(TextSpan(
        text: '$remaining more',
        style: TextStyle(color: AppColors.modalTextSecondary),
      ));
    }

    return Text.rich(
      TextSpan(
        style: TextStyle(fontSize: 14, height: 1.4),
        children: spans,
      ),
    );
  }

  Widget _buildAccordionRow(BuildContext context, int index,
      Map<String, CommunityItem> communityMap) {
    final group = _currentGroups[index];
    final resolvedCommunity = communityMap[group.communityId];
    final isOpen = _openIndex == index;
    final accentColor =
        group.isRelay ? AppColors.transferSage : AppColors.transferCoral;
    final members = _membersCache[group.communityId];
    final isLoading = _fetchingCommunities.contains(group.communityId);
    // Prefer the actual fetched member count over the proto-supplied count so
    // newly-added communities (which arrive with memberCount=0 from optimistic
    // state) show the correct number once the member list loads.
    final displayMemberCount = members?.length ?? group.memberCount;
    // Nameless (ad-hoc / per-item) communities render group-text style — "You
    // and Ada, Sam + 2 others" — resolved from the loaded community list rather
    // than showing a blank name (#2492 DISPLAY-1).
    final isNameless = group.communityName.trim().isEmpty;
    final displayName = communityDisplayNameFromList(
      communityId: group.communityId,
      fallbackName: group.communityName,
      fallbackMemberCount: displayMemberCount,
      communities: ref.watch(communitiesProvider).communities,
      l10n: context.l10n,
    );
    final memberUsers = members?.map((m) => m.user).toList() ?? const <User>[];

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Row button.
        Tappable(
          semanticsLabel: isOpen
              ? context.l10n.a11yContentAccessGroupCollapse(displayName)
              : context.l10n.a11yContentAccessGroupExpand(displayName),
          onTap: () => setState(() => _openIndex = isOpen ? -1 : index),
          child: AnimatedContainer(
            duration: accessibleDuration(context, const Duration(milliseconds: 150)),
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 12),
            decoration: BoxDecoration(
              borderRadius: BorderRadius.circular(12),
              border: Border.all(
                color: isOpen
                    ? accentColor.withValues(alpha: 0.4)
                    : Colors.transparent,
              ),
              color: isOpen
                  ? accentColor.withValues(alpha: 0.04)
                  : Colors.transparent,
            ),
            child: Row(
              children: [
                // A nameless audience reads as a group: a cluster of member
                // faces (like a group-chat icon) rather than a blank community
                // avatar. Falls back to the community avatar when members
                // haven't loaded yet.
                if (isNameless && memberUsers.isNotEmpty)
                  GroupAvatar(
                    members: memberUsers,
                    diameter: 40,
                    ringColor: AppColors.modalInsetCardBg,
                  )
                else
                  CommunityAvatar(
                    community: resolvedCommunity,
                    name: resolvedCommunity == null ? displayName : null,
                    radius: 20,
                  ),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        displayName,
                        style: TextStyle(
                          fontSize: 16,
                          fontWeight: FontWeight.w700,
                          color: AppColors.modalTextPrimary,
                        ),
                        overflow: TextOverflow.ellipsis,
                      ),
                      const SizedBox(height: 2),
                      Text(
                        context.l10n.accessSheetCommunitySubtitle(
                          group.sharedByName,
                          group.sharedTimeAgo,
                          displayMemberCount,
                        ),
                        style: TextStyle(
                          fontSize: 12,
                          color: AppColors.modalTextMuted,
                        ),
                      ),
                    ],
                  ),
                ),
                const SizedBox(width: 8),
                AnimatedRotation(
                  turns: isOpen ? 0.5 : 0,
                  duration: accessibleDuration(context, const Duration(milliseconds: 250)),
                  curve: Curves.easeInOut,
                  child: Icon(
                    Icons.keyboard_arrow_down,
                    size: 20,
                    color: AppColors.modalTextMuted,
                  ),
                ),
              ],
            ),
          ),
        ),
        // Expanded member list
        if (isOpen)
          Padding(
            padding: const EdgeInsets.only(left: 60, bottom: 8),
            child: _buildExpandedMembers(context, members, isLoading),
          ),
        // Divider after row (not after last)
        if (!isOpen && index < _currentGroups.length - 1)
          Divider(
            height: 1,
            color: AppColors.modalFooterDivider,
            indent: 8,
            endIndent: 8,
          ),
      ],
    );
  }

  /// Aggregate row for communities the viewing user is not a member of.
  /// Replaces individual rows so we don't leak their names or per-community
  /// share metadata (#1705 / #1676). Non-interactive: there's nothing to
  /// expand because we never fetch member lists for non-member communities.
  ///
  /// [count] is the number of other communities. [memberSum] is the combined
  /// member count across those communities; pass 0 when the member counts are
  /// not available (e.g. when the server omits non-member community details).
  Widget _buildOtherCommunitiesRow(
    BuildContext context,
    int count,
    int memberSum, {
    required bool hasMemberRowsBefore,
  }) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (hasMemberRowsBefore)
          Divider(
            height: 1,
            color: AppColors.modalFooterDivider,
            indent: 8,
            endIndent: 8,
          ),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 12),
          child: Row(
            children: [
              Container(
                width: 40,
                height: 40,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: AppColors.modalFooterDivider,
                ),
                child: Icon(
                  Icons.lock_outline,
                  size: 20,
                  color: AppColors.modalTextMuted,
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      context.l10n.accessSheetOtherCommunitiesTitle(count),
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                        color: AppColors.modalTextPrimary,
                      ),
                      overflow: TextOverflow.ellipsis,
                    ),
                    if (memberSum > 0) ...[
                      const SizedBox(height: 2),
                      Text(
                        context.l10n
                            .accessSheetOtherCommunitiesSubtitle(memberSum),
                        style: TextStyle(
                          fontSize: 12,
                          color: AppColors.modalTextMuted,
                        ),
                      ),
                    ],
                  ],
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _buildExpandedMembers(
      BuildContext context, List<CommunityMember>? members, bool isLoading) {
    if (isLoading || members == null) {
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 8),
        child: SizedBox(
          width: 16,
          height: 16,
          child: CircularProgressIndicator(
            strokeWidth: 1.5,
            color: AppColors.modalTextMuted,
          ),
        ),
      );
    }

    if (members.isEmpty) return const SizedBox.shrink();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        for (int j = 0; j < members.length; j++)
          Tappable(
            semanticsLabel: context.l10n.a11yContentAccessMember(members[j].user.name),
            onTap: () => ContentViewHelpers.openUserScreen(
                context, members[j].user.id),
            child: Padding(
              padding: const EdgeInsets.symmetric(vertical: 6),
              child: Row(
                children: [
                  UserAvatar(user: members[j].user, radius: 15),
                  const SizedBox(width: 10),
                  Text(
                    members[j].user.name,
                    style: TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w500,
                      color: AppColors.modalTextPrimary,
                    ),
                  ),
                ],
              ),
            ),
          ),
      ],
    );
  }

  Widget _buildEmptyState(BuildContext context) {
    const accentColor = AppColors.modalPrimaryButtonBackground;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: accentColor.withValues(alpha: 0.18)),
        color: accentColor.withValues(alpha: 0.03),
      ),
      child: Row(
        children: [
          Container(
            width: 32,
            height: 32,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              border: Border.all(
                  color: accentColor.withValues(alpha: 0.25), width: 1.5),
            ),
            child: Icon(Icons.group_add_outlined,
                size: 15, color: accentColor.withValues(alpha: 0.5)),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  context.l10n.accessSheetShareWithCommunity,
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w600,
                    color: AppColors.modalTextSecondary,
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  context.l10n.accessSheetNoCommunitiesSubtitle,
                  style: TextStyle(
                    fontSize: 11,
                    color: AppColors.modalTextMuted,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildBottomActions(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 0, 16, 14),
      child: GlassFooterButtons(
        secondaryLabel: context.l10n.accessSheetAddCommunity,
        secondaryEnabled:
            !_isAddingCommunity && widget.onAddCommunity != null,
        onSecondary: _isAddingCommunity || widget.onAddCommunity == null
            ? null
            : _handleAddCommunity,
        primaryLabel: context.l10n.accessSheetInvitePerson,
        primaryEnabled: widget.onInvitePerson != null,
        onPrimary:
            widget.onInvitePerson == null ? null : _handleInvitePerson,
      ),
    );
  }

  Future<void> _handleAddCommunity() async {
    setState(() => _isAddingCommunity = true);
    try {
      _applyFreshGroups(await widget.onAddCommunity!());
    } finally {
      if (mounted) setState(() => _isAddingCommunity = false);
    }
  }

  /// Opens the invite/share flow; on return the panel adopts the item's
  /// refreshed groups so a community/person added through that flow shows up
  /// in place (#2492).
  Future<void> _handleInvitePerson() async {
    _applyFreshGroups(await widget.onInvitePerson!());
  }

  /// Adopts a freshly-fetched group list (from add-community or invite),
  /// re-fetching members for the new set. No-op when [fresh] is null.
  void _applyFreshGroups(List<AccessGroup>? fresh) {
    if (!mounted || fresh == null) return;
    final sorted = _sorted(fresh);
    setState(() {
      _currentGroups = sorted;
      _openIndex = -1;
    });
    for (final group in sorted) {
      _fetchMembers(group.communityId);
    }
  }
}
