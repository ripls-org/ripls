# ripls Cloud Functions — alert-to-issue automation

Cloud Functions that file GitHub issues from production observability signals
(Crashlytics alerts and Cloud Monitoring incidents). Funnels into the existing
`claude_plan_issue.yaml` workflow via the `claude-fix` label rather than
duplicating any Claude planning workflow.

Design: [`docs/issues/1148-1149-alert-to-issue.md`](../docs/issues/1148-1149-alert-to-issue.md).
Operations / runbook context: [`docs/server/observability.md`](../docs/server/observability.md)
(extended in Phase 1 with the auto-filing section).

## Layout

```
cloud_functions/
├── package.json           # Node 24, firebase-functions v2, @octokit/rest, @octokit/auth-app
├── tsconfig.json          # Strict TypeScript, lib/ output
├── firebase.json          # Functions-only Firebase config (not the Flutter app/firebase.json)
├── .firebaserc            # Project aliases: dev → ripls-dev, prod → ripls-prod
└── src/
    ├── index.ts                # Entry point, exports the deployed handlers
    ├── githubAuth.ts           # Shared GitHub-App auth, Octokit factory, repo constants
    ├── upsertIssue.ts          # Label-based dedup helper + closeIssue helper
    ├── crashlyticsHandler.ts   # Crashlytics: Velocity / Regression / StabilityDigest triggers
    ├── gcpAlertsHandler.ts     # Cloud Monitoring → Pub/Sub → upsertIssue / closeIssue
    ├── dailySweepHandler.ts    # onSchedule daily close of stale gcp-policy:* issues
    └── __tests__/
        ├── upsertIssue.test.ts
        ├── crashlyticsHandler.test.ts
        ├── gcpAlertsHandler.test.ts
        └── dailySweepHandler.test.ts
```

## When to add code here vs elsewhere

- **Here**: external event-driven automation that consumes Firebase / GCP
  signals (Crashlytics alerts, Cloud Monitoring incidents via Pub/Sub) and
  files GitHub issues.
- **`server/services/feedback/`**: request-driven GitHub-issue creation where
  the trigger is a user RPC. Uses `server/github` directly.
- **`server/github/`**: shared Go library for any in-server GitHub
  interaction. Do not call this from the Cloud Function — different runtime.

The Cloud Function and `server/github` independently implement `upsertIssue`-shaped
helpers. The duplication is intentional and bounded; see the plan doc's
[Audit Notes](../docs/issues/1148-1149-alert-to-issue.md#audit-notes) for why
(Bryan's design directive on #1322 + #1323).

## Local development

Requires Node 24 and the Firebase CLI.

```bash
cd cloud_functions
npm install
npm run build      # tsc → lib/
npm test           # vitest
npm run lint       # eslint
```

## Deploy

Deployment is automated via `.github/workflows/deploy_cloud_functions.yaml`
on push to `main` for `cloud_functions/**`. Manual deploy from a local checkout:

```bash
firebase use prod        # or `dev`
firebase deploy --only functions:ripls-alerts
```

The Functions codebase is named `ripls-alerts` so it can coexist with any
future Functions codebases without sharing scaling rules.

## Secrets

The functions authenticate as a **dedicated GitHub App**, `ripls-alerts-bot`,
distinct from the `ripls-feedback-bot` App the Go server's feedback service
uses. The split keeps user-supplied feedback content away from the identity
`claude_plan_issue.yaml` allowlists; the reasoning is in the header comment of
`src/githubAuth.ts` (and PR #1762). Coordinates:

| Field            | Value                                                          |
|------------------|----------------------------------------------------------------|
| App ID           | `3637024` (`GITHUB_APP_ID` in `src/githubAuth.ts`)             |
| Installation ID  | `130393653` (`GITHUB_INSTALLATION_ID` in `src/githubAuth.ts`)  |
| Private key      | GCP Secret Manager: `github-app-alerts-private-key-base64`     |

`src/githubAuth.ts` declares
`defineSecret('github-app-alerts-private-key-base64')`, so Cloud Functions binds
the secret at deploy time. `terraform/environments/prod/main.tf` grants the
functions' runtime service account (the Compute Engine default SA)
`roles/secretmanager.secretAccessor` on it, and the `cloud-functions-deployer`
SA both `secretAccessor` and `secretmanager.viewer`, which the deploy CLI needs
to inspect the secret when binding it.

The secret value is a base64-encoded PEM, the same encoding the Go server uses
for its own App key. `decodeBase64Pem` in `src/githubAuth.ts` mirrors
`decodeBase64PEM` in `server/config/config.go`.

### Rotating the App private key

Only the Cloud Functions read this key; the server's feedback App has its own
key (`github-app-private-key-base64`) and its own rotation.

```bash
# 1. In GitHub, generate a new private key for the ripls-alerts-bot App
#    (Settings → Developer settings → GitHub Apps → Generate a private key).
#    This downloads a .pem file.
# 2. Base64-encode it on one line:
base64 -i downloaded-private-key.pem -o /tmp/key.b64

# 3. Push a new version to Secret Manager:
gcloud secrets versions add github-app-alerts-private-key-base64 \
  --data-file=/tmp/key.b64 --project=ripls-prod
rm /tmp/key.b64

# 4. Redeploy the functions (they pick up the new version on deploy):
firebase deploy --only functions:ripls-alerts --project prod

# 5. Revoke the old key in the GitHub App settings.
```

## Disabling in an emergency

If the auto-filer is creating noise in the issue tracker, disable the
deployed functions without redeploying:

```bash
firebase functions:delete crashlyticsVelocityAlert --region=us-central1
firebase functions:delete crashlyticsRegressionAlert --region=us-central1
```

The email notification channels in `terraform/modules/monitoring/main.tf` are
attached to alert policies in parallel, so disabling Cloud Functions does not
silence prod alerts — humans still see the email.
