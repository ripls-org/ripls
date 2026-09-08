import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/gear_booking.pbenum.dart' as pb
    show GearBookingState;
import 'package:ripls/data/repositories/gear_repository.dart' show GearBooking;
import 'package:ripls/presentation/screens/gear/widgets/gear_booking_grid.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_handoff_section.dart';
import 'package:ripls/presentation/screens/gear/widgets/gear_owner_reserve_dock.dart';
import 'package:ripls/presentation/viewmodels/gear_booking_view_model.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_morph_panel.dart';
import 'package:ripls/presentation/widgets/home/calendar/calendar_backdrop.dart';
import 'package:ripls/presentation/widgets/sharing/item_share_sheet.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';

/// GearWhosUsingCalendar is the full-screen "who's using it" booking calendar
/// the read shell's who's-using card / "See the calendar" CTA expands into. It
/// matches the home calendar's design language (full-bleed photo backdrop +
/// readability scrim, Monday-first month grid, serif day numbers, ring
/// treatments) while plotting the gear's bookings: who has it which days, with a
/// mint ring on the viewer's own days and "+" on open future days. The dock
/// below shows the selected range and lets the viewer claim an open day, drop
/// their own booking, set the pickup/drop-off hand-off, or open comments.
class GearWhosUsingCalendar extends ConsumerStatefulWidget {
  final String gearId;
  final String communityId;

  /// Hero media for the calendar backdrop (the gear's cover photo).
  final String heroMediaId;

  /// Opens the gear conversation (comments).
  final VoidCallback onOpenComments;

  const GearWhosUsingCalendar({
    super.key,
    required this.gearId,
    required this.communityId,
    required this.heroMediaId,
    required this.onOpenComments,
  });

  @override
  ConsumerState<GearWhosUsingCalendar> createState() =>
      _GearWhosUsingCalendarState();
}

class _GearWhosUsingCalendarState extends ConsumerState<GearWhosUsingCalendar> {
  late DateTime _month;
  // Inclusive selected range. For a booked day the range spans the whole
  // booking; for open days the user can extend a contiguous-open span before
  // claiming, so a row of days becomes one multi-day booking.
  DateTime? _selStart;
  DateTime? _selEnd;

  static const _mint = Color(0xFFA7C59E);

  @override
  void initState() {
    super.initState();
    final now = DateTime.now();
    _month = DateTime(now.year, now.month);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      ref
          .read(gearBookingProvider(widget.gearId).notifier)
          .initialize(communityId: widget.communityId);
    });
  }

  DateTime _dayKey(DateTime d) => DateTime(d.year, d.month, d.day);

  DateTime _toDay(int unixSec) =>
      _dayKey(DateTime.fromMillisecondsSinceEpoch(unixSec * 1000));

  /// Buckets each booked calendar day to its booking (a multi-day booking marks
  /// every day in its inclusive range).
  Map<DateTime, GearBooking> _bookingsByDay(List<GearBooking> bookings) {
    final map = <DateTime, GearBooking>{};
    for (final b in bookings) {
      var day = _toDay(b.startDateUnixSec.toInt());
      final end = _toDay(b.endDateUnixSec.toInt());
      while (!day.isAfter(end)) {
        map[day] = b;
        day = day.add(const Duration(days: 1));
      }
    }
    return map;
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(gearBookingProvider(widget.gearId));
    final byDay = _bookingsByDay(state.bookings);

    return ContentMorphPanel(
      scrimColor: Colors.transparent,
      child: Stack(
        fit: StackFit.expand,
        children: [
          CalendarBackdrop(mediaId: widget.heroMediaId),
          // Strong uniform scrim (same as the details panel) so the grid + dock
          // stay legible over any hero photo — the home calendar's lighter
          // gradient left the middle too bright to read.
          const ColoredBox(color: AppColors.modalContentScrimStrong),
          SafeArea(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                _header(context),
                _monthNav(context),
                Expanded(
                  child: SingleChildScrollView(
                    padding: const EdgeInsets.fromLTRB(16, 14, 16, 28),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        _grid(context, byDay),
                        const SizedBox(height: 18),
                        _dock(context, byDay, state),
                      ],
                    ),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  // ── Header + month nav ─────────────────────────────────────────────────────

  Widget _header(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 6, 12, 0),
      child: Row(
        children: [
          Expanded(
            child: Semantics(
              header: true,
              child: Text(
                context.l10n.gearSectionWhosUsingIt,
                style: const TextStyle(
                  fontFamily: AppTheme.headingFont,
                  fontSize: 28,
                  fontWeight: FontWeight.w600,
                  color: GlassTokens.textPrimary,
                ),
              ),
            ),
          ),
          IconAction(
            icon: Icons.close_rounded,
            semanticsLabel: context.l10n.a11yClose,
            color: GlassTokens.textPrimary,
            onPressed: () => Navigator.of(context).pop(),
          ),
        ],
      ),
    );
  }

  Widget _monthNav(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(24, 14, 20, 0),
      child: Row(
        children: [
          Expanded(
            child: Text(
              DateFormat('MMMM yyyy').format(_month),
              style: const TextStyle(
                fontFamily: AppTheme.headingFont,
                fontSize: 21,
                fontWeight: FontWeight.w600,
                color: GlassTokens.textPrimary,
              ),
            ),
          ),
          _navButton(Icons.chevron_left, () => _shiftMonth(-1),
              context.l10n.homeCalendarPrevMonth),
          const SizedBox(width: 8),
          _navButton(Icons.chevron_right, () => _shiftMonth(1),
              context.l10n.homeCalendarNextMonth),
        ],
      ),
    );
  }

  Widget _navButton(IconData icon, VoidCallback onTap, String label) {
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      child: Container(
        width: 30,
        height: 30,
        decoration: BoxDecoration(
          color: GlassTokens.fillSubtle,
          shape: BoxShape.circle,
        ),
        child: Icon(icon, size: 18, color: GlassTokens.textPrimary),
      ),
    );
  }

  void _shiftMonth(int delta) {
    setState(() => _month = DateTime(_month.year, _month.month + delta));
  }

  // ── Grid ───────────────────────────────────────────────────────────────────

  Widget _grid(BuildContext context, Map<DateTime, GearBooking> byDay) {
    return GearBookingGrid(
      month: _month,
      byDay: byDay,
      selStart: _selStart,
      selEnd: _selEnd,
      onTapDay: (day, booking) => _onTapDay(day, booking, byDay),
    );
  }

  // ── Selection ──────────────────────────────────────────────────────────────

  /// Whether every day in [start]..[end] is an open (unbooked) future day, so a
  /// contiguous span can be claimed as a single booking.
  bool _isOpenRange(
    DateTime start,
    DateTime end,
    Map<DateTime, GearBooking> byDay,
  ) {
    final today = _dayKey(DateTime.now());
    for (var d = start; !d.isAfter(end); d = d.add(const Duration(days: 1))) {
      if (d.isBefore(today) || byDay.containsKey(d)) return false;
    }
    return true;
  }

  bool _inSelection(DateTime day) =>
      _selStart != null &&
      _selEnd != null &&
      !day.isBefore(_selStart!) &&
      !day.isAfter(_selEnd!);

  void _onTapDay(DateTime day, GearBooking? booking, Map<DateTime, GearBooking> byDay) {
    setState(() {
      if (booking != null) {
        final bStart = _toDay(booking.startDateUnixSec.toInt());
        final bEnd = _toDay(booking.endDateUnixSec.toInt());
        // Tapping the already-selected booking unselects it; otherwise select
        // its whole span (a booking can't be partially selected).
        if (_selStart == bStart && _selEnd == bEnd) {
          _clearSelection();
        } else {
          _selStart = bStart;
          _selEnd = bEnd;
        }
        return;
      }

      // Tapping a day that's already selected toggles it back off.
      if (_inSelection(day)) {
        if (_selStart == _selEnd) {
          _clearSelection();
        } else if (day == _selStart) {
          // Trim the near edge inward.
          _selStart = day.add(const Duration(days: 1));
        } else if (day == _selEnd) {
          _selEnd = day.subtract(const Duration(days: 1));
        } else {
          // An interior day of a multi-day range — can't split a contiguous
          // selection, so clear it.
          _clearSelection();
        }
        return;
      }

      // Open day not yet selected: extend a contiguous-open selection, else
      // start fresh.
      if (_selStart != null &&
          _selEnd != null &&
          _isOpenRange(_selStart!, _selEnd!, byDay)) {
        final newStart = day.isBefore(_selStart!) ? day : _selStart!;
        final newEnd = day.isAfter(_selEnd!) ? day : _selEnd!;
        if (_isOpenRange(newStart, newEnd, byDay)) {
          _selStart = newStart;
          _selEnd = newEnd;
          return;
        }
      }
      _selStart = day;
      _selEnd = day;
    });
  }

  void _clearSelection() {
    _selStart = null;
    _selEnd = null;
  }

  String _rangeLabel(DateTime start, DateTime end) {
    if (start == end) return DateFormat('EEE, MMM d').format(start);
    return '${DateFormat('EEE MMM d').format(start)} – ${DateFormat('EEE MMM d').format(end)}';
  }

  // ── Dock (selected range hero + actions + hand-off) ────────────────────────

  Widget _dock(
    BuildContext context,
    Map<DateTime, GearBooking> byDay,
    GearBookingState state,
  ) {
    final start = _selStart;
    final end = _selEnd;
    final isOwner = ref.watch(gearProvider(widget.gearId)).isOwner;
    if (start == null || end == null) {
      // Owners reserve/share/block their gear; borrowers pick days to book.
      return _opentip(isOwner
          ? context.l10n.gearReserveTapTip
          : context.l10n.gearLoanCtaSub);
    }
    final booking = byDay[start];
    final isPast = start.isBefore(_dayKey(DateTime.now()));

    if (booking == null) {
      if (isPast) return _opentip(context.l10n.gearCalendarPastDayTip);
      // The owner can't borrow from themselves — they reserve, link, or block.
      if (isOwner) {
        return GearOwnerReserveDock(
          gearId: widget.gearId,
          communityId: widget.communityId,
          start: start,
          end: end,
          rangeLabel: _rangeLabel(start, end),
        );
      }
      return _openDock(context, start, end, state);
    }
    return _bookedDock(context, booking, state, isOwner: isOwner);
  }

  Widget _opentip(String text) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Text(
        text,
        style: TextStyle(
          color: GlassTokens.textFaint,
          fontSize: 13,
          fontStyle: FontStyle.italic,
        ),
      ),
    );
  }

  Widget _openDock(
    BuildContext context,
    DateTime start,
    DateTime end,
    GearBookingState state,
  ) {
    final l10n = context.l10n;
    final ownerName =
        ref.watch(gearProvider(widget.gearId)).gearDetails?.owner.name ?? '';
    final ownerFirst = ownerName.trim().isEmpty
        ? l10n.gearCalendarBorrowOwnerGeneric
        : ownerName.trim().split(' ').first;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        // "Open · Sun, Jun 21" — no inline action button anymore; the warm
        // full-width CTA below replaces the small "Claim" chip.
        _heroRow(
          label: l10n.gearCalendarOpenLabel,
          labelColor: _mint,
          title: _rangeLabel(start, end),
        ),
        const SizedBox(height: 12),
        Text.rich(
          TextSpan(
            children: [
              TextSpan(text: l10n.gearCalendarBorrowNoAskLead),
              TextSpan(
                text: l10n.gearCalendarBorrowNoAskEmph,
                style: const TextStyle(
                  color: GlassTokens.textPrimary,
                  fontWeight: FontWeight.w700,
                ),
              ),
              TextSpan(text: l10n.gearCalendarBorrowNoAskTail(ownerFirst)),
            ],
            style: const TextStyle(
              color: GlassTokens.textSecondary,
              fontSize: 14.5,
              height: 1.5,
            ),
          ),
        ),
        const SizedBox(height: 18),
        _borrowButton(context, start, end, enabled: !state.isMutating),
        const SizedBox(height: 10),
        Text(
          l10n.gearCalendarBorrowSubnote,
          textAlign: TextAlign.center,
          style: const TextStyle(
            color: GlassTokens.textFaint,
            fontSize: 11.5,
            height: 1.45,
          ),
        ),
      ],
    );
  }

  /// The warm, full-width "Borrow it →" CTA (gear-borrow-no-need-to-ask design):
  /// a friend-to-friend claim, not a request to be approved.
  Widget _borrowButton(
    BuildContext context,
    DateTime start,
    DateTime end, {
    required bool enabled,
  }) {
    return Tappable(
      semanticsLabel: context.l10n.gearCalendarBorrowIt,
      onTap: enabled ? () => _claim(start, end) : () {},
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 17),
        decoration: BoxDecoration(
          color: _mint,
          borderRadius: BorderRadius.circular(16),
        ),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Text(
              context.l10n.gearCalendarBorrowIt,
              style: const TextStyle(
                color: Color(0xFF23351C),
                fontSize: 16,
                fontWeight: FontWeight.w800,
              ),
            ),
            const SizedBox(width: 9),
            const Icon(Icons.arrow_forward_rounded,
                size: 18, color: Color(0xFF23351C)),
          ],
        ),
      ),
    );
  }

  Widget _bookedDock(
    BuildContext context,
    GearBooking booking,
    GearBookingState state, {
    required bool isOwner,
  }) {
    final start = _toDay(booking.startDateUnixSec.toInt());
    final end = _toDay(booking.endDateUnixSec.toInt());
    final range = _rangeLabel(start, end);
    final saving = state.isMutating;

    // A link reservation still held, awaiting acceptance.
    if (booking.isPending) {
      return _pendingDock(context, booking, range, isOwner: isOwner, saving: saving);
    }
    // The owner has blocked the days for their own use.
    if (booking.isOwnerBlock) {
      return _selfBlockDock(context, booking, range, saving: saving);
    }
    // Reserved for a person — the borrower's own booking, or the owner's
    // reservation for someone. Both the recipient and the owner coordinate.
    return _reservedDock(context, booking, range,
        canManage: booking.isMine || isOwner, saving: saving);
  }

  Widget _reservedDock(
    BuildContext context,
    GearBooking booking,
    String range, {
    required bool canManage,
    required bool saving,
  }) {
    final l10n = context.l10n;
    final isMine = booking.isMine;
    final hasBorrower = booking.hasBorrower();
    final firstName =
        hasBorrower ? booking.borrower.name.split(' ').first : '';
    // A started loan can't be dropped — the server rejects the release
    // (#2638 follow-up); returning goes through mark-returned instead
    // (who-card CTA / Home pill). Hide Drop rather than offering an error.
    final droppable =
        booking.state == pb.GearBookingState.GEAR_BOOKING_STATE_RESERVED;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _heroRow(
          label: isMine
              ? l10n.gearCalendarYourBooking
              : l10n.gearReserveReservedFor(firstName),
          labelColor: isMine ? _mint : const Color(0xFFE2A07F),
          title: range,
          who: isMine || !hasBorrower
              ? null
              : Row(
                  children: [
                    UserAvatar(user: booking.borrower, radius: 12),
                    const SizedBox(width: 8),
                    Text(
                      booking.borrower.name,
                      style: const TextStyle(
                        color: GlassTokens.textSecondary,
                        fontSize: 14,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ],
                ),
          action: canManage && droppable
              ? _actionButton(
                  icon: Icons.close_rounded,
                  caption: l10n.gearCalendarDrop,
                  background: GlassTokens.fillSubtle,
                  foreground: GlassTokens.textPrimary,
                  enabled: !saving,
                  onTap: () => _drop(booking),
                )
              : null,
        ),
        if (canManage && hasBorrower) ...[
          const SizedBox(height: 18),
          GearHandoffSection(gearId: widget.gearId, booking: booking),
        ],
        const SizedBox(height: 18),
        _commentsButton(context),
      ],
    );
  }

  Widget _selfBlockDock(
    BuildContext context,
    GearBooking booking,
    String range, {
    required bool saving,
  }) {
    final l10n = context.l10n;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _heroRow(
          label: l10n.gearReserveBlockedLabel,
          labelColor: const Color(0xFFCDBB8A),
          title: range,
          who: Text(
            l10n.gearReserveBlockedWho,
            style: const TextStyle(
              color: GlassTokens.textFaint,
              fontSize: 14,
              fontWeight: FontWeight.w600,
            ),
          ),
          action: _actionButton(
            icon: Icons.lock_open_rounded,
            caption: l10n.gearReserveOpenUp,
            background: GlassTokens.fillSubtle,
            foreground: GlassTokens.textPrimary,
            enabled: !saving,
            onTap: () => _drop(booking),
          ),
        ),
        const SizedBox(height: 14),
        Text(
          l10n.gearReserveBlockedTip,
          style: const TextStyle(
            color: GlassTokens.textFaint,
            fontSize: 13,
            fontStyle: FontStyle.italic,
            height: 1.5,
          ),
        ),
      ],
    );
  }

  Widget _pendingDock(
    BuildContext context,
    GearBooking booking,
    String range, {
    required bool isOwner,
    required bool saving,
  }) {
    final l10n = context.l10n;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _heroRow(
          label: l10n.gearReservePendingLabel,
          labelColor: const Color(0xFFD8A13A),
          title: range,
          action: isOwner
              ? _actionButton(
                  icon: Icons.close_rounded,
                  caption: l10n.commonCancel,
                  background: GlassTokens.fillSubtle,
                  foreground: GlassTokens.textPrimary,
                  enabled: !saving,
                  onTap: () => _drop(booking),
                )
              : null,
        ),
        const SizedBox(height: 16),
        if (isOwner)
          _pendingCard(context)
        else
          _acceptButton(context, booking, saving: saving),
      ],
    );
  }

  Widget _pendingCard(BuildContext context) {
    final l10n = context.l10n;
    const gold = Color(0xFFD8A13A);
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: gold.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(15),
        border: Border.all(color: gold.withValues(alpha: 0.34)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            l10n.gearReservePendingBody,
            style: const TextStyle(
              color: GlassTokens.textSecondary,
              fontSize: 13,
              height: 1.45,
            ),
          ),
          const SizedBox(height: 12),
          Tappable(
            semanticsLabel: l10n.gearReserveShareLink,
            onTap: _openShareSheet,
            child: Container(
              padding: const EdgeInsets.symmetric(vertical: 11),
              decoration: BoxDecoration(
                color: GlassTokens.scrimTint,
                borderRadius: BorderRadius.circular(11),
                border: Border.all(color: GlassTokens.hairline),
              ),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  const Icon(Icons.qr_code_rounded, size: 16, color: gold),
                  const SizedBox(width: 8),
                  Text(
                    l10n.gearReserveShareLink,
                    style: const TextStyle(
                      color: gold,
                      fontSize: 13,
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _acceptButton(
    BuildContext context,
    GearBooking booking, {
    required bool saving,
  }) {
    return Tappable(
      semanticsLabel: context.l10n.gearReserveAccept,
      onTap: saving ? () {} : () => _accept(booking),
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 16),
        decoration: BoxDecoration(
          color: _mint,
          borderRadius: BorderRadius.circular(15),
        ),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Text(
              context.l10n.gearReserveAccept,
              style: const TextStyle(
                color: Color(0xFF23351C),
                fontSize: 16,
                fontWeight: FontWeight.w800,
              ),
            ),
            const SizedBox(width: 9),
            const Icon(Icons.arrow_forward_rounded,
                size: 17, color: Color(0xFF23351C)),
          ],
        ),
      ),
    );
  }

  Widget _heroRow({
    required String label,
    required Color labelColor,
    required String title,
    Widget? who,
    Widget? action,
  }) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.end,
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                label.toUpperCase(),
                style: TextStyle(
                  color: labelColor,
                  fontSize: 10,
                  fontWeight: FontWeight.w800,
                  letterSpacing: 1,
                ),
              ),
              const SizedBox(height: 4),
              Text(
                title,
                style: const TextStyle(
                  fontFamily: AppTheme.headingFont,
                  fontSize: 27,
                  fontWeight: FontWeight.w600,
                  color: GlassTokens.textPrimary,
                  height: 1.02,
                ),
              ),
              if (who != null) ...[const SizedBox(height: 8), who],
            ],
          ),
        ),
        if (action != null) ...[const SizedBox(width: 12), action],
      ],
    );
  }

  Widget _actionButton({
    required IconData icon,
    required String caption,
    required Color background,
    required Color foreground,
    required bool enabled,
    required VoidCallback onTap,
  }) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Tappable(
          semanticsLabel: caption,
          onTap: enabled ? onTap : () {},
          child: Container(
            width: 50,
            height: 50,
            decoration: BoxDecoration(
              color: background,
              borderRadius: BorderRadius.circular(15),
            ),
            child: Icon(icon, color: foreground, size: 24),
          ),
        ),
        const SizedBox(height: 4),
        Text(
          caption,
          style: const TextStyle(
            color: GlassTokens.textFaint,
            fontSize: 9,
            fontWeight: FontWeight.w700,
          ),
        ),
      ],
    );
  }

  Widget _commentsButton(BuildContext context) {
    // Wrap in a SizedBox so the flex child of the dock's stretch Column is a
    // plain RenderBox, not the Tappable's Semantics (avoids a scroll-view
    // semantics assertion).
    return SizedBox(
      width: double.infinity,
      child: Tappable(
        semanticsLabel: context.l10n.contentMessagesStart,
        onTap: widget.onOpenComments,
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 14),
        decoration: BoxDecoration(
          color: GlassTokens.fillFaint,
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: GlassTokens.borderSoft),
        ),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Icon(Icons.forum_outlined,
                size: 19, color: GlassTokens.textPrimary),
            const SizedBox(width: 9),
            Text(
              context.l10n.contentMessagesStart,
              style: const TextStyle(
                color: GlassTokens.textPrimary,
                fontSize: 14.5,
                fontWeight: FontWeight.w600,
              ),
            ),
          ],
          ),
        ),
      ),
    );
  }

  // ── Actions ────────────────────────────────────────────────────────────────

  Future<void> _claim(DateTime start, DateTime end) async {
    final startUnix = start.millisecondsSinceEpoch ~/ 1000;
    final endUnix = end.millisecondsSinceEpoch ~/ 1000;
    final booking =
        await ref.read(gearBookingProvider(widget.gearId).notifier).claim(
              startDateUnixSec: startUnix,
              endDateUnixSec: endUnix,
            );
    if (!mounted || booking != null) return;
    // No success toast — booking a day is its own confirmation on the calendar.
    _showError();
  }

  /// A link recipient accepts a pending owner reservation, claiming the days.
  Future<void> _accept(GearBooking booking) async {
    final ok =
        await ref.read(gearBookingProvider(widget.gearId).notifier).accept(
              bookingId: booking.id,
              acceptToken: booking.acceptToken,
            );
    if (!mounted) return;
    if (ok) {
      ToastHelper.showSuccess(context, context.l10n.gearReserveAcceptedToast);
    } else {
      _showError();
    }
  }

  /// Opens the share sheet (QR code + invite link) for this gear so a link
  /// recipient can reach it and accept the held days.
  Future<void> _openShareSheet() async {
    final gear = ref.read(gearProvider(widget.gearId)).gearDetails;
    if (gear == null) return;
    await ItemShareSheet.show(
      context,
      itemType: ShareableItemType.gear,
      itemId: gear.id,
      itemName: gear.name,
    );
  }

  Future<void> _drop(GearBooking booking) async {
    final ok = await ref
        .read(gearBookingProvider(widget.gearId).notifier)
        .release(booking.id);
    if (!mounted) return;
    if (ok) {
      setState(() {
        _selStart = null;
        _selEnd = null;
      });
      ToastHelper.showSuccess(context, context.l10n.gearCalendarDroppedToast);
    } else {
      _showError();
    }
  }

  void _showError() {
    final err = ref.read(gearBookingProvider(widget.gearId)).error;
    ToastHelper.showError(
      context,
      err != null
          ? RpcErrorHandler.localize(err, context.l10n)
          : context.l10n.gearCalendarGenericError,
    );
  }
}
