import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/core/utils/date_time_formatter.dart';
import 'package:ripls/core/utils/distance_formatter.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart'
    show RequestContributionResponse, RequestGearOffer, RequestNeedResponse;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show TransferState, TransferType;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/request/widgets/request_top_bar.dart';
import 'package:ripls/presentation/viewmodels/content_expanded_provider.dart';
import 'package:ripls/presentation/viewmodels/needs_scope.dart';
import 'package:ripls/presentation/viewmodels/request_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/adaptive/content_column.dart';
import 'package:ripls/presentation/widgets/content/content_avatar.dart';
import 'package:ripls/presentation/widgets/content/content_discussion_card.dart';
import 'package:ripls/presentation/widgets/content/content_edges_card.dart';
import 'package:ripls/presentation/widgets/content/content_facts_row.dart';
import 'package:ripls/presentation/widgets/content/content_headline.dart';
import 'package:ripls/presentation/widgets/content/content_impact_row.dart';
import 'package:ripls/presentation/widgets/content/content_participation_overflow.dart';
import 'package:ripls/presentation/widgets/content/hero_content_wash.dart';
import 'package:ripls/presentation/widgets/content/need_quantity_badge.dart';
import 'package:ripls/presentation/widgets/needs/needs_actions.dart';
import 'package:ripls/presentation/widgets/request/compose/request_compose_sheet.dart';
import 'package:ripls/presentation/widgets/sharing/shared_with_card.dart';

/// RequestReadShell is the redesigned read-mode layout for a request: a
/// bottom-anchored editorial sheet over the full-bleed hero, mirroring
/// `ExperienceReadShell` (docs/issues/2293). It replaces the old tabbed
/// Request/Discuss layout — the discussion lives in a card that morph-expands
/// into the conversation, the WHERE fact card morph-expands into the location
/// panel, and the WHO'S IN card morph-expands into the helpers panel.
///
/// Read-only: edit mode routes through `RequestEditPane` + the edit bar. All
/// mutations flow through callbacks the content view owns, or (offer/withdraw)
/// directly via the callbacks below.
class RequestReadShell extends ConsumerWidget {
  final String requestId;
  final Color accentColor;

  /// Expands the discussion card into the full conversation.
  final ValueChanged<Rect> onExpandConversation;

  /// Expands the WHERE card into the full-screen location panel.
  final ValueChanged<Rect> onShowLocation;

  /// Expands the WHO'S IN card into the full-screen helpers panel.
  final ValueChanged<Rect> onShowHelpers;

  /// Opens the access sheet ("who can see this") from the Shared with card.
  final VoidCallback onShowAccess;

  /// Opens the item share sheet from the Shared with card's Invite button.
  /// Null for non-owners, who see the audience but cannot manage sharing.
  final VoidCallback? onInvite;

  /// Opens the owner manage sheet (owners only) from the top-bar overflow.
  final VoidCallback onManage;

  /// Opens the full impact receipt from the post-fulfillment impact row —
  /// the inline entry point so "View Impact" isn't buried in the manage
  /// menu (#2724). Null leaves the row's pills tappable but adds no
  /// row-level navigation.
  final VoidCallback? onViewImpact;

  /// Mute/unmute toggle for video heroes, rendered top-right of the sheet body.
  /// Null when the hero is not a video.
  final Widget? muteButton;

  /// Extra bottom inset so the sheet clears the home nav bar in feed context.
  final double bottomNavInset;

  const RequestReadShell({
    super.key,
    required this.requestId,
    required this.accentColor,
    required this.onExpandConversation,
    required this.onShowLocation,
    required this.onShowHelpers,
    required this.onShowAccess,
    required this.onManage,
    this.onViewImpact,
    this.onInvite,
    this.muteButton,
    this.bottomNavInset = 0,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(requestProvider(requestId));
    final request = state.requestDetails;
    if (request == null) return const SizedBox.shrink();

    // While a morph-reveal panel is open, hide this surface so the panel
    // overlays the still-playing hero alone.
    if (ref.watch(requestContentExpandedProvider(requestId))) {
      return const SizedBox.shrink();
    }

    final isTerminal = state.isTerminal;

    return Stack(
      children: [
        if (isTerminal)
          const Positioned.fill(
            child: IgnorePointer(
              child: ColoredBox(color: OverlayTokens.scrimFloor),
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
                          if (isTerminal) ...[
                            _terminalStatus(context, state),
                            const SizedBox(height: 10),
                          ],
                          ContentHeadline(title: request.title),
                          ?_discussion(context, state),
                          const SizedBox(height: 14),
                          _facts(context, state),
                          ?_impact(context, state),
                          const SizedBox(height: 12),
                          // Widening the audience is only worth offering while
                          // the request is open — once it closes, inviting
                          // someone to it asks nothing of them.
                          if (!isTerminal) ...[
                            SharedWithCard(
                              // Sharee base: the audience minus the requester, the
                              // same base the helpers panel's "Shared · no reply"
                              // math subtracts from (#2724).
                              totalPeople: SharedWithCard.shareeCount(
                                request.totalDistinctMemberCount,
                              ),
                              invitedIndividuals: request.invitedIndividuals,
                              onTap: onShowAccess,
                              // Any member may reshare the open link (#2630); the
                              // share sheet hides host-only rows for non-owners.
                              onInvite: onInvite,
                            ),
                            const SizedBox(height: 12),
                          ],
                          _whosIn(context, ref, state, isTerminal: isTerminal),
                          ?_resolutionSummary(context, state),
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
        RequestTopBar(
          attribution: state.backgroundAttribution,
          showOverflow: state.showManageOverflow,
          onManage: onManage,
        ),
      ],
    );
  }

  /// The discussion block in place of the description: the description rendered
  /// as the opening comment, the reply count, and a preview of the most recent
  /// comment. Null when there is neither a description nor a conversation.
  Widget? _discussion(BuildContext context, RequestState state) {
    final request = state.requestDetails!;
    final l10n = context.l10n;
    final hasConversation = request.conversationId.isNotEmpty;
    final hasDescription = request.description.trim().isNotEmpty;
    if (!hasConversation && !hasDescription) return null;

    // A non-empty description is seeded as the conversation's first comment,
    // so the raw message count includes it — the rendered reply count must
    // not, or a fresh request reads "1 reply" (#2724).
    final replyCount = hasDescription && request.messageCount > 0
        ? request.messageCount - 1
        : request.messageCount;
    final hasRecent = replyCount > 0 && request.lastMessageText.isNotEmpty;
    String? lastActivityLabel;
    if (hasRecent && request.lastMessageTimeAgo.isNotEmpty) {
      // The server renders compact tokens ("0m", "2h", "64d"); normalize the
      // broken-reading extremes before display (#2724).
      final ago = DateTimeFormatter.normalizeCompactTimeAgo(
        request.lastMessageTimeAgo,
      );
      lastActivityLabel = ago == null
          ? l10n.contentDiscussionJustNow
          : l10n.contentDiscussionLastActivity(ago);
    }
    return Padding(
      padding: const EdgeInsets.only(top: 12),
      child: ContentDiscussionCard(
        authorName: request.requester.name,
        firstComment: request.description,
        replyCount: replyCount,
        replyCountLabel: l10n.contentReplyCount(replyCount),
        startLabel: l10n.contentMessagesStart,
        lastActivityLabel: lastActivityLabel,
        hasUnread: request.unreadCount > 0,
        accentColor: accentColor,
        latestLine: hasRecent && request.lastMessageSender.name.isNotEmpty
            ? l10n.contentDiscussionLatest(
                request.lastMessageSender.name,
                request.lastMessageText,
              )
            : null,
        semanticsLabel: l10n.a11yContentOpenConversation,
        onTap: hasConversation ? onExpandConversation : null,
      ),
    );
  }

  /// The WHERE fact card. A request has no WHEN, so the row carries a single
  /// full-width card (the facts row stretches it). Tapping morph-expands the
  /// location panel.
  Widget _facts(BuildContext context, RequestState state) {
    final l10n = context.l10n;
    final name = state.locationName ?? '';
    final hasLocation = name.isNotEmpty;
    return ContentFactsRow(
      ctaAccentColor: accentColor,
      facts: [
        ContentFactData(
          label: l10n.contentFactWhere,
          value: hasLocation ? name : l10n.contentFactTbd,
          detail: hasLocation
              ? DistanceFormatter.formatWithAway(state.locationDistanceMeters)
              : null,
          onTap: onShowLocation,
          semanticsLabel: l10n.contentRowsEditLocation,
        ),
      ],
    );
  }

  /// The inline impact row, shown once a request is fulfilled. Null otherwise.
  Widget? _impact(BuildContext context, RequestState state) {
    if (!state.isFulfilled) return null;
    final row = ContentImpactRow.build(
      context: context,
      impact: state.requestStats?.impact,
      onViewImpact: onViewImpact,
    );
    if (row == null) return null;
    return Padding(padding: const EdgeInsets.only(top: 12), child: row);
  }

  /// The needs widget (request-needs-widget v3): a single "NEEDS · X of Y
  /// claimed" header, a claim-progress bar, then claimable need rows (tap the
  /// row to claim — no separate button) and, when present, a "BRINGING" group
  /// of free-form contributions. The list scrolls internally past a few items
  /// so the header count and Add CTA stay pinned. Tapping the card chrome
  /// opens the full helpers panel.
  Widget _whosIn(
    BuildContext context,
    WidgetRef ref,
    RequestState state, {
    required bool isTerminal,
  }) {
    final l10n = context.l10n;
    final needsState = ref.watch(requestNeedsProvider(requestId));
    final needs = needsState.needs;
    final claimedCount = needs.where((n) => n.slotsRemaining == 0).length;

    return ContentEdgesCard(
      accentColor: accentColor,
      headerColor: AppColors.darkTextTertiary,
      // Merged header: the screen title already says what this is, so the
      // section is just "NEEDS · X of Y claimed" on the card's header row.
      headerLabel: l10n.requestSectionNeeded,
      // Once the request is fulfilled and every need is claimed, the count
      // flips to the terminal "All N covered" reading (#2724). A request
      // fulfilled with open needs keeps the honest claimed-of-total count.
      headerTrailing: needs.isEmpty
          ? null
          : state.isFulfilled && claimedCount == needs.length
          ? l10n.requestNeedsAllCovered(needs.length)
          : l10n.requestNeedsClaimedCount(claimedCount, needs.length),
      onTap: onShowHelpers,
      semanticsLabel: l10n.a11yReqViewHelpers,
      rows: [_needsBody(context, ref, state)],
      footer: isTerminal ? null : _addCta(context, ref, state),
    );
  }

  Widget _needsBody(BuildContext context, WidgetRef ref, RequestState state) {
    final l10n = context.l10n;
    final needsState = ref.watch(requestNeedsProvider(requestId));
    // NEEDS are the requester's listed needs; BRINGING are free-form
    // contributions (no `fromNeedId`). Claims (contributions with a
    // `fromNeedId`) decorate their need row and never count as needs.
    final needs = needsState.needs;
    final claims = [
      for (final c in needsState.contributions)
        if (c.fromNeedId.isNotEmpty) c,
    ];
    final bringing = [
      for (final c in needsState.contributions)
        if (c.fromNeedId.isEmpty) c,
    ];
    // OFFERING are general hands raised via OfferToFulfill (#2701) — without
    // this group an offer is invisible on the card (it only appeared inside
    // the conversation). Contributors already shown in NEEDS/BRINGING rows
    // are skipped so nobody lists twice.
    final contributorIds = {
      for (final c in needsState.contributions) c.contributor.id,
    };
    final offerers = [
      for (final u in state.requestDetails!.offerers)
        if (!contributorIds.contains(u.id)) u,
    ];
    final claimedCount = needs.where((n) => n.slotsRemaining == 0).length;

    // Keep the collapsed card short: at most [kParticipationCardMaxItems] item
    // rows, then "N more". The whole card opens the expanded panel — nothing
    // claims in place here.
    final layout = collapseNeedsCardRows(
      needs: needs.length,
      bringing: bringing.length,
      offerers: offerers.length,
    );
    final shownNeeds = needs.take(layout.needRows).toList();
    final shownBring = bringing.take(layout.bringRows).toList();
    final shownOffers = offerers.take(layout.offerRows).toList();

    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (needs.isNotEmpty) ...[
          _progressBar(claimedCount, needs.length),
          const SizedBox(height: 4),
        ],
        for (final n in shownNeeds) _needRow(context, state, n, claims),
        if (layout.moreNeeds > 0) _moreRow(context, layout.moreNeeds),
        // The BRINGING / OFFERING headers render whenever their group is
        // non-empty — even with no row budget left — so their total counts
        // keep every entry discoverable (an invisible offer was bug #2701).
        if (bringing.isNotEmpty) ...[
          const SizedBox(height: 12),
          _sectionHeader(l10n.requestSectionContributing, '${bringing.length}'),
          for (final c in shownBring) _bringingRow(context, state, c),
        ],
        if (offerers.isNotEmpty) ...[
          const SizedBox(height: 12),
          _sectionHeader(l10n.requestSectionOffering, '${offerers.length}'),
          for (final u in shownOffers) _offererRow(context, state, u),
        ],
      ],
    );
  }

  /// A muted "N more" row — the overflow indicator for needs hidden past the
  /// item cap. Non-interactive itself; the whole card opens the expanded panel.
  Widget _moreRow(BuildContext context, int count) =>
      contentParticipationMoreRow(context.l10n.requestMoreItems(count));

  /// A section header row: an uppercase label on the left, a count on the right.
  Widget _sectionHeader(String label, String count) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.baseline,
        textBaseline: TextBaseline.alphabetic,
        children: [
          Expanded(
            child: Text(
              label.toUpperCase(),
              style: const TextStyle(
                color: AppColors.darkTextTertiary,
                fontSize: 11,
                fontWeight: FontWeight.w600,
                letterSpacing: 1.2,
              ),
            ),
          ),
          Text(
            count,
            style: const TextStyle(
              color: AppColors.darkTextSecondary,
              fontSize: 12,
            ),
          ),
        ],
      ),
    );
  }

  /// The claim-progress momentum bar: a sage segment sized to the claimed
  /// count, a muted segment to the remainder.
  Widget _progressBar(int claimed, int total) {
    final remaining = total - claimed;
    return Padding(
      padding: const EdgeInsets.only(bottom: 2),
      child: SizedBox(
        height: 4,
        child: Row(
          children: [
            if (claimed > 0)
              Expanded(
                flex: claimed,
                child: _seg(AppColors.experienceSageGreen),
              ),
            if (claimed > 0 && remaining > 0) const SizedBox(width: 5),
            if (remaining > 0)
              Expanded(flex: remaining, child: _seg(GlassTokens.fillSubtle)),
          ],
        ),
      ),
    );
  }

  Widget _seg(Color color) => DecoratedBox(
    decoration: BoxDecoration(
      color: color,
      borderRadius: BorderRadius.circular(2),
    ),
  );

  /// A need row: leading ring (open) or sage check (claimed), the need name,
  /// and a trailing label — "You" for the viewer's own claim or the claimer's
  /// first name. Display-only; tapping anywhere opens the expanded panel.
  Widget _needRow(
    BuildContext context,
    RequestState state,
    RequestNeedResponse need,
    List<RequestContributionResponse> claims,
  ) {
    final l10n = context.l10n;
    final claimed = need.slotsRemaining == 0;
    RequestContributionResponse? myClaim;
    RequestContributionResponse? anyClaim;
    for (final c in claims) {
      if (c.fromNeedId != need.id) continue;
      anyClaim ??= c;
      if (c.contributor.id == state.currentUserId) {
        myClaim = c;
        break;
      }
    }

    Widget? trailing;
    if (myClaim != null) {
      trailing = _whoText(l10n.needsRowContributorYou);
    } else if (claimed && anyClaim != null) {
      trailing = _whoText(anyClaim.contributor.name.split(' ').first);
    }
    final claim = myClaim ?? anyClaim;
    trailing = _withOfferTag(context, state, claim, trailing);
    if (state.isFulfilled &&
        claimed &&
        _deliveredOfferFor(state, claim) == null) {
      // A satisfied need on a fulfilled request with no gear transfer behind
      // it — the client can't tell how it changed hands, so it gets the
      // neutral terminal tag instead of a stale ACCEPTED/no label (#2724).
      trailing = _withLeadingTag(
        _rowTag(l10n.requestNeedDoneTag, emphasized: true),
        trailing,
      );
    }

    return _itemRow(
      leading: (claimed || myClaim != null) ? _checkCircle() : _hollowCircle(),
      name: need.name,
      quantity: need.slots,
      trailing: trailing,
    );
  }

  /// Appends the Lending/Giving tag when [contribution] carries a live
  /// gear-backed offer (#2702). Once the request is fulfilled, the live
  /// negotiation tag gives way to the resolved outcome instead — "HANDED OFF"
  /// for a delivered loan, "GIVEN" for a delivered giveaway (#2724).
  /// Display-only here — accepting lives on the expanded helpers panel.
  Widget? _withOfferTag(
    BuildContext context,
    RequestState state,
    RequestContributionResponse? contribution,
    Widget? trailing,
  ) {
    if (contribution == null) return trailing;
    final l10n = context.l10n;
    if (state.isFulfilled) {
      final delivered = _deliveredOfferFor(state, contribution);
      if (delivered == null) return trailing;
      final label = delivered.transferType == TransferType.TRANSFER_TYPE_LOAN
          ? l10n.requestNeedHandedOffTag
          : l10n.requestNeedGivenTag;
      return _withLeadingTag(_rowTag(label, emphasized: true), trailing);
    }
    const live = {
      TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
      TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
      TransferState.TRANSFER_STATE_ACTIVE,
    };
    for (final offer in state.requestDetails!.gearOffers) {
      if (offer.contributionId != contribution.id ||
          !live.contains(offer.state)) {
        continue;
      }
      final accepted = offer.hasAcceptedAtUnixSec();
      final label = accepted
          ? l10n.requestOfferAcceptedChip
          : offer.transferType == TransferType.TRANSFER_TYPE_LOAN
          ? l10n.requestOfferLendingTag
          : l10n.requestOfferGivingTag;
      return _withLeadingTag(_rowTag(label, emphasized: accepted), trailing);
    }
    return trailing;
  }

  /// The delivered gear transfer behind [contribution] — ACTIVE (a loan that
  /// was handed over) or COMPLETED — when one exists. Null for gear-less
  /// claims and for offers that never reached handoff.
  RequestGearOffer? _deliveredOfferFor(
    RequestState state,
    RequestContributionResponse? contribution,
  ) {
    if (contribution == null) return null;
    const delivered = {
      TransferState.TRANSFER_STATE_ACTIVE,
      TransferState.TRANSFER_STATE_COMPLETED,
    };
    for (final offer in state.requestDetails!.gearOffers) {
      if (offer.contributionId == contribution.id &&
          delivered.contains(offer.state)) {
        return offer;
      }
    }
    return null;
  }

  /// A tiny uppercase tag for a need/bringing row: sage when [emphasized]
  /// (accepted or resolved), muted otherwise.
  Widget _rowTag(String label, {required bool emphasized}) {
    final color = emphasized
        ? AppColors.experienceSageGreen
        : GlassTokens.textMuted;
    return Text(
      label.toUpperCase(),
      style: TextStyle(
        color: color,
        fontSize: 10,
        fontWeight: FontWeight.w700,
        letterSpacing: 0.8,
      ),
    );
  }

  /// Prepends [tag] before an optional [trailing] widget on an item row.
  Widget _withLeadingTag(Widget tag, Widget? trailing) {
    if (trailing == null) return tag;
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [tag, const SizedBox(width: 8), trailing],
    );
  }

  /// An "offering to help" row: the offerer's avatar and name — "You" when it
  /// is the viewer, which doubles as the helper's persistent confirmation that
  /// their offer landed (#2701).
  Widget _offererRow(BuildContext context, RequestState state, User user) {
    final isViewer = user.id == state.currentUserId;
    return _itemRow(
      leading: ContentAvatar(
        user: user,
        size: 24,
        backgroundColor: _avatarColor(user.id),
      ),
      name: user.name,
      trailing: isViewer ? _whoText(context.l10n.needsRowContributorYou) : null,
    );
  }

  /// A "bringing" row: a seeded avatar, the item, and the contributor's name.
  Widget _bringingRow(
    BuildContext context,
    RequestState state,
    RequestContributionResponse contribution,
  ) {
    final who = contribution.contributor.name.split(' ').first;
    return _itemRow(
      leading: ContentAvatar(
        user: contribution.contributor,
        size: 24,
        backgroundColor: _avatarColor(contribution.contributor.id),
      ),
      name: contribution.title,
      trailing: _withOfferTag(
        context,
        state,
        contribution,
        who.isEmpty ? null : _whoText(who),
      ),
    );
  }

  /// One needs/bringing row: a 24px leading slot, the item, optional trailing.
  /// Display-only (tight ~8px padding) — the whole card is the tap target.
  Widget _itemRow({
    required Widget leading,
    required String name,
    int quantity = 1,
    Widget? trailing,
  }) {
    const nameStyle = TextStyle(
      color: AppColors.onContentImage,
      fontSize: 16,
      height: 1.3,
    );
    return Container(
      decoration: BoxDecoration(
        border: Border(
          bottom: BorderSide(color: GlassTokens.hairline, width: 0.5),
        ),
      ),
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Row(
        children: [
          leading,
          const SizedBox(width: 12),
          Expanded(
            child: Row(
              children: [
                Flexible(
                  child: Text(
                    name,
                    overflow: TextOverflow.ellipsis,
                    style: nameStyle,
                  ),
                ),
                if (quantity > 1) ...[
                  const SizedBox(width: 6),
                  NeedQuantityBadge(
                    quantity: quantity,
                    style: nameStyle.copyWith(fontWeight: FontWeight.w700),
                  ),
                ],
              ],
            ),
          ),
          if (trailing != null) ...[const SizedBox(width: 10), trailing],
        ],
      ),
    );
  }

  Widget _whoText(String text) => Text(
    text,
    maxLines: 1,
    overflow: TextOverflow.ellipsis,
    style: TextStyle(color: GlassTokens.textMuted, fontSize: 13),
  );

  /// The single bottom CTA: add a need or log a contribution.
  Widget _addCta(BuildContext context, WidgetRef ref, RequestState state) {
    return Padding(
      padding: const EdgeInsets.only(top: 16),
      child: Tappable(
        semanticsLabel: context.l10n.requestAddNeedOrBringingCta,
        onTap: () => _add(context, ref, state),
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 12, horizontal: 12),
          decoration: BoxDecoration(
            border: Border.all(color: GlassTokens.border),
            borderRadius: BorderRadius.circular(24),
          ),
          child: Text(
            '${context.l10n.requestAddNeedOrBringingCta} ›',
            textAlign: TextAlign.center,
            style: TextStyle(
              color: GlassTokens.textSecondary,
              fontSize: 14,
              fontWeight: FontWeight.w500,
            ),
          ),
        ),
      ),
    );
  }

  /// A stable, seeded fill color for an avatar (distinct per contributor).
  Color _avatarColor(String seed) {
    const palette = [
      Color(0xFF5A7D9C),
      Color(0xFFA8704F),
      Color(0xFF6B8F71),
      Color(0xFF8A6D9C),
      Color(0xFFB08A4F),
      Color(0xFF4F8A8B),
    ];
    var h = 0;
    for (final code in seed.codeUnits) {
      h = (h * 31 + code) & 0x7fffffff;
    }
    return palette[seed.isEmpty ? 0 : h % palette.length];
  }

  Future<void> _add(
    BuildContext context,
    WidgetRef ref,
    RequestState state,
  ) async {
    final needsState = ref.read(requestNeedsProvider(requestId));
    final isTbd = needsState.needs.isEmpty && needsState.contributions.isEmpty;
    if (state.isOwner && isTbd) {
      await RequestComposeSheet.show(
        context,
        requestId: requestId,
        communityId: state.communityId ?? '',
      );
      return;
    }
    await NeedsActions(
      scope: NeedsScope.request(
        requestId: requestId,
        currentUserId: state.currentUserId,
        isTerminal: state.isTerminal,
        communityId: state.communityId ?? '',
        isOwner: state.isOwner,
        requestOwnerId: state.requestDetails!.requester.id,
        requestName: state.requestDetails!.title,
      ),
    ).openSingleAddPicker(context, ref);
  }

  Widget _checkCircle() => Container(
    width: 24,
    height: 24,
    alignment: Alignment.center,
    decoration: const BoxDecoration(
      shape: BoxShape.circle,
      color: AppColors.experienceSageGreen,
    ),
    child: const Icon(
      Icons.check_rounded,
      size: 14,
      color: AppColors.darkBackground,
    ),
  );

  Widget _hollowCircle() => Container(
    width: 24,
    height: 24,
    decoration: BoxDecoration(
      shape: BoxShape.circle,
      border: Border.all(
        color: AppColors.onContentImage.withValues(alpha: 0.4),
        width: 1.5,
      ),
    ),
  );

  /// Status pill shown above the headline for terminal requests: "FULFILLED ·
  /// {ago}" or "CANCELLED" — the same edge-status treatment terminal events
  /// get in ExperienceReadShell (#2724).
  Widget _terminalStatus(BuildContext context, RequestState state) {
    final l10n = context.l10n;
    final isCancelled = state.isCancelled;
    final color = isCancelled
        ? AppColors.darkTextTertiary
        : AppColors.statusInfoOnDark;
    final String label;
    if (isCancelled) {
      label = l10n.commonCancelled.toUpperCase();
    } else {
      final fulfilledAt = state.requestDetails!.fulfilledAtUnixSec.toInt();
      final ago = fulfilledAt > 0
          ? DateTimeFormatter.formatTimeAgo(fulfilledAt)
          : null;
      final fulfilled = l10n.requestStatusFulfilled.toUpperCase();
      label = ago != null ? '$fulfilled · $ago' : fulfilled;
    }
    return Align(
      alignment: Alignment.centerLeft,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
        decoration: BoxDecoration(
          color: color.withValues(alpha: 0.18),
          borderRadius: BorderRadius.circular(999),
        ),
        child: Text(
          label,
          style: TextStyle(
            color: color,
            fontSize: 10,
            fontWeight: FontWeight.w700,
            letterSpacing: 1,
          ),
        ),
      ),
    );
  }

  /// The owner's resolution note rendered once a request is fulfilled. Null
  /// when not fulfilled or there is no summary.
  Widget? _resolutionSummary(BuildContext context, RequestState state) {
    if (!state.isFulfilled) return null;
    final summary = state.resolutionSummary?.trim();
    if (summary == null || summary.isEmpty) return null;
    return Padding(
      padding: const EdgeInsets.only(top: 12),
      child: Text(
        summary,
        style: const TextStyle(
          color: AppColors.onContentImage,
          fontSize: 13.5,
          height: 1.5,
        ),
      ),
    );
  }
}

/// How the collapsed needs card folds its three groups (needs, bringing,
/// offering) into at most [maxItems] item rows.
///
/// The "{n} more ›" overflow row counts hidden *needs* only — total needs
/// minus visible need rows — so every viewer of the same needs list reads the
/// same number; hidden bringing/offering entries never inflate it, which used
/// to make the count differ by viewer (#2724). Those groups stay discoverable
/// instead through their section headers, which always render with each
/// group's total count.
({int needRows, int bringRows, int offerRows, int moreNeeds})
collapseNeedsCardRows({
  required int needs,
  required int bringing,
  required int offerers,
  int maxItems = kParticipationCardMaxItems,
}) {
  final needRows = needs > maxItems ? maxItems : needs;
  var budget = maxItems - needRows;
  final bringRows = bringing > budget ? budget : bringing;
  budget -= bringRows;
  final offerRows = offerers > budget ? budget : offerers;
  return (
    needRows: needRows,
    bringRows: bringRows,
    offerRows: offerRows,
    moreNeeds: needs - needRows,
  );
}
