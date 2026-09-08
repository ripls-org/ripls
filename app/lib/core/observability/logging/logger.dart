import 'package:logging/logging.dart';
import 'package:ripls/core/observability/breadcrumbs.dart';
import 'package:ripls/core/observability/logging/redactor.dart';
import 'package:ripls/core/observability/providers.dart';
import 'package:ripls/core/utils/request_id.dart';

/// Context for observable logging operations.
///
/// Holds long-lived ambient log fields (currently the authenticated user
/// ID) plus a free-form `extra` map. The HTTP-correlation `request_id`
/// is **not** stored here — it is scoped per-RPC through a Dart [Zone]
/// (see [RequestIdGenerator]) and read on demand by [ObservableLogger].
/// A global static would race when parallel RPCs are in flight.
class LogContext {
  /// The current authenticated user ID.
  final String? userId;

  /// Additional context fields to include in logs.
  final Map<String, dynamic> extra;

  const LogContext({
    this.userId,
    this.extra = const {},
  });

  /// Creates a new context with additional fields merged.
  LogContext copyWith({
    String? userId,
    Map<String, dynamic>? extra,
  }) {
    return LogContext(
      userId: userId ?? this.userId,
      extra: {...this.extra, ...?extra},
    );
  }

  /// Converts context to a map for logging.
  Map<String, dynamic> toMap() {
    final map = <String, dynamic>{};
    if (userId != null) map['user_id'] = LogRedactor.maskUserId(userId!);
    map.addAll(extra);
    return map;
  }
}

/// Global holder for the current log context.
///
/// This is a simple approach for propagating request context through
/// the app. In a more complex app, you might use InheritedWidget or
/// Riverpod to scope the context.
class LogContextHolder {
  static LogContext _current = const LogContext();
  static BreadcrumbBuffer? _breadcrumbBuffer;

  /// Gets the current log context.
  static LogContext get current => _current;

  /// Sets the current log context.
  static void setCurrent(LogContext context) {
    _current = context;
  }

  /// Resets the context to empty.
  static void clear() {
    _current = const LogContext();
  }

  /// Updates the current context with a user ID.
  ///
  /// Pass null to clear the user ID (e.g., on logout).
  static void setUserId(String? userId) {
    _current = LogContext(
      userId: userId,
      extra: _current.extra,
    );
  }

  /// Gets the shared breadcrumb buffer.
  static BreadcrumbBuffer get breadcrumbBuffer {
    _breadcrumbBuffer ??= BreadcrumbBuffer();
    return _breadcrumbBuffer!;
  }

  /// Sets a custom breadcrumb buffer (useful for testing).
  static void setBreadcrumbBuffer(BreadcrumbBuffer buffer) {
    _breadcrumbBuffer = buffer;
  }

  /// Resets the breadcrumb buffer.
  static void clearBreadcrumbBuffer() {
    _breadcrumbBuffer?.clear();
  }
}

/// A logger wrapper that integrates with the observability system.
///
/// Features:
/// - Automatically includes request_id and user_id from [LogContextHolder]
/// - Automatically records breadcrumbs for warning and above
/// - Automatically redacts PII in logged data
/// - Provides convenience methods for common log patterns
class ObservableLogger {
  final Logger _logger;

  /// Creates an ObservableLogger wrapping the given logger.
  ObservableLogger(this._logger);

  /// Creates a named ObservableLogger.
  ///
  /// The name is used to identify the source of log messages.
  factory ObservableLogger.named(String name) {
    return ObservableLogger(Logger(name));
  }

  /// The name of this logger.
  String get name => _logger.name;

  /// Logs a debug message.
  ///
  /// Debug logs are for verbose details useful during development.
  /// They are not recorded as breadcrumbs.
  void debug(String message, [Map<String, dynamic>? data]) {
    _log(Level.FINE, message, data);
  }

  /// Logs an info message.
  ///
  /// Info logs are for normal operations and successful completions.
  /// They are not recorded as breadcrumbs.
  void info(String message, [Map<String, dynamic>? data]) {
    _log(Level.INFO, message, data);
  }

  /// Logs a warning message.
  ///
  /// Warning logs are for unexpected but handled situations.
  /// They are recorded as breadcrumbs.
  void warning(String message, [Map<String, dynamic>? data]) {
    _log(Level.WARNING, message, data);
    _recordBreadcrumb(message, BreadcrumbLevel.warning, data);
  }

  /// Logs an error message.
  ///
  /// Error logs are for system failures requiring attention.
  /// They are recorded as breadcrumbs.
  void error(String message, [Object? error, StackTrace? stackTrace]) {
    final data = <String, dynamic>{};
    if (error != null) data['error'] = error.toString();
    if (stackTrace != null) data['stack_trace'] = stackTrace.toString();
    _log(Level.SEVERE, message, data, error, stackTrace);
    _recordBreadcrumb(message, BreadcrumbLevel.error, data);
  }

  /// Logs a network operation.
  ///
  /// Convenience method for logging API calls with structured data.
  void network(
    String operation, {
    String? method,
    String? endpoint,
    int? statusCode,
    int? durationMs,
    Map<String, dynamic>? extra,
  }) {
    final data = <String, dynamic>{
      'method': ?method,
      'endpoint': ?endpoint,
      'status_code': ?statusCode,
      'duration_ms': ?durationMs,
      ...?extra,
    };
    _log(Level.INFO, operation, data);
    LogContextHolder.breadcrumbBuffer.addNetwork(operation, data: data);
  }

  /// Logs a user action.
  ///
  /// Convenience method for logging user interactions.
  void userAction(String action, [Map<String, dynamic>? data]) {
    _log(Level.INFO, action, data);
    LogContextHolder.breadcrumbBuffer.addUserAction(action, data: data);
  }

  /// Logs a state change.
  ///
  /// Convenience method for logging app state transitions.
  void stateChange(String change, [Map<String, dynamic>? data]) {
    _log(Level.INFO, change, data);
    LogContextHolder.breadcrumbBuffer.addStateChange(change, data: data);
  }

  /// Internal logging method that includes context.
  void _log(
    Level level,
    String message,
    Map<String, dynamic>? data, [
    Object? error,
    StackTrace? stackTrace,
  ]) {
    final context = LogContextHolder.current;
    final contextMap = context.toMap();

    // Merge and redact data. The HTTP-correlation request_id (when one is
    // active in the surrounding Zone) goes in alongside ambient fields like
    // user_id, so each entry can be joined to its server-side counterpart.
    final mergedData = <String, dynamic>{
      ...contextMap,
      'request_id': ?RequestIdGenerator.current,
      if (data != null) ...LogRedactor.redactMap(data),
    };

    // Format message with context
    final formattedMessage = mergedData.isEmpty
        ? message
        : '$message ${_formatData(mergedData)}';

    _logger.log(level, formattedMessage, error, stackTrace);
  }

  /// Records a breadcrumb for crash context.
  void _recordBreadcrumb(
    String message,
    BreadcrumbLevel level,
    Map<String, dynamic>? data,
  ) {
    final context = LogContextHolder.current;
    final breadcrumbData = <String, dynamic>{
      ...context.toMap(),
      'request_id': ?RequestIdGenerator.current,
      ...?data, // Data is already redacted from _log
    };

    LogContextHolder.breadcrumbBuffer.add(Breadcrumb(
      category: name,
      message: message,
      data: breadcrumbData.isEmpty ? null : breadcrumbData,
      level: level,
    ));
  }

  /// Formats data as a readable string.
  String _formatData(Map<String, dynamic> data) {
    if (data.isEmpty) return '';
    final pairs = data.entries.map((e) => '${e.key}=${e.value}');
    return '[${pairs.join(', ')}]';
  }
}
