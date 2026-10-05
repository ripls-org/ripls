# =============================================================================
# SERVER IDENTITY MODULE OUTPUTS
# =============================================================================

output "email" {
  description = "Email of the server's service account, for creating its key"
  value       = google_service_account.server.email
}
