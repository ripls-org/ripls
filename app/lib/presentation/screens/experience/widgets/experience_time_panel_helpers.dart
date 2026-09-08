import 'dart:async';

import 'package:flutter/widgets.dart';
import 'package:ripls/core/utils/date_time_formatter.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/gen/ripls/api/portfolio.pb.dart'
    show DailyItemType, HomeUpNextEntry, HourForecast;
import 'package:ripls/data/gen/ripls/api/time.pb.dart'
    show ExperienceTime, TimeProposal, TimeVote, TimeVoteStatus;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/viewmodels/time_modal_state.dart';

/// The item-detail route type for a Home-feed agenda entry, or null when the
/// kind has no drill-in surface (so the When-screen agenda row isn't tappable).
String? _agendaRouteType(DailyItemType t) => switch (t) {
      DailyItemType.DAILY_ITEM_TYPE_TRANSFER ||
      DailyItemType.DAILY_ITEM_TYPE_GIVEAWAY =>
        'gear',
      DailyItemType.DAILY_ITEM_TYPE_EXPERIENCE => 'experience',
      DailyItemType.DAILY_ITEM_TYPE_REQUEST => 'request',
      _ => null,
    };

/// Whether the agenda entry [e] opens an item screen on tap.
bool canOpenAgendaItem(HomeUpNextEntry e) =>
    e.contentId.isNotEmpty && _agendaRouteType(e.itemType) != null;

/// Opens [e]'s gear / experience / request screen, sliding in from the right
/// (the app's standard item-detail navigation).
void openAgendaItem(BuildContext context, HomeUpNextEntry e) {
  final type = _agendaRouteType(e.itemType);
  if (type == null || e.contentId.isEmpty) return;
  unawaited(NavigationHelpers.pushToItemScreen(
    context: context,
    itemId: e.contentId,
    itemType: type,
  ));
}

/// Day-only key (local midnight) for [d].
DateTime timeDayKey(DateTime d) => DateTime(d.year, d.month, d.day);

/// The confirmed event's time on the When screen — the locked (confirmed)
/// proposal if any, else the set event time. Null when nothing is confirmed.
ExperienceTime? confirmedExperienceTime(TimeModalData data) {
  final locked = data.lockedProposal;
  if (locked != null && locked.hasTime()) return locked.time;
  final t = data.eventTime;
  if (t != null && (t.hasSpecific() || t.hasRange())) return t;
  return null;
}

/// Window start (Unix sec) of an [ExperienceTime], or null.
int? timeStartSec(ExperienceTime? t) {
  if (t == null) return null;
  if (t.hasSpecific()) return t.specific.unixTimestampSec.toInt();
  if (t.hasRange() && t.range.hasStartUnixSec()) {
    return t.range.startUnixSec.toInt();
  }
  return null;
}

/// Window end (Unix sec); defaults to a one-hour window when no duration/end.
int? timeEndSec(ExperienceTime? t) {
  if (t == null) return null;
  if (t.hasSpecific()) {
    final s = t.specific.unixTimestampSec.toInt();
    final mins = t.specific.durationMinutes;
    return s + (mins > 0 ? mins : 60) * 60;
  }
  if (t.hasRange() && t.range.hasEndUnixSec()) return t.range.endUnixSec.toInt();
  if (t.hasRange() && t.range.hasStartUnixSec()) {
    return t.range.startUnixSec.toInt() + 3600;
  }
  return null;
}

/// The committed event day (confirmed, non-poll), or null.
DateTime? confirmedEventDay(TimeModalData data) {
  if (data.timePollActive) return null;
  final s = timeStartSec(confirmedExperienceTime(data));
  if (s == null) return null;
  return timeDayKey(DateTime.fromMillisecondsSinceEpoch(s * 1000));
}

/// The last day the confirmed event spans (its end day), or null. Equals
/// [confirmedEventDay] for a single-day event; later for a multi-day one.
DateTime? confirmedEventEndDay(TimeModalData data) {
  if (data.timePollActive) return null;
  final ct = confirmedExperienceTime(data);
  final start = timeStartSec(ct);
  if (start == null) return null;
  // The window's end is exclusive (start + duration); a midnight end belongs to
  // the previous day, so step back one second before taking the day.
  final end = timeEndSec(ct) ?? start;
  final endDay = end > start ? end - 1 : start;
  return timeDayKey(DateTime.fromMillisecondsSinceEpoch(endDay * 1000));
}

/// A short ~6-hour slice of [hours] centred on [focusHour], for the When
/// screen's window weather strip.
List<HourForecast> windowHours(List<HourForecast> hours, int focusHour) {
  final lo = focusHour - 2;
  final hi = focusHour + 3;
  return [
    for (final h in hours)
      if (_localHour(h.timeUnixSec.toInt()) >= lo &&
          _localHour(h.timeUnixSec.toInt()) <= hi)
        h,
  ];
}

int _localHour(int unixSec) =>
    DateTime.fromMillisecondsSinceEpoch(unixSec * 1000).hour;

/// A time-poll proposal resolved for display in the When panel: a day-and-date
/// [name] ("Saturday, Jun 13"), a [subtitle] ("9:00 – 11:00 AM"), and the
/// underlying [dateTime] (for the calendar marker). Time lives inline on the
/// proposal, so resolution is synchronous (no provider needed, unlike Where).
class TimeOptionDisplay {
  final TimeProposal proposal;
  final DateTime? dateTime;
  final String name;
  final String subtitle;

  const TimeOptionDisplay({
    required this.proposal,
    required this.dateTime,
    required this.name,
    required this.subtitle,
  });
}

/// Resolves a [TimeProposal]'s [ExperienceTime] into display strings + a date.
TimeOptionDisplay resolveTimeOption(TimeProposal p) =>
    resolveExperienceTime(p.time, proposal: p);

/// Resolves a bare [ExperienceTime] (e.g. the confirmed event time) into
/// display strings + a date. [proposal] is carried through when available.
TimeOptionDisplay resolveExperienceTime(ExperienceTime t, {TimeProposal? proposal}) {
  final p = proposal ?? TimeProposal();
  if (t.hasSpecific()) {
    final sec = t.specific.unixTimestampSec.toInt();
    final dt = DateTime.fromMillisecondsSinceEpoch(sec * 1000);
    // Show an explicit start–end window ("9:00 – 10:00 AM") rather than an
    // opaque duration label ("1 hour"), so the actual end time is clear.
    final subtitle = t.specific.isAllDay
        ? 'All day'
        : DateTimeFormatter.formatWallClockTimeRange(
            sec, t.specific.durationMinutes);
    return TimeOptionDisplay(
      proposal: p,
      dateTime: dt,
      name: DateTimeFormatter.formatDayAndDate(sec),
      subtitle: subtitle,
    );
  }
  if (t.hasRange() && t.range.hasStartUnixSec()) {
    final sec = t.range.startUnixSec.toInt();
    final dt = DateTime.fromMillisecondsSinceEpoch(sec * 1000);
    return TimeOptionDisplay(
      proposal: p,
      dateTime: dt,
      name: DateTimeFormatter.formatDayAndDate(sec),
      subtitle: DateTimeFormatter.formatExperienceTime(t),
    );
  }
  return TimeOptionDisplay(
    proposal: p,
    dateTime: null,
    name: DateTimeFormatter.formatExperienceTime(t),
    subtitle: '',
  );
}

/// Distinct YES-voters on a time proposal, as users for the avatar stack.
List<User> timeYesVoters(List<TimeVote> votes) => [
      for (final v in votes)
        if (v.status == TimeVoteStatus.TIME_VOTE_STATUS_YES) v.user,
    ];
