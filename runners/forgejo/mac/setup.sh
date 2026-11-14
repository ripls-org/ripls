#!/usr/bin/env bash
# setup.sh — runs ON THE MAC, as the user that builds iOS. sync.sh ships it
# and invokes it; run it by hand only to repair the runner.
#
#   setup.sh --pending       exit 0 if the runner still needs registering
#   setup.sh                 render config, register (token on stdin), start
#
# The runner binary comes from Homebrew (`brew install forgejo-runner`); keep
# it at the version the Linux fleet pins in ../docker-compose.yaml.
#
# A registration token is read from stdin only when the runner is not yet
# registered, is handed straight to `forgejo-runner register`, and is never
# written to disk or printed.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
STATE="${STATE:-$HOME/forgejo-runner}"
LOGS="${LOGS:-$HOME/Library/Logs/forgejo-runner}"
INSTANCE="${INSTANCE:-https://code.overt.org}"
NAME="${RUNNER_NAME:-$(scutil --get LocalHostName)}"
RUNNER="${RUNNER:-/opt/homebrew/bin/forgejo-runner}"
LABEL="org.ripls.forgejo-runner"
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
DOMAIN="gui/$(id -u)"

# The toolchain, for the daemon and for every job step. Keep it in step with
# what Homebrew has installed on the Mac. Node is named by its keg because
# Homebrew does not link a versioned formula into bin/; the daemon needs it to
# run JavaScript actions (the GitHub runner ships its own instead).
BREW="/opt/homebrew"
JDK="$BREW/opt/openjdk@21"
TOOL_PATH="$BREW/opt/ruby/bin:$BREW/lib/ruby/gems/4.0.0/bin:$JDK/bin:$BREW/opt/node@22/bin:$BREW/bin:$BREW/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
JAVA_HOME_DIR="$JDK/libexec/openjdk.jdk/Contents/Home"

if [ "${1:-}" = "--pending" ]; then
  [ ! -f "$STATE/.runner" ]
  exit
fi

[ -x "$RUNNER" ] || {
  echo "setup.sh: $RUNNER is missing; run: brew install forgejo-runner" >&2
  exit 1
}

mkdir -p "$STATE/work" "$LOGS" "$(dirname "$PLIST")"
sed -e "s|@STATE@|$STATE|g" -e "s|@PATH@|$TOOL_PATH|g" -e "s|@JAVA_HOME@|$JAVA_HOME_DIR|g" \
  "$HERE/config.template.yaml" >"$STATE/config.yaml"

if [ ! -f "$STATE/.runner" ]; then
  IFS= read -r token || true
  if [ -z "${token:-}" ]; then
    echo "setup.sh: the runner needs registering, but no token arrived on stdin" >&2
    exit 1
  fi
  # Labels come from config.yaml, which wins over anything given here.
  (cd "$STATE" && "$RUNNER" register --no-interactive --config "$STATE/config.yaml" \
    --instance "$INSTANCE" --name "$NAME" --token "$token" >/dev/null)
  unset token
  # The file holds the runner's own credential.
  chmod 600 "$STATE/.runner"
  echo "registered $NAME with $INSTANCE"
fi

sed -e "s|@STATE@|$STATE|g" -e "s|@RUNNER@|$RUNNER|g" -e "s|@LOGS@|$LOGS|g" \
  -e "s|@PATH@|$TOOL_PATH|g" \
  "$HERE/$LABEL.plist" >"$PLIST"

# Into the user's GUI session by name. A bare `launchctl load` over SSH would
# put the agent in the SSH session instead, where the login keychain is out
# of reach and the agent dies with the connection.
if launchctl print "$DOMAIN/$LABEL" >/dev/null 2>&1; then
  launchctl bootout "$DOMAIN/$LABEL"
fi
launchctl bootstrap "$DOMAIN" "$PLIST"

echo "started $LABEL; log: $LOGS/stderr.log"
