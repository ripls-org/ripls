import 'package:logging/logging.dart';

final _log = Logger('LogoutDiag');

/// LogoutDiagnostics collects a timestamped, in-memory ring buffer of events
/// that happen during and immediately after a logout. The goal is to make the
/// "RenderIgnorePointer was mutated in performLayout" crash documented in
/// issue #2158 observable from a single dumped trace, instead of having to
/// piece together unrelated log lines from across the app.
///
/// Each [trace] call records (a) wall-clock time and (b) milliseconds since
/// the most recent [markLogoutStarted] (or null if no logout has been
/// observed yet). The ring buffer is capped at [_maxEvents] so it can stay
/// live in release builds without unbounded memory growth.
///
/// Wired into:
/// - `services/auth_state.dart` — marks the logout window and traces every
///   teardown step.
/// - `services/providers/cache_providers.dart` — traces every
///   `*CacheInvalidationNotifier.notify()` call.
/// - `services/providers/experience_providers.dart`,
///   `request_providers.dart`, `gear_providers.dart` — traces every
///   repository-level `_on*Invalidated` callback firing.
/// - `presentation/viewmodels/experience_needs_view_model.dart` and
///   `request_needs_view_model.dart` — traces mutation start, post-await
///   resumption, and `_loggedOut` guard hits.
/// - `main.dart` — `FlutterError.onError` dumps the trace whenever a
///   layout-class error matches the documented #2158 signature.
class LogoutDiagnostics {
  LogoutDiagnostics._();

  static const int _maxEvents = 300;
  static final List<_TraceEvent> _events = <_TraceEvent>[];
  static DateTime? _logoutStartedAt;
  static DateTime? _logoutFinishedAt;
  static int _logoutCount = 0;

  /// True while we are within [window] of a logout starting. Defaults to a
  /// 5-second window — generous enough to catch a late-firing RPC that
  /// resolves seconds after the user taps logout.
  static bool inLogoutWindow({
    Duration window = const Duration(seconds: 5),
  }) {
    final started = _logoutStartedAt;
    if (started == null) return false;
    return DateTime.now().difference(started) <= window;
  }

  /// Milliseconds since the most recent logout started, or `null` if no
  /// logout has been observed yet.
  static int? msSinceLogoutStart() {
    final started = _logoutStartedAt;
    if (started == null) return null;
    return DateTime.now().difference(started).inMilliseconds;
  }

  /// Marks the start of a logout. We do NOT clear the ring buffer so the
  /// dump can still show pre-logout context (e.g. the mutation BEGIN event
  /// from a tap that races logout). Natural eviction keeps the buffer size
  /// bounded.
  static void markLogoutStarted() {
    _logoutStartedAt = DateTime.now();
    _logoutFinishedAt = null;
    _logoutCount++;
    _record('LOGOUT_STARTED', 'logoutCount=$_logoutCount');
  }

  /// Marks the end of a logout. We do NOT clear [_logoutStartedAt] so the
  /// post-logout window detection ([inLogoutWindow]) still fires for events
  /// that happen in the next few seconds (the documented race window).
  static void markLogoutFinished() {
    _logoutFinishedAt = DateTime.now();
    _record('LOGOUT_FINISHED');
  }

  /// Record a single trace event. Cheap; always-on (the ring buffer caps at
  /// [_maxEvents]). Pass a short `label` (UPPER_SNAKE) and an optional
  /// free-form `details` string.
  static void trace(String label, [String? details]) {
    _record(label, details);
  }

  /// Returns a multi-line dump of the current ring buffer plus header. Safe
  /// to call from a crash handler — does not allocate beyond the StringBuffer.
  static String dump() {
    final sb = StringBuffer();
    sb.writeln('=== LogoutDiagnostics dump ===');
    sb.writeln('logoutCount=$_logoutCount');
    sb.writeln('logoutStartedAt=$_logoutStartedAt');
    sb.writeln('logoutFinishedAt=$_logoutFinishedAt');
    sb.writeln('eventCount=${_events.length}');
    sb.writeln('--- events (oldest first) ---');
    for (final e in _events) {
      sb.writeln('  $e');
    }
    sb.writeln('=== end ===');
    return sb.toString();
  }

  static void _record(String label, [String? details]) {
    final now = DateTime.now();
    final ms = _logoutStartedAt == null
        ? null
        : now.difference(_logoutStartedAt!).inMilliseconds;
    final event = _TraceEvent(now, ms, label, details);
    _events.add(event);
    if (_events.length > _maxEvents) {
      _events.removeAt(0);
    }
    // Mirror to the standard logger so the events also appear in adb logcat /
    // Xcode console at FINE/INFO level. We use INFO during the logout window
    // (so the trace is visible without enabling FINE) and FINE otherwise to
    // avoid noise during normal operation.
    final msg = '[$label]${details != null ? " $details" : ""}'
        '${ms != null ? " (+${ms}ms)" : ""}';
    if (inLogoutWindow()) {
      _log.info(msg);
    } else {
      _log.fine(msg);
    }
  }
}

class _TraceEvent {
  _TraceEvent(this.at, this.msSinceLogoutStart, this.label, this.details);

  final DateTime at;
  final int? msSinceLogoutStart;
  final String label;
  final String? details;

  @override
  String toString() {
    final tag = msSinceLogoutStart != null
        ? '+${msSinceLogoutStart}ms'
        : 'pre-logout';
    final detailPart = details != null ? ' | $details' : '';
    return '${at.toIso8601String()} [$tag] $label$detailPart';
  }
}
