# =============================================================================
# PRODUCTION ENVIRONMENT OUTPUTS
# =============================================================================

output "server_identity_email" {
  description = "Service account the self-hosted server authenticates as (its key is created out of band)"
  value       = module.server_identity.email
}
