import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/utils/date_time_formatter.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart' show Availability;
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart'
    show GiveawayPhase, TransferRequest, TransferState;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/screens/gear/widgets/gear_interest_roster.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// GearWhoCard is the gear read-shell "Who's using it" (loan) / "Who wants it"
/// (giveaway) widget, matching docs/cowork/App Design/gear-loan-and-giveaway.html.
///
/// A loan shows the availability line and a "Book for borrowing" CTA that opens
/// the booking calendar. A giveaway swaps the calendar for a lightweight signup
/// list — each interested member is a row, the viewer raises their hand with the
/// inline "I'm interested" CTA, and the owner keeps their workflow menu. The
/// primary action lives *inside* the card (the event-RSVP styling) with a short
/// sub-line beneath.
class GearWhoCard extends ConsumerWidget {
  final String gearId;
  final Color accentColor;

  /// Opens the booking calendar (loan).
  final VoidCallback onShowCalendar;

  /// Opens the giveaway interest panel, morphing from the card's footprint.
  final ValueChanged<Rect> onExpandInterest;

  /// Marks the viewer's active loan returned. Used as the card CTA when the
  /// viewer is the current holder — the gear screen is where people look for
  /// it (#2638 follow-up); the Home NEEDS-YOU pill remains the other surface.
  final VoidCallback onMarkReturned;

  const GearWhoCard({
    super.key,
    required this.gearId,
    required this.accentColor,
    required this.onShowCalendar,
    required this.onExpandInterest,
    required this.onMarkReturned,
  });

  // The "See the calendar" pill. These were two private literals, which is why
  // the button kept its green in the sentinel render while everything around it
  // turned garish — it followed no token and no theme change could reach it.
  // It is an action fill on an always-dark surface, which is exactly what the
  // glass action pair describes. Contrast improves from 6.96:1 to 8.06:1.
  static const _rsvp = GlassTokens.primary;
  static const _rsvpInk = GlassTokens.onPrimary;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(gearProvider(gearId));
    final gear = state.gearDetails;
    if (gear == null) return const SizedBox.shrink();
    final isGiveaway =
        gear.availability == Availability.AVAILABILITY_FOR_GIVEAWAY;
    return isGiveaway ? _giveaway(context, ref, state) : _loan(context, state);
  }

  // ── Loan ───────────────────────────────────────────────────────────────────

  Widget _loan(BuildContext context, GearState state) {
    final l10n = context.l10n;
    final gear = state.gearDetails!;
    final holder = gear.hasActiveLoan() ? gear.activeLoan.borrower : null;
    final viewerHasIt = holder != null && holder.id == state.currentUserId;
    final queue = [
      for (final r in state.transferContext?.pendingRequests ?? <TransferRequest>[])
        if (r.hasBorrower()) r,
    ];

    final List<Widget> rows;
    if (holder == null && queue.isEmpty) {
      rows = [
        _availRow(state.isOwner
            ? l10n.gearWhosUsingEmptyAvailableOwner
            : l10n.gearWhosUsingEmptyAvailable),
      ];
    } else {
      rows = [
        if (holder != null)
          _personRow(context, holder, tag: l10n.gearWhosUsingHasItNow),
        for (final r in queue)
          _personRow(context, r.borrower, dateTag: _bookingWindow(r)),
      ];
    }

    // No sub-line here — the booking guidance ("pick your days · the owner
    // confirms") lives on the expanded calendar instead.
    return _card(
      header: l10n.gearSectionWhosUsingIt,
      meta: null,
      // Tapping the card body opens the booking calendar.
      onBodyTap: onShowCalendar,
      semanticsLabel: l10n.a11yGearWhosUsing,
      rows: rows,
      // The current holder's primary action is returning the item — the CTA
      // people look for here (#2638 follow-up). Booking more days stays
      // reachable through the card body (opens the calendar).
      cta: viewerHasIt
          ? _rsvpButton(
              label: l10n.gearMarkReturned,
              semanticsLabel: l10n.a11yGearMarkReturned,
              onTap: state.isSaving ? null : onMarkReturned,
            )
          : _rsvpButton(
              label: state.isOwner
                  ? l10n.gearSeeCalendar
                  : l10n.gearBookForBorrowing,
              semanticsLabel: state.isOwner
                  ? l10n.gearSeeCalendar
                  : l10n.a11yGearBookForBorrowing,
              onTap: onShowCalendar,
            ),
    );
  }

  // ── Giveaway ─────────────────────────────────────────────────────────────────

  Widget _giveaway(BuildContext context, WidgetRef ref, GearState state) {
    final l10n = context.l10n;
    final ctx = state.transferContext;
    final isOwner = state.isOwner;
    final currentUserId = state.currentUserId;

    final selected = ctx?.hasSelectedRecipient() ?? false
        ? ctx!.selectedRecipient.borrower
        : null;
    final isCompleted =
        ctx?.overallPhase == GiveawayPhase.GIVEAWAY_PHASE_COMPLETED;
    final viewerSelected =
        selected != null && selected.id == currentUserId;
    final viewerExpressed = (ctx?.hasUserTransfer() ?? false) &&
        ctx!.userTransfer.state ==
            TransferState.TRANSFER_STATE_INTEREST_EXPRESSED;

    // The server keeps the RECIPIENT_SELECTED transfer inside
    // pendingRequests, so `pendingRequests.length + 1` double-counts the
    // chosen recipient (#2724). Count through the roster's de-dup rule so
    // the header meta always equals the signup rows the roster renders.
    final interestedCount = GearInterestRoster.interestedPeopleCount(ctx);

    // Done: a static "Giveaway completed" card naming the recipient — there's no
    // expanded version to open.
    if (isCompleted) {
      return _completedCard(context, selected);
    }

    return Builder(
      builder: (cardCtx) => Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _card(
            header: l10n.gearSectionWhoWantsIt,
            meta: interestedCount > 0
                ? l10n.gearInterestedCount(interestedCount)
                : null,
            // Tapping the card body opens the interest panel; the CTA below
            // keeps its own action (express / withdraw / arrange pickup).
            onBodyTap: () => onExpandInterest(_rectOf(cardCtx)),
            semanticsLabel: l10n.a11yGearWhoWantsIt,
            // The roster is the single source of truth for the signup list;
            // the small card caps it at two others (the panel shows everyone).
            rows: [GearInterestRoster(gearId: gearId, maxOthers: 2)],
            cta: _giveawayCta(context, ref, state,
                isOwner: isOwner,
                isCompleted: isCompleted,
                viewerSelected: viewerSelected,
                viewerExpressed: viewerExpressed,
                cardCtx: cardCtx),
          ),
          if (!isOwner && !isCompleted && !viewerSelected)
            _ctaSub(l10n.gearGiveawayCtaSub(_ownerFirstName(state))),
        ],
      ),
    );
  }

  Widget _giveawayCta(
    BuildContext context,
    WidgetRef ref,
    GearState state, {
    required bool isOwner,
    required bool isCompleted,
    required bool viewerSelected,
    required bool viewerExpressed,
    required BuildContext cardCtx,
  }) {
    final l10n = context.l10n;
    // The owner manages from the expanded interest panel (tap the card) and the
    // top-bar ··· menu — no inline manage button here.
    if (isOwner || isCompleted) return const SizedBox.shrink();
    if (viewerSelected) {
      return _rsvpButton(
        label: l10n.gearGiveawayArrangePickup,
        semanticsLabel: l10n.a11yGearArrangePickup,
        onTap: () => onExpandInterest(_rectOf(cardCtx)),
      );
    }
    if (viewerExpressed) {
      return _rsvpButton(
        label: l10n.gearInterestWithdraw,
        semanticsLabel: l10n.a11yGearWithdrawInterest,
        filled: false,
        onTap: state.isSaving
            ? null
            : () =>
                ref.read(gearProvider(gearId).notifier).withdrawInterest(),
      );
    }
    return _rsvpButton(
      label: l10n.gearInterestExpress,
      semanticsLabel: l10n.a11yGearExpressInterest,
      onTap: state.isSaving
          ? null
          : () => ref.read(gearProvider(gearId).notifier).expressInterest(),
    );
  }

  // ── Shared chrome ────────────────────────────────────────────────────────────

  /// The card shell. The whole card is a tap target opening the expanded form
  /// ([onCardTap]); the inline CTA inside keeps its own action (nested gesture
  /// detectors resolve a tap on the button to the button, elsewhere to the card).
  Widget _card({
    required String header,
    required String? meta,
    required List<Widget> rows,
    required Widget cta,
    required VoidCallback onBodyTap,
    required String semanticsLabel,
  }) {
    final head = Row(
      children: [
        Expanded(
          child: Text(
            header.toUpperCase(),
            style: const TextStyle(
              color: AppColors.darkTextSecondary,
              fontSize: 11.5,
              fontWeight: FontWeight.w700,
              letterSpacing: 1.2,
            ),
          ),
        ),
        if (meta != null)
          Text(
            meta,
            style: const TextStyle(
              color: AppColors.darkTextTertiary,
              fontSize: 12,
              fontWeight: FontWeight.w600,
            ),
          ),
      ],
    );

    return Container(
      padding: const EdgeInsets.fromLTRB(16, 15, 16, 15),
      decoration: BoxDecoration(
        color: AppColors.darkTextPrimary.withValues(alpha: 0.05),
        borderRadius: BorderRadius.circular(18),
        border: Border.all(color: AppColors.darkBorder),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          // Only the header + rows open the expanded form. The CTA below is a
          // separate tap target (its own action), so it is NOT nested inside
          // this Tappable — nesting made the card tap swallow the button tap.
          Tappable(
            semanticsLabel: semanticsLabel,
            onTap: onBodyTap,
            excludeChildSemantics: false,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              mainAxisSize: MainAxisSize.min,
              children: [
                Padding(
                  padding: const EdgeInsets.only(bottom: 11),
                  child: head,
                ),
                ...rows,
              ],
            ),
          ),
          const SizedBox(height: 13),
          cta,
        ],
      ),
    );
  }

  /// A static "Giveaway completed" card naming the recipient — not tappable
  /// (there is no expanded view once the giveaway is done).
  Widget _completedCard(BuildContext context, User? recipient) {
    final l10n = context.l10n;
    return Container(
      padding: const EdgeInsets.fromLTRB(16, 15, 16, 15),
      decoration: BoxDecoration(
        color: AppColors.darkTextPrimary.withValues(alpha: 0.05),
        borderRadius: BorderRadius.circular(18),
        border: Border.all(color: AppColors.darkBorder),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          Row(
            children: [
              const Icon(Icons.check_circle_rounded,
                  size: 16, color: AppColors.giveawayColorOnDark),
              const SizedBox(width: 7),
              Expanded(
                child: Text(
                  l10n.gearGiveawayCompleted.toUpperCase(),
                  style: const TextStyle(
                    color: AppColors.darkTextSecondary,
                    fontSize: 11.5,
                    fontWeight: FontWeight.w700,
                    letterSpacing: 1.2,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 11),
          if (recipient != null)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 4),
              child: Row(
                children: [
                  UserAvatar(user: recipient, radius: 16),
                  const SizedBox(width: 11),
                  Expanded(
                    child: Text(
                      recipient.name,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        color: AppColors.onContentImage,
                        fontSize: 15,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                  ),
                  _tagPill(l10n.gearGiveawayReceivedTag, AppColors.giveawayColorOnDark),
                ],
              ),
            )
          else
            Text(
              l10n.gearGiveawayGoneToNewHome,
              style: const TextStyle(
                color: AppColors.darkTextSecondary,
                fontSize: 13.5,
              ),
            ),
        ],
      ),
    );
  }

  /// The inline call-to-action, sized to match the experience content view's
  /// RSVP chip ([ContentActionChip]): a full-width pill, roomy but compact.
  Widget _rsvpButton({
    required String label,
    required String semanticsLabel,
    required VoidCallback? onTap,
    bool filled = true,
  }) {
    final fg = filled ? _rsvpInk : AppColors.onContentImage;
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap ?? () {},
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 13, vertical: 7),
        decoration: BoxDecoration(
          color: filled ? _rsvp : GlassTokens.fillFaint,
          borderRadius: BorderRadius.circular(999),
          border:
              filled ? null : Border.all(color: GlassTokens.borderSoft),
        ),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Flexible(
              child: Text(
                label,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  color: fg,
                  fontSize: 12.5,
                  fontWeight: FontWeight.w700,
                  height: 1.2,
                ),
              ),
            ),
            const SizedBox(width: 5),
            Text(
              '›',
              style: TextStyle(
                color: fg,
                fontSize: 13,
                fontWeight: FontWeight.w800,
                height: 1,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _ctaSub(String text) {
    return Padding(
      padding: const EdgeInsets.only(top: 9),
      child: Text(
        text,
        textAlign: TextAlign.center,
        style: const TextStyle(
          color: AppColors.darkTextTertiary,
          fontSize: 11.5,
        ),
      ),
    );
  }

  Widget _availRow(String text) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 2),
      child: Row(
        children: [
          Container(
            width: 9,
            height: 9,
            decoration: BoxDecoration(color: accentColor, shape: BoxShape.circle),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              text,
              style: const TextStyle(
                color: AppColors.onContentImage,
                fontSize: 14.5,
                fontWeight: FontWeight.w500,
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _personRow(BuildContext context, User user,
      {String? tag, String? dateTag}) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 7),
      child: Row(
        children: [
          UserAvatar(user: user, radius: 14),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              user.name,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                color: AppColors.onContentImage,
                fontSize: 14.5,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
          if (tag != null) _tagPill(tag, AppColors.statusWarningOnDark),
          if (dateTag != null) _tagPill(dateTag, AppColors.darkTextSecondary),
        ],
      ),
    );
  }

  /// Compact booking window ("Jul 18", "Jul 18–20") for a dated request, so
  /// multiple bookings by the same borrower read as separate reservations
  /// rather than duplicated rows (#2638). Null for undated interest requests.
  String? _bookingWindow(TransferRequest r) {
    if (!r.hasEstimatedPickupUnixSec()) return null;
    final start = r.estimatedPickupUnixSec.toInt();
    final end =
        r.hasExpectedReturnUnixSec() ? r.expectedReturnUnixSec.toInt() : start;
    return DateTimeFormatter.formatBookingWindow(start, end);
  }

  Widget _tagPill(String label, Color color) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.18),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        label.toUpperCase(),
        style: TextStyle(
          color: color,
          fontSize: 9,
          fontWeight: FontWeight.w700,
          letterSpacing: 0.6,
        ),
      ),
    );
  }

  String _ownerFirstName(GearState state) {
    final name = state.gearDetails?.owner.name ?? '';
    return name.isEmpty ? name : name.split(' ').first;
  }

  static Rect _rectOf(BuildContext context) {
    final box = context.findRenderObject();
    return box is RenderBox && box.hasSize
        ? box.localToGlobal(Offset.zero) & box.size
        : Rect.zero;
  }
}
