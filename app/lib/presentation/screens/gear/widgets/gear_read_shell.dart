import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/core/utils/distance_formatter.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GearMetadata;
import 'package:ripls/data/gen/ripls/api/transfer.pbenum.dart'
    show GiveawayPhase;
import 'package:ripls/presentation/screens/gear/widgets/gear_owner_quote_card.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_top_bar.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_who_card.dart';
import 'package:ripls/presentation/viewmodels/content_expanded_provider.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/adaptive/content_column.dart';
import 'package:ripls/presentation/widgets/content/content_headline.dart';
import 'package:ripls/presentation/widgets/content/hero_content_wash.dart';
import 'package:ripls/presentation/widgets/sharing/shared_with_card.dart';

/// GearReadShell is the redesigned read-mode layout for a gear item: a
/// bottom-anchored editorial sheet over the full-bleed hero, mirroring
/// `RequestReadShell` / `ExperienceReadShell` (#2509). It replaces the old
/// tabbed Loan/Details/Chat layout — the description lives in a card that
/// morph-expands into the conversation, the specs and WHERE sit in their own
/// cards, and the WHO'S-USING-IT card opens the borrower roster.
///
/// Read-only: edit mode routes through the edit pane + the edit bar. All
/// mutations flow through callbacks the content view owns (or, for the workflow
/// action button, through the gear view-model via the supplied menu items).
class GearReadShell extends ConsumerWidget {
  final String gearId;
  final Color accentColor;

  /// Expands the discussion card into the full conversation.
  final ValueChanged<Rect> onExpandConversation;

  /// Expands the WHERE card into the full-screen location panel, morphing from
  /// the card's footprint ([Rect]).
  final ValueChanged<Rect> onShowLocation;

  /// Expands the WHO'S-USING card into the full-screen roster/story panel,
  /// morphing from the card's footprint ([Rect]).
  final ValueChanged<Rect> onShowWhosUsing;

  /// Expands the DETAILS mini into the full-screen details panel, morphing from
  /// the card's footprint ([Rect]).
  final ValueChanged<Rect> onShowDetails;

  /// Opens the full-screen who's-using booking calendar (loan gear).
  final VoidCallback onShowCalendar;

  /// Marks the viewer's active loan returned (who-card CTA when the viewer
  /// is the current holder).
  final VoidCallback onMarkReturned;

  /// Opens the access sheet ("who can see this") from the Shared with card.
  final VoidCallback onShowAccess;

  /// Opens the item share sheet from the Shared with card's Invite button.
  /// Null for non-owners, who see the audience but cannot manage sharing.
  final VoidCallback? onInvite;

  /// Opens the owner manage sheet (owners only) from the top-bar overflow.
  final VoidCallback onManage;

  /// Mute/unmute toggle for video heroes, rendered top-right of the sheet body.
  /// Null when the hero is not a video.
  final Widget? muteButton;

  /// Extra bottom inset so the sheet clears the home nav bar in feed context.
  final double bottomNavInset;

  const GearReadShell({
    super.key,
    required this.gearId,
    required this.accentColor,
    required this.onExpandConversation,
    required this.onShowLocation,
    required this.onShowWhosUsing,
    required this.onShowDetails,
    required this.onShowCalendar,
    required this.onMarkReturned,
    required this.onShowAccess,
    required this.onManage,
    this.onInvite,
    this.muteButton,
    this.bottomNavInset = 0,
  });

  bool _isGiveaway(GearState state) =>
      state.gearDetails?.availability == Availability.AVAILABILITY_FOR_GIVEAWAY;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(gearProvider(gearId));
    final gear = state.gearDetails;
    if (gear == null) return const SizedBox.shrink();

    // While the conversation morph panel is open, hide this surface so the
    // panel overlays the still-playing hero alone.
    if (ref.watch(gearContentExpandedProvider(gearId))) {
      return const SizedBox.shrink();
    }

    final isGiveaway = _isGiveaway(state);
    final isTerminal =
        isGiveaway &&
        state.transferContext?.overallPhase ==
            GiveawayPhase.GIVEAWAY_PHASE_COMPLETED;

    return Stack(
      children: [
        if (isTerminal)
          const Positioned.fill(
            child: IgnorePointer(
              child: ColoredBox(color: OverlayTokens.scrimFlat),
            ),
          ),
        Positioned(
          left: 0,
          right: 0,
          bottom: 0,
          // The caption column (#2912): on a desktop-wide window the sheet —
          // and the wash, which sizes to it — holds the reading measure,
          // bottom-centered over the full-bleed hero. No-op at phone widths.
          child: ContentColumn(
            child: HeroContentWash(
              child: SafeArea(
                top: false,
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    Padding(
                      padding: const EdgeInsets.fromLTRB(20, 16, 20, 16),
                      child: Column(
                        mainAxisSize: MainAxisSize.min,
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          if (muteButton != null) ...[
                            Align(
                              alignment: Alignment.centerRight,
                              child: muteButton,
                            ),
                            const SizedBox(height: 8),
                          ],
                          ContentHeadline(title: gear.name),
                          const SizedBox(height: 12),
                          ?_ownerQuote(
                            context,
                            state,
                            isGiveaway: isGiveaway,
                            isCompleted: isTerminal,
                          ),
                          const SizedBox(height: 14),
                          _twoUp(context, state),
                          const SizedBox(height: 12),
                          // Widening the audience is only worth offering while
                          // the item is still going somewhere — once it's given,
                          // inviting someone to it asks nothing of them.
                          if (!isTerminal) ...[
                            SharedWithCard(
                              // Sharee base: the audience minus the owner, so the
                              // count matches the people the facepile shows (#2724).
                              totalPeople: SharedWithCard.shareeCount(
                                gear.totalDistinctMemberCount,
                              ),
                              invitedIndividuals: gear.invitedIndividuals,
                              onTap: onShowAccess,
                              // Any member may reshare the open link (#2630); the
                              // share sheet hides host-only rows for non-owners.
                              onInvite: onInvite,
                            ),
                            const SizedBox(height: 12),
                          ],
                          GearWhoCard(
                            gearId: gearId,
                            accentColor: accentColor,
                            onShowCalendar: onShowCalendar,
                            onExpandInterest: onShowWhosUsing,
                            onMarkReturned: onMarkReturned,
                          ),
                        ],
                      ),
                    ),
                    if (bottomNavInset > 0) SizedBox(height: bottomNavInset),
                  ],
                ),
              ),
            ),
          ),
        ),
        GearTopBar(
          attribution: state.backgroundAttribution,
          showOverflow: state.isOwner,
          onManage: onManage,
        ),
      ],
    );
  }

  // ── Owner quote (description → conversation) ─────────────────────────────

  /// The comment widget: the owner is the sender of the opening message. Shown
  /// whenever there's a description or a conversation to open.
  Widget? _ownerQuote(
    BuildContext context,
    GearState state, {
    required bool isGiveaway,
    required bool isCompleted,
  }) {
    final gear = state.gearDetails!;
    final l10n = context.l10n;
    final hasConversation = gear.conversationId.isNotEmpty;
    final hasDescription = gear.description.trim().isNotEmpty;
    if (!hasConversation && !hasDescription) return null;

    // A completed giveaway stops advertising "Giving this away for free" —
    // the role line flips to the terminal reading under the GIVEAWAY
    // COMPLETED card (#2724).
    final String roleLine;
    if (isGiveaway) {
      roleLine = isCompleted
          ? l10n.gearGiveawayGoneToNewHome
          : l10n.gearOwnerRoleGivingAway;
    } else {
      roleLine = l10n.gearOwnerRoleOwns;
    }

    return GearOwnerQuoteCard(
      owner: gear.owner,
      description: gear.description,
      roleLine: roleLine,
      accentColor: accentColor,
      hasUnread: gear.unreadCount > 0,
      semanticsLabel: l10n.a11yContentOpenConversation,
      // Always tappable: the handler creates the gear conversation on demand
      // (seeding this description as the first comment) when it doesn't exist.
      onTap: onExpandConversation,
    );
  }

  // ── Two-up: WHERE + DETAILS minis ────────────────────────────────────────

  Widget _twoUp(BuildContext context, GearState state) {
    return IntrinsicHeight(
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Expanded(child: _whereMini(context, state)),
          const SizedBox(width: 10),
          Expanded(child: _detailsMini(context, state)),
        ],
      ),
    );
  }

  Widget _whereMini(BuildContext context, GearState state) {
    final l10n = context.l10n;
    final name = state.locationName ?? '';
    final hasLocation = name.isNotEmpty;
    return Builder(
      builder: (cardCtx) => _miniCard(
        semanticsLabel: l10n.contentRowsEditLocation,
        onTap: () => onShowLocation(_rectOf(cardCtx)),
        label: l10n.contentFactWhere,
        showChevron: false,
        value: hasLocation ? name : l10n.contentFactTbd,
        sub: hasLocation
            ? DistanceFormatter.formatWithAway(state.locationDistanceMeters)
            : null,
      ),
    );
  }

  Widget _detailsMini(BuildContext context, GearState state) {
    final l10n = context.l10n;
    final gear = state.gearDetails!;
    final metadata = gear.hasMetadata() ? gear.metadata : null;
    final saved = state.gearStats?.valueSharedUsd ?? 0;
    return Builder(
      builder: (cardCtx) => _miniCard(
        semanticsLabel: l10n.a11yGearViewDetails,
        onTap: () => onShowDetails(_rectOf(cardCtx)),
        label: l10n.gearDetailTitle,
        showChevron: true,
        value: _detailsSummary(context, metadata),
        sub: saved > 0
            ? l10n.gearDetailsSavedAmount('\$${saved.round()}')
            : null,
        subAccent: true,
      ),
    );
  }

  /// The details card's value line: the most identifying facts detection
  /// found, in descending specificity — brand · model, brand, category, then
  /// an approximate value. Falls back to a prompt to open the panel, never to
  /// the card's own label, which reads as an unfilled placeholder beside the
  /// cards that carry real values (#2724). Items with no brand — every food
  /// item — take the later branches.
  String _detailsSummary(BuildContext context, GearMetadata? metadata) {
    final l10n = context.l10n;
    if (metadata == null) return l10n.gearDetailViewAll;
    final parts = [
      metadata.brand.value,
      metadata.model.value,
    ].where((p) => p.isNotEmpty).toList();
    if (parts.isNotEmpty) return parts.join(' · ');
    if (metadata.category.value.isNotEmpty) return metadata.category.value;
    final value = metadata.hasValueEstimate()
        ? metadata.valueEstimate.estimatedValueUsd
        : 0;
    if (value > 0) return '~\$${value.round()}';
    return l10n.gearDetailViewAll;
  }

  Widget _miniCard({
    required String semanticsLabel,
    required VoidCallback onTap,
    required String label,
    required bool showChevron,
    required String value,
    String? sub,
    bool subAccent = false,
  }) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 13),
        decoration: BoxDecoration(
          color: AppColors.darkTextPrimary.withValues(alpha: 0.04),
          borderRadius: BorderRadius.circular(16),
          border: Border.all(color: AppColors.darkBorder),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                // Flexible + ellipsis: in a half-width card at narrow phone
                // widths the label row otherwise overflows by a few pixels
                // (pre-existing; surfaced by the #2912 390px-wide test).
                Flexible(
                  child: Text(
                    label.toUpperCase(),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      color: AppColors.darkTextTertiary,
                      fontSize: 10.5,
                      fontWeight: FontWeight.w700,
                      letterSpacing: 1,
                    ),
                  ),
                ),
                if (showChevron)
                  const Text(
                    '›',
                    style: TextStyle(
                      color: AppColors.darkTextTertiary,
                      fontSize: 14,
                    ),
                  ),
              ],
            ),
            const SizedBox(height: 5),
            // Two lines, matching the shared ContentFactsRow value — one line
            // in a half-width card truncated addresses that fit ("1608 Alta
            // Vista …", #2724).
            Text(
              value,
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                color: AppColors.onContentImage,
                fontSize: 17,
                fontWeight: FontWeight.w600,
                height: 1.2,
              ),
            ),
            if (sub != null && sub.isNotEmpty) ...[
              const SizedBox(height: 3),
              Text(
                sub,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  color: subAccent ? accentColor : AppColors.darkTextSecondary,
                  fontSize: 12,
                  fontWeight: subAccent ? FontWeight.w700 : FontWeight.w400,
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }

  /// The card's on-screen footprint (global coords) so a panel can morph out of
  /// the tapped mini.
  static Rect _rectOf(BuildContext context) {
    final box = context.findRenderObject();
    return box is RenderBox && box.hasSize
        ? box.localToGlobal(Offset.zero) & box.size
        : Rect.zero;
  }
}
