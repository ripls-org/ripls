# =============================================================================
# METABASE MODULE VARIABLES
# =============================================================================

variable "service_name" {
  description = "Name of the Metabase Cloud Run service"
  type        = string
}

variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCP region for deployment"
  type        = string
  default     = "us-central1"
}

variable "container_image" {
  description = "Metabase container image"
  type        = string
  default     = "metabase/metabase:latest"
}

# ---------------------------------------------------------------------------
# Application database (read-only connection for dashboards)
# ---------------------------------------------------------------------------

variable "database_host" {
  description = "Application database host (private IP)"
  type        = string
}

variable "database_port" {
  description = "Application database port"
  type        = number
  default     = 5432
}

variable "database_name" {
  description = "Application database name"
  type        = string
}

variable "metabase_reader_user" {
  description = "Read-only database user for Metabase queries"
  type        = string
  default     = "metabase_reader"
}

variable "metabase_reader_password" {
  description = "Password for the read-only Metabase database user"
  type        = string
  sensitive   = true
}

# ---------------------------------------------------------------------------
# Metabase metadata database (stores dashboards, saved questions, users)
# ---------------------------------------------------------------------------

variable "metabase_db_name" {
  description = "Database name for Metabase internal metadata"
  type        = string
  default     = "metabase"
}

variable "metabase_db_user" {
  description = "Database user for Metabase internal metadata"
  type        = string
  default     = "metabase_admin"
}

variable "metabase_db_password" {
  description = "Password for the Metabase metadata database user. Used to create the SQL user; the running container reads it from Secret Manager via var.metabase_db_password_secret_id."
  type        = string
  sensitive   = true
}

variable "metabase_db_password_secret_id" {
  description = "Secret Manager secret ID that holds the Metabase metadata DB password. Mounted into the container as MB_DB_PASS via secret_key_ref; rotation = SM version bump + revision rollover."
  type        = string
}

# ---------------------------------------------------------------------------
# Cloud SQL instance (shared with application)
# ---------------------------------------------------------------------------

variable "cloud_sql_instance_name" {
  description = "Name of the existing Cloud SQL instance to add the metadata database to"
  type        = string
}

# ---------------------------------------------------------------------------
# Networking
# ---------------------------------------------------------------------------

variable "vpc_network" {
  description = "VPC network name for Direct VPC egress (private database access)"
  type        = string
}

variable "vpc_egress_subnet" {
  description = "Subnet name for Direct VPC egress"
  type        = string
}

# ---------------------------------------------------------------------------
# Cloud Run sizing
# ---------------------------------------------------------------------------

variable "cpu_limit" {
  description = "CPU limit for Metabase container"
  type        = string
  default     = "1000m"
}

variable "memory_limit" {
  description = "Memory limit for Metabase container (JVM needs >=2Gi)"
  type        = string
  default     = "2Gi"
}

variable "max_instances" {
  description = "Maximum number of Metabase instances"
  type        = number
  default     = 1
}

# ---------------------------------------------------------------------------
# Access control
# ---------------------------------------------------------------------------

variable "allow_public_access" {
  description = "Allow public access to Metabase (required for custom domain; Metabase handles its own auth)"
  type        = bool
  default     = false
}

variable "allowed_members" {
  description = "IAM members allowed to access Metabase when public access is disabled"
  type        = list(string)
  default     = []
}
