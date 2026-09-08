# =============================================================================
# MONITORING MODULE
# =============================================================================
# This module creates Cloud Monitoring resources for the Ripls server:
# - Uptime check with OIDC authentication (calls /health endpoint)
# - Log-based metrics for per-RPC errors and health check failures
# - Alert policies for uptime failures and elevated error rates
# - Email notification channels

# =============================================================================
# REQUIRED APIS
# =============================================================================
resource "google_project_service" "monitoring" {
  project = var.project_id
  service = "monitoring.googleapis.com"

  disable_on_destroy = false
}

# =============================================================================
# SERVICE ACCOUNT FOR UPTIME CHECKS
# =============================================================================
# Uptime checks need a service account to authenticate to Cloud Run via OIDC.
resource "google_service_account" "uptime_checker" {
  project      = var.project_id
  account_id   = "uptime-checker-${var.environment}"
  display_name = "Uptime Check Service Account (${var.environment})"
  description  = "Service account used by Cloud Monitoring uptime checks to authenticate to Cloud Run"
}

# Grant the service account permission to invoke the Cloud Run service
resource "google_cloud_run_v2_service_iam_member" "uptime_checker_invoker" {
  project  = var.project_id
  location = var.region
  name     = var.service_name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.uptime_checker.email}"
}

# =============================================================================
# UPTIME CHECK
# =============================================================================
# Calls the /health endpoint every 5 minutes (by default) with OIDC auth.
# The check validates that:
# 1. The server is reachable
# 2. The health endpoint returns HTTP 200
# Gated by var.enable_uptime_check — dev runs without it (#2809).
resource "google_monitoring_uptime_check_config" "health_check" {
  count        = var.enable_uptime_check ? 1 : 0
  project      = var.project_id
  display_name = "Health Check - ${var.service_name}"
  timeout      = var.uptime_check_timeout
  period       = var.uptime_check_period

  http_check {
    path           = var.health_check_path
    port           = 443
    use_ssl        = true
    validate_ssl   = true
    request_method = "GET"

    accepted_response_status_codes {
      status_class = "STATUS_CLASS_2XX"
    }
  }

  monitored_resource {
    type = "uptime_url"
    labels = {
      project_id = var.project_id
      host       = replace(var.service_url, "https://", "")
    }
  }

  # Use OIDC authentication to invoke Cloud Run
  checker_type = "STATIC_IP_CHECKERS"

  selected_regions = var.uptime_check_regions

  depends_on = [google_project_service.monitoring]
}

# =============================================================================
# LOG-BASED METRICS
# =============================================================================

# Metric for health check failures (when /health returns unhealthy)
resource "google_logging_metric" "health_check_failures" {
  project     = var.project_id
  name        = "health_check_failures_${var.environment}"
  description = "Count of health check completions where the server reported unhealthy"

  filter = <<-EOT
    resource.type="cloud_run_revision"
    resource.labels.service_name="${var.service_name}"
    jsonPayload.message="health_check_complete"
    jsonPayload.healthy=false
  EOT

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"
  }
}

# Metric for per-dependency health check failures with dependency and backend labels
# Used for state-change alerting per backend (e.g., "anthropic" not just "ai")
resource "google_logging_metric" "dependency_health_failures" {
  project     = var.project_id
  name        = "dependency_health_failures_${var.environment}"
  description = "Count of dependency health check failures by dependency name and backend"

  filter = <<-EOT
    resource.type="cloud_run_revision"
    resource.labels.service_name="${var.service_name}"
    jsonPayload.message="dependency_health_check_failed"
  EOT

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"

    labels {
      key         = "dependency"
      value_type  = "STRING"
      description = "The dependency category (e.g., ai, storage, email)"
    }

    labels {
      key         = "backend"
      value_type  = "STRING"
      description = "The specific backend that failed (e.g., anthropic, gemini, postgresql)"
    }
  }

  label_extractors = {
    "dependency" = "EXTRACT(jsonPayload.dependency)"
    "backend"    = "EXTRACT(jsonPayload.backend)"
  }
}

# Metric for ALL errors (severity ERROR or higher)
# This captures every error log, not just RPC-specific ones
resource "google_logging_metric" "all_errors" {
  count = var.enable_all_errors_alerting ? 1 : 0

  project     = var.project_id
  name        = "all_errors_${var.environment}"
  description = "Count of all error-level and higher log entries"

  filter = <<-EOT
    resource.type="cloud_run_revision"
    resource.labels.service_name="${var.service_name}"
    severity>="ERROR"
  EOT

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"
  }
}

# The legacy count-based rpc_errors metric + "RPC Errors Elevated" alert
# (>5 ERROR logs per operation in 5m) were retired in #2623 after the #1613
# overlap soak: the fraction-based rpc_error_rate alert (alerts.tf) covers
# failing responses, and all_errors below already fires on any single ERROR
# log — the legacy alert's only marginal value was a per-operation label on
# the notification.

# Metric for container-level crashes (issue #2490).
# Every metric above reads the *application* log stream (cloud_run_revision +
# jsonPayload). A process that dies hard — OOM kill, non-zero exit from a
# startup os.Exit(1), or an unrecovered panic (Go exits with code 2) — writes
# NO application error log, so none of those metrics fire. Crash ground truth
# lives in the Cloud Run *system* log stream (run.googleapis.com/varlog/system),
# which records container exits and OOM kills. Graceful SIGTERM shutdowns
# (deploys, min-instance recycles) exit 0, so we match only non-zero exits, OOM,
# and signal kills — routine churn does NOT count.
#
# NOTE: these Cloud Run system-message strings are the documented/known forms.
# We have no captured crash sample to template against, so on the first real
# crash confirm they match the alert and tighten if needed (#2490).
resource "google_logging_metric" "container_crashes" {
  project     = var.project_id
  name        = "container_crashes_${var.environment}"
  description = "Count of Cloud Run container crashes — non-zero exit, OOM kill, or signal termination (excludes graceful exit(0) shutdowns)"

  filter = <<-EOT
    resource.type="cloud_run_revision"
    resource.labels.service_name="${var.service_name}"
    logName="projects/${var.project_id}/logs/run.googleapis.com%2Fvarlog%2Fsystem"
    (
      textPayload=~"Container called exit\([1-9][0-9]*\)"
      OR textPayload:"Memory limit of"
      OR textPayload:"Container terminated on signal"
    )
  EOT

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"
  }
}

# Metric for serving-loop failures (issue #2490).
# Complements container_crashes: the HTTP serving goroutine can panic and be
# recovered (server/main.go:1762) — the container stays UP but stops serving, so
# the system stream shows no exit. These specific server-emitted messages catch
# that. They deliberately EXCLUDE the request-scoped "panic recovered" log from
# middleware.PanicRecovery (server/middleware/recovery.go), which does not crash
# anything.
resource "google_logging_metric" "serving_failures" {
  project     = var.project_id
  name        = "serving_failures_${var.environment}"
  description = "Count of server serving-loop failures — HTTP goroutine panic, fatal serve error, or listener bind failure (excludes recovered request panics)"

  filter = <<-EOT
    resource.type="cloud_run_revision"
    resource.labels.service_name="${var.service_name}"
    severity>="ERROR"
    (
      jsonPayload.message="panic in HTTP server goroutine"
      OR jsonPayload.message="server error"
      OR jsonPayload.message="server failed to bind listener"
    )
  EOT

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"
  }
}

# =============================================================================
# NOTIFICATION CHANNELS
# =============================================================================

# Create email notification channels for each recipient
resource "google_monitoring_notification_channel" "email" {
  for_each = toset(var.alert_email_addresses)

  project      = var.project_id
  display_name = "Email: ${each.value}"
  type         = "email"

  labels = {
    email_address = each.value
  }

  depends_on = [google_project_service.monitoring]
}

# =============================================================================
# PUB/SUB CHANNEL FOR GITHUB ISSUE AUTO-FILING (Phase 2 of #1148+#1149)
# =============================================================================
# Cloud Monitoring publishes alert incident JSON to this topic. The
# ripls-alerts Cloud Functions codebase subscribes via firebase-functions
# v2/pubsub.onMessagePublished and upserts a GitHub issue per incident.
# Email channels above stay attached in parallel — if the Cloud Function
# breaks, prod incidents still surface in inboxes.
#
# Gated by var.enable_github_pubsub_channel so dev/staging stay quiet
# until Phase 4 extends coverage.

resource "google_pubsub_topic" "github_alerts" {
  count = var.enable_github_pubsub_channel ? 1 : 0

  project = var.project_id
  name    = "gcp-alerts-to-github"

  depends_on = [google_project_service.monitoring]
}

resource "google_monitoring_notification_channel" "pubsub_github" {
  count = var.enable_github_pubsub_channel ? 1 : 0

  project      = var.project_id
  display_name = "Pub/Sub: gcp-alerts-to-github (ripls-alerts Cloud Function)"
  type         = "pubsub"

  labels = {
    topic = google_pubsub_topic.github_alerts[0].id
  }

  depends_on = [google_project_service.monitoring]
}

# Cloud Monitoring's service identity needs publisher rights on the topic.
# The exact identity is service-<project-number>@gcp-sa-monitoring-notification.iam.gserviceaccount.com.
resource "google_pubsub_topic_iam_member" "monitoring_publisher" {
  count = var.enable_github_pubsub_channel ? 1 : 0

  project = var.project_id
  topic   = google_pubsub_topic.github_alerts[0].id
  role    = "roles/pubsub.publisher"
  member  = "serviceAccount:service-${data.google_project.this.number}@gcp-sa-monitoring-notification.iam.gserviceaccount.com"
}

# Lookup project metadata once for the IAM grant above.
data "google_project" "this" {
  project_id = var.project_id
}

# Helper: the list of notification channel IDs each alert policy attaches to.
# Computed once so every policy uses the same expression.
locals {
  notification_channel_ids = concat(
    [for ch in google_monitoring_notification_channel.email : ch.id],
    var.enable_github_pubsub_channel ? [google_monitoring_notification_channel.pubsub_github[0].id] : [],
  )
}

# =============================================================================
# ALERT POLICIES
# =============================================================================

# Alert when uptime check fails
resource "google_monitoring_alert_policy" "uptime_failure" {
  count        = var.enable_uptime_check ? 1 : 0
  project      = var.project_id
  display_name = "Uptime Check Failed - ${var.service_name}"
  combiner     = "OR"
  severity     = "CRITICAL"

  conditions {
    display_name = "Uptime check failure"

    condition_threshold {
      filter          = "metric.type=\"monitoring.googleapis.com/uptime_check/check_passed\" AND resource.type=\"uptime_url\" AND metric.labels.check_id=\"${google_monitoring_uptime_check_config.health_check[0].uptime_check_id}\""
      comparison      = "COMPARISON_LT"
      threshold_value = 1
      duration        = "0s"

      aggregations {
        alignment_period   = var.uptime_check_period
        per_series_aligner = "ALIGN_FRACTION_TRUE"
      }

      trigger {
        count = var.uptime_failure_threshold
      }
    }
  }

  notification_channels = local.notification_channel_ids

  documentation {
    content   = <<-EOT
      The uptime check for ${var.service_name} has failed.

      **Service:** ${var.service_name}
      **Environment:** ${var.environment}
      **Endpoint:** ${var.service_url}${var.health_check_path}

      **Troubleshooting:**
      1. Check Cloud Run logs: https://console.cloud.google.com/run/detail/${var.region}/${var.service_name}/logs?project=${var.project_id}
      2. Verify the service is running: https://console.cloud.google.com/run/detail/${var.region}/${var.service_name}/revisions?project=${var.project_id}
      3. Check health endpoint directly: curl ${var.service_url}${var.health_check_path}
    EOT
    mime_type = "text/markdown"
  }

  alert_strategy {
    auto_close = "604800s" # 7 days
  }

  depends_on = [google_monitoring_uptime_check_config.health_check]
}

# Alert when a dependency health check fails (includes backend name in notification)
# Uses threshold-based alerting for state-change notifications per backend
# Groups by both dependency (category) and backend (specific implementation)
# Requires multiple consecutive failures (default 2) to avoid alerting on transient issues
resource "google_monitoring_alert_policy" "health_check_unhealthy" {
  project      = var.project_id
  display_name = "Health Check Unhealthy - ${var.service_name}"
  combiner     = "OR"
  severity     = "ERROR"

  conditions {
    display_name = "Dependency health check failed ${var.dependency_failure_threshold} consecutive times"

    condition_threshold {
      filter     = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.dependency_health_failures.name}\" AND resource.type=\"cloud_run_revision\""
      comparison = "COMPARISON_GT"
      # Alert when failures exceed threshold - 1 (i.e., at least N failures)
      threshold_value = var.dependency_failure_threshold - 1
      duration        = "0s"

      aggregations {
        # Alignment period covers N check windows to catch consecutive failures
        # Default: 2 failures * 300s = 600s (10 minutes)
        alignment_period     = "${var.dependency_failure_threshold * 300}s"
        per_series_aligner   = "ALIGN_SUM"
        cross_series_reducer = "REDUCE_SUM"
        # Group by backend for specific alerts (e.g., "anthropic" not just "ai")
        group_by_fields = ["metric.label.dependency", "metric.label.backend"]
      }

      trigger {
        count = 1
      }
    }
  }

  notification_channels = local.notification_channel_ids

  documentation {
    content   = <<-EOT
      A dependency health check has failed ${var.dependency_failure_threshold} consecutive times for ${var.service_name}.

      **Service:** ${var.service_name}
      **Environment:** ${var.environment}
      **Endpoint:** ${var.service_url}${var.health_check_path}
      **Threshold:** ${var.dependency_failure_threshold} consecutive failures (over ${var.dependency_failure_threshold * 5} minutes)

      **Failing Backend:** Check the `dependency` and `backend` labels in the alert details.
      The `dependency` label shows the category (e.g., "ai", "storage", "email").
      The `backend` label shows the specific service (e.g., "anthropic", "gemini", "postgresql").

      **Troubleshooting:**
      1. Check health endpoint for current status: curl ${var.service_url}${var.health_check_path} | jq
      2. View dependency failure logs: https://console.cloud.google.com/logs/query;query=resource.type%3D%22cloud_run_revision%22%0Aresource.labels.service_name%3D%22${var.service_name}%22%0AjsonPayload.message%3D%22dependency_health_check_failed%22?project=${var.project_id}
      3. Review the specific backend's status and configuration
    EOT
    mime_type = "text/markdown"
  }

  alert_strategy {
    # Auto-close after 1 hour of no failures (indicates recovery)
    auto_close = "3600s"
  }

  depends_on = [google_logging_metric.dependency_health_failures]
}

# Alert on EVERY error log (aggressive alerting)
# Uses condition_matched_log to include error details in notifications
# Excludes:
# - Health check errors (covered by dedicated alert)
# - Middleware "http request" logs (just status codes, no context)
# - Cloud Run infrastructure logs (no jsonPayload)
# This ensures we only alert once per failed request - on the service-level error
# that contains useful context (gear_id, user_id, actual error message, etc.)
resource "google_monitoring_alert_policy" "all_errors" {
  count = var.enable_all_errors_alerting ? 1 : 0

  project      = var.project_id
  display_name = "Server Error Logged - ${var.service_name}"
  combiner     = "OR"
  severity     = "ERROR"

  conditions {
    display_name = "Service-level error logged (excluding middleware and infrastructure)"

    condition_matched_log {
      filter = <<-EOT
        resource.type="cloud_run_revision"
        resource.labels.service_name="${var.service_name}"
        severity>="ERROR"
        jsonPayload:*
        NOT jsonPayload.message="dependency_health_check_failed"
        NOT jsonPayload.message="http request"
      EOT

      label_extractors = {
        "error_message" = "EXTRACT(jsonPayload.error)"
        "message"       = "EXTRACT(jsonPayload.message)"
        "operation"     = "EXTRACT(jsonPayload.operation)"
        "request_id"    = "EXTRACT(jsonPayload.request_id)"
        "user_id"       = "EXTRACT(jsonPayload.user_id)"
      }
    }
  }

  notification_channels = local.notification_channel_ids

  documentation {
    content   = <<-EOT
      An error was logged in ${var.service_name}.

      **Service:** ${var.service_name}
      **Environment:** ${var.environment}

      **Investigate:**
      1. View recent error logs: https://console.cloud.google.com/logs/query;query=resource.type%3D%22cloud_run_revision%22%0Aresource.labels.service_name%3D%22${var.service_name}%22%0Aseverity%3E%3DERROR;timeRange=PT1H?project=${var.project_id}
      2. Check request_id in logs for full trace
      3. Look for patterns: repeated errors, specific operations, or user actions
    EOT
    mime_type = "text/markdown"
  }

  alert_strategy {
    auto_close = "86400s" # 1 day (shorter since these are frequent)

    notification_rate_limit {
      period = "300s" # Rate limit: max 1 notification per 5 minutes to avoid spam
    }
  }
}

# =============================================================================
# CRASH / PROCESS-EXIT ALERTS (issue #2490)
# =============================================================================
# These answer the question the uptime check can't: not "is it reachable?" but
# "did the binary crash, and why?". They are CRITICAL, always on (a crash is
# never routine), and fire from the crash signal directly — so a real crash is
# never masked by the deploy-drain uptime noise that the uptime check produces
# during ordinary instance recycles.

# Alert on an actual container crash — non-zero exit, OOM kill, or signal kill.
# A graceful SIGTERM shutdown exits 0 and is excluded by the metric filter, so
# deploys and min-instance recycles do NOT page here.
resource "google_monitoring_alert_policy" "container_crash" {
  project      = var.project_id
  display_name = "Server Process Crash - ${var.service_name}"
  combiner     = "OR"
  severity     = "CRITICAL"

  conditions {
    display_name = "Container exited non-zero, was OOM-killed, or terminated on a signal"

    condition_matched_log {
      filter = <<-EOT
        resource.type="cloud_run_revision"
        resource.labels.service_name="${var.service_name}"
        logName="projects/${var.project_id}/logs/run.googleapis.com%2Fvarlog%2Fsystem"
        (
          textPayload=~"Container called exit\([1-9][0-9]*\)"
          OR textPayload:"Memory limit of"
          OR textPayload:"Container terminated on signal"
        )
      EOT
    }
  }

  notification_channels = local.notification_channel_ids

  documentation {
    content   = <<-EOT
      The ${var.service_name} container crashed — it exited non-zero, was
      OOM-killed, or was terminated on a signal. This is the *cause* behind any
      concurrent "Uptime Check Failed" alert. A graceful SIGTERM shutdown
      (deploy or instance recycle) exits 0 and does NOT trigger this alert, so
      this is a real crash.

      **Service:** ${var.service_name}
      **Environment:** ${var.environment}

      A repeated / re-firing alert here means a **crash loop**, not a one-off.

      **Triage:**
      1. Read the crash and surrounding system logs — note the exit code or OOM line: https://console.cloud.google.com/logs/query;query=resource.type%3D%22cloud_run_revision%22%0Aresource.labels.service_name%3D%22${var.service_name}%22%0AlogName%3D%22projects%2F${var.project_id}%2Flogs%2Frun.googleapis.com%252Fvarlog%252Fsystem%22?project=${var.project_id}
      2. "Memory limit of ... exceeded" → memory pressure / OOM; raise the Cloud Run memory limit or fix the leak (see #1478).
      3. exit(1) → fatal startup error (the server os.Exit(1)s on failed secret/DB init); check the app error logs around the crash time.
      4. exit(2) → an unrecovered Go panic; find the stack trace in the app logs.
      5. Revisions / instance history: https://console.cloud.google.com/run/detail/${var.region}/${var.service_name}/revisions?project=${var.project_id}
    EOT
    mime_type = "text/markdown"
  }

  alert_strategy {
    # Auto-close one hour after crashes stop. Rate-limit so a tight crash loop
    # sends at most one notification per 5 min rather than one per restart.
    # 300s is the platform minimum for log-based (condition_matched_log) policies.
    auto_close = "3600s"

    notification_rate_limit {
      period = "300s"
    }
  }
}

# Alert when the HTTP serving loop fails but the process stays up. The serving
# goroutine recovers a panic and logs it (server/main.go), so the container
# does NOT exit and the system-stream alert above won't fire — only the uptime
# check would, with no context. This catches that gap with the actual log line.
resource "google_monitoring_alert_policy" "serving_failure" {
  project      = var.project_id
  display_name = "Server Serving-Loop Failure - ${var.service_name}"
  combiner     = "OR"
  severity     = "CRITICAL"

  conditions {
    display_name = "HTTP serving goroutine panicked, serve errored, or listener bind failed"

    condition_matched_log {
      filter = <<-EOT
        resource.type="cloud_run_revision"
        resource.labels.service_name="${var.service_name}"
        severity>="ERROR"
        (
          jsonPayload.message="panic in HTTP server goroutine"
          OR jsonPayload.message="server error"
          OR jsonPayload.message="server failed to bind listener"
        )
      EOT

      label_extractors = {
        "message" = "EXTRACT(jsonPayload.message)"
        "error"   = "EXTRACT(jsonPayload.error)"
      }
    }
  }

  notification_channels = local.notification_channel_ids

  documentation {
    content   = <<-EOT
      The ${var.service_name} HTTP serving loop failed: the serving goroutine
      panicked, server.Serve returned a fatal error, or the listener failed to
      bind. The process may still be *running* but no longer serving traffic, so
      this can present as an uptime failure with no crash in the system logs.

      **Service:** ${var.service_name}
      **Environment:** ${var.environment}

      **Triage:**
      1. Find the panic / stack trace or serve error: https://console.cloud.google.com/logs/query;query=resource.type%3D%22cloud_run_revision%22%0Aresource.labels.service_name%3D%22${var.service_name}%22%0AjsonPayload.message%3D%22panic%20in%20HTTP%20server%20goroutine%22?project=${var.project_id}
      2. "server failed to bind listener" → port already in use / startup race; the process exits non-zero (also surfaces as Server Process Crash).
      3. If an instance is wedged (alive but not serving), deploy a new revision to force a clean restart.
    EOT
    mime_type = "text/markdown"
  }

  alert_strategy {
    auto_close = "3600s"

    notification_rate_limit {
      period = "300s" # platform minimum for log-based policies
    }
  }
}

# =============================================================================
# CLOUD SQL DATABASE MONITORING
# =============================================================================
# These alerts monitor the Cloud SQL instance directly using GCP metrics.
# They're only created if database_instance_id is provided.

locals {
  enable_database_monitoring = var.database_instance_id != ""
  connection_threshold       = floor(var.database_max_connections * var.database_connection_threshold_percent / 100)
}

# Alert when database connections approach the limit
resource "google_monitoring_alert_policy" "database_connections" {
  count = local.enable_database_monitoring ? 1 : 0

  project      = var.project_id
  display_name = "Database Connections High - ${var.database_instance_id}"
  combiner     = "OR"
  severity     = "WARNING"

  conditions {
    display_name = "PostgreSQL connections exceed ${var.database_connection_threshold_percent}% of max"

    condition_threshold {
      filter          = "metric.type=\"cloudsql.googleapis.com/database/postgresql/num_backends\" AND resource.type=\"cloudsql_database\" AND resource.labels.database_id=\"${var.project_id}:${var.database_instance_id}\""
      comparison      = "COMPARISON_GT"
      threshold_value = local.connection_threshold
      duration        = "60s"

      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_MEAN"
      }

      trigger {
        count = 1
      }
    }
  }

  notification_channels = local.notification_channel_ids

  documentation {
    content   = <<-EOT
      Database connections are high for ${var.database_instance_id}.

      **Instance:** ${var.database_instance_id}
      **Environment:** ${var.environment}
      **Threshold:** ${local.connection_threshold} connections (${var.database_connection_threshold_percent}% of ${var.database_max_connections} max)

      **Possible causes:**
      - Traffic spike causing more concurrent database requests
      - Connection leak in application code
      - Long-running queries holding connections
      - Multiple Cloud Run instances each opening connections

      **Troubleshooting:**
      1. Check health endpoint for current pool stats: curl ${var.service_url}/health | jq '.dependencies[] | select(.name=="database")'
      2. View Cloud SQL metrics: https://console.cloud.google.com/sql/instances/${var.database_instance_id}/monitoring?project=${var.project_id}
      3. Consider adding connection pool limits in the application
      4. Consider upgrading to a larger Cloud SQL tier if this persists
    EOT
    mime_type = "text/markdown"
  }

  alert_strategy {
    auto_close = "3600s" # Auto-close after 1 hour
  }
}

# Alert when database CPU is high
resource "google_monitoring_alert_policy" "database_cpu" {
  count = local.enable_database_monitoring ? 1 : 0

  project      = var.project_id
  display_name = "Database CPU High - ${var.database_instance_id}"
  combiner     = "OR"
  severity     = "WARNING"

  conditions {
    display_name = "CPU utilization exceeds ${var.database_cpu_threshold_percent}%"

    condition_threshold {
      filter          = "metric.type=\"cloudsql.googleapis.com/database/cpu/utilization\" AND resource.type=\"cloudsql_database\" AND resource.labels.database_id=\"${var.project_id}:${var.database_instance_id}\""
      comparison      = "COMPARISON_GT"
      threshold_value = var.database_cpu_threshold_percent / 100
      duration        = "300s" # 5 minutes sustained

      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_MEAN"
      }

      trigger {
        count = 1
      }
    }
  }

  notification_channels = local.notification_channel_ids

  documentation {
    content   = <<-EOT
      Database CPU utilization is high for ${var.database_instance_id}.

      **Instance:** ${var.database_instance_id}
      **Environment:** ${var.environment}
      **Threshold:** ${var.database_cpu_threshold_percent}% sustained for 5 minutes

      **Possible causes:**
      - Heavy query load
      - Inefficient queries (missing indexes, full table scans)
      - Background maintenance (vacuum, analyze)

      **Troubleshooting:**
      1. Check Query Insights: https://console.cloud.google.com/sql/instances/${var.database_instance_id}/query-insights?project=${var.project_id}
      2. Look for slow queries in Cloud SQL logs
      3. Consider upgrading to a larger Cloud SQL tier
    EOT
    mime_type = "text/markdown"
  }

  alert_strategy {
    auto_close = "3600s"
  }
}

# Alert when database memory is high
# Uses MQL to calculate total_usage / quota ratio dynamically, rather than
# the broken utilization metric which always shows 100% on small instances.
resource "google_monitoring_alert_policy" "database_memory" {
  count = local.enable_database_monitoring ? 1 : 0

  project      = var.project_id
  display_name = "Database Memory High - ${var.database_instance_id}"
  combiner     = "OR"
  severity     = "WARNING"

  conditions {
    display_name = "Memory usage exceeds ${var.database_memory_threshold_percent}% of quota"

    condition_prometheus_query_language {
      query    = "sum(cloudsql_googleapis_com:database_memory_total_usage{database_id=\"${var.project_id}:${var.database_instance_id}\"}) / sum(cloudsql_googleapis_com:database_memory_quota{database_id=\"${var.project_id}:${var.database_instance_id}\"}) > ${var.database_memory_threshold_percent / 100}"
      duration = "300s"
    }
  }

  notification_channels = local.notification_channel_ids

  documentation {
    content   = <<-EOT
      Database memory usage is high for ${var.database_instance_id}.

      **Instance:** ${var.database_instance_id}
      **Environment:** ${var.environment}
      **Threshold:** ${var.database_memory_threshold_percent}% of memory quota

      **Possible causes:**
      - Large result sets being cached
      - Many concurrent connections
      - Memory-intensive queries

      **Troubleshooting:**
      1. View memory metrics: https://console.cloud.google.com/sql/instances/${var.database_instance_id}/monitoring?project=${var.project_id}
      2. Check for queries with large result sets
      3. Consider upgrading to a larger Cloud SQL tier
    EOT
    mime_type = "text/markdown"
  }

  alert_strategy {
    auto_close = "3600s"
  }
}

# Alert when database disk is filling up
resource "google_monitoring_alert_policy" "database_disk" {
  count = local.enable_database_monitoring ? 1 : 0

  project      = var.project_id
  display_name = "Database Disk High - ${var.database_instance_id}"
  combiner     = "OR"
  severity     = "WARNING"

  conditions {
    display_name = "Disk utilization exceeds ${var.database_disk_threshold_percent}%"

    condition_threshold {
      filter          = "metric.type=\"cloudsql.googleapis.com/database/disk/utilization\" AND resource.type=\"cloudsql_database\" AND resource.labels.database_id=\"${var.project_id}:${var.database_instance_id}\""
      comparison      = "COMPARISON_GT"
      threshold_value = var.database_disk_threshold_percent / 100
      duration        = "300s"

      aggregations {
        alignment_period   = "60s"
        per_series_aligner = "ALIGN_MEAN"
      }

      trigger {
        count = 1
      }
    }
  }

  notification_channels = local.notification_channel_ids

  documentation {
    content   = <<-EOT
      Database disk utilization is high for ${var.database_instance_id}.

      **Instance:** ${var.database_instance_id}
      **Environment:** ${var.environment}
      **Threshold:** ${var.database_disk_threshold_percent}%

      **Action required:**
      - Cloud SQL has auto-resize enabled, but monitor for unexpected growth
      - Review data retention policies
      - Check for bloated tables that need VACUUM

      **Troubleshooting:**
      1. View disk metrics: https://console.cloud.google.com/sql/instances/${var.database_instance_id}/monitoring?project=${var.project_id}
      2. Check table sizes in the database
      3. Run VACUUM ANALYZE on large tables if needed
    EOT
    mime_type = "text/markdown"
  }

  alert_strategy {
    auto_close = "3600s"
  }
}
