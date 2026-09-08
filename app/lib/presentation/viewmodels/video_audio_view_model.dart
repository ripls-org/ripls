import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Tracks whether the user has explicitly unmuted feed videos in this app
/// session. Lives at app scope (non-`autoDispose`) so the decision survives
/// feed swipes and screen navigation — once a user taps unmute on one
/// experience, subsequent videos play unmuted until they re-mute or the
/// app is killed (per issue #1250's resolved Open Question).
///
/// The audio session category is swapped in response to changes here by a
/// root-level listener in `main.dart`. Individual content views read the
/// initial value to seed their per-screen `isMuted` state and write back
/// when the user toggles.
class VideoUnmutedNotifier extends Notifier<bool> {
  @override
  bool build() => false;

  /// Sets the session-wide unmuted state. Triggers an audio-session
  /// category swap via the root-level listener.
  void set(bool unmuted) {
    state = unmuted;
  }

  /// Convenience for content-view toggle handlers.
  void toggle() {
    state = !state;
  }
}

/// Session-scoped, non-`autoDispose` provider for the global "any video
/// unmuted" flag. Disposing this would reset the user's choice every
/// feed swipe, which is exactly what we don't want.
final videoUnmutedProvider =
    NotifierProvider<VideoUnmutedNotifier, bool>(VideoUnmutedNotifier.new);
