# =============================================================================
# DEVELOPMENT ENVIRONMENT
# =============================================================================
# This configuration deploys the ripls server to a development environment
# with minimal resources for cost efficiency and testing.

terraform {
  required_version = ">= 1.0"

  backend "gcs" {
    bucket = "ripls-dev-terraform-state"
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

# Configure the Google Cloud Provider
provider "google" {
  project = var.project_id
  region  = var.region
}

# Configure the Cloudflare Provider
provider "cloudflare" {
  # API token should be set via CLOUDFLARE_API_TOKEN environment variable
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
    # the respective build pipelines.
    "google-maps-api-key-server",
    "jwt-signing-secret",
    "github-app-private-key-base64",
    # Twilio platform SMS (#2492). Dev shares prod's Twilio account, campaign,
    # and Messaging Service (a 10DLC campaign binds to one Messaging Service, so
    # a separate dev campaign isn't worth it) — these mirror the prod values.
    # The server still won't send until -platform-sms-enabled flips on.
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
# /etc/nas/ripls-server-dev-sa.json.
module "server_identity" {
  source = "../../modules/server_identity"

  project_id   = var.project_id
  account_id   = "ripls-server-mimir"
  display_name = "Server runtime (mimir)"
  secret_ids = concat(
    local.server_runtime_secrets,
    [google_secret_manager_secret.feedback_signer_key.secret_id],
  )
  media_bucket = module.media_bucket.bucket_name
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

# =============================================================================
# CI/CD SERVICE ACCOUNT PERMISSIONS
# =============================================================================
# GitHub Actions service account in ripls-dev project
locals {
  github_actions_sa = "serviceAccount:github-actions@ripls-dev.iam.gserviceaccount.com"
}

# Grant Vertex AI permissions for integration tests
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

  name       = "ripls-media-dev"
  project_id = var.project_id
  region     = var.region
}

moved {
  from = module.storage.google_storage_bucket.media_bucket
  to   = module.media_bucket.google_storage_bucket.media_bucket
}

# =============================================================================
# DNS CONFIGURATION
# =============================================================================
# Both dev hostnames point at mimir (#3078), whose Caddy terminates TLS and
# proxies to the ripls_server_dev stack. Unproxied: Cloudflare's proxy would cut
# the server's long-lived streams.
resource "cloudflare_dns_record" "server_dev" {
  zone_id = var.cloudflare_zone_id
  name    = "server-dev"
  content = "home.overt.org"
  type    = "CNAME"
  ttl     = 1 # Auto TTL
  proxied = false
  comment = "ripls dev server on mimir"
}

# dev.ripls.app — invite links and the web app. Created by hand before
# Terraform managed DNS; imported so both dev hostnames are managed here.
resource "cloudflare_dns_record" "app_dev" {
  zone_id = var.cloudflare_app_zone_id
  name    = "dev"
  content = "home.overt.org"
  type    = "CNAME"
  ttl     = 1
  proxied = false
  comment = "ripls dev web app and invite links on mimir"
}

import {
  to = cloudflare_dns_record.app_dev
  id = "7a22ea9cff9f85e4a27fc4f8be7e8ec3/ae905de58db8f753a6e2ea77b2232199"
}

# =============================================================================
# MONITORING
# =============================================================================
module "monitoring" {
  source = "../../modules/monitoring"

  project_id   = var.project_id
  region       = var.region
  environment  = var.environment
  service_name = "server-dev"
  # The public hostname, which the uptime check probes and alerts link to. It
  # was Cloud Run's own *.run.app URL until the cutover (#3078).
  service_url = "https://server-dev.ripls.org"

  # Dev runs on mimir: its log shipper supplies the server's logs as
  # generic_task entries with job = service_name ("server-dev").
  log_source = "self_hosted"

  # Alert recipients
  alert_email_addresses = ["ops@ripls.org"]

  # On again since the cutover: it was off (#2809) only because its probes kept
  # Cloud Run's scale-to-zero dev server warm, and mimir's is always on.
  enable_uptime_check = true
}
