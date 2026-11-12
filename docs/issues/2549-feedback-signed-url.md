# Issue #2549: Feedback screenshot URLs break within ~2 weeks (GCS signBlob key rotation), not at the 1-year expiry

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2549
**Filed:** 2026-06-25 | **Last activity:** 2026-06-25
**Labels:** none | **Assignees:** briangbrown
**Context docs:** `docs/server/architecture.md` (anchor), `docs/secrets.md`, `docs/secret-rotation.md`, `docs/user_feedback.md`, `docs/reviews/security_review_prompt.md` (engaged: secrets management, PII, crypto/signing)

## Problem Statement

Feedback screenshots embedded in `user-feedback` GitHub issues stop loading (**HTTP 403 `SignatureDoesNotMatch`**) ~1–2 weeks after the issue is filed — far short of the **1-year** `Expires` the URL advertises. The screenshot renders right after submission, then silently breaks.

Root cause: the feedback service mints a **GCS V2 signed URL** signed via **IAM `signBlob` on the default compute service account** (`server/storage/bucket.go:197-208` supplies no `GoogleAccessID`/`PrivateKey`/`SignBytes`, so the client falls back to `signBlob`). `signBlob` signs with the service account's **Google-managed** key, which Google rotates roughly every 1–2 weeks. GCS verifies the signature against the SA's *currently valid* public keys, so once that key rotates out the signature is unverifiable — 403, regardless of `Expires`. The 1-year expiry at `server/services/feedback/service.go:95` is effectively a lie; the real lifetime is "until the next managed-key rotation."

This is **not** the app's fault: `FeedbackService` has a single RPC (`SubmitFeedback`); screenshots arrive as `bytes` in the request (`proto/ripls/api/feedback_service.proto:19`), are stored once (`service.go:85`), and the only consumer of the signed URL is the GitHub issue body (`service.go:101` → `formatIssueBody` `service.go:200-206`). There is no read-back RPC; the app's compose preview uses local bytes.

Two deliverables: **(1)** make feedback screenshot URLs durable for their full stated lifetime; **(2)** repair the already-broken URLs in existing issues.

## Context from Comments

No GitHub comments — the issue was filed and immediately planned in the same session. The direction is set by the maintainer (briangbrown) in that session:

- **Go with Option 2** (keep GCS, sign with a non-rotating user-managed key), explicitly **because a correctly-signed GCS URL is directly fetchable by anything holding the link** (no GitHub session), so automated triage / Claude can read the screenshot when processing an issue. A GitHub `user-attachments` URL (Option 1) requires a GitHub session and would not be fetchable that way.
- **Repair the existing broken URLs** as part of this work.

## Codebase Findings

(Confirmed by code search.)

- **Where the URL is minted:** `server/services/feedback/service.go:73-104` stores each screenshot (`s.bucket.Put`, line 85) then `s.bucket.GetSignedURL(ctx, key, 365*24*time.Hour)` (line 95). Key is `MediaBucketKey("feedback-system", uuid)` = `feedback-system/<uuid>` (`server/storage/bucket.go:24`).
- **Where signing happens:** `GCSBucketStorage.GetSignedURL` (`server/storage/bucket.go:196-209`) sets only `Method` + `Expires` on `storage.SignedURLOptions`; no explicit signer → `signBlob`. The struct (`bucket.go:67-70`) holds only `client` + `bucketName`; constructed via `NewGCSBucketStorage(ctx, bucketName)` (`bucket.go:74-83`) on ADC.
- **Feedback shares the media bucket:** `feedback.New(githubClient, owner, repo, bucket)` (`server/main.go:642/656/665`) is passed the same `bucket` returned by `initializeBucketStorage` (`main.go:141-170`, `--gcs-bucket` flag). Media presigns through the same `GetSignedURL` but with a 35-min expiry (`service.go` media path), which is *inside* the rotation window, so media is unaffected by this bug.
- **Secret injection pattern (the template to mirror):** secrets are fetched at container start by `server/entrypoint.sh` (`fetch_secret_to_file` / `fetch_secret_to_file_optional` → `$SECRETS_DIR/<name>`, 0400, trailing-whitespace stripped), then consumed via a `-<name>-file` flag resolved through `secretsflag.Resolve(name, value, path)` (`server/main.go:308-323`, `server/secretsflag/resolve.go:26`). **The GitHub App PEM private key is the direct precedent** — `github-app-private-key-base64` secret → `$SECRETS_DIR/github-app-private-key` → `-github-app-private-key-file` flag → `decodeBase64PEM` (`main.go:272-273, 323, 340-341`). The server binary never speaks to Secret Manager (`entrypoint.sh` does). Terraform grants `secretAccessor` per-secret via `local.server_runtime_secrets` (`terraform/environments/$ENV/main.tf`).
- **No Go Secret Manager SDK usage** — confirms the entrypoint-fetch-to-file pattern is the only sanctioned path.

## Why durable signing *requires* a user-managed key

Any `signBlob`-based signing — default compute SA *or* a dedicated SA — uses a Google-managed key and inherits the same rotation death. The only way to get a **signed URL** valid for the full `Expires` is to sign with a **user-managed key** (one we create and control, which does not auto-rotate) and pass it explicitly as `GoogleAccessID` + `PrivateKey`. That is the irreducible cost of the signed-URL options (A/B). (V4 signing would also work cryptographically but caps `Expires` at **7 days** — too short for an issue attachment — so a signed-URL approach stays on **V2**, which has no cap.)

The key word is *signed URL*. Option D avoids the whole problem by **not** handing the viewer a GCS-signed URL — the server itself is the capability boundary, reads via ADC, and never needs a non-rotating signing key. That distinction is what makes D compatible with the #1769 stance below.

## Potential Solutions

### Option A: Dedicated user-managed signing key, feedback-only durable path — recommended
- **Approach:** Provision a dedicated signing SA with a user-managed JSON key (stored in Secret Manager). Add a durable-signing path that uses `GoogleAccessID` + `PrivateKey` (V2) and have **only the feedback service** call it. Media keeps `signBlob` (its 35-min URLs are fine).
- **Pros:** Smallest blast radius for the long-lived key (used only on the feedback path); media's proven path untouched; fetchable-by-anyone URL satisfies the "Claude can read it" requirement; mirrors the existing GitHub-App-key secret pattern end to end.
- **Cons:** Introduces a long-lived SA key to secure + rotate; the signer SA needs `objectViewer` on the bucket (prefix-scoping in GCS IAM is limited); rotating/deleting the key invalidates outstanding URLs (forces a re-backfill).
- **Effort:** medium (Go + terraform + entrypoint + one-time backfill).

### Option B: Same key, but used for *all* GCS signing (media + feedback)
- **Approach:** Configure the explicit signer on `GCSBucketStorage`; when present, `GetSignedURL` uses it for everything; fall back to `signBlob` when absent.
- **Pros:** One code path; media also becomes rotation-proof (marginal benefit at 35 min).
- **Cons:** The long-lived key now signs every media URL → wider usage/exposure of the key, and the signer SA needs `objectViewer` across the entire media bucket. No real upside over A for the bug at hand.
- **Effort:** medium (similar to A, slightly less branching).

### Option C: GitHub `user-attachments` upload (drop GCS for feedback)
- **Approach:** Upload screenshot bytes to GitHub's `user-attachments` CDN and embed that URL.
- **Pros:** Permanent, renders inline in private-repo issues, no GCS/secret dependency.
- **Cons:** **No official API** — relies on the undocumented `upload/policies/assets` browser flow (fragile). And the URL needs a GitHub session to fetch → **breaks the "Claude can read the screenshot" requirement**. Rejected by the maintainer for that reason.
- **Effort:** large + ongoing fragility.

### Option D: Server capability-token image proxy (no new key) — added during audit
- **Approach:** Embed a stable URL pointing at a new **public, unauthenticated** server route (e.g. `GET /feedback-image/{token}`), where `token` is an HMAC (using the existing `jwt-signing-secret`) over the `feedback-system/<uuid>` object key. The handler verifies the HMAC (constant-time), validates the key shape, reads the object via **ADC** (the runtime SA already has bucket read — it writes these objects), and streams the bytes. GitHub's camo proxy fetches and caches it; the HMAC token is the bearer capability, so no GitHub session is needed.
- **Pros:** **No user-managed SA key** → sidesteps the #1769 stance entirely (see Recommendation). **No new IAM** (runtime SA already reads the bucket). **No re-backfill on key rotation** — the token is stable forever. Directly fetchable by Claude (satisfies the maintainer's driver). Mirrors the existing public-unauth content route pattern (`/go/{code}` web preview, `server/main.go:1371`).
- **Cons:** New public endpoint to secure (capability token, key-shape validation, prefix pinning to `feedback-system/`, constant-time compare); the server is now in the image-load path (bandwidth + availability). For feedback volume (≤3 images/issue, ~100 KB each, low rate) both are negligible. SSRF is not a concern — the only attacker-influenced input is the object key, which is validated and pinned to a fixed bucket/prefix.
- **Effort:** medium (one handler + token mint/verify + backfill), comparable to A but with no terraform key/SA provisioning.

## Decision: Option A (durable signed URL, dedicated signer key in Secret Manager)

**Chosen 2026-06-25 by the maintainer (briangbrown) after working the full trade-space below.** The feedback service keeps minting a GCS signed URL, but signs it with a **dedicated, minimally-privileged service-account key held only in Secret Manager** (never in the repo), using the explicit `GoogleAccessID` + `PrivateKey` path so the signature is created locally and does **not** depend on the rotating `signBlob` managed key. Expiry becomes meaningful again.

### Why Option A

- **Meets the load-bearing requirement** — the maintainer's driver is that *whoever can read the issue (incl. Claude running as that user) can read the image, with the server out of the byte path*. A directly-fetchable signed URL satisfies "Claude reads it" without a GitHub session, and signing-key durability is a small, **fully supported** change.
- **The auth-coupling ideal is physically unreachable for an external host.** GitHub renders external images through its **camo proxy, fetched unauthenticated** — the viewer's identity is never in the fetch. So *any* externally-hosted image (GCS, our own server) is a public bearer capability; access cannot be coupled to GitHub issue access. True coupling requires GitHub-native hosting, which we rejected (see Option C). We therefore accept a bearer URL, mitigated by the **private repo + unguessable UUID + bounded expiry**.
- **#1769 is satisfied, not violated.** The policy bans SA JSON keys **in the repo** and prefers ADC **where it works**. Here ADC = `signBlob` = the rotation bug itself, so ADC genuinely cannot do the job. A long-lived private key held **only in Secret Manager and fetched at runtime** is the already-sanctioned pattern — `github-app-private-key-base64` is the exact precedent. The key sits on a **dedicated** SA (not the privileged default compute SA), so a leak's blast radius is "read feedback screenshots," nothing more.

### Alternatives considered (and why rejected)

- **Option B — same key for *all* GCS signing (media + feedback):** dominated by A. Media's 35-min URLs already outlive the rotation window, so there's no benefit, and it would put the long-lived key on the highest-throughput signing path and require `objectViewer` across the whole media bucket. Rejected.
- **Option C — GitHub `user-attachments` hosting:** the *only* approach that truly couples image access to issue access (URLs inherit repo visibility) and is free (GitHub-hosted, no storage quota). **Rejected as too fragile:** there is no token-based API — uploads require replaying the browser's undocumented `upload/policies/assets` flow with a **web-session cookie** (PAT/App tokens return 401), so the GitHub-App-authenticated server can't do it. Automating it needs a bot account whose session cookie must be kept alive (2FA/device-verification re-login), against an endpoint GitHub doesn't document and actively polices for abuse. Unacceptable dependency for a core pipeline. (Research + sources captured in the issue thread.)
- **Option D — server capability-token image proxy:** technically clean and #1769-free, but **puts the server in the image path** (availability + bandwidth dependency for every render), which the maintainer explicitly ruled out. Rejected on that requirement.
- **Periodic "URL fixer" that re-signs `signBlob` URLs before each rotation:** needs no key, but produces public bearer URLs (no better than A on coupling) **and** is a perpetual treadmill — a cron rewriting every feedback issue every few days forever, where one missed run silently breaks every image. Rejected; the one-time backfill in Phase 2 is the bounded version of this.

### No reusable key exists

Confirmed by listing Secret Manager in `ripls-prod` and `ripls-dev` (2026-06-25): there is **no GCP SA signing key** to reuse. `github-app-private-key-base64` / `-alerts-` are GitHub App RSA keys (not GCS identities); `jwt-signing-secret` is symmetric; the rest are vendor API keys / passwords. The only GCP SA JSON key the project ever had (Firebase) was removed and revoked under #1769. So Phase 1 must mint a new dedicated signer key.

## Open Questions

- **Q0: Approach? — RESOLVED → Option A** (see Decision above).
- **Q1: Dedicated feedback bucket vs. shared media bucket?** — Recommendation: **shared** bucket for now (avoids migrating existing `feedback-system/` objects). Grant the signer SA `objectViewer` scoped to the `feedback-system/` prefix via an IAM Condition (`resource.name.startsWith(".../objects/feedback-system/")`) so the key can only authorize feedback objects, not all media. Revisit a dedicated bucket only if security review wants a harder boundary.
- **Q2: Signing scope — feedback-only vs. all signing?** — RESOLVED → **feedback-only**. Add a dedicated durable-sign method; leave media's `GetSignedURL` on `signBlob` (its 35-min URLs are unaffected by rotation). Don't put the long-lived key on media's high-throughput path.
- **Q3: Expiry length? — RESOLVED → 365 days.** A named constant `feedbackURLExpiry` (V2 has no cap). One year keeps the screenshot available for the realistic lifetime of an open issue; the Phase 2 backfill can re-sign if a URL is ever needed past it. Kept as a constant (not a flag) — change via code edit + redeploy if it ever needs tuning.
- **Q4: Key rotation semantics?** — Recommendation: document in `docs/secret-rotation.md` that **rotating/deleting the signer key invalidates every outstanding feedback URL** → forces a re-backfill. Rotate rarely and only with a planned re-backfill run.
- **Q5: Backfill ordering & object survival?** — Recommendation: run the backfill **only after Phase 1 is deployed**. **First** confirm there is no GCS lifecycle/TTL rule on the `feedback-system/` prefix that has already reaped objects — unrecoverable ones must be **logged**, not silently skipped.
- **Q6: Where does the backfill run?** — Recommendation: a one-off Go command under `server/cmd/` (e.g. `backfill-feedback-urls`), idempotent (rewrite only `![Screenshot N](…)` lines), `--dry-run` default, rate-limit-aware. Needs the signer key (sourced like `scripts/run_server_with_sm_secrets.sh`) + GitHub-App creds.

## Implementation Plan

### Phase 1: Durable feedback signing — Option A
**Goal:** New feedback screenshot URLs stay valid for their full stated expiry (no longer `signBlob`-rotation-bound).

**Recommended implementation order** (storage-layer first so each step is testable before the one above it depends on it; provisioning written alongside but applied last):

1. **`storage/bucket.go` — durable signer + method.** Add optional `signerEmail` / `signerPrivateKey` ([]byte PEM) to `GCSBucketStorage`; add `GetDurableSignedURL(ctx, key, dur)` that sets `Method`, `Expires`, `GoogleAccessID`, `PrivateKey` (default scheme = V2; V4 caps at 7d). Return a clear error if no signer is configured. Leave `GetSignedURL` (media, `signBlob`) untouched. **Unit test** with a freshly generated RSA key: asserts a V2 URL with `GoogleAccessID` + expected `Expires`, and the no-signer error. Fully testable in isolation — start here.
2. **`main.go` — flag + key parse + wiring.** Add `-feedback-signer-key` / `-feedback-signer-key-file`, resolve via `secretsflag.Resolve` (mirror `github-app-private-key` at `main.go:272-273,323`). Parse the SA JSON with `encoding/json` (no new dep) → `client_email`, `private_key`. Thread `signerEmail`/`signerPrivateKey` into the GCS bucket constructor (extend `NewGCSBucketStorage` / `initializeBucketStorage`). Absent key ⇒ leave signer unset (graceful degrade).
3. **`entrypoint.sh` — fetch secret.** `fetch_secret_to_file_optional feedback-url-signer-key "$SECRETS_DIR/feedback-url-signer-key"` and append `-feedback-signer-key-file=$SECRETS_DIR/feedback-url-signer-key` when present (optional, like `google-maps-api-key-server`).
4. **`feedback/service.go` — use the durable path.** Replace `GetSignedURL(... 365d)` at `service.go:95` with `GetDurableSignedURL`, expiry from the named constant `feedbackURLExpiry` (Q3, 365d). If no signer is configured (dev/local), `Warn` and fall back to the existing path — explicit, not silent.
5. **Terraform (write now, apply to deploy).** Dedicated SA `feedback-url-signer@$PROJECT`; `roles/storage.objectViewer` scoped to the `feedback-system/` prefix via IAM Condition (Q1); user-managed key; store JSON in Secret Manager as `feedback-url-signer-key`; add it to `local.server_runtime_secrets` so the runtime SA gets `secretAccessor`. **Key material lives only in SM / TF state — never in the repo** (#1769).
6. `npm run build` + `go test ./...` green.

**Validates:** a freshly minted feedback URL is signed with the user-managed key and survives a managed-key rotation window.

### Phase 2: Backfill existing broken issues
**Goal:** Repair screenshot URLs already embedded in `user-feedback` issues.
**Tasks:**
- [ ] Confirm Phase 1 is **deployed to prod** before running (Q5).
- [ ] Verify no lifecycle/TTL on the `feedback-system/` prefix has reaped objects (`gcloud storage` describe a sample + check bucket lifecycle config).
- [ ] Write `server/cmd/backfill-feedback-urls` (Q6): enumerate `label:user-feedback` issues, regex-extract `feedback-system/<uuid>` keys from each body, confirm each object still exists (log + skip missing), **mint a new durable URL** (Option D: `/feedback-image/<token>`; Option A: re-sign with the durable signer), and rewrite only the `![Screenshot N](…)` URLs via the GitHub API. Idempotent; `--dry-run` default; rate-limit-aware (today's GraphQL limit is a real constraint).
- [ ] Run dry-run, eyeball the diff, then apply. Record counts (repaired / skipped-missing / errors).
**Validates:** previously-broken screenshots load again; unrecoverable ones are enumerated.

### Phase 3: Hardening & docs (small)
**Goal:** Prevent regression and operationalize the new key.
**Tasks:**
- [ ] Document the durable-URL requirement so a future change can't silently revert to a `signBlob`-signed long-lived URL: note in `server/services/feedback/` (README/doc.go) and `docs/user_feedback.md`.
- [ ] **Option D:** document the `/feedback-image/{token}` capability route + the `jwt-signing-secret`-as-kill-switch semantics (Q3). **Option A:** add `feedback-url-signer-key` to `docs/secrets.md` and the "rotation forces re-backfill" note to `docs/secret-rotation.md`.
- [ ] (Optional) revisit Q3 (expiry/revocation) if security review wants a tighter exposure window.
**Validates:** the design is documented and the chosen approach's operational semantics are captured.

## Related Issues
- #2275 — the feedback issue whose broken screenshot surfaced this bug (diagnostic origin).
- #1529 — recently closed; same `GetSignedURL`/presigned-URL machinery (`GetMedia`), but that path uses short-lived URLs and is unaffected by rotation. Good cross-reference for the media-vs-feedback expiry distinction.
- **#1769** — removed a checked-in user-managed SA JSON key and established the ADC-only/no-JSON-key posture. **Directly governs the A-vs-D choice** (Q0): Option A must be reconciled against it; Option D avoids it.

---

## Audit notes (added by `/ripls-audit` on 2026-06-25)

Audited against the documentation context map. **Scope:** server + infra; subsystems feedback / GCS signing / secrets. **Engaged dimensions:** security (secrets, PII, signing), error_handling, observability, testing, code_quality.

**Material change — new policy conflict surfaced:**
- The original plan recommended Option A/B, which introduce a **user-managed SA JSON key**. This collides with the documented **#1769** decision (`docs/secret-rotation.md:57-93`, `docs/secrets.md:128`): prod is ADC-only specifically so there is "no user-managed key… to leak," and a checked-in SA key was treated as a leak incident. Added the conflict analysis to **Recommendation**, a new **Option D** (server capability-token proxy) that needs no new key / no new IAM / no re-backfill-on-rotation, and a new **Q0** making A-vs-D an explicit maintainer decision. Re-pointed the Implementation Plan at D with Option-A deltas. Noted the nuance that long-lived keys *in Secret Manager* (`github-app-private-key-base64`) are sanctioned, so A is *defensible* if framed that way — but D is cleaner against the docs.

**Smaller alignments:**
- Q3 reframed around the chosen approach (token kill-switch vs. signed-URL expiry) rather than assuming a signing key.
- Added explicit security requirements to Option D (constant-time HMAC compare, `feedback-system/` key-shape pinning to prevent traversal/arbitrary-object reads, generic error bodies, no key in logs) per the security review "Areas to Review."
- Added observability/error-handling tasks (standard field names, no PII, no `CodeInternal`-style leakage) per `docs/server/observability.md`.
- Flagged that the SA-JSON parse (Option A) uses `encoding/json` — no new dependency.

**Docs consulted:** `docs/llms.txt`, `docs/server/architecture.md` (anchor), `docs/secrets.md`, `docs/secret-rotation.md`, `docs/user_feedback.md`, `docs/reviews/security_review_prompt.md` (Areas to Review). Not consulted: client/accessibility/api_design docs — this is a server+infra change with no client surface or proto change.
