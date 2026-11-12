#!/usr/bin/env bash
# container-events.sh — report server container crashes into the server's own
# log stream, and by mail.
#
# Runs on mimir as the mimir repo's ripls-container-events unit. Cloud Run
# recorded crashes in its system log; nothing does that for a container here,
# and a process that dies hard (OOM kill, os.Exit(1), an unrecovered panic)
# writes no error log of its own. So this watches Docker for the server
# container of either stack exiting non-zero — a graceful stop exits 0 — and
# for each one:
#   - appends a JSON line to /var/log/ripls/<env>-container-events.jsonl, which
#     that stack's log shipper tails into Cloud Logging, where the monitoring
#     module's crash metric and alert match "server container exited";
#   - mails root through mimir's notify, so a crash is seen even if the log
#     shipper is what died.
# Exit code 137 is SIGKILL, usually the kernel's OOM killer.
set -uo pipefail

DIR=/var/log/ripls
NOTIFY=/opt/nas/scripts/notify
mkdir -p "$DIR"

docker events \
  --filter type=container \
  --filter event=die \
  --filter label=com.docker.compose.service=server \
  --format '{{json .}}' |
while read -r event; do
  project=$(jq -r '.Actor.Attributes["com.docker.compose.project"] // ""' <<<"$event")
  case "$project" in
    ripls_server_dev) env=dev ;;
    ripls_server_prod) env=prod ;;
    *) continue ;;
  esac
  code=$(jq -r '.Actor.Attributes.exitCode // ""' <<<"$event")
  [ "$code" = "0" ] && continue

  line=$(jq -c '{
      severity: "CRITICAL",
      message: "server container exited",
      exit_code: (.Actor.Attributes.exitCode | tonumber? // .),
      container: .Actor.Attributes.name,
      image: .Actor.Attributes.image
    }' <<<"$event")
  printf '%s\n' "$line" >> "$DIR/$env-container-events.jsonl"
  printf '%s\n' "$line"

  # </dev/null: notify appends stdin to the mail body, and stdin here is the
  # event stream, which it would otherwise swallow.
  "$NOTIFY" -e "Ripls server" -i alert \
    -s "ripls $env server exited with code $code" \
    -d "The ripls $env server container exited with code $code (137 is SIGKILL, usually an OOM kill). Docker's restart policy restarts it. Logs: cd /opt/ripls/server/ripls_server_$env && docker compose logs --tail 100 server" \
    </dev/null
done
