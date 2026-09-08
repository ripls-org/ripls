import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/home/face_stack.dart';
import 'package:ripls/presentation/widgets/home/home_decision_copy.dart';
import 'package:ripls/presentation/widgets/home/home_media_thumb.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// NeedsYouGroupRow is the editorial Needs-you row used under each kind group:
/// a 48px thumbnail/avatar, a serif title (2-line), a muted subtitle (bold
/// first name), and a trailing action chip whose label resolves from the
/// decision's kind and typed action (e.g. "Mark picked up" — see
/// [homeDecisionAcceptLabel]). Tapping the chip runs the typed action
/// ([onAction] — the same `runDecisionAction` the inbox root uses, which opens
/// the associated modal or screen); tapping the row body opens the item
/// ([onTap]).
class NeedsYouGroupRow extends StatelessWidget {
  final HomeDecision decision;
  final VoidCallback onTap;

  /// Runs when the action chip is tapped — the decision's typed action.
  final VoidCallback onAction;

  final bool showTopBorder;

  const NeedsYouGroupRow({
    super.key,
    required this.decision,
    required this.onTap,
    required this.onAction,
    this.showTopBorder = true,
  });

  HomeDecisionKind get _kind => decision.kind;
  bool get _isThanks => _kind == HomeDecisionKind.HOME_DECISION_KIND_ASK_CLAIM;
  bool get _isReply => _kind == HomeDecisionKind.HOME_DECISION_KIND_REPLY;
  bool get _isMarkDone =>
      _kind == HomeDecisionKind.HOME_DECISION_KIND_MARK_DONE;

  @override
  Widget build(BuildContext context) {
    final headline = homeDecisionHeadline(context.l10n, decision);
    final acceptLabel = homeDecisionAcceptLabel(context.l10n, decision);
    // The body (thumb + text) and the completion circle are separate tap
    // targets so the circle keeps its own semantics — wrapping the whole row in
    // one Tappable would exclude the circle's label from the semantics tree.
    return Container(
      padding: const EdgeInsets.symmetric(vertical: 14),
      decoration: BoxDecoration(
        border: showTopBorder
            ? Border(top: BorderSide(color: AppColors.border(context)))
            : null,
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          Expanded(
            child: Tappable(
              semanticsLabel: headline,
              onTap: onTap,
              inkBorderRadius: BorderRadius.zero,
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.center,
                children: [
                  _buildThumb(context),
                  const SizedBox(width: 15),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          headline,
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                          style: TextStyle(
                            fontFamily: AppTheme.headingFont,
                            fontSize: 16,
                            fontWeight: FontWeight.w500,
                            height: 1.3,
                            color: AppColors.textPrimary(context),
                          ),
                        ),
                        const SizedBox(height: 5),
                        _buildSubtitle(context),
                      ],
                    ),
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(width: 12),
          if (acceptLabel.isNotEmpty) _buildActionChip(context, acceptLabel),
        ],
      ),
    );
  }

  // ── Thumbnail: subject photo for thanks/done; avatar (+ object overlay) for
  // pickup/lend/reply — mirrors the inbox NeedsYou rows. ──
  Widget _buildThumb(BuildContext context) {
    if (_isThanks || _isMarkDone) {
      return HomeMediaThumb(mediaId: decision.thumbnailMediaId, size: 48);
    }
    final avatar = decision.hasCounterparty()
        ? UserAvatar(
            user: User(
              id: decision.counterparty.userId,
              name: decision.counterparty.displayName,
              mediaId: decision.counterparty.mediaId,
            ),
            radius: 24,
          )
        : HomeMediaThumb(mediaId: decision.thumbnailMediaId, size: 48);
    if (decision.objectMediaId.isEmpty) return avatar;
    return SizedBox(
      width: 48,
      height: 48,
      child: Stack(
        clipBehavior: Clip.none,
        children: [
          avatar,
          Positioned(
            bottom: -4,
            right: -4,
            child: Container(
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(7),
                border: Border.all(
                    color: AppColors.cardBackground(context), width: 2),
              ),
              child: HomeMediaThumb(mediaId: decision.objectMediaId, size: 22),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildSubtitle(BuildContext context) {
    final muted = TextStyle(
      fontSize: 12,
      height: 1.3,
      color: AppColors.textSecondary(context),
    );
    final emphasis = muted.copyWith(
      color: AppColors.primary(context),
      fontWeight: FontWeight.w600,
    );

    if (_isThanks) {
      return Row(
        children: [
          FaceStack(people: decision.helpers, size: 18),
          if (decision.helpers.isNotEmpty) const SizedBox(width: 6),
          Flexible(
            child: Text(
              context.l10n.homePitchedIn(_helperNames()),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: muted,
            ),
          ),
        ],
      );
    }
    if (_isMarkDone) {
      return Text(homeDecisionWhy(context.l10n, decision),
          maxLines: 1, overflow: TextOverflow.ellipsis, style: muted);
    }
    final who = decision.hasCounterparty()
        ? decision.counterparty.displayName.split(' ').first
        : '';
    final whoLabel =
        _isReply && decision.itemType == DailyItemType.DAILY_ITEM_TYPE_REQUEST
            ? context.l10n.homeReplyCommented(who)
            : who;
    // Message previews are verbatim user text and render quoted; composed
    // status copy ("ready to pick up") does not.
    final why = homeDecisionWhy(context.l10n, decision);
    final whyDisplay = homeDecisionWhyIsQuote(decision) ? '"$why"' : why;
    return Text.rich(
      TextSpan(
        style: muted,
        children: [
          if (whoLabel.isNotEmpty) TextSpan(text: whoLabel, style: emphasis),
          if (whoLabel.isNotEmpty && why.isNotEmpty)
            const TextSpan(text: ' · '),
          if (why.isNotEmpty) TextSpan(text: whyDisplay),
        ],
      ),
      maxLines: 1,
      overflow: TextOverflow.ellipsis,
    );
  }

  String _helperNames() {
    final names =
        decision.helpers.map((h) => h.displayName.split(' ').first).toList();
    if (names.isEmpty) return '';
    if (names.length == 1) return names.first;
    if (names.length == 2) return '${names[0]} & ${names[1]}';
    return '${names.sublist(0, names.length - 1).join(', ')} & ${names.last}';
  }

  /// Trailing action chip carrying the resolved accept label (e.g. "Mark
  /// picked up"). Mirrors the inbox-root pill exactly; tapping runs the typed
  /// action, which opens the associated modal or screen.
  Widget _buildActionChip(BuildContext context, String acceptLabel) {
    return Tappable(
      semanticsLabel: acceptLabel,
      onTap: onAction,
      inkBorderRadius: BorderRadius.circular(16),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 11, vertical: 5),
        decoration: BoxDecoration(
          color: AppColors.primary(context).withAlpha(28),
          borderRadius: BorderRadius.circular(16),
        ),
        child: Text(
          acceptLabel,
          style: TextStyle(
            fontSize: 11.5,
            fontWeight: FontWeight.w600,
            color: AppColors.primary(context),
          ),
        ),
      ),
    );
  }
}
