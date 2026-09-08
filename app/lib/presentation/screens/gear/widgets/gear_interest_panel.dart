import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart' show TransferRequest;
import 'package:ripls/data/gen/ripls/api/transfer.pbenum.dart'
    show GiveawayPhase, TransferState;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/gear_repository.dart' show GearBooking;
import 'package:ripls/presentation/screens/gear/widgets/gear_handoff_section.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_interest_roster.dart';
import 'package:ripls/presentation/viewmodels/gear_booking_view_model.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel.dart';
import 'package:ripls/presentation/widgets/sharing/item_share_sheet.dart';
import 'package:ripls/presentation/widgets/transfer/transfer_utils.dart'
    show formatRelativeTimestamp;
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/services/providers.dart';

/// GearInterestPanel is the full-screen "Who's interested" sign-up sheet a
/// giveaway's read-shell card morph-expands into. Unlike a loan (which uses the
/// booking calendar), a giveaway has no schedule: anyone can express interest,
/// the owner picks one recipient, and once chosen the owner and recipient
/// coordinate the pickup hand-off here (no drop-off — the item is theirs to
/// keep).
///
/// The pickup hand-off reuses [GearHandoffSection] backed by the selected
/// giveaway transfer (surfaced through [gearBookingProvider]); the server lets
/// either the owner or the recipient set it.
class GearInterestPanel extends ConsumerStatefulWidget {
  final String gearId;

  /// Community used to load the selected transfer's hand-off booking. Null when
  /// the gear isn't community-scoped yet (then no hand-off is shown).
  final String? communityId;

  /// Opens the gear conversation (the recipient "Message" button). Null hides it.
  final VoidCallback? onOpenConversation;

  const GearInterestPanel({
    super.key,
    required this.gearId,
    required this.communityId,
    this.onOpenConversation,
  });

  @override
  ConsumerState<GearInterestPanel> createState() => _GearInterestPanelState();
}

class _GearInterestPanelState extends ConsumerState<GearInterestPanel> {
  static const _mint = Color(0xFFA7C59E);

  @override
  void initState() {
    super.initState();
    final communityId = widget.communityId;
    if (communityId != null) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted) return;
        ref
            .read(gearBookingProvider(widget.gearId).notifier)
            .initialize(communityId: communityId);
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(gearProvider(widget.gearId));
    if (state.gearDetails == null) {
      return const ContentMorphPanel(child: SizedBox.shrink());
    }
    return ContentMorphPanel(
      child: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _header(context, state),
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 0, 20, 28),
                children: _body(context, state),
              ),
            ),
            ?_footer(context, state),
          ],
        ),
      ),
    );
  }

  Widget _header(BuildContext context, GearState state) {
    final l10n = context.l10n;
    final listedUnix = _listedUnix(state);
    final showSubtitle = state.isOwner && listedUnix != null;
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 8, 12, 8),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                Semantics(
                  header: true,
                  child: Text(
                    l10n.gearSectionWhoWantsIt,
                    style: const TextStyle(
                      fontFamily: AppTheme.headingFont,
                      fontSize: 24,
                      fontWeight: FontWeight.w600,
                      color: AppColors.onContentImage,
                    ),
                  ),
                ),
                if (showSubtitle) ...[
                  const SizedBox(height: 4),
                  Text(
                    l10n.gearReachListedAgo(formatRelativeTimestamp(listedUnix)),
                    style: const TextStyle(
                      fontSize: 12.5,
                      fontWeight: FontWeight.w600,
                      color: AppColors.darkTextSecondary,
                    ),
                  ),
                ],
              ],
            ),
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

  /// When the giveaway was listed (shared to the community), else when created.
  int? _listedUnix(GearState state) {
    final gear = state.gearDetails;
    if (gear == null) return null;
    if (gear.sharedAtUnixSec > 0) return gear.sharedAtUnixSec.toInt();
    if (gear.createdAtUnixSec > 0) return gear.createdAtUnixSec.toInt();
    return null;
  }

  List<Widget> _body(BuildContext context, GearState state) {
    final ctx = state.transferContext;
    final isOwner = state.isOwner;
    final selected = ctx?.hasSelectedRecipient() ?? false
        ? ctx!.selectedRecipient.borrower
        : null;
    final isSelectedPhase =
        ctx?.overallPhase == GiveawayPhase.GIVEAWAY_PHASE_RECIPIENT_SELECTED;
    final isCompleted =
        ctx?.overallPhase == GiveawayPhase.GIVEAWAY_PHASE_COMPLETED;
    // Owner, still open → each interested row gets an inline "Give to" button
    // (no modal); the selection happens right here in the panel.
    final canSelect = isOwner && selected == null && !isCompleted;
    final interestedCount = ctx?.pendingRequests.length ?? 0;

    // Done → a static completed summary naming the recipient. Without this
    // the panel renders a nearly-empty signup list whose recipient row still
    // reads as in-flight (#2724).
    if (isCompleted) {
      return [
        const SizedBox(height: 4),
        _completedSummary(context, selected),
      ];
    }

    // A recipient has been chosen → flip from choosing to handing off: confirm
    // the person, coordinate the pickup, reassure the others
    // (gear-giveaway-recipient-selected.html).
    if (selected != null && isSelectedPhase) {
      return _handoffBody(context, state, selected);
    }

    // Owner reach layout (gear-giveaway-who-wants-reach.html): a stat strip, a
    // nudge to share wider, then the labelled give-to list. The "share it to
    // your other circles" nudge only appears while there ARE other circles
    // left to share into — with everything already shared it's a false
    // prompt (#2724).
    if (canSelect) {
      return [
        const SizedBox(height: 4),
        _statStrip(context, state, interestedCount),
        if (_hasUnsharedCircles(state)) ...[
          const SizedBox(height: 16),
          _shareNudge(context, state),
        ],
        const SizedBox(height: 18),
        if (interestedCount > 0)
          _sectionLabel(context.l10n.gearRaisedHandCount(interestedCount)),
        GearInterestRoster(
          gearId: widget.gearId,
          onSelectRecipient: _selectRecipient,
        ),
      ];
    }

    return [
      // The full signup list — identical rows to the read-shell card, just
      // uncapped (the card shows the viewer + two others; this shows everyone).
      GearInterestRoster(gearId: widget.gearId),
      if (!isOwner && !isCompleted) ...[
        const SizedBox(height: 20),
        _cta(context, state),
      ],
    ];
  }

  // ── Completed → static summary ─────────────────────────────────────────────

  /// The terminal summary once the giveaway is done: a check header naming the
  /// outcome and the recipient with their "Received it" badge, falling back to
  /// "Gone to a new home" when the recipient isn't available. Mirrors the
  /// read-shell card's completed treatment so the collapsed and expanded
  /// surfaces agree (#2724).
  Widget _completedSummary(BuildContext context, User? recipient) {
    final l10n = context.l10n;
    return Container(
      padding: const EdgeInsets.all(17),
      decoration: BoxDecoration(
        color: _mint.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(20),
        border: Border.all(color: _mint.withValues(alpha: 0.38)),
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
            Row(
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
                Container(
                  padding:
                      const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
                  decoration: BoxDecoration(
                    color: AppColors.giveawayColorOnDark.withValues(alpha: 0.18),
                    borderRadius: BorderRadius.circular(999),
                  ),
                  child: Text(
                    l10n.gearGiveawayReceivedTag.toUpperCase(),
                    style: const TextStyle(
                      color: AppColors.giveawayColorOnDark,
                      fontSize: 9,
                      fontWeight: FontWeight.w700,
                      letterSpacing: 0.6,
                    ),
                  ),
                ),
              ],
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

  // ── Recipient selected → hand-off coordination ─────────────────────────────

  List<Widget> _handoffBody(
      BuildContext context, GearState state, User selected) {
    final l10n = context.l10n;
    final ctx = state.transferContext;
    final others = <User>[
      for (final r in ctx?.pendingRequests ?? const <TransferRequest>[])
        if (r.hasBorrower() && r.borrower.id != selected.id) r.borrower,
    ];
    return [
      const SizedBox(height: 4),
      _recipientCard(context, state, selected),
      const SizedBox(height: 18),
      ?_handoff(context, state, selected),
      if (others.isNotEmpty) ...[
        const SizedBox(height: 6),
        _sectionLabel(l10n.gearGiveawayTheOthers),
        _othersNote(context, others),
      ],
    ];
  }

  /// The confirmation card: a large avatar with a check, "Going to {name}", and
  /// (when wired) a button to message them.
  Widget _recipientCard(BuildContext context, GearState state, User selected) {
    final l10n = context.l10n;
    final ctx = state.transferContext;
    final pickedAt = ctx?.hasSelectedRecipient() ?? false
        ? ctx!.selectedRecipient.requestedAtUnixSec.toInt()
        : 0;
    final firstName = selected.name.split(' ').first;
    return Container(
      padding: const EdgeInsets.all(17),
      decoration: BoxDecoration(
        color: _mint.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(20),
        border: Border.all(color: _mint.withValues(alpha: 0.38)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Stack(
                clipBehavior: Clip.none,
                children: [
                  UserAvatar(user: selected, radius: 29),
                  Positioned(
                    right: -3,
                    bottom: -3,
                    child: Container(
                      width: 24,
                      height: 24,
                      decoration: BoxDecoration(
                        color: _mint,
                        shape: BoxShape.circle,
                        border: Border.all(
                            color: AppColors.darkBackground, width: 2.5),
                      ),
                      child: const Icon(Icons.check_rounded,
                          size: 13, color: Color(0xFF23351C)),
                    ),
                  ),
                ],
              ),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      l10n.gearGiveawayGoingTo.toUpperCase(),
                      style: const TextStyle(
                        fontSize: 10,
                        fontWeight: FontWeight.w800,
                        letterSpacing: 1,
                        color: _mint,
                      ),
                    ),
                    const SizedBox(height: 3),
                    Text(
                      selected.name,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontSize: 20,
                        fontWeight: FontWeight.w800,
                        color: AppColors.onContentImage,
                      ),
                    ),
                    if (pickedAt > 0) ...[
                      const SizedBox(height: 3),
                      Text(
                        l10n.gearGiveawayPicked(
                            firstName, formatRelativeTimestamp(pickedAt)),
                        style: const TextStyle(
                          fontSize: 12.5,
                          color: AppColors.darkTextSecondary,
                        ),
                      ),
                    ],
                  ],
                ),
              ),
            ],
          ),
          if (widget.onOpenConversation != null) ...[
            const SizedBox(height: 15),
            Tappable(
              semanticsLabel: l10n.gearGiveawayMessageName(firstName),
              onTap: widget.onOpenConversation,
              child: Container(
                padding: const EdgeInsets.symmetric(vertical: 11),
                decoration: BoxDecoration(
                  color: GlassTokens.fillSubtle,
                  borderRadius: BorderRadius.circular(999),
                  border: Border.all(color: GlassTokens.borderSoft),
                ),
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    const Icon(Icons.chat_bubble_outline_rounded,
                        size: 15, color: AppColors.onContentImage),
                    const SizedBox(width: 7),
                    Text(
                      l10n.gearGiveawayMessageName(firstName),
                      style: const TextStyle(
                        fontSize: 13.5,
                        fontWeight: FontWeight.w800,
                        color: AppColors.onContentImage,
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }

  /// "The others" reassurance: a mini avatar stack + a kindly note.
  Widget _othersNote(BuildContext context, List<User> others) {
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: GlassTokens.fillFaint,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: GlassTokens.hairline),
      ),
      child: Row(
        children: [
          SizedBox(
            width: 30.0 + (others.length - 1).clamp(0, 3) * 21,
            height: 30,
            child: Stack(
              children: [
                for (var i = 0; i < others.length && i < 4; i++)
                  Positioned(
                    left: i * 21.0,
                    child: Container(
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        border: Border.all(
                            color: AppColors.darkBackground, width: 2),
                      ),
                      child: UserAvatar(user: others[i], radius: 13),
                    ),
                  ),
              ],
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              context.l10n.gearGiveawayOthersReassure(others.length),
              style: TextStyle(
                fontSize: 12.5,
                height: 1.4,
                color: GlassTokens.textSecondary,
              ),
            ),
          ),
        ],
      ),
    );
  }

  /// Sticky footer: the owner closes the loop with "Mark as given".
  Widget? _footer(BuildContext context, GearState state) {
    final ctx = state.transferContext;
    final isSelectedPhase =
        ctx?.overallPhase == GiveawayPhase.GIVEAWAY_PHASE_RECIPIENT_SELECTED;
    if (!state.isOwner || !isSelectedPhase || ctx?.hasSelectedRecipient() != true) {
      return null;
    }
    final recipient = ctx!.selectedRecipient;
    final firstName = recipient.borrower.name.split(' ').first;
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 8, 20, 14),
      child: Tappable(
        semanticsLabel: context.l10n.a11yGearMarkGiven,
        onTap: state.isSaving
            ? () {}
            : () => _markGiven(recipient.transferId),
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 16),
          decoration: const ShapeDecoration(
            color: GlassTokens.primary,
            // Pill. This was a 15px squircle, one of several radii this panel
            // used for the same kind of control.
            shape: StadiumBorder(),
          ),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              const Icon(Icons.card_giftcard_rounded,
                  size: 18, color: GlassTokens.onPrimary),
              const SizedBox(width: 9),
              Text(
                context.l10n.gearGiveawayMarkGiven(firstName),
                style: const TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w800,
                  color: GlassTokens.onPrimary,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Future<void> _markGiven(String transferId) async {
    final l10n = context.l10n;
    try {
      await ref
          .read(transferRepositoryProvider)
          .completeTransfer(transferId: transferId);
      if (!mounted) return;
      await _refreshAfterSelection();
      if (!mounted) return;
      ToastHelper.showSuccess(context, l10n.gearGiveawayMarkedGivenToast);
    } catch (e) {
      if (!mounted) return;
      ToastHelper.showError(context, l10n.gearCalendarGenericError);
    }
  }

  // ── Owner reach elements ───────────────────────────────────────────────────

  Widget _statStrip(BuildContext context, GearState state, int interested) {
    final l10n = context.l10n;
    final listedUnix = _listedUnix(state);
    return Row(
      children: [
        Expanded(
          child: _statTile('$interested', l10n.gearReachStatInterested),
        ),
        const SizedBox(width: 8),
        Expanded(
          child: _statTile(
            listedUnix == null ? '–' : _compactAge(listedUnix),
            l10n.gearReachStatListed,
          ),
        ),
      ],
    );
  }

  Widget _statTile(String value, String label) {
    return Container(
      padding: const EdgeInsets.symmetric(vertical: 14, horizontal: 6),
      decoration: BoxDecoration(
        color: GlassTokens.fillFaint,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: GlassTokens.hairline),
      ),
      child: Column(
        children: [
          Text(
            value,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
              fontFamily: AppTheme.headingFont,
              fontSize: 24,
              fontWeight: FontWeight.w600,
              color: AppColors.onContentImage,
              height: 1,
            ),
          ),
          const SizedBox(height: 7),
          Text(
            label.toUpperCase(),
            style: TextStyle(
              fontSize: 9.5,
              fontWeight: FontWeight.w700,
              letterSpacing: 0.5,
              color: GlassTokens.textFaint,
            ),
          ),
        ],
      ),
    );
  }

  /// True when the owner belongs to at least one NAMED community the gear is
  /// not yet shared with. Nameless entries in the portfolio are per-item
  /// ad-hoc groups (#2492), not "circles" to reach into, so they don't count.
  bool _hasUnsharedCircles(GearState state) {
    final sharedIds =
        state.sharedCommunities.map((c) => c.communityId).toSet();
    // watch: called from build via _body, so the nudge reacts to portfolio
    // changes (e.g. the share flow adding the last unshared circle).
    return ref
        .watch(communitiesProvider)
        .communities
        .any((c) => c.name.trim().isNotEmpty && !sharedIds.contains(c.id));
  }

  Widget _shareNudge(BuildContext context, GearState state) {
    final l10n = context.l10n;
    const gold = Color(0xFFD8A13A);
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: gold.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: gold.withValues(alpha: 0.34)),
      ),
      child: Row(
        children: [
          Container(
            width: 38,
            height: 38,
            alignment: Alignment.center,
            decoration: BoxDecoration(
              color: gold.withValues(alpha: 0.2),
              borderRadius: BorderRadius.circular(11),
            ),
            child: const Icon(Icons.ios_share, size: 18, color: gold),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Text.rich(
              TextSpan(
                children: [
                  TextSpan(
                    text: l10n.gearReachNudgeLead,
                    style: const TextStyle(
                      color: AppColors.onContentImage,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  TextSpan(text: l10n.gearReachNudgeBody),
                ],
                style: TextStyle(
                  fontSize: 12.5,
                  height: 1.4,
                  color: GlassTokens.textSecondary,
                ),
              ),
            ),
          ),
          const SizedBox(width: 8),
          Tappable(
            semanticsLabel: l10n.a11yGearShareReach,
            onTap: () => _share(context, state),
            child: Padding(
              padding: const EdgeInsets.all(4),
              child: Text(
                l10n.commonShare,
                style: const TextStyle(
                  fontSize: 12.5,
                  fontWeight: FontWeight.w800,
                  color: gold,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _sectionLabel(String label) {
    return Padding(
      padding: const EdgeInsets.only(top: 6, bottom: 8),
      child: Row(
        children: [
          Text(
            label.toUpperCase(),
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w800,
              letterSpacing: 1,
              color: GlassTokens.textFaint,
            ),
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Container(
              height: 1,
              color: GlassTokens.hairline,
            ),
          ),
        ],
      ),
    );
  }

  /// Compact relative age for the stat tile ("2d", "3h", "new").
  String _compactAge(int unixSec) {
    final s = formatRelativeTimestamp(unixSec);
    if (s == 'Just now') return 'new';
    return s.replaceAll(' ago', '');
  }

  /// Opens the gear share sheet (QR + invite link) to reach more friends.
  Future<void> _share(BuildContext context, GearState state) async {
    final gear = state.gearDetails;
    if (gear == null) return;
    await ItemShareSheet.show(
      context,
      itemType: ShareableItemType.gear,
      itemId: gear.id,
      itemName: gear.name,
    );
  }

  /// Picks an interested member as the recipient inline — no modal. Refreshes
  /// the gear (so the panel flips to the hand-off) and offers a quick undo.
  Future<void> _selectRecipient(String transferId, User recipient) async {
    final l10n = context.l10n;
    final repo = ref.read(transferRepositoryProvider);
    // Outcome-first, named toast (#2724): "It's going to Dana", not the
    // cold "Recipient selected".
    final firstName = recipient.name.trim().split(' ').first;
    try {
      final result = await repo.selectRecipient(
        transferId: transferId,
        recipientId: recipient.id,
      );
      if (!mounted) return;
      await _refreshAfterSelection();
      if (!mounted) return;
      ToastHelper.showServerUndo(
        context: context,
        message: l10n.gearInterestRecipientSelected(firstName),
        undoLabel: l10n.commonUndo,
        onUndo: () =>
            repo.undoSelectRecipient(communityEventId: result.communityEventId),
        onUndoSucceeded: _refreshAfterSelection,
      );
    } catch (e) {
      if (!mounted) return;
      ToastHelper.showError(context, l10n.gearCalendarGenericError);
    }
  }

  /// Reload the gear (transfer context → who's selected) and the booking that
  /// backs the selected recipient's pickup hand-off. Re-`initialize` rather than
  /// `load` because the booking provider is autoDispose and may have been torn
  /// down (and its community context lost) while the reach view — which doesn't
  /// watch it — was on screen.
  Future<void> _refreshAfterSelection() async {
    await ref.read(gearProvider(widget.gearId).notifier).refreshGearDetails();
    if (!mounted) return;
    final communityId = widget.communityId;
    if (communityId != null) {
      await ref
          .read(gearBookingProvider(widget.gearId).notifier)
          .initialize(communityId: communityId);
    }
  }

  /// The pickup hand-off for the selected recipient, backed by their giveaway
  /// transfer (surfaced as a booking). Returns null until the booking loads.
  Widget? _handoff(BuildContext context, GearState state, User selected) {
    final bookings = ref.watch(gearBookingProvider(widget.gearId)).bookings;
    GearBooking? booking;
    for (final b in bookings) {
      if (b.borrower.id == selected.id) {
        booking = b;
        break;
      }
    }
    if (booking == null) return null;
    return GearHandoffSection(
      gearId: widget.gearId,
      booking: booking,
      showDropoff: false,
      title: context.l10n.gearGiveawayArrangeHandoff,
    );
  }

  /// The non-owner sign-up CTA: an interested member can withdraw; everyone
  /// else can express interest. (The owner selects inline per row, not here.)
  Widget _cta(BuildContext context, GearState state) {
    final l10n = context.l10n;
    final ctx = state.transferContext;

    final hasExpressed = (ctx?.hasUserTransfer() ?? false) &&
        ctx!.userTransfer.state ==
            TransferState.TRANSFER_STATE_INTEREST_EXPRESSED;
    if (hasExpressed) {
      return _button(
        label: l10n.gearInterestWithdraw,
        semanticsLabel: l10n.a11yGearWithdrawInterest,
        filled: false,
        onTap: state.isSaving
            ? null
            : () =>
                ref.read(gearProvider(widget.gearId).notifier).withdrawInterest(),
      );
    }
    return _button(
      label: l10n.gearInterestExpress,
      semanticsLabel: l10n.a11yGearExpressInterest,
      filled: true,
      onTap: state.isSaving
          ? null
          : () =>
              ref.read(gearProvider(widget.gearId).notifier).expressInterest(),
    );
  }

  /// The CTA, sized to match the experience content view's RSVP chip
  /// ([ContentActionChip]): a full-width pill, roomy but compact.
  Widget _button({
    required String label,
    required String semanticsLabel,
    required bool filled,
    required VoidCallback? onTap,
  }) {
    final fg = filled ? AppColors.darkBackground : AppColors.onContentImage;
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap ?? () {},
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 13, vertical: 7),
        decoration: BoxDecoration(
          color: filled
              ? AppColors.giveawayColorOnDark
              : GlassTokens.fillFaint,
          borderRadius: BorderRadius.circular(999),
          border: filled
              ? null
              : Border.all(color: GlassTokens.borderSoft),
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

}
