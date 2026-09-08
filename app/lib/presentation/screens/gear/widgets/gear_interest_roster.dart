import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show GearTransferContext, GiveawayPhase, TransferRequest, TransferState;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/transfer/transfer_utils.dart'
    show formatRelativeTimestamp;
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// GearInterestRoster is the single source of truth for a giveaway's "who wants
/// it" signup list, shared by the read-shell card ([maxOthers] = a small cap)
/// and the expanded interest panel ([maxOthers] = null → everyone). Both read
/// the same data (the live transfer context) and render identical rows so the
/// two surfaces can never disagree.
///
/// Rows, in order: the viewer's own status (non-owner), the chosen recipient,
/// then everyone else who's expressed interest. When capped and there are more,
/// a "+N more" row hints at the rest (the full list lives in the panel).
class GearInterestRoster extends ConsumerWidget {
  final String gearId;

  /// Max "other" interested people shown after the viewer's row + the selected
  /// recipient. Null shows everyone (the expanded panel).
  final int? maxOthers;

  /// When set (owner, giveaway still open — the expanded panel), each interested
  /// row gets an inline "Select" button that picks that person as the recipient
  /// directly, called with their (transferId, recipient). Null hides it (the
  /// small read-shell card, or non-owners).
  final void Function(String transferId, User recipient)? onSelectRecipient;

  const GearInterestRoster({
    super.key,
    required this.gearId,
    this.maxOthers,
    this.onSelectRecipient,
  });

  /// Number of distinct people who want this gear: the chosen recipient (if
  /// any) plus everyone else with a pending interest request. The server
  /// keeps the RECIPIENT_SELECTED transfer inside `pendingRequests`, so
  /// `pendingRequests.length + 1` would double-count the recipient (#2724).
  /// This applies the same de-dup the roster rows use, so a header count
  /// computed here always matches the signup rows rendered below.
  static int interestedPeopleCount(GearTransferContext? ctx) {
    final selected = ctx?.hasSelectedRecipient() ?? false
        ? ctx!.selectedRecipient.borrower
        : null;
    var count = selected != null ? 1 : 0;
    for (final r in ctx?.pendingRequests ?? const <TransferRequest>[]) {
      if (r.hasBorrower() &&
          (selected == null || r.borrower.id != selected.id)) {
        count++;
      }
    }
    return count;
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(gearProvider(gearId));
    final l10n = context.l10n;
    final ctx = state.transferContext;
    final isOwner = state.isOwner;
    final currentUserId = state.currentUserId;

    final interested = <TransferRequest>[
      for (final r in ctx?.pendingRequests ?? const <TransferRequest>[])
        if (r.hasBorrower()) r,
    ];
    final selected = ctx?.hasSelectedRecipient() ?? false
        ? ctx!.selectedRecipient.borrower
        : null;
    final isCompleted =
        ctx?.overallPhase == GiveawayPhase.GIVEAWAY_PHASE_COMPLETED;
    final viewerSelected = selected != null && selected.id == currentUserId;
    final viewerExpressed = (ctx?.hasUserTransfer() ?? false) &&
        ctx!.userTransfer.state ==
            TransferState.TRANSFER_STATE_INTEREST_EXPRESSED;

    // Once the giveaway completes, the recipient's badge flips from the
    // in-flight "Selected" to the terminal "Received it" — a stale SELECTED on
    // a finished giveaway reads as an unfinished handoff (#2724).
    final recipientTag = isCompleted
        ? l10n.gearGiveawayReceivedTag
        : l10n.gearWhosUsingSelected;

    final rows = <Widget>[];
    // The viewer's own signup row leads, for a non-owner.
    if (!isOwner) {
      if (viewerSelected) {
        rows.add(_signupRow(
          user: selected,
          title: l10n.gearGiveawayYouSelected,
          tag: recipientTag,
        ));
      } else if (viewerExpressed) {
        // No INTERESTED tag here — the title already says the viewer raised
        // their hand, and title + tag on one row read as a duplicate (#2724).
        rows.add(_signupRow(
          user: _viewerUser(currentUserId, interested),
          title: l10n.gearGiveawayYouInterested,
        ));
      } else if (!isCompleted) {
        rows.add(_signupRow(
          user: null,
          title: l10n.gearGiveawayYouNotRaised,
        ));
      }
    }
    // The chosen recipient (if it isn't the viewer, already shown above).
    if (selected != null && selected.id != currentUserId) {
      rows.add(_signupRow(
        user: selected,
        title: selected.name,
        tag: recipientTag,
      ));
    }
    // Everyone else, capped for the small card.
    final others = [
      for (final r in interested)
        if (r.borrower.id != currentUserId &&
            (selected == null || r.borrower.id != selected.id))
          r,
    ];
    final shown = maxOthers == null ? others : others.take(maxOthers!).toList();
    for (final r in shown) {
      rows.add(_signupRow(
        user: r.borrower,
        title: r.borrower.name,
        tag: onSelectRecipient == null ? l10n.gearInterested : null,
        sub: l10n.gearSignedUpAgo(
            formatRelativeTimestamp(r.requestedAtUnixSec.toInt())),
        trailing: onSelectRecipient == null
            ? null
            : _selectButton(context, r.transferId, r.borrower),
      ));
    }
    if (others.length > shown.length) {
      rows.add(_moreRow(context, others.length - shown.length));
    }

    if (rows.isEmpty) {
      rows.add(_emptyRow(isOwner
          ? l10n.gearWhosUsingEmptyGiveawayOwner
          : l10n.gearWhosUsingEmptyGiveaway));
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: rows,
    );
  }

  /// A signup row: avatar (or a dashed placeholder when the viewer hasn't
  /// raised their hand), a name/heading, an optional tag, and a sub-line.
  Widget _signupRow({
    required User? user,
    required String title,
    String? tag,
    String? sub,
    Widget? trailing,
  }) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Row(
        children: [
          if (user != null)
            UserAvatar(user: user, radius: 17)
          else
            Container(
              width: 34,
              height: 34,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                border: Border.all(
                  color: AppColors.darkTextTertiary,
                  width: 1.6,
                ),
              ),
            ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                Row(
                  children: [
                    Flexible(
                      child: Text(
                        title,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          color: AppColors.onContentImage,
                          fontSize: 14.5,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                    ),
                    if (tag != null) ...[
                      const SizedBox(width: 8),
                      Text(
                        tag.toUpperCase(),
                        style: const TextStyle(
                          color: AppColors.giveawayColorOnDark,
                          fontSize: 9,
                          fontWeight: FontWeight.w800,
                          letterSpacing: 0.5,
                        ),
                      ),
                    ],
                  ],
                ),
                if (sub != null) ...[
                  const SizedBox(height: 1),
                  Text(
                    sub,
                    style: const TextStyle(
                      color: AppColors.darkTextSecondary,
                      fontSize: 12,
                    ),
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

  /// The owner's inline "Select" pill — picks this interested person as the
  /// recipient directly, no modal.
  Widget _selectButton(BuildContext context, String transferId, User user) {
    return Tappable(
      semanticsLabel: context.l10n.a11yGearSelectRecipient(user.name),
      onTap: () => onSelectRecipient!(transferId, user),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 7),
        decoration: BoxDecoration(
          color: AppColors.giveawayColorOnDark,
          borderRadius: BorderRadius.circular(999),
        ),
        child: Text(
          context.l10n.gearInterestSelect,
          style: const TextStyle(
            color: AppColors.darkBackground,
            fontSize: 12.5,
            fontWeight: FontWeight.w700,
          ),
        ),
      ),
    );
  }

  /// The "+N more interested" row shown when the small card caps the list.
  Widget _moreRow(BuildContext context, int count) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Row(
        children: [
          const SizedBox(width: 34 + 12),
          Text(
            context.l10n.gearInterestMore(count),
            style: const TextStyle(
              color: AppColors.darkTextSecondary,
              fontSize: 13,
              fontWeight: FontWeight.w600,
            ),
          ),
        ],
      ),
    );
  }

  Widget _emptyRow(String text) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 6),
      child: Row(
        children: [
          Container(
            width: 8,
            height: 8,
            decoration: const BoxDecoration(
              color: AppColors.experienceSageGreen,
              shape: BoxShape.circle,
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              text,
              style: const TextStyle(
                color: AppColors.darkTextSecondary,
                fontSize: 13.5,
              ),
            ),
          ),
        ],
      ),
    );
  }

  /// Resolves the viewer's own [User] for their signup-row avatar from their
  /// pending request, if present.
  User? _viewerUser(String? currentUserId, List<TransferRequest> interested) {
    if (currentUserId == null) return null;
    for (final r in interested) {
      if (r.borrower.id == currentUserId) return r.borrower;
    }
    return null;
  }
}
