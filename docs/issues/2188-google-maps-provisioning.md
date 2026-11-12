# Google Maps API Key Provisioning — Issue #2188

CLI-driven setup for the Google Maps Platform API keys + GSM secrets
that the migration depends on. The architecture is **four keys per
environment**, each restricted to one platform (server, web, Android,
iOS). All eight keys are CLI-managed via
`scripts/provision_google_maps_2188.sh`; this doc is the walkthrough
that explains what each step does, how to apply application
restrictions when their values become known, and how local dev works.

## Current state (as of 2026-05-27)

What's been applied:

| | Dev (`ripls-dev`) | Prod (`ripls-prod`) |
|---|---|---|
| 4 keys + 4 GSM secrets | ✅ provisioned | ✅ provisioned |
| Terraform IAM bindings | ✅ applied | ✅ applied |
| Server key restrictions | none (intentional — local dev) | ⏳ Cloud NAT + IP restriction deferred to Phase 3 |
| Web key restrictions | none yet | ✅ `https://ripls.app/*` referrer |
| Android key restrictions | none yet | ✅ `org.ripls.app` + 2 SHA-1s (both fingerprints registered in Firebase for the prod app) |
| iOS key restrictions | none yet | ✅ bundle ID `org.ripls.app` |
| Budget alert ($/month) | ✅ $50, emails at 50/90/100% | ✅ $500, emails at 50/90/100% |
| Per-API quota caps | none | none (planned for Phase 3 cutover) |

## Prerequisites

- `gcloud` CLI installed and authenticated against the relevant
  project (`gcloud auth login` once per machine).
- Owner or Editor role on `ripls-dev` (and later `ripls-prod`) to
  enable services, create API keys, and write to Secret Manager.
- Optional: `billing.budgets.create` on the billing account if you
  want to manage the budget alert via CLI (otherwise: console).

## Key inventory

Each environment gets four keys. Naming is consistent across envs;
distinction is the GCP project (`ripls-dev` vs `ripls-prod`).

| GSM secret | Purpose | API restrictions | Application restrictions |
|---|---|---|---|
| `google-maps-api-key-server` | Server-side Places + Geocoding calls from `GoogleMapsClient` (Phase 3+) | Places API (New), Geocoding API | Dev: none. Prod: none today; Cloud Run egress static IP planned (Phase 3 follow-up). |
| `google-maps-api-key-web` | Maps JS SDK + Places + Geocoding from Flutter Web | Maps JavaScript API, Places, Geocoding | Dev: none. Prod: HTTP referrer `https://ripls.app/*`. |
| `google-maps-api-key-android` | Maps SDK Android + Places + Geocoding | Maps SDK Android, Places, Geocoding | Dev: none. Prod: package `org.ripls.app` + two SHA1 fingerprints (upload key + Play App Signing key). |
| `google-maps-api-key-ios` | Maps SDK iOS only | Maps SDK iOS, Places, Geocoding | Dev: none. Prod: bundle ID `org.ripls.app`. The Xcode project's `PRODUCT_BUNDLE_IDENTIFIER` is `org.ripls.app.dev` for dev sim builds; fastlane's prod lane overrides to `org.ripls.app` at build time. |
| `google-maps-api-key-client-places` | Client-side Places + Geocoding REST calls from the Flutter `GoogleLocationSearchService`, **all platforms** (#2246) | Places API (New), Geocoding API | Dev: none. **Prod: none, by design.** App restrictions on the REST path are forgeable HTTP headers, not a real control; abuse is bounded by the narrow API restriction + quotas/budget alerts. See #2246. |

> **#2246 — client REST key split from the map-SDK keys.** Originally the
> per-platform `google-maps-api-key-{android,ios,web}` keys doubled as both
> the native map-SDK key *and* the key the Flutter REST client used for
> autocomplete/geocoding. That forced the client to send per-platform
> application-restriction headers (`X-Android-Cert`, `X-Ios-Bundle-Identifier`)
> on REST calls. Android broke in prod because Places API (New) only accepts
> the `X-Android-Cert` fingerprint in bare lowercase-hex form, while the app
> sent the colon-separated uppercase form → `API_KEY_ANDROID_APP_BLOCKED`.
> The fix introduced `google-maps-api-key-client-places` (above): one
> app-unrestricted, API-restricted key the client uses everywhere, injected
> as the `GOOGLE_MAPS_PLACES_KEY` --dart-define. The map-SDK keys above are
> now used *only* by the native SDKs (manifest / `GMSServices` / web JS loader).

> **Verified state (2026-05-27):** `gcloud services api-keys list --project ripls-dev`
> returns blank for every restriction column on every dev key — dev keys
> are intentionally permissive so the same key works from any developer
> machine. The `gcloud` invocations in "Applying application restrictions"
> below are still useful as documentation of the intended *prod* shape;
> applying them to dev would lock developers out of their own builds and
> is **not** what we do today.

## Full happy path (CLI)

```bash
./scripts/provision_google_maps_2188.sh ripls-dev
# When Phase 3 starts:
./scripts/provision_google_maps_2188.sh ripls-prod
```

The script:

1. Enables the API Keys API (`apikeys.googleapis.com`).
2. Enables every Maps Platform API needed across all phases.
3. Creates each of the four keys (idempotent: skips if a key with the
   matching display name already exists).
4. Applies API restrictions at create time.
5. Stores each key string in GSM as `google-maps-api-key-<suffix>`
   (creates the secret if absent; adds a new version if present).

It does **not** apply application restrictions — those need values we
gather separately (Cloud NAT IP, Android SHA1s). Apply them with the
commands in the next section.

## Applying application restrictions

Run after `provision_google_maps_2188.sh` once the per-platform values
exist. Each `gcloud services api-keys update` invocation replaces all
application restrictions on the key (additive flags don't exist for
these), so include every value in a single call.

### Web key

Dev (`ripls-dev`) — **not applied today** (dev keys are intentionally
unrestricted; this block is documentation of the intended shape if we
ever want a referrer restriction in dev):

```bash
WEB_KEY_NAME=$(gcloud services api-keys list \
  --filter="displayName='Ripls Google Maps web (ripls-dev)'" \
  --project=ripls-dev --format='value(name)')

gcloud services api-keys update "$WEB_KEY_NAME" \
  --project=ripls-dev \
  --allowed-referrers='https://dev.ripls.app/*,http://localhost:*,http://127.0.0.1:*'
```

Prod (`ripls-prod`):

```bash
WEB_KEY_NAME=$(gcloud services api-keys list \
  --filter="displayName='Ripls Google Maps web (ripls-prod)'" \
  --project=ripls-prod --format='value(name)')

gcloud services api-keys update "$WEB_KEY_NAME" \
  --project=ripls-prod \
  --allowed-referrers='https://ripls.app/*'
```

### Android key

Each Android key restricts by `package_name + sha1_fingerprint`. The
right SHA1 depends on whether the app uses **Play App Signing**
(default for new apps since 2021):

- **Play App Signing enabled** (typical): there are two keys.
  - **Upload key** — fastlane uses this to sign the AAB locally
    before uploading. Lives in the release keystore (base64-encoded
    in the `ANDROID_KEYSTORE_BASE64` GitHub secret). Its SHA1 is
    visible via `keytool -list -v -keystore release.keystore -alias
    <alias>`.
  - **App signing key** — Google manages this; re-signs the AAB
    before sending to user devices. **This is the SHA1 the API key
    restriction must use**, because at install time the device sees
    the app-signing-key signature, not the upload-key signature.
    Visible in Play Console → Setup → App integrity → App signing
    key certificate → "SHA-1 certificate fingerprint".
- **No Play App Signing**: one key (the upload key is also the
  signing key); use that SHA1.

For dev:

Get the debug SHA1 for a developer's machine:

```bash
keytool -list -v -keystore ~/.android/debug.keystore \
  -alias androiddebugkey -storepass android -keypass android \
  | grep 'SHA1:' | head -1
```

Apply (dev) — **not applied today** (dev key is intentionally
unrestricted; documented for completeness in case we want to lock it
down per-developer in the future):

```bash
ANDROID_KEY_NAME=$(gcloud services api-keys list \
  --filter="displayName='Ripls Google Maps android (ripls-dev)'" \
  --project=ripls-dev --format='value(name)')

# org.ripls.app.dev is the debug build's applicationId (see
# app/android/app/build.gradle for the productFlavors). Verify before
# applying.
gcloud services api-keys update "$ANDROID_KEY_NAME" \
  --project=ripls-dev \
  --allowed-application=sha1_fingerprint=AA:BB:CC:...,package_name=org.ripls.app.dev \
  --allowed-application=sha1_fingerprint=DD:EE:FF:...,package_name=org.ripls.app.dev
  # ↑ one --allowed-application per developer's debug SHA1
```

Apply (prod):

```bash
ANDROID_KEY_NAME=$(gcloud services api-keys list \
  --filter="displayName='Ripls Google Maps android (ripls-prod)'" \
  --project=ripls-prod --format='value(name)')

gcloud services api-keys update "$ANDROID_KEY_NAME" \
  --project=ripls-prod \
  --allowed-application=sha1_fingerprint=<RELEASE_SHA1>,package_name=org.ripls.app
```

The release SHA1 is the fingerprint of the keystore CI signs production
builds with (lives in GSM as `android-release-keystore` or similar).
Pull from CI's signing config when adding the binding.

### iOS key

iOS keys restrict by bundle ID. **Only prod is restricted today** —
the dev iOS key is intentionally permissive so any developer's sim
build (whose `PRODUCT_BUNDLE_IDENTIFIER` is `org.ripls.app.dev`)
authorizes. The shape below was applied to prod; it's documented for
dev too in case we ever want to lock the dev key down.

Prod (`ripls-prod`):

```bash
IOS_KEY_NAME=$(gcloud services api-keys list \
  --filter="displayName='Ripls Google Maps ios (ripls-prod)'" \
  --project=ripls-prod --format='value(name)')

gcloud services api-keys update "$IOS_KEY_NAME" \
  --project=ripls-prod \
  --allowed-bundle-ids=org.ripls.app
```

Dev (`ripls-dev`) — **not applied today**:

```bash
IOS_KEY_NAME=$(gcloud services api-keys list \
  --filter="displayName='Ripls Google Maps ios (ripls-dev)'" \
  --project=ripls-dev --format='value(name)')

gcloud services api-keys update "$IOS_KEY_NAME" \
  --project=ripls-dev \
  --allowed-bundle-ids=org.ripls.app
```

### Server key (prod only)

Prod server key gets an IP restriction once Cloud NAT is in place
(needs separate terraform work — see "Server IP restriction" below).
Dev server key stays unrestricted so it works from any developer
laptop.

```bash
SERVER_KEY_NAME=$(gcloud services api-keys list \
  --filter="displayName='Ripls Google Maps server (ripls-prod)'" \
  --project=ripls-prod --format='value(name)')

gcloud services api-keys update "$SERVER_KEY_NAME" \
  --project=ripls-prod \
  --allowed-ips=35.X.Y.Z   # Cloud NAT static IP
```

#### Server IP restriction (Cloud NAT prerequisite)

Cloud Run egress IPs are part of Google's shared pool by default —
too broad to restrict against. To get a stable IP for the key
restriction, route Cloud Run egress through the existing VPC connector
plus a Cloud NAT with a reserved static external IP. This is a
separate terraform change (out of scope for this PR; tracked in Phase
3 of #2188). Until then, the prod server key stays unrestricted —
acceptable because:

- GSM access requires a service-account binding (the key isn't on the
  wire).
- The key is only used server-to-Google, never embedded in client
  code.
- API restrictions + budget alerts cap the damage if leaked.

## What the terraform changes add

The PR's terraform diffs:

- Both `terraform/environments/{dev,prod}/main.tf` add
  `google-maps-api-key-server` to `server_runtime_secrets`. The Cloud
  Run runtime SA gets `roles/secretmanager.secretAccessor` on the new
  secret so `server/entrypoint.sh` can fetch it at startup.
- `terraform/environments/prod/main.tf` adds three additional
  per-secret IAM grants on `google-maps-api-key-{web,android,ios}` to
  the prod GitHub Actions SA so release workflows can fetch them for
  client builds.

After **you** run `terraform apply` per env (terraform is committed
but never applied by Claude):

```bash
# Verify
gcloud secrets get-iam-policy google-maps-api-key-server --project=ripls-dev
gcloud secrets get-iam-policy google-maps-api-key-web --project=ripls-prod
```

## Local dev story

The system is designed so all four local-dev paths work without any
production credentials.

### Local server (developer's laptop)

```bash
npm run start:server
```

`scripts/run_server_with_sm_secrets.sh` fetches
`google-maps-api-key-server` from `ripls-dev` via gcloud ADC. The dev
server key has **no IP restriction** so any laptop works. If the secret
hasn't been provisioned yet, the script writes an empty file (no
failure) and the server starts with the Google client disabled — the
existing Mapbox path keeps working.

### Local web (`flutter run -d chrome`)

The Flutter web build embeds `GOOGLE_MAPS_API_KEY_WEB` via
`--dart-define` (Phase 4 work; the pattern mirrors `MAPBOX_ACCESS_TOKEN`).
The dev web key's referrer allowlist includes `http://localhost:*` so
the Maps JS SDK loads against any port Flutter binds.

### Local Android (`flutter run` on emulator or USB device)

The Android Maps SDK reads the API key from `AndroidManifest.xml`'s
`com.google.android.geo.API_KEY` meta-data. Gradle injects this from
`GOOGLE_MAPS_API_KEY_ANDROID` (Phase 4 work).

The catch: the dev Android key restricts by `package_name + SHA1
fingerprint`, and **the debug keystore SHA1 differs per developer's
machine**. Each developer registers their SHA1 once:

```bash
# 1. Get your debug SHA1
keytool -list -v -keystore ~/.android/debug.keystore \
  -alias androiddebugkey -storepass android -keypass android \
  | grep 'SHA1:'

# 2. Add it to the dev Android key's allowed list. Have an existing
# project Owner run:
ANDROID_KEY_NAME=$(gcloud services api-keys list \
  --filter="displayName='Ripls Google Maps android (ripls-dev)'" \
  --project=ripls-dev --format='value(name)')

# Note: --allowed-application is replace-all, not additive. The Owner
# must include every existing developer's SHA1 + the new one. Easier
# pattern: keep the list of registered SHA1s in
# infra/google-maps-android-dev-fingerprints.txt (one per line) and
# regenerate the update command from that file.
```

The annoying-but-real workflow: dev onboarding includes "send your
debug SHA1 to <owner> for registration." For a single-developer team,
do it once and forget. For a larger team, automate via a CI-driven
keychain or use a shared debug keystore committed to a private repo
location.

### Local iOS (Xcode simulator or USB device)

The iOS Maps SDK reads the API key from a `Config.swift` or Info.plist
entry that fastlane embeds at build time (Phase 4 work).

Dev iOS key restricts by bundle ID, which matches between dev and
prod today, so no per-developer setup is needed. `flutter run -d ios`
on any developer's machine works as long as the build embeds
`GOOGLE_MAPS_API_KEY_IOS`.

## Budget alert

Budgets are billing-account-scoped, not project-scoped. They generate
**email notifications** at the configured thresholds — they do NOT
hard-cap spending. To cap actual dollars you'd need either:

- **Per-API daily quota caps** (per-request-count, not per-dollar) —
  see "Quota caps" below.
- A Pub/Sub-triggered Cloud Function that disables the billing
  account when a threshold fires. This kills **all** GCP services in
  the project (including the Cloud Run server) — nuclear option,
  not recommended.

### CLI form (used to set up the current alerts)

```bash
BILLING_ACCOUNT=01911A-DA95C0-B0C18F   # Type2Fun, LLC Billing
DEV_PROJECT_NUM=$(gcloud projects describe ripls-dev --format='value(projectNumber)')
PROD_PROJECT_NUM=$(gcloud projects describe ripls-prod --format='value(projectNumber)')

# Maps Platform services this budget tracks
MAPS_SERVICES="services/places.googleapis.com,services/geocoding-backend.googleapis.com,services/maps-android-backend.googleapis.com,services/maps-ios-backend.googleapis.com,services/maps-backend.googleapis.com"

# One-time: enable the Budgets API on the project that issues the call
gcloud services enable billingbudgets.googleapis.com --project=ripls-dev
gcloud services enable billingbudgets.googleapis.com --project=ripls-prod

# Dev budget — $50/month
gcloud billing budgets create \
  --billing-account="$BILLING_ACCOUNT" \
  --display-name="Ripls Google Maps (ripls-dev) #2188" \
  --budget-amount=50USD \
  --threshold-rule=percent=0.5 \
  --threshold-rule=percent=0.9 \
  --threshold-rule=percent=1.0 \
  --filter-projects="projects/$DEV_PROJECT_NUM" \
  --filter-services="$MAPS_SERVICES"

# Prod budget — $500/month
gcloud billing budgets create \
  --billing-account="$BILLING_ACCOUNT" \
  --display-name="Ripls Google Maps (ripls-prod) #2188" \
  --budget-amount=500USD \
  --threshold-rule=percent=0.5 \
  --threshold-rule=percent=0.9 \
  --threshold-rule=percent=1.0 \
  --filter-projects="projects/$PROD_PROJECT_NUM" \
  --filter-services="$MAPS_SERVICES"
```

Notifications go to billing-account admins by email by default.
Revise the dollar amounts after Phase 4 surfaces real map-load
volume.

**#2246 — $100 prod tripwire budget.** A second prod budget on the
same Maps services, as an abuse early-warning well below the $500
ceiling (emails at $50 / $90 / $100). Budgets can't filter by API
key, so this watches all Maps spend — but a spike from a leaked
client key shows up here within days.

```bash
gcloud billing budgets create \
  --billing-account="$BILLING_ACCOUNT" \
  --display-name="Ripls Google Maps client-places tripwire (ripls-prod) #2246" \
  --budget-amount=100USD \
  --threshold-rule=percent=0.5 \
  --threshold-rule=percent=0.9 \
  --threshold-rule=percent=1.0 \
  --filter-projects="projects/$PROD_PROJECT_NUM" \
  --filter-services="$MAPS_SERVICES"
```

### Quota caps (#2246 — APPLIED in prod)

Per-API **daily** quotas are a hard request ceiling. The per-minute
project caps already exist by default (autocomplete 12000/min,
geocoding 3000/min), but the per-day caps were unlimited — the open
hole a leaked key could exploit overnight. Applied 2026-05-29 in
ripls-prod (`1/d/{project}` = 25000 on each):

```bash
# Places autocomplete + place details, and classic Geocoding v3.
gcloud alpha services quota update --force --consumer=projects/507242812633 \
  --service=places.googleapis.com \
  --metric=places.googleapis.com/AutocompletePlacesRequest \
  --unit='1/d/{project}' --value=25000
gcloud alpha services quota update --force --consumer=projects/507242812633 \
  --service=places.googleapis.com \
  --metric=places.googleapis.com/GetPlaceRequest \
  --unit='1/d/{project}' --value=25000
gcloud alpha services quota update --force --consumer=projects/507242812633 \
  --service=geocoding-backend.googleapis.com \
  --metric=geocoding-backend.googleapis.com/billable_default \
  --unit='1/d/{project}' --value=25000
```

**Important caveats:** (1) quotas are **per-project**, so these bound
the client *and* the server-side Google Maps usage together — not just
the client-places key. (2) 25000/day is a catastrophe-stop, far above
current scale; hitting it returns `OVER_QUERY_LIMIT` / `RESOURCE_EXHAUSTED`
to real users, so **raise it before any large user-growth step** rather
than discovering it via broken search. Not aligned to the $100 budget on
purpose: the budget is the early *alert*; the quota is the hard *ceiling*.

## Phase 0 parity audit

```bash
export GOOGLE_MAPS_API_KEY=$(gcloud secrets versions access latest \
  --secret=google-maps-api-key-server --project=ripls-dev)
export MAPBOX_ACCESS_TOKEN=$(gcloud secrets versions access latest \
  --secret=mapbox-access-token --project=ripls-dev)

npm run parity:2188
```

The harness uses the **server** key (it makes server-to-Google API
calls, not client-SDK loads). Fill in
[`2188-parity-audit.md`](2188-parity-audit.md) with the results.

## Server flag (already wired in this PR)

`server/main.go` gains `--google-maps-api-key-server` and
`--google-maps-api-key-server-file`. `server/entrypoint.sh` fetches the
secret on Cloud Run startup as an **optional** fetch — if the secret
doesn't exist or the SA can't access it, the entry-point writes an
empty file and the server starts with the Google client disabled. The
existing Mapbox path is unaffected.

## Key rotation

Per-key rotation procedure (mirror per platform):

```bash
PROJECT=ripls-prod
SUFFIX=server   # or web/android/ios
DISPLAY="Ripls Google Maps $SUFFIX ($PROJECT)"
SECRET=google-maps-api-key-$SUFFIX

# 1. Create a new key with the same display name + suffix
#    (the script will skip if it sees an existing key with this name,
#    so use a slightly different display name for the rotation key,
#    e.g. append "v2", or delete the old key first).
NEW_KEY_NAME=$(gcloud services api-keys create \
  --display-name="$DISPLAY v2" \
  --api-target=service=... \
  --project=$PROJECT --format='value(response.name)')

# 2. Pull the new key string and add it as a new GSM version.
NEW_KEY=$(gcloud services api-keys get-key-string "$NEW_KEY_NAME" \
  --project=$PROJECT --format='value(keyString)')
printf '%s' "$NEW_KEY" | gcloud secrets versions add "$SECRET" \
  --data-file=- --project=$PROJECT

# 3. Roll Cloud Run revisions (server-side) or trigger the next app
# release (mobile/web).

# 4. After confirming the new revision is healthy, disable the old key.
gcloud services api-keys delete "$OLD_KEY_NAME" --project=$PROJECT
```

## Monitoring

After Phase 3 deploys:

- **GCP Console → APIs & Services → Dashboard.** Per-API request rate.
  A sudden spike is the first sign of a bug (e.g. a retry loop).
- **Budget alert emails.** If 50% fires within a week, recalibrate
  Phase 3's mitigations (session tokens, server-side caching) before
  continuing.
- **Cloud Logging:** `jsonPayload.external_service="google_maps"`
  will show per-RPC volume and error rates once `GoogleMapsClient` is
  logging via `logging.LoggerWithContext`. Filter
  `severity>=WARNING` to see denied requests and quota errors.

## References

- Google Maps Platform pricing: <https://cloud.google.com/maps-platform/pricing>
- Places API (New) docs: <https://developers.google.com/maps/documentation/places/web-service/op-overview>
- Geocoding API: <https://developers.google.com/maps/documentation/geocoding/start>
- `gcloud services api-keys`: <https://cloud.google.com/sdk/gcloud/reference/services/api-keys>
- Maps SDK key restrictions: <https://developers.google.com/maps/api-security-best-practices>
