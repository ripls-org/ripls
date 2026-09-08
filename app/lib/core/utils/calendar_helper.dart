import 'package:add_2_calendar/add_2_calendar.dart';
import 'package:ripls/data/gen/ripls/api/experience.pb.dart';
import 'package:timezone/timezone.dart' as tz;

/// CalendarHelper provides utility methods for exporting experiences to calendar apps.
///
/// Supports single events, multi-day events, and all-day events.
/// Uses the add_2_calendar package to trigger native calendar integration.
class CalendarHelper {
  /// Adds an experience to the device calendar.
  ///
  /// Converts the experience time to a calendar event and opens the native
  /// calendar app for the user to save it.
  ///
  /// Parameters:
  /// - [experience]: The experience to add to calendar
  /// - [riplsUrl]: Optional public Ripls URL to embed in the event description
  ///   so the calendar entry links back into the app.
  /// - [locationDisplay]: Optional human-readable address (e.g. resolved from
  ///   the experience's location_id). Preferred over the experience's raw
  ///   lat/lng for the calendar event's location field.
  ///
  /// Returns true if the calendar app was opened successfully, false otherwise.
  static Future<bool> addToCalendar(
    Experience experience, {
    String? riplsUrl,
    String? locationDisplay,
  }) async {
    final time = experience.time;

    // Handle TBD events - cannot add to calendar
    if (!time.hasSpecific() && !time.hasRange()) {
      return false;
    }

    final event = _buildCalendarEvent(
      experience,
      riplsUrl: riplsUrl,
      locationDisplay: locationDisplay,
    );
    if (event == null) {
      return false;
    }

    return Add2Calendar.addEvent2Cal(event);
  }

  /// Adds a single timed (or all-day) event to the device calendar. Generic
  /// entry point used by non-experience flows (e.g. gear booking hand-offs).
  ///
  /// Opens the native calendar app pre-filled with the event. Returns true if
  /// the calendar app was opened. Not supported on web (the underlying plugin
  /// has no web implementation) — callers should guard with kIsWeb.
  static Future<bool> addTimedEvent({
    required String title,
    required DateTime start,
    DateTime? end,
    String? location,
    String? description,
    bool allDay = false,
  }) async {
    final event = Event(
      title: title,
      description: description,
      location: location,
      startDate: start,
      endDate: end ?? start.add(const Duration(hours: 1)),
      allDay: allDay,
      iosParams: IOSParams(reminder: const Duration(hours: 1)),
      androidParams: AndroidParams(emailInvites: []),
    );
    return Add2Calendar.addEvent2Cal(event);
  }

  /// Composes the calendar event description from the experience description
  /// and an optional Ripls URL.
  static String? composeDescription(String description, String? riplsUrl) {
    final hasDesc = description.isNotEmpty;
    final hasUrl = riplsUrl != null && riplsUrl.isNotEmpty;
    if (hasDesc && hasUrl) return '$description\n\nView in Ripls: $riplsUrl';
    if (hasDesc) return description;
    if (hasUrl) return 'View in Ripls: $riplsUrl';
    return null;
  }

  /// Builds a calendar Event from an Experience.
  ///
  /// Handles:
  /// - Single-point events (SpecificTime)
  /// - Multi-day events (TimeRange)
  /// - All-day events
  /// - Events with duration
  ///
  /// Returns null if the experience time is TBD or invalid.
  static Event? _buildCalendarEvent(
    Experience experience, {
    String? riplsUrl,
    String? locationDisplay,
  }) {
    final time = experience.time;

    if (time.hasSpecific()) {
      return _buildFromSpecificTime(
        experience,
        riplsUrl: riplsUrl,
        locationDisplay: locationDisplay,
      );
    } else if (time.hasRange()) {
      return _buildFromTimeRange(
        experience,
        riplsUrl: riplsUrl,
        locationDisplay: locationDisplay,
      );
    }

    return null; // TBD events
  }

  /// Builds a calendar event from SpecificTime.
  static Event _buildFromSpecificTime(
    Experience experience, {
    String? riplsUrl,
    String? locationDisplay,
  }) {
    final specific = experience.time.specific;
    final unixSec = specific.unixTimestampSec.toInt();

    // Get timezone location
    tz.Location location;
    try {
      location = specific.timezone.isNotEmpty
          ? tz.getLocation(specific.timezone)
          : tz.local;
    } catch (e) {
      location = tz.local;
    }

    // Convert unix timestamp to TZDateTime
    final startTime = tz.TZDateTime.fromMillisecondsSinceEpoch(
      location,
      unixSec * 1000,
    );

    // Calculate end time from duration
    DateTime? endTime;
    if (specific.durationMinutes > 0) {
      endTime = startTime.add(Duration(minutes: specific.durationMinutes));
    }

    return Event(
      title: experience.name,
      description: composeDescription(experience.description, riplsUrl),
      location: formatLocationForCalendar(experience, locationDisplay),
      startDate: startTime,
      endDate: endTime ?? startTime.add(const Duration(hours: 1)),
      allDay: specific.isAllDay,
      iosParams: IOSParams(
        reminder: const Duration(hours: 1),
      ),
      androidParams: AndroidParams(
        emailInvites: [],
      ),
    );
  }

  /// Builds a calendar event from TimeRange.
  static Event _buildFromTimeRange(
    Experience experience, {
    String? riplsUrl,
    String? locationDisplay,
  }) {
    final range = experience.time.range;
    final startUnixSec = range.startUnixSec.toInt();
    final endUnixSec = range.endUnixSec.toInt();

    // Use the range's timezone when set, mirroring _buildFromSpecificTime.
    tz.Location location;
    try {
      location =
          range.timezone.isNotEmpty ? tz.getLocation(range.timezone) : tz.local;
    } catch (e) {
      location = tz.local;
    }

    final startTime = tz.TZDateTime.fromMillisecondsSinceEpoch(
      location,
      startUnixSec * 1000,
    );
    final endTime = tz.TZDateTime.fromMillisecondsSinceEpoch(
      location,
      endUnixSec * 1000,
    );

    return Event(
      title: experience.name,
      description: composeDescription(experience.description, riplsUrl),
      location: formatLocationForCalendar(experience, locationDisplay),
      startDate: startTime,
      endDate: endTime,
      allDay: range.isAllDay,
      iosParams: IOSParams(
        reminder: const Duration(hours: 1),
      ),
      androidParams: AndroidParams(
        emailInvites: [],
      ),
    );
  }

  /// Formats the experience location for calendar display.
  ///
  /// Prefers a caller-provided human-readable address (resolved from the
  /// experience's location_id) so the calendar entry shows e.g.
  /// "Empower Field at Mile High, 1701 Bryant St, Denver, CO" instead of raw
  /// coordinates. Falls back to lat/lng coordinates only when no address is
  /// available, and returns null when there is no location information at all.
  static String? formatLocationForCalendar(
    Experience experience,
    String? locationDisplay,
  ) {
    if (locationDisplay != null && locationDisplay.isNotEmpty) {
      return locationDisplay;
    }

    // Fallback: lat/lng coordinates, only if non-zero.
    if (experience.latitudeDeg != 0 || experience.longitudeDeg != 0) {
      return '${experience.latitudeDeg.toStringAsFixed(6)}, ${experience.longitudeDeg.toStringAsFixed(6)}';
    }

    return null;
  }
}
