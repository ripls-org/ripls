# =============================================================================
# MONITORING MODULE VARIABLES
# =============================================================================

variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCP region for resources"
  type        = string
  default     = "us-central1"
}

variable "environment" {
  description = "Environment name (e.g., dev, staging, prod)"
  type        = string
}

# =============================================================================
# SERVICE CONFIGURATION
# =============================================================================

variable "service_name" {
  description = "Name of the Cloud Run service to monitor"
  type        = string
}

variable "service_url" {
  description = "URL of the Cloud Run service (e.g., https://server-abc123.run.app)"
  type        = string
}

variable "health_check_path" {
  description = "Path to the health check endpoint"
  type        = string
  default     = "/health"
}

# =============================================================================
# UPTIME CHECK CONFIGURATION
# =============================================================================

variable "enable_uptime_check" {
  description = <<-EOT
    Whether to create the external uptime check (and its failure alert).
    Disabled in dev: continuous probing would keep the scale-to-zero dev
    service warm 24/7 and re-trigger the paid dependency probes the uptime
    path used to drive (#2809). Log-based alerting is unaffected.
  EOT
  type        = bool
  default     = true
}

variable "uptime_check_period" {
  description = "How often to run the uptime check (e.g., '300s' for every 5 minutes)"
  type        = string
  default     = "300s"
}

variable "uptime_check_timeout" {
  description = "Timeout for each uptime check request"
  type        = string
  default     = "30s"
}

variable "uptime_check_regions" {
  description = "Regions to run uptime checks from"
  type        = list(string)
  default     = ["USA"]
}

# =============================================================================
# ALERTING CONFIGURATION
# =============================================================================

variable "alert_email_addresses" {
  description = "Email addresses to receive alerts"
  type        = list(string)
}

variable "uptime_failure_threshold" {
  description = "Number of consecutive uptime check failures before alerting"
  type        = number
  default     = 1
}

variable "dependency_failure_threshold" {
  description = "Number of consecutive health check failures required before alerting for a dependency"
  type        = number
  default     = 2
}

variable "enable_all_errors_alerting" {
  description = "Enable aggressive alerting for every error-level log. Sends an email for each error."
  type        = bool
  default     = true
}

# =============================================================================
# SLI/SLO ALERTING CONFIGURATION (issue #1613)
# =============================================================================

variable "rpc_error_rate_threshold" {
  description = "Fraction of 5xx responses per RPC method (0..1) that triggers the error-rate alert"
  type        = number
  default     = 0.05
}

variable "rpc_error_rate_min_errors_per_second" {
  description = "Minimum 5xx rate (errors/sec) required before the error-rate fraction can alert; suppresses noise on very-low-traffic RPCs where one error is a large fraction"
  type        = number
  default     = 0.016 # ~1 error/minute
}

variable "rpc_p95_latency_threshold_ms" {
  description = "Per-RPC P95 latency in milliseconds that triggers the latency alert for interactive (non-slow-class, non-streaming) RPCs; also the latency SLO's good-request cut (#2622 calibration)"
  type        = number
  default     = 2000
}

variable "rpc_p95_slow_class_regex" {
  description = "RE2 full-match regex of rpc_method values in the slow class — AI generation, media upload, synchronous third-party calls. These alert at rpc_p95_slow_latency_threshold_ms instead of rpc_p95_latency_threshold_ms and are excluded from the latency SLO (#2622)"
  type        = string
  default     = "GearService/GenGear|MediaService/AddMedia|MediaService/AddMediaFromURL|FeedbackService/SubmitFeedback"
}

variable "rpc_p95_slow_latency_threshold_ms" {
  description = "Per-RPC P95 latency in milliseconds that triggers the latency alert for slow-class RPCs (see rpc_p95_slow_class_regex)"
  type        = number
  default     = 8000
}

variable "slo_availability_target" {
  description = "Availability SLO goal: fraction of RPC responses that are non-5xx over the 28-day rolling window (#2624)"
  type        = number
  default     = 0.995
}

variable "slo_latency_target" {
  description = "Latency SLO goal: fraction of non-streaming, non-slow-class RPC responses under rpc_p95_latency_threshold_ms over the 28-day rolling window (#2624)"
  type        = number
  default     = 0.99
}

variable "db_pool_saturation_threshold" {
  description = "DB pool utilization (in_use/open, 0..1) that triggers the pool-saturation alert"
  type        = number
  default     = 0.9
}

variable "enable_github_pubsub_channel" {
  description = <<-EOT
    Whether to provision a Pub/Sub notification channel for the
    ripls-alerts Cloud Function (Phase 2 of #1148+#1149). When true,
    creates the gcp-alerts-to-github topic and a pubsub-typed
    google_monitoring_notification_channel, and attaches that channel
    to every alert policy alongside the email channel. When false,
    nothing changes in alert routing — only email fires.
    Default false to keep dev/staging environments quiet; opt in
    per-environment in terraform/environments/<env>/main.tf.
  EOT
  type        = bool
  default     = false
}

# =============================================================================
# DATABASE MONITORING CONFIGURATION
# =============================================================================

variable "database_instance_id" {
  description = "Cloud SQL instance ID for database monitoring (e.g., 'gear-db-dev'). Leave empty to disable database monitoring."
  type        = string
  default     = ""
}

variable "database_max_connections" {
  description = "Maximum connections for the database instance. Used to calculate connection alert threshold."
  type        = number
  default     = 25 # db-f1-micro default
}

variable "database_connection_threshold_percent" {
  description = "Alert when connections exceed this percentage of max_connections"
  type        = number
  default     = 80
}

variable "database_cpu_threshold_percent" {
  description = "Alert when CPU utilization exceeds this percentage"
  type        = number
  default     = 80
}

variable "database_memory_threshold_percent" {
  description = "Alert when memory utilization exceeds this percentage"
  type        = number
  default     = 80
}

variable "database_disk_threshold_percent" {
  description = "Alert when disk utilization exceeds this percentage"
  type        = number
  default     = 80
}
variable "sms_delivery_failure_rate_threshold" {
  description = "Fraction of terminal SMS outcomes (0..1) that may be failed/undelivered before the delivery alert fires"
  type        = number
  default     = 0.10
}

variable "sms_delivery_failure_min_count" {
  description = "Minimum SMS failures within the 30-minute alignment window before the failure fraction can alert; SMS volume is low enough that one failure would otherwise be 100% of a quiet window"
  type        = number
  default     = 1 # COMPARISON_GT, so a lone failure cannot fire; two can
}

variable "email_code_p95_latency_threshold_ms" {
  description = "P95 accepted→delivered latency (ms) for sign-in-code email above which the delivery alert fires"
  type        = number
  default     = 10000 # 10s — first pass; typical observed delivery is under 1s
}

variable "email_code_delivered_min_count" {
  description = "Minimum delivered sign-in-code emails within the 10-minute alignment window before the P95 latency condition can alert; at current single-digit-per-week volume a lone slow delivery would otherwise be 100% of a quiet window (#2923)"
  type        = number
  default     = 5 # COMPARISON_GT, so ≥6 deliveries per 10 min required; above current peak volume
}
