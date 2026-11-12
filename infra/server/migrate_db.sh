#!/usr/bin/env bash
# migrate_db.sh — copy a Cloud SQL database into a mimir stack's Postgres, for
# the cutover (#3078). Runs on mimir.
#
#   migrate_db.sh <env> <cloud-sql-ip> <database> <user>   < password
#
# The source password arrives on stdin so it never appears in argv, the
# environment of another process, or a log. Steps, each timed:
#   1. pg_dump the source with the stack's own pinned Postgres image (a pg16
#      client reads the pg15 server fine), over SSL, to a custom-format dump
#      kept in /srv/sites/ripls_server_<env>/ as the migration's record.
#   2. Stop the stack's server and recreate its database empty.
#   3. pg_restore with --no-owner --no-acl: Cloud SQL's roles do not exist
#      here, and everything is owned by the stack's role. Any error stops it.
#   4. Compare a fingerprint of source and copy — extensions, and every public
#      table's row count — before the server touches the copy.
#   5. Start the server: its idempotent schema setup runs, as on any boot.
#
# The source must be frozen (no writers) before step 1, and reachable from
# this host: the cutover runbook gives Cloud SQL a temporary public IP limited
# to mimir's address. This script changes nothing on the source.
set -euo pipefail

[ $# -eq 4 ] || { echo "usage: migrate_db.sh <dev|prod> <cloud-sql-ip> <database> <user>  < password" >&2; exit 2; }
ENV=$1 SRC_HOST=$2 SRC_DB=$3 SRC_USER=$4
case "$ENV" in dev|prod) ;; *) echo "unknown environment '$ENV'" >&2; exit 2 ;; esac

STACK=/opt/ripls/server/ripls_server_$ENV
PG=ripls_server_$ENV-postgres-1
OUT=/srv/sites/ripls_server_$ENV
DUMP="$OUT/cloudsql-$SRC_DB-$(date -u +%Y%m%dT%H%M%SZ).dump"
# Extensions, then every public table with its row count.
FINGERPRINT=$(cat <<'SQL'
SELECT 'ext ' || extname FROM pg_extension ORDER BY 1;
SELECT 'table ' || table_name || ' rows=' ||
       (xpath('/row/c/text()',
              query_to_xml(format('select count(*) as c from %I.%I', table_schema, table_name),
                           false, true, '')))[1]::text
FROM information_schema.tables
WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
ORDER BY 1;
SQL
)

# read fails at end of input without a trailing newline, which Secret Manager
# values usually lack, so judge by what was read.
IFS= read -r PASSWORD || true
[ -n "$PASSWORD" ] || { echo "no password on stdin" >&2; exit 2; }
IMAGE=$(docker inspect "$PG" --format '{{.Config.Image}}')
t0=$(date +%s)
step() { echo "[$(( $(date +%s) - t0 ))s] $*"; }

# A client container for the source side, with the password in a file the
# container reads, never on a command line.
pwdir=$(mktemp -d)
trap 'rm -rf "$pwdir"' EXIT
umask 077
# pgpass fields escape backslash and colon.
escaped=${PASSWORD//\\/\\\\}
escaped=${escaped//:/\\:}
printf '%s:5432:%s:%s:%s\n' "$SRC_HOST" "$SRC_DB" "$SRC_USER" "$escaped" > "$pwdir/pgpass"
unset PASSWORD escaped
src() {
  docker run --rm -i -e PGSSLMODE=require -e PGPASSFILE=/pw/pgpass \
    -v "$pwdir:/pw:ro" --user 0 "$IMAGE" "$@" -h "$SRC_HOST" -U "$SRC_USER" -d "$SRC_DB"
}

step "1/5 dumping $SRC_DB from $SRC_HOST"
src pg_dump -Fc --no-owner --no-acl > "$DUMP"
chmod 600 "$DUMP"
step "    $(stat -c %s "$DUMP" | numfmt --to=iec) -> $DUMP"
printf '%s' "$FINGERPRINT" | src psql -At -v ON_ERROR_STOP=1 > "$pwdir/fp-source"

step "2/5 stopping the $ENV server and recreating its database"
(cd "$STACK" && docker compose stop server >/dev/null)
docker exec "$PG" psql -U ripls -d postgres -v ON_ERROR_STOP=1 -q \
  -c 'DROP DATABASE IF EXISTS ripls' -c 'CREATE DATABASE ripls OWNER ripls'

step "3/5 restoring"
docker exec -i "$PG" pg_restore -U ripls -d ripls --no-owner --no-acl --exit-on-error < "$DUMP"

step "4/5 comparing source and copy"
printf '%s' "$FINGERPRINT" | docker exec -i "$PG" psql -U ripls -d ripls -At -v ON_ERROR_STOP=1 > "$pwdir/fp-copy"
if diff "$pwdir/fp-source" "$pwdir/fp-copy" > "$pwdir/fp-diff"; then
  step "    identical: $(grep -c '^ext ' "$pwdir/fp-copy") extensions, $(grep -c '^table ' "$pwdir/fp-copy") tables, $(awk -F'rows=' '/^table /{s+=$2} END{print s}' "$pwdir/fp-copy") rows"
else
  step "    DIFFERENT — the server stays stopped:"
  cat "$pwdir/fp-diff"
  exit 1
fi

step "5/5 starting the $ENV server"
(cd "$STACK" && docker compose up -d --wait --wait-timeout 240 server >/dev/null)
step "done: $ENV is serving the migrated data"
