import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/impact.pb.dart'
    show UserImpactMetrics;
import 'package:ripls/presentation/screens/communities/shared_calendar_screen.dart';
import 'package:ripls/presentation/screens/users/profile_library_screen.dart';
import 'package:ripls/presentation/viewmodels/content_expanded_provider.dart'
    show userProfileContentExpandedProvider;
import 'package:ripls/presentation/viewmodels/profile_sheet_view_model.dart';
import 'package:ripls/presentation/viewmodels/viewer_profile_view_model.dart';
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
import 'package:ripls/presentation/widgets/profile_section/profile_metric_rows.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_pulse_pill.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_rename_sheet.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';
import 'package:ripls/services/providers.dart' show userRepositoryProvider;

/// UserScreen renders the user (person) profile through the viewing
/// user's lens, on the v11 synthesis chassis
/// (`docs/cowork/App UXR/user-profile-v11-synthesis.html`): the
/// person's photo fills the top ~46% under a scrim dissolving into the
/// dark page (or the circular-monogram masthead when they have none —
/// same body, zero layout shift), then one scrolling column:
///
/// 1. Identity — "What {name} brings" kicker, serif name, tagline.
/// 2. Shared-groups line — "With you in {crew} +N" (hidden on self
///    view / zero overlap).
/// 3. Signature tags — clamped chips expanding in place.
/// 4. Live-pulse pill + the inline circular action row (Message
///    primary · Plans · Library).
/// 5. One editorial hero stat (Time together, serif equivalence line)
///    over the quiet ledger: Problems solved · Money saved · Library ·
///    Plans, each with plain-language subtext stating the
///    shared-communities scoping. Person metrics are
///    **target-aggregate** per the resolved decision in
///    docs/issues/2568-directory-and-profiles.md; cold start gates the
///    stat sections.
///
/// The top chrome is the content-view pair: back on the left, and on a
/// self view the overflow menu on the right (edit name).
///
/// Message opens the conversation of the most recent shared community —
/// the interim decision while there is no 1:1 DM surface (#2568).
class UserScreen extends ConsumerStatefulWidget {
  final String userId;
  final VoidCallback? onReturn;

  const UserScreen({super.key, required this.userId, this.onReturn});

  @override
  ConsumerState<UserScreen> createState() => _UserScreenState();
}

class _UserScreenState extends ConsumerState<UserScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  @override
  Widget build(BuildContext context) {
    final async = ref.watch(viewerProfileProvider(widget.userId));
    return buildSwipeableScaffold(
      // The profiles commit to the dark editorial look in both themes,
      // like the content views (hybrid v3).
      backgroundColor: AppColors.darkBackground,
      body: async.when(
        data: (state) => _buildContent(context, state),
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => _buildError(context, e),
      ),
    );
  }

  Widget _buildError(BuildContext context, Object e) {
    return Stack(
      children: [
        Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Text(
              e.toString(),
              style: const TextStyle(color: Colors.white70),
            ),
          ),
        ),
        _backChrome(context),
      ],
    );
  }

  Widget _buildContent(BuildContext context, ViewerProfileState state) {
    final l10n = context.l10n;
    final metrics = state.impactMetrics;
    // The presence read feeds the header's pulse pill (the next shared
    // gathering) and the cold-start gating; quiet while loading or on
    // failure so the profile never blocks on it.
    final sheetState = ref.watch(profileSheetProvider(widget.userId)).value ??
        const QuietSheet();
    final suppress = sheetState.suppressHistory;
    final pulseEvent = switch (sheetState) {
      NextEventSheet(event: final e) => e,
      ColdStartSheet(event: final e) => e,
      _ => null,
    };
    final url = state.targetMediaUrl;
    final hasPhoto = url != null && url.isNotEmpty;

    return Stack(
      fit: StackFit.expand,
      children: [
        // Backdrop (v11): the person's photo fills the top ~46% under a
        // scrim dissolving into the dark page — or the circular-monogram
        // masthead when they have none. Same body either way; zero
        // layout shift when a photo arrives.
        if (hasPhoto)
          ProfileHeroBackdrop(mediaUrl: url, mediaId: state.targetMediaId)
        else
          const ProfileMastheadBackdrop(),
        RefreshIndicator(
          onRefresh: () =>
              ref.read(viewerProfileProvider(widget.userId).notifier).refresh(),
          child: SingleChildScrollView(
            physics: const AlwaysScrollableScrollPhysics(),
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
                      child: ProfileMonogram(
                        text: _initials(state.targetName),
                        circular: true,
                      ),
                    ),
                  ),
                ProfileHero(
                  eyebrow: _heroEyebrow(context, state, sheetState),
                  title: state.targetName,
                  titleFontSize: 32,
                  sentence: state.targetDescription,
                ),
                const SizedBox(height: 8),
                if (!state.isSelfView) _sharedGroupsRow(context, state),
                if (state.knownFor.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.fromLTRB(22, 6, 22, 0),
                    child: ProfileChips(tags: state.knownFor),
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
                  child: _actionRow(context, state),
                ),
                const SizedBox(height: 18),
                if (!suppress && metrics != null) ...[
                  _heroStat(context, metrics),
                  ProfileMetricRows(metrics: _rows(context, state, metrics)),
                ],
                SizedBox(height: MediaQuery.paddingOf(context).bottom + 30),
              ],
            ),
          ),
        ),
        _backChrome(context),
        if (state.isSelfView) _overflowChrome(context, state),
      ],
    );
  }

  /// The header's inline action row (v11): Message primary, Plans and
  /// Library beside it — each expands its destination in place. Actions
  /// with nothing to open render disabled.
  Widget _actionRow(BuildContext context, ViewerProfileState state) {
    final l10n = context.l10n;
    final sharedIds =
        state.sharedCommunities.map((c) => c.id).toList(growable: false);
    return ProfileActionRow(actions: [
      ProfileActionRowItem(
        icon: Icons.chat_bubble_outline,
        label: l10n.profileActionMessage,
        isPrimary: true,
        onTapRect: _threadMessageAction(context, state),
      ),
      ProfileActionRowItem(
        icon: Icons.calendar_month_outlined,
        label: l10n.profileMetricPlans,
        onTapRect: sharedIds.isEmpty
            ? null
            : (rect) => _expandScreen(
                  rect,
                  SharedCalendarScreen(communityIds: sharedIds),
                  'shared_calendar',
                ),
      ),
      ProfileActionRowItem(
        icon: Icons.inventory_2_outlined,
        label: l10n.profileMetricLibrary,
        onTapRect: state.availableNowItems.isEmpty
            ? null
            : (rect) => _expandScreen(
                  rect,
                  ProfileLibraryScreen(items: state.availableNowItems),
                  'profile_library',
                ),
      ),
    ]);
  }

  /// The one editorial stat moment (v11): Time together, scoped to the
  /// viewer's shared groups, with the tiered equivalence sentence under
  /// it. No person-scoped time drill-down exists, so the block is
  /// inert (no chevron).
  Widget _heroStat(BuildContext context, UserImpactMetrics metrics) {
    final l10n = context.l10n;
    final hours = (metrics.timeBankedMinutes.mean / 60).round();
    return ProfileHeroStat(
      kicker: l10n.profileMetricTimeTogether,
      value: '$hours',
      unit: l10n.profileMetricUnitHours,
      subtext: l10n.profileTimeSubPerson,
      equivalence: timeGivenNarrative(
        metrics.timeBankedMinutes.mean.toDouble(),
        l10n,
      ),
    );
  }

  /// "JR" from "Jordan Reyes" — the monogram initials (first letters of
  /// the first two words).
  static String _initials(String name) {
    final words =
        name.trim().split(RegExp(r'\s+')).where((w) => w.isNotEmpty);
    return words.take(2).map((w) => w[0].toUpperCase()).join();
  }

  /// The standing thread with this person — every card's chat bubble
  /// expands it inline, regardless of what the card is asking, the same
  /// way `experienceContentView` and the other content views expand
  /// their conversation (a morph-reveal panel grown from the tapped
  /// bubble, via `openContentMorphPanel`). Interim routing: the most
  /// recent shared community's conversation (no 1:1 DM surface yet,
  /// #2568). Null (bubble hidden) when nothing is shared.
  ValueChanged<Rect>? _threadMessageAction(
    BuildContext context,
    ViewerProfileState state,
  ) {
    if (state.sharedCommunities.isEmpty) return null;
    final communityId = state.sharedCommunities.first.id;
    return (rect) => _expandScreen(
          rect,
          ProfileConversationPanel(communityId: communityId),
          'profile_conversation',
        );
  }

  /// Morph-opens [screen] in place, grown from [rect] (the shared
  /// content-view expansion used for the conversation and the Library /
  /// Plans drill-downs).
  void _expandScreen(Rect rect, Widget screen, String routeName) {
    openContentMorphPanel(
      context: context,
      ref: ref,
      expandedProvider: userProfileContentExpandedProvider(widget.userId),
      sourceRect: rect,
      routeName: routeName,
      screen: screen,
    );
  }

  /// The shared-groups line (v11): the first shared crew's tile +
  /// "With you in {crew} +N". No honest destination exists for the
  /// full shared-groups list yet, so the row stays plain (no chevron).
  Widget _sharedGroupsRow(BuildContext context, ViewerProfileState state) {
    final l10n = context.l10n;
    final shared = state.sharedCommunities;
    if (shared.isEmpty) return const SizedBox.shrink();
    final faces = shared
        .take(2)
        .map((c) =>
            FaceStackEntry(initial: c.name.isEmpty ? null : c.name[0]))
        .toList(growable: false);
    final overflow = shared.length - 1;
    final title = overflow > 0
        ? '${l10n.profileWithYouIn(shared.first.name)} +$overflow'
        : l10n.profileWithYouIn(shared.first.name);
    return ProfileMemberRow(faces: faces, title: title);
  }

  /// Hero eyebrow: self, cold-start ("New connection · via {crew}"),
  /// or the v11 "What {name} brings" frame.
  String _heroEyebrow(
    BuildContext context,
    ViewerProfileState state,
    ProfileSheetState sheetState,
  ) {
    final l10n = context.l10n;
    if (state.isSelfView) return l10n.profileHeroEyebrowSelf.toUpperCase();
    if (sheetState is ColdStartSheet) {
      final communityId = sheetState.event.communityId;
      for (final c in state.sharedCommunities) {
        if (c.id == communityId) {
          return l10n.profileHeroEyebrowNewConnection(c.name).toUpperCase();
        }
      }
    }
    return l10n
        .profileHeroEyebrowBrings(state.targetName.split(' ').first)
        .toUpperCase();
  }

  /// The quiet ledger under the hero stat (v11): Problems solved ·
  /// Money saved · Library · Plans, each with plain-language subtext
  /// stating the shared-communities scoping. Time lives in the hero
  /// stat above, not here.
  List<MetricTileData> _rows(
    BuildContext context,
    ViewerProfileState state,
    UserImpactMetrics metrics,
  ) {
    final l10n = context.l10n;
    final first = state.targetName.split(' ').first;
    final dollars = metrics.costSavingsUsd.mean.round();
    final money = NumberFormat.simpleCurrency(
      locale: Localizations.localeOf(context).toString(),
      decimalDigits: 0,
    ).format(dollars);
    // TODO(#2568): there is no user-level "problems solved" total on
    // UserImpactMetrics yet. costSavingsCount (the number of cost-saving
    // transactions the user took part in) is the closest existing proxy;
    // replace with a dedicated user problems-solved figure when one is
    // exposed (see docs/issues/2568-directory-and-profiles.md, Decision 2).
    final problems = metrics.costSavingsCount;
    // "Plans" — events the target has hosted; the closest existing
    // figure to a true plans-together count (see buildPayload in
    // server/services/profile/profile.go).
    final plans = state.payload?.eventsCount ?? 0;
    final libraryCount = state.availableNowItems.length;
    final sharedIds =
        state.sharedCommunities.map((c) => c.id).toList(growable: false);
    return [
      MetricTileData(
        value: '$problems',
        label: l10n.profileMetricProblemsSolved,
        subtitle: l10n.profileSubProblemsPerson(first),
      ),
      MetricTileData(
        value: money,
        label: l10n.profileMetricMoneySaved,
        subtitle: l10n.profileSubMoneyPerson,
      ),
      // Library and Plans expand their drill-downs in place, grown from
      // the tapped row's footprint — the same morph-reveal the
      // conversation uses.
      MetricTileData(
        value: '$libraryCount',
        label: l10n.profileMetricLibrary,
        subtitle: l10n.profileSubLibraryPerson(first),
        onTapRect: libraryCount == 0
            ? null
            : (rect) => _expandScreen(
                  rect,
                  ProfileLibraryScreen(items: state.availableNowItems),
                  'profile_library',
                ),
      ),
      MetricTileData(
        value: '$plans',
        label: l10n.profileMetricPlans,
        subtitle: l10n.profileSubPlansPerson,
        onTapRect: sharedIds.isEmpty
            ? null
            : (rect) => _expandScreen(
                  rect,
                  SharedCalendarScreen(communityIds: sharedIds),
                  'shared_calendar',
                ),
      ),
    ];
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
                color: Colors.black.withValues(alpha: 0.34),
                border: Border.all(color: Colors.white.withValues(alpha: 0.18)),
              ),
              child: const Icon(Icons.arrow_back_ios_new,
                  size: 16, color: Colors.white),
            ),
          ),
        ),
      ),
    );
  }

  /// The content-view overflow button (top-right, mirroring the back
  /// chrome) — self view only; it carries the edit-name operation.
  Widget _overflowChrome(BuildContext context, ViewerProfileState state) {
    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.only(right: 12, top: 4),
        child: Align(
          alignment: Alignment.topRight,
          child: Tappable(
            semanticsLabel: context.l10n.a11yProfileMoreOptions,
            onTap: () => _showOverflowMenu(context, state),
            child: Container(
              width: 38,
              height: 38,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: Colors.black.withValues(alpha: 0.34),
                border: Border.all(color: Colors.white.withValues(alpha: 0.18)),
              ),
              child:
                  const Icon(Icons.more_horiz, size: 18, color: Colors.white),
            ),
          ),
        ),
      ),
    );
  }

  void _showOverflowMenu(BuildContext context, ViewerProfileState state) {
    unawaited(showContentOverflowMenu(
      context: context,
      config: ContentOverflowMenuConfig(
        customItems: [
          MenuItemConfig(
            label: context.l10n.profileMenuEditName,
            icon: Icons.edit_outlined,
            onTap: () => unawaited(_editName(state)),
          ),
        ],
      ),
    ));
  }

  /// Renames the viewer's own profile: the rename sheet pops the new
  /// name, SaveUser persists it, and the profile refreshes in place.
  Future<void> _editName(ViewerProfileState state) async {
    final newName = await ProfileRenameSheet.show(
      context,
      initialName: state.targetName,
    );
    if (newName == null || !mounted) return;
    try {
      await ref
          .read(userRepositoryProvider)
          .saveUser(userId: widget.userId, name: newName);
    } catch (e) {
      if (mounted) ToastHelper.showError(context, e.toString());
      return;
    }
    if (!mounted) return;
    await ref.read(viewerProfileProvider(widget.userId).notifier).refresh();
  }
}
