# =============================================================================
# PRODUCTION ENVIRONMENT VARIABLES
# =============================================================================

variable "project_id" {
  description = "GCP project ID"
  type        = string
  default     = "ripls-prod"
}

variable "region" {
  description = "GCP region for deployment"
  type        = string
  default     = "us-central1"
}

variable "environment" {
  description = "Environment name (e.g., dev, staging, prod)"
  type        = string
  default     = "prod"
}

variable "cloudflare_zone_id" {
  description = "Cloudflare Zone ID for ripls.org"
  type        = string
  default     = "ca081c880ac3c16a2de85a70dc780207"
}

variable "cloudflare_app_zone_id" {
  description = "Cloudflare Zone ID for ripls.app (invite links and the web app)"
  type        = string
  default     = "7a22ea9cff9f85e4a27fc4f8be7e8ec3"
}

variable "billing_account_id" {
  description = "Billing account the monthly budget alert is attached to (covers all projects on the account)"
  type        = string
  default     = "01911A-DA95C0-B0C18F"
}
