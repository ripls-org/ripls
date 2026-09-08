# =============================================================================
# CONTAINER MODULE OUTPUTS
# =============================================================================

output "service_url" {
  description = "URL of the deployed server service"
  value       = google_cloud_run_v2_service.server.uri
}

output "service_name" {
  description = "Name of the deployed server service"
  value       = google_cloud_run_v2_service.server.name
}
