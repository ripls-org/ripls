# core/observability

Unified observability stack: crash reporting, performance monitoring, and analytics, all gated by user consent.

## Key types

- **`ObservabilityService`** (`service.dart`) — central facade. Routes calls to the appropriate provider or a no-op based on the current `ObservabilitySettings`. Call `initialize()` once at startup and `updateConsent()` when settings change.
- **`ObservabilityManager`** (`manager.dart`) — widget wrapper that loads persisted consent on startup and passes it to `ObservabilityService`.
- **`ObservabilityConsentTrigger`** (`manager.dart`) — widget that shows the consent dialog once, after the user is authenticated.
- **`ObservabilitySettings`** / **`ObservabilityConsent`** (`settings.dart`) — consent model with per-feature enum values (`granted` / `denied` / `notAsked`).
- **`AnalyticsEvent`** (`events.dart`) — base class for typed analytics events. Concrete event classes live in the same file.
- **`NavigationObserver`** / **`AnalyticsRouteObserver`** / **`PerformanceRouteObserver`** — `RouteObserver` implementations for automatic screen-view and performance logging.
- **`BreadcrumbBuffer`** (`breadcrumbs.dart`) — in-memory ring buffer of recent actions, flushed to the crash reporter when an error is recorded.
- **`providers.dart`** — Riverpod providers for `ObservabilityService` and `ObservabilitySettings`.

Provider implementations live in [`firebase/`](firebase/).
Logging utilities live in [`logging/`](logging/).

## When to add code here vs. elsewhere

- Add a new **analytics event class** to `events.dart`.
- Add a new **provider implementation** (crash reporter, performance monitor, analytics) to `firebase/` and register it via the `ObservabilityService` constructor.
- Logging utilities (redaction, context, `ObservableLogger`) belong in `logging/`.
- Feature-specific observability calls (logging a user action, recording a screen view) belong in the relevant service or viewmodel, not here.
