import 'dart:collection';

import 'package:ripls/core/observability/logging/redactor.dart';
import 'package:ripls/core/observability/providers.dart';

/// Standard categories for breadcrumbs.
///
/// Use these categories to classify breadcrumbs for crash context.
/// See Appendix B in the client_observability_plan for usage examples.
enum BreadcrumbCategory {
  /// Screen changes and navigation events.
  /// Example: "Navigated to GearDetailScreen"
  navigation('navigation'),

  /// Button taps, gestures, and user interactions.
  /// Example: "Tapped 'Request Loan' button"
  userAction('user_action'),

  /// API calls and network operations.
  /// Example: "POST /GearService/SaveGear"
  network('network'),

  /// Caught errors and exceptions.
  /// Example: "RPC error: permission_denied"
  error('error'),

  /// App state changes (lifecycle, settings, etc).
  /// Example: "App entered background"
  stateChange('state_change'),

  /// Authentication state changes.
  /// Example: "User logged in", "Token refreshed"
  auth('auth');

  final String value;
  const BreadcrumbCategory(this.value);
}

/// A ring buffer that stores the most recent breadcrumbs for crash context.
///
/// When the buffer is full, the oldest breadcrumb is automatically removed
/// to make room for new ones. This ensures memory usage stays bounded while
/// preserving the most recent context for crash reports.
///
/// PII is automatically redacted from breadcrumb data when added.
class BreadcrumbBuffer {
  /// Default capacity for the breadcrumb buffer.
  static const int defaultCapacity = 100;

  final int _capacity;
  final Queue<Breadcrumb> _buffer = Queue<Breadcrumb>();

  /// Creates a new BreadcrumbBuffer with the specified capacity.
  ///
  /// [capacity] defaults to [defaultCapacity] (100).
  BreadcrumbBuffer({int capacity = defaultCapacity})
      : _capacity = capacity > 0 ? capacity : defaultCapacity;

  /// The maximum number of breadcrumbs this buffer can hold.
  int get capacity => _capacity;

  /// The current number of breadcrumbs in the buffer.
  int get length => _buffer.length;

  /// Whether the buffer is empty.
  bool get isEmpty => _buffer.isEmpty;

  /// Whether the buffer is at full capacity.
  bool get isFull => _buffer.length >= _capacity;

  /// Adds a breadcrumb to the buffer.
  ///
  /// If the buffer is full, the oldest breadcrumb is removed first.
  /// PII in the breadcrumb's data field is automatically redacted.
  void add(Breadcrumb breadcrumb) {
    // Redact PII from breadcrumb data
    final redactedBreadcrumb = _redactBreadcrumb(breadcrumb);

    // Remove oldest if at capacity
    if (isFull) {
      _buffer.removeFirst();
    }

    _buffer.addLast(redactedBreadcrumb);
  }

  /// Adds a breadcrumb using standard category enum.
  ///
  /// Convenience method that creates a breadcrumb with the category string
  /// from [BreadcrumbCategory].
  void addWithCategory({
    required BreadcrumbCategory category,
    required String message,
    Map<String, dynamic>? data,
    BreadcrumbLevel level = BreadcrumbLevel.info,
    DateTime? timestamp,
  }) {
    add(Breadcrumb(
      category: category.value,
      message: message,
      data: data,
      level: level,
      timestamp: timestamp,
    ));
  }

  /// Returns all breadcrumbs in chronological order (oldest first).
  List<Breadcrumb> toList() => _buffer.toList();

  /// Returns all breadcrumbs in reverse chronological order (newest first).
  List<Breadcrumb> toReversedList() => _buffer.toList().reversed.toList();

  /// Returns the most recent [count] breadcrumbs in chronological order.
  ///
  /// If [count] is greater than the buffer length, returns all breadcrumbs.
  List<Breadcrumb> takeLast(int count) {
    if (count <= 0) return [];
    if (count >= _buffer.length) return toList();

    return _buffer.toList().sublist(_buffer.length - count);
  }

  /// Returns breadcrumbs matching the specified category.
  List<Breadcrumb> whereCategory(String category) {
    return _buffer.where((b) => b.category == category).toList();
  }

  /// Returns breadcrumbs at or above the specified level.
  List<Breadcrumb> whereLevel(BreadcrumbLevel minLevel) {
    return _buffer.where((b) => b.level.index >= minLevel.index).toList();
  }

  /// Clears all breadcrumbs from the buffer.
  void clear() => _buffer.clear();

  /// Creates a redacted copy of a breadcrumb.
  Breadcrumb _redactBreadcrumb(Breadcrumb breadcrumb) {
    if (breadcrumb.data == null || breadcrumb.data!.isEmpty) {
      return breadcrumb;
    }

    return Breadcrumb(
      category: breadcrumb.category,
      message: breadcrumb.message,
      data: LogRedactor.redactMap(breadcrumb.data!),
      level: breadcrumb.level,
      timestamp: breadcrumb.timestamp,
    );
  }
}

/// Extension methods for creating breadcrumbs with common categories.
extension BreadcrumbBufferExtensions on BreadcrumbBuffer {
  /// Adds a navigation breadcrumb.
  void addNavigation(String message, {Map<String, dynamic>? data}) {
    addWithCategory(
      category: BreadcrumbCategory.navigation,
      message: message,
      data: data,
    );
  }

  /// Adds a user action breadcrumb.
  void addUserAction(String message, {Map<String, dynamic>? data}) {
    addWithCategory(
      category: BreadcrumbCategory.userAction,
      message: message,
      data: data,
    );
  }

  /// Adds a network breadcrumb.
  void addNetwork(
    String message, {
    Map<String, dynamic>? data,
    BreadcrumbLevel level = BreadcrumbLevel.info,
  }) {
    addWithCategory(
      category: BreadcrumbCategory.network,
      message: message,
      data: data,
      level: level,
    );
  }

  /// Adds an error breadcrumb.
  void addError(String message, {Map<String, dynamic>? data}) {
    addWithCategory(
      category: BreadcrumbCategory.error,
      message: message,
      data: data,
      level: BreadcrumbLevel.error,
    );
  }

  /// Adds a state change breadcrumb.
  void addStateChange(String message, {Map<String, dynamic>? data}) {
    addWithCategory(
      category: BreadcrumbCategory.stateChange,
      message: message,
      data: data,
    );
  }

  /// Adds an auth breadcrumb.
  void addAuth(String message, {Map<String, dynamic>? data}) {
    addWithCategory(
      category: BreadcrumbCategory.auth,
      message: message,
      data: data,
    );
  }
}
