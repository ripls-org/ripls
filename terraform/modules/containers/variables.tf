# =============================================================================
# CONTAINER MODULE VARIABLES
# =============================================================================

variable "service_name" {
  description = "Name of the container service"
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
  description = "Container image URL (e.g., ghcr.io/owner/repo:tag)"
  type        = string
}

variable "container_port" {
  description = "Port the container listens on"
  type        = number
  default     = 8080
}

variable "database_host" {
  description = "Database host address"
  type        = string
}

variable "database_port" {
  description = "Database port"
  type        = number
  default     = 5432
}

variable "database_name" {
  description = "Database name"
  type        = string
}

variable "database_user" {
  description = "Database user"
  type        = string
}

variable "db_password_secret_id" {
  description = "Secret Manager secret ID that holds the application DB password. Mounted into the container as DB_PASSWORD via secret_key_ref; rotation = SM version bump + revision rollover."
  type        = string
}

variable "vpc_network" {
  description = "VPC network name for Direct VPC egress (private database access)"
  type        = string
}

variable "vpc_egress_subnet" {
  description = "Subnet name for Direct VPC egress"
  type        = string
}

variable "cpu_limit" {
  description = "CPU limit for container (e.g., '1000m' for 1 CPU)"
  type        = string
  default     = "1000m"
}

variable "memory_limit" {
  description = "Memory limit for container (e.g., '512Mi')"
  type        = string
  default     = "512Mi"
}

variable "min_instances" {
  description = "Minimum number of container instances"
  type        = number
  default     = 0
}

variable "cpu_idle" {
  description = <<-EOT
    Whether CPU is throttled between requests (request-based billing).
    false keeps CPU allocated at all times (instance-based billing), which
    always-on environments need for background work (realtime delivery,
    digests); true bills CPU only while requests are in flight and is the
    right choice for scale-to-zero environments (#2809).
  EOT
  type        = bool
  default     = false
}

variable "max_instances" {
  description = "Maximum number of container instances"
  type        = number
  default     = 10
}

variable "allow_public_access" {
  description = "Whether to allow public access to the service"
  type        = bool
  default     = true
}

variable "enable_dev_mode" {
  description = "Enable development mode (dev authentication, simulation clock, database reset)"
  type        = bool
  default     = false
}

variable "enable_dev_client" {
  description = "Enable serving the development web client"
  type        = bool
  default     = false
}

variable "log_source_location" {
  description = "Include source file:line in log entries (adds ~10-15% overhead)"
  type        = bool
  default     = false
}

variable "gcs_media_bucket" {
  description = "GCS bucket name for media storage"
  type        = string
}

variable "mailgun_domain" {
  description = "Mailgun domain for sending emails. Set per environment — the module carries no deployment identity (#2953)."
  type        = string
  default     = ""
}

variable "mailgun_from_address" {
  description = "From address for outgoing emails, e.g. \"Example <noreply@example.org>\". Set per environment."
  type        = string
  default     = ""
}

variable "mailgun_postal_address" {
  description = "CAN-SPAM physical postal address rendered in the off-app notification email footer (#2492). Non-secret — public in every email. No default, because a default belonging to one deployment would silently ship in another's footer; set it in the environment before flipping off_app_email_enabled."
  type        = string
  default     = ""
}

variable "off_app_email_enabled" {
  description = "Enable the off-app email notification channel (#2492): email a recipient with no active app device. Defaults to false; flip in the environment's main.tf only after the CAN-SPAM postal address and deliverability are signed off."
  type        = bool
  default     = false
}

variable "github_app_id" {
  description = "GitHub App ID for feedback bot"
  type        = number
  default     = 0
}

variable "github_installation_id" {
  description = "GitHub App Installation ID for the feedback repository"
  type        = number
  default     = 0
}

# The repo the feedback bot files issues against. No default: the server used
# to carry one deployment's owner as a compiled-in flag default, which is
# exactly the coupling #2953 removes. Each environment states its own.
variable "github_repo_owner" {
  description = "Owner (user or org) of the repository the feedback bot files issues against. Empty disables issue creation."
  type        = string
  default     = ""
}

variable "github_repo_name" {
  description = "Name of the repository the feedback bot files issues against. Empty disables issue creation."
  type        = string
  default     = ""
}

variable "waitlist_notify_email" {
  description = "Address that receives waitlist-signup notifications. Empty disables them."
  type        = string
  default     = ""
}

variable "google_client_id" {
  description = "Google OAuth Web Client ID for OIDC authentication"
  type        = string
  default     = ""
}

variable "invite_link_hostname" {
  description = "Hostname for invitation links — the environment's apex domain (e.g. example.app, or dev.example.app for a dev environment)"
  type        = string
}

# Server-side credentials previously plumbed as module variables
# (mapbox/mailgun/unsplash/pexels/pixabay/openai/anthropic API keys,
# JWT signing secret, GitHub App private key) are
# fetched at container startup from Secret Manager by /app/entrypoint.sh
# (#1768/#1770). They are no longer Terraform inputs.

variable "cors_allowed_origins" {
  description = "Comma-separated list of allowed CORS origins (e.g. https://example.app). Required in production; leave empty in dev mode to allow all origins."
  type        = string
  default     = ""
}

variable "map_provider" {
  description = "Active server-side location provider (#2188): 'mapbox' or 'google'. The selected provider's API key must also be available in Secret Manager (mapbox-access-token or google-maps-api-key-server)."
  type        = string
  default     = "mapbox"
  validation {
    condition     = contains(["mapbox", "google"], var.map_provider)
    error_message = "map_provider must be 'mapbox' or 'google'."
  }
}

variable "request_timeout" {
  description = "Maximum duration for a request in seconds (default: 300s, max: 3600s). Increase for streaming RPCs."
  type        = number
  default     = 900 # 15 minutes - allows longer streaming connections
}

variable "max_instance_request_concurrency" {
  description = "Maximum concurrent requests per instance. Set high for Go servers with streaming RPCs where idle connections (goroutines) should not trigger autoscaling. Invariant: max_instances * DB_MAX_OPEN_CONNS must be < database max_connections."
  type        = number
  default     = 250
}

# =============================================================================
# Activity digest (#1924)
# =============================================================================
# Two independent cadences (daily and weekly) share a recipient and a
# timezone. Both default to off so a fresh deploy is silent until ops
# opts in via the environment-level main.tf.

variable "platform_sms_enabled" {
  description = "Enable the platform-originated SMS notification channel (#2492). Defaults to false; flip in the environment's main.tf only after A2P 10DLC registration and counsel sign-off."
  type        = bool
  default     = false
}

variable "activity_digest_enabled" {
  description = "Enable the daily activity digest job (#1924). Defaults to false; flip in the environment's main.tf when ready."
  type        = bool
  default     = false
}

variable "activity_digest_weekly_enabled" {
  description = "Enable the weekly activity digest job (#1924). Defaults to false; independent of activity_digest_enabled."
  type        = bool
  default     = false
}

variable "activity_digest_notify_email" {
  description = "Recipient address for both daily and weekly activity digests. Set per environment alongside activity_digest_enabled."
  type        = string
  default     = ""
}

variable "activity_digest_timezone" {
  description = "IANA timezone defining 'yesterday' and 'last week' for the activity digest."
  type        = string
  default     = "America/Denver"
}

variable "activity_digest_send_hour" {
  description = "Local hour (0-23) the daily activity digest fires each day."
  type        = number
  default     = 8
}

variable "activity_digest_weekly_weekday" {
  description = "Weekday the weekly activity digest fires (English day name)."
  type        = string
  default     = "Monday"
}

variable "activity_digest_weekly_send_hour" {
  description = "Local hour (0-23) the weekly activity digest fires."
  type        = number
  default     = 8
}