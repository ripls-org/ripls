# Issue #2231: iOS: replace audio_session plugin with native category setter to avoid spurious NSMicrophoneUsageDescription (ITMS-90683)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2231
**Filed:** 2026-05-28 | **Last activity:** 2026-05-28
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

App Store submission **v0.23.0** is rejected with **ITMS-90683**, which demands an
`NSMicrophoneUsageDescription` purpose string. Ripls never records audio, so the
string would be a false signal — a **Microphone** entry on the App Store privacy
label of a sharing app that has no microphone feature.

The trigger is the `audio_session` Flutter plugin, added 2026-05-20 in commit
`67077c9cf` ("Add proper audio support") and first shipping in v0.23.0. Its iOS
binary references `[[AVAudioSession sharedInstance] requestRecordPermission:]`
(`audio_session-0.1.25/ios/.../DarwinAudioSession.m:244`). Apple's static binary
scan flags the *symbol's presence*, independent of whether the code path is
reachable — so the app is flagged even though our Dart code never invokes it.

We use `audio_session` for **playback session configuration only**: swapping the
platform audio category between a polite "muted background" mode and an exclusive
"audible playback" mode so feed videos coexist with / duck other apps' audio
(#1250). We need that behavior; we do **not** need any microphone-touching API.

The goal: keep the playback-coexistence feature while removing the
microphone-API symbol from the binary, so no purpose string is required and the
privacy label stays clean.

### Evidence the diagnosis is correct
- **Timeline:** v0.22.0 (2026-05-19) shipped and passed review with no mic string;
  v0.23.0 is the first build containing `audio_session`.
- **Camera exonerated:** `camera_avfoundation`'s CocoaPods checksum
  (`5675ca25…`) is byte-identical between v0.22.0 (passed review) and today, so
  the long-present camera plugin is not what changed.
- **We don't record:** `camera_viewport.dart:91` sets `enableAudio: false`;
  `audio_session_manager.dart` only ever sets `.ambient`/`.playback` (never
  `.record`/`.playAndRecord`); we only call `session.configure(...)`, never
  `requestRecordPermission()`; no `AVAudioRecorder`/record APIs exist in
  `app/lib` or `app/ios`.

## Context from Comments

**No comments yet** — the issue was filed 2026-05-28 from a diagnostic
investigation, so direction comes from the issue body and the originating
Slack thread (Bryan/Thomas), summarized:
- Bryan: "Why are we just hitting this now?" → answered by the timeline above
  (the dependency is 8-day-old code that hasn't shipped to prod until v0.23.0).
- Thomas: "playing audio is very different from recording" and "we should not
  need [the mic permission]" → correct; this plan removes the need rather than
  declaring the permission.
- An earlier ad-hoc explanation incorrectly blamed the camera plugin and cited a
  non-existent reference. This plan supersedes that with checksum/timeline
  evidence pointing squarely at `audio_session`.

Because there is no discussion thread, the Open Questions below carry more
weight than usual — confirm the iOS-channel-registration and Android-min-SDK
decisions before implementing.

## Potential Solutions

### Option A: native `MethodChannel` category setter (recommended)
- **Approach:** Keep `AudioSessionManager`'s public API
  (`configureForMutedPlayback()` / `configureForAudiblePlayback()`) but replace
  its internals: drop `package:audio_session` and call a `MethodChannel`
  (`ripls/audio_session`) backed by minimal native code. iOS sets the
  `AVAudioSession` category directly (`.ambient`+`mixWithOthers` /
  `.playback`+`duckOthers`); Android reproduces the focus behavior via
  `AudioManager`/`AudioFocusRequest`. **There is an in-repo precedent to mirror
  exactly:** `ConversationShortcutService` (Dart, `services/conversation_shortcut_service.dart`)
  + `ConversationShortcutsPlugin` (Kotlin, registered in
  `MainActivity.configureFlutterEngine`).
- **Pros:** Removes the `requestRecordPermission` symbol entirely → no purpose
  string, clean privacy label. Sheds a dependency. We only use 2 of the
  plugin's ~50 capabilities, so the native surface is tiny. Follows an
  established pattern in this codebase.
- **Cons:** We now own native code on **two** platforms (iOS Swift + Android
  Kotlin), including the Android API-level split for audio focus. Introduces the
  **first iOS `MethodChannel`** in the app (conversation shortcuts is
  Android-only), so the iOS registration wiring is new ground (see Open
  Questions). Manual device testing required for audio-coexistence parity.
- **Effort:** Medium.

### Option B: add `NSMicrophoneUsageDescription` (stopgap)
- **Approach:** Add a one-line plist string with honest wording, e.g.
  "Ripls plays sound in videos; the audio playback system references microphone
  APIs and requires this declaration. Ripls does not record audio."
- **Pros:** Unblocks v0.23.0 immediately; Apple-sanctioned (the rejection itself
  says a string is required even for referenced-but-unused APIs); fully
  revertible one-liner.
- **Cons:** Surfaces a **Microphone** permission on the App Store privacy label
  for an app that never records — exactly the signal we want to avoid on a
  community sharing app. Doesn't address the root cause.
- **Effort:** Tiny.

### Option C: swap to a different audio-session plugin
- **Approach:** Find a playback-only plugin that doesn't expose record APIs.
- **Pros:** Less native code than Option A.
- **Cons:** Common alternatives still wrap the full `AVAudioSession` surface
  (including `requestRecordPermission`); no well-maintained playback-only option
  is known. A dependency hunt isn't worth it given how small the Option A shim
  is. **Rejected.**

## Recommendation

**Option A as the durable fix, with Option B available as an immediate,
revertible stopgap if v0.23.0 cannot wait for the native work.**

If the release is genuinely blocked today, ship Option B (Phase 0) to get
v0.23.0 out, then land Option A and **remove the plist string** in the same PR
that removes the dependency. If the release can absorb a short delay, skip
Option B and go straight to Option A — it's a medium, well-scoped change with a
direct in-repo template (`ConversationShortcuts`).

Do **not** treat Option B as the end state: a microphone label on a no-mic app
is a recurring "why does Ripls want my mic?" support/trust cost, and the dep is
trivial to replace.

## Open Questions

- **How is the iOS `MethodChannel` registered given the implicit-engine
  `AppDelegate`?** `AppDelegate.swift` uses the newer
  `FlutterImplicitEngineDelegate` / `didInitializeImplicitFlutterEngine(_:)`
  pattern and registers plugins via `engineBridge.pluginRegistry`. There is no
  existing iOS channel to copy. — **Recommendation:** implement a tiny Swift
  `NSObject` conforming to `FlutterPlugin` and register it in
  `didInitializeImplicitFlutterEngine` via the plugin registry (so it gets a
  `registrar.messenger()` for the `FlutterMethodChannel`), mirroring how
  `GeneratedPluginRegistrant.register` is already called there. Verify the
  messenger is available at that point; if not, fall back to grabbing the root
  `FlutterViewController`'s `binaryMessenger` in `didFinishLaunchingWithOptions`.
- **Android `minSdk` vs `AudioFocusRequest` (API 26+).** `app/android/app/build.gradle.kts`
  uses `minSdk = flutter.minSdkVersion`, which is below 26 on current Flutter, so
  the modern `AudioFocusRequest` API isn't always available. — **Recommendation:**
  implement both paths — `AudioFocusRequest` (+ `AudioAttributes`) on API ≥ 26,
  and the deprecated `requestAudioFocus(listener, AudioManager.STREAM_MUSIC,
  hint)` below — matching the focus types `audio_session` produced
  (`gainTransientMayDuck` for muted, `gain` for audible). Confirm the resolved
  `flutter.minSdkVersion` first.
- **Do we need audio-focus lifecycle / interruption handling?** `audio_session`
  exposes interruption and focus-loss event streams, but our code never
  subscribes — we only call `configure(...)`. — **Recommendation:** do **not**
  port event handling. Match current behavior exactly (set category/request
  focus on demand). If a future feature needs interruption handling, add it then.
- **Should iOS deactivate the session / Android abandon focus on background?**
  We don't do this today (the plugin manages it, and we never call
  `setActive(false)`). — **Recommendation:** keep parity — set category +
  `setActive(true)` on configure; don't add teardown unless device testing shows
  a regression (e.g., other apps' audio not resuming).
- **Keep the `AudioSessionConfiguration` value objects?** The current
  `mutedPlaybackConfig`/`audiblePlaybackConfig` constants are `audio_session`
  package types and will no longer compile once the dep is removed. —
  **Recommendation:** delete them; the two intents collapse into two channel
  method names (`configureMuted`/`configureAudible`). The semantics they encoded
  move into the native handlers (documented there).
- **Error handling.** Startup already wraps the call in try/warn/continue
  (`main.dart:119-124`). — **Recommendation:** keep channel failures non-fatal;
  the Dart shim should catch and warn via `ObservableLogger.named('AudioSessionManager')`
  (the codebase's client logger, as `ConversationShortcutService` does with
  `ObservableLogger.named('ConversationShortcutService')`), so a platform hiccup
  never blocks startup or mute toggling. This warn-and-continue is the
  documented exception to CLAUDE.md's fail-fast rule — audio coexistence is a
  non-fatal polish layer, not correctness.
- **Platform scope — iOS, Android, macOS, and web (the web solution).** The
  `audio_session` dep is also registered on macOS
  (`app/macos/.../GeneratedPluginRegistrant.swift`) and the app builds for web
  (`kIsWeb` paths in `main.dart`). **Verified:** `audio_session` has **no web
  implementation** (no `web/` directory in the package), so on web today
  `AudioSession.instance.configure(...)` throws `MissingPluginException`, which
  is silently caught by the startup try/warn (`main.dart:119-124`) and the
  listener's `catchError` (`main.dart:314-316`) — i.e. **web already does nothing
  functional; it just logs caught warnings.** `AVAudioSession` likewise does not
  exist on macOS. — **Recommendation / web (+macOS) solution:** implement native
  handlers for **iOS and Android only**; on **web and macOS the correct solution
  is a deliberate no-op** — a browser exposes no API for a page to duck or mix
  other tabs'/apps' audio (the browser/OS owns that), and our own feed video's
  mute/volume is driven by `video_player`'s web `<video>` element independently
  of any audio session. Guard the Dart shim with `if (kIsWeb) return;` then
  `defaultTargetPlatform ∈ {iOS, Android}` — use `defaultTargetPlatform` from
  `package:flutter/foundation.dart`, **not** `dart:io Platform` (which doesn't
  compile on web) — mirroring `ConversationShortcutService`'s guard. This is
  **behavior-preserving on web** (it did nothing functional before) and strictly
  cleaner (no more caught `MissingPluginException` warnings).
- **Test strategy for a platform-channel wrapper.** `client/testing.md` warns
  against testing implementation details, but for a thin channel boundary the
  method-name contract *is* the behavior. — **Recommendation:** make the
  `MethodChannel` injectable in the constructor (defaulting to
  `const MethodChannel(channelName)`, exactly as `ConversationShortcutService`
  does) and unit-test by injecting a mock channel — preferred over the global
  `TestDefaultBinaryMessengerBinding.setMockMethodCallHandler` — so the test
  follows the codebase's dependency-injection ethos. Assert: (1) each method
  invokes the correct platform method name, (2) errors are swallowed and logged
  (non-fatal), (3) the platform guard short-circuits off iOS/Android.

## Implementation Plan

### Phase 0 (optional): unblock v0.23.0 with the stopgap string
**Goal:** Ship v0.23.0 immediately if it can't wait for the native migration.
**Tasks:**
- [ ] Add `NSMicrophoneUsageDescription` to `app/ios/Runner/Info.plist` with
  honest, playback-only wording.
- [ ] **i18n:** the purpose string is user-facing and iOS shows it verbatim. The
  app supports en + es (`client/i18n.md`), so localize it via
  `InfoPlist.strings` for both locales rather than hardcoding English in the
  base plist. (Purpose strings live in `InfoPlist.strings`, not the ARB files —
  ARB doesn't cover native plist metadata.)
- [ ] Note in the PR that this is a temporary stopgap removed by Phase 2.
**Validates:** v0.23.0 passes ITMS-90683 and ships. **Skip this phase entirely if
the release can wait for Phase 1–3.**

### Phase 1: Dart shim over a platform channel (+ test rewrite)
**Goal:** `AudioSessionManager` no longer imports `package:audio_session`; its
two public methods drive a `MethodChannel('ripls/audio_session')`.
**Tasks:**
- [ ] Rewrite `app/lib/core/audio/audio_session_manager.dart` to call
  `_channel.invokeMethod('configureMuted')` / `'configureAudible'`, mirroring
  `ConversationShortcutService`: constructor takes an injectable
  `MethodChannel? channel` defaulting to `const MethodChannel(channelName)`;
  **guard to iOS/Android** (`if (kIsWeb) return;` then `defaultTargetPlatform ∈
  {iOS, Android}` — use `defaultTargetPlatform`, **not** `dart:io Platform`, so
  the shim compiles on web), and catch+warn on failure via
  `ObservableLogger.named('AudioSessionManager')`. Web/macOS are deliberate
  no-ops (see Open Questions — the web solution).
  Keep `audioSessionManagerProvider` (a plain `Provider`, per architecture's
  "Provider holds services" — `AudioSessionManager` stays a **helper**, not a
  Notifier/"controller", consistent with `core/audio/README.md`) and the two
  method names so `main.dart:120` and `main.dart:310-313` are untouched. The
  three viewmodel/widget tests that override `audioSessionManagerProvider`
  (`video_audio_view_model_test.dart`, `experience_view_model_test.dart`,
  `video_mute_toggle_button_test.dart`) keep passing unchanged.
- [ ] Delete the `AudioSessionConfiguration` constants (package types).
- [ ] Rewrite `app/test/core/audio/audio_session_manager_test.dart`: the current
  tests assert on `audio_session` enums and will not compile. Replace with a
  channel test that injects a mock `MethodChannel` (preferred over the global
  `setMockMethodCallHandler`, per the DI ethos) asserting that
  `configureForMutedPlayback`/`configureForAudiblePlayback` invoke the correct
  method names and swallow errors. Drive `debugDefaultTargetPlatformOverride` to
  assert the iOS/Android path invokes and that non-mobile short-circuits.
**Validates:** Dart compiles without `audio_session`; unit tests prove the
Dart→platform mapping and the platform guard. (Behavior is inert until at least
one native handler lands.)

### Phase 2: iOS native handler + dependency removal + plist cleanup
**Goal:** iOS sets the audio category natively with no microphone symbol; the
dependency and any stopgap string are gone.
**Tasks:**
- [ ] Add a small Swift `FlutterPlugin` handling `ripls/audio_session`:
  - `configureMuted` → `setCategory(.ambient, options: .mixWithOthers)` +
    `setActive(true)`
  - `configureAudible` → `setCategory(.playback, options: .duckOthers)` +
    `setActive(true)`
  Register it from `AppDelegate.didInitializeImplicitFlutterEngine` (see Open
  Questions). No `requestRecordPermission`, no `AVCaptureDevice`.
- [ ] Remove `audio_session` from `app/pubspec.yaml:68`; run `flutter pub get`;
  it drops out of `pubspec.lock` and the regenerated
  `app/macos/Flutter/GeneratedPluginRegistrant.swift` automatically. No macOS or
  web handler is needed — `AVAudioSession` is iOS-only and the Dart guard makes
  those platforms clean no-ops (see Open Questions).
- [ ] Remove the Phase 0 `NSMicrophoneUsageDescription` string if it was added.
- [ ] Verify after `pod install` that no Pod references `requestRecordPermission`
  (`grep -r requestRecordPermission app/ios/Pods` → none from a removed dep).
**Validates:** A TestFlight/App Store archive uploads **without** ITMS-90683 and
**without** a mic purpose string. iOS audio coexistence still works on device.

### Phase 3: Android native handler + parity verification
**Goal:** Android reproduces the focus behavior `audio_session` provided.
**Tasks:**
- [ ] Add `AudioSessionPlugin.kt` next to `ConversationShortcutsPlugin.kt`
  (same `org.ripls.app` package), handling `configureMuted` (focus
  `gainTransientMayDuck`, `AudioAttributes` usage=media/content=movie) and
  `configureAudible` (focus `gain`). Support API ≥ 26 (`AudioFocusRequest`) and
  < 26 (legacy `requestAudioFocus`) per Open Questions.
- [ ] Register it in `MainActivity.configureFlutterEngine`, next to the existing
  `ConversationShortcutsPlugin(...).register(flutterEngine)` line.
- [ ] Manual parity matrix on iOS **and** Android (mirrors #1250): background
  music keeps playing on app open and while a muted feed video plays; unmuting a
  feed video ducks other audio; re-muting restores it.
- [ ] **Web:** confirm `flutter build web` succeeds with the dependency removed
  and that no `MissingPluginException` is logged (the guard short-circuits before
  any channel call); feed-video mute/unmute still works (driven by
  `video_player`, not the audio session).
**Validates:** Cross-platform parity with the `audio_session`-era behavior; the
feature is fully migrated and the dependency is gone everywhere; web/macOS are
clean, deliberate no-ops.

## Related Issues
- #1250 — the video-audio-takeover feature `audio_session` was added for; this
  plan preserves that behavior with native code.
- Commit `67077c9cf` "Add proper audio support" — introduced the dependency now
  being replaced.
- Apple ITMS-90683 / purpose strings:
  https://developer.apple.com/documentation/uikit/protecting_the_user_s_privacy/requesting_access_to_protected_resources

## Audit Log

Reviewed against `docs/client/architecture.md`, `docs/client/testing.md`,
`docs/client/accessibility.md`, `docs/client/i18n.md`, `docs/client/caching.md`,
`docs/server/architecture.md`, and `docs/server/observability.md`.
**Caching and both server docs are N/A** — this is a client-only platform-channel
change with no data/cache layer and no server involvement. **Accessibility is
N/A** — no UI changes; the `video_mute_toggle_button` semantics are untouched
(we only swap the audio-session backend behind the unchanged
`AudioSessionManager` API). Changes applied:

- **Architecture alignment (confirmed, documented):** `AudioSessionManager`
  stays a **helper** that owns a platform/SDK lifecycle (like `DiscoverMapHelper`),
  exposed via a plain `Provider` — not a Notifier or "controller." This matches
  architecture §5 ("No Controllers") and `core/audio/README.md`. Noted a minor,
  non-blocking placement tension: the precedent it mirrors
  (`ConversationShortcutService`) lives under `services/`, while
  `AudioSessionManager` lives under `core/audio/`; both are defensible and the
  existing README justifies `core/audio`, so we keep it there.
- **Platform scope incl. web (new Open Question + Phase 1/3 tasks + Validates):**
  verified `audio_session` ships **no web implementation** (no `web/` dir), so
  web already only threw caught `MissingPluginException`s and did nothing
  functional. Documented the **web (+macOS) solution as a deliberate no-op**
  (browsers expose no audio-ducking API; feed-video mute is driven by
  `video_player`'s `<video>`), guarded in Dart via `kIsWeb` +
  `defaultTargetPlatform` (corrected from `dart:io Platform`, which doesn't
  compile on web). Added a `flutter build web` + no-`MissingPluginException`
  validation. Behavior-preserving and strictly cleaner than the prior state.
- **Observability (Phase 1 + Open Question):** specified `ObservableLogger`
  (the client logger) for the catch-and-warn, and explicitly justified the
  warn-and-continue as the documented exception to CLAUDE.md fail-fast (non-fatal
  audio polish). The server `slog` observability conventions do not apply to
  client Dart or native mobile code.
- **Testing (refined Open Question + Phase 1):** prefer an injected mock
  `MethodChannel` (DI, matching `ConversationShortcutService`) over the global
  test binding, and assert method-name mapping, error-swallowing, and the
  platform guard. Confirmed the three provider-override tests stay green
  unchanged.
- **i18n (Phase 0):** if the Path B stopgap ships, localize
  `NSMicrophoneUsageDescription` via `InfoPlist.strings` for en + es (purpose
  strings aren't ARB-managed) — per `client/i18n.md`'s locale-parity expectation.
