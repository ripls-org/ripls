---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: How to run A/B tests in the app — deterministic server-side variant assignment via FNV-1a hashing, typed client analytics events through ObservabilityService, and Firebase/BigQuery result analysis.
  globs: [app/lib/core/observability/**]
  triggers: [ab-test, a/b, variant, experiment, observability, analytics, fnv, firebase]
  lens: [testing]
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# A/B Testing

How to run A/B tests in the Ripls app using the existing observability system.

## How it works

1. **Variant assignment** — the server assigns a variant deterministically using `FNV-1a(item_id) % N + 1`. Same item always gets the same variant.
2. **Client rendering** — the client reads the variant from the API response and renders the matching layout.
3. **Event logging** — the client logs typed analytics events (`ObservabilityService.logAnalyticsEvent()`) with the variant as a parameter. Events respect the user's analytics consent — no tracking without opt-in.
4. **Analysis** — query Firebase Analytics console or BigQuery export, filtering by the variant parameter.

## Adding a new A/B test

### 1. Define variants and add the field

Add an `optional int32` field to the relevant API proto message:

```protobuf
// proto/ripls/api/some_service.proto
message SomeItem {
  // ... existing fields ...
  optional int32 my_variant = N;  // must be optional (only set when applicable)
}
```

Run `npm run generate` then `npm run build`.

### 2. Assign variants on the server

In the Go service that builds the response, assign the variant:

```go
import "hash/fnv"

func assignVariant(itemID string, numVariants int) int32 {
    h := fnv.New32a()
    h.Write([]byte(itemID))
    return int32(h.Sum32()%uint32(numVariants)) + 1
}
```

Add data-compatibility fallbacks if a variant requires specific data (e.g., multiple images). Fall back to a safe default variant.

Write unit tests for determinism and fallbacks.

### 3. Add analytics events

Add typed event classes to `app/lib/core/observability/events.dart`:

```dart
class MyFeatureViewedEvent implements AnalyticsEvent {
  final int variant;
  final int viewDurationMs;

  MyFeatureViewedEvent({required this.variant, required this.viewDurationMs});

  @override
  String get name => 'my_feature_viewed';

  @override
  Map<String, dynamic> get parameters => {
    'variant': variant,
    'view_duration_ms': viewDurationMs,
  };
}
```

### 4. Log events from the client

In the widget (must be `ConsumerStatefulWidget` for lifecycle + `ref` access):

```dart
class _MyWidgetState extends ConsumerState<MyWidget> {
  final _stopwatch = Stopwatch();

  @override
  void initState() {
    super.initState();
    _stopwatch.start();
  }

  @override
  void dispose() {
    ref.read(observabilityServiceProvider).logAnalyticsEvent(
      MyFeatureViewedEvent(
        variant: widget.variant,
        viewDurationMs: _stopwatch.elapsedMilliseconds,
      ),
    );
    super.dispose();
  }
}
```

### 5. Test

Use `MockObservabilityService` to verify events:

```dart
final mockObservability = MockObservabilityService();
container = ProviderContainer(overrides: [
  observabilityServiceProvider.overrideWithValue(mockObservability),
]);

// ... trigger the action ...

expect(mockObservability.hasEventOfType<MyFeatureViewedEvent>(), isTrue);
final event = mockObservability.lastEventOfType<MyFeatureViewedEvent>();
expect(event?.parameters['variant'], 1);
```

See `test/core/observability/analytics_test_helper.dart` for the mock.

## Analyzing results

In Firebase Analytics (or BigQuery export):

- Filter events by name (e.g., `my_feature_viewed`)
- Group by the `variant` parameter
- Compare: average `view_duration_ms`, event count, tap-through rate

Wait for a meaningful sample size before drawing conclusions — with a small user base and N variants, each variant needs enough observations for the comparison to be useful.

## Checklist

- [ ] `optional` proto field for variant assignment
- [ ] Server: deterministic hash assignment + fallbacks + unit tests
- [ ] Client: typed event class(es) in `events.dart`
- [ ] Client: log events via `observabilityServiceProvider`
- [ ] Tests: verify events with `MockObservabilityService`
- [ ] Both ARB files updated if new user-visible strings are added
