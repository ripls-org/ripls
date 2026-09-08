import 'dart:async';
import 'dart:math';

import 'package:logging/logging.dart';
import 'package:ripls/services/community_service.dart';
import 'package:ripls/services/event_router.dart';

final _log = Logger('CommunityEventStream');

/// Manages the user's single gRPC event stream for sub-second real-time
/// updates across every community they belong to.
///
/// This used to hold one stream per community, which made the connection count
/// scale with membership. Once every item spawned a per-item community that
/// stopped being a handful: a browser caps connections per origin, so the
/// streams consumed every slot and ordinary requests were never sent (#2867).
///
/// There is no per-community subscribe call because the subscription names no
/// community — the server resolves membership per event, so joining or leaving
/// one takes effect on the open stream with no client action.
///
/// Events are routed through the [EventRouter] for deduplication and cache
/// invalidation. Reconnects with exponential backoff on failure.
class CommunityEventStreamService {
  final CommunityService _communityService;
  final EventRouter _eventRouter;

  StreamSubscription<StreamUserEventsResponse>? _subscription;

  /// High-water mark of received events, passed as `since` on reconnect so the
  /// server replays what was missed while the stream was down.
  int? _lastTimestamp;

  /// Guards against a reconnect that outlives a [disconnect]. A stream error
  /// or completion can arrive after teardown, and reconnecting then would
  /// resurrect a stream nothing will close — and, worse, route the previous
  /// user's events into the next user's just-cleared caches. See
  /// docs/client/logout.md § Invariants.
  bool _connected = false;

  // Reconnect backoff state.
  int _reconnectAttempts = 0;
  static const defaultBaseDelay = Duration(seconds: 2);
  static const _maxDelay = Duration(seconds: 60);
  final Duration _baseDelay;
  final _random = Random();

  /// [baseDelay] is the first reconnect backoff step. Tests shorten it so a
  /// reconnect is observable inside a test's lifetime — with the production
  /// two seconds, an assertion that a reconnect did *not* happen passes
  /// whether the guard works or not.
  CommunityEventStreamService(
    this._communityService,
    this._eventRouter, {
    Duration baseDelay = defaultBaseDelay,
  }) : _baseDelay = baseDelay;

  /// Open the user's event stream. Call on app foreground. Idempotent: a
  /// second call while connected reopens rather than stacking a second stream.
  void connect() {
    _log.info('connecting user event stream');
    _connected = true;
    _open();
  }

  /// Close the stream. Call on app background or logout.
  void disconnect() {
    _log.info('disconnecting user event stream');
    _connected = false;
    _subscription?.cancel();
    _subscription = null;
    _resetBackoff();
    // Preserve _lastTimestamp for catch-up on reconnect.
  }

  /// Clear all state. Call on logout.
  void reset() {
    disconnect();
    _lastTimestamp = null;
  }

  void _open() {
    _subscription?.cancel();

    final stream = _communityService.streamUserEvents(
      sinceUnixSec: _lastTimestamp,
    );

    _subscription = stream.listen(
      (response) {
        _resetBackoff();
        if (response.hasEvent()) {
          final event = response.event;
          _lastTimestamp = max(
            _lastTimestamp ?? 0,
            event.occurredAtUnixSec.toInt(),
          );
          _eventRouter.routeCommunityEvent(event);
        } else if (response.hasHeartbeat()) {
          _log.fine('user event stream heartbeat');
        }
      },
      onError: (error) {
        _log.warning('user event stream error: $error');
        // Poll backstop keeps us consistent until the stream is back.
        _scheduleReconnect();
      },
      onDone: () {
        _log.info('user event stream ended');
        _scheduleReconnect();
      },
    );
  }

  void _scheduleReconnect() {
    if (!_connected) return;
    final delay = _nextReconnectDelay();
    _log.info('reconnecting user event stream in ${delay.inSeconds}s');
    Future.delayed(delay, () {
      // Re-check: a disconnect may have landed while the delay elapsed.
      if (_connected) _open();
    });
  }

  Duration _nextReconnectDelay() {
    final exponential = _baseDelay * pow(2, min(_reconnectAttempts, 5));
    // Jitter spreads reconnects so a server restart doesn't bring every client
    // back at once. Bounded by the base delay rather than a flat second, so it
    // can't dwarf the delay it is perturbing — at the production base of 2s
    // this is the same 0-1000ms as before.
    final jitterCeilingMs = min(_baseDelay.inMilliseconds, 1000);
    final jitter = Duration(milliseconds: _random.nextInt(max(jitterCeilingMs, 1)));
    _reconnectAttempts++;
    final delay = exponential + jitter;
    return delay > _maxDelay ? _maxDelay : delay;
  }

  void _resetBackoff() {
    _reconnectAttempts = 0;
  }
}
