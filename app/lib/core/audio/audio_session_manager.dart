import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/observability/logging/logger.dart';

final _log = ObservableLogger.named('AudioSessionManager');

/// AudioSessionManager owns the platform audio-session configuration for the
/// app. The Flutter `video_player` plugin defaults to a "playback" category
/// that interrupts other apps' audio (Spotify, Apple Music, podcasts) on iOS,
/// even when the player's volume is muted. This helper swaps the platform
/// category between a polite "muted background" mode and an exclusive
/// "audible playback" mode that ducks other audio, depending on whether the
/// user has unmuted a feed video (issue #1250).
///
/// It drives a thin [MethodChannel] (`ripls/audio_session`) backed by native
/// handlers on iOS (AVAudioSession categories) and Android (AudioManager audio
/// focus). On web and desktop the calls are deliberate no-ops: a browser
/// exposes no API for a page to mix or duck other tabs'/apps' audio (the
/// browser/OS owns that), and the feed video's own mute/volume is handled by
/// `video_player` directly.
///
/// Lives in `core/audio` because it owns a platform lifecycle, analogous to
/// other helpers under `core/`. It is not a controller — it exposes pure async
/// methods and holds no UI state.
class AudioSessionManager {
  AudioSessionManager({MethodChannel? channel})
      : _channel = channel ?? const MethodChannel(channelName);

  /// Platform channel shared with the native iOS/Android handlers.
  static const String channelName = 'ripls/audio_session';

  final MethodChannel _channel;

  /// Configures the platform audio session so the app coexists with other
  /// audio without interrupting it (iOS `.ambient` + mixWithOthers; Android
  /// transient-may-duck focus). Safe to call multiple times. No-op off mobile.
  Future<void> configureForMutedPlayback() => _invoke('configureMuted');

  /// Configures the platform audio session for exclusive playback that ducks
  /// other apps (iOS `.playback` + duckOthers; Android exclusive gain). Call
  /// when the user has explicitly unmuted a feed video. Safe to call multiple
  /// times. No-op off mobile.
  Future<void> configureForAudiblePlayback() => _invoke('configureAudible');

  Future<void> _invoke(String method) async {
    // Web and desktop have no audio-session concept a page can configure;
    // short-circuit before touching the channel so we never log a
    // MissingPluginException. `defaultTargetPlatform` (not `dart:io Platform`)
    // keeps this compiling and correct on web.
    if (kIsWeb) return;
    if (defaultTargetPlatform != TargetPlatform.iOS &&
        defaultTargetPlatform != TargetPlatform.android) {
      return;
    }
    try {
      await _channel.invokeMethod<void>(method);
    } catch (e) {
      // Non-fatal: audio coexistence is a polish layer (#1250). A platform
      // hiccup must never block startup or the mute toggle, so we warn and
      // continue rather than rethrow.
      _log.warning('audio session "$method" failed: $e');
    }
  }
}

/// Provider for [AudioSessionManager]. Tests override this with a mock to
/// verify that `videoUnmutedProvider` changes drive the correct configure
/// call without touching the platform channel.
final audioSessionManagerProvider = Provider<AudioSessionManager>(
  (ref) => AudioSessionManager(),
);
