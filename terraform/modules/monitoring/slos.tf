# =============================================================================
# SERVICE + SLOs + BURN-RATE ALERTS (issue #2624)
# =============================================================================
# A Cloud Monitoring custom service with two request-based SLOs over the
# log-based metrics in log_metrics.tf, plus multi-window burn-rate alert
# policies. This completes the #1520 observability goals.
#
# Layering: the static threshold alerts in alerts.tf STAY. Their
# min-error-rate guard (rpc_error_rate_min_errors_per_second) has no
# burn-rate equivalent, which matters at low traffic where one 5xx among a
# handful of requests is a huge burn rate. Statics are the guarded floor;
# SLOs are the error-budget layer. Burn-rate conditions are WARNING severity
# and warn-classified by the GitHub auto-filer (display names contain
# "SLO burn rate") until they prove signal — promote fast-burn to page later
# if warranted.
#
# Caveat recorded in docs/server/observability.md: the availability SLI's
# denominator is dominated by high-frequency polling RPCs (ListCommunityEvents
# is ~80%+ of traffic today), so a failure confined to a low-traffic RPC
# barely dents the budget. The per-RPC static error alert is the compensating
# control.

resource "google_monitoring_custom_service" "server" {
  project      = var.project_id
  service_id   = "ripls-server-${var.environment}"
  display_name = "Ripls Server (${var.environment})"
}

# Availability: fraction of RPC responses that are non-5xx, 28-day rolling.
# Numerator and denominator both come from rpc_requests_{env} (bad = 5xx by
# the status label), so labels always join — same design as the
# rpc_error_rate alert.
resource "google_monitoring_slo" "availability" {
  project             = var.project_id
  service             = google_monitoring_custom_service.server.service_id
  slo_id              = "availability"
  display_name        = "Availability ${var.slo_availability_target * 100}% (non-5xx, 28d rolling)"
  goal                = var.slo_availability_target
  rolling_period_days = 28

  request_based_sli {
    good_total_ratio {
      bad_service_filter   = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.rpc_requests.name}\" resource.type=\"cloud_run_revision\" metric.label.status=monitoring.regex.full_match(\"5..\")"
      total_service_filter = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.rpc_requests.name}\" resource.type=\"cloud_run_revision\""
    }
  }
}

# Latency: fraction of interactive RPC responses completing under the same
# threshold the P95 alert uses, 28-day rolling. Streaming RPCs are excluded
# (duration = stream lifetime) and so is the slow class (their honest
# threshold is rpc_p95_slow_latency_threshold_ms; mixing them in would burn
# budget on by-design latency).
resource "google_monitoring_slo" "latency" {
  project             = var.project_id
  service             = google_monitoring_custom_service.server.service_id
  slo_id              = "latency"
  display_name        = "Latency ${var.slo_latency_target * 100}% under ${var.rpc_p95_latency_threshold_ms}ms (28d rolling)"
  goal                = var.slo_latency_target
  rolling_period_days = 28

  request_based_sli {
    distribution_cut {
      distribution_filter = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.rpc_request_duration.name}\" resource.type=\"cloud_run_revision\" NOT metric.label.rpc_method=monitoring.regex.full_match(\".*/Stream.*\") NOT metric.label.rpc_method=monitoring.regex.full_match(\"${var.rpc_p95_slow_class_regex}\")"

      range {
        max = var.rpc_p95_latency_threshold_ms
      }
    }
  }
}

# Multi-window burn-rate alerts, one policy per SLO. Burn rate 1.0 = spending
# budget exactly at the rate that exhausts it at period end; the fast window
# (1h > 14.4x) catches an acute outage within minutes-to-an-hour, the slow
# window (6h > 6x) catches a simmering regression. Standard SRE-workbook
# starting values — recalibrate when prod traffic ramps.
resource "google_monitoring_alert_policy" "slo_availability_burn" {
  project      = var.project_id
  display_name = "Availability SLO Burn Rate - ${var.service_name}"
  combiner     = "OR"
  severity     = "WARNING"

  conditions {
    display_name = "Availability SLO burn rate - fast window (1h)"

    condition_threshold {
      filter          = "select_slo_burn_rate(\"${google_monitoring_slo.availability.name}\", \"3600s\")"
      comparison      = "COMPARISON_GT"
      threshold_value = 14.4
      duration        = "0s"

      trigger {
        count = 1
      }
    }
  }

  conditions {
    display_name = "Availability SLO burn rate - slow window (6h)"

    condition_threshold {
      filter          = "select_slo_burn_rate(\"${google_monitoring_slo.availability.name}\", \"21600s\")"
      comparison      = "COMPARISON_GT"
      threshold_value = 6
      duration        = "0s"

      trigger {
        count = 1
      }
    }
  }

  notification_channels = local.notification_channel_ids

  documentation {
    content   = <<-EOT
      The ${var.service_name} availability SLO
      (${var.slo_availability_target * 100}% non-5xx over 28 days) is burning
      error budget faster than sustainable — fast window means an acute 5xx
      spike, slow window a simmering elevated error rate.

      **Service:** ${var.service_name}
      **Environment:** ${var.environment}

      **Triage:**
      1. Error-budget view: https://console.cloud.google.com/monitoring/services?project=${var.project_id}
      2. Which RPC: the "RPC Error Rate High" alert and the RPC Traffic &
         Errors dashboard break 5xx down by rpc_method.
      3. At low traffic a handful of 5xx can spike the burn rate — check the
         absolute error count before escalating.
    EOT
    mime_type = "text/markdown"
  }

  alert_strategy {
    auto_close = "604800s" # 7 days
  }
}

resource "google_monitoring_alert_policy" "slo_latency_burn" {
  project      = var.project_id
  display_name = "Latency SLO Burn Rate - ${var.service_name}"
  combiner     = "OR"
  severity     = "WARNING"

  conditions {
    display_name = "Latency SLO burn rate - fast window (1h)"

    condition_threshold {
      filter          = "select_slo_burn_rate(\"${google_monitoring_slo.latency.name}\", \"3600s\")"
      comparison      = "COMPARISON_GT"
      threshold_value = 14.4
      duration        = "0s"

      trigger {
        count = 1
      }
    }
  }

  conditions {
    display_name = "Latency SLO burn rate - slow window (6h)"

    condition_threshold {
      filter          = "select_slo_burn_rate(\"${google_monitoring_slo.latency.name}\", \"21600s\")"
      comparison      = "COMPARISON_GT"
      threshold_value = 6
      duration        = "0s"

      trigger {
        count = 1
      }
    }
  }

  notification_channels = local.notification_channel_ids

  documentation {
    content   = <<-EOT
      The ${var.service_name} latency SLO (${var.slo_latency_target * 100}% of
      interactive RPC responses under ${var.rpc_p95_latency_threshold_ms}ms
      over 28 days) is burning error budget faster than sustainable.

      **Service:** ${var.service_name}
      **Environment:** ${var.environment}

      **Triage:**
      1. Error-budget view: https://console.cloud.google.com/monitoring/services?project=${var.project_id}
      2. Which RPC: the RPC Latency dashboard's P95-by-rpc_method widget.
      3. DB health dashboard next — per-request DB time and slow queries say
         whether the time is going to Postgres.
      4. At low traffic, cold starts dominate the tail; compare with the
         Cloud Run instance-count metric before escalating.
    EOT
    mime_type = "text/markdown"
  }

  alert_strategy {
    auto_close = "604800s" # 7 days
  }
}
