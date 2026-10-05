# =============================================================================
# SERVER IDENTITY MODULE VARIABLES
# =============================================================================

variable "project_id" {
  description = "GCP project ID the service account and its grants live in"
  type        = string
}

variable "account_id" {
  description = "Service account ID (the part of the email before @)"
  type        = string
}

variable "display_name" {
  description = "Service account display name"
  type        = string
  default     = "Server runtime (self-hosted)"
}

variable "secret_ids" {
  description = "Secret Manager secret IDs the server reads at startup; each gets secretAccessor"
  type        = list(string)
}

variable "media_bucket" {
  description = "Name of the GCS bucket the server stores media in"
  type        = string
}
