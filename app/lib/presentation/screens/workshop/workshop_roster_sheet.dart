import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/utils/community_color.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem, CommunityMember;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/viewmodels/community_content_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';
import 'package:ripls/presentation/widgets/content/content_avatar.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';
import 'package:ripls/services/providers/auth_providers.dart';
import 'package:ripls/services/providers/community_providers.dart';
import 'package:ripls/services/providers/impact_providers.dart';
import 'package:ripls/services/providers/workshop_community_provider.dart';

/// WorkshopRosterSheet lists all members across the host's selected circles.
///
/// Slides in from the right via [WorkshopRosterSheet.push]. Shows a serif
/// headline with circle chips, then a dashed-divided member list with
/// per-member circle chips and impact stats.
class WorkshopRosterSheet extends ConsumerStatefulWidget {
  /// When true, renders with a transparent scaffold so it overlays the
  /// community photo as a morph-reveal panel (caller forces dark theme + paints
  /// the backdrop). When false (default), a normal opaque screen.
  final bool overlay;

  const WorkshopRosterSheet({super.key, this.overlay = false});

  /// push slides the roster sheet in from the right.
  static Future<void> push(BuildContext context) {
    return NavigationHelpers.pushScreen(
      context: context,
      screen: const WorkshopRosterSheet(),
      routeName: 'workshop_roster',
    );
  }

  @override
  ConsumerState<WorkshopRosterSheet> createState() =>
      _WorkshopRosterSheetState();
}

class _WorkshopRosterSheetState extends ConsumerState<WorkshopRosterSheet>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  String? _selectedCircleId;
  bool _chipsExpanded = false;

  void _toggleCircle(String id) {
    setState(() {
      _selectedCircleId = _selectedCircleId == id ? null : id;
    });
  }

  void _expandChips() {
    setState(() => _chipsExpanded = true);
  }

  @override
  Widget build(BuildContext context) {
    final communityState = ref.watch(communitiesProvider);
    // The roster honours the Workshop carousel pin: when a single
    // community is selected, only its members + circle chip render. When
    // "Everything" is selected, the roster spans the full portfolio.
    final ids = ref.watch(workshopEnabledCommunityIdsProvider);
    final selfId = ref.watch(authStateProvider).user?.id ?? '';

    // Collect members across circles; dedupe by user id, tracking all circles.
    final seen = <String>{};
    final allMembers = <CommunityMember>[];
    // Map member id → list of circle ids (in encounter order).
    final memberCircleIds = <String, List<String>>{};

    for (final id in ids) {
      final result = ref.watch(communityMembersProvider(id));
      result.whenData((circleMembers) {
        for (final m in circleMembers) {
          final uid = m.hasUser() ? m.user.id : '';
          if (uid.isEmpty) continue;
          memberCircleIds.putIfAbsent(uid, () => []);
          memberCircleIds[uid]!.add(id);
          if (seen.add(uid)) allMembers.add(m);
        }
      });
    }

    // Apply circle filter if one is selected.
    final filteredMembers = _selectedCircleId == null
        ? allMembers
        : allMembers.where((m) {
            final uid = m.hasUser() ? m.user.id : '';
            return memberCircleIds[uid]?.contains(_selectedCircleId) ?? false;
          }).toList();
    final displayCount = filteredMembers.length;

    final statsAsync = ref.watch(rosterStatsProvider);

    // Filter the user's circles down to the Workshop scope so the
    // chip row mirrors what the carousel pinned. Order is preserved.
    final scopeSet = ids.toSet();
    final scopedCircles = communityState.communities
        .where((c) => scopeSet.contains(c.id))
        .toList(growable: false);

    final appBar = _RosterAppBar(
      totalCount: displayCount,
      onBack: handleClose,
      overlay: widget.overlay,
    );
    final body = SafeArea(
      top: false,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(24, 0, 24, 32),
        children: [
          _buildLeadSection(context, displayCount, scopedCircles),
          _buildMembersSection(
            context,
            filteredMembers,
            memberCircleIds,
            communityState,
            selfId,
            statsAsync,
          ),
        ],
      ),
    );
    if (widget.overlay) {
      return Scaffold(
        backgroundColor: Colors.transparent,
        appBar: appBar,
        body: body,
      );
    }
    return buildSwipeableScaffold(
      backgroundColor: AppColors.background(context),
      appBar: appBar,
      body: body,
    );
  }

  Widget _buildLeadSection(
    BuildContext context,
    int totalCount,
    List<CommunityItem> circles,
  ) {
    const maxTopChips = 3;
    final visibleCircles =
        _chipsExpanded ? circles : circles.take(maxTopChips).toList();
    final overflow =
        _chipsExpanded ? 0 : math.max(0, circles.length - maxTopChips);

    return Padding(
      padding: const EdgeInsets.only(top: 16, bottom: 16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            context.l10n.workshopRosterAcrossCirclesLabel.toUpperCase(),
            style: Theme.of(context).textTheme.labelSmall?.copyWith(
                  color: AppColors.primary(context),
                  letterSpacing: 2.4,
                  fontWeight: FontWeight.w600,
                ),
          ),
          const SizedBox(height: 8),
          Text(
            _memberHeadline(totalCount),
            style: TextStyle(
              fontFamily: AppTheme.headingFont,
              fontWeight: FontWeight.w600,
              fontSize: 32,
              letterSpacing: -0.025 * 32,
              height: 1.05,
              color: AppColors.textPrimary(context),
            ),
          ),
          const SizedBox(height: 14),
          Wrap(
            spacing: 6,
            runSpacing: 6,
            children: [
              for (final c in visibleCircles)
                _TopCircleChip(
                  id: c.id,
                  name: c.name,
                  selected: _selectedCircleId == c.id,
                  onTap: () => _toggleCircle(c.id),
                ),
              if (overflow > 0)
                _MoreChip(
                  count: overflow,
                  large: true,
                  onTap: _expandChips,
                ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _buildMembersSection(
    BuildContext context,
    List<CommunityMember> members,
    Map<String, List<String>> memberCircleIds,
    CommunitiesState communityState,
    String selfId,
    AsyncValue<Map<String, MemberStats>> statsAsync,
  ) {
    final statsMap = statsAsync.asData?.value ?? {};
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Divider(height: 1, thickness: 2, color: AppColors.textPrimary(context)),
        const SizedBox(height: 14),
        Text(
          context.l10n.workshopRosterMembersLabel.toUpperCase(),
          style: Theme.of(context).textTheme.labelSmall?.copyWith(
                color: AppColors.primary(context),
                letterSpacing: 2.4,
                fontWeight: FontWeight.w600,
              ),
        ),
        const SizedBox(height: 4),
        for (int i = 0; i < members.length; i++) ...[
          if (i > 0) const _DashedDivider(),
          _MemberRow(
            member: members[i],
            circleIds: memberCircleIds[
                    members[i].hasUser() ? members[i].user.id : ''] ??
                [],
            communityState: communityState,
            isSelf: members[i].hasUser() && members[i].user.id == selfId,
            stats: statsMap[members[i].hasUser() ? members[i].user.id : ''],
          ),
        ],
      ],
    );
  }

  /// Converts a member count to a spelled-out serif headline, e.g. "Five members."
  String _memberHeadline(int count) {
    const words = [
      'Zero',
      'One',
      'Two',
      'Three',
      'Four',
      'Five',
      'Six',
      'Seven',
      'Eight',
      'Nine',
      'Ten',
    ];
    final word = count < words.length ? words[count] : count.toString();
    return '$word ${count == 1 ? 'member' : 'members'}.';
  }
}

// ---------------------------------------------------------------------------
// AppBar
// ---------------------------------------------------------------------------

class _RosterAppBar extends StatelessWidget implements PreferredSizeWidget {
  const _RosterAppBar({
    required this.totalCount,
    required this.onBack,
    this.overlay = false,
  });

  final int totalCount;
  final VoidCallback onBack;

  /// Overlay (morph) presentation: a close X in the upper-right instead of a
  /// back arrow in the upper-left, and a transparent bar.
  final bool overlay;

  @override
  Size get preferredSize => const Size.fromHeight(kToolbarHeight);

  @override
  Widget build(BuildContext context) {
    return AppBar(
      backgroundColor:
          overlay ? Colors.transparent : AppColors.background(context),
      elevation: 0,
      scrolledUnderElevation: 0,
      automaticallyImplyLeading: false,
      leadingWidth: 56,
      leading: overlay ? null : AppBarBackButton(onPressed: onBack),
      title: Text(
        context.l10n.workshopRosterCrumb(totalCount),
        style: Theme.of(context).textTheme.labelSmall?.copyWith(
              color: AppColors.textSecondary(context),
              letterSpacing: 2.2,
              fontWeight: FontWeight.w700,
              fontSize: 9.5,
            ),
      ),
      centerTitle: true,
      actions: overlay
          ? [
              IconAction(
                icon: Icons.close_rounded,
                semanticsLabel: context.l10n.a11yClose,
                onPressed: onBack,
              ),
              const SizedBox(width: 4),
            ]
          : null,
    );
  }
}

// ---------------------------------------------------------------------------
// Chip widgets
// ---------------------------------------------------------------------------

class _TopCircleChip extends StatelessWidget {
  const _TopCircleChip({
    required this.id,
    required this.name,
    this.selected = false,
    this.onTap,
  });

  final String id;
  final String name;
  final bool selected;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    // Unselected: neutral gray pill. Selected: community color to signal active filter.
    final bg = selected ? colorForCommunity(id) : Colors.black.withValues(alpha: 0.07);
    final textColor = selected ? Colors.white : AppColors.textSecondary(context);
    final semantics = selected
        ? context.l10n.workshopRosterClearFilter
        : context.l10n.workshopRosterFilterByCircle(name);
    return Tappable(
      semanticsLabel: semantics,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(99),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
        decoration:
            BoxDecoration(color: bg, borderRadius: BorderRadius.circular(99)),
        child: Text(
          name,
          style: Theme.of(context).textTheme.labelSmall?.copyWith(
                color: textColor,
                fontWeight: FontWeight.w500,
                fontSize: 12,
              ),
        ),
      ),
    );
  }
}

class _MemberCircleChip extends StatelessWidget {
  const _MemberCircleChip({required this.name});

  final String name;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.07),
        borderRadius: BorderRadius.circular(99),
      ),
      child: Text(
        name,
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
        style: Theme.of(context).textTheme.labelSmall?.copyWith(
              color: AppColors.textSecondary(context),
              fontWeight: FontWeight.w500,
              fontSize: 11,
            ),
      ),
    );
  }
}

class _MoreChip extends StatelessWidget {
  const _MoreChip({
    required this.count,
    this.large = false,
    this.onTap,
  });

  final int count;
  final bool large;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final chip = Container(
      padding: EdgeInsets.symmetric(
        horizontal: large ? 12 : 7,
        vertical: large ? 6 : 1,
      ),
      decoration: BoxDecoration(
        color: AppColors.surface(context),
        borderRadius: BorderRadius.circular(99),
      ),
      child: Text(
        '+$count',
        style: Theme.of(context).textTheme.labelSmall?.copyWith(
              color: AppColors.textSecondary(context),
              fontWeight: FontWeight.w600,
              fontSize: large ? 12 : 10,
            ),
      ),
    );
    if (onTap == null) return chip;
    return Tappable(
      semanticsLabel: context.l10n.workshopRosterExpandChips,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(99),
      child: chip,
    );
  }
}

// ---------------------------------------------------------------------------
// _YouPill
// ---------------------------------------------------------------------------

class _YouPill extends StatelessWidget {
  const _YouPill();

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.only(left: 6),
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: AppColors.primary(context).withValues(alpha: 0.10),
        borderRadius: BorderRadius.circular(99),
      ),
      child: Text(
        'You',
        style: TextStyle(
          fontSize: 9,
          fontWeight: FontWeight.w500,
          letterSpacing: 1.6,
          color: AppColors.primary(context),
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// _StatBlock
// ---------------------------------------------------------------------------

class _StatBlock extends StatelessWidget {
  const _StatBlock({required this.stats, required this.hoursUnit});

  final MemberStats? stats;
  final String hoursUnit;

  /// Splits a formatted time value like "13.5 hrs" into ("13.5", "hrs").
  (String, String) _splitHours(String raw) {
    final parts = raw.trim().split(RegExp(r'\s+'));
    if (parts.length >= 2) return (parts[0], parts.sublist(1).join(' '));
    return (raw, hoursUnit);
  }

  @override
  Widget build(BuildContext context) {
    final s = stats;
    final rawHours = s?.formattedHours;
    final (num, unit) =
        rawHours != null ? _splitHours(rawHours) : ('—', hoursUnit);

    return Container(
      constraints: const BoxConstraints(minWidth: 70),
      padding: const EdgeInsets.only(left: 10),
      decoration: BoxDecoration(
        border: Border(
          left: BorderSide(color: AppColors.border(context), width: 1),
        ),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.baseline,
        textBaseline: TextBaseline.alphabetic,
        children: [
          Text(
            num,
            style: TextStyle(
              fontFamily: AppTheme.headingFont,
              fontWeight: FontWeight.w700,
              fontSize: 22,
              letterSpacing: -0.025 * 22,
              height: 1,
              color: AppColors.textPrimary(context),
            ),
          ),
          const SizedBox(width: 4),
          Text(
            unit,
            style: Theme.of(context).textTheme.labelSmall?.copyWith(
                  color: AppColors.textSecondary(context),
                  fontWeight: FontWeight.w600,
                  letterSpacing: 0.2,
                  fontSize: 11,
                ),
          ),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// _DashedDivider
// ---------------------------------------------------------------------------

class _DashedDivider extends StatelessWidget {
  const _DashedDivider();

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 1,
      child: CustomPaint(
        painter: _DashPainter(color: AppColors.border(context)),
      ),
    );
  }
}

class _DashPainter extends CustomPainter {
  const _DashPainter({required this.color});

  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 1;
    const dashWidth = 4.0;
    const dashGap = 4.0;
    double x = 0;
    while (x < size.width) {
      canvas.drawLine(
        Offset(x, 0),
        Offset(math.min(x + dashWidth, size.width), 0),
        paint,
      );
      x += dashWidth + dashGap;
    }
  }

  @override
  bool shouldRepaint(_DashPainter old) => old.color != color;
}

// ---------------------------------------------------------------------------
// _MemberRow
// ---------------------------------------------------------------------------

class _MemberRow extends StatefulWidget {
  const _MemberRow({
    required this.member,
    required this.circleIds,
    required this.communityState,
    required this.isSelf,
    this.stats,
  });

  final CommunityMember member;
  final List<String> circleIds;
  final CommunitiesState communityState;
  final bool isSelf;
  final MemberStats? stats;

  @override
  State<_MemberRow> createState() => _MemberRowState();
}

class _MemberRowState extends State<_MemberRow> {
  bool _expanded = false;

  String _circleName(String id) => widget.communityState.communities
          .where((c) => c.id == id)
          .map((c) => c.name)
          .firstOrNull ??
      id;

  void _toggleExpanded() => setState(() => _expanded = !_expanded);

  void _openProfile(String userId) {
    ContentViewHelpers.openUserScreen(context, userId);
  }

  @override
  Widget build(BuildContext context) {
    final user = widget.member.hasUser() ? widget.member.user : User();
    final semantics = _expanded
        ? context.l10n.workshopRosterCollapseRow
        : context.l10n.workshopRosterExpandRow;

    // Transform.translate shifts the border visually into the ListView padding
    // without affecting layout or hit-testing.
    return Transform.translate(
      offset: const Offset(-2, 0),
      child: AnimatedContainer(
        duration: accessibleDuration(context, const Duration(milliseconds: 220)),
        padding: const EdgeInsets.only(left: 2),
        decoration: BoxDecoration(
          border: Border(
            left: BorderSide(
              color: _expanded
                  ? AppColors.primary(context)
                  : Colors.transparent,
              width: 2,
            ),
          ),
        ),
        child: Tappable(
          semanticsLabel: semantics,
          onTap: _toggleExpanded,
          excludeChildSemantics: false,
          child: Padding(
            padding: const EdgeInsets.symmetric(vertical: 7),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                _buildHeader(context, user),
                AnimatedSize(
                  duration: accessibleDuration(context, const Duration(milliseconds: 180)),
                  curve: Curves.easeInOut,
                  alignment: Alignment.topLeft,
                  child: _expanded
                      ? _buildExpandedDetails(context, user)
                      : const SizedBox(width: double.infinity),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildHeader(BuildContext context, User user) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        ContentAvatar(user: user, size: 40),
        const SizedBox(width: 12),
        Expanded(
          child: Row(
            children: [
              Flexible(
                child: Text(
                  user.name.isNotEmpty ? user.name : '—',
                  overflow: TextOverflow.ellipsis,
                  maxLines: 1,
                  style: TextStyle(
                    fontFamily: AppTheme.headingFont,
                    fontWeight: FontWeight.w600,
                    fontSize: 15.5,
                    letterSpacing: -0.005 * 15.5,
                    height: 1.2,
                    color: AppColors.textPrimary(context),
                  ),
                ),
              ),
              if (widget.isSelf) const _YouPill(),
            ],
          ),
        ),
        const SizedBox(width: 4),
        _StatBlock(
          stats: widget.stats,
          hoursUnit: context.l10n.workshopRosterHoursUnit,
        ),
        const SizedBox(width: 10),
        AnimatedRotation(
          turns: _expanded ? 0.5 : 0.0,
          duration: accessibleDuration(context, const Duration(milliseconds: 220)),
          child: Icon(
            Icons.keyboard_arrow_down_rounded,
            size: 18,
            color: _expanded
                ? AppColors.primary(context)
                : AppColors.textSecondary(context),
          ),
        ),
      ],
    );
  }

  Widget _buildExpandedDetails(BuildContext context, User user) {
    final stats = widget.stats;
    final money = stats?.formattedMoney;
    final emissions = stats?.formattedEmissions;
    final hasExtras = money != null || emissions != null;

    return Padding(
      // Indent past the avatar so the details align with the name.
      padding: const EdgeInsets.fromLTRB(52, 8, 0, 4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (widget.circleIds.isNotEmpty)
            Wrap(
              spacing: 4,
              runSpacing: 4,
              children: [
                for (final id in widget.circleIds)
                  _MemberCircleChip(name: _circleName(id)),
              ],
            ),
          if (hasExtras) ...[
            const SizedBox(height: 10),
            _ImpactStatsLine(money: money, emissions: emissions),
          ],
          const SizedBox(height: 10),
          if (user.id.isNotEmpty)
            _ViewProfileButton(onTap: () => _openProfile(user.id)),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// _ImpactStatsLine
// ---------------------------------------------------------------------------

class _ImpactStatsLine extends StatelessWidget {
  const _ImpactStatsLine({this.money, this.emissions});

  final String? money;
  final String? emissions;

  @override
  Widget build(BuildContext context) {
    final primary = AppColors.primary(context);
    final secondary = AppColors.textSecondary(context);
    const numStyle = TextStyle(
      fontFamily: AppTheme.headingFont,
      fontWeight: FontWeight.w600,
      fontSize: 17,
      letterSpacing: -0.01 * 17,
      height: 1.2,
    );
    final lblStyle = Theme.of(context).textTheme.labelSmall?.copyWith(
          color: secondary,
          fontWeight: FontWeight.w500,
          fontSize: 12,
          letterSpacing: 0.02 * 12,
        );
    final sepStyle = TextStyle(
      color: secondary.withValues(alpha: 0.5),
      fontSize: 15,
      height: 1.2,
    );

    return Wrap(
      crossAxisAlignment: WrapCrossAlignment.center,
      spacing: 4,
      children: [
        if (money != null) ...[
          Text(money!, style: numStyle.copyWith(color: primary)),
          Text('saved', style: lblStyle),
        ],
        if (money != null && emissions != null)
          Text('·', style: sepStyle),
        if (emissions != null) ...[
          Text(emissions!, style: numStyle.copyWith(color: primary)),
          Text('CO₂', style: lblStyle),
        ],
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// _ViewProfileButton
// ---------------------------------------------------------------------------

class _ViewProfileButton extends StatelessWidget {
  const _ViewProfileButton({required this.onTap});

  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final coral = AppColors.primary(context);
    return Tappable(
      semanticsLabel: context.l10n.workshopRosterViewProfile,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(4),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          Container(
            decoration: BoxDecoration(
              border: Border(
                bottom: BorderSide(color: coral, width: 1),
              ),
            ),
            child: Text(
              context.l10n.workshopRosterViewProfile,
              style: TextStyle(
                fontFamily: AppTheme.headingFont,
                fontSize: 14,
                fontWeight: FontWeight.w500,
                color: coral,
                height: 1.2,
                letterSpacing: -0.005 * 14,
              ),
            ),
          ),
          const SizedBox(width: 6),
          Container(
            width: 18,
            height: 18,
            decoration: BoxDecoration(
              color: coral.withValues(alpha: 0.12),
              borderRadius: BorderRadius.circular(4),
            ),
            child: Center(
              child: Icon(Icons.arrow_forward_rounded, size: 11, color: coral),
            ),
          ),
        ],
      ),
    );
  }
}
