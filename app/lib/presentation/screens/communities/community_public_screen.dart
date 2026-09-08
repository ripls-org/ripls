import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/impact.pb.dart';
import 'package:ripls/data/gen/ripls/api/item.pb.dart';
import 'package:ripls/data/repositories/impact_repository.dart'
    show ImpactMetricDimension;
import 'package:ripls/presentation/screens/communities/invite_sheet.dart';
import 'package:ripls/presentation/screens/communities/shared_calendar_screen.dart';
import 'package:ripls/presentation/screens/users/profile_library_screen.dart';
import 'package:ripls/presentation/screens/workshop/workshop_money_detail_screen.dart';
import 'package:ripls/presentation/screens/workshop/workshop_problems_solved_detail_screen.dart';
import 'package:ripls/presentation/screens/workshop/workshop_time_detail_screen.dart';
import 'package:ripls/presentation/viewmodels/community_content_view_model.dart'
    show communityMembersProvider;
import 'package:ripls/presentation/viewmodels/community_impact_view_model.dart';
import 'package:ripls/presentation/viewmodels/content_expanded_provider.dart'
    show communityProfileContentExpandedProvider;
import 'package:ripls/presentation/viewmodels/profile_sheet_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel_launcher.dart';
import 'package:ripls/presentation/widgets/content/content_overflow_menu.dart';
import 'package:ripls/presentation/widgets/profile_section/impact_row_narrative_tiers.dart'
    show timeGivenNarrative;
import 'package:ripls/presentation/widgets/profile_section/metric_tile_grid.dart'
    show MetricTileData;
import 'package:ripls/presentation/widgets/profile_section/profile_action_row.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_backdrop.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_chips.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_conversation_panel.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_face_stack.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_hero.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_hero_stat.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_member_row.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_members_panel.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_metric_rows.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_pulse_pill.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_rename_sheet.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_morph.dart'
    show WorkshopOverlayHost;
import 'package:ripls/services/community_service.dart';
import 'package:ripls/services/providers.dart';
import 'package:ripls/services/providers/media_providers.dart'
    show heroMediaUrlProvider;
import 'package:ripls/services/providers/workshop_providers.dart';

// Provider for a single community. Uses CommunityRepository for proper
// cache management.
final getCommunityProvider =
    FutureProvider.family<GetCommunityResponse, String>((
      ref,
      communityId,
    ) async {
      if (communityId.isEmpty) throw Exception('Community ID is required');
      final communityRepository = ref.watch(communityRepositoryProvider);
      return communityRepository.get(communityId);
    });

/// The community's open items + capability tags + events ticker, read
/// from the scope brief. Fails open to empty/zero when the brief is
/// unavailable.
final communityOpenItemsProvider = FutureProvider.family<
    ({List<Item> items, List<String> specialties, int eventsCount}),
    String>((ref, communityId) async {
  final repo = ref.watch(workshopRepositoryProvider);
  try {
    final resp = await repo.getBrief(communityIds: [communityId]);
    if (!resp.hasBrief()) {
      return (items: <Item>[], specialties: <String>[], eventsCount: 0);
    }
    return (
      items: resp.brief.availableNowItems,
      specialties: resp.brief.specialties,
      eventsCount: resp.brief.hasEventsCount() ? resp.brief.eventsCount : 0,
    );
  } catch (_) {
    return (items: <Item>[], specialties: <String>[], eventsCount: 0);
  }
});

/// CommunityPublicScreen — the community (group) profile on the v11
/// synthesis chassis (`docs/cowork/App UXR/group-profile-v11-synthesis.html`):
/// the cover photo fills the top ~46% under a scrim dissolving into the
/// dark page (or the monogram masthead when the group has no photo —
/// same body, zero layout shift), then one scrolling column — identity
/// (kicker · serif name · tagline), the single members row, the
/// interest chips (clamped, expanding in place), the live-pulse pill,
/// and the inline circular action row (Message primary · Plans ·
/// Library). Below the header: one editorial hero stat (Time together,
/// with its serif equivalence line) over a quiet ledger (Problems
/// solved · Money saved · Library · Plans, each with plain-language
/// subtext). Every row and action expands its destination in place via
/// the morph-reveal panel. The deeper Community / Members / Discuss
/// content remains the destination behind the rows (full entry-point
/// dedup with [CommunityScreen] is the #2568 reconciliation).
class CommunityPublicScreen extends ConsumerStatefulWidget {
  final String communityId;
  final ImpactMetricDimension initialMetric;

  /// Opens the community conversation as soon as the screen has laid out,
  /// as if the viewer had pressed Message. Set when a deep link already said
  /// where the reader is going — a "say hi" notification promises the
  /// discussion, so making them find it again is a broken promise (#2876).
  final bool openConversationOnLoad;

  const CommunityPublicScreen({
    super.key,
    required this.communityId,
    this.initialMetric =
        ImpactMetricDimension.IMPACT_METRIC_DIMENSION_MONEY,
    this.openConversationOnLoad = false,
  });

  @override
  ConsumerState<CommunityPublicScreen> createState() => _CommunityScreenState();
}

class _CommunityScreenState extends ConsumerState<CommunityPublicScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  /// Handle on the Message action, so an auto-open animates from the button
  /// the viewer would have pressed rather than from nowhere.
  final GlobalKey _messageActionKey = GlobalKey();

  /// Guards the auto-open so it fires once per mount, not on every rebuild
  /// the community provider triggers.
  bool _autoOpenedConversation = false;

  @override
  Widget build(BuildContext context) {
    final communityAsync = ref.watch(getCommunityProvider(widget.communityId));

    // Wait for real data: the action row (and therefore its footprint) doesn't
    // exist during the loading state, and opening over an error surface would
    // strand the viewer behind a panel with nothing underneath.
    if (widget.openConversationOnLoad &&
        !_autoOpenedConversation &&
        communityAsync.hasValue) {
      _autoOpenedConversation = true;
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted) return;
        _openConversationFromDeepLink();
      });
    }
    return buildSwipeableScaffold(
      // The profiles commit to the dark editorial look in both themes,
      // like the content views (hybrid v3).
      backgroundColor: AppColors.darkBackground,
      body: communityAsync.when(
        data: (community) => _buildContent(context, community),
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => _buildError(context),
      ),
    );
  }

  Widget _buildError(BuildContext context) {
    return Stack(
      children: [
        Center(
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              const Icon(Icons.error_outline, size: 56, color: Colors.white38),
              const SizedBox(height: 16),
              ElevatedButton(
                onPressed: () =>
                    ref.invalidate(getCommunityProvider(widget.communityId)),
                child: Text(context.l10n.commonRetry),
              ),
            ],
          ),
        ),
        _backChrome(context),
      ],
    );
  }

  Widget _buildContent(BuildContext context, GetCommunityResponse community) {
    final l10n = context.l10n;
    final mediaId =
        community.mediaIds.isNotEmpty ? community.mediaIds.first : null;
    final heroUrl = mediaId != null
        ? ref.watch(heroMediaUrlProvider(mediaId)).asData?.value.url
        : null;
    final hasPhoto = heroUrl != null && heroUrl.isNotEmpty;
    final impactAsync = ref.watch(communityImpactProvider(widget.communityId));
    final openAsync = ref.watch(communityOpenItemsProvider(widget.communityId));
    final eventsCount = openAsync.asData?.value.eventsCount ?? 0;
    final specialties = openAsync.asData?.value.specialties ?? const <String>[];
    // The presence read feeds the header's pulse pill (the next
    // gathering); quiet while loading or on failure so the profile
    // never blocks on it.
    final sheetState =
        ref.watch(communitySheetProvider(widget.communityId)).value ??
            const QuietSheet();
    final pulseEvent = switch (sheetState) {
      NextEventSheet(event: final e) => e,
      ColdStartSheet(event: final e) => e,
      _ => null,
    };
    // A nameless (ad-hoc) community has no name to put in the hero, so it reads
    // as its members instead of rendering blank (#2937). Named communities are
    // unaffected. The roster is already loaded for the members row below.
    final heroTitle = communityLabel(
      name: community.name,
      memberCount: community.numMembers,
      memberPreviewFirstNames: _previewFirstNames(community.name),
      l10n: l10n,
    );

    return Stack(
      fit: StackFit.expand,
      children: [
        // Backdrop (v11): the cover photo fills the top ~46% under a
        // scrim dissolving into the dark page — or the monogram
        // masthead gradient when the group has no photo yet. Same body
        // either way; zero layout shift when a photo arrives.
        if (hasPhoto)
          ProfileHeroBackdrop(mediaUrl: heroUrl, mediaId: mediaId)
        else
          const ProfileMastheadBackdrop(),
        SingleChildScrollView(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              SizedBox(
                height: MediaQuery.paddingOf(context).top +
                    (hasPhoto ? 118 : 64),
              ),
              if (!hasPhoto)
                Padding(
                  padding: const EdgeInsets.fromLTRB(22, 0, 22, 14),
                  child: Align(
                    alignment: Alignment.centerLeft,
                    child: ProfileMonogram(text: _initials(heroTitle)),
                  ),
                ),
              ProfileHero(
                // The members row below carries the roster, so the hero
                // names the work instead — it never doubles up.
                eyebrow: l10n.profileHeroEyebrowBuilding.toUpperCase(),
                title: heroTitle,
                titleFontSize: 32,
                sentence: community.description.isNotEmpty
                    ? community.description
                    : null,
              ),
              const SizedBox(height: 8),
              _memberRow(context, community),
              if (specialties.isNotEmpty)
                Padding(
                  padding: const EdgeInsets.fromLTRB(22, 6, 22, 0),
                  child: ProfileChips(tags: specialties),
                ),
              if (pulseEvent != null)
                Padding(
                  padding: const EdgeInsets.fromLTRB(22, 14, 22, 0),
                  child: Align(
                    alignment: Alignment.centerLeft,
                    child: ProfilePulsePill(
                      text: l10n.profilePulseNext(pulseEvent.title),
                    ),
                  ),
                ),
              Padding(
                padding: const EdgeInsets.fromLTRB(22, 16, 22, 0),
                child: _actionRow(context),
              ),
              const SizedBox(height: 18),
              impactAsync.when(
                data: (data) => Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    _heroStat(context, data.metrics),
                    ProfileMetricRows(
                      metrics: _rows(context, data.metrics, eventsCount),
                    ),
                  ],
                ),
                loading: () => const SizedBox(height: 320),
                error: (_, _) => const SizedBox.shrink(),
              ),
              SizedBox(height: MediaQuery.paddingOf(context).bottom + 30),
            ],
          ),
        ),
        _backChrome(context),
        _overflowChrome(context, community),
      ],
    );
  }

  /// The header's inline action row (v11): Message primary, then Plans,
  /// Library, and Invite — each expands its destination in place, except
  /// Invite, which opens the share sheet over the group.
  ///
  /// Invite lives here because this screen is where a host lands when they
  /// think "who else should be in this group": the group page is reached
  /// from the People tab, while every other route to [InviteSheet] hangs off
  /// something else (the feed's community card, the home header, Settings →
  /// Manage members).
  Widget _actionRow(BuildContext context) {
    final l10n = context.l10n;
    return ProfileActionRow(actions: [
      ProfileActionRowItem(
        icon: Icons.chat_bubble_outline,
        label: l10n.profileActionMessage,
        isPrimary: true,
        onTapRect: _expandConversation,
        anchorKey: _messageActionKey,
      ),
      ProfileActionRowItem(
        icon: Icons.calendar_month_outlined,
        label: l10n.profileMetricPlans,
        onTapRect: (rect) => _expandScreen(
          rect,
          SharedCalendarScreen(communityIds: [widget.communityId]),
          'community_calendar',
        ),
      ),
      ProfileActionRowItem(
        icon: Icons.inventory_2_outlined,
        label: l10n.profileMetricLibrary,
        onTapRect: (rect) => _expandScreen(
          rect,
          ProfileLibraryScreen(
            items: ref
                    .read(communityOpenItemsProvider(widget.communityId))
                    .asData
                    ?.value
                    .items ??
                const [],
          ),
          'community_library',
        ),
      ),
      ProfileActionRowItem(
        icon: Icons.person_add_alt_outlined,
        label: l10n.communityInvite,
        onTapRect: (_) => _showInviteSheet(),
      ),
    ]);
  }

  /// Opens the group's invite sheet (share link + QR + member count).
  /// Unlike its neighbours this is a bottom sheet, not a morph-reveal panel,
  /// so the source rect is unused.
  void _showInviteSheet() {
    final community =
        ref.read(getCommunityProvider(widget.communityId)).asData?.value;
    showAccessibleModal(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (sheetContext) => InviteSheet(
        communityId: widget.communityId,
        // Outbound: the sheet composes share text for non-members, so a
        // nameless community takes the generic public label, never its member
        // rollup (#2937). InviteSheet also normalizes, this is belt-and-braces.
        communityName: (community?.name.trim().isNotEmpty ?? false)
            ? community!.name
            : context.l10n.communityGenericGroupLabel,
        onClose: () => Navigator.of(sheetContext).pop(),
      ),
    );
  }

  /// The one editorial stat moment (v11): Time together, with the
  /// tiered equivalence sentence under it. Drills into the workshop
  /// time detail like the old row did.
  Widget _heroStat(BuildContext context, CommunityImpactMetrics metrics) {
    final l10n = context.l10n;
    final hours = (metrics.timeBankedMinutes.mean / 60).round();
    return ProfileHeroStat(
      kicker: l10n.profileMetricTimeTogether,
      value: '$hours',
      unit: l10n.profileMetricUnitHours,
      subtext: l10n.profileTimeSubGroup,
      equivalence: timeGivenNarrative(metrics.timeBankedMinutes.mean, l10n),
      onTapRect: (rect) => _expandOverlay(
        rect,
        WorkshopTimeDetailScreen(
            communityIds: [widget.communityId], overlay: true),
        'workshop_time_detail',
      ),
    );
  }

  /// "CC" from "Campus Crew" — the monogram initials (first letters of
  /// the first two words).
  static String _initials(String name) {
    final words =
        name.trim().split(RegExp(r'\s+')).where((w) => w.isNotEmpty);
    return words.take(2).map((w) => w[0].toUpperCase()).join();
  }

  /// Other members' first names, for the group-text label a nameless community
  /// gets instead of a name. Empty for a named community — it needs no rollup —
  /// and empty while the roster loads, which [communityLabel] handles by
  /// falling back to a member count rather than a blank string.
  List<String> _previewFirstNames(String communityName) {
    if (communityName.trim().isNotEmpty) return const [];
    final viewerId = ref.watch(authStateProvider).user?.id;
    final members =
        ref.watch(communityMembersProvider(widget.communityId)).asData?.value ??
            const [];
    return members
        .where((m) => m.user.id != viewerId)
        .map((m) => m.user.name.split(' ').first)
        .where((n) => n.isNotEmpty)
        .take(3)
        .toList(growable: false);
  }

  /// The members row (v11): up to 3 member faces + an Oxford-comma name
  /// list with an overflow tail; the chevron opens the full member list
  /// in place.
  Widget _memberRow(BuildContext context, GetCommunityResponse community) {
    final l10n = context.l10n;
    final membersAsync = ref.watch(communityMembersProvider(widget.communityId));
    final members = membersAsync.asData?.value ?? const [];
    if (members.isEmpty) return const SizedBox.shrink();

    const maxShown = 3;
    final shown = members.take(maxShown).toList(growable: false);
    final names = shown
        .map((m) => m.user.name.split(' ').first)
        .where((n) => n.isNotEmpty)
        .toList(growable: false);
    final otherCount =
        (community.numMembers - names.length).clamp(0, community.numMembers);
    final title = formatNameList(l10n, names, otherCount);

    final faces = shown
        .map((m) => FaceStackEntry(
              mediaId: m.user.mediaId.isEmpty ? null : m.user.mediaId,
              initial: m.user.name.isEmpty ? null : m.user.name[0],
            ))
        .toList(growable: false);

    return ProfileMemberRow(
      faces: faces,
      title: title.isEmpty ? community.name : title,
      subtitle: l10n.profileHeroAllMembers(community.numMembers),
      // Expands the members list in place, grown from the row — the
      // same morph-reveal the conversation uses.
      onTapRect: (rect) => _expandScreen(
        rect,
        ProfileMembersPanel(communityId: widget.communityId),
        'community_members',
      ),
      semanticsLabel: title.isEmpty ? community.name : title,
    );
  }

  /// The quiet ledger under the hero stat (v11): Problems solved ·
  /// Money saved · Library · Plans, each with its plain-language
  /// subtext. Time lives in the hero stat above, not here.
  List<MetricTileData> _rows(
    BuildContext context,
    CommunityImpactMetrics metrics,
    int eventsCount,
  ) {
    final l10n = context.l10n;
    final dollars = metrics.costSavingsUsd.mean.round();
    final money = NumberFormat.simpleCurrency(
      locale: Localizations.localeOf(context).toString(),
      decimalDigits: 0,
    ).format(dollars);
    // TODO(#2568): no single community "problems solved" total is on
    // CommunityImpactMetrics; the dedicated figure is
    // GetCommunityProblemsSolvedDetail.handled_count. costSavingsCount
    // (count of cost-saving transactions) is the closest inline proxy —
    // swap to the dedicated read in a later slice.
    final problems = metrics.costSavingsCount;
    // Each row opens the metric-breakdown detail screen for that
    // dimension, scoped to this community (#2568).
    final ids = [widget.communityId];
    // Every row expands its drill-down in place, grown from the tapped
    // row's footprint — the same morph-reveal the conversation uses.
    return [
      MetricTileData(
        value: '$problems',
        label: l10n.profileMetricProblemsSolved,
        subtitle: l10n.profileSubProblemsGroup,
        onTapRect: (rect) => _expandOverlay(
          rect,
          WorkshopProblemsSolvedDetailScreen(
              communityIds: ids, overlay: true),
          'workshop_problems_solved_detail',
        ),
      ),
      MetricTileData(
        value: money,
        label: l10n.profileMetricMoneySaved,
        subtitle: l10n.profileSubMoneyGroup,
        onTapRect: (rect) => _expandOverlay(
          rect,
          WorkshopMoneyDetailScreen(communityIds: ids, overlay: true),
          'workshop_money_detail',
        ),
      ),
      MetricTileData(
        value: '${metrics.gearCount}',
        label: l10n.profileMetricLibrary,
        subtitle: l10n.profileSubLibraryGroup,
        onTapRect: (rect) => _expandScreen(
          rect,
          ProfileLibraryScreen(
            items: ref
                    .read(communityOpenItemsProvider(widget.communityId))
                    .asData
                    ?.value
                    .items ??
                const [],
          ),
          'community_library',
        ),
      ),
      MetricTileData(
        value: '$eventsCount',
        label: l10n.profileMetricPlans,
        subtitle: l10n.profileSubPlansGroup,
        onTapRect: (rect) => _expandScreen(
          rect,
          SharedCalendarScreen(communityIds: ids),
          'community_calendar',
        ),
      ),
    ];
  }

  /// Expands the crew's standing conversation inline — the same
  /// morph-reveal pattern `experienceContentView` and the other content
  /// views use, grown from the tapped message bubble's own footprint.
  /// Opens the conversation for a viewer who arrived already pointed at it.
  /// Animates from the Message button's real footprint so the reveal reads the
  /// same as a tap; if the button hasn't laid out yet (a slow frame, a narrow
  /// viewport that reflowed the row) it retries once on the next frame and
  /// then gives up rather than growing a panel from `Rect.zero`.
  ///
  /// Announced to assistive tech: the viewer didn't press anything, so nothing
  /// else would tell a screen-reader user why the surface changed.
  void _openConversationFromDeepLink({bool retry = true}) {
    final rect = profileActionRowItemBounds(_messageActionKey);
    if (rect == null) {
      if (!retry) return;
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted) return;
        _openConversationFromDeepLink(retry: false);
      });
      return;
    }
    SemanticAnnouncer.announce(context, context.l10n.a11yCommunityDiscussionOpened);
    _expandConversation(rect);
  }

  void _expandConversation(Rect rect) {
    _expandScreen(
      rect,
      ProfileConversationPanel(communityId: widget.communityId),
      'community_conversation',
    );
  }

  /// Morph-opens [screen] in place, grown from [rect] (the shared
  /// content-view expansion used for the conversation, members, and
  /// metric drill-downs). Opaque screens pass through as-is.
  void _expandScreen(Rect rect, Widget screen, String routeName) {
    openContentMorphPanel(
      context: context,
      ref: ref,
      expandedProvider:
          communityProfileContentExpandedProvider(widget.communityId),
      sourceRect: rect,
      routeName: routeName,
      screen: screen,
    );
  }

  /// Morph-opens a transparent overlay destination (the roster and the
  /// workshop metric detail screens, `overlay: true`) hosted over the
  /// community's photo — the same [WorkshopOverlayHost] arrangement the
  /// Workshop overview used for these exact screens.
  void _expandOverlay(Rect rect, Widget screen, String routeName) {
    final community =
        ref.read(getCommunityProvider(widget.communityId)).asData?.value;
    final mediaId = (community?.mediaIds.isNotEmpty ?? false)
        ? community!.mediaIds.first
        : null;
    _expandScreen(
      rect,
      WorkshopOverlayHost(backdropMediaId: mediaId, child: screen),
      routeName,
    );
  }

  Widget _backChrome(BuildContext context) {
    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.only(left: 12, top: 4),
        child: Align(
          alignment: Alignment.topLeft,
          child: Tappable(
            semanticsLabel: MaterialLocalizations.of(context).backButtonTooltip,
            onTap: handleClose,
            child: Container(
              width: 38,
              height: 38,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: OverlayTokens.fieldFill,
                border: Border.all(color: GlassTokens.borderSoft),
              ),
              child: const Icon(Icons.arrow_back_ios_new,
                  size: 16, color: OverlayTokens.textPrimary),
            ),
          ),
        ),
      ),
    );
  }

  /// The content-view overflow button (top-right, mirroring the back
  /// chrome); it carries the edit-name operation.
  Widget _overflowChrome(BuildContext context, GetCommunityResponse community) {
    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.only(right: 12, top: 4),
        child: Align(
          alignment: Alignment.topRight,
          child: Tappable(
            semanticsLabel: context.l10n.a11yProfileMoreOptions,
            onTap: () => _showOverflowMenu(context, community),
            child: Container(
              width: 38,
              height: 38,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: OverlayTokens.fieldFill,
                border: Border.all(color: GlassTokens.borderSoft),
              ),
              child: const Icon(Icons.more_horiz,
                  size: 18, color: OverlayTokens.textPrimary),
            ),
          ),
        ),
      ),
    );
  }

  void _showOverflowMenu(BuildContext context, GetCommunityResponse community) {
    unawaited(showContentOverflowMenu(
      context: context,
      config: ContentOverflowMenuConfig(
        customItems: [
          MenuItemConfig(
            label: context.l10n.profileMenuEditName,
            icon: Icons.edit_outlined,
            onTap: () => unawaited(_editName(community)),
          ),
        ],
      ),
    ));
  }

  /// Renames the community: the rename sheet pops the new name;
  /// UpdateCommunity persists it (carrying the current description and
  /// media so the blanket update clears nothing), then the profile
  /// re-reads.
  Future<void> _editName(GetCommunityResponse community) async {
    final newName = await ProfileRenameSheet.show(
      context,
      initialName: community.name,
    );
    if (newName == null || !mounted) return;
    try {
      await ref.read(communityRepositoryProvider).updateCommunity(
            id: widget.communityId,
            name: newName,
            description: community.description,
            mediaIds: community.mediaIds,
          );
    } catch (e) {
      if (mounted) ToastHelper.showError(context, e.toString());
      return;
    }
    if (!mounted) return;
    ref.invalidate(getCommunityProvider(widget.communityId));
  }
}
