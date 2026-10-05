# =============================================================================
# SERVER IDENTITY MODULE
# =============================================================================
# A service account for running the server outside Google Cloud, where there is
# no metadata server and the server authenticates with this account's key
# (GOOGLE_APPLICATION_CREDENTIALS). It holds exactly what the server calls with
# its own identity — unlike the Compute Engine default service account Cloud
# Run uses, which also carries project Editor.
#
# The key itself is created out of band so the private key never lands in
# Terraform state; see the key runbook in the deployment's secret docs.

resource "google_service_account" "server" {
  project      = var.project_id
  account_id   = var.account_id
  display_name = var.display_name
  description  = "Server runtime identity for hosts outside Google Cloud; authenticates with a key."
}

locals {
  member = "serviceAccount:${google_service_account.server.email}"

  # Project-level roles, one per Google API the server calls with its identity.
  # Firebase Auth needs none: the server only verifies ID tokens, which checks
  # them against Google's public certificates.
  project_roles = [
    "roles/aiplatform.user",              # Vertex AI / Gemini, including the CountTokens health check
    "roles/firebasecloudmessaging.admin", # FCM sends (cloudmessaging.messages.create)
    "roles/logging.logWriter",            # the host ships the server's logs to Cloud Logging
  ]
}

# Every credential the container entrypoint fetches at startup.
resource "google_secret_manager_secret_iam_member" "secrets" {
  for_each  = toset(var.secret_ids)
  project   = var.project_id
  secret_id = each.value
  role      = "roles/secretmanager.secretAccessor"
  member    = local.member
}

# Media: objectAdmin for put/get/copy/delete. It does not include
# storage.buckets.get, which the storage health check (Bucket.Attrs) needs and
# which gates readiness, hence legacyBucketReader alongside. Signed URLs are
# signed locally with the key, so there is no signBlob grant.
resource "google_storage_bucket_iam_member" "media_objects" {
  bucket = var.media_bucket
  role   = "roles/storage.objectAdmin"
  member = local.member
}

resource "google_storage_bucket_iam_member" "media_bucket_read" {
  bucket = var.media_bucket
  role   = "roles/storage.legacyBucketReader"
  member = local.member
}

resource "google_project_iam_member" "project_roles" {
  for_each = toset(local.project_roles)
  project  = var.project_id
  role     = each.value
  member   = local.member
}
