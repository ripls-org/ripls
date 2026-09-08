#!/usr/bin/env bash
# Idempotently ensure the local Web-E2E Postgres database exists.
#
# The Web-E2E harness (docs/issues/2492-web2-pivot-e2e.md) points E2E_DB_URL at
# this database on the *shared, persistent* local Postgres container
# (docker-local-postgres.yaml), which only bootstraps the `ripls` dev DB on first
# init. Without this, a fresh checkout (or one after `npm run db:reset`, which
# wipes the volume) hits "database \"ripls_e2e_proof\" does not exist" when the
# harness boots the server.
#
# CI does NOT use this: it provisions a throwaway Postgres testcontainer per run
# via `server/cmd/web-smoke-db` (Ryuk-cleaned), so the DB lifecycle there is
# fully automatic. This script closes the same gap for local runs — invoked from
# `npm run db:start` so the one command devs already run provisions both DBs.
set -euo pipefail

cd "$(dirname "$0")/.."

COMPOSE=(docker compose -f docker-local-postgres.yaml)
DB="${E2E_DB_NAME:-ripls_e2e_proof}"

if "${COMPOSE[@]}" exec -T postgres \
    psql -U ripls -d ripls -tAc "SELECT 1 FROM pg_database WHERE datname='${DB}'" \
    | grep -q 1; then
  echo "e2e database '${DB}' already present"
else
  echo "creating e2e database '${DB}'"
  "${COMPOSE[@]}" exec -T postgres createdb -U ripls "${DB}"
fi
