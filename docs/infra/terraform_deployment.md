---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Operational runbook for this deployment's Terraform on GCP and Cloudflare — state buckets, project and API setup, Secret Manager secrets, DNS wiring, and how server images reach mimir for dev and prod.
  globs: [terraform/environments/**, .github/workflows/build_and_deploy_container.yaml, .github/workflows/deploy_*.yaml]
  triggers: [terraform, gcp, secret-manager, cloudflare, dns, deploy, mimir, state-bucket]
  lens: [infra]
  skills: [issue, audit]
  domain: infra
---
# Terraform deployment runbook — this instance

The operational half of the Terraform setup: the GCP projects this deployment
uses, the APIs to enable, the secrets to create, the DNS wiring, and how a
server build reaches its environment. Every command here names this
deployment's accounts; a fork substitutes its own.

The reusable half — module design, the environment/module split, how remote
state and variables work, and how to run plans locally — is generic and lives
in the public [`terraform/README.md`](../../terraform/README.md).

The server itself does not run on GCP. Both environments run as Docker Compose
stacks on the self-hosted host mimir (see
[`infra/server/README.md`](../../infra/server/README.md)) and have since the
cutover (#3078). The Cloud Run services, Cloud SQL instances, their VPC and
peering, and the Artifact Registry `images` repositories were decommissioned in
#3079. What the GCP projects still provide: Secret Manager, the media bucket,
Vertex AI / Gemini, Firebase (Auth, FCM, Crashlytics), Cloud Logging and Cloud
Monitoring, and — in prod — the alert-to-issue Cloud Functions
(`cloud_functions/`).

## Remote state buckets

| Environment | Bucket | Project |
|---|---|---|
| dev | `gs://ripls-dev-terraform-state` | `ripls-dev` |
| prod | `gs://ripls-prod-terraform-state` | `ripls-prod` |

Operators need `roles/storage.objectAdmin` on the bucket for their environment.

## What each environment's Terraform manages

- `module.server_identity` — the `ripls-server-mimir` service account the
  server on mimir authenticates as, with secretAccessor on each runtime secret
  (`local.server_runtime_secrets` plus the feedback signer key), object access on
  the media bucket, and the Vertex AI, FCM and Cloud Logging roles. Its key is
  created out of band (the secret-rotation runbook) and lives on mimir at
  `/etc/nas/ripls-server-<env>-sa.json`. The `server_identity_email` output
  names the account.
- `module.media_bucket` — the `ripls-<env>` media bucket, with
  `prevent_destroy`.
- The feedback URL signer service account and its secret container.
- `module.monitoring` — log-based metrics, alerts, SLOs, dashboards and the
  uptime check, reading mimir's shipped logs (`log_source = "self_hosted"`).
- Cloudflare DNS records for the server and web-app hostnames.
- GitHub Actions secret grants for build-time secrets, and its Vertex AI role.
- Prod only: the account-level billing budget ($40/month), and the Cloud
  Functions runtime and deploy grants on `github-app-alerts-private-key-base64`.

## Initial setup

1. **Install the gcloud CLI**: https://cloud.google.com/sdk/docs/install#installation_instructions

2. **Create GCP projects**: `ripls-dev` and `ripls-prod` (globally unique names).

3. **Authenticate**:
   ```bash
   gcloud auth application-default login
   ```

4. **Enable Secret Manager** in both projects (Terraform reads and grants on
   secrets, so it must be on before the first apply):
   ```bash
   gcloud services enable secretmanager.googleapis.com --project=ripls-dev
   gcloud services enable secretmanager.googleapis.com --project=ripls-prod
   ```
   Terraform enables `aiplatform`, `iamcredentials` and `playintegrity` itself,
   and `billingbudgets` in prod. Prod's Cloud Functions (gen2) run on Cloud Run
   and keep their images in the `gcf-artifacts` Artifact Registry repository, so
   `run` and `artifactregistry` stay enabled in prod for them.

   Disabled after the #3079 decommission: `sqladmin`, `vpcaccess` and
   `servicenetworking` in both projects, and `run` and `artifactregistry` in
   `ripls-dev`.

### Secret Management Setup

5. **Create secrets in Secret Manager.** The server's runtime secrets are the
   `server_runtime_secrets` list in each environment's `main.tf`; the full
   inventory, with what reads each one, is kept with the deployment's private
   docs. For example:
   ```bash
   gcloud secrets create mailgun-api-key --project=ripls-prod
   echo -n "your-mailgun-api-key" | gcloud secrets versions add mailgun-api-key --data-file=- --project=ripls-prod

   # GitHub App private key for the feedback bot (base64-encoded PEM, single line)
   base64 -w0 path/to/ripls-feedback-bot.pem > /tmp/key.b64
   gcloud secrets create github-app-private-key-base64 --project=ripls-dev
   gcloud secrets versions add github-app-private-key-base64 --data-file=/tmp/key.b64 --project=ripls-dev

   gcloud secrets create github-app-private-key-base64 --project=ripls-prod
   gcloud secrets versions add github-app-private-key-base64 --data-file=/tmp/key.b64 --project=ripls-prod
   rm /tmp/key.b64
   ```

   **Important**:
   - Use `echo -n` to avoid trailing newlines in secret values
   - The GitHub App private key is a PEM file — base64-encode it and use
     `--data-file`, not `echo`
   - The database password is not in Secret Manager: each stack's Postgres
     password lives on mimir in `/etc/nas/stack-env/ripls_server_<env>.env`

### Terraform Deployment

6. **Initialize and apply Terraform for both environments**:
    ```bash
    cd terraform/environments/dev/
    terraform init
    terraform plan
    terraform apply

    cd ../prod/
    terraform init
    terraform plan
    terraform apply
    ```

   Variables have defaults in `variables.tf`, so no `.tfvars` files are needed.
   A Cloudflare API token with DNS edit permission must be set as
   `CLOUDFLARE_API_TOKEN`.

### Domain Configuration

7. **DNS is managed in Cloudflare by Terraform.** Each environment has two
   records, CNAMEs to mimir's public name, unproxied so Caddy on mimir sees the
   real client connection and terminates TLS (Cloudflare's proxy would also cut
   the server's long-lived streams):

   | Environment | Hostnames |
   |---|---|
   | dev | `server-dev.ripls.org`, `dev.ripls.app` |
   | prod | `server.ripls.org`, `ripls.app` |

   The `ripls.app` and `dev.ripls.app` records were created by hand and are
   brought under Terraform with `import` blocks. The Caddy entries themselves
   live in the mimir repo, not here.

## Server deployment

There is no container registry and no image promotion between environments.
`.github/workflows/build_and_deploy_container.yaml` builds the image on a
self-hosted runner on mimir, loads it straight into the host's Docker daemon
as `ripls-server:<env>-<short sha>`, and runs `/opt/ripls/server/deploy.sh`
over SSH. `deploy.sh` points the stack at the new tag, waits for `/readyz`,
and rolls back to the previous image if it never becomes healthy; a failed
deploy fails the run.

| Trigger | Workflow | Environment |
|---|---|---|
| Push to `main` | `deploy_to_dev_on_push.yaml` | dev |
| Manual dispatch | `deploy_manual.yaml` (`Manually Deploy a Branch`) | dev or prod |
| Release-cut PR merged | `release_finalize.yaml` | prod |

Each environment builds its own image, because the bundled Flutter Web build
differs per environment.

**Rollback:** redeploy an earlier tag on the host, e.g.
`/opt/ripls/server/deploy.sh prod prod-<sha>`. `deploy.sh` keeps the newest ten
tags per environment plus `<env>-previous`; see
[`infra/server/README.md`](../../infra/server/README.md).

## History

Until #3079 the Terraform here also held `modules/containers` (the Cloud Run
services and their domain mappings) and `modules/storage` (Cloud SQL, its VPC,
egress subnet and private-services peering, and the media bucket). The bucket
moved to `modules/media_bucket` with `moved` blocks; the rest was destroyed. The
secrets retired with it, in both projects: `ripls-db-password`,
`mapbox-access-token`, `metabase-reader-password`, `metabase-admin-password`,
`metabase-admin-api-key` and `metrics-auth-token`.
