# core/observability/firebase

Concrete implementations of the observability provider interfaces backed by a crash-reporting and analytics platform.

## Key types

- **`FirebaseCrashReporter`** (`firebase_crash_reporter.dart`) — implements `CrashReporter`. Enables/disables crash collection, flushes `BreadcrumbBuffer` to the crash log before recording each error, and redacts PII from messages and custom keys.
- **`FirebaseAnalyticsProvider`** (`firebase_analytics_provider.dart`) — implements `AnalyticsProvider`. Logs events and screen views; clears analytics data on consent withdrawal.
- **`FirebasePerformanceMonitor`** (`firebase_performance_monitor.dart`) — implements `PerformanceMonitor`. Wraps traces and custom metrics.

## When to add code here vs. elsewhere

These files exist solely because the provider interfaces (defined in `core/observability/providers.dart`) need a concrete implementation. Add code here only when wiring a new capability to the underlying platform SDK.

- Changing **what** is logged (new event, new breadcrumb category) → `core/observability/events.dart` or the call site.
- Adding a **no-op** implementation for testing → `core/observability/providers.dart` (no-ops are already defined there alongside the interfaces).
- Adding a **different backend** (e.g., a different crash reporter) → create a new file in this directory implementing the same interface, then swap it in `services/providers.dart`.
