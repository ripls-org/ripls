---
name: ripls-app-logs
description: >
  Capture and read Ripls Flutter app logs from Android emulators and iOS
  simulators. Trigger when the user mentions app logs, device output, or
  emulator/simulator debugging — e.g. "watch the android app logs", "read the
  ios logs", "tail the flutter output", "show me errors from the app", "repro
  issue X and watch the logs", or any combination of {logs/output/errors} ×
  {android/ios/emulator/simulator/flutter/app}. Also trigger when the user
  asks to launch the Ripls app for debugging.
---

# Ripls App Logs Skill

Log files:
- Android: `/tmp/flutter_android.log`
- iOS:     `/tmp/flutter_ios.log`

Call `<platform>` = `android` or `ios` below.

## 1. Detect devices

```bash
xcrun simctl list devices booted 2>/dev/null          # iOS: look for (Booted)

# Make sure adb is on PATH; the macOS Android Studio default location is
# $HOME/Library/Android/sdk/platform-tools. If $ANDROID_HOME or
# $ANDROID_SDK_ROOT is set, prefer those.
export PATH="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-$HOME/Library/Android/sdk}}/platform-tools:$PATH"
adb devices 2>/dev/null                                # Android: ignore offline
```

`adb` is typically not on PATH by default — prepend platform-tools as shown.
If neither of the env vars is set and the SDK lives elsewhere, adjust the
path to wherever Android SDK `platform-tools` lives on the user's machine.

## 2. Launch the app (NOT in Claude's shell)

Never background `flutter run` in Claude's shell — the user wants a visible,
interactive session they can hot-reload (`r`) and quit (`q`).

Canonical command (swap `DEVICE_ID` and `<platform>`). The `cd` uses the
repo's git root so this works for anyone regardless of where they checked
the repo out:

```bash
cd "$(git rev-parse --show-toplevel)/app" && \
  flutter run --debug --dart-define-from-file=env.local.json -d DEVICE_ID 2>&1 | \
  tee /tmp/flutter_<platform>.log
```

**Do not pass `--no-interactive`** — not a valid flag, exits with 64.

Truncate the log first if you want a clean capture: `: > /tmp/flutter_<platform>.log`

**Where to run it:**
- If `$TERM_PROGRAM == "vscode"` (the usual case): print the command and ask
  the user to paste it into a new VS Code integrated terminal (Cmd+Shift+\`).
  VS Code has no reliable CLI for spawning a preset terminal from outside.
- iTerm fallback (only if not in VS Code): spawn via osascript. Build the
  command in a shell variable so you don't have to inline-escape quotes in
  the osascript:

  ```bash
  CMD='cd "$(git rev-parse --show-toplevel)/app" && flutter run --debug --dart-define-from-file=env.local.json -d DEVICE_ID 2>&1 | tee /tmp/flutter_android.log'
  osascript -e 'tell app "iTerm" to create window with default profile' \
            -e "tell current session of current window to write text \"$CMD\""
  ```

- Any other host: print the command and ask the user to run it.

Then verify in Claude's shell once they've started it:
```bash
tail -n 10 /tmp/flutter_<platform>.log
```

Gradle can take 30s+ on first build — don't give up too early.

## 3. Attach to an already-running app

If the app is running and you only need to tail logs (no launch), safe to
background in Claude's shell:

```bash
flutter logs -d DEVICE_ID 2>&1 | tee /tmp/flutter_<platform>.log &
# or, for Android native crashes that flutter logs misses:
adb -s DEVICE_ID logcat -c && adb -s DEVICE_ID logcat 2>&1 | tee /tmp/flutter_android.log &
```

## 4. Read the logs

```bash
tail -n 300 /tmp/flutter_<platform>.log                    # standard read
grep -iE "error|exception|fatal|crash|stacktrace" /tmp/flutter_<platform>.log | tail -n 50
grep -E "^\[|flutter:|I/flutter|E/flutter|W/flutter" /tmp/flutter_android.log | tail -n 100
ls -lh /tmp/flutter_*.log                                  # check staleness
```

## 5. Watch live

When the user says "watch" or "keep an eye on it", use the Monitor tool with
a `grep --line-buffered` filter — not `tail -f` (blocks). A good default
filter for launch + errors:

```
tail -f /tmp/flutter_<platform>.log | grep -E --line-buffered \
  "Installing|Dart VM Service|Running Gradle|BUILD FAILED|flutter:|I/flutter|E/flutter|W/flutter|Exception|Error|FAILED|Lost connection"
```

Tighten the filter for targeted repros (e.g. add `permission|Geolocator|camera|flyTo` when chasing location/map bugs).

## Troubleshooting

- **`adb: command not found`** → `export PATH="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-$HOME/Library/Android/sdk}}/platform-tools:$PATH"` (adjust if the SDK lives elsewhere)
- **`Could not find an option named '--no-interactive'`** → remove that flag. For stdin suppression use `< /dev/null`.
- **Log file empty after 30s** → app may have crashed; `tail -n 20 /tmp/flutter_<platform>.log`. On Android try `adb logcat` directly.
- **No devices found** → confirm the emulator/simulator is fully booted before detecting. If nothing is running:
  - iOS: `open -a Simulator` (boots the last-used simulator). Or `xcrun simctl boot "iPhone 16 Pro"` for a specific one.
  - Android: open Android Studio → Device Manager and start an AVD; or `emulator @<avd_name>` if `emulator` is on PATH.

### Reset system permissions for clean repros

Any system-permission repro (location, notifications, camera, microphone,
photos, contacts, calendar, Bluetooth, etc.) usually requires a "user
hasn't answered the prompt yet" state. Android and iOS both auto-promote
a single deny to "don't ask again" on modern SDKs, so once denied the
prompt won't re-appear until state is reset.

- **Android — reset all runtime permissions in one shot:**
  ```bash
  # $APP_ID is the dev flavor's applicationId — read it from
  # app/android/app/build.gradle.kts (namespace + applicationIdSuffix).
  adb -s DEVICE_ID shell pm reset-permissions "$APP_ID"
  ```
  Resets every runtime permission the app has ever been asked about
  (location, notifications, camera, etc.) to "never asked". There's no
  per-permission flag for this command — scoping is all-or-nothing at
  the app level.

- **iOS — reset by privacy category:**
  ```bash
  xcrun simctl privacy booted reset <category> "$APP_ID"
  ```
  Valid `<category>` values: `location`, `contacts`, `calendar`, `photos`,
  `microphone`, `camera`, `notifications`, `all`. Use `all` to match the
  Android "reset everything" behavior.

- **Nuclear: uninstall the app** (all state gone — permissions, secure
  storage, preferences, databases):
  ```bash
  adb -s DEVICE_ID uninstall "$APP_ID"        # Android
  xcrun simctl uninstall booted "$APP_ID"     # iOS
  ```
  Then relaunch via `flutter run`.
