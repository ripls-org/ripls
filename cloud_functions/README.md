# ripls Cloud Functions — alert-to-issue automation

Cloud Functions that file issues from production observability signals
(Crashlytics alerts and Cloud Monitoring incidents) in the private tracker on
Forgejo (#3293).

Alerts are private for now: a crash title can carry user data, a monitoring
alert names infrastructure, and some alerts are security signals. In the
private tracker the `claude-fix` label starts nothing; it marks the alerts
that met the bar for an automatic plan. Filing the safe ones publicly, where
the label would start `claude_plan_issue.yaml` again, is tracked as its own
issue in the code repository.

Design: the #1148/#1149 plan (archived).
Operations / runbook context: [`docs/server/observability.md`](../docs/server/observability.md)
(extended in Phase 1 with the auto-filing section).

## Layout

```
cloud_functions/
├── package.json           # Node 24, firebase-functions v2
├── tsconfig.json          # Strict TypeScript, lib/ output
├── firebase.json          # Functions-only Firebase config (not the Flutter app/firebase.json)
├── .firebaserc            # Project aliases: dev → ripls-dev, prod → ripls-prod
└── src/
    ├── index.ts                # Entry point, exports the deployed handlers
    ├── issueTracker.ts         # The five tracker operations the handlers use
    ├── forgejoTracker.ts       # Those operations against Forgejo; token secret, tracker constants
    ├── upsertIssue.ts          # Label-based dedup helper + closeIssue helper
    ├── crashlyticsHandler.ts   # Crashlytics: Velocity / Regression / StabilityDigest triggers
    ├── gcpAlertsHandler.ts     # Cloud Monitoring → Pub/Sub → upsertIssue / closeIssue
    ├── dailySweepHandler.ts    # onSchedule daily close of stale gcp-policy:* issues
    └── __tests__/
        ├── forgejoTracker.test.ts
        ├── upsertIssue.test.ts
        ├── crashlyticsHandler.test.ts
        ├── gcpAlertsHandler.test.ts
        └── dailySweepHandler.test.ts
```

## When to add code here vs elsewhere

- **Here**: external event-driven automation that consumes Firebase / GCP
  signals (Crashlytics alerts, Cloud Monitoring incidents via Pub/Sub) and
  files tracker issues.
- **`server/services/feedback/`**: request-driven issue creation where
  the trigger is a user RPC. Uses `server/github` directly.
- **`server/github/`**: shared Go library for any in-server issue-tracker
  interaction. Do not call this from the Cloud Function — different runtime.

The Cloud Function and `server/github` independently implement `upsertIssue`-shaped
helpers. The duplication is intentional and bounded; see the plan doc's
Audit Notes (the #1148/#1149 plan, archived) for why
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

The functions file issues as a **dedicated Forgejo account**, `ripls-alerts`,
which can write issues in the tracker and nothing else. It is distinct from
`ripls-feedback`, the account the Go server's feedback service uses, so each
credential can be rotated or revoked alone.

| Field        | Value                                                        |
|--------------|--------------------------------------------------------------|
| Instance     | `TRACKER_URL` in `src/forgejoTracker.ts`                     |
| Repository   | `REPO_OWNER` / `REPO_NAME`, same file                        |
| Access token | GCP Secret Manager: `forgejo-alerts-token`                   |

`src/forgejoTracker.ts` declares `defineSecret('forgejo-alerts-token')`, so
Cloud Functions binds the secret at deploy time.
`terraform/environments/prod/main.tf` creates the secret container and grants
the functions' runtime service account (the Compute Engine default SA)
`roles/secretmanager.secretAccessor` on it, and the `cloud-functions-deployer`
SA both `secretAccessor` and `secretmanager.viewer`, which the deploy CLI needs
to inspect the secret when binding it.

Unlike a GitHub App key, the token does not expire and is not exchanged for a
short-lived one, so its scope is what limits it: issues only, on one private
repository.

### Rotating the token

```bash
# 1. As an instance admin, mint a new token for the account, scoped to
#    issues, and push it straight to Secret Manager without displaying it:
forgejo admin user generate-access-token --username ripls-alerts \
  --token-name "alerts-$(date +%Y%m%d)" --scopes write:issue --raw \
  | gcloud secrets versions add forgejo-alerts-token --data-file=- --project=ripls-prod

# 2. Redeploy the functions (they pick up the new version on deploy):
firebase deploy --only functions:ripls-alerts --project prod

# 3. Delete the old token in the account's settings on the instance.
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
