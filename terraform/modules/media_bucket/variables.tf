# =============================================================================
# MEDIA BUCKET MODULE VARIABLES
# =============================================================================

variable "name" {
  description = "Name of the GCS bucket for media storage"
  type        = string
}

variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCP region for the bucket"
  type        = string
  default     = "us-central1"
}

variable "storage_class" {
  description = "Storage class for the bucket (STANDARD, NEARLINE, COLDLINE, ARCHIVE)"
  type        = string
  default     = "STANDARD"
}

variable "lifecycle_days" {
  description = "Days after which to delete old media (0 = never delete)"
  type        = number
  default     = 0
}

variable "versioning_enabled" {
  description = "Enable object versioning"
  type        = bool
  default     = false
}

variable "cors_origins" {
  description = "CORS allowed origins"
  type        = list(string)
  default     = ["*"]
}
