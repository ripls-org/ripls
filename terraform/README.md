# Terraform Infrastructure Configuration

This directory holds the Terraform for the Google Cloud and Cloudflare resources
the server depends on. It does not run the server: the server and its Postgres
run as Docker Compose stacks on a self-hosted machine (see
[`infra/server/`](../infra/server/README.md)), deployed by CI over SSH. The
server ran on Cloud Run with Cloud SQL until #3079 removed both, along with
their VPC networking and the Artifact Registry repositories.

What Terraform still manages:

- the server's runtime identity — a service account whose key the host uses —
  and its Secret Manager, Cloud Storage and project-role grants;
- the media bucket (Cloud Storage), with `prevent_destroy`;
- Cloud Monitoring: log-based metrics, alerts, SLOs, dashboards and the uptime
  check, all reading the logs the host ships to Cloud Logging;
- Cloudflare DNS records pointing the server and web-app hostnames at the host;
- the GitHub Actions and Cloud Functions secret grants, required APIs, and the
  billing budget.

## Directory Structure

```
terraform/
├── environments/          # Environment-specific configurations
│   ├── dev/              # Development environment
│   │   ├── main.tf       # Environment resource orchestration
│   │   ├── outputs.tf
│   │   └── variables.tf  # Environment-specific variable definitions
│   └── prod/             # Production environment
│       ├── main.tf
│       ├── outputs.tf
│       └── variables.tf
└── modules/              # Reusable infrastructure modules
    ├── media_bucket/     # Cloud Storage bucket for user-uploaded media
    ├── monitoring/       # Log-based metrics, alerts, SLOs, dashboards, uptime check
    └── server_identity/  # Server service account and its grants
```

## Architecture Overview

### Module Design Philosophy

**Media Bucket Module**: The Cloud Storage bucket for user-uploaded media. It
carries an unconditional `prevent_destroy`, so the bucket and its data outlive
any change to how the server is hosted.

**Server Identity Module**: A service account for running the server outside
Google Cloud, where there is no metadata server and the server authenticates
with a key. It grants exactly what the server calls with its own identity:
secretAccessor on each runtime secret, object access on the media bucket, and
the Vertex AI, FCM and Cloud Logging roles. The key is created out of band so it
never lands in Terraform state.

**Monitoring Module**: Reads the server's own logs. Its `log_source` variable
says whether they come from Cloud Run or from a self-hosted server whose log
shipper writes them as `generic_task` entries; both environments use
`self_hosted`.

**Environment Orchestration**: Each environment directory composes modules with
environment-specific parameters, allowing identical infrastructure patterns with
different configurations.

## Configuration Approach

### Environment Separation

**Development Environment**: Its own project, secrets, bucket, DNS records and
monitoring, so dev changes never touch prod resources.

**Production Environment**: The same shape, plus the account-level billing
budget and the Cloud Functions secret grants, which live only in prod.

### Variable Management

Environment-specific variables are defined in each environment's `variables.tf`.
Sensitive values (passwords, secrets) should be provided via `.tfvars` files or
environment variables, not committed to version control.

### Module Communication

Modules communicate through Terraform's input/output system: the media bucket
module outputs the bucket name, which the server identity module takes to scope
its storage grants. Environment configurations orchestrate this data flow.

## Remote State

Terraform state is stored in a GCS bucket per environment, with versioning and
state locking. Each environment declares its own bucket in its `backend` block;
the buckets themselves are deployment-specific and are not named here.

State is loaded automatically on `terraform init`. All operators see the same
state, and concurrent `terraform apply` calls are blocked by native GCS
locking.

**First-time setup for a new operator:**

```bash
cd terraform/environments/dev   # or prod
terraform init                  # downloads remote state automatically
terraform plan                  # verify you see the current infrastructure
```

**If a lock gets stuck** (e.g., operator's machine crashes during apply):

```bash
terraform force-unlock <LOCK_ID>
```

**Access requirements:** Operators need `roles/storage.objectAdmin` on the
state bucket for their environment.

## Local Development and Testing

### Prerequisites

- Terraform CLI installed
- Cloud provider CLI tools configured
- Appropriate authentication credentials set up

### Testing Commands

The project includes npm scripts for consistent testing:

```bash
# Format and validate configurations
npm run lint:terraform

# Test deployment plans without applying
npm run test:terraform

# Test specific environments
npm run plan:terraform:dev
npm run plan:terraform:prod
```

### Configuration Validation

The testing workflow validates:

- Terraform syntax and structure
- Module references and variable passing
- Provider configuration compatibility
- Resource dependency resolution

This local testing provides confidence before any cloud resources are created or
modified.

### Deployment Workflow

1. **Format**: Ensure consistent code style
2. **Validate**: Check syntax and configuration structure
3. **Plan**: Preview changes without applying them
4. **Apply**: Execute the planned changes (manual step)

The first three steps are automated through npm scripts and suitable for CI/CD
integration.

## Security and Best Practices

### Secret Management

- Secret values live in Secret Manager and are created out of band; Terraform
  manages only the grants (and, for the feedback signer key, the empty secret
  container), so no
  secret value enters Terraform state
- Service-account keys are likewise created out of band
- `.tfvars` files excluded from version control
- Environment variables for CI/CD authentication

### Resource Management

- Secret and storage grants are per secret and per bucket rather than
  project-wide
- The media bucket is protected with `prevent_destroy` in every environment

This configuration provides a foundation for reliable, secure, and
cost-effective infrastructure management while maintaining flexibility for
different deployment targets.
