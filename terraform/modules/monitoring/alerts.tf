# =============================================================================
# SLI/SLO ALERT POLICIES (issue #1613)
# =============================================================================
# Alerts on the log-based metrics defined in log_metrics.tf. The legacy
# count-based rpc_errors alert that soaked alongside rpc_error_rate was
# retired in #2623: zero incidents on either side during the overlap, and
# its unique signal (ERROR logs with non-5xx responses) is a subset of the
# any-ERROR-log all_errors policy in main.tf.
#
# Naming note: cloud_functions/src/gcpAlertsHandler.ts classifies incident
# severity (page vs warn) by substring-matching the CONDITION display name.
# "RPC errors ..." / "... latency ..." / "... SLO burn rate ..." → warn
# (daily-aggregated GitHub issue); "Database ..." → page. The classifier's
# unit tests pin the exact strings used here — renaming a condition requires
# updating cloud_functions/src/__tests__/gcpAlertsHandler.test.ts (#2622).

# Per-RPC 5xx error fraction. Numerator and denominator come from the same
# rpc_requests metric (filtered by the status label), so the ratio's labels
# always join. The min-error-rate guard suppresses low-traffic noise where a
# single failure is a large fraction of a handful of requests.
resource "google_monitoring_alert_policy" "rpc_error_rate" {
  project      = var.project_id
  display_name = "RPC Error Rate High - ${var.service_name}"
  combiner     = "OR"
  severity     = "ERROR"

  conditions {
    display_name = "RPC errors exceed ${var.rpc_error_rate_threshold * 100}% of requests"

    condition_prometheus_query_language {
      query = <<-EOT
        (
          sum by (rpc_method) (rate(logging_googleapis_com:user_rpc_requests_${var.environment}{status=~"5.."}[5m]))
          /
          sum by (rpc_method) (rate(logging_googleapis_com:user_rpc_requests_${var.environment}[5m]))
        ) > ${var.rpc_error_rate_threshold}
        and
        sum by (rpc_method) (rate(logging_googleapis_com:user_rpc_requests_${var.environment}{status=~"5.."}[5m])) > ${var.rpc_error_rate_min_errors_per_second}
      EOT

      duration = "600s"
    }
  }

  notification_channels = local.notification_channel_ids

  documentation {
    content   = <<-EOT
      An RPC method on ${var.service_name} is returning more than
      ${var.rpc_error_rate_threshold * 100}% 5xx responses, sustained for 10 minutes.

      **Service:** ${var.service_name}
      **Environment:** ${var.environment}
      **Failing RPC:** see the `rpc_method` label on this incident.

      **Triage:**
      1. Error logs for the RPC (add the method name to the query): https://console.cloud.google.com/logs/query;query=resource.type%3D%22cloud_run_revision%22%0Aresource.labels.service_name%3D%22${var.service_name}%22%0Aseverity%3E%3DERROR?project=${var.project_id}
      2. Compare with the per-RPC latency and traffic dashboards — a deploy,
         a dependency outage, and a traffic spike look different there.
      3. The "Server Error Logged" alert may fire alongside this one; they
         usually share a root cause (an ERROR log on the failing requests).
    EOT
    mime_type = "text/markdown"
  }

  alert_strategy {
    auto_close = "604800s" # 7 days
  }

  depends_on = [google_logging_metric.rpc_requests]
}

# Per-RPC P95 latency, two tiers (#2622 calibration from prod data):
#   - interactive RPCs alert at rpc_p95_latency_threshold_ms (2s default);
#   - slow-class RPCs (AI generation, media upload, synchronous third-party
#     calls — rpc_p95_slow_class_regex) alert at
#     rpc_p95_slow_latency_threshold_ms (8s default) instead of being
#     excluded outright, so a hung provider still surfaces.
# Streaming RPCs are excluded from both tiers: their duration_ms is the
# whole stream lifetime (minutes for chat/event streams), not a request
# latency. Cold-start tails (e.g. GetHomeView P50 ~190ms but P95 >2s at low
# traffic) deliberately stay in the interactive tier: sustained-15m plus
# missing-data=INACTIVE means isolated cold starts cannot page, and a real
# sustained regression on the app's front door should. Revisit membership
# and thresholds when prod traffic ramps.
resource "google_monitoring_alert_policy" "rpc_p95_latency" {
  project      = var.project_id
  display_name = "RPC P95 Latency High - ${var.service_name}"
  combiner     = "OR"
  severity     = "WARNING"

  conditions {
    display_name = "P95 latency above ${var.rpc_p95_latency_threshold_ms}ms for a non-streaming RPC"

    condition_threshold {
      filter          = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.rpc_request_duration.name}\" AND resource.type=\"${local.log_resource_type}\" AND NOT metric.label.rpc_method = monitoring.regex.full_match(\".*/Stream.*\") AND NOT metric.label.rpc_method = monitoring.regex.full_match(\"${var.rpc_p95_slow_class_regex}\")"
      comparison      = "COMPARISON_GT"
      threshold_value = var.rpc_p95_latency_threshold_ms
      duration        = "900s"

      # Log-based series are sparse: without this, one slow request followed
      # by silence keeps the condition "violating" for the whole duration
      # window and opens an incident (observed in dev on the first
      # GetHomeView cold start). Missing data must mean inactive.
      evaluation_missing_data = "EVALUATION_MISSING_DATA_INACTIVE"

      aggregations {
        alignment_period     = "300s"
        per_series_aligner   = "ALIGN_PERCENTILE_95"
        cross_series_reducer = "REDUCE_MAX"
        group_by_fields      = ["metric.label.rpc_method"]
      }

      trigger {
        count = 1
      }
    }
  }

  conditions {
    display_name = "P95 latency above ${var.rpc_p95_slow_latency_threshold_ms}ms for a slow-class RPC"

    condition_threshold {
      filter          = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.rpc_request_duration.name}\" AND resource.type=\"${local.log_resource_type}\" AND metric.label.rpc_method = monitoring.regex.full_match(\"${var.rpc_p95_slow_class_regex}\")"
      comparison      = "COMPARISON_GT"
      threshold_value = var.rpc_p95_slow_latency_threshold_ms
      duration        = "900s"

      evaluation_missing_data = "EVALUATION_MISSING_DATA_INACTIVE"

      aggregations {
        alignment_period     = "300s"
        per_series_aligner   = "ALIGN_PERCENTILE_95"
        cross_series_reducer = "REDUCE_MAX"
        group_by_fields      = ["metric.label.rpc_method"]
      }

      trigger {
        count = 1
      }
    }
  }

  notification_channels = local.notification_channel_ids

  documentation {
    content   = <<-EOT
      A non-streaming RPC on ${var.service_name} has had P95 latency above
      its class threshold for 15 minutes: ${var.rpc_p95_latency_threshold_ms}ms
      for interactive RPCs, ${var.rpc_p95_slow_latency_threshold_ms}ms for the
      slow class (${var.rpc_p95_slow_class_regex}).

      **Service:** ${var.service_name}
      **Environment:** ${var.environment}
      **Slow RPC:** see the `rpc_method` label on this incident.

      **Triage:**
      1. Check the DB health dashboard first — per-request DB time
         (db_request_duration) and slow-query rate tell you whether the time
         is going to Postgres.
      2. Check the Go runtime dashboard for GC pressure or a goroutine climb.
      3. Recent deploy? Compare revisions: https://console.cloud.google.com/run/detail/${var.region}/${var.service_name}/revisions?project=${var.project_id}
      4. If only this RPC is slow and it calls an external provider (AI,
         geocoding, email), check that dependency's health.
    EOT
    mime_type = "text/markdown"
  }

  alert_strategy {
    auto_close = "604800s" # 7 days
  }
}

# DB pool saturation. Utilization is precomputed server-side (in_use/max_open
# on the pool-stats log line), so this fires on a pool running out of
# connections, not on a busy connection in an idle pool.
# ALIGN_PERCENTILE_99 approximates the worst instance when several run.
resource "google_monitoring_alert_policy" "db_pool_saturation" {
  project      = var.project_id
  display_name = "Database Pool Saturation - ${var.service_name}"
  combiner     = "OR"
  severity     = "ERROR"

  conditions {
    display_name = "Database connection pool utilization above ${var.db_pool_saturation_threshold * 100}%"

    condition_threshold {
      filter          = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.db_pool_utilization.name}\" AND resource.type=\"${local.log_resource_type}\""
      comparison      = "COMPARISON_GT"
      threshold_value = var.db_pool_saturation_threshold
      duration        = "600s"

      # Pool stats stop when the instance scales to zero; treat that as
      # inactive rather than letting a stale violating point ride.
      evaluation_missing_data = "EVALUATION_MISSING_DATA_INACTIVE"

      aggregations {
        alignment_period     = "300s"
        per_series_aligner   = "ALIGN_PERCENTILE_99"
        cross_series_reducer = "REDUCE_MAX"
      }

      trigger {
        count = 1
      }
    }
  }

  notification_channels = local.notification_channel_ids

  documentation {
    content   = <<-EOT
      The ${var.service_name} DB connection pool has been more than
      ${var.db_pool_saturation_threshold * 100}% utilized for 10 minutes. Requests are
      about to queue on pool waits (watch db_pool_wait_count /
      db_pool_wait_seconds on the "db pool stats" log lines).

      **Service:** ${var.service_name}
      **Environment:** ${var.environment}

      **Triage:**
      1. Current pool stats: curl ${var.service_url}/health | jq '.dependencies[] | select(.name=="database")'
      2. Slow-query rate on the DB health dashboard — long queries hold
         connections; fix the query before raising the pool limit.
      3. Cross-check the Cloud SQL "Database Connections High" alert — if the
         instance is also near max_connections, raising the app pool limit
         will make things worse.
    EOT
    mime_type = "text/markdown"
  }

  alert_strategy {
    auto_close = "3600s"
  }
}

# Self-hosted only: the server has stopped logging. It writes "db pool stats"
# every 15 seconds for as long as it runs, so silence means the server is down
# or the log shipper is — and a dead shipper otherwise looks exactly like a
# healthy, quiet server, since every other alert here reads the same logs. On
# Cloud Run the platform ships the logs and dev scales to zero, so it is not
# created there.
resource "google_monitoring_alert_policy" "server_logs_absent" {
  count = local.self_hosted ? 1 : 0

  project      = var.project_id
  display_name = "Server Logs Absent - ${var.service_name}"
  combiner     = "OR"
  severity     = "CRITICAL"

  conditions {
    display_name = "No server log entries for ${var.log_absence_minutes} minutes"

    condition_absent {
      filter   = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.db_pool["open"].name}\" AND resource.type=\"${local.log_resource_type}\""
      duration = "${var.log_absence_minutes * 60}s"

      # db_pool_* are DELTA distributions, which take a percentile aligner
      # (as db_pool_saturation does) but not ALIGN_COUNT. Absence is judged on
      # whether any point arrives, so which percentile does not matter.
      aggregations {
        alignment_period     = "300s"
        per_series_aligner   = "ALIGN_PERCENTILE_50"
        cross_series_reducer = "REDUCE_MAX"
      }

      trigger {
        count = 1
      }
    }
  }

  notification_channels = local.notification_channel_ids

  documentation {
    content   = <<-EOT
      ${var.service_name} has written no logs to Cloud Logging for
      ${var.log_absence_minutes} minutes. Either the server is down or the log
      shipper beside it is; until this clears, every other alert in this
      module is blind.

      **Service:** ${var.service_name}
      **Environment:** ${var.environment}

      **Triage:**
      1. Is the server up? curl ${var.service_url}/readyz
      2. Up but silent: the log shipper is down or cannot authenticate. Check
         its container and logs on the host (the deployment's runbook names them).
    EOT
    mime_type = "text/markdown"
  }

  alert_strategy {
    auto_close = "3600s"
  }
}

# Outbound SMS delivery failure fraction (#2569).
#
# Numerator and denominator both come from sms_delivery_outcomes, filtered by
# the message_status label, so the ratio's labels always join — the same intent
# as rpc_error_rate above.
#
# **Not** PromQL, unlike rpc_error_rate, and the difference is forced rather
# than stylistic. Cloud Monitoring validates a PromQL condition against the
# Prometheus series catalog, which a log-based metric only enters once it has
# received a data point — and log-based metrics do not backfill. rpc_error_rate
# gets away with it because RPC traffic is constant, so its series exists within
# moments of the metric. Platform SMS is flag-off in prod (`-platform-sms-enabled`,
# see docs/sms_notifications.md), so `user_sms_delivery_outcomes_prod` has no
# series at all and a PromQL policy is rejected outright:
#
#   Error 400: The following PromQL metric(s) are invalid:
#   logging_googleapis_com:user_sms_delivery_outcomes_prod
#
# A condition_threshold validates against the *metric descriptor*, which exists
# the moment terraform creates the metric. That is the same construction
# dependency_health_failures uses over its log-based metric, and it means the
# alert can be in place before the channel it watches is ever switched on —
# which is the point, since the first real SMS traffic is exactly when a
# deliverability problem would appear unwatched.
#
# Applying this to a *fresh* environment takes two passes. The descriptor is
# created seconds before the policy that references it, and Cloud Monitoring has
# not registered it yet, so the first apply fails on the policy with the metric
# already created. depends_on does not help — the ordering is right, the
# propagation is what lags. Re-running the apply succeeds. Nothing to fix here;
# it is worth knowing before treating the first failure as a config error.
#
# The two conditions are combined with AND, not OR, because the second one is
# the min-volume floor rather than an independent trigger. Platform SMS is
# low-volume by design, so without a floor a single undelivered message in an
# otherwise quiet half-hour is a 100% failure rate and pages someone about
# nothing. A native ratio threshold cannot express "and at least N failures" in
# one condition, so the floor is its own condition and both must be in violation
# at once. Set so a lone failure cannot fire; two inside the window can.
#
# Condition naming: cloud_functions/src/gcpAlertsHandler.ts substring-matches
# the CONDITION display name to decide page-vs-warn, and an unmatched name
# classifies as "unknown" — which routes as a per-incident issue with no
# claude-fix, the misclassified state #2622 found. **Both** condition names below
# have to match, since either may be the one named on an incident; both contain
# "delivery failures", which the WARN pattern added alongside this policy
# matches, and the classifier's unit tests pin the ratio string.
resource "google_monitoring_alert_policy" "sms_delivery_failure_rate" {
  project      = var.project_id
  display_name = "SMS Delivery Failures High - ${var.service_name}"
  combiner     = "AND"
  severity     = "WARNING"

  conditions {
    display_name = "SMS delivery failures exceed ${var.sms_delivery_failure_rate_threshold * 100}% of messages"

    condition_threshold {
      filter             = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.sms_delivery_outcomes.name}\" AND resource.type=\"${local.log_resource_type}\" AND metric.labels.message_status=monitoring.regex.full_match(\"failed|undelivered\")"
      denominator_filter = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.sms_delivery_outcomes.name}\" AND resource.type=\"${local.log_resource_type}\""
      comparison         = "COMPARISON_GT"
      threshold_value    = var.sms_delivery_failure_rate_threshold
      duration           = "900s"

      aggregations {
        alignment_period     = "1800s"
        per_series_aligner   = "ALIGN_DELTA"
        cross_series_reducer = "REDUCE_SUM"
      }

      denominator_aggregations {
        alignment_period     = "1800s"
        per_series_aligner   = "ALIGN_DELTA"
        cross_series_reducer = "REDUCE_SUM"
      }
    }
  }

  conditions {
    display_name = "SMS delivery failures above the minimum volume floor"

    condition_threshold {
      filter          = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.sms_delivery_outcomes.name}\" AND resource.type=\"${local.log_resource_type}\" AND metric.labels.message_status=monitoring.regex.full_match(\"failed|undelivered\")"
      comparison      = "COMPARISON_GT"
      threshold_value = var.sms_delivery_failure_min_count
      duration        = "900s"

      aggregations {
        alignment_period     = "1800s"
        per_series_aligner   = "ALIGN_DELTA"
        cross_series_reducer = "REDUCE_SUM"
      }
    }
  }

  notification_channels = local.notification_channel_ids

  documentation {
    content   = <<-EOT
      More than ${var.sms_delivery_failure_rate_threshold * 100}% of outbound SMS
      on ${var.service_name} reached a terminal failure, sustained for 15 minutes.

      **Service:** ${var.service_name}
      **Environment:** ${var.environment}

      **Triage — start with the `twilio_code` label, which names the fix:**
      1. **30007** — carrier filtering. Content or sender reputation is being
         blocked upstream; a campaign/content change is the remedy, not a retry.
      2. **21610** — the recipient opted out via STOP. Expected in small
         numbers; a spike means we are sending to people who have left.
      3. **30003 / 30005** — unreachable or unknown handset. Usually stale
         phone numbers rather than a platform fault.
      4. Delivery logs: https://console.cloud.google.com/logs/query;query=jsonPayload.operation%3D%22SMSStatusCallback%22?project=${var.project_id}

      A broad spike across every code points at Twilio or the Messaging Service
      configuration rather than at any individual message.
    EOT
    mime_type = "text/markdown"
  }

  alert_strategy {
    auto_close = "604800s" # 7 days
  }

  depends_on = [google_logging_metric.sms_delivery_outcomes]
}

# P95 time-to-inbox for sign-in-code email (#2862).
#
# The measurement #2862 exists to produce. `email_code_age` only ever proxied
# this — it bundles delivery with however long someone took to read and type the
# code, and it exists only for codes that were actually used. This reads the
# real accepted→delivered interval, from the timestamp the send stamps into a
# Mailgun custom variable and the delivery webhook reads back.
#
# Scoped to email_type="email_code" deliberately. A digest arriving two minutes
# late is invisible; a sign-in code arriving two minutes late strands someone at
# a code-entry screen with no way in, now that #2571/#2864 has made the mailed
# code the only way into an email account. The other email types share the
# metric but not the urgency, so they are not worth waking anyone for.
#
# condition_threshold rather than PromQL, for the reason spelled out on the SMS
# policy above: PromQL is validated against the Prometheus series catalog, which
# a log-based metric only enters once it has data, and this metric has very
# little. See #2873.
#
# ALIGN_DELTA then REDUCE_PERCENTILE_95 is the house pattern for a DISTRIBUTION
# (dashboards.tf documents it): aligning per series keeps the distribution
# intact, and reducing across series merges the Cloud Run instances before the
# percentile is taken. Taking a percentile per-instance and then max-ing would
# report the worst instance rather than the worst experience.
#
# Volume-floor gate added in #2923 (second firing in three days, same policy).
# The alert fired twice at low volume (P95 = 24.96s and 12.16s on single
# messages) without any real deliverability regression, which is exactly the
# "if it proves noisy" branch the original docstring named. Fix: a second
# condition on the delivered *count* (combiner = "AND"), so a lone outlier
# cannot page. Floor set above current peak volume (~2/10 min) so the alert is
# silent until real volume arrives; the P95 threshold stays at 10s so a genuine
# slowdown still pages once the floor allows it.
#
# The floor reads the email_deliveries counter, not this policy's own
# distribution. A threshold condition needs a scalar and no aggregation produces
# one from a DELTA DISTRIBUTION: ALIGN_DELTA leaves it a distribution, and
# ALIGN_COUNT is rejected by the API for DISTRIBUTION value types (it counts
# points rather than samples in any case). log_metrics.tf carries the counter
# and the requirement that its filter stay identical to the latency metric's.
#
# Display names: the P95 condition name is pinned in
# cloud_functions/src/__tests__/gcpAlertsHandler.test.ts and must contain
# "latency" to classify as warn via /latency/i in gcpAlertsHandler.ts:100.
# The floor condition name also contains "latency" for the same reason (#2622).
resource "google_monitoring_alert_policy" "email_code_delivery_latency" {
  project      = var.project_id
  display_name = "Email Sign-in Code Delivery Slow - ${var.service_name}"
  combiner     = "AND"
  severity     = "WARNING"

  conditions {
    display_name = "P95 delivery latency above ${var.email_code_p95_latency_threshold_ms}ms for sign-in codes"

    condition_threshold {
      filter          = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.email_delivery_latency.name}\" AND resource.type=\"${local.log_resource_type}\" AND metric.labels.email_type=\"email_code\""
      comparison      = "COMPARISON_GT"
      threshold_value = var.email_code_p95_latency_threshold_ms
      duration        = "300s"

      aggregations {
        alignment_period     = "600s"
        per_series_aligner   = "ALIGN_DELTA"
        cross_series_reducer = "REDUCE_PERCENTILE_95"
      }
    }
  }

  conditions {
    display_name = "Sign-in code delivery latency: minimum sample floor of ${var.email_code_delivered_min_count} per 10 min"

    condition_threshold {
      filter          = "metric.type=\"logging.googleapis.com/user/${google_logging_metric.email_deliveries.name}\" AND resource.type=\"${local.log_resource_type}\" AND metric.labels.email_type=\"email_code\""
      comparison      = "COMPARISON_GT"
      threshold_value = var.email_code_delivered_min_count
      duration        = "300s"

      aggregations {
        alignment_period     = "600s"
        per_series_aligner   = "ALIGN_DELTA"
        cross_series_reducer = "REDUCE_SUM"
      }
    }
  }

  notification_channels = local.notification_channel_ids

  documentation {
    content   = <<-EOT
      The P95 time between accepting a sign-in code for delivery and the mail
      provider reporting it delivered exceeded
      ${var.email_code_p95_latency_threshold_ms}ms on ${var.service_name}.

      **Service:** ${var.service_name}
      **Environment:** ${var.environment}

      **Why this one matters more than it looks:** a mailed one-time code is the
      only way into an email account (#2571). A code that lands late is not a
      slow notification, it is someone sitting at a code-entry screen unable to
      get in — and they will usually request another, so the symptom compounds.

      **Triage:**
      1. Delivery events, with per-message latency:
         https://console.cloud.google.com/logs/query;query=jsonPayload.operation%3D%22EmailStatusWebhook%22?project=${var.project_id}
      2. Check whether `email delivery failed` is also rising — a provider having
         trouble usually slows before it starts bouncing.
      3. Compare against other `email_type` values on the same metric. Slowness
         confined to `email_code` points at content or reputation on that
         template; slowness across every type points at the provider.
      4. At low volume the P95 may be one or two messages. Confirm the sample
         size before treating this as a trend.
    EOT
    mime_type = "text/markdown"
  }

  alert_strategy {
    auto_close = "604800s" # 7 days
  }

  depends_on = [
    google_logging_metric.email_delivery_latency,
    google_logging_metric.email_deliveries,
  ]
}
