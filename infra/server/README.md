# Server stacks on mimir

The dev and prod server deployments on mimir (#3074): one compose project per
environment, each the server image plus its own Postgres. Both have served all
traffic since the cutover (#3078). The server ran on Cloud Run with Cloud SQL
before that; both were decommissioned in #3079, so there is no other place the
server runs.

## Files

- `common.yaml` — the `server` and `postgres` services both environments
  extend: pinned pgvector image, healthchecks, limits, and the settings the two
  environments share.
- `ripls_server_dev/docker-compose.yaml`, `ripls_server_prod/docker-compose.yaml`
  — what differs per environment: project name, image tag, port, data path,
  service-account key, and the settings from that environment's Terraform. The
  project name must equal the directory name; see the comment at the top.
- `deploy.sh` — rolls a stack onto an image CI loaded into the host daemon,
  waits for `/readyz`, and rolls back if it never becomes healthy.
- `fluent-bit/fluent-bit.yaml` — config for each stack's `logs` service, which
  ships the stack's logs to Cloud Logging.
- `container-events.sh` — the host's crash reporter, run by the mimir repo's
  `ripls-container-events` unit.
- `migrate_db.sh` — the cutover's one-time copy of a Cloud SQL database into a
  stack's Postgres, checked by a row-count fingerprint; usage at its top. Kept
  as a record: its source instances were deleted in #3079.
- `create_reader.sh` — creates the read-only `ripls_reader` role in a stack's
  database; see [Read-only database access](#read-only-database-access).

## Where things are on mimir

| What | Where | Owned by |
|---|---|---|
| These files | `/opt/ripls/server/` | this directory, via `infra/sync.sh` |
| Database password | `/etc/nas/stack-env/ripls_server_<env>.env`, linked as each stack's `.env` | host only |
| Read-only role's password | `/etc/nas/ripls-server-<env>-reader-password` (root, 0600) | host only; `create_reader.sh` |
| Service-account key | `/etc/nas/ripls-server-<env>-sa.json` (uid 1000, 0400) | host only; created per the secret-rotation runbook (kept privately) |
| Postgres data | `/var/lib/nas/ripls_server_<env>/db` (ZFS `fast/nas/ripls_server_<env>/db`, 16K records) | sanoid snapshots |
| Nightly dump | `/srv/sites/ripls_server_<env>/db.sql.zst` | mimir repo `scripts/dump-databases.sh`, then restic → B2 |
| Proxy entry | mimir repo `stacks/ripls_server_<env>/caddy.snippet` | mimir repo |

| Environment | Port | Hostnames |
|---|---|---|
| dev | 18793 | `server-dev.ripls.org`, `dev.ripls.app` |
| prod | 18794 | `server.ripls.org`, `ripls.app` |

## Deploying

CI does it: `build_and_deploy_container.yaml` loads `ripls-server:<env>-<sha>`
into the host daemon and runs `deploy.sh` over SSH. The key's `authorized_keys`
entry forces `deploy.sh`, and it is only accepted from Docker networks.

Every deploy — the push to `main` (dev), `Manually Deploy a Branch`, and
release finalization (prod) — goes to mimir, and a failed deploy fails the run.
Images are built on the self-hosted runner and loaded straight into the host's
Docker daemon; there is no registry.

By hand, on the host:

```sh
/opt/ripls/server/deploy.sh dev dev-3482c27      # any tag still in the daemon
docker images ripls-server                        # what is available
```

**Rollback:** redeploy an earlier tag with the command above. `deploy.sh` keeps
the newest ten per environment, plus `<env>-previous`, the image the last deploy
replaced.

## Logs and crash reports

Each stack has a `logs` service (Fluent Bit). The server and Postgres log to it
through Docker's `fluentd` driver on a loopback port (dev 24224, prod 24225), and
it writes to Cloud Logging in the environment's project as `generic_task`
entries with `job` = `server-dev` / `server`, authenticating with the stack's key.
That is the shape the monitoring module reads with `log_source = "self_hosted"`,
which both environments use since the cutover (#3078). It buffers to disk
(`/var/lib/nas/ripls_server_<env>/fluent-bit`), so a restart or an outage
delays logs rather than dropping them. `docker compose logs` still works:
Docker keeps a local copy next to any logging driver.

```sh
gcloud logging read 'resource.type="generic_task" AND resource.labels.job="server"' \
  --project=ripls-prod --freshness=10m --limit=20
```

**Crashes.** `container-events.sh` watches Docker for either stack's server
exiting non-zero (a graceful stop exits 0). It writes a `"server container
exited"` line to `/var/log/ripls/<env>-container-events.jsonl`, which the stack's
shipper tails into the same stream (the crash alert's source once
self-hosted), and mails root through mimir's `notify`. Tested by crashing the dev
server with SIGABRT.

## Read-only database access

For ad-hoc queries, such as `server/cmd/walkthrough-export`
(its runbook is kept privately), connect as `ripls_reader`: SELECT
only, including on tables the server creates later. `create_reader.sh` makes the
role from `scripts/readonly-db-user.sql` and keeps its password host-only; run it
once per environment, and again after restoring a database from a dump, which
does not carry roles:

```sh
ssh root@mimir /opt/ripls/server/create_reader.sh prod < scripts/readonly-db-user.sql
```

Postgres publishes no port, so reach it through an SSH tunnel to its container.
The tunnel is the encryption; the stack's Postgres has no TLS.

```sh
PG=$(ssh root@mimir 'docker inspect -f "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}" ripls_server_prod-postgres-1')
ssh -f -N -M -S /tmp/ripls-pg.sock -L 15432:$PG:5432 root@mimir
export PGPASSWORD=$(ssh root@mimir cat /etc/nas/ripls-server-prod-reader-password)
psql 'postgres://ripls_reader@127.0.0.1:15432/ripls?sslmode=disable'
unset PGPASSWORD; ssh -S /tmp/ripls-pg.sock -O exit root@mimir   # close the tunnel
```

## Resetting a database

To start an environment over with an empty database: stop its server, drop and
recreate `ripls`, and start the server, which builds the schema on startup.
`WITH (FORCE)` ends any other session, such as a `ripls_reader` tunnel.

```sh
ssh root@mimir 'cd /opt/ripls/server/ripls_server_prod && docker compose stop server &&
  docker compose exec -T postgres psql -U ripls -d postgres -v ON_ERROR_STOP=1 \
    -c "DROP DATABASE ripls WITH (FORCE)" -c "CREATE DATABASE ripls" &&
  docker compose start server'
```

The nightly dump (`/srv/sites/ripls_server_<env>/db.sql.zst`) is the way back.
Afterwards, re-run `create_reader.sh`: the role's grants were on the dropped
database.

## Changing these files

```sh
infra/sync.sh                                     # ship them
ssh root@mimir 'cd /opt/ripls/server/ripls_server_dev && docker compose up -d'
```

`up -d` recreates only what the change touched. The database image is pinned by
digest; upgrading Postgres is a deliberate dump-and-restore, never a pull.

## When to add code here vs. elsewhere

Anything the host needs to *run* the server belongs here. Anything the host's
own tooling discovers — Caddy entries, systemd units, backup scripts — belongs
in the mimir repo, which is checked out at `/opt/nas`.
