import 'package:ripls/core/observability/events.dart';
import 'package:ripls/core/observability/service.dart';

/// A mock ObservabilityService that captures analytics events for testing.
///
/// Usage:
/// ```dart
/// final mockService = MockObservabilityService();
///
/// container = ProviderContainer(
///   overrides: [
///     observabilityServiceProvider.overrideWithValue(mockService),
///   ],
/// );
///
/// // Perform some action that should trigger an analytics event
///
/// // Verify the event was logged
/// expect(mockService.loggedEvents, hasLength(1));
/// expect(mockService.loggedEvents.first, isA<TransferInterestEvent>());
/// ```
class MockObservabilityService extends ObservabilityService {
  /// List of analytics events that have been logged.
  final List<AnalyticsEvent> loggedEvents = [];

  /// List of raw events logged via logEvent.
  final List<Map<String, dynamic>> rawEvents = [];

  MockObservabilityService() : super();

  @override
  Future<void> logAnalyticsEvent(AnalyticsEvent event) async {
    loggedEvents.add(event);
  }

  @override
  Future<void> logEvent(
    String name, {
    Map<String, dynamic>? parameters,
  }) async {
    rawEvents.add({'name': name, 'parameters': parameters});
  }

  /// Clears all logged events.
  void clear() {
    loggedEvents.clear();
    rawEvents.clear();
  }

  /// Returns logged events of a specific type.
  List<T> eventsOfType<T extends AnalyticsEvent>() {
    return loggedEvents.whereType<T>().toList();
  }

  /// Checks if an event of a specific type was logged.
  bool hasEventOfType<T extends AnalyticsEvent>() {
    return loggedEvents.any((e) => e is T);
  }

  /// Returns the last logged event of a specific type, or null.
  T? lastEventOfType<T extends AnalyticsEvent>() {
    final events = eventsOfType<T>();
    return events.isNotEmpty ? events.last : null;
  }
}
