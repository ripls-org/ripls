# Issue #1074: Move Terraform state to a remote backend (GCS)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1074
**Filed:** 2026-04-09 | **Last activity:** 2026-04-09
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

Terraform state files are stored locally on developer machines, causing state
drift between operators, no locking against concurrent applies, a bus-factor
risk if a laptop is lost, and unencrypted secrets sitting in local `.tfstate`
files. The issue is not theoretical — during the Metabase deployment (#1072),
state had to be manually copied between developers, and a stale lock from a
crashed process had to be manually cleared.

Additionally, `.tfstate` files are currently tracked in git despite being in
`.gitignore` (likely force-added at some point). This means secrets are in
the repo history.

## Context from Comments

No comments on this issue. The issue body is comprehensive and includes a
complete implementation plan with code examples. No ambiguities to resolve.

## Potential Solutions

### Option A: GCS backend (recommended in the issue)

- **Approach:** Single GCS bucket in `ripls-dev` with per-environment prefixes
  (`environments/dev`, `environments/prod`). Native state locking via GCS.
- **Pros:** Standard GCP approach. Native locking. Versioned bucket for
  recovery. Encrypted at rest by default. All operators see the same state.
  Zero cost for the bucket size involved.
- **Cons:** Requires all operators to have `storage.objectAdmin` on the
  bucket. One-time migration step that must be coordinated (only one person
  should migrate each environment).
- **Effort:** Small.

### Option B: Separate buckets per project

- **Approach:** `gs://ripls-terraform-state-dev` in `ripls-dev` and
  `gs://ripls-terraform-state-prod` in `ripls-prod`.
- **Pros:** Stronger isolation — dev operators can't accidentally touch prod
  state. IAM is per-project, so existing project permissions apply naturally.
- **Cons:** Two buckets to manage. Slightly more setup. For a 3-person team,
  the isolation benefit is minimal.
- **Effort:** Small.

### Option C: Terraform Cloud / HCP

- **Approach:** Use HashiCorp's managed state backend.
- **Pros:** State locking, versioning, audit log, run history, and a UI for
  free (up to 5 users).
- **Cons:** External dependency. Adds latency to plan/apply. Requires HCP
  account setup. Goes against the project's preference for self-hosted infra.
- **Effort:** Small-medium (account setup + migration).

## Recommendation

**Option B: Separate buckets per project.** Each project owns its own state
bucket, providing clean isolation between environments. An operator working
on dev can't accidentally corrupt prod state. IAM follows existing project
boundaries — if someone has access to `ripls-prod`, they can manage prod
state; if not, they can't. This is the right default as the team grows.

Buckets (GCS names are globally unique, so they include the project name):
- `gs://ripls-dev-terraform-state` in `ripls-dev` (dev state)
- `gs://ripls-prod-terraform-state` in `ripls-prod` (prod state)

## Open Questions

- **Who has the most current state for each environment?** The migration must
  run from the machine with the most up-to-date `.tfstate`. — Recommendation:
  Brian ran the most recent `terraform apply` for both dev and prod (during
  the Metabase deployment), so Brian should run the migration.

- **Should we remove `.tfstate` files from git history?** They're currently
  tracked and contain secrets. — Recommendation: Yes, but as a separate step
  after migration. Use `git rm --cached` to untrack them, then consider
  `git filter-repo` to purge history if the secrets are sensitive enough to
  warrant it. The DB passwords and API keys in the state files are the same
  ones in Secret Manager, so rotating them after history cleanup is an option.

- **Should CI/CD run Terraform?** The issue mentions granting the CI/CD
  service account access. — Recommendation: Not yet. Keep Terraform applies
  manual for now. Grant CI/CD access later when automated deployments are
  needed. The bucket access for CI/CD is a one-line IAM change when ready.

## Implementation Plan

### Phase 1: Create bucket and migrate state

**Goal:** State stored remotely with locking. All operators see the same state.

**Tasks:**
- [x] Create `gs://ripls-dev-terraform-state` bucket in `ripls-dev`:
  - Location: `us-central1`
  - Uniform bucket-level access
  - Public access prevention enforced
  - Versioning enabled
- [x] Create `gs://ripls-prod-terraform-state` bucket in `ripls-prod`:
  - Same settings as dev
- [x] Add `backend "gcs"` block to `terraform/environments/dev/main.tf`:
  `bucket = "ripls-dev-terraform-state"`, `prefix = "state"`
- [x] Add `backend "gcs"` block to `terraform/environments/prod/main.tf`:
  `bucket = "ripls-prod-terraform-state"`, `prefix = "state"`
- [x] Run `terraform init -migrate-state` for dev (from Brian's machine)
- [x] Run `terraform init -migrate-state` for prod (from Brian's machine)
- [x] Verify `terraform plan` shows no changes for both environments
- [ ] Grant `roles/storage.objectAdmin` on each bucket to operators who
  manage that environment

**Validates:** Both environments use remote state. `terraform plan` from any
operator's machine shows the same state. Concurrent applies are blocked by
locking.

### Phase 2: Clean up local state and update docs

**Goal:** No local state files. Documentation updated.

**Tasks:**
- [x] Verify `.gitignore` already covers `*.tfstate` and `*.tfstate.*`
  (confirmed — `.tfstate` files are not tracked in git)
- [x] Delete local `.tfstate` and `.tfstate.backup` files from both
  environments
- [x] Update `terraform/README.md`:
  - Added "Remote State" section explaining the GCS backend
  - Added first-time setup instructions for new operators
  - Added `terraform force-unlock` instructions
- [ ] Notify Bryan that state is now remote — he should delete his local
  `.tfstate` files and re-run `terraform init`

**Validates:** No local state files exist. Documentation is current. All
operators can run `terraform plan` without manual state coordination.

## Related Issues

- #1072 — Metabase analytics deployment surfaced the state management pain
  (manual state file copying, stale lock from crashed process)
