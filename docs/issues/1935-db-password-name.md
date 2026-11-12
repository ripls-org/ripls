# Issue #1935: security: standardize dev/prod Secret Manager name for the application DB password

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1935
**Filed:** 2026-05-15 | **Last activity:** 2026-05-15
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The application database password lives under different Secret Manager
secret IDs in the two environments:

- **dev** (`ripls-dev`): `gear-db-password` (`terraform/environments/dev/main.tf:40`)
- **prod** (`ripls-prod`): `ripls-db-password` (`terraform/environments/prod/main.tf:40`)

The divergence is purely historical and confers no benefit. Standardize
on the prod name (`ripls-db-password`) in both projects: it matches the
broader `ripls-*` naming convention used in prod for the SQL instance,
database, and user, and it's the one that's actually correct in the
runbook for the higher-stakes environment. The price of the current
divergence shows up in three places:

- `docs/secret-rotation.md:196` documents only `gear-db-password` for
  `DB_PASSWORD` (the dev name). An operator following the runbook
  against prod gets a confusing error or an orphan secret.
- `terraform/README.md:251` carries a permanent footnote explaining
  the divergence as a workaround.
- The `containers` module had to gain a new `db_password_secret_id`
  input in #1770 specifically because the names diverge.

The goal is mechanical: have both environments resolve `DB_PASSWORD`
from a secret named `ripls-db-password` in their respective projects,
delete `gear-db-password` in dev, and remove the workaround text from
docs and Terraform.

**Direction note:** earlier drafts of this plan recommended converging
on `gear-db-password`. Reversed 2026-05-15 (per the issue thread) to
converge on `ripls-db-password` — keeping the prod name avoids touching
the higher-stakes environment and aligns with the dev SQL standardization
filed separately (`gear-db-dev` instance / `gear_library` DB / `gear_user`
user → `ripls-dev` / `ripls` / `ripls_user`).

## Context from Comments

The issue body was rewritten 2026-05-15 to flip the direction of the
rename (now: dev → prod's name, not the reverse). No external comments
yet. The body's "Proposed fix" section is the source of truth for the
recipe; this plan exists to nail down the safety properties of each
step (matching values before apply, IAM-grant timing, the Open Question
on the per-env module input).

## Codebase audit (2026-05-15)

Live audit of where `gear-db-password` is referenced (the secret being
retired):

```
terraform/environments/dev/main.tf:40     secret = "gear-db-password"           # data.google_secret_manager_secret_version
terraform/environments/dev/main.tf:84     "gear-db-password",                   # local.server_runtime_secrets list
terraform/environments/dev/main.tf:217    db_password_secret_id = "gear-db-password"   # module.containers wiring
README.md:253                             gcloud secrets versions access latest --secret="gear-db-password" ...
terraform/README.md:226-227               gcloud secrets create gear-db-password ...   # setup docs
terraform/README.md:251                   "Production uses different secret names..."
docs/secret-rotation.md:196               | DB_PASSWORD | ripls | gear-db-password |
docs/secret-rotation.md:221               gcloud secrets versions add gear-db-password ...
docs/secrets.md:30                        "gear-db-password in ripls-dev, ripls-db-password in ripls-prod"
```

Plus the dev/prod-divergence comments in
`terraform/environments/prod/main.tf:142,277-278` that point readers
at this issue.

Live IAM policy on `gear-db-password` in dev
(`gcloud secrets get-iam-policy gear-db-password --project=ripls-dev`):

```
members:    serviceAccount:685679309377-compute@developer.gserviceaccount.com
role:       roles/secretmanager.secretAccessor
```

**Only one IAM grant**, on the Cloud Run runtime SA, managed by the
`for_each` over `local.server_runtime_secrets` in
`terraform/environments/dev/main.tf:84`. No scripts, no other
services, no other accounts. Migration scope is small.

## Potential Solutions

### Option A: Same-value cutover (recommended)

- **Approach:**
  1. Manually `gcloud secrets create ripls-db-password` in `ripls-dev`
     and pipe the current `gear-db-password` value into it as version 1.
     Done outside Terraform — secrets aren't TF-managed in this repo
     (they're created via gcloud per `terraform/README.md:226-231`).
  2. Update three lines in `terraform/environments/dev/main.tf`:
     `data.google_secret_manager_secret_version.db_password.secret`
     (line 40), the entry in `local.server_runtime_secrets` (line 84),
     and the `db_password_secret_id` arg on `module.containers`
     (line 217). All three change `"gear-db-password"` →
     `"ripls-db-password"`.
  3. `terraform apply` in dev. The Terraform dependency graph adds
     the new IAM grant on `ripls-db-password` *before* the
     `google_cloud_run_v2_service` update that references it, so the
     new revision can resolve `secret_key_ref` from the moment it
     boots. The old IAM grant on `gear-db-password` is dropped after
     the new revision is healthy.
  4. Smoke-test dev server cold-start.
  5. Delete `gear-db-password` in dev via gcloud.
  6. Sweep docs (`README.md`, `terraform/README.md`,
     `docs/secret-rotation.md`, `docs/secrets.md`, plus the
     divergence comments in `terraform/environments/prod/main.tf`)
     for stale `gear-db-password` references.
- **Pros:** Safe by construction — both secrets hold the same value
  during the cutover, so the Postgres user's password never actually
  rotates. The two Cloud Run revisions (old + new) can both connect
  to the DB during the rollover window. No coordination with the SQL
  layer is needed. Reversible up to step 5. Operating on **dev only**;
  prod is untouched.
- **Cons:** Requires populating the new secret *exactly* from the old
  one — a one-byte mismatch and the new revision fails to connect.
  Mitigation: a single pipe (`access latest | versions add`) avoids
  any human transcription.
- **Effort:** Small. One commit changing three lines + a docs sweep.
  Manual gcloud steps before and after the apply.

### Option B: Rotate-and-rename (don't)

- **Approach:** Generate a fresh password, set it on the dev Postgres
  user via `gcloud sql users set-password`, create `ripls-db-password`
  in dev with the new value, retire `gear-db-password`.
- **Pros:** Doubles as a routine password rotation.
- **Cons:** Conflates a naming cleanup with a credential rotation —
  two changes to debug if anything goes wrong. The old Cloud Run
  revision can't fall back to the previous password if the new
  revision fails to come up, because the SQL user password has
  already moved. Worse rollback story than Option A.
- **Effort:** Small (about the same as A) but materially riskier.

### Option C: Keep both names indefinitely with an alias secret

- **Approach:** Leave `gear-db-password` in place forever; create
  `ripls-db-password` in dev with the same value; have a small CI/CD
  job keep them in sync.
- **Pros:** Zero risk of breakage.
- **Cons:** Two secrets to rotate per change, ongoing drift risk,
  defeats the purpose. Pure cargo cult.
- **Effort:** Permanent — the cost is forever.

## Recommendation

**Option A.** The same-value-then-rename shape is the canonical
"rename a secret" recipe and the IAM/cutover ordering naturally falls
out of Terraform's dependency graph. The only real risk is value
mismatch during the manual gcloud step, and that risk collapses to
zero if the new secret's first version is populated by piping
directly from the old secret (no human-typed value).

The plan keeps Option A's three phases small enough that each one is
trivially reviewable in isolation.

## Open Questions

- **Should the `db_password_secret_id` module input be removed after
  both envs use the same name?** — **My recommendation diverges from
  the issue body, which says "simplify it back to a hardcoded constant."**
  I recommend **keeping the input in place**: the per-env wiring is
  a one-line cost in each env's main.tf, and removing the input churns
  the module API to save those two lines. Counter-argument (the issue
  body's position): we added the input *specifically because* dev and
  prod diverged, and the entire point of this issue is to make them
  converge — once they have, the input is dead weight added in
  response to a problem that no longer exists. Either position is
  defensible; flagging it here so reviewers can decide explicitly
  rather than have one choice quietly inherit from the other PR.
  (Note: the metabase module also has a `metabase_db_password_secret_id`
  input that doesn't carry divergent values today. It exists because
  of the parallel structure with `db_password_secret_id` added in
  #1770, not because metabase independently needs the input. If we
  remove `db_password_secret_id` here, removing `metabase_db_password_secret_id`
  in the same PR for symmetry would be natural.)
- **Pin the new secret's first version to a specific Terraform plan
  apply, or just rely on `version = "latest"`?** — Recommendation:
  **`latest`** to stay consistent with how the other 11 SM secrets
  the server consumes are wired. Cloud Run picks up new versions at
  revision-creation time, which is exactly what the apply step
  triggers anyway.
- **Do we need any coordination with active in-flight requests
  during the apply?** — Recommendation: **no special coordination**.
  The same-value cutover means the Postgres user password never
  changes; the only behavior change is which named secret Cloud Run
  reads at revision creation. The brief connection-error window on
  the draining revision (already documented in the rotation runbook)
  is the same as any Cloud Run revision rollover.
- **What about applying the rename in prod too?** — Recommendation:
  **no-op**. Prod already uses `ripls-db-password`. This issue only
  affects dev.
- **Delete `gear-db-password` immediately after apply, or wait?** —
  Recommendation: **wait at least one business day** between Phase 2
  (apply) and Phase 3 (delete) so that if the new revision is rolled
  back for any unrelated reason, the old revision can still resolve
  the secret. Cloud Run's max revision retention is 100 — the old
  revision will still exist after a day.
- **Coordinate with the larger dev SQL standardization issue (filed
  separately)?** — Recommendation: **land this first, independently.**
  The secret rename here is a no-data-loss same-value cutover. The
  larger rename (instance / database / user from `gear-*` to `ripls-*`
  on the dev side) is destructive on dev data and needs its own
  scheduled apply window. Once both land, the dev side is fully
  symmetric with prod.

## Implementation Plan

### Phase 1: Pre-create and populate `ripls-db-password` in dev

**Goal:** Stand up the new secret with the exact value of the old
one, before any Terraform change references it.

**Tasks (manual gcloud; no code):**
- [ ] Create the empty secret: `gcloud secrets create ripls-db-password
      --project=ripls-dev --replication-policy=automatic`
- [ ] Pipe the current value into a new version with no temp file and
      no terminal echo:
      `gcloud secrets versions access latest --secret=gear-db-password
      --project=ripls-dev | gcloud secrets versions add ripls-db-password
      --project=ripls-dev --data-file=-`
- [ ] Verify byte-level match (don't print the value; compare hashes):
      `diff <(gcloud secrets versions access latest --secret=gear-db-password
      --project=ripls-dev | sha256sum) <(gcloud secrets versions access
      latest --secret=ripls-db-password --project=ripls-dev | sha256sum)`
- [ ] Spot-check exactly one ENABLED version:
      `gcloud secrets versions list ripls-db-password --project=ripls-dev`.

**Validates:** New secret exists, is accessible, and holds the
identical value the Postgres user is already using. No Terraform
state touched.

### Phase 2: Repoint Terraform and apply dev

**Goal:** Move Cloud Run dev from `gear-db-password` to
`ripls-db-password` via a single, reviewable diff.

**Tasks (one commit, one apply):**
- [ ] In `terraform/environments/dev/main.tf:40`, change
      `secret = "gear-db-password"` → `secret = "ripls-db-password"`.
- [ ] In the `local.server_runtime_secrets` list at
      `terraform/environments/dev/main.tf:84`, replace
      `"gear-db-password"` with `"ripls-db-password"`.
- [ ] At `terraform/environments/dev/main.tf:217`, change
      `db_password_secret_id = "gear-db-password"` →
      `db_password_secret_id = "ripls-db-password"`.
- [ ] Open PR. Branch + commit per the same conventions as #1936.
- [ ] After review, `cd terraform/environments/dev && terraform plan`.
      Expected diff: ~4 changes — drop IAM grant on `gear-db-password`,
      add IAM grant on `ripls-db-password`, update
      `data.google_secret_manager_secret_version.db_password` to the
      new name, update the Cloud Run service's `secret_key_ref` to
      point at `ripls-db-password`. The Cloud Run change creates a new
      revision.
- [ ] `terraform apply`. Confirm Terraform creates the new IAM grant
      before the Cloud Run revision update (dependency graph forces
      this ordering, so the new revision can resolve `secret_key_ref`
      from boot).
- [ ] Smoke-test: hit an authenticated RPC against
      `server-dev.ripls.org` that touches the DB. Confirm a fresh
      `server-dev-NNNNN` revision is serving 100% of traffic and logs
      show no `database connection refused` or `permission denied` on
      secret access.
- [ ] Merge the PR.

**Validates:** Cloud Run dev is fully migrated to the new secret
name. The old secret still exists at this point for a worst-case
rollback (revert the PR + apply).

### Phase 3: Cleanup — delete the old secret + docs sweep

**Goal:** Remove the dead `gear-db-password` and the residual
divergence footnotes from docs.

**Tasks:**
- [ ] Wait at least one business day after Phase 2's apply so any
      need to roll back the Cloud Run revision (for unrelated
      reasons) doesn't require the old secret to still exist.
- [ ] `gcloud secrets list --project=ripls-dev --filter="name:gear-db-password"`
      to confirm the secret still exists, then
      `gcloud secrets delete gear-db-password --project=ripls-dev`.
      (One-way; the historical secret-versions data is gone after
      this.)
- [ ] Open a small docs PR sweeping the stale references:
      - `README.md:253` — update the `gcloud secrets versions access`
        example to use `ripls-db-password`.
      - `terraform/README.md:226-227` — change the `gcloud secrets
        create gear-db-password` setup commands to `ripls-db-password`.
      - `terraform/README.md:251` — remove the "Production uses
        different secret names (`ripls-db-password` instead of
        `gear-db-password`)" bullet.
      - `docs/secret-rotation.md:196` — change the SM ID column from
        `gear-db-password` to `ripls-db-password`.
      - `docs/secret-rotation.md:215-225` — simplify step 3 to a
        single `gcloud secrets versions add ripls-db-password
        --project=...` command parameterized on `--project`. The
        dev/prod-divergence note added by #1770's Phase 3 docs
        cleanup goes away.
      - `docs/secrets.md:30-31` — drop the `gear-db-password` /
        `ripls-db-password` divergence sentence introduced by #1770;
        rewrite as "the application database password
        (`ripls-db-password`) and the Metabase metadata DB password
        (`metabase-admin-password`) are mounted into the Cloud Run
        container…".
      - `terraform/environments/prod/main.tf:142,277-278` — drop the
        "ripls-db-password where dev uses gear-db-password — see
        #1935" comments. Both envs now use the same name; no
        explanatory comment needed.
- [ ] Close #1935.

**Validates:** Zero references to `gear-db-password` remain in the
repo; the rotation runbook is uniformly correct for both
environments; the per-env divergence note is gone from onboarding
docs.

## Related Issues

- **#1770** — the work that surfaced the divergence and added the
  per-env `db_password_secret_id` module input. Already merged
  (#1936); this issue closes the loop.
- **#1766** — secrets umbrella. Not blocked on this, but worth
  comment after Phase 3 noting that the umbrella's residual
  naming-cleanup item is done.
- **#1774** — rotation runbook. Phase 3 of this plan tightens the
  runbook's DB-password section to remove the dev/prod-divergence
  note added by #1770.
- **#1883** — post-cutover docs and cleanup. Phase 3 of this plan
  picks up the secret-name-specific portion; remaining items in
  #1883 are unrelated.
- **#1941** — the larger dev-side SQL standardization (instance,
  database, user → `ripls-*`). Filed as the destructive companion
  to this issue; #1935 here is its hard prerequisite (sequence step
  1 in #1941's body). #1941 should not start until #1935 is fully
  closed.

## Audit log

Run of `/ripls-audit` against this plan (2026-05-15) — two minor
fixes applied in-place, no recipe changes:

1. **Open Question on the module input was overstating the case.**
   My recommendation (keep the input) diverged from the issue body's
   recommendation (remove the input), but I'd buried that in the
   "the issue body says…" parenthetical. Rewrote the Open Question
   to lead with "**My recommendation diverges from the issue body**"
   so the reviewer sees the disagreement explicitly and can decide
   on its merits. Both positions remain documented.
2. **Metabase analogy in the recommendation was circular.** I argued
   "the metabase module already has a parallel input we left as an
   input for the same reason" — but the metabase input only exists
   because `db_password_secret_id` does. Reframed as a *consequence*:
   if the reviewer chooses to remove `db_password_secret_id`, the
   metabase input should go in the same PR for symmetry.

Reference docs reviewed: `docs/client/{architecture,caching,i18n,
accessibility,testing}.md` and `docs/server/{architecture,observability}.md`.
None apply — this is a Terraform-only change with no service code,
RPC handlers, storage, structured logging, client widgets, or test
surfaces touched. No compliance findings from the reference docs.

Live codebase + IAM verification:
- `grep` confirms `ripls-db-password` appears only in
  `terraform/environments/prod/main.tf` (3 occurrences) and 4 docs
  files. No CI/CD workflows, scripts, fastlane lanes, server, or
  client code reference it.
- `gcloud secrets get-iam-policy ripls-db-password --project=ripls-prod`
  shows exactly one IAM binding (the Cloud Run runtime SA, managed
  by the `for_each` over `local.server_runtime_secrets`). Migration
  scope is therefore minimal — the Phase 2 Terraform diff is the
  only change of substance.
- All line refs verified against current `main` (HEAD `b6604a6d2`).

### 2026-05-15 — direction reversed

After the audit landed, the issue thread clarified that the desired
target name is `ripls-db-password` (keep prod, flip dev), not
`gear-db-password` (the original direction). Reasons:

- Aligns with the broader `ripls-*` naming convention used in prod
  for the SQL instance, database, and user.
- Touches dev (lower stakes) instead of prod.
- Sets up the dev-side standardization issue (filed separately) to
  land cleanly: that issue renames the dev SQL instance
  (`gear-db-dev` → `ripls-dev`), database (`gear_library` → `ripls`),
  and user (`gear_user` → `ripls_user`); converging the secret name
  on `ripls-db-password` first means each step of the bigger rename
  is independently sensible.

The recipe and risk profile are unchanged — a same-value cutover is
symmetric — so the only edits were to flip dev/prod throughout the
Problem Statement, codebase audit, Options, and Phases. Open
Questions and Audit log entries above were also touched up to
match.
