# =============================================================================
# STORAGE MODULE VARIABLES
# =============================================================================

variable "instance_name" {
  description = "Name of the Cloud SQL instance"
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

variable "postgres_version" {
  description = "PostgreSQL version"
  type        = string
  default     = "POSTGRES_15"
}

variable "tier" {
  description = "Cloud SQL tier (e.g., db-f1-micro, db-n1-standard-1)"
  type        = string
  default     = "db-f1-micro"
}

variable "storage_size_gb" {
  description = "Storage size in GB"
  type        = number
  default     = 10
}

variable "backup_retention_days" {
  description = "Number of days to retain backups"
  type        = number
  default     = 7
}

variable "transaction_log_retention_days" {
  description = "Number of days to retain transaction logs for point-in-time recovery"
  type        = number
  default     = 7
}

variable "high_availability" {
  description = "Enable high availability (regional)"
  type        = bool
  default     = false
}

variable "deletion_protection" {
  description = "Enable deletion protection"
  type        = bool
  default     = true
}

variable "database_name" {
  description = "Name of the application database"
  type        = string
  default     = "gear_library"
}

variable "database_user" {
  description = "Database user name"
  type        = string
  default     = "gear_user"
}

variable "database_password" {
  description = "Database password"
  type        = string
  sensitive   = true
}

# =============================================================================
# MEDIA BUCKET VARIABLES
# =============================================================================

variable "media_bucket_name" {
  description = "Name of the GCS bucket for media storage"
  type        = string
}

variable "media_storage_class" {
  description = "Storage class for media bucket (STANDARD, NEARLINE, COLDLINE, ARCHIVE)"
  type        = string
  default     = "STANDARD"
}

variable "media_lifecycle_days" {
  description = "Days after which to delete old media (0 = never delete)"
  type        = number
  default     = 0
}

variable "media_versioning_enabled" {
  description = "Enable versioning for media bucket"
  type        = bool
  default     = false
}

variable "media_cors_origins" {
  description = "CORS allowed origins for media bucket"
  type        = list(string)
  default     = ["*"]
}