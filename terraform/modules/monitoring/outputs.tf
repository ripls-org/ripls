# =============================================================================
# MONITORING MODULE OUTPUTS
# =============================================================================

output "uptime_check_id" {
  description = "ID of the uptime check (null when enable_uptime_check is false)"
  value       = one(google_monitoring_uptime_check_config.health_check[*].uptime_check_id)
}

output "uptime_check_name" {
  description = "Name of the uptime check (null when enable_uptime_check is false)"
  value       = one(google_monitoring_uptime_check_config.health_check[*].name)
}

output "uptime_checker_service_account" {
  description = "Email of the service account used for uptime checks"
  value       = google_service_account.uptime_checker.email
}

output "alert_policy_ids" {
  description = "Map of alert policy names to their IDs"
  value = {
    uptime_failure         = one(google_monitoring_alert_policy.uptime_failure[*].name)
    health_check_unhealthy = google_monitoring_alert_policy.health_check_unhealthy.name
  }
}

output "notification_channel_ids" {
  description = "List of notification channel IDs"
  value       = [for ch in google_monitoring_notification_channel.email : ch.id]
}

output "log_based_metric_names" {
  description = "Names of the log-based metrics created"
  value = {
    health_check_failures = google_logging_metric.health_check_failures.name
  }
}
