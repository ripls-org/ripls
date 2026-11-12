# =============================================================================
# MEDIA BUCKET MODULE - GCS
# =============================================================================
# The bucket for user-uploaded media (images, videos). Split out of the storage
# module when Cloud SQL was decommissioned (#3079), so the bucket and its data
# outlive the database. The server on mimir reads and writes it with its own
# service account (modules/server_identity), and signs URLs for it.

resource "google_storage_bucket" "media_bucket" {
  name     = var.name
  location = var.region
  project  = var.project_id

  # Storage class for cost optimization
  storage_class = var.storage_class

  # Lifecycle rules for automatic deletion of old media (optional)
  dynamic "lifecycle_rule" {
    for_each = var.lifecycle_days > 0 ? [1] : []
    content {
      condition {
        age = var.lifecycle_days
      }
      action {
        type = "Delete"
      }
    }
  }

  versioning {
    enabled = var.versioning_enabled
  }

  # CORS configuration for direct browser uploads (if needed in future)
  cors {
    origin          = var.cors_origins
    method          = ["GET", "HEAD"]
    response_header = ["Content-Type"]
    max_age_seconds = 3600
  }

  # Public access prevention - uniform bucket-level access enforces IAM only
  uniform_bucket_level_access = true

  # User data: never emptied or destroyed by Terraform, in any environment.
  force_destroy = false

  lifecycle {
    prevent_destroy = true
  }
}
