# core/audio

Platform audio-session configuration for the app. The Flutter `video_player`
plugin's iOS default audio category is `.playback`, which interrupts other
apps' audio (music, podcasts) even when the underlying player is muted.
This package centralizes the swap between a silent "muted background" mode
and an exclusive "audible playback" mode that ducks other audio.

The configuration is applied through a thin platform channel
(`ripls/audio_session`) backed by native handlers — iOS `AVAudioSession`
categories and Android `AudioManager` audio focus — rather than a third-party
plugin (see #2231 for why: the previous `audio_session` plugin referenced
microphone APIs and tripped Apple's ITMS-90683 purpose-string check on an app
that never records).
On web and desktop the calls are deliberate no-ops: a browser exposes no API
for a page to mix or duck other tabs'/apps' audio, and the feed video's own
mute/volume is handled by `video_player`.

## When to add code here

- Anything that interacts with the platform audio session via the
  `ripls/audio_session` channel or the underlying iOS `AVAudioSession` /
  Android `AudioManager` focus APIs.
- Configuration recipes for audio session categories used by the feed
  video player.

The native handlers live with the platform code: the `AudioSessionPlugin` class
in `app/ios/Runner/AppDelegate.swift` (registered in
`didInitializeImplicitFlutterEngine`) and
`app/android/app/src/main/kotlin/org/ripls/app/AudioSessionPlugin.kt`.

Code that *uses* an audio session (e.g., the actual `VideoPlayerController`
configuration) lives where the controller is created — typically
`core/utils/video_cache_helper.dart` or under
`presentation/widgets/media/`. This directory is reserved for the
session-level configuration that determines how the app coexists with
other audio sources.

## Key files

- [`audio_session_manager.dart`](audio_session_manager.dart) — exposes
  `configureForMutedPlayback()` and `configureForAudiblePlayback()`, plus
  the Riverpod `audioSessionManagerProvider` for test overrides.

## How it's wired

`main.dart` calls `configureForMutedPlayback()` once at startup so the
app's first feed render doesn't hijack other audio. A root-level provider
listener on `videoUnmutedProvider` (defined in
`presentation/viewmodels/video_audio_view_model.dart`) swaps the
configuration whenever the user toggles a feed video's mute state.

## See also

- Issue #1250 — the audio hijack defect this module addresses.
- [docs/issues/1250-video-audio-takeover.md](../../../../docs/issues/1250-video-audio-takeover.md)
  — implementation plan.
