# =============================================================================
# PRODUCTION ENVIRONMENT
# =============================================================================
# This configuration deploys the ripls server to a production environment
# with minimal resources but enhanced reliability and security.

terraform {
  required_version = ">= 1.0"

  backend "gcs" {
    bucket = "ripls-prod-terraform-state"
    prefix = "state"
  }

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 5.0"
    }
  }
}

# Configure the Google Cloud Provider. billing_project/user_project_override
# attribute API quota to the project instead of relying on the operator's
# local ADC quota-project setting — account-level APIs (billingbudgets)
# reject ADC calls without one.
provider "google" {
  project               = var.project_id
  region                = var.region
  billing_project       = var.project_id
  user_project_override = true
}

# Configure the Cloudflare Provider
provider "cloudflare" {
  # API token should be set via CLOUDFLARE_API_TOKEN environment variable
}

# Project metadata used by IAM grants below.
data "google_project" "current" {
  project_id = var.project_id
}

# =============================================================================
# CLOUD FUNCTIONS RUNTIME IAM (#1148+#1149)
# =============================================================================
# The Cloud Functions v2 runtime SA (Compute Engine default SA) needs read
# access to the alerts GitHub App private key so cloud_functions/ can
# authenticate as ripls-alerts-bot at runtime. This is a *separate* secret
# from the server's github-app-private-key-base64 (used by ripls-feedback-bot
# for the user feedback service) — the two App identities are kept distinct
# so claude_plan_issue.yaml's bot allowlist doesn't transitively allow
# user-supplied feedback content to trigger Claude planning. See PR #1762
# discussion for the trust-boundary analysis.
#
# Cloud Functions binds the alerts App's secret natively at runtime via
# firebase-functions defineSecret, which requires an explicit grant.

# Runtime SA → alerts App private key. Used at function invocation time by
# every handler in cloud_functions/ via octokitFromAppSecret().
resource "google_secret_manager_secret_iam_member" "functions_github_app_alerts_private_key_runtime" {
  project   = var.project_id
  secret_id = "github-app-alerts-private-key-base64"
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${data.google_project.current.number}-compute@developer.gserviceaccount.com"
}

# Deploy SA → alerts App private key. Required because the deploy CLI
# inspects the secret (verifies it exists, reads metadata) when binding
# it to the function via defineSecret. secretAccessor covers payload
# (versions.access); secretmanager.viewer adds metadata reads
# (secrets.get) which secretAccessor does not include.
resource "google_secret_manager_secret_iam_member" "functions_github_app_alerts_private_key_deploy_accessor" {
  project   = var.project_id
  secret_id = "github-app-alerts-private-key-base64"
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:cloud-functions-deployer@${var.project_id}.iam.gserviceaccount.com"
}

resource "google_secret_manager_secret_iam_member" "functions_github_app_alerts_private_key_deploy_viewer" {
  project   = var.project_id
  secret_id = "github-app-alerts-private-key-base64"
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:cloud-functions-deployer@${var.project_id}.iam.gserviceaccount.com"
}

# The functions now file in the private tracker on Forgejo (#3293) as the
# `ripls-alerts` account, whose access token is this secret. Container only:
# the token is added out of band so it never lands in TF state or the repo:
#   gcloud secrets versions add forgejo-alerts-token --data-file=- --project=<project>
# The three grants mirror the ones above, which stay until the GitHub App key
# is retired.
resource "google_secret_manager_secret" "forgejo_alerts_token" {
  project   = var.project_id
  secret_id = "forgejo-alerts-token"
  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_iam_member" "functions_forgejo_alerts_token_runtime" {
  project   = var.project_id
  secret_id = google_secret_manager_secret.forgejo_alerts_token.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${data.google_project.current.number}-compute@developer.gserviceaccount.com"
}

resource "google_secret_manager_secret_iam_member" "functions_forgejo_alerts_token_deploy_accessor" {
  project   = var.project_id
  secret_id = google_secret_manager_secret.forgejo_alerts_token.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:cloud-functions-deployer@${var.project_id}.iam.gserviceaccount.com"
}

resource "google_secret_manager_secret_iam_member" "functions_forgejo_alerts_token_deploy_viewer" {
  project   = var.project_id
  secret_id = google_secret_manager_secret.forgejo_alerts_token.secret_id
  role      = "roles/secretmanager.viewer"
  member    = "serviceAccount:cloud-functions-deployer@${var.project_id}.iam.gserviceaccount.com"
}

# =============================================================================
# DEPLOYS FROM FORGEJO ACTIONS (#3293)
# =============================================================================
# Forgejo Actions issues OpenID Connect tokens the way GitHub Actions does
# (a job opts in with `enable-openid-connect: true`), so a run can exchange
# one for the cloud-functions-deployer account's credentials with no stored
# key. This is the Forgejo counterpart of the `github-actions` pool, which was
# created out of band and trusts the GitHub repository.
#
# The provider accepts a token only from one repository, by its numeric id (a
# name can be re-registered; an id cannot), and only for a run on `main` that
# is not a pull request. A fork's pull request run therefore cannot deploy,
# whatever its workflow says.
resource "google_iam_workload_identity_pool" "forgejo_actions" {
  project                   = var.project_id
  workload_identity_pool_id = "forgejo-actions"
  display_name              = "Forgejo Actions"
  description               = "Actions runs on the Forgejo instance (#3293)"
}

resource "google_iam_workload_identity_pool_provider" "forgejo" {
  project                            = var.project_id
  workload_identity_pool_id          = google_iam_workload_identity_pool.forgejo_actions.workload_identity_pool_id
  workload_identity_pool_provider_id = "forgejo"
  display_name                       = "Forgejo"

  oidc {
    issuer_uri = "${var.forgejo_url}/api/actions"
  }

  attribute_mapping = {
    "google.subject"          = "assertion.sub"
    "attribute.repository"    = "assertion.repository"
    "attribute.repository_id" = "assertion.repository_id"
    "attribute.ref"           = "assertion.ref"
    "attribute.event_name"    = "assertion.event_name"
  }

  attribute_condition = join(" && ", [
    "assertion.repository_id == '${var.forgejo_repository_id}'",
    "assertion.ref == 'refs/heads/main'",
    "assertion.event_name != 'pull_request'",
    "assertion.event_name != 'pull_request_target'",
  ])
}

# Runs that pass the provider's condition may act as the deployer.
resource "google_service_account_iam_member" "functions_deployer_from_forgejo" {
  service_account_id = "projects/${var.project_id}/serviceAccounts/cloud-functions-deployer@${var.project_id}.iam.gserviceaccount.com"
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.forgejo_actions.name}/attribute.repository_id/${var.forgejo_repository_id}"
}

# =============================================================================
# SERVER RUNTIME SECRETS (#1768, #1770)
# =============================================================================
# The credentials the server's entrypoint fetches from Secret Manager at
# startup. module "server_identity" grants its service account secretAccessor
# on each.
locals {
  server_runtime_secrets = [
    "openai-api-key",
    "anthropic-api-key",
    "mailgun-api-key",
    "mailgun-webhook-signing-key",
    "unsplash-access-key",
    "pexels-api-key",
    "pixabay-api-key",
    # Server-side Google Maps key (#2188). The web/android/ios keys are
    # not fetched by the server — they live in GSM under
    # google-maps-api-key-{web,android,ios} and are accessed only by
    # the respective build pipelines via the GitHub Actions SA grants
    # below.
    "google-maps-api-key-server",
    "jwt-signing-secret",
    "github-app-private-key-base64",
    # Twilio platform SMS (#2492). All three exist in ripls-prod GSM.
    "twilio-account-sid",
    "twilio-auth-token",
    "twilio-messaging-service-sid",
  ]
}

# =============================================================================
# FEEDBACK SCREENSHOT URL SIGNER (#2549)
# =============================================================================
# Feedback screenshots embedded in GitHub issues need durable signed URLs that
# stay valid for months. The runtime SA's IAM signBlob signature dies when its
# Google-managed key rotates (~1-2 weeks), so the server signs these URLs with a
# dedicated, user-managed key instead. This SA does nothing else, so a leak of
# its key only exposes read access to feedback-system/ objects.
#
# terraform apply creates the SA, the (empty) secret container and the scoped
# objectViewer IAM; module "server_identity" grants the server read on the
# secret. The KEY MATERIAL is
# added out of band so it never lands in TF state or the repo (#1769):
#   gcloud iam service-accounts keys create /tmp/fb-signer.json \
#     --iam-account=${google_service_account.feedback_url_signer.email}
#   gcloud secrets versions add feedback-signer-key --data-file=/tmp/fb-signer.json \
#     --project=<project>   # then: shred -u /tmp/fb-signer.json
# Rotating/deleting the key invalidates every outstanding feedback URL — plan a
# Phase 2 re-backfill when rotating (see the #2549 plan, archived).
resource "google_service_account" "feedback_url_signer" {
  project      = var.project_id
  account_id   = "feedback-url-signer"
  display_name = "Feedback screenshot URL signer (#2549)"
}

# Secret container only — terraform never holds the key material (the version is
# added out of band, see above). Managed here (unlike the other out-of-band
# secrets) so the grant below can depend on it, letting a single apply create
# SA → container → grant without an ordering cycle.
resource "google_secret_manager_secret" "feedback_signer_key" {
  project   = var.project_id
  secret_id = "feedback-signer-key"
  replication {
    auto {}
  }
}

# Access token for the Forgejo account that files user-feedback issues, used
# when the server runs with FEEDBACK_FORGEJO_URL (#3293). Container only, like
# the signer key above: the token is added out of band so it never lands in TF
# state or the repo:
#   gcloud secrets versions add feedback-forgejo-token --data-file=- --project=<project>
# The server fetches it as an optional secret, so an empty container leaves
# feedback on GitHub or unconfigured.
resource "google_secret_manager_secret" "feedback_forgejo_token" {
  project   = var.project_id
  secret_id = "feedback-forgejo-token"
  replication {
    auto {}
  }
}

# A signed URL grants its bearer whatever the signer can do, so the signer needs
# object read — scoped by IAM Condition to the feedback-system/ prefix so the key
# can only ever authorize feedback screenshots, never other media.
resource "google_storage_bucket_iam_member" "feedback_url_signer_object_viewer" {
  bucket = module.media_bucket.bucket_name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${google_service_account.feedback_url_signer.email}"

  condition {
    title       = "feedback-system-objects-only"
    description = "Limit the signer to feedback screenshot objects (#2549)."
    expression  = "resource.name.startsWith(\"projects/_/buckets/${module.media_bucket.bucket_name}/objects/feedback-system/\")"
  }
}

# =============================================================================
# SELF-HOSTED SERVER IDENTITY (#3074)
# =============================================================================
# The server's identity on mimir, which has no metadata server and
# authenticates with this account's key. The key is created out of band (the
# secret-rotation runbook, kept privately) and lives on the host at
# /etc/nas/ripls-server-prod-sa.json.
module "server_identity" {
  source = "../../modules/server_identity"

  project_id   = var.project_id
  account_id   = "ripls-server-mimir"
  display_name = "Server runtime (mimir)"
  secret_ids = concat(
    local.server_runtime_secrets,
    [
      google_secret_manager_secret.feedback_signer_key.secret_id,
      google_secret_manager_secret.feedback_forgejo_token.secret_id,
    ],
  )
  media_bucket = module.media_bucket.bucket_name
}

# Grant the prod GitHub Actions SA secretAccessor on each build-time
# secret that the CI pipeline injects via scripts/fetch_secret.sh:
#   - firebase-web-api-key — embedded in the Flutter Web bundle by
#     build:web:prod (the apiKey field of FirebaseOptions for web init).
# Server-side runtime credentials are read by the server's own identity
# above, not by this CI SA, so the grant stays scoped per-secret.
locals {
  github_actions_ci_build_secrets = [
    "firebase-web-api-key",
  ]
}

resource "google_secret_manager_secret_iam_member" "github_actions_prod_ci" {
  for_each  = toset(local.github_actions_ci_build_secrets)
  project   = var.project_id
  secret_id = each.value
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:github-actions@ripls-prod.iam.gserviceaccount.com"
}

# Same grant pattern for the client-side Google Maps keys (#2188).
# Each platform's build pipeline reads the matching key from GSM via
# scripts/fetch_secret.sh and embeds it at build time:
#   - google-maps-api-key-web:     Flutter web build → index.html script tag
#   - google-maps-api-key-android: Gradle manifest placeholder → AndroidManifest meta-data
#   - google-maps-api-key-ios:     fastlane → Info.plist → AppDelegate
# The server-side key (google-maps-api-key-server) is read by the server's
# own identity above; the CI SA doesn't need access to it.
resource "google_secret_manager_secret_iam_member" "github_actions_prod_google_maps_web" {
  project   = var.project_id
  secret_id = "google-maps-api-key-web"
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:github-actions@ripls-prod.iam.gserviceaccount.com"
}

resource "google_secret_manager_secret_iam_member" "github_actions_prod_google_maps_android" {
  project   = var.project_id
  secret_id = "google-maps-api-key-android"
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:github-actions@ripls-prod.iam.gserviceaccount.com"
}

resource "google_secret_manager_secret_iam_member" "github_actions_prod_google_maps_ios" {
  project   = var.project_id
  secret_id = "google-maps-api-key-ios"
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:github-actions@ripls-prod.iam.gserviceaccount.com"
}

# Client-side Places/Geocoding key (#2246). A single, app-unrestricted,
# API-restricted (Places + Geocoding only) key the Flutter REST client
# embeds on every platform — replacing the per-platform map-SDK keys that
# previously doubled as the autocomplete key and the forgeable
# X-Android-Cert / X-Ios-Bundle-Identifier header dance. All three build
# pipelines (iOS, Android, web) read it from GSM and pass it as the
# GOOGLE_MAPS_PLACES_KEY --dart-define.
resource "google_secret_manager_secret_iam_member" "github_actions_prod_google_maps_client_places" {
  project   = var.project_id
  secret_id = "google-maps-api-key-client-places"
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:github-actions@ripls-prod.iam.gserviceaccount.com"
}

# =============================================================================
# REQUIRED APIS
# =============================================================================
# Enable APIs that aren't enabled by default
resource "google_project_service" "aiplatform" {
  project = var.project_id
  service = "aiplatform.googleapis.com"

  disable_on_destroy = false
}

resource "google_project_service" "iamcredentials" {
  project = var.project_id
  service = "iamcredentials.googleapis.com"

  disable_on_destroy = false
}

# Required for Firebase Phone Auth on Android. When enabled, Firebase uses
# invisible Play Integrity attestation instead of showing a reCAPTCHA challenge.
resource "google_project_service" "playintegrity" {
  project = var.project_id
  service = "playintegrity.googleapis.com"

  disable_on_destroy = false
}

resource "google_project_service" "billingbudgets" {
  project = var.project_id
  service = "billingbudgets.googleapis.com"

  disable_on_destroy = false
}

# =============================================================================
# BILLING BUDGET (#2809)
# =============================================================================
# Account-wide monthly budget (covers ripls-dev and ripls-prod). The #2809
# cost review found two lines that had been compounding for months (+20%/mo
# Artifact Registry, +9%/mo probe-driven geocoding) with no alarm; this makes
# the next anomaly visible within days. With the server and database on
# mimir (#3079) what is left is mostly Gemini usage, ~$12-20/mo, so $40
# trips on a real regression without crying wolf. Emails go to the
# billing account admins/users (the default recipients) at 50%, 80%, and
# 100% of actual spend. Lives in the prod root because it is account-level,
# not per-project.
resource "google_billing_budget" "monthly" {
  billing_account = var.billing_account_id
  display_name    = "ripls monthly budget"

  amount {
    specified_amount {
      currency_code = "USD"
      units         = "40"
    }
  }

  threshold_rules {
    threshold_percent = 0.5
  }
  threshold_rules {
    threshold_percent = 0.8
  }
  threshold_rules {
    threshold_percent = 1.0
  }

  depends_on = [google_project_service.billingbudgets]
}

# =============================================================================
# CI/CD SERVICE ACCOUNT PERMISSIONS
# =============================================================================
# GitHub Actions service account from ripls-dev project
locals {
  github_actions_sa = "serviceAccount:github-actions@ripls-dev.iam.gserviceaccount.com"
}

# Grant Vertex AI permissions
resource "google_project_iam_member" "github_actions_vertex_ai" {
  project = var.project_id
  role    = "roles/aiplatform.user"
  member  = local.github_actions_sa
}

# =============================================================================
# MEDIA STORAGE
# =============================================================================
module "media_bucket" {
  source = "../../modules/media_bucket"

  name               = "ripls-${var.environment}"
  project_id         = var.project_id
  region             = var.region
  versioning_enabled = true # Enable versioning for data protection
}

moved {
  from = module.storage.google_storage_bucket.media_bucket
  to   = module.media_bucket.google_storage_bucket.media_bucket
}

# =============================================================================
# DNS CONFIGURATION
# =============================================================================
# Both prod hostnames point at mimir since the cutover (#3078); Caddy there
# terminates TLS.
resource "cloudflare_dns_record" "server" {
  zone_id = var.cloudflare_zone_id
  name    = "server"
  content = "home.overt.org"
  type    = "CNAME"
  ttl     = 1     # Auto TTL
  proxied = false # Caddy on mimir needs the real client connection
  comment = "ripls prod server on mimir"
}

# ripls.app — invite links and the web app. Created by hand before Terraform
# managed DNS; imported so both prod hostnames are managed here. Cloudflare
# flattens the apex CNAME.
resource "cloudflare_dns_record" "app" {
  zone_id = var.cloudflare_app_zone_id
  name    = "ripls.app"
  content = "home.overt.org"
  type    = "CNAME"
  ttl     = 1
  proxied = false
  comment = "ripls prod web app and invite links on mimir"
}

import {
  to = cloudflare_dns_record.app
  id = "7a22ea9cff9f85e4a27fc4f8be7e8ec3/ca344bd93dc6a67ede9174efff4d13f5"
}

# =============================================================================
# MONITORING
# =============================================================================
module "monitoring" {
  source = "../../modules/monitoring"

  project_id  = var.project_id
  region      = var.region
  environment = var.environment
  # The uptime check probes the public hostname, which Caddy on mimir serves.
  service_name        = "server"
  service_url         = "https://server.ripls.org"
  enable_uptime_check = true

  # Since the cutover (#3078) the mimir stack's log shipper supplies the logs,
  # as generic_task entries with job = service_name ("server").
  log_source = "self_hosted"

  # Alert recipients
  alert_email_addresses = ["ops@ripls.org"]

  # Phase 2 of #1148+#1149: route every alert to a Pub/Sub topic that the
  # ripls-alerts Cloud Function consumes, in parallel with the email
  # channel. Email is preserved as a backup signal during rollout.
  enable_github_pubsub_channel = true
}