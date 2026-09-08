#!/usr/bin/env bash
# ripls_attach_logs.sh
# Detects running Android/iOS devices and starts log capture to known files.
# Usage: ./ripls_attach_logs.sh [android|ios|both]
# Default: both

set -e

ANDROID_LOG="/tmp/flutter_android.log"
IOS_LOG="/tmp/flutter_ios.log"
TARGET="${1:-both}"

detect_android() {
  adb devices 2>/dev/null | awk 'NR>1 && $2=="device" {print $1}' | head -1
}

detect_ios() {
  xcrun simctl list devices booted 2>/dev/null \
    | grep -E '\(Booted\)' \
    | grep -oE '[A-F0-9-]{36}' \
    | head -1
}

attach_android() {
  local device="$1"
  echo "[ripls-app-logs] Attaching to Android device: $device"
  : > "$ANDROID_LOG"  # truncate
  flutter logs -d "$device" 2>&1 | tee "$ANDROID_LOG" &
  echo "[ripls-app-logs] Android logs → $ANDROID_LOG (PID $!)"
}

attach_ios() {
  local device="$1"
  echo "[ripls-app-logs] Attaching to iOS simulator: $device"
  : > "$IOS_LOG"  # truncate
  flutter logs -d "$device" 2>&1 | tee "$IOS_LOG" &
  echo "[ripls-app-logs] iOS logs → $IOS_LOG (PID $!)"
}

# Android
if [[ "$TARGET" == "android" || "$TARGET" == "both" ]]; then
  ANDROID_DEVICE=$(detect_android)
  if [[ -z "$ANDROID_DEVICE" ]]; then
    echo "[ripls-app-logs] No Android emulator/device found — skipping"
  else
    attach_android "$ANDROID_DEVICE"
  fi
fi

# iOS
if [[ "$TARGET" == "ios" || "$TARGET" == "both" ]]; then
  IOS_DEVICE=$(detect_ios)
  if [[ -z "$IOS_DEVICE" ]]; then
    echo "[ripls-app-logs] No booted iOS simulator found — skipping"
  else
    attach_ios "$IOS_DEVICE"
  fi
fi

# Brief wait then confirm files are growing
sleep 3
echo ""
echo "[ripls-app-logs] Status:"
for f in "$ANDROID_LOG" "$IOS_LOG"; do
  if [[ -f "$f" ]]; then
    lines=$(wc -l < "$f")
    echo "  $f — $lines lines"
  fi
done
