# =============================================================================
# LOG-BASED METRICS FOR SLI/SLO (issue #1613)
# =============================================================================
# Time series extracted from the server's structured JSON logs. These replace
# the abandoned Prometheus remote-write pipeline (PR #1612): Cloud Monitoring
# has no Prometheus write endpoint reachable from Cloud Run, and every value
# we need already rides on an existing log line.
#
# Source log lines (field names are a contract with the Go side):
# - "http request"      server/middleware/logging.go   (every HTTP request)
# - "request db stats"  server/middleware/query_stats.go (per-request DB time)
# - "slow query"        server/storage/instrumented_db.go (queries >= 100ms)
# - "db pool stats"     server/statslog + server/main.go (every 15s)
# - "go runtime stats"  server/statslog + server/main.go (every 30s)
#
# Log-based metrics only support DELTA counters (count of matching entries)
# and DELTA distributions (extracted values). Gauge-like signals (pool sizes,
# goroutines, heap) are therefore distributions of sampled values — chart and
# alert on them with percentile aligners.
#
# NOTE: log-based metrics have no backfill; a metric only has data from the
# moment it is created. Apply this file before building dashboards or alerts
# so calibration data accumulates.

# -----------------------------------------------------------------------------
# RPC traffic
# -----------------------------------------------------------------------------

# Count of RPC requests by method and HTTP status. The error-rate alert and
# dashboards derive 5xx fractions by filtering the status label, so numerator
# and denominator come from the same series and labels always join.
resource "google_logging_metric" "rpc_requests" {
  project     = var.project_id
  name        = "rpc_requests_${var.environment}"
  description = "Count of Connect RPC requests by method and HTTP status, from the request-logging middleware"

  filter = <<-EOT
    ${local.log_source_filter}
    jsonPayload.message="http request"
    jsonPayload.rpc_method!=""
  EOT

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"

    labels {
      key         = "rpc_method"
      value_type  = "STRING"
      description = "Service/Method, e.g. GearService/SaveGear"
    }

    labels {
      key         = "status"
      value_type  = "STRING"
      description = "HTTP status code of the response"
    }
  }

  label_extractors = {
    "rpc_method" = "EXTRACT(jsonPayload.rpc_method)"
    "status"     = "EXTRACT(jsonPayload.status)"
  }
}

# Per-request latency distribution by RPC method. duration_ms covers the whole
# request; for streaming RPCs (rpc_method Stream*) that is the stream lifetime,
# which is why the P95 alert excludes them.
resource "google_logging_metric" "rpc_request_duration" {
  project     = var.project_id
  name        = "rpc_request_duration_${var.environment}"
  description = "Distribution of RPC request duration in milliseconds by method"

  filter = <<-EOT
    ${local.log_source_filter}
    jsonPayload.message="http request"
    jsonPayload.rpc_method!=""
  EOT

  value_extractor = "EXTRACT(jsonPayload.duration_ms)"

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "DISTRIBUTION"
    unit        = "ms"

    labels {
      key         = "rpc_method"
      value_type  = "STRING"
      description = "Service/Method, e.g. GearService/SaveGear"
    }
  }

  label_extractors = {
    "rpc_method" = "EXTRACT(jsonPayload.rpc_method)"
  }

  bucket_options {
    exponential_buckets {
      num_finite_buckets = 13
      growth_factor      = 2
      scale              = 5 # 5ms .. ~41s
    }
  }
}

# -----------------------------------------------------------------------------
# Database
# -----------------------------------------------------------------------------

# Total DB time per request by RPC method — the UNCENSORED DB latency signal
# (the slow-query metrics below only see queries >= 100ms).
resource "google_logging_metric" "db_request_duration" {
  project     = var.project_id
  name        = "db_request_duration_${var.environment}"
  description = "Distribution of total database time per request in milliseconds by RPC method"

  filter = <<-EOT
    ${local.log_source_filter}
    jsonPayload.message="request db stats"
    jsonPayload.rpc_method!=""
  EOT

  value_extractor = "EXTRACT(jsonPayload.db_duration_ms)"

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "DISTRIBUTION"
    unit        = "ms"

    labels {
      key         = "rpc_method"
      value_type  = "STRING"
      description = "Service/Method, e.g. GearService/SaveGear"
    }
  }

  label_extractors = {
    "rpc_method" = "EXTRACT(jsonPayload.rpc_method)"
  }

  bucket_options {
    exponential_buckets {
      num_finite_buckets = 14
      growth_factor      = 2
      scale              = 1 # 1ms .. ~16s
    }
  }
}

# Count of slow queries (>= 100ms) by kind. Distributions can't be
# rate-aligned, so slow-query volume gets its own counter.
resource "google_logging_metric" "db_slow_queries" {
  project     = var.project_id
  name        = "db_slow_queries_${var.environment}"
  description = "Count of SQL queries exceeding the slow-query threshold (100ms) by query kind"

  filter = <<-EOT
    ${local.log_source_filter}
    jsonPayload.message="slow query"
  EOT

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"

    labels {
      key         = "query_kind"
      value_type  = "STRING"
      description = "Leading SQL keyword: select, insert, update, delete, other"
    }
  }

  label_extractors = {
    "query_kind" = "EXTRACT(jsonPayload.query_kind)"
  }
}

# Duration distribution of slow queries by kind. CENSORED at the 100ms
# slow-query log threshold — this is "how slow are the slow ones", NOT
# overall DB latency (that is db_request_duration above).
resource "google_logging_metric" "db_slow_query_duration" {
  project     = var.project_id
  name        = "db_slow_query_duration_${var.environment}"
  description = "Distribution of slow-query (>=100ms) duration in milliseconds by query kind; censored below the slow-query log threshold"

  filter = <<-EOT
    ${local.log_source_filter}
    jsonPayload.message="slow query"
  EOT

  value_extractor = "EXTRACT(jsonPayload.duration_ms)"

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "DISTRIBUTION"
    unit        = "ms"

    labels {
      key         = "query_kind"
      value_type  = "STRING"
      description = "Leading SQL keyword: select, insert, update, delete, other"
    }
  }

  label_extractors = {
    "query_kind" = "EXTRACT(jsonPayload.query_kind)"
  }

  bucket_options {
    exponential_buckets {
      num_finite_buckets = 8
      growth_factor      = 2
      scale              = 100 # 100ms .. ~13s
    }
  }
}

# -----------------------------------------------------------------------------
# DB connection pool (sampled every 15s per instance)
# -----------------------------------------------------------------------------

# Pool counts share one linear bucket layout (0..50 connections). With
# multiple Cloud Run instances the series mixes instances; use percentile
# aligners (P99 ~= worst instance) rather than means when alerting.
locals {
  db_pool_bucket_options = {
    num_finite_buckets = 50
    width              = 1
    offset             = 0
  }
  db_pool_gauges = {
    open   = "Open connections in the DB pool (sampled)"
    in_use = "In-use connections in the DB pool (sampled)"
    idle   = "Idle connections in the DB pool (sampled)"
  }
}

resource "google_logging_metric" "db_pool" {
  for_each = local.db_pool_gauges

  project     = var.project_id
  name        = "db_pool_${each.key}_${var.environment}"
  description = each.value

  filter = <<-EOT
    ${local.log_source_filter}
    jsonPayload.message="db pool stats"
  EOT

  value_extractor = "EXTRACT(jsonPayload.db_pool_${each.key})"

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "DISTRIBUTION"
    unit        = "1"
  }

  bucket_options {
    linear_buckets {
      num_finite_buckets = local.db_pool_bucket_options.num_finite_buckets
      width              = local.db_pool_bucket_options.width
      offset             = local.db_pool_bucket_options.offset
    }
  }
}

# Pool utilization (in_use / max_open), precomputed server-side so the
# saturation alert is a single-metric threshold. Against the pool's limit, not
# the connections open right now: an idle server holds one open connection, and
# one query against it is not a saturated pool.
resource "google_logging_metric" "db_pool_utilization" {
  project     = var.project_id
  name        = "db_pool_utilization_${var.environment}"
  description = "DB pool utilization (in_use / max_open, 0..1), precomputed by the server every 15s"

  filter = <<-EOT
    ${local.log_source_filter}
    jsonPayload.message="db pool stats"
  EOT

  value_extractor = "EXTRACT(jsonPayload.db_pool_utilization)"

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "DISTRIBUTION"
    unit        = "1"
  }

  bucket_options {
    linear_buckets {
      num_finite_buckets = 20
      width              = 0.05
      offset             = 0
    }
  }
}

# -----------------------------------------------------------------------------
# Go runtime (sampled every 30s per instance)
# -----------------------------------------------------------------------------

resource "google_logging_metric" "go_goroutines" {
  project     = var.project_id
  name        = "go_goroutines_${var.environment}"
  description = "Goroutine count (sampled); a sustained climb indicates a goroutine leak"

  filter = <<-EOT
    ${local.log_source_filter}
    jsonPayload.message="go runtime stats"
  EOT

  value_extractor = "EXTRACT(jsonPayload.go_goroutines)"

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "DISTRIBUTION"
    unit        = "1"
  }

  bucket_options {
    exponential_buckets {
      num_finite_buckets = 14
      growth_factor      = 2
      scale              = 1 # 1 .. ~16k goroutines
    }
  }
}

resource "google_logging_metric" "go_heap_inuse_bytes" {
  project     = var.project_id
  name        = "go_heap_inuse_bytes_${var.environment}"
  description = "Heap in-use bytes (sampled); watch against the Cloud Run memory limit"

  filter = <<-EOT
    ${local.log_source_filter}
    jsonPayload.message="go runtime stats"
  EOT

  value_extractor = "EXTRACT(jsonPayload.heap_inuse_bytes)"

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "DISTRIBUTION"
    unit        = "By"
  }

  bucket_options {
    exponential_buckets {
      num_finite_buckets = 12
      growth_factor      = 2
      scale              = 1048576 # 1MiB .. ~4GiB
    }
  }
}

resource "google_logging_metric" "go_gc_pause_ms" {
  project     = var.project_id
  name        = "go_gc_pause_ms_${var.environment}"
  description = "Mean GC pause in milliseconds between runtime-stats samples"

  filter = <<-EOT
    ${local.log_source_filter}
    jsonPayload.message="go runtime stats"
  EOT

  value_extractor = "EXTRACT(jsonPayload.gc_pause_ms)"

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "DISTRIBUTION"
    unit        = "ms"
  }

  bucket_options {
    exponential_buckets {
      num_finite_buckets = 14
      growth_factor      = 2
      scale              = 0.1 # 0.1ms .. ~1.6s
    }
  }
}

# -----------------------------------------------------------------------------
# Email sign-in codes (#2571)
# -----------------------------------------------------------------------------

# How long a mailed sign-in code takes to come back — issued to verified.
#
# This is a PROXY for deliverability, not a measurement of it. It includes the
# time the person spent switching to their inbox, reading, and typing, and it is
# only observable for codes that were actually used. Nothing in the system sees
# when a message lands in an inbox: Mailgun reports only that it accepted the
# message, and the email counterpart of the Twilio delivery callbacks (#2569)
# does not exist yet.
#
# Read it as a threshold, not a number: a p95 in the tens of seconds means
# delivery is not the bottleneck. A p95 in minutes means it is, and that must be
# resolved before password sign-in is removed — at that point the mailed code is
# the ONLY way into an email account, so a slow send is a lockout.
resource "google_logging_metric" "email_code_age" {
  project     = var.project_id
  name        = "email_code_age_${var.environment}"
  description = "Distribution of seconds between issuing an email sign-in code and it being verified (delivery + human entry)"

  filter = <<-EOT
    ${local.log_source_filter}
    jsonPayload.message="email code verified"
  EOT

  value_extractor = "EXTRACT(jsonPayload.code_age_sec)"

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "DISTRIBUTION"
    unit        = "s"
  }

  bucket_options {
    exponential_buckets {
      num_finite_buckets = 12
      growth_factor      = 2
      scale              = 1 # 1s .. ~68min
    }
  }
}

# Failed code verifications by reason. The reasons are deliberately
# indistinguishable to the caller (one collapsed error, so the response cannot
# reveal whether a code is outstanding) but are separated here, where only
# operators can see them.
#
# Watch the shape, not the volume: a rise in "no_outstanding_code" or "expired"
# points at delivery, while a rise in "wrong_code" alongside steady successes is
# ordinary typing. A rise in "attempts_exhausted" is the one that suggests
# someone guessing.
resource "google_logging_metric" "email_code_failures" {
  project     = var.project_id
  name        = "email_code_failures_${var.environment}"
  description = "Count of failed email sign-in code verifications by reason"

  filter = <<-EOT
    ${local.log_source_filter}
    jsonPayload.message="email code verification failed"
  EOT

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"

    labels {
      key         = "reason"
      value_type  = "STRING"
      description = "wrong_code, expired, no_outstanding_code, attempts_exhausted, or test_account_code_mismatch"
    }
  }

  label_extractors = {
    "reason" = "EXTRACT(jsonPayload.reason)"
  }
}

# Interactive email sign-ins split by credential. Watching email_password fall to
# zero is the gate on removing the password path (#2571) — that is the whole
# reason EmailLogin labels the two differently rather than calling both
# "email_password".
resource "google_logging_metric" "email_signins_by_credential" {
  project     = var.project_id
  name        = "email_signins_by_credential_${var.environment}"
  description = "Count of successful email sign-ins by credential (email_code vs the legacy email_password)"

  filter = <<-EOT
    ${local.log_source_filter}
    jsonPayload.message="user logged in"
    (jsonPayload.login_method="email_code" OR jsonPayload.login_method="email_password")
  EOT

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"

    labels {
      key         = "login_method"
      value_type  = "STRING"
      description = "email_code or email_password"
    }
  }

  label_extractors = {
    "login_method" = "EXTRACT(jsonPayload.login_method)"
  }
}

# True accepted→delivered latency, from the mail provider's delivery webhook.
#
# This is the measurement `email_code_age` above only proxies: it excludes human
# entry time entirely, and it exists for every delivered message rather than only
# the ones whose code someone typed. The send stamps its own accept time into a
# provider custom variable and the webhook reads it back, so no correlation
# table is involved.
#
# Split by email_type because the tolerances differ by orders of magnitude: a
# digest arriving two minutes late is invisible, a sign-in code arriving two
# minutes late strands someone at a code-entry screen.
#
# Only counts this environment's own sends. Mailgun webhooks are configured per
# domain, so environments sharing one sending domain each receive every event;
# the handler drops what it did not send. Without that, a dev simulation run
# would dominate production's delivery numbers.
resource "google_logging_metric" "email_delivery_latency" {
  project     = var.project_id
  name        = "email_delivery_latency_${var.environment}"
  description = "Distribution of milliseconds between accepting an email for delivery and it being delivered, by email type"

  filter = <<-EOT
    ${local.log_source_filter}
    jsonPayload.message="email delivered"
    jsonPayload.delivery_latency_ms>0
  EOT

  value_extractor = "EXTRACT(jsonPayload.delivery_latency_ms)"

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "DISTRIBUTION"
    unit        = "ms"

    labels {
      key         = "email_type"
      value_type  = "STRING"
      description = "email_code, off_app_notification, activity_digest, …"
    }
  }

  label_extractors = {
    "email_type" = "EXTRACT(jsonPayload.email_type)"
  }

  bucket_options {
    exponential_buckets {
      num_finite_buckets = 16
      growth_factor      = 2
      scale              = 100 # 100ms .. ~1.8h
    }
  }
}

# Delivered-message count, over exactly the population email_delivery_latency
# samples.
#
# Redundant-looking next to the distribution above, and not avoidable: an
# alerting threshold condition needs a scalar, and there is no aggregation that
# turns a DELTA DISTRIBUTION into its sample count. ALIGN_DELTA keeps the value
# a distribution, and ALIGN_COUNT — which counts *points*, not samples — is
# rejected outright for DISTRIBUTION value types ("The aligner cannot be applied
# to metrics with kind DELTA and value type DISTRIBUTION"). So the volume floor
# on the sign-in-code latency alert reads this counter instead, the same
# ALIGN_DELTA/REDUCE_SUM shape sms_delivery_outcomes gives the SMS floor.
#
# The filter must stay byte-identical to email_delivery_latency's, including
# `delivery_latency_ms>0`. The floor is only meaningful as a count of the P95's
# own samples; a filter that admitted one more log line would gate the alert on
# a population the percentile never saw.
resource "google_logging_metric" "email_deliveries" {
  project     = var.project_id
  name        = "email_deliveries_${var.environment}"
  description = "Count of delivered emails by email type, matching the population sampled by email_delivery_latency"

  filter = <<-EOT
    ${local.log_source_filter}
    jsonPayload.message="email delivered"
    jsonPayload.delivery_latency_ms>0
  EOT

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"

    labels {
      key         = "email_type"
      value_type  = "STRING"
      description = "email_code, off_app_notification, activity_digest, …"
    }
  }

  label_extractors = {
    "email_type" = "EXTRACT(jsonPayload.email_type)"
  }
}

# Terminal delivery failures and spam complaints, by type.
#
# The send call cannot see any of this — it returns as soon as the provider
# accepts the message. A rising failure rate on email_code is the strongest
# available early warning that people are being locked out.
resource "google_logging_metric" "email_delivery_failures" {
  project     = var.project_id
  name        = "email_delivery_failures_${var.environment}"
  description = "Count of email delivery failures and spam complaints by email type and severity"

  filter = <<-EOT
    ${local.log_source_filter}
    (jsonPayload.message="email delivery failed" OR jsonPayload.message="email marked as spam by recipient")
  EOT

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"

    labels {
      key         = "email_type"
      value_type  = "STRING"
      description = "email_code, off_app_notification, activity_digest, …"
    }
    labels {
      key         = "severity_kind"
      value_type  = "STRING"
      description = "permanent (hard bounce), temporary (deferral), or empty for a spam complaint"
    }
  }

  label_extractors = {
    "email_type"    = "EXTRACT(jsonPayload.email_type)"
    "severity_kind" = "EXTRACT(jsonPayload.bounce_severity)"
  }
}

# Outbound SMS delivery outcomes, by terminal status (#2569).
#
# The counterpart of the email delivery metrics above, for the channel that got
# its status-callback handler first and its measurement last: #2569 built
# HandleSMSStatusCallback and left "define the log-based metric + alert" as
# unfinished ops work, which then sat unbuilt long enough that
# docs/sms_notifications.md began describing it as done.
#
# One counter rather than the email side's two metrics. That split exists
# because email latency needs a DISTRIBUTION and failures need a counter —
# forced by the data, not chosen. SMS has no latency to measure: Twilio reports
# a lifecycle, not a send timestamp we can stamp and read back the way
# server/email.stampDeliveryVariables does. What is left is outcomes, and
# counting successes and failures in a single metric gives the failure *rate* a
# denominator that always joins — the same property that makes the per-RPC
# rpc_error_rate ratio reliable.
#
# twilio_code is the actionable label: 30007 (carrier filtering), 21610 (the
# recipient opted out), and 30003 (unreachable handset) call for three different
# responses, and only the code distinguishes them. It is empty on success.
resource "google_logging_metric" "sms_delivery_outcomes" {
  project     = var.project_id
  name        = "sms_delivery_outcomes_${var.environment}"
  description = "Count of terminal outbound SMS delivery outcomes by status and Twilio error code"

  filter = <<-EOT
    ${local.log_source_filter}
    (jsonPayload.message="sms delivered" OR jsonPayload.message="sms delivery failed")
  EOT

  metric_descriptor {
    metric_kind = "DELTA"
    value_type  = "INT64"
    unit        = "1"

    labels {
      key         = "message_status"
      value_type  = "STRING"
      description = "delivered, failed, or undelivered"
    }
    labels {
      key         = "twilio_code"
      value_type  = "STRING"
      description = "Twilio numeric error code (30007 carrier filtering, 21610 opted out, 30003 unreachable); empty on success"
    }
  }

  label_extractors = {
    "message_status" = "EXTRACT(jsonPayload.message_status)"
    "twilio_code"    = "EXTRACT(jsonPayload.twilio_code)"
  }
}
