# =============================================================================
# MEDIA BUCKET MODULE OUTPUTS
# =============================================================================

output "bucket_name" {
  description = "Name of the GCS bucket for media storage"
  value       = google_storage_bucket.media_bucket.name
}
