#!/usr/bin/env bash
# create_reader.sh — create, or reset the password of, the read-only
# ripls_reader role in a mimir stack's database (#3078), for ad-hoc queries
# such as server/cmd/walkthrough-export. Runs on mimir, with the role's SQL on
# stdin:
#
#   ssh root@mimir /opt/ripls/server/create_reader.sh <env> < scripts/readonly-db-user.sql
#
# The password is host-only, in /etc/nas/ripls-server-<env>-reader-password,
# generated on the first run and kept after. Re-running is safe: the SQL is
# idempotent and the role's password is reset to the file's. To rotate, delete
# the file and re-run.
#
# The password reaches psql on stdin, never argv, in a session that logs no
# statement text even on error, since the stack's Postgres logs ship to Cloud
# Logging.
set -euo pipefail

[ $# -eq 1 ] || { echo "usage: create_reader.sh <dev|prod>  < scripts/readonly-db-user.sql" >&2; exit 2; }
ENV=$1
case "$ENV" in dev|prod) ;; *) echo "unknown environment '$ENV'" >&2; exit 2 ;; esac

PWFILE=/etc/nas/ripls-server-$ENV-reader-password
if [ ! -s "$PWFILE" ]; then
  (umask 077; openssl rand -hex 24 > "$PWFILE")
fi

cd "/opt/ripls/server/ripls_server_$ENV"
{
  echo "SET log_statement = 'none';"
  echo "SET log_min_error_statement = panic;"
  cat
  printf "ALTER ROLE ripls_reader PASSWORD '%s';\n" "$(cat "$PWFILE")"
} | docker compose exec -T postgres psql -U ripls -d ripls -v ON_ERROR_STOP=1 -q
echo "ripls_reader ready in ripls_server_$ENV; its password is in $PWFILE"
