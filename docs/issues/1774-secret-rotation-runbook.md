# Issue #1774: docs: secret rotation runbook

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1774
**Filed:** 2026-05-07 | **Last activity:** 2026-05-07
**Labels:** _none_ | **Assignees:** unassigned
**Context docs:** none provided

## Problem Statement

Today, `docs/secrets.md` catalogs *what* secrets the server consumes (provider, env var name, format) but does not document *how to rotate them*. Section "Rotate tokens regularly" (`docs/secrets.md:207-210`) gives only a cadence ("90 days for GitHub PATs", "annually for Mailgun") with no operational steps. This gap is now blocking work: issue #1767 — rotate every leaked credential found in the audit — needs a recipe per provider, not nine ad-hoc investigations.

The umbrella plan (#1766) calls this out explicitly: Phase 1 lands the runbook so that every Phase 2 per-credential cleanup has a recipe. Without it, the per-track loop in Phase 2 (#1768/#1769/#1770) becomes "figure out rotation under pressure" — exactly the failure mode that produced incident #1284 (closed) and the current cleanup.

A subtle scope question: 13 secrets are currently sourced from GCP Secret Manager and injected as **plaintext env vars** on Cloud Run (per #1770). Once #1770 lands, the rotation procedure for those secrets simplifies dramatically — it becomes a Secret Manager version bump + Cloud Run redeploy. The runbook needs to handle both pre-#1770 and post-#1770 worlds.

## Context from Comments

No comments on the issue. The issue body is the only authoritative content. Expectations are inferable from the umbrella (#1766) and the per-track Phase 2 issues (#1767, #1768, #1769, #1770), which the runbook supports.

Flag: an issue with no discussion is a signal to confirm scope assumptions in this plan rather than barrel ahead.

## Potential Solutions

### Option A: Extend `docs/secrets.md` with a new `## Rotation Procedures` section
- **Approach:** Append a long section to the existing 262-line doc with one subsection per provider. Keep inventory and rotation in the same file so a reader has one stop.
- **Pros:** Single source. Easy to keep cross-links between "what is this secret" and "how to rotate it" — the inventory line for each secret can directly anchor-link to its rotation subsection.
- **Cons:** `docs/secrets.md` would grow from 262 → ~700+ lines. Long files mix concerns: a quick "what env var does X read?" lookup gets buried under operational walls of text. Editing one provider's rotation procedure means a diff in the same file as inventory; review noise increases.
- **Effort:** Medium.

### Option B: New `docs/secret-rotation.md` sibling, cross-linked from `docs/secrets.md`
- **Approach:** Create `docs/secret-rotation.md` as a dedicated runbook. Each section is one provider, written as a step-by-step checklist a single operator can run end-to-end. `docs/secrets.md` gets a top-of-file pointer ("To rotate a secret, see `secret-rotation.md`") and per-secret line-level back-references.
- **Pros:** Clean separation of concerns — inventory vs procedure. Easy to extend over time without bloating `secrets.md`. Pattern matches the issue body's own suggestion ("`docs/secret-rotation.md` (or extend `docs/secrets.md`)").
- **Cons:** Two files to keep in sync; if a new secret is added, both need updates. Mitigation: the runbook is keyed to *providers* not secrets, so one provider often covers multiple keys (e.g. the "GCP" section covers `GCP_SERVICE_ACCOUNT_KEY_DEV` and `_PROD`).
- **Effort:** Medium.

### Option C: New `docs/runbooks/` directory with one file per provider
- **Approach:** Establish a `docs/runbooks/` convention. Files: `runbooks/firebase-service-account.md`, `runbooks/jwt-signing-secret.md`, etc. An `index.md` or `README.md` lists them.
- **Pros:** Maximum modularity. Sets up a pattern for future ops docs (incident response, deploy runbooks, on-call procedures). Each procedure is a self-contained, focused file.
- **Cons:** Heaviest scaffolding for what's currently 13 procedures — probably premature. No other runbooks exist to anchor the convention. Cross-linking from `docs/secrets.md` is more verbose (one link per provider). YAGNI risk: we're building an architecture for runbooks we haven't committed to writing.
- **Effort:** Large (mostly directory hygiene + index doc, not the procedures themselves).

## Recommendation

**Option B** — new `docs/secret-rotation.md`, cross-linked from `docs/secrets.md`.

The clinching factor is the existing `docs/secrets.md` is already tight and well-structured at 262 lines with 6 main sections. Doubling its length with operational procedures would dilute its current job (quick reference for "what env var does X read"). Option C's runbook directory is right *eventually* but premature — there's no second runbook to anchor the convention, and one-file-per-provider creates more files than the team has providers to maintain. Option B threads the needle: separation of concerns, no architectural overreach, and the file boundary makes it easy to migrate to Option C later if more runbooks accumulate.

The runbook itself is keyed to *provider* (a single section per issuing platform), not per-env-var. One provider section often handles multiple env vars: the "GCP" section covers `GCP_SERVICE_ACCOUNT_KEY_DEV` + `_PROD` + the Compute Engine default SA used for ADC; the "Database" section covers `DB_PASSWORD` + `MB_DB_PASS` + the Metabase reader password. This maps cleanly to how rotation actually works (you log into one console and rotate everything for that provider together).

## Open Questions

- **Should rotation be procedure-keyed by provider or by env var?** — Recommendation: **by provider**. Operators log into one vendor console at a time; grouping by provider matches the actual workflow. Where one provider issues multiple keys (GCP, Firebase, the AI providers if they have org-level + project-level keys), call out each within the same section. The umbrella's Phase 2 Track B already implicitly orders by provider (OpenAI → Anthropic → Mailgun → ...).

- **Is the runbook a pre-#1770 or post-#1770 doc?** — Recommendation: **document the post-#1770 happy path as the primary procedure**, with a "if Cloud Run env var is still plaintext (pre-#1770)" callout for each provider. This means the runbook is correct day 1 (covers our current state) and gets *simpler* as #1770 lands (operators ignore the callouts). The alternative — write for current plaintext-env-var world and rewrite later — burns the runbook twice.

- **Do we document the FCM/Firebase rotation as ADC-only or both ADC and JSON-file modes?** — Recommendation: **both, but treat ADC as the production primary**. Per the codebase research, `server/firebase/app.go:19-44` supports both modes; `--fcm-credentials` is a path that, when empty, falls through to ADC. Production almost certainly uses ADC via the Cloud Run runtime SA (`terraform/modules/containers/main.tf:242-245`). #1769 will remove the JSON-file path entirely. **Important nuance for the runbook:** with ADC in Cloud Run, there's no user-rotatable key — the runtime gets short-lived OAuth tokens from the metadata server, scoped to the configured runtime SA's identity. So the ADC-mode "rotation" procedure isn't really rotation; it's compromise-response: either swap the Cloud Run runtime SA (Terraform `google_cloud_run_v2_service.template.service_account`), revoke IAM bindings, or — if a user-managed key is also issued for the runtime SA and that's the suspected leak — delete that specific key in IAM. The runbook's Firebase section should make this distinction explicit: "no proactive rotation under ADC" vs "rotate the JSON file" (deprecated).

- **What account/org IDs do we need to capture, and where?** — Recommendation: **capture them in the runbook under each provider section, but expect placeholders.** The codebase research found that most provider accounts are undocumented (OpenAI, Anthropic, Mapbox, Unsplash, Pexels, Pixabay accounts have no recorded org/project IDs). The runbook needs these to be specific. Plan: write the runbook with `<TODO: account ID>` placeholders for unknowns; the operator (Brian) fills them in during a one-time pass with access to each console.

- **Should `google-client-id` be in the runbook?** — Recommendation: **document it but mark "rotation rare; not actually a secret."** It's an OAuth client *ID* (public by design) currently stored in Secret Manager (`google-client-id`). Rotation is needed only if the OAuth client is reissued (e.g. Google Cloud project change), not for security hygiene. Including it prevents confusion later.

- **Should the runbook cover incident response (e.g. "I just leaked a secret, what do I do *right now*")?** — Recommendation: **yes, as a top-of-document section.** The per-provider procedures are the *how*; an incident-response section gives the *order of operations* under pressure (rotate → verify → audit → notify) and links to each provider section. Keeps the runbook usable for both planned rotation and emergency response.

- **Where do test/CI key values live, and how do they rotate vs prod?** — Recommendation: **document the GitHub Actions secrets layer explicitly.** Per the research: `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `BUF_TOKEN`, `CLAUDE_WORKFLOW_PAT`, `CLAUDE_CODE_OAUTH_TOKEN`, and `GCP_SERVICE_ACCOUNT_KEY_DEV/PROD` are all GitHub-Actions-hosted. Rotation flow for these is "revoke at provider → mint new → update GitHub repo secret → re-run workflows." Different from Cloud Run secrets (which are GCP Secret Manager → version bump → redeploy). The runbook needs both flows.

## Implementation Plan

### Phase 1: Scaffold + highest-priority providers
**Goal:** Land a runbook that's immediately usable for the most blast-radius-y rotations Phase 2 will hit first (Firebase, JWT, GCP SA keys), without trying to cover everything in one PR.
**Tasks:**
- [ ] Create `docs/secret-rotation.md` with top-level structure: Title, "When to use this", "Incident response order of operations", "Per-provider procedures" (TOC), "Verification helpers".
- [ ] Add cross-link at top of `docs/secrets.md` pointing to the runbook.
- [ ] Write per-provider section for **Firebase service account** — covers ADC (primary, post-#1769; "no proactive rotation, compromise response is to swap or revoke the runtime SA") and explicit JSON-file path (deprecated; "rotate the key, update the JSON file, redeploy"). Include verification: send a test FCM push from a dev device.
- [ ] Write per-provider section for **JWT signing secret** — generation step (`openssl rand -base64 64`), Secret Manager version bump, redeploy, and explicit "all sessions invalidated" warning with copy for user comms.
- [ ] Write per-provider section for **GCP service account keys** (`GCP_SERVICE_ACCOUNT_KEY_DEV/PROD`) — IAM console steps, key creation, GitHub repo secret update, workflow re-run, old-key deletion timing.
- [ ] Add "Verification helpers" section with reusable snippets: `gcloud secrets versions list`, `gh secret list`, `gcloud run services describe`.
**Validates:** that the structure works for the three most operationally complex rotations. If we can rotate the Firebase SA and JWT secret using only this doc, the structure is right.

### Phase 2: Remaining providers
**Goal:** Cover every secret in Terraform / GitHub Actions so the runbook is complete.
**Tasks:**
- [ ] **Database passwords** (`DB_PASSWORD`, `MB_DB_PASS`, metabase reader) — Cloud SQL user password change coordination, Secret Manager version bump, Cloud Run redeploy, downtime expectation.
- [ ] **AI providers** — OpenAI, Anthropic. Console URL, account/org placeholders, key revocation, Secret Manager + GitHub Actions secret update (both layers used).
- [ ] **Email** — Mailgun. Domain (`ripls.app`), key revocation, sending-domain verification post-rotation.
- [ ] **Geocoding** — Mapbox. Token rotation, URL restriction reverification.
- [ ] **Stock images** — Unsplash, Pexels, Pixabay. Lower priority, mostly identical procedures.
- [ ] **GitHub App** (`GITHUB_APP_PRIVATE_KEY`) — App ID 2840389 / Installation ID 109363272. Generate new private key in App settings, base64-encode, Secret Manager update, redeploy. Verify via a test PR-event hook.
- [ ] **Metrics auth token** — generate new value, Secret Manager update, redeploy, document scrape-side update needed for any external Prometheus consumer.
- [ ] **Google Client ID** — note as "not actually a secret; rotation rare."
- [ ] **Other GitHub Actions secrets** — `BUF_TOKEN`, `CLAUDE_WORKFLOW_PAT`, `CLAUDE_CODE_OAUTH_TOKEN`. Brief sections.
- [ ] Fill in `<TODO: account ID>` placeholders for every provider where Brian has the access (one-time pass).
**Validates:** every secret in `docs/secrets.md` and every `${{ secrets.X }}` reference in `.github/workflows/*.yaml` has a corresponding rotation section.

### Phase 3: Pre-#1770 → post-#1770 simplification pass
**Goal:** Once #1770 lands (Cloud Run secrets switched to `secret_key_ref`), simplify the runbook to remove the plaintext-env-var caveat callouts.
**Tasks:**
- [ ] Remove the "if Cloud Run env var is still plaintext" sub-procedures from each section.
- [ ] Reword "redeploy" steps to "version bump + traffic-revision rotation" where possible.
- [ ] Add a top-of-section note: "Post-#1770: rotation = `gcloud secrets versions add ...` + redeploy."
**Validates:** runbook stays in sync with the architecture; no stale instructions referencing dead Terraform variables.

### Phase 4: Hardening pass
**Goal:** Make the runbook resilient against drift and useful under stress.
**Tasks:**
- [ ] Add a "Last rotated" table at the bottom of `docs/secret-rotation.md`. Each row: secret name, last rotation date, next planned date. Update each row in the same PR that rotates the secret.
- [ ] Cross-link each provider section from `docs/secrets.md` (per-secret line-level anchors) so a reader looking up "what is `OPENAI_API_KEY`" sees a "→ rotate this" link.
- [ ] Add a compact incident-response card (single-screen) at the top: "I leaked a secret. Order of operations:" with five bullets and links into the per-provider sections.
- [ ] Document the relationship to gitleaks (#1772): "If gitleaks fires on a leaked value, treat it as a real incident — start at the incident-response section."
**Validates:** runbook is usable under pressure (incident-response section), drift is detectable (last-rotated table), and the cross-link mesh between `secrets.md` ↔ `secret-rotation.md` ↔ `#1772` is complete.

## Audit Notes

`/ripls-audit` pass on 2026-05-07.

**Reference docs reviewed for overlap:**
- `docs/server/architecture.md` — no overlap. Architecture is service/storage/testing patterns. No section on secret rotation or operational runbooks.
- `docs/server/observability.md` — no overlap. Scoped to logging, health checks, alerting. Confirmed by grep for `secret|rotat|runbook|incident` returning zero matches in operational sections.
- Client docs (`docs/client/architecture.md`, `caching.md`, `i18n.md`, `accessibility.md`, `testing.md`) — not applicable. This plan introduces no client-side changes.

**Factual claims verified against the codebase:**
- `server/firebase/app.go:19-44` — Firebase init supports both explicit JSON file and ADC fallback. ✓
- GitHub App ID `2840389`, Installation ID `109363272` in `terraform/environments/dev/main.tf:248-249` and `prod/main.tf:265-266`, with explanatory comment at `prod/main.tf:80`. ✓
- Mailgun domain `ripls.app` documented at `docs/secrets.md:89-91`. ✓
- 13 secrets currently flow Secret Manager → Terraform variable → plaintext Cloud Run env var (per #1770). ✓

**Changes made during audit:**
- Refined the FCM/ADC Open Question to correct a technical imprecision: under ADC in Cloud Run there is no user-rotatable key (metadata-server-issued short-lived tokens), so "rotation" in ADC mode is really "compromise response" — swap the runtime SA, revoke IAM bindings, or delete a specific user-managed key if one was leaked. The original framing implied a routine GCP-managed key rotation that doesn't apply.
- Updated the Phase 1 Firebase task to surface the same distinction: ADC mode says "no proactive rotation"; JSON-file mode (deprecated) says "rotate the key, update the file, redeploy."

**No architectural conflicts found.** The plan respects the existing `docs/secrets.md` structure (extends rather than duplicates), correctly scopes to operational documentation, and does not touch any code path constrained by CLAUDE.md or the architectural reference docs.

## Related Issues
- #1766 — umbrella; this issue is Phase 1 of the umbrella plan.
- #1767 — credential rotation checklist; the runbook is the recipe operators follow when checking off items there.
- #1768 — package.json key removal; rotation steps for each affected provider are gated on this issue's Phase 2.
- #1769 — Firebase service account → ADC migration; reframes the Firebase section's primary procedure (post-#1769, ADC is the only mode).
- #1770 — Cloud Run env vars → Secret Manager refs; gates this plan's Phase 3 simplification pass.
- #1771 — git history rewrite; the runbook does not cover this — it's a one-time hygiene task, not an ongoing rotation procedure.
- #1772 — DIY gitleaks stack; the runbook's incident-response section is the operator's response when gitleaks fires.
- #1773 — GitHub native baseline + Secret Protection upgrade tracker; orthogonal to this runbook but worth a "see also" cross-link.
- #1284 (closed) — prior JWT hardcoded-secret incident; the JWT section in the runbook should reference this as historical context.
