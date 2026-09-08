import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/utils/calendar_helper.dart';
import 'package:ripls/core/utils/location_formatter.dart';
import 'package:ripls/core/utils/location_picker_helper.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/repositories/gear_repository.dart' show GearBooking;
import 'package:ripls/presentation/viewmodels/gear_booking_view_model.dart';
import 'package:ripls/presentation/viewmodels/gear_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/services/providers.dart';

/// GearHandoffSection renders the "Your hand-off" coordination block for a
/// booking: a PICKUP row (place + time + calendar export) and, for loans, a
/// matching DROP-OFF row. It is shared between the who's-using booking calendar
/// (loan, [showDropoff] true) and the giveaway interest panel (giveaway,
/// [showDropoff] false — a giveaway has no return, so only the pickup is
/// coordinated).
///
/// Each segment defaults to the gear's own location and edits persist through
/// [GearBookingNotifier.updateHandoff] (the server now lets either the owner or
/// the recipient set the hand-off). Custom hand-off locations resolve their
/// short display name asynchronously and cache.
class GearHandoffSection extends ConsumerStatefulWidget {
  final String gearId;
  final GearBooking booking;

  /// Whether to show the DROP-OFF row. Loans return the gear, so they show both
  /// pickup and drop-off; giveaways keep the gear, so they show pickup only.
  final bool showDropoff;

  /// Overrides the section header (defaults to "Your hand-off").
  final String? title;

  const GearHandoffSection({
    super.key,
    required this.gearId,
    required this.booking,
    this.showDropoff = true,
    this.title,
  });

  @override
  ConsumerState<GearHandoffSection> createState() => _GearHandoffSectionState();
}

class _GearHandoffSectionState extends ConsumerState<GearHandoffSection> {
  static const _mint = Color(0xFFA7C59E);

  // Resolved short names for custom hand-off locations, keyed by location id.
  final Map<String, String> _locNames = {};
  final Set<String> _resolvingLoc = {};

  GearBooking get _booking => widget.booking;

  DateTime _dayKey(DateTime d) => DateTime(d.year, d.month, d.day);

  DateTime _toDay(int unixSec) =>
      _dayKey(DateTime.fromMillisecondsSinceEpoch(unixSec * 1000));

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final gearState = ref.watch(gearProvider(widget.gearId));
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          (widget.title ?? l10n.gearCalendarYourHandoff).toUpperCase(),
          style: TextStyle(
            color: Colors.white.withAlpha(115),
            fontSize: 10,
            fontWeight: FontWeight.w800,
            letterSpacing: 0.8,
          ),
        ),
        const SizedBox(height: 9),
        _handoffSlot(context, gearState, isPickup: true),
        if (widget.showDropoff) ...[
          const SizedBox(height: 12),
          _handoffSlot(context, gearState, isPickup: false),
        ],
      ],
    );
  }

  Widget _handoffSlot(
    BuildContext context,
    GearState gearState, {
    required bool isPickup,
  }) {
    final l10n = context.l10n;
    // Loans pin the pickup to the booked range's first/last day; a giveaway has
    // no claimed range, so its pickup day is whatever the participants set (or
    // unset — then no day is shown).
    final DateTime? day;
    if (widget.showDropoff) {
      day = isPickup
          ? _toDay(_booking.startDateUnixSec.toInt())
          : _toDay(_booking.endDateUnixSec.toInt());
    } else {
      day = _booking.hasPickupTimeUnixSec()
          ? _toDay(_booking.pickupTimeUnixSec.toInt())
          : null;
    }
    final gearLocId = gearState.gearDetails?.locationId ?? '';
    final gearLocName = gearState.locationName ?? '';

    final locId =
        isPickup ? _booking.pickupLocationId : _booking.dropoffLocationId;
    final effectiveLocId = locId.isNotEmpty ? locId : gearLocId;
    final placeName = _locName(effectiveLocId, gearLocId, gearLocName);

    final timeUnix = isPickup
        ? (_booking.hasPickupTimeUnixSec()
            ? _booking.pickupTimeUnixSec.toInt()
            : null)
        : (_booking.hasDropoffTimeUnixSec()
            ? _booking.dropoffTimeUnixSec.toInt()
            : null);
    final timeText = timeUnix != null
        ? DateFormat('EEE h:mm a')
            .format(DateTime.fromMillisecondsSinceEpoch(timeUnix * 1000))
        : l10n.gearCalendarSetTime;

    final label = isPickup ? l10n.gearCalendarPickup : l10n.gearCalendarDropoff;
    final heading = day != null
        ? '$label · ${DateFormat('EEE MMM d').format(day)}'.toUpperCase()
        : label.toUpperCase();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.only(bottom: 9),
          child: Text(
            heading,
            style: TextStyle(
              color: Colors.white.withAlpha(115),
              fontSize: 10.5,
              fontWeight: FontWeight.w700,
              letterSpacing: 0.6,
            ),
          ),
        ),
        // IntrinsicHeight equalises the segments to the tallest content; the
        // SizedBox wrappers (not the Tappables) are the flex children, so stretch
        // never lands FlexParentData on a Semantics render object.
        IntrinsicHeight(
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              _seg(
                label: l10n.gearCalendarPlace,
                value:
                    placeName.isEmpty ? l10n.gearCalendarSetPlace : placeName,
                isSet: placeName.isNotEmpty,
                semanticsLabel: l10n.a11yGearEditHandoffPlace,
                onTap: () =>
                    _pickPlace(isPickup: isPickup, currentLocId: effectiveLocId),
              ),
              const SizedBox(width: 8),
              _seg(
                label: l10n.gearCalendarTime,
                value: timeText,
                isSet: timeUnix != null,
                semanticsLabel: l10n.a11yGearEditHandoffTime,
                onTap: () => _pickHandoffTime(
                  (t) => _setHandoff(
                    pickupTimeUnixSec: isPickup ? t : null,
                    dropoffTimeUnixSec: isPickup ? null : t,
                  ),
                ),
              ),
              const SizedBox(width: 8),
              _exportSeg(
                semanticsLabel: l10n.a11yGearExportHandoff,
                onTap: () => _exportHandoff(gearState, isPickup: isPickup),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _seg({
    required String label,
    required String value,
    required bool isSet,
    required String semanticsLabel,
    required VoidCallback onTap,
  }) {
    // The flex child is a SizedBox (a plain RenderBox), not the Tappable's
    // Semantics — putting FlexParentData on a Semantics render object trips a
    // framework assertion inside a scroll view.
    return Expanded(
      child: SizedBox(
        child: Tappable(
          semanticsLabel: semanticsLabel,
          onTap: onTap,
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 13, vertical: 9),
            decoration: BoxDecoration(
              color: Colors.white.withAlpha(15),
              borderRadius: BorderRadius.circular(12),
              border: Border.all(color: Colors.white.withAlpha(41)),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Text(
                  label.toUpperCase(),
                  style: TextStyle(
                    color: Colors.white.withAlpha(115),
                    fontSize: 9,
                    fontWeight: FontWeight.w700,
                    letterSpacing: 0.7,
                  ),
                ),
                const SizedBox(height: 3),
                Text(
                  value,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    color: isSet ? Colors.white : _mint,
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _exportSeg({
    required String semanticsLabel,
    required VoidCallback onTap,
  }) {
    return SizedBox(
      width: 46,
      child: Tappable(
        semanticsLabel: semanticsLabel,
        onTap: onTap,
        child: Container(
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: Colors.white.withAlpha(15),
            borderRadius: BorderRadius.circular(12),
            border: Border.all(color: Colors.white.withAlpha(41)),
          ),
          child:
              const Icon(Icons.event_outlined, size: 18, color: Colors.white),
        ),
      ),
    );
  }

  /// Resolves a location id to a short display name, falling back to the gear's
  /// own location (the default for a hand-off). Custom locations resolve
  /// asynchronously and cache.
  String _locName(String locId, String gearLocId, String gearName) {
    if (locId.isEmpty || locId == gearLocId) return gearName;
    final cached = _locNames[locId];
    if (cached != null) return cached;
    _resolveLoc(locId);
    return '…';
  }

  Future<void> _resolveLoc(String id) async {
    if (_locNames.containsKey(id) || _resolvingLoc.contains(id)) return;
    _resolvingLoc.add(id);
    try {
      final loc = await ref.read(locationRepositoryProvider).getLocation(id);
      final name = LocationFormatter.formatLocationNameShort(loc);
      if (!mounted) return;
      setState(() => _locNames[id] = name);
    } catch (_) {
      // Leave unresolved; the segment keeps showing the placeholder.
    } finally {
      _resolvingLoc.remove(id);
    }
  }

  Future<void> _pickHandoffTime(ValueChanged<int> onSetTime) async {
    final now = DateTime.now();
    final date = await showDatePicker(
      context: context,
      initialDate: now,
      firstDate: now.subtract(const Duration(days: 1)),
      lastDate: now.add(const Duration(days: 365)),
    );
    if (date == null || !mounted) return;
    final time = await showTimePicker(
      context: context,
      initialTime: TimeOfDay.fromDateTime(now),
    );
    if (time == null) return;
    final dt = DateTime(date.year, date.month, date.day, time.hour, time.minute);
    onSetTime(dt.millisecondsSinceEpoch ~/ 1000);
  }

  /// Opens the location picker for a hand-off place, defaulting to the current
  /// (gear) location, and persists the chosen place on the booking.
  Future<void> _pickPlace({
    required bool isPickup,
    required String currentLocId,
  }) async {
    final newId = await LocationPickerHelper.showLocationPicker(
      context: context,
      ref: ref,
      locationId: currentLocId.isEmpty ? null : currentLocId,
      showDirections: false,
      allowNonOwnerEdit: true,
    );
    if (newId == null || newId.isEmpty || !mounted) return;
    await _setHandoff(
      pickupLocationId: isPickup ? newId : null,
      dropoffLocationId: isPickup ? null : newId,
    );
  }

  Future<void> _setHandoff({
    int? pickupTimeUnixSec,
    int? dropoffTimeUnixSec,
    String? pickupLocationId,
    String? dropoffLocationId,
  }) async {
    final ok = await ref
        .read(gearBookingProvider(widget.gearId).notifier)
        .updateHandoff(
          bookingId: _booking.id,
          pickupTimeUnixSec: pickupTimeUnixSec,
          dropoffTimeUnixSec: dropoffTimeUnixSec,
          pickupLocationId: pickupLocationId,
          dropoffLocationId: dropoffLocationId,
        );
    if (!mounted || ok) return;
    final err = ref.read(gearBookingProvider(widget.gearId)).error;
    ToastHelper.showError(
      context,
      err != null
          ? RpcErrorHandler.localize(err, context.l10n)
          : context.l10n.gearCalendarGenericError,
    );
  }

  /// Exports a hand-off (pickup or drop-off) to the device calendar.
  Future<void> _exportHandoff(
    GearState gearState, {
    required bool isPickup,
  }) async {
    final l10n = context.l10n;
    if (kIsWeb) {
      ToastHelper.showError(context, l10n.webUnsupportedCalendar);
      return;
    }
    final gearName = gearState.gearDetails?.name ?? '';
    final timeUnix = isPickup
        ? (_booking.hasPickupTimeUnixSec()
            ? _booking.pickupTimeUnixSec.toInt()
            : null)
        : (_booking.hasDropoffTimeUnixSec()
            ? _booking.dropoffTimeUnixSec.toInt()
            : null);
    final DateTime fallbackDay;
    if (widget.showDropoff) {
      fallbackDay = isPickup
          ? _toDay(_booking.startDateUnixSec.toInt())
          : _toDay(_booking.endDateUnixSec.toInt());
    } else {
      fallbackDay = _dayKey(DateTime.now());
    }
    final start = timeUnix != null
        ? DateTime.fromMillisecondsSinceEpoch(timeUnix * 1000)
        : fallbackDay;
    final gearLocId = gearState.gearDetails?.locationId ?? '';
    final locId =
        isPickup ? _booking.pickupLocationId : _booking.dropoffLocationId;
    final placeName = _locName(
      locId.isNotEmpty ? locId : gearLocId,
      gearLocId,
      gearState.locationName ?? '',
    );

    await CalendarHelper.addTimedEvent(
      title: isPickup
          ? l10n.gearCalendarPickupTitle(gearName)
          : l10n.gearCalendarDropoffTitle(gearName),
      start: start,
      end: timeUnix != null ? start.add(const Duration(hours: 1)) : null,
      location: placeName.isEmpty || placeName == '…' ? null : placeName,
      allDay: timeUnix == null,
    );
    if (!mounted) return;
    ToastHelper.showSuccess(context, l10n.gearCalendarExportedToast);
  }
}
