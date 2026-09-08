import 'dart:async';

/// Lifecycle states for a streaming Gen* RPC subscription.
///
/// Flow: `idle` → `streaming` → one of `completed`, `errored`, or `cancelled`.
enum GenStreamStatus { idle, streaming, completed, errored, cancelled }

/// Owns a single server-stream subscription for a Gen* RPC and guarantees
/// safe disposal mid-stream.
///
/// Callers (typically ViewModels) wire [onEvent] / [onDone] / [onError]
/// callbacks and open the subscription via [start]. After [dispose] no further
/// callbacks fire, even for events already in flight. Each instance is
/// single-use: calling [start] after completion, error, or disposal throws.
class GenStreamController<TEvent> {
  GenStreamController({
    this.onEvent,
    this.onDone,
    this.onError,
  });

  final void Function(TEvent event)? onEvent;
  final void Function()? onDone;
  final void Function(Object error, StackTrace stackTrace)? onError;

  // False positive: both [cancel] and [dispose] cancel this subscription, but
  // they read it into a local first (so a re-entrant call can't cancel twice),
  // and the lint cannot trace cancellation through the local. Not deferred
  // work — there is nothing here to change.
  // ignore: cancel_subscriptions
  StreamSubscription<TEvent>? _subscription;
  GenStreamStatus _status = GenStreamStatus.idle;
  bool _disposed = false;

  GenStreamStatus get status => _status;
  bool get isActive => _status == GenStreamStatus.streaming;
  bool get isDisposed => _disposed;

  /// Subscribes to [stream] and routes each event through the registered
  /// callbacks. Must only be called once per controller.
  void start(Stream<TEvent> stream) {
    if (_disposed) {
      throw StateError('GenStreamController: start() after dispose()');
    }
    if (_status != GenStreamStatus.idle) {
      throw StateError(
        'GenStreamController: start() called in $_status; controllers are single-use',
      );
    }
    _status = GenStreamStatus.streaming;
    _subscription = stream.listen(
      (event) {
        if (_disposed) return;
        onEvent?.call(event);
      },
      onError: (Object error, StackTrace stackTrace) {
        if (_disposed) return;
        _status = GenStreamStatus.errored;
        onError?.call(error, stackTrace);
      },
      onDone: () {
        if (_disposed) return;
        if (_status == GenStreamStatus.streaming) {
          _status = GenStreamStatus.completed;
        }
        onDone?.call();
      },
      cancelOnError: true,
    );
  }

  /// Cancels the in-flight subscription without marking the controller
  /// disposed. Safe to call before [start] or after terminal events. No
  /// further callbacks fire after cancellation.
  Future<void> cancel() async {
    if (_status == GenStreamStatus.streaming) {
      _status = GenStreamStatus.cancelled;
    }
    final sub = _subscription;
    _subscription = null;
    await sub?.cancel();
  }

  /// Cancels the subscription and locks out further use. Idempotent — safe to
  /// call from `ref.onDispose()` regardless of current status.
  Future<void> dispose() async {
    if (_disposed) return;
    _disposed = true;
    final sub = _subscription;
    _subscription = null;
    await sub?.cancel();
  }
}
