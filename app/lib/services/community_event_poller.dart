import 'dart:async';
import 'dart:math';

import 'package:ripls/core/observability/logging/logger.dart';
import 'package:ripls/services/community_service.dart';
import 'package:ripls/services/event_router.dart';

final _log = ObservableLogger.named('CommunityEventPoller');

/// Polls for events across every community the user belongs to and routes them
/// through the [EventRouter]. Acts as the correctness backstop — even if the
/// stream and push both fail, polling ensures the client converges to correct
/// state within [_pollInterval].
///
/// One timer, one request. This used to run a timer per community, which made
/// poll traffic scale with membership: a 48-community portfolio issued 1.6
/// requests a second forever, and the last community's timer did not start
/// until 141 seconds after launch because starts were staggered by index
/// (#2867). The server now resolves membership inside the query.
class CommunityEventPoller {
  final CommunityService _communityService;
  final EventRouter _eventRouter;

  Timer? _timer;
  int? _lastPollTimestamp;

  /// Guards the post-await route: a poll in flight when [stop] fires would
  /// otherwise repopulate caches that logout just cleared, feeding the previous
  /// user's events into the next user's session. See
  /// docs/client/logout.md § Invariants.
  bool _running = false;

  static const _pollInterval = Duration(seconds: 30);

  CommunityEventPoller(this._communityService, this._eventRouter);

  /// Start polling. Call on app foreground.
  ///
  /// The timestamp is seeded to "now" on a cold start so the first poll only
  /// picks up events that occur after startup — the app already has fresh data
  /// from its normal load. A resume keeps the previous mark so the gap while
  /// backgrounded is caught up.
  void start() {
    _log.info('starting user event poller');
    _running = true;
    _lastPollTimestamp ??= DateTime.now().millisecondsSinceEpoch ~/ 1000;
    _timer?.cancel();
    // Poll immediately as well as on the interval. On a cold start the seeded
    // mark makes this a no-op; on resume it closes the backgrounded gap
    // without waiting a full interval. Cheap now that it is one request rather
    // than one per community.
    unawaited(_poll());
    _timer = Timer.periodic(_pollInterval, (_) => _poll());
  }

  /// Stop polling. Call on app background or logout.
  void stop() {
    _log.info('stopping user event poller');
    _running = false;
    _timer?.cancel();
    _timer = null;
    // Preserve _lastPollTimestamp for the next foreground cycle.
  }

  /// Clear all state. Call on logout.
  void reset() {
    stop();
    _lastPollTimestamp = null;
  }

  Future<void> _poll() async {
    try {
      final events = await _communityService.listUserEvents(
        sinceUnixSec: _lastPollTimestamp,
      );

      // Re-check after the await: teardown may have landed mid-request.
      if (!_running) return;

      for (final event in events) {
        _lastPollTimestamp = max(
          _lastPollTimestamp ?? 0,
          event.occurredAtUnixSec.toInt(),
        );
      }
      // Batch-route so each provider is notified at most once.
      _eventRouter.routeCommunityEvents(events);
    } catch (e) {
      _log.warning('user event poll failed: $e');
      // Swallow errors — the poll is a best-effort backstop.
    }
  }
}
