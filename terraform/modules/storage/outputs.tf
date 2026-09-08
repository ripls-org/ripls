# =============================================================================
# STORAGE MODULE OUTPUTS
# =============================================================================

output "database_host" {
  description = "Database host (private IP)"
  value       = local.database_host
}

output "database_port" {
  description = "Database port"
  value       = local.database_port
}

output "database_name" {
  description = "Name of the application database"
  value       = google_sql_database.gear_database.name
}

output "database_user" {
  description = "Database user name"
  value       = google_sql_user.gear_user.name
}

output "vpc_network_name" {
  description = "VPC network name, for Cloud Run Direct VPC egress"
  value       = google_compute_network.vpc.name
}

output "vpc_egress_subnet_name" {
  description = "Subnet name for Cloud Run Direct VPC egress"
  value       = google_compute_subnetwork.cloud_run_egress.name
}

output "instance_name" {
  description = "Cloud SQL instance name"
  value       = google_sql_database_instance.gear_db.name
}

output "media_bucket_name" {
  description = "Name of the GCS bucket for media storage"
  value       = google_storage_bucket.media_bucket.name
}