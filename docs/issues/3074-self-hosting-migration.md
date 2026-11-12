# Issue #3074: [Epic] Eliminate the hosting bill: move server + Postgres (dev and prod) from GCP to mimir

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/3074
**Filed:** 2026-09-18
**Sub-issues:** #3075 HOST-1 (phases 0–2) · #3076 HOST-2 (phases 3, 5) · #3077 HOST-3 (phase 4) · #3078 HOST-4 (phases 6, 7) · #3079 HOST-5 (phase 8)
**Context docs:** `ripls-notes/infra_mimir.md`, `ripls-notes/terraform_deployment.md`, `docs/secrets.md`, `ripls-notes/secrets_inventory.md`, `docs/server/observability.md`, `docs/realtime_updates.md`, `docs/release/cheat_sheet.md`, `docs/issues/2809-infra-cost-reduction.md`, `terraform/environments/{dev,prod}/main.tf`, `terraform/modules/{containers,storage,metabase,monitoring}`, and the **mimir repo** (`README.md`, `docs/service_layout.md`, `docs/secrets.md`, `stacks/caddy/Caddyfile`, `scripts/dump-databases.sh`).

## Problem Statement

The goal is to **eliminate the cloud hosting bill**. After the #2809 cleanup the
GCP bill sits around $120–150/mo, and ~85–90% of it is two pure-hosting lines:

| Line | ~$/mo | Kind | Plan |
|---|---:|---|---|
| Cloud Run prod server (min=1, always-allocated CPU) | ~57 | hosting | **move to mimir** |
| Cloud SQL prod (zonal `db-g1-small`) | ~27 + storage | hosting | **move to mimir** |
| Cloud SQL dev | ~25 | hosting | **move to mimir** |
| Cloud Run dev server (scale-to-zero) | ~0–5 | hosting | move |
| Artifact Registry | ~5 | hosting | **delete** — images build into mimir's daemon |
| Metabase on Cloud Run | ~0 | hosting | **delete** (decided: kill Metabase) |
| Vertex AI / Gemini | ~8 | usage | stay — API consumption, not hosting |
| Secret Manager | ~3.50 | near-free | stay (prune old versions → ~1.50) |
| GCS media buckets | ~0.75 | near-free | stay |
| Cloud Functions (`ripls-alerts`) | ~0.02 | near-free | stay |
| Logging / monitoring / misc | ~10–20 | follows Cloud Run/SQL | shrinks; see Phase 4 |
| FCM, Firebase Auth | 0 | free | stay |

**Target: ~$12–20/mo (≈90% reduction)**, nearly all of it real Gemini usage.
Leaving secrets, the media bucket, Vertex AI, FCM and Firebase Auth on GCP costs
almost nothing and avoids the two genuinely hard migrations (object storage with
signed URLs; re-plumbing every secret consumer including the mobile build
pipelines). Numbers are derived from #2809's actuals plus the fixes that landed;
**re-pull the billing report before starting** so the baseline is measured.

The consequence: hitting the target **requires moving the production
database**, not just the container. That is where the risk in this epic lives.

## What we found

### mimir is already set up for this

- **TLS is solved.** Caddy (`caddy-cloudflare`) terminates TLS with Cloudflare
  DNS-01 certificates. `ripls.org`, `go.ripls.org` and `*.staging.ripls.org`
  already run through it. DNS-01 means certs can be issued **before** the DNS
  flip, so cutover has no TLS gap. (Ripls-side docs still say Nginx-Proxy-Manager;
  that is stale.)
- **Adding a service is one directory**: `stacks/<name>/docker-compose.yml` +
  `caddy.snippet`, imported by a glob. Every hostname is an explicit unproxied
  CNAME to `home.overt.org` (no wildcard DNS).
- **Secrets posture:** nothing committed; 0600 files under `/etc/nas/`,
  per-stack env files in `/etc/nas/stack-env/` symlinked into the stack dir.
- **Backups:** sanoid snapshots of `fast/nas` (recursive: hourly ×48, daily ×30),
  nightly `dump-databases.sh` logical dumps into `/srv/sites/<site>/db.sql.zst`,
  restic → B2. Raw datadirs (`*/db`) are deliberately excluded from restic.
  `dump-databases.sh` today only matches `*_db` MariaDB containers plus a
  hard-coded `immich_postgres`.
- **Capacity is a non-issue:** 20 cores / 61 GiB with ~37 GiB available; `fast`
  mirror has 347 GB free. Uplink is Google Fiber (136.62.x).
- **Host freezes** (the Unraid-era D-state hangs) are plausibly fixed by the
  Ubuntu/ZFS migration but unproven — see Risks.

### The server is more portable than the container

- **The server binary is already cloud-agnostic** (`docs/secrets.md`): every
  credential arrives as a `-*-file` flag. Only `server/entrypoint.sh` is
  GCP-bound — it hard-exits at the metadata-server probe (`entrypoint.sh:45`).
  The root `docker-compose.yaml` is broken today for exactly this reason.
- **Every GCP client uses plain ADC** (GCS `server/storage/bucket.go:103`,
  Firebase `server/firebase/app.go:20`, FCM, Vertex via genkit), so a mounted
  service-account key via `GOOGLE_APPLICATION_CREDENTIALS` covers all of them.
  Signed URLs get *better*: with a key file the GCS client signs locally instead
  of the IAM `signBlob` RPC whose managed key rotates every ~1–2 weeks.
- **Sharp edge:** Firebase Auth init is startup-fatal outside `--dev-mode`
  (`server/main.go:216-220`). A bad or revoked key means prod does not boot.
- **Database is the most portable layer:** plain `lib/pq`, pgvector is the only
  extension, schema is idempotent DDL at boot, no Cloud SQL connector. Tests and
  local compose already use `pgvector/pgvector:pg16`. Cloud SQL is `POSTGRES_15`
  and private-IP-only with no proxy anywhere, so data leaves via
  `gcloud sql export sql` → GCS → mimir. `sslmode=require` is hard-coded in the
  entrypoint DSN.
- **Streaming is server-streaming only** (no bidi), capped at 14 min
  (`defaultStreamLifetime`) to fit Cloud Run's 900 s. `WriteTimeout` is 0 and the
  server speaks h2c. HTTP/2 to the browser must be preserved
  (`docs/realtime_updates.md:100-106` — the HTTP/1.1 connection-starvation
  incident). Caddy handles all of this; **Cloudflare's proxy would not** (idle
  timeout far below stream lifetime), so records stay unproxied.
- **Exactly one instance is strictly better for realtime.** The in-memory event
  bus, stream registries and per-IP rate limiter are only accidentally correct at
  `max_instances=2`. Background jobs are DB-claim-guarded either way. The cost:
  no rolling deploys — every deploy is a brief stream drop that clients already
  absorb via the FCM + polling backstop.
- **Client IP:** `server/middleware/remote_addr.go` trusts the leftmost
  `X-Forwarded-For` blindly. Caddy overwrites client-supplied XFF by default, so
  this stays correct *as long as Caddy is the only hop* — verify from cellular,
  per the mimir gotcha that LAN tests hairpin and prove nothing.

### Hostnames: nothing external needs to change if the names don't

Flutter Web is embedded in the server binary, so it is 4 hostnames onto 2
containers:

| Hostname | Today | After |
|---|---|---|
| `server.ripls.org`, `ripls.app` | Cloud Run domain mapping → `server` | Caddy → `ripls_server_prod` |
| `server-dev.ripls.org`, `dev.ripls.app` | Cloud Run domain mapping → `server-dev` | Caddy → `ripls_server_dev` |
| `analytics[-dev].ripls.org` | Cloud Run → Metabase | **deleted** |

Hostnames are load-bearing in three external consoles — Twilio webhook
signatures are HMAC'd over `https://<host><path>`, Mailgun webhooks target
`ripls.app` / `dev.ripls.app`, and universal/app links pin `ripls.app` (AASA and
`assetlinks.json` are compiled into the binary). **Keeping names identical means
none of these are touched** and no app release is needed (`SERVER_URL` is a
build-time dart-define that does not change).

Gaps: `ripls.app` is a separate Cloudflare zone that is **not in Terraform**
today and **not in Caddy's `CF_TOKEN` scope** (the Caddyfile lists `overt.org`,
`lightlyheld.dev`, `ripls.org`, `type2fun.org`).

### What silently disappears with Cloud Run / Cloud SQL

1. **All observability.** 21 log-based metrics, 13 alerts, 2 SLOs and 4
   dashboards filter on `resource.type="cloud_run_revision"`; there is no
   `/metrics` endpoint to scrape instead. `container_crashes` reads the Cloud Run
   *system* log and has no off-platform equivalent. Four alerts read Cloud SQL
   metrics. Only the uptime check is host-agnostic (and it carries Cloud Run OIDC
   invoker plumbing that becomes unnecessary).
2. **PITR.** Cloud SQL gives 7-day point-in-time recovery + 30 retained backups.
3. **Rolling deploys and revision rollback** (implicit today; nothing in the repo
   automates a rollback, and prod has no post-deploy smoke test).
4. **The implicit runtime identity.** The Compute default SA silently provides
   Secret Manager, GCS, signBlob, FCM, Firebase Auth and Vertex access via the
   metadata server.

### Blocker found: service-account keys are forbidden by org policy

`constraints/iam.disableServiceAccountKeyCreation` is **enforced on both
`ripls-dev` and `ripls-prod`** (verified live 2026-09-18). Off-GCP there is no
metadata server, so mimir needs a credential, and the obvious one cannot
currently be created.

## Decisions (made with Bryan, 2026-09-18)

| # | Decision | Notes |
|---|---|---|
| D1 | **Move compute + Postgres; leave near-free services on GCP** | Secret Manager, GCS media, Vertex AI, FCM, Firebase Auth, Cloud Functions stay. |
| D2 | **Kill Metabase** | Both envs. Keep the read-only DB role (used by `server/cmd/walkthrough-export`) under a neutral name. |
| D3 | **Prod DB durability: nightly dump only** | mimir's existing pattern (nightly `pg_dump` → restic → B2) plus the sanoid snapshots `fast/nas` already gets. **Accepted risk: up to ~24 h of prod data loss on box loss**, vs. PITR today. Hourly dumps / WAL archiving are a follow-up, not part of this epic. |
| D4 | **GCP auth from mimir: lift the org policy briefly, mint keys** | One dedicated least-privilege SA per env, one key each, policy re-enforced. Keys live 0600 under `/etc/nas/`. |
| D5 | **Observability: ship logs to Cloud Logging** | Keeps `terraform/modules/monitoring` and the Pub/Sub → `ripls-alerts` → GitHub-issue chain alive with a filter change. Verify log-based-metric ingestion cost first. |
| D6 | **Runner/docker.sock exposure: accept and track** | The 5 CI runners mount mimir's docker socket, so any workflow job — including the Claude automation jobs that process user-submitted feedback text — is root-equivalent on the host that will hold prod data and secrets. Proceed; file an isolation follow-up outside the critical path. |
| D7 | **Unused Terraform modules: delete** | `containers/`, `metabase/`, and the Cloud SQL + VPC half of `storage/` go once prod is decommissioned. A working compose stack becomes the documented self-host path for forks. |
| D8 | **Fallback: accept single-box downtime, no DR runbook yet** | Host reboots, ISP outages and deploys are user-visible. No GCP cold standby. |
| D9 | **Config home: this (private) repo** | mimir-specific server config lives under `infra/server/`, which `oss_manifest.txt` already excludes (`!infra/**`). Only what *must* live in the mimir repo goes there (see Architecture). |

Not asked, chosen as defaults — flag if wrong:

- **Ingress is direct**: unproxied CNAME → `home.overt.org` → Caddy, the existing
  pattern. The home IP is already public via `ripls.org`.
- **Dev data is migrated, not reset.** The dev migration is the dress rehearsal
  for prod, so it uses the identical export/import procedure.
- **One Postgres container per environment**, not one shared instance — dev is
  where Postgres upgrades get rehearsed.
- **Cloud SQL is kept stopped (~14 days, storage-only billing) after each
  cutover** as a short rollback window, then final-exported to GCS and deleted.
  This is a rollback for the migration itself, not the standing fallback D8
  declined.

## Target architecture

```
                       Cloudflare DNS (unproxied CNAME → home.overt.org)
                                        │ 443
                                  ┌─────▼─────┐
                                  │   Caddy   │  DNS-01 certs, HTTP/2 + HTTP/3
                                  └──┬─────┬──┘
              server.ripls.org       │     │      server-dev.ripls.org
              ripls.app              │     │      dev.ripls.app
                        ┌────────────▼┐   ┌▼────────────┐
                        │ server prod │   │ server dev  │   ripls-server:<sha>
                        └──┬───────┬──┘   └──┬───────┬──┘   (built into mimir's daemon)
                           │       │         │       │
                  ┌────────▼─┐     │  ┌──────▼───┐   │
                  │ pg16 prod│     │  │ pg16 dev │   │     pgvector/pgvector:pg16 (pinned)
                  └──────────┘     │  └──────────┘   │     datadir on fast/nas/<stack>/db
                                   │                 │
                     fluent-bit ───┴────────┬────────┘
                                            ▼
        GCP (kept): Cloud Logging → log metrics/alerts/SLOs → Pub/Sub → ripls-alerts → GitHub
                    Secret Manager · GCS media · Vertex AI · FCM · Firebase Auth · uptime check
```

**Where things live**

| Piece | Home | Why |
|---|---|---|
| Portable entrypoint + ADC secret fetcher, fixed root `docker-compose.yaml` | this repo, **public** paths (`server/`, root) | Forks need a working self-host path (D7); the binary stays hermetic. |
| `infra/server/` — compose for dev + prod, fluent-bit config, `deploy.sh`, `sync.sh`, README | this repo, **private-bound** | D9. Synced to `/opt/ripls-server/` on mimir, following the `runners/sync.sh` → `/opt/runners` precedent. |
| `caddy.snippet` ×2, systemd units, `dump-databases.sh` generalisation | **mimir repo** | Caddy only imports `/opt/nas/stacks/*/caddy.snippet`, `/opt/nas` is deploy-only, and the mimir repo already owns every `ripls-*` systemd unit and the backup scripts. |
| SA keys, DB passwords | `/etc/nas/` on mimir | mimir's secrets posture. Everything else stays in Secret Manager. |
| Cloudflare DNS, SAs + IAM, media bucket, monitoring, budget | Terraform (shrunk) | Declarative layer for everything still on GCP/Cloudflare. State stays in GCS. |

Declarative-infra split going forward: **Terraform** for cloud-side resources,
**compose + Caddy snippets + systemd units in git** for host-side. No Terraform
docker provider — it would fight mimir's "the checkout is the running config" model.

## Implementation Plan

Phases are grouped into five sub-issues by when they can land (see the header);
each is sized to be handed to `/ripls-issue`. HOST-3 can run in parallel with
HOST-2.

### Phase 0 — Kill Metabase (independent; can land first)

- [ ] Export any dashboards worth keeping (screenshots/CSV) — the metadata DB is not migrated.
- [ ] Terraform (both envs): remove `module "metabase"`, the `analytics[-dev]` domain mappings + Cloudflare CNAMEs, the `metabase-*` secret data sources, and `metabase-admin-password` from `server_runtime_secrets`. Apply.
- [ ] Delete `terraform/modules/metabase/`, `.claude/skills/ripls-analytics/`, the `metabase*` npm scripts, the `metabase` profile + volume in `docker-local-postgres.yaml`.
- [ ] Rename the read-only role for `walkthrough-export` (`metabase_reader` → `ripls_reader`); keep `scripts/metabase-readonly-user.sql` under a neutral name.
- [ ] Drop the Metabase DB/users on both Cloud SQL instances; delete the `metabase-*` secrets. Also delete the long-retired `metrics-auth-token` secret and its grant.
- [ ] Update `secrets_inventory.md`, `secret-rotation.md`, `.gitleaks.toml`, `oss_manifest.txt`, `cspell.yaml`.

### Phase 1 — Make the container run anywhere

- [ ] Add a small Go helper (e.g. `server/cmd/fetch-secrets`) that reads the same secret list via **ADC** and writes the `/tmp/secrets` files. ADC covers both the metadata server (Cloud Run) and `GOOGLE_APPLICATION_CREDENTIALS` (mimir), so it replaces the `curl`/`jq` metadata logic in `entrypoint.sh` rather than forking it. Separate binary, so the server stays hermetic per `docs/secrets.md`. Unit tests required.
- [ ] Entrypoint gains a **pre-populated `SECRETS_DIR` mode** (skip fetching entirely) for forks with no GCP at all.
- [ ] `DB_SSLMODE` env (default `require`) instead of the hard-coded DSN suffix.
- [ ] Fix the root `docker-compose.yaml` so `docker compose up` actually boots (it cannot today), and add a CI job that proves it.
- [ ] Verify the image still deploys to Cloud Run unchanged — this phase ships while Cloud Run is still live.
- [ ] Update `docs/secrets.md` wrapper table and the `Dockerfile` env-contract comment.

### Phase 2 — A GCP identity for mimir (D4)

- [ ] Terraform: `ripls-server-mimir` SA per project with **only** what the server uses — `secretAccessor` on the existing `server_runtime_secrets` list + `feedback-signer-key`, `storage.objectAdmin` on the media bucket, `aiplatform.user`, FCM + Firebase Auth roles (narrow the current `roles/firebase.admin`), and `logging.logWriter` for Phase 4. Do **not** copy the Compute default SA's grants wholesale.
- [ ] Check for `iam.serviceAccountKeyExpiryHours` before minting — an expiry policy would turn the startup-fatal Firebase init into a time bomb.
- [ ] Override `iam.disableServiceAccountKeyCreation` at project level → mint one key per env → **re-enforce** → confirm enforcement. Record the procedure in `secret-rotation.md` as the rotation runbook.
- [ ] Install keys at `/etc/nas/ripls-server-{dev,prod}-sa.json` (0600); add to mimir `docs/secrets.md`. Decide whether these belong in Bitwarden alongside `restic.env`.

### Phase 3 — The mimir stacks

- [ ] ZFS datasets `fast/nas/ripls_server_{dev,prod}` (picked up by sanoid's recursive `fast/nas` policy automatically); Postgres datadir in a `db/` subdir so restic's `*/db` exclude applies.
- [ ] `infra/server/`: compose per env — `server` (`ripls-server:${TAG}`, `restart: unless-stopped`, memory limit 2 GiB, elevated `cpu_shares` so a CI storm cannot starve prod), `postgres` (`pgvector/pgvector:pg16` **pinned by digest**, not published on a host port), `fluent-bit` (Phase 4). Non-secret env mirrors today's Terraform `containers` inputs. Compose project name must equal the directory name (see `runners/README.md`).
- [ ] New DB passwords in `/etc/nas/stack-env/ripls_server_{dev,prod}.env`. The Postgres container cannot read Secret Manager, so this one secret lives mimir-side; retire `ripls-db-password` at decommission.
- [ ] `infra/server/sync.sh` → `/opt/ripls-server/` with an include-allowlist like `runners/sync.sh`.
- [ ] mimir repo: `stacks/ripls_server_{dev,prod}/caddy.snippet` — `import dns_cf`, `reverse_proxy h2c://mimir:<port>` with `flush_interval -1`; both hostnames per env. Allocate ports (18793/18794 look free — confirm with `docker ps`).
- [ ] Widen Caddy's `CF_TOKEN` to the `ripls.app` zone; pre-issue all four certs before any DNS change.
- [ ] mimir repo: generalise `dump-databases.sh` to dump any Postgres container (it hard-codes `immich_postgres` today) into `/srv/sites/ripls_server_<env>/db.sql.zst`; note its site-dir lookup keys off the compose `working_dir` basename. Confirm `update-docker-stacks` never touches these stacks (it only walks `/opt/nas/stacks`, and holds DB images regardless).
- [ ] **Prove one restore** of a nightly dump into a scratch container. D8 declined a DR runbook; this is the minimum that makes D3's backups real rather than assumed.
- [ ] `infra/server/README.md` (required for multi-file dirs) and update `ripls-notes/infra_mimir.md` source-of-truth map.

### Phase 4 — Observability re-plumb (D5)

- [ ] **First: measure.** Pull current log volume and log-based-metric ingestion from billing. If D5 turns out to cost more than a few dollars, stop and revisit before building.
- [ ] fluent-bit tails the server containers' JSON logs → Cloud Logging (`generic_node` or `generic_task`, `severity_key severity`, payload parsed to `jsonPayload`), authenticating with the Phase 2 key.
- [ ] Parametrise the resource filter in `terraform/modules/monitoring` (`main.tf`, `log_metrics.tf`, `alerts.tf`, `slos.tf`, `dashboards.tf`) instead of the hard-coded `cloud_run_revision`; fix label extraction that reads `resource.labels.service_name`. Log-based metrics have no backfill — expect a gap and SLO windows that restart.
- [ ] Replace `container_crashes`: a host-side `docker events` watcher (systemd unit, mimir repo) that emits a structured line into the same log stream and calls mimir's `notify`.
- [ ] Uptime check: drop the OIDC SA + `run.invoker` plumbing (`/health` is public now). Dev's check can be re-enabled — there is no scale-to-zero to keep warm any more.
- [ ] Delete the four Cloud SQL alerts; DB health is already covered by the log-derived pool metrics plus `/readyz` (DB is a critical dependency, so an outage trips the uptime alert). Remove the Go Runtime dashboard's Cloud Run memory-limit reference.
- [ ] Shipper-down detection: a "no logs received" absence alert, otherwise a dead fluent-bit looks like a healthy quiet server.
- [ ] Update `docs/server/observability.md` and `scripts/analyze_cloud_logs.sh` (it queries Cloud Run logs).

### Phase 5 — CI/CD

- [ ] `build_and_deploy_container.yaml`: drop the Artifact Registry login/push; `load` the image into the host daemon as `ripls-server:<sha>` (the `build_runner_image.yaml` pattern). GCP auth **stays** — the Flutter Web build still calls `scripts/fetch_secret.sh`.
- [ ] Deploy = host-side `/opt/ripls-server/deploy.sh <env> <sha>`, invoked over SSH with a **forced-command key**: retag, `compose up -d server`, wait on `/readyz`, **auto-rollback to the previous tag on failure**, prune old `ripls-server:*` tags (keep ~10). The runner cannot run compose itself because `env_file` host paths are invisible inside the runner container.
- [ ] Prod gains a post-deploy smoke check (it has none today). Per-env concurrency groups replace the Cloud Run `ABORTED: Conflict` workaround.
- [ ] Confirm `test_dev_e2e` reaches `server-dev.ripls.org` from a runner on the same LAN (router hairpin), or point it at the LAN address.
- [ ] Strip `roles/run.admin`, `iam.serviceAccountUser` and `artifactregistry.writer` from the `github-actions` SA.
- [ ] Update `docs/release/cheat_sheet.md` — server deploy + rollback, and the "reset the production database" snippet that uses `gcloud sql`.

### Phase 6 — Cut over dev (the rehearsal)

- [ ] Lower Cloudflare TTLs. Bring the stack up on an empty DB; smoke with `curl --resolve` before touching DNS.
- [ ] Block Cloud Run ingress → `gcloud sql export sql` → GCS → mimir → restore into pg16 (15→16 via logical dump; pgvector comes with the image) → boot server (idempotent DDL) → flip both CNAMEs.
- [ ] Verify: Go e2e suite, web app streams hold for the full 14 min through Caddy, real client IP in logs **from cellular**, push delivery, media upload + signed URL, Gemini call, Mailgun + Twilio webhook signatures, logs + metrics arriving in Cloud Logging.
- [ ] **Time every step** — this sets the prod maintenance window. Soak ≥ 1 week, including at least one host reboot and one nightly `update-docker-stacks` + runner-recreate cycle.
- [ ] Stop the dev Cloud SQL instance (do not delete yet).

### Phase 7 — Cut over prod

- [ ] Announce a maintenance window sized from Phase 6 timings (expect ~15–30 min).
- [ ] Same procedure as dev. Keep Cloud Run deployed-but-blocked and Cloud SQL stopped for ~14 days as the migration rollback.
- [ ] Verify the activity digest fires the next morning (05:00 America/Denver) — `min_instances=1` semantics now depend on the container simply staying up.
- [ ] Bring `ripls.app` / `dev.ripls.app` DNS records under Terraform (second zone).

### Phase 8 — Decommission and shrink Terraform (D7)

- [ ] Final `gcloud sql export` of both DBs to GCS as a cold archive.
- [ ] Refactor `modules/storage`: split the media bucket + its IAM into its own module **with `moved` blocks** — the prod bucket must not be planned for destroy. Review every plan line before apply.
- [ ] Destroy: Cloud Run services + domain mappings, Cloud SQL (flip `deletion_protection` first), VPC + peering + egress subnet, Artifact Registry repos, the Compute-default-SA grants, the uptime-checker SA.
- [ ] Delete `modules/containers`, `modules/metabase`, the Cloud SQL half of `modules/storage`; disable now-unused APIs (`run`, `sqladmin`, `vpcaccess`, `servicenetworking`, `artifactregistry`).
- [ ] Retire secrets: `ripls-db-password`, `metabase-*`; prune old versions everywhere (#2809 item 6).
- [ ] Lower the billing budget from $300 to something that would actually trip (~$40).
- [ ] Docs sweep: `terraform/README.md`, `ripls-notes/terraform_deployment.md`, `ripls-notes/infra_mimir.md` (the "deploys to Cloud Run, not mimir" statements; NPM → Caddy), `docs/server/architecture.md`, `secrets_inventory.md`, `cloud_functions/README.md` (already stale on App id). Regenerate `docs/llms.txt`.
- [ ] File the follow-ups below and close the epic with a measured before/after bill.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| **Box loss = up to 24 h of prod data gone** (D3) and **no tested recovery path** (D8) | high, accepted | One proven restore in Phase 3. Follow-ups F1/F2. These two accepted risks compound: the first real restore would happen during an outage. |
| **CI jobs are root-equivalent on the prod host** (D6), incl. automation that reads user-submitted text | high, accepted | Follow-up F3. Private repo + trusted committers today; gets worse, not better, as automation grows. |
| **Bad/revoked/expired SA key → prod will not boot** (Firebase init is fatal) | medium | Expiry-policy check in Phase 2; rotation runbook; `deploy.sh` rollback does not help here, so alert on boot failure via the crash watcher. |
| **mimir host freezes** recur — root cause never proven | medium | Kernel hang detection now works; external uptime check catches it; recovery is still a manual power-cycle. |
| **Residential uplink / single box**: ISP outage, reboot, power beyond UPS runtime | medium, accepted | D8. Confirm Google Fiber's terms are fine with hosting a PBC's production API. |
| CPU/RAM contention with 5 CI runners, Plex, Immich | medium | Memory limit + `cpu_shares` on server and Postgres; watch RPC P95 during the dev soak under a full CI run. |
| Terraform refactor destroys the prod media bucket | low likelihood, severe | `moved` blocks, `prevent_destroy`, human-read plan. Bucket versioning is on. |
| D5 costs more than expected (log-based metric ingestion) | low | Measured first in Phase 4; fallback is the minimal uptime+crash+error set. |
| `update-docker-stacks` or a manual `compose pull` upgrades Postgres major version | low | Digest pin; the script already holds DB images; stacks live outside `/opt/nas/stacks`. |

## Proposed follow-up issues (outside this epic)

- **F1** — Tighten prod DB durability: hourly dumps offsite, then WAL archiving (pgBackRest/WAL-G → B2) for real PITR.
- **F2** — DR runbook + timed restore drill: newest B2 dump → any Docker host → DNS flip, with a recovery-time target.
- **F3** — Isolate CI runners from the host docker socket (VM or sysbox/rootless DinD + a local registry for image handoff).
- **F4** — Stale doc fix now, independent of this epic: `infra_mimir.md` / `infra/go-vanity/README.md` still describe Nginx-Proxy-Manager and `/mnt/user/` paths.

## Open questions

1. Is the ~$120–150/mo baseline still right? (Re-pull billing before Phase 0.)
2. Do you own the org policy (so lifting `disableServiceAccountKeyCreation` is a self-serve toggle), or does someone else administer the org?
3. Should the mimir SA keys also go in Bitwarden, given a lost key now means a prod outage rather than an inconvenience?
4. Anything in Metabase worth exporting before Phase 0 deletes it?
5. Sub-issue granularity: one per phase (9), or merge 0 into 8 and 6 into 7?
