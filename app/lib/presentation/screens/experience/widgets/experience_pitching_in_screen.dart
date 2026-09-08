import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/data/gen/ripls/api/common.pb.dart' show SharedCommunity;
import 'package:ripls/data/gen/ripls/api/experience_service.pb.dart'
    show RSVPIntention;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/communities/community_creation_modal.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_rsvp_controls.dart';
import 'package:ripls/presentation/screens/experience/widgets/whos_in_manage_sheet.dart';
import 'package:ripls/presentation/viewmodels/experience_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/viewmodels/needs_scope.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_avatar.dart';
import 'package:ripls/presentation/widgets/content/content_lifecycle_phase.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel.dart';
import 'package:ripls/presentation/widgets/content/content_need_chip.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/presentation/widgets/needs/need_claim_chip.dart';
import 'package:ripls/presentation/widgets/needs/needs_actions.dart';
import 'package:ripls/presentation/widgets/sharing/item_share_sheet.dart';
import 'package:ripls/services/providers.dart'
    show contentCacheInvalidationProvider;

/// ExperiencePitchingInScreen is the full-screen roster the "Who's pitching in?"
/// widget expands into when tapped (docs/issues/2280-pitching-in-expand.md). It
/// replaces the former bottom-sheet modal: pushed via [morphRevealRoute] (which
/// grows it from the card's footprint) and dismissed by the close button or
/// system back, it shows the stocked-needs progress, the open needs as
/// tap-to-claim pills, and everyone who responded grouped by Going / Maybe /
/// Not going.
///
/// All claim/edit/RSVP interactions route through the existing shared flows
/// ([NeedClaimChip] / [NeedsActions]); this screen only composes them.
class ExperiencePitchingInScreen extends ConsumerStatefulWidget {
  final String experienceId;
  final Color accentColor;

  /// Opens the access / "who's invited" sheet — the same sheet the content
  /// view's invites pill used to open, now reached from this screen's overflow.
  final VoidCallback onShowAccess;

  const ExperiencePitchingInScreen({
    super.key,
    required this.experienceId,
    required this.accentColor,
    required this.onShowAccess,
  });

  @override
  ConsumerState<ExperiencePitchingInScreen> createState() =>
      _ExperiencePitchingInScreenState();
}

class _ExperiencePitchingInScreenState
    extends ConsumerState<ExperiencePitchingInScreen> {
  String get experienceId => widget.experienceId;
  Color get accentColor => widget.accentColor;
  VoidCallback get onShowAccess => widget.onShowAccess;

  @override
  void initState() {
    super.initState();
    // Copy the ExperienceContentView approach: refresh this panel's experience
    // state whenever a content mutation invalidates the cache (e.g. an RSVP).
    // The content view's own listener doesn't reach this pushed panel, so it
    // listens itself — otherwise the roster goes stale while the panel is open.
    // listenManual in initState, not ref.listen in build.
    ref.listenManual(contentCacheInvalidationProvider, (previous, next) {
      if (previous == next) return;
      ref.read(experienceProvider(experienceId).notifier).scheduleRefresh();
    });
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(experienceProvider(experienceId));
    final needs = ref.watch(experienceNeedsProvider(experienceId));
    final details = state.experienceDetails;

    // Shared morph-panel chrome: the semi-transparent media scrim over the
    // still-playing hero (same treatment as the conversation panel) +
    // horizontal swipe-to-close.
    return ContentMorphPanel(
      child: SafeArea(
        child: details == null
            ? const SizedBox.shrink()
            : _content(context, ref, state, needs),
      ),
    );
  }

  Widget _content(
    BuildContext context,
    WidgetRef ref,
    ExperienceState state,
    ExperienceNeedsState needs,
  ) {
    final exp = state.experienceDetails!.experience;
    final groups = state.rosterGroups();
    final actions = _actions(state, exp.owner.id, exp.name);
    // Terminal (wrapped / cancelled) events freeze this pane into a record:
    // no We need / I'll bring / Invite tiles, inert need chips, read-only
    // RSVP pills (#2724). ExperienceRsvpControls hides itself.
    final isTerminal = _isTerminal(state);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _header(context),
        Expanded(
          child: ListView(
            padding: const EdgeInsets.fromLTRB(20, 4, 20, 28),
            children: [
              ExperienceRsvpControls(
                experienceId: experienceId,
                accentColor: accentColor,
              ),
              if (!isTerminal) ...[
                const SizedBox(height: 14),
                _actionRow(context, ref, state, actions),
              ],
              const SizedBox(height: 6),
              ..._neededSection(
                context,
                ref,
                needs,
                actions,
                state.currentUserId,
                isTerminal: isTerminal,
              ),
              // One flat attendee list (Partiful-style): each person's RSVP
              // status shows as a tappable pill on the right rather than
              // splitting the roster into sections. Named communities below
              // stay collapsed to a no-reply count (#2492).
              ..._attendeeRows(context, ref, groups, needs, actions, state,
                  isTerminal: isTerminal),
              ..._nameGroupLink(context, state),
              ..._communitiesGroup(context, state, isTerminal: isTerminal),
            ],
          ),
        ),
      ],
    );
  }

  /// Whether the event is in a terminal lifecycle phase (wrapped/cancelled).
  bool _isTerminal(ExperienceState state) {
    final phase = state.lifecyclePhase;
    return phase == ContentLifecyclePhase.wrapped ||
        phase == ContentLifecyclePhase.cancelled;
  }

  /// The "Needed · N open" section: every need as a chip — a dashed empty-slot
  /// while open, a solid ✓ chip once claimed (tagged "· you" when the viewer is
  /// bringing it). The header counts open slots only and flips to "All covered"
  /// when nothing is left. Adding a need now lives in the top action row. On a
  /// terminal event the chips render inert — no "+" and no claim sheet (#2724).
  List<Widget> _neededSection(
    BuildContext context,
    WidgetRef ref,
    ExperienceNeedsState needs,
    NeedsActions actions,
    String? currentUserId, {
    required bool isTerminal,
  }) {
    final l10n = context.l10n;
    // Hide the section entirely (including the "All covered" header) when there
    // is nothing to bring and no offers — an empty event shouldn't claim to be
    // "all covered".
    if (needs.needs.isEmpty && needs.contributions.isEmpty) return const [];
    // Titles the viewer is bringing, for the "· you" attribution on met chips.
    final myTitles = <String>{
      for (final c in needs.contributions)
        if (c.contributor.id == currentUserId && c.title.isNotEmpty)
          c.title.toLowerCase(),
    };
    final openCount = needs.needs.where((n) => n.slotsRemaining > 0).length;
    final header = openCount > 0
        ? l10n.pitchingInNeededOpen(openCount)
        : l10n.pitchingInAllCovered;
    return [
      _groupHeader(header),
      const SizedBox(height: 8),
      Wrap(
        spacing: 8,
        runSpacing: 8,
        children: [
          for (final n in needs.needs)
            ContentNeedChip(
              label: n.name,
              accentColor: accentColor,
              semanticsLabel: n.name,
              quantity: n.slots,
              style: n.slotsRemaining <= 0
                  ? ContentNeedChipStyle.claimed
                  : ContentNeedChipStyle.open,
              attribution: myTitles.contains(n.name.toLowerCase())
                  ? l10n.pitchingInClaimedByYou
                  : null,
              onTap: isTerminal
                  ? null
                  : () => actions.openClaimSheet(
                        context,
                        ref,
                        name: n.name,
                        note: n.note,
                        proposerName: n.proposer.name,
                        linkedNeedId: n.id,
                        slotsNeeded: n.slots,
                      ),
            ),
        ],
      ),
      const SizedBox(height: 8),
    ];
  }

  /// The top action row: "I'll bring" (primary) · "We need" · "Invite". These
  /// replace the inline "+ Add" chips that used to live in the Needed section
  /// and on the viewer's own row.
  Widget _actionRow(
    BuildContext context,
    WidgetRef ref,
    ExperienceState state,
    NeedsActions actions,
  ) {
    final l10n = context.l10n;
    return Row(
      children: [
        Expanded(
          child: _actionButton(
            context,
            label: l10n.pitchingInActionNeed,
            icon: Icons.add_task_rounded,
            semanticsLabel: l10n.a11yRosterAddNeed,
            onTap: () => actions.openAddNeed(context, ref),
          ),
        ),
        const SizedBox(width: 8),
        Expanded(
          child: _actionButton(
            context,
            label: l10n.pitchingInActionBring,
            icon: Icons.volunteer_activism_rounded,
            semanticsLabel: l10n.a11yRosterAddBringing,
            primary: true,
            onTap: () => actions.openAddContribution(
              context,
              ref,
              accentColor: accentColor,
            ),
          ),
        ),
        const SizedBox(width: 8),
        Expanded(
          child: _actionButton(
            context,
            label: l10n.pitchingInActionInvite,
            icon: Icons.person_add_alt_1_rounded,
            semanticsLabel: l10n.a11yRosterInvite,
            onTap: () => _openInvite(context, ref, state),
          ),
        ),
      ],
    );
  }

  /// One outlined action button: a leading accent [icon] over its [label]. The
  /// [primary] variant carries the accent border + a stronger accent tint.
  Widget _actionButton(
    BuildContext context, {
    required String label,
    required IconData icon,
    required String semanticsLabel,
    required VoidCallback onTap,
    bool primary = false,
  }) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      child: Container(
        constraints: const BoxConstraints(minHeight: 44),
        padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 9),
        decoration: BoxDecoration(
          color: primary
              ? accentColor.withValues(alpha: 0.26)
              : GlassTokens.fillSubtle,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: primary ? accentColor : GlassTokens.border,
          ),
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, color: accentColor, size: 18),
            const SizedBox(height: 3),
            Text(
              label,
              textAlign: TextAlign.center,
              style: const TextStyle(
                color: AppColors.onContentImage,
                fontSize: 12.5,
                fontWeight: FontWeight.w600,
                height: 1.2,
              ),
            ),
          ],
        ),
      ),
    );
  }

  /// Opens the share/invite sheet for the event. The sheet handles all
  /// additive paths itself — share link, inviting people from the host's
  /// communities, and adding more communities — so it opens regardless of
  /// whether the event is currently shared to any community.
  void _openInvite(BuildContext context, WidgetRef ref, ExperienceState state) {
    final exp = state.experienceDetails?.experience;
    if (exp == null) return;
    ItemShareSheet.show(
      context,
      itemType: ShareableItemType.experience,
      itemId: exp.id,
      itemName: exp.name,
    );
  }

  /// The serif title + close button.
  Widget _header(BuildContext context) {
    final l10n = context.l10n;
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 8, 12, 12),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          Expanded(
            child: Semantics(
              header: true,
              child: Text(
                l10n.pitchingInTitle,
                style: const TextStyle(
                  fontFamily: AppTheme.headingFont,
                  fontSize: 24,
                  fontWeight: FontWeight.w600,
                  color: AppColors.onContentImage,
                ),
              ),
            ),
          ),
          IconAction(
            icon: Icons.more_horiz,
            semanticsLabel: l10n.a11yPitchingInManageAccess,
            color: AppColors.onContentImage,
            onPressed: onShowAccess,
          ),
          IconAction(
            icon: Icons.close_rounded,
            semanticsLabel: l10n.a11yClose,
            color: AppColors.onContentImage,
            // Pop reverses the clip-reveal morph back into the card.
            onPressed: () => Navigator.of(context).pop(),
          ),
        ],
      ),
    );
  }

  Widget _groupHeader(String text) {
    return Semantics(
      header: true,
      child: Padding(
        padding: const EdgeInsets.only(top: 8, bottom: 2),
        child: Text(
          text.toUpperCase(),
          style: const TextStyle(
            color: AppColors.darkTextTertiary,
            fontSize: 10,
            fontWeight: FontWeight.w700,
            letterSpacing: 1.2,
          ),
        ),
      ),
    );
  }

  /// A single participant: avatar · name (· YOU / HOST) with their contribution
  /// pills below and an RSVP status pill on the right. Pills open the shared
  /// claim/edit sheet; the viewer's own row is marked by a ringed avatar + a
  /// bordered "YOU" badge.
  Widget _personRow(
    BuildContext context,
    WidgetRef ref,
    RosterEntry entry,
    ExperienceNeedsState needs,
    NeedsActions actions, {
    required bool isHost,
    required bool isTerminal,
  }) {
    final uid = entry.user.id;

    // The host taps an entry to manage that person (set RSVP / remove);
    // everyone else taps to view their profile (#2492). Once the event is
    // terminal the roster is a record — status pills go read-only (#2724).
    final VoidCallback? manageTap = (isHost && !isTerminal && uid.isNotEmpty)
        ? () => WhosInManageSheet.showForMember(
              context,
              experienceId: experienceId,
              memberUserId: uid,
              memberName: entry.user.name,
              currentStatus: entry.status,
            )
        : null;

    // A person's row shows what they're *bringing* (their contributions).
    // Group by title so duplicates collapse into a single chip with a ×N badge.
    final byTitle = <String, List<ExperienceContributionResponse>>{};
    for (final c in needs.contributions.where((c) => c.contributor.id == uid)) {
      (byTitle[c.title] ??= []).add(c);
    }

    final pills = <Widget>[
      for (final e in byTitle.entries)
        NeedClaimChip(
          actions: actions,
          title: e.key,
          note: _firstNote(e.value.map((c) => c.description)),
          quantity: e.value.length,
        ),
    ];

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 9),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          _profileTappable(
            context,
            entry.user,
            _avatar(entry, ringed: entry.isYou),
          ),
          const SizedBox(width: 11),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                _profileTappable(
                  context,
                  entry.user,
                  Row(
                    crossAxisAlignment: CrossAxisAlignment.center,
                    children: [
                      Flexible(
                        child: Text(
                          entry.user.name,
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(
                            color: AppColors.onContentImage,
                            fontSize: 15,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                      ),
                      const SizedBox(width: 8),
                      ?_roleTag(context, entry),
                    ],
                  ),
                ),
                if (pills.isNotEmpty) ...[
                  const SizedBox(height: 8),
                  Wrap(spacing: 8, runSpacing: 8, children: pills),
                ],
              ],
            ),
          ),
          const SizedBox(width: 8),
          // RSVP status as a tappable pill (with a chevron for the host, who can
          // change it / remove the person). Replaces the old per-status sections.
          _statusPill(context, entry.status, onTap: manageTap),
        ],
      ),
    );
  }

  /// Flat attendee list (Partiful-style): everyone in one list, each with their
  /// RSVP status as a pill on the right, no per-status section headers or
  /// dividers. Host first, then going / maybe / invited / not going.
  List<Widget> _attendeeRows(
    BuildContext context,
    WidgetRef ref,
    RosterGroups groups,
    ExperienceNeedsState needs,
    NeedsActions actions,
    ExperienceState state, {
    required bool isTerminal,
  }) {
    final all = <RosterEntry>[
      ...groups.going,
      ...groups.maybe,
      ...groups.noReply,
      ...groups.notGoing,
    ];
    if (all.isEmpty) return const [];
    final ownerId = state.experienceDetails?.experience.owner.id ?? '';
    final isHost = ownerId.isNotEmpty && ownerId == state.currentUserId;
    return [
      const SizedBox(height: 8),
      for (final entry in all)
        _personRow(context, ref, entry, needs, actions,
            isHost: isHost, isTerminal: isTerminal),
    ];
  }

  /// The RSVP status pill shown at the right of an attendee row. For the host
  /// it carries a chevron and [onTap] opens the manage sheet; for everyone else
  /// it's a plain read-only status chip.
  Widget _statusPill(
    BuildContext context,
    RosterStatus status, {
    VoidCallback? onTap,
  }) {
    final l10n = context.l10n;
    final (String label, Color color) = switch (status) {
      RosterStatus.going =>
        (l10n.experienceGoing, AppColors.experienceSageGreen),
      RosterStatus.maybe => (
        l10n.experienceRsvpMaybe,
        AppColors.statusWarningOnDark,
      ),
      RosterStatus.notGoing =>
        (l10n.pitchingInNotGoing, AppColors.darkTextTertiary),
      RosterStatus.noReply => ('Invited', AppColors.onContentImage),
    };
    final pill = Container(
      padding: const EdgeInsets.fromLTRB(10, 5, 6, 5),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.18),
        borderRadius: BorderRadius.circular(20),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            label,
            style: TextStyle(
              color: color,
              fontSize: 12,
              fontWeight: FontWeight.w600,
            ),
          ),
          Icon(
            Icons.keyboard_arrow_down,
            size: 16,
            color: onTap != null ? color : Colors.transparent,
          ),
        ],
      ),
    );
    if (onTap == null) return pill;
    return Tappable(
      semanticsLabel: l10n.pitchingInChangeStatus(label),
      onTap: onTap,
      child: pill,
    );
  }

  /// A 30px initials avatar, optionally wrapped in an accent ring (the viewer).
  Widget _avatar(RosterEntry entry, {bool ringed = false}) {
    final avatar = ContentAvatar(
      user: entry.user,
      size: 30,
      backgroundColor: accentColor,
    );
    if (!ringed) return avatar;
    return Container(
      padding: const EdgeInsets.all(2),
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        border: Border.all(color: accentColor, width: 2),
      ),
      child: avatar,
    );
  }

  /// Wraps [child] so tapping a person's name/avatar opens their profile,
  /// sliding in from the right. A no-op wrapper when the user id is unknown.
  Widget _profileTappable(BuildContext context, User user, Widget child,
      {VoidCallback? onTapOverride}) {
    if (user.id.isEmpty) return child;
    return Tappable(
      semanticsLabel: context.l10n.a11yRosterViewProfile(user.name),
      onTap: onTapOverride ??
          () => ContentViewHelpers.openUserScreen(context, user.id),
      excludeChildSemantics: false,
      child: child,
    );
  }

  /// "YOU" for the viewer (takes precedence), else "HOST" for the host, else
  /// nothing.
  Widget? _roleTag(BuildContext context, RosterEntry entry) {
    final l10n = context.l10n;
    if (entry.isYou) {
      return Container(
        padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 1.5),
        decoration: BoxDecoration(
          border: Border.all(color: accentColor),
          borderRadius: BorderRadius.circular(5),
        ),
        child: Text(
          l10n.needsRowContributorYou.toUpperCase(),
          style: TextStyle(
            color: accentColor,
            fontSize: 9,
            fontWeight: FontWeight.w800,
            letterSpacing: 0.8,
          ),
        ),
      );
    }
    if (entry.isHost) {
      return Text(
        l10n.experienceHostRole.toUpperCase(),
        style: const TextStyle(
          color: AppColors.darkTextTertiary,
          fontSize: 9,
          fontWeight: FontWeight.w700,
          letterSpacing: 0.8,
        ),
      );
    }
    return null;
  }

  /// The "Communities" section: one row per community the event is shared with,
  /// each with a count of its members who haven't responded yet (responders are
  /// listed individually in the groups above). Replaces the per-invitee
  /// "Name this group…" link, shown between the attendee list and the
  /// communities section when the event's per-item (origin) community is still
  /// nameless and the viewer is the host. Tapping it opens the create/AI modal
  /// in promote mode, turning the event's audience into a real named community
  /// (#2492). Empty otherwise.
  List<Widget> _nameGroupLink(BuildContext context, ExperienceState state) {
    final details = state.experienceDetails;
    if (details == null) return const [];
    final ownerId = details.experience.owner.id;
    final isHost = ownerId.isNotEmpty && ownerId == state.currentUserId;
    if (!isHost) return const [];
    // The event's own audience is the origin community; only nudge to name it
    // while it has no name yet.
    SharedCommunity? origin;
    for (final c in details.sharedCommunities) {
      if (c.isOriginCommunity) {
        origin = c;
        break;
      }
    }
    if (origin == null || origin.communityName.isNotEmpty) return const [];
    // Only nudge once the group has someone besides the host — naming an
    // audience of one is premature; the prompt appears when a second member
    // (an invitee) joins the per-item community (#2492).
    if (origin.memberCount < 2) return const [];
    final l10n = context.l10n;
    final originId = origin.communityId;
    return [
      const SizedBox(height: 6),
      Tappable(
        semanticsLabel: l10n.a11yPitchingInNameGroup,
        semanticsIdentifier: 'roster-name-group-link',
        excludeChildSemantics: true,
        onTap: () => _handleNameGroup(context, originId),
        child: Padding(
          padding: const EdgeInsets.symmetric(vertical: 9),
          child: Row(
            children: [
              // Rename glyph in an accent-tinted circle, matching the
              // community rows' 30px group avatars.
              Container(
                width: 30,
                height: 30,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: accentColor.withValues(alpha: 0.22),
                ),
                child: Icon(Icons.drive_file_rename_outline,
                    size: 17, color: accentColor),
              ),
              const SizedBox(width: 11),
              Text(
                l10n.pitchingInNameGroupLink,
                style: TextStyle(
                  color: accentColor,
                  fontSize: 15,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ],
          ),
        ),
      ),
    ];
  }

  Future<void> _handleNameGroup(BuildContext context, String communityId) async {
    final promoted = await CommunityCreationModal.show(
      context,
      promoteCommunityId: communityId,
    );
    if (promoted == null || !mounted) return;
    // Refresh so the roster's nameless label flips to the new name and this
    // link disappears.
    ref.read(experienceProvider(experienceId).notifier).scheduleRefresh();
  }

  /// no-reply list — the audience is shown as communities, not exploded members
  /// (#2492). Empty when the event isn't shared with any community.
  List<Widget> _communitiesGroup(
    BuildContext context,
    ExperienceState state, {
    required bool isTerminal,
  }) {
    final details = state.experienceDetails;
    if (details == null) return const [];
    // Hide the event's own ad-hoc origin community ONLY while it's nameless —
    // its members are the attendees listed above, so a nameless "Everyone
    // invited" row would be redundant. Once it's been *named* (promoted to a
    // real community), show it like any other shared community (#2492).
    final communities = details.sharedCommunities
        .where((c) => !c.isOriginCommunity || c.communityName.isNotEmpty)
        .toList();
    if (communities.isEmpty) return const [];
    final counts = details.communityNoReplyCounts;
    final ownerId = details.experience.owner.id;
    // The host's per-community manage sheet (set RSVPs / remove) freezes with
    // the rest of the pane once the event is terminal (#2724).
    final isHost =
        ownerId.isNotEmpty && ownerId == state.currentUserId && !isTerminal;
    return [
      const SizedBox(height: 10),
      _groupHeader(context.l10n.pitchingInCommunitiesHeader),
      for (final community in communities)
        _communityRow(
            context, community, counts[community.communityId] ?? 0, isHost),
    ];
  }

  Widget _communityRow(
    BuildContext context,
    SharedCommunity community,
    int noReplyCount,
    bool isHost,
  ) {
    final l10n = context.l10n;
    final name = community.communityName.isNotEmpty
        ? community.communityName
        : l10n.pitchingInCommunityNameless;
    final row = Padding(
      padding: const EdgeInsets.symmetric(vertical: 9),
      child: Row(
        children: [
          // Group glyph in an accent-tinted circle, matching the 30px avatars.
          Container(
            width: 30,
            height: 30,
            alignment: Alignment.center,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: accentColor.withValues(alpha: 0.22),
            ),
            child: Icon(Icons.groups_rounded, size: 18, color: accentColor),
          ),
          const SizedBox(width: 11),
          Expanded(
            child: Text(
              name,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                color: AppColors.onContentImage,
                fontSize: 15,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
          const SizedBox(width: 8),
          Text(
            noReplyCount > 0
                ? l10n.pitchingInCommunityNoReply(noReplyCount)
                : l10n.pitchingInCommunityAllResponded,
            style: const TextStyle(
              color: AppColors.darkTextTertiary,
              fontSize: 12,
              fontWeight: FontWeight.w600,
            ),
          ),
          if (isHost)
            Icon(Icons.keyboard_arrow_down,
                size: 16, color: AppColors.darkTextTertiary),
        ],
      ),
    );
    if (!isHost) return row;
    return Tappable(
      semanticsLabel: l10n.pitchingInManageCommunity(name),
      // Keep the child semantics (community name + no-reply count) in the tree
      // so screen readers still announce them alongside the manage action.
      excludeChildSemantics: false,
      onTap: () => WhosInManageSheet.showForCommunity(
        context,
        experienceId: experienceId,
        communityId: community.communityId,
        communityName: name,
      ),
      child: row,
    );
  }

  /// First non-empty note among [descriptions], or null — decides whether a
  /// chip shows the comment glyph.
  String? _firstNote(Iterable<String> descriptions) {
    for (final d in descriptions) {
      if (d.trim().isNotEmpty) return d;
    }
    return null;
  }

  NeedsActions _actions(ExperienceState state, String ownerId, String expName) {
    final phase = state.lifecyclePhase;
    final isTerminal =
        phase == ContentLifecyclePhase.wrapped ||
        phase == ContentLifecyclePhase.cancelled;
    final intention = state.currentUserIntention;
    return NeedsActions(
      scope: NeedsScope.experience(
        experienceId: experienceId,
        currentUserId: state.currentUserId,
        isTerminal: isTerminal,
        communityId: state.communityId ?? '',
        isRsvped:
            intention == RSVPIntention.RSVP_INTENTION_YES ||
            intention == RSVPIntention.RSVP_INTENTION_MAYBE,
        isRsvpedMaybe: intention == RSVPIntention.RSVP_INTENTION_MAYBE,
        ownerId: ownerId,
        experienceName: expName,
      ),
    );
  }
}
