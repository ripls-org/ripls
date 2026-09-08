#!/usr/bin/env bash
# Records the booted iOS simulator and converts the output to an animated GIF.
# Press Ctrl+C to stop recording and trigger conversion.

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
GIF_DIR="${SCRIPT_DIR}/tmp/gifs"
mkdir -p "$GIF_DIR"

n=
while [ -f "${GIF_DIR}/output${n}.gif" ] || [ -f "${GIF_DIR}/output${n}.mp4" ]; do
  n=$((${n:-0}+1))
done
F="${GIF_DIR}/output${n}"

xcrun simctl io booted recordVideo --codec=h264 --force "${F}.mp4" &
SIMCTL_PID=$!
echo "Recording started. Press Ctrl+C to stop and convert to GIF."

trap 'trap - INT; echo ""; echo "Stopping recording..."; kill -INT $SIMCTL_PID 2>/dev/null; wait $SIMCTL_PID; echo "Converting to GIF..."; ffmpeg -i "${F}.mp4" -vf "fps=15,scale=320:-1:flags=lanczos" -loop 0 "${F}.gif" && rm "${F}.mp4" && echo "Saved ${F}.gif"' INT

wait $SIMCTL_PID