import 'package:intl/intl.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart';
import 'package:timezone/timezone.dart' as tz;

/// DateTimeFormatter provides utility methods for formatting dates and times
/// in human-readable formats.
class DateTimeFormatter {
  /// Formats a return date as a human-readable string relative to today.
  ///
  /// Returns strings like:
  /// - "Return today" for same day
  /// - "Return tomorrow" for next day
  /// - "Return in X days" for future dates
  /// - "Return X days ago" for past dates
  ///
  /// The [expectedReturnUnixSec] should be a Unix timestamp in seconds.
  static String formatReturnDate(int expectedReturnUnixSec) {
    final returnDate = DateTime.fromMillisecondsSinceEpoch(
      expectedReturnUnixSec * 1000,
    );
    final now = DateTime.now();
    // Use UTC to avoid DST affecting calendar day arithmetic.
    final todayUtc = DateTime.utc(now.year, now.month, now.day);
    final dateOnlyUtc = DateTime.utc(
      returnDate.year,
      returnDate.month,
      returnDate.day,
    );

    final difference = dateOnlyUtc.difference(todayUtc).inDays;

    if (difference == 0) {
      return 'Return today';
    } else if (difference == 1) {
      return 'Return tomorrow';
    } else if (difference > 0) {
      return 'Return in $difference days';
    } else {
      return 'Return ${difference.abs()} days ago';
    }
  }

  /// Formats the duration between two dates as a human-readable string.
  ///
  /// Returns strings like:
  /// - "Same day" for 0 days
  /// - "1 day" for 1 day
  /// - "X days" for < 7 days
  /// - "X week(s)" for >= 7 days and < 30 days
  /// - "X month(s)" for >= 30 days
  ///
  /// Both [startUnixSec] and [endUnixSec] should be Unix timestamps in seconds.
  static String formatDuration(int startUnixSec, int endUnixSec) {
    final startDate = DateTime.fromMillisecondsSinceEpoch(startUnixSec * 1000);
    final endDate = DateTime.fromMillisecondsSinceEpoch(endUnixSec * 1000);
    final duration = endDate.difference(startDate).inDays;

    if (duration == 0) {
      return 'Same day';
    } else if (duration == 1) {
      return '1 day';
    } else if (duration < 7) {
      return '$duration days';
    } else if (duration < 30) {
      final weeks = (duration / 7).round();
      return '$weeks ${weeks == 1 ? "week" : "weeks"}';
    } else {
      final months = (duration / 30).round();
      return '$months ${months == 1 ? "month" : "months"}';
    }
  }

  /// Formats a loan duration with a more descriptive label.
  ///
  /// This is an alias for [formatDuration] with more context-specific naming.
  static String formatLoanDuration(int requestedAt, int expectedReturnUnixSec) {
    return formatDuration(requestedAt, expectedReturnUnixSec);
  }

  /// Formats a booking's inclusive day window as a compact date range.
  ///
  /// Returns strings like:
  /// - "Jul 18" for a single-day booking
  /// - "Jul 18–20" when the range stays within one month
  /// - "Jul 30 – Aug 2" when the range crosses a month boundary
  ///
  /// Both [startUnixSec] and [endUnixSec] should be Unix timestamps in seconds.
  static String formatBookingWindow(int startUnixSec, int endUnixSec) {
    final start = DateTime.fromMillisecondsSinceEpoch(startUnixSec * 1000);
    final end = DateTime.fromMillisecondsSinceEpoch(endUnixSec * 1000);
    final startLabel = DateFormat('MMM d').format(start);
    if (start.year == end.year &&
        start.month == end.month &&
        start.day == end.day) {
      return startLabel;
    }
    if (start.year == end.year && start.month == end.month) {
      return '$startLabel–${end.day}';
    }
    return '$startLabel – ${DateFormat('MMM d').format(end)}';
  }

  
  /// Formats a start time + duration as a wall-clock range ("9:00 – 10:00 AM"),
  /// omitting the start meridiem when it matches the end's (e.g. "9:00 – 10:00 AM"
  /// but "11:00 AM – 1:00 PM"). Returns just the start time when
  /// [durationMinutes] is non-positive, and an empty string when [startUnixSec]
  /// is zero.
  static String formatWallClockTimeRange(int startUnixSec, int durationMinutes) {
    if (startUnixSec == 0) return '';
    final start = DateTime.fromMillisecondsSinceEpoch(startUnixSec * 1000);
    if (durationMinutes <= 0) return DateFormat('h:mm a').format(start);
    final end = start.add(Duration(minutes: durationMinutes));
    final startAmPm = start.hour >= 12 ? 'PM' : 'AM';
    final endAmPm = end.hour >= 12 ? 'PM' : 'AM';
    final startTime = startAmPm == endAmPm
        ? DateFormat('h:mm').format(start)
        : DateFormat('h:mm a').format(start);
    final endTime = DateFormat('h:mm a').format(end);
    return '$startTime – $endTime';
  }

  /// Formats a timestamp as "Day, Month Date" (e.g., "Monday, Jan 15").
  ///
  /// The [unixSec] should be a Unix timestamp in seconds.
  static String formatDayAndDate(int unixSec) {
    final date = DateTime.fromMillisecondsSinceEpoch(unixSec * 1000);
    return DateFormat('EEEE, MMM d').format(date);
  }

  
  /// Formats an ExperienceTime as a human-readable string.
  ///
  /// Preferentially shows the informal_description if available (e.g., "next weekend").
  /// Falls back to formatted dates for specific times or ranges.
  ///
  /// Returns strings like:
  /// - "next weekend" (informal_description)
  /// - "tomorrow afternoon" (informal_description)
  /// - "Jan 15 at 2:30 PM" (formatted specific time)
  /// - "Jan 15-20" (formatted time range)
  /// - "TBD" (to-be-determined times)
  static String formatExperienceTime(ExperienceTime? time) {
    if (time == null) return 'TBD';

    // Prefer informal description when available
    if (time.informalDescription.isNotEmpty) {
      return time.informalDescription;
    }

    if (time.hasSpecific()) {
      return _formatSpecificTime(time.specific);
    } else if (time.hasRange()) {
      return _formatTimeRange(time.range);
    } else {
      return 'TBD';
    }
  }

  /// Formats a specific time with date and time, e.g. "May 10, 10:30 AM".
  static String _formatSpecificTime(SpecificTime specific) {
    final date = DateTime.fromMillisecondsSinceEpoch(
      specific.unixTimestampSec.toInt() * 1000,
    );
    return DateFormat('MMM d, h:mm a').format(date);
  }

  /// Formats a time range with start and end dates.
  static String _formatTimeRange(TimeRange range) {
    if (range.description.isNotEmpty) {
      return range.description;
    }

    final startDate = DateTime.fromMillisecondsSinceEpoch(
      range.startUnixSec.toInt() * 1000,
    );
    final endDate = DateTime.fromMillisecondsSinceEpoch(
      range.endUnixSec.toInt() * 1000,
    );

    // Same month: "Jan 15-20"
    if (startDate.month == endDate.month && startDate.year == endDate.year) {
      final month = DateFormat('MMM').format(startDate);
      return '$month ${startDate.day}-${endDate.day}';
    }

    // Different months: "Jan 15 - Feb 2"
    final startFormatted = DateFormat('MMM d').format(startDate);
    final endFormatted = DateFormat('MMM d').format(endDate);
    return '$startFormatted - $endFormatted';
  }

  /// Formats a Unix timestamp as a "time ago" string relative to now.
  ///
  /// Returns:
  /// - `null` when [unixSec] is `<= 0` (uninitialized timestamp).
  /// - `'just now'` when the elapsed time is under one minute — a literal
  ///   "0m ago" reads broken on status chips (#2724).
  /// - `'${m}m ago'` when the elapsed time is under one hour.
  /// - `'${h}h ago'` when the elapsed time is under one day.
  /// - `'${d}d ago'` when the elapsed time is under ~a month.
  /// - `'${mo}mo ago'` / `'${y}y ago'` beyond that — the unit ladder caps
  ///   at sane granularity instead of counting hundreds of days.
  ///
  /// Returning null for non-positive input prevents the "1970-01-01 →
  /// 20000+ days ago" rendering that occurs when an unset Unix timestamp
  /// is passed straight into [DateTime.fromMillisecondsSinceEpoch].
  static String? formatTimeAgo(int unixSec) {
    if (unixSec <= 0) return null;
    final timestamp = DateTime.fromMillisecondsSinceEpoch(unixSec * 1000);
    final age = DateTime.now().difference(timestamp);
    if (age.inDays >= 365) return '${age.inDays ~/ 365}y ago';
    if (age.inDays >= 30) return '${age.inDays ~/ 30}mo ago';
    if (age.inDays > 0) return '${age.inDays}d ago';
    if (age.inHours > 0) return '${age.inHours}h ago';
    if (age.inMinutes >= 1) return '${age.inMinutes}m ago';
    return 'just now';
  }

  /// Formats a Unix timestamp as relative time from now.
  ///
  /// For future events, returns abbreviated relative time strings:
  /// - "In 5min" for < 1 hour
  /// - "In 1hr 30min" for < 2 hours (shows both hours and minutes)
  /// - "In 3hrs" for >= 2 hours and < 24 hours
  /// - "tomorrow" for exactly 1 day
  /// - "In 5 days" for < 7 days
  /// - "Aug 15" for >= 7 days
  ///
  /// For past events, returns the provided status string (e.g., "Completed",
  /// "Cancelled"). If no status is provided, returns "Completed" by default.
  ///
  /// The [eventUnixSec] should be a Unix timestamp in seconds.
  /// The [status] parameter is optional and used for past events only.
  static String formatRelativeEventTime(int eventUnixSec, {String? status}) {
    final eventTime = DateTime.fromMillisecondsSinceEpoch(eventUnixSec * 1000);
    final now = DateTime.now();
    final difference = eventTime.difference(now);

    if (difference.isNegative) {
      // Past event - show status (Completed, Cancelled, etc.)
      return status ?? 'Completed';
    }

    // Use abbreviated formats with "In " prefix to minimize chip size
    if (difference.inMinutes < 60) {
      return 'In ${difference.inMinutes}min';
    } else if (difference.inHours < 2) {
      // For events under 2 hours away, show hours and minutes
      final hours = difference.inHours;
      final minutes = difference.inMinutes % 60;
      return 'In ${hours}hr ${minutes}min';
    } else if (difference.inHours < 24) {
      return 'In ${difference.inHours}hrs';
    } else if (difference.inDays == 1) {
      return 'tomorrow';
    } else if (difference.inDays < 7) {
      return 'In ${difference.inDays} days';
    } else {
      return DateFormat('MMM d').format(eventTime); // "Aug 15"
    }
  }

  /// Formats a Unix timestamp as a relative offset from now, e.g.
  /// "3d from now", "2h 15m from now", "5m from now", or "3d ago" for a
  /// past time. The unit ladder caps at sane granularity (#2724): months
  /// once ≥30 days ("2 months ago" instead of "64d 4h ago"), whole days once
  /// ≥1 day, and hours+minutes below that. A future time floors at "1m from
  /// now"; a past time under a minute reads "just now". Returns null for
  /// non-positive [unixSec] (unset timestamps).
  static String? formatRelativeFromNow(int unixSec) {
    if (unixSec <= 0) return null;
    final target = DateTime.fromMillisecondsSinceEpoch(unixSec * 1000);
    final diff = target.difference(DateTime.now());
    final isPast = diff.isNegative;
    final abs = diff.abs();
    final days = abs.inDays;
    final hours = abs.inHours % 24;
    final minutes = abs.inMinutes % 60;

    final String magnitude;
    if (days >= 30) {
      final months = days ~/ 30;
      magnitude = months == 1 ? '1 month' : '$months months';
    } else if (days > 0) {
      magnitude = '${days}d';
    } else if (hours > 0) {
      magnitude = minutes > 0 ? '${hours}h ${minutes}m' : '${hours}h';
    } else if (minutes < 1) {
      return isPast ? 'just now' : '1m from now';
    } else {
      magnitude = '${minutes}m';
    }
    return isPast ? '$magnitude ago' : '$magnitude from now';
  }

  /// Normalizes a server-rendered compact time-ago token ("0m", "5m", "2h",
  /// "64d" — see the API's `last_message_time_ago` fields) for display:
  /// sub-minute tokens ("0m") return null so callers can substitute a
  /// localized "just now", and day counts collapse to months/years once the
  /// unit ladder passes 30/365 days ("64d" → "2mo") so stale threads read
  /// humanized (#2724). Unrecognized tokens pass through unchanged.
  static String? normalizeCompactTimeAgo(String token) {
    final match = RegExp(r'^(\d+)([mhd])$').firstMatch(token.trim());
    if (match == null) return token.isEmpty ? null : token;
    final value = int.parse(match.group(1)!);
    final unit = match.group(2)!;
    if (unit == 'm' && value == 0) return null;
    if (unit == 'd' && value >= 365) return '${value ~/ 365}y';
    if (unit == 'd' && value >= 30) return '${value ~/ 30}mo';
    return '$value$unit';
  }

  
  /// Formats a Unix timestamp as relative time with timezone conversion.
  ///
  /// Converts event time from [eventTimezone] to [userTimezone] (or device timezone
  /// if not specified) before calculating relative time difference.
  ///
  /// For future events, returns abbreviated relative time strings:
  /// - "In 5min" for < 1 hour
  /// - "In 1hr 30min" for < 2 hours (shows both hours and minutes)
  /// - "In 3hrs" for >= 2 hours and < 24 hours
  /// - "tomorrow" for exactly 1 day
  /// - "In 5 days" for < 7 days
  /// - "Aug 15" for >= 7 days
  ///
  /// For all-day events ([isAllDay] = true), returns date only without time component.
  ///
  /// For past events, returns the provided [status] string (e.g., "Completed",
  /// "Cancelled"). If no status is provided, returns "Completed" by default.
  ///
  /// Parameters:
  /// - [eventUnixSec]: Unix timestamp in seconds for the event
  /// - [eventTimezone]: IANA timezone of the event (e.g., "America/Los_Angeles")
  /// - [userTimezone]: IANA timezone for display (e.g., "America/New_York")
  /// - [isAllDay]: Whether this is an all-day event (shows date only)
  /// - [status]: Optional status text for past events
  ///
  /// Returns formatted string representing relative time or status.
  static String formatRelativeEventTimeWithTimezone(
    int eventUnixSec, {
    String? eventTimezone,
    String? userTimezone,
    bool isAllDay = false,
    String? status,
  }) {
    // Get timezone locations with fallback to local timezone for invalid strings
    tz.Location eventLocation;
    try {
      eventLocation = eventTimezone != null
          ? tz.getLocation(eventTimezone)
          : tz.local;
    } catch (e) {
      // Invalid timezone string (e.g., "MST" instead of "America/Denver")
      // Fall back to local timezone
      eventLocation = tz.local;
    }

    tz.Location userLocation;
    try {
      userLocation = userTimezone != null
          ? tz.getLocation(userTimezone)
          : tz.local;
    } catch (e) {
      // Invalid timezone string - fall back to local timezone
      userLocation = tz.local;
    }

    // Convert event time to user's timezone
    final eventTime = tz.TZDateTime.fromMillisecondsSinceEpoch(
      eventLocation,
      eventUnixSec * 1000,
    );
    final eventTimeInUserTz = tz.TZDateTime.from(eventTime, userLocation);

    // Get current time in user's timezone
    final now = tz.TZDateTime.now(userLocation);
    final difference = eventTimeInUserTz.difference(now);

    // Handle past events
    if (difference.isNegative) {
      return status ?? 'Completed';
    }

    // For all-day events, show date only without time component
    if (isAllDay) {
      if (difference.inDays == 0) {
        return 'today';
      } else if (difference.inDays == 1) {
        return 'tomorrow';
      } else if (difference.inDays < 7) {
        return 'In ${difference.inDays} days';
      } else {
        return DateFormat('MMM d').format(eventTimeInUserTz);
      }
    }

    // Use abbreviated formats with "In " prefix to minimize chip size
    if (difference.inMinutes < 60) {
      return 'In ${difference.inMinutes}min';
    } else if (difference.inHours < 2) {
      // For events under 2 hours away, show hours and minutes
      final hours = difference.inHours;
      final minutes = difference.inMinutes % 60;
      return 'In ${hours}hr ${minutes}min';
    } else if (difference.inHours < 24) {
      return 'In ${difference.inHours}hrs';
    } else if (difference.inDays == 1) {
      return 'tomorrow';
    } else if (difference.inDays < 7) {
      return 'In ${difference.inDays} days';
    } else {
      return DateFormat('MMM d').format(eventTimeInUserTz);
    }
  }
}
