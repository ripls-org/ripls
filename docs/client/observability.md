---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Client observability system — opt-in consent model, PII redaction, and vendor-abstracted crash reporting, performance monitoring, and analytics fronted by ObservabilityService over Firebase implementations.
  globs: [app/lib/core/observability/**, app/lib/presentation/widgets/observability/**]
  triggers: [observability, crash-reporting, analytics, performance-monitoring, consent, redaction, crashlytics, telemetry]
  lens: [client, observability]
  domain: client
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Client Observability

This document provides a high-level overview of the Flutter client's observability system, covering crash reporting, performance monitoring, and analytics.

## Design Principles

### Privacy First

The observability system is built on an **opt-in consent model**. No data is collected until the user explicitly grants permission. Users can control each category independently:

- **Crash Reporting** - Stack traces and breadcrumbs for debugging
- **Performance Monitoring** - App startup, screen loads, API latency
- **Analytics** - Screen views and feature usage events

Consent state is persisted locally via `SharedPreferences` and can be changed at any time through the profile settings screen. When consent is revoked, unsent data is deleted and collection stops immediately.

### PII Protection

All logging and telemetry passes through a `LogRedactor` that automatically masks sensitive data:

- Emails: `alice@example.com` → `al***@example.com`
- Tokens: Shows first 8 characters only
- Phone numbers: Shows last 4 digits only
- UUIDs in routes: Redacted to prevent user tracking

This redaction happens at the observability layer, ensuring PII never reaches external services regardless of what application code logs.

### Vendor Abstraction

The system uses interface abstractions (`CrashReporter`, `PerformanceMonitor`, `AnalyticsProvider`) that decouple application code from specific vendors. Firebase is the current implementation, but the architecture supports future migration to Sentry or other providers without changing application code.

## Architecture

### Layered Design

```
┌─────────────────────────────────────────────────────────┐
│                    Application Code                      │
│   (screens, viewmodels, services, repositories)         │
└─────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────┐
│              ObservabilityService (Facade)               │
│   - Single entry point for all observability            │
│   - Manages consent state and provider switching        │
│   - Returns no-op implementations when consent denied   │
└─────────────────────────────────────────────────────────┘
                            │
          ┌─────────────────┼─────────────────┐
          ▼                 ▼                 ▼
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│  CrashReporter  │ │PerformanceMonitor│ │AnalyticsProvider│
│   (interface)   │ │   (interface)    │ │   (interface)   │
└─────────────────┘ └─────────────────┘ └─────────────────┘
          │                 │                 │
          ▼                 ▼                 ▼
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│ FirebaseCrash   │ │  FirebasePerf   │ │FirebaseAnalytics│
│    Reporter     │ │    Monitor      │ │    Provider     │
└─────────────────┘ └─────────────────┘ └─────────────────┘
```

### Key Components

**`ObservabilityService`** (`service.dart`) - The facade that application code interacts with. Accessed via `observabilityServiceProvider`. Manages consent-based routing to real or no-op implementations.

**`ObservabilitySettings`** (`settings.dart`) - Consent state model using Riverpod's Notifier pattern. Persisted to `SharedPreferences`. Accessed via `observabilitySettingsProvider`.

**`ObservabilityManager`** (`manager.dart`) - Widget that wraps `MaterialApp.router` in `main.dart`. Handles loading settings on startup and displaying the consent dialog when needed.

**Provider Interfaces** (`providers.dart`) - Abstract interfaces for crash reporting, performance monitoring, and analytics. Includes no-op implementations used when consent is denied.

**Firebase Implementations** (`firebase/`) - Concrete implementations wrapping Firebase Crashlytics, Performance, and Analytics SDKs.

## Crash Reporting

Crash reporting captures unhandled exceptions with context to aid debugging.

### Error Handlers

Three error handlers are configured in `main.dart`:

1. **`FlutterError.onError`** - Catches framework errors (widget build failures, etc.)
2. **`PlatformDispatcher.instance.onError`** - Catches async errors outside Flutter's zone
3. **`runZonedGuarded`** - Catches synchronous errors in the app zone

### Breadcrumbs

A ring buffer (`BreadcrumbBuffer`) maintains the last 100 breadcrumbs. These are automatically recorded for:

- **Navigation** - Screen changes via `BreadcrumbNavigatorObserver`
- **Errors** - Warning/error log messages via `ObservableLogger`
- **Network** - RPC calls (optional, via manual instrumentation)

When an error is recorded, breadcrumbs are flushed to Crashlytics to provide context about what the user was doing before the crash.

### RPC Error Integration

The `RpcErrorHandler` is integrated with crash reporting. System errors (internal, data_loss, unavailable) are reported as crashes with request_id for server log correlation. User errors (validation, not_found, permission_denied) are not reported as they're expected behavior.

## Performance Monitoring

Performance monitoring tracks timing data to identify bottlenecks.

### Automatic Traces

- **App Startup** - Measured from `Firebase.initializeApp()` to first frame render
- **Screen Loads** - Tracked via `PerformanceRouteObserver` attached to GoRouter

### Manual Traces

Every `RpcUtils.executeRpc` call requires a `operationName:` argument (enforced by the Dart compiler — see #1809). Passing `performanceMonitor:` additionally records a Firebase Performance trace keyed `rpc_<operationName>` with duration, success/error status, and error code attributes.

## Analytics

Analytics tracks user behavior to understand feature usage and user flows.

### Typed Events

All analytics events are defined as typed classes in `events.dart`, providing compile-time safety and consistent naming. Events are organized by domain:

- **Screen Views** - `FeedViewedEvent`, `DiscoverViewedEvent`, `InboxViewedEvent`
- **Authentication** - `LoginSuccessEvent`, `LoginFailedEvent`, `LogoutEvent`
- **Gear** - `GearCreatedEvent`, `GearEditedEvent`, `GearDeletedEvent`, `GearSharedEvent`
- **Transfers** - `TransferInterestEvent`, `TransferApprovedEvent`, `TransferCompletedEvent`, etc.
- **Requests** - `RequestCreatedEvent`, `RequestOfferSentEvent`, `RequestFulfilledEvent`
- **Community** - `CommunityCreatedEvent`, `CommunityJoinedEvent`, `CommunityInviteSharedEvent`
- **Conversation** - `ConversationOpenedEvent`, `MessageSentEvent`
- **Search** - `SearchPerformedEvent`, `SearchResultTappedEvent`

### Automatic Screen Views

`AnalyticsRouteObserver` attached to GoRouter automatically logs screen views on navigation. Route names are sanitized (query params removed, UUIDs redacted) before logging.

### Logging Events

```dart
ref.read(observabilityServiceProvider).logAnalyticsEvent(
  GearCreatedEvent(hasLocation: true),
);
```

## Structured Logging

The `ObservableLogger` wraps Dart's `Logger` with additional features:

- **Context Propagation** - Request ID and user ID flow through via `LogContext`
- **Automatic Breadcrumbs** - Warning and error logs create breadcrumbs
- **PII Redaction** - All log data passes through `LogRedactor`

### Usage

```dart
final _log = ObservableLogger.named('MyService');
_log.info('Operation completed', {'item_id': itemId});
```

## File Structure

```
lib/core/observability/
├── service.dart              # ObservabilityService facade
├── settings.dart             # Consent state management
├── providers.dart            # Interfaces + no-op implementations
├── events.dart               # Typed analytics event classes
├── manager.dart              # Startup widget for consent flow
├── breadcrumbs.dart          # Breadcrumb buffer and categories
├── navigation_observer.dart  # Breadcrumb recording for navigation
├── analytics_route_observer.dart    # Auto screen view logging
├── performance_route_observer.dart  # Auto screen timing
├── logging/
│   ├── logger.dart           # ObservableLogger wrapper
│   └── redactor.dart         # PII masking utilities
└── firebase/
    ├── firebase_crash_reporter.dart
    ├── firebase_performance_monitor.dart
    └── firebase_analytics_provider.dart
```

## Testing

Analytics instrumentation is verified using `MockObservabilityService` from `test/core/observability/analytics_test_helper.dart`:

```dart
final mockObservability = MockObservabilityService();
container = ProviderContainer(overrides: [
  observabilityServiceProvider.overrideWithValue(mockObservability),
]);

// After action...
expect(mockObservability.hasEventOfType<GearCreatedEvent>(), isTrue);
final event = mockObservability.lastEventOfType<GearCreatedEvent>();
expect(event?.parameters['has_location'], true);
```

## Future Considerations

The abstraction layer enables migration to alternative providers:

- **Sentry** - Better error grouping, self-hosting option, distributed tracing
- **PostHog** - Product analytics with session replay
- **Custom Backend** - Full data ownership

Migration requires implementing the provider interfaces and updating `ObservabilityService` to instantiate the new implementations.
