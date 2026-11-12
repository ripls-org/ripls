# Issue #1770: security: move Cloud Run env-var secrets to Secret Manager refs

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1770
**Filed:** 2026-05-07 | **Last activity:** 2026-05-09
**Labels:** none | **Assignees:** briangbrown
**Context docs:** none

## Problem Statement

The issue (filed 2026-05-07) inventoried 12 secret-shaped values passed to
Cloud Run as **plaintext env vars** in `terraform/modules/containers/main.tf`
and `terraform/modules/metabase/main.tf`, and asked that all of them be
converted to `value_source.secret_key_ref` so that:

1. Secret values stop appearing in Cloud Run service config (visible to
   anyone with Project Viewer / Cloud Run Developer IAM).
2. Secrets stop transiting through Terraform state in plaintext.
3. Rotation becomes "add a new Secret Manager version + redeploy", with no
   Terraform variable changes required.
4. The GCP console warning *"An environment variable in service server
   (us-central1) might contain a password."* disappears.

**The issue description is now materially stale.** Reading the current code:

- `terraform/modules/containers/main.tf` no longer carries
  `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `MAILGUN_API_KEY`,
  `UNSPLASH_ACCESS_KEY`, `PEXELS_API_KEY`, `PIXABAY_API_KEY`,
  `MAPBOX_ACCESS_TOKEN`, `JWT_SIGNING_SECRET`, `GITHUB_APP_PRIVATE_KEY`,
  or `METRICS_AUTH_TOKEN` as env vars. They were removed in the
  #1768/#1770 work and are now fetched at container startup by
  `server/entrypoint.sh` using the runtime SA's metadata-server token,
  then passed to the Go binary as command-line flags. Comment block at
  containers/main.tf:172-181 documents this.
- The IAM wiring (`google_secret_manager_secret_iam_member.server_runtime_sa`
  with a `for_each` over `local.server_runtime_secrets`) already exists in
  both `terraform/environments/dev/main.tf` and `terraform/environments/prod/main.tf`.
- Issue #1885 separately tracks the residual hardening (cmdline flags
  visible in `/proc/cmdline`), so that surface area is out of scope here.

**What actually remains for #1770 is two values:**

1. `DB_PASSWORD` — `terraform/modules/containers/main.tf:55-58`.
   Wired in as `value = var.database_password`. The application database
   password.
2. `MB_DB_PASS` — `terraform/modules/metabase/main.tf:95-98`.
   Wired in as `value = var.metabase_db_password`. The Metabase metadata
   database password.

Both currently flow `Secret Manager → data.google_secret_manager_secret_version
→ module input variable → Cloud Run env`, materializing the plaintext into
service config and into Terraform state along the way.

Note that the **GCP console warning** is what motivated this whole umbrella in
the first place (per the issue body), and `DB_PASSWORD` is almost certainly
the env var that triggered it. Closing this issue should make that warning
disappear.

**Wrinkle surfaced by audit:** dev and prod use **different Secret Manager
secret names** for the application DB password. `terraform/environments/dev/main.tf:40`
points at `gear-db-password`; `terraform/environments/prod/main.tf:40`
points at `ripls-db-password`. The rotation runbook
(`docs/secret-rotation.md:203`) documents `gear-db-password` (the dev name).
This divergence means the `containers` module can't hardcode the SM secret
name in `secret_key_ref.secret` — it must take the secret ID as a new module
input wired per-environment. Same shape for metabase (both envs use
`metabase-admin-password` so it could be hardcoded, but for consistency
treat it the same way).

## Context from Comments

The issue itself has **zero comments** — no clarifications, no maintainer
direction, no disagreements. That's a planning signal: the original framing
("convert all 12 to `secret_key_ref`") wasn't revised in writing as the
parallel work in #1768 (entrypoint.sh fetch) reshaped the scope. The
umbrella issue #1766 sequencing also assumed the env-var conversion was
unstarted when in fact the bulk of it had already been absorbed by the
entrypoint approach. Worth a comment on the umbrella when this lands so the
remaining post-cutover work (#1883, #1771) reflects reality.

## Potential Solutions

### Option A: Native `secret_key_ref` for the two remaining DB passwords

- **Approach:** In `terraform/modules/containers/main.tf:55-58` and
  `terraform/modules/metabase/main.tf:95-98`, replace each `env { value = ... }`
  with `env { value_source { secret_key_ref { secret = var.<name>_secret_id,
  version = "latest" } } }`. Add two new module inputs
  (`db_password_secret_id`, `metabase_db_password_secret_id`) so the
  dev/prod-divergent SM secret names (`gear-db-password` vs
  `ripls-db-password` for the app DB) can be wired per-environment. Add the
  same secret IDs to `local.server_runtime_secrets` in each env-level main.tf
  so the existing `for_each` IAM grant covers them. Leave the
  `data "google_secret_manager_secret_version"` blocks in place — they're
  still needed because `module.storage` (storage/main.tf:114) and
  `google_sql_user.metabase_admin` (metabase/main.tf:23) consume the plaintext
  to set the actual SQL user password. We can stop *passing the plaintext to
  Cloud Run* without removing it from Terraform's flow entirely.
- **Pros:** Canonical Cloud Run pattern. Makes the GCP console warning go
  away. Removes the value from Cloud Run service config. Aligns metabase
  (which can't use entrypoint.sh — image isn't ours) with how the rest of
  the secrets are handled. Cloud Run picks up new SM versions automatically
  with `version = "latest"` — no env-var change on rotation.
- **Cons:** The DB password still appears in Terraform state because the SQL
  user resource takes plaintext, so claim (2) from the issue (no plaintext in
  TF state) is only partially resolved. Rotation still needs a `terraform
  apply` to update the SQL user — but no variable-shape change.
- **Effort:** Small. ~20 lines of Terraform across 4 files.

### Option B: Extend entrypoint.sh for `DB_PASSWORD`, leave metabase alone

- **Approach:** Move `DB_PASSWORD` fetching into `server/entrypoint.sh`
  alongside the other 10 secrets, drop the env var from `containers/main.tf`.
  Leave metabase untouched (or do A just for metabase).
- **Pros:** Consistent with the server's existing pattern.
- **Cons:** Adds a tenth network round-trip on every cold start of the
  server (entrypoint already fetches 10 secrets serially — adding an 11th
  isn't a meaningful regression, but isn't free either). Still requires
  Option A for metabase because we can't modify the metabase image's
  entrypoint. So we'd end up with two patterns side-by-side for the same
  problem class, which is worse than the current "all server secrets in
  entrypoint, all infra wiring stays in TF" division.
- **Effort:** Small, but with a code-shape downside.

### Option C: Move to Cloud SQL IAM auth (no DB passwords at all)

- **Approach:** Replace password-based Postgres auth with Cloud SQL IAM
  database authentication. The Cloud Run runtime SA gets `roles/cloudsql.client`
  and `roles/cloudsql.instanceUser`, and the Postgres server uses the runtime
  SA's identity directly via a short-lived IAM access token.
- **Pros:** Eliminates the secret entirely. No rotation cost. Strongest
  posture.
- **Cons:** Material scope creep. Requires reworking `server/storage/`
  connection plumbing to use IAM tokens (refresh on expiry, etc.), Cloud SQL
  Proxy or `cloud-sql-go-connector`, and Metabase's IAM-auth driver
  configuration. Not what the issue asked for; belongs in its own issue if
  we want it.
- **Effort:** Large (multi-week).

## Recommendation

**Option A.** Two `env` blocks change, one IAM list grows by two strings,
and the GCP console warning that triggered the umbrella goes away. Metabase
*has* to use this approach (no entrypoint to extend), so consistency between
the two services lands us here regardless. The residual "DB password still
exists in TF state" concern is real but is a Cloud-SQL-IAM-auth conversation
(Option C), not a `secret_key_ref` conversation, and should be a separate
issue if we want to pursue it.

Specifically: don't touch entrypoint.sh, don't churn the 10 already-migrated
server secrets, and don't try to drop the existing `data` blocks (the SQL
user resources still need the plaintext).

## Open Questions

- **Should the `database_password` Terraform input variables on the
  `containers` and `metabase` modules be removed after this lands?** —
  Recommendation: **remove from `containers` module signature** (no other
  consumer in that module once the env block changes); **keep on `metabase`
  module signature** because `google_sql_user.metabase_admin` at
  metabase/main.tf:23 also needs the plaintext to create the SQL user. The
  module input for `database_password` on `storage` (used by
  `google_sql_user`) likewise stays. Net effect: one variable removal.
- **Does the metabase service use the same default compute SA as the server,
  or its own?** — Confirmed by reading prod/main.tf: both use the default
  compute SA (`${project_number}-compute@developer.gserviceaccount.com`),
  bound in `local.cloud_run_runtime_sa`. So one `for_each` grant covers both
  services' secret access. Recommendation: add `db-password` and
  `metabase-admin-password` to the existing `local.server_runtime_secrets`
  list rather than introducing a parallel structure.
- **Use `version = "latest"` or pin to a specific version?** —
  Recommendation: `"latest"`. The whole point of the migration is that
  rotation can be a SM version bump + redeploy without Terraform churn.
  Pinning defeats that. The two consumers we already have in entrypoint.sh
  use `/versions/latest`, so this stays consistent.
- **Do we need to verify any reference to `MB_DB_PASS` or `DB_PASSWORD` env
  vars elsewhere (scripts, tests, local dev)?** — Recommendation: grep
  before merge. `scripts/run_server_with_sm_secrets.sh` already handles
  local dev separately from the Cloud Run path. Metabase local dev should
  be unaffected (it's not run locally as part of normal dev).
- **Where does the doc update go?** — Recommendation: **`docs/secret-rotation.md`,
  not `docs/secrets.md`.** Per the audit, `secret-rotation.md` is the
  canonical rotation runbook and already has a "Database passwords (Cloud
  SQL)" section (line 197+) with both Pre-#1770 and Post-#1770 procedures.
  The edit needed is to **delete the Pre-#1770 branches** at lines 110-114
  (JWT) and 229 (DB passwords) once Phase 2 lands — they become obsolete.
  No new content is required; the post-cutover text is already written.
  Add the same Pre→Post cleanup for any other rotation entries that still
  carry the Pre-#1770 conditional.
- **Should the dev/prod SM-secret-name divergence be standardized in this
  work?** — Recommendation: **no, file a separate small issue.** Renaming
  `ripls-db-password` → `gear-db-password` (or vice versa) in prod means a
  short SM-side dance (create the new secret, update IAM, switch
  consumers, delete the old). It's orthogonal to the env→`secret_key_ref`
  migration and dragging it into this PR would inflate the blast radius of
  the change that makes the console warning go away. The runbook already
  notes that dev/prod can diverge per-project; that property is preserved
  by the module-input shape recommended in Option A.

## Implementation Plan

### Phase 1: Convert `MB_DB_PASS` to `secret_key_ref`

**Goal:** Land the smaller of the two changes first (single env block,
single service, same SM secret name in both envs), verify in dev, prove
the pattern end-to-end without touching the larger server service.

**Tasks:**
- [ ] In both `terraform/environments/dev/main.tf` and
      `terraform/environments/prod/main.tf`, add `"metabase-admin-password"`
      to the `local.server_runtime_secrets` set so the existing
      `google_secret_manager_secret_iam_member.server_runtime_sa` `for_each`
      grants `roles/secretmanager.secretAccessor` to the runtime SA on
      that secret.
- [ ] Add a `metabase_db_password_secret_id` (string) input to
      `terraform/modules/metabase/variables.tf`. Wire it through from each
      env-level main.tf to the module call (both envs pass
      `"metabase-admin-password"`).
- [ ] In `terraform/modules/metabase/main.tf:95-98`, replace the
      `env { name = "MB_DB_PASS"; value = var.metabase_db_password }`
      block with the `value_source.secret_key_ref` form pointing at
      `var.metabase_db_password_secret_id` / `"latest"`. Keep
      `var.metabase_db_password` on the module signature (still needed by
      `google_sql_user.metabase_admin` at metabase/main.tf:23).
- [ ] Plan + apply in dev. Verify the Metabase dev service comes up clean,
      can read its metadata DB, no warning in the Cloud Run console for the
      dev metabase service.
- [ ] Plan + apply in prod. Verify the same.

**Validates:** The `secret_key_ref` plumbing works against an existing SM
secret + existing IAM-grant pattern, and Metabase tolerates the env
variable being satisfied via SM rather than direct value.

### Phase 2: Convert `DB_PASSWORD` to `secret_key_ref`

**Goal:** Eliminate the GCP console warning on the prod server service.

**Tasks:**
- [ ] In `terraform/environments/dev/main.tf`, add `"gear-db-password"` to
      `local.server_runtime_secrets`.
- [ ] In `terraform/environments/prod/main.tf`, add `"ripls-db-password"`
      to `local.server_runtime_secrets`. (Different name from dev — see
      Problem Statement.)
- [ ] Add a `db_password_secret_id` (string) input to
      `terraform/modules/containers/variables.tf`. Wire it through from
      each env-level main.tf to the `module.containers` call, passing the
      env-appropriate secret name.
- [ ] In `terraform/modules/containers/main.tf:55-58`, replace the
      `DB_PASSWORD` env block with `value_source.secret_key_ref` pointing at
      `var.db_password_secret_id` / `"latest"`.
- [ ] Drop `var.database_password` from
      `terraform/modules/containers/variables.tf` and the corresponding
      `database_password = ...` lines in the env-level main.tf where they
      feed `module.containers`. Keep the `data
      "google_secret_manager_secret_version" "db_password"` blocks — they're
      still consumed by `module.storage` (and by `outputs.tf` in dev).
- [ ] Plan + apply in dev. Hit the dev server, exercise an authenticated
      RPC that touches the DB, watch logs for any auth failure on cold
      start. Cold-start path is critical because Cloud Run resolves the
      `secret_key_ref` once per container start, not per request.
- [ ] Plan + apply in prod. Verify the GCP console warning disappears for
      the prod `server` service.

**Validates:** The original issue's primary acceptance criterion — the
console warning going away on prod — is met. Also confirms the
non-entrypoint.sh path works for the server service.

### Phase 3: Documentation + umbrella update

**Goal:** Capture the rotation recipe so future operators don't have to
re-derive it, and update #1766 / #1883 to reflect that this is done and the
description was stale.

**Tasks:**
- [ ] In `docs/secret-rotation.md`, delete the "Pre-#1770" branches that
      are now obsolete: lines 110-114 (JWT rotation) and line 229 (DB
      passwords). Keep the Post-#1770 procedure as the sole path. Sweep
      the rest of the file for any other `Pre-#1770` / `Post-#1770`
      conditionals introduced when the runbook was written ahead of this
      change.
- [ ] In `docs/secrets.md`, update any reference that still says
      "DB_PASSWORD is passed as a plaintext env var" to reflect the
      `secret_key_ref` reality.
- [ ] Comment on #1766 noting that the original 12-item list in #1770 was
      stale — 10 secrets had already been handled by the entrypoint.sh
      approach in #1768; only `DB_PASSWORD` and `MB_DB_PASS` remained, and
      both are now on `secret_key_ref`.
- [ ] Close #1770.
- [ ] Re-read #1883's scope ("post-cutover docs and cleanup after SM-fetch
      lands") and prune any items now covered by this PR's docs update.
- [ ] After this PR lands, follow up on #1935 (filed for the dev/prod SM
      secret-name standardization — `gear-db-password` vs
      `ripls-db-password`). Once #1935 is done, the per-env
      `db_password_secret_id` module input added here can collapse back
      to a hardcoded constant.

**Validates:** Future readers (and the umbrella tracker) reflect the actual
post-migration state rather than the planning-time state.

## Related Issues

- **#1766** — the umbrella. Update its checklist after Phase 3.
- **#1768** — already closed; absorbed 10 of the 12 originally-listed
  secrets into the entrypoint.sh fetch pattern. Important context for
  understanding why #1770's description is stale.
- **#1883** — post-cutover docs and cleanup after #1768/#1770. Some of its
  scope is satisfied by Phase 3 of this plan; the rest (broader docs sweep)
  is a separate cleanup pass.
- **#1885** — pass server credentials via file paths instead of cmdline
  flags. Orthogonal to this issue, but tightens the same threat surface
  (credentials visible in `/proc/cmdline` even after they leave the env).
- **#1771** — git-history rewrite. Last item in the umbrella's Phase 3;
  blocked on all of Phase 2 (including this issue).
- **#1284** — closed; prior JWT hardcoded-secret report. Historical
  context only.
- **#1935** — standardize dev/prod SM secret name for the app DB password
  (`gear-db-password` vs `ripls-db-password`). Spun off from this plan's
  audit. Follow-up after #1770 lands; collapses the per-env module input
  back to a constant.

## Audit log

Run of `/ripls-audit` against this plan (2026-05-15) surfaced three
material fixes, all applied in-place above:

1. **Wrong SM secret name for prod `DB_PASSWORD`.** The plan originally
   said "add `db-password` to `local.server_runtime_secrets`". Actual
   secret names are `gear-db-password` (dev) and `ripls-db-password`
   (prod). Because the names diverge per environment, the `containers`
   module cannot hardcode the name and must take it as a new input
   (`db_password_secret_id`) wired per-env. Problem Statement now flags
   the divergence; Option A and the Phase 2 task list reflect the
   module-input shape.
2. **Doc-update target was wrong.** Phase 3 originally added a new
   subsection to `docs/secrets.md`. The canonical runbook is
   `docs/secret-rotation.md`, which already contains the correct
   post-cutover DB-password rotation procedure (line 230) and a
   matching post-cutover JWT procedure (line 116). The Pre-#1770
   branches in both sections become obsolete once Phase 2 lands; Phase
   3 task list updated to delete those branches rather than author new
   content.
3. **Open question added for SM-secret-name standardization.**
   `gear-db-password` (dev) vs `ripls-db-password` (prod) is a real
   inconsistency in the underlying infra that the rotation runbook
   already trips on (`docs/secret-rotation.md:203` documents only the
   dev name). Recommendation is to file a separate small issue rather
   than absorb it into this PR.

Reference docs reviewed: `docs/client/{architecture,caching,i18n,
accessibility,testing}.md` and `docs/server/{architecture,observability}.md`.
None of them carry guidance applicable to a Terraform-only change — the
plan does not touch service code, RPC handlers, storage, structured
logging, client widgets, or tests. No compliance findings from the
reference docs.
