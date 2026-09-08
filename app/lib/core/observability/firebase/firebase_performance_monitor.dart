import 'package:firebase_performance/firebase_performance.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/observability/providers.dart';

final _log = Logger('FirebasePerformanceMonitor');

/// Firebase Performance implementation of [PerformanceMonitor].
///
/// Wraps Firebase Performance SDK to provide performance monitoring
/// for traces, screen loads, and custom metrics.
///
/// Usage:
/// ```dart
/// final monitor = FirebasePerformanceMonitor();
/// await monitor.initialize();
///
/// // Start a custom trace
/// final trace = monitor.startTrace('my_operation');
/// trace.start();
/// // ... do work ...
/// trace.stop();
///
/// // Record a screen load
/// monitor.recordScreenLoad('HomeScreen', Duration(milliseconds: 250));
/// ```
class FirebasePerformanceMonitor implements PerformanceMonitor {
  final FirebasePerformance _performance;
  bool _isInitialized = false;

  /// Creates a FirebasePerformanceMonitor.
  ///
  /// If [performance] is not provided, uses the default instance.
  FirebasePerformanceMonitor({FirebasePerformance? performance})
      : _performance = performance ?? FirebasePerformance.instance;

  @override
  bool get isAvailable => _isInitialized;

  @override
  Future<void> initialize() async {
    if (_isInitialized) {
      _log.warning('FirebasePerformanceMonitor already initialized');
      return;
    }

    // Enable performance collection
    await _performance.setPerformanceCollectionEnabled(true);

    _isInitialized = true;
    _log.info('FirebasePerformanceMonitor initialized');
  }

  @override
  PerformanceTrace startTrace(String name) {
    if (!_isInitialized) {
      _log.warning('Attempted to start trace before initialization: $name');
      return _NoOpPerformanceTrace();
    }

    try {
      final trace = _performance.newTrace(name);
      return _FirebasePerformanceTrace(trace);
    } catch (e, s) {
      _log.warning('Failed to create trace: $name', e, s);
      return _NoOpPerformanceTrace();
    }
  }

  @override
  void recordScreenLoad(String screenName, Duration duration) {
    if (!_isInitialized) {
      return;
    }

    try {
      // Create and immediately complete a trace for the screen load
      final trace = _performance.newTrace('screen_load_$screenName');
      trace.start();
      trace.putAttribute('screen_name', screenName);
      trace.setMetric('duration_ms', duration.inMilliseconds);
      trace.stop();

      _log.fine('Recorded screen load: $screenName (${duration.inMilliseconds}ms)');
    } catch (e, s) {
      _log.warning('Failed to record screen load: $screenName', e, s);
    }
  }

  @override
  void recordMetric(String name, double value, {Map<String, String>? attributes}) {
    if (!_isInitialized) {
      return;
    }

    try {
      final trace = _performance.newTrace('metric_$name');
      trace.start();
      trace.setMetric(name, value.round());

      if (attributes != null) {
        for (final entry in attributes.entries) {
          // Firebase Performance limits attribute values to 100 chars
          final truncatedValue = entry.value.length > 100
              ? entry.value.substring(0, 100)
              : entry.value;
          trace.putAttribute(entry.key, truncatedValue);
        }
      }

      trace.stop();
      _log.fine('Recorded metric: $name = $value');
    } catch (e, s) {
      _log.warning('Failed to record metric: $name', e, s);
    }
  }

  /// Creates an HTTP metric for tracking network request performance.
  ///
  /// Use this to track API latency:
  /// ```dart
  /// final metric = monitor.startHttpMetric(url, 'POST');
  /// metric.start();
  /// final response = await http.post(url);
  /// metric.httpResponseCode = response.statusCode;
  /// metric.responsePayloadSize = response.contentLength;
  /// metric.stop();
  /// ```
  HttpMetric startHttpMetric(String url, HttpMethod method) {
    if (!_isInitialized) {
      _log.warning('Attempted to start HTTP metric before initialization');
      return _NoOpHttpMetric();
    }

    try {
      return _performance.newHttpMetric(url, method);
    } catch (e, s) {
      _log.warning('Failed to create HTTP metric: $url', e, s);
      return _NoOpHttpMetric();
    }
  }

  /// Disables performance collection.
  ///
  /// Call this when the user revokes consent.
  Future<void> disable() async {
    try {
      await _performance.setPerformanceCollectionEnabled(false);
      _isInitialized = false;
      _log.info('Performance monitoring disabled');
    } catch (e, s) {
      _log.warning('Failed to disable performance monitoring', e, s);
    }
  }
}

/// Firebase Performance implementation of [PerformanceTrace].
class _FirebasePerformanceTrace implements PerformanceTrace {
  final Trace _trace;
  bool _isStarted = false;

  _FirebasePerformanceTrace(this._trace);

  @override
  void start() {
    if (_isStarted) {
      _log.warning('Trace already started');
      return;
    }
    _trace.start();
    _isStarted = true;
  }

  @override
  void stop() {
    if (!_isStarted) {
      _log.warning('Trace not started');
      return;
    }
    _trace.stop();
  }

  @override
  void putMetric(String name, int value) {
    _trace.setMetric(name, value);
  }

  @override
  void putAttribute(String name, String value) {
    // Firebase Performance limits attribute values to 100 chars
    final truncatedValue = value.length > 100 ? value.substring(0, 100) : value;
    _trace.putAttribute(name, truncatedValue);
  }
}

/// No-op trace for when performance monitoring is disabled.
class _NoOpPerformanceTrace implements PerformanceTrace {
  @override
  void start() {}

  @override
  void stop() {}

  @override
  void putMetric(String name, int value) {}

  @override
  void putAttribute(String name, String value) {}
}

/// No-op HTTP metric for when performance monitoring is disabled.
class _NoOpHttpMetric implements HttpMetric {
  @override
  int? httpResponseCode;

  @override
  int? requestPayloadSize;

  @override
  int? responsePayloadSize;

  @override
  String? responseContentType;

  @override
  Future<void> start() async {}

  @override
  Future<void> stop() async {}

  @override
  String? getAttribute(String name) => null;

  @override
  Map<String, String> getAttributes() => {};

  @override
  void putAttribute(String name, String value) {}

  @override
  void removeAttribute(String name) {}
}
