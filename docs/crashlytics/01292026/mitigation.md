# Crashlytics Mitigation Plan - January 29, 2026

## Executive Summary

Four production crashes were reported on January 28, 2026, affecting version 0.1.4 (build 3274) on iOS. The crashes fall into two categories:

1. **Riverpod Provider Disposal Crashes** (2 issues) - Providers accessed after disposal
2. **Network Protocol Errors** (2 issues) - HTTP/2 and streaming RPC failures

Some fixes have already been implemented but not yet deployed to production.

---

## Problem Analysis

### Issue 1: ExperienceContentViewState Disposal Crash

**File:** `_ExperienceContentViewState.initState.txt`

**Stack Trace:**
```
ConsumerStatefulElement._assertNotDisposed + 432 (consumer.dart:432)
ConsumerStatefulElement.listenManual + 534 (consumer.dart:534)
_ExperienceContentViewState.initState.<fn> + 105 (experience_content_view.dart:105)
```

**Root Cause:** The `ExperienceContentView` widget is calling `listenManual` on a Riverpod provider during `initState`, but the element has already been disposed. This occurs when the widget is rapidly mounted/unmounted (e.g., during navigation transitions or tab switching).

**Architecture Violation:** Per [architecture.md](../../client/architecture.md#async-safety-in-notifiers), async operations in widgets require `mounted` checks, and provider subscriptions should use proper lifecycle management.

---

### Issue 2: HTTP/2 Connection Forcefully Terminated

**File:** `HTTP2 error.Connection error.Connection is being forcefully terminated.txt`

**Stack Trace:**
```
FirebaseCrashlytics.recordError + 119 (firebase_crashlytics.dart:119)
main.<fn> + 136 (main.dart:136)
```

**Root Cause:** The HTTP/2 connection to the server is being forcefully terminated, likely due to:
- Server-side connection limits or timeouts
- Network instability (cellular/WiFi transitions)
- Load balancer configuration on the server side

**Server Context:** Per [server architecture.md](../../server/architecture.md), Connect-Go handles HTTP/JSON with gRPC compatibility. The server may be terminating long-lived connections without proper graceful shutdown signals.

---

### Issue 3: Missing End-Stream Response in Chat Streaming

**File:** `protocol error.missing end-stream-response.txt`

**Stack Trace:**
```
_extension#0.skipEndStream + 189 (protocol.dart:189)
_Hooks.withHooks + 168 (transport.dart:168)
Client.server + 58 (client.dart:58)
ChatService.streamMessages + 211 (chat_service.dart:211)
```

**Root Cause:** The streaming RPC for chat messages is not receiving the expected end-stream response from the server. The client's protocol handler throws when the stream terminates abnormally without the proper gRPC trailer frame.

**Architecture Impact:** This affects real-time chat functionality and any other streaming RPC endpoints.

---

### Issue 4: GearNotifier State Access After Disposal

**File:** `NotifierProvider.GearNotifier.GearState.txt`

**Stack Trace:**
```
Ref._throwIfInvalidUsage + 220 (ref.dart:220)
AnyNotifier.state + 82 (notifier_provider.dart:82)
GearNotifier.loadGearDetails + 185 (gear_view_model.dart:185)
```

**Root Cause:** `GearNotifier.loadGearDetails()` accesses `state` after an async operation completes, but the provider has been disposed (user navigated away). This is the classic disposal race condition.

**Architecture Violation:** Per [architecture.md](../../client/architecture.md#async-safety-in-notifiers):
> "When using `autoDispose` providers, the notifier can be disposed while async operations are in flight... Check `ref.mounted` after async gaps."

---

## Potential Solutions

### Solution A: SafeNotifierMixin for Existing Notifiers

Add `SafeNotifierMixin` to all Notifiers with async operations:

```dart
class GearNotifier extends Notifier<GearState> with SafeNotifierMixin<GearState> {
  Future<void> loadGearDetails(String gearId) async {
    safeUpdateState((s) => s.copyWith(isLoading: true));
    final gear = await _repository.get(gearId);
    safeUpdateState((s) => s.copyWith(gear: gear, isLoading: false));
  }
}
```

**Pros:**
- Non-breaking change to existing code
- Logs warnings for debugging
- Quick to implement

**Cons:**
- Transitional solution, not ideal long-term
- Silently ignores state updates (may hide bugs)

---

### Solution B: Migrate to AsyncNotifier Pattern

Convert affected Notifiers to use Riverpod's `AsyncNotifier`:

```dart
final gearProvider = AsyncNotifierProvider.autoDispose<GearNotifier, GearState>(
  GearNotifier.new,
);

class GearNotifier extends AsyncNotifier<GearState> {
  @override
  Future<GearState> build() async {
    return _loadInitialState();
  }

  Future<void> loadGearDetails(String gearId) async {
    state = const AsyncLoading();
    state = await AsyncValue.guard(() => _repository.get(gearId));
  }
}
```

**Pros:**
- Riverpod handles disposal automatically in `build()`
- Built-in loading/error states via `AsyncValue`
- Cleaner widget code with `.when()`

**Cons:**
- Requires refactoring existing Notifiers
- Still needs `ref.mounted` checks in non-build methods

---

### Solution C: Widget Lifecycle Guards

For widget-level async operations like in `ExperienceContentView`:

```dart
@override
void initState() {
  super.initState();
  WidgetsBinding.instance.addPostFrameCallback((_) {
    if (!mounted) return;
    ref.listenManual(experienceProvider, (prev, next) {
      if (!mounted) return;
      // Handle state change
    });
  });
}
```

**Pros:**
- Explicit lifecycle management
- Clear intent in code

**Cons:**
- Verbose
- Easy to forget

---

### Solution D: Server-Side Streaming Improvements

For the protocol/streaming errors:

1. **Graceful Stream Termination:** Ensure server sends proper end-stream trailers
2. **Connection Keep-Alive:** Configure HTTP/2 PING frames
3. **Client Retry Logic:** Implement exponential backoff for stream reconnection

**Server Changes:**
```go
// Ensure proper stream cleanup
defer func() {
    if err := stream.Send(&apipb.StreamResponse{EndOfStream: true}); err != nil {
        logger.WarnContext(ctx, "failed to send end-of-stream", "error", err)
    }
}()
```

**Client Changes:**
```dart
Stream<Message> streamMessages() async* {
  await for (final msg in _retryableStream(() => _service.streamMessages())) {
    yield msg;
  }
}
```

---

## Recommendations

### Immediate Priority (Deploy with Next Release)

| Issue | Recommended Solution | Rationale |
|-------|---------------------|-----------|
| GearNotifier disposal | Solution A (SafeNotifierMixin) | Quick fix, already documented pattern |
| ExperienceContentView | Solution C (Widget guards) | Specific to widget lifecycle |
| HTTP/2 termination | **Both client + server** | Client retry for resilience; server investigation for root cause |
| Missing end-stream | **Both client + server** | Server must send proper trailers; client handles gracefully |

### Server-Side Changes (Immediate)

1. **Streaming RPC termination:** Ensure all streaming handlers send proper end-of-stream trailers
2. **HTTP/2 configuration:** Review Cloud Run / load balancer timeout settings
3. **Connection logging:** Add structured logging for connection lifecycle events to diagnose termination source

### Medium-Term (Next Sprint)

1. **Migrate GearNotifier to AsyncNotifier** (Solution B) - More robust long-term
2. **Audit all `autoDispose` Notifiers** for missing `ref.mounted` checks
3. **Add disposal tests** per [architecture.md](../../client/architecture.md#disposal-testing)
4. **HTTP/2 keep-alive:** Configure server-side PING frames to prevent idle timeouts

---

## Open Questions

1. **Connection termination source:** Is the HTTP/2 termination coming from:
   - Cloud Run autoscaling?
   - Load balancer idle timeout?
   - Client-side network transitions?

   **Status:** Unknown. Requires investigation.
   **Action:** Add server-side logging to correlate with client crashes.

2. **Chat stream frequency:** How often are chat streams opened/closed?
   **Action:** Check if rapid stream creation/destruction contributes to protocol errors.

## Confirmed Status

| Question | Answer |
|----------|--------|
| Already fixed in `alpha_2`? | **No** - None of these issues have been addressed yet |
| Disposal tests exist? | **No** - No disposal tests for affected Notifiers |

This means all four issues require fixes before the next production deployment.

---

## Phased Implementation Plan

### Phase 1: Immediate Stabilization (1-2 days) - COMPLETED

**Client:**
- [x] Add `SafeNotifierMixin` to `GearNotifier` - [gear_view_model.dart](../../../app/lib/presentation/viewmodels/gear_view_model.dart)
- [x] Add `mounted` checks to `ExperienceContentView.initState` - [experience_content_view.dart](../../../app/lib/presentation/screens/experience/experience_content_view.dart)
- [x] Wrap streaming calls with try-catch and user-friendly error messages - [chat_service.dart](../../../app/lib/services/chat_service.dart)
- [x] Add client-side network error handling (non-fatal) - [main.dart](../../../app/lib/main.dart)

**Server:**
- [x] Review streaming RPC handlers - Connect-Go handles trailers correctly; added lifecycle logging
- [x] Add connection lifecycle logging - [streaming.go](../../../server/services/chat/streaming.go)
- [x] Increase Cloud Run request timeout from 300s to 900s - [terraform/modules/containers](../../../terraform/modules/containers/)

**Deploy:**
- [ ] Verify fixes in staging environment
- [ ] Deploy server changes first, then client

### Phase 2: Robustness (1 week) - IN PROGRESS

**Async Safety Audit & Fixes (COMPLETED):**
- [x] Audit all Notifiers for async safety issues (17 Notifiers audited)
- [x] Fix HIGH RISK Notifiers:
  - [x] `UserProfileViewModel` - Added `SafeNotifierMixin`, 5 methods fixed
- [x] Fix MEDIUM RISK Notifiers (10 total):
  - [x] `ExperienceNotifier` - Added `SafeNotifierMixin`, 6 methods fixed
  - [x] `CommunityEditNotifier` - Added `SafeNotifierMixin`, 7 methods fixed
  - [x] `ExperienceCreationNotifier` - Added `SafeNotifierMixin`, 1 method fixed
  - [x] `RequestCreationNotifier` - Added `SafeNotifierMixin`, 1 method fixed
  - [x] `RequestSharingNotifier` - Added `SafeNotifierMixin`, 3 methods fixed
  - [x] `GenGearNotifier` - Added `SafeNotifierMixin`, 4 methods fixed
  - [x] `ItemSkillViewModel` - Added `SafeNotifierMixin`, 2 methods fixed
  - [x] `GovernanceNotifier` - Added `SafeNotifierMixin`, 3 methods fixed
  - [x] `CommunityContentViewModel` - Added `SafeNotifierMixin`, 4 methods fixed
- [x] LOW RISK Notifiers already fixed: `GearNotifier`, `SearchNotifier`

**Disposal Tests (COMPLETED):**
- [x] Add disposal tests for key `autoDispose` providers - [disposal_safety_test.dart](../../../app/test/presentation/viewmodels/disposal_safety_test.dart)
  - Tests verify async operations complete without throwing when provider disposed mid-await
  - Covers GearNotifier, CommunityContentViewModel, GovernanceNotifier
  - Also fixed bug in GearNotifier where `ref.read(observabilityServiceProvider)` was called after async gap

**Remaining Phase 2 Tasks:**
- [ ] Add connection health monitoring to chat service
- [ ] Configure HTTP/2 keep-alive (server-side PING frames)
- [ ] Document streaming RPC error handling patterns

### Phase 3: Monitoring & Prevention (Ongoing)

- [ ] Add crashlytics dashboards for these error categories
- [ ] Monitor production logs for connection lifecycle issues
- [ ] Add CI lint rule for missing `ref.mounted` checks
- [ ] Schedule quarterly async safety audits

### Phase 4: Preventive Measures (Ongoing)

- [ ] Add CI lint rule for missing `ref.mounted` checks
- [ ] Update developer documentation with streaming patterns
- [ ] Add crashlytics monitoring dashboards
- [ ] Schedule quarterly async safety audits

---

## References

- [Client Architecture - Async Safety](../../client/architecture.md#async-safety-in-notifiers)
- [Client Caching](../../client/caching.md)
- [Server Architecture](../../server/architecture.md)
- [SafeNotifierMixin Implementation](../../../app/lib/core/utils/safe_notifier.dart)

---

**Document Created:** January 29, 2026
**Author:** Claude (AI-assisted analysis)
**Status:** Phase 1 complete, Phase 2 in progress (12 Notifiers fixed, disposal tests added) - connection health monitoring, HTTP/2 keep-alive, and documentation pending
