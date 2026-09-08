# =============================================================================
# METABASE MODULE OUTPUTS
# =============================================================================

output "service_url" {
  description = "URL of the Metabase Cloud Run service"
  value       = google_cloud_run_v2_service.metabase.uri
}

output "service_name" {
  description = "Name of the Metabase Cloud Run service"
  value       = google_cloud_run_v2_service.metabase.name
}
