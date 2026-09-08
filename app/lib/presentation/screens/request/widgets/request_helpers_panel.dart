import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart'
    show RequestContributionResponse, RequestGearOffer, RequestNeedResponse;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show TransferState, TransferType;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/viewmodels/needs_scope.dart';
import 'package:ripls/presentation/viewmodels/request_needs_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/content/content_avatar.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel.dart';
import 'package:ripls/presentation/widgets/content/need_quantity_badge.dart';
import 'package:ripls/presentation/widgets/needs/needs_actions.dart';

/// RequestHelpersPanel is the full-screen "Who's helping" surface the
/// request's needs card expands into
/// (docs/cowork/App UXR/whos-in-experience-vs-request).
///
/// Task-first layout: the viewer's helping status, an action row
/// (We need / I'll bring / Share), a coverage banner, then NEEDS (claim-led),
/// BRINGING (free-form contributions), and SHARED · NO REPLY (people the
/// request was shared with who haven't engaged). There is no separate
/// "helping" roster — everyone helping surfaces through their claim or
/// contribution.
///
/// Watches [requestProvider] + [requestNeedsProvider] and drives claims through
/// the needs notifier, so it never calls a repository directly.
class RequestHelpersPanel extends ConsumerStatefulWidget {
  final String requestId;

  /// Opens the share/access sheet (who can see this request) from the `···`.
  final VoidCallback onShowAccess;

  /// Opens the invite/share-link flow from the action row's "Share".
  final VoidCallback onInvite;

  const RequestHelpersPanel({
    super.key,
    required this.requestId,
    required this.onShowAccess,
    required this.onInvite,
  });

  @override
  ConsumerState<RequestHelpersPanel> createState() =>
      _RequestHelpersPanelState();
}

class _RequestHelpersPanelState extends ConsumerState<RequestHelpersPanel> {
  /// Whether the "shared · no reply" list is expanded past its first few rows.
  bool _showAllShared = false;

  String get _requestId => widget.requestId;

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(requestProvider(_requestId));
    final request = state.requestDetails;
    if (request == null) {
      return const ContentMorphPanel(child: SizedBox.shrink());
    }
    final needsState = ref.watch(requestNeedsProvider(_requestId));

    return ContentMorphPanel(
      child: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _header(context),
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 0, 20, 28),
                children: _body(context, state, needsState),
              ),
            ),
          ],
        ),
      ),
    );
  }

  // ── Header ───────────────────────────────────────────────────────────────

  Widget _header(BuildContext context) {
    final l10n = context.l10n;
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 8, 12, 10),
      child: Row(
        children: [
          Expanded(
            child: Semantics(
              header: true,
              child: Text(
                l10n.requestWhosHelpingHeader,
                style: const TextStyle(
                  fontFamily: AppTheme.headingFont,
                  fontSize: 26,
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
            onPressed: widget.onShowAccess,
          ),
          IconAction(
            icon: Icons.close_rounded,
            semanticsLabel: l10n.a11yClose,
            color: AppColors.onContentImage,
            onPressed: () => Navigator.of(context).pop(),
          ),
        ],
      ),
    );
  }

  // ── Body ─────────────────────────────────────────────────────────────────

  List<Widget> _body(
    BuildContext context,
    RequestState state,
    RequestNeedsState needsState,
  ) {
    final l10n = context.l10n;
    final me = state.currentUserId;
    final actions = _needsActions(state);

    final needs = needsState.needs;
    final claims = [
      for (final c in needsState.contributions)
        if (c.fromNeedId.isNotEmpty) c,
    ];
    final bringing = [
      for (final c in needsState.contributions)
        if (c.fromNeedId.isEmpty) c,
    ];
    final myContribs = [
      for (final c in needsState.contributions)
        if (c.contributor.id == me) c,
    ];
    final isHelping = myContribs.isNotEmpty;
    final canClaim = !state.isTerminal && state.communityId != null;

    final claimedCount = needs.where((n) => n.slotsRemaining == 0).length;
    final openCount = needs.length - claimedCount;

    return [
      if (isHelping) ...[
        _statusRow(context, actions),
        _youveGotLine(context, myContribs),
        const SizedBox(height: 18),
      ],
      if (!state.isTerminal) ...[
        _actionRow(context, actions, bringing: isHelping),
        const SizedBox(height: 20),
      ],
      if (needs.isNotEmpty) _banner(context, openCount, needs.length),

      // NEEDS — claim-led.
      if (needs.isNotEmpty) ...[
        _sectionHeader(
          l10n.requestSectionNeeded,
          count: l10n.requestNeedsClaimedCount(claimedCount, needs.length),
        ),
        _progressBar(claimedCount, needs.length),
        for (final n in needs)
          _needRow(context, state, actions, n, claims,
              me: me, canClaim: canClaim),
      ],

      // BRINGING — free-form contributions only.
      if (bringing.isNotEmpty) ...[
        _sectionHeader(
          l10n.requestSectionContributing,
          count: '${bringing.length}',
        ),
        for (final c in bringing) _bringingRow(context, state, actions, c),
      ],

      // SHARED · NO REPLY — people it was shared with who haven't engaged.
      ..._sharedSection(context, state, needsState),
    ];
  }

  // ── Status + you've got ────────────────────────────────────────────────────

  Widget _statusRow(BuildContext context, NeedsActions actions) {
    final l10n = context.l10n;
    return Padding(
      padding: const EdgeInsets.only(top: 6, bottom: 2),
      child: Row(
        children: [
          _checkCircle(36),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              l10n.requestYoureHelping,
              style: const TextStyle(
                color: AppColors.onContentImage,
                fontSize: 17,
                fontWeight: FontWeight.w700,
              ),
            ),
          ),
          Tappable(
            semanticsLabel: l10n.requestTapToChange,
            onTap: () => actions.openPlanTabDispatcher(context, ref),
            child: Text(
              '${l10n.requestTapToChange} ›',
              style: TextStyle(
                color: GlassTokens.textMuted,
                fontSize: 13,
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _youveGotLine(
    BuildContext context,
    List<RequestContributionResponse> myContribs,
  ) {
    final items = [
      for (final c in myContribs)
        if (c.title.isNotEmpty) c.title,
    ].join(', ');
    if (items.isEmpty) return const SizedBox.shrink();
    return Padding(
      padding: const EdgeInsets.only(left: 48, top: 2),
      child: Text(
        context.l10n.requestYouveGot(items),
        style: TextStyle(
          color: GlassTokens.textFaint,
          fontSize: 13,
        ),
      ),
    );
  }

  // ── Action row ─────────────────────────────────────────────────────────────

  Widget _actionRow(
    BuildContext context,
    NeedsActions actions, {
    required bool bringing,
  }) {
    final l10n = context.l10n;
    return Row(
      children: [
        Expanded(
          child: _actionButton(
            label: l10n.pitchingInActionNeed,
            icon: Icons.add_task_rounded,
            semanticsLabel: l10n.a11yReqAddNeed,
            onTap: () => actions.openAddNeed(context, ref),
          ),
        ),
        const SizedBox(width: 9),
        Expanded(
          child: _actionButton(
            label: l10n.pitchingInActionBring,
            icon: Icons.volunteer_activism_rounded,
            semanticsLabel: l10n.a11yRosterAddBringing,
            on: bringing,
            onTap: () => actions.openAddContribution(
              context,
              ref,
              accentColor: AppColors.experienceSageGreen,
            ),
          ),
        ),
        const SizedBox(width: 9),
        Expanded(
          child: _actionButton(
            label: l10n.requestActionShare,
            icon: Icons.ios_share_rounded,
            semanticsLabel: l10n.requestActionShare,
            onTap: widget.onInvite,
          ),
        ),
      ],
    );
  }

  Widget _actionButton({
    required String label,
    required IconData icon,
    required String semanticsLabel,
    required VoidCallback onTap,
    bool on = false,
  }) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      child: Container(
        constraints: const BoxConstraints(minHeight: 56),
        padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 11),
        decoration: BoxDecoration(
          color: on
              ? AppColors.experienceSageGreen.withValues(alpha: 0.14)
              : GlassTokens.fillFaint,
          borderRadius: BorderRadius.circular(13),
          border: Border.all(
            color: on
                ? AppColors.experienceSageGreen.withValues(alpha: 0.6)
                : GlassTokens.borderSoft,
          ),
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              icon,
              color: on
                  ? AppColors.experienceSageGreen
                  : GlassTokens.textSecondary,
              size: 18,
            ),
            const SizedBox(height: 5),
            Text(
              label,
              textAlign: TextAlign.center,
              style: const TextStyle(
                color: AppColors.onContentImage,
                fontSize: 14,
                fontWeight: FontWeight.w600,
              ),
            ),
          ],
        ),
      ),
    );
  }

  // ── Coverage banner ─────────────────────────────────────────────────────────

  Widget _banner(BuildContext context, int open, int total) {
    final l10n = context.l10n;
    final covered = open == 0;
    return Padding(
      padding: const EdgeInsets.only(left: 4, bottom: 20),
      child: Text(
        covered
            ? l10n.pitchingInAllCovered.toUpperCase()
            : l10n.requestStillNeedSomeone(open, total).toUpperCase(),
        style: TextStyle(
          color: covered
              ? GlassTokens.textFaint
              : AppColors.statusWarningOnDark,
          fontSize: 12,
          fontWeight: FontWeight.w600,
          letterSpacing: 1.4,
        ),
      ),
    );
  }

  // ── Sections ────────────────────────────────────────────────────────────────

  Widget _sectionHeader(String label, {String? count}) {
    return Container(
      decoration: BoxDecoration(
        border: Border(
          top: BorderSide(
            color: GlassTokens.hairline,
            width: 0.5,
          ),
        ),
      ),
      padding: const EdgeInsets.only(top: 14, bottom: 4),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.baseline,
        textBaseline: TextBaseline.alphabetic,
        children: [
          Expanded(
            child: Text(
              label.toUpperCase(),
              style: TextStyle(
                color: GlassTokens.textFaint,
                fontSize: 12,
                fontWeight: FontWeight.w600,
                letterSpacing: 1.3,
              ),
            ),
          ),
          if (count != null)
            Text(
              count,
              style: TextStyle(
                color: GlassTokens.textFaint,
                fontSize: 12,
              ),
            ),
        ],
      ),
    );
  }

  Widget _progressBar(int claimed, int total) {
    final remaining = total - claimed;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: SizedBox(
        height: 4,
        child: Row(
          children: [
            if (claimed > 0)
              Expanded(flex: claimed, child: _seg(AppColors.experienceSageGreen)),
            if (claimed > 0 && remaining > 0) const SizedBox(width: 5),
            if (remaining > 0)
              Expanded(
                flex: remaining,
                child: _seg(GlassTokens.fillSubtle),
              ),
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

  /// A need row: claimed-by-you (check + "You" + Undo), claimed-by-other
  /// (claimer avatar + name), or open (ring + "I'll do it" cue). Tapping the
  /// row opens the claim sheet — the same modal the experience roster opens.
  Widget _needRow(
    BuildContext context,
    RequestState state,
    NeedsActions actions,
    RequestNeedResponse need,
    List<RequestContributionResponse> claims, {
    required String? me,
    required bool canClaim,
  }) {
    final claimed = need.slotsRemaining == 0;
    RequestContributionResponse? myClaim;
    RequestContributionResponse? anyClaim;
    for (final c in claims) {
      if (c.fromNeedId != need.id) continue;
      anyClaim ??= c;
      if (c.contributor.id == me) {
        myClaim = c;
        break;
      }
    }

    void openClaim() => actions.openClaimSheet(
          context,
          ref,
          name: need.name,
          note: need.note,
          proposerName: need.proposer.name,
          linkedNeedId: need.id,
          slotsNeeded: need.slots,
        );

    Widget leading;
    Widget? trailing;
    if (myClaim != null) {
      // Releasing a claim lives in the claim sheet (tap the row), not inline.
      leading = _checkCircle(24);
      trailing = _who(context.l10n.needsRowContributorYou);
    } else if (claimed && anyClaim != null) {
      leading = _avatar(anyClaim.contributor, 24);
      trailing = _who(anyClaim.contributor.name.split(' ').first);
    } else {
      leading = _ring();
      if (canClaim) trailing = _claimPill(context);
    }

    // A gear-backed claim carries its live offer (#2702): a Lending/Giving
    // tag for everyone, and the requester's "Offered" accept toggle. The
    // toggle renders as a SIBLING of the tappable row — nested inside it, the
    // row's merged semantics would swallow the chip on Flutter Web.
    final offerClaim = myClaim ?? anyClaim;
    final offer = offerClaim == null
        ? null
        : _liveOffer(state, offerClaim.id);
    Widget? action;
    if (offer != null) {
      if (state.isOwner && !state.isTerminal) {
        // The requester keeps the give/lend distinction visible: the static
        // Giving/Lending tag rides on the row while the Offered/Accepted chip
        // stays the tappable accept toggle (#2724 — a bare "Offered"
        // hides whether the item comes back).
        action = _offerChip(context, state, offer);
        trailing = _prependPill(_offerTypeTag(context, offer), trailing);
      } else {
        trailing = _withStaticOfferTag(context, state, offer, trailing);
      }
    }

    return _row(
      leading: leading,
      label: need.name,
      quantity: need.slots,
      trailing: trailing,
      action: action,
      onTap: canClaim ? openClaim : null,
    );
  }

  Widget _bringingRow(
    BuildContext context,
    RequestState state,
    NeedsActions actions,
    RequestContributionResponse contribution,
  ) {
    final who = contribution.contributor.name.split(' ').first;
    final offer = _liveOffer(state, contribution.id);
    Widget? trailing = who.isEmpty ? null : _who(who);
    Widget? action;
    if (offer != null) {
      if (state.isOwner && !state.isTerminal) {
        // Same as the need rows: the requester sees the Giving/Lending tag
        // alongside the accept toggle (#2724).
        action = _offerChip(context, state, offer);
        trailing = _prependPill(_offerTypeTag(context, offer), trailing);
      } else {
        trailing = _withStaticOfferTag(context, state, offer, trailing);
      }
    }
    return _row(
      leading: _avatar(contribution.contributor, 24),
      label: contribution.title,
      trailing: trailing,
      action: action,
      onTap: () => actions.onContributionTap(context, ref, contribution),
    );
  }

  /// The live gear-backed offer carried by [contributionId] on this request,
  /// or null when none exists (never offered, or the offer is terminal).
  RequestGearOffer? _liveOffer(RequestState state, String contributionId) {
    const live = {
      TransferState.TRANSFER_STATE_INTEREST_EXPRESSED,
      TransferState.TRANSFER_STATE_RECIPIENT_SELECTED,
      TransferState.TRANSFER_STATE_ACTIVE,
    };
    for (final offer in state.requestDetails!.gearOffers) {
      if (offer.contributionId == contributionId &&
          live.contains(offer.state)) {
        return offer;
      }
    }
    return null;
  }

  /// Prepends the static offer chip to a row's trailing widget (non-live
  /// views — display-only, safe inside the row's merged semantics). For the
  /// requester the chip shows the Offered/Accepted state, so the Giving/
  /// Lending tag rides alongside it to keep the give/lend distinction
  /// visible (#2724).
  Widget _withStaticOfferTag(
    BuildContext context,
    RequestState state,
    RequestGearOffer offer,
    Widget? trailing,
  ) {
    Widget chip = _offerChip(context, state, offer);
    if (state.isOwner) {
      chip = _prependPill(_offerTypeTag(context, offer), chip)!;
    }
    return _prependPill(chip, trailing)!;
  }

  /// Prepends [pill] to [trailing] with pill spacing; passes through when
  /// either side is missing.
  Widget? _prependPill(Widget? pill, Widget? trailing) {
    if (pill == null) return trailing;
    if (trailing == null) return pill;
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [pill, const SizedBox(width: 8), trailing],
    );
  }

  /// The static Lending/Giving tag for a gear-backed offer, or null when the
  /// offer carries no transfer type.
  Widget? _offerTypeTag(BuildContext context, RequestGearOffer offer) {
    final l10n = context.l10n;
    if (offer.transferType == TransferType.TRANSFER_TYPE_LOAN) {
      return _offerPill(l10n.requestOfferLendingTag, accepted: false);
    }
    if (offer.transferType == TransferType.TRANSFER_TYPE_GIVEAWAY) {
      return _offerPill(l10n.requestOfferGivingTag, accepted: false);
    }
    return null;
  }

  /// The offer chip on a gear-backed row (#2702). For the requester it is
  /// the accept toggle — "Offered" flips to a sage "Accepted ✓" (tap again
  /// to clear). Everyone else sees a static Lending/Giving tag.
  Widget _offerChip(
    BuildContext context,
    RequestState state,
    RequestGearOffer offer,
  ) {
    final l10n = context.l10n;
    final accepted = offer.hasAcceptedAtUnixSec();
    final isLoan = offer.transferType == TransferType.TRANSFER_TYPE_LOAN;

    final label = accepted
        ? l10n.requestOfferAcceptedChip
        : state.isOwner
            ? l10n.requestOfferedChip
            : isLoan
                ? l10n.requestOfferLendingTag
                : l10n.requestOfferGivingTag;
    final pill = _offerPill(label, accepted: accepted);

    if (!state.isOwner || state.isTerminal) return pill;
    return Toggle(
      semanticsLabel: l10n.a11yRequestOfferAccept,
      selected: accepted,
      inkBorderRadius: BorderRadius.circular(999),
      onTap: () async {
        try {
          await ref
              .read(requestProvider(_requestId).notifier)
              .acceptGearOffer(offer.contributionId);
        } catch (e) {
          if (!context.mounted) return;
          ToastHelper.showError(context, l10n.requestOfferAcceptFailed);
        }
      },
      child: pill,
    );
  }

  /// The shared pill visual for offer chips and Giving/Lending tags.
  Widget _offerPill(String label, {required bool accepted}) {
    final accent = accepted
        ? AppColors.experienceSageGreen
        : AppColors.onContentImage.withValues(alpha: 0.7);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: accepted
            ? AppColors.experienceSageGreen.withValues(alpha: 0.18)
            : Colors.transparent,
        border: Border.all(color: accent.withValues(alpha: 0.7)),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        label,
        style: TextStyle(
          color: accent,
          fontSize: 11,
          fontWeight: FontWeight.w700,
          letterSpacing: 0.4,
        ),
      ),
    );
  }

  List<Widget> _sharedSection(
    BuildContext context,
    RequestState state,
    RequestNeedsState needsState,
  ) {
    final request = state.requestDetails!;
    final engaged = <String>{
      request.requester.id,
      for (final u in request.offerers) u.id,
      for (final c in needsState.contributions) c.contributor.id,
    };
    final named = [
      for (final u in request.conversationParticipants)
        if (!engaged.contains(u.id)) u,
    ];
    // Same sharee base as the read shell's "Shared with N people" card
    // (SharedWithCard.shareeCount): the requester is inside both
    // totalDistinctMemberCount and `engaged`, so this difference equals
    // sharees − engaged sharees — the two surfaces reconcile (#2724).
    var total = request.totalDistinctMemberCount - engaged.length;
    if (total < named.length) total = named.length;
    if (total <= 0) return const [];

    final shown = _showAllShared ? named : named.take(3).toList();
    final remaining = total - shown.length;
    // Tapping "Shared with N more" first reveals the rest of the named people
    // inline; once those are shown, the remaining (un-named community members)
    // opens the full access sheet — mirroring the experience roster's expand.
    final moreOnTap = remaining <= 0
        ? null
        : (!_showAllShared && named.length > shown.length)
            ? () => setState(() => _showAllShared = true)
            : widget.onShowAccess;

    return [
      _sectionHeader('${context.l10n.requestSectionSharedNoReply} · $total'),
      for (final u in shown)
        _row(leading: _avatar(u, 36), label: u.name, dim: true),
      if (remaining > 0)
        _row(
          leading: _plusAvatar(remaining, 36),
          label: context.l10n.requestMoreShared(remaining),
          dim: true,
          onTap: moreOnTap,
        ),
    ];
  }

  // ── Row + atoms ─────────────────────────────────────────────────────────────

  Widget _row({
    required Widget leading,
    required String label,
    int quantity = 1,
    Widget? trailing,
    Widget? action,
    VoidCallback? onTap,
    bool dim = false,
  }) {
    final labelStyle = TextStyle(
      color: dim
          ? GlassTokens.textSecondary
          : AppColors.onContentImage,
      fontSize: 16,
      fontWeight: dim ? FontWeight.w500 : FontWeight.w400,
    );
    final row = Container(
      decoration: BoxDecoration(
        border: Border(
          bottom: BorderSide(
            color: GlassTokens.hairline,
            width: 0.5,
          ),
        ),
      ),
      padding: const EdgeInsets.symmetric(vertical: 12),
      child: Row(
        children: [
          leading,
          const SizedBox(width: 13),
          Expanded(
            child: Row(
              children: [
                Flexible(
                  child: Text(
                    label,
                    overflow: TextOverflow.ellipsis,
                    style: labelStyle,
                  ),
                ),
                if (quantity > 1) ...[
                  const SizedBox(width: 6),
                  NeedQuantityBadge(
                    quantity: quantity,
                    style: labelStyle.copyWith(fontWeight: FontWeight.w700),
                  ),
                ],
              ],
            ),
          ),
          if (trailing != null) ...[const SizedBox(width: 10), trailing],
        ],
      ),
    );
    final tappable = onTap == null
        ? row
        : Tappable(semanticsLabel: label, onTap: onTap, child: row);
    if (action == null) return tappable;
    // Interactive per-row actions (the requester's accept toggle, #2702) sit
    // OUTSIDE the tappable row — nested inside it, the row's merged
    // semantics would swallow them on Flutter Web.
    return Row(
      children: [
        Expanded(child: tappable),
        const SizedBox(width: 10),
        action,
      ],
    );
  }

  Widget _who(String text) => Text(
        text,
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
        style: TextStyle(
          color: GlassTokens.textMuted,
          fontSize: 13,
        ),
      );

  /// Visual "I'll do it" cue on an open need row. Not itself tappable — the
  /// whole row opens the claim sheet.
  Widget _claimPill(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 7),
      decoration: BoxDecoration(
        color: AppColors.experienceSageGreen,
        borderRadius: BorderRadius.circular(22),
      ),
      child: Text(
        context.l10n.requestClaimNeedCta,
        style: const TextStyle(
          color: AppColors.darkBackground,
          fontSize: 13,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }

  Widget _checkCircle(double size) => Container(
        width: size,
        height: size,
        alignment: Alignment.center,
        decoration: const BoxDecoration(
          shape: BoxShape.circle,
          color: AppColors.experienceSageGreen,
        ),
        child: Icon(
          Icons.check_rounded,
          size: size * 0.55,
          color: AppColors.darkBackground,
        ),
      );

  Widget _ring() => Container(
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

  Widget _avatar(User user, double size) => ContentAvatar(
        user: user,
        size: size,
        backgroundColor: _seedColor(user.id),
      );

  Widget _plusAvatar(int n, double size) => Container(
        width: size,
        height: size,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          shape: BoxShape.circle,
          color: GlassTokens.fillSubtle,
        ),
        child: Text(
          '+$n',
          style: TextStyle(
            color: GlassTokens.textMuted,
            fontSize: size * 0.32,
            fontWeight: FontWeight.w600,
          ),
        ),
      );

  Color _seedColor(String seed) {
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

  // ── Actions ──────────────────────────────────────────────────────────────

  NeedsActions _needsActions(RequestState state) {
    final request = state.requestDetails!;
    return NeedsActions(
      scope: NeedsScope.request(
        requestId: widget.requestId,
        currentUserId: state.currentUserId,
        isTerminal: state.isTerminal,
        communityId: state.communityId ?? '',
        isOwner: state.isOwner,
        requestOwnerId: request.requester.id,
        requestName: request.title,
      ),
    );
  }
}
